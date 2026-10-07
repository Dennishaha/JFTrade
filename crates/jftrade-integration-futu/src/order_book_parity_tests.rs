//! Original order-book wire samples through the production framed session.
use super::*;
use crate::{
    MarketMicrostructureError, MarketMicrostructureOperation, MarketMicrostructureReadPort,
    OpenDMarketMicrostructureReader, OpenDSessionCoordinator, OpenDSessionCoordinatorOutcome,
    OpenDTcpProbeConfig,
};
use jftrade_kernel::WireTimestamp;
use jftrade_marketdata::MarketDataRuntimeRecorder;
use serde_json::json;
use std::io::{Read, Write};
use std::net::{SocketAddr, TcpListener, TcpStream};
use std::sync::{Arc, Mutex};
use std::thread::JoinHandle;
use std::time::Duration;

fn varint(mut value: u64) -> Vec<u8> {
    let mut bytes = Vec::new();
    loop {
        let byte = (value & 0x7f) as u8;
        value >>= 7;
        bytes.push(byte | if value == 0 { 0 } else { 0x80 });
        if value == 0 {
            return bytes;
        }
    }
}

fn integer(field: u32, value: u64) -> Vec<u8> {
    let mut bytes = varint(u64::from(field) << 3);
    bytes.extend(varint(value));
    bytes
}

fn bytes(field: u32, value: &[u8]) -> Vec<u8> {
    let mut bytes = varint((u64::from(field) << 3) | 2);
    bytes.extend(varint(value.len() as u64));
    bytes.extend(value);
    bytes
}

fn double(field: u32, value: f64) -> Vec<u8> {
    let mut bytes = varint((u64::from(field) << 3) | 1);
    bytes.extend(value.to_le_bytes());
    bytes
}

fn security(market: u64, code: &str) -> Vec<u8> {
    [integer(1, market), bytes(2, code.as_bytes())].concat()
}

fn level(price: f64, volume: u64, count: u64) -> Vec<u8> {
    [double(1, price), integer(2, volume), integer(3, count)].concat()
}

fn original_nvda_s2c() -> Vec<u8> {
    let time = b"2026-06-01 08:24:35.732";
    [
        bytes(1, &security(11, "NVDA")),
        bytes(2, &level(215.82, 34, 0)),
        bytes(3, &level(215.86, 204, 0)),
        bytes(4, time),
        double(5, 1748766275.732),
        bytes(6, time),
        double(7, 1748766275.732),
        bytes(8, "英伟达".as_bytes()),
    ]
    .concat()
}

fn success(s2c: &[u8]) -> Vec<u8> {
    [integer(1, 0), bytes(2, b""), integer(3, 0), bytes(4, s2c)].concat()
}

struct Fixture {
    address: SocketAddr,
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
    calls: Arc<Mutex<Vec<crate::Frame>>>,
    server: Option<JoinHandle<()>>,
}

impl Fixture {
    fn new(protocol: u32, response: Vec<u8>, push: Option<Vec<u8>>) -> Self {
        let listener = TcpListener::bind("127.0.0.1:0").expect("listener");
        let address = listener.local_addr().expect("address");
        let calls = Arc::new(Mutex::new(Vec::new()));
        let recorded = Arc::clone(&calls);
        let server = std::thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept");
            stream
                .set_read_timeout(Some(Duration::from_secs(3)))
                .expect("read bound");
            stream
                .set_write_timeout(Some(Duration::from_secs(3)))
                .expect("write bound");
            let init = crate::transport::read_framed_frame(&mut stream).expect("init");
            assert_eq!(init.header.proto_id, crate::PROTO_INIT_CONNECT);
            let state = [integer(1, 1009), integer(3, 7)].concat();
            Self::reply(&mut stream, &init, &success(&state));
            let request = crate::transport::read_framed_frame(&mut stream).expect("request");
            assert_eq!(request.header.proto_id, protocol);
            if let Some(push) = push {
                stream
                    .write_all(
                        &crate::encode_frame(PROTO_UPDATE_ORDER_BOOK, 0, &push)
                            .expect("push frame"),
                    )
                    .expect("push");
            }
            recorded.lock().expect("calls").push(request.clone());
            Self::reply(&mut stream, &request, &response);
            let mut byte = [0];
            assert_eq!(stream.read(&mut byte).expect("client shutdown"), 0);
        });
        let coordinator = match OpenDSessionCoordinator::connect(
            OpenDTcpProbeConfig::new(address, Duration::from_secs(2)),
            Arc::new(MarketDataRuntimeRecorder::default()),
            Vec::new(),
            0,
        ) {
            Ok(coordinator) => Arc::new(Mutex::new(coordinator)),
            Err(error) => {
                let _ = TcpStream::connect_timeout(&address, Duration::from_millis(100));
                let _ = server.join();
                panic!("fixture handshake failed: {error}");
            }
        };
        Self {
            address,
            coordinator,
            calls,
            server: Some(server),
        }
    }

    fn reply(stream: &mut TcpStream, request: &crate::Frame, body: &[u8]) {
        stream
            .write_all(
                &crate::encode_frame(request.header.proto_id, request.header.serial_no, body)
                    .expect("reply frame"),
            )
            .expect("reply");
    }

    fn depth(&self) -> Result<serde_json::Value, MarketMicrostructureError> {
        OpenDMarketMicrostructureReader::new(Arc::clone(&self.coordinator)).query(
            MarketMicrostructureOperation::Depth,
            "US.NVDA",
            &json!({"num":10}),
        )
    }

    fn finish(mut self) {
        self.coordinator
            .lock()
            .expect("coordinator")
            .close()
            .expect("close");
        self.server
            .take()
            .expect("server")
            .join()
            .expect("fixture assertions");
    }
}

impl Drop for Fixture {
    fn drop(&mut self) {
        if let Some(server) = self.server.take() {
            if let Ok(mut coordinator) = self.coordinator.lock() {
                let _ = coordinator.close();
            }
            // Also unblock accept if setup failed before the managed connection.
            let _ = TcpStream::connect_timeout(&self.address, Duration::from_millis(100));
            let result = server.join();
            if !std::thread::panicking() {
                result.expect("fixture assertions");
            }
        }
    }
}

// Parity: go:452dea11:pkg/futu/opend/orderbook_test.go:13 TestParseOrderBookResponseCurrentOpenDWireLayout
#[test]
fn order_book_original_nvda_wire_projects_name_times_and_both_sides() {
    use crate::trade_proto::qot_get_order_book::{PROTOCOL_ID, Request, Response};
    let raw = success(&original_nvda_s2c());
    let decoded = Response::decode(raw.as_slice())
        .expect("wire")
        .s2c
        .expect("s2c");
    assert_eq!(decoded.security.market, 11);
    assert_eq!(decoded.security.code, "NVDA");
    assert_eq!(decoded.order_book_bid_list[0].volume, 204);
    assert_eq!(decoded.order_book_ask_list[0].volume, 34);
    let fixture = Fixture::new(PROTOCOL_ID, raw, None);
    let result = fixture.depth().expect("depth");
    let depth = &result["depth"];
    assert_eq!(depth["name"], "英伟达");
    assert_eq!(depth["svrRecvTimeBid"], "2026-06-01 08:24:35.732");
    assert_eq!(depth["svrRecvTimeAsk"], "2026-06-01 08:24:35.732");
    assert_eq!(depth["bids"].as_array().expect("bids").len(), 1);
    assert_eq!(depth["bids"][0]["price"], 215.86);
    assert_eq!(depth["bids"][0]["volume"].as_f64(), Some(204.0));
    assert_eq!(depth["asks"].as_array().expect("asks").len(), 1);
    assert_eq!(depth["asks"][0]["price"], 215.82);
    assert_eq!(depth["asks"][0]["volume"].as_f64(), Some(34.0));
    let calls = fixture.calls.lock().expect("calls");
    assert_eq!(calls.len(), 1);
    let request = Request::decode(calls[0].body.as_slice()).expect("request");
    assert_eq!(request.c2s.security.market, 11);
    assert_eq!(request.c2s.security.code, "NVDA");
    assert_eq!(request.c2s.num, 10);
    drop(calls);
    fixture.finish();
}

// Parity: go:452dea11:pkg/futu/opend/orderbook_test.go:94 TestParseOrderBookResponseError
#[test]
fn order_book_rejection_preserves_original_codes_with_rust_error_format() {
    let raw = [integer(1, 1), bytes(2, b"bad order book"), integer(3, 321)].concat();
    let fixture = Fixture::new(
        crate::trade_proto::qot_get_order_book::PROTOCOL_ID,
        raw,
        None,
    );
    let error = fixture.depth().expect_err("rejected depth");
    assert!(
        matches!(&error, MarketMicrostructureError::Rejected { operation: "Qot_GetOrderBook", ret_type: 1, err_code: 321, message } if message == "bad order book")
    );
    assert_eq!(
        error.to_string(),
        "OpenD Qot_GetOrderBook request rejected retType=1 errCode=321: bad order book"
    );
    assert_ne!(
        error.to_string(),
        "opend Qot_GetOrderBook retType=1 errCode=321 retMsg=bad order book",
        "original display difference remains partial"
    );
    fixture.finish();
}

// Parity: go:452dea11:pkg/futu/opend/proto_v108_contract_test.go:13 TestProto108OrderBookFieldNumbers
#[test]
fn order_book_v108_get_and_push_lock_each_original_field_number() {
    use crate::trade_proto::qot_get_order_book::{C2s, S2c};
    let raw_security = security(11, "NVDA");
    let request = C2s {
        security: crate::trade_proto::qot_common::Security {
            market: 11,
            code: "NVDA".into(),
        },
        num: 10,
        order_book_type: Some(2),
        header: None,
    };
    assert_eq!(
        request.encode_to_vec(),
        [bytes(1, &raw_security), integer(2, 10), integer(4, 2)].concat(),
        "request orderBookType is field 4"
    );

    // Distinct values prevent swapped sides, times, or optional fields from
    // passing a round trip through the same incorrectly numbered encoder.
    let fields = [
        bytes(1, &raw_security),
        bytes(2, &level(215.82, 34, 0)),
        bytes(3, &level(215.86, 204, 1)),
        bytes(4, b"bid-time"),
        double(5, 1.25),
        bytes(6, b"ask-time"),
        double(7, 2.5),
        bytes(8, "英伟达".as_bytes()),
        integer(9, 2),
    ]
    .concat();
    let get = S2c::decode(fields.as_slice()).expect("get wire");
    assert_eq!(get.order_book_ask_list[0].price, 215.82);
    assert_eq!(get.order_book_bid_list[0].price, 215.86);
    assert_eq!(get.svr_recv_time_bid.as_deref(), Some("bid-time"));
    assert_eq!(get.svr_recv_time_ask.as_deref(), Some("ask-time"));
    assert_eq!(get.name.as_deref(), Some("英伟达"));
    assert_eq!(get.order_book_type, Some(2));
    assert_eq!(
        get.encode_to_vec(),
        fields,
        "get response fields 2/3/4/6/8/9"
    );
    let push = OrderBookS2c::decode(fields.as_slice()).expect("push wire");
    assert_eq!(push.asks[0].price, Some(215.82));
    assert_eq!(push.bids[0].price, Some(215.86));
    assert_eq!(push.server_receive_time_bid.as_deref(), Some("bid-time"));
    assert_eq!(push.server_receive_time_ask.as_deref(), Some("ask-time"));
    assert_eq!(push.name.as_deref(), Some("英伟达"));
    assert_eq!(push.order_book_type, Some(2));
    assert_eq!(
        push.encode_to_vec(),
        fields,
        "push response fields 2/3/4/6/8/9"
    );
}

// Parity: go:452dea11:pkg/futu/opend/trading_reads_contracts_test.go:343 TestSubscribeOrderBookDispatchesSuccessfulPushes
#[test]
fn order_book_unsolicited_push_dispatches_while_global_state_rpc_completes() {
    let push = success(
        &[
            bytes(1, &security(1, "00700")),
            bytes(3, &level(321.5, 500, 1)),
        ]
        .concat(),
    );
    let response = success(
        &[
            integer(1, 3),
            integer(2, 8),
            integer(3, 6),
            integer(4, 6),
            integer(5, 2),
            integer(6, 1),
            integer(7, 1),
            integer(8, 900),
            integer(9, 5008),
            integer(10, 1717000000),
        ]
        .concat(),
    );
    let fixture = Fixture::new(crate::PROTO_GET_GLOBAL_STATE, response, Some(push));
    let mut coordinator = fixture.coordinator.lock().expect("coordinator");
    let probe = crate::OpenDTcpProbe::probe_initialized(coordinator.session().expect("session"))
        .expect("original global state RPC decodes successfully");
    assert_eq!(probe.server_version.as_deref(), Some("9.0.5008"));
    {
        let calls = fixture.calls.lock().expect("calls");
        assert_eq!(calls.len(), 1);
        assert_eq!(calls[0].body, bytes(1, &integer(1, 0)));
    }
    let outcome = coordinator
        .poll_once(
            "2026-06-01T00:00:01Z"
                .parse::<WireTimestamp>()
                .expect("time"),
            Duration::from_secs(1),
        )
        .expect("dispatch");
    let OpenDSessionCoordinatorOutcome::Push(QuotePush::OrderBook(book)) = outcome else {
        panic!("expected dispatched order book, got {outcome:?}");
    };
    assert_eq!(
        book.security,
        Some(Security {
            market: Some(1),
            code: Some("00700".into())
        })
    );
    assert_eq!(book.bids.len(), 1);
    assert_eq!(book.bids[0].price, Some(321.5));
    assert_eq!(book.bids[0].volume, Some(500));
    assert_eq!(book.bids[0].order_count, Some(1));
    assert_eq!(
        coordinator
            .poll_once(
                "2026-06-01T00:00:02Z"
                    .parse::<WireTimestamp>()
                    .expect("time"),
                Duration::from_millis(10)
            )
            .expect("no second push"),
        OpenDSessionCoordinatorOutcome::Idle
    );
    drop(coordinator);
    fixture.finish();
}
