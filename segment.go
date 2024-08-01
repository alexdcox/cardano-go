package cardano

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
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
	Message       Message
}

func (s *Segment) MarshalDataItem() (out []byte, err error) {
	messageCbor, err := cbor.Marshal(s.Message)
	if err != nil {
		err = errors.WithStack(err)
		return
	}
	s.Payload = messageCbor

	timestamp := uint32(1)
	timestampBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(timestampBytes, timestamp)

	protocolBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(protocolBytes, uint16(s.Protocol))

	s.PayloadLength = uint16(len(messageCbor))
	payloadLengthBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(payloadLengthBytes, s.PayloadLength)

	buf := &bytes.Buffer{}
	buf.Write(timestampBytes)
	buf.Write(protocolBytes)
	buf.Write(payloadLengthBytes)
	buf.Write(messageCbor)

	out = buf.Bytes()

	return
}

func (s *Segment) Complete() bool {
	return int(s.PayloadLength) == len(s.Payload)
}

func (s *Segment) SetMessage(message Message) (err error) {
	protocol, ok := MessageProtocolMap[reflect.TypeOf(message)]
	if !ok {
		return errors.Errorf("no protocol for message %T", message)
	}

	s.Message = message
	s.Protocol = protocol

	return
}

func NewSegmentReader(direction Direction) *SegmentReader {
	return &SegmentReader{
		Direction: direction,
		Log:       &globalLog,
		Stream:    make(chan *Segment),
	}
}

type SegmentReader struct {
	segment   *Segment
	Direction Direction
	Log       *zerolog.Logger
	Stream    chan *Segment
	Batching  bool
	Batch     []*Segment
	Mu        *sync.Mutex
}

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

			if len(r.segment.Payload) == 904 {
				fmt.Println("here?")
			}

			data2 := r.segment.Payload
			for {
				if len(data2) == 0 {
					break
				}

				var message []any
				remaining, err2 := StandardCborDecoder.UnmarshalFirst(data2, &message)
				if err2 != nil {
					log.Fatal().Msgf("%+v\n", err2)
				}

				if j, err := json.MarshalIndent(message, "", "  "); err == nil {
					fmt.Println(string(j))
				}

				if len(message) < 1 {
					panic("wtf?")
				}

				messageI, err2 := ProtocolToMessage(r.segment.Protocol, Subprotocol(message[0].(uint64)))
				if err2 != nil {
					log.Fatal().Msgf("%+v", errors.WithStack(err2))
				}
				fmt.Printf("MESSAGE: %T\n", messageI)

				if len(data2) == len(remaining) {
					break
				}

				data2 = remaining
			}

			if !r.Batching && r.segment.Protocol == ProtocolBlockFetch {
				batchStart := []byte{0x81, 0x02}
				if bytes.HasPrefix(r.segment.Payload, batchStart) {
					r.Log.Info().Msg("start batch marker found")
					r.Batching = true
					r.Batch = []*Segment{}
					r.segment.Payload = r.segment.Payload[2:]
				}
			}

			if r.Batching {
				r.Log.Debug().Msg("batching segment")
				r.Batch = append(r.Batch, r.segment)
				batchEnd := []byte{0x81, 0x05}
				if bytes.HasSuffix(r.segment.Payload, batchEnd) {
					batchPayload := []byte{}
					for _, b := range r.Batch {
						batchPayload = append(batchPayload, b.Payload...)
					}
					batchPayload = batchPayload[:len(batchPayload)-2]

					r.Log.Debug().Msgf("batch complete\n%x", batchPayload)

					m := &MessageBlock{}
					err = errors.WithStack(cbor.Unmarshal(batchPayload, m))
					if err != nil {
						return
					}

					r.Batching = false
					r.Stream <- &Segment{
						Timestamp:     0,
						Protocol:      ProtocolBlockFetch,
						PayloadLength: 0,
						Payload:       nil,
						Direction:     r.Direction,
						Message:       m,
					}
				}
				r.segment = nil
			} else {
				message, err2 := parseSegmentMessage(r.segment)
				if err2 != nil {
					err = err2
					return
				}
				r.segment.Message = message
				r.Stream <- r.segment
				r.segment = nil
			}
		}
	}

	return
}

func parseSegmentMessage(segment *Segment) (target Message, err error) {
	var temp any

	subprotocol := -1
	if err2 := cbor.Unmarshal(segment.Payload, &temp); err2 == nil {
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
		globalLog.Debug().Msgf("%s %s %T (%d)", segment.Direction.String(), segment.Protocol, target, subprotocol)
		err = errors.WithStack(cbor.Unmarshal(segment.Payload, target))
		if err != nil {
			fmt.Println(temp)
			fmt.Printf("%x\n", segment.Payload)
			fmt.Printf("protocol:    %s\n", segment.Protocol)
			fmt.Printf("subprotocol: %d\n", subprotocol)
			return
		}
		if j, err := json.MarshalIndent(target, "", "  "); err == nil {
			globalLog.Debug().Msg(string(j))
		} else {
			globalLog.Debug().Msgf("failed to marshal segment to json %+v", err)
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
