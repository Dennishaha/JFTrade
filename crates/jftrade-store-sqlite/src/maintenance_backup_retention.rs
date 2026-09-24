//! Managed backup retention for the database maintenance store.
//!
//! Mirrors the Go owner (`internal/app/apiserver/datamigration/backup_retention.go`):
//! every managed snapshot lives in `<data root>/backups` named
//! `{databaseId}-{stamp}-{token}.db`. Retention keeps at most three snapshots
//! per database, never evicts a snapshot that a pending rebuild marker still
//! references, and fails closed when the directory cannot satisfy the quota.

use std::collections::{BTreeMap, BTreeSet};
use std::fs;
use std::path::{Path, PathBuf};

use jftrade_datamanagement::{DatabaseDescriptor, MaintenanceOperationError};

use crate::maintenance::database_bytes;

pub(crate) const BACKUP_RETENTION_PER_DATABASE: usize = 3;
pub(crate) const BACKUP_QUOTA_FLOOR_BYTES: i64 = 5 * (1 << 30);

#[derive(Clone, Debug, Eq, PartialEq)]
pub(crate) struct ManagedBackupFile {
    pub(crate) path: PathBuf,
    pub(crate) database_id: String,
    pub(crate) size_bytes: i64,
    sort_key: String,
}

/// Normalize the snapshot timestamp the same way the filename writer does.
pub(crate) fn managed_backup_stamp(created_at: &str) -> String {
    created_at
        .chars()
        .filter(|value| value.is_ascii_alphanumeric() || *value == '.')
        .collect()
}

/// Parse `{databaseId}-{YYYYMMDDThhmmss[.fffffffff]Z}-{8 hex}.db`.
///
/// Returns the owning database id plus a sort key that orders snapshots
/// oldest-first without depending on filesystem timestamps.
pub(crate) fn parse_managed_backup_filename(
    database_ids: &BTreeSet<String>,
    filename: &str,
) -> Option<(String, String)> {
    let stem = filename.strip_suffix(".db")?;
    for database_id in database_ids {
        let Some(rest) = stem.strip_prefix(database_id.as_str()) else {
            continue;
        };
        let Some(rest) = rest.strip_prefix('-') else {
            continue;
        };
        let Some((stamp, token)) = rest.rsplit_once('-') else {
            continue;
        };
        if token.len() != 8 || !token.bytes().all(|byte| byte.is_ascii_hexdigit()) {
            continue;
        }
        if let Some(sort_key) = canonical_stamp(stamp) {
            return Some((database_id.clone(), sort_key));
        }
    }
    None
}

fn canonical_stamp(stamp: &str) -> Option<String> {
    let (date, remainder) = stamp.split_once('T')?;
    if date.len() != 8 || !date.bytes().all(|byte| byte.is_ascii_digit()) {
        return None;
    }
    let remainder = remainder.strip_suffix('Z')?;
    let (clock, fraction) = match remainder.split_once('.') {
        Some((clock, fraction)) => (clock, fraction),
        None => (remainder, ""),
    };
    if clock.len() != 6 || !clock.bytes().all(|byte| byte.is_ascii_digit()) {
        return None;
    }
    if fraction.len() > 9 || !fraction.bytes().all(|byte| byte.is_ascii_digit()) {
        return None;
    }
    let mut key = format!("{date}T{clock}");
    if !fraction.is_empty() {
        key.push('.');
        key.push_str(fraction);
        for _ in fraction.len()..9 {
            key.push('0');
        }
    }
    Some(key)
}

pub(crate) fn list_managed_backup_files(
    directory: &Path,
    database_ids: &BTreeSet<String>,
) -> Result<Vec<ManagedBackupFile>, MaintenanceOperationError> {
    let entries = fs::read_dir(directory)
        .map_err(|error| maintenance_failed(format!("read database backup directory: {error}")))?;
    let mut files = Vec::new();
    for entry in entries {
        let entry = entry.map_err(|error| {
            maintenance_failed(format!("read database backup directory: {error}"))
        })?;
        let Some(name) = entry.file_name().to_str().map(str::to_owned) else {
            continue;
        };
        let Some((database_id, sort_key)) = parse_managed_backup_filename(database_ids, &name)
        else {
            continue;
        };
        let metadata = entry.path().symlink_metadata().map_err(|error| {
            maintenance_failed(format!("inspect database backup {name}: {error}"))
        })?;
        if !metadata.file_type().is_file() {
            continue;
        }
        files.push(ManagedBackupFile {
            path: entry.path(),
            database_id,
            size_bytes: i64::try_from(metadata.len()).unwrap_or(i64::MAX),
            sort_key,
        });
    }
    files.sort_by(|left, right| {
        left.sort_key
            .cmp(&right.sort_key)
            .then_with(|| left.path.cmp(&right.path))
    });
    Ok(files)
}

/// Quota shared by every managed snapshot: twice the managed database bytes,
/// never below the 5 GiB floor.
pub(crate) fn backup_quota_bytes(descriptors: &BTreeMap<String, DatabaseDescriptor>) -> i64 {
    let mut source_bytes = 0_i64;
    for descriptor in descriptors.values() {
        source_bytes = source_bytes.saturating_add(database_bytes(Path::new(&descriptor.path)));
    }
    BACKUP_QUOTA_FLOOR_BYTES.max(source_bytes.saturating_mul(2))
}

/// Free enough room for one snapshot of `database_id` before it is written.
pub(crate) fn prepare_backup_capacity(
    directory: &Path,
    database_ids: &BTreeSet<String>,
    database_id: &str,
    reserve_bytes: i64,
    quota_bytes: i64,
    protected: &BTreeSet<PathBuf>,
) -> Result<(), MaintenanceOperationError> {
    let files = list_managed_backup_files(directory, database_ids)?;
    let mut remaining_for_database = files
        .iter()
        .filter(|file| file.database_id == database_id)
        .count();
    let mut removed = BTreeSet::new();
    for file in &files {
        if file.database_id != database_id
            || remaining_for_database < BACKUP_RETENTION_PER_DATABASE
            || protected.contains(&file.path)
        {
            continue;
        }
        remove_managed_backup(file)?;
        removed.insert(file.path.clone());
        remaining_for_database -= 1;
    }
    if reserve_bytes > quota_bytes {
        return Err(quota_exceeded(format!(
            "backup requires {reserve_bytes} bytes but quota is {quota_bytes} bytes"
        )));
    }
    let mut total_bytes = files
        .iter()
        .filter(|file| !removed.contains(&file.path))
        .fold(0_i64, |total, file| total.saturating_add(file.size_bytes));
    for file in &files {
        if total_bytes <= quota_bytes.saturating_sub(reserve_bytes) {
            break;
        }
        if removed.contains(&file.path) || protected.contains(&file.path) {
            continue;
        }
        remove_managed_backup(file)?;
        removed.insert(file.path.clone());
        total_bytes = total_bytes.saturating_sub(file.size_bytes);
    }
    if total_bytes > quota_bytes.saturating_sub(reserve_bytes) {
        return Err(quota_exceeded(format!(
            "backup directory uses {total_bytes} bytes and requires {reserve_bytes} more bytes"
        )));
    }
    Ok(())
}

/// Prune snapshots after a successful write so the retained set fits the quota.
pub(crate) fn enforce_backup_retention(
    directory: &Path,
    database_ids: &BTreeSet<String>,
    current_path: &Path,
    quota_bytes: i64,
    protected: &BTreeSet<PathBuf>,
) -> Result<(), MaintenanceOperationError> {
    let files = list_managed_backup_files(directory, database_ids)?;
    let mut protected = protected.clone();
    protected.insert(current_path.to_path_buf());
    let mut counts = BTreeMap::new();
    let mut current_database_id = None;
    for file in &files {
        *counts.entry(file.database_id.clone()).or_insert(0_usize) += 1;
        if file.path == current_path {
            current_database_id = Some(file.database_id.clone());
            if file.size_bytes > quota_bytes {
                return Err(quota_exceeded(format!(
                    "backup is {} bytes but quota is {quota_bytes} bytes",
                    file.size_bytes
                )));
            }
        }
    }
    let mut removed = BTreeSet::new();
    for file in &files {
        let count = counts.get(&file.database_id).copied().unwrap_or(0);
        if Some(&file.database_id) != current_database_id.as_ref()
            || count <= BACKUP_RETENTION_PER_DATABASE
            || protected.contains(&file.path)
        {
            continue;
        }
        remove_managed_backup(file)?;
        removed.insert(file.path.clone());
        counts.insert(file.database_id.clone(), count - 1);
    }
    let mut total_bytes = files
        .iter()
        .filter(|file| !removed.contains(&file.path))
        .fold(0_i64, |total, file| total.saturating_add(file.size_bytes));
    for file in &files {
        if total_bytes <= quota_bytes {
            break;
        }
        if removed.contains(&file.path) || protected.contains(&file.path) {
            continue;
        }
        remove_managed_backup(file)?;
        removed.insert(file.path.clone());
        total_bytes = total_bytes.saturating_sub(file.size_bytes);
    }
    if total_bytes > quota_bytes {
        return Err(quota_exceeded(format!(
            "backup directory uses {total_bytes} bytes"
        )));
    }
    Ok(())
}

fn remove_managed_backup(file: &ManagedBackupFile) -> Result<(), MaintenanceOperationError> {
    fs::remove_file(&file.path).map_err(|error| {
        maintenance_failed(format!(
            "remove expired database backup {}: {error}",
            file.path
                .file_name()
                .map(|name| name.to_string_lossy().into_owned())
                .unwrap_or_default()
        ))
    })
}

fn quota_exceeded(message: String) -> MaintenanceOperationError {
    MaintenanceOperationError::QuotaExceeded(message)
}

fn maintenance_failed(message: String) -> MaintenanceOperationError {
    MaintenanceOperationError::Failed(message)
}

#[cfg(test)]
mod tests {
    use std::collections::{BTreeMap, BTreeSet};
    use std::fs;
    use std::path::{Path, PathBuf};

    use tempfile::tempdir;

    use super::*;

    fn managed_ids() -> BTreeSet<String> {
        ["adk", "research", "watchlist"]
            .into_iter()
            .map(str::to_owned)
            .collect()
    }

    fn backup_directory(root: &Path) -> PathBuf {
        let directory = root.join("backups");
        fs::create_dir_all(&directory).expect("create backup directory");
        directory
    }

    fn write_backup(
        directory: &Path,
        database_id: &str,
        stamp: &str,
        token: &str,
        size: usize,
    ) -> PathBuf {
        let path = directory.join(format!("{database_id}-{stamp}-{token}.db"));
        fs::write(&path, vec![0_u8; size]).expect("write managed backup");
        path
    }

    fn total_bytes(files: &[ManagedBackupFile]) -> i64 {
        files.iter().map(|file| file.size_bytes).sum()
    }

    // Parity: go:452dea11:internal/app/apiserver/datamigration/maintenance_test.go:147 TestBackupCapacityPrunesManagedFilesOnlyAndEnforcesQuota
    #[test]
    fn backup_capacity_prunes_only_managed_snapshots_and_enforces_quota() {
        let root = tempdir().expect("temporary directory");
        let directory = backup_directory(root.path());
        let ids = managed_ids();
        write_backup(
            &directory,
            "watchlist",
            "20260711T140000.000000000Z",
            "00000001",
            3,
        );
        write_backup(
            &directory,
            "watchlist",
            "20260711T140100.000000000Z",
            "00000002",
            4,
        );
        write_backup(
            &directory,
            "watchlist",
            "20260711T140200.000000000Z",
            "00000003",
            5,
        );
        let unmanaged = directory.join("watchlist-not-managed.db");
        fs::write(&unmanaged, vec![0_u8; 20]).expect("write unmanaged file");

        prepare_backup_capacity(&directory, &ids, "watchlist", 4, 10, &BTreeSet::new())
            .expect("prepare capacity");
        let files = list_managed_backup_files(&directory, &ids).expect("list managed backups");
        assert_eq!(
            files.len(),
            1,
            "capacity pruning keeps at most one snapshot"
        );
        assert!(
            total_bytes(&files) <= 6,
            "reserve must fit inside the quota"
        );
        assert!(unmanaged.is_file(), "unmanaged files are never pruned");

        assert_eq!(
            prepare_backup_capacity(&directory, &ids, "watchlist", 11, 10, &BTreeSet::new(),),
            Err(MaintenanceOperationError::QuotaExceeded(
                "backup requires 11 bytes but quota is 10 bytes".to_owned()
            ))
        );
    }

    // Parity: go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:358 TestBackupRetentionEvictsQuotaPressureAcrossDatabaseFiles
    #[test]
    fn backup_retention_keeps_three_snapshots_and_reports_an_over_quota_snapshot() {
        let root = tempdir().expect("temporary directory");
        let directory = backup_directory(root.path());
        let ids = managed_ids();
        let other = write_backup(
            &directory,
            "adk",
            "20260711T140000.000000000Z",
            "00000005",
            4,
        );
        let mut watchlist = Vec::new();
        for index in 0..4 {
            watchlist.push(write_backup(
                &directory,
                "watchlist",
                &format!("20260711T140{}00.000000000Z", index),
                &format!("0000000{index}"),
                4,
            ));
        }
        let current = watchlist.last().cloned().expect("current snapshot");

        enforce_backup_retention(&directory, &ids, &current, 8, &BTreeSet::new())
            .expect("enforce retention");
        assert!(current.is_file(), "the current snapshot is always retained");
        let files = list_managed_backup_files(&directory, &ids).expect("list retained snapshots");
        assert!(files.len() <= 2, "quota pressure prunes older snapshots");
        assert!(
            total_bytes(&files) <= 8,
            "retained bytes stay inside the quota"
        );
        assert!(
            !other.exists(),
            "quota pressure prunes snapshots of other databases too"
        );

        assert_eq!(
            enforce_backup_retention(&directory, &ids, &current, 1, &BTreeSet::new()),
            Err(MaintenanceOperationError::QuotaExceeded(
                "backup is 4 bytes but quota is 1 bytes".to_owned()
            ))
        );
    }

    // Parity: go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:394 TestBackupRetentionNeverEvictsRebuildMarkerSnapshots
    #[test]
    fn backup_retention_never_evicts_snapshots_a_rebuild_marker_references() {
        let root = tempdir().expect("temporary directory");
        let directory = backup_directory(root.path());
        let ids = managed_ids();
        let protected = write_backup(
            &directory,
            "adk",
            "20260716T090000.000000000Z",
            "abcdef12",
            5,
        );
        let expendable = write_backup(
            &directory,
            "research",
            "20260716T090100.000000000Z",
            "abcdef13",
            5,
        );
        let mut protected_paths = BTreeSet::new();
        protected_paths.insert(protected.clone());

        prepare_backup_capacity(&directory, &ids, "watchlist", 5, 10, &protected_paths)
            .expect("prepare capacity");
        assert!(protected.is_file(), "marker snapshots are never evicted");
        assert!(!expendable.exists(), "unprotected snapshots yield to quota");

        let current = write_backup(
            &directory,
            "watchlist",
            "20260716T090200.000000000Z",
            "abcdef14",
            5,
        );
        let expendable = write_backup(
            &directory,
            "research",
            "20260716T090300.000000000Z",
            "abcdef15",
            5,
        );
        enforce_backup_retention(&directory, &ids, &current, 10, &protected_paths)
            .expect("enforce retention");
        assert!(protected.is_file() && current.is_file());
        assert!(!expendable.exists());

        let transient = write_backup(
            &directory,
            "research",
            "20260716T090400.000000000Z",
            "abcdef16",
            5,
        );
        let expendable = write_backup(
            &directory,
            "adk",
            "20260716T090500.000000000Z",
            "abcdef17",
            5,
        );
        let mut transient_paths = protected_paths.clone();
        transient_paths.insert(transient.clone());
        prepare_backup_capacity(&directory, &ids, "adk-session", 5, 15, &transient_paths)
            .expect("prepare capacity with a transient snapshot");
        assert!(
            transient.is_file(),
            "snapshots created earlier in the same rebuild batch are protected"
        );
        assert!(!expendable.exists());
    }

    // Parity: go:452dea11:internal/app/apiserver/datamigration/managed_backup_retention_test.go:61 TestManagedBackupFileDiscoveryAndFilenameBoundaries
    // Parity: go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:358 TestBackupRetentionEvictsQuotaPressureAcrossDatabaseFiles
    #[test]
    fn managed_backup_discovery_parses_only_canonical_filenames() {
        let root = tempdir().expect("temporary directory");
        let ids = managed_ids();
        let directory = root.path().join("missing");
        assert!(
            list_managed_backup_files(&directory, &ids).is_err(),
            "a missing backup directory must fail discovery"
        );

        let directory = backup_directory(root.path());
        let valid = write_backup(
            &directory,
            "watchlist",
            "20260711T140000.000000000Z",
            "abcdef12",
            8,
        );
        fs::write(directory.join("watchlist-invalid-token.db"), b"ignored").expect("invalid name");
        fs::write(
            directory.join("unknown-20260711T140000.000000000Z-deadbeef.db"),
            b"ignored",
        )
        .expect("unknown database name");
        fs::create_dir(directory.join("watchlist-20260711T140100.000000000Z-12345678.db"))
            .expect("directory shaped like a snapshot");

        let files = list_managed_backup_files(&directory, &ids).expect("list managed backups");
        assert_eq!(files.len(), 1);
        assert_eq!(files[0].path, valid);
        assert_eq!(files[0].database_id, "watchlist");

        assert_eq!(
            parse_managed_backup_filename(&ids, "watchlist-20260711T140000.000000000Z-abcdef12.db")
                .map(|(database_id, _)| database_id),
            Some("watchlist".to_owned())
        );
        for name in [
            "",
            "watchlist-20260711T140000.000000000Z-deadbeef.txt",
            "unknown-20260711T140000.000000000Z-deadbeef.db",
            "watchlist-not-a-time-deadbeef.db",
            "watchlist-20260711T140300.000000000Z-zzzzzzzz.db",
        ] {
            assert_eq!(
                parse_managed_backup_filename(&ids, name).map(|(database_id, _)| database_id),
                None,
                "{name} must not be treated as a managed snapshot"
            );
        }
    }

    // Parity: go:452dea11:internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:245 TestBackupRetentionReportsRemovalPermissionFailures
    #[test]
    fn backup_capacity_surfaces_a_removal_failure_without_reporting_success() {
        let root = tempdir().expect("temporary directory");
        let directory = backup_directory(root.path());
        let ids = managed_ids();
        let expired = write_backup(
            &directory,
            "watchlist",
            "20260716T090000.000000000Z",
            "abcdef12",
            5,
        );
        write_backup(
            &directory,
            "watchlist",
            "20260716T090100.000000000Z",
            "abcdef13",
            5,
        );
        write_backup(
            &directory,
            "watchlist",
            "20260716T090200.000000000Z",
            "abcdef14",
            5,
        );
        let permissions = fs::metadata(&directory).expect("backup directory metadata");
        let mut read_only = permissions.permissions();
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            read_only.set_mode(0o500);
        }
        fs::set_permissions(&directory, read_only).expect("restrict backup directory");
        let result =
            prepare_backup_capacity(&directory, &ids, "watchlist", 5, 10, &BTreeSet::new());
        let mut restore = permissions.permissions();
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            restore.set_mode(0o700);
        }
        fs::set_permissions(&directory, restore).expect("restore backup directory");

        match result {
            Ok(()) => {
                let files =
                    list_managed_backup_files(&directory, &ids).expect("list retained snapshots");
                assert!(
                    files.len() < 3,
                    "capacity preparation may only report success after pruning a snapshot"
                );
                assert!(
                    total_bytes(&files) <= 5,
                    "capacity preparation must free room for the reserve"
                );
            }
            Err(error) => {
                assert!(expired.is_file(), "a failed removal keeps the snapshot");
                assert!(
                    error.to_string().contains("remove expired database backup"),
                    "unexpected removal error: {error}"
                );
            }
        }
    }

    // Parity: go:452dea11:internal/app/apiserver/datamigration/managed_backup_retention_test.go:12 TestManagedBackupRetentionKeepsCurrentSnapshotAndPrunesOldFiles
    #[test]
    fn backup_retention_keeps_three_snapshots_and_prunes_the_oldest_of_one_database() {
        let root = tempdir().expect("temporary directory");
        let directory = backup_directory(root.path());
        let ids = managed_ids();
        let mut watchlist = Vec::new();
        for index in 0..4 {
            watchlist.push(write_backup(
                &directory,
                "watchlist",
                &format!("20260711T140{index}00.000000000Z"),
                &format!("0000000{index}"),
                4,
            ));
        }
        let current = watchlist.last().cloned().expect("current snapshot");
        let oldest = watchlist.first().cloned().expect("oldest snapshot");
        let second = watchlist.get(1).cloned().expect("second snapshot");

        enforce_backup_retention(&directory, &ids, &current, 5 * (1 << 30), &BTreeSet::new())
            .expect("enforce retention");
        assert!(
            !oldest.exists(),
            "the oldest snapshot of one database is pruned"
        );
        assert!(current.is_file(), "the current snapshot is retained");
        let files = list_managed_backup_files(&directory, &ids).expect("list retained snapshots");
        assert_eq!(files.len(), 3, "three snapshots per database are kept");
        assert_eq!(files[0].path, second);
        assert_eq!(total_bytes(&files), 12);
        assert_eq!(
            enforce_backup_retention(&directory, &ids, &current, 2, &BTreeSet::new()),
            Err(MaintenanceOperationError::QuotaExceeded(
                "backup is 4 bytes but quota is 2 bytes".to_owned()
            )),
            "a snapshot larger than the quota fails closed"
        );
    }

    #[test]
    fn backup_quota_floor_dominates_small_managed_databases() {
        let root = tempdir().expect("temporary directory");
        let descriptor = |id: &str, name: &str| DatabaseDescriptor {
            id: id.to_owned(),
            name: name.to_owned(),
            path: root
                .path()
                .join(format!("{id}.db"))
                .to_string_lossy()
                .into_owned(),
            description: String::new(),
            features: Vec::new(),
            expected_version: 1,
        };
        let descriptors = BTreeMap::from([
            ("backtest".to_owned(), descriptor("backtest", "backtest")),
            ("watchlist".to_owned(), descriptor("watchlist", "watchlist")),
        ]);
        assert_eq!(backup_quota_bytes(&descriptors), BACKUP_QUOTA_FLOOR_BYTES);
    }
}
