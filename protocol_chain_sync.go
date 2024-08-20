package cardano

type MessageRequestNext struct {
	WithSubprotocol
}

type MessageAwaitReply struct {
	WithSubprotocol
}

type MessageRollForward struct {
	WithSubprotocol
	Data SubtypeOf[MessageRollForwardData] `json:"data"`
	Tip  PointAndBlockNum                  `json:"tip"`
}

type MessageRollForwardData struct{}

type MessageRollForwardDataA struct {
	_           struct{} `cbor:",toarray"`
	Number      uint64   `json:"number"`
	BlockHeader HexBytes `json:"content"`
}

type MessageRollForwardDataB struct {
	_        struct{} `cbor:",toarray"`
	EraMaybe uint64
	Header   struct {
		_ struct{} `cbor:",toarray"`
		A struct {
			_ struct{} `cbor:",toarray"`
			A any
			B any
		}
		B HexBytes
	}
}

func (m MessageRollForwardData) Subtypes() []any {
	return []any{
		&MessageRollForwardDataA{},
		&MessageRollForwardDataB{},
	}
}

func (m MessageRollForward) BlockHeader() (header *BlockHeader, err error) {
	panic("reimplement me")
	return
	// header = &BlockHeader{}
	// err = errors.WithStack(cbor.Unmarshal(m.Data.BlockHeader, header))
	// return
}

type MessageRollBackward struct {
	WithSubprotocol
	Point Optional[Point]  `json:"point,omitempty"`
	Tip   PointAndBlockNum `json:"tip,omitempty"`
}

type MessageFindIntersect struct {
	WithSubprotocol
	Points []Point `json:"points"`
}

type MessageIntersectFound struct {
	WithSubprotocol
	Point Point            `json:"point,omitempty"`
	Tip   PointAndBlockNum `json:"tip,omitempty"`
}

type MessageIntersectNotFound struct {
	WithSubprotocol
	Tip PointAndBlockNum `json:"tip"`
}

type MessageChainSyncDone struct {
	WithSubprotocol
}
