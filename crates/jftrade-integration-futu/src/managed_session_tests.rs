use std::io::{Read, Write};
use std::net::{TcpListener, TcpStream};
use std::sync::Arc;
use std::sync::mpsc;
use std::thread;
use std::time::Duration;

use super::*;
use crate::transport::read_framed_frame;
use crate::{PROTO_UPDATE_BASIC_QOT, decode_frame};

const TIMEOUT: Duration = Duration::from_secs(1);

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
