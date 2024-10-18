package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
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

const (
	CardanoBinaryPath = "/usr/local/bin/cardano-cli"
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
	s.app.Get("/block/:numberorhash", s.getBlock)
	s.app.Get("/tx/:hash", s.getTransaction)
	s.app.Get("/tx/:hash/block", s.getTransactionBlock)
	s.app.Post("/broadcast", s.postBroadcast)
	s.app.Get("/status", s.getStatus)
	s.app.Get("/tip", s.getTip)
	s.app.Post("/cli", s.postCli)

	log.Info().Msgf("http/rpc server listening on %s", config.RpcHostPort)

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
	startChunkedBlock, endChunkedBlock, err := s.db.GetChunkedBlockSpan()
	if err != nil {
		return s.errorResponse(c, err)
	}

	startBlockPoint, endBlockPoint, err := s.db.GetPointSpan()
	if err != nil {
		return s.errorResponse(c, err)
	}

	fileInfo, err := os.Stat(s.config.DatabasePath)
	if err != nil {
		return s.errorResponse(c, err)
	}

	sizeInBytes := fileInfo.Size()
	sizeInMB := float64(sizeInBytes) / 1024 / 1024

	fees := map[string]any{}
	if protocolParamOut, err := s.runCardanoBinary("query protocol-parameters"); err == nil {
		protocolParamJson := gjson.ParseBytes(protocolParamOut)
		fees["txFeeFixed"] = protocolParamJson.Get("txFeeFixed").Uint()
		fees["txFeePerByte"] = protocolParamJson.Get("txFeePerByte").Uint()
		fees["utxoCostPerByte"] = protocolParamJson.Get("utxoCostPerByte").Uint()
	}

	nodeOut, err := s.runCardanoBinary("query tip")
	if err != nil {
		return s.errorResponse(c, err)
	}
	var node any
	err = json.Unmarshal(nodeOut, &node)
	if err != nil {
		return s.errorResponse(c, err)
	}

	return c.JSON(map[string]any{
		"db": map[string]any{
			"sizeMB": fmt.Sprintf("%.2f MB", sizeInMB),
		},
		"chunks": map[string]any{
			"startBlock": startChunkedBlock,
			"endBlock":   endChunkedBlock,
		},
		"client": map[string]any{
			"startBlock": startBlockPoint,
			"endBlock":   endBlockPoint,
			"tip":        s.followClient.GetTip(),
			"era":        s.followClient.GetEra(),
		},
		"config": s.config,
		"fees":   fees,
		"node":   node,
	})
}

func (s *HttpRpcServer) getUtxoForAddress(c *fiber.Ctx) error {
	addr := &Address{}
	if err := addr.ParseBech32String(c.Params("address"), Network(config.Network)); err != nil {
		return s.errorResponse(c, err)
	}

	output, err := s.runCardanoBinary("query utxo --out-file /dev/stdout --address " + c.Params("address"))
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
		hash := key.String()
		height, _ := s.db.GetBlockNumForTx(hash)

		formattedOutout = append(formattedOutout, rpcclient.Utxo{
			TxHash:  hash,
			Address: value.Get("address").String(),
			Value:   value.Get("value.lovelace").Uint(),
			Height:  height,
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

func (s *HttpRpcServer) returnBlock(c *fiber.Ctx, block *Block) error {
	if c.Get("Accept") == "application/hex" {
		blockBytes, _ := cbor.Marshal(block)
		return c.SendString(fmt.Sprintf("%x", blockBytes))
	}
	if c.Get("Accept") == "application/cbor" {
		return c.Send(block.Raw)
	}
	return c.JSON(block)
}

func (s *HttpRpcServer) getBlock(c *fiber.Ctx) error {
	param := c.Params("numberorhash")

	if len(param) == 64 {
		return s.getBlockByHash(c, param)
	} else {
		return s.getBlockByNumber(c, param)
	}
}

func (s *HttpRpcServer) getBlockByHash(c *fiber.Ctx, hash string) error {
	blockNumber, err := s.db.GetBlockNumForHash(hash)
	if err != nil {
		return s.errorResponse(c, err)
	}

	if point, err2 := s.db.GetBlockPoint(blockNumber); err2 == nil {
		if block, err3 := s.fetchClient.FetchBlock(point.Point); err3 == nil {
			return s.returnBlock(c, block)
		}
	}

	return s.errorResponse(c, errors.WithStack(ErrBlockNotFound))
}

func (s *HttpRpcServer) getBlockByNumber(c *fiber.Ctx, number string) error {
	blockNumber, err := strconv.Atoi(number)
	if err != nil {
		return s.errorResponse(c, err)
	}

	// Try to fetch from the node chunk directory first

	if block, err2 := s.chunkReader.GetBlock(uint64(blockNumber)); err2 == nil {
		return s.returnBlock(c, block)
	}

	// If not yet written to disk, fetch the block from the node

	if point, err2 := s.db.GetBlockPoint(uint64(blockNumber)); err2 == nil {
		if block, err3 := s.fetchClient.FetchBlock(point.Point); err3 == nil {
			return s.returnBlock(c, block)
		}
	}

	return s.errorResponse(c, errors.WithStack(ErrBlockNotFound))
}

func (s *HttpRpcServer) getTransaction(c *fiber.Ctx) error {
	transactionHash := c.Params("hash")
	blockNumber, err := s.db.GetBlockNumForTx(transactionHash)
	if err != nil {
		return s.errorResponse(c, err)
	}

	if point, err2 := s.db.GetBlockPoint(blockNumber); err2 == nil {
		if block, err3 := s.fetchClient.FetchBlock(point.Point); err3 == nil {
			for _, tx := range block.Data.TransactionBodies {
				hash, _ := tx.Hash()
				if hash.String() == transactionHash {
					return c.JSON(tx)
				}
			}
		}
	}

	return s.errorResponse(c, errors.WithStack(ErrTransactionNotFound))
}

func (s *HttpRpcServer) getTransactionBlock(c *fiber.Ctx) error {
	transactionHash := c.Params("hash")
	blockNumber, err := s.db.GetBlockNumForTx(transactionHash)
	if err != nil {
		return s.errorResponse(c, errors.WithStack(ErrTransactionNotFound))
	}

	if point, err2 := s.db.GetBlockPoint(blockNumber); err2 == nil {
		if block, err3 := s.fetchClient.FetchBlock(point.Point); err3 == nil {
			return s.returnBlock(c, block)
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
		// "type":        fmt.Sprintf("Witnessed Tx %sEra", s.followClient.GetEra().RawString()),
		"type":        fmt.Sprintf("Witnessed Tx ConwayEra"),
		"description": "Ledger Cddl Format",
		"cborHex":     hex.EncodeToString(req.Tx),
	})

	fmt.Println("-------------------------")
	fmt.Println("sending file")
	fmt.Println(string(txFile))
	fmt.Println("-------------------------")

	output, err := s.runCardanoBinaryWithFiles("transaction submit --cardano-mode --tx-file file://tx", []VirtualFile{
		{
			Name:    "tx",
			Content: txFile,
		},
	})

	if err != nil || strings.TrimSpace(string(output)) != "Transaction successfully submitted." {
		return c.Status(http.StatusInternalServerError).JSON(map[string]any{
			"output": string(output),
			"error":  fmt.Sprintf("%+v", err),
		})
	}

	return c.Status(http.StatusOK).JSON(nil)
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

	output, err := s.runCardanoBinaryWithFiles(data.Method+" "+data.Params, virtualFiles)
	if err != nil {
		fmt.Printf("%+v\n", err)
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

func (s *HttpRpcServer) runCardanoBinary(command string) (out []byte, err error) {
	return s.runCardanoBinaryWithFiles(command, nil)
}

func (s *HttpRpcServer) runCardanoBinaryWithFiles(command string, files []VirtualFile) (out []byte, err error) {
	return runCommandWithVirtualFiles(
		CardanoBinaryPath,
		command,
		[]string{
			fmt.Sprintf("CARDANO_NODE_NETWORK_ID=%d", config.Cardano.ShelleyConfig.NetworkMagic),
			"CARDANO_NODE_SOCKET_PATH=" + config.SocketPath,
		},
		files,
	)
}
