//! Startup application of a pending database rebuild marker.
//!
//! Scheduling a rebuild writes `database-rebuild.json` next to the settings
//! file together with a verified snapshot of every selected database. On the
//! next start the engine verifies those snapshots before deleting any source
//! file, recreates the schemas through the normal initialization path, and
//! only then clears the marker. Nothing is deleted while any snapshot is
//! unverifiable, and the marker survives every failure.

use std::collections::{BTreeMap, BTreeSet};
use std::fs;
use std::path::{Path, PathBuf};

use jftrade_datamanagement::DatabaseDescriptor;
use jftrade_store_sqlite::{validate_current, verify_managed_rebuild_backup};
use rusqlite::{Connection, OpenFlags};
use serde::Deserialize;

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
struct PendingRebuildMarker {
    database_ids: Vec<String>,
    backups: Vec<PendingRebuildBackup>,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
struct PendingRebuildBackup {
    database_id: String,
    path: String,
    size_bytes: i64,
    sha256: String,
}

/// Delete the databases a verified pending marker selected.
///
/// An empty result means no rebuild is pending. Every selected database must
/// have exactly one verified snapshot before the first source file is removed.
pub(super) fn apply_pending_rebuild(
    descriptors: &[DatabaseDescriptor],
    marker_path: &Path,
) -> Result<Vec<String>, String> {
    let Some(marker) = read_marker(marker_path)? else {
        return Ok(Vec::new());
    };
    let ids = normalize_ids(&marker.database_ids);
    if ids.is_empty() {
        return Ok(Vec::new());
    }
    let scheduled = ids.iter().cloned().collect::<BTreeSet<_>>();
    let mut backups = BTreeMap::new();
    for backup in &marker.backups {
        if backups.insert(backup.database_id.clone(), backup).is_some() {
            return Err(format!(
                "rebuild marker contains duplicate backup for {:?}",
                backup.database_id
            ));
        }
        if !scheduled.contains(&backup.database_id) {
            return Err(format!(
                "rebuild marker contains backup for unscheduled database {:?}",
                backup.database_id
            ));
        }
    }
    let by_id = descriptors
        .iter()
        .map(|descriptor| (descriptor.id.as_str(), descriptor))
        .collect::<BTreeMap<_, _>>();
    let backup_directory = managed_backup_directory(marker_path);
    for id in &ids {
        let descriptor = by_id
            .get(id.as_str())
            .ok_or_else(|| format!("rebuild marker contains unknown database id {id:?}"))?;
        let backup = backups
            .get(id)
            .ok_or_else(|| format!("rebuild marker for {id} has no verified backup"))?;
        verify_managed_rebuild_backup(
            &backup_directory,
            &descriptor.id,
            &backup.path,
            backup.size_bytes,
            &backup.sha256,
        )
        .map_err(|error| format!("verify rebuild backup for {id}: {error}"))?;
    }
    for id in &ids {
        let descriptor = by_id
            .get(id.as_str())
            .ok_or_else(|| format!("rebuild marker contains unknown database id {id:?}"))?;
        for suffix in ["", "-wal", "-shm"] {
            let path = PathBuf::from(format!("{}{suffix}", descriptor.path));
            match fs::remove_file(&path) {
                Ok(()) => {}
                Err(error) if error.kind() == std::io::ErrorKind::NotFound => {}
                Err(error) => {
                    return Err(format!(
                        "remove {id} database file {}: {error}",
                        path.display()
                    ));
                }
            }
        }
    }
    Ok(ids)
}

/// Clear the marker once every rebuilt database initialized successfully.
pub(super) fn complete_pending_rebuild(
    descriptors: &[DatabaseDescriptor],
    marker_path: &Path,
    applied: &[String],
) -> Result<(), String> {
    if applied.is_empty() {
        return Ok(());
    }
    for id in applied {
        let descriptor = descriptors
            .iter()
            .find(|descriptor| &descriptor.id == id)
            .ok_or_else(|| format!("rebuilt database {id} has no descriptor"))?;
        let connection = Connection::open_with_flags(
            &descriptor.path,
            OpenFlags::SQLITE_OPEN_READ_ONLY | OpenFlags::SQLITE_OPEN_NO_MUTEX,
        )
        .map_err(|error| {
            format!("rebuilt database {id} did not initialize successfully: {error}")
        })?;
        validate_current(
            &connection,
            &descriptor.path,
            &descriptor.id,
            descriptor.expected_version,
        )
        .map_err(|error| {
            format!("rebuilt database {id} did not initialize successfully: {error}")
        })?;
    }
    fs::remove_file(marker_path).map_err(|error| {
        format!(
            "remove completed rebuild marker {}: {error}",
            marker_path.display()
        )
    })
}

fn read_marker(marker_path: &Path) -> Result<Option<PendingRebuildMarker>, String> {
    match fs::read(marker_path) {
        Ok(bytes) => serde_json::from_slice(&bytes)
            .map(Some)
            .map_err(|error| format!("decode database rebuild marker: {error}")),
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(None),
        Err(error) => Err(format!(
            "read database rebuild marker {}: {error}",
            marker_path.display()
        )),
    }
}

fn normalize_ids(values: &[String]) -> Vec<String> {
    values
        .iter()
        .map(|value| value.trim().to_owned())
        .filter(|value| !value.is_empty())
        .collect::<BTreeSet<_>>()
        .into_iter()
        .collect()
}

fn managed_backup_directory(marker_path: &Path) -> PathBuf {
    marker_path
        .parent()
        .unwrap_or_else(|| Path::new("."))
        .join("backups")
}
