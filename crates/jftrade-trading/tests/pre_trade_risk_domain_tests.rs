use jftrade_kernel::Fixed8;
use jftrade_trading::{
    HardStop, PreTradeRiskOrder, PreTradeRiskPolicy, TradingEnvironment, evaluate_pre_trade_risk,
};
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
        quantity: Fixed8::from_str("10").unwrap(),
        price: Some(Fixed8::from_str("150").unwrap()),
        amount: None,
        legs: Vec::new(),
    }
}

fn valid_policy() -> PreTradeRiskPolicy {
    PreTradeRiskPolicy {
        control_plane_available: true,
        real_trading_enabled: true,
        kill_switch_active: false,
        effective_max_order_quantity: Some(Fixed8::from_str("100").unwrap()),
        effective_max_order_notional: Some(Fixed8::from_str("50000").unwrap()),
        hard_stops: Vec::new(),
    }
}

#[test]
fn pre_trade_risk_rejects_non_positive_quantity_in_units_mode() {
    let policy = valid_policy();
    let mut order = test_order(TradingEnvironment::Simulate);
    order.quantity = Fixed8::ZERO;
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("INVALID_ORDER_RISK_SHAPE")
    );
}

#[test]
fn pre_trade_risk_rejects_missing_or_negative_amount_in_amount_mode() {
    let policy = valid_policy();
    let mut order = test_order(TradingEnvironment::Simulate);
    order.quantity_mode = "amount".to_owned();
    order.amount = None;
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("INVALID_ORDER_RISK_SHAPE")
    );

    order.amount = Some(Fixed8::ZERO);
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("INVALID_ORDER_RISK_SHAPE")
    );
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
    policy.effective_max_order_quantity = Some(Fixed8::from_str("5").unwrap());
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
    policy.effective_max_order_notional = Some(Fixed8::from_str("1000").unwrap());
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
    policy.effective_max_order_notional = Some(Fixed8::from_str("5000").unwrap());
    let mut order = test_order(TradingEnvironment::Real);
    order.product_class = "option".to_owned();
    order.quantity = Fixed8::from_str("1").unwrap();
    order.price = Some(Fixed8::from_str("60").unwrap());
    // 1 contract * $60 * 100 multiplier = $6,000 > $5,000 limit
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("MAX_ORDER_NOTIONAL_EXCEEDED")
    );

    // 1 contract * $40 * 100 multiplier = $4,000 <= $5,000 limit
    order.price = Some(Fixed8::from_str("40").unwrap());
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(decision.allowed);
}

#[test]
fn pre_trade_risk_enforces_combo_multi_leg_notional_and_hard_stops() {
    use jftrade_trading::PreTradeRiskComboLeg;

    let mut policy = valid_policy();
    policy.effective_max_order_notional = Some(Fixed8::from_str("10000").unwrap());
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
            quantity: Fixed8::from_str("1").unwrap(),
            multiplier: Fixed8::from_str("100").unwrap(),
            price: Some(Fixed8::from_str("40").unwrap()), // $4,000
            product_class: "option".to_owned(),
        },
        PreTradeRiskComboLeg {
            symbol: "NVDA".to_owned(),
            market: "US".to_owned(),
            side: "SELL".to_owned(),
            quantity: Fixed8::from_str("2").unwrap(),
            multiplier: Fixed8::from_str("100").unwrap(),
            price: Some(Fixed8::from_str("35").unwrap()), // $7,000
            product_class: "option".to_owned(),
        },
    ];

    // Directional net notional is |4000 - 7000| = 3000 <= 10000 limit -> Allowed
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(decision.allowed);

    // Directional net notional exceeding limit: BUY $20,000, SELL $7,000 -> Net $13,000 > $10,000 limit
    order.legs[0].price = Some(Fixed8::from_str("200").unwrap());
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("MAX_ORDER_NOTIONAL_EXCEEDED")
    );

    // Leg price unavailable with NO combo price fails closed
    order.legs[0].price = Some(Fixed8::from_str("40").unwrap());
    order.legs[1].price = None;
    order.price = None;
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("RISK_PRICE_UNAVAILABLE")
    );

    // Leg price unavailable WITH combo price uses combo price ($50 * 1 contract * 100 = $5,000 <= $10,000 limit)
    order.quantity = Fixed8::from_str("1").unwrap();
    order.price = Some(Fixed8::from_str("50").unwrap());
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(decision.allowed);

    // Leg matching hard stop rejects combo
    order.legs[1].price = Some(Fixed8::from_str("10").unwrap());
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
    policy.effective_max_order_notional = Some(Fixed8::from_str("10000").unwrap());
    policy.effective_max_order_quantity = Some(Fixed8::from_str("50").unwrap());

    let mut order = test_order(TradingEnvironment::Real);
    order.order_kind = "combo".to_owned();
    order.quantity_mode = "amount".to_owned();
    order.amount = Some(Fixed8::from_str("8000").unwrap());
    order.price = None;
    order.legs = vec![
        PreTradeRiskComboLeg {
            symbol: "AAPL".to_owned(),
            market: "US".to_owned(),
            side: "BUY".to_owned(),
            quantity: Fixed8::ZERO,
            multiplier: Fixed8::from_str("100").unwrap(),
            price: None, // No price required in amount mode
            product_class: "option".to_owned(),
        },
        PreTradeRiskComboLeg {
            symbol: "NVDA".to_owned(),
            market: "US".to_owned(),
            side: "SELL".to_owned(),
            quantity: Fixed8::ZERO,
            multiplier: Fixed8::from_str("100").unwrap(),
            price: None, // No price required in amount mode
            product_class: "option".to_owned(),
        },
    ];

    // Amount $8,000 <= $10,000 limit -> Allowed even with 0 qty and no leg prices
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(decision.allowed);

    // Amount $12,000 > $10,000 limit -> Rejects on notional limit
    order.amount = Some(Fixed8::from_str("12000").unwrap());
    let decision = evaluate_pre_trade_risk(&policy, &order);
    assert!(!decision.allowed);
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("MAX_ORDER_NOTIONAL_EXCEEDED")
    );
}
