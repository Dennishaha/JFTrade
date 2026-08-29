use std::sync::{Arc, Mutex};
use std::time::{SystemTime, UNIX_EPOCH};

use jftrade_integration_futu::{
    OpenDProviderRuntime, OpenDProviderRuntimeConfig, OpenDSessionCoordinator, OpenDSessionRuntime,
    OpenDSessionRuntimeConfig, OpenDTradeReadClient, TradeReadPort,
};
use jftrade_integration_marketdata_helper::ProcessError;
use jftrade_integration_pine::PineProcessError;
use jftrade_marketdata::{MarketDataRuntimeRecorder, ProviderRouter};
use jftrade_settings::{
    MarketDataProvider, MarketDataProviderSettingsStorePort, normalize_market_data_provider,
};
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
mod product_runtime_helper_health;
pub(crate) use product_runtime_helper_health::HelperHealthMonitor;

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
    /// Explicit strategy/Pine/backtest execution adapter.  It is intentionally
    /// opt-in; absent configuration makes POST /api/v1/backtests fail closed.
    pub backtest_execution_port: Option<Arc<dyn crate::product::BacktestExecutionPort>>,
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

pub async fn start_product_runtime(
    mut config: ProductRuntimeConfig,
) -> Result<ProductRuntimeHandle, ProductRuntimeError> {
    if config.market_data_router.is_none()
        && config.market_data_opend_provider.is_none()
        && config.market_data_opend.is_none()
        && config.market_data_opend_task.is_none()
    {
        compose_market_data_runtime(&mut config)?;
    }
    let opend_configured = config.market_data_opend.is_some()
        || config.market_data_opend_task.is_some()
        || config.market_data_opend_provider.is_some();
    let opend_task_configured = config.market_data_opend_task.is_some();
    let helper_configured = config.marketdata_helper.is_some();
    let pine_workers_configured = !config.pine_workers.is_empty();
    let provider_configured = opend_configured
        || config.market_data_router.is_some()
        || config.market_data_runtime_recorder.is_some()
        || config.marketdata_helper.is_some();
    let worker_configured = pine_workers_configured || helper_configured;
    if config.market_data_opend_provider.is_some()
        && (config.market_data_runtime_recorder.is_some()
            || config.market_data_opend.is_some()
            || config.market_data_opend_task.is_some())
    {
        return Err(ProductRuntimeError::ConflictingMarketDataOwners);
    }
    if config.market_data_opend_task.is_some() && config.market_data_opend.is_none() {
        return Err(ProductRuntimeError::MissingOpenDSession);
    }
    // Validate/create the production schema before any external worker or
    // provider is started.  A migration failure therefore cannot leave an
    // external process running while the API is unable to serve its stores.
    if config.product.is_production() {
        crate::product_data_management::initialize_production_databases(
            config.product.settings_path(),
        )
        .map_err(ProductError::Storage)?;
    }
    let live_hub = config
        .product
        .live_hub
        .clone()
        .unwrap_or_else(|| Arc::new(jftrade_api::LiveHub::default()));
    config.product = config.product.with_live_hub(Arc::clone(&live_hub));
    let trade_runtime =
        Arc::new(crate::product::product_production_ports::SharedTradeReadRuntime::default());
    config.product = config
        .product
        .with_trade_runtime(Arc::clone(&trade_runtime));

    let market_data_router = config.market_data_router.take();
    let market_data_opend = config.market_data_opend.take();
    let mut market_data_opend_task = config.market_data_opend_task.take();
    if let Some(task) = market_data_opend_task.as_mut()
        && task.event_listener.is_none()
    {
        task.event_listener = Some(Arc::new(LiveHubOpenDEventListener::new(Arc::clone(
            &live_hub,
        ))));
    }
    let (market_data_opend_provider, market_data_router) =
        if let Some(mut provider) = config.market_data_opend_provider.clone() {
            let shared_router = Arc::clone(&provider.router);
            if provider.task.event_listener.is_none() {
                provider.task.event_listener = Some(Arc::new(LiveHubOpenDEventListener::new(
                    Arc::clone(&live_hub),
                )));
            }
            match OpenDProviderRuntime::start(provider) {
                Ok(runtime) => {
                    let trade_logged_in = runtime.trade_logged_in();
                    let trade_read_port = runtime
                        .coordinator()
                        .lock()
                        .ok()
                        .and_then(|coordinator| {
                            OpenDTradeReadClient::from_coordinator(&coordinator).ok()
                        })
                        .map(|client| Arc::new(client) as Arc<dyn TradeReadPort>);
                    let historical_reader = {
                        let coordinator = runtime.coordinator();
                        Arc::new(jftrade_integration_futu::OpenDHistoricalKlineReader::new(
                            coordinator,
                        ))
                            as Arc<dyn jftrade_integration_futu::HistoricalKlineReadPort>
                    };
                    config.product = config
                        .product
                        .clone()
                        .with_trade_read_port(trade_read_port, trade_logged_in);
                    trade_runtime.set(config.product.trade_read_port.clone(), trade_logged_in);
                    trade_runtime.set_historical_klines(Some(historical_reader));
                    (Some(runtime), Some(shared_router))
                }
                Err(error) => {
                    eprintln!("Warning: OpenD provider runtime failed to connect: {error}");
                    (None, Some(shared_router))
                }
            }
        } else {
            (None, market_data_router)
        };
    let market_data_runtime_recorder = if let Some(provider) = market_data_opend_provider.as_ref() {
        Some(
            provider
                .router()
                .lock()
                .unwrap_or_else(|error| error.into_inner())
                .runtime_recorder(),
        )
    } else if let Some(router) = market_data_router.as_ref() {
        Some(
            router
                .lock()
                .unwrap_or_else(|error| error.into_inner())
                .runtime_recorder(),
        )
    } else if let Some(coordinator) = market_data_opend.as_ref() {
        Some(
            coordinator
                .lock()
                .unwrap_or_else(|error| error.into_inner())
                .lifecycle()
                .recorder(),
        )
    } else {
        config.market_data_runtime_recorder.take()
    };
    if config.product.trade_read_port.is_none()
        && let Some(coordinator) = market_data_opend.as_ref()
        && let Ok(guard) = coordinator.lock()
        && let Ok(client) = OpenDTradeReadClient::from_coordinator(&guard)
    {
        config.product = config
            .product
            .clone()
            .with_trade_read_port(Some(Arc::new(client)), None);
    }
    if let Some(recorder) = market_data_runtime_recorder.as_ref() {
        config.product = config
            .product
            .with_market_data_runtime_status_port(recorder.clone());
    }
    if let Some(router) = market_data_router.as_ref() {
        config.product = config.product.with_market_data_router(Arc::clone(router));
    }
    let dynamic_opend: SharedOpenDProviderRuntime =
        Arc::new(Mutex::new(market_data_opend_provider));
    if market_data_router.is_some() {
        config.product = config.product.with_physical_subscription_port(Arc::new(
            DynamicOpenDPhysicalSubscriptionAdapter {
                runtime: Arc::clone(&dynamic_opend),
            },
        ));
    }
    if let Some(coordinator) = market_data_opend.as_ref()
        && config.product.physical_subscription_port.is_none()
    {
        config.product = config.product.with_physical_subscription_port(Arc::new(
            OpenDPhysicalSubscriptionAdapter {
                coordinator: Arc::clone(coordinator),
            },
        ));
    }
    if let Some(registry) = config.strategy_runtime_registry.take() {
        config.product = config.product.with_strategy_runtime_status_port(registry);
    }
    if let Some(port) = config.backtest_execution_port.take() {
        config.product = config.product.with_backtest_execution_port(port);
    }
    let state = ProductRuntimeState::configured(&config);
    let mut supervisor = if let Some(recorder) = config.shutdown_recorder.take() {
        ProductShutdownSupervisor::with_recorder(recorder)
    } else {
        ProductShutdownSupervisor::new()
    };
    supervisor.market_data_dynamic_opend = Some(Arc::clone(&dynamic_opend));
    supervisor.market_data_opend = market_data_opend;

    if let (Some(coordinator), Some(task_config)) = (
        supervisor.market_data_opend.as_ref(),
        market_data_opend_task,
    ) {
        match OpenDSessionRuntime::start(Arc::clone(coordinator), task_config) {
            Ok(task) => supervisor.market_data_opend_runtime = Some(task),
            Err(error) => {
                eprintln!("Warning: OpenD session runtime failed to connect: {error}");
            }
        }
    }

    for worker in std::mem::take(&mut config.pine_workers) {
        let result = start_pine_worker(worker).await;
        match result {
            Ok((process, _health)) => {
                supervisor.pine_workers.push(process);
            }
            Err(error) => {
                let _ = supervisor.execute_shutdown().await;
                return Err(ProductRuntimeError::Pine(error));
            }
        }
    }

    let helper_process = if let Some(helper) = config.marketdata_helper.take() {
        match start_marketdata_helper(helper).await {
            Ok((process, client, monitor)) => {
                config.product = config.product.with_market_data_helper(client);
                supervisor.helper_health = Some(Arc::clone(&monitor));
                Some(Arc::new(Mutex::new(Some(process))))
            }
            Err(error) => {
                let _ = supervisor.execute_shutdown().await;
                return Err(error);
            }
        }
    } else {
        None
    };
    supervisor.marketdata_helper = helper_process.clone();

    let initial_provider = if dynamic_opend
        .lock()
        .unwrap_or_else(|error| error.into_inner())
        .is_some()
    {
        Some(MarketDataProvider::Futu)
    } else {
        let settings_file = std::path::Path::new(config.product.settings_path());
        if settings_file.exists() {
            let store = SettingsFileStore::open_read_only(config.product.settings_path())
                .map_err(|error| ProductRuntimeError::Settings(error.to_string()))?;
            store
                .load_active_market_data_provider()
                .map_err(|error| ProductRuntimeError::Settings(error.to_string()))?
                .map(|provider| normalize_market_data_provider(&provider))
        } else {
            None
        }
    };
    let settings_path = config.product.settings_path().to_owned();
    let dynamic_readiness = product_runtime_provider_activation::dynamic_provider_readiness(
        &helper_process,
        supervisor.helper_health.clone(),
        &dynamic_opend,
        &market_data_router,
    );
    let activation = product_runtime_provider_activation::provider_activation(
        &helper_process,
        supervisor.helper_health.clone(),
        &dynamic_opend,
        &market_data_router,
        &live_hub,
        &settings_path,
        Arc::clone(&trade_runtime),
    )?;
    let active_provider_state = Arc::new(
        ActiveProviderState::new(initial_provider)
            .with_dynamic_readiness(dynamic_readiness)
            .with_activation(activation),
    );
    supervisor.active_provider_state = Some(Arc::clone(&active_provider_state));
    active_provider_state.set_readiness(
        config.product.market_data_helper.is_some() || supervisor.marketdata_helper.is_some(),
        dynamic_opend
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .is_some(),
        market_data_router.is_some(),
    );
    config.product = config
        .product
        .with_active_provider_state(active_provider_state);

    let provider_status = if (helper_configured && supervisor.marketdata_helper.is_none())
        || (opend_task_configured && supervisor.market_data_opend_runtime.is_none())
    {
        ProductionRuntimeStatus::Unavailable
    } else {
        production_provider_status(provider_configured, market_data_runtime_recorder.as_deref())
    };

    let opend_status = if opend_configured {
        if opend_task_configured && supervisor.market_data_opend_runtime.is_none() {
            ProductionRuntimeStatus::Unavailable
        } else {
            provider_status
        }
    } else {
        ProductionRuntimeStatus::Unavailable
    };

    let worker_status = if worker_configured {
        let helper_ok = if helper_configured {
            supervisor.marketdata_helper.is_some()
        } else {
            true
        };
        let pine_ok = if pine_workers_configured {
            !supervisor.pine_workers.is_empty()
        } else {
            true
        };
        if helper_ok && pine_ok {
            ProductionRuntimeStatus::Ready
        } else if supervisor.marketdata_helper.is_some() || !supervisor.pine_workers.is_empty() {
            ProductionRuntimeStatus::Degraded
        } else {
            ProductionRuntimeStatus::Unavailable
        }
    } else {
        ProductionRuntimeStatus::Unavailable
    };
    config.product = config.product.with_production_runtime_statuses(
        provider_status,
        opend_status,
        worker_status,
    );

    // Production ports, the 9 SQLite WriterLeases and every route adapter are
    // constructed inside `prepare_product_with_runtime_state`, while the HTTP
    // listener is not yet accepting traffic.  A fault between the two is
    // recovered by the supervisor's reverse-order rollback below, which must
    // release the provider/OpenD/helper/Pine resources and the port bundle so
    // every WriterLease can be re-acquired afterwards.
    let prepared =
        match prepare_product_with_runtime_state(config.product, Arc::clone(&state)).await {
            Ok(prepared) => prepared,
            Err(error) => {
                let _ = supervisor.execute_shutdown().await;
                return Err(error.into());
            }
        };

    #[cfg(test)]
    if config.inject_startup_failure {
        let ports = {
            let mut prepared = prepared;
            prepared.handle.take_production_ports()
        };
        supervisor.backtest_sync_workers = ports.as_ref().map(
            crate::product::product_production_ports::ProductionPortBundle::backtest_sync_workers,
        );
        supervisor.backtest_execution_workers = ports.as_ref().map(
            crate::product::product_production_ports::ProductionPortBundle::backtest_execution_workers,
        );
        supervisor.production_ports = ports;
        let _ = supervisor.execute_shutdown().await;
        return Err(ProductError::RouteRegistry(
            "injected startup fault after production lease acquisition, before HTTP exposure"
                .to_owned(),
        )
        .into());
    }

    match expose_prepared_product(prepared) {
        Ok(mut product) => {
            let ports = product.take_production_ports();
            supervisor.backtest_sync_workers = ports
                .as_ref()
                .map(crate::product::product_production_ports::ProductionPortBundle::backtest_sync_workers);
            supervisor.backtest_execution_workers = ports
                .as_ref()
                .map(crate::product::product_production_ports::ProductionPortBundle::backtest_execution_workers);
            supervisor.production_ports = ports;
            supervisor.product = Some(product);
        }
        Err(error) => {
            let _ = supervisor.execute_shutdown().await;
            return Err(error.into());
        }
    }
    Ok(ProductRuntimeHandle {
        supervisor,
        market_data_router,
    })
}

fn production_provider_status(
    configured: bool,
    recorder: Option<&MarketDataRuntimeRecorder>,
) -> ProductionRuntimeStatus {
    let Some(state) = recorder.map(MarketDataRuntimeRecorder::snapshot) else {
        return if configured {
            ProductionRuntimeStatus::Degraded
        } else {
            ProductionRuntimeStatus::Unavailable
        };
    };
    if state.closed {
        ProductionRuntimeStatus::Failed
    } else if state.connected {
        ProductionRuntimeStatus::Ready
    } else {
        ProductionRuntimeStatus::Degraded
    }
}

#[derive(Debug, Error)]
pub enum ProductRuntimeError {
    #[error(transparent)]
    Product(#[from] ProductError),
    #[error("start PineTS worker: {0}")]
    Pine(#[source] PineProcessError),
    #[error("configure market-data helper client: {0}")]
    HelperClient(#[from] jftrade_integration_marketdata_helper::HttpAdapterError),
    #[error("manage market-data helper: {0}")]
    HelperProcess(#[from] ProcessError),
    #[error("desktop PineTS worker count must be greater than zero")]
    InvalidWorkerCount,
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
