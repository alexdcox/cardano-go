package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"flag"
	"fmt"
	"strings"

	. "github.com/alexdcox/cardano-go"
	"github.com/btcsuite/btcutil/bech32"
	"github.com/fxamacker/cbor/v2"
	"github.com/pkg/errors"
)

var log = Log()

var key string

func main() {
	flag.StringVar(&key, "key", "", "The private/public key as hex/cbor")
	flag.Parse()

	if key == "" {
		fmt.Println("usage: addr_build --key KEY")
	}

	key = strings.Trim(key, " \"")

	fmt.Println("")
	fmt.Println("building addresses from existing key")
	fmt.Println("")

	switch {
	case attemptPrivateCbor():
	case attemptPrivateHex():
	default:
		fmt.Println("invalid key")
	}
}

func attemptPrivateHex() bool {
	keyBytes, err := hex.DecodeString(key)
	if err != nil {
		return false
	}

	if len(keyBytes) == 64 {
		keyBytes = keyBytes[:32]
		return false
	}

	if len(keyBytes) != 32 {
		return false
	}

	keyCbor, err := cbor.Marshal(keyBytes)
	if err != nil {
		log.Fatal().Msgf("%+v", err)
	}

	fmt.Println("key type:          private | hex")
	fmt.Printf("key cbor:          %x\n", keyCbor)
	fmt.Printf("key hex:               %x\n", keyBytes)

	pk := ed25519.NewKeyFromSeed(keyBytes)
	pub := pk.Public().(ed25519.PublicKey)

	encodePubkeyAllNetworks(pub)

	return true
}

func attemptPrivateCbor() bool {
	keyCbor, err := hex.DecodeString(key)
	if err != nil {
		return false
	}

	var keyBytes []byte
	if err2 := cbor.Unmarshal(keyCbor, &keyBytes); err2 != nil {
		return false
	}

	if len(keyBytes) == 64 {
		keyBytes = keyBytes[:32]
		return false
	}

	if len(keyBytes) != 32 {
		return false
	}

	fmt.Println("key type:          private | cbor")
	fmt.Printf("key hex:               %x\n", keyBytes)
	fmt.Printf("key cbor:          %x\n", keyCbor)

	pk := ed25519.NewKeyFromSeed(keyBytes)
	pub := pk.Public().(ed25519.PublicKey)

	encodePubkeyAllNetworks(pub)

	return true
}

func encodePubkeyAllNetworks(pub ed25519.PublicKey) {
	for _, net := range []Network{
		NetworkMainNet,
		NetworkPreProd,
		NetworkPrivateNet,
	} {
		addr, err2 := EncodeAddress(pub, net, AddressTypePayment)
		if err2 != nil {
			continue
		}

		encoded, err2 := addr.Bech32String(net)
		if err2 != nil {
			log.Fatal().Msgf("%+v", err2)
		}

		header, err2 := addr.Header()
		if err2 != nil {
			log.Fatal().Msgf("%+v", err2)
		}

		_, data, err2 := bech32.Decode(encoded)
		if err2 != nil {
			log.Fatal().Msgf("%+v", errors.WithStack(err2))
		}

		converted, err2 := bech32.ConvertBits(data, 5, 8, false)
		if err2 != nil {
			log.Fatal().Msgf("%+v", errors.WithStack(err2))
		}

		fmt.Println("")
		fmt.Printf("network:           %s\n", net)
		fmt.Printf("addr header byte:  %s\n", header)
		fmt.Printf("addr (8-bit):      %x\n", converted)
		fmt.Printf("addr (bech32):     %s\n", encoded)
	}

}
