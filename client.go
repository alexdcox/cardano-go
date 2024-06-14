package main

import (
	"net"

	"github.com/fxamacker/cbor/v2"
	"github.com/pkg/errors"
)

func NewClient(hostport string) *Client {
	return &Client{
		hostport: hostport,
	}
}

type Client struct {
	conn     net.Conn
	hostport string
}

func (c *Client) Dial() (err error) {
	c.conn, err = net.Dial("tcp", c.hostport)
	err = errors.WithStack(err)
	return
}

func (c *Client) Handshake() (err error) {
	message := MessageProposedVersions{
		WithSubprotocol: WithSubprotocol{
			Subprotocol: SubprotocolHandshakeProposedVersion,
		},
		VersionMap: VersionMap{
			V13: VersionField2{
				Network:                    NetworkMagicMainnet,
				InitiatorOnlyDiffusionMode: true,
				PeerSharing:                0,
				Query:                      false,
			},
		},
	}

	return c.SendMessage(message)
}

func (c *Client) SendMessage(message any) (err error) {
	messageCbor, err := cbor.Marshal(message)
	if err != nil {
		err = errors.WithStack(err)
		return
	}

	n, err := c.conn.Write(messageCbor)
	if err != nil {
		err = errors.WithStack(err)
		return
	}

	if n < len(messageCbor) {
		err = errors.Errorf(
			"write error: expected to write %d bytes, managed %d",
			len(messageCbor),
			n,
		)
	}

	return
}
