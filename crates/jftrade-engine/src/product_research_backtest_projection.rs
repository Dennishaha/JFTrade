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

pub(crate) struct NormalizedBacktestData {
    pub(crate) summary: Value,
    pub(crate) candles: Vec<Value>,
    pub(crate) trades: Vec<Value>,
    pub(crate) pnl_curve: Vec<Value>,
    pub(crate) drawdown_curve: Vec<Value>,
    pub(crate) orders: Vec<Value>,
    pub(crate) logs: Vec<Value>,
    pub(crate) warnings: Vec<Value>,
    pub(crate) runtime_errors: Vec<Value>,
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
            let is_warmup = formal_start_nanos
                .is_some_and(|fs| parse_timestamp_nanos(obj.get("time")).is_some_and(|t| t < fs));
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

#[derive(Clone, Copy, Default)]
struct OrderFeeTotals {
    broker_fee: f64,
    market_fee: f64,
    total_fee: f64,
}

fn format_decimal_str(val: Option<&Value>) -> String {
    match val {
        Some(Value::String(s)) => s.clone(),
        Some(Value::Number(n)) => n.to_string(),
        _ => String::new(),
    }
}

fn map_fill_to_trade(
    fill: &Value,
    symbol: &str,
    formal_start_nanos: Option<i128>,
    quote_currency: &str,
) -> Value {
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
    let time_str = time_val.as_str().unwrap_or_default().to_owned();

    let price_str = format_decimal_str(fill.get("price"));
    let qty_str = format_decimal_str(fill.get("qty").or_else(|| fill.get("quantity")));
    let side = fill
        .get("side")
        .or_else(|| fill.get("action"))
        .and_then(Value::as_str)
        .unwrap_or_default();
    let pnl = parse_f64_val(fill.get("pnl").or_else(|| fill.get("realizedPnl")));
    let broker_fee = parse_f64_val(fill.get("brokerFee"));
    let market_fee = parse_f64_val(fill.get("marketFee"));
    let mut total_fee = parse_f64_val(fill.get("totalFee").or_else(|| fill.get("fee")));
    if total_fee == 0.0 && (broker_fee > 0.0 || market_fee > 0.0) {
        total_fee = broker_fee + market_fee;
    }
    let fee_currency = fill
        .get("feeCurrency")
        .and_then(Value::as_str)
        .unwrap_or(quote_currency);

    json!({
        "time": time_str,
        "side": side,
        "price": price_str,
        "qty": qty_str,
        "warmup": is_warmup,
        "pnl": pnl,
        "brokerFee": broker_fee,
        "marketFee": market_fee,
        "totalFee": total_fee,
        "feeCurrency": fee_currency,
        "id": trade_id,
        "tradeId": trade_id,
        "orderId": order_id,
        "clientOrderId": fill.get("clientOrderId").cloned().unwrap_or(Value::Null),
        "symbol": fill.get("symbol").and_then(Value::as_str).unwrap_or(symbol),
        "quantity": qty_str,
        "realizedPnl": pnl,
    })
}

fn map_order_to_orderbook(
    order: &Value,
    symbol: &str,
    fees_by_order: &HashMap<String, OrderFeeTotals>,
    formal_start_nanos: Option<i128>,
    quote_currency: &str,
) -> Value {
    let order_id = order
        .get("orderId")
        .or_else(|| order.get("id"))
        .and_then(Value::as_str)
        .unwrap_or_default();
    let sub = order.get("submittedAt").or_else(|| order.get("time"));
    let fil = order.get("filledAt");
    let is_warmup = formal_start_nanos
        .is_some_and(|fs| parse_timestamp_nanos(sub.or(fil)).is_some_and(|t| t < fs));
    let fees = fees_by_order.get(order_id).copied().unwrap_or_default();
    let order_type = order
        .get("orderType")
        .or_else(|| order.get("type"))
        .and_then(Value::as_str)
        .unwrap_or("MARKET");
    let status = order
        .get("status")
        .and_then(Value::as_str)
        .unwrap_or("FILLED");
    let side = order
        .get("side")
        .and_then(Value::as_str)
        .unwrap_or_default();
    let quantity_str = format_decimal_str(order.get("quantity"));
    let order_price_str =
        format_decimal_str(order.get("orderPrice").or_else(|| order.get("price")));
    let filled_price_str =
        format_decimal_str(order.get("filledPrice").or_else(|| order.get("price")));
    let filled_quantity_str = format_decimal_str(
        order
            .get("filledQuantity")
            .or_else(|| order.get("quantity")),
    );
    let sub_str = sub.and_then(Value::as_str).unwrap_or_default();
    let fil_str = fil.and_then(Value::as_str).unwrap_or_default();
    let fee_currency = order
        .get("feeCurrency")
        .and_then(Value::as_str)
        .unwrap_or(quote_currency);

    json!({
        "orderId": order_id,
        "clientOrderId": order.get("clientOrderId").cloned().unwrap_or(Value::Null),
        "symbol": order.get("symbol").and_then(Value::as_str).unwrap_or(symbol),
        "side": side,
        "quantity": quantity_str,
        "orderType": order_type,
        "orderPrice": order_price_str,
        "submittedAt": sub_str,
        "status": status,
        "filledQuantity": filled_quantity_str,
        "filledPrice": filled_price_str,
        "filledAt": fil_str,
        "warmup": is_warmup,
        "brokerFee": fees.broker_fee,
        "marketFee": fees.market_fee,
        "totalFee": fees.total_fee,
        "feeCurrency": fee_currency,
        "id": order_id,
        "type": order_type,
        "time": sub.cloned().unwrap_or(Value::Null),
        "totalFees": fees.total_fee,
        "fee": fees.total_fee,
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
    let quote_currency = result_val
        .get("request")
        .and_then(|r| r.get("quoteCurrency"))
        .or_else(|| result_val.get("quoteCurrency"))
        .and_then(Value::as_str)
        .unwrap_or_else(|| {
            if symbol.starts_with("US.") {
                "USD"
            } else if symbol.starts_with("SH.")
                || symbol.starts_with("SZ.")
                || symbol.starts_with("CN.")
            {
                "CNY"
            } else {
                "HKD"
            }
        });
    let raw_orders = c
        .get("orders")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();
    let raw_fills = c
        .get("fills")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();

    let mut fees_by_order: HashMap<String, OrderFeeTotals> = HashMap::new();
    for fill in &raw_fills {
        if let Some(order_id) = fill.get("orderId").and_then(Value::as_str) {
            let broker_fee = parse_f64_val(fill.get("brokerFee"));
            let market_fee = parse_f64_val(fill.get("marketFee"));
            let mut total_fee = parse_f64_val(fill.get("totalFee").or_else(|| fill.get("fee")));
            if total_fee == 0.0 && (broker_fee > 0.0 || market_fee > 0.0) {
                total_fee = broker_fee + market_fee;
            }
            let entry = fees_by_order.entry(order_id.to_owned()).or_default();
            entry.broker_fee += broker_fee;
            entry.market_fee += market_fee;
            entry.total_fee += total_fee;
        }
    }

    let mut trades: Vec<Value> = raw_fills
        .into_iter()
        .map(|fill| map_fill_to_trade(&fill, symbol, formal_start_nanos, quote_currency))
        .collect();
    let mut orders: Vec<Value> = raw_orders
        .into_iter()
        .map(|ord| {
            map_order_to_orderbook(
                &ord,
                symbol,
                &fees_by_order,
                formal_start_nanos,
                quote_currency,
            )
        })
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
    let warnings = c
        .get("warnings")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();

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
        .or_else(|| {
            result_val
                .get("result")
                .and_then(|r| r.get("runtimeErrors"))
        })
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
    if let Some(existing) = result_val
        .get("result")
        .and_then(|r| r.get("summary"))
        .and_then(Value::as_object)
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
    let mut summary = result_node
        .get("summary")
        .cloned()
        .unwrap_or_else(|| json!({}));
    if let Some(s_obj) = summary.as_object_mut() {
        for k in [
            "pnl",
            "totalTrades",
            "winRate",
            "sharpeRatio",
            "maxDrawdown",
            "profitFactor",
        ] {
            if !s_obj.contains_key(k)
                && let Some(v) = result_node.get(k)
            {
                s_obj.insert(k.to_owned(), v.clone());
            }
        }
    }

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

fn result_view_run_payload(run_val: &Value) -> Value {
    let empty_map = serde_json::Map::new();
    let req = run_val
        .get("request")
        .and_then(Value::as_object)
        .unwrap_or(&empty_map);
    let provider = run_val
        .get("marketDataProvider")
        .or_else(|| run_val.get("market_data_provider"))
        .or_else(|| req.get("marketDataProviderOverride"))
        .or_else(|| req.get("marketDataProvider"))
        .and_then(Value::as_str)
        .unwrap_or("futu");

    let val = |k: &str| req.get(k).cloned().unwrap_or(Value::Null);

    json!({
        "id": run_val.get("id").unwrap_or(&Value::Null),
        "status": run_val.get("status").unwrap_or(&Value::Null),
        "definitionId": val("definitionId"),
        "definitionVersion": val("definitionVersion"),
        "market": val("market"),
        "code": val("code"),
        "symbol": val("symbol"),
        "instrumentType": val("instrumentType"),
        "marketDataProvider": provider,
        "interval": val("interval"),
        "startDate": val("startDate"),
        "endDate": val("endDate"),
        "startTime": val("startTime"),
        "endTime": val("endTime"),
        "marketTimezone": val("marketTimezone"),
        "initialBalance": val("initialBalance"),
        "rehabType": val("rehabType"),
        "chartType": val("chartType"),
        "executionModel": val("executionModel"),
        "useExtendedHours": val("useExtendedHours"),
        "tradingCosts": val("tradingCosts"),
        "createdAt": run_val.get("createdAt").unwrap_or(&Value::Null),
        "updatedAt": run_val.get("updatedAt").unwrap_or(&Value::Null),
        "request": Value::Object(req.clone()),
    })
}

fn enrich_summary_payload(
    mut summary: Value,
    run_val: &Value,
    data: &NormalizedBacktestData,
) -> Value {
    let req = run_val.get("request").unwrap_or(&Value::Null);
    let initial_balance = parse_f64_val(req.get("initialBalance"));
    let pnl = parse_f64_val(summary.get("pnl"));
    let quote_currency = run_val
        .get("quoteCurrency")
        .or_else(|| req.get("quoteCurrency"))
        .or_else(|| summary.get("quoteCurrency"))
        .and_then(Value::as_str)
        .map(ToOwned::to_owned)
        .unwrap_or_else(|| {
            let sym = req.get("symbol").and_then(Value::as_str).unwrap_or("");
            if sym.starts_with("US.") {
                "USD".to_owned()
            } else if sym.starts_with("SH.") || sym.starts_with("SZ.") || sym.starts_with("CN.") {
                "CNY".to_owned()
            } else {
                "HKD".to_owned()
            }
        });

    if let Some(obj) = summary.as_object_mut() {
        obj.insert("quoteCurrency".to_owned(), json!(quote_currency));
        if initial_balance > 0.0 {
            obj.insert("totalReturn".to_owned(), json!(pnl / initial_balance));
        }
        if let Some(last) = data.logs.last() {
            obj.insert("latestLog".to_owned(), last.clone());
        }
        if let Some(last) = data.warnings.last() {
            obj.insert("latestWarning".to_owned(), last.clone());
        }
        if let Some(last) = data.runtime_errors.last() {
            obj.insert("latestRuntimeError".to_owned(), last.clone());
        }
    }
    summary
}

pub(crate) fn project_authoritative_result_view(
    run_val: &Value,
    seed_candles: Option<&[Value]>,
    request: &BacktestResultViewRequest,
) -> Result<Value, BacktestResultViewError> {
    let params = validate_result_view_request(request)?;
    let mut data = extract_normalized_backtest(run_val, seed_candles);

    let native_interval = run_val
        .get("request")
        .and_then(|r| r.get("interval"))
        .and_then(Value::as_str)
        .unwrap_or("1m");

    let mut resolution_label = None;
    if params.view == "chart" {
        let default_includes = ["candles", "trades", "pnlCurve", "drawdownCurve"];
        let requested_includes: Vec<&str> = request
            .include
            .as_ref()
            .map(|arr| arr.iter().map(String::as_str).collect())
            .filter(|arr: &Vec<&str>| !arr.is_empty())
            .unwrap_or_else(|| default_includes.to_vec());

        if requested_includes.contains(&"candles") {
            let (label, candles) = result_view_candles(
                &data.candles,
                native_interval,
                params.resolution.as_deref(),
                request.start_time.as_deref(),
                request.end_time.as_deref(),
                params.limit,
            )?;
            resolution_label = Some(label);
            data.candles = candles;
        }
    }

    let mut window = serde_json::Map::new();
    window.insert("startTime".to_owned(), json!(request.start_time));
    window.insert("endTime".to_owned(), json!(request.end_time));
    window.insert("nativeInterval".to_owned(), json!(native_interval));
    if let Some(res) = resolution_label {
        window.insert("resolution".to_owned(), json!(res));
    }
    window.insert("limit".to_owned(), json!(params.limit));
    window.insert("cursor".to_owned(), json!(request.cursor));
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

    let run_payload = result_view_run_payload(run_val);
    let summary_payload = enrich_summary_payload(data.summary.clone(), run_val, &data);

    Ok(json!({
        "view": params.view,
        "run": run_payload,
        "summary": summary_payload,
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
            .map(|arr| {
                arr.iter()
                    .filter_map(Value::as_str)
                    .map(str::to_owned)
                    .collect()
            }),
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
