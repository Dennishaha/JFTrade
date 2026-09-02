use std::str::FromStr;

use jftrade_kernel::Fixed8;
use jftrade_strategy::{
    ExecutionMode, RuntimeState, Signal, StrategyCoordinator, StrategyError, TradeIntent,
    TradePlanReceipt, TradePlannerPort,
};

#[derive(Default)]
struct RecordingPlanner {
    intents: Vec<TradeIntent>,
}

impl TradePlannerPort for RecordingPlanner {
    fn plan_trade(&mut self, intent: TradeIntent) -> Result<TradePlanReceipt, String> {
        self.intents.push(intent);
        Ok(TradePlanReceipt {
            accepted: true,
            dispatch: false,
            reason_code: None,
        })
    }
}

fn signal() -> Signal {
    Signal {
        signal_id: "signal-1".to_owned(),
        trace_id: "trace-1".to_owned(),
        instance_id: "instance-1".to_owned(),
        broker_id: "futu".to_owned(),
        account_id: "account-1".to_owned(),
        market: "US".to_owned(),
        symbol: "AAPL".to_owned(),
        side: "BUY".to_owned(),
        quantity: Fixed8::from_str("1").expect("quantity"),
        price: Some(Fixed8::from_str("100").expect("price")),
        observed_at: "2026-08-19T00:00:00Z".parse().expect("timestamp"),
    }
}

fn running_coordinator() -> StrategyCoordinator<RecordingPlanner> {
    let mut coordinator =
        StrategyCoordinator::new(ExecutionMode::Paper, RecordingPlanner::default());
    assert!(coordinator.start());
    assert!(coordinator.ready());
    assert_eq!(coordinator.state(), RuntimeState::Running);
    coordinator
}

fn assert_rejected_without_planning(invalid_signal: Signal, expected: StrategyError, case: &str) {
    let mut coordinator = running_coordinator();
    assert_eq!(
        coordinator.handle_signal(invalid_signal),
        Err(expected),
        "{case}"
    );
    assert!(
        coordinator.into_planner().intents.is_empty(),
        "{case}: malformed signal reached the trading planner"
    );
}

#[test]
fn missing_signal_identity_is_rejected_before_planning() {
    let mut missing_signal_id = signal();
    missing_signal_id.signal_id = " ".to_owned();
    assert_rejected_without_planning(
        missing_signal_id,
        StrategyError::MissingField("signalId"),
        "signal id",
    );

    let mut missing_trace_id = signal();
    missing_trace_id.trace_id.clear();
    assert_rejected_without_planning(
        missing_trace_id,
        StrategyError::MissingField("traceId"),
        "trace id",
    );

    let mut missing_instance_id = signal();
    missing_instance_id.instance_id = "\t".to_owned();
    assert_rejected_without_planning(
        missing_instance_id,
        StrategyError::MissingField("instanceId"),
        "instance id",
    );

    let mut missing_symbol = signal();
    missing_symbol.symbol.clear();
    assert_rejected_without_planning(
        missing_symbol,
        StrategyError::MissingField("symbol"),
        "symbol",
    );
}

#[test]
fn non_positive_signal_quantity_is_rejected_before_planning() {
    let mut zero = signal();
    zero.quantity = Fixed8::ZERO;
    assert_rejected_without_planning(zero, StrategyError::InvalidQuantity, "zero quantity");

    let mut negative = signal();
    negative.quantity = Fixed8::from_str("-1").expect("negative quantity");
    assert_rejected_without_planning(
        negative,
        StrategyError::InvalidQuantity,
        "negative quantity",
    );
}

#[test]
fn unsupported_signal_side_is_rejected_before_planning() {
    for side in ["", "HOLD", "BUY_TO_OPEN"] {
        let mut invalid = signal();
        invalid.side = side.to_owned();
        assert_rejected_without_planning(invalid, StrategyError::InvalidSide, side);
    }
}
