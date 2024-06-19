package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
	"sync"

	"github.com/fxamacker/cbor/v2"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

type Direction bool

func (d Direction) String() string {
	if d == DirectionIn {
		return "IN"
	}
	return "OUT"
}

const (
	DirectionIn  Direction = false
	DirectionOut Direction = true
)

type Segment struct {
	Timestamp     uint32
	Protocol      Protocol
	PayloadLength uint16
	Payload       []byte
	Direction     Direction
	Message       any
}

// func (s *Segment) UnmarshalDataItem(data []byte) (length int, err error) {
// 	if len(data) < 8 {
// 		err = errors.Errorf("short segment data len=%d", len(data))
// 		return
// 	}
//
// 	timestampBytes := data[0:4]
// 	s.Timestamp = binary.BigEndian.Uint32(timestampBytes)
//
// 	protocolBytes := data[4:6]
// 	s.Protocol = Protocol(binary.BigEndian.Uint16(protocolBytes))
// 	if protocolBytes[0] == 0x80 {
// 		s.Protocol -= 32768
// 	}
//
// 	if s.Protocol > 10 {
// 		err = errors.Errorf(
// 			"protocol %d is obviously wrong, read from hex: %x",
// 			s.Protocol,
// 			protocolBytes)
// 		return
// 	}
//
// 	byteLenBytes := data[6:8]
// 	s.PayloadLength = binary.BigEndian.Uint16(byteLenBytes)
// 	// if byteLenBytes[0] == 0x80 {
// 	// 	s.PayloadLength -= 32768
// 	// }
//
// 	// if len(data)-8 != int(byteLenInt) {
// 	// 	log.Fatalf("%+v", errors.Errorf(
// 	// 		"expected %d bytes remaining, actually %d",
// 	// 		int(byteLenInt),
// 	// 		len(data[n:])-8))
// 	// }
//
// 	i := 8 + int(s.PayloadLength)
// 	if i > len(data) {
// 		i = len(data)
// 	}
// 	s.Payload = data[8:i]
// 	// first, _, err := cbor.DiagnoseFirst(payload)
// 	// if err != nil {
// 	// 	log.Fatalf("%+v", errors.WithStack(err))
// 	// }
// 	// fmt.Println(first)
// 	// fmt.Println("------------------------------")
//
// 	// if i < len(data) {
// 	// 	log.Warn().Msgf(
// 	// 		"segment parsed with %d additional bytes remaining",
// 	// 		len(data)-len(s.Payload))
// 	// }
//
// 	length = i
//
// 	return
// }

func (s *Segment) Complete() bool {
	return int(s.PayloadLength) == len(s.Payload)
}

// func (s *Segment) Append(data []byte) (read int) {
// 	remaining := int(s.PayloadLength) - len(s.Payload)
// 	read = int(math.Min(float64(len(data)), float64(remaining)))
// 	s.Payload = append(s.Payload, data[:read]...)
//
// 	// fmt.Printf("segment appended with %d bytes remaining\n", remaining)
//
// 	return
// }

func NewSegmentReader(direction Direction) *SegmentReader {
	return &SegmentReader{
		Direction: direction,
		Log:       zerolog.Nop(),
		Stream:    make(chan *Segment),
		Mu:        &sync.Mutex{},
	}
}

type SegmentReader struct {
	segment   *Segment
	Direction Direction
	Log       zerolog.Logger
	Stream    chan *Segment
	Batching  bool
	Batch     []*Segment
	Mu        *sync.Mutex
}

var globalSegmentReaderMutex = &sync.Mutex{}

func (r *SegmentReader) Read(data []byte) (n int, err error) {
	if r.Mu != nil {
		r.Mu.Lock()
		defer r.Mu.Unlock()
	}

	r.Log.Debug().Msgf("%s %d bytes\n%x", r.Direction, len(data), data)
	defer r.Log.Debug().Msg("-----------------------------------------------------------------------------------------------")

	for i := 0; i < len(data); i++ {
		if r.segment == nil {
			r.segment = &Segment{
				Direction: r.Direction,
			}

			timestampBytes := data[i : i+4]
			r.segment.Timestamp = binary.BigEndian.Uint32(timestampBytes)

			protocolBytes := data[i+4 : i+6]
			r.segment.Protocol = Protocol(binary.BigEndian.Uint16(protocolBytes))
			if protocolBytes[0] == 0x80 {
				r.segment.Protocol -= 32768
			}

			if r.segment.Protocol > 10 {
				err = errors.Errorf(
					"protocol %d is obviously wrong, read from hex: %x",
					r.segment.Protocol,
					protocolBytes)
				return
			}

			byteLenBytes := data[i+6 : i+8]
			r.segment.PayloadLength = binary.BigEndian.Uint16(byteLenBytes)

			r.Log.Debug().Msgf("new segment with payload length: %d", r.segment.PayloadLength)
			i += 8
		}

		r.segment.Payload = append(r.segment.Payload, data[i])
		if r.segment.Complete() {
			r.Log.Debug().Msg("segment complete")

			if r.Batching {
				r.Log.Debug().Msg("batching segment")
				r.Batch = append(r.Batch, r.segment)
				batchEnd := []byte{0x81, 0x05}
				if bytes.Equal(r.segment.Payload[len(r.segment.Payload)-2:], batchEnd) {

					batchPayload := []byte{}
					for _, b := range r.Batch {
						batchPayload = append(batchPayload, b.Payload...)
					}
					batchPayload = batchPayload[:len(batchPayload)-2]

					r.Log.Debug().Msgf("batch complete\n%x", batchPayload)

					m := &MessageBlock{}
					err = cbor.Unmarshal(batchPayload, m)
					if err != nil {
						return
					}

					block, err2 := m.Block()
					if err2 != nil {
						err = err2
						return
					}

					r.Log.Info().Msgf("parsed block: %v", block.Data.Header.Body.Number)
					r.Batching = false
				}
			} else {
				message, err2 := parseSegmentMessage(r.segment)

				if !r.Batching && message == nil {
					if r.segment.Protocol == ProtocolBlockFetch && r.Direction == DirectionIn {
						batchStart := []byte{0x81, 0x02}
						if bytes.Equal(r.segment.Payload[:2], batchStart) {
							r.Log.Info().Msg("alternate block start marker found")
							r.Log.Debug().Msg("start batch")
							r.Batching = true
							r.Batch = []*Segment{r.segment}
							continue
						}
					}
				}

				if err2 != nil {
					err = err2
					return
				}
				r.segment.Message = message
			}
			if reflect.TypeOf(r.segment.Message) == reflect.TypeOf(&MessageStartBatch{}) {
				r.Log.Debug().Msg("start batch")
				r.Batching = true
				r.Batch = []*Segment{}
			}
			r.Stream <- r.segment
			r.segment = nil
		}
	}

	return
}

func parseSegmentMessage(segment *Segment) (target any, err error) {
	var temp any

	subprotocol := -1
	if err := cbor.Unmarshal(segment.Payload, &temp); err == nil {
		subprotocol = int(temp.([]any)[0].(uint64))
	}

	if subprotocol == -1 {
		// if segment.Protocol == ProtocolBlockFetch {
		// 	target = &MessageBlock{}
		// } else {
		err = errors.Errorf("non-valid subprotocol segment, probably payload needs concatenating or splitting?\n%x", segment.Payload)
		return
		// }
	}

	target, _ = ProtocolToMessage(segment.Protocol, Subprotocol(subprotocol))

	// TODO: remove the debug code below

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
		// if j, err := json.MarshalIndent(target, "", "  "); err == nil {
		// 	fmt.Println(string(j))
		// }
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
