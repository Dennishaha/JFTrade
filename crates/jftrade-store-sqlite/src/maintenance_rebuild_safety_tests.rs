//! Parity: `go:452dea11:internal/app/apiserver/datamigration/rebuild_safety_test.go:12`
//! `TestVerifyMarkerBackupRejectsEveryUntrustedMarkerField` and
//! `go:452dea11:internal/app/apiserver/datamigration/rebuild_safety_test.go:221`
//! `TestProtectedBackupAndDigestFailureBoundaries`.
//!
//! A rebuild marker is untrusted input on the next start: every field is
//! re-derived from the file system before a source database may be deleted.

use std::collections::BTreeSet;
use std::fs;
use std::path::{Path, PathBuf};

use jftrade_datamanagement::{DATABASE_STRATEGY, DATABASE_WATCHLIST, DatabaseDescriptor};
use rusqlite::Connection;
use sha2::{Digest, Sha256};
use tempfile::tempdir;

use super::*;

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

fn initialize(path: &Path, component: &str) {
    let connection = Connection::open(path).expect("create database");
    crate::initialize_current(&connection, component).expect("initialize schema");
}

fn digest(path: &Path) -> String {
    let bytes = fs::read(path).expect("read backup");
    let mut hasher = Sha256::new();
    hasher.update(&bytes);
    hasher
        .finalize()
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect()
}

fn fixture() -> (tempfile::TempDir, PathBuf, PathBuf) {
    let directory = tempdir().expect("temporary directory");
    let database_path = directory.path().join("watchlists.db");
    initialize(&database_path, DATABASE_WATCHLIST);
    let backup_directory = directory.path().join("backups");
    fs::create_dir_all(&backup_directory).expect("create backup directory");
    (directory, database_path, backup_directory)
}

fn copy_backup(source: &Path, backup_directory: &Path, name: &str) -> PathBuf {
    let backup_path = backup_directory.join(name);
    fs::copy(source, &backup_path).expect("copy snapshot");
    backup_path
}

#[test]
fn marker_snapshots_must_live_in_the_managed_directory_with_a_managed_name() {
    let (directory, database_path, backup_directory) = fixture();
    let valid = copy_backup(
        &database_path,
        &backup_directory,
        "watchlist-20260724T010203.000000000Z-abcdef12.db",
    );
    let size = i64::try_from(fs::metadata(&valid).expect("snapshot metadata").len()).expect("size");
    let sha256 = digest(&valid);

    verify_managed_rebuild_backup(
        &backup_directory,
        DATABASE_WATCHLIST,
        &valid.to_string_lossy(),
        size,
        &sha256,
    )
    .expect("a managed snapshot verifies");
    assert_eq!(
        verify_managed_rebuild_backup(
            &backup_directory,
            DATABASE_WATCHLIST,
            &database_path.to_string_lossy(),
            size,
            &sha256,
        ),
        Err("rebuild backup is outside the managed backup directory".to_owned())
    );
    assert_eq!(
        verify_managed_rebuild_backup(
            &backup_directory,
            DATABASE_WATCHLIST,
            &backup_directory.to_string_lossy(),
            size,
            &sha256,
        ),
        Err("rebuild backup is outside the managed backup directory".to_owned())
    );

    let unmanaged = copy_backup(&valid, &backup_directory, "not-managed.db");
    assert_eq!(
        verify_managed_rebuild_backup(
            &backup_directory,
            DATABASE_WATCHLIST,
            &unmanaged.to_string_lossy(),
            size,
            &sha256,
        ),
        Err("rebuild backup filename is not managed for watchlist".to_owned())
    );

    assert_eq!(
        verify_managed_rebuild_backup(
            &backup_directory,
            DATABASE_WATCHLIST,
            &valid.to_string_lossy(),
            size + 1,
            &sha256,
        ),
        Err("rebuild backup size or file type does not match marker".to_owned())
    );
    assert_eq!(
        verify_managed_rebuild_backup(
            &backup_directory,
            DATABASE_WATCHLIST,
            &valid.to_string_lossy(),
            size,
            &"0".repeat(64),
        ),
        Err("rebuild backup SHA-256 does not match marker".to_owned())
    );
    assert_eq!(
        verify_managed_rebuild_backup(
            &backup_directory,
            DATABASE_STRATEGY,
            &valid.to_string_lossy(),
            size,
            &sha256,
        ),
        Err("rebuild backup filename is not managed for strategy".to_owned())
    );

    let missing = backup_directory.join("watchlist-20260724T010204.000000000Z-abcdef13.db");
    assert!(
        verify_managed_rebuild_backup(
            &backup_directory,
            DATABASE_WATCHLIST,
            &missing.to_string_lossy(),
            size,
            &sha256,
        )
        .is_err(),
        "a missing snapshot never verifies"
    );

    let corrupt = backup_directory.join("watchlist-20260724T010205.000000000Z-abcdef14.db");
    fs::write(&corrupt, b"not a sqlite database").expect("write corrupt snapshot");
    let corrupt_size =
        i64::try_from(fs::metadata(&corrupt).expect("metadata").len()).expect("size");
    let corrupt_digest = digest(&corrupt);
    assert!(
        verify_managed_rebuild_backup(
            &backup_directory,
            DATABASE_WATCHLIST,
            &corrupt.to_string_lossy(),
            corrupt_size,
            &corrupt_digest,
        )
        .is_err(),
        "a snapshot with a matching digest must still pass quick_check"
    );

    #[cfg(unix)]
    {
        let symlink = backup_directory.join("watchlist-20260724T010206.000000000Z-abcdef15.db");
        std::os::unix::fs::symlink(&valid, &symlink).expect("create snapshot symlink");
        assert_eq!(
            verify_managed_rebuild_backup(
                &backup_directory,
                DATABASE_WATCHLIST,
                &symlink.to_string_lossy(),
                size,
                &sha256,
            ),
            Err("rebuild backup size or file type does not match marker".to_owned())
        );
    }
    assert!(directory.path().join("backups").is_dir());
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/rebuild_safety_test.go:221 TestProtectedBackupAndDigestFailureBoundaries
#[test]
fn unreadable_marker_and_digest_errors_surface_without_evicting_snapshots() {
    let (directory, database_path, _) = fixture();
    let marker_path = directory.path().join("database-rebuild.json");
    fs::write(&marker_path, b"{").expect("write corrupt marker");
    let store = ManagedDatabaseMaintenanceStore::new(
        vec![descriptor(DATABASE_WATCHLIST, &database_path)],
        marker_path.clone(),
        "rebuild-safety-test",
    );
    assert!(
        store.protected_backup_paths(&BTreeSet::new()).is_err(),
        "a corrupt marker must not silently yield an empty protected set"
    );
    assert_eq!(
        store
            .backup(DATABASE_WATCHLIST, "2026-07-24T01:02:03Z")
            .expect_err("a corrupt marker blocks new snapshots")
            .to_string(),
        "database maintenance failed: decode database rebuild marker: EOF while parsing an object at line 1 column 1"
    );

    fs::remove_file(&marker_path).expect("remove corrupt marker");
    fs::create_dir(&marker_path).expect("replace the marker with a directory");
    assert!(
        store.protected_backup_paths(&BTreeSet::new()).is_err(),
        "an unreadable marker must surface as an error"
    );

    assert!(
        file_sha256(&directory.path().join("missing")).is_err(),
        "a missing snapshot has no digest"
    );
    assert!(
        file_sha256(directory.path()).is_err(),
        "a directory has no snapshot digest"
    );
}

// Parity: go:452dea11:internal/app/apiserver/datamigration/managed_backup_retention_test.go:105 TestBackupSnapshotFailureCleansUpPartialFilesAndVerificationRejectsInvalidSQLite
#[test]
fn snapshot_verification_rejects_invalid_sqlite_and_failed_backups_leave_no_partial_file() {
    let directory = tempdir().expect("temporary directory");
    let invalid = directory.path().join("not-sqlite.db");
    fs::write(&invalid, b"not a sqlite database").expect("write invalid snapshot");
    assert!(
        verify_backup(&invalid).is_err(),
        "a file that is not SQLite never verifies"
    );
    assert!(
        verify_backup(&directory.path().join("missing.db")).is_err(),
        "a missing snapshot never verifies"
    );
    let empty = directory.path().join("empty.db");
    fs::write(&empty, b"").expect("write empty snapshot");
    assert!(
        verify_backup(&empty).is_err(),
        "an empty snapshot never verifies"
    );

    let source_directory = directory.path().join("source.db");
    fs::create_dir(&source_directory).expect("create directory source");
    fs::write(source_directory.join("child"), b"x").expect("make the directory non-empty");
    let store = ManagedDatabaseMaintenanceStore::new(
        vec![descriptor(DATABASE_WATCHLIST, &source_directory)],
        directory.path().join("database-rebuild.json"),
        "rebuild-safety-test",
    );
    assert!(
        store
            .backup(DATABASE_WATCHLIST, "2026-07-24T01:02:03Z")
            .is_err(),
        "a source that is not a regular file has no snapshot"
    );
    let backups = directory.path().join("backups");
    let entries = fs::read_dir(&backups)
        .map(|entries| {
            entries
                .map(|entry| entry.expect("backup entry").path())
                .collect::<Vec<_>>()
        })
        .unwrap_or_default();
    assert!(
        entries.is_empty(),
        "a failed backup removes every partial snapshot: {entries:?}"
    );
}
