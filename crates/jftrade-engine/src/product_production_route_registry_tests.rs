use super::*;

fn fixture_registry() -> ProductionRouteRegistry {
    let ledger: RouteLedger = serde_json::from_str(PRODUCTION_ROUTE_MANIFEST).expect("ledger");
    let canonical_routes = ledger.operations.iter().map(|operation| format!("{} {}", operation.method.trim().to_uppercase(), operation.path.trim())).collect::<Vec<_>>();
    let bindings = ledger.operations.into_iter().map(|operation| {
        let method = operation.method.trim().to_uppercase(); let path = operation.path.trim().to_owned();
        let adapter = adapter_for(&operation.capability, &method, &path).expect("canonical operation adapter");
        ProductionRouteBinding { method, path, route_group: operation.capability, adapter, dispatch_target: adapter, adapter_binding: ProductionAdapterBinding::Ready, operation_bindings: BTreeMap::new() }
    }).collect::<Vec<_>>();
    ProductionRouteRegistry::finish(bindings, route_profile_digest(&canonical_routes)).expect("fixture registry")
}

#[test] fn resolver_returns_registered_target_for_dynamic_paths() { let registry = fixture_registry(); let binding = registry.resolve("get", "/api/v1/market-data/candles/US/AAPL").expect("dynamic route"); assert_eq!(binding.dispatch_target(), ProductionRouteAdapter::MarketDataCandlesRead); let binding = registry.resolve("POST", "/api/v1/strategies/instance-1/start").expect("strategy start"); assert_eq!(binding.dispatch_target(), ProductionRouteAdapter::StrategyRuntimeWrite); }
#[test] fn resolver_rejects_unknown_method_and_path() { let registry = fixture_registry(); assert!(registry.resolve("PATCH", "/api/v1/market-data/markets").is_none()); assert!(registry.resolve("GET", "/api/v1/market-data/markets/extra").is_none()); assert!(registry.resolve("GET", "/api/v1/unknown").is_none()); }
#[test] fn every_canonical_template_has_a_dispatch_target() { let registry = fixture_registry(); for binding in registry.bindings() { let concrete = binding.path.split('/').map(|segment| if segment.starts_with('{') && segment.ends_with('}') { "fixture-id" } else { segment }).collect::<Vec<_>>().join("/"); let resolved = registry.resolve(&binding.method, &concrete).expect("canonical operation resolves"); assert_eq!(resolved.dispatch_target(), binding.dispatch_target()); } }
