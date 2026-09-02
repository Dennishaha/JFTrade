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
