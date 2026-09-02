use std::path::Path;

use jftrade_store_sqlite::{
    BACKTEST_MARKET_DATA_PRODUCTION_PROFILE, BacktestMarketDataStore, BacktestMarketDataStoreError,
    StoredBacktestCandle, initialize_current,
};
use rusqlite::Connection;

const MINUTE_MS: i64 = 60_000;

fn store(path: &Path) -> BacktestMarketDataStore {
    let connection = Connection::open(path).expect("create database");
    initialize_current(&connection, "backtest").expect("initialize schema");
    drop(connection);
    BacktestMarketDataStore::open_existing(path, BACKTEST_MARKET_DATA_PRODUCTION_PROFILE)
        .expect("open store")
}

fn candle(index: i64, base: i64) -> StoredBacktestCandle {
    let start = index * MINUTE_MS;
    let open = base + index;
    StoredBacktestCandle {
        start_time: start,
        end_time: start + MINUTE_MS - 1,
        open: open.to_string(),
        high: (open + 2).to_string(),
        low: (open - 1).to_string(),
        close: (open + 1).to_string(),
        volume: format!("{}.25", index + 1),
    }
}

#[test]
fn regular_and_extended_session_tables_keep_direct_reads_isolated() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("session-isolation.db");
    let store = store(&path);
    let regular = candle(0, 100);
    let extended = candle(0, 200);

    store
        .insert_candles(
            "futu",
            "US.AAPL",
            "1m",
            "forward",
            "regular",
            std::slice::from_ref(&regular),
        )
        .expect("insert regular candle");
    store
        .insert_candles(
            "futu",
            "US.AAPL",
            "1m",
            "forward",
            "extended",
            std::slice::from_ref(&extended),
        )
        .expect("insert extended candle");

    assert_eq!(
        store
            .read_candles("futu", "US.AAPL", "1m", "forward", "regular", 0, MINUTE_MS)
            .expect("read regular candle"),
        vec![regular.clone()]
    );
    assert_eq!(
        store
            .read_candles("futu", "US.AAPL", "1m", "forward", "extended", 0, MINUTE_MS)
            .expect("read extended candle"),
        vec![extended]
    );
    assert_eq!(store.kline_table_count().expect("count scoped tables"), 3);
    let tables = store.kline_tables().expect("list scoped tables");
    assert!(tables.iter().any(|name| name.contains("__r__")));
    assert!(tables.iter().any(|name| name.contains("__x__")));

    let missing_scope = store.read_candles(
        "futu",
        "US.AAPL",
        "1m",
        "forward",
        "regular",
        MINUTE_MS,
        MINUTE_MS * 2,
    );
    assert!(matches!(missing_scope, Ok(rows) if rows.is_empty()));
}

#[test]
fn aggregate_reads_use_only_the_requested_session_scope() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("session-aggregation.db");
    let store = store(&path);
    let regular = (0..5).map(|index| candle(index, 100)).collect::<Vec<_>>();
    let extended = (0..5).map(|index| candle(index, 200)).collect::<Vec<_>>();

    store
        .insert_candles("futu", "US.AAPL", "1m", "forward", "regular", &regular)
        .expect("insert regular minute coverage");
    store
        .insert_candles("futu", "US.AAPL", "1m", "forward", "extended", &extended)
        .expect("insert extended minute coverage");

    let regular_five = store
        .read_candles(
            "futu",
            "US.AAPL",
            "5m",
            "forward",
            "regular",
            0,
            5 * MINUTE_MS,
        )
        .expect("aggregate regular coverage");
    assert_eq!(regular_five.len(), 1);
    assert_eq!(regular_five[0].open, "100");
    assert_eq!(regular_five[0].high, "106");
    assert_eq!(regular_five[0].low, "99");
    assert_eq!(regular_five[0].close, "105");
    assert_eq!(regular_five[0].volume, "16.25");

    let extended_five = store
        .read_candles(
            "futu",
            "US.AAPL",
            "5m",
            "forward",
            "extended",
            0,
            5 * MINUTE_MS,
        )
        .expect("aggregate extended coverage");
    assert_eq!(extended_five.len(), 1);
    assert_eq!(extended_five[0].open, "200");
    assert_eq!(extended_five[0].high, "206");
    assert_eq!(extended_five[0].low, "199");
    assert_eq!(extended_five[0].close, "205");
    assert_eq!(extended_five[0].volume, "16.25");

    let regular_without_coverage = store.read_candles(
        "futu",
        "US.AAPL",
        "5m",
        "forward",
        "regular",
        5 * MINUTE_MS,
        10 * MINUTE_MS,
    );
    assert!(matches!(
        regular_without_coverage,
        Err(BacktestMarketDataStoreError::Coverage(_))
    ));
}

#[test]
fn unknown_session_scope_is_rejected_without_creating_a_table() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("invalid-session.db");
    let store = store(&path);
    let row = candle(0, 100);

    let result = store.insert_candles(
        "futu",
        "US.AAPL",
        "1m",
        "forward",
        "overnight",
        std::slice::from_ref(&row),
    );
    assert!(matches!(
        result,
        Err(BacktestMarketDataStoreError::Validation(message))
            if message.contains("invalid session scope")
    ));
    assert_eq!(store.kline_table_count().expect("count tables"), 1);
}
