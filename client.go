package cardano

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
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
		in:                NewMessageReader(),
		params:            params,
		log:               Log(),
		tipStore:          options.TipStore,
		blockCallbackMu:   &sync.Mutex{},
		messageCallbackMu: &sync.Mutex{},
	}

	return
}

type Client struct {
	options           *ClientOptions
	conn              net.Conn
	keepAliveCaller   *PeriodicCaller
	in                *MessageReader
	params            *NetworkParams
	log               *zerolog.Logger
	tip               Tip
	tipStore          TipStore
	shutdown          bool
	blockCallbacks    []*func(block *Block)
	blockCallbackMu   *sync.Mutex
	messageCallbacks  []*func(message Message)
	messageCallbackMu *sync.Mutex
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

		notFound, err := ReceiveNextOfType[*MessageIntersectNotFound](c)
		if err != nil {
			c.log.Error().Msgf("follow chain failure: %+v", err)
			return
		}

		lastTip = notFound.Tip
	} else {
		c.log.Info().Msg("following chain from previous tip")
	}

	err := c.SendMessage(&MessageFindIntersect{Points: []Point{lastTip.Point}})
	if err != nil {
		c.log.Error().Msgf("follow chain failure: %+v", errors.WithStack(err))
		return
	}

	intersect, err := ReceiveNextOfType[*MessageIntersectFound](c)
	if err != nil {
		c.log.Error().Msgf("follow chain failure: %+v", err)
		return
	}

	c.log.Info().Msgf("new tip: %s", intersect.Tip)

	requestNext := func() (err error) {
		err = c.SendMessage(&MessageRequestNext{})
		if err != nil {
			c.log.Error().Msgf("follow chain failure: %+v", errors.WithStack(err))
			return
		}
		return
	}

	requestNext()

	fetchAndBroadcast := func(point Point) {
		block, err2 := c.FetchBlock(c.tip.Point)
		if err2 != nil {
			log.Error().Msgf("failed to fetch block post roll forward: %+v", errors.WithStack(err2))
			return
		}
		c.broadcastNextBlock(block)
	}

	c.OnMessage(func(message Message) {
		var logMessage bool

		switch msg := message.(type) {
		case *MessageRollForward:
			logMessage = true
			c.tip = msg.Tip
			go fetchAndBroadcast(msg.Tip.Point)
			requestNext()
		case *MessageRollBackward:
			logMessage = true
			c.tip = msg.Tip
			go fetchAndBroadcast(msg.Tip.Point)
			requestNext()
		case *MessageAwaitReply:
		}

		if logMessage {
			if j, err := json.MarshalIndent(message, "", "  "); err == nil {
				c.log.Debug().Msgf("%T -> %s", message, string(j))
			}
		}
	})
}

// var DefaultMainnetTip = Tip{
// 	Point: Point{
// 		Slot: 127350361,
// 		Hash: HexString("cb1a4a043fa0e00bc02945358216357cf314fcf69fca3ca0320614a0691dcd62").Bytes(),
// 	},
// 	Block: 10471759,
// }

func (c *Client) OnBlock(callback func(msg *Block)) func() {
	c.blockCallbackMu.Lock()
	defer c.blockCallbackMu.Unlock()

	c.blockCallbacks = append(c.blockCallbacks, &callback)

	return func() {
		c.blockCallbackMu.Lock()
		defer c.blockCallbackMu.Unlock()

		for i, cb := range c.blockCallbacks {
			if cb == &callback {
				lastIndex := len(c.blockCallbacks) - 1
				c.blockCallbacks[i] = c.blockCallbacks[lastIndex]
				c.blockCallbacks = c.blockCallbacks[:lastIndex]
				break
			}
		}
	}
}

func (c *Client) broadcastNextBlock(block *Block) {
	c.log.Info().Msgf("block in: %T", block)
	c.blockCallbackMu.Lock()
	callbacks := make([]*func(*Block), len(c.blockCallbacks))
	copy(callbacks, c.blockCallbacks)
	c.blockCallbackMu.Unlock()

	c.log.Debug().Msgf("BROADCAST BLOCK: %d\n", block.Data.Header.Body.Number)

	c.log.Debug().Msg("-------------------------")
	for _, cb := range callbacks {
		(*cb)(block)
	}
	c.log.Debug().Msg("+++++++++++++++++++++++++")
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
	// c.inStream = make(chan Message)
	wg.Done()

	// defer close(c.inStream)
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
			c.broadcastNextMessage(message)
		}
	}
}

func (c *Client) broadcastNextMessage(message Message) {
	protocol, _ := MessageProtocolMap[reflect.TypeOf(message)]
	if protocol == ProtocolKeepAlive {
		// we ignore incoming keep alive requests because we never close the connection
		return
	}
	c.log.Info().Msgf("message in: %T", message)
	c.messageCallbackMu.Lock()
	callbacks := make([]*func(Message), len(c.messageCallbacks))
	copy(callbacks, c.messageCallbacks)
	c.messageCallbackMu.Unlock()

	c.log.Debug().Msg("-------------------------")
	for _, cb := range callbacks {
		(*cb)(message)
	}
	c.log.Debug().Msg("+++++++++++++++++++++++++")
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

func (c *Client) keepAlive() {
	// c.keepAliveMu.Lock()
	// defer c.keepAliveMu.Unlock()

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
		// c.keepAliveMu.Lock()
		// defer c.keepAliveMu.Unlock()
	}

	writeBytes, err := segment.MarshalDataItem()
	if err != nil {
		return errors.WithStack(err)
	}

	for _, msg := range segment.messages {
		if segment.Protocol != ProtocolKeepAlive {
			c.log.Info().Msgf(
				"message out: %T, protocol: %d, subprotocol: %d",
				msg,
				segment.Protocol,
				msg.GetSubprotocol())
		}
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
	ready := make(chan struct{})
	readyClosed := false
	done := make(chan struct{})
	var mu sync.Mutex

	cleanup := c.OnMessage(func(message Message) {
		if !readyClosed {
			readyClosed = true
			close(ready)
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
			blocks = append(blocks, block)
			mu.Unlock()
			return

		case *MessageNoBlocks, *MessageBatchDone:
			close(done)
			return
		}
	})
	defer cleanup()

	<-ready

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

func (c *Client) OnMessage(callback func(msg Message)) func() {
	c.messageCallbackMu.Lock()
	defer c.messageCallbackMu.Unlock()

	c.messageCallbacks = append(c.messageCallbacks, &callback)

	return func() {
		c.messageCallbackMu.Lock()
		defer c.messageCallbackMu.Unlock()

		for i, cb := range c.messageCallbacks {
			if cb == &callback {
				lastIndex := len(c.messageCallbacks) - 1
				c.messageCallbacks[i] = c.messageCallbacks[lastIndex]
				c.messageCallbacks = c.messageCallbacks[:lastIndex]
				break
			}
		}
	}
}

func (c *Client) ReceiveNext() (message Message, err error) {
	cbChan := make(chan Message, 1)
	var cbClosed bool
	cbClose := c.OnMessage(func(msg Message) {
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
	var cbClose func()
	cbClose = c.OnMessage(func(msg Message) {
		if reflect.TypeOf(*message) != reflect.TypeOf(msg) {
			return
		}
		*message = msg
		cbChan <- struct{}{}
		cbClose()
		close(cbChan)
	})

	select {
	case <-cbChan:
		return

	case <-time.After(time.Second * 5):
		err = errors.New("timeout while waiting for node response")
		return
	}

	return
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
