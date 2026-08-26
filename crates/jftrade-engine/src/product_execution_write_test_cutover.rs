// Durable execution test-cutover adapter.
//
// This code is included only by Rust tests. Its SQLite schema is isolated
// from the Go execution ledger and it never connects to a broker/OpenD,
// starts an order-update worker, or emits a production side effect.

use rusqlite::OptionalExtension;
use serde_json::{Value, json};

use super::{
    ExecutionWriteContext, ExecutionWriteInput, ExecutionWriteOperation, ExecutionWritePort,
    ExecutionWritePortError,
};

pub struct ExecutionSqliteTestCutoverPort {
    path: std::path::PathBuf,
    connection: std::sync::Mutex<rusqlite::Connection>,
}

impl std::fmt::Debug for ExecutionSqliteTestCutoverPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ExecutionSqliteTestCutoverPort")
            .field("path", &self.path)
            .finish()
    }
}

#[allow(dead_code)]
impl ExecutionSqliteTestCutoverPort {
    pub fn open(path: impl AsRef<std::path::Path>) -> Result<Self, String> {
        let path = path.as_ref().to_owned();
        let connection = rusqlite::Connection::open(&path).map_err(|error| error.to_string())?;
        connection
            .execute_batch(
                "PRAGMA foreign_keys = ON;
                 CREATE TABLE IF NOT EXISTS execution_test_ids (
                    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
                    next_value INTEGER NOT NULL
                 );
                 INSERT OR IGNORE INTO execution_test_ids (singleton, next_value) VALUES (1, 1);
                 CREATE TABLE IF NOT EXISTS execution_test_orders (
                    id TEXT PRIMARY KEY,
                    kind TEXT NOT NULL,
                    status TEXT NOT NULL,
                    payload TEXT NOT NULL
                 );
                 CREATE TABLE IF NOT EXISTS execution_test_events (
                    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
                    operation TEXT NOT NULL,
                    resource_id TEXT NOT NULL,
                    payload TEXT NOT NULL
                 );",
            )
            .map_err(|error| error.to_string())?;
        Ok(Self {
            path,
            connection: std::sync::Mutex::new(connection),
        })
    }

    pub fn order_count(&self) -> Result<u64, String> {
        let connection = self.lock()?;
        let count = connection
            .query_row("SELECT COUNT(*) FROM execution_test_orders", [], |row| {
                row.get::<_, i64>(0)
            })
            .map_err(|error| error.to_string())?;
        u64::try_from(count).map_err(|_| "negative execution order count".to_owned())
    }

    pub fn order_status(&self, id: &str) -> Result<Option<String>, String> {
        let connection = self.lock()?;
        connection
            .query_row(
                "SELECT status FROM execution_test_orders WHERE id = ?1",
                rusqlite::params![id],
                |row| row.get(0),
            )
            .optional()
            .map_err(|error| error.to_string())
    }

    pub fn event_count(&self, operation: &str) -> Result<u64, String> {
        let connection = self.lock()?;
        let count = connection
            .query_row(
                "SELECT COUNT(*) FROM execution_test_events WHERE operation = ?1",
                rusqlite::params![operation],
                |row| row.get::<_, i64>(0),
            )
            .map_err(|error| error.to_string())?;
        u64::try_from(count).map_err(|_| "negative execution event count".to_owned())
    }

    pub fn reject_order_place_event(&self) -> Result<(), String> {
        let connection = self.lock()?;
        connection
            .execute_batch(
                "DROP TRIGGER IF EXISTS execution_test_reject_order_place;
                 CREATE TRIGGER execution_test_reject_order_place
                 BEFORE INSERT ON execution_test_events
                 WHEN NEW.operation = 'order-place' BEGIN
                    SELECT RAISE(ABORT, 'test-cutover order-place rejection');
                 END;",
            )
            .map_err(|error| error.to_string())?;
        Ok(())
    }

    pub fn clear_rejection(&self) -> Result<(), String> {
        let connection = self.lock()?;
        connection
            .execute_batch("DROP TRIGGER IF EXISTS execution_test_reject_order_place")
            .map_err(|error| error.to_string())?;
        Ok(())
    }

    fn lock(&self) -> Result<std::sync::MutexGuard<'_, rusqlite::Connection>, String> {
        self.connection
            .lock()
            .map_err(|_| "execution fixture lock poisoned".to_owned())
    }

    fn mutate_transaction(
        &self,
        input: &ExecutionWriteInput,
    ) -> Result<Value, ExecutionWritePortError> {
        reject_non_normal_context(input.context)?;
        let connection = self
            .connection
            .lock()
            .map_err(|_| failed("execution fixture lock poisoned"))?;
        let transaction = connection
            .unchecked_transaction()
            .map_err(|_| failed("execution transaction failed"))?;
        let result = match input.operation {
            ExecutionWriteOperation::OrderPlace => place(&transaction, "order", input)?,
            ExecutionWriteOperation::ComboPlace => place(&transaction, "combo", input)?,
            ExecutionWriteOperation::OrderCancel | ExecutionWriteOperation::ComboCancel => {
                cancel(&transaction, input)?
            }
            ExecutionWriteOperation::BuyingPower
            | ExecutionWriteOperation::ComboPreview
            | ExecutionWriteOperation::OrderPreview => json!({
                "accepted": true,
                "operation": input.operation.name(),
                "durableMutation": false,
            }),
        };
        transaction
            .commit()
            .map_err(|_| failed("execution commit failed"))?;
        Ok(result)
    }
}

impl ExecutionWritePort for ExecutionSqliteTestCutoverPort {
    fn mutate(&self, input: &ExecutionWriteInput) -> Result<Value, ExecutionWritePortError> {
        self.mutate_transaction(input)
    }
}

fn place(
    transaction: &rusqlite::Transaction<'_>,
    kind: &str,
    input: &ExecutionWriteInput,
) -> Result<Value, ExecutionWritePortError> {
    let id = next_id(transaction, kind)?;
    let payload = serde_json::to_string(&input.payload)
        .map_err(|_| failed("execution payload encode failed"))?;
    transaction
        .execute(
            "INSERT INTO execution_test_orders (id, kind, status, payload)
             VALUES (?1, ?2, 'submitted', ?3)",
            rusqlite::params![id, kind, payload],
        )
        .map_err(|_| failed("execution order write failed"))?;
    insert_event(transaction, input.operation.name(), &id, &input.payload)?;
    Ok(json!({
        "accepted": true,
        "operation": input.operation.name(),
        "internalOrderId": id,
        "status": "submitted",
    }))
}

fn cancel(
    transaction: &rusqlite::Transaction<'_>,
    input: &ExecutionWriteInput,
) -> Result<Value, ExecutionWritePortError> {
    let id = input.internal_order_id.as_deref().unwrap_or_default();
    let status = transaction
        .query_row(
            "SELECT status FROM execution_test_orders WHERE id = ?1",
            rusqlite::params![id],
            |row| row.get::<_, String>(0),
        )
        .optional()
        .map_err(|_| failed("execution order load failed"))?
        .ok_or_else(|| ExecutionWritePortError::Failed {
            status: 404,
            code: "EXECUTION_ORDER_NOT_FOUND".to_owned(),
            message: "execution order not found".to_owned(),
        })?;
    let transitioned = status != "cancelled";
    if transitioned {
        transaction
            .execute(
                "UPDATE execution_test_orders SET status = 'cancelled' WHERE id = ?1",
                rusqlite::params![id],
            )
            .map_err(|_| failed("execution order cancel failed"))?;
        insert_event(transaction, input.operation.name(), id, &Value::Null)?;
    }
    Ok(json!({
        "accepted": true,
        "operation": input.operation.name(),
        "internalOrderId": id,
        "status": "cancelled",
        "transitioned": transitioned,
    }))
}

fn next_id(
    transaction: &rusqlite::Transaction<'_>,
    prefix: &str,
) -> Result<String, ExecutionWritePortError> {
    let value = transaction
        .query_row(
            "SELECT next_value FROM execution_test_ids WHERE singleton = 1",
            [],
            |row| row.get::<_, i64>(0),
        )
        .map_err(|_| failed("execution id allocation failed"))?;
    transaction
        .execute(
            "UPDATE execution_test_ids SET next_value = next_value + 1 WHERE singleton = 1",
            [],
        )
        .map_err(|_| failed("execution id allocation failed"))?;
    Ok(format!("{prefix}-test-{value}"))
}

fn insert_event(
    transaction: &rusqlite::Transaction<'_>,
    operation: &str,
    resource_id: &str,
    payload: &Value,
) -> Result<(), ExecutionWritePortError> {
    let payload = serde_json::to_string(payload)
        .map_err(|_| failed("execution payload encode failed"))?;
    transaction
        .execute(
            "INSERT INTO execution_test_events (operation, resource_id, payload)
             VALUES (?1, ?2, ?3)",
            rusqlite::params![operation, resource_id, payload],
        )
        .map_err(|_| failed("execution event write failed"))?;
    Ok(())
}

fn reject_non_normal_context(context: ExecutionWriteContext) -> Result<(), ExecutionWritePortError> {
    match context {
        ExecutionWriteContext::Normal => Ok(()),
        ExecutionWriteContext::Canceled => Err(ExecutionWritePortError::Failed {
            status: 499,
            code: "REQUEST_CANCELLED".to_owned(),
            message: "execution request cancelled".to_owned(),
        }),
        ExecutionWriteContext::Deadline => Err(ExecutionWritePortError::Failed {
            status: 504,
            code: "BROKER_TIMEOUT".to_owned(),
            message: "execution request deadline exceeded".to_owned(),
        }),
    }
}

fn failed(message: &str) -> ExecutionWritePortError {
    ExecutionWritePortError::Failed {
        status: 500,
        code: "EXECUTION_TEST_FAILED".to_owned(),
        message: message.to_owned(),
    }
}
