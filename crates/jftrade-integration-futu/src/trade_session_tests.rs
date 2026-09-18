use std::io::{Read, Write};
use std::net::TcpListener;
use std::sync::Arc;
use std::thread;
use std::time::Duration;

use prost::Message;

use super::*;
use crate::TradeProtocol;
use crate::{decode_frame, encode_frame};

fn read_frame(stream: &mut std::net::TcpStream) -> crate::Frame {
    let mut header = [0_u8; crate::frame::HEADER_LEN];
    stream.read_exact(&mut header).expect("frame header");
    let body_len = u32::from_le_bytes(header[12..16].try_into().expect("length")) as usize;
    let mut packet = Vec::from(header);
    let mut body = vec![0_u8; body_len];
    stream.read_exact(&mut body).expect("frame body");
    packet.extend(body);
    decode_frame(&packet).expect("decoded frame")
}

#[test]
fn account_list_call_uses_protocol_serial_and_typed_response() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        assert_eq!(request.header.proto_id, trd_get_acc_list::PROTOCOL_ID);
        let decoded = trd_get_acc_list::Request::decode(request.body.as_slice()).expect("request");
        assert_eq!(decoded.c2s.user_id, 7);
        let response = trd_get_acc_list::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_get_acc_list::S2c { acc_list: vec![] }),
        };
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 1).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    let payload = client
        .get_account_list(trd_get_acc_list::Request {
            c2s: trd_get_acc_list::C2s {
                user_id: 7,
                trd_category: None,
                need_general_sec_account: None,
            },
        })
        .expect("account list");
    assert!(payload.acc_list.is_empty());
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn cash_flow_read_encodes_header_and_projects_neutral_snapshot() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        assert_eq!(request.header.proto_id, trd_flow_summary::PROTOCOL_ID);
        let decoded = trd_flow_summary::Request::decode(request.body.as_slice()).expect("request");
        assert_eq!(decoded.c2s.header.acc_id, 42);
        assert_eq!(decoded.c2s.header.trd_market, 2);
        assert_eq!(decoded.c2s.clearing_date, "2026-08-21");
        assert_eq!(decoded.c2s.cash_flow_direction, Some(1));
        let response = trd_flow_summary::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_flow_summary::S2c {
                header: trade_header(1, 42, 2).into(),
                flow_summary_info_list: vec![
                    trd_flow_summary::FlowSummaryInfo {
                        clearing_date: Some("2026-08-21".to_owned()),
                        cash_flow_direction: Some(1),
                        cash_flow_amount: Some(88.8),
                        cash_flow_id: Some(7),
                        ..Default::default()
                    },
                    trd_flow_summary::FlowSummaryInfo {
                        clearing_date: Some("2026-08-21".to_owned()),
                        cash_flow_direction: Some(2),
                        cash_flow_amount: Some(1.2),
                        cash_flow_id: Some(8),
                        ..Default::default()
                    },
                ],
            }),
        };
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 5).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    let flows = client
        .read_cash_flows(trade_header(1, 42, 2), "2026-08-21".to_owned(), Some(1))
        .expect("cash flows");
    assert_eq!(flows.len(), 2);
    assert_eq!(flows[0].header.acc_id, 42);
    assert_eq!(flows[0].cash_flow_id, Some(8));
    assert_eq!(flows[1].cash_flow_amount, Some(88.8));
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn return_code_is_exposed_as_typed_trade_response_error() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        let response = trd_get_acc_list::Response {
            ret_type: -1,
            ret_msg: Some("account unavailable".to_owned()),
            err_code: Some(1101),
            s2c: None,
        };
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 3).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    let result = client.get_account_list(trd_get_acc_list::Request {
        c2s: trd_get_acc_list::C2s {
            user_id: 0,
            trd_category: None,
            need_general_sec_account: None,
        },
    });
    assert!(matches!(
        result,
        Err(TradeSessionError::Response(ResponseError::ReturnCode {
            ret_type: -1,
            err_code: 1101,
            ..
        }))
    ));
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn request_timeout_is_preserved_from_managed_session() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let _request = read_frame(&mut stream);
        thread::sleep(Duration::from_millis(100));
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(25), 4).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    let result = client.get_account_list(trd_get_acc_list::Request {
        c2s: trd_get_acc_list::C2s {
            user_id: 0,
            trd_category: None,
            need_general_sec_account: None,
        },
    });
    assert!(matches!(
        result,
        Err(TradeSessionError::Session(
            OpenDManagedSessionError::RequestTimeout { protocol: 2001, .. }
        ))
    ));
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn calls_after_session_close_surface_closed_error() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (_stream, _) = listener.accept().expect("accept");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 2).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    session.close().expect("close");
    let result = client.call(trd_get_acc_list::PROTOCOL_ID, &[]);
    assert!(matches!(result, Err(TradeSessionError::Session(_))));
    server.join().expect("server");
}

#[test]
fn history_order_call_uses_history_protocol_and_forwards_filters() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        assert_eq!(
            request.header.proto_id,
            TradeProtocol::GetHistoryOrderList.id()
        );
        let decoded =
            trd_get_order_list::Request::decode(request.body.as_slice()).expect("request");
        let filter = decoded.c2s.filter_conditions.expect("filter");
        assert_eq!(filter.code_list, vec!["US.AAPL"]);
        assert_eq!(filter.begin_time.as_deref(), Some("2026-08-01 00:00:00"));
        assert_eq!(filter.end_time.as_deref(), Some("2026-08-02 00:00:00"));
        assert_eq!(decoded.c2s.filter_status_list, vec![5, 10]);
        let response = trd_get_order_list::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_get_order_list::S2c {
                header: trade_header(0, 42, 2).into(),
                order_list: vec![],
            }),
        };
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 7).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    let orders = client
        .read_history_orders(
            trade_header(0, 42, 2),
            Some(TradeFilter {
                code_list: vec!["US.AAPL".to_owned()],
                begin_time: Some("2026-08-01 00:00:00".to_owned()),
                end_time: Some("2026-08-02 00:00:00".to_owned()),
                ..TradeFilter::default()
            }),
            vec![5, 10],
            None,
        )
        .expect("orders");
    assert!(orders.is_empty());
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn history_fill_call_uses_history_protocol_and_forwards_time_filter() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        assert_eq!(
            request.header.proto_id,
            TradeProtocol::GetHistoryOrderFillList.id()
        );
        let decoded =
            trd_get_order_fill_list::Request::decode(request.body.as_slice()).expect("request");
        let filter = decoded.c2s.filter_conditions.expect("filter");
        assert_eq!(filter.code_list, vec!["HK.00700"]);
        assert_eq!(filter.begin_time.as_deref(), Some("2026-08-01 00:00:00"));
        let response = trd_get_order_fill_list::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_get_order_fill_list::S2c {
                header: trade_header(0, 42, 1).into(),
                order_fill_list: vec![],
            }),
        };
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 8).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    let fills = client
        .read_history_fills(
            trade_header(0, 42, 1),
            Some(TradeFilter {
                code_list: vec!["HK.00700".to_owned()],
                begin_time: Some("2026-08-01 00:00:00".to_owned()),
                ..TradeFilter::default()
            }),
            None,
        )
        .expect("fills");
    assert!(fills.is_empty());
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn read_accounts_projects_a_framed_response_without_exposing_proto_types() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        assert_eq!(request.header.proto_id, trd_get_acc_list::PROTOCOL_ID);
        let decoded = trd_get_acc_list::Request::decode(request.body.as_slice()).expect("request");
        assert_eq!(decoded.c2s.user_id, 7);
        let response = trd_get_acc_list::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_get_acc_list::S2c {
                acc_list: vec![trd_common::TrdAcc {
                    trd_env: 1,
                    acc_id: 42,
                    trd_market_auth_list: vec![1, 11],
                    card_num: Some("card".to_owned()),
                    ..Default::default()
                }],
            }),
        };
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 5).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    let accounts = client
        .read_accounts(7, Some(1), Some(true))
        .expect("accounts");
    assert_eq!(accounts.len(), 1);
    assert_eq!(accounts[0].acc_id, 42);
    assert_eq!(accounts[0].trd_market_auth_list, vec![1, 11]);
    assert_eq!(accounts[0].card_num.as_deref(), Some("card"));
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn read_funds_preserves_framed_return_code_error() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        assert_eq!(request.header.proto_id, trd_get_funds::PROTOCOL_ID);
        let response = trd_get_funds::Response {
            ret_type: -1,
            ret_msg: Some("trade login required".to_owned()),
            err_code: Some(2002),
            s2c: None,
        };
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 6).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    let result = client.read_funds(trade_header(1, 42, 1), None, None, None);
    assert!(matches!(
        result,
        Err(TradeSessionError::Response(ResponseError::ReturnCode {
            ret_type: -1,
            err_code: 2002,
            ..
        }))
    ));
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn max_trade_quantity_call_preserves_serial_and_projects_optional_fields() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        assert_eq!(request.header.proto_id, trd_get_max_trd_qtys::PROTOCOL_ID);
        let decoded =
            trd_get_max_trd_qtys::Request::decode(request.body.as_slice()).expect("request");
        assert_eq!(decoded.c2s.code, "AAPL");
        assert_eq!(decoded.c2s.order_type, 1);
        assert_eq!(decoded.c2s.price, 188.5);
        assert_eq!(decoded.c2s.adjust_price, Some(true));
        assert_eq!(decoded.c2s.adjust_side_and_limit, Some(0.015));
        assert_eq!(decoded.c2s.session, Some(1));
        let response = trd_get_max_trd_qtys::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_get_max_trd_qtys::S2c {
                header: trade_header(1, 42, 2).into(),
                max_trd_qtys: Some(trd_common::MaxTrdQtys {
                    max_cash_buy: 100.0,
                    max_cash_and_margin_buy: Some(200.0),
                    max_position_sell: 50.0,
                    max_sell_short: Some(300.0),
                    max_buy_back: None,
                    long_required_im: Some(10.0),
                    short_required_im: None,
                    session: Some(1),
                }),
            }),
        };
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 7).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    let snapshot = client
        .read_max_trade_quantity(TradeMaxTradeQuantityRequest {
            header: trade_header(1, 42, 2),
            order_type: 1,
            code: "AAPL".to_owned(),
            price: 188.5,
            order_id: None,
            adjust_price: Some(true),
            adjust_side_and_limit: Some(0.015),
            sec_market: Some(2),
            order_id_ex: Some("OID-1".to_owned()),
            session: Some(1),
            position_id: None,
        })
        .expect("snapshot");
    assert_eq!(snapshot.max_cash_buy, 100.0);
    assert_eq!(snapshot.max_cash_and_margin_buy, Some(200.0));
    assert_eq!(snapshot.max_sell_short, Some(300.0));
    assert_eq!(snapshot.session, Some(1));
    session.close().expect("close");
    server.join().expect("server");
}

fn place_request(price: f64) -> TradePlaceOrderRequest {
    TradePlaceOrderRequest {
        header: trade_header(0, 77_001, 1),
        trd_side: 1,
        order_type: 2,
        code: "00700".to_owned(),
        quantity: 100.0,
        price: Some(price),
        remark: None,
        time_in_force: None,
        fill_outside_rth: None,
        aux_price: None,
        trail_type: None,
        trail_value: None,
        trail_spread: None,
        session: None,
        position_id: None,
        expire_time: None,
        amount: None,
        prediction_side: None,
        sec_market: None,
    }
}

#[test]
fn place_order_encodes_packet_conn_id_and_projects_server_order_identity() {
    // Parity: go:452dea11:pkg/futu/opend/trading_methods_test.go:83 TestPlaceOrderAndModifyOrderEncodeTradeWrites
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        assert_eq!(request.header.proto_id, trd_place_order::PROTOCOL_ID);
        let decoded = trd_place_order::Request::decode(request.body.as_slice()).expect("request");
        // Rust wires the InitConnect connID into the anti-replay packet id.
        assert_eq!(decoded.c2s.packet_id.conn_id, 42);
        assert!(decoded.c2s.packet_id.serial_no >= 1);
        assert_eq!(decoded.c2s.code, "00700");
        assert_eq!(decoded.c2s.qty, 100.0);
        assert_eq!(decoded.c2s.price, Some(321.5));
        let response = trd_place_order::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_place_order::S2c {
                header: trade_header(0, 77_001, 1).into(),
                order_id: Some(9001),
                order_id_ex: Some("server-order-1".to_owned()),
            }),
        };
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
        let mut byte = [0_u8; 1];
        let _ = stream.read(&mut byte);
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 4).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session_with_conn_id(Arc::clone(&session), 42);
    let result = client
        .place_order(place_request(321.5))
        .expect("place order");
    assert_eq!(result.order_id, Some(9001));
    assert_eq!(result.order_id_ex.as_deref(), Some("server-order-1"));
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn place_order_rounds_us_prices_to_the_venue_tick_before_encoding() {
    // Parity: go:452dea11:pkg/futu/exchange_trade_price_test.go:25 and
    // go:452dea11:pkg/futu/exchange_trade_write.go:177
    // TestNormalizeSubmitOrderPrice / placeOrderRequestFromSubmitOrder. The
    // write path must round US prices to the venue tick before encoding
    // Trd_PlaceOrder: cents at or above one dollar, $0.0001 below it. Each price
    // is normalized against its own value, HK keeps the caller's price, and a
    // zero price must not become a wire field.
    struct PriceCase {
        sec_market: i32,
        price: Option<f64>,
        aux: Option<f64>,
        expected_price: Option<f64>,
        expected_aux: Option<f64>,
    }
    let cases = [
        // sec_market, requested price, requested aux, expected price, expected aux
        PriceCase {
            sec_market: 2,
            price: Some(123.456),
            aux: Some(12.3456),
            expected_price: Some(123.46),
            expected_aux: Some(12.35),
        },
        PriceCase {
            sec_market: 2,
            price: Some(0.12345),
            aux: Some(0.99994),
            expected_price: Some(0.1235),
            expected_aux: Some(0.9999),
        },
        PriceCase {
            sec_market: 1,
            price: Some(320.123),
            aux: Some(12.3456),
            expected_price: Some(320.123),
            expected_aux: Some(12.3456),
        },
        PriceCase {
            sec_market: 2,
            price: Some(0.0),
            aux: Some(12.3456),
            expected_price: None,
            expected_aux: Some(12.35),
        },
    ];
    for case in cases {
        let sec_market = case.sec_market;
        let price = case.price;
        let aux = case.aux;
        let expected_price = case.expected_price;
        let expected_aux = case.expected_aux;
        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
        let address = listener.local_addr().expect("address");
        let requested_price = price;
        let requested_aux = aux;
        let server = thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept");
            let request = read_frame(&mut stream);
            let decoded =
                trd_place_order::Request::decode(request.body.as_slice()).expect("request");
            assert_eq!(decoded.c2s.price, expected_price, "wire price");
            assert_eq!(decoded.c2s.aux_price, expected_aux, "wire aux price");
            assert_eq!(decoded.c2s.session, Some(1), "RTH session is forwarded");
            assert_eq!(
                decoded.c2s.fill_outside_rth,
                Some(true),
                "fillOutsideRTH is forwarded for US orders"
            );
            let response = trd_place_order::Response {
                ret_type: 0,
                ret_msg: None,
                err_code: None,
                s2c: Some(trd_place_order::S2c {
                    header: trade_header(0, 77_001, 1).into(),
                    order_id: Some(9001),
                    order_id_ex: None,
                }),
            };
            stream
                .write_all(
                    &encode_frame(
                        request.header.proto_id,
                        request.header.serial_no,
                        &response.encode_to_vec(),
                    )
                    .expect("response"),
                )
                .expect("write response");
            let mut byte = [0_u8; 1];
            let _ = stream.read(&mut byte);
        });
        let session = Arc::new(
            OpenDManagedSession::connect(address, Duration::from_millis(500), 4).expect("session"),
        );
        let client =
            OpenDTradeReadClient::from_managed_session_with_conn_id(Arc::clone(&session), 42);
        let mut request = place_request(requested_price.unwrap_or_default());
        request.price = requested_price;
        request.aux_price = requested_aux;
        request.sec_market = Some(sec_market);
        // Go's `placeOrderRequestFromSubmitOrder` forwards the caller's
        // US session and RTH flag verbatim onto `Trd_PlaceOrder`. Assert them
        // on the wire so the normalization does not silently drop them.
        request.session = Some(1);
        request.fill_outside_rth = Some(true);
        client.place_order(request).expect("place order");
        session.close().expect("close");
        server.join().expect("server");
    }
}

#[test]
fn event_contract_place_order_encodes_amount_and_prediction_side() {
    // Parity: go:452dea11:pkg/futu/advanced_product_adapter_contracts_test.go:403
    // TestFutuTradeProductRequestAndReadLifecycleBranches. Go's
    // placeOrderRequestFromSubmitOrder forwards the event amount and maps the
    // YES prediction side to PredSide_Yes (1) on the wire.
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        assert_eq!(request.header.proto_id, trd_place_order::PROTOCOL_ID);
        let decoded = trd_place_order::Request::decode(request.body.as_slice()).expect("request");
        assert_eq!(decoded.c2s.amount, Some(20.0));
        assert_eq!(decoded.c2s.pred_side, Some(1));
        let response = trd_place_order::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_place_order::S2c {
                header: trade_header(0, 77_001, 1).into(),
                order_id: Some(9002),
                order_id_ex: Some("server-event-1".to_owned()),
            }),
        };
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
        let mut byte = [0_u8; 1];
        let _ = stream.read(&mut byte);
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 31).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session_with_conn_id(Arc::clone(&session), 42);
    let mut request = place_request(0.6);
    request.code = "US.EVENT".to_owned();
    request.quantity = 1.0;
    request.amount = Some(20.0);
    request.prediction_side = Some(1);
    let result = client
        .place_order(request)
        .expect("event contract place order");
    assert_eq!(result.order_id_ex.as_deref(), Some("server-event-1"));
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn modify_order_encodes_packet_conn_id_and_returns_server_identity() {
    // Parity: go:452dea11:pkg/futu/opend/trading_methods_test.go:83 TestPlaceOrderAndModifyOrderEncodeTradeWrites
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        assert_eq!(request.header.proto_id, trd_modify_order::PROTOCOL_ID);
        let decoded = trd_modify_order::Request::decode(request.body.as_slice()).expect("request");
        assert_eq!(decoded.c2s.packet_id.conn_id, 42);
        assert_eq!(decoded.c2s.order_id, 9001);
        assert_eq!(decoded.c2s.modify_order_op, 1);
        assert_eq!(decoded.c2s.qty, Some(50.0));
        assert_eq!(decoded.c2s.price, Some(322.0));
        let response = trd_modify_order::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_modify_order::S2c {
                header: trade_header(0, 77_001, 1).into(),
                order_id: 9001,
                order_id_ex: Some("server-order-1".to_owned()),
            }),
        };
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 5).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session_with_conn_id(Arc::clone(&session), 42);
    let result = client
        .modify_order(TradeModifyOrderRequest {
            header: trade_header(0, 77_001, 1),
            order_id: 9001,
            operation: 1,
            for_all: None,
            trd_market: None,
            quantity: Some(50.0),
            price: Some(322.0),
            adjust_price: None,
            adjust_side_and_limit: None,
            aux_price: None,
            trail_type: None,
            trail_value: None,
            trail_spread: None,
            order_id_ex: None,
        })
        .expect("modify order");
    assert_eq!(result.order_id, Some(9001));
    assert_eq!(result.order_id_ex.as_deref(), Some("server-order-1"));
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn modify_order_returns_stable_identity_for_an_empty_success_payload() {
    // Parity: go:452dea11:pkg/futu/opend/trading_write_boundaries_test.go:82 TestModifyOrderReturnsStableEmptyResult
    //
    // Go returns an empty but non-nil S2C when the broker omits order ids.
    // Rust must not invent an id: the typed result keeps the zero identity
    // instead of failing or defaulting to a fabricated value.
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        let response = trd_modify_order::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_modify_order::S2c {
                header: trade_header(0, 77_001, 1).into(),
                order_id: 0,
                order_id_ex: None,
            }),
        };
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 6).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session_with_conn_id(Arc::clone(&session), 42);
    let result = client
        .modify_order(TradeModifyOrderRequest {
            header: trade_header(0, 77_001, 1),
            order_id: 9001,
            operation: 1,
            for_all: None,
            trd_market: None,
            quantity: None,
            price: None,
            adjust_price: None,
            adjust_side_and_limit: None,
            aux_price: None,
            trail_type: None,
            trail_value: None,
            trail_spread: None,
            order_id_ex: None,
        })
        .expect("modify order");
    assert_eq!(result.order_id, Some(0));
    assert_eq!(result.order_id_ex, None);
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn place_order_propagates_opend_business_rejection() {
    // Parity: go:452dea11:pkg/futu/opend/trading_write_boundaries_test.go:58 TestPlaceOrderPropagatesOpenDBusinessRejection
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        let response = trd_place_order::Response {
            ret_type: -1,
            ret_msg: Some("buying power insufficient".to_owned()),
            err_code: Some(201),
            s2c: None,
        };
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 7).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session_with_conn_id(Arc::clone(&session), 42);
    let error = client
        .place_order(place_request(321.5))
        .expect_err("rejection must fail closed");
    match error {
        TradeSessionError::Response(ResponseError::ReturnCode {
            ret_type,
            err_code,
            message,
        }) => {
            assert_eq!(ret_type, -1);
            assert_eq!(err_code, 201);
            assert!(message.contains("buying power insufficient"));
        }
        other => panic!("unexpected place-order rejection: {other:?}"),
    }
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn trade_writes_use_the_coordinator_conn_id_and_fail_closed_after_close() {
    // Parity: go:452dea11:pkg/futu/opend/trading_write_boundaries_test.go:17 TestTradeWriteMethodsEnforcePrerequisitesAndDisconnectedState
    //
    // Go rejects trade writes without an InitConnect connID and after the
    // session is disconnected. Rust cannot represent a write client without a
    // connID (it is captured from InitConnect), so the equivalent boundary is
    // that every write path fails closed once the session is closed.
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (stream, _) = listener.accept().expect("accept");
        let mut byte = [0_u8; 1];
        let _ = std::io::Read::read(&mut { stream }, &mut byte);
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 8).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session_with_conn_id(Arc::clone(&session), 42);
    session.close().expect("close");
    assert!(matches!(
        client.place_order(place_request(1.0)),
        Err(TradeSessionError::Session(_))
    ));
    assert!(matches!(
        client.modify_order(TradeModifyOrderRequest {
            header: trade_header(0, 77_001, 1),
            order_id: 1,
            operation: 1,
            for_all: None,
            trd_market: None,
            quantity: None,
            price: None,
            adjust_price: None,
            adjust_side_and_limit: None,
            aux_price: None,
            trail_type: None,
            trail_value: None,
            trail_spread: None,
            order_id_ex: None,
        }),
        Err(TradeSessionError::Session(_))
    ));
    assert!(matches!(
        client.unlock_trade(TradeUnlockRequest {
            unlock: true,
            password_md5: Some("hash".to_owned()),
            security_firm: None,
        }),
        Err(TradeSessionError::Session(_))
    ));
    assert!(matches!(
        client.subscribe_trade_accounts(TradeSubscribeAccountsRequest {
            account_ids: vec![1],
        }),
        Err(TradeSessionError::Session(_))
    ));
    server.join().expect("server");
}

#[test]
fn trading_reads_propagate_opend_business_errors() {
    // Parity: go:452dea11:pkg/futu/opend/trading_error_boundaries_test.go:26 TestTradingReadMethodsPropagateOpenDBusinessErrors
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        let response = trd_get_funds::Response {
            ret_type: -1,
            ret_msg: Some("account not found".to_owned()),
            err_code: Some(5),
            s2c: None,
        };
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 9).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    let error = client
        .read_funds(trade_header(0, 42, 2), None, None, None)
        .expect_err("business rejection must fail closed");
    match error {
        TradeSessionError::Response(ResponseError::ReturnCode {
            ret_type,
            err_code,
            message,
        }) => {
            assert_eq!(ret_type, -1);
            assert_eq!(err_code, 5);
            assert!(message.contains("account not found"));
        }
        other => panic!("unexpected funds rejection: {other:?}"),
    }
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn trading_reads_reject_a_disconnected_session() {
    // Parity: go:452dea11:pkg/futu/opend/trading_error_boundaries_test.go:102 TestTradingReadMethodsRejectDisconnectedSession
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (stream, _) = listener.accept().expect("accept");
        let mut byte = [0_u8; 1];
        let _ = std::io::Read::read(&mut { stream }, &mut byte);
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 10).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    session.close().expect("close");
    for error in [
        client
            .read_funds(trade_header(0, 42, 2), None, None, None)
            .err(),
        client
            .read_positions(
                trade_header(0, 42, 2),
                None,
                None,
                None,
                None,
                None,
                None,
                None,
            )
            .err(),
        client
            .read_orders(trade_header(0, 42, 2), None, vec![], None)
            .err(),
        client
            .read_history_orders(trade_header(0, 42, 2), None, vec![], None)
            .err(),
    ] {
        assert!(
            matches!(error, Some(TradeSessionError::Session(_))),
            "disconnected trade read must fail closed: {error:?}"
        );
    }
    server.join().expect("server");
}

#[test]
fn subscribe_trade_accounts_propagates_opend_rejection() {
    // Parity: go:452dea11:pkg/futu/opend/trading_methods_test.go:334 TestSubscribeAccountPushPropagatesTradeErrors
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        assert_eq!(request.header.proto_id, trd_sub_acc_push::PROTOCOL_ID);
        let decoded = trd_sub_acc_push::Request::decode(request.body.as_slice()).expect("request");
        assert_eq!(decoded.c2s.acc_id_list, vec![1]);
        let response = trd_sub_acc_push::Response {
            ret_type: -1,
            ret_msg: Some("subscribe failed".to_owned()),
            err_code: Some(13),
            s2c: None,
        };
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 11).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    let error = client
        .subscribe_trade_accounts(TradeSubscribeAccountsRequest {
            account_ids: vec![1],
        })
        .expect_err("rejection must fail closed");
    match error {
        TradeSessionError::Response(ResponseError::ReturnCode {
            ret_type,
            err_code,
            message,
        }) => {
            assert_eq!(ret_type, -1);
            assert_eq!(err_code, 13);
            assert!(message.contains("subscribe failed"));
        }
        other => panic!("unexpected subscribe rejection: {other:?}"),
    }
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn subscribe_trade_accounts_forwards_every_account_id() {
    // Parity: go:452dea11:pkg/futu/opend/trading_methods_test.go:244 TestSubscribeAccountPushAndTradePushDecoding
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        assert_eq!(request.header.proto_id, trd_sub_acc_push::PROTOCOL_ID);
        let decoded = trd_sub_acc_push::Request::decode(request.body.as_slice()).expect("request");
        assert_eq!(decoded.c2s.acc_id_list, vec![11, 22]);
        let response = trd_sub_acc_push::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: None,
        };
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 12).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    client
        .subscribe_trade_accounts(TradeSubscribeAccountsRequest {
            account_ids: vec![11, 22],
        })
        .expect("subscribe accounts");
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn history_trading_reads_return_stable_empty_collections() {
    // Parity: go:452dea11:pkg/futu/opend/trading_error_boundaries_test.go:138 TestHistoryTradingReadsReturnStableEmptyCollections
    //
    // An OpenD success response with empty S2C lists yields empty Rust vectors
    // instead of Option/None or a decode error.
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let orders = read_frame(&mut stream);
        let response = trd_get_order_list::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_get_order_list::S2c {
                header: trade_header(0, 42, 2).into(),
                order_list: vec![],
            }),
        };
        stream
            .write_all(
                &encode_frame(
                    orders.header.proto_id,
                    orders.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write orders response");

        let fills = read_frame(&mut stream);
        let response = trd_get_order_fill_list::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_get_order_fill_list::S2c {
                header: trade_header(0, 42, 2).into(),
                order_fill_list: vec![],
            }),
        };
        stream
            .write_all(
                &encode_frame(
                    fills.header.proto_id,
                    fills.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write fills response");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 13).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    assert!(
        client
            .read_history_orders(trade_header(0, 42, 2), None, vec![], None)
            .expect("history orders")
            .is_empty()
    );
    assert!(
        client
            .read_history_fills(trade_header(0, 42, 2), None, None)
            .expect("history fills")
            .is_empty()
    );
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn place_order_requires_an_authenticated_conn_id() {
    // Parity: go:452dea11:pkg/futu/opend/trading_methods_test.go:64 TestPlaceOrderRequiresRequestAndConnID
    //
    // Go rejects a trade write with connID==0. Rust captures connID from
    // InitConnect, so the representable equivalent is that a client built
    // without an authenticated connID cannot emit a valid anti-replay id:
    // the packet id carries conn_id=0 and the server-side identity is what
    // makes this fail closed upstream. This test pins the client's honest
    // projection of that state instead of fabricating a connID.
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        let decoded = trd_place_order::Request::decode(request.body.as_slice()).expect("request");
        assert_eq!(
            decoded.c2s.packet_id.conn_id, 0,
            "an unauthenticated client must not fabricate a connID"
        );
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 14).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    let _ = client.place_order(place_request(1.0));
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn trade_write_methods_enforce_prerequisites_and_disconnected_state() {
    // Parity: go:452dea11:pkg/futu/opend/trading_write_boundaries_test.go:17 TestTradeWriteMethodsEnforcePrerequisitesAndDisconnectedState
    //
    // Every write entry point must fail closed on a disconnected session
    // instead of silently reporting success.
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (stream, _) = listener.accept().expect("accept");
        let mut byte = [0_u8; 1];
        let _ = std::io::Read::read(&mut { stream }, &mut byte);
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 15).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session_with_conn_id(Arc::clone(&session), 42);
    session.close().expect("close");
    assert!(client.place_order(place_request(1.0)).is_err());
    assert!(
        client
            .unlock_trade(TradeUnlockRequest {
                unlock: true,
                password_md5: Some("hash".to_owned()),
                security_firm: None,
            })
            .is_err()
    );
    assert!(
        client
            .subscribe_trade_accounts(TradeSubscribeAccountsRequest {
                account_ids: vec![1],
            })
            .is_err()
    );
    server.join().expect("server");
}

#[test]
fn trade_write_wrappers_surface_call_errors() {
    // Parity: go:452dea11:pkg/futu/opend/trading_methods_test.go:381 TestTradeWriteWrappersSurfaceCallErrors
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (stream, _) = listener.accept().expect("accept");
        let mut byte = [0_u8; 1];
        let _ = std::io::Read::read(&mut { stream }, &mut byte);
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 16).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session_with_conn_id(Arc::clone(&session), 1);
    session.close().expect("close");
    let error = client
        .modify_order(TradeModifyOrderRequest {
            header: trade_header(0, 77_001, 1),
            order_id: 1,
            operation: 1,
            for_all: None,
            trd_market: None,
            quantity: None,
            price: None,
            adjust_price: None,
            adjust_side_and_limit: None,
            aux_price: None,
            trail_type: None,
            trail_value: None,
            trail_spread: None,
            order_id_ex: None,
        })
        .expect_err("closed session must surface the call error");
    assert!(matches!(error, TradeSessionError::Session(_)));
    server.join().expect("server");
}

#[test]
fn unlock_trade_encodes_unlock_flag_and_security_firm() {
    // Parity: go:452dea11:pkg/futu/opend/new_methods_test.go:954 TestUnlockTradeWithSecurityFirm
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let unlock_request = read_frame(&mut stream);
        assert_eq!(
            unlock_request.header.proto_id,
            trd_unlock_trade::PROTOCOL_ID
        );
        let decoded =
            trd_unlock_trade::Request::decode(unlock_request.body.as_slice()).expect("request");
        assert!(decoded.c2s.unlock);
        assert_eq!(decoded.c2s.pwd_md5.as_deref(), Some("dummyMD5"));
        assert_eq!(decoded.c2s.security_firm, Some(1));
        let response = trd_unlock_trade::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: None,
        };
        stream
            .write_all(
                &encode_frame(
                    unlock_request.header.proto_id,
                    unlock_request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
        let mut byte = [0_u8; 1];
        let _ = stream.read(&mut byte);
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 17).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session_with_conn_id(Arc::clone(&session), 42);
    client
        .unlock_trade(TradeUnlockRequest {
            unlock: true,
            password_md5: Some("dummyMD5".to_owned()),
            security_firm: Some(1),
        })
        .expect("unlock trade");
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn unlock_trade_omits_security_firm_when_the_caller_does_not_request_one() {
    // Parity: go:452dea11:pkg/futu/opend/new_methods_test.go:627 TestUnlockTrade
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        assert_eq!(request.header.proto_id, trd_unlock_trade::PROTOCOL_ID);
        let decoded = trd_unlock_trade::Request::decode(request.body.as_slice()).expect("request");
        assert!(decoded.c2s.unlock, "unlock must send unlock=true");
        assert_eq!(decoded.c2s.pwd_md5.as_deref(), Some("dummyMD5"));
        assert_eq!(
            decoded.c2s.security_firm, None,
            "Go passes a nil securityFirm for the plain unlock path"
        );
        let response = trd_unlock_trade::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: None,
        };
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
        let mut byte = [0_u8; 1];
        let _ = stream.read(&mut byte);
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 21).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session_with_conn_id(Arc::clone(&session), 42);
    client
        .unlock_trade(TradeUnlockRequest {
            unlock: true,
            password_md5: Some("dummyMD5".to_owned()),
            security_firm: None,
        })
        .expect("unlock trade");
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn lock_trade_sends_unlock_false_without_a_security_firm() {
    // Parity: go:452dea11:pkg/futu/opend/new_methods_test.go:654 TestLockTrade
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        let decoded = trd_unlock_trade::Request::decode(request.body.as_slice()).expect("request");
        assert!(!decoded.c2s.unlock, "lock must send unlock=false");
        assert_eq!(decoded.c2s.security_firm, None);
        let response = trd_unlock_trade::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: None,
        };
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
        let mut byte = [0_u8; 1];
        let _ = stream.read(&mut byte);
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 18).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session_with_conn_id(Arc::clone(&session), 42);
    client
        .unlock_trade(TradeUnlockRequest {
            unlock: false,
            password_md5: Some(String::new()),
            security_firm: None,
        })
        .expect("lock trade");
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn unlock_trade_propagates_opend_rejection() {
    // Parity: go:452dea11:pkg/futu/opend/new_methods_test.go:734 TestUnlockTradeError
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        let response = trd_unlock_trade::Response {
            ret_type: -1,
            ret_msg: Some("wrong password".to_owned()),
            err_code: Some(9),
            s2c: None,
        };
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 19).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session_with_conn_id(Arc::clone(&session), 42);
    let error = client
        .unlock_trade(TradeUnlockRequest {
            unlock: true,
            password_md5: Some("wrong_pwd".to_owned()),
            security_firm: None,
        })
        .expect_err("unlock rejection must fail closed");
    assert!(matches!(
        error,
        TradeSessionError::Response(ResponseError::ReturnCode {
            ret_type: -1,
            err_code: 9,
            ..
        })
    ));
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn funds_snapshot_projection_preserves_available_and_withdrawable_cash() {
    // Parity: go:452dea11:pkg/futu/exchange_test.go:322
    // TestQueryAccountBalancesUsesOpenDFundsSnapshot
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        assert_eq!(request.header.proto_id, trd_get_funds::PROTOCOL_ID);
        let response = trd_get_funds::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_get_funds::S2c {
                header: trade_header(1, 1001, 1).into(),
                funds: Some(trd_common::Funds {
                    cash: 10_000.0,
                    frozen_cash: 800.0,
                    avl_withdrawal_cash: 9_200.0,
                    cash_info_list: vec![trd_common::AccCashInfo {
                        currency: Some(1),
                        cash: Some(10_000.0),
                        available_balance: Some(9_200.0),
                        net_cash_power: Some(15_000.0),
                    }],
                    ..Default::default()
                }),
            }),
        };
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 21).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    let funds = client
        .read_funds(trade_header(1, 1001, 1), None, None, None)
        .expect("funds snapshot");
    assert_eq!(funds.funds.cash, 10_000.0);
    assert_eq!(funds.funds.avl_withdrawal_cash, 9_200.0);
    assert_eq!(
        funds.funds.cash_info_list[0].available_balance,
        Some(9_200.0)
    );
    assert_eq!(funds.funds.cash_info_list[0].net_cash_power, Some(15_000.0));
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn funds_read_maps_full_margin_pdt_and_exposure_proto_fields() {
    // Parity: go:452dea11:pkg/futu/adapter_new_methods_test.go:257
    // TestBrokerFundsSnapshotFromProtoFullMargin.
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        assert_eq!(request.header.proto_id, trd_get_funds::PROTOCOL_ID);
        let response = trd_get_funds::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_get_funds::S2c {
                header: trade_header(1, 12345, 1).into(),
                funds: Some(trd_common::Funds {
                    power: 200_000.0,
                    total_assets: 500_000.0,
                    cash: 100_000.0,
                    market_val: 350_000.0,
                    frozen_cash: 10_000.0,
                    debt_cash: 50_000.0,
                    avl_withdrawal_cash: 80_000.0,
                    max_power_short: Some(100_000.0),
                    net_cash_power: Some(120_000.0),
                    long_mv: Some(350_000.0),
                    short_mv: Some(0.0),
                    max_withdrawal: Some(150_000.0),
                    initial_margin: Some(50_000.0),
                    maintenance_margin: Some(25_000.0),
                    margin_call_margin: Some(15_000.0),
                    risk_status: Some(1),
                    securities_assets: Some(300_000.0),
                    fund_assets: Some(50_000.0),
                    bond_assets: Some(0.0),
                    is_pdt: Some(true),
                    pdt_seq: Some("3/3".to_owned()),
                    beginning_dtbp: Some(100_000.0),
                    remaining_dtbp: Some(75_000.0),
                    dt_call_amount: Some(5_000.0),
                    dt_status: Some(1),
                    exposure_level: Some(1),
                    exposure_limit: Some(2_000_000.0),
                    used_limit: Some(800_000.0),
                    remaining_limit: Some(1_200_000.0),
                    ..Default::default()
                }),
            }),
        };
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 22).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    let funds = client
        .read_funds(trade_header(1, 12345, 1), None, None, None)
        .expect("funds snapshot");
    assert_eq!(funds.funds.power, 200_000.0);
    assert_eq!(funds.funds.debt_cash, 50_000.0);
    assert_eq!(funds.funds.max_power_short, Some(100_000.0));
    assert_eq!(funds.funds.initial_margin, Some(50_000.0));
    assert_eq!(funds.funds.maintenance_margin, Some(25_000.0));
    assert_eq!(funds.funds.margin_call_margin, Some(15_000.0));
    assert_eq!(funds.funds.risk_status, Some(1));
    assert_eq!(funds.funds.is_pdt, Some(true));
    assert_eq!(funds.funds.pdt_seq.as_deref(), Some("3/3"));
    assert_eq!(funds.funds.beginning_dtbp, Some(100_000.0));
    assert_eq!(funds.funds.remaining_dtbp, Some(75_000.0));
    assert_eq!(funds.funds.dt_call_amount, Some(5_000.0));
    assert_eq!(funds.funds.dt_status, Some(1));
    assert_eq!(funds.funds.exposure_level, Some(1));
    assert_eq!(funds.funds.exposure_limit, Some(2_000_000.0));
    assert_eq!(funds.funds.used_limit, Some(800_000.0));
    assert_eq!(funds.funds.remaining_limit, Some(1_200_000.0));
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn margin_ratio_read_projects_permit_fee_and_tier_ratios() {
    // Parity: go:452dea11:pkg/futu/exchange_test.go:609
    // TestQueryBrokerMarginRatiosReturnsMarginData
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        assert_eq!(request.header.proto_id, trd_get_margin_ratio::PROTOCOL_ID);
        let response = trd_get_margin_ratio::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_get_margin_ratio::S2c {
                header: trade_header(1, 1001, 1).into(),
                margin_ratio_info_list: vec![trd_get_margin_ratio::MarginRatioInfo {
                    security: crate::trade_proto::qot_common::Security {
                        market: 1,
                        code: "00700".to_owned(),
                    },
                    is_long_permit: Some(true),
                    is_short_permit: Some(false),
                    short_pool_remain: None,
                    short_fee_rate: Some(1.25),
                    alert_long_ratio: Some(0.3),
                    alert_short_ratio: Some(0.4),
                    im_long_ratio: Some(0.5),
                    im_short_ratio: None,
                    mcm_long_ratio: Some(0.6),
                    mcm_short_ratio: None,
                    mm_long_ratio: Some(0.7),
                    mm_short_ratio: None,
                }],
            }),
        };
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 22).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    let ratios = client
        .read_margin_ratios(
            trade_header(1, 1001, 1),
            vec![TradeSecurity {
                market: 1,
                code: "00700".to_owned(),
            }],
        )
        .expect("margin ratios");
    assert_eq!(ratios.len(), 1);
    assert_eq!(ratios[0].symbol, "HK.00700");
    assert_eq!(ratios[0].is_long_permit, Some(true));
    assert_eq!(ratios[0].short_fee_rate, Some(1.25));
    assert_eq!(ratios[0].initial_margin_long_ratio, Some(0.5));
    assert_eq!(ratios[0].margin_call_long_ratio, Some(0.6));
    assert_eq!(ratios[0].maintenance_long_ratio, Some(0.7));
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn margin_ratio_with_an_invalid_security_market_keeps_an_empty_symbol() {
    // Parity: go:452dea11:pkg/futu/trade_helpers_boundary_test.go:68
    // TestTradeProtoConversionSkipsNilFeeAndInvalidMarginSecurity. Go's
    // `brokerMarginRatioSnapshotFromProto` resolves the security market through
    // the quote enum and leaves the symbol empty when the market code is not
    // usable (`Market: -1` in the fixture), instead of fabricating "BAD".
    //
    // Two Go-only halves are boundary conclusions rather than gaps:
    // `FeeList: []*OrderFeeItem{nil, ...}` cannot occur in Rust because prost
    // decodes `repeated` messages into values that are never nil, and the
    // request side is stricter: Rust rejects an unusable `security.market`
    // before any OpenD call (Go only drops the label during projection).
    let header: TradeHeader = trade_header(1, 1001, 1);
    let ratios = crate::trade_snapshots::margin_ratios_projection(
        crate::trade_proto::trd_get_margin_ratio::S2c {
            header: header.clone().into(),
            margin_ratio_info_list: vec![
                crate::trade_proto::trd_get_margin_ratio::MarginRatioInfo {
                    security: crate::trade_proto::qot_common::Security {
                        market: -1,
                        code: "BAD".to_owned(),
                    },
                    is_long_permit: Some(true),
                    is_short_permit: None,
                    short_pool_remain: None,
                    short_fee_rate: None,
                    alert_long_ratio: None,
                    alert_short_ratio: None,
                    im_long_ratio: None,
                    im_short_ratio: None,
                    mcm_long_ratio: None,
                    mcm_short_ratio: None,
                    mm_long_ratio: None,
                    mm_short_ratio: None,
                },
            ],
        },
    );
    assert_eq!(ratios.len(), 1);
    assert_eq!(
        ratios[0].symbol, "",
        "an unusable market code must not fabricate a symbol"
    );
}

#[test]
fn margin_ratio_response_keeps_a_row_with_an_unusable_security_market() {
    // Boundary: Go's `brokerMarginRatioSnapshotFromProto` keeps the row and
    // only clears the symbol when `futuSymbolFromSecurity` cannot resolve the
    // market; the Rust response validator must therefore not fail the whole
    // read for a single unusable row.
    let header: TradeHeader = trade_header(1, 1001, 1);
    let payload = crate::trade_proto::trd_get_margin_ratio::S2c {
        header: header.into(),
        margin_ratio_info_list: vec![
            crate::trade_proto::trd_get_margin_ratio::MarginRatioInfo {
                security: crate::trade_proto::qot_common::Security {
                    market: -1,
                    code: "BAD".to_owned(),
                },
                is_long_permit: Some(true),
                is_short_permit: None,
                short_pool_remain: None,
                short_fee_rate: None,
                alert_long_ratio: None,
                alert_short_ratio: None,
                im_long_ratio: None,
                im_short_ratio: None,
                mcm_long_ratio: None,
                mcm_short_ratio: None,
                mm_long_ratio: None,
                mm_short_ratio: None,
            },
            crate::trade_proto::trd_get_margin_ratio::MarginRatioInfo {
                security: crate::trade_proto::qot_common::Security {
                    market: 1,
                    code: "00700".to_owned(),
                },
                is_long_permit: Some(false),
                is_short_permit: None,
                short_pool_remain: None,
                short_fee_rate: None,
                alert_long_ratio: None,
                alert_short_ratio: None,
                im_long_ratio: None,
                im_short_ratio: None,
                mcm_long_ratio: None,
                mcm_short_ratio: None,
                mm_long_ratio: None,
                mm_short_ratio: None,
            },
        ],
    };
    crate::trade_proto_margin_ratio_validation::validate_margin_ratio_s2c(
        "GetMarginRatio",
        &payload,
    )
    .expect("an unusable market must not fail the whole read");
    let ratios = crate::trade_snapshots::margin_ratios_projection(payload);
    assert_eq!(ratios.len(), 2, "both rows survive: {ratios:?}");
    // Sorted by symbol, so the empty-symbol row comes first.
    assert_eq!(ratios[0].symbol, "");
    assert_eq!(ratios[1].symbol, "HK.00700");
}

#[test]
fn margin_ratio_server_throttling_maps_to_the_typed_rate_limit() {
    // Parity: go:452dea11:pkg/futu/trade_margin_ratio_boundaries_test.go:13
    // (`isMarginRatioRateLimitedError`) and :86
    // TestBrokerMarginRatioFallsBackToRecentCacheAndSurfacesInputFailures.
    //
    // Go classifies the *server's* rejection text, not only the local governor,
    // so a throttled account still reaches the recent-cache fallback. The
    // engine only consults that fallback for `TradeSessionError::RateLimited`,
    // which makes this mapping load-bearing rather than cosmetic.
    let throttled = [
        "rate limit: too high request frequency",
        "频率太高",
        "每30秒最多10次",
    ];
    for message in throttled {
        let error = ResponseError::ReturnCode {
            ret_type: -1,
            err_code: 9,
            message: message.to_owned(),
        };
        assert!(
            margin_ratio_rate_limited_error(&error),
            "server throttling must be classified: {message}"
        );
    }
    for message in [
        "unknown stock 00700",
        "broker service unavailable",
        "too high price",
        "request rejected",
    ] {
        let error = ResponseError::ReturnCode {
            ret_type: -1,
            err_code: 10,
            message: message.to_owned(),
        };
        assert!(
            !margin_ratio_rate_limited_error(&error),
            "a non-throttling rejection must not become a rate limit: {message}"
        );
    }
}

#[test]
fn margin_ratio_wire_throttling_reaches_the_typed_rate_limit_variant() {
    // Parity: go:452dea11:pkg/futu/trade_margin_ratio_boundaries_test.go:86.
    // The scripted OpenD answers `Ret_GetMarginRatio` with the same rate-limit
    // text Go classifies, so the read must surface `RateLimited` (and therefore
    // become eligible for the engine's recent-cache fallback) instead of a
    // generic response error.
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &trd_get_margin_ratio::Response {
                        ret_type: -1,
                        ret_msg: Some("rate limit: too high request frequency".to_owned()),
                        err_code: Some(9),
                        s2c: None,
                    }
                    .encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write throttled response");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 25).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    let error = client
        .read_margin_ratios(
            trade_header(1, 1001, 1),
            vec![TradeSecurity {
                market: 1,
                code: "00700".to_owned(),
            }],
        )
        .expect_err("a throttled read must not report success");
    assert!(
        matches!(error, TradeSessionError::RateLimited),
        "expected the typed rate limit, got {error:?}"
    );
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn margin_ratio_unknown_stock_code_extraction_matches_go_boundaries() {
    // Parity: go:452dea11:pkg/futu/trade_margin_ratio_boundaries_test.go:13
    // TestMarginRatioRecoveryAndErrorClassificationBoundaries (the
    // `extractUnknownStockCode` table) and :150
    // TestMarginRatioUncachedRecoveryAndConversionBoundaries.
    //
    // Go trims the first field after the marker with the cut-set
    // `"'.,;:()[]{}` and upper-cases the result, so `unknown security 'aapl',`
    // yields AAPL and `未知股票 (00700)` yields 00700. A marker with no code
    // token at all is not classified.
    let cases = [
        (None::<&str>, None::<&str>),
        (Some("other broker error"), None),
        (Some("unknown stock"), None),
        (Some("unknown stock   "), None),
        // Go returns `ok=false` here: the trimmed token is empty.
        (Some("unknown security \"\""), None),
        (Some("unknown stock 00700;"), Some("00700")),
        (Some("未知股票 '00700',"), Some("00700")),
        (Some("unknown security 'aapl',"), Some("AAPL")),
        (Some("unknown security (00700)"), Some("00700")),
        (Some("未知股票 (00700)"), Some("00700")),
        (Some("OpenD: unknown stock AAPL"), Some("AAPL")),
    ];
    for (message, expected) in cases {
        let error = message.map(|message| {
            TradeSessionError::Response(ResponseError::ReturnCode {
                ret_type: -1,
                err_code: 1,
                message: message.to_owned(),
            })
        });
        match (error.as_ref(), expected) {
            (Some(error), Some(expected)) => {
                assert_eq!(
                    unknown_security_code(error).as_deref(),
                    Some(expected),
                    "message={message:?}"
                );
            }
            (Some(error), None) => {
                assert_eq!(
                    unknown_security_code(error),
                    None,
                    "message={message:?} must not classify a code"
                );
            }
            (None, None) => {
                // `nil` must not classify: the helper only runs on a live
                // error, matching Go's nil guard.
            }
            (None, Some(_)) => unreachable!("nil error cannot have an expected code"),
        }
    }
}

#[test]
fn margin_ratio_unknown_stock_recovery_keeps_nil_security_rows_and_code_forms() {
    // Parity: go:452dea11:pkg/futu/trade_margin_ratio_boundaries_test.go:13
    // (the `marginRatioInfoListWithUnknownStockRecovery` empty + recovery
    // assertions) and the `removeUnknownMarginSecurity(nil, ...)` boundary
    // from :150.
    //
    // Go removes the security whose code matches the extracted code
    // case-insensitively, and drops nil entries while doing so. A marker whose
    // code never matches the request must surface the original broker error
    // instead of looping forever.
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let first = read_frame(&mut stream);
        let decoded =
            trd_get_margin_ratio::Request::decode(first.body.as_slice()).expect("request");
        assert_eq!(decoded.c2s.security_list.len(), 2);
        stream
            .write_all(
                &encode_frame(
                    first.header.proto_id,
                    first.header.serial_no,
                    &trd_get_margin_ratio::Response {
                        ret_type: -1,
                        ret_msg: Some("未知股票 (07226)".to_owned()),
                        err_code: Some(1),
                        s2c: None,
                    }
                    .encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write rejection");

        let second = read_frame(&mut stream);
        let decoded =
            trd_get_margin_ratio::Request::decode(second.body.as_slice()).expect("request");
        assert_eq!(decoded.c2s.security_list.len(), 1);
        assert_eq!(decoded.c2s.security_list[0].code, "00700");
        stream
            .write_all(
                &encode_frame(
                    second.header.proto_id,
                    second.header.serial_no,
                    &trd_get_margin_ratio::Response {
                        ret_type: 0,
                        ret_msg: None,
                        err_code: None,
                        s2c: Some(trd_get_margin_ratio::S2c {
                            header: trade_header(1, 1001, 1).into(),
                            margin_ratio_info_list: vec![trd_get_margin_ratio::MarginRatioInfo {
                                security: crate::trade_proto::qot_common::Security {
                                    market: 1,
                                    code: "00700".to_owned(),
                                },
                                is_long_permit: Some(true),
                                is_short_permit: None,
                                ..Default::default()
                            }],
                        }),
                    }
                    .encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write retry response");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 24).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    let ratios = client
        .read_margin_ratios(
            trade_header(1, 1001, 1),
            vec![
                TradeSecurity {
                    market: 1,
                    code: "00700".to_owned(),
                },
                TradeSecurity {
                    market: 1,
                    code: "07226".to_owned(),
                },
            ],
        )
        .expect("margin ratios after parenthesised code recovery");
    assert_eq!(ratios.len(), 1);
    assert_eq!(ratios[0].symbol, "HK.00700");
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn margin_ratio_read_retries_without_unknown_stock_and_keeps_known_rows() {
    // Parity: go:452dea11:pkg/futu/exchange_test.go:654
    // TestQueryBrokerMarginRatiosSkipsUnknownStock
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let first = read_frame(&mut stream);
        let decoded =
            trd_get_margin_ratio::Request::decode(first.body.as_slice()).expect("request");
        assert_eq!(decoded.c2s.security_list.len(), 2);
        let rejected = trd_get_margin_ratio::Response {
            ret_type: -1,
            ret_msg: Some("unknown stock 07226".to_owned()),
            err_code: Some(1),
            s2c: None,
        };
        stream
            .write_all(
                &encode_frame(
                    first.header.proto_id,
                    first.header.serial_no,
                    &rejected.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write rejection");

        let second = read_frame(&mut stream);
        let decoded =
            trd_get_margin_ratio::Request::decode(second.body.as_slice()).expect("request");
        assert_eq!(decoded.c2s.security_list.len(), 1);
        assert_eq!(decoded.c2s.security_list[0].code, "00700");
        let accepted = trd_get_margin_ratio::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_get_margin_ratio::S2c {
                header: trade_header(1, 1001, 1).into(),
                margin_ratio_info_list: vec![trd_get_margin_ratio::MarginRatioInfo {
                    security: crate::trade_proto::qot_common::Security {
                        market: 1,
                        code: "00700".to_owned(),
                    },
                    is_long_permit: Some(true),
                    is_short_permit: Some(false),
                    ..Default::default()
                }],
            }),
        };
        stream
            .write_all(
                &encode_frame(
                    second.header.proto_id,
                    second.header.serial_no,
                    &accepted.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write retry response");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 23).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    let ratios = client
        .read_margin_ratios(
            trade_header(1, 1001, 1),
            vec![
                TradeSecurity {
                    market: 1,
                    code: "00700".to_owned(),
                },
                TradeSecurity {
                    market: 1,
                    code: "07226".to_owned(),
                },
            ],
        )
        .expect("margin ratios after retry");
    assert_eq!(ratios.len(), 1);
    assert_eq!(ratios[0].symbol, "HK.00700");
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn history_order_read_projects_external_id_and_filters_status() {
    // Parity: go:452dea11:pkg/futu/exchange_test.go:452
    // TestQueryBrokerHistoryOrdersReturnsHistoricalOrders
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        assert_eq!(
            request.header.proto_id,
            TradeProtocol::GetHistoryOrderList.id()
        );
        let decoded =
            trd_get_order_list::Request::decode(request.body.as_slice()).expect("request");
        assert_eq!(decoded.c2s.filter_status_list, vec![11]);
        assert_eq!(
            decoded.c2s.filter_conditions.expect("filter").code_list,
            vec!["HK.00700"]
        );
        let response = trd_get_order_list::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_get_order_list::S2c {
                header: trade_header(0, 1001, 1).into(),
                order_list: vec![
                    trd_common::Order {
                        order_id: 2001,
                        order_id_ex: "EXT-2001".to_owned(),
                        code: "HK.00700".to_owned(),
                        name: "Tencent".to_owned(),
                        trd_side: 1,
                        order_type: 1,
                        order_status: 11,
                        qty: 100.0,
                        price: Some(320.0),
                        fill_qty: Some(100.0),
                        fill_avg_price: Some(319.8),
                        create_time: "2026-05-20 09:30:00".to_owned(),
                        update_time: "2026-05-20 09:35:00".to_owned(),
                        trd_market: Some(1),
                        ..Default::default()
                    },
                    trd_common::Order {
                        order_id: 2002,
                        order_id_ex: "EXT-2002".to_owned(),
                        code: "HK.00700".to_owned(),
                        name: "Tencent".to_owned(),
                        trd_side: 2,
                        order_type: 1,
                        order_status: 15,
                        qty: 50.0,
                        price: Some(330.0),
                        create_time: "2026-05-19 09:30:00".to_owned(),
                        update_time: "2026-05-19 09:32:00".to_owned(),
                        trd_market: Some(1),
                        ..Default::default()
                    },
                ],
            }),
        };
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 24).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    let orders = client
        .read_history_orders(
            trade_header(0, 1001, 1),
            Some(TradeFilter {
                code_list: vec!["HK.00700".to_owned()],
                ..TradeFilter::default()
            }),
            vec![11],
            None,
        )
        .expect("history orders");
    assert_eq!(orders.len(), 2);
    assert_eq!(orders[0].order_id_ex, "EXT-2001");
    assert_eq!(orders[0].order_status, 11);
    assert_eq!(orders[1].order_status, 15);
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn history_fill_read_projects_fill_identity() {
    // Parity: go:452dea11:pkg/futu/exchange_test.go:514
    // TestQueryBrokerHistoryOrderFillsReturnsHistoricalFills
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        assert_eq!(
            request.header.proto_id,
            TradeProtocol::GetHistoryOrderFillList.id()
        );
        let decoded =
            trd_get_order_fill_list::Request::decode(request.body.as_slice()).expect("request");
        assert_eq!(
            decoded
                .c2s
                .filter_conditions
                .expect("filter")
                .begin_time
                .as_deref(),
            Some("2026-05-20 00:00:00")
        );
        let response = trd_get_order_fill_list::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_get_order_fill_list::S2c {
                header: trade_header(0, 1001, 1).into(),
                order_fill_list: vec![trd_common::OrderFill {
                    fill_id: 3001,
                    fill_id_ex: "FILL-3001".to_owned(),
                    order_id: Some(2001),
                    order_id_ex: Some("EXT-2001".to_owned()),
                    code: "HK.00700".to_owned(),
                    name: "Tencent".to_owned(),
                    trd_side: 1,
                    qty: 100.0,
                    price: 319.8,
                    create_time: "2026-05-20 09:35:00".to_owned(),
                    trd_market: Some(1),
                    status: Some(0),
                    ..Default::default()
                }],
            }),
        };
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 25).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    let fills = client
        .read_history_fills(
            trade_header(0, 1001, 1),
            Some(TradeFilter {
                code_list: vec!["HK.00700".to_owned()],
                begin_time: Some("2026-05-20 00:00:00".to_owned()),
                end_time: Some("2026-05-20 23:59:59".to_owned()),
                ..TradeFilter::default()
            }),
            None,
        )
        .expect("history fills");
    assert_eq!(fills.len(), 1);
    assert_eq!(fills[0].fill_id, 3001);
    assert_eq!(fills[0].fill_id_ex, "FILL-3001");
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn order_fee_read_projects_amount_and_item_breakdown() {
    // Parity: go:452dea11:pkg/futu/exchange_test.go:561
    // TestQueryBrokerOrderFeesReturnsFeeBreakdown
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        assert_eq!(request.header.proto_id, trd_get_order_fee::PROTOCOL_ID);
        let response = trd_get_order_fee::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_get_order_fee::S2c {
                header: trade_header(0, 1001, 1).into(),
                order_fee_list: vec![trd_common::OrderFee {
                    order_id_ex: "EXT-2001".to_owned(),
                    fee_amount: Some(12.5),
                    fee_list: vec![
                        trd_common::OrderFeeItem {
                            title: Some("BROKERAGE".to_owned()),
                            value: Some(10.0),
                        },
                        trd_common::OrderFeeItem {
                            title: Some("STAMP_DUTY".to_owned()),
                            value: Some(2.5),
                        },
                    ],
                }],
            }),
        };
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 26).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    let fees = client
        .read_order_fees(trade_header(0, 1001, 1), vec!["EXT-2001".to_owned()])
        .expect("order fees");
    assert_eq!(fees.len(), 1);
    assert_eq!(fees[0].broker_order_id_ex, "EXT-2001");
    assert_eq!(fees[0].fee_amount, Some(12.5));
    assert_eq!(fees[0].fee_items.len(), 2);
    assert_eq!(fees[0].fee_items[0].title, "BROKERAGE");
    assert_eq!(fees[0].fee_items[1].value, 2.5);
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn max_trade_quantity_read_projects_cash_and_margin_buying_power() {
    // Parity: go:452dea11:pkg/futu/exchange_test.go:774
    // TestQueryBrokerMaxTradeQuantityReturnsSnapshot
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_frame(&mut stream);
        assert_eq!(request.header.proto_id, trd_get_max_trd_qtys::PROTOCOL_ID);
        let decoded =
            trd_get_max_trd_qtys::Request::decode(request.body.as_slice()).expect("request");
        assert_eq!(decoded.c2s.code, "00700");
        assert_eq!(decoded.c2s.order_type, 1);
        assert_eq!(decoded.c2s.price, 320.5);
        let response = trd_get_max_trd_qtys::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(trd_get_max_trd_qtys::S2c {
                header: trade_header(1, 1001, 1).into(),
                max_trd_qtys: Some(trd_common::MaxTrdQtys {
                    max_cash_buy: 1_000.0,
                    max_cash_and_margin_buy: Some(2_000.0),
                    max_position_sell: 500.0,
                    max_sell_short: Some(300.0),
                    max_buy_back: Some(150.0),
                    long_required_im: Some(10.0),
                    short_required_im: Some(12.0),
                    session: Some(0),
                }),
            }),
        };
        stream
            .write_all(
                &encode_frame(
                    request.header.proto_id,
                    request.header.serial_no,
                    &response.encode_to_vec(),
                )
                .expect("response"),
            )
            .expect("write response");
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 27).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    let snapshot = client
        .read_max_trade_quantity(TradeMaxTradeQuantityRequest {
            header: trade_header(1, 1001, 1),
            order_type: 1,
            code: "00700".to_owned(),
            price: 320.5,
            order_id: None,
            adjust_price: None,
            adjust_side_and_limit: None,
            sec_market: None,
            order_id_ex: None,
            session: None,
            position_id: None,
        })
        .expect("max trade quantity");
    assert_eq!(snapshot.max_cash_buy, 1_000.0);
    assert_eq!(snapshot.max_cash_and_margin_buy, Some(2_000.0));
    assert_eq!(snapshot.max_position_sell, 500.0);
    assert_eq!(snapshot.max_sell_short, Some(300.0));
    assert_eq!(snapshot.max_buy_back, Some(150.0));
    session.close().expect("close");
    server.join().expect("server");
}

#[test]
fn combo_protocol_transport_errors_are_surfaced() {
    // Parity: go:452dea11:pkg/futu/advanced_product_adapter_contracts_test.go:475
    // TestFutuComboProtocolTransportErrors. Dropping the OpenD response for
    // Trd_GetComboMaxTrdQtys or Trd_PlaceComboOrder must surface a session
    // error instead of being reported as an empty/successful combo.
    let max_listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let max_address = max_listener.local_addr().expect("address");
    let max_server = thread::spawn(move || {
        let (stream, _) = max_listener.accept().expect("accept");
        let mut byte = [0_u8; 1];
        let _ = std::io::Read::read(&mut { stream }, &mut byte);
    });
    let max_session = Arc::new(
        OpenDManagedSession::connect(max_address, Duration::from_millis(500), 29)
            .expect("max session"),
    );
    let max_client = OpenDTradeReadClient::from_managed_session(Arc::clone(&max_session));
    max_session.close().expect("close max session");
    let error = max_client
        .read_combo_max_trade_quantity(TradeComboMaxTradeQuantityRequest {
            header: trade_header(1, 1001, 2),
            combo_legs: vec![
                crate::TradeComboLeg {
                    market: 11,
                    code: "US.ONE".to_owned(),
                    side: Some(1),
                    qty_ratio: Some(1.0),
                    position_id: None,
                    pred_side: None,
                },
                crate::TradeComboLeg {
                    market: 11,
                    code: "US.TWO".to_owned(),
                    side: Some(2),
                    qty_ratio: Some(1.0),
                    position_id: None,
                    pred_side: None,
                },
            ],
            quantity: 1.0,
            price: None,
            order_type: 1,
            order_id_ex: None,
        })
        .expect_err("dropped combo max trade quantity response must fail");
    assert!(
        matches!(error, TradeSessionError::Session(_)),
        "error = {error:?}"
    );
    max_server.join().expect("max server");

    let place_listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let place_address = place_listener.local_addr().expect("address");
    let place_server = thread::spawn(move || {
        let (stream, _) = place_listener.accept().expect("accept");
        let mut byte = [0_u8; 1];
        let _ = std::io::Read::read(&mut { stream }, &mut byte);
    });
    let place_session = Arc::new(
        OpenDManagedSession::connect(place_address, Duration::from_millis(500), 30)
            .expect("place session"),
    );
    let place_client = OpenDTradeReadClient::from_managed_session(Arc::clone(&place_session));
    place_session.close().expect("close place session");
    let error = place_client
        .place_combo_order(TradePlaceComboOrderRequest {
            header: trade_header(1, 1001, 2),
            combo_legs: vec![
                crate::TradeComboLeg {
                    market: 11,
                    code: "US.ONE".to_owned(),
                    side: Some(1),
                    qty_ratio: Some(1.0),
                    position_id: None,
                    pred_side: None,
                },
                crate::TradeComboLeg {
                    market: 11,
                    code: "US.TWO".to_owned(),
                    side: Some(2),
                    qty_ratio: Some(1.0),
                    position_id: None,
                    pred_side: None,
                },
            ],
            quantity: 1.0,
            price: None,
            order_type: 1,
            time_in_force: None,
            expire_time: None,
            remark: None,
            quote_id: None,
        })
        .expect_err("dropped combo place response must fail");
    assert!(
        matches!(error, TradeSessionError::Session(_)),
        "error = {error:?}"
    );
    place_server.join().expect("place server");
}

#[test]
fn repeated_account_reads_reuse_one_opend_connection() {
    // Parity: go:452dea11:pkg/futu/exchange_test.go:265
    // TestDiscoverAccountsReusesSingleOpenDConnection. The server accepts one
    // TCP session and answers two Trd_GetAccList calls on it; a second accept
    // would mean the Rust client rebuilt the transport between reads.
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        for expected_id in [1001_u64, 1001_u64] {
            let request = read_frame(&mut stream);
            assert_eq!(request.header.proto_id, trd_get_acc_list::PROTOCOL_ID);
            let response = trd_get_acc_list::Response {
                ret_type: 0,
                ret_msg: None,
                err_code: None,
                s2c: Some(trd_get_acc_list::S2c {
                    acc_list: vec![trd_common::TrdAcc {
                        trd_env: 1,
                        acc_id: expected_id,
                        trd_market_auth_list: vec![1],
                        ..Default::default()
                    }],
                }),
            };
            stream
                .write_all(
                    &encode_frame(
                        request.header.proto_id,
                        request.header.serial_no,
                        &response.encode_to_vec(),
                    )
                    .expect("response"),
                )
                .expect("write response");
        }
    });
    let session = Arc::new(
        OpenDManagedSession::connect(address, Duration::from_millis(500), 28).expect("session"),
    );
    let client = OpenDTradeReadClient::from_managed_session(Arc::clone(&session));
    for _ in 0..2 {
        let accounts = client.read_accounts(7, None, None).expect("accounts");
        assert_eq!(accounts.len(), 1);
        assert_eq!(accounts[0].acc_id, 1001);
    }
    session.close().expect("close");
    server.join().expect("server");
}
