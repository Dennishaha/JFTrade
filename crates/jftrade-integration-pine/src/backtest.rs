//! Strategy/Pine backtest execution adapter.
//!
//! The product engine owns request validation, persistence, cancellation and
//! task lifecycle.  This module owns the optional worker-side execution
//! boundary and the deterministic Rust matcher adapter.  Keeping the port and
//! its implementation here prevents the composition root from depending
//! directly on the backtest capability crate.

use serde_json::{Value, json};
use thiserror::Error;

/// A validated candle handed to a strategy/Pine backtest adapter.
///
/// The shape intentionally stays independent of SQLite rows so the execution
/// port can be used by a worker or fixture without importing a store adapter.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct BacktestExecutionCandle {
    pub start_time: i64,
    pub end_time: i64,
    pub open: String,
    pub high: String,
    pub low: String,
    pub close: String,
    pub volume: String,
}

/// Input handed to a strategy/Pine backtest adapter after request and history
/// validation. The raw request is retained so adapters can preserve fields
/// that are not part of the Rust domain model yet.
#[derive(Clone, Debug)]
pub struct BacktestExecutionRequest {
    pub run_id: String,
    pub payload: Value,
    pub market_data_provider: String,
    pub candles: Vec<BacktestExecutionCandle>,
}

#[derive(Debug, Error, Clone, Eq, PartialEq)]
pub enum BacktestExecutionError {
    #[error("backtest execution is unavailable: {0}")]
    Unavailable(String),
    #[error("invalid backtest execution input: {0}")]
    Invalid(String),
    #[error("backtest execution failed: {0}")]
    Failed(String),
}

/// Narrow adapter contract for strategy/PineTS and the deterministic Rust
/// matcher. Implementations may perform blocking work; the product task
/// registry runs them behind `spawn_blocking` and fences the resulting write
/// with a status CAS.
pub trait BacktestExecutionPort: Send + Sync + std::fmt::Debug {
    fn execute(&self, request: BacktestExecutionRequest) -> Result<Value, BacktestExecutionError>;
}

/// Explicit adapter used by fixtures and local rehearsals. It invokes the
/// deterministic `jftrade-backtest` corpus boundary and returns decoded JSON.
/// A normal StartRequest is not itself a corpus; callers must provide a
/// `corpus` object (or a corpus-shaped payload) produced by the strategy/Pine
/// adapter.
#[derive(Clone, Copy, Debug, Default)]
pub struct RunJsonBacktestExecutionPort;

impl BacktestExecutionPort for RunJsonBacktestExecutionPort {
    fn execute(&self, request: BacktestExecutionRequest) -> Result<Value, BacktestExecutionError> {
        let mut corpus = request
            .payload
            .get("corpus")
            .cloned()
            .unwrap_or_else(|| request.payload.clone());
        // A strategy adapter may provide only corpus metadata while history is
        // resolved by the production market-data store. Fill an explicitly
        // empty first case from that validated history; never overwrite
        // worker-provided candles.
        if let Some(case) = corpus
            .get_mut("cases")
            .and_then(Value::as_array_mut)
            .and_then(|cases| cases.first_mut())
            && case
                .get("candles")
                .and_then(Value::as_array)
                .is_none_or(Vec::is_empty)
        {
            case["candles"] = Value::Array(request.candles.iter().map(candle_wire).collect());
        }
        let bytes = serde_json::to_vec(&corpus)
            .map_err(|error| BacktestExecutionError::Invalid(error.to_string()))?;
        let output = jftrade_backtest::run_json(&bytes)
            .map_err(|error| BacktestExecutionError::Failed(error.to_string()))?;
        serde_json::from_slice(&output)
            .map_err(|error| BacktestExecutionError::Failed(error.to_string()))
    }
}

fn candle_wire(candle: &BacktestExecutionCandle) -> Value {
    let timestamp = |millis: i64| {
        time::OffsetDateTime::from_unix_timestamp_nanos(i128::from(millis) * 1_000_000)
            .ok()
            .and_then(|value| {
                value
                    .format(&time::format_description::well_known::Rfc3339)
                    .ok()
            })
            .unwrap_or_else(|| "1970-01-01T00:00:00Z".to_owned())
    };
    json!({
        "start": timestamp(candle.start_time),
        "end": timestamp(candle.end_time),
        "open": candle.open,
        "high": candle.high,
        "low": candle.low,
        "close": candle.close,
        "volume": candle.volume,
    })
}

#[cfg(test)]
mod tests {
    use super::{
        BacktestExecutionCandle, BacktestExecutionPort, BacktestExecutionRequest,
        RunJsonBacktestExecutionPort,
    };
    use serde_json::json;

    #[test]
    fn run_json_adapter_fills_validated_history_into_empty_corpus_case() {
        let request = BacktestExecutionRequest {
            run_id: "run-fixture".to_owned(),
            payload: json!({
                "version": 1,
                "cases": [{
                    "id": "fixture",
                    "symbol": "US.AAPL",
                    "baseCurrency": "AAPL",
                    "quoteCurrency": "USD",
                    "initialBalance": "1000",
                    "market": {
                        "tickSize": "0.01",
                        "quantityStep": "1",
                        "minQuantity": "1"
                    },
                    "candles": []
                }]
            }),
            market_data_provider: "yfinance".to_owned(),
            candles: vec![BacktestExecutionCandle {
                start_time: 1_750_683_600_000,
                end_time: 1_750_683_659_999,
                open: "100".to_owned(),
                high: "101".to_owned(),
                low: "99".to_owned(),
                close: "100".to_owned(),
                volume: "10".to_owned(),
            }],
        };

        let output = RunJsonBacktestExecutionPort
            .execute(request)
            .expect("run deterministic corpus");
        assert_eq!(output["cases"][0]["processedBars"], 1);
    }
}
