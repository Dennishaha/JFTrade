use super::*;
use crate::product::product_production_route_registry::ProductionRouteRegistry;
use std::collections::BTreeSet;

fn trading_path(path: &str) -> bool {
    [
        "/api/v1/portfolio/",
        "/api/v1/brokers/",
        "/api/v1/execution/",
    ]
    .iter()
    .any(|prefix| path.starts_with(prefix))
}

// Parity: go:452dea11:internal/api/trading/openapi_route_alignment_test.go:14 TestTradingOpenAPIDocumentationRoutesMatchGinRegistration
#[tokio::test]
async fn trading_documented_operations_match_production_registration_and_resolve() {
    let (_directory, bundle, handle) = seeded_scope_fixture(None, Vec::new()).await;
    let registry = ProductionRouteRegistry::bind(&bundle).expect("production registry");
    let document: Value =
        serde_json::from_str(include_str!("../../../contracts/openapi/openapi.json")).unwrap();
    let mut documented = BTreeSet::new();
    for (path, operations) in document["paths"].as_object().unwrap() {
        if !trading_path(path) {
            continue;
        }
        for method in operations.as_object().unwrap().keys() {
            if ["get", "post", "delete", "put", "patch", "head", "options"]
                .contains(&method.as_str())
            {
                // The Rust route identity is its method and path template.
                let identity = format!("{} {path}", method.to_uppercase());
                assert!(!identity.trim().is_empty());
                assert!(
                    documented.insert(identity),
                    "duplicate documentation identity"
                );
            }
        }
    }
    let mut registered = BTreeSet::new();
    for binding in registry.bindings().iter().filter(|b| trading_path(&b.path)) {
        assert!(!binding.method.trim().is_empty());
        assert!(!binding.path.trim().is_empty());
        assert!(
            registered.insert(format!("{} {}", binding.method, binding.path)),
            "duplicate registration"
        );
        let concrete = binding
            .path
            .split('/')
            .map(|s| match s {
                "{brokerId}" => "futu",
                "{internalOrderId}" => "fixture-order",
                _ => s,
            })
            .collect::<Vec<_>>()
            .join("/");
        let resolved = registry
            .resolve(&binding.method, &concrete)
            .expect("documented concrete route resolves");
        assert_eq!(resolved.dispatch_target(), binding.dispatch_target());
    }
    assert_eq!(registered, documented);
    // Retain all original 22 documented method/path assertions. Rust's newer
    // capability and preview operations also participate in the set equality.
    for (method, path) in [
        ("GET", "/api/v1/portfolio/{brokerId}/cash-balances"),
        ("GET", "/api/v1/portfolio/{brokerId}/positions"),
        ("GET", "/api/v1/brokers/{brokerId}/funds"),
        ("GET", "/api/v1/brokers/{brokerId}/positions"),
        ("GET", "/api/v1/brokers/{brokerId}/orders"),
        ("GET", "/api/v1/brokers/{brokerId}/fills"),
        ("GET", "/api/v1/brokers/{brokerId}/cash-flows"),
        ("GET", "/api/v1/brokers/{brokerId}/order-fees"),
        ("GET", "/api/v1/brokers/{brokerId}/margin-ratios"),
        ("GET", "/api/v1/brokers/{brokerId}/max-trade-qtys"),
        ("GET", "/api/v1/brokers/{brokerId}/quote"),
        ("GET", "/api/v1/brokers/{brokerId}/klines"),
        ("GET", "/api/v1/brokers/{brokerId}/securities"),
        ("GET", "/api/v1/brokers/{brokerId}/runtime"),
        ("POST", "/api/v1/brokers/{brokerId}/orders"),
        ("DELETE", "/api/v1/brokers/{brokerId}/orders"),
        ("POST", "/api/v1/brokers/{brokerId}/unlock"),
        ("GET", "/api/v1/execution/orders"),
        ("GET", "/api/v1/execution/orders/{internalOrderId}"),
        ("POST", "/api/v1/execution/orders"),
        ("POST", "/api/v1/execution/orders/{internalOrderId}/cancel"),
        ("GET", "/api/v1/execution/orders/{internalOrderId}/events"),
    ] {
        assert!(
            registered.contains(&format!("{method} {path}")),
            "original route {method} {path}"
        );
    }
    handle.shutdown().await.unwrap();
    bundle.shutdown_adk_runtime().await.unwrap();
    bundle.shutdown_strategy_runtime().unwrap();
}
