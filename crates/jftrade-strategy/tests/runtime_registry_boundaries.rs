use jftrade_strategy::{
    RuntimeInstanceSummary, RuntimeRegistryError, RuntimeState, StrategyRuntimeRegistry,
};

fn instance(
    id: &str,
    state: RuntimeState,
    symbols: &[&str],
    last_error: Option<&str>,
) -> RuntimeInstanceSummary {
    let normalized_id = id.trim();
    RuntimeInstanceSummary {
        instance_id: id.to_owned(),
        definition_name: format!(" {normalized_id} definition "),
        actual_state: state,
        active_symbols: symbols.iter().map(|symbol| (*symbol).to_owned()).collect(),
        last_closed_kline_at: None,
        last_signal_at: None,
        last_order_at: None,
        last_error_at: None,
        last_error: last_error.map(str::to_owned),
        updated_at: None,
    }
}

#[test]
fn runtime_registry_reconciles_replacements_and_normalizes_snapshot_values() {
    let registry = StrategyRuntimeRegistry::default();
    assert_eq!(registry.snapshot().status(), "idle");
    assert_eq!(registry.snapshot().active_strategies(), 0);
    assert_eq!(
        registry.upsert(instance("  ", RuntimeState::Running, &[], None)),
        Err(RuntimeRegistryError::MissingInstanceId)
    );

    registry
        .upsert(instance(
            " z-runtime ",
            RuntimeState::Running,
            &[" US.AAPL ", "US.AAPL", "", "US.TSLA"],
            Some("  worker ready  "),
        ))
        .expect("running runtime");
    registry
        .upsert(instance(
            "a-runtime",
            RuntimeState::Paused,
            &["HK.00700"],
            Some("  "),
        ))
        .expect("paused runtime");

    let snapshot = registry.snapshot();
    assert_eq!(snapshot.status(), "active");
    assert_eq!(snapshot.active_strategies(), 2);
    assert_eq!(
        snapshot
            .active_instances
            .iter()
            .map(|instance| instance.instance_id.as_str())
            .collect::<Vec<_>>(),
        ["a-runtime", "z-runtime"]
    );
    let running = &snapshot.active_instances[1];
    assert_eq!(running.actual_state, RuntimeState::Running);
    assert_eq!(running.definition_name, "z-runtime definition");
    assert_eq!(running.active_symbols, ["US.AAPL", "US.TSLA"]);
    assert_eq!(running.last_error.as_deref(), Some("worker ready"));
    assert_eq!(snapshot.active_instances[0].last_error, None);

    registry
        .upsert(instance(
            " z-runtime ",
            RuntimeState::Stopped,
            &["US.MSFT"],
            Some(" worker exited "),
        ))
        .expect("reconciled runtime");
    let reconciled = registry.snapshot();
    assert_eq!(reconciled.active_strategies(), 2);
    let replaced = &reconciled.active_instances[1];
    assert_eq!(replaced.actual_state, RuntimeState::Stopped);
    assert_eq!(replaced.active_symbols, ["US.MSFT"]);
    assert_eq!(replaced.last_error.as_deref(), Some("worker exited"));
}

#[test]
fn runtime_registry_removes_trimmed_ids_and_returns_to_idle() {
    let registry = StrategyRuntimeRegistry::default();
    registry
        .upsert(instance("runtime", RuntimeState::Stopped, &[], None))
        .expect("runtime");

    assert!(registry.remove(" runtime "));
    assert!(!registry.remove("runtime"));
    let snapshot = registry.snapshot();
    assert_eq!(snapshot.status(), "idle");
    assert_eq!(snapshot.active_strategies(), 0);
}
