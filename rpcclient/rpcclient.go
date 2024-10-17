package rpcclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	. "github.com/alexdcox/cardano-go"
	"github.com/pkg/errors"
)

func NewRpcClient(hostPort string, network Network) (client *RpcClient, err error) {
	client = &RpcClient{
		HostPort: hostPort,
		Network:  network,
	}
	return
}

type RpcClient struct {
	HostPort string
	Network  Network
}

func (c *RpcClient) req(method string, path string, body io.Reader) (rsp *http.Response, out []byte, err error) {
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
		err = errors.Wrapf(ErrRpcFailed, "rpc response code %d with body %s", rsp.StatusCode, string(out))
		return
	}

	return
}

func (c *RpcClient) reqUnmarshal(method string, path string, body io.Reader, target any) (err error) {
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

func (c *RpcClient) get(path string, target any) (err error) {
	return c.reqUnmarshal(http.MethodGet, path, nil, target)
}

func (c *RpcClient) post(path string, in any, target any) (err error) {
	jsn, err := json.Marshal(in)
	if err != nil {
		err = errors.WithStack(err)
		return
	}

	return c.reqUnmarshal(http.MethodPost, path, bytes.NewReader(jsn), target)
}

type GetTipOut struct {
	Block           uint64 `json:"block"`
	Epoch           uint64 `json:"epoch"`
	Era             string `json:"era"`
	Hash            string `json:"hash"`
	Slot            uint64 `json:"slot"`
	SlotInEpoch     uint64 `json:"slotInEpoch"`
	SlotsToEpochEnd uint64 `json:"slotsToEpochEnd"`
	SyncProgress    string `json:"syncProgress"`
}

func (c *RpcClient) GetTip() (out *GetTipOut, err error) {
	in := map[string]any{"method": "query tip"}
	out = &GetTipOut{}
	err = c.post("/cli", in, out)
	return
}

type GetBlockOut Block

func (c *RpcClient) GetBlock(height uint64) (out *GetBlockOut, err error) {
	out = &GetBlockOut{}
	err = c.get(fmt.Sprintf("/block/%d", height), out)
	return
}

type GetTransactionOut struct {
	Block       uint64
	Transaction TransactionBody
}

func (c *RpcClient) GetTransaction(hash string) (out *GetTransactionOut, err error) {
	out = &GetTransactionOut{}
	err = c.get(fmt.Sprintf("/tx/%s", hash), out)
	return
}

type BroadcastRawTxIn struct {
	Tx []byte
}

type BroadcastRawTxOut struct{}

func (c *RpcClient) BroadcastRawTx(in *BroadcastRawTxIn) (out *BroadcastRawTxOut, err error) {
	out = &BroadcastRawTxOut{}
	err = c.post("/broadcast", in, out)
	return
}

type GetUtxosForAddressIn struct {
	Address Address
}

type GetUtxosForAddressOut []Utxo

type Utxo struct {
	TxHash  string `json:"txHash"`
	Address string `json:"address"`
	Value   uint64 `json:"value"`
	Height  uint64 `json:"height"`
}

func (c *RpcClient) GetUtxosForAddress(in *GetUtxosForAddressIn) (out *GetUtxosForAddressOut, err error) {
	addr, err := in.Address.Bech32String(c.Network)
	if err != nil {
		return
	}
	out = &GetUtxosForAddressOut{}
	err = c.get(fmt.Sprintf("/utxo/%s", addr), out)
	return
}

type Fees struct {
	TxFeeFixed      uint64 `json:"txFeeFixed"`
	TxFeePerByte    uint64 `json:"txFeePerByte"`
	UtxoCostPerByte uint64 `json:"utxoCostPerByte"`
}

type GetStatusOut struct {
	Fees Fees `json:"fees"`
}

func (c *RpcClient) GetStatus() (out *GetStatusOut, err error) {
	out = &GetStatusOut{}
	err = c.get("/status", out)
	return
}
