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
