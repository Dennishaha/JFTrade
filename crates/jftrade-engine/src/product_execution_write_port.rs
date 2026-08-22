//! Stage 9 test-cutover leaf for the seven execution mutation routes.
//!
//! Go remains the only owner of broker sessions, pre-trade risk, the execution
//! ledger, order-update workers, preview/RFQ state, and every command side
//! effect.  This file only preserves the HTTP-shaped input boundary and lets a
//! consumer-owned test port replay the Go result.  It is deliberately not
//! included from `product.rs`; the integration branch may add the smallest
//! authenticated test-cutover hook after this worker is reviewed.

use std::collections::BTreeMap;

use percent_encoding::percent_decode_str;
use serde::Deserialize;
use serde_json::{Deserializer, Value, json};

pub const EXECUTION_BUYING_POWER_PATH: &str = "/api/v1/execution/buying-power";
pub const EXECUTION_COMBO_PREVIEW_PATH: &str = "/api/v1/execution/combos/previews";
pub const EXECUTION_COMBO_PLACE_PATH: &str = "/api/v1/execution/combos";
pub const EXECUTION_COMBO_CANCEL_PATH: &str = "/api/v1/execution/combos/{internalOrderId}/cancel";
pub const EXECUTION_ORDER_PLACE_PATH: &str = "/api/v1/execution/orders";
pub const EXECUTION_ORDER_CANCEL_PATH: &str = "/api/v1/execution/orders/{internalOrderId}/cancel";
pub const EXECUTION_ORDER_PREVIEW_PATH: &str = "/api/v1/execution/previews";

pub const EXECUTION_WRITE_ROUTES: [(&str, &str); 7] = [
    ("POST", EXECUTION_BUYING_POWER_PATH),
    ("POST", EXECUTION_COMBO_PREVIEW_PATH),
    ("POST", EXECUTION_COMBO_PLACE_PATH),
    ("POST", EXECUTION_COMBO_CANCEL_PATH),
    ("POST", EXECUTION_ORDER_PLACE_PATH),
    ("POST", EXECUTION_ORDER_CANCEL_PATH),
    ("POST", EXECUTION_ORDER_PREVIEW_PATH),
];

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum ExecutionWriteOperation {
    BuyingPower,
    ComboPreview,
    ComboPlace,
    ComboCancel,
    OrderPlace,
    OrderCancel,
    OrderPreview,
}

impl ExecutionWriteOperation {
    pub const fn name(self) -> &'static str {
        match self {
            Self::BuyingPower => "buying-power",
            Self::ComboPreview => "combo-preview",
            Self::ComboPlace => "combo-place",
            Self::ComboCancel => "combo-cancel",
            Self::OrderPlace => "order-place",
            Self::OrderCancel => "order-cancel",
            Self::OrderPreview => "order-preview",
        }
    }

    const fn body_error(self) -> &'static str {
        match self {
            Self::BuyingPower => "invalid buying-power request",
            Self::ComboPreview => "invalid combo preview payload",
            Self::ComboPlace => "invalid combo order payload",
            Self::OrderPlace | Self::OrderPreview => "invalid execution order payload",
            Self::ComboCancel | Self::OrderCancel => "",
        }
    }
}

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub enum ExecutionWriteContext {
    #[default]
    Normal,
    Canceled,
    Deadline,
}

#[derive(Clone, Debug, PartialEq)]
pub struct ExecutionWriteRequest {
    pub method: String,
    pub path: String,
    pub body: Option<Vec<u8>>,
    pub context: ExecutionWriteContext,
}

#[derive(Clone, Debug, PartialEq)]
pub struct ExecutionWriteInput {
    pub operation: ExecutionWriteOperation,
    pub internal_order_id: Option<String>,
    pub payload: Value,
    pub context: ExecutionWriteContext,
}

#[allow(dead_code)]
#[derive(Clone, Debug, Eq, PartialEq)]
pub enum ExecutionWritePortError {
    Unavailable(String),
    Failed {
        status: u16,
        code: String,
        message: String,
    },
}

#[derive(Clone, Debug, PartialEq)]
pub struct ExecutionWriteResponse {
    pub status: u16,
    pub headers: BTreeMap<String, String>,
    pub body: Value,
}

/// Consumer-owned mutation boundary.  The future adapter must delegate to
/// the current Go execution service and return its already-computed result;
/// this leaf never opens SQLite, starts OpenD, submits an order, or emits an
/// event.
pub trait ExecutionWritePort: Send + Sync + std::fmt::Debug {
    fn mutate(&self, input: &ExecutionWriteInput) -> Result<Value, ExecutionWritePortError>;
}

pub fn execution_write_routes() -> &'static [(&'static str, &'static str); 7] {
    &EXECUTION_WRITE_ROUTES
}

pub fn dispatch_execution_write(
    request: &ExecutionWriteRequest,
    port: Option<&dyn ExecutionWritePort>,
    timestamp: &str,
) -> ExecutionWriteResponse {
    let (operation, internal_order_id) = match parse_route(&request.method, &request.path) {
        Ok(route) => route,
        Err(spec) => return error_response(spec, timestamp),
    };
    let payload = if operation.body_error().is_empty() {
        Value::Null
    } else {
        match parse_json_object(request.body.as_deref(), operation) {
            Ok(payload) => payload,
            Err(spec) => return error_response(spec, timestamp),
        }
    };
    let input = ExecutionWriteInput {
        operation,
        internal_order_id,
        payload,
        context: request.context,
    };
    let Some(port) = port else {
        return error_response(
            ErrorSpec {
                status: 503,
                code: "EXECUTION_WRITE_UNAVAILABLE".to_owned(),
                message: "execution write port is unavailable".to_owned(),
            },
            timestamp,
        );
    };
    match port.mutate(&input) {
        Ok(data) => success_response(data, timestamp),
        Err(ExecutionWritePortError::Unavailable(message)) => error_response(
            ErrorSpec {
                status: 503,
                code: "EXECUTION_WRITE_UNAVAILABLE".to_owned(),
                message,
            },
            timestamp,
        ),
        Err(ExecutionWritePortError::Failed {
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
    request_path: &str,
) -> Result<(ExecutionWriteOperation, Option<String>), ErrorSpec> {
    let (path, _) = split_path_query(request_path);
    let exact = |expected: &str| path == expected;
    if method == "POST" && exact(EXECUTION_BUYING_POWER_PATH) {
        return Ok((ExecutionWriteOperation::BuyingPower, None));
    }
    if method == "POST" && exact(EXECUTION_COMBO_PREVIEW_PATH) {
        return Ok((ExecutionWriteOperation::ComboPreview, None));
    }
    if method == "POST" && exact(EXECUTION_COMBO_PLACE_PATH) {
        return Ok((ExecutionWriteOperation::ComboPlace, None));
    }
    if method == "POST" && exact(EXECUTION_ORDER_PLACE_PATH) {
        return Ok((ExecutionWriteOperation::OrderPlace, None));
    }
    if method == "POST" && exact(EXECUTION_ORDER_PREVIEW_PATH) {
        return Ok((ExecutionWriteOperation::OrderPreview, None));
    }
    if method == "POST"
        && let Some(raw_id) = path
            .strip_prefix("/api/v1/execution/combos/")
            .and_then(|suffix| suffix.strip_suffix("/cancel"))
    {
        return parse_order_id(raw_id).map(|id| (ExecutionWriteOperation::ComboCancel, Some(id)));
    }
    if method == "POST"
        && let Some(raw_id) = path
            .strip_prefix("/api/v1/execution/orders/")
            .and_then(|suffix| suffix.strip_suffix("/cancel"))
    {
        return parse_order_id(raw_id).map(|id| (ExecutionWriteOperation::OrderCancel, Some(id)));
    }
    Err(not_found_spec())
}

fn parse_order_id(raw_id: &str) -> Result<String, ErrorSpec> {
    if raw_id.is_empty() || raw_id.contains('/') {
        return Err(not_found_spec());
    }
    if has_invalid_percent_escape(raw_id) {
        return Err(bad_request_spec("internalOrderId is invalid"));
    }
    let decoded = percent_decode_str(raw_id)
        .decode_utf8()
        .map_err(|_| bad_request_spec("internalOrderId is invalid"))?;
    if decoded.contains('/') {
        return Err(bad_request_spec("internalOrderId is invalid"));
    }
    Ok(decoded.trim().to_owned())
}

fn parse_json_object(
    body: Option<&[u8]>,
    operation: ExecutionWriteOperation,
) -> Result<Value, ErrorSpec> {
    let Some(body) = body.filter(|body| !body.is_empty()) else {
        return Err(bad_request_spec(operation.body_error()));
    };
    let mut decoder = Deserializer::from_slice(body);
    let value =
        Value::deserialize(&mut decoder).map_err(|_| bad_request_spec(operation.body_error()))?;
    if value.is_null() {
        return Ok(value);
    }
    let Value::Object(object) = &value else {
        return Err(bad_request_spec(operation.body_error()));
    };
    if !valid_known_field_types(operation, object) {
        return Err(bad_request_spec(operation.body_error()));
    }
    Ok(value)
}

fn valid_known_field_types(
    operation: ExecutionWriteOperation,
    object: &serde_json::Map<String, Value>,
) -> bool {
    let string_fields = match operation {
        ExecutionWriteOperation::BuyingPower => &[
            "brokerId",
            "accountId",
            "tradingEnvironment",
            "market",
            "featureId",
            "orderKind",
            "orderType",
            "session",
        ][..],
        ExecutionWriteOperation::ComboPreview | ExecutionWriteOperation::ComboPlace => &[
            "brokerId",
            "tradingEnvironment",
            "accountId",
            "market",
            "clientOrderId",
            "orderKind",
            "productClass",
            "previewId",
            "rfqId",
            "mvc",
            "underlyingInstrumentId",
            "optionStrategy",
            "nearExpiry",
            "farExpiry",
            "quoteExpiresAt",
        ][..],
        ExecutionWriteOperation::OrderPlace | ExecutionWriteOperation::OrderPreview => &[
            "brokerId",
            "tradingEnvironment",
            "env",
            "accountId",
            "market",
            "code",
            "symbol",
            "side",
            "orderType",
            "timeInForce",
            "session",
            "predictionSide",
            "productClass",
            "orderKind",
            "quantityMode",
            "previewId",
            "rfqId",
            "quoteExpiresAt",
            "clientOrderId",
            "remark",
        ][..],
        ExecutionWriteOperation::ComboCancel | ExecutionWriteOperation::OrderCancel => &[][..],
    };
    if string_fields
        .iter()
        .any(|field| !is_string_or_null(object.get(*field)))
    {
        return false;
    }
    let number_fields = match operation {
        ExecutionWriteOperation::BuyingPower => &["quantity", "amount", "price"][..],
        ExecutionWriteOperation::ComboPreview | ExecutionWriteOperation::ComboPlace => {
            &["spread", "amount", "price"][..]
        }
        ExecutionWriteOperation::OrderPlace | ExecutionWriteOperation::OrderPreview => {
            &["quantity", "price", "stopPrice", "amount"][..]
        }
        ExecutionWriteOperation::ComboCancel | ExecutionWriteOperation::OrderCancel => &[][..],
    };
    if number_fields
        .iter()
        .any(|field| !is_number_or_null(object.get(*field)))
    {
        return false;
    }
    if matches!(
        operation,
        ExecutionWriteOperation::BuyingPower
            | ExecutionWriteOperation::ComboPreview
            | ExecutionWriteOperation::ComboPlace
    ) && !is_object_or_null(object.get("instrument"))
    {
        return operation != ExecutionWriteOperation::BuyingPower
            || !object.contains_key("instrument");
    }
    if matches!(
        operation,
        ExecutionWriteOperation::ComboPreview
            | ExecutionWriteOperation::ComboPlace
            | ExecutionWriteOperation::OrderPlace
            | ExecutionWriteOperation::OrderPreview
            | ExecutionWriteOperation::BuyingPower
    ) && !is_array_of_objects_or_null(object.get("legs"))
    {
        return !object.contains_key("legs");
    }
    true
}

fn is_string_or_null(value: Option<&Value>) -> bool {
    value.is_none_or(|value| value.is_null() || value.is_string())
}

fn is_number_or_null(value: Option<&Value>) -> bool {
    value.is_none_or(|value| value.is_null() || value.is_number())
}

fn is_object_or_null(value: Option<&Value>) -> bool {
    value.is_none_or(|value| value.is_null() || value.is_object())
}

fn is_array_of_objects_or_null(value: Option<&Value>) -> bool {
    value.is_none_or(|value| {
        value.is_null()
            || value
                .as_array()
                .is_some_and(|items| items.iter().all(|item| item.is_null() || item.is_object()))
    })
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

fn split_path_query(path: &str) -> (&str, &str) {
    path.split_once('?').unwrap_or((path, ""))
}

fn success_response(data: Value, timestamp: &str) -> ExecutionWriteResponse {
    ExecutionWriteResponse {
        status: 200,
        headers: json_headers(),
        body: json!({"ok": true, "data": data, "timestamp": timestamp}),
    }
}

fn error_response(spec: ErrorSpec, timestamp: &str) -> ExecutionWriteResponse {
    ExecutionWriteResponse {
        status: spec.status,
        headers: json_headers(),
        body: json!({
            "ok": false,
            "error": {"code": spec.code, "message": spec.message},
            "timestamp": timestamp,
        }),
    }
}

fn bad_request_spec(message: impl Into<String>) -> ErrorSpec {
    ErrorSpec {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: message.into(),
    }
}

fn not_found_spec() -> ErrorSpec {
    ErrorSpec {
        status: 404,
        code: "NOT_FOUND".to_owned(),
        message: "resource not found".to_owned(),
    }
}

fn json_headers() -> BTreeMap<String, String> {
    BTreeMap::from([(
        "Content-Type".to_owned(),
        "application/json; charset=utf-8".to_owned(),
    )])
}
