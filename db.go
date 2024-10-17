package cardano

import (
	_ "github.com/mattn/go-sqlite3"
)

type Database interface {
	SetChunkRange(chunk, start, end uint64) (err error)
	GetChunkRange(block uint64) (chunk, start, end uint64, err error)
	GetChunkSpan() (first, last uint64, err error)
	GetChunkedBlockSpan() (first, last uint64, err error)

	AddTxsForBlock(txhashes []string, block uint64) (err error)

	GetBlockNumForTx(txhash string) (block uint64, err error)
	GetBlockNumForHash(blockHash string) (block uint64, err error)

	AddBlockPoint(point PointAndBlockNum) (err error)
	GetBlockPoint(block uint64) (point PointAndBlockNum, err error)
	GetPointSpan() (firstBlock, lastBlock uint64, err error)
}
