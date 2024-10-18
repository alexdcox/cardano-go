module github.com/alexdcox/cardano-go

go 1.22.1

toolchain go1.23.0

replace github.com/ethereum/go-ethereum => gitlab.com/mayachain/arbitrum/go-ethereum v1.3.0

require (
	github.com/btcsuite/btcutil v1.0.3-0.20201208143702-a53e38424cce
	github.com/cosmos/cosmos-sdk v0.45.9
	github.com/google/uuid v1.6.0 // indirect
	github.com/rs/zerolog v1.27.0
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)

require (
	github.com/cosmos/btcutil v1.0.4 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.6.0
	github.com/klauspost/compress v1.17.0 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/mr-tron/base58 v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/stretchr/testify v1.9.0
	golang.org/x/crypto v0.17.0
	golang.org/x/sys v0.16.0 // indirect
	golang.org/x/text v0.14.0
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

require github.com/mattn/go-colorable v0.1.13 // indirect

require (
	filippo.io/edwards25519 v1.0.0-beta.2
	github.com/fxamacker/cbor/v2 v2.7.0
	github.com/gofiber/fiber/v2 v2.52.5
	github.com/mattn/go-sqlite3 v1.14.5
	github.com/tidwall/gjson v1.6.7
)

require (
	github.com/andybalholm/brotli v1.0.5 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/tidwall/match v1.0.3 // indirect
	github.com/tidwall/pretty v1.0.2 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.51.0 // indirect
	github.com/valyala/tcplisten v1.0.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
)

replace (
	github.com/agl/ed25519 => github.com/binance-chain/edwards25519 v0.0.0-20200305024217-f36fc4b53d43
	github.com/binance-chain/tss-lib => gitlab.com/thorchain/tss/tss-lib v0.1.5
	github.com/gogo/protobuf => github.com/gogo/protobuf v1.3.2
	github.com/zondax/ledger-go => github.com/binance-chain/ledger-go v0.9.1
)
