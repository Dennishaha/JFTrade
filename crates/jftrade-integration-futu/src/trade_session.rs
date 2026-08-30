use std::sync::{
    Arc,
    atomic::{AtomicU32, Ordering},
};

use thiserror::Error;

use crate::health::OpenDInitializedSession;
use crate::managed_session::{OpenDManagedSession, OpenDManagedSessionError};
use crate::session_coordinator::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};
use crate::trade_proto::{
    ResponseError, common::PacketId, trd_common, trd_flow_summary, trd_get_acc_list, trd_get_funds,
    trd_get_margin_ratio, trd_get_max_trd_qtys, trd_get_order_fee, trd_get_order_fill_list,
    trd_get_order_list, trd_get_position_list, trd_modify_order, trd_place_combo_order,
    trd_place_order, trd_sub_acc_push, trd_unlock_trade,
};
use crate::trade_snapshots::{
    TradeAccountSnapshot, TradeCashFlowSnapshot, TradeComboLeg, TradeFillSnapshot, TradeFilter,
    TradeFundsSnapshot, TradeHeader, TradeMarginRatioSnapshot, TradeMaxTradeQuantityRequest,
    TradeMaxTradeQuantitySnapshot, TradeOrderFeeSnapshot, TradeOrderSnapshot,
    TradePositionSnapshot, TradeSecurity, account_projection, cash_flows_projection,
    fills_projection, funds_projection, margin_ratios_projection, max_trade_quantity_projection,
    order_fees_projection, orders_projection, positions_projection,
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

    fn read_order_fees(
        &self,
        header: TradeHeader,
        order_id_ex_list: Vec<String>,
    ) -> Result<Vec<TradeOrderFeeSnapshot>, TradeSessionError>;

    fn read_margin_ratios(
        &self,
        header: TradeHeader,
        securities: Vec<TradeSecurity>,
    ) -> Result<Vec<TradeMarginRatioSnapshot>, TradeSessionError>;

    fn read_max_trade_quantity(
        &self,
        request: TradeMaxTradeQuantityRequest,
    ) -> Result<TradeMaxTradeQuantitySnapshot, TradeSessionError>;

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

    fn read_history_orders(
        &self,
        header: TradeHeader,
        filter: Option<TradeFilter>,
        filter_status_list: Vec<i32>,
        refresh_cache: Option<bool>,
    ) -> Result<Vec<TradeOrderSnapshot>, TradeSessionError> {
        self.read_orders(header, filter, filter_status_list, refresh_cache)
    }

    fn read_fills(
        &self,
        header: TradeHeader,
        filter: Option<TradeFilter>,
        refresh_cache: Option<bool>,
    ) -> Result<Vec<TradeFillSnapshot>, TradeSessionError>;

    fn read_history_fills(
        &self,
        header: TradeHeader,
        filter: Option<TradeFilter>,
        refresh_cache: Option<bool>,
    ) -> Result<Vec<TradeFillSnapshot>, TradeSessionError> {
        self.read_fills(header, filter, refresh_cache)
    }
}

/// Neutral command DTOs used by the Rust production execution adapter.  They
/// intentionally contain no generated protobuf or transport types so engine
/// code cannot accidentally depend on OpenD wire details.
#[derive(Clone, Debug, PartialEq)]
pub struct TradePlaceOrderRequest {
    pub header: TradeHeader,
    pub trd_side: i32,
    pub order_type: i32,
    pub code: String,
    pub quantity: f64,
    pub price: Option<f64>,
    pub remark: Option<String>,
    pub time_in_force: Option<i32>,
    pub fill_outside_rth: Option<bool>,
    pub aux_price: Option<f64>,
    pub trail_type: Option<i32>,
    pub trail_value: Option<f64>,
    pub trail_spread: Option<f64>,
    pub session: Option<i32>,
    pub position_id: Option<u64>,
    pub expire_time: Option<String>,
    pub amount: Option<f64>,
    pub prediction_side: Option<i32>,
    pub sec_market: Option<i32>,
}

#[derive(Clone, Debug, PartialEq)]
pub struct TradePlaceOrderResult {
    pub header: TradeHeader,
    pub order_id: Option<u64>,
    pub order_id_ex: Option<String>,
}

#[derive(Clone, Debug, PartialEq)]
pub struct TradePlaceComboOrderRequest {
    pub header: TradeHeader,
    pub combo_legs: Vec<TradeComboLeg>,
    pub quantity: f64,
    pub price: Option<f64>,
    pub order_type: i32,
    pub time_in_force: Option<i32>,
    pub expire_time: Option<String>,
    pub remark: Option<String>,
    pub quote_id: Option<String>,
}

#[derive(Clone, Debug, PartialEq)]
pub struct TradePlaceComboOrderResult {
    pub header: TradeHeader,
    pub order_id_ex: Option<String>,
}

#[derive(Clone, Debug, PartialEq)]
pub struct TradeModifyOrderRequest {
    pub header: TradeHeader,
    pub order_id: u64,
    pub operation: i32,
    pub for_all: Option<bool>,
    pub trd_market: Option<i32>,
    pub quantity: Option<f64>,
    pub price: Option<f64>,
    pub adjust_price: Option<bool>,
    pub adjust_side_and_limit: Option<f64>,
    pub aux_price: Option<f64>,
    pub trail_type: Option<i32>,
    pub trail_value: Option<f64>,
    pub trail_spread: Option<f64>,
    pub order_id_ex: Option<String>,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct TradeUnlockRequest {
    pub unlock: bool,
    pub password_md5: Option<String>,
    pub security_firm: Option<i32>,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct TradeSubscribeAccountsRequest {
    pub account_ids: Vec<u64>,
}

/// Command-side OpenD contract. Implementations own framing, anti-replay
/// packet ids, response validation and connection lifecycle.
pub trait TradeWritePort: Send + Sync + std::fmt::Debug {
    fn place_order(
        &self,
        request: TradePlaceOrderRequest,
    ) -> Result<TradePlaceOrderResult, TradeSessionError>;
    fn place_combo_order(
        &self,
        request: TradePlaceComboOrderRequest,
    ) -> Result<TradePlaceComboOrderResult, TradeSessionError>;
    fn modify_order(
        &self,
        request: TradeModifyOrderRequest,
    ) -> Result<TradePlaceOrderResult, TradeSessionError>;
    fn unlock_trade(&self, request: TradeUnlockRequest) -> Result<(), TradeSessionError>;
    fn subscribe_trade_accounts(
        &self,
        request: TradeSubscribeAccountsRequest,
    ) -> Result<(), TradeSessionError>;
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
    conn_id: u64,
    command_serial: Arc<AtomicU32>,
}

impl std::fmt::Debug for OpenDTradeReadClient {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDTradeReadClient")
            .field("conn_id", &self.conn_id)
            .finish_non_exhaustive()
    }
}

impl OpenDTradeReadClient {
    pub fn from_session(session: OpenDInitializedSession) -> Self {
        let conn_id = session.conn_id();
        Self {
            session: session.managed_session_handle(),
            conn_id,
            command_serial: Arc::new(AtomicU32::new(0)),
        }
    }

    pub fn from_coordinator(
        coordinator: &OpenDSessionCoordinator,
    ) -> Result<Self, TradeSessionError> {
        Ok(Self::from_session(coordinator.session()?.clone()))
    }

    #[cfg(test)]
    fn from_managed_session(session: Arc<OpenDManagedSession>) -> Self {
        Self {
            session,
            conn_id: 0,
            command_serial: Arc::new(AtomicU32::new(0)),
        }
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

    pub(crate) fn get_history_order_list(
        &self,
        request: trd_get_order_list::Request,
    ) -> Result<trd_get_order_list::S2c, TradeSessionError> {
        let body = self.call(
            crate::trading::TradeProtocol::GetHistoryOrderList.id(),
            &trd_get_order_list::encode_request(&request),
        )?;
        Ok(trd_get_order_list::decode_response(&body)?)
    }

    pub(crate) fn get_history_order_fill_list(
        &self,
        request: trd_get_order_fill_list::Request,
    ) -> Result<trd_get_order_fill_list::S2c, TradeSessionError> {
        let body = self.call(
            crate::trading::TradeProtocol::GetHistoryOrderFillList.id(),
            &trd_get_order_fill_list::encode_request(&request),
        )?;
        Ok(trd_get_order_fill_list::decode_response(&body)?)
    }

    pub(crate) fn get_order_fee(
        &self,
        request: trd_get_order_fee::Request,
    ) -> Result<trd_get_order_fee::S2c, TradeSessionError> {
        let body = self.call(
            trd_get_order_fee::PROTOCOL_ID,
            &trd_get_order_fee::encode_request(&request),
        )?;
        Ok(trd_get_order_fee::decode_response(&body)?)
    }

    pub(crate) fn get_margin_ratio(
        &self,
        request: trd_get_margin_ratio::Request,
    ) -> Result<trd_get_margin_ratio::S2c, TradeSessionError> {
        let body = self.call(
            trd_get_margin_ratio::PROTOCOL_ID,
            &trd_get_margin_ratio::encode_request(&request),
        )?;
        Ok(trd_get_margin_ratio::decode_response(&body)?)
    }

    pub(crate) fn get_max_trade_qtys(
        &self,
        request: trd_get_max_trd_qtys::Request,
    ) -> Result<trd_common::MaxTrdQtys, TradeSessionError> {
        let body = self.call(
            trd_get_max_trd_qtys::PROTOCOL_ID,
            &trd_get_max_trd_qtys::encode_request(&request),
        )?;
        let payload = trd_get_max_trd_qtys::decode_response(&body)?;
        payload
            .max_trd_qtys
            .ok_or(crate::trade_proto::ResponseError::MissingMaxTradeQuantity.into())
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

    pub fn read_history_orders(
        &self,
        header: TradeHeader,
        filter: Option<TradeFilter>,
        filter_status_list: Vec<i32>,
        refresh_cache: Option<bool>,
    ) -> Result<Vec<TradeOrderSnapshot>, TradeSessionError> {
        let payload = self.get_history_order_list(trd_get_order_list::Request {
            c2s: trd_get_order_list::C2s {
                header: header.into(),
                filter_conditions: filter.map(Into::into),
                filter_status_list,
                refresh_cache,
            },
        })?;
        Ok(orders_projection(payload))
    }

    pub fn read_history_fills(
        &self,
        header: TradeHeader,
        filter: Option<TradeFilter>,
        refresh_cache: Option<bool>,
    ) -> Result<Vec<TradeFillSnapshot>, TradeSessionError> {
        let payload = self.get_history_order_fill_list(trd_get_order_fill_list::Request {
            c2s: trd_get_order_fill_list::C2s {
                header: header.into(),
                filter_conditions: filter.map(Into::into),
                refresh_cache,
            },
        })?;
        Ok(fills_projection(payload))
    }

    pub fn read_order_fees(
        &self,
        header: TradeHeader,
        order_id_ex_list: Vec<String>,
    ) -> Result<Vec<TradeOrderFeeSnapshot>, TradeSessionError> {
        let payload = self.get_order_fee(trd_get_order_fee::Request {
            c2s: trd_get_order_fee::C2s {
                header: header.into(),
                order_id_ex_list,
            },
        })?;
        Ok(order_fees_projection(payload))
    }

    pub fn read_margin_ratios(
        &self,
        header: TradeHeader,
        securities: Vec<TradeSecurity>,
    ) -> Result<Vec<TradeMarginRatioSnapshot>, TradeSessionError> {
        let mut remaining = securities;
        loop {
            let payload = self.get_margin_ratio(trd_get_margin_ratio::Request {
                c2s: trd_get_margin_ratio::C2s {
                    header: header.clone().into(),
                    security_list: remaining
                        .iter()
                        .map(|security| crate::trade_proto::qot_common::Security {
                            market: security.market,
                            code: security.code.clone(),
                        })
                        .collect(),
                },
            });
            match payload {
                Ok(payload) => return Ok(margin_ratios_projection(payload)),
                Err(error) => {
                    let Some(unknown_code) = unknown_security_code(&error) else {
                        return Err(error);
                    };
                    let before = remaining.len();
                    remaining.retain(|security| !security.code.eq_ignore_ascii_case(&unknown_code));
                    if remaining.len() == before {
                        return Err(error);
                    }
                    if remaining.is_empty() {
                        return Ok(Vec::new());
                    }
                }
            }
        }
    }

    pub fn read_max_trade_quantity(
        &self,
        request: TradeMaxTradeQuantityRequest,
    ) -> Result<TradeMaxTradeQuantitySnapshot, TradeSessionError> {
        let payload = self.get_max_trade_qtys(trd_get_max_trd_qtys::Request {
            c2s: trd_get_max_trd_qtys::C2s {
                header: request.header.clone().into(),
                order_type: request.order_type,
                code: request.code.clone(),
                price: request.price,
                order_id: request.order_id,
                adjust_price: request.adjust_price,
                adjust_side_and_limit: request.adjust_side_and_limit,
                sec_market: request.sec_market,
                order_id_ex: request.order_id_ex.clone(),
                session: request.session,
                position_id: request.position_id,
            },
        })?;
        Ok(max_trade_quantity_projection(&request, payload))
    }

    fn call(&self, protocol: u32, request_body: &[u8]) -> Result<Vec<u8>, TradeSessionError> {
        Ok(self.session.call(protocol, request_body)?)
    }

    fn next_command_packet_id(&self) -> PacketId {
        let serial_no = self
            .command_serial
            .fetch_update(Ordering::Relaxed, Ordering::Relaxed, |value| {
                Some(value.wrapping_add(1).max(1))
            })
            .unwrap_or(0)
            .wrapping_add(1)
            .max(1);
        PacketId {
            conn_id: self.conn_id,
            serial_no,
        }
    }

    fn place_order_command(
        &self,
        request: TradePlaceOrderRequest,
    ) -> Result<TradePlaceOrderResult, TradeSessionError> {
        let body = self.call(
            trd_place_order::PROTOCOL_ID,
            &trd_place_order::encode_request(&trd_place_order::Request {
                c2s: trd_place_order::C2s {
                    packet_id: self.next_command_packet_id(),
                    header: request.header.into(),
                    trd_side: request.trd_side,
                    order_type: request.order_type,
                    code: request.code,
                    qty: request.quantity,
                    price: request.price,
                    adjust_price: None,
                    adjust_side_and_limit: None,
                    sec_market: request.sec_market,
                    remark: request.remark,
                    time_in_force: request.time_in_force,
                    fill_outside_rth: request.fill_outside_rth,
                    aux_price: request.aux_price,
                    trail_type: request.trail_type,
                    trail_value: request.trail_value,
                    trail_spread: request.trail_spread,
                    session: request.session,
                    position_id: request.position_id,
                    expire_time: request.expire_time,
                    amount: request.amount,
                    pred_side: request.prediction_side,
                },
            }),
        )?;
        let payload = trd_place_order::decode_response(&body)?;
        Ok(TradePlaceOrderResult {
            header: payload.header.into(),
            order_id: payload.order_id,
            order_id_ex: payload.order_id_ex,
        })
    }
}

fn unknown_security_code(error: &TradeSessionError) -> Option<String> {
    let message = error.to_string();
    let lower = message.to_ascii_lowercase();
    for marker in ["unknown stock", "unknown security", "未知股票"] {
        let Some(index) = lower.find(&marker.to_ascii_lowercase()) else {
            continue;
        };
        let tail = message[index + marker.len()..]
            .trim_matches(|c: char| c.is_whitespace() || c == ':' || c == '"');
        let code = tail
            .split_whitespace()
            .next()?
            .trim_matches(|c: char| c == ',' || c == ';' || c == '"');
        if !code.is_empty() {
            return Some(code.to_owned());
        }
    }
    None
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

    fn read_order_fees(
        &self,
        header: TradeHeader,
        order_id_ex_list: Vec<String>,
    ) -> Result<Vec<TradeOrderFeeSnapshot>, TradeSessionError> {
        OpenDTradeReadClient::read_order_fees(self, header, order_id_ex_list)
    }

    fn read_margin_ratios(
        &self,
        header: TradeHeader,
        securities: Vec<TradeSecurity>,
    ) -> Result<Vec<TradeMarginRatioSnapshot>, TradeSessionError> {
        OpenDTradeReadClient::read_margin_ratios(self, header, securities)
    }

    fn read_max_trade_quantity(
        &self,
        request: TradeMaxTradeQuantityRequest,
    ) -> Result<TradeMaxTradeQuantitySnapshot, TradeSessionError> {
        OpenDTradeReadClient::read_max_trade_quantity(self, request)
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

    fn read_history_orders(
        &self,
        header: TradeHeader,
        filter: Option<TradeFilter>,
        filter_status_list: Vec<i32>,
        refresh_cache: Option<bool>,
    ) -> Result<Vec<TradeOrderSnapshot>, TradeSessionError> {
        OpenDTradeReadClient::read_history_orders(
            self,
            header,
            filter,
            filter_status_list,
            refresh_cache,
        )
    }

    fn read_history_fills(
        &self,
        header: TradeHeader,
        filter: Option<TradeFilter>,
        refresh_cache: Option<bool>,
    ) -> Result<Vec<TradeFillSnapshot>, TradeSessionError> {
        OpenDTradeReadClient::read_history_fills(self, header, filter, refresh_cache)
    }
}

impl TradeWritePort for OpenDTradeReadClient {
    fn place_order(
        &self,
        request: TradePlaceOrderRequest,
    ) -> Result<TradePlaceOrderResult, TradeSessionError> {
        self.place_order_command(request)
    }

    fn place_combo_order(
        &self,
        request: TradePlaceComboOrderRequest,
    ) -> Result<TradePlaceComboOrderResult, TradeSessionError> {
        let body = self.call(
            trd_place_combo_order::PROTOCOL_ID,
            &trd_place_combo_order::encode_request(&trd_place_combo_order::Request {
                c2s: trd_place_combo_order::C2s {
                    packet_id: self.next_command_packet_id(),
                    header: request.header.into(),
                    combo_legs: request
                        .combo_legs
                        .into_iter()
                        .map(|leg| crate::trade_proto::qot_common::ComboLeg {
                            security: crate::trade_proto::qot_common::Security {
                                market: leg.market,
                                code: leg.code,
                            },
                            side: leg.side,
                            qty_ratio: leg.qty_ratio,
                            position_id: leg.position_id,
                            pred_side: leg.pred_side,
                        })
                        .collect(),
                    qty: request.quantity,
                    price: request.price,
                    order_type: request.order_type,
                    time_in_force: request.time_in_force,
                    expire_time: request.expire_time,
                    remark: request.remark,
                    quote_id: request.quote_id,
                },
            }),
        )?;
        let payload = trd_place_combo_order::decode_response(&body)?;
        Ok(TradePlaceComboOrderResult {
            header: payload.header.into(),
            order_id_ex: payload.order_id_ex,
        })
    }

    fn modify_order(
        &self,
        request: TradeModifyOrderRequest,
    ) -> Result<TradePlaceOrderResult, TradeSessionError> {
        let body = self.call(
            trd_modify_order::PROTOCOL_ID,
            &trd_modify_order::encode_request(&trd_modify_order::Request {
                c2s: trd_modify_order::C2s {
                    packet_id: self.next_command_packet_id(),
                    header: request.header.into(),
                    order_id: request.order_id,
                    modify_order_op: request.operation,
                    for_all: request.for_all,
                    trd_market: request.trd_market,
                    qty: request.quantity,
                    price: request.price,
                    adjust_price: request.adjust_price,
                    adjust_side_and_limit: request.adjust_side_and_limit,
                    aux_price: request.aux_price,
                    trail_type: request.trail_type,
                    trail_value: request.trail_value,
                    trail_spread: request.trail_spread,
                    order_id_ex: request.order_id_ex,
                },
            }),
        )?;
        let payload = trd_modify_order::decode_response(&body)?;
        Ok(TradePlaceOrderResult {
            header: payload.header.into(),
            order_id: Some(payload.order_id),
            order_id_ex: payload.order_id_ex,
        })
    }

    fn unlock_trade(&self, request: TradeUnlockRequest) -> Result<(), TradeSessionError> {
        let body = self.call(
            trd_unlock_trade::PROTOCOL_ID,
            &trd_unlock_trade::encode_request(&trd_unlock_trade::Request {
                c2s: trd_unlock_trade::C2s {
                    unlock: request.unlock,
                    pwd_md5: request.password_md5,
                    security_firm: request.security_firm,
                },
            }),
        )?;
        trd_unlock_trade::decode_response(&body)?;
        Ok(())
    }

    fn subscribe_trade_accounts(
        &self,
        request: TradeSubscribeAccountsRequest,
    ) -> Result<(), TradeSessionError> {
        let body = self.call(
            trd_sub_acc_push::PROTOCOL_ID,
            &trd_sub_acc_push::encode_request(&trd_sub_acc_push::Request {
                c2s: trd_sub_acc_push::C2s {
                    acc_id_list: request.account_ids,
                },
            }),
        )?;
        trd_sub_acc_push::decode_response(&body)?;
        Ok(())
    }
}

#[cfg(test)]
#[path = "trade_session_tests.rs"]
mod tests;
