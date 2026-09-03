use std::fs;
use std::path::{Path, PathBuf};
use std::process;
use std::time::{SystemTime, UNIX_EPOCH};

use jftrade_store_sqlite::{SnapshotError, inspect_backtest_snapshot};
use rusqlite::Connection;

fn unique_database(name: &str) -> PathBuf {
    let nonce = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .expect("system clock after Unix epoch")
        .as_nanos();
    std::env::temp_dir().join(format!(
        "jftrade-rust-stage2-{name}-{}-{nonce}.db",
        process::id()
    ))
}

fn fixture_sql() -> String {
    let path = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../tests/fixtures/compatibility/storage/backtest-readonly.sql");
    fs::read_to_string(path).expect("read backtest SQLite fixture")
}

fn materialize(path: &Path, sql: &str) {
    let connection = Connection::open(path).expect("create fixture database");
    connection
        .execute_batch(sql)
        .expect("materialize fixture database");
    connection.close().expect("close fixture database");
}

#[test]
fn inspection_is_byte_preserving_and_orders_klines_by_end_time() {
    let path = unique_database("valid");
    materialize(&path, &fixture_sql());
    let before = fs::read(&path).expect("read fixture before inspection");

    let snapshot = inspect_backtest_snapshot(&path).expect("inspect fixture");

    let after = fs::read(&path).expect("read fixture after inspection");
    assert_eq!(after, before, "read-only inspection changed database bytes");
    assert_eq!(snapshot.component_id, "backtest");
    assert_eq!(snapshot.version, 3);
    assert_eq!(snapshot.pragmas.foreign_keys, 1);
    assert_eq!(snapshot.pragmas.query_only, 1);
    assert_eq!(snapshot.pragmas.busy_timeout, 10_000);
    assert_eq!(snapshot.pragmas.journal_mode, "delete");
    assert_eq!(snapshot.tables.len(), 2);
    assert_eq!(snapshot.klines.len(), 3);
    assert!(
        snapshot
            .klines
            .windows(2)
            .all(|rows| rows[0].end_time < rows[1].end_time)
    );
    assert_eq!(snapshot.klines[0].open, "99.875");
    assert_eq!(snapshot.klines[1].high, "100.50000001");
    assert_eq!(snapshot.klines[2].volume, "980.125");

    fs::remove_file(path).expect("remove fixture database");
}

#[test]
fn rejects_missing_files_and_incompatible_metadata_without_creating_files() {
    let missing = unique_database("missing");
    let error = inspect_backtest_snapshot(&missing).expect_err("missing file must fail");
    assert!(matches!(error, SnapshotError::NotRegularFile(_)));
    assert!(!missing.exists());

    let incompatible = unique_database("incompatible");
    let sql = fixture_sql().replace(
        "VALUES ('backtest', 3, '2026-08-19T00:00:00Z')",
        "VALUES ('backtest', 2, '2026-08-19T00:00:00Z')",
    );
    materialize(&incompatible, &sql);
    let before = fs::read(&incompatible).expect("read incompatible fixture");
    let error = inspect_backtest_snapshot(&incompatible).expect_err("version mismatch must fail");
    assert!(matches!(error, SnapshotError::Incompatible(_)));
    assert_eq!(
        fs::read(&incompatible).expect("reread incompatible fixture"),
        before
    );
    fs::remove_file(incompatible).expect("remove incompatible fixture");
}
