package cardano

import (
	"github.com/blinklabs-io/gouroboros/protocol/common"
)

type PointRef struct {
	Slot   uint64 `json:"slot"`
	Hash   string `json:"hash"`
	Type   int    `json:"era"`
	Height uint64 `json:"height"`
}

type TxRef struct {
	Hash        string `json:"hash"`
	BlockHeight uint64 `json:"blockHeight"`
}

func (pr PointRef) Common() common.Point {
	return common.Point{
		Slot: pr.Slot,
		Hash: HexString(pr.Hash).Bytes(),
	}
}

type Database interface {
	AddTxsForBlock(txHashes []string, block uint64) (err error)
	GetTxs(txHashes []string) (txs []TxRef, err error)
	GetBlockNumForTx(txHash string) (block uint64, err error)
	AddPoints(points []PointRef) (err error)
	SetPointHeights(updates []PointRef) (err error)
	GetPointByHash(blockHash string) (point PointRef, err error)
	GetPointByHeight(height uint64) (point PointRef, err error)
	GetBoundaryPointBehind(blockHash string) (point PointRef, err error)
	GetPointsBySlot(slot uint64) (points []PointRef, err error)
	GetPointsForLastSlot() (points []PointRef, err error)
	GetPointsForProcessing(batchSize int) (points []PointRef, err error)
	GetHighestPoint() (point PointRef, err error)
	HandleRollback(fromSlot uint64) (err error)
}
