use std::fs;
use std::path::{Path, PathBuf};

use jftrade_datamanagement::{
    DATABASE_ADK, DATABASE_STRATEGY, DATABASE_WATCHLIST, DatabaseMaintenancePort,
    DatabaseDescriptor, RebuildRequest,
};
use jftrade_store_sqlite::ManagedDatabaseMaintenanceStore;
use rusqlite::Connection;

use super::super::{
    apply_pending_rebuild, database_descriptors, initialize_production_databases,
    initialize_production_databases_inner,
};

const REBUILD_AT: &str = "2026-09-20T00:00:00Z";

fn descriptor<'a>(descriptors: &'a [DatabaseDescriptor], id: &str) -> &'a DatabaseDescriptor {
    descriptors
        .iter()
        .find(|descriptor| descriptor.id == id)
        .expect("managed descriptor")
}

fn schedule_rebuild(
    settings_path: &Path,
    descriptors: &[DatabaseDescriptor],
    marker_path: &Path,
    database_id: &str,
) {
    let maintenance = ManagedDatabaseMaintenanceStore::new(
        descriptors.to_vec(),
        marker_path.to_path_buf(),
        "pending-rebuild-test",
    );
    let result = maintenance
        .rebuild(
            &RebuildRequest {
                database_ids: vec![database_id.to_owned()],
                database_id: String::new(),
                mode: "single".to_owned(),
                confirmation: format!("REBUILD {database_id}"),
            },
            REBUILD_AT,
        )
        .expect("schedule rebuild");
    assert!(result.scheduled && result.restart_required);
    assert!(
        marker_path.is_file(),
        "scheduling a rebuild writes the marker for {}",
        settings_path.display()
    );
}

fn seed_row(path: &str, statement: &str) {
    let connection = Connection::open(path).expect("open managed database");
    connection
        .execute_batch(statement)
        .expect("seed managed database row");
}

fn row_count(path: &str, table: &str) -> i64 {
    let connection = Connection::open(path).expect("open managed database");
    connection
        .query_row(&format!("SELECT COUNT(*) FROM {table}"), [], |row| {
            row.get(0)
        })
        .expect("count managed database rows")
}

fn write_marker(marker_path: &Path, contents: &str) {
    fs::write(marker_path, contents).expect("write rebuild marker");
}

fn snapshot_files(path: &Path) -> Vec<(PathBuf, Vec<u8>)> {
    let mut files = Vec::new();
    for suffix in ["", "-wal", "-shm"] {
        let candidate = PathBuf::from(format!("{}{suffix}", path.display()));
        if let Ok(bytes) = fs::read(&candidate) {
            files.push((candidate, bytes));
        }
    }
    files
}

    // Parity: go:452dea11:internal/app/apiserver/server_test.go:472 TestDependenciesApplyScheduledDatabaseRebuildBeforeStartup
// Parity: go:452dea11:internal/app/apiserver/datamigration/manager_test.go:100 TestManagerApplyPendingDeletesOnlySelectedDatabaseFiles
#[test]
fn startup_applies_a_pending_rebuild_only_to_the_selected_databases() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    fs::write(&settings_path, b"{}\n").expect("settings");
    initialize_production_databases(&settings_path).expect("initialize databases");
    let (descriptors, marker_path) = database_descriptors(&settings_path, |_| None);
    let adk = descriptor(&descriptors, DATABASE_ADK).path.clone();
    let strategy = descriptor(&descriptors, DATABASE_STRATEGY).path.clone();
    seed_row(
        &adk,
        "INSERT INTO adk_agents (id, payload_json, created_at, updated_at) \
         VALUES ('agent-pending', '{}', 't0', 't1');",
    );
    seed_row(
        &strategy,
        "INSERT INTO strategy_design_definitions (id, script, visual_model_json, created_at, updated_at) \
         VALUES ('definition-kept', 'script', '{}', 't0', 't1');",
    );

    schedule_rebuild(&settings_path, &descriptors, &marker_path, DATABASE_ADK);
    initialize_production_databases(&settings_path).expect("apply pending rebuild");

    assert_eq!(
        row_count(&adk, "adk_agents"),
        0,
        "the selected database is recreated empty"
    );
    assert_eq!(
        row_count(&strategy, "strategy_design_definitions"),
        1,
        "databases outside the pending selection keep their rows"
    );
    assert!(
        !marker_path.exists(),
        "a completed rebuild clears its marker"
    );
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/manager_test.go:145 TestManagerApplyPendingRejectsTamperedBackupBeforeDeletingAnySource
#[test]
fn startup_rejects_a_tampered_pending_rebuild_without_deleting_any_source() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    fs::write(&settings_path, b"{}\n").expect("settings");
    initialize_production_databases(&settings_path).expect("initialize databases");
    let (descriptors, marker_path) = database_descriptors(&settings_path, |_| None);
    let adk = descriptor(&descriptors, DATABASE_ADK).path.clone();
    let strategy = descriptor(&descriptors, DATABASE_STRATEGY).path.clone();
    schedule_rebuild(&settings_path, &descriptors, &marker_path, DATABASE_ADK);
    schedule_rebuild(&settings_path, &descriptors, &marker_path, DATABASE_STRATEGY);
    let before = [
        snapshot_files(Path::new(&adk)),
        snapshot_files(Path::new(&strategy)),
    ];

    let marker: serde_json::Value =
        serde_json::from_slice(&fs::read(&marker_path).expect("read rebuild marker"))
            .expect("decode rebuild marker");
    let tampered = marker["backups"][0]["path"]
        .as_str()
        .expect("marker backup path");
    fs::write(tampered, b"tampered snapshot").expect("tamper rebuild backup");

    let error = initialize_production_databases(&settings_path)
        .expect_err("a tampered snapshot must block the pending rebuild");
    assert!(
        error.contains("verify rebuild backup"),
        "unexpected rebuild rejection: {error}"
    );
    assert_eq!(
        [
            snapshot_files(Path::new(&adk)),
            snapshot_files(Path::new(&strategy))
        ],
        before,
        "no source file may change while a rebuild snapshot is unverifiable"
    );
    assert!(
        marker_path.is_file(),
        "the marker survives a rejected rebuild"
    );
}

    // Parity: go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:459 TestStartForRunArgsClosesHandlerWhenDatabaseRebuildFinalizeFails
// Parity: go:452dea11:internal/app/apiserver/datamigration/manager_test.go:190 TestManagerKeepsMarkerWhenDeleteFails
#[test]
fn startup_keeps_the_marker_when_a_selected_database_cannot_be_deleted() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    fs::write(&settings_path, b"{}\n").expect("settings");
    initialize_production_databases(&settings_path).expect("initialize databases");
    let (descriptors, marker_path) = database_descriptors(&settings_path, |_| None);
    let watchlist = descriptor(&descriptors, DATABASE_WATCHLIST).path.clone();
    schedule_rebuild(
        &settings_path,
        &descriptors,
        &marker_path,
        DATABASE_WATCHLIST,
    );
    fs::remove_file(&watchlist).expect("remove scheduled database file");
    fs::create_dir(&watchlist).expect("replace the database with a directory");
    fs::write(
        Path::new(&watchlist).join("keep"),
        b"blocking directory",
    )
    .expect("make the blocking directory non-empty");

    let error = apply_pending_rebuild(&descriptors, &marker_path)
        .expect_err("a failing delete must abort the pending rebuild");
    assert!(
        error.contains("remove watchlist database file"),
        "unexpected delete failure: {error}"
    );
    assert!(
        marker_path.is_file(),
        "the marker survives a failed delete so the rebuild can be retried"
    );
    assert!(
        Path::new(&watchlist).join("keep").is_file(),
        "a failed delete must not damage the blocking path"
    );

    // The full startup path rejects the same layout even earlier: a managed
    // database that stopped being a regular file never reaches the delete.
    let error = initialize_production_databases(&settings_path)
        .expect_err("a non-regular managed database must fail startup");
    assert!(
        error.contains("is not a regular file"),
        "unexpected startup guard: {error}"
    );
    assert!(marker_path.is_file(), "the pending marker is never consumed");
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/manager_boundaries_test.go:74 TestManagerPendingLifecycleHandlesMissingCorruptAndUnknownMarkers
#[test]
fn startup_reports_pending_markers_that_are_corrupt_or_unknown() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    fs::write(&settings_path, b"{}\n").expect("settings");
    initialize_production_databases(&settings_path).expect("initialize databases");
    let (descriptors, marker_path) = database_descriptors(&settings_path, |_| None);
    assert!(
        apply_pending_rebuild(&descriptors, &marker_path)
            .expect("a missing marker is a no-op")
            .is_empty(),
        "startup without a pending marker applies nothing"
    );

    let cases = [
        (
            "{",
            "decode database rebuild marker",
            "a corrupt marker fails startup",
        ),
        (
            r#"{"databaseIds":["unknown"]}"#,
            "unknown database id",
            "a marker naming an unmanaged database fails startup",
        ),
    ];
    for (contents, expected, description) in cases {
        let directory = tempfile::tempdir().expect("temporary directory");
        let settings_path = directory.path().join("settings.json");
        fs::write(&settings_path, b"{}\n").expect("settings");
        initialize_production_databases(&settings_path).expect("initialize databases");
        let (descriptors, marker_path) = database_descriptors(&settings_path, |_| None);
        write_marker(&marker_path, contents);

        let error = initialize_production_databases_inner(&descriptors, &marker_path)
            .expect_err(description);
        assert!(
            error.contains(expected),
            "{description}: unexpected error {error}"
        );
        assert!(
            marker_path.is_file(),
            "{description}: the marker must survive"
        );
    }
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/rebuild_safety_test.go:77 TestApplyPendingRejectsDuplicateAndMissingBackups
#[test]
fn startup_reports_pending_markers_without_a_verified_backup() {
    let cases = [
        (
            r#"{"databaseIds":["adk"]}"#,
            "has no verified backup",
            "a scheduled database without a verified snapshot fails startup",
        ),
        (
            r#"{"databaseIds":["adk"],"backups":[{"databaseId":"adk","path":"x"},{"databaseId":"adk","path":"x"}]}"#,
            "duplicate backup",
            "a duplicated snapshot entry fails startup",
        ),
        (
            r#"{"databaseIds":["adk"],"backups":[{"databaseId":"strategy","path":"x"}]}"#,
            "unscheduled database",
            "a snapshot for an unselected database fails startup",
        ),
    ];
    for (contents, expected, description) in cases {
        let directory = tempfile::tempdir().expect("temporary directory");
        let settings_path = directory.path().join("settings.json");
        fs::write(&settings_path, b"{}\n").expect("settings");
        initialize_production_databases(&settings_path).expect("initialize databases");
        let (descriptors, marker_path) = database_descriptors(&settings_path, |_| None);
        write_marker(&marker_path, contents);

        let error = initialize_production_databases_inner(&descriptors, &marker_path)
            .expect_err(description);
        assert!(
            error.contains(expected),
            "{description}: unexpected error {error}"
        );
        assert!(
            marker_path.is_file(),
            "{description}: the marker must survive"
        );
    }
}

#[test]
fn startup_leaves_an_empty_pending_marker_for_the_operator() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    fs::write(&settings_path, b"{}\n").expect("settings");
    initialize_production_databases(&settings_path).expect("initialize databases");
    let (_, marker_path) = database_descriptors(&settings_path, |_| None);
    write_marker(&marker_path, r#"{"databaseIds":[]}"#);

    initialize_production_databases(&settings_path).expect("startup without a pending database");
    assert!(
        marker_path.is_file(),
        "an empty marker is left untouched, matching the Go owner"
    );
}
