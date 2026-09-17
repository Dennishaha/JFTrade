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

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
pub struct TradeOrderFeeItemSnapshot {
    pub title: String,
    pub value: f64,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
pub struct TradeOrderFeeSnapshot {
    pub header: TradeHeader,
    pub broker_order_id_ex: String,
    pub fee_amount: Option<f64>,
    pub fee_items: Vec<TradeOrderFeeItemSnapshot>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
pub struct TradeSecurity {
    pub market: i32,
    pub code: String,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
pub struct TradeMarginRatioSnapshot {
    pub header: TradeHeader,
    pub market: String,
    pub symbol: String,
    pub is_long_permit: Option<bool>,
    pub is_short_permit: Option<bool>,
    pub short_pool_remain: Option<f64>,
    pub short_fee_rate: Option<f64>,
    pub alert_long_ratio: Option<f64>,
    pub alert_short_ratio: Option<f64>,
    pub initial_margin_long_ratio: Option<f64>,
    pub initial_margin_short_ratio: Option<f64>,
    pub margin_call_long_ratio: Option<f64>,
    pub margin_call_short_ratio: Option<f64>,
    pub maintenance_long_ratio: Option<f64>,
    pub maintenance_short_ratio: Option<f64>,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
pub struct TradeMaxTradeQuantityRequest {
    pub header: TradeHeader,
    pub order_type: i32,
    pub code: String,
    pub price: f64,
    pub order_id: Option<u64>,
    pub adjust_price: Option<bool>,
    pub adjust_side_and_limit: Option<f64>,
    pub sec_market: Option<i32>,
    pub order_id_ex: Option<String>,
    pub session: Option<i32>,
    pub position_id: Option<u64>,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
pub struct TradeMaxTradeQuantitySnapshot {
    pub header: TradeHeader,
    pub code: String,
    pub order_type: i32,
    pub price: f64,
    pub max_cash_buy: f64,
    pub max_cash_and_margin_buy: Option<f64>,
    pub max_position_sell: f64,
    pub max_sell_short: Option<f64>,
    pub max_buy_back: Option<f64>,
    pub long_required_im: Option<f64>,
    pub short_required_im: Option<f64>,
    pub session: Option<i32>,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
pub struct TradeComboMaxTradeQuantityRequest {
    pub header: TradeHeader,
    pub combo_legs: Vec<TradeComboLeg>,
    pub quantity: f64,
    pub price: Option<f64>,
    pub order_type: i32,
    pub order_id_ex: Option<String>,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
pub struct TradeComboMaxTradeQuantitySnapshot {
    pub header: TradeHeader,
    pub nlv_change: Option<f64>,
    pub initial_margin_change: Option<f64>,
    pub maintenance_margin_change: Option<f64>,
    pub option_buy_power: Option<f64>,
    pub max_withdraw_change: Option<f64>,
    pub buying_power_decrease: Option<f64>,
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
                .filter_map(combo_leg_snapshot)
                .collect(),
        }
    }
}

/// Go `brokerOrderLegSnapshots`: keep only legs whose security resolves to a
/// supported Futu market, mapping the dedicated event-contract market (101)
/// onto the public `US.` namespace. A leg with an unknown market is dropped
/// rather than projected as a bogus instrument.
fn combo_leg_snapshot(leg: crate::trade_proto::qot_common::ComboLeg) -> Option<TradeComboLeg> {
    let market = leg.security.market;
    let code = leg.security.code.trim().to_ascii_uppercase();
    if code.is_empty() {
        return None;
    }
    if market == 101 {
        return Some(TradeComboLeg {
            market: 11,
            code,
            side: leg.side,
            qty_ratio: leg.qty_ratio,
            position_id: leg.position_id,
            pred_side: leg.pred_side,
        });
    }
    if !matches!(market, 1 | 11 | 21 | 22 | 31 | 41 | 51 | 61 | 71) {
        return None;
    }
    Some(TradeComboLeg {
        market,
        code,
        side: leg.side,
        qty_ratio: leg.qty_ratio,
        position_id: leg.position_id,
        pred_side: leg.pred_side,
    })
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

pub(crate) fn order_fees_projection(
    payload: trade_proto::trd_get_order_fee::S2c,
) -> Vec<TradeOrderFeeSnapshot> {
    let header: TradeHeader = payload.header.into();
    let mut fees = payload
        .order_fee_list
        .into_iter()
        .map(|fee| TradeOrderFeeSnapshot {
            header: header.clone(),
            broker_order_id_ex: fee.order_id_ex.trim().to_owned(),
            fee_amount: fee.fee_amount,
            fee_items: fee
                .fee_list
                .into_iter()
                .map(|item| TradeOrderFeeItemSnapshot {
                    title: item.title.unwrap_or_default().trim().to_owned(),
                    value: item.value.unwrap_or_default(),
                })
                .collect(),
        })
        .collect::<Vec<_>>();
    fees.sort_by(|left, right| left.broker_order_id_ex.cmp(&right.broker_order_id_ex));
    fees
}

pub(crate) fn margin_ratios_projection(
    payload: trade_proto::trd_get_margin_ratio::S2c,
) -> Vec<TradeMarginRatioSnapshot> {
    let header: TradeHeader = payload.header.into();
    let mut ratios = payload
        .margin_ratio_info_list
        .into_iter()
        .map(|info| {
            let security = info.security;
            let security_market = qot_security_market_label(security.market).unwrap_or_default();
            let market = trade_market_label(security.market).unwrap_or_default();
            let code = security.code.trim().to_ascii_uppercase();
            let symbol = if security_market.is_empty() {
                String::new()
            } else {
                format!("{security_market}.{code}")
            };
            TradeMarginRatioSnapshot {
                header: header.clone(),
                market: market.to_owned(),
                symbol,
                is_long_permit: info.is_long_permit,
                is_short_permit: info.is_short_permit,
                short_pool_remain: info.short_pool_remain,
                short_fee_rate: info.short_fee_rate,
                alert_long_ratio: info.alert_long_ratio,
                alert_short_ratio: info.alert_short_ratio,
                initial_margin_long_ratio: info.im_long_ratio,
                initial_margin_short_ratio: info.im_short_ratio,
                margin_call_long_ratio: info.mcm_long_ratio,
                margin_call_short_ratio: info.mcm_short_ratio,
                maintenance_long_ratio: info.mm_long_ratio,
                maintenance_short_ratio: info.mm_short_ratio,
            }
        })
        .collect::<Vec<_>>();
    ratios.sort_by(|left, right| left.symbol.cmp(&right.symbol));
    ratios
}

pub(crate) fn max_trade_quantity_projection(
    request: &TradeMaxTradeQuantityRequest,
    payload: trade_proto::trd_common::MaxTrdQtys,
) -> TradeMaxTradeQuantitySnapshot {
    TradeMaxTradeQuantitySnapshot {
        header: request.header.clone(),
        code: request.code.clone(),
        order_type: request.order_type,
        price: request.price,
        max_cash_buy: payload.max_cash_buy,
        max_cash_and_margin_buy: payload.max_cash_and_margin_buy,
        max_position_sell: payload.max_position_sell,
        max_sell_short: payload.max_sell_short,
        max_buy_back: payload.max_buy_back,
        long_required_im: payload.long_required_im,
        short_required_im: payload.short_required_im,
        session: payload.session,
    }
}

pub(crate) fn combo_max_trade_quantity_projection(
    request: &TradeComboMaxTradeQuantityRequest,
    payload: trade_proto::trd_common::ComboMaxTrdQtys,
) -> TradeComboMaxTradeQuantitySnapshot {
    TradeComboMaxTradeQuantitySnapshot {
        header: request.header.clone(),
        nlv_change: payload.nlv_change,
        initial_margin_change: payload.initial_margin_change,
        maintenance_margin_change: payload.maintenance_margin_change,
        option_buy_power: payload.option_buy_power,
        max_withdraw_change: payload.max_with_draw_change,
        buying_power_decrease: payload.buy_power_decrease,
    }
}

fn qot_security_market_label(value: i32) -> Option<&'static str> {
    match value {
        1 => Some("HK"),
        11 => Some("US"),
        21 => Some("SH"),
        22 => Some("SZ"),
        31 => Some("SG"),
        41 => Some("JP"),
        51 => Some("AU"),
        61 => Some("MY"),
        71 => Some("CA"),
        _ => None,
    }
}

fn trade_market_label(value: i32) -> Option<&'static str> {
    match value {
        21 | 22 => Some("CN"),
        _ => qot_security_market_label(value),
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
    let mut orders = payload
        .order_list
        .into_iter()
        .map(Into::into)
        .collect::<Vec<_>>();
    // Parity: `go:452dea11:pkg/futu/exchange_trade_read.go:285`
    // `brokerOrderSnapshotsFromProto` sorts by `brokerOrderSortKey` descending
    // (UpdatedAt, falling back to SubmittedAt) and breaks ties on the numeric
    // broker order id descending. OpenD's own ordering is not a contract, so
    // the neutral read must reproduce Go's newest-first rows.
    orders.sort_by(|left, right| {
        order_sort_key(right)
            .cmp(&order_sort_key(left))
            .then_with(|| right.order_id.cmp(&left.order_id))
    });
    orders
}
pub(crate) fn fills_projection(
    payload: trade_proto::trd_get_order_fill_list::S2c,
) -> Vec<TradeFillSnapshot> {
    let mut fills = payload
        .order_fill_list
        .into_iter()
        .map(Into::into)
        .collect::<Vec<_>>();
    // Parity: `go:452dea11:pkg/futu/exchange_trade_read.go:306`
    // `brokerOrderFillSnapshotsFromProto` sorts by FilledAt descending and
    // breaks ties on the numeric broker fill id descending.
    fills.sort_by(|left, right| {
        fill_sort_key(right)
            .cmp(&fill_sort_key(left))
            .then_with(|| right.fill_id.cmp(&left.fill_id))
    });
    fills
}

/// Sort key for `TradeOrderSnapshot`, mirroring Go's `brokerOrderSortKey`:
/// UpdatedAt wins when parseable, otherwise SubmittedAt; unparseable values
/// sort last.
fn order_sort_key(order: &TradeOrderSnapshot) -> (u8, i64, i64) {
    let updated = time_sort_key(order.update_timestamp, &order.update_time);
    if updated.0 == 1 {
        return updated;
    }
    time_sort_key(order.create_timestamp, &order.create_time)
}

fn fill_sort_key(fill: &TradeFillSnapshot) -> (u8, i64, i64) {
    time_sort_key(fill.create_timestamp, &fill.create_time)
}

/// Mirrors Go's `formatBrokerOrderTime(timestamp, fallback, ...)`: a positive
/// unix seconds timestamp wins over the textual fallback, and unparseable text
/// sorts last. Naive fallback strings are interpreted as UTC; the engine's
/// per-market timezone normalization happens before this projection.
fn time_sort_key(timestamp: Option<f64>, fallback: &str) -> (u8, i64, i64) {
    if let Some(timestamp) = timestamp.filter(|value| value.is_finite() && *value > 0.0) {
        let seconds = timestamp.trunc() as i64;
        let nanos = ((timestamp.fract()) * 1_000_000_000.0).round() as i64;
        return (1, seconds, nanos.clamp(0, 999_999_999));
    }
    parse_order_time(fallback)
        .map(|(seconds, nanos)| (1, seconds, nanos))
        .unwrap_or((0, 0, 0))
}

/// Parses the RFC3339 form OpenD emits (`2026-06-20T13:35:00Z`, fractional
/// seconds allowed) plus the space-separated fallback form
/// (`2026-06-20 09:30:00`), returning `(unix_seconds, nanosecond_remainder)`.
/// Anything unparseable yields `None` so callers can sort it last.
fn parse_order_time(value: &str) -> Option<(i64, i64)> {
    let trimmed = value.trim();
    if trimmed.is_empty() {
        return None;
    }
    if let Ok(parsed) =
        time::OffsetDateTime::parse(trimmed, &time::format_description::well_known::Rfc3339)
    {
        return Some((parsed.unix_timestamp(), i64::from(parsed.nanosecond())));
    }
    for format in [
        "[year]-[month]-[day] [hour]:[minute]:[second].[subsecond]",
        "[year]-[month]-[day] [hour]:[minute]:[second]",
    ] {
        if let Ok(format) = time::format_description::parse_borrowed::<2>(format)
            && let Ok(parsed) = time::PrimitiveDateTime::parse(trimmed, &format)
        {
            let utc = parsed.assume_utc();
            return Some((utc.unix_timestamp(), i64::from(utc.nanosecond())));
        }
    }
    None
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::trade_proto::trd_common::{Order, OrderFill};

    /// Parity: go:452dea11:pkg/futu/advanced_product_adapter_contracts_test.go:403
    /// `brokerPositionSnapshotFromProto` / `brokerOrderSnapshotFromProto`.
    ///
    /// Go asserts that a combo position keeps its `ComboID`, that a position on
    /// the FUTURES trade market projects as a future, that an order carrying an
    /// `orderAmount` is classified as an event-contract single order, and that
    /// combo legs keep only resolvable securities. Rust keeps those fields on
    /// `TradePositionSnapshot` / `TradeOrderSnapshot`, so the neutral protobuf
    /// projection is the authoritative owner for the same assertions.
    #[test]
    fn trade_snapshot_projection_preserves_combo_event_and_leg_identity() {
        let combo_position = TradePositionSnapshot::from(trade_proto::trd_common::Position {
            position_id: 7,
            position_side: 1,
            code: "OPTION-COMBO".to_owned(),
            name: String::new(),
            qty: 1.0,
            can_sell_qty: 1.0,
            price: 1.0,
            cost_price: None,
            val: 1.0,
            pl_val: 0.0,
            pl_ratio: None,
            sec_market: None,
            td_pl_val: None,
            td_trd_val: None,
            td_buy_val: None,
            td_buy_qty: None,
            td_sell_val: None,
            td_sell_qty: None,
            unrealized_pl: None,
            realized_pl: None,
            currency: None,
            trd_market: None,
            diluted_cost_price: None,
            average_cost_price: None,
            average_pl_ratio: None,
            combo_id: Some(99),
            strategy_type: Some(4),
            position_type: None,
            acc_id: None,
            jp_acc_type: None,
            payout_if_win: None,
        });
        assert_eq!(combo_position.combo_id, Some(99));
        assert_eq!(combo_position.strategy_type, Some(4));

        let futures_position = TradePositionSnapshot::from(trade_proto::trd_common::Position {
            code: "ES".to_owned(),
            trd_market: Some(1),
            ..combo_position_source()
        });
        assert_eq!(futures_position.code, "ES");
        assert_eq!(futures_position.trd_market, Some(1));

        let event_order = TradeOrderSnapshot::from(trade_proto::trd_common::Order {
            code: "EVENT".to_owned(),
            order_amount: Some(20.0),
            ..order(
                1,
                "2026-06-20T13:30:00Z",
                "2026-06-20T13:30:00Z",
                None,
                None,
            )
        });
        assert_eq!(event_order.order_amount, Some(20.0));

        let legs = TradeOrderSnapshot::from(trade_proto::trd_common::Order {
            qty: 1.0,
            combo_legs: vec![
                trade_proto::qot_common::ComboLeg {
                    security: trade_proto::qot_common::Security {
                        market: 999,
                        code: "BAD".to_owned(),
                    },
                    side: None,
                    qty_ratio: None,
                    position_id: None,
                    pred_side: None,
                },
                trade_proto::qot_common::ComboLeg {
                    security: trade_proto::qot_common::Security {
                        market: 101,
                        code: "EVENT".to_owned(),
                    },
                    side: Some(1),
                    qty_ratio: Some(1.0),
                    position_id: None,
                    pred_side: Some(1),
                },
            ],
            ..order(
                2,
                "2026-06-20T13:31:00Z",
                "2026-06-20T13:31:00Z",
                None,
                None,
            )
        });
        // Go drops the unresolvable market-999 leg and projects the
        // event-contract leg (101) into the public US namespace.
        assert_eq!(legs.combo_legs.len(), 1, "unresolvable legs are dropped");
        assert_eq!(legs.combo_legs[0].market, 11);
        assert_eq!(legs.combo_legs[0].code, "EVENT");
        assert_eq!(legs.combo_legs[0].pred_side, Some(1));
    }

    fn combo_position_source() -> trade_proto::trd_common::Position {
        trade_proto::trd_common::Position {
            position_id: 8,
            position_side: 1,
            code: String::new(),
            name: String::new(),
            qty: 1.0,
            can_sell_qty: 1.0,
            price: 1.0,
            cost_price: None,
            val: 1.0,
            pl_val: 0.0,
            pl_ratio: None,
            sec_market: None,
            td_pl_val: None,
            td_trd_val: None,
            td_buy_val: None,
            td_buy_qty: None,
            td_sell_val: None,
            td_sell_qty: None,
            unrealized_pl: None,
            realized_pl: None,
            currency: None,
            trd_market: None,
            diluted_cost_price: None,
            average_cost_price: None,
            average_pl_ratio: None,
            combo_id: None,
            strategy_type: None,
            position_type: None,
            acc_id: None,
            jp_acc_type: None,
            payout_if_win: None,
        }
    }

    fn order(
        id: u64,
        create_time: &str,
        update_time: &str,
        create_timestamp: Option<f64>,
        update_timestamp: Option<f64>,
    ) -> Order {
        Order {
            trd_side: 1,
            order_type: 1,
            order_status: 5,
            order_id: id,
            order_id_ex: String::new(),
            code: "AAPL".to_owned(),
            name: String::new(),
            qty: 1.0,
            price: None,
            create_time: create_time.to_owned(),
            update_time: update_time.to_owned(),
            fill_qty: None,
            fill_avg_price: None,
            last_err_msg: None,
            sec_market: None,
            create_timestamp,
            update_timestamp,
            remark: None,
            trd_market: None,
            expire_time: None,
            order_amount: None,
            time_in_force: None,
            fill_outside_rth: None,
            aux_price: None,
            trail_type: None,
            trail_value: None,
            trail_spread: None,
            currency: None,
            session: None,
            jp_acc_type: None,
            strategy_type: None,
            combo_legs: Vec::new(),
        }
    }

    fn fill(id: u64, create_time: &str, create_timestamp: Option<f64>) -> OrderFill {
        OrderFill {
            trd_side: 1,
            fill_id: id,
            fill_id_ex: String::new(),
            order_id: None,
            order_id_ex: None,
            code: "AAPL".to_owned(),
            name: String::new(),
            qty: 1.0,
            price: 1.0,
            create_time: create_time.to_owned(),
            counter_broker_id: None,
            counter_broker_name: None,
            sec_market: None,
            create_timestamp,
            update_timestamp: None,
            status: None,
            trd_market: None,
            jp_acc_type: None,
        }
    }

    #[test]
    fn orders_projection_sorts_newest_updated_first_with_id_tiebreak() {
        // Parity: go:452dea11:pkg/futu/exchange_business_boundary_test.go:92
        // TestBalanceMapFromFundsAndBrokerOrderSortBoundaries (order sort keys)
        // plus :285 `brokerOrderSnapshotsFromProto` ordering.
        let orders = orders_projection(trade_proto::trd_get_order_list::S2c {
            header: Default::default(),
            order_list: vec![
                order(11, "2026-06-20 09:30:00", "2026-06-20 13:35:00", None, None),
                order(
                    12,
                    "2026-06-20T13:30:00Z",
                    "2026-06-20T13:35:00Z",
                    None,
                    None,
                ),
                // Unparseable timestamps must sort last, not panic or win.
                order(13, "not-a-time", "", None, None),
                // Timestamp fields win over textual fallbacks, as in Go's
                // `formatBrokerOrderTime`.
                order(
                    14,
                    "2026-06-20T13:40:00Z",
                    "2026-06-20T13:40:00Z",
                    None,
                    Some(1_781_962_800.0),
                ),
            ],
        });
        assert_eq!(
            orders
                .iter()
                .map(|order| order.order_id)
                .collect::<Vec<_>>(),
            vec![14, 12, 11, 13],
            "updatedAt desc, then broker order id desc, unparseable last"
        );
    }

    #[test]
    fn orders_projection_falls_back_to_submitted_at_and_applies_timestamp_fields() {
        // Parity: go:452dea11:pkg/futu/exchange_trade_read.go:285 and
        // trade_read_convert.go:176 `brokerOrderSortKey` submitted fallback.
        let orders = orders_projection(trade_proto::trd_get_order_list::S2c {
            header: Default::default(),
            order_list: vec![
                order(21, "2026-06-20 09:30:00", "", None, None),
                order(22, "2026-06-20 09:31:00", "", None, None),
                order(23, "2026-06-20 14:00:00", "", Some(1_781_964_000.0), None),
            ],
        });
        assert_eq!(
            orders
                .iter()
                .map(|order| order.order_id)
                .collect::<Vec<_>>(),
            vec![23, 22, 21]
        );
    }

    #[test]
    fn fills_projection_sorts_filled_at_desc_with_id_tiebreak() {
        // Parity: go:452dea11:pkg/futu/exchange_business_boundary_test.go:92
        // `brokerOrderFillSortKey` plus :306 fill ordering.
        let fills = fills_projection(trade_proto::trd_get_order_fill_list::S2c {
            header: Default::default(),
            order_fill_list: vec![
                fill(31, "2026-06-20T13:31:00Z", None),
                fill(32, "2026-06-20T13:31:00Z", None),
                fill(33, "2026-06-20T13:35:00Z", None),
                fill(34, "", None),
            ],
        });
        assert_eq!(
            fills.iter().map(|fill| fill.fill_id).collect::<Vec<_>>(),
            vec![33, 32, 31, 34]
        );
    }
}
