use std::path::Path;

use jftrade_store_sqlite::{
    BACKTEST_MARKET_DATA_PRODUCTION_PROFILE, BacktestMarketDataStore, BacktestMarketDataStoreError,
    StoredBacktestCandle, initialize_current,
};
use rusqlite::Connection;

const MINUTE_MS: i64 = 60_000;

fn open_test_store(path: &Path) -> BacktestMarketDataStore {
    let connection = Connection::open(path).expect("create database");
    initialize_current(&connection, "backtest").expect("initialize schema");
    drop(connection);
    BacktestMarketDataStore::open_existing(path, BACKTEST_MARKET_DATA_PRODUCTION_PROFILE)
        .expect("open store")
}

fn make_candle(start_time: i64, price: i64, volume: &str) -> StoredBacktestCandle {
    StoredBacktestCandle {
        start_time,
        end_time: start_time + MINUTE_MS - 1,
        open: price.to_string(),
        high: (price + 2).to_string(),
        low: (price - 1).to_string(),
        close: (price + 1).to_string(),
        volume: volume.to_string(),
    }
}

/// TC-D4-01: Regular session 09:30~16:00 continuous 1m data for US.AAPL (390 bars).
/// Verify 5m (78 bars), 15m (26 bars), 30m (13 bars) aggregations are exact.
#[test]
fn test_tc_d4_01_regular_session_intraday_sub_hourly_aggregation() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("tc_d4_01.db");
    let store = open_test_store(&path);

    // 2024-03-08 EST (14:30 UTC to 21:00 UTC) -> 390 minutes
    let open_ms = 1709908200000i64;
    let close_ms = 1709931600000i64;
    let candles: Vec<StoredBacktestCandle> = (0..390)
        .map(|i| make_candle(open_ms + i * MINUTE_MS, 150 + (i % 10), "10.0"))
        .collect();

    store
        .insert_candles("futu", "US.AAPL", "1m", "forward", "regular", &candles)
        .expect("insert 1m regular candles");

    // 5m: 390 / 5 = 78 bars
    let bars_5m = store
        .read_candles(
            "futu", "US.AAPL", "5m", "forward", "regular", open_ms, close_ms,
        )
        .expect("read 5m candles");
    assert_eq!(bars_5m.len(), 78);
    assert_eq!(bars_5m[0].start_time, open_ms);
    assert_eq!(bars_5m[0].end_time, open_ms + 5 * MINUTE_MS - 1);
    assert_eq!(bars_5m.last().unwrap().end_time, close_ms - 1);

    // 15m: 390 / 15 = 26 bars
    let bars_15m = store
        .read_candles(
            "futu", "US.AAPL", "15m", "forward", "regular", open_ms, close_ms,
        )
        .expect("read 15m candles");
    assert_eq!(bars_15m.len(), 26);
    assert_eq!(bars_15m[0].start_time, open_ms);
    assert_eq!(bars_15m.last().unwrap().end_time, close_ms - 1);

    // 30m: 390 / 30 = 13 bars
    let bars_30m = store
        .read_candles(
            "futu", "US.AAPL", "30m", "forward", "regular", open_ms, close_ms,
        )
        .expect("read 30m candles");
    assert_eq!(bars_30m.len(), 13);
    assert_eq!(bars_30m[0].start_time, open_ms);
    assert_eq!(bars_30m.last().unwrap().end_time, close_ms - 1);
}

/// TC-D4-02 & TC-D4-03: DST boundary injection & 60m session-anchored aggregation.
/// 2024-03-08 (EST, open=14:30 UTC=1709908200000) & 2024-03-11 (EDT, open=13:30 UTC=1710163800000).
/// Verifies that 60m aggregation does NOT crash with missing coverage,
/// and first bucket start time exactly equals 14:30 UTC (EST) and 13:30 UTC (EDT).
#[test]
fn test_tc_d4_02_and_03_dst_boundary_and_60m_session_anchored_aggregation() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("tc_d4_02_03.db");
    let store = open_test_store(&path);

    // Day 1: 2024-03-08 EST (UTC-5)
    // 09:30 EST = 14:30 UTC = 1709908200000 ms
    // 16:00 EST = 21:00 UTC = 1709931600000 ms
    let day1_open = 1709908200000i64;
    let day1_close = 1709931600000i64;
    let day1_candles: Vec<StoredBacktestCandle> = (0..390)
        .map(|i| make_candle(day1_open + i * MINUTE_MS, 150 + i, "10.0"))
        .collect();

    store
        .insert_candles("futu", "US.AAPL", "1m", "forward", "regular", &day1_candles)
        .expect("insert day1 regular candles");

    // Day 2: 2024-03-11 EDT (UTC-4) after DST switch on 2024-03-10
    // 09:30 EDT = 13:30 UTC = 1710163800000 ms
    // 16:00 EDT = 20:00 UTC = 1710187200000 ms
    let day2_open = 1710163800000i64;
    let day2_close = 1710187200000i64;
    let day2_candles: Vec<StoredBacktestCandle> = (0..390)
        .map(|i| make_candle(day2_open + i * MINUTE_MS, 160 + i, "10.0"))
        .collect();

    store
        .insert_candles("futu", "US.AAPL", "1m", "forward", "regular", &day2_candles)
        .expect("insert day2 regular candles");

    // Query 60m for Day 1 (EST)
    let day1_60m = store
        .read_candles(
            "futu", "US.AAPL", "60m", "forward", "regular", day1_open, day1_close,
        )
        .expect("read 60m candles for EST day");
    // 390 min = 6 x 60m full bars + 1 x 30m session-tail truncated bar = 7 bars
    assert_eq!(
        day1_60m.len(),
        7,
        "EST trading day 390m must yield 7 session bars"
    );
    assert_eq!(
        day1_60m[0].start_time, 1709908200000,
        "EST open bucket start must be 14:30 UTC (1709908200000)"
    );
    assert_eq!(day1_60m[0].end_time, 1709908200000 + 60 * MINUTE_MS - 1);
    assert_eq!(day1_60m[0].open, "150");
    // Check tail truncated bar (15:30 to 16:00 EST)
    let day1_tail = day1_60m.last().unwrap();
    assert_eq!(day1_tail.start_time, day1_open + 360 * MINUTE_MS);
    assert_eq!(day1_tail.end_time, day1_close - 1);

    // Query 60m for Day 2 (EDT)
    let day2_60m = store
        .read_candles(
            "futu", "US.AAPL", "60m", "forward", "regular", day2_open, day2_close,
        )
        .expect("read 60m candles for EDT day");
    assert_eq!(
        day2_60m.len(),
        7,
        "EDT trading day 390m must yield 7 session bars"
    );
    assert_eq!(
        day2_60m[0].start_time, 1710163800000,
        "EDT open bucket start must be 13:30 UTC (1710163800000)"
    );
    assert_eq!(day2_60m[0].end_time, 1710163800000 + 60 * MINUTE_MS - 1);
    assert_eq!(day2_60m[0].open, "160");
}

/// TC-D4-04: Extended session isolation.
/// Data written from 04:00 to 10:00 ET.
/// Pre-market data (04:00~09:30) must NOT contaminate the 09:30 official open price in 60m output.
#[test]
fn test_tc_d4_04_extended_session_pre_market_does_not_pollute_regular_open() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("tc_d4_04.db");
    let store = open_test_store(&path);

    // 2024-03-11 EDT:
    // Pre-market: 04:00 EDT = 08:00 UTC = 1710144000000 ms
    // Regular open: 09:30 EDT = 13:30 UTC = 1710163800000 ms
    // Regular 10:00: 10:00 EDT = 14:00 UTC = 1710165600000 ms
    let pre_open = 1710144000000i64; // 04:00 EDT
    let reg_open = 1710163800000i64; // 09:30 EDT
    let test_end = 1710165600000i64; // 10:00 EDT

    // Total minutes = (09:30 - 04:00) = 330m pre + 30m reg = 360 minutes
    let mut candles = Vec::with_capacity(360);
    // Pre-market: price = 100
    for i in 0..330 {
        candles.push(make_candle(pre_open + i * MINUTE_MS, 100 + i, "5.0"));
    }
    // Regular session: official open at 09:30 = price 500!
    for i in 0..30 {
        candles.push(make_candle(reg_open + i * MINUTE_MS, 500 + i, "50.0"));
    }

    store
        .insert_candles("futu", "US.AAPL", "1m", "forward", "extended", &candles)
        .expect("insert extended candles");

    let bars_60m = store
        .read_candles(
            "futu", "US.AAPL", "60m", "forward", "extended", pre_open, test_end,
        )
        .expect("read 60m extended candles");

    // Pre-market buckets:
    // 04:00~05:00, 05:00~06:00, 06:00~07:00, 07:00~08:00, 08:00~09:00 (5 full 60m bars)
    // 09:00~09:30 (1 truncated 30m pre-market bar)
    // Regular market bucket:
    // 09:30~10:00 (1 truncated 30m regular bar or part of 09:30~10:30)
    assert_eq!(bars_60m.len(), 7);

    // Verify pre-market 09:00~09:30 bar
    let pre_last = &bars_60m[5];
    assert_eq!(pre_last.start_time, pre_open + 300 * MINUTE_MS); // 09:00 EDT
    assert_eq!(pre_last.end_time, reg_open - 1); // 09:29:59 EDT

    // Verify regular 09:30 bar has official regular open price (500), NOT pre-market price (100 or 400)!
    let reg_first = &bars_60m[6];
    assert_eq!(
        reg_first.start_time, reg_open,
        "Regular session bar must start strictly at 09:30 EDT"
    );
    assert_eq!(
        reg_first.open, "500",
        "Official 09:30 open price must not be polluted by pre-market prices"
    );
}

/// Safety Red Line: Missing minute within a bucket MUST fail closed with Coverage error.
#[test]
fn test_safety_red_line_missing_minute_fails_closed_with_coverage_error() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("red_line.db");
    let store = open_test_store(&path);

    // 2024-03-08 EST (14:30 UTC to 21:00 UTC) -> 390 minutes
    let open_ms = 1709908200000i64;
    let close_ms = 1709931600000i64;

    // Drop minute index 15 (09:45 EST)
    let candles: Vec<StoredBacktestCandle> = (0..390)
        .filter(|&i| i != 15)
        .map(|i| make_candle(open_ms + i * MINUTE_MS, 150 + i, "10.0"))
        .collect();

    store
        .insert_candles("futu", "US.AAPL", "1m", "forward", "regular", &candles)
        .expect("insert 1m regular candles with missing minute");

    let result = store.read_candles(
        "futu", "US.AAPL", "60m", "forward", "regular", open_ms, close_ms,
    );
    assert!(
        matches!(result, Err(BacktestMarketDataStoreError::Coverage(_))),
        "Missing minute must fail closed with Coverage error, got: {result:?}"
    );

    // Also verify missing minute in the tail truncated bucket (e.g. index 380: 15:50 EST)
    let dir2 = tempfile::tempdir().expect("tempdir");
    let path2 = dir2.path().join("red_line_tail.db");
    let store2 = open_test_store(&path2);

    let candles_tail_missing: Vec<StoredBacktestCandle> = (0..390)
        .filter(|&i| i != 380)
        .map(|i| make_candle(open_ms + i * MINUTE_MS, 150 + i, "10.0"))
        .collect();

    store2
        .insert_candles(
            "futu",
            "US.AAPL",
            "1m",
            "forward",
            "regular",
            &candles_tail_missing,
        )
        .expect("insert 1m regular candles with missing tail minute");

    let tail_result = store2.read_candles(
        "futu", "US.AAPL", "60m", "forward", "regular", open_ms, close_ms,
    );
    assert!(
        matches!(tail_result, Err(BacktestMarketDataStoreError::Coverage(_))),
        "Missing minute in tail bucket must fail closed with Coverage error, got: {tail_result:?}"
    );
}
