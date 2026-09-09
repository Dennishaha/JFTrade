use super::*;

#[test]
fn search_request_preserves_chinese_name_and_requests_full_candidate_window() {
    let request = wire::Request::decode(encode_request(" 分众传媒 ").unwrap().as_slice()).unwrap();
    assert_eq!(wire::PROTOCOL_ID, 3262);
    assert_eq!(request.c2s.keyword, "分众传媒");
    assert_eq!(request.c2s.max_count, Some(100));
    assert!(request.c2s.header.is_none());
    for keyword in ["", " ", "分众\n传媒"] {
        assert!(matches!(
            encode_request(keyword),
            Err(InstrumentSearchError::InvalidQuery)
        ));
    }
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
    assert!(matches!(
        decode_response(&missing),
        Err(InstrumentSearchError::MissingField("s2c"))
    ));
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
