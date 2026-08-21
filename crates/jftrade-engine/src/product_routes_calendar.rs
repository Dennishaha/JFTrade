fn product_calendar_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Vec<RouteSpec> {
    let mut routes = Vec::new();
    if ports.calendar_manager && capabilities.contains(ProductCapability::CalendarSources) {
        routes.push(route("GET", "/api/v1/system/exchange-calendars/sources"));
    }
    if ports.calendar_manager && capabilities.contains(ProductCapability::CalendarStatus) {
        routes.push(route("GET", "/api/v1/system/exchange-calendars/status"));
    }
    if ports.calendar_manager && capabilities.contains(ProductCapability::CalendarControl) {
        routes.extend([
            route("POST", "/api/v1/system/exchange-calendars/probe"),
            route(
                "POST",
                "/api/v1/system/exchange-calendars/probe/{market}",
            ),
            route("POST", "/api/v1/system/exchange-calendars/refresh"),
            route(
                "POST",
                "/api/v1/system/exchange-calendars/refresh/{market}",
            ),
        ]);
    }
    routes
}
