package utils

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

	"github.com/jtraglia/eth-diff-fuzz/processors/golang/types"
)

func GetPrecompile(addr int, client interface{}) vm.PrecompiledContract {
	switch client := client.(type) {
	case types.Geth:
		return client.Precompiles[common.BytesToAddress([]byte{byte(addr)})]
	case types.Erigon:
		return client.Precompiles[common.BytesToAddress([]byte{byte(addr)})]
	}

	return nil
}