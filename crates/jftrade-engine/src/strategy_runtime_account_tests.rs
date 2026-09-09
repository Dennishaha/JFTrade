use super::*;
use jftrade_integration_futu::*;

#[derive(Default)]
struct Accounts {
    headers: Mutex<Vec<TradeHeader>>,
}
impl TradeReadPort for Accounts {
    fn read_accounts(
        &self,
        _: u64,
        _: Option<i32>,
        _: Option<bool>,
    ) -> Result<Vec<TradeAccountSnapshot>, TradeSessionError> {
        Ok(vec![])
    }
    fn read_funds(
        &self,
        h: TradeHeader,
        _: Option<bool>,
        _: Option<i32>,
        _: Option<i32>,
    ) -> Result<TradeFundsSnapshot, TradeSessionError> {
        self.headers.lock().unwrap().push(h.clone());
        Ok(TradeFundsSnapshot{header:h,funds:serde_json::from_value(json!({"power":2400,"total_assets":9900,"cash":2400,"market_val":7500,"frozen_cash":0,"debt_cash":0,"avl_withdrawal_cash":2400,"available_funds":2400,"cash_info_list":[],"market_info_list":[]})).unwrap()})
    }
    fn read_cash_flows(
        &self,
        _: TradeHeader,
        _: String,
        _: Option<i32>,
    ) -> Result<Vec<TradeCashFlowSnapshot>, TradeSessionError> {
        Ok(vec![])
    }
    fn read_order_fees(
        &self,
        _: TradeHeader,
        _: Vec<String>,
    ) -> Result<Vec<TradeOrderFeeSnapshot>, TradeSessionError> {
        Ok(vec![])
    }
    fn read_margin_ratios(
        &self,
        _: TradeHeader,
        _: Vec<TradeSecurity>,
    ) -> Result<Vec<TradeMarginRatioSnapshot>, TradeSessionError> {
        Ok(vec![])
    }
    fn read_max_trade_quantity(
        &self,
        _: TradeMaxTradeQuantityRequest,
    ) -> Result<TradeMaxTradeQuantitySnapshot, TradeSessionError> {
        Err(TradeSessionError::Unsupported("not used".into()))
    }
    fn read_positions(
        &self,
        h: TradeHeader,
        _: Option<TradeFilter>,
        _: Option<f64>,
        _: Option<f64>,
        _: Option<bool>,
        _: Option<i32>,
        _: Option<i32>,
        _: Option<bool>,
    ) -> Result<Vec<TradePositionSnapshot>, TradeSessionError> {
        self.headers.lock().unwrap().push(h);
        Ok(vec![serde_json::from_value(json!({"position_id":1,"position_side":1,"code":"AAPL","name":"Apple","qty":50,"can_sell_qty":40,"price":150,"val":7500,"pl_val":0})).unwrap()])
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
    ) -> Result<Vec<TradeFillSnapshot>, TradeSessionError> {
        Ok(vec![])
    }
}

#[test]
fn simulated_broker_account_supplies_real_balance_and_sellable_positions() {
    let account = Accounts::default();
    let binding = json!({"brokerAccount":{"brokerId":"futu","accountId":"42","tradingEnvironment":"SIMULATE"}});
    let snapshot = strategy_account_snapshot(&account, &binding, "US", "AAPL").unwrap();
    assert_eq!(snapshot.available_cash, Some(2400.0));
    assert_eq!(snapshot.current_position, Some(50.0));
    assert_eq!(snapshot.sellable_quantity, Some(40.0));
    let headers = account.headers.lock().unwrap();
    assert_eq!(headers.len(), 2);
    assert!(
        headers
            .iter()
            .all(|h| h.trd_env == 0 && h.acc_id == 42 && h.trd_market == 11)
    );
}
