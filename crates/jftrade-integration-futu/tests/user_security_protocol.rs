//! Framed OpenD coverage for the user-security (watchlist) protocol.
//!
//! Baseline: `go:452dea11:pkg/futu/opend/user_security_test.go` and
//! `pkg/futu/opend/protocol_ids_test.go`.
//!
//! The fake server only stands in for OpenD's wire endpoint; framing, serial
//! routing, rejection mapping and the projection to the remote-watchlist port
//! all stay live.

use std::io::{Read, Write};
use std::net::{SocketAddr, TcpListener, TcpStream};
use std::sync::{Arc, Mutex};
use std::thread::{self, JoinHandle};
use std::time::Duration;

use jftrade_integration_futu::{
    Frame, FutuRemoteWatchlistReader, OpenDSessionCoordinator, OpenDTcpProbeConfig,
    PROTO_GET_USER_SECURITY, PROTO_GET_USER_SECURITY_GROUP, RemoteWatchlistReadPort,
    RemoteWatchlistWritePort, decode_frame, encode_frame,
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

#[derive(Clone, PartialEq, Message)]
struct GroupRequest {
    #[prost(message, optional, tag = "1")]
    c2s: Option<GroupC2s>,
}

#[derive(Clone, PartialEq, Message)]
struct GroupC2s {
    #[prost(int32, tag = "1")]
    group_type: i32,
}

#[derive(Clone, PartialEq, Message)]
struct GroupResponse {
    #[prost(int32, optional, tag = "1")]
    ret_type: Option<i32>,
    #[prost(string, optional, tag = "2")]
    ret_msg: Option<String>,
    #[prost(int32, optional, tag = "3")]
    err_code: Option<i32>,
    #[prost(message, optional, tag = "4")]
    s2c: Option<GroupS2c>,
}

#[derive(Clone, PartialEq, Message)]
struct GroupS2c {
    #[prost(message, repeated, tag = "1")]
    group_list: Vec<GroupEntry>,
}

#[derive(Clone, PartialEq, Message)]
struct GroupEntry {
    #[prost(string, optional, tag = "1")]
    group_name: Option<String>,
    #[prost(int32, optional, tag = "2")]
    group_type: Option<i32>,
}

#[derive(Clone, PartialEq, Message)]
struct SecurityRequest {
    #[prost(message, optional, tag = "1")]
    c2s: Option<SecurityC2s>,
}

#[derive(Clone, PartialEq, Message)]
struct SecurityC2s {
    #[prost(string, optional, tag = "1")]
    group_name: Option<String>,
}

#[derive(Clone, PartialEq, Message)]
struct SecurityResponse {
    #[prost(int32, optional, tag = "1")]
    ret_type: Option<i32>,
    #[prost(string, optional, tag = "2")]
    ret_msg: Option<String>,
    #[prost(int32, optional, tag = "3")]
    err_code: Option<i32>,
    #[prost(message, optional, tag = "4")]
    s2c: Option<SecurityS2c>,
}

#[derive(Clone, PartialEq, Message)]
struct SecurityS2c {
    #[prost(message, repeated, tag = "1")]
    static_info_list: Vec<SecurityStaticInfo>,
}

#[derive(Clone, PartialEq, Message)]
struct SecurityStaticInfo {
    #[prost(message, optional, tag = "1")]
    basic: Option<SecurityStaticBasic>,
}

#[derive(Clone, PartialEq, Message)]
struct SecurityStaticBasic {
    #[prost(message, optional, tag = "1")]
    security: Option<WireSecurity>,
    #[prost(int64, optional, tag = "2")]
    id: Option<i64>,
    #[prost(int32, optional, tag = "3")]
    lot_size: Option<i32>,
    #[prost(int32, optional, tag = "4")]
    sec_type: Option<i32>,
    #[prost(string, optional, tag = "5")]
    name: Option<String>,
}

#[derive(Clone, PartialEq, Message)]
struct WireSecurity {
    #[prost(int32, tag = "1")]
    market: i32,
    #[prost(string, tag = "2")]
    code: String,
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
    F: FnOnce(&mut TcpStream, Frame, &mut dyn FnMut(&mut TcpStream) -> Frame) + Send + 'static,
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
        let first = read_frame(&mut stream);
        // The reader may issue a second RPC on the same session (groups then
        // members). Keep it lazy so single-RPC cases can answer first and never
        // block waiting for a request that will not arrive.
        let mut next = |stream: &mut TcpStream| read_frame(stream);
        handler(&mut stream, first, &mut next);
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

// Parity: go:452dea11:pkg/futu/opend/user_security_test.go:15
// TestGetUserSecurityGroupsEncodesAllAndReturnsCustomAndSystemGroups
#[test]
fn groups_encode_group_type_all_and_project_custom_and_system() {
    let (address, task) = server(|stream, groups, _next| {
        assert_eq!(groups.header.proto_id, PROTO_GET_USER_SECURITY_GROUP);
        let request = GroupRequest::decode(groups.body.as_slice()).expect("group request");
        assert_eq!(
            request.c2s.expect("group c2s").group_type,
            3,
            "Go asks for GroupType_All"
        );
        write_response(
            stream,
            groups.header.proto_id,
            groups.header.serial_no,
            GroupResponse {
                ret_type: Some(0),
                ret_msg: None,
                err_code: None,
                s2c: Some(GroupS2c {
                    group_list: vec![
                        GroupEntry {
                            group_name: Some("Long Term".to_owned()),
                            group_type: Some(1),
                        },
                        GroupEntry {
                            group_name: Some("All".to_owned()),
                            group_type: Some(2),
                        },
                    ],
                }),
            }
            .encode_to_vec(),
        );
    });
    let coordinator = make_coordinator(address, Duration::from_secs(1));
    let reader = FutuRemoteWatchlistReader::new(Arc::clone(&coordinator));
    let groups = reader.groups().expect("groups");
    assert_eq!(groups.len(), 2);
    assert_eq!(groups[0]["name"], "Long Term");
    assert_eq!(groups[0]["type"], "custom");
    assert_eq!(groups[1]["name"], "All");
    assert_eq!(groups[1]["type"], "system");
    close(&coordinator);
    task.join().expect("server");
}

// Parity: go:452dea11:pkg/futu/advanced_product_adapter_contracts_test.go:22
// TestFutuAdvancedSpecializedReadersAndCustomizationSuccess (watchlist write)
//
// ApplyCustomization for the remote watchlist maps `op: 1` to the OpenD
// ModifyUserSecurity add operation and forwards the security list unchanged.
#[test]
fn remote_watchlist_modify_encodes_group_operation_and_security_list() {
    use jftrade_integration_futu::trade_proto::qot_modify_user_security as wire;

    let (address, task) = server(move |stream, request, _next| {
        assert_eq!(request.header.proto_id, 3214);
        let decoded = wire::Request::decode(request.body.as_slice()).expect("modify request");
        assert_eq!(decoded.c2s.group_name, "Favorites");
        assert_eq!(decoded.c2s.op, 1);
        assert_eq!(decoded.c2s.security_list.len(), 1);
        assert_eq!(decoded.c2s.security_list[0].market, 11);
        assert_eq!(decoded.c2s.security_list[0].code, "AAPL");
        write_response(
            stream,
            request.header.proto_id,
            request.header.serial_no,
            wire::Response {
                ret_type: 0,
                ret_msg: None,
                err_code: None,
                s2c: None,
            }
            .encode_to_vec(),
        );
    });
    let coordinator = make_coordinator(address, Duration::from_secs(1));
    let reader = FutuRemoteWatchlistReader::new(Arc::clone(&coordinator));
    let written = reader
        .modify(
            "Favorites",
            "add",
            &[serde_json::json!({"market": 11, "code": "AAPL"})],
        )
        .expect("remote watchlist write");
    assert_eq!(written["groupName"], "Favorites");
    assert_eq!(written["changed"], 1);
    close(&coordinator);
    task.join().expect("server");
}

// Parity: go:452dea11:pkg/futu/opend/user_security_test.go:51
// TestGetUserSecuritiesEncodesTrimmedGroupName
#[test]
fn members_encode_a_trimmed_group_name_and_project_static_info() {
    // `members` is the only RPC in this case, so the first frame the fake
    // server sees is already the Qot_GetUserSecurity request.
    let (address, task) = server(|stream, members, _next| {
        assert_eq!(members.header.proto_id, PROTO_GET_USER_SECURITY);
        let request = SecurityRequest::decode(members.body.as_slice()).expect("members request");
        assert_eq!(
            request.c2s.expect("members c2s").group_name.as_deref(),
            Some("Long Term"),
            "the group name must be trimmed before encoding"
        );
        write_response(
            stream,
            members.header.proto_id,
            members.header.serial_no,
            SecurityResponse {
                ret_type: Some(0),
                ret_msg: None,
                err_code: None,
                s2c: Some(SecurityS2c {
                    static_info_list: vec![SecurityStaticInfo {
                        basic: Some(SecurityStaticBasic {
                            security: Some(WireSecurity {
                                market: 1,
                                code: "00700".to_owned(),
                            }),
                            id: Some(700),
                            lot_size: Some(100),
                            sec_type: Some(3),
                            name: Some("Tencent".to_owned()),
                        }),
                    }],
                }),
            }
            .encode_to_vec(),
        );
    });
    let coordinator = make_coordinator(address, Duration::from_secs(1));
    let reader = FutuRemoteWatchlistReader::new(Arc::clone(&coordinator));
    let members = reader.members("  Long Term  ").expect("members");
    assert_eq!(members.len(), 1);
    assert_eq!(members[0]["instrumentId"], "HK.00700");
    assert_eq!(members[0]["lotSize"], 100);
    assert_eq!(members[0]["name"], "Tencent");
    close(&coordinator);
    task.join().expect("server");
}

// Parity: go:452dea11:pkg/futu/opend/user_security_test.go:103
// TestUserSecurityMethodsPropagateBusinessErrorsAndEmptyResults
#[test]
fn a_rejected_group_query_surfaces_the_opend_message() {
    let (address, task) = server(|stream, groups, _next| {
        assert_eq!(groups.header.proto_id, PROTO_GET_USER_SECURITY_GROUP);
        write_response(
            stream,
            groups.header.proto_id,
            groups.header.serial_no,
            GroupResponse {
                ret_type: Some(-1),
                ret_msg: Some("group query rate limited".to_owned()),
                err_code: Some(429),
                s2c: None,
            }
            .encode_to_vec(),
        );
    });
    let coordinator = make_coordinator(address, Duration::from_secs(1));
    let reader = FutuRemoteWatchlistReader::new(Arc::clone(&coordinator));
    let error = reader.groups().expect_err("rejection must fail closed");
    assert!(
        error.to_string().contains("group query rate limited"),
        "the OpenD message must be preserved: {error}"
    );
    close(&coordinator);
    task.join().expect("server");
}

// Parity: go:452dea11:pkg/futu/opend/user_security_test.go:125
// TestUserSecurityMethodsNormalizeMissingGroupsAndReportSecurityErrors
#[test]
fn a_missing_group_payload_is_an_empty_list_but_member_errors_stay_typed() {
    let (address, task) = server(|stream, groups, next| {
        write_response(
            stream,
            groups.header.proto_id,
            groups.header.serial_no,
            GroupResponse {
                ret_type: Some(0),
                ret_msg: None,
                err_code: None,
                s2c: None,
            }
            .encode_to_vec(),
        );
        let members = next(stream);
        assert_eq!(members.header.proto_id, PROTO_GET_USER_SECURITY);
        write_response(
            stream,
            members.header.proto_id,
            members.header.serial_no,
            SecurityResponse {
                ret_type: Some(-1),
                ret_msg: Some("watchlist access denied".to_owned()),
                err_code: Some(403),
                s2c: None,
            }
            .encode_to_vec(),
        );
    });
    let coordinator = make_coordinator(address, Duration::from_secs(1));
    let reader = FutuRemoteWatchlistReader::new(Arc::clone(&coordinator));
    assert!(
        reader.groups().expect("missing groups").is_empty(),
        "a missing S2C normalizes to an empty list, not an error"
    );
    let error = reader
        .members("Long Term")
        .expect_err("member rejection must fail closed");
    assert!(
        error.to_string().contains("watchlist access denied"),
        "the OpenD message must be preserved: {error}"
    );
    close(&coordinator);
    task.join().expect("server");
}

// Parity: go:452dea11:pkg/futu/opend/user_security_test.go:93
// TestUserSecurityMethodsRejectInvalidInputBeforeEncoding
//
// Go validates the trimmed group name before it builds any request. Rust keeps
// that validation in `members`, so a blank name must fail without an OpenD
// session at all — this test therefore installs no server and proves the call
// never depends on transport to reject the input.
#[test]
fn a_blank_group_name_is_rejected_before_any_rpc() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let task = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let init = read_frame(&mut stream);
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
        // The reader is dropped without issuing a request; the coordinator
        // tolerates the peer close because no call is pending.
        let mut byte = [0_u8; 1];
        let _ = stream.read(&mut byte);
    });
    let coordinator = make_coordinator(address, Duration::from_secs(1));
    let reader = FutuRemoteWatchlistReader::new(Arc::clone(&coordinator));
    for blank in ["", "   ", "\t\n"] {
        let error = reader
            .members(blank)
            .expect_err("blank name must be rejected");
        assert!(
            error.to_string().contains("group name is required"),
            "unexpected error for {blank:?}: {error}"
        );
    }
    close(&coordinator);
    task.join().expect("server");
}
