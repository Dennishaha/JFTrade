fn product_data_management_routes(capabilities: &ProductCapabilities) -> Vec<RouteSpec> {
    let mut routes = vec![route(
        "GET",
        "/api/v1/settings/data-management/databases",
    )];
    if capabilities.contains(ProductCapability::DataManagementPreview) {
        routes.push(route(
            "POST",
            "/api/v1/settings/data-management/cleanup/preview",
        ));
    }
    routes
}
