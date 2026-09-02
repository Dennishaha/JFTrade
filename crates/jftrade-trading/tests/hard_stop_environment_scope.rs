use std::str::FromStr;

use jftrade_kernel::Fixed8;
use jftrade_trading::{
    HardStop, OrderCommand, OrderSide, RiskConfig, RiskEngine, TradingEnvironment,
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
        quantity: Fixed8::from_str("1").expect("quantity"),
        price: Some(Fixed8::from_str("10").expect("price")),
        client_order_id: "client-1".to_owned(),
    }
}

fn engine(environment: Option<&str>) -> RiskEngine {
    RiskEngine::new(RiskConfig {
        real_trading_enabled: true,
        kill_switch_active: false,
        max_order_quantity: None,
        max_order_notional: None,
        hard_stops: vec![HardStop {
            broker_id: Some("futu".to_owned()),
            trading_environment: environment.map(str::to_owned),
            account_id: Some("acc-1".to_owned()),
            market: Some("US".to_owned()),
            symbol: Some("AAPL".to_owned()),
        }],
    })
}

#[test]
fn hard_stop_environment_scope_blocks_only_matching_real_commands() {
    let real = engine(Some("REAL"));
    assert_eq!(
        real.evaluate(&command(TradingEnvironment::Real))
            .reason_code
            .as_deref(),
        Some("REAL_TRADE_HARD_STOP_ACTIVE")
    );

    let simulate_scoped = engine(Some("SIMULATE"));
    assert!(
        simulate_scoped
            .evaluate(&command(TradingEnvironment::Real))
            .allowed,
        "a hard stop scoped to SIMULATE must not block REAL"
    );
}

#[test]
fn hard_stop_environment_scope_trims_case_and_supports_wildcards() {
    for scope in [Some(" real "), Some("*")] {
        assert_eq!(
            engine(scope)
                .evaluate(&command(TradingEnvironment::Real))
                .reason_code
                .as_deref(),
            Some("REAL_TRADE_HARD_STOP_ACTIVE"),
            "scope {scope:?} should match REAL"
        );
    }

    assert_eq!(
        engine(Some(" "))
            .evaluate(&command(TradingEnvironment::Real))
            .reason_code
            .as_deref(),
        Some("REAL_TRADE_HARD_STOP_ACTIVE"),
        "blank scope should match either environment"
    );
}
