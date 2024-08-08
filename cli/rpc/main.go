package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	. "github.com/alexdcox/cardano-go"
	db2 "github.com/alexdcox/cardano-go/db"
	"github.com/pkg/errors"
)

type _config struct {
	DbPath       string `json:"dbpath"`
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
	flag.StringVar(&c.DbPath, "dbpath", "/opt/cardano/data", "Path to the database directory")
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

	chunkReader, err := NewChunkReader(config.DbPath)
	if err != nil {
		log.Fatal().Msgf("%+v", errors.WithStack(err))
	}

	db, err := db2.NewSqlLiteDatabase("cardano.db")
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

func initialise(chunkReader *ChunkReader, db db2.Database, client *Client) (err error) {
	_, lastSavedChunk, err := db.GetChunkSpan()
	if err != nil {
		return
	}

	chunkReader.WaitForReady()

	err = chunkReader.LoadChunkFiles(lastSavedChunk)
	if err != nil {
		return
	}

	// Write chunk filesystem indexes to database

	result, duration := chunkReader.ProcessBlockRanges()
	fmt.Printf("Processing took %v\n", duration)

	for _, r := range result {
		err = db.SetChunkRange(r.Chunk, r.Min, r.Max)
	}

	// Watch chunk filesystem for updates and update database accordingly

	go func() {
		err = chunkReader.WatchChunkFiles(func(chunk, first, last uint64) {
			err = db.SetChunkRange(chunk, first, last)
			if err != nil {
				log.Error().Msgf("%+v", err)
			}
		})
		if err != nil {
			log.Error().Msgf("%+v", err)
		}
	}()

	err = client.Start()

	// Write node-to-node block updates to database

	client.OnBlock(func(block *Block) {
		err = db.AddBlockPoint(block.Data.Header.Body.Number, Point{
			Slot: block.Data.Header.Body.Slot,
			Hash: block.Data.Header.Body.Hash,
		})
		if err != nil {
			log.Fatal().Msgf("CRITICAL ERROR: unable to write block point index, %+v", err)
		}
		var hashes []string
		for i, tx := range block.Data.TransactionBodies {
			hash, err2 := tx.Hash()
			if err2 != nil {
				log.Fatal().Msgf(
					"CRITICAL ERROR: unable to hash tx %d from block %d: %+v",
					i,
					block.Data.Header.Body.Number,
					err2,
				)
			}
			hashes = append(hashes, hash.String())
		}
		err = db.AddTxsForBlock(hashes, block.Data.Header.Body.Number)
		if err != nil {
			log.Fatal().Msgf("CRITICAL ERROR: unable to update txhash to block index", err)
		}
	})

	return
}
