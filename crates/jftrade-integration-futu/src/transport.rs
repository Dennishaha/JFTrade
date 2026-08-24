use std::io::{self, Read, Write};
use std::net::{SocketAddr, TcpStream};
use std::time::Duration;

use thiserror::Error;

use crate::frame::{HEADER_LEN, MAX_BODY_LEN};
use crate::{FrameError, decode_frame, encode_frame};

pub trait OpenDTransport {
    type Error: std::error::Error + Send + Sync + 'static;

    fn exchange(&mut self, packet: &[u8]) -> Result<Vec<u8>, Self::Error>;
}

/// Synchronous loopback transport for the Futu OpenD framed TCP protocol.
///
/// The protocol client remains transport-agnostic; this adapter owns only the
/// socket and frame boundaries. Provider health, login state, subscriptions
/// and reconnect policy stay above this layer.
#[derive(Debug)]
pub struct OpenDTcpTransport {
    stream: TcpStream,
}

impl OpenDTcpTransport {
    pub fn connect(address: SocketAddr, timeout: Duration) -> Result<Self, TcpTransportError> {
        let stream = TcpStream::connect_timeout(&address, timeout)?;
        stream.set_read_timeout(Some(timeout))?;
        stream.set_write_timeout(Some(timeout))?;
        Ok(Self { stream })
    }

    pub fn from_stream(stream: TcpStream, timeout: Duration) -> Result<Self, TcpTransportError> {
        stream.set_read_timeout(Some(timeout))?;
        stream.set_write_timeout(Some(timeout))?;
        Ok(Self { stream })
    }
}

impl OpenDTransport for OpenDTcpTransport {
    type Error = TcpTransportError;

    fn exchange(&mut self, packet: &[u8]) -> Result<Vec<u8>, Self::Error> {
        self.stream.write_all(packet)?;
        let mut header = [0_u8; HEADER_LEN];
        self.stream.read_exact(&mut header)?;
        let mut body_len = [0_u8; 4];
        body_len.copy_from_slice(&header[12..16]);
        let body_len = u32::from_le_bytes(body_len) as usize;
        if body_len > MAX_BODY_LEN {
            return Err(TcpTransportError::Frame(FrameError::BodyTooLarge));
        }
        let mut response = Vec::with_capacity(HEADER_LEN + body_len);
        response.extend_from_slice(&header);
        response.resize(HEADER_LEN + body_len, 0);
        self.stream.read_exact(&mut response[HEADER_LEN..])?;
        decode_frame(&response)?;
        Ok(response)
    }
}

#[derive(Debug, Error)]
pub enum TransportError {
    #[error(transparent)]
    Frame(#[from] FrameError),
    #[error("opend transport failed: {0}")]
    Exchange(String),
    #[error("opend response protocol mismatch: expected {expected}, received {actual}")]
    ProtocolMismatch { expected: u32, actual: u32 },
    #[error("opend response serial mismatch: expected {expected}, received {actual}")]
    SerialMismatch { expected: u32, actual: u32 },
}

#[derive(Debug, Error)]
pub enum TcpTransportError {
    #[error("opend TCP transport I/O: {0}")]
    Io(#[from] io::Error),
    #[error(transparent)]
    Frame(#[from] FrameError),
}

pub struct OpenDClient<T> {
    transport: T,
    next_serial: u32,
}

impl<T: OpenDTransport> OpenDClient<T> {
    pub fn new(transport: T) -> Self {
        Self {
            transport,
            next_serial: 0,
        }
    }

    pub fn call(&mut self, proto_id: u32, protobuf_body: &[u8]) -> Result<Vec<u8>, TransportError> {
        self.next_serial = self.next_serial.saturating_add(1);
        let serial = self.next_serial;
        let request = encode_frame(proto_id, serial, protobuf_body)?;
        let response = self
            .transport
            .exchange(&request)
            .map_err(|error| TransportError::Exchange(error.to_string()))?;
        let frame = decode_frame(&response)?;
        if frame.header.proto_id != proto_id {
            return Err(TransportError::ProtocolMismatch {
                expected: proto_id,
                actual: frame.header.proto_id,
            });
        }
        if frame.header.serial_no != serial {
            return Err(TransportError::SerialMismatch {
                expected: serial,
                actual: frame.header.serial_no,
            });
        }
        Ok(frame.body)
    }

    pub fn into_inner(self) -> T {
        self.transport
    }
}

#[cfg(test)]
mod tests {
    use std::convert::Infallible;
    use std::io::{Read, Write};
    use std::net::TcpListener;
    use std::thread;
    use std::time::Duration;

    use super::*;

    struct Echo;

    impl OpenDTransport for Echo {
        type Error = Infallible;

        fn exchange(&mut self, packet: &[u8]) -> Result<Vec<u8>, Self::Error> {
            Ok(packet.to_vec())
        }
    }

    #[test]
    fn calls_match_protocol_and_serial() {
        let mut client = OpenDClient::new(Echo);
        assert_eq!(client.call(3004, b"protobuf").expect("call"), b"protobuf");
        assert_eq!(client.call(3006, b"next").expect("call"), b"next");
    }

    #[test]
    fn tcp_transport_round_trips_one_framed_exchange_with_deadlines() {
        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
        let address = listener.local_addr().expect("listener address");
        let server = thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept");
            let mut header = [0_u8; HEADER_LEN];
            stream.read_exact(&mut header).expect("request header");
            let mut body_len = [0_u8; 4];
            body_len.copy_from_slice(&header[12..16]);
            let body_len = u32::from_le_bytes(body_len) as usize;
            let mut body = vec![0_u8; body_len];
            stream.read_exact(&mut body).expect("request body");
            let request = [&header[..], &body[..]].concat();
            let frame = decode_frame(&request).expect("request frame");
            let response = encode_frame(frame.header.proto_id, frame.header.serial_no, b"ok")
                .expect("response frame");
            stream.write_all(&response).expect("response");
        });

        let transport = OpenDTcpTransport::connect(address, Duration::from_secs(1))
            .expect("connect mock OpenD");
        let mut client = OpenDClient::new(transport);
        assert_eq!(client.call(1001, b"hello").expect("OpenD call"), b"ok");
        server.join().expect("server thread");
    }
}
