package cardano

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"filippo.io/edwards25519"
	"github.com/alexdcox/cbor/v2"
	"github.com/mr-tron/base58"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
)

type HexString string

func (hs HexString) Bytes() (b []byte) {
	b, _ = hex.DecodeString(string(hs))
	return
}

type Base58Bytes []byte

func (b Base58Bytes) String() string {
	return base58.Encode(b)
}

func (b Base58Bytes) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, b)), nil
}

type HexBytes []byte

func (b HexBytes) String() string {
	return hex.EncodeToString(b)
}

func (b HexBytes) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, b)), nil
}

func (b HexBytes) Equals(o HexBytes) bool {
	return bytes.Equal(b, o)
}

func ExpandEd25519PrivateKey(private *ed25519.PrivateKey) {
	if len(*private) == 32 {
		var scalar edwards25519.Scalar
		// TODO: handle this error
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

type WithCborTag[T any] struct {
	Tag   int64
	Value T
}

func (w WithCborTag[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(w.Value)
}

// TODO: consider Unmarshal JSON?

func (w *WithCborTag[T]) UnmarshalCBOR(bytes []byte) error {
	if bytes[0]-0xC0 > 0 {
		bytes[0] -= 0xC0
		_bytes, err := cbor.UnmarshalFirst(bytes, &w.Tag)
		if err != nil {
			return errors.WithStack(err)
		}
		bytes = _bytes
	}

	return cbor.Unmarshal(bytes, &w.Value)
}

func (w WithCborTag[T]) MarshalCBOR() (out []byte, err error) {
	if w.Tag > 0 {
		out, _ = cbor.Marshal(w.Tag)
		out[0] += 0xC0
	}

	value, err := cbor.Marshal(w.Value)
	if err != nil {
		err = errors.WithStack(err)
		return
	}
	out = append(out, value...)

	return
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

func BinarySearchCallback(start, end uint64, callback func(uint64) (int, error)) error {
	for start <= end {
		mid := start + (end-start)/2

		result, err := callback(mid)
		if err != nil {
			return err
		}

		switch result {
		case -1:
			end = mid - 1
		case 1:
			start = mid + 1
		case 0:
			return nil
		default:
			return errors.Errorf("invalid callback result: %d", result)
		}
	}
	return errors.New("search completed without finding a match")
}

// Cast acts exactly like the builtin go cast but the return value (if ok)
// is automatically converted from aliases of the target type.
//
// e.g.
//
//	type Map map[string]any
//	nonBuiltinValue := Map{"test": 1}
//
//	if builtinValue, ok := Cast[map[string]any](nonBuiltinValue); ok {
//	  // We have our typed and auto-converted value here
//	}
func Cast[X any](target any) (casted X, ok bool) {
	x := new(X)
	typ := reflect.TypeOf(*x)
	if reflect.TypeOf(target).ConvertibleTo(typ) {
		casted = reflect.ValueOf(target).Convert(typ).Interface().(X)
		ok = true
	}
	return
}

func BlakeHash(target any) (hash []byte, err error) {
	bytes, err := cbor.Marshal(target)
	if err != nil {
		err = errors.WithStack(err)
		return
	}
	_hash := blake2b.Sum256(bytes)
	hash = _hash[:]
	return
}

func ChunkString(s string, chunkSize int) []string {
	if chunkSize <= 0 {
		return nil
	}

	var chunks []string
	runes := []rune(s)

	for i := 0; i < len(runes); i += chunkSize {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
	}

	return chunks
}
