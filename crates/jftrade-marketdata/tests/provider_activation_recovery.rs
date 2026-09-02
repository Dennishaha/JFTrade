use jftrade_kernel::Fixed8;
use jftrade_marketdata::{
    ActivationMode, HealthStatus, InstrumentRef, MarketDataError, ProviderCapabilities,
    ProviderConstraints, ProviderDescriptor, ProviderReadiness, ProviderRouter, Tick,
};

fn descriptor(selection_id: &str, streaming_quotes: bool) -> ProviderDescriptor {
    ProviderDescriptor {
        selection_id: selection_id.to_owned(),
        provider_id: selection_id.to_owned(),
        display_name: selection_id.to_owned(),
        broker_id: None,
        source: selection_id.to_owned(),
        default_market: "US".to_owned(),
        supported_markets: vec!["US".to_owned()],
        transports: vec![if streaming_quotes { "stream" } else { "poll" }.to_owned()],
        capabilities: ProviderCapabilities {
            snapshots: true,
            streaming_quotes,
            ..ProviderCapabilities::default()
        },
        constraints: ProviderConstraints::default(),
        notes: Vec::new(),
    }
}

fn health(readiness: ProviderReadiness, connected: bool, last_error: Option<&str>) -> HealthStatus {
    HealthStatus {
        connected,
        stream_mode: "snapshot-poll-delayed".to_owned(),
        readiness,
        last_error: last_error.map(str::to_owned),
        ..HealthStatus::default()
    }
}

fn snapshot_tick(generation: u64, observed_at_ms: i64) -> Tick {
    Tick {
        instrument_id: "US.AAPL".to_owned(),
        price: Fixed8::from_scaled(188_500_000_000),
        volume: "10".parse().expect("decimal volume"),
        snapshot: None,
        observed_at_ms,
        provider_generation: generation,
    }
}

fn register_pair(router: &mut ProviderRouter, secondary_health: HealthStatus) {
    router
        .register(
            descriptor("futu", true),
            health(ProviderReadiness::Ready, true, None),
        )
        .expect("register futu");
    router
        .register(descriptor("yfinance", false), secondary_health)
        .expect("register yfinance");
}

#[test]
fn provider_activation_fails_closed_preserves_previous_generation_and_recovers_after_health_update()
{
    // Mirrors Go runtime health/AKShare activation tests: a failed health gate
    // must not publish the target provider or clear the previous provider's
    // cache; a later healthy probe can commit a new generation.
    let mut router = ProviderRouter::new(2);
    register_pair(
        &mut router,
        health(ProviderReadiness::Failed, false, Some("helper unavailable")),
    );

    let first = router
        .activate("futu", ActivationMode::Explicit)
        .expect("activate initial provider");
    assert_eq!(first.generation, 1);
    router
        .cache_mut()
        .insert(snapshot_tick(first.generation, 1_000), first.generation)
        .expect("seed active-generation cache");

    assert_eq!(
        router.activate("yfinance", ActivationMode::Explicit),
        Err(MarketDataError::ProviderUnavailable {
            provider_id: "yfinance".to_owned(),
            reason: "helper unavailable".to_owned(),
        })
    );
    let after_rejection = router.runtime();
    assert_eq!(after_rejection.active_provider, "futu");
    assert_eq!(after_rejection.generation, first.generation);
    assert!(matches!(
        router.cache().lookup("US.AAPL", 1_100, 500),
        jftrade_marketdata::CacheLookup::Fresh(_)
    ));

    router
        .update_health("yfinance", health(ProviderReadiness::Ready, true, None))
        .expect("record helper recovery");
    let switched = router
        .activate("yfinance", ActivationMode::Explicit)
        .expect("activate recovered provider");
    assert_eq!(switched.generation, first.generation + 1);
    assert_eq!(router.runtime().active_provider, "yfinance");
    assert_eq!(
        router.cache().lookup("US.AAPL", 1_100, 500),
        jftrade_marketdata::CacheLookup::Missing
    );
}

#[test]
fn startup_restore_retains_warming_until_failure_is_reported() {
    // Startup restore deliberately retains a warming helper even when the
    // first health snapshot is disconnected.  The application health probe
    // owns the connected/ready gate for explicit activation; this lower-level
    // router only rejects unknown or failed state and an explicit error.
    let mut router = ProviderRouter::new(1);
    router
        .register(
            descriptor("warming", false),
            health(ProviderReadiness::Warming, true, None),
        )
        .expect("register warming provider");
    let runtime = router
        .activate("warming", ActivationMode::StartupRestore)
        .expect("connected warming provider is restorable");
    assert_eq!(runtime.readiness, ProviderReadiness::Warming);
    assert!(runtime.connected);

    router
        .update_health("warming", health(ProviderReadiness::Warming, false, None))
        .expect("record disconnected warming state");
    let runtime = router
        .activate("warming", ActivationMode::StartupRestore)
        .expect("disconnected warming provider remains restorable");
    assert_eq!(runtime.readiness, ProviderReadiness::Warming);
    assert!(!runtime.connected);

    router
        .update_health(
            "warming",
            health(ProviderReadiness::Warming, true, Some("warmup failed")),
        )
        .expect("record failed warmup state");
    assert!(matches!(
        router.activate("warming", ActivationMode::StartupRestore),
        Err(MarketDataError::ProviderUnavailable {
            provider_id,
            reason,
        }) if provider_id == "warming" && reason == "warmup failed"
    ));
}

#[test]
fn managed_streaming_demand_blocks_provider_switch_until_released() {
    let mut router = ProviderRouter::new(1);
    router
        .register(
            descriptor("futu", true),
            health(ProviderReadiness::Ready, true, None),
        )
        .expect("register futu");
    router
        .register(
            descriptor("other", true),
            health(ProviderReadiness::Ready, true, None),
        )
        .expect("register other");
    router
        .activate("futu", ActivationMode::Explicit)
        .expect("activate futu");
    let demand = InstrumentRef {
        channel: "KLINE".to_owned(),
        market: "US".to_owned(),
        symbol: "AAPL".to_owned(),
        interval: Some("1m".to_owned()),
    };
    router
        .acquire_demand("strategy", [demand.clone()], true, 10)
        .expect("acquire managed demand");
    assert_eq!(
        router.activate("other", ActivationMode::Explicit),
        Err(MarketDataError::ManagedSubscriptionsActive)
    );

    let (released, snapshot) = router.release_demand_consumer_with_time("strategy", 20);
    assert!(released);
    assert_eq!(snapshot.logical_count, 0);
    let switched = router
        .activate("other", ActivationMode::Explicit)
        .expect("switch after managed demand release");
    assert_eq!(switched.active_provider, "other");
}
