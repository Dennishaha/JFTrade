use super::*;
use jftrade_integration_futu::{
    TradeAccountSnapshot, TradeCashFlowSnapshot, TradeFillSnapshot, TradeFunds, TradeFundsSnapshot,
    TradeOrderSnapshot, TradePositionSnapshot,
};

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

    fn read_positions(&self, _: TradeHeader, _: Option<TradeFilter>, _: Option<f64>, _: Option<f64>, _: Option<bool>, _: Option<i32>, _: Option<i32>, _: Option<bool>) -> Result<Vec<TradePositionSnapshot>, TradeSessionError> { Ok(Vec::new()) }
    fn read_orders(&self, _: TradeHeader, _: Option<TradeFilter>, _: Vec<i32>, _: Option<bool>) -> Result<Vec<TradeOrderSnapshot>, TradeSessionError> { Ok(Vec::new()) }
    fn read_fills(&self, _: TradeHeader, _: Option<TradeFilter>, _: Option<bool>) -> Result<Vec<TradeFillSnapshot>, TradeSessionError> { Ok(Vec::new()) }
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
fn margin_ratios_require_symbols() {
    let port = ProductionBrokerPort { active_provider_state: ready_state(), trade_read_port: Some(Arc::new(FakeTradeRead)), trade_logged_in: Some(true), trade_runtime: None };
    let error = port
        .read("/api/v1/brokers/futu/margin-ratios", "accountId=42&market=US")
        .expect_err("missing symbol");
    assert!(matches!(error, BrokerReadSnapshotError::Invalid(message) if message.contains("symbol")));
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
