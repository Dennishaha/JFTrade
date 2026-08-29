use super::*;
use jftrade_api::LiveHub;
use jftrade_integration_futu::{
    ResponseError,
    TradeAccountSnapshot, TradeCashFlowSnapshot, TradeFillSnapshot, TradeFunds,
    TradeFundsSnapshot, TradeMarginRatioSnapshot, TradeMaxTradeQuantityRequest,
    TradeMaxTradeQuantitySnapshot, TradeOrderFeeSnapshot, TradeOrderSnapshot,
    TradePositionSnapshot, TradeSessionError,
};
use jftrade_marketdata::ProviderRouter;
use jftrade_settings::FutuIntegrationConfig;
use std::time::{Duration, Instant};

#[derive(Debug)]
struct FakeTradeRead;

impl TradeReadPort for FakeTradeRead {
    fn read_accounts(&self, _: u64, _: Option<i32>, _: Option<bool>) -> Result<Vec<TradeAccountSnapshot>, TradeSessionError> {
        Ok(vec![TradeAccountSnapshot {
            trd_env: 1,
            acc_id: 42,
            trd_market_auth_list: vec![1, 2],
            acc_type: Some(2),
            card_num: None,
            security_firm: Some(1),
            sim_acc_type: None,
            uni_card_num: None,
            acc_status: Some(0),
            acc_role: Some(1),
            jp_acc_type: Vec::new(),
            competition_acc_name: None,
        }])
    }

    fn read_funds(&self, header: TradeHeader, _: Option<bool>, _: Option<i32>, _: Option<i32>) -> Result<TradeFundsSnapshot, TradeSessionError> {
        Ok(TradeFundsSnapshot { header, funds: TradeFunds {
            power: 1.0, total_assets: 2.0, cash: 3.0, market_val: 4.0, frozen_cash: 0.0,
            debt_cash: 0.0, avl_withdrawal_cash: 3.0, currency: Some(1), available_funds: None,
            unrealized_pl: None, realized_pl: None, risk_level: None, initial_margin: None,
            maintenance_margin: None, cash_info_list: Vec::new(), max_power_short: None,
            net_cash_power: None, long_mv: None, short_mv: None, pending_asset: None,
            max_withdrawal: None, risk_status: None, margin_call_margin: None, is_pdt: None,
            pdt_seq: None, beginning_dtbp: None, remaining_dtbp: None, dt_call_amount: None,
            dt_status: None, securities_assets: None, fund_assets: None, bond_assets: None,
            market_info_list: Vec::new(), crypto_mv: None, exposure_level: None,
            exposure_limit: None, used_limit: None, remaining_limit: None,
        } })
    }

    fn read_cash_flows(&self, header: TradeHeader, _: String, _: Option<i32>) -> Result<Vec<TradeCashFlowSnapshot>, TradeSessionError> {
        Ok(vec![TradeCashFlowSnapshot {
            header,
            clearing_date: Some("2026-08-21".to_owned()),
            settlement_date: Some("2026-08-22".to_owned()),
            currency: Some(2),
            cash_flow_type: Some("DIVIDEND".to_owned()),
            cash_flow_direction: Some(1),
            cash_flow_amount: Some(12.5),
            cash_flow_remark: Some("fixture".to_owned()),
            cash_flow_id: Some(9),
            create_time: None,
        }])
    }

    fn read_order_fees(&self, header: TradeHeader, _: Vec<String>) -> Result<Vec<TradeOrderFeeSnapshot>, TradeSessionError> {
        Ok(vec![TradeOrderFeeSnapshot {
            header,
            broker_order_id_ex: "fee-2".to_owned(),
            fee_amount: Some(1.5),
            fee_items: vec![jftrade_integration_futu::TradeOrderFeeItemSnapshot {
                title: "commission".to_owned(), value: 1.5,
            }],
        }])
    }

    fn read_margin_ratios(&self, header: TradeHeader, _: Vec<TradeSecurity>) -> Result<Vec<TradeMarginRatioSnapshot>, TradeSessionError> {
        Ok(vec![TradeMarginRatioSnapshot {
            header,
            market: "US".to_owned(),
            symbol: "US.AAPL".to_owned(),
            is_long_permit: Some(true),
            is_short_permit: Some(false),
            short_pool_remain: Some(100.0),
            short_fee_rate: Some(0.02),
            alert_long_ratio: Some(0.5),
            alert_short_ratio: None,
            initial_margin_long_ratio: Some(0.3),
            initial_margin_short_ratio: None,
            margin_call_long_ratio: None,
            margin_call_short_ratio: None,
            maintenance_long_ratio: None,
            maintenance_short_ratio: Some(0.4),
        }])
    }

    fn read_max_trade_quantity(&self, request: TradeMaxTradeQuantityRequest) -> Result<TradeMaxTradeQuantitySnapshot, TradeSessionError> {
        Ok(TradeMaxTradeQuantitySnapshot {
            header: request.header,
            code: request.code,
            order_type: request.order_type,
            price: request.price,
            max_cash_buy: 0.0,
            max_cash_and_margin_buy: None,
            max_position_sell: 0.0,
            max_sell_short: None,
            max_buy_back: None,
            long_required_im: None,
            short_required_im: None,
            session: None,
        })
    }

    fn read_positions(&self, _: TradeHeader, _: Option<TradeFilter>, _: Option<f64>, _: Option<f64>, _: Option<bool>, _: Option<i32>, _: Option<i32>, _: Option<bool>) -> Result<Vec<TradePositionSnapshot>, TradeSessionError> { Ok(Vec::new()) }
    fn read_orders(&self, _: TradeHeader, _: Option<TradeFilter>, _: Vec<i32>, _: Option<bool>) -> Result<Vec<TradeOrderSnapshot>, TradeSessionError> { Ok(Vec::new()) }
    fn read_fills(&self, _: TradeHeader, _: Option<TradeFilter>, _: Option<bool>) -> Result<Vec<TradeFillSnapshot>, TradeSessionError> { Ok(Vec::new()) }
}

#[derive(Debug)]
struct ErrorTradeRead {
    message: &'static str,
}

impl ErrorTradeRead {
    fn error(&self) -> TradeSessionError {
        TradeSessionError::Response(ResponseError::ReturnCode {
            ret_type: -1,
            err_code: 429,
            message: self.message.to_owned(),
        })
    }
}

impl TradeReadPort for ErrorTradeRead {
    fn read_accounts(&self, user_id: u64, category: Option<i32>, general: Option<bool>) -> Result<Vec<TradeAccountSnapshot>, TradeSessionError> {
        FakeTradeRead.read_accounts(user_id, category, general)
    }
    fn read_funds(&self, _: TradeHeader, _: Option<bool>, _: Option<i32>, _: Option<i32>) -> Result<TradeFundsSnapshot, TradeSessionError> { Err(self.error()) }
    fn read_cash_flows(&self, _: TradeHeader, _: String, _: Option<i32>) -> Result<Vec<TradeCashFlowSnapshot>, TradeSessionError> { Err(self.error()) }
    fn read_order_fees(&self, _: TradeHeader, _: Vec<String>) -> Result<Vec<TradeOrderFeeSnapshot>, TradeSessionError> { Err(self.error()) }
    fn read_margin_ratios(&self, _: TradeHeader, _: Vec<TradeSecurity>) -> Result<Vec<TradeMarginRatioSnapshot>, TradeSessionError> { Err(self.error()) }
    fn read_max_trade_quantity(&self, _: TradeMaxTradeQuantityRequest) -> Result<TradeMaxTradeQuantitySnapshot, TradeSessionError> { Err(self.error()) }
    fn read_positions(&self, _: TradeHeader, _: Option<TradeFilter>, _: Option<f64>, _: Option<f64>, _: Option<bool>, _: Option<i32>, _: Option<i32>, _: Option<bool>) -> Result<Vec<TradePositionSnapshot>, TradeSessionError> { Err(self.error()) }
    fn read_orders(&self, _: TradeHeader, _: Option<TradeFilter>, _: Vec<i32>, _: Option<bool>) -> Result<Vec<TradeOrderSnapshot>, TradeSessionError> { Err(self.error()) }
    fn read_fills(&self, _: TradeHeader, _: Option<TradeFilter>, _: Option<bool>) -> Result<Vec<TradeFillSnapshot>, TradeSessionError> { Err(self.error()) }
}

fn ready_state() -> Arc<ActiveProviderState> {
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    state.set_readiness(false, true, true);
    state
}

#[test]
fn broker_read_fails_closed_without_trade_client() {
    let port = ProductionBrokerPort { active_provider_state: ready_state(), trade_read_port: None, trade_logged_in: Some(true), trade_runtime: None };
    let error = port.read("/api/v1/brokers/futu/funds", "accountId=42&market=US").expect_err("missing client");
    assert!(error.to_string().contains("trade read client"));
}

#[test]
fn broker_read_projects_futu_funds_from_neutral_client() {
    let port = ProductionBrokerPort { active_provider_state: ready_state(), trade_read_port: Some(Arc::new(FakeTradeRead)), trade_logged_in: Some(true), trade_runtime: None };
    let value = port.read("/api/v1/brokers/futu/funds", "accountId=42&market=US").expect("funds");
    assert_eq!(value["summary"]["totalAssets"], 2.0);
    assert_eq!(value["connectivity"], "connected");
}

#[test]
fn broker_read_projects_cash_flows_with_baseline_fields_and_sorting() {
    let port = ProductionBrokerPort { active_provider_state: ready_state(), trade_read_port: Some(Arc::new(FakeTradeRead)), trade_logged_in: Some(true), trade_runtime: None };
    let value = port
        .read("/api/v1/brokers/futu/cash-flows", "accountId=42&market=US&clearingDate=2026-08-21&direction=IN")
        .expect("cash flows");
    assert_eq!(value["connectivity"], "connected");
    assert_eq!(value["cashFlows"][0]["cashFlowId"], "9");
    assert_eq!(value["cashFlows"][0]["cashFlowDirection"], "IN");
    assert_eq!(value["cashFlows"][0]["cashFlowAmount"], 12.5);
}

#[test]
fn cash_flows_require_clearing_date() {
    let port = ProductionBrokerPort { active_provider_state: ready_state(), trade_read_port: Some(Arc::new(FakeTradeRead)), trade_logged_in: Some(true), trade_runtime: None };
    let error = port
        .read("/api/v1/brokers/futu/cash-flows", "accountId=42&market=US")
        .expect_err("missing clearing date");
    assert!(matches!(error, BrokerReadSnapshotError::Invalid(message) if message.contains("clearingDate")));
}

#[test]
fn broker_read_projects_order_fees_and_merges_order_id_queries() {
    let port = ProductionBrokerPort { active_provider_state: ready_state(), trade_read_port: Some(Arc::new(FakeTradeRead)), trade_logged_in: Some(true), trade_runtime: None };
    let value = port
        .read(
            "/api/v1/brokers/futu/order-fees",
            "accountId=42&market=US&orderIdEx=fee-1&orderIdEx=FEE-1&orderIdExList=fee-2,fee-1",
        )
        .expect("order fees");
    assert_eq!(value["connectivity"], "connected");
    assert_eq!(value["fees"][0]["brokerOrderIdEx"], "fee-2");
    assert_eq!(value["fees"][0]["feeAmount"], 1.5);
    assert_eq!(value["fees"][0]["feeItems"][0]["title"], "commission");
}

#[test]
fn order_fees_require_at_least_one_non_empty_order_id() {
    let port = ProductionBrokerPort { active_provider_state: ready_state(), trade_read_port: Some(Arc::new(FakeTradeRead)), trade_logged_in: Some(true), trade_runtime: None };
    let error = port
        .read("/api/v1/brokers/futu/order-fees", "accountId=42&market=US&orderIdEx=,")
        .expect_err("missing order id");
    assert!(matches!(error, BrokerReadSnapshotError::Invalid(message) if message.contains("orderIdEx")));
}

#[test]
fn broker_read_projects_margin_ratios_with_real_environment_and_omits_absent_values() {
    let port = ProductionBrokerPort { active_provider_state: ready_state(), trade_read_port: Some(Arc::new(FakeTradeRead)), trade_logged_in: Some(true), trade_runtime: None };
    let value = port
        .read("/api/v1/brokers/futu/margin-ratios", "accountId=42&market=US&symbol=US.AAPL")
        .expect("margin ratios");
    assert_eq!(value["connectivity"], "connected");
    assert_eq!(value["marginRatios"][0]["tradingEnvironment"], "REAL");
    assert_eq!(value["marginRatios"][0]["symbol"], "US.AAPL");
    assert_eq!(value["marginRatios"][0]["shortFeeRate"], 0.02);
    assert!(value["marginRatios"][0].get("alertShortRatio").is_none());
}

#[test]
fn portfolio_cash_balances_fall_back_to_summary_currency_when_breakdown_is_empty() {
    let funds = FakeTradeRead
        .read_funds(trade_header(1, 42, 2), None, None, None)
        .expect("funds")
        .funds;
    let resolved = ResolvedTradeRequest {
        account_id: "42".to_owned(),
        environment: "REAL".to_owned(),
        market: "US".to_owned(),
        header: trade_header(1, 42, 2),
    };
    let balances = portfolio_cash_balance_values("futu", &resolved, &funds);
    assert_eq!(balances.len(), 1);
    assert_eq!(balances[0]["brokerId"], "futu");
    assert_eq!(balances[0]["currency"], "HKD");
    assert_eq!(balances[0]["cashBalance"], 3.0);
    assert!(balances[0]["updatedAt"].as_str().is_some());
}

#[test]
fn margin_ratios_require_symbols() {
    let port = ProductionBrokerPort { active_provider_state: ready_state(), trade_read_port: Some(Arc::new(FakeTradeRead)), trade_logged_in: Some(true), trade_runtime: None };
    let error = port
        .read("/api/v1/brokers/futu/margin-ratios", "accountId=42&market=US")
        .expect_err("missing symbol");
    assert!(matches!(error, BrokerReadSnapshotError::Invalid(message) if message.contains("symbol")));
}

#[test]
fn broker_read_projects_max_trade_quantity_snapshot() {
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: Some(Arc::new(FakeTradeRead)),
        trade_logged_in: Some(true),
        trade_runtime: None,
    };
    let value = port
        .read(
            "/api/v1/brokers/futu/max-trade-qtys",
            "accountId=42&market=US&symbol=US.AAPL&orderType=LIMIT&price=100",
        )
        .expect("max trade quantity");
    assert_eq!(value["connectivity"], "connected");
    assert_eq!(value["maxTradeQuantity"]["symbol"], "US.AAPL");
    assert_eq!(value["maxTradeQuantity"]["orderType"], "LIMIT");
    assert_eq!(value["maxTradeQuantity"]["price"], 100.0);
}

#[test]
fn max_trade_quantity_rejects_invalid_inputs() {
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: Some(Arc::new(FakeTradeRead)),
        trade_logged_in: Some(true),
        trade_runtime: None,
    };
    for query in [
        "accountId=42&market=US&orderType=LIMIT&price=100",
        "accountId=42&market=US&symbol=US.AAPL&price=100",
        "accountId=42&market=US&symbol=US.AAPL&orderType=LIMIT&price=0",
        "accountId=42&market=US&symbol=US.AAPL&orderType=TRAILING&price=100",
    ] {
        assert!(matches!(
            port.read("/api/v1/brokers/futu/max-trade-qtys", query),
            Err(BrokerReadSnapshotError::Invalid(_))
        ));
    }
}

#[test]
fn margin_ratios_reject_symbol_with_conflicting_market() {
    let port = ProductionBrokerPort { active_provider_state: ready_state(), trade_read_port: Some(Arc::new(FakeTradeRead)), trade_logged_in: Some(true), trade_runtime: None };
    let error = port
        .read("/api/v1/brokers/futu/margin-ratios", "accountId=42&market=US&symbol=HK.00700")
        .expect_err("conflicting market");
    assert!(matches!(error, BrokerReadSnapshotError::Invalid(message) if message.contains("market")));
}

#[test]
fn margin_ratios_use_recent_cache_only_for_rate_limit_errors() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(FakeTradeRead)), Some(true));
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(Arc::clone(&runtime)),
    };
    let query = "accountId=42&market=US&symbol=US.AAPL";
    let initial = port
        .read("/api/v1/brokers/futu/margin-ratios", query)
        .expect("initial margin-ratio read");
    assert_eq!(initial["marginRatios"][0]["symbol"], "US.AAPL");

    runtime.set(
        Some(Arc::new(ErrorTradeRead {
            message: "rate limit exceeded",
        })),
        Some(true),
    );
    let fallback = port
        .read(
            "/api/v1/brokers/futu/margin-ratios",
            "accountId=42&market=US&symbol=US.AAPL",
        )
        .expect("recent cache fallback");
    assert_eq!(fallback["marginRatios"][0]["symbol"], "US.AAPL");

    runtime.margin_ratio_cache.put_at(
        "42|REAL|US|US.AAPL".to_owned(),
        vec![TradeMarginRatioSnapshot {
            header: TradeHeader {
                trd_env: 1,
                acc_id: 42,
                trd_market: 2,
                jp_acc_type: None,
            },
            market: "US".to_owned(),
            symbol: "US.AAPL".to_owned(),
            is_long_permit: None,
            is_short_permit: None,
            short_pool_remain: None,
            short_fee_rate: None,
            alert_long_ratio: None,
            alert_short_ratio: None,
            initial_margin_long_ratio: None,
            initial_margin_short_ratio: None,
            margin_call_long_ratio: None,
            margin_call_short_ratio: None,
            maintenance_long_ratio: None,
            maintenance_short_ratio: None,
        }],
        Instant::now() - Duration::from_secs(121),
    );
    let expired = port.read("/api/v1/brokers/futu/margin-ratios", query);
    assert!(matches!(expired, Err(BrokerReadSnapshotError::Unavailable(message)) if message.contains("rate limit")));

    runtime.set(
        Some(Arc::new(ErrorTradeRead {
            message: "broker service unavailable",
        })),
        Some(true),
    );
    let non_rate = port.read("/api/v1/brokers/futu/margin-ratios", query);
    assert!(matches!(non_rate, Err(BrokerReadSnapshotError::Unavailable(message)) if message.contains("broker service unavailable")));
}

#[test]
fn trade_header_uses_futu_trade_enums_not_quote_codes() {
    let request = TradeRequest::parse(
        "/api/v1/brokers/futu/funds",
        "accountId=42&tradingEnvironment=REAL&market=US",
    )
    .expect("request");
    let header = request.header().expect("header");
    assert_eq!(header.trd_env, 1);
    assert_eq!(header.trd_market, 2);
}

#[test]
fn account_projection_matches_broker_runtime_contract() {
    let value = account_value(TradeAccountSnapshot {
        trd_env: 0,
        acc_id: 42,
        trd_market_auth_list: vec![1, 2, 10, 17, 31],
        acc_type: Some(2),
        card_num: Some("ignored-card".to_owned()),
        security_firm: Some(1),
        sim_acc_type: Some(4),
        uni_card_num: None,
        acc_status: Some(0),
        acc_role: Some(1),
        jp_acc_type: Vec::new(),
        competition_acc_name: None,
    });
    assert_eq!(value["accountId"], "42");
    assert_eq!(value["tradingEnvironment"], "SIMULATE");
    assert_eq!(value["accountType"], "MARGIN");
    assert_eq!(value["securityFirm"], "FUTUSECURITIES");
    assert_eq!(value["simulatedAccountType"], "STOCKANDOPTION");
    assert_eq!(value["marketAuthorities"], json!(["HK", "US"]));
    assert!(value.get("tradingMarketAuth").is_none());
    assert!(value.get("cardNumber").is_none());
    assert_eq!(trade_market_authority(12), Some("SG"));
    assert_eq!(trade_market_authority(13), Some("JP"));
    assert_eq!(trade_market_authority(31), None);
}

#[test]
fn broker_runtime_requires_real_projection_sources() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(FakeTradeRead)), Some(true));
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(runtime),
    };
    let error = port
        .read("/api/v1/brokers/futu/runtime", "")
        .expect_err("runtime projection must not use fixture values");
    assert!(matches!(error, BrokerReadSnapshotError::Unavailable(message) if message.contains("projection") || message.contains("connection settings")));
}

#[test]
fn broker_runtime_projects_configured_connection_and_live_hub() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(FakeTradeRead)), Some(true));
    let hub = Arc::new(LiveHub::default());
    let connection = hub.connect();
    let mut config = FutuIntegrationConfig::current_default();
    config.host = "10.0.0.8".to_owned();
    config.api_port = 21_110;
    config.websocket_port = 21_111;
    config.use_encryption = true;
    runtime.set_runtime_projection(&config, Some(Arc::clone(&hub)), 7);
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(runtime),
    };
    let value = port
        .read("/api/v1/brokers/futu/runtime", "")
        .expect("runtime projection");
    assert_eq!(value["session"]["tradeLoggedIn"], true);
    assert_eq!(value["session"]["connection"]["host"], "10.0.0.8");
    assert_eq!(value["session"]["connection"]["apiPort"], 21_110);
    assert_eq!(value["session"]["connection"]["websocketPort"], 21_111);
    assert_eq!(value["session"]["connection"]["port"], 21_110);
    assert_eq!(value["session"]["connection"]["useEncryption"], true);
    assert_eq!(value["session"]["connection"]["marketDataTransport"], "bbgo-opend-tcp-api");
    assert_eq!(value["session"]["liveWebSocketClients"]["connected"], 1);
    assert_eq!(value["session"]["liveWebSocketClients"]["limit"], 7);
    assert_eq!(value["session"]["liveWebSocketClients"]["atLimit"], false);
    drop(connection);
}

#[test]
fn broker_securities_projects_real_futu_tick_cache() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(FakeTradeRead)), Some(true));
    let router = Arc::new(std::sync::Mutex::new(ProviderRouter::new(8)));
    router
        .lock()
        .expect("router lock")
        .cache_mut()
        .insert(
            jftrade_marketdata::Tick {
                instrument_id: "US.AAPL".to_owned(),
                price: jftrade_kernel::Fixed8::from_scaled(12_345_000_000),
                volume: "1000".parse().expect("decimal volume"),
                observed_at_ms: 1_700_000_000_000,
                provider_generation: 0,
            },
            0,
        )
        .expect("insert tick");
    runtime.set_market_data_router(Some(router));
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(runtime),
    };
    let value = port
        .read(
            "/api/v1/brokers/futu/securities",
            "symbol=US.AAPL&symbols=US.MSFT,US.AAPL",
        )
        .expect("securities snapshot");
    assert_eq!(value["connectivity"], "connected");
    assert_eq!(value["securities"]["snapshots"].as_array().unwrap().len(), 1);
    assert_eq!(value["securities"]["snapshots"][0]["symbol"], "US.AAPL");
    assert_eq!(value["securities"]["snapshots"][0]["lastPrice"], 123.45);
    assert_eq!(value["securities"]["snapshots"][0]["volume"], 1000);
}

#[test]
fn broker_securities_returns_real_empty_result_when_cache_has_no_symbol() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(FakeTradeRead)), Some(true));
    runtime.set_market_data_router(Some(Arc::new(std::sync::Mutex::new(
        ProviderRouter::new(8),
    ))));
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(runtime),
    };
    let value = port
        .read("/api/v1/brokers/futu/securities", "symbol=US.MSFT")
        .expect("empty securities snapshot");
    assert_eq!(value["securities"]["snapshots"], json!([]));
}

#[test]
fn broker_securities_fails_closed_without_market_data_router() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(FakeTradeRead)), Some(true));
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(runtime),
    };
    let error = port
        .read("/api/v1/brokers/futu/securities", "symbol=US.AAPL")
        .expect_err("missing router");
    assert!(matches!(error, BrokerReadSnapshotError::Unavailable(message) if message.contains("market-data runtime") || message.contains("router")));
}

#[test]
fn broker_securities_requires_symbol_query() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(FakeTradeRead)), Some(true));
    runtime.set_market_data_router(Some(Arc::new(std::sync::Mutex::new(
        ProviderRouter::new(8),
    ))));
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(runtime),
    };
    let error = port
        .read("/api/v1/brokers/futu/securities", "")
        .expect_err("missing symbol");
    assert!(matches!(error, BrokerReadSnapshotError::Invalid(message) if message.contains("symbol")));
}

#[test]
fn broker_quote_projects_real_futu_tick_cache_for_all_symbols() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(FakeTradeRead)), Some(true));
    let router = Arc::new(std::sync::Mutex::new(ProviderRouter::new(8)));
    router
        .lock()
        .expect("router lock")
        .cache_mut()
        .insert(
            jftrade_marketdata::Tick {
                instrument_id: "US.AAPL".to_owned(),
                price: jftrade_kernel::Fixed8::from_scaled(12_345_000_000),
                volume: "1000".parse().expect("decimal volume"),
                observed_at_ms: 1_700_000_000_000,
                provider_generation: 0,
            },
            0,
        )
        .expect("insert AAPL");
    router
        .lock()
        .expect("router lock")
        .cache_mut()
        .insert(
            jftrade_marketdata::Tick {
                instrument_id: "US.MSFT".to_owned(),
                price: jftrade_kernel::Fixed8::from_scaled(20_000_000_000),
                volume: "2000".parse().expect("decimal volume"),
                observed_at_ms: 1_700_000_000_100,
                provider_generation: 0,
            },
            0,
        )
        .expect("insert MSFT");
    runtime.set_market_data_router(Some(router));
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(runtime),
    };
    let value = port
        .read(
            "/api/v1/brokers/futu/quote",
            "accountId=42&symbol=US.AAPL&symbols=US.MSFT",
        )
        .expect("quote snapshot");
    assert_eq!(value["quote"]["accountId"], "42");
    assert_eq!(value["quote"]["symbol"], "US.AAPL");
    assert_eq!(value["quote"]["lastPrice"], 123.45);
    assert_eq!(value["quote"]["volume"], 1000);
    assert_eq!(value["quote"]["quotes"].as_array().unwrap().len(), 2);
    assert_eq!(value["quote"]["quotes"][1]["symbol"], "US.MSFT");
}

#[test]
fn broker_quote_requires_every_requested_symbol_in_real_cache() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(FakeTradeRead)), Some(true));
    runtime.set_market_data_router(Some(Arc::new(std::sync::Mutex::new(
        ProviderRouter::new(8),
    ))));
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(runtime),
    };
    let error = port
        .read(
            "/api/v1/brokers/futu/quote",
            "symbol=US.AAPL&symbols=US.MSFT",
        )
        .expect_err("missing cached quote");
    assert!(matches!(error, BrokerReadSnapshotError::Unavailable(message) if message.contains("US.AAPL")));
}

#[test]
fn broker_quote_fails_closed_without_market_data_runtime() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(FakeTradeRead)), Some(true));
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(runtime),
    };
    let error = port
        .read("/api/v1/brokers/futu/quote", "symbol=US.AAPL")
        .expect_err("missing market-data router");
    assert!(matches!(error, BrokerReadSnapshotError::Unavailable(message) if message.contains("runtime") || message.contains("router")));
}

#[test]
fn broker_quote_requires_symbol_query() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(FakeTradeRead)), Some(true));
    runtime.set_market_data_router(Some(Arc::new(std::sync::Mutex::new(
        ProviderRouter::new(8),
    ))));
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(runtime),
    };
    let error = port
        .read("/api/v1/brokers/futu/quote", "")
        .expect_err("missing symbol");
    assert!(matches!(error, BrokerReadSnapshotError::Invalid(message) if message.contains("symbol")));
}

#[test]
fn broker_klines_valid_request_fails_closed_without_historical_source() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(FakeTradeRead)), Some(true));
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(runtime),
    };
    let error = port
        .read(
            "/api/v1/brokers/futu/klines",
            "symbol=US.AAPL&period=1d&limit=10",
        )
        .expect_err("historical source is not wired");
    assert!(matches!(error, BrokerReadSnapshotError::Unavailable(message) if message.contains("historical klines")));
}

#[test]
fn broker_klines_rejects_invalid_period_and_time_combinations() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(FakeTradeRead)), Some(true));
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(runtime),
    };
    let invalid_period = port
        .read(
            "/api/v1/brokers/futu/klines",
            "symbol=US.AAPL&period=2h",
        )
        .expect_err("invalid period");
    assert!(matches!(invalid_period, BrokerReadSnapshotError::Invalid(message) if message.contains("period")));
    let conflicting = port
        .read(
            "/api/v1/brokers/futu/klines",
            "symbol=US.AAPL&before=2026-08-29T00:00:00Z&fromTime=2026-08-28",
        )
        .expect_err("before/from conflict");
    assert!(matches!(conflicting, BrokerReadSnapshotError::Invalid(message) if message.contains("combined")));
}

#[test]
fn broker_klines_requires_symbol_and_valid_before_timestamp() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(FakeTradeRead)), Some(true));
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(runtime),
    };
    let missing_symbol = port
        .read("/api/v1/brokers/futu/klines", "period=1d")
        .expect_err("missing symbol");
    assert!(matches!(missing_symbol, BrokerReadSnapshotError::Invalid(message) if message.contains("symbol")));
    let invalid_before = port
        .read(
            "/api/v1/brokers/futu/klines",
            "symbol=US.AAPL&before=not-a-time",
        )
        .expect_err("invalid before");
    assert!(matches!(invalid_before, BrokerReadSnapshotError::Invalid(message) if message.contains("RFC3339")));
}

#[test]
fn generated_trade_enum_values_are_preserved() {
    assert_eq!(order_type_label(5), "ABSOLUTELIMIT");
    assert_eq!(order_type_label(6), "AUCTION");
    assert_eq!(order_type_label(7), "AUCTIONLIMIT");
    assert_eq!(order_type_label(9), "SPECIALLIMIT_ALL");
    assert_eq!(order_status_label(5), "SUBMITTED");
    assert_eq!(order_status_label(10), "FILLED_PART");
    assert_eq!(order_status_label(11), "FILLED_ALL");
    assert_eq!(time_in_force_label(2), "IOC");
    assert_eq!(time_in_force_label(3), "GTD");
    assert_eq!(fill_status_label(0), "OK");
    assert_eq!(currency_label(Some(4)), Some("JPY"));
    assert_eq!(currency_label(Some(5)), Some("SGD"));
    assert_eq!(trade_side(3), "SELLSHORT");
    assert_eq!(trade_side(4), "BUYBACK");
}

#[test]
fn cleared_trade_runtime_cannot_fall_back_to_static_client() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(FakeTradeRead)), Some(false));
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: Some(Arc::new(FakeTradeRead)),
        trade_logged_in: Some(true),
        trade_runtime: Some(runtime),
    };
    let error = port
        .read("/api/v1/brokers/futu/funds", "accountId=42&market=US")
        .expect_err("runtime login false must fail closed");
    assert!(error.to_string().contains("trade session"));
}
