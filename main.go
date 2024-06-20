package main

import (
	_ "embed"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path"
	"strings"

	"github.com/pkg/errors"
)

var dir string
var nodeHostPort string

func main() {
	if len(os.Args) < 2 {
		globalLog.Fatal().Msgf("usage: cardano-go (client [--node <host:port>] | proxy-decode [--dir <dir>])")
	}

	fmt.Println(os.Args[2:])

	switch os.Args[1] {
	case "proxy-decode":
		fs := flag.NewFlagSet("proxy-decode", flag.ExitOnError)
		fs.StringVar(&dir, "dir", "", "")
		err := fs.Parse(os.Args[2:])
		if err != nil {
			return
		}
		testProxyDecode()
	case "client":
		fs := flag.NewFlagSet("client", flag.ExitOnError)
		fs.StringVar(&nodeHostPort, "node", "", "")
		err := fs.Parse(os.Args[2:])
		if err != nil {
			return
		}
		testClient()
	default:
		globalLog.Error().Msgf("invalid subcommand '%s'", os.Args[2])
	}
}

func testClient() {
	client := NewClient(nodeHostPort, NetworkMagicMainnet)

	err := client.Start()
	if err != nil {
		globalLog.Fatal().Msgf("%+v", errors.WithStack(err))
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

	if err = client.Stop(); err != nil {
		globalLog.Error().Msgf("failed to stop client gracefully: %+v", err)
	} else {
		globalLog.Info().Msg("client stopped gracefully")
	}
}

var globalSegmentStream chan *Segment

func testProxyDecode() {
	items, err := os.ReadDir(dir)
	if err != nil {
		globalLog.Fatal().Msgf("unable to read dir: %s, %+v", dir, errors.WithStack(err))
	}

	globalSegmentStream = make(chan *Segment)
	inReader := &SegmentReader{
		Direction: DirectionIn,
		Log:       globalLog,
		Stream:    globalSegmentStream,
		Mu:        globalSegmentReaderMutex,
	}
	outReader := &SegmentReader{
		Direction: DirectionOut,
		Log:       globalLog,
		Stream:    globalSegmentStream,
		Mu:        globalSegmentReaderMutex,
	}
	go func() {
		for {
			segment, ok := <-globalSegmentStream
			if !ok {
				globalLog.Info().Msg("segment stream closed")
				return
			}

			if x, ok2 := segment.Message.(*MessageBlock); ok2 {
				block, err2 := x.Block()
				if err2 != nil {
					globalLog.Fatal().Msgf("%+v", errors.WithStack(err2))
				}

				globalLog.Info().Msgf("decoded block: %d", block.Data.Header.Body.Number)
			}
		}
	}()
	for _, item := range items {
		h := loadHexFile(path.Join(dir, item.Name()))
		if strings.Contains(item.Name(), "i") {
			_, err = inReader.Read(h)
			if err != nil {
				globalLog.Fatal().Msgf("%+v", errors.WithStack(err))
			}
		} else {
			_, err = outReader.Read(h)
			if err != nil {
				globalLog.Fatal().Msgf("%+v", errors.WithStack(err))
			}
		}
	}
}

func loadFile(filePath string) (data []byte) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		globalLog.Fatal().Msgf("%+v", errors.WithStack(err))
	}
	return
}

func loadHexFile(filePath string) (data []byte) {
	data, err := hex.DecodeString(string(loadFile(filePath)))
	if err != nil {
		globalLog.Fatal().Msgf("%+v", errors.WithStack(err))
	}
	return
}
