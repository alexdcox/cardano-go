module github.com/alexdcox/cardano-go

go 1.22.11

toolchain go1.23.5

replace github.com/ethereum/go-ethereum => gitlab.com/mayachain/arbitrum/go-ethereum v1.3.0

require (
	github.com/btcsuite/btcutil v1.0.3-0.20201208143702-a53e38424cce
	github.com/cosmos/cosmos-sdk v0.45.9
	github.com/google/uuid v1.6.0 // indirect
	github.com/rs/zerolog v1.27.0
)

require (
	github.com/cosmos/btcutil v1.0.4 // indirect
	github.com/klauspost/compress v1.17.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/mr-tron/base58 v1.2.0
	github.com/pkg/errors v0.9.1
	golang.org/x/crypto v0.32.0
	golang.org/x/sys v0.29.0 // indirect
)

require github.com/mattn/go-colorable v0.1.13 // indirect

require (
	filippo.io/edwards25519 v1.1.0
	github.com/Salvionied/apollo v1.1.0
	github.com/alexdcox/cbor/v2 v2.0.0
	github.com/blinklabs-io/gouroboros v0.108.2
	github.com/gofiber/fiber/v2 v2.52.5
	github.com/mattn/go-sqlite3 v1.14.5
	github.com/tidwall/gjson v1.6.7
)

require (
	github.com/Salvionied/cbor/v2 v2.6.0 // indirect
	github.com/andybalholm/brotli v1.0.5 // indirect
	github.com/btcsuite/btcd/btcutil v1.1.6 // indirect
	github.com/fxamacker/cbor/v2 v2.7.0 // indirect
	github.com/jinzhu/copier v0.4.0 // indirect
	github.com/maestro-org/go-sdk v1.2.0 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/tidwall/match v1.0.3 // indirect
	github.com/tidwall/pretty v1.0.2 // indirect
	github.com/tyler-smith/go-bip39 v1.1.0 // indirect
	github.com/utxorpc/go-codegen v0.16.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.51.0 // indirect
	github.com/valyala/tcplisten v1.0.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	golang.org/x/exp v0.0.0-20230522175609-2e198f4a06a1 // indirect
	golang.org/x/text v0.21.0 // indirect
	google.golang.org/protobuf v1.36.3 // indirect
)

replace (
	github.com/agl/ed25519 => github.com/binance-chain/edwards25519 v0.0.0-20200305024217-f36fc4b53d43
	github.com/binance-chain/tss-lib => gitlab.com/thorchain/tss/tss-lib v0.1.5
	github.com/gogo/protobuf => github.com/gogo/protobuf v1.3.2
	github.com/zondax/ledger-go => github.com/binance-chain/ledger-go v0.9.1
)
