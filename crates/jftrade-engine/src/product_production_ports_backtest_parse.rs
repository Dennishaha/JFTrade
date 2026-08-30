use crate::product::product_backtests_write_port::BacktestsWritePortError;
use jftrade_settings::MarketDataProvider;

#[derive(Clone, Debug)]
pub(crate) struct ParsedBacktestStart {
    pub(crate) symbol: String,
    pub(crate) interval: String,
    pub(crate) rehab_type: String,
    pub(crate) session_scope: String,
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
