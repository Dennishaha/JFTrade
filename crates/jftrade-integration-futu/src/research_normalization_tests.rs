use super::*;
use serde_json::json;

#[test]
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
