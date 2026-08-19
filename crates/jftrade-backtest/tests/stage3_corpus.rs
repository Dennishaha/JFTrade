use std::fs;
use std::path::PathBuf;

use jftrade_backtest::run_json;
use serde_json::{Value, json};

fn fixture(name: &str) -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../tests/fixtures/rust-migration/stage3")
        .join(name)
}

#[test]
fn rust_replays_the_go_stage3_golden_without_differences() {
    let input = fs::read(fixture("backtest-corpus.json")).expect("read corpus");
    let expected: Value = serde_json::from_slice(
        &fs::read(fixture("backtest-corpus.expected.json")).expect("read expected output"),
    )
    .expect("decode expected output");
    let actual: Value =
        serde_json::from_slice(&run_json(&input).expect("run corpus")).expect("decode Rust output");
    assert_eq!(actual, expected);
}

#[test]
fn corpus_output_is_byte_deterministic_across_replays() {
    let input = fs::read(fixture("backtest-corpus.json")).expect("read corpus");
    let expected = run_json(&input).expect("first run");
    for _ in 0..100 {
        assert_eq!(run_json(&input).expect("replay"), expected);
    }
}

#[test]
fn cancellation_does_not_poison_a_recovered_run() {
    let input = fs::read(fixture("backtest-corpus.json")).expect("read corpus");
    let mut corpus: Value = serde_json::from_slice(&input).expect("decode corpus");
    let cases = corpus["cases"].as_array_mut().expect("cases");
    let cancelled = cases
        .iter_mut()
        .find(|case| case["id"] == "cancelled-before-next-bar")
        .expect("cancelled case");
    cancelled
        .as_object_mut()
        .expect("case object")
        .remove("cancelBeforeBar");
    let output: Value = serde_json::from_slice(
        &run_json(&serde_json::to_vec(&corpus).expect("encode recovery")).expect("recovered run"),
    )
    .expect("decode recovered output");
    let recovered = output["cases"]
        .as_array()
        .expect("output cases")
        .iter()
        .find(|case| case["id"] == "cancelled-before-next-bar")
        .expect("recovered case");
    assert_eq!(recovered["status"], "completed");
    assert_eq!(recovered["processedBars"], 3);
}

#[test]
fn flat_price_round_trips_conserve_equity_across_liquidity_partitions() {
    for quantity in 1..=16 {
        for volume in [10, 20, 50, 100] {
            let corpus = json!({
                "version": 1,
                "cases": [{
                    "id": format!("q{quantity}-v{volume}"),
                    "symbol": "US.AAPL",
                    "baseCurrency": "AAPL",
                    "quoteCurrency": "USD",
                    "initialBalance": "10000",
                    "processOrdersOnClose": false,
                    "slippageTicks": 0,
                    "market": {"tickSize":"0.01","quantityStep":"1","minQuantity":"1"},
                    "feeRules": [],
                    "indicatorPeriods": [],
                    "candles": [
                        {"start":"2026-07-10T13:30:00Z","end":"2026-07-10T13:30:59.999Z","open":"100","high":"100","low":"100","close":"100","volume":volume.to_string()},
                        {"start":"2026-07-10T13:31:00Z","end":"2026-07-10T13:31:59.999Z","open":"100","high":"100","low":"100","close":"100","volume":volume.to_string()},
                        {"start":"2026-07-10T13:32:00Z","end":"2026-07-10T13:32:59.999Z","open":"100","high":"100","low":"100","close":"100","volume":"1000"}
                    ],
                    "intents": [{"barIndex":0,"action":"submit","id":"entry","side":"buy","orderType":"market","quantity":quantity.to_string()}]
                }]
            });
            let output: Value = serde_json::from_slice(
                &run_json(&serde_json::to_vec(&corpus).expect("encode property case"))
                    .expect("run property case"),
            )
            .expect("decode property output");
            assert_eq!(output["cases"][0]["finalEquity"], "10000");
        }
    }
}

#[test]
fn malformed_and_truncated_inputs_fail_without_panicking() {
    let input = fs::read(fixture("backtest-corpus.json")).expect("read corpus");
    let final_json_byte = input
        .iter()
        .rposition(|byte| !byte.is_ascii_whitespace())
        .expect("non-empty corpus");
    for length in 0..=final_json_byte {
        assert!(run_json(&input[..length]).is_err());
    }
    for index in (0..input.len()).step_by(53) {
        let mut mutated = input.clone();
        mutated[index] = 0xff;
        let _ = run_json(&mutated);
    }
    let unknown = br#"{"version":1,"cases":[],"unexpected":true}"#;
    assert!(run_json(unknown).is_err());
}
