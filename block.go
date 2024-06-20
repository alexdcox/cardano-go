package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/fxamacker/cbor/v2"
	"github.com/pkg/errors"
)

type Block struct {
	_    struct{} `cbor:",toarray"`
	Era  Era      `json:"era"`
	Data struct {
		_                      struct{}                `cbor:",toarray"`
		Header                 BlockHeader             `json:"header"`
		TransactionBodies      []TransactionBody       `json:"transactionBodies"`
		TransactionWitnessSets []TransactionWitnessSet `json:"transactionWitnessSets"`
		AuxiliaryData          AuxData                 `json:"auxiliaryData"`
		InvalidTransactions    []uint64                `json:"invalidTransactions"`
	} `json:"data"`
}

type AuxData struct {
	Map any
}

func (a *AuxData) UnmarshalCBOR(bytes []byte) (err error) {
	var i any
	err = errors.WithStack(cbor.Unmarshal(bytes, &i))
	if err != nil {
		return
	}

	a.Map = make(map[uint64]any)

	type KV struct {
		K any
		V any
	}

	var mapToKVArray func(v any) any
	mapToKVArray = func(v any) any {
		reflectV := reflect.ValueOf(v)
		switch reflectV.Kind() {
		case reflect.Ptr:
			return mapToKVArray(reflectV.Elem().Interface())
		case reflect.String,
			reflect.Bool,
			reflect.Int,
			reflect.Int8,
			reflect.Int16,
			reflect.Int32,
			reflect.Int64,
			reflect.Uint8,
			reflect.Uint16,
			reflect.Uint32,
			reflect.Uint64,
			reflect.Float32,
			reflect.Float64:
			return v
		case reflect.Slice:
			var ret []interface{}
			for i := 0; i < reflectV.Len(); i++ {
				ret = append(ret, mapToKVArray(reflectV.Index(i).Interface()))
			}
			return ret
		case reflect.Map:
			ret := []KV{}
			for _, mk := range reflectV.MapKeys() {
				mv := reflectV.MapIndex(mk)
				ret = append(ret, KV{
					K: mapToKVArray(mk.Interface()),
					V: mapToKVArray(mv.Interface()),
				})
			}
			return ret

		case reflect.Struct:
			ret := []KV{}
			for i := 0; i < reflectV.NumField(); i++ {
				fieldName := reflectV.Type().Field(i).Name
				fieldJsonTag := reflectV.Type().Field(i).Tag.Get("json")
				if fieldJsonTag != "" {
					fieldName = strings.Split(fieldJsonTag, ",")[0]
					if len(fieldName) >= 1 {
						fieldName = strings.ToUpper(fieldName[:1]) + fieldName[1:]
					}
					if fieldName == "-" {
						continue
					}
				}
				if fieldJsonTag != "" {
					if strings.Contains(fieldJsonTag, "omitempty") {
						if reflectV.Field(i).IsZero() {
							continue
						}
					}
				}
				if regexp.MustCompile(`^[a-z]`).MatchString(fieldName) {
					// fmt.Println("skip unexported")
					continue
				}
				ret = append(ret, KV{
					K: fieldName,
					V: mapToKVArray(reflectV.Field(i).Interface()),
				})
			}
			return ret
		}
		return fmt.Sprintf("%v", v)
	}

	a.Map = mapToKVArray(i)

	return
}

type BlockHeader struct {
	_    struct{} `cbor:",toarray"`
	Body struct {
		_               struct{}    `cbor:",toarray"`
		Number          int         `json:"number,omitempty"`
		Slot            int         `json:"slot,omitempty"`
		PrevHash        Base58Bytes `json:"prevHash,omitempty"`
		IssuerVkey      Base58Bytes `json:"issuerVkey,omitempty"`
		VrfKey          Base58Bytes `json:"vrfKey,omitempty"`
		VrfResult       VrfCert     `json:"vrfResult,omitempty"`
		Size            int         `json:"size,omitempty"`
		Hash            Base58Bytes `json:"hash,omitempty"`
		OperationalCert struct {
			_         struct{}    `cbor:",toarray"`
			HotVkey   Base58Bytes `json:"hotVkey,omitempty"`
			Sequence  int         `json:"sequence,omitempty"`
			KesPeriod int         `json:"kesPeriod,omitempty"`
			Sigma     Base58Bytes `json:"sigma,omitempty"`
		} `json:"operationalCert"`
		ProtocolVersion struct {
			_   struct{} `cbor:",toarray"`
			Pt1 int      `json:"pt1,omitempty"`
			Pt2 int      `json:"pt2,omitempty"`
		} `json:"protocolVersion"`
	} `json:"body"`
	Signature Base58Bytes `json:"signature,omitempty"`
}

type TransactionBody struct {
	Inputs                []TransactionInput                            `cbor:"0,keyasint" json:"inputs,omitempty"`
	Outputs               []SubtypeOf[TransactionOutput]                `cbor:"1,keyasint" json:"outputs,omitempty"`
	Fee                   int                                           `cbor:"2,keyasint" json:"fee,omitempty"`
	Ttl                   int                                           `cbor:"3,keyasint" json:"ttl,omitempty"`
	Certificates          any                                           `cbor:"4,keyasint" json:"certificates,omitempty"`
	WithdrawalMap         map[cbor.ByteString]uint64                    `cbor:"5,keyasint" json:"withdrawalMap,omitempty"`
	UpdateDI              any                                           `cbor:"6,keyasint" json:"updateDI,omitempty"`
	MetadataHash          Base58Bytes                                   `cbor:"7,keyasint" json:"metadataHash,omitempty"`
	ValidityStartInterval int                                           `cbor:"8,keyasint" json:"validityStartInterval,omitempty"`
	MintMap               map[cbor.ByteString]map[cbor.ByteString]int64 `cbor:"9,keyasint" json:"mintMap,omitempty"`
	ScriptDataHash        Base58Bytes                                   `cbor:"11,keyasint" json:"scriptDataHash,omitempty"`
	CollateralInputs      []CollateralInput                             `cbor:"13,keyasint" json:"collateralInputs,omitempty"`
	RequiredSigners       []Base58Bytes                                 `cbor:"14,keyasint" json:"requiredSigners,omitempty"`
	NetworkId             int                                           `cbor:"15,keyasint" json:"networkId,omitempty"`
	CollateralReturn      SubtypeOf[CollateralReturn]                   `cbor:"16,keyasint" json:"collateralReturn,omitempty"`
	TotalCollateral       int                                           `cbor:"17,keyasint" json:"totalCollateral,omitempty"`
	ReferenceInputs       []TransactionInput                            `cbor:"18,keyasint" json:"referenceInputs,omitempty"`
	VotingProcedures      any                                           `cbor:"19,keyasint" json:"votingProcedures,omitempty"`
	ProposalProcedure     any                                           `cbor:"20,keyasint" json:"proposalProcedure,omitempty"`
	TreasuryValue         any                                           `cbor:"21,keyasint" json:"treasuryValue,omitempty"`
	DonationCoin          uint64                                        `cbor:"22,keyasint" json:"donationCoin,omitempty"`
}

type CollateralInput struct {
	_ struct{}    `cbor:",toarray" json:"_"`
	A Base58Bytes `json:"a,omitempty"`
	B int         `json:"b,omitempty"`
}

type CollateralReturn struct {
	HasSubtypes
}

func (r CollateralReturn) Subtypes() []any {
	return []any{
		&CollateralReturnA{},
		&CollateralReturnB{},
		&CollateralReturnC{},
		&CollateralReturnD{},
	}
}

type CollateralReturnA struct {
	_ struct{} `cbor:",toarray" json:"_"`
	A Base58Bytes
	B AmountData
}

type CollateralReturnB struct {
	_ struct{} `cbor:",toarray" json:"_"`
	A Base58Bytes
	B uint64
}

type CollateralReturnC struct {
	A Base58Bytes `cbor:"0,keyasint"`
	B uint64      `cbor:"1,keyasint"`
}

type CollateralReturnD struct {
	A Base58Bytes `cbor:"0,keyasint"`
	B AmountData  `cbor:"1,keyasint"`
}

type TransactionInput struct {
	_     struct{} `cbor:",toarray"`
	Txid  Base58Bytes
	Index int
}

type TransactionOutput struct {
	HasSubtypes
}

func (t TransactionOutput) Subtypes() []any {
	return []any{
		&TransactionOutputTest{},
		&TransactionOutputTest2{},
		&TransactionOutputTest3{},
		&TransactionOutputMappedExtraArray{},
		&TransactionOutputMappedExtraAddress{},
		&TransactionOutputMapped{},
		&TransactionOutputSimple{},
	}
}

type AmountData struct {
	_        struct{} `cbor:",toarray"`
	Amount   uint64
	Mappings map[cbor.ByteString]map[cbor.ByteString]uint64
}

func (t AmountData) MarshalJSON() ([]byte, error) {
	ret := []any{}

	ret = append(ret, t.Amount)

	mappings := make(map[string]map[string]uint64)

	for k, v := range t.Mappings {
		kHex := fmt.Sprintf("%x", k)
		mappings[kHex] = make(map[string]uint64)
		for k2, v2 := range v {
			k2Hex := string(k2)
			mappings[kHex][k2Hex] = v2
		}
	}

	ret = append(ret, mappings)

	return json.Marshal(ret)
}

type TransactionOutputExtra struct {
	_ struct{} `cbor:",toarray"`
	A uint64
	B Base58Bytes
}

type TransactionOutputTest struct {
	Address    Base58Bytes                      `cbor:"0,keyasint" json:"address"`
	AmountData AmountData                       `cbor:"1,keyasint" json:"amountData"`
	Extra      Optional[TransactionOutputExtra] `cbor:"2,keyasint" json:"extra"`
}

type TransactionOutputTest2 struct {
	Address Base58Bytes `cbor:"0,keyasint" json:"address"`
	Amount  uint64      `cbor:"1,keyasint" json:"amount"`
}

type TransactionOutputTest3 struct {
	_        struct{} `cbor:",toarray"`
	Address  Base58Bytes
	Amount   uint64
	Address2 Base58Bytes
}

type TransactionOutputMappedExtraArray struct {
	_          struct{}    `cbor:",toarray"`
	Address    Base58Bytes `json:"address"`
	AmountData AmountData  `json:"amountData"`
	Extra      []any       `json:"extra"`
}

type TransactionOutputMappedExtraAddress struct {
	_          struct{}    `cbor:",toarray"`
	Address    Base58Bytes `json:"address"`
	AmountData AmountData  `json:"amountData"`
	Extra      Base58Bytes `json:"extra"`
}

type TransactionOutputMapped struct {
	_          struct{}    `cbor:",toarray"`
	Address    Base58Bytes `json:"address"`
	AmountData AmountData  `json:"amountData"`
}

type TransactionOutputSimple struct {
	_       struct{}    `cbor:",toarray"`
	Address Base58Bytes `json:"address"`
	Amount  uint64      `json:"amount"`
}

type TransactionWitnessSet struct {
	VkeyWitness      any `cbor:"0,keyasint" json:"-"`
	NativeScript     any `cbor:"1,keyasint" json:"-"`
	BootstrapWitness any `cbor:"2,keyasint" json:"-"`
	PlutusV1Script   any `cbor:"3,keyasint" json:"-"`
	PlutusData       any `cbor:"4,keyasint" json:"-"`
	Redeemer         any `cbor:"5,keyasint" json:"-"`
	PlutusV2Script   any `cbor:"6,keyasint" json:"-"`
	PlutusV3Script   any `cbor:"7,keyasint" json:"-"`
}

func (p *Point) UnmarshalCBOR(bytes []byte) (err error) {
	var a any

	err = errors.WithStack(cbor.Unmarshal(bytes, &a))
	if err != nil {
		return
	}

	if b, ok := a.([]any); ok {
		if len(b) == 0 {
			return errors.New("empty")
		}
		if len(b) == 2 {
			*p = Point{
				Slot: b[0].(uint64),
				Hash: b[1].([]uint8),
			}
			return
		}
		err = errors.Errorf("unxpected format for point %v", a)
	} else {
		err = errors.Errorf("expected point to be an []any, got %T with value %v", a, a)
	}

	return
}

type VrfCert struct {
	_   struct{}    `cbor:",toarray"`
	Pt1 Base58Bytes `json:"pt1"`
	Pt2 Base58Bytes `json:"pt2"`
}
