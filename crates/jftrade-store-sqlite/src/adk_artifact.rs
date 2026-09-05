use std::path::{Path, PathBuf};
use std::sync::{Mutex, MutexGuard};
use std::time::Duration;

use jftrade_owner_lock::{OwnerDiagnostic, WriterLease, WriterLeaseError};
use rusqlite::{Connection, OpenFlags, OptionalExtension, params};
use serde::{Deserialize, Serialize};
use thiserror::Error;
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

use crate::schema_manifest::{SchemaManifestError, validate_current};

const ADK_ARTIFACT_COMPONENT: &str = "adk-artifact";
const ADK_ARTIFACT_SCHEMA_VERSION: i64 = 1;
pub const ADK_ARTIFACT_TEST_CUTOVER_PROFILE: &str = "cutover-test-only.v1";
pub const ADK_ARTIFACT_PRODUCTION_PROFILE: &str = "production.v1";

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredAdkArtifact {
    pub app_name: String,
    pub user_id: String,
    pub session_id: String,
    pub file_name: String,
    pub version: i64,
    pub part_json: String,
    pub mime_type: String,
    pub custom_metadata_json: Option<String>,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct PutAdkArtifactParams<'a> {
    pub app_name: &'a str,
    pub user_id: &'a str,
    pub session_id: &'a str,
    pub file_name: &'a str,
    pub version: i64,
    pub part_json: &'a str,
    pub mime_type: &'a str,
    pub custom_metadata_json: Option<&'a str>,
}

#[derive(Debug, Error)]
pub enum AdkArtifactStoreError {
    #[error("adk-artifact database path is required")]
    EmptyPath,
    #[error("unsupported adk-artifact writer profile: {0}")]
    UnsupportedProfile(String),
    #[error("adk-artifact database is not an existing regular file: {0}")]
    NotRegularFile(String),
    #[error(transparent)]
    WriterLease(#[from] WriterLeaseError),
    #[error("open adk-artifact database: {0}")]
    Open(#[source] rusqlite::Error),
    #[error("configure adk-artifact database: {0}")]
    Configure(#[source] rusqlite::Error),
    #[error(transparent)]
    Schema(#[from] SchemaManifestError),
    #[error("adk-artifact database lock is unavailable")]
    LockUnavailable,
    #[error("query adk-artifact database: {0}")]
    Query(#[source] rusqlite::Error),
    #[error("adk artifact not found: {0}")]
    NotFound(String),
    #[error("incompatible adk-artifact database: {0}")]
    Incompatible(String),
}

pub struct AdkArtifactStore {
    path: PathBuf,
    connection: Mutex<Connection>,
    _writer_lease: WriterLease,
}

impl std::fmt::Debug for AdkArtifactStore {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("AdkArtifactStore")
            .field("path", &self.path)
            .finish()
    }
}

impl AdkArtifactStore {
    pub fn delete_session_artifacts(
        &self,
        app_name: &str,
        user_id: &str,
        session_id: &str,
    ) -> Result<usize, AdkArtifactStoreError> {
        let connection = self.lock_connection()?;
        connection
            .execute(
                "DELETE FROM artifacts WHERE app_name = ?1 AND user_id = ?2 AND session_id = ?3",
                params![app_name, user_id, session_id],
            )
            .map_err(AdkArtifactStoreError::Query)
    }
    pub fn open(path: impl AsRef<Path>) -> Result<Self, AdkArtifactStoreError> {
        Self::open_existing(path, ADK_ARTIFACT_PRODUCTION_PROFILE)
    }

    pub fn open_existing(
        path: impl AsRef<Path>,
        profile: &str,
    ) -> Result<Self, AdkArtifactStoreError> {
        let path = path.as_ref();
        if path.as_os_str().is_empty() {
            return Err(AdkArtifactStoreError::EmptyPath);
        }
        if profile != ADK_ARTIFACT_TEST_CUTOVER_PROFILE
            && profile != ADK_ARTIFACT_PRODUCTION_PROFILE
        {
            return Err(AdkArtifactStoreError::UnsupportedProfile(
                profile.to_owned(),
            ));
        }
        if !path
            .metadata()
            .map(|metadata| metadata.is_file())
            .unwrap_or(false)
        {
            return Err(AdkArtifactStoreError::NotRegularFile(
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
        .map_err(AdkArtifactStoreError::Open)?;

        connection
            .busy_timeout(Duration::from_millis(5_000))
            .map_err(AdkArtifactStoreError::Configure)?;
        connection
            .pragma_update(None, "foreign_keys", "ON")
            .map_err(AdkArtifactStoreError::Configure)?;

        validate_current(
            &connection,
            &path.display().to_string(),
            ADK_ARTIFACT_COMPONENT,
            ADK_ARTIFACT_SCHEMA_VERSION,
        )?;

        Ok(Self {
            path: path.to_path_buf(),
            connection: Mutex::new(connection),
            _writer_lease: writer_lease,
        })
    }

    fn lock_connection(&self) -> Result<MutexGuard<'_, Connection>, AdkArtifactStoreError> {
        self.connection
            .lock()
            .map_err(|_| AdkArtifactStoreError::LockUnavailable)
    }

    fn now_rfc3339() -> String {
        OffsetDateTime::now_utc()
            .format(&Rfc3339)
            .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned())
    }

    pub fn put_artifact(
        &self,
        params: PutAdkArtifactParams<'_>,
    ) -> Result<StoredAdkArtifact, AdkArtifactStoreError> {
        let now = Self::now_rfc3339();
        let connection = self.lock_connection()?;
        connection
            .execute(
                "INSERT INTO artifacts (app_name, user_id, session_id, file_name, version, part_json, mime_type, custom_metadata_json, created_at, updated_at)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?9)
                 ON CONFLICT(app_name, user_id, session_id, file_name, version) DO UPDATE SET
                   part_json = ?6, mime_type = ?7, custom_metadata_json = ?8, updated_at = ?9",
                params![
                    params.app_name,
                    params.user_id,
                    params.session_id,
                    params.file_name,
                    params.version,
                    params.part_json,
                    params.mime_type,
                    params.custom_metadata_json,
                    now,
                ],
            )
            .map_err(AdkArtifactStoreError::Query)?;

        Ok(StoredAdkArtifact {
            app_name: params.app_name.to_owned(),
            user_id: params.user_id.to_owned(),
            session_id: params.session_id.to_owned(),
            file_name: params.file_name.to_owned(),
            version: params.version,
            part_json: params.part_json.to_owned(),
            mime_type: params.mime_type.to_owned(),
            custom_metadata_json: params.custom_metadata_json.map(ToOwned::to_owned),
            created_at: now.clone(),
            updated_at: now,
        })
    }

    pub fn get_artifact(
        &self,
        app_name: &str,
        user_id: &str,
        session_id: &str,
        file_name: &str,
        version: i64,
    ) -> Result<Option<StoredAdkArtifact>, AdkArtifactStoreError> {
        let connection = self.lock_connection()?;
        connection
            .query_row(
                "SELECT app_name, user_id, session_id, file_name, version, part_json, mime_type, custom_metadata_json, created_at, updated_at
                 FROM artifacts
                 WHERE app_name = ?1 AND user_id = ?2 AND session_id = ?3 AND file_name = ?4 AND version = ?5",
                params![app_name, user_id, session_id, file_name, version],
                |row| {
                    Ok(StoredAdkArtifact {
                        app_name: row.get(0)?,
                        user_id: row.get(1)?,
                        session_id: row.get(2)?,
                        file_name: row.get(3)?,
                        version: row.get(4)?,
                        part_json: row.get(5)?,
                        mime_type: row.get(6)?,
                        custom_metadata_json: row.get(7)?,
                        created_at: row.get(8)?,
                        updated_at: row.get(9)?,
                    })
                },
            )
            .optional()
            .map_err(AdkArtifactStoreError::Query)
    }

    pub fn list_session_artifacts(
        &self,
        session_id: &str,
    ) -> Result<Vec<StoredAdkArtifact>, AdkArtifactStoreError> {
        let connection = self.lock_connection()?;
        let mut statement = connection
            .prepare(
                "SELECT app_name, user_id, session_id, file_name, version, part_json, mime_type, custom_metadata_json, created_at, updated_at
                 FROM artifacts WHERE session_id = ?1 ORDER BY file_name ASC, version DESC",
            )
            .map_err(AdkArtifactStoreError::Query)?;
        let rows = statement
            .query_map(params![session_id], |row| {
                Ok(StoredAdkArtifact {
                    app_name: row.get(0)?,
                    user_id: row.get(1)?,
                    session_id: row.get(2)?,
                    file_name: row.get(3)?,
                    version: row.get(4)?,
                    part_json: row.get(5)?,
                    mime_type: row.get(6)?,
                    custom_metadata_json: row.get(7)?,
                    created_at: row.get(8)?,
                    updated_at: row.get(9)?,
                })
            })
            .map_err(AdkArtifactStoreError::Query)?;
        rows.map(|row| row.map_err(AdkArtifactStoreError::Query))
            .collect()
    }
}

#[derive(Debug)]
pub struct AdkArtifactTestCutoverStore {
    inner: AdkArtifactStore,
}

impl AdkArtifactTestCutoverStore {
    pub fn open_existing(
        path: impl AsRef<Path>,
        profile: &str,
    ) -> Result<Self, AdkArtifactStoreError> {
        let inner = AdkArtifactStore::open_existing(path, profile)?;
        Ok(Self { inner })
    }

    pub fn path(&self) -> &Path {
        &self.inner.path
    }

    pub fn put_artifact(
        &self,
        params: PutAdkArtifactParams<'_>,
    ) -> Result<StoredAdkArtifact, AdkArtifactStoreError> {
        self.inner.put_artifact(params)
    }

    pub fn get_artifact(
        &self,
        app_name: &str,
        user_id: &str,
        session_id: &str,
        file_name: &str,
        version: i64,
    ) -> Result<Option<StoredAdkArtifact>, AdkArtifactStoreError> {
        self.inner
            .get_artifact(app_name, user_id, session_id, file_name, version)
    }
}
