package cardano

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"strings"

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

func IterateDirectory(dir string, cb func(path string, info os.FileInfo, data []byte, err error) error) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			data, err2 := os.ReadFile(path)
			if err2 != nil {
				return errors.WithStack(err2)
			}
			return cb(path, info, data, err)
		}
		return nil
	})
}

var StandardCborDecoder, _ = cbor.DecOptions{
	UTF8: cbor.UTF8DecodeInvalid,
}.DecMode()
