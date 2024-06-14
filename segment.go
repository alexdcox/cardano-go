package main

import (
	"encoding/binary"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

type Segment struct {
	Timestamp     uint32
	Protocol      Protocol
	PayloadLength uint16
	Payload       []byte
}

func (s *Segment) UnmarshalDataItem(data []byte) (length int, err error) {
	if len(data) < 8 {
		err = errors.Errorf("short segment data len=%d", len(data))
		return
	}

	timestampBytes := data[0:4]
	s.Timestamp = binary.BigEndian.Uint32(timestampBytes)

	protocolBytes := data[4:6]
	s.Protocol = Protocol(binary.BigEndian.Uint16(protocolBytes))
	if protocolBytes[0] == 0x80 {
		s.Protocol -= 32768
	}

	if s.Protocol > 10 {
		err = errors.Errorf(
			"protocol %d is obviously wrong, read from hex: %x",
			s.Protocol,
			protocolBytes)
		return
	}

	byteLenBytes := data[6:8]
	s.PayloadLength = binary.BigEndian.Uint16(byteLenBytes)
	// if byteLenBytes[0] == 0x80 {
	// 	s.PayloadLength -= 32768
	// }

	// if len(data)-8 != int(byteLenInt) {
	// 	log.Fatalf("%+v", errors.Errorf(
	// 		"expected %d bytes remaining, actually %d",
	// 		int(byteLenInt),
	// 		len(data[n:])-8))
	// }

	i := 8 + int(s.PayloadLength)
	if i > len(data) {
		i = len(data)
	}
	s.Payload = data[8:i]
	// first, _, err := cbor.DiagnoseFirst(payload)
	// if err != nil {
	// 	log.Fatalf("%+v", errors.WithStack(err))
	// }
	// fmt.Println(first)
	// fmt.Println("------------------------------")

	// if i < len(data) {
	// 	log.Warn().Msgf(
	// 		"segment parsed with %d additional bytes remaining",
	// 		len(data)-len(s.Payload))
	// }

	length = i

	return
}

func (s *Segment) Complete() bool {
	return int(s.PayloadLength) == len(s.Payload)
}

func (s *Segment) Append(data []byte) (read int) {
	remaining := int(s.PayloadLength) - len(s.Payload)

	if len(data) <= remaining {
		s.Payload = append(s.Payload, data...)
		read = len(data)
	} else {
		s.Payload = append(s.Payload, data[:remaining]...)
		read = remaining
		// fmt.Printf("segment appended with %d bytes remaining\n", remaining)
	}

	return
}

type SegmentReader struct {
	s         *Segment
	hasInit   bool
	Direction string
	Log       zerolog.Logger
	// Stream      chan *Segment
	// StreamCount uint64
}

func (r *SegmentReader) Init() {
	// r.Stream = make(chan *Segment, 100)
}

func (r *SegmentReader) Read(p []byte) (n int, err error) {
	if !r.hasInit {
		r.Init()
	}

	r.Log.Info().Msgf("%s %d bytes", r.Direction, len(p))

	printSegmentStatus := func() {
		r.Log.Trace().Msgf(
			"segment %d(%d)/%d(%d), complete: %t, read: %d",
			len(r.s.Payload),
			len(r.s.Payload)+8,
			r.s.PayloadLength,
			r.s.PayloadLength+8,
			r.s.Complete(),
			n,
		)
	}

	handleIfComplete := func() bool {
		if r.s.Complete() {
			r.Log.Debug().Msgf("segment complete")
			_, err = handleSegment(r.s)
			if err != nil {
				log.Fatal().Msgf("%+v", errors.WithStack(err))
			}
			// r.Stream <- r.s
			r.s = nil
			return true
		}
		return false
	}

	for n < len(p) {
		if r.s == nil {
			r.s = &Segment{}
			read, err2 := r.s.UnmarshalDataItem(p[n:])
			if err2 != nil {
				err = err2
				return
			}
			n += read
			r.Log.Debug().Msg("new segment")
			printSegmentStatus()
			if handleIfComplete() {
				continue
			}
		}

		if n >= len(p) {
			break
		}

		r.Log.Debug().Msgf("append segment with %d bytes", len(p[n:]))
		n += r.s.Append(p[n:])
		printSegmentStatus()
		handleIfComplete()
	}

	r.Log.Debug().Msg("--------------------------------------------------")

	return
}
