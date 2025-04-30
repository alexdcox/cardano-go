package cardano

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/alexdcox/cbor/v2"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

type MessageReader struct {
	timestamp     []byte
	protocol      uint16
	length        uint16
	buffer        []byte
	partialHeader []byte
	batchBuffer   []byte
	batching      bool
	cbor          cbor.DecMode
	log           *zerolog.Logger
}

func NewMessageReader(log *zerolog.Logger) *MessageReader {
	return &MessageReader{
		cbor: StandardCborDecoder,
		log:  log,
	}
}

func (r *MessageReader) readHeader(data []byte) (timestamp []byte, protocol uint16, byteLen uint16, err error) {
	if len(data) < 8 {
		err = errors.Errorf("header must be at least 8 bytes, got %d bytes", len(data))
		return
	}

	timestamp = data[:4]

	protocolBytes := data[4:6]
	protocol = binary.BigEndian.Uint16(protocolBytes)
	if protocolBytes[0] == 0x80 {
		protocol -= 32768
	}

	if protocol > 10 {
		err = errors.Errorf(
			"protocol %d is obviously wrong, read from hex: %x",
			protocol,
			protocolBytes,
		)
		return
	}

	byteLenBytes := data[6:8]
	byteLen = binary.BigEndian.Uint16(byteLenBytes)

	r.log.Trace().Msgf("new segment. timestamp %x, protocol %v, byteLen %v", timestamp, protocol, byteLen)

	return
}

func (r *MessageReader) reset() {
	r.buffer = []byte{}
	r.timestamp = []byte{}
	r.protocol = 0
	r.length = 0
}

func (r *MessageReader) Read(data []byte) (messages []Message, err error) {
	r.log.Trace().Msgf("read: %x", data)

	i := 0
	var nextMessages []Message

	for {
		if i == len(data) {
			break
		}
		if i > len(data) {
			err = errors.New("read more bytes than exists?")
			return
		}

		if len(r.buffer) == 0 {
			if len(data[i:]) <= 8 {
				r.log.Debug().Msgf("partial/lone header detected [%d:%d] %x", i, len(data), data[i:])
				r.partialHeader = append(r.partialHeader, data[i:]...)
				return
			}

			headerData := data[i:]

			if len(r.partialHeader) > 0 {
				r.log.Debug().Msg("prepending partial header")
				headerData = append(r.partialHeader, data[i:]...)
			}

			r.timestamp, r.protocol, r.length, err = r.readHeader(headerData)
			if err != nil {
				return
			}

			i += 8 - len(r.partialHeader)
			r.partialHeader = []byte{}
		}

		bytesNeedRead := int(r.length) - len(r.buffer)
		bytesCanRead := len(data[i:])
		bytesShouldRead := bytesNeedRead
		if bytesShouldRead > bytesCanRead {
			bytesShouldRead = bytesCanRead
		}

		r.log.Trace().Msgf("read data to segment [%d:%d]", i, i+bytesShouldRead)
		r.buffer = append(r.buffer, data[i:i+bytesShouldRead]...)
		i += bytesShouldRead

		if len(r.buffer) == int(r.length) {
			if r.batching {
				r.log.Trace().Msg("batched segment complete")
				r.batchBuffer = append(r.batchBuffer, r.buffer...)

				blockMessages, remaining, err2 := r.nextBlocks(r.batchBuffer)
				if err2 != nil {
					err = err2
					return
				}

				messages = append(messages, blockMessages...)

				r.batchBuffer = remaining

				if len(messages) > 0 {
					if _, batchDone := messages[len(messages)-1].(*MessageBatchDone); batchDone {
						if len(r.batchBuffer) > 0 {
							err = errors.Errorf(
								"expected batch done message to complete batch buffer, have %d bytes remaining in buffer\n%x",
								len(r.batchBuffer),
								r.batchBuffer)
							return
						}

						r.batching = false
					}
				}

				r.reset()
				continue
			} else {
				r.log.Trace().Msg("segment complete")
			}

			var remaining []byte
			nextMessages, remaining, err = r.nextMessages(r.buffer)
			if err != nil {
				return
			}
			messages = append(messages, nextMessages...)

			if r.batching {
				r.batchBuffer = remaining
			}

			r.reset()
		}
	}

	return
}

// nextBlocks returns an array of MessageBlock and MessageBatchDone
func (r *MessageReader) nextBlocks(data []byte) (messages []Message, remaining []byte, err error) {
	blockStart := []byte{0x82, 0x04}
	_ = blockStart
	batchStart := []byte{0x82, 0x02}
	_ = batchStart
	batchEnd := []byte{0x81, 0x05}

	i := 0

	defer func() {
		remaining = data[i:]
	}()

	test := [][]byte{}
	test2 := data

	for {
		if len(data[i:]) == 0 {
			break
		}

		if bytes.HasPrefix(data[i:], batchEnd) {
			r.log.Trace().Msg("read batch done")
			i += 2
			messages = append(messages, &MessageBatchDone{})

			if len(data[i:]) > 0 {
				i += len(data[i:])
			}

			return
		}

		if !bytes.HasPrefix(data[i:], blockStart) {
			printFirstTenBytesHex := func(data []byte) string {
				n := len(data)
				if n > 10 {
					n = 10
				}
				return fmt.Sprintf("%x", data[:n])
			}
			err = errors.Errorf("expecting batch to have block start 0x8204, first 10 bytes are: %s at index %d", printFirstTenBytesHex(data[i:]), i)
			for x, t := range test {
				r.log.Debug().Msgf("\n%d --> %x", x, t)
			}
			r.log.Debug().Msgf("\nbatchBuffer:\n%x", test2)
			return
		}

		nextMessage := &MessageBlock{}

		left, err2 := cbor.UnmarshalFirst(data[i:], nextMessage)
		if err2 != nil {
			if err2.Error() == "unexpected EOF" {
				return
			}
			err = errors.WithStack(err2)
			return
		}

		bytesRead := len(data[i:]) - len(left)

		r.log.Trace().Msgf("read block message from buffer[%d:%d/%d]", i, i+bytesRead, len(data))

		messages = append(messages, nextMessage)
		test = append(test, data[i:i+bytesRead])

		i += bytesRead
	}

	return
}

func (r *MessageReader) nextMessages(data []byte) (messages []Message, remaining []byte, err error) {
	for {
		if len(data) == 0 {
			break
		}

		var msg Message
		msg, remaining, err = r.nextMessage(data)
		if err != nil {
			err = err
			return
		}
		if msg == nil {
			err = errors.New("expected message got nil")
			return
		}

		if _, is := msg.(*MessageStartBatch); is {
			r.log.Trace().Msg("read batch start")
			r.batching = true
		}

		messages = append(messages, msg)
		data = remaining

		if r.batching {
			return
		}
	}

	return
}

func (r *MessageReader) nextMessage(data []byte) (message Message, remaining []byte, err error) {
	if len(data) == 0 {
		return
	}

	var a []any
	remaining, err = r.cbor.UnmarshalFirst(data, &a)
	if err != nil {
		r.log.Debug().Msgf("unmarshal cbor error: %x", data)
		err = errors.WithStack(err)
		return
	}

	if len(a) < 1 {
		err = errors.New("expected at least one element in the cbor array")
		return
	}

	subprotocol, ok := a[0].(uint64)
	if !ok {
		err = errors.New("expected subprotocol uint64 to be the first element in the cbor array")
		return
	}

	message, err = ProtocolToMessage(Protocol(r.protocol), Subprotocol(subprotocol))
	if err != nil {
		return
	}

	read := data[:len(data)-len(remaining)]

	err = r.cbor.Unmarshal(read, message)
	if err != nil {
		err = errors.Wrapf(err, "failed to unmarshal message %T from data: %x", message, read)
		return
	}

	message.SetRaw(read)

	return
}
