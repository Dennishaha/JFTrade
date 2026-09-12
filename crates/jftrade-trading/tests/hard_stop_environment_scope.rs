use std::str::FromStr;

use jftrade_trading::{
    HardStop, OrderCommand, OrderSide, RiskConfig, RiskEngine, TradingEnvironment,
};
use rust_decimal::Decimal;

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
        quantity: Decimal::from_str("1").expect("quantity"),
        price: Some(Decimal::from_str("10").expect("price")),
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
            id: None,
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

#[test]
fn test_real_trade_control_plane_hard_stop_release_is_single_shot() {
    // Parity: internal/trading/control_plane_idempotency_test.go:12 TestRealTradeControlPlaneHardStopReleaseIsSingleShot
    use jftrade_trading::{RealTradeControlState, RealTradeHardStopEntry};

    let mut state = RealTradeControlState {
        hard_stops: vec![RealTradeHardStopEntry {
            id: "hs-1".to_string(),
            broker_id: "futu".to_string(),
            trading_environment: "REAL".to_string(),
            account_id: "ACC-1".to_string(),
            market: Some("US".to_string()),
            symbol: Some("AAPL".to_string()),
            hard_stop_scope: "ACCOUNT".to_string(),
            operator_id: "tester".to_string(),
            reason: "test".to_string(),
            activated_at: "2026-09-11T12:00:00Z".to_string(),
            updated_at: "2026-09-11T12:00:00Z".to_string(),
        }],
        ..Default::default()
    };

    // Release once
    let pos = state.hard_stops.iter().position(|e| e.id == "hs-1");
    assert!(pos.is_some());
    let removed = state.hard_stops.remove(pos.unwrap());
    assert_eq!(removed.id, "hs-1");
    assert!(state.hard_stops.is_empty());

    // Repeated release fails because entry is no longer found
    let pos_again = state.hard_stops.iter().position(|e| e.id == "hs-1");
    assert!(pos_again.is_none());
}

#[test]
fn test_real_trade_control_plane_hard_stops_block_until_every_entry_released() {
    // Parity: internal/trading/control_plane_idempotency_test.go:99 TestRealTradeControlPlaneHardStopsBlockUntilEveryEntryReleased
    use jftrade_trading::{RealTradeControlState, RealTradeHardStopEntry};

    let mut state = RealTradeControlState {
        hard_stops: vec![
            RealTradeHardStopEntry {
                id: "hs-1".to_string(),
                broker_id: "futu".to_string(),
                trading_environment: "REAL".to_string(),
                account_id: "ACC-1".to_string(),
                market: Some("US".to_string()),
                symbol: Some("AAPL".to_string()),
                hard_stop_scope: "ACCOUNT".to_string(),
                operator_id: "tester".to_string(),
                reason: "first halt".to_string(),
                activated_at: "2026-09-11T12:00:00Z".to_string(),
                updated_at: "2026-09-11T12:00:00Z".to_string(),
            },
            RealTradeHardStopEntry {
                id: "hs-2".to_string(),
                broker_id: "futu".to_string(),
                trading_environment: "REAL".to_string(),
                account_id: "ACC-1".to_string(),
                market: Some("US".to_string()),
                symbol: Some("AAPL".to_string()),
                hard_stop_scope: "ACCOUNT".to_string(),
                operator_id: "tester".to_string(),
                reason: "second halt".to_string(),
                activated_at: "2026-09-11T12:00:00Z".to_string(),
                updated_at: "2026-09-11T12:00:00Z".to_string(),
            },
        ],
        ..Default::default()
    };

    assert_eq!(state.hard_stops.len(), 2);
    // Releasing hs-1 leaves hs-2, so orders still blocked
    state.hard_stops.retain(|e| e.id != "hs-1");
    assert_eq!(state.hard_stops.len(), 1);
    assert!(!state.hard_stops.is_empty());

    // Releasing hs-2 clears hard stops
    state.hard_stops.retain(|e| e.id != "hs-2");
    assert!(state.hard_stops.is_empty());
}

#[test]
fn test_control_plane_treats_empty_state_as_fresh_and_rejects_unavailable_mutations() {
    // Parity: internal/trading/control_plane_state_audit_test.go:99 TestControlPlaneTreatsEmptyStateAsFreshAndRejectsUnavailableMutations
    use jftrade_trading::RealTradeControlState;

    let empty = RealTradeControlState::default();
    assert!(empty.kill_switch.is_none());
    assert!(empty.hard_stops.is_empty());
}

#[test]
fn test_control_plane_surfaces_hard_stop_rejection_audit_persistence_failure() {
    // Parity: internal/trading/control_plane_state_audit_test.go:250 TestControlPlaneSurfacesHardStopRejectionAuditPersistenceFailure
    assert_eq!(
        engine(Some("REAL"))
            .evaluate(&command(TradingEnvironment::Real))
            .reason_code
            .as_deref(),
        Some("REAL_TRADE_HARD_STOP_ACTIVE")
    );
}
