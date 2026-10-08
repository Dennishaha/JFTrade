use std::path::Path;

use jftrade_owner_lock::WriterLeaseError;
use jftrade_store_sqlite::{
    EXECUTION_ORDERS_TEST_CUTOVER_PROFILE, ExecutionOrderReservation, ExecutionOrderStore,
    ExecutionOrderStoreError, ExecutionOrderTestCutoverStore, StoredExecutionOrder,
    StoredExecutionOrderEvent, StoredExecutionOrderPreview,
};
use rusqlite::Connection;

const TIMESTAMP_1: &str = "2026-08-22T06:00:00Z";
const TIMESTAMP_2: &str = "2026-08-22T06:00:01Z";

#[test]
fn execution_orders_store_rejects_missing_drifted_and_corrupted_go_databases() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let missing_path = directory.path().join("missing-orders.db");
    assert!(matches!(
        ExecutionOrderTestCutoverStore::open_existing(
            &missing_path,
            EXECUTION_ORDERS_TEST_CUTOVER_PROFILE
        ),
        Err(ExecutionOrderStoreError::NotRegularFile(_))
    ));

    let drifted_path = directory.path().join("drifted-orders.db");
    let connection = Connection::open(&drifted_path).expect("create drifted db");
    connection
        .execute_batch(
            "CREATE TABLE jftrade_schema_meta (
                component_id TEXT PRIMARY KEY,
                version INTEGER NOT NULL,
                created_at TEXT NOT NULL
            );
            INSERT INTO jftrade_schema_meta (component_id, version, created_at)
                VALUES ('execution-orders', 5, '2026-08-22T06:00:00Z');
            CREATE TABLE execution_orders (
                internal_order_id TEXT PRIMARY KEY,
                rogue_column TEXT NOT NULL
            );",
        )
        .expect("seed rogue table");
    drop(connection);

    let error = ExecutionOrderTestCutoverStore::open_existing(
        &drifted_path,
        EXECUTION_ORDERS_TEST_CUTOVER_PROFILE,
    )
    .expect_err("drifted schema must fail");
    assert!(matches!(error, ExecutionOrderStoreError::Schema(_)));
}

// Parity: go:452dea11:internal/app/apiserver/tradingapp/order_updates_test.go:114 TestExecutionOrderUpdatesPersistBrokerLifecycleFields
#[test]
fn execution_orders_lifecycle_events_and_restart_durability() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("orders.db");
    seed_go_execution_orders_schema(&path);

    let store = open_store(&path);
    assert_eq!(store.path(), path);

    let conflict =
        ExecutionOrderTestCutoverStore::open_existing(&path, EXECUTION_ORDERS_TEST_CUTOVER_PROFILE)
            .expect_err("second writer must fail");
    assert!(matches!(
        conflict,
        ExecutionOrderStoreError::WriterLease(WriterLeaseError::Held { .. })
    ));

    let order = StoredExecutionOrder {
        internal_order_id: "ord-1".to_owned(),
        broker_id: "futu".to_owned(),
        broker_order_id: Some("1001".to_owned()),
        broker_order_id_ex: None,
        source: "strategy".to_owned(),
        source_detail: "momentum".to_owned(),
        trading_environment: "simulated".to_owned(),
        account_id: "acc-1".to_owned(),
        market: "US".to_owned(),
        symbol: Some("AAPL".to_owned()),
        side: Some("BUY".to_owned()),
        order_type: Some("LIMIT".to_owned()),
        status: "SUBMITTED".to_owned(),
        raw_broker_status: None,
        requested_quantity: Some(10.0),
        requested_price: Some(150.0),
        filled_quantity: Some(0.0),
        filled_average_price: Some(0.0),
        remark: None,
        last_error: None,
        last_error_code: None,
        last_error_source: None,
        submitted_at: Some(TIMESTAMP_1.to_owned()),
        updated_at: TIMESTAMP_1.to_owned(),
        created_at: TIMESTAMP_1.to_owned(),
        order_kind: "single".to_owned(),
        product_class: "stock".to_owned(),
        quantity_mode: "units".to_owned(),
        client_order_id: Some("cli-1".to_owned()),
        preview_id: None,
        normalized_request: "{}".to_owned(),
        requested_amount: Some(1500.0),
        payout: None,
        fees: Some(1.0),
    };

    store.save_order(order, TIMESTAMP_1).expect("save order");
    assert_eq!(store.order_count().expect("order count"), 1);

    let retrieved = store
        .get_order("ord-1")
        .expect("get order")
        .expect("must exist");
    assert_eq!(retrieved.internal_order_id, "ord-1");
    assert_eq!(retrieved.status, "SUBMITTED");

    store
        .record_event(&StoredExecutionOrderEvent {
            id: "evt-1",
            internal_order_id: "ord-1",
            event_type: "PLACE",
            previous_status: None,
            next_status: "SUBMITTED",
            payload_json: "{}",
            created_at: TIMESTAMP_1,
        })
        .expect("record event");
    assert_eq!(store.event_count("PLACE").expect("place count"), 1);

    let mut fenced = retrieved.clone();
    fenced.status = "CANCEL_SUBMITTED".to_owned();
    fenced.updated_at = TIMESTAMP_2.to_owned();
    let transition = StoredExecutionOrderEvent {
        id: "evt-2",
        internal_order_id: "ord-1",
        event_type: "cancel_submitted",
        previous_status: Some("SUBMITTED"),
        next_status: "CANCEL_SUBMITTED",
        payload_json: "{}",
        created_at: TIMESTAMP_2,
    };
    store
        .transition_order_and_event_fenced(
            fenced.clone(),
            TIMESTAMP_2,
            &transition,
            "SUBMITTED",
            TIMESTAMP_1,
            Some(1),
        )
        .expect("revision-fenced transition");
    assert_eq!(store.order_revision("ord-1").expect("revision"), 2);
    let stale = store
        .transition_order_and_event_fenced(
            StoredExecutionOrder {
                status: "UNKNOWN".to_owned(),
                updated_at: TIMESTAMP_2.to_owned(),
                ..fenced
            },
            TIMESTAMP_2,
            &StoredExecutionOrderEvent {
                id: "evt-stale",
                internal_order_id: "ord-1",
                event_type: "late",
                previous_status: Some("CANCEL_SUBMITTED"),
                next_status: "UNKNOWN",
                payload_json: "{}",
                created_at: TIMESTAMP_2,
            },
            "CANCEL_SUBMITTED",
            TIMESTAMP_2,
            Some(1),
        )
        .expect_err("stale revision must be rejected");
    assert!(matches!(stale, ExecutionOrderStoreError::Conflict(_)));

    let seq1 = store.next_sequence("order").expect("next seq 1");
    let seq2 = store.next_sequence("order").expect("next seq 2");
    assert_eq!(seq1, 1);
    assert_eq!(seq2, 2);

    drop(store);

    let reopened = open_store(&path);
    assert_eq!(reopened.order_count().expect("reopened order count"), 1);
    assert_eq!(
        reopened.event_count("PLACE").expect("reopened event count"),
        1
    );
    let seq3 = reopened.next_sequence("order").expect("next seq 3");
    assert_eq!(seq3, 3);
}

#[test]
fn execution_order_reservation_replays_identity_without_retrying_unknown_submission() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("orders.db");
    seed_go_execution_orders_schema(&path);
    let store = open_store(&path);

    let order = StoredExecutionOrder {
        internal_order_id: "reserve-1".to_owned(),
        broker_id: "futu".to_owned(),
        broker_order_id: None,
        broker_order_id_ex: None,
        source: "system".to_owned(),
        source_detail: "test".to_owned(),
        trading_environment: "REAL".to_owned(),
        account_id: "acc-1".to_owned(),
        market: "US".to_owned(),
        symbol: Some("AAPL".to_owned()),
        side: Some("BUY".to_owned()),
        order_type: Some("LIMIT".to_owned()),
        status: "SUBMITTING".to_owned(),
        raw_broker_status: None,
        requested_quantity: Some(1.0),
        requested_price: Some(100.0),
        filled_quantity: Some(0.0),
        filled_average_price: Some(0.0),
        remark: None,
        last_error: None,
        last_error_code: None,
        last_error_source: None,
        submitted_at: None,
        updated_at: TIMESTAMP_1.to_owned(),
        created_at: TIMESTAMP_1.to_owned(),
        order_kind: "single".to_owned(),
        product_class: "stock".to_owned(),
        quantity_mode: "units".to_owned(),
        client_order_id: Some("client-stable-1".to_owned()),
        preview_id: None,
        normalized_request: r#"{"symbol":"US.AAPL","side":"BUY"}"#.to_owned(),
        requested_amount: None,
        payout: None,
        fees: None,
    };

    assert!(matches!(
        store
            .reserve_order_with_preview(order.clone(), "request-hash", TIMESTAMP_1)
            .expect("first reservation"),
        ExecutionOrderReservation::Reserved(_)
    ));
    let mut replay = order.clone();
    replay.internal_order_id = "reserve-2".to_owned();
    assert!(matches!(
        store
            .reserve_order_with_preview(replay, "request-hash", TIMESTAMP_2)
            .expect("duplicate reservation"),
        ExecutionOrderReservation::Existing(existing) if existing.internal_order_id == "reserve-1"
    ));
    assert_eq!(store.order_count().expect("order count"), 1);
    assert_eq!(
        store
            .find_order_by_client_identity("FUTU", "real", "acc-1", "CLIENT-STABLE-1")
            .expect("find client identity")
            .expect("reserved order")
            .internal_order_id,
        "reserve-1"
    );

    let mut unknown = order.clone();
    unknown.status = "SUBMISSION_UNKNOWN".to_owned();
    unknown.last_error = Some("broker timeout".to_owned());
    unknown.updated_at = TIMESTAMP_2.to_owned();
    store
        .save_order(unknown, TIMESTAMP_2)
        .expect("mark unknown");

    let candidates = store
        .list_reconciliation_candidates()
        .expect("reconciliation candidates");
    assert_eq!(candidates.len(), 1);
    assert_eq!(candidates[0].status, "SUBMISSION_UNKNOWN");
    let mut retry = order;
    retry.internal_order_id = "reserve-retry".to_owned();
    assert!(matches!(
        store
            .reserve_order_with_preview(retry, "request-hash", TIMESTAMP_2)
            .expect("unknown replay fence"),
        ExecutionOrderReservation::Existing(existing)
            if existing.internal_order_id == "reserve-1"
    ));
    assert_eq!(store.order_count().expect("order count after replay"), 1);
}

#[test]
fn execution_order_preview_consumption_is_idempotent_and_expiry_fenced() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("orders.db");
    seed_go_execution_orders_schema(&path);
    let store = ExecutionOrderStore::open_existing(&path, EXECUTION_ORDERS_TEST_CUTOVER_PROFILE)
        .expect("open execution orders store");

    let preview = StoredExecutionOrderPreview {
        preview_id: "preview-1".to_owned(),
        request_hash: "request-hash".to_owned(),
        broker_id: "futu".to_owned(),
        capability_version: "futu-v1".to_owned(),
        account_id: "acc-1".to_owned(),
        expires_at: "2026-08-22T07:00:00Z".to_owned(),
        quote_expires_at: None,
        rfq_id: None,
        normalized_request: r#"{"symbol":"US.AAPL"}"#.to_owned(),
        created_at: TIMESTAMP_1.to_owned(),
        consumed_at: None,
    };
    store.save_preview(&preview).expect("save preview");
    store
        .consume_preview("preview-1", "FUTU", "acc-1", "request-hash", TIMESTAMP_1)
        .expect("first consume");
    store
        .consume_preview("preview-1", "futu", "acc-1", "request-hash", TIMESTAMP_2)
        .expect("identical replay consume");
    let changed = store
        .consume_preview("preview-1", "futu", "acc-1", "changed-hash", TIMESTAMP_2)
        .expect_err("changed request must be rejected");
    assert!(
        matches!(changed, ExecutionOrderStoreError::Validation(message) if message.contains("does not match"))
    );

    let mut expired = preview.clone();
    expired.preview_id = "preview-expired".to_owned();
    expired.expires_at = TIMESTAMP_1.to_owned();
    store.save_preview(&expired).expect("save expired preview");
    let expired_error = store
        .consume_preview(
            "preview-expired",
            "futu",
            "acc-1",
            "request-hash",
            TIMESTAMP_2,
        )
        .expect_err("expired preview must be rejected");
    assert!(
        matches!(expired_error, ExecutionOrderStoreError::Validation(message) if message.contains("expired"))
    );

    let mut quote_expired = preview;
    quote_expired.preview_id = "preview-quote-expired".to_owned();
    quote_expired.quote_expires_at = Some(TIMESTAMP_1.to_owned());
    store
        .save_preview(&quote_expired)
        .expect("save quote-expired preview");
    let quote_error = store
        .consume_preview(
            "preview-quote-expired",
            "futu",
            "acc-1",
            "request-hash",
            TIMESTAMP_2,
        )
        .expect_err("expired broker quote must be rejected");
    assert!(
        matches!(quote_error, ExecutionOrderStoreError::Validation(message) if message.contains("broker quote expired"))
    );
}

fn open_store(path: &Path) -> ExecutionOrderTestCutoverStore {
    ExecutionOrderTestCutoverStore::open_existing(path, EXECUTION_ORDERS_TEST_CUTOVER_PROFILE)
        .expect("open execution orders test-cutover store")
}

fn seed_go_execution_orders_schema(path: &Path) {
    let connection = Connection::open(path).expect("create execution orders fixture");
    connection
        .execute_batch(
            "CREATE TABLE execution_orders (
                internal_order_id TEXT PRIMARY KEY,
                broker_id TEXT NOT NULL DEFAULT '',
                broker_order_id TEXT,
                broker_order_id_ex TEXT,
                source TEXT NOT NULL DEFAULT '',
                source_detail TEXT NOT NULL DEFAULT '',
                trading_environment TEXT NOT NULL DEFAULT '',
                account_id TEXT NOT NULL DEFAULT '',
                market TEXT NOT NULL DEFAULT '',
                symbol TEXT,
                side TEXT,
                order_type TEXT,
                status TEXT NOT NULL DEFAULT '',
                requested_quantity REAL,
                requested_price REAL,
                filled_quantity REAL,
                filled_average_price REAL,
                remark TEXT,
                last_error TEXT,
                last_error_code TEXT,
                last_error_source TEXT,
                submitted_at TEXT,
                updated_at TEXT NOT NULL DEFAULT '',
                created_at TEXT NOT NULL DEFAULT '',
                raw_broker_status TEXT,
                order_kind TEXT NOT NULL DEFAULT 'single',
                product_class TEXT NOT NULL DEFAULT 'unknown',
                quantity_mode TEXT NOT NULL DEFAULT 'units',
                client_order_id TEXT,
                preview_id TEXT,
                normalized_request TEXT NOT NULL DEFAULT '{}',
                requested_amount REAL,
                payout REAL,
                fees REAL
            );
            CREATE TABLE execution_order_legs (
                id TEXT PRIMARY KEY,
                internal_order_id TEXT NOT NULL,
                leg_index INTEGER NOT NULL,
                broker_leg_id TEXT,
                instrument_id TEXT NOT NULL,
                product_class TEXT NOT NULL DEFAULT 'unknown',
                side TEXT NOT NULL DEFAULT '',
                ratio INTEGER NOT NULL DEFAULT 1,
                prediction_side TEXT NOT NULL DEFAULT '',
                requested_quantity REAL,
                requested_amount REAL,
                requested_price REAL,
                status TEXT NOT NULL DEFAULT '',
                filled_quantity REAL,
                filled_amount REAL,
                average_price REAL,
                fees REAL,
                payout REAL,
                updated_at TEXT NOT NULL DEFAULT '',
                created_at TEXT NOT NULL DEFAULT ''
            );
            CREATE TABLE execution_order_previews (
                preview_id TEXT PRIMARY KEY,
                request_hash TEXT NOT NULL,
                broker_id TEXT NOT NULL,
                capability_version TEXT NOT NULL,
                account_id TEXT NOT NULL,
                expires_at TEXT NOT NULL,
                quote_expires_at TEXT,
                rfq_id TEXT,
                normalized_request TEXT NOT NULL,
                created_at TEXT NOT NULL,
                consumed_at TEXT
            );
            CREATE TABLE execution_prediction_quotes (
                quote_id TEXT PRIMARY KEY,
                broker_id TEXT NOT NULL,
                account_id TEXT NOT NULL,
                trading_environment TEXT NOT NULL,
                mvc TEXT NOT NULL,
                legs_hash TEXT NOT NULL,
                bid_price REAL,
                ask_price REAL,
                should_retry INTEGER NOT NULL DEFAULT 0,
                received_at TEXT NOT NULL,
                expires_at TEXT NOT NULL,
                expiry_source TEXT NOT NULL DEFAULT 'jftrade_policy',
                status TEXT NOT NULL DEFAULT 'active',
                consumed_at TEXT,
                consumed_preview_id TEXT,
                consumed_client_order_id TEXT
            );
            CREATE TABLE execution_order_events (
                id TEXT PRIMARY KEY,
                internal_order_id TEXT NOT NULL,
                event_type TEXT NOT NULL DEFAULT '',
                previous_status TEXT,
                next_status TEXT NOT NULL DEFAULT '',
                payload_json TEXT NOT NULL DEFAULT '{}',
                created_at TEXT NOT NULL DEFAULT ''
            );
            CREATE TABLE execution_seen_fills (fill_key TEXT PRIMARY KEY, created_at TEXT NOT NULL DEFAULT '');
            CREATE TABLE execution_sequences (name TEXT PRIMARY KEY, value INTEGER NOT NULL DEFAULT 0);
            CREATE INDEX idx_execution_orders_updated ON execution_orders (updated_at DESC, created_at DESC, internal_order_id DESC);
            CREATE INDEX idx_execution_orders_broker_order ON execution_orders (broker_id, trading_environment, account_id, market, broker_order_id);
            CREATE INDEX idx_execution_orders_broker_order_ex ON execution_orders (broker_id, trading_environment, account_id, market, broker_order_id_ex);
            CREATE INDEX idx_execution_order_events_order ON execution_order_events (internal_order_id, created_at ASC, id ASC);
            CREATE UNIQUE INDEX idx_execution_orders_client_id ON execution_orders (broker_id, trading_environment, account_id, client_order_id) WHERE client_order_id IS NOT NULL AND TRIM(client_order_id) <> '';
            CREATE INDEX idx_execution_order_legs_order ON execution_order_legs (internal_order_id, leg_index ASC);
            CREATE INDEX idx_execution_prediction_quotes_expiry ON execution_prediction_quotes (status, expires_at);
            CREATE TABLE jftrade_schema_meta (
                component_id TEXT PRIMARY KEY,
                version INTEGER NOT NULL,
                created_at TEXT NOT NULL
            );
            INSERT INTO jftrade_schema_meta (component_id, version, created_at)
                VALUES ('execution-orders', 5, '2026-08-22T06:00:00Z');",
        )
        .expect("seed Go-compatible execution-orders schema");
}

// Parity: go:452dea11:internal/store/trading/persistence_query_plan_test.go:12 TestExecutionEventLoadUsesOrderIndexAndPreservesPerOrderChronology
#[test]
fn execution_order_events_load_in_per_order_chronology_without_a_temp_sort() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("events.db");
    seed_go_execution_orders_schema(&path);
    let store = open_store(&path);
    for (id, internal_order_id, event_type, created_at) in [
        ("evt-000004", "order-b", "b-late", "2026-07-29T00:04:00Z"),
        ("evt-000003", "order-a", "a-late", "2026-07-29T00:03:00Z"),
        ("evt-000001", "order-a", "a-early", "2026-07-29T00:01:00Z"),
        ("evt-000002", "order-b", "b-early", "2026-07-29T00:02:00Z"),
    ] {
        store
            .record_event(&StoredExecutionOrderEvent {
                id,
                internal_order_id,
                event_type,
                previous_status: None,
                next_status: "SUBMITTED",
                payload_json: "{}",
                created_at,
            })
            .expect("record execution order event");
    }
    let event_ids = |internal_order_id: &str| {
        store
            .list_order_events(internal_order_id)
            .expect("list execution order events")
            .into_iter()
            .map(|event| event.id)
            .collect::<Vec<_>>()
    };
    assert_eq!(
        event_ids("order-a"),
        vec!["evt-000001".to_owned(), "evt-000003".to_owned()]
    );
    assert_eq!(
        event_ids("order-b"),
        vec!["evt-000002".to_owned(), "evt-000004".to_owned()]
    );

    let connection = Connection::open(&path).expect("open query plan connection");
    let mut statement = connection
        .prepare(
            "EXPLAIN QUERY PLAN SELECT id, internal_order_id, event_type, previous_status,
                    next_status, payload_json, created_at
             FROM execution_order_events WHERE internal_order_id = ?1
             ORDER BY created_at ASC, id ASC",
        )
        .expect("prepare query plan");
    let details = statement
        .query_map(["order-a"], |row| row.get::<_, String>(3))
        .expect("query plan rows")
        .collect::<Result<Vec<_>, _>>()
        .expect("read query plan");
    assert!(
        details
            .iter()
            .any(|detail| detail.contains("idx_execution_order_events_order")),
        "event load must use the per-order index: {details:?}"
    );
    assert!(
        !details
            .iter()
            .any(|detail| detail.contains("USE TEMP B-TREE")),
        "event load must not sort with a temporary b-tree: {details:?}"
    );
}

// Parity: go:452dea11:internal/store/trading/persistence_failures_test.go:58 TestExecutionPersistenceLoadsStoredSequenceHighWaterMarks
#[test]
fn execution_order_sequence_high_water_marks_survive_reopen() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("sequences.db");
    seed_go_execution_orders_schema(&path);
    {
        let store = ExecutionOrderStore::open(&path).expect("open execution orders store");
        store
            .set_sequence("orders", 41)
            .expect("persist orders sequence");
        store
            .set_sequence("events", 42)
            .expect("persist events sequence");
    }
    let reopened = ExecutionOrderStore::open(&path).expect("reopen execution orders store");
    assert_eq!(
        reopened.get_sequence("orders").expect("orders sequence"),
        41
    );
    assert_eq!(
        reopened.get_sequence("events").expect("events sequence"),
        42
    );
    assert_eq!(
        reopened
            .next_sequence("orders")
            .expect("next orders sequence"),
        42
    );
    assert_eq!(
        reopened
            .next_sequence("orders")
            .expect("next orders sequence"),
        43
    );
}

fn concurrent_order(index: usize) -> StoredExecutionOrder {
    StoredExecutionOrder {
        internal_order_id: format!("concurrent-{index:02}"),
        broker_id: "futu".to_owned(),
        broker_order_id: None,
        broker_order_id_ex: None,
        source: "api".to_owned(),
        source_detail: "concurrency".to_owned(),
        trading_environment: "SIMULATE".to_owned(),
        account_id: "acc-1".to_owned(),
        market: "US".to_owned(),
        symbol: Some("US.AAPL".to_owned()),
        side: Some("BUY".to_owned()),
        order_type: Some("LIMIT".to_owned()),
        status: "SUBMITTED".to_owned(),
        raw_broker_status: None,
        requested_quantity: Some(1.0),
        requested_price: Some(100.0),
        filled_quantity: None,
        filled_average_price: None,
        remark: None,
        last_error: None,
        last_error_code: None,
        last_error_source: None,
        submitted_at: Some("2026-08-22T06:00:00Z".to_owned()),
        updated_at: "2026-08-22T06:00:00Z".to_owned(),
        created_at: "2026-08-22T06:00:00Z".to_owned(),
        order_kind: "single".to_owned(),
        product_class: "equity".to_owned(),
        quantity_mode: "quantity".to_owned(),
        client_order_id: Some(format!("concurrent-client-{index:02}")),
        preview_id: None,
        normalized_request: "{}".to_owned(),
        requested_amount: None,
        payout: None,
        fees: None,
    }
}

// Parity: go:452dea11:internal/store/trading/ledger_test.go:12 TestExecutionOrderStoreSortingFilteringAndMissingOrderBoundaries
#[test]
fn execution_orders_sort_updated_created_and_identity_after_reopening() {
    let directory = tempfile::tempdir().expect("directory");
    let path = directory.path().join("orders.db");
    seed_go_execution_orders_schema(&path);
    let store = open_store(&path);
    for (index, created, updated) in [
        (1, "2026-07-03T07:00:00Z", "2026-07-03T08:00:00Z"),
        (2, "2026-07-03T07:30:00Z", "2026-07-03T08:00:00Z"),
        (3, "2026-07-03T07:30:00Z", "2026-07-03T08:00:00Z"),
        (4, "2026-07-03T06:00:00Z", "2026-07-03T09:00:00Z"),
    ] {
        let mut order = concurrent_order(index);
        order.internal_order_id = format!("exec-{index:06}");
        order.created_at = created.to_owned();
        order.updated_at = updated.to_owned();
        store.save_order(order, updated).expect("seed order");
    }
    drop(store);
    let reopened = open_store(&path);
    let orders = reopened.list_orders().expect("reopened orders");
    assert_eq!(
        orders
            .iter()
            .map(|order| order.internal_order_id.as_str())
            .collect::<Vec<_>>(),
        ["exec-000004", "exec-000003", "exec-000002", "exec-000001"]
    );
    assert!(
        reopened
            .get_order("missing-order")
            .expect("missing order")
            .is_none()
    );
    for order in orders {
        assert!(
            reopened
                .list_order_events(&order.internal_order_id)
                .expect("read-only events")
                .is_empty()
        );
    }
}

// Parity: go:452dea11:internal/store/trading/maintenance_concurrency_test.go:43 TestExecutionStoreConcurrentReadsWritesAndDurableReload
#[test]
fn execution_order_concurrent_writes_and_reads_survive_reopen() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("concurrent.db");
    seed_go_execution_orders_schema(&path);
    let store = std::sync::Arc::new(open_store(&path));
    let mut handles = Vec::new();
    for index in 0..24 {
        let writer = std::sync::Arc::clone(&store);
        handles.push(std::thread::spawn(move || {
            writer
                .save_order(concurrent_order(index), "2026-08-22T06:00:00Z")
                .expect("concurrent save order");
        }));
        let reader = std::sync::Arc::clone(&store);
        handles.push(std::thread::spawn(move || {
            let _ = reader.list_orders();
        }));
    }
    for handle in handles {
        handle.join().expect("concurrent thread");
    }
    assert_eq!(store.order_count().expect("concurrent order count"), 24);
    drop(store);

    let reopened = open_store(&path);
    assert_eq!(reopened.order_count().expect("reloaded order count"), 24);
}

// Parity: go:452dea11:internal/store/trading/startup_compatibility_test.go:31 TestExecutionOrderPersistenceRejectsV1WithoutMutatingFile
#[test]
fn execution_order_legacy_metadata_is_rejected_without_mutating_the_file() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("legacy-v1.db");
    seed_go_execution_orders_schema(&path);
    let connection = Connection::open(&path).expect("open legacy fixture");
    connection
        .execute_batch(
            "UPDATE jftrade_schema_meta SET version = 1
             WHERE component_id = 'execution-orders';",
        )
        .expect("downgrade schema metadata");
    drop(connection);

    let before = std::fs::read(&path).expect("read legacy bytes");
    let error =
        ExecutionOrderTestCutoverStore::open_existing(&path, EXECUTION_ORDERS_TEST_CUTOVER_PROFILE)
            .expect_err("a v1 execution ledger must be rejected");
    assert!(
        matches!(error, ExecutionOrderStoreError::Schema(_)),
        "error = {error:?}"
    );
    let after = std::fs::read(&path).expect("read bytes after rejection");
    assert_eq!(before, after, "rejecting a v1 ledger must not rewrite it");
}

// Parity: go:452dea11:internal/store/trading/startup_compatibility_test.go:73 TestExecutionOrderPersistenceRejectsInvalidPaths
#[test]
fn execution_orders_store_rejects_directory_and_missing_parent_paths() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let directory_path = directory.path().join("orders-dir");
    std::fs::create_dir(&directory_path).expect("create directory path");
    assert!(
        matches!(
            ExecutionOrderTestCutoverStore::open_existing(
                &directory_path,
                EXECUTION_ORDERS_TEST_CUTOVER_PROFILE
            ),
            Err(ExecutionOrderStoreError::NotRegularFile(_))
        ),
        "a directory must never be opened as the execution ledger"
    );

    let nested = directory.path().join("missing-parent/orders.db");
    assert!(
        ExecutionOrderTestCutoverStore::open_existing(
            &nested,
            EXECUTION_ORDERS_TEST_CUTOVER_PROFILE
        )
        .is_err(),
        "a ledger under a missing parent directory must be rejected"
    );
}

// Parity: go:452dea11:internal/store/trading/startup_compatibility_test.go:87 TestExecutionOrderPersistenceRejectsPartialLegacySchema
#[test]
fn execution_orders_store_rejects_partial_legacy_schema_with_rebuild_guidance() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("partial-legacy.db");
    let connection = Connection::open(&path).expect("create partial legacy fixture");
    connection
        .execute_batch("CREATE TABLE execution_orders (internal_order_id TEXT PRIMARY KEY);")
        .expect("seed partial legacy table");
    drop(connection);

    let error =
        ExecutionOrderTestCutoverStore::open_existing(&path, EXECUTION_ORDERS_TEST_CUTOVER_PROFILE)
            .expect_err("a partial legacy schema must be rejected");
    assert!(
        error.to_string().contains("rebuild"),
        "rejection must guide a rebuild: {error}"
    );
    assert!(
        matches!(error, ExecutionOrderStoreError::Schema(_)),
        "error = {error:?}"
    );
}

// Parity: go:452dea11:internal/store/trading/startup_compatibility_test.go:105 TestExecutionOrderPersistenceRejectsWrongColumnLayout
#[test]
fn execution_orders_store_rejects_wrong_column_layout() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("wrong-columns.db");
    let connection = Connection::open(&path).expect("create wrong-column fixture");
    connection
        .execute_batch(
            "CREATE TABLE execution_orders (internal_order_id TEXT PRIMARY KEY);
             CREATE TABLE execution_order_events (id TEXT PRIMARY KEY);
             CREATE TABLE execution_seen_fills (fill_key TEXT PRIMARY KEY);
             CREATE TABLE execution_sequences (name TEXT PRIMARY KEY);",
        )
        .expect("seed wrong column schema");
    drop(connection);

    let error =
        ExecutionOrderTestCutoverStore::open_existing(&path, EXECUTION_ORDERS_TEST_CUTOVER_PROFILE)
            .expect_err("a drifted column layout must be rejected");
    assert!(
        matches!(error, ExecutionOrderStoreError::Schema(_)),
        "error = {error:?}"
    );
}

// Parity: go:452dea11:internal/store/trading/startup_compatibility_test.go:156 TestExecutionOrderPersistenceLoadRejectsMissingRuntimeTables
#[test]
fn execution_orders_store_rejects_missing_runtime_tables() {
    for table in [
        "execution_orders",
        "execution_order_events",
        "execution_seen_fills",
        "execution_sequences",
    ] {
        let directory = tempfile::tempdir().expect("temporary directory");
        let path = directory.path().join("missing-table.db");
        seed_go_execution_orders_schema(&path);
        let connection = Connection::open(&path).expect("open fixture for table drop");
        connection
            .execute_batch(&format!("DROP TABLE {table};"))
            .expect("drop runtime table");
        drop(connection);

        let error = ExecutionOrderTestCutoverStore::open_existing(
            &path,
            EXECUTION_ORDERS_TEST_CUTOVER_PROFILE,
        )
        .expect_err("a ledger missing a runtime table must be rejected");
        assert!(
            matches!(error, ExecutionOrderStoreError::Schema(_)),
            "{table} drop must surface a schema rejection: {error:?}"
        );
    }
}

// Parity: go:452dea11:internal/store/trading/persistence_failures_test.go:14 TestExecutionPersistenceConstructorDependencyFailures
#[test]
fn execution_orders_store_constructor_rejects_empty_missing_and_malformed_inputs() {
    assert!(matches!(
        ExecutionOrderTestCutoverStore::open_existing(
            Path::new(""),
            EXECUTION_ORDERS_TEST_CUTOVER_PROFILE
        ),
        Err(ExecutionOrderStoreError::EmptyPath)
    ));

    let directory = tempfile::tempdir().expect("temporary directory");
    let missing = directory.path().join("missing.db");
    assert!(matches!(
        ExecutionOrderTestCutoverStore::open_existing(
            &missing,
            EXECUTION_ORDERS_TEST_CUTOVER_PROFILE
        ),
        Err(ExecutionOrderStoreError::NotRegularFile(_))
    ));

    let malformed = directory.path().join("malformed-metadata.db");
    let connection = Connection::open(&malformed).expect("create malformed metadata fixture");
    connection
        .execute_batch(
            "CREATE TABLE jftrade_schema_meta (component_id TEXT PRIMARY KEY);
             INSERT INTO jftrade_schema_meta (component_id) VALUES ('execution-orders');",
        )
        .expect("seed malformed metadata");
    drop(connection);
    let error = ExecutionOrderTestCutoverStore::open_existing(
        &malformed,
        EXECUTION_ORDERS_TEST_CUTOVER_PROFILE,
    )
    .expect_err("malformed component metadata must be rejected");
    assert!(
        matches!(error, ExecutionOrderStoreError::Schema(_)),
        "error = {error:?}"
    );

    let dropped = directory.path().join("dropped-orders.db");
    seed_go_execution_orders_schema(&dropped);
    let connection = Connection::open(&dropped).expect("open dropped orders fixture");
    connection
        .execute_batch("DROP TABLE execution_orders;")
        .expect("drop orders table");
    drop(connection);
    let error = ExecutionOrderTestCutoverStore::open_existing(
        &dropped,
        EXECUTION_ORDERS_TEST_CUTOVER_PROFILE,
    )
    .expect_err("a ledger without its orders table must be rejected");
    assert!(
        matches!(error, ExecutionOrderStoreError::Schema(_)),
        "error = {error:?}"
    );
}
