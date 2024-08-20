package cardano

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"

	"github.com/fxamacker/cbor/v2"
	"github.com/pkg/errors"
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
	messages      []Message
}

func (s *Segment) Messages() (messages []Message, err error) {
	data := s.Payload

	for {
		if len(data) == 0 {
			break
		}

		var message []any
		remaining, err2 := StandardCborDecoder.UnmarshalFirst(data, &message)
		if err2 != nil {
			err = errors.Wrap(err2, "failed to unmarshal first cbor")
			return
		}

		messageI, err2 := ProtocolToMessage(s.Protocol, Subprotocol(message[0].(uint64)))
		if err2 != nil {
			err = err2
			return
		}

		if _, is := messageI.(*MessageStartBatch); is {
			fmt.Println("@message start batch")
		}

		_, err = StandardCborDecoder.UnmarshalFirst(data, messageI)
		if err != nil {
			err = errors.Wrap(err, "failed to unmarshal cbor to struct")
			return
		}

		messages = append(messages, messageI)

		if len(data) == len(remaining) {
			break
		}

		data = remaining
	}

	return
}

func (s *Segment) MarshalDataItem() (out []byte, err error) {
	for _, message := range s.messages {
		messageCbor, err2 := cbor.Marshal(message)
		if err2 != nil {
			err = errors.WithStack(err2)
			return
		}
		s.Payload = append(s.Payload, messageCbor...)
	}

	timestampBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(timestampBytes, s.Timestamp)

	protocolBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(protocolBytes, uint16(s.Protocol))

	s.PayloadLength = uint16(len(s.Payload))
	payloadLengthBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(payloadLengthBytes, s.PayloadLength)

	buf := &bytes.Buffer{}
	buf.Write(timestampBytes)
	buf.Write(protocolBytes)
	buf.Write(payloadLengthBytes)
	buf.Write(s.Payload)

	out = buf.Bytes()

	return
}

func (s *Segment) Complete() bool {
	return int(s.PayloadLength) == len(s.Payload)
}

func (s *Segment) AddMessage(message Message) (err error) {
	protocol, ok := MessageProtocolMap[reflect.TypeOf(message)]
	if !ok {
		return errors.Errorf("no protocol for message %T", message)
	}

	if len(s.messages) > 0 && protocol != s.Protocol {
		return errors.Errorf("cannot add messages with different protocols in the same segment")
	}

	s.Protocol = protocol
	s.messages = append(s.messages, message)

	return
}
