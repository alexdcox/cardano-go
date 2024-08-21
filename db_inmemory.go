package cardano

import (
	"database/sql"
	"sync"

	"github.com/pkg/errors"
)

type ChunkRange struct {
	Start uint64
	End   uint64
	Chunk uint64
}

type InMemoryDatabase struct {
	chunks map[uint64]ChunkRange
	txs    map[string]uint64
	tip    PointAndBlockNum
	points map[uint64]PointAndBlockNum
	mu     sync.RWMutex
}

var _ Database = &InMemoryDatabase{}

func NewInMemoryDatabase() *InMemoryDatabase {
	return &InMemoryDatabase{
		chunks: make(map[uint64]ChunkRange),
		txs:    make(map[string]uint64),
		points: make(map[uint64]PointAndBlockNum),
	}
}

func (db *InMemoryDatabase) SetChunkRange(chunk, start, end uint64) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if end <= start {
		return errors.Errorf("invalid chunk range %d to %d", start, end)
	}

	for c, r := range db.chunks {
		if c != chunk && ((start >= r.Start && start <= r.End) || (end >= r.Start && end <= r.End)) {
			return errors.Errorf("new range (%d, %d) overlaps with existing ranges", start, end)
		}
	}

	db.chunks[chunk] = ChunkRange{Start: start, End: end}
	return nil
}

func (db *InMemoryDatabase) GetChunkRange(number uint64) (chunk, start, end uint64, err error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	for c, r := range db.chunks {
		if number >= r.Start && number <= r.End {
			return c, r.Start, r.End, nil
		}
	}

	return 0, 0, 0, sql.ErrNoRows
}

func (db *InMemoryDatabase) AddTxsForBlock(txhashes []string, blockNumber uint64) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	for _, txhash := range txhashes {
		db.txs[txhash] = blockNumber
	}

	return nil
}

func (db *InMemoryDatabase) GetBlockForTx(txhash string) (blockNumber uint64, err error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	blockNumber, ok := db.txs[txhash]
	if !ok {
		return 0, sql.ErrNoRows
	}

	return blockNumber, nil
}

func (db *InMemoryDatabase) GetChunkSpan() (lowestChunk, highestChunk uint64, err error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if len(db.chunks) == 0 {
		return 0, 0, nil
	}

	for _, chunk := range db.chunks {
		if chunk.Chunk < lowestChunk || lowestChunk == 0 {
			lowestChunk = chunk.Chunk
		}
		if chunk.Chunk > highestChunk {
			highestChunk = chunk.Chunk
		}
	}

	return lowestChunk, highestChunk, nil
}

func (db *InMemoryDatabase) GetChunkedBlockSpan() (lowestBlock, highestBlock uint64, err error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if len(db.chunks) == 0 {
		return 0, 0, nil
	}

	for _, chunk := range db.chunks {
		if chunk.Start < lowestBlock || lowestBlock == 0 {
			lowestBlock = chunk.Start
		}
		if chunk.End > highestBlock {
			highestBlock = chunk.End
		}
	}

	return lowestBlock, highestBlock, nil
}

func (db *InMemoryDatabase) GetPointSpan() (lowestBlock, highestBlock uint64, err error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if len(db.chunks) == 0 {
		return 0, 0, nil
	}

	for blockNumber, _ := range db.points {
		if blockNumber < lowestBlock || lowestBlock == 0 {
			lowestBlock = blockNumber
		}
		if blockNumber > highestBlock {
			highestBlock = blockNumber
		}
	}

	return lowestBlock, highestBlock, nil
}

func (db *InMemoryDatabase) AddBlockPoint(point PointAndBlockNum) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	db.points[point.Block] = point
	return nil
}

func (db *InMemoryDatabase) GetBlockPoint(number uint64) (point PointAndBlockNum, err error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	point, ok := db.points[number]
	if !ok {
		return PointAndBlockNum{}, errors.New("block point not found")
	}

	return point, nil
}
