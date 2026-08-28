use serde_json::Value;

use crate::product::product_backtests_write_port::BacktestsWritePortError;

#[derive(Clone, Debug)]
pub(super) struct SyncRequest {
    pub(super) market: String,
    pub(super) symbol: String,
    pub(super) intervals: Vec<String>,
    pub(super) since: String,
    pub(super) until: String,
    pub(super) session_scope: String,
    pub(super) rehab_type: String,
}

pub(super) fn parse_sync_request(payload: &Value) -> Result<SyncRequest, BacktestsWritePortError> {
    let object = payload.as_object();
    let text = |key: &str| object.and_then(|value| value.get(key)).and_then(Value::as_str);
    let raw_market = text("market").unwrap_or("").trim().to_ascii_uppercase();
    let raw_symbol = text("symbol").unwrap_or("").trim().replace(':', ".");
    let raw_code = text("code").unwrap_or("").trim().to_ascii_uppercase();
    let (market, symbol) = normalize_sync_instrument(&raw_market, &raw_symbol, &raw_code)?;
    let intervals = object
        .and_then(|value| value.get("intervals"))
        .and_then(Value::as_array)
        .map(|values| values.iter().filter_map(Value::as_str).map(|value| value.trim().to_ascii_lowercase()).filter(|value| !value.is_empty()).collect::<Vec<_>>())
        .filter(|values| !values.is_empty())
        .unwrap_or_else(|| ["1m", "5m", "15m", "30m", "1h", "1d", "1w"].into_iter().map(str::to_owned).collect());
    for interval in &intervals {
        if !matches!(interval.as_str(), "1m" | "5m" | "15m" | "30m" | "60m" | "1h" | "2h" | "3h" | "4h" | "6h" | "8h" | "12h" | "1d" | "3d" | "1w" | "2w" | "1mo") {
            return Err(BacktestsWritePortError::BadRequest(format!("invalid interval: {interval}")));
        }
    }
    let now = time::OffsetDateTime::now_utc();
    let (since, until) = if text("startDate").unwrap_or("").trim() != "" || text("endDate").unwrap_or("").trim() != "" {
        let start = text("startDate").unwrap_or("").trim();
        let end = text("endDate").unwrap_or("").trim();
        if start.is_empty() || end.is_empty() {
            return Err(BacktestsWritePortError::BadRequest("startDate and endDate must be provided together".to_owned()));
        }
        let since = parse_market_date(start, &market)?;
        let end_date = parse_market_date(end, &market)?;
        (since, end_date + time::Duration::days(1) - time::Duration::nanoseconds(1))
    } else {
        let until = text("until").filter(|value| !value.trim().is_empty()).map(parse_timestamp).transpose().map_err(BacktestsWritePortError::BadRequest)?.unwrap_or(now);
        let since = text("since").filter(|value| !value.trim().is_empty()).map(parse_timestamp).transpose().map_err(BacktestsWritePortError::BadRequest)?.unwrap_or_else(|| until - time::Duration::days(30));
        (since, until)
    };
    if until <= since {
        return Err(BacktestsWritePortError::BadRequest("until must be after since".to_owned()));
    }
    let session_scope = match text("sessionScope").unwrap_or("regular") {
        "regular" | "extended" => text("sessionScope").unwrap_or("regular").to_owned(),
        _ => return Err(BacktestsWritePortError::BadRequest("sessionScope must be regular or extended".to_owned())),
    };
    let rehab_type = match text("rehabType").unwrap_or("forward").trim().to_ascii_lowercase().as_str() {
        "backward" => "backward".to_owned(),
        "none" => "none".to_owned(),
        _ => "forward".to_owned(),
    };
    Ok(SyncRequest { market, symbol: symbol.clone(), intervals: plan_sync_intervals(&symbol, &intervals, &session_scope), since: format_timestamp(since), until: format_timestamp(until), session_scope, rehab_type })
}

fn normalize_sync_instrument(market: &str, symbol: &str, code: &str) -> Result<(String, String), BacktestsWritePortError> {
    let (symbol_market, symbol_code) = symbol.split_once('.').map(|(prefix, value)| (prefix.trim().to_ascii_uppercase(), value.trim().to_ascii_uppercase())).unwrap_or_default();
    let symbol_has_market = !symbol_market.is_empty();
    if symbol.contains('.') && (symbol_market.is_empty() || symbol_code.is_empty()) {
        return Err(BacktestsWritePortError::BadRequest("symbol must be in MARKET.CODE form".to_owned()));
    }
    let chosen_market = if symbol_has_market {
        if !market.is_empty() && !markets_match(market, &symbol_market) {
            return Err(BacktestsWritePortError::BadRequest("market does not match symbol".to_owned()));
        }
        symbol_market.clone()
    } else if market.is_empty() {
        if code.is_empty() { return Ok(("HK".to_owned(), "HK.00700".to_owned())); }
        return Err(BacktestsWritePortError::BadRequest("market is required when symbol has no market prefix".to_owned()));
    } else { market.to_owned() };
    if !matches!(chosen_market.as_str(), "US" | "HK" | "CN" | "SH" | "SZ") {
        return Err(BacktestsWritePortError::BadRequest("invalid market".to_owned()));
    }
    if chosen_market == "CN" && !symbol_has_market {
        return Err(BacktestsWritePortError::BadRequest("market CN requires an exchange-qualified symbol like SH.600519 or SZ.000001".to_owned()));
    }
    let symbol_has_code = !symbol_code.is_empty();
    let bare_symbol = if symbol_has_market { String::new() } else { symbol.to_ascii_uppercase() };
    let chosen_code = if symbol_has_code { symbol_code.clone() } else if !bare_symbol.is_empty() { bare_symbol.clone() } else { code.to_owned() };
    if chosen_code.is_empty() { return Err(BacktestsWritePortError::BadRequest("symbol or code is required".to_owned())); }
    if (!code.is_empty() && symbol_has_code && code != symbol_code) || (!code.is_empty() && !symbol_has_code && !bare_symbol.is_empty() && code != bare_symbol) {
        return Err(BacktestsWritePortError::BadRequest("code does not match symbol".to_owned()));
    }
    if chosen_code.chars().any(|ch| ch.is_whitespace() || ch == '.') { return Err(BacktestsWritePortError::BadRequest("invalid symbol code".to_owned())); }
    let prefix = if symbol_has_market { symbol_market } else { chosen_market.clone() };
    let resolved_market = if matches!(prefix.as_str(), "SH" | "SZ") { "CN".to_owned() } else { chosen_market };
    Ok((resolved_market, format!("{prefix}.{chosen_code}")))
}

fn markets_match(requested: &str, qualified: &str) -> bool { requested == qualified || (requested == "CN" && matches!(qualified, "SH" | "SZ")) }

fn parse_market_date(value: &str, market: &str) -> Result<time::OffsetDateTime, BacktestsWritePortError> {
    let format = time::format_description::parse_borrowed::<1>("[year]-[month]-[day]").map_err(|_| BacktestsWritePortError::BadRequest("invalid date, use YYYY-MM-DD".to_owned()))?;
    let date = time::Date::parse(value, &format).map_err(|_| BacktestsWritePortError::BadRequest("invalid date, use YYYY-MM-DD".to_owned()))?;
    let timezone = match market { "US" => "America/New_York", "HK" => "Asia/Hong_Kong", "CN" | "SH" | "SZ" => "Asia/Shanghai", _ => "UTC" };
    let local_date = jiff::civil::Date::new(i16::try_from(date.year()).map_err(|_| BacktestsWritePortError::BadRequest("invalid date".to_owned()))?, i8::try_from(u8::from(date.month())).map_err(|_| BacktestsWritePortError::BadRequest("invalid date".to_owned()))?, i8::try_from(date.day()).map_err(|_| BacktestsWritePortError::BadRequest("invalid date".to_owned()))?).map_err(|_| BacktestsWritePortError::BadRequest("invalid date, use YYYY-MM-DD".to_owned()))?;
    let zone = jiff::tz::TimeZone::get(timezone).map_err(|error| BacktestsWritePortError::BadRequest(error.to_string()))?;
    let timestamp = local_date.to_zoned(zone).map_err(|error| BacktestsWritePortError::BadRequest(error.to_string()))?.timestamp();
    parse_timestamp(&timestamp.to_string()).map_err(BacktestsWritePortError::BadRequest)
}

fn plan_sync_intervals(symbol: &str, requested: &[String], session_scope: &str) -> Vec<String> {
    let mut planned = Vec::with_capacity(requested.len());
    for interval in requested {
        let normalized = match interval.as_str() {
            "60m" | "2h" | "3h" | "4h" | "6h" | "8h" | "12h" => "1h",
            "3d" | "2w" => "1d",
            value if session_scope == "extended" && symbol.starts_with("US.") && matches!(value, "1d" | "1w" | "1mo") => "1h",
            value => value,
        };
        if !planned.iter().any(|value| value == normalized) { planned.push(normalized.to_owned()); }
    }
    planned
}

pub(super) fn parse_timestamp(value: &str) -> Result<time::OffsetDateTime, String> {
    time::OffsetDateTime::parse(value.trim(), &time::format_description::well_known::Rfc3339).map_err(|error| format!("invalid timestamp: {error}"))
}

pub(super) fn format_timestamp(value: time::OffsetDateTime) -> String {
    value.format(&time::format_description::well_known::Rfc3339).unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned())
}
