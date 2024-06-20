package main

type MessageSubmitTx struct {
	WithSubprotocol
	BodyType Era
	TxBytes  []byte
}

type MessageAcceptTx struct {
	WithSubprotocol
}

type MessageRejectTx struct {
	WithSubprotocol
	Reason uint64
}
