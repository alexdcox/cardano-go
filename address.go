package cardano

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"

	ogbech "github.com/btcsuite/btcutil/bech32"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
)

type Address []byte

func (a Address) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, a)), nil
}

func (a Address) String() string {
	return hex.EncodeToString(a)
}

func (a Address) Header() (header AddressHeader, err error) {
	if len(a) == 0 {
		err = errors.New("cannot get header for empty address")
		return
	}
	header = AddressHeader(a[0])
	return
}

func (a Address) Type() (typ AddressType, err error) {
	header, err := a.Header()
	if err != nil {
		return
	}
	return header.Type()
}

func (a Address) Network() (net AddressHeaderNetwork, err error) {
	header, err := a.Header()
	if err != nil {
		return
	}
	return header.Network()
}

func (a Address) Bech32String(network Network) (encoded string, err error) {
	params, err := network.Params()
	if err != nil {
		return
	}

	encoded, err = bech32.ConvertAndEncode(params.AddressPrefix, a)
	if err != nil {
		err = errors.Errorf("failed to convert to bech32: %+v", err)
		return
	}

	return
}

func (a *Address) Binary() (out []byte, err error) {
	bech, err := a.Bech32String(NetworkMainNet)
	if err != nil {
		return
	}

	_, data, err := ogbech.Decode(bech)
	if err != nil {
		err = errors.Wrap(err, "failed to decode bech32 address")
		return
	}

	converted, err := ogbech.ConvertBits(data, 5, 8, false)
	if err != nil {
		err = errors.Wrap(err, "failed to convert bits")
		return
	}

	return converted, nil
}

func (a *Address) ParseBech32String(encoded string, network Network) (err error) {
	prefix, addr, err := bech32.DecodeAndConvert(encoded)
	if err != nil {
		return errors.Errorf("failed to decode bech32 address: %+v", err)
	}

	header := AddressHeader(addr[0])

	format, err := header.Format()
	if err != nil {
		return err
	}

	params, err := network.Params()
	if err != nil {
		return
	}

	if prefix != params.AddressPrefix && prefix != params.DelegationPrefix {
		return errors.Errorf(
			"invalid payment prefix: expected '%s' or '%s' (%s), got '%s'",
			params.AddressPrefix,
			params.DelegationPrefix,
			format.Type,
			prefix,
		)
	}

	*a = addr
	return nil
}

// DecodeAddress accepts a Bech32 address string and parses it into a Cardano
// Address interface.
func DecodeAddress(address string, network Network) (decoded Address, err error) {
	addr := &Address{}
	err = addr.ParseBech32String(address, network)
	decoded = *addr
	return
}

// EncodeAddress accepts an Ed25519 public key and returns a Cardano Address
// interface.
func EncodeAddress(publicKey []byte, net Network, typ AddressType) (addr Address, err error) {
	params, err := net.Params()
	if err != nil {
		return
	}

	format, err := typ.Format()
	if err != nil {
		return
	}

	if len(publicKey) != ed25519.PublicKeySize {
		err = errors.Errorf(
			"expected a %d length ed25519 public key, got %d bytes",
			ed25519.PublicKeySize,
			len(publicKey))
		return
	}

	hash, err := Blake2bSum224(publicKey)
	if err != nil {
		return
	}

	networkBits := AddressHeaderNetworkMainnet
	if params.Name != NetworkMainNet {
		networkBits = AddressHeaderNetworkTestnet
	}

	header := new(AddressHeader)
	header.SetType(format.HeaderType)
	header.SetNetwork(networkBits)

	addr = append([]byte{byte(*header)}, hash[:]...)

	return
}

func Blake2bSum224(data []byte) (hash []byte, err error) {
	hash = make([]byte, 28) // 224 bits = 28 bytes
	h, err := blake2b.New(28, nil)
	if err != nil {
		err = errors.Wrap(err, "failed to create blake2b hash")
		return
	}
	h.Write(data)
	h.Sum(hash[:0])
	return hash, nil
}

const (
	AddressTypePaymentAndStake AddressType = iota
	AddressTypeScriptAndStake
	AddressTypePaymentAndScript
	AddressTypeScriptAndScript
	AddressTypePaymentAndPointer
	AddressTypeScriptAndPointer
	AddressTypePayment
	AddressTypeScript
	AddressTypeStakeReward
	AddressTypeScriptReward
)

type AddressType int

func (a AddressType) String() string {
	switch a {
	case AddressTypePaymentAndStake:
		return "payment and stake"
	case AddressTypeScriptAndStake:
		return "script and stake"
	case AddressTypePaymentAndScript:
		return "payment and script"
	case AddressTypeScriptAndScript:
		return "script and script"
	case AddressTypePaymentAndPointer:
		return "payment and pointer"
	case AddressTypeScriptAndPointer:
		return "script and pointer"
	case AddressTypePayment:
		return "payment"
	case AddressTypeScript:
		return "script"
	case AddressTypeStakeReward:
		return "stake reward"
	case AddressTypeScriptReward:
		return "script reward"
	default:
		return "invalid"
	}
}

func (a AddressType) Format() (format AddressFormat, err error) {
	for _, t := range AddressTypeMap {
		if t.Type == a {
			return t, nil
		}
	}
	err = errors.Errorf("invalid address type '%d'", a)
	return
}

type AddressFormat struct {
	Type       AddressType
	HeaderType AddressHeaderType
}

var AddressTypeMap = map[AddressType]AddressFormat{
	AddressTypePaymentAndStake: {
		Type:       AddressTypePaymentAndStake,
		HeaderType: AddressHeaderTypeStakePaymentKeyHash,
	},
	AddressTypeScriptAndStake: {
		Type:       AddressTypeScriptAndStake,
		HeaderType: AddressHeaderTypeStakeScriptHash,
	},
	AddressTypePaymentAndScript: {
		Type:       AddressTypePaymentAndScript,
		HeaderType: AddressHeaderTypeScriptPaymentKeyHash,
	},
	AddressTypeScriptAndScript: {
		Type:       AddressTypeScriptAndScript,
		HeaderType: AddressHeaderTypeScriptScriptHash,
	},
	AddressTypePaymentAndPointer: {
		Type:       AddressTypePaymentAndPointer,
		HeaderType: AddressHeaderTypePointerPaymentKeyHash,
	},
	AddressTypeScriptAndPointer: {
		Type:       AddressTypeScriptAndPointer,
		HeaderType: AddressHeaderTypePointerScriptHash,
	},
	AddressTypePayment: {
		Type:       AddressTypePayment,
		HeaderType: AddressHeaderTypePaymentKeyHash,
	},
	AddressTypeScript: {
		Type:       AddressTypeScript,
		HeaderType: AddressHeaderTypeScriptHash,
	},
	AddressTypeStakeReward: {
		Type:       AddressTypeStakeReward,
		HeaderType: AddressHeaderTypeStakeRewardHash,
	},
	AddressTypeScriptReward: {
		Type:       AddressTypeScriptReward,
		HeaderType: AddressHeaderTypeScriptRewardHash,
	},
}

var (
	AddressHeaderTypeStakePaymentKeyHash   AddressHeaderType = [4]byte{0, 0, 0, 0}
	AddressHeaderTypeStakeScriptHash       AddressHeaderType = [4]byte{0, 0, 0, 1}
	AddressHeaderTypeScriptPaymentKeyHash  AddressHeaderType = [4]byte{0, 0, 1, 0}
	AddressHeaderTypeScriptScriptHash      AddressHeaderType = [4]byte{0, 0, 1, 1}
	AddressHeaderTypePointerPaymentKeyHash AddressHeaderType = [4]byte{0, 1, 0, 0}
	AddressHeaderTypePointerScriptHash     AddressHeaderType = [4]byte{0, 1, 0, 1}
	AddressHeaderTypePaymentKeyHash        AddressHeaderType = [4]byte{0, 1, 1, 0}
	AddressHeaderTypeScriptHash            AddressHeaderType = [4]byte{0, 1, 1, 1}
	AddressHeaderTypeStakeRewardHash       AddressHeaderType = [4]byte{1, 1, 1, 0}
	AddressHeaderTypeScriptRewardHash      AddressHeaderType = [4]byte{1, 1, 1, 1}

	AddressHeaderNetworkTestnet AddressHeaderNetwork = [4]byte{0, 0, 0, 0}
	AddressHeaderNetworkMainnet AddressHeaderNetwork = [4]byte{0, 0, 0, 1}
)

type (
	AddressHeader        byte
	AddressHeaderType    [4]byte
	AddressHeaderNetwork [4]byte
)

func (a AddressHeaderNetwork) String() string {
	switch a {
	case AddressHeaderNetworkTestnet:
		return "testnet"
	case AddressHeaderNetworkMainnet:
		return "mainnet"
	default:
		return "unknown"
	}
}

func (a AddressHeader) toBytes() (extracted [8]byte) {
	for i := 0; i < 8; i++ {
		if a&(1<<i) > 0 {
			extracted[7-i] = 1
		}
	}
	return
}

func (a *AddressHeader) fromBytes(bitAsBytes [8]byte) {
	x := byte(0)
	for i := 0; i < 8; i++ {
		if bitAsBytes[7-i] > 0 {
			x |= 1 << i
		}
	}
	*a = AddressHeader(x)
}

func (a AddressHeader) String() string {
	typ, err := a.Type()
	if err != nil {
		return fmt.Sprintf("%08b (invalid)", a)
	}
	network, err := a.Network()
	if err != nil {
		return fmt.Sprintf("%08b (invalid)", a)
	}
	return fmt.Sprintf("%s/%s | 0x%x | %08b", typ, network, byte(a), a)
}

func (a AddressHeader) Network() (network AddressHeaderNetwork, err error) {
	header := a.toBytes()
	for _, n := range []AddressHeaderNetwork{
		AddressHeaderNetworkMainnet,
		AddressHeaderNetworkTestnet,
	} {
		if bytes.HasSuffix(header[:], n[:]) {
			return n, nil
		}
	}
	err = errors.Errorf("invalid network bits in address header: %08b", a)
	return
}

func (a AddressHeader) Validate() (err error) {
	if _, networkErr := a.Network(); networkErr != nil {
		return networkErr
	}
	if _, typeErr := a.Type(); typeErr != nil {
		return typeErr
	}
	return
}

func (a AddressHeader) Valid() bool {
	return a.Validate() == nil
}

func (a AddressHeader) Type() (typ AddressType, err error) {
	header := a.toBytes()
	for _, t := range AddressTypeMap {
		if bytes.HasPrefix(header[:], t.HeaderType[:]) {
			return t.Type, nil
		}
	}
	err = errors.Errorf("invalid type bits in address header: %08b", a)
	return
}

func (a AddressHeader) Format() (format AddressFormat, err error) {
	typ, err := a.Type()
	if err != nil {
		return
	}
	return typ.Format()
}

func (a AddressHeader) IsForNetwork(params *NetworkParams) (match bool, err error) {
	network, err := a.Network()
	if err != nil {
		return
	}
	switch network {
	case AddressHeaderNetworkMainnet:
		match = params.Name == MainNetParams.Name
	case AddressHeaderNetworkTestnet:
		switch params.Name {
		case PreProdParams.Name, PrivateNetParams.Name:
			match = true
		}
	}
	return
}

func (a *AddressHeader) SetType(headerType AddressHeaderType) {
	b := a.toBytes()
	a.fromBytes([8]byte(append(headerType[:], b[4:]...)))
}

func (a *AddressHeader) SetNetwork(network AddressHeaderNetwork) {
	b := a.toBytes()
	a.fromBytes([8]byte(append(b[:4], network[:]...)))
}
