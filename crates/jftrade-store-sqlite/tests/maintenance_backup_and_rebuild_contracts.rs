//! Backup and rebuild contracts for the managed databases.
//!
//! Parity: `go:452dea11:internal/app/apiserver/datamigration/maintenance_test.go:82`
//! `TestDatabaseBackupCreatesVerifiedPrivateSnapshot`,
//! `.../maintenance_failure_paths_test.go:455`
//! `TestBackupSnapshotDoesNotMutateIncompatibleSource`,
//! `.../maintenance_failure_paths_test.go:341`
//! `TestBackupSnapshotRejectsBlockedDirectoryAndEmptySource`,
//! `.../rebuild_safety_test.go:106` `TestScheduleRebuildLockedRemovesSnapshotsAfterBatchFailure`,
//! `.../rebuild_safety_test.go:150` `TestScheduleRebuildIsIdempotentForAnExistingVerifiedBackup`,
//! `.../rebuild_safety_test.go:177` `TestRebuildSelectionAndLockRollbackBoundaries`,
//! `.../manager_test.go:49` `TestManagerScheduleRebuildSharesMaintenanceLocks` and
//! `.../maintenance_failure_paths_test.go:225` `TestMaintenanceBackupAndRebuildSurfacePersistentStateErrors`.
//!
//! The Rust store owns the same guarantees through the writer lease: a
//! snapshot is private, verified, never mutates its source, is written only
//! for a managed database, and a rebuild keeps exactly one verified snapshot
//! per scheduled database.

use std::fs;
use std::path::{Path, PathBuf};

use jftrade_datamanagement::{
    DATABASE_ADK, DATABASE_RESEARCH, DATABASE_WATCHLIST, DatabaseDescriptor,
    DatabaseMaintenancePort, RebuildRequest,
};
use jftrade_owner_lock::{OwnerDiagnostic, WriterLease};
use jftrade_store_sqlite::{ManagedDatabaseMaintenanceStore, initialize_current};
use rusqlite::{Connection, OpenFlags};
use tempfile::tempdir;

const CREATED_AT: &str = "2026-07-11T15:00:00Z";

fn initialize_database(path: &Path, component: &str) {
    let connection = Connection::open(path).expect("create database");
    initialize_current(&connection, component).expect("initialize schema");
}

fn descriptor(id: &str, path: &Path) -> DatabaseDescriptor {
    DatabaseDescriptor {
        id: id.to_owned(),
        name: id.to_owned(),
        path: path.to_string_lossy().into_owned(),
        description: String::new(),
        features: Vec::new(),
        expected_version: 1,
    }
}

fn maintenance(
    directory: &Path,
    descriptors: Vec<DatabaseDescriptor>,
) -> (ManagedDatabaseMaintenanceStore, PathBuf) {
    let marker_path = directory.join("database-rebuild.json");
    (
        ManagedDatabaseMaintenanceStore::new(
            descriptors,
            marker_path.clone(),
            "maintenance-contract-test",
        ),
        marker_path,
    )
}

fn rebuild_request(database_id: &str) -> RebuildRequest {
    RebuildRequest {
        database_ids: vec![database_id.to_owned()],
        database_id: String::new(),
        mode: "single".to_owned(),
        confirmation: format!("REBUILD {database_id}"),
    }
}

fn backup_files(directory: &Path) -> Vec<PathBuf> {
    let backups = directory.join("backups");
    let mut files = match fs::read_dir(&backups) {
        Ok(entries) => entries
            .map(|entry| entry.expect("backup entry").path())
            .collect::<Vec<_>>(),
        Err(_) => Vec::new(),
    };
    files.sort();
    files
}

fn marker_backups(marker_path: &Path) -> Vec<serde_json::Value> {
    let marker: serde_json::Value =
        serde_json::from_slice(&fs::read(marker_path).expect("read rebuild marker"))
            .expect("decode rebuild marker");
    marker["backups"]
        .as_array()
        .expect("marker backups")
        .clone()
}

#[test]
fn backup_snapshot_is_private_verified_and_limited_to_managed_databases() {
    let directory = tempdir().expect("temporary directory");
    let database_path = directory.path().join("watchlists.db");
    initialize_database(&database_path, DATABASE_WATCHLIST);
    Connection::open(&database_path)
        .expect("open watchlist database")
        .execute_batch(
            "CREATE TABLE backup_payload (value TEXT);
             INSERT INTO backup_payload(value) VALUES ('watchlist');",
        )
        .expect("seed backup payload");
    let (maintenance, _) = maintenance(
        directory.path(),
        vec![descriptor(DATABASE_WATCHLIST, &database_path)],
    );

    let result = maintenance
        .backup(DATABASE_WATCHLIST, CREATED_AT)
        .expect("create verified snapshot");
    assert_eq!(result.database_id, DATABASE_WATCHLIST);
    assert!(result.size_bytes > 0, "snapshot size is measured");
    assert_eq!(result.created_at, CREATED_AT);
    let backup_path = PathBuf::from(&result.backup_path);
    assert_eq!(
        backup_path.parent(),
        Some(directory.path().join("backups").as_path()),
        "snapshots live in the managed backup directory"
    );
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let mode = fs::metadata(&backup_path)
            .expect("snapshot metadata")
            .permissions()
            .mode()
            & 0o777;
        assert_eq!(mode, 0o600, "snapshots are private");
    }

    let snapshot = Connection::open_with_flags(
        &backup_path,
        OpenFlags::SQLITE_OPEN_READ_ONLY | OpenFlags::SQLITE_OPEN_NO_MUTEX,
    )
    .expect("open snapshot");
    let payload: String = snapshot
        .query_row("SELECT value FROM backup_payload", [], |row| row.get(0))
        .expect("read snapshot payload");
    assert_eq!(payload, "watchlist");
    drop(snapshot);

    let unknown = maintenance
        .backup("unknown", CREATED_AT)
        .expect_err("unmanaged databases have no snapshot");
    assert!(
        unknown.to_string().contains("unknown database id"),
        "unexpected error: {unknown}"
    );
    assert_eq!(backup_files(directory.path()).len(), 1);
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:455 TestBackupSnapshotDoesNotMutateIncompatibleSource
#[test]
fn backup_snapshot_leaves_an_incompatible_source_byte_identical() {
    let directory = tempdir().expect("temporary directory");
    let database_path = directory.path().join("legacy.db");
    Connection::open(&database_path)
        .expect("create legacy database")
        .execute_batch(
            "CREATE TABLE legacy (id INTEGER PRIMARY KEY, value TEXT);
             INSERT INTO legacy(value) VALUES ('preserve me');",
        )
        .expect("seed legacy database");
    let before = fs::read(&database_path).expect("read legacy database");
    let wal = PathBuf::from(format!("{}-wal", database_path.display()));
    let shm = PathBuf::from(format!("{}-shm", database_path.display()));
    assert!(!wal.exists() && !shm.exists());
    let (maintenance, _) = maintenance(
        directory.path(),
        vec![descriptor(DATABASE_WATCHLIST, &database_path)],
    );

    let result = maintenance
        .backup(DATABASE_WATCHLIST, CREATED_AT)
        .expect("snapshot an incompatible source");
    assert!(PathBuf::from(&result.backup_path).is_file());
    assert_eq!(
        fs::read(&database_path).expect("read source after snapshot"),
        before,
        "a snapshot must not mutate its source"
    );
    assert!(
        !wal.exists() && !shm.exists(),
        "a snapshot must not create source sidecars"
    );
}

#[test]
fn backup_and_compact_fail_closed_when_the_source_database_is_missing() {
    let directory = tempdir().expect("temporary directory");
    let database_path = directory.path().join("strategy-runtime.db");
    let (maintenance, marker_path) = maintenance(
        directory.path(),
        vec![descriptor("strategy", &database_path)],
    );

    let backup_error = maintenance
        .backup("strategy", CREATED_AT)
        .expect_err("a missing source has no snapshot");
    assert!(
        backup_error
            .to_string()
            .contains("unable to open database file"),
        "unexpected backup error: {backup_error}"
    );
    assert!(maintenance.compact("strategy", CREATED_AT).is_err());
    assert!(
        backup_files(directory.path()).is_empty(),
        "a failed backup leaves no partial snapshot"
    );
    assert!(!marker_path.exists(), "a failed backup writes no marker");
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/rebuild_safety_test.go:150 TestScheduleRebuildIsIdempotentForAnExistingVerifiedBackup
#[test]
fn rebuild_scheduling_is_idempotent_and_reverifies_marker_snapshots() {
    let directory = tempdir().expect("temporary directory");
    let database_path = directory.path().join("watchlists.db");
    initialize_database(&database_path, DATABASE_WATCHLIST);
    let (maintenance, marker_path) = maintenance(
        directory.path(),
        vec![descriptor(DATABASE_WATCHLIST, &database_path)],
    );

    for attempt in 1..=2 {
        let result = maintenance
            .rebuild(&rebuild_request(DATABASE_WATCHLIST), CREATED_AT)
            .unwrap_or_else(|error| panic!("schedule attempt {attempt}: {error}"));
        assert_eq!(result.database_ids, vec![DATABASE_WATCHLIST.to_owned()]);
        assert!(result.restart_required && result.scheduled);
    }
    let backups = marker_backups(&marker_path);
    assert_eq!(
        backups.len(),
        1,
        "scheduling the same rebuild twice keeps one verified snapshot"
    );

    let backup_path = PathBuf::from(backups[0]["path"].as_str().expect("snapshot path"));
    fs::write(&backup_path, b"tampered snapshot").expect("tamper marker snapshot");
    let error = maintenance
        .rebuild(&rebuild_request(DATABASE_WATCHLIST), CREATED_AT)
        .expect_err("a tampered snapshot blocks further scheduling");
    assert!(
        error
            .to_string()
            .contains("rebuild backup size or digest does not match marker"),
        "unexpected tampered-snapshot error: {error}"
    );
    let source = Connection::open(&database_path).expect("reopen source database");
    let quick_check: String = source
        .query_row("PRAGMA quick_check", [], |row| row.get(0))
        .expect("quick check source database");
    assert_eq!(quick_check, "ok");
    drop(source);
    assert!(
        marker_path.is_file(),
        "the marker survives a rejected schedule"
    );
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/rebuild_safety_test.go:106 TestScheduleRebuildLockedRemovesSnapshotsAfterBatchFailure
#[test]
fn a_failed_rebuild_batch_removes_every_snapshot_it_created() {
    let directory = tempdir().expect("temporary directory");
    let adk_path = directory.path().join("adk.db");
    let research_path = directory.path().join("research.db");
    initialize_database(&adk_path, DATABASE_ADK);
    initialize_database(&research_path, DATABASE_RESEARCH);
    let (maintenance, marker_path) = maintenance(
        directory.path(),
        vec![
            descriptor(DATABASE_ADK, &adk_path),
            descriptor(DATABASE_RESEARCH, &research_path),
        ],
    );
    maintenance
        .rebuild(&rebuild_request(DATABASE_ADK), CREATED_AT)
        .expect("schedule the first rebuild");
    let scheduled = backup_files(directory.path());
    assert_eq!(scheduled.len(), 1);

    let mut read_only = fs::metadata(&research_path)
        .expect("research metadata")
        .permissions();
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        read_only.set_mode(0o400);
    }
    fs::set_permissions(&research_path, read_only).expect("make the research database read-only");
    let denied = Connection::open_with_flags(
        &research_path,
        OpenFlags::SQLITE_OPEN_READ_WRITE | OpenFlags::SQLITE_OPEN_NO_MUTEX,
    )
    .is_err();
    if !denied {
        // A privileged runner can still open a read-only file for writing, so
        // the batch failure cannot be provoked on this host.
        return;
    }

    let error = maintenance
        .rebuild(&rebuild_request(DATABASE_RESEARCH), CREATED_AT)
        .expect_err("the batch fails when the second snapshot cannot be written");
    assert!(
        error.to_string().contains("unable to open database file"),
        "unexpected batch failure: {error}"
    );
    assert_eq!(
        backup_files(directory.path()),
        scheduled,
        "a failed batch removes the snapshots it created"
    );
    let backups = marker_backups(&marker_path);
    assert_eq!(backups.len(), 1, "only the successful schedule is recorded");
    assert_eq!(backups[0]["databaseId"], DATABASE_ADK);
    assert!(adk_path.is_file() && research_path.is_file());
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/manager_test.go:49 TestManagerScheduleRebuildSharesMaintenanceLocks
#[test]
fn a_held_writer_lease_rejects_maintenance_before_any_snapshot_is_written() {
    let directory = tempdir().expect("temporary directory");
    let database_path = directory.path().join("watchlists.db");
    initialize_database(&database_path, DATABASE_WATCHLIST);
    let (maintenance, marker_path) = maintenance(
        directory.path(),
        vec![descriptor(DATABASE_WATCHLIST, &database_path)],
    );
    let lease = WriterLease::acquire(
        &database_path,
        &OwnerDiagnostic::current("test", "held-lease"),
    )
    .expect("hold the writer lease");

    let backup_error = maintenance
        .backup(DATABASE_WATCHLIST, CREATED_AT)
        .expect_err("a held lease blocks snapshots");
    assert!(
        backup_error.to_string().contains("writer lease"),
        "unexpected lease error: {backup_error}"
    );
    assert!(
        maintenance
            .rebuild(&rebuild_request(DATABASE_WATCHLIST), CREATED_AT)
            .is_err()
    );
    drop(lease);

    assert!(
        backup_files(directory.path()).is_empty(),
        "a rejected schedule writes no snapshot"
    );
    assert!(
        !marker_path.exists(),
        "a rejected schedule must not write a rebuild marker"
    );
    assert!(database_path.is_file(), "the source database is untouched");
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:225 TestMaintenanceBackupAndRebuildSurfacePersistentStateErrors
#[test]
fn a_corrupt_rebuild_marker_blocks_backup_and_rebuild_without_deleting_data() {
    let directory = tempdir().expect("temporary directory");
    let database_path = directory.path().join("watchlists.db");
    initialize_database(&database_path, DATABASE_WATCHLIST);
    let before = fs::read(&database_path).expect("read source database");
    let (maintenance, marker_path) = maintenance(
        directory.path(),
        vec![descriptor(DATABASE_WATCHLIST, &database_path)],
    );
    fs::write(&marker_path, b"{").expect("write corrupt marker");

    for error in [
        maintenance
            .backup(DATABASE_WATCHLIST, CREATED_AT)
            .expect_err("backup fails closed on a corrupt marker"),
        maintenance
            .rebuild(&rebuild_request(DATABASE_WATCHLIST), CREATED_AT)
            .expect_err("rebuild fails closed on a corrupt marker"),
    ] {
        assert!(
            error.to_string().contains("decode database rebuild marker"),
            "unexpected corrupt-marker error: {error}"
        );
    }
    assert!(
        backup_files(directory.path()).is_empty(),
        "a corrupt marker blocks new snapshots"
    );
    assert_eq!(
        fs::read(&database_path).expect("read source after rejection"),
        before
    );
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:341 TestBackupSnapshotRejectsBlockedDirectoryAndEmptySource
#[test]
fn backup_snapshot_rejects_an_unusable_backup_directory_and_an_empty_source_path() {
    let directory = tempdir().expect("temporary directory");
    let blocked = directory.path().join("blocked");
    fs::write(&blocked, b"not a directory").expect("write blocking file");
    let (blocked_store, _) = maintenance(
        &blocked,
        vec![descriptor(
            DATABASE_WATCHLIST,
            &directory.path().join("watchlists.db"),
        )],
    );
    assert!(
        blocked_store
            .backup(DATABASE_WATCHLIST, CREATED_AT)
            .is_err(),
        "a backup directory below a regular file cannot be created"
    );

    let (empty_source, _) = maintenance(
        directory.path(),
        vec![descriptor(DATABASE_WATCHLIST, Path::new(""))],
    );
    assert!(
        empty_source.backup(DATABASE_WATCHLIST, CREATED_AT).is_err(),
        "an empty source path never produces a snapshot"
    );
    assert!(backup_files(directory.path()).is_empty());
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:502 TestMaintenanceCompactionAndBackupFailClosedWhenPersistentStateBreaks
#[test]
fn compaction_fails_closed_when_the_rebuild_marker_is_corrupt() {
    let directory = tempdir().expect("temporary directory");
    let database_path = directory.path().join("watchlists.db");
    initialize_database(&database_path, DATABASE_WATCHLIST);
    let before = fs::read(&database_path).expect("read source database");
    let (maintenance, marker_path) = maintenance(
        directory.path(),
        vec![descriptor(DATABASE_WATCHLIST, &database_path)],
    );
    fs::write(&marker_path, b"{").expect("write corrupt marker");

    let error = maintenance
        .compact(DATABASE_WATCHLIST, CREATED_AT)
        .expect_err("compaction fails closed on a corrupt marker");
    assert!(
        error.to_string().contains("decode database rebuild marker"),
        "unexpected compaction error: {error}"
    );
    assert!(backup_files(directory.path()).is_empty());
    assert_eq!(
        fs::read(&database_path).expect("read source after rejection"),
        before,
        "a rejected compaction leaves the database untouched"
    );
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:394 TestBackupRetentionNeverEvictsRebuildMarkerSnapshots
#[test]
fn rebuild_marker_snapshots_survive_later_retention_pressure() {
    let directory = tempdir().expect("temporary directory");
    let database_path = directory.path().join("watchlists.db");
    initialize_database(&database_path, DATABASE_WATCHLIST);
    let (maintenance, marker_path) = maintenance(
        directory.path(),
        vec![descriptor(DATABASE_WATCHLIST, &database_path)],
    );
    maintenance
        .rebuild(&rebuild_request(DATABASE_WATCHLIST), CREATED_AT)
        .expect("schedule the rebuild");
    let scheduled = marker_backups(&marker_path);
    let protected = PathBuf::from(scheduled[0]["path"].as_str().expect("snapshot path"));
    assert!(protected.is_file());

    for stamp in [
        "2026-07-11T16:00:00Z",
        "2026-07-11T17:00:00Z",
        "2026-07-11T18:00:00Z",
    ] {
        maintenance
            .backup(DATABASE_WATCHLIST, stamp)
            .expect("create an ad-hoc snapshot");
    }

    assert!(
        protected.is_file(),
        "retention never evicts a snapshot a pending rebuild marker references"
    );
    assert!(
        backup_files(directory.path()).len() <= 4,
        "retention still bounds the number of snapshots per database"
    );
    maintenance
        .rebuild(&rebuild_request(DATABASE_WATCHLIST), CREATED_AT)
        .expect("the pending rebuild still verifies");
    assert_eq!(marker_backups(&marker_path).len(), 1);
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/manager_boundaries_test.go:50 TestManagerScheduleRebuildValidatesModesAndSelection
#[test]
fn rebuild_selection_rejects_ambiguous_ids_and_an_empty_incompatible_batch() {
    let directory = tempdir().expect("temporary directory");
    let database_path = directory.path().join("watchlists.db");
    initialize_database(&database_path, DATABASE_WATCHLIST);
    let (maintenance, marker_path) = maintenance(
        directory.path(),
        vec![descriptor(DATABASE_WATCHLIST, &database_path)],
    );
    let mut request = rebuild_request(DATABASE_WATCHLIST);
    request.database_ids.clear();
    let error = maintenance
        .rebuild(&request, CREATED_AT)
        .expect_err("single mode requires exactly one database id");
    assert_eq!(error.to_string(), "exactly one database id is required");

    request.database_ids = vec![DATABASE_WATCHLIST.to_owned(), DATABASE_ADK.to_owned()];
    assert!(maintenance.rebuild(&request, CREATED_AT).is_err());

    let batch = RebuildRequest {
        database_ids: Vec::new(),
        database_id: String::new(),
        mode: "incompatible".to_owned(),
        confirmation: "wrong".to_owned(),
    };
    assert!(maintenance.rebuild(&batch, CREATED_AT).is_err());
    let batch = RebuildRequest {
        confirmation: "REBUILD INCOMPATIBLE DATABASES".to_owned(),
        ..batch
    };
    let error = maintenance
        .rebuild(&batch, CREATED_AT)
        .expect_err("a compatible install needs no batch rebuild");
    assert!(
        error.to_string().contains("no databases require rebuild"),
        "unexpected batch error: {error}"
    );
    assert!(!marker_path.exists());
    assert!(backup_files(directory.path()).is_empty());
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/manager_test.go:14 TestManagerSchedulesSingleAndBatchRebuilds
#[test]
fn batch_rebuild_schedules_every_incompatible_database() {
    let directory = tempdir().expect("temporary directory");
    let adk_path = directory.path().join("adk.db");
    let watchlist_path = directory.path().join("watchlists.db");
    for path in [&adk_path, &watchlist_path] {
        Connection::open(path)
            .expect("create legacy database")
            .execute_batch("CREATE TABLE legacy (id TEXT PRIMARY KEY);")
            .expect("shape legacy database");
    }
    let (maintenance, marker_path) = maintenance(
        directory.path(),
        vec![
            descriptor(DATABASE_ADK, &adk_path),
            descriptor(DATABASE_WATCHLIST, &watchlist_path),
        ],
    );

    let result = maintenance
        .rebuild(
            &RebuildRequest {
                database_ids: Vec::new(),
                database_id: String::new(),
                mode: "incompatible".to_owned(),
                confirmation: "REBUILD INCOMPATIBLE DATABASES".to_owned(),
            },
            CREATED_AT,
        )
        .expect("schedule the batch rebuild");
    assert_eq!(
        result.database_ids,
        vec![DATABASE_ADK.to_owned(), DATABASE_WATCHLIST.to_owned()]
    );
    assert!(result.scheduled && result.restart_required);
    let backups = marker_backups(&marker_path);
    assert_eq!(
        backups.len(),
        2,
        "every scheduled database keeps a verified snapshot"
    );
    let mut ids = backups
        .iter()
        .map(|backup| backup["databaseId"].as_str().expect("backup id").to_owned())
        .collect::<Vec<_>>();
    ids.sort();
    assert_eq!(
        ids,
        vec![DATABASE_ADK.to_owned(), DATABASE_WATCHLIST.to_owned()]
    );
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/rebuild_safety_test.go:128 TestScheduleRebuildLockedRequiresBackupForExistingMarkerIDs
#[test]
fn a_marker_with_an_unbacked_database_id_blocks_scheduling() {
    let directory = tempdir().expect("temporary directory");
    let adk_path = directory.path().join("adk.db");
    let watchlist_path = directory.path().join("watchlists.db");
    initialize_database(&adk_path, DATABASE_ADK);
    initialize_database(&watchlist_path, DATABASE_WATCHLIST);
    let (maintenance, marker_path) = maintenance(
        directory.path(),
        vec![
            descriptor(DATABASE_ADK, &adk_path),
            descriptor(DATABASE_WATCHLIST, &watchlist_path),
        ],
    );
    fs::write(&marker_path, br#"{"databaseIds":["adk"]}"#).expect("write incomplete marker");

    let error = maintenance
        .rebuild(&rebuild_request(DATABASE_WATCHLIST), CREATED_AT)
        .expect_err("an unbacked marker entry blocks the schedule");
    assert!(
        error
            .to_string()
            .contains("rebuild marker is missing a verified backup"),
        "unexpected incomplete-marker error: {error}"
    );
    assert!(
        backup_files(directory.path()).is_empty(),
        "the snapshot created for the new schedule is removed"
    );
    assert_eq!(
        fs::read(&marker_path).expect("read marker after rejection"),
        br#"{"databaseIds":["adk"]}"#,
        "a rejected schedule never rewrites the marker"
    );
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/rebuild_safety_test.go:177 TestRebuildSelectionAndLockRollbackBoundaries
#[test]
fn a_held_lease_on_one_database_rolls_back_the_batch_rebuild_locks() {
    let directory = tempdir().expect("temporary directory");
    let adk_path = directory.path().join("adk.db");
    let watchlist_path = directory.path().join("watchlists.db");
    for path in [&adk_path, &watchlist_path] {
        Connection::open(path)
            .expect("create legacy database")
            .execute_batch("CREATE TABLE legacy (id TEXT PRIMARY KEY);")
            .expect("shape legacy database");
    }
    let (maintenance, marker_path) = maintenance(
        directory.path(),
        vec![
            descriptor(DATABASE_ADK, &adk_path),
            descriptor(DATABASE_WATCHLIST, &watchlist_path),
        ],
    );
    let lease = WriterLease::acquire(
        &watchlist_path,
        &OwnerDiagnostic::current("test", "held-batch-lease"),
    )
    .expect("hold the second writer lease");

    let error = maintenance
        .rebuild(
            &RebuildRequest {
                database_ids: Vec::new(),
                database_id: String::new(),
                mode: "incompatible".to_owned(),
                confirmation: "REBUILD INCOMPATIBLE DATABASES".to_owned(),
            },
            CREATED_AT,
        )
        .expect_err("the held lease rejects the batch");
    assert!(
        error.to_string().contains("writer lease"),
        "unexpected batch lease error: {error}"
    );
    drop(lease);

    assert!(!marker_path.exists());
    assert!(backup_files(directory.path()).is_empty());
    maintenance
        .backup(DATABASE_ADK, CREATED_AT)
        .expect("the first batch lock was rolled back");
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/research_lifecycle_test.go:13 TestResearchDatabaseParticipatesInStatusBackupAndRebuild
#[test]
fn research_database_participates_in_status_backup_and_rebuild() {
    let directory = tempdir().expect("temporary directory");
    let research_path = directory.path().join("research.db");
    initialize_database(&research_path, DATABASE_RESEARCH);
    let (maintenance, marker_path) = maintenance(
        directory.path(),
        vec![descriptor(DATABASE_RESEARCH, &research_path)],
    );

    let connection = Connection::open(&research_path).expect("open research database");
    assert_eq!(
        jftrade_store_sqlite::current_version(&connection, DATABASE_RESEARCH),
        Some(1)
    );
    drop(connection);
    let backup = maintenance
        .backup(DATABASE_RESEARCH, CREATED_AT)
        .expect("snapshot the research database");
    assert_eq!(backup.database_id, DATABASE_RESEARCH);
    assert!(backup.size_bytes > 0);
    assert!(PathBuf::from(&backup.backup_path).is_file());

    let result = maintenance
        .rebuild(&rebuild_request(DATABASE_RESEARCH), CREATED_AT)
        .expect("schedule the research rebuild");
    assert_eq!(result.database_ids, vec![DATABASE_RESEARCH.to_owned()]);
    let backups = marker_backups(&marker_path);
    assert_eq!(backups[0]["databaseId"], DATABASE_RESEARCH);
}
