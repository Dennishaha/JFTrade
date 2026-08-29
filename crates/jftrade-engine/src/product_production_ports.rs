//! Production projections and adapter wiring for the Rust composition root.
//!
//! When `config.production` is true, all domain ports connect to the authoritative
//! SQLite databases (under `production.v1` lease profile) and production services
//! without falling back to test cutover or dummy fixtures.

use std::collections::BTreeMap;
use std::path::PathBuf;
use std::sync::Arc;

use jftrade_calendar::{CalendarManager, CalendarManagerSettings, CalendarSourceRegistry};
use jftrade_api::WebSessionValidator;
use jftrade_datamanagement::{
    DATABASE_ADK, DATABASE_ADK_ARTIFACT, DATABASE_ADK_SESSION, DATABASE_BACKTEST_RUNS,
    DATABASE_BACKTEST, DATABASE_EXECUTION, DATABASE_RESEARCH, DATABASE_STRATEGY,
    DATABASE_WATCHLIST,
};
use jftrade_settings::{
    BrokerSettingsStorePort, InterfaceSettingsStorePort,
    MarketDataProviderSettingsStorePort, SecuritySettingsService,
    normalize_live_websocket_connection_limit, normalize_market_data_provider,
};
use jftrade_store_settings_file::SettingsFileStore;
use jftrade_store_sqlite::{
    ADK_ARTIFACT_PRODUCTION_PROFILE, ADK_PRODUCTION_PROFILE, ADK_SESSION_PRODUCTION_PROFILE,
    AdkArtifactStore, AdkSessionStore, AdkStore, BACKTEST_MARKET_DATA_PRODUCTION_PROFILE,
    BACKTEST_RUNS_PRODUCTION_PROFILE, BacktestMarketDataStore, BacktestRunStore,
    BacktestSyncTaskStore,
    EXECUTION_ORDERS_PRODUCTION_PROFILE, ExecutionOrderStore,
    RESEARCH_PRESET_PRODUCTION_PROFILE, ResearchPresetStore,
    STRATEGY_DEFINITION_PRODUCTION_PROFILE, StrategyDefinitionStore, StrategyRuntimeStore,
    WATCHLIST_PRODUCTION_PROFILE, WatchlistStore,
};
use serde_json::Value;

use crate::product::product_market_data_provider_actions_port::MarketDataProviderActionsPort;
use crate::product::product_market_data_subscription_mutation_port::MarketDataSubscriptionMutationPort;
use crate::product::product_research_screen_write_port::ResearchScreenWritePort;
use crate::product::product_system_write_port::SystemWritePort;
use crate::product::product_watchlist_remote_write_port::RemoteWatchlistWritePort;
use crate::product::strategy_pine::StrategyPineAnalyzeSnapshotPort;
use crate::product::{
    MarketDataDerivativeReadSnapshotPort, MarketDataNewsActionsReadSnapshotPort,
    MarketDataNewsSearchReadSnapshotPort, MarketDataOptionsReadSnapshotPort,
    MarketDataPredictionReadSnapshotPort, MarketDataQuoteReadSnapshotPort,
    PortfolioSnapshotPort, RemoteWatchlistSnapshotPort,
    ResearchReadSnapshotPort, WsLiveSnapshotPort,
};

#[path = "product_production_ports_plugins.rs"]
mod product_production_ports_plugins;
#[path = "product_production_ports_unavailable.rs"]
mod product_production_ports_unavailable;
#[path = "product_production_ports_watchlist.rs"]
mod product_production_ports_watchlist;
#[path = "product_production_ports_strategy.rs"]
mod product_production_ports_strategy;
#[path = "product_production_ports_execution.rs"]
mod product_production_ports_execution;
#[path = "product_production_ports_trade.rs"]
mod product_production_ports_trade;
#[path = "product_backtest_sync_registry.rs"]
mod product_backtest_sync_registry;
#[path = "product_production_ports_system.rs"]
mod product_production_ports_system;
#[path = "product_production_ports_market_data.rs"]
mod product_production_ports_market_data;
#[path = "product_production_ports_provider.rs"]
mod product_production_ports_provider;
#[path = "product_production_ports_adk.rs"]
mod product_production_ports_adk;
#[path = "product_production_adapter_bindings.rs"]
mod product_production_adapter_bindings;

pub(crate) use product_production_ports_execution::{
    ProductionBacktestPort, ProductionExecutionPort,
};
pub(crate) use crate::product::product_backtest_execution::BacktestExecutionTaskRegistry;
pub(crate) use product_production_ports_trade::{ProductionBrokerPort, ProductionPortfolioPort};
pub(crate) use product_production_ports_trade::SharedTradeReadRuntime;
pub(crate) use product_backtest_sync_registry::BacktestSyncWorkerRegistry;
pub(crate) use product_production_ports_market_data::{
    ProductionMarketDataCatalogPort, ProductionMarketDataDerivativePort,
    ProductionMarketDataNewsPort, ProductionMarketDataOptionsPort,
    ProductionMarketDataPredictionPort, ProductionMarketDataProviderActionsPort,
    ProductionMarketDataQuotePort, ProductionMarketDataSubscriptionMutationPort,
};
pub(crate) use product_production_ports_provider::ProductionMarketDataProviderPort;
pub(crate) use product_production_ports_provider::provider_now_rfc3339;
pub(crate) use product_production_ports_plugins::ProductionPluginPort;
pub(crate) use product_production_ports_unavailable::ProductionWsLivePort;
pub(crate) use product_production_ports_adk::{ProductionAdkPort, ProductionToolCatalog};
pub(crate) use product_production_adapter_bindings::{
    MarketDataCapabilityMatrix, ProductionAdapterBinding,
};
pub(crate) use product_production_adapter_bindings::production_adapter_bindings;
pub(crate) use product_production_ports_system::{
    ProductionSystemPort, ProductionSystemWritePort,
};
pub(crate) use product_production_ports_strategy::{
    ProductionResearchPort, ProductionResearchPresetPort, ProductionResearchScreenPort,
    ProductionStrategyDefinitionPort, ProductionStrategyPinePort,
    ProductionStrategyRuntimePort,
};
pub(crate) use product_production_ports_watchlist::{
    ProductionRemoteWatchlistPort, ProductionWatchlistPort,
};

use crate::product::product_adk_chat_stream_port::AdkChatStreamPort;
use crate::product::product_adk_mutation_port::AdkMutationPort;
use crate::product::product_alerts_write_port::{
    AlertWriteAction, AlertWritePort, AlertWritePortError, AlertWriteResolution, AlertWriteRoute,
};
use crate::product::product_auth_session_manager::{
    AuthSessionInvalidationPort, ProductionAuthSessionManager,
};
use crate::product::product_backtests_write_port::BacktestsWritePort;
use crate::product::product_brokers_write_port::BrokersWritePort;
use crate::product::product_execution_write_port::ExecutionWritePort;
use crate::product::product_plugins_write_port::PluginWritePort;
use crate::product::product_research_preset_write_port::ResearchPresetWritePort;
use crate::product::product_strategy_definition_write_port::StrategyDefinitionWritePort;
use crate::product::product_strategy_runtime_write_port::StrategyRuntimeWritePort;
use crate::product::product_watchlist_write_port::WatchlistWritePort;
use crate::product::{
    AdkReadSnapshotPort, AlertKind, AlertSnapshotError, AlertSnapshotPort,
    AuthSessionSnapshotPort, AuthSessionWritePort, BacktestReadSnapshotPort,
    BacktestSyncReadSnapshotPort, BrokerReadSnapshotPort,
    ExecutionReadSnapshotPort,
    MarketDataCatalogReadSnapshotPort,
    MarketDataProviderReadSnapshotPort, PluginSnapshotPort,
    PluginUninstallGuidanceSnapshotPort, ProductConfig, ResearchPresetReadSnapshotPort,
    StrategyDefinitionSnapshotPort, StrategyReadSnapshotPort, StrategyRuntimeStatusPort,
    SystemReadSnapshotPort, WatchlistMembershipSnapshotPort,
    WatchlistReadSnapshotPort, product_data_management,
};
use crate::product::product_active_provider_state::ActiveProviderState;

// Plugins, Brokers, Alerts, System

#[derive(Clone, Debug)]
pub(crate) struct ProductionAlertPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
}

impl AlertSnapshotPort for ProductionAlertPort {
    fn snapshot(
        &self,
        _kind: AlertKind,
        _raw_query: &str,
    ) -> Result<Value, AlertSnapshotError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() || !snapshot.opend_ready {
            return Err(AlertSnapshotError::Unavailable(
                "alert provider runtime is not configured".to_owned(),
            ));
        }
        Err(AlertSnapshotError::Unavailable(
            "alert provider runtime is not configured".to_owned(),
        ))
    }
}

impl AlertWritePort for ProductionAlertPort {
    fn resolve(
        &self,
        _route: AlertWriteRoute,
        _broker_id: Option<&str>,
        _account_id: Option<&str>,
    ) -> Result<AlertWriteResolution, AlertWritePortError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() || !snapshot.opend_ready {
            return Err(AlertWritePortError::Unavailable(
                "alert provider runtime is not configured".to_owned(),
            ));
        }
        Err(AlertWritePortError::Unavailable(
            "alert provider runtime is not configured".to_owned(),
        ))
    }

    fn apply(
        &self,
        _resolution: &AlertWriteResolution,
        _action: &AlertWriteAction,
    ) -> Result<Option<Value>, AlertWritePortError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() || !snapshot.opend_ready {
            return Err(AlertWritePortError::Unavailable(
                "alert provider runtime is not configured".to_owned(),
            ));
        }
        Err(AlertWritePortError::Unavailable(
            "alert provider runtime is not configured".to_owned(),
        ))
    }
}

pub const PRODUCTION_DATABASE_IDS: [&str; 9] = [
    DATABASE_WATCHLIST,
    DATABASE_STRATEGY,
    DATABASE_RESEARCH,
    DATABASE_BACKTEST_RUNS,
    DATABASE_BACKTEST,
    DATABASE_EXECUTION,
    DATABASE_ADK,
    DATABASE_ADK_SESSION,
    DATABASE_ADK_ARTIFACT,
];

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ProductionDatabaseLeaseSnapshot {
    pub expected: usize,
    pub acquired: usize,
    pub databases: Vec<String>,
    pub status: &'static str,
}

impl ProductionDatabaseLeaseSnapshot {
    pub fn new(acquired_databases: Vec<String>) -> Self {
        let expected = PRODUCTION_DATABASE_IDS.len();
        let acquired = acquired_databases.len();
        let status = if acquired == expected && expected > 0 {
            "acquired"
        } else if acquired == 0 {
            "none"
        } else {
            "partial"
        };
        Self {
            expected,
            acquired,
            databases: acquired_databases,
            status,
        }
    }
}

// Bundle

#[derive(Clone)]
pub(crate) struct ProductionPortBundle {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
    // Lease evidence snapshot; consumed by test-support accessors and the
    // system port so integrity reporting never invents "ok".
    #[cfg_attr(not(test), allow(dead_code))]
    pub(crate) database_leases: ProductionDatabaseLeaseSnapshot,
    pub database_lease_status: &'static str,
    pub provider_status: &'static str,
    pub opend_status: &'static str,
    pub worker_status: &'static str,
    pub calendar_manager: Arc<CalendarManager>,
    pub auth_session: Arc<dyn AuthSessionSnapshotPort>,
    pub auth_session_write: Arc<dyn AuthSessionWritePort>,
    pub auth_session_validator: Arc<dyn WebSessionValidator>,
    pub auth_session_invalidation: Arc<dyn AuthSessionInvalidationPort>,
    pub watchlist: Arc<dyn WatchlistReadSnapshotPort>,
    pub watchlist_memberships: Arc<dyn WatchlistMembershipSnapshotPort>,
    pub watchlist_write: Arc<dyn WatchlistWritePort>,
    pub catalog: Arc<dyn MarketDataCatalogReadSnapshotPort>,
    pub provider: Arc<dyn MarketDataProviderReadSnapshotPort>,
    pub plugins: Arc<dyn PluginSnapshotPort>,
    pub plugin_guidance: Arc<dyn PluginUninstallGuidanceSnapshotPort>,
    pub plugin_write: Arc<dyn PluginWritePort>,
    pub broker: Arc<dyn BrokerReadSnapshotPort>,
    pub brokers_write: Arc<dyn BrokersWritePort>,
    pub strategy_definition: Arc<dyn StrategyDefinitionSnapshotPort>,
    pub strategy_definition_write: Arc<dyn StrategyDefinitionWritePort>,
    pub strategy_read: Arc<dyn StrategyReadSnapshotPort>,
    pub strategy_runtime_status: Arc<dyn StrategyRuntimeStatusPort>,
    pub strategy_runtime_write: Arc<dyn StrategyRuntimeWritePort>,
    pub research_preset_read: Arc<dyn ResearchPresetReadSnapshotPort>,
    pub research_preset_write: Arc<dyn ResearchPresetWritePort>,
    pub backtest_read: Arc<dyn BacktestReadSnapshotPort>,
    pub backtest_sync: Arc<dyn BacktestSyncReadSnapshotPort>,
    pub backtests_write: Arc<dyn BacktestsWritePort>,
    pub execution_read: Arc<dyn ExecutionReadSnapshotPort>,
    pub execution_write: Arc<dyn ExecutionWritePort>,
    pub adk_read: Arc<dyn AdkReadSnapshotPort>,
    pub adk_mutation: Arc<dyn AdkMutationPort>,
    pub adk_chat_stream: Arc<dyn AdkChatStreamPort>,
    pub alert_snapshot: Arc<dyn AlertSnapshotPort>,
    pub alert_write: Arc<dyn AlertWritePort>,
    pub system_read: Arc<dyn SystemReadSnapshotPort>,
    pub system_write: Arc<dyn SystemWritePort>,
    pub portfolio: Arc<dyn PortfolioSnapshotPort>,
    pub research_read: Arc<dyn ResearchReadSnapshotPort>,
    pub market_data_derivative: Arc<dyn MarketDataDerivativeReadSnapshotPort>,
    pub market_data_options: Arc<dyn MarketDataOptionsReadSnapshotPort>,
    pub market_data_news_actions: Arc<dyn MarketDataNewsActionsReadSnapshotPort>,
    pub market_data_news_search: Arc<dyn MarketDataNewsSearchReadSnapshotPort>,
    pub market_data_quote: Arc<dyn MarketDataQuoteReadSnapshotPort>,
    pub market_data_prediction: Arc<dyn MarketDataPredictionReadSnapshotPort>,
    pub remote_watchlist: Arc<dyn RemoteWatchlistSnapshotPort>,
    pub remote_watchlist_write: Arc<dyn RemoteWatchlistWritePort>,
    pub market_data_subscription_mutation: Arc<dyn MarketDataSubscriptionMutationPort>,
    pub market_data_provider_actions: Arc<dyn MarketDataProviderActionsPort>,
    pub research_screen_write: Arc<dyn ResearchScreenWritePort>,
    pub strategy_pine_analyze: Arc<dyn StrategyPineAnalyzeSnapshotPort>,
    pub ws_live: Arc<dyn WsLiveSnapshotPort>,
    pub(crate) bound_adapters: BTreeMap<ProductionRouteAdapter, ProductionAdapterBinding>,
    pub(crate) backtest_sync_workers: Arc<BacktestSyncWorkerRegistry>,
    pub(crate) backtest_execution_workers: Arc<BacktestExecutionTaskRegistry>,
    /// Whether the production backtest port has a real execution boundary.
    /// This remains false when no Pine worker was ready at composition time,
    /// which keeps the write route fail-closed with its existing 503 result.
    #[cfg(test)]
    pub(crate) backtest_execution_ready: bool,
    #[allow(dead_code)]
    pub(crate) trade_read_port: Option<Arc<dyn jftrade_integration_futu::TradeReadPort>>,
    #[allow(dead_code)]
    pub(crate) trade_logged_in: Option<bool>,
    #[allow(dead_code)]
    pub(crate) trade_runtime: Option<Arc<SharedTradeReadRuntime>>,
}

impl ProductionPortBundle {
    pub(crate) fn backtest_sync_workers(&self) -> Arc<BacktestSyncWorkerRegistry> {
        Arc::clone(&self.backtest_sync_workers)
    }

    pub(crate) fn backtest_execution_workers(&self) -> Arc<BacktestExecutionTaskRegistry> {
        Arc::clone(&self.backtest_execution_workers)
    }

    #[cfg(test)]
    pub(crate) const fn backtest_execution_ready(&self) -> bool {
        self.backtest_execution_ready
    }
}

use crate::product::ProductError;
use crate::product::product_production_route_registry::ProductionRouteAdapter;

pub(crate) fn production_ports(
    config: &ProductConfig,
    security: &SecuritySettingsService,
) -> Result<ProductionPortBundle, ProductError> {
    if !config.production {
        return Err(ProductError::RouteRegistry(
            "production ports requested for a non-production profile".to_owned(),
        ));
    }

    let descriptors =
        product_data_management::managed_database_runtime_descriptors(config.settings_path());
    let get_path = |key: &str| -> Result<PathBuf, ProductError> {
        descriptors
            .iter()
            .find(|d| d.id == key)
            .map(|d| PathBuf::from(&d.path))
            .ok_or_else(|| {
                ProductError::Storage(format!("missing managed database descriptor for {key}"))
            })
    };

    let mut acquired_databases = Vec::with_capacity(PRODUCTION_DATABASE_IDS.len());

    let watchlist_path = get_path(DATABASE_WATCHLIST)?;
    let watchlist_store = Arc::new(
        WatchlistStore::open_existing(&watchlist_path, WATCHLIST_PRODUCTION_PROFILE).map_err(
            |e| ProductError::Storage(format!("failed to open watchlist production store: {e}")),
        )?,
    );
    acquired_databases.push(DATABASE_WATCHLIST.to_owned());

    let strategy_path = get_path(DATABASE_STRATEGY)?;
    let strategy_def_store = Arc::new(
        StrategyDefinitionStore::open_existing(
            &strategy_path,
            STRATEGY_DEFINITION_PRODUCTION_PROFILE,
        )
        .map_err(|e| {
            ProductError::Storage(format!(
                "failed to open strategy definitions production store: {e}"
            ))
        })?,
    );
    let strategy_runtime_store = Arc::new(StrategyRuntimeStore::from_definition_store(
        &strategy_def_store,
    ));
    acquired_databases.push(DATABASE_STRATEGY.to_owned());

    let research_path = get_path(DATABASE_RESEARCH)?;
    let research_store = Arc::new(
        ResearchPresetStore::open_existing(&research_path, RESEARCH_PRESET_PRODUCTION_PROFILE)
            .map_err(|e| {
                ProductError::Storage(format!(
                    "failed to open research preset production store: {e}"
                ))
            })?,
    );
    acquired_databases.push(DATABASE_RESEARCH.to_owned());

    let backtest_path = get_path(DATABASE_BACKTEST_RUNS)?;
    let backtest_store = Arc::new(
        BacktestRunStore::open_existing(&backtest_path, BACKTEST_RUNS_PRODUCTION_PROFILE)
            .map_err(|e| {
                ProductError::Storage(format!("failed to open backtest runs production store: {e}"))
            })?,
    );
    let backtest_sync_tasks = Arc::new(BacktestSyncTaskStore::new(Arc::clone(&backtest_store)));
    acquired_databases.push(DATABASE_BACKTEST_RUNS.to_owned());

    let backtest_market_data_path = get_path(DATABASE_BACKTEST)?;
    let backtest_market_data_store = Arc::new(
        BacktestMarketDataStore::open_existing(
            &backtest_market_data_path,
            BACKTEST_MARKET_DATA_PRODUCTION_PROFILE,
        )
        .map_err(|e| {
            ProductError::Storage(format!(
                "failed to open backtest market-data production store: {e}"
            ))
        })?,
    );
    acquired_databases.push(DATABASE_BACKTEST.to_owned());

    let execution_path = get_path(DATABASE_EXECUTION)?;
    let execution_store = Arc::new(
        ExecutionOrderStore::open_existing(&execution_path, EXECUTION_ORDERS_PRODUCTION_PROFILE)
            .map_err(|e| {
                ProductError::Storage(format!(
                    "failed to open execution orders production store: {e}"
                ))
            })?,
    );
    acquired_databases.push(DATABASE_EXECUTION.to_owned());

    let adk_path = get_path(DATABASE_ADK)?;
    let adk_store = Arc::new(
        AdkStore::open_existing(&adk_path, ADK_PRODUCTION_PROFILE).map_err(|e| {
            ProductError::Storage(format!("failed to open ADK production store: {e}"))
        })?,
    );
    acquired_databases.push(DATABASE_ADK.to_owned());

    let adk_session_path = get_path(DATABASE_ADK_SESSION)?;
    let adk_session_store = Arc::new(
        AdkSessionStore::open_existing(&adk_session_path, ADK_SESSION_PRODUCTION_PROFILE).map_err(
            |e| ProductError::Storage(format!("failed to open ADK session production store: {e}")),
        )?,
    );
    acquired_databases.push(DATABASE_ADK_SESSION.to_owned());

    let adk_artifact_path = get_path(DATABASE_ADK_ARTIFACT)?;
    let adk_artifact_store = Arc::new(
        AdkArtifactStore::open_existing(&adk_artifact_path, ADK_ARTIFACT_PRODUCTION_PROFILE)
            .map_err(|e| {
                ProductError::Storage(format!("failed to open ADK artifact production store: {e}"))
            })?,
    );
    acquired_databases.push(DATABASE_ADK_ARTIFACT.to_owned());

    let database_leases = ProductionDatabaseLeaseSnapshot::new(acquired_databases);
    let database_lease_status = database_leases.status;

    let auth_session_mgr = Arc::new(
        ProductionAuthSessionManager::open(security.clone(), config.settings_path()).map_err(
            |error| ProductError::Storage(format!("failed to open Web session store: {error}")),
        )?,
    );
    let watchlist_port = Arc::new(ProductionWatchlistPort {
        store: watchlist_store.clone(),
    });
    let strategy_def_port = Arc::new(ProductionStrategyDefinitionPort {
        store: strategy_def_store.clone(),
    });
    let strategy_runtime_port = Arc::new(ProductionStrategyRuntimePort {
        store: strategy_runtime_store,
        definitions: strategy_def_store.clone(),
    });
    let research_preset_port = Arc::new(ProductionResearchPresetPort {
        store: research_store,
    });
    let execution_port = Arc::new(ProductionExecutionPort {
        store: execution_store.clone(),
    });
    let plugin_port = Arc::new(
        ProductionPluginPort::open(config.settings_path()).map_err(ProductError::Storage)?,
    );
    let market_data_settings = Arc::new(
        SettingsFileStore::open_read_only(config.settings_path()).map_err(|error| {
            ProductError::Storage(format!(
                "failed to open market-data provider settings: {error}"
            ))
        })?,
    );
    // Broker runtime projections must be sourced from the same persisted
    // settings document used by the rest of the production composition.  A
    // malformed settings file is a startup error; never manufacture a
    // connection or websocket status from defaults in a production route.
    let broker_settings = market_data_settings
        .load_broker_settings_inputs()
        .map_err(|error| {
            ProductError::Storage(format!(
                "failed to load broker runtime settings: {error}"
            ))
        })?;
    let interface_settings = market_data_settings
        .load_interface_settings()
        .map_err(|error| {
            ProductError::Storage(format!(
                "failed to load interface runtime settings: {error}"
            ))
        })?;
    if let Some(trade_runtime) = config.trade_runtime.as_ref() {
        trade_runtime.set_runtime_projection(
            &broker_settings.effective_config,
            config.live_hub.clone(),
            normalize_live_websocket_connection_limit(interface_settings.as_ref()),
        );
    }
    let active_provider_state = if let Some(state) = config.active_provider_state.as_ref() {
        Arc::clone(state)
    } else {
        let initial_provider = market_data_settings
            .load_active_market_data_provider()
            .map_err(|error| {
                ProductError::Storage(format!(
                    "failed to load active market-data provider settings: {error}"
                ))
            })?
            .as_deref()
            .map(normalize_market_data_provider);
        Arc::new(ActiveProviderState::new(initial_provider))
    };
    active_provider_state.set_readiness(
        config.market_data_helper.is_some(),
        config.market_data_runtime_status_port.is_some(),
        config.market_data_router.is_some(),
    );
    if let Some(trade_runtime) = config.trade_runtime.as_ref() {
        trade_runtime.set_market_data_router(config.market_data_router.clone());
    }
    let provider_snapshot = active_provider_state.snapshot();
    let active_provider = provider_snapshot.provider;
    let has_helper = provider_snapshot.helper_ready;
    let has_router = provider_snapshot.router_ready;
    let backtest_sync_workers = Arc::new(BacktestSyncWorkerRegistry::default());
    let backtest_execution_workers = Arc::new(BacktestExecutionTaskRegistry::default());
    let backtest_port = Arc::new(ProductionBacktestPort {
        store: backtest_store,
        sync_tasks: backtest_sync_tasks,
        _market_data_store: backtest_market_data_store,
        helper: config.market_data_helper.clone(),
        active_provider_state: Arc::clone(&active_provider_state),
        sync_workers: Arc::clone(&backtest_sync_workers),
        execution: config.backtest_execution_port.clone(),
        execution_workers: Arc::clone(&backtest_execution_workers),
        strategy_definitions: Arc::clone(&strategy_def_store),
    });
    let active_provider_str = match active_provider {
        Some(jftrade_settings::MarketDataProvider::Futu) => Some("futu"),
        Some(jftrade_settings::MarketDataProvider::Yfinance) => Some("yfinance"),
        Some(jftrade_settings::MarketDataProvider::Akshare) => Some("akshare"),
        None => None,
    };
    let capability_matrix = MarketDataCapabilityMatrix::new(
        active_provider_str,
        has_helper,
        has_router,
    );
    let bound_adapters = production_adapter_bindings(&capability_matrix);
    let tool_catalog = Arc::new(
        ProductionToolCatalog::from_bindings(&bound_adapters)
            .map_err(|adapter| ProductError::MissingProductionAdapter {
                method: "GET".to_owned(),
                path: "/api/v1/adk/tools".to_owned(),
                adapter,
            })?,
    );
    let adk_port = Arc::new(ProductionAdkPort {
        store: adk_store,
        session_store: adk_session_store,
        artifact_store: adk_artifact_store,
        tool_catalog,
        settings_path: config.settings_path().to_owned(),
    });
    let alert_port = Arc::new(ProductionAlertPort {
        active_provider_state: Arc::clone(&active_provider_state),
    });
    let broker_port = Arc::new(ProductionBrokerPort {
        active_provider_state: Arc::clone(&active_provider_state),
        trade_read_port: config.trade_read_port.clone(),
        trade_logged_in: config.trade_logged_in,
        trade_runtime: config.trade_runtime.clone(),
    });
    let portfolio_port = Arc::new(ProductionPortfolioPort {
        active_provider_state: Arc::clone(&active_provider_state),
        _execution_store: Arc::clone(&execution_store),
        trade_read_port: config.trade_read_port.clone(),
        trade_logged_in: config.trade_logged_in,
        trade_runtime: config.trade_runtime.clone(),
    });
    let research_port = Arc::new(ProductionResearchPort {
        active_provider_state: Arc::clone(&active_provider_state),
        helper: config.market_data_helper.clone(),
    });
    let research_screen_port = Arc::new(ProductionResearchScreenPort {
        active_provider_state: Arc::clone(&active_provider_state),
    });
    let strategy_pine_port = Arc::new(ProductionStrategyPinePort {
        worker_status: config.worker_runtime_status.as_str(),
    });
    let remote_watchlist_port = Arc::new(ProductionRemoteWatchlistPort {
        _store: watchlist_store.clone(),
        active_provider_state: Arc::clone(&active_provider_state),
    });
    let market_data_derivative_port = Arc::new(ProductionMarketDataDerivativePort {
        active_provider_state: Arc::clone(&active_provider_state),
    });
    let market_data_options_port = Arc::new(ProductionMarketDataOptionsPort {
        active_provider_state: Arc::clone(&active_provider_state),
    });
    let market_data_news_port = Arc::new(ProductionMarketDataNewsPort {
        active_provider_state: Arc::clone(&active_provider_state),
    });
    let market_data_prediction_port = Arc::new(ProductionMarketDataPredictionPort {
        active_provider_state: Arc::clone(&active_provider_state),
    });
    let system_write_port = Arc::new(
        ProductionSystemWritePort::open(config.real_trade_control_path()).map_err(|error| {
            ProductError::Storage(format!(
                "failed to open real-trade production control plane: {error}"
            ))
        })?,
    );
    let market_data_catalog_port = Arc::new(ProductionMarketDataCatalogPort::new(
        Arc::clone(&active_provider_state),
        config.market_data_helper.clone(),
    ));
    let calendar_manager = Arc::new(
        CalendarManager::new(
            CalendarSourceRegistry::default(),
            None,
            CalendarManagerSettings::default(),
        )
        .map_err(ProductError::Calendar)?,
    );
    let market_data_quote_port = Arc::new(ProductionMarketDataQuotePort::new(
        Arc::clone(&active_provider_state),
        config.market_data_router.clone(),
        config.market_data_helper.clone(),
        config.physical_subscription_port.clone(),
    ).with_calendar(Arc::clone(&calendar_manager)));
    let market_data_sub_port = Arc::new(ProductionMarketDataSubscriptionMutationPort::new(
        Arc::clone(&active_provider_state),
        config.market_data_router.clone(),
        config.physical_subscription_port.clone(),
    ));
    let market_data_actions_port = Arc::new(ProductionMarketDataProviderActionsPort::new(Some(
        market_data_quote_port.clone(),
    )));

    Ok(ProductionPortBundle {
        active_provider_state: Arc::clone(&active_provider_state),
        database_leases: database_leases.clone(),
        database_lease_status,
        provider_status: config.provider_runtime_status.as_str(),
        opend_status: config.opend_runtime_status.as_str(),
        worker_status: config.worker_runtime_status.as_str(),
        calendar_manager,
        auth_session: auth_session_mgr.clone(),
        auth_session_write: auth_session_mgr.clone(),
        auth_session_validator: auth_session_mgr.clone(),
        auth_session_invalidation: auth_session_mgr,
        watchlist: watchlist_port.clone(),
        watchlist_memberships: watchlist_port.clone(),
        watchlist_write: watchlist_port,
        catalog: market_data_catalog_port,
        provider: Arc::new(ProductionMarketDataProviderPort { active_provider_state, runtime_status: config.market_data_runtime_status_port.clone() }),
        plugins: plugin_port.clone(),
        plugin_guidance: plugin_port.clone(),
        plugin_write: plugin_port,
        broker: broker_port,
        brokers_write: execution_port.clone(),
        strategy_definition: strategy_def_port.clone(),
        strategy_definition_write: strategy_def_port,
        strategy_read: strategy_runtime_port.clone(),
        strategy_runtime_status: strategy_runtime_port.clone(),
        strategy_runtime_write: strategy_runtime_port,
        research_preset_read: research_preset_port.clone(),
        research_preset_write: research_preset_port,
        backtest_read: backtest_port.clone(),
        backtest_sync: backtest_port.clone(),
        backtests_write: backtest_port,
        execution_read: execution_port.clone(),
        execution_write: execution_port,
        adk_read: adk_port.clone(),
        adk_mutation: adk_port.clone(),
        adk_chat_stream: adk_port,
        alert_snapshot: alert_port.clone(),
        alert_write: alert_port,
        system_read: Arc::new(ProductionSystemPort {
            runtime_status: config.market_data_runtime_status_port.clone(),
            settings: market_data_settings.clone(),
            opend_status: config.opend_runtime_status,
            worker_status: config.worker_runtime_status,
            database_leases: database_leases.clone(),
        }),
        system_write: system_write_port,
        portfolio: portfolio_port,
        research_read: research_port,
        market_data_derivative: market_data_derivative_port,
        market_data_options: market_data_options_port,
        market_data_news_actions: market_data_news_port.clone(),
        market_data_news_search: market_data_news_port,
        market_data_quote: market_data_quote_port,
        market_data_prediction: market_data_prediction_port,
        remote_watchlist: remote_watchlist_port.clone(),
        remote_watchlist_write: remote_watchlist_port,
        market_data_subscription_mutation: market_data_sub_port,
        market_data_provider_actions: market_data_actions_port,
        research_screen_write: research_screen_port,
        strategy_pine_analyze: strategy_pine_port,
        ws_live: Arc::new(ProductionWsLivePort),
        bound_adapters,
        backtest_sync_workers,
        backtest_execution_workers,
        #[cfg(test)]
        backtest_execution_ready: config.backtest_execution_port.is_some(),
        trade_read_port: config.trade_read_port.clone(),
        trade_logged_in: config.trade_logged_in,
        trade_runtime: config.trade_runtime.clone(),
    })
}
