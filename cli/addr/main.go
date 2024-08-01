package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"

	"github.com/alexdcox/cardano-go"
	"github.com/btcsuite/btcutil/bech32"
	"github.com/pkg/errors"
)

var log = cardano.Log()

func main() {
	seed := make([]byte, ed25519.SeedSize)
	_, err := rand.Read(seed)
	if err != nil {
		log.Fatal().Msgf("failed to generate random seed: %+v", errors.WithStack(err))
	}

	pk := ed25519.NewKeyFromSeed(seed)
	pub := pk.Public().(ed25519.PublicKey)

	fmt.Println("")
	fmt.Println("Generated new cardano address:")
	fmt.Println("")
	fmt.Printf("key type:       ed25519\n")
	fmt.Printf("private:        %x\n", pk[:32])
	fmt.Printf("public:         %x\n", pk.Public())

	for _, net := range []cardano.Network{
		cardano.NetworkMainNet,
		cardano.NetworkPreProd,
		cardano.NetworkPrivateNet,
	} {
		addr, err2 := cardano.EncodeAddress(pub, net, cardano.AddressTypePayment)
		if err2 != nil {
			log.Fatal().Msgf("%+v", err2)
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
