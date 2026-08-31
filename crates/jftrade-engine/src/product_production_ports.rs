//! Production projections and adapter wiring for the Rust composition root.
//!
//! When `config.production` is true, all domain ports connect to the authoritative
//! SQLite databases (under `production.v1` lease profile) and production services
//! without falling back to test cutover or dummy fixtures.

use std::collections::BTreeMap;
use std::path::PathBuf;
use std::sync::Arc;

use crate::product::product_active_provider_state::ActiveProviderState;
use crate::product::product_adk_model_runtime::{
    ProductionAdkChatRuntime, RunCancellationRegistry,
};
use crate::product::{ProductConfig, product_data_management};
use jftrade_calendar::{
    CalendarManager, CalendarManagerSettings, CalendarManualOverride, CalendarSessionOverride,
    CalendarSnapshotStore, CalendarSourcePolicy, CalendarSourceRegistry,
};
use jftrade_datamanagement::{
    DATABASE_ADK, DATABASE_ADK_ARTIFACT, DATABASE_ADK_SESSION, DATABASE_BACKTEST,
    DATABASE_BACKTEST_RUNS, DATABASE_EXECUTION, DATABASE_RESEARCH, DATABASE_STRATEGY,
    DATABASE_WATCHLIST,
};
use jftrade_settings::{
    BacktestMarketDataProviderSettingsStorePort, BrokerSettingsStorePort, ExchangeCalendarSettings,
    ExchangeCalendarSettingsStorePort,
    InterfaceSettingsStorePort, MarketDataProviderSettingsStorePort, SecuritySettingsService,
    normalize_live_websocket_connection_limit, parse_market_data_provider,
};
use jftrade_store_settings_file::SettingsFileStore;
use jftrade_store_sqlite::{
    ADK_ARTIFACT_PRODUCTION_PROFILE, ADK_PRODUCTION_PROFILE, ADK_SESSION_PRODUCTION_PROFILE,
    AdkArtifactStore, AdkSessionStore, AdkStore, BACKTEST_MARKET_DATA_PRODUCTION_PROFILE,
    BACKTEST_RUNS_PRODUCTION_PROFILE, BacktestMarketDataStore, BacktestRunStore,
    BacktestSyncTaskStore, EXECUTION_ORDERS_PRODUCTION_PROFILE, ExecutionOrderStore,
    RESEARCH_PRESET_PRODUCTION_PROFILE, ResearchPresetStore,
    STRATEGY_DEFINITION_PRODUCTION_PROFILE, StrategyDefinitionStore, StrategyRuntimeStore,
    WATCHLIST_PRODUCTION_PROFILE, WatchlistStore,
};

#[path = "product_backtest_sync_registry.rs"]
mod product_backtest_sync_registry;
#[path = "product_production_adapter_bindings.rs"]
mod product_production_adapter_bindings;
#[path = "product_production_database_leases.rs"]
mod product_production_database_leases;
#[path = "product_production_ports_adk.rs"]
mod product_production_ports_adk;
#[path = "product_production_ports_execution.rs"]
mod product_production_ports_execution;
#[path = "product_production_ports_market_data.rs"]
mod product_production_ports_market_data;
#[path = "product_production_ports_plugins.rs"]
mod product_production_ports_plugins;
#[path = "product_production_ports_provider.rs"]
mod product_production_ports_provider;
#[path = "product_production_ports_strategy.rs"]
mod product_production_ports_strategy;
#[path = "product_production_ports_system.rs"]
mod product_production_ports_system;
#[path = "product_production_ports_storage.rs"]
mod product_production_ports_storage;
#[path = "product_production_ports_trade.rs"]
mod product_production_ports_trade;
#[path = "product_production_ports_types.rs"]
mod product_production_ports_types;
#[path = "product_production_ports_unavailable.rs"]
mod product_production_ports_unavailable;
#[path = "product_production_ports_watchlist.rs"]
mod product_production_ports_watchlist;

pub(crate) use crate::product::product_backtest_execution::BacktestExecutionTaskRegistry;
pub(crate) use product_backtest_sync_registry::BacktestSyncWorkerRegistry;
pub(crate) use product_production_adapter_bindings::production_adapter_bindings;
pub(crate) use product_production_adapter_bindings::{
    runtime_scoped_adapter, MarketDataCapabilityMatrix, OPTION_ANALYSIS_OPERATIONS,
    ProductionAdapterBinding,
};
pub(crate) use product_production_database_leases::{
    PRODUCTION_DATABASE_IDS, ProductionDatabaseLeaseSnapshot,
};
pub(crate) use product_production_ports_adk::{ProductionAdkPort, ProductionToolCatalog};
pub(crate) use product_production_ports_execution::{
    BacktestMarketDataProviderState, ExecutionReconciliationWorker, ProductionBacktestPort,
    ProductionExecutionPort,
};
pub(crate) use product_production_ports_market_data::{
    ProductionMarketDataCatalogPort, ProductionMarketDataDerivativePort,
    ProductionMarketDataNewsPort, ProductionMarketDataOptionsPort,
    ProductionMarketDataPredictionPort, ProductionMarketDataProviderActionsPort,
    ProductionMarketDataQuotePort, ProductionMarketDataSubscriptionMutationPort,
};
pub(crate) use product_production_ports_plugins::ProductionPluginPort;
pub(crate) use product_production_ports_provider::ProductionMarketDataProviderPort;
pub(crate) use product_production_ports_provider::provider_now_rfc3339;
pub(crate) use product_production_ports_strategy::{
    ProductionResearchPort, ProductionResearchPresetPort, ProductionResearchScreenPort,
    ProductionStrategyDefinitionPort, ProductionStrategyPinePort, ProductionStrategyRuntimePort,
    StrategyRuntimeManager,
};
pub(crate) use product_production_ports_system::{ProductionSystemPort, ProductionSystemWritePort};
pub(crate) use product_production_ports_trade::SharedTradeReadRuntime;
pub(crate) use product_production_ports_trade::{ProductionBrokerPort, ProductionPortfolioPort};
pub(crate) use product_production_ports_types::{
    ProductionAlertPort, ProductionPortBundle, provider_request_matches, research_tool_binding,
};
pub(crate) use product_production_ports_unavailable::ProductionWsLivePort;
pub(crate) use product_production_ports_watchlist::{
    ProductionRemoteWatchlistPort, ProductionWatchlistPort,
};

const EXCHANGE_CALENDAR_DIR_ENV: &str = "JFTRADE_EXCHANGE_CALENDAR_DIR";

fn exchange_calendar_snapshot_root(settings_path: &std::path::Path) -> PathBuf {
    std::env::var_os(EXCHANGE_CALENDAR_DIR_ENV)
        .filter(|value| !value.is_empty())
        .map(PathBuf::from)
        .unwrap_or_else(|| {
            settings_path
                .parent()
                .filter(|parent| !parent.as_os_str().is_empty())
                .map_or_else(
                    || PathBuf::from("exchange-calendars"),
                    |parent| parent.join("exchange-calendars"),
                )
        })
}

pub(crate) fn calendar_manager_settings(
    input: ExchangeCalendarSettings,
) -> CalendarManagerSettings {
    CalendarManagerSettings {
        auto_refresh_enabled: input.auto_refresh_enabled,
        error_notifications_enabled: input.error_notifications_enabled,
        refresh_interval_hours: input.refresh_interval_hours,
        warmup_markets: input.warmup_markets,
        source_policies: input
            .source_policies
            .into_iter()
            .map(|policy| CalendarSourcePolicy {
                market: policy.market,
                preferred_source_ids: policy.preferred_source_ids,
                enabled_source_ids: policy.enabled_source_ids,
                fallback_to_builtin: policy.fallback_to_builtin,
                require_official: policy.require_official,
                stale_after_hours: policy.stale_after_hours,
            })
            .collect(),
        manual_overrides: input
            .manual_overrides
            .into_iter()
            .map(|override_| CalendarManualOverride {
                market: override_.market,
                date: override_.date,
                status: override_.status,
                sessions: override_
                    .sessions
                    .into_iter()
                    .map(|session| CalendarSessionOverride {
                        kind: session.kind,
                        start_minute: session.start_minute,
                        end_minute: session.end_minute,
                    })
                    .collect(),
                reason: override_.reason,
                observed: override_.observed,
            })
            .collect(),
    }
}
use crate::product::ProductError;
use crate::product::product_auth_session_manager::ProductionAuthSessionManager;
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
        BacktestRunStore::open_existing(&backtest_path, BACKTEST_RUNS_PRODUCTION_PROFILE).map_err(
            |e| {
                ProductError::Storage(format!(
                    "failed to open backtest runs production store: {e}"
                ))
            },
        )?,
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
    let research_preset_port = Arc::new(ProductionResearchPresetPort {
        store: research_store,
    });
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
            .map(|provider| {
                parse_market_data_provider(provider).map_err(|error| {
                    ProductError::Storage(format!(
                        "invalid active market-data provider settings: {error}"
                    ))
                })
            })
            .transpose()?;
        Arc::new(ActiveProviderState::new(initial_provider))
    };
    let backtest_market_data_provider_state = if let Some(state) =
        config.backtest_market_data_provider_state.as_ref()
    {
        Arc::clone(state)
    } else {
        let initial_provider = market_data_settings
            .load_backtest_market_data_provider()
            .map_err(|error| {
                ProductError::Storage(format!(
                    "failed to load backtest market-data provider settings: {error}"
                ))
            })?
            .as_deref()
            .map(parse_market_data_provider)
            .transpose()
            .map_err(|error| {
                ProductError::Storage(format!(
                    "invalid backtest market-data provider settings: {error}"
                ))
            })?
            .unwrap_or_default();
        Arc::new(BacktestMarketDataProviderState::new(initial_provider))
    };
    let execution_port = Arc::new(ProductionExecutionPort {
        store: execution_store.clone(),
        active_provider_state: Arc::clone(&active_provider_state),
        trade_read_port: config.trade_read_port.clone(),
        trade_write_port: config.trade_write_port.clone(),
        trade_logged_in: config.trade_logged_in,
        trade_runtime: config.trade_runtime.clone(),
        cancel_inflight: Arc::new(std::sync::Mutex::new(std::collections::BTreeSet::new())),
    });
    let execution_reconciliation_worker = config.trade_runtime.as_ref().and_then(|runtime| {
        tokio::runtime::Handle::try_current().ok().map(|_| {
            ExecutionReconciliationWorker::start(
                Arc::clone(&execution_port),
                Some(runtime.reconciliation_wake()),
            )
        })
    });
    let plugin_port = Arc::new(
        ProductionPluginPort::open(config.settings_path()).map_err(ProductError::Storage)?,
    );
    let calendar_settings = market_data_settings
        .load_exchange_calendars()
        .map_err(|error| {
            ProductError::Storage(format!(
                "failed to load exchange calendar settings: {error}"
            ))
        })?
        .map(jftrade_settings::normalize_exchange_calendar_settings)
        .unwrap_or_default();
    // Broker runtime projections must be sourced from the same persisted
    // settings document used by the rest of the production composition.  A
    // malformed settings file is a startup error; never manufacture a
    // connection or websocket status from defaults in a production route.
    let broker_settings = market_data_settings
        .load_broker_settings_inputs()
        .map_err(|error| {
            ProductError::Storage(format!("failed to load broker runtime settings: {error}"))
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
    let backtest_store_for_storage = Arc::clone(&backtest_store);
    let backtest_sync_tasks_for_storage = Arc::clone(&backtest_sync_tasks);
    let backtest_port = Arc::new(ProductionBacktestPort {
        store: backtest_store,
        sync_tasks: backtest_sync_tasks,
        _market_data_store: backtest_market_data_store,
        helper: config.market_data_helper.clone(),
        trade_runtime: config.trade_runtime.clone(),
        backtest_market_data_provider_state: Arc::clone(&backtest_market_data_provider_state),
        sync_workers: Arc::clone(&backtest_sync_workers),
        execution: config.backtest_execution_port.clone(),
        execution_workers: Arc::clone(&backtest_execution_workers),
        strategy_definitions: Arc::clone(&strategy_def_store),
    });
    backtest_port
        .recover_orphaned_runs()
        .map_err(|error| ProductError::Storage(error.to_string()))?;
    backtest_port
        .recover_orphaned_sync_tasks()
        .map_err(ProductError::Storage)?;
    let active_provider_str = match active_provider {
        Some(jftrade_settings::MarketDataProvider::Futu) => Some("futu"),
        Some(jftrade_settings::MarketDataProvider::Yfinance) => Some("yfinance"),
        Some(jftrade_settings::MarketDataProvider::Akshare) => Some("akshare"),
        None => None,
    };
    let capability_matrix =
        MarketDataCapabilityMatrix::new(active_provider_str, has_helper, has_router);
    let mut bound_adapters = production_adapter_bindings(&capability_matrix);
    // Backtest execution is assembled after the PineTS readiness probe.  The
    // capability matrix intentionally has no worker input, so project the
    // concrete startup state here as soon as the verified execution port is
    // present; current_binding() continues to refresh this decision after
    // provider transitions.  BacktestStart reads only the local historical
    // candle store at request time, so helper/OpenD/router health is not a
    // prerequisite for this binding.
    bound_adapters.insert(
        ProductionRouteAdapter::BacktestStart,
        if config.backtest_execution_port.is_some() {
            ProductionAdapterBinding::Ready
        } else {
            ProductionAdapterBinding::ExternalUnavailable
        },
    );
    bound_adapters.insert(
        ProductionRouteAdapter::StrategyPine,
        if config.strategy_pine_worker_port.is_some() {
            ProductionAdapterBinding::Ready
        } else {
            ProductionAdapterBinding::ExternalUnavailable
        },
    );
    // ResearchRead is a shared adapter for helper-backed company research
    // operations.  Reflect helper readiness in the ADK catalog; otherwise all
    // research tools would remain non-callable even when the selected helper
    // is healthy.  Futu keeps the conservative unavailable state because only
    // valuation has an OpenD reader while the remaining operations do not.
    if matches!(
        active_provider,
        Some(jftrade_settings::MarketDataProvider::Yfinance)
            | Some(jftrade_settings::MarketDataProvider::Akshare)
    ) {
        bound_adapters.insert(
            ProductionRouteAdapter::ResearchRead,
            if provider_snapshot.helper_ready {
                ProductionAdapterBinding::Ready
            } else {
                ProductionAdapterBinding::ExternalUnavailable
            },
        );
    }
    // Keep the startup tool catalog aligned with the live transport state.
    // `MarketDataCapabilityMatrix` intentionally models logical router
    // capability for provider transitions, but a Futu snapshot/subscription
    // route is not usable until OpenD has completed its handshake.  Apply the
    // physical readiness gate before exposing the bound-adapter snapshot.
    if active_provider == Some(jftrade_settings::MarketDataProvider::Futu)
        && !provider_snapshot.opend_ready
    {
        for adapter in [
            ProductionRouteAdapter::MarketDataSnapshotsRead,
            ProductionRouteAdapter::MarketDataBatchSnapshotsWrite,
            ProductionRouteAdapter::MarketDataSubscriptionRead,
            ProductionRouteAdapter::MarketDataSubscriptionAcquireWrite,
            ProductionRouteAdapter::MarketDataSubscriptionReleaseWrite,
            ProductionRouteAdapter::MarketDataSubscriptionClearWrite,
            ProductionRouteAdapter::MarketDataSubscriptionHeartbeatWrite,
        ] {
            bound_adapters.insert(adapter, ProductionAdapterBinding::ExternalUnavailable);
        }
    }
    // Future contracts have a dedicated OpenD reader.  Keep the tool catalog
    // in sync with the route registry's operation-level readiness instead of
    // inheriting the generic derivatives (warrants) unavailable binding.
    let futures_ready = active_provider == Some(jftrade_settings::MarketDataProvider::Futu)
        && provider_snapshot.opend_ready
        && config
            .trade_runtime
            .as_ref()
            .is_some_and(|runtime| runtime.future_info_available());
    bound_adapters.insert(
        ProductionRouteAdapter::MarketDataFuturesRead,
        if futures_ready {
            ProductionAdapterBinding::Ready
        } else {
            ProductionAdapterBinding::ExternalUnavailable
        },
    );
    // ADK research tools are more granular than the public HTTP route's
    // compatibility umbrella. Derive readiness from the same operation-level
    // checks used by the route registry so unsupported operations are never
    // advertised as callable.
    let research_tool_bindings = BTreeMap::from([
        (
            "instrument",
            research_tool_binding(&provider_snapshot, config, "instrument"),
        ),
        (
            "financials",
            research_tool_binding(&provider_snapshot, config, "financials"),
        ),
        (
            "valuation",
            research_tool_binding(&provider_snapshot, config, "valuation"),
        ),
        (
            "news",
            bound_adapters
                .get(&ProductionRouteAdapter::MarketDataNewsSearchRead)
                .copied()
                .unwrap_or(ProductionAdapterBinding::ExternalUnavailable),
        ),
    ]);
    let tool_catalog = Arc::new(
        ProductionToolCatalog::from_bindings_with_research(
            &bound_adapters,
            &research_tool_bindings,
        )
        .map_err(|adapter| ProductError::MissingProductionAdapter {
            method: "GET".to_owned(),
            path: "/api/v1/adk/tools".to_owned(),
            adapter,
        })?
        .with_active_provider_state(Arc::clone(&active_provider_state))
        .with_trade_runtime(config.trade_runtime.clone())
        .with_backtest_execution_ready(config.backtest_execution_port.is_some()),
    );
    let cancellation_registry = Arc::new(RunCancellationRegistry::default());
    let adk_chat_runtime = Arc::new(ProductionAdkChatRuntime::new(
        Arc::clone(&adk_store),
        Arc::clone(&adk_session_store),
        config.settings_path(),
        Arc::clone(&cancellation_registry),
        Arc::clone(&tool_catalog),
    ));
    let mcp_catalog = Arc::clone(&tool_catalog);
    let adk_port = Arc::new(ProductionAdkPort {
        store: Arc::clone(&adk_store),
        session_store: adk_session_store,
        artifact_store: adk_artifact_store,
        tool_catalog: Arc::clone(&tool_catalog),
        settings_path: config.settings_path().to_owned(),
        chat_runtime: Some(adk_chat_runtime),
    });
    let alert_port = Arc::new(ProductionAlertPort {
        active_provider_state: Arc::clone(&active_provider_state),
        trade_runtime: config.trade_runtime.clone(),
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
        trade_runtime: config.trade_runtime.clone(),
    });
    let research_screen_port = Arc::new(ProductionResearchScreenPort {
        active_provider_state: Arc::clone(&active_provider_state),
    });
    let strategy_pine_port = Arc::new(ProductionStrategyPinePort {
        worker: config.strategy_pine_worker_port.clone(),
    });
    let remote_watchlist_port = Arc::new(ProductionRemoteWatchlistPort {
        _store: watchlist_store.clone(),
        active_provider_state: Arc::clone(&active_provider_state),
        trade_runtime: config.trade_runtime.clone(),
    });
    let market_data_derivative_port = Arc::new(ProductionMarketDataDerivativePort {
        active_provider_state: Arc::clone(&active_provider_state),
        trade_runtime: config.trade_runtime.clone(),
    });
    let market_data_options_port = Arc::new(ProductionMarketDataOptionsPort {
        active_provider_state: Arc::clone(&active_provider_state),
        trade_runtime: config.trade_runtime.clone(),
    });
    let market_data_news_port = Arc::new(ProductionMarketDataNewsPort {
        active_provider_state: Arc::clone(&active_provider_state),
        helper: config.market_data_helper.clone(),
        trade_runtime: config.trade_runtime.clone(),
    });
    let market_data_prediction_port = Arc::new(ProductionMarketDataPredictionPort {
        active_provider_state: Arc::clone(&active_provider_state),
        trade_runtime: config.trade_runtime.clone(),
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
            Some(Arc::new(CalendarSnapshotStore::new(
                exchange_calendar_snapshot_root(config.settings_path()),
            ))),
            calendar_manager_settings(calendar_settings),
        )
        .map_err(ProductError::Calendar)?,
    );
    let market_data_quote_port = Arc::new(
        ProductionMarketDataQuotePort::new(
            Arc::clone(&active_provider_state),
            config.market_data_router.clone(),
            config.market_data_helper.clone(),
            config.physical_subscription_port.clone(),
        )
        .with_calendar(Arc::clone(&calendar_manager)),
    );
    let strategy_runtime_manager = Arc::new(StrategyRuntimeManager::new(
        config.market_data_router.clone(),
        config.strategy_pine_worker_port.clone(),
        Some(market_data_quote_port.clone()),
        Some(execution_port.clone()),
        Arc::clone(&active_provider_state),
    ));
    let strategy_runtime_store_for_storage = Arc::clone(&strategy_runtime_store);
    let strategy_runtime_port = Arc::new(ProductionStrategyRuntimePort {
        store: strategy_runtime_store,
        definitions: strategy_def_store.clone(),
        manager: Arc::clone(&strategy_runtime_manager),
    });
    strategy_runtime_port
        .restore_running_instances()
        .map_err(|error| {
            ProductError::Storage(format!("recover strategy runtime instances: {error}"))
        })?;
    let market_data_sub_port = Arc::new(ProductionMarketDataSubscriptionMutationPort::new(
        Arc::clone(&active_provider_state),
        config.market_data_router.clone(),
        config.physical_subscription_port.clone(),
    )
    .with_trade_runtime(config.trade_runtime.clone()));
    let market_data_actions_port = Arc::new(
        ProductionMarketDataProviderActionsPort::new(Some(market_data_quote_port.clone()))
            .with_trade_runtime(config.trade_runtime.clone())
            .with_active_provider_state(Some(Arc::clone(&active_provider_state))),
    );

    let mut bundle = ProductionPortBundle {
        active_provider_state: Arc::clone(&active_provider_state),
        settings_store: Arc::clone(&market_data_settings),
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
        provider: Arc::new(ProductionMarketDataProviderPort {
            active_provider_state,
            runtime_status: config.market_data_runtime_status_port.clone(),
            router: config.market_data_router.clone(),
            physical: config.physical_subscription_port.clone(),
        }),
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
        strategy_runtime_manager,
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
            live_hub: config.live_hub.clone(),
            settings: market_data_settings.clone(),
            opend_status: config.opend_runtime_status,
            worker_status: config.worker_runtime_status,
            execution_reconciliation_worker: execution_reconciliation_worker.clone(),
            database_leases: database_leases.clone(),
            backtest_store: backtest_store_for_storage,
            backtest_sync_tasks: backtest_sync_tasks_for_storage,
            execution_store: Arc::clone(&execution_store),
            adk_store: Arc::clone(&adk_store),
            strategy_runtime_store: strategy_runtime_store_for_storage,
            real_trade_control: crate::real_trade_control::RealTradeControlReader::new(
                config.real_trade_control_path(),
            ),
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
        ws_live: Arc::new(ProductionWsLivePort::new(config.live_hub.clone())),
        installed_adapters: Default::default(),
        bound_adapters,
        backtest_sync_workers,
        backtest_execution_workers,
        execution_reconciliation_worker,
        backtest_execution_ready: config.backtest_execution_port.is_some(),
        trade_read_port: config.trade_read_port.clone(),
        trade_write_port: config.trade_write_port.clone(),
        trade_logged_in: config.trade_logged_in,
        trade_runtime: config.trade_runtime.clone(),
        mcp_catalog,
        mcp_store: Arc::clone(&adk_store),
    };
    bundle.installed_adapters = bundle.derive_installed_adapters();
    Ok(bundle)
}
