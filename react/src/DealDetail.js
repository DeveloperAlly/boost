/* global BigInt */

import React, {useEffect, useState} from "react";
import {useMutation, useQuery, useSubscription} from "@apollo/react-hooks";
import {DealCancelMutation, DealSubscription, EpochQuery} from "./gql";
import {useNavigate} from "react-router-dom";
import {dateFormat} from "./util-date";
import moment from "moment";
import {humanFIL, addCommas, humanFileSize} from "./util";
import {useParams} from "react-router-dom";
import './DealDetail.css'
import closeImg from './bootstrap-icons/icons/x-circle.svg'
import {Info} from "./Info";

export function DealDetail(props) {
    const params = useParams()
    const navigate = useNavigate()

    // Add a class to the document body when showing the deal detail page
    useEffect(() => {
        document.body.classList.add('modal-open')

        return function () {
            document.body.classList.remove('modal-open')
        }
    })

    var popupTimeout
    function showPopup(msg) {
        clearTimeout(popupTimeout)
        const el = document.body.querySelector('.content .popup')
        popupTimeout = addClassFor(el, 'showing', 2000)
        const msgEl = document.body.querySelector('.content .popup .message')
        msgEl.textContent = msg
    }

    function dealIDToClipboard() {
        navigator.clipboard.writeText(deal.ID)
        const el = document.body.querySelector('.content .title .copy')
        addClassFor(el, 'copied', 500)
        showPopup("Copied " + deal.ID + " to clipboard")
    }

    function allToClipboard() {
        const detailTableEl = document.body.querySelector('.deal-detail .deal-fields')
        const allDataAsText = getAllDataAsText(detailTableEl, deal.ID, logs)
        navigator.clipboard.writeText(allDataAsText)
        const el = document.body.querySelector('.content .title .copy-all')
        addClassFor(el, 'copied', 500)
        showPopup("Copied all data to clipboard")
    }

    const currentEpochData = useQuery(EpochQuery)

    const [cancelDeal] = useMutation(DealCancelMutation, {
        variables: {id: params.dealID}
    })

    const {loading, error, data} = useSubscription(DealSubscription, {
        variables: {id: params.dealID},
    })

    if (error) {
        return <div>Error: {error.message}</div>
    }

    if (loading) {
        return <div>Loading ...</div>
    }

    var deal = data.dealUpdate

    const currentEpoch = (((currentEpochData || {}).data || {}).epoch || {}).Epoch
    var startEpochTime, endEpochTime
    if (currentEpoch) {
        const secondsPerEpoch = currentEpochData.data.epoch.SecondsPerEpoch
        const startEpochDelta = Number(deal.StartEpoch - currentEpoch)
        startEpochTime = new Date(new Date().getTime() + startEpochDelta*secondsPerEpoch*1000)
        const endEpochDelta = Number(deal.EndEpoch - currentEpoch)
        endEpochTime = new Date(new Date().getTime() + endEpochDelta*secondsPerEpoch*1000)
    }

    var transferParams = deal.Transfer.Params
    try {
        const fields = JSON.parse(transferParams)
        transferParams = Object.keys(fields).map(k => (
            <div className="param">{k}: {fields[k]}</div>
        ))
    } catch (e) {
        console.error("parsing transfer params: "+e.message)
    }

    var logRowData = []
    var logs = (deal.Logs || []).sort((a, b) => a.CreatedAt.getTime() - b.CreatedAt.getTime())
    for (var i = 0; i < logs.length; i++) {
        var log = deal.Logs[i]
        var prev = i === 0 ? null : deal.Logs[i - 1]
        logRowData.push({log: log, prev: prev})
    }

    return <div className="deal-detail modal" id={deal.ID}>
        <div className="content">
            <div className="close" onClick={() => navigate(-1)}>
                <img className="icon" alt="" src={closeImg} />
            </div>
            <div className="popup">
                <div className="message"></div>
            </div>
            <div className="title">
                <span>Deal {deal.ID}</span>
                <span className="copy" onClick={dealIDToClipboard} title="Copy deal uuid to clipboard"></span>
                <span className="copy-all" onClick={allToClipboard} title="Copy all deal info to clipboard"></span>
            </div>
            <table className="deal-fields">
                <tbody>
                <tr>
                    <th>CreatedAt</th>
                    <td>
                        {moment(deal.CreatedAt).format(dateFormat)}
                        &nbsp;
                        <span className="aux">({moment(deal.CreatedAt).fromNow()} ago)</span>
                    </td>
                </tr>
                <tr>
                    <th>Client Address</th>
                    <td>
                        <a href={"https://filfox.info/en/address/"+deal.ClientAddress} target="_blank" rel="noreferrer">
                            {deal.ClientAddress}
                        </a>
                    </td>
                </tr>
                <tr>
                    <th>Client Peer ID</th>
                    <td>{deal.ClientPeerID}</td>
                </tr>
                <tr>
                    <th>Deal Data Root CID</th>
                    <td>{deal.DealDataRoot}</td>
                </tr>
                <tr>
                    <th>Verified</th>
                    <td>{deal.IsVerified ? 'Yes' : 'No'}</td>
                </tr>
                <tr>
                    <th>Piece CID</th>
                    <td>{deal.PieceCid}</td>
                </tr>
                <tr>
                    <th>Piece Size</th>
                    <td>
                        {humanFileSize(deal.PieceSize)}
                        &nbsp;
                        <span className="aux">({addCommas(deal.PieceSize)} bytes)</span>
                    </td>
                </tr>
                <tr>
                    <th>Client Collateral</th>
                    <td>{humanFIL(deal.ClientCollateral)}</td>
                </tr>
                <tr>
                    <th>Provider Collateral</th>
                    <td>{humanFIL(deal.ProviderCollateral)}</td>
                </tr>
                <tr>
                    <th>Storage Price Per Epoch</th>
                    <td>{humanFIL(deal.StoragePricePerEpoch)}</td>
                </tr>
                <tr>
                    <th>Current Epoch</th>
                    <td>{currentEpoch ? addCommas(currentEpoch) : null}</td>
                </tr>
                <tr>
                    <th>Start Epoch</th>
                    <td>
                        {addCommas(deal.StartEpoch)}
                        <span className="aux">
                            {startEpochTime ? ' (' + moment(startEpochTime).fromNow() + ')' : null}
                        </span>
                    </td>
                </tr>
                <tr>
                    <th>End Epoch</th>
                    <td>
                        {addCommas(deal.EndEpoch)}
                        <span className="aux">
                            {endEpochTime ? ' (' + moment(endEpochTime).fromNow() + ')' : null}
                        </span>
                    </td>
                </tr>
                <tr>
                    <th>Duration</th>
                    <td>
                        {addCommas(deal.EndEpoch-deal.StartEpoch)}
                        <span className="aux">
                            {startEpochTime && endEpochTime ? ' (' + moment(endEpochTime).diff(startEpochTime, 'days') + ' days)' : null}
                        </span>
                    </td>
                </tr>
                <tr>
                    <th>Storage Fee</th>
                    <td>
                        {humanFIL(deal.StoragePricePerEpoch * BigInt(deal.EndEpoch-deal.StartEpoch))}
                        &nbsp;
                        <span className="aux">
                            (Price per epoch x Duration)
                        </span>
                    </td>
                </tr>
                <tr>
                    <th>Transfer Mode</th>
                    <td>{deal.IsOffline ? 'Offline' : 'Online'}</td>
                </tr>
                {deal.IsOffline ? null : (
                    <tr>
                        <th>Transfer Type</th>
                        <td>{deal.Transfer.Type}</td>
                    </tr>
                )}
                <tr>
                    <th>Transfer Size</th>
                    <td>
                        {humanFileSize(deal.Transfer.Size)}
                        &nbsp;
                        <span className="aux">({addCommas(deal.Transfer.Size)} bytes)</span>
                    </td>
                </tr>
                {deal.IsOffline ? null : (
                    <tr>
                        <th>Transfer Params</th>
                        <td>{transferParams}</td>
                    </tr>
                )}
                {deal.Transfer.ClientID ? (
                    <tr>
                        <th>Transfer Client ID</th>
                        <td>{deal.Transfer.ClientID}</td>
                    </tr>
                ) : null}
                <tr>
                    <th>Transferred</th>
                    <td>
                        {humanFileSize(deal.Transferred)}
                        &nbsp;
                        <span className="aux">({addCommas(deal.Transferred)} bytes)</span>
                    </td>
                </tr>
                <tr>
                    <th>Inbound File Path</th>
                    <td>{deal.InboundFilePath}</td>
                </tr>
                {deal.Sector.ID > 0 ? (
                    <>
                    <tr>
                        <th>Sector ID</th>
                        <td>{deal.Sector.ID + ''}</td>
                    </tr>
                    <tr>
                        <th>Sector Data Offset</th>
                        <td>{addCommas(deal.Sector.Offset)}</td>
                    </tr>
                    <tr>
                        <th>Sector Data Length</th>
                        <td>{addCommas(deal.Sector.Length)}</td>
                    </tr>
                    </>
                ) : null}
                <tr>
                    <th>Publish Message CID</th>
                    <td>
                        <a href={"https://filfox.info/en/message/"+deal.PublishCid} target="_blank" rel="noreferrer">
                        {deal.PublishCid}
                        </a>
                    </td>
                </tr>
                <tr>
                    <th>Chain Deal ID</th>
                    <td>{deal.ChainDealID ? addCommas(deal.ChainDealID) : null}</td>
                </tr>
                <tr>
                    <th>Status</th>
                    <td>
                        {deal.Message}
                        <DealStatusInfo />
                    </td>
                </tr>
                <tr>
                    <th>Checkpoint</th>
                    <td>
                        {deal.Checkpoint}
                        {deal.CheckpointAt+'' !== (new Date(0))+'' ? (
                          <span>
                            &nbsp;
                            <span className="aux">({moment(deal.CheckpointAt).fromNow()} ago)</span>
                          </span>
                        ) : null}
                    </td>
                </tr>

                </tbody>
            </table>

            {deal.Checkpoint === 'Accepted' && !deal.IsOffline ? (
                <div className="buttons">
                    <div className="button cancel" onClick={cancelDeal}>Cancel Transfer</div>
                </div>
            ) : null}

            <h3>Deal Logs</h3>

            <table className="deal-logs">
                <tbody>
                {logRowData.map((l, i) => <DealLog key={i} log={l.log} prev={l.prev}/>)}
                </tbody>
            </table>
        </div>
    </div>
}

function DealLog(props) {
    var prev = props.prev
    var log = props.log
    var sinceLast = ''
    var sinceScale = ''
    if (prev != null) {
        var logMs = log.CreatedAt.getTime()
        var prevMs = prev.CreatedAt.getTime()
        var deltaMillis = logMs - prevMs
        if (deltaMillis < 1000) {
            sinceScale = 'since-ms'
            sinceLast = (logMs - prevMs) + 'ms'
        } else {
            sinceLast = moment(prev.CreatedAt).from(log.CreatedAt)
            if (deltaMillis < 10000) {
                sinceScale = 'since-s'
            } else {
                sinceScale = 'since-multi-s'
            }
        }
    }

    var logParams = {}
    if (log.LogParams && typeof log.LogParams === 'string') {
        try {
            const params = JSON.parse(log.LogParams)
            for (let i = 0; i < params.length; i+=2) {
                var k = params[i]
                var v = params[i+1]
                if (typeof k !== "string") {
                    k = JSON.stringify(k)
                }
                logParams[k] = v
            }
            delete logParams.id
        } catch(_) {
        }
    }

    return <tr className={'deal-log ' + sinceScale}>
        <td className="at">{moment(log.CreatedAt).format(dateFormat)}</td>
        <td className="since-last">{sinceLast}</td>
        <td className="log-line">
            <div className="message">
                <span className="subsystem">{log.Subsystem}{log.Subsystem ? ': ' : ''}</span>
                {log.LogMsg}
            </div>
            {Object.keys(logParams).sort().map(k => <LogParam k={k} v={logParams[k]} topLevel={true} key={k} />)}
        </td>
    </tr>
}

function LogParam(props) {
    const [expanded, setExpanded] = useState(false)

    var val = props.v
    const isObject = (val && typeof val === 'object')
    if (isObject) {
        val = Object.keys(val).sort().map(ck => <LogParam k={ck} v={val[ck]} key={ck} />)
    } else if ((typeof val === 'string' || typeof val === 'number') && (''+val).match(/^[0-9]+$/)) {
        val = addCommas(BigInt(val))
    }

    function toggleExpandState() {
        setExpanded(!expanded)
    }

    const expandable = isObject && props.topLevel
    return (
        <div className={"param" + (expandable ? ' expandable' : '') + (expanded ? ' expanded' : '')}>
            <span className="param-name" onClick={toggleExpandState}>
                {props.k}:
                {expandable ? (
                    <div className="expand-collapse"></div>
                ) : null}
            </span>
            &nbsp;
            {val}
        </div>
    )
}

function getAllDataAsText(detailTableEl, dealID, logs) {
    var lines = []
    lines.push('=== Deal ' + dealID + ' ===')
    lines.push('')
    for (var row of detailTableEl.querySelectorAll('tr')) {
        var fieldName = row.querySelector('th').textContent
        var fieldValue = row.querySelector('td').textContent
        lines.push(fieldName + ': ' + fieldValue)
    }

    lines.push('')
    lines.push('=== Logs ===')
    for (var log of logs) {
        var line = moment(log.CreatedAt).format(dateFormat)
        if (log.Subsystem) {
            line += ' [' + log.Subsystem + ']'
        }
        line += ': ' + log.LogMsg
        lines.push(line)
        if (log.LogParams) {
            try {
                var logParams = JSON.parse(log.LogParams)
                var obj = {}
                for (var i = 0; i < logParams.length; i+=2) {
                    obj[logParams[i]] = logParams[i+1]
                }
                delete obj.id
                var keys = Object.keys(obj)
                if (keys.length) {
                    lines.push(JSON.stringify(obj, null, "  "))
                }
            } catch (e) {}
        }
    }

    return lines.join('\n')+'\n'
}

function addClassFor(el, className, duration) {
    el.classList.add(className)
    return setTimeout(function() {
        el.classList.remove(className)
    }, duration)
}

function DealStatusInfo(props) {
    return <span className="deal-status-info">
        <Info>
            The deal can be in one of the following states:
            <p>
                <i>Transfer Queued</i><br/>
                <p>
                    The storage deal proposal has been accepted, and Boost is
                    about to start the data transfer.
                </p>
            </p>
            <p>
                <i>Awaiting Offline Data Import</i><br/>
                <p>
                    The client has made an offline deal proposal, and Boost is
                    waiting for the Storage Provider operator to import the deal
                    data.
                </p>
            </p>
            <p>
                <i>Transferring</i>
                <p>
                    The data for the deal is transferring.
                </p>
            </p>
            <p>
                <i>Transfer Complete</i>
                <p>
                    The data transfer is complete and Boost is verifying the data matches commp.
                </p>
            </p>
            <p>
                <i>Ready to Publish</i>
                <p>
                    The deal is in the batch publish queue, ready to be published.
                </p>
            </p>
            <p>
                <i>Awaiting Publish Confirmation</i>
                <p>
                    Boost sent a publish deal message for the deal and is waiting for on-chain confirmation.
                </p>
            </p>
            <p>
                <i>Adding to Sector</i>
                <p>
                    Boost is handing the deal off to the lotus-miner sealing subsystem
                    to be added to a sector.
                </p>
            </p>
            <p>
                <i>Announcing</i>
                <p>
                    Boost is announcing the deal to the network so that clients know where to retrieve it.
                </p>
            </p>
            <p>
                <i>Sealing</i>
                <p>
                    The deal has been added to a sector and is now sealing.
                </p>
            </p>
            <p>
                <i>Complete</i>
                <p>
                    The deal has reached its duration and expired.
                </p>
            </p>
            <p>
                <i>Cancelled</i>
                <p>
                    The deal was cancelled.
                </p>
            </p>
        </Info>
    </span>
}
