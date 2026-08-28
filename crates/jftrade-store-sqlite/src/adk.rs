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
                     WHERE id = ?4 AND status = 'PENDING'",
                    params![status, payload_json, now, id],
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
                 WHERE id = ?4 AND status = 'PENDING'",
                params![next_status, payload_json, now, run.id],
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

    pub fn update_run_status(&self, id: &str, status: &str) -> Result<bool, AdkStoreError> {
        self.inner.update_run_status(id, status)
    }

    pub fn update_run_payload(&self, id: &str, payload_json: &str) -> Result<bool, AdkStoreError> {
        self.inner.update_run_payload(id, payload_json)
    }

    pub fn update_run_state(
        &self,
        id: &str,
        status: &str,
        payload_json: &str,
    ) -> Result<bool, AdkStoreError> {
        self.inner.update_run_state(id, status, payload_json)
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

    pub fn delete_workflow(&self, id: &str) -> Result<bool, AdkStoreError> {
        self.inner.delete_workflow(id)
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

    pub fn list_approvals(&self) -> Result<Vec<StoredAdkApproval>, AdkStoreError> {
        self.inner.list_approvals()
    }

    pub fn list_memories(&self) -> Result<Vec<StoredAdkMemory>, AdkStoreError> {
        self.inner.list_memories()
    }
}
