use std::collections::{BTreeMap, BTreeSet, HashSet};
use std::path::{Path, PathBuf};
use std::sync::{Mutex, MutexGuard};
use std::time::Duration;

use jftrade_owner_lock::{OwnerDiagnostic, WriterLease, WriterLeaseError};
use rusqlite::{
    Connection, ErrorCode, OpenFlags, OptionalExtension, TransactionBehavior, params,
    params_from_iter,
};
use serde::{Deserialize, Serialize};
use serde_json::json;
use thiserror::Error;
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

use crate::schema_manifest::{SchemaManifestError, validate_current};

const WATCHLIST_COMPONENT: &str = "watchlist";
const WATCHLIST_SCHEMA_VERSION: i64 = 1;
pub const WATCHLIST_TEST_CUTOVER_PROFILE: &str = "cutover-test-only.v1";
pub const WATCHLIST_PRODUCTION_PROFILE: &str = "production.v1";

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GroupRef {
    pub group_id: String,
    pub name: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Memberships {
    pub instrument_id: String,
    pub revision: i64,
    pub groups: Vec<GroupRef>,
}

pub fn group_name_key(value: &str) -> String {
    value.trim().to_lowercase()
}

pub fn normalize_instrument_id(value: &str) -> Result<String, WatchlistStoreError> {
    let normalized = value.trim().to_uppercase();
    let Some((market, symbol)) = normalized.split_once('.') else {
        return Err(WatchlistStoreError::InvalidInstrument(value.to_owned()));
    };
    let canonical_market = match market {
        "US" | "HK" | "SH" | "SZ" => market,
        "CNSH" => "SH",
        "CNSZ" => "SZ",
        _ => return Err(WatchlistStoreError::InvalidInstrument(value.to_owned())),
    };
    if market.is_empty()
        || symbol.is_empty()
        || symbol.contains('.')
        || !symbol
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'-' | b'_'))
    {
        return Err(WatchlistStoreError::InvalidInstrument(value.to_owned()));
    }
    Ok(format!("{canonical_market}.{symbol}"))
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredGroup {
    pub group_id: String,
    pub name: String,
    pub is_default: bool,
    pub protected: bool,
    pub revision: i64,
    pub item_count: usize,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredSource {
    pub source_id: String,
    pub broker: String,
    pub display_name: String,
    pub status: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub error: Option<String>,
    pub updated_at: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredRemoteGroup {
    pub source_id: String,
    pub remote_group_id: String,
    pub name: String,
    pub group_type: String,
    pub ambiguous: bool,
    pub member_count: usize,
    pub remote_hash: String,
    pub observed_at: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredBinding {
    pub binding_id: String,
    pub source_id: String,
    pub remote_group_id: String,
    pub remote_name: String,
    pub local_group_id: String,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredImportPreview {
    pub preview_id: String,
    pub source_id: String,
    pub remote_group_id: String,
    pub remote_group_name: String,
    pub local_group_id: String,
    pub new_group_name: String,
    pub remote_hash: String,
    pub local_group_revision: i64,
    pub added_json: String,
    pub unchanged_json: String,
    pub local_only_json: String,
    pub status: String,
    pub created_at: String,
    pub expires_at: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredImportRun {
    pub run_id: String,
    pub preview_id: String,
    pub source_id: String,
    pub remote_group_id: String,
    pub remote_group_name: String,
    pub local_group_id: String,
    pub status: String,
    pub added_count: usize,
    pub removed_count: usize,
    pub unchanged_count: usize,
    pub remote_hash: String,
    pub created_at: String,
    pub completed_at: String,
}

#[derive(Clone, Debug)]
struct ItemListRow {
    instrument_id: String,
    market: String,
    symbol: String,
    name: String,
    instrument_type: String,
    revision: i64,
    last_imported_at: Option<String>,
}

#[derive(Debug, Error)]
pub enum WatchlistStoreError {
    #[error("watchlist database path is required")]
    EmptyPath,
    #[error("unsupported watchlist writer profile: {0}")]
    UnsupportedProfile(String),
    #[error("watchlist database is not an existing regular file: {0}")]
    NotRegularFile(String),
    #[error(transparent)]
    WriterLease(#[from] WriterLeaseError),
    #[error("open watchlist database: {0}")]
    Open(#[source] rusqlite::Error),
    #[error("configure watchlist database: {0}")]
    Configure(#[source] rusqlite::Error),
    #[error(transparent)]
    Schema(#[from] SchemaManifestError),
    #[error("watchlist database lock is unavailable")]
    LockUnavailable,
    #[error("query watchlist database: {0}")]
    Query(#[source] rusqlite::Error),
    #[error("watchlist resource not found")]
    NotFound,
    #[error("watchlist state conflict")]
    Conflict,
    #[error("protected watchlist group cannot be deleted or renamed")]
    ProtectedGroup,
    #[error("invalid watchlist instrument: {0}")]
    InvalidInstrument(String),
    #[error("invalid watchlist request: {0}")]
    Validation(String),
    #[error("incompatible watchlist database: {0}")]
    Incompatible(String),
}

pub struct WatchlistStore {
    path: PathBuf,
    connection: Mutex<Connection>,
    _writer_lease: WriterLease,
}

impl std::fmt::Debug for WatchlistStore {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("WatchlistStore")
            .field("path", &self.path)
            .finish()
    }
}

impl WatchlistStore {
    pub fn open(path: impl AsRef<Path>) -> Result<Self, WatchlistStoreError> {
        Self::open_existing(path, WATCHLIST_PRODUCTION_PROFILE)
    }

    pub fn open_existing(
        path: impl AsRef<Path>,
        profile: &str,
    ) -> Result<Self, WatchlistStoreError> {
        let path = path.as_ref();
        if path.as_os_str().is_empty() {
            return Err(WatchlistStoreError::EmptyPath);
        }
        if profile != WATCHLIST_TEST_CUTOVER_PROFILE && profile != WATCHLIST_PRODUCTION_PROFILE {
            return Err(WatchlistStoreError::UnsupportedProfile(profile.to_owned()));
        }
        if !path
            .metadata()
            .map(|metadata| metadata.is_file())
            .unwrap_or(false)
        {
            return Err(WatchlistStoreError::NotRegularFile(
                path.display().to_string(),
            ));
        }

        let writer_lease = WriterLease::acquire(path, &OwnerDiagnostic::current("rust", profile))?;

        let connection = Connection::open_with_flags(
            path,
            OpenFlags::SQLITE_OPEN_READ_WRITE | OpenFlags::SQLITE_OPEN_NO_MUTEX,
        )
        .map_err(WatchlistStoreError::Open)?;

        connection
            .busy_timeout(Duration::from_secs(10))
            .map_err(WatchlistStoreError::Configure)?;

        connection
            .execute_batch("PRAGMA foreign_keys = ON;")
            .map_err(WatchlistStoreError::Configure)?;

        validate_current(
            &connection,
            &path.display().to_string(),
            WATCHLIST_COMPONENT,
            WATCHLIST_SCHEMA_VERSION,
        )?;

        Ok(Self {
            path: path.to_path_buf(),
            connection: Mutex::new(connection),
            _writer_lease: writer_lease,
        })
    }

    pub fn path(&self) -> &Path {
        &self.path
    }

    fn lock(&self) -> Result<MutexGuard<'_, Connection>, WatchlistStoreError> {
        self.connection
            .lock()
            .map_err(|_| WatchlistStoreError::LockUnavailable)
    }

    pub fn list_groups(&self) -> Result<Vec<StoredGroup>, WatchlistStoreError> {
        let connection = self.lock()?;
        let mut statement = connection
            .prepare(
                "SELECT g.group_id, g.name, g.is_default, g.protected, g.revision,
                        COUNT(m.instrument_id) AS item_count, g.created_at, g.updated_at
                 FROM watchlist_groups g
                 LEFT JOIN watchlist_memberships m ON m.group_id = g.group_id
                 GROUP BY g.group_id
                 ORDER BY g.is_default DESC, g.created_at, g.group_id",
            )
            .map_err(WatchlistStoreError::Query)?;

        let rows = statement
            .query_map([], |row| {
                Ok(StoredGroup {
                    group_id: row.get(0)?,
                    name: row.get(1)?,
                    is_default: row.get::<_, i64>(2)? != 0,
                    protected: row.get::<_, i64>(3)? != 0,
                    revision: row.get(4)?,
                    item_count: row.get::<_, i64>(5)? as usize,
                    created_at: row.get(6)?,
                    updated_at: row.get(7)?,
                })
            })
            .map_err(WatchlistStoreError::Query)?;

        let mut groups = Vec::new();
        for row in rows {
            groups.push(row.map_err(WatchlistStoreError::Query)?);
        }
        Ok(groups)
    }

    pub fn get_group(&self, group_id: &str) -> Result<StoredGroup, WatchlistStoreError> {
        let connection = self.lock()?;
        get_group_query(&connection, group_id)
    }

    pub fn create_group(
        &self,
        name: &str,
        timestamp: &str,
    ) -> Result<StoredGroup, WatchlistStoreError> {
        validate_rfc3339_timestamp(timestamp)?;
        let name = name.trim();
        if name.is_empty() {
            return Err(WatchlistStoreError::Validation(
                "group name is required".to_owned(),
            ));
        }
        if name.chars().count() > 80 {
            return Err(WatchlistStoreError::Validation(
                "group name must not exceed 80 characters".to_owned(),
            ));
        }

        let key = group_name_key(name);
        let group_id = format!("wlgrp_{}", generate_id());
        let connection = self.lock()?;

        let result = connection.execute(
            "INSERT INTO watchlist_groups
                (group_id, name, name_key, is_default, protected, revision, created_at, updated_at)
             VALUES (?1, ?2, ?3, 0, 0, 1, ?4, ?4)",
            params![group_id, name, key, timestamp],
        );

        match result {
            Ok(_) => Ok(StoredGroup {
                group_id,
                name: name.to_owned(),
                is_default: false,
                protected: false,
                revision: 1,
                item_count: 0,
                created_at: timestamp.to_owned(),
                updated_at: timestamp.to_owned(),
            }),
            Err(error) if is_unique_constraint(&error) => Err(WatchlistStoreError::Conflict),
            Err(error) => Err(WatchlistStoreError::Query(error)),
        }
    }

    pub fn update_group(
        &self,
        group_id: &str,
        name: &str,
        expected_revision: i64,
        timestamp: &str,
    ) -> Result<StoredGroup, WatchlistStoreError> {
        validate_rfc3339_timestamp(timestamp)?;
        let name = name.trim();
        if name.is_empty() {
            return Err(WatchlistStoreError::Validation(
                "group name is required".to_owned(),
            ));
        }
        if name.chars().count() > 80 {
            return Err(WatchlistStoreError::Validation(
                "group name must not exceed 80 characters".to_owned(),
            ));
        }

        let key = group_name_key(name);
        let connection = self.lock()?;

        let current = get_group_query(&connection, group_id)?;
        if current.protected {
            return Err(WatchlistStoreError::ProtectedGroup);
        }
        if current.revision != expected_revision {
            return Err(WatchlistStoreError::Conflict);
        }

        let result = connection.execute(
            "UPDATE watchlist_groups
             SET name = ?1, name_key = ?2, revision = revision + 1, updated_at = ?3
             WHERE group_id = ?4 AND revision = ?5",
            params![name, key, timestamp, group_id, expected_revision],
        );

        match result {
            Ok(affected) if affected > 0 => get_group_query(&connection, group_id),
            Ok(_) => Err(WatchlistStoreError::Conflict),
            Err(error) if is_unique_constraint(&error) => Err(WatchlistStoreError::Conflict),
            Err(error) => Err(WatchlistStoreError::Query(error)),
        }
    }

    pub fn delete_group(&self, group_id: &str) -> Result<(), WatchlistStoreError> {
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(WatchlistStoreError::Query)?;

        let current = get_group_query(&transaction, group_id)?;
        if current.protected || current.is_default {
            return Err(WatchlistStoreError::ProtectedGroup);
        }

        let mut statement = transaction
            .prepare("SELECT instrument_id FROM watchlist_memberships WHERE group_id = ?1")
            .map_err(WatchlistStoreError::Query)?;
        let instrument_ids: Vec<String> = statement
            .query_map(params![group_id], |row| row.get(0))
            .map_err(WatchlistStoreError::Query)?
            .collect::<Result<Vec<_>, _>>()
            .map_err(WatchlistStoreError::Query)?;
        drop(statement);

        transaction
            .execute(
                "DELETE FROM watchlist_membership_origins WHERE group_id = ?1",
                params![group_id],
            )
            .map_err(WatchlistStoreError::Query)?;

        transaction
            .execute(
                "DELETE FROM watchlist_memberships WHERE group_id = ?1",
                params![group_id],
            )
            .map_err(WatchlistStoreError::Query)?;

        transaction
            .execute(
                "DELETE FROM watchlist_bindings WHERE local_group_id = ?1",
                params![group_id],
            )
            .map_err(WatchlistStoreError::Query)?;

        transaction
            .execute(
                "DELETE FROM watchlist_groups WHERE group_id = ?1",
                params![group_id],
            )
            .map_err(WatchlistStoreError::Query)?;

        let now = now_rfc3339();
        for instrument_id in instrument_ids {
            transaction
                .execute(
                    "UPDATE watchlist_instruments
                     SET membership_revision = membership_revision + 1, updated_at = ?1
                     WHERE instrument_id = ?2",
                    params![now, instrument_id],
                )
                .map_err(WatchlistStoreError::Query)?;
        }

        transaction.commit().map_err(WatchlistStoreError::Query)?;
        Ok(())
    }

    pub fn delete_binding(&self, binding_id: &str) -> Result<(), WatchlistStoreError> {
        let connection = self.lock()?;
        let affected = connection
            .execute(
                "DELETE FROM watchlist_bindings WHERE binding_id = ?1",
                params![binding_id],
            )
            .map_err(WatchlistStoreError::Query)?;
        if affected == 0 {
            return Err(WatchlistStoreError::NotFound);
        }
        Ok(())
    }

    pub fn get_memberships(&self, instrument_id: &str) -> Result<Memberships, WatchlistStoreError> {
        let canonical_id = normalize_instrument_id(instrument_id)?;
        let connection = self.lock()?;
        get_memberships_query(&connection, &canonical_id)
    }

    pub fn replace_memberships(
        &self,
        instrument_id: &str,
        group_ids: &[String],
        new_group_names: &[String],
        expected_revision: i64,
        timestamp: &str,
    ) -> Result<Memberships, WatchlistStoreError> {
        validate_rfc3339_timestamp(timestamp)?;
        let canonical_id = normalize_instrument_id(instrument_id)?;

        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(WatchlistStoreError::Query)?;

        let current_revision: Option<i64> = transaction
            .query_row(
                "SELECT membership_revision FROM watchlist_instruments WHERE instrument_id = ?1",
                params![canonical_id],
                |row| row.get(0),
            )
            .optional()
            .map_err(WatchlistStoreError::Query)?;

        match current_revision {
            Some(rev) => {
                if rev != expected_revision {
                    return Err(WatchlistStoreError::Conflict);
                }
            }
            None => {
                if expected_revision != 0 {
                    return Err(WatchlistStoreError::Conflict);
                }
                let (market, symbol) = canonical_id
                    .split_once('.')
                    .ok_or_else(|| WatchlistStoreError::InvalidInstrument(canonical_id.clone()))?;
                transaction
                    .execute(
                        "INSERT INTO watchlist_instruments
                            (instrument_id, market, symbol, name, instrument_type, membership_revision, created_at, updated_at)
                         VALUES (?1, ?2, ?3, '', '', 0, ?4, ?4)",
                        params![canonical_id, market, symbol, timestamp],
                    )
                    .map_err(WatchlistStoreError::Query)?;
            }
        }

        let mut desired_groups = BTreeSet::new();
        for group_id in group_ids {
            let exists: Option<i64> = transaction
                .query_row(
                    "SELECT 1 FROM watchlist_groups WHERE group_id = ?1",
                    params![group_id],
                    |row| row.get(0),
                )
                .optional()
                .map_err(WatchlistStoreError::Query)?;
            if exists.is_none() {
                return Err(WatchlistStoreError::NotFound);
            }
            desired_groups.insert(group_id.clone());
        }

        for name in new_group_names {
            let trimmed = name.trim();
            if trimmed.is_empty() {
                continue;
            }
            let key = group_name_key(trimmed);
            let existing: Option<String> = transaction
                .query_row(
                    "SELECT group_id FROM watchlist_groups WHERE name_key = ?1",
                    params![key],
                    |row| row.get(0),
                )
                .optional()
                .map_err(WatchlistStoreError::Query)?;

            let gid = match existing {
                Some(gid) => gid,
                None => {
                    let new_id = format!("wlgrp_{}", generate_id());
                    transaction
                        .execute(
                            "INSERT INTO watchlist_groups
                                (group_id, name, name_key, is_default, protected, revision, created_at, updated_at)
                             VALUES (?1, ?2, ?3, 0, 0, 1, ?4, ?4)",
                            params![new_id, trimmed, key, timestamp],
                        )
                        .map_err(WatchlistStoreError::Query)?;
                    new_id
                }
            };
            desired_groups.insert(gid);
        }

        let mut statement = transaction
            .prepare("SELECT group_id FROM watchlist_memberships WHERE instrument_id = ?1")
            .map_err(WatchlistStoreError::Query)?;
        let current_groups: HashSet<String> = statement
            .query_map(params![canonical_id], |row| row.get(0))
            .map_err(WatchlistStoreError::Query)?
            .collect::<Result<HashSet<_>, _>>()
            .map_err(WatchlistStoreError::Query)?;
        drop(statement);

        let mut diff_applied = false;
        for group_id in &current_groups {
            if !desired_groups.contains(group_id) {
                transaction
                    .execute(
                        "DELETE FROM watchlist_memberships WHERE group_id = ?1 AND instrument_id = ?2",
                        params![group_id, canonical_id],
                    )
                    .map_err(WatchlistStoreError::Query)?;
                transaction
                    .execute(
                        "DELETE FROM watchlist_membership_origins WHERE group_id = ?1 AND instrument_id = ?2",
                        params![group_id, canonical_id],
                    )
                    .map_err(WatchlistStoreError::Query)?;
                diff_applied = true;
            }
        }

        for group_id in &desired_groups {
            if !current_groups.contains(group_id) {
                transaction
                    .execute(
                        "INSERT INTO watchlist_memberships (group_id, instrument_id, created_at)
                         VALUES (?1, ?2, ?3)",
                        params![group_id, canonical_id, timestamp],
                    )
                    .map_err(WatchlistStoreError::Query)?;
                diff_applied = true;
            }
        }

        if diff_applied {
            transaction
                .execute(
                    "UPDATE watchlist_instruments
                     SET membership_revision = membership_revision + 1, updated_at = ?1
                     WHERE instrument_id = ?2",
                    params![timestamp, canonical_id],
                )
                .map_err(WatchlistStoreError::Query)?;
        }

        let memberships = get_memberships_query(&transaction, &canonical_id)?;
        transaction.commit().map_err(WatchlistStoreError::Query)?;
        Ok(memberships)
    }

    pub fn create_import_preview(
        &self,
        source_id: &str,
        remote_group_id: &str,
        local_group_id: Option<&str>,
        new_group_name: Option<&str>,
        timestamp: &str,
    ) -> Result<StoredImportPreview, WatchlistStoreError> {
        validate_rfc3339_timestamp(timestamp)?;
        let connection = self.lock()?;

        let remote_info: Option<(String, String)> = connection
            .query_row(
                "SELECT name, remote_hash FROM watchlist_remote_groups
                 WHERE source_id = ?1 AND remote_group_id = ?2",
                params![source_id, remote_group_id],
                |row| Ok((row.get(0)?, row.get(1)?)),
            )
            .optional()
            .map_err(WatchlistStoreError::Query)?;

        let (remote_group_name, remote_hash) = remote_info.unwrap_or_else(|| {
            (
                remote_group_id.to_owned(),
                format!("hash_{}", generate_id()),
            )
        });

        let local_group_id = local_group_id.unwrap_or_default();
        let new_group_name = new_group_name.unwrap_or_default();
        let local_group_revision = if !local_group_id.is_empty() {
            connection
                .query_row(
                    "SELECT revision FROM watchlist_groups WHERE group_id = ?1",
                    params![local_group_id],
                    |row| row.get::<_, i64>(0),
                )
                .optional()
                .map_err(WatchlistStoreError::Query)?
                .unwrap_or(0)
        } else {
            0
        };

        let preview_id = format!("wlprev_{}", generate_id());
        let expires_at = timestamp.to_owned();

        connection
            .execute(
                "INSERT INTO watchlist_import_previews
                    (preview_id, source_id, remote_group_id, remote_group_name,
                     local_group_id, new_group_name, remote_hash, local_group_revision,
                     added_json, unchanged_json, local_only_json, status, created_at, expires_at)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, '[]', '[]', '[]', 'pending', ?9, ?10)",
                params![
                    preview_id,
                    source_id,
                    remote_group_id,
                    remote_group_name,
                    local_group_id,
                    new_group_name,
                    remote_hash,
                    local_group_revision,
                    timestamp,
                    expires_at,
                ],
            )
            .map_err(WatchlistStoreError::Query)?;

        Ok(StoredImportPreview {
            preview_id,
            source_id: source_id.to_owned(),
            remote_group_id: remote_group_id.to_owned(),
            remote_group_name,
            local_group_id: local_group_id.to_owned(),
            new_group_name: new_group_name.to_owned(),
            remote_hash,
            local_group_revision,
            added_json: "[]".to_owned(),
            unchanged_json: "[]".to_owned(),
            local_only_json: "[]".to_owned(),
            status: "pending".to_owned(),
            created_at: timestamp.to_owned(),
            expires_at,
        })
    }

    pub fn commit_import_preview(
        &self,
        preview_id: &str,
        delete_instrument_ids: &[String],
        timestamp: &str,
    ) -> Result<StoredImportRun, WatchlistStoreError> {
        validate_rfc3339_timestamp(timestamp)?;
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(WatchlistStoreError::Query)?;

        let preview: StoredImportPreview = transaction
            .query_row(
                "SELECT preview_id, source_id, remote_group_id, remote_group_name,
                        local_group_id, new_group_name, remote_hash, local_group_revision,
                        added_json, unchanged_json, local_only_json, status, created_at, expires_at
                 FROM watchlist_import_previews WHERE preview_id = ?1",
                params![preview_id],
                |row| {
                    Ok(StoredImportPreview {
                        preview_id: row.get(0)?,
                        source_id: row.get(1)?,
                        remote_group_id: row.get(2)?,
                        remote_group_name: row.get(3)?,
                        local_group_id: row.get(4)?,
                        new_group_name: row.get(5)?,
                        remote_hash: row.get(6)?,
                        local_group_revision: row.get(7)?,
                        added_json: row.get(8)?,
                        unchanged_json: row.get(9)?,
                        local_only_json: row.get(10)?,
                        status: row.get(11)?,
                        created_at: row.get(12)?,
                        expires_at: row.get(13)?,
                    })
                },
            )
            .optional()
            .map_err(WatchlistStoreError::Query)?
            .ok_or(WatchlistStoreError::NotFound)?;

        if preview.status != "pending" {
            return Err(WatchlistStoreError::Conflict);
        }

        let local_group_id = if !preview.local_group_id.is_empty() {
            preview.local_group_id.clone()
        } else {
            let name = if !preview.new_group_name.is_empty() {
                preview.new_group_name.as_str()
            } else {
                preview.remote_group_name.as_str()
            };
            let key = group_name_key(name);
            let gid = format!("wlgrp_{}", generate_id());
            transaction
                .execute(
                    "INSERT INTO watchlist_groups
                        (group_id, name, name_key, is_default, protected, revision, created_at, updated_at)
                     VALUES (?1, ?2, ?3, 0, 0, 1, ?4, ?4)",
                    params![gid, name, key, timestamp],
                )
                .map_err(WatchlistStoreError::Query)?;
            gid
        };

        transaction
            .execute(
                "UPDATE watchlist_import_previews SET status = 'committed' WHERE preview_id = ?1",
                params![preview_id],
            )
            .map_err(WatchlistStoreError::Query)?;

        let run_id = format!("wlrun_{}", generate_id());
        let run = StoredImportRun {
            run_id: run_id.clone(),
            preview_id: preview.preview_id.clone(),
            source_id: preview.source_id.clone(),
            remote_group_id: preview.remote_group_id.clone(),
            remote_group_name: preview.remote_group_name.clone(),
            local_group_id: local_group_id.clone(),
            status: "completed".to_owned(),
            added_count: 0,
            removed_count: delete_instrument_ids.len(),
            unchanged_count: 0,
            remote_hash: preview.remote_hash.clone(),
            created_at: timestamp.to_owned(),
            completed_at: timestamp.to_owned(),
        };

        transaction
            .execute(
                "INSERT INTO watchlist_import_runs
                    (run_id, preview_id, source_id, remote_group_id, remote_group_name,
                     local_group_id, status, added_count, removed_count, unchanged_count,
                     remote_hash, created_at, completed_at)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13)",
                params![
                    run.run_id,
                    run.preview_id,
                    run.source_id,
                    run.remote_group_id,
                    run.remote_group_name,
                    run.local_group_id,
                    run.status,
                    run.added_count as i64,
                    run.removed_count as i64,
                    run.unchanged_count as i64,
                    run.remote_hash,
                    run.created_at,
                    run.completed_at,
                ],
            )
            .map_err(WatchlistStoreError::Query)?;

        transaction.commit().map_err(WatchlistStoreError::Query)?;
        Ok(run)
    }

    pub fn list_items(&self) -> Result<Vec<serde_json::Value>, WatchlistStoreError> {
        let connection = self.lock()?;
        let mut stmt = connection
            .prepare(
                "SELECT instrument_id, market, symbol, name, instrument_type, membership_revision, created_at, updated_at
                 FROM watchlist_instruments ORDER BY market, symbol",
            )
            .map_err(WatchlistStoreError::Query)?;
        let rows = stmt
            .query_map([], |row| {
                Ok(serde_json::json!({
                    "instrumentId": row.get::<_, String>(0)?,
                    "market": row.get::<_, String>(1)?,
                    "symbol": row.get::<_, String>(2)?,
                    "name": row.get::<_, String>(3)?,
                    "instrumentType": row.get::<_, String>(4)?,
                    "membershipRevision": row.get::<_, i64>(5)?,
                    "createdAt": row.get::<_, String>(6)?,
                    "updatedAt": row.get::<_, String>(7)?,
                }))
            })
            .map_err(WatchlistStoreError::Query)?;
        let mut items = Vec::new();
        for item in rows {
            items.push(item.map_err(WatchlistStoreError::Query)?);
        }
        Ok(items)
    }

    /// List local instruments using the same cursor, market, text and group
    /// filters as the Go watchlist repository.  The extra row is fetched so
    /// the returned cursor is only present when another page really exists.
    pub fn list_items_page(
        &self,
        group_id: Option<&str>,
        cursor: Option<&str>,
        limit: usize,
        query: Option<&str>,
        market: Option<&str>,
    ) -> Result<(Vec<serde_json::Value>, Option<String>), WatchlistStoreError> {
        if limit == 0 {
            return Err(WatchlistStoreError::Validation(
                "limit must be a positive integer".to_owned(),
            ));
        }
        let connection = self.lock()?;
        let rows = query_item_rows(
            &connection,
            group_id.unwrap_or_default(),
            cursor.unwrap_or_default(),
            query.unwrap_or_default(),
            market.unwrap_or_default(),
            limit,
        )?;
        let next_cursor = rows
            .get(limit)
            .map(|_| rows[limit - 1].instrument_id.clone());
        let selected_rows = if next_cursor.is_some() {
            &rows[..limit]
        } else {
            &rows[..]
        };
        let items = hydrate_item_rows(&connection, selected_rows)?;
        Ok((items, next_cursor))
    }

    pub fn list_sources(&self) -> Result<Vec<StoredSource>, WatchlistStoreError> {
        let connection = self.lock()?;
        let mut stmt = connection
            .prepare("SELECT source_id, broker, display_name, status, last_error, updated_at FROM watchlist_sources ORDER BY source_id")
            .map_err(WatchlistStoreError::Query)?;
        let rows = stmt
            .query_map([], |row| {
                let err: String = row.get(4)?;
                Ok(StoredSource {
                    source_id: row.get(0)?,
                    broker: row.get(1)?,
                    display_name: row.get(2)?,
                    status: row.get(3)?,
                    error: if err.is_empty() { None } else { Some(err) },
                    updated_at: row.get(5)?,
                })
            })
            .map_err(WatchlistStoreError::Query)?;
        let mut sources = Vec::new();
        for source in rows {
            sources.push(source.map_err(WatchlistStoreError::Query)?);
        }
        Ok(sources)
    }

    pub fn source_exists(&self, source_id: &str) -> Result<bool, WatchlistStoreError> {
        let connection = self.lock()?;
        connection
            .query_row(
                "SELECT EXISTS(SELECT 1 FROM watchlist_sources WHERE source_id = ?1)",
                params![source_id],
                |row| row.get::<_, i64>(0),
            )
            .map(|value| value != 0)
            .map_err(WatchlistStoreError::Query)
    }

    pub fn list_remote_groups(
        &self,
        source_id: &str,
    ) -> Result<Vec<StoredRemoteGroup>, WatchlistStoreError> {
        let connection = self.lock()?;
        let mut statement = connection
            .prepare(
                "SELECT source_id, remote_group_id, name, group_type, ambiguous,
                        member_count, remote_hash, observed_at
                 FROM watchlist_remote_groups
                 WHERE source_id = ?1
                 ORDER BY name, remote_group_id",
            )
            .map_err(WatchlistStoreError::Query)?;
        let rows = statement
            .query_map(params![source_id], |row| {
                Ok(StoredRemoteGroup {
                    source_id: row.get(0)?,
                    remote_group_id: row.get(1)?,
                    name: row.get(2)?,
                    group_type: row.get(3)?,
                    ambiguous: row.get::<_, i64>(4)? != 0,
                    member_count: row.get::<_, i64>(5)? as usize,
                    remote_hash: row.get(6)?,
                    observed_at: row.get(7)?,
                })
            })
            .map_err(WatchlistStoreError::Query)?;
        rows.map(|row| row.map_err(WatchlistStoreError::Query))
            .collect()
    }

    pub fn list_bindings(&self) -> Result<Vec<StoredBinding>, WatchlistStoreError> {
        let connection = self.lock()?;
        let mut stmt = connection
            .prepare("SELECT binding_id, source_id, remote_group_id, remote_name, local_group_id, created_at, updated_at FROM watchlist_bindings ORDER BY binding_id")
            .map_err(WatchlistStoreError::Query)?;
        let rows = stmt
            .query_map([], |row| {
                Ok(StoredBinding {
                    binding_id: row.get(0)?,
                    source_id: row.get(1)?,
                    remote_group_id: row.get(2)?,
                    remote_name: row.get(3)?,
                    local_group_id: row.get(4)?,
                    created_at: row.get(5)?,
                    updated_at: row.get(6)?,
                })
            })
            .map_err(WatchlistStoreError::Query)?;
        let mut bindings = Vec::new();
        for binding in rows {
            bindings.push(binding.map_err(WatchlistStoreError::Query)?);
        }
        Ok(bindings)
    }

    pub fn list_import_runs(&self) -> Result<Vec<StoredImportRun>, WatchlistStoreError> {
        let connection = self.lock()?;
        let mut stmt = connection
            .prepare("SELECT run_id, preview_id, source_id, remote_group_id, remote_group_name, local_group_id, status, added_count, removed_count, unchanged_count, remote_hash, created_at, completed_at FROM watchlist_import_runs ORDER BY completed_at DESC, run_id DESC")
            .map_err(WatchlistStoreError::Query)?;
        let rows = stmt
            .query_map([], |row| {
                Ok(StoredImportRun {
                    run_id: row.get(0)?,
                    preview_id: row.get(1)?,
                    source_id: row.get(2)?,
                    remote_group_id: row.get(3)?,
                    remote_group_name: row.get(4)?,
                    local_group_id: row.get(5)?,
                    status: row.get(6)?,
                    added_count: row.get::<_, i64>(7)? as usize,
                    removed_count: row.get::<_, i64>(8)? as usize,
                    unchanged_count: row.get::<_, i64>(9)? as usize,
                    remote_hash: row.get(10)?,
                    created_at: row.get(11)?,
                    completed_at: row.get(12)?,
                })
            })
            .map_err(WatchlistStoreError::Query)?;
        let mut runs = Vec::new();
        for run in rows {
            runs.push(run.map_err(WatchlistStoreError::Query)?);
        }
        Ok(runs)
    }

    pub fn list_import_runs_page(
        &self,
        source_id: Option<&str>,
        cursor: Option<&str>,
        limit: usize,
    ) -> Result<(Vec<StoredImportRun>, Option<String>), WatchlistStoreError> {
        if limit == 0 {
            return Err(WatchlistStoreError::Validation(
                "limit must be a positive integer".to_owned(),
            ));
        }
        let connection = self.lock()?;
        let mut args = Vec::new();
        let mut sql = String::from(
            "SELECT run_id, preview_id, source_id, remote_group_id, remote_group_name,
                    local_group_id, status, added_count, removed_count, unchanged_count,
                    remote_hash, created_at, completed_at
             FROM watchlist_import_runs WHERE 1 = 1",
        );
        if let Some(source_id) = source_id.filter(|value| !value.is_empty()) {
            sql.push_str(" AND source_id = ?");
            args.push(rusqlite::types::Value::Text(source_id.to_owned()));
        }
        if let Some(cursor) = cursor.filter(|value| !value.is_empty()) {
            let created_at: Option<String> = connection
                .query_row(
                    "SELECT created_at FROM watchlist_import_runs WHERE run_id = ?1",
                    params![cursor],
                    |row| row.get(0),
                )
                .optional()
                .map_err(WatchlistStoreError::Query)?;
            let Some(created_at) = created_at else {
                return Err(WatchlistStoreError::NotFound);
            };
            sql.push_str(" AND (created_at < ? OR (created_at = ? AND run_id < ?))");
            args.extend([
                rusqlite::types::Value::Text(created_at.clone()),
                rusqlite::types::Value::Text(created_at),
                rusqlite::types::Value::Text(cursor.to_owned()),
            ]);
        }
        sql.push_str(" ORDER BY created_at DESC, run_id DESC LIMIT ?");
        let fetch_limit = i64::try_from(limit.saturating_add(1)).unwrap_or(i64::MAX);
        args.push(rusqlite::types::Value::Integer(fetch_limit));

        let mut statement = connection
            .prepare(&sql)
            .map_err(WatchlistStoreError::Query)?;
        let rows = statement
            .query_map(params_from_iter(args.iter()), |row| {
                Ok(StoredImportRun {
                    run_id: row.get(0)?,
                    preview_id: row.get(1)?,
                    source_id: row.get(2)?,
                    remote_group_id: row.get(3)?,
                    remote_group_name: row.get(4)?,
                    local_group_id: row.get(5)?,
                    status: row.get(6)?,
                    added_count: row.get::<_, i64>(7)? as usize,
                    removed_count: row.get::<_, i64>(8)? as usize,
                    unchanged_count: row.get::<_, i64>(9)? as usize,
                    remote_hash: row.get(10)?,
                    created_at: row.get(11)?,
                    completed_at: row.get(12)?,
                })
            })
            .map_err(WatchlistStoreError::Query)?;
        let runs = rows
            .map(|row| row.map_err(WatchlistStoreError::Query))
            .collect::<Result<Vec<_>, _>>()?;
        let next_cursor = runs.get(limit).map(|_| runs[limit - 1].run_id.clone());
        let runs = if next_cursor.is_some() {
            runs[..limit].to_vec()
        } else {
            runs
        };
        Ok((runs, next_cursor))
    }
}

fn query_item_rows(
    connection: &Connection,
    group_id: &str,
    cursor: &str,
    query: &str,
    market: &str,
    limit: usize,
) -> Result<Vec<ItemListRow>, WatchlistStoreError> {
    let mut args = Vec::new();
    let mut sql = String::from(
        "SELECT i.instrument_id, i.market, i.symbol, i.name, i.instrument_type,
                i.membership_revision,
                (SELECT MAX(o.last_imported_at)
                   FROM watchlist_membership_origins o
                  WHERE o.instrument_id = i.instrument_id)
         FROM ",
    );
    let order_column = if group_id.is_empty() {
        sql.push_str(
            "watchlist_instruments i
             WHERE EXISTS (
                 SELECT 1 FROM watchlist_memberships member
                  WHERE member.instrument_id = i.instrument_id)",
        );
        "i.instrument_id"
    } else {
        sql.push_str(
            "watchlist_memberships member
             JOIN watchlist_instruments i ON i.instrument_id = member.instrument_id
             WHERE member.group_id = ?",
        );
        args.push(rusqlite::types::Value::Text(group_id.to_owned()));
        "member.instrument_id"
    };
    if !cursor.is_empty() {
        sql.push_str(" AND ");
        sql.push_str(order_column);
        sql.push_str(" > ?");
        args.push(rusqlite::types::Value::Text(cursor.to_owned()));
    }
    if !query.is_empty() {
        sql.push_str(" AND (UPPER(i.instrument_id) LIKE UPPER(?) OR UPPER(i.name) LIKE UPPER(?))");
        let pattern = format!("%{query}%");
        args.push(rusqlite::types::Value::Text(pattern.clone()));
        args.push(rusqlite::types::Value::Text(pattern));
    }
    match market {
        "CN" => sql.push_str(" AND i.market IN ('SH', 'SZ')"),
        "" => {}
        _ => {
            sql.push_str(" AND i.market = ?");
            args.push(rusqlite::types::Value::Text(market.to_owned()));
        }
    }
    sql.push_str(" ORDER BY ");
    sql.push_str(order_column);
    sql.push_str(" LIMIT ?");
    args.push(rusqlite::types::Value::Integer(
        i64::try_from(limit.saturating_add(1)).unwrap_or(i64::MAX),
    ));

    let mut statement = connection
        .prepare(&sql)
        .map_err(WatchlistStoreError::Query)?;
    let rows = statement
        .query_map(params_from_iter(args.iter()), |row| {
            Ok(ItemListRow {
                instrument_id: row.get(0)?,
                market: row.get(1)?,
                symbol: row.get(2)?,
                name: row.get(3)?,
                instrument_type: row.get(4)?,
                revision: row.get(5)?,
                last_imported_at: row.get(6)?,
            })
        })
        .map_err(WatchlistStoreError::Query)?;
    rows.map(|row| row.map_err(WatchlistStoreError::Query))
        .collect()
}

fn hydrate_item_rows(
    connection: &Connection,
    rows: &[ItemListRow],
) -> Result<Vec<serde_json::Value>, WatchlistStoreError> {
    if rows.is_empty() {
        return Ok(Vec::new());
    }
    let instrument_ids = rows
        .iter()
        .map(|row| row.instrument_id.clone())
        .collect::<Vec<_>>();
    let placeholders = std::iter::repeat_n("?", instrument_ids.len())
        .collect::<Vec<_>>()
        .join(",");
    let mut groups_by_instrument: BTreeMap<String, Vec<serde_json::Value>> = BTreeMap::new();
    let group_sql = format!(
        "SELECT m.instrument_id, g.group_id, g.name
           FROM watchlist_memberships m
           JOIN watchlist_groups g ON g.group_id = m.group_id
          WHERE m.instrument_id IN ({placeholders})
          ORDER BY m.instrument_id, g.is_default DESC, g.created_at, g.group_id"
    );
    let mut statement = connection
        .prepare(&group_sql)
        .map_err(WatchlistStoreError::Query)?;
    let group_rows = statement
        .query_map(params_from_iter(instrument_ids.iter()), |row| {
            let group_id: String = row.get(1)?;
            let name: String = row.get(2)?;
            Ok((
                row.get::<_, String>(0)?,
                json!({"groupId": group_id, "name": name}),
            ))
        })
        .map_err(WatchlistStoreError::Query)?;
    for group_row in group_rows {
        let (instrument_id, group) = group_row.map_err(WatchlistStoreError::Query)?;
        groups_by_instrument
            .entry(instrument_id)
            .or_default()
            .push(group);
    }

    let mut sources_by_instrument: BTreeMap<String, Vec<String>> = BTreeMap::new();
    let source_sql = format!(
        "SELECT DISTINCT instrument_id, source_id
           FROM watchlist_membership_origins
          WHERE instrument_id IN ({placeholders})
          ORDER BY instrument_id, source_id"
    );
    let mut statement = connection
        .prepare(&source_sql)
        .map_err(WatchlistStoreError::Query)?;
    let source_rows = statement
        .query_map(params_from_iter(instrument_ids.iter()), |row| {
            Ok((row.get::<_, String>(0)?, row.get::<_, String>(1)?))
        })
        .map_err(WatchlistStoreError::Query)?;
    for source_row in source_rows {
        let (instrument_id, source_id) = source_row.map_err(WatchlistStoreError::Query)?;
        sources_by_instrument
            .entry(instrument_id)
            .or_default()
            .push(source_id);
    }

    rows.iter()
        .map(|row| {
            let groups = groups_by_instrument
                .remove(&row.instrument_id)
                .unwrap_or_default();
            let group_ids = groups
                .iter()
                .filter_map(|group| group.get("groupId").and_then(|value| value.as_str()))
                .map(str::to_owned)
                .collect::<Vec<_>>();
            let mut item = serde_json::Map::new();
            item.insert("instrumentId".to_owned(), json!(row.instrument_id));
            item.insert("market".to_owned(), json!(row.market));
            item.insert("symbol".to_owned(), json!(row.symbol));
            if !row.name.is_empty() {
                item.insert("name".to_owned(), json!(row.name));
            }
            if !row.instrument_type.is_empty() {
                item.insert("type".to_owned(), json!(row.instrument_type));
            }
            item.insert("revision".to_owned(), json!(row.revision));
            item.insert("groupIds".to_owned(), json!(group_ids));
            item.insert("groups".to_owned(), json!(groups));
            if let Some(last_imported_at) = row
                .last_imported_at
                .as_deref()
                .filter(|value| !value.is_empty())
            {
                item.insert("lastImportedAt".to_owned(), json!(last_imported_at));
            }
            if let Some(source_ids) = sources_by_instrument.remove(&row.instrument_id)
                && !source_ids.is_empty()
            {
                item.insert("sourceIds".to_owned(), json!(source_ids));
            }
            Ok(serde_json::Value::Object(item))
        })
        .collect()
}

fn get_group_query(
    connection: &Connection,
    group_id: &str,
) -> Result<StoredGroup, WatchlistStoreError> {
    connection
        .query_row(
            "SELECT g.group_id, g.name, g.is_default, g.protected, g.revision,
                    (SELECT COUNT(*) FROM watchlist_memberships m WHERE m.group_id = g.group_id) AS item_count,
                    g.created_at, g.updated_at
             FROM watchlist_groups g
             WHERE g.group_id = ?1",
            params![group_id],
            |row| {
                Ok(StoredGroup {
                    group_id: row.get(0)?,
                    name: row.get(1)?,
                    is_default: row.get::<_, i64>(2)? != 0,
                    protected: row.get::<_, i64>(3)? != 0,
                    revision: row.get(4)?,
                    item_count: row.get::<_, i64>(5)? as usize,
                    created_at: row.get(6)?,
                    updated_at: row.get(7)?,
                })
            },
        )
        .optional()
        .map_err(WatchlistStoreError::Query)?
        .ok_or(WatchlistStoreError::NotFound)
}

fn get_memberships_query(
    connection: &Connection,
    canonical_id: &str,
) -> Result<Memberships, WatchlistStoreError> {
    let revision: i64 = connection
        .query_row(
            "SELECT membership_revision FROM watchlist_instruments WHERE instrument_id = ?1",
            params![canonical_id],
            |row| row.get(0),
        )
        .optional()
        .map_err(WatchlistStoreError::Query)?
        .unwrap_or(0);

    let mut statement = connection
        .prepare(
            "SELECT g.group_id, g.name
             FROM watchlist_groups g
             JOIN watchlist_memberships m ON m.group_id = g.group_id
             WHERE m.instrument_id = ?1
             ORDER BY g.is_default DESC, g.created_at, g.group_id",
        )
        .map_err(WatchlistStoreError::Query)?;

    let groups = statement
        .query_map(params![canonical_id], |row| {
            Ok(GroupRef {
                group_id: row.get(0)?,
                name: row.get(1)?,
            })
        })
        .map_err(WatchlistStoreError::Query)?
        .collect::<Result<Vec<_>, _>>()
        .map_err(WatchlistStoreError::Query)?;

    Ok(Memberships {
        instrument_id: canonical_id.to_owned(),
        revision,
        groups,
    })
}

fn is_unique_constraint(error: &rusqlite::Error) -> bool {
    if let rusqlite::Error::SqliteFailure(code, _) = error {
        code.code == ErrorCode::ConstraintViolation
    } else {
        false
    }
}

fn validate_rfc3339_timestamp(timestamp: &str) -> Result<(), WatchlistStoreError> {
    OffsetDateTime::parse(timestamp, &Rfc3339)
        .map(|_| ())
        .map_err(|error| {
            WatchlistStoreError::Incompatible(format!(
                "invalid RFC3339 timestamp {timestamp:?}: {error}"
            ))
        })
}

fn now_rfc3339() -> String {
    OffsetDateTime::now_utc()
        .format(&Rfc3339)
        .unwrap_or_else(|_| "2026-08-26T00:00:00Z".to_owned())
}

fn generate_id() -> String {
    use std::sync::atomic::{AtomicU64, Ordering};
    static COUNTER: AtomicU64 = AtomicU64::new(1);
    let id = COUNTER.fetch_add(1, Ordering::Relaxed);
    let timestamp = OffsetDateTime::now_utc().unix_timestamp_nanos();
    format!("{timestamp:x}_{id}")
}

#[derive(Debug)]
pub struct WatchlistTestCutoverStore {
    inner: WatchlistStore,
}

impl WatchlistTestCutoverStore {
    pub fn open_existing(
        path: impl AsRef<Path>,
        profile: &str,
    ) -> Result<Self, WatchlistStoreError> {
        let inner = WatchlistStore::open_existing(path, profile)?;
        Ok(Self { inner })
    }

    pub fn path(&self) -> &Path {
        self.inner.path()
    }

    pub fn list_groups(&self) -> Result<Vec<StoredGroup>, WatchlistStoreError> {
        self.inner.list_groups()
    }

    pub fn get_group(&self, group_id: &str) -> Result<StoredGroup, WatchlistStoreError> {
        self.inner.get_group(group_id)
    }

    pub fn create_group(
        &self,
        name: &str,
        timestamp: &str,
    ) -> Result<StoredGroup, WatchlistStoreError> {
        self.inner.create_group(name, timestamp)
    }

    pub fn update_group(
        &self,
        group_id: &str,
        name: &str,
        expected_revision: i64,
        timestamp: &str,
    ) -> Result<StoredGroup, WatchlistStoreError> {
        self.inner
            .update_group(group_id, name, expected_revision, timestamp)
    }

    pub fn delete_group(&self, group_id: &str) -> Result<(), WatchlistStoreError> {
        self.inner.delete_group(group_id)
    }

    pub fn delete_binding(&self, binding_id: &str) -> Result<(), WatchlistStoreError> {
        self.inner.delete_binding(binding_id)
    }

    pub fn get_memberships(&self, instrument_id: &str) -> Result<Memberships, WatchlistStoreError> {
        self.inner.get_memberships(instrument_id)
    }

    pub fn replace_memberships(
        &self,
        instrument_id: &str,
        group_ids: &[String],
        new_group_names: &[String],
        expected_revision: i64,
        timestamp: &str,
    ) -> Result<Memberships, WatchlistStoreError> {
        self.inner.replace_memberships(
            instrument_id,
            group_ids,
            new_group_names,
            expected_revision,
            timestamp,
        )
    }

    pub fn create_import_preview(
        &self,
        source_id: &str,
        remote_group_id: &str,
        local_group_id: Option<&str>,
        new_group_name: Option<&str>,
        timestamp: &str,
    ) -> Result<StoredImportPreview, WatchlistStoreError> {
        self.inner.create_import_preview(
            source_id,
            remote_group_id,
            local_group_id,
            new_group_name,
            timestamp,
        )
    }

    pub fn commit_import_preview(
        &self,
        preview_id: &str,
        delete_instrument_ids: &[String],
        timestamp: &str,
    ) -> Result<StoredImportRun, WatchlistStoreError> {
        self.inner
            .commit_import_preview(preview_id, delete_instrument_ids, timestamp)
    }

    pub fn list_items(&self) -> Result<Vec<serde_json::Value>, WatchlistStoreError> {
        self.inner.list_items()
    }

    pub fn list_items_page(
        &self,
        group_id: Option<&str>,
        cursor: Option<&str>,
        limit: usize,
        query: Option<&str>,
        market: Option<&str>,
    ) -> Result<(Vec<serde_json::Value>, Option<String>), WatchlistStoreError> {
        self.inner
            .list_items_page(group_id, cursor, limit, query, market)
    }

    pub fn list_sources(&self) -> Result<Vec<StoredSource>, WatchlistStoreError> {
        self.inner.list_sources()
    }

    pub fn source_exists(&self, source_id: &str) -> Result<bool, WatchlistStoreError> {
        self.inner.source_exists(source_id)
    }

    pub fn list_remote_groups(
        &self,
        source_id: &str,
    ) -> Result<Vec<StoredRemoteGroup>, WatchlistStoreError> {
        self.inner.list_remote_groups(source_id)
    }

    pub fn list_bindings(&self) -> Result<Vec<StoredBinding>, WatchlistStoreError> {
        self.inner.list_bindings()
    }

    pub fn list_import_runs(&self) -> Result<Vec<StoredImportRun>, WatchlistStoreError> {
        self.inner.list_import_runs()
    }

    pub fn list_import_runs_page(
        &self,
        source_id: Option<&str>,
        cursor: Option<&str>,
        limit: usize,
    ) -> Result<(Vec<StoredImportRun>, Option<String>), WatchlistStoreError> {
        self.inner.list_import_runs_page(source_id, cursor, limit)
    }
}
