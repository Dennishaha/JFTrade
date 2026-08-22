use super::product_market_data_subscription_mutation_port::MARKET_DATA_SUBSCRIPTION_MUTATION_ROUTES;

/// The integration branch must gate these routes on this group port and its
/// explicit authenticated test-cutover profile.  The default product route
/// catalog must remain unchanged by this worker slice.
pub fn market_data_subscription_mutation_route_specs() -> Vec<(&'static str, &'static str)> {
    MARKET_DATA_SUBSCRIPTION_MUTATION_ROUTES.to_vec()
}
