//! Durable strategy-definition test-cutover adapter.
//!
//! This module is only included by Rust test targets. It uses an isolated
//! fixture schema and is never constructed by the default product profile.

use serde_json::{Value, json};

use super::product_strategy_definition_write_port::{
    StrategyDefinitionWriteInput, StrategyDefinitionWriteOperation, StrategyDefinitionWritePort,
    StrategyDefinitionWritePortError,
};

/// Durable strategy-definition adapter used only by explicit Rust test-cutover
/// rehearsals. It owns an isolated fixture schema and is never constructed by
/// the default product profile or production composition.
use rusqlite::OptionalExtension;

pub struct StrategyDefinitionSqliteTestCutoverPort {
    path: std::path::PathBuf,
    connection: std::sync::Mutex<rusqlite::Connection>,
    next_id: std::sync::atomic::AtomicU64,
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
        let connection = rusqlite::Connection::open(&path).map_err(|error| error.to_string())?;
        connection
            .execute_batch(
                "CREATE TABLE IF NOT EXISTS strategy_definition_test_cutover (
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
                );",
            )
            .map_err(|error| error.to_string())?;
        Ok(Self {
            path,
            connection: std::sync::Mutex::new(connection),
            next_id: std::sync::atomic::AtomicU64::new(1),
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
        let connection = self
            .connection
            .lock()
            .map_err(|_| "strategy definition fixture lock poisoned".to_owned())?;
        connection
            .execute(
                "INSERT OR REPLACE INTO strategy_definition_test_cutover
                    (id, version, payload, linked_ids, deleted)
                 VALUES (?1, '0.1.0', ?2, ?3, 0)",
                rusqlite::params![definition_id, &payload, linked_ids],
            )
            .map_err(|error| error.to_string())?;
        connection
            .execute(
                "INSERT OR REPLACE INTO strategy_definition_test_cutover_versions
                    (definition_id, version, payload)
                 VALUES (?1, '0.1.0', ?2)",
                rusqlite::params![definition_id, &payload],
            )
            .map_err(|error| error.to_string())?;
        Ok(())
    }

    pub fn set_linked_ids(&self, definition_id: &str, linked_ids: &[&str]) -> Result<(), String> {
        let linked_ids = serde_json::to_string(linked_ids).map_err(|error| error.to_string())?;
        let connection = self
            .connection
            .lock()
            .map_err(|_| "strategy definition fixture lock poisoned".to_owned())?;
        connection
            .execute(
                "UPDATE strategy_definition_test_cutover SET linked_ids = ?2 WHERE id = ?1",
                rusqlite::params![definition_id, linked_ids],
            )
            .map_err(|error| error.to_string())?;
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

    fn mutate_create(
        &self,
        input: &StrategyDefinitionWriteInput,
    ) -> Result<Value, StrategyDefinitionWritePortError> {
        let payload = input
            .definition
            .as_ref()
            .ok_or_else(|| strategy_write_failure("invalid definition payload"))?;
        let definition_id = format!(
            "definition-test-{}",
            self.next_id
                .fetch_add(1, std::sync::atomic::Ordering::Relaxed)
        );
        let payload = projection_with_identity(payload, &definition_id, "0.1.0");
        let payload_text = serde_json::to_string(&payload)
            .map_err(|_| strategy_write_failure("failed to save strategy definition"))?;
        let connection = self
            .connection
            .lock()
            .map_err(|_| strategy_write_failure("strategy definition fixture lock poisoned"))?;
        let transaction = connection
            .unchecked_transaction()
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
        let connection = self
            .connection
            .lock()
            .map_err(|_| strategy_write_failure("strategy definition fixture lock poisoned"))?;
        let transaction = connection
            .unchecked_transaction()
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
            transaction
                .execute(
                    "INSERT INTO strategy_definition_test_cutover
                        (id, version, payload, linked_ids, deleted)
                     VALUES (?1, ?2, ?3, '[]', 0)",
                    rusqlite::params![
                        definition_id,
                        next_version,
                        &serde_json::to_string(&next_payload).map_err(|_| {
                            strategy_write_failure("failed to save strategy definition")
                        })?
                    ],
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
        let connection = self
            .connection
            .lock()
            .map_err(|_| strategy_write_failure("strategy definition fixture lock poisoned"))?;
        let transaction = connection
            .unchecked_transaction()
            .map_err(|_| strategy_write_failure("failed to delete strategy definition"))?;
        let row = transaction
            .query_row(
                "SELECT payload, linked_ids FROM strategy_definition_test_cutover WHERE id = ?1 AND deleted = 0",
                rusqlite::params![definition_id],
                |row| Ok((row.get::<_, String>(0)?, row.get::<_, String>(1)?)),
            )
            .optional()
            .map_err(|_| strategy_write_failure("failed to delete strategy definition"))?
            .ok_or_else(strategy_not_found)?;
        let linked_ids: Vec<String> = serde_json::from_str(&row.1)
            .map_err(|_| strategy_write_failure("failed to delete strategy definition"))?;
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
        let current = self
            .current(definition_id)
            .map_err(|_| strategy_write_failure("definition store unavailable"))?;
        if current.is_none() {
            return Err(strategy_not_found());
        }
        Ok(
            json!({"definitionId": definition_id, "latestVersion": current.expect("checked").0, "totalLinked": 0, "applied": [], "alreadyLatest": [], "skippedBusy": []}),
        )
    }

    fn mutate_instantiate(
        &self,
        input: &StrategyDefinitionWriteInput,
    ) -> Result<Value, StrategyDefinitionWritePortError> {
        let definition_id = input
            .definition_id
            .as_deref()
            .ok_or_else(|| strategy_write_failure("invalid definition id"))?;
        let current = self
            .current(definition_id)
            .map_err(|_| strategy_write_failure("definition store unavailable"))?;
        let Some((version, payload, _)) = current else {
            return Err(strategy_not_found());
        };
        if let Some(message) = input.binding_error.as_deref() {
            return Err(StrategyDefinitionWritePortError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: message.to_owned(),
            });
        }
        let instance_id = format!(
            "instance-test-{}",
            self.next_id
                .fetch_add(1, std::sync::atomic::Ordering::Relaxed)
        );
        Ok(
            json!({"id": instance_id, "definitionId": definition_id, "definitionVersion": version, "definition": payload, "binding": input.binding.clone().unwrap_or_else(|| json!({})), "status": "STOPPED"}),
        )
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
