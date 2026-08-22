use std::collections::BTreeSet;

#[derive(Clone, Copy, Debug, Eq, Ord, PartialEq, PartialOrd)]
enum ProductCapability {
    AuthSession,
    AppearanceWrite,
    OnboardingWrite,
    CalendarSettingsWrite,
    MarketDataProviderWrite,
    BacktestMarketDataProviderWrite,
    ExecutionWrite,
    AssistantRuntimeWrite,
    McpServerWrite,
    SystemNotificationsWrite,
    PineWorkerWrite,
    SecurityWrite,
    BrokerSettingsWrite,
    DataManagementPreview,
    DataManagementMaintenance,
    CalendarSources,
    CalendarStatus,
    CalendarControl,
    WatchlistMemberships,
    WatchlistRead,
    Portfolio,
    ResearchRead,
    ResearchPresetRead,
    ExecutionRead,
    MarketDataProviderRead,
    MarketDataCatalogRead,
    MarketDataDerivativeRead,
    MarketDataOptionsRead,
    MarketDataNewsActionsRead,
    MarketDataNewsSearchRead,
    AdkRead,
    MarketDataQuoteRead,
    MarketDataPredictionRead,
    BrokerRead,
    RemoteWatchlistRead,
    SystemRead,
    Plugins,
    PluginUninstallGuidance,
    Alerts,
    StrategyDefinitions,
    BacktestRead,
    BacktestSyncRead,
    StrategyRead,
    WsLive,
}

#[derive(Clone, Debug, Default)]
struct ProductCapabilities(BTreeSet<ProductCapability>);

impl ProductCapabilities {
    #[cfg(test)]
    fn test_cutover() -> Self {
        Self(BTreeSet::from([
            ProductCapability::AuthSession,
            ProductCapability::AppearanceWrite,
            ProductCapability::OnboardingWrite,
            ProductCapability::CalendarSettingsWrite,
            ProductCapability::MarketDataProviderWrite,
            ProductCapability::BacktestMarketDataProviderWrite,
            ProductCapability::ExecutionWrite,
            ProductCapability::AssistantRuntimeWrite,
            ProductCapability::McpServerWrite,
            ProductCapability::SystemNotificationsWrite,
            ProductCapability::PineWorkerWrite,
            ProductCapability::SecurityWrite,
            ProductCapability::BrokerSettingsWrite,
            ProductCapability::DataManagementPreview,
            ProductCapability::DataManagementMaintenance,
            ProductCapability::CalendarSources,
            ProductCapability::CalendarStatus,
            ProductCapability::CalendarControl,
            ProductCapability::WatchlistMemberships,
            ProductCapability::WatchlistRead,
            ProductCapability::Portfolio,
            ProductCapability::ResearchRead,
            ProductCapability::ResearchPresetRead,
            ProductCapability::ExecutionRead,
            ProductCapability::MarketDataProviderRead,
            ProductCapability::MarketDataCatalogRead,
            ProductCapability::MarketDataDerivativeRead,
            ProductCapability::MarketDataOptionsRead,
            ProductCapability::MarketDataNewsActionsRead,
            ProductCapability::MarketDataNewsSearchRead,
            ProductCapability::AdkRead,
            ProductCapability::MarketDataQuoteRead,
            ProductCapability::MarketDataPredictionRead,
            ProductCapability::BrokerRead,
            ProductCapability::RemoteWatchlistRead,
            ProductCapability::SystemRead,
            ProductCapability::Plugins,
            ProductCapability::PluginUninstallGuidance,
            ProductCapability::Alerts,
            ProductCapability::StrategyDefinitions,
            ProductCapability::BacktestRead,
            ProductCapability::BacktestSyncRead,
            ProductCapability::StrategyRead,
            ProductCapability::WsLive,
        ]))
    }

    fn contains(&self, capability: ProductCapability) -> bool {
        self.0.contains(&capability)
    }

    fn is_empty(&self) -> bool {
        self.0.is_empty()
    }

    fn requires_writable_settings(&self) -> bool {
        self.0.iter().any(|capability| {
            !matches!(
                capability,
                ProductCapability::DataManagementPreview
                    | ProductCapability::AuthSession
                    | ProductCapability::DataManagementMaintenance
                    | ProductCapability::CalendarSources
                    | ProductCapability::CalendarStatus
                    | ProductCapability::CalendarControl
                    | ProductCapability::WatchlistMemberships
                    | ProductCapability::WatchlistRead
                    | ProductCapability::Portfolio
                    | ProductCapability::ResearchRead
                    | ProductCapability::ResearchPresetRead
                    | ProductCapability::ExecutionRead
                    | ProductCapability::MarketDataProviderRead
                    | ProductCapability::MarketDataCatalogRead
                    | ProductCapability::MarketDataDerivativeRead
                    | ProductCapability::MarketDataOptionsRead
                    | ProductCapability::MarketDataNewsActionsRead
                    | ProductCapability::MarketDataNewsSearchRead
                    | ProductCapability::AdkRead
                    | ProductCapability::MarketDataQuoteRead
                    | ProductCapability::MarketDataPredictionRead
                    | ProductCapability::BrokerRead
                    | ProductCapability::RemoteWatchlistRead
                    | ProductCapability::SystemRead
                    | ProductCapability::Plugins
                    | ProductCapability::PluginUninstallGuidance
                    | ProductCapability::Alerts
                    | ProductCapability::StrategyDefinitions
                    | ProductCapability::BacktestRead
                    | ProductCapability::BacktestSyncRead
                    | ProductCapability::StrategyRead
                    | ProductCapability::WsLive
            )
        })
    }

    #[cfg(test)]
    fn only(capability: ProductCapability) -> Self {
        Self(BTreeSet::from([capability]))
    }
}

#[derive(Clone, Copy, Debug, Default)]
struct ProductRoutePorts {
    auth_session: bool,
    alerts: bool,
    calendar_manager: bool,
    watchlist_memberships: bool,
    watchlist_read: bool,
    portfolio: bool,
    research_read: bool,
    research_preset_read: bool,
    execution_read: bool,
    market_data_provider_read: bool,
    market_data_catalog_read: bool,
    market_data_derivative_read: bool,
    market_data_options_read: bool,
    market_data_news_actions_read: bool,
    market_data_news_search_read: bool,
    adk_read: bool,
    market_data_quote_read: bool,
    market_data_prediction_read: bool,
    broker_read: bool,
    remote_watchlist: bool,
    system_read: bool,
    plugins: bool,
    plugin_uninstall_guidance: bool,
    strategy_definitions: bool,
    backtest_read: bool,
    backtest_sync_read: bool,
    strategy_read: bool,
    ws_live: bool,
}

fn product_route_ports(config: &ProductConfig) -> ProductRoutePorts {
    ProductRoutePorts {
        auth_session: config.auth_session_snapshot_port.is_some(),
        alerts: config.alert_snapshot_port.is_some(),
        calendar_manager: config.calendar_manager.is_some(),
        watchlist_memberships: config.watchlist_membership_snapshot_port.is_some(),
        watchlist_read: config.watchlist_read_snapshot_port.is_some(),
        portfolio: config.portfolio_snapshot_port.is_some(),
        research_read: config.research_read_snapshot_port.is_some(),
        research_preset_read: config.research_preset_read_snapshot_port.is_some(),
        execution_read: config.execution_read_snapshot_port.is_some(),
        market_data_provider_read: config
            .market_data_provider_read_snapshot_port
            .is_some(),
        market_data_catalog_read: config
            .market_data_catalog_read_snapshot_port
            .is_some(),
        market_data_derivative_read: config
            .market_data_derivative_read_snapshot_port
            .is_some(),
        market_data_options_read: config
            .market_data_options_read_snapshot_port
            .is_some(),
        market_data_news_actions_read: config
            .market_data_news_actions_read_snapshot_port
            .is_some(),
        market_data_news_search_read: config
            .market_data_news_search_read_snapshot_port
            .is_some(),
        adk_read: config.adk_read_snapshot_port.is_some(),
        market_data_quote_read: config.market_data_quote_read_snapshot_port.is_some(),
        market_data_prediction_read: config
            .market_data_prediction_read_snapshot_port
            .is_some(),
        broker_read: config.broker_read_snapshot_port.is_some(),
        system_read: config.system_read_snapshot_port.is_some(),
        backtest_read: config.backtest_read_snapshot_port.is_some(),
        backtest_sync_read: config.backtest_sync_read_snapshot_port.is_some(),
        strategy_read: config.strategy_read_snapshot_port.is_some(),
        ws_live: config
            .ws_live_snapshot_port
            .as_ref()
            .is_some_and(|port| port.enabled()),
        remote_watchlist: config.remote_watchlist_snapshot_port.is_some(),
        plugin_uninstall_guidance: config.plugin_uninstall_guidance_snapshot_port.is_some(),
        plugins: config.plugin_snapshot_port.is_some(),
        strategy_definitions: config.strategy_definition_snapshot_port.is_some(),
    }
}

fn product_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Result<RouteCatalog, RouteCatalogError> {
    let mut routes = Vec::new();
    routes.extend(product_auth_routes(capabilities, ports));
    routes.extend(product_system_routes(capabilities, ports));
    routes.extend(product_settings_routes(capabilities));
    if ports.alerts && capabilities.contains(ProductCapability::Alerts) {
        routes.extend(product_alert_routes());
    }
    routes.extend(product_calendar_routes(capabilities, ports));
    routes.extend(product_data_management_routes(capabilities));
    routes.extend(product_backtest_routes(capabilities, ports));
    routes.extend(product_backtest_sync_routes(capabilities, ports));
    routes.extend(product_execution_read_routes(capabilities, ports));
    routes.extend(product_market_data_provider_read_routes(capabilities, ports));
    routes.extend(product_market_data_catalog_read_routes(capabilities, ports));
    routes.extend(product_market_data_derivative_read_routes(capabilities, ports));
    routes.extend(product_market_data_options_read_routes(capabilities, ports));
    routes.extend(product_market_data_news_actions_read_routes(capabilities, ports));
    routes.extend(product_market_data_news_search_read_routes(capabilities, ports));
    routes.extend(product_adk_read_routes(capabilities, ports));
    routes.extend(product_market_data_quote_read_routes(capabilities, ports));
    routes.extend(product_market_data_prediction_read_routes(capabilities, ports));
    routes.extend(product_strategy_read_routes(capabilities, ports));
    if ports.ws_live && capabilities.contains(ProductCapability::WsLive) {
        routes.push(route(WS_LIVE_ROUTE.0, WS_LIVE_ROUTE.1));
    }
    routes.extend(product_watchlist_research_trading_routes(
        capabilities,
        ports,
    ));
    RouteCatalog::new(routes)
}

include!("product_routes_system.rs");
include!("product_routes_auth.rs");
include!("product_routes_settings.rs");
include!("product_routes_alerts.rs");
include!("product_routes_calendar.rs");
include!("product_routes_data_management.rs");
include!("product_routes_backtests.rs");
include!("product_routes_execution.rs");
include!("product_routes_market_data_provider_read.rs");
include!("product_routes_market_data_catalog_read.rs");
include!("product_routes_market_data_derivative_read.rs");
include!("product_routes_market_data_options_read.rs");
include!("product_market_data_news_actions_read_routes.rs");
include!("product_market_data_news_search_read_routes.rs");
include!("product_adk_read_routes.rs");
include!("product_market_data_quote_read_routes.rs");
include!("product_routes_market_data_prediction_read.rs");
include!("product_routes_strategies.rs");
include!("product_routes_watchlist_research_trading.rs");
