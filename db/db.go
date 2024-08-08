package db

import (
	"database/sql"
	"sync"

	"github.com/alexdcox/cardano-go"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
)

type Database interface {
	SetChunkRange(chunk, start, end uint64) (err error)
	GetChunkRange(number uint64) (start, end uint64, chunk uint64, err error)
	GetChunkSpan() (first, last uint64, err error)

	AddTxsForBlock(txhashes []string, blockNumber uint64) (err error)
	GetBlockForTx(txhash string) (blockNumber uint64, err error)

	SetTip(tip cardano.Tip) error
	GetTip() (cardano.Tip, error)

	AddBlockPoint(number uint64, point cardano.Point) (err error)
	GetBlockPoint(number uint64) (point cardano.Point, err error)
}

type SqlLiteDatabase struct {
	db *sql.DB
	mu sync.Mutex
}

var _ Database = &SqlLiteDatabase{}

func NewSqlLiteDatabase(path string) (db *SqlLiteDatabase, err error) {
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

func (s *SqlLiteDatabase) GetChunkRange(number uint64) (chunk, start, end uint64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	err = s.db.
		QueryRow(
			"SELECT chunk, start, end FROM chunks WHERE start <= ? AND end >= ?",
			number, number).
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

func (s *SqlLiteDatabase) GetBlockForTx(txhash string) (blockNumber uint64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	err = s.db.QueryRow("SELECT block_number FROM txs WHERE txhash = ?", txhash).Scan(&blockNumber)
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

func (s *SqlLiteDatabase) SetTip(tip cardano.Tip) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return errors.WithStack(err)
	}
	defer tx.Rollback()

	_, err = tx.Exec("DELETE FROM tip")
	if err != nil {
		return errors.WithStack(err)
	}

	_, err = tx.Exec(`
        INSERT INTO tip (slot, hash, number)
        VALUES (?, ?, ?)`,
		tip.Point.Slot, tip.Point.Hash, tip.Block)
	if err != nil {
		return errors.WithStack(err)
	}

	err = tx.Commit()
	return errors.WithStack(err)
}

func (s *SqlLiteDatabase) GetTip() (cardano.Tip, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var tip cardano.Tip
	err := s.db.QueryRow(`
        SELECT slot, hash, number
        FROM tip
        LIMIT 1`).Scan(&tip.Point.Slot, &tip.Point.Hash, &tip.Block)

	if err == sql.ErrNoRows {
		return cardano.Tip{
			Point: cardano.Point{
				Slot: 0,
				Hash: make(cardano.HexBytes, 32),
			},
			Block: 0,
		}, nil
	}

	return tip, errors.WithStack(err)
}

func (s *SqlLiteDatabase) AddBlockPoint(number uint64, point cardano.Point) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err = s.db.Exec(
		"INSERT OR REPLACE INTO points (number, slot, hash) VALUES (?, ?, ?)",
		number, point.Slot, point.Hash.String())

	return errors.WithStack(err)
}

func (s *SqlLiteDatabase) GetBlockPoint(number uint64) (point cardano.Point, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// TODO: continue from here
	err = s.db.QueryRow(
		"SELECT slot, hash FROM points WHERE number = ?",
		number).Scan(&point.Slot, &point.Hash)

	if err == sql.ErrNoRows {
		err = errors.New("block point not found")
	}

	return point, errors.WithStack(err)
}
