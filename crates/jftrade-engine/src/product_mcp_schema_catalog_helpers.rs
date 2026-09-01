fn instrument_operation_properties(name: &str) -> Properties {
    let operations = operation_values(name).unwrap_or(&[]);
    instrument_operation_properties_with(name, operations)
}

fn instrument_operation_properties_with(_name: &str, operations: &[&str]) -> Properties {
    let mut properties = instrument_properties();
    if !operations.is_empty() {
        properties.insert("operation".to_owned(), enum_schema(operations));
    }
    properties.insert("startTime".to_owned(), string_schema(1, 40));
    properties.insert("endTime".to_owned(), string_schema(1, 40));
    properties.insert("period".to_owned(), string_schema(1, 20));
    properties
}

fn operation_properties(name: &str) -> Properties {
    let operations = operation_values(name).unwrap_or(&[]);
    operation_properties_with(name, operations)
}

fn operation_properties_with(name: &str, operations: &[&str]) -> Properties {
    let mut properties = Properties::new();
    if !operations.is_empty() {
        properties.insert("operation".to_owned(), enum_schema(operations));
    }
    properties.insert("instrumentId".to_owned(), string_schema(3, 80));
    properties.insert("underlying".to_owned(), string_schema(3, 80));
    match name {
        "research.rankings" => {
            properties.insert("direction".to_owned(), enum_schema(&["up", "down"]));
            properties.insert(
                "plateType".to_owned(),
                enum_schema(&["industry", "concept", "theme"]),
            );
        }
        "research.calendar" => {
            for key in ["beginDate", "endDate", "date"] {
                properties.insert(key.to_owned(), string_schema(10, 10));
            }
            properties.insert(
                "sort".to_owned(),
                enum_schema(&[
                    "hot",
                    "market_cap",
                    "option_volume",
                    "iv",
                    "iv_rank",
                    "iv_percentile",
                ]),
            );
            properties.insert(
                "stockScope".to_owned(),
                enum_schema(&["all", "watchlist", "position", "special"]),
            );
            for key in [
                "marketCapMin",
                "marketCapMax",
                "optionVolumeMin",
                "optionVolumeMax",
            ] {
                properties.insert(key.to_owned(), calendar_numeric_filter_schema(None));
            }
            for key in [
                "ivMin",
                "ivMax",
                "ivRankMin",
                "ivRankMax",
                "ivPercentileMin",
                "ivPercentileMax",
            ] {
                properties.insert(key.to_owned(), calendar_numeric_filter_schema(Some(100.0)));
            }
        }
        "research.macro" => {
            properties.insert("indicatorId".to_owned(), string_schema(1, 120));
        }
        "research.institutions" => {
            properties.insert(
                "institutionId".to_owned(),
                json!({"type": "integer", "minimum": 1}),
            );
        }
        "research.industry" => {
            properties.insert(
                "plateType".to_owned(),
                enum_schema(&["all", "industry", "concept", "region"]),
            );
            properties.insert(
                "plateSetType".to_owned(),
                enum_schema(&["all", "industry", "concept", "region"]),
            );
            properties.insert(
                "chainId".to_owned(),
                json!({"type": "integer", "minimum": 1}),
            );
            properties.insert(
                "plateId".to_owned(),
                json!({"type": "integer", "minimum": 1}),
            );
        }
        _ => {}
    }
    properties
}

fn research_macro_schema() -> Value {
    let mut schema = strict_object(
        read_properties(operation_properties("research.macro")),
        &[],
    );
    schema["if"] = json!({
        "properties": {"operation": {"const": "indicator_history"}},
        "required": ["operation"],
    });
    schema["then"] = json!({"required": ["indicatorId"]});
    schema
}

fn operation_values(name: &str) -> Option<&'static [&'static str]> {
    Some(match name {
        "market.candles" => &["current", "historical"],
        "market.depth" => &["depth"],
        "market.capital_flow" => &["flow", "distribution"],
        "derivatives.option_chain" => &["expirations", "chain"],
        "derivatives.option_analysis" => &[
            "quote",
            "volatility",
            "exercise_probability",
            "strategy",
            "strategy_analysis",
            "strategy_spread",
            "market_statistics",
            "historical_statistics",
            "underlying_overview",
            "historical_volatility",
            "underlying_rank",
            "contract_rank",
        ],
        "derivatives.option_events" => &[
            "unusual",
            "zero_dte",
            "zero_dte_contract",
            "earnings",
            "seller",
        ],
        "derivatives.warrants" => &["related", "list", "screen"],
        "derivatives.futures" => &[],
        "research.macro" => &[
            "indicators",
            "indicator_history",
            "fed_target_rate",
            "fed_dot_plot",
        ],
        "research.institutions" => &[
            "list",
            "profile",
            "distribution",
            "holding_changes",
            "holdings",
            "ark_fund_holdings",
            "ark_stock_activity",
            "ark_transactions",
        ],
        "research.industry" => &[
            "chains",
            "chain_detail",
            "chains_by_plate",
            "plate",
            "plate_stocks",
            "owner_plates",
            "plate_list",
            "plate_members",
        ],
        "research.technical_indicators" => &["list", "calculate"][..],
        _ => return None,
    })
}

fn instrument_properties() -> Properties {
    object([("instrumentId", string_schema(3, 80))])
}

fn read_properties(extra: Properties) -> Properties {
    let mut properties = object([
        ("brokerId", string_schema(1, 64)),
        ("accountId", string_schema(1, 128)),
        ("market", enum_schema(&["HK", "US", "SH", "SZ"])),
        ("cursor", string_schema(1, 512)),
        (
            "pageSize",
            json!({"type": "integer", "minimum": 1, "maximum": 100}),
        ),
        ("refresh", json!({"type": "boolean"})),
    ]);
    properties.extend(extra);
    properties
}

fn common_capability_properties() -> Properties {
    object([
        ("brokerId", string_schema(1, 64)),
        ("accountId", string_schema(1, 128)),
        ("tradingEnvironment", enum_schema(&["SIMULATE", "REAL"])),
        ("market", enum_schema(&["HK", "US", "SH", "SZ"])),
        ("featureId", string_schema(1, 100)),
    ])
}

fn calendar_numeric_filter_schema(maximum: Option<f64>) -> Value {
    let mut number = json!({"type": "number", "minimum": 0});
    if let Some(maximum) = maximum {
        number["maximum"] = json!(maximum);
    }
    json!({"anyOf": [number, string_schema(1, 40)]})
}

fn enum_schema(values: &[&str]) -> Value {
    json!({"type": "string", "enum": values})
}

fn string_schema(min_length: usize, max_length: usize) -> Value {
    json!({"type": "string", "minLength": min_length, "maxLength": max_length})
}

fn positive_number_schema() -> Value {
    json!({"type": "number", "exclusiveMinimum": 0})
}

fn strict_object(properties: Properties, required: &[&str]) -> Value {
    let mut schema = json!({
        "type": "object",
        "properties": Value::Object(properties),
        "additionalProperties": false,
    });
    if !required.is_empty() {
        schema["required"] = json!(required);
    }
    schema
}

fn object(entries: impl IntoIterator<Item = (&'static str, Value)>) -> Properties {
    entries
        .into_iter()
        .map(|(key, value)| (key.to_owned(), value))
        .collect()
}
