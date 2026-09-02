use crate::product::product_backtests_write_port::BacktestsWritePortError;
use jftrade_settings::MarketDataProvider;

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
        "unsupported backtest executionModel: {value}"
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
