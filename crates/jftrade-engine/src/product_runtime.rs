use std::sync::{Arc, Mutex};
use std::time::{SystemTime, UNIX_EPOCH};

use jftrade_integration_futu::{
    OpenDPredictionMarketReader, OpenDProviderRuntime, OpenDProviderRuntimeConfig,
    OpenDSessionCoordinator, OpenDSessionRuntime, OpenDSessionRuntimeConfig, OpenDTradeReadClient,
    TradeReadPort, TradeWritePort,
};
use jftrade_integration_marketdata_helper::ProcessError;
use jftrade_integration_pine::{
    GrpcPineExecutionPort, PineBacktestExecutionAdapter, PineExecutionConfig, PineProcessError,
};
use jftrade_marketdata::{MarketDataRuntimeRecorder, ProviderRouter};
use jftrade_settings::MarketDataProvider;
use jftrade_store_settings_file::SettingsFileStore;
use jftrade_strategy::StrategyRuntimeRegistry;
use thiserror::Error;

use crate::product::{
    ActiveProviderState, ProductConfig, ProductError, ProductionRuntimeStatus,
    expose_prepared_product, prepare_product_with_runtime_state,
};

#[path = "product_runtime_workers.rs"]
mod product_runtime_workers;
pub use product_runtime_workers::{
    DesktopMarketDataRuntimeConfig, DesktopPineRuntimeConfig, DesktopRetainedRuntimeConfig,
    MarketDataHelperRuntimeConfig, PineWorkerRuntimeConfig,
};
use product_runtime_workers::{
    desktop_marketdata_helper, desktop_pine_workers, start_marketdata_helper, start_pine_worker,
};

#[path = "product_runtime_helper_health.rs"]
pub mod product_runtime_helper_health;
pub use product_runtime_helper_health::{
    HelperHealthMonitor, HelperHealthSnapshot, HelperRestartPolicy, compute_helper_backoff,
};

#[path = "product_runtime_provider_activation.rs"]
mod product_runtime_provider_activation;

#[path = "product_runtime_opend_listener.rs"]
mod product_runtime_opend_listener;
use product_runtime_opend_listener::LiveHubOpenDEventListener;

#[path = "product_runtime_composition.rs"]
mod product_runtime_composition;
use product_runtime_composition::{
    DynamicOpenDPhysicalSubscriptionAdapter, OpenDPhysicalSubscriptionAdapter,
    SharedOpenDProviderRuntime, compose_market_data_runtime,
};

#[derive(Clone, Debug)]
pub struct ProductRuntimeConfig {
    pub product: ProductConfig,
    pub pine_workers: Vec<PineWorkerRuntimeConfig>,
    pub marketdata_helper: Option<MarketDataHelperRuntimeConfig>,
    pub market_data_router: Option<Arc<Mutex<ProviderRouter>>>,
    pub market_data_runtime_recorder: Option<Arc<MarketDataRuntimeRecorder>>,
    /// Explicitly composed OpenD session. The caller owns demand/timer
    /// driving; the runtime only exposes the shared status recorder and closes
    /// the session during shutdown.
    pub market_data_opend: Option<Arc<Mutex<OpenDSessionCoordinator>>>,
    pub market_data_opend_task: Option<OpenDSessionRuntimeConfig>,
    /// Explicit Futu/OpenD provider bridge. This owns the router, health
    /// activation and runtime task as one composition unit.
    pub market_data_opend_provider: Option<OpenDProviderRuntimeConfig>,
    pub strategy_runtime_registry: Option<Arc<StrategyRuntimeRegistry>>,
    /// Explicit strategy/Pine/backtest execution adapter. When absent, a
    /// healthy configured Pine worker is composed into this port at startup;
    /// absent worker configuration keeps POST /api/v1/backtests fail closed.
    pub backtest_execution_port: Option<Arc<dyn crate::product::BacktestExecutionPort>>,
    /// Native PineTS AnalyzeScript adapter created after worker readiness.
    pub strategy_pine_worker_port: Option<Arc<jftrade_integration_pine::GrpcPineExecutionPort>>,
    pub(crate) shutdown_recorder: Option<product_runtime_supervisor::ShutdownEventRecorder>,
    #[cfg(test)]
    pub inject_startup_failure: bool,
}

pub struct ProductRuntimeBuilder {
    config: ProductRuntimeConfig,
}

impl ProductRuntimeBuilder {
    pub fn from_process_env() -> Result<Self, ProductRuntimeError> {
        let product = ProductConfig::from_process_env()?;
        let retained = DesktopRetainedRuntimeConfig::from_process_env();
        let mut config = ProductRuntimeConfig::desktop(product, retained)?;
        compose_market_data_runtime(&mut config)?;
        Ok(Self { config })
    }

    pub fn with_desktop_assets(
        product: ProductConfig,
        retained: DesktopRetainedRuntimeConfig,
    ) -> Result<Self, ProductRuntimeError> {
        let mut config = ProductRuntimeConfig::desktop(product, retained)?;
        compose_market_data_runtime(&mut config)?;
        Ok(Self { config })
    }

    pub async fn start(self) -> Result<ProductRuntimeHandle, ProductRuntimeError> {
        start_product_runtime(self.config).await
    }
}

impl ProductRuntimeConfig {
    pub fn desktop(
        product: ProductConfig,
        retained: DesktopRetainedRuntimeConfig,
    ) -> Result<Self, ProductRuntimeError> {
        let pine_workers = retained
            .pine
            .map(desktop_pine_workers)
            .transpose()?
            .unwrap_or_default();
        let marketdata_helper = retained
            .marketdata
            .map(desktop_marketdata_helper)
            .transpose()?;
        Ok(Self {
            product,
            pine_workers,
            marketdata_helper,
            market_data_router: None,
            market_data_runtime_recorder: None,
            market_data_opend: None,
            market_data_opend_task: None,
            market_data_opend_provider: None,
            strategy_runtime_registry: None,
            backtest_execution_port: None,
            strategy_pine_worker_port: None,
            shutdown_recorder: None,
            #[cfg(test)]
            inject_startup_failure: false,
        })
    }

    pub fn with_market_data_runtime_recorder(
        mut self,
        recorder: Arc<MarketDataRuntimeRecorder>,
    ) -> Self {
        self.market_data_runtime_recorder = Some(recorder);
        self
    }

    pub fn with_market_data_router(mut self, router: Arc<Mutex<ProviderRouter>>) -> Self {
        self.market_data_router = Some(router);
        self.market_data_runtime_recorder = None;
        self
    }

    pub fn with_opend_session_coordinator(
        mut self,
        coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
    ) -> Self {
        let recorder = coordinator
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .lifecycle()
            .recorder();
        self.market_data_opend = Some(coordinator);
        self.market_data_opend_task = None;
        self.market_data_runtime_recorder = Some(recorder);
        self
    }

    pub fn with_opend_session_runtime(
        mut self,
        coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
        task: OpenDSessionRuntimeConfig,
    ) -> Self {
        self = self.with_opend_session_coordinator(coordinator);
        self.market_data_opend_task = Some(task);
        self
    }

    pub fn with_opend_provider_runtime(mut self, provider: OpenDProviderRuntimeConfig) -> Self {
        self.market_data_opend_provider = Some(provider);
        self
    }

    pub fn with_strategy_runtime_registry(
        mut self,
        registry: Arc<StrategyRuntimeRegistry>,
    ) -> Self {
        self.strategy_runtime_registry = Some(registry);
        self
    }

    pub fn with_backtest_execution_port(
        mut self,
        port: Arc<dyn crate::product::BacktestExecutionPort>,
    ) -> Self {
        self.backtest_execution_port = Some(port);
        self
    }

    pub fn with_strategy_pine_worker_port(
        mut self,
        port: Arc<jftrade_integration_pine::GrpcPineExecutionPort>,
    ) -> Self {
        self.strategy_pine_worker_port = Some(port);
        self
    }
}

#[path = "product_runtime_resources.rs"]
mod product_runtime_resources;
pub(crate) use product_runtime_resources::ProductRuntimeState;
pub use product_runtime_resources::RuntimeResourceDescriptor;

#[path = "product_runtime_supervisor.rs"]
mod product_runtime_supervisor;
pub(crate) use product_runtime_supervisor::ProductShutdownSupervisor;
#[cfg(test)]
pub(crate) use product_runtime_supervisor::ShutdownEventRecorder;

#[path = "product_runtime_start.rs"]
mod product_runtime_start;
pub use product_runtime_start::start_product_runtime;

pub struct ProductRuntimeHandle {
    supervisor: ProductShutdownSupervisor,
    market_data_router: Option<Arc<Mutex<ProviderRouter>>>,
}

impl ProductRuntimeHandle {
    pub fn startup_record(&self) -> &crate::product::ProductStartupRecord {
        self.supervisor
            .product
            .as_ref()
            .expect("running product runtime must own its product handle")
            .startup_record()
    }

    #[cfg(test)]
    pub(crate) fn database_leases(
        &self,
    ) -> Option<&crate::product::product_production_ports::ProductionDatabaseLeaseSnapshot> {
        self.supervisor
            .production_ports
            .as_ref()
            .map(|ports| ports.database_leases())
    }

    #[cfg(test)]
    pub(crate) fn helper_health(&self) -> Option<Arc<HelperHealthMonitor>> {
        self.supervisor.helper_health.clone()
    }

    #[cfg(test)]
    pub(crate) fn backtest_execution_ready(&self) -> Option<bool> {
        self.supervisor
            .production_ports
            .as_ref()
            .map(|ports| ports.backtest_execution_ready())
    }

    #[cfg(test)]
    pub(crate) fn shutdown_recorder(&self) -> ShutdownEventRecorder {
        self.supervisor.recorder.clone()
    }

    /// Returns the single live event hub owned by the Rust product listener.
    /// Provider runtimes may publish already-shaped tick/depth/error events
    /// through this handle without creating a second websocket owner.
    pub fn live_hub(&self) -> Option<Arc<jftrade_api::LiveHub>> {
        self.supervisor
            .product
            .as_ref()
            .map(crate::product::ProductHandle::live_hub)
    }

    pub fn market_data_router(&self) -> Option<Arc<Mutex<ProviderRouter>>> {
        self.market_data_router.clone().or_else(|| {
            self.supervisor
                .market_data_opend_provider
                .as_ref()
                .map(OpenDProviderRuntime::router)
        })
    }

    pub fn market_data_opend(&self) -> Option<Arc<Mutex<OpenDSessionCoordinator>>> {
        self.supervisor
            .market_data_dynamic_opend
            .as_ref()
            .and_then(|runtime| {
                runtime
                    .lock()
                    .unwrap_or_else(|error| error.into_inner())
                    .as_ref()
                    .map(OpenDProviderRuntime::coordinator)
            })
            .or_else(|| self.supervisor.market_data_opend.clone())
            .or_else(|| {
                self.supervisor
                    .market_data_opend_provider
                    .as_ref()
                    .map(OpenDProviderRuntime::coordinator)
            })
    }

    /// Compatibility accessor for callers that need the explicitly composed
    /// session task. Dynamically activated Futu uses the status accessor
    /// below because its owner is held behind a runtime mutex.
    pub fn market_data_opend_runtime(&self) -> Option<&OpenDSessionRuntime> {
        self.supervisor.market_data_opend_runtime.as_ref()
    }

    pub fn market_data_opend_runtime_status(
        &self,
    ) -> Option<jftrade_integration_futu::OpenDSessionRuntimeStatus> {
        if let Some(status) =
            self.supervisor
                .market_data_dynamic_opend
                .as_ref()
                .and_then(|runtime| {
                    runtime
                        .lock()
                        .unwrap_or_else(|error| error.into_inner())
                        .as_ref()
                        .map(|provider| provider.runtime().status())
                })
        {
            return Some(status);
        }
        self.supervisor
            .market_data_opend_runtime
            .as_ref()
            .map(OpenDSessionRuntime::status)
            .or_else(|| {
                self.supervisor
                    .market_data_opend_provider
                    .as_ref()
                    .map(|provider| provider.runtime().status())
            })
    }

    /// Wake the execution reconciliation worker after an OpenD ready or
    /// reconnect transition.  The worker also keeps a bounded timer cadence,
    /// so this hint is safe to call when no worker was composed.
    pub fn wake_execution_reconciliation(&self) -> bool {
        let Some(worker) = self.supervisor.execution_reconciliation_worker.as_ref() else {
            return false;
        };
        worker.wake();
        true
    }

    pub fn set_market_data_opend_demand(
        &self,
        demand: Vec<jftrade_marketdata::InstrumentRef>,
    ) -> bool {
        if let Some(result) =
            self.supervisor
                .market_data_dynamic_opend
                .as_ref()
                .and_then(|runtime| {
                    runtime
                        .lock()
                        .unwrap_or_else(|error| error.into_inner())
                        .as_ref()
                        .map(|provider| provider.set_demand(demand.clone(), current_unix_millis()))
                })
        {
            return result.is_ok();
        }
        if let Some(provider) = self.supervisor.market_data_opend_provider.as_ref() {
            return provider.set_demand(demand, current_unix_millis()).is_ok();
        }
        let Some(runtime) = self.supervisor.market_data_opend_runtime.as_ref() else {
            return false;
        };
        runtime.set_demand(demand);
        true
    }

    pub async fn shutdown(mut self) -> Result<(), ProductRuntimeError> {
        self.supervisor.execute_shutdown().await
    }
}

fn current_unix_millis() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .ok()
        .and_then(|duration| i64::try_from(duration.as_millis()).ok())
        .unwrap_or_default()
}

impl Drop for ProductRuntimeHandle {
    fn drop(&mut self) {
        self.supervisor.execute_sync_drop();
    }
}

#[derive(Debug, Error)]
pub enum ProductRuntimeError {
    #[error(transparent)]
    Product(#[from] ProductError),
    #[error("start PineTS worker: {0}")]
    Pine(#[source] PineProcessError),
    #[error("configure PineTS backtest execution port: {0}")]
    PineExecution(String),
    #[error("configure market-data helper client: {0}")]
    HelperClient(#[from] jftrade_integration_marketdata_helper::HttpAdapterError),
    #[error("manage market-data helper: {0}")]
    HelperProcess(#[from] ProcessError),
    #[error("desktop PineTS worker count must be greater than zero")]
    InvalidWorkerCount,
    #[error(
        "production runtime does not support PineTS worker failover; configured {configured} workers (maximum 1)"
    )]
    PineWorkerFailoverUnsupported { configured: usize },
    #[error("market-data router and OpenD session cannot share one runtime owner")]
    ConflictingMarketDataOwners,
    #[error("OpenD runtime task requires an explicitly composed OpenD session")]
    MissingOpenDSession,
    #[error("manage OpenD runtime task: {0}")]
    OpenDTask(#[from] jftrade_integration_futu::OpenDSessionRuntimeError),
    #[error("compose OpenD provider runtime: {0}")]
    OpenDProvider(#[from] jftrade_integration_futu::OpenDProviderRuntimeError),
    #[error("market-data runtime settings: {0}")]
    Settings(String),
    #[error("stop Rust product runtime: {0}")]
    Shutdown(String),
}

#[cfg(test)]
#[path = "product_runtime_tests.rs"]
mod product_runtime_tests;
