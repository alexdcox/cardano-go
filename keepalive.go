package main

type MessageKeepAlivePing struct {
	WithSubprotocol
	Word uint16 `json:"word,omitempty"`
}

type MessageKeepAliveEcho struct {
	WithSubprotocol
	Word uint16 `json:"word,omitempty"`
}
