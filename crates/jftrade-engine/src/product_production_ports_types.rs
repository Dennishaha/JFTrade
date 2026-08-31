use std::collections::{BTreeMap, BTreeSet};
use std::sync::Arc;

use jftrade_api::WebSessionValidator;
use jftrade_calendar::CalendarManager;
use jftrade_settings::MarketDataProvider;
use jftrade_store_sqlite::AdkStore;
use jftrade_store_settings_file::SettingsFileStore;
use serde_json::Value;

use super::product_backtest_sync_registry::BacktestSyncWorkerRegistry;
use super::product_production_ports_execution::ExecutionReconciliationWorker;
use super::product_production_adapter_bindings::ProductionAdapterBinding;
use super::product_production_database_leases::ProductionDatabaseLeaseSnapshot;
use super::product_production_ports_strategy::StrategyRuntimeManager;
use super::product_production_ports_trade::SharedTradeReadRuntime;
use crate::product::product_active_provider_state::{ActiveProviderState, ProviderRuntimeSnapshot};
use crate::product::product_adk_chat_stream_port::AdkChatStreamPort;
use crate::product::product_adk_mutation_port::AdkMutationPort;
use crate::product::product_alerts_write_port::{
    AlertWriteAction, AlertWritePort, AlertWritePortError, AlertWriteResolution, AlertWriteRoute,
};
use crate::product::product_auth_session_manager::AuthSessionInvalidationPort;
use crate::product::product_backtest_execution::BacktestExecutionTaskRegistry;
use crate::product::product_backtests_write_port::BacktestsWritePort;
use crate::product::product_brokers_write_port::BrokersWritePort;
use crate::product::product_execution_write_port::ExecutionWritePort;
use crate::product::product_market_data_provider_actions_port::MarketDataProviderActionsPort;
use crate::product::product_market_data_subscription_mutation_port::MarketDataSubscriptionMutationPort;
use crate::product::product_plugins_write_port::PluginWritePort;
use crate::product::product_production_route_registry::ProductionRouteAdapter;
use crate::product::product_research_preset_write_port::ResearchPresetWritePort;
use crate::product::product_research_screen_write_port::ResearchScreenWritePort;
use crate::product::product_strategy_definition_write_port::StrategyDefinitionWritePort;
use crate::product::product_strategy_runtime_write_port::StrategyRuntimeWritePort;
use crate::product::product_system_write_port::SystemWritePort;
use crate::product::product_watchlist_remote_write_port::RemoteWatchlistWritePort;
use crate::product::product_watchlist_write_port::WatchlistWritePort;
use crate::product::strategy_pine::StrategyPineAnalyzeSnapshotPort;
use crate::product::{
    AdkReadSnapshotPort, AlertKind, AlertSnapshotError, AlertSnapshotPort, AuthSessionSnapshotPort,
    AuthSessionWritePort, BacktestReadSnapshotPort, BacktestSyncReadSnapshotPort,
    BrokerReadSnapshotPort, ExecutionReadSnapshotPort, MarketDataCatalogReadSnapshotPort,
    MarketDataDerivativeReadSnapshotPort, MarketDataNewsActionsReadSnapshotPort,
    MarketDataNewsSearchReadSnapshotPort, MarketDataOptionsReadSnapshotPort,
    MarketDataPredictionReadSnapshotPort, MarketDataProviderReadSnapshotPort,
    MarketDataQuoteReadSnapshotPort, PluginSnapshotPort, PluginUninstallGuidanceSnapshotPort,
    PortfolioSnapshotPort, ProductConfig, ResearchPresetReadSnapshotPort, ResearchReadSnapshotPort,
    StrategyDefinitionSnapshotPort, StrategyReadSnapshotPort, StrategyRuntimeStatusPort,
    SystemReadSnapshotPort, WatchlistMembershipSnapshotPort, WatchlistReadSnapshotPort,
    WsLiveSnapshotPort,
};

pub(crate) fn provider_request_matches(
    provider: MarketDataProvider,
    query: &crate::product::product_query::QueryMap,
) -> bool {
    ["brokerId", "providerBrokerId"].iter().all(|key| {
        query
            .get_first(key)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .is_none_or(|requested| provider_matches_broker_id(provider, requested))
    })
}

pub(crate) fn provider_matches_broker_id(provider: MarketDataProvider, requested: &str) -> bool {
    let requested = requested.trim();
    match provider {
        MarketDataProvider::Futu => requested.eq_ignore_ascii_case("futu"),
        MarketDataProvider::Yfinance => {
            requested.eq_ignore_ascii_case("yfinance")
                || requested.eq_ignore_ascii_case("yahoo-finance")
        }
        MarketDataProvider::Akshare => requested.eq_ignore_ascii_case("akshare"),
    }
}

pub(crate) fn research_tool_binding(
    snapshot: &ProviderRuntimeSnapshot,
    config: &ProductConfig,
    operation: &str,
) -> ProductionAdapterBinding {
    let helper_provider = matches!(
        snapshot.provider,
        Some(MarketDataProvider::Yfinance) | Some(MarketDataProvider::Akshare)
    );
    let ready = match operation {
        "instrument" | "financials" => snapshot.helper_ready && helper_provider,
        "valuation" => {
            snapshot.provider == Some(MarketDataProvider::Futu)
                && snapshot.opend_ready
                && config
                    .trade_runtime
                    .as_ref()
                    .is_some_and(|runtime| runtime.valuation_detail_available())
        }
        "news" => {
            (snapshot.helper_ready && helper_provider)
                || (snapshot.provider == Some(MarketDataProvider::Futu)
                    && snapshot.opend_ready
                    && config
                        .trade_runtime
                        .as_ref()
                        .is_some_and(|runtime| runtime.news_reader_available()))
        }
        // Stock screening has a concrete embedded-helper adapter.  Keep it
        // separate from the ResearchRead umbrella used by the legacy GET
        // route so provider transitions cannot leave the screen tool Ready
        // after switching away from a healthy helper.
        "screen" | "screens" => snapshot.helper_ready && helper_provider,
        // The compatibility catalog retains these broker operations, but the
        // production bundle has no technical/short-interest/institution
        // reader. They must remain explicitly unavailable until a typed
        // adapter is installed.
        "technical" | "technical_indicators" | "short-interest" | "short_interest"
        | "institutions" => false,
        _ => false,
    };
    if ready {
        ProductionAdapterBinding::Ready
    } else {
        ProductionAdapterBinding::ExternalUnavailable
    }
}

#[derive(Clone, Debug)]
pub(crate) struct ProductionAlertPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
    pub(crate) trade_runtime: Option<Arc<super::SharedTradeReadRuntime>>,
}

impl AlertSnapshotPort for ProductionAlertPort {
    fn snapshot(&self, kind: AlertKind, raw_query: &str) -> Result<Value, AlertSnapshotError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() || !snapshot.opend_ready {
            return Err(AlertSnapshotError::Unavailable(
                "alert provider runtime is not configured".to_owned(),
            ));
        }
        let reader = self
            .trade_runtime
            .as_ref()
            .and_then(|runtime| runtime.alert_reader())
            .ok_or_else(|| {
                AlertSnapshotError::Unavailable("alert provider runtime is unavailable".to_owned())
            })?;
        let (entries, feature_id) = match kind {
            AlertKind::Price => {
                let market = raw_query
                    .split('&')
                    .find_map(|pair| pair.strip_prefix("market="))
                    .filter(|v| !v.is_empty());
                (
                    reader
                        .price(market)
                        .map_err(|e| AlertSnapshotError::Provider {
                            status: None,
                            message: e.to_string(),
                        })?,
                    "alerts.price.list",
                )
            }
            AlertKind::OptionEvents => {
                let count = raw_query
                    .split('&')
                    .find_map(|pair| pair.strip_prefix("pageSize="))
                    .and_then(|v| v.parse::<i32>().ok())
                    .unwrap_or(100);
                let page = raw_query
                    .split('&')
                    .find_map(|pair| pair.strip_prefix("cursor="))
                    .filter(|v| !v.is_empty());
                (
                    reader.option_events(count, page).map_err(|e| {
                        AlertSnapshotError::Provider {
                            status: None,
                            message: e.to_string(),
                        }
                    })?,
                    "alerts.option_event.list",
                )
            }
        };
        let now = crate::product::product_production_ports::provider_now_rfc3339();
        Ok(serde_json::json!({
            "asOf": now,
            "entries": entries,
            "hasMore": false,
            "total": entries.len(),
            "metadata": {"source": "futu-opend"},
            "provider": {"brokerId": "futu", "featureId": feature_id, "capability": "available", "selectionReason": "active_provider", "resolvedAt": now, "asOf": now}
        }))
    }
}

impl AlertWritePort for ProductionAlertPort {
    fn resolve(
        &self,
        _route: AlertWriteRoute,
        broker_id: Option<&str>,
        _account_id: Option<&str>,
    ) -> Result<AlertWriteResolution, AlertWritePortError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() || !snapshot.opend_ready {
            return Err(AlertWritePortError::Unavailable(
                "alert provider runtime is not configured".to_owned(),
            ));
        }
        if broker_id.is_some_and(|id| !id.eq_ignore_ascii_case("futu")) {
            return Err(AlertWritePortError::CapabilityUnavailable(
                "only futu alerts are supported".to_owned(),
            ));
        }
        Ok(AlertWriteResolution {
            broker_id: "futu".to_owned(),
            security_firm: "Futu/Moomoo via OpenD".to_owned(),
            capability: "available".to_owned(),
            selection_reason: if broker_id.is_some() {
                "explicit_broker".to_owned()
            } else {
                "active_provider".to_owned()
            },
        })
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
        let runtime = self
            .trade_runtime
            .as_ref()
            .and_then(|runtime| runtime.alert_writer())
            .ok_or_else(|| {
                AlertWritePortError::Unavailable("alert provider runtime is unavailable".to_owned())
            })?;
        let payload = _action
            .payload
            .as_ref()
            .ok_or_else(|| AlertWritePortError::Internal("alert payload is required".to_owned()))?;
        validate_alert_payload(_action.route, payload)?;
        let value = match _action.route {
            AlertWriteRoute::Price => runtime.set_price(payload),
            AlertWriteRoute::OptionEvents => runtime.set_option_event(payload),
        };
        value
            .map(Some)
            .map_err(|error| AlertWritePortError::Provider {
                status: None,
                message: error.to_string(),
            })
    }
}

fn validate_alert_payload(
    route: AlertWriteRoute,
    payload: &Value,
) -> Result<(), AlertWritePortError> {
    let object = payload
        .as_object()
        .ok_or_else(|| AlertWritePortError::Provider {
            status: Some(400),
            message: "alert payload must be an object".to_owned(),
        })?;
    match route {
        AlertWriteRoute::Price => {
            let symbol = object
                .get("symbol")
                .and_then(Value::as_str)
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .ok_or_else(|| AlertWritePortError::Provider {
                    status: Some(400),
                    message: "symbol is required".to_owned(),
                })?;
            if !symbol.contains('.') {
                return Err(AlertWritePortError::Provider {
                    status: Some(400),
                    message: "symbol must be MARKET.CODE".to_owned(),
                });
            }
            let valid_price = object
                .get("price")
                .and_then(Value::as_f64)
                .is_some_and(|value| value.is_finite() && value > 0.0);
            if !valid_price {
                return Err(AlertWritePortError::Provider {
                    status: Some(400),
                    message: "price must be a positive finite number".to_owned(),
                });
            }
            if !object.get("enabled").is_some_and(Value::is_boolean) {
                return Err(AlertWritePortError::Provider {
                    status: Some(400),
                    message: "enabled must be a boolean".to_owned(),
                });
            }
        }
        AlertWriteRoute::OptionEvents => {
            if object.get("operation").is_none() {
                return Err(AlertWritePortError::Provider {
                    status: Some(400),
                    message: "operation is required".to_owned(),
                });
            }
            let alert_list = object
                .get("alertList")
                .and_then(Value::as_array)
                .filter(|items| !items.is_empty())
                .ok_or_else(|| AlertWritePortError::Provider {
                    status: Some(400),
                    message: "alertList must contain at least one alert".to_owned(),
                })?;
            if let Some(index) = alert_list.iter().position(|item| !item.is_object()) {
                return Err(AlertWritePortError::Provider {
                    status: Some(400),
                    message: format!("alertList[{index}] must be an object"),
                });
            }
        }
    }
    Ok(())
}

#[derive(Clone)]
pub(crate) struct ProductionPortBundle {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
    /// Settings file handle opened by the production composition.  Keeping
    /// this concrete handle in the bundle lets route installation proof be
    /// derived from the adapters that were actually constructed rather than
    /// from a separate hand-maintained list.
    pub(crate) settings_store: Arc<SettingsFileStore>,
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
    pub strategy_runtime_manager: Arc<StrategyRuntimeManager>,
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
    pub remote_watchlist: Arc<dyn crate::product::RemoteWatchlistSnapshotPort>,
    pub remote_watchlist_write: Arc<dyn RemoteWatchlistWritePort>,
    pub market_data_subscription_mutation: Arc<dyn MarketDataSubscriptionMutationPort>,
    pub market_data_provider_actions: Arc<dyn MarketDataProviderActionsPort>,
    pub research_screen_write: Arc<dyn ResearchScreenWritePort>,
    pub strategy_pine_analyze: Arc<dyn StrategyPineAnalyzeSnapshotPort>,
    pub ws_live: Arc<dyn WsLiveSnapshotPort>,
    /// Concrete production adapters installed by the composition root.
    /// Readiness in `bound_adapters` may legitimately be
    /// `ExternalUnavailable`; this set is the independent proof that the
    /// handler/port itself was actually wired.
    pub(crate) installed_adapters: BTreeSet<ProductionRouteAdapter>,
    pub(crate) bound_adapters: BTreeMap<ProductionRouteAdapter, ProductionAdapterBinding>,
    pub(crate) backtest_sync_workers: Arc<BacktestSyncWorkerRegistry>,
    pub(crate) backtest_execution_workers: Arc<BacktestExecutionTaskRegistry>,
    pub(crate) execution_reconciliation_worker: Option<Arc<ExecutionReconciliationWorker>>,
    pub(crate) backtest_execution_ready: bool,
    #[allow(dead_code)]
    pub(crate) trade_read_port: Option<Arc<dyn jftrade_integration_futu::TradeReadPort>>,
    #[allow(dead_code)]
    pub(crate) trade_write_port: Option<Arc<dyn jftrade_integration_futu::TradeWritePort>>,
    #[allow(dead_code)]
    pub(crate) trade_logged_in: Option<bool>,
    #[allow(dead_code)]
    pub(crate) trade_runtime: Option<Arc<SharedTradeReadRuntime>>,
    /// Concrete ADK catalog/store used by the production MCP listener. The
    /// listener is assembled at the composition root and shuts down before
    /// this bundle releases its SQLite leases.
    pub(crate) mcp_catalog: Arc<super::ProductionToolCatalog>,
    pub(crate) mcp_store: Arc<AdkStore>,
}

impl ProductionPortBundle {
    /// Derive the route-adapter installation proof from the concrete ports in
    /// this bundle.  Readiness is deliberately kept separate: a concrete
    /// adapter can be installed while its external Provider/OpenD dependency
    /// is unavailable and therefore projects `ExternalUnavailable`.
    pub(crate) fn derive_installed_adapters(&self) -> BTreeSet<ProductionRouteAdapter> {
        let mut adapters = BTreeSet::new();
        macro_rules! register_port {
            ($port:expr, $($adapter:ident),+ $(,)?) => {{
                let _ = &$port;
                adapters.extend([$(ProductionRouteAdapter::$adapter),+]);
            }};
        }

        register_port!(self.settings_store, Settings, SystemCore);
        register_port!(self.database_leases, DataManagement);
        register_port!(self.auth_session, AuthSessionRead);
        register_port!(self.auth_session_write, AuthSessionWrite);
        register_port!(self.system_read, SystemRead);
        register_port!(self.system_write, SystemOpenDWrite, RealTradeControlWrite);
        register_port!(self.calendar_manager, Calendar);
        register_port!(self.watchlist_memberships, WatchlistMemberships);
        register_port!(self.watchlist, WatchlistRead);
        register_port!(self.watchlist_write, WatchlistWrite);
        register_port!(self.remote_watchlist, RemoteWatchlistRead, RemoteWatchlistWrite);
        register_port!(self.strategy_definition, StrategyDefinitionRead, StrategyDefinitionWrite);
        register_port!(self.strategy_read, StrategyRuntimeRead);
        register_port!(self.strategy_runtime_status, StrategyRuntimeRead);
        register_port!(self.strategy_runtime_write, StrategyRuntimeWrite);
        register_port!(self.strategy_pine_analyze, StrategyPine);
        register_port!(self.research_read, ResearchRead, ResearchRankingsRead, ResearchIndustriesRead, ResearchCalendarRead, ResearchMacroRead);
        register_port!(self.research_preset_read, ResearchPresetRead);
        register_port!(self.research_preset_write, ResearchPresetWrite);
        register_port!(self.research_screen_write, ResearchCatalog, ResearchScreenWrite);
        register_port!(self.backtest_read, BacktestRead);
        register_port!(self.backtest_sync, BacktestSyncRead);
        register_port!(self.backtests_write, BacktestStart, BacktestDelete, BacktestSyncStart, BacktestSyncCancel);
        register_port!(self.execution_read, ExecutionRead);
        register_port!(self.execution_write, ExecutionWrite);
        register_port!(self.broker, BrokerRead);
        register_port!(self.brokers_write, BrokerWrite);
        register_port!(self.portfolio, PortfolioRead);
        register_port!(self.provider, MarketDataProviderRead);
        register_port!(self.catalog, MarketDataMarketsRead, MarketDataSearchRead);
        register_port!(self.market_data_quote, MarketDataSubscriptionRead, MarketDataSecuritiesRead, MarketDataSnapshotsRead, MarketDataCandlesRead, MarketDataDepthRead, MarketDataTicksRead, MarketDataBrokerQueueRead, MarketDataCapitalFlowRead, MarketDataIntradayRead, MarketDataProfileRead);
        register_port!(self.market_data_derivative, MarketDataDerivativeRead, MarketDataFuturesRead);
        register_port!(self.market_data_options, MarketDataOptionsRead, MarketDataOptionsChainRead, MarketDataOptionsExpirationsRead, MarketDataOptionsScreenRead, MarketDataOptionsAnalysisRead, MarketDataOptionsEventsRead, MarketDataOptionsUnusualRead, MarketDataOptionsZeroDteRead, MarketDataOptionsZeroDteContractRead, MarketDataOptionsEarningsRead, MarketDataOptionsSellerRead);
        register_port!(self.market_data_news_actions, MarketDataNewsActionsRead);
        register_port!(self.market_data_news_search, MarketDataNewsSearchRead);
        register_port!(self.market_data_prediction, MarketDataPredictionRead);
        register_port!(self.market_data_subscription_mutation, MarketDataSubscriptionAcquireWrite, MarketDataSubscriptionReleaseWrite, MarketDataSubscriptionClearWrite, MarketDataSubscriptionHeartbeatWrite, MarketDataPredictionSubscriptionAcquireWrite, MarketDataPredictionSubscriptionReleaseWrite);
        register_port!(self.market_data_provider_actions, MarketDataInstrumentsNormalizeWrite, MarketDataBatchSnapshotsWrite, MarketDataOptionsAnalysisWrite, MarketDataZeroDteWrite, MarketDataPredictionCombosWrite);
        register_port!(self.plugins, PluginsRead);
        register_port!(self.plugin_write, PluginsWrite);
        register_port!(self.plugin_guidance, PluginGuidanceRead);
        register_port!(self.alert_snapshot, AlertsRead);
        register_port!(self.alert_write, AlertsWrite);
        register_port!(self.adk_read, AdkTemplatesRead, AdkRead);
        register_port!(self.adk_mutation, AdkMutation);
        register_port!(self.adk_chat_stream, AdkChat);
        register_port!(self.ws_live, WebSocketLive);
        adapters
    }

    pub(crate) fn shutdown_strategy_runtime(&self) {
        self.strategy_runtime_manager.shutdown();
    }

    /// Stop assistant provider calls and join approval continuations before
    /// the ADK SQLite leases are dropped.  The port owns the concrete runtime
    /// behind the trait object, so the supervisor does not need to know its
    /// implementation details.
    pub(crate) fn shutdown_adk_runtime(&self) {
        self.adk_chat_stream.shutdown();
    }

    pub(crate) fn backtest_sync_workers(&self) -> Arc<BacktestSyncWorkerRegistry> {
        Arc::clone(&self.backtest_sync_workers)
    }
    pub(crate) fn backtest_execution_workers(&self) -> Arc<BacktestExecutionTaskRegistry> {
        Arc::clone(&self.backtest_execution_workers)
    }

    pub(crate) fn execution_reconciliation_worker(
        &self,
    ) -> Option<Arc<ExecutionReconciliationWorker>> {
        self.execution_reconciliation_worker.clone()
    }
    #[cfg(test)]
    pub(crate) const fn backtest_execution_ready(&self) -> bool {
        self.backtest_execution_ready
    }
}
