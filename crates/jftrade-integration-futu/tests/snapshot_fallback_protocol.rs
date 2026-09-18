//! Framed OpenD coverage for the delayed `Qot_StockScreen` (3252) fallback.
//!
//! Parity: go:pkg/futu/snapshot_fallback_test.go drives the adapter through a
//! scripted OpenD server and asserts `subCallCount() == 0`, i.e. the delayed
//! path resolves static info, pages `Qot_StockScreen` and never creates a
//! BasicQot subscription. The same is asserted here against a framed loopback
//! server that records every request protocol id.

use std::io::{Read, Write};
use std::net::{SocketAddr, TcpListener, TcpStream};
use std::sync::{Arc, Mutex};
use std::thread::{self, JoinHandle};
use std::time::Duration;

use jftrade_integration_futu::{
    Frame, OpenDSessionCoordinator, OpenDTcpProbeConfig, STOCK_SCREEN_SNAPSHOT_SOURCE,
    StockScreenSnapshotFallback, decode_frame, encode_frame, trade_proto,
};
use jftrade_marketdata::MarketDataRuntimeRecorder;
use prost::Message;

use trade_proto::qot_common::Security;
use trade_proto::qot_common::SecurityStaticBasic;
use trade_proto::qot_common::SecurityStaticInfo;
use trade_proto::qot_get_static_info as static_info;
use trade_proto::qot_stock_screen as screen;

#[derive(Clone, PartialEq, Message)]
struct InitResponse {
    #[prost(int32, optional, tag = "1")]
    ret_type: Option<i32>,
    #[prost(message, optional, tag = "4")]
    s2c: Option<InitStateWire>,
}

#[derive(Clone, PartialEq, Message)]
struct InitStateWire {
    #[prost(int32, tag = "1")]
    server_ver: i32,
    #[prost(uint64, tag = "3")]
    conn_id: u64,
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

fn static_info_response(entries: Vec<(u64, i32, &str, &str)>) -> Vec<u8> {
    static_info::Response {
        ret_type: 0,
        ret_msg: None,
        err_code: None,
        s2c: Some(static_info::S2c {
            static_info_list: entries
                .into_iter()
                .map(|(id, market, code, name)| SecurityStaticInfo {
                    basic: SecurityStaticBasic {
                        id: id as i64,
                        security: Security {
                            market,
                            code: code.to_owned(),
                        },
                        name: name.to_owned(),
                        ..Default::default()
                    },
                    ..Default::default()
                })
                .collect(),
        }),
    }
    .encode_to_vec()
}

fn screen_row(stock_id: u64, price: f64, values: &[(i32, f64)]) -> screen::StockScreenItem {
    let mut results = vec![screen::RspItemResult {
        simple_property_result: Some(screen::ResultPropertySimple {
            property: Some(screen::PropertySimple { name: Some(2201) }),
            dval: Some(price),
            ..Default::default()
        }),
        ..Default::default()
    }];
    for (property, value) in values {
        if *property == 3101 {
            results.push(screen::RspItemResult {
                cumulative_property_result: Some(screen::ResultPropertyCumulative {
                    property: Some(screen::PropertyCumulative {
                        name: Some(*property),
                        days: Some(1),
                        ..Default::default()
                    }),
                    dval: Some(*value),
                    ..Default::default()
                }),
                ..Default::default()
            });
        } else {
            results.push(screen::RspItemResult {
                simple_property_result: Some(screen::ResultPropertySimple {
                    property: Some(screen::PropertySimple {
                        name: Some(*property),
                    }),
                    dval: Some(*value),
                    ..Default::default()
                }),
                ..Default::default()
            });
        }
    }
    screen::StockScreenItem {
        stock_id: Some(stock_id),
        results,
    }
}

/// Serves the InitConnect handshake, then answers each recorded request in
/// order. Every protocol id is recorded so the "no Qot_Sub" contract stays
/// observable after the fact.
fn server(responses: Vec<Vec<u8>>) -> (SocketAddr, Arc<Mutex<Vec<Frame>>>, JoinHandle<()>) {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let requests: Arc<Mutex<Vec<Frame>>> = Arc::new(Mutex::new(Vec::new()));
    let recorded = Arc::clone(&requests);
    let task = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let init = read_frame(&mut stream);
        assert_eq!(init.header.proto_id, 1001);
        write_response(
            &mut stream,
            1001,
            init.header.serial_no,
            InitResponse {
                ret_type: Some(0),
                s2c: Some(InitStateWire {
                    server_ver: 1009,
                    conn_id: 7,
                }),
            }
            .encode_to_vec(),
        );
        for body in responses {
            let frame = read_frame(&mut stream);
            assert_ne!(
                frame.header.proto_id, 3001,
                "the delayed fallback must never issue Qot_Sub"
            );
            recorded.lock().expect("requests").push(frame.clone());
            write_response(
                &mut stream,
                frame.header.proto_id,
                frame.header.serial_no,
                body,
            );
        }
    });
    (address, requests, task)
}

fn coordinator(address: SocketAddr) -> Arc<Mutex<OpenDSessionCoordinator>> {
    Arc::new(Mutex::new(
        OpenDSessionCoordinator::connect(
            OpenDTcpProbeConfig::new(address, Duration::from_secs(2)),
            Arc::new(MarketDataRuntimeRecorder::default()),
            Vec::new(),
            0,
        )
        .expect("managed OpenD coordinator"),
    ))
}

#[test]
fn delayed_snapshot_fallback_reads_static_info_and_pages_stock_screen_without_subscribing() {
    let static_body = static_info_response(vec![
        (101, 21, "600519", "Kweichow Moutai"),
        (202, 22, "000001", "Ping An Bank"),
        (303, 11, "AAPL", "Apple"),
    ]);
    // Market values ascend, so the US page (2) is read before the A-share page
    // (3). The US page returns no usable row: the delayed provider simply has
    // nothing for US.AAPL, and Go must not synthesize one.
    let us_body = screen::Response {
        ret_type: 0,
        ret_msg: None,
        err_code: None,
        s2c: Some(screen::S2c {
            data_list: Vec::new(),
            last_page: Some(1),
            all_count: Some(0),
        }),
    }
    .encode_to_vec();
    let a_share_body = screen::Response {
        ret_type: 0,
        ret_msg: None,
        err_code: None,
        s2c: Some(screen::S2c {
            data_list: vec![
                screen_row(101, 1500.0, &[(2203, 1490.0)]),
                screen_row(202, 12.3, &[(3101, 0.3)]),
            ],
            last_page: Some(1),
            all_count: Some(2),
        }),
    }
    .encode_to_vec();
    let (address, requests, task) = server(vec![static_body, us_body, a_share_body]);
    let fallback = StockScreenSnapshotFallback::new(coordinator(address));

    let items = fallback
        .query(&[
            "SH.600519".to_owned(),
            "SZ.000001".to_owned(),
            "US.AAPL".to_owned(),
        ])
        .expect("delayed fallback query");
    assert_eq!(items.len(), 2, "a missing delayed row is never synthesized");
    assert_eq!(items[0].symbol, "SH.600519");
    assert_eq!(items[0].name.as_deref(), Some("Kweichow Moutai"));
    assert_eq!(items[0].source, STOCK_SCREEN_SNAPSHOT_SOURCE);
    assert_eq!(items[1].symbol, "SZ.000001");
    assert_eq!(
        items[1].previous_close,
        Some("12".parse().expect("decimal"))
    );

    let recorded = requests.lock().expect("requests");
    let protocols = recorded
        .iter()
        .map(|frame| frame.header.proto_id)
        .collect::<Vec<_>>();
    assert_eq!(
        protocols,
        vec![
            static_info::PROTOCOL_ID,
            screen::PROTOCOL_ID,
            screen::PROTOCOL_ID
        ],
        "static info resolves ids first, then one StockScreen page per market; no Qot_Sub"
    );

    // Each recorded StockScreen request carries the strict delayed fields for
    // exactly one market value.
    let us = screen::Request::decode(recorded[1].body.as_slice()).expect("us screen request");
    assert_eq!(us.c2s.watchlist_stock_ids, vec![303]);
    assert_eq!(us.c2s.page_count, Some(1));
    assert_eq!(
        us.c2s.filter_list[0]
            .simple_field_query
            .as_ref()
            .map(|query| (query.simple_field, query.screen_value_list.clone())),
        Some((Some(1), vec![2]))
    );
    let a_share =
        screen::Request::decode(recorded[2].body.as_slice()).expect("a-share screen request");
    assert_eq!(a_share.c2s.watchlist_stock_ids, vec![101, 202]);
    assert_eq!(a_share.c2s.page_count, Some(2));
    assert_eq!(
        a_share.c2s.filter_list[0]
            .simple_field_query
            .as_ref()
            .map(|query| (query.simple_field, query.screen_value_list.clone())),
        Some((Some(1), vec![3]))
    );
    assert_eq!(
        a_share.c2s.filter_list[1]
            .simple_field_query
            .as_ref()
            .map(|query| (query.simple_field, query.screen_value_list.clone())),
        Some((Some(4), vec![1]))
    );
    drop(recorded);
    fallback
        .reader_coordinator()
        .lock()
        .expect("lock")
        .close()
        .expect("close");
    task.join().expect("server");
}
