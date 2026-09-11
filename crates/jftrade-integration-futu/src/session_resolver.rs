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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_market_sessions_for_candle_sessions() {
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
