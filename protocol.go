package main

import (
	"encoding/hex"
	"fmt"
	"reflect"

	"github.com/alexdcox/dashutil/base58"
	"github.com/pkg/errors"
)

type Protocol uint16
type Subprotocol uint16

const (
	ProtocolHandshake Protocol = iota
	ProtocolDeltaQueue
	ProtocolChainSync
	ProtocolBlockFetch
	ProtocolTxSubmission
	ProtocolLocalChainSync
	ProtocolLocalTx
	ProtocolLocalState
	ProtocolKeepAlive
)
const (
	SubprotocolHandshakeProposedVersion Subprotocol = iota
	SubprotocolHandshakeAcceptVersion
	SubprotocolHandshakeRefuse
	SubprotocolHandshakeQueryReply
)
const (
	SubprotocolChainSyncRequestNext Subprotocol = iota
	SubprotocolChainSyncAwaitReply
	SubprotocolChainSyncRollForward
	SubprotocolChainSyncRollBackward
	SubprotocolChainSyncFindIntersect
	SubprotocolChainSyncIntersectFound
	SubprotocolChainSyncIntersectNotFound
	SubprotocolChainSyncDone
)

const (
	SubprotocolBlockFetchRequestRange Subprotocol = iota
	SubprotocolBlockFetchClientDone
	SubprotocolBlockFetchStartBatch
	SubprotocolBlockFetchNoBlocks
	SubprotocolBlockFetchBlock
	SubprotocolBlockFetchBatchDone
)
const (
	SubprotocolKeepAliveEcho Subprotocol = iota
	SubprotocolKeepAlivePing
)

var ProtocolStringMap = map[Protocol]string{
	ProtocolHandshake:      "handshake",
	ProtocolChainSync:      "chain sync",
	ProtocolBlockFetch:     "block fetch",
	ProtocolTxSubmission:   "tx submission",
	ProtocolLocalChainSync: "local chain sync",
	ProtocolLocalTx:        "local tx",
	ProtocolLocalState:     "local state",
	ProtocolKeepAlive:      "keep alive",
	// ProtocolLocalTxMonitor: "local tx monitor",
}

var ProtocolMessageMap = map[Protocol]map[Subprotocol]any{
	ProtocolHandshake: {
		SubprotocolHandshakeProposedVersion: &MessageProposeVersions{},
		SubprotocolHandshakeAcceptVersion:   &MessageAcceptVersion{},
		SubprotocolHandshakeRefuse:          &MessageRefuse{},
		SubprotocolHandshakeQueryReply:      &MessageQueryReply{},
	},
	ProtocolChainSync: {
		SubprotocolChainSyncRequestNext:       &MessageRequestNext{},
		SubprotocolChainSyncAwaitReply:        &MessageAwaitReply{},
		SubprotocolChainSyncRollForward:       &MessageRollForward{},
		SubprotocolChainSyncRollBackward:      &MessageRollBackward{},
		SubprotocolChainSyncFindIntersect:     &MessageFindIntersect{},
		SubprotocolChainSyncIntersectFound:    &MessageIntersectFound{},
		SubprotocolChainSyncIntersectNotFound: &MessageIntersectNotFound{},
		SubprotocolChainSyncDone:              &MessageChainSyncDone{},
	},
	ProtocolBlockFetch: {
		SubprotocolBlockFetchRequestRange: &MessageRequestRange{},
		SubprotocolBlockFetchClientDone:   &MessageClientDone{},
		SubprotocolBlockFetchStartBatch:   &MessageStartBatch{},
		SubprotocolBlockFetchNoBlocks:     &MessageNoBlocks{},
		SubprotocolBlockFetchBlock:        &MessageBlock{},
		SubprotocolBlockFetchBatchDone:    &MessageBatchDone{},
	},
	ProtocolKeepAlive: {
		SubprotocolKeepAliveEcho: &MessageKeepAliveResponse{},
		SubprotocolKeepAlivePing: &MessageKeepAlive{},
	},
	ProtocolLocalTx: {},
}

func ProtocolToMessage(protocol Protocol, subprotocol Subprotocol) (message any, err error) {
	subprotocolMap, protocolOk := ProtocolMessageMap[protocol]
	if !protocolOk {
		err = errors.Errorf("unknown protocol %d", protocol)
		return
	}

	message, subprotocolOk := subprotocolMap[subprotocol]
	if !subprotocolOk {
		err = errors.Errorf("unknown subprotocol %d", protocol)
		return
	}

	return
}

var MessageProtocolMap = map[reflect.Type]Protocol{
	reflect.TypeOf(&MessageProposeVersions{}):   ProtocolHandshake,
	reflect.TypeOf(&MessageRefuse{}):            ProtocolHandshake,
	reflect.TypeOf(&MessageQueryReply{}):        ProtocolHandshake,
	reflect.TypeOf(&MessageAcceptVersion{}):     ProtocolHandshake,
	reflect.TypeOf(&MessageRequestNext{}):       ProtocolChainSync,
	reflect.TypeOf(&MessageAwaitReply{}):        ProtocolChainSync,
	reflect.TypeOf(&MessageRollForward{}):       ProtocolChainSync,
	reflect.TypeOf(&MessageRollBackward{}):      ProtocolChainSync,
	reflect.TypeOf(&MessageFindIntersect{}):     ProtocolChainSync,
	reflect.TypeOf(&MessageIntersectFound{}):    ProtocolChainSync,
	reflect.TypeOf(&MessageIntersectNotFound{}): ProtocolChainSync,
	reflect.TypeOf(&MessageChainSyncDone{}):     ProtocolChainSync,
	reflect.TypeOf(&MessageRequestRange{}):      ProtocolBlockFetch,
	reflect.TypeOf(&MessageClientDone{}):        ProtocolBlockFetch,
	reflect.TypeOf(&MessageStartBatch{}):        ProtocolBlockFetch,
	reflect.TypeOf(&MessageNoBlocks{}):          ProtocolBlockFetch,
	reflect.TypeOf(&MessageBlock{}):             ProtocolBlockFetch,
	reflect.TypeOf(&MessageBatchDone{}):         ProtocolBlockFetch,
	reflect.TypeOf(&MessageKeepAliveResponse{}): ProtocolKeepAlive,
	reflect.TypeOf(&MessageKeepAlive{}):         ProtocolKeepAlive,
}

var MessageSubprotocolMap = map[any]Subprotocol{
	reflect.TypeOf(&MessageProposeVersions{}):   SubprotocolHandshakeProposedVersion,
	reflect.TypeOf(&MessageRefuse{}):            SubprotocolHandshakeRefuse,
	reflect.TypeOf(&MessageQueryReply{}):        SubprotocolHandshakeQueryReply,
	reflect.TypeOf(&MessageAcceptVersion{}):     SubprotocolHandshakeAcceptVersion,
	reflect.TypeOf(&MessageRequestNext{}):       SubprotocolChainSyncRequestNext,
	reflect.TypeOf(&MessageAwaitReply{}):        SubprotocolChainSyncAwaitReply,
	reflect.TypeOf(&MessageRollForward{}):       SubprotocolChainSyncRollForward,
	reflect.TypeOf(&MessageRollBackward{}):      SubprotocolChainSyncRollBackward,
	reflect.TypeOf(&MessageFindIntersect{}):     SubprotocolChainSyncFindIntersect,
	reflect.TypeOf(&MessageIntersectFound{}):    SubprotocolChainSyncIntersectFound,
	reflect.TypeOf(&MessageIntersectNotFound{}): SubprotocolChainSyncIntersectNotFound,
	reflect.TypeOf(&MessageChainSyncDone{}):     SubprotocolChainSyncDone,
	reflect.TypeOf(&MessageRequestRange{}):      SubprotocolBlockFetchRequestRange,
	reflect.TypeOf(&MessageClientDone{}):        SubprotocolBlockFetchClientDone,
	reflect.TypeOf(&MessageStartBatch{}):        SubprotocolBlockFetchStartBatch,
	reflect.TypeOf(&MessageNoBlocks{}):          SubprotocolBlockFetchNoBlocks,
	reflect.TypeOf(&MessageBlock{}):             SubprotocolBlockFetchBlock,
	reflect.TypeOf(&MessageBatchDone{}):         SubprotocolBlockFetchBatchDone,
	reflect.TypeOf(&MessageKeepAliveResponse{}): SubprotocolKeepAliveEcho,
	reflect.TypeOf(&MessageKeepAlive{}):         SubprotocolKeepAlivePing,
}

func MessageToProtocol(message any) (protocol Protocol, subprotocol Subprotocol, err error) {
	protocol, ok := MessageProtocolMap[reflect.TypeOf(message)]
	if !ok {
		err = errors.Errorf("unknown protocol for message %T", message)
		return
	}

	subprotocol, ok = MessageSubprotocolMap[reflect.TypeOf(message)]
	if !ok {
		err = errors.Errorf("unknown subprotocol for message %T", message)
		return
	}

	return
}

func (p Protocol) String() string {
	if s, ok := ProtocolStringMap[p]; ok {
		return fmt.Sprintf("%s (%d)", s, p)
	} else {
		return "unknown"
	}
}

type HexString string

func (hs HexString) Bytes() (b []byte) {
	b, _ = hex.DecodeString(string(hs))
	return
}

type Base58Bytes []byte

func (b Base58Bytes) String() string {
	return base58.Encode(b)
}

func (b Base58Bytes) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, b)), nil
}

type WithSubprotocol struct {
	_           struct{}    `cbor:",toarray"`
	Subprotocol Subprotocol `json:"subprotocol"`
}

type NetworkMagic uint64

const (
	NetworkMagicMainnet       NetworkMagic = 764824073
	NetworkMagicLegacyTestnet NetworkMagic = 1097911063
	NetworkMagicPreProd       NetworkMagic = 1
	NetworkMagicPreview       NetworkMagic = 2
	NetworkMagicSanchonet     NetworkMagic = 4
)

const (
	RelayMainnet   = "relays-new.cardano-mainnet.iohk.io:3001"
	RelayTestnet   = "relays-new.cardano-testnet.iohkdev.io:3001"
	RelayPreProd   = "preprod-node.world.dev.cardano.org:30000"
	RelayPreview   = "preview-node.world.dev.cardano.org:30002"
	RelaySanchonet = "sanchonet-node.play.dev.cardano.org:3001"
)
