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
    PredictionComboQuotePort, PredictionMarketReadPort, PredictionMarketSubscriptionPort,
    decode_frame, encode_frame,
};
use jftrade_marketdata::MarketDataRuntimeRecorder;
use prost::Message;

const INIT_CONNECT: u32 = 1001;
const CATEGORY: u32 = 3434;
const COMBO_RFQ: u32 = 3454;
const SUB_EVENT_CONTRACT: u32 = 3455;

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
// Parity: go:452dea11:pkg/futu/advanced_product_adapter_contracts_test.go:22 TestFutuAdvancedSpecializedReadersAndCustomizationSuccess
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

/// Parity: go:452dea11:pkg/futu/adapter_advanced_protocol_test.go:471
/// TestFutuAdvancedProtocolTransportFailureIsReturned.
///
/// Go's loopback server drops protocol 3434 (the prediction category read),
/// and `QueryPredictionMarket(categories)` must surface the transport failure
/// instead of returning an empty success. `Qot_GetEventContractCategory` is a
/// replay-safe `Get` protocol in the allowlist, but the Rust typed reader has
/// no dispatcher that could retry it implicitly, so the request must be issued
/// exactly once and the error must reach the caller.
#[test]
fn prediction_category_transport_failure_is_returned_and_never_retried() {
    let calls = Arc::new(std::sync::atomic::AtomicUsize::new(0));
    let observed = Arc::clone(&calls);
    let (address, task) = server(move |stream, request| {
        assert_eq!(request.header.proto_id, CATEGORY);
        observed.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
        // Drop the session without a response, mirroring `setDropProto(3434)`.
        let _ = stream.shutdown(std::net::Shutdown::Both);
    });
    let reader = make_reader(address);
    let error = reader
        .read(
            "/api/v1/market-data/prediction/categories",
            "operation=categories",
        )
        .expect_err("a dropped category response must surface as an error");
    assert!(
        matches!(
            error,
            jftrade_integration_futu::PredictionMarketReadError::Transport(_)
                | jftrade_integration_futu::PredictionMarketReadError::Session(_)
        ),
        "unexpected prediction category error: {error:?}"
    );
    assert_eq!(
        calls.load(std::sync::atomic::Ordering::SeqCst),
        1,
        "the category read must not be silently retried on the same session"
    );
    task.join().expect("server");
}

/// Parity: go:452dea11:pkg/futu/adapter_advanced_protocol_test.go:15
/// TestFutuAdvancedAdapterReaderSurfaceAndPredictionSubscriptions.
///
/// The Go loopback asserts the prediction subscription lifecycle across a
/// client invalidation: subscribe once, reconnect, then the next call must
/// replay the active subscription (`predictionSubCalls == 2`), and an explicit
/// unsubscribe must issue the final call and clear the active set
/// (`predictionSubCalls == 3`, `predictionSubscriptions` empty). Rust owns the
/// same demand in `OpenDPredictionMarketReader`, fenced on the coordinator's
/// session generation.
#[test]
fn prediction_subscriptions_replay_after_reconnect_and_clear_on_unsubscribe() {
    use jftrade_integration_futu::trade_proto::qot_sub_event_contract as wire;

    let calls = Arc::new(std::sync::atomic::AtomicUsize::new(0));
    let observed = Arc::clone(&calls);
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let task = thread::spawn(move || {
        // First authenticated session: subscribe, then a category read that
        // drops the connection and forces the coordinator onto a new
        // generation.
        let (mut first, _) = listener.accept().expect("first accept");
        let init = read_frame(&mut first);
        assert_eq!(init.header.proto_id, INIT_CONNECT);
        write_response(
            &mut first,
            init.header.proto_id,
            init.header.serial_no,
            InitResponse {
                ret_type: Some(0),
                s2c: Some(InitState {
                    server_ver: 1009,
                    conn_id: 1,
                }),
            }
            .encode_to_vec(),
        );
        let subscribe = read_frame(&mut first);
        assert_eq!(subscribe.header.proto_id, SUB_EVENT_CONTRACT);
        let decoded =
            wire::Request::decode(subscribe.body.as_slice()).expect("subscription request");
        let c2s = decoded.c2s;
        assert!(c2s.is_sub_or_un_sub);
        assert_eq!(c2s.security_list.len(), 1);
        assert_eq!(c2s.security_list[0].code, "EVENT.ONE");
        assert_eq!(c2s.sub_type_list, vec![2, 11, 4]);
        assert_eq!(c2s.kline_source, vec![1]);
        observed.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
        write_response(
            &mut first,
            subscribe.header.proto_id,
            subscribe.header.serial_no,
            wire::Response {
                ret_type: 0,
                ret_msg: None,
                err_code: None,
                s2c: None,
            }
            .encode_to_vec(),
        );
        let category = read_frame(&mut first);
        assert_eq!(category.header.proto_id, CATEGORY);
        drop(first);

        // Second session: the next prediction call must replay the lease
        // before serving the caller's own request.
        let (mut second, _) = listener.accept().expect("reconnect accept");
        let init = read_frame(&mut second);
        assert_eq!(init.header.proto_id, INIT_CONNECT);
        write_response(
            &mut second,
            init.header.proto_id,
            init.header.serial_no,
            InitResponse {
                ret_type: Some(0),
                s2c: Some(InitState {
                    server_ver: 1009,
                    conn_id: 2,
                }),
            }
            .encode_to_vec(),
        );
        let replay = read_frame(&mut second);
        assert_eq!(replay.header.proto_id, SUB_EVENT_CONTRACT);
        let decoded = wire::Request::decode(replay.body.as_slice()).expect("replay request");
        assert!(decoded.c2s.is_sub_or_un_sub);
        observed.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
        write_response(
            &mut second,
            replay.header.proto_id,
            replay.header.serial_no,
            wire::Response {
                ret_type: 0,
                ret_msg: None,
                err_code: None,
                s2c: None,
            }
            .encode_to_vec(),
        );
        let category = read_frame(&mut second);
        assert_eq!(category.header.proto_id, CATEGORY);
        let decoded = jftrade_integration_futu::trade_proto::qot_get_event_contract_category::Request::decode(
            category.body.as_slice(),
        )
        .expect("category request");
        assert_eq!(decoded.c2s.category, None);
        write_response(
            &mut second,
            category.header.proto_id,
            category.header.serial_no,
            jftrade_integration_futu::trade_proto::qot_get_event_contract_category::Response {
                ret_type: 0,
                ret_msg: None,
                err_code: None,
                s2c: Some(
                    jftrade_integration_futu::trade_proto::qot_get_event_contract_category::S2c {
                        category_list: Vec::new(),
                    },
                ),
            }
            .encode_to_vec(),
        );
        let unsubscribe = read_frame(&mut second);
        assert_eq!(unsubscribe.header.proto_id, SUB_EVENT_CONTRACT);
        let decoded =
            wire::Request::decode(unsubscribe.body.as_slice()).expect("unsubscribe request");
        let c2s = decoded.c2s;
        assert!(!c2s.is_sub_or_un_sub);
        assert_eq!(c2s.sub_type_list, Vec::<i32>::new());
        observed.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
        write_response(
            &mut second,
            unsubscribe.header.proto_id,
            unsubscribe.header.serial_no,
            wire::Response {
                ret_type: 0,
                ret_msg: None,
                err_code: None,
                s2c: None,
            }
            .encode_to_vec(),
        );
    });

    let coordinator = Arc::new(Mutex::new(
        OpenDSessionCoordinator::connect(
            OpenDTcpProbeConfig::new(address, Duration::from_secs(1)),
            Arc::new(MarketDataRuntimeRecorder::default()),
            Vec::new(),
            0,
        )
        .expect("managed OpenD coordinator"),
    ));
    let reader = OpenDPredictionMarketReader::new(Arc::clone(&coordinator));
    reader
        .subscribe(
            "US.EVENT.ONE",
            &[
                "ORDER_BOOK".to_owned(),
                "KLINE".to_owned(),
                "TICKER".to_owned(),
            ],
        )
        .expect("first prediction subscribe");
    assert_eq!(calls.load(std::sync::atomic::Ordering::SeqCst), 1);

    // The first post-close read observes the dropped session. Go hides this
    // behind `withRetryingClient`; Rust's runtime owns reconnect cadence, so
    // the coordinator is polled explicitly before the next read.
    let error = reader
        .read(
            "/api/v1/market-data/prediction/categories",
            "operation=categories",
        )
        .expect_err("the dropped session must surface before reconnect");
    assert!(matches!(
        error,
        jftrade_integration_futu::PredictionMarketReadError::Transport(_)
            | jftrade_integration_futu::PredictionMarketReadError::Session(_)
    ));
    let now: jftrade_kernel::WireTimestamp = "2026-09-01T00:00:00Z".parse().expect("timestamp");
    let outcome = coordinator
        .lock()
        .expect("coordinator lock")
        .poll_once(now, Duration::from_secs(1))
        .expect("reconnect");
    assert!(matches!(
        outcome,
        jftrade_integration_futu::OpenDSessionCoordinatorOutcome::Reconnected { .. }
    ));

    reader
        .read(
            "/api/v1/market-data/prediction/categories",
            "operation=categories",
        )
        .expect("post-reconnect category read");
    assert_eq!(
        calls.load(std::sync::atomic::Ordering::SeqCst),
        2,
        "the active prediction subscription must be replayed after reconnect"
    );

    reader
        .unsubscribe("US.EVENT.ONE")
        .expect("unsubscribe prediction");
    assert_eq!(
        calls.load(std::sync::atomic::Ordering::SeqCst),
        3,
        "unsubscribe must issue exactly one final call"
    );
    coordinator
        .lock()
        .expect("coordinator lock")
        .close()
        .expect("close");
    task.join().expect("server");
}
