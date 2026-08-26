//! Durable strategy-definition test-cutover adapter.
//!
//! This module is only included by Rust test targets. It uses an isolated
//! fixture schema and is never constructed by the default product profile.

use std::fs::{File, OpenOptions};
use std::io::{Seek, SeekFrom, Write};
use std::path::Path;
use std::time::Duration;

use rusqlite::{OptionalExtension, Transaction, TransactionBehavior};
use serde_json::{Value, json};

use super::product_strategy_definition_write_port::{
    StrategyDefinitionWriteInput, StrategyDefinitionWriteOperation, StrategyDefinitionWritePort,
    StrategyDefinitionWritePortError,
};

/// Durable strategy-definition adapter used only by explicit Rust test-cutover
/// rehearsals. It owns an isolated fixture schema and is never constructed by
/// the default product profile or production composition.
const TEST_CUTOVER_PROFILE: &str = "cutover-test-only.v1";

pub struct StrategyDefinitionSqliteTestCutoverPort {
    path: std::path::PathBuf,
    connection: std::sync::Mutex<rusqlite::Connection>,
    _writer_lease: File,
}

impl std::fmt::Debug for StrategyDefinitionSqliteTestCutoverPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("StrategyDefinitionSqliteTestCutoverPort")
            .field("path", &self.path)
            .finish()
    }
}

#[allow(dead_code)]
impl StrategyDefinitionSqliteTestCutoverPort {
    pub fn open(path: impl AsRef<std::path::Path>) -> Result<Self, String> {
        let path = path.as_ref().to_owned();
        let writer_lease = acquire_writer_lease(&path)?;
        let connection = rusqlite::Connection::open(&path).map_err(|error| error.to_string())?;
        connection
            .busy_timeout(Duration::from_secs(10))
            .map_err(|error| error.to_string())?;
        connection
            .execute_batch(
                "PRAGMA foreign_keys = ON;
                 CREATE TABLE IF NOT EXISTS strategy_definition_test_cutover (
                    id TEXT PRIMARY KEY,
                    version TEXT NOT NULL,
                    payload TEXT NOT NULL,
                    linked_ids TEXT NOT NULL,
                    deleted INTEGER NOT NULL DEFAULT 0
                );
                CREATE TABLE IF NOT EXISTS strategy_definition_test_cutover_versions (
                    definition_id TEXT NOT NULL,
                    version TEXT NOT NULL,
                    payload TEXT NOT NULL,
                    PRIMARY KEY (definition_id, version)
                );
                CREATE TABLE IF NOT EXISTS strategy_definition_test_cutover_ids (
                    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
                    next_value INTEGER NOT NULL
                );
                INSERT OR IGNORE INTO strategy_definition_test_cutover_ids
                    (singleton, next_value) VALUES (1, 1);
                CREATE TABLE IF NOT EXISTS strategy_definition_test_cutover_instances (
                    id TEXT PRIMARY KEY,
                    definition_id TEXT NOT NULL,
                    definition_version TEXT NOT NULL,
                    payload TEXT NOT NULL,
                    binding TEXT NOT NULL,
                    status TEXT NOT NULL DEFAULT 'STOPPED'
                );",
            )
            .map_err(|error| error.to_string())?;
        Ok(Self {
            path,
            connection: std::sync::Mutex::new(connection),
            _writer_lease: writer_lease,
        })
    }

    pub fn seed_definition(
        &self,
        definition_id: &str,
        payload: Value,
        linked_ids: &[&str],
    ) -> Result<(), String> {
        let payload = serde_json::to_string(&payload).map_err(|error| error.to_string())?;
        let linked_ids = serde_json::to_string(linked_ids).map_err(|error| error.to_string())?;
        let mut connection = self
            .connection
            .lock()
            .map_err(|_| "strategy definition fixture lock poisoned".to_owned())?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(|error| error.to_string())?;
        let updated = transaction
            .execute(
                "UPDATE strategy_definition_test_cutover
                 SET version = '0.1.0', payload = ?2, linked_ids = ?3, deleted = 0
                 WHERE id = ?1",
                rusqlite::params![definition_id, &payload, linked_ids],
            )
            .map_err(|error| error.to_string())?;
        if updated == 0 {
            transaction
                .execute(
                    "INSERT INTO strategy_definition_test_cutover
                        (id, version, payload, linked_ids, deleted)
                     VALUES (?1, '0.1.0', ?2, ?3, 0)",
                    rusqlite::params![definition_id, &payload, linked_ids],
                )
                .map_err(|error| error.to_string())?;
        }
        transaction
            .execute(
                "INSERT OR REPLACE INTO strategy_definition_test_cutover_versions
                    (definition_id, version, payload)
                 VALUES (?1, '0.1.0', ?2)",
                rusqlite::params![definition_id, &payload],
            )
            .map_err(|error| error.to_string())?;
        let linked_ids =
            serde_json::from_str::<Vec<String>>(&linked_ids).map_err(|error| error.to_string())?;
        reconcile_linked_instances(&transaction, definition_id, &linked_ids)?;
        transaction.commit().map_err(|error| error.to_string())?;
        Ok(())
    }

    pub fn set_linked_ids(&self, definition_id: &str, linked_ids: &[&str]) -> Result<(), String> {
        let linked_ids_json =
            serde_json::to_string(linked_ids).map_err(|error| error.to_string())?;
        let mut connection = self
            .connection
            .lock()
            .map_err(|_| "strategy definition fixture lock poisoned".to_owned())?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(|error| error.to_string())?;
        transaction
            .execute(
                "UPDATE strategy_definition_test_cutover SET linked_ids = ?2 WHERE id = ?1",
                rusqlite::params![definition_id, &linked_ids_json],
            )
            .map_err(|error| error.to_string())?;
        let linked_ids = linked_ids
            .iter()
            .map(|id| (*id).to_owned())
            .collect::<Vec<_>>();
        reconcile_linked_instances(&transaction, definition_id, &linked_ids)?;
        transaction.commit().map_err(|error| error.to_string())?;
        Ok(())
    }

    pub fn reject_version(&self, version: &str) -> Result<(), String> {
        let connection = self
            .connection
            .lock()
            .map_err(|_| "strategy definition fixture lock poisoned".to_owned())?;
        connection
            .execute_batch("DROP TRIGGER IF EXISTS strategy_definition_test_reject_version")
            .map_err(|error| error.to_string())?;
        let statement = format!(
            "CREATE TRIGGER strategy_definition_test_reject_version
             BEFORE INSERT ON strategy_definition_test_cutover_versions
             WHEN NEW.version = '{}' BEGIN
                 SELECT RAISE(ABORT, 'test-cutover version rejection');
             END",
            version.replace('\'', "''")
        );
        connection
            .execute_batch(&statement)
            .map_err(|error| error.to_string())?;
        Ok(())
    }

    pub fn clear_version_rejection(&self) -> Result<(), String> {
        let connection = self
            .connection
            .lock()
            .map_err(|_| "strategy definition fixture lock poisoned".to_owned())?;
        connection
            .execute_batch("DROP TRIGGER IF EXISTS strategy_definition_test_reject_version")
            .map_err(|error| error.to_string())
    }

    pub fn reject_instance_create(&self) -> Result<(), String> {
        let connection = self
            .connection
            .lock()
            .map_err(|_| "strategy definition fixture lock poisoned".to_owned())?;
        connection
            .execute_batch(
                "DROP TRIGGER IF EXISTS strategy_definition_test_reject_instance;
                 CREATE TRIGGER strategy_definition_test_reject_instance
                 BEFORE INSERT ON strategy_definition_test_cutover_instances
                 BEGIN
                     SELECT RAISE(ABORT, 'test-cutover instance rejection');
                 END;",
            )
            .map_err(|error| error.to_string())
    }

    pub fn clear_instance_rejection(&self) -> Result<(), String> {
        let connection = self
            .connection
            .lock()
            .map_err(|_| "strategy definition fixture lock poisoned".to_owned())?;
        connection
            .execute_batch("DROP TRIGGER IF EXISTS strategy_definition_test_reject_instance")
            .map_err(|error| error.to_string())
    }

    pub fn current(&self, definition_id: &str) -> Result<Option<(String, Value, bool)>, String> {
        let connection = self
            .connection
            .lock()
            .map_err(|_| "strategy definition fixture lock poisoned".to_owned())?;
        connection
            .query_row(
                "SELECT version, payload, deleted
                 FROM strategy_definition_test_cutover WHERE id = ?1",
                rusqlite::params![definition_id],
                |row| {
                    let version: String = row.get(0)?;
                    let payload: String = row.get(1)?;
                    let deleted: i64 = row.get(2)?;
                    Ok((version, payload, deleted != 0))
                },
            )
            .optional()
            .map_err(|error| error.to_string())?
            .map(|(version, payload, deleted)| {
                let payload = serde_json::from_str(&payload).expect("fixture payload JSON");
                (version, payload, deleted)
            })
            .map_or_else(|| Ok(None), |value| Ok(Some(value)))
    }

    pub fn version_count(&self, definition_id: &str) -> Result<u64, String> {
        let connection = self
            .connection
            .lock()
            .map_err(|_| "strategy definition fixture lock poisoned".to_owned())?;
        connection
            .query_row(
                "SELECT COUNT(*) FROM strategy_definition_test_cutover_versions
                 WHERE definition_id = ?1",
                rusqlite::params![definition_id],
                |row| row.get::<_, i64>(0),
            )
            .map_err(|error| error.to_string())
            .and_then(|count| {
                u64::try_from(count).map_err(|_| "negative fixture version count".to_owned())
            })
    }

    pub fn instance_count(&self, definition_id: &str) -> Result<u64, String> {
        let connection = self
            .connection
            .lock()
            .map_err(|_| "strategy definition fixture lock poisoned".to_owned())?;
        connection
            .query_row(
                "SELECT COUNT(*) FROM strategy_definition_test_cutover_instances
                 WHERE definition_id = ?1",
                rusqlite::params![definition_id],
                |row| row.get::<_, i64>(0),
            )
            .map_err(|error| error.to_string())
            .and_then(|count| {
                u64::try_from(count).map_err(|_| "negative fixture instance count".to_owned())
            })
    }

    pub fn instance_ids(&self, definition_id: &str) -> Result<Vec<String>, String> {
        let connection = self
            .connection
            .lock()
            .map_err(|_| "strategy definition fixture lock poisoned".to_owned())?;
        let mut statement = connection
            .prepare(
                "SELECT id FROM strategy_definition_test_cutover_instances
                 WHERE definition_id = ?1 ORDER BY rowid",
            )
            .map_err(|error| error.to_string())?;
        statement
            .query_map(rusqlite::params![definition_id], |row| row.get(0))
            .map_err(|error| error.to_string())?
            .collect::<Result<Vec<String>, _>>()
            .map_err(|error| error.to_string())
    }

    pub fn linked_ids(&self, definition_id: &str) -> Result<Vec<String>, String> {
        let connection = self
            .connection
            .lock()
            .map_err(|_| "strategy definition fixture lock poisoned".to_owned())?;
        let stored: Option<String> = connection
            .query_row(
                "SELECT linked_ids FROM strategy_definition_test_cutover WHERE id = ?1",
                rusqlite::params![definition_id],
                |row| row.get(0),
            )
            .optional()
            .map_err(|error| error.to_string())?;
        stored
            .map(|value| serde_json::from_str(&value).map_err(|error| error.to_string()))
            .unwrap_or_else(|| Ok(Vec::new()))
    }

    fn mutate_create(
        &self,
        input: &StrategyDefinitionWriteInput,
    ) -> Result<Value, StrategyDefinitionWritePortError> {
        let payload = input
            .definition
            .as_ref()
            .ok_or_else(|| strategy_write_failure("invalid definition payload"))?;
        let mut connection = self
            .connection
            .lock()
            .map_err(|_| strategy_write_failure("strategy definition fixture lock poisoned"))?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(|_| strategy_write_failure("failed to save strategy definition"))?;
        let definition_id = allocate_id(&transaction, "definition")?;
        let payload = projection_with_identity(payload, &definition_id, "0.1.0");
        let payload_text = serde_json::to_string(&payload)
            .map_err(|_| strategy_write_failure("failed to save strategy definition"))?;
        transaction
            .execute(
                "INSERT INTO strategy_definition_test_cutover
                    (id, version, payload, linked_ids, deleted)
                 VALUES (?1, '0.1.0', ?2, '[]', 0)",
                rusqlite::params![definition_id, payload_text],
            )
            .map_err(|_| strategy_write_failure("failed to save strategy definition"))?;
        transaction
            .execute(
                "INSERT INTO strategy_definition_test_cutover_versions
                    (definition_id, version, payload) VALUES (?1, '0.1.0', ?2)",
                rusqlite::params![definition_id, payload_text],
            )
            .map_err(|_| strategy_write_failure("failed to save strategy definition"))?;
        transaction
            .commit()
            .map_err(|_| strategy_write_failure("failed to save strategy definition"))?;
        Ok(payload)
    }

    fn mutate_update(
        &self,
        input: &StrategyDefinitionWriteInput,
    ) -> Result<Value, StrategyDefinitionWritePortError> {
        let definition_id = input
            .definition_id
            .as_deref()
            .ok_or_else(|| strategy_write_failure("invalid definition id"))?;
        let payload = input
            .definition
            .as_ref()
            .ok_or_else(|| strategy_write_failure("invalid definition payload"))?;
        let mut connection = self
            .connection
            .lock()
            .map_err(|_| strategy_write_failure("strategy definition fixture lock poisoned"))?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(|_| strategy_write_failure("failed to save strategy definition"))?;
        let current = transaction
            .query_row(
                "SELECT version, payload, deleted FROM strategy_definition_test_cutover WHERE id = ?1",
                rusqlite::params![definition_id],
                |row| Ok((row.get::<_, String>(0)?, row.get::<_, String>(1)?, row.get::<_, i64>(2)?)),
            )
            .optional()
            .map_err(|_| strategy_write_failure("failed to save strategy definition"))?;
        let (version, current_payload, deleted, exists) = current.map_or_else(
            || ("0.1.0".to_owned(), "{}".to_owned(), 0, false),
            |(version, payload, deleted)| (version, payload, deleted, true),
        );
        let current_value: Value = serde_json::from_str(&current_payload)
            .map_err(|_| strategy_write_failure("failed to load strategy definition"))?;
        let next_payload = projection_with_identity(payload, definition_id, &version);
        let same_payload = comparable_payload(&current_value) == comparable_payload(&next_payload);
        let next_version = if current_payload == "{}" || deleted != 0 {
            "0.1.0".to_owned()
        } else if same_payload {
            version.clone()
        } else {
            increment_patch_version(&version)
        };
        let next_payload = projection_with_identity(payload, definition_id, &next_version);
        if !exists {
            let next_payload_text = serde_json::to_string(&next_payload)
                .map_err(|_| strategy_write_failure("failed to save strategy definition"))?;
            transaction
                .execute(
                    "INSERT INTO strategy_definition_test_cutover
                        (id, version, payload, linked_ids, deleted)
                     VALUES (?1, ?2, ?3, '[]', 0)",
                    rusqlite::params![definition_id, next_version, &next_payload_text],
                )
                .map_err(|_| strategy_write_failure("failed to save strategy definition"))?;
            transaction
                .execute(
                    "INSERT INTO strategy_definition_test_cutover_versions
                        (definition_id, version, payload) VALUES (?1, ?2, ?3)",
                    rusqlite::params![definition_id, next_version, &next_payload_text],
                )
                .map_err(|_| strategy_write_failure("failed to save strategy definition"))?;
        } else if next_version != version || deleted != 0 {
            let next_payload_text = serde_json::to_string(&next_payload)
                .map_err(|_| strategy_write_failure("failed to save strategy definition"))?;
            transaction
                .execute(
                    "UPDATE strategy_definition_test_cutover
                     SET version = ?2, payload = ?3, deleted = 0 WHERE id = ?1",
                    rusqlite::params![definition_id, next_version, &next_payload_text],
                )
                .map_err(|_| strategy_write_failure("failed to save strategy definition"))?;
            transaction
                .execute(
                    "INSERT INTO strategy_definition_test_cutover_versions
                        (definition_id, version, payload) VALUES (?1, ?2, ?3)",
                    rusqlite::params![definition_id, next_version, &next_payload_text],
                )
                .map_err(|_| strategy_write_failure("failed to save strategy definition"))?;
        }
        transaction
            .commit()
            .map_err(|_| strategy_write_failure("failed to save strategy definition"))?;
        Ok(next_payload)
    }

    fn mutate_delete(
        &self,
        input: &StrategyDefinitionWriteInput,
    ) -> Result<Value, StrategyDefinitionWritePortError> {
        let definition_id = input
            .definition_id
            .as_deref()
            .ok_or_else(|| strategy_write_failure("invalid definition id"))?;
        let mut connection = self
            .connection
            .lock()
            .map_err(|_| strategy_write_failure("strategy definition fixture lock poisoned"))?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(|_| strategy_write_failure("failed to delete strategy definition"))?;
        let row = transaction
            .query_row(
                "SELECT payload, linked_ids FROM strategy_definition_test_cutover
                 WHERE id = ?1 AND deleted = 0",
                rusqlite::params![definition_id],
                |row| Ok((row.get::<_, String>(0)?, row.get::<_, String>(1)?)),
            )
            .optional()
            .map_err(|_| strategy_write_failure("failed to delete strategy definition"))?
            .ok_or_else(strategy_not_found)?;
        let linked_ids = linked_ids_for(&transaction, definition_id, &row.1)?;
        if !linked_ids.is_empty() {
            return Err(StrategyDefinitionWritePortError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: format!(
                    "当前有 {} 个实例仍关联该策略，请先删除对应实例再删除。实例: {}",
                    linked_ids.len(),
                    linked_ids.join(", ")
                ),
            });
        }
        transaction
            .execute(
                "UPDATE strategy_definition_test_cutover SET deleted = 1 WHERE id = ?1",
                rusqlite::params![definition_id],
            )
            .map_err(|_| strategy_write_failure("failed to delete strategy definition"))?;
        transaction
            .commit()
            .map_err(|_| strategy_write_failure("failed to delete strategy definition"))?;
        let payload: Value = serde_json::from_str(&row.0)
            .map_err(|_| strategy_write_failure("failed to delete strategy definition"))?;
        Ok(payload)
    }

    fn mutate_apply(
        &self,
        input: &StrategyDefinitionWriteInput,
    ) -> Result<Value, StrategyDefinitionWritePortError> {
        let definition_id = input
            .definition_id
            .as_deref()
            .ok_or_else(|| strategy_write_failure("invalid definition id"))?;
        let mut connection = self
            .connection
            .lock()
            .map_err(|_| strategy_write_failure("strategy definition fixture lock poisoned"))?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(|_| strategy_write_failure("definition store unavailable"))?;
        let definition = transaction
            .query_row(
                "SELECT version, payload FROM strategy_definition_test_cutover
                 WHERE id = ?1 AND deleted = 0",
                rusqlite::params![definition_id],
                |row| Ok((row.get::<_, String>(0)?, row.get::<_, String>(1)?)),
            )
            .optional()
            .map_err(|_| strategy_write_failure("definition store unavailable"))?
            .ok_or_else(strategy_not_found)?;
        let definition_payload: Value = serde_json::from_str(&definition.1)
            .map_err(|_| strategy_write_failure("definition store unavailable"))?;
        let linked = load_instances(&transaction, definition_id)?;
        let mut applied = Vec::new();
        let mut already_latest = Vec::new();
        let mut skipped_busy = Vec::new();
        for (instance_id, version, status) in &linked {
            if status != "STOPPED" {
                skipped_busy.push(instance_id.clone());
            } else if version == &definition.0 {
                already_latest.push(instance_id.clone());
            } else {
                let payload_text = serde_json::to_string(&definition_payload)
                    .map_err(|_| strategy_write_failure("failed to apply linked instances"))?;
                transaction
                    .execute(
                        "UPDATE strategy_definition_test_cutover_instances
                         SET definition_version = ?2, payload = ?3 WHERE id = ?1",
                        rusqlite::params![instance_id, &definition.0, payload_text],
                    )
                    .map_err(|_| strategy_write_failure("failed to apply linked instances"))?;
                applied.push(instance_id.clone());
            }
        }
        transaction
            .commit()
            .map_err(|_| strategy_write_failure("failed to apply linked instances"))?;
        Ok(json!({
            "definitionId": definition_id,
            "latestVersion": definition.0,
            "totalLinked": linked.len(),
            "applied": applied,
            "alreadyLatest": already_latest,
            "skippedBusy": skipped_busy,
        }))
    }

    fn mutate_instantiate(
        &self,
        input: &StrategyDefinitionWriteInput,
    ) -> Result<Value, StrategyDefinitionWritePortError> {
        let definition_id = input
            .definition_id
            .as_deref()
            .ok_or_else(|| strategy_write_failure("invalid definition id"))?;
        let mut connection = self
            .connection
            .lock()
            .map_err(|_| strategy_write_failure("strategy definition fixture lock poisoned"))?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(|_| strategy_write_failure("definition store unavailable"))?;
        let (version, payload, stored_linked_ids) = transaction
            .query_row(
                "SELECT version, payload, linked_ids FROM strategy_definition_test_cutover
                 WHERE id = ?1 AND deleted = 0",
                rusqlite::params![definition_id],
                |row| {
                    Ok((
                        row.get::<_, String>(0)?,
                        row.get::<_, String>(1)?,
                        row.get::<_, String>(2)?,
                    ))
                },
            )
            .optional()
            .map_err(|_| strategy_write_failure("definition store unavailable"))?
            .ok_or_else(strategy_not_found)?;
        if let Some(message) = input.binding_error.as_deref() {
            return Err(StrategyDefinitionWritePortError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: message.to_owned(),
            });
        }
        let instance_id = allocate_id(&transaction, "instance")?;
        let binding = input.binding.clone().unwrap_or_else(|| json!({}));
        let payload_text = serde_json::to_string(&payload)
            .map_err(|_| strategy_write_failure("failed to instantiate strategy"))?;
        let binding_text = serde_json::to_string(&binding)
            .map_err(|_| strategy_write_failure("failed to instantiate strategy"))?;
        transaction
            .execute(
                "INSERT INTO strategy_definition_test_cutover_instances
                    (id, definition_id, definition_version, payload, binding, status)
                 VALUES (?1, ?2, ?3, ?4, ?5, 'STOPPED')",
                rusqlite::params![
                    &instance_id,
                    definition_id,
                    &version,
                    &payload_text,
                    &binding_text,
                ],
            )
            .map_err(|_| strategy_write_failure("failed to instantiate strategy"))?;
        let mut linked_ids = linked_ids_for(&transaction, definition_id, &stored_linked_ids)?;
        if !linked_ids.iter().any(|id| id == &instance_id) {
            linked_ids.push(instance_id.clone());
        }
        let linked_ids_text = serde_json::to_string(&linked_ids)
            .map_err(|_| strategy_write_failure("failed to instantiate strategy"))?;
        transaction
            .execute(
                "UPDATE strategy_definition_test_cutover SET linked_ids = ?2 WHERE id = ?1",
                rusqlite::params![definition_id, linked_ids_text],
            )
            .map_err(|_| strategy_write_failure("failed to instantiate strategy"))?;
        transaction
            .commit()
            .map_err(|_| strategy_write_failure("failed to instantiate strategy"))?;
        Ok(json!({
            "id": instance_id,
            "definitionId": definition_id,
            "definitionVersion": version,
            "definition": payload,
            "binding": binding,
            "status": "STOPPED",
        }))
    }
}

impl StrategyDefinitionWritePort for StrategyDefinitionSqliteTestCutoverPort {
    fn mutate(
        &self,
        input: &StrategyDefinitionWriteInput,
    ) -> Result<Value, StrategyDefinitionWritePortError> {
        match input.operation {
            StrategyDefinitionWriteOperation::Create => self.mutate_create(input),
            StrategyDefinitionWriteOperation::Update => self.mutate_update(input),
            StrategyDefinitionWriteOperation::Delete => self.mutate_delete(input),
            StrategyDefinitionWriteOperation::ApplyLinkedInstances => self.mutate_apply(input),
            StrategyDefinitionWriteOperation::Instantiate => self.mutate_instantiate(input),
        }
    }
}

include!("product_strategy_definition_write_test_cutover_support.rs");

fn projection_with_identity(payload: &Value, definition_id: &str, version: &str) -> Value {
    let mut payload = payload.clone();
    if let Value::Object(object) = &mut payload {
        object.insert("id".to_owned(), Value::String(definition_id.to_owned()));
        object.insert("version".to_owned(), Value::String(version.to_owned()));
    }
    payload
}

fn comparable_payload(payload: &Value) -> Value {
    let mut payload = payload.clone();
    if let Value::Object(object) = &mut payload {
        object.remove("id");
        object.remove("version");
    }
    payload
}

fn increment_patch_version(version: &str) -> String {
    let mut parts = version
        .split('.')
        .map(|part| part.parse::<u64>().unwrap_or(0));
    let major = parts.next().unwrap_or(0);
    let minor = parts.next().unwrap_or(0);
    let patch = parts.next().unwrap_or(0).saturating_add(1);
    format!("{major}.{minor}.{patch}")
}

fn strategy_write_failure(message: &str) -> StrategyDefinitionWritePortError {
    StrategyDefinitionWritePortError::Failed {
        status: 500,
        code: "STRATEGY_FAILED".to_owned(),
        message: message.to_owned(),
    }
}

fn strategy_not_found() -> StrategyDefinitionWritePortError {
    StrategyDefinitionWritePortError::Failed {
        status: 404,
        code: "NOT_FOUND".to_owned(),
        message: "strategy resource not found".to_owned(),
    }
}
