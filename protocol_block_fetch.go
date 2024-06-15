package main

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
	Block Block
}
type MessageBatchDone struct {
	WithSubprotocol
}
