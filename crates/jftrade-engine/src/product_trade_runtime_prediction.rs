//! Validation and provider projection helpers for prediction subscriptions.

use serde_json::{Value, json};

use super::super::super::product_production_ports_market_data::product_production_ports_market_data_projection::{
    current_unix_millis, format_unix_millis_rfc3339,
};

pub(super) fn normalize_prediction_code(value: &str) -> Result<String, String> {
    let value = value.trim();
    let value = value.strip_prefix("US.").unwrap_or(value);
    if value.is_empty()
        || value.len() > 512
        || value.chars().any(|ch| {
            ch.is_whitespace() || ch.is_control() || matches!(ch, '/' | '\\' | '?' | '#')
        })
    {
        return Err("invalid prediction subscription code".to_owned());
    }
    Ok(value.to_ascii_uppercase())
}

pub(super) fn normalize_prediction_data_types(values: &[String]) -> Result<Vec<String>, String> {
    let mut result = Vec::new();
    for value in values {
        let value = value.trim().to_ascii_uppercase();
        if !matches!(value.as_str(), "ORDER_BOOK" | "KLINE" | "TICKER") {
            return Err(format!("unsupported prediction data type {value:?}"));
        }
        if !result.contains(&value) {
            result.push(value);
        }
    }
    if result.is_empty() {
        return Err("at least one prediction data type is required".to_owned());
    }
    result.sort();
    Ok(result)
}

pub(super) fn prediction_provider_value(data_types: &[String]) -> Value {
    let feature_id = if data_types.len() == 1 && data_types[0] == "ORDER_BOOK" {
        "prediction.depth"
    } else {
        "prediction.history"
    };
    let resolved_at = format_unix_millis_rfc3339(current_unix_millis());
    json!({
        "brokerId": "futu",
        "securityFirm": "Futu/Moomoo via OpenD",
        "featureId": feature_id,
        "capability": "available",
        "selectionReason": "adapter_request",
        "resolvedAt": resolved_at,
        "asOf": resolved_at,
    })
}
