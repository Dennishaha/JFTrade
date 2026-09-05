fn broker_orders_or_fills_schema() -> Value {
    strict_object(
        object([
            ("tradingEnvironment", enum_schema(&["SIMULATE", "REAL"])),
            ("accountId", json!({"type": "string"})),
            ("market", json!({"type": "string"})),
            ("scope", enum_schema(&["CURRENT", "HISTORY"])),
            ("symbol", json!({"type": "string"})),
            ("startTime", json!({"type": "string"})),
            ("endTime", json!({"type": "string"})),
        ]),
        &[],
    )
}

fn broker_cash_flows_schema() -> Value {
    strict_object(
        object([
            ("clearingDate", json!({"type": "string"})),
            ("direction", json!({"type": "string"})),
            ("tradingEnvironment", json!({"type": "string"})),
            ("accountId", json!({"type": "string"})),
            ("market", json!({"type": "string"})),
        ]),
        &["clearingDate"],
    )
}

fn broker_fees_schema() -> Value {
    strict_object(
        object([
            (
                "orderIdEx",
                json!({"type": "array", "items": {"type": "string"}}),
            ),
            (
                "orderIdExList",
                json!({"type": "array", "items": {"type": "string"}}),
            ),
            ("tradingEnvironment", json!({"type": "string"})),
            ("accountId", json!({"type": "string"})),
            ("market", json!({"type": "string"})),
        ]),
        &[],
    )
}

fn broker_margin_ratios_schema() -> Value {
    strict_object(
        object([
            ("symbol", json!({"type": "string"})),
            (
                "symbols",
                json!({"type": "array", "items": {"type": "string"}}),
            ),
            ("tradingEnvironment", json!({"type": "string"})),
            ("accountId", json!({"type": "string"})),
            ("market", json!({"type": "string"})),
        ]),
        &[],
    )
}

fn execution_order_events_schema() -> Value {
    strict_object(
        object([("internalOrderId", json!({"type": "string"}))]),
        &[],
    )
}

fn watchlist_schema() -> Value {
    strict_object(
        object([
            (
                "group",
                json!({"type": "string", "description": "本地分组 ID 或名称；留空时返回分组摘要。"}),
            ),
            (
                "groupName",
                json!({"type": "string", "description": "group 的兼容名称参数。"}),
            ),
            (
                "market",
                json!({"type": "string", "description": "可选市场过滤，例如 HK、US、SH。"}),
            ),
            (
                "query",
                json!({"type": "string", "description": "按名称、代码或 instrumentId 搜索。"}),
            ),
            (
                "cursor",
                json!({"type": "string", "description": "上一页返回的游标。"}),
            ),
            (
                "limit",
                json!({"type": "integer", "minimum": 1, "maximum": 200, "default": 50}),
            ),
            (
                "includeQuotes",
                json!({"type": "boolean", "default": false, "description": "是否附带批量快照；默认 false，不触发行情请求。"}),
            ),
        ]),
        &[],
    )
}

fn portfolio_schema(orders: bool) -> Value {
    let mut properties = object([
        ("accountId", string_schema(1, 128)),
        ("tradingEnvironment", enum_schema(&["SIMULATE", "REAL"])),
        ("market", enum_schema(&["HK", "US", "SH", "SZ"])),
    ]);
    if orders {
        properties.insert("activeOnly".to_owned(), json!({"type": "boolean"}));
    }
    strict_object(properties, &["tradingEnvironment"])
}

fn backtest_runs_schema() -> Value {
    strict_object(
        object([
            (
                "definitionId",
                json!({"type": "string", "description": "可选策略定义 ID 过滤。"}),
            ),
            (
                "definitionVersion",
                json!({"type": "string", "description": "可选不可变策略版本号过滤。"}),
            ),
            (
                "status",
                json!({"type": "string", "description": "可选回测状态过滤，不区分大小写。"}),
            ),
            (
                "marketDataProvider",
                json!({
                    "type": "string",
                    "enum": ["futu", "yfinance", "akshare"],
                    "description": "可选行情提供者过滤。"
                }),
            ),
            (
                "limit",
                json!({
                    "type": "integer",
                    "minimum": 1,
                    "maximum": 200,
                    "description": "最多返回的匹配运行数。"
                }),
            ),
        ]),
        &[],
    )
}

fn backtest_result_view_schema() -> Value {
    strict_object(
        object([
            ("runId", json!({"type": "string"})),
            (
                "view",
                enum_schema(&["summary", "chart", "orders", "logs", "warnings", "errors"]),
            ),
            (
                "resolution",
                json!({
                    "type": "string",
                    "description": "chart 视图精度，auto 或 1m/5m/1h/1d 等；不得细于原生周期。"
                }),
            ),
            ("startTime", json!({"type": "string"})),
            ("endTime", json!({"type": "string"})),
            (
                "include",
                json!({
                    "type": "array",
                    "items": enum_schema(&["candles", "trades", "pnlCurve", "drawdownCurve"]),
                }),
            ),
            (
                "limit",
                json!({"type": "integer", "minimum": 1, "maximum": 2000}),
            ),
            ("cursor", json!({"type": "string"})),
        ]),
        &["runId"],
    )
}

fn backtest_kline_sync_status_schema() -> Value {
    strict_object(
        object([
            ("taskId", json!({"type": "string"})),
            (
                "waitForCompletionMs",
                json!({
                    "type": "integer",
                    "minimum": 0,
                    "maximum": 25000,
                    "description": "可选短等待，最多 25000ms。"
                }),
            ),
        ]),
        &["taskId"],
    )
}

fn strategy_definition_versions_list_schema() -> Value {
    strict_object(
        object([(
            "definitionId",
            json!({"type": "string", "description": "策略定义 ID。"}),
        )]),
        &["definitionId"],
    )
}

fn strategy_definition_versions_get_schema() -> Value {
    strict_object(
        object([
            (
                "definitionId",
                json!({"type": "string", "description": "策略定义 ID。"}),
            ),
            (
                "version",
                json!({"type": "string", "description": "不可变策略版本号，例如 0.1.0。"}),
            ),
        ]),
        &["definitionId", "version"],
    )
}

fn strategy_instance_activity_schema() -> Value {
    strict_object(
        object([
            ("instanceId", json!({"type": "string", "minLength": 1})),
            ("kind", enum_schema(&["logs", "audit"])),
            (
                "limit",
                json!({"type": "integer", "minimum": 1, "maximum": 200}),
            ),
            ("offset", json!({"type": "integer", "minimum": 0})),
        ]),
        &["instanceId"],
    )
}

fn strategy_pine_spec_schema() -> Value {
    strict_object(
        object([
            (
                "section",
                enum_schema(&[
                    "overview",
                    "syntax",
                    "expressions",
                    "indicators",
                    "orders",
                    "unsupported",
                    "examples",
                ]),
            ),
            ("includeExamples", json!({"type": "boolean"})),
        ]),
        &[],
    )
}

fn strategy_validate_pine_schema() -> Value {
    strict_object(
        object([
            (
                "script",
                json!({
                    "type": "string",
                    "description": "待校验的 Pine Script v6 策略脚本。"
                }),
            ),
            ("includeRequirements", json!({"type": "boolean"})),
        ]),
        &["script"],
    )
}

fn trading_costs_schema() -> Value {
    let fee_rule = strict_object(
        object([
            ("id", json!({"type": "string"})),
            ("label", json!({"type": "string"})),
            ("category", json!({"type": "string"})),
            ("side", json!({"type": "string"})),
            ("basis", json!({"type": "string"})),
            ("rate", json!({"type": "number"})),
            ("fixedAmount", json!({"type": "number"})),
            ("minAmount", json!({"type": "number"})),
            ("maxAmount", json!({"type": "number"})),
            ("maxRate", json!({"type": "number"})),
            ("rounding", json!({"type": "string"})),
            ("currency", json!({"type": "string"})),
            (
                "appliesTo",
                json!({"type": "array", "items": {"type": "string"}}),
            ),
            ("effectiveFrom", json!({"type": "string"})),
            ("effectiveTo", json!({"type": "string"})),
            ("sourceUrl", json!({"type": "string"})),
        ]),
        &["id", "label", "category", "basis"],
    );
    let fee_schedule = strict_object(
        object([
            (
                "mode",
                enum_schema(&["none", "market_preset", "custom", "script"]),
            ),
            ("presetId", json!({"type": "string"})),
            ("rules", json!({"type": "array", "items": fee_rule})),
        ]),
        &[],
    );
    strict_object(
        object([
            ("brokerFees", fee_schedule.clone()),
            ("marketFees", fee_schedule),
        ]),
        &[],
    )
}

fn backtest_result_view_options_schema() -> Value {
    strict_object(
        object([
            ("runId", json!({"type": "string"})),
            (
                "view",
                enum_schema(&[
                    "summary", "chart", "orders", "logs", "warnings", "errors",
                ]),
            ),
            (
                "resolution",
                json!({
                    "type": "string",
                    "description": "chart 视图精度，auto 或 1m/5m/1h/1d 等；不得细于原生周期。"
                }),
            ),
            ("startTime", json!({"type": "string"})),
            ("endTime", json!({"type": "string"})),
            (
                "include",
                json!({
                    "type": "array",
                    "items": enum_schema(&["candles", "trades", "pnlCurve", "drawdownCurve"]),
                }),
            ),
            (
                "limit",
                json!({"type": "integer", "minimum": 1, "maximum": 2000}),
            ),
            ("cursor", json!({"type": "string"})),
        ]),
        &[],
    )
}

fn strategy_research_backtest_schema() -> Value {
    let mut schema = strict_object(
        object([
            (
                "script",
                json!({
                    "type": "string",
                    "description": "临时 Pine Script v6 策略脚本；不会保存为策略定义。"
                }),
            ),
            ("market", json!({"type": "string"})),
            ("symbol", json!({"type": "string"})),
            ("code", json!({"type": "string"})),
            (
                "interval",
                json!({
                    "type": "string",
                    "description": "回测原生周期，例如 1m、5m、1d；默认 1m。"
                }),
            ),
            ("instrumentType", enum_schema(&["stock", "etf"])),
            ("startDate", json!({"type": "string"})),
            ("endDate", json!({"type": "string"})),
            (
                "startTime",
                json!({
                    "type": "string",
                    "description": "RFC3339 开始时间。"
                }),
            ),
            (
                "endTime",
                json!({
                    "type": "string",
                    "description": "RFC3339 结束时间。"
                }),
            ),
            ("initialBalance", json!({"type": "number", "exclusiveMinimum": 0})),
            ("chartType", enum_schema(&["standard", "heikinashi"])),
            ("rehabType", enum_schema(&["forward", "backward", "none"])),
            ("useExtendedHours", json!({"type": "boolean"})),
            ("tradingCosts", trading_costs_schema()),
            ("executionModel", enum_schema(&["conservative-bar-v1"])),
            (
                "marketDataProvider",
                enum_schema(&["futu", "yfinance", "akshare"]),
            ),
            (
                "waitForCompletionMs",
                json!({
                    "type": "integer",
                    "minimum": 0,
                    "maximum": 25000,
                    "description": "可选短等待，最多 25000ms；长轮询请用 workflow.wait 后再查 backtest.result_view。"
                }),
            ),
            ("resultView", backtest_result_view_options_schema()),
        ]),
        &["script", "market"],
    );
    if let Some(obj) = schema.as_object_mut() {
        obj.insert(
            "anyOf".to_owned(),
            json!([
                {"required": ["symbol"]},
                {"required": ["code"]}
            ]),
        );
    }
    schema
}

fn default_query_schema() -> Value {
    strict_object(
        object([(
            "query",
            json!({"type": "string", "description": "原始用户请求或提取后的查询内容。"}),
        )]),
        &[],
    )
}
