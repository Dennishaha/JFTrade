use std::path::Path;

use jftrade_owner_lock::WriterLeaseError;
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

fn candle(index: i64) -> StoredBacktestCandle {
    let start = index * MINUTE_MS;
    let open = 100 + index;
    StoredBacktestCandle {
        start_time: start,
        end_time: start + MINUTE_MS - 1,
        open: open.to_string(),
        high: (open + 2).to_string(),
        low: (open - 1).to_string(),
        close: (open + 1).to_string(),
        volume: format!("{}.250000000", index + 1),
    }
}

fn seed_minutes(store: &BacktestMarketDataStore, count: i64) {
    let rows = (0..count).map(candle).collect::<Vec<_>>();
    store
        .insert_candles("futu", "US.AAPL", "1m", "forward", "regular", &rows)
        .expect("insert one-minute rows");
}

#[test]
fn aggregates_complete_one_minute_coverage_into_canonical_five_and_fifteen_minute_bars() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("aggregation.db");
    let store = store(&path);
    seed_minutes(&store, 15);

    let five = store
        .read_candles("futu", "US.AAPL", "5m", "forward", "regular", 0, 900_000)
        .expect("aggregate five-minute rows");
    assert_eq!(five.len(), 3);
    assert_eq!(five[0].start_time, 0);
    assert_eq!(five[0].end_time, 299_999);
    assert_eq!(five[0].open, "100");
    assert_eq!(five[0].high, "106");
    assert_eq!(five[0].low, "99");
    assert_eq!(five[0].close, "105");
    assert_eq!(five[0].volume, "16.25");

    let fifteen = store
        .read_candles("futu", "US.AAPL", "15m", "forward", "regular", 0, 900_000)
        .expect("aggregate fifteen-minute rows");
    assert_eq!(fifteen.len(), 1);
    assert_eq!(fifteen[0].start_time, 0);
    assert_eq!(fifteen[0].end_time, 899_999);
    assert_eq!(fifteen[0].open, "100");
    assert_eq!(fifteen[0].high, "116");
    assert_eq!(fifteen[0].low, "99");
    assert_eq!(fifteen[0].close, "115");
    assert_eq!(fifteen[0].volume, "123.75");
}

#[test]
fn direct_interval_rows_win_and_paging_is_deterministic() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("direct-priority.db");
    let store = store(&path);
    seed_minutes(&store, 10);
    let direct = StoredBacktestCandle {
        start_time: 0,
        end_time: 299_999,
        open: "900".to_owned(),
        high: "901".to_owned(),
        low: "899".to_owned(),
        close: "900.5".to_owned(),
        volume: "1".to_owned(),
    };
    store
        .insert_candles(
            "futu",
            "US.AAPL",
            "5m",
            "forward",
            "regular",
            std::slice::from_ref(&direct),
        )
        .expect("insert direct five-minute row");

    assert_eq!(
        store
            .read_candles("futu", "US.AAPL", "5m", "forward", "regular", 0, 300_000)
            .expect("read direct row"),
        vec![direct]
    );
    let forward = store
        .query_candles_forward("futu", "US.AAPL", "5m", "forward", "regular", 0, 2)
        .expect("forward page");
    assert_eq!(forward.len(), 1);
    assert_eq!(forward[0].open, "900");
    let backward = store
        .query_candles_backward("futu", "US.AAPL", "5m", "forward", "regular", 300_000, 1)
        .expect("backward page");
    assert_eq!(backward.len(), 1);
    assert_eq!(backward[0].start_time, 0);
}

#[test]
fn interval_aliases_share_a_canonical_storage_table() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("interval-alias.db");
    let store = store(&path);
    let row = candle(0);
    let mut expected = row.clone();
    expected.volume = "1.25".to_owned();
    store
        .insert_candles(
            "futu",
            "US.AAPL",
            "1min",
            "forward",
            "regular",
            std::slice::from_ref(&row),
        )
        .expect("insert alias interval");
    assert_eq!(
        store
            .read_candles("futu", "US.AAPL", "1m", "forward", "regular", 0, MINUTE_MS)
            .expect("read canonical interval"),
        vec![expected]
    );
    assert_eq!(store.kline_table_count().expect("count tables"), 2);
}

#[test]
fn production_store_lists_manifest_catalog() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("manifest.db");
    let store = store(&path);
    assert_eq!(
        store.kline_tables().expect("read tables"),
        vec!["local_klines__manifest__symbol__1m__forward__r__00000000"]
    );
    assert_eq!(store.kline_table_count().expect("count tables"), 1);
}

#[test]
fn production_store_rejects_a_second_writer_lease() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("lease.db");
    let connection = Connection::open(&path).expect("create database");
    initialize_current(&connection, "backtest").expect("initialize schema");
    drop(connection);

    let first = BacktestMarketDataStore::open(&path).expect("first store");
    let second = BacktestMarketDataStore::open(&path);
    assert!(matches!(
        second,
        Err(BacktestMarketDataStoreError::WriterLease(
            WriterLeaseError::Held { .. }
        ))
    ));
    drop(first);
}

#[test]
fn missing_empty_partial_and_corrupt_lower_coverage_fail_closed() {
    let cases = ["missing", "empty", "partial"];
    for case in cases {
        let directory = tempfile::tempdir().expect("temporary directory");
        let path = directory.path().join(format!("{case}.db"));
        let store = store(&path);
        match case {
            "empty" => {
                store
                    .insert_candles("futu", "US.AAPL", "1m", "forward", "regular", &[])
                    .expect("create empty source table");
            }
            "partial" => {
                let rows = (0..4).map(candle).collect::<Vec<_>>();
                store
                    .insert_candles("futu", "US.AAPL", "1m", "forward", "regular", &rows)
                    .expect("insert partial source table");
            }
            _ => {}
        }
        let result = store.read_candles("futu", "US.AAPL", "5m", "forward", "regular", 0, 300_000);
        assert!(
            matches!(result, Err(BacktestMarketDataStoreError::Coverage(_))),
            "{case}: {result:?}"
        );
    }

    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("corrupt.db");
    let store = store(&path);
    seed_minutes(&store, 5);
    let connection = Connection::open(&path).expect("open for corruption");
    let table = store
        .kline_tables()
        .expect("list tables")
        .into_iter()
        .find(|name| name.contains("us_aapl") && name.contains("__1m__"))
        .expect("source table");
    connection
        .execute_batch(&format!(
            "DROP TABLE \"{table}\"; CREATE TABLE \"{table}\" (end_time INTEGER PRIMARY KEY)"
        ))
        .expect("damage source schema");
    let result = store.read_candles("futu", "US.AAPL", "5m", "forward", "regular", 0, 300_000);
    assert!(matches!(
        result,
        Err(BacktestMarketDataStoreError::Query(_))
    ));
}
