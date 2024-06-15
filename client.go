package main

import (
	"crypto/rand"
	"encoding/binary"
	"net"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/pkg/errors"
)

const DefaultKeepAliveInterval = time.Second * 30

func NewClient(hostport string) *Client {
	return &Client{
		hostport:          hostport,
		keepAliveInterval: DefaultKeepAliveInterval,
		inSegmentReader:   NewSegmentReader(DirectionIn),
	}
}

type Client struct {
	conn              net.Conn
	hostport          string
	keepAliveInterval time.Duration
	inSegmentReader   *SegmentReader
}

func (c *Client) Dial() (err error) {
	c.conn, err = net.Dial("tcp", c.hostport)
	err = errors.WithStack(err)
	return
}

func (c *Client) Handshake() (err error) {
	message := MessageProposeVersions{
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

	err = c.SendMessage(message)
	if err != nil {
		return
	}

	select {
	case segment := <-c.inSegmentReader.Stream:
		ireply, err2 := handleSegment(segment)
		if err2 != nil {
			err = err2
			return
		}

		if reply, ok := ireply.(*MessageAcceptVersion); ok {
			log.Info().Msgf("handshake ok, node accepted version %d", reply.Version)
			return
		} else {
			err = errors.Errorf(
				"handshake failed, received message %T, expected %T",
				ireply,
				&MessageAcceptVersion{},
			)
		}

	case <-time.After(time.Second * 3):
		err = errors.New("timeout while waiting for node version response")
	}

	return
}

func (c *Client) KeepAlive() {
	for {
		time.Sleep(time.Second * 30)

		cookieBytes := make([]byte, 4)
		_, _ = rand.Read(cookieBytes)
		cookie := binary.BigEndian.Uint16(cookieBytes)

		err := c.SendMessage(&MessageKeepAlive{
			Cookie: cookie,
		})
		if err != nil {
			log.Error().Err(err)
		}
	}
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

func (c *Client) FetchBlock(point Point) (block Block, err error) {
	panic("not implemented")

	err = c.SendMessage(&MessageStartBatch{})
	if err != nil {
		return
	}

	return
}
