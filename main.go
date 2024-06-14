package main

import (
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/fxamacker/cbor/v2"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

//go:embed _data/block1
var hexString string

var target string

func main() {
	flag.StringVar(&target, "target", "", "")
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
	case "decode-proxy-files":
		decodeProxyFiles()
	case "test-deserialize-serialize":
		testDeserializeSerialize()
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
	testDeserializeSerialize()
}

// func printSegmentsFromStream(reader *SegmentReader) {
// 	for {
// 		segment, ok := <-reader.Stream
// 		if ok {
// 			t, err := handleSegment(segment)
// 			if err != nil {
// 				log.Fatal().Msgf("%+v", errors.WithStack(err))
// 			}
// 			fmt.Printf("%T\n", t)
// 		} else {
// 			log.Error().Msg("unable to read segment")
// 		}
// 	}
// }

func decodeProxyFiles() {
	dir := "_data/proxy-20240610-182733/"
	items, _ := ioutil.ReadDir(dir)
	inReader := &SegmentReader{
		Direction: "IN",
		Log:       log.Level(zerolog.DebugLevel).With().Logger(),
	}
	// go printSegmentsFromStream(inReader)
	outReader := &SegmentReader{
		Direction: "OUT",
		Log:       log.Level(zerolog.DebugLevel).With().Logger(),
	}
	// go printSegmentsFromStream(outReader)
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

	fmt.Printf("read %d files\n", len(items))
	fmt.Println("done")
	// fmt.Printf("%x\n", x)
}

func testDeserializeSerialize() {
	dir := "_data/proxy-20240610-182733/"
	items, _ := ioutil.ReadDir(dir)
	inReader := &SegmentReader{
		Direction: "IN",
		Log:       log.Level(zerolog.DebugLevel).With().Logger(),
	}
	outReader := &SegmentReader{
		Direction: "OUT",
		Log:       log.Level(zerolog.DebugLevel).With().Logger(),
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

func decodeAuxData() {
	b := "A101A11902A2A1636D736781781C4D696E737761703A205377617020457861637420496E204F72646572"

	bb, err := hex.DecodeString(b)
	if err != nil {
		log.Fatal().Msgf("%+v", errors.WithStack(err))
	}

	a := AuxData{}

	err = cbor.Unmarshal(bb, &a)
	if err != nil {
		log.Fatal().Msgf("%+v", errors.WithStack(err))
	}

	if j, err2 := json.MarshalIndent(a, "", "  "); err2 == nil {
		fmt.Println(string(j))
	} else {
		log.Fatal().Msgf("%+v\n", err2)
	}

	fmt.Println("ok")
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

			_ = os.WriteFile(fmt.Sprintf("block%d", i), []byte(fmt.Sprintf("%x", b.BlockData)), os.ModePerm)
		}
	}
}

func decodeRollBackward() {
	rb1 := loadHexFile("./_data/rollbackward")
	rb2 := loadHexFile("./_data/rollbackward2")

	do := func(data []byte) {
		first, _, err := cbor.DiagnoseFirst(data)
		if err != nil {
			log.Fatal().Msgf("%+v", errors.WithStack(err))
		}
		fmt.Println(first)

		m := &MessageRollBackward{}
		err = cbor.Unmarshal(data, m)
		if err != nil {
			log.Fatal().Msgf("%+v", errors.WithStack(err))
		}
		fmt.Println("ok")

		if j, err := json.MarshalIndent(m, "", "  "); err == nil {
			fmt.Println(string(j))
		}
	}

	do(rb1)
	do(rb2)
}

var output string

func handleSegment(segment *Segment) (target any, err error) {
	var temp any

	subprotocol := -1
	if err := cbor.Unmarshal(segment.Payload, &temp); err == nil {
		subprotocol = int(temp.([]any)[0].(uint64))
	}

	if subprotocol == -1 {
		log.Info().Msg("SKIPPING non-valid subprotocol segment, probably needs concatenating?")
		// log.Info().Msgf("%x", segment.Payload)
		output += fmt.Sprintf("%x\n", segment.Payload)
		return
	}

	target, _ = ProtocolToMessage(segment.Protocol, Subprotocol(subprotocol))

	// TODO: handle the above error and remove the debug code below

	if target != nil {
		fmt.Printf("%s > %T (%d)\n", segment.Protocol, target, subprotocol)
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
