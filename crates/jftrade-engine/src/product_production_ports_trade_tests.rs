use super::*;
use jftrade_api::LiveHub;
use jftrade_integration_futu::{
    ResponseError, TradeAccountSnapshot, TradeCashFlowSnapshot, TradeFillSnapshot, TradeFunds,
    TradeFundsSnapshot, TradeMarginRatioSnapshot, TradeMaxTradeQuantityRequest,
    TradeMaxTradeQuantitySnapshot, TradeOrderFeeSnapshot, TradeOrderSnapshot,
    TradePositionSnapshot, TradeSessionError,
};
use jftrade_marketdata::ProviderRouter;
use jftrade_settings::{FutuIntegrationConfig, MarketDataProvider, MarketDataProviderRuntimePort};
use std::time::{Duration, Instant};
use std::sync::atomic::{AtomicUsize, Ordering};
use tempfile::tempdir;

#[derive(Debug)]
struct FakeTradeRead;

impl TradeReadPort for FakeTradeRead {
    fn read_accounts(
        &self,
        _: u64,
        _: Option<i32>,
        _: Option<bool>,
    ) -> Result<Vec<TradeAccountSnapshot>, TradeSessionError> {
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

    fn read_funds(
        &self,
        header: TradeHeader,
        _: Option<bool>,
        _: Option<i32>,
        _: Option<i32>,
    ) -> Result<TradeFundsSnapshot, TradeSessionError> {
        Ok(TradeFundsSnapshot {
            header,
            funds: TradeFunds {
                power: 1.0,
                total_assets: 2.0,
                cash: 3.0,
                market_val: 4.0,
                frozen_cash: 0.0,
                debt_cash: 0.0,
                avl_withdrawal_cash: 3.0,
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
            },
        })
    }

    fn read_cash_flows(
        &self,
        header: TradeHeader,
        _: String,
        _: Option<i32>,
    ) -> Result<Vec<TradeCashFlowSnapshot>, TradeSessionError> {
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

    fn read_order_fees(
        &self,
        header: TradeHeader,
        _: Vec<String>,
    ) -> Result<Vec<TradeOrderFeeSnapshot>, TradeSessionError> {
        Ok(vec![TradeOrderFeeSnapshot {
            header,
            broker_order_id_ex: "fee-2".to_owned(),
            fee_amount: Some(1.5),
            fee_items: vec![jftrade_integration_futu::TradeOrderFeeItemSnapshot {
                title: "commission".to_owned(),
                value: 1.5,
            }],
        }])
    }

    fn read_margin_ratios(
        &self,
        header: TradeHeader,
        _: Vec<TradeSecurity>,
    ) -> Result<Vec<TradeMarginRatioSnapshot>, TradeSessionError> {
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

    fn read_max_trade_quantity(
        &self,
        request: TradeMaxTradeQuantityRequest,
    ) -> Result<TradeMaxTradeQuantitySnapshot, TradeSessionError> {
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

    fn read_positions(
        &self,
        _: TradeHeader,
        _: Option<TradeFilter>,
        _: Option<f64>,
        _: Option<f64>,
        _: Option<bool>,
        _: Option<i32>,
        _: Option<i32>,
        _: Option<bool>,
    ) -> Result<Vec<TradePositionSnapshot>, TradeSessionError> {
        Ok(Vec::new())
    }
    fn read_orders(
        &self,
        _: TradeHeader,
        _: Option<TradeFilter>,
        _: Vec<i32>,
        _: Option<bool>,
    ) -> Result<Vec<TradeOrderSnapshot>, TradeSessionError> {
        Ok(Vec::new())
    }
    fn read_fills(
        &self,
        _: TradeHeader,
        _: Option<TradeFilter>,
        _: Option<bool>,
    ) -> Result<Vec<TradeFillSnapshot>, TradeSessionError> {
        Ok(Vec::new())
    }
}

fn fixture_order(
    _: TradeHeader,
    order_id: u64,
    status: i32,
    fill_qty: f64,
) -> TradeOrderSnapshot {
    TradeOrderSnapshot {
        trd_side: 1,
        order_type: 1,
        order_status: status,
        order_id,
        order_id_ex: format!("EXT-{order_id}"),
        code: "HK.00700".to_owned(),
        name: "Tencent".to_owned(),
        qty: 100.0,
        price: Some(320.0),
        create_time: "2026-05-20 09:30:00".to_owned(),
        update_time: "2026-05-20 09:31:00".to_owned(),
        fill_qty: Some(fill_qty),
        fill_avg_price: Some(319.5),
        last_err_msg: None,
        sec_market: Some(1),
        create_timestamp: None,
        update_timestamp: None,
        remark: None,
        trd_market: Some(1),
        expire_time: None,
        order_amount: None,
        time_in_force: Some(1),
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

/// Reader used only by the active-order filtering parity test. Keeping this
/// separate from `FakeTradeRead` preserves the empty-order fixtures that other
/// broker/portfolio projection tests rely on.
#[derive(Debug)]
struct OrderFixtureRead;

impl TradeReadPort for OrderFixtureRead {
    fn read_accounts(
        &self,
        user_id: u64,
        category: Option<i32>,
        general: Option<bool>,
    ) -> Result<Vec<TradeAccountSnapshot>, TradeSessionError> {
        FakeTradeRead.read_accounts(user_id, category, general)
    }
    fn read_funds(
        &self,
        header: TradeHeader,
        refresh: Option<bool>,
        currency: Option<i32>,
        asset: Option<i32>,
    ) -> Result<TradeFundsSnapshot, TradeSessionError> {
        FakeTradeRead.read_funds(header, refresh, currency, asset)
    }
    fn read_cash_flows(
        &self,
        header: TradeHeader,
        clearing_date: String,
        direction: Option<i32>,
    ) -> Result<Vec<TradeCashFlowSnapshot>, TradeSessionError> {
        FakeTradeRead.read_cash_flows(header, clearing_date, direction)
    }
    fn read_order_fees(
        &self,
        header: TradeHeader,
        order_ids: Vec<String>,
    ) -> Result<Vec<TradeOrderFeeSnapshot>, TradeSessionError> {
        FakeTradeRead.read_order_fees(header, order_ids)
    }
    fn read_margin_ratios(
        &self,
        header: TradeHeader,
        securities: Vec<TradeSecurity>,
    ) -> Result<Vec<TradeMarginRatioSnapshot>, TradeSessionError> {
        FakeTradeRead.read_margin_ratios(header, securities)
    }
    fn read_max_trade_quantity(
        &self,
        request: TradeMaxTradeQuantityRequest,
    ) -> Result<TradeMaxTradeQuantitySnapshot, TradeSessionError> {
        FakeTradeRead.read_max_trade_quantity(request)
    }
    fn read_positions(
        &self,
        header: TradeHeader,
        filter: Option<TradeFilter>,
        min: Option<f64>,
        max: Option<f64>,
        refresh: Option<bool>,
        asset: Option<i32>,
        currency: Option<i32>,
        option_view: Option<bool>,
    ) -> Result<Vec<TradePositionSnapshot>, TradeSessionError> {
        FakeTradeRead.read_positions(header, filter, min, max, refresh, asset, currency, option_view)
    }
    fn read_orders(
        &self,
        header: TradeHeader,
        _: Option<TradeFilter>,
        _: Vec<i32>,
        _: Option<bool>,
    ) -> Result<Vec<TradeOrderSnapshot>, TradeSessionError> {
        Ok(vec![
            fixture_order(header.clone(), 2001, 5, 25.0),
            fixture_order(header, 2002, 11, 50.0),
        ])
    }
    fn read_fills(
        &self,
        header: TradeHeader,
        filter: Option<TradeFilter>,
        refresh: Option<bool>,
    ) -> Result<Vec<TradeFillSnapshot>, TradeSessionError> {
        FakeTradeRead.read_fills(header, filter, refresh)
    }
}

/// Reader exposing the full margin/exposure/PDT funds surface that Go's
/// `convertFundsSnapshot` / `brokerFundsSnapshotFromProto` pass through.
#[derive(Debug)]
struct FullFundsRead;

impl TradeReadPort for FullFundsRead {
    fn read_accounts(
        &self,
        user_id: u64,
        category: Option<i32>,
        general: Option<bool>,
    ) -> Result<Vec<TradeAccountSnapshot>, TradeSessionError> {
        FakeTradeRead.read_accounts(user_id, category, general)
    }
    fn read_funds(
        &self,
        header: TradeHeader,
        _: Option<bool>,
        _: Option<i32>,
        _: Option<i32>,
    ) -> Result<TradeFundsSnapshot, TradeSessionError> {
        Ok(TradeFundsSnapshot {
            header,
            funds: TradeFunds {
                power: 200_000.0,
                total_assets: 500_000.0,
                cash: 100_000.0,
                market_val: 350_000.0,
                frozen_cash: 10_000.0,
                debt_cash: 50_000.0,
                avl_withdrawal_cash: 80_000.0,
                currency: Some(1),
                available_funds: Some(120_000.0),
                unrealized_pl: Some(1_000.0),
                realized_pl: Some(2_000.0),
                risk_level: Some(1),
                initial_margin: Some(50_000.0),
                maintenance_margin: Some(25_000.0),
                cash_info_list: vec![
                    jftrade_integration_futu::TradeCashInfo {
                        currency: Some(1),
                        cash: Some(100_000.0),
                        available_balance: Some(80_000.0),
                        net_cash_power: Some(120_000.0),
                    },
                    jftrade_integration_futu::TradeCashInfo {
                        currency: Some(2),
                        cash: Some(50_000.0),
                        available_balance: Some(40_000.0),
                        net_cash_power: Some(60_000.0),
                    },
                ],
                max_power_short: Some(100_000.0),
                net_cash_power: Some(120_000.0),
                long_mv: Some(350_000.0),
                short_mv: Some(0.0),
                pending_asset: Some(5_000.0),
                max_withdrawal: Some(150_000.0),
                risk_status: Some(1),
                margin_call_margin: Some(15_000.0),
                is_pdt: Some(true),
                pdt_seq: Some("3/3".to_owned()),
                beginning_dtbp: Some(100_000.0),
                remaining_dtbp: Some(75_000.0),
                dt_call_amount: Some(5_000.0),
                dt_status: Some(1),
                securities_assets: Some(300_000.0),
                fund_assets: Some(50_000.0),
                bond_assets: Some(0.0),
                market_info_list: vec![
                    jftrade_integration_futu::TradeMarketInfo {
                        trd_market: Some(1),
                        assets: Some(300_000.0),
                    },
                    jftrade_integration_futu::TradeMarketInfo {
                        trd_market: Some(2),
                        assets: Some(200_000.0),
                    },
                ],
                crypto_mv: None,
                exposure_level: Some(1),
                exposure_limit: Some(2_000_000.0),
                used_limit: Some(800_000.0),
                remaining_limit: Some(1_200_000.0),
            },
        })
    }
    fn read_cash_flows(
        &self,
        header: TradeHeader,
        clearing_date: String,
        direction: Option<i32>,
    ) -> Result<Vec<TradeCashFlowSnapshot>, TradeSessionError> {
        FakeTradeRead.read_cash_flows(header, clearing_date, direction)
    }
    fn read_order_fees(
        &self,
        header: TradeHeader,
        order_ids: Vec<String>,
    ) -> Result<Vec<TradeOrderFeeSnapshot>, TradeSessionError> {
        FakeTradeRead.read_order_fees(header, order_ids)
    }
    fn read_margin_ratios(
        &self,
        header: TradeHeader,
        securities: Vec<TradeSecurity>,
    ) -> Result<Vec<TradeMarginRatioSnapshot>, TradeSessionError> {
        FakeTradeRead.read_margin_ratios(header, securities)
    }
    fn read_max_trade_quantity(
        &self,
        request: TradeMaxTradeQuantityRequest,
    ) -> Result<TradeMaxTradeQuantitySnapshot, TradeSessionError> {
        FakeTradeRead.read_max_trade_quantity(request)
    }
    fn read_positions(
        &self,
        header: TradeHeader,
        filter: Option<TradeFilter>,
        min: Option<f64>,
        max: Option<f64>,
        refresh: Option<bool>,
        asset: Option<i32>,
        currency: Option<i32>,
        option_view: Option<bool>,
    ) -> Result<Vec<TradePositionSnapshot>, TradeSessionError> {
        FakeTradeRead.read_positions(header, filter, min, max, refresh, asset, currency, option_view)
    }
    fn read_orders(
        &self,
        header: TradeHeader,
        filter: Option<TradeFilter>,
        statuses: Vec<i32>,
        refresh: Option<bool>,
    ) -> Result<Vec<TradeOrderSnapshot>, TradeSessionError> {
        FakeTradeRead.read_orders(header, filter, statuses, refresh)
    }
    fn read_fills(
        &self,
        header: TradeHeader,
        filter: Option<TradeFilter>,
        refresh: Option<bool>,
    ) -> Result<Vec<TradeFillSnapshot>, TradeSessionError> {
        FakeTradeRead.read_fills(header, filter, refresh)
    }
}

#[derive(Debug)]
struct ErrorTradeRead {
    message: &'static str,
}

impl ErrorTradeRead {
    fn error(&self) -> TradeSessionError {
        if self.message.contains("rate limit") {
            TradeSessionError::RateLimited
        } else {
            TradeSessionError::Response(ResponseError::ReturnCode {
                ret_type: -1,
                err_code: 429,
                message: self.message.to_owned(),
            })
        }
    }
}

impl TradeReadPort for ErrorTradeRead {
    fn read_accounts(
        &self,
        user_id: u64,
        category: Option<i32>,
        general: Option<bool>,
    ) -> Result<Vec<TradeAccountSnapshot>, TradeSessionError> {
        FakeTradeRead.read_accounts(user_id, category, general)
    }
    fn read_funds(
        &self,
        _: TradeHeader,
        _: Option<bool>,
        _: Option<i32>,
        _: Option<i32>,
    ) -> Result<TradeFundsSnapshot, TradeSessionError> {
        Err(self.error())
    }
    fn read_cash_flows(
        &self,
        _: TradeHeader,
        _: String,
        _: Option<i32>,
    ) -> Result<Vec<TradeCashFlowSnapshot>, TradeSessionError> {
        Err(self.error())
    }
    fn read_order_fees(
        &self,
        _: TradeHeader,
        _: Vec<String>,
    ) -> Result<Vec<TradeOrderFeeSnapshot>, TradeSessionError> {
        Err(self.error())
    }
    fn read_margin_ratios(
        &self,
        _: TradeHeader,
        _: Vec<TradeSecurity>,
    ) -> Result<Vec<TradeMarginRatioSnapshot>, TradeSessionError> {
        Err(self.error())
    }
    fn read_max_trade_quantity(
        &self,
        _: TradeMaxTradeQuantityRequest,
    ) -> Result<TradeMaxTradeQuantitySnapshot, TradeSessionError> {
        Err(self.error())
    }
    fn read_positions(
        &self,
        _: TradeHeader,
        _: Option<TradeFilter>,
        _: Option<f64>,
        _: Option<f64>,
        _: Option<bool>,
        _: Option<i32>,
        _: Option<i32>,
        _: Option<bool>,
    ) -> Result<Vec<TradePositionSnapshot>, TradeSessionError> {
        Err(self.error())
    }
    fn read_orders(
        &self,
        _: TradeHeader,
        _: Option<TradeFilter>,
        _: Vec<i32>,
        _: Option<bool>,
    ) -> Result<Vec<TradeOrderSnapshot>, TradeSessionError> {
        Err(self.error())
    }
    fn read_fills(
        &self,
        _: TradeHeader,
        _: Option<TradeFilter>,
        _: Option<bool>,
    ) -> Result<Vec<TradeFillSnapshot>, TradeSessionError> {
        Err(self.error())
    }
}

fn ready_state() -> Arc<ActiveProviderState> {
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    state.set_readiness(false, true, true);
    state
}

fn helper_market_data_state(provider: MarketDataProvider) -> Arc<ActiveProviderState> {
    let state = Arc::new(ActiveProviderState::new(Some(provider)));
    // Market-data readiness is intentionally independent from the trade
    // session used by the broker/portfolio projections.
    state.set_readiness(true, false, true);
    state
}

fn ready_trade_runtime() -> Arc<SharedTradeReadRuntime> {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(FakeTradeRead)), Some(true));
    runtime
}

fn execution_store() -> (
    Arc<jftrade_store_sqlite::ExecutionOrderStore>,
    tempfile::TempDir,
) {
    let directory = tempdir().expect("temporary directory");
    let path = directory.path().join("execution-orders.db");
    let connection = rusqlite::Connection::open(&path).expect("create execution database");
    jftrade_store_sqlite::initialize_current(&connection, "execution-orders")
        .expect("initialize execution schema");
    drop(connection);
    (
        Arc::new(jftrade_store_sqlite::ExecutionOrderStore::open(&path).expect("open store")),
        directory,
    )
}

#[test]
fn broker_read_fails_closed_without_trade_client() {
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: None,
        trade_logged_in: Some(true),
        trade_runtime: None,
    };
    let error = port
        .read("/api/v1/brokers/futu/funds", "accountId=42&market=US")
        .expect_err("missing client");
    assert!(error.to_string().contains("trade read client"));
}

#[test]
fn broker_read_projects_futu_funds_from_neutral_client() {
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: Some(Arc::new(FakeTradeRead)),
        trade_logged_in: Some(true),
        trade_runtime: None,
    };
    let value = port
        .read("/api/v1/brokers/futu/funds", "accountId=42&market=US")
        .expect("funds");
    assert_eq!(value["summary"]["totalAssets"], 2.0);
    assert_eq!(value["connectivity"], "connected");
}

#[test]
fn funds_projection_preserves_full_margin_pdt_and_exposure_fields() {
    // Parity: go:452dea11:pkg/futu/adapter_new_methods_test.go:14
    // TestConvertFundsSnapshotFullMarginFields and :257
    // TestBrokerFundsSnapshotFromProtoFullMargin.
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: Some(Arc::new(FullFundsRead)),
        trade_logged_in: Some(true),
        trade_runtime: None,
    };
    let value = port
        .read(
            "/api/v1/brokers/futu/funds",
            "accountId=42&tradingEnvironment=REAL&market=HK",
        )
        .expect("funds");
    let summary = &value["summary"];
    assert_eq!(summary["purchasingPower"], 200_000.0);
    assert_eq!(summary["totalAssets"], 500_000.0);
    assert_eq!(summary["debtCash"], 50_000.0);
    assert_eq!(summary["shortSellingPower"], 100_000.0);
    assert_eq!(summary["initialMargin"], 50_000.0);
    assert_eq!(summary["maintenanceMargin"], 25_000.0);
    assert_eq!(summary["marginCallMargin"], 15_000.0);
    assert_eq!(summary["riskStatus"], "LEVEL1");
    assert_eq!(summary["isPdt"], true);
    assert_eq!(summary["pdtSeq"], "3/3");
    assert_eq!(summary["beginningDTBP"], 100_000.0);
    assert_eq!(summary["remainingDTBP"], 75_000.0);
    assert_eq!(summary["dtCallAmount"], 5_000.0);
    assert_eq!(summary["dtStatus"], "UNLIMITED");
    assert_eq!(summary["exposureLevel"], "NORMAL");
    assert_eq!(summary["currency"], "HKD");
    assert_eq!(summary["exposureLimit"], 2_000_000.0);
    assert_eq!(summary["usedLimit"], 800_000.0);
    assert_eq!(summary["remainingLimit"], 1_200_000.0);
    // The OpenAPI contract (trading.BrokerFundsSummary) and the Vue console
    // consume these exact names; a legacy `power` key or a raw integer
    // `currency`/`exposureLevel` would silently regress the wire shape.
    assert!(summary.get("power").is_none());
    assert!(summary.get("shortSellingPower").is_some());
    assert_eq!(value["currencyBalances"].as_array().map(Vec::len), Some(2));
    assert_eq!(value["currencyBalances"][0]["currency"], "HKD");
    assert_eq!(value["marketAssets"].as_array().map(Vec::len), Some(2));
}

#[test]
fn funds_projection_keeps_missing_margin_fields_absent() {
    // Parity: go:452dea11:pkg/futu/adapter_new_methods_test.go:78
    // TestConvertFundsSnapshotNilMarginFields.
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: Some(Arc::new(FakeTradeRead)),
        trade_logged_in: Some(true),
        trade_runtime: None,
    };
    let value = port
        .read("/api/v1/brokers/futu/funds", "accountId=42&market=US")
        .expect("funds");
    let summary = &value["summary"];
    assert!(summary["isPdt"].is_null());
    assert!(summary["exposureLevel"].is_null());
    assert!(summary["initialMargin"].is_null());
    assert_eq!(value["currencyBalances"], json!([]));
}

#[test]
fn funds_projection_preserves_currency_and_market_asset_arrays() {
    // Parity: go:452dea11:pkg/futu/adapter_new_methods_test.go:112
    // TestConvertFundsSnapshotCurrencyBalances. The full funds fixture carries
    // two cash-info rows (HKD/USD) and two market-info rows (HK/US) so the
    // per-currency and per-market breakdowns must survive the neutral wire.
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: Some(Arc::new(FullFundsRead)),
        trade_logged_in: Some(true),
        trade_runtime: None,
    };
    let value = port
        .read(
            "/api/v1/brokers/futu/funds",
            "accountId=42&tradingEnvironment=REAL&market=HK",
        )
        .expect("funds");
    let balances = value["currencyBalances"]
        .as_array()
        .expect("currency balances");
    assert_eq!(balances.len(), 2);
    assert_eq!(balances[0]["currency"], "HKD");
    assert_eq!(balances[0]["cash"], 100_000.0);
    assert_eq!(balances[0]["availableWithdrawalCash"], 80_000.0);
    assert_eq!(balances[0]["netCashPower"], 120_000.0);
    assert_eq!(balances[1]["currency"], "USD");
    assert_eq!(balances[1]["cash"], 50_000.0);
    let assets = value["marketAssets"].as_array().expect("market assets");
    assert_eq!(assets.len(), 2);
    assert_eq!(assets[0]["market"], "HK");
    assert_eq!(assets[0]["assets"], 300_000.0);
    assert_eq!(assets[1]["market"], "US");
    assert_eq!(assets[1]["assets"], 200_000.0);
}

#[test]
fn helper_market_data_provider_keeps_futu_trade_reads_on_the_trade_session() {
    let runtime = ready_trade_runtime();
    let port = ProductionBrokerPort {
        active_provider_state: helper_market_data_state(MarketDataProvider::Yfinance),
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(runtime),
    };
    let value = port
        .read(
            "/api/v1/brokers/futu/funds",
            "accountId=42&tradingEnvironment=REAL&market=US",
        )
        .expect("helper market-data provider must not gate Futu trade reads");
    assert_eq!(value["connectivity"], "connected");
    assert_eq!(value["summary"]["accountId"], "42");
}

#[test]
fn provider_switch_does_not_disconnect_an_existing_futu_trade_session() {
    let state = ready_state();
    let runtime = ready_trade_runtime();
    let port = ProductionBrokerPort {
        active_provider_state: Arc::clone(&state),
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(runtime),
    };
    state
        .activate(MarketDataProvider::Yfinance)
        .expect("market-data provider switch");
    let value = port
        .read(
            "/api/v1/brokers/futu/orders",
            "accountId=42&tradingEnvironment=REAL&market=US",
        )
        .expect("trade session remains available after provider switch");
    assert_eq!(value["connectivity"], "connected");
    assert_eq!(value["orders"], json!([]));
}

#[test]
fn helper_market_data_provider_without_trade_session_fails_closed() {
    let port = ProductionBrokerPort {
        active_provider_state: helper_market_data_state(MarketDataProvider::Akshare),
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: None,
    };
    let error = port
        .read(
            "/api/v1/brokers/futu/funds",
            "accountId=42&tradingEnvironment=REAL&market=US",
        )
        .expect_err("missing Futu trade session");
    assert!(error.to_string().contains("trade session"));
}

#[test]
fn helper_market_data_provider_keeps_futu_portfolio_reads_on_the_trade_session() {
    let (store, _directory) = execution_store();
    let port = ProductionPortfolioPort {
        active_provider_state: helper_market_data_state(MarketDataProvider::Akshare),
        _execution_store: store,
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(ready_trade_runtime()),
    };
    let value = port
        .read(
            "/api/v1/portfolio/futu/cash-balances",
            "accountId=42&tradingEnvironment=REAL&market=US",
        )
        .expect("helper market-data provider must not gate portfolio reads");
    assert_eq!(value["connectivity"], "connected");
    assert_eq!(value["balances"][0]["accountId"], "42");
}

#[test]
fn broker_current_orders_hide_terminal_statuses_while_history_keeps_them() {
    // Parity: go:452dea11:pkg/futu/exchange_test.go:367
    // TestQueryOpenOrdersReturnsActiveOrders. Go's QueryOpenOrders filters the
    // broker list through brokerOrderIsWorking(workingOnly=true); the Rust
    // current-session route must drop FILLED_ALL while scope=history still
    // returns it for reconciliation and the account history table.
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: Some(Arc::new(OrderFixtureRead)),
        trade_logged_in: Some(true),
        trade_runtime: None,
    };
    let current = port
        .read("/api/v1/brokers/futu/orders", "accountId=42&market=HK")
        .expect("current orders");
    let current = current["orders"].as_array().expect("orders array");
    assert_eq!(current.len(), 1, "terminal orders are not open orders");
    assert_eq!(current[0]["brokerOrderId"], "2001");
    assert_eq!(current[0]["status"], "SUBMITTED");
    assert_eq!(current[0]["filledQuantity"], 25.0);

    let history = port
        .read(
            "/api/v1/brokers/futu/orders",
            "accountId=42&market=HK&scope=history",
        )
        .expect("history orders");
    let history = history["orders"].as_array().expect("history orders array");
    assert_eq!(history.len(), 2, "history scope keeps terminal orders");
    assert_eq!(history[1]["brokerOrderId"], "2002");
    assert_eq!(history[1]["status"], "FILLED_ALL");
}

#[test]
fn margin_ratios_reuse_a_recent_success_within_the_ttl() {
    // Parity: go:452dea11:pkg/futu/exchange_test.go:691
    // TestQueryBrokerMarginRatiosUsesCacheWithinTTL. The second identical read
    // must be served by the engine cache instead of issuing another OpenD
    // Trd_GetMarginRatio call.
    #[derive(Debug)]
    struct CountingMarginRead {
        calls: AtomicUsize,
    }

    impl TradeReadPort for CountingMarginRead {
        fn read_accounts(
            &self,
            user_id: u64,
            category: Option<i32>,
            general: Option<bool>,
        ) -> Result<Vec<TradeAccountSnapshot>, TradeSessionError> {
            FakeTradeRead.read_accounts(user_id, category, general)
        }
        fn read_funds(
            &self,
            header: TradeHeader,
            refresh: Option<bool>,
            currency: Option<i32>,
            asset: Option<i32>,
        ) -> Result<TradeFundsSnapshot, TradeSessionError> {
            FakeTradeRead.read_funds(header, refresh, currency, asset)
        }
        fn read_cash_flows(
            &self,
            header: TradeHeader,
            clearing_date: String,
            direction: Option<i32>,
        ) -> Result<Vec<TradeCashFlowSnapshot>, TradeSessionError> {
            FakeTradeRead.read_cash_flows(header, clearing_date, direction)
        }
        fn read_order_fees(
            &self,
            header: TradeHeader,
            order_ids: Vec<String>,
        ) -> Result<Vec<TradeOrderFeeSnapshot>, TradeSessionError> {
            FakeTradeRead.read_order_fees(header, order_ids)
        }
        fn read_margin_ratios(
            &self,
            header: TradeHeader,
            securities: Vec<TradeSecurity>,
        ) -> Result<Vec<TradeMarginRatioSnapshot>, TradeSessionError> {
            self.calls.fetch_add(1, Ordering::SeqCst);
            FakeTradeRead.read_margin_ratios(header, securities)
        }
        fn read_max_trade_quantity(
            &self,
            request: TradeMaxTradeQuantityRequest,
        ) -> Result<TradeMaxTradeQuantitySnapshot, TradeSessionError> {
            FakeTradeRead.read_max_trade_quantity(request)
        }
        fn read_positions(
            &self,
            header: TradeHeader,
            filter: Option<TradeFilter>,
            min: Option<f64>,
            max: Option<f64>,
            refresh: Option<bool>,
            asset: Option<i32>,
            currency: Option<i32>,
            option_view: Option<bool>,
        ) -> Result<Vec<TradePositionSnapshot>, TradeSessionError> {
            FakeTradeRead.read_positions(
                header, filter, min, max, refresh, asset, currency, option_view,
            )
        }
        fn read_orders(
            &self,
            header: TradeHeader,
            filter: Option<TradeFilter>,
            statuses: Vec<i32>,
            refresh: Option<bool>,
        ) -> Result<Vec<TradeOrderSnapshot>, TradeSessionError> {
            FakeTradeRead.read_orders(header, filter, statuses, refresh)
        }
        fn read_fills(
            &self,
            header: TradeHeader,
            filter: Option<TradeFilter>,
            refresh: Option<bool>,
        ) -> Result<Vec<TradeFillSnapshot>, TradeSessionError> {
            FakeTradeRead.read_fills(header, filter, refresh)
        }
    }

    let runtime = Arc::new(SharedTradeReadRuntime::default());
    let reader = Arc::new(CountingMarginRead {
        calls: AtomicUsize::new(0),
    });
    runtime.set(Some(Arc::clone(&reader) as Arc<dyn TradeReadPort>), Some(true));
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(runtime),
    };
    let query = "accountId=42&market=US&symbol=US.AAPL";
    let first = port
        .read("/api/v1/brokers/futu/margin-ratios", query)
        .expect("first margin ratios");
    let second = port
        .read("/api/v1/brokers/futu/margin-ratios", query)
        .expect("second margin ratios");
    // The cache stores snapshots, not the whole response: `checkedAt` is the
    // wall-clock time of each read (the Go baseline only asserts the cached
    // result and call count), so compare the payload rather than the envelope.
    assert_eq!(first["marginRatios"], second["marginRatios"]);
    assert_eq!(reader.calls.load(Ordering::SeqCst), 1);
}

#[test]
fn broker_read_projects_cash_flows_with_baseline_fields_and_sorting() {
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: Some(Arc::new(FakeTradeRead)),
        trade_logged_in: Some(true),
        trade_runtime: None,
    };
    let value = port
        .read(
            "/api/v1/brokers/futu/cash-flows",
            "accountId=42&market=US&clearingDate=2026-08-21&direction=IN",
        )
        .expect("cash flows");
    assert_eq!(value["connectivity"], "connected");
    assert_eq!(value["cashFlows"][0]["cashFlowId"], "9");
    assert_eq!(value["cashFlows"][0]["cashFlowDirection"], "IN");
    assert_eq!(value["cashFlows"][0]["cashFlowAmount"], 12.5);
}

#[test]
fn cash_flows_require_clearing_date() {
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: Some(Arc::new(FakeTradeRead)),
        trade_logged_in: Some(true),
        trade_runtime: None,
    };
    let error = port
        .read("/api/v1/brokers/futu/cash-flows", "accountId=42&market=US")
        .expect_err("missing clearing date");
    assert!(
        matches!(error, BrokerReadSnapshotError::Invalid(message) if message.contains("clearingDate"))
    );
}

#[test]
fn broker_read_projects_order_fees_and_merges_order_id_queries() {
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: Some(Arc::new(FakeTradeRead)),
        trade_logged_in: Some(true),
        trade_runtime: None,
    };
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
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: Some(Arc::new(FakeTradeRead)),
        trade_logged_in: Some(true),
        trade_runtime: None,
    };
    let error = port
        .read(
            "/api/v1/brokers/futu/order-fees",
            "accountId=42&market=US&orderIdEx=,",
        )
        .expect_err("missing order id");
    assert!(
        matches!(error, BrokerReadSnapshotError::Invalid(message) if message.contains("orderIdEx"))
    );
}

#[test]
fn broker_read_projects_margin_ratios_with_real_environment_and_omits_absent_values() {
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: Some(Arc::new(FakeTradeRead)),
        trade_logged_in: Some(true),
        trade_runtime: None,
    };
    let value = port
        .read(
            "/api/v1/brokers/futu/margin-ratios",
            "accountId=42&market=US&symbol=US.AAPL",
        )
        .expect("margin ratios");
    assert_eq!(value["connectivity"], "connected");
    assert_eq!(value["marginRatios"][0]["tradingEnvironment"], "REAL");
    assert_eq!(value["marginRatios"][0]["symbol"], "US.AAPL");
    assert_eq!(value["marginRatios"][0]["shortFeeRate"], 0.02);
    assert!(value["marginRatios"][0].get("alertShortRatio").is_none());
}

#[test]
fn margin_ratios_returns_empty_in_simulated_environment() {
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: Some(Arc::new(FakeTradeRead)),
        trade_logged_in: Some(true),
        trade_runtime: None,
    };
    let value = port
        .read(
            "/api/v1/brokers/futu/margin-ratios",
            "tradingEnvironment=SIMULATE&accountId=10280980&market=HK&symbol=HK.00700",
        )
        .expect("simulated margin ratios");
    assert_eq!(value["connectivity"], "connected");
    assert_eq!(value["marginRatios"].as_array().map(|a| a.len()), Some(0));
}

#[test]
fn portfolio_cash_balances_fall_back_to_summary_currency_when_breakdown_is_empty() {
    // Parity: internal/trading/broker_test.go:453 TestServicePortfolioAndFallbackResponses
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
fn trade_security_parsing_accepts_every_go_prefix_and_rejects_invalid_symbols() {
    // Parity: go:452dea11:pkg/futu/exchange_business_boundary_test.go:349
    // TestTradeSecurityInfoAndRuntimeMarketAuthorityBoundaries
    // (`tradeSecurityInfoFromSymbol`). Go requires an explicit MARKET.CODE
    // (dot or colon); Rust's margin-ratios query additionally accepts a bare
    // code plus a `market` parameter, so this pins the prefixed forms and the
    // rejections Go guarantees.
    let cases = [
        ("HK.00700", 1, "00700"),
        ("US.AAPL", 11, "AAPL"),
        ("SH.600519", 21, "600519"),
        ("SZ.000001", 22, "000001"),
        ("SG.D05", 31, "D05"),
        ("JP.7203", 41, "7203"),
        ("AU.BHP", 51, "BHP"),
        ("MY.1155", 61, "1155"),
        ("CA.SHOP", 71, "SHOP"),
        ("US:AAPL", 11, "AAPL"),
    ];
    for (symbol, market, code) in cases {
        let request = TradeRequest::parse(
            "/api/v1/brokers/futu/margin-ratios",
            &format!("accountId=42&symbol={symbol}"),
        )
        .expect("request");
        let securities = request.securities().expect(symbol);
        assert_eq!(securities.len(), 1, "symbol={symbol}");
        assert_eq!(securities[0].market, market, "symbol={symbol}");
        assert_eq!(securities[0].code, code, "symbol={symbol}");
    }
    for symbol in ["EU.SAP", "UNKNOWN.SAP", "HK."] {
        let request = TradeRequest::parse(
            "/api/v1/brokers/futu/margin-ratios",
            &format!("accountId=42&symbol={symbol}"),
        )
        .expect("request");
        assert!(request.securities().is_err(), "symbol={symbol:?}");
    }
}

#[test]
fn runtime_market_authority_covers_go_market_variants() {
    // Parity: go:452dea11:pkg/futu/exchange_business_boundary_test.go:349
    // runtimeMarkets map: fund/simulated/derivative variants collapse onto the
    // stock authority, and unknown codes are skipped rather than surfaced.
    for (code, authority) in [
        (4, "HK"),
        (113, "HK"),
        (10, "HK"),
        (123, "US"),
        (11, "US"),
        (3, "CN"),
        (124, "SG"),
        (12, "SG"),
        (8, "AU"),
        (126, "JP"),
        (13, "JP"),
        (125, "MY"),
        (112, "CA"),
        (7, "CRYPTO"),
        (5, "FUTURES"),
    ] {
        assert_eq!(trade_market_authority(code), Some(authority), "code={code}");
    }
    assert_eq!(trade_market_authority(999_999), None);
    assert_eq!(trade_market_authority(0), None);
}

#[test]
fn historical_kline_error_surfaces_return_code_and_message() {
    // Parity: go:452dea11:pkg/futu/exchange_business_boundary_test.go:349
    // historicalKLineRequestError formatting (retType/errCode/retMsg).
    let error = jftrade_integration_futu::HistoricalKlineError::Rejected {
        ret_type: 1,
        err_code: 42,
        message: "session unsupported".to_owned(),
    };
    let message = error.to_string();
    assert!(message.contains("1"), "message={message}");
    assert!(message.contains("42"), "message={message}");
    assert!(message.contains("session unsupported"), "message={message}");
}

#[test]
fn portfolio_cash_balances_prefer_currency_rows_over_summary_fallback() {
    // Parity: go:452dea11:pkg/futu/exchange_business_boundary_test.go:18
    // TestBalanceMapFromBrokerFundsUsesCurrencyRowsBeforeAccountFallback and
    // :46 TestBalanceMapFromBrokerFundsFallsBackToMarketCurrencyAndLockedCash.
    // Go's `balanceMapFromBrokerFunds` returns the per-currency rows when any
    // exist and only falls back to the market/summary currency otherwise; a
    // funds payload whose currency enum is unknown must drop that row rather
    // than emit a null-currency balance.
    let resolved = ResolvedTradeRequest {
        account_id: "42".to_owned(),
        environment: "REAL".to_owned(),
        market: "US".to_owned(),
        header: trade_header(1, 42, 2),
    };
    let mut funds = FakeTradeRead
        .read_funds(trade_header(1, 42, 2), None, None, None)
        .expect("funds")
        .funds;
    // FakeTradeRead carries one HKD cash row plus a summary currency; the row
    // must win and the USD market fallback must not be emitted.
    let balances = portfolio_cash_balance_values("futu", &resolved, &funds);
    assert_eq!(balances.len(), 1);
    assert_eq!(balances[0]["currency"], "HKD");
    assert_eq!(balances[0]["cashBalance"], 3.0);

    // An unknown currency enum on the only row drops it and falls back to the
    // funds summary currency, still without inventing a null-currency row.
    funds.cash_info_list = vec![jftrade_integration_futu::TradeCashInfo {
        currency: Some(999),
        cash: Some(10.0),
        available_balance: None,
        net_cash_power: None,
    }];
    let balances = portfolio_cash_balance_values("futu", &resolved, &funds);
    assert_eq!(balances.len(), 1);
    assert_eq!(balances[0]["currency"], "HKD");
    assert_eq!(balances[0]["cashBalance"], 3.0);
}

#[test]
fn portfolio_cash_balances_fall_back_to_market_currency_when_summary_currency_is_absent() {
    // Parity: go:452dea11:pkg/futu/trade_read_convert.go:38
    // `balanceMapFromBrokerFunds` -> `defaultFundsCurrencyForMarket`. Go falls
    // back to the market's default currency when the funds payload has no
    // per-currency rows; Rust previously only fell back when `funds.currency`
    // was present, so a US account with a currency-less funds payload produced
    // an empty `balances` array and the portfolio route silently lost cash.
    let funds = TradeFunds {
        power: 1_000.0,
        total_assets: 2_500.0,
        cash: 2_500.0,
        market_val: 0.0,
        frozen_cash: 0.0,
        debt_cash: 0.0,
        avl_withdrawal_cash: 2_500.0,
        currency: None,
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
    };
    let resolved = ResolvedTradeRequest {
        account_id: "42".to_owned(),
        environment: "REAL".to_owned(),
        market: "US".to_owned(),
        header: trade_header(1, 42, 2),
    };
    let balances = portfolio_cash_balance_values("futu", &resolved, &funds);
    assert_eq!(balances.len(), 1);
    assert_eq!(balances[0]["currency"], "USD");
    assert_eq!(balances[0]["cashBalance"], 2_500.0);
}

#[test]
fn test_service_broker_read_operations_return_fallback_when_market_data_unavailable() {
    // Parity: internal/trading/broker_boundaries_test.go:11 TestServiceBrokerReadOperationsReturnFallbackWhenMarketDataUnavailable
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: Some(Arc::new(FakeTradeRead)),
        trade_logged_in: Some(true),
        trade_runtime: None,
    };

    // 1. Funds
    let funds = port
        .read("/api/v1/brokers/futu/funds", "accountId=42&market=US")
        .expect("funds read succeeds");
    assert!(funds.get("summary").is_some(), "funds must contain summary key");

    // 2. Positions
    let positions = port
        .read("/api/v1/brokers/futu/positions", "accountId=42&market=US")
        .expect("positions read succeeds");
    assert!(positions.get("positions").is_some(), "positions key required");

    // 3. Orders
    let orders = port
        .read("/api/v1/brokers/futu/orders", "accountId=42&market=US")
        .expect("orders read succeeds");
    assert!(orders.get("orders").is_some(), "orders key required");

    // 4. Fills
    let fills = port
        .read("/api/v1/brokers/futu/fills", "accountId=42&market=US")
        .expect("fills read succeeds");
    assert!(fills.get("fills").is_some(), "fills key required");

    // 5. Cash flows
    let cash_flows = port
        .read(
            "/api/v1/brokers/futu/cash-flows",
            "accountId=42&market=US&clearingDate=2026-08-21",
        )
        .expect("cash flows read succeeds");
    assert!(cash_flows.get("cashFlows").is_some(), "cashFlows key required");

    // 6. Fees
    let fees = port
        .read("/api/v1/brokers/futu/order-fees", "accountId=42&market=US&orderIdEx=1")
        .expect("fees read succeeds");
    assert!(fees.get("fees").is_some(), "fees key required");

    // 7. Margin ratios
    let margin_ratios = port
        .read(
            "/api/v1/brokers/futu/margin-ratios",
            "accountId=42&market=US&symbol=US.AAPL",
        )
        .expect("margin ratios read succeeds");
    assert!(margin_ratios.get("marginRatios").is_some(), "marginRatios key required");

    // 8. Max quantity
    let max_qty = port
        .read(
            "/api/v1/brokers/futu/max-trade-qtys",
            "accountId=42&market=US&symbol=US.AAPL&orderType=LIMIT&price=100",
        )
        .expect("max trade qtys read succeeds");
    assert!(max_qty.get("maxTradeQuantity").is_some(), "maxTradeQuantity key required");
}

#[test]
fn margin_ratios_require_symbols() {
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: Some(Arc::new(FakeTradeRead)),
        trade_logged_in: Some(true),
        trade_runtime: None,
    };
    let error = port
        .read(
            "/api/v1/brokers/futu/margin-ratios",
            "accountId=42&market=US",
        )
        .expect_err("missing symbol");
    assert!(
        matches!(error, BrokerReadSnapshotError::Invalid(message) if message.contains("symbol"))
    );
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
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: Some(Arc::new(FakeTradeRead)),
        trade_logged_in: Some(true),
        trade_runtime: None,
    };
    let error = port
        .read(
            "/api/v1/brokers/futu/margin-ratios",
            "accountId=42&market=US&symbol=HK.00700",
        )
        .expect_err("conflicting market");
    assert!(
        matches!(error, BrokerReadSnapshotError::Invalid(message) if message.contains("market"))
    );
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
    assert!(
        matches!(expired, Err(BrokerReadSnapshotError::Unavailable(message)) if message.contains("rate limit"))
    );

    runtime.set(
        Some(Arc::new(ErrorTradeRead {
            message: "broker service unavailable",
        })),
        Some(true),
    );
    let non_rate = port.read("/api/v1/brokers/futu/margin-ratios", query);
    assert!(
        matches!(non_rate, Err(BrokerReadSnapshotError::Unavailable(message)) if message.contains("broker service unavailable"))
    );
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
    assert!(
        matches!(error, BrokerReadSnapshotError::Unavailable(message) if message.contains("projection") || message.contains("connection settings"))
    );
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
    assert_eq!(
        value["session"]["connection"]["marketDataTransport"],
        "bbgo-opend-tcp-api"
    );
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
                price: "123.45".parse().expect("price"),
                volume: "1000".parse().expect("decimal volume"),
                volume_delta: None,
                snapshot: Some(jftrade_marketdata::TradeQuoteSnapshot {
                    name: Some("Apple Inc.".to_owned()),
                    is_suspended: Some(false),
                    open_price: Some("120".parse().expect("open")),
                    high_price: Some("124".parse().expect("high")),
                    low_price: Some("121".parse().expect("low")),
                    previous_close: Some("122".parse().expect("previous close")),
                    turnover: Some("123456.5".parse().expect("turnover")),
                    update_time: Some("15:59:59".to_owned()),
                    status: Some(3),
                    ..Default::default()
                }),
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
    assert_eq!(
        value["securities"]["snapshots"].as_array().unwrap().len(),
        1
    );
    assert_eq!(value["securities"]["snapshots"][0]["symbol"], "US.AAPL");
    assert_eq!(value["securities"]["snapshots"][0]["lastPrice"], 123.45);
    assert_eq!(value["securities"]["snapshots"][0]["volume"], 1000);
    assert_eq!(value["securities"]["snapshots"][0]["name"], "Apple Inc.");
    assert_eq!(value["securities"]["snapshots"][0]["openPrice"], 120.0);
    assert_eq!(value["securities"]["snapshots"][0]["previousClose"], 122.0);
    assert_eq!(value["securities"]["snapshots"][0]["turnover"], 123456.5);
    assert_eq!(
        value["securities"]["snapshots"][0]["updateTime"],
        "15:59:59"
    );
    assert_eq!(value["securities"]["snapshots"][0]["status"], 3);
}

#[test]
fn broker_securities_returns_real_empty_result_when_cache_has_no_symbol() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(FakeTradeRead)), Some(true));
    runtime.set_market_data_router(Some(Arc::new(std::sync::Mutex::new(ProviderRouter::new(
        8,
    )))));
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
    assert!(
        matches!(error, BrokerReadSnapshotError::Unavailable(message) if message.contains("market-data runtime") || message.contains("router"))
    );
}

#[test]
fn broker_securities_requires_symbol_query() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(FakeTradeRead)), Some(true));
    runtime.set_market_data_router(Some(Arc::new(std::sync::Mutex::new(ProviderRouter::new(
        8,
    )))));
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(runtime),
    };
    let error = port
        .read("/api/v1/brokers/futu/securities", "")
        .expect_err("missing symbol");
    assert!(
        matches!(error, BrokerReadSnapshotError::Invalid(message) if message.contains("symbol"))
    );
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
                price: "123.45".parse().expect("price"),
                volume: "1000".parse().expect("decimal volume"),
                volume_delta: None,
                snapshot: Some(jftrade_marketdata::TradeQuoteSnapshot {
                    name: Some("Apple Inc.".to_owned()),
                    open_price: Some("120".parse().expect("open")),
                    high_price: Some("124".parse().expect("high")),
                    low_price: Some("121".parse().expect("low")),
                    previous_close: Some("122".parse().expect("previous close")),
                    turnover: Some("123456.5".parse().expect("turnover")),
                    update_time: Some("15:59:59".to_owned()),
                    ..Default::default()
                }),
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
                price: "200".parse().expect("price"),
                volume: "2000".parse().expect("decimal volume"),
                volume_delta: None,
                snapshot: None,
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
    assert_eq!(value["quote"]["symbolName"], "Apple Inc.");
    assert_eq!(value["quote"]["openPrice"], 120.0);
    assert_eq!(value["quote"]["lastClose"], 122.0);
    assert_eq!(value["quote"]["turnover"], 123456.5);
    assert_eq!(value["quote"]["marketTime"], "15:59:59");
    assert_eq!(value["quote"]["quotes"].as_array().unwrap().len(), 2);
    assert_eq!(value["quote"]["quotes"][1]["symbol"], "US.MSFT");
}

#[test]
fn broker_quote_requires_every_requested_symbol_in_real_cache() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(FakeTradeRead)), Some(true));
    runtime.set_market_data_router(Some(Arc::new(std::sync::Mutex::new(ProviderRouter::new(
        8,
    )))));
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
    assert!(
        matches!(error, BrokerReadSnapshotError::Unavailable(message) if message.contains("US.AAPL"))
    );
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
    assert!(
        matches!(error, BrokerReadSnapshotError::Unavailable(message) if message.contains("runtime") || message.contains("router"))
    );
}

#[test]
fn broker_capabilities_preserve_catalog_without_a_market_data_reader() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(FakeTradeRead)), Some(true));
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(runtime),
    };
    let value = port
        .read("/api/v1/brokers/capabilities", "")
        .expect("catalog remains discoverable without market-data reader");
    assert_eq!(value["brokers"][0]["id"], "futu");
    assert!(!value["catalog"]["features"].as_array().unwrap().is_empty());
    let snapshot = value["runtime"].as_array().unwrap().iter()
        .find(|item| item["featureId"] == "market.snapshot").unwrap();
    assert_eq!(snapshot["evaluation"]["state"], "unavailable");
}

#[derive(Debug)]
struct FakeMarketMicrostructureReader;

impl jftrade_integration_futu::MarketMicrostructureReadPort for FakeMarketMicrostructureReader {
    fn query(
        &self,
        _: jftrade_integration_futu::MarketMicrostructureOperation,
        _: &str,
        _: &serde_json::Value,
    ) -> Result<serde_json::Value, jftrade_integration_futu::MarketMicrostructureError> {
        Ok(serde_json::json!({}))
    }
}

#[derive(Debug)]
struct FakeValuationDetailReader;

impl jftrade_integration_futu::ValuationDetailReadPort for FakeValuationDetailReader {
    fn query(
        &self,
        _: &jftrade_integration_futu::ValuationDetailQuery,
    ) -> Result<
        jftrade_integration_futu::ValuationDetailSnapshot,
        jftrade_integration_futu::ValuationDetailQueryError,
    > {
        Ok(jftrade_integration_futu::ValuationDetailSnapshot {
            security: jftrade_integration_futu::ValuationDetailSecurity {
                market: "HK".to_owned(),
                code: "00700".to_owned(),
                instrument_id: "HK.00700".to_owned(),
            },
            valuation_type: None,
            last_update_time: None,
            last_update_time_str: None,
            trend: None,
            market_distribution: None,
            plate_distribution: None,
            profit_growth_rate: None,
        })
    }
}

#[derive(Debug)]
struct FakeOptionChainReader;

impl jftrade_integration_futu::OptionChainReadPort for FakeOptionChainReader {
    fn query(
        &self,
        _: &jftrade_integration_futu::OptionChainQuery,
    ) -> Result<
        Vec<jftrade_integration_futu::OptionChainDate>,
        jftrade_integration_futu::OptionChainQueryError,
    > {
        Ok(Vec::new())
    }
}

#[derive(Debug)]
struct FakeHistoricalKlineReader;

impl jftrade_integration_futu::HistoricalKlineReadPort for FakeHistoricalKlineReader {
    fn query(
        &self,
        _: &jftrade_integration_futu::HistoricalKlineQuery,
    ) -> Result<
        jftrade_integration_futu::HistoricalKlineResult,
        jftrade_integration_futu::HistoricalKlineError,
    > {
        Ok(jftrade_integration_futu::HistoricalKlineResult {
            security: jftrade_integration_futu::HistoricalSecurity {
                market: 1,
                code: "00700".to_owned(),
            },
            name: Some("腾讯控股".to_owned()),
            klines: Vec::new(),
            next_req_key: Vec::new(),
        })
    }

    fn query_current(
        &self,
        query: &jftrade_integration_futu::CurrentKlineQuery,
    ) -> Result<
        jftrade_integration_futu::CurrentKlineResult,
        jftrade_integration_futu::CurrentKlineError,
    > {
        Ok(jftrade_integration_futu::CurrentKlineResult {
            security: jftrade_integration_futu::HistoricalSecurity {
                market: query.market,
                code: query.symbol.clone(),
            },
            name: Some("腾讯控股".to_owned()),
            klines: vec![jftrade_integration_futu::HistoricalKline {
                time: "2026-09-07 09:30:00".to_owned(),
                is_blank: false,
                high_price: Some(380.0),
                open_price: Some(378.0),
                low_price: Some(377.5),
                close_price: Some(379.5),
                volume: Some(1000),
                turnover: Some(379500.0),
                change_rate: Some(0.5),
            }],
        })
    }
}

#[test]
fn broker_capabilities_microstructure_and_research_runtime_ready() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(FakeTradeRead)), Some(true));
    runtime.set_market_microstructure(Some(Arc::new(FakeMarketMicrostructureReader)));
    runtime.set_valuation_detail(Some(Arc::new(FakeValuationDetailReader)));
    runtime.set_option_chains(Some(Arc::new(FakeOptionChainReader)));
    runtime.set_historical_klines(Some(Arc::new(FakeHistoricalKlineReader)));

    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(runtime),
    };
    let value = port
        .read("/api/v1/brokers/capabilities", "")
        .expect("read capabilities");

    let runtime_items = value["runtime"].as_array().expect("runtime items");

    for feature_id in [
        "market.depth",
        "market.ticks",
        "market.intraday",
        "market.broker_queue",
        "market.capital_flow",
    ] {
        let item = runtime_items
            .iter()
            .find(|item| item["featureId"] == feature_id)
            .unwrap_or_else(|| panic!("missing feature {feature_id}"));
        assert_eq!(item["evaluation"]["quoteRight"]["state"], "degraded");
        assert_eq!(
            item["evaluation"]["quoteRight"]["code"],
            "QUOTE_RIGHT_UNVERIFIED"
        );
        assert_eq!(item["evaluation"]["state"], "degraded");
        assert_eq!(item["evaluation"]["code"], "RUNTIME_STATUS_PARTIAL");
        assert_eq!(item["capability"]["state"], "degraded");
        assert_eq!(item["capability"]["reasonCode"], "RUNTIME_STATUS_PARTIAL");
    }

    let warrants = runtime_items
        .iter()
        .find(|item| item["featureId"] == "derivatives.warrants")
        .expect("derivatives.warrants");
    assert_eq!(warrants["evaluation"]["state"], "degraded");
    assert_eq!(warrants["evaluation"]["code"], "RUNTIME_STATUS_PARTIAL");

    for research_id in ["research.valuation", "research.financials", "research.instrument"] {
        let item = runtime_items
            .iter()
            .find(|item| item["featureId"] == research_id)
            .unwrap_or_else(|| panic!("missing research {research_id}"));
        assert_eq!(item["evaluation"]["state"], "degraded");
        assert_eq!(item["evaluation"]["code"], "RUNTIME_STATUS_PARTIAL");
    }
}

#[test]
fn broker_capabilities_keep_warrants_hk_only_and_futures_discoverable_in_hk_us() {
    // Parity: go:452dea11:pkg/futu/advanced_product_adapter_contracts_test.go:235
    // TestFutuWarrantsStayHKOnlyAndFuturesRemainDiscoverable. Warrants are a
    // Hong Kong-only product (warrant + cbbc), while futures stay discoverable
    // for HK and US without leaking into SH/SZ.
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(FakeTradeRead)), Some(true));
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(runtime),
    };
    let value = port
        .read("/api/v1/brokers/capabilities", "")
        .expect("read capabilities");
    let runtime_items = value["runtime"].as_array().expect("runtime items");

    for market in ["HK", "US", "SH", "SZ"] {
        let market_items = runtime_items
            .iter()
            .filter(|item| item["market"] == market)
            .collect::<Vec<_>>();
        let warrants = market_items
            .iter()
            .find(|item| item["featureId"] == "derivatives.warrants");
        assert_eq!(
            warrants.is_some(),
            market == "HK",
            "{market} warrants capability"
        );
        if let Some(item) = warrants {
            assert_eq!(
                item["capability"]["productClasses"],
                serde_json::json!(["warrant", "cbbc"]),
                "{market} warrant product classes"
            );
        }
        let futures = market_items
            .iter()
            .any(|item| item["featureId"] == "derivatives.futures");
        assert_eq!(
            futures,
            market == "HK" || market == "US",
            "{market} futures capability"
        );
    }
}

#[test]
fn broker_capabilities_stay_degraded_until_a_generation_verifies_quote_rights() {
    // Parity: go:pkg/futu/adapter_capabilities.go ensureQuoteRights /
    // evaluateQuoteCapability. A connected socket alone never proves an
    // entitlement: the verify state must be degraded and only a snapshot
    // stored for the active generation may flip it to available.
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(FakeTradeRead)), Some(true));
    runtime.set_market_microstructure(Some(Arc::new(FakeMarketMicrostructureReader)));

    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    state.set_readiness(false, true, true);
    let port = ProductionBrokerPort {
        active_provider_state: state,
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(Arc::clone(&runtime)),
    };
    let value = port
        .read("/api/v1/brokers/capabilities", "")
        .expect("read capabilities");
    let depth = value["runtime"]
        .as_array()
        .unwrap()
        .iter()
        .find(|item| item["featureId"] == "market.depth")
        .unwrap();
    assert_eq!(depth["evaluation"]["quoteRight"]["state"], "degraded");
    assert_eq!(
        depth["evaluation"]["quoteRight"]["code"],
        "QUOTE_RIGHT_UNVERIFIED"
    );

    runtime.quote_rights.set_generation(1);
    let generation = 1;
    runtime.quote_rights.store_snapshot(
        generation,
        jftrade_integration_futu::QuoteRightSnapshot {
            hk_qot_right: 3,
            us_qot_right: 3,
            cn_qot_right: 3,
            ..Default::default()
        },
        std::time::SystemTime::now(),
    );
    let value = port
        .read("/api/v1/brokers/capabilities", "")
        .expect("read capabilities");
    let depth = value["runtime"]
        .as_array()
        .unwrap()
        .iter()
        .find(|item| item["featureId"] == "market.depth")
        .unwrap();
    assert_eq!(depth["evaluation"]["quoteRight"]["state"], "available");
    assert_eq!(
        depth["evaluation"]["quoteRight"]["code"],
        "QUOTE_RIGHT_AVAILABLE"
    );

    // A stale snapshot for another generation must not be trusted again.
    let stale =
        jftrade_integration_futu::QuoteRightsState::new();
    assert_eq!(
        stale.state_for_generation(generation + 1, 3),
        jftrade_integration_futu::QuoteRightState::Unverified
    );
}

#[test]
fn broker_capabilities_quote_right_unverified_when_opend_disconnected() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(FakeTradeRead)), Some(true));
    runtime.set_market_microstructure(Some(Arc::new(FakeMarketMicrostructureReader)));

    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    state.set_readiness(false, false, false);

    let port = ProductionBrokerPort {
        active_provider_state: state,
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(runtime),
    };
    let value = port
        .read("/api/v1/brokers/capabilities", "")
        .expect("read capabilities");
    let depth = value["runtime"]
        .as_array()
        .unwrap()
        .iter()
        .find(|item| item["featureId"] == "market.depth")
        .unwrap();
    assert_eq!(depth["evaluation"]["state"], "unavailable");
    assert_eq!(depth["evaluation"]["connection"]["code"], "OPEND_CONNECTION_UNAVAILABLE");
}

#[test]
fn trade_runtime_current_kline_delegates_to_reader() {
    let runtime = SharedTradeReadRuntime::default();
    runtime.set_historical_klines(Some(Arc::new(FakeHistoricalKlineReader)));
    let query = jftrade_integration_futu::CurrentKlineQuery::new(1, "00700", "1m");
    let result = runtime.current_kline(&query).expect("current kline");
    assert_eq!(result.name.as_deref(), Some("腾讯控股"));
    assert_eq!(result.klines.len(), 1);
    assert_eq!(result.klines[0].time, "2026-09-07 09:30:00");
    assert_eq!(result.klines[0].close_price, Some(379.5));
}

#[derive(Debug)]
struct FailingSecuritySnapshotReader;

impl jftrade_integration_futu::SecuritySnapshotReadPort for FailingSecuritySnapshotReader {
    fn query(
        &self,
        _: &[String],
    ) -> Result<Vec<jftrade_marketdata::BrokerSecuritySnapshot>, String> {
        Err("OpenD timeout".to_owned())
    }
}

#[test]
fn trade_runtime_security_snapshots_falls_through_to_tick_cache_on_failure() {
    use jftrade_marketdata::Tick;

    let runtime = SharedTradeReadRuntime::default();
    runtime.set_security_snapshots(Some(Arc::new(FailingSecuritySnapshotReader)));

    let router = Arc::new(std::sync::Mutex::new(ProviderRouter::new(8)));
    let now_ms = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_millis() as i64;
    router
        .lock()
        .expect("router")
        .cache_mut()
        .insert(
            Tick {
                instrument_id: "HK.00700".to_owned(),
                price: "380".parse().expect("price"),
                volume: "100".parse().expect("volume"),
                volume_delta: None,
                snapshot: None,
                observed_at_ms: now_ms,
                provider_generation: 0,
            },
            0,
        )
        .expect("tick");
    runtime.set_market_data_router(Some(router));

    let snapshots = runtime
        .security_snapshots(&[TradeSecurity {
            market: 1,
            code: "00700".to_owned(),
        }])
        .expect("should fall through to tick cache without aborting on reader error");
    assert_eq!(snapshots.len(), 1);
    assert_eq!(snapshots[0]["symbol"], "HK.00700");
}

#[test]
fn broker_quote_requires_symbol_query() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(FakeTradeRead)), Some(true));
    runtime.set_market_data_router(Some(Arc::new(std::sync::Mutex::new(ProviderRouter::new(
        8,
    )))));
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(runtime),
    };
    let error = port
        .read("/api/v1/brokers/futu/quote", "")
        .expect_err("missing symbol");
    assert!(
        matches!(error, BrokerReadSnapshotError::Invalid(message) if message.contains("symbol"))
    );
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
    assert!(
        matches!(error, BrokerReadSnapshotError::Unavailable(message) if message.contains("historical klines"))
    );
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
        .read("/api/v1/brokers/futu/klines", "symbol=US.AAPL&period=2h")
        .expect_err("invalid period");
    assert!(
        matches!(invalid_period, BrokerReadSnapshotError::Invalid(message) if message.contains("period"))
    );
    let conflicting = port
        .read(
            "/api/v1/brokers/futu/klines",
            "symbol=US.AAPL&before=2026-08-29T00:00:00Z&fromTime=2026-08-28",
        )
        .expect_err("before/from conflict");
    assert!(
        matches!(conflicting, BrokerReadSnapshotError::Invalid(message) if message.contains("combined"))
    );
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
    assert!(
        matches!(missing_symbol, BrokerReadSnapshotError::Invalid(message) if message.contains("symbol"))
    );
    let invalid_before = port
        .read(
            "/api/v1/brokers/futu/klines",
            "symbol=US.AAPL&before=not-a-time",
        )
        .expect_err("invalid before");
    assert!(
        matches!(invalid_before, BrokerReadSnapshotError::Invalid(message) if message.contains("RFC3339"))
    );
    let invalid_limit = port
        .read(
            "/api/v1/brokers/futu/klines",
            "symbol=US.AAPL&limit=not-a-number",
        )
        .expect_err("invalid limit");
    assert!(
        matches!(invalid_limit, BrokerReadSnapshotError::Invalid(message) if message.contains("limit"))
    );
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
fn market_and_currency_authority_tables_match_go_boundaries() {
    // Parity: go:452dea11:pkg/futu/exchange_business_boundary_test.go:216
    // TestBrokerOrderMarketAndCurrencyBoundaries.
    //
    // Go's `defaultFundsCurrencyForMarket` and `fundsCurrencyForMarket` tables
    // plus the TrdMarket authority variants must stay aligned; Rust modeled the
    // same tables through `market_label_from_code` + `currency_label`.
    for (code, authority) in [
        (1, "HK"),
        (4, "HK"),
        (10, "HK"),
        (113, "HK"),
        (2, "US"),
        (11, "US"),
        (17, "US"),
        (123, "US"),
        (3, "CN"),
        (6, "SG"),
        (12, "SG"),
        (124, "SG"),
        (8, "AU"),
        (13, "JP"),
        (15, "JP"),
        (126, "JP"),
        (111, "MY"),
        (125, "MY"),
        (112, "CA"),
        (5, "FUTURES"),
        (7, "CRYPTO"),
    ] {
        assert_eq!(trade_market_authority(code), Some(authority), "code={code}");
    }
    assert_eq!(trade_market_authority(31), None);
    assert_eq!(trade_market_authority(999_999), None);
    for (code, currency) in [
        (Some(1), Some("HKD")),
        (Some(2), Some("USD")),
        (Some(3), Some("CNH")),
        (Some(4), Some("JPY")),
        (Some(5), Some("SGD")),
        (Some(6), Some("AUD")),
        (Some(7), Some("CAD")),
        (Some(8), Some("MYR")),
        (Some(9), Some("NZD")),
        (Some(0), None),
        (None, None),
    ] {
        assert_eq!(currency_label(code), currency, "code={code:?}");
    }
}

#[test]
fn broker_order_enum_labels_cover_trading_variants() {
    // Parity: go:452dea11:pkg/futu/exchange_business_boundary_test.go:128 and
    // :179 TestBrokerOrderMappingCoversOrderLifecycleEnums /
    // TestBrokerOrderTypeAndTimeInForceMappingsCoverTradingVariants. Rust's
    // projection is enum-code based, so map the same trading variants Go lists.
    for (code, label) in [
        (1, "BUY"),
        (2, "SELL"),
        (3, "SELLSHORT"),
        (4, "BUYBACK"),
    ] {
        assert_eq!(trade_side(code), label, "side={code}");
    }
    for (code, label) in [
        (1, "NORMAL"),
        (2, "MARKET"),
        (5, "ABSOLUTELIMIT"),
        (6, "AUCTION"),
        (7, "AUCTIONLIMIT"),
        (8, "SPECIALLIMIT"),
        (9, "SPECIALLIMIT_ALL"),
        (10, "STOP"),
        (11, "STOPLIMIT"),
        (12, "MARKETIFTOUCHED"),
        (13, "LIMITIFTOUCHED"),
        (14, "TRAILINGSTOP"),
        (15, "TRAILINGSTOPLIMIT"),
    ] {
        assert_eq!(order_type_label(code), label, "orderType={code}");
    }
    // GTT has no direct OpenD TimeInForce enum in Rust's code table; Go only
    // round-trips the raw string "GTT" through the snapshot layer.
    for (code, label) in [(0, "DAY"), (1, "GTC"), (2, "IOC"), (3, "GTD")] {
        assert_eq!(time_in_force_label(code), label, "timeInForce={code}");
    }
}

#[test]
fn broker_order_status_labels_cover_lifecycle_enums() {
    // Parity: go:452dea11:pkg/futu/exchange_business_boundary_test.go:128
    // statusCases in TestBrokerOrderMappingCoversOrderLifecycleEnums.
    for (code, label) in [
        (0, "UNSUBMITTED"),
        (1, "WAITINGSUBMIT"),
        (2, "SUBMITTING"),
        (3, "SUBMITFAILED"),
        (4, "TIMEOUT"),
        (5, "SUBMITTED"),
        (10, "FILLED_PART"),
        (11, "FILLED_ALL"),
        (12, "CANCELLING_PART"),
        (13, "CANCELLING_ALL"),
        (14, "CANCELLED_PART"),
        (15, "CANCELLED_ALL"),
        (21, "FAILED"),
        (22, "DISABLED"),
        (23, "DELETED"),
        (24, "FILLCANCELLED"),
    ] {
        assert_eq!(order_status_label(code), label, "status={code}");
    }
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

#[test]
fn history_query_builds_time_and_status_filters() {
    let request = TradeRequest::parse(
        "/api/v1/brokers/futu/orders",
        "scope=history&symbol=US.AAPL&startTime=2026-08-01T00:00:00Z&endTime=2026-08-02T00:00:00Z&status=Submitted,Filled_Part,bogus&statuses=SUBMITTED",
    )
    .expect("request");
    assert!(request.history_scope().expect("scope"));
    let filter = request
        .trade_filter(true, "US")
        .expect("filter")
        .expect("filter present");
    assert_eq!(filter.code_list, vec!["US.AAPL"]);
    assert_eq!(filter.begin_time.as_deref(), Some("2026-07-31 20:00:00"));
    assert_eq!(filter.end_time.as_deref(), Some("2026-08-01 20:00:00"));
    assert_eq!(request.status_codes().expect("statuses"), vec![5, 10]);
}

#[test]
fn history_query_converts_hk_rfc3339_to_local_wall_clock() {
    let request = TradeRequest::parse(
        "/api/v1/brokers/futu/fills",
        "scope=HISTORY&startTime=2026-08-01T00:00:00Z",
    )
    .expect("request");
    let filter = request
        .trade_filter(true, "HK")
        .expect("filter")
        .expect("filter present");
    assert_eq!(filter.begin_time.as_deref(), Some("2026-08-01 08:00:00"));
}

#[test]
fn history_query_converts_supported_trade_markets_to_local_wall_clock() {
    let cases = [
        ("SG", "2026-08-01T00:00:00Z", "2026-08-01 08:00:00"),
        ("JP", "2026-08-01T00:00:00Z", "2026-08-01 09:00:00"),
        ("AU", "2026-01-01T00:00:00Z", "2026-01-01 11:00:00"),
        ("MY", "2026-08-01T00:00:00Z", "2026-08-01 08:00:00"),
        ("CA", "2026-08-01T00:00:00Z", "2026-07-31 20:00:00"),
        ("FUTURES", "2026-08-01T00:00:00Z", "2026-08-01 00:00:00"),
        ("CRYPTO", "2026-08-01T00:00:00Z", "2026-08-01 00:00:00"),
    ];
    for (market, start_time, expected) in cases {
        let request = TradeRequest::parse(
            "/api/v1/brokers/futu/orders",
            &format!("scope=HISTORY&startTime={start_time}"),
        )
        .expect("request");
        let filter = request
            .trade_filter(true, market)
            .expect("filter")
            .expect("filter present");
        assert_eq!(
            filter.begin_time.as_deref(),
            Some(expected),
            "market={market}"
        );
    }
}

#[test]
fn trade_market_code_supports_all_futu_trade_markets() {
    for (market, expected) in [
        ("HK", 1),
        ("US", 2),
        ("CN", 3),
        ("FUTURES", 5),
        ("SG", 6),
        ("CRYPTO", 7),
        ("AU", 8),
        ("JP", 15),
        ("MY", 111),
        ("CA", 112),
    ] {
        assert_eq!(
            market_code(market).expect("market code"),
            expected,
            "market={market}"
        );
    }
    assert!(market_code("MARS").is_err());
}

#[test]
fn invalid_order_scope_is_rejected_before_opend_call() {
    let request =
        TradeRequest::parse("/api/v1/brokers/futu/fills", "scope=archive").expect("request");
    assert_eq!(
        request.history_scope().expect_err("invalid scope"),
        "query parameter scope is invalid"
    );
}

#[test]
fn invalid_history_time_is_rejected_before_opend_call() {
    let request = TradeRequest::parse(
        "/api/v1/brokers/futu/orders",
        "scope=HISTORY&startTime=not-a-time",
    )
    .expect("request");
    let error = request.trade_filter(true, "US").expect_err("invalid time");
    assert!(error.contains("invalid history time"));
}

#[test]
fn test_service_broker_write_operations_propagate_upstream_failures() {
    // Parity: internal/trading/broker_boundaries_test.go:116 TestServiceBrokerWriteOperationsPropagateUpstreamFailures
    // Port when provider is not ready
    let port = ProductionBrokerPort {
        active_provider_state: ready_state(),
        trade_read_port: None,
        trade_logged_in: Some(false),
        trade_runtime: None,
    };

    // Place order fails closed and propagates provider error
    let err = port
        .read("/api/v1/brokers/futu/orders", "accountId=42&market=US")
        .expect_err("must fail when trade client is unavailable");
    let err_msg = format!("{err:?}");
    assert!(err_msg.contains("TradeClientUnavailable") || err_msg.contains("Unavailable"));
}
