//! Durable strategy-runtime test-cutover adapter.
//!
//! This module is compiled only for Rust tests. It owns an isolated fixture
//! schema and is never constructed by the default product profile.

use rusqlite::OptionalExtension;
use serde_json::{Value, json};

use super::product_strategy_runtime_write_port::{
    StrategyRuntimeWriteInput, StrategyRuntimeWriteOperation, StrategyRuntimeWritePort,
    StrategyRuntimeWritePortError,
};

pub struct StrategyRuntimeSqliteTestCutoverPort {
    path: std::path::PathBuf,
    connection: std::sync::Mutex<rusqlite::Connection>,
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
    pub fn open(path: impl AsRef<std::path::Path>) -> Result<Self, String> {
        let path = path.as_ref().to_owned();
        let connection = rusqlite::Connection::open(&path).map_err(|error| error.to_string())?;
        connection
            .execute_batch(
                "PRAGMA foreign_keys = ON;
                 CREATE TABLE IF NOT EXISTS strategy_runtime_test_instances (
                    id TEXT PRIMARY KEY,
                    status TEXT NOT NULL,
                    binding TEXT NOT NULL,
                    runtime_risk TEXT NOT NULL,
                    definition_revision INTEGER NOT NULL DEFAULT 0,
                    runtime_active INTEGER NOT NULL DEFAULT 0,
                    deleted INTEGER NOT NULL DEFAULT 0
                 );
                 CREATE TABLE IF NOT EXISTS strategy_runtime_test_events (
                    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
                    instance_id TEXT NOT NULL,
                    operation TEXT NOT NULL,
                    FOREIGN KEY (instance_id)
                        REFERENCES strategy_runtime_test_instances(id)
                 );",
            )
            .map_err(|error| error.to_string())?;
        Ok(Self {
            path,
            connection: std::sync::Mutex::new(connection),
        })
    }

    pub fn seed_instance(&self, instance_id: &str, status: &str) -> Result<(), String> {
        let connection = self
            .connection
            .lock()
            .map_err(|_| "strategy runtime fixture lock poisoned".to_owned())?;
        connection
            .execute(
                "INSERT OR REPLACE INTO strategy_runtime_test_instances
                    (id, status, binding, runtime_risk, definition_revision,
                     runtime_active, deleted)
                 VALUES (?1, ?2, '{}', '{}', 0, ?3, 0)",
                rusqlite::params![instance_id, status, i64::from(status == "RUNNING")],
            )
            .map_err(|error| error.to_string())?;
        Ok(())
    }

    pub fn snapshot(&self, instance_id: &str) -> Result<Option<Value>, String> {
        let connection = self
            .connection
            .lock()
            .map_err(|_| "strategy runtime fixture lock poisoned".to_owned())?;
        load_projection(&connection, instance_id).map_err(|error| error.to_string())
    }

    pub fn event_count(&self, instance_id: &str, operation: &str) -> Result<u64, String> {
        let connection = self
            .connection
            .lock()
            .map_err(|_| "strategy runtime fixture lock poisoned".to_owned())?;
        let count = connection
            .query_row(
                "SELECT COUNT(*) FROM strategy_runtime_test_events
                 WHERE instance_id = ?1 AND operation = ?2",
                rusqlite::params![instance_id, operation],
                |row| row.get::<_, i64>(0),
            )
            .map_err(|error| error.to_string())?;
        u64::try_from(count).map_err(|_| "negative strategy runtime event count".to_owned())
    }

    pub fn reject_status(&self, status: &str) -> Result<(), String> {
        let connection = self
            .connection
            .lock()
            .map_err(|_| "strategy runtime fixture lock poisoned".to_owned())?;
        connection
            .execute_batch("DROP TRIGGER IF EXISTS strategy_runtime_test_reject_status")
            .map_err(|error| error.to_string())?;
        let statement = format!(
            "CREATE TRIGGER strategy_runtime_test_reject_status
             BEFORE UPDATE OF status ON strategy_runtime_test_instances
             WHEN NEW.status = '{}' BEGIN
                 SELECT RAISE(ABORT, 'test-cutover status rejection');
             END",
            status.replace('\'', "''")
        );
        connection
            .execute_batch(&statement)
            .map_err(|error| error.to_string())?;
        Ok(())
    }

    pub fn clear_rejection(&self) -> Result<(), String> {
        let connection = self
            .connection
            .lock()
            .map_err(|_| "strategy runtime fixture lock poisoned".to_owned())?;
        connection
            .execute_batch("DROP TRIGGER IF EXISTS strategy_runtime_test_reject_status")
            .map_err(|error| error.to_string())?;
        Ok(())
    }

    fn mutate_transaction(
        &self,
        input: &StrategyRuntimeWriteInput,
    ) -> Result<Value, StrategyRuntimeWritePortError> {
        let connection = self.connection.lock().map_err(|_| {
            strategy_failure(input.operation, "strategy runtime fixture lock poisoned")
        })?;
        let transaction = connection.unchecked_transaction().map_err(|_| {
            strategy_failure(input.operation, "strategy runtime transaction failed")
        })?;
        let current = load_projection(&transaction, &input.instance_id)
            .map_err(|_| strategy_failure(input.operation, "strategy instance load failed"))?
            .ok_or_else(strategy_not_found)?;
        if current["deleted"] == true {
            return Err(strategy_not_found());
        }

        apply_operation(&transaction, input, &current)?;
        transaction
            .execute(
                "INSERT INTO strategy_runtime_test_events (instance_id, operation)
                 VALUES (?1, ?2)",
                rusqlite::params![input.instance_id, input.operation.name()],
            )
            .map_err(|_| strategy_failure(input.operation, "strategy runtime event failed"))?;
        let projection = load_projection(&transaction, &input.instance_id)
            .map_err(|_| strategy_failure(input.operation, "strategy instance reload failed"))?
            .ok_or_else(strategy_not_found)?;
        transaction
            .commit()
            .map_err(|_| strategy_failure(input.operation, "strategy runtime commit failed"))?;
        Ok(projection)
    }
}

impl StrategyRuntimeWritePort for StrategyRuntimeSqliteTestCutoverPort {
    fn mutate(
        &self,
        input: &StrategyRuntimeWriteInput,
    ) -> Result<Value, StrategyRuntimeWritePortError> {
        self.mutate_transaction(input)
    }
}

fn apply_operation(
    transaction: &rusqlite::Transaction<'_>,
    input: &StrategyRuntimeWriteInput,
    current: &Value,
) -> Result<(), StrategyRuntimeWritePortError> {
    let statement = match input.operation {
        StrategyRuntimeWriteOperation::Update => {
            let payload = serde_json::to_string(input.binding.as_ref().unwrap_or(&Value::Null))
                .map_err(|_| strategy_failure(input.operation, "invalid strategy binding"))?;
            transaction
                .execute(
                    "UPDATE strategy_runtime_test_instances SET binding = ?2 WHERE id = ?1",
                    rusqlite::params![input.instance_id, payload],
                )
                .map(|_| ())
        }
        StrategyRuntimeWriteOperation::UpdateRuntimeRisk => {
            let payload =
                serde_json::to_string(input.runtime_risk.as_ref().unwrap_or(&Value::Null))
                    .map_err(|_| strategy_failure(input.operation, "invalid runtime risk"))?;
            transaction
                .execute(
                    "UPDATE strategy_runtime_test_instances SET runtime_risk = ?2 WHERE id = ?1",
                    rusqlite::params![input.instance_id, payload],
                )
                .map(|_| ())
        }
        StrategyRuntimeWriteOperation::Delete => {
            if current["runtimeActive"] == true || current["status"] == "RUNNING" {
                return Err(StrategyRuntimeWritePortError::Failed {
                    status: 400,
                    code: "BAD_REQUEST".to_owned(),
                    message: "strategy instance is busy".to_owned(),
                });
            }
            transaction
                .execute(
                    "UPDATE strategy_runtime_test_instances SET deleted = 1 WHERE id = ?1",
                    rusqlite::params![input.instance_id],
                )
                .map(|_| ())
        }
        StrategyRuntimeWriteOperation::Pause => set_status(transaction, input, "PAUSED", false),
        StrategyRuntimeWriteOperation::Stop => set_status(transaction, input, "STOPPED", false),
        StrategyRuntimeWriteOperation::Start => set_status(transaction, input, "RUNNING", true),
        StrategyRuntimeWriteOperation::RefreshDefinition => transaction
            .execute(
                "UPDATE strategy_runtime_test_instances
                 SET definition_revision = definition_revision + 1 WHERE id = ?1",
                rusqlite::params![input.instance_id],
            )
            .map(|_| ()),
    };
    statement.map_err(|_| strategy_failure(input.operation, "strategy runtime mutation failed"))
}

fn set_status(
    transaction: &rusqlite::Transaction<'_>,
    input: &StrategyRuntimeWriteInput,
    status: &str,
    active: bool,
) -> rusqlite::Result<()> {
    transaction
        .execute(
            "UPDATE strategy_runtime_test_instances
             SET status = ?2, runtime_active = ?3 WHERE id = ?1",
            rusqlite::params![input.instance_id, status, i64::from(active)],
        )
        .map(|_| ())
}

fn load_projection(
    connection: &rusqlite::Connection,
    instance_id: &str,
) -> rusqlite::Result<Option<Value>> {
    connection
        .query_row(
            "SELECT status, binding, runtime_risk, definition_revision,
                    runtime_active, deleted
             FROM strategy_runtime_test_instances WHERE id = ?1",
            rusqlite::params![instance_id],
            |row| {
                let status: String = row.get(0)?;
                let binding: String = row.get(1)?;
                let runtime_risk: String = row.get(2)?;
                let definition_revision: i64 = row.get(3)?;
                let runtime_active: i64 = row.get(4)?;
                let deleted: i64 = row.get(5)?;
                Ok((
                    status,
                    binding,
                    runtime_risk,
                    definition_revision,
                    runtime_active,
                    deleted,
                ))
            },
        )
        .optional()
        .map(|row| {
            row.map(
                |(status, binding, runtime_risk, revision, active, deleted)| {
                    json!({
                        "id": instance_id,
                        "status": status,
                        "binding": serde_json::from_str::<Value>(&binding).unwrap_or(Value::Null),
                        "runtimeRisk": serde_json::from_str::<Value>(&runtime_risk).unwrap_or(Value::Null),
                        "definitionVersion": format!("0.1.{revision}"),
                        "runtimeActive": active != 0,
                        "deleted": deleted != 0,
                    })
                },
            )
        })
}

fn strategy_failure(
    operation: StrategyRuntimeWriteOperation,
    message: &str,
) -> StrategyRuntimeWritePortError {
    let (status, code) = if operation == StrategyRuntimeWriteOperation::Start {
        (502, "STRATEGY_RUNTIME_START_FAILED")
    } else {
        (500, "STRATEGY_FAILED")
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
        message: "strategy instance not found".to_owned(),
    }
}
