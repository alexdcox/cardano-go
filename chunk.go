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
		cache:        NewChunkCache(time.Second * 30),
	}

	return
}

type ChunkReaderStatus struct {
	FirstBlock       uint64        `json:"firstBlock"`
	LastBlock        uint64        `json:"lastBlock"`
	TimeToStart      time.Duration `json:"timeToStart,string"`
	Started          time.Time     `json:"started"`
	FirstChunkNumber uint64        `json:"firstChunkNumber"`
	LastChunkNumber  uint64        `json:"lastChunkNumber"`
	LastChunkTime    time.Time     `json:"lastChunkTime"`
}

type Chunk struct {
	FirstBlock      uint64
	LastBlock       uint64
	BlockContainers []*ChunkedBlock
	ChunkPath       string
}

// TODO: Refactor this!
const FirstConwayChunk = 3348

type ChunkReader struct {
	dir          string
	immutableDir string
	Status       ChunkReaderStatus
	cache        *ChunkCache
	db           Database
}

type ChunkedBlock struct {
	block *Block
	data  []byte
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

func (r *ChunkReader) LoadChunkFiles(skipUntilNumber uint64) (err error) {
	r.Status.Started = time.Now().UTC()

	log.Info().Msg("locating chunk files...")

	var initialChunkFiles []string
	err = filepath.Walk(r.immutableDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(info.Name(), ".chunk") {
			initialChunkFiles = append(initialChunkFiles, path)
		}
		return nil
	})
	if err != nil {
		log.Fatal().Msgf("error walking through directory: %v", err)
	}

	firstNumber, _ := r.chunkFileNumber(initialChunkFiles[0])
	lastNumber, _ := r.chunkFileNumber(initialChunkFiles[len(initialChunkFiles)-1])

	log.Info().Msgf(
		"loading %d chunk files ranging from %d to %d...",
		len(initialChunkFiles),
		firstNumber,
		lastNumber,
	)

	var chunks []*Chunk
	for i, chunkFile := range initialChunkFiles {
		number, _ := r.chunkFileNumber(chunkFile)
		if number < FirstConwayChunk || number <= skipUntilNumber {
			continue
		}
		firstBlockOnly := true
		if i == len(initialChunkFiles)-1 {
			firstBlockOnly = false
		}
		chunk, err2 := r.processChunkFile(chunkFile, firstBlockOnly)
		if errors.Is(err2, ErrEraBeforeConway) {
			continue
		}
		if err2 != nil {
			log.Error().Msgf("%+v", err2)
		}
		if firstBlockOnly {
			chunk.BlockContainers = []*ChunkedBlock{}
		}
		chunks = append(chunks, chunk)

		i2 := len(chunks) - 1
		if i2 == 0 {
			continue
		}
		if i2 == len(initialChunkFiles)-1 {
			// TODO: Do we need to save tx hashes?

			chunks[i2].BlockContainers = []*ChunkedBlock{}

			chunkNumber := number - 1
			firstBlock := chunks[i2].FirstBlock
			lastBlock := chunks[i2].LastBlock - 1
			blockRange := chunks[i2].LastBlock - chunks[i2].FirstBlock

			err = r.db.SetChunkRange(chunkNumber, firstBlock, lastBlock)
			if err != nil {
				return
			}

			log.Info().Msgf(
				"loaded chunk %d, block range %d to %d (%d blocks)",
				chunkNumber,
				firstBlock,
				lastBlock,
				blockRange,
			)
			break
		}

		// TODO: Same question as above: save tx hashes here?

		chunkNumber := number - 1
		firstBlock := chunks[i2-1].FirstBlock
		lastBlock := chunks[i2].FirstBlock - 1
		blockRange := chunks[i2].FirstBlock - chunks[i2-1].FirstBlock

		err = r.db.SetChunkRange(chunkNumber, firstBlock, lastBlock)
		if err != nil {
			return
		}

		log.Info().Msgf(
			"loaded chunk %d, block range %d to %d (%d blocks)",
			chunkNumber,
			firstBlock,
			lastBlock,
			blockRange,
		)
	}

	r.Status.TimeToStart = time.Now().UTC().Sub(r.Status.Started)
	p := message.NewPrinter(language.English)

	firstBlock, lastBlock, err := r.db.GetChunkedBlockSpan()
	if err != nil {
		return
	}
	blockRange := lastBlock - firstBlock

	log.Info().Msgf("loaded %s blocks in %s", p.Sprintf("%d", blockRange), r.Status.TimeToStart.String())

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
					chunk, err2 := r.processChunkFile(event.Name, false)
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

					chunkNumber, err2 := r.chunkFileNumber(event.Name)
					if err2 != nil {
						log.Fatal().Msgf("%+v", errors.WithStack(err2))
					}

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
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("error:", err)
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

func (r *ChunkReader) GetChunkedBlock(blockNumber uint64) (block *ChunkedBlock, err error) {
	findBlockInChunk := func(chunk *Chunk) (*ChunkedBlock, error) {
		for _, b := range chunk.BlockContainers {
			if b.block.Data.Header.Body.Number == blockNumber {
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

	chunkFile := fmt.Sprintf("%05d.chunk", chunkNumber)
	chunkPath := path.Join(r.immutableDir, chunkFile)

	chunk, err := r.processChunkFile(chunkPath, false)
	if err != nil {
		return
	}

	block, err = findBlockInChunk(chunk)
	if err == nil {
		r.cache.Set(chunkNumber, chunk)
		return
	}

	return nil, errors.Wrapf(ErrBlockNotFound, "requested block: %d", blockNumber)
}

func (r *ChunkReader) GetBlock(blockNumber uint64) (block *Block, err error) {
	chunkedBlock, err := r.GetChunkedBlock(blockNumber)
	if err != nil {
		return
	}
	return chunkedBlock.block, nil
}

func (r *ChunkReader) chunkFileNumber(chunkPath string) (number uint64, err error) {
	numberStr := filepath.Base(chunkPath)[:len(filepath.Base(chunkPath))-6] // Remove ".chunk"
	number, err = strconv.ParseUint(numberStr, 10, 64)
	if err != nil {
		err = errors.WithStack(err)
	}
	return
}

func (r *ChunkReader) processChunkFile(chunkPath string, firstBlockOnly bool) (c *Chunk, err error) {
	if filepath.Ext(chunkPath) != ".chunk" {
		err = errors.WithStack(ErrNotChunkFile)
		return
	}

	re := regexp.MustCompile(`^\d+\.chunk$`)
	if !re.MatchString(filepath.Base(chunkPath)) {
		return
	}

	number, err := r.chunkFileNumber(chunkPath)
	if err != nil {
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

	if r.Status.FirstChunkNumber == 0 || number < r.Status.FirstChunkNumber {
		r.Status.FirstChunkNumber = number
	}

	if r.Status.FirstBlock == 0 || c.FirstBlock < r.Status.FirstBlock {
		r.Status.FirstBlock = c.FirstBlock
	}

	if c.LastBlock > r.Status.LastBlock {
		r.Status.LastBlock = c.LastBlock
	}

	if number > r.Status.LastChunkNumber {
		r.Status.LastChunkNumber = number
		r.Status.LastChunkTime = time.Now().UTC()
	}

	return
}

func (r *ChunkReader) readChunkData(data []byte, firstBlockOnly bool) (out *Chunk, err error) {
	out = &Chunk{}

	for {
		if len(data) == 0 {
			break
		}

		block := &Block{}

		remaining, err2 := StandardCborDecoder.UnmarshalFirst(data, block)
		if err2 != nil {
			err = errors.Wrap(err2, "failed to read chunk")
			return
		}

		blockContainer := &ChunkedBlock{
			block: block,
			data:  data[:len(data)-len(remaining)],
		}

		blockBody := block.Data.Header.Body

		if len(out.BlockContainers) == 0 {
			out.FirstBlock = blockBody.Number
		}

		if blockBody.Number < out.FirstBlock {
			out.FirstBlock = blockBody.Number
		}

		if blockBody.Number > out.LastBlock {
			out.LastBlock = blockBody.Number
		}

		out.BlockContainers = append(out.BlockContainers, blockContainer)

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

func (c *ChunkCache) Set(chunkNumber uint64, value *Chunk) {
	log.Info().Msgf("loading chunk %s into cache", path.Base(value.ChunkPath))

	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.data[chunkNumber]; exists {
		entry.timer.Stop()
	}

	timer := time.AfterFunc(c.duration, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		value.BlockContainers = []*ChunkedBlock{}
		if _, exists := c.data[chunkNumber]; exists {
			for x := value.FirstBlock; x <= value.LastBlock; x++ {
				delete(c.data, x)
			}
			log.Info().Msgf("removing chunk %s from cache", path.Base(value.ChunkPath))
		}
	})

	for x := value.FirstBlock; x <= value.LastBlock; x++ {
		c.data[x] = &chunkCacheEntry{
			value: value,
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
