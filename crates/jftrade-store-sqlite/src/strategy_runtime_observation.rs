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
                                  AND r.at_ms >= a.at_ms
                                  AND (
                                       (r.kind = 'ORDER_RESERVATION_RELEASED' AND r.detail = a.detail) OR
                                       (r.kind = 'ORDER_SUBMITTED' AND a.detail != '' AND (r.detail = a.detail OR r.detail LIKE '%reservation: ' || a.detail || ')%'))
                                  ))))",
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

#[cfg(test)]
mod tests {
    use crate::strategy_definition::{
        STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE, StrategyDefinitionStore,
    };
    use crate::strategy_runtime::{StrategyRuntimeStore, StrategyRuntimeStoreError};
    use std::sync::Arc;

    #[test]
    fn test_reserve_daily_order_no_double_counting_and_safe_reclaim() {
        let directory = tempfile::tempdir().expect("tempdir");
        let path = directory.path().join("strategy.db");
        let conn = rusqlite::Connection::open(&path).expect("open test db");
        crate::initialize_current(&conn, "strategy").expect("initialize strategy schema");
        drop(conn);

        let def_store = Arc::new(
            StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
                .expect("open def store"),
        );
        let store = StrategyRuntimeStore::from_definition_store(&def_store);
        store
            .seed_instance("inst-quota", "RUNNING", "2026-08-30T00:00:00Z")
            .expect("seed instance");

        let limit = Some(2);
        let since_ms = 1000;

        // 1. First reservation succeeds
        store
            .reserve_daily_order("inst-quota", 0, since_ms, limit, "res-1", 1010)
            .expect("first reservation must succeed");

        // Second reservation in-flight also succeeds (now count = 2)
        store
            .reserve_daily_order("inst-quota", 0, since_ms, limit, "res-2", 1020)
            .expect("second reservation must succeed");

        // Third reservation in-flight must be rejected due to limit 2 reached
        let err = store
            .reserve_daily_order("inst-quota", 0, since_ms, limit, "res-3", 1030)
            .expect_err("third reservation must fail");
        assert!(matches!(
            err,
            crate::strategy_runtime::StrategyRuntimeStoreError::Conflict
        ));

        // 2. Submit the first reservation: 1 reserved + 1 submitted = 1 quota consumed, not 2
        store
            .append_audit_event(
                "inst-quota",
                "ORDER_SUBMITTED",
                "US.AAPL BUY 10 (clientOrderId: client-1; reservation: res-1)",
                1040,
            )
            .expect("append submit event");

        // Still count = 2 (1 submitted from res-1 + 1 in-flight res-2)
        let err = store
            .reserve_daily_order("inst-quota", 0, since_ms, limit, "res-3", 1050)
            .expect_err("still at limit of 2");
        assert!(matches!(
            err,
            crate::strategy_runtime::StrategyRuntimeStoreError::Conflict
        ));

        // 3. Safe quota reclaim: broker had no record or order failed -> release res-2 reservation
        store
            .release_order_reservation("inst-quota", "res-2", 1060)
            .expect("release reservation");

        // Now count is only 1 (the submitted res-1). Reserving res-3 must succeed!
        store
            .reserve_daily_order("inst-quota", 0, since_ms, limit, "res-3", 1070)
            .expect("reservation after safe reclaim must succeed");

        // Submit res-3: count is now 2 (res-1 submitted + res-3 submitted)
        store
            .append_audit_event(
                "inst-quota",
                "ORDER_SUBMITTED",
                "US.TSLA BUY 5 (clientOrderId: client-3; reservation: res-3)",
                1080,
            )
            .expect("append submit event");

        // Verifying count_daily_orders reports 2 submitted orders
        assert_eq!(store.count_daily_orders("inst-quota", since_ms).unwrap(), 2);
    }

    #[test]
    fn test_stress_quota_reservation_substring_collision_counter_evidence() {
        let directory = tempfile::tempdir().expect("tempdir");
        let path = directory.path().join("strategy.db");
        let conn = rusqlite::Connection::open(&path).expect("open test db");
        crate::initialize_current(&conn, "strategy").expect("initialize strategy schema");
        drop(conn);

        let def_store = Arc::new(
            StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
                .expect("open def store"),
        );
        let store = StrategyRuntimeStore::from_definition_store(&def_store);
        store
            .seed_instance("inst-collision", "RUNNING", "2026-08-30T00:00:00Z")
            .expect("seed instance");

        let limit = Some(2);
        let since_ms = 1000;

        // Realistic Pine reservation keys: {instance}:{symbol}:{bar}:{index}
        let key_1 = "inst-collision:US.AAPL:1:1";
        let key_2 = "inst-collision:US.AAPL:1:10"; // contains key_1 as substring!

        // Order 1 reserves key_1 at t=1010
        store
            .reserve_daily_order("inst-collision", 0, since_ms, limit, key_1, 1010)
            .expect("key_1 reservation must succeed");

        // Order 2 reserves key_2 at t=1020
        store
            .reserve_daily_order("inst-collision", 0, since_ms, limit, key_2, 1020)
            .expect("key_2 reservation must succeed");

        // Order 2 submits at t=1030 with detail embedding "reservation: key_2"
        store
            .append_audit_event(
                "inst-collision",
                "ORDER_SUBMITTED",
                &format!("US.AAPL BUY 10 (clientOrderId: client-10; reservation: {key_2})"),
                1030,
            )
            .expect("append submit event");

        // Order 1 is STILL IN FLIGHT (reserved, not submitted, not released).
        // With limit = 2, Order 1 is in-flight (1) and Order 2 is submitted (1), consuming all 2 quota slots.
        // Reserving a 3rd order (key_3) must be REJECTED with Err(StrategyRuntimeStoreError::Conflict).
        // With the anchored delimiter pattern (r.detail = a.detail OR r.detail LIKE '%reservation: ' || a.detail || ')%'),
        // key_1's active reservation is NOT prematurely dropped when key_2 (:1:10) is submitted.
        let key_3 = "inst-collision:US.AAPL:1:2";
        let result = store.reserve_daily_order("inst-collision", 0, since_ms, limit, key_3, 1040);

        assert!(
            matches!(result, Err(StrategyRuntimeStoreError::Conflict)),
            "reserving a 3rd order must be REJECTED with Err(StrategyRuntimeStoreError::Conflict) when :1:1 is in-flight and :1:10 is submitted under limit = 2, got: {:?}",
            result
        );

        // Now Order 1 finishes in-flight submission:
        store
            .append_audit_event(
                "inst-collision",
                "ORDER_SUBMITTED",
                &format!("US.AAPL BUY 10 (clientOrderId: client-1; reservation: {key_1})"),
                1050,
            )
            .expect("append submit event for key_1");

        // The daily quota was limit=2, and exactly 2 submitted orders exist in the database!
        assert_eq!(
            store
                .count_daily_orders("inst-collision", since_ms)
                .unwrap(),
            2
        );

        // Subsequent reservation after both orders submitted is also rejected with Conflict
        let post_result =
            store.reserve_daily_order("inst-collision", 0, since_ms, limit, key_3, 1060);
        assert!(
            matches!(post_result, Err(StrategyRuntimeStoreError::Conflict)),
            "reserving after both orders submit must still be rejected with Conflict"
        );
    }

    #[test]
    fn test_stress_concurrent_quota_reservations() {
        let directory = tempfile::tempdir().expect("tempdir");
        let path = directory.path().join("strategy.db");
        let conn = rusqlite::Connection::open(&path).expect("open test db");
        crate::initialize_current(&conn, "strategy").expect("initialize strategy schema");
        drop(conn);

        let def_store = Arc::new(
            StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
                .expect("open def store"),
        );
        let store = Arc::new(StrategyRuntimeStore::from_definition_store(&def_store));
        store
            .seed_instance("inst-concurrent", "RUNNING", "2026-08-30T00:00:00Z")
            .expect("seed instance");

        let limit = Some(3);
        let since_ms = 1000;
        let mut handles = Vec::new();

        for i in 0..10 {
            let store_clone = Arc::clone(&store);
            handles.push(std::thread::spawn(move || {
                store_clone.reserve_daily_order(
                    "inst-concurrent",
                    0,
                    since_ms,
                    limit,
                    &format!("concurrent-res-{i}"),
                    1010 + i as i64,
                )
            }));
        }

        let mut successes = 0;
        let mut conflicts = 0;
        for h in handles {
            match h.join().unwrap() {
                Ok(()) => successes += 1,
                Err(crate::strategy_runtime::StrategyRuntimeStoreError::Conflict) => conflicts += 1,
                Err(other) => panic!("Unexpected error: {:?}", other),
            }
        }
        println!("Concurrent reservation results: successes={successes}, conflicts={conflicts}");
        assert_eq!(successes, 3);
        assert_eq!(conflicts, 7);
    }

    #[test]
    fn test_stress_quota_lifecycle_exact_counts() {
        let directory = tempfile::tempdir().expect("tempdir");
        let path = directory.path().join("strategy.db");
        let conn = rusqlite::Connection::open(&path).expect("open test db");
        crate::initialize_current(&conn, "strategy").expect("initialize strategy schema");
        drop(conn);

        let def_store = Arc::new(
            StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
                .expect("open def store"),
        );
        let store = StrategyRuntimeStore::from_definition_store(&def_store);
        store
            .seed_instance("inst-stress", "RUNNING", "2026-08-30T00:00:00Z")
            .expect("seed instance");

        let limit = Some(5);
        let since_ms = 1000;

        // Stage 1: Reserve 5 orders
        for i in 1..=5 {
            store
                .reserve_daily_order(
                    "inst-stress",
                    0,
                    since_ms,
                    limit,
                    &format!("order-res-{i}"),
                    1000 + i as i64 * 10,
                )
                .unwrap_or_else(|_| panic!("Reservation {i} must succeed"));
        }

        // Stage 2: 6th reservation must fail with Conflict (all 5 slots reserved in-flight)
        let err = store
            .reserve_daily_order("inst-stress", 0, since_ms, limit, "order-res-6", 1060)
            .expect_err("6th reservation must fail at quota limit 5");
        assert!(matches!(
            err,
            crate::strategy_runtime::StrategyRuntimeStoreError::Conflict
        ));
        assert_eq!(
            store.count_daily_orders("inst-stress", since_ms).unwrap(),
            0
        );

        // Stage 3: Submit order 1 and 2. Slots remain 5 (2 submitted + 3 reserved).
        store
            .append_audit_event(
                "inst-stress",
                "ORDER_SUBMITTED",
                "US.AAPL BUY 10 (clientOrderId: c-1; reservation: order-res-1)",
                1070,
            )
            .unwrap();
        store
            .append_audit_event(
                "inst-stress",
                "ORDER_SUBMITTED",
                "US.AAPL BUY 10 (clientOrderId: c-2; reservation: order-res-2)",
                1080,
            )
            .unwrap();
        assert_eq!(
            store.count_daily_orders("inst-stress", since_ms).unwrap(),
            2
        );
        // Reservation must still fail (5 slots full)
        let err = store
            .reserve_daily_order("inst-stress", 0, since_ms, limit, "order-res-6", 1085)
            .expect_err("still at limit 5");
        assert!(matches!(
            err,
            crate::strategy_runtime::StrategyRuntimeStoreError::Conflict
        ));

        // Stage 4: Release order 3. Count drops to 4 (2 submitted + 2 in-flight reserved).
        store
            .release_order_reservation("inst-stress", "order-res-3", 1090)
            .unwrap();

        // Stage 5: Reserve order 6. Succeeds! Now 5 slots occupied (2 submitted + 3 reserved).
        store
            .reserve_daily_order("inst-stress", 0, since_ms, limit, "order-res-6", 1100)
            .unwrap();

        // Stage 6: Attempt order 7. Fails with Conflict.
        let err = store
            .reserve_daily_order("inst-stress", 0, since_ms, limit, "order-res-7", 1110)
            .expect_err("limit 5 reached");
        assert!(matches!(
            err,
            crate::strategy_runtime::StrategyRuntimeStoreError::Conflict
        ));

        // Stage 7: Release order 4. Count drops to 4 (2 submitted + 2 in-flight).
        store
            .release_order_reservation("inst-stress", "order-res-4", 1120)
            .unwrap();

        // Stage 8: Submit order 5. Count remains 4 (3 submitted + 1 in-flight order-6).
        store
            .append_audit_event(
                "inst-stress",
                "ORDER_SUBMITTED",
                "US.AAPL BUY 10 (clientOrderId: c-5; reservation: order-res-5)",
                1130,
            )
            .unwrap();
        assert_eq!(
            store.count_daily_orders("inst-stress", since_ms).unwrap(),
            3
        );

        // Stage 9: Submit order 6. Count remains 4 (4 submitted + 0 in-flight).
        store
            .append_audit_event(
                "inst-stress",
                "ORDER_SUBMITTED",
                "US.AAPL BUY 10 (clientOrderId: c-6; reservation: order-res-6)",
                1140,
            )
            .unwrap();
        assert_eq!(
            store.count_daily_orders("inst-stress", since_ms).unwrap(),
            4
        );

        // Stage 10: Reserve order 7. Succeeds! Count = 5 (4 submitted + 1 in-flight).
        store
            .reserve_daily_order("inst-stress", 0, since_ms, limit, "order-res-7", 1150)
            .unwrap();

        // Stage 11: Release order 7. Count drops back to 4.
        store
            .release_order_reservation("inst-stress", "order-res-7", 1160)
            .unwrap();

        // Stage 12: Reserve order 8. Count = 5 (4 submitted + 1 in-flight).
        store
            .reserve_daily_order("inst-stress", 0, since_ms, limit, "order-res-8", 1170)
            .unwrap();

        // Stage 13: Submit order 8. Count = 5 (5 submitted + 0 in-flight).
        store
            .append_audit_event(
                "inst-stress",
                "ORDER_SUBMITTED",
                "US.AAPL BUY 10 (clientOrderId: c-8; reservation: order-res-8)",
                1180,
            )
            .unwrap();
        assert_eq!(
            store.count_daily_orders("inst-stress", since_ms).unwrap(),
            5
        );

        // Stage 14: All 5 slots used by 5 submitted orders. Attempt order 9 must fail!
        let err = store
            .reserve_daily_order("inst-stress", 0, since_ms, limit, "order-res-9", 1190)
            .expect_err("daily quota limit exhausted");
        assert!(matches!(
            err,
            crate::strategy_runtime::StrategyRuntimeStoreError::Conflict
        ));
        assert_eq!(
            store.count_daily_orders("inst-stress", since_ms).unwrap(),
            5
        );
    }
}
