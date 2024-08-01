package cardano

import (
	"bytes"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/pkg/errors"
)

func TestBlockCBORUnmarshal(t *testing.T) {
	blockHex := "820685828a1a00a02e2b1a079f4fb65820a9125dadcda00c06200c6a67352c27b944c4b4fa9c462b91fb5a1d10496c952c58206f6f7c200377adaa60d4c7897a9bbff4ecf45149e811d2825e6be7cfecd3000d58203fc84f411a4bd323a232fca8c6aba07c80ccdff29cb490e457e9b4e33634b87d8258402f3fa7d7535aeccfc2fba5f2f7bf31c8a1c4e550d7b5b77c2f92d1be16b7f54c20ba8eda22c011b4503c47eea9a1b9545b84aaeeff9fed527340ad29f7d5863e5850f0a04f2a6cd627e6cb179e9fa27e2530b318caeb5474b1b0a52bf25fc96dbe8c887ccbb758b3d8f47a4f452a520d0b65627ecbdc6c2c0010be050ce97fbd2c1d6c5cab3266782e549cb54e5f36e77b0518eb58207ab0cbeaf88c3b543b6544b41f5791c5cfdc379d612be1eda6031b07500e2e63845820bd7422836a0bcbf51fdd8e48ea51c84e99d46133455b73645f0febdad2d00636021903b45840234d07e4db2328c4ac041af9f1853200b093787f726806cd191dd835f7693741cac0b4684176db57373ae7d68e5a27c3dd2931c852578667b4b26d52f4dbf1068208005901c071b6379cb59a78056262ffa21332ae17d4700b5a7768b6b9b2b68e2bf8e511020e59108110f0951d58ebe6e9946b374d0aa549dcd8bc08f4a5c93022083e280505d87afb3c51b5f98e890c3ae6f16f8594239596e2c651e450d69bc51556417e38f4b7707ab5a7f6edae69c59d567c99fd4078b1b69460410456a9c96561d9dbf09aa6db493188237a3091d6b3b8400ac91c12df42e8986d915e26bbfe9ec7bb1954928bcf3a468e4fa0d190f6e5865409660cd84dc7a8fd345ab8bcc14e27d0394a88fde40c54b5a1f07b5a4e9a242f4098c7fcf4662e2de8257725af8071188a1c277a38c5a310b7e2dbcb1e549a3fe16fa0ae0de13f646660e1d770eeb27c676330b54290221f8e4016fe843ea216fcb6cbc88f364d65f71b3e46c5a2736f280c2fda5c8cdc9d289e91b43cb3c53a0462615b724a313c9042284d443268823a9201b4e450e45524c787c54174981c4656edaeccf3b732d8b3549cb5f26a9437120f07ff72e38c6460696dd56e1754cfe966ab51283dae98b18a02160d2fcdfaee346b979c4dd85a1f2dbaf6ad0b6b57f9cd9666acfb7eb60bf37bb210cc2d00a362d3eed29d8b0cee06a3b9a0e81a9b14c69dbde54447186345d4a3982bdd81a40081825820710cb03bdce782b7d8f4e9cd2395dd98036ce9a7f7ed019086816fb6aee806b201018282581d61c0155b37c96884187b00f35eddb8492660ed642b4cb1c7a91193722f1a0013e17582581d618c309901c237ca9bd09f699588c01217efa550816e0d06ceb26291e61a07b87e6d021a0002c7e5031a079f5da781a10081825820848e4e417aad5169b72492ecfefd6446f59d634da51ff948b37611eacf66e591584053ec8410c832f5ae60711151153df677f74746bebe8942b74d7655b14d4a4a0422dcd0f18ad16a2048de4ed06597601b2306ccacf8eeea302caa5671db22d201a080"

	block := &Block{}

	err := cbor.Unmarshal(HexString(blockHex).Bytes(), block)
	if err != nil {
		t.Fatalf("%+v", errors.WithStack(err))
	}

	// Expected values taken from block explorer:
	// https://explorer.cardano.org/en/block?id=1bdf0957fe8c17efaba170eab337c3335d8e985a9ed75ce5e4ccf2ea01b2ace2

	var (
		expectedEra                     = EraConway
		expectedNumber           int64  = 10497579
		expectedSlot             int64  = 127881142
		expectedSize             int64  = 235
		expectedTransactionCount        = 1
		expectedInputCount              = 1
		expectedInput1Txid              = "710cb03bdce782b7d8f4e9cd2395dd98036ce9a7f7ed019086816fb6aee806b2"
		expectedInput1Index      int64  = 1
		expectedOutputCount             = 2
		expectedOutput1Address          = "addr1v8qp2kehe95ggxrmqre4ahdcfynxpmty9dxtr3afzxfhytcfl5f09"
		expectedOutput1Amount    uint64 = 1302901
		expectedOutput2Address          = "addr1vxxrpxgpcgmu4x7sna5etzxqzgt7lf2ss9hq6pkwkf3frestvu8tz"
		expectedOutput2Amount    uint64 = 129531501
	)

	if block.Era != expectedEra {
		t.Fatalf("expected era %s, got %s", expectedEra, block.Era)
	}

	header := block.Data.Header.Body

	if header.Number != expectedNumber {
		t.Fatalf("expected block number %d, got %d", expectedNumber, header.Number)
	}

	if header.Slot != expectedSlot {
		t.Fatalf("expected slot %d, got %d", expectedSlot, header.Slot)
	}

	if header.Size != expectedSize {
		t.Fatalf("expected size %d, got %d", expectedSize, header.Size)
	}

	if len(block.Data.TransactionBodies) != expectedTransactionCount {
		t.Fatalf("expected %d txs, got %d", expectedTransactionCount, len(block.Data.TransactionBodies))
	}

	tx1 := block.Data.TransactionBodies[0]

	if len(tx1.Inputs) != expectedInputCount {
		t.Fatalf("expected tx1 to have %d inputs, got %d", expectedInputCount, len(tx1.Inputs))
	}

	if !bytes.Equal(tx1.Inputs[0].Txid, HexString(expectedInput1Txid).Bytes()) {
		t.Fatalf("expected tx1 txid to equal %s, got %x", expectedInput1Txid, tx1.Inputs[0].Txid)
	}

	if tx1.Inputs[0].Index != expectedInput1Index {
		t.Fatalf("expected tx1 index to equal %d, got %d", expectedInput1Index, tx1.Inputs[0].Index)
	}

	if len(tx1.Outputs) != expectedOutputCount {
		t.Fatalf("expected tx1 to have %d outputs, got %d", expectedOutputCount, len(tx1.Outputs))
	}

	// TODO: This is were converting from all the possible subtypes to a single
	//       unified type covering all possibilities would make things a lot easier.
	//       I'll come back to this, for now I'll just cast into the type I know it should be.

	iOutput1 := tx1.Outputs[0]
	output1, output1Ok := iOutput1.Subtype.(*TransactionOutputG)
	if !output1Ok {
		t.Fatalf("invalid type for tx1 output1, got %T", iOutput1.Subtype)
	}

	if output1.Address.String() != expectedOutput1Address {
		t.Fatalf("expected tx1 output1 address %s, got %s", expectedOutput1Address, output1.Address.String())
	}

	if output1.Amount != expectedOutput1Amount {
		t.Fatalf("expected tx1 output1 address %d, got %d", expectedOutput1Amount, output1.Amount)
	}

	iOutput2 := tx1.Outputs[1]
	output2, output2Ok := iOutput2.Subtype.(*TransactionOutputG)
	if !output2Ok {
		t.Fatalf("invalid type for tx2 output2, got %T", iOutput2.Subtype)
	}

	if output2.Address.String() != expectedOutput2Address {
		t.Fatalf("expected tx2 output2 address %s, got %s", expectedOutput2Address, output2.Address.String())
	}

	if output2.Amount != expectedOutput2Amount {
		t.Fatalf("expected tx2 output2 address %d, got %d", expectedOutput2Amount, output2.Amount)
	}
}

func TestSlot_Time(t *testing.T) {
	var (
		block           uint64 = 10497579
		slot                   = 127881142
		expectedTime, _        = time.Parse("2006/01/02 15:04:05", "2024/06/27 00:17:13")
		network                = NetworkMainNet
	)

	target := Slot(slot)
	output := target.Time(network, block)
	if !output.Equal(expectedTime) {
		t.Fatalf("expected time %s, got %s", expectedTime.String(), output.String())
	}
}
