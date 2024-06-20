package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/alexdcox/dashutil/base58"
	"github.com/fxamacker/cbor/v2"
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

var ProtocolMessageMap = map[Protocol]map[Subprotocol]Message{
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

func ProtocolToMessage(protocol Protocol, subprotocol Subprotocol) (message Message, err error) {
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

var MessageSubprotocolMap = map[reflect.Type]Subprotocol{
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

func MessageToProtocol(message Message) (protocol Protocol, subprotocol Subprotocol, err error) {
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

type Message interface {
	SetSubprotocol(Subprotocol)
}

type WithSubprotocol struct {
	_           struct{}    `cbor:",toarray"`
	Subprotocol Subprotocol `json:"subprotocol"`
}

func (w *WithSubprotocol) SetSubprotocol(subprotocol Subprotocol) {
	w.Subprotocol = subprotocol
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

type Point struct {
	_    struct{}    `cbor:",toarray"`
	Slot uint64      `cbor:",omitempty" json:"slot"`
	Hash Base58Bytes `chor:",omitempty" json:"hash"`
}

var WellKnownMainnetPoint Point = Point{
	Slot: 16588737,
	Hash: HexString("4e9bbbb67e3ae262133d94c3da5bffce7b1127fc436e7433b87668dba34c354a").Bytes(),
}

var WellKnownTestnetPoint Point = Point{
	Slot: 13694363,
	Hash: HexString("b596f9739b647ab5af901c8fc6f75791e262b0aeba81994a1d622543459734f2").Bytes(),
}

var WellKnownPreprodPoint Point = Point{
	Slot: 87480,
	Hash: HexString("528c3e6a00c82dd5331b116103b6e427acf447891ce3ade6c4c7a61d2f0a2b1c").Bytes(),
}

var WellKnownPreviewPoint Point = Point{
	Slot: 8000,
	Hash: HexString("70da683c00985e23903da00656fae96644e1f31dce914aab4ed50e35e4c4842d").Bytes(),
}

var WellKnownSanchonetPoint Point = Point{
	Slot: 20,
	Hash: HexString("6a7d97aae2a65ca790fd14802808b7fce00a3362bd7b21c4ed4ccb4296783b98").Bytes(),
}

type Tip struct {
	_     struct{} `cbor:",toarray"`
	Point Point    `json:"point"`
	Block int64    `json:"block"`
}

func (t Tip) String() string {
	return fmt.Sprintf("%d/%d/%x", t.Block, t.Point.Slot, t.Point.Hash)
}

type HasSubtypes interface {
	Subtypes() []any
}

type SubtypeOf[T HasSubtypes] struct {
	Subtype any
}

func (r SubtypeOf[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"type":  fmt.Sprintf("%T", r.Subtype),
		"value": r.Subtype,
	})

	// TODO: hide the type - only useful while testing things

	// return json.Marshal(r.Subtype)
}

func (r *SubtypeOf[T]) UnmarshalCBOR(bytes []byte) (err error) {
	for _, subtype := range (*new(T)).Subtypes() {
		if err = cbor.Unmarshal(bytes, subtype); err == nil {
			r.Subtype = subtype
			return
		}
	}

	return errors.WithStack(CBORUnmarshalError{Target: new(T), Bytes: bytes})
}

type Optional[T any] struct {
	Valid bool
	Value *T
}

func (o *Optional[T]) UnmarshalCBOR(data []byte) error {
	o.Valid = true
	err := cbor.Unmarshal(data, &o.Value)
	if err != nil && err.Error() == "empty" {
		o.Valid = false
		err = nil
	}
	return errors.WithStack(err)
}

func (f Optional[T]) MarshalCBOR() (out []byte, err error) {
	if !f.Valid || f.Value == nil {
		return []byte("[]"), nil
	}
	out, err = cbor.Marshal(*f.Value)
	err = errors.WithStack(err)
	return
}

func (o *Optional[T]) UnmarshalJSON(data []byte) error {
	o.Valid = true
	return errors.WithStack(json.Unmarshal(data, &o.Value))
}

func (f Optional[T]) MarshalJSON() (out []byte, err error) {
	if !f.Valid || f.Value == nil {
		return []byte("null"), nil
	}
	out, err = json.Marshal(*f.Value)
	err = errors.WithStack(err)
	return
}

type CBORUnmarshalError struct {
	Target any
	Bytes  []byte
}

func (c CBORUnmarshalError) Error() string {
	var a any
	if err := cbor.Unmarshal(c.Bytes, &a); err == nil {
		fmt.Println(a)
	}

	f, _, _ := cbor.DiagnoseFirst(c.Bytes)
	return fmt.Sprintf("unable to unmarshal %T from cbor:\n%x\n%v\n%s", c.Target, c.Bytes, a, f)
}
