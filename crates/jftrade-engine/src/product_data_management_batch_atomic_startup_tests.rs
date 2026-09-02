use std::fs;
use std::path::{Path, PathBuf};

use jftrade_datamanagement::{
    DatabaseDescriptor, DATABASE_BACKTEST, DATABASE_STRATEGY,
};
use jftrade_owner_lock::{OwnerDiagnostic, WriterLease};
use jftrade_store_sqlite::current_version;
use rusqlite::Connection;

use super::super::{
    database_descriptors, initialize_production_databases, initialize_production_databases_inner,
};

#[test]
fn startup_failure_restores_previously_migrated_descriptor_files() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    fs::write(&settings_path, b"{}\n").expect("settings");
    initialize_production_databases(&settings_path).expect("initialize databases");

    let descriptors = database_descriptors(&settings_path, |_| None).0;
    let backtest = descriptor(&descriptors, DATABASE_BACKTEST);
    let strategy = descriptor(&descriptors, DATABASE_STRATEGY);
    set_version(&backtest, 2);
    shape_strategy_with_broken_history(&strategy.path);

    let backtest_before = snapshot_files(Path::new(&backtest.path));
    let strategy_before = snapshot_files(Path::new(&strategy.path));
    let error = initialize_production_databases_inner(&[backtest.clone(), strategy.clone()])
        .expect_err("later descriptor failure must roll back the batch");

    assert!(error.contains("migration failed"), "migration error: {error}");
    assert_eq!(
        snapshot_files(Path::new(&backtest.path)),
        backtest_before,
        "a descriptor migrated before the failure must be restored byte-for-byte"
    );
    assert_eq!(
        snapshot_files(Path::new(&strategy.path)),
        strategy_before,
        "the failing descriptor must be restored byte-for-byte"
    );
    assert_eq!(current_version_at(&backtest), Some(2));
    assert_eq!(current_version_at(&strategy), Some(1));
    assert!(
        migration_backup_path(&backtest).is_file(),
        "the earlier descriptor migration backup must be retained"
    );
    assert!(
        migration_backup_path(&strategy).is_file(),
        "the failing descriptor migration backup must be retained"
    );
}

#[test]
fn startup_acquires_all_writer_leases_in_stable_order_before_migrating() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    fs::write(&settings_path, b"{}\n").expect("settings");
    initialize_production_databases(&settings_path).expect("initialize databases");

    let descriptors = database_descriptors(&settings_path, |_| None).0;
    let backtest = descriptor(&descriptors, DATABASE_BACKTEST);
    let strategy = descriptor(&descriptors, DATABASE_STRATEGY);
    set_version(&backtest, 2);
    set_version(&strategy, 1);

    let diagnostic = OwnerDiagnostic::current("test", "startup-lease-order");
    let _backtest_lease = WriterLease::acquire(Path::new(&backtest.path), &diagnostic)
        .expect("hold first lease");
    let _strategy_lease = WriterLease::acquire(Path::new(&strategy.path), &diagnostic)
        .expect("hold second lease");

    let error = initialize_production_databases_inner(&[strategy.clone(), backtest.clone()])
        .expect_err("held lease must prevent the whole startup batch");
    assert!(
        error.contains(&backtest.path),
        "the path-sorted first lease should fail first: {error}"
    );
    assert_eq!(current_version_at(&backtest), Some(2));
    assert_eq!(current_version_at(&strategy), Some(1));
    assert!(
        !migration_backup_path(&backtest).exists()
            && !migration_backup_path(&strategy).exists(),
        "no descriptor may migrate before every lease is acquired"
    );
}

fn descriptor(
    descriptors: &[DatabaseDescriptor],
    id: &str,
) -> DatabaseDescriptor {
    descriptors
        .iter()
        .find(|descriptor| descriptor.id == id)
        .cloned()
        .expect("descriptor")
}

fn set_version(descriptor: &DatabaseDescriptor, version: i64) {
    let connection = Connection::open(&descriptor.path).expect("open database");
    connection
        .execute(
            "UPDATE jftrade_schema_meta SET version = ?1 WHERE component_id = ?2",
            (version, &descriptor.id),
        )
        .expect("set legacy metadata version");
}

fn shape_strategy_with_broken_history(path: &str) {
    let connection = Connection::open(path).expect("open strategy database");
    connection
        .execute_batch(
            "DROP TRIGGER trg_strategy_definition_versions_immutable;
             DROP INDEX idx_strategy_definition_versions_saved_at;
             DROP TABLE strategy_definition_versions;
             CREATE TABLE strategy_definition_versions (broken TEXT);
             UPDATE jftrade_schema_meta SET version = 1 WHERE component_id = 'strategy';",
        )
        .expect("shape unsupported strategy schema");
}

fn current_version_at(descriptor: &DatabaseDescriptor) -> Option<i64> {
    let connection = Connection::open(&descriptor.path).expect("open schema metadata");
    current_version(&connection, &descriptor.id)
}

fn migration_backup_path(descriptor: &DatabaseDescriptor) -> PathBuf {
    PathBuf::from(format!("{}.pre-migration.bak", descriptor.path))
}

fn snapshot_files(path: &Path) -> Vec<(PathBuf, Vec<u8>)> {
    ["", "-wal", "-shm", "-journal"]
        .into_iter()
        .map(|suffix| PathBuf::from(format!("{}{suffix}", path.display())))
        .filter_map(|path| fs::read(&path).ok().map(|bytes| (path, bytes)))
        .collect()
}
