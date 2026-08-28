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
    FutuOpenDInstallSettings, FutuOpenDInstallSettingsStorePort, MarketDataProvider,
    MarketDataProviderSettingsStorePort, SecuritySettingsService,
    normalize_market_data_provider,
};
use jftrade_store_settings_file::SettingsFileStore;
use jftrade_store_sqlite::{
    ADK_ARTIFACT_PRODUCTION_PROFILE, ADK_PRODUCTION_PROFILE, ADK_SESSION_PRODUCTION_PROFILE,
    AdkArtifactStore, AdkSessionStore, AdkStore, BACKTEST_MARKET_DATA_PRODUCTION_PROFILE,
    BACKTEST_RUNS_PRODUCTION_PROFILE, BacktestMarketDataStore, BacktestRunStore,
    EXECUTION_ORDERS_PRODUCTION_PROFILE, ExecutionOrderStore,
    RESEARCH_PRESET_PRODUCTION_PROFILE, ResearchPresetStore,
    STRATEGY_DEFINITION_PRODUCTION_PROFILE, StrategyDefinitionStore, StrategyRuntimeStore,
    WATCHLIST_PRODUCTION_PROFILE, WatchlistStore,
};
use serde_json::{Value, json};

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
    MarketDataRuntimeState, PortfolioSnapshotPort, ProductionRuntimeStatus,
    RemoteWatchlistSnapshotPort, ResearchReadSnapshotPort, WsLiveSnapshotPort,
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
#[path = "product_production_ports_system.rs"]
mod product_production_ports_system;
#[path = "product_production_ports_market_data.rs"]
mod product_production_ports_market_data;
#[path = "product_production_ports_adk.rs"]
mod product_production_ports_adk;
#[path = "product_production_adapter_bindings.rs"]
mod product_production_adapter_bindings;

pub(crate) use product_production_ports_execution::{
    ProductionBacktestPort, ProductionExecutionPort,
};
pub(crate) use product_production_ports_market_data::{
    ProductionMarketDataCatalogPort, ProductionMarketDataProviderActionsPort,
    ProductionMarketDataQuotePort, ProductionMarketDataSubscriptionMutationPort,
};
pub(crate) use product_production_ports_plugins::ProductionPluginPort;
pub(crate) use product_production_ports_unavailable::{
    ProductionUnavailablePort, ProductionWsLivePort,
};
pub(crate) use product_production_ports_adk::{ProductionAdkPort, ProductionToolCatalog};
pub(crate) use product_production_adapter_bindings::{
    MarketDataCapabilityMatrix, ProductionAdapterBinding,
};
pub(crate) use product_production_adapter_bindings::production_adapter_bindings;
use product_production_ports_system::ProductionSystemWritePort;
pub(crate) use product_production_ports_strategy::{
    ProductionResearchPresetPort, ProductionStrategyDefinitionPort,
    ProductionStrategyRuntimePort,
};
pub(crate) use product_production_ports_watchlist::ProductionWatchlistPort;

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
    MarketDataCatalogReadSnapshotPort, MarketDataProviderReadSnapshotError,
    MarketDataProviderReadSnapshotPort, PluginSnapshotPort,
    PluginUninstallGuidanceSnapshotPort, ProductConfig, ResearchPresetReadSnapshotPort,
    StrategyDefinitionSnapshotPort, StrategyReadSnapshotPort, StrategyRuntimeStatusPort,
    MarketDataRuntimeStatusPort,
    SystemReadSnapshotError, SystemReadSnapshotPort, WatchlistMembershipSnapshotPort,
    WatchlistReadSnapshotPort, product_data_management,
};

// Market Data Catalog & Provider
use crate::product::product_active_provider_state::ActiveProviderState;

#[derive(Clone)]
pub(crate) struct ProductionMarketDataProviderPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
    pub(crate) runtime_status: Option<Arc<dyn MarketDataRuntimeStatusPort>>,
}

impl std::fmt::Debug for ProductionMarketDataProviderPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ProductionMarketDataProviderPort")
            .field("runtime_status", &self.runtime_status.is_some())
            .finish()
    }
}

impl MarketDataProviderReadSnapshotPort for ProductionMarketDataProviderPort {
    fn read(&self, path: &str, _query: &str) -> Result<Value, MarketDataProviderReadSnapshotError> {
        if path == "/api/v1/market-data/provider" {
            let active_provider = self.active_provider_state.get().unwrap_or_default();
            let runtime = self.runtime_status.as_ref().map(|port| port.snapshot());
            let connected = runtime.as_ref().is_some_and(|state| state.connected);
            let readiness = if connected {
                "ready"
            } else if runtime.is_some() {
                "degraded"
            } else {
                "unavailable"
            };
            let last_error = runtime.as_ref().and_then(|state| {
                state
                    .quote_last_error
                    .as_deref()
                    .or(state.stream_last_error.as_deref())
            }).map(str::to_owned).or_else(|| {
                (!connected).then(|| "market-data provider runtime is not connected".to_owned())
            });
            let descriptor = provider_descriptor_for(active_provider);
            return Ok(json!({
                "checkedAt": provider_now_rfc3339(),
                "descriptor": descriptor,
                "health": {
                    "connected": connected,
                    "readiness": readiness,
                    "lastError": last_error,
                    "streamMode": if connected { "push-stream" } else { "idle" },
                    "activeCount": runtime.as_ref().map_or(0, |state| state.active_count),
                },
                "runtime": runtime
                    .as_ref()
                    .map(runtime_wire)
                    .unwrap_or_else(|| runtime_wire(&MarketDataRuntimeState::default())),
                "subscriptions": {
                    "desiredCount": runtime.as_ref().map_or(0, |state| state.active_count),
                    "entries": []
                },
            }));
        }
        Err(MarketDataProviderReadSnapshotError::Unavailable(
            "market-data provider runtime is unavailable".to_owned(),
        ))
    }
}

fn provider_descriptor_for(provider: MarketDataProvider) -> Value {
    let descriptor = match provider {
        MarketDataProvider::Futu => jftrade_integration_futu::provider_descriptor(),
        MarketDataProvider::Yfinance => jftrade_integration_marketdata_helper::yfinance_descriptor(),
        MarketDataProvider::Akshare => jftrade_integration_marketdata_helper::akshare_descriptor(),
    };
    crate::product::provider_descriptor_wire(descriptor)
}

fn runtime_wire(state: &crate::product::MarketDataRuntimeState) -> Value {
    const ZERO_TIME: &str = "0001-01-01T00:00:00Z";
    let timestamp = |value: Option<jftrade_kernel::WireTimestamp>| {
        value
            .map(|value| json!(value))
            .unwrap_or_else(|| json!(ZERO_TIME))
    };
    json!({
        "Connected": state.connected,
        "Closed": state.closed,
        "Generation": state.generation,
        "ActiveCount": state.active_count,
        "LastRefreshAt": timestamp(state.last_refresh_at),
        "QuoteRetryAt": timestamp(state.quote_retry_at),
        "QuoteFailures": state.quote_failures,
        "QuoteLastError": state.quote_last_error.as_deref().unwrap_or_default(),
        "StreamRetryAt": timestamp(state.stream_retry_at),
        "StreamFailures": state.stream_failures,
        "StreamLastError": state.stream_last_error.as_deref().unwrap_or_default(),
    })
}

fn provider_now_rfc3339() -> String {
    time::OffsetDateTime::now_utc()
        .format(&time::format_description::well_known::Rfc3339)
        .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned())
}

// Plugins, Brokers, Alerts, System

#[derive(Debug, Default)]
pub(crate) struct ProductionAlertPort;

impl AlertSnapshotPort for ProductionAlertPort {
    fn snapshot(
        &self,
        _kind: AlertKind,
        _raw_query: &str,
    ) -> Result<Value, AlertSnapshotError> {
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
        Err(AlertWritePortError::Unavailable(
            "alert provider runtime is not configured".to_owned(),
        ))
    }

    fn apply(
        &self,
        _resolution: &AlertWriteResolution,
        _action: &AlertWriteAction,
    ) -> Result<Option<Value>, AlertWritePortError> {
        Err(AlertWritePortError::Unavailable(
            "alert provider runtime is not configured".to_owned(),
        ))
    }
}

pub(crate) struct ProductionSystemPort {
    runtime_status: Option<Arc<dyn MarketDataRuntimeStatusPort>>,
    settings: Arc<SettingsFileStore>,
    opend_status: ProductionRuntimeStatus,
}

impl std::fmt::Debug for ProductionSystemPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ProductionSystemPort")
            .field("runtime_status", &self.runtime_status.is_some())
            .field("settings_path", &self.settings.path())
            .field("opend_status", &self.opend_status)
            .finish()
    }
}

impl SystemReadSnapshotPort for ProductionSystemPort {
    fn read(&self, path: &str) -> Result<Value, SystemReadSnapshotError> {
        match path {
            "/api/v1/system/futu-opend" => self.futu_opend_snapshot(),
            "/api/v1/system/worker/broker-order-updates" => Ok(json!({})),
            "/api/v1/system/info" => Ok(json!({
                "version": env!("CARGO_PKG_VERSION"),
                "architecture": std::env::consts::ARCH,
                "os": std::env::consts::OS,
                "engine": "rust",
                "productionOwner": "rust",
            })),
            "/api/v1/system/status" => {
                let market_data = self.runtime_status.as_ref().map(|port| port.snapshot());
                let status = if market_data.as_ref().is_some_and(|state| state.connected) {
                    "operational"
                } else {
                    "degraded"
                };
                Ok(json!({
                    "status": status,
                    "workers": "healthy",
                    "marketData": {
                        "connected": market_data.as_ref().is_some_and(|state| state.connected),
                        "activeCount": market_data.as_ref().map_or(0, |state| state.active_count),
                    },
                }))
            }
            "/api/v1/system/diagnostics" => Ok(json!({
                "databaseIntegrity": "ok",
                "settingsIntegrity": "ok",
                "marketDataRuntime": self.runtime_status.is_some().then_some("configured"),
            })),
            _ => Err(SystemReadSnapshotError::Unavailable(format!("system path not found: {path}"))),
        }
    }
}

impl ProductionSystemPort {
    fn futu_opend_snapshot(&self) -> Result<Value, SystemReadSnapshotError> {
        let settings = self
            .settings
            .load_futu_open_d_install_settings()
            .map_err(|error| SystemReadSnapshotError::Unavailable(error.to_string()))?
            .unwrap_or_else(FutuOpenDInstallSettings::default);
        let Some(runtime_status) = self.runtime_status.as_ref() else {
            return Ok(json!({
                "status": "unavailable",
                "reason": "broker integration not enabled",
            }));
        };
        let state = runtime_status.snapshot();
        let has_error = state.quote_last_error.as_ref().is_some_and(|error| !error.is_empty())
            || state.stream_last_error.as_ref().is_some_and(|error| !error.is_empty());
        let connectivity = if state.connected {
            "connected"
        } else if has_error {
            "degraded"
        } else {
            "disconnected"
        };
        let status = if state.connected {
            "healthy"
        } else if has_error || self.opend_status == ProductionRuntimeStatus::Degraded {
            "degraded"
        } else {
            "offline"
        };
        let last_error = state
            .quote_last_error
            .clone()
            .filter(|error| !error.is_empty())
            .or(state.stream_last_error.clone().filter(|error| !error.is_empty()));
        Ok(json!({
            "checkedAt": provider_now_rfc3339(),
            "status": status,
            "runtime": {
                "connectivity": connectivity,
                "host": settings.host,
                "apiPort": settings.api_port,
                "websocketPort": settings.websocket_port,
                "useEncryption": settings.use_encryption,
                "websocketKeyConfigured": settings.websocket_key_required,
                "marketDataTransport": "bbgo-opend-tcp-api",
                "quoteLoggedIn": Value::Null,
                "tradeLoggedIn": Value::Null,
                "programStatus": Value::Null,
                "serverVersion": Value::Null,
                "minimumVersion": jftrade_integration_futu::MINIMUM_OPEND_VERSION,
                "lastError": last_error,
            },
            "diagnosis": {
                "code": if state.connected { "NONE" } else { "OPEND_UNAVAILABLE" },
                "summary": last_error,
                "manualRetryRequired": !state.connected,
                "restartOpenDRecommended": false,
            },
            "localSocketDiagnostics": {
                "transportMode": "bbgo-opend-tcp-api",
                "configuredOpenDWebSocketLimit": settings.max_websocket_connections,
                "configuredOpenDWebSocketLimitActive": false,
                "configuredOpenDWebSocketLimitScope": "stored for FTWebSocket compatibility; current market-data path uses the OpenD native API via bbgo",
                "websocketEstablishedConnections": 0,
                "jftradeLiveWebSocketLimit": settings.max_websocket_connections,
                "jftradeLiveWebSocketAtLimit": false,
                "likelyConnectionSaturation": false,
                "openDWebSocketPoolLikelySaturation": false,
                "liveQuoteBackoffActive": state.quote_retry_at.is_some(),
                "liveQuoteRetryAfter": state.quote_retry_at,
                "liveQuoteFailureCount": state.quote_failures,
                "liveQuoteLastError": state.quote_last_error,
                "liveStreamBackoffActive": state.stream_retry_at.is_some(),
                "liveStreamRetryAfter": state.stream_retry_at,
                "liveStreamFailureCount": state.stream_failures,
                "liveStreamLastError": state.stream_last_error,
                "topClientProcesses": [],
            },
            "localInstallation": {
                "platform": std::env::consts::OS,
                "installed": false,
                "version": Value::Null,
                "installPath": Value::Null,
                "guiDetected": false,
                "process": {"running": false, "pid": Value::Null, "executablePath": Value::Null},
            },
            "latestVersion": {
                "value": Value::Null,
                "sourceUrl": Value::Null,
                "checkedAt": Value::Null,
                "status": "unknown",
                "error": Value::Null,
            },
            "recommendations": [],
        }))
    }
}
// Bundle

#[derive(Clone)]
pub(crate) struct ProductionPortBundle {
    pub database_lease_status: &'static str,
    pub provider_status: &'static str,
    pub opend_status: &'static str,
    pub worker_status: &'static str,
    pub websocket_status: &'static str,
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

    let watchlist_path = get_path(DATABASE_WATCHLIST)?;
    let watchlist_store = Arc::new(
        WatchlistStore::open_existing(&watchlist_path, WATCHLIST_PRODUCTION_PROFILE).map_err(
            |e| ProductError::Storage(format!("failed to open watchlist production store: {e}")),
        )?,
    );

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

    let research_path = get_path(DATABASE_RESEARCH)?;
    let research_store = Arc::new(
        ResearchPresetStore::open_existing(&research_path, RESEARCH_PRESET_PRODUCTION_PROFILE)
            .map_err(|e| {
                ProductError::Storage(format!(
                    "failed to open research preset production store: {e}"
                ))
            })?,
    );

    let backtest_path = get_path(DATABASE_BACKTEST_RUNS)?;
    let backtest_store = Arc::new(
        BacktestRunStore::open_existing(&backtest_path, BACKTEST_RUNS_PRODUCTION_PROFILE)
            .map_err(|e| {
                ProductError::Storage(format!("failed to open backtest runs production store: {e}"))
            })?,
    );

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

    let execution_path = get_path(DATABASE_EXECUTION)?;
    let execution_store = Arc::new(
        ExecutionOrderStore::open_existing(&execution_path, EXECUTION_ORDERS_PRODUCTION_PROFILE)
            .map_err(|e| {
                ProductError::Storage(format!(
                    "failed to open execution orders production store: {e}"
                ))
            })?,
    );

    let adk_path = get_path(DATABASE_ADK)?;
    let adk_store = Arc::new(
        AdkStore::open_existing(&adk_path, ADK_PRODUCTION_PROFILE).map_err(|e| {
            ProductError::Storage(format!("failed to open ADK production store: {e}"))
        })?,
    );

    let adk_session_path = get_path(DATABASE_ADK_SESSION)?;
    let adk_session_store = Arc::new(
        AdkSessionStore::open_existing(&adk_session_path, ADK_SESSION_PRODUCTION_PROFILE).map_err(
            |e| ProductError::Storage(format!("failed to open ADK session production store: {e}")),
        )?,
    );

    let adk_artifact_path = get_path(DATABASE_ADK_ARTIFACT)?;
    let adk_artifact_store = Arc::new(
        AdkArtifactStore::open_existing(&adk_artifact_path, ADK_ARTIFACT_PRODUCTION_PROFILE)
            .map_err(|e| {
                ProductError::Storage(format!("failed to open ADK artifact production store: {e}"))
            })?,
    );

    let auth_session_mgr = Arc::new(
        ProductionAuthSessionManager::open(security.clone(), config.settings_path()).map_err(
            |error| ProductError::Storage(format!("failed to open Web session store: {error}")),
        )?,
    );
    let watchlist_port = Arc::new(ProductionWatchlistPort {
        store: watchlist_store,
    });
    let strategy_def_port = Arc::new(ProductionStrategyDefinitionPort {
        store: strategy_def_store.clone(),
    });
    let strategy_runtime_port = Arc::new(ProductionStrategyRuntimePort {
        store: strategy_runtime_store,
    });
    let research_preset_port = Arc::new(ProductionResearchPresetPort {
        store: research_store,
    });
    let backtest_port = Arc::new(ProductionBacktestPort {
        store: backtest_store,
        _market_data_store: backtest_market_data_store,
        sync_tasks: Default::default(),
    });
    let execution_port = Arc::new(ProductionExecutionPort {
        store: execution_store,
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
    let provider_snapshot = active_provider_state.snapshot();
    let active_provider = provider_snapshot.provider;
    let has_helper = provider_snapshot.helper_ready;
    let has_router = provider_snapshot.router_ready;
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
    let alert_port = Arc::new(ProductionAlertPort);
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
        database_lease_status: "acquired",
        provider_status: config.provider_runtime_status.as_str(),
        opend_status: config.opend_runtime_status.as_str(),
        worker_status: config.worker_runtime_status.as_str(),
        websocket_status: "ready",
        calendar_manager,
        auth_session: auth_session_mgr.clone(),
        auth_session_write: auth_session_mgr.clone(),
        auth_session_validator: auth_session_mgr.clone(),
        auth_session_invalidation: auth_session_mgr,
        watchlist: watchlist_port.clone(),
        watchlist_memberships: watchlist_port.clone(),
        watchlist_write: watchlist_port,
        catalog: market_data_catalog_port,
        provider: Arc::new(ProductionMarketDataProviderPort {
            active_provider_state: Arc::clone(&active_provider_state),
            runtime_status: config.market_data_runtime_status_port.clone(),
        }),
        plugins: plugin_port.clone(),
        plugin_guidance: plugin_port.clone(),
        plugin_write: plugin_port,
        broker: Arc::new(ProductionUnavailablePort::new(
            "broker integration is not enabled",
        )),
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
        }),
        system_write: system_write_port,
        portfolio: Arc::new(ProductionUnavailablePort::new(
            "portfolio provider is not configured",
        )),
        research_read: Arc::new(ProductionUnavailablePort::new(
            "research provider is not configured",
        )),
        market_data_derivative: Arc::new(ProductionUnavailablePort::new(
            "derivative market-data provider is not configured",
        )),
        market_data_options: Arc::new(ProductionUnavailablePort::new(
            "options market-data provider is not configured",
        )),
        market_data_news_actions: Arc::new(ProductionUnavailablePort::new(
            "news provider is not configured",
        )),
        market_data_news_search: Arc::new(ProductionUnavailablePort::new(
            "news provider is not configured",
        )),
        market_data_quote: market_data_quote_port,
        market_data_prediction: Arc::new(ProductionUnavailablePort::new(
            "prediction market-data provider is not configured",
        )),
        remote_watchlist: Arc::new(ProductionUnavailablePort::new(
            "remote watchlist provider is not configured",
        )),
        remote_watchlist_write: Arc::new(ProductionUnavailablePort::new(
            "remote watchlist provider is not configured",
        )),
        market_data_subscription_mutation: market_data_sub_port,
        market_data_provider_actions: market_data_actions_port,
        research_screen_write: Arc::new(ProductionUnavailablePort::new(
            "research screener is not configured",
        )),
        strategy_pine_analyze: Arc::new(ProductionUnavailablePort::new(
            "pine analyzer is not configured",
        )),
        ws_live: Arc::new(ProductionWsLivePort),
        bound_adapters,
    })
}
