package cardano

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

func NewChunkReader(dir string, db Database) (reader *ChunkReader, err error) {
	reader = &ChunkReader{
		dir:          dir,
		immutableDir: path.Join(dir, "./immutable/"),
		db:           db,
		cache:        NewChunkCache(time.Second * 10),
	}

	return
}

type Chunk struct {
	FirstBlock uint64
	LastBlock  uint64
	Blocks     []*Block
	ChunkPath  string
	Number     uint64
}

type ChunkReader struct {
	dir          string
	immutableDir string
	cache        *ChunkCache
	db           Database
}

func (r *ChunkReader) Start() (err error) {
	_, lastSavedChunk, err := r.db.GetChunkSpan()
	if err != nil {
		return
	}

	r.WaitForReady()

	err = r.LoadChunkFiles(lastSavedChunk)
	if err != nil {
		return
	}

	go func() {
		if err := r.WatchChunkFiles(); err != nil {
			log.Fatal().Msgf("error while watching chunk files: %+v", err)
		}
	}()

	return
}

func (r *ChunkReader) writeIndexForChunk(chunk *Chunk) (err error) {
	err = r.db.SetChunkRange(chunk.Number, chunk.FirstBlock, chunk.LastBlock)
	if err != nil {
		return
	}

	log.Info().Msgf(
		"indexed chunk %d block range %d to %d (%d blocks)",
		chunk.Number,
		chunk.FirstBlock,
		chunk.LastBlock,
		chunk.LastBlock-chunk.FirstBlock,
	)

	return
}

func (r *ChunkReader) LoadChunkFiles(skipUntilNumber uint64) (err error) {
	log.Info().Msgf("locating chunk files (onwards from chunk %d)...", skipUntilNumber)

	var startChunk, endChunk uint64
	err = filepath.Walk(r.immutableDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(info.Name(), ".chunk") {
			number, err2 := r.chunkFileNumber(info.Name())
			if err2 != nil {
				return errors.WithStack(err2)
			}
			if number < startChunk {
				startChunk = number
			}
			if number > endChunk {
				endChunk = number
			}
		}
		return nil
	})
	if err != nil {
		log.Fatal().Msgf("error walking through directory: %v", err)
	}

	conwayBlock, conwayChunk, err := r.findFirstConwayBlock(startChunk, endChunk)
	if err != nil && !errors.Is(err, ErrBlockNotFound) {
		return
	}

	if conwayBlock != nil {
		log.Info().Msgf("found conway block at %d in chunk %d", conwayBlock.Data.Header.Body.Number, conwayChunk.Number)
		if conwayChunk.Number > startChunk {
			startChunk = conwayChunk.Number
		}

		if startChunk < skipUntilNumber {
			startChunk = skipUntilNumber
		}
	} else {
		log.Warn().Msg("unable to find conway block in chunk files")
	}

	if endChunk > startChunk {
		log.Info().Msgf("loading chunk files ranging from %d to %d...", startChunk, endChunk)
	} else {
		log.Info().Msg("no new chunk files to load")
	}

	var previousChunk *Chunk

	for chunkNumber := startChunk; chunkNumber <= endChunk; chunkNumber++ {
		chunk, err2 := r.processChunkFile(chunkNumber, chunkNumber != endChunk)
		if errors.Is(err2, ErrEraBeforeConway) {
			continue
		}
		if err2 != nil {
			return err2
		}
		if chunk.Blocks == nil {
			continue
		}

		if previousChunk == nil {
			previousChunk = chunk
			continue
		}

		previousChunk.LastBlock = chunk.FirstBlock - 1

		if err = r.writeIndexForChunk(previousChunk); err != nil {
			return
		}

		previousChunk = chunk

		if chunkNumber == endChunk {
			if err = r.writeIndexForChunk(chunk); err != nil {
				return
			}
		}
	}

	firstBlock, lastBlock, err := r.db.GetChunkedBlockSpan()
	if err != nil {
		return
	}
	blockRange := lastBlock - firstBlock + 1

	p := message.NewPrinter(language.English)
	log.Info().Msgf("found %s chunked blocks", p.Sprintf("%d", blockRange))

	return
}

func (r *ChunkReader) WatchChunkFiles() (err error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal().Msgf("%+v", err)
	}
	defer func() {
		if err2 := watcher.Close(); err2 != nil {
			log.Error().Msgf("error closing chunk watcher: %+v", err2)
		}
	}()

	done := make(chan bool)
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					chunkNumber, err2 := r.chunkFileNumber(event.Name)
					if err != nil {
						err = errors.WithStack(err2)
						return
					}
					chunk, err2 := r.processChunkFile(chunkNumber, false)
					if errors.Is(err2, ErrNotChunkFile) {
						continue
					}
					if err2 != nil {
						log.Fatal().Msgf("%+v", errors.WithStack(err2))
					}
					// fmt.Println(event.Name)
					// for x := chunk.FirstBlock; x <= chunk.LastBlock; x++ {
					// 	log.Info().Msgf("watch %d -> %s", x, chunk.ChunkPath)
					// 	r.numberPathMap[x] = &chunk.ChunkPath
					// }

					// TODO: Same question as above: save tx hashes here?

					err = r.db.SetChunkRange(chunkNumber, chunk.FirstBlock, chunk.LastBlock)
					if err != nil {
						return
					}

					log.Info().Msgf(
						"reloaded chunk %d, block range %d to %d (%d blocks)",
						chunkNumber,
						chunk.FirstBlock,
						chunk.LastBlock,
						chunk.LastBlock-chunk.FirstBlock,
					)
				}
			case err2, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Error().Msgf("error in chunk watcher: %+v", err2)
			}
		}
	}()

	log.Info().Msgf("watching directory '%s' for new chunk data...", r.immutableDir)
	err = watcher.Add(r.immutableDir)
	if err != nil {
		log.Fatal().Msgf("%+v", err)
	}
	<-done

	return
}

func (r *ChunkReader) GetBlock(blockNumber uint64) (block *Block, err error) {
	findBlockInChunk := func(chunk *Chunk) (*Block, error) {
		for _, b := range chunk.Blocks {
			if b.Data.Header.Body.Number == blockNumber {
				return b, nil
			}
		}
		return nil, errors.Wrapf(ErrChunkBlockMissing, "requested chunk: %s", chunk.ChunkPath)
	}

	if cachedChunk, ok := r.cache.Get(blockNumber); ok {
		return findBlockInChunk(cachedChunk)
	}

	chunkNumber, _, _, err := r.db.GetChunkRange(blockNumber)
	if err != nil {
		return
	}

	chunk, err := r.processChunkFile(chunkNumber, false)
	if err != nil {
		return
	}

	block, err = findBlockInChunk(chunk)
	if err != nil {
		err = errors.Wrapf(ErrBlockNotFound, "requested block: %d", blockNumber)
		return
	}

	r.cache.Set(chunk)
	return
}

func (r *ChunkReader) chunkFileNumber(chunkPath string) (number uint64, err error) {
	numberStr := filepath.Base(chunkPath)[:len(filepath.Base(chunkPath))-6] // Remove ".chunk"
	number, err = strconv.ParseUint(numberStr, 10, 64)
	if err != nil {
		err = errors.WithStack(err)
	}
	return
}

func (r *ChunkReader) processChunkFile(chunkNumber uint64, firstBlockOnly bool) (c *Chunk, err error) {
	chunkFile := fmt.Sprintf("%05d.chunk", chunkNumber)
	chunkPath := path.Join(r.immutableDir, chunkFile)

	if filepath.Ext(chunkPath) != ".chunk" {
		err = errors.WithStack(ErrNotChunkFile)
		return
	}

	re := regexp.MustCompile(`^\d+\.chunk$`)
	if !re.MatchString(filepath.Base(chunkPath)) {
		return
	}

	data, err := os.ReadFile(chunkPath)
	if err != nil {
		err = errors.Wrap(err, "error reading chunk file")
		return
	}

	c, err = r.readChunkData(data, firstBlockOnly)
	if err != nil {
		return
	}

	c.ChunkPath = chunkPath
	c.Number = chunkNumber

	return
}

func (r *ChunkReader) readChunkData(data []byte, firstBlockOnly bool) (out *Chunk, err error) {
	out = &Chunk{}

	for {
		if len(data) == 0 {
			break
		}

		var a []any
		rest, err2 := StandardCborDecoder.UnmarshalFirst(data, &a)
		if err2 != nil {
			err = errors.WithStack(err2)
			return
		}

		if a[0].(uint64) < uint64(EraConway) {
			data = rest
			if firstBlockOnly {
				err = errors.WithStack(ErrEraBeforeConway)
				return
			}
			continue
		}

		block := &Block{}

		remaining, err2 := StandardCborDecoder.UnmarshalFirst(data, block)
		if err2 != nil {
			err = errors.Wrap(err2, "failed to read chunk")
			return
		}

		block.Raw = data[:len(data)-len(remaining)]

		blockBody := block.Data.Header.Body

		if len(out.Blocks) == 0 {
			out.FirstBlock = blockBody.Number
		}

		if blockBody.Number < out.FirstBlock {
			out.FirstBlock = blockBody.Number
		}

		if blockBody.Number > out.LastBlock {
			out.LastBlock = blockBody.Number
		}

		out.Blocks = append(out.Blocks, block)

		if len(remaining) == len(data) {
			log.Info().Msg("seems like we got it all")
			break
		}

		if firstBlockOnly {
			return
		}

		data = remaining
	}

	return
}

func (r *ChunkReader) Ready() error {
	if !DirectoryExists(r.immutableDir) {
		return errors.WithStack(ErrDataDirectoryNotFound)
	}
	return nil
}

func (r *ChunkReader) WaitForReady() {
	for {
		if err := r.Ready(); err != nil {
			if errors.Is(err, ErrDataDirectoryNotFound) {
				log.Warn().Msgf("cardano data directory '%s' does not exist, waiting...", r.immutableDir)
			} else {
				log.Fatal().Msgf("%+v", err)
			}
			time.Sleep(time.Second)
		} else {
			break
		}
	}
}

func (r *ChunkReader) findFirstConwayBlock(startChunk, endChunk uint64) (conwayBlock *Block, conwayChunk *Chunk, err error) {
	log.Info().Msgf("locating first conway chunk/block (in %d chunks)...", endChunk-startChunk+1)

	var conwayChunkNum uint64

	if endChunk > startChunk {
		err = BinarySearchCallback(startChunk, endChunk, func(chunkNumber uint64) (search int, err error) {
			hasBlocks := func(chunkNumber uint64) bool {
				if chunk, err2 := r.processChunkFile(chunkNumber, true); err2 == nil {
					return chunk.Blocks != nil
				}
				return false
			}

			if hasBlocks(chunkNumber) {
				if hasBlocks(chunkNumber - 1) {
					return -1, nil
				} else {
					conwayChunkNum = chunkNumber
					return 0, nil
				}
			}

			return 1, nil
		})
		if err != nil {
			err = errors.Wrap(ErrBlockNotFound, "failed to find conway chunk")
			return
		}
	}

	conwayChunk, err = r.processChunkFile(conwayChunkNum, false)
	if err != nil {
		return
	}

	if len(conwayChunk.Blocks) < 1 {
		err = errors.Wrap(ErrBlockNotFound, "conway block not found")
		return
	}

	conwayBlock = conwayChunk.Blocks[0]

	return
}

type ChunkCache struct {
	data     map[uint64]*chunkCacheEntry
	duration time.Duration
	mu       sync.Mutex
}

type chunkCacheEntry struct {
	value *Chunk
	timer *time.Timer
}

func NewChunkCache(duration time.Duration) *ChunkCache {
	return &ChunkCache{
		data:     make(map[uint64]*chunkCacheEntry),
		duration: duration,
	}
}

func (c *ChunkCache) Set(chunk *Chunk) {
	log.Info().Msgf("loading chunk %d into cache", chunk.Number)

	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.data[chunk.Number]; exists {
		entry.timer.Stop()
	}

	timer := time.AfterFunc(c.duration, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		chunk.Blocks = []*Block{}
		for x := chunk.FirstBlock; x <= chunk.LastBlock; x++ {
			delete(c.data, x)
		}
		log.Info().Msgf("removing chunk %d from cache", chunk.Number)
	})

	for x := chunk.FirstBlock; x <= chunk.LastBlock; x++ {
		c.data[x] = &chunkCacheEntry{
			value: chunk,
			timer: timer,
		}
	}
}

func (c *ChunkCache) Get(number uint64) (*Chunk, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.data[number]; exists {
		entry.timer.Reset(c.duration)
		return entry.value, true
	}

	return nil, false
}
