package db

import (
	"database/sql"
	"fmt"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

type Database interface {
	AddBlockRange(chunk, start, end uint64)
	GetBlockRange(number uint64) (start, end uint64, chunk uint64, err error)
	AddTxBlockMap(txhash string, blockNumber uint64)
	GetBlockForTx(txhash string) (blockNumber uint64, err error)
	GetChunkRange() (start, end uint64, err error)
}

type SqlLiteDatabase struct {
	db *sql.DB
	mu sync.Mutex
}

func NewSqlLiteDatabase() (db *SqlLiteDatabase, err error) {
	sqldb, err := sql.Open("sqlite3", "cardano.db")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := sqldb.Ping(); err != nil {
		sqldb.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db = &SqlLiteDatabase{db: sqldb}
	if err := db.initTables(); err != nil {
		sqldb.Close()
		return nil, fmt.Errorf("failed to initialize tables: %w", err)
	}

	return db, nil
}

func (s *SqlLiteDatabase) initTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS block_ranges (
			chunk INTEGER PRIMARY KEY,
			start INTEGER,
			end INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS tx_block_map (
			txhash TEXT PRIMARY KEY,
			block_number INTEGER
		)`,
	}

	for _, query := range queries {
		_, err := s.db.Exec(query)
		if err != nil {
			return fmt.Errorf("failed to execute query %s: %w", query, err)
		}
	}

	return nil
}

func (s *SqlLiteDatabase) AddBlockRange(chunk, start, end uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("INSERT OR REPLACE INTO block_ranges (chunk, start, end) VALUES (?, ?, ?)",
		chunk, start, end)
	if err != nil {
		// Log the error or handle it as appropriate for your application
		fmt.Printf("Error adding block range: %v\n", err)
	}
}

func (s *SqlLiteDatabase) GetBlockRange(number uint64) (chunk, start, end uint64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	err = s.db.QueryRow("SELECT chunk, start, end FROM block_ranges WHERE start <= ? AND end >= ?",
		number, number).Scan(&chunk, &start, &end)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, 0, 0, fmt.Errorf("no block range found for number %d", number)
		}
		return 0, 0, 0, fmt.Errorf("error querying block range: %w", err)
	}

	return chunk, start, end, nil
}

func (s *SqlLiteDatabase) AddTxBlockMap(txhash string, blockNumber uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("INSERT OR REPLACE INTO tx_block_map (txhash, block_number) VALUES (?, ?)",
		txhash, blockNumber)
	if err != nil {
		// Log the error or handle it as appropriate for your application
		fmt.Printf("Error adding tx block map: %v\n", err)
	}
}

func (s *SqlLiteDatabase) GetBlockForTx(txhash string) (blockNumber uint64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	err = s.db.QueryRow("SELECT block_number FROM tx_block_map WHERE txhash = ?", txhash).Scan(&blockNumber)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("no block number found for txhash %s", txhash)
		}
		return 0, fmt.Errorf("error querying block number: %w", err)
	}

	return blockNumber, nil
}

func (s *SqlLiteDatabase) GetChunkRange() (lowestChunk, highestChunk uint64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
		SELECT 
			COALESCE(MIN(chunk), 0) as lowest_chunk,
			COALESCE(MAX(chunk), 0) as highest_chunk
		FROM block_ranges
	`

	err = s.db.QueryRow(query).Scan(&lowestChunk, &highestChunk)
	if err != nil {
		return 0, 0, fmt.Errorf("error querying chunk range: %w", err)
	}

	return lowestChunk, highestChunk, nil
}

var _ Database = &SqlLiteDatabase{}
