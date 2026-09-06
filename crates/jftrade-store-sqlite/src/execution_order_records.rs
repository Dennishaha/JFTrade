use rusqlite::params;
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

use super::{ExecutionOrderStoreError, StoredExecutionOrder, StoredExecutionOrderEvent};

pub(super) fn upsert_order(
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

pub(super) fn read_order(row: &rusqlite::Row<'_>) -> rusqlite::Result<StoredExecutionOrder> {
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

pub(super) fn query_active_orders_for_instance(
    connection: &rusqlite::Connection,
    instance_id: &str,
) -> Result<Vec<StoredExecutionOrder>, ExecutionOrderStoreError> {
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
             FROM execution_orders
             WHERE source = 'strategy-runtime'
               AND source_detail = ?1
               AND status IN ('SUBMITTING', 'SUBMITTED', 'WAITING', 'OPEN')
             ORDER BY created_at DESC",
        )
        .map_err(ExecutionOrderStoreError::Query)?;
    let rows = statement
        .query_map(params![instance_id], read_order)
        .map_err(ExecutionOrderStoreError::Query)?;
    rows.map(|row| row.map_err(ExecutionOrderStoreError::Query))
        .collect()
}

pub(super) fn insert_event(
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

pub(super) fn validate_rfc3339_timestamp(timestamp: &str) -> Result<(), ExecutionOrderStoreError> {
    OffsetDateTime::parse(timestamp, &Rfc3339)
        .map(|_| ())
        .map_err(|error| {
            ExecutionOrderStoreError::Incompatible(format!(
                "invalid RFC3339 timestamp {timestamp:?}: {error}"
            ))
        })
}
