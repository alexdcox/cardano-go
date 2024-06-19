package main

import (
	"crypto/rand"
	"encoding/binary"
	"math"
	"net"
	"reflect"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

const DefaultKeepAliveInterval = time.Second * 30

func NewClient(hostport string, networkMagic NetworkMagic) *Client {
	client := &Client{
		hostport:          hostport,
		keepAliveInterval: DefaultKeepAliveInterval,
		keepAliveChan:     make(chan *MessageKeepAliveResponse, 1),
		inSegmentReader:   NewSegmentReader(DirectionIn),
		networkMagic:      networkMagic,
		log:               globalLog,
	}

	return client
}

type Client struct {
	conn              net.Conn
	hostport          string
	keepAliveInterval time.Duration
	keepAliveChan     chan *MessageKeepAliveResponse
	inSegmentReader   *SegmentReader
	batching          bool
	networkMagic      NetworkMagic
	log               zerolog.Logger
}

func (c *Client) Dial() (err error) {
	c.log.Info().Msgf("dialing node %s", c.hostport)
	c.conn, err = net.Dial("tcp", c.hostport)
	if err != nil {
		return errors.WithStack(err)
	}
	c.log.Info().Msg("connected to node")

	c.inSegmentReader = NewSegmentReader(DirectionIn)
	go c.beginReadStream()

	return
}

func (c *Client) beginReadStream() {
	for {
		buf := make([]byte, int(math.Pow(2, 20)))
		n, err := c.conn.Read(buf)
		if err != nil {
			c.log.Error().Msgf("%+v", errors.WithStack(err))
			time.Sleep(time.Second * 3)
			continue
		}
		_, err = c.inSegmentReader.Read(buf[:n])
		if err != nil {
			c.log.Error().Msgf("%+v", errors.WithStack(err))
			time.Sleep(time.Second * 3)
			continue
		}
	}
}

func (c *Client) Handshake() (err error) {
	c.log.Info().Msg("begin handshake")
	messageProposeVersions := &MessageProposeVersions{
		VersionMap: defaultVersionMap(c.networkMagic),
	}

	err = c.SendMessage(messageProposeVersions)
	if err != nil {
		return
	}

	c.log.Info().Msg("wait for version accept message")

	acceptVersionMsg, err := c.ReceiveNextWithTimeout(time.Second * 10)
	if reply, ok := acceptVersionMsg.(*MessageAcceptVersion); ok {
		c.log.Info().Msgf("handshake ok, node accepted version %d", reply.Version)
		return
	} else {
		err = errors.Errorf(
			"handshake failed, received message %T, expected %T",
			acceptVersionMsg,
			&MessageAcceptVersion{},
		)
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
			c.log.Error().Err(err)
		}
	}
}

func (c *Client) SendMessage(message any) (err error) {
	segment := &Segment{
		Timestamp: 0,
		Direction: DirectionOut,
	}

	err = segment.SetMessage(message)
	if err != nil {
		return
	}

	writeBytes, err := segment.MarshalDataItem()
	if err != nil {
		return errors.WithStack(err)
	}

	c.log.Debug().Msgf("sending message %T\n%x", message, writeBytes)

	n, err := c.conn.Write(writeBytes)
	if err != nil {
		return errors.WithStack(err)
	}

	if n < len(writeBytes) {
		err = errors.Errorf(
			"write error: expected to write %d bytes, managed %d",
			len(writeBytes),
			n,
		)
	}

	return
}

func (c *Client) FetchBlock(point Point) (block *Block, err error) {

	// err = c.SendMessage(&MessageFindIntersect{
	// 	Points: []Point{WellKnownMainnetPoint},
	// })
	// if err != nil {
	// 	return
	// }
	//
	// intersectFoundResponse, err := c.ReceiveNext()
	// if err != nil {
	// 	return
	// }
	//
	// if reflect.TypeOf(intersectFoundResponse) != reflect.TypeOf(&intersectFoundResponse{}) {
	// 	err = errors.Errorf("expected intersect found message, got %T", intersectFoundResponse)
	// 	return
	// }

	err = c.SendMessage(&MessageRequestRange{
		From: point,
		To:   point,
	})
	if err != nil {
		return
	}

	for {
		var next any

		next, err = c.ReceiveNext()
		if err != nil {
			return
		}

		if reflect.TypeOf(next) == reflect.TypeOf(&MessageStartBatch{}) {
			continue
		}

		if reflect.TypeOf(next) != reflect.TypeOf(&MessageBlock{}) {
			err = errors.Errorf("expected block message, got %T", next)
			return
		}

		msg := next.(*MessageBlock)
		return msg.Block()
	}
}

func (c *Client) ReceiveNext() (message any, err error) {
	segment, ok := <-c.inSegmentReader.Stream
	if !ok {
		err = errors.New("segment stream closed")
		return
	}
	for {
		message, err = c.messageFromSegment(segment)
		if err != nil {
			return
		}
		if reflect.TypeOf(message) == reflect.TypeOf(&MessageKeepAlive{}) {
			err = errors.New("unexpected keep alive")
			return
		}
		if reflect.TypeOf(message) == reflect.TypeOf(&MessageKeepAliveResponse{}) {
			c.log.Info().Msg("got keep alive response, forwarding to alternative channel and reading next message")
			c.keepAliveChan <- message.(*MessageKeepAliveResponse)
			continue
		}
		return
	}
}

func (c *Client) ReceiveNextWithTimeout(timeout time.Duration) (message any, err error) {
	select {
	case segment := <-c.inSegmentReader.Stream:
		return c.messageFromSegment(segment)

	case <-time.After(timeout):
		err = errors.New("timeout while waiting for node version response")
		return
	}
}

func (c *Client) messageFromSegment(segment *Segment) (message any, err error) {
	if segment.Message == nil {
		err = errors.New("invalid segment, no message")
		return
	}

	message = segment.Message
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
