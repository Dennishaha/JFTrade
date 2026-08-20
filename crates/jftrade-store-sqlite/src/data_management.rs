use std::collections::BTreeSet;
use std::fs;
use std::path::{Path, PathBuf};
use std::time::Duration;

use jftrade_datamanagement::{
    CLEANUP_BACKTEST_HISTORY, CLEANUP_SOFT_DELETED, CleanableItem, CleanupCandidatePort,
    CleanupCandidateQuery, CleanupCandidateRecord, DatabaseDescriptor, DatabaseInspection,
    DatabaseOverviewPort, DatabaseStatus, StorageStats,
};
use rusqlite::{Connection, OpenFlags};
use serde::Deserialize;

use crate::schema_manifest::{current_version, validate_current};

#[derive(Clone, Debug)]
pub struct ManagedDatabaseOverviewStore {
    rebuild_marker_path: PathBuf,
}

#[derive(Clone, Debug, Default)]
pub struct ManagedDatabaseCleanupCandidateStore;

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase")]
struct RebuildMarker {
    #[serde(default)]
    database_ids: Vec<String>,
}

impl ManagedDatabaseOverviewStore {
    pub fn new(rebuild_marker_path: impl Into<PathBuf>) -> Self {
        Self {
            rebuild_marker_path: rebuild_marker_path.into(),
        }
    }
}

impl DatabaseOverviewPort for ManagedDatabaseOverviewStore {
    fn scheduled_rebuilds(&self) -> Result<BTreeSet<String>, String> {
        let contents = match fs::read(&self.rebuild_marker_path) {
            Ok(contents) => contents,
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
                return Ok(BTreeSet::new());
            }
            Err(error) => return Err(error.to_string()),
        };
        let marker: RebuildMarker = serde_json::from_slice(&contents)
            .map_err(|error| format!("decode database rebuild marker: {error}"))?;
        Ok(marker
            .database_ids
            .into_iter()
            .map(|value| value.trim().to_owned())
            .filter(|value| !value.is_empty())
            .collect())
    }

    fn inspect(&self, descriptor: &DatabaseDescriptor, summary_only: bool) -> DatabaseInspection {
        inspect_database(descriptor, summary_only)
    }
}

impl CleanupCandidatePort for ManagedDatabaseCleanupCandidateStore {
    fn candidates(
        &self,
        descriptor: &DatabaseDescriptor,
        query: &CleanupCandidateQuery,
    ) -> Result<Vec<CleanupCandidateRecord>, String> {
        let connection = open_read_only(Path::new(&descriptor.path))
            .map_err(|error| format!("open cleanup candidate database: {error}"))?;
        match query.kind.as_str() {
            CLEANUP_BACKTEST_HISTORY => backtest_candidates(&connection, query),
            CLEANUP_SOFT_DELETED if descriptor.id == "strategy" => query_candidates(
                &connection,
                "SELECT id, LENGTH(script) + LENGTH(visual_model_json) FROM strategy_design_definitions WHERE deleted_at IS NOT NULL AND TRIM(deleted_at) <> '' ORDER BY id",
                "策略定义",
            ),
            CLEANUP_SOFT_DELETED if descriptor.id == "adk" => {
                let mut candidates = Vec::new();
                for (sql, category) in [
                    (
                        "SELECT id, LENGTH(payload_json) FROM adk_agents WHERE COALESCE(json_extract(payload_json, '$.deletedAt'), '') <> '' ORDER BY id",
                        "智能体",
                    ),
                    (
                        "SELECT id, LENGTH(payload_json) FROM adk_workflows WHERE COALESCE(json_extract(payload_json, '$.deletedAt'), '') <> '' ORDER BY id",
                        "工作流",
                    ),
                    (
                        "SELECT id, LENGTH(payload_json) FROM adk_workflow_triggers WHERE COALESCE(json_extract(payload_json, '$.deletedAt'), '') <> '' OR workflow_id IN (SELECT id FROM adk_workflows WHERE COALESCE(json_extract(payload_json, '$.deletedAt'), '') <> '') ORDER BY id",
                        "触发器",
                    ),
                ] {
                    candidates.extend(query_candidates(&connection, sql, category)?);
                }
                Ok(candidates)
            }
            _ => Err(format!(
                "cleanup kind {:?} is unsupported for database {:?}",
                query.kind, descriptor.id
            )),
        }
    }
}

fn backtest_candidates(
    connection: &Connection,
    query: &CleanupCandidateQuery,
) -> Result<Vec<CleanupCandidateRecord>, String> {
    let cutoff = query
        .cutoff
        .ok_or_else(|| "backtest cleanup cutoff is required".to_owned())?
        .into_inner();
    let mut statement = connection
        .prepare(
            "SELECT id, updated_at, LENGTH(request_json) + LENGTH(result_json) FROM backtest_runs WHERE status IN ('completed', 'failed', 'cancelled') ORDER BY updated_at DESC, id ASC",
        )
        .map_err(|error| format!("prepare backtest cleanup candidates: {error}"))?;
    let mut rows = statement
        .query([])
        .map_err(|error| format!("query backtest cleanup candidates: {error}"))?;
    let mut candidates = Vec::new();
    let mut index = 0_i32;
    while let Some(row) = rows
        .next()
        .map_err(|error| format!("read backtest cleanup candidate: {error}"))?
    {
        let id: String = row
            .get(0)
            .map_err(|error| format!("decode backtest cleanup id: {error}"))?;
        let updated_at: String = row
            .get(1)
            .map_err(|error| format!("decode backtest cleanup timestamp: {error}"))?;
        let estimated_bytes: i64 = row
            .get(2)
            .map_err(|error| format!("decode backtest cleanup size: {error}"))?;
        let is_old = updated_at
            .parse::<jftrade_kernel::WireTimestamp>()
            .map(|timestamp| timestamp.into_inner() < cutoff)
            .unwrap_or(false);
        if index >= query.keep_latest && is_old {
            candidates.push(CleanupCandidateRecord {
                id,
                category: "回测结果".to_owned(),
                estimated_bytes,
            });
        }
        index = index.saturating_add(1);
    }
    Ok(candidates)
}

fn query_candidates(
    connection: &Connection,
    sql: &str,
    category: &str,
) -> Result<Vec<CleanupCandidateRecord>, String> {
    let mut statement = connection
        .prepare(sql)
        .map_err(|error| format!("prepare cleanup candidates: {error}"))?;
    let mut rows = statement
        .query([])
        .map_err(|error| format!("query cleanup candidates: {error}"))?;
    let mut candidates = Vec::new();
    while let Some(row) = rows
        .next()
        .map_err(|error| format!("read cleanup candidate: {error}"))?
    {
        candidates.push(CleanupCandidateRecord {
            id: row
                .get(0)
                .map_err(|error| format!("decode cleanup candidate id: {error}"))?,
            category: category.to_owned(),
            estimated_bytes: row
                .get(1)
                .map_err(|error| format!("decode cleanup candidate size: {error}"))?,
        });
    }
    Ok(candidates)
}

fn inspect_database(descriptor: &DatabaseDescriptor, summary_only: bool) -> DatabaseInspection {
    let path = Path::new(&descriptor.path);
    let metadata = match path.metadata() {
        Ok(metadata) => metadata,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
            return inspection(DatabaseStatus::Missing, None, String::new());
        }
        Err(error) => {
            return inspection(DatabaseStatus::Unavailable, None, error.to_string());
        }
    };
    if !metadata.is_file() {
        return inspection(
            DatabaseStatus::Unavailable,
            None,
            "database path is not a regular file".to_owned(),
        );
    }
    let connection = match open_read_only(path) {
        Ok(connection) => connection,
        Err(error) => {
            return inspection(DatabaseStatus::Unavailable, None, error.to_string());
        }
    };
    let version = current_version(&connection, &descriptor.id);
    let mut result = match validate_current(
        &connection,
        &descriptor.path,
        &descriptor.id,
        descriptor.expected_version,
    ) {
        Ok(()) => inspection(DatabaseStatus::Ready, version, String::new()),
        Err(error) if error.is_incompatible() => {
            inspection(DatabaseStatus::Incompatible, version, error.to_string())
        }
        Err(error) => inspection(DatabaseStatus::Unavailable, version, error.to_string()),
    };
    if summary_only {
        return result;
    }
    result.storage = storage_stats(path, &connection, result.status == DatabaseStatus::Ready);
    if result.status == DatabaseStatus::Ready {
        result.cleanable = cleanable_items(&connection, &descriptor.id);
    }
    result
}

fn inspection(
    status: DatabaseStatus,
    current_version: Option<i64>,
    error: String,
) -> DatabaseInspection {
    DatabaseInspection {
        status,
        current_version,
        error,
        storage: StorageStats::default(),
        cleanable: None,
    }
}

fn open_read_only(path: &Path) -> Result<Connection, rusqlite::Error> {
    let connection = Connection::open_with_flags(
        path,
        OpenFlags::SQLITE_OPEN_READ_ONLY | OpenFlags::SQLITE_OPEN_NO_MUTEX,
    )?;
    connection.execute_batch("PRAGMA foreign_keys = ON; PRAGMA query_only = ON;")?;
    connection.busy_timeout(Duration::from_secs(10))?;
    Ok(connection)
}

fn storage_stats(path: &Path, connection: &Connection, ready: bool) -> StorageStats {
    let main_bytes = file_size(path);
    let wal_bytes = file_size(&PathBuf::from(format!("{}-wal", path.display())));
    let shm_bytes = file_size(&PathBuf::from(format!("{}-shm", path.display())));
    let mut stats = StorageStats {
        main_bytes,
        wal_bytes,
        shm_bytes,
        total_bytes: main_bytes + wal_bytes + shm_bytes,
        reclaimable_bytes: wal_bytes,
        ..StorageStats::default()
    };
    if !ready {
        return stats;
    }
    match connection.query_row(
        "SELECT page_size, freelist_count FROM pragma_page_size(), pragma_freelist_count()",
        [],
        |row| Ok((row.get::<_, i64>(0)?, row.get::<_, i64>(1)?)),
    ) {
        Ok((page_size, free_pages)) => {
            stats.free_page_bytes = page_size * free_pages;
            stats.reclaimable_bytes += stats.free_page_bytes;
        }
        Err(error) => stats.error = error.to_string(),
    }
    stats
}

fn cleanable_items(connection: &Connection, database_id: &str) -> Option<Vec<CleanableItem>> {
    match database_id {
        "strategy" => query_cleanable(
            connection,
            "soft-deleted",
            "已删除策略",
            "SELECT COUNT(*), COALESCE(SUM(LENGTH(script) + LENGTH(visual_model_json)), 0) FROM strategy_design_definitions WHERE deleted_at IS NOT NULL AND TRIM(deleted_at) <> ''",
        ),
        "backtest-runs" => query_cleanable(
            connection,
            "backtest-history",
            "已结束回测",
            "SELECT COUNT(*), COALESCE(SUM(LENGTH(request_json) + LENGTH(result_json)), 0) FROM backtest_runs WHERE status IN ('completed', 'failed', 'cancelled')",
        ),
        "adk" => {
            let mut items = Vec::with_capacity(3);
            for (label, table) in [
                ("已删除智能体", "adk_agents"),
                ("已删除工作流", "adk_workflows"),
                ("已删除触发器", "adk_workflow_triggers"),
            ] {
                let query = format!(
                    "SELECT COUNT(*), COALESCE(SUM(LENGTH(payload_json)), 0) FROM {table} WHERE COALESCE(json_extract(payload_json, '$.deletedAt'), '') <> ''"
                );
                items.extend(query_cleanable(connection, "soft-deleted", label, &query)?);
            }
            Some(items)
        }
        _ => None,
    }
}

fn query_cleanable(
    connection: &Connection,
    kind: &str,
    label: &str,
    query: &str,
) -> Option<Vec<CleanableItem>> {
    connection
        .query_row(query, [], |row| {
            Ok(CleanableItem {
                kind: kind.to_owned(),
                label: label.to_owned(),
                count: row.get(0)?,
                estimated_bytes: row.get(1)?,
            })
        })
        .ok()
        .map(|item| vec![item])
}

fn file_size(path: &Path) -> i64 {
    path.metadata()
        .ok()
        .filter(|metadata| metadata.is_file())
        .and_then(|metadata| i64::try_from(metadata.len()).ok())
        .unwrap_or(0)
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use jftrade_datamanagement::{
        DATABASE_BACKTEST, ManagedDatabasePaths, OverviewRequest, OverviewService,
        managed_database_descriptors,
    };
    use tempfile::tempdir;

    use super::*;

    #[test]
    fn missing_database_and_marker_are_read_without_creating_files() {
        let root = tempdir().expect("tempdir");
        let database_path = root.path().join("backtest.db");
        let paths = ManagedDatabasePaths::new([(
            DATABASE_BACKTEST,
            database_path.to_string_lossy().into_owned(),
        )]);
        let store = Arc::new(ManagedDatabaseOverviewStore::new(
            root.path().join("database-rebuild.json"),
        ));
        let response = OverviewService::new(managed_database_descriptors(&paths), store)
            .overview(
                OverviewRequest {
                    summary_only: true,
                    database_id: DATABASE_BACKTEST.to_owned(),
                },
                "2026-08-20T00:00:00Z".to_owned(),
            )
            .expect("overview");
        assert_eq!(response.databases[0].status, "missing");
        assert!(!database_path.exists());
    }
}
