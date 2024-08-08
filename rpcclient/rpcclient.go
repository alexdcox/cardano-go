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

func NewRpcClient(hostPort string) (client *RpcClient, err error) {
	client = &RpcClient{
		HostPort: hostPort,
	}
	return
}

type RpcClient struct {
	HostPort string
	client   *Client
}

func (c *RpcClient) get(path string, out any) (err error) {
	httpRsp, err := http.Get(c.HostPort + path)
	if err != nil {
		err = errors.WithStack(err)
		return
	}

	body, err := io.ReadAll(httpRsp.Body)
	if err != nil {
		err = errors.WithStack(err)
		return
	}

	err = errors.WithStack(json.Unmarshal(body, out))

	return
}

func (c *RpcClient) post(path string, in any, out any) (err error) {
	jsn, err := json.Marshal(in)
	if err != nil {
		err = errors.WithStack(err)
		return
	}

	httpRsp, err := http.Post(c.HostPort+path, "application/json", bytes.NewBuffer(jsn))
	if err != nil {
		err = errors.WithStack(err)
		return
	}

	body, err := io.ReadAll(httpRsp.Body)
	if err != nil {
		err = errors.WithStack(err)
		return
	}

	err = errors.WithStack(json.Unmarshal(body, out))

	return
}

type QueryTipOut struct {
	Block           int    `json:"block"`
	Epoch           int    `json:"epoch"`
	Era             string `json:"era"`
	Hash            string `json:"hash"`
	Slot            int    `json:"slot"`
	SlotInEpoch     int    `json:"slotInEpoch"`
	SlotsToEpochEnd int    `json:"slotsToEpochEnd"`
	SyncProgress    string `json:"syncProgress"`
}

func (c *RpcClient) QueryTip() (out *QueryTipOut, err error) {
	in := map[string]any{"method": "query tip"}
	out = &QueryTipOut{}
	err = errors.WithStack(c.post("/cli", in, out))
	return
}

type QueryBlockOut Block

func (c *RpcClient) QueryBlock(height int64) (out *QueryBlockOut, err error) {
	out = &QueryBlockOut{}
	err = errors.WithStack(c.get(fmt.Sprintf("/block/%d", height), out))
	return
}

type BroadcastRawTxIn struct {
	Tx []byte
}

type BroadcastRawTxOut struct {
	Hash string
}

func (c *RpcClient) BroadcastRawTx(in *BroadcastRawTxIn) (out *BroadcastRawTxOut, err error) {
	out = &BroadcastRawTxOut{}
	err = errors.WithStack(c.post("/broadcast", in, out))
	return
}

type QueryUtxosForAddressIn struct {
	Address string
}

type QueryUtxosForAddressOut struct {
	Address string
}

type Utxo struct {
	TxHash  string `json:"txHash"`
	Address string `json:"address"`
	Value   uint64 `json:"value"`
}

func (c *RpcClient) QueryUtxosForAddress(in *QueryUtxosForAddressIn) (out *QueryUtxosForAddressOut, err error) {
	err = errors.WithStack(c.get(fmt.Sprintf("/utxo/%s", in.Address), out))
	return
}
