//! Parity tests for the OpenD push → LiveHub `market-data.tick` projection.
//!
//! Baseline: `go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go`
//! and `internal/marketdata/responses.go::TickEventDTO.JSON`.

use std::sync::Arc;
use std::time::Duration;

use jftrade_api::{LiveHub, LiveHubConnection};
use jftrade_integration_futu::OpenDSessionEventListener;
use jftrade_integration_futu::{
    BasicQuote, BasicQuotePush, OpenDSessionCoordinatorOutcome, QuotePush, Security,
};

use super::LiveHubOpenDEventListener;

fn push(security: Security, volume: Option<i64>, update_timestamp: Option<f64>) -> QuotePush {
    QuotePush::Basic(BasicQuotePush {
        quotes: vec![BasicQuote {
            security: Some(security),
            cur_price: Some(380.5),
            volume,
            update_time: Some("2026-07-20T10:00:00+08:00".to_owned()),
            update_timestamp,
            high_price: Some(381.0),
            low_price: Some(377.0),
            open_price: Some(378.0),
            last_close_price: Some(375.0),
            turnover: Some(1_000.0),
            turnover_rate: None,
            amplitude: None,
            dark_status: None,
            list_timestamp: None,
            name: None,
            is_suspended: None,
            list_time: None,
            price_spread: None,
            pre_market: None,
            after_market: None,
            sec_status: None,
            overnight: None,
            hp_volume: None,
        }],
    })
}

fn hk(volume: Option<i64>, update_timestamp: Option<f64>) -> QuotePush {
    push(
        Security {
            market: Some(1),
            code: Some("00700".to_owned()),
        },
        volume,
        update_timestamp,
    )
}

/// Builds a hub with one subscribed client, mirroring the websocket shell.
fn subscribed(
    instruments: &[&str],
) -> (Arc<LiveHub>, LiveHubOpenDEventListener, LiveHubConnection) {
    let hub = Arc::new(LiveHub::new(16));
    let listener = LiveHubOpenDEventListener::with_reconciliation_wake(
        Arc::clone(&hub),
        Arc::new(tokio::sync::Notify::new()),
    );
    let connection = hub.connect();
    connection.set_subscription(
        "futu",
        &instruments
            .iter()
            .map(|value| (*value).to_owned())
            .collect::<Vec<_>>(),
    );
    (hub, listener, connection)
}

/// Reads the next delivered event, bounded so filtering bugs fail fast.
async fn next_event(connection: &mut LiveHubConnection) -> Option<serde_json::Value> {
    tokio::time::timeout(Duration::from_secs(1), connection.recv())
        .await
        .ok()
        .flatten()
}

// Parity: go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go:188
// TestStreamMarketTradeCarriesDeltaAndCumulativeVolume
#[tokio::test]
async fn basic_quote_pushes_publish_delta_and_cumulative_volume() {
    let (_hub, listener, mut connection) = subscribed(&["HK.00700"]);
    for (volume, at) in [(1_000, 1_784_518_800.0), (1_015, 1_784_518_801.0)] {
        listener.on_event(&OpenDSessionCoordinatorOutcome::Push(hk(
            Some(volume),
            Some(at),
        )));
    }

    let first = next_event(&mut connection).await.expect("first tick");
    assert_eq!(first["type"], "market-data.tick");
    assert_eq!(first["entityId"], "HK.00700");
    assert_eq!(first["payload"]["brokerId"], "futu");
    assert_eq!(first["payload"]["cumulativeVolume"], "1000");
    assert_eq!(
        first["payload"]["volumeDelta"], "0",
        "the first cumulative sample is only a baseline"
    );
    assert_eq!(first["payload"]["instrument"]["market"], "HK");
    assert_eq!(first["payload"]["instrument"]["symbol"], "00700");
    assert_eq!(first["payload"]["snapshot"]["price"], "380.5");
    assert_eq!(first["payload"]["snapshot"]["volume"], "1000");

    let second = next_event(&mut connection).await.expect("second tick");
    assert_eq!(second["payload"]["cumulativeVolume"], "1015");
    assert_eq!(second["payload"]["volumeDelta"], "15");
}

// Parity: go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go:215
// TestStreamMarketTradePreservesVolumeBeyondLegacyFixedpointRange
#[tokio::test]
async fn basic_quote_pushes_keep_exact_volume_beyond_fixedpoint_range() {
    let (_hub, listener, mut connection) = subscribed(&["HK.00700"]);
    for (volume, at) in [
        (9_007_199_254_740_993, 1_784_518_800.0),
        (9_007_199_254_740_995, 1_784_518_801.0),
    ] {
        listener.on_event(&OpenDSessionCoordinatorOutcome::Push(hk(
            Some(volume),
            Some(at),
        )));
    }

    let _first = next_event(&mut connection).await.expect("first tick");
    let second = next_event(&mut connection).await.expect("second tick");
    assert_eq!(second["payload"]["cumulativeVolume"], "9007199254740995");
    assert_eq!(
        second["payload"]["volumeDelta"], "2",
        "the delta must stay exact beyond 2^53"
    );
}

#[tokio::test]
async fn basic_quote_pushes_without_a_volume_counter_report_zero_delta() {
    let (_hub, listener, mut connection) = subscribed(&["HK.00700"]);
    listener.on_event(&OpenDSessionCoordinatorOutcome::Push(hk(
        None,
        Some(1_784_518_800.0),
    )));
    let event = next_event(&mut connection).await.expect("tick");
    assert_eq!(event["payload"]["volumeDelta"], "0");
    assert!(event["payload"]["cumulativeVolume"].is_null());
    assert!(event["payload"]["snapshot"]["volume"].is_null());
}

// Parity: go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go:115
// TestStreamPushHandlersRejectInactiveMalformedAndEmptyQuotes
#[tokio::test]
async fn basic_quote_pushes_drop_rows_without_a_usable_security_or_price() {
    let (_hub, listener, mut connection) = subscribed(&["HK.00700"]);

    // Unknown market code (Go's Market=-1 fixture).
    listener.on_event(&OpenDSessionCoordinatorOutcome::Push(push(
        Security {
            market: Some(-1),
            code: Some("BAD".to_owned()),
        },
        Some(10),
        Some(1_784_518_800.0),
    )));
    // Zero price (Go's `CurPrice: 0.0` fixture).
    let mut zero_price = hk(Some(10), Some(1_784_518_800.0));
    if let QuotePush::Basic(push) = &mut zero_price {
        push.quotes[0].cur_price = Some(0.0);
    }
    listener.on_event(&OpenDSessionCoordinatorOutcome::Push(zero_price));

    assert!(
        tokio::time::timeout(Duration::from_millis(50), connection.recv())
            .await
            .is_err(),
        "unusable rows must not reach the live hub"
    );
}

#[tokio::test]
async fn a_session_change_resets_the_cumulative_volume_baseline() {
    let (_hub, listener, mut connection) = subscribed(&["US.AAPL"]);
    let us = |volume: i64, at: f64| {
        push(
            Security {
                market: Some(11),
                code: Some("AAPL".to_owned()),
            },
            Some(volume),
            Some(at),
        )
    };
    // 2026-07-20T10:00 America/New_York is the regular session and
    // 2026-07-20T17:00 America/New_York is after-hours.
    listener.on_event(&OpenDSessionCoordinatorOutcome::Push(us(
        1_000,
        1_784_556_000.0,
    )));
    listener.on_event(&OpenDSessionCoordinatorOutcome::Push(us(
        1_015,
        1_784_581_200.0,
    )));

    let first = next_event(&mut connection).await.expect("regular tick");
    let second = next_event(&mut connection).await.expect("after-hours tick");
    assert_eq!(first["payload"]["snapshot"]["session"], "regular");
    assert_eq!(second["payload"]["snapshot"]["session"], "after");
    assert_eq!(
        second["payload"]["volumeDelta"], "0",
        "a session change resets the baseline like Go's (tradingDay, session) key"
    );
}
