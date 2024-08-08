package cardano

import (
	"crypto/ed25519"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"filippo.io/edwards25519"
	"github.com/fxamacker/cbor/v2"
	"github.com/pkg/errors"
)

func ExpandEd25519PrivateKey(private *ed25519.PrivateKey) {
	if len(*private) == 32 {
		var scalar edwards25519.Scalar
		_, _ = scalar.SetBytesWithClamping(*private)
		var p edwards25519.Point
		p.ScalarBaseMult(&scalar)
		*private = append(*private, p.Bytes()...)
	}
}

func IndentCbor(input string) string {
	var index = 0
	var output = ""
	indent := 0
	var newline bool

	for {
		if index >= len(input) {
			break
		}

		nextChar := input[index]

		if nextChar == '[' || nextChar == '{' {
			if newline {
				output += strings.Repeat(" ", indent*2)
				newline = false
			}
			indent++
			output += string(nextChar) + "\n" + strings.Repeat(" ", indent*2)
		} else if nextChar == ']' || nextChar == '}' {
			indent--
			output += "\n" + strings.Repeat(" ", indent*2) + string(nextChar)
		} else if nextChar == ',' {
			output += ",\n"
			newline = true
		} else {
			if newline {
				if nextChar == ' ' {
					index++
					continue
				}
				output += strings.Repeat(" ", indent*2)
				newline = false
			}
			output += string(nextChar)
		}

		index++
	}

	return output
}

func DirectoryExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		log.Info().Msgf("error checking directory: %v", err)
		return false
	}
	return info.IsDir()
}

func IterateChunkDirectory(dir string, cb func(path string, data []byte, err error) error) error {
	type fileInfo struct {
		Number uint64
		Path   string
	}
	var files []fileInfo
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			segments := strings.Split(info.Name(), "-")
			number, _ := strconv.ParseUint(segments[0], 10, 64)
			files = append(files, fileInfo{
				Number: number,
				Path:   path,
			})
		}
		return nil
	})
	if err != nil {
		return err
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Number < files[j].Number
	})

	for _, file := range files {
		data, err2 := os.ReadFile(file.Path)
		if err2 != nil {
			return errors.WithStack(err2)
		}
		err = cb(file.Path, data, err)
		if err != nil {
			return err
		}
	}

	return nil
}

var StandardCborDecoder, _ = cbor.DecOptions{
	UTF8:             cbor.UTF8DecodeInvalid,
	MaxArrayElements: math.MaxInt32,
	MaxMapPairs:      math.MaxInt32,
}.DecMode()

type PeriodicCaller struct {
	mu           sync.Mutex
	lastCallTime time.Time
	interval     time.Duration
	action       func()
	stopChan     chan struct{}
	stopped      bool
}

func NewPeriodicCaller(interval time.Duration, action func()) *PeriodicCaller {
	return &PeriodicCaller{
		interval: interval,
		action:   action,
		stopChan: make(chan struct{}),
	}
}

func (pc *PeriodicCaller) Postpone() {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if pc.stopped {
		return
	}

	now := time.Now()
	if now.Sub(pc.lastCallTime) >= pc.interval {
		go pc.action() // Run the action in a goroutine
	}
	pc.lastCallTime = now
}

func (pc *PeriodicCaller) Start() {
	go func() {
		ticker := time.NewTicker(pc.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				pc.mu.Lock()
				if !pc.stopped && time.Since(pc.lastCallTime) >= pc.interval {
					go pc.action() // Run the action in a goroutine
					pc.lastCallTime = time.Now()
				}
				pc.mu.Unlock()
			case <-pc.stopChan:
				return
			}
		}
	}()
}

func (pc *PeriodicCaller) Stop() {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if !pc.stopped {
		pc.stopped = true
		close(pc.stopChan)
	}
}
