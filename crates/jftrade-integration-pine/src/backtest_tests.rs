use std::sync::Arc;

use serde_json::json;

use super::{
    BacktestExecutionCandle, BacktestExecutionError, BacktestExecutionPort,
    BacktestExecutionRequest, PineBacktestExecutionAdapter, RunJsonBacktestExecutionPort,
};
use crate::{
    PineExecutionError, PineExecutionFuture, PineExecutionPort, PineOrderIntent, PineRunResult,
};

#[derive(Clone, Debug)]
struct FakePinePort {
    result: Result<PineRunResult, PineExecutionError>,
}

impl PineExecutionPort for FakePinePort {
    fn run<'a>(&'a self, _request: crate::PineRunRequest) -> PineExecutionFuture<'a> {
        let result = self.result.clone();
        Box::pin(async move { result })
    }
}

fn execution_request(payload: serde_json::Value) -> BacktestExecutionRequest {
    BacktestExecutionRequest {
        run_id: "run-adapter".to_owned(),
        payload,
        market_data_provider: "yfinance".to_owned(),
        candles: vec![BacktestExecutionCandle {
            start_time: 1_750_683_600_000,
            end_time: 1_750_683_659_999,
            open: "100".to_owned(),
            high: "101".to_owned(),
            low: "99".to_owned(),
            close: "100".to_owned(),
            volume: "10".to_owned(),
        }],
    }
}

fn adapter_result() -> PineRunResult {
    PineRunResult {
        order_intents: vec![PineOrderIntent {
            kind: "entry".to_owned(),
            id: "entry-1".to_owned(),
            direction: "long".to_owned(),
            quantity: 1.0,
            has_quantity: true,
            bar_index: 0,
            ..PineOrderIntent::default()
        }],
        ..PineRunResult::default()
    }
}

#[test]
fn pine_adapter_runs_worker_intent_through_deterministic_matcher() {
    let request = execution_request(json!({
        "strategyScript": "strategy('fixture')",
        "definitionId": "fixture",
        "symbol": "US.AAPL",
        "interval": "1m",
        "initialBalance": "1000"
    }));
    let adapter = PineBacktestExecutionAdapter::new(Arc::new(FakePinePort {
        result: Ok(adapter_result()),
    }));
    let output = adapter.execute(request).expect("run pine corpus");
    assert_eq!(output["cases"][0]["processedBars"], 1);
    assert_eq!(output["cases"][0]["orders"][0]["clientOrderId"], "entry-1");
}

#[test]
fn pine_adapter_maps_worker_unavailability_without_fabricating_result() {
    let request = execution_request(json!({
        "strategyScript": "strategy('fixture')",
        "symbol": "US.AAPL",
        "interval": "1m"
    }));
    let adapter = PineBacktestExecutionAdapter::new(Arc::new(FakePinePort {
        result: Err(PineExecutionError::Unavailable("worker stopped".to_owned())),
    }));
    let error = adapter.execute(request).expect_err("worker failure");
    assert!(
        matches!(error, BacktestExecutionError::Unavailable(message) if message == "worker stopped")
    );
}

#[test]
fn pine_adapter_rejects_invalid_worker_intent() {
    let request = execution_request(json!({
        "strategyScript": "strategy('fixture')",
        "symbol": "US.AAPL",
        "interval": "1m"
    }));
    let mut result = adapter_result();
    result.order_intents[0].direction = "sideways".to_owned();
    let adapter = PineBacktestExecutionAdapter::new(Arc::new(FakePinePort { result: Ok(result) }));
    let error = adapter.execute(request).expect_err("invalid direction");
    assert!(
        matches!(error, BacktestExecutionError::Invalid(message) if message.contains("requires long/short"))
    );
}

#[test]
fn pine_adapter_requires_source_and_history() {
    let adapter = PineBacktestExecutionAdapter::new(Arc::new(FakePinePort {
        result: Ok(adapter_result()),
    }));
    let missing_source = adapter.execute(execution_request(json!({
        "symbol": "US.AAPL",
        "interval": "1m"
    })));
    assert!(
        matches!(missing_source, Err(BacktestExecutionError::Invalid(message)) if message.contains("strategy source"))
    );

    let mut missing_history = execution_request(json!({
        "strategyScript": "strategy('fixture')",
        "symbol": "US.AAPL",
        "interval": "1m"
    }));
    missing_history.candles.clear();
    let error = adapter
        .execute(missing_history)
        .expect_err("missing history");
    assert!(
        matches!(error, BacktestExecutionError::Unavailable(message) if message.contains("history"))
    );
}

#[test]
fn run_json_adapter_fills_validated_history_into_empty_corpus_case() {
    let request = BacktestExecutionRequest {
        run_id: "run-fixture".to_owned(),
        payload: json!({
            "version": 1,
            "cases": [{
                "id": "fixture",
                "symbol": "US.AAPL",
                "baseCurrency": "AAPL",
                "quoteCurrency": "USD",
                "initialBalance": "1000",
                "market": {
                    "tickSize": "0.01",
                    "quantityStep": "1",
                    "minQuantity": "1"
                },
                "candles": []
            }]
        }),
        market_data_provider: "yfinance".to_owned(),
        candles: vec![BacktestExecutionCandle {
            start_time: 1_750_683_600_000,
            end_time: 1_750_683_659_999,
            open: "100".to_owned(),
            high: "101".to_owned(),
            low: "99".to_owned(),
            close: "100".to_owned(),
            volume: "10".to_owned(),
        }],
    };

    let output = RunJsonBacktestExecutionPort
        .execute(request)
        .expect("run deterministic corpus");
    assert_eq!(output["cases"][0]["processedBars"], 1);
}

#[test]
fn pine_adapter_filters_warmup_intents_and_shifts_evaluation_bar_indices() {
    let warmup_candle = BacktestExecutionCandle {
        start_time: 1_750_683_540_000,
        end_time: 1_750_683_599_999,
        open: "99".to_owned(),
        high: "100".to_owned(),
        low: "98".to_owned(),
        close: "99".to_owned(),
        volume: "10".to_owned(),
    };
    let formal_candle = BacktestExecutionCandle {
        start_time: 1_750_683_600_000,
        end_time: 1_750_683_659_999,
        open: "100".to_owned(),
        high: "101".to_owned(),
        low: "99".to_owned(),
        close: "100".to_owned(),
        volume: "10".to_owned(),
    };

    let request = BacktestExecutionRequest {
        run_id: "run-warmup".to_owned(),
        payload: json!({
            "strategyScript": "strategy('fixture')",
            "symbol": "US.AAPL",
            "interval": "1m",
            "warmupBars": 1,
        }),
        market_data_provider: "yfinance".to_owned(),
        candles: vec![warmup_candle, formal_candle],
    };

    let mut result = adapter_result();
    result.order_intents = vec![
        PineOrderIntent {
            id: "warmup-intent".to_owned(),
            kind: "order".to_owned(),
            direction: "long".to_owned(),
            bar_index: 0,
            has_quantity: true,
            quantity: 1.0,
            ..Default::default()
        },
        PineOrderIntent {
            id: "formal-intent".to_owned(),
            kind: "order".to_owned(),
            direction: "long".to_owned(),
            bar_index: 1,
            has_quantity: true,
            quantity: 1.0,
            ..Default::default()
        },
    ];

    let adapter = PineBacktestExecutionAdapter::new(Arc::new(FakePinePort { result: Ok(result) }));
    let output = adapter.execute(request).expect("execute with warmup");
    assert_eq!(output["cases"][0]["processedBars"], 1);
    assert_eq!(
        output["cases"][0]["orders"].as_array().map(Vec::len),
        Some(1)
    );
}
