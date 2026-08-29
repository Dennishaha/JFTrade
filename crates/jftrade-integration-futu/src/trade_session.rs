use std::sync::Arc;

use thiserror::Error;

use crate::health::OpenDInitializedSession;
use crate::managed_session::{OpenDManagedSession, OpenDManagedSessionError};
use crate::session_coordinator::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};
use crate::trade_proto::{
    ResponseError, trd_common, trd_flow_summary, trd_get_acc_list, trd_get_funds,
    trd_get_margin_ratio, trd_get_max_trd_qtys, trd_get_order_fee, trd_get_order_fill_list,
    trd_get_order_list, trd_get_position_list,
};
use crate::trade_snapshots::{
    TradeAccountSnapshot, TradeCashFlowSnapshot, TradeFillSnapshot, TradeFilter,
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

#[cfg(test)]
#[path = "trade_session_tests.rs"]
mod tests;
