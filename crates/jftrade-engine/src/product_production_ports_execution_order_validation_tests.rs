use super::execution_order_hash::preview_request_hash;
use super::execution_order_parse::{parse_combo, parse_order};
use serde_json::{Value, json};

/// Go hands the normalized order type to the Futu adapter, which resolves it
/// through `trdOrderTypeFromBBGOOrderType` onto OpenD's `Trd_Common.OrderType`
/// enum (`Normal=1`, `Market=2`, `AbsoluteLimit=5`, `Auction=6`,
/// `AuctionLimit=7`, `Stop=10`, `StopLimit=11`, `MarketifTouched=12`,
/// `LimitifTouched=13`). The parsed code is sent verbatim as
/// `Trd_PlaceOrder.orderType`, so it must already speak the wire enum rather
/// than a private numbering.
#[test]
fn parsed_order_type_matches_the_opend_wire_enum() {
    let cases = [
        ("LIMIT", 1),
        ("NORMAL", 1),
        ("MARKET", 2),
        ("ABSOLUTE_LIMIT", 5),
        ("AUCTION", 6),
        ("AUCTION_LIMIT", 7),
        ("STOP", 10),
        ("STOP_MARKET", 10),
        ("STOP_LIMIT", 11),
    ];
    for (raw, expected) in cases {
        let payload = json!({
            "accountId": "1001",
            "market": "US",
            "symbol": "AAPL",
            "side": "BUY",
            "orderType": raw,
            "quantity": 1,
            "price": 100,
            "stopPrice": 99,
            "clientOrderId": "wire-order-type",
        });
        let parsed = parse_order(&payload)
            .unwrap_or_else(|error| panic!("parse_order({raw}) failed: {error}"));
        assert_eq!(parsed.order_type, expected, "wire orderType for {raw}");
        assert_eq!(
            parsed.to_trade_request().order_type,
            expected,
            "Trd_PlaceOrder.orderType for {raw}"
        );
    }
    // Go's `normalizeExecutionOrderType` accepts LIMIT/MARKET/STOP/STOP_LIMIT
    // only, so BBGO-only aliases stay rejected on the execution route.
    for raw in ["ICEBERG", "LIMIT_MAKER", "TAKE_PROFIT", "TRAILING_STOP_MARKET"] {
        let unsupported = json!({
            "accountId": "1001",
            "market": "US",
            "symbol": "AAPL",
            "side": "BUY",
            "orderType": raw,
            "quantity": 1,
            "price": 100,
            "stopPrice": 99,
            "clientOrderId": "wire-order-type",
        });
        let error = parse_order(&unsupported).expect_err(raw);
        assert!(error.contains("unsupported orderType"), "{raw} error = {error:?}");
    }
}

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
fn combo_intent_rejects_missing_kind_legs_and_account() {
    // Parity: go:452dea11:pkg/futu/advanced_product_adapter_contracts_test.go:150
    // TestFutuComboAdapterErrorPropagationBranches. The Go adapter rejects an
    // empty ComboOrderIntent, a leg whose instrumentId has no market prefix,
    // and a valid intent whose accountId cannot resolve a trade account.
    assert!(parse_combo(&json!({})).is_err(), "empty combo intent");

    let bad_symbol = json!({
        "accountId": "1001",
        "market": "US",
        "clientOrderId": "combo-bad-symbol",
        "orderKind": "option_combo",
        "productClass": "option",
        "underlyingInstrumentId": "US.AAPL",
        "optionStrategy": "vertical",
        "nearExpiry": "2026-07-17",
        "spread": 10,
        "legs": [
            {"instrumentId": "BAD", "productClass": "option", "side": "BUY", "ratio": 1},
            {
                "instrumentId": "US.AAPL260717C00210000",
                "productClass": "option",
                "side": "SELL",
                "ratio": 1
            }
        ]
    });
    let error = parse_combo(&bad_symbol).expect_err("unqualified option leg must be rejected");
    assert!(
        error.contains("MARKET.CODE"),
        "unqualified leg error = {error:?}"
    );

    let missing_account = json!({
        "market": "US",
        "clientOrderId": "combo-missing-account",
        "orderKind": "option_combo",
        "productClass": "option",
        "underlyingInstrumentId": "US.AAPL",
        "optionStrategy": "vertical",
        "nearExpiry": "2026-07-17",
        "spread": 10,
        "legs": [
            {"instrumentId": "US.ONE", "side": "BUY", "ratio": 1},
            {"instrumentId": "US.TWO", "side": "SELL", "ratio": 1}
        ]
    });
    let error = parse_combo(&missing_account).expect_err("accountId is required");
    assert!(error.contains("accountId"), "error = {error:?}");
}

#[test]
fn event_single_rejects_negative_amount_and_invalid_prediction_side() {
    // Parity: go:452dea11:pkg/futu/advanced_product_adapter_contracts_test.go:403
    // TestFutuTradeProductRequestAndReadLifecycleBranches. A negative
    // event amount and a "MAYBE" prediction side fail before the request is
    // built; a valid YES contract maps to Futu's PredSide_Yes (1).
    let base = |amount: Value, side: &str| {
        json!({
            "accountId": "1001",
            "market": "US",
            "symbol": "US.EVENT",
            "side": "BUY",
            "orderType": "LIMIT",
            "quantity": 1,
            "price": 0.6,
            "clientOrderId": "event-1",
            "orderKind": "event_single",
            "productClass": "event_contract",
            "amount": amount,
            "predictionSide": side,
        })
    };
    let error = parse_order(&base(json!(-1.0), "YES")).expect_err("negative amount");
    assert!(error.contains("quantity must be positive"), "error = {error:?}");

    let error = parse_order(&base(json!(20.0), "MAYBE")).expect_err("invalid prediction side");
    assert!(error.contains("predictionSide"), "error = {error:?}");

    let parsed = parse_order(&base(json!(20.0), "YES")).expect("valid event contract");
    assert_eq!(parsed.prediction_side, Some(1));
    assert_eq!(parsed.amount, Some(20.0));
    assert_eq!(parsed.order_kind, "event_single");
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

#[test]
fn test_normalize_execution_order_defaults_us_limit_order() {
    // Parity: internal/trading/execution_test.go:16 TestNormalizeExecutionOrderDefaultsUSLimitOrder
    let payload = json!({
        "accountId": "1001",
        "market": "us",
        "symbol": "aapl",
        "side": "buy",
        "quantity": 10,
        "price": 123.45,
        "brokerId": "test-broker",
    });
    let order = parse_order(&payload).expect("default US limit order");
    assert_eq!(order.broker_id, "test-broker");
    assert_eq!(order.market, "US");
    assert_eq!(order.symbol, "US.AAPL");
    assert_eq!(order.side, 1); // BUY
    assert_eq!(order.order_type, 1); // LIMIT
    assert_eq!(order.header.trd_env, 0); // SIMULATE default
    // Trade market codes are OpenD's TrdMarket enum (US = 2), not the quote
    // market enum (11) used on the read side.
    assert_eq!(order.header.trd_market, 2); // US trade market
    assert_eq!(order.time_in_force, Some(0)); // DAY
    assert_eq!(order.session, Some(1)); // RTH
    assert_eq!(order.fill_outside_rth, Some(false));
}

#[test]
fn test_normalize_execution_order_supports_extended_us_limit_sessions() {
    // Parity: internal/trading/execution_test.go:47 TestNormalizeExecutionOrderSupportsExtendedUSLimitSessions
    let payload = json!({
        "accountId": "1001",
        "market": "US",
        "symbol": "AAPL",
        "side": "SELL",
        "orderType": "LIMIT",
        "session": "ETH",
        "quantity": 5,
        "price": 88.0,
    });
    let order = parse_order(&payload).expect("ETH limit order");
    assert_eq!(order.side, 2); // SELL
    assert_eq!(order.session, Some(2)); // ETH
    assert_eq!(order.fill_outside_rth, Some(true));
}

#[test]
fn test_normalize_execution_order_supports_stop_and_market_orders() {
    // Parity: internal/trading/execution_test.go:71 TestNormalizeExecutionOrderSupportsStopAndMarketOrders
    let stop = json!({
        "accountId": "1001",
        "market": "US",
        "symbol": "AAPL",
        "side": "SELL",
        "orderType": "STOP",
        "quantity": 4,
        "stopPrice": 97.5,
    });
    let stop_order = parse_order(&stop).expect("stop order");
    assert_eq!(stop_order.order_type, 10); // OpenD Stop
    assert_eq!(stop_order.stop_price, Some(97.5));

    let market = json!({
        "accountId": "1001",
        "market": "HK",
        "symbol": "00700",
        "side": "BUY",
        "orderType": "MARKET",
        "quantity": 100,
    });
    let market_order = parse_order(&market).expect("market order");
    assert_eq!(market_order.order_type, 2); // OpenD Market
    assert_eq!(market_order.market, "HK");
    assert_eq!(market_order.session, None);
}

#[test]
fn test_normalize_execution_order_rejects_business_rule_violations() {
    // Parity: internal/trading/execution_test.go:96 TestNormalizeExecutionOrderRejectsBusinessRuleViolations
    let missing_price = json!({
        "accountId": "1001",
        "market": "US",
        "symbol": "AAPL",
        "side": "BUY",
        "quantity": 1,
    });
    let error = parse_order(&missing_price).expect_err("limit order requires price");
    assert!(error.contains("requires price"), "error = {error:?}");

    let missing_stop = json!({
        "accountId": "1001",
        "market": "US",
        "symbol": "AAPL",
        "side": "BUY",
        "orderType": "STOP_LIMIT",
        "quantity": 1,
        "price": 10.0,
    });
    let error = parse_order(&missing_stop).expect_err("stop limit requires stopPrice");
    assert!(error.contains("stopPrice"), "error = {error:?}");

    let hk_session = json!({
        "accountId": "1001",
        "market": "HK",
        "symbol": "00700",
        "side": "BUY",
        "quantity": 1,
        "orderType": "MARKET",
        "session": "ETH",
    });
    let error = parse_order(&hk_session).expect_err("HK session must be rejected");
    assert!(
        error.contains("US market orders only"),
        "error = {error:?}"
    );

    let fok = json!({
        "accountId": "1001",
        "market": "US",
        "symbol": "AAPL",
        "side": "BUY",
        "quantity": 1,
        "price": 10.0,
        "timeInForce": "FOK",
    });
    let error = parse_order(&fok).expect_err("FOK must be rejected");
    assert!(
        error.to_ascii_lowercase().contains("timeinforce")
            && error.to_ascii_uppercase().contains("FOK"),
        "error = {error:?}"
    );

    let stop_limit = json!({
        "accountId": "1001",
        "market": "US",
        "symbol": "AAPL",
        "side": "BUY",
        "quantity": 1,
        "orderType": "STOP_LIMIT",
        "price": 10.0,
        "stopPrice": 9.0,
    });
    parse_order(&stop_limit).expect("stop limit with both prices is valid");
}

#[test]
fn test_normalize_execution_order_preserves_broker_abstraction() {
    // Parity: internal/trading/execution_test.go:159 TestNormalizeExecutionOrderPreservesBrokerAbstraction
    let payload = json!({
        "accountId": "1001",
        "brokerId": "ib",
        "market": "US",
        "symbol": "AAPL",
        "side": "BUY",
        "quantity": 1,
        "price": 10.0,
    });
    let order = parse_order(&payload).expect("broker-selected order");
    assert_eq!(order.broker_id, "ib");
}

#[test]
fn test_futu_security_from_symbol_uses_market_parser() {
    // Parity: pkg/futu/exchange_test.go:152 TestFutuSecurityFromSymbolUsesMarketParser
    use super::execution_order_parse::normalize_instrument;

    for (raw, want_market, want_code, want_canonical) in [
        ("HK.00700", "HK", "00700", "HK.00700"),
        ("HK:00700", "HK", "00700", "HK.00700"),
        ("US.AAPL", "US", "AAPL", "US.AAPL"),
        ("SH.600519", "CN", "600519", "SH.600519"),
        ("SZ.000001", "CN", "000001", "SZ.000001"),
    ] {
        let (market, canonical, code) = normalize_instrument(None, raw, None)
            .unwrap_or_else(|error| panic!("{raw}: {error}"));
        assert_eq!(market, want_market, "market for {raw}");
        assert_eq!(canonical, want_canonical, "canonical for {raw}");
        assert_eq!(code, want_code, "code for {raw}");
    }

    // `CN` is the aggregate SH/SZ market; it must not resolve on its own,
    // matching Go's "requires an exchange-qualified symbol" error.
    let error = normalize_instrument(None, "CN.600519", None)
        .expect_err("aggregate CN prefix must fail closed");
    assert!(
        error.contains("exchange-qualified"),
        "error = {error:?}"
    );
}
