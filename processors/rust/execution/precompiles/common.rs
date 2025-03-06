use std::collections::HashMap;
use revm::precompile::{Address, u64_to_address};
use std::sync::LazyLock;

pub static PRECOMPILE_TO_ADDR: LazyLock<HashMap<&'static str, Address>> = LazyLock::new(|| {
    let mut map = HashMap::new();
    map.insert("ecrecover", u64_to_address(0x1));
    map.insert("sha256", u64_to_address(0x2));
    map.insert("ripemd160", u64_to_address(0x3));
    map.insert("dataCopy", u64_to_address(0x4));
    map.insert("bigModExp", u64_to_address(0x5));
    map.insert("bn256Add", u64_to_address(0x6));
    map.insert("bn256ScalarMul", u64_to_address(0x7));
    map.insert("bn256Pairing", u64_to_address(0x8));
    map.insert("blake2F", u64_to_address(0x9));
    map.insert("kzgPointEvaluation", u64_to_address(0xa));
    map.insert("bls12381G1Add", u64_to_address(0xb));
    map.insert("bls12381G1MultiExp", u64_to_address(0xc));
    map.insert("bls12381G2Add", u64_to_address(0xd));
    map.insert("bls12381G2MultiExp", u64_to_address(0xe));
    map.insert("bls12381Pairing", u64_to_address(0xf));
    map.insert("bls12381MapG1", u64_to_address(0x10));
    map.insert("bls12381MapG2", u64_to_address(0x11));
    map
});
