use std::sync::{Arc, Mutex};

use jftrade_marketdata::{
    ActivationMode, HealthStatus, InstrumentRef, ProviderDescriptor, ProviderRouter,
};
use thiserror::Error;

use crate::{
    OpenDSessionCoordinator, OpenDSessionCoordinatorError, OpenDSessionRuntime,
    OpenDSessionRuntimeConfig, OpenDSessionRuntimeError, OpenDTcpProbe, OpenDTcpProbeConfig,
    OpenDTcpProbeError, market_data_health_from_probe,
};

/// Explicit composition input for the Futu/OpenD provider runtime.
///
/// The router is deliberately supplied by the composition root. This keeps
/// provider registration, activation and demand ownership in one place while
/// the OpenD task remains a protocol/lifecycle adapter.
#[derive(Clone, Debug)]
pub struct OpenDProviderRuntimeConfig {
    pub router: Arc<Mutex<ProviderRouter>>,
    pub descriptor: ProviderDescriptor,
    pub opend: OpenDTcpProbeConfig,
    pub desired: Vec<InstrumentRef>,
    pub now_ms: i64,
    pub demand_consumer_id: String,
    pub demand_managed: bool,
    pub task: OpenDSessionRuntimeConfig,
}

impl OpenDProviderRuntimeConfig {
    pub fn with_defaults(
        router: Arc<Mutex<ProviderRouter>>,
        descriptor: ProviderDescriptor,
        opend: OpenDTcpProbeConfig,
        desired: Vec<InstrumentRef>,
        now_ms: i64,
    ) -> Self {
        Self {
            router,
            descriptor,
            opend,
            desired,
            now_ms,
            demand_consumer_id: "futu-opend-runtime".to_owned(),
            demand_managed: false,
            task: OpenDSessionRuntimeConfig::default(),
        }
    }
}

/// Single-owner bridge between a ProviderRouter and an authenticated OpenD
/// runtime task. It is never created by the default desktop profile.
pub struct OpenDProviderRuntime {
    router: Arc<Mutex<ProviderRouter>>,
    provider_id: String,
    demand_consumer_id: String,
    runtime: OpenDSessionRuntime,
}

impl std::fmt::Debug for OpenDProviderRuntime {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDProviderRuntime")
            .field("provider_id", &self.provider_id)
            .field("demand_consumer_id", &self.demand_consumer_id)
            .field("runtime", &self.runtime)
            .finish()
    }
}

#[derive(Debug, Error)]
pub enum OpenDProviderRuntimeError {
    #[error("probe Futu OpenD: {0}")]
    Probe(#[from] OpenDTcpProbeError),
    #[error("configure Futu provider: {0}")]
    Provider(#[from] jftrade_marketdata::MarketDataError),
    #[error("compose OpenD session: {0}")]
    Coordinator(#[from] OpenDSessionCoordinatorError),
    #[error("start OpenD provider task: {0}")]
    Runtime(#[from] OpenDSessionRuntimeError),
}

impl OpenDProviderRuntime {
    /// Probes, registers and explicitly activates one Futu provider, then
    /// starts the OpenD task against the router's recorder, demand and cache.
    /// No provider or socket is touched unless this function is called by a
    /// composition root.
    pub fn start(config: OpenDProviderRuntimeConfig) -> Result<Self, OpenDProviderRuntimeError> {
        let probe = OpenDTcpProbe::probe(config.opend.clone())?;
        let health: HealthStatus = market_data_health_from_probe(true, &probe);
        let provider_id = config.descriptor.selection_id.clone();
        let demand_consumer_id = config.demand_consumer_id.trim().to_owned();
        {
            let mut router = config
                .router
                .lock()
                .unwrap_or_else(|error| error.into_inner());
            router.register(config.descriptor, health)?;
            router.activate(&provider_id, ActivationMode::Explicit)?;
            if !config.desired.is_empty() {
                router.acquire_demand(
                    &demand_consumer_id,
                    config.desired.clone(),
                    config.demand_managed,
                    config.now_ms,
                )?;
            }
        }
        let recorder = config
            .router
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .runtime_recorder();
        let coordinator = match OpenDSessionCoordinator::connect(
            config.opend,
            recorder,
            config.desired,
            config.now_ms,
        ) {
            Ok(coordinator) => Arc::new(Mutex::new(coordinator)),
            Err(error) => {
                release_demand(&config.router, &demand_consumer_id);
                let _ = config
                    .router
                    .lock()
                    .unwrap_or_else(|poisoned| poisoned.into_inner())
                    .deactivate(&provider_id);
                return Err(error.into());
            }
        };
        let runtime = match OpenDSessionRuntime::start_with_provider_router(
            Arc::clone(&coordinator),
            Arc::clone(&config.router),
            config.task,
        ) {
            Ok(runtime) => runtime,
            Err(error) => {
                let _ = coordinator
                    .lock()
                    .unwrap_or_else(|poisoned| poisoned.into_inner())
                    .close();
                release_demand(&config.router, &demand_consumer_id);
                let _ = config
                    .router
                    .lock()
                    .unwrap_or_else(|poisoned| poisoned.into_inner())
                    .deactivate(&provider_id);
                return Err(error.into());
            }
        };
        Ok(Self {
            router: config.router,
            provider_id,
            demand_consumer_id,
            runtime,
        })
    }

    pub fn router(&self) -> Arc<Mutex<ProviderRouter>> {
        Arc::clone(&self.router)
    }

    pub fn provider_id(&self) -> &str {
        &self.provider_id
    }

    pub fn coordinator(&self) -> Arc<Mutex<OpenDSessionCoordinator>> {
        self.runtime.coordinator()
    }

    pub fn runtime(&self) -> &OpenDSessionRuntime {
        &self.runtime
    }

    pub fn shutdown(mut self) -> Result<(), OpenDProviderRuntimeError> {
        let result = self
            .runtime
            .shutdown()
            .map_err(OpenDProviderRuntimeError::from);
        release_and_deactivate(&self.router, &self.demand_consumer_id, &self.provider_id);
        result
    }
}

impl Drop for OpenDProviderRuntime {
    fn drop(&mut self) {
        let _ = self.runtime.shutdown();
        release_and_deactivate(&self.router, &self.demand_consumer_id, &self.provider_id);
    }
}

fn release_demand(router: &Arc<Mutex<ProviderRouter>>, consumer_id: &str) {
    if !consumer_id.is_empty() {
        let _ = router
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .release_demand(consumer_id);
    }
}

fn release_and_deactivate(
    router: &Arc<Mutex<ProviderRouter>>,
    consumer_id: &str,
    provider_id: &str,
) {
    release_demand(router, consumer_id);
    if provider_id.is_empty() {
        return;
    }
    let _ = router
        .lock()
        .unwrap_or_else(|error| error.into_inner())
        .deactivate(provider_id);
}

#[cfg(test)]
mod tests {
    use super::*;
    use jftrade_marketdata::{
        HealthStatus, ProviderCapabilities, ProviderConstraints, ProviderReadiness,
    };

    fn descriptor() -> ProviderDescriptor {
        ProviderDescriptor {
            selection_id: "futu".to_owned(),
            provider_id: "futu".to_owned(),
            display_name: "Futu OpenD".to_owned(),
            broker_id: Some("futu".to_owned()),
            source: "futu".to_owned(),
            default_market: "US".to_owned(),
            supported_markets: vec!["US".to_owned()],
            transports: vec!["stream".to_owned()],
            capabilities: ProviderCapabilities {
                snapshots: true,
                streaming_quotes: true,
                ..Default::default()
            },
            constraints: ProviderConstraints::default(),
            notes: Vec::new(),
        }
    }

    #[test]
    fn release_and_deactivate_clears_bridge_owned_router_state() {
        let router = Arc::new(Mutex::new(ProviderRouter::new(2)));
        {
            let mut guard = router.lock().expect("router");
            guard
                .register(
                    descriptor(),
                    HealthStatus {
                        connected: true,
                        readiness: ProviderReadiness::Ready,
                        stream_mode: "streaming".to_owned(),
                        ..Default::default()
                    },
                )
                .expect("register");
            guard
                .activate("futu", ActivationMode::Explicit)
                .expect("activate");
            guard
                .acquire_demand(
                    "futu-opend-runtime",
                    [InstrumentRef {
                        channel: "SNAPSHOT".to_owned(),
                        market: "US".to_owned(),
                        symbol: "AAPL".to_owned(),
                        interval: None,
                    }],
                    false,
                    0,
                )
                .expect("demand");
        }

        release_and_deactivate(&router, "futu-opend-runtime", "futu");

        let guard = router.lock().expect("router");
        assert!(guard.runtime().active_provider.is_empty());
        assert!(guard.demand().active.is_empty());
    }
}
