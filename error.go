package cardano

import (
	"fmt"
)

var (
	ErrNotChunkFile          = fmt.Errorf("not a chunk file")
	ErrEraBeforeConway       = fmt.Errorf("era before conway")
	ErrChunkBlockMissing     = fmt.Errorf("chunk doesn't contain expected block")
	ErrDataDirectoryNotFound = fmt.Errorf("data directory not found")
	ErrBlockNotFound         = fmt.Errorf("block not found")
)
