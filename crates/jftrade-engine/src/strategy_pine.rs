//! Private Stage 9 leaf for the strategy-pine analyze route.
//!
//! Go remains the only production owner of Pine parsing and PineTS worker
//! lifecycle. This leaf only validates the HTTP-shaped input and forwards a
//! complete, opaque analysis projection through a consumer-owned snapshot
//! port. Product composition intentionally wires it only in an explicit
//! test-cutover profile.

use std::collections::BTreeMap;

use serde_json::Value;
use thiserror::Error;

pub const STRATEGY_PINE_ANALYZE_PATH: &str = "/api/v1/strategy-pine/analyze";
pub const JSON_CONTENT_TYPE: &str = "application/json; charset=utf-8";
pub const PINE_V6_SOURCE_FORMAT: &str = "pine-v6";

/// The normalized request handed to the Go-owned analyzer adapter.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct StrategyPineAnalyzeInput {
    pub script: String,
    pub source_format: String,
    pub include_ast: bool,
}

/// A complete Go-compatible response projection. `data` is intentionally an
/// opaque JSON value so Rust does not duplicate Pine's analysis schema.
#[derive(Clone, Debug, PartialEq)]
pub struct StrategyPineAnalyzeResponse {
    pub status: u16,
    pub headers: BTreeMap<String, String>,
    pub data: Option<Value>,
    pub error: Option<StrategyPineAnalyzeError>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct StrategyPineAnalyzeError {
    pub code: String,
    pub message: String,
    pub retry_after_seconds: Option<u64>,
}

/// Go-owned analysis and PineTS shadow projection boundary.
pub trait StrategyPineAnalyzeSnapshotPort: Send + Sync + std::fmt::Debug {
    fn analyze(
        &self,
        input: &StrategyPineAnalyzeInput,
    ) -> Result<Value, StrategyPineAnalyzeSnapshotError>;
}

#[derive(Clone, Debug, Error, Eq, PartialEq)]
pub enum StrategyPineAnalyzeSnapshotError {
    #[error("strategy-pine analyze snapshot is unavailable: {0}")]
    Unavailable(String),
    #[error("strategy-pine analyze snapshot failed: {code}: {message}")]
    Failed {
        status: u16,
        code: String,
        message: String,
        retry_after_seconds: Option<u64>,
    },
}

/// Replays the one-operation group without owning product composition.
pub fn dispatch_strategy_pine_analyze(
    port: Option<&dyn StrategyPineAnalyzeSnapshotPort>,
    method: &str,
    path: &str,
    body: &[u8],
) -> StrategyPineAnalyzeResponse {
    if method != "POST" || path != STRATEGY_PINE_ANALYZE_PATH {
        return error_response(404, "NOT_FOUND", format!("unknown endpoint {path}"), None);
    }

    let input = match decode_input(body) {
        Ok(input) => input,
        Err(message) => return error_response(400, "BAD_REQUEST", message, None),
    };
    if input.source_format != PINE_V6_SOURCE_FORMAT {
        return error_response(
            400,
            "BAD_REQUEST",
            "strategy-pine analyze supports pine-v6 only".to_owned(),
            None,
        );
    }

    let Some(port) = port else {
        return error_response(
            503,
            "STRATEGY_PINE_ANALYZE_UNAVAILABLE",
            "strategy-pine analyze snapshot is not configured".to_owned(),
            None,
        );
    };
    match port.analyze(&input) {
        Ok(data) => success_response(data),
        Err(error) => snapshot_error_response(error),
    }
}

fn decode_input(body: &[u8]) -> Result<StrategyPineAnalyzeInput, String> {
    let value: Value = serde_json::from_slice(body)
        .map_err(|_| "invalid strategy pine analyze payload".to_owned())?;
    let Some(object) = value.as_object() else {
        if value.is_null() {
            return Ok(zero_input());
        }
        return Err("invalid strategy pine analyze payload".to_owned());
    };

    let script = string_field(object, "script")?;
    let source_format = string_field(object, "sourceFormat")?;
    let include_ast = bool_field(object, "includeAst")?;
    let normalized_source_format = normalize_source_format(&source_format);
    Ok(StrategyPineAnalyzeInput {
        script,
        source_format: normalized_source_format,
        include_ast,
    })
}

fn zero_input() -> StrategyPineAnalyzeInput {
    StrategyPineAnalyzeInput {
        script: String::new(),
        source_format: PINE_V6_SOURCE_FORMAT.to_owned(),
        include_ast: false,
    }
}

fn string_field(object: &serde_json::Map<String, Value>, name: &str) -> Result<String, String> {
    match object.get(name) {
        None | Some(Value::Null) => Ok(String::new()),
        Some(Value::String(value)) => Ok(value.clone()),
        Some(_) => Err("invalid strategy pine analyze payload".to_owned()),
    }
}

fn bool_field(object: &serde_json::Map<String, Value>, name: &str) -> Result<bool, String> {
    match object.get(name) {
        None | Some(Value::Null) => Ok(false),
        Some(Value::Bool(value)) => Ok(*value),
        Some(_) => Err("invalid strategy pine analyze payload".to_owned()),
    }
}

fn normalize_source_format(value: &str) -> String {
    let normalized = value.trim().to_lowercase();
    if normalized.is_empty() {
        PINE_V6_SOURCE_FORMAT.to_owned()
    } else {
        normalized
    }
}

fn success_response(data: Value) -> StrategyPineAnalyzeResponse {
    StrategyPineAnalyzeResponse {
        status: 200,
        headers: content_type_headers(None),
        data: Some(data),
        error: None,
    }
}

fn snapshot_error_response(error: StrategyPineAnalyzeSnapshotError) -> StrategyPineAnalyzeResponse {
    match error {
        StrategyPineAnalyzeSnapshotError::Unavailable(message) => {
            error_response(503, "STRATEGY_PINE_ANALYZE_UNAVAILABLE", message, None)
        }
        StrategyPineAnalyzeSnapshotError::Failed {
            status,
            code,
            message,
            retry_after_seconds,
        } => error_response(status, code, message, retry_after_seconds),
    }
}

fn error_response(
    status: u16,
    code: impl Into<String>,
    message: impl Into<String>,
    retry_after_seconds: Option<u64>,
) -> StrategyPineAnalyzeResponse {
    StrategyPineAnalyzeResponse {
        status,
        headers: content_type_headers(retry_after_seconds),
        data: None,
        error: Some(StrategyPineAnalyzeError {
            code: code.into(),
            message: message.into(),
            retry_after_seconds,
        }),
    }
}

fn content_type_headers(retry_after_seconds: Option<u64>) -> BTreeMap<String, String> {
    let mut headers = BTreeMap::from([("Content-Type".to_owned(), JSON_CONTENT_TYPE.to_owned())]);
    if let Some(seconds) = retry_after_seconds {
        headers.insert("Retry-After".to_owned(), seconds.to_string());
    }
    headers
}
