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
	"github.com/rs/zerolog"
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
		log.Fatal().Msgf("%+v", errors.WithStack(err))
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
	client := NewClient(nodeHostPort)

	err := client.Dial()
	if err != nil {
		log.Fatal().Msgf("%+v", errors.WithStack(err))
	}

	err = client.Handshake()
	if err != nil {
		log.Fatal().Msgf("%+v", errors.WithStack(err))
	}

	go client.KeepAlive()

	block, err := client.FetchBlock(Point{})
	if err != nil {
		log.Fatal().Msgf("%+v", errors.WithStack(err))
	}

	log.Info().Msgf(
		"block number: %v, transactions: %d",
		block.Data.Header.Body.Number,
		len(block.Data.TransactionBodies),
	)
}

func printSegmentsFromStream() {
	// log.Info().Msg("streaming segments")
	for {
		segment, ok := <-globalSegmentStream
		if ok {
			t, err := handleSegment(segment)
			if err != nil {
				log.Fatal().Msgf("%+v", errors.WithStack(err))
			}
			// fmt.Printf("%T\n", t)
			_ = t
		} else {
			log.Error().Msg("unable to read segment")
		}
	}
}

var globalSegmentStream chan *Segment

// func decodeProxyFiles() {
// 	globalSegmentStream = make(chan *Segment)
//
// 	dir := "_data/proxy-20240610-182733/"
// 	items, _ := ioutil.ReadDir(dir)
// 	inReader := &SegmentReader{
// 		Direction: "IN",
// 		Log:       log.Level(zerolog.DebugLevel).With().Logger(),
// 		Stream:    globalSegmentStream,
// 	}
// 	outReader := &SegmentReader{
// 		Direction: "OUT",
// 		Log:       log.Level(zerolog.DebugLevel).With().Logger(),
// 		Stream:    globalSegmentStream,
// 	}
// 	var err error
//
// 	for _, item := range items {
// 		h := loadHexFile(dir + item.Name())
// 		if strings.Contains(item.Name(), "i") {
// 			_, err = inReader.Read(h)
// 			if err != nil {
// 				log.Fatal().Msgf("%+v", errors.WithStack(err))
// 			}
// 		} else {
// 			_, err = outReader.Read(h)
// 			if err != nil {
// 				log.Fatal().Msgf("%+v", errors.WithStack(err))
// 			}
// 		}
// 	}
//
// 	fmt.Printf("read %d files\n", len(items))
// 	fmt.Println("done")
// 	// fmt.Printf("%x\n", x)
// }

func testDeserializeSerialize() {
	dir := "_data/proxy-20240610-182733/"
	items, _ := os.ReadDir(dir)
	globalSegmentStream = make(chan *Segment)
	go printSegmentsFromStream()
	inReader := &SegmentReader{
		Direction: DirectionIn,
		Log:       log.Level(zerolog.TraceLevel),
		Stream:    globalSegmentStream,
	}
	outReader := &SegmentReader{
		Direction: DirectionOut,
		Log:       log.Level(zerolog.TraceLevel),
		Stream:    globalSegmentStream,
	}
	var err error
	for _, item := range items {
		h := loadHexFile(dir + item.Name())
		if strings.Contains(item.Name(), "i") {
			_, err = inReader.Read(h)
			if err != nil {
				log.Fatal().Msgf("%+v", errors.WithStack(err))
			}
		} else {
			_, err = outReader.Read(h)
			if err != nil {
				log.Fatal().Msgf("%+v", errors.WithStack(err))
			}
		}
	}
}

func loadFile(filePath string) (data []byte) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatal().Msgf("%+v", errors.WithStack(err))
	}
	return
}

func loadHexFile(filePath string) (data []byte) {
	data, err := hex.DecodeString(string(loadFile(filePath)))
	if err != nil {
		log.Fatal().Msgf("%+v", errors.WithStack(err))
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
				log.Fatal().Msgf("%+v", errors.WithStack(err))
			}

			b := &MessageBlock{}
			err = cbor.Unmarshal(h, b)
			if err != nil {
				log.Fatal().Msgf("%+v", errors.WithStack(err))
			}

			_ = os.WriteFile(fmt.Sprintf("block%d", i), []byte(fmt.Sprintf("%x", b.Block)), os.ModePerm)
		}
	}
}

func handleSegment(segment *Segment) (target any, err error) {
	var temp any

	subprotocol := -1
	if err := cbor.Unmarshal(segment.Payload, &temp); err == nil {
		subprotocol = int(temp.([]any)[0].(uint64))
	}

	if subprotocol == -1 {
		log.Info().Msg("SKIPPING non-valid subprotocol segment, probably needs concatenating?")
		// log.Info().Msgf("%x", segment.Payload)
		return
	}

	target, _ = ProtocolToMessage(segment.Protocol, Subprotocol(subprotocol))

	// TODO: handle the above error and remove the debug code below

	if target != nil {
		fmt.Printf("%s %s %T (%d)\n", segment.Direction.String(), segment.Protocol, target, subprotocol)
		err = cbor.Unmarshal(segment.Payload, target)
		if err != nil {
			fmt.Println(temp)
			fmt.Printf("%x\n", segment.Payload)
			fmt.Printf("protocol:    %s\n", segment.Protocol)
			fmt.Printf("subprotocol: %d\n", subprotocol)
			err = errors.WithStack(err)
			return
		}
		if j, err := json.MarshalIndent(target, "", "  "); err == nil {
			fmt.Println(string(j))
		}
	} else {
		fmt.Println(temp)
		fmt.Printf("%x\n", segment.Payload)
		fmt.Printf("protocol:    %s\n", segment.Protocol)
		fmt.Printf("subprotocol: %d\n", subprotocol)

		err = errors.Errorf(
			"no target message to deserialize for payload %x\n",
			segment.Payload)
	}

	return
}

func decodeBlock(data []byte) {
	a := Block{}

	err := cbor.Unmarshal(data, &a)
	if err != nil {
		if x, err := cbor.Diagnose(data); err == nil {
			fmt.Println(x)
		}

		log.Fatal().Msgf("%+v", errors.WithStack(err))
	}

	t := a

	if j, err2 := json.MarshalIndent(t, "", "  "); err2 == nil {
		fmt.Println(string(j))
	} else {
		if x, err := cbor.Marshal(t); err == nil {
			fmt.Printf("%x\n", x)
		}

		fmt.Printf("%T %+v\n", t, t)

		log.Fatal().Msgf("%+v\n", err2)
	}

	fmt.Println("ok!")
}
