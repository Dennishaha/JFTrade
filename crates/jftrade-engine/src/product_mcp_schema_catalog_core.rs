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
            ("definitionId", json!({"type": "string"})),
            ("definitionVersion", json!({"type": "string"})),
            ("status", json!({"type": "string"})),
            (
                "marketDataProvider",
                enum_schema(&["futu", "yfinance", "akshare"]),
            ),
            (
                "limit",
                json!({"type": "integer", "minimum": 1, "maximum": 200}),
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
            ("resolution", json!({"type": "string"})),
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
                json!({"type": "integer", "minimum": 0, "maximum": 25000}),
            ),
        ]),
        &["taskId"],
    )
}

fn strategy_definition_versions_list_schema() -> Value {
    strict_object(
        object([(
            "definitionId",
            json!({"type": "string"}), /* description is wire metadata */
        )]),
        &["definitionId"],
    )
}

fn strategy_definition_versions_get_schema() -> Value {
    strict_object(
        object([
            ("definitionId", json!({"type": "string"})),
            ("version", json!({"type": "string"})),
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

fn default_query_schema() -> Value {
    strict_object(
        object([(
            "query",
            json!({"type": "string", "description": "原始用户请求或提取后的查询内容。"}),
        )]),
        &[],
    )
}
