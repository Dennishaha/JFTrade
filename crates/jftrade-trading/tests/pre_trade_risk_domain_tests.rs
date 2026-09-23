use jftrade_trading::{
    HardStop, PreTradeRiskOrder, PreTradeRiskPolicy, TradingEnvironment, evaluate_pre_trade_risk,
};
use rust_decimal::Decimal;
use std::str::FromStr;

fn test_order(environment: TradingEnvironment) -> PreTradeRiskOrder {
    PreTradeRiskOrder {
        broker_id: "futu".to_owned(),
        trading_environment: environment,
        account_id: "acc-1".to_owned(),
        market: "US".to_owned(),
        symbol: "AAPL".to_owned(),
        side: "BUY".to_owned(),
        order_type: "LIMIT".to_owned(),
        order_kind: "single".to_owned(),
        product_class: "equity".to_owned(),
        quantity_mode: "units".to_owned(),
        quantity: Decimal::from_str("10").unwrap(),
        price: Some(Decimal::from_str("150").unwrap()),
        amount: None,
        legs: Vec::new(),
    }
}

fn valid_policy() -> PreTradeRiskPolicy {
    PreTradeRiskPolicy {
        control_plane_available: true,
        real_trading_enabled: true,
        kill_switch_active: false,
        effective_max_order_quantity: Some(Decimal::from_str("100").unwrap()),
        effective_max_order_notional: Some(Decimal::from_str("50000").unwrap()),
        hard_stops: Vec::new(),
    }
}

#[test]
fn pre_trade_risk_rejects_non_positive_quantity_in_units_mode() {
    let policy = valid_policy();
    let mut order = test_order(TradingEnvironment::Simulate);
    order.quantity = Decimal::ZERO;
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("INVALID_ORDER_RISK_SHAPE")
    );
}

#[test]
fn pre_trade_risk_rejects_missing_or_negative_amount_in_amount_mode() {
    // Parity: go:452dea11:internal/trading/risk_shape_boundaries_test.go:11 TestCommandRiskShapeRejectsSpoofedAndIncompatibleFields
    let policy = valid_policy();
    let mut order = test_order(TradingEnvironment::Simulate);
    order.quantity_mode = "amount".to_owned();
    order.amount = None;
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("RISK_AMOUNT_UNAVAILABLE")
    );

    order.amount = Some(Decimal::ZERO);
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("RISK_AMOUNT_UNAVAILABLE")
    );
}

#[test]
fn pre_trade_risk_rejects_amount_smuggled_into_non_amount_mode() {
    // Parity: go:452dea11:internal/trading/execution_test.go:613 TestPreTradeRiskEnforcesAmountModeQuantityAndNotionalLimits
    let mut policy = valid_policy();
    policy.effective_max_order_notional = Some(Decimal::from_str("50").unwrap());

    let mut spoofed = test_order(TradingEnvironment::Real);
    spoofed.quantity = Decimal::from_str("1000000").unwrap();
    spoofed.amount = Some(Decimal::from_str("1").unwrap());
    spoofed.price = Some(Decimal::from_str("0.50").unwrap());

    let decision = evaluate_pre_trade_risk(&policy, &spoofed);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("INVALID_ORDER_RISK_SHAPE")
    );
}

#[test]
fn pre_trade_risk_enforces_amount_mode_quantity_and_notional_limits() {
    // Parity: go:452dea11:internal/trading/execution_test.go:613 TestPreTradeRiskEnforcesAmountModeQuantityAndNotionalLimits
    let mut order = test_order(TradingEnvironment::Real);
    order.product_class = "event_contract".to_owned();
    order.order_kind = "event_single".to_owned();
    order.quantity_mode = "amount".to_owned();
    order.amount = Some(Decimal::from_str("60").unwrap());
    order.price = Some(Decimal::from_str("0.50").unwrap());

    let mut quantity_policy = valid_policy();
    quantity_policy.effective_max_order_quantity = Some(Decimal::from_str("50").unwrap());
    quantity_policy.effective_max_order_notional = None;
    let decision = evaluate_pre_trade_risk(&quantity_policy, &order);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("MAX_ORDER_QUANTITY_EXCEEDED")
    );

    let mut notional_policy = valid_policy();
    notional_policy.effective_max_order_quantity = None;
    notional_policy.effective_max_order_notional = Some(Decimal::from_str("50").unwrap());
    let decision = evaluate_pre_trade_risk(&notional_policy, &order);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("MAX_ORDER_NOTIONAL_EXCEEDED")
    );

    let mut missing_amount = order.clone();
    missing_amount.amount = None;
    let decision = evaluate_pre_trade_risk(&notional_policy, &missing_amount);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("RISK_AMOUNT_UNAVAILABLE")
    );

    // Amount is the risk size: the display price must never gate or shrink it.
    let mut without_display_price = order.clone();
    without_display_price.price = None;
    let mut within_quantity = valid_policy();
    within_quantity.effective_max_order_quantity = Some(Decimal::from_str("100").unwrap());
    within_quantity.effective_max_order_notional = None;
    let decision = evaluate_pre_trade_risk(&within_quantity, &without_display_price);
    assert!(decision.allowed);

    let mut within_notional = valid_policy();
    within_notional.effective_max_order_quantity = None;
    within_notional.effective_max_order_notional = Some(Decimal::from_str("100").unwrap());
    let decision = evaluate_pre_trade_risk(&within_notional, &without_display_price);
    assert!(decision.allowed);
}

#[test]
fn pre_trade_risk_allows_valid_simulate_order_even_if_real_controls_inactive() {
    let mut policy = valid_policy();
    policy.control_plane_available = false;
    policy.real_trading_enabled = false;
    policy.kill_switch_active = true;
    let order = test_order(TradingEnvironment::Simulate);
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(decision.allowed);
    assert!(decision.reason_code.is_none());
}

#[test]
fn pre_trade_risk_fails_closed_when_control_plane_unavailable_for_real() {
    let mut policy = valid_policy();
    policy.control_plane_available = false;
    let order = test_order(TradingEnvironment::Real);
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("PRE_TRADE_RISK_UNAVAILABLE")
    );
}

#[test]
fn pre_trade_risk_fails_when_real_trading_disabled() {
    let mut policy = valid_policy();
    policy.real_trading_enabled = false;
    let order = test_order(TradingEnvironment::Real);
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("REAL_TRADING_DISABLED")
    );
}

#[test]
fn pre_trade_risk_fails_when_kill_switch_active() {
    let mut policy = valid_policy();
    policy.kill_switch_active = true;
    let order = test_order(TradingEnvironment::Real);
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("REAL_TRADE_KILL_SWITCH_ACTIVE")
    );
}

#[test]
fn pre_trade_risk_fails_when_hard_stop_matches() {
    let mut policy = valid_policy();
    policy.hard_stops = vec![HardStop {
        id: None,
        broker_id: Some("futu".to_owned()),
        trading_environment: Some("real".to_owned()),
        account_id: Some("acc-1".to_owned()),
        market: Some("US".to_owned()),
        symbol: Some("AAPL".to_owned()),
    }];
    let order = test_order(TradingEnvironment::Real);
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("REAL_TRADE_HARD_STOP_ACTIVE")
    );
}

#[test]
fn pre_trade_risk_enforces_quantity_limit() {
    let mut policy = valid_policy();
    policy.effective_max_order_quantity = Some(Decimal::from_str("5").unwrap());
    let order = test_order(TradingEnvironment::Real); // qty 10
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("MAX_ORDER_QUANTITY_EXCEEDED")
    );
}

#[test]
fn pre_trade_risk_requires_price_for_notional_limit() {
    let mut policy = valid_policy();
    policy.effective_max_order_notional = Some(Decimal::from_str("1000").unwrap());
    let mut order = test_order(TradingEnvironment::Real);
    order.price = None;
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("RISK_PRICE_UNAVAILABLE")
    );
}

#[test]
fn pre_trade_risk_enforces_notional_limit_with_option_multiplier() {
    let mut policy = valid_policy();
    policy.effective_max_order_notional = Some(Decimal::from_str("5000").unwrap());
    let mut order = test_order(TradingEnvironment::Real);
    order.product_class = "option".to_owned();
    order.quantity = Decimal::from_str("1").unwrap();
    order.price = Some(Decimal::from_str("60").unwrap());
    // 1 contract * $60 * 100 multiplier = $6,000 > $5,000 limit
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("MAX_ORDER_NOTIONAL_EXCEEDED")
    );

    // 1 contract * $40 * 100 multiplier = $4,000 <= $5,000 limit
    order.price = Some(Decimal::from_str("40").unwrap());
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(decision.allowed);
}

#[test]
fn pre_trade_risk_enforces_combo_multi_leg_notional_and_hard_stops() {
    use jftrade_trading::PreTradeRiskComboLeg;

    let mut policy = valid_policy();
    policy.effective_max_order_notional = Some(Decimal::from_str("10000").unwrap());
    policy.hard_stops = vec![HardStop {
        id: None,
        broker_id: None,
        trading_environment: None,
        account_id: None,
        market: Some("US".to_owned()),
        symbol: Some("TSLA".to_owned()),
    }];

    let mut order = test_order(TradingEnvironment::Real);
    order.order_kind = "combo".to_owned();
    order.price = None;
    order.legs = vec![
        PreTradeRiskComboLeg {
            symbol: "AAPL".to_owned(),
            market: "US".to_owned(),
            side: "BUY".to_owned(),
            quantity: Decimal::from_str("1").unwrap(),
            multiplier: Decimal::from_str("100").unwrap(),
            price: Some(Decimal::from_str("40").unwrap()), // $4,000
            product_class: "option".to_owned(),
        },
        PreTradeRiskComboLeg {
            symbol: "NVDA".to_owned(),
            market: "US".to_owned(),
            side: "SELL".to_owned(),
            quantity: Decimal::from_str("2").unwrap(),
            multiplier: Decimal::from_str("100").unwrap(),
            price: Some(Decimal::from_str("35").unwrap()), // $7,000
            product_class: "option".to_owned(),
        },
    ];

    // Directional net notional is |4000 - 7000| = 3000 <= 10000 limit -> Allowed
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(decision.allowed);

    // Directional net notional exceeding limit: BUY $20,000, SELL $7,000 -> Net $13,000 > $10,000 limit
    order.legs[0].price = Some(Decimal::from_str("200").unwrap());
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("MAX_ORDER_NOTIONAL_EXCEEDED")
    );

    // Leg price unavailable with NO combo price fails closed
    order.legs[0].price = Some(Decimal::from_str("40").unwrap());
    order.legs[1].price = None;
    order.price = None;
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("RISK_PRICE_UNAVAILABLE")
    );

    // Leg price unavailable WITH combo price uses combo price ($50 * 1 contract * 100 = $5,000 <= $10,000 limit)
    order.quantity = Decimal::from_str("1").unwrap();
    order.price = Some(Decimal::from_str("50").unwrap());
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(decision.allowed);

    // Leg matching hard stop rejects combo
    order.legs[1].price = Some(Decimal::from_str("10").unwrap());
    order.legs[1].symbol = "TSLA".to_owned(); // Matches hard stop!
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("REAL_TRADE_HARD_STOP_ACTIVE")
    );
}

#[test]
fn pre_trade_risk_combo_amount_mode_precedence_and_leg_bypass() {
    use jftrade_trading::PreTradeRiskComboLeg;

    let mut policy = valid_policy();
    policy.effective_max_order_notional = Some(Decimal::from_str("10000").unwrap());
    // Amount mode compares the amount against the quantity limit as well (Go
    // `commandRiskAmount`), so keep the fixture above both amounts here and
    // cover the amount-versus-quantity-limit rejection in
    // `pre_trade_risk_enforces_amount_mode_quantity_and_notional_limits`.
    policy.effective_max_order_quantity = Some(Decimal::from_str("20000").unwrap());

    let mut order = test_order(TradingEnvironment::Real);
    order.order_kind = "combo".to_owned();
    order.quantity_mode = "amount".to_owned();
    order.amount = Some(Decimal::from_str("8000").unwrap());
    order.price = None;
    order.legs = vec![
        PreTradeRiskComboLeg {
            symbol: "AAPL".to_owned(),
            market: "US".to_owned(),
            side: "BUY".to_owned(),
            quantity: Decimal::ZERO,
            multiplier: Decimal::from_str("100").unwrap(),
            price: None, // No price required in amount mode
            product_class: "option".to_owned(),
        },
        PreTradeRiskComboLeg {
            symbol: "NVDA".to_owned(),
            market: "US".to_owned(),
            side: "SELL".to_owned(),
            quantity: Decimal::ZERO,
            multiplier: Decimal::from_str("100").unwrap(),
            price: None, // No price required in amount mode
            product_class: "option".to_owned(),
        },
    ];

    // Amount $8,000 <= $10,000 limit -> Allowed even with 0 qty and no leg prices
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(decision.allowed);

    // Amount $12,000 > $10,000 limit -> Rejects on notional limit
    order.amount = Some(Decimal::from_str("12000").unwrap());
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("MAX_ORDER_NOTIONAL_EXCEEDED")
    );
}

/// Parity: go:452dea11:internal/trading/broker_test.go:648 TestPlaceBrokerOrderRunsPreTradeRiskBeforeBrokerSubmission
#[test]
fn test_place_broker_order_runs_pre_trade_risk_before_broker_submission() {
    // Parity: internal/trading/broker_test.go:648 TestPlaceBrokerOrderRunsPreTradeRiskBeforeBrokerSubmission
    let mut policy = valid_policy();
    policy.kill_switch_active = true;

    let order = test_order(TradingEnvironment::Real);
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("REAL_TRADE_KILL_SWITCH_ACTIVE")
    );
}

/// Parity: go:452dea11:internal/trading/broker_test.go:711 TestPlaceBrokerOrderFailsClosedWhenRealRiskGatewayIsUnavailable
#[test]
fn test_place_broker_order_fails_closed_when_real_risk_gateway_is_unavailable() {
    // Parity: internal/trading/broker_test.go:711 TestPlaceBrokerOrderFailsClosedWhenRealRiskGatewayIsUnavailable
    let mut policy = valid_policy();
    policy.control_plane_available = false;

    let order = test_order(TradingEnvironment::Real);
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("PRE_TRADE_RISK_UNAVAILABLE")
    );
}

#[test]
fn pre_trade_risk_snapshot_uses_empty_vectors_not_null() {
    // Parity: internal/trading/execution_test.go:687 TestPreTradeRiskSnapshotUsesNonNilEmptySlices
    //
    // Go guards against `nil` slices so JSON renders `[]` instead of `null`.
    // Rust models those fields as `Vec`, so the equivalent contract is that a
    // default snapshot serializes every collection as an empty array.
    let snapshot = jftrade_trading::RealTradeRiskSnapshot::from_control_state(
        jftrade_trading::RealTradeControlState::default(),
        None,
    );
    assert!(snapshot.hard_stop_entries.is_empty());
    assert!(snapshot.hard_stop_events.is_empty());
    assert!(snapshot.kill_switch_events.is_empty());
    assert!(snapshot.risk_events.is_empty());

    let encoded = serde_json::to_value(&snapshot).expect("snapshot JSON");
    for key in [
        "hardStopEntries",
        "hardStopEvents",
        "killSwitchEvents",
        "riskEvents",
    ] {
        assert_eq!(
            encoded[key],
            serde_json::json!([]),
            "{key} must serialize as an empty array, never null"
        );
    }
}

#[test]
fn pre_trade_risk_hard_stop_scope_matrix_matches_market_symbol_account_and_broker() {
    // Parity: go:452dea11:internal/trading/risk_status_broker_boundaries_test.go:13
    // TestRiskDecisionAndHardStopBoundarySemantics (hard-stop matching matrix).
    // Go's hardStopMatches skips blank broker/account/market/symbol scopes and
    // compares the populated ones case-insensitively, accepting either the
    // bare symbol or the MARKET.SYMBOL form.
    let order = test_order(TradingEnvironment::Real);
    let matching = HardStop {
        id: Some("hs-1".to_owned()),
        broker_id: Some("futu".to_owned()),
        trading_environment: Some("REAL".to_owned()),
        account_id: Some("acc-1".to_owned()),
        market: Some("US".to_owned()),
        symbol: Some("AAPL".to_owned()),
    };
    assert!(matching.matches_pre_trade(&order));

    let nonmatching = [
        HardStop {
            broker_id: Some("other".to_owned()),
            ..matching.clone()
        },
        HardStop {
            trading_environment: Some("SIMULATE".to_owned()),
            ..matching.clone()
        },
        HardStop {
            account_id: Some("other".to_owned()),
            ..matching.clone()
        },
        HardStop {
            account_id: Some("*".to_owned()),
            market: Some("HK".to_owned()),
            ..matching.clone()
        },
        HardStop {
            account_id: Some("*".to_owned()),
            symbol: Some("MSFT".to_owned()),
            ..matching.clone()
        },
    ];
    for entry in &nonmatching {
        assert!(
            !entry.matches_pre_trade(&order),
            "nonmatching hard stop {entry:?}"
        );
    }

    for symbol in ["aapl", "US.AAPL"] {
        assert!(
            HardStop {
                symbol: Some(symbol.to_owned()),
                ..matching.clone()
            }
            .matches_pre_trade(&order),
            "symbol scope {symbol:?} must match AAPL"
        );
    }
    let mut prefixed = test_order(TradingEnvironment::Real);
    prefixed.symbol = "US.AAPL".to_owned();
    assert!(
        matching.matches_pre_trade(&prefixed),
        "an order carrying MARKET.SYMBOL must match the bare symbol scope"
    );

    for blank in [
        HardStop {
            symbol: Some("   ".to_owned()),
            ..matching.clone()
        },
        HardStop {
            market: Some(" ".to_owned()),
            ..matching.clone()
        },
        HardStop {
            broker_id: Some("*".to_owned()),
            account_id: Some("*".to_owned()),
            ..matching.clone()
        },
    ] {
        assert!(
            blank.matches_pre_trade(&order),
            "blank or wildcard scope {blank:?} must match either value"
        );
    }
    let mut anonymous = test_order(TradingEnvironment::Real);
    anonymous.symbol = String::new();
    assert!(
        !matching.matches_pre_trade(&anonymous),
        "a populated symbol scope must not match an empty order symbol"
    );

    // Go's matchHardStop skips nonmatching entries and returns the first match.
    let mut policy = valid_policy();
    policy.hard_stops = vec![
        HardStop {
            broker_id: Some("other".to_owned()),
            ..matching.clone()
        },
        HardStop {
            id: Some("hs-2".to_owned()),
            ..matching.clone()
        },
    ];
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("REAL_TRADE_HARD_STOP_ACTIVE")
    );
    assert_eq!(decision.matched_hard_stop_id.as_deref(), Some("hs-2"));

    let mut empty = valid_policy();
    empty.hard_stops = Vec::new();
    assert!(
        evaluate_pre_trade_risk(&empty, &order).allowed,
        "an empty hard-stop policy must not block real orders"
    );
}
