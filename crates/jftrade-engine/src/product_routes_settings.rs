fn product_settings_routes(capabilities: &ProductCapabilities) -> Vec<RouteSpec> {
    let mut routes = [
        "/api/v1/settings/ui",
        "/api/v1/settings/brokers",
        "/api/v1/settings/onboarding",
        "/api/v1/settings/execution",
        "/api/v1/settings/adk",
        "/api/v1/settings/adk/mcp",
        "/api/v1/settings/system-notifications",
        "/api/v1/settings/pine-worker",
        "/api/v1/settings/security",
        "/api/v1/settings/market-data-provider",
        "/api/v1/settings/backtest-market-data-provider",
        "/api/v1/settings/exchange-calendars",
    ]
    .into_iter()
    .map(|path| route("GET", path))
    .collect::<Vec<_>>();
    for (capability, method, path) in [
        (ProductCapability::AppearanceWrite, "PUT", "/api/v1/settings/ui"),
        (ProductCapability::OnboardingWrite, "PUT", "/api/v1/settings/onboarding"),
        (ProductCapability::CalendarSettingsWrite, "PUT", "/api/v1/settings/exchange-calendars"),
        (ProductCapability::MarketDataProviderWrite, "PUT", "/api/v1/settings/market-data-provider"),
        (ProductCapability::BacktestMarketDataProviderWrite, "PUT", "/api/v1/settings/backtest-market-data-provider"),
        (ProductCapability::ExecutionWrite, "PUT", "/api/v1/settings/execution"),
        (ProductCapability::AssistantRuntimeWrite, "PUT", "/api/v1/settings/adk"),
        (ProductCapability::McpServerWrite, "PUT", "/api/v1/settings/adk/mcp"),
        (ProductCapability::McpServerWrite, "POST", "/api/v1/settings/adk/mcp/token/reset"),
        (ProductCapability::SystemNotificationsWrite, "PUT", "/api/v1/settings/system-notifications"),
        (ProductCapability::SystemNotificationsWrite, "POST", "/api/v1/settings/system-notifications/test"),
        (ProductCapability::PineWorkerWrite, "PUT", "/api/v1/settings/pine-worker"),
        (ProductCapability::SecurityWrite, "PUT", "/api/v1/settings/security"),
        (ProductCapability::BrokerSettingsWrite, "PUT", "/api/v1/settings/brokers/{brokerId}/integration"),
        (ProductCapability::BrokerSettingsWrite, "POST", "/api/v1/settings/broker-accounts"),
        (ProductCapability::BrokerSettingsWrite, "PUT", "/api/v1/settings/broker-accounts/{accountRecordId}"),
        (ProductCapability::BrokerSettingsWrite, "DELETE", "/api/v1/settings/broker-accounts/{accountRecordId}"),
    ] {
        if capabilities.contains(capability) {
            routes.push(route(method, path));
        }
    }
    routes
}
