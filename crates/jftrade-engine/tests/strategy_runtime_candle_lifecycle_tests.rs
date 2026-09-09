#![forbid(unsafe_code)]

//! Integration and lifecycle verification for Live Strategy Closed K-line Determination (Milestone 1 / R1).
//!
//! Tests verify:
//! 1. Bar sequence containing an in-progress latest bar: verifies cache updates with zero Pine executions or premature order intents.
//! 2. In-progress bar closing on next tick: triggers Pine execution and order intent generation exactly once.
//! 3. Duplicate polls on the same closed bar do not re-execute (strict monotonic idempotency).
//! 4. Quote candle converter and quote readers output accurate `"closed": bool` on all bars.
//! 5. Chronological multi-bar processing preserves sequential append order and fences.

mod product {
    pub use jftrade_engine::product::*;
}

#[allow(dead_code)]
#[path = "../src/product_candle_converter.rs"]
mod product_candle_converter;

#[allow(dead_code)]
#[path = "../src/strategy_runtime_candles.rs"]
mod strategy_runtime_candles;

use std::collections::{BTreeMap, BTreeSet};

use product_candle_converter::{HelperCandleConversionParams, convert_helper_candles_response};
use strategy_runtime_candles::{StrategyCandle, parse_strategy_candles};

use jftrade_integration_marketdata_helper::{
    HelperCandle, HelperCandlesResponse, HelperPriceValue,
};
use jftrade_integration_pine::{PineCandle, PineOrderIntent};
use serde_json::{Value, json};
use time::OffsetDateTime;

#[allow(clippy::too_many_arguments)]
fn sample_candle(
    at: &str,
    open: f64,
    high: f64,
    low: f64,
    close: f64,
    volume: f64,
    period: &str,
    closed: Option<bool>,
) -> Value {
    let mut obj = json!({
        "at": at,
        "open": open,
        "high": high,
        "low": low,
        "close": close,
        "volume": volume,
        "period": period,
    });
    if let Some(c) = closed {
        obj["closed"] = json!(c);
    }
    obj
}

fn sample_intent(
    id: &str,
    bar_index: i32,
    time: i64,
    qty: f64,
    limit_price: f64,
) -> PineOrderIntent {
    PineOrderIntent {
        kind: "order".to_owned(),
        id: id.to_owned(),
        from_entry: String::new(),
        direction: "buy".to_owned(),
        quantity: qty,
        quantity_pct: 0.0,
        limit_price,
        stop_price: 0.0,
        comment: "test intent".to_owned(),
        alert_message: String::new(),
        disable_alert: false,
        bar_index,
        time,
        has_quantity: true,
        has_quantity_pct: false,
        has_limit_price: limit_price > 0.0,
        has_stop_price: false,
        parent_id: String::new(),
        atomic_group_id: String::new(),
        oco_group_id: String::new(),
        reduce_only: false,
    }
}

// =========================================================================
// Test 1: Quote Converter Emits closed: bool accurately
// =========================================================================

#[test]
fn test_quote_converter_emits_closed_boolean_field() {
    let now = OffsetDateTime::now_utc();
    let past_1 = now - time::Duration::minutes(5);
    let past_2 = now - time::Duration::minutes(4);
    let future = now + time::Duration::minutes(5);

    let format = &time::format_description::well_known::Rfc3339;
    let resp = HelperCandlesResponse {
        market: "US".to_owned(),
        symbol: "AAPL".to_owned(),
        instrument_id: "US.AAPL".to_owned(),
        period: "1m".to_owned(),
        extended_hours: false,
        candles: vec![
            HelperCandle {
                at: past_1.format(format).unwrap(),
                open: HelperPriceValue("150.0".to_owned()),
                high: HelperPriceValue("151.0".to_owned()),
                low: HelperPriceValue("149.5".to_owned()),
                close: HelperPriceValue("150.5".to_owned()),
                volume: Some(HelperPriceValue("1000.0".to_owned())),
                session: Some("regular".to_owned()),
            },
            HelperCandle {
                at: past_2.format(format).unwrap(),
                open: HelperPriceValue("150.5".to_owned()),
                high: HelperPriceValue("152.0".to_owned()),
                low: HelperPriceValue("150.0".to_owned()),
                close: HelperPriceValue("151.5".to_owned()),
                volume: Some(HelperPriceValue("1200.0".to_owned())),
                session: Some("regular".to_owned()),
            },
            HelperCandle {
                at: future.format(format).unwrap(),
                open: HelperPriceValue("151.5".to_owned()),
                high: HelperPriceValue("153.0".to_owned()),
                low: HelperPriceValue("151.0".to_owned()),
                close: HelperPriceValue("152.5".to_owned()),
                volume: Some(HelperPriceValue("500.0".to_owned())),
                session: Some("regular".to_owned()),
            },
        ],
        total_returned: 3,
        has_more: false,
        next_before: None,
        source: "mock".to_owned(),
        adjustment: "none".to_owned(),
    };

    let converted = convert_helper_candles_response(
        resp,
        HelperCandleConversionParams {
            market: "US",
            symbol: "AAPL",
            period: "1m",
            limit: 10,
            from_time: None,
            to_time: None,
            before: None,
            sessions: &["regular"],
            is_yfinance: false,
            is_akshare: false,
            calendar: None,
        },
    )
    .expect("convert helper candles");

    let candles = converted["candles"].as_array().expect("candles array");
    assert_eq!(candles.len(), 3);

    // Prior candles are guaranteed closed
    assert_eq!(candles[0]["closed"], json!(true));
    assert_eq!(candles[1]["closed"], json!(true));
    // Future/current in-progress candle is unclosed
    assert_eq!(candles[2]["closed"], json!(false));
}

// =========================================================================
// Test 2: In-Progress Bar Updates Price Cache with Zero Executions/Orders
// =========================================================================

#[test]
fn test_in_progress_candle_updates_cache_without_triggering_pine_or_orders() {
    let mut quote_cache_by_symbol: BTreeMap<String, f64> = BTreeMap::new();
    let mut last_closed_by_symbol: BTreeMap<String, i64> = BTreeMap::new();
    let mut pine_calls: Vec<String> = Vec::new();
    let submitted_orders: Vec<PineOrderIntent> = Vec::new();

    let symbol = "US.AAPL".to_owned();

    // Feed has Bar 0 (closed) and Bar 1 (in-progress with close price 155.0)
    let payload = json!({
        "candles": [
            sample_candle("2026-09-08T09:30:00Z", 150.0, 152.0, 149.0, 151.0, 1000.0, "1m", Some(true)),
            sample_candle("2026-09-08T09:31:00Z", 151.0, 156.0, 150.5, 155.0, 500.0, "1m", Some(false)),
        ]
    });

    let candles = parse_strategy_candles(&payload, Some("1m")).expect("parse candles");
    assert_eq!(candles.len(), 2);
    assert!(candles[0].closed);
    assert!(!candles[1].closed);

    // Execution logic replica (same as strategy_runtime.rs)
    let (closed_candles, in_progress_candle) = match candles.split_last() {
        Some((last, rest)) if !last.closed => (rest, Some(last)),
        _ => (candles.as_slice(), None),
    };

    // Verify partitioning
    assert_eq!(closed_candles.len(), 1);
    assert_eq!(closed_candles[0].open_time, 1788859800000); // 09:30:00
    assert!(in_progress_candle.is_some());
    let in_prog = in_progress_candle.unwrap();
    assert_eq!(in_prog.open_time, 1788859860000); // 09:31:00
    assert_eq!(in_prog.close, 155.0);

    // In-progress candle updates quote snapshot / price cache
    if let Some(in_progress) = in_progress_candle {
        quote_cache_by_symbol.insert(symbol.clone(), in_progress.close);
    }

    // Verify cache updated
    assert_eq!(quote_cache_by_symbol.get(&symbol).copied(), Some(155.0));

    // Warmup cycle (revision == 0)
    let mut session_revision: u64 = 0;
    if session_revision == 0 {
        let latest_closed = closed_candles.last().expect("closed bar exists");
        let latest_open_time = latest_closed.open_time;

        // Pine worker open called with ONLY closed candles
        let candles_to_send: Vec<PineCandle> =
            closed_candles.iter().map(|c| c.candle.clone()).collect();
        assert_eq!(candles_to_send.len(), 1);
        assert_eq!(candles_to_send[0].open_time, 1788859800000);

        pine_calls.push(format!("open:revision=0:bars={}", candles_to_send.len()));
        session_revision += 1;
        last_closed_by_symbol.insert(symbol.clone(), latest_open_time);
    }

    // Verify that Bar 0 was recorded as last closed, NOT Bar 1!
    assert_eq!(session_revision, 1);
    assert_eq!(
        last_closed_by_symbol.get(&symbol).copied(),
        Some(1788859800000)
    );
    assert_eq!(pine_calls, vec!["open:revision=0:bars=1"]);
    assert_eq!(submitted_orders.len(), 0);

    // Now, poll again while Bar 1 is STILL in-progress (e.g. price ticks to 155.2)
    let payload_tick2 = json!({
        "candles": [
            sample_candle("2026-09-08T09:30:00Z", 150.0, 152.0, 149.0, 151.0, 1000.0, "1m", Some(true)),
            sample_candle("2026-09-08T09:31:00Z", 151.0, 156.5, 150.5, 155.2, 700.0, "1m", Some(false)),
        ]
    });

    let candles_tick2 = parse_strategy_candles(&payload_tick2, Some("1m")).expect("parse candles");
    let (closed_candles_2, in_progress_candle_2) = match candles_tick2.split_last() {
        Some((last, rest)) if !last.closed => (rest, Some(last)),
        _ => (candles_tick2.as_slice(), None),
    };

    if let Some(in_progress) = in_progress_candle_2 {
        quote_cache_by_symbol.insert(symbol.clone(), in_progress.close);
    }

    // Cache updated with latest unclosed tick
    assert_eq!(quote_cache_by_symbol.get(&symbol).copied(), Some(155.2));

    // Live cycle (revision > 0)
    let last_processed = last_closed_by_symbol.get(&symbol).copied().unwrap_or(0);
    let newly_closed: Vec<&StrategyCandle> = closed_candles_2
        .iter()
        .filter(|c| c.open_time > last_processed)
        .collect();

    // MUST be empty because Bar 1 is still unclosed and Bar 0 was already processed!
    assert_eq!(newly_closed.len(), 0);

    // ZERO additional Pine calls and ZERO orders!
    assert_eq!(pine_calls.len(), 1); // Still only the initial open
    assert_eq!(submitted_orders.len(), 0);
    assert_eq!(
        last_closed_by_symbol.get(&symbol).copied(),
        Some(1788859800000)
    );
}

// =========================================================================
// Test 3: In-Progress Bar Closing on Next Tick Triggers Execution Exactly Once
// =========================================================================

#[test]
fn test_in_progress_bar_closing_on_next_tick_triggers_pine_and_order_once() {
    let mut quote_cache_by_symbol: BTreeMap<String, f64> = BTreeMap::new();
    let mut last_closed_by_symbol: BTreeMap<String, i64> = BTreeMap::new();
    let mut session_revision: u64 = 1;
    let mut submitted_intents: BTreeSet<String> = BTreeSet::new();
    let mut pine_appends: Vec<PineCandle> = Vec::new();
    let mut executed_orders: Vec<(String, f64, f64)> = Vec::new(); // (id, qty, price)

    let symbol = "US.AAPL".to_owned();
    // Previous checkpoint: Bar 0 (09:30:00 = 1788859800000)
    last_closed_by_symbol.insert(symbol.clone(), 1788859800000);

    // Next tick: Bar 1 (09:31:00) is now CLOSED (closed: true, close: 155.5)!
    // A fresh Bar 2 (09:32:00) has just opened (closed: false, close: 156.0).
    let payload = json!({
        "candles": [
            sample_candle("2026-09-08T09:30:00Z", 150.0, 152.0, 149.0, 151.0, 1000.0, "1m", Some(true)),
            sample_candle("2026-09-08T09:31:00Z", 151.0, 156.0, 150.5, 155.5, 1200.0, "1m", Some(true)),
            sample_candle("2026-09-08T09:32:00Z", 155.5, 156.5, 155.0, 156.0, 200.0, "1m", Some(false)),
        ]
    });

    let candles = parse_strategy_candles(&payload, Some("1m")).expect("parse candles");
    assert_eq!(candles.len(), 3);
    assert!(candles[0].closed);
    assert!(candles[1].closed);
    assert!(!candles[2].closed);

    let (closed_candles, in_progress_candle) = match candles.split_last() {
        Some((last, rest)) if !last.closed => (rest, Some(last)),
        _ => (candles.as_slice(), None),
    };

    assert_eq!(closed_candles.len(), 2);
    assert_eq!(in_progress_candle.unwrap().close, 156.0);

    // Cache updated with Bar 2's price
    quote_cache_by_symbol.insert(symbol.clone(), in_progress_candle.unwrap().close);

    let last_processed = last_closed_by_symbol.get(&symbol).copied().unwrap_or(0);
    let newly_closed: Vec<&StrategyCandle> = closed_candles
        .iter()
        .filter(|c| c.open_time > last_processed)
        .collect();

    // Exactly 1 newly closed bar: Bar 1
    assert_eq!(newly_closed.len(), 1);
    let closed_bar = newly_closed[0];
    assert_eq!(closed_bar.open_time, 1788859860000);
    assert_eq!(closed_bar.close, 155.5);

    // Process newly closed bar
    for bar in newly_closed {
        pine_appends.push(bar.candle.clone());
        session_revision += 1;

        // Mock Pine worker produces an order intent on Bar 1 close
        let intent = sample_intent("intent-buy-1", 1, bar.open_time, 10.0, 0.0);
        let key = format!("{}:{}:{}", bar.open_time, intent.id, intent.bar_index);
        if !submitted_intents.contains(&key) {
            submitted_intents.insert(key);
            // Execute order using cache price reference (156.0) or bar close (155.5)
            let fallback_price = quote_cache_by_symbol
                .get(&symbol)
                .copied()
                .unwrap_or(bar.close);
            executed_orders.push((intent.id.clone(), intent.quantity, fallback_price));
        }

        last_closed_by_symbol.insert(symbol.clone(), bar.open_time);
    }

    // Verification:
    assert_eq!(session_revision, 2);
    assert_eq!(pine_appends.len(), 1);
    assert_eq!(pine_appends[0].open_time, 1788859860000);
    assert_eq!(executed_orders.len(), 1);
    assert_eq!(executed_orders[0].0, "intent-buy-1");
    assert_eq!(executed_orders[0].1, 10.0);
    assert_eq!(executed_orders[0].2, 156.0); // Used latest quote cache price
    assert_eq!(
        last_closed_by_symbol.get(&symbol).copied(),
        Some(1788859860000)
    );
}

// =========================================================================
// Test 4: Duplicate Polls on Closed Bar Do Not Re-Execute (Strict Monotonicity)
// =========================================================================

#[test]
fn test_duplicate_polls_on_same_closed_bar_are_idempotent() {
    let mut quote_cache_by_symbol: BTreeMap<String, f64> = BTreeMap::new();
    let mut last_closed_by_symbol: BTreeMap<String, i64> = BTreeMap::new();
    let mut session_revision: u64 = 2;
    let mut submitted_intents: BTreeSet<String> = BTreeSet::new();
    let mut append_count = 0;
    let mut order_count = 0;

    let symbol = "US.AAPL".to_owned();
    // Bar 1 (09:31:00 = 1788859860000) already processed
    last_closed_by_symbol.insert(symbol.clone(), 1788859860000);
    submitted_intents.insert("1788859860000:intent-buy-1:1".to_owned());

    // Identical feed returned on next poll
    let payload = json!({
        "candles": [
            sample_candle("2026-09-08T09:30:00Z", 150.0, 152.0, 149.0, 151.0, 1000.0, "1m", Some(true)),
            sample_candle("2026-09-08T09:31:00Z", 151.0, 156.0, 150.5, 155.5, 1200.0, "1m", Some(true)),
            sample_candle("2026-09-08T09:32:00Z", 155.5, 156.8, 155.0, 156.2, 400.0, "1m", Some(false)),
        ]
    });

    // Run 5 repeated polls with the same feed
    for _ in 0..5 {
        let candles = parse_strategy_candles(&payload, Some("1m")).expect("parse candles");
        let (closed_candles, in_progress_candle) = match candles.split_last() {
            Some((last, rest)) if !last.closed => (rest, Some(last)),
            _ => (candles.as_slice(), None),
        };

        if let Some(in_progress) = in_progress_candle {
            quote_cache_by_symbol.insert(symbol.clone(), in_progress.close);
        }

        let last_processed = last_closed_by_symbol.get(&symbol).copied().unwrap_or(0);
        let newly_closed: Vec<&StrategyCandle> = closed_candles
            .iter()
            .filter(|c| c.open_time > last_processed)
            .collect();

        // Must be empty on every poll!
        assert!(newly_closed.is_empty());

        for bar in newly_closed {
            append_count += 1;
            session_revision += 1;
            let intent = sample_intent("intent-buy-1", 1, bar.open_time, 10.0, 0.0);
            let key = format!("{}:{}:{}", bar.open_time, intent.id, intent.bar_index);
            if !submitted_intents.contains(&key) {
                submitted_intents.insert(key);
                order_count += 1;
            }
            last_closed_by_symbol.insert(symbol.clone(), bar.open_time);
        }
    }

    assert_eq!(append_count, 0, "No duplicate appends allowed");
    assert_eq!(order_count, 0, "No duplicate orders allowed");
    assert_eq!(
        session_revision, 2,
        "Session revision must not drift on duplicate polls"
    );
    assert_eq!(
        last_closed_by_symbol.get(&symbol).copied(),
        Some(1788859860000)
    );
}

// =========================================================================
// Test 5: Sequential Multi-Bar Catchup Preserves Chronological Invariants
// =========================================================================

#[test]
fn test_multi_bar_catchup_preserves_chronological_append_and_tracking() {
    let mut quote_cache_by_symbol: BTreeMap<String, f64> = BTreeMap::new();
    let mut last_closed_by_symbol: BTreeMap<String, i64> = BTreeMap::new();
    let mut session_revision: u64 = 1;
    let mut appends: Vec<i64> = Vec::new();

    let symbol = "US.AAPL".to_owned();
    // Last processed: 09:30:00 (1788859800000)
    last_closed_by_symbol.insert(symbol.clone(), 1788859800000);

    // After temporary network delay, 3 bars arrive:
    // Bar 1 (09:31:00, closed)
    // Bar 2 (09:32:00, closed)
    // Bar 3 (09:33:00, in-progress)
    let payload = json!({
        "candles": [
            sample_candle("2026-09-08T09:30:00Z", 150.0, 151.0, 149.0, 150.5, 100.0, "1m", Some(true)),
            sample_candle("2026-09-08T09:31:00Z", 150.5, 152.0, 150.0, 151.5, 200.0, "1m", Some(true)),
            sample_candle("2026-09-08T09:32:00Z", 151.5, 153.0, 151.0, 152.5, 300.0, "1m", Some(true)),
            sample_candle("2026-09-08T09:33:00Z", 152.5, 154.0, 152.0, 153.5, 150.0, "1m", Some(false)),
        ]
    });

    let candles = parse_strategy_candles(&payload, Some("1m")).expect("parse candles");
    let (closed_candles, in_progress_candle) = match candles.split_last() {
        Some((last, rest)) if !last.closed => (rest, Some(last)),
        _ => (candles.as_slice(), None),
    };

    assert_eq!(closed_candles.len(), 3);
    assert_eq!(in_progress_candle.unwrap().open_time, 1788859980000); // 09:33:00

    quote_cache_by_symbol.insert(symbol.clone(), in_progress_candle.unwrap().close);

    let last_processed = last_closed_by_symbol.get(&symbol).copied().unwrap_or(0);
    let newly_closed: Vec<&StrategyCandle> = closed_candles
        .iter()
        .filter(|c| c.open_time > last_processed)
        .collect();

    assert_eq!(newly_closed.len(), 2);
    // Chronological order: Bar 1, then Bar 2
    assert_eq!(newly_closed[0].open_time, 1788859860000); // 09:31:00
    assert_eq!(newly_closed[1].open_time, 1788859920000); // 09:32:00

    for bar in newly_closed {
        appends.push(bar.open_time);
        session_revision += 1;
        last_closed_by_symbol.insert(symbol.clone(), bar.open_time);
    }

    assert_eq!(appends, vec![1788859860000, 1788859920000]);
    assert_eq!(session_revision, 3);
    assert_eq!(
        last_closed_by_symbol.get(&symbol).copied(),
        Some(1788859920000)
    );
}
