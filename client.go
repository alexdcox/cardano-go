package cardano

import (
	"database/sql"
	"fmt"
	"net"
	"sync"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/ledger"
	common2 "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	"github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

type ClientOptions struct {
	NtNHostPort string
	NtCHostPort string
	Network     Network
	Database    Database
	StartPoint  PointRef
	// DisableFollowChain bool
	LogLevel    zerolog.Level
	ReorgWindow int
}

func (o *ClientOptions) setDefaults() {
	if o.NtNHostPort == "" {
		o.NtNHostPort = defaultClientOptions.NtNHostPort
	}

	if o.NtCHostPort == "" {
		o.NtCHostPort = defaultClientOptions.NtCHostPort
	}

	if o.Network == "" {
		o.Network = defaultClientOptions.Network
	}

	if len(o.StartPoint.Hash) == 0 {
		o.StartPoint = PointRef{}
	}

	if o.ReorgWindow < 0 {
		o.ReorgWindow = defaultClientOptions.ReorgWindow
	}
}

var defaultClientOptions = &ClientOptions{
	NtNHostPort: "localhost:3000",
	NtCHostPort: "localhost:3001",
	Network:     NetworkMainNet,
	ReorgWindow: 6,
}

func NewClient(options *ClientOptions) (client *Client, err error) {
	if options == nil {
		options = &ClientOptions{}
	}
	options.setDefaults()

	if options.Database == nil {
		err = errors.New("database is required")
		return
	}

	params, err := options.Network.Params()
	if err != nil {
		return
	}

	clientLog := LogAtLevel(options.LogLevel)

	client = &Client{
		Options:    options,
		params:     params,
		log:        clientLog,
		db:         options.Database,
		shutdownWg: &sync.WaitGroup{},
	}

	return
}

type Client struct {
	Options    *ClientOptions
	params     *NetworkParams
	log        *zerolog.Logger
	db         Database
	shutdown   bool
	shutdownWg *sync.WaitGroup
	latestEra  Era
	errChan    chan error
	ntn        *ouroboros.Connection
	// ntc        *ouroboros.Connection
	processor  *BlockProcessor
	startPoint PointRef
	points     []PointRef
	tipReached bool
}

func (c *Client) Start() (err error) {
	c.log.Info().Msg("starting client")

	c.errChan = make(chan error, 1)

	c.processor, err = NewBlockProcessor(c.db, c.log, c.Options.ReorgWindow)
	if err != nil {
		return
	}

	if err = c.connect(); err != nil {
		return
	}

	c.processor.ntn = c.ntn

	go c.followChain()

	return
}

func (c *Client) connect() error {
	opt := c.Options
	maxAttempts := 10
	retryDelay := time.Second

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		c.log.Info().
			Str("host", opt.NtNHostPort).
			Int("attempt", attempt).
			Int("maxAttempts", maxAttempts).
			Msg("attempting to dial node-to-node")

		ntnConn, err := net.Dial("tcp", opt.NtNHostPort)
		if err != nil {
			if attempt == maxAttempts {
				c.log.Error().
					Err(err).
					Str("host", opt.NtNHostPort).
					Msg("failed to connect after all attempts")
				return errors.WithStack(err)
			}

			c.log.Warn().
				Err(err).
				Str("host", opt.NtNHostPort).
				Int("attempt", attempt).
				Int("nextAttempt", attempt+1).
				Dur("retryDelay", retryDelay).
				Msg("connection attempt failed, retrying after delay")

			time.Sleep(retryDelay)
			continue
		}

		c.log.Info().
			Int("attempt", attempt).
			Msg("successfully established TCP connection")

		var ouroborosErr error
		c.ntn, ouroborosErr = ouroboros.New(
			ouroboros.WithConnection(ntnConn),
			ouroboros.WithNetworkMagic(uint32(c.params.Magic)),
			ouroboros.WithNodeToNode(true),
			ouroboros.WithKeepAlive(true),
			ouroboros.WithBlockFetchConfig(blockfetch.Config{
				BlockFunc:     c.processor.onBlock,
				BatchDoneFunc: c.processor.onBatchDone,
			}),
			ouroboros.WithChainSyncConfig(chainsync.Config{
				RollBackwardFunc: c.onRollBackwards,
				RollForwardFunc:  c.onRollForwards,
			}),
		)
		if ouroborosErr != nil {
			ntnConn.Close() // Clean up the connection if Ouroboros initialization fails
			c.log.Error().
				Err(ouroborosErr).
				Msg("failed to initialize Ouroboros client")
			return errors.WithStack(ouroborosErr)
		}

		c.log.Info().
			Str("host", opt.NtNHostPort).
			Msg("successfully connected to node")

		return nil
	}

	return errors.New("failed to open node-to-node connection")
}

func (c *Client) ntc() (ntc *ouroboros.Connection, err error) {
	opt := c.Options

	c.log.Debug().Msgf("dialing node-to-client: '%s'", opt.NtCHostPort)

	ntcConn, err := net.Dial("tcp", opt.NtCHostPort)
	if err != nil {
		err = errors.WithStack(err)
		return
	}

	ntc, err = ouroboros.New(
		ouroboros.WithConnection(ntcConn),
		ouroboros.WithNetworkMagic(uint32(c.params.Magic)),
		ouroboros.WithKeepAlive(true),
	)
	if err != nil {
		err = errors.WithStack(err)
		return
	}

	return
}

func (c *Client) onRollBackwards(context chainsync.CallbackContext, point common.Point, tip chainsync.Tip) error {
	if point.Slot > c.startPoint.Slot {
		c.log.Info().Msgf(
			"roll backwards: slot: %d | hash: %x",
			point.Slot,
			point.Hash,
		)

		if err := c.processor.HandleRollback(point.Slot); err != nil {
			return errors.Wrap(err, "failed to handle rollback")
		}
	}

	return nil
}

func (c *Client) onRollForwards(context chainsync.CallbackContext, blockType uint, blockData any, tip chainsync.Tip) error {
	var point PointRef

	c.latestEra = Era(blockType)

	switch v := blockData.(type) {
	case ledger.Block:
		point = PointRef{
			Slot: v.SlotNumber(),
			Hash: v.Hash(),
			Type: int(blockType),
		}
	case ledger.BlockHeader:
		point = PointRef{
			Slot: v.SlotNumber(),
			Hash: v.Hash(),
			Type: int(blockType),
		}
	}

	// Check for chain splits
	if err := c.processor.DetectChainSplit(point.Slot); err != nil {
		c.log.Error().Err(err).Msg("failed to detect chain split")
	}

	c.points = append(c.points, point)

	if !c.tipReached && tip.Point.Slot == point.Slot {
		c.tipReached = true
		c.log.Info().Msg("node tip reached")
	}

	if len(c.points) >= BlockBatchSize || c.tipReached {
		if err := c.db.AddPoints(c.points); err != nil {
			c.log.Fatal().Msgf("failed to add points: %+v", err)
		}

		c.processor.TryProcessBlocks()
		c.points = nil
	}

	return nil
}

func (c *Client) followChain() {
	var hasStartPoint bool
	startPoint, err := c.db.GetHighestPoint()
	if err == nil {
		hasStartPoint = true
	} else if !errors.Is(err, sql.ErrNoRows) {
		log.Fatal().Msgf("failed to read db for start point: %+v", err)
	}

	syncStart := []common.Point{}
	if hasStartPoint {
		c.log.Info().
			Uint64("height", startPoint.Height).
			Uint64("slot", startPoint.Slot).
			Str("hash", startPoint.Hash).
			Msgf("syncing from previous point")
		syncStart = append(syncStart, common.NewPoint(
			startPoint.Slot,
			HexString(startPoint.Hash).Bytes(),
		))
	} else {
		c.log.Info().Msg("syncing from genesis block")
	}

	if err := c.ntn.ChainSync().Client.Sync(syncStart); err != nil {
		c.log.Fatal().Err(err).Msg("failed to start chain sync")
		return
	}
}

func (c *Client) Stop() (err error) {
	if c.shutdown {
		c.log.Warn().Msg("client shutdown already in progress...")
		return
	}
	c.shutdown = true

	c.log.Info().Msg("closing node connection")

	if err = errors.WithStack(c.ntn.Close()); err != nil {
		return
	}

	return
}

func (c *Client) GetTip() (point PointRef, err error) {
	point, err = c.db.GetHighestPoint()
	if errors.Is(err, sql.ErrNoRows) {
		// Return an empty point if we don't have a tip yet
		err = nil
	}
	return
}

type UtxoWithHeight struct {
	TxHash  string `json:"txHash"`
	Address string `json:"address"`
	Amount  uint64 `json:"amount"`
	Index   uint64 `json:"index"`
	Height  uint64 `json:"height"`
}

func (c *Client) GetUtxoForAddress(address string) (utxos []UtxoWithHeight, err error) {
	ntc, err := c.ntc()
	if err != nil {
		return
	}

	ledgerAddress, err := ledger.NewAddress(address)
	if err != nil {
		err = errors.WithStack(err)
		return
	}

	utxosRsp, err := ntc.LocalStateQuery().Client.GetUTxOByAddress([]ledger.Address{ledgerAddress})
	if err != nil {
		err = errors.WithStack(err)
		return
	}

	// Filter out utxos from transactions the node is aware of that we haven't processed yet.

	txHashesMap := map[string]struct{}{}
	for id, _ := range utxosRsp.Results {
		txHashesMap[id.Hash.String()] = struct{}{}
	}
	var txHashesSlice []string
	for txHash, _ := range txHashesMap {
		txHashesSlice = append(txHashesSlice, txHash)
	}

	recordedTxs, err := c.db.GetTxs(txHashesSlice)
	if err != nil {
		return
	}
	recordedTxsMap := make(map[string]TxRef)
	for _, tx := range recordedTxs {
		recordedTxsMap[tx.Hash] = tx
	}

	for id, utxo := range utxosRsp.Results {
		if ref, exists := recordedTxsMap[id.Hash.String()]; exists {
			utxos = append(utxos, UtxoWithHeight{
				TxHash:  id.Hash.String(),
				Address: utxo.Address().String(),
				Amount:  utxo.Amount(),
				Index:   uint64(id.Idx),
				Height:  ref.BlockHeight,
			})
		}
	}

	fmt.Printf("node saw %d utxos | we see %d utxos\n", len(utxosRsp.Results), len(utxos))

	return
}

func (c *Client) GetProtocolParams() (params common2.ProtocolParameters, err error) {
	ntc, err := c.ntc()
	if err != nil {
		return
	}

	params, err = ntc.LocalStateQuery().Client.GetCurrentProtocolParams()
	if err != nil {
		err = errors.WithStack(err)
		return
	}
	return
}

func (c *Client) FetchLatestBlock() (block ledger.Block, err error) {
	point, err := c.db.GetHighestPoint()
	if err != nil {
		return
	}

	block, err = c.ntn.BlockFetch().Client.GetBlock(point.Common())
	err = errors.WithStack(err)

	return
}

func (c *Client) GetBlockByPoint(point PointRef) (block ledger.Block, err error) {
	block, err = c.ntn.BlockFetch().Client.GetBlock(point.Common())
	err = errors.WithStack(err)
	return
}

func (c *Client) GetBlockByHeight(height uint64) (block ledger.Block, err error) {
	point, err := c.db.GetPointByHeight(height)
	if err != nil {
		return
	}
	block, err = c.ntn.BlockFetch().Client.GetBlock(point.Common())
	err = errors.WithStack(err)
	return
}

func (c *Client) GetTransaction(hash string) (tx ledger.Transaction, block ledger.Block, err error) {
	height, err := c.db.GetBlockNumForTx(hash)
	if err != nil {
		return
	}
	point, err := c.db.GetPointByHeight(height)
	if err != nil {
		return
	}
	block, err = c.ntn.BlockFetch().Client.GetBlock(point.Common())
	if err != nil {
		return
	}
	for _, blockTx := range block.Transactions() {
		if blockTx.Hash() == hash {
			tx = blockTx
		}
	}
	if tx == nil {
		err = errors.WithStack(ErrTransactionNotFound)
	}
	return
}

func (c *Client) SubmitTx(signedTx []byte) (err error) {
	ntc, err := c.ntc()
	if err != nil {
		return
	}

	return errors.WithStack(ntc.LocalTxSubmission().Client.SubmitTx(uint16(EraConway)-1, signedTx))
}

func (c *Client) GetHeight() (height PointRef, err error) {
	if c.processor.highestPoint.Hash == "" {
		err = errors.WithStack(ErrPointNotFound)
		return
	}
	return c.processor.highestPoint, nil
}
