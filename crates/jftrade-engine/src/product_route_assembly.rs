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
    Plugins,
    PluginUninstallGuidance,
    Alerts,
    StrategyDefinitions,
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
            ProductCapability::Plugins,
            ProductCapability::PluginUninstallGuidance,
            ProductCapability::Alerts,
            ProductCapability::StrategyDefinitions,
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
                    | ProductCapability::Plugins
                    | ProductCapability::PluginUninstallGuidance
                    | ProductCapability::Alerts
                    | ProductCapability::StrategyDefinitions
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
    plugins: bool,
    plugin_uninstall_guidance: bool,
    strategy_definitions: bool,
}

fn product_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Result<RouteCatalog, RouteCatalogError> {
    let mut routes = Vec::new();
    routes.extend(product_system_routes(capabilities));
    routes.extend(product_settings_routes(capabilities));
    if ports.alerts && capabilities.contains(ProductCapability::Alerts) {
        routes.extend(product_alert_routes());
    }
    routes.extend(product_calendar_routes(capabilities, ports));
    routes.extend(product_data_management_routes(capabilities));
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
include!("product_routes_watchlist_research_trading.rs");
