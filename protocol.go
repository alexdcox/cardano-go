package main

import (
	"fmt"
	"reflect"

	"github.com/pkg/errors"
)

type Protocol uint16
type Subprotocol uint16

const (
	ProtocolHandshake Protocol = iota
	ProtocolMISSING
	ProtocolChainSync
	ProtocolBlockFetch
	ProtocolTxSubmission
	ProtocolLocalChainSync
	ProtocolLocalTx
	ProtocolLocalState
	ProtocolKeepAlive
	ProtocolLocalTxMonitor
)
const (
	SubprotocolHandshakeProposedVersion Subprotocol = iota
	SubprotocolHandshakeAcceptVersion
)
const (
	SubprotocolChainSync0Unknown Subprotocol = iota
	SubprotocolChainSyncAwaitReply
	SubprotocolChainSyncRollForward
	SubprotocolChainSyncRollBackward
	SubprotocolChainSync4Unknown
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
	ProtocolLocalTxMonitor: "local tx monitor",
}

var ProtocolMessageMap = map[Protocol]map[Subprotocol]any{
	ProtocolHandshake: {
		SubprotocolHandshakeProposedVersion: &MessageProposedVersions{},
		SubprotocolHandshakeAcceptVersion:   &MessageAcceptVersion{},
	},
	ProtocolChainSync: {
		SubprotocolChainSync0Unknown:       &MessageChainSyncRequestNext{},
		SubprotocolChainSyncAwaitReply:     &MessageAwaitReply{},
		SubprotocolChainSyncRollForward:    &MessageRollForward{},
		SubprotocolChainSyncRollBackward:   &MessageRollBackward{},
		SubprotocolChainSync4Unknown:       &MessageChainSync4TODO{},
		SubprotocolChainSyncIntersectFound: &MessageIntersectFound{},
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
		SubprotocolKeepAliveEcho: &MessageKeepAliveEcho{},
		SubprotocolKeepAlivePing: &MessageKeepAlivePing{},
	},
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
	reflect.TypeOf(&MessageProposedVersions{}):     ProtocolHandshake,
	reflect.TypeOf(&MessageAcceptVersion{}):        ProtocolHandshake,
	reflect.TypeOf(&MessageChainSyncRequestNext{}): ProtocolChainSync,
	reflect.TypeOf(&MessageAwaitReply{}):           ProtocolChainSync,
	reflect.TypeOf(&MessageRollForward{}):          ProtocolChainSync,
	reflect.TypeOf(&MessageRollBackward{}):         ProtocolChainSync,
	reflect.TypeOf(&MessageChainSync4TODO{}):       ProtocolChainSync,
	reflect.TypeOf(&MessageIntersectFound{}):       ProtocolChainSync,
	reflect.TypeOf(&MessageRequestRange{}):         ProtocolBlockFetch,
	reflect.TypeOf(&MessageClientDone{}):           ProtocolBlockFetch,
	reflect.TypeOf(&MessageStartBatch{}):           ProtocolBlockFetch,
	reflect.TypeOf(&MessageNoBlocks{}):             ProtocolBlockFetch,
	reflect.TypeOf(&MessageBlock{}):                ProtocolBlockFetch,
	reflect.TypeOf(&MessageBatchDone{}):            ProtocolBlockFetch,
	reflect.TypeOf(&MessageKeepAliveEcho{}):        ProtocolKeepAlive,
	reflect.TypeOf(&MessageKeepAlivePing{}):        ProtocolKeepAlive,
}

var MessageSubprotocolMap = map[any]Subprotocol{
	reflect.TypeOf(&MessageProposedVersions{}):     SubprotocolHandshakeProposedVersion,
	reflect.TypeOf(&MessageAcceptVersion{}):        SubprotocolHandshakeAcceptVersion,
	reflect.TypeOf(&MessageChainSyncRequestNext{}): SubprotocolChainSync0Unknown,
	reflect.TypeOf(&MessageAwaitReply{}):           SubprotocolChainSyncAwaitReply,
	reflect.TypeOf(&MessageRollForward{}):          SubprotocolChainSyncRollForward,
	reflect.TypeOf(&MessageRollBackward{}):         SubprotocolChainSyncRollBackward,
	reflect.TypeOf(&MessageChainSync4TODO{}):       SubprotocolChainSync4Unknown,
	reflect.TypeOf(&MessageIntersectFound{}):       SubprotocolChainSyncIntersectFound,
	reflect.TypeOf(&MessageRequestRange{}):         SubprotocolBlockFetchRequestRange,
	reflect.TypeOf(&MessageClientDone{}):           SubprotocolBlockFetchClientDone,
	reflect.TypeOf(&MessageStartBatch{}):           SubprotocolBlockFetchStartBatch,
	reflect.TypeOf(&MessageNoBlocks{}):             SubprotocolBlockFetchNoBlocks,
	reflect.TypeOf(&MessageBlock{}):                SubprotocolBlockFetchBlock,
	reflect.TypeOf(&MessageBatchDone{}):            SubprotocolBlockFetchBatchDone,
	reflect.TypeOf(&MessageKeepAliveEcho{}):        SubprotocolKeepAliveEcho,
	reflect.TypeOf(&MessageKeepAlivePing{}):        SubprotocolKeepAlivePing,
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

type ByteString []uint8

func (b ByteString) String() string {
	return fmt.Sprintf("%x", string(b))
}

func (b ByteString) MarshalJSON() ([]byte, error) {
	return []byte(`"` + fmt.Sprintf("%x", b) + `"`), nil
}

type WithSubprotocol struct {
	_           struct{}    `cbor:",toarray"`
	Subprotocol Subprotocol `json:"subprotocol"`
}

type NetworkMagic uint64

const (
	NetworkMagicMainnet NetworkMagic = 764824073
)
