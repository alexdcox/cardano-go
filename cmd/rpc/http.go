package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	. "github.com/alexdcox/cardano-go"
	"github.com/alexdcox/cardano-go/rpcclient"
	"github.com/alexdcox/cbor/v2"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
)

const (
	CardanoBinaryPath = "/usr/local/bin/cardano-cli"
)

func NewHttpRpcServer(config *_config, db Database, client *Client) (server *HttpRpcServer, err error) {
	server = &HttpRpcServer{
		config: config,
		client: client,
		db:     db,
	}

	return
}

type HttpRpcServer struct {
	app    *fiber.App
	client *Client
	config *_config
	db     Database
}

func (s *HttpRpcServer) Start() (err error) {
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
	s.app.Post("/tx/build", s.postTransactionBuild)
	s.app.Post("/tx/sign", s.postTransactionSign)
	s.app.Post("/tx/broadcast", s.postTransactionBroadcast)
	s.app.Post("/tx/estimate-fee", s.postTransactionEstimateFee)
	s.app.Get("/status", s.getStatus)
	s.app.Get("/height", s.getHeight)
	s.app.Post("/tools/pubkey-to-address", s.postPubkeyToAddress)

	// TODO: clean up this dangerous command
	s.app.Post("/cli", s.postCli)

	log.Info().Msgf("http/rpc server listening on %s", config.RpcHostPort)

	err = errors.WithStack(s.app.Listen(config.RpcHostPort))

	return
}

func (s *HttpRpcServer) Stop() (err error) {
	return errors.WithStack(s.app.Shutdown())
}

func (s *HttpRpcServer) errorResponse(c *fiber.Ctx, err error) error {
	statusCode := http.StatusInternalServerError

	reportedErr := err

	for _, match := range []error{
		ErrPointNotFound,
		ErrBlockNotFound,
		ErrTransactionNotFound,
	} {
		if errors.Is(err, match) {
			reportedErr = match
			statusCode = http.StatusNotFound
			break
		}
	}

	return c.Status(statusCode).JSON(map[string]any{
		"error":   reportedErr.Error(),
		"details": fmt.Sprintf("%+v", err),
	})
}

func (s *HttpRpcServer) blockResponse(block ledger.Block) rpcclient.BlockResponse {
	transactions := make([]rpcclient.TxResponse, 0, len(block.Transactions()))
	for _, tx := range block.Transactions() {
		transactions = append(transactions, s.txResponse(tx))
	}

	return rpcclient.BlockResponse{
		Height:       block.BlockNumber(),
		Slot:         block.SlotNumber(),
		Hash:         block.Hash(),
		Type:         block.Type(),
		Transactions: transactions,
	}
}

func (s *HttpRpcServer) txResponse(tx ledger.Transaction) rpcclient.TxResponse {
	inputs := make([]rpcclient.TxInput, 0, len(tx.Inputs()))
	for _, input := range tx.Inputs() {
		inputs = append(inputs, rpcclient.TxInput{
			Hash:  input.Id().String(),
			Index: input.Index(),
		})
	}

	outputs := make([]rpcclient.TxOutput, 0, len(tx.Outputs()))
	for _, output := range tx.Outputs() {
		outputs = append(outputs, rpcclient.TxOutput{
			Amount:  output.Amount(),
			Address: output.Address().String(),
		})
	}

	memo := ""
	if tx.Metadata() != nil {
		auxData := &AuxData{}
		if err := cbor.Unmarshal(tx.Metadata().Cbor(), auxData); err == nil {
			if j, err := json.Marshal(auxData); err == nil {
				// TODO: DONT TRUST IT - Probably need to loop the parent array here instead of the next level down
				for _, x := range gjson.ParseBytes(j).Get("0.0.1337.memo").Array() {
					memo += x.String()
				}
			}
		}
	}

	return rpcclient.TxResponse{
		Hash:    tx.Hash(),
		Inputs:  inputs,
		Outputs: outputs,
		Memo:    memo,
		Fee:     tx.Fee(),
	}
}

func (s *HttpRpcServer) getStatus(c *fiber.Ctx) error {
	highestPoint, err := s.client.GetHeight()
	if err != nil {
		return s.errorResponse(c, err)
	}

	fileInfo, err := os.Stat(s.config.DatabasePath)
	if err != nil {
		return s.errorResponse(c, err)
	}

	sizeInBytes := fileInfo.Size()
	sizeInMB := float64(sizeInBytes) / 1024 / 1024

	tip, err := s.client.GetTip()
	if err != nil {
		return s.errorResponse(c, err)
	}

	protocol, err := s.client.GetProtocolParams()
	if err != nil {
		return s.errorResponse(c, err)
	}
	pp := protocol.Utxorpc()

	out := rpcclient.GetStatusOut{
		Tip: highestPoint,
		Protocol: rpcclient.ProtocolOut{
			CoinsPerUtxoByte:  pp.CoinsPerUtxoByte,
			MaxTxSize:         pp.MaxTxSize,
			MinFeeCoefficient: pp.MinFeeCoefficient,
			MinFeeConstant:    pp.MinFeeConstant,
			MinUtxoThreshold:  MinUtxoBytes * pp.CoinsPerUtxoByte,
		},
	}

	return c.JSON(out)

	return c.JSON(map[string]any{
		"db": map[string]any{
			"sizeMB": fmt.Sprintf("%.2f MB", sizeInMB),
		},
		"height": highestPoint,
		"tip":    tip,
		"config": s.config,
		"protocol": map[string]any{
			"coinsPerUtxoByte":  pp.CoinsPerUtxoByte,
			"maxTxSize":         pp.MaxTxSize,
			"minFeeCoefficient": pp.MinFeeCoefficient,
			"minFeeConstant":    pp.MinFeeConstant,
		},
	})
}

func (s *HttpRpcServer) getUtxoForAddress(c *fiber.Ctx) error {
	addr := &Address{}
	if err := addr.ParseBech32String(c.Params("address"), Network(config.Network)); err != nil {
		return s.errorResponse(c, err)
	}

	utxos, err := s.client.GetUtxoForAddress(c.Params("address"))
	if err != nil {
		return s.errorResponse(c, err)
	}

	response := []any{}

	for _, utxo := range utxos {
		response = append(response, rpcclient.Utxo{
			TxHash:  utxo.TxHash,
			Index:   utxo.Index,
			Amount:  utxo.Amount,
			Address: utxo.Address,
			Height:  utxo.Height,
		})
	}

	return c.JSON(response)
}

func (s *HttpRpcServer) getLatestBlock(c *fiber.Ctx) error {
	point, err := s.db.GetHighestPoint()
	if err != nil {
		return s.errorResponse(c, err)
	}

	fmt.Printf("latest block: %v\n", point)

	block, err := s.client.GetBlockByPoint(point)
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
		height, err := strconv.ParseUint(param, 10, 64)
		if err != nil {
			return s.errorResponse(c, err)
		}
		return s.getBlockByHeight(c, height)
	}
}

func (s *HttpRpcServer) getBlockByHash(c *fiber.Ctx, hash string) error {
	point, err := s.db.GetPointByHash(hash)
	if err != nil {
		// TODO: maybe handle 404/norow in errorResponse
		return s.errorResponse(c, err)
	}

	return s.getBlockByPoint(c, point)
}

func (s *HttpRpcServer) getBlockByHeight(c *fiber.Ctx, number uint64) error {
	point, err := s.db.GetPointByHeight(number)
	if err != nil {
		// TODO: maybe handle 404/norow in errorResponse
		return s.errorResponse(c, err)
	}

	return s.getBlockByPoint(c, point)
}

func (s *HttpRpcServer) getBlockByPoint(c *fiber.Ctx, point PointRef) error {
	block, err := s.client.GetBlockByPoint(point)
	if err != nil {
		return s.errorResponse(c, err)
	}

	return c.JSON(s.blockResponse(block))
}

func (s *HttpRpcServer) getTransaction(c *fiber.Ctx) error {
	txHash := c.Params("hash")

	height, err := s.db.GetBlockNumForTx(txHash)
	if err != nil {
		return s.errorResponse(c, err)
	}

	block, err := s.client.GetBlockByHeight(height)
	if err != nil {
		return s.errorResponse(c, err)
	}

	for _, tx := range block.Transactions() {
		if tx.Hash() == txHash {
			return c.JSON(s.txResponse(tx))
		}
	}

	return s.errorResponse(c, errors.WithStack(ErrTransactionNotFound))
}

func (s *HttpRpcServer) getTransactionBlock(c *fiber.Ctx) error {
	txHash := c.Params("hash")

	height, err := s.db.GetBlockNumForTx(txHash)
	if err != nil {
		return s.errorResponse(c, err)
	}

	block, err := s.client.GetBlockByHeight(height)
	if err != nil {
		return s.errorResponse(c, err)
	}

	return c.JSON(s.blockResponse(block))
}

func (s *HttpRpcServer) postTransactionBuild(c *fiber.Ctx) error {
	in := &rpcclient.TransactionBuildIn{}
	if err := s.unmarshalJson(c, &in); err != nil {
		return err
	}

	args := []string{
		"conway transaction build",
		"--cardano-mode",
		"--change-address " + in.ChangeAddress,
		"--out-file /dev/stdout",
	}

	for _, txIn := range in.Inputs {
		args = append(args, fmt.Sprintf("--tx-in %s#%d", txIn.TxHash, txIn.Index))
	}

	for _, txOut := range in.Outputs {
		args = append(args, fmt.Sprintf("--tx-out %s+%d", txOut.Address, txOut.Value))
	}

	var files []VirtualFile

	if in.Memo != "" {
		metadataJson, err := json.Marshal(map[string]any{
			"0": map[string]any{
				"1337": map[string]any{
					"memo": ChunkString(in.Memo, MaxAuxDataStringSize),
				},
			},
		})
		if err != nil {
			return s.errorResponse(c, err)
		}
		args = append(args, "--metadata-json-file file://metadata")
		files = []VirtualFile{
			{
				Name:    "metadata",
				Content: metadataJson,
			},
		}
	}

	buildResult, err := s.runCardanoBinaryWithFiles(strings.Join(args, " "), files)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(map[string]any{
			"details": string(buildResult),
			"error":   ErrNodeCommandFailed.Error(),
		})
	}

	// NOTE: The buildResult will be a JSON response followed by a fee estimate string like:
	//
	// {
	//   "type": "Unwitnessed Tx ConwayEra",
	//   "description": "Ledger Cddl Format",
	//   "cborHex ": "..."
	//  }
	//  Estimated transaction fee: 170429 Lovelace

	buildResultLines := strings.Split(strings.TrimSpace(string(buildResult)), "\n")
	if len(buildResultLines) < 2 {
		return s.errorResponse(c, errors.Wrapf(
			ErrInvalidCliResponse,
			"unable to parse build tx cardano-cli response: %s",
			string(buildResult)))
	}

	buildResultJson := strings.Join(buildResultLines[:len(buildResultLines)-1], "\n")
	buildResultFeeEstimate := buildResultLines[len(buildResultLines)-1]

	buildJson := gjson.Parse(buildResultJson)

	hashResult, err := s.runCardanoBinaryWithFiles(strings.Join(
		[]string{
			"conway transaction txid",
			"--tx-file file://tx",
		},
		" ",
	), []VirtualFile{
		{
			Name:    "tx",
			Content: []byte(buildJson.Raw),
		},
	})
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(map[string]any{
			"details": string(hashResult),
			"error":   ErrNodeCommandFailed.Error(),
		})
	}

	matches := regexp.
		MustCompile(`(?i)(fee:\s*)(\d+)(\s+lovelace)`).
		FindAllStringSubmatch(buildResultFeeEstimate, 1)
	if len(matches) != 1 || len(matches[0]) != 4 || matches[0][2] == "" {
		return s.errorResponse(c, errors.Wrapf(
			ErrInvalidCliResponse,
			"unable to find estimated fee from cardano-cli response: %s",
			string(buildResult)))
	}
	fee, err := strconv.ParseUint(matches[0][2], 10, 64)
	if err != nil {
		return s.errorResponse(c, errors.Wrapf(
			ErrInvalidCliResponse,
			"unable to parse estimated fee from cardano-cli response: %s",
			string(buildResult)))
	}

	return c.JSON(&rpcclient.TransactionBuildOut{
		EstimatedFee: fee,
		RawHex:       buildJson.Get("cborHex").String(),
		Hash:         HexString(strings.TrimSpace(string(hashResult))),
	})
}

func (s *HttpRpcServer) postTransactionSign(c *fiber.Ctx) error {
	in := &rpcclient.TransactionSignIn{}
	if err := s.unmarshalJson(c, &in); err != nil {
		return err
	}

	args := []string{
		"conway transaction sign",
		"--tx-body-file file://tx",
		"--signing-key-file file://key",
		"--out-file /dev/stdout",
	}

	txJson, err := json.Marshal(map[string]any{
		"type":        "Unwitnessed Tx ConwayEra",
		"description": "Ledger Cddl Format",
		"cborHex":     in.Tx,
	})
	if err != nil {
		return s.errorResponse(c, err)
	}

	keyBytes, err := hex.DecodeString(in.PrivateKeyHex)
	if err != nil {
		return s.errorResponse(c, ErrInvalidPrivateKey)
	}

	keyCborBytes, err := cbor.Marshal(keyBytes)
	if err != nil {
		return s.errorResponse(c, err)
	}

	keyCborHex := hex.EncodeToString(keyCborBytes)

	keyJson, err := json.Marshal(map[string]any{
		"type":        "PaymentSigningKeyShelley_ed25519",
		"description": "Payment Signing Key",
		"cborHex":     keyCborHex,
	})
	if err != nil {
		return s.errorResponse(c, err)
	}

	files := []VirtualFile{
		{
			Name:    "tx",
			Content: txJson,
		},
		{
			Name:    "key",
			Content: keyJson,
		},
	}

	signResult, err := s.runCardanoBinaryWithFiles(strings.Join(args, " "), files)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(map[string]any{
			"details": string(signResult),
			"error":   ErrNodeCommandFailed.Error(),
		})
	}

	signJson := gjson.ParseBytes(signResult)

	return c.JSON(&rpcclient.TransactionSignOut{
		Tx: HexString(signJson.Get("cborHex").String()),
	})
}

func (s *HttpRpcServer) getHeight(c *fiber.Ctx) error {
	height, err := s.client.GetHeight()
	if errors.Is(err, ErrPointNotFound) {
		return c.Status(http.StatusNotFound).SendString("Block processor starting...")
	} else if err != nil {
		return s.errorResponse(c, err)
	}

	return c.JSON(height)
}

func (s *HttpRpcServer) postPubkeyToAddress(c *fiber.Ctx) error {
	var req rpcclient.PublicKeyToAddress
	if err := s.unmarshalJson(c, &req); err != nil {
		return s.errorResponse(c, err)
	}

	log.Debug().Msgf(
		"converting public key to address | network '%s' | address hex '%s'",
		req.Network,
		req.PublicKeyHex)

	if !req.Network.Valid() {
		return s.errorResponse(c, ErrNetworkInvalid)
	}

	publicKeyBytes, err := hex.DecodeString(req.PublicKeyHex)
	if err != nil {
		return s.errorResponse(c, ErrInvalidPublicKey)
	}

	address, err := EncodeAddress(publicKeyBytes, req.Network, AddressTypePayment)
	if err != nil {
		return s.errorResponse(c, err)
	}

	addrBech32, err := address.Bech32String(req.Network)

	log.Debug().Msgf("encoded address: '%s'", addrBech32)

	return c.JSON(map[string]any{"address": addrBech32})
}

func (s *HttpRpcServer) postTransactionBroadcast(c *fiber.Ctx) error {
	var req rpcclient.BroadcastTxIn
	if err := s.unmarshalJson(c, &req); err != nil {
		return err
	}

	fmt.Printf("txhex: %s\n", req.TxHex)

	txFile, _ := json.Marshal(map[string]any{
		// "type":        fmt.Sprintf("Witnessed Tx %sEra", s.followClient.GetEra().RawString()),
		"type":        fmt.Sprintf("Witnessed Tx ConwayEra"),
		"description": "Ledger Cddl Format",
		"cborHex":     req.TxHex,
	})

	fmt.Println("-------------------------")
	fmt.Println("sending file")
	fmt.Println(string(txFile))
	fmt.Println("-------------------------")

	files := []VirtualFile{
		{
			Name:    "tx",
			Content: txFile,
		},
	}

	broadcastResult, err := s.runCardanoBinaryWithFiles(strings.Join(
		[]string{
			"conway transaction submit",
			"--cardano-mode",
			"--tx-file file://tx",
		}, " "),
		files,
	)
	if err != nil || strings.TrimSpace(string(broadcastResult)) != "Transaction successfully submitted." {
		return c.Status(http.StatusInternalServerError).JSON(map[string]any{
			"details": string(broadcastResult),
			"error":   ErrNodeCommandFailed.Error(),
		})
	}

	hashResult, err := s.runCardanoBinaryWithFiles(strings.Join(
		[]string{
			"conway transaction txid",
			"--tx-file file://tx",
		},
		" ",
	), files)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(map[string]any{
			"details": string(hashResult),
			"error":   ErrNodeCommandFailed.Error(),
		})
	}

	return c.Status(http.StatusOK).JSON(map[string]any{
		"txHash": HexString(strings.TrimSpace(string(hashResult))),
	})
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
			"details": string(output),
			"error":   ErrNodeCommandFailed.Error(),
		})
	}

	var mime = "text/plain"
	if gjson.ParseBytes(output).IsObject() {
		mime = "application/json"
	}

	c.Type(mime)

	return c.Send(output)
}

func (s *HttpRpcServer) postTransactionEstimateFee(c *fiber.Ctx) error {
	var req rpcclient.TransactionEstimateFeeIn
	if err := s.unmarshalJson(c, &req); err != nil {
		return err
	}

	fmt.Println("--------------------------------------------------")
	fmt.Println("ESTIMATE FEE")
	if j, err := json.MarshalIndent(req, "", "  "); err == nil {
		fmt.Println(string(j))
	}

	tx2 := &TxSubmission{}
	inputs := []TransactionInput{}
	for i := 0; i < req.Inputs; i++ {
		inputs = append(inputs, TransactionInput{
			Txid:  make(HexBytes, 32),
			Index: 1,
		})
	}
	outputs := []SubtypeOf[TransactionOutput]{}
	for _, outputAmount := range req.OutputAmounts {
		outputs = append(outputs, SubtypeOf[TransactionOutput]{
			Subtype: &TransactionOutputG{
				Address: make(Address, 29),
				Amount:  outputAmount,
			},
		})
	}
	tx2.Body = TxSubmissionBody{
		Inputs: WithCborTag[[]TransactionInput]{
			Tag:   258,
			Value: inputs,
		},
		Outputs:           outputs,
		Fee:               200000,
		AuxiliaryDataHash: make(HexBytes, 32),
	}
	tx2.AlonzoEval = true
	tx2.Witness = TxSubmissionWitness{
		Signers: WithCborTag[[]TxSigner]{
			Tag: 258,
			Value: []TxSigner{
				{
					Key:       make([]byte, 32),
					Signature: make([]byte, 64),
				},
			},
		},
	}
	if req.Memo != "" {
		tx2.AuxiliaryData = &AuxData{
			Value: KVSlice{
				KV{
					K: "Tag",
					V: uint64(259),
				},
				KV{
					K: "Content",
					V: KVSlice{
						{
							K: 0,
							V: KVSlice{
								{
									K: 0,
									V: KVSlice{
										{
											K: 1337,
											V: KVSlice{
												{
													K: "memo",
													V: ChunkString(req.Memo, MaxAuxDataStringSize),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
	}
	tx2Bytes, err := cbor.Marshal(tx2)
	if err != nil {
		return s.errorResponse(c, err)
	}

	estimatedSize := len(tx2Bytes)
	estimatedFee := 155381 + ((estimatedSize + 100) * 44)

	rsp := rpcclient.TransactionEstimateFeeOut{
		Size: uint64(estimatedSize),
		Fee:  uint64(estimatedFee),
	}
	if j, err := json.MarshalIndent(rsp, "", "  "); err == nil {
		fmt.Println(string(j))
	}

	return c.JSON(rsp)
}

func (s *HttpRpcServer) runCardanoBinary(command string) (out []byte, err error) {
	return s.runCardanoBinaryWithFiles(command, nil)
}

func (s *HttpRpcServer) runCardanoBinaryWithFiles(command string, files []VirtualFile) (out []byte, err error) {
	params, err := s.client.Options.Network.Params()
	if err != nil {
		return
	}

	return runCommandWithVirtualFiles(
		CardanoBinaryPath,
		command,
		[]string{
			fmt.Sprintf("CARDANO_NODE_NETWORK_ID=%d", params.Magic),
			"CARDANO_NODE_SOCKET_PATH=" + config.SocketPath,
		},
		files,
	)
}
