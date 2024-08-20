package cardano

import (
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

func NewChunkReader(dir string) (reader *ChunkReader, err error) {
	reader = &ChunkReader{
		dir:           dir,
		immutableDir:  path.Join(dir, "./immutable/"),
		numberPathMap: make(map[uint64]*string),
		cache:         NewChunkCache(time.Second * 30),
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
	dir           string
	immutableDir  string
	numberPathMap map[uint64]*string // block number -> chunk file path
	Status        ChunkReaderStatus
	cache         *ChunkCache
}

type ChunkedBlock struct {
	block *Block
	data  []byte
}

func (r *ChunkReader) LoadChunkFiles(skipUntilNumber uint64) (err error) {
	r.Status.Started = time.Now().UTC()

	log.Info().Msg("initiating block reporter, reading all chunk files...")
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
			for x := chunks[i2].FirstBlock; x <= chunks[i2].LastBlock; x++ {
				r.numberPathMap[x] = &chunks[i2].ChunkPath
			}
			chunks[i2].BlockContainers = []*ChunkedBlock{}
			log.Info().Msgf(
				"loaded chunk %d, block range %d to %d (%d blocks)",
				number-1,
				chunks[i2].FirstBlock,
				chunks[i2].LastBlock-1,
				chunks[i2].LastBlock-chunks[i2].FirstBlock,
			)
			break
		}
		for x := chunks[i2-1].FirstBlock; x < chunks[i2].FirstBlock; x++ {
			r.numberPathMap[x] = &chunks[i2-1].ChunkPath
		}
		log.Info().Msgf(
			"loaded chunk %d, block range %d to %d (%d blocks)",
			number-1,
			chunks[i2-1].FirstBlock,
			chunks[i2].FirstBlock-1,
			chunks[i2].FirstBlock-chunks[i2-1].FirstBlock,
		)
	}

	r.Status.TimeToStart = time.Now().UTC().Sub(r.Status.Started)
	p := message.NewPrinter(language.English)
	log.Info().Msgf("loaded %s blocks in %s", p.Sprintf("%d", len(r.numberPathMap)), r.Status.TimeToStart.String())

	return
}

func (r *ChunkReader) WatchChunkFiles(cb func(chunk, first, last uint64)) (err error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal().Msgf("%+v", err)
	}
	defer watcher.Close()

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

					number, err2 := r.chunkFileNumber(event.Name)
					if err2 != nil {
						log.Fatal().Msgf("%+v", errors.WithStack(err2))
					}

					log.Info().Msgf(
						"reloaded chunk %d, block range %d to %d (%d blocks)",
						number,
						chunk.FirstBlock,
						chunk.LastBlock,
						chunk.LastBlock-chunk.FirstBlock,
					)

					cb(number, chunk.FirstBlock, chunk.LastBlock)
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

type ChunkRange struct {
	Start uint64
	End   uint64
	Chunk uint64
}

func (r *ChunkReader) ProcessBlockRanges() ([]ChunkRange, time.Duration) {
	startTime := time.Now()

	// Create a map to store min and max for each chunk path
	chunkRanges := make(map[string]ChunkRange)

	// Iterate through the numberPathMap
	for blockNum, pathPtr := range r.numberPathMap {
		if pathPtr == nil {
			continue // Skip if path is nil
		}
		path := *pathPtr

		// If this path is not in chunkRanges, initialize it
		if _, exists := chunkRanges[path]; !exists {
			chunkRanges[path] = ChunkRange{
				Start: math.MaxUint64, // Initialize Start to maximum possible value
				End:   0,              // Initialize End to minimum possible value
				Chunk: blockNum,
			}
		}

		// Update the range for this path
		current := chunkRanges[path]
		current.Start = uint64(math.Min(float64(current.Start), float64(blockNum)))
		current.End = uint64(math.Max(float64(current.End), float64(blockNum)))
		chunkRanges[path] = current
	}

	// Convert map to slice
	result := make([]ChunkRange, 0, len(chunkRanges))
	for _, r := range chunkRanges {
		result = append(result, r)
	}

	// Sort the result slice by Start block number
	sort.Slice(result, func(i, j int) bool {
		return result[i].Start < result[j].Start
	})

	elapsedTime := time.Since(startTime)
	return result, elapsedTime
}

func (r *ChunkReader) GetChunkedBlock(number uint64) (block *ChunkedBlock, err error) {
	findBlockInChunk := func(chunk *Chunk) (*ChunkedBlock, error) {
		for _, b := range chunk.BlockContainers {
			if b.block.Data.Header.Body.Number == number {
				return b, nil
			}
		}
		return nil, errors.WithStack(ErrChunkBlockMissing)
	}

	if cachedChunk, ok := r.cache.Get(number); ok {
		return findBlockInChunk(cachedChunk)
	}

	if chunkPath, ok := r.numberPathMap[number]; ok {
		var chunk *Chunk
		chunk, err = r.processChunkFile(*chunkPath, false)
		if err != nil {
			return
		}
		return findBlockInChunk(chunk)
	}

	return nil, errors.WithStack(ErrBlockNotFound)
}

func (r *ChunkReader) GetBlock(number uint64) (block *Block, err error) {
	chunkedBlock, err := r.GetChunkedBlock(number)
	if err != nil {
		return
	}
	return chunkedBlock.block, nil
}

func (r *ChunkReader) chunkFileNumber(path string) (number uint64, err error) {
	numberStr := filepath.Base(path)[:len(filepath.Base(path))-6] // Remove ".chunk"
	number, err = strconv.ParseUint(numberStr, 10, 64)
	if err != nil {
		err = errors.WithStack(err)
	}
	return
}

func (r *ChunkReader) processChunkFile(path string, firstBlockOnly bool) (c *Chunk, err error) {
	if filepath.Ext(path) != ".chunk" {
		err = errors.WithStack(ErrNotChunkFile)
		return
	}

	re := regexp.MustCompile(`^\d+\.chunk$`)
	if !re.MatchString(filepath.Base(path)) {
		return
	}

	number, err := r.chunkFileNumber(path)

	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("Error reading file %s: %v", path, err)
		return
	}

	c, err = r.readChunkData(data, firstBlockOnly)
	if err != nil {
		return
	}

	c.ChunkPath = path

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

func (c *ChunkCache) Set(number uint64, value *Chunk) {
	log.Info().Msgf("loading chunk %s into cache", path.Base(value.ChunkPath))

	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.data[number]; exists {
		entry.timer.Stop()
	}

	timer := time.AfterFunc(c.duration, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		value.BlockContainers = []*ChunkedBlock{}
		if _, exists := c.data[number]; exists {
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
