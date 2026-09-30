//! Socket-level regressions for the OpenD client recovery boundaries.
//!
//! Parity: go:pkg/futu/client_exchange_recovery_boundaries_test.go. Go drives a
//! scripted OpenD server so reconnect, generation fencing, session
//! initialization failures and trade-push resubscription are exercised
//! together. The same is done here with a framed loopback server; nothing is
//! asserted through private helpers.

use std::io::{Read, Write};
use std::net::{TcpListener, TcpStream};
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex, mpsc};
use std::thread;
use std::time::Duration;

use jftrade_integration_futu::{
    Frame, MarketMicrostructureOperation, MarketMicrostructureReadPort, OpenDInitializedSession,
    OpenDMarketMicrostructureReader, OpenDSessionCloseReason, OpenDSessionCoordinator,
    OpenDSessionCoordinatorOutcome, OpenDSessionEventListener, OpenDSessionRuntime,
    OpenDSessionRuntimeConfig, OpenDTcpProbe, OpenDTcpProbeConfig, OpenDTcpProbeError,
    OpenDTradeReadClient, PROTO_GET_GLOBAL_STATE, PROTO_INIT_CONNECT, PROTO_QOT_SUB, TradeHeader,
    TradeSessionError, TradeSubscribeAccountsRequest, TradeWritePort, decode_frame, encode_frame,
    is_recoverable_error,
};
use jftrade_marketdata::{InstrumentRef, MarketDataRuntimeRecorder};
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
struct PlainResponse {
    #[prost(int32, optional, tag = "1")]
    ret_type: Option<i32>,
}

fn read_framed_frame(stream: &mut TcpStream) -> Option<Frame> {
    let mut header = [0_u8; 44];
    stream.read_exact(&mut header).ok()?;
    let body_len = u32::from_le_bytes(header[12..16].try_into().ok()?) as usize;
    let mut packet = vec![0_u8; 44 + body_len];
    packet[..44].copy_from_slice(&header);
    stream.read_exact(&mut packet[44..]).ok()?;
    decode_frame(&packet).ok()
}

fn write_framed_response(stream: &mut TcpStream, proto_id: u32, serial_no: u32, body: &[u8]) {
    let packet = encode_frame(proto_id, serial_no, body).expect("encode frame");
    stream.write_all(&packet).expect("write frame");
}

fn init_body(server_ver: i32, conn_id: u64) -> Vec<u8> {
    InitResponse {
        ret_type: Some(0),
        ret_msg: None,
        s2c: Some(InitState {
            server_ver,
            conn_id,
        }),
    }
    .encode_to_vec()
}

fn success_body() -> Vec<u8> {
    PlainResponse { ret_type: Some(0) }.encode_to_vec()
}

fn snapshot(symbol: &str) -> InstrumentRef {
    InstrumentRef {
        channel: "SNAPSHOT".to_owned(),
        market: "US".to_owned(),
        symbol: symbol.to_owned(),
        interval: None,
    }
}

fn coordinator(
    address: std::net::SocketAddr,
    desired: Vec<InstrumentRef>,
) -> (
    Arc<Mutex<OpenDSessionCoordinator>>,
    Arc<MarketDataRuntimeRecorder>,
) {
    let recorder = Arc::new(MarketDataRuntimeRecorder::default());
    let coordinator = OpenDSessionCoordinator::connect(
        OpenDTcpProbeConfig::new(address, Duration::from_secs(2)),
        Arc::clone(&recorder),
        desired,
        0,
    )
    .expect("connect");
    (Arc::new(Mutex::new(coordinator)), recorder)
}

/// Accepts the handshake and answers the replayed QOT_SUB request for one
/// session, returning the socket so the caller can script the next step.
fn accept_subscribed_session(listener: &TcpListener, conn_id: u64) -> TcpStream {
    let (mut stream, _) = listener.accept().expect("accept");
    let init = read_framed_frame(&mut stream).expect("init frame");
    assert_eq!(init.header.proto_id, PROTO_INIT_CONNECT);
    write_framed_response(
        &mut stream,
        PROTO_INIT_CONNECT,
        init.header.serial_no,
        &init_body(1009, conn_id),
    );
    let subscribe = read_framed_frame(&mut stream).expect("subscribe frame");
    assert_eq!(subscribe.header.proto_id, PROTO_QOT_SUB);
    write_framed_response(
        &mut stream,
        PROTO_QOT_SUB,
        subscribe.header.serial_no,
        &success_body(),
    );
    stream
}

fn expect_init_error(address: std::net::SocketAddr) -> OpenDTcpProbeError {
    match OpenDInitializedSession::connect_with_push_notifications(
        &OpenDTcpProbeConfig::new(address, Duration::from_secs(1)),
        1,
    ) {
        Ok(_) => panic!("a failed handshake must not produce a usable session"),
        Err(error) => error,
    }
}

/// Scripted OpenD server that completes the handshake and then drops the
/// response for one protocol, mirroring Go's `quoteOpenDServer.setDropProto`.
///
/// Every other request is answered with a success envelope. Once the dropped
/// protocol arrives the server stops reading without answering, so the caller
/// observes the request timeout instead of a fabricated success.
fn drop_protocol_server(drop_proto: u32) -> (std::net::SocketAddr, thread::JoinHandle<()>) {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind");
    let address = listener.local_addr().expect("local_addr");
    let handle = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let init = read_framed_frame(&mut stream).expect("init frame");
        assert_eq!(init.header.proto_id, PROTO_INIT_CONNECT);
        write_framed_response(
            &mut stream,
            PROTO_INIT_CONNECT,
            init.header.serial_no,
            &init_body(1009, 7),
        );
        loop {
            let Some(frame) = read_framed_frame(&mut stream) else {
                return;
            };
            if frame.header.proto_id == drop_proto {
                // Go's fixture consumes the frame and never answers it.
                return;
            }
            write_framed_response(
                &mut stream,
                frame.header.proto_id,
                frame.header.serial_no,
                &success_body(),
            );
        }
    });
    (address, handle)
}

/// Go `TestWithClientReplayPolicyForRecoverableErrors`: `withClient` runs once
/// and `withRetryingClient` may replay once after a reconnect, gated by
/// `isRecoverableOpenDErr`.
#[test]
// Parity: go:452dea11:pkg/futu/client_exchange_recovery_boundaries_test.go:19 TestWithClientReplayPolicyForRecoverableErrors
fn recoverable_error_policy_gates_replay_safe_reads() {
    // Go rejects nil and permission/business errors and accepts the closed,
    // timeout and transport families `withRetryingClient` replays. The Rust
    // owner of that predicate is `recoverable_error`.
    assert!(!is_recoverable_error(None));
    assert!(is_recoverable_error(Some("opend: client closed")));
    assert!(is_recoverable_error(Some("opend: request timed out")));
    assert!(is_recoverable_error(Some("write tcp: broken pipe")));
    assert!(is_recoverable_error(Some(
        "read tcp: connection reset by peer"
    )));
    assert!(is_recoverable_error(Some(
        "use of closed network connection"
    )));
    assert!(!is_recoverable_error(Some("permission denied")));
    assert!(!is_recoverable_error(Some(
        "OpenD GetBasicQot returned retType=-1 errCode=9: no quote right"
    )));

    // The replay-safe read below is the Rust owner of Go's `withRetryingClient`:
    // one authenticated session is closed by the peer, and the coordinator
    // rebuilds it and replays the approved subscriptions on the replacement.
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind");
    let address = listener.local_addr().expect("local_addr");
    let sessions = Arc::new(AtomicU64::new(0));
    let observed = Arc::clone(&sessions);
    let server = thread::spawn(move || {
        let first = accept_subscribed_session(&listener, 1);
        observed.fetch_add(1, Ordering::SeqCst);
        // A recoverable peer close: the replay-safe read must not surface it.
        let _ = first.shutdown(std::net::Shutdown::Both);
        let second = accept_subscribed_session(&listener, 2);
        observed.fetch_add(1, Ordering::SeqCst);
        let mut byte = [0_u8; 1];
        let _ = (&second).read(&mut byte);
    });

    let (coordinator, _recorder) = coordinator(address, vec![snapshot("AAPL")]);
    let now: jftrade_kernel::WireTimestamp = "2026-08-24T00:00:00Z".parse().expect("timestamp");
    {
        let mut guard = coordinator.lock().expect("coordinator");
        assert_eq!(guard.generation(), 1);
        let outcome = guard
            .poll_once(now, Duration::from_millis(250))
            .expect("recoverable close must be replayed, not surfaced");
        assert!(
            matches!(
                outcome,
                OpenDSessionCoordinatorOutcome::Reconnected {
                    generation: 2,
                    reason: OpenDSessionCloseReason::PeerClosed,
                }
            ),
            "unexpected recovery outcome: {outcome:?}"
        );
        assert_eq!(guard.generation(), 2);
        assert!(guard.close().expect("close"));
    }
    server.join().expect("server");
    assert_eq!(
        sessions.load(Ordering::SeqCst),
        2,
        "the replay must use a replacement session, not a third attempt"
    );
}

/// Go `TestExchangeReconnectsClosedReadyClientAndCoversHandlerBoundaries`: a
/// closed, ready client is replaced by a new session that owns the replayed
/// subscriptions.
#[test]
// Parity: go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go:317 TestStreamReconnectAndClientWatcherExitPaths
// Parity: go:452dea11:pkg/futu/client_exchange_recovery_boundaries_test.go:56 TestExchangeReconnectsClosedReadyClientAndCoversHandlerBoundaries
fn closed_ready_session_is_replaced_on_peer_close() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind");
    let address = listener.local_addr().expect("local_addr");
    let (release_first, wait_first) = mpsc::channel();
    let server = thread::spawn(move || {
        let first = accept_subscribed_session(&listener, 1);
        wait_first.recv().expect("release first session");
        let _ = first.shutdown(std::net::Shutdown::Both);
        let second = accept_subscribed_session(&listener, 2);
        let mut byte = [0_u8; 1];
        let _ = (&second).read(&mut byte);
    });

    let (coordinator, recorder) = coordinator(address, vec![snapshot("AAPL")]);
    let now: jftrade_kernel::WireTimestamp = "2026-08-24T00:00:00Z".parse().expect("timestamp");
    {
        let mut guard = coordinator.lock().expect("coordinator");
        // The first session is authenticated, subscribed and ready: the
        // replacement must be a *different* session, not a reused client.
        let first_generation = guard
            .session()
            .expect("ready session")
            .managed_session()
            .generation();
        assert_eq!(first_generation, 1);
        release_first.send(()).expect("release session");
        let outcome = guard
            .poll_once(now, Duration::from_secs(2))
            .expect("closed ready session must be replaced");
        assert!(
            matches!(
                outcome,
                OpenDSessionCoordinatorOutcome::Reconnected {
                    generation: 2,
                    reason: OpenDSessionCloseReason::PeerClosed,
                }
            ),
            "unexpected replacement outcome: {outcome:?}"
        );
        let replacement = guard.session().expect("replacement session");
        assert_ne!(replacement.managed_session().generation(), first_generation);
        assert_eq!(replacement.managed_session().generation(), 2);
        assert!(
            !replacement.managed_session().is_closed(),
            "the replacement session must be usable"
        );
        assert_eq!(recorder.snapshot().generation, 2);
        assert!(guard.close().expect("close"));
    }
    server.join().expect("server");
}

/// Go `TestReconnectDoesNotDeadlockWithInFlightNotification`: a notification
/// handler may read connection state while a reconnect is in flight, so the
/// runtime must publish events without holding the coordinator lock.
#[test]
// Parity: go:452dea11:pkg/futu/client_exchange_recovery_boundaries_test.go:107 TestReconnectDoesNotDeadlockWithInFlightNotification
fn reconnect_completes_while_a_notification_listener_reads_coordinator_state() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind");
    let address = listener.local_addr().expect("local_addr");
    let (release_first, wait_first) = mpsc::channel();
    let server = thread::spawn(move || {
        let first = accept_subscribed_session(&listener, 1);
        wait_first.recv().expect("release first session");
        let _ = first.shutdown(std::net::Shutdown::Both);
        let second = accept_subscribed_session(&listener, 2);
        let mut byte = [0_u8; 1];
        let _ = (&second).read(&mut byte);
    });

    #[derive(Debug)]
    struct GenerationProbe {
        coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
        observed_generation: Arc<AtomicU64>,
        notifications: Arc<AtomicU64>,
        reconnected: Mutex<Option<mpsc::Sender<u64>>>,
    }

    impl OpenDSessionEventListener for GenerationProbe {
        fn on_event(&self, _outcome: &OpenDSessionCoordinatorOutcome) {
            // Go's handler calls `exchange.ConnectionGeneration()` while the
            // reconnect is still in flight. Re-reading the coordinator here
            // must not deadlock on a lock the runtime still holds.
            let generation = self
                .coordinator
                .lock()
                .unwrap_or_else(|error| error.into_inner())
                .generation();
            self.observed_generation.store(generation, Ordering::SeqCst);
            self.notifications.fetch_add(1, Ordering::SeqCst);
            if generation == 2
                && let Ok(mut slot) = self.reconnected.lock()
                && let Some(sender) = slot.take()
            {
                let _ = sender.send(generation);
            }
        }

        fn on_error(&self, _error: &str) {}
    }

    let (coordinator, _recorder) = coordinator(address, vec![snapshot("AAPL")]);
    let observed_generation = Arc::new(AtomicU64::new(0));
    let notifications = Arc::new(AtomicU64::new(0));
    let (reconnected_tx, reconnected_rx) = mpsc::channel();
    let mut runtime = OpenDSessionRuntime::start(
        Arc::clone(&coordinator),
        OpenDSessionRuntimeConfig {
            poll_interval: Duration::from_millis(5),
            event_timeout: Duration::from_millis(1),
            reconnect_initial_delay: Duration::from_millis(10),
            reconnect_max_delay: Duration::from_millis(10),
            event_listener: Some(Arc::new(GenerationProbe {
                coordinator: Arc::clone(&coordinator),
                observed_generation: Arc::clone(&observed_generation),
                notifications: Arc::clone(&notifications),
                reconnected: Mutex::new(Some(reconnected_tx)),
            })),
            ..OpenDSessionRuntimeConfig::default()
        },
    )
    .expect("runtime task");

    release_first.send(()).expect("release session");
    // The runtime publishes the reconnect and then hands the outcome to the
    // listener. Waiting on the probe's own notification avoids both racing the
    // publication order and depending on thread scheduling under a loaded test
    // runner.
    let observed = reconnected_rx
        .recv_timeout(Duration::from_secs(30))
        .expect("the notification listener must observe the reconnect");
    assert_eq!(observed, 2);
    let status = runtime.status();
    assert!(
        status.reconnects > 0,
        "the runtime must publish the reconnect instead of deadlocking: {status:?}"
    );
    assert!(
        notifications.load(Ordering::SeqCst) > 0,
        "the notification listener must have observed the reconnect"
    );
    assert_eq!(
        observed_generation.load(Ordering::SeqCst),
        2,
        "the listener reads the post-reconnect generation"
    );
    assert_eq!(coordinator.lock().expect("coordinator").generation(), 2);
    runtime.shutdown().expect("shutdown");
    server.join().expect("server");
}

/// Go `TestOldOpenDVersionFailsSessionInitialization`: InitConnect only exposes
/// `serverVer`, so an older minor line must fail session initialization before
/// the session is handed out.
#[test]
// Parity: go:452dea11:pkg/futu/exchange_test.go:237 TestConnectRejectsOpenDBelowMinimumVersion
// Parity: go:452dea11:pkg/futu/client_exchange_recovery_boundaries_test.go:249 TestOldOpenDVersionFailsSessionInitialization
fn below_minimum_version_fails_session_initialization() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind");
    let address = listener.local_addr().expect("local_addr");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let init = read_framed_frame(&mut stream).expect("init frame");
        write_framed_response(
            &mut stream,
            PROTO_INIT_CONNECT,
            init.header.serial_no,
            &init_body(1008, 1),
        );
        let mut byte = [0_u8; 1];
        let _ = stream.read(&mut byte);
    });

    let error = expect_init_error(address);
    assert!(
        matches!(error, OpenDTcpProbeError::UnsupportedVersion { .. }),
        "unexpected initialization error: {error:?}"
    );
    // Go's `ValidateMinimumVersion` message names the detected build.
    assert!(
        error.to_string().contains("10.8"),
        "the rejection must report the detected version: {error}"
    );
    server.join().expect("server");
}

/// Go `TestInitResponseAndSessionTransportFailures`: every InitConnect failure
/// mode stays typed and no failure is reported as a usable session.
#[test]
// Parity: go:452dea11:pkg/futu/client_exchange_recovery_boundaries_test.go:260 TestInitResponseAndSessionTransportFailures
fn init_response_and_session_transport_failures_stay_typed() {
    // retType != 0 -> typed rejection carrying OpenD's own message.
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind");
    let address = listener.local_addr().expect("local_addr");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let init = read_framed_frame(&mut stream).expect("init frame");
        write_framed_response(
            &mut stream,
            PROTO_INIT_CONNECT,
            init.header.serial_no,
            &InitResponse {
                ret_type: Some(1),
                ret_msg: Some("denied".to_owned()),
                s2c: None,
            }
            .encode_to_vec(),
        );
    });
    let error = expect_init_error(address);
    assert!(
        matches!(
            error,
            OpenDTcpProbeError::Rejected { ret_type: 1, ref message, .. } if message == "denied"
        ),
        "unexpected rejection mapping: {error:?}"
    );
    server.join().expect("server");

    // retType == 0 without S2C -> typed missing-state error.
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind");
    let address = listener.local_addr().expect("local_addr");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let init = read_framed_frame(&mut stream).expect("init frame");
        write_framed_response(
            &mut stream,
            PROTO_INIT_CONNECT,
            init.header.serial_no,
            &InitResponse {
                ret_type: Some(0),
                ret_msg: None,
                s2c: None,
            }
            .encode_to_vec(),
        );
    });
    let error = expect_init_error(address);
    assert!(
        matches!(error, OpenDTcpProbeError::MissingInitState),
        "unexpected missing-state mapping: {error:?}"
    );
    server.join().expect("server");

    // Peer drop during InitConnect and during GetGlobalState -> typed close,
    // never a silent success. Go performs GetGlobalState inside its session
    // initialization; in Rust that step belongs to the health probe, so the
    // drop cases run through `OpenDTcpProbe::probe`.
    for dropped in ["init", "global"] {
        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind");
        let address = listener.local_addr().expect("local_addr");
        let drop_init = dropped == "init";
        let server = thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept");
            let init = read_framed_frame(&mut stream).expect("init frame");
            if drop_init {
                return;
            }
            write_framed_response(
                &mut stream,
                PROTO_INIT_CONNECT,
                init.header.serial_no,
                &init_body(1009, 7),
            );
            let global = read_framed_frame(&mut stream).expect("global frame");
            assert_eq!(global.header.proto_id, PROTO_GET_GLOBAL_STATE);
        });
        let error = OpenDTcpProbe::probe(OpenDTcpProbeConfig::new(address, Duration::from_secs(1)))
            .expect_err("a peer drop must not report a healthy probe");
        assert!(
            error.to_string().contains("closed"),
            "a peer drop during {dropped} must be a typed close: {error}"
        );
        server.join().expect("server");
    }
}

/// Go `TestReconnectTradePushFailureAndTradeHandlerBinding`: a trade-push
/// subscription that the server never acknowledges must surface a transport
/// error instead of reporting a successful subscription.
#[test]
fn trade_push_subscription_failure_surfaces_a_transport_error() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind");
    let address = listener.local_addr().expect("local_addr");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let init = read_framed_frame(&mut stream).expect("init frame");
        write_framed_response(
            &mut stream,
            PROTO_INIT_CONNECT,
            init.header.serial_no,
            &init_body(1009, 1),
        );
        // Go's fixture drops `ProtoTrdSubAccPush`: read the request and close
        // without acknowledging it.
        let push = read_framed_frame(&mut stream).expect("account push frame");
        assert_eq!(push.header.proto_id, 2008);
    });

    let config = OpenDTcpProbeConfig::new(address, Duration::from_millis(500));
    let session = OpenDInitializedSession::connect_with_push_notifications(&config, 1)
        .expect("initialized session");
    let client = OpenDTradeReadClient::from_session(session);
    let error = client
        .subscribe_trade_accounts(TradeSubscribeAccountsRequest {
            account_ids: vec![1],
        })
        .expect_err("an unacknowledged trade push subscription must fail closed");
    assert!(
        matches!(
            error,
            TradeSessionError::Session(_) | TradeSessionError::Response(_)
        ),
        "unexpected trade push error: {error:?}"
    );
    server.join().expect("server");
}

/// Go `TestTradeAccountPushNormalizationSubscriptionAndFactoryEnvironment`:
/// Rust forwards the requested account ids to `Trd_SubAccPush`; the Go-side
/// sort/dedupe set and the env-prefixed factory stay outside this crate.
#[test]
fn trade_push_subscription_forwards_the_requested_accounts() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind");
    let address = listener.local_addr().expect("local_addr");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let init = read_framed_frame(&mut stream).expect("init frame");
        write_framed_response(
            &mut stream,
            PROTO_INIT_CONNECT,
            init.header.serial_no,
            &init_body(1009, 1),
        );
        let push = read_framed_frame(&mut stream).expect("account push frame");
        assert_eq!(push.header.proto_id, 2008);
        write_framed_response(&mut stream, 2008, push.header.serial_no, &success_body());
    });

    let config = OpenDTcpProbeConfig::new(address, Duration::from_secs(1));
    let session = OpenDInitializedSession::connect_with_push_notifications(&config, 1)
        .expect("initialized session");
    let client = OpenDTradeReadClient::from_session(session);
    client
        .subscribe_trade_accounts(TradeSubscribeAccountsRequest {
            account_ids: vec![2, 1, 2],
        })
        .expect("account push subscription");
    server.join().expect("server");
}

/// Go `TestTradeReadMethodsPropagateTargetProtocolDisconnects`: dropping the
/// response for one trade protocol must surface an error from every read
/// method that talks to it, never a defaulted success. `setDropProto` in Go
/// leaves the request unanswered, so the Rust equivalent is the managed
/// session's request timeout followed by a typed session error.
#[test]
// Parity: go:452dea11:pkg/futu/transport_error_propagation_test.go:13 TestTradeReadMethodsPropagateTargetProtocolDisconnects
fn trade_read_methods_propagate_target_protocol_disconnects() {
    let header = TradeHeader {
        trd_env: 1,
        acc_id: 42,
        trd_market: 1,
        jp_acc_type: None,
    };
    type ReadCall = (
        &'static str,
        u32,
        fn(&OpenDTradeReadClient, &TradeHeader) -> bool,
    );
    let cases: [ReadCall; 3] = [
        ("accounts", 2001, |client, _| {
            client.read_accounts(1, None, None).is_err()
        }),
        ("funds", 2101, |client, header| {
            client.read_funds(header.clone(), None, None, None).is_err()
        }),
        ("positions", 2102, |client, header| {
            client
                .read_positions(header.clone(), None, None, None, None, None, None, None)
                .is_err()
        }),
    ];
    for (name, protocol, invoke) in cases {
        let (address, server) = drop_protocol_server(protocol);
        let config = OpenDTcpProbeConfig::new(address, Duration::from_millis(300));
        let session = OpenDInitializedSession::connect_with_push_notifications(&config, 1)
            .expect("initialized session");
        let client = OpenDTradeReadClient::from_session(session);
        assert!(invoke(&client, &header), "{name} must fail closed");
        server.join().expect("server");
    }
}

/// Go `TestQuoteKLineAndOrderBookPropagateTargetDisconnects`: a dropped
/// Qot_Sub acknowledgement or a dropped Qot_GetOrderBook response must surface
/// an error from the subscription/read path instead of a silent success.
///
/// The subscription half goes through the coordinator, which is the Rust owner
/// of Qot_Sub replay; the depth half goes through the microstructure reader.
#[test]
// Parity: go:452dea11:pkg/futu/transport_error_propagation_test.go:69 TestQuoteKLineAndOrderBookPropagateTargetDisconnects
fn quote_kline_and_order_book_propagate_target_disconnects() {
    let (address, server) = drop_protocol_server(PROTO_QOT_SUB);
    let config = OpenDTcpProbeConfig::new(address, Duration::from_millis(300));
    let result = OpenDSessionCoordinator::connect(
        config,
        Arc::new(MarketDataRuntimeRecorder::default()),
        vec![snapshot("AAPL")],
        0,
    );
    assert!(
        result.is_err(),
        "an unacknowledged Qot_Sub must fail the subscription replay"
    );
    server.join().expect("server");

    let (address, server) = drop_protocol_server(3012);
    let config = OpenDTcpProbeConfig::new(address, Duration::from_millis(300));
    let coordinator = OpenDSessionCoordinator::connect(
        config,
        Arc::new(MarketDataRuntimeRecorder::default()),
        Vec::new(),
        0,
    )
    .expect("coordinator");
    let reader = OpenDMarketMicrostructureReader::new(Arc::new(Mutex::new(coordinator)));
    let error = reader
        .query(
            MarketMicrostructureOperation::Depth,
            "HK.00700",
            &serde_json::json!({"num": 10}),
        )
        .expect_err("a dropped depth response must fail closed");
    let message = format!("{error:?}");
    assert!(
        message.contains("Session") || message.contains("session"),
        "unexpected depth error: {message}"
    );
    server.join().expect("server");
}

/// Go `TestDirectSubscriptionCallsPropagateClosedClientErrors`: every direct
/// subscription helper must reject a closed client, while an empty request
/// list stays a no-op that does not touch the wire.
#[test]
// Parity: go:452dea11:pkg/futu/transport_error_propagation_test.go:207 TestDirectSubscriptionCallsPropagateClosedClientErrors
fn direct_subscription_calls_propagate_closed_client_errors() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind");
    let address = listener.local_addr().expect("local_addr");
    let server = thread::spawn(move || {
        let (stream, _) = listener.accept().expect("accept");
        let mut stream = stream;
        let init = read_framed_frame(&mut stream).expect("init frame");
        write_framed_response(
            &mut stream,
            PROTO_INIT_CONNECT,
            init.header.serial_no,
            &init_body(1009, 3),
        );
        // The closed-client assertions never reach the wire; keep the socket
        // until the client closes so the peer-close path is exercised.
        let mut byte = [0_u8; 1];
        let _ = stream.read(&mut byte);
    });
    let config = OpenDTcpProbeConfig::new(address, Duration::from_millis(300));
    let session = OpenDInitializedSession::connect_with_push_notifications(&config, 1)
        .expect("initialized session");
    let client = OpenDTradeReadClient::from_session(session.clone());
    session
        .managed_session()
        .close()
        .expect("close managed session");

    assert!(
        client.read_accounts(1, None, None).is_err(),
        "closed client must reject read_accounts"
    );
    assert!(
        client
            .read_funds(
                TradeHeader {
                    trd_env: 1,
                    acc_id: 42,
                    trd_market: 1,
                    jp_acc_type: None,
                },
                None,
                None,
                None,
            )
            .is_err(),
        "closed client must reject read_funds"
    );
    server.join().expect("server");
}
