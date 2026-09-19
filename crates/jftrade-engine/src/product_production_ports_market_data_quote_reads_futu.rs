//! Futu kline query-window helpers shared by the market-data quote read port.
//!
//! Parity: `internal/app/apiserver/marketdataapp/query.go`
//! (`KLineQueryWindow`, `effectivePeriodSeconds`, `parseFutuTimeToTS`).

use crate::product::product_production_ports::product_production_ports_trade::canonical_candle_time;
use super::quote_reads::calendar_session_for_route;

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

/// Go's `queryBrokerKLines` rejects a reversed explicit window before touching
/// the provider; without this the provider clamps the window and answers with
/// unrelated candles, which hides the caller bug.
pub(super) fn reject_reversed_time_window(
    from_time: Option<&str>,
    to_time: Option<&str>,
) -> Result<(), crate::product::MarketDataQuoteReadSnapshotError> {
    use crate::product::MarketDataQuoteReadSnapshotError;

    let (Some(from), Some(to)) = (from_time, to_time) else {
        return Ok(());
    };
    let (Ok(from_at), Ok(to_at)) = (
        time::OffsetDateTime::parse(from, &time::format_description::well_known::Rfc3339),
        time::OffsetDateTime::parse(to, &time::format_description::well_known::Rfc3339),
    ) else {
        return Ok(());
    };
    if from_at > to_at {
        return Err(MarketDataQuoteReadSnapshotError::Failed {
            status: 400,
            code: "BAD_REQUEST".to_owned(),
            message: "fromTime must be earlier than or equal to toTime".to_owned(),
            retry_after_seconds: None,
        });
    }
    Ok(())
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

/// Go `filterKLinesByWindow`: trim the `Qot_GetKL` current-unclosed bucket to
/// the caller's explicit `[begin, end]` window before it is merged into the
/// historical series.
///
/// Go's `QueryKLines` merges the current bucket through
/// `mergeKLinesByStartTime(klines, filterKLinesByWindow(currentKLines, beginAt, endAt))`.
/// The `Qot_GetKL` request can only express `reqNum` (Go always asks for 2:
/// the previous closed bucket plus the unfinished one), so the window has to be
/// enforced on the response. Without this, an explicit `from`/`to` window still
/// received the current unfinished bucket even when the window excluded it.
///
/// Go drops a candle when its finish is before `begin` or its start is after
/// `end`. OpenD labels an intraday candle by the end of its bucket, and the
/// engine's canonical time is that start (label minus one period), so the
/// comparison here uses the canonical start against the window end plus the
/// same period as the candle's finish. Unparsable bounds or candle labels keep
/// the candle: Go only trims what it can compare.
pub(super) fn filter_current_klines_by_window(
    klines: &[jftrade_integration_futu::HistoricalKline],
    market: &str,
    period: &str,
    begin_time: &str,
    end_time: &str,
) -> Vec<jftrade_integration_futu::HistoricalKline> {
    let period_secs = effective_period_seconds(period).max(1);
    let window_start = parse_window_bound(begin_time, market)
        .unwrap_or(time::OffsetDateTime::UNIX_EPOCH);
    let Some(window_end) = parse_window_bound(end_time, market) else {
        return klines.to_vec();
    };
    klines
        .iter()
        .filter(|kline| {
            let Some(start) = parse_window_bound(&kline.time, market) else {
                return true;
            };
            let finish = start + time::Duration::seconds(period_secs);
            finish >= window_start && start <= window_end
        })
        .cloned()
        .collect()
}

/// Parse one OpenD wall-clock label (or RFC3339 bound) into an instant.
fn parse_window_bound(value: &str, market: &str) -> Option<time::OffsetDateTime> {
    let canonical = canonical_candle_time(value, market);
    time::OffsetDateTime::parse(&canonical, &time::format_description::well_known::Rfc3339).ok()
}

/// Go `filterKLinesBySessions`: drop the merged candles whose exchange-calendar
/// session is not in the caller's requested set.
///
/// Go's `QueryKLinesForSessions` runs this **twice**: once over the historical
/// pages inside the routing loop and again over the merged list
/// (`if filterBySession { klines = filterKLinesBySessions(klines, sessions) }`)
/// after the `Qot_GetKL` current bucket has been merged in. The second pass is
/// what keeps `QueryKLinesForSessions(..., [regular])` from handing back a
/// pre/after-hours current bar: the routed history pages are already filtered
/// per route, but the current bucket arrives un-routed and would otherwise slip
/// through.
///
/// Candles the schedule cannot classify are kept, matching the page filter so a
/// missing calendar never silently empties the caller's result.
pub(super) fn filter_klines_by_sessions(
    klines: &[jftrade_integration_futu::HistoricalKline],
    calendar: Option<&jftrade_calendar::CalendarManager>,
    market: &str,
    period: &str,
    sessions: &[&str],
) -> Vec<jftrade_integration_futu::HistoricalKline> {
    let Some(calendar) = calendar else {
        return klines.to_vec();
    };
    if sessions.is_empty() {
        return klines.to_vec();
    }
    // The caller-facing session vocabulary is the engine's (`regular` /
    // `extended` / `overnight`), while the calendar returns the four-way market
    // session. Go compares against the caller's set directly; the same
    // projection is applied here so `extended` covers both pre and after.
    let keep = |session: jftrade_integration_futu::MarketSession| match session {
        jftrade_integration_futu::MarketSession::Regular => sessions.contains(&"regular"),
        jftrade_integration_futu::MarketSession::Pre
        | jftrade_integration_futu::MarketSession::After => sessions.contains(&"extended"),
        jftrade_integration_futu::MarketSession::Overnight => sessions.contains(&"overnight"),
    };
    let _ = period;
    klines
        .iter()
        .filter(|kline| {
            let at = canonical_candle_time(&kline.time, market);
            // Go's `filterKLinesBySessions` compares the clock session against
            // the requested set, so a bar the calendar cannot classify is not
            // a member of any requested session and must be dropped.
            let Some(session) = calendar_session_for_route(Some(calendar), market, &at) else {
                return false;
            };
            keep(session)
        })
        .cloned()
        .collect()
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

    #[test]
    fn broker_kline_pagination_helpers_cover_sessions_bounds_and_listing_dates() {
        // Parity: go:8a78fc78:pkg/futu/adapter_kline_pagination_test.go:284
        // TestBrokerKLinePaginationHelpersCoverSessionsBoundsAndListingDates.
        //
        // Identical bounds collapse to the documented lookback rather than
        // producing a reversed window; an invalid from-time falls back the same
        // way; listing-date parsing accepts both timestamp and date forms and
        // returns None (Go falls back to 1900) for garbage.
        let (begin, end, _) = futu_kline_query_window(
            "HK",
            "1d",
            10,
            Some("2026-07-18T00:00:00Z"),
            Some("2026-07-18T00:00:00Z"),
            None,
        );
        assert_eq!(end, "2026-07-18 08:00:00");
        assert!(
            begin < end,
            "identical bounds must collapse to the lookback: {begin}..{end}"
        );

        let (fallback_begin, fallback_end, _) = futu_kline_query_window(
            "HK",
            "1m",
            10,
            Some("not-a-time"),
            Some("2026-07-18T04:00:00Z"),
            None,
        );
        assert_eq!(fallback_end, "2026-07-18 12:00:00");
        assert!(fallback_begin < fallback_end);

        let tz = jiff::tz::TimeZone::UTC;
        assert_eq!(
            parse_futu_time_to_ts("2004-06-16", &tz).map(|ts| ts.strftime("%Y-%m-%d").to_string()),
            Some("2004-06-16".to_owned())
        );
        assert!(parse_futu_time_to_ts("not-a-date", &tz).is_none());
        assert!(parse_futu_time_to_ts("2004-06-16 09:30:00", &tz).is_some());
    }
}
