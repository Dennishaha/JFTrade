use std::net::{Ipv4Addr, SocketAddr};

use jftrade_api::AccessPolicy;
use tempfile::tempdir;

use super::*;

#[tokio::test]
async fn product_runtime_without_optional_workers_starts_and_stops_cleanly() {
    let directory = tempdir().expect("temporary directory");
    let product = ProductConfig::new(
        SocketAddr::new(Ipv4Addr::LOCALHOST.into(), 0),
        directory.path().join("settings.json"),
        AccessPolicy::default(),
    )
    .expect("product config");
    let config = ProductRuntimeConfig {
        product,
        pine_workers: Vec::new(),
        marketdata_helper: None,
        market_data_router: None,
        market_data_runtime_recorder: None,
        market_data_opend: None,
        market_data_opend_task: None,
        market_data_opend_provider: None,
        strategy_runtime_registry: None,
    };
    let snapshot = ProductRuntimeState::configured(&config).snapshot();
    let runtime = start_product_runtime(config).await.expect("start runtime");
    assert_eq!(runtime.startup_record().owned_routes, 26);
    assert_eq!(snapshot.resources.len(), 11);
    assert_eq!(snapshot.resources[0].id, "settings-file");
    assert_eq!(snapshot.resources[1].id, "backtest-kline-db");
    assert_eq!(snapshot.resources[9].id, "research-db");
    assert_eq!(snapshot.resources[10].id, "real-trade-control");
    assert!(
        snapshot.resources[1..10]
            .iter()
            .all(|resource| resource.kind == "sqlite")
    );
    runtime.shutdown().await.expect("shutdown");
}

#[tokio::test]
async fn opend_runtime_task_requires_explicit_session_composition() {
    let directory = tempdir().expect("temporary directory");
    let product = ProductConfig::new(
        SocketAddr::new(Ipv4Addr::LOCALHOST.into(), 0),
        directory.path().join("settings.json"),
        AccessPolicy::default(),
    )
    .expect("product config");
    let config = ProductRuntimeConfig {
        product,
        pine_workers: Vec::new(),
        marketdata_helper: None,
        market_data_router: None,
        market_data_runtime_recorder: None,
        market_data_opend: None,
        market_data_opend_task: Some(OpenDSessionRuntimeConfig::default()),
        market_data_opend_provider: None,
        strategy_runtime_registry: None,
    };
    assert!(matches!(
        start_product_runtime(config).await,
        Err(ProductRuntimeError::MissingOpenDSession)
    ));
}
