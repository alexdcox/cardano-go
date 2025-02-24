package shared

import (
	. "github.com/alexdcox/cardano-go"
)

var log = Log()

// func NodeUpdatesToDatabase(client *Client, db Database) {
// 	log.Warn().Msg("@NodeUpdatesToDatabase")
// 	client.Subscribe(func(i any) {
// 		block, ok := i.(*Block)
// 		if !ok {
// 			return
// 		}
//
// 		point, err := block.PointAndNumber()
// 		if err != nil {
// 			log.Error().Msgf("unable to determine block point: %+v", err)
// 			return
// 		}
//
// 		if err = db.AddBlockPoint(point); err != nil {
// 			log.Error().Msgf("unable to write block point index: %+v", err)
// 			return
// 		}
//
// 		var hashes []string
//
// 		txCount := len(block.Data.TransactionBodies)
// 		if txCount > 0 {
// 			log.Warn().Msgf("block %d txs %d", block.Data.Header.Body.Number, txCount)
// 		}
//
// 		for i, tx := range block.Data.TransactionBodies {
//
// 			if encoded, err2 := cbor.Marshal(tx); err2 == nil {
// 				log.Warn().Msgf("tx encoded %x", encoded)
// 			} else {
// 				log.Error().Msg("failed to re-encode transaction body")
// 			}
//
// 			attempt2 := TransactionBody{
// 				Inputs:            tx.Inputs,
// 				Outputs:           tx.Outputs,
// 				Fee:               tx.Fee,
// 				AuxiliaryDataHash: tx.AuxiliaryDataHash,
// 			}
// 			if encoded, err2 := cbor.Marshal(attempt2); err2 == nil {
// 				log.Warn().Msgf("tx ATTEMPT2 encoded %x", encoded)
// 			} else {
// 				log.Error().Msg("failed to re-encode ATTEMPT2 transaction body")
// 			}
//
// 			hash, err2 := tx.Hash()
// 			if err2 != nil {
// 				log.Fatal().Msgf(
// 					"CRITICAL ERROR: unable to hash tx %d from block %d: %+v",
// 					i,
// 					point.Block,
// 					err2,
// 				)
// 			}
// 			hashes = append(hashes, hash.String())
// 			log.Warn().Msgf("adding hash %s", hash.String())
// 		}
// 		err = db.AddTxsForBlock(hashes, point.Block)
// 		if err != nil {
// 			log.Fatal().Msgf("CRITICAL ERROR: unable to update txhash to block index", err)
// 		}
// 	})
// }
