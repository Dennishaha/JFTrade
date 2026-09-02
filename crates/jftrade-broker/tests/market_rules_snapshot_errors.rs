use std::error::Error;
use std::fmt;
use std::time::Duration;

use jftrade_broker::{
    MarketQuantityConstraints, MarketRuleItem, SnapshotAvailabilityError, SnapshotAvailabilityKind,
    SnapshotRateLimitError, SymbolScopedSnapshotError, apply_market_rule, apply_market_rules,
    is_snapshot_fallback_eligible, is_snapshot_rate_limited, is_symbol_scoped_snapshot_error,
    snapshot_availability, snapshot_retry_after,
};
use jftrade_kernel::Fixed8;

fn fixed(value: &str) -> Fixed8 {
    value.parse().expect("valid fixed8 fixture")
}

#[test]
fn market_rules_match_trimmed_symbols_and_apply_overrides_in_order() {
    let market = MarketQuantityConstraints {
        symbol: " hk.00700 ".to_owned(),
        min_quantity: fixed("1"),
        step_size: fixed("1"),
    };
    let rules = [
        MarketRuleItem {
            symbol: "US.AAPL".to_owned(),
            lot_size: Some(100),
            ..MarketRuleItem::default()
        },
        MarketRuleItem {
            symbol: " HK.00700 ".to_owned(),
            lot_size: Some(100),
            min_quantity: Some(fixed("200")),
            step_size: Some(fixed("50")),
        },
        MarketRuleItem {
            symbol: "HK.00700".to_owned(),
            lot_size: Some(999),
            ..MarketRuleItem::default()
        },
    ];

    let enriched = apply_market_rules(market.clone(), &rules);
    assert_eq!(enriched.min_quantity, fixed("200"));
    assert_eq!(enriched.step_size, fixed("50"));

    let unmatched = apply_market_rules(market.clone(), &rules[..1]);
    assert_eq!(unmatched, market);

    let lot_only = apply_market_rule(
        MarketQuantityConstraints {
            symbol: "HK.00700".to_owned(),
            min_quantity: fixed("1"),
            step_size: fixed("1"),
        },
        &rules[1],
    );
    assert_eq!(lot_only.min_quantity, fixed("200"));
    assert_eq!(lot_only.step_size, fixed("50"));
}

#[test]
fn market_rules_ignore_missing_non_positive_and_non_finite_constraints() {
    let market = MarketQuantityConstraints {
        symbol: "HK.00700".to_owned(),
        min_quantity: fixed("5"),
        step_size: fixed("5"),
    };
    let unchanged = apply_market_rule(
        market.clone(),
        &MarketRuleItem {
            symbol: "HK.00700".to_owned(),
            lot_size: Some(0),
            min_quantity: Some(Fixed8::ZERO),
            step_size: Some(fixed("-1")),
        },
    );
    assert_eq!(unchanged, market);

    let negative_lot = apply_market_rule(
        market.clone(),
        &MarketRuleItem {
            symbol: "HK.00700".to_owned(),
            lot_size: Some(-100),
            ..MarketRuleItem::default()
        },
    );
    assert_eq!(negative_lot, market);

    let non_finite = apply_market_rule(
        market.clone(),
        &MarketRuleItem {
            symbol: "HK.00700".to_owned(),
            min_quantity: Some(Fixed8::POS_INFINITY),
            step_size: Some(Fixed8::POS_INFINITY),
            ..MarketRuleItem::default()
        },
    );
    assert_eq!(non_finite, market);
}

#[test]
fn market_rule_json_preserves_optional_wire_fields() {
    let rule = MarketRuleItem {
        symbol: "HK.00700".to_owned(),
        lot_size: Some(100),
        min_quantity: Some(fixed("200")),
        step_size: None,
    };
    let encoded = serde_json::to_value(rule).expect("encode market rule");
    let expected: serde_json::Value =
        serde_json::from_str(r#"{"symbol":"HK.00700","lotSize":100,"minQuantity":200.00000000}"#)
            .expect("decode expected market rule");
    assert_eq!(encoded, expected);
}

#[test]
fn symbol_scoped_snapshot_errors_are_detectable_through_context() {
    let marked = SymbolScopedSnapshotError::new("bad symbol");
    assert_eq!(marked.to_string(), "bad symbol");
    assert!(is_symbol_scoped_snapshot_error(&marked));

    let wrapped = ContextError::new("outer", Box::new(marked));
    assert!(is_symbol_scoped_snapshot_error(&wrapped));
    assert!(!is_symbol_scoped_snapshot_error(&std::io::Error::other(
        "transport failure",
    )));
}

#[test]
fn snapshot_rate_limits_preserve_retry_delay_and_context() {
    let explicit =
        SnapshotRateLimitError::with_message(Duration::from_millis(2_500), "quota exhausted");
    assert_eq!(explicit.to_string(), "quota exhausted");
    assert_eq!(explicit.retry_after(), Duration::from_millis(2_500));
    assert!(is_snapshot_rate_limited(&explicit));
    assert_eq!(
        snapshot_retry_after(&ContextError::new("wrapped", Box::new(explicit))),
        Some(Duration::from_millis(2_500))
    );

    let defaulted = SnapshotRateLimitError::new(Duration::ZERO);
    assert_eq!(defaulted.retry_after(), Duration::from_secs(1));
    assert_eq!(
        defaulted.to_string(),
        "broker snapshot rate limited; retry after 1s"
    );
    assert!(!is_snapshot_rate_limited(&std::io::Error::other("plain")));
    assert_eq!(snapshot_retry_after(&std::io::Error::other("plain")), None);
}

#[test]
fn snapshot_availability_kinds_control_fallback_eligibility() {
    for (kind, eligible) in [
        ("entitlement", true),
        ("unsupported", true),
        ("subscription_quota", true),
        ("provider_busy", false),
    ] {
        let error = SnapshotAvailabilityError::new(
            SnapshotAvailabilityKind::new(kind),
            "BasicQot entitlement is unavailable",
        );
        assert_eq!(
            snapshot_availability(&error),
            Some(SnapshotAvailabilityKind::new(kind))
        );
        assert_eq!(is_snapshot_fallback_eligible(&error), eligible);

        let wrapped = ContextError::new("wrapped", Box::new(error));
        assert_eq!(
            snapshot_availability(&wrapped),
            Some(SnapshotAvailabilityKind::new(kind))
        );
        assert_eq!(is_snapshot_fallback_eligible(&wrapped), eligible);
    }
    assert_eq!(snapshot_availability(&std::io::Error::other("plain")), None);
    assert!(!is_snapshot_fallback_eligible(&std::io::Error::other(
        "plain"
    )));
}

#[derive(Debug)]
struct ContextError {
    message: String,
    source: Box<dyn Error + Send + Sync>,
}

impl ContextError {
    fn new(message: impl Into<String>, source: Box<dyn Error + Send + Sync>) -> Self {
        Self {
            message: message.into(),
            source,
        }
    }
}

impl fmt::Display for ContextError {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str(&self.message)
    }
}

impl Error for ContextError {
    fn source(&self) -> Option<&(dyn Error + 'static)> {
        Some(self.source.as_ref())
    }
}
