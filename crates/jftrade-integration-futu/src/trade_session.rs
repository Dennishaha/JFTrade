use std::sync::Arc;

use thiserror::Error;

use crate::health::OpenDInitializedSession;
use crate::managed_session::{OpenDManagedSession, OpenDManagedSessionError};
use crate::session_coordinator::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};
use crate::trade_proto::{
    ResponseError, trd_common, trd_get_acc_list, trd_get_funds, trd_get_order_fill_list,
    trd_get_order_list, trd_get_position_list,
};

#[derive(Debug, Error)]
pub enum TradeSessionError {
    #[error("OpenD trade session call failed: {0}")]
    Session(#[from] OpenDManagedSessionError),
    #[error("OpenD trade response rejected: {0}")]
    Response(#[from] ResponseError),
    #[error("OpenD trade coordinator is unavailable: {0}")]
    Coordinator(#[from] OpenDSessionCoordinatorError),
}

/// Authenticated, read-only Futu trade RPC client.
///
/// The client delegates framing, serial correlation, timeout and close
/// handling to the single-reader managed session. It does not own reconnects
/// or any trading state; callers obtain it from the production coordinator.
#[derive(Clone)]
pub struct OpenDTradeReadClient {
    session: Arc<OpenDManagedSession>,
}

impl OpenDTradeReadClient {
    pub fn from_session(session: OpenDInitializedSession) -> Self {
        Self {
            session: session.managed_session_handle(),
        }
    }

    pub fn from_coordinator(
        coordinator: &OpenDSessionCoordinator,
    ) -> Result<Self, TradeSessionError> {
        Ok(Self::from_session(coordinator.session()?.clone()))
    }

    #[cfg(test)]
    fn from_managed_session(session: Arc<OpenDManagedSession>) -> Self {
        Self { session }
    }

    pub fn get_account_list(
        &self,
        request: trd_get_acc_list::Request,
    ) -> Result<trd_get_acc_list::S2c, TradeSessionError> {
        let body = self.call(
            trd_get_acc_list::PROTOCOL_ID,
            &trd_get_acc_list::encode_request(&request),
        )?;
        Ok(trd_get_acc_list::decode_response(&body)?)
    }

    pub fn get_funds(
        &self,
        request: trd_get_funds::Request,
    ) -> Result<trd_common::Funds, TradeSessionError> {
        let body = self.call(
            trd_get_funds::PROTOCOL_ID,
            &trd_get_funds::encode_request(&request),
        )?;
        Ok(trd_get_funds::decode_response(&body)?)
    }

    pub fn get_position_list(
        &self,
        request: trd_get_position_list::Request,
    ) -> Result<trd_get_position_list::S2c, TradeSessionError> {
        let body = self.call(
            trd_get_position_list::PROTOCOL_ID,
            &trd_get_position_list::encode_request(&request),
        )?;
        Ok(trd_get_position_list::decode_response(&body)?)
    }

    pub fn get_order_list(
        &self,
        request: trd_get_order_list::Request,
    ) -> Result<trd_get_order_list::S2c, TradeSessionError> {
        let body = self.call(
            trd_get_order_list::PROTOCOL_ID,
            &trd_get_order_list::encode_request(&request),
        )?;
        Ok(trd_get_order_list::decode_response(&body)?)
    }

    pub fn get_order_fill_list(
        &self,
        request: trd_get_order_fill_list::Request,
    ) -> Result<trd_get_order_fill_list::S2c, TradeSessionError> {
        let body = self.call(
            trd_get_order_fill_list::PROTOCOL_ID,
            &trd_get_order_fill_list::encode_request(&request),
        )?;
        Ok(trd_get_order_fill_list::decode_response(&body)?)
    }

    pub fn call(&self, protocol: u32, request_body: &[u8]) -> Result<Vec<u8>, TradeSessionError> {
        Ok(self.session.call(protocol, request_body)?)
    }
}

/// Builds the common trade header used by funds, positions, orders and fills.
pub const fn trade_header(trd_env: i32, acc_id: u64, trd_market: i32) -> trd_common::TrdHeader {
    trd_common::TrdHeader {
        trd_env,
        acc_id,
        trd_market,
        jp_acc_type: None,
    }
}

#[cfg(test)]
mod tests {
    use std::io::{Read, Write};
    use std::net::TcpListener;
    use std::thread;
    use std::time::Duration;

    use prost::Message;

    use super::*;
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
            let decoded =
                trd_get_acc_list::Request::decode(request.body.as_slice()).expect("request");
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
}
