package cardano

import (
	"crypto/rand"
	"encoding/binary"
	"net"
	"reflect"
	"sync"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/blake2b"
)

type ClientOptions struct {
	HostPort           string
	Network            Network
	Database           Database
	StartPoint         PointAndBlockNum
	DisableFollowChain bool
	LogLevel           zerolog.Level
}

func (o *ClientOptions) setDefaults() {
	if o.HostPort == "" {
		o.HostPort = defaultClientOptions.HostPort
	}

	if o.Network == "" {
		o.Network = defaultClientOptions.Network
	}

	if o.Database == nil {
		o.Database = NewInMemoryDatabase()
	}

	if len(o.StartPoint.Point.Hash) == 0 {
		o.StartPoint = NewPointAndNum()
	}
}

var defaultClientOptions = &ClientOptions{
	HostPort: "localhost:3000",
	Network:  NetworkMainNet,
}

func NewClient(options *ClientOptions) (client *Client, err error) {
	if options == nil {
		options = &ClientOptions{}
	}
	options.setDefaults()

	params, err := options.Network.Params()
	if err != nil {
		return
	}

	clientLog := LogAtLevel(options.LogLevel)

	client = &Client{
		options: options,
		keepAliveCaller: NewPeriodicCaller(time.Second*3, func() {
			client.keepAlive()
		}),
		in:         NewMessageReader(clientLog),
		params:     params,
		log:        clientLog,
		db:         options.Database,
		pubsub:     NewQueue[any](),
		shutdownWg: &sync.WaitGroup{},
	}

	return
}

type Client struct {
	options         *ClientOptions
	conn            net.Conn
	keepAliveCaller *PeriodicCaller
	in              *MessageReader
	params          *NetworkParams
	log             *zerolog.Logger
	tip             PointAndBlockNum
	db              Database
	shutdown        bool
	shutdownWg      *sync.WaitGroup
	pubsub          PubSubQueue[any]
	latestEra       Era
}

func (c *Client) Start() (err error) {
	c.log.Info().Msg("starting client")

	err = c.dial()
	if err != nil {
		return
	}

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go c.beginReadStream(wg)
	wg.Wait()

	c.shutdownWg.Add(1)

	err = c.handshake()
	if err != nil {
		return
	}

	c.keepAliveCaller.Start()
	c.tip = c.options.StartPoint

	if !c.options.DisableFollowChain {
		go c.followChain()
	}

	return
}

func (c *Client) followChain() {
	if c.tip.Point.Slot == 0 {
		c.log.Info().Msg("no previous or configured tip, following chain from node tip")

		go func() {
			err := c.SendMessage(&MessageFindIntersect{Points: []Point{NewPoint()}})
			if err != nil {
				c.log.Error().Msgf("follow chain failure: %+v", errors.WithStack(err))
				return
			}
		}()

		notFound, err := ReceiveNextOfType[*MessageIntersectNotFound](c)
		if err != nil {
			c.log.Error().Msgf("follow chain failure: %+v", err)
			return
		}

		c.tip = notFound.Tip
	} else {
		c.log.Info().Msgf("following chain from previous tip: %s", c.tip)
	}

	go func() {
		err := c.SendMessage(&MessageFindIntersect{Points: []Point{c.tip.Point}})
		if err != nil {
			c.log.Error().Msgf("follow chain failure: %+v", errors.WithStack(err))
			return
		}
	}()

	_, err := ReceiveNextOfType[*MessageIntersectFound](c)
	if err != nil {
		c.log.Error().Msgf("follow chain failure: %+v", err)
		return
	}

	requestNext := func() (err error) {
		err = c.SendMessage(&MessageRequestNext{})
		if err != nil {
			c.log.Error().Msgf("follow chain failure: %+v", errors.WithStack(err))
			return
		}
		return
	}

	requestNext()

	c.pubsub.On(func(i any) {
		message, ok := i.(Message)
		if !ok {
			return
		}
		switch msg := message.(type) {
		case *MessageRollForward:
			var nextTip PointAndBlockNum
			if a, ok := msg.Data.Subtype.(*MessageRollForwardDataA); ok {
				hash := blake2b.Sum256(a.BlockHeader)

				header := &BlockHeader{}
				err = cbor.Unmarshal(a.BlockHeader, &header)
				if err != nil {
					log.Fatal().Msgf("%+v", errors.WithStack(err))
				}

				nextTip = PointAndBlockNum{
					Block: header.Body.Number,
					Point: Point{
						Slot: header.Body.Slot,
						Hash: hash[:],
					},
				}
			} else {
				log.Fatal().Msgf("unexpected roll forward message type: %T", msg)
			}

			block, err2 := c.FetchBlock(nextTip.Point)
			if err2 != nil {
				log.Error().Msgf("failed to fetch block post roll forward: %+v", err2)
				return
			}

			c.log.Info().Msgf("roll forward: %s", c.tip)
			c.pubsub.Broadcast(block)
			c.tip = nextTip
			c.latestEra = block.Era
			requestNext()

		case *MessageRollBackward:
			block, err2 := c.FetchBlock(*msg.Point.Value)
			if err2 != nil {
				log.Error().Msgf("failed to fetch block post roll backward: %+v", err2)
				return
			}
			c.tip = PointAndBlockNum{
				Point: *msg.Point.Value,
				Block: block.Data.Header.Body.Number,
			}
			c.log.Info().Msgf("roll backward: %s", c.tip)
			// TODO: Roll backwards logic (clean db)
			requestNext()

		case *MessageAwaitReply:
			// do as they say, patiently wait
		}
	})
}

func (c *Client) Stop() (err error) {
	if c.shutdown {
		c.log.Warn().Msg("client shutdown already in progress...")
		return
	}
	c.shutdown = true
	c.log.Info().Msg("stopping client")

	c.keepAliveCaller.Stop()

	err = errors.WithStack(c.conn.Close())
	if err != nil {
		return
	}

	return
}

func (c *Client) dial() (err error) {
	c.log.Info().Msgf("dialing node: '%s'", c.options.HostPort)
	c.conn, err = net.Dial("tcp", c.options.HostPort)
	if err != nil {
		return errors.WithStack(err)
	}
	c.log.Info().Msg("connected to node")

	return
}

func (c *Client) beginReadStream(wg *sync.WaitGroup) {
	wg.Done()
	for {
		if c.shutdown {
			return
		}

		buf := make([]byte, 402)
		n, err := c.conn.Read(buf)
		if err != nil {
			if c.shutdown {
				// NOTE: throwing away last read, client shutting down
				return
			}
			c.log.Error().Msgf("conn read error: %+v", errors.WithStack(err))
			// TODO: If the connection was reset by peer, we need to renegotiate the handshake.
			//       The error string looks like:
			//       read tcp 127.0.0.1:50430->127.0.0.1:3000: read: connection reset by peer
			return
		}

		messages, err := c.in.Read(buf[:n])
		if err != nil {
			c.log.Error().Msgf("%+v", errors.WithStack(err))
			return
		}
		for _, message := range messages {
			c.log.Debug().Msgf("message in: %T", message)
			c.pubsub.Broadcast(message)
		}
	}
}

func (c *Client) handshake() (err error) {
	c.log.Info().Msg("begin handshake")
	err = c.SendMessage(&MessageProposeVersions{
		VersionMap: defaultVersionMap(c.params.Magic),
	})
	if err != nil {
		return
	}

	acceptVersionMsg, err := ReceiveNextOfType[*MessageAcceptVersion](c)
	if err != nil {
		err = errors.Wrap(err, "handshake failed")
		return
	}

	c.log.Info().Msgf("handshake ok, node accepted version %d", acceptVersionMsg.Version)

	return
}

func (c *Client) restart() {
	c.Stop()
	var err error
	c, err = NewClient(c.options)
	if err != nil {
		log.Fatal().Msgf("failed to restart client: %+v", err)
	}
	c.Start()
}

func (c *Client) keepAlive() {
	cookieBytes := make([]byte, 4)
	_, _ = rand.Read(cookieBytes)
	cookie := binary.BigEndian.Uint16(cookieBytes)

	err := c.SendMessage(&MessageKeepAliveResponse{Cookie: cookie})
	if err != nil {
		c.log.Error().Msgf("failed to send keep alive message: %+v", err)
		c.restart()
		return
	}
}

func GetCardanoTimestamp() uint32 {
	mono := time.Now().UnixNano()
	microSeconds := mono / 1000
	return uint32(microSeconds & 0xFFFFFFFF)
}

func EncodeCardanoTimestamp(timestamp uint32) []byte {
	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, timestamp)
	return bytes
}

func (c *Client) SendSegment(segment *Segment) (err error) {
	writeBytes, err := segment.MarshalDataItem()
	if err != nil {
		return errors.WithStack(err)
	}

	for _, msg := range segment.messages {
		c.log.Debug().Msgf(
			"message out: %T, protocol: %d, subprotocol: %d",
			msg,
			segment.Protocol,
			msg.GetSubprotocol())
	}
	c.log.Trace().Msgf("write: %x", writeBytes)

	n, err := c.conn.Write(writeBytes)
	if err != nil {
		return errors.WithStack(err)
	}

	c.keepAliveCaller.Postpone()

	if n < len(writeBytes) {
		err = errors.Errorf(
			"write error: expected to write %d bytes, managed %d",
			len(writeBytes),
			n,
		)
	}

	return
}

func (c *Client) SendMessage(message Message) (err error) {
	if c.shutdown {
		err = errors.Errorf("dropping message %T, client shutting down", message)
		return
	}
	if c.conn == nil {
		return errors.New("no connection")
	}

	if subprotocol, ok := MessageSubprotocolMap[reflect.TypeOf(message)]; ok {
		message.SetSubprotocol(subprotocol)
	} else {
		return errors.Errorf("no subprotocol defined for message type %T", message)
	}

	// timestamp := GetCardanoTimestamp()
	// c.log.Debug().Msg("Cardano Timestamp: %d\n", timestamp)

	// encodedTimestamp := EncodeCardanoTimestamp(timestamp)
	// c.log.Debug().Msg("Encoded Timestamp: %v\n", encodedTimestamp)

	segment := &Segment{
		Timestamp: 0,
		Direction: DirectionOut,
	}

	err = segment.AddMessage(message)
	if err != nil {
		return
	}

	return c.SendSegment(segment)
}

func (c *Client) FetchTip() (tip PointAndBlockNum, err error) {
	err = c.SendMessage(&MessageFindIntersect{
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

	intersectNotFound, err := ReceiveNextOfType[*MessageIntersectNotFound](c)
	if err != nil {
		return
	}

	tip = intersectNotFound.Tip

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
	blocks, err := c.FetchRange(point, point)
	if err != nil {
		return
	}
	if len(blocks) != 1 {
		err = errors.Errorf("expected 1 block, got %d", len(blocks))
		return
	}
	block = blocks[0]
	return
}

func (c *Client) FetchRange(from, to Point) (blocks []*Block, err error) {
	done := make(chan struct{})
	var mu sync.Mutex

	cleanup := c.pubsub.On(func(i any) {
		message, ok := i.(Message)
		if !ok {
			return
		}
		switch msg := message.(type) {
		case *MessageStartBatch:
			return

		case *MessageBlock:
			var block *Block
			block, err = msg.Block()
			if err != nil {
				return
			}
			mu.Lock()
			block.Raw = msg.BlockData
			blocks = append(blocks, block)
			mu.Unlock()
			return

		case *MessageNoBlocks, *MessageBatchDone:
			close(done)
			return
		}
	})
	defer cleanup()

	err = c.SendMessage(&MessageRequestRange{
		From: from,
		To:   to,
	})
	if err != nil {
		return
	}

	select {
	case <-done:
		return
	case <-time.After(60 * time.Second): // TODO: can a batch take this long?
		err = errors.New("timeout waiting for range fetch to complete")
		return
	}
}

func (c *Client) ReceiveNext() (message Message, err error) {
	cbChan := make(chan Message, 1)
	var cbClosed bool
	cbClose := c.pubsub.On(func(i any) {
		msg, ok := i.(Message)
		if !ok {
			return
		}
		if cbClosed {
			return
		}
		cbClosed = true
		cbChan <- msg
		close(cbChan)
	})
	defer cbClose()

	select {
	case message = <-cbChan:
		return

	case <-time.After(time.Second * 5):
		err = errors.New("timeout while waiting for node response")
		return
	}

	return
}

func (c *Client) ReceiveNextOfType(message *Message) (err error) {
	cbChan := make(chan struct{})
	cbClose := c.pubsub.On(func(i any) {
		if reflect.TypeOf(*message) != reflect.TypeOf(i) {
			return
		}
		*message = i.(Message)
		cbChan <- struct{}{}
		close(cbChan)
	})
	defer cbClose()

	select {
	case <-cbChan:
		return

	case <-time.After(time.Second * 5):
		err = errors.Errorf("timeout while waiting for node response: %s", reflect.TypeOf(*message))
		return
	}

	return
}

func (c *Client) Subscribe(cb func(i any)) func() {
	return c.pubsub.On(cb)
}

func (c *Client) GetTip() PointAndBlockNum {
	return c.tip
}

func (c *Client) GetEra() Era {
	return c.latestEra
}

func ReceiveNextOfType[T Message](c *Client) (message T, err error) {
	var msg Message = *new(T)
	err = c.ReceiveNextOfType(&msg)
	if err != nil {
		return
	}

	if typed, ok := msg.(T); ok {
		message = typed
		return
	}

	err = errors.Errorf("expected next message of type %T, got %T", *new(T), msg)

	return
}
