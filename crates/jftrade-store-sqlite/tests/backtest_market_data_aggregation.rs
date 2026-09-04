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

#[test]
fn aggregates_fifteen_thirty_and_sixty_minute_bars_directly_from_five_minute_source() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("five-minute-synthesis.db");
    let store = store(&path);

    const FIVE_MIN_MS: i64 = 5 * MINUTE_MS;
    let five_min_rows = (0..12)
        .map(|index| {
            let start = index * FIVE_MIN_MS;
            let open = 100 + index * 5;
            StoredBacktestCandle {
                start_time: start,
                end_time: start + FIVE_MIN_MS - 1,
                open: open.to_string(),
                high: (open + 4).to_string(),
                low: (open - 1).to_string(),
                close: (open + 3).to_string(),
                volume: "10.0".to_string(),
            }
        })
        .collect::<Vec<_>>();

    store
        .insert_candles(
            "futu",
            "US.AAPL",
            "5m",
            "forward",
            "regular",
            &five_min_rows,
        )
        .expect("insert five-minute rows");

    // 15m synthesis: 3 x 5m bars per 15m bar
    let fifteen = store
        .read_candles("futu", "US.AAPL", "15m", "forward", "regular", 0, 3_600_000)
        .expect("aggregate fifteen-minute rows from 5m");
    assert_eq!(fifteen.len(), 4);
    assert_eq!(fifteen[0].start_time, 0);
    assert_eq!(fifteen[0].end_time, 899_999);
    assert_eq!(fifteen[0].open, "100");
    assert_eq!(fifteen[0].volume, "30");

    // 30m synthesis: 6 x 5m bars per 30m bar
    let thirty = store
        .read_candles("futu", "US.AAPL", "30m", "forward", "regular", 0, 3_600_000)
        .expect("aggregate thirty-minute rows from 5m");
    assert_eq!(thirty.len(), 2);
    assert_eq!(thirty[0].start_time, 0);
    assert_eq!(thirty[0].end_time, 1_799_999);
    assert_eq!(thirty[0].open, "100");
    assert_eq!(thirty[0].volume, "60");

    // 60m / 1h synthesis: 12 x 5m bars per 60m bar
    let sixty = store
        .read_candles("futu", "US.AAPL", "1h", "forward", "regular", 0, 3_600_000)
        .expect("aggregate 1h rows from 5m");
    assert_eq!(sixty.len(), 1);
    assert_eq!(sixty[0].start_time, 0);
    assert_eq!(sixty[0].end_time, 3_599_999);
    assert_eq!(sixty[0].open, "100");
    assert_eq!(sixty[0].volume, "120");
}

#[test]
fn closest_period_priority_selects_five_minute_over_one_minute_when_both_exist() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("closest_period.db");
    let store = store(&path);

    // 1m data: 15 bars, open = "100", volume = "1.00000000"
    let one_min_rows = (0..15)
        .map(|i| {
            let start = i * 60_000;
            StoredBacktestCandle {
                start_time: start,
                end_time: start + 59_999,
                open: "100".to_owned(),
                high: "101".to_owned(),
                low: "99".to_owned(),
                close: "100".to_owned(),
                volume: "1.00000000".to_owned(),
            }
        })
        .collect::<Vec<_>>();
    store
        .insert_candles("futu", "US.AAPL", "1m", "forward", "regular", &one_min_rows)
        .expect("insert 1m rows");

    // 5m data: 3 bars covering the same 15 minutes, with distinctive marker open = "200", volume = "50.00000000"
    let five_min_rows = (0..3)
        .map(|i| {
            let start = i * 300_000;
            StoredBacktestCandle {
                start_time: start,
                end_time: start + 299_999,
                open: "200".to_owned(),
                high: "205".to_owned(),
                low: "195".to_owned(),
                close: "200".to_owned(),
                volume: "50.00000000".to_owned(),
            }
        })
        .collect::<Vec<_>>();
    store
        .insert_candles(
            "futu",
            "US.AAPL",
            "5m",
            "forward",
            "regular",
            &five_min_rows,
        )
        .expect("insert 5m rows");

    // When synthesizing 15m, Go's closest-period priority mandates using 5m (closest divisor) rather than 1m!
    let fifteen = store
        .read_candles("futu", "US.AAPL", "15m", "forward", "regular", 0, 900_000)
        .expect("read 15m candles");
    assert_eq!(fifteen.len(), 1);
    assert_eq!(
        fifteen[0].open, "200",
        "15m must be synthesized from 5m (open=200), not 1m (open=100)"
    );
    assert_eq!(
        fifteen[0].volume, "150",
        "15m volume must be synthesized from 5m (3 x 50 = 150), not 1m (15 x 1 = 15)"
    );

    // Also verify query_candles in DESC order with limit preserves 5m priority
    let query_fifteen = store
        .query_candles(
            "futu", "US.AAPL", "15m", "forward", "regular", 0, 900_000, "DESC", 10,
        )
        .expect("query 15m DESC");
    assert_eq!(query_fifteen.len(), 1);
    assert_eq!(query_fifteen[0].open, "200");
    assert_eq!(query_fifteen[0].volume, "150");
}

#[test]
fn closest_period_priority_general_divisor_selection_and_no_error_masking() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("general-closest-period.db");
    let store = store(&path);

    // 1m data: 60 bars (open=10)
    let one_min_rows = (0..60)
        .map(|i| {
            let start = i * 60_000;
            StoredBacktestCandle {
                start_time: start,
                end_time: start + 59_999,
                open: "10".to_owned(),
                high: "11".to_owned(),
                low: "9".to_owned(),
                close: "10".to_owned(),
                volume: "1".to_owned(),
            }
        })
        .collect::<Vec<_>>();
    store
        .insert_candles("futu", "US.AAPL", "1m", "forward", "regular", &one_min_rows)
        .expect("insert 1m rows");

    // 15m data: 4 bars (open=150)
    let fifteen_min_rows = (0..4)
        .map(|i| {
            let start = i * 900_000;
            StoredBacktestCandle {
                start_time: start,
                end_time: start + 899_999,
                open: "150".to_owned(),
                high: "155".to_owned(),
                low: "145".to_owned(),
                close: "150".to_owned(),
                volume: "10".to_owned(),
            }
        })
        .collect::<Vec<_>>();
    store
        .insert_candles(
            "futu",
            "US.AAPL",
            "15m",
            "forward",
            "regular",
            &fifteen_min_rows,
        )
        .expect("insert 15m rows");

    // 30m data: 2 bars (open=300)
    let thirty_min_rows = (0..2)
        .map(|i| {
            let start = i * 1_800_000;
            StoredBacktestCandle {
                start_time: start,
                end_time: start + 1_799_999,
                open: "300".to_owned(),
                high: "305".to_owned(),
                low: "295".to_owned(),
                close: "300".to_owned(),
                volume: "20".to_owned(),
            }
        })
        .collect::<Vec<_>>();
    store
        .insert_candles(
            "futu",
            "US.AAPL",
            "30m",
            "forward",
            "regular",
            &thirty_min_rows,
        )
        .expect("insert 30m rows");

    // Synthesizing 60m must pick 30m (the closest divisor among 30m, 15m, 5m, 1m)
    let sixty = store
        .read_candles("futu", "US.AAPL", "60m", "forward", "regular", 0, 3_600_000)
        .expect("read 60m candles");
    assert_eq!(sixty.len(), 1);
    assert_eq!(
        sixty[0].open, "300",
        "60m must be synthesized from 30m (open=300)"
    );
    assert_eq!(
        sixty[0].volume, "40",
        "volume must be from 30m (2 x 20 = 40)"
    );

    // Now test corruption non-masking: Corrupt a row in 30m table
    let table_30m = store
        .kline_tables()
        .expect("list tables")
        .into_iter()
        .find(|name| name.contains("us_aapl") && name.contains("__30m__"))
        .expect("30m table");
    let conn = Connection::open(&path).expect("open raw conn");
    conn.execute(
        &format!("UPDATE \"{table_30m}\" SET close = 'not_a_valid_number' WHERE start_time = 0"),
        [],
    )
    .expect("corrupt row");
    drop(conn);

    // Reading 60m MUST fail with validation error, NOT silently mask by falling back to 15m or 1m!
    let err = store
        .read_candles("futu", "US.AAPL", "60m", "forward", "regular", 0, 3_600_000)
        .expect_err("reading 60m with corrupted 30m candidate must fail");
    assert!(
        matches!(err, BacktestMarketDataStoreError::Validation(_)),
        "expected Validation error, got: {err:?}"
    );
}
