use thiserror::Error;

use crate::{FrameError, decode_frame, encode_frame};

pub trait OpenDTransport {
    type Error: std::error::Error + Send + Sync + 'static;

    fn exchange(&mut self, packet: &[u8]) -> Result<Vec<u8>, Self::Error>;
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
}
