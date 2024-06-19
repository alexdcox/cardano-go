package main

import (
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/fxamacker/cbor/v2"
	"github.com/pkg/errors"
)

//go:embed _data/rollforwardheader
var hexString string

var target string
var nodeHostPort string

func main() {
	flag.StringVar(&target, "target", "", "")
	flag.StringVar(&nodeHostPort, "node", "", "")
	flag.Parse()

	decodedHex, err := hex.DecodeString(hexString)
	if err != nil {
		globalLog.Fatal().Msgf("%+v", errors.WithStack(err))
	}
	_ = decodedHex

	switch target {
	case "block":
		decodeBlock(decodedHex)
		return
	case "blocks":
		decodeBlocks()
		return
	// case "decode-proxy-files":
	// 	decodeProxyFiles()
	case "test-deserialize-serialize":
		testDeserializeSerialize()
	case "client":
		testClient()
	}

	h, err := hex.DecodeString("201ef1cc800300028102")
	if err != nil {
		globalLog.Fatal().Msgf("%+v", errors.WithStack(err))
	}

	fmt.Println(h[10:])

	// decode()
	// decodeAuxData()
	// decodeBlockMessages()
	// decodeProxyFiles()
	// decodeProxyFiles2()
	// decodeProxyFiles3()
	// decodeRollBackward()
	// decodeProxyFiles3()
	// decodeSegment(decodedHex)
	// encode()
	// encodeVersionMap()
	// stripInvalidMapKeys()
	// testDeserializeSerialize()
}

func testClient() {
	client := NewClient(nodeHostPort, NetworkMagicMainnet)

	err := client.Dial()
	if err != nil {
		globalLog.Fatal().Msgf("%+v", errors.WithStack(err))
	}

	err = client.Handshake()
	if err != nil {
		globalLog.Fatal().Msgf("%+v", errors.WithStack(err))
	}

	go client.KeepAlive()

	block, err := client.FetchBlock(WellKnownMainnetPoint)
	if err != nil {
		globalLog.Fatal().Msgf("%+v", errors.WithStack(err))
	}

	globalLog.Info().Msgf(
		"block number: %v, transactions: %d",
		block.Data.Header.Body.Number,
		len(block.Data.TransactionBodies),
	)
}

var globalSegmentStream chan *Segment

func testDeserializeSerialize() {
	dir := "_data/proxy-20240610-182733/"
	items, _ := os.ReadDir(dir)
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

			if x, ok := segment.Message.(*MessageBlock); ok {
				block, err := x.Block()
				if err != nil {
					globalLog.Fatal().Msgf("%+v", errors.WithStack(err))
				}
				globalLog.Info().Msgf("decoded block: %d", block.Data.Header.Body.Number)
			}
		}
	}()
	var err error
	for _, item := range items {
		h := loadHexFile(dir + item.Name())
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

func decodeBlocks() {
	for i := 1; i <= 14; i++ {
		block := loadHexFile(fmt.Sprintf("./_data/block%d", i))
		decodeBlock(block)
		fmt.Printf("block %d ok\n", i)
	}
}

func decodeBlockMessages() {
	blocksFile := loadFile("./_data/blocks")
	var i int
	for _, line := range strings.Split(string(blocksFile), "\n") {
		if len(line) > 10 && line[:10] == "8204d81859" {
			fmt.Printf("at block %d\n", i)
			i++

			h, err := hex.DecodeString(line)
			if err != nil {
				globalLog.Fatal().Msgf("%+v", errors.WithStack(err))
			}

			b := &MessageBlock{}
			err = cbor.Unmarshal(h, b)
			if err != nil {
				globalLog.Fatal().Msgf("%+v", errors.WithStack(err))
			}

			_ = os.WriteFile(fmt.Sprintf("block%d", i), []byte(fmt.Sprintf("%x", b.BlockData)), os.ModePerm)
		}
	}
}

func decodeBlock(data []byte) {
	a := Block{}

	err := cbor.Unmarshal(data, &a)
	if err != nil {
		if x, err := cbor.Diagnose(data); err == nil {
			fmt.Println(x)
		}

		globalLog.Fatal().Msgf("%+v", errors.WithStack(err))
	}

	t := a

	if j, err2 := json.MarshalIndent(t, "", "  "); err2 == nil {
		fmt.Println(string(j))
	} else {
		if x, err := cbor.Marshal(t); err == nil {
			fmt.Printf("%x\n", x)
		}

		fmt.Printf("%T %+v\n", t, t)

		globalLog.Fatal().Msgf("%+v\n", err2)
	}

	fmt.Println("ok!")
}
