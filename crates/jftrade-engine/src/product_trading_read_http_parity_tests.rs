//! Production HTTP order receipts, filtering and native broker read boundaries.

use super::*;
use jftrade_integration_futu::{
    TradeAccountSnapshot, TradeCashFlowSnapshot, TradeFilter, TradeFundsSnapshot, TradeHeader,
    TradeMarginRatioSnapshot, TradeMaxTradeQuantityRequest, TradeMaxTradeQuantitySnapshot,
    TradeOrderFeeSnapshot, TradeOrderSnapshot, TradePositionSnapshot, TradeReadPort, TradeSecurity,
    TradeSessionError,
};
use jftrade_store_sqlite::{ExecutionOrderStore, StoredExecutionOrder};
use std::sync::Mutex;
use std::sync::atomic::{AtomicBool, Ordering};

const AUTH: &[(&str, &str)] = &[("Authorization", "Bearer aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")];
const WHEN: &str = "2026-07-25T08:00:00Z";

fn config(directory: &tempfile::TempDir) -> ProductConfig {
    let path = directory.path().join("settings.json");
    std::fs::write(
        &path,
        r#"{"execution":{"defaultTradingEnvironment":"REAL"}}"#,
    )
    .expect("settings");
    product_data_management::initialize_production_databases(&path).expect("databases");
    let mut config = ProductConfig::desktop_production(
        "127.0.0.1:0".parse().expect("address"),
        &path,
        "a".repeat(32),
    )
    .expect("production config");
    config.capabilities = ProductCapabilities::all();
    config
}

async fn get(handle: &ProductHandle, path: &str) -> (u16, Value) {
    request_json_with_status(handle.startup_record().address, "GET", path, None, AUTH).await
}

fn order(id: &str, status: &str) -> StoredExecutionOrder {
    serde_json::from_value(json!({
        "internalOrderId": id, "brokerId":"futu", "source":"api",
        "sourceDetail":"http-parity", "tradingEnvironment":"REAL", "accountId":"acc-1",
        "market":"US", "symbol":"AAPL", "side":"BUY", "orderType":"LIMIT",
        "status":status, "rawBrokerStatus":"FILLED_PART", "requestedQuantity":10.0,
        "requestedPrice":100.0, "filledQuantity":2.0, "filledAveragePrice":100.0,
        "updatedAt":WHEN, "createdAt":WHEN, "orderKind":"single", "productClass":"equity",
        "quantityMode":"units", "normalizedRequest":"{}"
    }))
    .expect("stored order")
}

fn seed(directory: &tempfile::TempDir, orders: Vec<StoredExecutionOrder>) {
    let store = ExecutionOrderStore::open_existing(
        directory.path().join("execution-orders.db"),
        jftrade_store_sqlite::EXECUTION_ORDERS_PRODUCTION_PROFILE,
    )
    .expect("seed writer");
    for order in orders {
        store.save_order(order, WHEN).expect("seed order");
    }
    // The seed writer releases its lease before the product owns the database.
}

// Parity: go:452dea11:internal/api/trading/execution_test.go:249 TestHandleExecutionOrderDetailsReturnsCanonicalReceiptAndNotFound
#[tokio::test]
async fn production_http_execution_detail_preserves_partial_fill_receipt_after_restart() {
    let directory = tempdir().expect("directory");
    let config = config(&directory);
    seed(&directory, vec![order("internal-1", "PARTIALLY_FILLED")]);
    for _ in 0..2 {
        let handle = start_product(config.clone()).await.expect("production");
        let (status, response) = get(&handle, "/api/v1/execution/orders/internal-1").await;
        assert_eq!(status, 200, "{response}");
        assert_eq!(response["data"]["order"]["internalOrderId"], "internal-1");
        assert_eq!(response["data"]["order"]["status"], "PARTIALLY_FILLED");
        assert_eq!(response["data"]["order"]["rawBrokerStatus"], "FILLED_PART");
        let (status, response) = get(&handle, "/api/v1/execution/orders/missing").await;
        assert_eq!(status, 404, "{response}");
        assert_eq!(response["error"]["code"], "ORDER_NOT_FOUND");
        handle.shutdown().await.expect("shutdown");
    }
}

// Parity: go:452dea11:internal/api/trading/execution_test.go:110 TestHandleExecutionOrdersNormalizesScopeAndFilter
#[tokio::test]
async fn production_http_execution_filter_uses_live_default_environment_and_explicit_override() {
    let directory = tempdir().expect("directory");
    let config = config(&directory);
    let real = order("real-active", "PARTIALLY_FILLED");
    let mut terminal = order("real-filled", "FILLED");
    terminal.raw_broker_status = Some("FILLED_ALL".into());
    terminal.filled_quantity = Some(10.0);
    let mut simulate = real.clone();
    simulate.internal_order_id = "simulate-active".into();
    simulate.trading_environment = "SIMULATE".into();
    let mut other_account = real.clone();
    other_account.internal_order_id = "other-account".into();
    other_account.account_id = "acc-2".into();
    let mut other_broker = real.clone();
    other_broker.internal_order_id = "other-broker".into();
    other_broker.broker_id = "ib".into();
    let mut other_market = real.clone();
    other_market.internal_order_id = "other-market".into();
    other_market.market = "HK".into();
    seed(
        &directory,
        vec![
            real,
            terminal,
            simulate,
            other_account,
            other_broker,
            other_market,
        ],
    );
    let handle = start_product(config).await.expect("production");
    let path = "/api/v1/execution/orders?scope=%20active%20&brokerId=%20futu%20&accountId=%20acc-1%20&market=us";
    let (status, response) = get(&handle, path).await;
    assert_eq!(status, 200, "{response}");
    assert_eq!(
        response["data"]["orders"].as_array().expect("orders").len(),
        1,
        "{response}"
    );
    assert_eq!(
        response["data"]["orders"][0]["internalOrderId"],
        "real-active"
    );
    let (status, response) = get(
        &handle,
        &format!("{path}&tradingEnvironment=%20simulate%20"),
    )
    .await;
    assert_eq!(status, 200, "{response}");
    assert_eq!(
        response["data"]["orders"].as_array().expect("orders").len(),
        1,
        "{response}"
    );
    assert_eq!(
        response["data"]["orders"][0]["internalOrderId"],
        "simulate-active"
    );
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "PUT",
        "/api/v1/settings/execution",
        Some(r#"{"defaultTradingEnvironment":"SIMULATE"}"#),
        AUTH,
    )
    .await;
    assert_eq!(status, 200, "{response}");
    let (status, response) = get(&handle, path).await;
    assert_eq!(status, 200, "{response}");
    assert_eq!(
        response["data"]["orders"].as_array().expect("orders").len(),
        1,
        "{response}"
    );
    assert_eq!(
        response["data"]["orders"][0]["internalOrderId"],
        "simulate-active"
    );
    let (status, response) = get(&handle, &format!("{path}&tradingEnvironment=REAL")).await;
    assert_eq!(status, 200, "{response}");
    assert_eq!(
        response["data"]["orders"][0]["internalOrderId"],
        "real-active"
    );
    handle.shutdown().await.expect("shutdown");
}

#[derive(Debug, Default)]
struct ReadOwner(Mutex<Vec<Value>>, AtomicBool);

impl TradeReadPort for ReadOwner {
    fn read_accounts(
        &self,
        _: u64,
        _: Option<i32>,
        _: Option<bool>,
    ) -> Result<Vec<TradeAccountSnapshot>, TradeSessionError> {
        Ok(vec![
            serde_json::from_value(json!({"trd_env":1,"acc_id":42,
            "trd_market_auth_list":[1,2],"jp_acc_type":[]}))
            .expect("account"),
        ])
    }
    fn read_funds(
        &self,
        header: TradeHeader,
        _: Option<bool>,
        _: Option<i32>,
        _: Option<i32>,
    ) -> Result<TradeFundsSnapshot, TradeSessionError> {
        self.0
            .lock()
            .expect("calls")
            .push(json!({"operation":"funds","header":header}));
        if self.1.load(Ordering::SeqCst) {
            return Err(TradeSessionError::Coordinator(
                jftrade_integration_futu::OpenDSessionCoordinatorError::Closed,
            ));
        }
        Ok(serde_json::from_value(json!({"header":header,"funds":{
            "power":1234.5,"total_assets":1234.5,"cash":1234.5,"market_val":0.0,
            "frozen_cash":0.0,"debt_cash":0.0,"avl_withdrawal_cash":1234.5,"currency":2,
            "cash_info_list":[],"market_info_list":[]}}))
        .expect("funds"))
    }
    fn read_cash_flows(
        &self,
        header: TradeHeader,
        date: String,
        direction: Option<i32>,
    ) -> Result<Vec<TradeCashFlowSnapshot>, TradeSessionError> {
        self.0.lock().expect("calls").push(
            json!({"operation":"cash-flows","header":header,"date":date,"direction":direction}),
        );
        Ok(vec![])
    }
    fn read_order_fees(
        &self,
        header: TradeHeader,
        ids: Vec<String>,
    ) -> Result<Vec<TradeOrderFeeSnapshot>, TradeSessionError> {
        self.0
            .lock()
            .expect("calls")
            .push(json!({"operation":"fees","header":header,"ids":ids}));
        Ok(vec![])
    }
    fn read_margin_ratios(
        &self,
        header: TradeHeader,
        securities: Vec<TradeSecurity>,
    ) -> Result<Vec<TradeMarginRatioSnapshot>, TradeSessionError> {
        self.0
            .lock()
            .expect("calls")
            .push(json!({"operation":"margins","header":header,"securities":securities}));
        Ok(vec![])
    }
    fn read_max_trade_quantity(
        &self,
        _: TradeMaxTradeQuantityRequest,
    ) -> Result<TradeMaxTradeQuantitySnapshot, TradeSessionError> {
        unreachable!("unused max quantity")
    }
    fn read_positions(
        &self,
        header: TradeHeader,
        _: Option<TradeFilter>,
        _: Option<f64>,
        _: Option<f64>,
        _: Option<bool>,
        _: Option<i32>,
        _: Option<i32>,
        _: Option<bool>,
    ) -> Result<Vec<TradePositionSnapshot>, TradeSessionError> {
        self.0
            .lock()
            .expect("calls")
            .push(json!({"operation":"positions","header":header}));
        let position: TradePositionSnapshot =
            serde_json::from_value(json!({"position_id":1,"position_side":0,"code":"AAPL",
            "name":"Apple","qty":10.0,"can_sell_qty":10.0,"price":100.0,"cost_price":98.7,
            "val":1000.0,"pl_val":13.0,"average_cost_price":98.7,"sec_market":11,
            "trd_market":2,"acc_id":42}))
            .expect("position");
        let mut legacy = position.clone();
        legacy.position_id = 2;
        legacy.code = "MSFT".into();
        legacy.average_cost_price = None;
        let mut missing_cost = legacy.clone();
        missing_cost.position_id = 3;
        missing_cost.code = "NVDA".into();
        missing_cost.cost_price = None;
        Ok(vec![position, legacy, missing_cost])
    }
    fn read_orders(
        &self,
        _: TradeHeader,
        _: Option<TradeFilter>,
        _: Vec<i32>,
        _: Option<bool>,
    ) -> Result<Vec<TradeOrderSnapshot>, TradeSessionError> {
        Ok(vec![])
    }
    fn read_fills(
        &self,
        _: TradeHeader,
        _: Option<TradeFilter>,
        _: Option<bool>,
    ) -> Result<Vec<jftrade_integration_futu::TradeFillSnapshot>, TradeSessionError> {
        Ok(vec![])
    }
}

async fn broker_product(directory: &tempfile::TempDir) -> (ProductHandle, Arc<ReadOwner>) {
    let mut config = config(directory);
    let owner = Arc::new(ReadOwner::default());
    let runtime =
        Arc::new(crate::product::product_production_ports::SharedTradeReadRuntime::default());
    runtime.set(Some(owner.clone()), Some(true));
    let router = Arc::new(Mutex::new(jftrade_marketdata::ProviderRouter::new(8)));
    for symbol in ["US.AAPL", "US.MSFT"] {
        router
            .lock()
            .expect("router")
            .cache_mut()
            .insert(
                jftrade_marketdata::Tick {
                    instrument_id: symbol.into(),
                    price: "100".parse().expect("price"),
                    volume: "10".parse().expect("volume"),
                    volume_delta: None,
                    snapshot: None,
                    observed_at_ms: 1_700_000_000_000,
                    provider_generation: 0,
                },
                0,
            )
            .expect("tick");
    }
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, true);
    config = config
        .with_trade_runtime(runtime)
        .with_market_data_router(router)
        .with_active_provider_state(state);
    (start_product(config).await.expect("production"), owner)
}

// Parity: go:452dea11:internal/api/trading/routes_read_handlers_test.go:71 TestTradingReadHandlersValidateAndNormalizeBusinessQueries
#[tokio::test]
async fn production_http_broker_queries_validate_and_forward_native_account_fee_and_margin_inputs()
{
    let directory = tempdir().expect("directory");
    let (handle, owner) = broker_product(&directory).await;
    for path in [
        "/api/v1/brokers/futu/cash-flows?accountId=acc-1",
        "/api/v1/brokers/futu/margin-ratios?market=US",
        "/api/v1/brokers/futu/margin-ratios?market=US&symbol=HK.00700",
        "/api/v1/brokers/futu/quote",
        "/api/v1/brokers/futu/securities",
    ] {
        let (status, response) = get(&handle, path).await;
        assert_eq!(status, 400, "{path}: {response}");
        assert_eq!(response["error"]["code"], "BAD_REQUEST");
    }
    assert!(owner.0.lock().expect("calls").is_empty());
    let(status,response)=get(&handle,"/api/v1/brokers/futu/cash-flows?accountId=acc-1&tradingEnvironment=REAL&market=US&clearingDate=2026-06-23&direction=OUT").await;
    assert_eq!(
        status, 400,
        "native Futu account IDs are numeric: {response}"
    );
    for path in [
        "/api/v1/brokers/futu/cash-flows?accountId=42&tradingEnvironment=REAL&market=US&clearingDate=2026-06-23&direction=OUT",
        "/api/v1/brokers/futu/order-fees?orderIdEx=ord-1&orderIdExList=ord-2,ord-1&market=US",
        "/api/v1/brokers/futu/margin-ratios?market=US&symbol=AAPL&symbols=MSFT,AAPL",
    ] {
        let (status, response) = get(&handle, path).await;
        assert_eq!(status, 200, "{path}: {response}");
    }
    let calls = owner.0.lock().expect("calls").clone();
    assert_eq!(calls.len(), 3, "{calls:?}");
    assert_eq!(calls[0]["date"], "2026-06-23");
    assert_eq!(calls[0]["direction"], 2);
    assert_eq!(calls[0]["header"]["acc_id"], 42);
    assert_eq!(calls[0]["header"]["trd_market"], 2);
    assert_eq!(calls[1]["ids"], json!(["ord-1", "ord-2"]));
    assert_eq!(
        calls[2]["securities"],
        json!([{"market":11,"code":"AAPL"},{"market":11,"code":"MSFT"}])
    );
    for (resource, key) in [("quote", "quotes"), ("securities", "snapshots")] {
        let (status, response) = get(
            &handle,
            &format!("/api/v1/brokers/futu/{resource}?symbol=US.AAPL&symbols=US.MSFT,US.AAPL"),
        )
        .await;
        assert_eq!(status, 200, "{response}");
        let values = response["data"][resource][key]
            .as_array()
            .expect("quotes or snapshots");
        assert_eq!(values.len(), 2, "{response}");
        assert_eq!(values[0]["symbol"], "US.AAPL");
        assert_eq!(values[1]["symbol"], "US.MSFT");
    }
    let (status, _) = get(&handle, "/api/v1/brokers/futu/unsupported-resource").await;
    assert_eq!(status, 404);
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/trading/routes_broker_contracts_test.go:143 TestPortfolioRoutesUseBrokerSnapshotsAndMissingBrokerSemantics
#[tokio::test]
async fn production_http_portfolio_projects_native_cash_positions_and_rejects_unknown_broker() {
    let directory = tempdir().expect("directory");
    let (handle, owner) = broker_product(&directory).await;
    for resource in ["cash-balances", "positions"] {
        let (status, response) = get(
            &handle,
            &format!("/api/v1/portfolio/futu/{resource}?accountId=acc-1&tradingEnvironment=REAL"),
        )
        .await;
        assert_eq!(
            status, 503,
            "original acc-1 has no matching native Futu account: {response}"
        );
        assert_eq!(response["error"]["code"], "PORTFOLIO_UNAVAILABLE");
    }
    assert!(owner.0.lock().expect("calls").is_empty());
    let (status, response) = get(
        &handle,
        "/api/v1/portfolio/futu/cash-balances?accountId=42&tradingEnvironment=REAL",
    )
    .await;
    assert_eq!(status, 200, "{response}");
    assert_eq!(response["data"]["balances"][0]["currency"], "USD");
    assert_eq!(response["data"]["balances"][0]["cashBalance"], 1234.5);
    assert_eq!(response["data"].get("lastError"), Some(&Value::Null));
    let (status, response) = get(
        &handle,
        "/api/v1/portfolio/futu/positions?accountId=42&tradingEnvironment=REAL",
    )
    .await;
    assert_eq!(status, 200, "{response}");
    assert_eq!(response["data"]["positions"][0]["symbol"], "US.AAPL");
    assert_eq!(response["data"]["positions"][0]["market"], "US");
    assert_eq!(response["data"]["positions"][0]["averagePrice"], 98.7);
    assert_eq!(
        response["data"]["positions"]
            .as_array()
            .expect("positions")
            .len(),
        3
    );
    assert_eq!(response["data"]["positions"][1]["averagePrice"], 98.7);
    assert_eq!(response["data"]["positions"][2]["averagePrice"], 0.0);
    assert_eq!(response["data"].get("lastError"), Some(&Value::Null));
    for field in [
        "accountId",
        "averagePrice",
        "brokerId",
        "createdAt",
        "market",
        "marketValue",
        "quantity",
        "symbol",
        "tradingEnvironment",
        "updatedAt",
    ] {
        assert!(
            response["data"]["positions"][0].get(field).is_some(),
            "required PortfolioPosition.{field}: {response}"
        );
    }
    assert_eq!(response["data"]["positions"][0]["brokerId"], "futu");
    assert!(
        response["data"]["positions"][0]["createdAt"]
            .as_str()
            .is_some_and(|value| !value.is_empty())
    );
    assert_eq!(
        response["data"]["positions"][0]["createdAt"],
        response["data"]["positions"][0]["updatedAt"]
    );
    let (status, response) = get(
        &handle,
        "/api/v1/brokers/futu/positions?accountId=42&tradingEnvironment=REAL",
    )
    .await;
    assert_eq!(status, 200, "{response}");
    assert_eq!(response["data"]["positions"][0]["market"], "US");
    assert_eq!(response["data"]["positions"][0]["symbol"], "US.AAPL");
    assert_eq!(response["data"]["positions"][0]["averagePrice"], 98.7);
    let calls = owner.0.lock().expect("calls").clone();
    assert_eq!(calls.len(), 3, "{calls:?}");
    for call in calls {
        assert_eq!(call["header"]["acc_id"], 42);
        assert_eq!(call["header"]["trd_env"], 1);
    }
    let (status, response) = get(&handle, "/api/v1/portfolio/ib/positions").await;
    assert_eq!(status, 404, "{response}");
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/trading/routes_helper_boundaries_test.go:17 TestTradingRouteHelpersWriteHTTPBoundaryErrors
#[tokio::test]
async fn production_http_broker_missing_identifiers_and_native_read_failure_preserve_owner_boundaries()
 {
    let directory = tempdir().expect("directory");
    let (handle, owner) = broker_product(&directory).await;
    for path in ["/api/v1/brokers//funds", "/api/v1/brokers/unknown/quote"] {
        let (status, response) = get(&handle, path).await;
        assert_eq!(status, 404, "{path}: {response}");
    }
    let (status, response) = get(&handle, "/api/v1/brokers/futu/funds?accountId=bad").await;
    assert_eq!(status, 400, "{response}");
    assert_eq!(response["error"]["code"], "BAD_REQUEST");
    assert!(owner.0.lock().expect("calls").is_empty());
    let (status, response) = get(&handle, "/api/v1/brokers/futu/funds?Page=bad").await;
    assert_eq!(
        status, 200,
        "Page is not a declared native broker parameter: {response}"
    );
    assert_eq!(owner.0.lock().expect("calls").len(), 1);
    owner.1.store(true, Ordering::SeqCst);
    let (status, response) = get(&handle, "/api/v1/brokers/futu/funds").await;
    assert_eq!(
        status, 503,
        "native disconnected reader fails closed: {response}"
    );
    assert_eq!(response["error"]["code"], "BROKER_READ_UNAVAILABLE");
    assert_eq!(owner.0.lock().expect("calls").len(), 2);
    handle.shutdown().await.expect("shutdown");
}
