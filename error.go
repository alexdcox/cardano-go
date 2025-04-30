package cardano

import (
	"github.com/pkg/errors"
)

var (
	ErrNotChunkFile          = errors.New("not a chunk file")
	ErrVersionRejected       = errors.New("ntn version rejected")
	ErrEraBeforeConway       = errors.New("era before conway")
	ErrChunkBlockMissing     = errors.New("chunk doesn't contain expected block")
	ErrDataDirectoryNotFound = errors.New("data directory not found")
	ErrPointNotFound         = errors.New("point not found")
	ErrBlockNotFound         = errors.New("block not found")
	ErrTransactionNotFound   = errors.New("transaction not found")
	ErrNotEnoughFunds        = errors.New("not enough funds")
	ErrInvalidPrivateKey     = errors.New("invalid private key")
	ErrInvalidPublicKey      = errors.New("invalid public key")
	ErrInvalidPublicKeyType  = errors.New("invalid public key type")
	ErrInvalidCliResponse    = errors.New("invalid cardano-cli response")
	ErrRpcFailed             = errors.New("rpc failed")
	ErrNodeCommandFailed     = errors.New("node command failed")
	ErrNetworkInvalid        = errors.New("network invalid")
	ErrNodeUnavailable       = errors.New("node unavailable")
)

var AllErrors = []error{
	ErrNotChunkFile,
	ErrVersionRejected,
	ErrEraBeforeConway,
	ErrChunkBlockMissing,
	ErrDataDirectoryNotFound,
	ErrPointNotFound,
	ErrBlockNotFound,
	ErrTransactionNotFound,
	ErrNotEnoughFunds,
	ErrInvalidPublicKeyType,
	ErrInvalidCliResponse,
	ErrRpcFailed,
	ErrNodeCommandFailed,
}
