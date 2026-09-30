use super::*;
use crate::research_params::ResearchQueryScope;
use crate::research_params::{
    inject_advanced_cursor, inject_advanced_defaults, inject_advanced_page_size,
    translate_top_movers_direction,
};
use serde_json::{Map, json};

#[test]
// Parity: go:452dea11:pkg/futu/adapter_research_normalization_boundaries_test.go:178 TestResearchNormalizationCoversAlternateWireShapes
fn alternate_wire_shapes_keep_scalars_and_add_aliases() {
    // Parity: go:pkg/futu/adapter_research_normalization_boundaries_test.go:178
    let payload = json!({
        "list": [
            "preserve",
            {"marketVal": 100.0, "dividendYieldTTM": 2.0}
        ]
    });
    let normalized = normalize_research_protocol_payload("Qot_GetHighDividendSOERank", &payload);
    let values = normalized["list"].as_array().expect("list");
    assert_eq!(values[0], json!("preserve"));
    let row = values[1].as_object().expect("row");
    assert_eq!(row["marketValue"], json!(100.0));
    assert_eq!(row["dividendYield"], json!(2.0));
    assert_eq!(row["marketVal"], json!(100.0));

    let entries = vec![json!({"id": 1}), json!({"id": 2})];
    let page = apply_research_local_pagination(&entries, 1, "local:99", "Qot_GetPlateSet")
        .expect("out-of-range cursor");
    assert!(page.entries.is_empty());
    assert_eq!(page.total, 2);
    assert!(!page.has_more);
    assert_eq!(page.next_cursor, "");
}

#[test]
// Parity: go:452dea11:pkg/futu/adapter_research_normalization_boundaries_test.go:208 TestResearchNormalizationCoversProductAndCalendarVariants
// Parity: go:452dea11:pkg/futu/advanced_product_adapter_contracts_test.go:336 TestFutuSnapshotProductExtensionsAndSecurityTypeMapping
fn product_and_calendar_variants_match_go_projection() {
    // Parity: go:pkg/futu/adapter_research_normalization_boundaries_test.go:208
    for (value, want) in [
        (json!("eqty"), "equity"),
        (json!("trust"), "fund"),
        (json!("drvt"), "option"),
        (json!("bwrt"), "warrant"),
        (json!("future"), "future"),
        (json!(1), "bond"),
    ] {
        assert_eq!(research_security_type(&value), Some(want), "{value}");
    }
    assert_eq!(research_security_type(&json!({"unsupported": true})), None);

    let empty = Map::new();
    assert_eq!(
        research_product_class("Qot_GetStaticInfo", &empty, "", &empty).as_deref(),
        Some("fund")
    );
    let explicit = Map::from_iter([("productClass".to_owned(), json!("OPTION"))]);
    assert_eq!(
        research_product_class("Qot_GetStaticInfo", &explicit, "", &empty).as_deref(),
        Some("option")
    );

    let mut ipo = Map::from_iter([
        ("listPrice".to_owned(), json!(10.0)),
        ("ipoPriceMin".to_owned(), json!(9.0)),
        ("ipoPriceMax".to_owned(), json!(11.0)),
        ("listTime".to_owned(), json!("2026-07-23")),
        ("issueSize".to_owned(), json!(1000)),
        ("listTimestamp".to_owned(), json!(1_753_228_800.0)),
    ]);
    flatten_research_ipo(&mut ipo);
    assert_eq!(ipo["issuePrice"], json!(10.0));
    assert_eq!(ipo["listingDate"], json!("2026-07-23"));
    assert_eq!(ipo["issueVolume"], json!(1000));

    let mut ipo_price = Map::from_iter([("ipoPrice".to_owned(), json!(12.0))]);
    flatten_research_ipo(&mut ipo_price);
    assert_eq!(ipo_price["issuePrice"], json!(12.0));

    let mut earnings = Map::from_iter([
        ("earningsDate".to_owned(), json!("2026-07-25")),
        ("earningsTimestamp".to_owned(), json!(1_753_401_600.0)),
    ]);
    normalize_research_calendar_fields("Qot_GetEarningsCalendar", &mut earnings);
    assert_eq!(earnings["calendarType"], json!("earnings"));
    assert_eq!(earnings["eventDate"], json!("2026-07-25"));

    normalize_research_calendar_fields(
        "Qot_GetDividendCalendar",
        &mut Map::from_iter([("exDate".to_owned(), json!("2026-07-24"))]),
    );

    let mut institution = Map::from_iter([
        ("institutionId".to_owned(), json!(7)),
        ("positionValueChange".to_owned(), json!(2.0)),
        ("positionCountChange".to_owned(), json!(3)),
    ]);
    normalize_research_institution_fields("Qot_GetInstitutionHoldingChange", &mut institution);
    assert_eq!(institution["marketValueChange"], json!(2.0));
    assert_eq!(institution["holdingCountChange"], json!(3));
}

#[test]
fn local_pagination_walks_plate_and_catalog_windows() {
    let entries: Vec<Value> = (0..5).map(|index| json!({"row": index})).collect();
    for protocol in [
        "Qot_GetPlateSet",
        "Qot_GetPlateSecurity",
        "Qot_GetStaticInfo",
    ] {
        let first = apply_research_local_pagination(&entries, 2, "", protocol).expect("first");
        assert_eq!(first.entries.len(), 2);
        assert_eq!(first.next_cursor, "local:2");
        assert!(first.has_more);
        assert_eq!(first.total, 5);

        let second = apply_research_local_pagination(&entries, 2, &first.next_cursor, protocol)
            .expect("second");
        assert_eq!(second.entries[0], json!({"row": 2}));
        assert_eq!(second.next_cursor, "local:4");

        let last = apply_research_local_pagination(&entries, 2, &second.next_cursor, protocol)
            .expect("last");
        assert_eq!(last.entries.len(), 1);
        assert!(!last.has_more);
        assert_eq!(last.next_cursor, "");
    }
    assert!(
        apply_research_local_pagination(&entries, 2, "server-cursor", "Qot_GetStaticInfo").is_err()
    );
    assert!(apply_research_local_pagination(&entries, 2, "local:-1", "Qot_GetStaticInfo").is_err());
}

#[test]
fn pagination_only_payloads_and_unknown_protocols_are_returned_unchanged() {
    let metadata = json!({"allCount": 17, "currency": "USD"});
    assert_eq!(
        normalize_research_protocol_payload("Qot_GetTopMoversRank", &metadata),
        metadata
    );
    let unknown = json!({"marketVal": 1.0});
    assert_eq!(
        normalize_research_protocol_payload("Qot_UnknownProtocol", &unknown),
        unknown
    );
}

#[test]
fn wire_security_rows_gain_canonical_fields_without_losing_opend_fields() {
    // Parity: go:452dea11:pkg/futu/adapter_research_contract_test.go:115
    // TestResearchProtocolPayloadAddsCanonicalFieldsWithoutDroppingOpenDFields.
    //
    // Go runs `normalizeOpenDMap` before the projection, so a raw row carrying
    // `{market: "QotMarket_US_Security", code}` arrives at
    // `researchEntrySecurity` with an `instrumentId` already derived. Each
    // sub-test below pins one of Go's five shapes: ranking rows, plate rows,
    // fund static info, IPO rows, and the calendar/institution projections.
    let ranking = normalize_research_protocol_payload(
        "Qot_GetTopMoversRank",
        &json!({
            "dataList": [{
                "security": {"market": "QotMarket_US_Security", "code": "aapl"},
                "name": "Apple", "curPrice": 210.0, "changeRatio": 2.5,
                "dividendYieldTTM": 0.55,
            }],
            "allCount": 17,
        }),
    );
    let entry = &ranking["dataList"][0];
    assert_eq!(entry["instrumentId"], json!("US.AAPL"));
    assert_eq!(entry["market"], json!("US"));
    assert_eq!(entry["symbol"], json!("AAPL"));
    assert_eq!(entry["name"], json!("Apple"));
    assert_eq!(entry["productClass"], json!("equity"));
    assert_eq!(entry["changeRate"], json!(2.5));
    assert_eq!(entry["changeRatio"], json!(2.5));
    assert_eq!(entry["price"], json!(210.0));
    assert_eq!(entry["dividendYield"], json!(0.55));
    // The OpenD fields survive the projection.
    assert_eq!(entry["curPrice"], json!(210.0));
    assert_eq!(ranking["allCount"], json!(17));

    let plate = normalize_research_protocol_payload(
        "Qot_GetPlateSet",
        &json!({
            "plateInfoList": [{
                "plate": {"market": "QotMarket_HK_Security", "code": "bk1001"},
                "name": "Semiconductors", "plateType": 1,
            }],
        }),
    );
    let entry = &plate["plateInfoList"][0];
    assert_eq!(entry["instrumentId"], json!("HK.BK1001"));
    assert_eq!(entry["symbol"], json!("BK1001"));
    assert_eq!(entry["name"], json!("Semiconductors"));
    assert_eq!(entry["productClass"], json!("plate"));
    assert_eq!(entry["plateType"], json!(1), "original plate field dropped");
    assert_eq!(entry["plate"]["code"], json!("bk1001"));

    let fund = normalize_research_protocol_payload(
        "Qot_GetStaticInfo",
        &json!({
            "staticInfoList": [{"basic": {
                "security": {"market": "QotMarket_US_Security", "code": "spy"},
                "name": "SPDR S&P 500 ETF", "secType": 4, "lotSize": 1,
            }}],
        }),
    );
    let entry = &fund["staticInfoList"][0];
    assert_eq!(entry["instrumentId"], json!("US.SPY"));
    assert_eq!(entry["symbol"], json!("SPY"));
    assert_eq!(entry["productClass"], json!("fund"));
    assert!(
        entry["basic"].is_object(),
        "original static basic was dropped: {entry}"
    );

    let ipo = normalize_research_protocol_payload(
        "Qot_GetIpoList",
        &json!({
            "ipoList": [{
                "basic": {
                    "security": {"market": "QotMarket_US_Security", "code": "ipox"},
                    "name": "Example IPO", "listTime": "2026-08-01",
                },
                "usExData": {
                    "ipoPriceMin": 10.0, "ipoPriceMax": 12.0, "issueSize": 5_000_000.0,
                },
            }],
        }),
    );
    let entry = &ipo["ipoList"][0];
    assert_eq!(entry["instrumentId"], json!("US.IPOX"));
    assert_eq!(entry["issuePriceMin"], json!(10.0));
    assert_eq!(entry["issuePriceMax"], json!(12.0));
    assert_eq!(entry["issueVolume"], json!(5_000_000.0));
    assert_eq!(entry["listingDate"], json!("2026-08-01"));
    assert_eq!(entry["eventDate"], json!("2026-08-01"));
    assert_eq!(entry["calendarType"], json!("ipo"));
    assert!(
        entry["usExData"].is_object(),
        "usExData was dropped: {entry}"
    );

    // Numeric `Qot_Common.QotMarket` codes resolve through the same table.
    let numeric = normalize_research_protocol_payload(
        "Qot_GetTopMovers",
        &json!({"security": {"market": 11, "code": "msft"}, "name": "MSFT"}),
    );
    assert_eq!(numeric["instrumentId"], json!("US.MSFT"));
    assert_eq!(numeric["symbol"], json!("MSFT"));

    // An unresolvable market keeps the row untouched rather than fabricating
    // an identity, matching Go's `normalizeOpenDSecurity` fallthrough.
    let unknown = normalize_research_protocol_payload(
        "Qot_GetTopMovers",
        &json!({"security": {"market": "QotMarket_Unknown", "code": "x"}, "name": "X"}),
    );
    assert!(unknown.get("instrumentId").is_none(), "{unknown}");
}

#[test]
fn local_pagination_never_leaks_into_the_opend_request() {
    // Parity: go:452dea11:pkg/futu/adapter_research_contract_test.go:237
    // TestResearchCatalogLocalPaginationKeepsOpenDRequestUnchanged.
    //
    // Go asserts that injecting a client-side cursor and page size for the
    // three locally paginated catalog protocols leaves the OpenD request map
    // completely empty; those parameters are consumed by
    // `applyResearchLocalPagination` after the response arrives.
    for protocol in [
        "Qot_GetPlateSet",
        "Qot_GetPlateSecurity",
        "Qot_GetStaticInfo",
    ] {
        let mut params = Map::new();
        inject_advanced_cursor(&mut params, protocol, "local:2");
        inject_advanced_page_size(&mut params, protocol, 2);
        assert!(
            params.is_empty(),
            "local pagination leaked into {protocol} request: {params:?}"
        );
    }
}

#[test]
fn open_d_enum_text_is_stripped_like_go() {
    // Parity: go:452dea11:pkg/futu/adapter_advanced_normalization.go
    // (`normalizeOpenDEnum`). The projection consumes enum text such as
    // `QotMarket_US_Security`; the prefix table is what makes the market
    // resolvable at all.
    for (raw, want) in [
        ("QotMarket_US_Security", "us_security"),
        ("SecurityType_Eqty", "eqty"),
        ("KLType_Day", "day"),
        ("EC_Series_List", "list"),
        ("unchanged", "unchanged"),
    ] {
        assert_eq!(normalize_open_d_enum(raw), want, "{raw}");
    }
}

#[test]
fn economic_calendar_pagination_honors_explicit_has_more_and_empty_rows() {
    // Parity: go:452dea11:pkg/futu/adapter_research_contract_test.go:314
    // TestEconomicCalendarPaginationHonorsExplicitHasMoreAndEmptyRows.
    //
    // Go asserts that a payload whose only list is empty still reports
    // `hasMore=false`, drops the stale cursor, keeps `nextPage`/`hasMore` in
    // the metadata, and never turns pagination metadata into an event row.
    let payload = json!({
        "nextPage": "stale-next-page",
        "hasMore": false,
    });
    let envelope = research_payload_envelope(&payload);
    assert!(
        envelope.entries.is_empty(),
        "pagination metadata became an event: {:?}",
        envelope.entries
    );
    assert!(!envelope.has_more);
    assert_eq!(envelope.next_cursor, "");
    assert_eq!(envelope.total, 0);

    // With real rows the explicit `hasMore=false` still clears the cursor.
    let with_rows = json!({
        "itemList": [{"title": "CPI", "timestamp": 1_784_764_800.0, "country": "US", "star": 3.0}],
        "nextPage": "stale-next-page",
        "hasMore": false,
    });
    let envelope = research_payload_envelope(&with_rows);
    assert_eq!(envelope.entries.len(), 1);
    assert!(!envelope.has_more);
    assert_eq!(envelope.next_cursor, "");
    assert_eq!(envelope.total, 1);

    // An explicit `hasMore=true` without a cursor is reported as-is, and the
    // cursor defaults to `nextPage` when `hasMore` is absent.
    let explicit_true = research_payload_envelope(&json!({"itemList": [], "hasMore": true}));
    assert!(explicit_true.has_more);
    assert_eq!(explicit_true.next_cursor, "");

    let derived =
        research_payload_envelope(&json!({"itemList": [], "nextPage": "page-2", "allCount": 17}));
    assert!(derived.has_more);
    assert_eq!(derived.next_cursor, "page-2");
    assert_eq!(derived.total, 17, "allCount wins over the entry count");

    // `nextKey` is the second cursor probe, matching Go's `firstString`.
    let next_key = research_payload_envelope(&json!({"itemList": [], "nextKey": "key-9"}));
    assert_eq!(next_key.next_cursor, "key-9");

    // A payload with no list at all is not a single entry when it only carries
    // pagination metadata.
    let bare = research_payload_envelope(&json!({"total": 3}));
    assert!(bare.entries.is_empty());
    assert_eq!(bare.total, 3);
}

#[test]
fn payload_envelope_picks_the_first_sorted_entry_list_like_go() {
    // Parity: go:452dea11:pkg/futu/adapter_advanced_helpers.go::payloadEntries.
    // Go sorts the payload keys and takes the first list-of-objects key; the
    // remaining keys become metadata. Non-object rows inside a list are
    // dropped, and a row-shaped payload becomes its own single entry.
    let envelope = research_payload_envelope(&json!({
        "dataList": [{"code": "AAPL"}, "scalar", {"code": "MSFT"}],
        "allCount": 2,
        "currency": "USD",
    }));
    assert_eq!(envelope.entries.len(), 2);
    assert_eq!(envelope.entries[0]["code"], json!("AAPL"));
    assert_eq!(envelope.entries[1]["code"], json!("MSFT"));
    assert_eq!(envelope.total, 2);
    assert!(
        envelope.metadata.get("dataList").is_none(),
        "the chosen list must not stay in metadata"
    );

    // `allCount` is consumed as the total and not echoed back as metadata.
    let row_shaped = research_payload_envelope(&json!({"title": "CPI", "country": "US"}));
    assert_eq!(row_shaped.entries.len(), 1);
    assert_eq!(row_shaped.entries[0]["title"], json!("CPI"));
    assert!(row_shaped.metadata.is_empty());
}

#[test]
fn catalog_operations_build_strict_opend_requests_like_go() {
    // Parity: go:452dea11:pkg/futu/adapter_research_contract_test.go:12
    // TestResearchCatalogOperationsBuildStrictOpenDRequests.
    //
    // The seven catalog operations and the exact OpenD field each one must
    // carry after the public query is translated. Go also asserts the business
    // `direction` never leaks into the OpenD params.
    let cases = [
        (
            "Qot_GetPlateSet",
            "HK",
            json!({"plateType": "concept"}),
            "plateSetType",
            json!(3),
        ),
        ("Qot_GetStaticInfo", "US", json!({}), "secType", json!(4)),
        (
            "Qot_GetHeatMapData",
            "US",
            json!({"plateType": "concept"}),
            "plateType",
            json!(1),
        ),
        (
            "Qot_GetEconomicCalendar",
            "SH",
            json!({"beginDate": "2026-07-23"}),
            "marketList",
            json!([21]),
        ),
        (
            "Qot_GetInstitutionHoldingChange",
            "US",
            json!({"institutionId": 7}),
            "institutionId",
            json!(7),
        ),
    ];
    for (protocol, market, raw, field, want) in cases {
        let mut params = raw.as_object().expect("object").clone();
        inject_advanced_defaults(&mut params, protocol, &ResearchQueryScope::new(market, ""))
            .unwrap_or_else(|error| panic!("{protocol}: {error}"));
        assert_eq!(params[field], want, "{protocol} {field}");
    }

    // Down movers translate the business direction into `sortDir` and remove
    // the business field so it never reaches OpenD.
    let mut movers = Map::new();
    movers.insert("direction".to_owned(), json!("down"));
    translate_top_movers_direction(&mut movers).expect("down movers");
    assert_eq!(movers["sortDir"], json!(1));
    assert!(
        !movers.contains_key("direction"),
        "business direction leaked into OpenD params: {movers:?}"
    );

    // Plate members need the exact plate instrument, which the caller resolves
    // from `MARKET.CODE` before the defaults run.
    let mut members = Map::new();
    members.insert("plate".to_owned(), json!({"market": 1, "code": "BK1001"}));
    inject_advanced_defaults(
        &mut members,
        "Qot_GetPlateSecurity",
        &ResearchQueryScope::new("HK", ""),
    )
    .expect("plate members");
    assert_eq!(members["plate"]["market"], json!(1));
    assert_eq!(members["plate"]["code"], json!("BK1001"));
}

#[test]
fn catalog_operations_reject_the_exact_go_parameter_matrix() {
    // Parity: go:452dea11:pkg/futu/adapter_research_contract_test.go:84
    // TestResearchCatalogOperationsRejectMissingOrInvalidParameters.
    //
    // The same ten cases with Go's per-case `FeatureQuery.Market` values; two
    // of them deliberately pass an empty market, which is what makes Go
    // require the market/date inputs at all.
    let cases = [
        ("HK", "Qot_GetPlateSet", json!({}), "requires plateType"),
        (
            "HK",
            "Qot_GetPlateSet",
            json!({"plateType": "theme"}),
            "unsupported plateType",
        ),
        (
            "HK",
            "Qot_GetPlateSet",
            json!({"plateType": 4.0}),
            "unsupported plateType",
        ),
        (
            "HK",
            "Qot_GetPlateSecurity",
            json!({}),
            "exact plate instrumentId",
        ),
        ("", "Qot_GetStaticInfo", json!({}), "requires market"),
        (
            "",
            "Qot_GetEconomicCalendar",
            json!({}),
            "requires beginDate",
        ),
        ("HK", "Qot_GetDividendCalendar", json!({}), "requires date"),
        (
            "US",
            "Qot_GetInstitutionHoldingChange",
            json!({}),
            "requires a positive integer institutionId",
        ),
        (
            "US",
            "Qot_GetInstitutionHoldingChange",
            json!({"institutionId": 1.5}),
            "requires a positive integer institutionId",
        ),
        (
            "US",
            "Qot_GetTopMoversRank",
            json!({"direction": "flat"}),
            "unsupported top movers direction",
        ),
    ];
    for (market, protocol, raw, want) in cases {
        let mut params = raw.as_object().expect("object").clone();
        let error =
            inject_advanced_defaults(&mut params, protocol, &ResearchQueryScope::new(market, ""))
                .expect_err(&format!(
                    "{protocol}/market={market:?} must be rejected, want {want:?}"
                ));
        // The rejection must carry the Go substring.
        assert!(
            error.message().contains(want),
            "{protocol}/market={market:?} got {:?}, want {want:?}",
            error.message()
        );
    }
}
