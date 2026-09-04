//! Research backtest projection for ADK tools.
//!
//! Executes temporary strategy backtests using `BacktestsWritePort::Start`,
//! polling `BacktestReadSnapshotPort` when `waitForCompletionMs > 0`, and
//! generating the result view projection with full slice and window filtering.

use std::time::Duration;

use serde_json::{Value, json};
use sha2::{Digest, Sha256};

use super::product_backtests_write_port::{BacktestsWriteInput, BacktestsWritePortResult};
use crate::product::product_production_ports::ProductionPortBundle;

fn research_script_hash(script: &str) -> String {
    let mut hasher = Sha256::new();
    hasher.update(script.trim().as_bytes());
    let result = hasher.finalize();
    result[..8]
        .iter()
        .map(|b| format!("{b:02x}"))
        .collect::<String>()
}

fn validate_research_script(
    script: &str,
) -> Result<jftrade_strategy::pinespec::ValidationPayload, String> {
    let validation = jftrade_strategy::pinespec::validate_script(script, true, false);
    if !validation.ok {
        return Err(format!(
            "strategy script validation failed: {}",
            validation.errors.join("; ")
        ));
    }
    Ok(validation)
}

fn prepare_start_payload(arguments: &Value, script: &str) -> Value {
    let mut start_payload = arguments.clone();
    if let Some(obj) = start_payload.as_object_mut() {
        obj.insert(
            "strategyScript".to_owned(),
            Value::String(script.to_owned()),
        );
        if !obj.contains_key("market") {
            obj.insert("market".to_owned(), Value::String("HK".to_owned()));
        }
    }
    start_payload
}

fn start_research_backtest(
    ports: &ProductionPortBundle,
    payload: Value,
) -> Result<(String, String), String> {
    let start_result = ports
        .backtests_write
        .mutate(&BacktestsWriteInput::Start { payload })
        .map_err(|e| format!("failed to start research backtest: {e:?}"))?;

    match start_result {
        BacktestsWritePortResult::Data(data) => {
            let id = data
                .get("runId")
                .or_else(|| data.get("id"))
                .and_then(Value::as_str)
                .ok_or_else(|| "backtest start response missing runId".to_owned())?
                .to_owned();
            let status = data
                .get("status")
                .and_then(Value::as_str)
                .unwrap_or("queued")
                .to_owned();
            Ok((id, status))
        }
        other => Err(format!("unexpected backtest start result: {other:?}")),
    }
}

fn poll_backtest_completion(
    ports: &ProductionPortBundle,
    run_id: &str,
    wait_ms: u64,
    initial_status: String,
) -> String {
    let mut current_status = initial_status;
    if wait_ms == 0
        || matches!(
            current_status.as_str(),
            "completed" | "failed" | "cancelled"
        )
    {
        return current_status;
    }
    let start_instant = std::time::Instant::now();
    let timeout = Duration::from_millis(wait_ms);
    let poll_interval = Duration::from_millis(50);

    while start_instant.elapsed() < timeout {
        std::thread::sleep(poll_interval);
        if let Ok(Some(status_val)) = ports.backtest_read.status(run_id)
            && let Some(st) = status_val.get("status").and_then(Value::as_str)
        {
            current_status = st.to_owned();
            if matches!(
                current_status.as_str(),
                "completed" | "failed" | "cancelled"
            ) {
                break;
            }
        }
    }
    current_status
}

fn slice_items(items: &[Value], offset: usize, limit: usize) -> (Vec<Value>, Option<String>) {
    if offset >= items.len() {
        return (Vec::new(), None);
    }
    let end = (offset + limit).min(items.len());
    let next = if end < items.len() {
        Some(end.to_string())
    } else {
        None
    };
    (items[offset..end].to_vec(), next)
}

fn parse_timestamp_nanos(val: Option<&Value>) -> Option<i128> {
    match val {
        Some(Value::Number(n)) => {
            let ms = n.as_i64()?;
            Some((ms as i128) * 1_000_000)
        }
        Some(Value::String(s)) => parse_rfc3339_nanos(s),
        _ => None,
    }
}

fn parse_rfc3339_nanos(s: &str) -> Option<i128> {
    if let Ok(dt) = time::OffsetDateTime::parse(s, &time::format_description::well_known::Rfc3339) {
        Some(dt.unix_timestamp_nanos())
    } else if let Ok(ms) = s.parse::<i64>() {
        Some((ms as i128) * 1_000_000)
    } else {
        None
    }
}

fn item_in_time_window(
    time_val: Option<&Value>,
    start_time: Option<&str>,
    end_time: Option<&str>,
) -> bool {
    if start_time.is_none() && end_time.is_none() {
        return true;
    }
    let Some(t_nanos) = parse_timestamp_nanos(time_val) else {
        return true;
    };
    if let Some(start) = start_time
        && let Some(start_nanos) = parse_rfc3339_nanos(start)
        && t_nanos < start_nanos
    {
        return false;
    }
    if let Some(end) = end_time
        && let Some(end_nanos) = parse_rfc3339_nanos(end)
        && t_nanos > end_nanos
    {
        return false;
    }
    true
}

fn filter_timed_items(
    items: &[Value],
    time_field: &str,
    start_time: Option<&str>,
    end_time: Option<&str>,
) -> Vec<Value> {
    if start_time.is_none() && end_time.is_none() {
        return items.to_vec();
    }
    items
        .iter()
        .filter(|item| {
            let t = item.get(time_field);
            item_in_time_window(t, start_time, end_time)
        })
        .cloned()
        .collect()
}

struct ChartWindowArgs<'a> {
    start_time: Option<&'a str>,
    end_time: Option<&'a str>,
    offset: usize,
    limit: usize,
}

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

fn extract_normalized_backtest(
    result_val: &Value,
    result_node: &serde_json::Map<String, Value>,
) -> NormalizedBacktestData {
    if let Some(cases) = result_node
        .get("cases")
        .and_then(Value::as_array)
        .filter(|c| !c.is_empty())
    {
        let c = &cases[0];
        let orders = c
            .get("orders")
            .and_then(Value::as_array)
            .cloned()
            .unwrap_or_default();
        let trades = c
            .get("fills")
            .and_then(Value::as_array)
            .cloned()
            .unwrap_or_default();
        let pnl_curve = c
            .get("equityCurve")
            .and_then(Value::as_array)
            .cloned()
            .unwrap_or_default();
        let drawdown_curve = c
            .get("drawdownCurve")
            .and_then(Value::as_array)
            .cloned()
            .unwrap_or_default();
        let warnings = c
            .get("warnings")
            .and_then(Value::as_array)
            .cloned()
            .unwrap_or_default();
        let candles = result_node
            .get("candles")
            .or_else(|| result_val.get("request").and_then(|r| r.get("candles")))
            .and_then(Value::as_array)
            .cloned()
            .unwrap_or_default();
        let logs = result_node
            .get("logs")
            .and_then(Value::as_array)
            .cloned()
            .unwrap_or_default();
        let runtime_errors = result_node
            .get("runtimeErrors")
            .or_else(|| result_node.get("errors"))
            .and_then(Value::as_array)
            .cloned()
            .unwrap_or_default();

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

        if let Some(existing) = result_node.get("summary").and_then(Value::as_object)
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
    } else {
        let empty_vec = Vec::new();
        let orders = result_node
            .get("orderBook")
            .or_else(|| result_node.get("orders"))
            .and_then(Value::as_array)
            .unwrap_or(&empty_vec)
            .clone();
        let trades = result_node
            .get("trades")
            .and_then(Value::as_array)
            .unwrap_or(&empty_vec)
            .clone();
        let candles = result_node
            .get("candles")
            .and_then(Value::as_array)
            .unwrap_or(&empty_vec)
            .clone();
        let pnl_curve = result_node
            .get("pnlCurve")
            .and_then(Value::as_array)
            .unwrap_or(&empty_vec)
            .clone();
        let drawdown_curve = result_node
            .get("drawdownCurve")
            .and_then(Value::as_array)
            .unwrap_or(&empty_vec)
            .clone();
        let logs = result_node
            .get("logs")
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

pub(crate) fn project_result_view(result_val: &Value, options: Option<&Value>) -> Value {
    let view = options
        .and_then(|o| o.get("view"))
        .and_then(Value::as_str)
        .unwrap_or("summary");
    let limit = options
        .and_then(|o| o.get("limit"))
        .and_then(Value::as_u64)
        .unwrap_or(500)
        .clamp(1, 2000) as usize;
    let offset = options
        .and_then(|o| o.get("cursor"))
        .and_then(Value::as_str)
        .and_then(|c| c.parse::<usize>().ok())
        .unwrap_or(0);
    let start_time = options
        .and_then(|o| o.get("startTime"))
        .and_then(Value::as_str);
    let end_time = options
        .and_then(|o| o.get("endTime"))
        .and_then(Value::as_str);

    let empty_obj = serde_json::Map::new();
    let result_node = result_val
        .get("result")
        .and_then(Value::as_object)
        .or_else(|| result_val.as_object())
        .unwrap_or(&empty_obj);
    let data = extract_normalized_backtest(result_val, result_node);

    let mut window = serde_json::Map::new();
    window.insert("startTime".to_owned(), json!(start_time));
    window.insert("endTime".to_owned(), json!(end_time));
    window.insert("limit".to_owned(), json!(limit));
    window.insert("offset".to_owned(), json!(offset));
    window.insert("truncated".to_owned(), json!(false));
    window.insert("nextCursor".to_owned(), json!(""));
    let mut returned = serde_json::Map::new();
    let mut series = serde_json::Map::new();

    let args = ChartWindowArgs {
        start_time,
        end_time,
        offset,
        limit,
    };
    dispatch_view_projection(
        view,
        &data,
        options,
        &args,
        &mut series,
        &mut returned,
        &mut window,
    );
    window.insert("returned".to_owned(), Value::Object(returned));

    let run_payload = json!({
        "id": result_val.get("id").unwrap_or(&Value::Null),
        "status": result_val.get("status").unwrap_or(&Value::Null),
        "request": result_val.get("request").unwrap_or(&Value::Null),
        "marketDataProvider": result_val.get("marketDataProvider").unwrap_or(&Value::Null),
    });

    json!({
        "view": view,
        "run": run_payload,
        "summary": data.summary,
        "window": window,
        "series": series,
    })
}

pub(crate) fn extract_run_metadata(
    result_val: Option<&Value>,
    arguments: &Value,
) -> (String, String, String, bool, String, Value) {
    let request = result_val.and_then(|r| r.get("request"));
    let provider = result_val
        .and_then(|r| r.get("marketDataProvider"))
        .or_else(|| request.and_then(|req| req.get("marketDataProvider")))
        .or_else(|| arguments.get("marketDataProvider"))
        .and_then(Value::as_str)
        .unwrap_or("futu")
        .to_owned();
    let chart_type = result_val
        .and_then(|r| r.get("chartType"))
        .or_else(|| request.and_then(|req| req.get("chartType")))
        .or_else(|| arguments.get("chartType"))
        .and_then(Value::as_str)
        .unwrap_or("standard")
        .to_owned();
    let instrument_type = result_val
        .and_then(|r| r.get("instrumentType"))
        .or_else(|| request.and_then(|req| req.get("instrumentType")))
        .or_else(|| arguments.get("instrumentType"))
        .and_then(Value::as_str)
        .unwrap_or("stock")
        .to_owned();
    let use_extended_hours = result_val
        .and_then(|r| r.get("useExtendedHours"))
        .or_else(|| request.and_then(|req| req.get("useExtendedHours")))
        .or_else(|| arguments.get("useExtendedHours"))
        .and_then(Value::as_bool)
        .unwrap_or(false);
    let execution_model = result_val
        .and_then(|r| r.get("executionModel"))
        .or_else(|| request.and_then(|req| req.get("executionModel")))
        .or_else(|| arguments.get("executionModel"))
        .and_then(Value::as_str)
        .unwrap_or("conservative-bar-v1")
        .to_owned();
    let trading_costs = result_val
        .and_then(|r| r.get("tradingCosts"))
        .or_else(|| request.and_then(|req| req.get("tradingCosts")))
        .or_else(|| arguments.get("tradingCosts"))
        .cloned()
        .unwrap_or(Value::Null);

    (
        provider,
        chart_type,
        instrument_type,
        use_extended_hours,
        execution_model,
        trading_costs,
    )
}

pub(crate) fn execute_research_backtest(
    ports: &ProductionPortBundle,
    arguments: &Value,
) -> Result<Value, String> {
    let script = arguments
        .get("script")
        .or_else(|| arguments.get("strategyScript"))
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .ok_or_else(|| "script is required".to_owned())?;

    let validation = validate_research_script(script)?;
    let start_payload = prepare_start_payload(arguments, script);
    let (run_id, initial_status) = start_research_backtest(ports, start_payload)?;

    let wait_ms = arguments
        .get("waitForCompletionMs")
        .and_then(Value::as_i64)
        .unwrap_or(0)
        .clamp(0, 25_000) as u64;

    let mut current_status = poll_backtest_completion(ports, &run_id, wait_ms, initial_status);

    let result_snapshot = ports.backtest_read.result(&run_id).ok().flatten();
    if let Some(ref res) = result_snapshot
        && let Some(st) = res.get("status").and_then(Value::as_str)
    {
        current_status = st.to_owned();
    }

    let (provider, chart_type, inst_type, extended, exec_model, fees) =
        extract_run_metadata(result_snapshot.as_ref(), arguments);

    let mut response = json!({
        "ok": true,
        "status": current_status,
        "runId": run_id,
        "marketDataProvider": provider,
        "chartType": chart_type,
        "instrumentType": inst_type,
        "useExtendedHours": extended,
        "executionModel": exec_model,
        "tradingCosts": fees,
        "scriptHash": research_script_hash(script),
        "validation": {
            "metadata": validation.metadata,
            "hooks": validation.hooks,
            "warnings": validation.warnings,
        },
        "saveRecommendation": "仅当用户明确要求保存/发布/更新策略定义时，再调用 strategy.save_definition。",
    });

    if let Some(ref res) = result_snapshot {
        let view_opts = arguments.get("resultView");
        response["resultView"] = project_result_view(res, view_opts);
    } else {
        response["resultViewError"] = Value::String("result view snapshot unavailable".to_owned());
    }

    Ok(response)
}
