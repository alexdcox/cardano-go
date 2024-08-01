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

// TODO: Are these not variables?
const (
	ByronProcessTime  = 20
	ShellyProcessTime = 1
)

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

const (
	RelayMainnet   = "relays-new.cardano-mainnet.iohk.io:3001"
	RelayTestnet   = "relays-new.cardano-testnet.iohkdev.io:3001"
	RelayPreProd   = "preprod-node.world.dev.cardano.org:30000"
	RelayPreview   = "preview-node.world.dev.cardano.org:30002"
	RelaySanchonet = "sanchonet-node.play.dev.cardano.org:3001"
)

var WellKnownMainnetPoint Point = Point{
	Slot: 16588737,
	Hash: HexString("4e9bbbb67e3ae262133d94c3da5bffce7b1127fc436e7433b87668dba34c354a").Bytes(),
}

var WellKnownTestnetPoint Point = Point{
	Slot: 13694363,
	Hash: HexString("b596f9739b647ab5af901c8fc6f75791e262b0aeba81994a1d622543459734f2").Bytes(),
}

var WellKnownPreprodPoint Point = Point{
	Slot: 87480,
	Hash: HexString("528c3e6a00c82dd5331b116103b6e427acf447891ce3ade6c4c7a61d2f0a2b1c").Bytes(),
}

var WellKnownPreviewPoint Point = Point{
	Slot: 8000,
	Hash: HexString("70da683c00985e23903da00656fae96644e1f31dce914aab4ed50e35e4c4842d").Bytes(),
}

var WellKnownSanchonetPoint Point = Point{
	Slot: 20,
	Hash: HexString("6a7d97aae2a65ca790fd14802808b7fce00a3362bd7b21c4ed4ccb4296783b98").Bytes(),
}
