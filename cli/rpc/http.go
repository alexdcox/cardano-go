package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	. "github.com/alexdcox/cardano-go"
	db2 "github.com/alexdcox/cardano-go/db"
	"github.com/alexdcox/cardano-go/rpcclient"
	"github.com/fxamacker/cbor/v2"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
)

func NewHttpRpcServer(config *_config, chunkReader *ChunkReader, db db2.Database) (server *HttpRpcServer, err error) {
	client, err := NewClient(config.NodeHostPort, Network(config.Network))
	if err != nil {
		return
	}

	server = &HttpRpcServer{
		config:      config,
		client:      client,
		chunkReader: chunkReader,
		db:          db,
	}

	return
}

type HttpRpcServer struct {
	echo        *echo.Echo
	client      *Client
	chunkReader *ChunkReader
	config      *_config
	db          db2.Database
}

func (s *HttpRpcServer) Start() (err error) {
	// TODO: Uncomment && move higher
	// err = s.client.Start()
	// if err != nil {
	// 	return
	// }

	s.echo = echo.New()
	s.echo.HideBanner = true
	s.echo.HidePort = true
	s.echo.Debug = true
	s.echo.Server.ReadTimeout = 30 * time.Second
	s.echo.Server.WriteTimeout = 30 * time.Second
	s.echo.Server.IdleTimeout = 120 * time.Second
	s.echo.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus: true, LogRemoteIP: true, LogMethod: true, LogURIPath: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) (err error) {
			log.Info().Msgf("[%d] %s - %s %s", v.Status, v.RemoteIP, v.Method, v.URIPath)
			return
		},
	}))
	s.echo.Use(middleware.Recover())
	s.echo.GET("/utxo/:address", s.getUtxoForAddress)
	s.echo.GET("/block/latest", s.getLatestBlock)
	s.echo.GET("/block/:number", s.getBlock)
	s.echo.POST("/broadcast", s.postBroadcast)
	s.echo.GET("/status", s.getStatus)
	s.echo.GET("/tip", s.getTip)
	s.echo.POST("/cli", s.postCli)

	log.Info().Msgf("server listening on %s", config.RpcHostPort)

	err = errors.WithStack(s.echo.Start(config.RpcHostPort))

	return
}

func (s *HttpRpcServer) Stop() (err error) {
	return s.client.Stop()
}

func (s *HttpRpcServer) getStatus(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{
		"db":     s.chunkReader.Status,
		"config": s.config,
	})
}

func (s *HttpRpcServer) getUtxoForAddress(c echo.Context) error {
	addr := &Address{}
	if err := addr.ParseBech32String(c.Param("address"), Network(config.Network)); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "address invalid",
		})
	}
	output, err := runCommandWithVirtualFiles(
		"/usr/local/bin/cardano-cli",
		"query utxo --out-file /dev/stdout --address "+c.Param("address"),
		[]string{
			fmt.Sprintf("CARDANO_NODE_NETWORK_ID=%d", config.Cardano.ShelleyConfig.NetworkMagic),
			"CARDANO_NODE_SOCKET_PATH=" + config.SocketPath,
		},
		[]VirtualFile{},
	)

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"output": string(output),
			"error":  fmt.Sprintf("%+v", err),
		})
	}

	var mime = "text/plain"
	if gjson.ParseBytes(output).IsObject() {
		mime = "application/json"
	}

	formattedOutput := []Utxo{}

	return c.Blob(http.StatusOK, mime, output)
}

func (s *HttpRpcServer) getLatestBlock(c echo.Context) error {
	// TODO: This needs to use the http rpc client not create a new one

	client, err := NewClient("localhost:3000", NetworkPrivateNet)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err)
	}

	err = client.Start()
	defer client.Stop()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err)
	}

	block, err := client.FetchLatestBlock()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err)
	}

	return c.JSON(http.StatusOK, block)
}

func (s *HttpRpcServer) getBlock(c echo.Context) error {
	blockNumberStr := c.Param("number")
	blockNumber, err := strconv.Atoi(blockNumberStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid block number"})
	}

	returnBlock := func(block *Block) error {
		if c.Request().Header.Get("Content-Type") == "application/hex" {
			blockBytes, _ := cbor.Marshal(block)
			return c.String(http.StatusOK, fmt.Sprintf("%x", blockBytes))
		}
		if c.Request().Header.Get("Content-Type") == "application/cbor" {
			blockBytes, _ := cbor.Marshal(block)
			return c.Blob(http.StatusOK, "application/cbor", blockBytes)
		}
		return c.JSON(http.StatusOK, block)
	}

	if block, err2 := s.chunkReader.GetBlock(int64(blockNumber)); err2 == nil {
		return returnBlock(block)
	}

	// TODO: client.GetBlock
	// if block, err2 := s.client.GetBlock(int64(blockNumber)); err2 == nil {
	// 	return returnBlock(block)
	// }

	return c.JSON(http.StatusNotFound, map[string]any{
		"error": ErrBlockNotFound,
	})
}

func (s *HttpRpcServer) getTip(c echo.Context) error {
	output, err := runCommandWithVirtualFiles(
		"/usr/local/bin/cardano-cli",
		"query tip",
		[]string{
			fmt.Sprintf("CARDANO_NODE_NETWORK_ID=%d", config.Cardano.ShelleyConfig.NetworkMagic),
			"CARDANO_NODE_SOCKET_PATH=" + config.SocketPath,
		},
		[]VirtualFile{},
	)

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"output": string(output),
			"error":  fmt.Sprintf("%+v", err),
		})
	}

	var mime = "text/plain"
	if gjson.ParseBytes(output).IsObject() {
		mime = "application/json"
	}

	return c.Blob(http.StatusOK, mime, output)
}

func (s *HttpRpcServer) postBroadcast(c echo.Context) error {
	req := rpcclient.BroadcastRawTxRequest{}
	if err := s.unmarshalJson(c, &req); err != nil {
		return err
	}

	panic("not implemented")
	// TODO: convert to tx submit
}

func (s *HttpRpcServer) unmarshalJson(c echo.Context, target any) (err error) {
	if c.Request().Header.Get("Content-Type") != "application/json" {
		return c.NoContent(http.StatusBadRequest)
	}

	bodyBytes, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("%+v", err),
		})
	}

	err = json.Unmarshal(bodyBytes, target)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("%+v", err),
		})
	}

	return
}

func (s *HttpRpcServer) postCli(c echo.Context) error {
	var data InputData
	if err := s.unmarshalJson(c, &data); err != nil {
		return err
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
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"output": string(output),
			"error":  fmt.Sprintf("%+v", err),
		})
	}

	var mime = "text/plain"
	if gjson.ParseBytes(output).IsObject() {
		mime = "application/json"
	}

	return c.Blob(http.StatusOK, mime, output)
}
