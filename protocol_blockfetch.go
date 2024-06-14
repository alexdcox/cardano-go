package main

type MessageRequestRange struct {
	WithSubprotocol
	From Point
	To   Point
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
	BlockData ByteString
}
type MessageBatchDone struct {
	WithSubprotocol
}
