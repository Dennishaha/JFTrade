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

/// Reject a candle price-adjustment label that no provider can serve.
///
/// Go's `normalizeCandleOptions` lowercases and trims the label, defaults a
/// missing value to `none`, and rejects everything outside
/// `none`/`forward`/`backward` with `ErrInvalidQuery`. The empty string is
/// accepted and mapped to the default so the assistant tool and the HTTP route
/// share one vocabulary.
#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum CandleAdjustmentError {
    Unsupported(String),
}

/// Go's `brokerKLineRehabType` label set.
pub(crate) fn parse_candle_adjustment(
    raw: Option<&str>,
) -> Result<&'static str, CandleAdjustmentError> {
    let Some(raw) = raw else {
        return Ok("none");
    };
    let normalized = raw.trim().to_ascii_lowercase();
    match normalized.as_str() {
        "" | "none" => Ok("none"),
        "forward" => Ok("forward"),
        "backward" => Ok("backward"),
        other => Err(CandleAdjustmentError::Unsupported(other.to_owned())),
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum QueryTimeError {
    Invalid(String),
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum QueryBoolError {
    Invalid(String),
}

/// Decode an optional boolean query value with the reference owner's alias
/// table.  The baseline treats `0/false/no/n/off` and a blank value as false
/// and `1/true/yes/y/on` as true; anything else is rejected so the route can
/// answer 400 instead of silently guessing.
pub(crate) fn parse_optional_query_bool(value: &str) -> Result<bool, QueryBoolError> {
    let trimmed = value.trim();
    match trimmed.to_ascii_lowercase().as_str() {
        "1" | "true" | "yes" | "y" | "on" => Ok(true),
        "0" | "false" | "no" | "n" | "off" | "" => Ok(false),
        _ => Err(QueryBoolError::Invalid(trimmed.to_owned())),
    }
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
    // Parity: go:452dea11:internal/api/watchlist/route_error_handling_test.go:77 TestBindQueryRejectsMalformedAndInvalidValues
    // Parity: go:452dea11:internal/api/watchlist/routes_test.go:84 TestWatchlistListAndBindingRoutesRejectMalformedQueryEncoding
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

    // Parity: go:452dea11:internal/api/httpserver/bindings_boundaries_test.go:13 TestOptionalBoolRecognizesFalseAliases
    // Parity: go:452dea11:internal/api/httpserver/bindings_test.go:100 TestOptionalQueryValueParsingSemantics
    // The reference `OptionalBoolValue` treats every documented falsy alias and
    // a blank value as false, the truthy aliases as true (after trim + lower),
    // and any other text as invalid so the binding fails with 400.
    #[test]
    fn optional_query_bool_matches_the_reference_alias_table() {
        for input in ["0", "false", "no", "n", "off", "", "   ", " OFF "] {
            assert_eq!(parse_optional_query_bool(input), Ok(false), "{input:?}");
        }
        for input in ["1", "true", "yes", "y", "on", " YeS ", "ON"] {
            assert_eq!(parse_optional_query_bool(input), Ok(true), "{input:?}");
        }
        for input in ["maybe", "not-a-boolean", "2"] {
            assert_eq!(
                parse_optional_query_bool(input),
                Err(QueryBoolError::Invalid(input.trim().to_owned())),
                "{input:?}"
            );
        }
    }

    #[test]
    // Parity: go:452dea11:internal/api/httpserver/bindings_boundaries_test.go:37 TestNormalizeCandlePeriodSupportsEveryDocumentedFamily
    // Parity: go:452dea11:internal/api/httpserver/bindings_test.go:186 TestCandlePeriodAndPaginationNormalization
    // Parity: go:452dea11:internal/app/apiserver/marketdataapp/query_test.go:11 TestNormalizeCandlePeriodMapsAliases
    // The reference owner normalizes the same documented family aliases and
    // trims surrounding whitespace; the pagination half is owned by the candle
    // route clamp in `product_production_ports_market_data_quote_reads.rs`.
    // The padded k_60m alias resolves through the same trim-then-match path.
    // Parity: go:452dea11:internal/api/httpserver/bindings_boundaries_test.go:27 TestCandlePeriodValueHandlesEmptyAndUnsupportedInputs
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
        // `CandlePeriodValue.UnmarshalText` trims before dispatching to the
        // normalizer, so a padded alias has to resolve the same way.
        assert_eq!(normalize_candle_period(" 60m ").unwrap(), "1h");
        assert_eq!(normalize_candle_period(" Ticker ").unwrap(), "tick");
        assert_eq!(normalize_candle_period("day").unwrap(), "1d");

        // Reject 1y and 2h
        assert!(normalize_candle_period("1y").is_err());
        assert!(normalize_candle_period("2h").is_err());
    }

    #[test]
    // Parity: go:452dea11:internal/api/httpserver/bindings_boundaries_test.go:37 TestNormalizeCandlePeriodSupportsEveryDocumentedFamily
    // Verbatim replay of the Go alias table: every documented family has to
    // land on the canonical label the candle routes publish.
    fn documented_candle_period_families_match_the_go_table() {
        for (input, want) in [
            ("ticker", "tick"),
            ("1min", "1m"),
            ("k_3m", "3m"),
            ("5min", "5m"),
            ("k_10m", "10m"),
            ("15min", "15m"),
            ("k_30m", "30m"),
            ("k_60m", "1h"),
            ("day", "1d"),
            ("k_week", "1w"),
            ("k_month", "1mo"),
        ] {
            assert_eq!(
                normalize_candle_period(input).unwrap(),
                want,
                "input {input:?}"
            );
        }
    }

    #[test]
    // Parity: go:452dea11:internal/api/httpserver/bindings_test.go:14 TestParseQueryTimeNormalizesToUTC
    // Go's `ParseQueryTime` turns timezone-less timestamps and dates into UTC
    // and converts explicit offsets, so the host's local zone can never leak
    // into the wire value.
    fn optional_query_time_normalizes_to_utc_and_blank_means_absent() {
        assert_eq!(
            normalize_optional_query_time("2026-06-20 09:30:00").unwrap(),
            Some("2026-06-20T09:30:00Z".to_owned())
        );
        assert_eq!(
            normalize_optional_query_time("2026-06-20").unwrap(),
            Some("2026-06-20T00:00:00Z".to_owned())
        );
        assert_eq!(
            normalize_optional_query_time("2026-06-20T09:30:00+08:00").unwrap(),
            Some("2026-06-20T01:30:00Z".to_owned())
        );

        // Parity: go:452dea11:internal/api/httpserver/bindings_boundaries_test.go:61 TestParseQueryTimeReturnsCallerFallback
        // Blank input leaves the caller's fallback in charge. Go additionally
        // swallows unparseable text and returns the fallback there; Rust answers
        // 400 "time must be a valid timestamp" instead, which is registered as
        // a deliberate difference in the parity inventory.
        assert_eq!(normalize_optional_query_time("   ").unwrap(), None);
        assert!(normalize_optional_query_time("not-a-time").is_err());
    }

    #[test]
    // Parity: go:452dea11:internal/api/httpserver/bindings_test.go:56 TestBindURIRejectsMalformedEscapeInRequestURI
    // Parity: go:452dea11:internal/api/httpserver/bindings_test.go:75 TestBindURIAllowsEscapedLiteralPercent
    // Parity: go:452dea11:internal/api/httpserver/bindings_boundaries_test.go:70 TestBindURIHandlesBindingAndFallbackEscapeValidation
    // `BindURI` validates the escape sequence before decoding the segment, so a
    // malformed `%ZZ`/`%2` fails while `%25` decodes to a literal percent and
    // `%20` to a space.
    fn uri_escape_validation_accepts_literal_percent_and_rejects_malformed() {
        assert!(has_invalid_percent_escape("bad%ZZ"));
        assert!(has_invalid_percent_escape("bad%2"));
        assert_eq!(
            decode_query_component("bad%ZZ").unwrap_err(),
            QueryError::InvalidUrlEscape
        );

        assert!(!has_invalid_percent_escape("value%25"));
        assert_eq!(decode_query_component("value%25").unwrap(), "value%");
        assert_eq!(
            decode_query_component("valid%20value").unwrap(),
            "valid value"
        );
    }

    // Parity: go:452dea11:internal/app/apiserver/marketdataapp/query_test.go:45 TestDecodeMarketCandlesQueryParsesRepeatedSessions
    // Parity: go:452dea11:internal/productfeatures/candle_query_options_test.go:8 TestNormalizeCandleOptionsAcceptsSessionsAndAdjustments
    #[test]
    fn candle_sessions_parse_dedup_order_and_reject_invalid() {
        // Parity: go:452dea11:internal/marketdata/candle_sessions_test.go:8
        // TestParseCandleSessionsNormalizesCSVAndRepeatedValues
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

    // Parity: go:452dea11:internal/marketdata/candle_sessions_test.go:8
    // TestParseCandleSessionsNormalizesCSVAndRepeatedValues
    #[test]
    fn candle_sessions_normalize_csv_and_repeated_values_for_parity() {
        let values = vec![
            "overnight,regular".to_owned(),
            "extended".to_owned(),
            "regular".to_owned(),
        ];
        let sessions = parse_candle_sessions(Some(&values))
            .expect("session input should parse")
            .expect("session input should be present");
        assert_eq!(sessions, vec!["regular", "extended", "overnight"]);
    }

    // Parity: go:452dea11:internal/marketdata/candle_sessions_test.go:18
    // TestParseCandleSessionsRejectsEmptyAndUnknownValues
    #[test]
    fn candle_sessions_reject_empty_and_unknown_values_for_parity() {
        let empty = vec!["".to_owned()];
        assert_eq!(
            parse_candle_sessions(Some(&empty)).unwrap_err(),
            CandleSessionError::Empty
        );

        let unknown = vec!["regular,invalid".to_owned()];
        assert_eq!(
            parse_candle_sessions(Some(&unknown)).unwrap_err(),
            CandleSessionError::Invalid("invalid".to_owned())
        );
    }

    /// Parity: go:452dea11:internal/productfeatures/market_data_reads_test.go:226
    /// TestNormalizeCoreCandleQueryAcceptsSessionParameterShapes
    ///
    /// Go accepts both a `[]any` session list and a comma-joined string and
    /// requires the two normalized sessions to stay distinct. On the Rust wire
    /// both shapes are the repeated `sessions` query parameter, and every
    /// element is trimmed and lowercased before deduplication.
    #[test]
    fn candle_sessions_accept_padded_and_csv_query_shapes() {
        let array_shape = vec![" regular ".to_owned(), "extended".to_owned()];
        let parsed = parse_candle_sessions(Some(&array_shape)).unwrap().unwrap();
        assert_eq!(parsed, vec!["regular", "extended"]);
        assert_ne!(parsed[0], parsed[1], "the two sessions must stay distinct");

        let string_shape = vec!["regular, overnight".to_owned()];
        let parsed = parse_candle_sessions(Some(&string_shape)).unwrap().unwrap();
        assert_eq!(parsed, vec!["regular", "overnight"]);
        assert_ne!(parsed[0], parsed[1], "the two sessions must stay distinct");
    }

    /// Parity: go:452dea11:internal/productfeatures/candle_query_options_test.go:8 TestNormalizeCandleOptionsAcceptsSessionsAndAdjustments
    /// Parity: go:452dea11:internal/productfeatures/candle_query_options_test.go:9
    /// TestNormalizeCandleOptionsAcceptsSessionsAndAdjustments and :17
    /// TestNormalizeCandleOptionsRejectsUnsupportedValues.
    #[test]
    // Parity: go:452dea11:internal/productfeatures/candle_query_options_test.go:18 TestNormalizeCandleOptionsRejectsUnsupportedValues
    fn candle_adjustment_normalizes_and_rejects_unsupported_labels() {
        assert_eq!(parse_candle_adjustment(Some(" FORWARD ")), Ok("forward"));
        assert_eq!(parse_candle_adjustment(Some("Backward")), Ok("backward"));
        assert_eq!(parse_candle_adjustment(Some("none")), Ok("none"));
        assert_eq!(
            parse_candle_adjustment(None),
            Ok("none"),
            "Go defaults a missing adjustment to none"
        );
        assert_eq!(parse_candle_adjustment(Some("  ")), Ok("none"));
        for label in ["split", "split-adjusted", "rehab", "1"] {
            assert_eq!(
                parse_candle_adjustment(Some(label)),
                Err(CandleAdjustmentError::Unsupported(label.to_owned())),
                "unsupported adjustment {label:?}"
            );
        }
    }

    // Parity: go:452dea11:internal/api/marketdata/routes_boundaries_test.go:229 TestNormalizeOptionalQueryTimeAcceptsEmptyAndRejectsMalformedValues
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
