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

fn map_futu_screen_error(
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

fn project_futu_screen_result(
    page: jftrade_integration_futu::StockScreenPage,
    request: &ResearchScreenWriteQuery,
) -> Result<Value, ResearchScreenWritePortError> {
    let as_of = crate::product::product_production_ports::provider_now_rfc3339();
    let mut entries = Vec::with_capacity(page.items.len());
    for item in &page.items {
        if futu_screen_market_matches(request.market.as_str(), item) {
            entries.push(project_futu_screen_entry(item, request)?);
        }
    }
    let exact_mainland = matches!(request.market.as_str(), "SH" | "SZ");
    let raw_len = page.items.len() as i64;
    let all_count = page.all_count.map(i64::from);
    let has_more = !page.last_page
        && all_count.is_some_and(|total| request.offset.saturating_add(raw_len) < total);
    let mut result = json!({
        "provider": {
            "brokerId": "futu",
            "securityFirm": "Futu/Moomoo via OpenD",
            "featureId": "research.screen",
            "capability": "available",
            "selectionReason": "adapter_request",
            "resolvedAt": as_of,
            "asOf": as_of,
        },
        "asOf": as_of,
        "entries": entries,
        "hasMore": has_more,
        "catalogVersion": request.definition["catalogVersion"].clone(),
        "columns": request
            .columns
            .iter()
            .map(|column| {
                let mut value = serde_json::Map::new();
                value.insert("columnId".to_owned(), json!(column.column_id));
                value.insert("instanceId".to_owned(), json!(column.instance_id));
                value.insert("factorKey".to_owned(), json!(column.factor_key));
                if !column.label.is_empty() {
                    value.insert("label".to_owned(), json!(column.label));
                }
                if !column.unit.is_empty() {
                    value.insert("unit".to_owned(), json!(column.unit));
                }
                Value::Object(value)
            })
            .collect::<Vec<_>>(),
    });
    if !exact_mainland {
        if let Some(total) = all_count {
            result["total"] = json!(total);
        }
    } else {
        result["warnings"] = json!([EXACT_MAINLAND_TOTAL_WARNING]);
    }
    if has_more {
        result["nextOffset"] = json!(request.offset.saturating_add(raw_len));
    }
    Ok(result)
}

fn futu_screen_market_matches(
    request_market: &str,
    item: &jftrade_integration_futu::StockScreenItem,
) -> bool {
    match request_market {
        "SH" | "SZ" => item
            .security
            .as_ref()
            .is_some_and(|security| security.market == request_market),
        "CN" => item.security.as_ref().is_some_and(|security| {
            matches!(security.market.as_str(), "SH" | "SZ")
        }),
        _ => true,
    }
}

fn project_futu_screen_entry(
    item: &jftrade_integration_futu::StockScreenItem,
    request: &ResearchScreenWriteQuery,
) -> Result<Value, ResearchScreenWritePortError> {
    let security = item.security.as_ref();
    let market = security
        .map(|value| value.market.clone())
        .filter(|value| !value.is_empty())
        .unwrap_or_else(|| {
            if matches!(request.market.as_str(), "SH" | "SZ" | "CN") {
                String::new()
            } else {
                request.market.clone()
            }
        });
    let symbol = security.map(|value| value.code.clone()).unwrap_or_else(|| {
        result_text(&item.results, "basic.code").unwrap_or_default()
    });
    let instrument_id = security
        .map(|value| value.instrument_id.clone())
        .filter(|value| !value.is_empty())
        .or_else(|| (!symbol.is_empty() && !market.is_empty()).then(|| format!("{market}.{symbol}")));
    let name = result_text(&item.results, "basic.name").unwrap_or_default();
    let industry = result_text(&item.results, "basic.industry").unwrap_or_default();
    let mut cells = serde_json::Map::new();
    for column in &request.columns {
        let value = find_futu_result(item, column, request);
        cells.insert(column.column_id.clone(), futu_cell(column, value));
    }
    let mut row = json!({
        "stockId": item.stock_id.to_string(),
        "productClass": "equity",
        "cells": cells,
    });
    if let Some(instrument_id) = instrument_id {
        row["instrumentId"] = json!(instrument_id);
    }
    if !market.is_empty() {
        row["market"] = json!(market);
    }
    if !symbol.is_empty() {
        row["symbol"] = json!(symbol.to_ascii_uppercase());
    }
    if !name.trim().is_empty() {
        row["name"] = json!(name.trim());
    }
    if !industry.trim().is_empty() {
        row["industry"] = json!(industry.trim());
    }
    if let Some(currency) = futu_quote_currency(&market, &symbol, &name) {
        row["quoteCurrency"] = json!(currency);
    }
    Ok(row)
}

fn result_text(
    results: &[jftrade_integration_futu::StockScreenResult],
    factor_key: &str,
) -> Option<String> {
    results
        .iter()
        .find(|result| result.factor_key == factor_key)
        .and_then(|result| match &result.value {
            jftrade_integration_futu::StockScreenValue::String { value } => Some(value.clone()),
            jftrade_integration_futu::StockScreenValue::Integer { value } => Some(value.to_string()),
            _ => None,
        })
}

fn find_futu_result<'a>(
    item: &'a jftrade_integration_futu::StockScreenItem,
    column: &crate::product::product_research_screen_write_port::ResearchScreenColumn,
    request: &ResearchScreenWriteQuery,
) -> Option<&'a jftrade_integration_futu::StockScreenResult> {
    let candidates = item
        .results
        .iter()
        .filter(|result| result.factor_key == column.factor_key)
        .collect::<Vec<_>>();
    if candidates.len() <= 1 {
        return candidates.into_iter().next();
    }
    candidates
        .into_iter()
        .find(|result| result_params_match(result, column, request))
}

fn result_params_match(
    result: &jftrade_integration_futu::StockScreenResult,
    column: &crate::product::product_research_screen_write_port::ResearchScreenColumn,
    request: &ResearchScreenWriteQuery,
) -> bool {
    let Some(columns) = request.definition.get("columns").and_then(Value::as_array) else {
        return false;
    };
    let Some(factor) = columns.iter().find_map(|value| {
        let object = value.as_object()?;
        if object.get("columnId").and_then(Value::as_str) == Some(column.column_id.as_str()) {
            object.get("factor")?.as_object()
        } else {
            None
        }
    }) else {
        return false;
    };
    let Some(desired) = factor.get("params").and_then(Value::as_object) else {
        return result.property.params
            == jftrade_integration_futu::StockScreenPropertyParams::default();
    };
    let actual = serde_json::to_value(&result.property.params).unwrap_or_else(|_| json!({}));
    let Some(actual) = actual.as_object() else {
        return false;
    };
    desired.iter().all(|(key, expected)| {
        actual
            .get(key)
            .is_some_and(|value| json_values_equal(value, expected))
    })
}

fn json_values_equal(left: &Value, right: &Value) -> bool {
    if left == right {
        return true;
    }
    left.as_i64().is_some_and(|left| right.as_i64() == Some(left))
        || left.as_u64().is_some_and(|left| right.as_u64() == Some(left))
}

fn futu_cell(
    column: &crate::product::product_research_screen_write_port::ResearchScreenColumn,
    result: Option<&jftrade_integration_futu::StockScreenResult>,
) -> Value {
    let mut value = serde_json::Map::new();
    match result.map(|result| &result.value) {
        Some(jftrade_integration_futu::StockScreenValue::String { value: text }) => {
            value.insert("type".to_owned(), json!("string"));
            value.insert("string".to_owned(), json!(text));
        }
        Some(jftrade_integration_futu::StockScreenValue::Integer { value: number }) => {
            value.insert("type".to_owned(), json!("integer"));
            value.insert("integer".to_owned(), json!(number));
        }
        Some(jftrade_integration_futu::StockScreenValue::IntegerArray { values }) => {
            value.insert("type".to_owned(), json!("integer_array"));
            value.insert("integers".to_owned(), json!(values));
        }
        Some(jftrade_integration_futu::StockScreenValue::Number { value: number }) => {
            value.insert("type".to_owned(), json!("number"));
            value.insert("number".to_owned(), json!(number));
        }
        Some(jftrade_integration_futu::StockScreenValue::Missing) | None => {
            value.insert("type".to_owned(), json!("missing"));
        }
    }
    if !column.unit.is_empty() {
        value.insert("unit".to_owned(), json!(column.unit));
    }
    let mut cell = json!({
        "columnId": column.column_id,
        "instanceId": column.instance_id,
        "factorKey": column.factor_key,
        "value": value,
    });
    if let Some(result) = result {
        if let Some(value) = &result.enum_type_name {
            cell["value"]["enumType"] = json!(value);
        }
        if let Some(value) = &result.enum_name {
            cell["value"]["enumName"] = json!(value);
        }
        if let Some(value) = result.end_time {
            cell["value"]["endTime"] = json!(value);
        }
    }
    cell
}

fn futu_quote_currency(market: &str, symbol: &str, name: &str) -> Option<&'static str> {
    let market = market.trim().to_ascii_uppercase();
    let symbol = symbol.trim().to_ascii_uppercase();
    let name = name.trim().to_ascii_uppercase();
    if symbol.is_empty() {
        return None;
    }
    match market.as_str() {
        "US" => Some("USD"),
        "SH" | "SZ" => Some("CNY"),
        "HK" if symbol.len() == 5 && !name.is_empty() => {
            let rmb_code = symbol.starts_with('8');
            let rmb_name = name.ends_with("-R");
            if rmb_code && rmb_name {
                Some("CNY")
            } else if symbol.starts_with('0') && !rmb_name {
                Some("HKD")
            } else {
                None
            }
        }
        _ => None,
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
            let bounds = condition_value.as_object().ok_or_else(|| {
                ResearchScreenWritePortError::Failed(
                    "stock-screen between value is not an object".to_owned(),
                )
            })?;
            if let Some(min) = bounds.get("min") {
                output.insert("min".to_owned(), min.clone());
            }
            if let Some(max) = bounds.get("max") {
                output.insert("max".to_owned(), max.clone());
            }
            if !output.contains_key("min") && !output.contains_key("max") {
                return Err(ResearchScreenWritePortError::Failed(
                    "stock-screen between value has no bounds".to_owned(),
                ));
            }
        }
        "gte" | "gt" => {
            output.insert("min".to_owned(), condition_value.clone());
        }
        "lte" | "lt" => {
            output.insert("max".to_owned(), condition_value.clone());
        }
        "eq" => {
            output.insert("min".to_owned(), condition_value.clone());
            output.insert("max".to_owned(), condition_value.clone());
        }
        _ => {
            return Err(ResearchScreenWritePortError::Capability(format!(
                "stock-screen condition operator {operator:?} is not supported by the helper"
            )));
        }
    }
    Ok(Value::Object(output))
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
        let value = match column.factor_key.as_str() {
            "basic.code" => Some(Value::String(symbol.to_owned())),
            "basic.name" if !name.trim().is_empty() => Some(Value::String(name.to_owned())),
            "basic.industry" if !industry.trim().is_empty() => {
                Some(Value::String(industry.to_owned()))
            }
            _ => values.get(&column.factor_key).cloned(),
        };
        let projected = match value {
            Some(number) if number.as_f64().is_some_and(f64::is_finite) => json!({
                "columnId": column.column_id,
                "instanceId": column.instance_id,
                "factorKey": column.factor_key,
                "value": {"type": "number", "number": number, "unit": column.unit},
            }),
            Some(Value::String(text)) if !text.trim().is_empty() => json!({
                "columnId": column.column_id,
                "instanceId": column.instance_id,
                "factorKey": column.factor_key,
                "value": {"type": "string", "string": text, "unit": column.unit},
            }),
            Some(Value::String(_)) | None => json!({
                "columnId": column.column_id,
                "instanceId": column.instance_id,
                "factorKey": column.factor_key,
                "value": {"type": "missing", "unit": column.unit},
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
