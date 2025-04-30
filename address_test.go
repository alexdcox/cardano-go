package cardano

import (
	"testing"
)

func TestAddressHeader_ByteEncoding(t *testing.T) {
	nets := []AddressHeaderNetwork{
		AddressHeaderNetworkMainnet,
		AddressHeaderNetworkTestnet,
	}

	typs := []AddressHeaderType{
		AddressHeaderTypeStakePaymentKeyHash,
		AddressHeaderTypeStakeScriptHash,
		AddressHeaderTypeScriptPaymentKeyHash,
		AddressHeaderTypeScriptScriptHash,
		AddressHeaderTypePointerPaymentKeyHash,
		AddressHeaderTypePointerScriptHash,
		AddressHeaderTypePaymentKeyHash,
		AddressHeaderTypeScriptHash,
		AddressHeaderTypeStakeRewardHash,
		AddressHeaderTypeScriptRewardHash,
	}

	var outputs []byte

	for _, net := range nets {
		for _, typ := range typs {
			hdr := new(AddressHeader)
			hdr.SetType(typ)
			hdr.SetNetwork(net)

			if err := hdr.Validate(); err != nil {
				t.Fatalf("header invalid for net %s, type %s: %+v", net, typ, err)
			}

			for _, o := range outputs {
				if byte(*hdr) == o {
					t.Fatalf("expecting unique byte output for all variations, %08b was duplicated", o)
				}
			}
		}
	}
}

func TestAddress_EncodeDecode(t *testing.T) {
	testCases := []struct {
		publicKey     string
		network       Network
		typ           AddressType
		networkHeader AddressHeaderNetwork
		bech32Addr    string
	}{
		{
			publicKey:     "ce13cd433cdcb3dfb00c04e216956aeb622dcd7f282b03304d9fc9de804723b2",
			network:       NetworkPrivateNet,
			typ:           AddressTypePayment,
			networkHeader: AddressHeaderNetworkTestnet,
			bech32Addr:    "addr_test1vztc80na8320zymhjekl40yjsnxkcvhu58x59mc2fuwvgkc332vxv",
		},
	}

	for _, testCase := range testCases {
		publicBytes := HexString(testCase.publicKey).Bytes()

		addr, err := EncodeAddress(publicBytes, testCase.network, testCase.typ)
		if err != nil {
			t.Fatalf("%+v", err)
		}

		encoded, err := addr.Bech32String(testCase.network)
		if err != nil {
			t.Fatalf("%+v", err)
		}

		if addr.String() != encoded {
			t.Fatal("expected the default address encoding to be Bech32")
		}

		if encoded != testCase.bech32Addr {
			t.Fatalf("invalid encoding, expected %s, got %s", testCase.bech32Addr, encoded)
		}

		// decode and validate

		decoded, err := DecodeAddress(encoded, testCase.network)
		if err != nil {
			t.Fatalf("%+v", err)
		}

		typ, err := decoded.Type()
		if err != nil {
			t.Fatalf("%+v", err)
		}

		if typ != testCase.typ {
			t.Fatalf("expected address type %s, got %s", testCase.typ, typ)
		}

		net, err := decoded.Network()
		if err != nil {
			t.Fatalf("%+v", err)
		}

		if net != testCase.networkHeader {
			t.Fatalf("expected header network %s, got %s", testCase.networkHeader, net)
		}

		if decoded.String() != testCase.bech32Addr {
			t.Fatalf("invalid decoding, expected %s, got %s", testCase.bech32Addr, decoded)
		}
	}
}

func TestAddress_ParseBech32String(t *testing.T) {
	testCases := []struct {
		in  string
		net Network
	}{
		{
			in:  "addr1qx2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzer3n0d3vllmyqwsx5wktcd8cc3sq835lu7drv2xwl2wywfgse35a3x",
			net: NetworkMainNet,
		}, {
			in:  "addr1z8phkx6acpnf78fuvxn0mkew3l0fd058hzquvz7w36x4gten0d3vllmyqwsx5wktcd8cc3sq835lu7drv2xwl2wywfgs9yc0hh",
			net: NetworkMainNet,
		}, {
			in:  "addr1yx2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzerkr0vd4msrxnuwnccdxlhdjar77j6lg0wypcc9uar5d2shs2z78ve",
			net: NetworkMainNet,
		}, {
			in:  "addr1x8phkx6acpnf78fuvxn0mkew3l0fd058hzquvz7w36x4gt7r0vd4msrxnuwnccdxlhdjar77j6lg0wypcc9uar5d2shskhj42g",
			net: NetworkMainNet,
		}, {
			in:  "addr1gx2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzer5pnz75xxcrzqf96k",
			net: NetworkMainNet,
		}, {
			in:  "addr128phkx6acpnf78fuvxn0mkew3l0fd058hzquvz7w36x4gtupnz75xxcrtw79hu",
			net: NetworkMainNet,
		}, {
			in:  "addr1vx2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzers66hrl8",
			net: NetworkMainNet,
		}, {
			in:  "addr1w8phkx6acpnf78fuvxn0mkew3l0fd058hzquvz7w36x4gtcyjy7wx",
			net: NetworkMainNet,
		}, {
			in:  "stake1uyehkck0lajq8gr28t9uxnuvgcqrc6070x3k9r8048z8y5gh6ffgw",
			net: NetworkMainNet,
		}, {
			in:  "stake178phkx6acpnf78fuvxn0mkew3l0fd058hzquvz7w36x4gtcccycj5",
			net: NetworkMainNet,
		}, {
			in:  "addr_test1qz2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzer3n0d3vllmyqwsx5wktcd8cc3sq835lu7drv2xwl2wywfgs68faae",
			net: NetworkPrivateNet,
		}, {
			in:  "addr_test1zrphkx6acpnf78fuvxn0mkew3l0fd058hzquvz7w36x4gten0d3vllmyqwsx5wktcd8cc3sq835lu7drv2xwl2wywfgsxj90mg",
			net: NetworkPrivateNet,
		}, {
			in:  "addr_test1yz2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzerkr0vd4msrxnuwnccdxlhdjar77j6lg0wypcc9uar5d2shsf5r8qx",
			net: NetworkPrivateNet,
		}, {
			in:  "addr_test1xrphkx6acpnf78fuvxn0mkew3l0fd058hzquvz7w36x4gt7r0vd4msrxnuwnccdxlhdjar77j6lg0wypcc9uar5d2shs4p04xh",
			net: NetworkPrivateNet,
		}, {
			in:  "addr_test1gz2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzer5pnz75xxcrdw5vky",
			net: NetworkPrivateNet,
		}, {
			in:  "addr_test12rphkx6acpnf78fuvxn0mkew3l0fd058hzquvz7w36x4gtupnz75xxcryqrvmw",
			net: NetworkPrivateNet,
		}, {
			in:  "addr_test1vz2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzerspjrlsz",
			net: NetworkPrivateNet,
		}, {
			in:  "addr_test1wrphkx6acpnf78fuvxn0mkew3l0fd058hzquvz7w36x4gtcl6szpr",
			net: NetworkPrivateNet,
		}, {
			in:  "stake_test1uqehkck0lajq8gr28t9uxnuvgcqrc6070x3k9r8048z8y5gssrtvn",
			net: NetworkPrivateNet,
		}, {
			in:  "stake_test17rphkx6acpnf78fuvxn0mkew3l0fd058hzquvz7w36x4gtcljw6kf",
			net: NetworkPrivateNet,
		},
	}

	for i, testCase := range testCases {
		addr := &Address{}
		err := addr.ParseBech32String(testCase.in, testCase.net)
		if err != nil {
			t.Fatalf("test case %s (%d) failed: %+v", testCase.in, i, err)
			return
		}
	}
}
