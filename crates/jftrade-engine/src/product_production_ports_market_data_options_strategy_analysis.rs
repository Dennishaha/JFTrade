//! Production projection for OpenD option-strategy payoff analysis.

use std::sync::Arc;

use serde_json::{Map, Value, json};

use super::super::product_production_ports_trade::SharedTradeReadRuntime;
use crate::product::MarketDataOptionsReadSnapshotError;

pub(crate) fn read(
    runtime: Option<&Arc<SharedTradeReadRuntime>>,
    _path: &str,
    query: &str,
) -> Result<Value, MarketDataOptionsReadSnapshotError> {
    let runtime = runtime.ok_or_else(|| {
        MarketDataOptionsReadSnapshotError::Unavailable(
            "Futu option analysis runtime is not configured".to_owned(),
        )
    })?;
    if !runtime.option_strategy_analysis_available() {
        return Err(MarketDataOptionsReadSnapshotError::Unavailable(
            "Futu option strategy analysis reader is not ready".to_owned(),
        ));
    }
    let request = parse_request(query)?;
    let snapshot = runtime.option_strategy_analysis(&request).map_err(map_error)?;
    let entry = serde_json::to_value(snapshot).map_err(|error| {
        MarketDataOptionsReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: format!("failed to serialize OpenD option strategy analysis: {error}"),
        }
    })?;
    let as_of = super::super::provider_now_rfc3339();
    Ok(json!({
        "provider": {
            "brokerId": "futu",
            "securityFirm": "Futu/Moomoo via OpenD",
            "featureId": "derivatives.option_analysis",
            "capability": "available",
            "selectionReason": "adapter_request",
            "resolvedAt": as_of,
            "asOf": as_of,
        },
        "asOf": as_of,
        "entries": [entry],
        "hasMore": false,
        "total": 1,
        "metadata": { "multiLegCount": request.multi_legs.len() },
    }))
}

fn parse_request(
    query: &str,
) -> Result<jftrade_integration_futu::OptionStrategyAnalysisQuery, MarketDataOptionsReadSnapshotError>
{
    let query_map = crate::product::product_query::QueryMap::parse(query)
        .map_err(|_| bad_request("invalid URL escape"))?;
    let values = ["multiLegs", "multi_legs", "legs"]
        .iter()
        .find_map(|key| query_map.get_all(key));
    let Some(values) = values else {
        return Err(bad_request(
            "strategy_analysis requires multiLegs (or legs) query data",
        ));
    };
    if values.is_empty() {
        return Err(bad_request("strategy_analysis requires at least one combo leg"));
    }
    let mut legs = Vec::new();
    for raw in values {
        let raw = raw.trim();
        if raw.is_empty() {
            return Err(bad_request("strategy_analysis combo leg is empty"));
        }
        if let Ok(value) = serde_json::from_str::<Value>(raw) {
            append_json_legs(value, &mut legs)?;
        } else {
            legs.push(parse_compact_leg(raw)?);
        }
    }
    let request = jftrade_integration_futu::OptionStrategyAnalysisQuery { multi_legs: legs };
    request.validate().map_err(|error| bad_request(&error.to_string()))?;
    Ok(request)
}

fn append_json_legs(
    value: Value,
    legs: &mut Vec<jftrade_integration_futu::OptionStrategyLeg>,
) -> Result<(), MarketDataOptionsReadSnapshotError> {
    match value {
        Value::Array(items) => {
            if items.is_empty() {
                return Err(bad_request("strategy_analysis combo leg list is empty"));
            }
            for item in items {
                legs.push(parse_json_leg(item)?);
            }
        }
        item @ Value::Object(_) => legs.push(parse_json_leg(item)?),
        _ => return Err(bad_request("strategy_analysis combo legs must be JSON objects")),
    }
    Ok(())
}

fn parse_json_leg(value: Value) -> Result<jftrade_integration_futu::OptionStrategyLeg, MarketDataOptionsReadSnapshotError> {
    let object = value
        .as_object()
        .ok_or_else(|| bad_request("strategy_analysis combo leg must be an object"))?;
    let security_value = object.get("security").unwrap_or(&value);
    let security = security_from_value(security_value, object)?;
    let side = object
        .get("side")
        .or_else(|| object.get("trdSide"))
        .and_then(value_to_side);
    let qty_ratio = object
        .get("qtyRatio")
        .or_else(|| object.get("qty_ratio"))
        .or_else(|| object.get("ratio"))
        .and_then(Value::as_f64)
        .or_else(|| {
            object
                .get("qtyRatio")
                .or_else(|| object.get("qty_ratio"))
                .or_else(|| object.get("ratio"))
                .and_then(Value::as_str)
                .and_then(|value| value.parse::<f64>().ok())
        });
    let position_id = object
        .get("positionId")
        .or_else(|| object.get("position_id"))
        .and_then(Value::as_u64);
    let pred_side = object
        .get("predSide")
        .or_else(|| object.get("pred_side"))
        .and_then(value_to_i32);
    Ok(jftrade_integration_futu::OptionStrategyLeg {
        security,
        side,
        qty_ratio,
        position_id,
        pred_side,
    })
}

fn security_from_value(
    value: &Value,
    root: &Map<String, Value>,
) -> Result<jftrade_integration_futu::OptionStrategySecurity, MarketDataOptionsReadSnapshotError> {
    let object = value.as_object().unwrap_or(root);
    let mut market = object
        .get("market")
        .and_then(Value::as_str)
        .map(str::to_owned)
        .unwrap_or_default();
    let mut code = object
        .get("code")
        .or_else(|| object.get("symbol"))
        .and_then(Value::as_str)
        .map(str::to_owned)
        .unwrap_or_default();
    if (market.is_empty() || code.is_empty())
        && let Some(instrument_id) = object
            .get("instrumentId")
            .or_else(|| object.get("instrument_id"))
            .and_then(Value::as_str)
        && let Some((instrument_market, instrument_code)) = instrument_id.split_once('.')
    {
        market = instrument_market.to_owned();
        code = instrument_code.to_owned();
    }
    if market.is_empty() || code.is_empty() {
        return Err(bad_request(
            "strategy_analysis combo leg security requires market and code",
        ));
    }
    let market = market.trim().to_ascii_uppercase();
    let code = code.trim().to_ascii_uppercase();
    let quote_market = object
        .get("quoteMarket")
        .or_else(|| object.get("quote_market"))
        .and_then(Value::as_str)
        .unwrap_or(&market)
        .trim()
        .to_ascii_uppercase();
    let trade_market = object
        .get("tradeMarket")
        .or_else(|| object.get("trade_market"))
        .and_then(Value::as_str)
        .unwrap_or(&market)
        .trim()
        .to_ascii_uppercase();
    let instrument_id = object
        .get("instrumentId")
        .or_else(|| object.get("instrument_id"))
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_ascii_uppercase)
        .unwrap_or_else(|| format!("{market}.{code}"));
    Ok(jftrade_integration_futu::OptionStrategySecurity {
        market,
        code,
        quote_market,
        trade_market,
        instrument_id,
    })
}

fn parse_compact_leg(
    value: &str,
) -> Result<jftrade_integration_futu::OptionStrategyLeg, MarketDataOptionsReadSnapshotError> {
    let mut parts = value.split(':');
    let instrument = parts.next().unwrap_or_default().trim();
    let (market, code) = instrument
        .split_once('.')
        .filter(|(market, code)| !market.trim().is_empty() && !code.trim().is_empty())
        .ok_or_else(|| bad_request("strategy_analysis compact leg must be MARKET.CODE:SIDE:RATIO"))?;
    let side = parts
        .next()
        .and_then(|value| value_to_side(&Value::String(value.to_owned())));
    let qty_ratio = parts.next().and_then(|value| value.parse::<f64>().ok());
    Ok(jftrade_integration_futu::OptionStrategyLeg {
        security: jftrade_integration_futu::OptionStrategySecurity {
            market: market.trim().to_ascii_uppercase(),
            code: code.trim().to_ascii_uppercase(),
            quote_market: market.trim().to_ascii_uppercase(),
            trade_market: market.trim().to_ascii_uppercase(),
            instrument_id: format!(
                "{}.{}",
                market.trim().to_ascii_uppercase(),
                code.trim().to_ascii_uppercase()
            ),
        },
        side,
        qty_ratio,
        position_id: None,
        pred_side: None,
    })
}

fn value_to_i32(value: &Value) -> Option<i32> {
    value
        .as_i64()
        .and_then(|value| i32::try_from(value).ok())
        .or_else(|| value.as_str()?.trim().parse::<i32>().ok())
}

fn value_to_side(value: &Value) -> Option<i32> {
    value_to_i32(value).or_else(|| match value.as_str()?.trim().to_ascii_uppercase().as_str() {
        "BUY" => Some(1),
        "SELL" => Some(2),
        "SELL_SHORT" | "SELLSHORT" => Some(3),
        "BUY_BACK" | "BUYBACK" => Some(4),
        _ => None,
    })
}

fn bad_request(message: &str) -> MarketDataOptionsReadSnapshotError {
    MarketDataOptionsReadSnapshotError::Failed {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: message.to_owned(),
    }
}

fn map_error(
    error: jftrade_integration_futu::OptionStrategyAnalysisQueryError,
) -> MarketDataOptionsReadSnapshotError {
    match error {
        jftrade_integration_futu::OptionStrategyAnalysisQueryError::InvalidQuery(message) => {
            bad_request(&message)
        }
        other => MarketDataOptionsReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: other.to_string(),
        },
    }
}
