use serde_json::{Value, json};

use jftrade_backtest::run_json;

fn candle(index: usize, close: &str, volume: &str) -> Value {
    let minute = 30 + index;
    json!({
        "start": format!("2026-07-21T13:{minute:02}:00Z"),
        "end": format!("2026-07-21T13:{minute:02}:59.999Z"),
        "open": close,
        "high": close,
        "low": close,
        "close": close,
        "volume": volume,
    })
}

fn run_case(candles: Vec<Value>, intents: Vec<Value>) -> Value {
    let input = json!({
        "version": 1,
        "cases": [{
            "id": "result-reporting",
            "symbol": "US.AAPL",
            "baseCurrency": "AAPL",
            "quoteCurrency": "USD",
            "initialBalance": "10000",
            "processOrdersOnClose": true,
            "market": {
                "tickSize": "0.01",
                "quantityStep": "1",
                "minQuantity": "1"
            },
            "candles": candles,
            "intents": intents
        }]
    });
    let output: Value = serde_json::from_slice(
        &run_json(&serde_json::to_vec(&input).expect("encode input")).expect("run case"),
    )
    .expect("decode output");
    output["cases"][0].clone()
}

fn submit(bar_index: usize, id: &str, side: &str, quantity: &str) -> Value {
    json!({
        "barIndex": bar_index,
        "action": "submit",
        "id": id,
        "side": side,
        "orderType": "market",
        "quantity": quantity,
    })
}

fn cancel(bar_index: usize, target_id: &str) -> Value {
    json!({
        "barIndex": bar_index,
        "action": "cancel",
        "id": format!("cancel-{target_id}"),
        "targetId": target_id,
    })
}

#[test]
fn weighted_position_cost_reports_realized_loss_for_a_long_close() {
    let result = run_case(
        vec![
            candle(0, "100", "1000"),
            candle(1, "110", "1000"),
            candle(2, "105", "1000"),
        ],
        vec![
            submit(0, "buy-100", "buy", "2"),
            submit(1, "buy-110", "buy", "3"),
            submit(2, "sell-105", "sell", "5"),
        ],
    );

    assert_eq!(result["totalFills"], 3);
    assert_eq!(result["totalTrades"], 1);
    assert_eq!(result["winningTrades"], 0);
    assert_eq!(result["winRate"], "0");
    assert_eq!(result["realizedPnl"], "-5");
    assert_eq!(result["basePosition"], "0");
    assert_eq!(result["finalEquity"], "9995");

    let fills = result["fills"].as_array().expect("fills array");
    assert_eq!(fills[0]["realizedPnl"], "0");
    assert_eq!(fills[1]["realizedPnl"], "0");
    assert_eq!(fills[2]["price"], "105");
    assert_eq!(fills[2]["quantity"], "5");
    assert_eq!(fills[2]["realizedPnl"], "-5");
}

#[test]
fn short_covers_and_reversals_count_each_closing_order_once() {
    let short_cover = run_case(
        vec![
            candle(0, "100", "1000"),
            candle(1, "90", "1000"),
            candle(2, "110", "1000"),
        ],
        vec![
            submit(0, "short-entry", "sell", "2"),
            submit(1, "profitable-cover", "buy", "1"),
            submit(2, "losing-cover", "buy", "1"),
        ],
    );
    assert_eq!(short_cover["totalTrades"], 2);
    assert_eq!(short_cover["winningTrades"], 1);
    assert_eq!(short_cover["winRate"], "0.5");
    assert_eq!(short_cover["realizedPnl"], "0");
    let short_fills = short_cover["fills"].as_array().expect("short fills");
    assert_eq!(short_fills[1]["realizedPnl"], "10");
    assert_eq!(short_fills[2]["realizedPnl"], "-10");

    let reversal = run_case(
        vec![
            candle(0, "100", "1000"),
            candle(1, "90", "1000"),
            candle(2, "80", "1000"),
        ],
        vec![
            submit(0, "long-entry", "buy", "3"),
            submit(1, "sell-reversal", "sell", "5"),
            submit(2, "short-cover", "buy", "2"),
        ],
    );
    assert_eq!(reversal["totalTrades"], 2);
    assert_eq!(reversal["winningTrades"], 1);
    assert_eq!(reversal["winRate"], "0.5");
    assert_eq!(reversal["realizedPnl"], "-10");
    assert_eq!(reversal["basePosition"], "0");
    let reversal_fills = reversal["fills"].as_array().expect("reversal fills");
    assert_eq!(reversal_fills[1]["realizedPnl"], "-30");
    assert_eq!(reversal_fills[2]["realizedPnl"], "20");
}

#[test]
fn partial_closing_fills_aggregate_pnl_before_trade_finalization() {
    let result = run_case(
        vec![
            candle(0, "100", "1000"),
            candle(1, "110", "40"),
            candle(2, "100", "60"),
        ],
        vec![
            submit(0, "long-entry", "buy", "10"),
            submit(1, "partial-close", "sell", "10"),
        ],
    );

    assert_eq!(result["totalFills"], 3);
    assert_eq!(result["totalTrades"], 1);
    assert_eq!(result["winningTrades"], 1);
    assert_eq!(result["winRate"], "1");
    assert_eq!(result["realizedPnl"], "40");
    assert_eq!(result["orders"][1]["status"], "FILLED");
    assert_eq!(result["orders"][1]["filledQuantity"], "10");
    assert_eq!(result["orders"][1]["filledPrice"], "104");

    let fills = result["fills"].as_array().expect("fills array");
    assert_eq!(fills[1]["quantity"], "4");
    assert_eq!(fills[1]["price"], "110");
    assert_eq!(fills[1]["realizedPnl"], "40");
    assert_eq!(fills[2]["quantity"], "6");
    assert_eq!(fills[2]["price"], "100");
    assert_eq!(fills[2]["realizedPnl"], "0");
}

#[test]
fn pending_partial_closing_order_is_finalized_at_run_boundary() {
    let result = run_case(
        vec![candle(0, "100", "1000"), candle(1, "110", "40")],
        vec![
            submit(0, "long-entry", "buy", "10"),
            submit(1, "partial-close", "sell", "10"),
        ],
    );

    assert_eq!(result["totalFills"], 2);
    assert_eq!(result["totalTrades"], 1);
    assert_eq!(result["winningTrades"], 1);
    assert_eq!(result["winRate"], "1");
    assert_eq!(result["realizedPnl"], "40");
    assert_eq!(result["basePosition"], "6");
    assert_eq!(result["orders"][1]["status"], "PARTIALLY_FILLED");
    assert_eq!(result["orders"][1]["filledQuantity"], "4");
}

#[test]
fn partial_closing_cancellation_finalizes_the_filled_segment() {
    let result = run_case(
        vec![candle(0, "100", "1000"), candle(1, "90", "40")],
        vec![
            submit(0, "long-entry", "buy", "10"),
            submit(1, "partial-close", "sell", "10"),
            cancel(1, "partial-close"),
        ],
    );

    assert_eq!(result["totalFills"], 2);
    assert_eq!(result["totalTrades"], 1);
    assert_eq!(result["winningTrades"], 0);
    assert_eq!(result["winRate"], "0");
    assert_eq!(result["realizedPnl"], "-40");
    assert_eq!(result["basePosition"], "6");
    assert_eq!(result["orders"][1]["status"], "CANCELED");
    assert_eq!(result["orders"][1]["filledQuantity"], "4");
    assert_eq!(result["orders"][1]["filledPrice"], "90");
}

#[test]
fn equity_report_tracks_peak_maximum_and_current_drawdown() {
    let result = run_case(
        vec![
            candle(0, "100", "1000"),
            candle(1, "120", "1000"),
            candle(2, "90", "1000"),
            candle(3, "110", "1000"),
        ],
        vec![
            submit(0, "long-entry", "buy", "1"),
            submit(3, "long-exit", "sell", "1"),
        ],
    );

    assert_eq!(result["equityCurve"][0]["equity"], "10000");
    assert_eq!(result["equityCurve"][1]["equity"], "10020");
    assert_eq!(result["equityCurve"][2]["equity"], "9990");
    assert_eq!(result["equityCurve"][3]["equity"], "10010");
    assert_eq!(result["maxDrawdown"], "0.002994011976");
    assert_eq!(result["currentDrawdown"], "0.000998003992");
    assert_eq!(result["drawdownCurve"][2]["drawdown"], "0.002994011976");
    assert_eq!(result["drawdownCurve"][3]["drawdown"], "0.000998003992");
}

// Deliberate migration gaps from the reviewed Go tests:
// - RunResult.Snapshot/AddRuntimeError depend on a mutable concurrent Go DTO;
//   this Rust leaf returns owned, immutable-by-default corpus values and has no
//   runtime-error sample/count fields to clone.
// - warmupUntil/order warmup flags, QueryAccount finalization, and
//   deriveStrategyWarmupCandles/session-scope/replay-capacity helpers belong to
//   the Go collector/runtime boundary and are absent from the Rust corpus
//   contract. They are not approximated here.
