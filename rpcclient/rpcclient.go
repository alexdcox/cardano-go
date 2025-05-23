package rpcclient

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	. "github.com/alexdcox/cardano-go"
	"github.com/alexdcox/cbor/v2"
	"github.com/pkg/errors"
)

func NewHttpRpcClient(hostPort string, network Network) (client RpcClient, err error) {
	client = &HttpRpcClient{
		HostPort: hostPort,
		Network:  network,
	}
	return
}

var _ RpcClient = &HttpRpcClient{}

type HttpRpcClient struct {
	HostPort string
	Network  Network
}

func (c *HttpRpcClient) req(method string, path string, body io.Reader) (rsp *http.Response, out []byte, err error) {
	req, err2 := http.NewRequest(method, c.HostPort+path, body)
	if err2 != nil {
		err = err2
		return
	}

	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}

	rsp, err = http.DefaultClient.Do(req)
	if err != nil {
		err = errors.WithStack(err)
		return
	}

	out, err = io.ReadAll(rsp.Body)
	if err != nil {
		err = errors.WithStack(err)
		return
	}

	if rsp.Status[0] != '2' {
		errRsp := &RpcError{}
		if decodeErr := json.Unmarshal(out, errRsp); decodeErr == nil {
			err = errRsp
			return
		}

		err = errors.Wrapf(ErrRpcFailed, "rpc response code %d with body %s", rsp.StatusCode, string(out))
		return
	}

	return
}

func (c *HttpRpcClient) reqUnmarshal(method string, path string, body io.Reader, target any) (err error) {
	_, rspBody, err := c.req(method, path, body)
	if err != nil {
		err = errors.WithStack(err)
		return
	}

	err = json.Unmarshal(rspBody, target)
	if err != nil {
		err = errors.Wrapf(err, "unable to unmarshal body: %s", string(rspBody))
		return
	}

	return

}

func (c *HttpRpcClient) get(path string, target any) (err error) {
	return c.reqUnmarshal(http.MethodGet, path, nil, target)
}

func (c *HttpRpcClient) post(path string, in any, target any) (err error) {
	jsn, err := json.Marshal(in)
	if err != nil {
		err = errors.WithStack(err)
		return
	}

	return c.reqUnmarshal(http.MethodPost, path, bytes.NewReader(jsn), target)
}

type GetHeightOut PointRef

func (c *HttpRpcClient) GetHeight() (out *GetHeightOut, err error) {
	out = &GetHeightOut{}
	err = c.get("/height", out)
	return
}

type GetTransactionOut TxResponse

func (c *HttpRpcClient) GetTransaction(hash string) (out *GetTransactionOut, err error) {
	out = &GetTransactionOut{}
	err = c.get(fmt.Sprintf("/tx/%s", hash), out)
	return
}

type BroadcastTxIn struct {
	TxHex string `json:"tx"`
}

type BroadcastTxOut struct {
	TxHash string `json:"txHash"`
}

func (c *HttpRpcClient) BroadcastTx(in *BroadcastTxIn) (out *BroadcastTxOut, err error) {
	out = &BroadcastTxOut{}
	err = c.post("/tx/broadcast", in, out)
	return
}

type GetUtxosForAddressIn struct {
	Address Address
}

type GetUtxosForAddressOut []Utxo

type Utxo struct {
	TxHash  string `json:"txHash"`
	Address string `json:"address"`
	Amount  uint64 `json:"amount"`
	Index   uint64 `json:"index"`
	Height  uint64 `json:"height"`
}

func (c *HttpRpcClient) GetUtxosForAddress(in *GetUtxosForAddressIn) (out GetUtxosForAddressOut, err error) {
	addr, err := in.Address.Bech32String(c.Network)
	if err != nil {
		return
	}
	out = GetUtxosForAddressOut{}
	err = c.get(fmt.Sprintf("/utxo/%s", addr), &out)
	return
}

type ProtocolOut struct {
	CoinsPerUtxoByte  uint64 `json:"coinsPerUtxoByte"`
	MaxTxSize         uint64 `json:"maxTxSize"`
	MinFeeCoefficient uint64 `json:"minFeeCoefficient"`
	MinFeeConstant    uint64 `json:"minFeeConstant"`
	MinUtxoThreshold  uint64 `json:"minUtxoThreshold"`
}

type GetStatusOut struct {
	Tip         PointRef     `json:"tip"`
	Protocol    *ProtocolOut `json:"fees,omitempty"`
	ReorgWindow int          `json:"reorgWindow"`
}

func (c *HttpRpcClient) GetStatus() (out *GetStatusOut, err error) {
	out = &GetStatusOut{}
	err = c.get("/status", out)
	return
}

type TransactionBuildInput struct {
	TxHash string `json:"txHash"`
	Index  uint64 `json:"index"`
}

type TransactionBuildOutput struct {
	Address string `json:"address"`
	Value   uint64 `json:"value"`
}

type TransactionBuildIn struct {
	Inputs        []TransactionBuildInput  `json:"txIn"`
	Outputs       []TransactionBuildOutput `json:"txOut"`
	ChangeAddress string                   `json:"changeAddress"`
	Memo          string                   `json:"memo"`
}

type TransactionBuildOut struct {
	Hash         HexString `json:"hash"`
	RawHex       string    `json:"rawHex"`
	EstimatedFee uint64    `json:"estimatedFee"`
}

type TransactionEstimateFeeIn struct {
	Inputs        int
	OutputAmounts []uint64
	Memo          string
}

type TransactionEstimateFeeOut struct {
	Size uint64 `json:"size"`
	Fee  uint64 `json:"fee"`
}

type TransactionSignIn struct {
	Tx            HexString `json:"tx"`
	PrivateKeyHex string    `json:"privateKeyHex"`
}

type TransactionSignOut struct {
	Tx HexString `json:"tx"`
}

func (o *TransactionBuildOut) Submission() (submission *TxSubmission, err error) {
	if o == nil {
		return nil, errors.New("transaction build out is nil")
	}

	txHex, err := hex.DecodeString(o.RawHex)
	if err != nil {
		err = errors.Wrap(err, "failed to decode tx cbor from build output")
		return
	}

	submission = &TxSubmission{}
	if err = cbor.Unmarshal(txHex, submission); err != nil {
		err = errors.Wrap(err, "failed to unmarshal tx build output to tx submission struct")
		return
	}

	return
}

func (c *HttpRpcClient) BuildTx(in *TransactionBuildIn) (out *TransactionBuildOut, err error) {
	out = &TransactionBuildOut{}
	err = c.post("/tx/build", in, out)
	return
}

type GetBlockIn struct {
	Height uint64 `json:"height"`
}

func (c *HttpRpcClient) GetBlockByHash(hash string) (out *BlockResponse, err error) {
	out = &BlockResponse{}
	err = c.get(fmt.Sprintf("/block/%s", hash), out)
	return
}

func (c *HttpRpcClient) GetBlockByHeight(height uint64) (out *BlockResponse, err error) {
	out = &BlockResponse{}
	err = c.get(fmt.Sprintf("/block/%d", height), out)
	return
}

type BlockResponse struct {
	Height       uint64       `json:"height"`
	Slot         uint64       `json:"slot"`
	Hash         string       `json:"hash"`
	Type         int          `json:"type"`
	Transactions []TxResponse `json:"transactions"`
}

type TxInput struct {
	Hash  string `json:"hash"`
	Index uint32 `json:"index"`
}

type TxOutput struct {
	Amount  uint64 `json:"amount"`
	Address string `json:"address"`
}

type TxResponse struct {
	Hash    string     `json:"hash"`
	Inputs  []TxInput  `json:"inputs"`
	Outputs []TxOutput `json:"outputs"`
	Memo    string     `json:"memo,omitempty"`
	Fee     uint64     `json:"fee"`
}

type PublicKeyToAddress struct {
	Network      Network `json:"network"`
	PublicKeyHex string  `json:"publicKeyHex"`
}

type RpcClient interface {
	GetHeight() (out *GetHeightOut, err error)
	GetTransaction(hash string) (out *GetTransactionOut, err error)
	BroadcastTx(in *BroadcastTxIn) (out *BroadcastTxOut, err error)
	GetUtxosForAddress(in *GetUtxosForAddressIn) (out GetUtxosForAddressOut, err error)
	GetStatus() (out *GetStatusOut, err error)
	BuildTx(in *TransactionBuildIn) (out *TransactionBuildOut, err error)
	GetBlockByHash(hash string) (out *BlockResponse, err error)
	GetBlockByHeight(height uint64) (out *BlockResponse, err error)
}
