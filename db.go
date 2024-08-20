package cardano

import (
	_ "github.com/mattn/go-sqlite3"
)

type Database interface {
	SetChunkRange(chunk, start, end uint64) (err error)
	GetChunkRange(number uint64) (start, end uint64, chunk uint64, err error)
	GetChunkSpan() (first, last uint64, err error)
	GetBlockSpan() (first, last uint64, err error)

	AddTxsForBlock(txhashes []string, blockNumber uint64) (err error)
	GetBlockForTx(txhash string) (blockNumber uint64, err error)

	SetTip(tip PointAndBlockNum) error
	GetTip() (PointAndBlockNum, error)

	AddBlockPoint(number uint64, point Point) (err error)
	GetBlockPoint(number uint64) (point Point, err error)
}
