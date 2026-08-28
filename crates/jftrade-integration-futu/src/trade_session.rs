use std::sync::Arc;

use thiserror::Error;

use crate::health::OpenDInitializedSession;
use crate::managed_session::{OpenDManagedSession, OpenDManagedSessionError};
use crate::session_coordinator::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};
use crate::trade_proto::{
    ResponseError, trd_common, trd_flow_summary, trd_get_acc_list, trd_get_funds,
    trd_get_order_fill_list, trd_get_order_list, trd_get_position_list,
};
use crate::trade_snapshots::{
    TradeAccountSnapshot, TradeCashFlowSnapshot, TradeFillSnapshot, TradeFilter,
    TradeFundsSnapshot, TradeHeader, TradeOrderSnapshot, TradePositionSnapshot, account_projection,
    cash_flows_projection, fills_projection, funds_projection, orders_projection,
    positions_projection,
};

/// Engine-facing read contract for the authenticated OpenD trade account.
///
/// This trait deliberately contains only neutral request and response types;
/// generated protobuf messages stay private to this integration crate.
pub trait TradeReadPort: Send + Sync {
    fn read_accounts(
        &self,
        user_id: u64,
        trd_category: Option<i32>,
        need_general_sec_account: Option<bool>,
    ) -> Result<Vec<TradeAccountSnapshot>, TradeSessionError>;

    fn read_funds(
        &self,
        header: TradeHeader,
        refresh_cache: Option<bool>,
        currency: Option<i32>,
        asset_category: Option<i32>,
    ) -> Result<TradeFundsSnapshot, TradeSessionError>;

    fn read_cash_flows(
        &self,
        header: TradeHeader,
        clearing_date: String,
        cash_flow_direction: Option<i32>,
    ) -> Result<Vec<TradeCashFlowSnapshot>, TradeSessionError>;

    #[allow(clippy::too_many_arguments)]
    fn read_positions(
        &self,
        header: TradeHeader,
        filter: Option<TradeFilter>,
        filter_pl_ratio_min: Option<f64>,
        filter_pl_ratio_max: Option<f64>,
        refresh_cache: Option<bool>,
        asset_category: Option<i32>,
        currency: Option<i32>,
        option_strategy_view: Option<bool>,
    ) -> Result<Vec<TradePositionSnapshot>, TradeSessionError>;

    fn read_orders(
        &self,
        header: TradeHeader,
        filter: Option<TradeFilter>,
        filter_status_list: Vec<i32>,
        refresh_cache: Option<bool>,
    ) -> Result<Vec<TradeOrderSnapshot>, TradeSessionError>;

    fn read_fills(
        &self,
        header: TradeHeader,
        filter: Option<TradeFilter>,
        refresh_cache: Option<bool>,
    ) -> Result<Vec<TradeFillSnapshot>, TradeSessionError>;
}

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

    pub(crate) fn get_account_list(
        &self,
        request: trd_get_acc_list::Request,
    ) -> Result<trd_get_acc_list::S2c, TradeSessionError> {
        let body = self.call(
            trd_get_acc_list::PROTOCOL_ID,
            &trd_get_acc_list::encode_request(&request),
        )?;
        Ok(trd_get_acc_list::decode_response(&body)?)
    }

    pub(crate) fn get_funds(
        &self,
        request: trd_get_funds::Request,
    ) -> Result<trd_common::Funds, TradeSessionError> {
        let body = self.call(
            trd_get_funds::PROTOCOL_ID,
            &trd_get_funds::encode_request(&request),
        )?;
        Ok(trd_get_funds::decode_response(&body)?)
    }

    pub(crate) fn get_position_list(
        &self,
        request: trd_get_position_list::Request,
    ) -> Result<trd_get_position_list::S2c, TradeSessionError> {
        let body = self.call(
            trd_get_position_list::PROTOCOL_ID,
            &trd_get_position_list::encode_request(&request),
        )?;
        Ok(trd_get_position_list::decode_response(&body)?)
    }

    pub(crate) fn get_order_list(
        &self,
        request: trd_get_order_list::Request,
    ) -> Result<trd_get_order_list::S2c, TradeSessionError> {
        let body = self.call(
            trd_get_order_list::PROTOCOL_ID,
            &trd_get_order_list::encode_request(&request),
        )?;
        Ok(trd_get_order_list::decode_response(&body)?)
    }

    pub(crate) fn get_order_fill_list(
        &self,
        request: trd_get_order_fill_list::Request,
    ) -> Result<trd_get_order_fill_list::S2c, TradeSessionError> {
        let body = self.call(
            trd_get_order_fill_list::PROTOCOL_ID,
            &trd_get_order_fill_list::encode_request(&request),
        )?;
        Ok(trd_get_order_fill_list::decode_response(&body)?)
    }

    pub(crate) fn get_flow_summary(
        &self,
        request: trd_flow_summary::Request,
    ) -> Result<trd_flow_summary::S2c, TradeSessionError> {
        let body = self.call(
            trd_flow_summary::PROTOCOL_ID,
            &trd_flow_summary::encode_request(&request),
        )?;
        Ok(trd_flow_summary::decode_response(&body)?)
    }

    /// Reads account metadata without exposing the generated protobuf type.
    pub fn read_accounts(
        &self,
        user_id: u64,
        trd_category: Option<i32>,
        need_general_sec_account: Option<bool>,
    ) -> Result<Vec<TradeAccountSnapshot>, TradeSessionError> {
        let payload = self.get_account_list(trd_get_acc_list::Request {
            c2s: trd_get_acc_list::C2s {
                user_id,
                trd_category,
                need_general_sec_account,
            },
        })?;
        Ok(account_projection(payload))
    }

    /// Reads funds using a neutral header and returns an engine-neutral snapshot.
    pub fn read_funds(
        &self,
        header: TradeHeader,
        refresh_cache: Option<bool>,
        currency: Option<i32>,
        asset_category: Option<i32>,
    ) -> Result<TradeFundsSnapshot, TradeSessionError> {
        let proto_header: trd_common::TrdHeader = header.clone().into();
        let funds = self.get_funds(trd_get_funds::Request {
            c2s: trd_get_funds::C2s {
                header: proto_header,
                refresh_cache,
                currency,
                asset_category,
            },
        })?;
        Ok(funds_projection(header.into(), funds))
    }

    /// Reads account cash-flow summaries without exposing protobuf types.
    pub fn read_cash_flows(
        &self,
        header: TradeHeader,
        clearing_date: String,
        cash_flow_direction: Option<i32>,
    ) -> Result<Vec<TradeCashFlowSnapshot>, TradeSessionError> {
        let payload = self.get_flow_summary(trd_flow_summary::Request {
            c2s: trd_flow_summary::C2s {
                header: header.into(),
                clearing_date,
                cash_flow_direction,
                start_create_date: None,
                end_create_date: None,
            },
        })?;
        Ok(cash_flows_projection(payload))
    }

    /// Reads positions with optional neutral filtering and returns snapshots.
    #[allow(clippy::too_many_arguments)]
    pub fn read_positions(
        &self,
        header: TradeHeader,
        filter: Option<TradeFilter>,
        filter_pl_ratio_min: Option<f64>,
        filter_pl_ratio_max: Option<f64>,
        refresh_cache: Option<bool>,
        asset_category: Option<i32>,
        currency: Option<i32>,
        option_strategy_view: Option<bool>,
    ) -> Result<Vec<TradePositionSnapshot>, TradeSessionError> {
        let payload = self.get_position_list(trd_get_position_list::Request {
            c2s: trd_get_position_list::C2s {
                header: header.into(),
                filter_conditions: filter.map(Into::into),
                filter_pl_ratio_min,
                filter_pl_ratio_max,
                refresh_cache,
                asset_category,
                currency,
                option_strategy_view,
            },
        })?;
        Ok(positions_projection(payload))
    }

    /// Reads orders with optional neutral filtering and returns snapshots.
    pub fn read_orders(
        &self,
        header: TradeHeader,
        filter: Option<TradeFilter>,
        filter_status_list: Vec<i32>,
        refresh_cache: Option<bool>,
    ) -> Result<Vec<TradeOrderSnapshot>, TradeSessionError> {
        let payload = self.get_order_list(trd_get_order_list::Request {
            c2s: trd_get_order_list::C2s {
                header: header.into(),
                filter_conditions: filter.map(Into::into),
                filter_status_list,
                refresh_cache,
            },
        })?;
        Ok(orders_projection(payload))
    }

    /// Reads fills with optional neutral filtering and returns snapshots.
    pub fn read_fills(
        &self,
        header: TradeHeader,
        filter: Option<TradeFilter>,
        refresh_cache: Option<bool>,
    ) -> Result<Vec<TradeFillSnapshot>, TradeSessionError> {
        let payload = self.get_order_fill_list(trd_get_order_fill_list::Request {
            c2s: trd_get_order_fill_list::C2s {
                header: header.into(),
                filter_conditions: filter.map(Into::into),
                refresh_cache,
            },
        })?;
        Ok(fills_projection(payload))
    }

    fn call(&self, protocol: u32, request_body: &[u8]) -> Result<Vec<u8>, TradeSessionError> {
        Ok(self.session.call(protocol, request_body)?)
    }
}

/// Builds the common trade header used by funds, positions, orders and fills.
pub const fn trade_header(trd_env: i32, acc_id: u64, trd_market: i32) -> TradeHeader {
    TradeHeader {
        trd_env,
        acc_id,
        trd_market,
        jp_acc_type: None,
    }
}

impl TradeReadPort for OpenDTradeReadClient {
    fn read_accounts(
        &self,
        user_id: u64,
        trd_category: Option<i32>,
        need_general_sec_account: Option<bool>,
    ) -> Result<Vec<TradeAccountSnapshot>, TradeSessionError> {
        OpenDTradeReadClient::read_accounts(self, user_id, trd_category, need_general_sec_account)
    }

    fn read_funds(
        &self,
        header: TradeHeader,
        refresh_cache: Option<bool>,
        currency: Option<i32>,
        asset_category: Option<i32>,
    ) -> Result<TradeFundsSnapshot, TradeSessionError> {
        OpenDTradeReadClient::read_funds(self, header, refresh_cache, currency, asset_category)
    }

    fn read_cash_flows(
        &self,
        header: TradeHeader,
        clearing_date: String,
        cash_flow_direction: Option<i32>,
    ) -> Result<Vec<TradeCashFlowSnapshot>, TradeSessionError> {
        OpenDTradeReadClient::read_cash_flows(self, header, clearing_date, cash_flow_direction)
    }

    #[allow(clippy::too_many_arguments)]
    fn read_positions(
        &self,
        header: TradeHeader,
        filter: Option<TradeFilter>,
        filter_pl_ratio_min: Option<f64>,
        filter_pl_ratio_max: Option<f64>,
        refresh_cache: Option<bool>,
        asset_category: Option<i32>,
        currency: Option<i32>,
        option_strategy_view: Option<bool>,
    ) -> Result<Vec<TradePositionSnapshot>, TradeSessionError> {
        OpenDTradeReadClient::read_positions(
            self,
            header,
            filter,
            filter_pl_ratio_min,
            filter_pl_ratio_max,
            refresh_cache,
            asset_category,
            currency,
            option_strategy_view,
        )
    }

    fn read_orders(
        &self,
        header: TradeHeader,
        filter: Option<TradeFilter>,
        filter_status_list: Vec<i32>,
        refresh_cache: Option<bool>,
    ) -> Result<Vec<TradeOrderSnapshot>, TradeSessionError> {
        OpenDTradeReadClient::read_orders(self, header, filter, filter_status_list, refresh_cache)
    }

    fn read_fills(
        &self,
        header: TradeHeader,
        filter: Option<TradeFilter>,
        refresh_cache: Option<bool>,
    ) -> Result<Vec<TradeFillSnapshot>, TradeSessionError> {
        OpenDTradeReadClient::read_fills(self, header, filter, refresh_cache)
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
    fn cash_flow_read_encodes_header_and_projects_neutral_snapshot() {
        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
        let address = listener.local_addr().expect("address");
        let server = thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept");
            let request = read_frame(&mut stream);
            assert_eq!(request.header.proto_id, trd_flow_summary::PROTOCOL_ID);
            let decoded =
                trd_flow_summary::Request::decode(request.body.as_slice()).expect("request");
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
    fn read_accounts_projects_a_framed_response_without_exposing_proto_types() {
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
}
