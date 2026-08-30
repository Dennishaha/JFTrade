use std::collections::BTreeMap;
use std::path::{Path, PathBuf};
use std::sync::{Mutex, MutexGuard};
use std::time::Duration;

use jftrade_owner_lock::{OwnerDiagnostic, WriterLease, WriterLeaseError};
use rusqlite::{Connection, OpenFlags, OptionalExtension, TransactionBehavior, params};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use thiserror::Error;
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

use crate::schema_manifest::{SchemaManifestError, validate_current};

const ADK_COMPONENT: &str = "adk";
const ADK_SCHEMA_VERSION: i64 = 4;
const ADK_SESSION_COMPONENT: &str = "adk-session";
const ADK_SESSION_SCHEMA_VERSION: i64 = 4;
pub const ADK_TEST_CUTOVER_PROFILE: &str = "cutover-test-only.v1";
pub const ADK_PRODUCTION_PROFILE: &str = "production.v1";

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredAdkEntity {
    pub id: String,
    pub payload_json: String,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredAdkRun {
    pub id: String,
    pub session_id: String,
    pub agent_id: String,
    pub status: String,
    pub client_request_id: String,
    pub request_fingerprint: String,
    pub payload_json: String,
    pub created_at: String,
    pub updated_at: String,
}

/// Durable execution lease for an ADK run.  The fencing token is monotonic
/// across takeovers and is the token carried by every tool invocation claim.
#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredAdkRunLease {
    pub run_id: String,
    pub owner_id: String,
    pub fencing_token: i64,
    pub heartbeat_at_unix_ms: i64,
    pub expires_at_unix_ms: i64,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredAdkApproval {
    pub id: String,
    pub run_id: String,
    pub agent_id: String,
    pub status: String,
    pub payload_json: String,
    pub created_at: String,
    pub updated_at: String,
}

/// Durable result of one model tool invocation.  The invocation key is the
/// model supplied call id (or the deterministic run/round/index fallback),
/// making retries after a crash idempotent while keeping the public ADK JSON
/// projection unchanged.
#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredAdkToolInvocation {
    pub run_id: String,
    pub idempotency_key: String,
    pub tool_name: String,
    pub status: String,
    pub owner_id: String,
    pub fencing_token: i64,
    pub run_lease_token: i64,
    pub lease_expires_at_unix_ms: i64,
    pub input_json: String,
    pub output_json: String,
    pub created_at: String,
    pub updated_at: String,
}

/// Result of atomically appending a tool result to a run projection.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AdkToolResultCommit {
    pub changed: bool,
    pub invocation: StoredAdkToolInvocation,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub enum AdkToolInvocationClaim {
    /// The caller owns execution of this invocation under the durable lease.
    Execute(StoredAdkToolInvocation),
    /// A terminal outcome already exists and must be replayed.
    Replay(StoredAdkToolInvocation),
}

/// Atomic result of resolving an approval and staging its pending run.
///
/// The approval row, sibling approval rows and the embedded run projection
/// are committed together so a restart cannot expose a half-resolved run.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AdkApprovalResolution {
    pub approval: StoredAdkApproval,
    pub changed: bool,
    pub run: Option<StoredAdkRun>,
    pub should_continue: bool,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredAdkTask {
    pub id: String,
    pub status: String,
    pub agent_id: String,
    pub run_id: String,
    pub payload_json: String,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredAdkMemory {
    pub id: String,
    pub agent_id: String,
    pub scope: String,
    pub memory_key: String,
    pub payload_json: String,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredAdkWorkflow {
    pub id: String,
    pub status: String,
    pub payload_json: String,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredAdkWorkflowTrigger {
    pub id: String,
    pub workflow_id: String,
    pub trigger_type: String,
    pub status: String,
    pub next_run_at: String,
    pub payload_json: String,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredAdkWorkflowTriggerLog {
    pub id: String,
    pub workflow_id: String,
    pub trigger_id: String,
    pub trigger_type: String,
    pub status: String,
    pub run_id: String,
    pub payload_json: String,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredAdkAuditEvent {
    pub id: String,
    pub kind: String,
    pub subject_id: String,
    pub payload_json: String,
    pub created_at: String,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct CreateAdkRunParams<'a> {
    pub id: &'a str,
    pub session_id: &'a str,
    pub agent_id: &'a str,
    pub status: &'a str,
    pub client_request_id: &'a str,
    pub request_fingerprint: &'a str,
    pub payload_json: &'a str,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AdkApprovalStage<'a> {
    pub id: &'a str,
    pub run_id: &'a str,
    pub agent_id: &'a str,
    pub payload_json: &'a str,
}

/// Session event committed together with a terminal ADK run projection.
///
/// The session database is a separate SQLite file for compatibility with the
/// existing ADK schema.  `AdkStore` attaches it for the duration of one
/// transaction so a terminal run cannot become visible without its event.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AdkRunEvent<'a> {
    pub id: &'a str,
    pub session_id: &'a str,
    pub invocation_id: &'a str,
    pub author: &'a str,
    pub content: &'a str,
}

#[derive(Debug, Error)]
pub enum AdkStoreError {
    #[error("adk database path is required")]
    EmptyPath,
    #[error("unsupported adk writer profile: {0}")]
    UnsupportedProfile(String),
    #[error("adk database is not an existing regular file: {0}")]
    NotRegularFile(String),
    #[error(transparent)]
    WriterLease(#[from] WriterLeaseError),
    #[error("open adk database: {0}")]
    Open(#[source] rusqlite::Error),
    #[error("configure adk database: {0}")]
    Configure(#[source] rusqlite::Error),
    #[error(transparent)]
    Schema(#[from] SchemaManifestError),
    #[error("adk database lock is unavailable")]
    LockUnavailable,
    #[error("query adk database: {0}")]
    Query(#[source] rusqlite::Error),
    #[error("adk record not found: {0}")]
    NotFound(String),
    #[error("adk conflict: {0}")]
    Conflict(String),
    #[error("invalid adk request: {0}")]
    Validation(String),
    #[error("incompatible adk database: {0}")]
    Incompatible(String),
}

pub struct AdkStore {
    path: PathBuf,
    connection: Mutex<Connection>,
    _writer_lease: WriterLease,
}

impl std::fmt::Debug for AdkStore {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("AdkStore")
            .field("path", &self.path)
            .finish()
    }
}

impl AdkStore {
    pub fn open(path: impl AsRef<Path>) -> Result<Self, AdkStoreError> {
        Self::open_existing(path, ADK_PRODUCTION_PROFILE)
    }

    pub fn open_existing(path: impl AsRef<Path>, profile: &str) -> Result<Self, AdkStoreError> {
        let path = path.as_ref();
        if path.as_os_str().is_empty() {
            return Err(AdkStoreError::EmptyPath);
        }
        if profile != ADK_TEST_CUTOVER_PROFILE && profile != ADK_PRODUCTION_PROFILE {
            return Err(AdkStoreError::UnsupportedProfile(profile.to_owned()));
        }
        if !path
            .metadata()
            .map(|metadata| metadata.is_file())
            .unwrap_or(false)
        {
            return Err(AdkStoreError::NotRegularFile(path.display().to_string()));
        }

        let writer_lease = WriterLease::acquire(path, &OwnerDiagnostic::current("rust", profile))?;

        let connection = Connection::open_with_flags(
            path,
            OpenFlags::SQLITE_OPEN_READ_WRITE
                | OpenFlags::SQLITE_OPEN_NO_MUTEX
                | OpenFlags::SQLITE_OPEN_URI,
        )
        .map_err(AdkStoreError::Open)?;

        connection
            .busy_timeout(Duration::from_millis(5_000))
            .map_err(AdkStoreError::Configure)?;
        connection
            .pragma_update(None, "foreign_keys", "ON")
            .map_err(AdkStoreError::Configure)?;

        validate_current(
            &connection,
            &path.display().to_string(),
            ADK_COMPONENT,
            ADK_SCHEMA_VERSION,
        )?;

        Ok(Self {
            path: path.to_path_buf(),
            connection: Mutex::new(connection),
            _writer_lease: writer_lease,
        })
    }

    fn lock_connection(&self) -> Result<MutexGuard<'_, Connection>, AdkStoreError> {
        self.connection
            .lock()
            .map_err(|_| AdkStoreError::LockUnavailable)
    }

    fn now_rfc3339() -> String {
        OffsetDateTime::now_utc()
            .format(&Rfc3339)
            .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned())
    }

    // --- Providers ---
    pub fn upsert_provider(
        &self,
        id: &str,
        payload_json: &str,
    ) -> Result<StoredAdkEntity, AdkStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        let created_at = connection
            .query_row(
                "SELECT created_at FROM adk_providers WHERE id = ?1",
                params![id],
                |row| row.get::<_, String>(0),
            )
            .optional()
            .map_err(AdkStoreError::Query)?
            .unwrap_or_else(|| now.clone());
        connection
            .execute(
                "INSERT INTO adk_providers (id, payload_json, created_at, updated_at)
                 VALUES (?1, ?2, ?3, ?4)
                 ON CONFLICT(id) DO UPDATE SET payload_json = ?2, updated_at = ?4",
                params![id, payload_json, created_at, now],
            )
            .map_err(AdkStoreError::Query)?;

        Ok(StoredAdkEntity {
            id: id.to_owned(),
            payload_json: payload_json.to_owned(),
            created_at,
            updated_at: now,
        })
    }

    pub fn delete_provider(&self, id: &str) -> Result<bool, AdkStoreError> {
        let connection = self.lock_connection()?;
        let affected = connection
            .execute("DELETE FROM adk_providers WHERE id = ?1", params![id])
            .map_err(AdkStoreError::Query)?;
        Ok(affected > 0)
    }

    pub fn get_provider(&self, id: &str) -> Result<Option<StoredAdkEntity>, AdkStoreError> {
        let connection = self.lock_connection()?;
        connection
            .query_row(
                "SELECT id, payload_json, created_at, updated_at FROM adk_providers WHERE id = ?1",
                params![id],
                |row| {
                    Ok(StoredAdkEntity {
                        id: row.get(0)?,
                        payload_json: row.get(1)?,
                        created_at: row.get(2)?,
                        updated_at: row.get(3)?,
                    })
                },
            )
            .optional()
            .map_err(AdkStoreError::Query)
    }

    // --- Agents ---
    pub fn upsert_agent(
        &self,
        id: &str,
        payload_json: &str,
    ) -> Result<StoredAdkEntity, AdkStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        let created_at = connection
            .query_row(
                "SELECT created_at FROM adk_agents WHERE id = ?1",
                params![id],
                |row| row.get::<_, String>(0),
            )
            .optional()
            .map_err(AdkStoreError::Query)?
            .unwrap_or_else(|| now.clone());
        connection
            .execute(
                "INSERT INTO adk_agents (id, payload_json, created_at, updated_at)
                 VALUES (?1, ?2, ?3, ?4)
                 ON CONFLICT(id) DO UPDATE SET payload_json = ?2, updated_at = ?4",
                params![id, payload_json, created_at, now],
            )
            .map_err(AdkStoreError::Query)?;

        Ok(StoredAdkEntity {
            id: id.to_owned(),
            payload_json: payload_json.to_owned(),
            created_at,
            updated_at: now,
        })
    }

    pub fn delete_agent(&self, id: &str) -> Result<bool, AdkStoreError> {
        let connection = self.lock_connection()?;
        let affected = connection
            .execute("DELETE FROM adk_agents WHERE id = ?1", params![id])
            .map_err(AdkStoreError::Query)?;
        Ok(affected > 0)
    }

    pub fn get_agent(&self, id: &str) -> Result<Option<StoredAdkEntity>, AdkStoreError> {
        let connection = self.lock_connection()?;
        connection
            .query_row(
                "SELECT id, payload_json, created_at, updated_at FROM adk_agents WHERE id = ?1",
                params![id],
                |row| {
                    Ok(StoredAdkEntity {
                        id: row.get(0)?,
                        payload_json: row.get(1)?,
                        created_at: row.get(2)?,
                        updated_at: row.get(3)?,
                    })
                },
            )
            .optional()
            .map_err(AdkStoreError::Query)
    }

    // --- Sessions ---
    pub fn upsert_session(
        &self,
        id: &str,
        agent_id: &str,
        payload_json: &str,
    ) -> Result<StoredAdkEntity, AdkStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        let created_at = connection
            .query_row(
                "SELECT created_at FROM adk_sessions WHERE id = ?1",
                params![id],
                |row| row.get::<_, String>(0),
            )
            .optional()
            .map_err(AdkStoreError::Query)?
            .unwrap_or_else(|| now.clone());
        connection
            .execute(
                "INSERT INTO adk_sessions (id, agent_id, payload_json, created_at, updated_at)
                 VALUES (?1, ?2, ?3, ?4, ?4)
                 ON CONFLICT(id) DO UPDATE SET agent_id = ?2, payload_json = ?3, updated_at = ?5",
                params![id, agent_id, payload_json, created_at, now],
            )
            .map_err(AdkStoreError::Query)?;

        Ok(StoredAdkEntity {
            id: id.to_owned(),
            payload_json: payload_json.to_owned(),
            created_at,
            updated_at: now,
        })
    }

    pub fn delete_session(&self, id: &str) -> Result<bool, AdkStoreError> {
        let mut connection = self.lock_connection()?;
        let exists = connection
            .query_row(
                "SELECT 1 FROM adk_sessions WHERE id = ?1 LIMIT 1",
                params![id],
                |row| row.get::<_, i64>(0),
            )
            .optional()
            .map_err(AdkStoreError::Query)?
            .is_some();
        if !exists {
            return Ok(false);
        }

        let transaction = connection.transaction().map_err(AdkStoreError::Query)?;
        for statement in [
            "DELETE FROM adk_tool_invocations WHERE run_id IN (SELECT id FROM adk_runs WHERE session_id = ?1)",
            "DELETE FROM adk_run_leases WHERE run_id IN (SELECT id FROM adk_runs WHERE session_id = ?1)",
            "DELETE FROM adk_approvals WHERE run_id IN (SELECT id FROM adk_runs WHERE session_id = ?1)",
            "DELETE FROM adk_tasks WHERE run_id IN (SELECT id FROM adk_runs WHERE session_id = ?1)",
            "DELETE FROM adk_runs WHERE session_id = ?1",
            "DELETE FROM adk_session_contexts WHERE id = ?1",
            "DELETE FROM adk_session_context_state WHERE id = ?1",
            "DELETE FROM adk_handoff_segments WHERE session_id = ?1",
            "DELETE FROM adk_session_notices WHERE session_id = ?1",
            "DELETE FROM adk_session_composer_state WHERE session_id = ?1",
            "DELETE FROM adk_sessions WHERE id = ?1",
        ] {
            transaction
                .execute(statement, params![id])
                .map_err(AdkStoreError::Query)?;
        }
        transaction.commit().map_err(AdkStoreError::Query)?;
        Ok(true)
    }

    pub fn get_session(&self, id: &str) -> Result<Option<StoredAdkEntity>, AdkStoreError> {
        let connection = self.lock_connection()?;
        connection
            .query_row(
                "SELECT id, payload_json, created_at, updated_at FROM adk_sessions WHERE id = ?1",
                params![id],
                stored_entity,
            )
            .optional()
            .map_err(AdkStoreError::Query)
    }

    pub fn list_sessions(&self) -> Result<Vec<StoredAdkEntity>, AdkStoreError> {
        let connection = self.lock_connection()?;
        let mut statement = connection
            .prepare(
                "SELECT id, payload_json, created_at, updated_at
                 FROM adk_sessions ORDER BY updated_at DESC, id ASC",
            )
            .map_err(AdkStoreError::Query)?;
        let rows = statement
            .query_map([], stored_entity)
            .map_err(AdkStoreError::Query)?;
        collect_rows(rows)
    }

    pub fn get_session_context(
        &self,
        session_id: &str,
    ) -> Result<Option<StoredAdkEntity>, AdkStoreError> {
        self.get_simple_entity("adk_session_context_state", session_id)
    }

    pub fn get_session_composer_state(
        &self,
        session_id: &str,
    ) -> Result<Option<StoredAdkEntity>, AdkStoreError> {
        let connection = self.lock_connection()?;
        connection
            .query_row(
                "SELECT id, payload_json, created_at, updated_at
                 FROM adk_session_composer_state WHERE session_id = ?1
                 ORDER BY updated_at DESC LIMIT 1",
                params![session_id],
                stored_entity,
            )
            .optional()
            .map_err(AdkStoreError::Query)
    }

    pub fn upsert_session_composer_state(
        &self,
        session_id: &str,
        payload_json: &str,
    ) -> Result<StoredAdkEntity, AdkStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        let created_at = connection
            .query_row(
                "SELECT created_at FROM adk_session_composer_state WHERE session_id = ?1 ORDER BY updated_at DESC LIMIT 1",
                params![session_id],
                |row| row.get::<_, String>(0),
            )
            .optional()
            .map_err(AdkStoreError::Query)?
            .unwrap_or_else(|| now.clone());
        connection
            .execute(
                "INSERT INTO adk_session_composer_state (id, session_id, payload_json, created_at, updated_at)
                 VALUES (?1, ?1, ?2, ?3, ?4)
                 ON CONFLICT(id) DO UPDATE SET session_id = ?1, payload_json = ?2, updated_at = ?4",
                params![session_id, payload_json, created_at, now],
            )
            .map_err(AdkStoreError::Query)?;
        Ok(StoredAdkEntity {
            id: session_id.to_owned(),
            payload_json: payload_json.to_owned(),
            created_at,
            updated_at: now,
        })
    }

    // --- Runs ---
    /// Acquire the single durable execution lease for a run.
    ///
    /// An unexpired lease is never silently stolen.  Once the lease expires,
    /// takeover increments the fencing token so writes from the old executor
    /// cannot commit.  Run existence is checked in the same transaction to
    /// avoid creating orphan lease rows.
    pub fn claim_run_lease(
        &self,
        run_id: &str,
        owner_id: &str,
        lease_ttl: Duration,
    ) -> Result<StoredAdkRunLease, AdkStoreError> {
        let run_id = run_id.trim();
        let owner_id = owner_id.trim();
        if run_id.is_empty() || owner_id.is_empty() || lease_ttl.is_zero() {
            return Err(AdkStoreError::Validation(
                "run lease requires run id, owner id and positive TTL".to_owned(),
            ));
        }
        let now = OffsetDateTime::now_utc();
        let now_ms = (now.unix_timestamp_nanos() / 1_000_000) as i64;
        let expires_ms = now_ms.saturating_add(lease_ttl.as_millis().min(i64::MAX as u128) as i64);
        let now_text = now
            .format(&Rfc3339)
            .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned());
        let mut connection = self.lock_connection()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(AdkStoreError::Query)?;
        let exists = transaction
            .query_row(
                "SELECT 1 FROM adk_runs WHERE id = ?1 LIMIT 1",
                params![run_id],
                |row| row.get::<_, i64>(0),
            )
            .optional()
            .map_err(AdkStoreError::Query)?
            .is_some();
        if !exists {
            return Err(AdkStoreError::NotFound(run_id.to_owned()));
        }
        let existing = transaction
            .query_row(
                "SELECT run_id, owner_id, fencing_token, heartbeat_at_unix_ms,
                        expires_at_unix_ms, created_at, updated_at
                 FROM adk_run_leases WHERE run_id = ?1",
                params![run_id],
                stored_run_lease,
            )
            .optional()
            .map_err(AdkStoreError::Query)?;
        if let Some(existing) = existing {
            if existing.expires_at_unix_ms > now_ms {
                return Err(AdkStoreError::Conflict(format!(
                    "run {run_id} lease is held by {}",
                    existing.owner_id
                )));
            }
            let fencing_token = existing.fencing_token.saturating_add(1);
            let affected = transaction
                .execute(
                    "UPDATE adk_run_leases
                     SET owner_id = ?1, fencing_token = ?2,
                         heartbeat_at_unix_ms = ?3, expires_at_unix_ms = ?4,
                         updated_at = ?5
                     WHERE run_id = ?6 AND expires_at_unix_ms <= ?7",
                    params![
                        owner_id,
                        fencing_token,
                        now_ms,
                        expires_ms,
                        now_text,
                        run_id,
                        now_ms
                    ],
                )
                .map_err(AdkStoreError::Query)?;
            if affected != 1 {
                return Err(AdkStoreError::Conflict(format!(
                    "run {run_id} lease changed before takeover"
                )));
            }
            transaction.commit().map_err(AdkStoreError::Query)?;
            return Ok(StoredAdkRunLease {
                run_id: run_id.to_owned(),
                owner_id: owner_id.to_owned(),
                fencing_token,
                heartbeat_at_unix_ms: now_ms,
                expires_at_unix_ms: expires_ms,
                created_at: existing.created_at,
                updated_at: now_text,
            });
        }
        transaction
            .execute(
                "INSERT INTO adk_run_leases
                 (run_id, owner_id, fencing_token, heartbeat_at_unix_ms,
                  expires_at_unix_ms, created_at, updated_at)
                 VALUES (?1, ?2, 1, ?3, ?4, ?5, ?5)",
                params![run_id, owner_id, now_ms, expires_ms, now_text],
            )
            .map_err(AdkStoreError::Query)?;
        transaction.commit().map_err(AdkStoreError::Query)?;
        Ok(StoredAdkRunLease {
            run_id: run_id.to_owned(),
            owner_id: owner_id.to_owned(),
            fencing_token: 1,
            heartbeat_at_unix_ms: now_ms,
            expires_at_unix_ms: expires_ms,
            created_at: now_text.clone(),
            updated_at: now_text,
        })
    }

    /// Refresh an unexpired run lease only for its current owner and fence.
    pub fn heartbeat_run_lease(
        &self,
        lease: &StoredAdkRunLease,
        lease_ttl: Duration,
    ) -> Result<StoredAdkRunLease, AdkStoreError> {
        if lease.run_id.trim().is_empty() || lease.owner_id.trim().is_empty() || lease_ttl.is_zero()
        {
            return Err(AdkStoreError::Validation(
                "run lease heartbeat requires a valid lease and positive TTL".to_owned(),
            ));
        }
        let now = OffsetDateTime::now_utc();
        let now_ms = (now.unix_timestamp_nanos() / 1_000_000) as i64;
        let expires_ms = now_ms.saturating_add(lease_ttl.as_millis().min(i64::MAX as u128) as i64);
        let now_text = now
            .format(&Rfc3339)
            .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned());
        let connection = self.lock_connection()?;
        let affected = connection
            .execute(
                "UPDATE adk_run_leases
                 SET heartbeat_at_unix_ms = ?1, expires_at_unix_ms = ?2,
                     updated_at = ?3
                 WHERE run_id = ?4 AND owner_id = ?5 AND fencing_token = ?6
                   AND expires_at_unix_ms > ?7",
                params![
                    now_ms,
                    expires_ms,
                    now_text,
                    lease.run_id,
                    lease.owner_id,
                    lease.fencing_token,
                    now_ms
                ],
            )
            .map_err(AdkStoreError::Query)?;
        if affected != 1 {
            return Err(AdkStoreError::Conflict(format!(
                "run {} lease fencing token {} is no longer current",
                lease.run_id, lease.fencing_token
            )));
        }
        Ok(StoredAdkRunLease {
            heartbeat_at_unix_ms: now_ms,
            expires_at_unix_ms: expires_ms,
            updated_at: now_text,
            ..lease.clone()
        })
    }

    /// Release a run lease idempotently.  Incrementing the fence invalidates
    /// any in-flight worker that still holds the old token.
    pub fn release_run_lease(&self, lease: &StoredAdkRunLease) -> Result<bool, AdkStoreError> {
        if lease.run_id.trim().is_empty() || lease.owner_id.trim().is_empty() {
            return Ok(false);
        }
        let now = OffsetDateTime::now_utc();
        let now_ms = (now.unix_timestamp_nanos() / 1_000_000) as i64;
        let now_text = now
            .format(&Rfc3339)
            .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned());
        let connection = self.lock_connection()?;
        let affected = connection
            .execute(
                "UPDATE adk_run_leases
                 SET owner_id = '', fencing_token = fencing_token + 1,
                     heartbeat_at_unix_ms = ?1, expires_at_unix_ms = ?1,
                     updated_at = ?2
                 WHERE run_id = ?3 AND owner_id = ?4 AND fencing_token = ?5",
                params![
                    now_ms,
                    now_text,
                    lease.run_id,
                    lease.owner_id,
                    lease.fencing_token
                ],
            )
            .map_err(AdkStoreError::Query)?;
        Ok(affected == 1)
    }

    pub fn get_run_lease(&self, run_id: &str) -> Result<Option<StoredAdkRunLease>, AdkStoreError> {
        let connection = self.lock_connection()?;
        connection
            .query_row(
                "SELECT run_id, owner_id, fencing_token, heartbeat_at_unix_ms,
                        expires_at_unix_ms, created_at, updated_at
                 FROM adk_run_leases WHERE run_id = ?1",
                params![run_id],
                stored_run_lease,
            )
            .optional()
            .map_err(AdkStoreError::Query)
    }

    pub fn create_run(
        &self,
        params: CreateAdkRunParams<'_>,
    ) -> Result<StoredAdkRun, AdkStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        connection
            .execute(
                "INSERT INTO adk_runs (id, session_id, agent_id, status, client_request_id, request_fingerprint, payload_json, created_at, updated_at)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?8)",
                params![
                    params.id,
                    params.session_id,
                    params.agent_id,
                    params.status,
                    params.client_request_id,
                    params.request_fingerprint,
                    params.payload_json,
                    now,
                ],
            )
            .map_err(AdkStoreError::Query)?;

        Ok(StoredAdkRun {
            id: params.id.to_owned(),
            session_id: params.session_id.to_owned(),
            agent_id: params.agent_id.to_owned(),
            status: params.status.to_owned(),
            client_request_id: params.client_request_id.to_owned(),
            request_fingerprint: params.request_fingerprint.to_owned(),
            payload_json: params.payload_json.to_owned(),
            created_at: now.clone(),
            updated_at: now,
        })
    }

    /// Create a run projection and its initial session event in one attached
    /// transaction.  This prevents a newly accepted chat request from being
    /// durable without the user message that established its transcript.
    pub fn create_run_with_event(
        &self,
        params: CreateAdkRunParams<'_>,
        session_db_path: &Path,
        event: &AdkRunEvent<'_>,
    ) -> Result<StoredAdkRun, AdkStoreError> {
        if params.id.trim().is_empty()
            || params.session_id.trim().is_empty()
            || event.id.trim().is_empty()
            || event.session_id != params.session_id
        {
            return Err(AdkStoreError::Validation(
                "run and initial session event identities must be present and match".to_owned(),
            ));
        }
        let mut connection = self.lock_connection()?;
        attach_adk_session_database(&connection, session_db_path)?;
        let now = Self::now_rfc3339();
        let result = (|| {
            let transaction = connection
                .transaction_with_behavior(TransactionBehavior::Immediate)
                .map_err(AdkStoreError::Query)?;
            transaction
                .execute(
                    "INSERT INTO adk_runs (id, session_id, agent_id, status, client_request_id, request_fingerprint, payload_json, created_at, updated_at)
                     VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?8)",
                    params![
                        params.id,
                        params.session_id,
                        params.agent_id,
                        params.status,
                        params.client_request_id,
                        params.request_fingerprint,
                        params.payload_json,
                        now,
                    ],
                )
                .map_err(AdkStoreError::Query)?;
            append_adk_session_events(&transaction, &now, params.id, std::slice::from_ref(event))?;
            transaction.commit().map_err(AdkStoreError::Query)?;
            Ok(StoredAdkRun {
                id: params.id.to_owned(),
                session_id: params.session_id.to_owned(),
                agent_id: params.agent_id.to_owned(),
                status: params.status.to_owned(),
                client_request_id: params.client_request_id.to_owned(),
                request_fingerprint: params.request_fingerprint.to_owned(),
                payload_json: params.payload_json.to_owned(),
                created_at: now.clone(),
                updated_at: now.clone(),
            })
        })();
        let detach = connection
            .execute("DETACH DATABASE adk_session_events", [])
            .map_err(AdkStoreError::Query);
        match (result, detach) {
            (Err(error), _) => Err(error),
            (Ok(_), Err(error)) => Err(error),
            (Ok(value), Ok(_)) => Ok(value),
        }
    }

    pub fn update_run_status(&self, id: &str, status: &str) -> Result<bool, AdkStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        let affected = connection
            .execute(
                "UPDATE adk_runs SET status = ?1, updated_at = ?2 WHERE id = ?3",
                params![status, now, id],
            )
            .map_err(AdkStoreError::Query)?;
        Ok(affected > 0)
    }

    pub fn update_run_payload(&self, id: &str, payload_json: &str) -> Result<bool, AdkStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        let affected = connection
            .execute(
                "UPDATE adk_runs SET payload_json = ?1, updated_at = ?2 WHERE id = ?3",
                params![payload_json, now, id],
            )
            .map_err(AdkStoreError::Query)?;
        Ok(affected > 0)
    }

    /// Update a run projection only when its indexed lifecycle status still
    /// equals `expected_status`. Streaming writers use this CAS boundary so a
    /// concurrent cancellation cannot be overwritten by stale provider data.
    pub fn update_run_payload_if_status(
        &self,
        id: &str,
        expected_status: &str,
        payload_json: &str,
    ) -> Result<bool, AdkStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        let affected = connection
            .execute(
                "UPDATE adk_runs
                 SET payload_json = ?1, updated_at = ?2
                 WHERE id = ?3 AND status = ?4",
                params![payload_json, now, id, expected_status],
            )
            .map_err(AdkStoreError::Query)?;
        Ok(affected > 0)
    }

    /// CAS variant that also fences the caller's durable revision token.  ADK
    /// rows predate an explicit integer revision column, so `updated_at` is
    /// the persisted opaque revision token exposed by every stored run.
    pub fn update_run_payload_if_status_and_revision(
        &self,
        id: &str,
        expected_status: &str,
        expected_updated_at: &str,
        payload_json: &str,
    ) -> Result<bool, AdkStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        let affected = connection
            .execute(
                "UPDATE adk_runs
                 SET payload_json = ?1, updated_at = ?2
                 WHERE id = ?3 AND status = ?4 AND updated_at = ?5",
                params![payload_json, now, id, expected_status, expected_updated_at],
            )
            .map_err(AdkStoreError::Query)?;
        Ok(affected > 0)
    }

    /// CAS a run projection while proving that the caller still owns the
    /// current, unexpired durable execution lease.  The lease predicate is
    /// part of the same SQLite statement as the payload write, so a takeover
    /// cannot race between a process-local lease check and the mutation.
    pub fn update_run_payload_if_status_and_revision_with_lease(
        &self,
        id: &str,
        expected_status: &str,
        expected_updated_at: &str,
        payload_json: &str,
        owner_id: &str,
        run_lease_token: i64,
    ) -> Result<bool, AdkStoreError> {
        validate_run_lease_identity(owner_id, run_lease_token)?;
        let now = Self::now_rfc3339();
        let now_ms = (OffsetDateTime::now_utc().unix_timestamp_nanos() / 1_000_000) as i64;
        let connection = self.lock_connection()?;
        let affected = connection
            .execute(
                "UPDATE adk_runs
                 SET payload_json = ?1, updated_at = ?2
                 WHERE id = ?3 AND status = ?4 AND updated_at = ?5
                   AND EXISTS (
                     SELECT 1 FROM adk_run_leases
                     WHERE run_id = ?3 AND owner_id = ?6 AND fencing_token = ?7
                       AND expires_at_unix_ms > ?8
                   )",
                params![
                    payload_json,
                    now,
                    id,
                    expected_status,
                    expected_updated_at,
                    owner_id,
                    run_lease_token,
                    now_ms
                ],
            )
            .map_err(AdkStoreError::Query)?;
        Ok(affected == 1)
    }

    /// Atomically updates the indexed lifecycle status and its JSON
    /// projection.  Run lifecycle mutations must update both columns as one
    /// unit; otherwise a crash between two independent statements can leave
    /// list endpoints and run detail endpoints disagreeing.
    pub fn update_run_state(
        &self,
        id: &str,
        status: &str,
        payload_json: &str,
    ) -> Result<bool, AdkStoreError> {
        let now = Self::now_rfc3339();
        let mut connection = self.lock_connection()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(AdkStoreError::Query)?;
        let affected = transaction
            .execute(
                "UPDATE adk_runs
                 SET status = ?1, payload_json = ?2, updated_at = ?3
                 WHERE id = ?4",
                params![status, payload_json, now, id],
            )
            .map_err(AdkStoreError::Query)?;
        transaction.commit().map_err(AdkStoreError::Query)?;
        Ok(affected > 0)
    }

    /// Atomically transition a run only when it is still in
    /// `expected_status`. This prevents terminal completion/failure from
    /// clobbering a cancellation committed by another request.
    pub fn update_run_state_if_status(
        &self,
        id: &str,
        expected_status: &str,
        status: &str,
        payload_json: &str,
    ) -> Result<bool, AdkStoreError> {
        let now = Self::now_rfc3339();
        let mut connection = self.lock_connection()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(AdkStoreError::Query)?;
        let affected = transaction
            .execute(
                "UPDATE adk_runs
                 SET status = ?1, payload_json = ?2, updated_at = ?3
                 WHERE id = ?4 AND status = ?5",
                params![status, payload_json, now, id, expected_status],
            )
            .map_err(AdkStoreError::Query)?;
        transaction.commit().map_err(AdkStoreError::Query)?;
        Ok(affected > 0)
    }

    /// Atomically transitions a run only when both its lifecycle status and
    /// persisted revision token still match the caller's snapshot.
    pub fn update_run_state_if_status_and_revision(
        &self,
        id: &str,
        expected_status: &str,
        expected_updated_at: &str,
        status: &str,
        payload_json: &str,
    ) -> Result<bool, AdkStoreError> {
        let now = Self::now_rfc3339();
        let mut connection = self.lock_connection()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(AdkStoreError::Query)?;
        let affected = transaction
            .execute(
                "UPDATE adk_runs
                 SET status = ?1, payload_json = ?2, updated_at = ?3
                 WHERE id = ?4 AND status = ?5 AND updated_at = ?6",
                params![
                    status,
                    payload_json,
                    now,
                    id,
                    expected_status,
                    expected_updated_at
                ],
            )
            .map_err(AdkStoreError::Query)?;
        transaction.commit().map_err(AdkStoreError::Query)?;
        Ok(affected > 0)
    }

    /// Terminal-state CAS fenced to the current durable run lease.
    pub fn update_run_state_if_status_and_revision_with_lease(
        &self,
        id: &str,
        expected_status: &str,
        expected_updated_at: &str,
        status: &str,
        payload_json: &str,
        owner_id: &str,
        run_lease_token: i64,
    ) -> Result<bool, AdkStoreError> {
        validate_run_lease_identity(owner_id, run_lease_token)?;
        let now = Self::now_rfc3339();
        let now_ms = (OffsetDateTime::now_utc().unix_timestamp_nanos() / 1_000_000) as i64;
        let connection = self.lock_connection()?;
        let affected = connection
            .execute(
                "UPDATE adk_runs
                 SET status = ?1, payload_json = ?2, updated_at = ?3
                 WHERE id = ?4 AND status = ?5 AND updated_at = ?6
                   AND EXISTS (
                     SELECT 1 FROM adk_run_leases
                     WHERE run_id = ?4 AND owner_id = ?7 AND fencing_token = ?8
                       AND expires_at_unix_ms > ?9
                   )",
                params![
                    status,
                    payload_json,
                    now,
                    id,
                    expected_status,
                    expected_updated_at,
                    owner_id,
                    run_lease_token,
                    now_ms
                ],
            )
            .map_err(AdkStoreError::Query)?;
        Ok(affected == 1)
    }

    /// Atomically update a run payload and append its ADK session events.
    ///
    /// Streaming projections mutate `streamEvents` and `providerEvents`
    /// without changing the indexed run status.  Keeping this CAS and the
    /// attached session journal write in one transaction closes the crash
    /// window where a run projection could become visible without its event.
    #[allow(clippy::too_many_arguments)]
    pub fn update_run_payload_if_status_and_revision_with_events_with_lease(
        &self,
        id: &str,
        expected_status: &str,
        expected_updated_at: &str,
        payload_json: &str,
        session_db_path: &Path,
        events: &[AdkRunEvent<'_>],
        owner_id: &str,
        run_lease_token: i64,
    ) -> Result<bool, AdkStoreError> {
        validate_run_lease_identity(owner_id, run_lease_token)?;
        let mut connection = self.lock_connection()?;
        attach_adk_session_database(&connection, session_db_path)?;
        let result = (|| {
            let now = Self::now_rfc3339();
            let transaction = connection
                .transaction_with_behavior(TransactionBehavior::Immediate)
                .map_err(AdkStoreError::Query)?;
            ensure_current_run_lease(&transaction, id, owner_id, run_lease_token)?;
            let affected = transaction
                .execute(
                    "UPDATE adk_runs
                     SET payload_json = ?1, updated_at = ?2
                     WHERE id = ?3 AND status = ?4 AND updated_at = ?5",
                    params![payload_json, now, id, expected_status, expected_updated_at],
                )
                .map_err(AdkStoreError::Query)?;
            if affected == 1 {
                append_adk_session_events(&transaction, &now, id, events)?;
            }
            transaction.commit().map_err(AdkStoreError::Query)?;
            Ok(affected == 1)
        })();
        let detach = connection
            .execute("DETACH DATABASE adk_session_events", [])
            .map_err(AdkStoreError::Query);
        match (result, detach) {
            (Err(error), _) => Err(error),
            (Ok(_), Err(error)) => Err(error),
            (Ok(value), Ok(_)) => Ok(value),
        }
    }

    /// Atomically transition a run and append its session events.
    ///
    /// ADK runs and session events intentionally remain in their historical
    /// separate SQLite files.  SQLite's attached-database transaction gives
    /// us one commit/rollback boundary across both files while preserving the
    /// public schema and ownership model.  A constraint or foreign-key error
    /// aborts the whole transaction, keeping a failed completion fail-closed.
    pub fn update_run_state_if_status_and_revision_with_events(
        &self,
        id: &str,
        expected_status: &str,
        expected_updated_at: &str,
        status: &str,
        payload_json: &str,
        session_db_path: &Path,
        events: &[AdkRunEvent<'_>],
    ) -> Result<bool, AdkStoreError> {
        self.update_run_state_if_status_and_revision_with_events_inner(
            id,
            expected_status,
            expected_updated_at,
            status,
            payload_json,
            session_db_path,
            events,
            None,
        )
    }

    /// Atomically transition a run, append session events and validate the
    /// current durable execution lease in the same attached-database
    /// transaction.
    #[allow(clippy::too_many_arguments)]
    pub fn update_run_state_if_status_and_revision_with_events_with_lease(
        &self,
        id: &str,
        expected_status: &str,
        expected_updated_at: &str,
        status: &str,
        payload_json: &str,
        session_db_path: &Path,
        events: &[AdkRunEvent<'_>],
        owner_id: &str,
        run_lease_token: i64,
    ) -> Result<bool, AdkStoreError> {
        validate_run_lease_identity(owner_id, run_lease_token)?;
        self.update_run_state_if_status_and_revision_with_events_inner(
            id,
            expected_status,
            expected_updated_at,
            status,
            payload_json,
            session_db_path,
            events,
            Some((owner_id, run_lease_token)),
        )
    }

    #[allow(clippy::too_many_arguments)]
    fn update_run_state_if_status_and_revision_with_events_inner(
        &self,
        id: &str,
        expected_status: &str,
        expected_updated_at: &str,
        status: &str,
        payload_json: &str,
        session_db_path: &Path,
        events: &[AdkRunEvent<'_>],
        lease: Option<(&str, i64)>,
    ) -> Result<bool, AdkStoreError> {
        let mut connection = self.lock_connection()?;
        attach_adk_session_database(&connection, session_db_path)?;
        let result = (|| {
            let now = Self::now_rfc3339();
            let transaction = connection
                .transaction_with_behavior(TransactionBehavior::Immediate)
                .map_err(AdkStoreError::Query)?;
            if let Some((owner_id, run_lease_token)) = lease {
                ensure_current_run_lease(&transaction, id, owner_id, run_lease_token)?;
            }
            let affected = transaction
                .execute(
                    "UPDATE adk_runs
                     SET status = ?1, payload_json = ?2, updated_at = ?3
                     WHERE id = ?4 AND status = ?5 AND updated_at = ?6",
                    params![
                        status,
                        payload_json,
                        now,
                        id,
                        expected_status,
                        expected_updated_at
                    ],
                )
                .map_err(AdkStoreError::Query)?;
            if affected == 1 {
                append_adk_session_events(&transaction, &now, id, events)?;
            }
            transaction.commit().map_err(AdkStoreError::Query)?;
            Ok(affected == 1)
        })();
        let detach = connection
            .execute("DETACH DATABASE adk_session_events", [])
            .map_err(AdkStoreError::Query);
        match (result, detach) {
            (Err(error), _) => Err(error),
            (Ok(_), Err(error)) => Err(error),
            (Ok(value), Ok(_)) => Ok(value),
        }
    }

    /// Read a previously persisted tool invocation by its run-scoped
    /// idempotency key.  A terminal row is replayable and must never cause
    /// the executor to run the side effect a second time after a retry.
    pub fn get_tool_invocation(
        &self,
        run_id: &str,
        idempotency_key: &str,
    ) -> Result<Option<StoredAdkToolInvocation>, AdkStoreError> {
        let connection = self.lock_connection()?;
        connection
            .query_row(
                "SELECT run_id, idempotency_key, tool_name, status, owner_id,
                        fencing_token, run_lease_token, input_json, output_json,
                        lease_expires_at_unix_ms, created_at, updated_at
                 FROM adk_tool_invocations
                 WHERE run_id = ?1 AND idempotency_key = ?2",
                params![run_id, idempotency_key],
                stored_tool_invocation,
            )
            .optional()
            .map_err(AdkStoreError::Query)
    }

    /// Claim a tool invocation before invoking any side effect.  The unique
    /// `(run_id, idempotency_key)` row is the durable fence: a second worker
    /// either replays a terminal result or receives a conflict while the
    /// first worker's lease is alive.  Expired RUNNING rows are fenced over
    /// with a monotonically increased token for crash recovery.
    pub fn claim_tool_invocation_if_status_and_revision(
        &self,
        run_id: &str,
        idempotency_key: &str,
        tool_name: &str,
        input_json: &str,
        expected_status: &str,
        expected_updated_at: &str,
        owner_id: &str,
        run_lease_token: i64,
        lease_ttl: Duration,
    ) -> Result<AdkToolInvocationClaim, AdkStoreError> {
        if lease_ttl.is_zero()
            || run_id.trim().is_empty()
            || idempotency_key.trim().is_empty()
            || owner_id.trim().is_empty()
            || run_lease_token <= 0
        {
            return Err(AdkStoreError::Validation(
                "tool invocation claim identity, run lease and TTL are required".to_owned(),
            ));
        }
        serde_json::from_str::<Value>(input_json).map_err(|error| {
            AdkStoreError::Validation(format!("invalid tool input JSON: {error}"))
        })?;
        let now = OffsetDateTime::now_utc();
        let now_ms = now.unix_timestamp_nanos() / 1_000_000;
        let expires_ms = now_ms.saturating_add(lease_ttl.as_millis() as i128);
        let now_text = now
            .format(&Rfc3339)
            .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned());
        let mut connection = self.lock_connection()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(AdkStoreError::Query)?;
        let run_revision = transaction
            .query_row(
                "SELECT status, updated_at FROM adk_runs WHERE id = ?1",
                params![run_id],
                |row| Ok((row.get::<_, String>(0)?, row.get::<_, String>(1)?)),
            )
            .optional()
            .map_err(AdkStoreError::Query)?;
        let Some((run_status, run_updated_at)) = run_revision else {
            return Err(AdkStoreError::NotFound(run_id.to_owned()));
        };
        if !run_status.eq_ignore_ascii_case(expected_status)
            || run_updated_at != expected_updated_at
        {
            transaction.commit().map_err(AdkStoreError::Query)?;
            return Err(AdkStoreError::Conflict(format!(
                "run {run_id} changed before tool invocation claim"
            )));
        }
        let lease_valid = transaction
            .query_row(
                "SELECT 1 FROM adk_run_leases
                 WHERE run_id = ?1 AND owner_id = ?2 AND fencing_token = ?3
                   AND expires_at_unix_ms > ?4",
                params![run_id, owner_id, run_lease_token, now_ms as i64],
                |row| row.get::<_, i64>(0),
            )
            .optional()
            .map_err(AdkStoreError::Query)?
            .is_some();
        if !lease_valid {
            return Err(AdkStoreError::Conflict(format!(
                "run {run_id} lease fencing token {run_lease_token} is no longer current"
            )));
        }
        let existing = transaction
            .query_row(
                "SELECT run_id, idempotency_key, tool_name, status, owner_id,
                        fencing_token, run_lease_token, input_json, output_json,
                        lease_expires_at_unix_ms, created_at, updated_at
                 FROM adk_tool_invocations
                 WHERE run_id = ?1 AND idempotency_key = ?2",
                params![run_id, idempotency_key],
                stored_tool_invocation,
            )
            .optional()
            .map_err(AdkStoreError::Query)?;
        if let Some(existing) = existing {
            if existing.tool_name != tool_name || existing.input_json != input_json {
                return Err(AdkStoreError::Conflict(format!(
                    "tool invocation key {idempotency_key} was reused with different input"
                )));
            }
            if matches!(
                existing.status.to_ascii_uppercase().as_str(),
                "SUCCEEDED" | "FAILED" | "UNKNOWN"
            ) {
                transaction.commit().map_err(AdkStoreError::Query)?;
                return Ok(AdkToolInvocationClaim::Replay(existing));
            }
            let lease_expires: i64 = transaction
                .query_row(
                    "SELECT lease_expires_at_unix_ms FROM adk_tool_invocations
                     WHERE run_id = ?1 AND idempotency_key = ?2",
                    params![run_id, idempotency_key],
                    |row| row.get(0),
                )
                .map_err(AdkStoreError::Query)?;
            if lease_expires > now_ms as i64 {
                return Err(AdkStoreError::Conflict(format!(
                    "tool invocation {idempotency_key} is already in flight"
                )));
            }
            let fencing_token: i64 = transaction
                .query_row(
                    "SELECT fencing_token FROM adk_tool_invocations
                     WHERE run_id = ?1 AND idempotency_key = ?2",
                    params![run_id, idempotency_key],
                    |row| row.get(0),
                )
                .map_err(AdkStoreError::Query)?;
            let invocation_affected = transaction
                .execute(
                    "UPDATE adk_tool_invocations
                     SET status = 'RUNNING', owner_id = ?1, fencing_token = ?2,
                         run_lease_token = ?3, lease_expires_at_unix_ms = ?4,
                         updated_at = ?5
                     WHERE run_id = ?6 AND idempotency_key = ?7
                       AND status = 'RUNNING' AND lease_expires_at_unix_ms <= ?8",
                    params![
                        owner_id,
                        fencing_token.saturating_add(1),
                        run_lease_token,
                        expires_ms as i64,
                        now_text,
                        run_id,
                        idempotency_key,
                        now_ms as i64,
                    ],
                )
                .map_err(AdkStoreError::Query)?;
            if invocation_affected != 1 {
                return Err(AdkStoreError::Conflict(format!(
                    "tool invocation {idempotency_key} lease is no longer current"
                )));
            }
            transaction.commit().map_err(AdkStoreError::Query)?;
            return Ok(AdkToolInvocationClaim::Execute(StoredAdkToolInvocation {
                owner_id: owner_id.to_owned(),
                fencing_token: fencing_token.saturating_add(1),
                run_lease_token,
                lease_expires_at_unix_ms: expires_ms as i64,
                updated_at: now_text,
                ..existing
            }));
        }
        transaction
            .execute(
                "INSERT INTO adk_tool_invocations
                 (run_id, idempotency_key, tool_name, status, owner_id,
                  fencing_token, run_lease_token, input_json, output_json,
                  lease_expires_at_unix_ms, created_at, updated_at)
                 VALUES (?1, ?2, ?3, 'RUNNING', ?4, 1, ?5, ?6, 'null', ?7, ?8, ?8)",
                params![
                    run_id,
                    idempotency_key,
                    tool_name,
                    owner_id,
                    run_lease_token,
                    input_json,
                    expires_ms as i64,
                    now_text
                ],
            )
            .map_err(AdkStoreError::Query)?;
        transaction.commit().map_err(AdkStoreError::Query)?;
        Ok(AdkToolInvocationClaim::Execute(StoredAdkToolInvocation {
            run_id: run_id.to_owned(),
            idempotency_key: idempotency_key.to_owned(),
            tool_name: tool_name.to_owned(),
            status: "RUNNING".to_owned(),
            owner_id: owner_id.to_owned(),
            fencing_token: 1,
            run_lease_token,
            lease_expires_at_unix_ms: expires_ms as i64,
            input_json: input_json.to_owned(),
            output_json: "null".to_owned(),
            created_at: now_text.clone(),
            updated_at: now_text,
        }))
    }

    /// Atomically append one tool result to the run projection, invocation
    /// ledger and ADK session event journal.  The run status and updated_at
    /// token form the CAS fence; if another continuation or cancellation won
    /// the race no invocation/result is written.  Existing terminal rows are
    /// replayed without executing the tool again.
    pub fn commit_tool_result_if_status_and_revision_with_event(
        &self,
        id: &str,
        expected_status: &str,
        expected_updated_at: &str,
        payload_json: &str,
        idempotency_key: &str,
        tool_name: &str,
        input_json: &str,
        output_json: &str,
        status: &str,
        owner_id: &str,
        fencing_token: i64,
        run_lease_token: i64,
        session_db_path: &Path,
        event: &AdkRunEvent<'_>,
    ) -> Result<AdkToolResultCommit, AdkStoreError> {
        let status = status.trim().to_ascii_uppercase();
        if !matches!(status.as_str(), "SUCCEEDED" | "FAILED" | "UNKNOWN") {
            return Err(AdkStoreError::Validation(
                "tool invocation status must be SUCCEEDED, FAILED or UNKNOWN".to_owned(),
            ));
        }
        if id.trim().is_empty() || idempotency_key.trim().is_empty() || tool_name.trim().is_empty()
        {
            return Err(AdkStoreError::Validation(
                "tool invocation identity is required".to_owned(),
            ));
        }
        // Validate opaque JSON fields before opening the transaction so a
        // malformed result can never be persisted as a successful execution.
        serde_json::from_str::<Value>(input_json).map_err(|error| {
            AdkStoreError::Validation(format!("invalid tool input JSON: {error}"))
        })?;
        serde_json::from_str::<Value>(output_json).map_err(|error| {
            AdkStoreError::Validation(format!("invalid tool output JSON: {error}"))
        })?;

        let mut connection = self.lock_connection()?;
        attach_adk_session_database(&connection, session_db_path)?;
        let result = (|| {
            let now = Self::now_rfc3339();
            let transaction = connection
                .transaction_with_behavior(TransactionBehavior::Immediate)
                .map_err(AdkStoreError::Query)?;
            let existing = transaction
                .query_row(
                    "SELECT run_id, idempotency_key, tool_name, status, owner_id,
                            fencing_token, run_lease_token, input_json, output_json,
                            lease_expires_at_unix_ms, created_at, updated_at
                     FROM adk_tool_invocations
                     WHERE run_id = ?1 AND idempotency_key = ?2",
                    params![id, idempotency_key],
                    stored_tool_invocation,
                )
                .optional()
                .map_err(AdkStoreError::Query)?;
            if let Some(existing) = existing.as_ref() {
                if existing.tool_name != tool_name || existing.input_json != input_json {
                    return Err(AdkStoreError::Conflict(format!(
                        "tool invocation key {idempotency_key} was reused with different input"
                    )));
                }
                if matches!(
                    existing.status.to_ascii_uppercase().as_str(),
                    "SUCCEEDED" | "FAILED" | "UNKNOWN"
                ) {
                    transaction.commit().map_err(AdkStoreError::Query)?;
                    return Ok(AdkToolResultCommit {
                        changed: false,
                        invocation: existing.clone(),
                    });
                }
            }

            let Some(invocation) = existing.as_ref() else {
                return Err(AdkStoreError::Conflict(format!(
                    "tool invocation {idempotency_key} must be claimed before result commit"
                )));
            };
            if invocation.run_lease_token != run_lease_token {
                return Err(AdkStoreError::Conflict(format!(
                    "tool invocation {idempotency_key} belongs to run lease token {}, not {run_lease_token}",
                    invocation.run_lease_token
                )));
            }
            let lease_valid = transaction
                .query_row(
                    "SELECT 1 FROM adk_run_leases
                     WHERE run_id = ?1 AND owner_id = ?2 AND fencing_token = ?3
                       AND expires_at_unix_ms > ?4",
                    params![
                        id,
                        invocation.owner_id,
                        invocation.run_lease_token,
                        (OffsetDateTime::now_utc().unix_timestamp_nanos() / 1_000_000) as i64
                    ],
                    |row| row.get::<_, i64>(0),
                )
                .optional()
                .map_err(AdkStoreError::Query)?
                .is_some();
            if !lease_valid {
                return Err(AdkStoreError::Conflict(format!(
                    "run {id} lease fencing token {} is no longer current",
                    invocation.run_lease_token
                )));
            }

            let affected = transaction
                .execute(
                    "UPDATE adk_runs
                     SET payload_json = ?1, updated_at = ?2
                     WHERE id = ?3 AND status = ?4 AND updated_at = ?5",
                    params![payload_json, now, id, expected_status, expected_updated_at],
                )
                .map_err(AdkStoreError::Query)?;
            if affected != 1 {
                return Err(AdkStoreError::Conflict(format!(
                    "run {id} changed before tool result commit"
                )));
            }
            let invocation_affected = transaction
                .execute(
                    "INSERT INTO adk_tool_invocations
                     (run_id, idempotency_key, tool_name, status, owner_id,
                      fencing_token, run_lease_token, input_json, output_json,
                      lease_expires_at_unix_ms, created_at, updated_at)
                     VALUES (?1, ?2, ?3, ?4, ?8, ?9, ?10, ?5, ?6, 0, ?7, ?7)
                     ON CONFLICT(run_id, idempotency_key) DO UPDATE SET
                       tool_name = excluded.tool_name,
                       status = excluded.status,
                       input_json = excluded.input_json,
                       output_json = excluded.output_json,
                       owner_id = excluded.owner_id,
                       run_lease_token = excluded.run_lease_token,
                       updated_at = excluded.updated_at
                     WHERE adk_tool_invocations.owner_id = ?8
                       AND adk_tool_invocations.fencing_token = ?9
                       AND adk_tool_invocations.status = 'RUNNING'",
                    params![
                        id,
                        idempotency_key,
                        tool_name,
                        status,
                        input_json,
                        output_json,
                        now,
                        owner_id,
                        fencing_token,
                        invocation.run_lease_token
                    ],
                )
                .map_err(AdkStoreError::Query)?;
            if invocation_affected != 1 {
                return Err(AdkStoreError::Conflict(format!(
                    "tool invocation {idempotency_key} lease is no longer current"
                )));
            }
            append_adk_session_events(&transaction, &now, id, std::slice::from_ref(event))?;
            transaction.commit().map_err(AdkStoreError::Query)?;
            Ok(AdkToolResultCommit {
                changed: true,
                invocation: StoredAdkToolInvocation {
                    run_id: id.to_owned(),
                    idempotency_key: idempotency_key.to_owned(),
                    tool_name: tool_name.to_owned(),
                    status,
                    owner_id: owner_id.to_owned(),
                    fencing_token,
                    run_lease_token: invocation.run_lease_token,
                    lease_expires_at_unix_ms: 0,
                    input_json: input_json.to_owned(),
                    output_json: output_json.to_owned(),
                    created_at: now.clone(),
                    updated_at: now,
                },
            })
        })();
        let detach = connection
            .execute("DETACH DATABASE adk_session_events", [])
            .map_err(AdkStoreError::Query);
        match (result, detach) {
            (Err(error), _) => Err(error),
            (Ok(_), Err(error)) => Err(error),
            (Ok(value), Ok(_)) => Ok(value),
        }
    }

    /// Atomically stage a model tool-call round: update the run projection,
    /// create all approval rows and append the corresponding session events.
    /// A failed insert (including a duplicate approval id) rolls back the
    /// complete round so no half-visible approval can be resumed.
    pub fn stage_tool_calls_if_status_and_revision_with_events(
        &self,
        id: &str,
        expected_status: &str,
        expected_updated_at: &str,
        status: &str,
        payload_json: &str,
        approvals: &[AdkApprovalStage<'_>],
        session_db_path: &Path,
        events: &[AdkRunEvent<'_>],
    ) -> Result<bool, AdkStoreError> {
        self.stage_tool_calls_if_status_and_revision_with_events_inner(
            id,
            expected_status,
            expected_updated_at,
            status,
            payload_json,
            approvals,
            session_db_path,
            events,
            None,
        )
    }

    /// Atomically stage a tool-call round while fencing it to the current
    /// durable run lease.  The lease check is part of the same SQLite
    /// transaction as the run/approval/event writes, so an expired worker
    /// cannot commit a late approval after another worker takes over.
    pub fn stage_tool_calls_if_status_and_revision_with_events_with_lease(
        &self,
        id: &str,
        expected_status: &str,
        expected_updated_at: &str,
        status: &str,
        payload_json: &str,
        approvals: &[AdkApprovalStage<'_>],
        session_db_path: &Path,
        events: &[AdkRunEvent<'_>],
        owner_id: &str,
        run_lease_token: i64,
    ) -> Result<bool, AdkStoreError> {
        if owner_id.trim().is_empty() || run_lease_token <= 0 {
            return Err(AdkStoreError::Validation(
                "tool-call staging requires a valid run lease owner and token".to_owned(),
            ));
        }
        self.stage_tool_calls_if_status_and_revision_with_events_inner(
            id,
            expected_status,
            expected_updated_at,
            status,
            payload_json,
            approvals,
            session_db_path,
            events,
            Some((owner_id, run_lease_token)),
        )
    }

    fn stage_tool_calls_if_status_and_revision_with_events_inner(
        &self,
        id: &str,
        expected_status: &str,
        expected_updated_at: &str,
        status: &str,
        payload_json: &str,
        approvals: &[AdkApprovalStage<'_>],
        session_db_path: &Path,
        events: &[AdkRunEvent<'_>],
        lease: Option<(&str, i64)>,
    ) -> Result<bool, AdkStoreError> {
        let mut connection = self.lock_connection()?;
        attach_adk_session_database(&connection, session_db_path)?;
        let result = (|| {
            let now = Self::now_rfc3339();
            let transaction = connection
                .transaction_with_behavior(TransactionBehavior::Immediate)
                .map_err(AdkStoreError::Query)?;
            if let Some((owner_id, run_lease_token)) = lease {
                let now_ms = (OffsetDateTime::now_utc().unix_timestamp_nanos() / 1_000_000) as i64;
                let lease_valid = transaction
                    .query_row(
                        "SELECT 1 FROM adk_run_leases
                         WHERE run_id = ?1 AND owner_id = ?2 AND fencing_token = ?3
                           AND expires_at_unix_ms > ?4",
                        params![id, owner_id, run_lease_token, now_ms],
                        |row| row.get::<_, i64>(0),
                    )
                    .optional()
                    .map_err(AdkStoreError::Query)?
                    .is_some();
                if !lease_valid {
                    return Err(AdkStoreError::Conflict(format!(
                        "run {id} lease fencing token {run_lease_token} is no longer current"
                    )));
                }
            }
            let affected = transaction
                .execute(
                    "UPDATE adk_runs
                     SET status = ?1, payload_json = ?2, updated_at = ?3
                     WHERE id = ?4 AND status = ?5 AND updated_at = ?6",
                    params![
                        status,
                        payload_json,
                        now,
                        id,
                        expected_status,
                        expected_updated_at
                    ],
                )
                .map_err(AdkStoreError::Query)?;
            if affected != 1 {
                transaction.commit().map_err(AdkStoreError::Query)?;
                return Ok(false);
            }
            for approval in approvals {
                transaction
                    .execute(
                        "INSERT INTO adk_approvals
                         (id, run_id, agent_id, status, payload_json, created_at, updated_at)
                         VALUES (?1, ?2, ?3, 'PENDING', ?4, ?5, ?5)",
                        params![
                            approval.id,
                            approval.run_id,
                            approval.agent_id,
                            approval.payload_json,
                            now
                        ],
                    )
                    .map_err(AdkStoreError::Query)?;
            }
            append_adk_session_events(&transaction, &now, id, events)?;
            transaction.commit().map_err(AdkStoreError::Query)?;
            Ok(true)
        })();
        let detach = connection
            .execute("DETACH DATABASE adk_session_events", [])
            .map_err(AdkStoreError::Query);
        match (result, detach) {
            (Err(error), _) => Err(error),
            (Ok(_), Err(error)) => Err(error),
            (Ok(value), Ok(_)) => Ok(value),
        }
    }

    // --- Approvals ---
    pub fn create_approval(
        &self,
        id: &str,
        run_id: &str,
        agent_id: &str,
        status: &str,
        payload_json: &str,
    ) -> Result<StoredAdkApproval, AdkStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        connection
            .execute(
                "INSERT INTO adk_approvals (id, run_id, agent_id, status, payload_json, created_at, updated_at)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?6)",
                params![id, run_id, agent_id, status, payload_json, now],
            )
            .map_err(AdkStoreError::Query)?;

        Ok(StoredAdkApproval {
            id: id.to_owned(),
            run_id: run_id.to_owned(),
            agent_id: agent_id.to_owned(),
            status: status.to_owned(),
            payload_json: payload_json.to_owned(),
            created_at: now.clone(),
            updated_at: now,
        })
    }

    pub fn update_approval_status(&self, id: &str, status: &str) -> Result<bool, AdkStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        let affected = connection
            .execute(
                "UPDATE adk_approvals SET status = ?1, updated_at = ?2 WHERE id = ?3",
                params![status, now, id],
            )
            .map_err(AdkStoreError::Query)?;
        Ok(affected > 0)
    }

    /// Resolve an approval and, when it belongs to a pending run, atomically
    /// merge the authoritative approval rows into that run.  This mirrors the
    /// Go store's ResolveAndStageApproval boundary while keeping all writes in
    /// the Rust-owned ADK database lease.
    pub fn resolve_and_stage_approval(
        &self,
        id: &str,
        status: &str,
    ) -> Result<Option<AdkApprovalResolution>, AdkStoreError> {
        let status = status.trim().to_ascii_uppercase();
        if !matches!(status.as_str(), "APPROVED" | "DENIED") {
            return Err(AdkStoreError::Validation(
                "approval status must be APPROVED or DENIED".to_owned(),
            ));
        }
        let now = Self::now_rfc3339();
        let mut connection = self.lock_connection()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(AdkStoreError::Query)?;
        let Some(mut approval) = transaction
            .query_row(
                "SELECT id, run_id, agent_id, status, payload_json, created_at, updated_at
                 FROM adk_approvals WHERE id = ?1",
                params![id],
                stored_approval,
            )
            .optional()
            .map_err(AdkStoreError::Query)?
        else {
            transaction.commit().map_err(AdkStoreError::Query)?;
            return Ok(None);
        };

        let mut changed = false;
        if approval.status.trim().eq_ignore_ascii_case("PENDING") {
            let mut payload = decode_json_object(&approval.payload_json, "approval")?;
            payload.insert("status".to_owned(), Value::String(status.clone()));
            payload.insert("updatedAt".to_owned(), Value::String(now.clone()));
            let payload_json = serde_json::to_string(&Value::Object(payload))
                .map_err(|error| AdkStoreError::Validation(error.to_string()))?;
            let affected = transaction
                .execute(
                    "UPDATE adk_approvals
                     SET status = ?1, payload_json = ?2, updated_at = ?3
                     WHERE id = ?4 AND status = 'PENDING' AND updated_at = ?5",
                    params![status, payload_json, now, id, approval.updated_at],
                )
                .map_err(AdkStoreError::Query)?;
            if affected == 1 {
                approval.status = status.clone();
                approval.payload_json = payload_json;
                approval.updated_at = now.clone();
                changed = true;
            }
        }

        if !approval.status.trim().eq_ignore_ascii_case(&status) {
            transaction.commit().map_err(AdkStoreError::Query)?;
            return Ok(Some(AdkApprovalResolution {
                approval,
                changed,
                run: None,
                should_continue: false,
            }));
        }

        let Some(mut run) = transaction
            .query_row(
                "SELECT id, session_id, agent_id, status, client_request_id, request_fingerprint,
                        payload_json, created_at, updated_at
                 FROM adk_runs WHERE id = ?1",
                params![approval.run_id],
                stored_run,
            )
            .optional()
            .map_err(AdkStoreError::Query)?
        else {
            transaction.commit().map_err(AdkStoreError::Query)?;
            return Ok(Some(AdkApprovalResolution {
                approval,
                changed,
                run: None,
                should_continue: false,
            }));
        };
        if !run.status.trim().eq_ignore_ascii_case("PENDING") {
            transaction.commit().map_err(AdkStoreError::Query)?;
            return Ok(Some(AdkApprovalResolution {
                approval,
                changed,
                run: None,
                should_continue: false,
            }));
        }

        let mut run_payload = decode_json_object(&run.payload_json, "run")?;
        if !matches!(run_payload.get("pendingApprovals"), Some(Value::Array(_))) {
            transaction.commit().map_err(AdkStoreError::Query)?;
            return Ok(Some(AdkApprovalResolution {
                approval,
                changed,
                run: Some(run),
                should_continue: false,
            }));
        }
        let authoritative = {
            let mut statement = transaction
                .prepare(
                    "SELECT id, run_id, agent_id, status, payload_json, created_at, updated_at
                     FROM adk_approvals WHERE run_id = ?1",
                )
                .map_err(AdkStoreError::Query)?;
            let rows = statement
                .query_map(params![approval.run_id], stored_approval)
                .map_err(AdkStoreError::Query)?;
            rows.collect::<Result<Vec<_>, _>>()
                .map_err(AdkStoreError::Query)?
        };
        let mut authoritative_values = BTreeMap::new();
        for item in &authoritative {
            authoritative_values.insert(item.id.clone(), stored_approval_value(item)?);
        }

        let (replaced, denied) = {
            let pending_approvals = run_payload
                .get_mut("pendingApprovals")
                .and_then(Value::as_array_mut)
                .expect("pendingApprovals was checked as an array");
            let mut replaced = false;
            let mut denied = false;
            for item in pending_approvals.iter_mut() {
                let item_id = item
                    .as_object()
                    .and_then(|item_object| item_object.get("id"))
                    .and_then(Value::as_str)
                    .unwrap_or_default()
                    .to_owned();
                if let Some(authoritative_value) = authoritative_values.get(&item_id) {
                    *item = authoritative_value.clone();
                }
                replaced = replaced || item_id == approval.id;
                denied = denied
                    || item
                        .get("status")
                        .and_then(Value::as_str)
                        .is_some_and(|value| value.eq_ignore_ascii_case("DENIED"));
            }
            (replaced, denied)
        };
        if !replaced {
            transaction.commit().map_err(AdkStoreError::Query)?;
            return Ok(Some(AdkApprovalResolution {
                approval,
                changed,
                run: Some(run),
                should_continue: false,
            }));
        }

        if denied {
            {
                let pending_approvals = run_payload
                    .get_mut("pendingApprovals")
                    .and_then(Value::as_array_mut)
                    .expect("pendingApprovals was checked as an array");
                for item in pending_approvals.iter_mut() {
                    let Some(item_object) = item.as_object_mut() else {
                        continue;
                    };
                    let item_id = item_object
                        .get("id")
                        .and_then(Value::as_str)
                        .unwrap_or_default()
                        .to_owned();
                    let item_status = item_object
                        .get("status")
                        .and_then(Value::as_str)
                        .unwrap_or_default()
                        .to_owned();
                    if !item_status.eq_ignore_ascii_case("PENDING") {
                        continue;
                    }
                    let updated_at = Self::now_rfc3339();
                    item_object.insert("status".to_owned(), Value::String("DENIED".to_owned()));
                    item_object.insert("updatedAt".to_owned(), Value::String(updated_at.clone()));
                    let payload_json = serde_json::to_string(item)
                        .map_err(|error| AdkStoreError::Validation(error.to_string()))?;
                    transaction
                        .execute(
                            "UPDATE adk_approvals
                             SET status = 'DENIED', payload_json = ?1, updated_at = ?2
                             WHERE id = ?3 AND status = 'PENDING'",
                            params![payload_json, updated_at, item_id],
                        )
                        .map_err(AdkStoreError::Query)?;
                }
            }
            if let Some(Value::Array(tool_calls)) = run_payload.get_mut("toolCalls") {
                for tool_call in tool_calls {
                    let Some(tool_call) = tool_call.as_object_mut() else {
                        continue;
                    };
                    let status = tool_call
                        .get("status")
                        .and_then(Value::as_str)
                        .unwrap_or_default();
                    if matches!(status, "PENDING_APPROVAL" | "pending_approval" | "PENDING") {
                        tool_call.insert("status".to_owned(), Value::String("DENIED".to_owned()));
                        tool_call.insert("requiresUser".to_owned(), Value::Bool(false));
                        tool_call.insert("completedAt".to_owned(), Value::String(now.clone()));
                    }
                }
            }
        }

        let has_pending = run_payload
            .get("pendingApprovals")
            .and_then(Value::as_array)
            .is_some_and(|pending_approvals| {
                pending_approvals.iter().any(|item| {
                    item.get("status")
                        .and_then(Value::as_str)
                        .is_some_and(|value| value.eq_ignore_ascii_case("PENDING"))
                })
            });
        let should_continue = !has_pending;
        let next_status = if should_continue {
            "RUNNING"
        } else {
            "PENDING"
        };
        run_payload.insert("status".to_owned(), Value::String(next_status.to_owned()));
        run_payload.insert("updatedAt".to_owned(), Value::String(now.clone()));
        if should_continue {
            run_payload.insert("startedAt".to_owned(), Value::String(now.clone()));
            run_payload.insert(
                "resumeState".to_owned(),
                Value::String("approval_resuming".to_owned()),
            );
            run_payload.insert(
                "message".to_owned(),
                Value::String(if denied {
                    "审批已拒绝，正在后台结束运行。".to_owned()
                } else {
                    "审批已通过，正在后台继续执行。".to_owned()
                }),
            );
            if !denied && let Some(Value::Array(tool_calls)) = run_payload.get_mut("toolCalls") {
                for tool_call in tool_calls {
                    let Some(tool_call) = tool_call.as_object_mut() else {
                        continue;
                    };
                    if tool_call
                        .get("status")
                        .and_then(Value::as_str)
                        .is_some_and(|value| value.eq_ignore_ascii_case("PENDING_APPROVAL"))
                    {
                        tool_call.insert("status".to_owned(), Value::String("RUNNING".to_owned()));
                        tool_call.insert("requiresUser".to_owned(), Value::Bool(false));
                        tool_call.insert("updatedAt".to_owned(), Value::String(now.clone()));
                    }
                }
            }
        }
        let payload_json = serde_json::to_string(&Value::Object(run_payload))
            .map_err(|error| AdkStoreError::Validation(error.to_string()))?;
        let affected = transaction
            .execute(
                "UPDATE adk_runs
                 SET status = ?1, payload_json = ?2, updated_at = ?3
                 WHERE id = ?4 AND status = 'PENDING' AND updated_at = ?5",
                params![next_status, payload_json, now, run.id, run.updated_at],
            )
            .map_err(AdkStoreError::Query)?;
        if affected != 1 {
            return Err(AdkStoreError::Conflict(format!(
                "approval continuation for run {} was changed concurrently",
                run.id
            )));
        }
        run.status = next_status.to_owned();
        run.payload_json = payload_json;
        run.updated_at = now;
        transaction.commit().map_err(AdkStoreError::Query)?;
        Ok(Some(AdkApprovalResolution {
            approval,
            changed,
            run: Some(run),
            should_continue,
        }))
    }

    // --- Memory ---
    pub fn upsert_memory(
        &self,
        id: &str,
        agent_id: &str,
        scope: &str,
        memory_key: &str,
        payload_json: &str,
    ) -> Result<StoredAdkMemory, AdkStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        let existing = connection
            .query_row(
                "SELECT id, created_at FROM adk_memory
                 WHERE agent_id = ?1 AND scope = ?2 AND memory_key = ?3",
                params![agent_id, scope, memory_key],
                |row| Ok((row.get::<_, String>(0)?, row.get::<_, String>(1)?)),
            )
            .optional()
            .map_err(AdkStoreError::Query)?;
        let created_at = existing
            .as_ref()
            .map(|(_, created_at)| created_at.clone())
            .unwrap_or_else(|| now.clone());
        connection
            .execute(
                "INSERT INTO adk_memory (id, agent_id, scope, memory_key, payload_json, created_at, updated_at)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)
                 ON CONFLICT(agent_id, scope, memory_key) DO UPDATE SET
                   payload_json = ?5, updated_at = ?7",
                params![id, agent_id, scope, memory_key, payload_json, created_at, now],
            )
            .map_err(AdkStoreError::Query)?;

        let persisted = connection
            .query_row(
                "SELECT id, agent_id, scope, memory_key, payload_json, created_at, updated_at
                 FROM adk_memory WHERE agent_id = ?1 AND scope = ?2 AND memory_key = ?3",
                params![agent_id, scope, memory_key],
                |row| {
                    Ok(StoredAdkMemory {
                        id: row.get(0)?,
                        agent_id: row.get(1)?,
                        scope: row.get(2)?,
                        memory_key: row.get(3)?,
                        payload_json: row.get(4)?,
                        created_at: row.get(5)?,
                        updated_at: row.get(6)?,
                    })
                },
            )
            .map_err(AdkStoreError::Query)?;

        Ok(persisted)
    }

    pub fn delete_memory(&self, id: &str) -> Result<bool, AdkStoreError> {
        let connection = self.lock_connection()?;
        let affected = connection
            .execute("DELETE FROM adk_memory WHERE id = ?1", params![id])
            .map_err(AdkStoreError::Query)?;
        Ok(affected > 0)
    }

    pub fn get_memory(&self, id: &str) -> Result<Option<StoredAdkMemory>, AdkStoreError> {
        let connection = self.lock_connection()?;
        connection
            .query_row(
                "SELECT id, agent_id, scope, memory_key, payload_json, created_at, updated_at
                 FROM adk_memory WHERE id = ?1",
                params![id],
                |row| {
                    Ok(StoredAdkMemory {
                        id: row.get(0)?,
                        agent_id: row.get(1)?,
                        scope: row.get(2)?,
                        memory_key: row.get(3)?,
                        payload_json: row.get(4)?,
                        created_at: row.get(5)?,
                        updated_at: row.get(6)?,
                    })
                },
            )
            .optional()
            .map_err(AdkStoreError::Query)
    }

    // --- Workflows ---
    pub fn upsert_workflow(
        &self,
        id: &str,
        status: &str,
        payload_json: &str,
    ) -> Result<StoredAdkWorkflow, AdkStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        let created_at = connection
            .query_row(
                "SELECT created_at FROM adk_workflows WHERE id = ?1",
                params![id],
                |row| row.get::<_, String>(0),
            )
            .optional()
            .map_err(AdkStoreError::Query)?
            .unwrap_or_else(|| now.clone());
        connection
            .execute(
                "INSERT INTO adk_workflows (id, status, payload_json, created_at, updated_at)
                 VALUES (?1, ?2, ?3, ?4, ?5)
                 ON CONFLICT(id) DO UPDATE SET status = ?2, payload_json = ?3, updated_at = ?5",
                params![id, status, payload_json, created_at, now],
            )
            .map_err(AdkStoreError::Query)?;

        Ok(StoredAdkWorkflow {
            id: id.to_owned(),
            status: status.to_owned(),
            payload_json: payload_json.to_owned(),
            created_at,
            updated_at: now,
        })
    }

    /// Update a workflow only when the caller still owns the persisted
    /// `updated_at` revision token.  ADK workflows predate an integer
    /// revision column; the timestamp is therefore the opaque CAS token used
    /// by the production mutation adapter.
    pub fn update_workflow_if_revision(
        &self,
        id: &str,
        expected_updated_at: &str,
        status: &str,
        payload_json: &str,
    ) -> Result<bool, AdkStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        let affected = connection
            .execute(
                "UPDATE adk_workflows
                 SET status = ?1, payload_json = ?2, updated_at = ?3
                 WHERE id = ?4 AND updated_at = ?5",
                params![status, payload_json, now, id, expected_updated_at],
            )
            .map_err(AdkStoreError::Query)?;
        Ok(affected == 1)
    }

    /// Atomically soft-delete a workflow and all of its non-deleted triggers.
    /// The workflow revision is the fence for the whole operation, so a
    /// concurrent workflow update cannot leave child triggers half deleted.
    pub fn soft_delete_workflow_if_revision(
        &self,
        id: &str,
        expected_updated_at: &str,
        payload_json: &str,
        deleted_at: &str,
    ) -> Result<bool, AdkStoreError> {
        let now = Self::now_rfc3339();
        let mut connection = self.lock_connection()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(AdkStoreError::Query)?;
        let affected = transaction
            .execute(
                "UPDATE adk_workflows
                 SET status = 'DISABLED', payload_json = ?1, updated_at = ?2
                 WHERE id = ?3 AND updated_at = ?4",
                params![payload_json, now, id, expected_updated_at],
            )
            .map_err(AdkStoreError::Query)?;
        if affected == 1 {
            transaction
                .execute(
                    "UPDATE adk_workflow_triggers
                     SET status = 'DISABLED',
                         payload_json = json_set(payload_json,
                             '$.status', 'DISABLED', '$.deletedAt', ?1),
                         updated_at = ?2
                     WHERE workflow_id = ?3
                       AND COALESCE(json_extract(payload_json, '$.deletedAt'), '') = ''",
                    params![deleted_at, now, id],
                )
                .map_err(AdkStoreError::Query)?;
        }
        transaction.commit().map_err(AdkStoreError::Query)?;
        Ok(affected == 1)
    }

    pub fn delete_workflow(&self, id: &str) -> Result<bool, AdkStoreError> {
        let connection = self.lock_connection()?;
        let affected = connection
            .execute("DELETE FROM adk_workflows WHERE id = ?1", params![id])
            .map_err(AdkStoreError::Query)?;
        Ok(affected > 0)
    }

    pub fn record_audit_event(
        &self,
        id: &str,
        kind: &str,
        subject_id: &str,
        payload_json: &str,
    ) -> Result<(), AdkStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        connection
            .execute(
                "INSERT INTO adk_audit_events (id, kind, subject_id, payload_json, created_at)
                 VALUES (?1, ?2, ?3, ?4, ?5)",
                params![id, kind, subject_id, payload_json, now],
            )
            .map_err(AdkStoreError::Query)?;
        Ok(())
    }

    pub fn list_providers(&self) -> Result<Vec<StoredAdkEntity>, AdkStoreError> {
        let connection = self.lock_connection()?;
        let mut statement = connection
            .prepare(
                "SELECT id, payload_json, created_at, updated_at
                 FROM adk_providers ORDER BY created_at DESC",
            )
            .map_err(AdkStoreError::Query)?;
        let rows = statement
            .query_map([], |row| {
                Ok(StoredAdkEntity {
                    id: row.get(0)?,
                    payload_json: row.get(1)?,
                    created_at: row.get(2)?,
                    updated_at: row.get(3)?,
                })
            })
            .map_err(AdkStoreError::Query)?;
        let mut result = Vec::new();
        for row in rows {
            result.push(row.map_err(AdkStoreError::Query)?);
        }
        Ok(result)
    }

    pub fn list_agents(&self) -> Result<Vec<StoredAdkEntity>, AdkStoreError> {
        let connection = self.lock_connection()?;
        let mut statement = connection
            .prepare(
                "SELECT id, payload_json, created_at, updated_at
                 FROM adk_agents ORDER BY created_at DESC",
            )
            .map_err(AdkStoreError::Query)?;
        let rows = statement
            .query_map([], |row| {
                Ok(StoredAdkEntity {
                    id: row.get(0)?,
                    payload_json: row.get(1)?,
                    created_at: row.get(2)?,
                    updated_at: row.get(3)?,
                })
            })
            .map_err(AdkStoreError::Query)?;
        let mut result = Vec::new();
        for row in rows {
            result.push(row.map_err(AdkStoreError::Query)?);
        }
        Ok(result)
    }

    pub fn list_workflows(&self) -> Result<Vec<StoredAdkWorkflow>, AdkStoreError> {
        let connection = self.lock_connection()?;
        let mut statement = connection
            .prepare(
                "SELECT id, status, payload_json, created_at, updated_at
                 FROM adk_workflows ORDER BY created_at DESC",
            )
            .map_err(AdkStoreError::Query)?;
        let rows = statement
            .query_map([], |row| {
                Ok(StoredAdkWorkflow {
                    id: row.get(0)?,
                    status: row.get(1)?,
                    payload_json: row.get(2)?,
                    created_at: row.get(3)?,
                    updated_at: row.get(4)?,
                })
            })
            .map_err(AdkStoreError::Query)?;
        let mut result = Vec::new();
        for row in rows {
            result.push(row.map_err(AdkStoreError::Query)?);
        }
        Ok(result)
    }

    pub fn list_approvals(&self) -> Result<Vec<StoredAdkApproval>, AdkStoreError> {
        let connection = self.lock_connection()?;
        let mut statement = connection
            .prepare(
                "SELECT id, run_id, agent_id, status, payload_json, created_at, updated_at
                 FROM adk_approvals ORDER BY created_at DESC",
            )
            .map_err(AdkStoreError::Query)?;
        let rows = statement
            .query_map([], |row| {
                Ok(StoredAdkApproval {
                    id: row.get(0)?,
                    run_id: row.get(1)?,
                    agent_id: row.get(2)?,
                    status: row.get(3)?,
                    payload_json: row.get(4)?,
                    created_at: row.get(5)?,
                    updated_at: row.get(6)?,
                })
            })
            .map_err(AdkStoreError::Query)?;
        let mut result = Vec::new();
        for row in rows {
            result.push(row.map_err(AdkStoreError::Query)?);
        }
        Ok(result)
    }

    pub fn list_memories(&self) -> Result<Vec<StoredAdkMemory>, AdkStoreError> {
        let connection = self.lock_connection()?;
        let mut statement = connection
            .prepare(
                "SELECT id, agent_id, scope, memory_key, payload_json, created_at, updated_at
                 FROM adk_memory ORDER BY created_at DESC",
            )
            .map_err(AdkStoreError::Query)?;
        let rows = statement
            .query_map([], |row| {
                Ok(StoredAdkMemory {
                    id: row.get(0)?,
                    agent_id: row.get(1)?,
                    scope: row.get(2)?,
                    memory_key: row.get(3)?,
                    payload_json: row.get(4)?,
                    created_at: row.get(5)?,
                    updated_at: row.get(6)?,
                })
            })
            .map_err(AdkStoreError::Query)?;
        let mut result = Vec::new();
        for row in rows {
            result.push(row.map_err(AdkStoreError::Query)?);
        }
        Ok(result)
    }

    pub fn list_runs(&self) -> Result<Vec<StoredAdkRun>, AdkStoreError> {
        let connection = self.lock_connection()?;
        let mut statement = connection
            .prepare(
                "SELECT id, session_id, agent_id, status, client_request_id, request_fingerprint, payload_json, created_at, updated_at
                 FROM adk_runs ORDER BY created_at DESC",
            )
            .map_err(AdkStoreError::Query)?;
        let rows = statement
            .query_map([], stored_run)
            .map_err(AdkStoreError::Query)?;
        collect_rows(rows)
    }

    pub fn get_run(&self, id: &str) -> Result<Option<StoredAdkRun>, AdkStoreError> {
        let connection = self.lock_connection()?;
        connection
            .query_row(
                "SELECT id, session_id, agent_id, status, client_request_id, request_fingerprint, payload_json, created_at, updated_at
                 FROM adk_runs WHERE id = ?1",
                params![id],
                stored_run,
            )
            .optional()
            .map_err(AdkStoreError::Query)
    }

    pub fn upsert_skill(
        &self,
        id: &str,
        payload_json: &str,
    ) -> Result<StoredAdkEntity, AdkStoreError> {
        self.upsert_simple_entity("adk_skills", id, payload_json)
    }

    pub fn delete_skill(&self, id: &str) -> Result<bool, AdkStoreError> {
        self.delete_simple_entity("adk_skills", id)
    }

    pub fn get_skill(&self, id: &str) -> Result<Option<StoredAdkEntity>, AdkStoreError> {
        self.get_simple_entity("adk_skills", id)
    }

    pub fn list_skills(&self) -> Result<Vec<StoredAdkEntity>, AdkStoreError> {
        self.list_simple_entities("adk_skills")
    }

    pub fn upsert_optimization_task(
        &self,
        id: &str,
        payload_json: &str,
    ) -> Result<StoredAdkEntity, AdkStoreError> {
        self.upsert_simple_entity("adk_optimization_tasks", id, payload_json)
    }

    pub fn update_optimization_task_if_revision(
        &self,
        id: &str,
        expected_updated_at: &str,
        payload_json: &str,
    ) -> Result<bool, AdkStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        let affected = connection
            .execute(
                "UPDATE adk_optimization_tasks
                 SET payload_json = ?1, updated_at = ?2
                 WHERE id = ?3 AND updated_at = ?4",
                params![payload_json, now, id, expected_updated_at],
            )
            .map_err(AdkStoreError::Query)?;
        Ok(affected > 0)
    }

    pub fn get_optimization_task(
        &self,
        id: &str,
    ) -> Result<Option<StoredAdkEntity>, AdkStoreError> {
        self.get_simple_entity("adk_optimization_tasks", id)
    }

    pub fn list_optimization_tasks(&self) -> Result<Vec<StoredAdkEntity>, AdkStoreError> {
        self.list_simple_entities("adk_optimization_tasks")
    }

    pub fn upsert_task(
        &self,
        id: &str,
        status: &str,
        agent_id: &str,
        run_id: &str,
        payload_json: &str,
    ) -> Result<StoredAdkTask, AdkStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        let created_at = connection
            .query_row(
                "SELECT created_at FROM adk_tasks WHERE id = ?1",
                params![id],
                |row| row.get::<_, String>(0),
            )
            .optional()
            .map_err(AdkStoreError::Query)?
            .unwrap_or_else(|| now.clone());
        connection
            .execute(
                "INSERT INTO adk_tasks (id, status, agent_id, run_id, payload_json, created_at, updated_at)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)
                 ON CONFLICT(id) DO UPDATE SET status = ?2, agent_id = ?3, run_id = ?4, payload_json = ?5, updated_at = ?7",
                params![id, status, agent_id, run_id, payload_json, created_at, now],
            )
            .map_err(AdkStoreError::Query)?;
        connection
            .query_row(
                "SELECT id, status, agent_id, run_id, payload_json, created_at, updated_at
                 FROM adk_tasks WHERE id = ?1",
                params![id],
                stored_task,
            )
            .map_err(AdkStoreError::Query)
    }

    /// Update a task projection only when the caller still owns the durable
    /// status/revision snapshot.  The timestamp is the legacy ADK revision
    /// token persisted by this schema.
    pub fn update_task_if_status_and_revision(
        &self,
        id: &str,
        expected_status: &str,
        expected_updated_at: &str,
        status: &str,
        agent_id: &str,
        run_id: &str,
        payload_json: &str,
    ) -> Result<bool, AdkStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        let affected = connection
            .execute(
                "UPDATE adk_tasks
                 SET status = ?1, agent_id = ?2, run_id = ?3, payload_json = ?4, updated_at = ?5
                 WHERE id = ?6 AND status = ?7 AND updated_at = ?8",
                params![
                    status,
                    agent_id,
                    run_id,
                    payload_json,
                    now,
                    id,
                    expected_status,
                    expected_updated_at
                ],
            )
            .map_err(AdkStoreError::Query)?;
        Ok(affected > 0)
    }

    pub fn delete_task(&self, id: &str) -> Result<bool, AdkStoreError> {
        let connection = self.lock_connection()?;
        connection
            .execute("DELETE FROM adk_tasks WHERE id = ?1", params![id])
            .map(|affected| affected > 0)
            .map_err(AdkStoreError::Query)
    }

    pub fn get_task(&self, id: &str) -> Result<Option<StoredAdkTask>, AdkStoreError> {
        let connection = self.lock_connection()?;
        connection
            .query_row(
                "SELECT id, status, agent_id, run_id, payload_json, created_at, updated_at
                 FROM adk_tasks WHERE id = ?1",
                params![id],
                stored_task,
            )
            .optional()
            .map_err(AdkStoreError::Query)
    }

    pub fn list_tasks(&self) -> Result<Vec<StoredAdkTask>, AdkStoreError> {
        let connection = self.lock_connection()?;
        let mut statement = connection
            .prepare(
                "SELECT id, status, agent_id, run_id, payload_json, created_at, updated_at
                 FROM adk_tasks ORDER BY updated_at DESC",
            )
            .map_err(AdkStoreError::Query)?;
        let rows = statement
            .query_map([], stored_task)
            .map_err(AdkStoreError::Query)?;
        collect_rows(rows)
    }

    pub fn get_workflow(&self, id: &str) -> Result<Option<StoredAdkWorkflow>, AdkStoreError> {
        let connection = self.lock_connection()?;
        connection
            .query_row(
                "SELECT id, status, payload_json, created_at, updated_at
                 FROM adk_workflows WHERE id = ?1",
                params![id],
                |row| {
                    Ok(StoredAdkWorkflow {
                        id: row.get(0)?,
                        status: row.get(1)?,
                        payload_json: row.get(2)?,
                        created_at: row.get(3)?,
                        updated_at: row.get(4)?,
                    })
                },
            )
            .optional()
            .map_err(AdkStoreError::Query)
    }

    pub fn upsert_workflow_trigger(
        &self,
        id: &str,
        workflow_id: &str,
        trigger_type: &str,
        status: &str,
        next_run_at: &str,
        payload_json: &str,
    ) -> Result<StoredAdkWorkflowTrigger, AdkStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        let created_at = connection
            .query_row(
                "SELECT created_at FROM adk_workflow_triggers WHERE id = ?1",
                params![id],
                |row| row.get::<_, String>(0),
            )
            .optional()
            .map_err(AdkStoreError::Query)?
            .unwrap_or_else(|| now.clone());
        connection
            .execute(
                "INSERT INTO adk_workflow_triggers (id, workflow_id, trigger_type, status, next_run_at, payload_json, created_at, updated_at)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)
                 ON CONFLICT(id) DO UPDATE SET workflow_id = ?2, trigger_type = ?3, status = ?4, next_run_at = ?5, payload_json = ?6, updated_at = ?8",
                params![id, workflow_id, trigger_type, status, next_run_at, payload_json, created_at, now],
            )
            .map_err(AdkStoreError::Query)?;
        connection
            .query_row(
                "SELECT id, workflow_id, trigger_type, status, next_run_at, payload_json, created_at, updated_at
                 FROM adk_workflow_triggers WHERE id = ?1",
                params![id],
                stored_workflow_trigger,
            )
            .map_err(AdkStoreError::Query)
    }

    /// Update a workflow trigger only when its timestamp revision still
    /// matches the caller's snapshot.
    pub fn update_workflow_trigger_if_revision(
        &self,
        id: &str,
        expected_updated_at: &str,
        workflow_id: &str,
        trigger_type: &str,
        status: &str,
        next_run_at: &str,
        payload_json: &str,
    ) -> Result<bool, AdkStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        let affected = connection
            .execute(
                "UPDATE adk_workflow_triggers
                 SET workflow_id = ?1, trigger_type = ?2, status = ?3,
                     next_run_at = ?4, payload_json = ?5, updated_at = ?6
                 WHERE id = ?7 AND updated_at = ?8",
                params![
                    workflow_id,
                    trigger_type,
                    status,
                    next_run_at,
                    payload_json,
                    now,
                    id,
                    expected_updated_at
                ],
            )
            .map_err(AdkStoreError::Query)?;
        Ok(affected == 1)
    }

    /// Soft-delete one workflow trigger behind its revision fence.
    pub fn soft_delete_workflow_trigger_if_revision(
        &self,
        id: &str,
        expected_updated_at: &str,
        workflow_id: &str,
        payload_json: &str,
    ) -> Result<bool, AdkStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        let affected = connection
            .execute(
                "UPDATE adk_workflow_triggers
                 SET status = 'DISABLED', payload_json = ?1, updated_at = ?2
                 WHERE id = ?3 AND workflow_id = ?4 AND updated_at = ?5",
                params![payload_json, now, id, workflow_id, expected_updated_at],
            )
            .map_err(AdkStoreError::Query)?;
        Ok(affected == 1)
    }

    pub fn delete_workflow_trigger(&self, id: &str) -> Result<bool, AdkStoreError> {
        let connection = self.lock_connection()?;
        connection
            .execute(
                "DELETE FROM adk_workflow_triggers WHERE id = ?1",
                params![id],
            )
            .map(|affected| affected > 0)
            .map_err(AdkStoreError::Query)
    }

    pub fn get_workflow_trigger(
        &self,
        id: &str,
    ) -> Result<Option<StoredAdkWorkflowTrigger>, AdkStoreError> {
        let connection = self.lock_connection()?;
        connection
            .query_row(
                "SELECT id, workflow_id, trigger_type, status, next_run_at, payload_json, created_at, updated_at
                 FROM adk_workflow_triggers WHERE id = ?1",
                params![id],
                stored_workflow_trigger,
            )
            .optional()
            .map_err(AdkStoreError::Query)
    }

    pub fn list_workflow_triggers(
        &self,
        workflow_id: &str,
    ) -> Result<Vec<StoredAdkWorkflowTrigger>, AdkStoreError> {
        let connection = self.lock_connection()?;
        let mut statement = connection
            .prepare(
                "SELECT id, workflow_id, trigger_type, status, next_run_at, payload_json, created_at, updated_at
                 FROM adk_workflow_triggers WHERE workflow_id = ?1 ORDER BY updated_at DESC",
            )
            .map_err(AdkStoreError::Query)?;
        let rows = statement
            .query_map(params![workflow_id], stored_workflow_trigger)
            .map_err(AdkStoreError::Query)?;
        collect_rows(rows)
    }

    pub fn list_audit_events(&self) -> Result<Vec<StoredAdkAuditEvent>, AdkStoreError> {
        let connection = self.lock_connection()?;
        let mut statement = connection
            .prepare(
                "SELECT id, kind, subject_id, payload_json, created_at
                 FROM adk_audit_events ORDER BY created_at DESC",
            )
            .map_err(AdkStoreError::Query)?;
        let rows = statement
            .query_map([], |row| {
                Ok(StoredAdkAuditEvent {
                    id: row.get(0)?,
                    kind: row.get(1)?,
                    subject_id: row.get(2)?,
                    payload_json: row.get(3)?,
                    created_at: row.get(4)?,
                })
            })
            .map_err(AdkStoreError::Query)?;
        collect_rows(rows)
    }

    pub fn list_workflow_trigger_logs(
        &self,
    ) -> Result<Vec<StoredAdkWorkflowTriggerLog>, AdkStoreError> {
        let connection = self.lock_connection()?;
        let mut statement = connection
            .prepare(
                "SELECT id, workflow_id, trigger_id, trigger_type, status, run_id,
                        payload_json, created_at, updated_at
                 FROM adk_workflow_trigger_logs ORDER BY created_at DESC, id ASC",
            )
            .map_err(AdkStoreError::Query)?;
        let rows = statement
            .query_map([], stored_workflow_trigger_log)
            .map_err(AdkStoreError::Query)?;
        collect_rows(rows)
    }

    fn upsert_simple_entity(
        &self,
        table: &str,
        id: &str,
        payload_json: &str,
    ) -> Result<StoredAdkEntity, AdkStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        let created_at = connection
            .query_row(
                &format!("SELECT created_at FROM {table} WHERE id = ?1"),
                params![id],
                |row| row.get::<_, String>(0),
            )
            .optional()
            .map_err(AdkStoreError::Query)?
            .unwrap_or_else(|| now.clone());
        let sql = format!(
            "INSERT INTO {table} (id, payload_json, created_at, updated_at)
             VALUES (?1, ?2, ?3, ?4)
             ON CONFLICT(id) DO UPDATE SET payload_json = ?2, updated_at = ?4"
        );
        connection
            .execute(&sql, params![id, payload_json, created_at, now])
            .map_err(AdkStoreError::Query)?;
        connection
            .query_row(
                &format!(
                    "SELECT id, payload_json, created_at, updated_at FROM {table} WHERE id = ?1"
                ),
                params![id],
                stored_entity,
            )
            .map_err(AdkStoreError::Query)
    }

    fn delete_simple_entity(&self, table: &str, id: &str) -> Result<bool, AdkStoreError> {
        let connection = self.lock_connection()?;
        let sql = format!("DELETE FROM {table} WHERE id = ?1");
        connection
            .execute(&sql, params![id])
            .map(|affected| affected > 0)
            .map_err(AdkStoreError::Query)
    }

    fn get_simple_entity(
        &self,
        table: &str,
        id: &str,
    ) -> Result<Option<StoredAdkEntity>, AdkStoreError> {
        let connection = self.lock_connection()?;
        let sql =
            format!("SELECT id, payload_json, created_at, updated_at FROM {table} WHERE id = ?1");
        connection
            .query_row(&sql, params![id], stored_entity)
            .optional()
            .map_err(AdkStoreError::Query)
    }

    fn list_simple_entities(&self, table: &str) -> Result<Vec<StoredAdkEntity>, AdkStoreError> {
        let connection = self.lock_connection()?;
        let sql = format!(
            "SELECT id, payload_json, created_at, updated_at FROM {table} ORDER BY created_at DESC"
        );
        let mut statement = connection.prepare(&sql).map_err(AdkStoreError::Query)?;
        let rows = statement
            .query_map([], stored_entity)
            .map_err(AdkStoreError::Query)?;
        collect_rows(rows)
    }
}

fn collect_rows<T>(
    rows: rusqlite::MappedRows<'_, impl FnMut(&rusqlite::Row<'_>) -> rusqlite::Result<T>>,
) -> Result<Vec<T>, AdkStoreError> {
    rows.map(|row| row.map_err(AdkStoreError::Query)).collect()
}

fn validate_run_lease_identity(owner_id: &str, run_lease_token: i64) -> Result<(), AdkStoreError> {
    if owner_id.trim().is_empty() || run_lease_token <= 0 {
        return Err(AdkStoreError::Validation(
            "run mutation requires a valid lease owner and fencing token".to_owned(),
        ));
    }
    Ok(())
}

fn ensure_current_run_lease(
    transaction: &rusqlite::Transaction<'_>,
    run_id: &str,
    owner_id: &str,
    run_lease_token: i64,
) -> Result<(), AdkStoreError> {
    let now_ms = (OffsetDateTime::now_utc().unix_timestamp_nanos() / 1_000_000) as i64;
    let valid = transaction
        .query_row(
            "SELECT 1 FROM adk_run_leases
             WHERE run_id = ?1 AND owner_id = ?2 AND fencing_token = ?3
               AND expires_at_unix_ms > ?4",
            params![run_id, owner_id, run_lease_token, now_ms],
            |row| row.get::<_, i64>(0),
        )
        .optional()
        .map_err(AdkStoreError::Query)?
        .is_some();
    if !valid {
        return Err(AdkStoreError::Conflict(format!(
            "run {run_id} lease fencing token {run_lease_token} is no longer current"
        )));
    }
    Ok(())
}

fn attach_adk_session_database(
    connection: &Connection,
    session_db_path: &Path,
) -> Result<(), AdkStoreError> {
    let uri = preflight_adk_session_database(session_db_path)?;
    connection
        .execute("ATTACH DATABASE ?1 AS adk_session_events", params![uri])
        .map_err(AdkStoreError::Query)?;
    Ok(())
}

fn preflight_adk_session_database(session_db_path: &Path) -> Result<String, AdkStoreError> {
    if !session_db_path
        .metadata()
        .map(|metadata| metadata.is_file())
        .unwrap_or(false)
    {
        return Err(AdkStoreError::NotRegularFile(
            session_db_path.display().to_string(),
        ));
    }
    let canonical = session_db_path
        .canonicalize()
        .map_err(|_| AdkStoreError::NotRegularFile(session_db_path.display().to_string()))?;
    let session = Connection::open_with_flags(
        &canonical,
        OpenFlags::SQLITE_OPEN_READ_WRITE
            | OpenFlags::SQLITE_OPEN_NO_MUTEX
            | OpenFlags::SQLITE_OPEN_URI,
    )
    .map_err(AdkStoreError::Open)?;
    session
        .busy_timeout(Duration::from_millis(5_000))
        .map_err(AdkStoreError::Configure)?;
    session
        .pragma_update(None, "foreign_keys", "ON")
        .map_err(AdkStoreError::Configure)?;
    validate_current(
        &session,
        &canonical.display().to_string(),
        ADK_SESSION_COMPONENT,
        ADK_SESSION_SCHEMA_VERSION,
    )?;
    drop(session);
    Ok(sqlite_read_write_uri(&canonical))
}

fn sqlite_read_write_uri(path: &Path) -> String {
    let raw = path.to_string_lossy().replace('\\', "/");
    let mut uri = String::from("file:");
    for byte in raw.bytes() {
        if byte.is_ascii_alphanumeric() || matches!(byte, b'-' | b'.' | b'_' | b'~' | b'/' | b':') {
            uri.push(char::from(byte));
        } else {
            use std::fmt::Write as _;
            let _ = write!(uri, "%{byte:02X}");
        }
    }
    uri.push_str("?mode=rw");
    uri
}

fn append_adk_session_events(
    transaction: &rusqlite::Transaction<'_>,
    now: &str,
    run_id: &str,
    events: &[AdkRunEvent<'_>],
) -> Result<(), AdkStoreError> {
    let run_session_id = transaction
        .query_row(
            "SELECT session_id FROM adk_runs WHERE id = ?1",
            params![run_id],
            |row| row.get::<_, String>(0),
        )
        .optional()
        .map_err(AdkStoreError::Query)?
        .ok_or_else(|| AdkStoreError::NotFound(format!("run {run_id}")))?;
    validate_adk_session_events(run_id, &run_session_id, events)?;
    if !events.is_empty() {
        transaction
            .execute(
                "INSERT OR IGNORE INTO adk_session_events.sessions
                 (app_name, user_id, id, state, create_time, update_time)
                 VALUES ('jftrade', 'local', ?1, '{}', ?2, ?2)",
                params![run_session_id, now],
            )
            .map_err(AdkStoreError::Query)?;
    }
    for event in events {
        let affected = transaction
            .execute(
                "INSERT INTO adk_session_events.events
                 (id, app_name, user_id, session_id, invocation_id, author,
                  content, timestamp)
                 VALUES (?1, 'jftrade', 'local', ?2, ?3, ?4, ?5, ?6)
                 ON CONFLICT(id, app_name, user_id, session_id) DO NOTHING",
                params![
                    event.id,
                    event.session_id,
                    event.invocation_id,
                    event.author,
                    event.content,
                    now,
                ],
            )
            .map_err(AdkStoreError::Query)?;
        if affected == 0 {
            ensure_existing_adk_session_event_matches(transaction, event)?;
        }
    }
    Ok(())
}

fn validate_adk_session_events(
    run_id: &str,
    run_session_id: &str,
    events: &[AdkRunEvent<'_>],
) -> Result<(), AdkStoreError> {
    for event in events {
        if event.id.trim().is_empty()
            || event.session_id.trim().is_empty()
            || event.invocation_id.trim().is_empty()
            || event.author.trim().is_empty()
        {
            return Err(AdkStoreError::Validation(
                "session event identity and author are required".to_owned(),
            ));
        }
        if event.session_id != run_session_id || event.invocation_id != run_id {
            return Err(AdkStoreError::Conflict(format!(
                "session event {} does not belong to run {run_id} and session {run_session_id}",
                event.id
            )));
        }
    }
    Ok(())
}

fn ensure_existing_adk_session_event_matches(
    transaction: &rusqlite::Transaction<'_>,
    event: &AdkRunEvent<'_>,
) -> Result<(), AdkStoreError> {
    let existing = transaction
        .query_row(
            "SELECT invocation_id, author, content
             FROM adk_session_events.events
             WHERE id = ?1 AND app_name = 'jftrade' AND user_id = 'local'
               AND session_id = ?2",
            params![event.id, event.session_id],
            |row| {
                Ok((
                    row.get::<_, String>(0)?,
                    row.get::<_, String>(1)?,
                    row.get::<_, String>(2)?,
                ))
            },
        )
        .optional()
        .map_err(AdkStoreError::Query)?;
    match existing {
        Some((invocation_id, author, content))
            if invocation_id == event.invocation_id
                && author == event.author
                && content == event.content =>
        {
            Ok(())
        }
        _ => Err(AdkStoreError::Conflict(format!(
            "session event key {} was reused with different content",
            event.id
        ))),
    }
}

fn stored_entity(row: &rusqlite::Row<'_>) -> rusqlite::Result<StoredAdkEntity> {
    Ok(StoredAdkEntity {
        id: row.get(0)?,
        payload_json: row.get(1)?,
        created_at: row.get(2)?,
        updated_at: row.get(3)?,
    })
}

fn stored_run(row: &rusqlite::Row<'_>) -> rusqlite::Result<StoredAdkRun> {
    Ok(StoredAdkRun {
        id: row.get(0)?,
        session_id: row.get(1)?,
        agent_id: row.get(2)?,
        status: row.get(3)?,
        client_request_id: row.get(4)?,
        request_fingerprint: row.get(5)?,
        payload_json: row.get(6)?,
        created_at: row.get(7)?,
        updated_at: row.get(8)?,
    })
}

fn stored_run_lease(row: &rusqlite::Row<'_>) -> rusqlite::Result<StoredAdkRunLease> {
    Ok(StoredAdkRunLease {
        run_id: row.get(0)?,
        owner_id: row.get(1)?,
        fencing_token: row.get(2)?,
        heartbeat_at_unix_ms: row.get(3)?,
        expires_at_unix_ms: row.get(4)?,
        created_at: row.get(5)?,
        updated_at: row.get(6)?,
    })
}

fn stored_approval(row: &rusqlite::Row<'_>) -> rusqlite::Result<StoredAdkApproval> {
    Ok(StoredAdkApproval {
        id: row.get(0)?,
        run_id: row.get(1)?,
        agent_id: row.get(2)?,
        status: row.get(3)?,
        payload_json: row.get(4)?,
        created_at: row.get(5)?,
        updated_at: row.get(6)?,
    })
}

fn stored_tool_invocation(row: &rusqlite::Row<'_>) -> rusqlite::Result<StoredAdkToolInvocation> {
    Ok(StoredAdkToolInvocation {
        run_id: row.get(0)?,
        idempotency_key: row.get(1)?,
        tool_name: row.get(2)?,
        status: row.get(3)?,
        owner_id: row.get(4)?,
        fencing_token: row.get(5)?,
        run_lease_token: row.get(6)?,
        input_json: row.get(7)?,
        output_json: row.get(8)?,
        lease_expires_at_unix_ms: row.get(9)?,
        created_at: row.get(10)?,
        updated_at: row.get(11)?,
    })
}

fn decode_json_object(
    raw: &str,
    resource: &str,
) -> Result<serde_json::Map<String, Value>, AdkStoreError> {
    let value: Value = serde_json::from_str(raw).map_err(|error| {
        AdkStoreError::Validation(format!("stored {resource} JSON is invalid: {error}"))
    })?;
    value.as_object().cloned().ok_or_else(|| {
        AdkStoreError::Validation(format!("stored {resource} payload must be a JSON object"))
    })
}

fn stored_approval_value(approval: &StoredAdkApproval) -> Result<Value, AdkStoreError> {
    let mut value = decode_json_object(&approval.payload_json, "approval")?;
    value.insert("id".to_owned(), Value::String(approval.id.clone()));
    value.insert("runId".to_owned(), Value::String(approval.run_id.clone()));
    value.insert(
        "agentId".to_owned(),
        Value::String(approval.agent_id.clone()),
    );
    value.insert("status".to_owned(), Value::String(approval.status.clone()));
    value.insert(
        "createdAt".to_owned(),
        Value::String(approval.created_at.clone()),
    );
    value.insert(
        "updatedAt".to_owned(),
        Value::String(approval.updated_at.clone()),
    );
    Ok(Value::Object(value))
}

fn stored_task(row: &rusqlite::Row<'_>) -> rusqlite::Result<StoredAdkTask> {
    Ok(StoredAdkTask {
        id: row.get(0)?,
        status: row.get(1)?,
        agent_id: row.get(2)?,
        run_id: row.get(3)?,
        payload_json: row.get(4)?,
        created_at: row.get(5)?,
        updated_at: row.get(6)?,
    })
}

fn stored_workflow_trigger(row: &rusqlite::Row<'_>) -> rusqlite::Result<StoredAdkWorkflowTrigger> {
    Ok(StoredAdkWorkflowTrigger {
        id: row.get(0)?,
        workflow_id: row.get(1)?,
        trigger_type: row.get(2)?,
        status: row.get(3)?,
        next_run_at: row.get(4)?,
        payload_json: row.get(5)?,
        created_at: row.get(6)?,
        updated_at: row.get(7)?,
    })
}

fn stored_workflow_trigger_log(
    row: &rusqlite::Row<'_>,
) -> rusqlite::Result<StoredAdkWorkflowTriggerLog> {
    Ok(StoredAdkWorkflowTriggerLog {
        id: row.get(0)?,
        workflow_id: row.get(1)?,
        trigger_id: row.get(2)?,
        trigger_type: row.get(3)?,
        status: row.get(4)?,
        run_id: row.get(5)?,
        payload_json: row.get(6)?,
        created_at: row.get(7)?,
        updated_at: row.get(8)?,
    })
}

#[derive(Debug)]
pub struct AdkTestCutoverStore {
    inner: AdkStore,
}

impl AdkTestCutoverStore {
    pub fn open_existing(path: impl AsRef<Path>, profile: &str) -> Result<Self, AdkStoreError> {
        let inner = AdkStore::open_existing(path, profile)?;
        Ok(Self { inner })
    }

    pub fn path(&self) -> &Path {
        &self.inner.path
    }

    pub fn upsert_provider(
        &self,
        id: &str,
        payload_json: &str,
    ) -> Result<StoredAdkEntity, AdkStoreError> {
        self.inner.upsert_provider(id, payload_json)
    }

    pub fn delete_provider(&self, id: &str) -> Result<bool, AdkStoreError> {
        self.inner.delete_provider(id)
    }

    pub fn get_provider(&self, id: &str) -> Result<Option<StoredAdkEntity>, AdkStoreError> {
        self.inner.get_provider(id)
    }

    pub fn upsert_agent(
        &self,
        id: &str,
        payload_json: &str,
    ) -> Result<StoredAdkEntity, AdkStoreError> {
        self.inner.upsert_agent(id, payload_json)
    }

    pub fn delete_agent(&self, id: &str) -> Result<bool, AdkStoreError> {
        self.inner.delete_agent(id)
    }

    pub fn get_agent(&self, id: &str) -> Result<Option<StoredAdkEntity>, AdkStoreError> {
        self.inner.get_agent(id)
    }

    pub fn upsert_session(
        &self,
        id: &str,
        agent_id: &str,
        payload_json: &str,
    ) -> Result<StoredAdkEntity, AdkStoreError> {
        self.inner.upsert_session(id, agent_id, payload_json)
    }

    pub fn delete_session(&self, id: &str) -> Result<bool, AdkStoreError> {
        self.inner.delete_session(id)
    }

    pub fn create_run(
        &self,
        params: CreateAdkRunParams<'_>,
    ) -> Result<StoredAdkRun, AdkStoreError> {
        self.inner.create_run(params)
    }

    pub fn create_run_with_event(
        &self,
        params: CreateAdkRunParams<'_>,
        session_db_path: &Path,
        event: &AdkRunEvent<'_>,
    ) -> Result<StoredAdkRun, AdkStoreError> {
        self.inner
            .create_run_with_event(params, session_db_path, event)
    }

    pub fn claim_run_lease(
        &self,
        run_id: &str,
        owner_id: &str,
        lease_ttl: Duration,
    ) -> Result<StoredAdkRunLease, AdkStoreError> {
        self.inner.claim_run_lease(run_id, owner_id, lease_ttl)
    }

    pub fn heartbeat_run_lease(
        &self,
        lease: &StoredAdkRunLease,
        lease_ttl: Duration,
    ) -> Result<StoredAdkRunLease, AdkStoreError> {
        self.inner.heartbeat_run_lease(lease, lease_ttl)
    }

    pub fn release_run_lease(&self, lease: &StoredAdkRunLease) -> Result<bool, AdkStoreError> {
        self.inner.release_run_lease(lease)
    }

    pub fn get_run_lease(&self, run_id: &str) -> Result<Option<StoredAdkRunLease>, AdkStoreError> {
        self.inner.get_run_lease(run_id)
    }

    pub fn update_run_status(&self, id: &str, status: &str) -> Result<bool, AdkStoreError> {
        self.inner.update_run_status(id, status)
    }

    pub fn update_run_payload(&self, id: &str, payload_json: &str) -> Result<bool, AdkStoreError> {
        self.inner.update_run_payload(id, payload_json)
    }

    pub fn update_run_payload_if_status(
        &self,
        id: &str,
        expected_status: &str,
        payload_json: &str,
    ) -> Result<bool, AdkStoreError> {
        self.inner
            .update_run_payload_if_status(id, expected_status, payload_json)
    }

    pub fn update_run_payload_if_status_and_revision(
        &self,
        id: &str,
        expected_status: &str,
        expected_updated_at: &str,
        payload_json: &str,
    ) -> Result<bool, AdkStoreError> {
        self.inner.update_run_payload_if_status_and_revision(
            id,
            expected_status,
            expected_updated_at,
            payload_json,
        )
    }

    pub fn update_run_payload_if_status_and_revision_with_lease(
        &self,
        id: &str,
        expected_status: &str,
        expected_updated_at: &str,
        payload_json: &str,
        owner_id: &str,
        run_lease_token: i64,
    ) -> Result<bool, AdkStoreError> {
        self.inner
            .update_run_payload_if_status_and_revision_with_lease(
                id,
                expected_status,
                expected_updated_at,
                payload_json,
                owner_id,
                run_lease_token,
            )
    }

    #[allow(clippy::too_many_arguments)]
    pub fn update_run_payload_if_status_and_revision_with_events_with_lease(
        &self,
        id: &str,
        expected_status: &str,
        expected_updated_at: &str,
        payload_json: &str,
        session_db_path: &Path,
        events: &[AdkRunEvent<'_>],
        owner_id: &str,
        run_lease_token: i64,
    ) -> Result<bool, AdkStoreError> {
        self.inner
            .update_run_payload_if_status_and_revision_with_events_with_lease(
                id,
                expected_status,
                expected_updated_at,
                payload_json,
                session_db_path,
                events,
                owner_id,
                run_lease_token,
            )
    }

    pub fn update_run_state(
        &self,
        id: &str,
        status: &str,
        payload_json: &str,
    ) -> Result<bool, AdkStoreError> {
        self.inner.update_run_state(id, status, payload_json)
    }

    pub fn update_run_state_if_status(
        &self,
        id: &str,
        expected_status: &str,
        status: &str,
        payload_json: &str,
    ) -> Result<bool, AdkStoreError> {
        self.inner
            .update_run_state_if_status(id, expected_status, status, payload_json)
    }

    pub fn update_run_state_if_status_and_revision(
        &self,
        id: &str,
        expected_status: &str,
        expected_updated_at: &str,
        status: &str,
        payload_json: &str,
    ) -> Result<bool, AdkStoreError> {
        self.inner.update_run_state_if_status_and_revision(
            id,
            expected_status,
            expected_updated_at,
            status,
            payload_json,
        )
    }

    pub fn update_run_state_if_status_and_revision_with_lease(
        &self,
        id: &str,
        expected_status: &str,
        expected_updated_at: &str,
        status: &str,
        payload_json: &str,
        owner_id: &str,
        run_lease_token: i64,
    ) -> Result<bool, AdkStoreError> {
        self.inner
            .update_run_state_if_status_and_revision_with_lease(
                id,
                expected_status,
                expected_updated_at,
                status,
                payload_json,
                owner_id,
                run_lease_token,
            )
    }

    pub fn update_run_state_if_status_and_revision_with_events(
        &self,
        id: &str,
        expected_status: &str,
        expected_updated_at: &str,
        status: &str,
        payload_json: &str,
        session_db_path: &Path,
        events: &[AdkRunEvent<'_>],
    ) -> Result<bool, AdkStoreError> {
        self.inner
            .update_run_state_if_status_and_revision_with_events(
                id,
                expected_status,
                expected_updated_at,
                status,
                payload_json,
                session_db_path,
                events,
            )
    }

    #[allow(clippy::too_many_arguments)]
    pub fn update_run_state_if_status_and_revision_with_events_with_lease(
        &self,
        id: &str,
        expected_status: &str,
        expected_updated_at: &str,
        status: &str,
        payload_json: &str,
        session_db_path: &Path,
        events: &[AdkRunEvent<'_>],
        owner_id: &str,
        run_lease_token: i64,
    ) -> Result<bool, AdkStoreError> {
        self.inner
            .update_run_state_if_status_and_revision_with_events_with_lease(
                id,
                expected_status,
                expected_updated_at,
                status,
                payload_json,
                session_db_path,
                events,
                owner_id,
                run_lease_token,
            )
    }

    pub fn get_tool_invocation(
        &self,
        run_id: &str,
        idempotency_key: &str,
    ) -> Result<Option<StoredAdkToolInvocation>, AdkStoreError> {
        self.inner.get_tool_invocation(run_id, idempotency_key)
    }

    pub fn claim_tool_invocation_if_status_and_revision(
        &self,
        run_id: &str,
        idempotency_key: &str,
        tool_name: &str,
        input_json: &str,
        expected_status: &str,
        expected_updated_at: &str,
        owner_id: &str,
        run_lease_token: i64,
        lease_ttl: Duration,
    ) -> Result<AdkToolInvocationClaim, AdkStoreError> {
        self.inner.claim_tool_invocation_if_status_and_revision(
            run_id,
            idempotency_key,
            tool_name,
            input_json,
            expected_status,
            expected_updated_at,
            owner_id,
            run_lease_token,
            lease_ttl,
        )
    }

    pub fn commit_tool_result_if_status_and_revision_with_event(
        &self,
        id: &str,
        expected_status: &str,
        expected_updated_at: &str,
        payload_json: &str,
        idempotency_key: &str,
        tool_name: &str,
        input_json: &str,
        output_json: &str,
        status: &str,
        owner_id: &str,
        fencing_token: i64,
        run_lease_token: i64,
        session_db_path: &Path,
        event: &AdkRunEvent<'_>,
    ) -> Result<AdkToolResultCommit, AdkStoreError> {
        self.inner
            .commit_tool_result_if_status_and_revision_with_event(
                id,
                expected_status,
                expected_updated_at,
                payload_json,
                idempotency_key,
                tool_name,
                input_json,
                output_json,
                status,
                owner_id,
                fencing_token,
                run_lease_token,
                session_db_path,
                event,
            )
    }

    pub fn stage_tool_calls_if_status_and_revision_with_events(
        &self,
        id: &str,
        expected_status: &str,
        expected_updated_at: &str,
        status: &str,
        payload_json: &str,
        approvals: &[AdkApprovalStage<'_>],
        session_db_path: &Path,
        events: &[AdkRunEvent<'_>],
    ) -> Result<bool, AdkStoreError> {
        self.inner
            .stage_tool_calls_if_status_and_revision_with_events(
                id,
                expected_status,
                expected_updated_at,
                status,
                payload_json,
                approvals,
                session_db_path,
                events,
            )
    }

    pub fn stage_tool_calls_if_status_and_revision_with_events_with_lease(
        &self,
        id: &str,
        expected_status: &str,
        expected_updated_at: &str,
        status: &str,
        payload_json: &str,
        approvals: &[AdkApprovalStage<'_>],
        session_db_path: &Path,
        events: &[AdkRunEvent<'_>],
        owner_id: &str,
        run_lease_token: i64,
    ) -> Result<bool, AdkStoreError> {
        self.inner
            .stage_tool_calls_if_status_and_revision_with_events_with_lease(
                id,
                expected_status,
                expected_updated_at,
                status,
                payload_json,
                approvals,
                session_db_path,
                events,
                owner_id,
                run_lease_token,
            )
    }

    pub fn create_approval(
        &self,
        id: &str,
        run_id: &str,
        agent_id: &str,
        status: &str,
        payload_json: &str,
    ) -> Result<StoredAdkApproval, AdkStoreError> {
        self.inner
            .create_approval(id, run_id, agent_id, status, payload_json)
    }

    pub fn update_approval_status(&self, id: &str, status: &str) -> Result<bool, AdkStoreError> {
        self.inner.update_approval_status(id, status)
    }

    pub fn upsert_memory(
        &self,
        id: &str,
        agent_id: &str,
        scope: &str,
        memory_key: &str,
        payload_json: &str,
    ) -> Result<StoredAdkMemory, AdkStoreError> {
        self.inner
            .upsert_memory(id, agent_id, scope, memory_key, payload_json)
    }

    pub fn delete_memory(&self, id: &str) -> Result<bool, AdkStoreError> {
        self.inner.delete_memory(id)
    }

    pub fn get_memory(&self, id: &str) -> Result<Option<StoredAdkMemory>, AdkStoreError> {
        self.inner.get_memory(id)
    }

    pub fn upsert_workflow(
        &self,
        id: &str,
        status: &str,
        payload_json: &str,
    ) -> Result<StoredAdkWorkflow, AdkStoreError> {
        self.inner.upsert_workflow(id, status, payload_json)
    }

    pub fn update_workflow_if_revision(
        &self,
        id: &str,
        expected_updated_at: &str,
        status: &str,
        payload_json: &str,
    ) -> Result<bool, AdkStoreError> {
        self.inner
            .update_workflow_if_revision(id, expected_updated_at, status, payload_json)
    }

    pub fn soft_delete_workflow_if_revision(
        &self,
        id: &str,
        expected_updated_at: &str,
        payload_json: &str,
        deleted_at: &str,
    ) -> Result<bool, AdkStoreError> {
        self.inner.soft_delete_workflow_if_revision(
            id,
            expected_updated_at,
            payload_json,
            deleted_at,
        )
    }

    pub fn delete_workflow(&self, id: &str) -> Result<bool, AdkStoreError> {
        self.inner.delete_workflow(id)
    }

    pub fn update_workflow_trigger_if_revision(
        &self,
        id: &str,
        expected_updated_at: &str,
        workflow_id: &str,
        trigger_type: &str,
        status: &str,
        next_run_at: &str,
        payload_json: &str,
    ) -> Result<bool, AdkStoreError> {
        self.inner.update_workflow_trigger_if_revision(
            id,
            expected_updated_at,
            workflow_id,
            trigger_type,
            status,
            next_run_at,
            payload_json,
        )
    }

    pub fn soft_delete_workflow_trigger_if_revision(
        &self,
        id: &str,
        expected_updated_at: &str,
        workflow_id: &str,
        payload_json: &str,
    ) -> Result<bool, AdkStoreError> {
        self.inner.soft_delete_workflow_trigger_if_revision(
            id,
            expected_updated_at,
            workflow_id,
            payload_json,
        )
    }

    pub fn record_audit_event(
        &self,
        id: &str,
        kind: &str,
        subject_id: &str,
        payload_json: &str,
    ) -> Result<(), AdkStoreError> {
        self.inner
            .record_audit_event(id, kind, subject_id, payload_json)
    }

    pub fn list_providers(&self) -> Result<Vec<StoredAdkEntity>, AdkStoreError> {
        self.inner.list_providers()
    }

    pub fn list_agents(&self) -> Result<Vec<StoredAdkEntity>, AdkStoreError> {
        self.inner.list_agents()
    }

    pub fn list_workflows(&self) -> Result<Vec<StoredAdkWorkflow>, AdkStoreError> {
        self.inner.list_workflows()
    }

    pub fn get_workflow(&self, id: &str) -> Result<Option<StoredAdkWorkflow>, AdkStoreError> {
        self.inner.get_workflow(id)
    }

    pub fn upsert_workflow_trigger(
        &self,
        id: &str,
        workflow_id: &str,
        trigger_type: &str,
        status: &str,
        next_run_at: &str,
        payload_json: &str,
    ) -> Result<StoredAdkWorkflowTrigger, AdkStoreError> {
        self.inner.upsert_workflow_trigger(
            id,
            workflow_id,
            trigger_type,
            status,
            next_run_at,
            payload_json,
        )
    }

    pub fn get_workflow_trigger(
        &self,
        id: &str,
    ) -> Result<Option<StoredAdkWorkflowTrigger>, AdkStoreError> {
        self.inner.get_workflow_trigger(id)
    }

    pub fn list_approvals(&self) -> Result<Vec<StoredAdkApproval>, AdkStoreError> {
        self.inner.list_approvals()
    }

    pub fn list_memories(&self) -> Result<Vec<StoredAdkMemory>, AdkStoreError> {
        self.inner.list_memories()
    }
}
