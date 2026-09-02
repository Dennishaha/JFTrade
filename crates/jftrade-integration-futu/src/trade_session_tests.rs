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
