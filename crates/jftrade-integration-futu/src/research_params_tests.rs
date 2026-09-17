use super::*;
use serde_json::json;

fn params(pairs: &[(&'static str, Value)]) -> Map<String, Value> {
    params_from_pairs(pairs.iter().map(|(key, value)| (*key, value.clone())))
}

fn scope(market: &str) -> ResearchQueryScope {
    ResearchQueryScope::new(market, "")
}

#[test]
fn advanced_research_defaults_reject_incomplete_queries() {
    // Parity: go:pkg/futu/adapter_research_normalization_boundaries_test.go:16
    let cases: Vec<(&str, &str, Value, &str)> = vec![
        (
            "plate list market",
            "Qot_GetPlateSet",
            json!({"plateType": "industry"}),
            "",
        ),
        (
            "plate members instrument",
            "Qot_GetPlateSecurity",
            json!({}),
            "",
        ),
        ("fund catalog market", "Qot_GetStaticInfo", json!({}), ""),
        ("economic date", "Qot_GetEconomicCalendar", json!({}), ""),
        (
            "economic market",
            "Qot_GetEconomicCalendar",
            json!({"beginDate": "2026-07-23"}),
            "XX",
        ),
        ("dividend date", "Qot_GetDividendCalendar", json!({}), ""),
        (
            "institution id",
            "Qot_GetInstitutionProfile",
            json!({"institutionId": 1.5}),
            "",
        ),
    ];
    for (name, protocol, raw, market) in cases {
        let mut params = raw.as_object().expect("object").clone();
        let scope = scope(market);
        let error =
            inject_advanced_research_defaults(&mut params, protocol, &scope).expect_err(name);
        assert!(!error.message().is_empty(), "{name}");
    }
}

#[test]
fn advanced_research_defaults_translate_public_inputs() {
    // Parity: go:pkg/futu/adapter_research_normalization_boundaries_test.go:54
    let mut plate = params(&[("market", json!(11)), ("plateType", json!("region"))]);
    inject_advanced_research_defaults(&mut plate, "Qot_GetPlateSet", &scope("US"))
        .expect("plate defaults");
    assert_eq!(plate["plateSetType"], json!(2));
    assert!(!plate.contains_key("plateType"));

    let mut economic = params(&[("beginDate", json!("2026-07-23"))]);
    inject_advanced_research_defaults(&mut economic, "Qot_GetEconomicCalendar", &scope("HK"))
        .expect("economic defaults");
    assert_eq!(economic["marketList"], json!([1]));

    let mut institution = params(&[("institutionId", json!(7.0))]);
    inject_advanced_research_defaults(
        &mut institution,
        "Qot_GetInstitutionHoldingList",
        &scope(""),
    )
    .expect("institution defaults");
    assert_eq!(institution["institutionId"], json!(7));

    let mut news = params(&[]);
    inject_advanced_research_defaults(
        &mut news,
        "Qot_GetSearchNews",
        &ResearchQueryScope::new("US", "US.AAPL"),
    )
    .expect("news defaults");
    assert_eq!(news["keyword"], json!("AAPL"));

    let mut unknown = params(&[]);
    inject_advanced_research_defaults(&mut unknown, "unknown", &scope(""))
        .expect("unknown protocol defaults");
}

#[test]
fn advanced_research_enum_translations_match_go_bounds() {
    // Parity: go:pkg/futu/adapter_research_normalization_boundaries_test.go:96
    for (raw, want) in [("", None), ("up", Some(0)), ("down", Some(1))] {
        let mut params = params(&[("direction", json!(raw))]);
        translate_top_movers_direction(&mut params).expect("top mover");
        match want {
            Some(want) => assert_eq!(params["sortDir"], json!(want)),
            None => assert!(!params.contains_key("sortDir")),
        }
        assert!(!params.contains_key("direction"));
    }
    for direction in ["gainers", "losers", "ascending", "descending", "sideways"] {
        let mut params = params(&[("direction", json!(direction))]);
        assert!(
            translate_top_movers_direction(&mut params).is_err(),
            "{direction}"
        );
    }

    for (raw, want) in [
        (json!(2.0), 2),
        (json!("industry"), 0),
        (json!("concept"), 1),
        (json!("theme"), 2),
    ] {
        let mut params = params(&[("plateType", raw)]);
        translate_heat_map_plate_type(&mut params).expect("heatmap");
        assert_eq!(params["plateType"], json!(want));
    }
    translate_heat_map_plate_type(&mut params_from_pairs([])).expect("missing heatmap type");
    for raw in [json!(3.0), json!("unsupported")] {
        let mut params = params(&[("plateType", raw)]);
        assert!(translate_heat_map_plate_type(&mut params).is_err());
    }

    for (raw, want) in [
        (json!(3.0), 3),
        (json!("all"), 0),
        (json!("industry"), 1),
        (json!("region"), 2),
        (json!("concept"), 3),
    ] {
        let mut params = params(&[("plateSetType", raw)]);
        translate_plate_set_type(&mut params).expect("plate set");
        assert_eq!(params["plateSetType"], json!(want));
    }
    for params in [
        params_from_pairs([]),
        params(&[("plateSetType", json!(f64::NAN))]),
        params(&[("plateSetType", json!("unsupported"))]),
    ] {
        let mut params = params;
        assert!(translate_plate_set_type(&mut params).is_err());
    }
    assert_eq!(bounded_research_enum(&json!(1.5), 0, 3), None);
}

#[test]
fn research_number_accepts_go_supported_scalar_types() {
    // Parity: go:pkg/futu/adapter_research_normalization_boundaries_test.go:284
    for value in [json!(1.0), json!(1), json!(" 1 ")] {
        assert_eq!(research_number_parity(&value), Some(1.0), "{value}");
    }
    for value in [json!(u64::MAX), json!(i64::MAX), json!(1)] {
        assert!(research_number_parity(&value).is_some(), "{value}");
    }
    assert_eq!(research_number_parity(&json!(true)), None);
}

#[test]
fn high_dividend_state_is_hk_only() {
    // Parity: go:pkg/futu/adapter_advanced_defaults.go:63 (Go
    // TestAdvancedResearchDefaultsRejectIncompleteQueries covers the guard).
    let mut params = params_from_pairs([]);
    assert!(
        inject_advanced_option_defaults(&mut params, "Qot_GetHighDividendSOERank", &scope("SH"))
            .is_err()
    );
    inject_advanced_option_defaults(&mut params, "Qot_GetHighDividendSOERank", &scope("hk"))
        .expect("HK high dividend");
}

#[test]
fn top_movers_leaves_sort_dir_untouched_when_combo_provides_it() {
    let mut params = params(&[("direction", json!("UP")), ("sortDir", json!(5))]);
    translate_top_movers_direction(&mut params).expect("translation");
    assert_eq!(params["sortDir"], json!(0));
}
