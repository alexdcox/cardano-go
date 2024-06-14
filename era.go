package main

import (
	"fmt"

	"github.com/pkg/errors"
)

const (
	EraByron Era = iota
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
