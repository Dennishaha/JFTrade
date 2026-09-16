use super::*;

#[test]
fn search_request_preserves_chinese_name_and_requests_full_candidate_window() {
    let request =
        wire::Request::decode(encode_request(" 分众传媒 ", 100).unwrap().as_slice()).unwrap();
    assert_eq!(wire::PROTOCOL_ID, 3262);
    assert_eq!(request.c2s.keyword, "分众传媒");
    assert_eq!(request.c2s.max_count, Some(100));
    assert!(request.c2s.header.is_none());
    for keyword in ["", " ", "分众\n传媒"] {
        assert!(matches!(
            encode_request(keyword, 100),
            Err(InstrumentSearchError::InvalidQuery)
        ));
    }

    // Parity: go:452dea11:pkg/futu/opend/search_quote_test.go:48
    // TestGetSearchQuoteValidatesInputAndPreservesOpenDErrors
    //
    // Go forwards the caller's maxCount and rejects anything outside 1..=100
    // before encoding, so a product limit must not be silently rewritten.
    for max_count in [0, -1, 101, i32::MAX] {
        assert!(matches!(
            encode_request("AAPL", max_count),
            Err(InstrumentSearchError::InvalidMaxCount(value)) if value == max_count
        ));
    }
    for max_count in [1, 10, 100] {
        let request =
            wire::Request::decode(encode_request("AAPL", max_count).unwrap().as_slice()).unwrap();
        assert_eq!(
            request.c2s.max_count,
            Some(max_count),
            "the caller's maxCount must reach OpenD unchanged"
        );
    }
}

#[test]
fn a_success_without_a_payload_normalizes_to_an_empty_candidate_list() {
    // Parity: go:452dea11:pkg/futu/opend/search_quote_test.go:77
    // TestGetSearchQuoteNormalizesMissingPayload
    let body = wire::Response {
        ret_type: 0,
        ret_msg: None,
        err_code: None,
        s2c: None,
    }
    .encode_to_vec();
    assert!(
        decode_response(&body)
            .expect("missing payload is not an error")
            .is_empty(),
        "a payload-less success must normalize to an empty list"
    );
}

fn response(entries: Vec<wire::SearchQuote>) -> Vec<u8> {
    wire::Response {
        ret_type: 0,
        ret_msg: None,
        err_code: None,
        s2c: Some(wire::S2c {
            search_quote_list: entries,
        }),
    }
    .encode_to_vec()
}

#[test]
fn search_response_maps_chinese_stock_and_preserves_unavailable_markets() {
    let entries = decode_response(&response(vec![
        wire::SearchQuote {
            market: Some(22),
            code: Some("002027".to_owned()),
            name: Some("分众传媒".to_owned()),
            sec_type: Some(3),
            is_watched: Some(true),
        },
        wire::SearchQuote {
            market: Some(41),
            code: Some("9988".to_owned()),
            name: None,
            sec_type: None,
            is_watched: None,
        },
    ]))
    .unwrap();
    assert_eq!(entries[0].market, "SZ");
    assert_eq!(entries[0].code, "002027");
    assert_eq!(entries[0].name.as_deref(), Some("分众传媒"));
    assert_eq!(entries[0].security_type.as_deref(), Some("EQUITY"));
    assert!(entries[0].is_watched);
    assert_eq!(entries[1].market, "JP");
}

#[test]
fn search_failures_and_malformed_responses_are_not_empty_successes() {
    let rejected = wire::Response {
        ret_type: -1,
        ret_msg: Some("denied".to_owned()),
        err_code: Some(7),
        s2c: None,
    }
    .encode_to_vec();
    assert!(matches!(
        decode_response(&rejected),
        Err(InstrumentSearchError::Rejected { err_code: 7, .. })
    ));
    let missing = wire::Response {
        ret_type: 0,
        ret_msg: None,
        err_code: None,
        s2c: None,
    }
    .encode_to_vec();
    // Go's `GetSearchQuote` treats a payload-less success as "no matches", so
    // this stays a distinct empty-success case rather than a protocol error.
    assert!(
        decode_response(&missing)
            .expect("missing payload")
            .is_empty()
    );
    assert!(decode_response(&response(vec![wire::SearchQuote::default()])).is_err());
    assert!(decode_response(&[0xff]).is_err());
    assert!(decode_response(&response(vec![])).unwrap().is_empty());
}

#[test]
fn qualified_lookup_encodes_exact_security_without_market_catalog_or_subscription() {
    use crate::trade_proto::qot_get_static_info as static_wire;
    let request =
        static_wire::Request::decode(encode_lookup("SZ", "002027").unwrap().as_slice()).unwrap();
    assert_eq!(static_wire::PROTOCOL_ID, 3202);
    assert!(request.c2s.market.is_none());
    assert!(request.c2s.sec_type.is_none());
    assert_eq!(request.c2s.security_list.len(), 1);
    assert_eq!(request.c2s.security_list[0].market, 22);
    assert_eq!(request.c2s.security_list[0].code, "002027");
    let body = static_wire::Response {
        ret_type: 0,
        s2c: Some(static_wire::S2c {
            static_info_list: vec![crate::trade_proto::qot_common::SecurityStaticInfo {
                basic: crate::trade_proto::qot_common::SecurityStaticBasic {
                    security: request.c2s.security_list[0].clone(),
                    name: "分众传媒".to_owned(),
                    sec_type: 3,
                    lot_size: 100,
                    ..Default::default()
                },
                ..Default::default()
            }],
        }),
        ..Default::default()
    }
    .encode_to_vec();
    let entries = decode_lookup(&body).unwrap();
    assert_eq!(entries[0].code, "002027");
    assert_eq!(entries[0].lot_size, Some(100));
    assert_eq!(entries[0].name.as_deref(), Some("分众传媒"));
}

#[test]
fn static_info_lookup_maps_every_security_static_basic_field() {
    // Parity: go:452dea11:pkg/futu/opend/new_methods_test.go:534 TestGetStaticInfo
    //
    // Go's GetStaticInfo returns the raw SecurityStaticInfo list; Rust exposes
    // the neutral lookup projection, so this pins every field the neutral
    // contract keeps (identity, name, security type and lot size) and proves
    // the request targets Qot_GetStaticInfo (3202) for HK.00700.
    use crate::trade_proto::qot_common::{Security, SecurityStaticBasic, SecurityStaticInfo};
    use crate::trade_proto::qot_get_static_info as static_wire;

    let request = static_wire::Request::decode(encode_lookup("HK", "00700").unwrap().as_slice())
        .expect("decode lookup request");
    assert_eq!(request.c2s.security_list.len(), 1);
    assert_eq!(request.c2s.security_list[0].market, 1);
    assert_eq!(request.c2s.security_list[0].code, "00700");

    let body = static_wire::Response {
        ret_type: 0,
        s2c: Some(static_wire::S2c {
            static_info_list: vec![SecurityStaticInfo {
                basic: SecurityStaticBasic {
                    security: Security {
                        market: 1,
                        code: "00700".to_owned(),
                    },
                    id: 700,
                    name: "Tencent".to_owned(),
                    sec_type: 3,
                    list_time: "2004-06-16".to_owned(),
                    lot_size: 100,
                    ..Default::default()
                },
                ..Default::default()
            }],
        }),
        ..Default::default()
    }
    .encode_to_vec();

    let entries = decode_lookup(&body).expect("static info lookup");
    assert_eq!(entries.len(), 1);
    assert_eq!(entries[0].market, "HK");
    assert_eq!(entries[0].code, "00700");
    assert_eq!(entries[0].name.as_deref(), Some("Tencent"));
    assert_eq!(entries[0].security_type.as_deref(), Some("EQUITY"));
    assert_eq!(entries[0].lot_size, Some(100));

    // A rejection keeps the OpenD return details instead of fabricating a row.
    let rejected = static_wire::Response {
        ret_type: -1,
        ret_msg: Some("static info denied".to_owned()),
        err_code: Some(1003),
        s2c: None,
    }
    .encode_to_vec();
    assert!(matches!(
        decode_lookup(&rejected),
        Err(InstrumentSearchError::Rejected {
            ret_type: -1,
            err_code: 1003,
            ..
        })
    ));
}
