fn prediction_discovery_schema() -> Value {
    strict_object(
        read_properties({
            let mut properties = operation_properties_with(
                "prediction.discover",
                &[
                    "categories",
                    "competitions",
                    "series",
                    "events",
                    "contracts",
                    "milestones",
                ],
            );
            properties.insert("category".to_owned(), string_schema(1, 120));
            properties.insert("tag".to_owned(), string_schema(1, 120));
            properties.insert("seriesId".to_owned(), string_schema(1, 120));
            properties.insert("eventId".to_owned(), string_schema(1, 120));
            properties
        }),
        &["operation"],
    )
}

fn prediction_quote_schema() -> Value {
    strict_object(
        read_properties(object([
            ("brokerId", string_schema(1, 64)),
            ("accountId", string_schema(1, 128)),
            ("tradingEnvironment", enum_schema(&["SIMULATE", "REAL"])),
            ("market", enum_schema(&["US"])),
            ("mvc", string_schema(1, 160)),
            (
                "legs",
                json!({
                    "type": "array",
                    "minItems": 2,
                    "maxItems": 20,
                    "items": event_leg_schema(),
                }),
            ),
        ])),
        &["accountId", "mvc", "legs"],
    )
}

fn technical_indicator_schema() -> Value {
    let kline = strict_object(
        object([
            ("time", string_schema(1, 80)),
            ("isBlank", json!({"type": "boolean"})),
            ("highPrice", json!({"type": "number"})),
            ("openPrice", json!({"type": "number"})),
            ("lowPrice", json!({"type": "number"})),
            ("closePrice", json!({"type": "number"})),
            ("lastClosePrice", json!({"type": "number"})),
            ("volume", json!({"type": "integer", "minimum": 0})),
            ("turnover", json!({"type": "number"})),
            ("turnoverRate", json!({"type": "number"})),
            ("pe", json!({"type": "number"})),
            ("changeRate", json!({"type": "number"})),
            ("timestamp", json!({"type": "number"})),
            ("hpVolume", json!({"type": "number"})),
        ]),
        &["time"],
    );
    let input = strict_object(
        object([
            ("index", json!({"type": "integer", "minimum": 0})),
            ("value", string_schema(0, 256)),
        ]),
        &["index"],
    );
    let mut properties = read_properties(instrument_operation_properties(
        "research.technical_indicators",
    ));
    properties.extend(object([
        ("searchKey", string_schema(0, 128)),
        ("langType", json!({"type": "integer", "minimum": 0, "maximum": 2})),
        ("searchMode", json!({"type": "integer", "enum": [0, 1]})),
        ("shortName", string_schema(1, 128)),
        ("klType", json!({"type": "integer", "minimum": 1, "maximum": 15})),
        (
            "kLine",
            json!({"type": "array", "items": kline, "maxItems": 2000}),
        ),
        ("num", json!({"type": "integer", "minimum": 1, "maximum": 2000})),
        (
            "inputs",
            json!({"type": "array", "items": input, "maxItems": 100}),
        ),
    ]));
    // The list operation accepts langType 0, while calculation is restricted
    // to the MyLang/Python engines and needs the complete calculation payload.
    let mut schema = strict_object(properties, &["instrumentId"]);
    schema["if"] = json!({
        "properties": {"operation": {"const": "calculate"}},
        "required": ["operation"],
    });
    schema["then"] = json!({
        "required": ["shortName", "langType", "klType", "kLine"],
        "properties": {"langType": {"enum": [1, 2]}},
    });
    schema
}

fn market_series_schema(candles: bool) -> Value {
    let mut properties = read_properties(object([
        ("instrumentId", string_schema(3, 80)),
        ("symbol", string_schema(1, 80)),
    ]));
    if candles {
        properties.insert(
            "operation".to_owned(),
            enum_schema(&["current", "historical"]),
        );
        properties.insert("period".to_owned(), string_schema(1, 20));
        properties.insert(
            "limit".to_owned(),
            json!({"type": "integer", "minimum": 1, "maximum": 500}),
        );
        for key in ["startTime", "endTime", "beforeTime"] {
            properties.insert(key.to_owned(), string_schema(1, 40));
        }
        properties.insert(
            "sessions".to_owned(),
            json!({
                "type": "array",
                "items": enum_schema(&["regular", "extended", "overnight"]),
                "minItems": 1,
                "maxItems": 3,
            }),
        );
        properties.insert(
            "adjustment".to_owned(),
            enum_schema(&["none", "forward", "backward"]),
        );
    } else {
        properties.insert(
            "num".to_owned(),
            json!({"type": "integer", "minimum": 1, "maximum": 50}),
        );
    }
    let mut schema = strict_object(properties, &[]);
    schema["anyOf"] = json!([
        {"required": ["instrumentId"]},
        {"required": ["market", "symbol"]},
    ]);
    schema
}

fn research_screen_schema() -> Value {
    let factor_params = strict_object(
        object([
            ("days", json!({"type": "integer"})),
            ("periodAverage", json!({"type": "integer"})),
            ("term", json!({"type": "integer"})),
            ("duration", json!({"type": "integer"})),
            ("year", json!({"type": "integer"})),
            ("futureDuration", json!({"type": "integer"})),
            ("period", json!({"type": "integer"})),
            ("rangePeriod", json!({"type": "integer"})),
            ("firstCustomParam", json!({"type": "integer"})),
            (
                "indicatorParams",
                json!({"type": "array", "items": {"type": "integer"}}),
            ),
            ("brokerParam", json!({"type": "string"})),
            ("optionParamType", json!({"type": "integer"})),
            ("optionParamString", json!({"type": "string"})),
            ("optionParamInteger", json!({"type": "integer"})),
            (
                "optionParamIntegers",
                json!({"type": "array", "items": {"type": "integer"}}),
            ),
            ("optionHvPeriod", json!({"type": "integer"})),
        ]),
        &[],
    );
    let factor = strict_object(
        object([
            ("instanceId", string_schema(1, 120)),
            ("factorKey", string_schema(1, 160)),
            ("params", factor_params),
        ]),
        &["factorKey"],
    );
    let condition = strict_object(
        object([
            ("id", string_schema(1, 120)),
            ("factor", factor.clone()),
            ("operator", string_schema(1, 40)),
            ("value", json!({})),
            ("secondFactor", factor.clone()),
        ]),
        &["factor", "operator"],
    );
    let column = strict_object(
        object([
            ("columnId", string_schema(1, 120)),
            ("factor", factor.clone()),
            ("label", string_schema(0, 160)),
        ]),
        &["columnId", "factor"],
    );
    let sort = strict_object(
        object([
            ("sortId", string_schema(0, 120)),
            ("columnId", string_schema(0, 120)),
            ("factor", factor),
            (
                "direction",
                enum_schema(&["asc", "desc", "abs_asc", "abs_desc"]),
            ),
        ]),
        &["factor", "direction"],
    );
    let mut properties = read_properties(object([
        ("operation", enum_schema(&["stock_v2"])),
        ("tradingEnvironment", enum_schema(&["SIMULATE", "REAL"])),
        (
            "pool",
            strict_object(
                object([
                    (
                        "watchlistStockIds",
                        json!({"type": "array", "items": string_schema(1, 120)}),
                    ),
                    (
                        "plates",
                        json!({
                            "type": "array",
                            "items": strict_object(
                                object([
                                    ("parentPlateId", string_schema(0, 80)),
                                    (
                                        "plateIds",
                                        json!({"type": "array", "items": string_schema(1, 80)}),
                                    ),
                                ]),
                                &["plateIds"],
                            ),
                        }),
                    ),
                ]),
                &[],
            ),
        ),
        (
            "conditions",
            json!({"type": "array", "items": condition, "maxItems": 50}),
        ),
        (
            "columns",
            json!({"type": "array", "items": column, "maxItems": 50}),
        ),
        (
            "sorts",
            json!({"type": "array", "items": sort, "maxItems": 20}),
        ),
        ("catalogVersion", string_schema(1, 120)),
        (
            "querySchemaVersion",
            json!({"type": "integer", "minimum": 2}),
        ),
        (
            "page",
            strict_object(
                object([
                    ("offset", json!({"type": "integer", "minimum": 0})),
                    (
                        "limit",
                        json!({"type": "integer", "minimum": 1, "maximum": 100}),
                    ),
                ]),
                &[],
            ),
        ),
    ]));
    // `read_properties` already adds the common routing fields; this update
    // preserves the Go schema's explicit market enum in that common map.
    properties.insert("market".to_owned(), enum_schema(&["HK", "US", "SH", "SZ"]));
    strict_object(
        properties,
        &["market", "pool", "catalogVersion", "querySchemaVersion"],
    )
}

fn product_rule_properties() -> Properties {
    object([
        ("brokerId", string_schema(1, 64)),
        ("accountId", string_schema(1, 128)),
        ("tradingEnvironment", enum_schema(&["SIMULATE", "REAL"])),
        ("market", enum_schema(&["HK", "US", "SH", "SZ"])),
        ("featureId", enum_schema(&["execution.buying_power"])),
        ("instrument", instrument_schema()),
        (
            "orderKind",
            enum_schema(&["single", "option_combo", "event_single", "event_parlay"]),
        ),
        ("orderType", string_schema(1, 40)),
        ("session", string_schema(1, 40)),
        ("quantity", positive_number_schema()),
        ("amount", positive_number_schema()),
        ("price", positive_number_schema()),
        ("legs", order_legs_schema()),
    ])
}

fn instrument_schema() -> Value {
    strict_object(
        object([
            ("instrumentId", string_schema(3, 80)),
            ("code", string_schema(1, 80)),
            (
                "productClass",
                enum_schema(&[
                    "equity",
                    "fund",
                    "option",
                    "warrant",
                    "cbbc",
                    "future",
                    "event_contract",
                    "index",
                    "bond",
                ]),
            ),
            (
                "marketSegment",
                enum_schema(&["securities", "derivatives", "prediction"]),
            ),
            ("quoteMarket", enum_schema(&["HK", "US", "SH", "SZ"])),
            ("tradeMarket", enum_schema(&["HK", "US", "SH", "SZ"])),
            (
                "quantityMode",
                enum_schema(&["units", "contracts", "amount"]),
            ),
        ]),
        &[
            "instrumentId",
            "productClass",
            "marketSegment",
            "quoteMarket",
            "quantityMode",
        ],
    )
}

fn order_legs_schema() -> Value {
    json!({"type": "array", "minItems": 1, "maxItems": 20, "items": order_leg_schema()})
}

fn order_leg_schema() -> Value {
    strict_object(
        object([
            ("instrumentId", string_schema(3, 80)),
            ("productClass", enum_schema(&["option", "event_contract"])),
            ("side", enum_schema(&["BUY", "SELL"])),
            (
                "ratio",
                json!({"type": "integer", "minimum": 1, "maximum": 100}),
            ),
            ("quantity", positive_number_schema()),
            ("amount", positive_number_schema()),
            ("price", positive_number_schema()),
            ("predictionSide", enum_schema(&["YES", "NO"])),
        ]),
        &["instrumentId", "productClass", "side", "ratio"],
    )
}

fn event_leg_schema() -> Value {
    strict_object(
        object([
            ("instrumentId", string_schema(3, 80)),
            ("predictionSide", enum_schema(&["YES", "NO"])),
            ("side", enum_schema(&["BUY"])),
            (
                "ratio",
                json!({"type": "integer", "minimum": 1, "maximum": 100}),
            ),
        ]),
        &["instrumentId", "predictionSide"],
    )
}
