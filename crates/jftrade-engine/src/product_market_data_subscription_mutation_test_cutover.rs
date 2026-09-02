// Durable market-data subscription test-cutover adapter.
//
// This code is included only by Rust tests. Its SQLite schema is isolated
// from Go's subscription registry and lease store. It never connects to
// Provider/OpenD, creates live demand, or publishes a user-visible update.

use rusqlite::OptionalExtension;
use serde_json::{Value, json};

use super::{
    MarketDataSubscriptionMutationPort, MarketDataSubscriptionMutationPortError,
    MarketDataSubscriptionMutationRequest,
};

pub struct MarketDataSubscriptionMutationSqliteTestCutoverPort {
    path: std::path::PathBuf,
    connection: std::sync::Mutex<rusqlite::Connection>,
    reject_next_event: std::sync::Mutex<bool>,
}

impl std::fmt::Debug for MarketDataSubscriptionMutationSqliteTestCutoverPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("MarketDataSubscriptionMutationSqliteTestCutoverPort")
            .field("path", &self.path)
            .finish()
    }
}

#[allow(dead_code)]
impl MarketDataSubscriptionMutationSqliteTestCutoverPort {
    pub fn open(path: impl AsRef<std::path::Path>) -> Result<Self, String> {
        let path = path.as_ref().to_owned();
        let connection = rusqlite::Connection::open(&path).map_err(|error| error.to_string())?;
        connection
            .execute_batch(
                "PRAGMA foreign_keys = ON;
                 CREATE TABLE IF NOT EXISTS market_data_test_ids (
                    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
                    next_value INTEGER NOT NULL
                 );
                 INSERT OR IGNORE INTO market_data_test_ids (singleton, next_value)
                    VALUES (1, 1);
                 CREATE TABLE IF NOT EXISTS market_data_test_subscriptions (
                    consumer_id TEXT NOT NULL,
                    market TEXT NOT NULL,
                    symbol TEXT NOT NULL,
                    active INTEGER NOT NULL,
                    heartbeats INTEGER NOT NULL,
                    PRIMARY KEY (consumer_id, market, symbol)
                 );
                 CREATE TABLE IF NOT EXISTS market_data_test_leases (
                    lease_id TEXT PRIMARY KEY,
                    contract_code TEXT NOT NULL,
                    data_types TEXT NOT NULL,
                    status TEXT NOT NULL
                 );
                 CREATE TABLE IF NOT EXISTS market_data_test_events (
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

    pub fn active_subscription_count(&self) -> Result<u64, String> {
        let connection = self.lock()?;
        let count = connection
            .query_row(
                "SELECT COUNT(*) FROM market_data_test_subscriptions WHERE active = 1",
                [],
                |row| row.get::<_, i64>(0),
            )
            .map_err(|error| error.to_string())?;
        u64::try_from(count).map_err(|_| "negative subscription count".to_owned())
    }

    pub fn lease_status(&self, lease_id: &str) -> Result<Option<String>, String> {
        let connection = self.lock()?;
        connection
            .query_row(
                "SELECT status FROM market_data_test_leases WHERE lease_id = ?1",
                rusqlite::params![lease_id],
                |row| row.get(0),
            )
            .optional()
            .map_err(|error| error.to_string())
    }

    pub fn event_count(&self, operation: &str) -> Result<u64, String> {
        let connection = self.lock()?;
        let count = connection
            .query_row(
                "SELECT COUNT(*) FROM market_data_test_events WHERE operation = ?1",
                rusqlite::params![operation],
                |row| row.get::<_, i64>(0),
            )
            .map_err(|error| error.to_string())?;
        u64::try_from(count).map_err(|_| "negative subscription event count".to_owned())
    }

    pub fn reject_next_event(&self) -> Result<(), String> {
        let mut reject = self
            .reject_next_event
            .lock()
            .map_err(|_| "subscription rejection lock poisoned".to_owned())?;
        *reject = true;
        Ok(())
    }

    fn lock(&self) -> Result<std::sync::MutexGuard<'_, rusqlite::Connection>, String> {
        self.connection
            .lock()
            .map_err(|_| "subscription connection lock poisoned".to_owned())
    }

    fn take_event_rejection(&self) -> Result<bool, MarketDataSubscriptionMutationPortError> {
        let mut reject = self
            .reject_next_event
            .lock()
            .map_err(|_| failed("subscription rejection lock poisoned"))?;
        let value = *reject;
        *reject = false;
        Ok(value)
    }

    fn mutate_transaction(
        &self,
        request: &MarketDataSubscriptionMutationRequest,
    ) -> Result<Value, MarketDataSubscriptionMutationPortError> {
        let operation = operation(request)?;
        let payload = parse_body(&request.body)?;
        let reject_event = self.take_event_rejection()?;
        let connection = self
            .connection
            .lock()
            .map_err(|_| failed("subscription connection lock poisoned"))?;
        let transaction = connection
            .unchecked_transaction()
            .map_err(|_| failed("subscription mutation transaction failed"))?;
        let result = match operation {
            Operation::Acquire => acquire(&transaction, &payload, reject_event),
            Operation::Clear => clear(&transaction, &request.query, reject_event),
            Operation::Release => release(&transaction, &payload, reject_event),
            Operation::Heartbeat => heartbeat(&transaction, &payload, reject_event),
            Operation::PredictionAcquire { code } => {
                prediction_acquire(&transaction, code, &payload, reject_event)
            }
            Operation::PredictionRelease { lease_id } => {
                prediction_release(&transaction, lease_id, reject_event)
            }
        }?;
        transaction
            .commit()
            .map_err(|_| failed("subscription mutation commit failed"))?;
        Ok(result)
    }
}

impl MarketDataSubscriptionMutationPort
    for MarketDataSubscriptionMutationSqliteTestCutoverPort
{
    fn dispatch(
        &self,
        request: &MarketDataSubscriptionMutationRequest,
    ) -> Result<Value, MarketDataSubscriptionMutationPortError> {
        self.mutate_transaction(request)
    }
}

enum Operation<'a> {
    Acquire,
    Clear,
    Release,
    Heartbeat,
    PredictionAcquire { code: &'a str },
    PredictionRelease { lease_id: &'a str },
}

fn operation(
    request: &MarketDataSubscriptionMutationRequest,
) -> Result<Operation<'_>, MarketDataSubscriptionMutationPortError> {
    match (request.method.as_str(), request.path.as_str()) {
        ("POST", "/api/v1/market-data/subscriptions") => Ok(Operation::Acquire),
        ("DELETE", "/api/v1/market-data/subscriptions") => Ok(Operation::Clear),
        ("POST", "/api/v1/market-data/subscriptions/release") => Ok(Operation::Release),
        ("POST", "/api/v1/market-data/subscriptions/heartbeat") => Ok(Operation::Heartbeat),
        ("POST", path) => prediction_operation(path, false),
        ("DELETE", path) => prediction_operation(path, true),
        _ => Err(failed("unknown subscription mutation route")),
    }
}

fn prediction_operation(
    path: &str,
    release: bool,
) -> Result<Operation<'_>, MarketDataSubscriptionMutationPortError> {
    let Some(prefix) = path.strip_prefix("/api/v1/market-data/prediction/contracts/") else {
        return Err(failed("unknown subscription mutation route"));
    };
    let mut segments = prefix.split('/');
    let code = segments
        .next()
        .filter(|value| !value.is_empty())
        .ok_or_else(|| failed("prediction contract code is missing"))?;
    if segments.next() != Some("subscriptions") {
        return Err(failed("unknown subscription mutation route"));
    }
    if release {
        let lease_id = segments
            .next()
            .filter(|value| !value.is_empty())
            .ok_or_else(|| failed("prediction lease id is missing"))?;
        if segments.next().is_some() {
            return Err(failed("unknown subscription mutation route"));
        }
        Ok(Operation::PredictionRelease { lease_id })
    } else if segments.next().is_none() {
        Ok(Operation::PredictionAcquire { code })
    } else {
        Err(failed("unknown subscription mutation route"))
    }
}

fn parse_body(body: &[u8]) -> Result<Value, MarketDataSubscriptionMutationPortError> {
    if body.is_empty() {
        return Ok(Value::Null);
    }
    serde_json::from_slice(body).map_err(|_| failed("subscription request body decode failed"))
}

fn acquire(
    transaction: &rusqlite::Transaction<'_>,
    payload: &Value,
    reject_event: bool,
) -> Result<Value, MarketDataSubscriptionMutationPortError> {
    let consumer_id = string_field(payload, "consumerId");
    let instruments = payload
        .get("instruments")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();
    for instrument in instruments {
        let market = string_field(&instrument, "market");
        let symbol = string_field(&instrument, "symbol");
        if market.is_empty() || symbol.is_empty() {
            continue;
        }
        transaction
            .execute(
                "INSERT INTO market_data_test_subscriptions
                    (consumer_id, market, symbol, active, heartbeats)
                 VALUES (?1, ?2, ?3, 1, 0)
                 ON CONFLICT(consumer_id, market, symbol)
                 DO UPDATE SET active = 1",
                rusqlite::params![consumer_id, market, symbol],
            )
            .map_err(|_| failed("subscription acquire write failed"))?;
    }
    insert_event(transaction, "acquire", &consumer_id, payload, reject_event)?;
    snapshot(transaction, &consumer_id, false)
}

fn clear(
    transaction: &rusqlite::Transaction<'_>,
    query: &str,
    reject_event: bool,
) -> Result<Value, MarketDataSubscriptionMutationPortError> {
    let consumer_id = first_query_value(query, "consumerId");
    let changed = transaction
        .execute(
            "UPDATE market_data_test_subscriptions
             SET active = 0
             WHERE active = 1 AND (?1 = '' OR consumer_id = ?1)",
            rusqlite::params![consumer_id],
        )
        .map_err(|_| failed("subscription clear write failed"))?;
    insert_event(
        transaction,
        "clear",
        &consumer_id,
        &json!({"cleared": changed > 0}),
        reject_event,
    )?;
    Ok(json!({
        "source": "sqlite-test",
        "cleared": true,
        "transitioned": changed > 0,
    }))
}

fn release(
    transaction: &rusqlite::Transaction<'_>,
    payload: &Value,
    reject_event: bool,
) -> Result<Value, MarketDataSubscriptionMutationPortError> {
    let consumer_id = string_field(payload, "consumerId");
    let target = payload
        .get("instruments")
        .and_then(Value::as_array)
        .and_then(|items| items.first());
    let changed = if let Some(target) = target {
        let market = string_field(target, "market");
        let symbol = string_field(target, "symbol");
        transaction.execute(
            "UPDATE market_data_test_subscriptions
             SET active = 0
             WHERE consumer_id = ?1 AND market = ?2 AND symbol = ?3 AND active = 1",
            rusqlite::params![consumer_id, market, symbol],
        )
        .map_err(|_| failed("subscription release write failed"))?
    } else {
        transaction.execute(
            "UPDATE market_data_test_subscriptions
             SET active = 0
             WHERE consumer_id = ?1 AND active = 1",
            rusqlite::params![consumer_id],
        )
        .map_err(|_| failed("subscription release write failed"))?
    };
    insert_event(
        transaction,
        "release",
        &consumer_id,
        payload,
        reject_event,
    )?;
    Ok(json!({
        "source": "sqlite-test",
        "released": true,
        "transitioned": changed > 0,
    }))
}

fn heartbeat(
    transaction: &rusqlite::Transaction<'_>,
    payload: &Value,
    reject_event: bool,
) -> Result<Value, MarketDataSubscriptionMutationPortError> {
    let consumer_id = string_field(payload, "consumerId");
    transaction.execute(
        "UPDATE market_data_test_subscriptions
         SET heartbeats = heartbeats + 1
         WHERE consumer_id = ?1 AND active = 1",
        rusqlite::params![consumer_id],
    )
    .map_err(|_| failed("subscription heartbeat write failed"))?;
    insert_event(transaction, "heartbeat", &consumer_id, payload, reject_event)?;
    snapshot(transaction, &consumer_id, true)
}

fn prediction_acquire(
    transaction: &rusqlite::Transaction<'_>,
    code: &str,
    payload: &Value,
    reject_event: bool,
) -> Result<Value, MarketDataSubscriptionMutationPortError> {
    let value = next_id(transaction)?;
    let lease_id = format!("lease-test-{value}");
    let data_types = payload
        .get("dataTypes")
        .cloned()
        .unwrap_or_else(|| json!([]));
    let data_types_text = serde_json::to_string(&data_types)
        .map_err(|_| failed("prediction data types encode failed"))?;
    transaction.execute(
        "INSERT INTO market_data_test_leases
            (lease_id, contract_code, data_types, status)
         VALUES (?1, ?2, ?3, 'active')",
        rusqlite::params![lease_id, code, data_types_text],
    )
    .map_err(|_| failed("prediction lease write failed"))?;
    insert_event(
        transaction,
        "prediction-acquire",
        &lease_id,
        payload,
        reject_event,
    )?;
    Ok(json!({
        "source": "sqlite-test",
        "leaseId": lease_id,
        "contractCode": code,
        "dataTypes": data_types,
    }))
}

fn prediction_release(
    transaction: &rusqlite::Transaction<'_>,
    lease_id: &str,
    reject_event: bool,
) -> Result<Value, MarketDataSubscriptionMutationPortError> {
    let changed = transaction.execute(
        "UPDATE market_data_test_leases
         SET status = 'released'
         WHERE lease_id = ?1 AND status = 'active'",
        rusqlite::params![lease_id],
    )
    .map_err(|_| failed("prediction lease release failed"))?;
    if changed > 0 {
        insert_event(
            transaction,
            "prediction-release",
            lease_id,
            &Value::Null,
            reject_event,
        )?;
    }
    Ok(json!({
        "source": "sqlite-test",
        "released": true,
        "leaseId": lease_id,
        "transitioned": changed > 0,
    }))
}

fn snapshot(
    transaction: &rusqlite::Transaction<'_>,
    consumer_id: &str,
    heartbeat: bool,
) -> Result<Value, MarketDataSubscriptionMutationPortError> {
    let mut statement = transaction
        .prepare(
            "SELECT market, symbol, heartbeats
             FROM market_data_test_subscriptions
             WHERE consumer_id = ?1 AND active = 1
             ORDER BY market, symbol",
        )
        .map_err(|_| failed("subscription snapshot query failed"))?;
    let rows = statement
        .query_map(rusqlite::params![consumer_id], |row| {
            Ok(json!({
                "market": row.get::<_, String>(0)?,
                "symbol": row.get::<_, String>(1)?,
                "heartbeats": row.get::<_, i64>(2)?,
            }))
        })
        .map_err(|_| failed("subscription snapshot query failed"))?;
    let mut instruments = Vec::new();
    for row in rows {
        instruments.push(row.map_err(|_| failed("subscription snapshot row failed"))?);
    }
    Ok(json!({
        "source": "sqlite-test",
        "consumerId": consumer_id,
        "heartbeat": heartbeat,
        "instruments": instruments,
    }))
}

fn next_id(transaction: &rusqlite::Transaction<'_>) -> Result<i64, MarketDataSubscriptionMutationPortError> {
    let value = transaction
        .query_row(
            "SELECT next_value FROM market_data_test_ids WHERE singleton = 1",
            [],
            |row| row.get::<_, i64>(0),
        )
        .map_err(|_| failed("subscription lease allocation failed"))?;
    transaction
        .execute(
            "UPDATE market_data_test_ids SET next_value = next_value + 1 WHERE singleton = 1",
            [],
        )
        .map_err(|_| failed("subscription lease allocation failed"))?;
    Ok(value)
}

fn insert_event(
    transaction: &rusqlite::Transaction<'_>,
    operation: &str,
    resource_id: &str,
    payload: &Value,
    reject_event: bool,
) -> Result<(), MarketDataSubscriptionMutationPortError> {
    if reject_event {
        return Err(failed("subscription event write rejected"));
    }
    let payload = serde_json::to_string(payload)
        .map_err(|_| failed("subscription event payload encode failed"))?;
    transaction.execute(
        "INSERT INTO market_data_test_events (operation, resource_id, payload)
         VALUES (?1, ?2, ?3)",
        rusqlite::params![operation, resource_id, payload],
    )
    .map_err(|_| failed("subscription event write failed"))?;
    Ok(())
}

fn string_field(value: &Value, field: &str) -> String {
    value
        .get(field)
        .and_then(Value::as_str)
        .unwrap_or_default()
        .trim()
        .to_owned()
}

fn first_query_value(query: &str, key: &str) -> String {
    query
        .split('&')
        .filter_map(|pair| pair.split_once('='))
        .find_map(|(name, value)| (name == key).then(|| value.trim().to_owned()))
        .unwrap_or_default()
}

fn failed(message: &str) -> MarketDataSubscriptionMutationPortError {
    MarketDataSubscriptionMutationPortError::Failed {
        status: 500,
        code: "MARKET_DATA_SUBSCRIPTION_TEST_FAILED".to_owned(),
        message: message.to_owned(),
        retry_after_seconds: None,
    }
}
