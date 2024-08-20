package cardano

import (
	"os"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
)

func TestDatabase(t *testing.T) {
	defer func() {
		_ = os.Remove("cardano-test.db")
	}()

	db, err := NewSqlLiteDatabase("cardano-test.db")
	assert.Nil(t, err)

	// Chunks

	first, last, err := db.GetChunkSpan()
	assert.Nil(t, err)
	assert.Equal(t, uint64(0), first)
	assert.Equal(t, uint64(0), last)

	err = db.SetChunkRange(0, 1, 10)
	assert.Nil(t, err)
	err = db.SetChunkRange(1, 11, 20)
	assert.Nil(t, err)

	err = db.SetChunkRange(1, 21, 30)
	assert.Nil(t, err, "should be able to update a chunk range")

	err = db.SetChunkRange(9, 0, 0)
	assert.Error(t, err, "expected error creating zero length chunk range")

	err = db.SetChunkRange(9, 2, 1)
	assert.Error(t, err, "expected error creating inverted chunk range")

	err = db.SetChunkRange(10, 29, 31)
	assert.Error(t, err, "expected error creating overlapping chunk range (start)")
	err = db.SetChunkRange(10, 20, 22)
	assert.Error(t, err, "expected error creating overlapping chunk range (end)")

	err = db.SetChunkRange(2, 11, 20)
	assert.Nil(t, err)

	err = db.SetChunkRange(3, 31, 40)
	assert.Nil(t, err)

	first, last, err = db.GetChunkSpan()
	assert.Nil(t, err)
	assert.Equal(t, uint64(0), first)
	assert.Equal(t, uint64(3), last)

	chunk, chunkStart, chunkEnd, err := db.GetChunkRange(13)
	assert.Nil(t, err)
	assert.Equal(t, uint64(2), chunk)
	assert.Equal(t, uint64(11), chunkStart)
	assert.Equal(t, uint64(20), chunkEnd)

	// Tx

	err = db.AddTxsForBlock([]string{"somehashA", "somehashB"}, 20)
	assert.Nil(t, err)

	height, err := db.GetBlockForTx("somehashA")
	assert.Nil(t, err)
	assert.Equal(t, uint64(20), height)

	height, err = db.GetBlockForTx("somehashB")
	assert.Nil(t, err)
	assert.Equal(t, uint64(20), height)

	// Tip

	err = db.SetTip(PointAndBlockNum{
		Point: Point{
			Slot: 1,
			Hash: HexBytes("somehash"),
		},
		Block: 2,
	})
	assert.Nil(t, err)

	tip, err := db.GetTip()
	assert.Nil(t, err)
	assert.Equal(t, uint64(1), tip.Point.Slot)
	assert.Equal(t, HexBytes("somehash"), tip.Point.Hash)
	assert.Equal(t, uint64(2), tip.Block)
}
