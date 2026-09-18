use super::*;
use jftrade_integration_futu::{HistoricalKline, HistoricalKlineResult};

fn request(query: &str) -> super::super::TradeRequest {
    super::super::TradeRequest::parse("/api/v1/brokers/futu/klines", query).expect("request")
}

#[test]
fn sessions_default_and_validation_follow_go_extended_hours_rules() {
    let regular = request("symbol=HK.00700&period=1d");
    assert_eq!(
        parse_requested_sessions(&regular.query, false).expect("sessions"),
        vec!["regular"]
    );
    let extended = request("symbol=US.AAPL&period=5m");
    assert_eq!(
        parse_requested_sessions(&extended.query, true).expect("sessions"),
        vec!["regular", "extended", "overnight"]
    );
    let invalid = request("symbol=HK.00700&period=1d&sessions=extended");
    assert!(parse_requested_sessions(&invalid.query, false).is_err());
}

#[test]
fn session_selection_normalizes_aliases_and_rejects_unsupported_ones() {
    // Parity: go:452dea11:pkg/futu/marketdata_reader_boundaries_test.go:108
    // TestBrokerKLineSessionHelpersNormalizeAndRejectSelections.
    //
    // Go's `resolveBrokerKLineSessions` defaults to all four US sessions, splits
    // on commas and trims, orders the result regular/pre/after/overnight, and
    // rejects extended/overnight for a non-extended market as well as unknown
    // names. Rust's owner is `parse_requested_sessions`.
    let mixed = request("symbol=US.NVDA&period=1m&sessions=%20overnight,regular&sessions=extended");
    assert_eq!(
        parse_requested_sessions(&mixed.query, true).expect("normalized sessions"),
        vec!["regular", "extended", "overnight"],
        "the caller's order is normalized, not preserved"
    );

    // Go distinguishes "unsupported for this market" from "unknown session";
    // Rust maps both onto the same rejection, so assert the rejection itself.
    for (query_string, extended) in [
        ("symbol=HK.00700&period=1d&sessions=overnight", false),
        ("symbol=HK.00700&period=1d&sessions=extended", false),
        ("symbol=US.NVDA&period=1m&sessions=unknown", true),
    ] {
        let parsed = request(query_string);
        assert!(
            parse_requested_sessions(&parsed.query, extended).is_err(),
            "session query {query_string:?} must be rejected"
        );
    }

    // A non-extended market still accepts an explicit regular selection.
    let regular = request("symbol=HK.00700&period=1d&sessions=regular");
    assert_eq!(
        parse_requested_sessions(&regular.query, false).expect("regular"),
        vec!["regular"]
    );
}

#[test]
fn broker_kline_snapshot_session_fields_follow_go_classification_helpers() {
    // Parity: go:452dea11:pkg/futu/marketdata_reader_boundaries_test.go:126
    // `hasNonRegularBrokerSession` and `brokerKLineSessionLabel`.
    //
    // Go derives the broker KLines snapshot's `extendedHours` from the
    // *resolved* session selection, not from the market's extended-hours
    // capability: `sessions=regular` on a US intraday window answers
    // `extendedHours: false`, and any non-regular selection answers
    // `session: "all"` instead of echoing the token back.
    let result = HistoricalKlineResult {
        security: jftrade_integration_futu::HistoricalSecurity {
            market: 11,
            code: "AAPL".to_owned(),
        },
        name: None,
        klines: vec![],
        next_req_key: vec![],
    };
    let req = request("symbol=US.AAPL&period=5m");

    // Default US intraday selection: regular + pre/after + overnight.
    let default = historical_snapshot(
        &req,
        &result,
        "5m",
        true,
        &["regular", "extended", "overnight"],
        None,
    );
    assert_eq!(default["extendedHours"], true);
    assert_eq!(default["session"], "all");
    assert_eq!(
        default["sessions"],
        serde_json::json!(["regular", "extended", "overnight"])
    );

    // An explicit regular-only US intraday selection stays regular even
    // though the market supports extended hours.
    let regular = historical_snapshot(&req, &result, "5m", true, &["regular"], None);
    assert_eq!(regular["extendedHours"], false);
    assert_eq!(regular["session"], "regular");

    // A single extended/overnight selection still collapses to `all`.
    for sessions in [&["extended"][..], &["overnight"][..]] {
        let extended = historical_snapshot(&req, &result, "5m", true, sessions, None);
        assert_eq!(extended["extendedHours"], true);
        assert_eq!(extended["session"], "all", "sessions={sessions:?}");
    }

    // A non-extended market keeps the documented regular label.
    let hk = request("symbol=HK.00700&period=1d");
    let hk_snapshot = historical_snapshot(&hk, &result, "1d", false, &["regular"], None);
    assert_eq!(hk_snapshot["extendedHours"], false);
    assert_eq!(hk_snapshot["session"], "regular");
}

#[test]
fn market_time_conversion_uses_exchange_wall_clock() {
    assert_eq!(
        super::super::normalize_history_time("2026-08-01T00:00:00Z", "HK").expect("time"),
        "2026-08-01 08:00:00"
    );
    assert_eq!(
        super::super::normalize_history_time("2026-01-01T12:00:00Z", "US").expect("time"),
        "2026-01-01 07:00:00"
    );
    assert_eq!(
        super::super::normalize_history_time("2026-07-01T13:30:00Z", "US").expect("time"),
        "2026-07-01 09:30:00"
    );
    assert_eq!(
        canonical_candle_time("2026-03-08 01:30:00", "US"),
        "2026-03-08T06:30:00Z"
    );
    assert_eq!(
        canonical_candle_time("2026-03-08 03:30:00", "US"),
        "2026-03-08T07:30:00Z"
    );
}

#[test]
fn snapshot_pagination_requires_next_key_and_uses_earliest_candle() {
    let req = request("symbol=US.AAPL&period=5m");
    let result = HistoricalKlineResult {
        security: jftrade_integration_futu::HistoricalSecurity {
            market: 11,
            code: "AAPL".to_owned(),
        },
        name: None,
        klines: vec![HistoricalKline {
            time: "2026-08-01 09:30:00".to_owned(),
            is_blank: false,
            high_price: Some(3.0),
            open_price: Some(2.0),
            low_price: Some(1.0),
            close_price: Some(2.5),
            volume: Some(10),
            turnover: Some(20.0),
            change_rate: None,
        }],
        next_req_key: vec![1],
    };
    let snapshot = historical_snapshot(&req, &result, "5m", true, &["regular"], None);
    assert_eq!(snapshot["pagination"]["hasMore"], true);
    assert_eq!(snapshot["pagination"]["nextBefore"], "2026-08-01T13:30:00Z");
    assert_eq!(snapshot["klines"][0]["open"], 2.0);
}

#[test]
fn bounded_snapshot_suppresses_cursor_even_when_opend_has_more_pages() {
    let req = request("symbol=US.AAPL&period=5m&fromTime=2026-08-01T00:00:00Z");
    let result = HistoricalKlineResult {
        security: jftrade_integration_futu::HistoricalSecurity {
            market: 11,
            code: "AAPL".to_owned(),
        },
        name: None,
        klines: vec![],
        next_req_key: vec![1],
    };
    let snapshot = historical_snapshot(&req, &result, "5m", true, &["regular"], None);
    assert_eq!(
        snapshot["pagination"],
        serde_json::json!({"hasMore": false})
    );
}
