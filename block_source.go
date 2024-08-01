package cardano

import (
	"fmt"
	"sync"
)

type BlockSource interface {
	GetBlock(number int64) (*Block, error)
	GetBlockChan() chan *Block
}

type BlockStream struct {
	sources []BlockSource
	blocks  map[int64]*Block
	mu      sync.RWMutex
	blockCh chan *Block
}

func NewBlockStream() *BlockStream {
	bs := &BlockStream{
		blocks:  make(map[int64]*Block),
		blockCh: make(chan *Block),
	}
	go bs.aggregateBlocks()
	return bs
}

func (s *BlockStream) aggregateBlocks() {
	for _, source := range s.sources {
		go func(src BlockSource) {
			for block := range src.GetBlockChan() {
				s.mu.Lock()
				if _, exists := s.blocks[block.Data.Header.Body.Number]; !exists {
					s.blocks[block.Data.Header.Body.Number] = block
					s.blockCh <- block
				}
				s.mu.Unlock()
			}
		}(source)
	}
}

func (s *BlockStream) AddSource(source BlockSource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sources = append(s.sources, source)
	go func() {
		for block := range source.GetBlockChan() {
			s.mu.Lock()
			if _, exists := s.blocks[block.Data.Header.Body.Number]; !exists {
				s.blocks[block.Data.Header.Body.Number] = block
				s.blockCh <- block
			}
			s.mu.Unlock()
		}
	}()
}

func (s *BlockStream) GetBlock(height int64) (*Block, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if block, exists := s.blocks[height]; exists {
		return block, nil
	}

	for _, source := range s.sources {
		if block, err := source.GetBlock(height); err == nil {
			s.blocks[height] = block
			return block, nil
		}
	}

	return nil, fmt.Errorf("block at height %d not found", height)
}

func (s *BlockStream) GetBlockChan() chan *Block {
	return s.blockCh
}
