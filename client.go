package cardano

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

func NewClient(hostport string, network Network) (client *Client, err error) {
	params, err := network.Params()
	if err != nil {
		return
	}

	client = &Client{
		hostport:          hostport,
		keepAliveInterval: DefaultKeepAliveInterval,
		keepAliveChan:     make(chan *MessageKeepAliveResponse, 1),
		inSegmentReader:   NewSegmentReader(DirectionIn),
		params:            params,
		log:               Log(),
		tipLoader:         FileSystemTipLoader("latest-tip.json"),
	}

	return
}

type Client struct {
	conn              net.Conn
	hostport          string
	keepAliveInterval time.Duration
	keepAliveChan     chan *MessageKeepAliveResponse
	inSegmentReader   *SegmentReader
	batching          bool
	params            *NetworkParams
	log               *zerolog.Logger
	tipLoader         TipStore
	tip               Tip
	shutdown          bool
}

func (c *Client) GetTip() (tip Tip, err error) {
	return c.tip, nil
}

func (c *Client) Start() (err error) {
	c.log.Info().Msg("starting client")

	err = c.dial()
	if err != nil {
		return
	}

	err = c.handshake()
	if err != nil {
		return
	}

	err = c.loadPreviousTip()
	if err != nil {
		return
	}

	go c.keepAlive()
	go c.followChain()

	return
}

func (c *Client) followChain() {
	err := c.sendMessage(&MessageFindIntersect{Points: []Point{{
		Slot: 1,
		Hash: make([]byte, 32),
	}}})
	if err != nil {
		c.log.Error().Msgf("follow chain failure: %+v", errors.WithStack(err))
		return
	}

	in, err := c.receiveNext()
	if err != nil {
		c.log.Error().Msgf("follow chain failure: %+v", errors.WithStack(err))
		return
	}

	notFound, ok := in.(*MessageIntersectNotFound)
	if !ok {
		c.log.Error().Msgf("follow chain failure: expecting intersect not found, got %T", in)
		return
	}

	err = c.sendMessage(&MessageFindIntersect{Points: []Point{notFound.Tip.Point}})
	if err != nil {
		c.log.Error().Msgf("follow chain failure: %+v", errors.WithStack(err))
		return
	}

	in, err = c.receiveNext()
	if err != nil {
		c.log.Error().Msgf("follow chain failure: %+v", errors.WithStack(err))
		return
	}

	intersect, ok := in.(*MessageIntersectFound)
	if !ok {
		c.log.Error().Msgf("follow chain failure: expecting intersect found, got %T", in)
		return
	}

	c.log.Info().Msgf("got intersect %+v", intersect)

	for {
		err = c.sendMessage(&MessageRequestNext{})
		if err != nil {
			c.log.Error().Msgf("follow chain failure: %+v", errors.WithStack(err))
			return
		}

		in, err = c.receiveNext()
		if err != nil {
			if c.shutdown {
				return
			}
			c.log.Error().Msgf("follow chain failure: %+v", errors.WithStack(err))
			return
		}

		c.log.Info().Msgf("got %T %+v", in, in)

		if rollForward, ok := in.(*MessageRollForward); ok {
			if Era(rollForward.Data.Number) < EraBabbage {
				c.log.Warn().Msgf("skipping 'number???': %d", rollForward.Data.Number)
				continue
			}

			c.tip = rollForward.Tip

			header, err2 := rollForward.BlockHeader()
			if err2 != nil {
				c.log.Error().Msgf("follow chain failure: unable to parse block header %+v\n%x", err2, []byte(rollForward.Data.BlockHeader))
			} else {
				c.log.Info().Msgf(
					"chain sync for era: %d, block: %d, slot: %d, hash: %s",
					rollForward.Data.Number,
					header.Body.Number,
					header.Body.Slot,
					header.Body.Hash,
				)
			}
		}
	}

}

var DefaultMainnetTip = Tip{
	Point: Point{
		Slot: 127350361,
		Hash: HexString("cb1a4a043fa0e00bc02945358216357cf314fcf69fca3ca0320614a0691dcd62").Bytes(),
	},
	Block: 10471759,
}

func (c *Client) loadPreviousTip() (err error) {
	c.tip, err = c.tipLoader.LoadTip()
	if err == nil {
		return
	}
	c.log.Warn().Msg("previous tip could not be loaded, using default tip")
	c.tip = DefaultMainnetTip
	return nil
}

func (c *Client) Stop() (err error) {
	if c.shutdown {
		return
	}
	c.shutdown = true
	c.log.Info().Msg("stopping client")

	err = errors.WithStack(c.conn.Close())
	if err != nil {
		return
	}

	return errors.WithStack(c.tipLoader.SaveTip(c.tip))
}

func (c *Client) dial() (err error) {
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
	defer close(c.inSegmentReader.Stream)
	for {
		if c.shutdown {
			return
		}

		buf := make([]byte, int(math.Pow(2, 20)))
		n, err := c.conn.Read(buf)
		if err != nil {
			if c.shutdown {
				c.log.Info().Msg("throwing away last read, client shutting down")
				return
			}
			c.log.Error().Msgf("%+v", errors.WithStack(err))
			time.Sleep(time.Second * 3)
			continue
		}

		c.log.Debug().Msgf("read: %x", buf[:n])

		_, err = c.inSegmentReader.Read(buf[:n])
		if err != nil {
			c.log.Error().Msgf("%+v", errors.WithStack(err))
			time.Sleep(time.Second * 3)
			continue
		}
	}
}

func (c *Client) handshake() (err error) {
	c.log.Info().Msg("begin handshake")
	messageProposeVersions := &MessageProposeVersions{
		VersionMap: defaultVersionMap(c.params.Magic),
	}

	err = c.sendMessage(messageProposeVersions)
	if err != nil {
		return
	}

	c.log.Info().Msg("wait for version accept message")

	acceptVersionMsg, err := c.receiveNextWithTimeout(time.Second * 10)
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

func (c *Client) handleMessage(message Message) (err error) {
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

func (c *Client) keepAlive() {
	for {
		time.Sleep(time.Second * 30)
		if c.shutdown {
			return
		}

		cookieBytes := make([]byte, 4)
		_, _ = rand.Read(cookieBytes)
		cookie := binary.BigEndian.Uint16(cookieBytes)

		err := c.sendMessage(&MessageKeepAlive{
			Cookie: cookie,
		})
		if err != nil {
			c.log.Error().Err(err)
		}
	}
}

func (c *Client) sendMessage(message Message) (err error) {
	if c.shutdown {
		err = errors.Errorf("dropping message %T, client shutting down", message)
		return
	}

	c.log.Info().Msgf("sending message %T", message)

	if subprotocol, ok := MessageSubprotocolMap[reflect.TypeOf(message)]; ok {
		c.log.Info().Msgf("message: %T, subprotocol: %v", message, subprotocol)
		message.SetSubprotocol(subprotocol)
	} else {
		return errors.Errorf("no subprotocol defined for message type %T", message)
	}

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

	c.log.Debug().Msgf("write: %x", writeBytes)

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

func (c *Client) FetchTip() (tip Tip, err error) {
	err = c.sendMessage(&MessageFindIntersect{
		Points: []Point{
			{
				Slot: 0,
				Hash: make([]byte, 32),
			},
		},
	})
	if err != nil {
		return
	}

	next, err := c.receiveNext()
	if err != nil {
		return
	}

	if intersectFound, ok := next.(*MessageIntersectNotFound); ok {
		tip = intersectFound.Tip
	} else {
		err = errors.Errorf("expected intersect found message, got %T", next)
	}

	return
}

func (c *Client) FetchLatestBlock() (block *Block, err error) {
	tip, err := c.FetchTip()
	if err != nil {
		return
	}

	return c.FetchBlock(tip.Point)
}

func (c *Client) FetchBlock(point Point) (block *Block, err error) {
	err = c.sendMessage(&MessageRequestRange{
		From: point,
		To:   point,
	})
	if err != nil {
		return
	}

	for {
		var next any

		next, err = c.receiveNext()
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

func (c *Client) receiveNext() (message Message, err error) {
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

func (c *Client) receiveNextWithTimeout(timeout time.Duration) (message any, err error) {
	select {
	case segment := <-c.inSegmentReader.Stream:
		return c.messageFromSegment(segment)

	case <-time.After(timeout):
		err = errors.New("timeout while waiting for node version response")
		return
	}
}

func (c *Client) messageFromSegment(segment *Segment) (message Message, err error) {
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
