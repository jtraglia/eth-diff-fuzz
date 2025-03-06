package precompiles

import (
	"fmt"

	"github.com/jtraglia/eth-diff-fuzz/processors/golang/types"
)

type GethPrecompile types.Geth

func (g *GethPrecompile) HandlePrecompileCall(method string, input []byte) ([]byte, error) {
	precompile := g.Precompiles[PrecompileToAddr[method]]
	if precompile == nil {
		return nil, fmt.Errorf("precompile not found")
	}

	return precompile.Run(input)
}