module github.com/alexdcox/cardano-go

go 1.21.6

replace github.com/fxamacker/cbor/v2 => /Users/adc/go/src/github.com/fxamacker/cbor

require (
	github.com/alexdcox/dashutil v0.0.0-20230613003859-fc4feafd54e5
	github.com/fxamacker/cbor/v2 v2.6.0
	github.com/pkg/errors v0.9.1
	github.com/rs/zerolog v1.33.0
)

require (
	github.com/alexdcox/dashd-go v0.22.0-beta.0.20230612235143-d76c4b53223d // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.1 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	golang.org/x/crypto v0.0.0-20220214200702-86341886e292 // indirect
	golang.org/x/sys v0.12.0 // indirect
)
