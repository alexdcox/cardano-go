package main

import "github.com/fxamacker/cbor/v2"

type MessageRequestRange struct {
	WithSubprotocol
	From Point `json:"from"`
	To   Point `json:"to"`
}

type MessageClientDone struct {
	WithSubprotocol
}

type MessageStartBatch struct {
	WithSubprotocol
}

type MessageNoBlocks struct {
	WithSubprotocol
}

type MessageBlock struct {
	WithSubprotocol
	BlockData []byte
}

func (b *MessageBlock) Block() (block *Block, err error) {
	block = &Block{}
	err = cbor.Unmarshal(b.BlockData, block)
	return
}

type MessageBatchDone struct {
	WithSubprotocol
}
