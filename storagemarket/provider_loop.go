package storagemarket

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/filecoin-project/boost/api"
	"github.com/filecoin-project/boost/db"
	"github.com/filecoin-project/boost/fundmanager"
	"github.com/filecoin-project/boost/sealingpipeline"
	"github.com/filecoin-project/boost/storagemanager"
	"github.com/filecoin-project/boost/storagemarket/types"
	smtypes "github.com/filecoin-project/boost/storagemarket/types"
	"github.com/filecoin-project/boost/storagemarket/types/dealcheckpoints"
	"github.com/google/uuid"
	"github.com/libp2p/go-eventbus"
	"golang.org/x/xerrors"
)

type acceptDealReq struct {
	rsp      chan acceptDealResp
	deal     *types.ProviderDealState
	dh       *dealHandler
	isImport bool
}

type acceptDealResp struct {
	ri  *api.ProviderDealRejectionInfo
	err error
}

type finishedDealReq struct {
	deal *types.ProviderDealState
	done chan struct{}
}

type publishDealReq struct {
	deal *types.ProviderDealState
	done chan struct{}
}

type storageSpaceDealReq struct {
	deal *types.ProviderDealState
	done chan struct{}
}

func (p *Provider) logFunds(id uuid.UUID, trsp *fundmanager.TagFundsResp) {
	p.dealLogger.Infow(id, "tagged funds for deal",
		"tagged for deal publish", trsp.PublishMessage,
		"tagged for deal collateral", trsp.Collateral,
		"total tagged for publish", trsp.TotalPublishMessage,
		"total tagged for collateral", trsp.TotalCollateral,
		"total available for publish", trsp.AvailablePublishMessage,
		"total available for collateral", trsp.AvailableCollateral)
}

// acceptError is used to distinguish between a regular error and a severe error
type acceptError struct {
	error
	// isSevereError indicates whether the error is severe (eg can't connect
	// to database) or not (eg not enough funds for deal)
	isSevereError bool
	// The reason sent to the client for why their deal was rejected
	reason string
}

func (p *Provider) processDealProposal(deal *types.ProviderDealState) *acceptError {
	// Check that the deal proposal is unique
	if aerr := p.checkDealPropUnique(deal); aerr != nil {
		return aerr
	}

	// Check that the deal uuid is unique
	if aerr := p.checkDealUuidUnique(deal); aerr != nil {
		return aerr
	}

	// get current sealing pipeline status
	status, err := sealingpipeline.GetStatus(p.ctx, p.fullnodeApi, p.sps)
	if err != nil {
		return &acceptError{
			error:         fmt.Errorf("failed to fetch sealing pipleine status: %w", err),
			reason:        "server error: get sealing status",
			isSevereError: true,
		}
	}

	// run custom decision logic
	params := types.DealParams{
		DealUUID:           deal.DealUuid,
		ClientDealProposal: deal.ClientDealProposal,
		DealDataRoot:       deal.DealDataRoot,
		Transfer:           deal.Transfer,
	}

	accept, reason, err := p.df(p.ctx, types.DealFilterParams{
		DealParams:           &params,
		SealingPipelineState: status})

	if err != nil {
		return &acceptError{
			error:         fmt.Errorf("failed to invoke deal filter: %w", err),
			reason:        "server error: deal filter error",
			isSevereError: true,
		}
	}

	if !accept {
		return &acceptError{
			error:         fmt.Errorf("deal filter rejected deal: %s", reason),
			reason:        reason,
			isSevereError: false,
		}
	}

	cleanup := func() {
		collat, pub, errf := p.fundManager.UntagFunds(p.ctx, deal.DealUuid)
		if errf != nil && !xerrors.Is(errf, db.ErrNotFound) {
			p.dealLogger.LogError(deal.DealUuid, "failed to untag funds during deal cleanup", errf)
		} else if errf == nil {
			p.dealLogger.Infow(deal.DealUuid, "untagged funds for deal cleanup", "untagged publish", pub, "untagged collateral", collat,
				"err", errf)
		}

		errs := p.storageManager.Untag(p.ctx, deal.DealUuid)
		if errs != nil && !xerrors.Is(errs, db.ErrNotFound) {
			p.dealLogger.LogError(deal.DealUuid, "failed to untag storage during deal cleanup", errs)
		} else if errs == nil {
			p.dealLogger.Infow(deal.DealUuid, "untagged storage for deal cleanup", deal.Transfer.Size)
		}

		if deal.InboundFilePath != "" {
			_ = os.Remove(deal.InboundFilePath)
		}
	}

	// tag the funds required for escrow and sending the publish deal message
	// so that they are not used for other deals
	trsp, err := p.fundManager.TagFunds(p.ctx, deal.DealUuid, deal.ClientDealProposal.Proposal)
	if err != nil {
		cleanup()

		err = fmt.Errorf("failed to tag funds for deal: %w", err)
		aerr := &acceptError{
			error:         err,
			reason:        "server error: tag funds",
			isSevereError: true,
		}
		if xerrors.Is(err, fundmanager.ErrInsufficientFunds) {
			aerr.reason = "server error: provider has insufficient funds to accept deal"
			aerr.isSevereError = false
		}
		return aerr
	}
	p.logFunds(deal.DealUuid, trsp)

	// tag the storage required for the deal in the staging area
	err = p.storageManager.Tag(p.ctx, deal.DealUuid, deal.Transfer.Size)
	if err != nil {
		cleanup()

		err = fmt.Errorf("failed to tag storage for deal: %w", err)
		aerr := &acceptError{
			error:         err,
			reason:        "server error: tag storage",
			isSevereError: true,
		}
		if xerrors.Is(err, storagemanager.ErrNoSpaceLeft) {
			aerr.reason = "server error: provider has no space left for storage deals"
			aerr.isSevereError = false
		}
		return aerr
	}

	// create a file in the staging area to which we will download the deal data
	downloadFilePath, err := p.storageManager.DownloadFilePath(deal.DealUuid)
	if err != nil {
		cleanup()

		return &acceptError{
			error:         fmt.Errorf("failed to create download staging file for deal: %w", err),
			reason:        "server error: creating download staging file",
			isSevereError: true,
		}
	}
	deal.InboundFilePath = downloadFilePath
	p.dealLogger.Infow(deal.DealUuid, "created deal download staging file", "path", deal.InboundFilePath)

	// write deal state to the database
	deal.CreatedAt = time.Now()
	deal.Checkpoint = dealcheckpoints.Accepted
	deal.CheckpointAt = time.Now()
	err = p.dealsDB.Insert(p.ctx, deal)
	if err != nil {
		cleanup()

		return &acceptError{
			error:         fmt.Errorf("failed to insert deal in db: %w", err),
			reason:        "server error: save to db",
			isSevereError: true,
		}
	}

	p.dealLogger.Infow(deal.DealUuid, "inserted deal into deals DB")

	return nil
}

// processOfflineDealProposal just saves the deal to the database.
// Execution resumes when processImportOfflineDealData is called.
func (p *Provider) processOfflineDealProposal(ds *smtypes.ProviderDealState) *acceptError {
	// Check that the deal proposal is unique
	if aerr := p.checkDealPropUnique(ds); aerr != nil {
		return aerr
	}

	// Check that the deal uuid is unique
	if aerr := p.checkDealUuidUnique(ds); aerr != nil {
		return aerr
	}

	// Save deal to DB
	ds.CreatedAt = time.Now()
	ds.Checkpoint = dealcheckpoints.Accepted
	ds.CheckpointAt = time.Now()
	if err := p.dealsDB.Insert(p.ctx, ds); err != nil {
		return &acceptError{
			error:         fmt.Errorf("failed to insert deal in db: %w", err),
			reason:        "server error: save to db",
			isSevereError: true,
		}
	}

	// Set up pubsub for deal updates
	dh := p.mkAndInsertDealHandler(ds.DealUuid)
	pub, err := dh.bus.Emitter(&types.ProviderDealState{}, eventbus.Stateful)
	if err != nil {
		err = fmt.Errorf("failed to create event emitter: %w", err)
		p.failDeal(pub, ds, err)
		p.cleanupDealLogged(ds)
		return &acceptError{
			error:         err,
			reason:        "server error: setup pubsub",
			isSevereError: true,
		}
	}

	// publish "new deal" event
	p.fireEventDealNew(ds)
	// publish an event with the current state of the deal
	p.fireEventDealUpdate(pub, ds)

	return nil
}

func (p *Provider) processImportOfflineDealData(deal *types.ProviderDealState) *acceptError {
	cleanup := func() {
		collat, pub, errf := p.fundManager.UntagFunds(p.ctx, deal.DealUuid)
		if errf != nil && !xerrors.Is(errf, db.ErrNotFound) {
			p.dealLogger.LogError(deal.DealUuid, "failed to untag funds during deal cleanup", errf)
		} else if errf == nil {
			p.dealLogger.Infow(deal.DealUuid, "untagged funds for deal cleanup", "untagged publish", pub, "untagged collateral", collat)
		}
	}

	// tag the funds required for escrow and sending the publish deal message
	// so that they are not used for other deals
	trsp, err := p.fundManager.TagFunds(p.ctx, deal.DealUuid, deal.ClientDealProposal.Proposal)
	if err != nil {
		cleanup()

		err = fmt.Errorf("failed to tag funds for deal: %w", err)
		aerr := &acceptError{
			error:         err,
			reason:        "server error: tag funds",
			isSevereError: true,
		}
		if xerrors.Is(err, fundmanager.ErrInsufficientFunds) {
			aerr.reason = "server error: provider has insufficient funds to accept deal"
			aerr.isSevereError = false
		}
		return aerr
	}
	p.logFunds(deal.DealUuid, trsp)
	return nil
}

func (p *Provider) checkDealPropUnique(deal *smtypes.ProviderDealState) *acceptError {
	signedPropCid, err := deal.SignedProposalCid()
	if err != nil {
		return &acceptError{
			error:         fmt.Errorf("getting signed deal proposal cid: %w", err),
			reason:        "server error: signed proposal cid",
			isSevereError: true,
		}
	}

	dl, err := p.dealsDB.BySignedProposalCID(p.ctx, signedPropCid)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			// If there was no deal in the DB with this signed proposal cid,
			// then it's unique
			return nil
		}
		return &acceptError{
			error:         fmt.Errorf("looking up deal by signed deal proposal cid: %w", err),
			reason:        "server error: lookup by proposal cid",
			isSevereError: true,
		}
	}

	// The database lookup did not return a "not found" error, meaning we found
	// a deal with a matching deal proposal cid. Therefore the deal proposal
	// is not unique.
	err = fmt.Errorf("deal proposal is identical to deal %s (proposed at %s)", dl.DealUuid, dl.CreatedAt)
	return &acceptError{
		error:         err,
		reason:        err.Error(),
		isSevereError: false,
	}
}

func (p *Provider) checkDealUuidUnique(deal *smtypes.ProviderDealState) *acceptError {
	dl, err := p.dealsDB.ByID(p.ctx, deal.DealUuid)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			// If there was no deal in the DB with this uuid, then it's unique
			return nil
		}
		return &acceptError{
			error:         fmt.Errorf("looking up deal by uuid: %w", err),
			reason:        "server error: unique check: lookup by deal uuid",
			isSevereError: true,
		}
	}

	// The database lookup did not return a "not found" error, meaning we found
	// a deal with a matching deal uuid. Therefore the deal proposal is not unique.
	err = fmt.Errorf("deal has the same uuid as deal %s (proposed at %s)", dl.DealUuid, dl.CreatedAt)
	return &acceptError{
		error:         err,
		reason:        err.Error(),
		isSevereError: false,
	}
}

// The provider loop effectively implements a lock over resources used by
// the provider, like funds and storage space, so that only one deal at a
// time can change the value of these resources.
func (p *Provider) loop() {
	defer func() {
		p.wg.Done()
		log.Info("provider event loop complete")
	}()

	for {
		select {
		// Process a request to
		// - accept a deal proposal and execute it immediately
		// - accept an offline deal proposal and save it for execution later
		//   when the data is imported
		// - accept a request to import data for an offline deal
		case dealReq := <-p.acceptDealChan:
			deal := dealReq.deal
			p.dealLogger.Infow(deal.DealUuid, "processing deal acceptance request")

			var aerr *acceptError
			if deal.IsOffline {
				// It's an offline deal
				if dealReq.isImport {
					// The Storage Provider is importing the deal data, so tag
					// funds for the deal and execute it
					aerr = p.processImportOfflineDealData(dealReq.deal)
				} else {
					// When the client proposes an offline deal, save the deal
					// to the database but don't execute the deal. The deal
					// will be executed when the Storage Provider imports the
					// deal data.
					aerr = p.processOfflineDealProposal(dealReq.deal)
					if aerr == nil {
						// The deal proposal was successful. Send an Accept response to the client.
						dealReq.rsp <- acceptDealResp{ri: &api.ProviderDealRejectionInfo{Accepted: true}}
						// Don't execute the deal now, wait for data import.
						continue
					}
				}
			} else {
				// Process a regular deal proposal
				aerr = p.processDealProposal(dealReq.deal)
			}
			if aerr != nil {
				// If the error is a severe error (eg can't connect to database)
				if aerr.isSevereError {
					// Send a rejection message to the client with a reason for rejection
					resp := acceptDealResp{ri: &api.ProviderDealRejectionInfo{Accepted: false, Reason: aerr.reason}}
					// Log an error with more details for the provider
					p.dealLogger.LogError(deal.DealUuid, "error while processing deal acceptance request", aerr)
					dealReq.rsp <- resp
					continue
				}

				// The error is not a severe error, so don't log an error, just
				// send a message to the client with a rejection reason
				p.dealLogger.Infow(deal.DealUuid, "deal acceptance request rejected", "reason", aerr.reason)
				dealReq.rsp <- acceptDealResp{ri: &api.ProviderDealRejectionInfo{Accepted: false, Reason: aerr.reason}, err: nil}
				continue
			}

			// start executing the deal
			p.wg.Add(1)
			go func() {
				defer p.wg.Done()
				p.doDeal(deal, dealReq.dh)
				p.dealLogger.Infow(deal.DealUuid, "deal go-routine finished execution")
			}()

			dealReq.rsp <- acceptDealResp{&api.ProviderDealRejectionInfo{Accepted: true}, nil}

		case storageSpaceDealReq := <-p.storageSpaceChan:
			deal := storageSpaceDealReq.deal
			if err := p.storageManager.Untag(p.ctx, deal.DealUuid); err != nil && !xerrors.Is(err, db.ErrNotFound) {
				p.dealLogger.LogError(deal.DealUuid, "failed to untag storage space", err)
			} else {
				p.dealLogger.Infow(deal.DealUuid, "untagged storage space")
			}
			close(storageSpaceDealReq.done)

		case publishedDeal := <-p.publishedDealChan:
			deal := publishedDeal.deal
			collat, pub, errf := p.fundManager.UntagFunds(p.ctx, deal.DealUuid)
			if errf != nil {
				p.dealLogger.LogError(deal.DealUuid, "failed to untag funds", errf)
			} else {
				p.dealLogger.Infow(deal.DealUuid, "untagged funds for deal after publish", "untagged publish", pub, "untagged collateral", collat)
			}
			publishedDeal.done <- struct{}{}

		case finishedDeal := <-p.finishedDealChan:
			deal := finishedDeal.deal
			p.dealLogger.Infow(deal.DealUuid, "deal finished")
			collat, pub, errf := p.fundManager.UntagFunds(p.ctx, deal.DealUuid)
			if errf != nil && !xerrors.Is(errf, db.ErrNotFound) {
				p.dealLogger.LogError(deal.DealUuid, "failed to untag funds", errf)
			} else if errf == nil {
				p.dealLogger.Infow(deal.DealUuid, "untagged funds for deal as deal finished", "untagged publish", pub, "untagged collateral", collat,
					"err", errf)
			}

			errs := p.storageManager.Untag(p.ctx, deal.DealUuid)
			if errs != nil && !xerrors.Is(errs, db.ErrNotFound) {
				p.dealLogger.LogError(deal.DealUuid, "failed to untag storage", errs)
			} else if errs == nil {
				p.dealLogger.Infow(deal.DealUuid, "untagged storage space for deal")
			}
			finishedDeal.done <- struct{}{}

		case <-p.ctx.Done():
			return
		}
	}
}
