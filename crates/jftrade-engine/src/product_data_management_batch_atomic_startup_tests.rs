use std::fs;
use std::path::{Path, PathBuf};

use jftrade_datamanagement::{
    DatabaseDescriptor, DATABASE_ADK, DATABASE_ADK_SESSION, DATABASE_BACKTEST, DATABASE_STRATEGY,
};
use jftrade_store_sqlite::{AdkSessionStore, AdkStore};
use jftrade_owner_lock::{OwnerDiagnostic, WriterLease};
use jftrade_store_sqlite::current_version;
use rusqlite::Connection;

use super::super::{
    database_descriptors, initialize_production_databases, initialize_production_databases_inner,
};

    // Parity: go:452dea11:internal/app/apiserver/stores/handle_test.go:42 TestHandleRollsBackAndStopsAfterOpenFailure
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
    let error = initialize_production_databases_inner(
        &[backtest.clone(), strategy.clone()],
        &directory.path().join("database-rebuild.json"),
    )
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

    // Parity: go:452dea11:internal/app/apiserver/stores/handle_test.go:10 TestHandleClosesStoresInReverseOpenOrder
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

    let error = initialize_production_databases_inner(
        &[strategy.clone(), backtest.clone()],
        &directory.path().join("database-rebuild.json"),
    )
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

/// Parity: go:452dea11:internal/assistant/assembly/runtime_test.go:45
/// TestRuntimeDatabaseProbesUseProvidedLayout
///
/// Go probes the runtime and session databases at the injected
/// `Paths{Database, Session}` layout and fails when the file is missing or its
/// schema is not the pinned baseline. Rust derives the same two databases from
/// the settings directory (or the per-database environment override), so
/// startup must create and validate exactly the provided paths instead of
/// silently falling back to the default root.
#[test]
fn database_probes_use_the_provided_layout() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    fs::write(&settings_path, b"{}\n").expect("settings");
    let provided = directory.path().join("provided-layout");
    fs::create_dir_all(&provided).expect("provided layout directory");
    let adk_override = provided.join("adk.db").to_string_lossy().into_owned();
    let session_override = provided.join("adk-session.db").to_string_lossy().into_owned();

    let descriptors = database_descriptors(&settings_path, |name| match name {
        "JFTRADE_ADK_DB" => Some(adk_override.clone()),
        "JFTRADE_ADK_SESSION_DB" => Some(session_override.clone()),
        _ => None,
    })
    .0;
    initialize_production_databases_inner(
        &descriptors,
        &directory.path().join("database-rebuild.json"),
    )
    .expect("initialize provided layout");

    let adk = descriptor(&descriptors, DATABASE_ADK);
    let session = descriptor(&descriptors, DATABASE_ADK_SESSION);
    assert_eq!(adk.path, adk_override, "runtime database keeps its path");
    assert_eq!(session.path, session_override, "session database keeps its path");
    assert!(
        Path::new(&adk.path).is_file(),
        "runtime database must exist at the provided path"
    );
    assert!(
        Path::new(&session.path).is_file(),
        "session database must exist at the provided path"
    );
    AdkStore::open(&adk.path).expect("probe the provided runtime database");
    AdkSessionStore::open(&session.path).expect("probe the provided session database");
    assert!(
        current_version_at(&adk).is_some(),
        "runtime database carries a pinned schema version after initialization"
    );
}
