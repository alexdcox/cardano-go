package main

type MessageKeepAlive struct {
	WithSubprotocol
	Cookie uint16 `json:"cookie,omitempty"`
}

type MessageKeepAliveResponse struct {
	WithSubprotocol
	Cookie uint16 `json:"cookie,omitempty"`
}
