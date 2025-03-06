package types

import (
	"math/big"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
)

type Geth struct {
	Precompiles vm.PrecompiledContracts
	Rules       params.Rules
}

func (g *Geth) New() *Geth {
	rules := params.Rules {
		ChainID: big.NewInt(1),
		IsHomestead: true,
		IsEIP150: true,
		IsEIP155: true,
		IsEIP158: true,
		IsByzantium: true,
		IsConstantinople: true,
		IsPetersburg: true,
		IsIstanbul: true,
		IsBerlin: true,
		IsLondon: true,
		IsMerge: true,
		IsShanghai: true,
		IsCancun:      true,
		IsVerkle:      false,
	}

	return &Geth {
		Precompiles: vm.ActivePrecompiledContracts(rules),
		Rules:       rules,
	}
}
