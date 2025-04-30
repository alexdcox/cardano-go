package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	. "github.com/alexdcox/cardano-go"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

type _config struct {
	DatabasePath string `json:"databasepath"`
	NtNHostPort  string `json:"ntnhostport"`
	NtCHostPort  string `json:"ntchostport"`
	Network      string `json:"network"`
	RpcHostPort  string `json:"rpchostport"`
	SocketPath   string `json:"socketpath"`
	LogLevel     string `json:"loglevel"`
	ReorgWindow  int    `json:"reorgwindow"`
}

func (c *_config) Load() (err error) {
	flag.StringVar(&c.DatabasePath, "databasepath", "cardano-rpc.db", "Path to the cardano-go sqlite database")
	flag.StringVar(&c.NtNHostPort, "ntnhostport", "localhost:3000", "Set host:port for the node-to-node connection")
	flag.StringVar(&c.NtCHostPort, "ntchostport", "localhost:3001", "Set host:port for the node-to-client connection")
	flag.StringVar(&c.Network, "network", "", "Set network (mainnet|preprod|privnet)")
	flag.StringVar(&c.RpcHostPort, "rpchostport", "localhost:3002", "Set host:port for the http/rpc listener")
	flag.StringVar(&c.SocketPath, "socketpath", "/opt/cardano/ipc/socket", "Path to the node socket")
	flag.StringVar(&c.LogLevel, "loglevel", "", "Set the log level (trace|debug|info|warn|error|fatal) Can also be set via the CARDANO_RPC_LOG_LEVEL environment variable")
	flag.IntVar(&c.ReorgWindow, "reorgwindow", -1, "Set the blocks to confirm / reorg window (default: 6)")
	flag.Parse()

	// c.Cardano = &Config{}

	// if j, err2 := json.MarshalIndent(config, "", "  "); err2 == nil {
	// 	log.Info().Msgf("config loaded:\n%s", string(j))
	// }

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
		envLogLevel := os.Getenv("CARDANO_RPC_LOG_LEVEL")
		if envLogLevel != "" {
			config.LogLevel = envLogLevel
		} else {
			config.LogLevel = "info"
		}
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

	client, err := NewClient(&ClientOptions{
		NtNHostPort: config.NtNHostPort,
		NtCHostPort: config.NtCHostPort,
		Network:     Network(config.Network),
		LogLevel:    logLevel,
		Database:    db,
		ReorgWindow: config.ReorgWindow,
	})

	if config.NtNHostPort == "" {
		log.Fatal().Msg("node-to-node rpc host/port not configured")
	}

	if config.NtCHostPort == "" {
		log.Fatal().Msg("node-to-client rpc host/port not configured")
	}

	httpServer, err := NewHttpRpcServer(config, db, client)
	if err != nil {
		log.Fatal().Msgf("%+v", err)
	}

	go func() {
		if err = client.Start(); err != nil {
			log.Fatal().Msgf("%+v", err)
		}
	}()

	go func() {
		if err = httpServer.Start(); err != nil {
			log.Fatal().Msgf("%+v", err)
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	<-c

	log.Info().Msg("caught interrupt/terminate signal, attempting graceful shutdown...")

	if err = client.Stop(); err != nil {
		log.Fatal().Msgf("%+v", err)
	}

	if err = httpServer.Stop(); err != nil {
		log.Fatal().Msgf("%+v", err)
	}

	log.Info().Msg("graceful shutdown complete")
}
