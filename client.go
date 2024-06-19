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
		keepAliveChan:     make(chan *MessageKeepAliveResponse, 1),
		inSegmentReader:   NewSegmentReader(DirectionIn),
	}
}

type Client struct {
	conn              net.Conn
	hostport          string
	keepAliveInterval time.Duration
	keepAliveChan     chan *MessageKeepAliveResponse
	inSegmentReader   *SegmentReader
	batching          bool
}

func (c *Client) Dial() (err error) {
	c.conn, err = net.Dial("tcp", c.hostport)
	err = errors.WithStack(err)
	return
}

func (c *Client) Handshake() (err error) {
	messageProposeVersions := MessageProposeVersions{
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

	err = c.SendMessage(messageProposeVersions)
	if err != nil {
		return
	}

	select {
	case segment := <-c.inSegmentReader.Stream:
		if reply, ok := segment.Message.(*MessageAcceptVersion); ok {
			log.Info().Msgf("handshake ok, node accepted version %d", reply.Version)
			return
		} else {
			err = errors.Errorf(
				"handshake failed, received message %T, expected %T",
				segment.Message,
				&MessageAcceptVersion{},
			)
		}

	case <-time.After(time.Second * 3):
		err = errors.New("timeout while waiting for node version response")
	}

	for {
		segment, ok := <-c.inSegmentReader.Stream
		if !ok {
			break
		}

		err = c.handleMessage(segment.Message)
		if err != nil {
			log.Fatal().Msgf("%+v", err)
		}
	}

	return
}

func (c *Client) handleMessage(message any) (err error) {
	switch m := message.(type) {
	case *MessageKeepAliveResponse:
		c.keepAliveChan <- m
	case *MessageBlock:
		return c.handleMessageBlock(m)
	case *MessageRollForward:
		return c.handleMessageRollForward(m)
	case *MessageRollBackward:
		return c.handleMessageRollBackward(m)
	case *MessageStartBatch:
		return c.handleMessageStartBatch(m)
	case *MessageBatchDone:
		return c.handleMessageBatchDone(m)
	case *MessageAwaitReply:
		return c.handleMessageAwaitReply(m)
	case *MessageIntersectFound:
		return c.handleMessageIntersectFound(m)
	case *MessageFindIntersect:
		return c.handleMessageFindIntersect(m)
	case *MessageRequestNext:
		return c.handleMessageRequestNext(m)
	}

	return errors.Errorf("no client handler for message type %T", message)
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

	err = c.SendMessage(&MessageRequestRange{})
	if err != nil {
		return
	}

	return
}

func (c *Client) handleMessageStartBatch(m *MessageStartBatch) (err error) {
	c.batching = true
	return
}

func (c *Client) handleMessageBatchDone(m *MessageBatchDone) (err error) {
	c.batching = false
	return
}

func (c *Client) handleMessageAwaitReply(m *MessageAwaitReply) (err error) {
	panic("not implemented")
}

func (c *Client) handleMessageIntersectFound(m *MessageIntersectFound) (err error) {
	panic("not implemented")
}

func (c *Client) handleMessageRollBackward(m *MessageRollBackward) (err error) {
	panic("not implemented")
}

func (c *Client) handleMessageRollForward(m *MessageRollForward) (err error) {
	panic("not implemented")
}

func (c *Client) handleMessageAcceptVersion(m *MessageAcceptVersion) (err error) {
	panic("not implemented")
}

func (c *Client) handleMessageFindIntersect(m *MessageFindIntersect) (err error) {
	panic("not implemented")
}

func (c *Client) handleMessageRequestNext(m *MessageRequestNext) (err error) {
	panic("not implemented")
}

func (c *Client) handleMessageBlock(m *MessageBlock) (err error) {
	panic("not implemented")
}
