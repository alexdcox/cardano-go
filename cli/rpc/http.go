package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	. "github.com/alexdcox/cardano-go"
	"github.com/alexdcox/cardano-go/rpcclient"
	"github.com/fxamacker/cbor/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/tidwall/gjson"
)

func NewHttpRpcServer(config *_config, chunkReader *ChunkReader, db Database, followClient *Client) (server *HttpRpcServer, err error) {
	fetchClient, err := NewClient(&ClientOptions{
		HostPort:           config.NodeHostPort,
		Network:            Network(config.Network),
		DisableFollowChain: true,
		LogLevel:           zerolog.InfoLevel,
	})
	if err != nil {
		return
	}

	server = &HttpRpcServer{
		config:       config,
		followClient: followClient,
		fetchClient:  fetchClient,
		chunkReader:  chunkReader,
		db:           db,
	}

	return
}

type HttpRpcServer struct {
	app          *fiber.App
	followClient *Client
	fetchClient  *Client
	chunkReader  *ChunkReader
	config       *_config
	db           Database
}

func (s *HttpRpcServer) Start() (err error) {
	err = s.fetchClient.Start()
	if err != nil {
		return
	}

	s.app = fiber.New(fiber.Config{
		ReadTimeout:           30 * time.Second,
		WriteTimeout:          30 * time.Second,
		IdleTimeout:           120 * time.Second,
		DisableStartupMessage: true,
	})
	s.app.Use(recover.New())
	s.app.Use(func(c *fiber.Ctx) error {
		rsp := c.Next()
		log.Info().Msgf("http response: [%d] %s - %s %s", c.Response().StatusCode(), c.IP(), c.Method(), c.Path())
		return rsp
	})

	s.app.Get("/utxo/:address", s.getUtxoForAddress)
	s.app.Get("/block/latest", s.getLatestBlock)
	s.app.Get("/block/:number", s.getBlock)
	s.app.Post("/broadcast", s.postBroadcast)
	s.app.Get("/status", s.getStatus)
	s.app.Get("/tip", s.getTip)
	s.app.Post("/cli", s.postCli)

	log.Info().Msgf("server listening on %s", config.RpcHostPort)

	err = errors.WithStack(s.app.Listen(config.RpcHostPort))

	return
}

func (s *HttpRpcServer) Stop() (err error) {
	if err = s.app.Shutdown(); err != nil {
		return err
	}
	return s.fetchClient.Stop()
}

func (s *HttpRpcServer) errorStringResponse(c *fiber.Ctx, error string) error {
	return c.Status(http.StatusInternalServerError).JSON(map[string]any{"error": error})
}

func (s *HttpRpcServer) errorResponse(c *fiber.Ctx, err error) error {
	return s.errorStringResponse(c, fmt.Sprintf("%+v", err))
}

func (s *HttpRpcServer) getStatus(c *fiber.Ctx) error {
	firstChunkedBlock, lastChunkedBlock, err := s.db.GetChunkedBlockSpan()
	if err != nil {
		return s.errorResponse(c, err)
	}

	firstBlockPoint, lastBlockPoint, err := s.db.GetPointSpan()
	if err != nil {
		return s.errorResponse(c, err)
	}

	return c.JSON(map[string]any{
		"chunks": map[string]any{
			"firstBlock": firstChunkedBlock,
			"lastBlock":  lastChunkedBlock,
		},
		"client": map[string]any{
			"firstBlock": firstBlockPoint,
			"lastBlock":  lastBlockPoint,
			"tip":        s.followClient.GetTip(),
		},
		"config": s.config,
	})
}

func (s *HttpRpcServer) getUtxoForAddress(c *fiber.Ctx) error {
	addr := &Address{}
	if err := addr.ParseBech32String(c.Params("address"), Network(config.Network)); err != nil {
		return s.errorResponse(c, err)
	}

	output, err := runCommandWithVirtualFiles(
		"/usr/local/bin/cardano-cli",
		"query utxo --out-file /dev/stdout --address "+c.Params("address"),
		[]string{
			fmt.Sprintf("CARDANO_NODE_NETWORK_ID=%d", config.Cardano.ShelleyConfig.NetworkMagic),
			"CARDANO_NODE_SOCKET_PATH=" + config.SocketPath,
		},
		[]VirtualFile{},
	)

	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(map[string]any{
			"output": string(output),
			"error":  fmt.Sprintf("%+v", err),
		})
	}

	jsn := gjson.ParseBytes(output)
	if !jsn.IsObject() {
		return c.Status(http.StatusOK).SendString(string(output))
	}

	formattedOutout := []any{}
	jsn.ForEach(func(key, value gjson.Result) bool {
		formattedOutout = append(formattedOutout, rpcclient.Utxo{
			TxHash:  key.String(),
			Address: value.Get("address").String(),
			Value:   value.Get("value.lovelace").Uint(),
		})
		return true
	})

	return c.JSON(formattedOutout)
}

func (s *HttpRpcServer) getLatestBlock(c *fiber.Ctx) error {
	block, err := s.fetchClient.FetchLatestBlock()
	if err != nil {
		return s.errorResponse(c, err)
	}

	return c.JSON(block)
}

func (s *HttpRpcServer) getBlock(c *fiber.Ctx) error {
	blockNumberStr := c.Params("number")
	blockNumber, err := strconv.Atoi(blockNumberStr)
	if err != nil {
		return s.errorResponse(c, err)
	}

	returnBlock := func(block *Block) error {
		if c.Get("Content-Type") == "application/hex" {
			blockBytes, _ := cbor.Marshal(block)
			return c.SendString(fmt.Sprintf("%x", blockBytes))
		}
		if c.Get("Content-Type") == "application/cbor" {
			return c.Send(block.Raw)
		}
		return c.JSON(block)
	}

	// Try to fetch from the node chunk directory first

	if block, err2 := s.chunkReader.GetBlock(uint64(blockNumber)); err2 == nil {
		return returnBlock(block)
	}

	// If not yet written to disk, fetch the block from the node

	if point, err2 := s.db.GetBlockPoint(uint64(blockNumber)); err2 == nil {
		if block, err3 := s.fetchClient.FetchBlock(point.Point); err3 == nil {
			return returnBlock(block)
		}
	}

	return s.errorResponse(c, errors.WithStack(ErrBlockNotFound))
}

func (s *HttpRpcServer) getTip(c *fiber.Ctx) error {
	return c.JSON(s.followClient.GetTip())
}

func (s *HttpRpcServer) postBroadcast(c *fiber.Ctx) error {
	var req rpcclient.BroadcastRawTxIn
	if err := s.unmarshalJson(c, &req); err != nil {
		return err
	}

	txFile, _ := json.Marshal(map[string]any{
		"type":        "Witnessed Tx BabbageEra",
		"description": "Ledger Cddl Format",
		"cborHex":     req.Tx,
	})

	output, err := runCommandWithVirtualFiles(
		"/usr/local/bin/cardano-cli",
		"transaction submit --cardano-mode --tx-file file://tx --out-file /dev/stdout",
		[]string{
			fmt.Sprintf("CARDANO_NODE_NETWORK_ID=%d", config.Cardano.ShelleyConfig.NetworkMagic),
			"CARDANO_NODE_SOCKET_PATH=" + config.SocketPath,
		},
		[]VirtualFile{
			{
				Name:    "tx",
				Content: txFile,
			},
		},
	)

	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(map[string]any{
			"output": string(output),
			"error":  fmt.Sprintf("%+v", err),
		})
	}

	var mime = "text/plain"
	if gjson.ParseBytes(output).IsObject() {
		mime = "application/json"
	}

	c.Type(mime)

	return c.Send(output)
}

func (s *HttpRpcServer) unmarshalJson(c *fiber.Ctx, target any) (err error) {
	if c.Get("Content-Type") != "application/json" {
		return c.SendStatus(http.StatusBadRequest)
	}

	return c.BodyParser(target)
}

func (s *HttpRpcServer) postCli(c *fiber.Ctx) error {
	var data InputData
	if err := s.unmarshalJson(c, &data); err != nil {
		return s.errorResponse(c, err)
	}

	virtualFiles := []VirtualFile{}
	for filename, content := range data.Files {
		virtualFiles = append(virtualFiles, VirtualFile{Name: filename, Content: []byte(content)})
	}

	output, err := runCommandWithVirtualFiles(
		"/usr/local/bin/cardano-cli",
		data.Method+" "+data.Params,
		[]string{
			fmt.Sprintf("CARDANO_NODE_NETWORK_ID=%d", config.Cardano.ShelleyConfig.NetworkMagic),
			"CARDANO_NODE_SOCKET_PATH=" + config.SocketPath,
		},
		virtualFiles,
	)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(map[string]any{
			"output": string(output),
			"error":  fmt.Sprintf("%+v", err),
		})
	}

	var mime = "text/plain"
	if gjson.ParseBytes(output).IsObject() {
		mime = "application/json"
	}

	c.Type(mime)

	return c.Send(output)
}
