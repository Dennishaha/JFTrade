use std::path::{Path, PathBuf};
use std::sync::{Mutex, MutexGuard};
use std::time::Duration;

use jftrade_owner_lock::{OwnerDiagnostic, WriterLease, WriterLeaseError};
use rusqlite::{Connection, OpenFlags, OptionalExtension, params};
use serde::{Deserialize, Serialize};
use thiserror::Error;
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

use crate::schema_manifest::{SchemaManifestError, validate_current};

const ADK_SESSION_COMPONENT: &str = "adk-session";
const ADK_SESSION_SCHEMA_VERSION: i64 = 4;
pub const ADK_SESSION_TEST_CUTOVER_PROFILE: &str = "cutover-test-only.v1";
pub const ADK_SESSION_PRODUCTION_PROFILE: &str = "production.v1";

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredAdkSessionState {
    pub app_name: String,
    pub user_id: String,
    pub id: String,
    pub state: String,
    pub create_time: String,
    pub update_time: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredAdkEvent {
    pub id: String,
    pub app_name: String,
    pub user_id: String,
    pub session_id: String,
    pub invocation_id: String,
    pub author: String,
    pub content: String,
    pub timestamp: String,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct RecordAdkEventParams<'a> {
    pub id: &'a str,
    pub app_name: &'a str,
    pub user_id: &'a str,
    pub session_id: &'a str,
    pub invocation_id: &'a str,
    pub author: &'a str,
    pub content: &'a str,
}

#[derive(Debug, Error)]
pub enum AdkSessionStoreError {
    #[error("adk-session database path is required")]
    EmptyPath,
    #[error("unsupported adk-session writer profile: {0}")]
    UnsupportedProfile(String),
    #[error("adk-session database is not an existing regular file: {0}")]
    NotRegularFile(String),
    #[error(transparent)]
    WriterLease(#[from] WriterLeaseError),
    #[error("open adk-session database: {0}")]
    Open(#[source] rusqlite::Error),
    #[error("configure adk-session database: {0}")]
    Configure(#[source] rusqlite::Error),
    #[error(transparent)]
    Schema(#[from] SchemaManifestError),
    #[error("adk-session database lock is unavailable")]
    LockUnavailable,
    #[error("query adk-session database: {0}")]
    Query(#[source] rusqlite::Error),
    #[error("adk session not found: {0}")]
    NotFound(String),
    #[error("incompatible adk-session database: {0}")]
    Incompatible(String),
}

pub struct AdkSessionStore {
    path: PathBuf,
    connection: Mutex<Connection>,
    _writer_lease: WriterLease,
}

impl std::fmt::Debug for AdkSessionStore {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("AdkSessionStore")
            .field("path", &self.path)
            .finish()
    }
}

impl AdkSessionStore {
    pub fn open(path: impl AsRef<Path>) -> Result<Self, AdkSessionStoreError> {
        Self::open_existing(path, ADK_SESSION_PRODUCTION_PROFILE)
    }

    pub fn open_existing(
        path: impl AsRef<Path>,
        profile: &str,
    ) -> Result<Self, AdkSessionStoreError> {
        let path = path.as_ref();
        if path.as_os_str().is_empty() {
            return Err(AdkSessionStoreError::EmptyPath);
        }
        if profile != ADK_SESSION_TEST_CUTOVER_PROFILE && profile != ADK_SESSION_PRODUCTION_PROFILE
        {
            return Err(AdkSessionStoreError::UnsupportedProfile(profile.to_owned()));
        }
        if !path
            .metadata()
            .map(|metadata| metadata.is_file())
            .unwrap_or(false)
        {
            return Err(AdkSessionStoreError::NotRegularFile(
                path.display().to_string(),
            ));
        }

        let writer_lease = WriterLease::acquire(path, &OwnerDiagnostic::current("rust", profile))?;

        let connection = Connection::open_with_flags(
            path,
            OpenFlags::SQLITE_OPEN_READ_WRITE
                | OpenFlags::SQLITE_OPEN_NO_MUTEX
                | OpenFlags::SQLITE_OPEN_URI,
        )
        .map_err(AdkSessionStoreError::Open)?;

        connection
            .busy_timeout(Duration::from_millis(5_000))
            .map_err(AdkSessionStoreError::Configure)?;
        connection
            .pragma_update(None, "foreign_keys", "ON")
            .map_err(AdkSessionStoreError::Configure)?;

        validate_current(
            &connection,
            &path.display().to_string(),
            ADK_SESSION_COMPONENT,
            ADK_SESSION_SCHEMA_VERSION,
        )?;

        Ok(Self {
            path: path.to_path_buf(),
            connection: Mutex::new(connection),
            _writer_lease: writer_lease,
        })
    }

    fn lock_connection(&self) -> Result<MutexGuard<'_, Connection>, AdkSessionStoreError> {
        self.connection
            .lock()
            .map_err(|_| AdkSessionStoreError::LockUnavailable)
    }

    fn now_rfc3339() -> String {
        OffsetDateTime::now_utc()
            .format(&Rfc3339)
            .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned())
    }

    pub fn upsert_session(
        &self,
        app_name: &str,
        user_id: &str,
        id: &str,
        state: &str,
    ) -> Result<StoredAdkSessionState, AdkSessionStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        connection
            .execute(
                "INSERT INTO sessions (app_name, user_id, id, state, create_time, update_time)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?5)
                 ON CONFLICT(app_name, user_id, id) DO UPDATE SET state = ?4, update_time = ?5",
                params![app_name, user_id, id, state, now],
            )
            .map_err(AdkSessionStoreError::Query)?;

        Ok(StoredAdkSessionState {
            app_name: app_name.to_owned(),
            user_id: user_id.to_owned(),
            id: id.to_owned(),
            state: state.to_owned(),
            create_time: now.clone(),
            update_time: now,
        })
    }

    pub fn delete_session(
        &self,
        app_name: &str,
        user_id: &str,
        id: &str,
    ) -> Result<bool, AdkSessionStoreError> {
        let connection = self.lock_connection()?;
        let affected = connection
            .execute(
                "DELETE FROM sessions WHERE app_name = ?1 AND user_id = ?2 AND id = ?3",
                params![app_name, user_id, id],
            )
            .map_err(AdkSessionStoreError::Query)?;
        Ok(affected > 0)
    }

    pub fn list_sessions(&self) -> Result<Vec<StoredAdkSessionState>, AdkSessionStoreError> {
        let connection = self.lock_connection()?;
        let mut statement = connection
            .prepare(
                "SELECT app_name, user_id, id, state, create_time, update_time
                 FROM sessions ORDER BY update_time DESC",
            )
            .map_err(AdkSessionStoreError::Query)?;
        let rows = statement
            .query_map([], stored_session)
            .map_err(AdkSessionStoreError::Query)?;
        rows.map(|row| row.map_err(AdkSessionStoreError::Query))
            .collect()
    }

    pub fn get_session_by_id(
        &self,
        id: &str,
    ) -> Result<Option<StoredAdkSessionState>, AdkSessionStoreError> {
        let connection = self.lock_connection()?;
        connection
            .query_row(
                "SELECT app_name, user_id, id, state, create_time, update_time
                 FROM sessions WHERE id = ?1 ORDER BY update_time DESC LIMIT 1",
                params![id],
                stored_session,
            )
            .optional()
            .map_err(AdkSessionStoreError::Query)
    }

    pub fn record_event(
        &self,
        params: RecordAdkEventParams<'_>,
    ) -> Result<StoredAdkEvent, AdkSessionStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        connection
            .execute(
                "INSERT INTO events (id, app_name, user_id, session_id, invocation_id, author, content, timestamp)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)",
                params![
                    params.id,
                    params.app_name,
                    params.user_id,
                    params.session_id,
                    params.invocation_id,
                    params.author,
                    params.content,
                    now,
                ],
            )
            .map_err(AdkSessionStoreError::Query)?;

        Ok(StoredAdkEvent {
            id: params.id.to_owned(),
            app_name: params.app_name.to_owned(),
            user_id: params.user_id.to_owned(),
            session_id: params.session_id.to_owned(),
            invocation_id: params.invocation_id.to_owned(),
            author: params.author.to_owned(),
            content: params.content.to_owned(),
            timestamp: now,
        })
    }

    pub fn list_events(
        &self,
        session_id: &str,
    ) -> Result<Vec<StoredAdkEvent>, AdkSessionStoreError> {
        let connection = self.lock_connection()?;
        let mut statement = connection
            .prepare(
                "SELECT id, app_name, user_id, session_id, invocation_id, author, content, timestamp
                 FROM events WHERE session_id = ?1 ORDER BY timestamp ASC, id ASC",
            )
            .map_err(AdkSessionStoreError::Query)?;
        let rows = statement
            .query_map(params![session_id], |row| {
                Ok(StoredAdkEvent {
                    id: row.get(0)?,
                    app_name: row.get(1)?,
                    user_id: row.get(2)?,
                    session_id: row.get(3)?,
                    invocation_id: row.get(4)?,
                    author: row.get(5)?,
                    content: row.get(6)?,
                    timestamp: row.get(7)?,
                })
            })
            .map_err(AdkSessionStoreError::Query)?;
        rows.map(|row| row.map_err(AdkSessionStoreError::Query))
            .collect()
    }

    pub fn upsert_app_state(
        &self,
        app_name: &str,
        state: &str,
    ) -> Result<(), AdkSessionStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        connection
            .execute(
                "INSERT INTO app_states (app_name, state, update_time)
                 VALUES (?1, ?2, ?3)
                 ON CONFLICT(app_name) DO UPDATE SET state = ?2, update_time = ?3",
                params![app_name, state, now],
            )
            .map_err(AdkSessionStoreError::Query)?;
        Ok(())
    }

    pub fn upsert_user_state(
        &self,
        app_name: &str,
        user_id: &str,
        state: &str,
    ) -> Result<(), AdkSessionStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        connection
            .execute(
                "INSERT INTO user_states (app_name, user_id, state, update_time)
                 VALUES (?1, ?2, ?3, ?4)
                 ON CONFLICT(app_name, user_id) DO UPDATE SET state = ?3, update_time = ?4",
                params![app_name, user_id, state, now],
            )
            .map_err(AdkSessionStoreError::Query)?;
        Ok(())
    }
}

fn stored_session(row: &rusqlite::Row<'_>) -> rusqlite::Result<StoredAdkSessionState> {
    Ok(StoredAdkSessionState {
        app_name: row.get(0)?,
        user_id: row.get(1)?,
        id: row.get(2)?,
        state: row.get(3)?,
        create_time: row.get(4)?,
        update_time: row.get(5)?,
    })
}

#[derive(Debug)]
pub struct AdkSessionTestCutoverStore {
    inner: AdkSessionStore,
}

impl AdkSessionTestCutoverStore {
    pub fn open_existing(
        path: impl AsRef<Path>,
        profile: &str,
    ) -> Result<Self, AdkSessionStoreError> {
        let inner = AdkSessionStore::open_existing(path, profile)?;
        Ok(Self { inner })
    }

    pub fn path(&self) -> &Path {
        &self.inner.path
    }

    pub fn upsert_session(
        &self,
        app_name: &str,
        user_id: &str,
        id: &str,
        state: &str,
    ) -> Result<StoredAdkSessionState, AdkSessionStoreError> {
        self.inner.upsert_session(app_name, user_id, id, state)
    }

    pub fn delete_session(
        &self,
        app_name: &str,
        user_id: &str,
        id: &str,
    ) -> Result<bool, AdkSessionStoreError> {
        self.inner.delete_session(app_name, user_id, id)
    }

    pub fn record_event(
        &self,
        params: RecordAdkEventParams<'_>,
    ) -> Result<StoredAdkEvent, AdkSessionStoreError> {
        self.inner.record_event(params)
    }

    pub fn upsert_app_state(
        &self,
        app_name: &str,
        state: &str,
    ) -> Result<(), AdkSessionStoreError> {
        self.inner.upsert_app_state(app_name, state)
    }

    pub fn upsert_user_state(
        &self,
        app_name: &str,
        user_id: &str,
        state: &str,
    ) -> Result<(), AdkSessionStoreError> {
        self.inner.upsert_user_state(app_name, user_id, state)
    }
}
