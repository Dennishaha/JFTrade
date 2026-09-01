//! Framed OpenD coverage for the advanced research readers.
//!
//! These tests deliberately use a real loopback TCP session.  The fake server
//! only stands in for OpenD's wire endpoint; request framing, serial routing,
//! managed-session errors, and the typed response projections all remain live.

use std::io::{Read, Write};
use std::net::{SocketAddr, TcpListener, TcpStream};
use std::sync::{Arc, Mutex};
use std::thread::{self, JoinHandle};
use std::time::Duration;

use jftrade_integration_futu::{
    Frame, FutuIndicatorInput, FutuIndicatorReadPort,
    FutuInstitutionOperation as InstitutionOperation, FutuInstitutionQuery as InstitutionQuery,
    FutuInstitutionReadPort, FutuShortInterestReadPort, FutuTechnicalIndicatorReader,
    IndicatorCalcQuery, IndicatorKline, IndicatorListQuery, OpenDInstitutionReader,
    OpenDSessionCloseReason, OpenDSessionCoordinator, OpenDSessionCoordinatorOutcome,
    OpenDShortInterestReader, OpenDTcpProbeConfig, ShortInterestOperation, ShortInterestQuery,
    decode_frame, encode_frame,
};
use jftrade_marketdata::MarketDataRuntimeRecorder;
use prost::Message;

const INIT_CONNECT: u32 = 1001;
const DAILY_SHORT_VOLUME: u32 = 3248;
const SHORT_INTEREST: u32 = 3249;
const INDICATOR_LIST: u32 = 3259;
const INDICATOR_CALC: u32 = 3260;
const INSTITUTION_LIST: u32 = 3418;

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

#[derive(Clone, PartialEq, Message)]
struct WireSecurity {
    #[prost(int32, tag = "1")]
    market: i32,
    #[prost(string, tag = "2")]
    code: String,
}

#[derive(Clone, PartialEq, Message)]
struct ShortRequest {
    #[prost(message, optional, tag = "1")]
    c2s: Option<ShortC2s>,
}

#[derive(Clone, PartialEq, Message)]
struct ShortC2s {
    #[prost(message, optional, tag = "1")]
    security: Option<WireSecurity>,
    #[prost(string, optional, tag = "2")]
    next_key: Option<String>,
    #[prost(int32, optional, tag = "3")]
    num: Option<i32>,
}

#[derive(Clone, PartialEq, Message)]
struct DailyResponse {
    #[prost(int32, optional, tag = "1")]
    ret_type: Option<i32>,
    #[prost(message, optional, tag = "4")]
    s2c: Option<DailyS2c>,
}

#[derive(Clone, PartialEq, Message)]
struct DailyS2c {
    #[prost(message, repeated, tag = "1")]
    us_item_list: Vec<UsDailyItem>,
    #[prost(string, optional, tag = "3")]
    next_key: Option<String>,
}

#[derive(Clone, PartialEq, Message)]
struct UsDailyItem {
    #[prost(int64, optional, tag = "1")]
    timestamp: Option<i64>,
    #[prost(string, optional, tag = "2")]
    timestamp_str: Option<String>,
    #[prost(uint64, optional, tag = "3")]
    total_shares_short: Option<u64>,
    #[prost(double, optional, tag = "6")]
    short_percent: Option<f64>,
    #[prost(uint64, optional, tag = "7")]
    volume: Option<u64>,
    #[prost(double, optional, tag = "8")]
    close_price: Option<f64>,
}

#[derive(Clone, PartialEq, Message)]
struct ShortInterestResponse {
    #[prost(int32, optional, tag = "1")]
    ret_type: Option<i32>,
    #[prost(message, optional, tag = "4")]
    s2c: Option<ShortInterestS2c>,
}

#[derive(Clone, PartialEq, Message)]
struct ShortInterestS2c {
    #[prost(string, optional, tag = "3")]
    next_key: Option<String>,
}

#[derive(Clone, PartialEq, Message)]
struct IndicatorListRequest {
    #[prost(message, optional, tag = "1")]
    c2s: Option<IndicatorListC2s>,
}

#[derive(Clone, PartialEq, Message)]
struct IndicatorListC2s {
    #[prost(string, optional, tag = "1")]
    search_key: Option<String>,
    #[prost(int32, optional, tag = "2")]
    lang_type: Option<i32>,
    #[prost(int32, optional, tag = "3")]
    search_mode: Option<i32>,
}

#[derive(Clone, PartialEq, Message)]
struct IndicatorListResponse {
    #[prost(int32, optional, tag = "1")]
    ret_type: Option<i32>,
    #[prost(message, optional, tag = "4")]
    s2c: Option<IndicatorListS2c>,
}

#[derive(Clone, PartialEq, Message)]
struct IndicatorListS2c {
    #[prost(message, repeated, tag = "1")]
    indicator_list: Vec<IndicatorEntry>,
}

#[derive(Clone, PartialEq, Message)]
struct IndicatorEntry {
    #[prost(message, optional, tag = "1")]
    my_lang: Option<IndicatorInfo>,
}

#[derive(Clone, PartialEq, Message)]
struct IndicatorInfo {
    #[prost(string, optional, tag = "1")]
    short_name: Option<String>,
    #[prost(string, optional, tag = "2")]
    full_name: Option<String>,
    #[prost(string, optional, tag = "5")]
    script: Option<String>,
}

#[derive(Clone, PartialEq, Message)]
struct IndicatorCalcRequest {
    #[prost(message, optional, tag = "1")]
    c2s: Option<IndicatorCalcC2s>,
}

#[derive(Clone, PartialEq, Message)]
struct IndicatorCalcC2s {
    #[prost(string, tag = "1")]
    short_name: String,
    #[prost(int32, tag = "2")]
    lang_type: i32,
    #[prost(message, optional, tag = "3")]
    data: Option<IndicatorCalcData>,
    #[prost(int32, optional, tag = "4")]
    num: Option<i32>,
    #[prost(message, repeated, tag = "5")]
    inputs: Vec<IndicatorInputItem>,
}

#[derive(Clone, PartialEq, Message)]
struct IndicatorCalcData {
    #[prost(message, optional, tag = "1")]
    security: Option<WireSecurity>,
    #[prost(int32, tag = "2")]
    kl_type: i32,
    #[prost(message, repeated, tag = "3")]
    k_line: Vec<WireKline>,
}

#[derive(Clone, PartialEq, Message)]
struct WireKline {
    #[prost(string, tag = "1")]
    time: String,
    #[prost(bool, tag = "2")]
    is_blank: bool,
    #[prost(double, optional, tag = "6")]
    close_price: Option<f64>,
}

#[derive(Clone, PartialEq, Message)]
struct IndicatorInputItem {
    #[prost(int32, tag = "1")]
    index: i32,
    #[prost(string, optional, tag = "2")]
    value: Option<String>,
}

#[derive(Clone, PartialEq, Message)]
struct IndicatorCalcResponse {
    #[prost(int32, optional, tag = "1")]
    ret_type: Option<i32>,
    #[prost(message, optional, tag = "4")]
    s2c: Option<IndicatorCalcS2c>,
}

#[derive(Clone, PartialEq, Message)]
struct IndicatorCalcS2c {
    #[prost(string, tag = "1")]
    calc_id: String,
}

#[derive(Clone, PartialEq, Message)]
struct InstitutionRequest {
    #[prost(message, optional, tag = "1")]
    c2s: Option<InstitutionC2s>,
}

#[derive(Clone, PartialEq, Message)]
struct InstitutionC2s {
    #[prost(int32, tag = "1")]
    market: i32,
    #[prost(int32, optional, tag = "2")]
    sort_field: Option<i32>,
    #[prost(int32, optional, tag = "3")]
    sort_dir: Option<i32>,
    #[prost(int32, optional, tag = "4")]
    count: Option<i32>,
    #[prost(string, optional, tag = "5")]
    page: Option<String>,
    #[prost(string, optional, tag = "6")]
    name_part: Option<String>,
}

#[derive(Clone, PartialEq, Message)]
struct InstitutionResponse {
    #[prost(int32, optional, tag = "1")]
    ret_type: Option<i32>,
    #[prost(message, optional, tag = "4")]
    s2c: Option<InstitutionS2c>,
}

#[derive(Clone, PartialEq, Message)]
struct InstitutionS2c {
    #[prost(message, repeated, tag = "1")]
    data_list: Vec<InstitutionItem>,
    #[prost(int32, optional, tag = "2")]
    all_count: Option<i32>,
    #[prost(string, optional, tag = "3")]
    next_page: Option<String>,
    #[prost(string, optional, tag = "4")]
    currency: Option<String>,
}

#[derive(Clone, PartialEq, Message)]
struct InstitutionItem {
    #[prost(int32, tag = "1")]
    institution_id: i32,
    #[prost(string, optional, tag = "2")]
    institution_name: Option<String>,
    #[prost(double, optional, tag = "3")]
    position_value: Option<f64>,
    #[prost(int32, optional, tag = "5")]
    position_count: Option<i32>,
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

fn close(coordinator: &Arc<Mutex<OpenDSessionCoordinator>>) {
    coordinator
        .lock()
        .expect("coordinator lock")
        .close()
        .expect("close");
}

#[test]
fn short_interest_reader_preserves_wire_identity_and_projects_rows() {
    let (address, task) = server(|stream, request| {
        assert_eq!(request.header.proto_id, DAILY_SHORT_VOLUME);
        let protocol = request.header.proto_id;
        let serial = request.header.serial_no;
        let request = ShortRequest::decode(request.body.as_slice()).expect("short request");
        let c2s = request.c2s.expect("c2s");
        let security = c2s.security.expect("security");
        assert_eq!(security.market, 11);
        assert_eq!(security.code, "AAPL");
        assert_eq!(c2s.next_key.as_deref(), Some("cursor"));
        assert_eq!(c2s.num, Some(7));
        write_response(
            stream,
            protocol,
            serial,
            DailyResponse {
                ret_type: Some(0),
                s2c: Some(DailyS2c {
                    us_item_list: vec![UsDailyItem {
                        timestamp: Some(1_700_000_000),
                        timestamp_str: Some("2023-11-14".to_owned()),
                        total_shares_short: Some(12_000),
                        short_percent: Some(12.5),
                        volume: Some(80_000),
                        close_price: Some(101.25),
                    }],
                    next_key: Some("-1".to_owned()),
                }),
            }
            .encode_to_vec(),
        );
    });
    let coordinator = make_coordinator(address, Duration::from_secs(1));
    let reader = OpenDShortInterestReader::new(Arc::clone(&coordinator));
    let result = reader
        .query(&ShortInterestQuery {
            market: 11,
            code: " aapl ".to_owned(),
            operation: ShortInterestOperation::DailyVolume,
            next_key: Some("cursor".to_owned()),
            limit: 7,
        })
        .expect("short volume result");
    assert_eq!(result.security.instrument_id, "US.AAPL");
    assert_eq!(result.items.len(), 1);
    assert_eq!(result.items[0].shares_short, Some(12_000));
    assert_eq!(result.next_key, None);
    close(&coordinator);
    task.join().expect("server");
}

#[test]
fn technical_and_institution_readers_use_their_protocol_ids() {
    let (address, task) = server(|stream, request| {
        assert_eq!(request.header.proto_id, INDICATOR_LIST);
        let protocol = request.header.proto_id;
        let serial = request.header.serial_no;
        let request =
            IndicatorListRequest::decode(request.body.as_slice()).expect("indicator request");
        let c2s = request.c2s.expect("indicator c2s");
        assert_eq!(c2s.search_key.as_deref(), Some("rsi"));
        assert_eq!(c2s.lang_type, Some(2));
        write_response(
            stream,
            protocol,
            serial,
            IndicatorListResponse {
                ret_type: Some(0),
                s2c: Some(IndicatorListS2c {
                    indicator_list: vec![IndicatorEntry {
                        my_lang: Some(IndicatorInfo {
                            short_name: Some("RSI".to_owned()),
                            full_name: Some("Relative Strength Index".to_owned()),
                            script: Some("rsi(close, 14)".to_owned()),
                        }),
                    }],
                }),
            }
            .encode_to_vec(),
        );
    });
    let coordinator = make_coordinator(address, Duration::from_secs(1));
    let reader = FutuTechnicalIndicatorReader::new(Arc::clone(&coordinator));
    let result = reader
        .list(&IndicatorListQuery {
            search_key: Some("rsi".to_owned()),
            lang_type: Some(2),
            search_mode: Some(1),
        })
        .expect("indicator list");
    assert_eq!(result.indicators.len(), 1);
    assert_eq!(
        result.indicators[0]
            .my_lang
            .as_ref()
            .and_then(|v| v.short_name.as_deref()),
        Some("RSI")
    );
    close(&coordinator);
    task.join().expect("server");

    let (address, task) = server(|stream, request| {
        assert_eq!(request.header.proto_id, INDICATOR_CALC);
        let protocol = request.header.proto_id;
        let serial = request.header.serial_no;
        let request =
            IndicatorCalcRequest::decode(request.body.as_slice()).expect("indicator calc request");
        let c2s = request.c2s.expect("indicator calc c2s");
        assert_eq!(c2s.short_name, "RSI");
        assert_eq!(c2s.lang_type, 2);
        assert_eq!(c2s.num, Some(1));
        let data = c2s.data.expect("indicator calc data");
        let security = data.security.expect("indicator calc security");
        assert_eq!(security.market, 11);
        assert_eq!(security.code, "AAPL");
        assert_eq!(data.kl_type, 2);
        assert_eq!(data.k_line.len(), 1);
        assert_eq!(c2s.inputs[0].index, 0);
        assert_eq!(c2s.inputs[0].value.as_deref(), Some("14"));
        write_response(
            stream,
            protocol,
            serial,
            IndicatorCalcResponse {
                ret_type: Some(0),
                s2c: Some(IndicatorCalcS2c {
                    calc_id: "calc-7".to_owned(),
                }),
            }
            .encode_to_vec(),
        );
    });
    let coordinator = make_coordinator(address, Duration::from_secs(1));
    let reader = FutuTechnicalIndicatorReader::new(Arc::clone(&coordinator));
    let result = reader
        .calculate(&IndicatorCalcQuery {
            short_name: " RSI ".to_owned(),
            lang_type: 2,
            market: 11,
            code: "aapl".to_owned(),
            kl_type: 2,
            k_line: vec![IndicatorKline {
                time: "2023-11-14".to_owned(),
                is_blank: false,
                high_price: None,
                open_price: None,
                low_price: None,
                close_price: Some(100.0),
                last_close_price: None,
                volume: Some(1),
                turnover: None,
                turnover_rate: None,
                pe: None,
                change_rate: None,
                timestamp: None,
                hp_volume: None,
            }],
            num: Some(1),
            inputs: vec![FutuIndicatorInput {
                index: 0,
                value: Some("14".to_owned()),
            }],
        })
        .expect("indicator calculation");
    assert_eq!(result.calc_id, "calc-7");
    close(&coordinator);
    task.join().expect("server");

    let (address, task) = server(|stream, request| {
        assert_eq!(request.header.proto_id, INSTITUTION_LIST);
        let protocol = request.header.proto_id;
        let serial = request.header.serial_no;
        let request =
            InstitutionRequest::decode(request.body.as_slice()).expect("institution request");
        let c2s = request.c2s.expect("institution c2s");
        assert_eq!(c2s.market, 11);
        assert_eq!(c2s.count, Some(2));
        assert_eq!(c2s.name_part.as_deref(), Some("fund"));
        write_response(
            stream,
            protocol,
            serial,
            InstitutionResponse {
                ret_type: Some(0),
                s2c: Some(InstitutionS2c {
                    data_list: vec![InstitutionItem {
                        institution_id: 42,
                        institution_name: Some("Fund".to_owned()),
                        position_value: Some(1_000_000.0),
                        position_count: Some(3),
                    }],
                    all_count: Some(1),
                    next_page: Some("-1".to_owned()),
                    currency: Some("USD".to_owned()),
                }),
            }
            .encode_to_vec(),
        );
    });
    let coordinator = make_coordinator(address, Duration::from_secs(1));
    let reader = OpenDInstitutionReader::new(Arc::clone(&coordinator));
    let result = reader
        .query(&InstitutionQuery {
            operation: InstitutionOperation::List,
            market: 11,
            count: Some(2),
            name_part: Some("fund".to_owned()),
            ..InstitutionQuery::default()
        })
        .expect("institution list");
    assert_eq!(result.entries[0].institution_id, Some(42));
    assert_eq!(result.currency.as_deref(), Some("USD"));
    close(&coordinator);
    task.join().expect("server");
}

#[test]
fn advanced_readers_fail_closed_on_rejection_missing_body_timeout_and_eof() {
    let (address, task) = server(|stream, request| {
        assert_eq!(request.header.proto_id, SHORT_INTEREST);
        write_response(
            stream,
            request.header.proto_id,
            request.header.serial_no,
            ErrorResponse {
                ret_type: Some(-1),
                ret_msg: Some("permission denied".to_owned()),
                err_code: Some(403),
            }
            .encode_to_vec(),
        );
    });
    let coordinator = make_coordinator(address, Duration::from_secs(1));
    let reader = OpenDShortInterestReader::new(Arc::clone(&coordinator));
    let error = reader
        .query(&ShortInterestQuery {
            market: 11,
            code: "AAPL".to_owned(),
            ..ShortInterestQuery::default()
        })
        .expect_err("rejection");
    assert!(error.to_string().contains("permission denied"));
    close(&coordinator);
    task.join().expect("server");

    let (address, task) = server(|stream, request| {
        assert_eq!(request.header.proto_id, INDICATOR_LIST);
        write_response(
            stream,
            request.header.proto_id,
            request.header.serial_no,
            ErrorResponse {
                ret_type: Some(0),
                ret_msg: None,
                err_code: None,
            }
            .encode_to_vec(),
        );
    });
    let coordinator = make_coordinator(address, Duration::from_secs(1));
    let reader = FutuTechnicalIndicatorReader::new(Arc::clone(&coordinator));
    let error = reader
        .list(&IndicatorListQuery::default())
        .expect_err("missing s2c");
    assert!(error.to_string().contains("missing s2c"));
    close(&coordinator);
    task.join().expect("server");

    let (address, task) = server(|_stream, _request| thread::sleep(Duration::from_millis(100)));
    let coordinator = make_coordinator(address, Duration::from_millis(20));
    let reader = OpenDShortInterestReader::new(Arc::clone(&coordinator));
    let error = reader
        .query(&ShortInterestQuery {
            market: 11,
            code: "AAPL".to_owned(),
            ..ShortInterestQuery::default()
        })
        .expect_err("timeout");
    assert!(error.to_string().contains("timed out"));
    close(&coordinator);
    task.join().expect("server");

    let (address, task) = server(|_stream, _request| {});
    let coordinator = make_coordinator(address, Duration::from_secs(1));
    let reader = OpenDShortInterestReader::new(Arc::clone(&coordinator));
    let error = reader
        .query(&ShortInterestQuery {
            market: 11,
            code: "AAPL".to_owned(),
            ..ShortInterestQuery::default()
        })
        .expect_err("eof");
    assert!(matches!(
        error,
        jftrade_integration_futu::ShortInterestQueryError::Session(_)
    ));
    close(&coordinator);
    task.join().expect("server");
}

#[test]
fn short_interest_reader_uses_new_generation_after_peer_close() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let task = thread::spawn(move || {
        let (mut first, _) = listener.accept().expect("first accept");
        let init = read_frame(&mut first);
        assert_eq!(init.header.proto_id, INIT_CONNECT);
        write_response(
            &mut first,
            init.header.proto_id,
            init.header.serial_no,
            InitResponse {
                ret_type: Some(0),
                s2c: Some(InitState { conn_id: 1 }),
            }
            .encode_to_vec(),
        );
        let request = read_frame(&mut first);
        assert_eq!(request.header.proto_id, SHORT_INTEREST);
        drop(first);

        let (mut second, _) = listener.accept().expect("reconnect accept");
        let init = read_frame(&mut second);
        assert_eq!(init.header.proto_id, INIT_CONNECT);
        write_response(
            &mut second,
            init.header.proto_id,
            init.header.serial_no,
            InitResponse {
                ret_type: Some(0),
                s2c: Some(InitState { conn_id: 2 }),
            }
            .encode_to_vec(),
        );
        let request = read_frame(&mut second);
        assert_eq!(request.header.proto_id, SHORT_INTEREST);
        write_response(
            &mut second,
            request.header.proto_id,
            request.header.serial_no,
            ShortInterestResponse {
                ret_type: Some(0),
                s2c: Some(ShortInterestS2c {
                    next_key: Some("-1".to_owned()),
                }),
            }
            .encode_to_vec(),
        );
    });

    let coordinator = make_coordinator(address, Duration::from_secs(1));
    let reader = OpenDShortInterestReader::new(Arc::clone(&coordinator));
    let first_error = reader
        .query(&ShortInterestQuery {
            market: 11,
            code: "AAPL".to_owned(),
            ..ShortInterestQuery::default()
        })
        .expect_err("peer close");
    assert!(first_error.to_string().contains("closed"));
    let now: jftrade_kernel::WireTimestamp = "2026-09-01T00:00:00Z".parse().expect("timestamp");
    let outcome = coordinator
        .lock()
        .expect("coordinator lock")
        .poll_once(now, Duration::from_secs(1))
        .expect("reconnect");
    assert!(matches!(
        outcome,
        OpenDSessionCoordinatorOutcome::Reconnected {
            generation: 1,
            reason: OpenDSessionCloseReason::PeerClosed
        }
    ));
    let result = reader
        .query(&ShortInterestQuery {
            market: 11,
            code: "AAPL".to_owned(),
            ..ShortInterestQuery::default()
        })
        .expect("post-reconnect query");
    assert_eq!(result.next_key, None);
    close(&coordinator);
    task.join().expect("server");
}
