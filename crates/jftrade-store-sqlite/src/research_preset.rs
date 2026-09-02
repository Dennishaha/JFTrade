use std::path::{Path, PathBuf};
use std::sync::{Mutex, MutexGuard};
use std::time::Duration;

use jftrade_owner_lock::{OwnerDiagnostic, WriterLease, WriterLeaseError};
use rusqlite::{Connection, ErrorCode, OpenFlags, OptionalExtension, params};
use serde::Serialize;
use serde_json::Value;
use thiserror::Error;
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

use crate::schema_manifest::{SchemaManifestError, validate_current};

const RESEARCH_COMPONENT: &str = "research";
const RESEARCH_SCHEMA_VERSION: i64 = 1;
const QUERY_SCHEMA_VERSION: u32 = 2;
const MAX_PRESET_NAME_CHARS: usize = 80;
pub const RESEARCH_PRESET_TEST_CUTOVER_PROFILE: &str = "cutover-test-only.v1";
pub const RESEARCH_PRESET_PRODUCTION_PROFILE: &str = "production.v1";

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ResearchPresetMutation {
    pub preset_id: String,
    pub name: String,
    pub query_schema_version: u32,
    pub definition: Value,
    pub revision: u64,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredResearchPreset {
    #[serde(flatten)]
    pub preset: ResearchPresetMutation,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Debug, Error)]
pub enum ResearchPresetStoreError {
    #[error("research preset database path is required")]
    EmptyPath,
    #[error("unsupported research preset writer profile: {0}")]
    UnsupportedProfile(String),
    #[error("research preset database is not an existing regular file: {0}")]
    NotRegularFile(String),
    #[error(transparent)]
    WriterLease(#[from] WriterLeaseError),
    #[error("open research preset database: {0}")]
    Open(#[source] rusqlite::Error),
    #[error("configure research preset database: {0}")]
    Configure(#[source] rusqlite::Error),
    #[error(transparent)]
    Schema(#[from] SchemaManifestError),
    #[error("research preset database lock is unavailable")]
    LockUnavailable,
    #[error("query research preset database: {0}")]
    Query(#[source] rusqlite::Error),
    #[error("research preset not found")]
    NotFound,
    #[error("research preset revision or name conflict")]
    Conflict,
    #[error("research preset database contains incompatible data: {0}")]
    Incompatible(String),
}

pub struct ResearchPresetStore {
    path: PathBuf,
    connection: Mutex<Connection>,
    _writer_lease: WriterLease,
}

impl std::fmt::Debug for ResearchPresetStore {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ResearchPresetStore")
            .field("path", &self.path)
            .finish()
    }
}

impl ResearchPresetStore {
    pub fn open(path: impl AsRef<Path>) -> Result<Self, ResearchPresetStoreError> {
        Self::open_existing(path, RESEARCH_PRESET_PRODUCTION_PROFILE)
    }

    pub fn open_existing(
        path: impl AsRef<Path>,
        profile: &str,
    ) -> Result<Self, ResearchPresetStoreError> {
        if profile != RESEARCH_PRESET_TEST_CUTOVER_PROFILE
            && profile != RESEARCH_PRESET_PRODUCTION_PROFILE
        {
            return Err(ResearchPresetStoreError::UnsupportedProfile(
                profile.to_owned(),
            ));
        }
        let path = path.as_ref();
        if path.as_os_str().is_empty() {
            return Err(ResearchPresetStoreError::EmptyPath);
        }
        if !path
            .metadata()
            .map(|metadata| metadata.is_file())
            .unwrap_or(false)
        {
            return Err(ResearchPresetStoreError::NotRegularFile(
                path.display().to_string(),
            ));
        }
        let writer_lease = WriterLease::acquire(path, &OwnerDiagnostic::current("rust", profile))?;
        let connection = Connection::open_with_flags(
            path,
            OpenFlags::SQLITE_OPEN_READ_WRITE | OpenFlags::SQLITE_OPEN_NO_MUTEX,
        )
        .map_err(ResearchPresetStoreError::Open)?;
        connection
            .execute_batch("PRAGMA foreign_keys = ON;")
            .map_err(ResearchPresetStoreError::Configure)?;
        connection
            .busy_timeout(Duration::from_secs(10))
            .map_err(ResearchPresetStoreError::Configure)?;
        validate_current(
            &connection,
            &path.display().to_string(),
            RESEARCH_COMPONENT,
            RESEARCH_SCHEMA_VERSION,
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

    pub fn list(&self) -> Result<Vec<StoredResearchPreset>, ResearchPresetStoreError> {
        let connection = self.connection()?;
        let mut statement = connection
            .prepare(
                "SELECT preset_id, name, query_schema_version, query_json, revision, created_at, updated_at \
                 FROM research_screen_presets ORDER BY updated_at DESC, preset_id",
            )
            .map_err(ResearchPresetStoreError::Query)?;
        let rows = statement
            .query_map([], read_raw_preset)
            .map_err(ResearchPresetStoreError::Query)?
            .collect::<Result<Vec<_>, _>>()
            .map_err(ResearchPresetStoreError::Query)?;
        rows.into_iter().map(validate_raw_preset).collect()
    }

    pub fn get(&self, preset_id: &str) -> Result<StoredResearchPreset, ResearchPresetStoreError> {
        let connection = self.connection()?;
        get_with_connection(&connection, preset_id)
    }

    pub fn insert(
        &self,
        preset: &ResearchPresetMutation,
        timestamp: &str,
    ) -> Result<StoredResearchPreset, ResearchPresetStoreError> {
        validate_write_preset(preset, 1)?;
        validate_timestamp(timestamp, "createdAt")?;
        let definition = serde_json::to_string(&preset.definition)
            .map_err(|error| ResearchPresetStoreError::Incompatible(error.to_string()))?;
        let connection = self.connection()?;
        connection
            .execute(
                "INSERT INTO research_screen_presets \
                 (preset_id, name, name_key, query_schema_version, query_json, revision, created_at, updated_at) \
                 VALUES (?1, ?2, ?3, ?4, ?5, 1, ?6, ?6)",
                params![
                    preset.preset_id,
                    preset.name,
                    preset_name_key(&preset.name),
                    i64::from(preset.query_schema_version),
                    definition,
                    timestamp,
                ],
            )
            .map_err(map_write_error)?;
        get_with_connection(&connection, &preset.preset_id)
    }

    pub fn replace_revision(
        &self,
        preset: &ResearchPresetMutation,
        expected_revision: u64,
        timestamp: &str,
    ) -> Result<StoredResearchPreset, ResearchPresetStoreError> {
        let next_revision = expected_revision.checked_add(1).ok_or_else(|| {
            ResearchPresetStoreError::Incompatible("revision overflow".to_owned())
        })?;
        validate_write_preset(preset, next_revision)?;
        validate_timestamp(timestamp, "updatedAt")?;
        let definition = serde_json::to_string(&preset.definition)
            .map_err(|error| ResearchPresetStoreError::Incompatible(error.to_string()))?;
        let expected_revision = i64::try_from(expected_revision).map_err(|_| {
            ResearchPresetStoreError::Incompatible("expected revision exceeds SQLite".to_owned())
        })?;
        let connection = self.connection()?;
        let affected = connection
            .execute(
                "UPDATE research_screen_presets SET \
                 name = ?1, name_key = ?2, query_schema_version = ?3, query_json = ?4, \
                 revision = revision + 1, updated_at = ?5 WHERE preset_id = ?6 AND revision = ?7",
                params![
                    preset.name,
                    preset_name_key(&preset.name),
                    i64::from(preset.query_schema_version),
                    definition,
                    timestamp,
                    preset.preset_id,
                    expected_revision,
                ],
            )
            .map_err(map_write_error)?;
        if affected == 0 {
            return if preset_exists(&connection, &preset.preset_id)? {
                Err(ResearchPresetStoreError::Conflict)
            } else {
                Err(ResearchPresetStoreError::NotFound)
            };
        }
        get_with_connection(&connection, &preset.preset_id)
    }

    pub fn delete(&self, preset_id: &str) -> Result<(), ResearchPresetStoreError> {
        let connection = self.connection()?;
        let affected = connection
            .execute(
                "DELETE FROM research_screen_presets WHERE preset_id = ?1",
                [preset_id.trim()],
            )
            .map_err(map_write_error)?;
        if affected == 0 {
            return Err(ResearchPresetStoreError::NotFound);
        }
        Ok(())
    }

    fn connection(&self) -> Result<MutexGuard<'_, Connection>, ResearchPresetStoreError> {
        self.connection
            .lock()
            .map_err(|_| ResearchPresetStoreError::LockUnavailable)
    }
}

struct RawResearchPreset {
    preset_id: String,
    name: String,
    query_schema_version: i64,
    definition: String,
    revision: i64,
    created_at: String,
    updated_at: String,
}

fn read_raw_preset(row: &rusqlite::Row<'_>) -> Result<RawResearchPreset, rusqlite::Error> {
    Ok(RawResearchPreset {
        preset_id: row.get(0)?,
        name: row.get(1)?,
        query_schema_version: row.get(2)?,
        definition: row.get(3)?,
        revision: row.get(4)?,
        created_at: row.get(5)?,
        updated_at: row.get(6)?,
    })
}

fn validate_raw_preset(
    raw: RawResearchPreset,
) -> Result<StoredResearchPreset, ResearchPresetStoreError> {
    if raw.query_schema_version != i64::from(QUERY_SCHEMA_VERSION) || raw.revision <= 0 {
        return Err(ResearchPresetStoreError::Incompatible(format!(
            "preset {} has schema version {} and revision {}",
            raw.preset_id, raw.query_schema_version, raw.revision
        )));
    }
    validate_timestamp(&raw.created_at, "createdAt")?;
    validate_timestamp(&raw.updated_at, "updatedAt")?;
    let definition: Value = serde_json::from_str(&raw.definition).map_err(|error| {
        ResearchPresetStoreError::Incompatible(format!(
            "preset {} definition is invalid JSON: {error}",
            raw.preset_id
        ))
    })?;
    let preset = ResearchPresetMutation {
        preset_id: raw.preset_id.clone(),
        name: raw.name,
        query_schema_version: QUERY_SCHEMA_VERSION,
        definition,
        revision: u64::try_from(raw.revision).map_err(|_| {
            ResearchPresetStoreError::Incompatible(format!(
                "preset {} revision is outside the supported range",
                raw.preset_id
            ))
        })?,
    };
    validate_write_preset(&preset, preset.revision).map_err(|_| {
        ResearchPresetStoreError::Incompatible(format!(
            "preset {} contains non-normalized or invalid data",
            raw.preset_id
        ))
    })?;
    Ok(StoredResearchPreset {
        preset,
        created_at: raw.created_at,
        updated_at: raw.updated_at,
    })
}

fn validate_write_preset(
    preset: &ResearchPresetMutation,
    expected_revision: u64,
) -> Result<(), ResearchPresetStoreError> {
    if preset.preset_id.is_empty()
        || preset.preset_id.trim() != preset.preset_id
        || preset.name.is_empty()
        || preset.name.trim() != preset.name
        || preset.name.chars().count() > MAX_PRESET_NAME_CHARS
        || !preset.definition.is_object()
        || preset.query_schema_version != QUERY_SCHEMA_VERSION
        || preset.revision != expected_revision
    {
        return Err(ResearchPresetStoreError::Incompatible(
            "preset is not normalized for the expected revision".to_owned(),
        ));
    }
    Ok(())
}

fn get_with_connection(
    connection: &Connection,
    preset_id: &str,
) -> Result<StoredResearchPreset, ResearchPresetStoreError> {
    let raw = connection
        .query_row(
            "SELECT preset_id, name, query_schema_version, query_json, revision, created_at, updated_at \
             FROM research_screen_presets WHERE preset_id = ?1",
            [preset_id.trim()],
            read_raw_preset,
        )
        .optional()
        .map_err(ResearchPresetStoreError::Query)?
        .ok_or(ResearchPresetStoreError::NotFound)?;
    validate_raw_preset(raw)
}

fn preset_exists(
    connection: &Connection,
    preset_id: &str,
) -> Result<bool, ResearchPresetStoreError> {
    connection
        .query_row(
            "SELECT 1 FROM research_screen_presets WHERE preset_id = ?1",
            [preset_id.trim()],
            |_| Ok(()),
        )
        .optional()
        .map(|value| value.is_some())
        .map_err(ResearchPresetStoreError::Query)
}

fn preset_name_key(value: &str) -> String {
    value.trim().to_lowercase()
}

fn validate_timestamp(value: &str, field: &str) -> Result<(), ResearchPresetStoreError> {
    OffsetDateTime::parse(value, &Rfc3339).map_err(|error| {
        ResearchPresetStoreError::Incompatible(format!("{field} timestamp is invalid: {error}"))
    })?;
    Ok(())
}

fn map_write_error(error: rusqlite::Error) -> ResearchPresetStoreError {
    if matches!(
        error,
        rusqlite::Error::SqliteFailure(ref failure, _)
            if failure.code == ErrorCode::ConstraintViolation
    ) {
        ResearchPresetStoreError::Conflict
    } else {
        ResearchPresetStoreError::Query(error)
    }
}

#[derive(Debug)]
pub struct ResearchPresetTestCutoverStore {
    inner: ResearchPresetStore,
}

impl ResearchPresetTestCutoverStore {
    pub fn open_existing(
        path: impl AsRef<Path>,
        profile: &str,
    ) -> Result<Self, ResearchPresetStoreError> {
        let inner = ResearchPresetStore::open_existing(path, profile)?;
        Ok(Self { inner })
    }

    pub fn path(&self) -> &Path {
        self.inner.path()
    }

    pub fn list(&self) -> Result<Vec<StoredResearchPreset>, ResearchPresetStoreError> {
        self.inner.list()
    }

    pub fn get(&self, preset_id: &str) -> Result<StoredResearchPreset, ResearchPresetStoreError> {
        self.inner.get(preset_id)
    }

    pub fn insert(
        &self,
        preset: &ResearchPresetMutation,
        timestamp: &str,
    ) -> Result<StoredResearchPreset, ResearchPresetStoreError> {
        self.inner.insert(preset, timestamp)
    }

    pub fn replace_revision(
        &self,
        preset: &ResearchPresetMutation,
        expected_revision: u64,
        timestamp: &str,
    ) -> Result<StoredResearchPreset, ResearchPresetStoreError> {
        self.inner
            .replace_revision(preset, expected_revision, timestamp)
    }

    pub fn delete(&self, preset_id: &str) -> Result<(), ResearchPresetStoreError> {
        self.inner.delete(preset_id)
    }
}
