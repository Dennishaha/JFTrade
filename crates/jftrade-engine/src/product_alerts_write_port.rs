use std::collections::BTreeMap;

use percent_encoding::percent_decode_str;
use serde_json::{Value, json};

pub const PRICE_ALERT_WRITE_PATH: &str = "/api/v1/alerts/price";
pub const OPTION_EVENT_ALERT_WRITE_PATH: &str = "/api/v1/alerts/option-events";

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum AlertWriteRoute {
    Price,
    OptionEvents,
}

impl AlertWriteRoute {
    fn from_path(path: &str) -> Option<Self> {
        match path {
            PRICE_ALERT_WRITE_PATH => Some(Self::Price),
            OPTION_EVENT_ALERT_WRITE_PATH => Some(Self::OptionEvents),
            _ => None,
        }
    }

    pub fn feature_id(self) -> &'static str {
        match self {
            Self::Price => "alerts.price.set",
            Self::OptionEvents => "alerts.option_event.set",
        }
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub enum AlertWritePayloadState {
    Nil,
    EmptyObject,
    Object,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AlertWriteRequest {
    pub method: String,
    pub path: String,
    pub body: Option<Vec<u8>>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AlertWriteAction {
    pub route: AlertWriteRoute,
    pub feature_id: &'static str,
    pub broker_id: String,
    pub account_id: Option<String>,
    pub action: &'static str,
    pub payload: Option<Value>,
    pub payload_state: AlertWritePayloadState,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AlertWriteResolution {
    pub broker_id: String,
    pub security_firm: String,
    pub capability: String,
    pub selection_reason: String,
}

#[allow(dead_code)]
#[derive(Clone, Debug, Eq, PartialEq)]
pub enum AlertWritePortError {
    Unavailable(String),
    CapabilityUnavailable(String),
    Provider {
        status: Option<u16>,
        message: String,
    },
    Internal(String),
    RateLimited {
        retry_after: u64,
        message: String,
    },
}

pub trait AlertWritePort: Send + Sync {
    fn resolve(
        &self,
        route: AlertWriteRoute,
        broker_id: Option<&str>,
        account_id: Option<&str>,
    ) -> Result<AlertWriteResolution, AlertWritePortError>;

    fn apply(
        &self,
        resolution: &AlertWriteResolution,
        action: &AlertWriteAction,
    ) -> Result<Option<Value>, AlertWritePortError>;
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AlertWriteResponse {
    pub status: u16,
    pub headers: BTreeMap<String, String>,
    pub body: Value,
}

pub fn dispatch_alert_write(
    request: &AlertWriteRequest,
    port: Option<&dyn AlertWritePort>,
    timestamp: &str,
) -> AlertWriteResponse {
    let (path, raw_query) = split_path_query(&request.path);
    let Some(route) = AlertWriteRoute::from_path(path) else {
        return error_response(404, "NOT_FOUND", "resource not found", None, timestamp);
    };
    if request.method != "POST" {
        return error_response(404, "NOT_FOUND", "resource not found", None, timestamp);
    }

    let (payload, payload_state) = match parse_payload(request.body.as_deref()) {
        Ok(value) => value,
        Err(()) => {
            return error_response(400, "BAD_REQUEST", "invalid request body", None, timestamp);
        }
    };
    let broker_id = first_query_value(raw_query, "brokerId");
    let account_id = first_query_value(raw_query, "accountId");
    let Some(port) = port else {
        return error_response(
            503,
            "ALERTS_UNAVAILABLE",
            "alerts write port is unavailable",
            None,
            timestamp,
        );
    };
    let resolution = match port.resolve(
        route,
        non_empty(broker_id.as_deref()),
        non_empty(account_id.as_deref()),
    ) {
        Ok(resolution) => resolution,
        Err(error) => return port_error_response(error, timestamp),
    };
    let action = AlertWriteAction {
        route,
        feature_id: route.feature_id(),
        broker_id: resolution.broker_id.clone(),
        account_id: non_empty(account_id.as_deref()).map(str::to_owned),
        action: "set",
        payload,
        payload_state,
    };
    let result = match port.apply(&resolution, &action) {
        Ok(Some(Value::Object(result))) => result,
        Ok(Some(_)) => {
            return error_response(
                502,
                "BROKER_FEATURE_FAILED",
                "alert write port returned a non-object result",
                None,
                timestamp,
            );
        }
        Ok(None) => serde_json::Map::new(),
        Err(error) => return port_error_response(error, timestamp),
    };
    success_response(
        with_provider(result, &resolution, route.feature_id(), timestamp),
        timestamp,
    )
}

fn split_path_query(path: &str) -> (&str, &str) {
    path.split_once('?').unwrap_or((path, ""))
}

fn parse_payload(body: Option<&[u8]>) -> Result<(Option<Value>, AlertWritePayloadState), ()> {
    let body = body.ok_or(())?;
    let value: Value = serde_json::from_slice(body).map_err(|_| ())?;
    match value {
        Value::Null => Ok((None, AlertWritePayloadState::Nil)),
        Value::Object(object) if object.is_empty() => Ok((
            Some(Value::Object(object)),
            AlertWritePayloadState::EmptyObject,
        )),
        Value::Object(object) => Ok((Some(Value::Object(object)), AlertWritePayloadState::Object)),
        _ => Err(()),
    }
}

fn with_provider(
    mut result: serde_json::Map<String, Value>,
    resolution: &AlertWriteResolution,
    feature_id: &str,
    timestamp: &str,
) -> Value {
    let mut provider = serde_json::Map::new();
    provider.insert("brokerId".to_owned(), json!(resolution.broker_id));
    if !resolution.security_firm.is_empty() {
        provider.insert("securityFirm".to_owned(), json!(resolution.security_firm));
    }
    provider.insert("featureId".to_owned(), json!(feature_id));
    provider.insert("capability".to_owned(), json!(resolution.capability));
    provider.insert(
        "selectionReason".to_owned(),
        json!(resolution.selection_reason),
    );
    provider.insert("resolvedAt".to_owned(), json!(timestamp));
    provider.insert("asOf".to_owned(), json!(timestamp));
    result.insert("provider".to_owned(), Value::Object(provider));
    Value::Object(result)
}

fn success_response(data: Value, timestamp: &str) -> AlertWriteResponse {
    AlertWriteResponse {
        status: 200,
        headers: json_headers(None),
        body: json!({"ok": true, "data": data, "timestamp": timestamp}),
    }
}

fn port_error_response(error: AlertWritePortError, timestamp: &str) -> AlertWriteResponse {
    match error {
        AlertWritePortError::Unavailable(message) => {
            error_response(503, "ALERTS_UNAVAILABLE", &message, None, timestamp)
        }
        AlertWritePortError::CapabilityUnavailable(message) => error_response(
            409,
            "BROKER_CAPABILITY_UNAVAILABLE",
            &message,
            None,
            timestamp,
        ),
        AlertWritePortError::Provider { status, message } => {
            if status.is_some_and(|value| (400..500).contains(&value)) {
                error_response(
                    status.expect("checked provider status"),
                    "PROVIDER_REQUEST_FAILED",
                    &message,
                    None,
                    timestamp,
                )
            } else {
                error_response(502, "BROKER_FEATURE_FAILED", &message, None, timestamp)
            }
        }
        AlertWritePortError::Internal(message) => {
            error_response(502, "BROKER_FEATURE_FAILED", &message, None, timestamp)
        }
        AlertWritePortError::RateLimited {
            retry_after,
            message,
        } => error_response(
            429,
            "MARKET_SNAPSHOT_RATE_LIMITED",
            &message,
            Some(retry_after.max(1).to_string()),
            timestamp,
        ),
    }
}

fn error_response(
    status: u16,
    code: &str,
    message: &str,
    retry_after: Option<String>,
    timestamp: &str,
) -> AlertWriteResponse {
    AlertWriteResponse {
        status,
        headers: json_headers(retry_after),
        body: json!({
            "ok": false,
            "error": {"code": code, "message": message},
            "timestamp": timestamp,
        }),
    }
}

fn json_headers(retry_after: Option<String>) -> BTreeMap<String, String> {
    let mut headers = BTreeMap::new();
    headers.insert(
        "Content-Type".to_owned(),
        "application/json; charset=utf-8".to_owned(),
    );
    if let Some(retry_after) = retry_after {
        headers.insert("Retry-After".to_owned(), retry_after);
    }
    headers
}

fn first_query_value(query: &str, wanted: &str) -> Option<String> {
    query.split('&').find_map(|pair| {
        let (name, value) = pair.split_once('=').unwrap_or((pair, ""));
        (decode_query_component(name) == wanted).then(|| decode_query_component(value))
    })
}

fn decode_query_component(value: &str) -> String {
    percent_decode_str(&value.replace('+', " "))
        .decode_utf8_lossy()
        .into_owned()
}

fn non_empty(value: Option<&str>) -> Option<&str> {
    value.filter(|value| !value.trim().is_empty())
}
