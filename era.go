package cardano

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
)

const (
	EraByron Era = iota + 1
	EraShelley
	EraAllegra
	EraMary
	EraAlonzo
	EraBabbage
	EraConway
)

type Era uint64

func (e Era) MarshalJSON() ([]byte, error) {
	return []byte(`"` + e.String() + `"`), nil
}

var EraStringMap = map[Era]string{
	EraByron:   "Byron",
	EraShelley: "Shelley",
	EraAllegra: "Allegra",
	EraMary:    "Mary",
	EraAlonzo:  "Alonzo",
	EraBabbage: "Babbage",
	EraConway:  "Conway",
}

func (e Era) String() string {
	if s, ok := EraStringMap[e]; ok {
		return fmt.Sprintf("%s (%d)", s, e)
	}
	return fmt.Sprintf("Unknown (%d)", e)
}

func (e Era) Valid() bool {
	return e >= EraByron && e <= EraConway
}

func (e Era) RawString() string {
	if s, ok := EraStringMap[e]; ok {
		return s
	}
	return "Unknown"
}

func (e *Era) UnmarshalCBOR(data []byte) (err error) {
	if len(data) == 1 {
		*e = Era(data[0])
		return
	}

	if len(data) < 2 {
		return errors.Errorf("not enough bytes for era, got %d, expected 2", len(data))
	}

	if data[0] != 0x82 {
		return errors.Errorf("expected era to start with cbor tag 'binary64, big endian, Typed Array' (0x82), got %x", data[0])
	}

	*e = Era(data[1])

	return
}

type Slot uint

func (s Slot) Time(network Network, block uint64) time.Time {
	params, err := network.Params()
	if err != nil {
		params = &MainNetParams
	}

	blockSeconds := params.StartTime
	lastByronBlock := params.ByronBlock
	byronActualSlot := lastByronBlock
	actualSlot := uint64(s - 0)

	// TODO: Refactor this when you understand it.

	if block > lastByronBlock {
		otherBlocks := actualSlot - byronActualSlot
		blockSeconds += byronActualSlot * ByronProcessTime
		blockSeconds += otherBlocks * ShellyProcessTime
	} else {
		blockSeconds += actualSlot * ByronProcessTime
	}

	// t := time.Time{}
	t := time.Unix(int64(blockSeconds), 0).In(time.UTC)
	return t
}
