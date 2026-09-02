use serde_json::{Value, json};

use jftrade_backtest::run_json;

fn candle(index: usize, open: &str, close: &str, volume: &str) -> Value {
    let minute = 30 + index;
    json!({
        "start": format!("2026-07-22T13:{minute:02}:00Z"),
        "end": format!("2026-07-22T13:{minute:02}:59.999Z"),
        "open": open,
        "high": open,
        "low": open,
        "close": close,
        "volume": volume,
    })
}

fn run_case(case: Value) -> Result<Value, String> {
    let input = json!({
        "version": 1,
        "cases": [case],
    });
    let encoded = serde_json::to_vec(&input).map_err(|error| error.to_string())?;
    let output = run_json(&encoded).map_err(|error| error.to_string())?;
    let output: Value = serde_json::from_slice(&output).map_err(|error| error.to_string())?;
    output
        .get("cases")
        .and_then(Value::as_array)
        .and_then(|cases| cases.first())
        .cloned()
        .ok_or_else(|| "backtest output did not contain a case".to_owned())
}

fn base_case(id: &str, candles: Vec<Value>, intents: Vec<Value>) -> Value {
    json!({
        "id": id,
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "100000",
        "market": {
            "tickSize": "0.01",
            "quantityStep": "1",
            "minQuantity": "1"
        },
        "candles": candles,
        "intents": intents,
    })
}

fn submit(bar_index: usize, id: &str, side: &str) -> Value {
    json!({
        "barIndex": bar_index,
        "action": "submit",
        "id": id,
        "side": side,
        "orderType": "market",
        "quantity": "1",
    })
}

#[test]
fn sell_side_fee_uses_max_amount_cap_without_charging_buy_fills() {
    let mut case = base_case(
        "sell-fee-cap",
        vec![
            candle(0, "100", "100", "1000"),
            candle(1, "100", "100", "1000"),
            candle(2, "100", "100", "1000"),
        ],
        vec![submit(0, "entry", "buy"), submit(1, "exit", "sell")],
    );
    case["feeRules"] = json!([{
        "id": "sell-cap",
        "label": "Sell cap",
        "group": "broker",
        "side": " SELL ",
        "basis": " NOTIONAL ",
        "rate": "0.02",
        "maxAmount": "1.5"
    }]);

    let result = run_case(case).expect("run capped sell fee case");
    assert_eq!(result["totalBrokerFees"], "1.5");
    assert_eq!(result["totalMarketFees"], "0");
    assert_eq!(result["totalFees"], "1.5");
    assert_eq!(result["cash"], "99998.5");
    assert_eq!(result["basePosition"], "0");
    let fills = result["fills"].as_array().expect("fills array");
    assert_eq!(fills.len(), 2);
    assert_eq!(fills[0]["side"], "buy");
    assert_eq!(fills[0]["brokerFee"], "0");
    assert_eq!(fills[1]["side"], "sell");
    assert_eq!(fills[1]["brokerFee"], "1.5");
    let breakdown = result["feeBreakdown"].as_array().expect("fee breakdown");
    assert_eq!(breakdown.len(), 1);
    assert_eq!(breakdown[0]["ruleId"], "sell-cap");
    assert_eq!(breakdown[0]["amount"], "1.5");
    assert_eq!(breakdown[0]["count"], 1);
}

#[test]
fn per_order_fee_is_charged_once_for_each_distinct_order() {
    let mut case = base_case(
        "distinct-order-fees",
        vec![
            candle(0, "100", "100", "1000"),
            candle(1, "100", "100", "1000"),
            candle(2, "100", "100", "1000"),
        ],
        vec![submit(0, "first", "buy"), submit(1, "second", "buy")],
    );
    case["feeRules"] = json!([{
        "id": "per-order",
        "label": "Per order",
        "group": "broker",
        "side": "both",
        "basis": "order",
        "fixedAmount": "2"
    }]);

    let result = run_case(case).expect("run distinct order fee case");
    assert_eq!(result["totalFills"], 2);
    assert_eq!(result["totalBrokerFees"], "4");
    assert_eq!(result["totalFees"], "4");
    let fills = result["fills"].as_array().expect("fills array");
    assert_eq!(fills[0]["clientOrderId"], "first");
    assert_eq!(fills[0]["totalFee"], "2");
    assert_eq!(fills[1]["clientOrderId"], "second");
    assert_eq!(fills[1]["totalFee"], "2");
    let breakdown = result["feeBreakdown"].as_array().expect("fee breakdown");
    assert_eq!(breakdown.len(), 1);
    assert_eq!(breakdown[0]["amount"], "4");
    assert_eq!(breakdown[0]["count"], 2);
}

#[test]
fn cancellation_before_first_bar_reports_zero_trade_metrics_and_empty_curves() {
    let mut case = base_case(
        "cancel-before-first-bar",
        vec![candle(0, "100", "100", "1000")],
        Vec::new(),
    );
    case["initialBalance"] = json!("500");
    case["cancelBeforeBar"] = json!(0);

    let result = run_case(case).expect("run cancelled case");
    assert_eq!(result["status"], "cancelled");
    assert_eq!(result["processedBars"], 0);
    assert_eq!(result["cash"], "500");
    assert_eq!(result["basePosition"], "0");
    assert_eq!(result["finalEquity"], "500");
    assert_eq!(result["realizedPnl"], "0");
    assert_eq!(result["totalFills"], 0);
    assert_eq!(result["totalTrades"], 0);
    assert_eq!(result["winningTrades"], 0);
    assert_eq!(result["winRate"], "0");
    assert_eq!(result["maxDrawdown"], "0");
    assert_eq!(result["currentDrawdown"], "0");
    assert!(
        result["equityCurve"]
            .as_array()
            .expect("equity curve")
            .is_empty()
    );
    assert!(
        result["drawdownCurve"]
            .as_array()
            .expect("drawdown curve")
            .is_empty()
    );
}

#[test]
fn flat_no_trade_run_preserves_equity_and_zero_drawdown() {
    let result = run_case(base_case(
        "flat-no-trades",
        vec![
            candle(0, "100", "100", "1000"),
            candle(1, "100", "100", "1000"),
        ],
        Vec::new(),
    ))
    .expect("run flat case");

    assert_eq!(result["status"], "completed");
    assert_eq!(result["processedBars"], 2);
    assert_eq!(result["finalEquity"], "100000");
    assert_eq!(result["totalFills"], 0);
    assert_eq!(result["totalTrades"], 0);
    assert_eq!(result["winningTrades"], 0);
    assert_eq!(result["winRate"], "0");
    assert_eq!(result["maxDrawdown"], "0");
    assert_eq!(result["currentDrawdown"], "0");
    let equity = result["equityCurve"].as_array().expect("equity curve");
    assert_eq!(equity.len(), 2);
    assert_eq!(equity[0]["equity"], "100000");
    assert_eq!(equity[1]["equity"], "100000");
    let drawdown = result["drawdownCurve"].as_array().expect("drawdown curve");
    assert_eq!(drawdown.len(), 2);
    assert!(drawdown.iter().all(|point| point["drawdown"] == "0"));
}
