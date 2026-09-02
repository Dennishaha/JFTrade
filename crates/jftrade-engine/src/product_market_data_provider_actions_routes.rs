use super::product_market_data_provider_actions_port::MARKET_DATA_PROVIDER_ACTIONS_ROUTES;

/// The integration branch must gate these routes on both this group port and
/// its explicit test-cutover capability. The default product route catalog
/// must not include them.
pub fn market_data_provider_actions_route_specs() -> Vec<(&'static str, &'static str)> {
    MARKET_DATA_PROVIDER_ACTIONS_ROUTES.to_vec()
}
