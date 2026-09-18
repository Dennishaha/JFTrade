//! Parity: go:452dea11:internal/store/trading/submission_safety_test.go:117
//! TestPredictionRFQPersistsBindingExpiryAndSingleConsumption
//!
//! Go stores a server-issued RFQ in `execution_prediction_quotes` and rejects a
//! changed broker/account/environment/MVC/legs binding, an expired window and a
//! second consumer while replaying the identical preview/client pair
//! idempotently. Rust must own the same durable fence inside the
//! execution-orders database.

use std::path::Path;

use jftrade_store_sqlite::{
    EXECUTION_ORDERS_TEST_CUTOVER_PROFILE, ExecutionOrderStore, ExecutionOrderStoreError,
    StoredPredictionQuote, initialize_current,
};
use rusqlite::Connection;

const RECEIVED_AT: &str = "2026-08-22T06:00:00Z";
const EXPIRES_AT: &str = "2026-08-22T06:00:30Z";
const BEFORE_EXPIRY: &str = "2026-08-22T06:00:10Z";
const AFTER_EXPIRY: &str = "2026-08-22T06:00:31Z";
const LEGS_HASH: &str = "b6a1d0e5b1f5c3b0a3d2f4e6c7a8b9d0e1f2a3b4c5d6e7f8091a2b3c4d5e6f70";

fn open_store(path: &Path) -> ExecutionOrderStore {
    ExecutionOrderStore::open_existing(path, EXECUTION_ORDERS_TEST_CUTOVER_PROFILE)
        .expect("open execution orders store")
}

fn active_quote(quote_id: &str) -> StoredPredictionQuote {
    StoredPredictionQuote {
        quote_id: quote_id.to_owned(),
        broker_id: "futu".to_owned(),
        account_id: "acct-1".to_owned(),
        trading_environment: "REAL".to_owned(),
        mvc: "mvc-1".to_owned(),
        legs_hash: LEGS_HASH.to_owned(),
        bid_price: Some(0.42),
        ask_price: Some(0.45),
        should_retry: true,
        received_at: RECEIVED_AT.to_owned(),
        expires_at: EXPIRES_AT.to_owned(),
        expiry_source: "jftrade_policy".to_owned(),
        status: "active".to_owned(),
        consumed_at: None,
        consumed_preview_id: None,
        consumed_client_order_id: None,
    }
}

#[test]
fn prediction_rfq_persists_binding_expiry_and_single_consumption() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("prediction-rfq.db");
    seed_prediction_quotes_schema(&path);
    let store = open_store(&path);

    let quote = active_quote("rfq-1");
    store
        .save_prediction_quote(&quote)
        .expect("save prediction quote");

    let validated = store
        .validate_prediction_quote(
            "rfq-1",
            "futu",
            "acct-1",
            "REAL",
            "mvc-1",
            LEGS_HASH,
            BEFORE_EXPIRY,
        )
        .expect("validate stored quote");
    assert_eq!(validated.expiry_source, "jftrade_policy");
    assert_eq!(validated.bid_price, Some(0.42));
    assert_eq!(validated.ask_price, Some(0.45));
    assert!(validated.should_retry);

    // A different account must never price against this RFQ.
    let cross_account = store
        .validate_prediction_quote(
            "rfq-1",
            "futu",
            "acct-2",
            "REAL",
            "mvc-1",
            LEGS_HASH,
            BEFORE_EXPIRY,
        )
        .expect_err("cross-account validation must fail");
    assert!(
        matches!(&cross_account, ExecutionOrderStoreError::Validation(message) if message.contains("changed")),
        "unexpected cross-account error: {cross_account:?}"
    );

    // Changed legs, MVC and environment are equally hard failures.
    for (environment, mvc, hash) in [
        ("SIMULATE", "mvc-1", LEGS_HASH),
        ("REAL", "mvc-2", LEGS_HASH),
        (
            "REAL",
            "mvc-1",
            "0000000000000000000000000000000000000000000000000000000000000000",
        ),
    ] {
        assert!(
            matches!(
                store.validate_prediction_quote(
                    "rfq-1",
                    "futu",
                    "acct-1",
                    environment,
                    mvc,
                    hash,
                    BEFORE_EXPIRY,
                ),
                Err(ExecutionOrderStoreError::Validation(_))
            ),
            "changed binding {environment}/{mvc}/{hash} was accepted"
        );
    }

    store
        .consume_prediction_quote(
            "rfq-1",
            "futu",
            "acct-1",
            "REAL",
            "mvc-1",
            LEGS_HASH,
            "preview-1",
            "client-1",
            BEFORE_EXPIRY,
        )
        .expect("first consume");
    store
        .consume_prediction_quote(
            "rfq-1",
            "futu",
            "acct-1",
            "REAL",
            "mvc-1",
            LEGS_HASH,
            "preview-1",
            "client-1",
            BEFORE_EXPIRY,
        )
        .expect("identical replay is idempotent");
    let reused = store
        .consume_prediction_quote(
            "rfq-1",
            "futu",
            "acct-1",
            "REAL",
            "mvc-1",
            LEGS_HASH,
            "preview-2",
            "client-2",
            BEFORE_EXPIRY,
        )
        .expect_err("a second consumer must fail");
    assert!(
        matches!(&reused, ExecutionOrderStoreError::Conflict(message) if message.contains("consumed")),
        "unexpected reuse error: {reused:?}"
    );
}

#[test]
fn prediction_rfq_rejects_missing_expired_and_malformed_rows() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("prediction-rfq-failures.db");
    seed_prediction_quotes_schema(&path);
    let store = open_store(&path);

    // Unknown quote ids are not found; a blank id is never looked up.
    assert!(matches!(
        store.validate_prediction_quote(
            "missing",
            "futu",
            "acct-1",
            "REAL",
            "mvc-1",
            LEGS_HASH,
            BEFORE_EXPIRY,
        ),
        Err(ExecutionOrderStoreError::NotFound(_))
    ));
    assert!(matches!(
        store.validate_prediction_quote(
            "  ",
            "futu",
            "acct-1",
            "REAL",
            "mvc-1",
            LEGS_HASH,
            BEFORE_EXPIRY,
        ),
        Err(ExecutionOrderStoreError::NotFound(_))
    ));

    // Save-time validation keeps incomplete RFQs out of the ledger.
    let mut incomplete = active_quote("rfq-incomplete");
    incomplete.legs_hash = "  ".to_owned();
    assert!(matches!(
        store.save_prediction_quote(&incomplete),
        Err(ExecutionOrderStoreError::Validation(_))
    ));
    let mut bad_source = active_quote("rfq-bad-source");
    bad_source.expiry_source = String::new();
    assert!(matches!(
        store.save_prediction_quote(&bad_source),
        Err(ExecutionOrderStoreError::Validation(_))
    ));
    let mut bad_status = active_quote("rfq-bad-status");
    bad_status.status = "pending".to_owned();
    assert!(matches!(
        store.save_prediction_quote(&bad_status),
        Err(ExecutionOrderStoreError::Validation(_))
    ));
    let mut bad_timestamp = active_quote("rfq-bad-timestamp");
    bad_timestamp.expires_at = "not-a-timestamp".to_owned();
    assert!(matches!(
        store.save_prediction_quote(&bad_timestamp),
        Err(ExecutionOrderStoreError::Validation(_))
    ));

    // An expired RFQ must be refused even though it was persisted while valid.
    let mut expired = active_quote("rfq-expired");
    expired.expires_at = BEFORE_EXPIRY.to_owned();
    store
        .save_prediction_quote(&expired)
        .expect("save expired quote");
    let expired_error = store
        .validate_prediction_quote(
            "rfq-expired",
            "futu",
            "acct-1",
            "REAL",
            "mvc-1",
            LEGS_HASH,
            AFTER_EXPIRY,
        )
        .expect_err("expired quote must be rejected");
    assert!(
        matches!(&expired_error, ExecutionOrderStoreError::Validation(message) if message.contains("expired")),
        "unexpected expiry error: {expired_error:?}"
    );

    // A bad caller timestamp is a validation failure, not a silent success.
    assert!(matches!(
        store.validate_prediction_quote(
            "rfq-1", "futu", "acct-1", "REAL", "mvc-1", LEGS_HASH, "nope",
        ),
        Err(ExecutionOrderStoreError::Validation(_))
    ));
}

#[test]
fn prediction_rfq_consume_is_fenced_by_status_and_expiry_in_one_transaction() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("prediction-rfq-fence.db");
    seed_prediction_quotes_schema(&path);
    let store = open_store(&path);

    let mut quote = active_quote("rfq-fence");
    quote.consumed_at = Some(RECEIVED_AT.to_owned());
    quote.consumed_preview_id = Some("preview-old".to_owned());
    quote.consumed_client_order_id = Some("client-old".to_owned());
    store
        .save_prediction_quote(&quote)
        .expect("save pre-consumed quote");
    store
        .consume_prediction_quote(
            "rfq-fence",
            "futu",
            "acct-1",
            "REAL",
            "mvc-1",
            LEGS_HASH,
            "preview-old",
            "client-old",
            BEFORE_EXPIRY,
        )
        .expect("identical replay is idempotent");
    let other = store
        .consume_prediction_quote(
            "rfq-fence",
            "futu",
            "acct-1",
            "REAL",
            "mvc-1",
            LEGS_HASH,
            "preview-new",
            "client-new",
            BEFORE_EXPIRY,
        )
        .expect_err("other consumer must fail");
    assert!(matches!(other, ExecutionOrderStoreError::Conflict(_)));

    // Consuming after the window fails even for the original pair.
    let quote = active_quote("rfq-late");
    store.save_prediction_quote(&quote).expect("save quote");
    let late = store
        .consume_prediction_quote(
            "rfq-late",
            "futu",
            "acct-1",
            "REAL",
            "mvc-1",
            LEGS_HASH,
            "preview-late",
            "client-late",
            AFTER_EXPIRY,
        )
        .expect_err("late consume must fail");
    assert!(matches!(late, ExecutionOrderStoreError::Validation(_)));
}

/// Build the database from the pinned Go schema manifest so the store's
/// schema validation and the prediction-quote table come from the same
/// authoritative definition the production adapter uses.
fn seed_prediction_quotes_schema(path: &Path) {
    let connection = Connection::open(path).expect("create prediction quote fixture");
    initialize_current(&connection, "execution-orders")
        .expect("initialize execution-orders schema");
}
