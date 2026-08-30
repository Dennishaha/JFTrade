//! Helpers for production execution adapters.

use std::collections::BTreeSet;
use std::sync::{Arc, Mutex};
use jftrade_store_sqlite::{ExecutionOrderStoreError, StoredExecutionOrder};
use jftrade_integration_futu::TradeSessionError;
use serde_json::{Value, json};
use sha2::{Digest, Sha256};
use super::execution_order_parse::ParsedOrder;
use crate::product::product_brokers_write_port::BrokersWritePortError;
use crate::product::product_execution_write_port::ExecutionWritePortError;

pub(crate) fn header_from_order(
    order: &StoredExecutionOrder,
) -> Result<jftrade_integration_futu::TradeHeader, ExecutionWritePortError> {
    let account_id = order
        .account_id
        .parse::<u64>()
        .map_err(|_| failed(400, "BAD_REQUEST", "stored accountId is not numeric"))?;
    Ok(jftrade_integration_futu::TradeHeader {
        trd_env: i32::from(order.trading_environment.eq_ignore_ascii_case("REAL")),
        acc_id: account_id,
        trd_market: super::execution_order_parse::trade_market(&order.market),
        jp_acc_type: None,
    })
}

pub(crate) fn merge_query(
    payload: &Value,
    query: &crate::product::product_brokers_write_port::BrokersWriteQuery,
) -> Value {
    let mut object = payload.as_object().cloned().unwrap_or_default();
    object
        .entry("brokerId".to_owned())
        .or_insert_with(|| Value::String(query.broker_id.clone()));
    object
        .entry("accountId".to_owned())
        .or_insert_with(|| Value::String(query.account_id.clone()));
    object
        .entry("tradingEnvironment".to_owned())
        .or_insert_with(|| Value::String(query.trading_environment.clone()));
    object
        .entry("market".to_owned())
        .or_insert_with(|| Value::String(query.market.clone()));
    Value::Object(object)
}

pub(crate) fn order_value(
    order: &StoredExecutionOrder,
) -> Result<Value, ExecutionWritePortError> {
    let mut value = json!({
        "internalOrderId": order.internal_order_id,
        "brokerId": order.broker_id,
        "brokerOrderId": order.broker_order_id,
        "brokerOrderIdEx": order.broker_order_id_ex,
        "source": order.source,
        "sourceDetail": order.source_detail,
        "tradingEnvironment": order.trading_environment,
        "accountId": order.account_id,
        "market": order.market,
        "orderKind": order.order_kind,
        "productClass": order.product_class,
        "quantityMode": order.quantity_mode,
        "clientOrderId": order.client_order_id,
        "previewId": order.preview_id,
        "status": order.status,
        "rawBrokerStatus": order.raw_broker_status,
        "symbol": order.symbol,
        "side": order.side,
        "orderType": order.order_type,
        "requestedQuantity": order.requested_quantity,
        "requestedPrice": order.requested_price,
        "filledQuantity": order.filled_quantity,
        "filledAveragePrice": order.filled_average_price,
        "requestedAmount": order.requested_amount,
        "fees": order.fees,
        "payout": order.payout,
        "remark": order.remark,
        "lastError": order.last_error,
        "lastErrorCode": order.last_error_code,
        "lastErrorSource": order.last_error_source,
        "submittedAt": order.submitted_at,
        "createdAt": order.created_at,
        "updatedAt": order.updated_at,
    });
    if order.normalized_request.trim().is_empty() {
        if let Some(object) = value.as_object_mut() {
            object.remove("normalizedRequest");
        }
    } else if let Some(object) = value.as_object_mut() {
        object.insert(
            "normalizedRequest".to_owned(),
            Value::String(order.normalized_request.clone()),
        );
    }
    if matches!(order.order_kind.trim().to_ascii_lowercase().as_str(), "option_combo" | "event_parlay") {
        let legs = normalized_legs_from_request(&order.normalized_request, order)?;
        if let Some(object) = value.as_object_mut() {
            object.insert("legs".to_owned(), Value::Array(legs));
        }
    }
    Ok(value)
}

/// Project persisted combo legs into the public ExecutionOrderLeg wire shape.
/// The execution schema stores the normalized request as JSON; decoding it at
/// read time preserves all optional leg fields without introducing a second
/// SQLite table or synthetic defaults.
fn normalized_legs_from_request(
    raw: &str,
    order: &StoredExecutionOrder,
) -> Result<Vec<Value>, ExecutionWritePortError> {
    let value: Value = serde_json::from_str(raw).map_err(|error| {
        failed(
            500,
            "EXECUTION_ORDER_DATA_INVALID",
            format!("stored normalized request is invalid JSON: {error}"),
        )
    })?;
    let legs = value
        .get("legs")
        .and_then(Value::as_array)
        .ok_or_else(|| {
            failed(
                500,
                "EXECUTION_ORDER_DATA_INVALID",
                "stored combo order is missing legs",
            )
        })?;
    if legs.len() < 2 {
        return Err(failed(
            500,
            "EXECUTION_ORDER_DATA_INVALID",
            "stored combo order must contain at least two legs",
        ));
    }
    let mut projected_legs = Vec::with_capacity(legs.len());
    for (index, leg) in legs.iter().enumerate() {
        let object = leg.as_object().ok_or_else(|| {
            failed(
                500,
                "EXECUTION_ORDER_DATA_INVALID",
                format!("stored combo leg {index} must be an object"),
            )
        })?;
        let instrument = object
            .get("instrumentId")
            .or_else(|| object.get("symbol"))
            .or_else(|| object.get("code"))
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| {
                failed(
                    500,
                    "EXECUTION_ORDER_DATA_INVALID",
                    format!("stored combo leg {index} is missing instrumentId"),
                )
            })?;
        let product_class = object
            .get("productClass")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| {
                failed(
                    500,
                    "EXECUTION_ORDER_DATA_INVALID",
                    format!("stored combo leg {index} is missing productClass"),
                )
            })?;
        let side = object
            .get("side")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| {
                failed(
                    500,
                    "EXECUTION_ORDER_DATA_INVALID",
                    format!("stored combo leg {index} is missing side"),
                )
            })?
            .to_ascii_uppercase();
        if !matches!(side.as_str(), "BUY" | "B" | "SELL" | "S") {
            return Err(failed(
                500,
                "EXECUTION_ORDER_DATA_INVALID",
                format!("stored combo leg {index} has invalid side"),
            ));
        }
        let ratio_value = object
            .get("ratio")
            .or_else(|| object.get("qtyRatio"))
            .ok_or_else(|| {
                failed(
                    500,
                    "EXECUTION_ORDER_DATA_INVALID",
                    format!("stored combo leg {index} is missing ratio"),
                )
            })?;
        let ratio = ratio_value
            .as_i64()
            .or_else(|| {
                ratio_value.as_f64().and_then(|value| {
                    (value.is_finite() && value > 0.0 && value.fract() == 0.0)
                        .then_some(value as i64)
                })
            })
            .filter(|value| *value > 0)
            .ok_or_else(|| {
                failed(
                    500,
                    "EXECUTION_ORDER_DATA_INVALID",
                    format!("stored combo leg {index} has invalid ratio"),
                )
            })?;
            let id = format!("{}-leg-{:03}", order.internal_order_id, index + 1);
            let mut projected = json!({
                "id": id,
                "internalOrderId": order.internal_order_id,
                "index": index,
                "instrumentId": instrument,
                "productClass": product_class,
                "side": side,
                "ratio": ratio,
                "status": order.status,
                "filledQuantity": Value::Null,
                "filledAmount": Value::Null,
                "averagePrice": Value::Null,
                "fees": Value::Null,
                "payout": Value::Null,
                "updatedAt": order.updated_at,
                "createdAt": order.created_at,
            });
            if let Some(value) = object.get("predictionSide")
                && !value.is_null()
            {
                projected["predictionSide"] = value.clone();
            }
            for (from, to) in [
                ("quantity", "requestedQuantity"),
                ("amount", "requestedAmount"),
                ("price", "requestedPrice"),
            ] {
                if let Some(value) = object.get(from) {
                    projected[to] = value.clone();
                }
            }
            projected_legs.push(projected);
    }
    Ok(projected_legs)
}

pub(crate) fn value_identifier(value: Option<&Value>) -> Option<String> {
    match value {
        Some(Value::String(value)) => {
            let value = value.trim();
            (!value.is_empty()).then(|| value.to_owned())
        }
        Some(Value::Number(value)) => value
            .as_u64()
            .map(|value| value.to_string())
            .or_else(|| value.as_i64().filter(|value| *value >= 0).map(|value| value.to_string())),
        _ => None,
    }
}

pub(crate) struct CancelInFlightGuard {
    ids: Arc<Mutex<BTreeSet<String>>>,
    id: String,
}

impl CancelInFlightGuard {
    pub(crate) fn acquire(ids: Arc<Mutex<BTreeSet<String>>>, id: &str) -> Result<Self, ExecutionWritePortError> {
        let mut guard = ids
            .lock()
            .map_err(|_| failed(500, "EXECUTION_LOCK_ERROR", "cancel lock is poisoned"))?;
        if !guard.insert(id.to_owned()) {
            return Err(failed(
                409,
                "EXECUTION_ORDER_CANCEL_IN_PROGRESS",
                "execution order cancellation is already in progress",
            ));
        }
        drop(guard);
        Ok(Self {
            ids,
            id: id.to_owned(),
        })
    }
}

impl Drop for CancelInFlightGuard {
    fn drop(&mut self) {
        if let Ok(mut ids) = self.ids.lock() {
            ids.remove(&self.id);
        }
    }
}

pub(crate) fn parse_preview_order(
    object: &serde_json::Map<String, Value>,
) -> Result<ParsedOrder, ExecutionWritePortError> {
    let mut normalized = object.clone();
    if !normalized.contains_key("symbol")
        && !normalized.contains_key("code")
        && let Some(instrument_id) = normalized
            .get("instrument")
            .and_then(Value::as_object)
            .and_then(|instrument| instrument.get("instrumentId"))
    {
        normalized.insert("symbol".to_owned(), instrument_id.clone());
    }
    if !normalized.contains_key("quantity") {
        normalized.insert("quantity".to_owned(), json!(1.0));
    }
    let value = Value::Object(normalized);
    super::execution_order_parse::parse_order(&value).map_err(|message| failed(400, "BAD_REQUEST", message))
}

#[derive(Clone, Debug)]
pub(crate) struct ProductRuleRequest {
    pub(crate) product_class: String,
    pub(crate) order_kind: String,
    pub(crate) order_type: String,
    pub(crate) market: String,
    pub(crate) quantity: Option<f64>,
    pub(crate) amount: Option<f64>,
    pub(crate) price: Option<f64>,
    pub(crate) session: Option<String>,
}

/// Decode the broker.ProductRuleQuery wire shape.  Its instrument is nested
/// and account identifiers are opaque strings; unlike a trade command this
/// query does not need to coerce accountId into the numeric OpenD header.
pub(crate) fn parse_product_rule_request(
    payload: &Value,
) -> Result<ProductRuleRequest, ExecutionWritePortError> {
    let object = payload
        .as_object()
        .ok_or_else(|| failed(400, "BAD_REQUEST", "invalid buying-power request"))?;
    let instrument = object.get("instrument").and_then(Value::as_object);
    let product_class = instrument
        .and_then(|value| value.get("productClass"))
        .and_then(Value::as_str)
        .or_else(|| object.get("productClass").and_then(Value::as_str))
        .unwrap_or("equity")
        .trim()
        .to_ascii_lowercase();
    let order_kind = object
        .get("orderKind")
        .and_then(Value::as_str)
        .unwrap_or("single")
        .trim()
        .to_ascii_lowercase();
    let order_type = object
        .get("orderType")
        .and_then(Value::as_str)
        .unwrap_or("LIMIT")
        .trim()
        .to_ascii_uppercase();
    let market = instrument
        .and_then(|value| value.get("tradeMarket").or_else(|| value.get("quoteMarket")))
        .and_then(Value::as_str)
        .or_else(|| object.get("market").and_then(Value::as_str))
        .unwrap_or("US")
        .trim()
        .to_ascii_uppercase();
    Ok(ProductRuleRequest {
        product_class,
        order_kind,
        order_type,
        market,
        quantity: object.get("quantity").and_then(Value::as_f64),
        amount: object.get("amount").and_then(Value::as_f64),
        price: object.get("price").and_then(Value::as_f64),
        session: object
            .get("session")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .map(str::to_owned),
    })
}

pub(crate) fn product_rule_rejection(request: &ProductRuleRequest) -> Option<(&'static str, &'static str)> {
    if request.order_kind == "event_single"
        && request.product_class != "event_contract"
    {
        return Some((
            "PRODUCT_MISMATCH",
            "event order requires an event-contract instrument",
        ));
    }
    if request.order_kind == "event_single" {
        if request.market != "US" {
            return Some((
                "MARKET_MISMATCH",
                "prediction contracts trade in the US market",
            ));
        }
        if request.amount.is_none_or(|value| !value.is_finite() || value <= 0.0) {
            return Some((
                "INVALID_AMOUNT",
                "event-contract amount must be positive",
            ));
        }
        if request
            .price
            .is_none_or(|value| !value.is_finite() || !(0.01..=0.99).contains(&value))
        {
            return Some((
                "INVALID_PRICE",
                "event-contract price must be between 0.01 and 0.99",
            ));
        }
        if request.order_type != "LIMIT" {
            return Some((
                "INVALID_ORDER_TYPE",
                "event-contract orders require LIMIT order type",
            ));
        }
        // No typed active-contract reader is installed in the execution
        // production port yet.  Refuse the request explicitly instead of
        // claiming that an arbitrary prediction instrument is tradable.
        return Some((
            "EVENT_NOT_TRADABLE",
            "active event-contract validation is unavailable",
        ));
    }
    if matches!(request.product_class.as_str(), "option" | "future")
        && request
            .quantity
            .is_none_or(|value| !value.is_finite() || value <= 0.0 || value.fract() != 0.0)
    {
        return Some((
            "INVALID_CONTRACT_QUANTITY",
            "derivative quantity must be a positive integer",
        ));
    }
    if request.product_class == "option" && request.session.is_some() {
        return Some((
            "INVALID_SESSION",
            "option orders do not inherit stock extended-hours sessions",
        ));
    }
    None
}

/// Construct the normalized command representation used by Go's execution
/// hash.  Preview and place requests can differ in presentation fields or in
/// alias casing; hashing this canonical form keeps the credential bound to
/// the same broker/account/order command instead of the raw HTTP JSON.
pub(crate) fn canonical_execution_request(
    payload: &Value,
    parsed: &ParsedOrder,
    legs: Option<Value>,
) -> Result<Value, ExecutionWritePortError> {
    let mut outer = serde_json::Map::new();
    outer.insert("brokerId".to_owned(), Value::String(parsed.broker_id.clone()));
    outer.insert("orderKind".to_owned(), Value::String(parsed.order_kind.clone()));
    outer.insert("productClass".to_owned(), Value::String(parsed.product_class.clone()));
    outer.insert("query".to_owned(), canonical_query_value(parsed));
    let legs = match legs {
        Some(value) => validate_canonical_legs(value)?,
        None => canonical_legs_from_payload(payload)?,
    };
    outer.insert("legs".to_owned(), legs);
    Ok(Value::Object(outer))
}

fn canonical_query_value(parsed: &ParsedOrder) -> Value {
    let mut query = serde_json::Map::new();
    query.insert("brokerId".to_owned(), Value::String(parsed.broker_id.clone()));
    query.insert("accountId".to_owned(), Value::String(parsed.header.acc_id.to_string()));
    query.insert(
        "tradingEnvironment".to_owned(),
        Value::String(if parsed.header.trd_env == 1 { "REAL" } else { "SIMULATE" }.to_owned()),
    );
    query.insert("market".to_owned(), Value::String(super::execution_order_parse::market_label(parsed.header.trd_market)));
    query.insert("symbol".to_owned(), Value::String(parsed.symbol.clone()));
    query.insert("productClass".to_owned(), Value::String(parsed.product_class.clone()));
    query.insert("quantityMode".to_owned(), Value::String(parsed.quantity_mode.clone()));
    query.insert("side".to_owned(), Value::String(super::execution_order_parse::side_label(parsed.side).to_owned()));
    query.insert("orderType".to_owned(), Value::String(super::execution_order_parse::order_type_label(parsed.order_type).to_owned()));
    query.insert("quantity".to_owned(), json!(parsed.quantity));
    if let Some(value) = parsed.price { query.insert("price".to_owned(), json!(value)); }
    if let Some(value) = parsed.amount { query.insert("amount".to_owned(), json!(value)); }
    if let Some(value) = parsed.stop_price { query.insert("stopPrice".to_owned(), json!(value)); }
    if let Some(value) = parsed.prediction_side {
        query.insert("predictionSide".to_owned(), Value::String(if value == 1 { "YES" } else { "NO" }.to_owned()));
    }
    if let Some(value) = parsed.time_in_force {
        query.insert("timeInForce".to_owned(), Value::String(match value { 0 => "DAY", 1 => "GTC", 2 => "IOC", 3 => "GTD", _ => "" }.to_owned()));
    }
    if let Some(value) = parsed.client_order_id.as_deref().filter(|value| !value.is_empty()) {
        query.insert("clientOrderId".to_owned(), Value::String(value.to_owned()));
    }
    if let Some(value) = parsed.remark.as_deref().or(parsed.client_order_id.as_deref()).filter(|value| !value.is_empty()) {
        query.insert("remark".to_owned(), Value::String(value.to_owned()));
    }
    if let Some(value) = parsed.session {
        query.insert("session".to_owned(), Value::String(match value { 0 => "RTH", 1 => "ETH", 2 => "OVERNIGHT", _ => "" }.to_owned()));
    }
    if let Some(value) = parsed.fill_outside_rth { query.insert("fillOutsideRTH".to_owned(), Value::Bool(value)); }
    Value::Object(query)
}

fn canonical_legs_from_payload(payload: &Value) -> Result<Value, ExecutionWritePortError> {
    let Some(value) = payload.get("legs") else {
        return Ok(Value::Null);
    };
    let legs = value.as_array().ok_or_else(|| {
        failed(
            400,
            "BAD_REQUEST",
            "combo legs must be an array",
        )
    })?;
    if legs.is_empty() {
        return Err(failed(
            400,
            "BAD_REQUEST",
            "combo legs must not be empty",
        ));
    }
    let mut canonical = Vec::with_capacity(legs.len());
    for (index, value) in legs.iter().enumerate() {
        let object = value.as_object().ok_or_else(|| {
            failed(
                400,
                "BAD_REQUEST",
                format!("combo leg {index} must be an object"),
            )
        })?;
        let instrument = object
            .get("instrumentId")
            .or_else(|| object.get("symbol"))
            .or_else(|| object.get("code"))
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| {
                failed(
                    400,
                    "BAD_REQUEST",
                    format!("combo leg {index} instrumentId is required"),
                )
            })?;
        let side = object
            .get("side")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| {
                failed(
                    400,
                    "BAD_REQUEST",
                    format!("combo leg {index} side is required"),
                )
            })?
            .to_ascii_uppercase();
        if !matches!(side.as_str(), "BUY" | "B" | "SELL" | "S") {
            return Err(failed(
                400,
                "BAD_REQUEST",
                format!("combo leg {index} has invalid side"),
            ));
        }
        let ratio_value = object
            .get("ratio")
            .or_else(|| object.get("qtyRatio"))
            .ok_or_else(|| {
                failed(
                    400,
                    "BAD_REQUEST",
                    format!("combo leg {index} ratio is required"),
                )
            })?;
        let ratio = ratio_value
            .as_i64()
            .or_else(|| {
                ratio_value.as_f64().and_then(|value| {
                    (value.is_finite() && value > 0.0 && value.fract() == 0.0)
                        .then_some(value as i64)
                })
            })
            .filter(|value| *value > 0)
            .ok_or_else(|| {
                failed(
                    400,
                    "BAD_REQUEST",
                    format!("combo leg {index} ratio must be a positive integer"),
                )
            })?;
        let product_class = object
            .get("productClass")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| {
                failed(
                    400,
                    "BAD_REQUEST",
                    format!("combo leg {index} productClass is required"),
                )
            })?;
        let mut leg = serde_json::Map::new();
        leg.insert(
            "instrumentId".to_owned(),
            Value::String(instrument.to_ascii_uppercase()),
        );
        leg.insert(
            "productClass".to_owned(),
            Value::String(product_class.to_owned()),
        );
        leg.insert("side".to_owned(), Value::String(side));
        leg.insert("ratio".to_owned(), json!(ratio));
        for key in ["quantity", "amount", "price"] {
            if let Some(value) = object.get(key) {
                leg.insert(key.to_owned(), value.clone());
            }
        }
        if let Some(value) = object.get("predictionSide") {
            let side = value.as_str().map(str::trim).filter(|value| !value.is_empty()).ok_or_else(|| {
                failed(400, "BAD_REQUEST", format!("combo leg {index} predictionSide is invalid"))
            })?;
            if !matches!(side.to_ascii_uppercase().as_str(), "YES" | "NO") {
                return Err(failed(400, "BAD_REQUEST", format!("combo leg {index} predictionSide is invalid")));
            }
            leg.insert("predictionSide".to_owned(), Value::String(side.to_ascii_uppercase()));
        }
        canonical.push(Value::Object(leg));
    }
    Ok(Value::Array(canonical))
}

fn validate_canonical_legs(value: Value) -> Result<Value, ExecutionWritePortError> {
    let legs = value.as_array().ok_or_else(|| {
        failed(500, "EXECUTION_ORDER_DATA_INVALID", "canonical combo legs must be an array")
    })?;
    if legs.is_empty() {
        return Err(failed(500, "EXECUTION_ORDER_DATA_INVALID", "canonical combo legs must not be empty"));
    }
    for (index, leg) in legs.iter().enumerate() {
        let object = leg.as_object().ok_or_else(|| {
            failed(500, "EXECUTION_ORDER_DATA_INVALID", format!("canonical combo leg {index} must be an object"))
        })?;
        for field in ["instrumentId", "productClass", "side", "ratio"] {
            let valid = match field {
                "ratio" => object.get(field).and_then(Value::as_i64).is_some_and(|value| value > 0),
                _ => object.get(field).and_then(Value::as_str).is_some_and(|value| !value.trim().is_empty()),
            };
            if !valid {
                return Err(failed(500, "EXECUTION_ORDER_DATA_INVALID", format!("canonical combo leg {index} is missing or invalid {field}")));
            }
        }
    }
    Ok(value)
}

pub(crate) fn preview_request_hash(
    payload: &Value,
    parsed: &ParsedOrder,
    legs: Option<Value>,
) -> Result<String, ExecutionWritePortError> {
    let request = canonical_execution_request(payload, parsed, legs)?;
    let bytes = serde_json::to_vec(&request).map_err(|error| {
        failed(500, "EXECUTION_REQUEST_SERIALIZE_FAILED", error.to_string())
    })?;
    Ok(Sha256::digest(bytes)
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect())
}

pub(crate) fn is_terminal_status(status: &str) -> bool {
    matches!(
        status.trim().to_ascii_uppercase().as_str(),
        "FILLED" | "CANCELLED" | "CANCELED" | "REJECTED" | "EXPIRED" | "FAILED" | "PRECHECK_REJECTED"
    )
}

pub(crate) fn store_error(error: impl std::fmt::Display) -> ExecutionWritePortError {
    failed(500, "EXECUTION_STORE_ERROR", error.to_string())
}

pub(crate) fn map_transition_store_error(error: ExecutionOrderStoreError) -> ExecutionWritePortError {
    match error {
        ExecutionOrderStoreError::Conflict(message) => {
            failed(409, "EXECUTION_ORDER_CONFLICT", message)
        }
        ExecutionOrderStoreError::NotFound(id) => failed(
            404,
            "EXECUTION_ORDER_NOT_FOUND",
            format!("execution order not found: {id}"),
        ),
        other => store_error(other),
    }
}

pub(crate) fn failed(
    status: u16,
    code: impl Into<String>,
    message: impl Into<String>,
) -> ExecutionWritePortError {
    ExecutionWritePortError::Failed {
        status,
        code: code.into(),
        message: message.into(),
    }
}

pub(crate) fn broker_failed(
    status: u16,
    code: impl Into<String>,
    message: impl Into<String>,
) -> BrokersWritePortError {
    BrokersWritePortError::Failed {
        status,
        code: code.into(),
        message: message.into(),
    }
}

pub(crate) fn broker_error(error: ExecutionWritePortError) -> BrokersWritePortError {
    match error {
        ExecutionWritePortError::Unavailable(message) => {
            BrokersWritePortError::Unavailable(message)
        }
        ExecutionWritePortError::Failed {
            status,
            code,
            message,
        } => BrokersWritePortError::Failed {
            status,
            code,
            message,
        },
    }
}

pub(crate) fn execution_error_details(error: &ExecutionWritePortError) -> (String, Option<String>) {
    match error {
        ExecutionWritePortError::Unavailable(message) => (message.clone(), None),
        ExecutionWritePortError::Failed {
            code, message, ..
        } => (message.clone(), Some(code.clone())),
    }
}

pub(crate) fn map_trade_error(error: TradeSessionError) -> ExecutionWritePortError {
    let message = error.to_string();
    let lower = message.to_ascii_lowercase();
    if lower.contains("timeout") || lower.contains("timed out") {
        failed(504, "BROKER_TIMEOUT", message)
    } else if lower.contains("rate") || lower.contains("quota") {
        failed(429, "BROKER_RATE_LIMITED", message)
    } else {
        failed(502, "BROKER_UNAVAILABLE", message)
    }
}
