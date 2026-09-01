use std::path::Path;

use jftrade_store_sqlite::{
    BACKTEST_MARKET_DATA_PRODUCTION_PROFILE, BacktestMarketDataStore, BacktestMarketDataStoreError,
    StoredBacktestCandle, initialize_current,
};
use rusqlite::Connection;

const START_TIME: i64 = 1_000;
const END_TIME: i64 = 2_000;

fn seed_database(path: &Path) {
    let connection = Connection::open(path).expect("create backtest database");
    initialize_current(&connection, "backtest").expect("initialize backtest schema");
}

fn open_store(path: &Path) -> BacktestMarketDataStore {
    BacktestMarketDataStore::open_existing(path, BACKTEST_MARKET_DATA_PRODUCTION_PROFILE)
        .expect("open backtest market-data store")
}

fn candle() -> StoredBacktestCandle {
    StoredBacktestCandle {
        start_time: START_TIME,
        end_time: END_TIME,
        open: "100".to_owned(),
        high: "110".to_owned(),
        low: "90".to_owned(),
        close: "105".to_owned(),
        volume: "10".to_owned(),
    }
}

fn dynamic_table(store: &BacktestMarketDataStore) -> String {
    store
        .kline_tables()
        .expect("list dynamic tables")
        .into_iter()
        .find(|name| !name.starts_with("local_klines__manifest__"))
        .expect("dynamic table")
}

#[test]
fn opening_database_with_damaged_dynamic_schema_fails_closed_without_replacing_file() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("damaged-before-open.db");
    seed_database(&path);
    let connection = Connection::open(&path).expect("open database for corruption");
    connection
        .execute_batch(
            "CREATE TABLE local_klines__futu__us_damaged__1m__forward__r__00000000 (
                 end_time INTEGER PRIMARY KEY
             )",
        )
        .expect("create damaged dynamic table");
    drop(connection);
    let before = std::fs::read(&path).expect("read database before open");

    let result =
        BacktestMarketDataStore::open_existing(&path, BACKTEST_MARKET_DATA_PRODUCTION_PROFILE);
    assert!(matches!(
        result,
        Err(BacktestMarketDataStoreError::Schema(error)) if error.is_incompatible()
    ));
    assert_eq!(
        std::fs::read(&path).expect("read database after rejected open"),
        before,
        "schema rejection must not replace or rewrite the damaged database"
    );
}

#[test]
fn missing_symbol_table_reports_storage_error_then_recovers_after_insert() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("missing-symbol.db");
    seed_database(&path);
    let store = open_store(&path);

    let missing = store.read_candles("futu", "US.MISSING", "1m", "forward", "regular", 0, 3_000);
    assert!(matches!(
        missing,
        Err(BacktestMarketDataStoreError::Query(_))
    ));

    store
        .insert_candles(
            "futu",
            "US.MISSING",
            "1m",
            "forward",
            "regular",
            &[candle()],
        )
        .expect("insert should create the missing table");
    let recovered = store
        .read_candles("futu", "US.MISSING", "1m", "forward", "regular", 0, 3_000)
        .expect("read after table recovery");
    assert_eq!(recovered, vec![candle()]);
}

#[test]
fn damaged_dynamic_schema_is_rejected_without_overwriting_and_recovers_after_repair() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("damaged-schema.db");
    seed_database(&path);
    let store = open_store(&path);
    store
        .insert_candles(
            "futu",
            "US.DAMAGED",
            "1m",
            "forward",
            "regular",
            &[candle()],
        )
        .expect("seed valid dynamic table");
    let table = dynamic_table(&store);

    let connection = Connection::open(&path).expect("open database for corruption");
    connection
        .execute_batch(&format!(
            "DROP TABLE \"{table}\";
             CREATE TABLE \"{table}\" (end_time INTEGER PRIMARY KEY)"
        ))
        .expect("replace dynamic table with damaged schema");

    let insert = store.insert_candles(
        "futu",
        "US.DAMAGED",
        "1m",
        "forward",
        "regular",
        &[candle()],
    );
    assert!(matches!(
        insert,
        Err(BacktestMarketDataStoreError::Validation(_))
    ));

    let read = store.read_candles("futu", "US.DAMAGED", "1m", "forward", "regular", 0, 3_000);
    assert!(matches!(read, Err(BacktestMarketDataStoreError::Query(_))));
    drop(store);

    connection
        .execute_batch(&format!(
            "DROP TABLE \"{table}\";
             CREATE TABLE \"{table}\" (
                 end_time INTEGER NOT NULL,
                 start_time INTEGER NOT NULL,
                 open TEXT NOT NULL,
                 high TEXT NOT NULL,
                 low TEXT NOT NULL,
                 close TEXT NOT NULL,
                 volume TEXT NOT NULL,
                 PRIMARY KEY (end_time)
             ) WITHOUT ROWID"
        ))
        .expect("repair dynamic table schema");
    drop(connection);

    let reopened = open_store(&path);
    reopened
        .insert_candles(
            "futu",
            "US.DAMAGED",
            "1m",
            "forward",
            "regular",
            &[candle()],
        )
        .expect("insert after schema repair");
    assert_eq!(
        reopened
            .read_candles("futu", "US.DAMAGED", "1m", "forward", "regular", 0, 3_000,)
            .expect("read after schema repair"),
        vec![candle()]
    );
}

#[test]
fn empty_regular_and_extended_tables_return_empty_results() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("empty-scopes.db");
    seed_database(&path);
    let store = open_store(&path);

    for scope in ["regular", "extended"] {
        assert_eq!(
            store
                .insert_candles("futu", "US.EMPTY", "1m", "forward", scope, &[])
                .expect("create empty scoped table"),
            0
        );
        assert!(
            store
                .read_candles("futu", "US.EMPTY", "1m", "forward", scope, 0, 3_000)
                .expect("read empty scoped table")
                .is_empty(),
            "{scope} table should be an explicit empty result"
        );
    }
}

#[test]
fn reopening_after_store_drop_releases_writer_and_preserves_empty_table() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("reopen.db");
    seed_database(&path);
    let store = open_store(&path);
    store
        .insert_candles("futu", "US.REOPEN", "1m", "forward", "regular", &[])
        .expect("create empty table");
    drop(store);

    let reopened = open_store(&path);
    assert!(
        reopened
            .read_candles("futu", "US.REOPEN", "1m", "forward", "regular", 0, 3_000)
            .expect("read after reopen")
            .is_empty()
    );
}
