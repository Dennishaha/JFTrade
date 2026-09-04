use std::sync::Arc;

use jftrade_integration_pine::{
    BacktestExecutionCandle, BacktestExecutionPort, BacktestExecutionRequest,
    PineBacktestExecutionAdapter, PineExecutionFuture, PineExecutionPort, PineOrderIntent,
    PineRunRequest, PineRunResult,
};
use serde_json::json;

#[derive(Clone, Debug)]
struct StaticMockPinePort {
    intents: Vec<PineOrderIntent>,
}

impl PineExecutionPort for StaticMockPinePort {
    fn run<'a>(&'a self, _request: PineRunRequest) -> PineExecutionFuture<'a> {
        let result = Ok(PineRunResult {
            order_intents: self.intents.clone(),
            ..Default::default()
        });
        Box::pin(async move { result })
    }
}

fn sample_candle(index: usize, start_ms: i64) -> BacktestExecutionCandle {
    BacktestExecutionCandle {
        start_time: start_ms + (index as i64) * 60_000,
        end_time: start_ms + (index as i64) * 60_000 + 59_999,
        open: "100.0".to_owned(),
        high: "105.0".to_owned(),
        low: "95.0".to_owned(),
        close: "102.0".to_owned(),
        volume: "100.0".to_owned(),
    }
}

#[test]
fn close_all_normalized_exit_intent_flattens_multi_entry_position_in_matcher() {
    let candles = (0..3)
        .map(|i| sample_candle(i, 1_700_000_000_000))
        .collect();
    let intents = vec![
        PineOrderIntent {
            id: "entry_A".to_owned(),
            kind: "entry".to_owned(),
            direction: "long".to_owned(),
            has_quantity: true,
            quantity: 3.0,
            bar_index: 0,
            ..Default::default()
        },
        PineOrderIntent {
            id: "entry_B".to_owned(),
            kind: "entry".to_owned(),
            direction: "long".to_owned(),
            has_quantity: true,
            quantity: 4.0,
            bar_index: 1,
            ..Default::default()
        },
        // Worker normalizes strategy.close_all() into kind "exit", quantity = total open position (7.0)
        PineOrderIntent {
            id: "close_all".to_owned(),
            kind: "exit".to_owned(),
            direction: "long".to_owned(),
            has_quantity: true,
            quantity: 7.0,
            bar_index: 2,
            ..Default::default()
        },
    ];

    let adapter = PineBacktestExecutionAdapter::new(Arc::new(StaticMockPinePort { intents }));
    let request = BacktestExecutionRequest {
        run_id: "test-close-all".to_owned(),
        payload: json!({
            "strategyScript": "strategy('test')",
            "symbol": "US.AAPL",
            "interval": "1m",
            "processOrdersOnClose": true,
            "initialBalance": "100000",
        }),
        market_data_provider: "yfinance".to_owned(),
        candles,
    };

    let result = adapter.execute(request).expect("execute backtest");
    let case = &result["cases"][0];
    assert_eq!(case["status"], "completed");
    assert_eq!(case["processedBars"], 3);
    // Base position must be completely flattened back to 0
    assert_eq!(case["basePosition"], "0");
    // Three orders were processed (2 entries, 1 close_all)
    let orders = case["orders"].as_array().expect("orders array");
    assert_eq!(orders.len(), 3);
    assert_eq!(orders[2]["clientOrderId"], "close_all");
}

#[test]
fn parameterless_close_normalized_exit_intent_flattens_named_entry() {
    let candles = (0..2)
        .map(|i| sample_candle(i, 1_700_000_000_000))
        .collect();
    let intents = vec![
        PineOrderIntent {
            id: "LongEntry".to_owned(),
            kind: "entry".to_owned(),
            direction: "long".to_owned(),
            has_quantity: true,
            quantity: 5.0,
            bar_index: 0,
            ..Default::default()
        },
        // Worker normalizes strategy.close("LongEntry") into kind "exit", quantity = 5.0
        PineOrderIntent {
            id: "close_LongEntry".to_owned(),
            kind: "exit".to_owned(),
            direction: "long".to_owned(),
            has_quantity: true,
            quantity: 5.0,
            bar_index: 1,
            ..Default::default()
        },
    ];

    let adapter = PineBacktestExecutionAdapter::new(Arc::new(StaticMockPinePort { intents }));
    let request = BacktestExecutionRequest {
        run_id: "test-scoped-close".to_owned(),
        payload: json!({
            "strategyScript": "strategy('test')",
            "symbol": "US.AAPL",
            "interval": "1m",
            "processOrdersOnClose": true,
            "initialBalance": "50000",
        }),
        market_data_provider: "yfinance".to_owned(),
        candles,
    };

    let result = adapter.execute(request).expect("execute backtest");
    let case = &result["cases"][0];
    assert_eq!(case["status"], "completed");
    assert_eq!(case["basePosition"], "0");
}

#[test]
fn cancel_all_diff_emitted_cancels_cleanly_cancel_pending_orders() {
    let candles = (0..3)
        .map(|i| sample_candle(i, 1_700_000_000_000))
        .collect();
    let intents = vec![
        PineOrderIntent {
            id: "limit_order_1".to_owned(),
            kind: "order".to_owned(),
            direction: "long".to_owned(),
            has_quantity: true,
            quantity: 2.0,
            has_limit_price: true,
            limit_price: 50.0, // Far below low (95.0), will stay pending
            bar_index: 0,
            ..Default::default()
        },
        // Worker emits cancel intent for the vanished order upon strategy.cancel_all()
        PineOrderIntent {
            id: "limit_order_1".to_owned(),
            kind: "cancel".to_owned(),
            bar_index: 1,
            ..Default::default()
        },
    ];

    let adapter = PineBacktestExecutionAdapter::new(Arc::new(StaticMockPinePort { intents }));
    let request = BacktestExecutionRequest {
        run_id: "test-cancel-all-diff".to_owned(),
        payload: json!({
            "strategyScript": "strategy('test')",
            "symbol": "US.AAPL",
            "interval": "1m",
            "processOrdersOnClose": true,
            "initialBalance": "100000",
        }),
        market_data_provider: "yfinance".to_owned(),
        candles,
    };

    let result = adapter.execute(request).expect("execute backtest");
    let case = &result["cases"][0];
    assert_eq!(case["status"], "completed");
    let orders = case["orders"].as_array().expect("orders array");
    assert_eq!(orders.len(), 1);
    assert_eq!(orders[0]["status"], "CANCELED");
}

#[test]
fn intent_with_has_quantity_pct_and_resolved_quantity_executes_successfully() {
    let candles = (0..1)
        .map(|i| sample_candle(i, 1_700_000_000_000))
        .collect();
    let intents = vec![PineOrderIntent {
        id: "pct_entry".to_owned(),
        kind: "entry".to_owned(),
        direction: "long".to_owned(),
        has_quantity: true,
        quantity: 10.0,
        has_quantity_pct: true,
        quantity_pct: 50.0,
        bar_index: 0,
        ..Default::default()
    }];

    let adapter = PineBacktestExecutionAdapter::new(Arc::new(StaticMockPinePort { intents }));
    let request = BacktestExecutionRequest {
        run_id: "test-qty-pct".to_owned(),
        payload: json!({
            "strategyScript": "strategy('test')",
            "symbol": "US.AAPL",
            "interval": "1m",
            "processOrdersOnClose": true,
            "initialBalance": "100000",
        }),
        market_data_provider: "yfinance".to_owned(),
        candles,
    };

    let result = adapter
        .execute(request)
        .expect("execute backtest with resolved quantity_pct");
    let case = &result["cases"][0];
    assert_eq!(case["status"], "completed");
    assert_eq!(case["basePosition"], "10");
}

#[test]
fn warmup_entry_and_formal_exit_inherits_position_and_pnl_in_matcher() {
    // 2 warmup candles, 2 formal candles
    let mut candles = Vec::new();
    for i in 0..4 {
        let mut c = sample_candle(i, 1_700_000_000_000);
        if i == 0 {
            c.open = "100.0".to_owned();
            c.close = "100.0".to_owned();
        } else if i == 3 {
            c.open = "110.0".to_owned();
            c.high = "115.0".to_owned();
            c.low = "105.0".to_owned();
            c.close = "110.0".to_owned();
        }
        candles.push(c);
    }

    let intents = vec![
        PineOrderIntent {
            id: "warmup_buy".to_owned(),
            kind: "entry".to_owned(),
            direction: "long".to_owned(),
            has_quantity: true,
            quantity: 10.0,
            bar_index: 0,
            ..Default::default()
        },
        PineOrderIntent {
            id: "formal_exit".to_owned(),
            kind: "exit".to_owned(),
            direction: "long".to_owned(),
            has_quantity: true,
            quantity: 10.0,
            bar_index: 3,
            ..Default::default()
        },
    ];

    let adapter = PineBacktestExecutionAdapter::new(Arc::new(StaticMockPinePort { intents }));
    let request = BacktestExecutionRequest {
        run_id: "test-warmup-pos-inherit".to_owned(),
        payload: json!({
            "strategyScript": "strategy('test')",
            "symbol": "US.AAPL",
            "interval": "1m",
            "warmupBars": 2,
            "processOrdersOnClose": true,
            "initialBalance": "10000",
        }),
        market_data_provider: "yfinance".to_owned(),
        candles,
    };

    let result = adapter
        .execute(request)
        .expect("execute backtest with warmup position inheritance");
    let case = &result["cases"][0];
    assert_eq!(case["status"], "completed");
    assert_eq!(case["processedBars"], 4);
    assert_eq!(case["basePosition"], "0");
    assert_eq!(case["totalTrades"], 1);
    let realized_pnl: f64 = case["realizedPnl"].as_str().unwrap().parse().unwrap();
    assert!(realized_pnl > 90.0, "realized pnl should reflect 10 shares from 100 to 110 minus fees");
}
