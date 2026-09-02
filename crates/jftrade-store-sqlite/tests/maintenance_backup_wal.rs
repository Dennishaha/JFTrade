use std::path::PathBuf;

use jftrade_datamanagement::{DatabaseDescriptor, DatabaseMaintenancePort};
use jftrade_store_sqlite::{ManagedDatabaseMaintenanceStore, initialize_current};
use rusqlite::{Connection, MAIN_DB};
use tempfile::tempdir;

#[test]
fn backup_snapshot_includes_uncheckpointed_wal_and_restores() {
    let directory = tempdir().expect("temporary directory");
    let source_path = directory.path().join("backtest-runs.db");
    let source = Connection::open(&source_path).expect("create source database");
    initialize_current(&source, "backtest-runs").expect("initialize source schema");
    source
        .execute_batch(
            "PRAGMA journal_mode = WAL;
             PRAGMA wal_autocheckpoint = 0;
             INSERT INTO backtest_runs
                 (id, status, request_json, result_json, created_at, updated_at)
             VALUES ('wal-row', 'completed', '{}', '{\"equity\":42}', 't0', 't1');",
        )
        .expect("write source row to WAL");
    let wal_path = PathBuf::from(format!("{}-wal", source_path.display()));
    let shm_path = PathBuf::from(format!("{}-shm", source_path.display()));
    assert!(
        wal_path.is_file(),
        "source WAL must contain uncheckpointed data"
    );
    assert!(
        shm_path.is_file(),
        "source SHM must exist while source is open"
    );

    let descriptor = DatabaseDescriptor {
        id: "backtest-runs".to_owned(),
        name: "backtest runs".to_owned(),
        path: source_path.to_string_lossy().into_owned(),
        description: String::new(),
        features: Vec::new(),
        expected_version: 1,
    };
    let maintenance = ManagedDatabaseMaintenanceStore::new(
        vec![descriptor],
        directory.path().join("database-rebuild.json"),
        "test-maintenance",
    );
    let result = maintenance
        .backup("backtest-runs", "2026-08-30T00:00:00Z")
        .expect("create consistent backup");
    drop(source);

    let backup_path = PathBuf::from(&result.backup_path);
    let backup = Connection::open(&backup_path).expect("open backup snapshot");
    let row: (String, String) = backup
        .query_row(
            "SELECT id, result_json FROM backtest_runs WHERE id = 'wal-row'",
            [],
            |row| Ok((row.get(0)?, row.get(1)?)),
        )
        .expect("read row from backup snapshot");
    assert_eq!(row, ("wal-row".to_owned(), "{\"equity\":42}".to_owned()));
    drop(backup);

    let restored_path = directory.path().join("restored.db");
    let mut restored = Connection::open(&restored_path).expect("create restore target");
    restored
        .restore(
            MAIN_DB,
            &backup_path,
            None::<fn(rusqlite::backup::Progress)>,
        )
        .expect("restore backup snapshot");
    let restored_id: String = restored
        .query_row(
            "SELECT id FROM backtest_runs WHERE id = 'wal-row'",
            [],
            |row| row.get(0),
        )
        .expect("read restored row");
    assert_eq!(restored_id, "wal-row");
}
