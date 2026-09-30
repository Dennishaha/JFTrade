use std::io::{Read, Write};
use std::net::{TcpListener, TcpStream};
use std::sync::Arc;
use std::sync::mpsc;
use std::thread;
use std::time::Duration;

use super::*;
use crate::frame::{HEADER_LEN, MAX_BODY_LEN};
use crate::transport::read_framed_frame;
use crate::{PROTO_UPDATE_BASIC_QOT, decode_frame};

const TIMEOUT: Duration = Duration::from_secs(1);

/// Records every observed OpenD RPC so the Go `Client.Call` observability
/// correlation can be asserted without the API transport crate.
#[derive(Debug, Default)]
struct RecordingOpenDCallObserver {
    records: std::sync::Mutex<Vec<jftrade_kernel::OpenDCallRecord>>,
}

impl jftrade_kernel::OpenDCallObserver for RecordingOpenDCallObserver {
    fn record_open_d_call(&self, record: &jftrade_kernel::OpenDCallRecord) {
        self.records.lock().expect("records").push(record.clone());
    }
}

#[test]
fn call_failure_records_request_correlation_against_the_observer() {
    // Parity: go:452dea11:pkg/futu/opend/client_test.go:77
    // TestCallFailureRecordsRequestCorrelation.
    //
    // Go's `Client.Call` always calls `observability.RecordOpenDCall` with
    // `proto_<id>`, the surrounding request ID and the failure text. A closed
    // Rust session must report the same correlation instead of dropping the
    // failure at the transport boundary.
    let observer = Arc::new(RecordingOpenDCallObserver::default());
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("listener address");
    let server = thread::spawn(move || {
        let (stream, _) = listener.accept().expect("accept");
        wait_for_peer_close(stream);
    });
    let session = OpenDManagedSession::connect(address, TIMEOUT, 51)
        .expect("session")
        .with_observer(Arc::clone(&observer) as jftrade_kernel::SharedOpenDCallObserver)
        .with_request_id("request-opend-1");
    assert!(session.close().expect("close"));
    let error = session
        .call(crate::PROTO_INIT_CONNECT, b"request")
        .expect_err("closed session must reject the call");
    assert!(matches!(error, OpenDManagedSessionError::Closed(_)));
    server.join().expect("server thread");

    let records = observer.records.lock().expect("records");
    assert_eq!(
        records.len(),
        1,
        "closed call must be observed exactly once"
    );
    assert_eq!(records[0].operation, "proto_1001");
    assert_eq!(records[0].request_id, "request-opend-1");
    assert!(
        records[0]
            .error
            .as_deref()
            .is_some_and(|error| !error.is_empty()),
        "failed call must carry the failure text: {records:?}"
    );
}

#[test]
fn probe_config_observer_receives_pushed_call_outcomes() {
    // Parity: go:452dea11:pkg/futu/opend/client_test.go:77 —
    // the composition-root side of the same contract. A session created from a
    // config carrying the observer must report the InitConnect handshake RPC
    // with the configured request ID, covering both success and failure text.
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("listener address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_framed_frame(&mut stream).expect("init request");
        write_frame(
            &mut stream,
            request.header.proto_id,
            request.header.serial_no,
            &[],
        );
        wait_for_peer_close(stream);
    });
    let observer = Arc::new(RecordingOpenDCallObserver::default());
    let config = crate::OpenDTcpProbeConfig::new(address, TIMEOUT)
        .with_open_d_call_observer(Arc::clone(&observer) as jftrade_kernel::SharedOpenDCallObserver)
        .with_request_id("request-opend-config");
    drop(crate::OpenDInitializedSession::connect_with_push_notifications(&config, 53));
    server.join().expect("server thread");

    let records = observer.records.lock().expect("records");
    assert_eq!(records.len(), 1, "handshake RPC must be observed once");
    assert_eq!(records[0].operation, "proto_1001");
    assert_eq!(records[0].request_id, "request-opend-config");
    assert!(
        records[0].error.is_none(),
        "the transport call itself succeeded; protocol decoding is owned by the caller"
    );
}

#[test]
fn rpc_waiter_survives_unsolicited_push_before_response() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("listener address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_framed_frame(&mut stream).expect("request");
        write_frame(&mut stream, PROTO_UPDATE_BASIC_QOT, 0, b"push");
        write_frame(
            &mut stream,
            request.header.proto_id,
            request.header.serial_no,
            b"response",
        );
        wait_for_peer_close(stream);
    });

    let session = OpenDManagedSession::connect(address, TIMEOUT, 7).expect("session");
    assert_eq!(session.call(1001, b"request").expect("call"), b"response");
    assert_eq!(
        session.receive_event_timeout(TIMEOUT).expect("push event"),
        OpenDSessionEvent::UnsolicitedFrame {
            generation: 7,
            frame: frame(PROTO_UPDATE_BASIC_QOT, 0, b"push"),
        }
    );
    assert!(session.close().expect("close"));
    server.join().expect("server thread");
}

#[test]
// Parity: go:452dea11:pkg/futu/opend/client_test.go:405 TestCallIgnoresMismatchedProtoOnSameSerial
fn same_serial_with_wrong_protocol_is_unsolicited_until_exact_response_arrives() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("listener address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_framed_frame(&mut stream).expect("request");
        write_frame(
            &mut stream,
            PROTO_UPDATE_BASIC_QOT,
            request.header.serial_no,
            b"not-the-response",
        );
        write_frame(
            &mut stream,
            request.header.proto_id,
            request.header.serial_no,
            b"response",
        );
        wait_for_peer_close(stream);
    });

    let session = OpenDManagedSession::connect(address, TIMEOUT, 9).expect("session");
    assert_eq!(session.call(1001, b"request").expect("call"), b"response");
    assert_eq!(
        session.receive_event_timeout(TIMEOUT).expect("push event"),
        OpenDSessionEvent::UnsolicitedFrame {
            generation: 9,
            frame: frame(PROTO_UPDATE_BASIC_QOT, 1, b"not-the-response"),
        }
    );
    assert!(session.close().expect("close"));
    server.join().expect("server thread");
}

#[test]
// Parity: go:452dea11:pkg/futu/opend/client_test.go:181 TestCallRoundTrip
fn concurrent_rpc_responses_are_routed_by_protocol_and_serial() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("listener address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let first = read_framed_frame(&mut stream).expect("first request");
        let second = read_framed_frame(&mut stream).expect("second request");
        write_frame(
            &mut stream,
            second.header.proto_id,
            second.header.serial_no,
            &second.body,
        );
        write_frame(
            &mut stream,
            first.header.proto_id,
            first.header.serial_no,
            &first.body,
        );
        wait_for_peer_close(stream);
    });

    let session = Arc::new(
        OpenDManagedSession::connect(address, TIMEOUT, 11).expect("managed OpenD session"),
    );
    let first_session = Arc::clone(&session);
    let first = thread::spawn(move || first_session.call(3004, b"first"));
    let second_session = Arc::clone(&session);
    let second = thread::spawn(move || second_session.call(3006, b"second"));
    assert_eq!(
        first.join().expect("first call").expect("first response"),
        b"first"
    );
    assert_eq!(
        second
            .join()
            .expect("second call")
            .expect("second response"),
        b"second"
    );
    assert!(session.close().expect("close"));
    server.join().expect("server thread");
}

#[test]
fn peer_eof_fans_out_to_pending_call_and_closed_event() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("listener address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        read_framed_frame(&mut stream).expect("request");
    });

    let session = OpenDManagedSession::connect(address, TIMEOUT, 13).expect("session");
    assert!(matches!(
        session.call(1002, b"request"),
        Err(OpenDManagedSessionError::Closed(
            OpenDSessionCloseReason::PeerClosed
        ))
    ));
    assert_eq!(
        session
            .receive_event_timeout(TIMEOUT)
            .expect("closed event"),
        OpenDSessionEvent::Closed {
            generation: 13,
            reason: OpenDSessionCloseReason::PeerClosed,
        }
    );
    assert!(session.is_closed());
    assert_eq!(
        session.close_reason().expect("close reason"),
        Some(OpenDSessionCloseReason::PeerClosed)
    );
    assert!(!session.close().expect("idempotent close"));
    server.join().expect("server thread");
}

#[test]
// Parity: go:452dea11:pkg/futu/opend/client_test.go:206 TestRequestTimeout
fn response_after_request_timeout_is_not_delivered_to_a_stale_waiter() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("listener address");
    let (request_sender, request_receiver) = mpsc::sync_channel(1);
    let (release_sender, release_receiver) = mpsc::sync_channel(1);
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_framed_frame(&mut stream).expect("request");
        request_sender
            .send(request.clone())
            .expect("request signal");
        release_receiver.recv().expect("release response");
        write_frame(
            &mut stream,
            request.header.proto_id,
            request.header.serial_no,
            b"late",
        );
        wait_for_peer_close(stream);
    });

    let timeout = Duration::from_millis(25);
    let session = Arc::new(OpenDManagedSession::connect(address, timeout, 15).expect("session"));
    let call_session = Arc::clone(&session);
    let call = thread::spawn(move || call_session.call(1002, b"request"));
    let request = request_receiver.recv_timeout(TIMEOUT).expect("request");
    assert!(matches!(
        call.join().expect("call thread"),
        Err(OpenDManagedSessionError::RequestTimeout {
            protocol: 1002,
            serial: 1,
        })
    ));
    release_sender.send(()).expect("release response");
    assert_eq!(
        session
            .receive_event_timeout(TIMEOUT)
            .expect("late response event"),
        OpenDSessionEvent::UnsolicitedFrame {
            generation: 15,
            frame: request_response_frame(&request, b"late"),
        }
    );
    assert!(session.close().expect("close"));
    server.join().expect("server thread");
}

#[test]
fn close_is_idempotent_and_joins_the_single_reader() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("listener address");
    let (accepted_sender, accepted_receiver) = mpsc::sync_channel(1);
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        accepted_sender.send(()).expect("accepted signal");
        let mut byte = [0_u8; 1];
        assert_eq!(stream.read(&mut byte).expect("read peer close"), 0);
    });

    let session = OpenDManagedSession::connect(address, TIMEOUT, 17).expect("session");
    accepted_receiver.recv_timeout(TIMEOUT).expect("accepted");
    assert!(session.close().expect("first close"));
    assert!(!session.close().expect("second close"));
    assert_eq!(
        session
            .receive_event_timeout(TIMEOUT)
            .expect("local close event"),
        OpenDSessionEvent::Closed {
            generation: 17,
            reason: OpenDSessionCloseReason::Local,
        }
    );
    server.join().expect("server thread");
}

#[test]
fn closed_session_rejects_connect_like_rpcs_before_touching_the_transport() {
    // Parity: go:452dea11:pkg/futu/opend/client_transport_boundaries_test.go:69 TestConnectSurfacesInvalidAddressAndClosedClient
    //
    // Go's closed client returns ErrClosed from Connect and from every typed
    // read helper. The Rust equivalent boundary is that a closed managed
    // session rejects new RPCs before writing, which is how the transport
    // failure surfaces to callers.
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("listener address");
    let server = thread::spawn(move || {
        let (stream, _) = listener.accept().expect("accept");
        wait_for_peer_close(stream);
    });

    let session = OpenDManagedSession::connect(address, TIMEOUT, 21).expect("session");
    assert!(session.close().expect("close"));
    assert!(matches!(
        session.call(crate::PROTO_INIT_CONNECT, b"init"),
        Err(OpenDManagedSessionError::Closed(
            OpenDSessionCloseReason::Local
        ))
    ));
    assert!(matches!(
        session.call(crate::PROTO_GET_GLOBAL_STATE, b"global-state"),
        Err(OpenDManagedSessionError::Closed(
            OpenDSessionCloseReason::Local
        ))
    ));
    server.join().expect("server thread");
}

#[test]
fn closed_session_reports_subscribe_transport_failure() {
    // Parity: go:452dea11:pkg/futu/opend/client_transport_boundaries_test.go:327 TestSubscribeQuotesReportsTransportFailure
    //
    // Go's SubscribeQuotes returns ErrClosed when there is no live connection.
    // The Rust subscription path goes through the managed session, so a closed
    // session must reject the Qot_Sub RPC with the typed closed reason.
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("listener address");
    let server = thread::spawn(move || {
        let (stream, _) = listener.accept().expect("accept");
        wait_for_peer_close(stream);
    });

    let session = OpenDManagedSession::connect(address, TIMEOUT, 47).expect("session");
    assert!(session.close().expect("close"));
    assert!(matches!(
        session.call(crate::PROTO_QOT_SUB, b"subscribe"),
        Err(OpenDManagedSessionError::Closed(
            OpenDSessionCloseReason::Local
        ))
    ));
    server.join().expect("server thread");
}

#[test]
fn close_shuts_down_the_transport_before_joining_the_reader() {
    // Parity: go:452dea11:pkg/futu/opend/client_transport_boundaries_test.go:107 TestClientCloseWaitsForReadWorkerAfterClosingTransport
    //
    // The Rust reader is parked inside a blocking frame read. `close` must
    // first shut the socket down and then join that worker, so the peer sees
    // EOF only once the transport is closed, and `close` never returns while
    // the reader is still live.
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("listener address");
    let (accepted_sender, accepted_receiver) = mpsc::sync_channel(1);
    let (eof_sender, eof_receiver) = mpsc::sync_channel(1);
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        accepted_sender.send(()).expect("accepted signal");
        let mut byte = [0_u8; 1];
        assert_eq!(stream.read(&mut byte).expect("read peer close"), 0);
        eof_sender.send(()).expect("eof signal");
    });

    let session = OpenDManagedSession::connect(address, TIMEOUT, 23).expect("session");
    accepted_receiver.recv_timeout(TIMEOUT).expect("accepted");
    assert!(session.close().expect("close joins the reader"));
    assert!(session.is_closed());
    eof_receiver
        .recv_timeout(TIMEOUT)
        .expect("the transport was shut down before close returned");
    server.join().expect("server thread");
}

#[test]
fn close_drains_pending_requests_when_the_peer_closes() {
    // Parity: go:452dea11:pkg/futu/opend/client_transport_boundaries_test.go:143 TestClientCloseDrainsPendingRequestsAfterBestEffortCloseFailure
    //
    // Go closes every pending response channel even when the best-effort
    // network close fails. Rust publishes a typed close reason to every waiter;
    // this pins that a pending call resolves instead of hanging.
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("listener address");
    let (request_sender, request_receiver) = mpsc::sync_channel(1);
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_framed_frame(&mut stream).expect("request");
        request_sender.send(request).expect("request signal");
        drop(stream);
    });

    let session = Arc::new(OpenDManagedSession::connect(address, TIMEOUT, 25).expect("session"));
    let call_session = Arc::clone(&session);
    let call = thread::spawn(move || call_session.call(3004, b"pending"));
    request_receiver.recv_timeout(TIMEOUT).expect("request");
    assert!(matches!(
        call.join().expect("call thread"),
        Err(OpenDManagedSessionError::Closed(
            OpenDSessionCloseReason::PeerClosed
        ))
    ));
    assert!(!session.close().expect("idempotent close"));
    server.join().expect("server thread");
}

#[test]
fn keep_alive_worker_sends_frames_and_stops_on_close() {
    // Parity: go:452dea11:pkg/futu/opend/client_transport_boundaries_test.go:166 TestKeepAliveLoopHalvesLongIntervalsAndStopsForClosedClient
    // Parity: go:452dea11:pkg/futu/opend/client_test.go:243 TestKeepAliveFailureClosesClient
    //
    // The observed request must be protocol 1004 with the encoded `time`
    // field. After close the worker must stop issuing calls instead of looping
    // until the process exits. `keep_alive_tick_halves_long_intervals` pins the
    // halving rule itself.
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("listener address");
    let (request_sender, request_receiver) = mpsc::sync_channel(8);
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        while let Ok(request) = read_framed_frame(&mut stream) {
            request_sender.send(request).expect("request signal");
            let response =
                encode_frame(crate::PROTO_KEEP_ALIVE, 0, &[0x08, 0x01]).expect("keep-alive body");
            if stream.write_all(&response).is_err() {
                return;
            }
        }
    });

    let session = OpenDManagedSession::connect(address, TIMEOUT, 27).expect("session");
    assert!(!session.keep_alive_started());
    assert!(session.start_keep_alive(Duration::from_millis(200)));
    assert!(session.keep_alive_started());
    // A second start must not spawn a competing worker.
    assert!(!session.start_keep_alive(Duration::from_millis(200)));

    assert_eq!(
        keep_alive_tick(Duration::from_secs(2)),
        Duration::from_secs(1)
    );
    assert_eq!(
        keep_alive_tick(Duration::from_millis(500)),
        Duration::from_millis(500)
    );

    let request = request_receiver
        .recv_timeout(TIMEOUT)
        .expect("keep-alive request");
    assert_eq!(request.header.proto_id, crate::PROTO_KEEP_ALIVE);
    assert!(!request.body.is_empty(), "keep-alive encodes its UTC time");

    assert!(session.close().expect("close"));
    while request_receiver
        .recv_timeout(Duration::from_millis(50))
        .is_ok()
    {}
    server.join().expect("server thread");
}

#[test]
fn start_keep_alive_rejects_a_closed_session() {
    // Parity: go:452dea11:pkg/futu/opend/client_transport_boundaries_test.go:182 TestStartKeepAliveRejectsClosedClientWithoutOwningWorker
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("listener address");
    let server = thread::spawn(move || {
        let (stream, _) = listener.accept().expect("accept");
        wait_for_peer_close(stream);
    });

    let session = OpenDManagedSession::connect(address, TIMEOUT, 29).expect("session");
    assert!(session.close().expect("close"));
    assert!(!session.start_keep_alive(Duration::from_secs(30)));
    assert!(!session.keep_alive_started());
    server.join().expect("server thread");
}

#[test]
fn start_keep_alive_ignores_non_positive_intervals() {
    // Parity: go:452dea11:pkg/futu/opend/client_test.go:104 TestStartKeepAliveIgnoresNonPositiveIntervalsWithoutConsumingStart
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("listener address");
    let server = thread::spawn(move || {
        let (stream, _) = listener.accept().expect("accept");
        wait_for_peer_close(stream);
    });

    let session = OpenDManagedSession::connect(address, TIMEOUT, 31).expect("session");
    assert!(!session.start_keep_alive(Duration::ZERO));
    assert!(!session.keep_alive_started());
    assert!(session.close().expect("close"));
    server.join().expect("server thread");
}

#[test]
fn reader_discards_undecodable_frames_and_closes_on_truncation() {
    // Parity: go:452dea11:pkg/futu/opend/client_transport_boundaries_test.go:290 TestReadLoopDiscardsInvalidFramesAndClosesOnTruncation
    //
    // Go's read loop skips a frame whose body fails wire validation and keeps
    // the session alive, then closes the client when a declared body is
    // truncated by peer EOF.
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("listener address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        // Bad magic: a complete, length-consistent header that cannot be a
        // Futu frame.
        let bad_packet = [0_u8; HEADER_LEN];
        stream.write_all(&bad_packet).expect("bad packet");
        // Truncated frame: declares a one-byte body and then drops the peer.
        let mut truncated_header = [0_u8; HEADER_LEN];
        truncated_header[0] = b'F';
        truncated_header[1] = b'T';
        truncated_header[12..16].copy_from_slice(&1_u32.to_le_bytes());
        stream
            .write_all(&truncated_header)
            .expect("truncated header");
    });

    let session = OpenDManagedSession::connect(address, TIMEOUT, 33).expect("session");
    assert_eq!(
        session
            .receive_event_timeout(TIMEOUT)
            .expect("closed event"),
        OpenDSessionEvent::Closed {
            generation: 33,
            reason: OpenDSessionCloseReason::PeerClosed,
        }
    );
    assert!(session.is_closed());
    server.join().expect("server thread");
}

#[test]
fn reader_closes_on_oversized_declared_body() {
    // Parity: go:452dea11:pkg/futu/opend/client_transport_boundaries_test.go:263 TestReadLoopClosesConnectionOnOversizedFrame
    //
    // A header that declares more than 32MiB must fail the pending request and
    // close the session instead of allocating an attacker-controlled buffer.
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("listener address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_framed_frame(&mut stream).expect("request");
        let mut oversized_header = [0_u8; HEADER_LEN];
        oversized_header[0] = b'F';
        oversized_header[1] = b'T';
        oversized_header[2..6].copy_from_slice(&request.header.proto_id.to_le_bytes());
        oversized_header[8..12].copy_from_slice(&request.header.serial_no.to_le_bytes());
        oversized_header[12..16].copy_from_slice(&(MAX_BODY_LEN as u32 + 1).to_le_bytes());
        stream
            .write_all(&oversized_header)
            .expect("oversized header");
        // The peer must observe the session close rather than stay writable.
        let mut byte = [0_u8; 1];
        let _ = stream.read(&mut byte);
    });

    let session = OpenDManagedSession::connect(address, TIMEOUT, 35).expect("session");
    let call_session = Arc::clone(&Arc::new(session));
    let call = thread::spawn(move || call_session.call(1001, b"request"));
    assert!(matches!(
        call.join().expect("call thread"),
        Err(OpenDManagedSessionError::Closed(
            OpenDSessionCloseReason::InvalidFrame(FrameError::BodyTooLarge)
        ))
    ));
    server.join().expect("server thread");
}

#[test]
fn dispatch_drops_a_duplicate_response_when_the_pending_buffer_is_full() {
    // Parity: go:452dea11:pkg/futu/opend/client_transport_boundaries_test.go:315 TestDispatchDropsDuplicateResponseWhenPendingBufferIsFull
    //
    // The matched pending call is removed before the buffered response is
    // consumed, so a duplicate frame with the same protocol/serial becomes an
    // unsolicited event instead of blocking the reader or resolving twice.
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("listener address");
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_framed_frame(&mut stream).expect("request");
        write_frame(
            &mut stream,
            request.header.proto_id,
            request.header.serial_no,
            b"first-response",
        );
        write_frame(
            &mut stream,
            request.header.proto_id,
            request.header.serial_no,
            b"duplicate",
        );
        wait_for_peer_close(stream);
    });

    let session = OpenDManagedSession::connect(address, TIMEOUT, 37).expect("session");
    assert_eq!(
        session
            .call(crate::PROTO_INIT_CONNECT, b"request")
            .expect("call"),
        b"first-response"
    );
    assert_eq!(
        session
            .receive_event_timeout(TIMEOUT)
            .expect("duplicate response event"),
        OpenDSessionEvent::UnsolicitedFrame {
            generation: 37,
            frame: frame(crate::PROTO_INIT_CONNECT, 1, b"duplicate"),
        }
    );
    assert!(session.close().expect("close"));
    server.join().expect("server thread");
}

#[test]
fn call_rejects_oversized_payload_before_touching_the_socket() {
    // Parity: go:452dea11:pkg/futu/opend/client_transport_boundaries_test.go:217 TestCallFrameRejectsMarshalAndOversizedPayloads
    //
    // Go rejects the marshal failure and the >32MiB body before any network
    // write. Rust has no reflection-based marshal failure on this path, so the
    // encode-side size guard is the equivalent boundary: the caller sees the
    // typed frame error, and the request path stays usable afterwards.
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("listener address");
    let (request_sender, request_receiver) = mpsc::sync_channel(2);
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let first = read_framed_frame(&mut stream).expect("first request");
        request_sender.send(first).expect("request signal");
        wait_for_peer_close(stream);
    });

    let session = OpenDManagedSession::connect(address, TIMEOUT, 39).expect("session");
    let oversized = vec![0_u8; MAX_BODY_LEN + 1];
    assert!(matches!(
        session.call(crate::PROTO_INIT_CONNECT, &oversized),
        Err(OpenDManagedSessionError::Frame(FrameError::BodyTooLarge))
    ));
    let session = Arc::new(session);
    let call_session = Arc::clone(&session);
    let call = thread::spawn(move || call_session.call(crate::PROTO_INIT_CONNECT, b"valid"));
    let request = request_receiver.recv_timeout(TIMEOUT).expect("request");
    // The request actually reaching the peer is the valid one, proving the
    // oversized call never wrote to the socket.
    assert_eq!(request.body, b"valid");
    assert_eq!(request.header.proto_id, crate::PROTO_INIT_CONNECT);
    // The peer never answers, so this valid call resolves through its request
    // timeout instead of hanging. Either way the oversized call must not have
    // written, which the `valid` body above proves.
    assert!(call.join().expect("call thread").is_err());
    let _ = session.close();
    server.join().expect("server thread");
}

#[test]
fn call_reports_write_failure_and_closed_session_to_the_waiter() {
    // Parity: go:452dea11:pkg/futu/opend/client_transport_boundaries_test.go:230 TestCallFrameReturnsWriteAndCloseFailures
    //
    // Sub-case 1: writing to a peer that already closed fails the call.
    // Sub-case 2: closing the session while a call waits releases the waiter
    // with the typed close reason instead of hanging until the timeout.
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("listener address");
    let (accepted_sender, accepted_receiver) = mpsc::sync_channel(1);
    let server = thread::spawn(move || {
        let (stream, _) = listener.accept().expect("accept");
        accepted_sender.send(()).expect("accepted signal");
        drop(stream);
    });
    let session = OpenDManagedSession::connect(address, TIMEOUT, 41).expect("session");
    accepted_receiver.recv_timeout(TIMEOUT).expect("accepted");
    // Give the peer drop time to surface as a write failure or a read close;
    // both are valid transport outcomes for this boundary.
    let outcome = session.call(crate::PROTO_INIT_CONNECT, b"request");
    assert!(matches!(
        outcome,
        Err(OpenDManagedSessionError::Io(_))
            | Err(OpenDManagedSessionError::Closed(
                OpenDSessionCloseReason::PeerClosed | OpenDSessionCloseReason::Transport(_)
            ))
    ));
    server.join().expect("server thread");

    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("listener address");
    let (request_sender, request_receiver) = mpsc::sync_channel(1);
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_framed_frame(&mut stream).expect("request");
        request_sender.send(request).expect("request signal");
        wait_for_peer_close(stream);
    });
    let session = Arc::new(OpenDManagedSession::connect(address, TIMEOUT, 43).expect("session"));
    let call_session = Arc::clone(&session);
    let call = thread::spawn(move || call_session.call(crate::PROTO_INIT_CONNECT, b"request"));
    request_receiver.recv_timeout(TIMEOUT).expect("request");
    assert!(session.close().expect("close"));
    assert!(matches!(
        call.join().expect("call thread"),
        Err(OpenDManagedSessionError::Closed(
            OpenDSessionCloseReason::Local
        ))
    ));
    server.join().expect("server thread");
}

#[test]
fn keep_alive_failure_closes_the_session() {
    // Parity: go:452dea11:pkg/futu/opend/client_test.go:243 TestKeepAliveFailureClosesClient
    //
    // A stalled OpenD (peer accepts the heartbeat but never answers) must make
    // the keep-alive worker terminate the session, so later calls fail closed
    // instead of silently trusting a dead socket.
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("listener address");
    let (request_sender, request_receiver) = mpsc::sync_channel(1);
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let request = read_framed_frame(&mut stream).expect("keep-alive request");
        request_sender.send(request).expect("request signal");
        wait_for_peer_close(stream);
    });

    let session = OpenDManagedSession::connect(address, TIMEOUT, 45).expect("session");
    assert!(session.start_keep_alive(Duration::from_millis(50)));
    let request = request_receiver.recv_timeout(TIMEOUT).expect("request");
    assert_eq!(request.header.proto_id, crate::PROTO_KEEP_ALIVE);
    let deadline = std::time::Instant::now() + TIMEOUT;
    while !session.is_closed() && std::time::Instant::now() < deadline {
        thread::sleep(Duration::from_millis(5));
    }
    assert!(
        session.is_closed(),
        "stalled keep-alive must close the session"
    );
    assert!(matches!(
        session.close_reason().expect("close reason"),
        Some(OpenDSessionCloseReason::Transport(_))
    ));
    assert!(matches!(
        session.call(crate::PROTO_INIT_CONNECT, b"after-keepalive-failure"),
        Err(OpenDManagedSessionError::Closed(_))
    ));
    server.join().expect("server thread");
}

fn write_frame(stream: &mut TcpStream, protocol: u32, serial: u32, body: &[u8]) {
    stream
        .write_all(&encode_frame(protocol, serial, body).expect("encode frame"))
        .expect("write frame");
}

fn frame(protocol: u32, serial: u32, body: &[u8]) -> Frame {
    decode_frame(&encode_frame(protocol, serial, body).expect("encode frame"))
        .expect("decode frame")
}

fn request_response_frame(request: &Frame, body: &[u8]) -> Frame {
    frame(request.header.proto_id, request.header.serial_no, body)
}

fn wait_for_peer_close(mut stream: TcpStream) {
    let mut byte = [0_u8; 1];
    assert_eq!(stream.read(&mut byte).expect("read peer close"), 0);
}
