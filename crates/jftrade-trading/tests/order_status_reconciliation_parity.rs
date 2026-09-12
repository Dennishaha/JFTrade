use jftrade_trading::{OrderStatus, canonical_broker_status, reconcile_status};

#[test]
fn test_canonical_broker_order_status_covers_futu_lifecycle() {
    // Parity: internal/trading/order_status_test.go:5 TestCanonicalBrokerOrderStatusCoversFutuLifecycle
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
    ];

    for (raw, want) in cases {
        let got = canonical_broker_status(raw);
        assert_eq!(
            got, want,
            "canonical_broker_status({raw:?}) = {got:?}, want {want:?}"
        );
    }
}

#[test]
fn test_reconcile_canonical_order_status_prevents_broker_regressions() {
    // Parity: internal/trading/order_status_test.go:33 TestReconcileCanonicalOrderStatusPreventsBrokerRegressions
    struct TestCase {
        name: &'static str,
        current: OrderStatus,
        incoming: OrderStatus,
        want: OrderStatus,
        accepted: bool,
    }

    let tests = [
        TestCase {
            name: "accepted to partial",
            current: OrderStatus::BrokerAccepted,
            incoming: OrderStatus::PartiallyFilled,
            want: OrderStatus::PartiallyFilled,
            accepted: true,
        },
        TestCase {
            name: "partial to submitted regression",
            current: OrderStatus::PartiallyFilled,
            incoming: OrderStatus::BrokerAccepted,
            want: OrderStatus::PartiallyFilled,
            accepted: false,
        },
        TestCase {
            name: "cancel race fills",
            current: OrderStatus::CancelRequested,
            incoming: OrderStatus::Filled,
            want: OrderStatus::Filled,
            accepted: true,
        },
        TestCase {
            name: "cancel request ignores partial regression",
            current: OrderStatus::CancelRequested,
            incoming: OrderStatus::PartiallyFilled,
            want: OrderStatus::CancelRequested,
            accepted: false,
        },
        TestCase {
            name: "filled is terminal",
            current: OrderStatus::Filled,
            incoming: OrderStatus::BrokerAccepted,
            want: OrderStatus::Filled,
            accepted: false,
        },
        TestCase {
            name: "unknown recovers",
            current: OrderStatus::Unknown,
            incoming: OrderStatus::BrokerAccepted,
            want: OrderStatus::BrokerAccepted,
            accepted: true,
        },
        TestCase {
            name: "known ignores unknown",
            current: OrderStatus::BrokerAccepted,
            incoming: OrderStatus::Unknown,
            want: OrderStatus::BrokerAccepted,
            accepted: false,
        },
    ];

    for t in tests {
        let (got, accepted) = reconcile_status(t.current, t.incoming);
        assert_eq!(
            (got, accepted),
            (t.want, t.accepted),
            "reconcile_status({:?}, {:?}) for '{}' = ({:?}, {}), want ({:?}, {})",
            t.current,
            t.incoming,
            t.name,
            got,
            accepted,
            t.want,
            t.accepted
        );
    }
}

#[test]
fn test_canonical_terminal_order_status() {
    // Parity: internal/trading/order_status_test.go:61 TestCanonicalTerminalOrderStatus
    let terminals = [
        OrderStatus::PrecheckRejected,
        OrderStatus::Filled,
        OrderStatus::Cancelled,
        OrderStatus::Rejected,
        OrderStatus::Expired,
    ];
    for status in terminals {
        assert!(status.is_terminal(), "status {status:?} should be terminal");
    }

    let non_terminals = [
        OrderStatus::Created,
        OrderStatus::Submitting,
        OrderStatus::SubmissionUnknown,
        OrderStatus::Submitted,
        OrderStatus::BrokerAccepted,
        OrderStatus::PartiallyFilled,
        OrderStatus::CancelRequested,
        OrderStatus::Unknown,
    ];
    for status in non_terminals {
        assert!(
            !status.is_terminal(),
            "status {status:?} should not be terminal"
        );
    }
}
