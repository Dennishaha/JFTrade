use std::path::Path;

use jftrade_owner_lock::WriterLeaseError;
use jftrade_store_sqlite::{
    EXECUTION_ORDERS_TEST_CUTOVER_PROFILE, ExecutionOrderStoreError,
    ExecutionOrderTestCutoverStore, StoredExecutionOrder, StoredExecutionOrderEvent,
};
use rusqlite::Connection;

const TIMESTAMP_1: &str = "2026-08-22T06:00:00Z";

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
