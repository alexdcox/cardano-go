package cardano

import (
	"github.com/alexdcox/cbor/v2"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
)

type MessageSubmitTx struct {
	WithSubprotocol
	BodyType Era
	TxBytes  []byte
}

type MessageAcceptTx struct {
	WithSubprotocol
}

type MessageRejectTx struct {
	WithSubprotocol
	Reason uint64
}

type TxSubmission struct {
	_             struct{}            `cbor:",toarray"`
	Body          TxSubmissionBody    `json:"body"`
	Witness       TxSubmissionWitness `json:"witness"`
	AlonzoEval    bool                `json:"alonzoEval"`
	AuxiliaryData *AuxData            `json:"auxiliaryData"`
}

func (tx *TxSubmission) Hash() (hash HexBytes, err error) {
	bytes, err := cbor.Marshal(tx.Body)
	if err != nil {
		err = errors.WithStack(err)
		return
	}
	_hash := blake2b.Sum256(bytes)
	hash = _hash[:]
	return
}

type TxSubmissionBody struct {
	Inputs            WithCborTag[[]TransactionInput] `cbor:"0,keyasint" json:"inputs"`
	Outputs           []SubtypeOf[TransactionOutput]  `cbor:"1,keyasint" json:"outputs"`
	Fee               uint64                          `cbor:"2,keyasint" json:"fee"`
	AuxiliaryDataHash HexBytes                        `cbor:"7,keyasint,omitempty" json:"auxiliaryDataHash,omitempty"`
}

type TxSubmissionWitness struct {
	Signers []TxSigner `cbor:"0,keyasint" json:"signers"`
}

type TxSigner struct {
	_         struct{} `cbor:",toarray"`
	Key       HexBytes `json:"key"`
	Signature HexBytes `json:"signature"`
}
