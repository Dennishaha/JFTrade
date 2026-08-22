//! Stage 9 test-cutover leaf for the four backtest mutation routes.
//!
//! Go remains the only owner of strategy compilation, PineTS, market-data
//! sync, run/task stores, and asynchronous recovery. This module only binds
//! the public route shapes and delegates state changes to an explicitly
//! injected consumer-owned port; it is not registered by any product profile.

use std::collections::BTreeMap;

use percent_encoding::percent_decode_str;
use serde::Deserialize;
use serde_json::{Deserializer, Value, json};

pub const BACKTEST_START_PATH: &str = "/api/v1/backtests";
pub const BACKTEST_SYNC_START_PATH: &str = "/api/v1/backtests/sync";
pub const BACKTEST_SYNC_CANCEL_PATH: &str = "/api/v1/backtests/sync/{taskId}";
pub const BACKTEST_DELETE_PATH: &str = "/api/v1/backtests/{runId}";

pub const BACKTESTS_WRITE_ROUTES: [(&str, &str); 4] = [
    ("POST", BACKTEST_START_PATH),
    ("POST", BACKTEST_SYNC_START_PATH),
    ("DELETE", BACKTEST_SYNC_CANCEL_PATH),
    ("DELETE", BACKTEST_DELETE_PATH),
];

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum BacktestsWriteOperation {
    Start,
    Sync,
    CancelSync,
    Delete,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub enum BacktestsWriteInput {
    Start { payload: Value },
    Sync { payload: Value },
    CancelSync { task_id: String },
    Delete { run_id: String },
}

impl BacktestsWriteInput {
    pub const fn operation(&self) -> BacktestsWriteOperation {
        match self {
            Self::Start { .. } => BacktestsWriteOperation::Start,
            Self::Sync { .. } => BacktestsWriteOperation::Sync,
            Self::CancelSync { .. } => BacktestsWriteOperation::CancelSync,
            Self::Delete { .. } => BacktestsWriteOperation::Delete,
        }
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct BacktestsWriteRequest {
    pub method: String,
    pub path: String,
    pub body: Option<Vec<u8>>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
#[allow(dead_code)]
pub enum BacktestsWriteDeleteResult {
    Deleted,
    Missing,
    NotTerminal,
}

#[derive(Clone, Debug, Eq, PartialEq)]
#[allow(dead_code)]
pub enum BacktestsWritePortResult {
    Data(Value),
    SyncCancelled(bool),
    RunDeleted(BacktestsWriteDeleteResult),
}

#[derive(Clone, Debug, Eq, PartialEq)]
#[allow(dead_code)]
pub enum BacktestsWritePortError {
    Unavailable(String),
    BadRequest(String),
    StrategyNotFound(String),
    Failed(String),
}

/// Consumer-owned mutation boundary. A future integration adapter may call
/// the current Go-owned service, but this port itself has no store, worker,
/// provider, notification, or production-owner capability.
pub trait BacktestsWritePort: Send + Sync + std::fmt::Debug {
    fn mutate(
        &self,
        input: &BacktestsWriteInput,
    ) -> Result<BacktestsWritePortResult, BacktestsWritePortError>;
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct BacktestsWriteResponse {
    pub status: u16,
    pub headers: BTreeMap<String, String>,
    pub body: Value,
}

pub fn backtests_write_routes() -> &'static [(&'static str, &'static str); 4] {
    &BACKTESTS_WRITE_ROUTES
}

pub fn dispatch_backtests_write(
    request: &BacktestsWriteRequest,
    port: Option<&dyn BacktestsWritePort>,
    timestamp: &str,
) -> BacktestsWriteResponse {
    let input = match parse_input(request) {
        Ok(input) => input,
        Err(spec) => return error_response(spec, timestamp),
    };
    let Some(port) = port else {
        return error_response(
            ErrorSpec {
                status: 503,
                code: "BACKTESTS_WRITE_UNAVAILABLE".to_owned(),
                message: "backtests write port is unavailable".to_owned(),
            },
            timestamp,
        );
    };
    let operation = input.operation();
    match port.mutate(&input) {
        Ok(BacktestsWritePortResult::Data(data))
            if matches!(
                operation,
                BacktestsWriteOperation::Start | BacktestsWriteOperation::Sync
            ) =>
        {
            success_response(data, timestamp)
        }
        Ok(BacktestsWritePortResult::SyncCancelled(true))
            if operation == BacktestsWriteOperation::CancelSync =>
        {
            if let BacktestsWriteInput::CancelSync { task_id } = &input {
                success_response(json!({"taskId": task_id, "status": "cancelled"}), timestamp)
            } else {
                error_response(
                    operation_failure(operation, "backtests write port returned an invalid input"),
                    timestamp,
                )
            }
        }
        Ok(BacktestsWritePortResult::SyncCancelled(false))
            if operation == BacktestsWriteOperation::CancelSync =>
        {
            error_response(
                ErrorSpec {
                    status: 404,
                    code: "NOT_FOUND".to_owned(),
                    message: "sync task not found or already completed".to_owned(),
                },
                timestamp,
            )
        }
        Ok(BacktestsWritePortResult::RunDeleted(result))
            if operation == BacktestsWriteOperation::Delete =>
        {
            match result {
                BacktestsWriteDeleteResult::Deleted => {
                    if let BacktestsWriteInput::Delete { run_id } = &input {
                        success_response(json!({"deleted": true, "id": run_id}), timestamp)
                    } else {
                        error_response(
                            operation_failure(
                                operation,
                                "backtests write port returned an invalid input",
                            ),
                            timestamp,
                        )
                    }
                }
                BacktestsWriteDeleteResult::Missing => error_response(
                    ErrorSpec {
                        status: 404,
                        code: "NOT_FOUND".to_owned(),
                        message: "backtest run not found".to_owned(),
                    },
                    timestamp,
                ),
                BacktestsWriteDeleteResult::NotTerminal => error_response(
                    ErrorSpec {
                        status: 400,
                        code: "BAD_REQUEST".to_owned(),
                        message: "only completed, failed or cancelled backtest runs can be deleted"
                            .to_owned(),
                    },
                    timestamp,
                ),
            }
        }
        Ok(_) => error_response(
            operation_failure(operation, "backtests write port returned an invalid result"),
            timestamp,
        ),
        Err(error) => error_response(port_error(operation, error), timestamp),
    }
}

struct ErrorSpec {
    status: u16,
    code: String,
    message: String,
}

fn parse_input(request: &BacktestsWriteRequest) -> Result<BacktestsWriteInput, ErrorSpec> {
    let path = request
        .path
        .split_once('?')
        .map_or(request.path.as_str(), |pair| pair.0);
    if request.method == "POST" && path == BACKTEST_START_PATH {
        return parse_json_payload(request.body.as_deref(), "invalid backtest request")
            .map(|payload| BacktestsWriteInput::Start { payload });
    }
    if request.method == "POST" && path == BACKTEST_SYNC_START_PATH {
        return parse_json_payload(request.body.as_deref(), "invalid sync request")
            .map(|payload| BacktestsWriteInput::Sync { payload });
    }
    if request.method == "DELETE" {
        if let Some(raw_id) = path.strip_prefix("/api/v1/backtests/sync/") {
            if raw_id.is_empty() || raw_id.contains('/') {
                return Err(not_found_spec());
            }
            let task_id = decode_path_id(raw_id, "taskId is invalid")?;
            return Ok(BacktestsWriteInput::CancelSync {
                task_id: task_id.trim().to_owned(),
            });
        }
        if let Some(raw_id) = path.strip_prefix("/api/v1/backtests/") {
            if raw_id.is_empty() || raw_id.contains('/') {
                return Err(not_found_spec());
            }
            let run_id = decode_path_id(raw_id, "backtest run id is invalid")?;
            if run_id.trim().is_empty() {
                return Err(bad_request_spec("backtest run id is required"));
            }
            return Ok(BacktestsWriteInput::Delete {
                run_id: run_id.trim().to_owned(),
            });
        }
    }
    Err(not_found_spec())
}

fn parse_json_payload(body: Option<&[u8]>, message: &'static str) -> Result<Value, ErrorSpec> {
    let Some(body) = body.filter(|body| !body.is_empty()) else {
        return Err(bad_request_spec(message));
    };
    let mut decoder = Deserializer::from_slice(body);
    let value = Value::deserialize(&mut decoder).map_err(|_| bad_request_spec(message))?;
    if value.is_null() || value.is_object() {
        Ok(value)
    } else {
        Err(bad_request_spec(message))
    }
}

fn decode_path_id(raw_id: &str, label: &'static str) -> Result<String, ErrorSpec> {
    if !valid_percent_escapes(raw_id) {
        return Err(bad_request_spec(label));
    }
    let decoded = percent_decode_str(raw_id)
        .decode_utf8()
        .map_err(|_| bad_request_spec(label))?;
    if decoded.contains('/') {
        return Err(bad_request_spec(label));
    }
    Ok(decoded.into_owned())
}

fn valid_percent_escapes(value: &str) -> bool {
    let bytes = value.as_bytes();
    let mut index = 0;
    while index < bytes.len() {
        if bytes[index] != b'%' {
            index += 1;
            continue;
        }
        if index + 2 >= bytes.len()
            || !bytes[index + 1].is_ascii_hexdigit()
            || !bytes[index + 2].is_ascii_hexdigit()
        {
            return false;
        }
        index += 3;
    }
    true
}

fn port_error(operation: BacktestsWriteOperation, error: BacktestsWritePortError) -> ErrorSpec {
    match error {
        BacktestsWritePortError::Unavailable(message) => ErrorSpec {
            status: 503,
            code: "BACKTESTS_WRITE_UNAVAILABLE".to_owned(),
            message,
        },
        BacktestsWritePortError::BadRequest(message) => bad_request_spec_owned(message),
        BacktestsWritePortError::StrategyNotFound(message) => ErrorSpec {
            status: 404,
            code: "NOT_FOUND".to_owned(),
            message,
        },
        BacktestsWritePortError::Failed(message) => match operation {
            BacktestsWriteOperation::Start => ErrorSpec {
                status: 500,
                code: "BACKTEST_START_FAILED".to_owned(),
                message: "start backtest failed".to_owned(),
            },
            BacktestsWriteOperation::Sync => ErrorSpec {
                status: 500,
                code: "SYNC_FAILED".to_owned(),
                message,
            },
            BacktestsWriteOperation::CancelSync => ErrorSpec {
                status: 503,
                code: "BACKTESTS_WRITE_UNAVAILABLE".to_owned(),
                message,
            },
            BacktestsWriteOperation::Delete => ErrorSpec {
                status: 500,
                code: "BACKTEST_RUN_STORE_FAILED".to_owned(),
                message: "delete backtest run failed".to_owned(),
            },
        },
    }
}

fn operation_failure(operation: BacktestsWriteOperation, message: &str) -> ErrorSpec {
    port_error(
        operation,
        BacktestsWritePortError::Failed(message.to_owned()),
    )
}

fn bad_request_spec(message: &'static str) -> ErrorSpec {
    bad_request_spec_owned(message.to_owned())
}

fn bad_request_spec_owned(message: String) -> ErrorSpec {
    ErrorSpec {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message,
    }
}

fn not_found_spec() -> ErrorSpec {
    ErrorSpec {
        status: 404,
        code: "NOT_FOUND".to_owned(),
        message: "resource not found".to_owned(),
    }
}

fn success_response(data: Value, timestamp: &str) -> BacktestsWriteResponse {
    BacktestsWriteResponse {
        status: 200,
        headers: json_headers(),
        body: json!({"ok": true, "data": data, "timestamp": timestamp}),
    }
}

fn error_response(spec: ErrorSpec, timestamp: &str) -> BacktestsWriteResponse {
    BacktestsWriteResponse {
        status: spec.status,
        headers: json_headers(),
        body: json!({
            "ok": false,
            "error": {"code": spec.code, "message": spec.message},
            "timestamp": timestamp,
        }),
    }
}

fn json_headers() -> BTreeMap<String, String> {
    BTreeMap::from([(
        "Content-Type".to_owned(),
        "application/json; charset=utf-8".to_owned(),
    )])
}
