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

/// Parity: go:452dea11:pkg/futu/adapter_advanced_test.go:14
/// TestAdvancedFeatureDefaultsBuildStrictOpenDRequests
///
/// Go builds the strict C2S payload for six advanced protocols and asserts the
/// translated fields exist while `count` never appears on a protocol that does
/// not declare it. Rust owns that translation in `research_params`; the typed
/// readers (`OptionChainQuery`, `OptionScreenQuery`, `FutuNewsQuery`) then
/// re-validate the translated values before any RPC.
#[test]
fn advanced_feature_defaults_build_strict_opend_requests() {
    struct Case {
        name: &'static str,
        protocol: &'static str,
        market: &'static str,
        instrument: &'static str,
        page_size: i32,
        want: &'static [&'static str],
        absent: &'static [&'static str],
    }
    let cases = [
        Case {
            name: "option chain",
            protocol: "Qot_GetOptionChain",
            market: "US",
            instrument: "US.AAPL",
            page_size: 50,
            want: &["beginTime", "endTime"],
            absent: &["count"],
        },
        Case {
            name: "option screen",
            protocol: "Qot_OptionScreen",
            market: "US",
            instrument: "",
            page_size: 50,
            want: &["marketCategoryList", "pageCount"],
            absent: &["count"],
        },
        Case {
            name: "warrant list",
            protocol: "Qot_GetWarrant",
            market: "HK",
            instrument: "HK.00700",
            page_size: 50,
            want: &["begin", "num", "sortField", "ascend"],
            absent: &["count"],
        },
        Case {
            name: "top movers",
            protocol: "Qot_GetTopMoversRank",
            market: "US",
            instrument: "",
            page_size: 50,
            want: &["count"],
            absent: &[],
        },
        Case {
            name: "macro",
            protocol: "Qot_GetMacroIndicatorList",
            market: "US",
            instrument: "",
            page_size: 50,
            want: &["region"],
            absent: &["count"],
        },
        Case {
            name: "news",
            protocol: "Qot_GetSearchNews",
            market: "US",
            instrument: "US.AAPL",
            page_size: 30,
            want: &["keyword"],
            absent: &["count"],
        },
    ];
    for case in cases {
        let mut params = params_from_pairs([]);
        inject_advanced_page_size(&mut params, case.protocol, case.page_size);
        let scope = ResearchQueryScope::new(case.market, case.instrument);
        // Go's `injectAdvancedDefaults` owns the shared scope defaults and then
        // delegates to `injectAdvancedProtocolDefaults`, so both rule sets are
        // applied from one entry point.
        inject_advanced_defaults(&mut params, case.protocol, &scope)
            .unwrap_or_else(|error| panic!("{} defaults: {error}", case.name));
        for field in case.want {
            assert!(
                params.get(*field).is_some_and(|value| !value.is_null()),
                "{} is missing {field}: {params:?}",
                case.name
            );
        }
        for field in case.absent {
            assert!(
                !params.contains_key(*field),
                "{} unexpectedly declared {field}: {params:?}",
                case.name
            );
        }
        // A protocol that declares no pagination field must never receive one,
        // and a declared field is clamped to the adapter limit.
        if let Some(field) = advanced_page_size_field(case.protocol) {
            let value = params[field].as_i64().expect("numeric page size");
            assert_eq!(
                value,
                i64::from(case.page_size),
                "{} page size was not preserved",
                case.name
            );
        }
    }

    // The translated values keep the exact OpenD shapes the typed readers
    // validate and send: option-chain dates, HK/US market categories, warrant
    // minimums and the news keyword derived from the instrument id.
    let mut chain = params_from_pairs([]);
    inject_advanced_option_defaults(
        &mut chain,
        "Qot_GetOptionChain",
        &ResearchQueryScope::new("US", ""),
    )
    .expect("option chain dates");
    for field in ["beginTime", "endTime"] {
        let value = chain[field].as_str().expect("date text");
        assert_eq!(value.len(), 10, "{field} must be YYYY-MM-DD: {value}");
    }
    for (market, want) in [("US", 0), ("HK", 3)] {
        let mut screen = params_from_pairs([]);
        inject_advanced_option_defaults(
            &mut screen,
            "Qot_OptionScreen",
            &ResearchQueryScope::new(market, ""),
        )
        .expect("option screen category");
        assert_eq!(screen["marketCategoryList"], json!([want]));
        screen.remove("marketCategoryList");
    }
    let mut warrant = params_from_pairs([]);
    inject_advanced_option_defaults(
        &mut warrant,
        "Qot_GetWarrant",
        &ResearchQueryScope::new("HK", "HK.00700"),
    )
    .expect("warrant defaults");
    assert_eq!(warrant["begin"], json!(0));
    assert_eq!(warrant["sortField"], json!(12));
    assert_eq!(warrant["ascend"], json!(false));
    let mut news = params_from_pairs([]);
    inject_advanced_research_defaults(
        &mut news,
        "Qot_GetSearchNews",
        &ResearchQueryScope::new("US", "US.AAPL"),
    )
    .expect("news keyword");
    assert_eq!(news["keyword"], json!("AAPL"));
}

/// Parity: go:452dea11:pkg/futu/adapter_advanced_test.go:80
/// TestIndustrialChainListPageSizeRespectsOpenDLimit
///
/// Go clamps every requested page size into `[1, advancedPageSizeLimit]`
/// before injecting it into the protocol's declared pagination field.
/// `Qot_GetIndustrialChainList` is the one protocol whose OpenD `count` range
/// is `[1, 50]`; everything else stops at 100.
#[test]
fn advanced_page_size_respects_protocol_limits() {
    let mut params = params_from_pairs([("market", json!(11))]);
    inject_advanced_page_size(&mut params, "Qot_GetIndustrialChainList", 100);
    assert_eq!(params["count"], json!(50));
    assert_eq!(advanced_page_size_limit("Qot_GetIndustrialChainList"), 50);

    // The lower bound and the default cap both come from Go's `min`/`max`.
    for (requested, want) in [(0, 1), (-5, 1), (1, 1), (100, 100), (1000, 100)] {
        assert_eq!(
            clamp_advanced_page_size("Qot_GetTopMoversRank", requested),
            want,
            "top movers page size {requested}"
        );
    }
    assert_eq!(
        clamp_advanced_page_size("Qot_GetIndustrialChainList", 1000),
        50
    );

    // A protocol that declares no pagination field must not receive one, and a
    // caller-owned value is never overwritten.
    let mut chain_only = params_from_pairs([]);
    inject_advanced_page_size(&mut chain_only, "Qot_GetMacroIndicatorList", 100);
    assert!(chain_only.is_empty(), "{chain_only:?}");
    let mut owned = params_from_pairs([("count", json!(7))]);
    inject_advanced_page_size(&mut owned, "Qot_GetTopMoversRank", 100);
    assert_eq!(owned["count"], json!(7));

    // Every declared field name matches the generated C2S payload: the values
    // here are read straight from `proto/futu`, so a drift in the probe order
    // or in a protocol's declared field fails this table.
    assert_eq!(
        advanced_page_size_field("Qot_OptionScreen"),
        Some("pageCount")
    );
    assert_eq!(advanced_page_size_field("Qot_GetWarrant"), Some("num"));
    assert_eq!(
        advanced_page_size_field("Qot_GetEventContractKline"),
        Some("maxCount")
    );
    assert_eq!(advanced_page_size_field("Qot_GetTicker"), Some("maxRetNum"));
    assert_eq!(
        advanced_page_size_field("Qot_GetIndustrialChainList"),
        Some("count")
    );
    assert_eq!(advanced_page_size_field("Qot_GetMacroIndicatorList"), None);
    assert_eq!(advanced_page_size_field("Qot_GetOptionChain"), None);
}

/// Parity: go:452dea11:pkg/futu/adapter_advanced_test.go:124
/// TestAdvancedProtocolReplaySafetyDefaultsToNoReplay
///
/// Rust keeps no generic "advanced protocol" dispatcher, so the replay policy
/// itself is the shared owner: only read protocols may be retried after a
/// reconnect, and a mutation-shaped protocol fails closed by default.
#[test]
fn advanced_protocol_replay_safety_defaults_to_no_replay() {
    for protocol in [
        "Qot_SetPriceReminder",
        "Qot_SetOptionEventAlert",
        "Qot_ModifyUserSecurity",
        "Qot_GetEventContractComboRfq",
        "Qot_UnknownMutation",
    ] {
        assert!(
            !advanced_protocol_replay_safe(protocol),
            "{protocol} must not be replayed automatically"
        );
    }
    for protocol in [
        "Qot_GetOptionChain",
        "Qot_RequestTradeDate",
        "Qot_FilterCompetition",
        "Qot_OptionScreen",
        "Qot_SubEventContract",
        "Qot_WarrantScreen",
        "Qot_StockFilter",
        "Qot_StockScreen",
    ] {
        assert!(
            advanced_protocol_replay_safe(protocol),
            "{protocol} is a replay-safe read"
        );
    }
    // The RFQ keeps its `Get` prefix but creates a short-lived quote, so the
    // prefix rule must not be the only gate.
    assert!(advanced_protocol_replay_safe(
        "Qot_GetEventContractComboList"
    ));
}

/// Parity: go:452dea11:pkg/futu/adapter_advanced_protocol_test.go:149
/// TestFutuAdvancedAdapterProtocolValidationDefaultsAndPayloadHelpers
///
/// Go's shared scope defaults are `market`/`offset`/`pageFrom`, applied only
/// when the protocol's C2S payload declares the field, plus the cursor field
/// probe order (`nextPage`, `page`, `nextKey`). Rust owns all three in
/// `research_params`, so the same table is asserted directly.
#[test]
fn advanced_scope_defaults_and_cursor_injection_follow_the_protocol_payload() {
    // Market translation is scoped to protocols that declare `market`.
    let mut top_movers = params_from_pairs([]);
    inject_advanced_defaults(
        &mut top_movers,
        "Qot_GetTopMoversRank",
        &ResearchQueryScope::new("US", ""),
    )
    .expect("top movers defaults");
    assert_eq!(top_movers["market"], json!(11));
    assert_eq!(top_movers["offset"], json!(0));

    let mut hk = params_from_pairs([]);
    inject_advanced_defaults(
        &mut hk,
        "Qot_GetIndustrialChainList",
        &ResearchQueryScope::new("HK", ""),
    )
    .expect("industrial chain defaults");
    assert_eq!(hk["market"], json!(1));

    // A protocol without a `market` field is never given one.
    let mut warrant = params_from_pairs([]);
    inject_advanced_defaults(
        &mut warrant,
        "Qot_GetWarrant",
        &ResearchQueryScope::new("HK", ""),
    )
    .expect("warrant defaults");
    assert!(!warrant.contains_key("market"));
    assert!(!warrant.contains_key("offset"));

    // `pageFrom` is only defaulted for protocols that declare it.
    let mut screen = params_from_pairs([]);
    inject_advanced_defaults(
        &mut screen,
        "Qot_OptionScreen",
        &ResearchQueryScope::new("US", ""),
    )
    .expect("option screen defaults");
    assert_eq!(screen["pageFrom"], json!(0));
    let mut chain = params_from_pairs([]);
    inject_advanced_defaults(
        &mut chain,
        "Qot_GetIndustrialChainList",
        &ResearchQueryScope::new("HK", ""),
    )
    .expect("industrial chain defaults");
    assert!(!chain.contains_key("pageFrom"));

    // An unsupported market only fails when one was actually supplied.
    let error = inject_advanced_defaults(
        &mut params_from_pairs([]),
        "Qot_GetTopMoversRank",
        &ResearchQueryScope::new("XX", ""),
    )
    .expect_err("unsupported market");
    assert!(error.message().contains("unsupported market"), "{error}");
    inject_advanced_defaults(
        &mut params_from_pairs([]),
        "Qot_GetTopMoversRank",
        &ResearchQueryScope::new("", ""),
    )
    .expect("empty market keeps the historic unknown code");

    // Cursor probe order: `nextPage` before `page` before `nextKey`.
    for (protocol, field) in [
        ("Qot_GetEventContractComboList", "nextPage"),
        ("Qot_GetInstitutionList", "page"),
        ("Qot_GetShortInterest", "nextKey"),
    ] {
        let mut params = params_from_pairs([]);
        inject_advanced_cursor(&mut params, protocol, "cursor-2");
        assert_eq!(params[field], json!("cursor-2"), "{protocol}");
    }
    let mut none = params_from_pairs([]);
    inject_advanced_cursor(&mut none, "Qot_GetOptionChain", "cursor-2");
    assert!(none.is_empty(), "{none:?}");
    let mut owned = params_from_pairs([("nextPage", json!("caller"))]);
    inject_advanced_cursor(&mut owned, "Qot_GetEventContractComboList", "cursor-2");
    assert_eq!(owned["nextPage"], json!("caller"));
    let mut blank = params_from_pairs([]);
    inject_advanced_cursor(&mut blank, "Qot_GetEventContractComboList", "   ");
    assert!(blank.is_empty(), "{blank:?}");
}
