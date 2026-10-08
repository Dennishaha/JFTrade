use super::*;
use crate::product::product_production_ports::{ProductionPortBundle, production_ports};
use crate::product::{ProductConfig, ProductHandle, start_product};
use jftrade_settings::{ExecutionSettings, ExecutionSettingsStorePort, SecuritySettingsService};
use jftrade_store_settings_file::SettingsFileStore;
use jftrade_store_sqlite::{ExecutionOrderStore, StoredExecutionOrder, StoredExecutionOrderEvent};
use serde_json::json;

#[path = "product_production_ports_execution_http_route_tests.rs"]
mod execution_http_route_tests;

fn seeded_order(environment: &str, market: &str, symbol: &str) -> StoredExecutionOrder {
    let simulate = environment == "SIMULATE";
    StoredExecutionOrder {
        internal_order_id: format!("scope-{environment}"),
        broker_id: "futu".to_owned(),
        broker_order_id: Some(if simulate { "1001" } else { "2001" }.to_owned()),
        broker_order_id_ex: None,
        source: "api".to_owned(),
        source_detail: String::new(),
        trading_environment: environment.to_owned(),
        account_id: if simulate { "SIM-001" } else { "REAL-001" }.to_owned(),
        market: market.to_owned(),
        symbol: Some(symbol.to_owned()),
        side: Some("BUY".to_owned()),
        order_type: Some("LIMIT".to_owned()),
        status: "SUBMITTED".to_owned(),
        raw_broker_status: Some("SUBMITTED".to_owned()),
        requested_quantity: Some(if simulate { 100.0 } else { 1.0 }),
        requested_price: None,
        filled_quantity: None,
        filled_average_price: None,
        remark: None,
        last_error: None,
        last_error_code: None,
        last_error_source: None,
        submitted_at: None,
        updated_at: "2026-10-08T00:00:00Z".to_owned(),
        created_at: "2026-10-08T00:00:00Z".to_owned(),
        order_kind: "single".to_owned(),
        product_class: "equity".to_owned(),
        quantity_mode: "units".to_owned(),
        client_order_id: None,
        preview_id: None,
        normalized_request: "{}".to_owned(),
        requested_amount: None,
        payout: None,
        fees: None,
    }
}

async fn scope_fixture(
    default_environment: Option<&str>,
    real_market: &str,
    real_symbol: &str,
) -> (tempfile::TempDir, ProductionPortBundle, ProductHandle) {
    let mut initial = vec![
        seeded_order("SIMULATE", "HK", "HK.00700"),
        seeded_order("REAL", real_market, real_symbol),
    ];
    if real_market == "HK" {
        for order in &mut initial {
            order.requested_quantity = Some(100.0);
        }
    }
    seeded_scope_fixture(default_environment, initial).await
}

async fn seeded_scope_fixture(
    default_environment: Option<&str>,
    initial: Vec<StoredExecutionOrder>,
) -> (tempfile::TempDir, ProductionPortBundle, ProductHandle) {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let directory = tempfile::tempdir().unwrap();
    let settings = directory.path().join("settings.json");
    std::fs::write(&settings, b"{}").unwrap();
    crate::product::product_data_management::initialize_production_databases(&settings).unwrap();
    let settings_store = Arc::new(SettingsFileStore::open(&settings).unwrap());
    if let Some(environment) = default_environment {
        settings_store
            .save_execution(&ExecutionSettings {
                default_trading_environment: environment.to_owned(),
                ..ExecutionSettings::default()
            })
            .unwrap();
    }
    // Seed before composition obtains the production writer lease.
    let store = ExecutionOrderStore::open(directory.path().join("execution-orders.db")).unwrap();
    for order in initial {
        let id = order.internal_order_id.clone();
        let timestamp = order.updated_at.clone();
        store
            .save_order_and_event(
                order,
                &timestamp,
                &StoredExecutionOrderEvent {
                    id: &format!("placed-{id}"),
                    internal_order_id: &id,
                    event_type: "COMMAND_PLACE_ACCEPTED",
                    previous_status: None,
                    next_status: "SUBMITTED",
                    payload_json: "{}",
                    created_at: &timestamp,
                },
            )
            .unwrap();
    }
    drop(store);
    let security = SecuritySettingsService::new(settings_store);
    let mut production = ProductConfig::new(
        "127.0.0.1:0".parse().unwrap(),
        &settings,
        jftrade_api::AccessPolicy::desktop(Some("a".repeat(32))),
    )
    .unwrap();
    production.production = true;
    production.capabilities = crate::product::ProductCapabilities::all();
    let bundle = production_ports(&production, &security).unwrap();
    let http = ProductConfig::test_cutover("127.0.0.1:0".parse().unwrap(), &settings)
        .unwrap()
        .with_execution_read_snapshot_port(bundle.execution_read.clone());
    let handle = start_product(http).await.unwrap();
    (directory, bundle, handle)
}

// Parity: go:452dea11:internal/store/trading/ledger_test.go:12 TestExecutionOrderStoreSortingFilteringAndMissingOrderBoundaries
#[tokio::test]
async fn execution_http_ledger_filters_broker_and_market_and_preserves_descending_ties() {
    let initial = [
        ("exec-000001", "2026-07-03T07:00:00Z"),
        ("exec-000002", "2026-07-03T07:30:00Z"),
        ("exec-000003", "2026-07-03T07:30:00Z"),
    ]
    .into_iter()
    .map(|(id, created)| {
        let mut order = seeded_order("SIMULATE", "HK", "HK.00700");
        order.internal_order_id = id.to_owned();
        order.account_id = "SIM-001".to_owned();
        order.created_at = created.to_owned();
        order.updated_at = "2026-07-03T08:00:00Z".to_owned();
        order
    })
    .collect();
    let (directory, bundle, handle) = seeded_scope_fixture(None, initial).await;
    let all = orders(&handle, "?brokerId=FUTU").await;
    let rows = all.as_array().expect("orders");
    assert_eq!(rows.len(), 3);
    assert_eq!(
        rows.iter()
            .map(|row| row["internalOrderId"].as_str().expect("id"))
            .collect::<Vec<_>>(),
        ["exec-000003", "exec-000002", "exec-000001"]
    );
    assert_eq!(rows[0]["createdAt"], "2026-07-03T07:30:00Z");
    assert_eq!(rows[2]["createdAt"], "2026-07-03T07:00:00Z");
    assert!(
        rows.iter()
            .all(|row| row["updatedAt"] == "2026-07-03T08:00:00Z")
    );
    assert_eq!(orders(&handle, "?brokerId=ib").await, json!([]));
    assert_eq!(orders(&handle, "?market=US").await, json!([]));
    assert_eq!(orders(&handle, "?brokerId=futu&market=hk").await, all);
    let response = reqwest::Client::builder()
        .no_proxy()
        .timeout(std::time::Duration::from_secs(5))
        .build()
        .expect("client")
        .get(format!(
            "http://{}/api/v1/execution/orders/missing-order",
            handle.startup_record().address
        ))
        .send()
        .await
        .expect("missing order HTTP");
    assert_eq!(response.status().as_u16(), 404);
    let absent: Value = response.json().await.expect("error envelope");
    assert_eq!(absent["ok"], false);
    assert_eq!(absent["error"]["code"], "ORDER_NOT_FOUND");
    handle.shutdown().await.expect("HTTP shutdown");
    bundle
        .shutdown_adk_runtime()
        .await
        .expect("Assistant shutdown");
    bundle
        .shutdown_strategy_runtime()
        .expect("strategy shutdown");
    drop(bundle);
    let reopened = ExecutionOrderStore::open(directory.path().join("execution-orders.db"))
        .expect("reopen ledger");
    let durable = reopened.list_orders().expect("persisted sorted orders");
    assert_eq!(
        durable
            .iter()
            .map(|row| row.internal_order_id.as_str())
            .collect::<Vec<_>>(),
        ["exec-000003", "exec-000002", "exec-000001"]
    );
    for row in durable {
        assert_eq!(
            reopened
                .list_order_events(&row.internal_order_id)
                .expect("events")
                .len(),
            1,
            "reads must not add lifecycle events"
        );
    }
    assert!(
        reopened
            .get_order("missing-order")
            .expect("absent persisted order")
            .is_none()
    );
}

async fn orders(handle: &ProductHandle, query: &str) -> Value {
    let response = reqwest::Client::builder()
        .timeout(std::time::Duration::from_secs(5))
        .build()
        .unwrap()
        .get(format!(
            "http://{}/api/v1/execution/orders{query}",
            handle.startup_record().address
        ))
        .send()
        .await
        .unwrap();
    assert_eq!(response.status(), 200);
    let body: Value = response.json().await.unwrap();
    assert_eq!(body["ok"], true, "{body}");
    body["data"]["orders"].clone()
}

// Parity: go:452dea11:internal/app/apiserver/servercoretest/execution_routes_test.go:53 TestExecutionOrdersEndpointFiltersByTradingEnvironmentAndScope
#[tokio::test]
async fn execution_http_orders_filter_original_opaque_accounts_and_environment() {
    let (_directory, bundle, handle) = scope_fixture(None, "US", "US.AAPL").await;
    let default = orders(&handle, "").await;
    let real = orders(&handle, "?tradingEnvironment=REAL").await;
    let scoped = orders(
        &handle,
        "?brokerId=futu&tradingEnvironment=REAL&accountId=REAL-001&market=US",
    )
    .await;
    let mismatch = orders(
        &handle,
        "?brokerId=futu&tradingEnvironment=REAL&accountId=SIM-001&market=HK",
    )
    .await;
    handle.shutdown().await.unwrap();
    bundle.shutdown_adk_runtime().await.unwrap();
    bundle.shutdown_strategy_runtime().unwrap();
    assert_eq!(default.as_array().unwrap().len(), 1);
    assert_eq!(default[0]["tradingEnvironment"], "SIMULATE");
    assert_eq!(default[0]["accountId"], "SIM-001");
    assert_eq!(real.as_array().unwrap().len(), 1);
    assert_eq!(real[0]["tradingEnvironment"], "REAL");
    assert_eq!(scoped.as_array().unwrap().len(), 1);
    assert_eq!(scoped[0]["accountId"], "REAL-001");
    assert_eq!(scoped[0]["market"], "US");
    assert_eq!(scoped, real);
    assert_eq!(mismatch, json!([]));
}

// Parity: go:452dea11:internal/app/apiserver/servercoretest/execution_routes_test.go:110 TestExecutionOrdersEndpointDefaultTradingEnvironmentFromSettings
#[tokio::test]
async fn execution_http_orders_default_to_real_from_production_settings() {
    let (_directory, bundle, handle) = scope_fixture(Some("REAL"), "HK", "HK.00700").await;
    let default = orders(&handle, "").await;
    let explicit = orders(&handle, "?tradingEnvironment=SIMULATE").await;
    handle.shutdown().await.unwrap();
    bundle.shutdown_adk_runtime().await.unwrap();
    bundle.shutdown_strategy_runtime().unwrap();
    assert_eq!(default.as_array().unwrap().len(), 1);
    assert_eq!(default[0]["tradingEnvironment"], "REAL");
    assert_eq!(default[0]["accountId"], "REAL-001");
    assert_eq!(default[0]["market"], "HK");
    assert_eq!(default[0]["symbol"], "HK.00700");
    assert_eq!(explicit.as_array().unwrap().len(), 1);
    assert_eq!(explicit[0]["tradingEnvironment"], "SIMULATE");
    assert_eq!(explicit[0]["accountId"], "SIM-001");
}
