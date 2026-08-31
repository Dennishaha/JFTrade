use std::fs;
use std::path::{Path, PathBuf};

use jftrade_datamanagement::{DatabaseDescriptor, DatabaseMaintenancePort, DATABASE_STRATEGY};
use jftrade_store_sqlite::{ManagedDatabaseMaintenanceStore, current_version};
use rusqlite::{Connection, MAIN_DB, OpenFlags};
use tempfile::TempDir;

use super::super::{database_descriptors, initialize_production_databases};

const FIXED_TIMESTAMP: &str = "2026-08-31T00:00:00Z";

#[test]
fn backup_restore_upgrade_corruption_and_rollback_are_fail_closed() {
    let source_root = setup_root();
    let source_settings = settings_path(&source_root);
    initialize_production_databases(&source_settings).expect("initialize source databases");
    let source_descriptor = strategy_descriptor(&source_settings);
    shape_strategy_v1(&source_descriptor.path);
    let source_bytes = snapshot_files(Path::new(&source_descriptor.path));

    let maintenance = ManagedDatabaseMaintenanceStore::new(
        vec![source_descriptor.clone()],
        source_root.path().join("database-rebuild.json"),
        "backup-restore-drill",
    );
    let backup = maintenance
        .backup(DATABASE_STRATEGY, FIXED_TIMESTAMP)
        .expect("create verified backup");
    assert_eq!(
        source_bytes,
        snapshot_files(Path::new(&source_descriptor.path)),
        "backup must not mutate the legacy source"
    );

    let upgrade_root = setup_root();
    let upgrade_settings = settings_path(&upgrade_root);
    let upgrade_path = strategy_descriptor(&upgrade_settings).path;
    restore_snapshot(Path::new(&backup.backup_path), Path::new(&upgrade_path));
    initialize_production_databases(&upgrade_settings).expect("upgrade restored database");
    assert_eq!(
        current_version_from_path(Path::new(&upgrade_path)),
        Some(2),
        "restored strategy database must complete the supported v1 to v2 upgrade"
    );
    let upgraded = Connection::open(&upgrade_path).expect("open upgraded database");
    let history_name: String = upgraded
        .query_row(
            "SELECT name FROM strategy_definition_versions WHERE definition_id = 'fixture-definition'",
            [],
            |row| row.get(0),
        )
        .expect("read migrated definition history");
    assert_eq!(history_name, "Fixture strategy");
    assert!(
        PathBuf::from(format!("{upgrade_path}.pre-migration.bak")).is_file(),
        "production migration must retain a verified rollback snapshot"
    );
    drop(upgraded);

    let corrupt_root = setup_root();
    let corrupt_settings = settings_path(&corrupt_root);
    let corrupt_path = strategy_descriptor(&corrupt_settings).path;
    restore_snapshot(Path::new(&backup.backup_path), Path::new(&corrupt_path));
    let mut corrupt_bytes = fs::read(&corrupt_path).expect("read restored corruption fixture");
    corrupt_bytes[..b"SQLite format 3\0".len()].copy_from_slice(b"CORRUPTED-DB!!!!");
    fs::write(&corrupt_path, &corrupt_bytes).expect("persist corruption fixture");
    let corrupt_before = snapshot_files(Path::new(&corrupt_path));
    let corruption_error = initialize_production_databases(&corrupt_settings)
        .expect_err("corrupted database must fail startup");
    assert!(
        corruption_error.contains("database") || corruption_error.contains("SQLite"),
        "corruption error should identify storage failure: {corruption_error}"
    );
    assert_eq!(
        corrupt_before,
        snapshot_files(Path::new(&corrupt_path)),
        "corruption detection must preserve the original file"
    );
    assert!(
        !PathBuf::from(format!("{corrupt_path}.pre-migration.bak")).exists(),
        "corruption must be rejected before a migration backup is claimed"
    );

    let rollback_root = setup_root();
    let rollback_settings = settings_path(&rollback_root);
    let rollback_path = strategy_descriptor(&rollback_settings).path;
    restore_snapshot(Path::new(&backup.backup_path), Path::new(&rollback_path));
    shape_strategy_with_broken_history(&rollback_path);
    let rollback_before = snapshot_files(Path::new(&rollback_path));
    let rollback_error = initialize_production_databases(&rollback_settings)
        .expect_err("unsupported schema shape must roll back");
    assert!(
        rollback_error.contains("migration failed"),
        "migration error should retain its rollback context: {rollback_error}"
    );
    assert_eq!(
        rollback_before,
        snapshot_files(Path::new(&rollback_path)),
        "failed schema upgrade must preserve the original bytes"
    );
    assert!(
        PathBuf::from(format!("{rollback_path}.pre-migration.bak")).is_file(),
        "failed migration must leave the verified rollback artifact"
    );
    assert_eq!(
        current_version_from_path(Path::new(&source_descriptor.path)),
        Some(1),
        "the backup drill must not upgrade or replace the original source"
    );
}

fn setup_root() -> TempDir {
    tempfile::tempdir().expect("temporary root")
}

fn settings_path(root: &TempDir) -> PathBuf {
    let path = root.path().join("settings.json");
    fs::write(&path, b"{}").expect("settings fixture");
    path
}

fn strategy_descriptor(settings_path: &Path) -> DatabaseDescriptor {
    database_descriptors(settings_path, |_| None)
        .0
        .into_iter()
        .find(|descriptor| descriptor.id == DATABASE_STRATEGY)
        .expect("strategy descriptor")
}

fn shape_strategy_v1(path: &str) {
    let connection = Connection::open(path).expect("open strategy fixture");
    connection
        .execute_batch(
            "INSERT INTO strategy_design_definitions
                (id, name, version, description, runtime, source_format, symbol,
                 interval, script, visual_model_json, created_at, updated_at)
             VALUES
                ('fixture-definition', 'Fixture strategy', 'v1', 'fixture', 'pine', 'pine',
                 'HK.00700', '1m', 'close > open', '{\"nodes\":[]}', 't0', 't1');
             DROP TRIGGER trg_strategy_definition_versions_immutable;
             DROP INDEX idx_strategy_definition_versions_saved_at;
             DROP TABLE strategy_definition_versions;
             UPDATE jftrade_schema_meta SET version = 1 WHERE component_id = 'strategy';",
        )
        .expect("shape supported strategy v1 fixture");
}

fn shape_strategy_with_broken_history(path: &str) {
    let connection = Connection::open(path).expect("open rollback fixture");
    connection
        .execute_batch(
            "CREATE TABLE strategy_definition_versions (broken TEXT);
             UPDATE jftrade_schema_meta SET version = 1 WHERE component_id = 'strategy';",
        )
        .expect("shape unsupported strategy history");
}

fn restore_snapshot(backup_path: &Path, target_path: &Path) {
    let mut target = Connection::open(target_path).expect("create restore target");
    target
        .restore(MAIN_DB, backup_path, None::<fn(rusqlite::backup::Progress)>)
        .expect("restore verified snapshot");
}

fn current_version_from_path(path: &Path) -> Option<i64> {
    let connection = Connection::open_with_flags(path, OpenFlags::SQLITE_OPEN_READ_ONLY)
        .expect("open schema version database");
    current_version(&connection, DATABASE_STRATEGY)
}

fn snapshot_files(path: &Path) -> Vec<(PathBuf, Vec<u8>)> {
    [
        path.to_owned(),
        PathBuf::from(format!("{}-wal", path.display())),
        PathBuf::from(format!("{}-shm", path.display())),
    ]
    .into_iter()
    .filter_map(|path| fs::read(&path).ok().map(|bytes| (path, bytes)))
    .collect()
}
