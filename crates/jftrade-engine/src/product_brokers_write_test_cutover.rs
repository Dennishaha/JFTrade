// Durable broker mutation test-cutover adapter.
//
// This code is included only by Rust tests. Its SQLite schema is isolated
// from the Go broker/order stores and it never connects to a broker/OpenD,
// submits an order, or emits a production side effect.

use rusqlite::OptionalExtension;
use serde_json::{Value, json};

use super::{
    BrokersWriteContext, BrokersWriteInput, BrokersWriteOperation, BrokersWritePort,
    BrokersWritePortError,
};

pub struct BrokersWriteSqliteTestCutoverPort {
    path: std::path::PathBuf,
    connection: std::sync::Mutex<rusqlite::Connection>,
    reject_next_event: std::sync::Mutex<bool>,
}

impl std::fmt::Debug for BrokersWriteSqliteTestCutoverPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("BrokersWriteSqliteTestCutoverPort")
            .field("path", &self.path)
            .finish()
    }
}

#[allow(dead_code)]
impl BrokersWriteSqliteTestCutoverPort {
    pub fn open(path: impl AsRef<std::path::Path>) -> Result<Self, String> {
        let path = path.as_ref().to_owned();
        let connection = rusqlite::Connection::open(&path).map_err(|error| error.to_string())?;
        connection
            .execute_batch(
                "PRAGMA foreign_keys = ON;
                 CREATE TABLE IF NOT EXISTS brokers_test_ids (
                    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
                    next_value INTEGER NOT NULL
                 );
                 INSERT OR IGNORE INTO brokers_test_ids (singleton, next_value)
                    VALUES (1, 1);
                 CREATE TABLE IF NOT EXISTS brokers_test_orders (
                    order_id INTEGER PRIMARY KEY,
                    broker_id TEXT NOT NULL,
                    account_id TEXT NOT NULL,
                    trading_environment TEXT NOT NULL,
                    market TEXT NOT NULL,
                    status TEXT NOT NULL,
                    payload TEXT NOT NULL
                 );
                 CREATE TABLE IF NOT EXISTS brokers_test_sessions (
                    broker_id TEXT PRIMARY KEY,
                    unlocked INTEGER NOT NULL,
                    payload TEXT NOT NULL
                 );
                 CREATE TABLE IF NOT EXISTS brokers_test_events (
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
            reject_next_event: std::sync::Mutex::new(false),
        })
    }

    pub fn order_count(&self) -> Result<u64, String> {
        let connection = self.lock()?;
        let count = connection
            .query_row("SELECT COUNT(*) FROM brokers_test_orders", [], |row| {
                row.get::<_, i64>(0)
            })
            .map_err(|error| error.to_string())?;
        u64::try_from(count).map_err(|_| "negative broker order count".to_owned())
    }

    pub fn order_status(&self, order_id: i64) -> Result<Option<String>, String> {
        let connection = self.lock()?;
        connection
            .query_row(
                "SELECT status FROM brokers_test_orders WHERE order_id = ?1",
                rusqlite::params![order_id],
                |row| row.get(0),
            )
            .optional()
            .map_err(|error| error.to_string())
    }

    pub fn event_count(&self, operation: &str) -> Result<u64, String> {
        let connection = self.lock()?;
        let count = connection
            .query_row(
                "SELECT COUNT(*) FROM brokers_test_events WHERE operation = ?1",
                rusqlite::params![operation],
                |row| row.get::<_, i64>(0),
            )
            .map_err(|error| error.to_string())?;
        u64::try_from(count).map_err(|_| "negative broker event count".to_owned())
    }

    pub fn session_unlocked(&self, broker_id: &str) -> Result<Option<bool>, String> {
        let connection = self.lock()?;
        connection
            .query_row(
                "SELECT unlocked FROM brokers_test_sessions WHERE broker_id = ?1",
                rusqlite::params![broker_id],
                |row| row.get::<_, i64>(0),
            )
            .optional()
            .map(|value| value.map(|unlocked| unlocked != 0))
            .map_err(|error| error.to_string())
    }

    pub fn reject_next_event(&self) -> Result<(), String> {
        let mut reject = self
            .reject_next_event
            .lock()
            .map_err(|_| "broker fixture rejection lock poisoned".to_owned())?;
        *reject = true;
        Ok(())
    }

    fn lock(&self) -> Result<std::sync::MutexGuard<'_, rusqlite::Connection>, String> {
        self.connection
            .lock()
            .map_err(|_| "broker fixture connection lock poisoned".to_owned())
    }

    fn take_event_rejection(&self) -> Result<bool, BrokersWritePortError> {
        let mut reject = self
            .reject_next_event
            .lock()
            .map_err(|_| failed("broker fixture rejection lock poisoned"))?;
        let value = *reject;
        *reject = false;
        Ok(value)
    }

    fn mutate_transaction(
        &self,
        input: &BrokersWriteInput,
    ) -> Result<Value, BrokersWritePortError> {
        reject_non_normal_context(input.context)?;
        let reject_event = self.take_event_rejection()?;
        let connection = self
            .connection
            .lock()
            .map_err(|_| failed("broker fixture connection lock poisoned"))?;
        let transaction = connection
            .unchecked_transaction()
            .map_err(|_| failed("broker mutation transaction failed"))?;
        let result = match input.operation {
            BrokersWriteOperation::PlaceOrder => place(&transaction, input, reject_event),
            BrokersWriteOperation::CancelOrders => cancel(&transaction, input, reject_event),
            BrokersWriteOperation::Unlock => unlock(&transaction, input, reject_event),
        }?;
        transaction
            .commit()
            .map_err(|_| failed("broker mutation commit failed"))?;
        Ok(result)
    }
}

impl BrokersWritePort for BrokersWriteSqliteTestCutoverPort {
    fn mutate(&self, input: &BrokersWriteInput) -> Result<Value, BrokersWritePortError> {
        self.mutate_transaction(input)
    }
}

fn place(
    transaction: &rusqlite::Transaction<'_>,
    input: &BrokersWriteInput,
    reject_event: bool,
) -> Result<Value, BrokersWritePortError> {
    let order_id = next_order_id(transaction)?;
    let payload = serde_json::to_string(&input.payload)
        .map_err(|_| failed("broker place payload encode failed"))?;
    transaction
        .execute(
            "INSERT INTO brokers_test_orders (
                order_id, broker_id, account_id, trading_environment, market, status, payload
             ) VALUES (?1, ?2, ?3, ?4, ?5, 'submitted', ?6)",
            rusqlite::params![
                order_id,
                input.query.broker_id,
                input.query.account_id,
                input.query.trading_environment,
                input.query.market,
                payload,
            ],
        )
        .map_err(|_| failed("broker order write failed"))?;
    insert_event(
        transaction,
        "place-order",
        &order_id.to_string(),
        &input.payload,
        reject_event,
    )?;
    Ok(json!({
        "accepted": true,
        "operation": "place-order",
        "orderId": order_id,
        "status": "submitted",
        "brokerId": input.query.broker_id,
    }))
}

fn cancel(
    transaction: &rusqlite::Transaction<'_>,
    input: &BrokersWriteInput,
    reject_event: bool,
) -> Result<Value, BrokersWritePortError> {
    let orders = input
        .payload
        .get("orders")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();
    let mut cancelled = 0;
    for item in orders {
        let Some(order_id) = item.get("orderId").and_then(Value::as_i64) else {
            continue;
        };
        let changed = transaction
            .execute(
                "UPDATE brokers_test_orders
                 SET status = 'cancelled'
                 WHERE order_id = ?1 AND status = 'submitted'",
                rusqlite::params![order_id],
            )
            .map_err(|_| failed("broker order cancel failed"))?;
        if changed == 1 {
            cancelled += 1;
            insert_event(
                transaction,
                "cancel-orders",
                &order_id.to_string(),
                &item,
                reject_event,
            )?;
        }
    }
    Ok(json!({"accepted": true, "operation": "cancel-orders", "cancelled": cancelled}))
}

fn unlock(
    transaction: &rusqlite::Transaction<'_>,
    input: &BrokersWriteInput,
    reject_event: bool,
) -> Result<Value, BrokersWritePortError> {
    let payload = serde_json::to_string(&input.payload)
        .map_err(|_| failed("broker unlock payload encode failed"))?;
    transaction
        .execute(
            "INSERT INTO brokers_test_sessions (broker_id, unlocked, payload)
             VALUES (?1, 1, ?2)
             ON CONFLICT(broker_id) DO UPDATE SET unlocked = 1, payload = excluded.payload",
            rusqlite::params![input.query.broker_id, payload],
        )
        .map_err(|_| failed("broker unlock write failed"))?;
    insert_event(
        transaction,
        "unlock",
        &input.query.broker_id,
        &input.payload,
        reject_event,
    )?;
    Ok(json!({
        "accepted": true,
        "operation": "unlock",
        "unlocked": true,
        "brokerId": input.query.broker_id,
    }))
}

fn next_order_id(transaction: &rusqlite::Transaction<'_>) -> Result<i64, BrokersWritePortError> {
    let value = transaction
        .query_row(
            "SELECT next_value FROM brokers_test_ids WHERE singleton = 1",
            [],
            |row| row.get::<_, i64>(0),
        )
        .map_err(|_| failed("broker order id allocation failed"))?;
    transaction
        .execute(
            "UPDATE brokers_test_ids SET next_value = next_value + 1 WHERE singleton = 1",
            [],
        )
        .map_err(|_| failed("broker order id allocation failed"))?;
    Ok(value)
}

fn insert_event(
    transaction: &rusqlite::Transaction<'_>,
    operation: &str,
    resource_id: &str,
    payload: &Value,
    reject_event: bool,
) -> Result<(), BrokersWritePortError> {
    if reject_event {
        return Err(failed("broker event write rejected"));
    }
    let payload = serde_json::to_string(payload)
        .map_err(|_| failed("broker event payload encode failed"))?;
    transaction
        .execute(
            "INSERT INTO brokers_test_events (operation, resource_id, payload)
             VALUES (?1, ?2, ?3)",
            rusqlite::params![operation, resource_id, payload],
        )
        .map_err(|_| failed("broker event write failed"))?;
    Ok(())
}

fn reject_non_normal_context(context: BrokersWriteContext) -> Result<(), BrokersWritePortError> {
    match context {
        BrokersWriteContext::Normal => Ok(()),
        BrokersWriteContext::Canceled => Err(BrokersWritePortError::Failed {
            status: 499,
            code: "REQUEST_CANCELLED".to_owned(),
            message: "broker request cancelled".to_owned(),
        }),
        BrokersWriteContext::Deadline => Err(BrokersWritePortError::Failed {
            status: 504,
            code: "BROKER_TIMEOUT".to_owned(),
            message: "broker request deadline exceeded".to_owned(),
        }),
    }
}

fn failed(message: &str) -> BrokersWritePortError {
    BrokersWritePortError::Failed {
        status: 500,
        code: "BROKERS_TEST_FAILED".to_owned(),
        message: message.to_owned(),
    }
}
