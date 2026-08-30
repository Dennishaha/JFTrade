//! Go-compatible canonical execution request hashing.
//!
//! Go's execution credentials are SHA-256 digests of JSON produced from
//! concrete structs.  A `serde_json::Map` is backed by a `BTreeMap` in this
//! workspace, so using `Value` for the complete request would reorder fields
//! and make an otherwise identical Rust request incompatible with Go.  The
//! small serializable structs below deliberately follow the Go declaration
//! order (and the Go outer map's lexicographic key order).

use serde::{Serialize, Serializer};
use serde_json::Value;
use sha2::{Digest, Sha256};

use super::execution_order_helpers::failed;
use super::execution_order_parse::ParsedOrder;
use crate::product::product_execution_write_port::ExecutionWritePortError;

/// A JSON number rendered like Go's `encoding/json` for the ordinary order
/// values accepted by the execution API.  In particular, Go emits `1` for a
/// float64 whose value is exactly one, whereas serde's f64 serializer emits
/// `1.0`.
#[derive(Clone, Copy, Debug)]
struct GoNumber(f64);

impl Serialize for GoNumber {
    fn serialize<S>(&self, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: Serializer,
    {
        let value = self.0;
        if value.is_finite() && value.fract() == 0.0 && value.abs() <= i64::MAX as f64 {
            serializer.serialize_i64(value as i64)
        } else {
            serializer.serialize_f64(value)
        }
    }
}

#[derive(Serialize)]
struct CanonicalSingleEnvelope {
    #[serde(rename = "brokerId")]
    broker_id: String,
    legs: Value,
    #[serde(rename = "orderKind")]
    order_kind: String,
    #[serde(rename = "productClass")]
    product_class: String,
    query: CanonicalPlaceOrderQuery,
}

/// Field order mirrors broker.PlaceOrderQuery and its embedded ReadQuery.
#[derive(Serialize)]
struct CanonicalPlaceOrderQuery {
    #[serde(rename = "brokerId", skip_serializing_if = "Option::is_none")]
    broker_id: Option<String>,
    #[serde(rename = "accountId")]
    account_id: String,
    #[serde(rename = "tradingEnvironment")]
    trading_environment: String,
    market: String,
    symbol: String,
    #[serde(rename = "productClass", skip_serializing_if = "Option::is_none")]
    product_class: Option<String>,
    #[serde(rename = "quantityMode", skip_serializing_if = "Option::is_none")]
    quantity_mode: Option<String>,
    side: String,
    #[serde(rename = "orderType")]
    order_type: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    price: Option<GoNumber>,
    #[serde(rename = "stopPrice", skip_serializing_if = "Option::is_none")]
    stop_price: Option<GoNumber>,
    quantity: GoNumber,
    #[serde(skip_serializing_if = "Option::is_none")]
    amount: Option<GoNumber>,
    #[serde(rename = "predictionSide", skip_serializing_if = "Option::is_none")]
    prediction_side: Option<String>,
    #[serde(rename = "timeInForce", skip_serializing_if = "Option::is_none")]
    time_in_force: Option<String>,
    #[serde(rename = "clientOrderId", skip_serializing_if = "Option::is_none")]
    client_order_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    session: Option<String>,
    #[serde(rename = "fillOutsideRTH", skip_serializing_if = "Option::is_none")]
    fill_outside_rth: Option<bool>,
}

#[derive(Serialize)]
struct CanonicalComboIntent {
    #[serde(rename = "brokerId", skip_serializing_if = "String::is_empty")]
    broker_id: String,
    #[serde(rename = "accountId")]
    account_id: String,
    #[serde(rename = "tradingEnvironment")]
    trading_environment: String,
    market: String,
    #[serde(rename = "clientOrderId")]
    client_order_id: String,
    #[serde(rename = "orderKind")]
    order_kind: String,
    #[serde(rename = "productClass")]
    product_class: String,
    #[serde(rename = "previewId")]
    preview_id: String,
    #[serde(rename = "rfqId", skip_serializing_if = "Option::is_none")]
    rfq_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    mvc: Option<String>,
    #[serde(rename = "underlyingInstrumentId", skip_serializing_if = "Option::is_none")]
    underlying_instrument_id: Option<String>,
    #[serde(rename = "optionStrategy", skip_serializing_if = "Option::is_none")]
    option_strategy: Option<String>,
    #[serde(rename = "nearExpiry", skip_serializing_if = "Option::is_none")]
    near_expiry: Option<String>,
    #[serde(rename = "farExpiry", skip_serializing_if = "Option::is_none")]
    far_expiry: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    spread: Option<GoNumber>,
    #[serde(rename = "quoteExpiresAt", skip_serializing_if = "Option::is_none")]
    quote_expires_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    amount: Option<GoNumber>,
    #[serde(skip_serializing_if = "Option::is_none")]
    price: Option<GoNumber>,
    legs: Vec<CanonicalComboLeg>,
}

#[derive(Serialize)]
struct CanonicalComboLeg {
    #[serde(rename = "instrumentId")]
    instrument_id: String,
    #[serde(rename = "productClass")]
    product_class: String,
    side: String,
    ratio: i64,
    #[serde(skip_serializing_if = "Option::is_none")]
    quantity: Option<GoNumber>,
    #[serde(skip_serializing_if = "Option::is_none")]
    amount: Option<GoNumber>,
    #[serde(skip_serializing_if = "Option::is_none")]
    price: Option<GoNumber>,
    #[serde(rename = "predictionSide", skip_serializing_if = "Option::is_none")]
    prediction_side: Option<String>,
}

/// Return the canonical JSON persisted with a newly reserved order.
pub(crate) fn canonical_execution_request(
    payload: &Value,
    parsed: &ParsedOrder,
    _legs: Option<Value>,
) -> Result<String, ExecutionWritePortError> {
    if parsed.order_kind == "option_combo" || parsed.order_kind == "event_parlay" {
        let intent = canonical_combo_intent(payload, parsed)?;
        return serde_json::to_string(&intent).map_err(serialize_error);
    }

    let query = canonical_place_order_query(parsed);
    let envelope = CanonicalSingleEnvelope {
        broker_id: parsed.broker_id.clone(),
        // Go's executionCommandHash always includes the nil legs field.
        legs: Value::Null,
        order_kind: parsed.order_kind.clone(),
        product_class: parsed.product_class.clone(),
        query,
    };
    serde_json::to_string(&envelope).map_err(serialize_error)
}

pub(crate) fn preview_request_hash(
    payload: &Value,
    parsed: &ParsedOrder,
    legs: Option<Value>,
) -> Result<String, ExecutionWritePortError> {
    let canonical = canonical_execution_request(payload, parsed, legs)?;
    Ok(Sha256::digest(canonical.as_bytes())
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect())
}

fn canonical_place_order_query(parsed: &ParsedOrder) -> CanonicalPlaceOrderQuery {
    let market = parsed.market.clone();
    let symbol = parsed.symbol.trim().to_ascii_uppercase();
    let symbol = if symbol.contains('.') {
        symbol
    } else {
        format!("{market}.{symbol}")
    };
    CanonicalPlaceOrderQuery {
        broker_id: (!parsed.broker_id.is_empty()).then(|| parsed.broker_id.clone()),
        account_id: parsed.header.acc_id.to_string(),
        trading_environment: if parsed.header.trd_env == 1 {
            "REAL".to_owned()
        } else {
            "SIMULATE".to_owned()
        },
        market,
        symbol,
        product_class: (!parsed.product_class.is_empty()).then(|| parsed.product_class.clone()),
        quantity_mode: (!parsed.quantity_mode.is_empty()).then(|| parsed.quantity_mode.clone()),
        side: super::execution_order_parse::side_label(parsed.side).to_owned(),
        order_type: super::execution_order_parse::order_type_label(parsed.order_type).to_owned(),
        price: parsed.price.map(GoNumber),
        stop_price: parsed.stop_price.map(GoNumber),
        quantity: GoNumber(parsed.quantity),
        amount: parsed.amount.map(GoNumber),
        prediction_side: parsed.prediction_side.map(|value| {
            if value == 1 {
                "YES".to_owned()
            } else {
                "NO".to_owned()
            }
        }),
        time_in_force: parsed.time_in_force.map(|value| match value {
            0 => "DAY".to_owned(),
            1 => "GTC".to_owned(),
            2 => "IOC".to_owned(),
            3 => "GTD".to_owned(),
            _ => String::new(),
        }),
        client_order_id: parsed
            .client_order_id
            .as_deref()
            .filter(|value| !value.is_empty())
            .map(str::to_owned),
        session: parsed.session.map(|value| match value {
            1 => "RTH".to_owned(),
            2 => "ETH".to_owned(),
            3 => "ALL".to_owned(),
            4 => "OVERNIGHT".to_owned(),
            _ => String::new(),
        }),
        fill_outside_rth: parsed.fill_outside_rth,
    }
}

fn canonical_combo_intent(
    payload: &Value,
    parsed: &ParsedOrder,
) -> Result<CanonicalComboIntent, ExecutionWritePortError> {
    let object = payload
        .as_object()
        .ok_or_else(|| failed(400, "BAD_REQUEST", "combo payload must be an object"))?;
    Ok(CanonicalComboIntent {
        broker_id: parsed.broker_id.clone(),
        account_id: parsed.header.acc_id.to_string(),
        trading_environment: if parsed.header.trd_env == 1 {
            "REAL".to_owned()
        } else {
            "SIMULATE".to_owned()
        },
        market: object
            .get("market")
            .and_then(Value::as_str)
            .map(str::trim)
            .unwrap_or_default()
            .to_ascii_uppercase(),
        client_order_id: required_string(object, "clientOrderId")?,
        order_kind: parsed.order_kind.clone(),
        product_class: parsed.product_class.clone(),
        preview_id: String::new(),
        rfq_id: optional_string(object, "rfqId"),
        mvc: optional_string(object, "mvc"),
        underlying_instrument_id: optional_string(object, "underlyingInstrumentId")
            .map(|value| value.to_ascii_uppercase()),
        option_strategy: optional_string(object, "optionStrategy")
            .map(|value| value.to_ascii_lowercase()),
        near_expiry: optional_string(object, "nearExpiry"),
        far_expiry: optional_string(object, "farExpiry"),
        spread: optional_number(object, "spread")?,
        quote_expires_at: optional_timestamp(object, "quoteExpiresAt")?,
        amount: optional_number(object, "amount")?,
        price: optional_number(object, "price")?,
        legs: canonical_combo_legs(payload, &parsed.product_class)?,
    })
}

fn canonical_combo_legs(
    payload: &Value,
    default_product_class: &str,
) -> Result<Vec<CanonicalComboLeg>, ExecutionWritePortError> {
    let legs = payload
        .get("legs")
        .and_then(Value::as_array)
        .ok_or_else(|| failed(400, "BAD_REQUEST", "combo legs must be an array"))?;
    if legs.is_empty() {
        return Err(failed(400, "BAD_REQUEST", "combo legs must not be empty"));
    }
    legs.iter()
        .enumerate()
        .map(|(index, value)| {
            let object = value.as_object().ok_or_else(|| {
                failed(
                    400,
                    "BAD_REQUEST",
                    format!("combo leg {index} must be an object"),
                )
            })?;
            let instrument_id = ["instrumentId", "symbol", "code"]
                .into_iter()
                .find_map(|key| optional_string(object, key))
                .ok_or_else(|| {
                    failed(
                        400,
                        "BAD_REQUEST",
                        format!("combo leg {index} instrumentId is required"),
                    )
                })?
                .to_ascii_uppercase();
            let product_class = optional_string(object, "productClass")
                .unwrap_or_else(|| default_product_class.to_owned())
                .to_ascii_lowercase();
            let side = match required_string(object, "side")?
                .to_ascii_uppercase()
                .as_str()
            {
                "B" | "BUY" => "BUY".to_owned(),
                "S" | "SELL" => "SELL".to_owned(),
                _ => {
                    return Err(failed(
                        400,
                        "BAD_REQUEST",
                        format!("combo leg {index} has invalid side"),
                    ));
                }
            };
            let ratio = object
                .get("ratio")
                .or_else(|| object.get("qtyRatio"))
                .and_then(|value| {
                    value.as_i64().or_else(|| {
                        value.as_f64().and_then(|value| {
                            (value.is_finite() && value > 0.0 && value.fract() == 0.0)
                                .then_some(value as i64)
                        })
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
            let prediction_side = optional_string(object, "predictionSide")
                .map(|value| value.to_ascii_uppercase());
            if prediction_side
                .as_deref()
                .is_some_and(|value| value != "YES" && value != "NO")
            {
                return Err(failed(
                    400,
                    "BAD_REQUEST",
                    format!("combo leg {index} predictionSide is invalid"),
                ));
            }
            Ok(CanonicalComboLeg {
                instrument_id,
                product_class,
                side,
                ratio,
                quantity: optional_number(object, "quantity")?,
                amount: optional_number(object, "amount")?,
                price: optional_number(object, "price")?,
                prediction_side,
            })
        })
        .collect()
}

fn required_string(
    object: &serde_json::Map<String, Value>,
    key: &str,
) -> Result<String, ExecutionWritePortError> {
    optional_string(object, key).ok_or_else(|| {
        failed(
            400,
            "BAD_REQUEST",
            format!("{key} is required"),
        )
    })
}

fn optional_string(object: &serde_json::Map<String, Value>, key: &str) -> Option<String> {
    object
        .get(key)
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_owned)
}

fn optional_number(
    object: &serde_json::Map<String, Value>,
    key: &str,
) -> Result<Option<GoNumber>, ExecutionWritePortError> {
    let Some(value) = object.get(key) else {
        return Ok(None);
    };
    if value.is_null() {
        return Ok(None);
    }
    let number = value.as_f64().ok_or_else(|| {
        failed(
            400,
            "BAD_REQUEST",
            format!("{key} must be a number"),
        )
    })?;
    if !number.is_finite() {
        return Err(failed(
            400,
            "BAD_REQUEST",
            format!("{key} must be finite"),
        ));
    }
    Ok(Some(GoNumber(number)))
}

fn optional_timestamp(
    object: &serde_json::Map<String, Value>,
    key: &str,
) -> Result<Option<String>, ExecutionWritePortError> {
    let Some(value) = optional_string(object, key) else {
        return Ok(None);
    };
    let parsed = time::OffsetDateTime::parse(
        &value,
        &time::format_description::well_known::Rfc3339,
    )
    .map_err(|error| failed(400, "BAD_REQUEST", format!("{key} is invalid: {error}")))?;
    parsed
        .format(&time::format_description::well_known::Rfc3339)
        .map(Some)
        .map_err(|error| failed(500, "EXECUTION_TIME_ERROR", error.to_string()))
}

fn serialize_error(error: serde_json::Error) -> ExecutionWritePortError {
    failed(500, "EXECUTION_REQUEST_SERIALIZE_FAILED", error.to_string())
}
