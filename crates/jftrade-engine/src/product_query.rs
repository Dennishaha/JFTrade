use std::collections::BTreeMap;
use time::format_description::well_known::Rfc3339;
use time::{Date, OffsetDateTime, PrimitiveDateTime, Time};

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum QueryError {
    InvalidUrlEscape,
}

#[derive(Debug, Clone, Default)]
pub(crate) struct QueryMap {
    params: BTreeMap<String, Vec<String>>,
}

impl QueryMap {
    pub(crate) fn parse(query_str: &str) -> Result<Self, QueryError> {
        let mut params: BTreeMap<String, Vec<String>> = BTreeMap::new();
        let trimmed = query_str.trim().trim_start_matches('?');
        if trimmed.is_empty() {
            return Ok(Self { params });
        }
        for pair in trimmed.split('&') {
            if pair.is_empty() {
                continue;
            }
            let (raw_key, raw_val) = match pair.split_once('=') {
                Some((k, v)) => (k, v),
                None => (pair, ""),
            };
            let key = decode_query_component(raw_key)?;
            let val = decode_query_component(raw_val)?;
            params.entry(key).or_default().push(val);
        }
        Ok(Self { params })
    }

    pub(crate) fn get_first(&self, key: &str) -> Option<&str> {
        self.params
            .get(key)
            .and_then(|values| values.first().map(String::as_str))
    }

    pub(crate) fn get_all(&self, key: &str) -> Option<&[String]> {
        self.params.get(key).map(Vec::as_slice)
    }
}

pub(crate) fn has_invalid_percent_escape(value: &str) -> bool {
    let bytes = value.as_bytes();
    let mut i = 0;
    while i < bytes.len() {
        if bytes[i] == b'%' {
            if i + 2 >= bytes.len()
                || !bytes[i + 1].is_ascii_hexdigit()
                || !bytes[i + 2].is_ascii_hexdigit()
            {
                return true;
            }
            i += 3;
        } else {
            i += 1;
        }
    }
    false
}

pub(crate) fn decode_query_component(value: &str) -> Result<String, QueryError> {
    if has_invalid_percent_escape(value) {
        return Err(QueryError::InvalidUrlEscape);
    }
    let replaced = value.replace('+', " ");
    let mut bytes = Vec::with_capacity(replaced.len());
    let raw_bytes = replaced.as_bytes();
    let mut i = 0;
    while i < raw_bytes.len() {
        if raw_bytes[i] == b'%' && i + 2 < raw_bytes.len() {
            let h1 = char::from(raw_bytes[i + 1]).to_digit(16);
            let h2 = char::from(raw_bytes[i + 2]).to_digit(16);
            if let (Some(n1), Some(n2)) = (h1, h2) {
                #[allow(clippy::cast_possible_truncation)]
                bytes.push((n1 << 4 | n2) as u8);
                i += 3;
                continue;
            }
        }
        bytes.push(raw_bytes[i]);
        i += 1;
    }
    String::from_utf8(bytes).map_err(|_| QueryError::InvalidUrlEscape)
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum CandlePeriodError {
    Unsupported(String),
}

pub(crate) fn normalize_candle_period(raw: &str) -> Result<&'static str, CandlePeriodError> {
    match raw.trim().to_ascii_lowercase().as_str() {
        "tick" | "ticker" | "k_tick" => Ok("tick"),
        "1m" | "1min" | "k_1m" => Ok("1m"),
        "3m" | "3min" | "k_3m" => Ok("3m"),
        "5m" | "5min" | "k_5m" => Ok("5m"),
        "10m" | "10min" | "k_10m" => Ok("10m"),
        "15m" | "15min" | "k_15m" => Ok("15m"),
        "30m" | "30min" | "k_30m" => Ok("30m"),
        "60m" | "60min" | "1h" | "1hour" | "k_60m" => Ok("1h"),
        "1d" | "day" | "daily" | "d" | "k_day" => Ok("1d"),
        "1w" | "week" | "weekly" | "w" | "k_week" => Ok("1w"),
        "1mo" | "month" | "mth" | "monthly" | "k_month" => Ok("1mo"),
        _ => Err(CandlePeriodError::Unsupported(raw.to_owned())),
    }
}

pub(crate) fn is_intraday_candle_period(period: &str) -> bool {
    matches!(period, "1m" | "3m" | "5m" | "10m" | "15m" | "30m" | "1h")
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum CandleSessionError {
    Empty,
    Invalid(String),
}

pub(crate) fn parse_candle_sessions(
    raw_values: Option<&[String]>,
) -> Result<Option<Vec<&'static str>>, CandleSessionError> {
    let Some(values) = raw_values else {
        return Ok(None);
    };
    let mut seen_regular = false;
    let mut seen_extended = false;
    let mut seen_overnight = false;
    let mut had_token = false;

    for value in values {
        for token in value.split(',') {
            had_token = true;
            let trimmed = token.trim().to_ascii_lowercase();
            if trimmed.is_empty() {
                continue;
            }
            match trimmed.as_str() {
                "regular" => seen_regular = true,
                "extended" => seen_extended = true,
                "overnight" => seen_overnight = true,
                other => return Err(CandleSessionError::Invalid(other.to_owned())),
            }
        }
    }

    if had_token && !seen_regular && !seen_extended && !seen_overnight {
        return Err(CandleSessionError::Empty);
    }

    let mut result = Vec::new();
    if seen_regular {
        result.push("regular");
    }
    if seen_extended {
        result.push("extended");
    }
    if seen_overnight {
        result.push("overnight");
    }
    Ok(Some(result))
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum QueryTimeError {
    Invalid(String),
}

pub(crate) fn parse_candle_before_time(value: &str) -> Result<Option<String>, QueryTimeError> {
    let trimmed = value.trim();
    if trimmed.is_empty() {
        return Ok(None);
    }
    if let Ok(dt) = OffsetDateTime::parse(trimmed, &Rfc3339) {
        let utc = dt.to_offset(time::UtcOffset::UTC);
        if let Ok(s) = utc.format(&Rfc3339) {
            return Ok(Some(s));
        }
    }
    Err(QueryTimeError::Invalid(trimmed.to_owned()))
}

pub(crate) fn normalize_optional_query_time(value: &str) -> Result<Option<String>, QueryTimeError> {
    let trimmed = value.trim();
    if trimmed.is_empty() {
        return Ok(None);
    }
    // 1. Try RFC3339 standard / nano
    if let Ok(dt) = OffsetDateTime::parse(trimmed, &Rfc3339) {
        let utc = dt.to_offset(time::UtcOffset::UTC);
        if let Ok(s) = utc.format(&Rfc3339) {
            return Ok(Some(s));
        }
    }

    // 2. Try "2006-01-02 15:04:05"
    if let Ok(format) = time::format_description::parse_borrowed::<1>(
        "[year]-[month]-[day] [hour]:[minute]:[second]",
    ) && let Ok(pdt) = PrimitiveDateTime::parse(trimmed, &format)
    {
        let dt = pdt.assume_utc();
        if let Ok(s) = dt.format(&Rfc3339) {
            return Ok(Some(s));
        }
    }

    // 3. Try "2006-01-02"
    if let Ok(format) = time::format_description::parse_borrowed::<1>("[year]-[month]-[day]")
        && let Ok(d) = Date::parse(trimmed, &format)
    {
        let pdt = PrimitiveDateTime::new(d, Time::MIDNIGHT);
        let dt = pdt.assume_utc();
        if let Ok(s) = dt.format(&Rfc3339) {
            return Ok(Some(s));
        }
    }

    Err(QueryTimeError::Invalid(trimmed.to_owned()))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn query_map_handles_percent_plus_and_multi_values() {
        let q =
            QueryMap::parse("q=apple+pie&sessions=regular&sessions=extended%2Covernight&blank=")
                .unwrap();
        assert_eq!(q.get_first("q"), Some("apple pie"));
        assert_eq!(
            q.get_all("sessions"),
            Some(&["regular".to_owned(), "extended,overnight".to_owned()][..])
        );
        assert_eq!(q.get_first("blank"), Some(""));

        // Invalid URL escapes
        assert_eq!(
            QueryMap::parse("q=%zz").unwrap_err(),
            QueryError::InvalidUrlEscape
        );
        assert_eq!(
            QueryMap::parse("q=%").unwrap_err(),
            QueryError::InvalidUrlEscape
        );
        assert_eq!(
            QueryMap::parse("q=%1").unwrap_err(),
            QueryError::InvalidUrlEscape
        );
    }

    #[test]
    fn candle_period_normalizes_aliases_and_rejects_unsupported() {
        assert_eq!(normalize_candle_period("ticker").unwrap(), "tick");
        assert_eq!(normalize_candle_period("k_tick").unwrap(), "tick");
        assert_eq!(normalize_candle_period("1min").unwrap(), "1m");
        assert_eq!(normalize_candle_period("k_3m").unwrap(), "3m");
        assert_eq!(normalize_candle_period("5min").unwrap(), "5m");
        assert_eq!(normalize_candle_period("k_10m").unwrap(), "10m");
        assert_eq!(normalize_candle_period("15min").unwrap(), "15m");
        assert_eq!(normalize_candle_period("k_30m").unwrap(), "30m");
        assert_eq!(normalize_candle_period("k_60m").unwrap(), "1h");
        assert_eq!(normalize_candle_period("60min").unwrap(), "1h");
        assert_eq!(normalize_candle_period("1hour").unwrap(), "1h");
        assert_eq!(normalize_candle_period("day").unwrap(), "1d");
        assert_eq!(normalize_candle_period("k_week").unwrap(), "1w");
        assert_eq!(normalize_candle_period("k_month").unwrap(), "1mo");

        // Reject 1y and 2h
        assert!(normalize_candle_period("1y").is_err());
        assert!(normalize_candle_period("2h").is_err());
    }

    #[test]
    fn candle_sessions_parse_dedup_order_and_reject_invalid() {
        let multi = vec![
            "overnight,regular".to_owned(),
            "extended".to_owned(),
            "regular".to_owned(),
        ];
        let sessions = parse_candle_sessions(Some(&multi)).unwrap().unwrap();
        assert_eq!(sessions, vec!["regular", "extended", "overnight"]);

        // Empty session value
        let empty_val = vec!["".to_owned()];
        assert_eq!(
            parse_candle_sessions(Some(&empty_val)).unwrap_err(),
            CandleSessionError::Empty
        );

        // Invalid token
        let invalid_token = vec!["all".to_owned()];
        assert_eq!(
            parse_candle_sessions(Some(&invalid_token)).unwrap_err(),
            CandleSessionError::Invalid("all".to_owned())
        );

        // None
        assert!(parse_candle_sessions(None).unwrap().is_none());
    }

    #[test]
    fn query_time_parses_rfc3339_datetime_and_date() {
        assert_eq!(
            normalize_optional_query_time("2026-08-28T11:50:30Z").unwrap(),
            Some("2026-08-28T11:50:30Z".to_owned())
        );
        assert_eq!(
            normalize_optional_query_time("2026-08-28 11:50:30").unwrap(),
            Some("2026-08-28T11:50:30Z".to_owned())
        );
        assert_eq!(
            normalize_optional_query_time("2026-08-28").unwrap(),
            Some("2026-08-28T00:00:00Z".to_owned())
        );
        assert_eq!(normalize_optional_query_time("  ").unwrap(), None);
        assert!(normalize_optional_query_time("not-a-time").is_err());

        // before: strict RFC3339 only
        assert_eq!(
            parse_candle_before_time("2026-08-28T11:50:30Z").unwrap(),
            Some("2026-08-28T11:50:30Z".to_owned())
        );
        assert!(parse_candle_before_time("2026-08-28 11:50:30").is_err());
        assert!(parse_candle_before_time("2026-08-28").is_err());
    }
}
