//! Durable strategy-definition test-cutover adapter backed by `jftrade-store-sqlite`.
//!
//! This module is only included by Rust test targets. It connects to the real
//! strategy SQLite schema with schema validation and single-writer lease,
//! and is never constructed by the default product profile.

use std::path::{Path, PathBuf};
use std::sync::Arc;

use jftrade_store_sqlite::{
    STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE, StoredStrategyDefinition,
    StrategyDefinitionStoreError, StrategyDefinitionTestCutoverStore,
};
use rusqlite::params;
use serde_json::{Value, json};
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

use super::product_strategy_definition_write_port::{
    StrategyDefinitionWriteInput, StrategyDefinitionWriteOperation, StrategyDefinitionWritePort,
    StrategyDefinitionWritePortError,
};

pub struct StrategyDefinitionSqliteTestCutoverPort {
    path: PathBuf,
    store: Arc<StrategyDefinitionTestCutoverStore>,
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
    pub fn open(path: impl AsRef<Path>) -> Result<Self, String> {
        let path = path.as_ref().to_owned();
        let store = StrategyDefinitionTestCutoverStore::open_existing(
            &path,
            STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE,
        )
        .map_err(|err| err.to_string())?;
        Ok(Self {
            path,
            store: Arc::new(store),
        })
    }

    pub fn path(&self) -> &Path {
        &self.path
    }

    pub fn store(&self) -> &StrategyDefinitionTestCutoverStore {
        &self.store
    }

    pub fn seed_definition(
        &self,
        definition_id: &str,
        payload: Value,
        linked_ids: &[&str],
    ) -> Result<(), String> {
        let timestamp = now_rfc3339();
        let def = definition_from_value(definition_id, &payload);
        self.store
            .save_definition(def, &timestamp)
            .map_err(|e| e.to_string())?;
        self.set_linked_ids(definition_id, linked_ids)?;
        Ok(())
    }

    pub fn set_linked_ids(&self, definition_id: &str, linked_ids: &[&str]) -> Result<(), String> {
        let connection = rusqlite::Connection::open(&self.path).map_err(|e| e.to_string())?;
        connection
            .execute(
                "DELETE FROM strategy_catalog_operations WHERE plugin_id = ?1",
                params![definition_id],
            )
            .map_err(|e| e.to_string())?;
        for id in linked_ids {
            connection
                .execute(
                    "INSERT INTO strategy_catalog_operations (operation_id, plugin_id, status, updated_at, payload_json)
                     VALUES (?1, ?2, 'STOPPED', ?3, '{}')",
                    params![id, definition_id, now_rfc3339()],
                )
                .map_err(|e| e.to_string())?;
        }
        Ok(())
    }

    pub fn current(&self, definition_id: &str) -> Result<Option<(String, Value, bool)>, String> {
        let def = self
            .store
            .get_definition(definition_id, true)
            .map_err(|e| e.to_string())?;
        Ok(def.map(|d| {
            let is_deleted = d.deleted_at.is_some();
            let val = value_from_definition(&d);
            (d.version, val, is_deleted)
        }))
    }

    pub fn version_count(&self, definition_id: &str) -> Result<u64, String> {
        let versions = self
            .store
            .list_versions(definition_id)
            .map_err(|e| e.to_string())?;
        Ok(versions.len() as u64)
    }

    pub fn instance_count(&self, definition_id: &str) -> Result<u64, String> {
        let connection = rusqlite::Connection::open(&self.path).map_err(|e| e.to_string())?;
        let count: i64 = connection
            .query_row(
                "SELECT COUNT(*) FROM strategy_catalog_operations WHERE plugin_id = ?1",
                params![definition_id],
                |row| row.get(0),
            )
            .map_err(|e| e.to_string())?;
        Ok(count as u64)
    }

    pub fn instance_ids(&self, definition_id: &str) -> Result<Vec<String>, String> {
        let connection = rusqlite::Connection::open(&self.path).map_err(|e| e.to_string())?;
        let mut stmt = connection
            .prepare("SELECT operation_id FROM strategy_catalog_operations WHERE plugin_id = ?1 ORDER BY rowid")
            .map_err(|e| e.to_string())?;
        let rows = stmt
            .query_map(params![definition_id], |row| row.get(0))
            .map_err(|e| e.to_string())?;
        let mut ids = Vec::new();
        for row in rows {
            ids.push(row.map_err(|e| e.to_string())?);
        }
        Ok(ids)
    }

    pub fn linked_ids(&self, definition_id: &str) -> Result<Vec<String>, String> {
        self.instance_ids(definition_id)
    }

    pub fn reject_instance_create(&self) -> Result<(), String> {
        let connection = rusqlite::Connection::open(&self.path).map_err(|e| e.to_string())?;
        connection
            .execute_batch(
                "DROP TRIGGER IF EXISTS strategy_definition_test_reject_instance;
                 CREATE TRIGGER strategy_definition_test_reject_instance
                 BEFORE INSERT ON strategy_catalog_operations
                 BEGIN
                     SELECT RAISE(ABORT, 'test-cutover instance rejection');
                 END;",
            )
            .map_err(|e| e.to_string())
    }

    pub fn clear_instance_rejection(&self) -> Result<(), String> {
        let connection = rusqlite::Connection::open(&self.path).map_err(|e| e.to_string())?;
        connection
            .execute_batch("DROP TRIGGER IF EXISTS strategy_definition_test_reject_instance")
            .map_err(|e| e.to_string())
    }

    pub fn reject_version(&self, version: &str) -> Result<(), String> {
        let connection = rusqlite::Connection::open(&self.path).map_err(|e| e.to_string())?;
        connection
            .execute_batch("DROP TRIGGER IF EXISTS strategy_definition_test_reject_version")
            .map_err(|e| e.to_string())?;
        let stmt = format!(
            "CREATE TRIGGER strategy_definition_test_reject_version
             BEFORE INSERT ON strategy_definition_versions
             WHEN NEW.version = '{}' BEGIN
                 SELECT RAISE(ABORT, 'test-cutover version rejection');
             END;",
            version.replace('\'', "''")
        );
        connection.execute_batch(&stmt).map_err(|e| e.to_string())?;
        Ok(())
    }

    pub fn clear_version_rejection(&self) -> Result<(), String> {
        let connection = rusqlite::Connection::open(&self.path).map_err(|e| e.to_string())?;
        connection
            .execute_batch("DROP TRIGGER IF EXISTS strategy_definition_test_reject_version")
            .map_err(|e| e.to_string())?;
        Ok(())
    }

    fn mutate_create(
        &self,
        input: &StrategyDefinitionWriteInput,
    ) -> Result<Value, StrategyDefinitionWritePortError> {
        let payload = input
            .definition
            .clone()
            .unwrap_or_else(|| json!({"name": "New Strategy"}));
        let id = payload["id"]
            .as_str()
            .map(ToOwned::to_owned)
            .unwrap_or_else(|| format!("strat_{}", generate_id()));
        let def = definition_from_value(&id, &payload);
        let saved = self
            .store
            .save_definition(def, &now_rfc3339())
            .map_err(map_store_error)?;
        Ok(value_from_definition(&saved))
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
            .clone()
            .unwrap_or_else(|| json!({"name": "Updated Strategy"}));
        let existing = self
            .store
            .get_definition(definition_id, false)
            .map_err(map_store_error)?
            .ok_or_else(strategy_not_found)?;
        let mut def = definition_from_value(definition_id, &payload);
        def.created_at = existing.created_at;
        let saved = self
            .store
            .save_definition(def, &now_rfc3339())
            .map_err(map_store_error)?;
        Ok(value_from_definition(&saved))
    }

    fn mutate_delete(
        &self,
        input: &StrategyDefinitionWriteInput,
    ) -> Result<Value, StrategyDefinitionWritePortError> {
        let definition_id = input
            .definition_id
            .as_deref()
            .ok_or_else(|| strategy_write_failure("invalid definition id"))?;
        let deleted = self
            .store
            .delete_definition(definition_id, &now_rfc3339())
            .map_err(map_store_error)?;
        Ok(value_from_definition(&deleted))
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
            .store
            .get_definition(definition_id, false)
            .map_err(map_store_error)?
            .ok_or_else(strategy_not_found)?;
        let linked = self.instance_ids(definition_id).unwrap_or_default();
        Ok(json!({
            "definitionId": definition_id,
            "latestVersion": current.version,
            "totalLinked": linked.len(),
            "applied": linked,
            "alreadyLatest": [],
            "skippedBusy": [],
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
        let current = self
            .store
            .get_definition(definition_id, false)
            .map_err(map_store_error)?
            .ok_or_else(strategy_not_found)?;
        if let Some(message) = input.binding_error.as_deref() {
            return Err(StrategyDefinitionWritePortError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: message.to_owned(),
            });
        }
        let instance_id = format!("inst_{}", generate_id());
        let binding = input.binding.clone().unwrap_or_else(|| json!({}));
        let connection = rusqlite::Connection::open(&self.path)
            .map_err(|e| strategy_write_failure(&e.to_string()))?;
        connection
            .execute(
                "INSERT INTO strategy_catalog_operations (operation_id, plugin_id, status, updated_at, payload_json)
                 VALUES (?1, ?2, 'STOPPED', ?3, ?4)",
                params![instance_id, definition_id, now_rfc3339(), binding.to_string()],
            )
            .map_err(|e| strategy_write_failure(&e.to_string()))?;
        let def_val = value_from_definition(&current);
        Ok(json!({
            "id": instance_id,
            "definitionId": definition_id,
            "definitionVersion": current.version,
            "definition": def_val,
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

fn definition_from_value(id: &str, val: &Value) -> StoredStrategyDefinition {
    StoredStrategyDefinition {
        id: id.to_owned(),
        name: val["name"].as_str().unwrap_or("").to_owned(),
        version: val["version"].as_str().unwrap_or("0.1.0").to_owned(),
        description: val["description"].as_str().unwrap_or("").to_owned(),
        runtime: val["runtime"].as_str().unwrap_or("pine").to_owned(),
        source_format: val["sourceFormat"].as_str().unwrap_or("pine").to_owned(),
        symbol: val["symbol"].as_str().unwrap_or("US.AAPL").to_owned(),
        interval: val["interval"].as_str().unwrap_or("1m").to_owned(),
        script: val["script"].as_str().unwrap_or("").to_owned(),
        visual_model_json: val["visualModelJson"].as_str().unwrap_or("{}").to_owned(),
        created_at: val["createdAt"].as_str().unwrap_or("").to_owned(),
        updated_at: val["updatedAt"].as_str().unwrap_or("").to_owned(),
        deleted_at: None,
    }
}

fn value_from_definition(def: &StoredStrategyDefinition) -> Value {
    json!({
        "id": def.id,
        "name": def.name,
        "version": def.version,
        "description": def.description,
        "runtime": def.runtime,
        "sourceFormat": def.source_format,
        "symbol": def.symbol,
        "interval": def.interval,
        "script": def.script,
        "visualModelJson": def.visual_model_json,
        "createdAt": def.created_at,
        "updatedAt": def.updated_at,
        "deletedAt": def.deleted_at,
    })
}

fn map_store_error(error: StrategyDefinitionStoreError) -> StrategyDefinitionWritePortError {
    match error {
        StrategyDefinitionStoreError::NotFound => StrategyDefinitionWritePortError::Failed {
            status: 404,
            code: "NOT_FOUND".to_owned(),
            message: "strategy resource not found".to_owned(),
        },
        StrategyDefinitionStoreError::DeleteGuard(message) => {
            StrategyDefinitionWritePortError::Failed {
                status: 400,
                code: "STRATEGY_INVALID".to_owned(),
                message,
            }
        }
        StrategyDefinitionStoreError::Validation(message) => {
            StrategyDefinitionWritePortError::Failed {
                status: 400,
                code: "STRATEGY_INVALID".to_owned(),
                message,
            }
        }
        _ => StrategyDefinitionWritePortError::Failed {
            status: 500,
            code: "STRATEGY_FAILED".to_owned(),
            message: error.to_string(),
        },
    }
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
