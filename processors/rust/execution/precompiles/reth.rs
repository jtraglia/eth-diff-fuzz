use crate::execution::types::reth::Reth;
use alloy_primitives::Bytes;

use super::common::PRECOMPILE_TO_ADDR;

impl Reth {
    pub fn handle_precompile_call(&self, method: &str, input: &[u8]) -> Result<Vec<u8>, String> {
        let addr = PRECOMPILE_TO_ADDR.get(method);
        if addr.is_none() {
            return Err(format!("Invalid precompile method: {}", method));
        }

        let precompile = self.precompiles.get(addr.unwrap());
        if precompile.is_none() {
            return Err(format!("Precompile not found: {}", method));
        }
        let precompile = precompile.unwrap();

        let input_bytes = Bytes::copy_from_slice(input);
        let output = precompile(&input_bytes, 0); // [@todo nethoxa] gas_limit in geth maybish ??

        match output {
            Ok(output) => Ok(output.bytes.to_vec()),
            Err(e) => Err(format!("Precompile error: {:?}", e)),
        }
    }
}
