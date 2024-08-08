package cardano

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/pkg/errors"
)

// TODO: use sqlite, no longer needed

type TipStore interface {
	GetTip() (Tip, error)
	SetTip(Tip) error
}

type fileSystemJsonTipLoader struct {
	path string
}

func (f fileSystemJsonTipLoader) LoadTip() (tip Tip, err error) {
	tipFile, err := os.ReadFile(f.path)
	if err != nil {
		err = errors.Wrap(err, "unable to read tip file")
		return
	}

	tip = Tip{}
	err = json.Unmarshal(tipFile, &tip)
	if err != nil {
		err = errors.Wrap(err, "unable to unmarshal tip file")
		return
	}

	globalLog.Info().Msgf("read latest tip (block: %d) from file", tip.Block)

	return
}

func (f fileSystemJsonTipLoader) SaveTip(tip Tip) (err error) {
	tipJson, err := json.Marshal(tip)
	if err != nil {
		err = errors.Wrap(err, "unable to marshal tip")
		return
	}

	err = os.WriteFile(f.path, tipJson, os.ModePerm)
	if err != nil {
		err = errors.Wrap(err, "unable to write tip to file")
		return
	}

	globalLog.Info().Msgf("wrote latest tip (block: %d) to file %s", tip.Block, f.path)

	return
}

var FileSystemTipLoader = func(path string) fileSystemJsonTipLoader {
	return fileSystemJsonTipLoader{path}
}

type InMemoryTipStore struct {
	mu         sync.RWMutex
	currentTip *Tip
}

func NewInMemoryTipStore() *InMemoryTipStore {
	return &InMemoryTipStore{}
}

func (s *InMemoryTipStore) GetTip() (Tip, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.currentTip == nil {
		return Tip{
			Point: Point{
				Slot: 0,
				Hash: make(HexBytes, 32),
			},
			Block: 0,
		}, nil
	}

	return *s.currentTip, nil
}

func (s *InMemoryTipStore) SetTip(tip Tip) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.currentTip = &tip
	return nil
}
