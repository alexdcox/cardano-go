package main

import (
	"encoding/json"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	. "github.com/alexdcox/cardano-go"
	"github.com/alexdcox/cardano-go/cli/rpc/shared"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

type _config struct {
	DatabasePath string `json:"databasepath"`
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
	LogLevel          string  `json:"loglevel"`
}

func (c *_config) Load() (err error) {
	flag.StringVar(&c.DatabasePath, "databasepath", "cardano-rpc.db", "Path to the cardano-go sqlite database")
	flag.StringVar(&c.NodeDataPath, "nodedatapath", "/opt/cardano/data", "Path to the node chunk directory")
	flag.StringVar(&c.NodeHostPort, "nodehostport", "localhost:3000", "Set host:port for the http/rpc listener")
	flag.StringVar(&c.Network, "network", "", "Set network (mainnet|preprod|privnet)")
	flag.StringVar(&c.RpcHostPort, "rpchostport", "localhost:3001", "Set host:port for the http/rpc listener")
	flag.StringVar(&c.SocketPath, "socketpath", "/opt/cardano/ipc/socket", "Path to the node socket")
	flag.StringVar(&c.NodeConfigPath, "nodeconfigpath", "", "Path to the node config json")
	flag.StringVar(&c.ByronConfigPath, "byronconfigpath", "", "Path to the byron config json")
	flag.StringVar(&c.ShelleyConfigPath, "shelleyconfigpath", "", "Path to the shelley config json")
	flag.StringVar(&c.LogLevel, "loglevel", "", "Set the log level (trace|debug|info|warn|error|fatal)")
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

	if err := config.Load(); err != nil {
		log.Fatal().Msgf("%+v", err)
	}

	if config.LogLevel == "" {
		config.LogLevel = "info"
	}
	logLevel, err := zerolog.ParseLevel(config.LogLevel)
	if err != nil {
		log.Fatal().Msgf("%+v", errors.WithStack(err))
	}

	log.Info().Msgf("setting log level to: '%s'", logLevel)
	zerolog.SetGlobalLevel(logLevel)

	db, err := NewSqlLiteDatabase(config.DatabasePath)
	if err != nil {
		log.Fatal().Msgf("%+v", err)
	}

	chunkReader, err := NewChunkReader(config.NodeDataPath, db)
	if err != nil {
		log.Fatal().Msgf("%+v", err)
	}

	if err = chunkReader.Start(); err != nil {
		log.Fatal().Msgf("%+v", err)
	}

	startChunkedBlock, endChunkedBlock, err := db.GetChunkedBlockSpan()
	if err != nil {
		log.Fatal().Msgf("%+v", err)
	}

	startBlockPoint, endBlockPoint, err := db.GetPointSpan()
	if err != nil {
		log.Fatal().Msgf("%+v", err)
	}

	log.Info().Msgf("chunk index: %d to %d", startChunkedBlock, endChunkedBlock)

	if startBlockPoint > 0 {
		log.Info().Msgf("point index: %d to %d", startBlockPoint, endBlockPoint)
	} else {
		log.Info().Msg("point index: not established")
	}

	var clientStartPoint PointAndBlockNum

	if startBlockPoint > 0 && startBlockPoint <= endChunkedBlock+1 {
		log.Info().Msg("chunks overlap client recorded points, continuing from client tip")

		clientStartPoint, err = db.GetBlockPoint(endBlockPoint)
		if err != nil {
			log.Fatal().Msgf("%+v", err)
		}

	} else if endChunkedBlock > 0 {
		log.Info().Msg("detected gap between chunks and client tip, or client tip hasn't been established, continuing from chunked tip")

		block, err2 := chunkReader.GetBlock(endChunkedBlock)
		if err2 != nil {
			log.Fatal().Msgf("%+v", err2)
		}

		clientStartPoint, err2 = block.PointAndNumber()
		if err2 != nil {
			log.Fatal().Msgf("%+v", err2)
		}
	} else {
		log.Info().Msg("no finalised chunks, assuming connected node is syncing, continuing from node tip")
	}

	followClient, err := NewClient(&ClientOptions{
		HostPort:   config.NodeHostPort,
		Network:    Network(config.Network),
		Database:   db,
		StartPoint: clientStartPoint,
		LogLevel:   zerolog.InfoLevel, // TODO: log level
	})
	if err != nil {
		log.Fatal().Msgf("%+v", err)
	}

	for {
		if pingErr := followClient.Ping(); pingErr == nil {
			break
		}
		log.Info().Msg("unable to connect to node, waiting...")
		time.Sleep(time.Second)
	}

	shared.NodeUpdatesToDatabase(followClient, db)

	if err = followClient.Start(); err != nil {
		log.Fatal().Msgf("%+v", err)
	}

	httpServer, err := NewHttpRpcServer(config, chunkReader, db, followClient)
	if err != nil {
		log.Fatal().Msgf("%+v", err)
	}

	go func() {
		if err = httpServer.Start(); err != nil {
			log.Fatal().Msgf("%+v", err)
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	<-c

	log.Info().Msg("caught interrupt/terminate signal, attempting graceful shutdown...")

	if err = httpServer.Stop(); err != nil {
		log.Fatal().Msgf("%+v", err)
	}

	log.Info().Msg("graceful shutdown complete")
}
