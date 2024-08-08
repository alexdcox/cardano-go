package cardano

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"reflect"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

type ClientOptions struct {
	HostPort    string
	Network     Network
	TipOverride *Tip
	TipStore    TipStore
}

func (o *ClientOptions) setDefaults() {
	if o.HostPort == "" {
		o.HostPort = defaultClientOptions.HostPort
	}

	if o.Network == "" {
		o.Network = defaultClientOptions.Network
	}

	if o.TipStore == nil {
		o.TipStore = NewInMemoryTipStore()
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

	client = &Client{
		options: options,
		keepAliveCaller: NewPeriodicCaller(time.Second*4, func() {
			client.keepAlive()
		}),
		keepAliveMu: &sync.Mutex{},
		in:          NewMessageReader(),
		params:      params,
		log:         Log(),
		tipStore:    options.TipStore,
	}

	return
}

type Client struct {
	options         *ClientOptions
	conn            net.Conn
	keepAliveCaller *PeriodicCaller
	keepAliveMu     *sync.Mutex
	in              *MessageReader
	inStream        chan Message
	params          *NetworkParams
	log             *zerolog.Logger
	tipStore        TipStore
	tip             Tip
	shutdown        bool
	blockCallback   func(block *Block)
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

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go c.beginReadStream(wg)
	wg.Wait()

	err = c.handshake()
	if err != nil {
		return
	}

	c.keepAliveCaller.Start()

	if c.options.TipOverride != nil {
		c.tip = *c.options.TipOverride
	} else {
		c.tip, err = c.tipStore.GetTip()
		if err != nil {
			return
		}
	}

	go c.followChain()

	return
}

func (c *Client) followChain() {
	fmt.Println("FOLLLLLLLOW")

	lastTip := c.tip

	if c.tip.Point.Slot == 0 {
		c.log.Info().Msg("no previous or configured tip, following chain from node tip")

		err := c.SendMessage(&MessageFindIntersect{Points: []Point{{
			Slot: 0,
			Hash: make([]byte, 32),
		}}})
		if err != nil {
			c.log.Error().Msgf("follow chain failure: %+v", errors.WithStack(err))
			return
		}

		notFound, err := ReceiveNext[*MessageIntersectNotFound](c)
		if err != nil {
			c.log.Error().Msgf("follow chain failure: %+v", err)
			return
		}

		lastTip = notFound.Tip
	}

	err := c.SendMessage(&MessageFindIntersect{Points: []Point{lastTip.Point}})
	if err != nil {
		c.log.Error().Msgf("follow chain failure: %+v", errors.WithStack(err))
		return
	}

	intersect, err := ReceiveNext[*MessageIntersectFound](c)
	if err != nil {
		c.log.Error().Msgf("follow chain failure: %+v", err)
		return
	}

	c.log.Info().Msgf("got intersect %+v", intersect)

	for {
		err = c.SendMessage(&MessageRequestNext{})
		if err != nil {
			c.log.Error().Msgf("follow chain failure: %+v", errors.WithStack(err))
			return
		}

		next, err2 := c.ReceiveNext()
		if err2 != nil {
			err = err2
			return
		}

		var ok bool

		switch msg := next.(type) {
		case *MessageRollForward:
			fmt.Println("ROLL FORWARD")
			ok = true
			c.tip = msg.Tip
			c.broadcastNextBlock(c.tip)
		case *MessageRollBackward:
			fmt.Println("ROLL BACKWARD")
			ok = true
		case *MessageAwaitReply:
			fmt.Println("AWAIT REPLY")
			ok = true
		}

		if ok {
			if j, err := json.MarshalIndent(next, "", "  "); err == nil {
				fmt.Println(string(j))
			}
		} else {
			err = errors.Errorf("expected chain sync protocol message, got %T", next)
			// TODO: clean
			if err != nil {
				log.Fatal().Msgf("%+v", errors.WithStack(err))
			}
			return
		}
	}
}

// var DefaultMainnetTip = Tip{
// 	Point: Point{
// 		Slot: 127350361,
// 		Hash: HexString("cb1a4a043fa0e00bc02945358216357cf314fcf69fca3ca0320614a0691dcd62").Bytes(),
// 	},
// 	Block: 10471759,
// }

func (c *Client) OnBlock(cb func(block *Block)) {
	c.blockCallback = cb
}

func (c *Client) broadcastNextBlock(tip Tip) {
	if c.blockCallback == nil {
		return
	}
	block, err := c.FetchBlock(tip.Point)
	if err != nil {
		log.Error().Msgf("%+v", errors.WithStack(err))
		return
	}
	c.blockCallback(block)
}

func (c *Client) Stop() (err error) {
	if c.shutdown {
		return
	}
	c.shutdown = true
	c.log.Info().Msg("stopping client")

	c.keepAliveCaller.Stop()

	err = errors.WithStack(c.conn.Close())
	if err != nil {
		return
	}

	return errors.WithStack(c.tipStore.SetTip(c.tip))
}

func (c *Client) dial() (err error) {
	c.log.Info().Msgf("dialing node %s", c.options.HostPort)
	c.conn, err = net.Dial("tcp", c.options.HostPort)
	if err != nil {
		return errors.WithStack(err)
	}
	c.log.Info().Msg("connected to node")

	return
}

func (c *Client) beginReadStream(wg *sync.WaitGroup) {
	c.inStream = make(chan Message)
	wg.Done()

	defer close(c.inStream)
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
			c.log.Info().Msgf("message in: %T", message)
			c.inStream <- message
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

	acceptVersionMsg, err := ReceiveNextWithTimeout[*MessageAcceptVersion](c, time.Second*5)
	if err != nil {
		err = errors.Errorf(
			"handshake failed, received message %T, expected %T\n%+v",
			acceptVersionMsg,
			&MessageAcceptVersion{},
		)
		return
	}

	c.log.Info().Msgf("handshake ok, node accepted version %d", acceptVersionMsg.Version)

	return
}

func (c *Client) keepAlive() {
	c.keepAliveMu.Lock()
	defer c.keepAliveMu.Unlock()

	cookieBytes := make([]byte, 4)
	_, _ = rand.Read(cookieBytes)
	cookie := binary.BigEndian.Uint16(cookieBytes)

	err := c.SendMessage(&MessageKeepAliveResponse{Cookie: cookie})
	if err != nil {
		c.log.Error().Msgf("failed to send keep alive message: %+v", err)
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
	if segment.Protocol != ProtocolKeepAlive {
		c.keepAliveMu.Lock()
		defer c.keepAliveMu.Unlock()
	}

	writeBytes, err := segment.MarshalDataItem()
	if err != nil {
		return errors.WithStack(err)
	}

	for _, msg := range segment.messages {
		c.log.Info().Msgf(
			"message out: %T, protocol: %d, subprotocol: %d",
			msg,
			segment.Protocol,
			msg.GetSubprotocol())
	}
	c.log.Debug().Msgf("write: %x", writeBytes)

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
	// fmt.Printf("Cardano Timestamp: %d\n", timestamp)

	// encodedTimestamp := EncodeCardanoTimestamp(timestamp)
	// fmt.Printf("Encoded Timestamp: %v\n", encodedTimestamp)

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

func (c *Client) FetchTip() (tip Tip, err error) {
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

	intersectNotFound, err := ReceiveNext[*MessageIntersectNotFound](c)
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
	err = c.SendMessage(&MessageRequestRange{
		From: from,
		To:   to,
	})
	if err != nil {
		return
	}

	for {
		var next Message
		next, err = c.ReceiveNext()
		if err != nil {
			return
		}

		switch msg := next.(type) {
		case *MessageStartBatch:
			continue

		case *MessageBlock:
			var block *Block
			block, err = msg.Block()
			if err != nil {
				return
			}
			blocks = append(blocks, block)
			continue

		case *MessageNoBlocks, *MessageBatchDone:
			return
		}

		err = errors.Errorf("expected block fetch protocol message, got %T", next)
		return
	}
}

func (c *Client) ReceiveNext() (message Message, err error) {
	message, ok := <-c.inStream
	if !ok {
		err = errors.New("message stream closed")
		return
	}
	if _, isKeepAlive := message.(*MessageKeepAlive); isKeepAlive {
		// we ignore incoming keep alive requests because we never close the connection
		return c.ReceiveNext()
	}
	return
}

func (c *Client) ReceiveNextWithTimeout(timeout time.Duration) (message any, err error) {
	select {
	case message = <-c.inStream:
		return

	case <-time.After(timeout):
		err = errors.New("timeout while waiting for node response")
		return
	}
}

func ReceiveNext[T Message](c *Client) (message T, err error) {
	m, err := c.ReceiveNext()
	if err != nil {
		return
	}

	if typed, ok := m.(T); ok {
		message = typed
		return
	}

	err = errors.Errorf("expected next message of type %T, got %T", *new(T), m)
	return
}

func ReceiveNextWithTimeout[T Message](c *Client, timeout time.Duration) (message T, err error) {
	m, err := c.ReceiveNextWithTimeout(timeout)
	if err != nil {
		return
	}

	if typed, ok := m.(T); ok {
		message = typed
		return
	}

	err = errors.Errorf("expected next message of type %T, got %T", *new(T), m)
	return
}
