//! Framed OpenD coverage for the typed earnings-calendar reader
//! (`Qot_GetEarningsCalendar`, protocol 3401).
//!
//! The fake server only stands in for OpenD's wire endpoint: request framing,
//! serial routing, protobuf encode/decode and the typed projection stay live.

use std::io::{Read, Write};
use std::net::{SocketAddr, TcpListener, TcpStream};
use std::sync::{Arc, Mutex};
use std::thread::{self, JoinHandle};
use std::time::Duration;

use jftrade_integration_futu::{
    EARNINGS_CALENDAR_PROTOCOL_ID, EarningsCalendarBoundary, EarningsCalendarFilter,
    EarningsCalendarInterval, EarningsCalendarQuery, EarningsCalendarReadPort,
    OpenDEarningsCalendarReader, OpenDSessionCoordinator, OpenDTcpProbeConfig, decode_frame,
    encode_frame, trade_proto,
};
use jftrade_marketdata::MarketDataRuntimeRecorder;
use prost::Message;

const INIT_CONNECT: u32 = 1001;

#[derive(Clone, PartialEq, Message)]
struct InitResponse {
    #[prost(int32, optional, tag = "1")]
    ret_type: Option<i32>,
    #[prost(message, optional, tag = "4")]
    s2c: Option<InitState>,
}

#[derive(Clone, PartialEq, Message)]
struct InitState {
    #[prost(int32, tag = "1")]
    server_ver: i32,
    #[prost(uint64, tag = "3")]
    conn_id: u64,
}

fn read_frame(stream: &mut TcpStream) -> jftrade_integration_futu::Frame {
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
    F: FnOnce(&mut TcpStream, jftrade_integration_futu::Frame) + Send + 'static,
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
                s2c: Some(InitState {
                    server_ver: 1009,
                    conn_id: 7,
                }),
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

#[test]
fn earnings_calendar_request_encodes_market_sort_dates_and_filters() {
    let (address, task) = server(|stream, request| {
        assert_eq!(request.header.proto_id, EARNINGS_CALENDAR_PROTOCOL_ID);
        assert_eq!(
            request.header.proto_id,
            trade_proto::qot_get_earnings_calendar::PROTOCOL_ID
        );
        let protocol = request.header.proto_id;
        let serial = request.header.serial_no;
        let call = trade_proto::qot_get_earnings_calendar::Request::decode(request.body.as_slice())
            .expect("earnings calendar request");
        let c2s = call.c2s;
        assert_eq!(c2s.market, 11);
        assert_eq!(c2s.sort_type, Some(6));
        assert_eq!(c2s.begin_date.as_deref(), Some("2026-07-01"));
        assert_eq!(c2s.end_date.as_deref(), Some("2026-07-07"));
        assert_eq!(c2s.filter_list.len(), 2);
        assert_eq!(c2s.filter_list[0].indicator_type, 4);
        assert_eq!(
            c2s.filter_list[0]
                .indicator_value
                .as_ref()
                .map(|value| value.value_list.as_slice()),
            Some([1_i64].as_slice())
        );
        let interval = c2s.filter_list[1]
            .indicator_value
            .as_ref()
            .and_then(|value| value.value_interval.as_ref())
            .expect("IV interval");
        assert_eq!(
            interval
                .filter_min
                .as_ref()
                .map(|boundary| (boundary.value, boundary.includes)),
            Some((10.0, true))
        );
        assert_eq!(
            interval
                .filter_max
                .as_ref()
                .map(|boundary| (boundary.value, boundary.includes)),
            Some((80.0, true))
        );

        use trade_proto::qot_get_earnings_calendar::{
            EarningsCalendarItem as WireItem, EstimateData, Response, S2c,
        };
        write_response(
            stream,
            protocol,
            serial,
            Response {
                ret_type: 0,
                ret_msg: None,
                err_code: None,
                s2c: Some(S2c {
                    item_list: vec![WireItem {
                        security: trade_proto::qot_common::Security {
                            market: 11,
                            code: "aapl".to_owned(),
                        },
                        name: Some("Apple".to_owned()),
                        earnings_date: Some("2026-07-22".to_owned()),
                        earnings_timestamp: Some(1_784_000_000.0),
                        pub_type: Some(2),
                        period_text: Some("2025Q2".to_owned()),
                        estimate_list: vec![EstimateData {
                            estimate_type: Some(1),
                            actual_value: None,
                            predict_value: Some(1.5),
                            currency: Some("USD".to_owned()),
                            period_type: Some(1),
                        }],
                        option_volume: Some(12_000),
                        iv: Some(42.5),
                        iv_rank: Some(11.0),
                        iv_percentile: Some(30.0),
                        market_cap: Some(1_000_000_000.0),
                        price: Some(190.25),
                    }],
                }),
            }
            .encode_to_vec(),
        );
    });
    let coordinator = make_coordinator(address, Duration::from_secs(1));
    let reader = OpenDEarningsCalendarReader::new(Arc::clone(&coordinator));
    let page = reader
        .query(&EarningsCalendarQuery {
            market: 11,
            sort_type: Some(6),
            begin_date: "2026-07-01".to_owned(),
            end_date: "2026-07-07".to_owned(),
            filters: vec![
                EarningsCalendarFilter {
                    indicator_type: 4,
                    value_list: vec![1],
                    interval: None,
                },
                EarningsCalendarFilter {
                    indicator_type: 6,
                    value_list: Vec::new(),
                    interval: Some(EarningsCalendarInterval {
                        min: Some(EarningsCalendarBoundary {
                            value: 10.0,
                            includes: true,
                        }),
                        max: Some(EarningsCalendarBoundary {
                            value: 80.0,
                            includes: true,
                        }),
                    }),
                },
            ],
        })
        .expect("earnings calendar page");
    assert_eq!(page.items.len(), 1);
    let row = &page.items[0];
    assert_eq!(row.security.market, "US");
    assert_eq!(row.security.code, "AAPL");
    assert_eq!(row.security.instrument_id, "US.AAPL");
    assert_eq!(row.earnings_date.as_deref(), Some("2026-07-22"));
    assert_eq!(row.iv, Some(42.5));
    assert_eq!(row.estimate_list.len(), 1);
    assert_eq!(row.estimate_list[0].predict_value, Some(1.5));
    coordinator
        .lock()
        .expect("coordinator lock")
        .close()
        .expect("close");
    task.join().expect("server");
}
