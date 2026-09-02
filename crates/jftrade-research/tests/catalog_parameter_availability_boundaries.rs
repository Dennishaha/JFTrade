use jftrade_research::{ScreenCatalogError, screen_catalog};
use serde_json::Value;

fn factor<'a>(catalog: &'a Value, key: &str) -> &'a Value {
    catalog["factors"]
        .as_array()
        .expect("catalog factors")
        .iter()
        .find(|candidate| candidate["key"].as_str() == Some(key))
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
        assert_eq!(factor(&catalog, "simple.price")["unit"], "currency");
        assert_eq!(factor(&catalog, "simple.price")["displayFormat"], "price");
        assert_eq!(factor(&catalog, "simple.volume")["valueType"], "integer");
        assert_eq!(factor(&catalog, "simple.volume")["unit"], "shares");
        assert_eq!(
            factor(&catalog, "simple.volume")["displayFormat"],
            "integer"
        );
    }

    assert_eq!(
        screen_catalog("yfinance", "HK"),
        Err(ScreenCatalogError::UnsupportedEmbeddedMarket(
            "yfinance".into()
        ))
    );
}
