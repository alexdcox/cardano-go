package cardano

import "github.com/pkg/errors"

func init() {
	MainNetParams.Name = NetworkMainNet
	MainNetParams.Magic = NetworkMagicMainNet
	MainNetParams.StartTime = 1_506_203_091
	MainNetParams.ByronBlock = 4_492_800
	MainNetParams.AddressPrefix = "addr"
	MainNetParams.DelegationPrefix = "stake"

	PreProdParams.Name = NetworkPreProd
	PreProdParams.Magic = NetworkMagicPreProd
	PreProdParams.StartTime = 1_564_020_236
	PreProdParams.ByronBlock = 46
	PreProdParams.AddressPrefix = "addr"
	PreProdParams.DelegationPrefix = "stake"

	PrivateNetParams.Name = NetworkPrivateNet
	PrivateNetParams.Magic = NetworkMagicPrivateNet
	PrivateNetParams.AddressPrefix = "addr_test"
	PrivateNetParams.DelegationPrefix = "stake_test"
}

type NetworkParams struct {
	Name             Network
	Magic            NetworkMagic
	StartTime        uint64
	ByronBlock       uint64
	AddressPrefix    string
	DelegationPrefix string
}

var MainNetParams = NetworkParams{}
var PreProdParams = NetworkParams{}
var PrivateNetParams = NetworkParams{}

const (
	NetworkMainNet    Network = "mainnet"
	NetworkPreProd    Network = "preprod"
	NetworkPrivateNet Network = "privnet"
)

type Network string

func (n Network) Valid() bool {
	return n == NetworkMainNet || n == NetworkPreProd || n == NetworkPrivateNet
}

func (n Network) Validate() (err error) {
	if !n.Valid() {
		err = errors.Errorf("invalid network: '%s'", n)
	}
	return
}

func (n Network) Params() (params *NetworkParams, err error) {
	if err = n.Validate(); err != nil {
		return
	}

	switch n {
	case NetworkMainNet:
		return &MainNetParams, nil
	case NetworkPreProd:
		return &PreProdParams, nil
	case NetworkPrivateNet:
		return &PrivateNetParams, nil
	}

	return
}

type NetworkMagic uint64

const (
	NetworkMagicMainNet       NetworkMagic = 764824073
	NetworkMagicLegacyTestnet NetworkMagic = 1097911063
	NetworkMagicPreProd       NetworkMagic = 1
	NetworkMagicPreview       NetworkMagic = 2
	NetworkMagicSanchonet     NetworkMagic = 4
	NetworkMagicPrivateNet    NetworkMagic = 42
)

const MayaProtocolAuxKey = 6676
