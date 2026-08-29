//! Validation and wire projection for the provider news/actions endpoints.
//!
//! The Python sidecar emits its Pydantic models with snake_case names.  The
//! product HTTP contract remains camelCase, so this module is the boundary
//! where helper payloads are validated and projected.

use serde_json::{Map, Value};

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
    let Some(value) = value else {
        return Ok(Value::Array(Vec::new()));
    };
    let Some(events) = value.as_array() else {
        return if value.is_null() {
            // See project_news_entries: a nil sidecar slice is a valid empty
            // result at the product boundary, not a JSON null collection.
            Ok(Value::Array(Vec::new()))
        } else {
            Err(news_actions_bad_gateway(
                "market-data helper response events must be an array",
            ))
        };
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
