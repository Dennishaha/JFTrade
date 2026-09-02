//! Stage 9 test-cutover leaf for strategy instance runtime mutations.
//!
//! Go remains the only production owner of the strategy catalog, runtime
//! manager, activity state, PineTS lifecycle, subscriptions, notifications,
//! and SQLite writes. This boundary only binds the existing HTTP-shaped
//! request and delegates the mutation to a consumer-owned test port.

use std::collections::BTreeMap;

use jftrade_api::ApiRequest;
use percent_encoding::percent_decode_str;
use serde::{Deserialize, Serialize};
use serde_json::{Map, Value, json};

pub const STRATEGY_INSTANCE_UPDATE_PATH: &str = "/api/v1/strategies/{instanceId}";
pub const STRATEGY_RUNTIME_RISK_UPDATE_PATH: &str = "/api/v1/strategies/{instanceId}/runtime-risk";
pub const STRATEGY_INSTANCE_DELETE_PATH: &str = "/api/v1/strategies/{instanceId}";
pub const STRATEGY_INSTANCE_PAUSE_PATH: &str = "/api/v1/strategies/{instanceId}/pause";
pub const STRATEGY_INSTANCE_STOP_PATH: &str = "/api/v1/strategies/{instanceId}/stop";
pub const STRATEGY_INSTANCE_START_PATH: &str = "/api/v1/strategies/{instanceId}/start";
pub const STRATEGY_INSTANCE_REFRESH_DEFINITION_PATH: &str =
    "/api/v1/strategies/{instanceId}/refresh-definition";

pub const STRATEGY_RUNTIME_WRITE_ROUTES: [(&str, &str); 7] = [
    ("PUT", STRATEGY_INSTANCE_UPDATE_PATH),
    ("PUT", STRATEGY_RUNTIME_RISK_UPDATE_PATH),
    ("DELETE", STRATEGY_INSTANCE_DELETE_PATH),
    ("POST", STRATEGY_INSTANCE_PAUSE_PATH),
    ("POST", STRATEGY_INSTANCE_STOP_PATH),
    ("POST", STRATEGY_INSTANCE_START_PATH),
    ("POST", STRATEGY_INSTANCE_REFRESH_DEFINITION_PATH),
];

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum StrategyRuntimeWriteOperation {
    Update,
    UpdateRuntimeRisk,
    Delete,
    Pause,
    Stop,
    Start,
    RefreshDefinition,
}

#[allow(dead_code)]
impl StrategyRuntimeWriteOperation {
    pub const fn name(self) -> &'static str {
        match self {
            Self::Update => "update",
            Self::UpdateRuntimeRisk => "update-runtime-risk",
            Self::Delete => "delete",
            Self::Pause => "pause",
            Self::Stop => "stop",
            Self::Start => "start",
            Self::RefreshDefinition => "refresh-definition",
        }
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct StrategyRuntimeWriteInput {
    pub operation: StrategyRuntimeWriteOperation,
    pub instance_id: String,
    pub binding: Option<Value>,
    pub runtime_risk: Option<Value>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct StrategyRuntimeWriteResponse {
    pub status: u16,
    pub headers: BTreeMap<String, String>,
    pub body: Value,
}

#[allow(dead_code)]
#[derive(Clone, Debug, Eq, PartialEq)]
pub enum StrategyRuntimeWritePortError {
    Unavailable(String),
    Failed {
        status: u16,
        code: String,
        message: String,
    },
}

/// Consumer-owned mutation boundary for all seven strategy runtime writes.
///
/// The injected implementation owns catalog/runtime coordination, persistence,
/// rollback, subscriptions, notifications, and recovery. The leaf never
/// opens SQLite, starts PineTS, calls a broker, or publishes a runtime event.
pub trait StrategyRuntimeWritePort: Send + Sync + std::fmt::Debug {
    fn mutate(
        &self,
        input: &StrategyRuntimeWriteInput,
    ) -> Result<Value, StrategyRuntimeWritePortError>;
}

pub fn strategy_runtime_write_routes() -> &'static [(&'static str, &'static str); 7] {
    &STRATEGY_RUNTIME_WRITE_ROUTES
}

pub fn dispatch_strategy_runtime_write(
    request: &ApiRequest,
    port: Option<&dyn StrategyRuntimeWritePort>,
    timestamp: &str,
) -> StrategyRuntimeWriteResponse {
    let (operation, instance_id) = match parse_route(&request.method, &request.path) {
        Ok(route) => route,
        Err(spec) => return error_response(spec, timestamp),
    };
    let input = match parse_input(operation, instance_id, &request.body) {
        Ok(input) => input,
        Err(spec) => return error_response(spec, timestamp),
    };
    let Some(port) = port else {
        return error_response(
            ErrorSpec {
                status: 503,
                code: "STRATEGY_UNAVAILABLE".to_owned(),
                message: "strategy runtime write port is unavailable".to_owned(),
            },
            timestamp,
        );
    };
    match port.mutate(&input) {
        Ok(data) => success_response(data, timestamp),
        Err(StrategyRuntimeWritePortError::Unavailable(message)) => error_response(
            ErrorSpec {
                status: 503,
                code: "STRATEGY_UNAVAILABLE".to_owned(),
                message,
            },
            timestamp,
        ),
        Err(StrategyRuntimeWritePortError::Failed {
            status,
            code,
            message,
        }) => error_response(
            ErrorSpec {
                status,
                code,
                message,
            },
            timestamp,
        ),
    }
}

struct ErrorSpec {
    status: u16,
    code: String,
    message: String,
}

fn parse_route(
    method: &str,
    path: &str,
) -> Result<(StrategyRuntimeWriteOperation, String), ErrorSpec> {
    let Some(suffix) = path.strip_prefix("/api/v1/strategies/") else {
        return Err(not_found_spec(path));
    };
    let mut parts = suffix.split('/');
    let raw_id = parts.next().unwrap_or_default();
    let action = parts.next();
    if parts.next().is_some() || raw_id.is_empty() {
        return Err(not_found_spec(path));
    }
    let operation = match (method, action) {
        ("PUT", None) => StrategyRuntimeWriteOperation::Update,
        ("DELETE", None) => StrategyRuntimeWriteOperation::Delete,
        ("PUT", Some("runtime-risk")) => StrategyRuntimeWriteOperation::UpdateRuntimeRisk,
        ("POST", Some("pause")) => StrategyRuntimeWriteOperation::Pause,
        ("POST", Some("stop")) => StrategyRuntimeWriteOperation::Stop,
        ("POST", Some("start")) => StrategyRuntimeWriteOperation::Start,
        ("POST", Some("refresh-definition")) => StrategyRuntimeWriteOperation::RefreshDefinition,
        _ => return Err(not_found_spec(path)),
    };
    decode_instance_id(raw_id)
        .map(|instance_id| (operation, instance_id))
        .map_err(bad_request_spec)
}

fn decode_instance_id(raw_id: &str) -> Result<String, String> {
    if has_invalid_percent_escape(raw_id) {
        return Err("invalid instance id".to_owned());
    }
    let decoded = percent_decode_str(raw_id)
        .decode_utf8()
        .map_err(|_| "invalid instance id".to_owned())?;
    if decoded.is_empty() || decoded.contains('/') {
        return Err("invalid instance id".to_owned());
    }
    Ok(decoded.into_owned())
}

fn has_invalid_percent_escape(value: &str) -> bool {
    let bytes = value.as_bytes();
    let mut index = 0;
    while index < bytes.len() {
        if bytes[index] != b'%' {
            index += 1;
            continue;
        }
        if index + 2 >= bytes.len() || !is_hex(bytes[index + 1]) || !is_hex(bytes[index + 2]) {
            return true;
        }
        index += 3;
    }
    false
}

fn is_hex(value: u8) -> bool {
    value.is_ascii_hexdigit()
}

fn parse_input(
    operation: StrategyRuntimeWriteOperation,
    instance_id: String,
    body: &[u8],
) -> Result<StrategyRuntimeWriteInput, ErrorSpec> {
    let (binding, runtime_risk) = match operation {
        StrategyRuntimeWriteOperation::Update => (Some(parse_binding(body)?), None),
        StrategyRuntimeWriteOperation::UpdateRuntimeRisk => (None, Some(parse_runtime_risk(body)?)),
        StrategyRuntimeWriteOperation::Delete
        | StrategyRuntimeWriteOperation::Pause
        | StrategyRuntimeWriteOperation::Stop
        | StrategyRuntimeWriteOperation::Start
        | StrategyRuntimeWriteOperation::RefreshDefinition => (None, None),
    };
    Ok(StrategyRuntimeWriteInput {
        operation,
        instance_id,
        binding,
        runtime_risk,
    })
}

fn parse_binding(body: &[u8]) -> Result<Value, ErrorSpec> {
    let value = parse_first_json_object(body, "invalid instance payload")?;
    let binding = serde_json::from_value::<InstanceBindingWire>(value)
        .map_err(|_| bad_request_spec("invalid instance payload"))?;
    serde_json::to_value(binding).map_err(|_| bad_request_spec("invalid instance payload"))
}

fn parse_runtime_risk(body: &[u8]) -> Result<Value, ErrorSpec> {
    let value = parse_first_json_object(body, "invalid runtime risk payload")?;
    let risk = serde_json::from_value::<RuntimeRiskWire>(value)
        .map_err(|_| bad_request_spec("invalid runtime risk payload"))?;
    serde_json::to_value(risk).map_err(|_| bad_request_spec("invalid runtime risk payload"))
}

fn parse_first_json_object(body: &[u8], message: &'static str) -> Result<Value, ErrorSpec> {
    if body.is_empty() {
        return Err(bad_request_spec(message));
    }
    let mut decoder = serde_json::Deserializer::from_slice(body);
    let value = Value::deserialize(&mut decoder).map_err(|_| bad_request_spec(message))?;
    match value {
        Value::Null => Ok(Value::Object(Map::new())),
        Value::Object(object) => Ok(Value::Object(object)),
        _ => Err(bad_request_spec(message)),
    }
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
struct InstanceBindingWire {
    #[serde(default, skip_serializing_if = "option_vec_is_none_or_empty")]
    instruments: Option<Vec<BindingInstrumentWire>>,
    #[serde(default)]
    symbols: Option<Vec<String>>,
    #[serde(default)]
    interval: String,
    #[serde(default)]
    chart_type: String,
    #[serde(default)]
    execution_mode: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    broker_account: Option<BrokerAccountWire>,
    #[serde(default)]
    runtime_risk: RuntimeRiskWire,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
struct BindingInstrumentWire {
    #[serde(default)]
    market: String,
    #[serde(default)]
    code: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
struct BrokerAccountWire {
    #[serde(default)]
    broker_id: String,
    #[serde(default)]
    account_id: String,
    #[serde(default)]
    trading_environment: String,
    #[serde(default)]
    market: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
struct RuntimeRiskWire {
    #[serde(default)]
    mode: String,
    #[serde(default)]
    close_only: bool,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    max_order_quantity: Option<f64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    max_order_notional: Option<f64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    daily_max_orders: Option<i64>,
    #[serde(default)]
    pause_on_reject: bool,
}

fn option_vec_is_none_or_empty<T>(value: &Option<Vec<T>>) -> bool {
    value.as_ref().is_none_or(Vec::is_empty)
}

fn success_response(data: Value, timestamp: &str) -> StrategyRuntimeWriteResponse {
    StrategyRuntimeWriteResponse {
        status: 200,
        headers: json_headers(),
        body: json!({"ok": true, "data": data, "timestamp": timestamp}),
    }
}

fn error_response(spec: ErrorSpec, timestamp: &str) -> StrategyRuntimeWriteResponse {
    StrategyRuntimeWriteResponse {
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

fn bad_request_spec(message: impl Into<String>) -> ErrorSpec {
    ErrorSpec {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: message.into(),
    }
}

fn not_found_spec(path: &str) -> ErrorSpec {
    ErrorSpec {
        status: 404,
        code: "NOT_FOUND".to_owned(),
        message: format!("unknown endpoint {path}"),
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::*;

    #[derive(Debug)]
    struct RecordingPort;

    impl StrategyRuntimeWritePort for RecordingPort {
        fn mutate(
            &self,
            input: &StrategyRuntimeWriteInput,
        ) -> Result<Value, StrategyRuntimeWritePortError> {
            Ok(json!({
                "operation": input.operation.name(),
                "instanceId": input.instance_id,
                "binding": input.binding,
                "runtimeRisk": input.runtime_risk,
            }))
        }
    }

    fn request(method: &str, path: &str, body: &[u8]) -> ApiRequest {
        ApiRequest {
            method: method.to_owned(),
            path: path.to_owned(),
            query: String::new(),
            body: body.to_vec(),
            request_id: "strategy-runtime-write-test".to_owned(),
            desktop_trusted: true,
            origin_provided: false,
            origin_allowed: true,
            browser_authenticated: true,
            csrf_valid: false,
            session_cookie: None,
        }
    }

    #[test]
    fn route_contract_has_exactly_seven_mutations() {
        assert_eq!(strategy_runtime_write_routes().len(), 7);
        assert_eq!(
            strategy_runtime_write_routes()
                .iter()
                .filter(|(method, _)| *method == "POST")
                .count(),
            4
        );
        assert_eq!(
            strategy_runtime_write_routes()
                .iter()
                .filter(|(method, _)| *method == "PUT")
                .count(),
            2
        );
    }

    #[test]
    fn go_json_binding_accepts_null_and_ignores_trailing_values() {
        let port = Arc::new(RecordingPort);
        let response = dispatch_strategy_runtime_write(
            &request("PUT", "/api/v1/strategies/instance-1", b"null{}"),
            Some(port.as_ref()),
            "2026-08-23T00:00:00Z",
        );
        assert_eq!(response.status, 200);
        assert_eq!(response.body["data"]["binding"]["symbols"], Value::Null);
        assert_eq!(
            response.body["data"]["binding"]["runtimeRisk"]["closeOnly"],
            false
        );
    }

    #[test]
    fn malformed_input_precedes_missing_port_and_non_mutations_are_isolated() {
        let malformed = dispatch_strategy_runtime_write(
            &request("PUT", "/api/v1/strategies/instance-1", b"{"),
            None,
            "2026-08-23T00:00:00Z",
        );
        assert_eq!(malformed.status, 400);
        assert_eq!(malformed.body["error"]["code"], "BAD_REQUEST");

        let read = dispatch_strategy_runtime_write(
            &request("GET", "/api/v1/strategies", b""),
            None,
            "2026-08-23T00:00:00Z",
        );
        assert_eq!(read.status, 404);
        assert_eq!(read.body["error"]["code"], "NOT_FOUND");
    }

    #[test]
    fn valid_request_fails_closed_without_test_port() {
        let response = dispatch_strategy_runtime_write(
            &request(
                "POST",
                "/api/v1/strategies/instance-1/start",
                b"malformed body is ignored by Go control routes",
            ),
            None,
            "2026-08-23T00:00:00Z",
        );
        assert_eq!(response.status, 503);
        assert_eq!(response.body["error"]["code"], "STRATEGY_UNAVAILABLE");
    }
}
