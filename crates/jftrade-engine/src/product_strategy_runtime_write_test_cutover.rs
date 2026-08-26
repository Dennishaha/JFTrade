//! Durable strategy-runtime test-cutover adapter backed by `jftrade-store-sqlite`.
//!
//! This module is compiled only for Rust tests. It connects to the real
//! strategy SQLite schema with schema validation and single-writer lease,
//! and is never constructed by the default product profile.

use std::path::{Path, PathBuf};
use std::sync::Arc;

use jftrade_store_sqlite::{
    STRATEGY_RUNTIME_TEST_CUTOVER_PROFILE, StoredRuntimeInstance, StrategyRuntimeStoreError,
    StrategyRuntimeTestCutoverStore,
};
use rusqlite::params;
use serde_json::{Value, json};
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

use super::product_strategy_runtime_write_port::{
    StrategyRuntimeWriteInput, StrategyRuntimeWriteOperation, StrategyRuntimeWritePort,
    StrategyRuntimeWritePortError,
};

pub struct StrategyRuntimeSqliteTestCutoverPort {
    path: PathBuf,
    store: Arc<StrategyRuntimeTestCutoverStore>,
}

impl std::fmt::Debug for StrategyRuntimeSqliteTestCutoverPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("StrategyRuntimeSqliteTestCutoverPort")
            .field("path", &self.path)
            .finish()
    }
}

#[allow(dead_code)]
impl StrategyRuntimeSqliteTestCutoverPort {
    pub fn open(path: impl AsRef<Path>) -> Result<Self, String> {
        let path = path.as_ref().to_owned();
        let store = StrategyRuntimeTestCutoverStore::open_existing(
            &path,
            STRATEGY_RUNTIME_TEST_CUTOVER_PROFILE,
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

    pub fn store(&self) -> &StrategyRuntimeTestCutoverStore {
        &self.store
    }

    pub fn seed_instance(&self, instance_id: &str, status: &str) -> Result<(), String> {
        self.store
            .seed_instance(instance_id, status, &now_rfc3339())
            .map_err(|e| e.to_string())
    }

    pub fn snapshot(&self, instance_id: &str) -> Result<Option<Value>, String> {
        let instance = self
            .store
            .get_instance(instance_id)
            .map_err(|e| e.to_string())?;
        Ok(instance.map(|inst| projection_from_instance(&inst)))
    }

    pub fn event_count(&self, instance_id: &str, operation: &str) -> Result<u64, String> {
        let connection = rusqlite::Connection::open(&self.path).map_err(|e| e.to_string())?;
        let kind = match operation {
            "start" => "STARTED",
            "stop" => "STOPPED",
            "pause" => "PAUSED",
            _ => operation,
        };
        let count: i64 = connection
            .query_row(
                "SELECT COUNT(*) FROM strategy_audit_events WHERE instance_id = ?1 AND kind = ?2",
                params![instance_id, kind],
                |row| row.get(0),
            )
            .map_err(|e| e.to_string())?;
        Ok(count as u64)
    }

    pub fn reject_status(&self, status: &str) -> Result<(), String> {
        let connection = rusqlite::Connection::open(&self.path).map_err(|e| e.to_string())?;
        connection
            .execute_batch("DROP TRIGGER IF EXISTS strategy_runtime_test_reject_status")
            .map_err(|e| e.to_string())?;
        let stmt = format!(
            "CREATE TRIGGER strategy_runtime_test_reject_status
             BEFORE UPDATE OF status ON strategy_catalog_operations
             WHEN NEW.status = '{}' BEGIN
                 SELECT RAISE(ABORT, 'test-cutover status rejection');
             END;",
            status.replace('\'', "''")
        );
        connection.execute_batch(&stmt).map_err(|e| e.to_string())?;
        Ok(())
    }

    pub fn clear_rejection(&self) -> Result<(), String> {
        let connection = rusqlite::Connection::open(&self.path).map_err(|e| e.to_string())?;
        connection
            .execute_batch("DROP TRIGGER IF EXISTS strategy_runtime_test_reject_status")
            .map_err(|e| e.to_string())?;
        Ok(())
    }
}

impl StrategyRuntimeWritePort for StrategyRuntimeSqliteTestCutoverPort {
    fn mutate(
        &self,
        input: &StrategyRuntimeWriteInput,
    ) -> Result<Value, StrategyRuntimeWritePortError> {
        let timestamp = now_rfc3339();
        let result = match input.operation {
            StrategyRuntimeWriteOperation::Start => {
                self.store
                    .update_status(&input.instance_id, "RUNNING", &timestamp)
            }
            StrategyRuntimeWriteOperation::Stop => {
                self.store
                    .update_status(&input.instance_id, "STOPPED", &timestamp)
            }
            StrategyRuntimeWriteOperation::Pause => {
                self.store
                    .update_status(&input.instance_id, "PAUSED", &timestamp)
            }
            StrategyRuntimeWriteOperation::Delete => {
                let current = self
                    .store
                    .get_instance(&input.instance_id)
                    .map_err(|err| map_store_error(input.operation, err))?
                    .ok_or_else(strategy_not_found)?;
                if current.runtime_active || current.status == "RUNNING" {
                    return Err(StrategyRuntimeWritePortError::Failed {
                        status: 400,
                        code: "BAD_REQUEST".to_owned(),
                        message: "strategy instance is busy".to_owned(),
                    });
                }
                self.store.delete_instance(&input.instance_id, &timestamp)
            }
            StrategyRuntimeWriteOperation::Update => {
                let binding = input.binding.clone().unwrap_or(Value::Null);
                self.store
                    .update_binding(&input.instance_id, binding, &timestamp)
            }
            StrategyRuntimeWriteOperation::UpdateRuntimeRisk => {
                let risk = input.runtime_risk.clone().unwrap_or(Value::Null);
                self.store.update_risk(&input.instance_id, risk, &timestamp)
            }
            StrategyRuntimeWriteOperation::RefreshDefinition => self
                .store
                .refresh_definition(&input.instance_id, &timestamp),
        };

        match result {
            Ok(inst) => Ok(projection_from_instance(&inst)),
            Err(err) => Err(map_store_error(input.operation, err)),
        }
    }
}

fn projection_from_instance(inst: &StoredRuntimeInstance) -> Value {
    json!({
        "id": inst.id,
        "status": inst.status,
        "binding": inst.binding,
        "runtimeRisk": inst.runtime_risk,
        "definitionRevision": inst.definition_revision,
        "runtimeActive": inst.runtime_active,
        "deleted": inst.deleted,
    })
}

fn map_store_error(
    operation: StrategyRuntimeWriteOperation,
    error: StrategyRuntimeStoreError,
) -> StrategyRuntimeWritePortError {
    match error {
        StrategyRuntimeStoreError::NotFound => strategy_not_found(),
        StrategyRuntimeStoreError::Validation(message) => StrategyRuntimeWritePortError::Failed {
            status: 400,
            code: "BAD_REQUEST".to_owned(),
            message,
        },
        _ => strategy_failure(operation, &error.to_string()),
    }
}

fn strategy_failure(
    operation: StrategyRuntimeWriteOperation,
    message: &str,
) -> StrategyRuntimeWritePortError {
    let (status, code) = match operation {
        StrategyRuntimeWriteOperation::Start => (502, "STRATEGY_RUNTIME_START_FAILED"),
        StrategyRuntimeWriteOperation::Stop => (502, "STRATEGY_RUNTIME_STOP_FAILED"),
        StrategyRuntimeWriteOperation::Pause => (502, "STRATEGY_RUNTIME_PAUSE_FAILED"),
        _ => (500, "STRATEGY_FAILED"),
    };
    StrategyRuntimeWritePortError::Failed {
        status,
        code: code.to_owned(),
        message: message.to_owned(),
    }
}

fn strategy_not_found() -> StrategyRuntimeWritePortError {
    StrategyRuntimeWritePortError::Failed {
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
