package main

import (
	"encoding/json"
	"os"

	"github.com/pkg/errors"
)

type TipStore interface {
	LoadTip() (Tip, error)
	SaveTip(Tip) error
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

	globalLog.Info().Msgf("wrote latest tip %s to file %s", tip, f.path)

	return
}

var FileSystemTipLoader = func(path string) fileSystemJsonTipLoader {
	return fileSystemJsonTipLoader{path}
}
