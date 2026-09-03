fn new_market_data_subscription_mutation_api(
    optional_ports: &ProductOptionalPorts,
) -> MarketDataSubscriptionMutationApi {
    MarketDataSubscriptionMutationApi::new(
        optional_ports
            .write_ports
            .market_data_subscription_mutation
            .clone(),
    )
}

fn is_broker_integration_path(path: &str) -> bool {
    path.strip_prefix("/api/v1/settings/brokers/")
        .and_then(|value| value.strip_suffix("/integration"))
        .is_some_and(|id| !id.is_empty() && !id.contains('/'))
}

fn duration_millis(duration: Duration) -> u64 {
    u64::try_from(duration.as_millis()).unwrap_or(u64::MAX)
}
