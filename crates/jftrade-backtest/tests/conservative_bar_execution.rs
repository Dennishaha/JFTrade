use jftrade_backtest::{CorpusInput, run_corpus, run_json};
use serde_json::{Value, json};

// ============================================================================
// Go -> Rust Mapping Documentation & Audit
// ============================================================================
//
// 1. TestNormalizeExecutionModelName:
//    - Go tested string trimming, case-insensitivity ("  conservative-bar-v1  ",
//      "CONSERVATIVE-BAR-V1"), and rejection of unsupported "optimistic".
//    - The Rust leaf corpus has no execution-model input: it always emits the fixed
//      `conservative-bar-v1` metadata and separately validates `CORPUS_VERSION`.
//    - `execution_model_metadata_and_corpus_version_contract` verifies that narrower Rust
//      contract. It does not replace Go's user/API-facing normalization behavior, so that Go
//      normalization test remains outside this G1 deletion scope.
//
// 2. TestConservativeBarExecutorValidationErrors:
//    - Go nil-receiver (`nil.SubmitOrders`, `nil.SubmitAtomicPineOrders`, `nil.CancelOrders`) and
//      missing runtime dependencies (`account == nil`, `stream == nil`):
//      [NOT APPLICABLE] in Rust because Rust's type system and ownership model prevent
//      calling methods on nil receivers or constructing invalid runtime handles.
//    - Rust additionally tests validation branches expressible through the corpus boundary:
//      * Empty case ID
//      * Negative initial balance
//      * Non-positive tick_size, quantity_step, and negative min_quantity
//      * Empty candle list
//      * Strictly out-of-order candles (start/end timestamp inversion)
//      * Intent targeting unavailable bar index (barIndex >= candles.len())
//      * A non-empty atomic group containing fewer than 2 orders
//      * Atomic child without parent in group
//      * Missing limit price for limit / limit_maker orders
//      * Missing stop price for stop_market orders
//      * Missing stop and limit prices for stop_limit orders
//      * Unsupported intent side (e.g. "BAD", "HOLD")
//      * Non-positive quantity (<= 0)
//      * Unsupported intent action (e.g. "modify")
//      * Duplicate intent client order IDs
//      * Duplicate case IDs in corpus
//      * Unsupported corpus version
//    - An empty `atomicGroupId` is the representation of a non-atomic intent in Rust and cannot
//      reach `submit_atomic`; this differs from Go's direct empty-group helper call. The covered
//      Rust validation cases live in `input_and_atomic_bracket_validation_errors` and
//      `atomic_child_without_parent_is_rejected`.
//
// 3. TestConservativeBarExecutorFillsMarketOrderOnNextOpenWithLiquidityCap:
//    - Go tested 50-share buy market order filling partially (10 shares @ 101) across Bar 1 (vol 100),
//      then completing remainder (40 shares @ 102) on Bar 2 (vol 1000).
//    - Fully mapped with complete field-by-field assertions (order status transitions,
//      filled quantities, average price, timestamps, fill quoteQuantities, maker flags,
//      cash, basePosition, equity, fees, drawdown, warnings) in
//      `market_order_fills_on_next_open_with_liquidity_cap`.
//
// 4. TestConservativeBarExecutorRunsParentBracketAtomicallyAndStopFirst:
//    - Go tested parent buy entry with simultaneous protective stop (99.9) and limit (100.3).
//      On Bar 1 where both prices are within [99, 101], conservative priority executes the stop
//      first and cancels the limit via OCO.
//    - Fully mapped with complete field-by-field assertions for all 3 orders and 2 fills in
//      `parent_bracket_runs_atomically_with_stop_first_protection`.
//
// 5. TestConservativeBarExecutorRejectsAtomicChildWithoutParent:
//    - Go tested rejection when an atomic child references a missing parent ID.
//    - Fully mapped in `atomic_child_without_parent_is_rejected`.
//
// 6. TestConservativeBarExecutorFillsAtomicBracketOnSignalClose:
//    - Go tested processOrdersOnClose=true filling parent entry and protective stop on the
//      same signal close bar (at close price 100), canceling protective limit via OCO.
//    - Fully mapped with complete field-by-field assertions in `atomic_bracket_fills_on_signal_close`.
//
// 7. TestConservativeBarExecutorCancelParentCancelsProtectiveChildren:
//    - Go tested explicit cancel of parent order cascading to cancel all protective children.
//    - Fully mapped with field-by-field assertions in `canceling_parent_order_cancels_all_protective_children`.
//
// 8. TestConservativeBarExecutorCancelsReduceOnlyOrderWithoutPosition:
//    - Go tested sell reduce-only order canceled when base position is 0.
//    - Fully mapped with field-by-field assertions in `reduce_only_order_without_position_is_canceled`.
//
// 9. TestConservativeBarExecutorLimitsReduceOnlyFillToOpenPosition:
//    - Go tested 2-share sell reduce-only order filling 1 share against open long position of 1,
//      and canceling the remaining 1 share.
//    - Fully mapped with field-by-field assertions in `reduce_only_fill_is_limited_to_open_position`.
//
// 10. TestConservativeBarExecutorCancelOrders:
//     - Go tested cancelling order by ID, cancelling order by client ID, and skipping missing order.
//     - Fully mapped with field-by-field assertions in `explicit_cancel_orders_and_unmatched_target_handling`
//       (by client order ID) and `explicit_cancel_orders_by_generated_order_id` (by generated order ID).
//
// 11. TestConservativeBarExecutorProcessOrdersOnCloseUsesSignalClose:
//     - Go tested processOrdersOnClose=true executing buy market order at signal candle close price.
//     - Fully mapped with field-by-field assertions in `process_orders_on_close_executes_at_signal_close_price`.
//
// 12. TestConservativeBarExecutorSellMarketAndSlippage:
//     - Go tested downward slippage (2 ticks of 0.05 = 0.10) applied to sell market order at open (101 -> 100.90).
//     - Fully mapped with field-by-field assertions in `sell_market_order_applies_downward_slippage`.
//
// 13. TestConservativeBarExecutorLimitOrderGetsGapImprovement:
//     - Go tested buy limit order (limit 100) receiving favorable open gap improvement (open 99).
//     - Fully mapped with field-by-field assertions in `limit_order_receives_favorable_gap_improvement`.
//
// 14. TestConservativeBarExecutorLimitSellAndClosePointBranches:
//     - Go tested limit sell close-point fill (105), open improvement (102), and intrabar fill (103).
//     - Fully mapped with field-by-field assertions in `limit_sell_open_improvement_intrabar_and_close_point`.
//
// 15. TestConservativeBarExecutorStopOrders:
//     - Go tested buy stop market open trigger (106), sell stop market intrabar trigger (95),
//       and buy stop limit triggering on Bar 1 and filling with open improvement on Bar 2 (102).
//     - Fully mapped with field-by-field assertions in `stop_market_and_stop_limit_orders_execution`.
//
// 16. TestConservativeBarExecutorWarningsAndUnmatchedOrders:
//     - Go tested warning emission and deduplication for zero-volume bars and unsupported order types.
//     - Rust maps those warnings with exact string and deduplication assertions in
//       `warnings_emitted_and_deduplicated_for_zero_volume_and_unsupported_order`.
//     - Go also submitted an order for a different symbol. Rust binds every intent to the case
//       symbol and has no per-intent symbol field, so that invalid internal state is not expressible.
//
// 17. TestConservativeBarExecutorLiquidityWarnings:
//     - Go tested warnings when liquidity budget is below quantity step or below min quantity.
//     - Fully mapped with field-by-field assertions in `liquidity_warnings_for_sub_step_and_below_min_quantity`.
//
// 18. TestConservativeBarExecutorHelperBranches:
//     - Go tested helper logic: zero-volume liquidity budget, buy/sell limit close and intrabar pricing,
//       zero/bad price and side rejection, buy/sell stop market close and intrabar pricing, unreachable stops,
//       negative slippage clamping to zero, upward buy slippage, event time, and warn-once deduplication.
//     - Matching/pricing behavior is mapped into deterministic end-to-end cases and validation
//       tests in `matching_and_pricing_helper_branches_behavior`. Go-only invalid internal states
//       (nil pending pointers, a missing warning sink, or event-time lookup without a bar) are not
//       representable through Rust's safe corpus boundary.
//
// 19. TestConservativeBarExecutorCancelSkipsUnmatchedPendingOrders:
//     - Go directly injected `nil` pointers and zero-remaining elements into an internal `pending` slice
//       to verify compaction and unmatched skipping. In Rust, internal `nil` pointer injection into pending
//       orders is [NOT APPLICABLE] due to safe memory representations without nil pointers.
//     - Unmatched cancellation, cancellation of an already closed order, and preservation of valid
//       pending orders are mapped with field-by-field assertions in
//       `cancel_skips_unmatched_pending_orders_without_side_effects`.
//
// Regression & Ordering Guarantees:
//     - `multiple_atomic_groups_preserve_input_order_over_lexicographical_sorting` guarantees
//       reverse-lexicographical groups retain first-occurrence order even when an ordinary intent
//       appears between group members. Reusing a processed group ID on a later bar is ignored for
//       the entire case, matching the Go Stage 3 reference harness lifecycle.
// ============================================================================

fn test_bar(index: usize, open: f64, high: f64, low: f64, close: f64, volume: f64) -> Value {
    let minute = 30 + index;
    json!({
        "start": format!("2026-06-29T09:{minute:02}:00Z"),
        "end": format!("2026-06-29T09:{minute:02}:59.999Z"),
        "open": open.to_string(),
        "high": high.to_string(),
        "low": low.to_string(),
        "close": close.to_string(),
        "volume": volume.to_string(),
    })
}

fn default_market() -> Value {
    json!({
        "tickSize": "0.01",
        "quantityStep": "1",
        "minQuantity": "1"
    })
}

fn run_single_case(case: Value) -> Result<Value, String> {
    let corpus = json!({
        "version": 1,
        "cases": [case]
    });
    let bytes = serde_json::to_vec(&corpus).map_err(|e| e.to_string())?;
    let output_bytes = run_json(&bytes).map_err(|e| e.to_string())?;
    let mut output: Value = serde_json::from_slice(&output_bytes).map_err(|e| e.to_string())?;
    let cases = output["cases"]
        .as_array_mut()
        .ok_or_else(|| "expected cases array in output".to_string())?;
    if cases.is_empty() {
        return Err("expected at least one case in output".to_string());
    }
    Ok(cases.remove(0))
}

#[derive(Debug, Clone, Copy)]
struct ExpectedOrder<'a> {
    pub order_id: &'a str,
    pub client_order_id: &'a str,
    pub side: &'a str,
    pub order_type: &'a str,
    pub quantity: &'a str,
    pub status: &'a str,
    pub filled_quantity: &'a str,
    pub filled_price: &'a str,
    pub submitted_at: &'a str,
    pub filled_at: &'a str,
    pub reduce_only: bool,
}

#[derive(Debug, Clone, Copy)]
struct ExpectedFill<'a> {
    pub trade_id: &'a str,
    pub order_id: &'a str,
    pub client_order_id: &'a str,
    pub side: &'a str,
    pub price: &'a str,
    pub quantity: &'a str,
    pub quote_quantity: &'a str,
    pub time: &'a str,
    pub maker: bool,
    pub broker_fee: &'a str,
    pub market_fee: &'a str,
    pub total_fee: &'a str,
    pub realized_pnl: &'a str,
}

#[track_caller]
fn assert_order(order: &Value, expected: &ExpectedOrder<'_>) {
    assert_eq!(
        order["orderId"], expected.order_id,
        "orderId mismatch for {}",
        expected.client_order_id
    );
    assert_eq!(
        order["clientOrderId"], expected.client_order_id,
        "clientOrderId mismatch"
    );
    assert_eq!(
        order["side"], expected.side,
        "side mismatch for {}",
        expected.client_order_id
    );
    assert_eq!(
        order["orderType"], expected.order_type,
        "orderType mismatch for {}",
        expected.client_order_id
    );
    assert_eq!(
        order["quantity"], expected.quantity,
        "quantity mismatch for {}",
        expected.client_order_id
    );
    assert_eq!(
        order["status"], expected.status,
        "status mismatch for {}",
        expected.client_order_id
    );
    assert_eq!(
        order["filledQuantity"], expected.filled_quantity,
        "filledQuantity mismatch for {}",
        expected.client_order_id
    );
    assert_eq!(
        order["filledPrice"], expected.filled_price,
        "filledPrice mismatch for {}",
        expected.client_order_id
    );
    assert_eq!(
        order["submittedAt"], expected.submitted_at,
        "submittedAt mismatch for {}",
        expected.client_order_id
    );
    assert_eq!(
        order["filledAt"], expected.filled_at,
        "filledAt mismatch for {}",
        expected.client_order_id
    );
    assert_eq!(
        order["reduceOnly"], expected.reduce_only,
        "reduceOnly mismatch for {}",
        expected.client_order_id
    );
}

#[track_caller]
fn assert_fill(fill: &Value, expected: &ExpectedFill<'_>) {
    assert_eq!(
        fill["tradeId"], expected.trade_id,
        "tradeId mismatch for fill {}",
        expected.client_order_id
    );
    assert_eq!(
        fill["orderId"], expected.order_id,
        "orderId mismatch for fill {}",
        expected.client_order_id
    );
    assert_eq!(
        fill["clientOrderId"], expected.client_order_id,
        "clientOrderId mismatch for fill"
    );
    assert_eq!(
        fill["side"], expected.side,
        "side mismatch for fill {}",
        expected.client_order_id
    );
    assert_eq!(
        fill["price"], expected.price,
        "price mismatch for fill {}",
        expected.client_order_id
    );
    assert_eq!(
        fill["quantity"], expected.quantity,
        "quantity mismatch for fill {}",
        expected.client_order_id
    );
    assert_eq!(
        fill["quoteQuantity"], expected.quote_quantity,
        "quoteQuantity mismatch for fill {}",
        expected.client_order_id
    );
    assert_eq!(
        fill["time"], expected.time,
        "time mismatch for fill {}",
        expected.client_order_id
    );
    assert_eq!(
        fill["maker"], expected.maker,
        "maker mismatch for fill {}",
        expected.client_order_id
    );
    assert_eq!(
        fill["brokerFee"], expected.broker_fee,
        "brokerFee mismatch for fill {}",
        expected.client_order_id
    );
    assert_eq!(
        fill["marketFee"], expected.market_fee,
        "marketFee mismatch for fill {}",
        expected.client_order_id
    );
    assert_eq!(
        fill["totalFee"], expected.total_fee,
        "totalFee mismatch for fill {}",
        expected.client_order_id
    );
    assert_eq!(
        fill["realizedPnl"], expected.realized_pnl,
        "realizedPnl mismatch for fill {}",
        expected.client_order_id
    );
}

#[test]
fn execution_model_metadata_and_corpus_version_contract() {
    let case = json!({
        "id": "model-check",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "10000",
        "processOrdersOnClose": false,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0)],
        "intents": []
    });
    let corpus = json!({
        "version": 1,
        "cases": [case]
    });
    let bytes = serde_json::to_vec(&corpus).expect("encode");
    let output_bytes = run_json(&bytes).expect("run");
    let output: Value = serde_json::from_slice(&output_bytes).expect("decode");

    assert_eq!(output["version"], 1);
    assert_eq!(output["executionModel"], "conservative-bar-v1");

    let case_res = &output["cases"][0];
    assert_eq!(case_res["id"], "model-check");
    assert_eq!(case_res["status"], "completed");
    assert_eq!(case_res["processedBars"], 1);
    assert_eq!(case_res["cash"], "10000");
    assert_eq!(case_res["basePosition"], "0");
    assert_eq!(case_res["finalEquity"], "10000");
    assert_eq!(case_res["realizedPnl"], "0");
    assert_eq!(case_res["totalBrokerFees"], "0");
    assert_eq!(case_res["totalMarketFees"], "0");
    assert_eq!(case_res["totalFees"], "0");
    assert_eq!(case_res["totalFills"], 0);
    assert_eq!(case_res["totalTrades"], 0);
    assert_eq!(case_res["winningTrades"], 0);
    assert_eq!(case_res["winRate"], "0");
    assert_eq!(case_res["maxDrawdown"], "0");
    assert_eq!(case_res["currentDrawdown"], "0");
    assert!(case_res["orders"].as_array().unwrap().is_empty());
    assert!(case_res["fills"].as_array().unwrap().is_empty());
    assert_eq!(case_res["equityCurve"].as_array().unwrap().len(), 1);
    assert_eq!(
        case_res["equityCurve"][0]["time"],
        "2026-06-29T09:30:59.999Z"
    );
    assert_eq!(case_res["equityCurve"][0]["equity"], "10000");
    assert_eq!(case_res["drawdownCurve"].as_array().unwrap().len(), 1);
    assert_eq!(case_res["drawdownCurve"][0]["drawdown"], "0");
    assert!(!case_res["resultHash"].as_str().unwrap().is_empty());

    // Version mismatch is strictly rejected
    let invalid_version = json!({
        "version": 999,
        "cases": []
    });
    let err = run_json(&serde_json::to_vec(&invalid_version).unwrap());
    assert!(err.is_err());
    assert!(
        err.unwrap_err()
            .to_string()
            .contains("unsupported corpus version 999; expected 1")
    );

    // Duplicate case IDs rejected
    let dup_cases = json!({
        "version": 1,
        "cases": [
            {
                "id": "same-id",
                "symbol": "US.AAPL",
                "baseCurrency": "AAPL",
                "quoteCurrency": "USD",
                "initialBalance": "1000",
                "market": default_market(),
                "candles": [test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0)],
            },
            {
                "id": "same-id",
                "symbol": "US.AAPL",
                "baseCurrency": "AAPL",
                "quoteCurrency": "USD",
                "initialBalance": "1000",
                "market": default_market(),
                "candles": [test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0)],
            }
        ]
    });
    let dup_err = run_json(&serde_json::to_vec(&dup_cases).unwrap()).unwrap_err();
    assert!(dup_err.to_string().contains("duplicate case id same-id"));
}

#[test]
fn input_and_atomic_bracket_validation_errors() {
    // 1. Empty case ID / symbol
    let bad_id = json!({
        "id": "",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "10000",
        "market": default_market(),
        "candles": [test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0)],
    });
    assert!(
        run_single_case(bad_id)
            .unwrap_err()
            .contains("case id and symbol are required")
    );

    // 2. Negative initial balance
    let bad_balance = json!({
        "id": "neg-bal",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "-100",
        "market": default_market(),
        "candles": [test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0)],
    });
    assert!(
        run_single_case(bad_balance)
            .unwrap_err()
            .contains("initial balance cannot be negative")
    );

    // 3. Non-positive market increments (tickSize=0, quantityStep=0, minQuantity<0)
    let bad_tick = json!({
        "id": "bad-tick",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "market": {"tickSize": "0", "quantityStep": "1", "minQuantity": "1"},
        "candles": [test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0)],
    });
    assert!(
        run_single_case(bad_tick)
            .unwrap_err()
            .contains("market increments must be positive")
    );

    let bad_step = json!({
        "id": "bad-step",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "market": {"tickSize": "0.01", "quantityStep": "0", "minQuantity": "1"},
        "candles": [test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0)],
    });
    assert!(
        run_single_case(bad_step)
            .unwrap_err()
            .contains("market increments must be positive")
    );

    let bad_min = json!({
        "id": "bad-min",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "market": {"tickSize": "0.01", "quantityStep": "1", "minQuantity": "-1"},
        "candles": [test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0)],
    });
    assert!(
        run_single_case(bad_min)
            .unwrap_err()
            .contains("market increments must be positive")
    );

    // 4. Empty candles
    let no_candles = json!({
        "id": "no-candles",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "market": default_market(),
        "candles": [],
    });
    assert!(
        run_single_case(no_candles)
            .unwrap_err()
            .contains("at least one candle is required")
    );

    // 5. Unordered candles
    let bad_order_candles = json!({
        "id": "bad-candles",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "market": default_market(),
        "candles": [test_bar(1, 100.0, 101.0, 99.0, 100.0, 1000.0), test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0)],
    });
    assert!(
        run_single_case(bad_order_candles)
            .unwrap_err()
            .contains("candles must be strictly ordered")
    );

    // 6. Intent targeting unavailable bar
    let bad_bar_intent = json!({
        "id": "bad-intent-bar",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "market": default_market(),
        "candles": [test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0)],
        "intents": [{"barIndex": 5, "action": "submit", "id": "o1", "side": "buy", "quantity": "1"}]
    });
    assert!(
        run_single_case(bad_bar_intent)
            .unwrap_err()
            .contains("intent o1 targets an unavailable bar")
    );

    // 7. Atomic group requires ID and at least 2 orders
    let atomic_single = json!({
        "id": "atomic-single",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "market": default_market(),
        "candles": [test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0)],
        "intents": [
            {"barIndex": 0, "action": "submit", "id": "o1", "side": "buy", "quantity": "1", "atomicGroupId": "grp1"}
        ]
    });
    assert!(
        run_single_case(atomic_single)
            .unwrap_err()
            .contains("atomic group requires an id and at least two orders")
    );

    // 8. Intent missing limit price
    let limit_missing_price = json!({
        "id": "limit-missing-price",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "market": default_market(),
        "candles": [test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0)],
        "intents": [
            {"barIndex": 0, "action": "submit", "id": "l1", "side": "buy", "orderType": "limit", "quantity": "1"}
        ]
    });
    assert!(
        run_single_case(limit_missing_price)
            .unwrap_err()
            .contains("intent l1 requires a limit price")
    );

    // 9. Intent missing stop price
    let stop_missing_price = json!({
        "id": "stop-missing-price",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "market": default_market(),
        "candles": [test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0)],
        "intents": [
            {"barIndex": 0, "action": "submit", "id": "s1", "side": "buy", "orderType": "stop_market", "quantity": "1"}
        ]
    });
    assert!(
        run_single_case(stop_missing_price)
            .unwrap_err()
            .contains("intent s1 requires a stop price")
    );

    // 10. Intent missing stop_limit prices
    let stop_limit_missing = json!({
        "id": "stop-limit-missing-prices",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "market": default_market(),
        "candles": [test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0)],
        "intents": [
            {"barIndex": 0, "action": "submit", "id": "sl1", "side": "buy", "orderType": "stop_limit", "quantity": "1", "stopPrice": "105"}
        ]
    });
    assert!(
        run_single_case(stop_limit_missing)
            .unwrap_err()
            .contains("intent sl1 requires stop and limit prices")
    );

    // 11. Intent unsupported side
    let bad_side_intent = json!({
        "id": "bad-side-case",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "market": default_market(),
        "candles": [test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0)],
        "intents": [
            {"barIndex": 0, "action": "submit", "id": "bad-side", "side": "BAD", "orderType": "market", "quantity": "1"}
        ]
    });
    assert!(
        run_single_case(bad_side_intent)
            .unwrap_err()
            .contains("intent bad-side has unsupported side BAD")
    );

    // 12. Intent non-positive quantity
    let zero_qty_intent = json!({
        "id": "zero-qty-case",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "market": default_market(),
        "candles": [test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0)],
        "intents": [
            {"barIndex": 0, "action": "submit", "id": "z1", "side": "buy", "orderType": "market", "quantity": "0"}
        ]
    });
    assert!(
        run_single_case(zero_qty_intent)
            .unwrap_err()
            .contains("submit intent requires id and positive quantity")
    );

    // 13. Intent unsupported action
    let bad_action_intent = json!({
        "id": "bad-action-case",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "market": default_market(),
        "candles": [test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0)],
        "intents": [
            {"barIndex": 0, "action": "modify", "id": "m1", "side": "buy", "orderType": "market", "quantity": "1"}
        ]
    });
    assert!(
        run_single_case(bad_action_intent)
            .unwrap_err()
            .contains("unsupported intent action modify")
    );

    // 14. Duplicate intent ID
    let dup_intent_id = json!({
        "id": "dup-intent-id-case",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "market": default_market(),
        "candles": [test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0)],
        "intents": [
            {"barIndex": 0, "action": "submit", "id": "same-order-id", "side": "buy", "orderType": "market", "quantity": "1"},
            {"barIndex": 0, "action": "submit", "id": "same-order-id", "side": "buy", "orderType": "market", "quantity": "1"}
        ]
    });
    assert!(
        run_single_case(dup_intent_id)
            .unwrap_err()
            .contains("duplicate order intent id same-order-id")
    );
}

#[test]
fn market_order_fills_on_next_open_with_liquidity_cap() {
    let case = json!({
        "id": "next-open-liquidity-cap",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "10000",
        "processOrdersOnClose": false,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0),
            test_bar(1, 101.0, 102.0, 100.0, 101.0, 100.0),
            test_bar(2, 102.0, 103.0, 101.0, 102.0, 1000.0),
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "next-open",
                "side": "buy",
                "orderType": "market",
                "quantity": "50"
            }
        ]
    });

    let res = run_single_case(case).expect("run case");

    assert_eq!(res["status"], "completed");
    assert_eq!(res["processedBars"], 3);
    assert_eq!(res["cash"], "4910");
    assert_eq!(res["basePosition"], "50");
    assert_eq!(res["finalEquity"], "10010");
    assert_eq!(res["realizedPnl"], "0");
    assert_eq!(res["totalBrokerFees"], "0");
    assert_eq!(res["totalMarketFees"], "0");
    assert_eq!(res["totalFees"], "0");
    assert_eq!(res["totalFills"], 2);
    assert_eq!(res["totalTrades"], 0);
    assert_eq!(res["winningTrades"], 0);
    assert_eq!(res["winRate"], "0");
    assert_eq!(res["maxDrawdown"], "0");
    assert_eq!(res["currentDrawdown"], "0");

    let orders = res["orders"].as_array().expect("orders");
    assert_eq!(orders.len(), 1);
    assert_order(
        &orders[0],
        &ExpectedOrder {
            order_id: "1100000001",

            client_order_id: "next-open",

            side: "buy",

            order_type: "market",

            quantity: "50",

            status: "FILLED",

            filled_quantity: "50",

            filled_price: "101.8",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "2026-06-29T09:32:00Z",

            reduce_only: false,
        },
    );

    let fills = res["fills"].as_array().expect("fills");
    assert_eq!(fills.len(), 2);
    assert_fill(
        &fills[0],
        &ExpectedFill {
            trade_id: "1200000001",

            order_id: "1100000001",

            client_order_id: "next-open",

            side: "buy",

            price: "101",

            quantity: "10",

            quote_quantity: "1010",

            time: "2026-06-29T09:31:00Z",

            maker: false,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "0",
        },
    );
    assert_fill(
        &fills[1],
        &ExpectedFill {
            trade_id: "1200000002",

            order_id: "1100000001",

            client_order_id: "next-open",

            side: "buy",

            price: "102",

            quantity: "40",

            quote_quantity: "4080",

            time: "2026-06-29T09:32:00Z",

            maker: false,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "0",
        },
    );

    assert!(res["warnings"].as_array().unwrap().is_empty());
}

#[test]
fn parent_bracket_runs_atomically_with_stop_first_protection() {
    let case = json!({
        "id": "atomic-parent-bracket-stop-first",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "10000",
        "processOrdersOnClose": false,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0),
            test_bar(1, 100.0, 101.0, 99.0, 100.0, 1000.0),
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "Long",
                "side": "buy",
                "orderType": "market",
                "quantity": "1",
                "atomicGroupId": "parent-bracket"
            },
            {
                "barIndex": 0,
                "action": "submit",
                "id": "XL:limit",
                "parentId": "Long",
                "side": "sell",
                "orderType": "limit",
                "limitPrice": "100.3",
                "quantity": "1",
                "atomicGroupId": "parent-bracket",
                "ocoGroupId": "XL-oco",
                "reduceOnly": true
            },
            {
                "barIndex": 0,
                "action": "submit",
                "id": "XL:stop",
                "parentId": "Long",
                "side": "sell",
                "orderType": "stop_market",
                "stopPrice": "99.9",
                "quantity": "1",
                "atomicGroupId": "parent-bracket",
                "ocoGroupId": "XL-oco",
                "reduceOnly": true
            }
        ]
    });

    let res = run_single_case(case).expect("run case");

    assert_eq!(res["status"], "completed");
    assert_eq!(res["processedBars"], 2);
    assert_eq!(res["cash"], "9999.9");
    assert_eq!(res["basePosition"], "0");
    assert_eq!(res["finalEquity"], "9999.9");
    assert_eq!(res["realizedPnl"], "-0.1");
    assert_eq!(res["totalBrokerFees"], "0");
    assert_eq!(res["totalMarketFees"], "0");
    assert_eq!(res["totalFees"], "0");
    assert_eq!(res["totalFills"], 2);
    assert_eq!(res["totalTrades"], 1);
    assert_eq!(res["winningTrades"], 0);
    assert_eq!(res["winRate"], "0");
    assert_eq!(res["maxDrawdown"], "0.00001");
    assert_eq!(res["currentDrawdown"], "0.00001");
    assert!(res["warnings"].as_array().unwrap().is_empty());

    let orders = res["orders"].as_array().expect("orders");
    assert_eq!(orders.len(), 3);
    assert_order(
        &orders[0],
        &ExpectedOrder {
            order_id: "1100000001",

            client_order_id: "Long",

            side: "buy",

            order_type: "market",

            quantity: "1",

            status: "FILLED",

            filled_quantity: "1",

            filled_price: "100",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "2026-06-29T09:31:00Z",

            reduce_only: false,
        },
    );
    assert_order(
        &orders[1],
        &ExpectedOrder {
            order_id: "1100000002",

            client_order_id: "XL:limit",

            side: "sell",

            order_type: "limit",

            quantity: "1",

            status: "CANCELED",

            filled_quantity: "0",

            filled_price: "0",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "2026-06-29T09:31:00Z",

            reduce_only: true,
        },
    );
    assert_order(
        &orders[2],
        &ExpectedOrder {
            order_id: "1100000003",

            client_order_id: "XL:stop",

            side: "sell",

            order_type: "stop_market",

            quantity: "1",

            status: "FILLED",

            filled_quantity: "1",

            filled_price: "99.9",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "2026-06-29T09:31:00Z",

            reduce_only: true,
        },
    );

    let fills = res["fills"].as_array().expect("fills");
    assert_eq!(fills.len(), 2);
    assert_fill(
        &fills[0],
        &ExpectedFill {
            trade_id: "1200000001",

            order_id: "1100000001",

            client_order_id: "Long",

            side: "buy",

            price: "100",

            quantity: "1",

            quote_quantity: "100",

            time: "2026-06-29T09:31:00Z",

            maker: false,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "0",
        },
    );
    assert_fill(
        &fills[1],
        &ExpectedFill {
            trade_id: "1200000002",

            order_id: "1100000003",

            client_order_id: "XL:stop",

            side: "sell",

            price: "99.9",

            quantity: "1",

            quote_quantity: "99.9",

            time: "2026-06-29T09:31:00Z",

            maker: false,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "-0.1",
        },
    );
}

#[test]
fn atomic_child_without_parent_is_rejected() {
    let case = json!({
        "id": "broken-bracket",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "market": default_market(),
        "candles": [test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0)],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "entry",
                "side": "buy",
                "orderType": "market",
                "quantity": "1",
                "atomicGroupId": "broken-bracket"
            },
            {
                "barIndex": 0,
                "action": "submit",
                "id": "exit",
                "parentId": "missing",
                "side": "sell",
                "orderType": "market",
                "quantity": "1",
                "atomicGroupId": "broken-bracket"
            }
        ]
    });

    let err = run_single_case(case).unwrap_err();
    assert!(err.contains("atomic group broken-bracket child exit has no parent missing"));
}

#[test]
fn atomic_bracket_fills_on_signal_close() {
    let case = json!({
        "id": "close-bracket",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": true,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0)
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "entry",
                "side": "buy",
                "orderType": "market",
                "quantity": "1",
                "atomicGroupId": "close-bracket"
            },
            {
                "barIndex": 0,
                "action": "submit",
                "id": "stop",
                "parentId": "entry",
                "side": "sell",
                "orderType": "stop_market",
                "stopPrice": "101",
                "quantity": "1",
                "atomicGroupId": "close-bracket",
                "ocoGroupId": "protect",
                "reduceOnly": true
            },
            {
                "barIndex": 0,
                "action": "submit",
                "id": "limit",
                "parentId": "entry",
                "side": "sell",
                "orderType": "limit",
                "limitPrice": "99",
                "quantity": "1",
                "atomicGroupId": "close-bracket",
                "ocoGroupId": "protect",
                "reduceOnly": true
            }
        ]
    });

    let res = run_single_case(case).expect("run case");
    assert_eq!(res["status"], "completed");
    assert_eq!(res["processedBars"], 1);
    assert_eq!(res["cash"], "1000");
    assert_eq!(res["basePosition"], "0");
    assert_eq!(res["finalEquity"], "1000");
    assert_eq!(res["realizedPnl"], "0");
    assert_eq!(res["totalBrokerFees"], "0");
    assert_eq!(res["totalMarketFees"], "0");
    assert_eq!(res["totalFees"], "0");
    assert_eq!(res["totalFills"], 2);
    assert_eq!(res["totalTrades"], 1);
    assert_eq!(res["winningTrades"], 0);
    assert_eq!(res["winRate"], "0");
    assert!(res["warnings"].as_array().unwrap().is_empty());

    let orders = res["orders"].as_array().expect("orders");
    assert_eq!(orders.len(), 3);
    assert_order(
        &orders[0],
        &ExpectedOrder {
            order_id: "1100000001",

            client_order_id: "entry",

            side: "buy",

            order_type: "market",

            quantity: "1",

            status: "FILLED",

            filled_quantity: "1",

            filled_price: "100",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "2026-06-29T09:30:59.999Z",

            reduce_only: false,
        },
    );
    assert_order(
        &orders[1],
        &ExpectedOrder {
            order_id: "1100000002",

            client_order_id: "stop",

            side: "sell",

            order_type: "stop_market",

            quantity: "1",

            status: "FILLED",

            filled_quantity: "1",

            filled_price: "100",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "2026-06-29T09:30:59.999Z",

            reduce_only: true,
        },
    );
    assert_order(
        &orders[2],
        &ExpectedOrder {
            order_id: "1100000003",

            client_order_id: "limit",

            side: "sell",

            order_type: "limit",

            quantity: "1",

            status: "CANCELED",

            filled_quantity: "0",

            filled_price: "0",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "2026-06-29T09:30:59.999Z",

            reduce_only: true,
        },
    );

    let fills = res["fills"].as_array().expect("fills");
    assert_eq!(fills.len(), 2);
    assert_fill(
        &fills[0],
        &ExpectedFill {
            trade_id: "1200000001",

            order_id: "1100000001",

            client_order_id: "entry",

            side: "buy",

            price: "100",

            quantity: "1",

            quote_quantity: "100",

            time: "2026-06-29T09:30:59.999Z",

            maker: false,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "0",
        },
    );
    assert_fill(
        &fills[1],
        &ExpectedFill {
            trade_id: "1200000002",

            order_id: "1100000002",

            client_order_id: "stop",

            side: "sell",

            price: "100",

            quantity: "1",

            quote_quantity: "100",

            time: "2026-06-29T09:30:59.999Z",

            maker: false,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "0",
        },
    );
}

#[test]
fn canceling_parent_order_cancels_all_protective_children() {
    let case = json!({
        "id": "cancel-parent-cascade",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": false,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0),
            test_bar(1, 100.0, 101.0, 99.0, 100.0, 1000.0)
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "entry",
                "side": "buy",
                "orderType": "market",
                "quantity": "1",
                "atomicGroupId": "cancel-bracket"
            },
            {
                "barIndex": 0,
                "action": "submit",
                "id": "stop",
                "parentId": "entry",
                "side": "sell",
                "orderType": "stop_market",
                "stopPrice": "95",
                "quantity": "1",
                "atomicGroupId": "cancel-bracket",
                "ocoGroupId": "protect",
                "reduceOnly": true
            },
            {
                "barIndex": 0,
                "action": "submit",
                "id": "limit",
                "parentId": "entry",
                "side": "sell",
                "orderType": "limit",
                "limitPrice": "105",
                "quantity": "1",
                "atomicGroupId": "cancel-bracket",
                "ocoGroupId": "protect",
                "reduceOnly": true
            },
            {
                "barIndex": 0,
                "action": "cancel",
                "targetId": "entry",
                "id": "cancel-entry"
            }
        ]
    });

    let res = run_single_case(case).expect("run case");
    assert_eq!(res["status"], "completed");
    assert_eq!(res["processedBars"], 2);
    assert_eq!(res["cash"], "1000");
    assert_eq!(res["basePosition"], "0");
    assert_eq!(res["finalEquity"], "1000");
    assert_eq!(res["realizedPnl"], "0");
    assert_eq!(res["totalFills"], 0);
    assert_eq!(res["totalTrades"], 0);
    assert!(res["warnings"].as_array().unwrap().is_empty());

    let orders = res["orders"].as_array().expect("orders");
    assert_eq!(orders.len(), 3);
    assert_order(
        &orders[0],
        &ExpectedOrder {
            order_id: "1100000001",

            client_order_id: "entry",

            side: "buy",

            order_type: "market",

            quantity: "1",

            status: "CANCELED",

            filled_quantity: "0",

            filled_price: "0",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "2026-06-29T09:30:59.999Z",

            reduce_only: false,
        },
    );
    assert_order(
        &orders[1],
        &ExpectedOrder {
            order_id: "1100000002",

            client_order_id: "stop",

            side: "sell",

            order_type: "stop_market",

            quantity: "1",

            status: "CANCELED",

            filled_quantity: "0",

            filled_price: "0",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "2026-06-29T09:30:59.999Z",

            reduce_only: true,
        },
    );
    assert_order(
        &orders[2],
        &ExpectedOrder {
            order_id: "1100000003",

            client_order_id: "limit",

            side: "sell",

            order_type: "limit",

            quantity: "1",

            status: "CANCELED",

            filled_quantity: "0",

            filled_price: "0",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "2026-06-29T09:30:59.999Z",

            reduce_only: true,
        },
    );
    assert!(res["fills"].as_array().unwrap().is_empty());
}

#[test]
fn reduce_only_order_without_position_is_canceled() {
    let case = json!({
        "id": "reduce-only-no-pos",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": false,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0),
            test_bar(1, 100.0, 101.0, 99.0, 100.0, 1000.0)
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "reduce-only",
                "side": "sell",
                "orderType": "market",
                "quantity": "1",
                "reduceOnly": true
            }
        ]
    });

    let res = run_single_case(case).expect("run case");
    assert_eq!(res["status"], "completed");
    assert_eq!(res["processedBars"], 2);
    assert_eq!(res["cash"], "1000");
    assert_eq!(res["basePosition"], "0");
    assert_eq!(res["finalEquity"], "1000");
    assert_eq!(res["realizedPnl"], "0");
    assert_eq!(res["totalFills"], 0);
    assert_eq!(res["totalTrades"], 0);
    assert!(res["warnings"].as_array().unwrap().is_empty());

    let orders = res["orders"].as_array().expect("orders");
    assert_eq!(orders.len(), 1);
    assert_order(
        &orders[0],
        &ExpectedOrder {
            order_id: "1100000001",

            client_order_id: "reduce-only",

            side: "sell",

            order_type: "market",

            quantity: "1",

            status: "CANCELED",

            filled_quantity: "0",

            filled_price: "0",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "2026-06-29T09:31:00Z",

            reduce_only: true,
        },
    );
    assert!(res["fills"].as_array().unwrap().is_empty());
}

#[test]
fn reduce_only_fill_is_limited_to_open_position() {
    let case = json!({
        "id": "oversized-reduce-only",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": false,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0),
            test_bar(1, 100.0, 101.0, 99.0, 100.0, 1000.0),
            test_bar(2, 100.0, 101.0, 99.0, 100.0, 1000.0),
            test_bar(3, 100.0, 101.0, 99.0, 100.0, 1000.0)
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "entry-buy",
                "side": "buy",
                "orderType": "market",
                "quantity": "1"
            },
            {
                "barIndex": 1,
                "action": "submit",
                "id": "oversized-reduce-only",
                "side": "sell",
                "orderType": "market",
                "quantity": "2",
                "reduceOnly": true
            }
        ]
    });

    let res = run_single_case(case).expect("run case");
    assert_eq!(res["status"], "completed");
    assert_eq!(res["processedBars"], 4);
    assert_eq!(res["cash"], "1000");
    assert_eq!(res["basePosition"], "0");
    assert_eq!(res["finalEquity"], "1000");
    assert_eq!(res["realizedPnl"], "0");
    assert_eq!(res["totalFills"], 2);
    assert_eq!(res["totalTrades"], 1);
    assert_eq!(res["winningTrades"], 0);
    assert_eq!(res["winRate"], "0");
    assert!(res["warnings"].as_array().unwrap().is_empty());

    let orders = res["orders"].as_array().expect("orders");
    assert_eq!(orders.len(), 2);
    assert_order(
        &orders[0],
        &ExpectedOrder {
            order_id: "1100000001",

            client_order_id: "entry-buy",

            side: "buy",

            order_type: "market",

            quantity: "1",

            status: "FILLED",

            filled_quantity: "1",

            filled_price: "100",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "2026-06-29T09:31:00Z",

            reduce_only: false,
        },
    );
    assert_order(
        &orders[1],
        &ExpectedOrder {
            order_id: "1100000002",

            client_order_id: "oversized-reduce-only",

            side: "sell",

            order_type: "market",

            quantity: "2",

            status: "CANCELED",

            filled_quantity: "1",

            filled_price: "100",

            submitted_at: "2026-06-29T09:31:59.999Z",

            filled_at: "2026-06-29T09:33:00Z",

            reduce_only: true,
        },
    );

    let fills = res["fills"].as_array().expect("fills");
    assert_eq!(fills.len(), 2);
    assert_fill(
        &fills[0],
        &ExpectedFill {
            trade_id: "1200000001",

            order_id: "1100000001",

            client_order_id: "entry-buy",

            side: "buy",

            price: "100",

            quantity: "1",

            quote_quantity: "100",

            time: "2026-06-29T09:31:00Z",

            maker: false,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "0",
        },
    );
    assert_fill(
        &fills[1],
        &ExpectedFill {
            trade_id: "1200000002",

            order_id: "1100000002",

            client_order_id: "oversized-reduce-only",

            side: "sell",

            price: "100",

            quantity: "1",

            quote_quantity: "100",

            time: "2026-06-29T09:32:00Z",

            maker: false,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "0",
        },
    );
}

#[test]
fn explicit_cancel_orders_and_unmatched_target_handling() {
    let case = json!({
        "id": "explicit-cancel",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": false,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0),
            test_bar(1, 100.0, 101.0, 99.0, 100.0, 1000.0)
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "cancel-by-id",
                "side": "buy",
                "orderType": "market",
                "quantity": "1"
            },
            {
                "barIndex": 0,
                "action": "submit",
                "id": "keep",
                "side": "buy",
                "orderType": "market",
                "quantity": "1"
            },
            {
                "barIndex": 0,
                "action": "cancel",
                "targetId": "cancel-by-id",
                "id": "c1"
            },
            {
                "barIndex": 0,
                "action": "cancel",
                "targetId": "missing",
                "id": "c2"
            }
        ]
    });

    let res = run_single_case(case).expect("run case");
    assert_eq!(res["status"], "completed");
    assert_eq!(res["processedBars"], 2);
    assert_eq!(res["cash"], "900");
    assert_eq!(res["basePosition"], "1");
    assert_eq!(res["finalEquity"], "1000");
    assert_eq!(res["realizedPnl"], "0");
    assert_eq!(res["totalFills"], 1);
    assert_eq!(res["totalTrades"], 0);
    assert!(res["warnings"].as_array().unwrap().is_empty());

    let orders = res["orders"].as_array().expect("orders");
    assert_eq!(orders.len(), 2);
    assert_order(
        &orders[0],
        &ExpectedOrder {
            order_id: "1100000001",

            client_order_id: "cancel-by-id",

            side: "buy",

            order_type: "market",

            quantity: "1",

            status: "CANCELED",

            filled_quantity: "0",

            filled_price: "0",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "2026-06-29T09:30:59.999Z",

            reduce_only: false,
        },
    );
    assert_order(
        &orders[1],
        &ExpectedOrder {
            order_id: "1100000002",

            client_order_id: "keep",

            side: "buy",

            order_type: "market",

            quantity: "1",

            status: "FILLED",

            filled_quantity: "1",

            filled_price: "100",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "2026-06-29T09:31:00Z",

            reduce_only: false,
        },
    );

    let fills = res["fills"].as_array().expect("fills");
    assert_eq!(fills.len(), 1);
    assert_fill(
        &fills[0],
        &ExpectedFill {
            trade_id: "1200000001",

            order_id: "1100000002",

            client_order_id: "keep",

            side: "buy",

            price: "100",

            quantity: "1",

            quote_quantity: "100",

            time: "2026-06-29T09:31:00Z",

            maker: false,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "0",
        },
    );
}

#[test]
fn process_orders_on_close_executes_at_signal_close_price() {
    let case = json!({
        "id": "same-close-point",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": true,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 100.0, 106.0, 99.0, 105.0, 1000.0)
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "same-close",
                "side": "buy",
                "orderType": "market",
                "quantity": "2"
            }
        ]
    });

    let res = run_single_case(case).expect("run case");
    assert_eq!(res["status"], "completed");
    assert_eq!(res["processedBars"], 1);
    assert_eq!(res["cash"], "790");
    assert_eq!(res["basePosition"], "2");
    assert_eq!(res["finalEquity"], "1000");
    assert_eq!(res["realizedPnl"], "0");
    assert_eq!(res["totalFills"], 1);
    assert_eq!(res["totalTrades"], 0);
    assert!(res["warnings"].as_array().unwrap().is_empty());

    let orders = res["orders"].as_array().expect("orders");
    assert_eq!(orders.len(), 1);
    assert_order(
        &orders[0],
        &ExpectedOrder {
            order_id: "1100000001",

            client_order_id: "same-close",

            side: "buy",

            order_type: "market",

            quantity: "2",

            status: "FILLED",

            filled_quantity: "2",

            filled_price: "105",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "2026-06-29T09:30:59.999Z",

            reduce_only: false,
        },
    );

    let fills = res["fills"].as_array().expect("fills");
    assert_eq!(fills.len(), 1);
    assert_fill(
        &fills[0],
        &ExpectedFill {
            trade_id: "1200000001",

            order_id: "1100000001",

            client_order_id: "same-close",

            side: "buy",

            price: "105",

            quantity: "2",

            quote_quantity: "210",

            time: "2026-06-29T09:30:59.999Z",

            maker: false,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "0",
        },
    );
}

#[test]
fn sell_market_order_applies_downward_slippage() {
    let case = json!({
        "id": "sell-market-slippage",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": false,
        "slippageTicks": 2,
        "market": {
            "tickSize": "0.05",
            "quantityStep": "1",
            "minQuantity": "1"
        },
        "candles": [
            test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0),
            test_bar(1, 101.0, 102.0, 100.0, 101.0, 1000.0)
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "sell-with-slippage",
                "side": "sell",
                "orderType": "market",
                "quantity": "2"
            }
        ]
    });

    let res = run_single_case(case).expect("run case");
    // Open is 101.00. 2 ticks of 0.05 = 0.10. Slipped sell price = 100.90.
    // 2 * 100.90 = 201.80. Final cash = 1000 + 201.80 = 1201.80.
    // Final equity = 1201.80 + (-2)*101.00 = 999.80.
    assert_eq!(res["status"], "completed");
    assert_eq!(res["processedBars"], 2);
    assert_eq!(res["cash"], "1201.8");
    assert_eq!(res["basePosition"], "-2");
    assert_eq!(res["finalEquity"], "999.8");
    assert_eq!(res["realizedPnl"], "0");
    assert_eq!(res["totalFills"], 1);
    assert_eq!(res["totalTrades"], 0);
    assert!(res["warnings"].as_array().unwrap().is_empty());

    let orders = res["orders"].as_array().expect("orders");
    assert_eq!(orders.len(), 1);
    assert_order(
        &orders[0],
        &ExpectedOrder {
            order_id: "1100000001",

            client_order_id: "sell-with-slippage",

            side: "sell",

            order_type: "market",

            quantity: "2",

            status: "FILLED",

            filled_quantity: "2",

            filled_price: "100.9",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "2026-06-29T09:31:00Z",

            reduce_only: false,
        },
    );

    let fills = res["fills"].as_array().expect("fills");
    assert_eq!(fills.len(), 1);
    assert_fill(
        &fills[0],
        &ExpectedFill {
            trade_id: "1200000001",

            order_id: "1100000001",

            client_order_id: "sell-with-slippage",

            side: "sell",

            price: "100.9",

            quantity: "2",

            quote_quantity: "201.8",

            time: "2026-06-29T09:31:00Z",

            maker: false,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "0",
        },
    );
}

#[test]
fn limit_order_receives_favorable_gap_improvement() {
    let case = json!({
        "id": "buy-limit-gap-improvement",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": false,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 101.0, 102.0, 100.0, 101.0, 1000.0),
            test_bar(1, 99.0, 101.0, 98.0, 100.0, 1000.0)
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "buy-limit",
                "side": "buy",
                "orderType": "limit",
                "limitPrice": "100",
                "quantity": "1"
            }
        ]
    });

    let res = run_single_case(case).expect("run case");
    assert_eq!(res["status"], "completed");
    assert_eq!(res["processedBars"], 2);
    assert_eq!(res["cash"], "901");
    assert_eq!(res["basePosition"], "1");
    assert_eq!(res["finalEquity"], "1001");
    assert_eq!(res["realizedPnl"], "0");
    assert_eq!(res["totalFills"], 1);
    assert_eq!(res["totalTrades"], 0);
    assert!(res["warnings"].as_array().unwrap().is_empty());

    let orders = res["orders"].as_array().expect("orders");
    assert_eq!(orders.len(), 1);
    assert_order(
        &orders[0],
        &ExpectedOrder {
            order_id: "1100000001",

            client_order_id: "buy-limit",

            side: "buy",

            order_type: "limit",

            quantity: "1",

            status: "FILLED",

            filled_quantity: "1",

            filled_price: "99",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "2026-06-29T09:31:00Z",

            reduce_only: false,
        },
    );

    let fills = res["fills"].as_array().expect("fills");
    assert_eq!(fills.len(), 1);
    assert_fill(
        &fills[0],
        &ExpectedFill {
            trade_id: "1200000001",

            order_id: "1100000001",

            client_order_id: "buy-limit",

            side: "buy",

            price: "99",

            quantity: "1",

            quote_quantity: "99",

            time: "2026-06-29T09:31:00Z",

            maker: true,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "0",
        },
    );
}

#[test]
fn limit_sell_open_improvement_intrabar_and_close_point() {
    // 1. Close-point fill
    let close_case = json!({
        "id": "sell-limit-close",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": true,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 100.0, 106.0, 99.0, 105.0, 1000.0)
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "sell-limit-close",
                "side": "sell",
                "orderType": "limit",
                "limitPrice": "104",
                "quantity": "1"
            }
        ]
    });
    let res_close = run_single_case(close_case).expect("run case");
    assert_eq!(res_close["cash"], "1105");
    assert_eq!(res_close["basePosition"], "-1");
    assert_eq!(res_close["finalEquity"], "1000");
    assert_eq!(res_close["totalFills"], 1);
    assert_order(
        &res_close["orders"][0],
        &ExpectedOrder {
            order_id: "1100000001",

            client_order_id: "sell-limit-close",

            side: "sell",

            order_type: "limit",

            quantity: "1",

            status: "FILLED",

            filled_quantity: "1",

            filled_price: "105",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "2026-06-29T09:30:59.999Z",

            reduce_only: false,
        },
    );
    assert_fill(
        &res_close["fills"][0],
        &ExpectedFill {
            trade_id: "1200000001",

            order_id: "1100000001",

            client_order_id: "sell-limit-close",

            side: "sell",

            price: "105",

            quantity: "1",

            quote_quantity: "105",

            time: "2026-06-29T09:30:59.999Z",

            maker: true,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "0",
        },
    );

    // 2. Open improvement (open 102 >= limit 101)
    let open_improvement_case = json!({
        "id": "sell-limit-open-imp",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": false,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0),
            test_bar(1, 102.0, 103.0, 101.0, 102.0, 1000.0)
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "sell-limit-open",
                "side": "sell",
                "orderType": "limit",
                "limitPrice": "101",
                "quantity": "1"
            }
        ]
    });
    let res_open = run_single_case(open_improvement_case).expect("run case");
    assert_eq!(res_open["cash"], "1102");
    assert_eq!(res_open["basePosition"], "-1");
    assert_eq!(res_open["finalEquity"], "1000");
    assert_eq!(res_open["totalFills"], 1);
    assert_order(
        &res_open["orders"][0],
        &ExpectedOrder {
            order_id: "1100000001",

            client_order_id: "sell-limit-open",

            side: "sell",

            order_type: "limit",

            quantity: "1",

            status: "FILLED",

            filled_quantity: "1",

            filled_price: "102",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "2026-06-29T09:31:00Z",

            reduce_only: false,
        },
    );
    assert_fill(
        &res_open["fills"][0],
        &ExpectedFill {
            trade_id: "1200000001",

            order_id: "1100000001",

            client_order_id: "sell-limit-open",

            side: "sell",

            price: "102",

            quantity: "1",

            quote_quantity: "102",

            time: "2026-06-29T09:31:00Z",

            maker: true,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "0",
        },
    );

    // 3. Intrabar fill (open 100 < limit 103 <= high 104)
    let intrabar_case = json!({
        "id": "sell-limit-intrabar",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": false,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0),
            test_bar(1, 100.0, 104.0, 99.0, 101.0, 1000.0)
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "sell-limit-intra",
                "side": "sell",
                "orderType": "limit",
                "limitPrice": "103",
                "quantity": "1"
            }
        ]
    });
    let res_intra = run_single_case(intrabar_case).expect("run case");
    assert_eq!(res_intra["cash"], "1103");
    assert_eq!(res_intra["basePosition"], "-1");
    assert_eq!(res_intra["finalEquity"], "1002"); // 1103 - 1*101
    assert_eq!(res_intra["totalFills"], 1);
    assert_order(
        &res_intra["orders"][0],
        &ExpectedOrder {
            order_id: "1100000001",

            client_order_id: "sell-limit-intra",

            side: "sell",

            order_type: "limit",

            quantity: "1",

            status: "FILLED",

            filled_quantity: "1",

            filled_price: "103",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "2026-06-29T09:31:00Z",

            reduce_only: false,
        },
    );
    assert_fill(
        &res_intra["fills"][0],
        &ExpectedFill {
            trade_id: "1200000001",

            order_id: "1100000001",

            client_order_id: "sell-limit-intra",

            side: "sell",

            price: "103",

            quantity: "1",

            quote_quantity: "103",

            time: "2026-06-29T09:31:00Z",

            maker: true,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "0",
        },
    );
}

#[test]
fn stop_market_and_stop_limit_orders_execution() {
    let case = json!({
        "id": "stop-orders-execution",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": false,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0),
            test_bar(1, 106.0, 107.0, 94.0, 100.0, 1000.0),
            test_bar(2, 102.0, 103.0, 101.0, 102.0, 1000.0)
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "buy-stop-market",
                "side": "buy",
                "orderType": "stop_market",
                "stopPrice": "105",
                "quantity": "1"
            },
            {
                "barIndex": 0,
                "action": "submit",
                "id": "sell-stop-market",
                "side": "sell",
                "orderType": "stop_market",
                "stopPrice": "95",
                "quantity": "1"
            },
            {
                "barIndex": 0,
                "action": "submit",
                "id": "stop-limit",
                "side": "buy",
                "orderType": "stop_limit",
                "stopPrice": "104",
                "limitPrice": "103",
                "quantity": "1"
            }
        ]
    });

    let res = run_single_case(case).expect("run case");
    assert_eq!(res["status"], "completed");
    assert_eq!(res["processedBars"], 3);
    assert_eq!(res["cash"], "887"); // 1000 - 106 + 95 - 102 = 887
    assert_eq!(res["basePosition"], "1"); // 1 - 1 + 1 = 1
    assert_eq!(res["finalEquity"], "989"); // 887 + 1*102 = 989
    assert_eq!(res["realizedPnl"], "-11");
    assert_eq!(res["totalBrokerFees"], "0");
    assert_eq!(res["totalMarketFees"], "0");
    assert_eq!(res["totalFees"], "0");
    assert_eq!(res["totalFills"], 3);
    assert_eq!(res["totalTrades"], 1);
    assert_eq!(res["winningTrades"], 0);
    assert_eq!(res["winRate"], "0");
    assert!(res["warnings"].as_array().unwrap().is_empty());

    let orders = res["orders"].as_array().expect("orders");
    assert_eq!(orders.len(), 3);
    assert_order(
        &orders[0],
        &ExpectedOrder {
            order_id: "1100000001",

            client_order_id: "buy-stop-market",

            side: "buy",

            order_type: "stop_market",

            quantity: "1",

            status: "FILLED",

            filled_quantity: "1",

            filled_price: "106",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "2026-06-29T09:31:00Z",

            reduce_only: false,
        },
    );
    assert_order(
        &orders[1],
        &ExpectedOrder {
            order_id: "1100000002",

            client_order_id: "sell-stop-market",

            side: "sell",

            order_type: "stop_market",

            quantity: "1",

            status: "FILLED",

            filled_quantity: "1",

            filled_price: "95",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "2026-06-29T09:31:00Z",

            reduce_only: false,
        },
    );
    assert_order(
        &orders[2],
        &ExpectedOrder {
            order_id: "1100000003",

            client_order_id: "stop-limit",

            side: "buy",

            order_type: "stop_limit",

            quantity: "1",

            status: "FILLED",

            filled_quantity: "1",

            filled_price: "102",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "2026-06-29T09:32:00Z",

            reduce_only: false,
        },
    );

    let fills = res["fills"].as_array().expect("fills");
    assert_eq!(fills.len(), 3);
    assert_fill(
        &fills[0],
        &ExpectedFill {
            trade_id: "1200000001",

            order_id: "1100000001",

            client_order_id: "buy-stop-market",

            side: "buy",

            price: "106",

            quantity: "1",

            quote_quantity: "106",

            time: "2026-06-29T09:31:00Z",

            maker: false,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "0",
        },
    );
    assert_fill(
        &fills[1],
        &ExpectedFill {
            trade_id: "1200000002",

            order_id: "1100000002",

            client_order_id: "sell-stop-market",

            side: "sell",

            price: "95",

            quantity: "1",

            quote_quantity: "95",

            time: "2026-06-29T09:31:00Z",

            maker: false,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "-11",
        },
    );
    assert_fill(
        &fills[2],
        &ExpectedFill {
            trade_id: "1200000003",

            order_id: "1100000003",

            client_order_id: "stop-limit",

            side: "buy",

            price: "102",

            quantity: "1",

            quote_quantity: "102",

            time: "2026-06-29T09:32:00Z",

            maker: true,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "0",
        },
    );
}

#[test]
fn warnings_emitted_and_deduplicated_for_zero_volume_and_unsupported_order() {
    let case = json!({
        "id": "warnings-and-unsupported",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": false,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 100.0, 101.0, 99.0, 100.0, 0.0),
            test_bar(1, 100.0, 101.0, 99.0, 100.0, 0.0),
            test_bar(2, 100.0, 101.0, 99.0, 100.0, 1000.0)
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "unsupported",
                "side": "buy",
                "orderType": "TRAILING_STOP",
                "quantity": "1"
            }
        ]
    });

    let res = run_single_case(case).expect("run case");
    assert_eq!(res["status"], "completed");
    assert_eq!(res["processedBars"], 3);
    assert_eq!(res["cash"], "1000");
    assert_eq!(res["basePosition"], "0");
    assert_eq!(res["finalEquity"], "1000");
    assert_eq!(res["totalFills"], 0);

    let warnings = res["warnings"].as_array().expect("warnings");
    assert_eq!(warnings.len(), 2);
    assert_eq!(
        warnings[0].as_str().unwrap(),
        "conservative-bar-v1: US.AAPL bar ending 2026-06-29T09:31:59.999Z has no positive volume; pending orders cannot fill on this bar"
    );
    assert_eq!(
        warnings[1].as_str().unwrap(),
        "conservative-bar-v1: unsupported order type TRAILING_STOP remains pending"
    );

    let orders = res["orders"].as_array().expect("orders");
    assert_eq!(orders.len(), 1);
    assert_order(
        &orders[0],
        &ExpectedOrder {
            order_id: "1100000001",

            client_order_id: "unsupported",

            side: "buy",

            order_type: "TRAILING_STOP",

            quantity: "1",

            status: "NEW",

            filled_quantity: "0",

            filled_price: "0",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "",

            reduce_only: false,
        },
    );
    assert!(res["fills"].as_array().unwrap().is_empty());
}

#[test]
fn liquidity_warnings_for_sub_step_and_below_min_quantity() {
    // 1. Below quantity step
    let step_case = json!({
        "id": "below-step-warning",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": false,
        "slippageTicks": 0,
        "market": {
            "tickSize": "0.01",
            "quantityStep": "1",
            "minQuantity": "0.01"
        },
        "candles": [
            test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0),
            test_bar(1, 100.0, 101.0, 99.0, 100.0, 0.5) // 10% volume budget = 0.05, step is 1 -> 0
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "below-step",
                "side": "buy",
                "orderType": "market",
                "quantity": "1"
            }
        ]
    });
    let res_step = run_single_case(step_case).expect("run case");
    assert_eq!(res_step["status"], "completed");
    assert_eq!(res_step["processedBars"], 2);
    assert_eq!(res_step["cash"], "1000");
    assert_eq!(res_step["basePosition"], "0");
    assert_eq!(res_step["finalEquity"], "1000");
    assert_eq!(res_step["totalFills"], 0);

    let warnings_step = res_step["warnings"].as_array().expect("warnings");
    assert_eq!(warnings_step.len(), 1);
    assert_eq!(
        warnings_step[0].as_str().unwrap(),
        "conservative-bar-v1: liquidity budget for US.AAPL is below tradable quantity step; order below-step remains pending"
    );
    assert_order(
        &res_step["orders"][0],
        &ExpectedOrder {
            order_id: "1100000001",

            client_order_id: "below-step",

            side: "buy",

            order_type: "market",

            quantity: "1",

            status: "NEW",

            filled_quantity: "0",

            filled_price: "0",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "",

            reduce_only: false,
        },
    );

    // 2. Below min quantity
    let min_case = json!({
        "id": "below-min-warning",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": false,
        "slippageTicks": 0,
        "market": {
            "tickSize": "0.01",
            "quantityStep": "1",
            "minQuantity": "2"
        },
        "candles": [
            test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0),
            test_bar(1, 100.0, 101.0, 99.0, 100.0, 10.0) // Go baseline: 10% volume budget = 1 share
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "below-min",
                "side": "buy",
                "orderType": "market",
                "quantity": "1" // requested 1 share, but min quantity is 2 -> remaining 1 share < minQuantity 2
            }
        ]
    });
    // With candle volume 10.0, 10% budget = 1.0. Quantity requested = 1. Truncate to step = 1.
    // 1 < minQuantity 2 -> triggers minQuantity warning!
    let res_min = run_single_case(min_case).expect("run case");
    assert_eq!(res_min["status"], "completed");
    assert_eq!(res_min["processedBars"], 2);
    assert_eq!(res_min["cash"], "1000");
    assert_eq!(res_min["basePosition"], "0");
    assert_eq!(res_min["finalEquity"], "1000");
    assert_eq!(res_min["totalFills"], 0);

    let warnings_min = res_min["warnings"].as_array().expect("warnings");
    assert_eq!(warnings_min.len(), 1);
    assert_eq!(
        warnings_min[0].as_str().unwrap(),
        "conservative-bar-v1: liquidity budget for US.AAPL is below min quantity 2; order below-min remains pending"
    );
    assert_order(
        &res_min["orders"][0],
        &ExpectedOrder {
            order_id: "1100000001",

            client_order_id: "below-min",

            side: "buy",

            order_type: "market",

            quantity: "1",

            status: "NEW",

            filled_quantity: "0",

            filled_price: "0",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "",

            reduce_only: false,
        },
    );
}

#[test]
fn matching_and_pricing_helper_branches_behavior() {
    // 1. Negative slipped price clamps safely
    let neg_slippage_case = json!({
        "id": "neg-slippage-clamp",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": false,
        "slippageTicks": 10,
        "market": {
            "tickSize": "1",
            "quantityStep": "1",
            "minQuantity": "1"
        },
        "candles": [
            test_bar(0, 10.0, 11.0, 9.0, 10.0, 1000.0),
            test_bar(1, 0.5, 1.0, 0.1, 0.5, 1000.0) // Sell slipped = 0.5 - 10 <= 0 -> 0 -> unfillable
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "sell-slipped-zero",
                "side": "sell",
                "orderType": "market",
                "quantity": "1"
            }
        ]
    });
    let res_neg = run_single_case(neg_slippage_case).expect("run case");
    assert_eq!(res_neg["totalFills"], 0);
    assert_order(
        &res_neg["orders"][0],
        &ExpectedOrder {
            order_id: "1100000001",

            client_order_id: "sell-slipped-zero",

            side: "sell",

            order_type: "market",

            quantity: "1",

            status: "NEW",

            filled_quantity: "0",

            filled_price: "0",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "",

            reduce_only: false,
        },
    );

    // 2. Buy upward slippage
    let buy_slippage_case = json!({
        "id": "buy-slippage",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": false,
        "slippageTicks": 1,
        "market": {
            "tickSize": "1",
            "quantityStep": "1",
            "minQuantity": "1"
        },
        "candles": [
            test_bar(0, 10.0, 11.0, 9.0, 10.0, 1000.0),
            test_bar(1, 1.0, 2.0, 0.5, 1.0, 1000.0) // Buy slipped = 1.0 + 1.0 = 2.0
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "buy-slipped",
                "side": "buy",
                "orderType": "market",
                "quantity": "1"
            }
        ]
    });
    let res_buy = run_single_case(buy_slippage_case).expect("run case");
    assert_eq!(res_buy["cash"], "998");
    assert_eq!(res_buy["basePosition"], "1");
    assert_eq!(res_buy["finalEquity"], "999"); // 998 + 1*1
    assert_eq!(res_buy["totalFills"], 1);
    assert_order(
        &res_buy["orders"][0],
        &ExpectedOrder {
            order_id: "1100000001",

            client_order_id: "buy-slipped",

            side: "buy",

            order_type: "market",

            quantity: "1",

            status: "FILLED",

            filled_quantity: "1",

            filled_price: "2",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "2026-06-29T09:31:00Z",

            reduce_only: false,
        },
    );
    assert_fill(
        &res_buy["fills"][0],
        &ExpectedFill {
            trade_id: "1200000001",

            order_id: "1100000001",

            client_order_id: "buy-slipped",

            side: "buy",

            price: "2",

            quantity: "1",

            quote_quantity: "2",

            time: "2026-06-29T09:31:00Z",

            maker: false,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "0",
        },
    );

    // 3. Unreachable stop market does not trigger
    let unreachable_stop_case = json!({
        "id": "unreachable-stop",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": false,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0),
            test_bar(1, 100.0, 105.0, 95.0, 101.0, 1000.0)
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "unreachable-stop",
                "side": "buy",
                "orderType": "stop_market",
                "stopPrice": "200",
                "quantity": "1"
            }
        ]
    });
    let res_stop = run_single_case(unreachable_stop_case).expect("run case");
    assert_eq!(res_stop["totalFills"], 0);
    assert_order(
        &res_stop["orders"][0],
        &ExpectedOrder {
            order_id: "1100000001",

            client_order_id: "unreachable-stop",

            side: "buy",

            order_type: "stop_market",

            quantity: "1",

            status: "NEW",

            filled_quantity: "0",

            filled_price: "0",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "",

            reduce_only: false,
        },
    );

    // 4. Unfilled limit order
    let unfilled_limit_case = json!({
        "id": "unfilled-limit",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": false,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0),
            test_bar(1, 100.0, 105.0, 95.0, 101.0, 1000.0)
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "unfilled-limit",
                "side": "buy",
                "orderType": "limit",
                "limitPrice": "90",
                "quantity": "1"
            }
        ]
    });
    let res_limit = run_single_case(unfilled_limit_case).expect("run case");
    assert_eq!(res_limit["totalFills"], 0);
    assert_order(
        &res_limit["orders"][0],
        &ExpectedOrder {
            order_id: "1100000001",

            client_order_id: "unfilled-limit",

            side: "buy",

            order_type: "limit",

            quantity: "1",

            status: "NEW",

            filled_quantity: "0",

            filled_price: "0",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "",

            reduce_only: false,
        },
    );

    // 5. Buy stop close-point trigger (close 100 >= stop 100)
    let buy_stop_close_case = json!({
        "id": "buy-stop-close",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": true,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 99.0, 101.0, 98.0, 100.0, 100.0)
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "buy-stop-close-order",
                "side": "buy",
                "orderType": "stop_market",
                "stopPrice": "100",
                "quantity": "1"
            }
        ]
    });
    let res_bsc = run_single_case(buy_stop_close_case).expect("run case");
    assert_eq!(res_bsc["totalFills"], 1);
    assert_fill(
        &res_bsc["fills"][0],
        &ExpectedFill {
            trade_id: "1200000001",

            order_id: "1100000001",

            client_order_id: "buy-stop-close-order",

            side: "buy",

            price: "100",

            quantity: "1",

            quote_quantity: "100",

            time: "2026-06-29T09:30:59.999Z",

            maker: false,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "0",
        },
    );

    // 6. Sell stop close-point trigger (close 100 <= stop 100)
    let sell_stop_close_case = json!({
        "id": "sell-stop-close",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": true,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 101.0, 102.0, 99.0, 100.0, 100.0)
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "sell-stop-close-order",
                "side": "sell",
                "orderType": "stop_market",
                "stopPrice": "100",
                "quantity": "1"
            }
        ]
    });
    let res_ssc = run_single_case(sell_stop_close_case).expect("run case");
    assert_eq!(res_ssc["totalFills"], 1);
    assert_fill(
        &res_ssc["fills"][0],
        &ExpectedFill {
            trade_id: "1200000001",

            order_id: "1100000001",

            client_order_id: "sell-stop-close-order",

            side: "sell",

            price: "100",

            quantity: "1",

            quote_quantity: "100",

            time: "2026-06-29T09:30:59.999Z",

            maker: false,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "0",
        },
    );

    // 7. Buy stop full-bar intrabar trigger (open 100 < stop 104 <= high 105)
    let buy_stop_intra_case = json!({
        "id": "buy-stop-intra",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": false,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0),
            test_bar(1, 100.0, 105.0, 99.0, 101.0, 100.0)
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "buy-stop-intra-order",
                "side": "buy",
                "orderType": "stop_market",
                "stopPrice": "104",
                "quantity": "1"
            }
        ]
    });
    let res_bsi = run_single_case(buy_stop_intra_case).expect("run case");
    assert_eq!(res_bsi["totalFills"], 1);
    assert_fill(
        &res_bsi["fills"][0],
        &ExpectedFill {
            trade_id: "1200000001",

            order_id: "1100000001",

            client_order_id: "buy-stop-intra-order",

            side: "buy",

            price: "104",

            quantity: "1",

            quote_quantity: "104",

            time: "2026-06-29T09:31:00Z",

            maker: false,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "0",
        },
    );

    // 8. Sell stop full-bar open trigger (open 99 <= stop 100)
    let sell_stop_open_case = json!({
        "id": "sell-stop-open",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": false,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0),
            test_bar(1, 99.0, 101.0, 98.0, 100.0, 100.0)
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "sell-stop-open-order",
                "side": "sell",
                "orderType": "stop_market",
                "stopPrice": "100",
                "quantity": "1"
            }
        ]
    });
    let res_sso = run_single_case(sell_stop_open_case).expect("run case");
    assert_eq!(res_sso["totalFills"], 1);
    assert_fill(
        &res_sso["fills"][0],
        &ExpectedFill {
            trade_id: "1200000001",

            order_id: "1100000001",

            client_order_id: "sell-stop-open-order",

            side: "sell",

            price: "99",

            quantity: "1",

            quote_quantity: "99",

            time: "2026-06-29T09:31:00Z",

            maker: false,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "0",
        },
    );

    // 9. Buy limit close-point fill (close 101 <= limit 102)
    let buy_limit_close_case = json!({
        "id": "buy-limit-close",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": true,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 100.0, 101.0, 99.0, 101.0, 100.0)
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "buy-limit-close-order",
                "side": "buy",
                "orderType": "limit",
                "limitPrice": "102",
                "quantity": "1"
            }
        ]
    });
    let res_blc = run_single_case(buy_limit_close_case).expect("run case");
    assert_eq!(res_blc["totalFills"], 1);
    assert_fill(
        &res_blc["fills"][0],
        &ExpectedFill {
            trade_id: "1200000001",

            order_id: "1100000001",

            client_order_id: "buy-limit-close-order",

            side: "buy",

            price: "101",

            quantity: "1",

            quote_quantity: "101",

            time: "2026-06-29T09:30:59.999Z",

            maker: true,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "0",
        },
    );

    // 10. Buy limit full-bar intrabar fill (open 100 > limit 98, low 97 <= limit 98)
    let buy_limit_intra_case = json!({
        "id": "buy-limit-intra",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": false,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0),
            test_bar(1, 100.0, 101.0, 97.0, 99.0, 100.0)
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "buy-limit-intra-order",
                "side": "buy",
                "orderType": "limit",
                "limitPrice": "98",
                "quantity": "1"
            }
        ]
    });
    let res_bli = run_single_case(buy_limit_intra_case).expect("run case");
    assert_eq!(res_bli["totalFills"], 1);
    assert_fill(
        &res_bli["fills"][0],
        &ExpectedFill {
            trade_id: "1200000001",

            order_id: "1100000001",

            client_order_id: "buy-limit-intra-order",

            side: "buy",

            price: "98",

            quantity: "1",

            quote_quantity: "98",

            time: "2026-06-29T09:31:00Z",

            maker: true,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "0",
        },
    );

    // 11. Untriggered stop-limit order does not match before stop price is reached
    let untriggered_stop_limit_case = json!({
        "id": "untriggered-stop-limit",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": false,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0),
            test_bar(1, 100.0, 105.0, 95.0, 101.0, 1000.0)
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "untriggered-sl",
                "side": "buy",
                "orderType": "stop_limit",
                "stopPrice": "200",
                "limitPrice": "100",
                "quantity": "1"
            }
        ]
    });
    let res_usl = run_single_case(untriggered_stop_limit_case).expect("run case");
    assert_eq!(res_usl["totalFills"], 0);
    assert_order(
        &res_usl["orders"][0],
        &ExpectedOrder {
            order_id: "1100000001",

            client_order_id: "untriggered-sl",

            side: "buy",

            order_type: "stop_limit",

            quantity: "1",

            status: "NEW",

            filled_quantity: "0",

            filled_price: "0",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "",

            reduce_only: false,
        },
    );

    // 12. Zero-volume bar deferral
    let zero_vol_defer_case = json!({
        "id": "zero-vol-defer",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "10000",
        "processOrdersOnClose": false,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0),
            test_bar(1, 100.0, 101.0, 99.0, 100.0, 0.0),
            test_bar(2, 100.0, 101.0, 99.0, 100.0, 1000.0)
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "defer-buy",
                "side": "buy",
                "orderType": "market",
                "quantity": "10"
            }
        ]
    });
    let res_zvd = run_single_case(zero_vol_defer_case).expect("run case");
    assert_eq!(res_zvd["totalFills"], 1);
    assert_eq!(res_zvd["warnings"].as_array().unwrap().len(), 1);
    assert_fill(
        &res_zvd["fills"][0],
        &ExpectedFill {
            trade_id: "1200000001",

            order_id: "1100000001",

            client_order_id: "defer-buy",

            side: "buy",

            price: "100",

            quantity: "10",

            quote_quantity: "1000",

            time: "2026-06-29T09:32:00Z",

            maker: false,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "0",
        },
    );
}

#[test]
fn cancel_skips_unmatched_pending_orders_without_side_effects() {
    let case = json!({
        "id": "cancel-unmatched-skip",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": false,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0),
            test_bar(1, 100.0, 101.0, 99.0, 100.0, 1000.0)
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "order-to-cancel",
                "side": "buy",
                "orderType": "market",
                "quantity": "1"
            },
            {
                "barIndex": 0,
                "action": "submit",
                "id": "order-to-fill",
                "side": "buy",
                "orderType": "market",
                "quantity": "1"
            },
            {
                "barIndex": 0,
                "action": "submit",
                "id": "order-unfilled-limit",
                "side": "buy",
                "orderType": "limit",
                "limitPrice": "80",
                "quantity": "1"
            },
            {
                "barIndex": 0,
                "action": "cancel",
                "targetId": "order-to-cancel",
                "id": "c-explicit"
            },
            {
                "barIndex": 0,
                "action": "cancel",
                "targetId": "nonexistent-3",
                "id": "c-nonexistent"
            },
            {
                "barIndex": 1,
                "action": "cancel",
                "targetId": "order-to-fill",
                "id": "c-already-filled"
            }
        ]
    });

    let res = run_single_case(case).expect("run case");
    assert_eq!(res["status"], "completed");
    assert_eq!(res["processedBars"], 2);
    assert_eq!(res["cash"], "900");
    assert_eq!(res["basePosition"], "1");
    assert_eq!(res["finalEquity"], "1000");
    assert_eq!(res["totalFills"], 1);
    assert!(res["warnings"].as_array().unwrap().is_empty());

    let orders = res["orders"].as_array().expect("orders");
    assert_eq!(orders.len(), 3);
    assert_order(
        &orders[0],
        &ExpectedOrder {
            order_id: "1100000001",

            client_order_id: "order-to-cancel",

            side: "buy",

            order_type: "market",

            quantity: "1",

            status: "CANCELED",

            filled_quantity: "0",

            filled_price: "0",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "2026-06-29T09:30:59.999Z",

            reduce_only: false,
        },
    );
    assert_order(
        &orders[1],
        &ExpectedOrder {
            order_id: "1100000002",

            client_order_id: "order-to-fill",

            side: "buy",

            order_type: "market",

            quantity: "1",

            status: "FILLED",

            filled_quantity: "1",

            filled_price: "100",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "2026-06-29T09:31:00Z",

            reduce_only: false,
        },
    );
    assert_order(
        &orders[2],
        &ExpectedOrder {
            order_id: "1100000003",

            client_order_id: "order-unfilled-limit",

            side: "buy",

            order_type: "limit",

            quantity: "1",

            status: "NEW",

            filled_quantity: "0",

            filled_price: "0",

            submitted_at: "2026-06-29T09:30:59.999Z",

            filled_at: "",

            reduce_only: false,
        },
    );

    let fills = res["fills"].as_array().expect("fills");
    assert_eq!(fills.len(), 1);
    assert_fill(
        &fills[0],
        &ExpectedFill {
            trade_id: "1200000001",

            order_id: "1100000002",

            client_order_id: "order-to-fill",

            side: "buy",

            price: "100",

            quantity: "1",

            quote_quantity: "100",

            time: "2026-06-29T09:31:00Z",

            maker: false,

            broker_fee: "0",

            market_fee: "0",

            total_fee: "0",

            realized_pnl: "0",
        },
    );
}

#[test]
fn strongly_typed_corpus_input_runs_successfully() {
    let raw_case = json!({
        "id": "typed-case",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "10000",
        "processOrdersOnClose": false,
        "slippageTicks": 0,
        "market": default_market(),
        "feeRules": [],
        "indicatorPeriods": [5],
        "candles": [
            test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0),
            test_bar(1, 101.0, 102.0, 100.0, 101.0, 1000.0)
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "typed-buy",
                "side": "buy",
                "orderType": "market",
                "quantity": "10"
            }
        ]
    });

    let corpus_input: CorpusInput = serde_json::from_value(json!({
        "version": 1,
        "cases": [raw_case]
    }))
    .expect("deserialize typed corpus");

    let output = run_corpus(&corpus_input).expect("run strongly typed corpus");
    assert_eq!(output.version, 1);
    assert_eq!(output.execution_model, "conservative-bar-v1");
    assert_eq!(output.cases.len(), 1);

    let case_out = &output.cases[0];
    assert_eq!(case_out.id, "typed-case");
    assert_eq!(case_out.cash, "8990");
    assert_eq!(case_out.base_position, "10");
    assert_eq!(case_out.final_equity, "10000"); // 8990 + 10 * 101
    assert_eq!(case_out.total_fills, 1);
    assert_eq!(case_out.orders.len(), 1);
    assert_eq!(case_out.orders[0].order_id, "1100000001");
    assert_eq!(case_out.orders[0].client_order_id, "typed-buy");
    assert_eq!(case_out.orders[0].side, "buy");
    assert_eq!(case_out.orders[0].order_type, "market");
    assert_eq!(case_out.orders[0].quantity, "10");
    assert_eq!(case_out.orders[0].status, "FILLED");
    assert_eq!(case_out.orders[0].filled_quantity, "10");
    assert_eq!(case_out.orders[0].filled_price, "101");
    assert_eq!(case_out.orders[0].submitted_at, "2026-06-29T09:30:59.999Z");
    assert_eq!(case_out.orders[0].filled_at, "2026-06-29T09:31:00Z");
    assert!(!case_out.orders[0].reduce_only);

    assert_eq!(case_out.fills.len(), 1);
    assert_eq!(case_out.fills[0].trade_id, "1200000001");
    assert_eq!(case_out.fills[0].order_id, "1100000001");
    assert_eq!(case_out.fills[0].client_order_id, "typed-buy");
    assert_eq!(case_out.fills[0].side, "buy");
    assert_eq!(case_out.fills[0].price, "101");
    assert_eq!(case_out.fills[0].quantity, "10");
    assert_eq!(case_out.fills[0].quote_quantity, "1010");
    assert_eq!(case_out.fills[0].time, "2026-06-29T09:31:00Z");
    assert!(!case_out.fills[0].maker);
    assert_eq!(case_out.fills[0].broker_fee, "0");
    assert_eq!(case_out.fills[0].market_fee, "0");
    assert_eq!(case_out.fills[0].total_fee, "0");
    assert_eq!(case_out.fills[0].realized_pnl, "0");
}

#[test]
fn explicit_cancel_orders_by_generated_order_id() {
    let case = json!({
        "id": "cancel-by-generated-order-id",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "1000",
        "processOrdersOnClose": false,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0),
            test_bar(1, 100.0, 101.0, 99.0, 100.0, 1000.0)
        ],
        "intents": [
            {
                "barIndex": 0,
                "action": "submit",
                "id": "order-1",
                "side": "buy",
                "orderType": "market",
                "quantity": "1"
            },
            {
                "barIndex": 0,
                "action": "submit",
                "id": "order-2",
                "side": "buy",
                "orderType": "market",
                "quantity": "1"
            },
            {
                "barIndex": 0,
                "action": "cancel",
                "targetId": "1100000001",
                "id": "c-by-gen-id"
            },
            {
                "barIndex": 0,
                "action": "cancel",
                "targetId": "9999999999",
                "id": "c-unknown-id"
            },
            {
                "barIndex": 0,
                "action": "cancel",
                "targetId": "-1100000001",
                "id": "c-negative-id"
            }
        ]
    });

    let res = run_single_case(case).expect("run case");
    assert_eq!(res["status"], "completed");
    assert_eq!(res["processedBars"], 2);
    assert_eq!(res["cash"], "900");
    assert_eq!(res["basePosition"], "1");
    assert_eq!(res["finalEquity"], "1000");
    assert_eq!(res["realizedPnl"], "0");
    assert_eq!(res["totalFills"], 1);
    assert_eq!(res["totalTrades"], 0);
    assert!(res["warnings"].as_array().unwrap().is_empty());

    let orders = res["orders"].as_array().expect("orders");
    assert_eq!(orders.len(), 2);
    assert_order(
        &orders[0],
        &ExpectedOrder {
            order_id: "1100000001",
            client_order_id: "order-1",
            side: "buy",
            order_type: "market",
            quantity: "1",
            status: "CANCELED",
            filled_quantity: "0",
            filled_price: "0",
            submitted_at: "2026-06-29T09:30:59.999Z",
            filled_at: "2026-06-29T09:30:59.999Z",
            reduce_only: false,
        },
    );
    assert_order(
        &orders[1],
        &ExpectedOrder {
            order_id: "1100000002",
            client_order_id: "order-2",
            side: "buy",
            order_type: "market",
            quantity: "1",
            status: "FILLED",
            filled_quantity: "1",
            filled_price: "100",
            submitted_at: "2026-06-29T09:30:59.999Z",
            filled_at: "2026-06-29T09:31:00Z",
            reduce_only: false,
        },
    );

    let fills = res["fills"].as_array().expect("fills");
    assert_eq!(fills.len(), 1);
    assert_fill(
        &fills[0],
        &ExpectedFill {
            trade_id: "1200000001",
            order_id: "1100000002",
            client_order_id: "order-2",
            side: "buy",
            price: "100",
            quantity: "1",
            quote_quantity: "100",
            time: "2026-06-29T09:31:00Z",
            maker: false,
            broker_fee: "0",
            market_fee: "0",
            total_fee: "0",
            realized_pnl: "0",
        },
    );
}

#[test]
fn multiple_atomic_groups_preserve_input_order_over_lexicographical_sorting() {
    let case = json!({
        "id": "multi-atomic-input-order",
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": "USD",
        "initialBalance": "10000",
        "processOrdersOnClose": false,
        "slippageTicks": 0,
        "market": default_market(),
        "candles": [
            test_bar(0, 100.0, 101.0, 99.0, 100.0, 1000.0),
            test_bar(1, 100.0, 101.0, 99.0, 100.0, 100.0), // 10% budget = 10 shares
            test_bar(2, 100.0, 101.0, 99.0, 100.0, 100.0)  // 10% budget = 10 shares
        ],
        "intents": [
            // z-bracket appears FIRST in input order
            {
                "barIndex": 0,
                "action": "submit",
                "id": "z-entry",
                "side": "buy",
                "orderType": "market",
                "quantity": "10",
                "atomicGroupId": "z-bracket"
            },
            // This ordinary intent is textually between the z-bracket members. The complete
            // atomic group must still be installed before this order at the group's first sighting.
            {
                "barIndex": 0,
                "action": "submit",
                "id": "ordinary-between-groups",
                "side": "buy",
                "orderType": "limit",
                "limitPrice": "80",
                "quantity": "1"
            },
            {
                "barIndex": 0,
                "action": "submit",
                "id": "z-stop",
                "parentId": "z-entry",
                "side": "sell",
                "orderType": "stop_market",
                "stopPrice": "90",
                "quantity": "10",
                "atomicGroupId": "z-bracket",
                "ocoGroupId": "z-oco",
                "reduceOnly": true
            },
            // a-bracket appears SECOND in input order (lexicographically before z-bracket)
            {
                "barIndex": 0,
                "action": "submit",
                "id": "a-entry",
                "side": "buy",
                "orderType": "market",
                "quantity": "10",
                "atomicGroupId": "a-bracket"
            },
            {
                "barIndex": 0,
                "action": "submit",
                "id": "a-stop",
                "parentId": "a-entry",
                "side": "sell",
                "orderType": "stop_market",
                "stopPrice": "90",
                "quantity": "10",
                "atomicGroupId": "a-bracket",
                "ocoGroupId": "a-oco",
                "reduceOnly": true
            },
            // The Go Stage 3 harness records processed atomic group IDs for the whole case.
            // Reusing z-bracket on a later bar must therefore be ignored rather than installed.
            {
                "barIndex": 1,
                "action": "submit",
                "id": "z-late-entry",
                "side": "buy",
                "orderType": "market",
                "quantity": "1",
                "atomicGroupId": "z-bracket"
            },
            {
                "barIndex": 1,
                "action": "submit",
                "id": "z-late-stop",
                "parentId": "z-late-entry",
                "side": "sell",
                "orderType": "stop_market",
                "stopPrice": "90",
                "quantity": "1",
                "atomicGroupId": "z-bracket",
                "reduceOnly": true
            }
        ]
    });

    let res = run_single_case(case).expect("run case");
    assert_eq!(res["status"], "completed");
    assert_eq!(res["processedBars"], 3);
    assert_eq!(res["cash"], "8000");
    assert_eq!(res["basePosition"], "20");
    assert_eq!(res["finalEquity"], "10000");
    assert_eq!(res["realizedPnl"], "0");
    assert_eq!(res["totalBrokerFees"], "0");
    assert_eq!(res["totalMarketFees"], "0");
    assert_eq!(res["totalFees"], "0");
    assert_eq!(res["totalFills"], 2);
    assert_eq!(res["totalTrades"], 0);
    assert_eq!(res["winningTrades"], 0);
    assert_eq!(res["winRate"], "0");
    assert_eq!(res["maxDrawdown"], "0");
    assert_eq!(res["currentDrawdown"], "0");
    assert!(res["warnings"].as_array().unwrap().is_empty());

    let orders = res["orders"].as_array().expect("orders");
    assert_eq!(orders.len(), 5);
    assert_order(
        &orders[0],
        &ExpectedOrder {
            order_id: "1100000001",
            client_order_id: "z-entry",
            side: "buy",
            order_type: "market",
            quantity: "10",
            status: "FILLED",
            filled_quantity: "10",
            filled_price: "100",
            submitted_at: "2026-06-29T09:30:59.999Z",
            filled_at: "2026-06-29T09:31:00Z",
            reduce_only: false,
        },
    );
    assert_order(
        &orders[1],
        &ExpectedOrder {
            order_id: "1100000002",
            client_order_id: "z-stop",
            side: "sell",
            order_type: "stop_market",
            quantity: "10",
            status: "NEW",
            filled_quantity: "0",
            filled_price: "0",
            submitted_at: "2026-06-29T09:30:59.999Z",
            filled_at: "",
            reduce_only: true,
        },
    );
    assert_order(
        &orders[2],
        &ExpectedOrder {
            order_id: "1100000003",
            client_order_id: "ordinary-between-groups",
            side: "buy",
            order_type: "limit",
            quantity: "1",
            status: "NEW",
            filled_quantity: "0",
            filled_price: "0",
            submitted_at: "2026-06-29T09:30:59.999Z",
            filled_at: "",
            reduce_only: false,
        },
    );
    assert_order(
        &orders[3],
        &ExpectedOrder {
            order_id: "1100000004",
            client_order_id: "a-entry",
            side: "buy",
            order_type: "market",
            quantity: "10",
            status: "FILLED",
            filled_quantity: "10",
            filled_price: "100",
            submitted_at: "2026-06-29T09:30:59.999Z",
            filled_at: "2026-06-29T09:32:00Z",
            reduce_only: false,
        },
    );
    assert_order(
        &orders[4],
        &ExpectedOrder {
            order_id: "1100000005",
            client_order_id: "a-stop",
            side: "sell",
            order_type: "stop_market",
            quantity: "10",
            status: "NEW",
            filled_quantity: "0",
            filled_price: "0",
            submitted_at: "2026-06-29T09:30:59.999Z",
            filled_at: "",
            reduce_only: true,
        },
    );

    let fills = res["fills"].as_array().expect("fills");
    assert_eq!(fills.len(), 2);
    // z-bracket fill occurs on Bar 1 (2026-06-29T09:31:00Z) with trade_id 1200000001
    assert_fill(
        &fills[0],
        &ExpectedFill {
            trade_id: "1200000001",
            order_id: "1100000001",
            client_order_id: "z-entry",
            side: "buy",
            price: "100",
            quantity: "10",
            quote_quantity: "1000",
            time: "2026-06-29T09:31:00Z",
            maker: false,
            broker_fee: "0",
            market_fee: "0",
            total_fee: "0",
            realized_pnl: "0",
        },
    );
    // a-bracket fill occurs on Bar 2 (2026-06-29T09:32:00Z) with trade_id 1200000002
    assert_fill(
        &fills[1],
        &ExpectedFill {
            trade_id: "1200000002",
            order_id: "1100000004",
            client_order_id: "a-entry",
            side: "buy",
            price: "100",
            quantity: "10",
            quote_quantity: "1000",
            time: "2026-06-29T09:32:00Z",
            maker: false,
            broker_fee: "0",
            market_fee: "0",
            total_fee: "0",
            realized_pnl: "0",
        },
    );
}
