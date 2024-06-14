package main

type MessageAwaitReply struct {
	WithSubprotocol
}

type MessageIntersectFound struct {
	WithSubprotocol
	Point Point `json:"point,omitempty"`
	Tip   Tip   `json:"tip,omitempty"`
}

type MessageChainSyncRequestNext struct {
	WithSubprotocol
}

type MessageChainSync4TODO struct {
	WithSubprotocol
	A []struct {
		_     struct{} `cbor:",toarray"`
		Point uint64
		Hash  []byte
	}
}

type MessageRollForward struct {
	WithSubprotocol
	A any
	B any
	// ByronEbBlock ByronEbBlock
	// ByronMainBlock ByronMainBlock
	// Block          Block
	// Tip            Tip
}

// type ByronEbBlock struct {
// 	Header ByronEbHead
// 	Body   ByronEbBody
// 	Cbor   string
// }
//
// type ByronEbHead struct {
// 	ProtocolMagic uint64
// 	PrevBlock     string
// 	BodyProof     string
// 	ConsensusData ByronEbBlockCons
// 	ExtraData     string
// }
//
// type ByronEbBlockCons struct {
// 	Epoch      uint64
// 	Difficulty uint64
// }
//
// type ByronEbBody struct {
// 	StakeholderIds []string
// }

// type ByronMainBlock struct {
// 	Header ByronBlockHead
// 	Body   ByronBlockBody
// 	Cbor   String
// }

type MessageRollBackward struct {
	WithSubprotocol
	Point Optional[Point] `json:"point,omitempty"`
	Tip   Tip             `json:"tip,omitempty"`
}
