package main

import (
	"flag"
	"fmt"
	"strings"

	. "github.com/alexdcox/cardano-go"
	"github.com/btcsuite/btcutil/bech32"
	"github.com/pkg/errors"
)

var log = Log()

var address string

func main() {
	flag.StringVar(&address, "address", "", "The address to decode")
	flag.Parse()

	if address == "" {
		fmt.Println("usage: addr_decode --address BECH32")
	}

	address = strings.Trim(address, " \"")

	fmt.Printf("\ndecoding address:  %s\n\n", address)

	for _, net := range []Network{
		NetworkMainNet,
		NetworkPreProd,
		NetworkPrivateNet,
	} {
		decoded, err := DecodeAddress(address, net)

		fmt.Printf("network:           %s\n", net)

		if err != nil {
			fmt.Println("failed / invalid\n")
		} else {
			reencoded, err2 := decoded.Bech32String(net)
			if err2 != nil {
				log.Fatal().Msgf("%+v", err2)
			}

			header, err2 := decoded.Header()
			if err2 != nil {
				log.Fatal().Msgf("%+v", err2)
			}

			_, data, err2 := bech32.Decode(reencoded)
			if err2 != nil {
				log.Fatal().Msgf("%+v", errors.WithStack(err2))
			}

			converted, err2 := bech32.ConvertBits(data, 5, 8, false)
			if err2 != nil {
				log.Fatal().Msgf("%+v", errors.WithStack(err2))
			}

			fmt.Printf("addr header byte:  %s\n", header)
			fmt.Printf("addr (8-bit):      %x\n", converted)
			fmt.Printf("addr (bech32):     %s\n", reencoded)
		}
	}
}
