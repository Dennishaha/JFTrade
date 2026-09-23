//! Portfolio / account-order parity tests for the ADK tool surface.
//!
//! Owner: `internal/assistant/assembly/portfolio_tools_test.go` on the `go`
//! branch. The fixture reader records every trade read so the layered tools can
//! be checked for cross-layer fan-out, and it fails individual reads so the
//! partial/error projection is exercised without a live OpenD session.

use super::*;
use std::collections::BTreeSet;

// ---------------------------------------------------------------------------
// `portfolio_tools_test.go` parity fixture
// ---------------------------------------------------------------------------

/// Fixture trade reader for the portfolio/account-order parity tests.
///
/// Records every read so a test can prove the layered tools do not fan out
/// into unrelated broker reads, and fails individual reads so the
/// partial/error projection can be exercised without a live trading session.
struct AdkPortfolioFixtureRead {
    accounts: Vec<jftrade_integration_futu::TradeAccountSnapshot>,
    accounts_error: Option<String>,
    funds: BTreeMap<u64, jftrade_integration_futu::TradeFunds>,
    positions: BTreeMap<u64, Vec<jftrade_integration_futu::TradePositionSnapshot>>,
    orders: BTreeMap<u64, Vec<jftrade_integration_futu::TradeOrderSnapshot>>,
    fail_funds: BTreeSet<u64>,
    fail_positions: BTreeSet<u64>,
    fail_orders: BTreeSet<u64>,
    ledger: Arc<std::sync::Mutex<Vec<String>>>,
}

impl AdkPortfolioFixtureRead {
    fn new(accounts: Vec<jftrade_integration_futu::TradeAccountSnapshot>) -> Self {
        Self {
            accounts,
            accounts_error: None,
            funds: BTreeMap::new(),
            positions: BTreeMap::new(),
            orders: BTreeMap::new(),
            fail_funds: BTreeSet::new(),
            fail_positions: BTreeSet::new(),
            fail_orders: BTreeSet::new(),
            ledger: Arc::new(std::sync::Mutex::new(Vec::new())),
        }
    }

    fn with_funds(mut self, acc_id: u64, funds: jftrade_integration_futu::TradeFunds) -> Self {
        self.funds.insert(acc_id, funds);
        self
    }

    fn with_positions(mut self, acc_id: u64, symbols: &[&str]) -> Self {
        self.positions.insert(
            acc_id,
            symbols
                .iter()
                .map(|symbol| adk_fixture_position(acc_id, symbol))
                .collect(),
        );
        self
    }

    fn with_orders(mut self, acc_id: u64, count: u64) -> Self {
        self.orders.insert(
            acc_id,
            (1..=count)
                .map(|id| adk_fixture_order(acc_id, id))
                .collect(),
        );
        self
    }

    fn with_accounts_error(mut self, message: &str) -> Self {
        self.accounts_error = Some(message.to_owned());
        self
    }

    fn failing_funds(mut self, acc_id: u64) -> Self {
        self.fail_funds.insert(acc_id);
        self
    }

    fn failing_reads(mut self, acc_id: u64) -> Self {
        self.fail_funds.insert(acc_id);
        self.fail_positions.insert(acc_id);
        self.fail_orders.insert(acc_id);
        self
    }

    fn recorded(&self) -> Vec<String> {
        self.ledger.lock().expect("read ledger").clone()
    }

    fn record(&self, entry: String) {
        self.ledger.lock().expect("read ledger").push(entry);
    }

    fn fail(capability: &str) -> jftrade_integration_futu::TradeSessionError {
        jftrade_integration_futu::TradeSessionError::Unsupported(format!(
            "{capability} unavailable"
        ))
    }
}

impl jftrade_integration_futu::TradeReadPort for AdkPortfolioFixtureRead {
    fn read_accounts(
        &self,
        _: u64,
        _: Option<i32>,
        _: Option<bool>,
    ) -> Result<
        Vec<jftrade_integration_futu::TradeAccountSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
        self.record("read_accounts".to_owned());
        match &self.accounts_error {
            Some(message) => Err(jftrade_integration_futu::TradeSessionError::Unsupported(
                message.clone(),
            )),
            None => Ok(self.accounts.clone()),
        }
    }

    fn read_funds(
        &self,
        header: jftrade_integration_futu::TradeHeader,
        _: Option<bool>,
        _: Option<i32>,
        _: Option<i32>,
    ) -> Result<
        jftrade_integration_futu::TradeFundsSnapshot,
        jftrade_integration_futu::TradeSessionError,
    > {
        self.record(format!("read_funds:{}", header.acc_id));
        if self.fail_funds.contains(&header.acc_id) {
            return Err(Self::fail("funds"));
        }
        let funds = self
            .funds
            .get(&header.acc_id)
            .cloned()
            .unwrap_or_else(adk_fixture_zero_funds);
        Ok(jftrade_integration_futu::TradeFundsSnapshot { header, funds })
    }

    fn read_cash_flows(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: String,
        _: Option<i32>,
    ) -> Result<
        Vec<jftrade_integration_futu::TradeCashFlowSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
        Err(Self::fail("cash flows"))
    }

    fn read_order_fees(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Vec<String>,
    ) -> Result<
        Vec<jftrade_integration_futu::TradeOrderFeeSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
        Err(Self::fail("order fees"))
    }

    fn read_margin_ratios(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Vec<jftrade_integration_futu::TradeSecurity>,
    ) -> Result<
        Vec<jftrade_integration_futu::TradeMarginRatioSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
        Err(Self::fail("margin ratios"))
    }

    fn read_max_trade_quantity(
        &self,
        _: jftrade_integration_futu::TradeMaxTradeQuantityRequest,
    ) -> Result<
        jftrade_integration_futu::TradeMaxTradeQuantitySnapshot,
        jftrade_integration_futu::TradeSessionError,
    > {
        Err(Self::fail("max trade quantity"))
    }

    fn read_positions(
        &self,
        header: jftrade_integration_futu::TradeHeader,
        _: Option<jftrade_integration_futu::TradeFilter>,
        _: Option<f64>,
        _: Option<f64>,
        _: Option<bool>,
        _: Option<i32>,
        _: Option<i32>,
        _: Option<bool>,
    ) -> Result<
        Vec<jftrade_integration_futu::TradePositionSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
        self.record(format!("read_positions:{}", header.acc_id));
        if self.fail_positions.contains(&header.acc_id) {
            return Err(Self::fail("positions"));
        }
        Ok(self
            .positions
            .get(&header.acc_id)
            .cloned()
            .unwrap_or_default())
    }

    fn read_orders(
        &self,
        header: jftrade_integration_futu::TradeHeader,
        _: Option<jftrade_integration_futu::TradeFilter>,
        _: Vec<i32>,
        _: Option<bool>,
    ) -> Result<
        Vec<jftrade_integration_futu::TradeOrderSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
        self.record(format!("read_orders:{}", header.acc_id));
        if self.fail_orders.contains(&header.acc_id) {
            return Err(Self::fail("orders"));
        }
        Ok(self.orders.get(&header.acc_id).cloned().unwrap_or_default())
    }

    fn read_fills(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Option<jftrade_integration_futu::TradeFilter>,
        _: Option<bool>,
    ) -> Result<
        Vec<jftrade_integration_futu::TradeFillSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
        Err(Self::fail("fills"))
    }
}

fn adk_fixture_account(
    acc_id: u64,
    environment: i32,
    markets: Vec<i32>,
) -> jftrade_integration_futu::TradeAccountSnapshot {
    jftrade_integration_futu::TradeAccountSnapshot {
        trd_env: environment,
        acc_id,
        trd_market_auth_list: markets,
        acc_type: Some(2),
        card_num: None,
        security_firm: Some(1),
        sim_acc_type: None,
        uni_card_num: None,
        acc_status: Some(0),
        acc_role: Some(1),
        jp_acc_type: Vec::new(),
        competition_acc_name: None,
    }
}

fn adk_fixture_zero_funds() -> jftrade_integration_futu::TradeFunds {
    jftrade_integration_futu::TradeFunds {
        power: 0.0,
        total_assets: 0.0,
        cash: 0.0,
        market_val: 0.0,
        frozen_cash: 0.0,
        debt_cash: 0.0,
        avl_withdrawal_cash: 0.0,
        currency: Some(1),
        available_funds: None,
        unrealized_pl: None,
        realized_pl: None,
        risk_level: None,
        initial_margin: None,
        maintenance_margin: None,
        cash_info_list: Vec::new(),
        max_power_short: None,
        net_cash_power: None,
        long_mv: None,
        short_mv: None,
        pending_asset: None,
        max_withdrawal: None,
        risk_status: None,
        margin_call_margin: None,
        is_pdt: None,
        pdt_seq: None,
        beginning_dtbp: None,
        remaining_dtbp: None,
        dt_call_amount: None,
        dt_status: None,
        securities_assets: None,
        fund_assets: None,
        bond_assets: None,
        market_info_list: Vec::new(),
        crypto_mv: None,
        exposure_level: None,
        exposure_limit: None,
        used_limit: None,
        remaining_limit: None,
    }
}

fn adk_fixture_cash_funds(cash: f64) -> jftrade_integration_futu::TradeFunds {
    jftrade_integration_futu::TradeFunds {
        power: cash,
        total_assets: cash,
        cash,
        avl_withdrawal_cash: cash,
        available_funds: Some(cash),
        ..adk_fixture_zero_funds()
    }
}

fn adk_fixture_currency_balance_funds(cash: f64) -> jftrade_integration_futu::TradeFunds {
    jftrade_integration_futu::TradeFunds {
        cash_info_list: vec![jftrade_integration_futu::TradeCashInfo {
            currency: Some(1),
            cash: Some(cash),
            available_balance: None,
            net_cash_power: None,
        }],
        ..adk_fixture_zero_funds()
    }
}

fn adk_fixture_market_asset_funds(assets: f64) -> jftrade_integration_futu::TradeFunds {
    jftrade_integration_futu::TradeFunds {
        market_info_list: vec![jftrade_integration_futu::TradeMarketInfo {
            trd_market: Some(1),
            assets: Some(assets),
        }],
        ..adk_fixture_zero_funds()
    }
}

fn adk_fixture_position(
    acc_id: u64,
    symbol: &str,
) -> jftrade_integration_futu::TradePositionSnapshot {
    jftrade_integration_futu::TradePositionSnapshot {
        position_id: 1,
        position_side: 1,
        code: symbol.to_owned(),
        name: symbol.to_owned(),
        qty: 2.0,
        can_sell_qty: 2.0,
        price: 100.0,
        cost_price: Some(95.0),
        val: 200.0,
        pl_val: 10.0,
        pl_ratio: Some(5.0),
        sec_market: Some(1),
        trd_market: Some(1),
        diluted_cost_price: None,
        average_cost_price: None,
        average_pl_ratio: None,
        td_pl_val: None,
        td_trd_val: None,
        td_buy_val: None,
        td_buy_qty: None,
        td_sell_val: None,
        td_sell_qty: None,
        unrealized_pl: None,
        realized_pl: None,
        currency: Some(1),
        acc_id: Some(acc_id),
        combo_id: None,
        strategy_type: None,
        position_type: None,
        jp_acc_type: None,
        payout_if_win: None,
    }
}

fn adk_fixture_order(_acc_id: u64, order_id: u64) -> jftrade_integration_futu::TradeOrderSnapshot {
    jftrade_integration_futu::TradeOrderSnapshot {
        trd_side: 1,
        order_type: 1,
        order_status: 2,
        order_id,
        order_id_ex: format!("order-{order_id}"),
        code: "US.AAPL".to_owned(),
        name: "AAPL".to_owned(),
        qty: 2.0,
        price: Some(100.0),
        create_time: "2026-09-01 09:30:00".to_owned(),
        update_time: "2026-09-01 09:31:00".to_owned(),
        fill_qty: None,
        fill_avg_price: None,
        last_err_msg: None,
        sec_market: Some(1),
        create_timestamp: None,
        update_timestamp: None,
        remark: None,
        trd_market: Some(1),
        expire_time: None,
        order_amount: None,
        time_in_force: None,
        fill_outside_rth: None,
        aux_price: None,
        trail_type: None,
        trail_value: None,
        trail_spread: None,
        currency: Some(1),
        session: None,
        jp_acc_type: None,
        strategy_type: None,
        combo_legs: Vec::new(),
    }
}

#[derive(Debug, Default)]
struct AdkPortfolioFixtureOrders {
    queries: std::sync::Mutex<Vec<String>>,
}

impl AdkPortfolioFixtureOrders {
    fn recorded(&self) -> Vec<String> {
        self.queries.lock().expect("order queries").clone()
    }
}

impl crate::product::ExecutionReadSnapshotPort for AdkPortfolioFixtureOrders {
    fn read(
        &self,
        path: &str,
        query: &str,
    ) -> Result<Value, crate::product::ExecutionReadSnapshotError> {
        assert_eq!(path, "/api/v1/execution/orders");
        self.queries
            .lock()
            .expect("order queries")
            .push(query.to_owned());
        Ok(json!({
            "orders": [{
                "internalOrderId": "order-1",
                "accountId": "8240",
                "tradingEnvironment": "REAL",
                "market": "US",
                "status": "SUBMITTED",
            }],
            "checkedAt": "2026-09-01T00:00:00Z",
        }))
    }
}

fn portfolio_fixture_ports(
    reader: Arc<dyn jftrade_integration_futu::TradeReadPort>,
    execution_read: Arc<AdkPortfolioFixtureOrders>,
) -> (
    tempfile::TempDir,
    Arc<crate::product::product_production_ports::ProductionPortBundle>,
) {
    let bundle_dir = tempfile::tempdir().expect("bundle directory");
    let settings_path = bundle_dir.path().join("settings.json");
    std::fs::write(&settings_path, b"{}").expect("write settings");
    crate::product::product_data_management::initialize_production_databases(&settings_path)
        .expect("initialize production databases");
    let settings = Arc::new(
        jftrade_store_settings_file::SettingsFileStore::open(&settings_path)
            .expect("settings store"),
    );
    let security = crate::product::SecuritySettingsService::new(settings);
    let active = Arc::new(crate::product::ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    let runtime =
        Arc::new(crate::product::product_production_ports::SharedTradeReadRuntime::default());
    let mut config = crate::product::ProductConfig::new(
        "127.0.0.1:0".parse().expect("bind address"),
        &settings_path,
        crate::product::AccessPolicy::default(),
    )
    .expect("product config")
    .with_active_provider_state(active)
    .with_trade_runtime(runtime)
    .with_trade_read_port(Some(reader), Some(true))
    .with_market_data_runtime_status_port(Arc::new(AdkTestReadyRuntimeStatus))
    .with_backtest_execution_port(Arc::new(AdkTestBacktestExecution));
    config.capabilities = crate::product::ProductCapabilities::all();
    config.production = true;
    let mut ports = crate::product::product_production_ports::production_ports(&config, &security)
        .expect("production ports");
    ports.execution_read = execution_read;
    (bundle_dir, Arc::new(ports))
}

fn portfolio_fixture_executor(
    ports: &Arc<crate::product::product_production_ports::ProductionPortBundle>,
) -> crate::product::product_adk_model_runtime::ProductionAdkToolExecutor {
    crate::product::product_adk_model_runtime::ProductionAdkToolExecutor::with_ports(
        Arc::clone(&ports.mcp_catalog),
        Arc::clone(&ports.mcp_store),
        Arc::clone(ports),
    )
}

/// Parity: go:452dea11:internal/assistant/assembly/portfolio_tools_test.go:85
/// `TestPortfolioLayeredToolsKeepDiscoveryOverviewAndPositionsSeparate`.
///
/// Go keeps discovery, overview, and positions on separate reads: the accounts
/// tool only discovers, the overview composes one funds+positions+orders read
/// per account, and the positions tool never reads funds. Rust splits Go's
/// combined funds+positions read into explicit per-capability reads, so the
/// ledger asserts that split instead of a combined counter.
#[tokio::test]
async fn portfolio_layered_tools_keep_discovery_overview_and_positions_separate() {
    use crate::product::product_adk_model_runtime::AdkToolExecutor;

    let reader = Arc::new(
        AdkPortfolioFixtureRead::new(vec![adk_fixture_account(8240, 1, vec![2])])
            .with_funds(8240, adk_fixture_cash_funds(1_000.0))
            .with_positions(8240, &["US.AAPL"])
            .with_orders(8240, 1),
    );
    let (_directory, ports) = portfolio_fixture_ports(
        Arc::clone(&reader) as Arc<dyn jftrade_integration_futu::TradeReadPort>,
        Arc::new(AdkPortfolioFixtureOrders::default()),
    );
    let executor = portfolio_fixture_executor(&ports);

    let accounts = executor
        .execute("portfolio.accounts", &json!({"tradingEnvironment": "REAL"}))
        .expect("portfolio.accounts");
    assert_eq!(accounts["discoveredAccounts"].as_array().unwrap().len(), 1);
    assert_eq!(accounts["selection"]["selectedAccountIds"][0], "8240");
    assert!(accounts.get("funds").is_none());
    assert_eq!(reader.recorded(), ["read_accounts"]);

    let overview = executor
        .execute("portfolio.overview", &json!({"tradingEnvironment": "REAL"}))
        .expect("portfolio.overview");
    let overviews = overview["accountOverviews"].as_array().unwrap();
    assert_eq!(overviews.len(), 1);
    assert_eq!(overviews[0]["positionCount"], 1);
    assert_eq!(overviews[0]["orderCount"], 1);
    assert!(overviews[0].get("funds").is_none());
    assert!(overviews[0].get("positions").is_none());
    assert!(overview.get("funds").is_none());

    let positions = executor
        .execute(
            "portfolio.positions",
            &json!({"tradingEnvironment": "REAL"}),
        )
        .expect("portfolio.positions");
    let account_positions = positions["accountPositions"].as_array().unwrap();
    assert_eq!(account_positions.len(), 1);
    assert_eq!(
        account_positions[0]["positions"].as_array().unwrap().len(),
        1
    );
    assert!(account_positions[0].get("funds").is_none());

    assert_eq!(
        reader.recorded(),
        [
            "read_accounts",
            "read_accounts",
            "read_funds:8240",
            "read_positions:8240",
            "read_orders:8240",
            "read_accounts",
            "read_positions:8240",
        ],
        "each tool re-discovers accounts, and the positions layer must not read funds or orders"
    );
}

/// Parity: go:452dea11:internal/assistant/assembly/portfolio_tools_test.go:155
/// `TestPortfolioLayeredToolsReportValidationDiscoveryAndPartialReadStates`
/// (discovery failure and read-warning halves).
#[tokio::test]
async fn portfolio_layered_tools_report_discovery_failure_and_partial_read_states() {
    use crate::product::product_adk_model_runtime::AdkToolExecutor;

    let failing_discovery = Arc::new(
        AdkPortfolioFixtureRead::new(Vec::new())
            .with_accounts_error("OpenD account discovery failed"),
    );
    let (_directory, ports) = portfolio_fixture_ports(
        Arc::clone(&failing_discovery) as Arc<dyn jftrade_integration_futu::TradeReadPort>,
        Arc::new(AdkPortfolioFixtureOrders::default()),
    );
    let executor = portfolio_fixture_executor(&ports);
    for name in [
        "portfolio.accounts",
        "portfolio.overview",
        "portfolio.positions",
    ] {
        let payload = executor
            .execute(name, &json!({"tradingEnvironment": "REAL"}))
            .expect("discovery failure payload");
        assert_eq!(payload["selection"]["status"], "discovery_failed", "{name}");
        assert_eq!(payload["partial"], true, "{name}");
        assert!(
            !payload["warnings"].as_array().unwrap().is_empty(),
            "{name}"
        );
    }

    let failing_reads = Arc::new(
        AdkPortfolioFixtureRead::new(vec![adk_fixture_account(8240, 1, vec![2])])
            .failing_reads(8240),
    );
    let (_directory, ports) = portfolio_fixture_ports(
        Arc::clone(&failing_reads) as Arc<dyn jftrade_integration_futu::TradeReadPort>,
        Arc::new(AdkPortfolioFixtureOrders::default()),
    );
    let executor = portfolio_fixture_executor(&ports);

    let overview = executor
        .execute("portfolio.overview", &json!({"tradingEnvironment": "REAL"}))
        .expect("portfolio.overview with failing reads");
    let item = &overview["accountOverviews"][0];
    assert_eq!(overview["partial"], true);
    assert_eq!(item["partial"], true);
    assert_eq!(item["hasAssetsOrPositions"], false);
    assert_eq!(item["errors"].as_array().unwrap().len(), 3);
    let warnings = overview["warnings"].as_array().unwrap();
    assert_eq!(warnings.len(), 3);
    for capability in ["funds", "positions", "orders"] {
        assert!(
            warnings
                .iter()
                .any(|warning| warning.as_str().unwrap().contains(capability)),
            "warnings = {warnings:?}"
        );
    }

    let positions = executor
        .execute(
            "portfolio.positions",
            &json!({"tradingEnvironment": "REAL"}),
        )
        .expect("portfolio.positions with failing reads");
    assert_eq!(positions["partial"], true);
    assert_eq!(
        positions["accountPositions"][0]["errors"]
            .as_array()
            .unwrap()
            .len(),
        1
    );
    assert_eq!(positions["accountPositions"][0]["positions"], json!([]));
    assert_eq!(positions["warnings"].as_array().unwrap().len(), 1);
}

/// Parity: go:452dea11:internal/assistant/assembly/portfolio_tools_test.go:255
/// `TestPortfolioAccountResolutionSupportsExactSuffixAndIsolation`.
#[tokio::test]
async fn portfolio_account_resolution_matches_the_go_exact_suffix_and_isolation_matrix() {
    use crate::product::product_adk_model_runtime::AdkToolExecutor;

    let reader = Arc::new(AdkPortfolioFixtureRead::new(vec![
        adk_fixture_account(8240, 1, vec![2]),
        adk_fixture_account(18240, 1, vec![1]),
        adk_fixture_account(7281, 1, vec![2]),
        adk_fixture_account(17281, 0, vec![2]),
    ]));
    let (_directory, ports) = portfolio_fixture_ports(
        Arc::clone(&reader) as Arc<dyn jftrade_integration_futu::TradeReadPort>,
        Arc::new(AdkPortfolioFixtureOrders::default()),
    );
    let executor = portfolio_fixture_executor(&ports);

    let cases: [(&str, Value, &str, &str, &[&str]); 6] = [
        (
            "exact wins",
            json!({"tradingEnvironment": "REAL", "accountId": "8240"}),
            "resolved",
            "exact",
            &["8240"],
        ),
        (
            "unique suffix",
            json!({"tradingEnvironment": "REAL", "accountId": "281"}),
            "resolved",
            "unique_suffix",
            &["7281"],
        ),
        (
            "suffix conflict",
            json!({"tradingEnvironment": "REAL", "accountId": "240"}),
            "ambiguous",
            "suffix",
            &[],
        ),
        (
            "missing",
            json!({"tradingEnvironment": "REAL", "accountId": "9999"}),
            "not_found",
            "account_id",
            &[],
        ),
        (
            "environment isolation",
            json!({"tradingEnvironment": "SIMULATE", "accountId": "281"}),
            "resolved",
            "unique_suffix",
            &["17281"],
        ),
        (
            "market isolation",
            json!({"tradingEnvironment": "REAL", "market": "HK", "accountId": "240"}),
            "resolved",
            "unique_suffix",
            &["18240"],
        ),
    ];
    for (name, arguments, status, mode, selected) in cases {
        let payload = executor
            .execute("portfolio.accounts", &arguments)
            .expect("portfolio.accounts resolution");
        assert_eq!(payload["selection"]["status"], status, "{name}");
        assert_eq!(payload["selection"]["mode"], mode, "{name}");
        let selected_ids = payload["selection"]["selectedAccountIds"]
            .as_array()
            .unwrap()
            .iter()
            .map(|value| value.as_str().unwrap().to_owned())
            .collect::<Vec<_>>();
        assert_eq!(selected_ids, selected, "{name}");
        if name == "suffix conflict" {
            assert_eq!(
                payload["selection"]["candidateAccounts"]
                    .as_array()
                    .unwrap()
                    .len(),
                2,
                "{name}"
            );
        }
    }

    let unfiltered = executor
        .execute("portfolio.accounts", &json!({"tradingEnvironment": "REAL"}))
        .expect("portfolio.accounts all matching");
    assert_eq!(unfiltered["selection"]["status"], "resolved");
    assert_eq!(unfiltered["selection"]["mode"], "all_matching_accounts");
    assert_eq!(
        unfiltered["selection"]["selectedAccountIds"]
            .as_array()
            .unwrap()
            .len(),
        3
    );

    let empty = executor
        .execute(
            "portfolio.accounts",
            &json!({"tradingEnvironment": "REAL", "market": "CN"}),
        )
        .expect("portfolio.accounts without a matching market");
    assert_eq!(empty["selection"]["status"], "not_found");
    assert_eq!(empty["selection"]["mode"], "all_matching_accounts");
}

/// Parity: go:452dea11:internal/assistant/assembly/portfolio_tools_test.go:338
/// `TestAccountOrdersFiltersAccountEnvironmentMarketAndActiveStatus`.
///
/// The suffix `240` resolves to account `8240` before the execution filter is
/// built, and the tool reports the resolution through `selection` instead of
/// forwarding an account id the store would treat as an exact miss.
#[tokio::test]
async fn account_orders_resolves_requested_account_suffixes_through_discovery() {
    use crate::product::product_adk_model_runtime::AdkToolExecutor;

    let reader = Arc::new(AdkPortfolioFixtureRead::new(vec![
        adk_fixture_account(8240, 1, vec![2]),
        adk_fixture_account(9999, 1, vec![2]),
    ]));
    let orders = Arc::new(AdkPortfolioFixtureOrders::default());
    let (_directory, ports) = portfolio_fixture_ports(
        Arc::clone(&reader) as Arc<dyn jftrade_integration_futu::TradeReadPort>,
        Arc::clone(&orders),
    );
    let executor = portfolio_fixture_executor(&ports);

    let resolved = executor
        .execute(
            "account.orders",
            &json!({
                "accountId": "240",
                "tradingEnvironment": "REAL",
                "market": "US",
                "activeOnly": true,
            }),
        )
        .expect("account.orders by suffix");
    assert_eq!(resolved["count"], 1);
    assert_eq!(resolved["activeOnly"], true);
    assert_eq!(resolved["partial"], false);
    assert_eq!(resolved["selection"]["mode"], "unique_suffix");
    assert_eq!(resolved["selection"]["selectedAccountIds"][0], "8240");
    assert_eq!(resolved["discoveredAccounts"].as_array().unwrap().len(), 2);
    assert_eq!(
        orders.recorded(),
        ["scope=ACTIVE&tradingEnvironment=REAL&accountId=8240&market=US"]
    );

    let unresolved = executor
        .execute(
            "account.orders",
            &json!({"accountId": "0000", "tradingEnvironment": "REAL"}),
        )
        .expect("account.orders without a matching account");
    assert_eq!(unresolved["orders"], json!([]));
    assert_eq!(unresolved["count"], 0);
    assert_eq!(unresolved["partial"], true);
    assert_eq!(unresolved["selection"]["status"], "not_found");
    assert_eq!(unresolved["selection"]["mode"], "account_id");
    assert_eq!(unresolved["warnings"].as_array().unwrap().len(), 1);
    assert_eq!(
        orders.recorded().len(),
        1,
        "an unresolved account must not reach the execution store"
    );

    let unfiltered = executor
        .execute("account.orders", &json!({"tradingEnvironment": "REAL"}))
        .expect("account.orders without an account filter");
    assert_eq!(unfiltered["selection"]["mode"], "all_matching_orders");
    assert_eq!(unfiltered["partial"], false);
    assert_eq!(unfiltered["discoveredAccounts"], json!([]));
    assert_eq!(
        orders.recorded().last().unwrap(),
        "scope=CURRENT&tradingEnvironment=REAL"
    );

    let failing_discovery = Arc::new(
        AdkPortfolioFixtureRead::new(Vec::new())
            .with_accounts_error("OpenD account discovery timed out"),
    );
    let failing_orders = Arc::new(AdkPortfolioFixtureOrders::default());
    let (_directory, ports) = portfolio_fixture_ports(
        Arc::clone(&failing_discovery) as Arc<dyn jftrade_integration_futu::TradeReadPort>,
        Arc::clone(&failing_orders),
    );
    let executor = portfolio_fixture_executor(&ports);
    let failed = executor
        .execute(
            "account.orders",
            &json!({"accountId": "240", "tradingEnvironment": "REAL"}),
        )
        .expect("account.orders with failing discovery");
    assert_eq!(failed["orders"], json!([]));
    assert_eq!(failed["partial"], true);
    assert_eq!(failed["selection"]["status"], "discovery_failed");
    assert!(
        failed["warnings"][0]
            .as_str()
            .unwrap()
            .contains("discovery timed out")
    );
    assert!(failing_orders.recorded().is_empty());
}

/// Parity: go:452dea11:internal/assistant/assembly/portfolio_tools_test.go:444
/// `TestApplicationPortfolioMarksIncompleteBrokerResponsesPartial`.
///
/// Rust reads funds and positions through `Result`-returning ports, so an
/// incomplete broker response is an error rather than a nil payload; the
/// projection still has to mark the item partial and must not classify
/// zero-value funds as assets. The currency-balance and market-asset branches
/// mirror Go's `brokerFundsHaveAssets` matrix.
#[tokio::test]
async fn portfolio_overview_reports_partial_reads_without_classifying_empty_funds_as_assets() {
    use crate::product::product_adk_model_runtime::AdkToolExecutor;

    async fn overview_for(funds: jftrade_integration_futu::TradeFunds) -> Value {
        let reader = Arc::new(
            AdkPortfolioFixtureRead::new(vec![adk_fixture_account(8240, 1, vec![2])])
                .with_funds(8240, funds),
        );
        let (_directory, ports) = portfolio_fixture_ports(
            Arc::clone(&reader) as Arc<dyn jftrade_integration_futu::TradeReadPort>,
            Arc::new(AdkPortfolioFixtureOrders::default()),
        );
        let executor = portfolio_fixture_executor(&ports);
        executor
            .execute("portfolio.overview", &json!({"tradingEnvironment": "REAL"}))
            .expect("portfolio.overview")
    }

    let zero = overview_for(adk_fixture_cash_funds(0.0)).await;
    assert_eq!(zero["accountOverviews"][0]["hasAssetsOrPositions"], false);
    assert_eq!(zero["partial"], false);
    assert_eq!(zero["warnings"], json!([]));

    let currency_balance = overview_for(adk_fixture_currency_balance_funds(10.0)).await;
    assert_eq!(
        currency_balance["accountOverviews"][0]["hasAssetsOrPositions"], true,
        "a currency balance is an asset"
    );

    let market_asset = overview_for(adk_fixture_market_asset_funds(20.0)).await;
    assert_eq!(
        market_asset["accountOverviews"][0]["hasAssetsOrPositions"], true,
        "a market asset is an asset"
    );

    let partial_reader = Arc::new(
        AdkPortfolioFixtureRead::new(vec![adk_fixture_account(8240, 1, vec![2])])
            .with_positions(8240, &["US.AAPL"])
            .failing_funds(8240),
    );
    let (_directory, ports) = portfolio_fixture_ports(
        Arc::clone(&partial_reader) as Arc<dyn jftrade_integration_futu::TradeReadPort>,
        Arc::new(AdkPortfolioFixtureOrders::default()),
    );
    let executor = portfolio_fixture_executor(&ports);
    let payload = executor
        .execute("portfolio.overview", &json!({"tradingEnvironment": "REAL"}))
        .expect("portfolio.overview with an incomplete funds response");
    let item = &payload["accountOverviews"][0];
    assert_eq!(payload["partial"], true);
    assert_eq!(item["partial"], true);
    assert_eq!(item["errors"].as_array().unwrap().len(), 1);
    assert_eq!(item["hasAssetsOrPositions"], true);
    assert_eq!(item["positionCount"], 1);
    assert_eq!(payload["warnings"].as_array().unwrap().len(), 1);
}
