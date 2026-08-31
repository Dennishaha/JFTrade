//! Validation and wire projection for the provider news/actions endpoints.
//!
//! The Python sidecar emits its Pydantic models with snake_case names.  The
//! product HTTP contract remains camelCase, so this module is the boundary
//! where helper payloads are validated and projected.

use serde_json::{Map, Value};
use std::sync::Arc;

use crate::product::MarketDataNewsActionsReadSnapshotError;

pub(super) fn validate_news_actions_payload(
    payload: Value,
    operation: &str,
    expected_market: &str,
    expected_symbol: &str,
) -> Result<Value, MarketDataNewsActionsReadSnapshotError> {
    let Some(object) = payload.as_object() else {
        return Err(news_actions_bad_gateway(
            "market-data helper returned a non-object news response",
        ));
    };
    let market = required_text(object, &["market"])?;
    let symbol = required_text(object, &["symbol"])?;
    let instrument_id = required_text(object, &["instrumentId", "instrument_id"])?;
    let source = required_text(object, &["source"])?;
    let expected_id = format!("{expected_market}.{expected_symbol}");
    if !market.eq_ignore_ascii_case(expected_market)
        || !symbol.eq_ignore_ascii_case(expected_symbol)
        || !instrument_id.eq_ignore_ascii_case(&expected_id)
    {
        return Err(news_actions_bad_gateway(
            "news/actions response identity does not match request",
        ));
    }
    let mut projected = Map::new();
    projected.insert(
        "market".to_owned(),
        Value::String(expected_market.to_owned()),
    );
    projected.insert(
        "symbol".to_owned(),
        Value::String(expected_symbol.to_owned()),
    );
    projected.insert("instrumentId".to_owned(), Value::String(expected_id));
    projected.insert("source".to_owned(), Value::String(source));
    if operation == "news" {
        projected.insert(
            "entries".to_owned(),
            project_news_entries(object.get("entries"))?,
        );
    } else {
        projected.insert(
            "events".to_owned(),
            project_corporate_events(object.get("events"))?,
        );
    }
    Ok(Value::Object(projected))
}

/// Read the broker-owned news/corporate-actions operations through the live
/// OpenD readers.  This is deliberately separate from the helper projection:
/// OpenD has no date filter for dividends/splits, so filtering is performed on
/// the typed events before they cross the product boundary.
pub(crate) fn read_futu(
    runtime: Option<&Arc<super::super::product_production_ports_trade::SharedTradeReadRuntime>>,
    path: &str,
    query: &str,
) -> Result<Value, MarketDataNewsActionsReadSnapshotError> {
    let runtime = runtime.ok_or_else(|| {
        MarketDataNewsActionsReadSnapshotError::Unavailable(
            "Futu news/actions runtime is not configured".to_owned(),
        )
    })?;
    let (operation, market, symbol, _) = super::news_actions_helper_request(path, query)?;
    if operation == "news" {
        let instrument_query = if query.trim().is_empty() {
            format!("instrumentId={market}.{symbol}")
        } else {
            format!("instrumentId={market}.{symbol}&{query}")
        };
        return super::product_production_ports_market_data_news_search::read_futu_news(
            Some(runtime),
            &instrument_query,
        )
        .map_err(|error| match error {
            crate::product::MarketDataNewsSearchReadSnapshotError::Unavailable(message) => {
                MarketDataNewsActionsReadSnapshotError::Unavailable(message)
            }
            crate::product::MarketDataNewsSearchReadSnapshotError::Failed {
                status,
                code,
                message,
                retry_after_seconds,
            } => MarketDataNewsActionsReadSnapshotError::Failed {
                status,
                code,
                message,
                retry_after_seconds,
            },
        });
    }
    let market_code = futu_market_code(&market).ok_or_else(|| {
        super::news_actions_capability("Futu corporate actions market is unsupported")
    })?;
    let query_map = crate::product::product_query::QueryMap::parse(query)
        .map_err(|_| super::news_actions_bad_request("invalid URL escape"))?;
    let from = parse_action_date(&query_map, "from")?;
    let to = parse_action_date(&query_map, "to")?;
    let base = |kind| jftrade_integration_futu::FutuCorporateActionsQuery {
        market: market_code,
        code: symbol.clone(),
        kind,
        from,
        to,
        next_key: None,
        limit: 50,
    };
    let dividends = runtime
        .corporate_actions(&base(
            jftrade_integration_futu::CorporateActionKind::Dividends,
        ))
        .map_err(map_futu_actions_error)?;
    let splits = runtime
        .corporate_actions(&base(
            jftrade_integration_futu::CorporateActionKind::StockSplits,
        ))
        .map_err(map_futu_actions_error)?;
    let mut events = dividends.events;
    events.extend(splits.events);
    events.sort_by(|left, right| left.ex_date.cmp(&right.ex_date));
    let events = events
        .into_iter()
        .map(|event| {
            serde_json::to_value(event)
                .map_err(|error| news_actions_bad_gateway(&error.to_string()))
        })
        .collect::<Result<Vec<_>, _>>()?;
    Ok(serde_json::json!({
        "market": market,
        "symbol": symbol,
        "instrumentId": format!("{market}.{symbol}"),
        "events": events,
        "source": "futu-opend",
    }))
}

fn futu_market_code(market: &str) -> Option<i32> {
    match market.to_ascii_uppercase().as_str() {
        "HK" => Some(1),
        "US" => Some(11),
        "SH" => Some(21),
        "SZ" => Some(22),
        _ => None,
    }
}

fn parse_action_date(
    query: &crate::product::product_query::QueryMap,
    key: &'static str,
) -> Result<Option<time::Date>, MarketDataNewsActionsReadSnapshotError> {
    let Some(raw) = query
        .get_first(key)
        .map(str::trim)
        .filter(|value| !value.is_empty())
    else {
        return Ok(None);
    };
    let parsed = time::OffsetDateTime::parse(raw, &time::format_description::well_known::Rfc3339)
        .map_err(|_| {
        super::news_actions_bad_request(&format!("{key} must be a valid timestamp"))
    })?;
    Ok(Some(parsed.date()))
}

fn map_futu_actions_error(message: String) -> MarketDataNewsActionsReadSnapshotError {
    if message.starts_with("invalid OpenD corporate actions query") {
        super::news_actions_bad_request(&message)
    } else {
        MarketDataNewsActionsReadSnapshotError::Failed {
            status: 502,
            code: "FUTU_CORPORATE_ACTIONS_FAILED".to_owned(),
            message,
            retry_after_seconds: None,
        }
    }
}

fn required_text(
    object: &Map<String, Value>,
    keys: &[&str],
) -> Result<String, MarketDataNewsActionsReadSnapshotError> {
    keys.iter()
        .find_map(|key| {
            object
                .get(*key)
                .and_then(Value::as_str)
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .map(ToOwned::to_owned)
        })
        .ok_or_else(|| {
            news_actions_bad_gateway(&format!(
                "market-data helper response is missing {}",
                keys[0]
            ))
        })
}

fn project_news_entries(
    value: Option<&Value>,
) -> Result<Value, MarketDataNewsActionsReadSnapshotError> {
    let Some(value) = value else {
        return Ok(Value::Array(Vec::new()));
    };
    let Some(entries) = value.as_array() else {
        return if value.is_null() {
            // The Go facade always initializes the provider-neutral slice
            // before projecting the FeatureResult.  Preserve that public
            // empty-list shape even when an older sidecar serializes a nil
            // slice as JSON null.
            Ok(Value::Array(Vec::new()))
        } else {
            Err(news_actions_bad_gateway(
                "market-data helper response entries must be an array",
            ))
        };
    };
    entries
        .iter()
        .map(project_news_entry)
        .collect::<Result<Vec<_>, _>>()
        .map(Value::Array)
}

fn project_news_entry(value: &Value) -> Result<Value, MarketDataNewsActionsReadSnapshotError> {
    let object = value
        .as_object()
        .ok_or_else(|| news_actions_bad_gateway("news entry must be an object"))?;
    let mut entry = Map::new();
    for key in ["title", "link", "publisher", "summary"] {
        let value = object.get(key).cloned().unwrap_or(Value::Null);
        if !value.is_null() && !value.is_string() {
            return Err(news_actions_bad_gateway(&format!(
                "news entry {key} must be a string"
            )));
        }
        entry.insert(key.to_owned(), value);
    }
    let published = object
        .get("publishedAt")
        .or_else(|| object.get("published_at"))
        .cloned()
        .unwrap_or(Value::Null);
    if !published.is_null() {
        let Some(timestamp) = published.as_str().map(str::trim) else {
            return Err(news_actions_bad_gateway(
                "news entry publishedAt must be RFC3339",
            ));
        };
        if time::OffsetDateTime::parse(timestamp, &time::format_description::well_known::Rfc3339)
            .is_err()
        {
            return Err(news_actions_bad_gateway(
                "news entry publishedAt must be RFC3339",
            ));
        }
    }
    entry.insert("publishedAt".to_owned(), published);
    Ok(Value::Object(entry))
}

fn project_corporate_events(
    value: Option<&Value>,
) -> Result<Value, MarketDataNewsActionsReadSnapshotError> {
    // Corporate-actions responses are only successful when the helper
    // explicitly supplies the collection.  Treating a missing/null field as
    // an empty result hides malformed Futu/helper payloads and can make a
    // request look successful when no projection was performed.
    let value = value.ok_or_else(|| {
        news_actions_bad_gateway("market-data helper response is missing events")
    })?;
    let Some(events) = value.as_array() else {
        return Err(news_actions_bad_gateway(
            "market-data helper response events must be an array",
        ));
    };
    events
        .iter()
        .map(project_corporate_event)
        .collect::<Result<Vec<_>, _>>()
        .map(Value::Array)
}

fn project_corporate_event(value: &Value) -> Result<Value, MarketDataNewsActionsReadSnapshotError> {
    let object = value
        .as_object()
        .ok_or_else(|| news_actions_bad_gateway("corporate action event must be an object"))?;
    let kind = object
        .get("kind")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| news_actions_bad_gateway("corporate action event kind is required"))?;
    let kind = kind.to_ascii_lowercase();
    if !matches!(kind.as_str(), "dividend" | "split") {
        return Err(news_actions_bad_gateway(
            "corporate action event kind must be dividend or split",
        ));
    }
    let ex_date = object
        .get("exDate")
        .or_else(|| object.get("ex_date"))
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| news_actions_bad_gateway("corporate action event exDate is required"))?;
    let date_format = time::format_description::parse_borrowed::<1>("[year]-[month]-[day]")
        .map_err(|_| {
            news_actions_bad_gateway("corporate action event exDate must be YYYY-MM-DD")
        })?;
    time::Date::parse(ex_date, &date_format).map_err(|_| {
        news_actions_bad_gateway("corporate action event exDate must be YYYY-MM-DD")
    })?;
    let mut event = Map::new();
    event.insert("kind".to_owned(), Value::String(kind));
    event.insert("exDate".to_owned(), Value::String(ex_date.to_owned()));
    for output in ["amount", "ratio"] {
        let value = object.get(output).cloned().unwrap_or(Value::Null);
        if !value.is_null() && !value.is_number() {
            return Err(news_actions_bad_gateway(&format!(
                "corporate action event {output} must be a number"
            )));
        }
        event.insert(output.to_owned(), value);
    }
    Ok(Value::Object(event))
}

fn news_actions_bad_gateway(message: &str) -> MarketDataNewsActionsReadSnapshotError {
    MarketDataNewsActionsReadSnapshotError::Failed {
        status: 502,
        code: "BAD_GATEWAY".to_owned(),
        message: message.to_owned(),
        retry_after_seconds: None,
    }
}
