package main

import (
	"encoding/json"
	"flag"
	"os"
	"os/signal"
	"syscall"

	. "github.com/alexdcox/cardano-go"
	"github.com/alexdcox/cardano-go/cli/rpc/shared"
	"github.com/pkg/errors"
)

type _config struct {
	NodeDataPath string `json:"nodedatapath"`
	NodeHostPort string `json:"nodehostport"`
	Network      string `json:"network"`
	// TODO: This optional parameter is needed for this client to work with custom/3p private networks
	// NetworkMagic      string          `json:"networkmagic"`
	RpcHostPort       string  `json:"rpchostport"`
	ConfigPath        string  `json:"configpath"`
	SocketPath        string  `json:"socketpath"`
	NodeConfigPath    string  `json:"nodeconfigpath"`
	ByronConfigPath   string  `json:"byronconfigpath"`
	ShelleyConfigPath string  `json:"shelleyconfigpath"`
	Cardano           *Config `json:"cardano"`
}

func (c *_config) Load() (err error) {
	flag.StringVar(&c.NodeDataPath, "nodedatapath", "/opt/cardano/data", "Path to the node chunk directory")
	flag.StringVar(&c.NodeHostPort, "nodehostport", "localhost:3000", "Set host:port for the http/rpc listener")
	flag.StringVar(&c.Network, "network", "", "Set network (mainnet|preprod|privnet)")
	flag.StringVar(&c.RpcHostPort, "rpchostport", "localhost:3001", "Set host:port for the http/rpc listener")
	flag.StringVar(&c.SocketPath, "socketpath", "/opt/cardano/ipc/socket", "Path to the node socket")
	flag.StringVar(&c.NodeConfigPath, "nodeconfigpath", "", "Path to the node config json")
	flag.StringVar(&c.ByronConfigPath, "byronconfigpath", "", "Path to the byron config json")
	flag.StringVar(&c.ShelleyConfigPath, "shelleyconfigpath", "", "Path to the shelley config json")
	flag.Parse()

	c.Cardano = &Config{}

	for _, cfg := range []struct {
		path   string
		target any
	}{
		{
			path:   config.NodeConfigPath,
			target: &c.Cardano.NodeConfig,
		},
		{
			path:   config.ByronConfigPath,
			target: &c.Cardano.ByronConfig,
		},
		{
			path:   config.ShelleyConfigPath,
			target: &c.Cardano.ShelleyConfig,
		},
	} {
		if cfg.path == "" {
			continue
		}

		log.Info().Msgf("loading config file: %s", cfg.path)

		var data []byte
		data, err = os.ReadFile(cfg.path)
		if err != nil {
			err = errors.Wrapf(err, "failed to read config file: %s", cfg.path)
			return
		}

		err = json.Unmarshal(data, cfg.target)
		if err != nil {
			err = errors.Wrapf(err, "failed to unmarshal json from config file: %s", cfg.path)
			return
		}
	}

	if j, err2 := json.MarshalIndent(config, "", "  "); err2 == nil {
		log.Info().Msgf("config loaded:\n%s", string(j))
	}

	return
}

var log = Log()

var config *_config

func main() {
	config = &_config{}

	err := config.Load()
	if err != nil {
		log.Fatal().Msgf("%+v", errors.WithStack(err))
	}

	chunkReader, err := NewChunkReader(config.NodeDataPath)
	if err != nil {
		log.Fatal().Msgf("%+v", errors.WithStack(err))
	}

	db, err := NewSqlLiteDatabase("cardano.db")
	if err != nil {
		log.Fatal().Msgf("%+v", errors.WithStack(err))
	}

	client, err := NewClient(&ClientOptions{
		HostPort: config.NodeHostPort,
		Network:  Network(config.Network),
		TipStore: db,
	})
	if err != nil {
		log.Fatal().Msgf("%+v", errors.WithStack(err))
	}

	if err = initialise(chunkReader, db, client); err != nil {
		log.Fatal().Msgf("%+v", errors.WithStack(err))
	}

	httpServer, err := NewHttpRpcServer(config, chunkReader, db, client)
	if err != nil {
		log.Fatal().Msgf("%+v", errors.WithStack(err))
	}

	go func() {
		err = httpServer.Start()
		if err != nil {
			log.Error().Msgf("%+v", errors.WithStack(err))
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	<-c

	log.Info().Msg("caught interrupt/terminate signal, attempting graceful shutdown...")

	err = httpServer.Stop()
	if err != nil {
		log.Fatal().Msgf("%+v", errors.WithStack(err))
	}

	log.Info().Msg("graceful shutdown complete")
}

func initialise(chunkReader *ChunkReader, db Database, client *Client) (err error) {
	shared.ChunkUpdatesToDatabase(chunkReader, db)
	err = client.Start()
	shared.NodeUpdatesToDatabase(client, db)
	return
}
