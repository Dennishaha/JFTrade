use super::execution_order_hash::preview_request_hash;
use super::execution_order_parse::{parse_combo, parse_order};
use serde_json::{Value, json};

fn single_order_payload() -> Value {
    json!({
        "accountId": "1001",
        "market": "US",
        "symbol": "AAPL",
        "side": "BUY",
        "orderType": "LIMIT",
        "quantity": 1,
        "price": 100,
        "clientOrderId": "client-a"
    })
}

// Parity: internal/api/trading/execution_test.go:78 TestHandleExecutionPlaceRejectsEquityAmountModeSpoof
// Verifies equity orders reject event-contract fields (amount, predictionSide, quantityMode) with BAD_REQUEST before broker call
#[test]
fn single_equity_rejects_event_only_fields() {
    let cases = [
        (
            "amount",
            json!({"amount": 1}),
            "amount is supported for event contracts only",
        ),
        (
            "prediction side",
            json!({"predictionSide": "YES"}),
            "predictionSide is supported for event contracts only",
        ),
        (
            "amount quantity mode",
            json!({"quantityMode": "amount"}),
            "quantityMode",
        ),
    ];

    for (name, override_fields, expected_message) in cases {
        let mut payload = single_order_payload();
        payload
            .as_object_mut()
            .expect("single order payload object")
            .extend(
                override_fields
                    .as_object()
                    .expect("override object")
                    .clone(),
            );
        let error = parse_order(&payload).expect_err(name);
        assert!(
            error.contains(expected_message),
            "{name} error = {error:?}, want substring {expected_message:?}"
        );
    }
}

#[test]
fn single_preview_hash_binds_client_order_id() {
    let first = single_order_payload();
    let mut second = first.clone();
    second["clientOrderId"] = json!("client-b");
    let first_order = parse_order(&first).expect("first single order");
    let second_order = parse_order(&second).expect("second single order");

    let first_hash = preview_request_hash(&first, &first_order, None).expect("first hash");
    let second_hash = preview_request_hash(&second, &second_order, None).expect("second hash");
    assert_ne!(first_hash, second_hash);
}

fn option_combo_payload(client_order_id: &str) -> Value {
    json!({
        "accountId": "1001",
        "market": "US",
        "clientOrderId": client_order_id,
        "orderKind": "option_combo",
        "productClass": "option",
        "underlyingInstrumentId": "US.AAPL",
        "optionStrategy": "vertical",
        "nearExpiry": "2026-07-17",
        "spread": 10,
        "legs": [
            {
                "instrumentId": "US.AAPL260717C00200000",
                "productClass": "option",
                "side": "BUY",
                "ratio": 1
            },
            {
                "instrumentId": "US.AAPL260717C00210000",
                "productClass": "option",
                "side": "SELL",
                "ratio": 1
            }
        ]
    })
}

#[test]
fn combo_preview_hash_binds_client_order_id() {
    let first = option_combo_payload("client-a");
    let second = option_combo_payload("client-b");
    let first_combo = parse_combo(&first).expect("first combo");
    let second_combo = parse_combo(&second).expect("second combo");

    let first_hash = preview_request_hash(
        &first,
        &first_combo.order,
        Some(json!(first_combo.leg_payloads)),
    )
    .expect("first combo hash");
    let second_hash = preview_request_hash(
        &second,
        &second_combo.order,
        Some(json!(second_combo.leg_payloads)),
    )
    .expect("second combo hash");
    assert_ne!(first_hash, second_hash);
}

#[test]
fn test_normalize_execution_order_uses_env_fallback_and_supports_non_limit_us_sessions() {
    // Parity: internal/trading/execution_test.go:965 TestNormalizeExecutionOrderUsesEnvFallbackAndSupportsNonLimitUSSessions
    let payload = json!({
        "env": "real",
        "market": "US",
        "symbol": "AAPL",
        "side": "BUY",
        "orderType": "MARKET",
        "session": "OVERNIGHT",
        "quantity": 2,
        "accountId": "1001",
    });

    let order = parse_order(&payload).expect("parse market overnight order");
    assert_eq!(order.header.trd_env, 1); // REAL
    assert_eq!(order.session, Some(4)); // OVERNIGHT
    assert_eq!(order.fill_outside_rth, None); // Must be None for non-limit orders
}

#[test]
fn test_execution_normalization_helpers_reject_unsupported_inputs() {
    // Parity: internal/trading/execution_test.go:988 TestExecutionNormalizationHelpersRejectUnsupportedInputs
    let unsupported_type = json!({
        "accountId": "1001",
        "market": "US",
        "symbol": "AAPL",
        "side": "BUY",
        "orderType": "iceberg",
        "quantity": 1,
        "price": 100,
    });
    let err = parse_order(&unsupported_type).expect_err("iceberg must be rejected");
    assert!(err.to_lowercase().contains("unsupported") || err.to_lowercase().contains("invalid"));

    let unsupported_session = json!({
        "accountId": "1001",
        "market": "US",
        "symbol": "AAPL",
        "side": "BUY",
        "orderType": "LIMIT",
        "session": "pre-open",
        "quantity": 1,
        "price": 100,
    });
    let err = parse_order(&unsupported_session).expect_err("pre-open session must be rejected");
    assert!(err.to_lowercase().contains("session") || err.to_lowercase().contains("unsupported") || err.to_lowercase().contains("invalid"));
}

#[test]
fn test_normalize_execution_order_rejects_invalid_instrument() {
    // Parity: internal/trading/execution_test.go:997 TestNormalizeExecutionOrderRejectsInvalidInstrumentAndUnsupportedOrderType
    let missing_symbol = json!({
        "accountId": "1001",
        "market": "US",
        "side": "BUY",
        "orderType": "LIMIT",
        "quantity": 1,
        "price": 100,
    });
    let err = parse_order(&missing_symbol).expect_err("missing symbol must be rejected");
    assert!(err.to_lowercase().contains("symbol") || err.to_lowercase().contains("instrument") || err.to_lowercase().contains("code"));
}
