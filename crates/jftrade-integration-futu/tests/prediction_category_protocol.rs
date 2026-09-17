//! Framed OpenD coverage for the prediction category reader (`:22`).
//!
//! The Go loopback test
//! `pkg/futu/advanced_product_adapter_contracts_test.go:22` exercises the
//! generic customization read through
//! `Qot_GetEventContractCategory` (3434). This test keeps the same wire
//! boundary live: request framing, serial routing and the typed projection all
//! run against a loopback TCP session.

use std::io::{Read, Write};
use std::net::{SocketAddr, TcpListener, TcpStream};
use std::sync::{Arc, Mutex};
use std::thread::{self, JoinHandle};
use std::time::Duration;

use jftrade_integration_futu::{
    Frame, OpenDPredictionMarketReader, OpenDSessionCoordinator, OpenDTcpProbeConfig,
    PredictionComboQuotePort, PredictionMarketReadPort, decode_frame, encode_frame,
};
use jftrade_marketdata::MarketDataRuntimeRecorder;
use prost::Message;

const INIT_CONNECT: u32 = 1001;
const CATEGORY: u32 = 3434;
const COMBO_RFQ: u32 = 3454;

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

fn make_reader(address: SocketAddr) -> OpenDPredictionMarketReader {
    let coordinator = Arc::new(Mutex::new(
        OpenDSessionCoordinator::connect(
            OpenDTcpProbeConfig::new(address, Duration::from_secs(1)),
            Arc::new(MarketDataRuntimeRecorder::default()),
            Vec::new(),
            0,
        )
        .expect("managed OpenD coordinator"),
    ));
    OpenDPredictionMarketReader::new(coordinator)
}

#[test]
fn prediction_category_read_encodes_protocol_and_projects_entries() {
    use jftrade_integration_futu::trade_proto::qot_get_event_contract_category as wire;

    let (address, task) = server(move |stream, request| {
        assert_eq!(request.header.proto_id, CATEGORY);
        let decoded = wire::Request::decode(request.body.as_slice()).expect("category request");
        assert_eq!(decoded.c2s.category.as_deref(), Some("SPORTS"));
        write_response(
            stream,
            request.header.proto_id,
            request.header.serial_no,
            wire::Response {
                ret_type: 0,
                ret_msg: None,
                err_code: None,
                s2c: Some(wire::S2c {
                    category_list: vec![wire::CategoryItem {
                        category: "SPORTS".to_owned(),
                        category_name: Some("体育".to_owned()),
                        tags: vec!["BASEBALL".to_owned(), "FOOTBALL".to_owned()],
                    }],
                }),
            }
            .encode_to_vec(),
        );
    });
    let reader = make_reader(address);
    let value = reader
        .read(
            "/api/v1/market-data/prediction/categories",
            "operation=categories&category=SPORTS",
        )
        .expect("category read");
    assert_eq!(value["entries"][0]["category"], "SPORTS");
    assert_eq!(value["entries"][0]["categoryName"], "体育");
    assert_eq!(value["entries"][0]["tags"][0], "BASEBALL");
    task.join().expect("server");
}

/// Parity: go:452dea11:pkg/futu/adapter_advanced_test.go:124
/// TestAdvancedProtocolReplaySafetyDefaultsToNoReplay
///
/// `Qot_GetEventContractComboRfq` keeps its `Get` prefix but creates a
/// short-lived quote, so Go routes it through `withClient` instead of
/// `withRetryingClient`: when the first response outcome is unknown the request
/// must never be duplicated. Rust's owner is the typed combo-quote port, which
/// performs exactly one framed call; the server below fails the first request
/// and would serve a second one if the port retried.
#[test]
fn combo_rfq_creates_the_quote_once_and_never_replays_after_a_transport_failure() {
    use jftrade_integration_futu::trade_proto::qot_get_event_contract_combo_rfq as wire;

    let calls = Arc::new(std::sync::atomic::AtomicUsize::new(0));
    let observed = Arc::clone(&calls);
    let (address, task) = server(move |stream, request| {
        assert_eq!(request.header.proto_id, COMBO_RFQ);
        let decoded = wire::Request::decode(request.body.as_slice()).expect("combo request");
        let c2s = decoded.c2s;
        assert_eq!(c2s.mvc, "mvc-1");
        assert_eq!(c2s.combo_leg_list.len(), 2);
        observed.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
        // Drop the session without a response: the RFQ outcome is unknown.
        let _ = stream.shutdown(std::net::Shutdown::Both);
    });
    let reader = make_reader(address);
    let error = reader
        .quote(&serde_json::json!({
            "mvc": "mvc-1",
            "legs": [
                {"instrumentId": "US.EC.A", "predictionSide": "YES", "side": "BUY", "ratio": 1},
                {"instrumentId": "US.EC.B", "predictionSide": "NO", "side": "BUY", "ratio": 1},
            ],
        }))
        .expect_err("unknown RFQ outcome must surface");
    assert!(
        matches!(
            error,
            jftrade_integration_futu::PredictionMarketReadError::Transport(_)
                | jftrade_integration_futu::PredictionMarketReadError::Session(_)
        ),
        "unexpected combo RFQ error: {error:?}"
    );
    assert_eq!(
        calls.load(std::sync::atomic::Ordering::SeqCst),
        1,
        "the RFQ must be created exactly once"
    );
    task.join().expect("server");
}
