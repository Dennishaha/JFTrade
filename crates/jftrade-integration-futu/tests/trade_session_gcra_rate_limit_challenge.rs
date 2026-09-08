#![forbid(unsafe_code)]

use std::io::{Read, Write};
use std::net::{TcpListener, TcpStream};
use std::num::NonZeroU32;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::thread;
use std::time::{Duration, Instant};

use governor::{Quota, RateLimiter};
use jftrade_integration_futu::{
    Frame, OpenDInitializedSession, OpenDTcpProbeConfig, OpenDTradeReadClient, PROTO_INIT_CONNECT,
    TradeHeader, TradeSecurity, TradeSessionError, decode_frame, encode_frame,
};
use prost::Message;

#[derive(Clone, PartialEq, Message)]
struct InitResponse {
    #[prost(int32, optional, tag = "1")]
    ret_type: Option<i32>,
    #[prost(string, optional, tag = "2")]
    ret_msg: Option<String>,
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
struct TrdHeaderProto {
    #[prost(int32, tag = "1")]
    trd_env: i32,
    #[prost(uint64, tag = "2")]
    acc_id: u64,
    #[prost(int32, tag = "3")]
    trd_market: i32,
}

#[derive(Clone, PartialEq, Message)]
struct MarginRatioResponse {
    #[prost(int32, optional, tag = "1")]
    ret_type: Option<i32>,
    #[prost(string, optional, tag = "2")]
    ret_msg: Option<String>,
    #[prost(int32, optional, tag = "3")]
    err_code: Option<i32>,
    #[prost(message, optional, tag = "4")]
    s2c: Option<MarginRatioS2c>,
}

#[derive(Clone, PartialEq, Message)]
struct MarginRatioS2c {
    #[prost(message, required, tag = "1")]
    header: TrdHeaderProto,
    #[prost(message, repeated, tag = "2")]
    margin_ratio_info_list: Vec<MarginRatioInfo>,
}

#[derive(Clone, PartialEq, Message)]
struct MarginRatioInfo {
    #[prost(message, required, tag = "1")]
    security: SecurityInfo,
    #[prost(bool, optional, tag = "2")]
    is_long_permit: Option<bool>,
    #[prost(bool, optional, tag = "3")]
    is_short_permit: Option<bool>,
    #[prost(double, optional, tag = "4")]
    short_pool_remain: Option<f64>,
    #[prost(double, optional, tag = "5")]
    short_fee_rate: Option<f64>,
    #[prost(double, optional, tag = "6")]
    alert_long_ratio: Option<f64>,
    #[prost(double, optional, tag = "7")]
    alert_short_ratio: Option<f64>,
    #[prost(double, optional, tag = "8")]
    im_long_ratio: Option<f64>,
    #[prost(double, optional, tag = "9")]
    im_short_ratio: Option<f64>,
    #[prost(double, optional, tag = "10")]
    mcm_long_ratio: Option<f64>,
    #[prost(double, optional, tag = "11")]
    mcm_short_ratio: Option<f64>,
    #[prost(double, optional, tag = "12")]
    mm_long_ratio: Option<f64>,
    #[prost(double, optional, tag = "13")]
    mm_short_ratio: Option<f64>,
}

#[derive(Clone, PartialEq, Message)]
struct SecurityInfo {
    #[prost(int32, tag = "1")]
    market: i32,
    #[prost(string, tag = "2")]
    code: String,
}

fn read_framed_frame(stream: &mut TcpStream) -> Option<Frame> {
    let mut header = [0u8; 44];
    stream.read_exact(&mut header).ok()?;
    let mut body_len_bytes = [0u8; 4];
    body_len_bytes.copy_from_slice(&header[12..16]);
    let body_len = u32::from_le_bytes(body_len_bytes) as usize;
    let mut packet = vec![0u8; 44 + body_len];
    packet[..44].copy_from_slice(&header);
    stream.read_exact(&mut packet[44..]).ok()?;
    decode_frame(&packet).ok()
}

fn write_framed_response(stream: &mut TcpStream, proto_id: u32, serial_no: u32, body: &[u8]) {
    let packet = encode_frame(proto_id, serial_no, body).expect("encode frame");
    stream.write_all(&packet).expect("write frame");
}

#[test]
fn test_gcra_governor_burst_9_and_10th_fails_within_3100ms() {
    let quota = Quota::with_period(Duration::from_millis(3100))
        .expect("valid 3100ms quota period")
        .allow_burst(NonZeroU32::new(9).expect("non-zero burst"));
    let limiter = RateLimiter::direct(quota);

    let start = Instant::now();

    // 1. Burst exactly 9 calls: all must succeed immediately
    for i in 1..=9 {
        assert!(
            limiter.check().is_ok(),
            "Burst call #{} should succeed under burst=9 quota",
            i
        );
    }

    // 2. The 10th call within 3100ms MUST fail
    let call_10_res = limiter.check();
    assert!(
        call_10_res.is_err(),
        "Call #10 within 3100ms MUST trigger rate limit"
    );

    // Verify elapsed time is well within 3100ms (almost instantaneous)
    assert!(
        start.elapsed() < Duration::from_millis(1000),
        "Burst should execute in under 1 second"
    );

    // 3. Test intermediate wait (e.g. 1000ms < 3100ms): must still fail
    thread::sleep(Duration::from_millis(1000));
    assert!(
        limiter.check().is_err(),
        "Call after only 1000ms should still be rate limited"
    );

    // 4. Wait until 3100ms from start has elapsed: exactly 1 cell should replenish
    let remaining_to_wait = Duration::from_millis(3200).saturating_sub(start.elapsed());
    if !remaining_to_wait.is_zero() {
        thread::sleep(remaining_to_wait);
    }

    assert!(
        limiter.check().is_ok(),
        "After 3100ms period has elapsed, 1 token must replenish and succeed"
    );

    // 5. Immediate 11th call must fail again because only 1 token replenished
    assert!(
        limiter.check().is_err(),
        "Subsequent call without replenishment must fail again"
    );
}

#[test]
fn test_gcra_governor_concurrent_burst_thread_safety() {
    let quota = Quota::with_period(Duration::from_millis(3100))
        .expect("valid 3100ms quota period")
        .allow_burst(NonZeroU32::new(9).expect("non-zero burst"));
    let limiter = Arc::new(RateLimiter::direct(quota));

    let success_count = Arc::new(AtomicUsize::new(0));
    let fail_count = Arc::new(AtomicUsize::new(0));

    let mut handles = Vec::new();
    // Launch 30 concurrent threads trying to grab a token
    for _ in 0..30 {
        let lim = Arc::clone(&limiter);
        let s = Arc::clone(&success_count);
        let f = Arc::clone(&fail_count);
        handles.push(thread::spawn(move || {
            if lim.check().is_ok() {
                s.fetch_add(1, Ordering::SeqCst);
            } else {
                f.fetch_add(1, Ordering::SeqCst);
            }
        }));
    }

    for h in handles {
        h.join().unwrap();
    }

    assert_eq!(
        success_count.load(Ordering::SeqCst),
        9,
        "Exactly 9 concurrent threads must succeed in the burst"
    );
    assert_eq!(
        fail_count.load(Ordering::SeqCst),
        21,
        "Remaining 21 concurrent threads must be rejected"
    );
}

#[test]
fn test_opend_trade_read_client_burst_and_10th_call_preemption() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind");
    let address = listener.local_addr().expect("local_addr");
    let margin_requests_received = Arc::new(AtomicUsize::new(0));
    let margin_clone = Arc::clone(&margin_requests_received);

    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        // 1. Initial connect handshake
        let init_frame = read_framed_frame(&mut stream).expect("init frame");
        assert_eq!(init_frame.header.proto_id, PROTO_INIT_CONNECT);
        write_framed_response(
            &mut stream,
            PROTO_INIT_CONNECT,
            init_frame.header.serial_no,
            &InitResponse {
                ret_type: Some(0),
                ret_msg: Some("ok".to_owned()),
                s2c: Some(InitState {
                    server_ver: 1009,
                    conn_id: 42,
                }),
            }
            .encode_to_vec(),
        );

        // 2. Loop answering margin ratio requests (proto 2223)
        while let Some(frame) = read_framed_frame(&mut stream) {
            if frame.header.proto_id == 2223 {
                margin_clone.fetch_add(1, Ordering::SeqCst);
                let resp = MarginRatioResponse {
                    ret_type: Some(0),
                    ret_msg: Some("ok".to_owned()),
                    err_code: None,
                    s2c: Some(MarginRatioS2c {
                        header: TrdHeaderProto {
                            trd_env: 1,
                            acc_id: 42,
                            trd_market: 11,
                        },
                        margin_ratio_info_list: vec![MarginRatioInfo {
                            security: SecurityInfo {
                                market: 11,
                                code: "AAPL".to_owned(),
                            },
                            is_long_permit: Some(true),
                            is_short_permit: Some(true),
                            short_pool_remain: Some(1000.0),
                            short_fee_rate: Some(0.01),
                            alert_long_ratio: Some(1.2),
                            alert_short_ratio: Some(1.3),
                            im_long_ratio: Some(1.5),
                            im_short_ratio: Some(1.5),
                            mcm_long_ratio: Some(1.3),
                            mcm_short_ratio: Some(1.3),
                            mm_long_ratio: Some(1.25),
                            mm_short_ratio: Some(1.25),
                        }],
                    }),
                };
                write_framed_response(
                    &mut stream,
                    frame.header.proto_id,
                    frame.header.serial_no,
                    &resp.encode_to_vec(),
                );
            }
        }
    });

    let config = OpenDTcpProbeConfig::new(address, Duration::from_millis(500));
    let session = OpenDInitializedSession::connect(&config, 1).expect("initialized session");
    let client = OpenDTradeReadClient::from_session(session);

    let header = TradeHeader {
        trd_env: 1,
        acc_id: 42,
        trd_market: 11,
        jp_acc_type: None,
    };
    let securities = vec![TradeSecurity {
        market: 11,
        code: "AAPL".to_owned(),
    }];

    // 1. Burst 9 calls on the client: all 9 MUST succeed
    for i in 1..=9 {
        let res = client.read_margin_ratios(header.clone(), securities.clone());
        assert!(
            res.is_ok(),
            "Client margin ratio read #{} must succeed: {:?}",
            i,
            res.err()
        );
    }
    assert_eq!(
        margin_requests_received.load(Ordering::SeqCst),
        9,
        "Server must have received exactly 9 requests"
    );

    // 2. 10th call MUST fail immediately with TradeSessionError::RateLimited
    let call_10_res = client.read_margin_ratios(header.clone(), securities.clone());
    assert!(
        matches!(call_10_res, Err(TradeSessionError::RateLimited)),
        "10th call MUST return TradeSessionError::RateLimited, got: {:?}",
        call_10_res
    );

    // 3. EMPIRICAL VERIFICATION OF PREEMPTION BEFORE SOCKET I/O:
    // The server MUST NOT have received a 10th request!
    assert_eq!(
        margin_requests_received.load(Ordering::SeqCst),
        9,
        "Server MUST NOT have received a 10th request; rate limiter preempted before socket I/O"
    );

    // Drop client to close connection and join server
    drop(client);
    server.join().unwrap();
}
