use revm::precompile::{Precompiles, PrecompileSpecId};

pub struct Reth {
    pub precompiles: Precompiles,
}

impl Reth {
    pub fn new() -> Self {
        Self {
            precompiles: Precompiles::new(PrecompileSpecId::LATEST).clone(),
        }
    }
}
