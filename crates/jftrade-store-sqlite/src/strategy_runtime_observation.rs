use rusqlite::{OptionalExtension, params};

use crate::strategy_runtime::{StrategyRuntimeStore, StrategyRuntimeStoreError};
use crate::strategy_runtime_records::{RuntimeObservationRow, observation_timestamp};

/// Persisted runtime observation for a strategy instance.
///
/// The observation table is written by the runtime worker and is deliberately
/// kept separate from the catalog operation payload. Production status
/// projections must read this record instead of manufacturing empty values.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct StoredRuntimeObservation {
    pub instance_id: String,
    pub actual_status: String,
    pub active_symbols: Vec<String>,
    pub last_closed_kline_at: Option<String>,
    pub last_signal_at: Option<String>,
    pub last_order_at: Option<String>,
    pub last_error_at: Option<String>,
    pub last_error: Option<String>,
    pub updated_at: Option<String>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct StoredStrategyLogEvent {
    pub raw: String,
    pub level: String,
    pub at_ms: i64,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct StoredStrategyAuditEvent {
    pub instance_id: String,
    pub kind: String,
    pub detail: String,
    pub at_ms: i64,
}

impl StrategyRuntimeStore {
    /// Atomically reserves one daily order slot against the current runtime-risk
    /// revision. The reservation is represented in the durable audit stream so
    /// concurrent workers cannot oversell the daily limit before broker submit.
    pub fn reserve_daily_order(
        &self,
        instance_id: &str,
        expected_revision: i64,
        since_ms: i64,
        daily_max_orders: Option<i64>,
        reservation_key: &str,
        at_ms: i64,
    ) -> Result<(), StrategyRuntimeStoreError> {
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(rusqlite::TransactionBehavior::Immediate)
            .map_err(StrategyRuntimeStoreError::Query)?;
        let instance =
            crate::strategy_runtime_records::get_instance_query(&transaction, instance_id)?
                .ok_or(StrategyRuntimeStoreError::NotFound)?;
        if instance.deleted || instance.runtime_risk_revision != expected_revision {
            return Err(StrategyRuntimeStoreError::Conflict);
        }
        if let Some(limit) = daily_max_orders.filter(|value| *value > 0) {
            let count: i64 = transaction
                .query_row(
                    "SELECT COUNT(*) FROM strategy_audit_events a
                     WHERE a.instance_id = ?1 AND a.at_ms >= ?2
                       AND (a.kind = 'ORDER_SUBMITTED' OR
                            (a.kind = 'ORDER_RESERVED' AND NOT EXISTS (
                                SELECT 1 FROM strategy_audit_events r
                                WHERE r.instance_id = a.instance_id
                                  AND r.kind = 'ORDER_RESERVATION_RELEASED'
                                  AND r.detail = a.detail
                                  AND r.at_ms >= a.at_ms)))",
                    rusqlite::params![instance_id, since_ms],
                    |row| row.get(0),
                )
                .map_err(StrategyRuntimeStoreError::Query)?;
            if count >= limit {
                return Err(StrategyRuntimeStoreError::Conflict);
            }
        }
        transaction
            .execute(
                "INSERT INTO strategy_audit_events (instance_id, kind, detail, at_ms)
                 VALUES (?1, 'ORDER_RESERVED', ?2, ?3)",
                rusqlite::params![instance_id, reservation_key, at_ms],
            )
            .map_err(StrategyRuntimeStoreError::Query)?;
        transaction
            .commit()
            .map_err(StrategyRuntimeStoreError::Query)
    }

    pub fn release_order_reservation(
        &self,
        instance_id: &str,
        reservation_key: &str,
        at_ms: i64,
    ) -> Result<(), StrategyRuntimeStoreError> {
        self.append_audit_event(
            instance_id,
            "ORDER_RESERVATION_RELEASED",
            reservation_key,
            at_ms,
        )
    }

    pub fn get_observation(
        &self,
        instance_id: &str,
    ) -> Result<Option<StoredRuntimeObservation>, StrategyRuntimeStoreError> {
        let connection = self.lock()?;
        let row: Option<RuntimeObservationRow> = connection
            .query_row(
                "SELECT instance_id, actual_status_snapshot, active_symbols_json,
                        last_closed_kline_at_ms, last_signal_at_ms, last_order_at_ms,
                        last_error_at_ms, last_error, updated_at_ms
                 FROM strategy_runtime_observations WHERE instance_id = ?1",
                params![instance_id],
                |row| {
                    Ok((
                        row.get(0)?,
                        row.get(1)?,
                        row.get(2)?,
                        row.get(3)?,
                        row.get(4)?,
                        row.get(5)?,
                        row.get(6)?,
                        row.get(7)?,
                        row.get(8)?,
                    ))
                },
            )
            .optional()
            .map_err(StrategyRuntimeStoreError::Query)?;

        row.map(|(
            instance_id,
            actual_status,
            active_symbols_json,
            last_closed_kline_at_ms,
            last_signal_at_ms,
            last_order_at_ms,
            last_error_at_ms,
            last_error,
            updated_at_ms,
        )| {
            let active_symbols = serde_json::from_str::<Vec<String>>(&active_symbols_json)
                .map_err(|error| {
                    StrategyRuntimeStoreError::Incompatible(format!(
                        "strategy runtime observation {instance_id:?} contains invalid active symbols JSON: {error}"
                    ))
                })?;
            Ok(StoredRuntimeObservation {
                instance_id,
                actual_status,
                active_symbols,
                last_closed_kline_at: observation_timestamp(last_closed_kline_at_ms)?,
                last_signal_at: observation_timestamp(last_signal_at_ms)?,
                last_order_at: observation_timestamp(last_order_at_ms)?,
                last_error_at: observation_timestamp(last_error_at_ms)?,
                last_error: last_error.and_then(|value| {
                    let value = value.trim().to_owned();
                    (!value.is_empty()).then_some(value)
                }),
                updated_at: observation_timestamp(updated_at_ms)?,
            })
        })
        .transpose()
    }

    pub fn list_log_events(
        &self,
        instance_id: &str,
    ) -> Result<Vec<StoredStrategyLogEvent>, StrategyRuntimeStoreError> {
        let connection = self.lock()?;
        let mut statement = connection
            .prepare(
                "SELECT raw, level, at_ms FROM strategy_log_events \
                 WHERE instance_id = ?1 ORDER BY at_ms DESC, id DESC",
            )
            .map_err(StrategyRuntimeStoreError::Query)?;
        let rows = statement
            .query_map([instance_id], |row| {
                Ok(StoredStrategyLogEvent {
                    raw: row.get(0)?,
                    level: row.get(1)?,
                    at_ms: row.get(2)?,
                })
            })
            .map_err(StrategyRuntimeStoreError::Query)?;
        rows.collect::<Result<Vec<_>, _>>()
            .map_err(StrategyRuntimeStoreError::Query)
    }

    pub fn list_audit_events(
        &self,
        instance_id: &str,
    ) -> Result<Vec<StoredStrategyAuditEvent>, StrategyRuntimeStoreError> {
        let connection = self.lock()?;
        let mut statement = connection
            .prepare(
                "SELECT instance_id, kind, detail, at_ms FROM strategy_audit_events \
                 WHERE instance_id = ?1 ORDER BY at_ms DESC, id DESC",
            )
            .map_err(StrategyRuntimeStoreError::Query)?;
        let rows = statement
            .query_map([instance_id], |row| {
                Ok(StoredStrategyAuditEvent {
                    instance_id: row.get(0)?,
                    kind: row.get(1)?,
                    detail: row.get(2)?,
                    at_ms: row.get(3)?,
                })
            })
            .map_err(StrategyRuntimeStoreError::Query)?;
        rows.collect::<Result<Vec<_>, _>>()
            .map_err(StrategyRuntimeStoreError::Query)
    }

    /// Atomically records the worker's latest observation. Runtime workers
    /// use this instead of mutating the catalog payload, keeping status and
    /// liveness projections durable across process restarts.
    pub fn update_observation(
        &self,
        instance_id: &str,
        actual_status: &str,
        active_symbols: &[String],
        last_error: Option<&str>,
        updated_at_ms: i64,
    ) -> Result<(), StrategyRuntimeStoreError> {
        self.update_observation_with_events(
            instance_id,
            actual_status,
            active_symbols,
            last_error,
            None,
            None,
            None,
            updated_at_ms,
        )
    }

    /// Persist a worker observation together with the latest market/signal/order
    /// timestamps. The event timestamps are monotonic projections: a worker
    /// may omit an event on a heartbeat, but it must never erase a timestamp
    /// already recovered from a previous process invocation.
    #[allow(clippy::too_many_arguments)]
    pub fn update_observation_with_events(
        &self,
        instance_id: &str,
        actual_status: &str,
        active_symbols: &[String],
        last_error: Option<&str>,
        last_closed_kline_at_ms: Option<i64>,
        last_signal_at_ms: Option<i64>,
        last_order_at_ms: Option<i64>,
        updated_at_ms: i64,
    ) -> Result<(), StrategyRuntimeStoreError> {
        let connection = self.lock()?;
        let symbols = serde_json::to_string(active_symbols).map_err(|error| {
            StrategyRuntimeStoreError::Incompatible(format!("encode active symbols: {error}"))
        })?;
        connection
            .execute(
                "INSERT INTO strategy_runtime_observations
                    (instance_id, actual_status_snapshot, active_symbols_json,
                     last_closed_kline_at_ms, last_signal_at_ms, last_order_at_ms,
                     last_error, updated_at_ms)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)
                 ON CONFLICT(instance_id) DO UPDATE SET
                    actual_status_snapshot = excluded.actual_status_snapshot,
                    active_symbols_json = excluded.active_symbols_json,
                    last_closed_kline_at_ms = COALESCE(excluded.last_closed_kline_at_ms,
                        strategy_runtime_observations.last_closed_kline_at_ms),
                    last_signal_at_ms = COALESCE(excluded.last_signal_at_ms,
                        strategy_runtime_observations.last_signal_at_ms),
                    last_order_at_ms = COALESCE(excluded.last_order_at_ms,
                        strategy_runtime_observations.last_order_at_ms),
                    last_error = excluded.last_error,
                    last_error_at_ms = CASE WHEN excluded.last_error IS NULL OR excluded.last_error = '' THEN strategy_runtime_observations.last_error_at_ms ELSE excluded.updated_at_ms END,
                    updated_at_ms = excluded.updated_at_ms",
                params![
                    instance_id,
                    actual_status,
                    symbols,
                    last_closed_kline_at_ms,
                    last_signal_at_ms,
                    last_order_at_ms,
                    last_error.unwrap_or_default(),
                    updated_at_ms,
                ],
            )
            .map_err(StrategyRuntimeStoreError::Query)?;
        Ok(())
    }

    /// Append a worker diagnostic to the durable strategy activity stream.
    /// Callers are expected to pass a validated instance id; SQLite foreign
    /// key enforcement remains the source of truth for malformed ids.
    pub fn append_log_event(
        &self,
        instance_id: &str,
        raw: &str,
        level: &str,
        at_ms: i64,
    ) -> Result<(), StrategyRuntimeStoreError> {
        if instance_id.trim().is_empty() || raw.trim().is_empty() {
            return Err(StrategyRuntimeStoreError::Validation(
                "strategy log instance and message are required".to_owned(),
            ));
        }
        let connection = self.lock()?;
        connection
            .execute(
                "INSERT INTO strategy_log_events (instance_id, at_ms, raw, level, source)
                 VALUES (?1, ?2, ?3, ?4, 'rust-production-runtime')",
                params![instance_id, at_ms, raw, level],
            )
            .map_err(StrategyRuntimeStoreError::Query)?;
        Ok(())
    }

    /// Append a state transition/audit diagnostic using the same durable
    /// stream read by the strategy activity endpoints.
    pub fn append_audit_event(
        &self,
        instance_id: &str,
        kind: &str,
        detail: &str,
        at_ms: i64,
    ) -> Result<(), StrategyRuntimeStoreError> {
        if instance_id.trim().is_empty() || kind.trim().is_empty() {
            return Err(StrategyRuntimeStoreError::Validation(
                "strategy audit instance and kind are required".to_owned(),
            ));
        }
        let connection = self.lock()?;
        connection
            .execute(
                "INSERT INTO strategy_audit_events (instance_id, kind, detail, at_ms)
                 VALUES (?1, ?2, ?3, ?4)",
                params![instance_id, kind, detail, at_ms],
            )
            .map_err(StrategyRuntimeStoreError::Query)?;
        Ok(())
    }

    pub fn count_daily_orders(
        &self,
        instance_id: &str,
        since_ms: i64,
    ) -> Result<i64, StrategyRuntimeStoreError> {
        let connection = self.lock()?;
        let count: i64 = connection
            .query_row(
                "SELECT COUNT(*) FROM strategy_audit_events
                 WHERE instance_id = ?1 AND kind = 'ORDER_SUBMITTED' AND at_ms >= ?2",
                params![instance_id, since_ms],
                |row| row.get(0),
            )
            .map_err(StrategyRuntimeStoreError::Query)?;
        Ok(count)
    }
}
