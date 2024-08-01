package cardano

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
		_           struct{} `cbor:",toarray"`
		Number      uint64   `json:"number"`
		BlockHeader HexBytes `json:"content"`
	} `json:"data"`
	Tip Tip `json:"tip"`
}

func (m MessageRollForward) BlockHeader() (header *BlockHeader, err error) {
	header = &BlockHeader{}
	err = errors.WithStack(cbor.Unmarshal(m.Data.BlockHeader, header))
	return
}

func (m MessageRollForward) MarshalJSON() (out []byte, err error) {
	blockHeader, err := m.BlockHeader()
	if err != nil {
		return
	}

	out, err = json.Marshal(map[string]any{
		"number": m.Data.Number, // TODO: What's the number all about?
		"header": blockHeader,
	})
	err = errors.WithStack(err)

	return
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
