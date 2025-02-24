package cardano

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/alexdcox/cbor/v2"
	"github.com/pkg/errors"
)

type Protocol uint16
type Subprotocol uint16

const (
	// MinUtxoBytes The smallest size in bytes a cardano transaction can be.
	//
	// It was originally 160 as defined here:
	// https://github.com/IntersectMBO/cardano-ledger/blob/a6f276a9e3df45d69be3a593d3db6cd9dfaa7a02/eras/babbage/impl/src/Cardano/Ledger/Babbage/TxOut.hs#L680
	//
	// However, the smallest conway transaction I've been able to build was 197 bytes.
	MinUtxoBytes         = 197
	MaxAuxDataStringSize = 64
)

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
	SubprotocolLocalStateAcquire               = 0
	SubprotocolLocalStateAcquired              = 1
	SubprotocolLocalStateFailure               = 2
	SubprotocolLocalStateQuery                 = 3
	SubprotocolLocalStateResult                = 4
	SubprotocolLocalStateRelease               = 5
	SubprotocolLocalStateReacquire             = 6
	SubprotocolLocalStateDone                  = 7
	SubprotocolLocalStateAcquireVolatileTip    = 8
	SubprotocolLocalStateReacquireVolatileTip  = 9
	SubprotocolLocalStateAcquireImmutableTip   = 10
	SubprotocolLocalStateReacquireImmutableTip = 11
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
		SubprotocolHandshakeAcceptVersion:   &MessageAcceptVersionNtC{},
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
	ProtocolLocalState: {
		SubprotocolLocalStateAcquire:               &MessageLocalStateAcquire{},
		SubprotocolLocalStateAcquired:              &MessageLocalStateAcquired{},
		SubprotocolLocalStateFailure:               &MessageLocalStateFailure{},
		SubprotocolLocalStateQuery:                 &MessageLocalStateQuery{},
		SubprotocolLocalStateResult:                &MessageLocalStateResult{},
		SubprotocolLocalStateRelease:               &MessageLocalStateRelease{},
		SubprotocolLocalStateReacquire:             &MessageLocalStateReacquire{},
		SubprotocolLocalStateDone:                  &MessageLocalStateDone{},
		SubprotocolLocalStateAcquireVolatileTip:    &MessageLocalStateAcquireVolatileTip{},
		SubprotocolLocalStateReacquireVolatileTip:  &MessageLocalStateReacquireVolatileTip{},
		SubprotocolLocalStateAcquireImmutableTip:   &MessageLocalStateAcquireImmutableTip{},
		SubprotocolLocalStateReacquireImmutableTip: &MessageLocalStateReacquireImmutableTip{},
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
	reflect.TypeOf(&MessageAcceptVersionNtN{}):  ProtocolHandshake,
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
	reflect.TypeOf(&MessageAcceptVersionNtN{}):  SubprotocolHandshakeAcceptVersion,
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

type Message interface {
	SetSubprotocol(Subprotocol)
	GetSubprotocol() Subprotocol
	Raw() []byte
	SetRaw(raw []byte)
}

type WithSubprotocol struct {
	_           struct{}    `cbor:",toarray"`
	_raw        []byte      `json:"-" cbor:"-"`
	Subprotocol Subprotocol `json:"subprotocol"`
}

func (w *WithSubprotocol) SetSubprotocol(subprotocol Subprotocol) {
	w.Subprotocol = subprotocol
}

func (w *WithSubprotocol) GetSubprotocol() (subprotocol Subprotocol) {
	return w.Subprotocol
}

func (w *WithSubprotocol) Raw() []byte {
	return w._raw
}

func (w *WithSubprotocol) SetRaw(raw []byte) {
	w._raw = raw
}

type Point struct {
	_    struct{} `cbor:",toarray"`
	Slot uint64   `cbor:",omitempty" json:"slot"`
	Hash HexBytes `chor:",omitempty" json:"hash"`
}

func NewPoint() Point {
	return Point{
		Slot: 0,
		Hash: make([]byte, 32),
	}
}

func (p *Point) Equals(o Point) bool {
	return p.Slot == o.Slot && p.Hash.Equals(o.Hash)
}

func (p Point) String() string {
	return fmt.Sprintf("slot: %d | hash: %s", p.Slot, p.Hash)
}

func NewPointAndNum() PointAndBlockNum {
	return PointAndBlockNum{Point: NewPoint()}
}

type PointAndBlockNum struct {
	_     struct{} `cbor:",toarray"`
	Point Point    `json:"point"`
	Block uint64   `json:"block"`
}

func (t PointAndBlockNum) Equals(o PointAndBlockNum) bool {
	return t.Point.Equals(o.Point) && t.Block == o.Block
}

func (t PointAndBlockNum) String() string {
	return fmt.Sprintf("number: %d | %s", t.Block, t.Point)
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

func (r *SubtypeOf[T]) MarshalCBOR() (bytes []byte, err error) {
	return cbor.Marshal(r.Subtype)
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
		return cbor.Marshal([]any{})
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
