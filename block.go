package cardano

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/alexdcox/cbor/v2"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"golang.org/x/crypto/blake2b"
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
	Raw []byte `cbor:"-" json:"-"`
}

func (b *Block) PointAndNumber() (pn PointAndBlockNum, err error) {
	point, err := b.Point()
	if err != nil {
		return
	}
	pn = PointAndBlockNum{
		Point: point,
		Block: b.Data.Header.Body.Number,
	}
	return
}

func (b *Block) Point() (point Point, err error) {
	hash, err := b.Hash()
	if err != nil {
		return
	}

	point = Point{
		Slot: b.Data.Header.Body.Slot,
		Hash: hash,
	}

	return
}

func (b *Block) Hash() (hash HexBytes, err error) {
	headerCbor, err := cbor.Marshal(b.Data.Header)
	if err != nil {
		err = errors.Wrap(err, "failed to marshal header when calculating block hash")
		return
	}

	h := blake2b.Sum256(headerCbor)
	hash = h[:]

	return
}

type AuxData struct {
	Value any
}

func (a *AuxData) JSON() (jsn gjson.Result, err error) {
	jsnString, err := json.Marshal(a)
	if err != nil {
		err = errors.WithStack(err)
		return
	}

	jsn = gjson.ParseBytes(jsnString)

	return
}

func (a *AuxData) MarshalJSON() (bytes []byte, err error) {
	if a.Value == nil {
		return []byte("{}"), nil
	}

	if m, ok := Cast[KVSlice](a.Value); ok {
		// If we've extracted the cbor.Tag, just return the content and drop the tag number
		if len(m) == 2 && m[1].K == "Content" {
			return json.Marshal(m[1].V)
		}
		return json.Marshal(m)
	}

	return json.Marshal(a.Value)
}

func (a *AuxData) MarshalCBOR() (bytes []byte, err error) {
	if m, ok := Cast[KVSlice](a.Value); ok {
		if len(m) == 2 && m[1].K == "Content" {

			return cbor.Marshal(cbor.Tag{
				Number:  m[0].V.(uint64),
				Content: m[1].V,
			})
		}
		return cbor.Marshal(m)
	}

	if b, ok := a.Value.([]byte); ok {
		return b, nil
	}

	return cbor.Marshal(a.Value)
}

type KV struct {
	K any
	V any
}

type KVSlice []KV

func (s KVSlice) MarshalCBOR() ([]byte, error) {
	m := map[any]any{}

	for _, elem := range s {
		converted := elem.V
		if v2, ok := Cast[KVSlice](converted); ok {
			converted = v2
		}
		m[elem.K] = converted
	}

	ret := []byte{byte(0xA0 + len(m))}
	for k, v := range m {
		kCbor, err := cbor.Marshal(k)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		vCbor, err := cbor.Marshal(v)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		ret = append(ret, kCbor...)
		ret = append(ret, vCbor...)
	}

	return ret, nil
}

func (s KVSlice) MarshalJSON() ([]byte, error) {
	v := map[string]any{}

	for _, elem := range s {
		converted := elem.V
		if v2, ok := Cast[KVSlice](converted); ok {
			converted = v2
		}
		v[fmt.Sprint(elem.K)] = converted
	}

	return json.Marshal(v)
}

func (a *AuxData) UnmarshalCBOR(bytes []byte) (err error) {
	var i any
	err = errors.WithStack(cbor.Unmarshal(bytes, &i))
	if err != nil {
		return
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
			ret := KVSlice{}
			for _, mk := range reflectV.MapKeys() {
				mv := reflectV.MapIndex(mk)
				ret = append(ret, KV{
					K: mapToKVArray(mk.Interface()),
					V: mapToKVArray(mv.Interface()),
				})
			}
			return ret

		case reflect.Struct:
			ret := KVSlice{}
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

	a.Value = mapToKVArray(i)

	return
}

type BlockHeader struct {
	_    struct{} `cbor:",toarray"`
	Body struct {
		_               struct{} `cbor:",toarray"`
		Number          uint64   `json:"number,omitempty"`
		Slot            uint64   `json:"slot,omitempty"`
		PrevHash        HexBytes `json:"prevHash,omitempty"`
		IssuerVkey      HexBytes `json:"issuerVkey,omitempty"`
		VrfKey          HexBytes `json:"vrfKey,omitempty"`
		VrfResult       VrfCert  `json:"vrfResult,omitempty"`
		BodySize        uint64   `json:"bodySize,omitempty"`
		BodyHash        HexBytes `json:"bodyHash,omitempty"`
		OperationalCert struct {
			_         struct{} `cbor:",toarray"`
			HotVkey   HexBytes `json:"hotVkey,omitempty"`
			Sequence  uint64   `json:"sequence,omitempty"`
			KesPeriod int64    `json:"kesPeriod,omitempty"`
			Sigma     HexBytes `json:"sigma,omitempty"`
		} `json:"operationalCert"`
		ProtocolVersion struct {
			_   struct{} `cbor:",toarray"`
			Pt1 int64    `json:"pt1,omitempty"`
			Pt2 int64    `json:"pt2,omitempty"`
		} `json:"protocolVersion"`
	} `json:"body"`
	Signature HexBytes `json:"signature,omitempty"`
}

type TransactionBody struct {
	Inputs                []TransactionInput                            `cbor:"0,keyasint,omitempty" json:"inputs,omitempty"`
	Outputs               []SubtypeOf[TransactionOutput]                `cbor:"1,keyasint,omitempty" json:"outputs,omitempty"`
	Fee                   uint64                                        `cbor:"2,keyasint,omitempty" json:"fee,omitempty"`
	Ttl                   int64                                         `cbor:"3,keyasint,omitempty" json:"ttl,omitempty"`
	Certificates          any                                           `cbor:"4,keyasint,omitempty" json:"certificates,omitempty"`
	WithdrawalMap         map[cbor.ByteString]uint64                    `cbor:"5,keyasint,omitempty" json:"withdrawalMap,omitempty"`
	UpdateDI              any                                           `cbor:"6,keyasint,omitempty" json:"updateDI,omitempty"`
	AuxiliaryDataHash     HexBytes                                      `cbor:"7,keyasint,omitempty" json:"auxiliaryDataHash,omitempty"`
	ValidityStartInterval int64                                         `cbor:"8,keyasint,omitempty" json:"validityStartInterval,omitempty"`
	MintMap               map[cbor.ByteString]map[cbor.ByteString]int64 `cbor:"9,keyasint,omitempty" json:"mintMap,omitempty"`
	ScriptDataHash        HexBytes                                      `cbor:"11,keyasint,omitempty" json:"scriptDataHash,omitempty"`
	CollateralInputs      []CollateralInput                             `cbor:"13,keyasint,omitempty" json:"collateralInputs,omitempty"`
	RequiredSigners       []HexBytes                                    `cbor:"14,keyasint,omitempty" json:"requiredSigners,omitempty"`
	NetworkId             int64                                         `cbor:"15,keyasint,omitempty" json:"networkId,omitempty"`
	CollateralReturn      *SubtypeOf[CollateralReturn]                  `cbor:"16,keyasint,omitempty" json:"collateralReturn,omitempty"`
	TotalCollateral       int64                                         `cbor:"17,keyasint,omitempty" json:"totalCollateral,omitempty"`
	ReferenceInputs       []TransactionInput                            `cbor:"18,keyasint,omitempty" json:"referenceInputs,omitempty"`
	VotingProcedures      any                                           `cbor:"19,keyasint,omitempty" json:"votingProcedures,omitempty"`
	ProposalProcedure     any                                           `cbor:"20,keyasint,omitempty" json:"proposalProcedure,omitempty"`
	TreasuryValue         any                                           `cbor:"21,keyasint,omitempty" json:"treasuryValue,omitempty"`
	DonationCoin          uint64                                        `cbor:"22,keyasint,omitempty" json:"donationCoin,omitempty"`
}

func (tb *TransactionBody) IterateOutputs(cb func(index int, output *TransactionOutputGeneric, err error) error) (err error) {
	for index, outputSubtypeWrapper := range tb.Outputs {
		output, err2 := new(TransactionOutput).ToGeneric(outputSubtypeWrapper.Subtype)
		if err2 != nil {
			err = err2
			return
		}

		err = cb(index, output, err)
		if err != nil {
			return
		}
	}

	return
}

func (tb *TransactionBody) Hash() (hash HexBytes, err error) {
	bytes, err := cbor.Marshal(tb)
	if err != nil {
		err = errors.WithStack(err)
		return
	}
	_hash := blake2b.Sum256(bytes)
	hash = _hash[:]
	return
}

type CollateralInput struct {
	_ struct{} `cbor:",toarray" json:"_"`
	A HexBytes `json:"a,omitempty"`
	B int64    `json:"b,omitempty"`
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
	A HexBytes
	B AmountData
}

type CollateralReturnB struct {
	_ struct{} `cbor:",toarray" json:"_"`
	A HexBytes
	B uint64
}

type CollateralReturnC struct {
	A HexBytes `cbor:"0,keyasint"`
	B uint64   `cbor:"1,keyasint"`
}

type CollateralReturnD struct {
	A HexBytes   `cbor:"0,keyasint"`
	B AmountData `cbor:"1,keyasint"`
}

type TransactionInput struct {
	_     struct{} `cbor:",toarray"`
	Txid  HexBytes `json:"txid"`
	Index int64    `json:"index"`
}

type TransactionOutput struct {
	HasSubtypes
}

func (t TransactionOutput) Subtypes() []any {
	return []any{
		&TransactionOutputA{},
		&TransactionOutputB{},
		&TransactionOutputC{},
		&TransactionOutputD{},
		&TransactionOutputE{},
		&TransactionOutputF{},
		&TransactionOutputG{},
	}
}

func (TransactionOutput) ToGeneric(in any) (out *TransactionOutputGeneric, err error) {
	switch output := in.(type) {
	case TransactionOutputA:
		return &TransactionOutputGeneric{
			Address: output.Address,
			Amount:  output.AmountData.Amount,
		}, nil

	case TransactionOutputB:
		return &TransactionOutputGeneric{
			Address: output.Address,
			Amount:  output.Amount,
		}, nil

	case TransactionOutputC:
		return &TransactionOutputGeneric{
			Address: output.Address,
			Amount:  output.Amount,
		}, nil

	case TransactionOutputD:
		return &TransactionOutputGeneric{
			Address: output.Address,
			Amount:  output.AmountData.Amount,
		}, nil

	case TransactionOutputE:
		return &TransactionOutputGeneric{
			Address: output.Address,
			Amount:  output.AmountData.Amount,
		}, nil

	case TransactionOutputF:
		return &TransactionOutputGeneric{
			Address: output.Address,
			Amount:  output.AmountData.Amount,
		}, nil

	case TransactionOutputG:
		return &TransactionOutputGeneric{
			Address: output.Address,
			Amount:  output.Amount,
		}, nil
	}

	return nil, errors.Errorf("unknown transaction output type: %T", in)
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
	B HexBytes
}

type TransactionOutputA struct {
	Address    Address                          `cbor:"0,keyasint" json:"address"`
	AmountData AmountData                       `cbor:"1,keyasint" json:"amountData"`
	Extra      Optional[TransactionOutputExtra] `cbor:"2,keyasint" json:"extra"`
}

type TransactionOutputB struct {
	Address Address `cbor:"0,keyasint" json:"address"`
	Amount  uint64  `cbor:"1,keyasint" json:"amount"`
}

type TransactionOutputC struct {
	_        struct{} `cbor:",toarray"`
	Address  Address
	Amount   uint64
	Address2 Address
}

type TransactionOutputD struct {
	_          struct{}   `cbor:",toarray"`
	Address    Address    `json:"address"`
	AmountData AmountData `json:"amountData"`
	Extra      []any      `json:"extra"`
}

type TransactionOutputE struct {
	_          struct{}   `cbor:",toarray"`
	Address    Address    `json:"address"`
	AmountData AmountData `json:"amountData"`
	Extra      HexBytes   `json:"extra"`
}

type TransactionOutputF struct {
	_          struct{}   `cbor:",toarray"`
	Address    Address    `json:"address"`
	AmountData AmountData `json:"amountData"`
}

type TransactionOutputG struct {
	_       struct{} `cbor:",toarray"`
	Address Address  `json:"address"`
	Amount  uint64   `json:"amount"`
}

type TransactionOutputGeneric struct {
	Address Address
	Amount  uint64
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
			*p = Point{}
			return
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
	_   struct{} `cbor:",toarray"`
	Pt1 HexBytes `json:"pt1"`
	Pt2 HexBytes `json:"pt2"`
}
