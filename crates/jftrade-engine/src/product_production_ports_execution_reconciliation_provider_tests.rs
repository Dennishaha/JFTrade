use super::*;

use crate::product::product_production_ports::{
    ProductionBrokerPort, ProductionPortfolioPort, SharedTradeReadRuntime,
};
use crate::product::{BrokerReadSnapshotPort, PortfolioSnapshotPort};
use jftrade_api::LiveHub;
use jftrade_integration_futu::{TradeCashInfo, TradeFunds, TradeFundsSnapshot, TradeReadPort};
use jftrade_settings::{FutuIntegrationConfig, MarketDataProvider};
use std::sync::{Arc, Mutex};

fn helper_reconciliation_port(
    provider: MarketDataProvider,
    store: Arc<jftrade_store_sqlite::ExecutionOrderStore>,
    reader: Arc<FixtureTradeReader>,
) -> ProductionExecutionPort {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    let reader_port: Arc<dyn TradeReadPort> = reader;
    runtime.set(Some(reader_port), Some(true));
    let state = Arc::new(ActiveProviderState::new(Some(provider)));
    state.set_readiness(true, true, true);
    ProductionExecutionPort {
        store,
        active_provider_state: state,
        trade_logged_in: None,
        trade_read_port: None,
        trade_write_port: None,
        trade_runtime: Some(runtime),
        cancel_inflight: Arc::new(Mutex::new(std::collections::BTreeSet::new())),
        risk_coordinator: None,
        default_trading_environment: None,
        notification_projector: None,
    }
}

fn projection_funds() -> TradeFundsSnapshot {
    TradeFundsSnapshot {
        header: jftrade_integration_futu::trade_header(1, 42, 2),
        funds: TradeFunds {
            power: 500.0,
            total_assets: 1_000.0,
            cash: 321.0,
            market_val: 679.0,
            frozen_cash: 0.0,
            debt_cash: 0.0,
            avl_withdrawal_cash: 300.0,
            currency: Some(2),
            available_funds: Some(500.0),
            unrealized_pl: None,
            realized_pl: None,
            risk_level: None,
            initial_margin: None,
            maintenance_margin: None,
            cash_info_list: vec![TradeCashInfo {
                currency: Some(2),
                cash: Some(321.0),
                available_balance: Some(300.0),
                net_cash_power: Some(500.0),
            }],
            max_power_short: None,
            net_cash_power: Some(500.0),
            long_mv: None,
            short_mv: None,
            pending_asset: None,
            max_withdrawal: Some(300.0),
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
    }
}

fn projection_reader() -> Arc<FixtureTradeReader> {
    let mut account = account();
    account.trd_market_auth_list = vec![2];
    Arc::new(FixtureTradeReader {
        accounts: vec![account],
        active_orders: vec![order_snapshot(11, Some(5.0))],
        history_orders: vec![order_snapshot(10, Some(3.0))],
        active_fills: vec![fill("2026-08-31T01:00:00Z", 2.0, "active-fill")],
        history_fills: vec![fill("2026-08-31T02:00:00Z", 3.0, "history-fill")],
        fees: vec![fee(1.5)],
        funds: Some(projection_funds()),
        ..FixtureTradeReader::default()
    })
}

fn projection_ports(
    provider: MarketDataProvider,
    reader: Arc<FixtureTradeReader>,
    store: Arc<jftrade_store_sqlite::ExecutionOrderStore>,
) -> (ProductionBrokerPort, ProductionPortfolioPort) {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    let reader_port: Arc<dyn TradeReadPort> = reader;
    runtime.set(Some(reader_port), Some(true));
    runtime.set_runtime_projection(
        &FutuIntegrationConfig::current_default(),
        Some(Arc::new(LiveHub::default())),
        4,
    );
    let state = Arc::new(ActiveProviderState::new(Some(provider)));
    // helper readiness and OpenD readiness are independent: the test models
    // yfinance/AKShare owning quotes while Futu owns the trade session.
    state.set_readiness(true, true, true);
    let broker = ProductionBrokerPort {
        active_provider_state: Arc::clone(&state),
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(Arc::clone(&runtime)),
    };
    let portfolio = ProductionPortfolioPort {
        active_provider_state: state,
        _execution_store: store,
        trade_read_port: None,
        trade_logged_in: None,
        trade_runtime: Some(runtime),
    };
    (broker, portfolio)
}

#[test]
fn helper_market_data_providers_reconcile_futu_account_order_fill_and_fee() {
    for provider in [MarketDataProvider::Yfinance, MarketDataProvider::Akshare] {
        let (store, _directory) = reconciliation_store();
        store
            .save_order(pending_order("SUBMITTED"), "2026-08-30T00:00:01Z")
            .expect("save pending order");
        let reader = Arc::new(FixtureTradeReader {
            accounts: vec![account()],
            history_orders: vec![order_snapshot(11, Some(5.0))],
            history_fills: vec![fill("2026-08-31T00:00:00Z", 5.0, "provider-fill")],
            fees: vec![fee(1.5)],
            ..FixtureTradeReader::default()
        });
        let port = helper_reconciliation_port(provider, Arc::clone(&store), reader);

        assert_eq!(port.reconcile_pending_orders().expect("provider scan"), 1);
        let saved = store
            .get_order("rust-order-reconcile")
            .expect("reload order")
            .expect("order exists");
        assert_eq!(saved.status, "FILLED");
        assert_eq!(saved.filled_quantity, Some(5.0));
        assert_eq!(saved.fees, Some(1.5));
    }
}

#[test]
fn helper_market_data_providers_fail_closed_without_futu_trade_session() {
    for provider in [MarketDataProvider::Yfinance, MarketDataProvider::Akshare] {
        let (store, _directory) = reconciliation_store();
        let state = Arc::new(ActiveProviderState::new(Some(provider)));
        state.set_readiness(true, true, true);
        let broker = ProductionBrokerPort {
            active_provider_state: Arc::clone(&state),
            trade_read_port: None,
            trade_logged_in: None,
            trade_runtime: None,
        };
        let error = broker
            .read(
                "/api/v1/brokers/futu/funds",
                "accountId=42&tradingEnvironment=REAL&market=US",
            )
            .expect_err("missing Futu trade session");
        assert!(error.to_string().contains("trade session"));

        let portfolio = ProductionPortfolioPort {
            active_provider_state: state,
            _execution_store: store,
            trade_read_port: None,
            trade_logged_in: None,
            trade_runtime: None,
        };
        let error = portfolio
            .read(
                "/api/v1/portfolio/futu/cash-balances",
                "accountId=42&tradingEnvironment=REAL&market=US",
            )
            .expect_err("portfolio must fail closed without Futu session");
        assert!(error.to_string().contains("trade session"));
    }
}

#[test]
fn reconciliation_persists_unknown_broker_status_once() {
    let (store, _directory) = reconciliation_store();
    store
        .save_order(pending_order("SUBMITTED"), "2026-08-30T00:00:01Z")
        .expect("save pending order");
    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        history_orders: vec![order_snapshot(999, None)],
        ..FixtureTradeReader::default()
    });
    let port = helper_reconciliation_port(MarketDataProvider::Akshare, Arc::clone(&store), reader);

    let error = port
        .reconcile_pending_orders()
        .expect_err("unknown broker status must fail closed");
    assert!(error.contains("BROKER_STATUS_UNKNOWN"));
    let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
    assert_eq!(saved.status, "UNKNOWN");
    assert_eq!(
        saved.last_error_code.as_deref(),
        Some("BROKER_STATUS_UNKNOWN")
    );
    assert_eq!(
        store
            .list_order_events("rust-order-reconcile")
            .unwrap()
            .len(),
        1
    );

    let second = port
        .reconcile_pending_orders()
        .expect_err("unknown status remains retryable");
    assert!(second.contains("BROKER_STATUS_UNKNOWN"));
    assert_eq!(
        store
            .list_order_events("rust-order-reconcile")
            .unwrap()
            .len(),
        1
    );
}

#[test]
fn reconciliation_deduplicates_same_fill_from_active_and_history() {
    let (store, _directory) = reconciliation_store();
    store
        .save_order(pending_order("SUBMITTED"), "2026-08-30T00:00:01Z")
        .expect("save pending order");
    let duplicate = fill("2026-08-31T00:00:00Z", 2.0, "duplicate-fill");
    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        history_orders: vec![order_snapshot(10, None)],
        active_fills: vec![duplicate.clone()],
        history_fills: vec![duplicate],
        ..FixtureTradeReader::default()
    });
    let port = helper_reconciliation_port(MarketDataProvider::Yfinance, Arc::clone(&store), reader);

    assert_eq!(
        port.reconcile_pending_orders().expect("deduplicated scan"),
        1
    );
    let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
    assert_eq!(saved.status, "PARTIALLY_FILLED");
    assert_eq!(saved.filled_quantity, Some(2.0));
    assert_eq!(
        store
            .list_order_events("rust-order-reconcile")
            .unwrap()
            .len(),
        2
    );
}

#[test]
fn helper_market_data_providers_project_futu_broker_and_portfolio_routes() {
    for provider in [MarketDataProvider::Yfinance, MarketDataProvider::Akshare] {
        let (store, _directory) = reconciliation_store();
        let reader = projection_reader();
        let reader_for_assertions = Arc::clone(&reader);
        let (broker, portfolio) = projection_ports(provider, reader, Arc::clone(&store));
        let query = "accountId=42&tradingEnvironment=REAL&market=US";

        let runtime = broker
            .read("/api/v1/brokers/futu/runtime", "")
            .expect("account projection");
        assert_eq!(runtime["accounts"][0]["accountId"], "42");
        assert_eq!(runtime["session"]["tradeLoggedIn"], true);

        let funds = broker
            .read("/api/v1/brokers/futu/funds", query)
            .expect("broker funds projection");
        assert_eq!(funds["summary"]["cash"], 321.0);

        let orders = broker
            .read("/api/v1/brokers/futu/orders", query)
            .expect("active order projection");
        assert_eq!(orders["orders"][0]["status"], "FILLED_ALL");
        let history_orders = broker
            .read(
                "/api/v1/brokers/futu/orders",
                &format!("{query}&scope=history"),
            )
            .expect("history order projection");
        assert_eq!(history_orders["orders"][0]["status"], "FILLED_PART");

        let fills = broker
            .read("/api/v1/brokers/futu/fills", query)
            .expect("active fill projection");
        assert_eq!(fills["fills"][0]["brokerFillIdEx"], "active-fill");
        let history_fills = broker
            .read(
                "/api/v1/brokers/futu/fills",
                &format!("{query}&scope=history"),
            )
            .expect("history fill projection");
        assert_eq!(history_fills["fills"][0]["brokerFillIdEx"], "history-fill");

        let fees = broker
            .read(
                "/api/v1/brokers/futu/order-fees",
                &format!("{query}&orderIdEx=order-ex"),
            )
            .expect("fee projection");
        assert_eq!(fees["fees"][0]["feeAmount"], 1.5);

        let balances = portfolio
            .read("/api/v1/portfolio/futu/cash-balances", query)
            .expect("portfolio cash projection");
        assert_eq!(balances["balances"][0]["cashBalance"], 321.0);

        let calls = reader_for_assertions.calls.lock().expect("fixture calls");
        assert!(calls.accounts >= 7, "all projections must discover account");
        assert!(calls.active_orders >= 1);
        assert!(calls.history_orders >= 1);
        assert!(calls.active_fills >= 1);
        assert!(calls.history_fills >= 1);
        assert!(calls.fees >= 1);
    }
}

#[test]
fn reconciliation_fill_write_rolls_back_when_event_id_conflicts() {
    let (store, _directory) = reconciliation_store();
    store
        .save_order(pending_order("SUBMITTED"), "2026-08-30T00:00:01Z")
        .expect("save pending order");
    let conflict = StoredExecutionOrderEvent {
        id: "rust-order-reconcile-fill-1",
        internal_order_id: "rust-order-reconcile",
        event_type: "fixture",
        previous_status: Some("SUBMITTED"),
        next_status: "SUBMITTED",
        payload_json: "{}",
        created_at: "2026-08-30T00:00:02Z",
    };
    store
        .record_event(&conflict)
        .expect("record conflict fixture");
    let port = helper_reconciliation_port(
        MarketDataProvider::Akshare,
        Arc::clone(&store),
        Arc::new(FixtureTradeReader::default()),
    );
    let current = store.get_order("rust-order-reconcile").unwrap().unwrap();

    let error = port
        .apply_fill_snapshot(
            &current,
            &fill("2026-08-31T00:00:00Z", 1.0, "rollback-fill"),
            1,
        )
        .expect_err("duplicate event id must roll back the transition");
    assert!(matches!(
        error,
        ExecutionWritePortError::Failed {
            status: 500,
            ref code,
            ..
        } if code == "EXECUTION_STORE_ERROR"
    ));
    let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
    assert_eq!(saved.status, "SUBMITTED");
    assert_eq!(saved.filled_quantity, None);
    assert_eq!(
        store
            .list_order_events("rust-order-reconcile")
            .unwrap()
            .len(),
        1
    );
}
