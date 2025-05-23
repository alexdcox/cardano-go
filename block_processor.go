package cardano

import (
	"database/sql"
	"sync"
	"sync/atomic"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	"github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

const (
	BlockBatchSize = 10000
)

type BlockProcessor struct {
	db              Database
	ntn             *ouroboros.Connection
	isProcessing    atomic.Bool
	processingMutex sync.Mutex
	log             *zerolog.Logger
	blockChan       chan ledger.Block
	highestPoint    PointRef
	reorgWindow     int
}

func NewBlockProcessor(db Database, logger *zerolog.Logger, reorgWindow int) (bp *BlockProcessor, err error) {
	highestPoint, err := db.GetHighestPoint()
	if errors.Is(err, sql.ErrNoRows) {
		// no previous point, at block height 0
		err = nil
	} else if err != nil {
		return
	}

	logger.Info().Msgf("block processor set to trail node tip by a reorganisation window of %d blocks", reorgWindow)

	bp = &BlockProcessor{
		db:           db,
		log:          logger,
		highestPoint: highestPoint,
		reorgWindow:  reorgWindow,
	}

	return
}

func (bp *BlockProcessor) TryProcessBlocks() {
	if !bp.isProcessing.CompareAndSwap(false, true) {
		bp.log.Debug().Msg("block processing already running")
		return
	}

	go func() {
		defer bp.isProcessing.Store(false)

		if err := bp.processBlocks(); err != nil {
			bp.log.Error().Err(err).Msg("error processing blocks")
		}
	}()
}

func (bp *BlockProcessor) onBlock(_ blockfetch.CallbackContext, _ uint, block ledger.Block) error {
	bp.blockChan <- block
	return nil
}

func (bp *BlockProcessor) onBatchDone(_ blockfetch.CallbackContext) error {
	close(bp.blockChan)
	return nil
}

func (bp *BlockProcessor) processBlocks() error {
	bp.processingMutex.Lock()
	defer bp.processingMutex.Unlock()

	for {
		startTime := time.Now()

		points, err := bp.db.GetPointsForProcessing(BlockBatchSize + bp.reorgWindow)
		if err != nil {
			return errors.Wrap(err, "failed to get points for processing")
		}

		if bp.reorgWindow > 0 && len(points)-bp.reorgWindow <= 0 {
			return nil
		}

		points = points[:len(points)-bp.reorgWindow]

		bp.log.Info().
			Int("count", len(points)).
			Uint64("first_slot", points[0].Slot).
			Uint64("last_slot", points[len(points)-1].Slot).
			Msg("processing blocks")

		height := bp.highestPoint.Height
		if bp.highestPoint.Hash != "" {
			height++
		}

		var prevHash, prevBoundaryHash string
		if height > 0 {
			prevHash = bp.highestPoint.Hash

			prevBoundaryPoint, err := bp.db.GetBoundaryPointBehind(bp.highestPoint.Hash)
			if err == nil {
				prevBoundaryHash = prevBoundaryPoint.Hash
			} else if !errors.Is(err, sql.ErrNoRows) {
				return errors.Wrap(err, "failed to get previous boundary point")
			}
		}

		bp.blockChan = make(chan ledger.Block, BlockBatchSize)
		errChan := make(chan error, 1)
		doneChan := make(chan PointRef, 1)

		startPoint := common.NewPoint(points[0].Slot, HexString(points[0].Hash).Bytes())
		endPoint := common.NewPoint(points[len(points)-1].Slot, HexString(points[len(points)-1].Hash).Bytes())

		go func() {
			if err := bp.ntn.BlockFetch().Client.GetBlockRange(startPoint, endPoint); err != nil {
				errChan <- errors.Wrap(err, "failed to fetch block range")
				close(bp.blockChan)
				return
			}
		}()

		var pointRefs []PointRef
		var txRefs []TxRef

		go func() {
			var maxPoint PointRef
			for block := range bp.blockChan {
				isBoundary := block.Type() == 0

				var blockPrevHash string
				var blockHash string

				blockHash = block.Hash()
				blockPrevHash = block.PrevHash()

				if height > 0 {
					if !isBoundary && blockPrevHash != prevHash && blockPrevHash != prevBoundaryHash {
						errChan <- errors.Errorf("block chain broken: expected prev hash %s or boundary %s, got %s",
							prevHash, prevBoundaryHash, blockPrevHash)
						return
					}
				}

				for _, tx := range block.Transactions() {
					txRefs = append(txRefs, TxRef{
						Hash:        tx.Hash(),
						BlockHeight: height,
					})
				}

				updatedPoint := PointRef{
					Hash:   blockHash,
					Height: height,
					Slot:   block.SlotNumber(),
					Type:   block.Type(),
				}
				pointRefs = append(pointRefs, updatedPoint)
				maxPoint = updatedPoint

				if isBoundary {
					if height == 0 {
						height++
					}
					prevBoundaryHash = blockHash
				} else {
					height++
					prevHash = blockHash
				}
			}
			doneChan <- maxPoint
			close(doneChan)
		}()

		// Wait for completion or error
		select {
		case err := <-errChan:
			return err
		case latestHeight := <-doneChan:
			if err := bp.db.SetPointHeights(pointRefs); err != nil {
				return errors.Wrap(err, "failed to update point heights")
			}

			if err := bp.db.AddTxsForBlock(txRefs); err != nil {
				return errors.Wrap(err, "failed to store transactions")
			}

			bp.highestPoint = latestHeight
		}

		bp.log.Debug().Msgf("batch took %s", time.Since(startTime))
	}

	return nil
}

func (bp *BlockProcessor) HandleRollback(rollbackSlot uint64) error {
	bp.processingMutex.Lock()
	defer bp.processingMutex.Unlock()

	var highestSlot uint64
	highestPoint, err := bp.db.GetHighestPoint()
	if errors.Is(err, sql.ErrNoRows) {
		// no previous point, at block height 0
	} else if err != nil {
		return errors.Wrap(err, "failed to get highest point")
	} else {
		highestSlot = highestPoint.Slot
	}

	if rollbackSlot < highestSlot {
		bp.log.Info().
			Uint64("rollback_slot", rollbackSlot).
			Uint64("highest_slot", highestSlot).
			Msg("rolling back to slot")

		if err := bp.db.HandleRollback(rollbackSlot); err != nil {
			return errors.Wrap(err, "failed to handle rollback")
		}

		rollbackPoint, err2 := bp.db.GetHighestPoint()
		if err2 != nil {
			return errors.Wrap(err, "failed to fetch highest point after rollback")
		} else {
			bp.highestPoint = rollbackPoint
		}
	}

	return nil
}
