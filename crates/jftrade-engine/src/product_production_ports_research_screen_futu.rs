//! Futu OpenD stock-screen projection.
//!
//! The embedded-catalog branch of the stock-screen facade lives in
//! `product_production_ports_research_screen.rs`; this module owns the Futu
//! branch: OpenD query construction, row/cell projection, currency derivation
//! and the OpenD error mapping.

use super::{EXACT_MAINLAND_TOTAL_WARNING, ResearchScreenWriteQuery, ResearchScreenWritePortError};
use serde_json::{Value, json};

pub(super) fn project_futu_screen_result(
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

pub(crate) fn futu_quote_currency(market: &str, symbol: &str, name: &str) -> Option<&'static str> {
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

