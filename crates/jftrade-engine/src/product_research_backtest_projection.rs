//! Authoritative ResultView projection for backtest outcomes.
//!
//! Provides deterministic view filtering, warmup trimming,
//! and mapping from CorpusOutput (`cases[]`, `fills`, `orders`) to client view series.

use std::collections::HashMap;

use serde_json::{Value, json};

#[path = "product_research_backtest_projection_filters.rs"]
mod filters;
pub(crate) use filters::*;

pub(crate) use crate::product::product_research_backtest_execution::execute_research_backtest;
use crate::product::{BacktestResultViewError, BacktestResultViewRequest};

struct NormalizedBacktestData {
    summary: Value,
    candles: Vec<Value>,
    trades: Vec<Value>,
    pnl_curve: Vec<Value>,
    drawdown_curve: Vec<Value>,
    orders: Vec<Value>,
    logs: Vec<Value>,
    warnings: Vec<Value>,
    runtime_errors: Vec<Value>,
}

fn parse_f64_val(v: Option<&Value>) -> f64 {
    v.and_then(|val| {
        val.as_f64()
            .or_else(|| val.as_str().and_then(|s| s.parse::<f64>().ok()))
    })
    .unwrap_or(0.0)
}

struct CaseLengths {
    orders: usize,
    trades: usize,
    candles: usize,
    pnl: usize,
    drawdown: usize,
    warnings: usize,
    logs: usize,
    errors: usize,
}

fn extract_case_summary(c: &Value, lengths: &CaseLengths) -> Value {
    json!({
        "pnl": parse_f64_val(c.get("realizedPnl")),
        "realizedPnl": c.get("realizedPnl").cloned().unwrap_or_else(|| json!(parse_f64_val(c.get("realizedPnl")))),
        "finalBalance": parse_f64_val(c.get("finalEquity")),
        "finalEquity": c.get("finalEquity").cloned().unwrap_or_else(|| json!(parse_f64_val(c.get("finalEquity")))),
        "cash": c.get("cash").cloned().unwrap_or_else(|| json!(parse_f64_val(c.get("cash")))),
        "maxDrawdown": parse_f64_val(c.get("maxDrawdown")),
        "currentDrawdown": parse_f64_val(c.get("currentDrawdown")),
        "totalTrades": c.get("totalTrades").cloned().unwrap_or(json!(0)),
        "tradeCount": c.get("totalTrades").cloned().unwrap_or(json!(0)),
        "winningTrades": c.get("winningTrades").cloned().unwrap_or(json!(0)),
        "winRate": c.get("winRate").cloned().unwrap_or_else(|| json!(parse_f64_val(c.get("winRate")))),
        "totalFees": c.get("totalFees").cloned().unwrap_or_else(|| json!(parse_f64_val(c.get("totalFees")))),
        "totalBrokerFees": parse_f64_val(c.get("totalBrokerFees")),
        "totalMarketFees": parse_f64_val(c.get("totalMarketFees")),
        "processedBars": c.get("processedBars").cloned().unwrap_or(json!(0)),
        "totalFills": c.get("totalFills").cloned().unwrap_or(json!(0)),
        "warnings": c.get("warnings").cloned().unwrap_or(json!([])),
        "orderBookCount": lengths.orders,
        "tradesCount": lengths.trades,
        "candlesCount": lengths.candles,
        "pnlCurveCount": lengths.pnl,
        "drawdownCurveCount": lengths.drawdown,
        "warningCount": lengths.warnings,
        "logsCount": lengths.logs,
        "runtimeErrorCount": lengths.errors,
    })
}

fn annotate_and_trim_warmup(
    formal_start_nanos: Option<i128>,
    candles: &mut Vec<Value>,
    pnl_curve: &mut Vec<Value>,
    drawdown_curve: &mut Vec<Value>,
    trades: &mut [Value],
    orders: &mut [Value],
) {
    if let Some(fs) = formal_start_nanos {
        candles.retain(|c| {
            parse_timestamp_nanos(c.get("start").or_else(|| c.get("time"))).is_none_or(|t| t >= fs)
        });
        pnl_curve.retain(|p| parse_timestamp_nanos(p.get("time")).is_none_or(|t| t >= fs));
        drawdown_curve.retain(|p| parse_timestamp_nanos(p.get("time")).is_none_or(|t| t >= fs));
    }
    for trade in trades {
        if let Some(obj) = trade.as_object_mut() {
            let is_warmup = formal_start_nanos.is_some_and(|fs| {
                parse_timestamp_nanos(obj.get("time")).is_some_and(|t| t < fs)
            });
            obj.insert("warmup".to_owned(), json!(is_warmup));
        }
    }
    for order in orders {
        if let Some(obj) = order.as_object_mut() {
            let is_warmup = formal_start_nanos.is_some_and(|fs| {
                parse_timestamp_nanos(obj.get("submittedAt").or_else(|| obj.get("filledAt")))
                    .is_some_and(|t| t < fs)
            });
            obj.insert("warmup".to_owned(), json!(is_warmup));
        }
    }
}

fn map_fill_to_trade(fill: &Value, symbol: &str, formal_start_nanos: Option<i128>) -> Value {
    let trade_id = fill
        .get("tradeId")
        .or_else(|| fill.get("id"))
        .and_then(Value::as_str)
        .unwrap_or_default();
    let order_id = fill
        .get("orderId")
        .and_then(Value::as_str)
        .unwrap_or_default();
    let time_val = fill.get("time").cloned().unwrap_or(Value::Null);
    let is_warmup = formal_start_nanos
        .is_some_and(|fs| parse_timestamp_nanos(Some(&time_val)).is_some_and(|t| t < fs));
    let fee = parse_f64_val(fill.get("totalFee").or_else(|| fill.get("fee")));
    json!({
        "id": trade_id,
        "tradeId": trade_id,
        "orderId": order_id,
        "clientOrderId": fill.get("clientOrderId").cloned().unwrap_or(Value::Null),
        "symbol": fill.get("symbol").and_then(Value::as_str).unwrap_or(symbol),
        "side": fill.get("side").or_else(|| fill.get("action")).and_then(Value::as_str).unwrap_or_default(),
        "action": fill.get("side").or_else(|| fill.get("action")).and_then(Value::as_str).unwrap_or_default(),
        "price": parse_f64_val(fill.get("price")),
        "quantity": parse_f64_val(fill.get("quantity")),
        "quoteQuantity": parse_f64_val(fill.get("quoteQuantity")),
        "time": time_val,
        "fee": fee,
        "totalFee": fee,
        "realizedPnl": parse_f64_val(fill.get("realizedPnl")),
        "warmup": is_warmup,
    })
}

fn map_order_to_orderbook(
    order: &Value,
    symbol: &str,
    fees_by_order: &HashMap<String, f64>,
    formal_start_nanos: Option<i128>,
) -> Value {
    let order_id = order
        .get("orderId")
        .or_else(|| order.get("id"))
        .and_then(Value::as_str)
        .unwrap_or_default();
    let sub = order.get("submittedAt").or_else(|| order.get("time"));
    let fil = order.get("filledAt");
    let is_warmup = formal_start_nanos.is_some_and(|fs| {
        parse_timestamp_nanos(sub.or(fil)).is_some_and(|t| t < fs)
    });
    let fee = fees_by_order.get(order_id).copied().unwrap_or(0.0);
    let order_type = order
        .get("orderType")
        .or_else(|| order.get("type"))
        .and_then(Value::as_str)
        .unwrap_or("MARKET");
    json!({
        "id": order_id,
        "orderId": order_id,
        "clientOrderId": order.get("clientOrderId").cloned().unwrap_or(Value::Null),
        "symbol": order.get("symbol").and_then(Value::as_str).unwrap_or(symbol),
        "side": order.get("side").and_then(Value::as_str).unwrap_or_default(),
        "orderType": order_type,
        "type": order_type,
        "quantity": parse_f64_val(order.get("quantity")),
        "status": order.get("status").and_then(Value::as_str).unwrap_or("FILLED"),
        "filledQuantity": parse_f64_val(order.get("filledQuantity")),
        "filledPrice": parse_f64_val(order.get("filledPrice")),
        "submittedAt": sub.cloned().unwrap_or(Value::Null),
        "filledAt": fil.cloned().unwrap_or(Value::Null),
        "time": sub.cloned().unwrap_or(Value::Null),
        "totalFees": fee,
        "fee": fee,
        "warmup": is_warmup,
    })
}

fn map_curve_point(point: &Value, val_key: &str) -> Value {
    let mut obj = point.as_object().cloned().unwrap_or_default();
    if let Some(v) = obj.get(val_key) {
        let num = parse_f64_val(Some(v));
        obj.insert(val_key.to_owned(), json!(num));
    }
    Value::Object(obj)
}

fn extract_corpus_case_backtest(
    result_val: &Value,
    c: &Value,
    seed_candles: Option<&[Value]>,
    formal_start_nanos: Option<i128>,
) -> NormalizedBacktestData {
    let symbol = result_val
        .get("request")
        .and_then(|r| r.get("symbol"))
        .and_then(Value::as_str)
        .unwrap_or_default();
    let raw_orders = c.get("orders").and_then(Value::as_array).cloned().unwrap_or_default();
    let raw_fills = c.get("fills").and_then(Value::as_array).cloned().unwrap_or_default();

    let mut fees_by_order: HashMap<String, f64> = HashMap::new();
    for fill in &raw_fills {
        if let Some(order_id) = fill.get("orderId").and_then(Value::as_str) {
            let fee = parse_f64_val(fill.get("totalFee").or_else(|| fill.get("fee")));
            *fees_by_order.entry(order_id.to_owned()).or_default() += fee;
        }
    }

    let mut trades: Vec<Value> = raw_fills
        .into_iter()
        .map(|fill| map_fill_to_trade(&fill, symbol, formal_start_nanos))
        .collect();
    let mut orders: Vec<Value> = raw_orders
        .into_iter()
        .map(|ord| map_order_to_orderbook(&ord, symbol, &fees_by_order, formal_start_nanos))
        .collect();
    let mut pnl_curve: Vec<Value> = c
        .get("equityCurve")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default()
        .into_iter()
        .map(|p| map_curve_point(&p, "equity"))
        .collect();
    let mut drawdown_curve: Vec<Value> = c
        .get("drawdownCurve")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default()
        .into_iter()
        .map(|p| map_curve_point(&p, "drawdown"))
        .collect();
    let warnings = c.get("warnings").and_then(Value::as_array).cloned().unwrap_or_default();

    let mut candles = if let Some(seed) = seed_candles {
        seed.to_vec()
    } else {
        result_val
            .get("result")
            .and_then(|r| r.get("candles"))
            .or_else(|| result_val.get("candles"))
            .or_else(|| result_val.get("request").and_then(|r| r.get("candles")))
            .and_then(Value::as_array)
            .cloned()
            .unwrap_or_default()
    };

    let logs = result_val
        .get("logs")
        .or_else(|| result_val.get("result").and_then(|r| r.get("logs")))
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();
    let runtime_errors = result_val
        .get("runtimeErrors")
        .or_else(|| result_val.get("errors"))
        .or_else(|| result_val.get("result").and_then(|r| r.get("runtimeErrors")))
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();

    annotate_and_trim_warmup(
        formal_start_nanos,
        &mut candles,
        &mut pnl_curve,
        &mut drawdown_curve,
        &mut trades,
        &mut orders,
    );

    let candles_count = if !candles.is_empty() {
        candles.len()
    } else {
        c.get("processedBars").and_then(Value::as_u64).unwrap_or(0) as usize
    };

    let lengths = CaseLengths {
        orders: orders.len(),
        trades: trades.len(),
        candles: candles_count,
        pnl: pnl_curve.len(),
        drawdown: drawdown_curve.len(),
        warnings: warnings.len(),
        logs: logs.len(),
        errors: runtime_errors.len(),
    };
    let mut summary = extract_case_summary(c, &lengths);
    if let Some(existing) = result_val.get("result").and_then(|r| r.get("summary")).and_then(Value::as_object)
        && let Some(s_obj) = summary.as_object_mut()
    {
        for (k, v) in existing {
            s_obj.entry(k.clone()).or_insert_with(|| v.clone());
        }
    }

    NormalizedBacktestData {
        summary,
        candles,
        trades,
        pnl_curve,
        drawdown_curve,
        orders,
        logs,
        warnings,
        runtime_errors,
    }
}

fn extract_legacy_backtest(
    result_val: &Value,
    result_node: &serde_json::Map<String, Value>,
    seed_candles: Option<&[Value]>,
    formal_start_nanos: Option<i128>,
) -> NormalizedBacktestData {
    let empty_vec = Vec::new();
    let mut orders = result_node
        .get("orderBook")
        .or_else(|| result_node.get("orders"))
        .or_else(|| result_val.get("orders"))
        .and_then(Value::as_array)
        .unwrap_or(&empty_vec)
        .clone();
    let mut trades = result_node
        .get("trades")
        .or_else(|| result_val.get("trades"))
        .and_then(Value::as_array)
        .unwrap_or(&empty_vec)
        .clone();
    let mut candles = if let Some(seed) = seed_candles {
        seed.to_vec()
    } else {
        result_node
            .get("candles")
            .or_else(|| result_val.get("candles"))
            .and_then(Value::as_array)
            .unwrap_or(&empty_vec)
            .clone()
    };
    let mut pnl_curve = result_node
        .get("pnlCurve")
        .or_else(|| result_node.get("equityCurve"))
        .or_else(|| result_node.get("equity"))
        .or_else(|| result_val.get("pnlCurve"))
        .or_else(|| result_val.get("equityCurve"))
        .or_else(|| result_val.get("equity"))
        .and_then(Value::as_array)
        .unwrap_or(&empty_vec)
        .clone();
    let mut drawdown_curve = result_node
        .get("drawdownCurve")
        .and_then(Value::as_array)
        .unwrap_or(&empty_vec)
        .clone();
    let logs = result_node
        .get("logs")
        .or_else(|| result_val.get("logs"))
        .and_then(Value::as_array)
        .unwrap_or(&empty_vec)
        .clone();
    let warnings = result_node
        .get("warnings")
        .and_then(Value::as_array)
        .unwrap_or(&empty_vec)
        .clone();
    let runtime_errors = result_node
        .get("runtimeErrors")
        .or_else(|| result_node.get("errors"))
        .and_then(Value::as_array)
        .unwrap_or(&empty_vec)
        .clone();
    let summary = result_node
        .get("summary")
        .cloned()
        .unwrap_or_else(|| json!({}));

    annotate_and_trim_warmup(
        formal_start_nanos,
        &mut candles,
        &mut pnl_curve,
        &mut drawdown_curve,
        &mut trades,
        &mut orders,
    );

    NormalizedBacktestData {
        summary,
        candles,
        trades,
        pnl_curve,
        drawdown_curve,
        orders,
        logs,
        warnings,
        runtime_errors,
    }
}

fn extract_normalized_backtest(
    result_val: &Value,
    seed_candles: Option<&[Value]>,
) -> NormalizedBacktestData {
    let formal_start_nanos = result_val
        .get("request")
        .and_then(|r| r.get("startTime").or_else(|| r.get("startDate")))
        .and_then(|v| parse_timestamp_nanos(Some(v)));

    let empty_obj = serde_json::Map::new();
    let result_node = result_val
        .get("result")
        .and_then(Value::as_object)
        .or_else(|| result_val.as_object())
        .unwrap_or(&empty_obj);

    if let Some(cases) = result_node
        .get("cases")
        .and_then(Value::as_array)
        .filter(|c| !c.is_empty())
    {
        extract_corpus_case_backtest(result_val, &cases[0], seed_candles, formal_start_nanos)
    } else {
        extract_legacy_backtest(result_val, result_node, seed_candles, formal_start_nanos)
    }
}

fn project_chart_series(
    data: &NormalizedBacktestData,
    include_set: &[&str],
    args: &ChartWindowArgs<'_>,
    series: &mut serde_json::Map<String, Value>,
    returned: &mut serde_json::Map<String, Value>,
    window: &mut serde_json::Map<String, Value>,
) {
    let chart_keys: [(&str, &str, &[Value]); 4] = [
        ("candles", "time", &data.candles),
        ("trades", "time", &data.trades),
        ("pnlCurve", "time", &data.pnl_curve),
        ("drawdownCurve", "time", &data.drawdown_curve),
    ];
    for (key, time_field, items) in chart_keys {
        if !include_set.contains(&key) {
            continue;
        }
        let filtered = filter_timed_items(items, time_field, args.start_time, args.end_time);
        let (sliced, next) = slice_items(&filtered, args.offset, args.limit);
        returned.insert(key.to_owned(), json!(sliced.len()));
        series.insert(key.to_owned(), Value::Array(sliced));
        if let Some(next_cursor) = next {
            window.insert("truncated".to_owned(), json!(true));
            if window
                .get("nextCursor")
                .and_then(Value::as_str)
                .unwrap_or("")
                .is_empty()
            {
                window.insert("nextCursor".to_owned(), Value::String(next_cursor));
            }
        }
    }
}

fn project_orders_view(
    orders: &[Value],
    args: &ChartWindowArgs<'_>,
    series: &mut serde_json::Map<String, Value>,
    returned: &mut serde_json::Map<String, Value>,
    window: &mut serde_json::Map<String, Value>,
) {
    let filtered: Vec<Value> = orders
        .iter()
        .filter(|item| {
            let sub = item.get("submittedAt");
            let fil = item.get("filledAt");
            item_in_time_window(sub, args.start_time, args.end_time)
                || item_in_time_window(fil, args.start_time, args.end_time)
        })
        .cloned()
        .collect();
    let (sliced, next) = slice_items(&filtered, args.offset, args.limit);
    returned.insert("orderBook".to_owned(), json!(sliced.len()));
    series.insert("orderBook".to_owned(), Value::Array(sliced));
    if let Some(next_cursor) = next {
        window.insert("truncated".to_owned(), json!(true));
        window.insert("nextCursor".to_owned(), Value::String(next_cursor));
    }
}

fn project_text_series_view(
    items: &[Value],
    key: &str,
    args: &ChartWindowArgs<'_>,
    series: &mut serde_json::Map<String, Value>,
    returned: &mut serde_json::Map<String, Value>,
    window: &mut serde_json::Map<String, Value>,
) {
    let filtered: Vec<Value> =
        if key == "logs" && (args.start_time.is_some() || args.end_time.is_some()) {
            items
                .iter()
                .filter(|item| {
                    let t = item
                        .get("timestamp")
                        .or_else(|| item.get("time"))
                        .or_else(|| item.get("at"));
                    item_in_time_window(t, args.start_time, args.end_time)
                })
                .cloned()
                .collect()
        } else {
            items.to_vec()
        };
    let (sliced, next) = slice_items(&filtered, args.offset, args.limit);
    returned.insert(key.to_owned(), json!(sliced.len()));
    series.insert(key.to_owned(), Value::Array(sliced));
    if let Some(next_cursor) = next {
        window.insert("truncated".to_owned(), json!(true));
        window.insert("nextCursor".to_owned(), Value::String(next_cursor));
    }
}

fn dispatch_view_projection(
    view: &str,
    data: &NormalizedBacktestData,
    options: Option<&Value>,
    args: &ChartWindowArgs<'_>,
    series: &mut serde_json::Map<String, Value>,
    returned: &mut serde_json::Map<String, Value>,
    window: &mut serde_json::Map<String, Value>,
) {
    match view {
        "chart" => {
            let default_includes = ["candles", "trades", "pnlCurve", "drawdownCurve"];
            let requested_includes = options
                .and_then(|o| o.get("include"))
                .and_then(Value::as_array)
                .map(|arr| arr.iter().filter_map(Value::as_str).collect::<Vec<&str>>());
            let include_refs = requested_includes
                .as_deref()
                .filter(|v| !v.is_empty())
                .unwrap_or(&default_includes);
            project_chart_series(data, include_refs, args, series, returned, window);
        }
        "orders" => project_orders_view(&data.orders, args, series, returned, window),
        "logs" => project_text_series_view(&data.logs, "logs", args, series, returned, window),
        "warnings" => {
            project_text_series_view(&data.warnings, "warnings", args, series, returned, window)
        }
        "errors" => project_text_series_view(
            &data.runtime_errors,
            "runtimeErrors",
            args,
            series,
            returned,
            window,
        ),
        _ => {}
    }
}

pub(crate) fn project_authoritative_result_view(
    run_val: &Value,
    seed_candles: Option<&[Value]>,
    request: &BacktestResultViewRequest,
) -> Result<Value, BacktestResultViewError> {
    let params = validate_result_view_request(request)?;
    let mut data = extract_normalized_backtest(run_val, seed_candles);

    if params.view == "chart" {
        let default_includes = ["candles", "trades", "pnlCurve", "drawdownCurve"];
        let requested_includes: Vec<&str> = request
            .include
            .as_ref()
            .map(|arr| arr.iter().map(String::as_str).collect())
            .filter(|arr: &Vec<&str>| !arr.is_empty())
            .unwrap_or_else(|| default_includes.to_vec());

        if let Some(res_ms) = params.resolution_ms {
            if requested_includes.contains(&"candles") {
                data.candles = downsample_candles(&data.candles, res_ms);
            }
            if requested_includes.contains(&"pnlCurve") {
                data.pnl_curve = downsample_curve(&data.pnl_curve, "equity", res_ms);
            }
            if requested_includes.contains(&"drawdownCurve") {
                data.drawdown_curve = downsample_curve(&data.drawdown_curve, "drawdown", res_ms);
            }
        }
    }

    let mut window = serde_json::Map::new();
    window.insert("startTime".to_owned(), json!(request.start_time));
    window.insert("endTime".to_owned(), json!(request.end_time));
    window.insert("limit".to_owned(), json!(params.limit));
    window.insert("offset".to_owned(), json!(params.offset));
    window.insert("truncated".to_owned(), json!(false));
    window.insert("nextCursor".to_owned(), json!(""));
    let mut returned = serde_json::Map::new();
    let mut series = serde_json::Map::new();

    let args = ChartWindowArgs {
        start_time: request.start_time.as_deref(),
        end_time: request.end_time.as_deref(),
        offset: params.offset,
        limit: params.limit,
    };
    let include_opt = request.include.as_ref().map(|inc| json!({"include": inc}));

    dispatch_view_projection(
        &params.view,
        &data,
        include_opt.as_ref(),
        &args,
        &mut series,
        &mut returned,
        &mut window,
    );
    window.insert("returned".to_owned(), Value::Object(returned));

    let run_payload = json!({
        "id": run_val.get("id").unwrap_or(&Value::Null),
        "status": run_val.get("status").unwrap_or(&Value::Null),
        "request": run_val.get("request").unwrap_or(&Value::Null),
        "marketDataProvider": run_val.get("marketDataProvider").unwrap_or(&Value::Null),
    });

    Ok(json!({
        "view": params.view,
        "run": run_payload,
        "summary": data.summary,
        "window": window,
        "series": series,
    }))
}

#[allow(dead_code)]
pub(crate) fn project_result_view(result_val: &Value, options: Option<&Value>) -> Value {
    let req = BacktestResultViewRequest {
        run_id: result_val
            .get("id")
            .and_then(Value::as_str)
            .unwrap_or_default()
            .to_owned(),
        view: options
            .and_then(|o| o.get("view"))
            .and_then(Value::as_str)
            .map(str::to_owned),
        include: options
            .and_then(|o| o.get("include"))
            .and_then(Value::as_array)
            .map(|arr| arr.iter().filter_map(Value::as_str).map(str::to_owned).collect()),
        start_time: options
            .and_then(|o| o.get("startTime"))
            .and_then(Value::as_str)
            .map(str::to_owned),
        end_time: options
            .and_then(|o| o.get("endTime"))
            .and_then(Value::as_str)
            .map(str::to_owned),
        cursor: options
            .and_then(|o| o.get("cursor"))
            .and_then(Value::as_str)
            .map(str::to_owned),
        limit: options
            .and_then(|o| o.get("limit"))
            .and_then(Value::as_u64)
            .map(|v| v as usize),
        resolution: options
            .and_then(|o| o.get("resolution"))
            .and_then(Value::as_str)
            .map(str::to_owned),
    };
    project_authoritative_result_view(result_val, None, &req)
        .unwrap_or_else(|err| json!({"error": err.to_string()}))
}
