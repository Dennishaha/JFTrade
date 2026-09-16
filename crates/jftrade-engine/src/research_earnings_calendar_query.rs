//! Futu earnings-calendar query translation, OpenD 7-day chunking and merge.
//!
//! The public `GET /api/v1/research/calendars?operation=earnings` request is
//! translated into `Qot_GetEarningsCalendar` parameters here. OpenD rejects
//! ranges longer than seven days, so the engine splits the requested window,
//! calls the typed reader once per chunk and merges the de-duplicated rows.

use std::collections::BTreeSet;
use std::sync::Arc;

use jftrade_integration_futu::{
    EarningsCalendarBoundary, EarningsCalendarFilter, EarningsCalendarInterval,
    EarningsCalendarItem, EarningsCalendarQuery, EarningsCalendarQueryError,
    earnings_calendar_market_value,
};
use serde_json::{Map, Value, json};

use crate::product::ResearchReadSnapshotError;
use crate::product::product_production_ports::SharedTradeReadRuntime;
use crate::product::product_query::QueryMap;

/// One OpenD call must not span more than seven days.
pub(crate) const EARNINGS_CALENDAR_CHUNK_DAYS: i64 = 7;
/// The inclusive query window is capped at 42 days = six 7-day chunks.
pub(crate) const EARNINGS_CALENDAR_MAX_QUERY_DAYS: i64 = 42;
/// `EarningsCalendarIndicatorType_StockListType`.
const STOCK_LIST_TYPE_FIELD: i32 = 4;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
struct RangeParam {
    min_key: &'static str,
    max_key: &'static str,
    indicator_type: i32,
    percentage: bool,
    option_only: bool,
}

const RANGE_PARAMS: &[RangeParam] = &[
    RangeParam {
        min_key: "marketCapMin",
        max_key: "marketCapMax",
        indicator_type: 3,
        percentage: false,
        option_only: false,
    },
    RangeParam {
        min_key: "optionVolumeMin",
        max_key: "optionVolumeMax",
        indicator_type: 5,
        percentage: false,
        option_only: true,
    },
    RangeParam {
        min_key: "ivMin",
        max_key: "ivMax",
        indicator_type: 6,
        percentage: true,
        option_only: true,
    },
    RangeParam {
        min_key: "ivRankMin",
        max_key: "ivRankMax",
        indicator_type: 7,
        percentage: true,
        option_only: true,
    },
    RangeParam {
        min_key: "ivPercentileMin",
        max_key: "ivPercentileMax",
        indicator_type: 8,
        percentage: true,
        option_only: true,
    },
];

#[derive(Clone, Debug, PartialEq)]
pub(crate) struct EarningsCalendarDateChunk {
    pub(crate) begin: String,
    pub(crate) end: String,
}

/// Translates the public query into one typed OpenD request.
pub(super) fn read_futu_earnings_calendar(
    runtime: Option<&Arc<SharedTradeReadRuntime>>,
    query: &str,
) -> Result<Value, ResearchReadSnapshotError> {
    let runtime = runtime.ok_or_else(|| {
        ResearchReadSnapshotError::Unavailable(
            "Futu earnings-calendar research runtime is not configured".to_owned(),
        )
    })?;
    if !runtime.earnings_calendar_reader_available() {
        return Err(ResearchReadSnapshotError::Unavailable(
            "Futu OpenD earnings-calendar reader is not ready".to_owned(),
        ));
    }
    let query_map = QueryMap::parse(query)
        .map_err(|_| ResearchReadSnapshotError::Invalid("invalid URL escape".to_owned()))?;
    let market = query_map
        .get_first("market")
        .unwrap_or_default()
        .trim()
        .to_ascii_uppercase();
    let market_value = earnings_calendar_market_value(&market).ok_or_else(|| {
        ResearchReadSnapshotError::Invalid(format!(
            "earnings calendar does not support market {market:?}"
        ))
    })?;
    let params = translate_earnings_calendar_params(&query_map, &market)?;
    let chunks = earnings_calendar_date_chunks(&query_map)?;
    let mut entries = Vec::new();
    for chunk in &chunks {
        let request = EarningsCalendarQuery {
            market: market_value,
            sort_type: params.sort_type,
            begin_date: chunk.begin.clone(),
            end_date: chunk.end.clone(),
            // Reuse the same translated filter set for every chunk. The request
            // is cloned per chunk so a partially failed batch cannot leak
            // filters from the failed attempt into the retry.
            filters: params.filters.clone(),
        };
        let page = runtime
            .earnings_calendar(&request)
            .map_err(map_earnings_calendar_error)?;
        entries.extend(page.items);
    }
    let entries = deduplicate_earnings_calendar_entries(entries);
    let range_chunks = chunks.len();
    let begin_date = chunks
        .first()
        .map(|chunk| chunk.begin.clone())
        .unwrap_or_default();
    let end_date = chunks
        .last()
        .map(|chunk| chunk.end.clone())
        .unwrap_or_default();
    let total = entries.len();
    Ok(json!({
        "provider": {
            "brokerId": "futu",
            "securityFirm": "Futu/Moomoo via OpenD",
            "featureId": "research.calendar",
            "capability": "available",
            "selectionReason": "adapter_request",
            "resolvedAt": crate::product::product_production_ports::provider_now_rfc3339(),
            "asOf": crate::product::product_production_ports::provider_now_rfc3339(),
        },
        "asOf": crate::product::product_production_ports::provider_now_rfc3339(),
        "entries": entries
            .into_iter()
            .map(earnings_calendar_entry_value)
            .collect::<Vec<_>>(),
        "total": total,
        "metadata": {
            "rangeChunks": range_chunks,
            "beginDate": begin_date,
            "endDate": end_date,
        },
    }))
}

#[derive(Clone, Debug, PartialEq)]
struct TranslatedEarningsParams {
    sort_type: Option<i32>,
    filters: Vec<EarningsCalendarFilter>,
}

fn translate_earnings_calendar_params(
    query: &QueryMap,
    market: &str,
) -> Result<TranslatedEarningsParams, ResearchReadSnapshotError> {
    // Option-only semantics follow the Go baseline: HK and US are the only
    // markets whose earnings calendar exposes the option-derived filters.
    let option_market = matches!(market, "HK" | "US");
    let sort_value = query
        .get_first("sort")
        .unwrap_or_default()
        .trim()
        .to_ascii_lowercase();
    let sort_type = match sort_value.as_str() {
        "" => None,
        "hot" => Some(1),
        "market_cap" => Some(2),
        "option_volume" => Some(3),
        "iv" => Some(4),
        "iv_rank" => Some(5),
        "iv_percentile" => Some(6),
        other => {
            return Err(ResearchReadSnapshotError::Invalid(format!(
                "earnings calendar does not support sort {other:?}"
            )));
        }
    };
    if sort_type.is_some_and(|value| value > 2) && !option_market {
        return Err(ResearchReadSnapshotError::Invalid(format!(
            "earnings calendar sort {sort_value:?} is available only for HK/US"
        )));
    }

    let scope_value = query
        .get_first("stockScope")
        .unwrap_or_default()
        .trim()
        .to_ascii_lowercase();
    let scope_type = match scope_value.as_str() {
        "" | "all" => 0,
        "watchlist" => 1,
        "position" => 2,
        "special" => 3,
        other => {
            return Err(ResearchReadSnapshotError::Invalid(format!(
                "earnings calendar does not support stockScope {other:?}"
            )));
        }
    };
    let mut filters = Vec::new();
    if scope_type > 0 {
        filters.push(EarningsCalendarFilter {
            indicator_type: STOCK_LIST_TYPE_FIELD,
            value_list: vec![scope_type],
            interval: None,
        });
    }
    for definition in RANGE_PARAMS {
        let min = earnings_calendar_numeric_param(query, definition.min_key)?;
        let max = earnings_calendar_numeric_param(query, definition.max_key)?;
        let Some(filter) =
            earnings_calendar_range_filter(definition, min, max, option_market)?
        else {
            continue;
        };
        filters.push(filter);
    }
    Ok(TranslatedEarningsParams { sort_type, filters })
}

fn earnings_calendar_numeric_param(
    query: &QueryMap,
    key: &str,
) -> Result<Option<f64>, ResearchReadSnapshotError> {
    let Some(raw) = query.get_first(key).map(str::trim) else {
        return Ok(None);
    };
    if raw.is_empty() {
        // An empty string is the documented "not provided" form; Go keeps the
        // parameter but treats it as absent.
        return Ok(None);
    }
    let value = raw.parse::<f64>().map_err(|_| {
        ResearchReadSnapshotError::Invalid(format!(
            "earnings calendar {key} must be a finite number"
        ))
    })?;
    if !value.is_finite() {
        return Err(ResearchReadSnapshotError::Invalid(format!(
            "earnings calendar {key} must be a finite number"
        )));
    }
    if value < 0.0 {
        return Err(ResearchReadSnapshotError::Invalid(format!(
            "earnings calendar {key} must not be negative"
        )));
    }
    Ok(Some(value))
}

fn earnings_calendar_range_filter(
    definition: &RangeParam,
    min: Option<f64>,
    max: Option<f64>,
    option_market: bool,
) -> Result<Option<EarningsCalendarFilter>, ResearchReadSnapshotError> {
    if min.is_none() && max.is_none() {
        return Ok(None);
    }
    if definition.option_only && !option_market {
        return Err(ResearchReadSnapshotError::Invalid(format!(
            "earnings calendar filters {}/{} are available only for HK/US",
            definition.min_key, definition.max_key
        )));
    }
    if definition.percentage
        && (min.is_some_and(|value| value > 100.0) || max.is_some_and(|value| value > 100.0))
    {
        return Err(ResearchReadSnapshotError::Invalid(
            "earnings calendar percentage filters must not exceed 100".to_owned(),
        ));
    }
    if let (Some(min), Some(max)) = (min, max)
        && min > max
    {
        return Err(ResearchReadSnapshotError::Invalid(format!(
            "earnings calendar filter {} must not exceed {}",
            definition.min_key, definition.max_key
        )));
    }
    let boundary = |value: f64| EarningsCalendarBoundary {
        value,
        includes: true,
    };
    Ok(Some(EarningsCalendarFilter {
        indicator_type: definition.indicator_type,
        value_list: Vec::new(),
        interval: Some(EarningsCalendarInterval {
            min: min.map(boundary),
            max: max.map(boundary),
        }),
    }))
}

/// Splits the requested inclusive window into <=7-day segments. A missing
/// `beginDate` defaults to today; a missing `endDate` collapses to `beginDate`.
pub(crate) fn earnings_calendar_date_chunks(
    query: &QueryMap,
) -> Result<Vec<EarningsCalendarDateChunk>, ResearchReadSnapshotError> {
    let begin_value = query.get_first("beginDate").unwrap_or_default().trim();
    let end_value = query.get_first("endDate").unwrap_or_default().trim();
    if begin_value.is_empty() {
        if !end_value.is_empty() {
            return Err(ResearchReadSnapshotError::Invalid(
                "earnings calendar beginDate is required when endDate is provided".to_owned(),
            ));
        }
        let today = today_utc_date();
        return Ok(vec![EarningsCalendarDateChunk {
            begin: today.clone(),
            end: today,
        }]);
    }
    let begin = parse_calendar_date(begin_value, "beginDate")?;
    let end = if end_value.is_empty() {
        begin
    } else {
        parse_calendar_date(end_value, "endDate")?
    };
    if end < begin {
        return Err(ResearchReadSnapshotError::Invalid(
            "earnings calendar endDate must not precede beginDate".to_owned(),
        ));
    }
    if end.saturating_sub(begin) + 1 > EARNINGS_CALENDAR_MAX_QUERY_DAYS {
        return Err(ResearchReadSnapshotError::Invalid(format!(
            "earnings calendar range must not exceed {EARNINGS_CALENDAR_MAX_QUERY_DAYS} days"
        )));
    }
    let mut chunks = Vec::new();
    let mut cursor = begin;
    while cursor <= end {
        let chunk_end = (cursor + EARNINGS_CALENDAR_CHUNK_DAYS - 1).min(end);
        chunks.push(EarningsCalendarDateChunk {
            begin: format_calendar_date(cursor),
            end: format_calendar_date(chunk_end),
        });
        cursor += EARNINGS_CALENDAR_CHUNK_DAYS;
    }
    Ok(chunks)
}

/// Days since the Unix epoch for a validated `YYYY-MM-DD` string.
fn parse_calendar_date(value: &str, field: &str) -> Result<i64, ResearchReadSnapshotError> {
    let bytes = value.as_bytes();
    // Reject non-ASCII input before slicing, otherwise a multi-byte value such
    // as "ééééé" could split a UTF-8 boundary and panic instead of returning
    // the same invalid-input error the Go baseline produces.
    let invalid = bytes.len() != 10
        || bytes[4] != b'-'
        || bytes[7] != b'-'
        || !bytes
            .iter()
            .enumerate()
            .all(|(index, byte)| matches!(index, 4 | 7) || byte.is_ascii_digit());
    if invalid {
        return Err(ResearchReadSnapshotError::Invalid(format!(
            "earnings calendar {field} must use YYYY-MM-DD"
        )));
    }
    let year = value[0..4].parse::<i64>().ok();
    let month = value[5..7].parse::<i64>().ok();
    let day = value[8..10].parse::<i64>().ok();
    let (Some(year), Some(month), Some(day)) = (year, month, day) else {
        return Err(ResearchReadSnapshotError::Invalid(format!(
            "earnings calendar {field} must use YYYY-MM-DD"
        )));
    };
    if !(1..=12).contains(&month) || day < 1 || day > days_in_month(year, month) {
        return Err(ResearchReadSnapshotError::Invalid(format!(
            "earnings calendar {field} must use YYYY-MM-DD"
        )));
    }
    Ok(days_from_civil(year, month, day))
}

fn days_in_month(year: i64, month: i64) -> i64 {
    match month {
        1 | 3 | 5 | 7 | 8 | 10 | 12 => 31,
        4 | 6 | 9 | 11 => 30,
        2 if is_leap_year(year) => 29,
        2 => 28,
        _ => 0,
    }
}

fn is_leap_year(year: i64) -> bool {
    (year % 4 == 0 && year % 100 != 0) || year % 400 == 0
}

/// Howard Hinnant's days-from-civil algorithm, used instead of a time crate so
/// the arithmetic stays deterministic and timezone-independent.
fn days_from_civil(year: i64, month: i64, day: i64) -> i64 {
    let year = if month <= 2 { year - 1 } else { year };
    let era = if year >= 0 { year } else { year - 399 } / 400;
    let year_of_era = year - era * 400;
    let adjusted_month = month + if month > 2 { -3 } else { 9 };
    let day_of_year = (153 * adjusted_month + 2) / 5 + day - 1;
    let day_of_era =
        year_of_era * 365 + year_of_era / 4 - year_of_era / 100 + day_of_year;
    era * 146_097 + day_of_era - 719_468
}

fn civil_from_days(days: i64) -> (i64, i64, i64) {
    let days = days + 719_468;
    let era = if days >= 0 { days } else { days - 146_096 } / 146_097;
    let day_of_era = days - era * 146_097;
    let year_of_era =
        (day_of_era - day_of_era / 1460 + day_of_era / 36_524 - day_of_era / 146_096) / 365;
    let year = year_of_era + era * 400;
    let day_of_year = day_of_era - (365 * year_of_era + year_of_era / 4 - year_of_era / 100);
    let month_prime = (5 * day_of_year + 2) / 153;
    let day = day_of_year - (153 * month_prime + 2) / 5 + 1;
    let month = month_prime + if month_prime < 10 { 3 } else { -9 };
    (year + if month <= 2 { 1 } else { 0 }, month, day)
}

fn format_calendar_date(days: i64) -> String {
    let (year, month, day) = civil_from_days(days);
    format!("{year:04}-{month:02}-{day:02}")
}

fn today_utc_date() -> String {
    let now = time::OffsetDateTime::now_utc();
    format!(
        "{:04}-{:02}-{:02}",
        now.year(),
        u8::from(now.month()),
        now.day()
    )
}

/// De-duplicates on `eventDate|instrumentId|symbol`, matching the Go baseline.
/// Rows without any identity fall back to their full serialized form so two
/// genuinely identical anonymous rows still collapse.
fn deduplicate_earnings_calendar_entries(
    entries: Vec<EarningsCalendarItem>,
) -> Vec<EarningsCalendarItem> {
    let mut seen = BTreeSet::new();
    let mut result = Vec::with_capacity(entries.len());
    for entry in entries {
        let key = format!(
            "{}\u{0}{}\u{0}{}",
            entry.earnings_date.as_deref().unwrap_or_default(),
            entry.security.instrument_id,
            entry.security.code
        );
        let key = if key == "\u{0}\u{0}" {
            serde_json::to_string(&earnings_calendar_entry_value(entry.clone()))
                .unwrap_or_default()
        } else {
            key
        };
        if seen.insert(key) {
            result.push(entry);
        }
    }
    result
}

fn earnings_calendar_entry_value(entry: EarningsCalendarItem) -> Value {
    let mut object = match serde_json::to_value(&entry) {
        Ok(Value::Object(object)) => object,
        _ => Map::new(),
    };
    // The public calendar contract keys rows by `instrumentId`/`symbol` and
    // carries the event identity (`calendarType`, `eventDate`).
    let instrument_id = entry.security.instrument_id.clone();
    object.insert("instrumentId".to_owned(), json!(instrument_id));
    object.insert("market".to_owned(), json!(entry.security.market));
    object.insert("symbol".to_owned(), json!(entry.security.code));
    object.insert("calendarType".to_owned(), json!("earnings"));
    if let Some(event_date) = entry.earnings_date.as_deref() {
        object.insert("eventDate".to_owned(), json!(event_date));
        object
            .entry("date".to_owned())
            .or_insert_with(|| json!(event_date));
    }
    Value::Object(object)
}

fn map_earnings_calendar_error(
    error: EarningsCalendarQueryError,
) -> ResearchReadSnapshotError {
    match error {
        EarningsCalendarQueryError::InvalidQuery(message) => {
            ResearchReadSnapshotError::Invalid(message)
        }
        EarningsCalendarQueryError::Session(_)
        | EarningsCalendarQueryError::MissingS2c => ResearchReadSnapshotError::Unavailable(
            "Futu OpenD earnings-calendar reader is not ready".to_owned(),
        ),
        EarningsCalendarQueryError::Rejected {
            ret_type,
            err_code,
            message,
        } => ResearchReadSnapshotError::Failed {
            status: 502,
            code: "OPEND_EARNINGS_CALENDAR_FAILED".to_owned(),
            message: format!(
                "OpenD earnings-calendar request rejected ({ret_type}/{err_code}): {message}"
            ),
            retry_after_seconds: None,
        },
        other => ResearchReadSnapshotError::Failed {
            status: 502,
            code: "OPEND_EARNINGS_CALENDAR_FAILED".to_owned(),
            message: other.to_string(),
            retry_after_seconds: None,
        },
    }
}

#[cfg(test)]
#[path = "research_earnings_calendar_query_tests.rs"]
mod tests;
