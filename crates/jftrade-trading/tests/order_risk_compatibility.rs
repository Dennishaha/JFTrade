use std::str::FromStr;

use jftrade_kernel::Fixed8;
use jftrade_trading::{
    HardStop, OrderCommand, OrderSide, OrderStatus, RiskConfig, RiskEngine, TradingEnvironment,
    TradingError, canonical_broker_status, canonical_stored_status, reconcile_status,
};

fn fixed(value: &str) -> Fixed8 {
    Fixed8::from_str(value)
        .unwrap_or_else(|error| panic!("invalid Fixed8 value {value:?}: {error}"))
}

fn command() -> OrderCommand {
    OrderCommand {
        idempotency_key: "key-1".to_owned(),
        trace_id: "trace-1".to_owned(),
        broker_id: "futu".to_owned(),
        account_id: "acc-1".to_owned(),
        environment: TradingEnvironment::Real,
        market: "US".to_owned(),
        symbol: "AAPL".to_owned(),
        side: OrderSide::Buy,
        quantity: fixed("10"),
        price: Some(fixed("10")),
        client_order_id: "client-1".to_owned(),
    }
}

#[test]
fn canonical_broker_status_covers_futu_lifecycle_families() {
    let cases = [
        ("Unsubmitted", OrderStatus::Submitting),
        ("WAITING_SUBMIT", OrderStatus::Submitting),
        ("Submitting", OrderStatus::Submitting),
        ("NEW", OrderStatus::BrokerAccepted),
        ("Submitted", OrderStatus::BrokerAccepted),
        ("Filled_Part", OrderStatus::PartiallyFilled),
        ("Filled_All", OrderStatus::Filled),
        ("Cancelling_Part", OrderStatus::CancelRequested),
        ("Cancelling_All", OrderStatus::CancelRequested),
        ("Cancelled_Part", OrderStatus::Cancelled),
        ("Cancelled_All", OrderStatus::Cancelled),
        ("SubmitFailed", OrderStatus::Rejected),
        ("Failed", OrderStatus::Rejected),
        ("Disabled", OrderStatus::Rejected),
        ("Deleted", OrderStatus::Cancelled),
        ("FillCancelled", OrderStatus::Rejected),
        ("TimeOut", OrderStatus::Unknown),
        ("unexpected", OrderStatus::Unknown),
        (" ORDER_STATUS_FILLED-ALL ", OrderStatus::Filled),
    ];

    for (raw, expected) in cases {
        assert_eq!(canonical_broker_status(raw), expected, "raw status {raw:?}");
    }
}

#[test]
fn stored_status_and_terminal_classification_match_go_contract() {
    let stored = [
        ("created", OrderStatus::Created),
        ("precheck_rejected", OrderStatus::PrecheckRejected),
        ("submitting", OrderStatus::Submitting),
        ("submission_unknown", OrderStatus::SubmissionUnknown),
        ("submitted", OrderStatus::Submitted),
        ("broker_accepted", OrderStatus::BrokerAccepted),
        ("partially_filled", OrderStatus::PartiallyFilled),
        ("filled", OrderStatus::Filled),
        ("cancel_requested", OrderStatus::CancelRequested),
        ("cancelled", OrderStatus::Cancelled),
        ("rejected", OrderStatus::Rejected),
        ("expired", OrderStatus::Expired),
        ("unknown", OrderStatus::Unknown),
        ("order_status_broker_accepted", OrderStatus::BrokerAccepted),
    ];
    for (raw, expected) in stored {
        assert_eq!(
            canonical_stored_status(raw),
            expected,
            "stored status {raw:?}"
        );
    }

    for status in [
        OrderStatus::PrecheckRejected,
        OrderStatus::Filled,
        OrderStatus::Cancelled,
        OrderStatus::Rejected,
        OrderStatus::Expired,
    ] {
        assert!(status.is_terminal(), "status {status:?} should be terminal");
    }
    for status in [
        OrderStatus::Created,
        OrderStatus::Submitting,
        OrderStatus::SubmissionUnknown,
        OrderStatus::Submitted,
        OrderStatus::BrokerAccepted,
        OrderStatus::PartiallyFilled,
        OrderStatus::CancelRequested,
        OrderStatus::Unknown,
    ] {
        assert!(
            !status.is_terminal(),
            "status {status:?} should not be terminal"
        );
    }
}

#[test]
fn status_reconciliation_prevents_regressions_and_preserves_cancel_races() {
    let cases = [
        (
            "accepted to partial",
            OrderStatus::BrokerAccepted,
            OrderStatus::PartiallyFilled,
            (OrderStatus::PartiallyFilled, true),
        ),
        (
            "partial to submitted regression",
            OrderStatus::PartiallyFilled,
            OrderStatus::BrokerAccepted,
            (OrderStatus::PartiallyFilled, false),
        ),
        (
            "cancel race fills",
            OrderStatus::CancelRequested,
            OrderStatus::Filled,
            (OrderStatus::Filled, true),
        ),
        (
            "cancel request ignores partial regression",
            OrderStatus::CancelRequested,
            OrderStatus::PartiallyFilled,
            (OrderStatus::CancelRequested, false),
        ),
        (
            "filled is terminal",
            OrderStatus::Filled,
            OrderStatus::BrokerAccepted,
            (OrderStatus::Filled, false),
        ),
        (
            "unknown recovers",
            OrderStatus::Unknown,
            OrderStatus::BrokerAccepted,
            (OrderStatus::BrokerAccepted, true),
        ),
        (
            "known ignores unknown",
            OrderStatus::BrokerAccepted,
            OrderStatus::Unknown,
            (OrderStatus::BrokerAccepted, false),
        ),
        (
            "submitted ignores created regression",
            OrderStatus::Submitted,
            OrderStatus::Created,
            (OrderStatus::Submitted, false),
        ),
    ];

    for (name, current, incoming, expected) in cases {
        assert_eq!(reconcile_status(current, incoming), expected, "case {name}");
    }
}

#[test]
fn order_validation_rejects_non_positive_quantity_and_optional_price() {
    let mut zero_quantity = command();
    zero_quantity.quantity = Fixed8::ZERO;
    assert_eq!(zero_quantity.validate(), Err(TradingError::InvalidQuantity));

    let mut negative_quantity = command();
    negative_quantity.quantity = fixed("-1");
    assert_eq!(
        negative_quantity.validate(),
        Err(TradingError::InvalidQuantity)
    );

    let mut zero_price = command();
    zero_price.price = Some(Fixed8::ZERO);
    assert_eq!(zero_price.validate(), Err(TradingError::InvalidPrice));

    let mut negative_price = command();
    negative_price.price = Some(fixed("-1"));
    assert_eq!(negative_price.validate(), Err(TradingError::InvalidPrice));
}

#[test]
fn real_risk_limits_fail_closed_at_missing_price_and_hard_stop_boundaries() {
    let engine = RiskEngine::new(RiskConfig {
        real_trading_enabled: true,
        kill_switch_active: false,
        max_order_quantity: Some(fixed("10")),
        max_order_notional: Some(fixed("100")),
        hard_stops: vec![HardStop {
            broker_id: Some("FUTU".to_owned()),
            trading_environment: Some("REAL".to_owned()),
            account_id: Some("*".to_owned()),
            market: Some("us".to_owned()),
            symbol: Some("US.AAPL".to_owned()),
        }],
    });

    let decision = engine.evaluate(&command());
    assert_eq!(
        decision.reason_code.as_deref(),
        Some("REAL_TRADE_HARD_STOP_ACTIVE")
    );

    let mut outside_stop = command();
    outside_stop.symbol = "MSFT".to_owned();
    outside_stop.quantity = fixed("11");
    assert_eq!(
        engine.evaluate(&outside_stop).reason_code.as_deref(),
        Some("MAX_ORDER_QUANTITY_EXCEEDED")
    );

    let mut missing_price = outside_stop;
    missing_price.quantity = fixed("10");
    missing_price.price = None;
    assert_eq!(
        engine.evaluate(&missing_price).reason_code.as_deref(),
        Some("RISK_PRICE_UNAVAILABLE")
    );

    let mut notional_exceeded = command();
    notional_exceeded.symbol = "MSFT".to_owned();
    notional_exceeded.price = Some(fixed("11"));
    assert_eq!(
        engine.evaluate(&notional_exceeded).reason_code.as_deref(),
        Some("MAX_ORDER_NOTIONAL_EXCEEDED")
    );
}
