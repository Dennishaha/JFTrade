use serde_json::json;

use super::actions_test_support::{
    account_snapshot, combo_quote_port, combo_quote_request, failing_combo_quote_port,
};
use super::product_prediction_combo_quote::{PredictionComboQuoteRequest, prediction_quote_expiry};
use super::*;

/// Parity: go:452dea11:internal/productfeatures/prediction_quote_candle_bridge_test.go:13
/// TestQuotePredictionComboValidatesPersistsAndPublishesServerExpiry
///
/// Go normalizes the request context and legs, forwards the ORDER_OF legs to the
/// adapter, then stamps `receivedAt`/`quoteExpiresAt`/`expirySource` from its own
/// 30 second policy and persists the quote. The saved record must carry the
/// normalized binding, the adapter's bid/ask/retry metadata and a non-empty legs
/// hash that is stable across whitespace and case differences.
#[tokio::test]
async fn prediction_combo_quote_validates_persists_and_publishes_server_expiry() {
    let (directory, store, port) = combo_quote_port(
        json!({
            "quoteId": "quote-1",
            "bidPrice": 0.42,
            "askPrice": 0.45,
            "shouldRetry": true,
        }),
        "2026-07-18T12:00:00Z",
        vec![account_snapshot(1, Some(2), vec![11])],
    );
    let request = combo_quote_request(
        br#"{
            "brokerId":"futu",
            "accountId":"1",
            "tradingEnvironment":" simulate ",
            "mvc":" mvc-1 ",
            "legs":[
                {"instrumentId":" us.event-one ","side":" buy ","predictionSide":" yes ","ratio":1},
                {"instrumentId":"US.EVENT-TWO","productClass":"event_contract","side":"SELL","predictionSide":"NO","ratio":2}
            ]
        }"#,
        "",
    );
    let result = port.dispatch(&request).await.expect("RFQ must succeed");
    assert_eq!(result["metadata"]["quoteId"], "quote-1");
    assert_eq!(result["metadata"]["mvc"], "mvc-1");
    assert_eq!(result["metadata"]["receivedAt"], "2026-07-18T12:00:00Z");
    assert_eq!(result["metadata"]["quoteExpiresAt"], "2026-07-18T12:00:30Z");
    assert_eq!(result["metadata"]["expirySource"], "jftrade_policy");
    assert_eq!(result["warnings"].as_array().map(Vec::len), Some(1));

    let binding = PredictionComboQuoteRequest::parse(
        &json!({
            "brokerId": "futu",
            "accountId": "1",
            "tradingEnvironment": "SIMULATE",
            "mvc": "mvc-1",
            "legs": [
                {"instrumentId": "US.EVENT-ONE", "productClass": "event_contract", "side": "BUY", "ratio": 1, "predictionSide": "YES"},
                {"instrumentId": "US.EVENT-TWO", "productClass": "event_contract", "side": "SELL", "ratio": 2, "predictionSide": "NO"},
            ],
        }),
        "",
    )
    .expect("expected binding");
    let saved = store
        .validate_prediction_quote(
            "quote-1",
            "futu",
            "1",
            "SIMULATE",
            "mvc-1",
            &binding.legs_hash(),
            "2026-07-18T12:00:10Z",
        )
        .expect("stored RFQ must validate");
    assert_eq!(saved.expiry_source, "jftrade_policy");
    assert_eq!(saved.bid_price, Some(0.42));
    assert_eq!(saved.ask_price, Some(0.45));
    assert!(saved.should_retry);
    assert_eq!(saved.expires_at, "2026-07-18T12:00:30Z");
    drop(directory);
}

/// Parity: go:452dea11:internal/productfeatures/prediction_quote_candle_bridge_test.go:61
/// TestQuotePredictionComboRejectsInvalidAndUnpersistableQuotes
///
/// Every invalid leg shape, a missing adapter `quoteId`, a missing persistence
/// owner and a failing store are request/upstream errors that must surface
/// before anything is written. The adapter must never be called for an invalid
/// request.
#[tokio::test]
// Parity: go:452dea11:internal/api/productfeatures/prediction_combo_routes_test.go:15 TestPredictionComboQuoteAcceptsContextFromQueryAndMapsFailures
async fn prediction_combo_quote_rejects_invalid_and_unpersistable_requests() {
    let (directory, _store, port) = combo_quote_port(
        json!({"quoteId": "quote-1"}),
        "2026-07-18T12:00:00Z",
        vec![account_snapshot(1, Some(2), vec![11])],
    );
    let valid = json!({
        "brokerId": "futu",
        "accountId": "1",
        "tradingEnvironment": "SIMULATE",
        "mvc": "mvc-1",
        "legs": [
            {"instrumentId": "US.EC.ONE", "side": "BUY", "predictionSide": "YES", "ratio": 1},
            {"instrumentId": "US.EC.TWO", "side": "BUY", "predictionSide": "NO", "ratio": 1},
        ],
    });
    let cases = [
        ("missing context", {
            let mut value = valid.clone();
            value["accountId"] = json!(" ");
            value
        }),
        ("one leg", {
            let mut value = valid.clone();
            value["legs"] = json!([{"instrumentId": "US.EC.ONE", "side": "BUY", "predictionSide": "YES", "ratio": 1}]);
            value
        }),
        ("wrong product", {
            let mut value = valid.clone();
            value["legs"][0]["productClass"] = json!("equity");
            value
        }),
        ("wrong market", {
            let mut value = valid.clone();
            value["legs"][0]["instrumentId"] = json!("HK.1");
            value
        }),
        ("wrong side", {
            let mut value = valid.clone();
            value["legs"][0]["side"] = json!("hold");
            value
        }),
        ("wrong prediction side", {
            let mut value = valid.clone();
            value["legs"][0]["predictionSide"] = json!("maybe");
            value
        }),
        ("wrong ratio", {
            let mut value = valid.clone();
            value["legs"][0]["ratio"] = json!(0);
            value
        }),
    ];
    for (name, payload) in cases {
        let request = combo_quote_request(payload.to_string().as_bytes(), "");
        match port.dispatch(&request).await {
            Err(MarketDataProviderActionsPortError::Failed { status, code, .. }) => {
                assert_eq!(status, 400, "{name} status");
                assert_eq!(code, "BAD_REQUEST", "{name} code");
            }
            other => panic!("{name} must be a 400 BAD_REQUEST, got {other:?}"),
        }
    }

    // Query parameters back-fill the body exactly like the Go handler.
    let body_only_legs = json!({
        "mvc": "mvc-1",
        "legs": valid["legs"].clone(),
    });
    let request = combo_quote_request(
        body_only_legs.to_string().as_bytes(),
        "brokerId=futu&accountId=1&tradingEnvironment=simulate",
    );
    let _ = port
        .dispatch(&request)
        .await
        .expect("query context must fill missing fields");
    drop(directory);

    // A missing adapter quoteId and a missing persistence owner are hard errors.
    let (directory, _store, port) = combo_quote_port(
        json!({}),
        "2026-07-18T12:00:00Z",
        vec![account_snapshot(1, Some(2), vec![11])],
    );
    let request = combo_quote_request(valid.to_string().as_bytes(), "");
    let error = port
        .dispatch(&request)
        .await
        .expect_err("missing quoteId must fail");
    assert!(
        matches!(&error, MarketDataProviderActionsPortError::Failed { message, .. } if message.contains("quoteId")),
        "missing quoteId error = {error:?}"
    );
    drop(directory);

    let (directory, _store, port) = combo_quote_port(
        json!({"quoteId": "quote-1"}),
        "2026-07-18T12:00:00Z",
        vec![account_snapshot(1, Some(2), vec![11])],
    );
    let storeless = port.with_prediction_quotes(None);
    let request = combo_quote_request(valid.to_string().as_bytes(), "");
    let error = storeless
        .dispatch(&request)
        .await
        .expect_err("missing persistence must fail");
    assert!(
        matches!(&error, MarketDataProviderActionsPortError::Failed { message, .. } if message.contains("persistence")),
        "missing persistence error = {error:?}"
    );
    drop(directory);
}

/// Go's route test asserts the RFQ failure wire contract: an ineligible
/// account is `403 PREDICTION_MARKET_INELIGIBLE` before the adapter runs, and an
/// upstream RFQ failure stays a `502 BROKER_FEATURE_FAILED` carrying the
/// adapter message.
#[tokio::test]
async fn prediction_combo_quote_maps_ineligibility_and_upstream_failure() {
    let body = json!({
        "brokerId": "futu",
        "accountId": "1",
        "tradingEnvironment": "SIMULATE",
        "mvc": "mvc-1",
        "legs": [
            {"instrumentId": "US.EC.ONE", "side": "BUY", "predictionSide": "YES", "ratio": 1},
            {"instrumentId": "US.EC.TWO", "side": "BUY", "predictionSide": "NO", "ratio": 1},
        ],
    });

    // HK-authority account: the same request is 403 and never reaches the
    // adapter, which is Go's `predictionEligibility` gate.
    let (directory, _store, port) = combo_quote_port(
        json!({"quoteId": "quote-1"}),
        "2026-07-18T12:00:00Z",
        vec![account_snapshot(1, Some(2), vec![12])],
    );
    let request = combo_quote_request(body.to_string().as_bytes(), "");
    match port.dispatch(&request).await {
        Err(MarketDataProviderActionsPortError::Failed { status, code, message, .. }) => {
            assert_eq!(status, 403);
            assert_eq!(code, "PREDICTION_MARKET_INELIGIBLE");
            assert!(
                message.starts_with("prediction market requires an eligible Moomoo US account"),
                "message = {message}"
            );
        }
        other => panic!("HK authority must be 403, got {other:?}"),
    }
    drop(directory);

    // Non-FUTUINC firm: also 403.
    let (directory, _store, port) = combo_quote_port(
        json!({"quoteId": "quote-1"}),
        "2026-07-18T12:00:00Z",
        vec![account_snapshot(1, Some(1), vec![11])],
    );
    let request = combo_quote_request(body.to_string().as_bytes(), "");
    match port.dispatch(&request).await {
        Err(MarketDataProviderActionsPortError::Failed { status, code, .. }) => {
            assert_eq!(status, 403);
            assert_eq!(code, "PREDICTION_MARKET_INELIGIBLE");
        }
        other => panic!("non-FUTUINC firm must be 403, got {other:?}"),
    }
    drop(directory);

    // Upstream RFQ failure: 502 with the adapter message preserved.
    let (directory, _store, port) = failing_combo_quote_port(
        "rfq unavailable",
        "2026-07-18T12:00:00Z",
        vec![account_snapshot(1, Some(2), vec![11])],
    );
    let request = combo_quote_request(body.to_string().as_bytes(), "");
    match port.dispatch(&request).await {
        Err(MarketDataProviderActionsPortError::Failed { status, code, message, .. }) => {
            assert_eq!(status, 502);
            assert_eq!(code, "BROKER_FEATURE_FAILED");
            assert!(message.contains("rfq unavailable"), "message = {message}");
        }
        other => panic!("upstream RFQ failure must be 502, got {other:?}"),
    }
    drop(directory);
}

#[test]
fn prediction_quote_expiry_always_adds_the_server_window() {
    assert_eq!(
        prediction_quote_expiry("2026-07-18T12:00:00Z").expect("expiry"),
        "2026-07-18T12:00:30Z"
    );
    assert_eq!(
        prediction_quote_expiry("2026-07-18T12:00:00.500Z").expect("expiry"),
        "2026-07-18T12:00:30.5Z"
    );
    assert!(prediction_quote_expiry("not-a-timestamp").is_err());
}

/// Go's `broker.PredictionQuoteLegsHash`: normalized legs only, so whitespace
/// and case never change the digest while a real change does.
/// Parity: go:452dea11:pkg/broker/product_capability_contracts_test.go:11 TestPredictionQuoteLegsHashNormalizesBrokerNeutralLegs
#[test]
fn prediction_quote_legs_hash_matches_go_normalization() {
    let parse = |value: serde_json::Value| {
        PredictionComboQuoteRequest::parse(&value, "")
            .unwrap_or_else(|error| panic!("unexpected parse error: {error}"))
    };
    let left = parse(json!({
        "brokerId": "futu",
        "accountId": "1",
        "tradingEnvironment": "SIMULATE",
        "mvc": " mvc ",
        "legs": [
            {"instrumentId": " us.ec.one ", "productClass": "event_contract", "side": " buy ", "ratio": 1, "amount": 10.0, "predictionSide": " yes "},
            {"instrumentId": "US.EC.TWO", "side": "sell", "ratio": 2, "predictionSide": "no"},
        ],
    }));
    let right = parse(json!({
        "brokerId": "futu",
        "accountId": "1",
        "tradingEnvironment": "SIMULATE",
        "mvc": "mvc",
        "legs": [
            {"instrumentId": "US.EC.ONE", "productClass": "event_contract", "side": "BUY", "ratio": 1, "amount": 10.0, "predictionSide": "YES"},
            {"instrumentId": "US.EC.TWO", "productClass": "event_contract", "side": "SELL", "ratio": 2, "predictionSide": "NO"},
        ],
    }));
    let left_hash = left.legs_hash();
    assert_eq!(left_hash.len(), 64, "legs hash is a hex sha256");
    assert_eq!(left_hash, right.legs_hash());

    // mvc is part of the binding, so changing it changes the digest.
    let changed_mvc = parse(json!({
        "brokerId": "futu",
        "accountId": "1",
        "tradingEnvironment": "SIMULATE",
        "mvc": "mvc-2",
        "legs": [
            {"instrumentId": "US.EC.ONE", "productClass": "event_contract", "side": "BUY", "ratio": 1, "amount": 10.0, "predictionSide": "YES"},
            {"instrumentId": "US.EC.TWO", "productClass": "event_contract", "side": "SELL", "ratio": 2, "predictionSide": "NO"},
        ],
    }));
    assert_ne!(left_hash, changed_mvc.legs_hash());

    // A changed prediction side also changes the digest.
    let changed_side = parse(json!({
        "brokerId": "futu",
        "accountId": "1",
        "tradingEnvironment": "SIMULATE",
        "mvc": "mvc",
        "legs": [
            {"instrumentId": "US.EC.ONE", "productClass": "event_contract", "side": "BUY", "ratio": 1, "amount": 10.0, "predictionSide": "NO"},
            {"instrumentId": "US.EC.TWO", "productClass": "event_contract", "side": "SELL", "ratio": 2, "predictionSide": "NO"},
        ],
    }));
    assert_ne!(left_hash, changed_side.legs_hash());
}
