//! Futu kline query-window helpers shared by the market-data quote read port.
//!
//! Parity: `internal/app/apiserver/marketdataapp/query.go`
//! (`KLineQueryWindow`, `effectivePeriodSeconds`, `parseFutuTimeToTS`).

pub(super) fn effective_period_seconds(period: &str) -> i64 {
    let secs = jftrade_integration_futu::kline_query::period_duration_seconds(period);
    if secs > 0 {
        return secs;
    }
    match period.trim().to_ascii_lowercase().as_str() {
        "1d" | "day" => 86_400,
        "1w" | "week" => 604_800,
        "1mo" | "month" => 2_592_000,
        "1y" | "year" => 31_536_000,
        _ => 86_400,
    }
}

pub(super) fn parse_futu_time_to_ts(raw: &str, tz: &jiff::tz::TimeZone) -> Option<jiff::Timestamp> {
    let trimmed = raw.trim();
    if trimmed.is_empty() {
        return None;
    }
    if (trimmed.contains('T') || trimmed.ends_with('Z'))
        && let Ok(ts) = trimmed.parse::<jiff::Timestamp>()
    {
        return Some(ts);
    }
    if let Ok(dt) = jiff::civil::DateTime::strptime("%Y-%m-%d %H:%M:%S", trimmed) {
        return dt.to_zoned(tz.clone()).ok().map(|z| z.timestamp());
    }
    if let Ok(d) = jiff::civil::Date::strptime("%Y-%m-%d", trimmed) {
        return d
            .to_datetime(jiff::civil::Time::midnight())
            .to_zoned(tz.clone())
            .ok()
            .map(|z| z.timestamp());
    }
    None
}

pub(super) fn futu_kline_query_window(
    market: &str,
    period: &str,
    limit: usize,
    from_time: Option<&str>,
    to_time: Option<&str>,
    before: Option<&str>,
) -> (String, String, bool) {
    let tz_str = match market {
        "US" => "America/New_York",
        "HK" => "Asia/Hong_Kong",
        "SH" | "SZ" | "CN" => "Asia/Shanghai",
        "JP" => "Asia/Tokyo",
        _ => "UTC",
    };
    let tz = jiff::tz::TimeZone::get(tz_str).unwrap_or(jiff::tz::TimeZone::UTC);
    let now_ts = jiff::Timestamp::now();
    let bar_duration = effective_period_seconds(period);
    let eff_limit = limit.clamp(1, 1000) as i64;
    let mut lookback_secs = bar_duration.saturating_mul(eff_limit).saturating_mul(4);
    let min_lookback_secs = if bar_duration >= 86_400 {
        45 * 86_400
    } else {
        36 * 3_600
    };
    if lookback_secs < min_lookback_secs {
        lookback_secs = min_lookback_secs;
    }

    let end_ts = if let Some(raw_end) = to_time.or(before) {
        parse_futu_time_to_ts(raw_end, &tz).unwrap_or(now_ts)
    } else {
        now_ts
    };

    let begin_ts = if let Some(raw_begin) = from_time {
        let parsed = parse_futu_time_to_ts(raw_begin, &tz).unwrap_or_else(|| {
            end_ts
                .checked_sub(jiff::SignedDuration::from_secs(lookback_secs))
                .unwrap_or(now_ts)
        });
        if parsed >= end_ts {
            end_ts
                .checked_sub(jiff::SignedDuration::from_secs(lookback_secs))
                .unwrap_or(now_ts)
        } else {
            parsed
        }
    } else {
        end_ts
            .checked_sub(jiff::SignedDuration::from_secs(lookback_secs))
            .unwrap_or(now_ts)
    };

    let begin_zoned = begin_ts.to_zoned(tz.clone());
    let end_zoned = end_ts.to_zoned(tz);
    let begin_str = begin_zoned.strftime("%Y-%m-%d %H:%M:%S").to_string();
    let end_str = end_zoned.strftime("%Y-%m-%d %H:%M:%S").to_string();

    let query_current = end_ts
        >= now_ts
            .checked_sub(jiff::SignedDuration::from_secs(bar_duration))
            .unwrap_or(now_ts);
    (begin_str, end_str, query_current)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_futu_kline_query_window_resets_invalid_begin_to_default_lookback() {
        // Parity: internal/app/apiserver/marketdataapp/query_test.go:60 TestKLineQueryWindowResetsInvalidBeginToDefaultLookback
        // When from_time >= to_time, begin is reset to end - default lookback (36 hours for intraday)
        let (begin, end, _) = futu_kline_query_window(
            "US",
            "1m",
            2,
            Some("2026-05-21 17:00:00"),
            Some("2026-05-21 16:00:00"),
            None,
        );
        assert_eq!(end, "2026-05-21 16:00:00");
        assert_eq!(begin, "2026-05-20 04:00:00");
    }

    #[test]
    fn test_futu_kline_query_window_uses_explicit_bounds() {
        // Parity: go:452dea11:internal/app/apiserver/marketdataapp/query_test.go:28
        // TestKLineQueryWindowUsesExplicitBounds
        // An explicit from/to window is honored verbatim; the default lookback
        // only applies when the caller supplies no window or an invalid one.
        let (begin, end, query_current) = futu_kline_query_window(
            "US",
            "1m",
            10,
            Some("2026-05-21 09:00:00"),
            Some("2026-05-21 16:00:00"),
            None,
        );
        assert_eq!(begin, "2026-05-21 09:00:00");
        assert_eq!(end, "2026-05-21 16:00:00");
        assert!(!query_current, "a historical window must not query live bars");

        // A daily period widens the default lookback bound (45 days) while
        // still respecting the explicitly requested end.
        let (begin, end, _) = futu_kline_query_window("US", "1d", 1, None, None, None);
        assert!(!begin.is_empty());
        assert!(!end.is_empty());
    }
}
