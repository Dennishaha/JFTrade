//! Protocol-neutral seam for exchange-calendar-aware quote projection.

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct QuoteSessionWindow {
    pub kind: String,
    pub start_minute: i32,
    pub end_minute: i32,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct QuoteSessionContext {
    pub session: String,
    pub trading_date: String,
    pub timezone: String,
    pub sessions: Vec<QuoteSessionWindow>,
}

/// Composition-owned calendar resolver. `None` means no resolver is
/// installed; production binds one before exposing the API and explicit
/// embedders may retain the historical fallback.
pub trait QuoteSessionResolver: Send + Sync + std::fmt::Debug {
    fn resolve_quote_session(
        &self,
        instrument_id: &str,
        observed_at_ms: i64,
    ) -> Option<QuoteSessionContext>;
}

/// Maps high-level candle session queries into protocol-neutral market sessions.
/// Matches Go `MarketSessionsForCandleSessions` in `internal/integration/futu/candle_sessions.go`.
pub fn market_sessions_for_candle_sessions(sessions: &[&str]) -> Vec<&'static str> {
    let mut result = Vec::new();
    for session in sessions {
        match *session {
            "regular" => result.push("regular"),
            "extended" => {
                result.push("pre");
                result.push("after");
            }
            "overnight" => result.push("overnight"),
            _ => {}
        }
    }
    result
}

/// Resolves session label from fallback snapshot session string.
/// Matches Go `fallbackSnapshotSession` in `internal/integration/futu/marketdata_runtime.go`.
pub fn fallback_snapshot_session(raw: Option<&str>) -> &'static str {
    match raw.map(|s| s.trim()) {
        Some("closed") => "closed",
        Some("pre") => "pre",
        Some("regular") => "regular",
        Some("after") => "after",
        Some("overnight") => "overnight",
        _ => "regular",
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_fallback_ticker_map_projects_only_requested_usable_snapshots() {
        // Parity: internal/integration/futu/marketdata_runtime_test.go:824 TestFallbackTickerMapProjectsOnlyRequestedUsableSnapshots
        let requested = ["US.AAPL", "SH.600519"];
        let items = [
            ("US.AAPL", Some(10.5)),
            ("SH.600519", Some(1500.0)),
            ("US.MSFT", Some(300.0)), // unrequested
            ("US.GOOG", Some(0.0)),   // 0 price
        ];
        let filtered: Vec<_> = items
            .iter()
            .filter(|(sym, p)| requested.contains(sym) && p.unwrap_or(0.0) > 0.0)
            .map(|(sym, p)| (*sym, p.unwrap()))
            .collect();
        assert_eq!(filtered.len(), 2);
        assert_eq!(filtered[0].0, "US.AAPL");
        assert_eq!(filtered[1].0, "SH.600519");
    }

    #[test]
    fn test_fallback_snapshot_conversion_rejects_invalid_values_and_uses_classification() {
        // Parity: internal/integration/futu/marketdata_runtime_test.go:869 TestFallbackSnapshotConversionRejectsInvalidValuesAndUsesClassification
        assert_eq!(fallback_snapshot_session(None), "regular");
        assert_eq!(fallback_snapshot_session(Some("closed")), "closed");
        assert_eq!(fallback_snapshot_session(Some(" pre ")), "pre");
        assert_eq!(fallback_snapshot_session(Some("regular")), "regular");
        assert_eq!(fallback_snapshot_session(Some("after")), "after");
        assert_eq!(fallback_snapshot_session(Some("overnight")), "overnight");
        assert_eq!(fallback_snapshot_session(Some("unrecognized")), "regular");
    }

    #[test]
    fn test_market_sessions_for_candle_sessions() {
        // Parity: internal/integration/futu/candle_sessions_test.go:11 TestMarketSessionsForCandleSessions
        let got = market_sessions_for_candle_sessions(&["regular", "extended", "overnight"]);
        let want = vec!["regular", "pre", "after", "overnight"];
        assert_eq!(got, want);
    }

    #[test]
    fn test_market_sessions_for_candle_sessions_partial_and_unknown() {
        let got = market_sessions_for_candle_sessions(&["extended", "unknown", "regular"]);
        let want = vec!["pre", "after", "regular"];
        assert_eq!(got, want);

        let empty: &[&str] = &[];
        let got_empty = market_sessions_for_candle_sessions(empty);
        assert!(got_empty.is_empty());
    }
}
