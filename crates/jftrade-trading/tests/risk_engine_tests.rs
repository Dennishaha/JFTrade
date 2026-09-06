use std::str::FromStr;

use jftrade_kernel::Fixed8;
use jftrade_trading::{
    OrderCommand, OrderSide, RUNTIME_RISK_MODE_ENFORCE, RUNTIME_RISK_MODE_MONITOR,
    RUNTIME_RISK_MODE_OFF, RiskConfig, RiskEngine, RuntimeRiskContext, RuntimeRiskOrder,
    RuntimeRiskSettings, TradingEnvironment, evaluate_runtime_risk,
};

fn command(environment: TradingEnvironment) -> OrderCommand {
    OrderCommand {
        idempotency_key: "key-1".to_owned(),
        trace_id: "trace-1".to_owned(),
        broker_id: "futu".to_owned(),
        account_id: "acc-1".to_owned(),
        environment,
        market: "US".to_owned(),
        symbol: "AAPL".to_owned(),
        side: OrderSide::Buy,
        quantity: Fixed8::from_str("11").expect("quantity"),
        price: Some(Fixed8::from_str("10").expect("price")),
        client_order_id: "client-1".to_owned(),
    }
}

#[test]
fn simulate_bypasses_real_trade_controls_but_real_fails_closed() {
    let engine = RiskEngine::new(RiskConfig {
        real_trading_enabled: false,
        kill_switch_active: true,
        max_order_quantity: None,
        max_order_notional: None,
        hard_stops: Vec::new(),
    });
    assert!(
        engine
            .evaluate(&command(TradingEnvironment::Simulate))
            .allowed
    );
    assert_eq!(
        engine
            .evaluate(&command(TradingEnvironment::Real))
            .reason_code
            .as_deref(),
        Some("REAL_TRADING_DISABLED")
    );
}

#[test]
fn real_notional_uses_fixed_point_arithmetic() {
    let engine = RiskEngine::new(RiskConfig {
        real_trading_enabled: true,
        kill_switch_active: false,
        max_order_quantity: Some(Fixed8::from_str("20").expect("maximum quantity")),
        max_order_notional: Some(Fixed8::from_str("100").expect("maximum notional")),
        hard_stops: Vec::new(),
    });
    assert_eq!(
        engine
            .evaluate(&command(TradingEnvironment::Real))
            .reason_code
            .as_deref(),
        Some("MAX_ORDER_NOTIONAL_EXCEEDED")
    );
}

fn runtime_order(side: &str, quantity: &str, price: Option<&str>) -> RuntimeRiskOrder {
    RuntimeRiskOrder {
        symbol: "US.AAPL".to_owned(),
        side: side.to_owned(),
        quantity: quantity.parse().expect("quantity"),
        price: price.map(|value| value.parse().expect("price")),
    }
}

fn runtime_context(sellable: &str, current_price: Option<&str>, count: i64) -> RuntimeRiskContext {
    RuntimeRiskContext {
        current_price: current_price.map(|value| value.parse().expect("current price")),
        sellable_quantity: sellable.parse().expect("sellable quantity"),
        today_submitted_order_count: count,
    }
}

#[test]
fn runtime_risk_normalizes_modes_and_clears_off_limits() {
    let normalized = RuntimeRiskSettings {
        mode: " unknown ".to_owned(),
        close_only: true,
        max_order_quantity: Some("-1".parse().expect("quantity")),
        max_order_notional: Some(Fixed8::POS_INFINITY),
        daily_max_orders: Some(0),
        pause_on_reject: true,
    }
    .normalize();
    assert_eq!(normalized.mode, RUNTIME_RISK_MODE_OFF);
    assert!(!normalized.close_only);
    assert!(normalized.max_order_quantity.is_none());
    assert!(normalized.max_order_notional.is_none());
    assert!(normalized.daily_max_orders.is_none());
    assert!(!normalized.pause_on_reject);

    assert_eq!(
        RuntimeRiskSettings {
            mode: " MONITOR ".to_owned(),
            ..RuntimeRiskSettings::default()
        }
        .normalize()
        .mode,
        RUNTIME_RISK_MODE_MONITOR
    );
}

#[test]
fn runtime_risk_off_ignores_configured_limits() {
    let decision = evaluate_runtime_risk(
        RuntimeRiskSettings {
            mode: RUNTIME_RISK_MODE_OFF.to_owned(),
            max_order_quantity: Some("1".parse().expect("quantity")),
            ..RuntimeRiskSettings::default()
        },
        &runtime_order("BUY", "10", None),
        &RuntimeRiskContext::default(),
    );
    assert_eq!(decision, jftrade_trading::RuntimeRiskDecision::default());
}

#[test]
fn runtime_risk_enforce_applies_close_only_quantity_notional_and_daily_limits() {
    let settings = RuntimeRiskSettings {
        mode: RUNTIME_RISK_MODE_ENFORCE.to_owned(),
        close_only: true,
        max_order_quantity: Some("5".parse().expect("quantity")),
        max_order_notional: Some("500".parse().expect("notional")),
        daily_max_orders: Some(3),
        pause_on_reject: true,
    };
    let cases = [
        ("BUY", "1", None, "close_only"),
        ("SELL", "5", None, "close_only_insufficient_position"),
        ("SELL", "4", Some("130"), "max_order_notional"),
        ("SELL", "4", Some("100"), ""),
    ];
    for (side, quantity, price, reason) in cases {
        let decision = evaluate_runtime_risk(
            settings.clone(),
            &runtime_order(side, quantity, price),
            &runtime_context("4", Some("100"), 0),
        );
        assert_eq!(decision.reason.as_deref().unwrap_or(""), reason);
        if reason.is_empty() {
            assert_eq!(decision, jftrade_trading::RuntimeRiskDecision::default());
        } else {
            assert!(decision.matched && decision.rejected && decision.pause_on_reject);
        }
    }

    let quantity = evaluate_runtime_risk(
        RuntimeRiskSettings {
            close_only: false,
            ..settings.clone()
        },
        &runtime_order("BUY", "6", None),
        &runtime_context("4", Some("100"), 0),
    );
    assert_eq!(quantity.reason.as_deref(), Some("max_order_quantity"));

    let daily = evaluate_runtime_risk(
        RuntimeRiskSettings {
            close_only: false,
            max_order_quantity: None,
            max_order_notional: None,
            daily_max_orders: Some(3),
            ..settings
        },
        &runtime_order("BUY", "1", None),
        &runtime_context("0", None, 3),
    );
    assert_eq!(daily.reason.as_deref(), Some("daily_max_orders"));
}

#[test]
fn runtime_risk_monitor_records_match_without_rejecting_and_uses_fallback_price() {
    let decision = evaluate_runtime_risk(
        RuntimeRiskSettings {
            mode: RUNTIME_RISK_MODE_MONITOR.to_owned(),
            max_order_notional: Some("100".parse().expect("notional")),
            ..RuntimeRiskSettings::default()
        },
        &runtime_order("BUY", "2", None),
        &runtime_context("0", Some("60"), 0),
    );
    assert!(decision.matched);
    assert!(!decision.rejected);
    assert!(!decision.pause_on_reject);
    assert_eq!(decision.reason.as_deref(), Some("max_order_notional"));
    assert!(
        decision
            .detail
            .as_deref()
            .is_some_and(|detail| detail.starts_with("rule=max_order_notional"))
    );
}

#[test]
fn runtime_order_can_be_derived_from_canonical_order_command() {
    let command = command(TradingEnvironment::Real);
    let order = RuntimeRiskOrder::from_command(&command);
    assert_eq!(order.symbol, "AAPL");
    assert_eq!(order.side, "BUY");
    assert_eq!(order.quantity, command.quantity);
    assert_eq!(order.price, command.price);
}
