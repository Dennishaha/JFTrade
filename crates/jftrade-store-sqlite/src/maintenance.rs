use std::collections::{BTreeMap, BTreeSet};
use std::fs::{self, File};
use std::io::{Read, Write};
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::Duration;

use jftrade_datamanagement::{
    ApprovedCleanupPreview, BackupResult, CleanupCandidate, CleanupResult, CompactResult,
    DatabaseDescriptor, DatabaseMaintenancePort, MaintenanceOperationError, RebuildRequest,
    RebuildResult, verify_execute,
};
use jftrade_owner_lock::{OwnerDiagnostic, WriterLease, WriterLeaseError};
use rusqlite::{Connection, OpenFlags, Transaction, TransactionBehavior, params};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use tempfile::Builder;

use crate::data_management::maintenance_candidates;
use crate::schema_manifest::validate_current;

const BATCH_REBUILD_CONFIRMATION: &str = "REBUILD INCOMPATIBLE DATABASES";
static BACKUP_SEQUENCE: AtomicU64 = AtomicU64::new(1);

pub struct ManagedDatabaseMaintenanceStore {
    descriptors: BTreeMap<String, DatabaseDescriptor>,
    marker_path: PathBuf,
    profile: String,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
struct RebuildMarker {
    database_ids: Vec<String>,
    backups: Vec<VerifiedBackup>,
    created_at: String,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
struct VerifiedBackup {
    database_id: String,
    path: String,
    size_bytes: i64,
    sha256: String,
}

impl ManagedDatabaseMaintenanceStore {
    pub fn new(
        descriptors: Vec<DatabaseDescriptor>,
        marker_path: impl Into<PathBuf>,
        profile: impl Into<String>,
    ) -> Self {
        Self {
            descriptors: descriptors
                .into_iter()
                .map(|descriptor| (descriptor.id.clone(), descriptor))
                .collect(),
            marker_path: marker_path.into(),
            profile: profile.into(),
        }
    }

    fn descriptor(
        &self,
        database_id: &str,
    ) -> Result<&DatabaseDescriptor, MaintenanceOperationError> {
        self.descriptors.get(database_id.trim()).ok_or_else(|| {
            MaintenanceOperationError::Rejected(format!(
                "unknown database id {:?}",
                database_id.trim()
            ))
        })
    }

    fn lease(
        &self,
        descriptor: &DatabaseDescriptor,
    ) -> Result<WriterLease, MaintenanceOperationError> {
        WriterLease::acquire(
            &descriptor.path,
            &OwnerDiagnostic::current("rust-sqlite", self.profile.clone()),
        )
        .map_err(map_lease_error)
    }

    fn open_ready(
        &self,
        descriptor: &DatabaseDescriptor,
    ) -> Result<Connection, MaintenanceOperationError> {
        let connection = open_read_write(Path::new(&descriptor.path))?;
        validate_current(
            &connection,
            &descriptor.path,
            &descriptor.id,
            descriptor.expected_version,
        )
        .map_err(|error| MaintenanceOperationError::Rejected(error.to_string()))?;
        Ok(connection)
    }

    fn create_backup_locked(
        &self,
        descriptor: &DatabaseDescriptor,
        created_at: &str,
    ) -> Result<(BackupResult, VerifiedBackup), MaintenanceOperationError> {
        let backup_directory = self
            .marker_path
            .parent()
            .unwrap_or_else(|| Path::new("."))
            .join("backups");
        fs::create_dir_all(&backup_directory).map_err(failed)?;
        harden_directory(&backup_directory)?;
        let sequence = BACKUP_SEQUENCE.fetch_add(1, Ordering::Relaxed);
        let timestamp = created_at
            .bytes()
            .filter(u8::is_ascii_alphanumeric)
            .map(char::from)
            .collect::<String>();
        let backup_path = backup_directory.join(format!(
            "{}-{}-{:08x}.db",
            descriptor.id, timestamp, sequence
        ));
        if backup_path.exists() {
            return Err(MaintenanceOperationError::Conflict(
                "backup filename collision".to_owned(),
            ));
        }
        let source = open_read_write(Path::new(&descriptor.path))?;
        let quoted = backup_path.to_string_lossy().replace('\'', "''");
        if let Err(error) = source.execute_batch(&format!("VACUUM INTO '{quoted}'")) {
            let _ = fs::remove_file(&backup_path);
            return Err(failed(error));
        }
        harden_path(&backup_path)?;
        if let Err(error) = verify_backup(&backup_path) {
            let _ = fs::remove_file(&backup_path);
            return Err(error);
        }
        let size_bytes = file_bytes(&backup_path);
        let sha256 = file_sha256(&backup_path)?;
        let result = BackupResult {
            database_id: descriptor.id.clone(),
            backup_path: backup_path.to_string_lossy().into_owned(),
            size_bytes,
            created_at: created_at.to_owned(),
        };
        let verified = VerifiedBackup {
            database_id: descriptor.id.clone(),
            path: result.backup_path.clone(),
            size_bytes,
            sha256,
        };
        Ok((result, verified))
    }

    fn select_rebuild_ids(
        &self,
        request: &RebuildRequest,
    ) -> Result<Vec<String>, MaintenanceOperationError> {
        let mut ids = request
            .database_ids
            .iter()
            .chain(std::iter::once(&request.database_id))
            .map(|value| value.trim().to_owned())
            .filter(|value| !value.is_empty())
            .collect::<BTreeSet<_>>()
            .into_iter()
            .collect::<Vec<_>>();
        if request.mode.trim() == "incompatible" {
            if request.confirmation.trim() != BATCH_REBUILD_CONFIRMATION {
                return Err(rejected("confirmation text does not match"));
            }
            ids = self
                .descriptors
                .values()
                .filter(|descriptor| database_is_incompatible(descriptor))
                .map(|descriptor| descriptor.id.clone())
                .collect();
        } else {
            if ids.len() != 1 {
                return Err(rejected("exactly one database id is required"));
            }
            self.descriptor(&ids[0])?;
            if request.confirmation.trim() != format!("REBUILD {}", ids[0]) {
                return Err(rejected("confirmation text does not match"));
            }
        }
        if ids.is_empty() {
            return Err(rejected("no databases require rebuild"));
        }
        Ok(ids)
    }

    fn read_marker(&self) -> Result<Option<RebuildMarker>, MaintenanceOperationError> {
        match fs::read(&self.marker_path) {
            Ok(bytes) => serde_json::from_slice(&bytes).map(Some).map_err(failed),
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(None),
            Err(error) => Err(failed(error)),
        }
    }

    fn write_marker(&self, marker: &RebuildMarker) -> Result<(), MaintenanceOperationError> {
        let directory = self.marker_path.parent().unwrap_or_else(|| Path::new("."));
        fs::create_dir_all(directory).map_err(failed)?;
        let mut temporary = Builder::new()
            .prefix(".database-rebuild-")
            .suffix(".tmp")
            .tempfile_in(directory)
            .map_err(failed)?;
        harden_file(temporary.as_file())?;
        serde_json::to_writer_pretty(&mut temporary, marker).map_err(failed)?;
        temporary.write_all(b"\n").map_err(failed)?;
        temporary.as_file().sync_all().map_err(failed)?;
        temporary
            .persist(&self.marker_path)
            .map_err(|error| failed(error.error))?;
        sync_directory(directory)?;
        Ok(())
    }
}

impl DatabaseMaintenancePort for ManagedDatabaseMaintenanceStore {
    fn execute_cleanup(
        &self,
        approved: &ApprovedCleanupPreview,
    ) -> Result<CleanupResult, MaintenanceOperationError> {
        let descriptor = self.descriptor(&approved.response.database_id)?;
        let _lease = self.lease(descriptor)?;
        let before_bytes = database_bytes(Path::new(&descriptor.path));
        let mut connection = self.open_ready(descriptor)?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(failed)?;
        verify_candidates(&transaction, descriptor, approved)?;
        let deleted_count = delete_candidates(&transaction, descriptor, approved)?;
        transaction.commit().map_err(failed)?;
        let compact_error = compact_connection(&connection).err();
        drop(connection);
        let after_bytes = database_bytes(Path::new(&descriptor.path));
        Ok(CleanupResult {
            database_id: descriptor.id.clone(),
            deleted_count,
            estimated_bytes: approved.response.estimated_bytes,
            before_bytes,
            after_bytes,
            reclaimed_bytes: before_bytes.saturating_sub(after_bytes),
            compacted: compact_error.is_none(),
            warning: compact_error
                .map(|error| format!("数据已删除，但文件未完全收缩：{error}"))
                .unwrap_or_default(),
        })
    }

    fn compact(
        &self,
        database_id: &str,
        created_at: &str,
    ) -> Result<CompactResult, MaintenanceOperationError> {
        let descriptor = self.descriptor(database_id)?;
        let _lease = self.lease(descriptor)?;
        let connection = self.open_ready(descriptor)?;
        let before_bytes = database_bytes(Path::new(&descriptor.path));
        self.create_backup_locked(descriptor, created_at)?;
        compact_connection(&connection)?;
        drop(connection);
        let after_bytes = database_bytes(Path::new(&descriptor.path));
        Ok(CompactResult {
            database_id: descriptor.id.clone(),
            before_bytes,
            after_bytes,
            reclaimed_bytes: before_bytes.saturating_sub(after_bytes),
            compacted: true,
        })
    }

    fn backup(
        &self,
        database_id: &str,
        created_at: &str,
    ) -> Result<BackupResult, MaintenanceOperationError> {
        let descriptor = self.descriptor(database_id)?;
        let _lease = self.lease(descriptor)?;
        self.create_backup_locked(descriptor, created_at)
            .map(|(result, _)| result)
    }

    fn rebuild(
        &self,
        request: &RebuildRequest,
        created_at: &str,
    ) -> Result<RebuildResult, MaintenanceOperationError> {
        let ids = self.select_rebuild_ids(request)?;
        let descriptors = ids
            .iter()
            .map(|id| self.descriptor(id).cloned())
            .collect::<Result<Vec<_>, _>>()?;
        for descriptor in &descriptors {
            ensure_available_for_backup(descriptor)?;
        }
        let mut leases = Vec::with_capacity(descriptors.len());
        for descriptor in &descriptors {
            leases.push(self.lease(descriptor)?);
        }
        let existing = self.read_marker()?.unwrap_or_else(|| RebuildMarker {
            database_ids: Vec::new(),
            backups: Vec::new(),
            created_at: created_at.to_owned(),
        });
        for backup in &existing.backups {
            verify_marker_backup(&self.marker_path, backup)?;
        }
        let mut backups = existing
            .backups
            .into_iter()
            .map(|backup| (backup.database_id.clone(), backup))
            .collect::<BTreeMap<_, _>>();
        let mut created_paths = Vec::new();
        for descriptor in &descriptors {
            if backups.contains_key(&descriptor.id) {
                continue;
            }
            let (_, backup) = match self.create_backup_locked(descriptor, created_at) {
                Ok(result) => result,
                Err(error) => {
                    remove_files(&created_paths);
                    return Err(error);
                }
            };
            created_paths.push(PathBuf::from(&backup.path));
            backups.insert(descriptor.id.clone(), backup);
        }
        let database_ids = existing
            .database_ids
            .into_iter()
            .chain(ids)
            .collect::<BTreeSet<_>>()
            .into_iter()
            .collect::<Vec<_>>();
        let marker = RebuildMarker {
            backups: database_ids
                .iter()
                .filter_map(|id| backups.get(id).cloned())
                .collect(),
            database_ids: database_ids.clone(),
            created_at: created_at.to_owned(),
        };
        if marker.backups.len() != marker.database_ids.len() {
            remove_files(&created_paths);
            return Err(failed("rebuild marker is missing a verified backup"));
        }
        if let Err(error) = self.write_marker(&marker) {
            remove_files(&created_paths);
            return Err(error);
        }
        drop(leases);
        Ok(RebuildResult {
            database_ids,
            restart_required: true,
            scheduled: true,
        })
    }
}

fn verify_candidates(
    transaction: &Transaction<'_>,
    descriptor: &DatabaseDescriptor,
    approved: &ApprovedCleanupPreview,
) -> Result<(), MaintenanceOperationError> {
    let current = maintenance_candidates(transaction, descriptor, &approved.query)
        .map_err(MaintenanceOperationError::Failed)?;
    let current = current
        .into_iter()
        .map(|candidate| CleanupCandidate {
            id: candidate.id,
            category: candidate.category,
        })
        .collect();
    let preview = jftrade_datamanagement::CleanupPreview {
        database_id: approved.response.database_id.clone(),
        candidates: approved
            .candidates
            .iter()
            .map(|candidate| CleanupCandidate {
                id: candidate.id.clone(),
                category: candidate.category.clone(),
            })
            .collect(),
        fingerprint: approved.fingerprint.clone(),
    };
    verify_execute(&preview, current, None)
        .map(|_| ())
        .map_err(|_| MaintenanceOperationError::Stale)
}

fn delete_candidates(
    transaction: &Transaction<'_>,
    descriptor: &DatabaseDescriptor,
    approved: &ApprovedCleanupPreview,
) -> Result<i64, MaintenanceOperationError> {
    let mut deleted = 0_i64;
    let ordered = ["触发器", "工作流", "智能体", "策略定义", "回测结果"];
    for category in ordered {
        for candidate in approved
            .candidates
            .iter()
            .filter(|candidate| candidate.category == category)
        {
            let (sql, id) = deletion_statement(descriptor, category, &candidate.id)?;
            deleted = deleted.saturating_add(
                i64::try_from(transaction.execute(sql, params![id]).map_err(failed)?)
                    .unwrap_or(i64::MAX),
            );
        }
    }
    if deleted != i64::try_from(approved.candidates.len()).unwrap_or(i64::MAX) {
        return Err(rejected("cleanup candidates changed"));
    }
    Ok(deleted)
}

fn deletion_statement<'a>(
    descriptor: &DatabaseDescriptor,
    category: &str,
    id: &'a str,
) -> Result<(&'static str, &'a str), MaintenanceOperationError> {
    let sql = match (descriptor.id.as_str(), category) {
        ("backtest-runs", "回测结果") => {
            "DELETE FROM backtest_runs WHERE id = ?1 AND status IN ('completed', 'failed', 'cancelled')"
        }
        ("strategy", "策略定义") => {
            "DELETE FROM strategy_design_definitions WHERE id = ?1 AND deleted_at IS NOT NULL AND TRIM(deleted_at) <> ''"
        }
        ("adk", "触发器") => "DELETE FROM adk_workflow_triggers WHERE id = ?1",
        ("adk", "工作流") => "DELETE FROM adk_workflows WHERE id = ?1",
        ("adk", "智能体") => "DELETE FROM adk_agents WHERE id = ?1",
        _ => return Err(rejected("cleanup candidate category is unsupported")),
    };
    Ok((sql, id.trim()))
}

fn open_read_write(path: &Path) -> Result<Connection, MaintenanceOperationError> {
    let connection = Connection::open_with_flags(
        path,
        OpenFlags::SQLITE_OPEN_READ_WRITE | OpenFlags::SQLITE_OPEN_NO_MUTEX,
    )
    .map_err(failed)?;
    connection
        .execute_batch("PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 10000;")
        .map_err(failed)?;
    connection
        .busy_timeout(Duration::from_secs(10))
        .map_err(failed)?;
    Ok(connection)
}

fn compact_connection(connection: &Connection) -> Result<(), MaintenanceOperationError> {
    connection
        .execute_batch("PRAGMA wal_checkpoint(TRUNCATE); VACUUM;")
        .map_err(failed)
}

fn ensure_available_for_backup(
    descriptor: &DatabaseDescriptor,
) -> Result<(), MaintenanceOperationError> {
    let path = Path::new(&descriptor.path);
    if !path
        .metadata()
        .map(|metadata| metadata.is_file())
        .unwrap_or(false)
    {
        return Err(rejected(format!(
            "database {} is not available for backup",
            descriptor.id
        )));
    }
    verify_backup(path)
}

fn database_is_incompatible(descriptor: &DatabaseDescriptor) -> bool {
    let Ok(connection) = open_read_write(Path::new(&descriptor.path)) else {
        return false;
    };
    validate_current(
        &connection,
        &descriptor.path,
        &descriptor.id,
        descriptor.expected_version,
    )
    .is_err()
}

fn verify_backup(path: &Path) -> Result<(), MaintenanceOperationError> {
    let metadata = path.metadata().map_err(failed)?;
    if !metadata.is_file() || metadata.len() == 0 {
        return Err(failed("backup is not a non-empty regular file"));
    }
    let connection = Connection::open_with_flags(
        path,
        OpenFlags::SQLITE_OPEN_READ_ONLY | OpenFlags::SQLITE_OPEN_NO_MUTEX,
    )
    .map_err(failed)?;
    let quick_check: String = connection
        .query_row("PRAGMA quick_check", [], |row| row.get(0))
        .map_err(failed)?;
    if quick_check != "ok" {
        return Err(failed(format!("backup quick_check returned {quick_check}")));
    }
    let foreign_key_errors: i64 = connection
        .query_row("SELECT COUNT(*) FROM pragma_foreign_key_check", [], |row| {
            row.get(0)
        })
        .map_err(failed)?;
    if foreign_key_errors != 0 {
        return Err(failed("backup contains foreign key violations"));
    }
    Ok(())
}

fn verify_marker_backup(
    marker_path: &Path,
    backup: &VerifiedBackup,
) -> Result<(), MaintenanceOperationError> {
    let backup_path = Path::new(&backup.path);
    let backup_directory = marker_path
        .parent()
        .unwrap_or_else(|| Path::new("."))
        .join("backups");
    if !backup_path.starts_with(&backup_directory) {
        return Err(failed(
            "rebuild backup is outside the managed backup directory",
        ));
    }
    verify_backup(backup_path)?;
    if file_bytes(backup_path) != backup.size_bytes || file_sha256(backup_path)? != backup.sha256 {
        return Err(failed(
            "rebuild backup size or digest does not match marker",
        ));
    }
    Ok(())
}

fn file_sha256(path: &Path) -> Result<String, MaintenanceOperationError> {
    let mut file = File::open(path).map_err(failed)?;
    let mut hasher = Sha256::new();
    let mut buffer = [0_u8; 64 * 1024];
    loop {
        let read = file.read(&mut buffer).map_err(failed)?;
        if read == 0 {
            break;
        }
        hasher.update(&buffer[..read]);
    }
    let digest = hasher.finalize();
    let mut encoded = String::with_capacity(digest.len() * 2);
    const HEX: &[u8; 16] = b"0123456789abcdef";
    for byte in digest {
        encoded.push(char::from(HEX[usize::from(byte >> 4)]));
        encoded.push(char::from(HEX[usize::from(byte & 0x0f)]));
    }
    Ok(encoded)
}

fn database_bytes(path: &Path) -> i64 {
    file_bytes(path)
        .saturating_add(file_bytes(&PathBuf::from(format!(
            "{}-wal",
            path.display()
        ))))
        .saturating_add(file_bytes(&PathBuf::from(format!(
            "{}-shm",
            path.display()
        ))))
}

fn file_bytes(path: &Path) -> i64 {
    path.metadata()
        .ok()
        .and_then(|metadata| i64::try_from(metadata.len()).ok())
        .unwrap_or(0)
}

fn remove_files(paths: &[PathBuf]) {
    for path in paths {
        let _ = fs::remove_file(path);
    }
}

fn map_lease_error(error: WriterLeaseError) -> MaintenanceOperationError {
    match error {
        WriterLeaseError::Held { .. } => MaintenanceOperationError::Conflict(error.to_string()),
        _ => MaintenanceOperationError::Failed(error.to_string()),
    }
}

fn rejected(message: impl Into<String>) -> MaintenanceOperationError {
    MaintenanceOperationError::Rejected(message.into())
}

fn failed(error: impl std::fmt::Display) -> MaintenanceOperationError {
    MaintenanceOperationError::Failed(error.to_string())
}

#[cfg(unix)]
fn harden_directory(path: &Path) -> Result<(), MaintenanceOperationError> {
    use std::os::unix::fs::PermissionsExt;
    fs::set_permissions(path, fs::Permissions::from_mode(0o700)).map_err(failed)
}

#[cfg(not(unix))]
fn harden_directory(_path: &Path) -> Result<(), MaintenanceOperationError> {
    Ok(())
}

#[cfg(unix)]
fn harden_path(path: &Path) -> Result<(), MaintenanceOperationError> {
    use std::os::unix::fs::PermissionsExt;
    fs::set_permissions(path, fs::Permissions::from_mode(0o600)).map_err(failed)
}

#[cfg(not(unix))]
fn harden_path(_path: &Path) -> Result<(), MaintenanceOperationError> {
    Ok(())
}

#[cfg(unix)]
fn harden_file(file: &File) -> Result<(), MaintenanceOperationError> {
    use std::os::unix::fs::PermissionsExt;
    file.set_permissions(fs::Permissions::from_mode(0o600))
        .map_err(failed)
}

#[cfg(not(unix))]
fn harden_file(_file: &File) -> Result<(), MaintenanceOperationError> {
    Ok(())
}

#[cfg(unix)]
fn sync_directory(path: &Path) -> Result<(), MaintenanceOperationError> {
    File::open(path)
        .and_then(|directory| directory.sync_all())
        .map_err(failed)
}

#[cfg(not(unix))]
fn sync_directory(_path: &Path) -> Result<(), MaintenanceOperationError> {
    Ok(())
}
