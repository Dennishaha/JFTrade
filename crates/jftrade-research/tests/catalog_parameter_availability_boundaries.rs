use jftrade_research::{ScreenCatalogError, screen_catalog};
use serde_json::Value;

fn factor<'a>(catalog: &'a Value, key: &str) -> &'a Value {
    catalog["factors"]
        .as_array()
        .expect("catalog factors")
        .iter()
        .find(|candidate| {
            candidate["key"]
                .as_str()
                .is_some_and(|candidate| candidate.eq_ignore_ascii_case(key))
        })
        .unwrap_or_else(|| panic!("catalog is missing factor {key}"))
}

fn parameter<'a>(catalog: &'a Value, factor_key: &str, name: &str) -> &'a Value {
    factor(catalog, factor_key)["parameters"]
        .as_array()
        .unwrap_or_else(|| panic!("factor {factor_key} has no parameters"))
        .iter()
        .find(|candidate| candidate["name"].as_str() == Some(name))
        .unwrap_or_else(|| panic!("factor {factor_key} is missing parameter {name}"))
}

/// Iterates catalog factor keys the way the Go editor lookup does: matching the
/// frozen key case-insensitively so `SIMPLE.CHANGE_PCT` resolves too.
fn factor_keys(catalog: &Value) -> impl Iterator<Item = &str> {
    catalog["factors"]
        .as_array()
        .expect("catalog factors")
        .iter()
        .filter_map(|candidate| candidate["key"].as_str())
}

/// Parity: go:452dea11:pkg/researchscreen/catalog_test.go:9
/// TestCatalogIsCompleteStableAndDoesNotExposeProviderEnums
///
/// Go pins the catalog header, the 402-factor / 11-category surface, the
/// period/term enum sizes, ten representative factors and the guarantee that
/// provider enum values never leak into the public projection.
#[test]
fn futu_catalog_preserves_stable_shape_and_public_projection() {
    let catalog = screen_catalog("futu", "").expect("futu catalog");
    assert_eq!(catalog["version"], "futu-stock-screen-v1");
    assert_eq!(catalog["schemaVersion"], 2);
    assert_eq!(catalog["querySchemaVersion"], 2);
    assert_eq!(catalog["provider"], "futu");
    assert_eq!(catalog["providerVersion"], "10.9.6908");
    assert_eq!(catalog["factors"].as_array().expect("factors").len(), 402);
    assert_eq!(
        catalog["categories"].as_array().expect("categories").len(),
        11
    );
    assert_eq!(
        catalog["enums"]["period"]
            .as_array()
            .expect("period enum")
            .len(),
        10
    );
    assert_eq!(
        catalog["enums"]["term"]
            .as_array()
            .expect("term enum")
            .len(),
        14
    );

    for key in [
        "basic.code",
        "simple.price",
        "cumulative.price_change_pct",
        "financial.net_profit",
        "indicator.macd_dif",
        "pattern.macd_gold_cross",
        "featured.chips_profit_ratio",
        "broker.holdings_ratio",
        "option.stock_iv",
        "kline_shape.shape_type",
    ] {
        let descriptor = factor(&catalog, key);
        assert!(
            descriptor.get("providerId").is_none(),
            "{key} leaked provider id"
        );
    }
    assert!(catalog.to_string().find("ProviderID").is_none());
}

/// Parity: go:452dea11:pkg/researchscreen/catalog_test.go:45
/// TestCatalogParametersExposeEditorContract
///
/// Go requires every parameter of the editor-facing factors to carry an editor
/// type, default, step and minimum, the factors to publish roles/help/search
/// keywords, and every filterable factor to expose a condition editor,
/// operators and a resolvable value enum.
#[test]
fn futu_parameters_preserve_editor_bounds_defaults_and_enum_metadata() {
    let catalog = screen_catalog("futu", "").expect("futu catalog");
    let expectations = [
        (
            "cumulative.price_change_pct",
            "days",
            "number",
            true,
            serde_json::json!(1),
            1,
            Some(3650),
            "",
        ),
        (
            "cumulative.price_change_pct",
            "periodAverage",
            "number",
            false,
            serde_json::json!(0),
            0,
            None,
            "",
        ),
        (
            "indicator.ma",
            "period",
            "select",
            true,
            serde_json::json!(11),
            0,
            None,
            "period",
        ),
        (
            "indicator.ma",
            "indicatorParams",
            "multiNumber",
            false,
            serde_json::json!([]),
            0,
            None,
            "",
        ),
        (
            "financial.roe",
            "term",
            "select",
            false,
            serde_json::json!(10),
            0,
            None,
            "term",
        ),
        (
            "financial.net_profit",
            "futureDuration",
            "select",
            false,
            serde_json::json!(0),
            0,
            None,
            "future_duration",
        ),
        (
            "featured.chips_profit_ratio",
            "rangePeriod",
            "select",
            false,
            serde_json::json!(1),
            0,
            None,
            "range_period",
        ),
        (
            "financial.net_profit",
            "duration",
            "number",
            false,
            serde_json::json!(0),
            0,
            None,
            "",
        ),
        (
            "financial.net_profit",
            "year",
            "number",
            false,
            serde_json::json!(0),
            0,
            None,
            "",
        ),
        (
            "featured.chips_profit_ratio",
            "firstCustomParam",
            "number",
            false,
            serde_json::json!(0),
            0,
            None,
            "",
        ),
        (
            "broker.holdings_ratio",
            "brokerParam",
            "text",
            false,
            serde_json::json!(""),
            0,
            None,
            "",
        ),
        (
            "option.stock_iv",
            "optionParam",
            "union",
            false,
            serde_json::json!(""),
            0,
            None,
            "",
        ),
        (
            "option.stock_iv",
            "optionHvPeriod",
            "select",
            false,
            serde_json::json!(0),
            0,
            None,
            "option_hv_period",
        ),
    ];

    for (factor_key, name, editor, required, default, minimum, maximum, enum_name) in expectations {
        let descriptor = parameter(&catalog, factor_key, name);
        assert_eq!(
            descriptor["editorType"], editor,
            "{factor_key}.{name} editor"
        );
        assert_eq!(
            descriptor["required"], required,
            "{factor_key}.{name} required"
        );
        assert_eq!(
            descriptor["default"], default,
            "{factor_key}.{name} default"
        );
        assert_eq!(
            descriptor["minimum"], minimum,
            "{factor_key}.{name} minimum"
        );
        assert_eq!(descriptor["step"], 1, "{factor_key}.{name} step");
        match maximum {
            Some(maximum) => assert_eq!(descriptor["maximum"], maximum),
            None => assert!(descriptor.get("maximum").is_none()),
        }
        if enum_name.is_empty() {
            assert!(descriptor.get("enum").is_none());
        } else {
            assert_eq!(descriptor["enum"], enum_name);
            assert!(
                catalog["enums"][enum_name]
                    .as_array()
                    .is_some_and(|values| !values.is_empty())
            );
        }
    }

    // Every generated row receives the same editor contract before it is
    // exposed. This guards against a provider refresh introducing an
    // uneditable or non-serializable parameter outside the examples above.
    for candidate in catalog["factors"].as_array().expect("catalog factors") {
        for descriptor in candidate
            .get("parameters")
            .and_then(Value::as_array)
            .into_iter()
            .flatten()
        {
            assert!(
                descriptor["name"]
                    .as_str()
                    .is_some_and(|name| !name.is_empty())
            );
            assert!(
                descriptor["type"]
                    .as_str()
                    .is_some_and(|kind| !kind.is_empty())
            );
            assert!(
                descriptor["editorType"]
                    .as_str()
                    .is_some_and(|editor| !editor.is_empty())
            );
            assert!(
                descriptor
                    .get("default")
                    .is_some_and(|value| !value.is_null())
            );
            assert!(descriptor.get("minimum").is_some_and(Value::is_number));
            assert!(descriptor.get("step").is_some_and(Value::is_number));
        }
    }

    // Go requires the editor-facing factors to publish roles, help text and
    // search keywords, and every filterable factor to expose a condition editor,
    // operators and a resolvable value enum.
    for factor_key in [
        "cumulative.price_change_pct",
        "financial.roe",
        "indicator.ma",
        "option.stock_iv",
    ] {
        let descriptor = factor(&catalog, factor_key);
        assert!(
            descriptor["roles"]
                .as_array()
                .is_some_and(|roles| !roles.is_empty()),
            "{factor_key} must publish roles"
        );
        assert!(
            descriptor["help"]
                .as_str()
                .is_some_and(|help| !help.is_empty()),
            "{factor_key} must publish help text"
        );
        assert!(
            descriptor["searchKeywords"]
                .as_array()
                .is_some_and(|keywords| !keywords.is_empty()),
            "{factor_key} must publish search keywords"
        );
    }
    for candidate in catalog["factors"].as_array().expect("catalog factors") {
        if candidate["filter"] != Value::Bool(true) {
            continue;
        }
        let key = candidate["key"].as_str().expect("factor key");
        assert!(
            candidate["conditionEditor"]
                .as_str()
                .is_some_and(|editor| !editor.is_empty()),
            "{key} must publish a condition editor"
        );
        assert!(
            candidate["operators"]
                .as_array()
                .is_some_and(|operators| !operators.is_empty()),
            "{key} must publish its operators"
        );
        if let Some(enum_name) = candidate["valueEnum"]
            .as_str()
            .filter(|name| !name.is_empty())
        {
            assert!(
                catalog["enums"][enum_name]
                    .as_array()
                    .is_some_and(|values| !values.is_empty()),
                "{key} value enum {enum_name} must resolve"
            );
        }
    }
}

#[test]
fn catalog_availability_preserves_market_limits_and_reasons() {
    let cases = [
        (
            "futu",
            "",
            "broker.holdings_ratio",
            "unsupported",
            Some("OpenD 10.9 documents this broker-holdings factor as unsupported"),
        ),
        (
            "futu",
            "US",
            "broker.holdings_ratio",
            "unsupported",
            Some("factor is unavailable in US"),
        ),
        (
            "futu",
            "SH",
            "option.stock_iv",
            "unsupported",
            Some("factor is unavailable in SH"),
        ),
        ("futu", "HK", "option.stock_iv", "available", None),
        ("futu", "US", "option.stock_iv", "available", None),
    ];

    for (broker, market, factor_key, availability, reason) in cases {
        let catalog = screen_catalog(broker, market).expect("catalog variant");
        let descriptor = factor(&catalog, factor_key);
        assert_eq!(
            descriptor["availability"], availability,
            "{broker}|{market}.{factor_key}"
        );
        match reason {
            Some(reason) => assert_eq!(descriptor["reason"], reason),
            None => assert!(descriptor.get("reason").is_none()),
        }
    }

    let catalog = screen_catalog("futu", "HK").expect("HK futu catalog");
    assert_eq!(
        factor(&catalog, "broker.holdings_ratio")["markets"],
        serde_json::json!(["HK"])
    );
    assert_eq!(
        factor(&catalog, "option.stock_iv")["markets"],
        serde_json::json!(["HK", "US"])
    );
}

/// Parity: go:452dea11:pkg/researchscreen/catalog_embedded_test.go:31
/// TestEmbeddedCatalogShapeAndSemantics
///
/// Go pins the embedded header, the nine-factor intersection, the akshare and
/// yfinance market labels, the role/unit/format semantics of the shared
/// factors, and that generated-only factors never appear in the embedded
/// catalog. Lookups match keys case-insensitively (`SIMPLE.CHANGE_PCT`).
#[test]
fn embedded_catalog_keeps_provider_intersection_roles_and_units() {
    for (broker, market) in [("yfinance", "US"), ("akshare", "CN")] {
        let catalog = screen_catalog(broker, market).expect("embedded catalog");
        assert_eq!(catalog["provider"], broker);
        assert_eq!(catalog["market"], market);
        for factor_key in ["basic.code", "basic.name", "basic.industry"] {
            assert_eq!(factor(&catalog, factor_key)["availability"], "available");
            assert!(factor(&catalog, factor_key).get("markets").is_none());
        }
        assert_eq!(
            factor(&catalog, "basic.code")["roles"],
            serde_json::json!(["column", "sort"])
        );
        assert_eq!(
            factor(&catalog, "basic.name")["roles"],
            serde_json::json!(["column"])
        );
        assert_eq!(
            factor(&catalog, "basic.industry")["roles"],
            serde_json::json!(["column"])
        );
        assert_eq!(factor(&catalog, "simple.price")["filter"], true);
        assert_eq!(factor(&catalog, "simple.price")["retrieve"], true);
        assert_eq!(factor(&catalog, "simple.price")["sort"], true);
        assert_eq!(factor(&catalog, "simple.price")["filterKind"], "interval");
        assert_eq!(factor(&catalog, "simple.price")["unit"], "currency");
        assert_eq!(factor(&catalog, "simple.price")["displayFormat"], "price");
        assert_eq!(
            factor(&catalog, "simple.price")["operators"],
            serde_json::json!(["between"])
        );
        assert_eq!(
            factor(&catalog, "SIMPLE.CHANGE_PCT")["displayFormat"],
            "percent"
        );
        assert_eq!(factor(&catalog, "SIMPLE.CHANGE_PCT")["unit"], "percent");
        assert_eq!(factor(&catalog, "simple.volume")["valueType"], "integer");
        assert_eq!(factor(&catalog, "simple.volume")["unit"], "shares");
        assert_eq!(
            factor(&catalog, "simple.volume")["displayFormat"],
            "integer"
        );
    }

    let akshare = screen_catalog("akshare", "").expect("akshare catalog");
    assert_eq!(akshare["version"], "embedded-stock-screen-v1");
    assert_eq!(akshare["provider"], "akshare");
    assert_eq!(akshare["querySchemaVersion"], 2);
    assert_eq!(akshare["factors"].as_array().expect("factors").len(), 9);
    assert_eq!(
        akshare["markets"],
        serde_json::json!(["SH", "SZ", "CN", "HK", "US"])
    );
    assert!(
        factor_keys(&akshare).all(|key| !key.eq_ignore_ascii_case("indicator.ma")),
        "generated-only factors must stay out of the embedded catalog"
    );

    let yfinance = screen_catalog("yfinance", "US").expect("yfinance catalog");
    assert_eq!(yfinance["market"], "US");
    assert_eq!(yfinance["markets"], serde_json::json!(["US"]));

    assert_eq!(
        screen_catalog("yfinance", "HK"),
        Err(ScreenCatalogError::UnsupportedEmbeddedMarket(
            "yfinance".into()
        ))
    );
}

/// Parity: go:452dea11:internal/assistant/assembly/application_adapter_test.go:226
/// `TestApplicationAdapterProvidesScreenCatalogAndCancelResult` (catalog half).
/// Go's screen-catalog helper trims and upper-cases the requested market, so a
/// padded lowercase label resolves to the same catalog as its canonical
/// spelling, while a market futu does not serve stays an error.
#[test]
fn screen_catalog_normalizes_padded_lowercase_markets_and_rejects_unsupported_labels() {
    let catalog = screen_catalog(" futu ", " us ").expect("padded lowercase market");
    assert_eq!(catalog["market"], "US");
    assert_eq!(catalog["provider"], "futu");
    assert_eq!(catalog["version"], "futu-stock-screen-v1");

    assert_eq!(
        screen_catalog("futu", " cn "),
        Err(ScreenCatalogError::UnsupportedFutuMarket)
    );
}

/// Parity: go:452dea11:pkg/researchscreen/catalog_test.go:97
/// TestFactorDisplaySemanticsAreExplicitAndCorrected
///
/// Go pins unit, currency basis and display format for ten corrected factors;
/// factors without a unit or basis keep empty strings.
#[test]
fn futu_factor_display_semantics_match_the_editor_table() {
    let catalog = screen_catalog("futu", "").expect("futu catalog");
    for (key, unit, currency_basis, display_format) in [
        ("simple.price", "currency", "quote", "price"),
        ("simple.market_cap", "currency", "quote", "compact_amount"),
        (
            "financial.net_profit",
            "currency",
            "reporting",
            "compact_amount",
        ),
        (
            "financial.float_market_cap",
            "currency",
            "quote",
            "compact_amount",
        ),
        ("financial.equity_multiplier", "", "", "number"),
        ("financial.money_turnover_cycle", "days", "", "integer"),
        (
            "financial.stockholder_profit_cagr",
            "percent",
            "",
            "percent",
        ),
        (
            "financial.surprise_revenue_date",
            "timestamp",
            "",
            "timestamp",
        ),
        ("featured.cash_flow_net_in_count", "count", "", "integer"),
        ("indicator.ma", "currency", "quote", "price"),
    ] {
        let descriptor = factor(&catalog, key);
        assert_eq!(
            descriptor["unit"].as_str().unwrap_or_default(),
            unit,
            "{key} unit"
        );
        assert_eq!(
            descriptor["currencyBasis"].as_str().unwrap_or_default(),
            currency_basis,
            "{key} currency basis"
        );
        assert_eq!(
            descriptor["displayFormat"].as_str().unwrap_or_default(),
            display_format,
            "{key} display format"
        );
    }
}

/// Parity: go:452dea11:pkg/researchscreen/catalog_edges_test.go:8
/// TestCatalogHelperContractsCoverEditorVariants
///
/// Go asserts the generation-time `conditionContract`,
/// `parameterEditorType` and `parameterHelp` mapping. Rust consumes the frozen
/// projection those helpers produce, so the reachable rows are asserted against
/// representative catalog entries: interval → range, enum → singleSelect, set →
/// multiSelect, interval_or_set → rangeOrSet, position → indicatorCompare and
/// pattern → pattern, plus the select/multiNumber/union/text/number editor
/// types and the window/cumulative/indicator/union help texts.
#[test]
fn catalog_condition_and_parameter_editor_variants_match_the_frozen_projection() {
    let catalog = screen_catalog("futu", "").expect("futu catalog");
    for (key, editor, value_enum) in [
        ("simple.price", "range", ""),
        ("field.market", "singleSelect", "market"),
        ("kline_shape.shape_type", "multiSelect", "kline_shape_type"),
        (
            "featured.cash_flow_net_in_count",
            "rangeOrSet",
            "cash_flow_period",
        ),
        ("indicator.ma", "indicatorCompare", ""),
        ("pattern.ma_long", "pattern", ""),
    ] {
        let descriptor = factor(&catalog, key);
        assert_eq!(
            descriptor["conditionEditor"], editor,
            "{key} condition editor"
        );
        assert_eq!(
            descriptor["valueEnum"].as_str().unwrap_or_default(),
            value_enum,
            "{key} value enum"
        );
    }
    assert!(
        factor(&catalog, "basic.name")
            .get("conditionEditor")
            .is_none(),
        "non-filter factors publish no condition editor"
    );

    for (factor_key, name, editor) in [
        ("cumulative.price_change_pct", "days", "number"),
        ("indicator.ma", "period", "select"),
        ("indicator.ma", "indicatorParams", "multiNumber"),
        ("option.stock_iv", "optionParam", "union"),
        ("broker.holdings_ratio", "brokerParam", "text"),
    ] {
        assert_eq!(
            parameter(&catalog, factor_key, name)["editorType"],
            editor,
            "{factor_key}.{name} editor"
        );
    }
    for (name, fragment) in [
        ("days", "统计窗口"),
        ("duration", "累计周期"),
        ("indicatorParams", "指标专用参数"),
        ("optionParam", "期权参数联合类型"),
        ("periodAverage", "可选参数"),
    ] {
        let help = parameter(&catalog, help_factor(name), name)["help"]
            .as_str()
            .unwrap_or_default()
            .to_owned();
        assert!(
            help.contains(fragment),
            "parameterHelp({name}) = {help:?} must contain {fragment:?}"
        );
    }
    assert_eq!(
        parameter(&catalog, "indicator.ma", "period")["help"],
        "从目录枚举中选择",
        "enum parameters publish the enum help text"
    );
}

fn help_factor(parameter_name: &str) -> &'static str {
    match parameter_name {
        "days" => "cumulative.price_change_pct",
        "duration" => "financial.net_profit",
        "indicatorParams" => "indicator.ma",
        "optionParam" => "option.stock_iv",
        _ => "financial.net_profit",
    }
}
