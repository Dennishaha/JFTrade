//! Atomic preview consumption and execution-order identity reservation.

use rusqlite::{OptionalExtension, TransactionBehavior, params};
use serde_json::Value;
use sha2::{Digest, Sha256};

use super::execution_order::{ExecutionOrderStore, ExecutionOrderStoreError, StoredExecutionOrder};

/// Result of the durable submission fence.
#[derive(Clone, Debug, PartialEq)]
pub enum ExecutionOrderReservation {
    /// The caller owns the newly inserted `SUBMITTING` order and may call the
    /// broker exactly once.
    Reserved(StoredExecutionOrder),
    /// An earlier request already owns this client-order identity.  The
    /// caller must compare request hashes before replaying this projection.
    Existing(StoredExecutionOrder),
}

impl ExecutionOrderStore {
    /// Find a previously reserved client-order identity without opening a
    /// write transaction. Callers use this only as a readiness preflight;
    /// `reserve_order_with_preview_checked` remains the atomic race fence.
    pub fn find_order_by_client_identity(
        &self,
        broker_id: &str,
        trading_environment: &str,
        account_id: &str,
        client_order_id: &str,
    ) -> Result<Option<StoredExecutionOrder>, ExecutionOrderStoreError> {
        let client_order_id = client_order_id.trim();
        if client_order_id.is_empty() {
            return Ok(None);
        }
        let connection = self.lock()?;
        connection
            .query_row(
                "SELECT internal_order_id, broker_id, broker_order_id, broker_order_id_ex,
                        source, source_detail, trading_environment, account_id, market,
                        symbol, side, order_type, status, raw_broker_status,
                        requested_quantity, requested_price, filled_quantity,
                        filled_average_price, remark, last_error, last_error_code,
                        last_error_source, submitted_at, updated_at, created_at,
                        order_kind, product_class, quantity_mode, client_order_id,
                        preview_id, normalized_request, requested_amount, payout, fees
                 FROM execution_orders
                 WHERE broker_id = ?1 COLLATE NOCASE
                   AND trading_environment = ?2 COLLATE NOCASE
                   AND account_id = ?3
                   AND client_order_id = ?4 COLLATE NOCASE
                 ORDER BY created_at ASC, internal_order_id ASC
                 LIMIT 1",
                params![broker_id, trading_environment, account_id, client_order_id],
                read_order,
            )
            .optional()
            .map_err(ExecutionOrderStoreError::Query)
    }

    /// Atomically validates/consumes an optional preview and reserves an order
    /// identity.  The transaction first checks the complete broker,
    /// environment, account, and client-order tuple.  Existing identities are
    /// returned without consuming another preview; new identities consume and
    /// insert in one immediate transaction so a failed validation cannot leave
    /// a credential spent or a broker-call placeholder behind.
    pub fn reserve_order_with_preview(
        &self,
        order: StoredExecutionOrder,
        request_hash: &str,
        timestamp: &str,
    ) -> Result<ExecutionOrderReservation, ExecutionOrderStoreError> {
        self.reserve_order_with_preview_checked(order, request_hash, timestamp, None)
    }

    /// Production variant that verifies the capability version inside the
    /// same transaction as preview consumption.
    pub fn reserve_order_with_preview_checked(
        &self,
        order: StoredExecutionOrder,
        request_hash: &str,
        timestamp: &str,
        expected_capability_version: Option<&str>,
    ) -> Result<ExecutionOrderReservation, ExecutionOrderStoreError> {
        if request_hash.trim().is_empty() {
            return Err(ExecutionOrderStoreError::Validation(
                "execution request hash is required".to_owned(),
            ));
        }
        validate_timestamp(timestamp)?;
        let now =
            time::OffsetDateTime::parse(timestamp, &time::format_description::well_known::Rfc3339)
                .map_err(|error| {
                    ExecutionOrderStoreError::Validation(format!(
                        "invalid preview timestamp: {error}"
                    ))
                })?;
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(ExecutionOrderStoreError::Query)?;

        if let Some(existing) = find_existing_order(&transaction, &order)? {
            transaction
                .commit()
                .map_err(ExecutionOrderStoreError::Query)?;
            return Ok(ExecutionOrderReservation::Existing(existing));
        }

        if let Some(preview_id) = order.preview_id.as_deref() {
            super::execution_order_preview::consume_preview_in_transaction(
                &transaction,
                preview_id,
                &order.broker_id,
                &order.account_id,
                request_hash,
                timestamp,
                now,
                expected_capability_version,
            )?;
        }
        insert_order(&transaction, &order, timestamp)?;
        transaction
            .commit()
            .map_err(ExecutionOrderStoreError::Query)?;
        Ok(ExecutionOrderReservation::Reserved(order))
    }
}

fn find_existing_order(
    transaction: &rusqlite::Transaction<'_>,
    order: &StoredExecutionOrder,
) -> Result<Option<StoredExecutionOrder>, ExecutionOrderStoreError> {
    let Some(client_order_id) = order
        .client_order_id
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty())
    else {
        return Ok(None);
    };
    transaction
        .query_row(
            "SELECT internal_order_id, broker_id, broker_order_id, broker_order_id_ex,
                    source, source_detail, trading_environment, account_id, market,
                    symbol, side, order_type, status, raw_broker_status,
                    requested_quantity, requested_price, filled_quantity,
                    filled_average_price, remark, last_error, last_error_code,
                    last_error_source, submitted_at, updated_at, created_at,
                    order_kind, product_class, quantity_mode, client_order_id,
                    preview_id, normalized_request, requested_amount, payout, fees
             FROM execution_orders
             WHERE broker_id = ?1 COLLATE NOCASE
               AND trading_environment = ?2 COLLATE NOCASE
               AND account_id = ?3
               AND client_order_id = ?4 COLLATE NOCASE
             ORDER BY created_at ASC, internal_order_id ASC
             LIMIT 1",
            params![
                order.broker_id,
                order.trading_environment,
                order.account_id,
                client_order_id,
            ],
            read_order,
        )
        .optional()
        .map_err(ExecutionOrderStoreError::Query)
}

fn insert_order(
    transaction: &rusqlite::Transaction<'_>,
    order: &StoredExecutionOrder,
    timestamp: &str,
) -> Result<(), ExecutionOrderStoreError> {
    let created_at = if order.created_at.is_empty() {
        timestamp.to_owned()
    } else {
        order.created_at.clone()
    };
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

fn validate_timestamp(timestamp: &str) -> Result<(), ExecutionOrderStoreError> {
    time::OffsetDateTime::parse(timestamp, &time::format_description::well_known::Rfc3339)
        .map(|_| ())
        .map_err(|error| {
            ExecutionOrderStoreError::Validation(format!("invalid RFC3339 timestamp: {error}"))
        })
}

/// Hash a persisted canonical normalized request for idempotency replay.
/// Older rows that cannot be decoded are deliberately rejected by callers as
/// a different request instead of being replayed on weak evidence.
pub fn normalized_request_hash(order: &StoredExecutionOrder) -> Option<String> {
    if order.normalized_request.trim().is_empty() {
        return None;
    }
    serde_json::from_str::<Value>(&order.normalized_request).ok()?;
    Some(
        Sha256::digest(order.normalized_request.as_bytes())
            .iter()
            .map(|byte| format!("{byte:02x}"))
            .collect(),
    )
}
