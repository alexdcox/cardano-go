package cardano

import (
	"time"
)

type Config struct {
	NodeConfig    NodeConfig    `json:"nodeConfig"`
	ByronConfig   ByronConfig   `json:"byronConfig"`
	ShelleyConfig ShelleyConfig `json:"shelleyConfig"`
}

type NodeConfig struct {
	MinNodeVersion string
	Protocol       string
}

type ByronConfig struct {
	BlockVersionData struct {
		MaxTxSize    int `json:"maxTxSize,string"`
		SlotDuration int `json:"slotDuration,string"`
	} `json:"blockVersionData"`
	ProtocolConsts struct {
		K             int `json:"k"`
		ProtocolMagic int `json:"protocolMagic"`
	} `json:"protocolConsts"`
}

type ShelleyConfig struct {
	ProtocolParams struct {
		MinFeeA int `json:"minFeeA"`
		MinFeeB int `json:"minFeeB"`
	} `json:"protocolParams"`
	NetworkId    string    `json:"networkId"`
	NetworkMagic int       `json:"networkMagic"`
	EpochLength  int       `json:"epochLength"`
	SystemStart  time.Time `json:"systemStart"`
	SlotLength   float64   `json:"slotLength"`
}
