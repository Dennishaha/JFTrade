fn product_calendar_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Vec<RouteSpec> {
    let mut routes = Vec::new();
    if ports.calendar_sources && capabilities.contains(ProductCapability::CalendarSources) {
        routes.push(route("GET", "/api/v1/system/exchange-calendars/sources"));
    }
    if ports.calendar_status && capabilities.contains(ProductCapability::CalendarStatus) {
        routes.push(route("GET", "/api/v1/system/exchange-calendars/status"));
    }
    routes
}
