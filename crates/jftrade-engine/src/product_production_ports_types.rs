use std::collections::BTreeMap;
use std::sync::Arc;

use jftrade_api::WebSessionValidator;
use jftrade_calendar::CalendarManager;
use jftrade_settings::MarketDataProvider;
use serde_json::Value;

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
use crate::product::product_plugins_write_port::PluginWritePort;
use crate::product::product_research_preset_write_port::ResearchPresetWritePort;
use crate::product::product_strategy_definition_write_port::StrategyDefinitionWritePort;
use crate::product::product_strategy_runtime_write_port::StrategyRuntimeWritePort;
use crate::product::product_system_write_port::SystemWritePort;
use crate::product::product_watchlist_remote_write_port::RemoteWatchlistWritePort;
use crate::product::product_watchlist_write_port::WatchlistWritePort;
use crate::product::strategy_pine::StrategyPineAnalyzeSnapshotPort;
use crate::product::{
    AdkReadSnapshotPort, AlertKind, AlertSnapshotError, AlertSnapshotPort,
    AuthSessionSnapshotPort, AuthSessionWritePort, BacktestReadSnapshotPort,
    BacktestSyncReadSnapshotPort, BrokerReadSnapshotPort, ExecutionReadSnapshotPort,
    MarketDataCatalogReadSnapshotPort, MarketDataDerivativeReadSnapshotPort,
    MarketDataNewsActionsReadSnapshotPort, MarketDataNewsSearchReadSnapshotPort,
    MarketDataOptionsReadSnapshotPort, MarketDataPredictionReadSnapshotPort,
    MarketDataProviderReadSnapshotPort, MarketDataQuoteReadSnapshotPort, PluginSnapshotPort,
    PluginUninstallGuidanceSnapshotPort, ProductConfig, PortfolioSnapshotPort,
    ResearchPresetReadSnapshotPort, ResearchReadSnapshotPort, StrategyDefinitionSnapshotPort,
    StrategyReadSnapshotPort, StrategyRuntimeStatusPort, SystemReadSnapshotPort,
    WatchlistMembershipSnapshotPort, WatchlistReadSnapshotPort, WsLiveSnapshotPort,
};
use super::product_production_adapter_bindings::ProductionAdapterBinding;
use super::product_production_ports_trade::SharedTradeReadRuntime;
use super::product_production_database_leases::ProductionDatabaseLeaseSnapshot;
use crate::product::product_production_route_registry::ProductionRouteAdapter;
use super::product_backtest_sync_registry::BacktestSyncWorkerRegistry;
use crate::product::product_market_data_provider_actions_port::MarketDataProviderActionsPort;
use crate::product::product_market_data_subscription_mutation_port::MarketDataSubscriptionMutationPort;
use crate::product::product_research_screen_write_port::ResearchScreenWritePort;

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
        "news" => snapshot.helper_ready && helper_provider,
        _ => false,
    };
    if ready { ProductionAdapterBinding::Ready } else { ProductionAdapterBinding::ExternalUnavailable }
}

#[derive(Clone, Debug)]
pub(crate) struct ProductionAlertPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
}

impl AlertSnapshotPort for ProductionAlertPort {
    fn snapshot(&self, _kind: AlertKind, _raw_query: &str) -> Result<Value, AlertSnapshotError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() || !snapshot.opend_ready {
            return Err(AlertSnapshotError::Unavailable("alert provider runtime is not configured".to_owned()));
        }
        Err(AlertSnapshotError::Unavailable("alert provider runtime is not configured".to_owned()))
    }
}

impl AlertWritePort for ProductionAlertPort {
    fn resolve(&self, _route: AlertWriteRoute, _broker_id: Option<&str>, _account_id: Option<&str>) -> Result<AlertWriteResolution, AlertWritePortError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() || !snapshot.opend_ready {
            return Err(AlertWritePortError::Unavailable("alert provider runtime is not configured".to_owned()));
        }
        Err(AlertWritePortError::Unavailable("alert provider runtime is not configured".to_owned()))
    }
    fn apply(&self, _resolution: &AlertWriteResolution, _action: &AlertWriteAction) -> Result<Option<Value>, AlertWritePortError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() || !snapshot.opend_ready {
            return Err(AlertWritePortError::Unavailable("alert provider runtime is not configured".to_owned()));
        }
        Err(AlertWritePortError::Unavailable("alert provider runtime is not configured".to_owned()))
    }
}

#[derive(Clone)]
pub(crate) struct ProductionPortBundle {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
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
    pub remote_watchlist: Arc<dyn crate::product::RemoteWatchlistSnapshotPort>,
    pub remote_watchlist_write: Arc<dyn RemoteWatchlistWritePort>,
    pub market_data_subscription_mutation: Arc<dyn MarketDataSubscriptionMutationPort>,
    pub market_data_provider_actions: Arc<dyn MarketDataProviderActionsPort>,
    pub research_screen_write: Arc<dyn ResearchScreenWritePort>,
    pub strategy_pine_analyze: Arc<dyn StrategyPineAnalyzeSnapshotPort>,
    pub ws_live: Arc<dyn WsLiveSnapshotPort>,
    pub(crate) bound_adapters: BTreeMap<ProductionRouteAdapter, ProductionAdapterBinding>,
    pub(crate) backtest_sync_workers: Arc<BacktestSyncWorkerRegistry>,
    pub(crate) backtest_execution_workers: Arc<BacktestExecutionTaskRegistry>,
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
    pub(crate) fn backtest_sync_workers(&self) -> Arc<BacktestSyncWorkerRegistry> { Arc::clone(&self.backtest_sync_workers) }
    pub(crate) fn backtest_execution_workers(&self) -> Arc<BacktestExecutionTaskRegistry> { Arc::clone(&self.backtest_execution_workers) }
    #[cfg(test)]
    pub(crate) const fn backtest_execution_ready(&self) -> bool { self.backtest_execution_ready }
}
