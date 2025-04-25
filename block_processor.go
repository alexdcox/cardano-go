package cardano

import (
	"database/sql"
	"fmt"
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

		points, err := bp.db.GetPointsForProcessing(BlockBatchSize)
		if err != nil {
			return errors.Wrap(err, "failed to get points for processing")
		}

		if bp.reorgWindow > 0 && len(points) <= bp.reorgWindow {
			bp.log.Debug().Msg("not enough points for processing")
			break
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

		// if errors.Is(err, sql.ErrNoRows) {
		// 	// no previous point, at block height 0
		// } else if err != nil {
		// 	return errors.Wrap(err, "failed to get highest point")
		// } else {
		// 	height = highestPoint.Height + 1
		// }

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

		var updates []PointRef

		go func() {
			var maxPoint PointRef
			for block := range bp.blockChan {
				var isBoundary bool
				var blockPrevHash string
				var blockHash string

				blockHash = block.Hash()
				blockPrevHash = block.PrevHash()
				fmt.Printf("• [%d] %s\n", height, blockHash)

				if height > 0 {
					if !isBoundary && blockPrevHash != prevHash && blockPrevHash != prevBoundaryHash {
						errChan <- errors.Errorf("block chain broken: expected prev hash %s or boundary %s, got %s",
							prevHash, prevBoundaryHash, blockPrevHash)
						return
					}
				}

				var txHashes []string
				for _, tx := range block.Transactions() {
					fmt.Printf("+ %s\n", tx.Hash())
					txHashes = append(txHashes, tx.Hash())
				}

				if err := bp.db.AddTxsForBlock(txHashes, height); err != nil {
					errChan <- errors.Wrap(err, "failed to store transactions")
					return
				}

				updatedPoint := PointRef{
					Hash:   blockHash,
					Height: height,
					Slot:   block.SlotNumber(),
					Type:   block.Type(),
				}
				updates = append(updates, updatedPoint)
				maxPoint = updatedPoint

				if isBoundary {
					prevBoundaryHash = blockHash
				} else {
					prevHash = blockHash
				}

				height++
			}
			doneChan <- maxPoint
			close(doneChan)
		}()

		// Wait for completion or error
		select {
		case err := <-errChan:
			return err
		case latestHeight := <-doneChan:
			if err := bp.db.SetPointHeights(updates); err != nil {
				return errors.Wrap(err, "failed to update point heights")
			}
			bp.highestPoint = latestHeight
		}

		bp.log.Debug().Msgf("batch took %s", time.Since(startTime))
	}

	return nil
}

func (bp *BlockProcessor) HandleRollback(rollbackSlot uint64) error {
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
			Msg("chain reorg detected, rolling back")

		if err := bp.db.HandleRollback(rollbackSlot); err != nil {
			return errors.Wrap(err, "failed to handle rollback")
		}
	}

	return nil
}

// DetectChainSplit checks for and logs chain splits
func (bp *BlockProcessor) DetectChainSplit(slot uint64) error {
	isSplit, err := bp.db.DetectChainSplit(slot)
	if err != nil {
		return errors.Wrap(err, "failed to detect chain split")
	}

	if isSplit {
		bp.log.Warn().
			Uint64("slot", slot).
			Msg("chain split detected")
	}

	return nil
}
