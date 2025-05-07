package cardano

import (
	"net/http"

	"github.com/pkg/errors"
)

type RpcError struct {
	Msg     string
	Details string
	Code    int
}

func (r RpcError) Error() string {
	return r.Msg
}

func (r RpcError) Is(target error) bool {
	t := &RpcError{}
	if target != nil && errors.As(target, t) {
		return t.Msg == r.Msg
	}
	return false
}

var (
	ErrBlockProcessorStarting = RpcError{Msg: "block processor starting", Code: http.StatusNotFound}
	ErrPointNotFound          = RpcError{Msg: "point not found", Code: http.StatusNotFound}
	ErrBlockNotFound          = RpcError{Msg: "block not found", Code: http.StatusNotFound}
	ErrTransactionNotFound    = RpcError{Msg: "transaction not found", Code: http.StatusNotFound}
	ErrNotEnoughFunds         = RpcError{Msg: "not enough funds", Code: http.StatusBadRequest}
	ErrInvalidPrivateKey      = RpcError{Msg: "invalid private key", Code: http.StatusBadRequest}
	ErrInvalidPublicKey       = RpcError{Msg: "invalid public key", Code: http.StatusBadRequest}
	ErrInvalidPublicKeyType   = RpcError{Msg: "invalid public key type", Code: http.StatusBadRequest}
	ErrInvalidCliResponse     = RpcError{Msg: "invalid cardano-cli response", Code: http.StatusBadRequest}
	ErrRpcFailed              = RpcError{Msg: "rpc failed", Code: 0}
	ErrNodeCommandFailed      = RpcError{Msg: "node command failed", Code: http.StatusBadRequest}
	ErrNetworkInvalid         = RpcError{Msg: "network invalid", Code: http.StatusBadRequest}
	ErrNodeUnavailable        = RpcError{Msg: "node unavailable", Code: http.StatusInternalServerError}
)
