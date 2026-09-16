//! Framed OpenD coverage for the typed stock-screen reader (`Qot_StockScreen`).
//!
//! The fake server only stands in for OpenD's wire endpoint: request framing,
//! the serial routing of `OpenDSessionCoordinator`, protobuf encode/decode and
//! the typed projection all stay live.

use std::io::{Read, Write};
use std::net::{SocketAddr, TcpListener, TcpStream};
use std::sync::{Arc, Mutex};
use std::thread::{self, JoinHandle};
use std::time::Duration;

use jftrade_integration_futu::{
    Frame, OpenDSessionCoordinator, OpenDStockScreenReader, OpenDTcpProbeConfig, StockScreenQuery,
    StockScreenQueryError, StockScreenReadPort, StockScreenValue, decode_frame, encode_frame,
    trade_proto,
};
use jftrade_marketdata::MarketDataRuntimeRecorder;
use prost::Message;

const INIT_CONNECT: u32 = 1001;
const STOCK_SCREEN: u32 = trade_proto::qot_stock_screen::PROTOCOL_ID;
const GET_STATIC_INFO: u32 = trade_proto::qot_get_static_info::PROTOCOL_ID;

type StaticInfoResponse = trade_proto::qot_get_static_info::Response;
type StaticInfoS2c = trade_proto::qot_get_static_info::S2c;
type StaticSecurity = trade_proto::qot_common::Security;
type StaticSecurityInfo = trade_proto::qot_common::SecurityStaticInfo;
type StaticSecurityBasic = trade_proto::qot_common::SecurityStaticBasic;

#[derive(Clone, PartialEq, Message)]
struct InitResponse {
    #[prost(int32, optional, tag = "1")]
    ret_type: Option<i32>,
    #[prost(message, optional, tag = "4")]
    s2c: Option<InitState>,
}

#[derive(Clone, PartialEq, Message)]
struct InitState {
    #[prost(uint64, tag = "3")]
    conn_id: u64,
}

#[derive(Clone, PartialEq, Message)]
struct ErrorResponse {
    #[prost(int32, optional, tag = "1")]
    ret_type: Option<i32>,
    #[prost(string, optional, tag = "2")]
    ret_msg: Option<String>,
    #[prost(int32, optional, tag = "3")]
    err_code: Option<i32>,
}

fn read_frame(stream: &mut TcpStream) -> Frame {
    let mut header = [0_u8; 44];
    stream.read_exact(&mut header).expect("frame header");
    let body_len = u32::from_le_bytes(header[12..16].try_into().expect("body length")) as usize;
    let mut packet = vec![0_u8; 44 + body_len];
    packet[..44].copy_from_slice(&header);
    stream.read_exact(&mut packet[44..]).expect("frame body");
    decode_frame(&packet).expect("valid OpenD frame")
}

fn write_response(stream: &mut TcpStream, protocol: u32, serial: u32, body: Vec<u8>) {
    stream
        .write_all(&encode_frame(protocol, serial, &body).expect("frame"))
        .expect("write response");
}

fn server<F>(handler: F) -> (SocketAddr, JoinHandle<()>)
where
    F: FnOnce(&mut TcpStream, Frame) + Send + 'static,
{
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let task = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let init = read_frame(&mut stream);
        assert_eq!(init.header.proto_id, INIT_CONNECT);
        write_response(
            &mut stream,
            init.header.proto_id,
            init.header.serial_no,
            InitResponse {
                ret_type: Some(0),
                s2c: Some(InitState { conn_id: 7 }),
            }
            .encode_to_vec(),
        );
        let request = read_frame(&mut stream);
        handler(&mut stream, request);
    });
    (address, task)
}

fn make_coordinator(address: SocketAddr, timeout: Duration) -> Arc<Mutex<OpenDSessionCoordinator>> {
    Arc::new(Mutex::new(
        OpenDSessionCoordinator::connect(
            OpenDTcpProbeConfig::new(address, timeout),
            Arc::new(MarketDataRuntimeRecorder::default()),
            Vec::new(),
            0,
        )
        .expect("managed OpenD coordinator"),
    ))
}

fn definition() -> serde_json::Value {
    serde_json::json!({
        "market": "US",
        "conditions": [
            {"factor": {"factorKey": "simple.price", "params": {}}, "value": {"min": 10.0, "max": 200.0}},
            {"factor": {"factorKey": "financial.net_profit", "params": {"term": 10, "year": 2025}}, "value": {"min": 1.0}}
        ],
        "columns": [
            {"columnId": "price", "factor": {"instanceId": "price", "factorKey": "simple.price", "params": {}}},
            {"columnId": "roe", "factor": {"instanceId": "roe", "factorKey": "financial.roe", "params": {}}}
        ],
        "sorts": [{"factor": {"factorKey": "simple.market_cap"}, "direction": "desc"}],
        "pool": {"watchlistStockIds": ["7"]}
    })
}

fn static_info_response(entries: Vec<(u64, i32, &str)>) -> Vec<u8> {
    StaticInfoResponse {
        ret_type: 0,
        ret_msg: None,
        err_code: None,
        s2c: Some(StaticInfoS2c {
            static_info_list: entries
                .into_iter()
                .map(|(id, market, code)| StaticSecurityInfo {
                    basic: StaticSecurityBasic {
                        id: id as i64,
                        security: StaticSecurity {
                            market,
                            code: code.to_owned(),
                        },
                        ..Default::default()
                    },
                    ..Default::default()
                })
                .collect(),
        }),
    }
    .encode_to_vec()
}

#[test]
fn stock_screen_request_encodes_typed_filters_retrieve_sort_and_resolves_mainland_identity() {
    use trade_proto::qot_stock_screen::{Response, S2c, StockScreenItem as WireItem};

    let (address, task) = server(|stream, request| {
        // The reader always issues the screen call first, then resolves A-share
        // identities through Qot_GetStaticInfo with both SH and SZ candidates.
        assert_eq!(request.header.proto_id, STOCK_SCREEN);
        let protocol = request.header.proto_id;
        let serial = request.header.serial_no;
        let call = trade_proto::qot_stock_screen::Request::decode(request.body.as_slice())
            .expect("screen request");
        let c2s = call.c2s;
        assert_eq!(c2s.page_from, Some(3));
        assert_eq!(c2s.page_count, Some(25));
        assert_eq!(c2s.watchlist_stock_ids, vec![7]);
        // No `field.market` condition is present, so the adapter prepends the
        // implicit market filter (simpleField=1, screenValueList=[market]).
        assert_eq!(c2s.filter_list.len(), 3);
        assert_eq!(
            c2s.filter_list[0]
                .simple_field_query
                .as_ref()
                .and_then(|query| query.simple_field),
            Some(1)
        );
        assert_eq!(
            c2s.filter_list[0]
                .simple_field_query
                .as_ref()
                .map(|query| query.screen_value_list.as_slice()),
            // SH/SZ/CN all map to the combined A-share market value 3.
            Some([3_i64].as_slice())
        );
        assert_eq!(
            c2s.filter_list[1]
                .simple_property_query
                .as_ref()
                .and_then(|query| query.property.name),
            Some(2201)
        );
        assert_eq!(
            c2s.filter_list[1]
                .simple_property_query
                .as_ref()
                .and_then(|query| query.filter_min.as_ref().map(|boundary| boundary.value)),
            Some(10.0)
        );
        assert_eq!(
            c2s.filter_list[1]
                .simple_property_query
                .as_ref()
                .and_then(|query| query.filter_max.as_ref().map(|boundary| boundary.value)),
            Some(200.0)
        );
        assert_eq!(c2s.retrieve_list.len(), 3);
        assert_eq!(
            c2s.retrieve_list[0]
                .basic_property
                .as_ref()
                .and_then(|property| property.name),
            Some(1101)
        );
        assert_eq!(
            c2s.sort_list[0]
                .simple_property
                .as_ref()
                .and_then(|property| property.name),
            Some(2301)
        );
        assert_eq!(c2s.sort_list[0].direction, 2);

        write_response(
            stream,
            protocol,
            serial,
            Response {
                ret_type: 0,
                ret_msg: None,
                err_code: None,
                s2c: Some(S2c {
                    data_list: vec![WireItem {
                        stock_id: Some(101),
                        results: vec![trade_proto::qot_stock_screen::RspItemResult {
                            basic_property_result: Some(
                                trade_proto::qot_stock_screen::ResultPropertyBasic {
                                    property: Some(trade_proto::qot_stock_screen::PropertyBasic {
                                        name: Some(1101),
                                    }),
                                    value_type: Some(1),
                                    sval: Some("600519".to_owned()),
                                    ..Default::default()
                                },
                            ),
                            simple_property_result: Some(
                                trade_proto::qot_stock_screen::ResultPropertySimple {
                                    property: Some(trade_proto::qot_stock_screen::PropertySimple {
                                        name: Some(2201),
                                    }),
                                    value_type: Some(4),
                                    dval: Some(1_700.0),
                                    ..Default::default()
                                },
                            ),
                            ..Default::default()
                        }],
                    }],
                    last_page: Some(0),
                    all_count: Some(20),
                }),
            }
            .encode_to_vec(),
        );

        let identity = read_frame(stream);
        assert_eq!(identity.header.proto_id, GET_STATIC_INFO);
        let request = trade_proto::qot_get_static_info::Request::decode(identity.body.as_slice())
            .expect("static info request");
        let securities = request.c2s.security_list;
        assert_eq!(securities.len(), 2);
        assert_eq!(securities[0].market, 21);
        assert_eq!(securities[1].market, 22);
        assert_eq!(securities[0].code, "600519");
        write_response(
            stream,
            identity.header.proto_id,
            identity.header.serial_no,
            static_info_response(vec![(101, 21, "600519")]),
        );
    });
    let coordinator = make_coordinator(address, Duration::from_secs(1));
    let reader = OpenDStockScreenReader::new(Arc::clone(&coordinator));
    let mut definition = definition();
    definition["market"] = serde_json::json!("SH");
    let query = StockScreenQuery::new("SH", definition, 3, 25).expect("query");
    let page = reader.query(&query).expect("stock screen page");
    assert!(!page.last_page);
    assert_eq!(page.all_count, Some(20));
    assert_eq!(page.items.len(), 1);
    let security = page.items[0].security.as_ref().expect("resolved identity");
    assert_eq!(security.market, "SH");
    assert_eq!(security.code, "600519");
    assert_eq!(security.instrument_id, "SH.600519");
    assert!(
        matches!(page.items[0].results[0].value, StockScreenValue::String { ref value } if value == "600519")
    );
    coordinator.lock().expect("lock").close().expect("close");
    task.join().expect("server");
}

#[test]
fn stock_screen_reader_rejects_unsupported_market_and_throttles_after_ten_calls() {
    let (address, task) = server(|stream, request| {
        assert_eq!(request.header.proto_id, STOCK_SCREEN);
        write_response(
            stream,
            request.header.proto_id,
            request.header.serial_no,
            ErrorResponse {
                ret_type: Some(-1),
                ret_msg: Some("rate limited".to_owned()),
                err_code: Some(1000),
            }
            .encode_to_vec(),
        );
        // Drain the remaining probes so the limiter can observe a live session.
        for _ in 0..9 {
            let request = read_frame(stream);
            write_response(
                stream,
                request.header.proto_id,
                request.header.serial_no,
                ErrorResponse {
                    ret_type: Some(-1),
                    ret_msg: Some("rate limited".to_owned()),
                    err_code: Some(1000),
                }
                .encode_to_vec(),
            );
        }
    });
    let coordinator = make_coordinator(address, Duration::from_secs(1));
    let reader = OpenDStockScreenReader::new(Arc::clone(&coordinator));

    assert!(matches!(
        StockScreenQuery::new("SG", definition(), 0, 25),
        Err(StockScreenQueryError::InvalidQuery(_))
    ));
    assert!(matches!(
        StockScreenQuery::new("US", definition(), 0, 0),
        Err(StockScreenQueryError::InvalidQuery(_))
    ));
    assert!(matches!(
        StockScreenQuery::new("US", definition(), -1, 25),
        Err(StockScreenQueryError::InvalidQuery(_))
    ));

    let query = StockScreenQuery::new("US", definition(), 0, 25).expect("query");
    for _ in 0..10 {
        let error = reader.query(&query).expect_err("OpenD rejection");
        assert!(matches!(error, StockScreenQueryError::Rejected { .. }));
    }
    let throttled = reader.query(&query).expect_err("rate limited");
    assert!(matches!(
        throttled,
        StockScreenQueryError::RateLimited { retry_after_ms } if retry_after_ms > 0
    ));
    coordinator.lock().expect("lock").close().expect("close");
    task.join().expect("server");
}

#[test]
fn stock_screen_identity_resolution_fails_closed_without_static_info() {
    use trade_proto::qot_stock_screen::{Response, S2c, StockScreenItem as WireItem};

    let (address, task) = server(|stream, request| {
        assert_eq!(request.header.proto_id, STOCK_SCREEN);
        write_response(
            stream,
            request.header.proto_id,
            request.header.serial_no,
            Response {
                ret_type: 0,
                ret_msg: None,
                err_code: None,
                s2c: Some(S2c {
                    data_list: vec![WireItem {
                        stock_id: Some(4001),
                        results: vec![trade_proto::qot_stock_screen::RspItemResult {
                            basic_property_result: Some(
                                trade_proto::qot_stock_screen::ResultPropertyBasic {
                                    property: Some(trade_proto::qot_stock_screen::PropertyBasic {
                                        name: Some(1101),
                                    }),
                                    value_type: Some(1),
                                    sval: Some("600519".to_owned()),
                                    ..Default::default()
                                },
                            ),
                            ..Default::default()
                        }],
                    }],
                    last_page: Some(1),
                    all_count: Some(1),
                }),
            }
            .encode_to_vec(),
        );
        let identity = read_frame(stream);
        assert_eq!(identity.header.proto_id, GET_STATIC_INFO);
        // The provider returns no matching static entry, so the adapter must
        // fail closed instead of relabelling the row with the request market.
        write_response(
            stream,
            identity.header.proto_id,
            identity.header.serial_no,
            static_info_response(Vec::new()),
        );
    });
    let coordinator = make_coordinator(address, Duration::from_secs(1));
    let reader = OpenDStockScreenReader::new(Arc::clone(&coordinator));
    let query = StockScreenQuery::new("CN", definition(), 0, 25).expect("query");
    let error = reader.query(&query).expect_err("unresolved identity");
    assert!(
        matches!(error, StockScreenQueryError::InvalidResponse(message) if message.contains("could not resolve mainland"))
    );
    coordinator.lock().expect("lock").close().expect("close");
    task.join().expect("server");
}
