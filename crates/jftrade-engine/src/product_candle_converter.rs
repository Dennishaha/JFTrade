//! Go-compatible candle conversion backed by the shared exchange calendar.

use std::cmp::Ordering;
use std::str::FromStr;

use jftrade_calendar::CalendarManager;
use jftrade_integration_marketdata_helper::{HelperCandlesResponse, HelperPriceValue};
use jftrade_kernel::{DecimalText, WireTimestamp};
use serde_json::{Value, json};

use crate::product::MarketDataQuoteReadSnapshotError;

pub(crate) struct HelperCandleConversionParams<'a> {
    pub market: &'a str,
    pub symbol: &'a str,
    pub period: &'a str,
    pub limit: usize,
    pub from_time: Option<&'a str>,
    pub to_time: Option<&'a str>,
    pub before: Option<&'a str>,
    pub sessions: &'a [&'a str],
    pub is_yfinance: bool,
    pub is_akshare: bool,
    pub calendar: Option<&'a CalendarManager>,
}

pub(crate) fn convert_helper_candles_response(
    resp: HelperCandlesResponse,
    params: HelperCandleConversionParams<'_>,
) -> Result<Value, MarketDataQuoteReadSnapshotError> {
    validate_response_identity(&resp, &params)?;
    let bounds = CandleBounds::parse(&params)?;
    let mut candles = Vec::with_capacity(resp.candles.len());
    let mut previous = None;

    for candle in &resp.candles {
        let at = parse_timestamp("at", &candle.at)?;
        validate_timestamp_order(previous, at)?;
        bounds.validate(at)?;
        previous = Some(at);
        let values = CandleDecimals::parse(candle)?;
        let session = candle_session(&params, candle.session.as_deref(), at)?;
        if !session.keep || !requested_session(params.sessions, session.label.as_deref()) {
            continue;
        }
        let volume = candle_volume(&values, session.label.as_deref(), params.is_yfinance);
        candles.push(json!({
            "at": canonical_timestamp(at),
            "close": values.close.as_str(),
            "high": values.high.as_str(),
            "low": values.low.as_str(),
            "open": values.open.as_str(),
            "period": params.period,
            "session": session.label,
            "volume": volume,
        }));
    }

    let pagination = validate_pagination(&resp, &candles, &params)?;
    let include_session = candles.iter().any(|candle| candle["session"].is_string());
    let actual_extended = candles.iter().any(|candle| {
        matches!(
            candle["session"].as_str(),
            Some("pre" | "after" | "overnight")
        )
    });
    // Older helper fixtures omitted `extended_hours`; when the returned rows
    // themselves prove extended sessions, preserve the Go observable value.
    let extended_hours =
        include_session && actual_extended && (resp.extended_hours || params.is_yfinance);
    let instrument_id = format!("{}.{}", params.market, params.symbol);
    let mut meta = json!({
        "extendedHours": extended_hours,
        "fromCache": false,
        "instrumentId": instrument_id,
        "resolvedAt": current_utc_rfc3339(),
        "sessions": params.sessions,
        "source": resp.source,
    });
    if include_session {
        meta["session"] = json!(if extended_hours { "all" } else { "regular" });
    }

    Ok(json!({
        "candles": candles,
        "meta": meta,
        "pagination": pagination,
        "request": {
            "instrument": {
                "instrumentId": instrument_id,
                "market": params.market,
                "symbol": params.symbol,
            },
            "limit": params.limit,
            "period": params.period,
            "sessions": params.sessions,
        },
        "totalReturned": candles.len(),
    }))
}

fn validate_response_identity(
    response: &HelperCandlesResponse,
    params: &HelperCandleConversionParams<'_>,
) -> Result<(), MarketDataQuoteReadSnapshotError> {
    let expected_id = format!("{}.{}", params.market, params.symbol);
    if !response.market.eq_ignore_ascii_case(params.market)
        || !response.symbol.eq_ignore_ascii_case(params.symbol)
        || !response.instrument_id.eq_ignore_ascii_case(&expected_id)
        || !response.period.eq_ignore_ascii_case(params.period)
    {
        return failed(format!(
            "helper returned mismatched candle identity or period: {} {} {} {}",
            response.market, response.symbol, response.instrument_id, response.period
        ));
    }
    if params.is_akshare && response.extended_hours {
        return failed("AKShare returned extended-hours candles");
    }
    if response.total_returned != response.candles.len() {
        return failed("helper returned mismatched totalReturned count");
    }
    Ok(())
}

#[derive(Clone, Copy)]
struct CandleBounds {
    from: Option<time::OffsetDateTime>,
    to: Option<time::OffsetDateTime>,
    before: Option<time::OffsetDateTime>,
}

impl CandleBounds {
    fn parse(
        params: &HelperCandleConversionParams<'_>,
    ) -> Result<Self, MarketDataQuoteReadSnapshotError> {
        Ok(Self {
            from: params
                .from_time
                .map(|value| parse_timestamp("from", value))
                .transpose()?,
            to: params
                .to_time
                .map(|value| parse_timestamp("to", value))
                .transpose()?,
            before: params
                .before
                .map(|value| parse_timestamp("before", value))
                .transpose()?,
        })
    }

    fn validate(&self, at: time::OffsetDateTime) -> Result<(), MarketDataQuoteReadSnapshotError> {
        if self.before.is_some_and(|before| at >= before) {
            return failed("candle page violates before cursor");
        }
        if self.from.is_some_and(|from| at < from) {
            return failed("candle precedes requested from boundary");
        }
        if self.to.is_some_and(|to| at > to) {
            return failed("candle exceeds requested to boundary");
        }
        Ok(())
    }

    fn bounded(self) -> bool {
        self.from.is_some() || self.to.is_some()
    }
}

struct CandleDecimals {
    open: DecimalText,
    high: DecimalText,
    low: DecimalText,
    close: DecimalText,
    volume: Option<DecimalText>,
}

impl CandleDecimals {
    fn parse(
        candle: &jftrade_integration_marketdata_helper::HelperCandle,
    ) -> Result<Self, MarketDataQuoteReadSnapshotError> {
        let open = positive_decimal("open", &candle.open)?;
        let high = positive_decimal("high", &candle.high)?;
        let low = positive_decimal("low", &candle.low)?;
        let close = positive_decimal("close", &candle.close)?;
        if decimal_cmp(&high, &low) == Ordering::Less
            || decimal_cmp(&high, &open) == Ordering::Less
            || decimal_cmp(&high, &close) == Ordering::Less
            || decimal_cmp(&low, &open) == Ordering::Greater
            || decimal_cmp(&low, &close) == Ordering::Greater
        {
            return failed("helper returned invalid OHLC bounds");
        }
        let volume = candle
            .volume
            .as_ref()
            .map(|value| non_negative_decimal("volume", value))
            .transpose()?;
        Ok(Self {
            open,
            high,
            low,
            close,
            volume,
        })
    }
}

struct ResolvedSession {
    label: Option<String>,
    keep: bool,
}

fn candle_session(
    params: &HelperCandleConversionParams<'_>,
    helper_session: Option<&str>,
    at: time::OffsetDateTime,
) -> Result<ResolvedSession, MarketDataQuoteReadSnapshotError> {
    if params.is_akshare || matches!(params.period, "1d" | "1w" | "1mo") {
        return Ok(ResolvedSession {
            label: None,
            keep: true,
        });
    }
    if !params.is_yfinance {
        return Ok(ResolvedSession {
            label: helper_session.map(str::to_owned),
            keep: true,
        });
    }
    let calendar = params.calendar.ok_or_else(|| {
        candle_error("exchange calendar is required for yfinance intraday candles")
    })?;
    let calendar_session = calendar
        .classify_session(params.market, WireTimestamp::from_offset_datetime(at))
        .map_err(|error| {
            candle_error(format!("exchange calendar classification failed: {error}"))
        })?;
    if let Some(actual) = helper_session
        .map(str::trim)
        .filter(|value| !value.is_empty())
        && calendar_session.as_deref() != Some(actual)
    {
        return failed(format!(
            "helper candle session {actual:?} disagrees with exchange calendar"
        ));
    }
    let keep = matches!(
        calendar_session.as_deref(),
        Some("pre" | "regular" | "after")
    );
    Ok(ResolvedSession {
        label: calendar_session,
        keep,
    })
}

fn candle_volume(values: &CandleDecimals, session: Option<&str>, yfinance: bool) -> Value {
    if yfinance && matches!(session, Some("pre" | "after")) {
        return Value::Null;
    }
    values
        .volume
        .as_ref()
        .map_or(Value::Null, |volume| json!(volume.as_str()))
}

fn requested_session(requested: &[&str], actual: Option<&str>) -> bool {
    let group = match actual {
        Some("pre" | "after" | "overnight") => "extended",
        _ => "regular",
    };
    requested.iter().any(|requested| {
        *requested == "all"
            || *requested == group
            || actual.is_some_and(|actual| actual == *requested)
    })
}

fn validate_pagination(
    response: &HelperCandlesResponse,
    candles: &[Value],
    params: &HelperCandleConversionParams<'_>,
) -> Result<Value, MarketDataQuoteReadSnapshotError> {
    if candles.len() > params.limit {
        return failed("candle page exceeds the requested limit");
    }
    let bounds = CandleBounds::parse(params)?;
    let next_before = response
        .next_before
        .as_deref()
        .map(str::trim)
        .filter(|v| !v.is_empty());
    if bounds.bounded() {
        if response.has_more || next_before.is_some() {
            return failed("bounded candle response cannot continue pagination");
        }
        return Ok(json!({"hasMore": false}));
    }
    if !response.has_more {
        if next_before.is_some() {
            return failed("terminal candle page has next_before");
        }
        return Ok(json!({"hasMore": false}));
    }
    let Some(earliest) = candles.first().and_then(|candle| candle["at"].as_str()) else {
        return failed("invalid paged candle count");
    };
    let next = next_before.ok_or_else(|| candle_error("next_before is required"))?;
    let next = canonical_timestamp(parse_timestamp("next_before", next)?);
    if next != earliest {
        return failed("next_before must equal earliest candle");
    }
    Ok(json!({"hasMore": true, "nextBefore": next}))
}

fn parse_timestamp(
    field: &str,
    value: &str,
) -> Result<time::OffsetDateTime, MarketDataQuoteReadSnapshotError> {
    time::OffsetDateTime::parse(value.trim(), &time::format_description::well_known::Rfc3339)
        .map_err(|_| {
            candle_error(format!(
                "helper returned invalid {field} timestamp: {value}"
            ))
        })
}

fn canonical_timestamp(value: time::OffsetDateTime) -> String {
    value
        .to_offset(time::UtcOffset::UTC)
        .format(&time::format_description::well_known::Rfc3339)
        .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned())
}

fn validate_timestamp_order(
    previous: Option<time::OffsetDateTime>,
    current: time::OffsetDateTime,
) -> Result<(), MarketDataQuoteReadSnapshotError> {
    if previous.is_some_and(|previous| current <= previous) {
        return failed("helper returned out-of-order candles");
    }
    Ok(())
}

fn positive_decimal(
    field: &str,
    value: &HelperPriceValue,
) -> Result<DecimalText, MarketDataQuoteReadSnapshotError> {
    let parsed = DecimalText::from_str(value.as_str())
        .map_err(|_| candle_error(format!("helper returned invalid {field} decimal")))?;
    if decimal_cmp(&parsed, &decimal_zero()) != Ordering::Greater {
        return failed(format!("helper returned non-positive {field} price"));
    }
    Ok(parsed)
}

fn non_negative_decimal(
    field: &str,
    value: &HelperPriceValue,
) -> Result<DecimalText, MarketDataQuoteReadSnapshotError> {
    let parsed = DecimalText::from_str(value.as_str())
        .map_err(|_| candle_error(format!("helper returned invalid {field} decimal")))?;
    if decimal_cmp(&parsed, &decimal_zero()) == Ordering::Less {
        return failed(format!("helper returned negative {field}"));
    }
    Ok(parsed)
}

fn decimal_zero() -> DecimalText {
    DecimalText::from_str("0").expect("zero is a valid decimal")
}

fn decimal_cmp(left: &DecimalText, right: &DecimalText) -> Ordering {
    let (left_negative, left_abs) = decimal_parts(left.as_str());
    let (right_negative, right_abs) = decimal_parts(right.as_str());
    match (left_negative, right_negative) {
        (true, false) => Ordering::Less,
        (false, true) => Ordering::Greater,
        (false, false) => decimal_abs_cmp(left_abs, right_abs),
        (true, true) => decimal_abs_cmp(left_abs, right_abs).reverse(),
    }
}

fn decimal_parts(value: &str) -> (bool, &str) {
    value
        .strip_prefix('-')
        .map_or((false, value), |value| (true, value))
}

fn decimal_abs_cmp(left: &str, right: &str) -> Ordering {
    let (left_integer, left_fraction) = left.split_once('.').unwrap_or((left, ""));
    let (right_integer, right_fraction) = right.split_once('.').unwrap_or((right, ""));
    left_integer
        .len()
        .cmp(&right_integer.len())
        .then_with(|| left_integer.cmp(right_integer))
        .then_with(|| {
            let width = left_fraction.len().max(right_fraction.len());
            left_fraction
                .bytes()
                .chain(std::iter::repeat(b'0'))
                .take(width)
                .cmp(
                    right_fraction
                        .bytes()
                        .chain(std::iter::repeat(b'0'))
                        .take(width),
                )
        })
}

fn current_utc_rfc3339() -> String {
    canonical_timestamp(time::OffsetDateTime::now_utc())
}

fn failed<T>(message: impl Into<String>) -> Result<T, MarketDataQuoteReadSnapshotError> {
    Err(candle_error(message))
}

fn candle_error(message: impl Into<String>) -> MarketDataQuoteReadSnapshotError {
    MarketDataQuoteReadSnapshotError::Failed {
        status: 502,
        code: "OPEND_CANDLES_FAILED".to_owned(),
        message: message.into(),
        retry_after_seconds: None,
    }
}
