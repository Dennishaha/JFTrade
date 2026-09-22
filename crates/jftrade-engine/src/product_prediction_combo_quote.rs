//! Go-compatible prediction Parlay RFQ ownership.
//!
//! Go's `productfeatures.Service.QuotePredictionCombo` is the single owner of
//! the RFQ request contract: it normalizes broker/account/environment/MVC,
//! rejects a request with fewer than two valid event-contract legs, forwards
//! the normalized legs to the adapter, then applies the server-side 30 second
//! expiry policy and persists the quote through `PredictionQuoteStore` before
//! the caller ever sees it. Keeping that sequence here — rather than in the
//! HTTP handler or the MCP executor — gives both transports one validation,
//! one legs hash and one durable write.

use serde::{Serialize, Serializer};
use serde_json::{Map, Value, json};
use sha2::{Digest, Sha256};

use super::ProductionMarketDataProviderActionsPort;
use crate::product::product_market_data_provider_actions_port::MarketDataProviderActionsPortError;
use super::super::product_production_ports_market_data_prediction::{
    PREDICTION_INELIGIBLE_CODE, PREDICTION_INELIGIBLE_MESSAGE,
};

/// Receive clock for a freshly issued RFQ.
pub(super) type PredictionQuoteClock = std::sync::Arc<dyn Fn() -> String + Send + Sync>;
/// The service-owned 30 second RFQ window derived from the receive instant.
pub(super) type PredictionQuoteExpiry =
    std::sync::Arc<dyn Fn(&str) -> Result<String, String> + Send + Sync>;

/// Go hard-codes a 30 second server-side RFQ window regardless of what the
/// adapter reports, and records that policy in `expirySource`.
const PREDICTION_QUOTE_TTL_SECONDS: i64 = 30;
const PREDICTION_QUOTE_EXPIRY_SOURCE: &str = "jftrade_policy";
const PREDICTION_QUOTE_WARNING: &str =
    "RFQ 有效期由 JFTrade 服务端接收时间起固定为 30 秒。";
/// Go's `ErrInvalidQuery` prefix; the HTTP edge maps this sentinel to
/// `400 BAD_REQUEST`, so the wording is part of the wire contract.
const INVALID_QUERY_PREFIX: &str = "invalid product feature query";

/// One normalized leg. Field order mirrors Go's `broker.OrderLegIntent` so the
/// legs hash stays byte-identical to `broker.PredictionQuoteLegsHash`.
#[derive(Clone, Debug, Serialize)]
pub(super) struct CanonicalLeg {
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

#[derive(Debug, Serialize)]
struct CanonicalQuoteBinding {
    mvc: String,
    legs: Vec<CanonicalLeg>,
}

/// A JSON number rendered the way Go's `encoding/json` renders a struct
/// `float64`: `1` for an exact integer, otherwise `1.5`.
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

/// The normalized request shared by the adapter call and the quote record.
#[derive(Clone, Debug)]
pub(super) struct PredictionComboQuoteRequest {
    pub(super) broker_id: String,
    pub(super) account_id: String,
    pub(super) trading_environment: String,
    pub(super) mvc: String,
    pub(super) legs: Vec<CanonicalLeg>,
}

impl PredictionComboQuoteRequest {
    /// Go `Service.QuotePredictionCombo` validation and normalization.
    ///
    /// Body fields win over the route query, exactly like
    /// `handlePredictionComboQuote` back-filling `brokerId`, `accountId` and
    /// `tradingEnvironment` from the query string before the service runs.
    pub(super) fn parse(payload: &Value, query: &str) -> Result<Self, String> {
        let object = payload
            .as_object()
            .ok_or_else(|| invalid("prediction combo quote payload must be an object"))?;
        let query = QueryValues::parse(query);
        let broker_id = text(object, "brokerId")
            .or_else(|| query.get("brokerId"))
            .unwrap_or_default();
        let account_id = text(object, "accountId")
            .or_else(|| query.get("accountId"))
            .unwrap_or_default();
        let trading_environment = text(object, "tradingEnvironment")
            .or_else(|| query.get("tradingEnvironment"))
            .unwrap_or_default()
            .to_ascii_uppercase();
        let mvc = text(object, "mvc").unwrap_or_default();
        if broker_id.is_empty()
            || account_id.is_empty()
            || trading_environment.is_empty()
            || mvc.is_empty()
        {
            return Err(invalid(
                "brokerId, accountId, tradingEnvironment, and mvc are required",
            ));
        }
        let legs = object
            .get("legs")
            .and_then(Value::as_array)
            .filter(|legs| legs.len() >= 2)
            .ok_or_else(|| invalid("prediction combo quote requires at least two legs"))?;
        let mut normalized = Vec::with_capacity(legs.len());
        for (index, leg) in legs.iter().enumerate() {
            normalized.push(normalize_leg(leg, index)?);
        }
        Ok(Self {
            broker_id,
            account_id,
            trading_environment,
            mvc,
            legs: normalized,
        })
    }

    /// Go `broker.PredictionQuoteLegsHash`: `sha256(JSON{mvc, legs})` in hex.
    pub(super) fn legs_hash(&self) -> String {
        prediction_quote_binding_hash(&self.mvc, &self.legs)
    }
}

fn prediction_quote_binding_hash(mvc: &str, legs: &[CanonicalLeg]) -> String {
    let binding = CanonicalQuoteBinding {
        mvc: mvc.trim().to_owned(),
        legs: legs.to_vec(),
    };
    let serialized = serde_json::to_vec(&binding).unwrap_or_default();
    Sha256::digest(&serialized)
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect()
}

/// Go `broker.PredictionQuoteLegsHash` over an execution combo payload.
///
/// Go's trading service binds a parlay submission to the stored RFQ with
/// `PredictionQuoteLegsHash(intent.MVC, intent.Legs)`; the execution write port
/// therefore has to canonicalize the *order* payload exactly like the RFQ route
/// canonicalized the quote payload, or a legitimately issued quote would look
/// changed. Returning the MVC as well keeps both halves of the binding in one
/// place.
pub(in crate::product::product_production_ports) fn prediction_quote_binding(
    payload: &Value,
) -> Result<(String, String), String> {
    let object = payload
        .as_object()
        .ok_or_else(|| invalid("prediction combo quote payload must be an object"))?;
    let mvc = text(object, "mvc").ok_or_else(|| invalid("prediction RFQ requires mvc"))?;
    let legs = object
        .get("legs")
        .and_then(Value::as_array)
        .filter(|legs| legs.len() >= 2)
        .ok_or_else(|| invalid("prediction combo quote requires at least two legs"))?;
    let mut normalized = Vec::with_capacity(legs.len());
    for (index, leg) in legs.iter().enumerate() {
        normalized.push(normalize_leg(leg, index)?);
    }
    let mvc = mvc.trim().to_owned();
    let legs_hash = prediction_quote_binding_hash(&mvc, &normalized);
    Ok((mvc, legs_hash))
}

fn normalize_leg(leg: &Value, index: usize) -> Result<CanonicalLeg, String> {
    let object = leg
        .as_object()
        .ok_or_else(|| invalid(&format!("prediction combo leg {index} is invalid")))?;
    let instrument_id = text(object, "instrumentId")
        .unwrap_or_default()
        .to_ascii_uppercase();
    let side = text(object, "side").unwrap_or_default().to_ascii_uppercase();
    let prediction_side = text(object, "predictionSide").map(|value| value.to_ascii_uppercase());
    let product_class = text(object, "productClass").unwrap_or_else(|| "event_contract".to_owned());
    if product_class != "event_contract"
        || !instrument_id.starts_with("US.")
        || !matches!(side.as_str(), "BUY" | "SELL")
        || !matches!(prediction_side.as_deref(), Some("YES" | "NO"))
    {
        return Err(invalid(&format!("prediction combo leg {index} is invalid")));
    }
    let ratio = object
        .get("ratio")
        .and_then(Value::as_i64)
        .filter(|ratio| *ratio > 0)
        .ok_or_else(|| invalid(&format!("prediction combo leg {index} is invalid")))?;
    Ok(CanonicalLeg {
        instrument_id,
        product_class,
        side,
        ratio,
        quantity: number(object, "quantity"),
        amount: number(object, "amount"),
        price: number(object, "price"),
        prediction_side,
    })
}

fn number(object: &Map<String, Value>, key: &str) -> Option<GoNumber> {
    object.get(key).and_then(Value::as_f64).map(GoNumber)
}

fn text(object: &Map<String, Value>, key: &str) -> Option<String> {
    object
        .get(key)
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_owned)
}

fn invalid(detail: &str) -> String {
    format!("{INVALID_QUERY_PREFIX}: {detail}")
}

/// Minimal reader for the three values Go back-fills from the route query.
struct QueryValues(Map<String, Value>);

impl QueryValues {
    fn parse(raw: &str) -> Self {
        let mut values = Map::new();
        for pair in raw.split('&') {
            let mut parts = pair.splitn(2, '=');
            let key = parts.next().unwrap_or_default().trim();
            let value = parts.next().unwrap_or_default().trim();
            if !key.is_empty() && !value.is_empty() {
                values.insert(key.to_owned(), Value::String(value.to_owned()));
            }
        }
        Self(values)
    }

    fn get(&self, key: &str) -> Option<String> {
        self.0.get(key).and_then(Value::as_str).map(str::to_owned)
    }
}

impl ProductionMarketDataProviderActionsPort {
    /// Go `Service.QuotePredictionCombo` for one provider-backed RFQ request.
    pub(super) fn prediction_combo_quote(
        &self,
        request: &super::MarketDataProviderActionsRequest,
    ) -> Result<Value, MarketDataProviderActionsPortError> {
        let snapshot = self
            .active_provider_state
            .as_ref()
            .map(|state| state.snapshot());
        if snapshot.as_ref().is_none_or(|value| {
            value.provider != Some(jftrade_settings::MarketDataProvider::Futu)
                || !value.opend_ready
        }) {
            return Err(MarketDataProviderActionsPortError::Unavailable(
                "Futu prediction combo quote provider is not ready".to_owned(),
            ));
        }
        let runtime = self.trade_runtime.as_ref().ok_or_else(|| {
            MarketDataProviderActionsPortError::Unavailable(
                "Futu prediction combo quote runtime is not configured".to_owned(),
            )
        })?;
        if !runtime.prediction_combo_quote_available() {
            return Err(MarketDataProviderActionsPortError::Unavailable(
                "Futu prediction combo quote adapter is not ready".to_owned(),
            ));
        }
        let payload: Value = serde_json::from_slice(&request.body).map_err(|_| {
            super::action_bad_request("BAD_REQUEST", "invalid prediction combo quote payload")
        })?;
        let parsed = PredictionComboQuoteRequest::parse(&payload, &request.query)
            .map_err(|message| super::action_bad_request("BAD_REQUEST", &message))?;
        // Go resolves the feature through the broker registry, which runs
        // `predictionEligibility` for every `prediction.*` feature before the
        // adapter is reached: a non-FUTUINC or non-US account can never mint a
        // persisted RFQ.
        if let Err(detail) = runtime.prediction_combo_quote_eligibility(&parsed.account_id) {
            return Err(MarketDataProviderActionsPortError::Failed {
                status: 403,
                code: PREDICTION_INELIGIBLE_CODE.to_owned(),
                message: format!("{PREDICTION_INELIGIBLE_MESSAGE}: {detail}"),
                retry_after_seconds: None,
            });
        }
        let adapter_payload = normalized_adapter_payload(&payload, &parsed);
        let result = runtime
            .prediction_combo_quote(&adapter_payload)
            .map_err(super::map_prediction_combo_quote_error)?;
        let quote_id = result
            .pointer("/metadata/quoteId")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| failed("prediction combo quote did not return quoteId".to_owned()))?
            .to_owned();
        let received_at = (self.prediction_quote_clock)();
        let expires_at = (self.prediction_quote_expiry)(&received_at)
            .map_err(failed)?;
        let store = self
            .prediction_quotes
            .as_ref()
            .ok_or_else(|| failed("prediction quote persistence is unavailable".to_owned()))?;
        store
            .save_prediction_quote(&jftrade_store_sqlite::StoredPredictionQuote {
                quote_id,
                broker_id: parsed.broker_id.clone(),
                account_id: parsed.account_id.clone(),
                trading_environment: parsed.trading_environment.clone(),
                mvc: parsed.mvc.clone(),
                legs_hash: parsed.legs_hash(),
                bid_price: metadata_number(&result, "bidPrice"),
                ask_price: metadata_number(&result, "askPrice"),
                should_retry: result
                    .pointer("/metadata/shouldRetry")
                    .and_then(Value::as_bool)
                    .unwrap_or(false),
                received_at: received_at.clone(),
                expires_at: expires_at.clone(),
                expiry_source: PREDICTION_QUOTE_EXPIRY_SOURCE.to_owned(),
                status: "active".to_owned(),
                consumed_at: None,
                consumed_preview_id: None,
                consumed_client_order_id: None,
            })
            .map_err(|error| failed(format!("persist prediction quote: {error}")))?;
        let mut result = result;
        apply_quote_metadata(&mut result, &parsed, &received_at, &expires_at);
        Ok(result)
    }
}

fn failed(message: String) -> MarketDataProviderActionsPortError {
    MarketDataProviderActionsPortError::Failed {
        status: 502,
        code: "BROKER_FEATURE_FAILED".to_owned(),
        message,
        retry_after_seconds: None,
    }
}

/// Hand the adapter the normalized context and legs while leaving any other
/// caller fields untouched.
fn normalized_adapter_payload(payload: &Value, parsed: &PredictionComboQuoteRequest) -> Value {
    let mut payload = payload.clone();
    if let Some(object) = payload.as_object_mut() {
        object.insert("brokerId".to_owned(), json!(parsed.broker_id));
        object.insert("accountId".to_owned(), json!(parsed.account_id));
        object.insert(
            "tradingEnvironment".to_owned(),
            json!(parsed.trading_environment),
        );
        object.insert("mvc".to_owned(), json!(parsed.mvc));
        object.insert(
            "legs".to_owned(),
            serde_json::to_value(&parsed.legs).unwrap_or(Value::Null),
        );
    }
    payload
}

fn metadata_number(result: &Value, key: &str) -> Option<f64> {
    result
        .pointer(&format!("/metadata/{key}"))
        .and_then(Value::as_f64)
}

/// Go rewrites `quoteId`/`mvc`/`receivedAt`/`quoteExpiresAt`/`expirySource`
/// after the adapter response and appends exactly one policy warning.
fn apply_quote_metadata(
    result: &mut Value,
    request: &PredictionComboQuoteRequest,
    received_at: &str,
    expires_at: &str,
) {
    let Some(object) = result.as_object_mut() else {
        return;
    };
    let metadata = object.entry("metadata").or_insert_with(|| json!({}));
    if let Some(metadata) = metadata.as_object_mut() {
        metadata.insert("mvc".to_owned(), json!(request.mvc));
        metadata.insert("receivedAt".to_owned(), json!(received_at));
        metadata.insert("quoteExpiresAt".to_owned(), json!(expires_at));
        metadata.insert(
            "expirySource".to_owned(),
            json!(PREDICTION_QUOTE_EXPIRY_SOURCE),
        );
    }
    let warnings = object.entry("warnings").or_insert_with(|| json!([]));
    if let Some(warnings) = warnings.as_array_mut() {
        warnings.push(json!(PREDICTION_QUOTE_WARNING));
    }
}

/// Add the fixed 30 second server-side window to the receive instant.
pub(super) fn prediction_quote_expiry(received_at: &str) -> Result<String, String> {
    let received = time::OffsetDateTime::parse(
        received_at,
        &time::format_description::well_known::Rfc3339,
    )
    .map_err(|error| format!("quoteReceivedAt is invalid: {error}"))?;
    (received + time::Duration::seconds(PREDICTION_QUOTE_TTL_SECONDS))
        .format(&time::format_description::well_known::Rfc3339)
        .map_err(|error| format!("quoteExpiresAt is invalid: {error}"))
}
