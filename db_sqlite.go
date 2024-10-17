package cardano

import (
	"database/sql"
	"sync"

	"github.com/pkg/errors"
)

type SqlLiteDatabase struct {
	db *sql.DB
	mu sync.Mutex
}

var _ Database = &SqlLiteDatabase{}

func NewSqlLiteDatabase(path string) (db *SqlLiteDatabase, err error) {
	log.Info().Msgf("opening sqlite db at: '%s'", path)

	sqldb, err := sql.Open("sqlite3", path)
	if err != nil {
		err = errors.Wrap(err, "failed to open database")
		return
	}

	if err = sqldb.Ping(); err != nil {
		_ = sqldb.Close()
		err = errors.Wrap(err, "failed to ping database")
		return
	}

	db = &SqlLiteDatabase{db: sqldb}
	if err = db.initTables(); err != nil {
		_ = sqldb.Close()
		err = errors.Wrap(err, "failed to init tables")
		return
	}

	return
}

func (s *SqlLiteDatabase) initTables() (err error) {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS chunks (
			chunk INTEGER PRIMARY KEY,
			start INTEGER,
			end INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS txs (
			txhash TEXT PRIMARY KEY,
			block_number INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS tip (
			slot INTEGER PRIMARY KEY,
			hash TEXT,
			number INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS points (
			number INTEGER PRIMARY KEY,
			slot INTEGER,
			hash TEXT
		)`,
	}

	for i, query := range queries {
		_, err = s.db.Exec(query)
		if err != nil {
			err = errors.Wrapf(err, "failed to execute query: %d", i)
			return
		}
	}

	return
}

func (s *SqlLiteDatabase) SetChunkRange(chunk, start, end uint64) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if end <= start {
		err = errors.Errorf("invalid chunk range %d to %d", start, end)
		return
	}

	var count int
	err = s.db.QueryRow(`
		SELECT COUNT(*) FROM chunks 
		WHERE
			(
				(:start BETWEEN start AND end) OR
				(:end BETWEEN start AND end)
			)
	    AND chunk IS NOT :chunk`,
		sql.Named("chunk", chunk),
		sql.Named("start", start),
		sql.Named("end", end),
	).Scan(&count)
	if err != nil {
		return errors.WithStack(err)
	}
	if count > 0 {
		return errors.Errorf("new range (%d, %d) overlaps with existing ranges", start, end)
	}

	_, err = s.db.Exec(
		`INSERT INTO chunks (chunk, start, end) VALUES (?, ?, ?)
		ON CONFLICT DO UPDATE SET chunk=excluded.chunk, start=excluded.start, end=excluded.end`,
		chunk, start, end)

	return errors.WithStack(err)
}

func (s *SqlLiteDatabase) GetChunkRange(blockNumber uint64) (chunk, start, end uint64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	err = s.db.
		QueryRow(
			"SELECT chunk, start, end FROM chunks WHERE start <= ? AND end >= ?",
			blockNumber, blockNumber).
		Scan(&chunk, &start, &end)

	err = errors.WithStack(err)

	return
}

func (s *SqlLiteDatabase) AddTxsForBlock(txhashes []string, blockNumber uint64) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return errors.WithStack(err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("INSERT OR REPLACE INTO txs (txhash, block_number) VALUES (?, ?)")
	if err != nil {
		return errors.WithStack(err)
	}
	defer stmt.Close()

	for _, txhash := range txhashes {
		_, err = stmt.Exec(txhash, blockNumber)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	err = tx.Commit()
	return errors.WithStack(err)
}

func (s *SqlLiteDatabase) GetBlockNumForTx(txhash string) (blockNumber uint64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	err = s.db.QueryRow("SELECT block_number FROM txs WHERE txhash = ?", txhash).Scan(&blockNumber)
	err = errors.WithStack(err)

	return
}

func (s *SqlLiteDatabase) GetBlockNumForHash(blockHash string) (blockNumber uint64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	err = s.db.QueryRow("SELECT number FROM points WHERE hash = ?", blockHash).Scan(&blockNumber)
	err = errors.WithStack(err)

	return
}

func (s *SqlLiteDatabase) GetChunkSpan() (lowestChunk, highestChunk uint64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
		SELECT 
			COALESCE(MIN(chunk), 0) as lowest_chunk,
			COALESCE(MAX(chunk), 0) as highest_chunk
		FROM chunks
	`

	err = s.db.QueryRow(query).Scan(&lowestChunk, &highestChunk)
	err = errors.WithStack(err)

	return
}

func (s *SqlLiteDatabase) GetChunkedBlockSpan() (lowestBlock, highestBlock uint64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
		SELECT 
			COALESCE(MIN(start), 0) as lowest_block,
			COALESCE(MAX(end), 0) as highest_block
		FROM chunks
	`

	err = s.db.QueryRow(query).Scan(&lowestBlock, &highestBlock)
	err = errors.WithStack(err)

	return
}

func (s *SqlLiteDatabase) GetPointSpan() (lowestBlock, highestBlock uint64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
		SELECT 
			COALESCE(MIN(number), 0) as lowest_block,
			COALESCE(MAX(number), 0) as highest_block
		FROM points
	`

	err = s.db.QueryRow(query).Scan(&lowestBlock, &highestBlock)
	err = errors.WithStack(err)

	return
}

func (s *SqlLiteDatabase) AddBlockPoint(p PointAndBlockNum) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err = s.db.Exec(
		"INSERT OR REPLACE INTO points (number, slot, hash) VALUES (?, ?, ?)",
		p.Block,
		p.Point.Slot,
		p.Point.Hash.String(),
	)

	return errors.WithStack(err)
}

func (s *SqlLiteDatabase) GetBlockPoint(blockNumber uint64) (p PointAndBlockNum, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var hash string

	err = s.db.
		QueryRow("SELECT slot, hash FROM points WHERE number = ?", blockNumber).
		Scan(&p.Point.Slot, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		err = errors.Wrap(ErrBlockNotFound, "block point not found")
		return
	} else if err != nil {
		err = errors.WithStack(err)
		return
	}

	p.Block = blockNumber
	p.Point.Hash = HexString(hash).Bytes()

	return
}
