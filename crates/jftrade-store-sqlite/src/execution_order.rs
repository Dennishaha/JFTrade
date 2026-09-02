use crate::schema_manifest::{SchemaManifestError, validate_current};
use jftrade_owner_lock::{OwnerDiagnostic, WriterLease, WriterLeaseError};
use rusqlite::{Connection, OpenFlags, OptionalExtension, TransactionBehavior, params};
use serde::{Deserialize, Serialize};
use std::path::{Path, PathBuf};
use std::sync::{Mutex, MutexGuard};
use std::time::Duration;
use thiserror::Error;
use time::{OffsetDateTime, format_description::well_known::Rfc3339};
const EXECUTION_ORDERS_COMPONENT: &str = "execution-orders";
const EXECUTION_ORDERS_SCHEMA_VERSION: i64 = 5;
pub const EXECUTION_ORDERS_TEST_CUTOVER_PROFILE: &str = "cutover-test-only.v1";
pub const EXECUTION_ORDERS_PRODUCTION_PROFILE: &str = "production.v1";
#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredExecutionOrder {
    pub internal_order_id: String,
    pub broker_id: String,
    pub broker_order_id: Option<String>,
    pub broker_order_id_ex: Option<String>,
    pub source: String,
    pub source_detail: String,
    pub trading_environment: String,
    pub account_id: String,
    pub market: String,
    pub symbol: Option<String>,
    pub side: Option<String>,
    pub order_type: Option<String>,
    pub status: String,
    pub raw_broker_status: Option<String>,
    pub requested_quantity: Option<f64>,
    pub requested_price: Option<f64>,
    pub filled_quantity: Option<f64>,
    pub filled_average_price: Option<f64>,
    pub remark: Option<String>,
    pub last_error: Option<String>,
    pub last_error_code: Option<String>,
    pub last_error_source: Option<String>,
    pub submitted_at: Option<String>,
    pub updated_at: String,
    pub created_at: String,
    pub order_kind: String,
    pub product_class: String,
    pub quantity_mode: String,
    pub client_order_id: Option<String>,
    pub preview_id: Option<String>,
    pub normalized_request: String,
    pub requested_amount: Option<f64>,
    pub payout: Option<f64>,
    pub fees: Option<f64>,
}
#[derive(Debug, Error)]
pub enum ExecutionOrderStoreError {
    #[error("execution orders database path is required")]
    EmptyPath,
    #[error("unsupported execution orders writer profile: {0}")]
    UnsupportedProfile(String),
    #[error("execution orders database is not an existing regular file: {0}")]
    NotRegularFile(String),
    #[error(transparent)]
    WriterLease(#[from] WriterLeaseError),
    #[error("open execution orders database: {0}")]
    Open(#[source] rusqlite::Error),
    #[error("configure execution orders database: {0}")]
    Configure(#[source] rusqlite::Error),
    #[error(transparent)]
    Schema(#[from] SchemaManifestError),
    #[error("execution orders database lock is unavailable")]
    LockUnavailable,
    #[error("query execution orders database: {0}")]
    Query(#[source] rusqlite::Error),
    #[error("execution order not found: {0}")]
    NotFound(String),
    #[error("execution order state conflict: {0}")]
    Conflict(String),
    #[error("invalid execution order request: {0}")]
    Validation(String),
    #[error("incompatible execution orders database: {0}")]
    Incompatible(String),
}
pub struct ExecutionOrderStore {
    path: PathBuf,
    connection: Mutex<Connection>,
    _writer_lease: WriterLease,
}
impl std::fmt::Debug for ExecutionOrderStore {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ExecutionOrderStore")
            .field("path", &self.path)
            .finish()
    }
}
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct StoredExecutionOrderEvent<'a> {
    pub id: &'a str,
    pub internal_order_id: &'a str,
    pub event_type: &'a str,
    pub previous_status: Option<&'a str>,
    pub next_status: &'a str,
    pub payload_json: &'a str,
    pub created_at: &'a str,
}
#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredExecutionOrderEventRecord {
    pub id: String,
    pub internal_order_id: String,
    pub event_type: String,
    pub previous_status: Option<String>,
    pub next_status: String,
    pub payload_json: String,
    pub created_at: String,
}
impl ExecutionOrderStore {
    pub fn open(path: impl AsRef<Path>) -> Result<Self, ExecutionOrderStoreError> {
        Self::open_existing(path, EXECUTION_ORDERS_PRODUCTION_PROFILE)
    }
    pub fn open_existing(
        path: impl AsRef<Path>,
        profile: &str,
    ) -> Result<Self, ExecutionOrderStoreError> {
        let path = path.as_ref();
        if path.as_os_str().is_empty() {
            return Err(ExecutionOrderStoreError::EmptyPath);
        }
        if profile != EXECUTION_ORDERS_TEST_CUTOVER_PROFILE
            && profile != EXECUTION_ORDERS_PRODUCTION_PROFILE
        {
            return Err(ExecutionOrderStoreError::UnsupportedProfile(
                profile.to_owned(),
            ));
        }
        if !path
            .metadata()
            .map(|metadata| metadata.is_file())
            .unwrap_or(false)
        {
            return Err(ExecutionOrderStoreError::NotRegularFile(
                path.display().to_string(),
            ));
        }
        let writer_lease = WriterLease::acquire(path, &OwnerDiagnostic::current("rust", profile))?;
        let connection = Connection::open_with_flags(
            path,
            OpenFlags::SQLITE_OPEN_READ_WRITE | OpenFlags::SQLITE_OPEN_NO_MUTEX,
        )
        .map_err(ExecutionOrderStoreError::Open)?;
        connection
            .busy_timeout(Duration::from_secs(10))
            .map_err(ExecutionOrderStoreError::Configure)?;
        connection
            .execute_batch("PRAGMA foreign_keys = ON;")
            .map_err(ExecutionOrderStoreError::Configure)?;
        validate_current(
            &connection,
            &path.display().to_string(),
            EXECUTION_ORDERS_COMPONENT,
            EXECUTION_ORDERS_SCHEMA_VERSION,
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
    pub(crate) fn lock(&self) -> Result<MutexGuard<'_, Connection>, ExecutionOrderStoreError> {
        self.connection
            .lock()
            .map_err(|_| ExecutionOrderStoreError::LockUnavailable)
    }
    pub fn save_order(
        &self,
        order: StoredExecutionOrder,
        timestamp: &str,
    ) -> Result<StoredExecutionOrder, ExecutionOrderStoreError> {
        validate_rfc3339_timestamp(timestamp)?;
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(ExecutionOrderStoreError::Query)?;
        let created_at = if order.created_at.is_empty() {
            timestamp.to_owned()
        } else {
            order.created_at.clone()
        };
        upsert_order(&transaction, &order, timestamp, &created_at)?;
        transaction
            .commit()
            .map_err(ExecutionOrderStoreError::Query)?;
        let mut saved = order;
        saved.created_at = created_at;
        saved.updated_at = timestamp.to_owned();
        Ok(saved)
    }

    /// Reserve an order identity without overwriting an order that another
    /// request may have created concurrently.  The client-order uniqueness
    /// fence is the complete broker/environment/account/client tuple defined
    /// by the execution schema.  A false result means the tuple (or internal
    /// id) was already present and the caller must replay that durable order.
    pub fn reserve_order(
        &self,
        order: StoredExecutionOrder,
        timestamp: &str,
    ) -> Result<bool, ExecutionOrderStoreError> {
        validate_rfc3339_timestamp(timestamp)?;
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(ExecutionOrderStoreError::Query)?;
        let created_at = if order.created_at.is_empty() {
            timestamp.to_owned()
        } else {
            order.created_at.clone()
        };
        let result = transaction
            .execute(
                "INSERT OR IGNORE INTO execution_orders (
                internal_order_id, broker_id, broker_order_id, broker_order_id_ex,
                source, source_detail, trading_environment, account_id, market,
                symbol, side, order_type, status, raw_broker_status,
                requested_quantity, requested_price, filled_quantity, filled_average_price,
                remark, last_error, last_error_code, last_error_source,
                submitted_at, updated_at, created_at, order_kind, product_class,
                quantity_mode, client_order_id, preview_id, normalized_request,
                requested_amount, payout, fees
            ) VALUES (
                ?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9,
                ?10, ?11, ?12, ?13, ?14, ?15, ?16, ?17, ?18,
                ?19, ?20, ?21, ?22, ?23, ?24, ?25, ?26, ?27,
                ?28, ?29, ?30, ?31, ?32, ?33, ?34
            )",
                params![
                    order.internal_order_id,
                    order.broker_id,
                    order.broker_order_id,
                    order.broker_order_id_ex,
                    order.source,
                    order.source_detail,
                    order.trading_environment,
                    order.account_id,
                    order.market,
                    order.symbol,
                    order.side,
                    order.order_type,
                    order.status,
                    order.raw_broker_status,
                    order.requested_quantity,
                    order.requested_price,
                    order.filled_quantity,
                    order.filled_average_price,
                    order.remark,
                    order.last_error,
                    order.last_error_code,
                    order.last_error_source,
                    order.submitted_at,
                    timestamp,
                    created_at,
                    order.order_kind,
                    order.product_class,
                    order.quantity_mode,
                    order.client_order_id,
                    order.preview_id,
                    order.normalized_request,
                    order.requested_amount,
                    order.payout,
                    order.fees,
                ],
            )
            .map_err(ExecutionOrderStoreError::Query)?;
        transaction
            .commit()
            .map_err(ExecutionOrderStoreError::Query)?;
        Ok(result > 0)
    }
    /// Atomically persists an order projection and its lifecycle event.
    ///
    /// Command handlers use this for every broker acknowledgement/failure so
    /// a durable order status can never be committed without its matching
    /// event (or vice versa).
    pub fn save_order_and_event(
        &self,
        order: StoredExecutionOrder,
        timestamp: &str,
        event: &StoredExecutionOrderEvent<'_>,
    ) -> Result<StoredExecutionOrder, ExecutionOrderStoreError> {
        validate_rfc3339_timestamp(timestamp)?;
        validate_rfc3339_timestamp(event.created_at)?;
        if event.internal_order_id != order.internal_order_id {
            return Err(ExecutionOrderStoreError::Validation(
                "order and event internal ids must match".to_owned(),
            ));
        }
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(ExecutionOrderStoreError::Query)?;
        let created_at = if order.created_at.is_empty() {
            timestamp.to_owned()
        } else {
            order.created_at.clone()
        };
        upsert_order(&transaction, &order, timestamp, &created_at)?;
        insert_event(&transaction, event)?;
        transaction
            .commit()
            .map_err(ExecutionOrderStoreError::Query)?;
        let mut saved = order;
        saved.created_at = created_at;
        saved.updated_at = timestamp.to_owned();
        Ok(saved)
    }
    /// Atomically applies a lifecycle transition fenced by the state observed
    /// by the caller.  The execution-orders schema is shared with Go and does
    /// not expose a revision column, so the durable CAS token is the pair of
    /// `status` and `updated_at`.  Both values are checked inside the same
    /// immediate transaction as the order/event write; a stale worker can
    /// therefore never overwrite a newer broker update.
    pub fn transition_order_and_event(
        &self,
        order: StoredExecutionOrder,
        timestamp: &str,
        event: &StoredExecutionOrderEvent<'_>,
        expected_status: &str,
        expected_updated_at: &str,
    ) -> Result<StoredExecutionOrder, ExecutionOrderStoreError> {
        self.transition_order_and_event_fenced(
            order,
            timestamp,
            event,
            expected_status,
            expected_updated_at,
            None,
        )
    }

    /// Applies a transition only when the durable event revision still
    /// matches the caller's snapshot.  Event count is the schema-compatible
    /// revision: reservations start at zero and every committed transition
    /// appends exactly one event in the same immediate transaction.
    pub fn transition_order_and_event_fenced(
        &self,
        order: StoredExecutionOrder,
        timestamp: &str,
        event: &StoredExecutionOrderEvent<'_>,
        expected_status: &str,
        expected_updated_at: &str,
        expected_revision: Option<u64>,
    ) -> Result<StoredExecutionOrder, ExecutionOrderStoreError> {
        validate_rfc3339_timestamp(timestamp)?;
        validate_rfc3339_timestamp(event.created_at)?;
        if event.internal_order_id != order.internal_order_id {
            return Err(ExecutionOrderStoreError::Validation(
                "order and event internal ids must match".to_owned(),
            ));
        }
        if event.previous_status != Some(expected_status) {
            return Err(ExecutionOrderStoreError::Validation(
                "event previous status does not match the CAS status".to_owned(),
            ));
        }
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(ExecutionOrderStoreError::Query)?;
        let current = transaction
            .query_row(
                "SELECT status, updated_at FROM execution_orders
                 WHERE internal_order_id = ?1",
                params![order.internal_order_id],
                |row| Ok((row.get::<_, String>(0)?, row.get::<_, String>(1)?)),
            )
            .optional()
            .map_err(ExecutionOrderStoreError::Query)?;
        let Some((current_status, current_updated_at)) = current else {
            return Err(ExecutionOrderStoreError::NotFound(
                order.internal_order_id.clone(),
            ));
        };
        if current_status != expected_status || current_updated_at != expected_updated_at {
            return Err(ExecutionOrderStoreError::Conflict(format!(
                "order {} changed while applying {} (expected status={} updatedAt={}, got status={} updatedAt={})",
                order.internal_order_id,
                event.event_type,
                expected_status,
                expected_updated_at,
                current_status,
                current_updated_at,
            )));
        }
        if let Some(expected_revision) = expected_revision {
            let current_revision: i64 = transaction
                .query_row(
                    "SELECT COUNT(*) FROM execution_order_events
                     WHERE internal_order_id = ?1",
                    params![order.internal_order_id],
                    |row| row.get(0),
                )
                .map_err(ExecutionOrderStoreError::Query)?;
            if current_revision < 0 || current_revision as u64 != expected_revision {
                return Err(ExecutionOrderStoreError::Conflict(format!(
                    "order {} revision changed while applying {} (expected revision={}, got {})",
                    order.internal_order_id, event.event_type, expected_revision, current_revision,
                )));
            }
        }
        let created_at = if order.created_at.is_empty() {
            timestamp.to_owned()
        } else {
            order.created_at.clone()
        };
        upsert_order(&transaction, &order, timestamp, &created_at)?;
        insert_event(&transaction, event)?;
        transaction
            .commit()
            .map_err(ExecutionOrderStoreError::Query)?;
        let mut saved = order;
        saved.created_at = created_at;
        saved.updated_at = timestamp.to_owned();
        Ok(saved)
    }

    /// Returns the durable lifecycle revision for an order.  This is derived
    /// from the event journal to keep the shared Go SQLite schema unchanged.
    pub fn order_revision(&self, id: &str) -> Result<u64, ExecutionOrderStoreError> {
        let connection = self.lock()?;
        let revision: i64 = connection
            .query_row(
                "SELECT COUNT(*) FROM execution_order_events WHERE internal_order_id = ?1",
                params![id],
                |row| row.get(0),
            )
            .map_err(ExecutionOrderStoreError::Query)?;
        u64::try_from(revision).map_err(|_| {
            ExecutionOrderStoreError::Incompatible(format!(
                "negative execution order revision for {id}"
            ))
        })
    }

    /// Lists non-terminal rows that may need broker reconciliation after a
    /// timeout, crash, or process restart.  Rows without broker identifiers
    /// are intentionally retained so callers can surface them as UNKNOWN
    /// rather than retrying a potentially duplicated command.
    pub fn list_reconciliation_candidates(
        &self,
    ) -> Result<Vec<StoredExecutionOrder>, ExecutionOrderStoreError> {
        let connection = self.lock()?;
        let mut statement = connection
            .prepare(
                "SELECT internal_order_id, broker_id, broker_order_id, broker_order_id_ex,
                        source, source_detail, trading_environment, account_id, market,
                        symbol, side, order_type, status, raw_broker_status,
                        requested_quantity, requested_price, filled_quantity,
                        filled_average_price, remark, last_error, last_error_code,
                        last_error_source, submitted_at, updated_at, created_at,
                        order_kind, product_class, quantity_mode, client_order_id,
                        preview_id, normalized_request, requested_amount, payout, fees
                 FROM execution_orders
                 WHERE UPPER(status) IN (
                    'SUBMITTING', 'SUBMISSION_UNKNOWN', 'UNKNOWN',
                    'SUBMITTED', 'BROKER_ACCEPTED', 'PARTIALLY_FILLED',
                    'CANCEL_REQUESTED', 'CANCEL_SUBMITTED'
                 )
                 ORDER BY updated_at ASC, created_at ASC, internal_order_id ASC",
            )
            .map_err(ExecutionOrderStoreError::Query)?;
        let rows = statement
            .query_map([], read_order)
            .map_err(ExecutionOrderStoreError::Query)?;
        rows.map(|row| row.map_err(ExecutionOrderStoreError::Query))
            .collect()
    }
    pub fn get_order(
        &self,
        id: &str,
    ) -> Result<Option<StoredExecutionOrder>, ExecutionOrderStoreError> {
        let connection = self.lock()?;
        let row = connection
            .query_row(
                "SELECT internal_order_id, broker_id, broker_order_id, broker_order_id_ex,
                        source, source_detail, trading_environment, account_id, market,
                        symbol, side, order_type, status, raw_broker_status,
                        requested_quantity, requested_price, filled_quantity, filled_average_price,
                        remark, last_error, last_error_code, last_error_source,
                        submitted_at, updated_at, created_at, order_kind, product_class,
                        quantity_mode, client_order_id, preview_id, normalized_request,
                        requested_amount, payout, fees
                 FROM execution_orders WHERE internal_order_id = ?1",
                params![id],
                |row| {
                    Ok(StoredExecutionOrder {
                        internal_order_id: row.get(0)?,
                        broker_id: row.get(1)?,
                        broker_order_id: row.get(2)?,
                        broker_order_id_ex: row.get(3)?,
                        source: row.get(4)?,
                        source_detail: row.get(5)?,
                        trading_environment: row.get(6)?,
                        account_id: row.get(7)?,
                        market: row.get(8)?,
                        symbol: row.get(9)?,
                        side: row.get(10)?,
                        order_type: row.get(11)?,
                        status: row.get(12)?,
                        raw_broker_status: row.get(13)?,
                        requested_quantity: row.get(14)?,
                        requested_price: row.get(15)?,
                        filled_quantity: row.get(16)?,
                        filled_average_price: row.get(17)?,
                        remark: row.get(18)?,
                        last_error: row.get(19)?,
                        last_error_code: row.get(20)?,
                        last_error_source: row.get(21)?,
                        submitted_at: row.get(22)?,
                        updated_at: row.get(23)?,
                        created_at: row.get(24)?,
                        order_kind: row.get(25)?,
                        product_class: row.get(26)?,
                        quantity_mode: row.get(27)?,
                        client_order_id: row.get(28)?,
                        preview_id: row.get(29)?,
                        normalized_request: row.get(30)?,
                        requested_amount: row.get(31)?,
                        payout: row.get(32)?,
                        fees: row.get(33)?,
                    })
                },
            )
            .optional()
            .map_err(ExecutionOrderStoreError::Query)?;
        Ok(row)
    }
    pub fn order_count(&self) -> Result<u64, ExecutionOrderStoreError> {
        let connection = self.lock()?;
        let count: i64 = connection
            .query_row("SELECT COUNT(*) FROM execution_orders", [], |row| {
                row.get(0)
            })
            .map_err(ExecutionOrderStoreError::Query)?;
        Ok(count as u64)
    }
    pub fn list_orders(&self) -> Result<Vec<StoredExecutionOrder>, ExecutionOrderStoreError> {
        let connection = self.lock()?;
        let mut statement = connection
            .prepare(
                "SELECT internal_order_id, broker_id, broker_order_id, broker_order_id_ex,
                        source, source_detail, trading_environment, account_id, market,
                        symbol, side, order_type, status, raw_broker_status,
                        requested_quantity, requested_price, filled_quantity, filled_average_price,
                        remark, last_error, last_error_code, last_error_source,
                        submitted_at, updated_at, created_at, order_kind, product_class,
                        quantity_mode, client_order_id, preview_id, normalized_request,
                        requested_amount, payout, fees
                 FROM execution_orders ORDER BY created_at DESC",
            )
            .map_err(ExecutionOrderStoreError::Query)?;
        let rows = statement
            .query_map([], |row| {
                Ok(StoredExecutionOrder {
                    internal_order_id: row.get(0)?,
                    broker_id: row.get(1)?,
                    broker_order_id: row.get(2)?,
                    broker_order_id_ex: row.get(3)?,
                    source: row.get(4)?,
                    source_detail: row.get(5)?,
                    trading_environment: row.get(6)?,
                    account_id: row.get(7)?,
                    market: row.get(8)?,
                    symbol: row.get(9)?,
                    side: row.get(10)?,
                    order_type: row.get(11)?,
                    status: row.get(12)?,
                    raw_broker_status: row.get(13)?,
                    requested_quantity: row.get(14)?,
                    requested_price: row.get(15)?,
                    filled_quantity: row.get(16)?,
                    filled_average_price: row.get(17)?,
                    remark: row.get(18)?,
                    last_error: row.get(19)?,
                    last_error_code: row.get(20)?,
                    last_error_source: row.get(21)?,
                    submitted_at: row.get(22)?,
                    updated_at: row.get(23)?,
                    created_at: row.get(24)?,
                    order_kind: row.get(25)?,
                    product_class: row.get(26)?,
                    quantity_mode: row.get(27)?,
                    client_order_id: row.get(28)?,
                    preview_id: row.get(29)?,
                    normalized_request: row.get(30)?,
                    requested_amount: row.get(31)?,
                    payout: row.get(32)?,
                    fees: row.get(33)?,
                })
            })
            .map_err(ExecutionOrderStoreError::Query)?;
        let mut orders = Vec::new();
        for row in rows {
            orders.push(row.map_err(ExecutionOrderStoreError::Query)?);
        }
        Ok(orders)
    }
    pub fn cancel_order(
        &self,
        id: &str,
        timestamp: &str,
    ) -> Result<bool, ExecutionOrderStoreError> {
        validate_rfc3339_timestamp(timestamp)?;
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(ExecutionOrderStoreError::Query)?;
        let changes = transaction
            .execute(
                "UPDATE execution_orders SET status = 'cancelled', updated_at = ?2
                 WHERE internal_order_id = ?1 AND status <> 'cancelled'",
                params![id, timestamp],
            )
            .map_err(ExecutionOrderStoreError::Query)?;
        transaction
            .commit()
            .map_err(ExecutionOrderStoreError::Query)?;
        Ok(changes > 0)
    }
    pub fn record_event(
        &self,
        event: &StoredExecutionOrderEvent<'_>,
    ) -> Result<(), ExecutionOrderStoreError> {
        validate_rfc3339_timestamp(event.created_at)?;
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(ExecutionOrderStoreError::Query)?;
        insert_event(&transaction, event)?;
        transaction
            .commit()
            .map_err(ExecutionOrderStoreError::Query)?;
        Ok(())
    }
    pub fn event_count(&self, event_type: &str) -> Result<u64, ExecutionOrderStoreError> {
        let connection = self.lock()?;
        let count: i64 = connection
            .query_row(
                "SELECT COUNT(*) FROM execution_order_events WHERE event_type = ?1",
                params![event_type],
                |row| row.get(0),
            )
            .map_err(ExecutionOrderStoreError::Query)?;
        Ok(count as u64)
    }
    pub fn list_order_events(
        &self,
        internal_order_id: &str,
    ) -> Result<Vec<StoredExecutionOrderEventRecord>, ExecutionOrderStoreError> {
        let connection = self.lock()?;
        let mut statement = connection
            .prepare(
                "SELECT id, internal_order_id, event_type, previous_status,
                        next_status, payload_json, created_at
                 FROM execution_order_events
                 WHERE internal_order_id = ?1 ORDER BY created_at ASC, id ASC",
            )
            .map_err(ExecutionOrderStoreError::Query)?;
        let rows = statement
            .query_map(params![internal_order_id], |row| {
                Ok(StoredExecutionOrderEventRecord {
                    id: row.get(0)?,
                    internal_order_id: row.get(1)?,
                    event_type: row.get(2)?,
                    previous_status: row.get(3)?,
                    next_status: row.get(4)?,
                    payload_json: row.get(5)?,
                    created_at: row.get(6)?,
                })
            })
            .map_err(ExecutionOrderStoreError::Query)?;
        rows.map(|row| row.map_err(ExecutionOrderStoreError::Query))
            .collect()
    }
    pub fn next_sequence(&self, name: &str) -> Result<i64, ExecutionOrderStoreError> {
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(ExecutionOrderStoreError::Query)?;
        transaction
            .execute(
                "INSERT INTO execution_sequences (name, value) VALUES (?1, 1)
                 ON CONFLICT(name) DO UPDATE SET value = value + 1",
                params![name],
            )
            .map_err(ExecutionOrderStoreError::Query)?;
        let val: i64 = transaction
            .query_row(
                "SELECT value FROM execution_sequences WHERE name = ?1",
                params![name],
                |row| row.get(0),
            )
            .map_err(ExecutionOrderStoreError::Query)?;
        transaction
            .commit()
            .map_err(ExecutionOrderStoreError::Query)?;
        Ok(val)
    }
}
fn upsert_order(
    transaction: &rusqlite::Transaction<'_>,
    order: &StoredExecutionOrder,
    timestamp: &str,
    created_at: &str,
) -> Result<(), ExecutionOrderStoreError> {
    transaction
        .execute(
            "INSERT INTO execution_orders (
                internal_order_id, broker_id, broker_order_id, broker_order_id_ex,
                source, source_detail, trading_environment, account_id, market,
                symbol, side, order_type, status, raw_broker_status,
                requested_quantity, requested_price, filled_quantity, filled_average_price,
                remark, last_error, last_error_code, last_error_source,
                submitted_at, updated_at, created_at, order_kind, product_class,
                quantity_mode, client_order_id, preview_id, normalized_request,
                requested_amount, payout, fees
            ) VALUES (
                ?1, ?2, ?3, ?4,
                ?5, ?6, ?7, ?8, ?9,
                ?10, ?11, ?12, ?13, ?14,
                ?15, ?16, ?17, ?18,
                ?19, ?20, ?21, ?22,
                ?23, ?24, ?25, ?26, ?27,
                ?28, ?29, ?30, ?31,
                ?32, ?33, ?34
            ) ON CONFLICT(internal_order_id) DO UPDATE SET
                broker_id = excluded.broker_id,
                broker_order_id = excluded.broker_order_id,
                broker_order_id_ex = excluded.broker_order_id_ex,
                source = excluded.source,
                source_detail = excluded.source_detail,
                trading_environment = excluded.trading_environment,
                account_id = excluded.account_id,
                market = excluded.market,
                symbol = excluded.symbol,
                side = excluded.side,
                order_type = excluded.order_type,
                status = excluded.status,
                raw_broker_status = excluded.raw_broker_status,
                requested_quantity = excluded.requested_quantity,
                requested_price = excluded.requested_price,
                filled_quantity = excluded.filled_quantity,
                filled_average_price = excluded.filled_average_price,
                remark = excluded.remark,
                last_error = excluded.last_error,
                last_error_code = excluded.last_error_code,
                last_error_source = excluded.last_error_source,
                submitted_at = excluded.submitted_at,
                updated_at = excluded.updated_at,
                order_kind = excluded.order_kind,
                product_class = excluded.product_class,
                quantity_mode = excluded.quantity_mode,
                client_order_id = excluded.client_order_id,
                preview_id = excluded.preview_id,
                normalized_request = excluded.normalized_request,
                requested_amount = excluded.requested_amount,
                payout = excluded.payout,
                fees = excluded.fees",
            params![
                order.internal_order_id,
                order.broker_id,
                order.broker_order_id,
                order.broker_order_id_ex,
                order.source,
                order.source_detail,
                order.trading_environment,
                order.account_id,
                order.market,
                order.symbol,
                order.side,
                order.order_type,
                order.status,
                order.raw_broker_status,
                order.requested_quantity,
                order.requested_price,
                order.filled_quantity,
                order.filled_average_price,
                order.remark,
                order.last_error,
                order.last_error_code,
                order.last_error_source,
                order.submitted_at,
                timestamp,
                created_at,
                order.order_kind,
                order.product_class,
                order.quantity_mode,
                order.client_order_id,
                order.preview_id,
                order.normalized_request,
                order.requested_amount,
                order.payout,
                order.fees,
            ],
        )
        .map_err(ExecutionOrderStoreError::Query)?;
    Ok(())
}

fn read_order(row: &rusqlite::Row<'_>) -> rusqlite::Result<StoredExecutionOrder> {
    Ok(StoredExecutionOrder {
        internal_order_id: row.get(0)?,
        broker_id: row.get(1)?,
        broker_order_id: row.get(2)?,
        broker_order_id_ex: row.get(3)?,
        source: row.get(4)?,
        source_detail: row.get(5)?,
        trading_environment: row.get(6)?,
        account_id: row.get(7)?,
        market: row.get(8)?,
        symbol: row.get(9)?,
        side: row.get(10)?,
        order_type: row.get(11)?,
        status: row.get(12)?,
        raw_broker_status: row.get(13)?,
        requested_quantity: row.get(14)?,
        requested_price: row.get(15)?,
        filled_quantity: row.get(16)?,
        filled_average_price: row.get(17)?,
        remark: row.get(18)?,
        last_error: row.get(19)?,
        last_error_code: row.get(20)?,
        last_error_source: row.get(21)?,
        submitted_at: row.get(22)?,
        updated_at: row.get(23)?,
        created_at: row.get(24)?,
        order_kind: row.get(25)?,
        product_class: row.get(26)?,
        quantity_mode: row.get(27)?,
        client_order_id: row.get(28)?,
        preview_id: row.get(29)?,
        normalized_request: row.get(30)?,
        requested_amount: row.get(31)?,
        payout: row.get(32)?,
        fees: row.get(33)?,
    })
}

fn insert_event(
    transaction: &rusqlite::Transaction<'_>,
    event: &StoredExecutionOrderEvent<'_>,
) -> Result<(), ExecutionOrderStoreError> {
    transaction
        .execute(
            "INSERT INTO execution_order_events (
                id, internal_order_id, event_type, previous_status, next_status, payload_json, created_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)",
            params![
                event.id,
                event.internal_order_id,
                event.event_type,
                event.previous_status,
                event.next_status,
                event.payload_json,
                event.created_at,
            ],
        )
        .map_err(ExecutionOrderStoreError::Query)?;
    Ok(())
}
fn validate_rfc3339_timestamp(timestamp: &str) -> Result<(), ExecutionOrderStoreError> {
    OffsetDateTime::parse(timestamp, &Rfc3339)
        .map(|_| ())
        .map_err(|error| {
            ExecutionOrderStoreError::Incompatible(format!(
                "invalid RFC3339 timestamp {timestamp:?}: {error}"
            ))
        })
}
#[derive(Debug)]
pub struct ExecutionOrderTestCutoverStore {
    inner: ExecutionOrderStore,
}
impl ExecutionOrderTestCutoverStore {
    pub fn open_existing(
        path: impl AsRef<Path>,
        profile: &str,
    ) -> Result<Self, ExecutionOrderStoreError> {
        let inner = ExecutionOrderStore::open_existing(path, profile)?;
        Ok(Self { inner })
    }
    pub fn path(&self) -> &Path {
        self.inner.path()
    }
    pub fn save_order(
        &self,
        order: StoredExecutionOrder,
        timestamp: &str,
    ) -> Result<StoredExecutionOrder, ExecutionOrderStoreError> {
        self.inner.save_order(order, timestamp)
    }
    pub fn reserve_order_with_preview(
        &self,
        order: StoredExecutionOrder,
        request_hash: &str,
        timestamp: &str,
    ) -> Result<
        crate::execution_order_reservation::ExecutionOrderReservation,
        ExecutionOrderStoreError,
    > {
        self.inner
            .reserve_order_with_preview(order, request_hash, timestamp)
    }
    pub fn reserve_order_with_preview_checked(
        &self,
        order: StoredExecutionOrder,
        request_hash: &str,
        timestamp: &str,
        expected_capability_version: Option<&str>,
    ) -> Result<
        crate::execution_order_reservation::ExecutionOrderReservation,
        ExecutionOrderStoreError,
    > {
        self.inner.reserve_order_with_preview_checked(
            order,
            request_hash,
            timestamp,
            expected_capability_version,
        )
    }
    pub fn find_order_by_client_identity(
        &self,
        broker_id: &str,
        trading_environment: &str,
        account_id: &str,
        client_order_id: &str,
    ) -> Result<Option<StoredExecutionOrder>, ExecutionOrderStoreError> {
        self.inner.find_order_by_client_identity(
            broker_id,
            trading_environment,
            account_id,
            client_order_id,
        )
    }
    pub fn save_order_and_event(
        &self,
        order: StoredExecutionOrder,
        timestamp: &str,
        event: &StoredExecutionOrderEvent<'_>,
    ) -> Result<StoredExecutionOrder, ExecutionOrderStoreError> {
        self.inner.save_order_and_event(order, timestamp, event)
    }
    pub fn transition_order_and_event(
        &self,
        order: StoredExecutionOrder,
        timestamp: &str,
        event: &StoredExecutionOrderEvent<'_>,
        expected_status: &str,
        expected_updated_at: &str,
    ) -> Result<StoredExecutionOrder, ExecutionOrderStoreError> {
        self.inner.transition_order_and_event(
            order,
            timestamp,
            event,
            expected_status,
            expected_updated_at,
        )
    }
    pub fn transition_order_and_event_fenced(
        &self,
        order: StoredExecutionOrder,
        timestamp: &str,
        event: &StoredExecutionOrderEvent<'_>,
        expected_status: &str,
        expected_updated_at: &str,
        expected_revision: Option<u64>,
    ) -> Result<StoredExecutionOrder, ExecutionOrderStoreError> {
        self.inner.transition_order_and_event_fenced(
            order,
            timestamp,
            event,
            expected_status,
            expected_updated_at,
            expected_revision,
        )
    }
    pub fn order_revision(&self, id: &str) -> Result<u64, ExecutionOrderStoreError> {
        self.inner.order_revision(id)
    }
    pub fn list_reconciliation_candidates(
        &self,
    ) -> Result<Vec<StoredExecutionOrder>, ExecutionOrderStoreError> {
        self.inner.list_reconciliation_candidates()
    }
    pub fn get_order(
        &self,
        id: &str,
    ) -> Result<Option<StoredExecutionOrder>, ExecutionOrderStoreError> {
        self.inner.get_order(id)
    }
    pub fn order_count(&self) -> Result<u64, ExecutionOrderStoreError> {
        self.inner.order_count()
    }
    pub fn list_orders(&self) -> Result<Vec<StoredExecutionOrder>, ExecutionOrderStoreError> {
        self.inner.list_orders()
    }
    pub fn cancel_order(
        &self,
        id: &str,
        timestamp: &str,
    ) -> Result<bool, ExecutionOrderStoreError> {
        self.inner.cancel_order(id, timestamp)
    }
    pub fn record_event(
        &self,
        event: &StoredExecutionOrderEvent<'_>,
    ) -> Result<(), ExecutionOrderStoreError> {
        self.inner.record_event(event)
    }
    pub fn event_count(&self, event_type: &str) -> Result<u64, ExecutionOrderStoreError> {
        self.inner.event_count(event_type)
    }
    pub fn list_order_events(
        &self,
        internal_order_id: &str,
    ) -> Result<Vec<StoredExecutionOrderEventRecord>, ExecutionOrderStoreError> {
        self.inner.list_order_events(internal_order_id)
    }
    pub fn next_sequence(&self, name: &str) -> Result<i64, ExecutionOrderStoreError> {
        self.inner.next_sequence(name)
    }
}
