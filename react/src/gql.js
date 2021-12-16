import gql from "graphql-tag";
import { ApolloClient, ApolloLink, HttpLink, InMemoryCache, split, from } from "@apollo/client";
import { getMainDefinition } from '@apollo/client/utilities';
import { WebSocketLink } from "@apollo/client/link/ws";
import Observable from 'zen-observable';
import { transformResponse } from "./transform";

const graphqlEndpoint = "localhost:8080"

// Transform response data (eg convert date string to Date object)
const transformResponseLink = new ApolloLink((operation, forward) => {
    const res = forward(operation)
    return Observable.from(res).map(data => {
        transformResponse(data)
        return data
    });
});

// HTTP Link
const httpLink = new HttpLink({
    uri: `http://${graphqlEndpoint}/graphql/query`,
});

// WebSocket Link
const wsLink = new WebSocketLink({
    uri: `ws://${graphqlEndpoint}/graphql/subscription`,
    options: {
        reconnect: true,
        lazy: true,
    },
});

// Send query request based on the type definition
const link = from([
    transformResponseLink,
    split(
    ({ query }) => {
            const definition = getMainDefinition(query);
            return (
                definition.kind === 'OperationDefinition' &&
                definition.operation === 'subscription'
            );
        },
        wsLink,
        httpLink
    )
]);

const cache = new InMemoryCache();

const gqlClient = new ApolloClient({
    link,
    cache
});

const DealsListQuery = gql`
    query AppDealsListQuery($first: ID, $limit: Int) {
        deals(first: $first, limit: $limit) {
            deals {
                ID
                CreatedAt
                PieceCid
                PieceSize
                ClientAddress
                StartEpoch
                EndEpoch
                ProviderCollateral
                ClientPeerID
                DealDataRoot
                PublishCid
                Stage
                Message
                Transfer {
                    Type
                    Size
                    Params
                }
                Sector {
                    ID
                    Offset
                    Length
                }
                Logs {
                    CreatedAt
                    Text
                }
            }
            totalCount
            next
        }
    }
`;

const DealSubscription = gql`
    subscription AppDealSubscription($id: ID!) {
        dealUpdate(id: $id) {
            ID
            CreatedAt
            PieceCid
            PieceSize
            ClientAddress
            StartEpoch
            EndEpoch
            ProviderCollateral
            ClientPeerID
            DealDataRoot
            PublishCid
            Stage
            Message
            Transfer {
                Type
                Size
                Params
            }
            Sector {
                ID
                Offset
                Length
            }
            Logs {
                CreatedAt
                Text
            }
        }
    }
`;

const DealCancelMutation = gql`
    mutation AppDealCancelMutation($id: ID!) {
        dealCancel(id: $id)
    }
`;

const NewDealsSubscription = gql`
    subscription AppNewDealsSubscription {
        dealNew {
            ID
            CreatedAt
            PieceCid
            PieceSize
            ClientAddress
            StartEpoch
            EndEpoch
            ProviderCollateral
            ClientPeerID
            DealDataRoot
            PublishCid
            Stage
            Message
            Transfer {
                Type
                Size
                Params
            }
            Sector {
                ID
                Offset
                Length
            }
            Logs {
                CreatedAt
                Text
            }
        }
    }
`;

const StorageQuery = gql`
    query AppStorageQuery {
        storage {
            Staged
            Transferred
            Pending
            Free
            MountPoint
        }
    }
`;

const SealingPipelineQuery = gql`
    query AppSealingPipelineQuery {
        sealingpipeline {
            Committing
            PreCommitting
            WaitSeed
        }
    }
`;

const FundsQuery = gql`
    query AppFundsQuery {
        funds {
            Escrow {
                Tagged
                Available
                Locked
            }
            Collateral {
                Address
                Balance
            }
            PubMsg {
                Address
                Balance
                Tagged
            }
        }
    }
`;

const TransfersQuery = gql`
    query AppTransfersQuery {
        transfers {
            At
            Bytes
        }
    }
`;

const FundsLogsQuery = gql`
    query AppFundsLogsQuery {
        fundsLogs {
            totalCount
            next
            logs {
                CreatedAt
                DealUUID
                Amount
                Text
            }
        }
    }
`;

const DealPublishQuery = gql`
    query AppDealPublishQuery {
        dealPublish {
            Start
            Period
            MaxDealsPerMsg
            Deals {
                ID
                CreatedAt
            }
        }
    }
`;

const DealPublishNowMutation = gql`
    mutation AppDealPublishNowMutation {
        dealPublishNow
    }
`;

const FundsMoveToEscrow = gql`
    mutation AppDealPublishNowMutation($amount: BigInt!) {
        fundsMoveToEscrow(amount: $amount)
    }
`;

const MpoolQuery = gql`
    query AppMpoolQuery($local: Boolean!) {
        mpool(local: $local) {
            From
            To
            Nonce
            Value
            GasFeeCap
            GasLimit
            GasPremium
            Method
            Params
            BaseFee
        }
    }
`;

export {
    gqlClient,
    DealsListQuery,
    DealSubscription,
    DealCancelMutation,
    NewDealsSubscription,
    StorageQuery,
    FundsQuery,
    FundsLogsQuery,
    DealPublishQuery,
    DealPublishNowMutation,
    FundsMoveToEscrow,
    TransfersQuery,
    MpoolQuery,
    SealingPipelineQuery,
}
