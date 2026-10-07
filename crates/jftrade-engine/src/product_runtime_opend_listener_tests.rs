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
use jftrade_integration_futu::{OrderBookLevel, OrderBookPush};

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

fn depth() -> QuotePush {
    QuotePush::OrderBook(OrderBookPush {
        security: Some(Security {
            market: Some(1),
            code: Some("00700".to_owned()),
        }),
        name: Some("Tencent".to_owned()),
        asks: vec![OrderBookLevel {
            price: Some(320.0),
            volume: Some(100),
            order_count: Some(1),
            details: Vec::new(),
            high_precision_volume: None,
        }],
        bids: vec![OrderBookLevel {
            price: Some(319.0),
            volume: Some(150),
            order_count: Some(1),
            details: Vec::new(),
            high_precision_volume: None,
        }],
        server_receive_time_bid: Some("2025-01-01 10:00:00.000".to_owned()),
        server_receive_time_bid_timestamp: None,
        server_receive_time_ask: Some("2025-01-01 10:00:01.000".to_owned()),
        server_receive_time_ask_timestamp: None,
        order_book_type: None,
    })
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

// Parity: go:452dea11:internal/app/apiserver/servercore/market_depth_test.go:33 TestMarketDepthWebSocketSendsInitialPayload
// Parity: go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go:188
// TestStreamMarketTradeCarriesDeltaAndCumulativeVolume
#[tokio::test]
// Parity: go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go:292 TestOrderBookStreamConnectionBoundaries
async fn order_book_pushes_publish_depth_for_the_subscribed_instrument() {
    // Parity: go:452dea11:pkg/futu/exchange_business_boundary_test.go:277
    // TestExchangeLocalMarketAndOrderBookHandlerBoundaries. Go registers a
    // per-symbol order-book callback and notifies "HK.00700" exactly once;
    // Rust publishes a market.depth envelope scoped to that instrument, and
    // LiveHub delivers it only to subscribers of the same instrument.
    let (_hub, listener, mut connection) = subscribed(&["HK.00700"]);
    listener.on_event(&OpenDSessionCoordinatorOutcome::Push(depth()));
    let event = next_event(&mut connection).await.expect("depth event");
    assert_eq!(event["type"], "market.depth");
    assert_eq!(event["entityId"], "HK.00700");
    assert_eq!(event["payload"]["instrumentId"], "HK.00700");
    assert_eq!(event["payload"]["depth"]["symbol"], "HK.00700");
    assert_eq!(event["payload"]["depth"]["asks"][0]["price"], 320.0);
    assert_eq!(event["payload"]["depth"]["bids"][0]["price"], 319.0);
    assert_eq!(event["payload"]["depth"]["asks"][0]["volume"], 100.0);

    // A push for an unsubscribed symbol must not leak to this connection.
    listener.on_event(&OpenDSessionCoordinatorOutcome::Push(QuotePush::OrderBook(
        OrderBookPush {
            security: Some(Security {
                market: Some(1),
                code: Some("00005".to_owned()),
            }),
            name: None,
            asks: Vec::new(),
            bids: Vec::new(),
            server_receive_time_bid: Some("2025-01-01 10:00:02.000".to_owned()),
            server_receive_time_bid_timestamp: None,
            server_receive_time_ask: None,
            server_receive_time_ask_timestamp: None,
            order_book_type: None,
        },
    )));
    assert!(
        next_event(&mut connection).await.is_none(),
        "unsubscribed order-book pushes must not reach this connection"
    );
}

// Parity: go:452dea11:pkg/futu/stream_orderbook.go:48 handleOrderBookPush
#[tokio::test]
// Parity: go:452dea11:pkg/futu/exchange_orderbook_test.go:363 TestHandleOrderBookPushEmitsSingleCompleteBookTicker
async fn order_book_pushes_no_longer_require_server_receive_times() {
    // Go's handler only needs a resolvable security plus at least one non-zero
    // best price; it never reads `SvrRecvTimeBid`/`SvrRecvTimeAsk`. Rust must
    // not silently drop a push whose timestamps are absent.
    let (_hub, listener, mut connection) = subscribed(&["HK.00700"]);
    listener.on_event(&OpenDSessionCoordinatorOutcome::Push(QuotePush::OrderBook(
        OrderBookPush {
            security: Some(Security {
                market: Some(1),
                code: Some("00700".to_owned()),
            }),
            name: None,
            asks: vec![OrderBookLevel {
                price: Some(320.0),
                volume: Some(100),
                order_count: Some(1),
                details: Vec::new(),
                high_precision_volume: None,
            }],
            bids: vec![OrderBookLevel {
                price: Some(319.0),
                volume: Some(150),
                order_count: Some(1),
                details: Vec::new(),
                high_precision_volume: None,
            }],
            server_receive_time_bid: None,
            server_receive_time_bid_timestamp: None,
            server_receive_time_ask: None,
            server_receive_time_ask_timestamp: None,
            order_book_type: None,
        },
    )));
    let event = next_event(&mut connection)
        .await
        .expect("a push without server receive times must still publish");
    assert_eq!(event["type"], "market.depth");
    assert_eq!(event["entityId"], "HK.00700");
    assert_eq!(event["payload"]["depth"]["bids"][0]["price"], 319.0);
    assert_eq!(event["payload"]["depth"]["asks"][0]["price"], 320.0);
}

// Parity: go:452dea11:pkg/futu/stream_orderbook.go:48 handleOrderBookPush
#[tokio::test]
async fn order_book_pushes_without_any_price_are_dropped() {
    // Go builds a BookTicker and returns before emitting when both best bid and
    // best ask are zero. Rust must not publish an all-empty depth envelope.
    let (_hub, listener, mut connection) = subscribed(&["HK.00700"]);
    listener.on_event(&OpenDSessionCoordinatorOutcome::Push(QuotePush::OrderBook(
        OrderBookPush {
            security: Some(Security {
                market: Some(1),
                code: Some("00700".to_owned()),
            }),
            name: None,
            asks: Vec::new(),
            bids: Vec::new(),
            server_receive_time_bid: Some("2025-01-01 10:00:00.000".to_owned()),
            server_receive_time_bid_timestamp: None,
            server_receive_time_ask: None,
            server_receive_time_ask_timestamp: None,
            order_book_type: None,
        },
    )));
    assert!(
        next_event(&mut connection).await.is_none(),
        "a push without any best price must not publish an empty depth event"
    );
}

// Parity: go:452dea11:pkg/futu/stream_orderbook.go:48 handleOrderBookPush
#[tokio::test]
async fn order_book_pushes_with_zero_best_prices_do_not_publish_deeper_levels() {
    let (hub, listener, mut connection) = subscribed(&["HK.00700"]);
    for (bid, ask) in [(Some(0.0), Some(-0.0)), (None, None)] {
        let QuotePush::OrderBook(mut book) = depth() else {
            unreachable!()
        };
        book.bids.push(book.bids[0].clone());
        book.asks.push(book.asks[0].clone());
        book.bids[0].price = bid;
        book.asks[0].price = ask;
        listener.on_event(&OpenDSessionCoordinatorOutcome::Push(QuotePush::OrderBook(
            book,
        )));
        // An ordered control marker proves that no depth frame preceded it.
        // Non-zero deeper levels must not replace either zero best price.
        let marker = serde_json::json!({"type":"stale","payload":{"reason":"depth-marker"}});
        assert!(hub.publish(marker.clone()));
        assert_eq!(next_event(&mut connection).await, Some(marker));
    }
}

// Parity: go:452dea11:pkg/futu/exchange_orderbook_test.go:363 TestHandleOrderBookPushEmitsSingleCompleteBookTicker
#[tokio::test]
async fn order_book_pushes_emit_one_original_best_bid_ask_snapshot() {
    let (hub, listener, mut connection) = subscribed(&["HK.00700"]);
    let QuotePush::OrderBook(mut book) = depth() else {
        unreachable!()
    };
    book.bids[0].price = Some(700.1);
    book.bids[0].volume = Some(1200);
    book.asks[0].price = Some(700.2);
    book.asks[0].volume = Some(800);
    book.server_receive_time_bid = None;
    book.server_receive_time_ask = None;
    listener.on_event(&OpenDSessionCoordinatorOutcome::Push(QuotePush::OrderBook(
        book,
    )));
    let marker = serde_json::json!({"type":"stale","payload":{"reason":"snapshot-marker"}});
    assert!(hub.publish(marker.clone()));
    let event = next_event(&mut connection).await.expect("complete depth");
    assert_eq!(event["type"], "market.depth");
    assert_eq!(event["payload"]["depth"]["symbol"], "HK.00700");
    assert_eq!(event["payload"]["depth"]["bids"][0]["price"], 700.1);
    assert_eq!(event["payload"]["depth"]["bids"][0]["volume"], 1200);
    assert_eq!(event["payload"]["depth"]["asks"][0]["price"], 700.2);
    assert_eq!(event["payload"]["depth"]["asks"][0]["volume"], 800);
    assert_eq!(
        next_event(&mut connection).await,
        Some(marker),
        "one push produces one complete frame"
    );
}

#[tokio::test]
async fn order_book_pushes_publish_when_either_best_side_has_a_price() {
    let (_hub, listener, mut connection) = subscribed(&["HK.00700"]);
    for (bid, ask) in [(Some(319.0), None), (None, Some(320.0))] {
        let QuotePush::OrderBook(mut book) = depth() else {
            unreachable!()
        };
        book.bids[0].price = bid;
        book.asks[0].price = ask;
        listener.on_event(&OpenDSessionCoordinatorOutcome::Push(QuotePush::OrderBook(
            book,
        )));
        let event = next_event(&mut connection).await.expect("one priced side");
        assert_eq!(event["type"], "market.depth");
        assert_eq!(
            event["payload"]["depth"]["bids"]
                .as_array()
                .expect("bids")
                .len(),
            usize::from(bid.is_some())
        );
        assert_eq!(
            event["payload"]["depth"]["asks"]
                .as_array()
                .expect("asks")
                .len(),
            usize::from(ask.is_some())
        );
    }
}

// Parity: go:452dea11:pkg/futu/exchange_kline_test.go:428
// TestStreamConnectEmitsBasicQotPushAsBBGOEvents.
//
// Go connects a stream, registers exactly one subscription
// (`stream.Subscribe(types.MarketTradeChannel, "HK.00700")`), and the fake OpenD
// session then pushes a BasicQot and an OrderBook frame for that symbol. Both
// must reach the same consumer: the trade carries Go's
// `Price=700/Quantity=0/CumulativeVolume=1000` contract (the first cumulative
// sample is only a baseline, so the per-event delta is zero) and the book
// ticker exposes the pushed best bid/ask. Rust's owner is the
// composition-held OpenD listener: one live subscription for "HK.00700" must
// deliver the `market-data.tick` and `market.depth` envelopes from both push
// kinds, which is why this test reuses a single `subscribed()` connection.
#[tokio::test]
// Parity: go:452dea11:internal/app/apiserver/servercore/server_market_test.go:11 TestMarketDataSubscriptionHeartbeat
async fn one_live_subscription_publishes_both_trade_and_depth_pushes() {
    let (_hub, listener, mut connection) = subscribed(&["HK.00700"]);

    // Go's fake server answers the BasicQot push with Last=700 and the
    // OrderBook push with best bid == best ask == 700.
    let mut trade_push = hk(Some(1_000), Some(1_784_518_800.0));
    if let QuotePush::Basic(push) = &mut trade_push {
        push.quotes[0].cur_price = Some(700.0);
    }
    let mut depth_push = depth();
    if let QuotePush::OrderBook(push) = &mut depth_push {
        push.bids[0].price = Some(700.0);
        push.asks[0].price = Some(700.0);
    }
    listener.on_event(&OpenDSessionCoordinatorOutcome::Push(trade_push));
    listener.on_event(&OpenDSessionCoordinatorOutcome::Push(depth_push));

    let trade = next_event(&mut connection).await.expect("trade push");
    assert_eq!(trade["type"], "market-data.tick");
    assert_eq!(trade["entityId"], "HK.00700");
    assert_eq!(trade["payload"]["instrument"]["symbol"], "00700");
    assert_eq!(trade["payload"]["snapshot"]["price"], "700");
    assert_eq!(trade["payload"]["cumulativeVolume"], "1000");
    assert_eq!(
        trade["payload"]["volumeDelta"], "0",
        "Go publishes Quantity=0 for the first cumulative sample"
    );

    let book = next_event(&mut connection).await.expect("book ticker push");
    assert_eq!(book["type"], "market.depth");
    assert_eq!(book["entityId"], "HK.00700");
    assert_eq!(book["payload"]["depth"]["bids"][0]["price"], 700.0);
    assert_eq!(book["payload"]["depth"]["asks"][0]["price"], 700.0);
}

// Parity: go:452dea11:internal/app/apiserver/servercore/live_adapter_volume_test.go:11 TestMarketTradeFromTickUsesExplicitVolumeDelta
// Parity: go:452dea11:internal/app/apiserver/servercore/ws_events_test.go:152 TestLiveWebSocketInitialMarketTickRefreshesObservedAt
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

// Parity: go:452dea11:internal/app/apiserver/servercore/live_adapter_volume_test.go:29 TestMarketTradeFromTickKeepsDecimalVolumeWhenLegacyQuantityOverflows
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

// Parity: go:452dea11:internal/app/apiserver/servercore/live_adapter_volume_test.go:49 TestMarketTradeFromTickRejectsAmbiguousOrInvalidDelta
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

// Parity: go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go:239
// TestStreamRejectsNegativeSnapshotVolume
//
// Go's `emitBasicQotSnapshot` returns before `EmitMarketTrade` when the
// cumulative counter is negative, so the trade-event count stays zero. The
// book-ticker update is published first and is not part of that count, so the
// assertion is specifically "no `market-data.tick` event".
#[tokio::test]
async fn negative_cumulative_volume_publishes_no_trade_event() {
    let (_hub, listener, mut connection) = subscribed(&["HK.00700"]);
    listener.on_event(&OpenDSessionCoordinatorOutcome::Push(hk(
        Some(-1),
        Some(1_784_518_800.0),
    )));
    assert!(
        tokio::time::timeout(Duration::from_millis(50), connection.recv())
            .await
            .is_err(),
        "a negative cumulative counter must not publish a trade event"
    );

    // The rejected sample must also leave the retained baseline untouched, so
    // the next valid sample still measures from the last accepted counter.
    listener.on_event(&OpenDSessionCoordinatorOutcome::Push(hk(
        Some(1_000),
        Some(1_784_518_801.0),
    )));
    let baseline = next_event(&mut connection).await.expect("baseline tick");
    assert_eq!(baseline["payload"]["volumeDelta"], "0");

    listener.on_event(&OpenDSessionCoordinatorOutcome::Push(hk(
        Some(1_015),
        Some(1_784_518_802.0),
    )));
    let delta = next_event(&mut connection).await.expect("delta tick");
    assert_eq!(
        delta["payload"]["volumeDelta"], "15",
        "the negative sample must not consume the baseline"
    );
    assert_eq!(delta["payload"]["cumulativeVolume"], "1015");
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

// The reconnect path below is distinct from the subscribe-triggered console refresh
// (see ws-live corpus subscription-event-order-and-normalization): it fires on
// coordinator Reconnected outcomes with a market-data source, not on subscribe.
// Parity: go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go:317
// TestStreamReconnectAndClientWatcherExitPaths.
//
// Go's `reconnectLoop` reacts to `ReconnectC` by re-running `OnConnect`, while
// `watchClientLoop` raises `ReconnectC` when the watched client dies. The Rust
// counterpart of `OnConnect` is the coordinator's `Reconnected` outcome held by
// the composition-owned listener: it must wake reconciliation and publish the
// resync events the console already consumes, and it must do so only for a
// genuine reconnect.
#[tokio::test]
async fn provider_reconnect_publishes_resync_events_and_wakes_reconciliation() {
    let hub = Arc::new(LiveHub::new(16));
    let reconciliation_wake = Arc::new(tokio::sync::Notify::new());
    let listener = LiveHubOpenDEventListener::with_reconciliation_wake(
        Arc::clone(&hub),
        Arc::clone(&reconciliation_wake),
    );
    let mut connection = hub.connect();
    connection.set_subscription("futu", &["HK.00700".to_owned()]);

    listener.on_event(&OpenDSessionCoordinatorOutcome::Reconnected {
        generation: 2,
        reason: jftrade_integration_futu::OpenDSessionCloseReason::PeerClosed,
    });

    let resync = next_event(&mut connection).await.expect("resync event");
    assert_eq!(resync["type"], "market-data.resync");
    assert_eq!(resync["entityId"], "futu");
    assert_eq!(resync["payload"]["reason"], "provider_reconnected");
    assert_eq!(resync["payload"]["action"], "refresh");
    let refresh = next_event(&mut connection).await.expect("refresh event");
    assert_eq!(refresh["type"], "console.refresh");
    assert_eq!(refresh["payload"]["scope"], "market-data");

    // Go's reconnect handler also re-arms reconciliation; the wake must be
    // observable without blocking, which `notify_one` guarantees by storing a
    // permit when no waiter is parked yet.
    tokio::time::timeout(Duration::from_millis(100), reconciliation_wake.notified())
        .await
        .expect("reconnect must wake reconciliation");

    // A plain disconnect is a different outcome: it publishes `stale` and must
    // not pretend a reconnect happened.
    listener.on_event(&OpenDSessionCoordinatorOutcome::Dropped);
    let stale = next_event(&mut connection).await.expect("stale event");
    assert_eq!(stale["type"], "market-data.stale");
    assert_eq!(stale["payload"]["reason"], "provider_disconnected");
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
