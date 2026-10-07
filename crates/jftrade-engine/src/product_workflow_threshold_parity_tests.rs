use super::*;

fn now() -> OffsetDateTime {
    OffsetDateTime::parse("2026-07-01T01:00:00Z", &Rfc3339).expect("frozen time")
}

// Parity: go:452dea11:internal/assistant/workflow_crud_test.go:616 TestWorkflowMarketThresholdAndConfigHelpersCoverEdges
#[test]
fn threshold_original_nested_cross_down_and_config_scalars_preserve_values() {
    let mut config = json!({"instrumentIds":"US.AAPL, hk.00700, US.AAPL",
        "snapshotPath":"price","value":100,"edge":"cross_down","cooldownSec":"0"});
    let events = [
        json!({"payload":{"instrument":{"instrumentId":"US.AAPL"},"price":101}}),
        json!({"payload":{"instrumentId":"US.AAPL","price":99}}),
        json!({"entityId":"US.MSFT","price":50}),
    ];
    let (matches, changed) = evaluate_market_threshold_trigger(&mut config, &events, now());
    assert!(changed);
    assert_eq!(matches.len(), 1);
    assert_eq!(matches[0].threshold["edge"], "cross_down");
    assert_eq!(matches[0].threshold["current"], 99.0);
    assert!(threshold_fired("below", "<=", None, 100.0, 100.0));
    assert!(threshold_fired("above", ">=", None, 100.0, 100.0));
    assert!(compare_threshold("<", 99.0, 100.0));
    assert!(!compare_threshold(">", 99.0, 100.0));
    assert_eq!(
        numeric_at_path(&json!({"snapshot":"bad"}), "snapshot.price"),
        None
    );
    assert_eq!(
        event_instrument_id(&json!({"payload":{"entityId":"hk.00700"}})).as_deref(),
        Some("HK.00700")
    );
    assert_eq!(
        config_string_slice(&json!({"ids":[" US.AAPL ",700,""]}), "ids"),
        ["US.AAPL", "700"]
    );
    assert_eq!(
        config_string_slice(&json!({"ids":["a","a"," "]}), "ids"),
        ["a"]
    );
    assert_eq!(config_scalar_string(&json!(42)).as_deref(), Some("42"));
    for value in [json!(1.5), json!(2), json!(3), json!("4.5")] {
        assert!(any_f64(&value).is_some(), "{value}");
    }
    assert_eq!(any_f64(&json!("bad")), None);
}

// Parity: go:452dea11:internal/assistant/workflow/rules_test.go:135 TestRuleHelpersAndValidationEdges
#[test]
fn threshold_rule_numeric_paths_state_and_cooldown_cover_original_boundaries() {
    assert!(
        crate::product_workflow_cron::next_schedule_run(
            &json!({"cron":"0 8 * * 1-5","timezone":"Mars/Base"}),
            now()
        )
        .is_err()
    );
    assert!(
        crate::product_workflow_cron::next_run_at_string(&json!({"cron":"bad"}), now()).is_err()
    );
    assert!(threshold_fired("below", "<=", None, 100.0, 100.0));
    assert!(threshold_fired("above", ">=", None, 100.0, 100.0));
    assert!(compare_threshold("<", 99.0, 100.0));
    assert!(!compare_threshold(">", 99.0, 100.0));
    assert_eq!(
        numeric_at_path(&json!({"snapshot":"bad"}), "snapshot.price"),
        None
    );
    assert_eq!(
        numeric_at_path(&json!({"snapshot":{"price":101}}), "snapshot..price"),
        Some(101.0)
    );
    assert_eq!(
        event_instrument_id(&json!({"payload":{"entityId":"hk.00700"}})).as_deref(),
        Some("HK.00700")
    );
    assert_eq!(
        config_string_slice(&json!({"ids":[" US.AAPL ",700,""]}), "ids"),
        ["US.AAPL", "700"]
    );
    assert_eq!(
        config_string_slice(&json!({"ids":["a","a"," "]}), "ids"),
        ["a"]
    );
    assert!(config_string_slice(&json!({}), "ids").is_empty());
    for value in [json!(1), json!(2), json!(3), json!(4), json!("5")] {
        assert!(any_f64(&value).is_some());
    }
    assert_eq!(any_f64(&json!("bad")), None);
    let mut detached = Value::Null;
    assert!(ensure_config_state(&mut detached).is_empty());
    assert!(cooldown_allows(Some("bad timestamp"), now(), 60));
    assert!(!cooldown_allows(Some("2026-07-01T00:59:30Z"), now(), 60));
}

// Parity: go:452dea11:internal/assistant/workflow/rules_test.go:207 TestRuleFallbacksMismatchesAndValidVariants
#[test]
fn threshold_original_mismatched_events_do_not_prime_unrelated_instruments() {
    assert!(
        crate::product_workflow_cron::next_schedule_run(&json!({}), now())
            .expect_err("missing cron")
            .to_string()
            .contains("required")
    );
    assert!(
        crate::product_workflow_cron::next_schedule_run(&json!({"cron":"x x x x x"}), now())
            .is_err()
    );
    assert!(
        !crate::product_workflow_cron::next_run_at_string(&json!({"cron":"0 8 * * *"}), now())
            .expect("default timezone")
            .is_empty()
    );
    for mut config in [
        json!({}),
        json!({"instrumentIds":["US.AAPL"]}),
        json!({"instrumentIds":["US.AAPL"],"value":100}),
    ] {
        let (matches, changed) = evaluate_market_threshold_trigger(&mut config, &[], now());
        assert!(matches.is_empty());
        assert!(!changed);
    }
    let mut config =
        json!({"instrumentIds":["US.AAPL"],"value":100,"edge":"cross_down","cooldownSec":0});
    let events = [
        json!({}),
        json!({"entityId":"US.MSFT","snapshot":{"price":101}}),
        json!({"entityId":"US.AAPL","snapshot":{"price":"bad"}}),
        json!({"entityId":"US.AAPL","payload":{"snapshot":{"price":101}}}),
    ];
    let (matches, changed) = evaluate_market_threshold_trigger(&mut config, &events, now());
    assert!(matches.is_empty());
    assert!(changed);
    assert!(config["state"]["lastValues"].get("US.MSFT").is_none());
    let (matches, changed) = evaluate_market_threshold_trigger(
        &mut config,
        &[json!({"instrument":{"instrumentId":"us.aapl"},"snapshot":{"price":99}})],
        now() + time::Duration::seconds(1),
    );
    assert_eq!(matches.len(), 1);
    assert!(changed);
    assert_eq!(matches[0].threshold["previous"], 101.0);
    assert_eq!(matches[0].threshold["current"], 99.0);
    assert!(threshold_fired("above", "", None, 101.0, 100.0));
    assert_eq!(
        config_string_slice(&json!({"ids":" A, B, A "}), "ids"),
        ["A", "B"]
    );
    assert!(config_string_slice(&json!({"ids":42}), "ids").is_empty());
    assert_eq!(
        event_instrument_id(&json!({"instrumentId":"hk.00700"})).as_deref(),
        Some("HK.00700")
    );
    assert_eq!(event_instrument_id(&json!({})), None);
    let mut rounded =
        json!({"instrumentIds":["US.AAPL"],"value":100,"edge":"above","cooldownSec":1.6});
    let event = json!({"entityId":"US.AAPL","snapshot":{"price":101}});
    for (offset, expected) in [(0, 1), (1, 0), (2, 1)] {
        let (matches, _) = evaluate_market_threshold_trigger(
            &mut rounded,
            std::slice::from_ref(&event),
            now() + time::Duration::seconds(offset),
        );
        assert_eq!(matches.len(), expected, "rounded cooldown at {offset}s");
    }
}

// Parity: go:452dea11:internal/assistant/workflows_extended_test.go:14 TestWorkflowTriggerValidationAndBoundaryHelpers
#[test]
fn threshold_original_cooling_and_missing_paths_preserve_state_decisions() {
    assert!(
        crate::product_workflow_cron::next_schedule_run(
            &json!({"cron":"0 8 * * 1-5","timezone":"Mars/Base"}),
            now()
        )
        .is_err()
    );
    for mut config in [json!({}), json!({"instrumentIds":["US.AAPL"]})] {
        let (matches, changed) =
            evaluate_market_threshold_trigger(&mut config, &[json!({"entityId":"US.AAPL"})], now());
        assert!(matches.is_empty());
        assert!(!changed);
    }
    let mut config = json!({"instrumentIds":["US.AAPL"],"value":100,"edge":"above","cooldownSec":60,
        "state":{"lastTriggeredAt":{"US.AAPL":"2026-07-01T01:00:00Z"}}});
    let (matches, changed) = evaluate_market_threshold_trigger(
        &mut config,
        &[json!({"entityId":"US.AAPL","snapshot":{"price":101}})],
        now() + time::Duration::seconds(10),
    );
    assert!(matches.is_empty());
    assert!(changed);
    assert_eq!(config["state"]["lastValues"]["US.AAPL"], 101.0);
    let mut config = json!({"instrumentIds":["US.AAPL"],"value":100});
    let (matches, changed) = evaluate_market_threshold_trigger(
        &mut config,
        &[json!({"entityId":"US.AAPL","snapshot":{"bad":101}})],
        now(),
    );
    assert!(matches.is_empty());
    assert!(!changed);
    let mut detached = Value::Null;
    assert!(ensure_config_state(&mut detached).is_empty());
    let mut legacy = json!({"state":"legacy"});
    assert!(ensure_config_state(&mut legacy).is_empty());
    assert!(legacy["state"].is_object());
    assert!(cooldown_allows(Some("bad timestamp"), now(), 60));
    assert!(cooldown_allows(Some("2026-07-01T00:59:00Z"), now(), 60));
    assert!(!cooldown_allows(Some("2026-07-01T00:59:30Z"), now(), 60));
    for config in [json!({}), json!({"ids":42})] {
        assert!(config_string_slice(&config, "ids").is_empty());
    }
    assert_eq!(
        numeric_at_path(&json!({"snapshot":{"price":"bad"}}), "snapshot.price"),
        None
    );
    assert_eq!(
        event_instrument_id(&json!({"payload":{"instrument":{"instrumentId":null}}})),
        None
    );
}
