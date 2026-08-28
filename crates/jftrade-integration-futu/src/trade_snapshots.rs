//! Engine-neutral read projections for the Futu trade protocols.
//!
//! The generated protobuf messages intentionally remain behind the integration
//! boundary.  Consumers of this crate use these stable DTOs instead of taking a
//! dependency on OpenD's wire types.

use serde::{Deserialize, Serialize};
use std::cmp::Reverse;

use crate::trade_proto::{self, trd_common};

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
pub struct TradeHeader {
    pub trd_env: i32,
    pub acc_id: u64,
    pub trd_market: i32,
    pub jp_acc_type: Option<i32>,
}

impl From<trd_common::TrdHeader> for TradeHeader {
    fn from(value: trd_common::TrdHeader) -> Self {
        Self {
            trd_env: value.trd_env,
            acc_id: value.acc_id,
            trd_market: value.trd_market,
            jp_acc_type: value.jp_acc_type,
        }
    }
}

impl From<TradeHeader> for trd_common::TrdHeader {
    fn from(value: TradeHeader) -> Self {
        Self {
            trd_env: value.trd_env,
            acc_id: value.acc_id,
            trd_market: value.trd_market,
            jp_acc_type: value.jp_acc_type,
        }
    }
}

#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
pub struct TradeFilter {
    pub code_list: Vec<String>,
    pub id_list: Vec<u64>,
    pub begin_time: Option<String>,
    pub end_time: Option<String>,
    pub order_id_ex_list: Vec<String>,
    pub filter_market: Option<i32>,
}

impl From<TradeFilter> for trd_common::TrdFilterConditions {
    fn from(value: TradeFilter) -> Self {
        Self {
            code_list: value.code_list,
            id_list: value.id_list,
            begin_time: value.begin_time,
            end_time: value.end_time,
            order_id_ex_list: value.order_id_ex_list,
            filter_market: value.filter_market,
        }
    }
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
pub struct TradeAccountSnapshot {
    pub trd_env: i32,
    pub acc_id: u64,
    pub trd_market_auth_list: Vec<i32>,
    pub acc_type: Option<i32>,
    pub card_num: Option<String>,
    pub security_firm: Option<i32>,
    pub sim_acc_type: Option<i32>,
    pub uni_card_num: Option<String>,
    pub acc_status: Option<i32>,
    pub acc_role: Option<i32>,
    pub jp_acc_type: Vec<i32>,
    pub competition_acc_name: Option<String>,
}

impl From<trd_common::TrdAcc> for TradeAccountSnapshot {
    fn from(value: trd_common::TrdAcc) -> Self {
        Self {
            trd_env: value.trd_env,
            acc_id: value.acc_id,
            trd_market_auth_list: value.trd_market_auth_list,
            acc_type: value.acc_type,
            card_num: value.card_num,
            security_firm: value.security_firm,
            sim_acc_type: value.sim_acc_type,
            uni_card_num: value.uni_card_num,
            acc_status: value.acc_status,
            acc_role: value.acc_role,
            jp_acc_type: value.jp_acc_type,
            competition_acc_name: value.competition_acc_name,
        }
    }
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
pub struct TradeFundsSnapshot {
    pub header: TradeHeader,
    pub funds: TradeFunds,
}

/// A single account cash-flow entry returned by Trd_FlowSummary.
#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
pub struct TradeCashFlowSnapshot {
    pub header: TradeHeader,
    pub clearing_date: Option<String>,
    pub settlement_date: Option<String>,
    pub currency: Option<i32>,
    pub cash_flow_type: Option<String>,
    pub cash_flow_direction: Option<i32>,
    pub cash_flow_amount: Option<f64>,
    pub cash_flow_remark: Option<String>,
    pub cash_flow_id: Option<u64>,
    pub create_time: Option<String>,
}

impl TradeCashFlowSnapshot {
    pub(crate) fn from_proto(
        header: TradeHeader,
        value: trade_proto::trd_flow_summary::FlowSummaryInfo,
    ) -> Self {
        Self {
            header,
            clearing_date: optional_text(value.clearing_date),
            settlement_date: optional_text(value.settlement_date),
            currency: value.currency,
            cash_flow_type: optional_text(value.cash_flow_type),
            cash_flow_direction: value
                .cash_flow_direction
                .filter(|direction| matches!(direction, 1 | 2)),
            cash_flow_amount: value.cash_flow_amount,
            cash_flow_remark: optional_text(value.cash_flow_remark),
            cash_flow_id: value.cash_flow_id,
            create_time: value.create_time,
        }
    }
}

fn optional_text(value: Option<String>) -> Option<String> {
    value.and_then(|value| {
        let value = value.trim().to_owned();
        (!value.is_empty()).then_some(value)
    })
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
pub struct TradeFunds {
    pub power: f64,
    pub total_assets: f64,
    pub cash: f64,
    pub market_val: f64,
    pub frozen_cash: f64,
    pub debt_cash: f64,
    pub avl_withdrawal_cash: f64,
    pub currency: Option<i32>,
    pub available_funds: Option<f64>,
    pub unrealized_pl: Option<f64>,
    pub realized_pl: Option<f64>,
    pub risk_level: Option<i32>,
    pub initial_margin: Option<f64>,
    pub maintenance_margin: Option<f64>,
    pub cash_info_list: Vec<TradeCashInfo>,
    pub max_power_short: Option<f64>,
    pub net_cash_power: Option<f64>,
    pub long_mv: Option<f64>,
    pub short_mv: Option<f64>,
    pub pending_asset: Option<f64>,
    pub max_withdrawal: Option<f64>,
    pub risk_status: Option<i32>,
    pub margin_call_margin: Option<f64>,
    pub is_pdt: Option<bool>,
    pub pdt_seq: Option<String>,
    pub beginning_dtbp: Option<f64>,
    pub remaining_dtbp: Option<f64>,
    pub dt_call_amount: Option<f64>,
    pub dt_status: Option<i32>,
    pub securities_assets: Option<f64>,
    pub fund_assets: Option<f64>,
    pub bond_assets: Option<f64>,
    pub market_info_list: Vec<TradeMarketInfo>,
    pub crypto_mv: Option<f64>,
    pub exposure_level: Option<i32>,
    pub exposure_limit: Option<f64>,
    pub used_limit: Option<f64>,
    pub remaining_limit: Option<f64>,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
pub struct TradeCashInfo {
    pub currency: Option<i32>,
    pub cash: Option<f64>,
    pub available_balance: Option<f64>,
    pub net_cash_power: Option<f64>,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
pub struct TradeMarketInfo {
    pub trd_market: Option<i32>,
    pub assets: Option<f64>,
}

impl From<trd_common::Funds> for TradeFunds {
    fn from(value: trd_common::Funds) -> Self {
        Self {
            power: value.power,
            total_assets: value.total_assets,
            cash: value.cash,
            market_val: value.market_val,
            frozen_cash: value.frozen_cash,
            debt_cash: value.debt_cash,
            avl_withdrawal_cash: value.avl_withdrawal_cash,
            currency: value.currency,
            available_funds: value.available_funds,
            unrealized_pl: value.unrealized_pl,
            realized_pl: value.realized_pl,
            risk_level: value.risk_level,
            initial_margin: value.initial_margin,
            maintenance_margin: value.maintenance_margin,
            cash_info_list: value
                .cash_info_list
                .into_iter()
                .map(|v| TradeCashInfo {
                    currency: v.currency,
                    cash: v.cash,
                    available_balance: v.available_balance,
                    net_cash_power: v.net_cash_power,
                })
                .collect(),
            max_power_short: value.max_power_short,
            net_cash_power: value.net_cash_power,
            long_mv: value.long_mv,
            short_mv: value.short_mv,
            pending_asset: value.pending_asset,
            max_withdrawal: value.max_withdrawal,
            risk_status: value.risk_status,
            margin_call_margin: value.margin_call_margin,
            is_pdt: value.is_pdt,
            pdt_seq: value.pdt_seq,
            beginning_dtbp: value.beginning_dtbp,
            remaining_dtbp: value.remaining_dtbp,
            dt_call_amount: value.dt_call_amount,
            dt_status: value.dt_status,
            securities_assets: value.securities_assets,
            fund_assets: value.fund_assets,
            bond_assets: value.bond_assets,
            market_info_list: value
                .market_info_list
                .into_iter()
                .map(|v| TradeMarketInfo {
                    trd_market: v.trd_market,
                    assets: v.assets,
                })
                .collect(),
            crypto_mv: value.crypto_mv,
            exposure_level: value.exposure_level,
            exposure_limit: value.exposure_limit,
            used_limit: value.used_limit,
            remaining_limit: value.remaining_limit,
        }
    }
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
pub struct TradePositionSnapshot {
    pub position_id: u64,
    pub position_side: i32,
    pub code: String,
    pub name: String,
    pub qty: f64,
    pub can_sell_qty: f64,
    pub price: f64,
    pub cost_price: Option<f64>,
    pub val: f64,
    pub pl_val: f64,
    pub pl_ratio: Option<f64>,
    pub sec_market: Option<i32>,
    pub trd_market: Option<i32>,
    pub diluted_cost_price: Option<f64>,
    pub average_cost_price: Option<f64>,
    pub average_pl_ratio: Option<f64>,
    pub td_pl_val: Option<f64>,
    pub td_trd_val: Option<f64>,
    pub td_buy_val: Option<f64>,
    pub td_buy_qty: Option<f64>,
    pub td_sell_val: Option<f64>,
    pub td_sell_qty: Option<f64>,
    pub unrealized_pl: Option<f64>,
    pub realized_pl: Option<f64>,
    pub currency: Option<i32>,
    pub acc_id: Option<u64>,
    pub combo_id: Option<u64>,
    pub strategy_type: Option<i32>,
    pub position_type: Option<i32>,
    pub jp_acc_type: Option<i32>,
    pub payout_if_win: Option<f64>,
}

impl From<trd_common::Position> for TradePositionSnapshot {
    fn from(value: trd_common::Position) -> Self {
        Self {
            position_id: value.position_id,
            position_side: value.position_side,
            code: value.code,
            name: value.name,
            qty: value.qty,
            can_sell_qty: value.can_sell_qty,
            price: value.price,
            cost_price: value.cost_price,
            val: value.val,
            pl_val: value.pl_val,
            pl_ratio: value.pl_ratio,
            sec_market: value.sec_market,
            trd_market: value.trd_market,
            diluted_cost_price: value.diluted_cost_price,
            average_cost_price: value.average_cost_price,
            average_pl_ratio: value.average_pl_ratio,
            td_pl_val: value.td_pl_val,
            td_trd_val: value.td_trd_val,
            td_buy_val: value.td_buy_val,
            td_buy_qty: value.td_buy_qty,
            td_sell_val: value.td_sell_val,
            td_sell_qty: value.td_sell_qty,
            unrealized_pl: value.unrealized_pl,
            realized_pl: value.realized_pl,
            currency: value.currency,
            acc_id: value.acc_id,
            combo_id: value.combo_id,
            strategy_type: value.strategy_type,
            position_type: value.position_type,
            jp_acc_type: value.jp_acc_type,
            payout_if_win: value.payout_if_win,
        }
    }
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
pub struct TradeComboLeg {
    pub market: i32,
    pub code: String,
    pub side: Option<i32>,
    pub qty_ratio: Option<f64>,
    pub position_id: Option<u64>,
    pub pred_side: Option<i32>,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
pub struct TradeOrderSnapshot {
    pub trd_side: i32,
    pub order_type: i32,
    pub order_status: i32,
    pub order_id: u64,
    pub order_id_ex: String,
    pub code: String,
    pub name: String,
    pub qty: f64,
    pub price: Option<f64>,
    pub create_time: String,
    pub update_time: String,
    pub fill_qty: Option<f64>,
    pub fill_avg_price: Option<f64>,
    pub last_err_msg: Option<String>,
    pub sec_market: Option<i32>,
    pub create_timestamp: Option<f64>,
    pub update_timestamp: Option<f64>,
    pub remark: Option<String>,
    pub trd_market: Option<i32>,
    pub expire_time: Option<String>,
    pub order_amount: Option<f64>,
    pub time_in_force: Option<i32>,
    pub fill_outside_rth: Option<bool>,
    pub aux_price: Option<f64>,
    pub trail_type: Option<i32>,
    pub trail_value: Option<f64>,
    pub trail_spread: Option<f64>,
    pub currency: Option<i32>,
    pub session: Option<i32>,
    pub jp_acc_type: Option<i32>,
    pub strategy_type: Option<i32>,
    pub combo_legs: Vec<TradeComboLeg>,
}

impl From<trd_common::Order> for TradeOrderSnapshot {
    fn from(value: trd_common::Order) -> Self {
        Self {
            trd_side: value.trd_side,
            order_type: value.order_type,
            order_status: value.order_status,
            order_id: value.order_id,
            order_id_ex: value.order_id_ex,
            code: value.code,
            name: value.name,
            qty: value.qty,
            price: value.price,
            create_time: value.create_time,
            update_time: value.update_time,
            fill_qty: value.fill_qty,
            fill_avg_price: value.fill_avg_price,
            last_err_msg: value.last_err_msg,
            sec_market: value.sec_market,
            create_timestamp: value.create_timestamp,
            update_timestamp: value.update_timestamp,
            remark: value.remark,
            trd_market: value.trd_market,
            expire_time: value.expire_time,
            order_amount: value.order_amount,
            time_in_force: value.time_in_force,
            fill_outside_rth: value.fill_outside_rth,
            aux_price: value.aux_price,
            trail_type: value.trail_type,
            trail_value: value.trail_value,
            trail_spread: value.trail_spread,
            currency: value.currency,
            session: value.session,
            jp_acc_type: value.jp_acc_type,
            strategy_type: value.strategy_type,
            combo_legs: value
                .combo_legs
                .into_iter()
                .map(|leg| TradeComboLeg {
                    market: leg.security.market,
                    code: leg.security.code,
                    side: leg.side,
                    qty_ratio: leg.qty_ratio,
                    position_id: leg.position_id,
                    pred_side: leg.pred_side,
                })
                .collect(),
        }
    }
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
pub struct TradeFillSnapshot {
    pub trd_side: i32,
    pub fill_id: u64,
    pub fill_id_ex: String,
    pub order_id: Option<u64>,
    pub order_id_ex: Option<String>,
    pub code: String,
    pub name: String,
    pub qty: f64,
    pub price: f64,
    pub create_time: String,
    pub counter_broker_id: Option<i32>,
    pub counter_broker_name: Option<String>,
    pub sec_market: Option<i32>,
    pub create_timestamp: Option<f64>,
    pub update_timestamp: Option<f64>,
    pub status: Option<i32>,
    pub trd_market: Option<i32>,
    pub jp_acc_type: Option<i32>,
}

impl From<trd_common::OrderFill> for TradeFillSnapshot {
    fn from(value: trd_common::OrderFill) -> Self {
        Self {
            trd_side: value.trd_side,
            fill_id: value.fill_id,
            fill_id_ex: value.fill_id_ex,
            order_id: value.order_id,
            order_id_ex: value.order_id_ex,
            code: value.code,
            name: value.name,
            qty: value.qty,
            price: value.price,
            create_time: value.create_time,
            counter_broker_id: value.counter_broker_id,
            counter_broker_name: value.counter_broker_name,
            sec_market: value.sec_market,
            create_timestamp: value.create_timestamp,
            update_timestamp: value.update_timestamp,
            status: value.status,
            trd_market: value.trd_market,
            jp_acc_type: value.jp_acc_type,
        }
    }
}

pub(crate) fn account_projection(
    payload: trade_proto::trd_get_acc_list::S2c,
) -> Vec<TradeAccountSnapshot> {
    payload.acc_list.into_iter().map(Into::into).collect()
}
pub(crate) fn funds_projection(
    header: trd_common::TrdHeader,
    funds: trd_common::Funds,
) -> TradeFundsSnapshot {
    TradeFundsSnapshot {
        header: header.into(),
        funds: funds.into(),
    }
}
pub(crate) fn cash_flows_projection(
    payload: trade_proto::trd_flow_summary::S2c,
) -> Vec<TradeCashFlowSnapshot> {
    let header: TradeHeader = payload.header.into();
    let mut flows = payload
        .flow_summary_info_list
        .into_iter()
        .map(|flow| TradeCashFlowSnapshot::from_proto(header.clone(), flow))
        .collect::<Vec<_>>();
    flows.sort_by_key(|flow| {
        (
            Reverse(flow.clearing_date.as_deref().unwrap_or_default().to_owned()),
            Reverse(flow.cash_flow_id.unwrap_or_default()),
        )
    });
    flows
}
pub(crate) fn positions_projection(
    payload: trade_proto::trd_get_position_list::S2c,
) -> Vec<TradePositionSnapshot> {
    payload.position_list.into_iter().map(Into::into).collect()
}
pub(crate) fn orders_projection(
    payload: trade_proto::trd_get_order_list::S2c,
) -> Vec<TradeOrderSnapshot> {
    payload.order_list.into_iter().map(Into::into).collect()
}
pub(crate) fn fills_projection(
    payload: trade_proto::trd_get_order_fill_list::S2c,
) -> Vec<TradeFillSnapshot> {
    payload
        .order_fill_list
        .into_iter()
        .map(Into::into)
        .collect()
}
