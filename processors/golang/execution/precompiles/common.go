package precompiles

import (
	"github.com/ethereum/go-ethereum/common"
)

// Maps precompiles to their corresponding addresses
var PrecompileToAddr = map[string]common.Address{
	"ecrecover": common.BytesToAddress([]byte{0x1}),
	"sha256": common.BytesToAddress([]byte{0x2}),
	"ripemd160": common.BytesToAddress([]byte{0x3}),
	"dataCopy": common.BytesToAddress([]byte{0x4}),
	"bigModExp": common.BytesToAddress([]byte{0x5}),
	"bn256Add": common.BytesToAddress([]byte{0x6}),
	"bn256ScalarMul": common.BytesToAddress([]byte{0x7}),
	"bn256Pairing": common.BytesToAddress([]byte{0x8}),
	"blake2F": common.BytesToAddress([]byte{0x9}),
	"kzgPointEvaluation": common.BytesToAddress([]byte{0xa}),
	"bls12381G1Add": common.BytesToAddress([]byte{0xb}),
	"bls12381G1MultiExp": common.BytesToAddress([]byte{0xc}),
	"bls12381G2Add": common.BytesToAddress([]byte{0xd}),
	"bls12381G2MultiExp": common.BytesToAddress([]byte{0xe}),
	"bls12381Pairing": common.BytesToAddress([]byte{0xf}),
	"bls12381MapG1": common.BytesToAddress([]byte{0x10}),
	"bls12381MapG2": common.BytesToAddress([]byte{0x11}),
}
