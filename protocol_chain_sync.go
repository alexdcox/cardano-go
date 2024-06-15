package main

import (
	"encoding/json"

	"github.com/fxamacker/cbor/v2"
	"github.com/pkg/errors"
)

type MessageRequestNext struct {
	WithSubprotocol
}

type MessageAwaitReply struct {
	WithSubprotocol
}

type MessageRollForward struct {
	WithSubprotocol
	Data struct {
		_           struct{}    `cbor:",toarray"`
		Number      uint64      `json:"number"`
		BlockHeader Base58Bytes `json:"content"`
	} `json:"data"`
	Tip Tip `json:"tip"`
}

func (m MessageRollForward) MarshalJSON() (out []byte, err error) {
	// TODO: What's the number all about? The era? I'm dropping it atm but perhaps shouldn't.

	blockHeader := &BlockHeader{}
	err = cbor.Unmarshal(m.Data.BlockHeader, blockHeader)
	if err != nil {
		err = errors.WithStack(err)
		return
	}
	return json.Marshal(blockHeader)
}

type MessageRollBackward struct {
	WithSubprotocol
	Point Optional[Point] `json:"point,omitempty"`
	Tip   Tip             `json:"tip,omitempty"`
}

type MessageFindIntersect struct {
	WithSubprotocol
	Points []Point `json:"points"`
}

type MessageIntersectFound struct {
	WithSubprotocol
	Point Point `json:"point,omitempty"`
	Tip   Tip   `json:"tip,omitempty"`
}

type MessageIntersectNotFound struct {
	WithSubprotocol
	Tip Tip `json:"tip"`
}

type MessageChainSyncDone struct {
	WithSubprotocol
}
