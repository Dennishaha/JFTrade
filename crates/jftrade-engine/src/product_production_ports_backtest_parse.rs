use crate::product::product_backtests_write_port::BacktestsWritePortError;
use jftrade_settings::MarketDataProvider;
use serde_json::Value;

pub(crate) const DEFAULT_EXECUTION_MODEL: &str = "conservative-bar-v1";

#[derive(Clone, Debug)]
pub(crate) struct ParsedBacktestStart {
    pub(crate) symbol: String,
    pub(crate) interval: String,
    pub(crate) rehab_type: String,
    pub(crate) session_scope: String,
    pub(crate) execution_model: String,
    pub(crate) start_time_ms: i64,
    pub(crate) end_time_ms: i64,
}

pub(crate) fn provider_id(provider: MarketDataProvider) -> &'static str {
    match provider {
        MarketDataProvider::Futu => "futu",
        MarketDataProvider::Yfinance => "yfinance",
        MarketDataProvider::Akshare => "akshare",
    }
}

pub(crate) fn parse_start_timestamp(value: &str) -> Result<i64, BacktestsWritePortError> {
    parse_backtest_timestamp(value, false)
}

pub(crate) fn parse_end_timestamp(value: &str) -> Result<i64, BacktestsWritePortError> {
    parse_backtest_timestamp(value, true)
}

/// Normalize the public execution-model name at the production request
/// boundary. This mirrors `pkg/backtest.NormalizeExecutionModelName`: omitted
/// or blank values select the conservative bar model, while every other value
/// is rejected without silently falling back to a different matcher.
pub(crate) fn normalize_execution_model_name(
    value: &str,
) -> Result<String, BacktestsWritePortError> {
    let normalized = value.trim().to_ascii_lowercase();
    if normalized.is_empty() || normalized == DEFAULT_EXECUTION_MODEL {
        return Ok(DEFAULT_EXECUTION_MODEL.to_owned());
    }
    Err(BacktestsWritePortError::BadRequest(format!(
        "unsupported backtest executionModel: {normalized}"
    )))
}

/// Return a payload carrying the canonical execution model selected during
/// request validation. The payload is cloned so callers can use one copy for
/// the worker's private execution input and another for persisted/public
/// request metadata without mutating the caller-owned JSON value.
pub(crate) fn with_execution_model(
    payload: &serde_json::Value,
    execution_model: &str,
) -> Result<serde_json::Value, BacktestsWritePortError> {
    let mut normalized = payload.clone();
    let object = normalized.as_object_mut().ok_or_else(|| {
        BacktestsWritePortError::BadRequest("invalid backtest request".to_owned())
    })?;
    object.insert(
        "executionModel".to_owned(),
        serde_json::Value::String(execution_model.to_owned()),
    );
    Ok(normalized)
}

fn parse_backtest_timestamp(value: &str, date_end: bool) -> Result<i64, BacktestsWritePortError> {
    let value = value.trim();
    let parsed = if let Ok(parsed) =
        time::OffsetDateTime::parse(value, &time::format_description::well_known::Rfc3339)
    {
        parsed
    } else {
        let format = time::format_description::parse_borrowed::<2>("[year]-[month]-[day]")
            .map_err(|_| BacktestsWritePortError::BadRequest("invalid backtest time".to_owned()))?;
        let mut date = time::Date::parse(value, &format)
            .map_err(|_| BacktestsWritePortError::BadRequest("invalid backtest time".to_owned()))?;
        if date_end {
            date = date.next_day().ok_or_else(|| {
                BacktestsWritePortError::BadRequest("backtest time is out of range".to_owned())
            })?;
        }
        time::PrimitiveDateTime::new(date, time::Time::MIDNIGHT).assume_utc()
    };
    parsed
        .unix_timestamp_nanos()
        .checked_div(1_000_000)
        .and_then(|value| i64::try_from(value).ok())
        .ok_or_else(|| {
            BacktestsWritePortError::BadRequest("backtest time is out of range".to_owned())
        })
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub(crate) struct ResolvedBacktestTimeRange {
    pub(crate) start_time_ms: i64,
    pub(crate) end_time_ms: i64,
    pub(crate) start_time: String,
    pub(crate) end_time: String,
    pub(crate) start_date: String,
    pub(crate) end_date: String,
    pub(crate) market_timezone: String,
}

/// Market timezone used by the reference resolver.  Profiles that the frozen
/// baseline does not know keep UTC so already-normalized symbols still run.
pub(crate) fn backtest_market_timezone(symbol: &str) -> &'static str {
    match symbol.split('.').next().unwrap_or("") {
        "US" => "America/New_York",
        "HK" => "Asia/Hong_Kong",
        "SH" | "SZ" | "CN" => "Asia/Shanghai",
        _ => "UTC",
    }
}

/// Resolve the public start-time inputs into one canonical range.
///
/// Explicit market dates win over timestamps, exactly like the frozen
/// baseline: they are read as local midnights of the symbol market so a DST
/// day stays 23 or 25 hours long, and the inclusive end is the next local
/// midnight minus one nanosecond.
pub(crate) fn resolve_backtest_time_range(
    symbol: &str,
    start_date: &str,
    end_date: &str,
    start_time: &str,
    end_time: &str,
) -> Result<Option<ResolvedBacktestTimeRange>, BacktestsWritePortError> {
    let start_date = start_date.trim();
    let end_date = end_date.trim();
    let start_time = start_time.trim();
    let end_time = end_time.trim();
    if start_date.is_empty() && end_date.is_empty() && start_time.is_empty() && end_time.is_empty() {
        return Ok(None);
    }
    let market_timezone = backtest_market_timezone(symbol).to_owned();
    if !start_date.is_empty() || !end_date.is_empty() {
        if start_date.is_empty() || end_date.is_empty() {
            return Err(BacktestsWritePortError::BadRequest(
                "startDate and endDate must be provided together".to_owned(),
            ));
        }
        let start = market_date_value(start_date)?;
        let end = market_date_value(end_date)?;
        let next_day = end.next_day().ok_or_else(|| {
            BacktestsWritePortError::BadRequest("invalid endDate, use YYYY-MM-DD format".to_owned())
        })?;
        let start_nanos = local_midnight_nanos(start, &market_timezone)?;
        let end_nanos = local_midnight_nanos(next_day, &market_timezone)?
            .checked_sub(1)
            .ok_or_else(|| {
                BacktestsWritePortError::BadRequest("backtest time is out of range".to_owned())
            })?;
        if end_nanos < start_nanos {
            return Err(BacktestsWritePortError::BadRequest(
                "endDate must not be before startDate".to_owned(),
            ));
        }
        return Ok(Some(ResolvedBacktestTimeRange {
            start_time_ms: nanos_to_millis(start_nanos)?,
            end_time_ms: nanos_to_millis(end_nanos)?,
            start_time: format_utc_nanos(start_nanos)?,
            end_time: format_utc_nanos(end_nanos)?,
            start_date: start_date.to_owned(),
            end_date: end_date.to_owned(),
            market_timezone,
        }));
    }
    if start_time.is_empty() || end_time.is_empty() {
        return Err(BacktestsWritePortError::BadRequest(
            "startTime and endTime are required".to_owned(),
        ));
    }
    let start = parse_backtest_instant(start_time, "startTime")?;
    let end = parse_backtest_instant(end_time, "endTime")?;
    Ok(Some(ResolvedBacktestTimeRange {
        start_time_ms: nanos_to_millis(start.unix_timestamp_nanos())?,
        end_time_ms: nanos_to_millis(end.unix_timestamp_nanos())?,
        start_time: format_utc_nanos(start.unix_timestamp_nanos())?,
        end_time: format_utc_nanos(end.unix_timestamp_nanos())?,
        start_date: String::new(),
        end_date: String::new(),
        market_timezone,
    }))
}

/// Rewrite a persisted start payload with the canonical range fields the
/// baseline exposes on the run request.  Absent labels stay absent, matching
/// the `omitempty` wire shape.
pub(crate) fn with_normalized_time_range(
    payload: &Value,
    symbol: &str,
) -> Result<Value, BacktestsWritePortError> {
    let text = |name: &str| {
        payload
            .get(name)
            .and_then(Value::as_str)
            .unwrap_or("")
            .trim()
            .to_owned()
    };
    let Some(resolved) = resolve_backtest_time_range(
        symbol,
        &text("startDate"),
        &text("endDate"),
        &text("startTime"),
        &text("endTime"),
    )?
    else {
        return Ok(payload.clone());
    };
    let mut normalized = payload.clone();
    let Some(object) = normalized.as_object_mut() else {
        return Ok(normalized);
    };
    object.insert(
        "startTime".to_owned(),
        Value::String(resolved.start_time),
    );
    object.insert("endTime".to_owned(), Value::String(resolved.end_time));
    object.insert(
        "marketTimezone".to_owned(),
        Value::String(resolved.market_timezone),
    );
    if resolved.start_date.is_empty() {
        object.remove("startDate");
    } else {
        object.insert("startDate".to_owned(), Value::String(resolved.start_date));
    }
    if resolved.end_date.is_empty() {
        object.remove("endDate");
    } else {
        object.insert("endDate".to_owned(), Value::String(resolved.end_date));
    }
    Ok(normalized)
}

/// Chart representation of the run request.  Empty, legacy and unknown
/// values keep the historical standard-candle behaviour, matching the
/// reference `chart.NormalizeChartType`.
pub(crate) fn with_normalized_chart_type(
    payload: &Value,
) -> Result<Value, BacktestsWritePortError> {
    let requested = payload.get("chartType");
    let normalized = match requested {
        None | Some(Value::Null) => "standard",
        Some(Value::String(value)) if value.trim().eq_ignore_ascii_case("heikinashi") => {
            "heikinashi"
        }
        Some(Value::String(_)) => "standard",
        Some(_) => {
            return Err(BacktestsWritePortError::BadRequest(
                "chartType must be a string".to_owned(),
            ));
        }
    };
    let mut normalized_payload = payload.clone();
    if let Some(object) = normalized_payload.as_object_mut() {
        object.insert(
            "chartType".to_owned(),
            Value::String(normalized.to_owned()),
        );
    }
    Ok(normalized_payload)
}

fn market_date_value(value: &str) -> Result<time::Date, BacktestsWritePortError> {
    let format = time::format_description::parse_borrowed::<2>("[year]-[month]-[day]")
        .map_err(|_| {
            BacktestsWritePortError::BadRequest("invalid date, use YYYY-MM-DD format".to_owned())
        })?;
    time::Date::parse(value, &format).map_err(|_| {
        BacktestsWritePortError::BadRequest("invalid date, use YYYY-MM-DD format".to_owned())
    })
}

fn local_midnight_nanos(
    date: time::Date,
    timezone: &str,
) -> Result<i128, BacktestsWritePortError> {
    let civil = jiff::civil::Date::new(
        i16::try_from(date.year())
            .map_err(|_| BacktestsWritePortError::BadRequest("invalid date".to_owned()))?,
        i8::try_from(u8::from(date.month()))
            .map_err(|_| BacktestsWritePortError::BadRequest("invalid date".to_owned()))?,
        i8::try_from(date.day())
            .map_err(|_| BacktestsWritePortError::BadRequest("invalid date".to_owned()))?,
    )
    .map_err(|_| BacktestsWritePortError::BadRequest("invalid date".to_owned()))?;
    let zone = jiff::tz::TimeZone::get(timezone)
        .map_err(|error| BacktestsWritePortError::BadRequest(error.to_string()))?;
    let timestamp = civil
        .to_zoned(zone)
        .map_err(|error| BacktestsWritePortError::BadRequest(error.to_string()))?
        .timestamp();
    Ok(timestamp.as_nanosecond())
}

fn parse_backtest_instant(
    value: &str,
    label: &str,
) -> Result<time::OffsetDateTime, BacktestsWritePortError> {
    time::OffsetDateTime::parse(value, &time::format_description::well_known::Rfc3339).map_err(
        |_| BacktestsWritePortError::BadRequest(format!("invalid {label}, use RFC3339 format")),
    )
}

fn nanos_to_millis(nanos: i128) -> Result<i64, BacktestsWritePortError> {
    i64::try_from(nanos.div_euclid(1_000_000))
        .map_err(|_| BacktestsWritePortError::BadRequest("backtest time is out of range".to_owned()))
}

fn format_utc_nanos(nanos: i128) -> Result<String, BacktestsWritePortError> {
    let instant = time::OffsetDateTime::from_unix_timestamp_nanos(nanos).map_err(|_| {
        BacktestsWritePortError::BadRequest("backtest time is out of range".to_owned())
    })?;
    instant
        .format(&time::format_description::well_known::Rfc3339)
        .map_err(|_| {
            BacktestsWritePortError::BadRequest("backtest time is out of range".to_owned())
        })
}
