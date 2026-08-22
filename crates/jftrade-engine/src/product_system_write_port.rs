//! Stage 9 test-cutover leaf for system control mutations.
//!
//! Go remains the only owner of the OpenD runtime, real-trade control plane,
//! persistence, broker commands, and user-visible side effects.  This leaf
//! only preserves the HTTP boundary and delegates each mutation to an
//! explicitly injected test port.  It is intentionally not wired into the
//! default product profile.

use std::collections::BTreeMap;

use percent_encoding::percent_decode_str;
use serde::{Deserialize, Serialize, Serializer};
use serde_json::{Map, Value, json};
use thiserror::Error;

pub const SYSTEM_FUTU_OPEND_MANUAL_RETRY_PATH: &str = "/api/v1/system/futu-opend/manual-retry";
pub const SYSTEM_HARD_STOP_ACTIVATE_PATH: &str = "/api/v1/system/real-trade-hard-stops";
pub const SYSTEM_HARD_STOP_RELEASE_PATH: &str =
    "/api/v1/system/real-trade-hard-stops/{hardStopId}/release";
pub const SYSTEM_KILL_SWITCH_ACTIVATE_PATH: &str = "/api/v1/system/real-trade-kill-switch/activate";
pub const SYSTEM_KILL_SWITCH_RELEASE_PATH: &str = "/api/v1/system/real-trade-kill-switch/release";
pub const SYSTEM_RISK_UPDATE_PATH: &str = "/api/v1/system/real-trade-risk-limits";
pub const SYSTEM_RISK_DISABLE_PATH: &str = "/api/v1/system/real-trade-risk-limits";

pub const SYSTEM_WRITE_ROUTES: [(&str, &str); 7] = [
    ("DELETE", SYSTEM_RISK_DISABLE_PATH),
    ("POST", SYSTEM_FUTU_OPEND_MANUAL_RETRY_PATH),
    ("POST", SYSTEM_HARD_STOP_ACTIVATE_PATH),
    ("POST", SYSTEM_HARD_STOP_RELEASE_PATH),
    ("POST", SYSTEM_KILL_SWITCH_ACTIVATE_PATH),
    ("POST", SYSTEM_KILL_SWITCH_RELEASE_PATH),
    ("PUT", SYSTEM_RISK_UPDATE_PATH),
];

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum SystemWriteOperation {
    DisableRisk,
    ManualRetry,
    ActivateHardStop,
    ReleaseHardStop,
    ActivateKillSwitch,
    ReleaseKillSwitch,
    UpdateRisk,
}

#[allow(dead_code)]
impl SystemWriteOperation {
    pub const fn name(self) -> &'static str {
        match self {
            Self::DisableRisk => "disable-risk",
            Self::ManualRetry => "manual-retry",
            Self::ActivateHardStop => "activate-hard-stop",
            Self::ReleaseHardStop => "release-hard-stop",
            Self::ActivateKillSwitch => "activate-kill-switch",
            Self::ReleaseKillSwitch => "release-kill-switch",
            Self::UpdateRisk => "update-risk",
        }
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct SystemWriteRequest {
    pub method: String,
    pub path: String,
    pub body: Vec<u8>,
}

#[derive(Clone, Debug, PartialEq)]
pub struct SystemWriteInput {
    pub operation: SystemWriteOperation,
    pub hard_stop_id: Option<String>,
    pub kill_switch: Option<RealTradeKillSwitchCommand>,
    pub hard_stop: Option<RealTradeHardStopCommand>,
    pub risk: Option<RealTradeRuntimeRiskCommand>,
}

#[derive(Clone, Debug, Default, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RealTradeKillSwitchCommand {
    #[serde(default)]
    pub trading_environment: String,
    #[serde(default)]
    pub operator_id: String,
    #[serde(default)]
    pub reason: String,
}

#[derive(Clone, Debug, Default, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RealTradeHardStopCommand {
    #[serde(default)]
    pub broker_id: String,
    #[serde(default)]
    pub trading_environment: String,
    #[serde(default)]
    pub account_id: String,
    #[serde(default)]
    pub market: String,
    #[serde(default)]
    pub symbol: String,
    #[serde(default)]
    pub hard_stop_scope: String,
    #[serde(default)]
    pub operator_id: String,
    #[serde(default)]
    pub reason: String,
}

#[derive(Clone, Debug, Default, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RealTradeRuntimeRiskCommand {
    #[serde(default)]
    pub trading_environment: String,
    #[serde(default)]
    pub real_trading_enabled: bool,
    #[serde(default)]
    #[serde(serialize_with = "serialize_go_optional_float")]
    pub max_order_quantity: Option<f64>,
    #[serde(default)]
    #[serde(serialize_with = "serialize_go_optional_float")]
    pub max_order_notional: Option<f64>,
    #[serde(default)]
    pub operator_id: String,
    #[serde(default)]
    pub reason: String,
}

fn serialize_go_optional_float<S>(value: &Option<f64>, serializer: S) -> Result<S::Ok, S::Error>
where
    S: Serializer,
{
    match value {
        Some(value) if value.is_finite() && value.fract() == 0.0 => {
            serializer.serialize_i64(*value as i64)
        }
        Some(value) => serializer.serialize_f64(*value),
        None => serializer.serialize_none(),
    }
}

#[allow(dead_code)]
#[derive(Clone, Debug, Eq, PartialEq, Error)]
pub enum SystemWritePortError {
    #[error("system write port is unavailable: {0}")]
    Unavailable(String),
    #[error("{code}: {message}")]
    Failed {
        status: u16,
        code: String,
        message: String,
    },
}

/// Consumer-owned boundary for all seven system writes.
///
/// The injected adapter owns OpenD reset, real-trade state, persistence,
/// broker fencing, rollback, cancellation and recovery.  The leaf has no
/// database, broker, OpenD, notification, or transaction capability.
pub trait SystemWritePort: Send + Sync + std::fmt::Debug {
    fn mutate(&self, input: &SystemWriteInput) -> Result<Value, SystemWritePortError>;
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct SystemWriteResponse {
    pub status: u16,
    pub headers: BTreeMap<String, String>,
    pub body: Value,
}

pub fn system_write_routes() -> &'static [(&'static str, &'static str); 7] {
    &SYSTEM_WRITE_ROUTES
}

pub fn dispatch_system_write(
    request: &SystemWriteRequest,
    port: Option<&dyn SystemWritePort>,
    timestamp: &str,
) -> SystemWriteResponse {
    let path = request.path.split('?').next().unwrap_or(&request.path);
    let (operation, hard_stop_id) = match parse_route(&request.method, path) {
        Ok(route) => route,
        Err(error) => return error_response(error, timestamp),
    };
    let input = match parse_input(operation, hard_stop_id, &request.body) {
        Ok(input) => input,
        Err(error) => return error_response(error, timestamp),
    };
    let Some(port) = port else {
        return error_response(
            ErrorSpec {
                status: 503,
                code: "SYSTEM_WRITE_UNAVAILABLE".to_owned(),
                message: "system write port is unavailable".to_owned(),
            },
            timestamp,
        );
    };
    match port.mutate(&input) {
        Ok(data) => success_response(data, timestamp),
        Err(SystemWritePortError::Unavailable(message)) => error_response(
            ErrorSpec {
                status: 503,
                code: "SYSTEM_WRITE_UNAVAILABLE".to_owned(),
                message,
            },
            timestamp,
        ),
        Err(SystemWritePortError::Failed {
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

#[derive(Clone, Debug, Eq, PartialEq)]
struct ErrorSpec {
    status: u16,
    code: String,
    message: String,
}

fn parse_route(
    method: &str,
    path: &str,
) -> Result<(SystemWriteOperation, Option<String>), ErrorSpec> {
    let exact = match (method, path) {
        ("DELETE", SYSTEM_RISK_DISABLE_PATH) => Some(SystemWriteOperation::DisableRisk),
        ("POST", SYSTEM_FUTU_OPEND_MANUAL_RETRY_PATH) => Some(SystemWriteOperation::ManualRetry),
        ("POST", SYSTEM_HARD_STOP_ACTIVATE_PATH) => Some(SystemWriteOperation::ActivateHardStop),
        ("POST", SYSTEM_KILL_SWITCH_ACTIVATE_PATH) => {
            Some(SystemWriteOperation::ActivateKillSwitch)
        }
        ("POST", SYSTEM_KILL_SWITCH_RELEASE_PATH) => Some(SystemWriteOperation::ReleaseKillSwitch),
        ("PUT", SYSTEM_RISK_UPDATE_PATH) => Some(SystemWriteOperation::UpdateRisk),
        _ => None,
    };
    if let Some(operation) = exact {
        return Ok((operation, None));
    }

    let prefix = "/api/v1/system/real-trade-hard-stops/";
    if method != "POST" || !path.starts_with(prefix) || !path.ends_with("/release") {
        return Err(not_found_spec(path));
    }
    let raw_id = &path[prefix.len()..path.len() - "/release".len()];
    if raw_id.is_empty() || raw_id.contains('/') {
        return Err(not_found_spec(path));
    }
    let hard_stop_id = decode_path_segment(raw_id).map_err(bad_request_spec)?;
    let hard_stop_id = hard_stop_id.trim().to_owned();
    if hard_stop_id.is_empty() {
        return Err(bad_request_spec("hard stop id is required"));
    }
    Ok((SystemWriteOperation::ReleaseHardStop, Some(hard_stop_id)))
}

fn decode_path_segment(raw_id: &str) -> Result<String, String> {
    if has_invalid_percent_escape(raw_id) {
        return Err("invalid hard stop id".to_owned());
    }
    percent_decode_str(raw_id)
        .decode_utf8()
        .map(|value| value.into_owned())
        .map_err(|_| "invalid hard stop id".to_owned())
}

fn has_invalid_percent_escape(value: &str) -> bool {
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
            return true;
        }
        index += 3;
    }
    false
}

fn parse_input(
    operation: SystemWriteOperation,
    hard_stop_id: Option<String>,
    body: &[u8],
) -> Result<SystemWriteInput, ErrorSpec> {
    let (kill_switch, hard_stop, risk) = match operation {
        SystemWriteOperation::ManualRetry => (None, None, None),
        SystemWriteOperation::ActivateKillSwitch => (
            Some(parse_body(
                body,
                true,
                "invalid real-trade kill switch payload",
            )?),
            None,
            None,
        ),
        SystemWriteOperation::ReleaseKillSwitch => (
            Some(parse_body(
                body,
                false,
                "invalid real-trade kill switch release payload",
            )?),
            None,
            None,
        ),
        SystemWriteOperation::ActivateHardStop => (
            None,
            Some(parse_body(
                body,
                true,
                "invalid real-trade hard stop payload",
            )?),
            None,
        ),
        SystemWriteOperation::ReleaseHardStop => (
            None,
            Some(parse_body(
                body,
                false,
                "invalid real-trade hard stop release payload",
            )?),
            None,
        ),
        SystemWriteOperation::UpdateRisk => (
            None,
            None,
            Some(parse_body(
                body,
                true,
                "invalid real-trade runtime risk payload",
            )?),
        ),
        SystemWriteOperation::DisableRisk => (
            None,
            None,
            Some(parse_body(
                body,
                false,
                "invalid real-trade runtime risk disable payload",
            )?),
        ),
    };

    if let Some(command) = risk.as_ref() {
        validate_risk(command)?;
    }
    Ok(SystemWriteInput {
        operation,
        hard_stop_id,
        kill_switch,
        hard_stop,
        risk,
    })
}

fn parse_body<T>(body: &[u8], required: bool, message: &'static str) -> Result<T, ErrorSpec>
where
    T: for<'de> Deserialize<'de> + Default,
{
    if body.is_empty() {
        return if required {
            Err(bad_request_spec(message))
        } else {
            Ok(T::default())
        };
    }
    let mut decoder = serde_json::Deserializer::from_slice(body);
    let value = Value::deserialize(&mut decoder).map_err(|_| bad_request_spec(message))?;
    let object = match value {
        Value::Null => Map::new(),
        Value::Object(object) => object,
        _ => return Err(bad_request_spec(message)),
    };
    serde_json::from_value(Value::Object(object)).map_err(|_| bad_request_spec(message))
}

fn validate_risk(command: &RealTradeRuntimeRiskCommand) -> Result<(), ErrorSpec> {
    if command
        .max_order_quantity
        .is_some_and(|value| value <= 0.0 || !value.is_finite())
    {
        return Err(bad_request_spec(
            "maxOrderQuantity must be positive when provided",
        ));
    }
    if command
        .max_order_notional
        .is_some_and(|value| value <= 0.0 || !value.is_finite())
    {
        return Err(bad_request_spec(
            "maxOrderNotional must be positive when provided",
        ));
    }
    if command.real_trading_enabled
        && command.max_order_quantity.is_none()
        && command.max_order_notional.is_none()
    {
        return Err(bad_request_spec(
            "at least one positive runtime risk limit is required before enabling real trading",
        ));
    }
    Ok(())
}

fn success_response(data: Value, timestamp: &str) -> SystemWriteResponse {
    SystemWriteResponse {
        status: 200,
        headers: json_headers(),
        body: json!({"ok": true, "data": data, "timestamp": timestamp}),
    }
}

fn error_response(error: ErrorSpec, timestamp: &str) -> SystemWriteResponse {
    SystemWriteResponse {
        status: error.status,
        headers: json_headers(),
        body: json!({
            "ok": false,
            "error": {"code": error.code, "message": error.message},
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
    use super::*;

    #[derive(Debug)]
    struct RecordingPort;

    impl SystemWritePort for RecordingPort {
        fn mutate(&self, input: &SystemWriteInput) -> Result<Value, SystemWritePortError> {
            Ok(json!({"operation": input.operation.name()}))
        }
    }

    fn request(method: &str, path: &str, body: &[u8]) -> SystemWriteRequest {
        SystemWriteRequest {
            method: method.to_owned(),
            path: path.to_owned(),
            body: body.to_vec(),
        }
    }

    #[test]
    fn route_contract_has_exactly_seven_operations() {
        assert_eq!(system_write_routes().len(), 7);
        assert_eq!(
            system_write_routes()
                .iter()
                .filter(|(method, _)| *method == "POST")
                .count(),
            5
        );
        assert!(system_write_routes().contains(&("DELETE", SYSTEM_RISK_DISABLE_PATH)));
        assert!(system_write_routes().contains(&("PUT", SYSTEM_RISK_UPDATE_PATH)));
    }

    #[test]
    fn go_json_boundary_accepts_null_and_ignores_trailing_values() {
        let response = dispatch_system_write(
            &request(
                "PUT",
                SYSTEM_RISK_UPDATE_PATH,
                br#"{"maxOrderQuantity":1.5}{"ignored":true}"#,
            ),
            Some(&RecordingPort),
            "fixture-time",
        );
        assert_eq!(response.status, 200);
        let response = dispatch_system_write(
            &request("DELETE", SYSTEM_RISK_DISABLE_PATH, b"null"),
            Some(&RecordingPort),
            "fixture-time",
        );
        assert_eq!(response.status, 200);
    }

    #[test]
    fn validation_precedes_port_and_path_id_is_trimmed() {
        let malformed = dispatch_system_write(
            &request(
                "PUT",
                SYSTEM_RISK_UPDATE_PATH,
                br#"{"realTradingEnabled":true}"#,
            ),
            Some(&RecordingPort),
            "fixture-time",
        );
        assert_eq!(malformed.status, 400);
        assert_eq!(malformed.body["error"]["code"], "BAD_REQUEST");

        let response = dispatch_system_write(
            &request(
                "POST",
                "/api/v1/system/real-trade-hard-stops/%20hs-1%20/release",
                b"",
            ),
            Some(&RecordingPort),
            "fixture-time",
        );
        assert_eq!(response.status, 200);
    }

    #[test]
    fn valid_request_fails_closed_without_test_port() {
        let response = dispatch_system_write(
            &request("POST", SYSTEM_KILL_SWITCH_RELEASE_PATH, b""),
            None,
            "fixture-time",
        );
        assert_eq!(response.status, 503);
        assert_eq!(response.body["error"]["code"], "SYSTEM_WRITE_UNAVAILABLE");
    }

    #[test]
    fn read_route_is_not_a_system_write() {
        let response = dispatch_system_write(
            &request("GET", "/api/v1/system/real-trade-risk-limits", b""),
            None,
            "fixture-time",
        );
        assert_eq!(response.status, 404);
        assert_eq!(response.body["error"]["code"], "NOT_FOUND");
    }
}
