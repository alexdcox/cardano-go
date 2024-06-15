package main

type MessageSubmitTx struct {
	WithSubprotocol
	Tx TransactionBody
}

type MessageAcceptTx struct {
	WithSubprotocol
}

type MessageRejectTx struct {
	WithSubprotocol
	Reason uint64
}
