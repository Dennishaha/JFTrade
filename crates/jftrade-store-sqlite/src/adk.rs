use std::path::{Path, PathBuf};
use std::sync::{Mutex, MutexGuard};
use std::time::Duration;

use jftrade_owner_lock::{OwnerDiagnostic, WriterLease, WriterLeaseError};
use rusqlite::{Connection, OpenFlags, OptionalExtension, params};
use serde::{Deserialize, Serialize};
use thiserror::Error;
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

use crate::schema_manifest::{SchemaManifestError, validate_current};

const ADK_COMPONENT: &str = "adk";
const ADK_SCHEMA_VERSION: i64 = 4;
pub const ADK_TEST_CUTOVER_PROFILE: &str = "cutover-test-only.v1";

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

pub struct AdkTestCutoverStore {
    path: PathBuf,
    connection: Mutex<Connection>,
    _writer_lease: WriterLease,
}

impl std::fmt::Debug for AdkTestCutoverStore {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("AdkTestCutoverStore")
            .field("path", &self.path)
            .finish()
    }
}

impl AdkTestCutoverStore {
    pub fn open_existing(path: impl AsRef<Path>, profile: &str) -> Result<Self, AdkStoreError> {
        let path = path.as_ref();
        if path.as_os_str().is_empty() {
            return Err(AdkStoreError::EmptyPath);
        }
        if profile != ADK_TEST_CUTOVER_PROFILE {
            return Err(AdkStoreError::UnsupportedProfile(profile.to_owned()));
        }
        if !path
            .metadata()
            .map(|metadata| metadata.is_file())
            .unwrap_or(false)
        {
            return Err(AdkStoreError::NotRegularFile(path.display().to_string()));
        }

        let writer_lease = WriterLease::acquire(
            path,
            &OwnerDiagnostic::current("rust", ADK_TEST_CUTOVER_PROFILE),
        )?;

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
        connection
            .execute(
                "INSERT INTO adk_providers (id, payload_json, created_at, updated_at)
                 VALUES (?1, ?2, ?3, ?3)
                 ON CONFLICT(id) DO UPDATE SET payload_json = ?2, updated_at = ?3",
                params![id, payload_json, now],
            )
            .map_err(AdkStoreError::Query)?;

        Ok(StoredAdkEntity {
            id: id.to_owned(),
            payload_json: payload_json.to_owned(),
            created_at: now.clone(),
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
        connection
            .execute(
                "INSERT INTO adk_agents (id, payload_json, created_at, updated_at)
                 VALUES (?1, ?2, ?3, ?3)
                 ON CONFLICT(id) DO UPDATE SET payload_json = ?2, updated_at = ?3",
                params![id, payload_json, now],
            )
            .map_err(AdkStoreError::Query)?;

        Ok(StoredAdkEntity {
            id: id.to_owned(),
            payload_json: payload_json.to_owned(),
            created_at: now.clone(),
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
        connection
            .execute(
                "INSERT INTO adk_sessions (id, agent_id, payload_json, created_at, updated_at)
                 VALUES (?1, ?2, ?3, ?4, ?4)
                 ON CONFLICT(id) DO UPDATE SET agent_id = ?2, payload_json = ?3, updated_at = ?4",
                params![id, agent_id, payload_json, now],
            )
            .map_err(AdkStoreError::Query)?;

        Ok(StoredAdkEntity {
            id: id.to_owned(),
            payload_json: payload_json.to_owned(),
            created_at: now.clone(),
            updated_at: now,
        })
    }

    pub fn delete_session(&self, id: &str) -> Result<bool, AdkStoreError> {
        let connection = self.lock_connection()?;
        let affected = connection
            .execute("DELETE FROM adk_sessions WHERE id = ?1", params![id])
            .map_err(AdkStoreError::Query)?;
        Ok(affected > 0)
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
        connection
            .execute(
                "INSERT INTO adk_memory (id, agent_id, scope, memory_key, payload_json, created_at, updated_at)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?6)
                 ON CONFLICT(agent_id, scope, memory_key) DO UPDATE SET
                   payload_json = ?5, updated_at = ?6",
                params![id, agent_id, scope, memory_key, payload_json, now],
            )
            .map_err(AdkStoreError::Query)?;

        Ok(StoredAdkMemory {
            id: id.to_owned(),
            agent_id: agent_id.to_owned(),
            scope: scope.to_owned(),
            memory_key: memory_key.to_owned(),
            payload_json: payload_json.to_owned(),
            created_at: now.clone(),
            updated_at: now,
        })
    }

    pub fn delete_memory(&self, id: &str) -> Result<bool, AdkStoreError> {
        let connection = self.lock_connection()?;
        let affected = connection
            .execute("DELETE FROM adk_memory WHERE id = ?1", params![id])
            .map_err(AdkStoreError::Query)?;
        Ok(affected > 0)
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
        connection
            .execute(
                "INSERT INTO adk_workflows (id, status, payload_json, created_at, updated_at)
                 VALUES (?1, ?2, ?3, ?4, ?4)
                 ON CONFLICT(id) DO UPDATE SET status = ?2, payload_json = ?3, updated_at = ?4",
                params![id, status, payload_json, now],
            )
            .map_err(AdkStoreError::Query)?;

        Ok(StoredAdkWorkflow {
            id: id.to_owned(),
            status: status.to_owned(),
            payload_json: payload_json.to_owned(),
            created_at: now.clone(),
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

    // --- Audit Events ---
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
}
