#![forbid(unsafe_code)]

//! Deterministic, transport-free implementation of JFTrade's
//! `conservative-bar-v1` backtest computation boundary.
//!
//! PineTS remains the strategy runtime. This crate consumes only normalized
//! candles and order intents and owns no HTTP, worker, provider, or database
//! lifecycle.

mod engine;
mod fees;
mod fingerprint;
mod indicators;
mod matching;
mod model;
mod report;
mod validation;

pub use model::{CorpusInput, CorpusOutput};

use jftrade_kernel::CodecError;
use thiserror::Error;

use crate::engine::run_case;
use crate::model::{CORPUS_VERSION, EXECUTION_MODEL};

#[derive(Debug, Error)]
pub enum BacktestError {
    #[error("invalid backtest input: {0}")]
    InvalidInput(String),
    #[error("backtest arithmetic failed: {0}")]
    Arithmetic(String),
    #[error(transparent)]
    Codec(#[from] CodecError),
    #[error(transparent)]
    Json(#[from] serde_json::Error),
}

pub fn run_corpus(input: &CorpusInput) -> Result<CorpusOutput, BacktestError> {
    if input.version != CORPUS_VERSION {
        return Err(BacktestError::InvalidInput(format!(
            "unsupported corpus version {}; expected {CORPUS_VERSION}",
            input.version
        )));
    }
    let mut ids = std::collections::BTreeSet::new();
    for case in &input.cases {
        if !ids.insert(case.id.as_str()) {
            return Err(BacktestError::InvalidInput(format!(
                "duplicate case id {}",
                case.id
            )));
        }
    }
    let cases = input
        .cases
        .iter()
        .map(run_case)
        .collect::<Result<Vec<_>, _>>()?;
    Ok(CorpusOutput {
        version: input.version,
        execution_model: EXECUTION_MODEL,
        cases,
    })
}

pub fn run_json(input: &[u8]) -> Result<Vec<u8>, BacktestError> {
    let corpus: CorpusInput = serde_json::from_slice(input)?;
    Ok(serde_json::to_vec(&run_corpus(&corpus)?)?)
}
