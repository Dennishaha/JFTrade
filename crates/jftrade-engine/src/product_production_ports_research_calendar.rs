//! Production adapters for AKShare research calendar and macro routes.
//!
//! The market-data helper owns these feeds.  The public API keeps the broker
//! `FeatureResult` envelope used by the Go product-feature service, while the
//! helper payloads are validated before projection so malformed upstream data
//! cannot be presented as a successful empty result.

use std::thread;

use jftrade_integration_marketdata_helper::{HelperClient, HttpAdapterError};
use jftrade_settings::MarketDataProvider;
use serde_json::{Map, Value, json};

use crate::product::ResearchReadSnapshotError;
use crate::product::product_query::QueryMap;

const DEFAULT_MACRO_LIMIT: usize = 100;
const MAX_MACRO_LIMIT: usize = 500;

pub(crate) fn read_market_calendar(
    provider: MarketDataProvider,
    helper_ready: bool,
    helper: Option<&HelperClient>,
    path: &str,
    query: &str,
) -> Result<Value, ResearchReadSnapshotError> {
    let query = QueryMap::parse(query)
        .map_err(|_| ResearchReadSnapshotError::Invalid("invalid URL escape".to_owned()))?;
    if provider == MarketDataProvider::Futu {
        return Err(unavailable(
            "Futu research calendar/macro runtime is not ready",
        ));
    }
    if provider != MarketDataProvider::Akshare {
        return Err(capability("research calendar/macro", "provider"));
    }
    if !helper_ready {
        return Err(unavailable("market-data helper is not ready"));
    }
    let helper = helper.ok_or_else(|| unavailable("market-data helper is not configured"))?;
    match path {
        "/api/v1/research/calendars" => read_calendar(helper, &query),
        "/api/v1/research/macro" => read_macro(helper, &query),
        _ => Err(unavailable("unsupported market calendar route")),
    }
}

fn read_calendar(
    helper: &HelperClient,
    query: &QueryMap,
) -> Result<Value, ResearchReadSnapshotError> {
    let operation = query
        .get_first("operation")
        .unwrap_or("")
        .trim()
        .to_ascii_lowercase();
    match operation.as_str() {
        "earnings" => {
            let (begin, end) = date_window(query)?;
            let payload = fetch_json(
                helper,
                &["calendar", "earnings"],
                vec![("begin_date", begin.clone()), ("end_date", end.clone())],
            )?;
            let entries = calendar_entries(&payload, "earnings", |entry| {
                let id = required_text(entry, "instrument_id")?;
                let mut output = identity(entry, id);
                copy_text(entry, &mut output, "name", "name");
                copy_text(entry, &mut output, "symbol", "symbol");
                copy_text(entry, &mut output, "event_date", "eventDate");
                copy_text(entry, &mut output, "period_text", "periodText");
                copy_value(entry, &mut output, "market_cap", "marketCap");
                copy_value(entry, &mut output, "price", "price");
                Ok(Value::Object(output))
            })?;
            Ok(feature_result(
                "research.calendar",
                entries,
                "akshare-calendar",
            ))
        }
        "dividends" => {
            let date = required_date(query, "date")?;
            let payload = fetch_json(helper, &["calendar", "dividends"], vec![("date", date)])?;
            let entries = calendar_entries(&payload, "dividends", |entry| {
                let id = required_text(entry, "instrument_id")?;
                let mut output = identity(entry, id);
                copy_text(entry, &mut output, "name", "name");
                copy_text(entry, &mut output, "symbol", "symbol");
                copy_text(entry, &mut output, "statement", "statement");
                copy_text(entry, &mut output, "ex_date", "exDate");
                copy_text(entry, &mut output, "record_date", "recordDate");
                copy_text(entry, &mut output, "payable_date", "dividendPayableDate");
                Ok(Value::Object(output))
            })?;
            Ok(feature_result(
                "research.calendar",
                entries,
                "akshare-calendar",
            ))
        }
        "economic" => {
            let (begin, end) = date_window(query)?;
            let payload = fetch_json(
                helper,
                &["calendar", "economic"],
                vec![("begin_date", begin), ("end_date", end)],
            )?;
            let entries = calendar_entries(&payload, "economic", |entry| {
                let event_id = required_text(entry, "event_id")?;
                let mut output = Map::new();
                output.insert("eventId".to_owned(), json!(event_id));
                for (from, to) in [("title", "title"), ("region", "region")] {
                    copy_text(entry, &mut output, from, to);
                }
                if let Some(timestamp) = entry
                    .get("event_timestamp")
                    .and_then(Value::as_i64)
                    .filter(|timestamp| *timestamp != 0)
                {
                    output.insert("eventTimestamp".to_owned(), json!(timestamp));
                    let moment = time::OffsetDateTime::from_unix_timestamp(timestamp)
                        .map_err(|_| bad_gateway("economic event timestamp is invalid"))?
                        .to_offset(time::UtcOffset::from_hms(8, 0, 0).map_err(|_| {
                            bad_gateway("economic event display timezone is invalid")
                        })?);
                    output.insert("eventDate".to_owned(), json!(moment.date().to_string()));
                    output.insert(
                        "eventTime".to_owned(),
                        json!(format!("{:02}:{:02}", moment.hour(), moment.minute())),
                    );
                } else {
                    copy_text(entry, &mut output, "event_date", "eventDate");
                }
                for (from, to) in [
                    ("previous_value", "previousValue"),
                    ("forecast_value", "forecastValue"),
                    ("actual_value", "actualValue"),
                ] {
                    copy_text(entry, &mut output, from, to);
                }
                copy_value(entry, &mut output, "importance", "importance");
                Ok(Value::Object(output))
            })?;
            Ok(feature_result(
                "research.calendar",
                entries,
                "akshare-calendar",
            ))
        }
        "ipos" => {
            let payload = fetch_json(helper, &["calendar", "ipos"], Vec::new())?;
            let entries = calendar_entries(&payload, "ipos", |entry| {
                let id = required_text(entry, "instrument_id")?;
                let mut output = identity(entry, id);
                copy_text(entry, &mut output, "name", "name");
                copy_text(entry, &mut output, "symbol", "symbol");
                copy_text(entry, &mut output, "status", "status");
                copy_text(entry, &mut output, "listing_date", "listingDate");
                for (from, to) in [
                    ("issue_volume", "issueVolume"),
                    ("issue_price", "issuePrice"),
                    ("issue_price_min", "issuePriceMin"),
                    ("issue_price_max", "issuePriceMax"),
                ] {
                    copy_value(entry, &mut output, from, to);
                }
                Ok(Value::Object(output))
            })?;
            Ok(feature_result(
                "research.calendar",
                entries,
                "akshare-calendar",
            ))
        }
        "trade_dates" => Err(capability("research.calendar", "trade_dates")),
        _ => Err(capability("research.calendar", &operation)),
    }
}

fn read_macro(helper: &HelperClient, query: &QueryMap) -> Result<Value, ResearchReadSnapshotError> {
    let operation = query
        .get_first("operation")
        .unwrap_or("")
        .trim()
        .to_ascii_lowercase();
    match operation.as_str() {
        "indicators" => {
            let payload = fetch_json(helper, &["macro", "indicators"], Vec::new())?;
            let object = payload
                .as_object()
                .ok_or_else(|| bad_gateway("macro indicators response must be an object"))?;
            let categories = object
                .get("categories")
                .and_then(Value::as_array)
                .ok_or_else(|| bad_gateway("macro indicators response is missing categories"))?;
            let mut entries = Vec::with_capacity(categories.len());
            for category in categories {
                let category = category
                    .as_object()
                    .ok_or_else(|| bad_gateway("macro category must be an object"))?;
                let name = required_text(category, "category_name")?;
                let indicators = category
                    .get("indicators")
                    .and_then(Value::as_array)
                    .ok_or_else(|| bad_gateway("macro category is missing indicators"))?;
                let mut list = Vec::with_capacity(indicators.len());
                for indicator in indicators {
                    let indicator = indicator
                        .as_object()
                        .ok_or_else(|| bad_gateway("macro indicator must be an object"))?;
                    let id = required_text(indicator, "indicator_id")?;
                    let name = required_text(indicator, "name")?;
                    let region = required_text(indicator, "region")?;
                    let unit = required_text(indicator, "unit")?;
                    let frequency = required_text(indicator, "frequency")?;
                    let unit_type = required_integer(indicator, "unit_type")?;
                    let mut projected = Map::new();
                    projected.insert("indicatorId".to_owned(), json!(id));
                    projected.insert("name".to_owned(), json!(name));
                    projected.insert("region".to_owned(), json!(region));
                    projected.insert("unit".to_owned(), json!(unit));
                    projected.insert("frequency".to_owned(), json!(frequency));
                    projected.insert("unitType".to_owned(), json!(unit_type));
                    list.push(Value::Object(projected));
                }
                entries.push(json!({"categoryName": name, "indicatorList": list}));
            }
            Ok(feature_result("research.macro", entries, "akshare-macro"))
        }
        "indicator_history" => {
            let indicator_id = query
                .get_first("indicatorId")
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .ok_or_else(|| {
                    capability("research.macro", "indicator_history (missing indicatorId)")
                })?;
            let limit = macro_limit(query)?;
            let payload = fetch_json(
                helper,
                &["macro", "indicator-history"],
                vec![
                    ("indicator_id", indicator_id.to_owned()),
                    ("limit", limit.to_string()),
                ],
            )?;
            let object = payload
                .as_object()
                .ok_or_else(|| bad_gateway("macro history response must be an object"))?;
            let echoed = required_text(object, "indicator_id")?;
            if echoed != indicator_id {
                return Err(bad_gateway(
                    "macro history indicator_id does not match request",
                ));
            }
            let points = object
                .get("entries")
                .and_then(Value::as_array)
                .ok_or_else(|| bad_gateway("macro history response is missing entries"))?;
            let mut entries = Vec::with_capacity(points.len());
            for point in points {
                let point = point
                    .as_object()
                    .ok_or_else(|| bad_gateway("macro history entry must be an object"))?;
                let mut projected = Map::new();
                copy_text_required(point, &mut projected, "data_time", "dataTime")?;
                let unit = required_text(point, "unit")?;
                let unit_type = required_integer(point, "unit_type")?;
                projected.insert("unit".to_owned(), json!(unit));
                projected.insert("unitType".to_owned(), json!(unit_type));
                for (from, to) in [
                    ("value", "value"),
                    ("predict_value", "predictValue"),
                    ("previous_value", "previousValue"),
                ] {
                    copy_optional_number(point, &mut projected, from, to)?;
                }
                entries.push(Value::Object(projected));
            }
            Ok(feature_result("research.macro", entries, "akshare-macro"))
        }
        "fed_target_rate" | "fed_dot_plot" => Err(capability("research.macro", &operation)),
        _ => Err(capability("research.macro", &operation)),
    }
}

fn fetch_json(
    helper: &HelperClient,
    segments: &[&str],
    query: Vec<(&str, String)>,
) -> Result<Value, ResearchReadSnapshotError> {
    let helper = helper.clone();
    let segments = segments
        .iter()
        .map(|part| (*part).to_owned())
        .collect::<Vec<_>>();
    let query = query
        .into_iter()
        .map(|(key, value)| (key.to_owned(), value))
        .collect::<Vec<_>>();
    let result = thread::spawn(move || {
        let runtime = tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .map_err(|error| HttpAdapterError::Unavailable(error.to_string()))?;
        let segments = segments.iter().map(String::as_str).collect::<Vec<_>>();
        let query = query
            .iter()
            .map(|(key, value)| (key.as_str(), value.as_str()))
            .collect::<Vec<_>>();
        runtime.block_on(helper.get_provider_json_with_query::<Value>("akshare", &segments, &query))
    })
    .join()
    .map_err(|_| unavailable("research helper task panicked"))?;
    result.map_err(map_helper_error)
}

fn map_helper_error(error: HttpAdapterError) -> ResearchReadSnapshotError {
    match error {
        HttpAdapterError::Remote {
            status,
            code,
            message,
            retry_after_seconds,
        } => ResearchReadSnapshotError::Failed {
            status,
            code: if code.is_empty() {
                "BAD_GATEWAY".to_owned()
            } else {
                code
            },
            message,
            retry_after_seconds,
        },
        HttpAdapterError::Timeout => ResearchReadSnapshotError::Failed {
            status: 504,
            code: "GATEWAY_TIMEOUT".to_owned(),
            message: "market-data helper request timed out".to_owned(),
            retry_after_seconds: None,
        },
        HttpAdapterError::InvalidResponse(message) => ResearchReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message,
            retry_after_seconds: None,
        },
        other => unavailable(other.to_string()),
    }
}

fn calendar_entries<F>(
    payload: &Value,
    kind: &str,
    project: F,
) -> Result<Vec<Value>, ResearchReadSnapshotError>
where
    F: Fn(&Map<String, Value>) -> Result<Value, ResearchReadSnapshotError>,
{
    let object = payload
        .as_object()
        .ok_or_else(|| bad_gateway(format!("{kind} calendar response must be an object")))?;
    let entries = object
        .get("entries")
        .and_then(Value::as_array)
        .ok_or_else(|| bad_gateway(format!("{kind} calendar response is missing entries")))?;
    entries
        .iter()
        .map(|entry| {
            let entry = entry
                .as_object()
                .ok_or_else(|| bad_gateway(format!("{kind} calendar entry must be an object")))?;
            project(entry)
        })
        .collect()
}

fn feature_result(feature_id: &str, entries: Vec<Value>, source: &str) -> Value {
    let as_of = crate::product::product_production_ports::provider_now_rfc3339();
    let total = entries.len();
    json!({
        "provider": {
            "brokerId": "akshare",
            "featureId": feature_id,
            "capability": "available",
            "selectionReason": "embedded-market-data-provider",
            "resolvedAt": as_of,
            "asOf": as_of,
        },
        "asOf": as_of,
        "entries": entries,
        "hasMore": false,
        "total": total,
        "metadata": {"source": source},
    })
}

fn date_window(query: &QueryMap) -> Result<(String, String), ResearchReadSnapshotError> {
    let begin = required_date(query, "beginDate")?;
    let end = required_date(query, "endDate")?;
    if begin > end {
        return Err(ResearchReadSnapshotError::Invalid(
            "beginDate must not be after endDate".to_owned(),
        ));
    }
    Ok((begin, end))
}

fn required_date(query: &QueryMap, key: &str) -> Result<String, ResearchReadSnapshotError> {
    let value = query
        .get_first(key)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| {
            ResearchReadSnapshotError::Invalid(format!("{key} must be a YYYY-MM-DD date"))
        })?;
    let valid = if value.is_ascii()
        && value.len() == 10
        && value.as_bytes()[4] == b'-'
        && value.as_bytes()[7] == b'-'
    {
        match (
            value[..4].parse::<u16>().ok(),
            value[5..7].parse::<u8>().ok(),
            value[8..10].parse::<u8>().ok(),
        ) {
            (Some(year), Some(month), Some(day)) if year >= 1 && (1..=12).contains(&month) => {
                let leap = year % 4 == 0 && (year % 100 != 0 || year % 400 == 0);
                let days = match month {
                    2 if leap => 29,
                    2 => 28,
                    4 | 6 | 9 | 11 => 30,
                    _ => 31,
                };
                (1..=days).contains(&day)
            }
            _ => false,
        }
    } else {
        false
    };
    valid.then(|| value.to_owned()).ok_or_else(|| {
        ResearchReadSnapshotError::Invalid(format!("{key} must be a YYYY-MM-DD date"))
    })
}

fn macro_limit(query: &QueryMap) -> Result<usize, ResearchReadSnapshotError> {
    // The public Go handler treats a positive pageSize as authoritative.  A
    // missing, zero, negative, or malformed pageSize falls back to the legacy
    // limit parameter, then to the default.  Keep this compatibility behavior
    // while still clamping the final value before it reaches the helper.
    let positive = |value: Option<&str>| {
        value
            .and_then(|value| value.trim().parse::<i64>().ok())
            .filter(|value| *value > 0)
            .and_then(|value| usize::try_from(value).ok())
    };
    let parsed = positive(query.get_first("pageSize"))
        .or_else(|| positive(query.get_first("limit")))
        .unwrap_or(DEFAULT_MACRO_LIMIT);
    Ok(parsed.clamp(1, MAX_MACRO_LIMIT))
}

fn required_text<'a>(
    object: &'a Map<String, Value>,
    key: &str,
) -> Result<&'a str, ResearchReadSnapshotError> {
    object
        .get(key)
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| bad_gateway(format!("research response is missing {key}")))
}

fn required_integer(
    object: &Map<String, Value>,
    key: &str,
) -> Result<i64, ResearchReadSnapshotError> {
    object
        .get(key)
        .and_then(Value::as_i64)
        .ok_or_else(|| bad_gateway(format!("research response is missing or invalid {key}")))
}

fn copy_optional_number(
    source: &Map<String, Value>,
    target: &mut Map<String, Value>,
    from: &str,
    to: &str,
) -> Result<(), ResearchReadSnapshotError> {
    let Some(value) = source.get(from).filter(|value| !value.is_null()) else {
        return Ok(());
    };
    if !value.is_number() {
        return Err(bad_gateway(format!(
            "research response field {from} must be numeric"
        )));
    }
    target.insert(to.to_owned(), value.clone());
    Ok(())
}

fn identity(entry: &Map<String, Value>, instrument_id: &str) -> Map<String, Value> {
    let id = instrument_id.trim().to_ascii_uppercase();
    let mut output = Map::new();
    output.insert("instrumentId".to_owned(), json!(id.clone()));
    if let Some((market, symbol)) = id.split_once('.') {
        output.insert("market".to_owned(), json!(market));
        output.insert("symbol".to_owned(), json!(symbol));
    } else {
        copy_text(entry, &mut output, "symbol", "symbol");
    }
    output
}

fn copy_text(source: &Map<String, Value>, target: &mut Map<String, Value>, from: &str, to: &str) {
    if let Some(value) = source
        .get(from)
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
    {
        target.insert(to.to_owned(), json!(value));
    }
}

fn copy_text_required(
    source: &Map<String, Value>,
    target: &mut Map<String, Value>,
    from: &str,
    to: &str,
) -> Result<(), ResearchReadSnapshotError> {
    let value = required_text(source, from)?;
    target.insert(to.to_owned(), json!(value));
    Ok(())
}

fn copy_value(source: &Map<String, Value>, target: &mut Map<String, Value>, from: &str, to: &str) {
    if let Some(value) = source.get(from).filter(|value| !value.is_null()) {
        target.insert(to.to_owned(), value.clone());
    }
}

fn unavailable(message: impl Into<String>) -> ResearchReadSnapshotError {
    ResearchReadSnapshotError::Unavailable(message.into())
}

fn capability(feature: &str, operation: &str) -> ResearchReadSnapshotError {
    ResearchReadSnapshotError::Failed {
        status: 409,
        code: "CAPABILITY_UNAVAILABLE".to_owned(),
        message: format!("{feature} operation {operation:?} is unavailable"),
        retry_after_seconds: None,
    }
}

fn bad_gateway(message: impl Into<String>) -> ResearchReadSnapshotError {
    ResearchReadSnapshotError::Failed {
        status: 502,
        code: "BAD_GATEWAY".to_owned(),
        message: message.into(),
        retry_after_seconds: None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn macro_history_uses_page_size_before_legacy_limit() {
        let query = QueryMap::parse("pageSize=60&limit=5").expect("query");
        assert_eq!(macro_limit(&query).expect("limit"), 60);
    }

    #[test]
    fn macro_history_limit_is_bounded_and_defaults() {
        let default = QueryMap::parse("").expect("query");
        assert_eq!(macro_limit(&default).expect("default"), DEFAULT_MACRO_LIMIT);

        let oversized = QueryMap::parse("pageSize=9999").expect("query");
        assert_eq!(macro_limit(&oversized).expect("clamp"), MAX_MACRO_LIMIT);

        let zero = QueryMap::parse("pageSize=0").expect("query");
        assert_eq!(macro_limit(&zero).expect("default"), DEFAULT_MACRO_LIMIT);

        let legacy = QueryMap::parse("pageSize=0&limit=5").expect("query");
        assert_eq!(macro_limit(&legacy).expect("legacy fallback"), 5);
    }

    #[test]
    fn macro_projection_rejects_missing_or_malformed_typed_fields() {
        let mut indicator = Map::new();
        indicator.insert("unit_type".to_owned(), json!("percent"));
        assert!(matches!(
            required_integer(&indicator, "unit_type"),
            Err(ResearchReadSnapshotError::Failed { status: 502, .. })
        ));

        let mut point = Map::new();
        point.insert("value".to_owned(), json!("1.2"));
        let mut projected = Map::new();
        assert!(matches!(
            copy_optional_number(&point, &mut projected, "value", "value"),
            Err(ResearchReadSnapshotError::Failed { status: 502, .. })
        ));
    }
}
