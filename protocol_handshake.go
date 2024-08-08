package cardano

import (
	"fmt"

	"github.com/fxamacker/cbor/v2"
	"github.com/pkg/errors"
)

const (
	N2NProtocolV4 = iota + 4
	N2NProtocolV5
	N2NProtocolV6
	N2NProtocolV7
	N2NProtocolV8
	N2NProtocolV9
	N2NProtocolV10
	N2NProtocolV11
	N2NProtocolV12
	N2NProtocolV13
)

type MessageProposeVersions struct {
	WithSubprotocol
	VersionMap VersionMap `json:"versionMap"`
}

type MessageAcceptVersion struct {
	WithSubprotocol
	Version     uint16        `json:"version,omitempty"`
	VersionData VersionField2 `json:"versionData,omitempty"`
}

type MessageRefuse struct {
	WithSubprotocol
	Details struct {
		_       struct{} `cbor:",toarray"`
		Code    uint16   `json:"something"`
		Version uint16   `json:"version"`
		Reason  string   `json:"reason"`
	} `json:"details"`
}

type MessageQueryReply struct {
	WithSubprotocol
}

type VersionMap struct {
	V4  VersionField1 `cbor:"4,keyasint"`
	V5  VersionField1 `cbor:"5,keyasint"`
	V6  VersionField1 `cbor:"6,keyasint"`
	V7  VersionField1 `cbor:"7,keyasint"`
	V8  VersionField1 `cbor:"8,keyasint"`
	V9  VersionField1 `cbor:"9,keyasint"`
	V10 VersionField1 `cbor:"10,keyasint"`
	V11 VersionField2 `cbor:"11,keyasint"`
	V12 VersionField2 `cbor:"12,keyasint"`
	V13 VersionField2 `cbor:"13,keyasint"`
}

type VersionField1 struct {
	_                          struct{}     `cbor:",toarray"`
	Network                    NetworkMagic `json:"network"`
	InitiatorOnlyDiffusionMode bool         `json:"initiatorOnlyDiffusionMode"`
}

type VersionField2 struct {
	_                          struct{}     `cbor:",toarray"`
	Network                    NetworkMagic `json:"network,omitempty"`
	InitiatorOnlyDiffusionMode bool         `json:"initiatorOnlyDiffusionMode,omitempty"`
	PeerSharing                int          `json:"peerSharing,omitempty"`
	Query                      bool         `json:"query,omitempty"`
}

func defaultVersionMap(network NetworkMagic) VersionMap {
	return VersionMap{
		V4: VersionField1{
			Network:                    network,
			InitiatorOnlyDiffusionMode: true,
		},
		V5: VersionField1{
			Network:                    network,
			InitiatorOnlyDiffusionMode: true,
		},
		V6: VersionField1{
			Network:                    network,
			InitiatorOnlyDiffusionMode: true,
		},
		V7: VersionField1{
			Network:                    network,
			InitiatorOnlyDiffusionMode: true,
		},
		V8: VersionField1{
			Network:                    network,
			InitiatorOnlyDiffusionMode: true,
		},
		V9: VersionField1{
			Network:                    network,
			InitiatorOnlyDiffusionMode: true,
		},
		V10: VersionField1{
			Network:                    network,
			InitiatorOnlyDiffusionMode: true,
		},
		V11: VersionField2{
			Network:                    network,
			InitiatorOnlyDiffusionMode: true,
			PeerSharing:                0,
			Query:                      false,
		},
		V12: VersionField2{
			Network:                    network,
			InitiatorOnlyDiffusionMode: true,
			PeerSharing:                0,
			Query:                      false,
		},
		V13: VersionField2{
			Network:                    network,
			InitiatorOnlyDiffusionMode: true,
			PeerSharing:                0,
			Query:                      false,
		},
	}
}

func encodeVersionMap() {
	h := MessageProposeVersions{
		WithSubprotocol: WithSubprotocol{
			Subprotocol: 0,
		},
		VersionMap: defaultVersionMap(764824073),
	}

	b, err := cbor.Marshal(h)
	if err != nil {
		globalLog.Fatal().Msgf("%+v", errors.WithStack(err))
	}

	fmt.Printf("%x\n", b)

}
