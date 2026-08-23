//! Stage 9 test-cutover leaf for the three broker mutation routes.
//!
//! Go remains the only owner of broker sessions, OpenD, pre-trade risk, order
//! state, SQLite, and every trading side effect. This leaf only reproduces the
//! current HTTP boundary and delegates the already-owned result to an
//! explicitly injected test port. It is intentionally not included from
//! `product.rs`; the integration branch owns authenticated test-cutover wiring.

use std::collections::BTreeMap;

use percent_encoding::percent_decode_str;
use serde::Deserialize;
use serde_json::{Deserializer, Value, json};

pub const BROKER_CANCEL_ORDERS_PATH: &str = "/api/v1/brokers/{brokerId}/orders";
pub const BROKER_PLACE_ORDER_PATH: &str = "/api/v1/brokers/{brokerId}/orders";
pub const BROKER_UNLOCK_PATH: &str = "/api/v1/brokers/{brokerId}/unlock";

pub const BROKERS_WRITE_ROUTES: [(&str, &str); 3] = [
    ("DELETE", BROKER_CANCEL_ORDERS_PATH),
    ("POST", BROKER_PLACE_ORDER_PATH),
    ("POST", BROKER_UNLOCK_PATH),
];

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum BrokersWriteOperation {
    CancelOrders,
    PlaceOrder,
    Unlock,
}

#[allow(dead_code)]
impl BrokersWriteOperation {
    pub const fn name(self) -> &'static str {
        match self {
            Self::CancelOrders => "cancel-orders",
            Self::PlaceOrder => "place-order",
            Self::Unlock => "unlock",
        }
    }

    const fn body_type_name(self) -> &'static str {
        match self {
            Self::CancelOrders => "trading.CancelOrdersRequest",
            Self::PlaceOrder => "trading.PlaceOrderRequest",
            Self::Unlock => "trading.UnlockTradeRequest",
        }
    }
}

#[allow(dead_code)]
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub enum BrokersWriteContext {
    #[default]
    Normal,
    Canceled,
    Deadline,
}

#[derive(Clone, Debug, PartialEq)]
pub struct BrokersWriteRequest {
    pub method: String,
    pub path: String,
    pub body: Option<Vec<u8>>,
    pub context: BrokersWriteContext,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct BrokersWriteQuery {
    pub broker_id: String,
    pub account_id: String,
    pub trading_environment: String,
    pub market: String,
}

#[derive(Clone, Debug, PartialEq)]
pub struct BrokersWriteInput {
    pub operation: BrokersWriteOperation,
    pub query: BrokersWriteQuery,
    pub payload: Value,
    pub context: BrokersWriteContext,
}

#[allow(dead_code)]
#[derive(Clone, Debug, Eq, PartialEq)]
pub enum BrokersWritePortError {
    Unavailable(String),
    Failed {
        status: u16,
        code: String,
        message: String,
    },
}

#[derive(Clone, Debug, PartialEq)]
pub struct BrokersWriteResponse {
    pub status: u16,
    pub headers: BTreeMap<String, String>,
    pub body: Value,
}

/// Consumer-owned broker mutation boundary. The adapter behind this port is
/// the only component allowed to call the current Go owner during rehearsal;
/// this leaf has no broker, OpenD, database, ledger, or notification access.
pub trait BrokersWritePort: Send + Sync + std::fmt::Debug {
    fn mutate(&self, input: &BrokersWriteInput) -> Result<Value, BrokersWritePortError>;
}

pub fn brokers_write_routes() -> &'static [(&'static str, &'static str); 3] {
    &BROKERS_WRITE_ROUTES
}

pub fn dispatch_brokers_write(
    request: &BrokersWriteRequest,
    port: Option<&dyn BrokersWritePort>,
    timestamp: &str,
) -> BrokersWriteResponse {
    let (path, raw_query) = split_path_query(&request.path);
    let (operation, raw_broker_id) = match parse_route(&request.method, path) {
        Ok(route) => route,
        Err(error) => return error_response(error, timestamp),
    };
    let query = match parse_query(raw_query, raw_broker_id, operation) {
        Ok(query) => query,
        Err(error) => return error_response(error, timestamp),
    };
    let payload = match parse_body(request.body.as_deref(), operation) {
        Ok(payload) => payload,
        Err(error) => return error_response(error, timestamp),
    };
    let Some(port) = port else {
        return error_response(
            ErrorSpec {
                status: 503,
                code: "BROKERS_WRITE_UNAVAILABLE".to_owned(),
                message: "brokers write port is unavailable".to_owned(),
            },
            timestamp,
        );
    };
    let input = BrokersWriteInput {
        operation,
        query,
        payload,
        context: request.context,
    };
    match port.mutate(&input) {
        Ok(data) => success_response(data, timestamp),
        Err(BrokersWritePortError::Unavailable(message)) => error_response(
            ErrorSpec {
                status: 503,
                code: "BROKERS_WRITE_UNAVAILABLE".to_owned(),
                message,
            },
            timestamp,
        ),
        Err(BrokersWritePortError::Failed {
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

fn parse_route<'a>(
    method: &str,
    path: &'a str,
) -> Result<(BrokersWriteOperation, &'a str), ErrorSpec> {
    let prefix = "/api/v1/brokers/";
    let Some(suffix) = path.strip_prefix(prefix) else {
        return Err(not_found_spec());
    };
    let mut parts = suffix.split('/');
    let raw_broker_id = parts.next().unwrap_or_default();
    let resource = parts.next().unwrap_or_default();
    if raw_broker_id.is_empty() || parts.next().is_some() {
        return Err(not_found_spec());
    }
    match (method, resource) {
        ("DELETE", "orders") => Ok((BrokersWriteOperation::CancelOrders, raw_broker_id)),
        ("POST", "orders") => Ok((BrokersWriteOperation::PlaceOrder, raw_broker_id)),
        ("POST", "unlock") => Ok((BrokersWriteOperation::Unlock, raw_broker_id)),
        _ => Err(not_found_spec()),
    }
}

fn parse_query(
    raw_query: &str,
    raw_broker_id: &str,
    operation: BrokersWriteOperation,
) -> Result<BrokersWriteQuery, ErrorSpec> {
    let broker_id = decode_path_segment(raw_broker_id)
        .map_err(|_| bad_request_spec("invalid broker id"))?
        .trim()
        .to_owned();
    if broker_id.is_empty() {
        return Err(not_found_spec());
    }
    let mut account_id = String::new();
    let mut trading_environment = String::new();
    let mut market = String::new();
    let mut account_id_seen = false;
    let mut trading_environment_seen = false;
    let mut market_seen = false;
    for pair in raw_query.split('&').filter(|pair| !pair.is_empty()) {
        let (raw_name, raw_value) = pair.split_once('=').unwrap_or((pair, ""));
        let name = decode_query_component(raw_name).map_err(|_| query_error())?;
        let value = decode_query_component(raw_value).map_err(|_| query_error())?;
        match name.as_str() {
            "accountId" if !account_id_seen => {
                account_id = value;
                account_id_seen = true;
            }
            "tradingEnvironment" if !trading_environment_seen => {
                trading_environment = value;
                trading_environment_seen = true;
            }
            "market" if !market_seen => {
                market = value;
                market_seen = true;
            }
            _ => {}
        }
    }
    account_id = account_id.trim().to_owned();
    trading_environment = trading_environment.trim().to_owned();
    market = market.trim().to_owned();
    if market.is_empty() {
        market = "HK".to_owned();
    }
    if operation == BrokersWriteOperation::PlaceOrder {
        trading_environment = trading_environment.to_uppercase();
        if trading_environment.is_empty() {
            trading_environment = "SIMULATE".to_owned();
        }
    }
    Ok(BrokersWriteQuery {
        broker_id,
        account_id,
        trading_environment,
        market,
    })
}

fn parse_body(body: Option<&[u8]>, operation: BrokersWriteOperation) -> Result<Value, ErrorSpec> {
    let Some(body) = body.filter(|body| !body.is_empty()) else {
        return Err(body_error(operation, "EOF"));
    };
    let mut decoder = Deserializer::from_slice(body);
    let value = Value::deserialize(&mut decoder).map_err(|error| {
        let message = if error.is_eof() {
            "unexpected EOF".to_owned()
        } else {
            error.to_string()
        };
        body_error(operation, &message)
    })?;
    if value.is_null() {
        return Ok(value);
    }
    let Value::Object(object) = &value else {
        let source = json_source_type(Some(&value), "");
        return Err(body_type_error(operation, &source));
    };
    validate_known_field_types(operation, object)?;
    Ok(value)
}

fn validate_known_field_types(
    operation: BrokersWriteOperation,
    object: &serde_json::Map<String, Value>,
) -> Result<(), ErrorSpec> {
    match operation {
        BrokersWriteOperation::PlaceOrder => {
            for field in [
                "symbol",
                "side",
                "orderType",
                "timeInForce",
                "clientOrderId",
                "remark",
                "session",
            ] {
                if !is_string_or_null(object.get(field)) {
                    return Err(body_field_type_error(
                        operation,
                        field,
                        "string",
                        object.get(field),
                    ));
                }
            }
            for field in ["price", "stopPrice", "quantity"] {
                if !is_number_or_null(object.get(field)) {
                    return Err(body_field_type_error(
                        operation,
                        field,
                        "float64",
                        object.get(field),
                    ));
                }
            }
            if !is_bool_or_null(object.get("fillOutsideRTH")) {
                return Err(body_field_type_error(
                    operation,
                    "fillOutsideRTH",
                    "bool",
                    object.get("fillOutsideRTH"),
                ));
            }
        }
        BrokersWriteOperation::CancelOrders => {
            let Some(orders) = object.get("orders") else {
                return Ok(());
            };
            if orders.is_null() {
                return Ok(());
            }
            let Some(items) = orders.as_array() else {
                return Err(body_field_type_error(
                    operation,
                    "orders",
                    "[]trading.CancelOrderItem",
                    Some(orders),
                ));
            };
            for item in items {
                let Some(item) = item.as_object() else {
                    return Err(body_field_type_error(
                        operation,
                        "orders",
                        "[]trading.CancelOrderItem",
                        Some(item),
                    ));
                };
                if let Some(order_id) = item.get("orderId")
                    && !order_id.is_null()
                    && !(order_id.as_u64().is_some())
                {
                    return Err(body_field_type_error(
                        operation,
                        "orders.orderId",
                        "uint64",
                        Some(order_id),
                    ));
                }
                for field in ["brokerOrderId", "symbol"] {
                    if !is_string_or_null(item.get(field)) {
                        return Err(body_field_type_error(
                            operation,
                            &format!("orders.{field}"),
                            "string",
                            item.get(field),
                        ));
                    }
                }
            }
        }
        BrokersWriteOperation::Unlock => {
            if !is_bool_or_null(object.get("unlock")) {
                return Err(body_field_type_error(
                    operation,
                    "unlock",
                    "bool",
                    object.get("unlock"),
                ));
            }
            if !is_string_or_null(object.get("passwordMd5")) {
                return Err(body_field_type_error(
                    operation,
                    "passwordMd5",
                    "string",
                    object.get("passwordMd5"),
                ));
            }
        }
    }
    Ok(())
}

fn is_string_or_null(value: Option<&Value>) -> bool {
    value.is_none_or(|value| value.is_null() || value.is_string())
}

fn is_number_or_null(value: Option<&Value>) -> bool {
    value.is_none_or(|value| value.is_null() || value.is_number())
}

fn is_bool_or_null(value: Option<&Value>) -> bool {
    value.is_none_or(|value| value.is_null() || value.is_boolean())
}

fn body_error(_operation: BrokersWriteOperation, detail: &str) -> ErrorSpec {
    ErrorSpec {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: format!("invalid request body: {detail}"),
    }
}

fn body_type_error(operation: BrokersWriteOperation, source: &str) -> ErrorSpec {
    body_error(
        operation,
        &format!(
            "json: cannot unmarshal {source} into Go value of type {}",
            operation.body_type_name()
        ),
    )
}

fn body_field_type_error(
    operation: BrokersWriteOperation,
    field: &str,
    target: &str,
    value: Option<&Value>,
) -> ErrorSpec {
    let type_name = match (operation, field.starts_with("orders.")) {
        (BrokersWriteOperation::CancelOrders, true) => "CancelOrderItem",
        (BrokersWriteOperation::CancelOrders, false) => "CancelOrdersRequest",
        (BrokersWriteOperation::PlaceOrder, _) => "PlaceOrderRequest",
        (BrokersWriteOperation::Unlock, _) => "UnlockTradeRequest",
    };
    let source = json_source_type(value, target);
    body_error(
        operation,
        &format!(
            "json: cannot unmarshal {source} into Go struct field {type_name}.{field} of type {target}"
        ),
    )
}

fn json_source_type(value: Option<&Value>, target: &str) -> String {
    let Some(value) = value else {
        return "null".to_owned();
    };
    if value.is_string() {
        return "string".to_owned();
    }
    if value.is_number() {
        if target == "uint64" {
            return format!("number {value}");
        }
        return "number".to_owned();
    }
    if value.is_boolean() {
        return "bool".to_owned();
    }
    if value.is_array() {
        return "array".to_owned();
    }
    if value.is_object() {
        return "object".to_owned();
    }
    "null".to_owned()
}

fn query_error() -> ErrorSpec {
    bad_request_spec("invalid broker write query")
}

fn decode_path_segment(value: &str) -> Result<String, ()> {
    if has_invalid_percent_escape(value) {
        return Err(());
    }
    percent_decode_str(value)
        .decode_utf8()
        .map(|value| value.into_owned())
        .map_err(|_| ())
}

fn decode_query_component(value: &str) -> Result<String, ()> {
    let value = value.replace('+', " ");
    decode_path_segment(&value)
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

fn success_response(data: Value, timestamp: &str) -> BrokersWriteResponse {
    BrokersWriteResponse {
        status: 200,
        headers: json_headers(),
        body: json!({"ok": true, "data": data, "timestamp": timestamp}),
    }
}

fn error_response(error: ErrorSpec, timestamp: &str) -> BrokersWriteResponse {
    BrokersWriteResponse {
        status: error.status,
        headers: json_headers(),
        body: json!({
            "ok": false,
            "error": {"code": error.code, "message": error.message},
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
