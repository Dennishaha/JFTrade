use std::collections::BTreeSet;

#[derive(Clone, Copy, Debug, Eq, Ord, PartialEq, PartialOrd)]
enum ProductCapability {
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
}

#[derive(Clone, Debug, Default)]
struct ProductCapabilities(BTreeSet<ProductCapability>);

impl ProductCapabilities {
    #[cfg(test)]
    fn test_cutover() -> Self {
        Self(BTreeSet::from([
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
                    | ProductCapability::DataManagementMaintenance
                    | ProductCapability::CalendarSources
                    | ProductCapability::CalendarStatus
                    | ProductCapability::CalendarControl
                    | ProductCapability::WatchlistMemberships
                    | ProductCapability::WatchlistRead
                    | ProductCapability::Portfolio
                    | ProductCapability::ResearchRead
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
    alerts: bool,
    calendar_manager: bool,
    watchlist_memberships: bool,
    watchlist_read: bool,
    portfolio: bool,
    research_read: bool,
    broker_read: bool,
    remote_watchlist: bool,
    system_read: bool,
    plugins: bool,
    plugin_uninstall_guidance: bool,
    strategy_definitions: bool,
    backtest_read: bool,
    backtest_sync_read: bool,
    strategy_read: bool,
}

fn product_route_ports(config: &ProductConfig) -> ProductRoutePorts {
    ProductRoutePorts {
        alerts: config.alert_snapshot_port.is_some(),
        calendar_manager: config.calendar_manager.is_some(),
        watchlist_memberships: config.watchlist_membership_snapshot_port.is_some(),
        watchlist_read: config.watchlist_read_snapshot_port.is_some(),
        portfolio: config.portfolio_snapshot_port.is_some(),
        research_read: config.research_read_snapshot_port.is_some(),
        broker_read: config.broker_read_snapshot_port.is_some(),
        system_read: config.system_read_snapshot_port.is_some(),
        backtest_read: config.backtest_read_snapshot_port.is_some(),
        backtest_sync_read: config.backtest_sync_read_snapshot_port.is_some(),
        strategy_read: config.strategy_read_snapshot_port.is_some(),
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
    routes.extend(product_system_routes(capabilities, ports));
    routes.extend(product_settings_routes(capabilities));
    if ports.alerts && capabilities.contains(ProductCapability::Alerts) {
        routes.extend(product_alert_routes());
    }
    routes.extend(product_calendar_routes(capabilities, ports));
    routes.extend(product_data_management_routes(capabilities));
    routes.extend(product_backtest_routes(capabilities, ports));
    routes.extend(product_backtest_sync_routes(capabilities, ports));
    routes.extend(product_strategy_read_routes(capabilities, ports));
    routes.extend(product_watchlist_research_trading_routes(
        capabilities,
        ports,
    ));
    RouteCatalog::new(routes)
}

include!("product_routes_system.rs");
include!("product_routes_settings.rs");
include!("product_routes_alerts.rs");
include!("product_routes_calendar.rs");
include!("product_routes_data_management.rs");
include!("product_routes_backtests.rs");
include!("product_routes_strategies.rs");
include!("product_routes_watchlist_research_trading.rs");
