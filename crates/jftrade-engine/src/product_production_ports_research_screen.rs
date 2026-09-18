//! Production stock-screen adapter backed by the embedded market-data helper.
//!
//! The public request is normalized by `product_research_screen_write_port`.
//! This module only translates that normalized definition into the provider
//! neutral helper contract and validates the response before handing it back
//! to the public wire projector.

use crate::product::product_active_provider_state::ActiveProviderState;
use crate::product::product_research_screen_write_port::{
    ResearchScreenWritePort, ResearchScreenWritePortError, ResearchScreenWriteQuery,
};
use jftrade_integration_marketdata_helper::{HelperClient, HttpAdapterError};
use serde_json::{Value, json};
use std::sync::Arc;
use std::thread;

const EMBEDDED_CATALOG_VERSION: &str = "embedded-stock-screen-v1";
const FUTU_CATALOG_VERSION: &str = "futu-stock-screen-v1";
const EXACT_MAINLAND_TOTAL_WARNING: &str =
    "OpenD reports only a combined A-share total; total is omitted for exact SH/SZ results.";

#[path = "product_production_ports_research_screen_futu.rs"]
mod futu;
use crate::product::product_production_ports::product_production_ports_helper_runtime::classify_helper_runtime_code;
#[cfg(test)]
pub(super) use futu::futu_quote_currency;
use futu::project_futu_screen_result;


/// Concrete stock-screen adapter for yfinance and AKShare.
///
/// A helper client is optional because the external process is allowed to be
/// unavailable at runtime.  In that state the route remains registered and
/// returns the normal `503` unavailable envelope; no fixture data is used.
pub(crate) struct ProductionResearchScreenHelperPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
    pub(crate) helper: Option<HelperClient>,
    pub(crate) trade_runtime: Option<Arc<crate::product::product_production_ports::SharedTradeReadRuntime>>,
}

impl std::fmt::Debug for ProductionResearchScreenHelperPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ProductionResearchScreenHelperPort")
            .field("helper", &self.helper.is_some())
            .field("futu", &self.trade_runtime.is_some())
            .finish()
    }
}

impl ResearchScreenWritePort for ProductionResearchScreenHelperPort {
    fn query(
        &self,
        request: &ResearchScreenWriteQuery,
    ) -> Result<Value, ResearchScreenWritePortError> {
        let snapshot = self.active_provider_state.snapshot();
        let provider = match snapshot.provider {
            Some(jftrade_settings::MarketDataProvider::Yfinance) => "yfinance",
            Some(jftrade_settings::MarketDataProvider::Akshare) => "akshare",
            Some(jftrade_settings::MarketDataProvider::Futu) => {
                return self.query_futu(request, snapshot.opend_ready);
            }
            None => {
                return Err(ResearchScreenWritePortError::Capability(
                    "stock screen is unavailable for the active provider".to_owned(),
                ));
            }
        };
        if !request.broker_id.trim().is_empty()
            && !request.broker_id.eq_ignore_ascii_case(provider)
            && !(provider == "yfinance"
                && request.broker_id.eq_ignore_ascii_case("yahoo-finance"))
        {
            return Err(ResearchScreenWritePortError::Capability(format!(
                "requested broker {:?} does not match active provider {provider:?}",
                request.broker_id
            )));
        }
        if let Some(catalog) = request
            .definition
            .get("catalogVersion")
            .and_then(Value::as_str)
            .filter(|catalog| !catalog.trim().is_empty())
            && catalog != EMBEDDED_CATALOG_VERSION
        {
            return Err(ResearchScreenWritePortError::Capability(format!(
                "catalog {catalog:?} requires the futu broker"
            )));
        }
        if !snapshot.helper_ready {
            return Err(ResearchScreenWritePortError::Unavailable);
        }
        let helper = self
            .helper
            .clone()
            .ok_or(ResearchScreenWritePortError::Unavailable)?;
        let payload = screen_helper_request(request)?;
        let result = thread::spawn(move || {
            let runtime = tokio::runtime::Builder::new_current_thread()
                .enable_all()
                .build()
                .map_err(|error| HttpAdapterError::Unavailable(error.to_string()))?;
            runtime.block_on(
                helper.post_json::<Value, Value>(&["providers", provider, "screen"], &payload),
            )
        })
        .join()
        .map_err(|_| {
            ResearchScreenWritePortError::Failed("research screen helper task panicked".to_owned())
        })?
        .map_err(map_screen_helper_error)?;
        project_screen_helper_result(result, request, provider)
    }
}

impl ProductionResearchScreenHelperPort {
    fn query_futu(
        &self,
        request: &ResearchScreenWriteQuery,
        opend_ready: bool,
    ) -> Result<Value, ResearchScreenWritePortError> {
        if !request.broker_id.trim().is_empty()
            && !request.broker_id.eq_ignore_ascii_case("futu")
        {
            return Err(ResearchScreenWritePortError::Capability(
                "requested broker does not match active provider \"futu\"".to_owned(),
            ));
        }
        if request
            .definition
            .get("catalogVersion")
            .and_then(Value::as_str)
            != Some(FUTU_CATALOG_VERSION)
        {
            return Err(ResearchScreenWritePortError::Capability(
                "catalog requires the futu broker".to_owned(),
            ));
        }
        if !opend_ready {
            return Err(ResearchScreenWritePortError::Unavailable);
        }
        let runtime = self
            .trade_runtime
            .as_ref()
            .ok_or(ResearchScreenWritePortError::Unavailable)?;
        if !runtime.stock_screen_reader_available() {
            return Err(ResearchScreenWritePortError::Unavailable);
        }
        query_futu_runtime(runtime, request)
    }
}

pub(crate) fn query_futu_runtime(
    runtime: &crate::product::product_production_ports::SharedTradeReadRuntime,
    request: &ResearchScreenWriteQuery,
) -> Result<Value, ResearchScreenWritePortError> {
    let query = jftrade_integration_futu::StockScreenQuery::new(
        request.market.clone(),
        request.definition.clone(),
        request.offset,
        request.limit,
    )
    .map_err(map_futu_screen_error)?;
    let page = runtime.stock_screen(&query).map_err(map_futu_screen_error)?;
    project_futu_screen_result(page, request)
}

pub(super) fn map_futu_screen_error(
    error: jftrade_integration_futu::StockScreenQueryError,
) -> ResearchScreenWritePortError {
    use jftrade_integration_futu::StockScreenQueryError;
    match error {
        StockScreenQueryError::RateLimited { retry_after_ms } => {
            ResearchScreenWritePortError::RateLimited {
                message: "Futu stock screen request rate limited".to_owned(),
                retry_after: retry_after_ms.div_ceil(1000).max(1),
            }
        }
        StockScreenQueryError::InvalidQuery(message)
            if message.contains("runtime is unavailable")
                || message.contains("session")
                || message.contains("not ready") => ResearchScreenWritePortError::Unavailable,
        StockScreenQueryError::Session(_) => ResearchScreenWritePortError::Unavailable,
        StockScreenQueryError::InvalidQuery(message) => {
            ResearchScreenWritePortError::Failed(message)
        }
        other => ResearchScreenWritePortError::Failed(other.to_string()),
    }
}

fn screen_helper_request(
    request: &ResearchScreenWriteQuery,
) -> Result<Value, ResearchScreenWritePortError> {
    let object = request.definition.as_object().ok_or_else(|| {
        ResearchScreenWritePortError::Failed(
            "normalized stock-screen definition is not an object".to_owned(),
        )
    })?;
    let conditions = object
        .get("conditions")
        .and_then(Value::as_array)
        .map(|items| {
            items
                .iter()
                .map(screen_helper_condition)
                .collect::<Result<Vec<_>, _>>()
        })
        .transpose()?
        .unwrap_or_default();
    let sorts = object
        .get("sorts")
        .and_then(Value::as_array)
        .map(|items| {
            items
                .iter()
                .map(screen_helper_sort)
                .collect::<Result<Vec<_>, _>>()
        })
        .transpose()?
        .unwrap_or_default();
    Ok(json!({
        "market": request.market,
        "conditions": conditions,
        "sorts": sorts,
        "offset": request.offset,
        "limit": request.limit,
    }))
}

fn screen_helper_condition(value: &Value) -> Result<Value, ResearchScreenWritePortError> {
    let object = value.as_object().ok_or_else(|| {
        ResearchScreenWritePortError::Failed(
            "normalized stock-screen condition is not an object".to_owned(),
        )
    })?;
    let factor = object
        .get("factor")
        .and_then(Value::as_object)
        .and_then(|factor| factor.get("factorKey"))
        .and_then(Value::as_str)
        .filter(|factor| !factor.trim().is_empty())
        .ok_or_else(|| {
            ResearchScreenWritePortError::Failed(
                "stock-screen condition factor is missing".to_owned(),
            )
        })?;
    let operator = object
        .get("operator")
        .and_then(Value::as_str)
        .unwrap_or("eq")
        .trim()
        .to_ascii_lowercase();
    let condition_value = object.get("value").ok_or_else(|| {
        ResearchScreenWritePortError::Failed("stock-screen condition value is missing".to_owned())
    })?;
    let mut output = serde_json::Map::new();
    output.insert("factor_key".to_owned(), json!(factor));
    match operator.as_str() {
        "between" => {
            // Go's embeddedScreenCondition folds a decoded interval into
            // provider-neutral min/max text; every other operator has no
            // embedded-catalog form and stays capability-unavailable.
            let bounds = condition_value.as_object().ok_or_else(|| {
                ResearchScreenWritePortError::Capability(
                    "stock-screen condition value must be an interval".to_owned(),
                )
            })?;
            if bounds.contains_key("intervals") {
                return Err(ResearchScreenWritePortError::Capability(
                    "stock-screen multi-interval conditions are not executable against the embedded catalog"
                        .to_owned(),
                ));
            }
            if let Some(min) = bounds.get("min").and_then(embedded_screen_bound) {
                output.insert("min".to_owned(), min);
            }
            if let Some(max) = bounds.get("max").and_then(embedded_screen_bound) {
                output.insert("max".to_owned(), max);
            }
            if !output.contains_key("min") && !output.contains_key("max") {
                return Err(ResearchScreenWritePortError::Capability(
                    "stock-screen condition requires min or max".to_owned(),
                ));
            }
        }
        _ => {
            return Err(ResearchScreenWritePortError::Capability(format!(
                "stock-screen condition operator {operator:?} is not executable against the embedded catalog"
            )));
        }
    }
    Ok(Value::Object(output))
}

/// Mirror Go's `embeddedScreenNumber`: an integral float bound is rendered as
/// the exact integer text the editor validated instead of a float-shaped
/// `100.0`, and non-numeric bounds are dropped so the caller can decide
/// whether the condition still has a bound.
fn embedded_screen_bound(value: &Value) -> Option<Value> {
    let number = value.as_number()?;
    if number.is_i64() || number.is_u64() {
        return Some(value.clone());
    }
    let float = number.as_f64()?;
    if !float.is_finite() {
        return None;
    }
    if float.fract() == 0.0 && float.abs() <= 9_007_199_254_740_992.0 {
        return Some(json!((float as i64)));
    }
    Some(value.clone())
}

fn screen_helper_sort(value: &Value) -> Result<Value, ResearchScreenWritePortError> {
    let object = value.as_object().ok_or_else(|| {
        ResearchScreenWritePortError::Failed(
            "normalized stock-screen sort is not an object".to_owned(),
        )
    })?;
    let factor = object
        .get("factor")
        .and_then(Value::as_object)
        .and_then(|factor| factor.get("factorKey"))
        .and_then(Value::as_str)
        .filter(|factor| !factor.trim().is_empty())
        .ok_or_else(|| {
            ResearchScreenWritePortError::Failed("stock-screen sort factor is missing".to_owned())
        })?;
    let direction = object
        .get("direction")
        .and_then(Value::as_str)
        .unwrap_or("desc")
        .trim()
        .to_ascii_lowercase();
    if !matches!(direction.as_str(), "asc" | "desc") {
        return Err(ResearchScreenWritePortError::Capability(format!(
            "stock-screen sort direction {direction:?} is not supported by the helper"
        )));
    }
    Ok(json!({"factor_key": factor, "direction": direction}))
}

fn project_screen_helper_result(
    value: Value,
    request: &ResearchScreenWriteQuery,
    provider: &str,
) -> Result<Value, ResearchScreenWritePortError> {
    let object = value.as_object().ok_or_else(|| {
        ResearchScreenWritePortError::Failed(
            "market-data helper returned a non-object screen response".to_owned(),
        )
    })?;
    let entries = object
        .get("entries")
        .and_then(Value::as_array)
        .ok_or_else(|| {
            ResearchScreenWritePortError::Failed(
                "market-data helper screen response is missing entries".to_owned(),
            )
        })?;
    let mut rows = Vec::with_capacity(entries.len());
    for entry in entries {
        rows.push(project_screen_helper_entry(entry, request)?);
    }
    let total = object.get("total").and_then(Value::as_u64).ok_or_else(|| {
        ResearchScreenWritePortError::Failed(
            "market-data helper screen response has invalid total".to_owned(),
        )
    })?;
    let has_more = object
        .get("has_more")
        .and_then(Value::as_bool)
        .ok_or_else(|| {
            ResearchScreenWritePortError::Failed(
                "market-data helper screen response has invalid has_more".to_owned(),
            )
        })?;
    let mut result = json!({
        "entries": rows,
        "total": total,
        "hasMore": has_more,
        "provider": {
            "brokerId": provider,
            "featureId": "research.screen",
            "capability": "available",
            "selectionReason": "embedded-market-data-provider",
        },
    });
    if let Some(next) = object.get("next_offset").and_then(Value::as_i64) {
        result["nextOffset"] = json!(next);
    }
    if let Some(as_of) = object.get("as_of").and_then(Value::as_str) {
        result["asOf"] = json!(as_of);
        result["provider"]["asOf"] = json!(as_of);
        result["provider"]["resolvedAt"] = json!(as_of);
    }
    Ok(result)
}

fn project_screen_helper_entry(
    value: &Value,
    request: &ResearchScreenWriteQuery,
) -> Result<Value, ResearchScreenWritePortError> {
    let object = value.as_object().ok_or_else(|| {
        ResearchScreenWritePortError::Failed(
            "market-data helper screen entry is not an object".to_owned(),
        )
    })?;
    let instrument_id = required_screen_text(object, "instrument_id")?.to_ascii_uppercase();
    let (market, symbol) = instrument_id.split_once('.').ok_or_else(|| {
        ResearchScreenWritePortError::Failed(
            "market-data helper screen entry has invalid instrument_id".to_owned(),
        )
    })?;
    if market != request.market {
        return Err(ResearchScreenWritePortError::Failed(
            "market-data helper screen entry market mismatch".to_owned(),
        ));
    }
    let values = object
        .get("values")
        .and_then(Value::as_object)
        .ok_or_else(|| {
            ResearchScreenWritePortError::Failed(
                "market-data helper screen entry is missing values".to_owned(),
            )
        })?;
    let name = object.get("name").and_then(Value::as_str).unwrap_or_default();
    let industry = object
        .get("industry")
        .and_then(Value::as_str)
        .unwrap_or_default();
    let quote_currency = object
        .get("quote_currency")
        .and_then(Value::as_str)
        .unwrap_or_default();
    let mut cells = serde_json::Map::new();
    for column in &request.columns {
        // Go's projectProviderScreen renderers key on the embedded catalog's
        // factor descriptor: the catalog's own unit string is the cell unit, and
        // factors declared as integer-valued render through the integer cell
        // type (the Futu ival mapping) rather than a generic number.
        let factor = embedded_screen_factor(&request.market, &column.factor_key);
        let unit = factor
            .as_ref()
            .and_then(|factor| factor.get("unit"))
            .and_then(Value::as_str)
            .unwrap_or(&column.unit);
        let integer_factor = factor
            .as_ref()
            .and_then(|factor| factor.get("valueType"))
            .and_then(Value::as_str)
            == Some("integer");
        let value = match column.factor_key.as_str() {
            "basic.code" => Some(Value::String(symbol.to_owned())),
            "basic.name" if !name.trim().is_empty() => Some(Value::String(name.to_owned())),
            "basic.industry" if !industry.trim().is_empty() => {
                Some(Value::String(industry.to_owned()))
            }
            _ => values.get(&column.factor_key).cloned(),
        };
        let projected = match value {
            Some(number)
                if integer_factor
                    && number.as_i64().is_some()
                    && number.as_f64().is_some_and(f64::is_finite) =>
            {
                json!({
                    "columnId": column.column_id,
                    "instanceId": column.instance_id,
                    "factorKey": column.factor_key,
                    "value": {"type": "integer", "integer": number, "unit": unit},
                })
            }
            Some(number) if number.as_f64().is_some_and(f64::is_finite) => json!({
                "columnId": column.column_id,
                "instanceId": column.instance_id,
                "factorKey": column.factor_key,
                "value": {"type": "number", "number": number, "unit": unit},
            }),
            Some(Value::String(text)) if !text.trim().is_empty() => json!({
                "columnId": column.column_id,
                "instanceId": column.instance_id,
                "factorKey": column.factor_key,
                "value": {"type": "string", "string": text, "unit": unit},
            }),
            Some(Value::String(_)) | None => json!({
                "columnId": column.column_id,
                "instanceId": column.instance_id,
                "factorKey": column.factor_key,
                "value": {"type": "missing", "unit": unit},
            }),
            Some(_) => {
                return Err(ResearchScreenWritePortError::Failed(
                    "market-data helper screen value is not finite".to_owned(),
                ));
            }
        };
        cells.insert(column.column_id.clone(), projected);
    }
    let mut row = json!({
        "stockId": instrument_id,
        "instrumentId": instrument_id,
        "market": market,
        "symbol": symbol,
        "productClass": "equity",
        "cells": cells,
    });
    if !name.trim().is_empty() {
        row["name"] = json!(name.trim());
    }
    if !industry.trim().is_empty() {
        row["industry"] = json!(industry.trim());
    }
    if !quote_currency.trim().is_empty() {
        row["quoteCurrency"] = json!(quote_currency.trim().to_ascii_uppercase());
    }
    Ok(row)
}

/// Look up a factor descriptor in the embedded screen catalog.
///
/// Go resolves the descriptor through `rescreens.LookupEmbedded` and renders
/// cells from it, so the helper mirrors that lookup for the factor's unit and
/// value type. An unknown factor is reported as absent rather than guessed.
fn embedded_screen_factor(market: &str, key: &str) -> Option<Value> {
    for broker in ["yfinance", "akshare"] {
        let Ok(catalog) = jftrade_research::screen_catalog(broker, market) else {
            continue;
        };
        if let Some(factor) = catalog
            .get("factors")
            .and_then(Value::as_array)
            .and_then(|factors| {
                factors
                    .iter()
                    .find(|factor| factor.get("key").and_then(Value::as_str) == Some(key))
            })
        {
            return Some(factor.clone());
        }
    }
    None
}

fn required_screen_text<'a>(
    object: &'a serde_json::Map<String, Value>,
    key: &str,
) -> Result<&'a str, ResearchScreenWritePortError> {
    object
        .get(key)
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| {
            ResearchScreenWritePortError::Failed(format!(
                "market-data helper screen entry is missing {key}"
            ))
        })
}

fn map_screen_helper_error(error: HttpAdapterError) -> ResearchScreenWritePortError {
    match error {
        HttpAdapterError::Remote {
            status: _,
            code,
            message: _,
            retry_after_seconds: _,
        } if classify_helper_runtime_code(&code).is_some() => {
            // Helper runtime pressure keeps the market-data lifecycle identity
            // so the transport answers 503 with the fixed Retry-After contract.
            let runtime = classify_helper_runtime_code(&code).expect("matched runtime code");
            if runtime.code == "MARKET_DATA_PROVIDER_WARMING" {
                ResearchScreenWritePortError::ProviderWarming
            } else {
                ResearchScreenWritePortError::ProviderBusy
            }
        }
        HttpAdapterError::Remote {
            status: 429,
            code: _,
            message,
            retry_after_seconds,
        } => ResearchScreenWritePortError::RateLimited {
            message,
            retry_after: retry_after_seconds.unwrap_or(1),
        },
        HttpAdapterError::Remote {
            status: 400,
            message,
            ..
        } => ResearchScreenWritePortError::Failed(message),
        HttpAdapterError::Remote {
            status: 409,
            message,
            ..
        } => ResearchScreenWritePortError::Capability(message),
        HttpAdapterError::Remote { message, .. } => ResearchScreenWritePortError::Failed(message),
        HttpAdapterError::Timeout => ResearchScreenWritePortError::ProviderBusy,
        HttpAdapterError::InvalidResponse(message) => ResearchScreenWritePortError::Failed(message),
        _ => ResearchScreenWritePortError::Unavailable,
    }
}

#[cfg(test)]
#[path = "product_production_ports_research_screen_tests.rs"]
mod tests;
