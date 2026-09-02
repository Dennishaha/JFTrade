use std::env;
use std::net::SocketAddr;
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use jftrade_integration_futu::{
    OpenDProviderRuntime, OpenDProviderRuntimeConfig, OpenDTcpProbeConfig, provider_descriptor,
};
use jftrade_marketdata::{CacheLookup, InstrumentRef, ProviderReadiness, ProviderRouter};

const LIVE_TEST_ENV: &str = "JFTRADE_FUTU_LIVE_TEST";
const OPEND_ADDRESS_ENV: &str = "FUTU_OPEND_ADDR";
const DEFAULT_OPEND_ADDRESS: &str = "127.0.0.1:11110";

#[test]
#[ignore = "requires the explicitly confirmed self-hosted OpenD live workflow"]
fn live_opend_provider_runtime_reads_generation_fenced_hk_quote() {
    assert_eq!(
        env::var(LIVE_TEST_ENV).as_deref(),
        Ok("1"),
        "set {LIVE_TEST_ENV}=1 only in the explicit OpenD live workflow"
    );
    let address = live_opend_address();
    let router = Arc::new(Mutex::new(ProviderRouter::new(2)));
    let mut config = OpenDProviderRuntimeConfig::with_defaults(
        Arc::clone(&router),
        provider_descriptor(),
        OpenDTcpProbeConfig::new(address, Duration::from_secs(3)),
        vec![snapshot_demand("00700")],
        now_ms(),
    );
    config.demand_managed = true;
    config.task.poll_interval = Duration::from_millis(100);
    config.task.event_timeout = Duration::from_millis(10);
    let runtime = OpenDProviderRuntime::start(config).expect("start live OpenD provider runtime");

    let tick = wait_for_live_tick(&runtime, "HK.00700", Duration::from_secs(15));
    assert_eq!(tick.instrument_id, "HK.00700");
    assert!(tick.price.scaled() > 0, "live quote price must be positive");
    let state = router.lock().expect("router").runtime();
    assert_eq!(state.active_provider, "futu");
    assert_eq!(state.readiness, ProviderReadiness::Ready);
    assert!(state.connected);
    assert_eq!(state.active_demand, 1);

    runtime
        .shutdown()
        .expect("shutdown live OpenD provider runtime");
    let guard = router.lock().expect("router after shutdown");
    assert!(guard.runtime().active_provider.is_empty());
    assert!(guard.demand().active.is_empty());
}

fn live_opend_address() -> SocketAddr {
    env::var(OPEND_ADDRESS_ENV)
        .ok()
        .filter(|value| !value.trim().is_empty())
        .unwrap_or_else(|| DEFAULT_OPEND_ADDRESS.to_owned())
        .parse()
        .unwrap_or_else(|error| panic!("parse {OPEND_ADDRESS_ENV}: {error}"))
}

fn snapshot_demand(symbol: &str) -> InstrumentRef {
    InstrumentRef {
        channel: "SNAPSHOT".to_owned(),
        market: "HK".to_owned(),
        symbol: symbol.to_owned(),
        interval: None,
    }
}

fn wait_for_live_tick(
    runtime: &OpenDProviderRuntime,
    instrument_id: &str,
    timeout: Duration,
) -> jftrade_marketdata::Tick {
    let deadline = Instant::now() + timeout;
    loop {
        let generation = runtime
            .router()
            .lock()
            .expect("router generation")
            .runtime_recorder()
            .snapshot()
            .generation;
        let lookup = runtime
            .runtime()
            .cache()
            .lock()
            .expect("tick cache")
            .lookup_for_generation(instrument_id, now_ms(), 30_000, generation);
        if let CacheLookup::Fresh(tick) = lookup {
            return tick;
        }
        assert!(
            Instant::now() < deadline,
            "live OpenD quote did not become generation-fresh: {:?}",
            runtime.runtime().status()
        );
        std::thread::sleep(Duration::from_millis(100));
    }
}

fn now_ms() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .ok()
        .and_then(|duration| i64::try_from(duration.as_millis()).ok())
        .expect("current Unix time fits i64 milliseconds")
}
