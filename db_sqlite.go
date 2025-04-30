package cardano

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"

	_ "github.com/mattn/go-sqlite3"
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
		`CREATE TABLE IF NOT EXISTS tx (
			txhash TEXT PRIMARY KEY,
			block_number INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS point (
			slot INTEGER,
			hash TEXT,
			type INTEGER,
			height INTEGER DEFAULT -1,
			PRIMARY KEY (slot, hash)
		)`,
		`CREATE TABLE IF NOT EXISTS point_rollback (
			slot INTEGER,
			hash TEXT,
			height INTEGER,
			rollback_time DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (slot, hash)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_point_height ON point(height)`,
		`CREATE INDEX IF NOT EXISTS idx_point_slot ON point(slot)`,
		`CREATE INDEX IF NOT EXISTS idx_point_hash ON point(hash)`,
		`CREATE INDEX IF NOT EXISTS idx_rollback_slot ON point_rollback(slot)`,
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

// HandleRollback moves points to the rollback table and cleans up related data
func (s *SqlLiteDatabase) HandleRollback(fromSlot uint64) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return errors.WithStack(err)
	}
	defer tx.Rollback()

	// Move points to rollback table
	_, err = tx.Exec(`
		INSERT INTO point_rollback (slot, hash, height)
		SELECT slot, hash, height
		FROM point
		WHERE slot >= ?`,
		fromSlot)
	if err != nil {
		return errors.WithStack(err)
	}

	// Delete points that were rolled back
	_, err = tx.Exec("DELETE FROM point WHERE slot >= ?", fromSlot)
	if err != nil {
		return errors.WithStack(err)
	}

	// Clean up any transactions from rolled back blocks
	_, err = tx.Exec(`
		DELETE FROM tx 
		WHERE block_number IN (
			SELECT height 
			FROM point_rollback 
			WHERE slot >= ? AND height >= 0
		)`, fromSlot)
	if err != nil {
		return errors.WithStack(err)
	}

	err = tx.Commit()
	return errors.WithStack(err)
}

// GetPointsForProcessing returns points ready for block fetching, respecting BlocksUntilConfirmed-distance from tip
func (s *SqlLiteDatabase) GetPointsForProcessing(batchSize int) (points []PointRef, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`
		SELECT slot, hash, type 
		FROM point 
		WHERE height = -1 
		ORDER BY slot ASC 
		LIMIT ?`,
		batchSize)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer rows.Close()

	points = make([]PointRef, 0)
	for rows.Next() {
		var point PointRef
		if err = rows.Scan(&point.Slot, &point.Hash, &point.Type); err != nil {
			return nil, errors.WithStack(err)
		}
		points = append(points, point)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.WithStack(err)
	}

	return
}

func (s *SqlLiteDatabase) AddTxsForBlock(refs []TxRef) (err error) {
	if len(refs) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return errors.WithStack(err)
	}

	const chunkSize = 499 // Stay under SQLite's parameter limit (999/2)

	for i := 0; i < len(refs); i += chunkSize {
		end := i + chunkSize
		if end > len(refs) {
			end = len(refs)
		}

		chunk := refs[i:end]
		valueStrings := make([]string, len(chunk))
		valueArgs := make([]interface{}, 0, len(chunk)*2)

		for j := range chunk {
			valueStrings[j] = "(?, ?)"
			valueArgs = append(valueArgs, chunk[j].Hash, chunk[j].BlockHeight)
		}

		stmt := fmt.Sprintf(
			"INSERT INTO tx (txhash, block_number) VALUES %s",
			strings.Join(valueStrings, ","))

		if _, err = tx.Exec(stmt, valueArgs...); err != nil {
			return errors.WithStack(err)
		}
	}

	return errors.WithStack(tx.Commit())
}

func (s *SqlLiteDatabase) GetTxs(txHashes []string) (txs []TxRef, err error) {
	if len(txHashes) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	query := `SELECT txhash, block_number FROM tx WHERE txhash IN (?` + strings.Repeat(",?", len(txHashes)-1) + `)`

	// Convert []string to []interface{} for query parameters
	args := make([]interface{}, len(txHashes))
	for i, hash := range txHashes {
		args[i] = hash
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		err = errors.Wrap(err, "failed to query transactions")
		return
	}
	defer rows.Close()

	for rows.Next() {
		tx := TxRef{}
		if err2 := rows.Scan(&tx.Hash, &tx.BlockHeight); err != nil {
			err = errors.Wrap(err2, "failed to scan row")
			return
		}
		txs = append(txs, tx)
	}

	if err = rows.Err(); err != nil {
		err = errors.Wrap(err, "error during row iteration")
		return
	}

	return
}

func (s *SqlLiteDatabase) GetBlockNumForTx(txhash string) (blockNumber uint64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	err = s.db.QueryRow("SELECT block_number FROM tx WHERE txhash = ?", txhash).Scan(&blockNumber)
	if errors.Is(err, sql.ErrNoRows) {
		err = errors.Wrapf(ErrTransactionNotFound, "tx not found by hash %s", txhash)
		return
	}
	err = errors.WithStack(err)

	return
}

// func (s *SqlLiteDatabase) GetBlockNumForHash(blockHash string) (blockNumber uint64, err error) {
// 	s.mu.Lock()
// 	defer s.mu.Unlock()
//
// 	err = s.db.QueryRow("SELECT height FROM point WHERE hash = ? AND height >= 0", blockHash).Scan(&blockNumber)
// 	if errors.Is(err, sql.ErrNoRows) {
// 		err = errors.Wrapf(ErrBlockNotFound, "block not found by hash %s", blockHash)
// 		return
// 	}
// 	err = errors.WithStack(err)
//
// 	return
// }

func (s *SqlLiteDatabase) AddPoints(points []PointRef) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return errors.WithStack(err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO point (slot, hash, type, height) 
		VALUES (?, ?, ?, COALESCE((SELECT height FROM point WHERE slot = ? AND hash = ?), -1))
	`)
	if err != nil {
		return errors.WithStack(err)
	}
	defer stmt.Close()

	for _, point := range points {
		_, err = stmt.Exec(
			point.Slot,
			point.Hash,
			point.Type,
			point.Slot,
			point.Hash,
		)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	err = tx.Commit()
	return errors.WithStack(err)
}

func (s *SqlLiteDatabase) SetPointHeights(updates []PointRef) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return errors.WithStack(err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("UPDATE point SET height = ? WHERE hash = ?")
	if err != nil {
		return errors.WithStack(err)
	}
	defer stmt.Close()

	for _, update := range updates {
		_, err = stmt.Exec(update.Height, update.Hash)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	err = tx.Commit()
	return errors.WithStack(err)
}

// func (s *SqlLiteDatabase) GetHighestBlock() (height uint64, err error) {
// 	s.mu.Lock()
// 	defer s.mu.Unlock()
//
// 	err = s.db.QueryRow("SELECT COALESCE(MAX(height), 0) FROM point WHERE height >= 0").Scan(&height)
// 	if errors.Is(err, sql.ErrNoRows) {
// 		err = errors.Wrap(ErrPointNotFound, "no points with height")
// 		return
// 	}
// 	err = errors.WithStack(err)
//
// 	return
// }

func (s *SqlLiteDatabase) GetHighestPoint() (point PointRef, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	err = s.db.QueryRow(`
        SELECT slot, hash, type, height
        FROM point 
        WHERE height = (
            SELECT MAX(height) 
            FROM point 
            WHERE height >= 0
        ) AND type != 0
        LIMIT 1`,
	).Scan(&point.Slot, &point.Hash, &point.Type, &point.Height)

	err = errors.WithStack(err)

	return
}

func (s *SqlLiteDatabase) GetPointByHash(blockHash string) (point PointRef, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	err = s.db.QueryRow(
		"SELECT slot, hash, type, height FROM point WHERE hash = ?",
		blockHash,
	).Scan(&point.Slot, &point.Hash, &point.Type, &point.Height)

	if errors.Is(err, sql.ErrNoRows) {
		err = errors.Wrapf(ErrBlockNotFound, "point not found by block hash %s", blockHash)
		return
	} else if err != nil {
		err = errors.WithStack(err)
		return
	}

	return
}

func (s *SqlLiteDatabase) GetPointByHeight(height uint64) (point PointRef, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	err = s.db.QueryRow(
		"SELECT slot, hash, type, height FROM point WHERE height = ? AND type != 0",
		height,
	).Scan(&point.Slot, &point.Hash, &point.Type, &point.Height)

	if errors.Is(err, sql.ErrNoRows) {
		err = errors.Wrapf(ErrPointNotFound, "point not found by height %d", height)
		return
	} else if err != nil {
		err = errors.WithStack(err)
		return
	}

	return
}

func (s *SqlLiteDatabase) GetBoundaryPointBehind(blockHash string) (point PointRef, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// First get the slot of the provided block
	var targetSlot uint64
	err = s.db.QueryRow(
		"SELECT slot FROM point WHERE hash = ?",
		blockHash,
	).Scan(&targetSlot)

	if errors.Is(err, sql.ErrNoRows) {
		err = errors.Wrapf(ErrPointNotFound, "boundary point not found by behind hash %s", blockHash)
		return
	} else if err != nil {
		err = errors.WithStack(err)
		return
	}

	// Now find the closest unprocessed point before this slot
	err = s.db.QueryRow(`
        SELECT slot, hash, type, height 
        FROM point 
        WHERE slot < ? AND height = -1
        ORDER BY slot DESC 
        LIMIT 1`,
		targetSlot,
	).Scan(&point.Slot, &point.Hash, &point.Type, &point.Height)

	err = errors.WithStack(err)

	return
}

func (s *SqlLiteDatabase) GetPointsBySlot(slot uint64) (points []PointRef, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(
		"SELECT slot, hash, type FROM point WHERE slot = ?",
		slot,
	)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer rows.Close()

	points = make([]PointRef, 0)
	for rows.Next() {
		var point PointRef
		if err = rows.Scan(&point.Slot, &point.Hash, &point.Type); err != nil {
			return nil, errors.WithStack(err)
		}
		points = append(points, point)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.WithStack(err)
	}

	if len(points) == 0 {
		err = errors.Wrap(ErrBlockNotFound, "no points found for slot")
		return
	}

	return
}

func (s *SqlLiteDatabase) GetPointsForLastSlot() (points []PointRef, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`
		SELECT slot, hash, type 
		FROM point 
		WHERE slot = (SELECT MAX(slot) FROM point)
	`)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer rows.Close()

	points = make([]PointRef, 0)
	for rows.Next() {
		var point PointRef
		if err = rows.Scan(&point.Slot, &point.Hash, &point.Type); err != nil {
			return nil, errors.WithStack(err)
		}
		points = append(points, point)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.WithStack(err)
	}

	if len(points) == 0 {
		err = errors.New("no points found in database")
		return
	}

	return
}
