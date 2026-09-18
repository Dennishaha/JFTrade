//! Generated Futu trade protobuf modules and strict response validation.

/// Canonical OpenD protocol id for a framed request/response pair, resolved
/// from the generated module that actually encodes it.
///
/// The capability catalog mirrors the same wire ids, so this lookup is the
/// single place a test can prove "every catalog protocol maps to a framed
/// OpenD protocol" instead of trusting a duplicated literal.
pub fn framed_protocol_id(protocol: &str) -> Option<u32> {
    match protocol {
        "Qot_FilterCompetition" => Some(crate::trade_proto::qot_filter_competition::PROTOCOL_ID),
        "Qot_GetArkActiveTransaction" => {
            Some(crate::trade_proto::qot_get_ark_active_transaction::PROTOCOL_ID)
        }
        "Qot_GetArkFundHolding" => Some(crate::trade_proto::qot_get_ark_fund_holding::PROTOCOL_ID),
        "Qot_GetArkStockDynamic" => {
            Some(crate::trade_proto::qot_get_ark_stock_dynamic::PROTOCOL_ID)
        }
        "Qot_GetBroker" => Some(crate::trade_proto::qot_get_broker::PROTOCOL_ID),
        "Qot_GetCapitalDistribution" => {
            Some(crate::trade_proto::qot_get_capital_distribution::PROTOCOL_ID)
        }
        "Qot_GetCapitalFlow" => Some(crate::trade_proto::qot_get_capital_flow::PROTOCOL_ID),
        "Qot_GetCompanyProfile" => Some(crate::trade_proto::qot_get_company_profile::PROTOCOL_ID),
        "Qot_GetCorporateActionsBuybacks" => {
            Some(crate::trade_proto::qot_get_corporate_actions_buybacks::PROTOCOL_ID)
        }
        "Qot_GetCorporateActionsDividends" => {
            Some(crate::trade_proto::qot_get_corporate_actions_dividends::PROTOCOL_ID)
        }
        "Qot_GetCorporateActionsStockSplits" => {
            Some(crate::trade_proto::qot_get_corporate_actions_stock_splits::PROTOCOL_ID)
        }
        "Qot_GetDailyShortVolume" => {
            Some(crate::trade_proto::qot_get_daily_short_volume::PROTOCOL_ID)
        }
        "Qot_GetEarningsCalendar" => {
            Some(crate::trade_proto::qot_get_earnings_calendar::PROTOCOL_ID)
        }
        "Qot_GetEventContract" => Some(crate::trade_proto::qot_get_event_contract::PROTOCOL_ID),
        "Qot_GetEventContractCategory" => {
            Some(crate::trade_proto::qot_get_event_contract_category::PROTOCOL_ID)
        }
        "Qot_GetEventContractComboList" => {
            Some(crate::trade_proto::qot_get_event_contract_combo_list::PROTOCOL_ID)
        }
        "Qot_GetEventContractComboRfq" => {
            Some(crate::trade_proto::qot_get_event_contract_combo_rfq::PROTOCOL_ID)
        }
        "Qot_GetEventContractEventList" => {
            Some(crate::trade_proto::qot_get_event_contract_event_list::PROTOCOL_ID)
        }
        "Qot_GetEventContractKline" => {
            Some(crate::trade_proto::qot_get_event_contract_kline::PROTOCOL_ID)
        }
        "Qot_GetEventContractMilestoneList" => {
            Some(crate::trade_proto::qot_get_event_contract_milestone_list::PROTOCOL_ID)
        }
        "Qot_GetEventContractOrderBook" => {
            Some(crate::trade_proto::qot_get_event_contract_order_book::PROTOCOL_ID)
        }
        "Qot_GetEventContractSeriesList" => {
            Some(crate::trade_proto::qot_get_event_contract_series_list::PROTOCOL_ID)
        }
        "Qot_GetEventContractSnapshot" => {
            Some(crate::trade_proto::qot_get_event_contract_snapshot::PROTOCOL_ID)
        }
        "Qot_GetEventContractTicker" => {
            Some(crate::trade_proto::qot_get_event_contract_ticker::PROTOCOL_ID)
        }
        "Qot_GetFutureInfo" => Some(crate::trade_proto::qot_get_future_info::PROTOCOL_ID),
        "Qot_GetIndicatorList" => Some(crate::trade_proto::qot_get_indicator_list::PROTOCOL_ID),
        "Qot_GetInstitutionDistribution" => {
            Some(crate::trade_proto::qot_get_institution_distribution::PROTOCOL_ID)
        }
        "Qot_GetInstitutionHoldingChange" => {
            Some(crate::trade_proto::qot_get_institution_holding_change::PROTOCOL_ID)
        }
        "Qot_GetInstitutionHoldingList" => {
            Some(crate::trade_proto::qot_get_institution_holding_list::PROTOCOL_ID)
        }
        "Qot_GetInstitutionList" => Some(crate::trade_proto::qot_get_institution_list::PROTOCOL_ID),
        "Qot_GetInstitutionProfile" => {
            Some(crate::trade_proto::qot_get_institution_profile::PROTOCOL_ID)
        }
        "Qot_GetKl" => Some(crate::trade_proto::qot_get_kl::PROTOCOL_ID),
        "Qot_GetOptionChain" => Some(crate::trade_proto::qot_get_option_chain::PROTOCOL_ID),
        "Qot_GetOptionEarningsScreener" => {
            Some(crate::trade_proto::qot_get_option_earnings_screener::PROTOCOL_ID)
        }
        "Qot_GetOptionEvent" => Some(crate::trade_proto::qot_get_option_event::PROTOCOL_ID),
        "Qot_GetOptionExerciseProbability" => {
            Some(crate::trade_proto::qot_get_option_exercise_probability::PROTOCOL_ID)
        }
        "Qot_GetOptionExpirationDate" => {
            Some(crate::trade_proto::qot_get_option_expiration_date::PROTOCOL_ID)
        }
        "Qot_GetOptionMarketStatistic" => {
            Some(crate::trade_proto::qot_get_option_market_statistic::PROTOCOL_ID)
        }
        "Qot_GetOptionQuote" => Some(crate::trade_proto::qot_get_option_quote::PROTOCOL_ID),
        "Qot_GetOptionRank" => Some(crate::trade_proto::qot_get_option_rank::PROTOCOL_ID),
        "Qot_GetOptionSellerScreener" => {
            Some(crate::trade_proto::qot_get_option_seller_screener::PROTOCOL_ID)
        }
        "Qot_GetOptionStrategy" => Some(crate::trade_proto::qot_get_option_strategy::PROTOCOL_ID),
        "Qot_GetOptionStrategyAnalysis" => {
            Some(crate::trade_proto::qot_get_option_strategy_analysis::PROTOCOL_ID)
        }
        "Qot_GetOptionStrategySpread" => {
            Some(crate::trade_proto::qot_get_option_strategy_spread::PROTOCOL_ID)
        }
        "Qot_GetOptionUnderlyingHisStatistic" => {
            Some(crate::trade_proto::qot_get_option_underlying_his_statistic::PROTOCOL_ID)
        }
        "Qot_GetOptionUnderlyingHisVolatility" => {
            Some(crate::trade_proto::qot_get_option_underlying_his_volatility::PROTOCOL_ID)
        }
        "Qot_GetOptionUnderlyingOverview" => {
            Some(crate::trade_proto::qot_get_option_underlying_overview::PROTOCOL_ID)
        }
        "Qot_GetOptionUnderlyingRank" => {
            Some(crate::trade_proto::qot_get_option_underlying_rank::PROTOCOL_ID)
        }
        "Qot_GetOptionVolatility" => {
            Some(crate::trade_proto::qot_get_option_volatility::PROTOCOL_ID)
        }
        "Qot_GetOptionZeroDteContract" => {
            Some(crate::trade_proto::qot_get_option_zero_dte_contract::PROTOCOL_ID)
        }
        "Qot_GetOptionZeroDteScreener" => {
            Some(crate::trade_proto::qot_get_option_zero_dte_screener::PROTOCOL_ID)
        }
        "Qot_GetOrderBook" => Some(crate::trade_proto::qot_get_order_book::PROTOCOL_ID),
        "Qot_GetRt" => Some(crate::trade_proto::qot_get_rt::PROTOCOL_ID),
        "Qot_GetSearchNews" => Some(crate::trade_proto::qot_get_search_news::PROTOCOL_ID),
        "Qot_GetSearchQuote" => Some(crate::trade_proto::qot_get_search_quote::PROTOCOL_ID),
        "Qot_GetShortInterest" => Some(crate::trade_proto::qot_get_short_interest::PROTOCOL_ID),
        "Qot_GetStaticInfo" => Some(crate::trade_proto::qot_get_static_info::PROTOCOL_ID),
        "Qot_GetTicker" => Some(crate::trade_proto::qot_get_ticker::PROTOCOL_ID),
        "Qot_GetValuationDetail" => Some(crate::trade_proto::qot_get_valuation_detail::PROTOCOL_ID),
        "Qot_OptionScreen" => Some(crate::trade_proto::qot_option_screen::PROTOCOL_ID),
        "Qot_RequestHistoryEventContractKl" => {
            Some(crate::trade_proto::qot_request_history_event_contract_kl::PROTOCOL_ID)
        }
        "Qot_RequestIndicatorCalc" => {
            Some(crate::trade_proto::qot_request_indicator_calc::PROTOCOL_ID)
        }
        "Qot_StockScreen" => Some(crate::trade_proto::qot_stock_screen::PROTOCOL_ID),
        "Qot_SubEventContract" => Some(crate::trade_proto::qot_sub_event_contract::PROTOCOL_ID),
        "Trd_GetComboMaxTrdQtys" => {
            Some(crate::trade_proto::trd_get_combo_max_trd_qtys::PROTOCOL_ID)
        }
        "Trd_GetMaxTrdQtys" => Some(crate::trade_proto::trd_get_max_trd_qtys::PROTOCOL_ID),
        _ => None,
    }
}

pub mod common {
    include!(concat!(env!("OUT_DIR"), "/common.rs"));
}
pub mod qot_common {
    include!(concat!(env!("OUT_DIR"), "/qot_common.rs"));
}
pub mod qot_option_common {
    include!(concat!(env!("OUT_DIR"), "/qot_option_common.rs"));
}
pub mod qot_stock_screen {
    include!(concat!(env!("OUT_DIR"), "/qot_stock_screen.rs"));
    pub const PROTOCOL_ID: u32 = 3252;
}
pub mod qot_get_security_snapshot {
    include!(concat!(env!("OUT_DIR"), "/qot_get_security_snapshot.rs"));
}
pub mod qot_get_search_quote {
    include!(concat!(env!("OUT_DIR"), "/qot_get_search_quote.rs"));
    pub const PROTOCOL_ID: u32 = 3262;
}
pub mod qot_get_static_info {
    include!(concat!(env!("OUT_DIR"), "/qot_get_static_info.rs"));
    pub const PROTOCOL_ID: u32 = 3202;
}
pub mod qot_get_kl {
    include!(concat!(env!("OUT_DIR"), "/qot_get_kl.rs"));
    pub const PROTOCOL_ID: u32 = 3006;
}
pub mod qot_get_order_book {
    include!(concat!(env!("OUT_DIR"), "/qot_get_order_book.rs"));
    pub const PROTOCOL_ID: u32 = 3012;
}
pub mod qot_get_ticker {
    include!(concat!(env!("OUT_DIR"), "/qot_get_ticker.rs"));
    pub const PROTOCOL_ID: u32 = 3010;
}
pub mod qot_get_broker {
    include!(concat!(env!("OUT_DIR"), "/qot_get_broker.rs"));
    pub const PROTOCOL_ID: u32 = 3014;
}
pub mod qot_get_rt {
    include!(concat!(env!("OUT_DIR"), "/qot_get_rt.rs"));
    pub const PROTOCOL_ID: u32 = 3008;
}
pub mod qot_get_capital_flow {
    include!(concat!(env!("OUT_DIR"), "/qot_get_capital_flow.rs"));
    pub const PROTOCOL_ID: u32 = 3211;
}
pub mod qot_get_capital_distribution {
    include!(concat!(env!("OUT_DIR"), "/qot_get_capital_distribution.rs"));
    pub const PROTOCOL_ID: u32 = 3212;
}
pub mod qot_get_company_profile {
    include!(concat!(env!("OUT_DIR"), "/qot_get_company_profile.rs"));
    pub const PROTOCOL_ID: u32 = 3243;
}
pub mod qot_get_daily_short_volume {
    include!(concat!(env!("OUT_DIR"), "/qot_get_daily_short_volume.rs"));
    pub const PROTOCOL_ID: u32 = 3248;
}
pub mod qot_get_short_interest {
    include!(concat!(env!("OUT_DIR"), "/qot_get_short_interest.rs"));
    pub const PROTOCOL_ID: u32 = 3249;
}
pub mod qot_get_indicator_list {
    include!(concat!(env!("OUT_DIR"), "/qot_get_indicator_list.rs"));
    pub const PROTOCOL_ID: u32 = 3259;
}
pub mod qot_request_indicator_calc {
    include!(concat!(env!("OUT_DIR"), "/qot_request_indicator_calc.rs"));
    pub const PROTOCOL_ID: u32 = 3260;
}
pub mod qot_get_institution_list {
    include!(concat!(env!("OUT_DIR"), "/qot_get_institution_list.rs"));
    pub const PROTOCOL_ID: u32 = 3418;
}
pub mod qot_get_institution_profile {
    include!(concat!(env!("OUT_DIR"), "/qot_get_institution_profile.rs"));
    pub const PROTOCOL_ID: u32 = 3419;
}
pub mod qot_get_institution_distribution {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_institution_distribution.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3420;
}
pub mod qot_get_institution_holding_change {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_institution_holding_change.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3421;
}
pub mod qot_get_institution_holding_list {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_institution_holding_list.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3422;
}
pub mod qot_get_ark_fund_holding {
    include!(concat!(env!("OUT_DIR"), "/qot_get_ark_fund_holding.rs"));
    pub const PROTOCOL_ID: u32 = 3423;
}
pub mod qot_get_ark_stock_dynamic {
    include!(concat!(env!("OUT_DIR"), "/qot_get_ark_stock_dynamic.rs"));
    pub const PROTOCOL_ID: u32 = 3424;
}
pub mod qot_get_ark_active_transaction {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_ark_active_transaction.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3425;
}
pub mod qot_get_user_security_group {
    include!(concat!(env!("OUT_DIR"), "/qot_get_user_security_group.rs"));
}
pub mod qot_get_user_security {
    include!(concat!(env!("OUT_DIR"), "/qot_get_user_security.rs"));
}
pub mod qot_modify_user_security {
    include!(concat!(env!("OUT_DIR"), "/qot_modify_user_security.rs"));
}
pub mod qot_get_price_reminder {
    include!(concat!(env!("OUT_DIR"), "/qot_get_price_reminder.rs"));
}
pub mod qot_set_price_reminder {
    include!(concat!(env!("OUT_DIR"), "/qot_set_price_reminder.rs"));
}
pub mod qot_get_option_event_alert {
    include!(concat!(env!("OUT_DIR"), "/qot_get_option_event_alert.rs"));
}
pub mod qot_set_option_event_alert {
    include!(concat!(env!("OUT_DIR"), "/qot_set_option_event_alert.rs"));
}
pub mod qot_get_future_info {
    include!(concat!(env!("OUT_DIR"), "/qot_get_future_info.rs"));
    pub const PROTOCOL_ID: u32 = 3218;
}
pub mod qot_get_valuation_detail {
    include!(concat!(env!("OUT_DIR"), "/qot_get_valuation_detail.rs"));
    pub const PROTOCOL_ID: u32 = 3232;
}
pub mod qot_get_search_news {
    include!(concat!(env!("OUT_DIR"), "/qot_get_search_news.rs"));
    pub const PROTOCOL_ID: u32 = 3263;
}
pub mod qot_get_corporate_actions_dividends {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_corporate_actions_dividends.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3234;
}
pub mod qot_get_corporate_actions_buybacks {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_corporate_actions_buybacks.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3235;
}
pub mod qot_get_corporate_actions_stock_splits {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_corporate_actions_stock_splits.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3236;
}
pub mod qot_get_option_expiration_date {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_option_expiration_date.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3224;
}
pub mod qot_get_option_chain {
    include!(concat!(env!("OUT_DIR"), "/qot_get_option_chain.rs"));
    pub const PROTOCOL_ID: u32 = 3209;
}
pub mod qot_get_option_quote {
    include!(concat!(env!("OUT_DIR"), "/qot_get_option_quote.rs"));
    pub const PROTOCOL_ID: u32 = 3255;
}
pub mod qot_get_option_volatility {
    include!(concat!(env!("OUT_DIR"), "/qot_get_option_volatility.rs"));
    pub const PROTOCOL_ID: u32 = 3250;
}
pub mod qot_get_option_exercise_probability {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_option_exercise_probability.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3251;
}
pub mod qot_get_option_strategy {
    include!(concat!(env!("OUT_DIR"), "/qot_get_option_strategy.rs"));
    pub const PROTOCOL_ID: u32 = 3256;
}
pub mod qot_get_option_strategy_analysis {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_option_strategy_analysis.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3257;
}
pub mod qot_get_option_underlying_overview {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_option_underlying_overview.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3303;
}
pub mod qot_get_option_market_statistic {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_option_market_statistic.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3301;
}
pub mod qot_get_option_underlying_his_statistic {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_option_underlying_his_statistic.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3302;
}
pub mod qot_get_option_strategy_spread {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_option_strategy_spread.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3258;
}
pub mod qot_get_option_underlying_his_volatility {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_option_underlying_his_volatility.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3304;
}
pub mod qot_get_option_underlying_rank {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_option_underlying_rank.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3305;
}
pub mod qot_get_option_rank {
    include!(concat!(env!("OUT_DIR"), "/qot_get_option_rank.rs"));
    pub const PROTOCOL_ID: u32 = 3306;
}
pub mod qot_get_option_event {
    include!(concat!(env!("OUT_DIR"), "/qot_get_option_event.rs"));
    pub const PROTOCOL_ID: u32 = 3307;
}
pub mod qot_get_option_zero_dte_screener {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_option_zero_dte_screener.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3311;
}
pub mod qot_get_option_zero_dte_contract {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_option_zero_dte_contract.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3312;
}
pub mod qot_get_option_earnings_screener {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_option_earnings_screener.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3313;
}
pub mod qot_get_earnings_calendar {
    include!(concat!(env!("OUT_DIR"), "/qot_get_earnings_calendar.rs"));
    pub const PROTOCOL_ID: u32 = 3401;
}
pub mod qot_get_option_seller_screener {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_option_seller_screener.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3314;
}
pub mod qot_option_screen {
    include!(concat!(env!("OUT_DIR"), "/qot_option_screen.rs"));
    pub const PROTOCOL_ID: u32 = 3253;
}
pub mod qot_get_event_contract_category {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_event_contract_category.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3434;
}
pub mod qot_filter_competition {
    include!(concat!(env!("OUT_DIR"), "/qot_filter_competition.rs"));
    pub const PROTOCOL_ID: u32 = 3435;
}
pub mod qot_get_event_contract_series_list {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_event_contract_series_list.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3436;
}
pub mod qot_get_event_contract_event_list {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_event_contract_event_list.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3437;
}
pub mod qot_get_event_contract {
    include!(concat!(env!("OUT_DIR"), "/qot_get_event_contract.rs"));
    pub const PROTOCOL_ID: u32 = 3438;
}
pub mod qot_get_event_contract_milestone_list {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_event_contract_milestone_list.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3439;
}
pub mod qot_get_event_contract_snapshot {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_event_contract_snapshot.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3445;
}
pub mod qot_get_event_contract_order_book {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_event_contract_order_book.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3446;
}
pub mod qot_get_event_contract_kline {
    include!(concat!(env!("OUT_DIR"), "/qot_get_event_contract_kline.rs"));
    pub const PROTOCOL_ID: u32 = 3447;
}
pub mod qot_get_event_contract_ticker {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_event_contract_ticker.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3448;
}
pub mod qot_get_event_contract_combo_list {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_event_contract_combo_list.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3453;
}
pub mod qot_get_event_contract_combo_rfq {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_get_event_contract_combo_rfq.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3454;
}
pub mod qot_sub_event_contract {
    include!(concat!(env!("OUT_DIR"), "/qot_sub_event_contract.rs"));
    pub const PROTOCOL_ID: u32 = 3455;
}
pub mod qot_update_event_contract_order_book {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_update_event_contract_order_book.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3450;
}
pub mod qot_update_event_contract_kline {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_update_event_contract_kline.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3451;
}
pub mod qot_update_event_contract_ticker {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_update_event_contract_ticker.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3452;
}
pub mod qot_request_history_event_contract_kl {
    include!(concat!(
        env!("OUT_DIR"),
        "/qot_request_history_event_contract_kl.rs"
    ));
    pub const PROTOCOL_ID: u32 = 3456;
}
pub mod trd_common {
    include!(concat!(env!("OUT_DIR"), "/trd_common.rs"));
}

use crate::trade_proto_fee_validation::validate_order_fee_s2c;
use crate::trade_proto_fill_validation::validate_fill_s2c;
use crate::trade_proto_margin_ratio_validation::validate_margin_ratio_s2c;
use crate::trade_proto_max_qty_validation::validate_max_trade_quantity_s2c;
use crate::trade_proto_order_validation::validate_order_s2c;
use crate::trade_proto_validation::{
    validate_account_s2c, validate_cash_flow_s2c, validate_funds, validate_position_s2c,
};

#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum ResponseError {
    #[error("OpenD retType={ret_type} errCode={err_code}: {message}")]
    ReturnCode {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD response missing required s2c")]
    MissingS2c,
    #[error("OpenD response missing required maxTrdQtys")]
    MissingMaxTradeQuantity,
    #[error("failed to decode OpenD {operation} response: {message}")]
    Decode {
        operation: &'static str,
        message: String,
    },
    #[error("OpenD {operation} field {field} must be a YYYY-MM-DD HH:MM:SS[.MS] timestamp")]
    InvalidTime {
        operation: &'static str,
        field: &'static str,
    },
    #[error("{0}")]
    Validation(#[from] ValidationError),
}

#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum ValidationError {
    #[error("OpenD {operation} field {field} must be finite")]
    NonFinite {
        operation: &'static str,
        field: String,
    },
    #[error("OpenD {operation} field {field} must not be empty")]
    EmptyField {
        operation: &'static str,
        field: &'static str,
    },
    #[error("OpenD {operation} field {field} must be non-negative")]
    Negative {
        operation: &'static str,
        field: &'static str,
    },
    #[error("OpenD {operation} field {field} must be a YYYY-MM-DD HH:MM:SS[.MS] timestamp")]
    InvalidTime {
        operation: &'static str,
        field: &'static str,
    },
    #[error("OpenD {operation} field {field} has an unsupported value")]
    UnsupportedValue {
        operation: &'static str,
        field: String,
    },
}

pub(crate) fn validate_response_for(
    ret_type: i32,
    err_code: Option<i32>,
    ret_msg: Option<&str>,
    has_s2c: bool,
) -> Result<(), ResponseError> {
    if ret_type != 0 {
        return Err(ResponseError::ReturnCode {
            ret_type,
            err_code: err_code.unwrap_or_default(),
            message: ret_msg.unwrap_or_default().to_owned(),
        });
    }
    if !has_s2c {
        return Err(ResponseError::MissingS2c);
    }
    Ok(())
}

macro_rules! trade_list_proto {
    (
        $module:ident,
        $file:literal,
        $operation:literal,
        $protocol_id:literal,
        $validator:ident
    ) => {
        pub mod $module {
            use prost::Message;

            include!(concat!(env!("OUT_DIR"), "/", $file));

            /// Futu OpenD protocol identifier for this operation.
            pub const PROTOCOL_ID: u32 = $protocol_id;

            /// Encodes a typed request body for the OpenD frame payload.
            pub fn encode_request(request: &Request) -> Vec<u8> {
                request.encode_to_vec()
            }

            /// Decodes and validates a typed OpenD response, returning its S2C payload.
            pub fn decode_response(body: &[u8]) -> Result<S2c, super::ResponseError> {
                let response =
                    Response::decode(body).map_err(|error| super::ResponseError::Decode {
                        operation: $operation,
                        message: error.to_string(),
                    })?;
                super::validate_response_for(
                    response.ret_type,
                    response.err_code,
                    response.ret_msg.as_deref(),
                    true,
                )?;
                let payload = response.s2c.unwrap_or_default();
                super::$validator($operation, &payload)?;
                Ok(payload)
            }
        }
    };
}

macro_rules! trade_funds_proto {
    ($module:ident, $file:literal, $operation:literal, $protocol_id:literal) => {
        pub mod $module {
            use prost::Message;

            include!(concat!(env!("OUT_DIR"), "/", $file));

            /// Futu OpenD protocol identifier for this operation.
            pub const PROTOCOL_ID: u32 = $protocol_id;

            /// Encodes a typed request body for the OpenD frame payload.
            pub fn encode_request(request: &Request) -> Vec<u8> {
                request.encode_to_vec()
            }

            /// Decodes the funds projection, normalizing absent S2C/funds to zero values.
            pub fn decode_response(
                body: &[u8],
            ) -> Result<super::trd_common::Funds, super::ResponseError> {
                let response =
                    Response::decode(body).map_err(|error| super::ResponseError::Decode {
                        operation: $operation,
                        message: error.to_string(),
                    })?;
                super::validate_response_for(
                    response.ret_type,
                    response.err_code,
                    response.ret_msg.as_deref(),
                    true,
                )?;
                let funds = response.s2c.and_then(|s2c| s2c.funds).unwrap_or_default();
                super::validate_funds($operation, &funds)?;
                Ok(funds)
            }
        }
    };
}

trade_list_proto!(
    trd_get_acc_list,
    "trd_get_acc_list.rs",
    "GetAccountList",
    2001,
    validate_account_s2c
);

trade_list_proto!(
    trd_flow_summary,
    "trd_flow_summary.rs",
    "FlowSummary",
    2226,
    validate_cash_flow_s2c
);
trade_funds_proto!(trd_get_funds, "trd_get_funds.rs", "GetFunds", 2101);
trade_list_proto!(
    trd_get_position_list,
    "trd_get_position_list.rs",
    "GetPositionList",
    2102,
    validate_position_s2c
);
trade_list_proto!(
    trd_get_order_list,
    "trd_get_order_list.rs",
    "GetOrderList",
    2201,
    validate_order_s2c
);
trade_list_proto!(
    trd_get_order_fill_list,
    "trd_get_order_fill_list.rs",
    "GetOrderFillList",
    2211,
    validate_fill_s2c
);
trade_list_proto!(
    trd_get_order_fee,
    "trd_get_order_fee.rs",
    "GetOrderFee",
    2225,
    validate_order_fee_s2c
);
trade_list_proto!(
    trd_get_margin_ratio,
    "trd_get_margin_ratio.rs",
    "GetMarginRatio",
    2223,
    validate_margin_ratio_s2c
);
pub mod trd_get_max_trd_qtys {
    use prost::Message;

    include!(concat!(env!("OUT_DIR"), "/trd_get_max_trd_qtys.rs"));

    pub const PROTOCOL_ID: u32 = 2111;

    pub fn encode_request(request: &Request) -> Vec<u8> {
        request.encode_to_vec()
    }

    pub fn decode_response(body: &[u8]) -> Result<S2c, super::ResponseError> {
        let response = Response::decode(body).map_err(|error| super::ResponseError::Decode {
            operation: "GetMaxTrdQtys",
            message: error.to_string(),
        })?;
        super::validate_response_for(
            response.ret_type,
            response.err_code,
            response.ret_msg.as_deref(),
            response.s2c.is_some(),
        )?;
        let payload = response.s2c.ok_or(super::ResponseError::MissingS2c)?;
        if payload.max_trd_qtys.is_none() {
            return Err(super::ResponseError::MissingMaxTradeQuantity);
        }
        super::validate_max_trade_quantity_s2c("GetMaxTrdQtys", &payload)?;
        Ok(payload)
    }
}

pub mod trd_get_combo_max_trd_qtys {
    use prost::Message;

    include!(concat!(env!("OUT_DIR"), "/trd_get_combo_max_trd_qtys.rs"));

    pub const PROTOCOL_ID: u32 = 2112;

    pub fn encode_request(request: &Request) -> Vec<u8> {
        request.encode_to_vec()
    }

    /// Decodes the combo buying-power response.
    ///
    /// Go `GetComboMaxTrdQtys` is deliberately permissive here: a zero
    /// retType with an absent `s2c`, or an `s2c` without `maxTrdQtys`, is an
    /// empty *success* rather than an error, because the preview only needs the
    /// delta fields when the venue supplies them. A missing `s2c` on a
    /// *rejected* response still fails through `validate_response_for`.
    /// Test seam for the combo payload validator; production callers decode a
    /// full response body through [`Self::decode_response`].
    pub fn validate_payload_for_test(payload: &S2c) -> Result<(), super::ResponseError> {
        super::validate_combo_max_trade_quantity_s2c(payload)
    }

    pub fn decode_response(body: &[u8]) -> Result<S2c, super::ResponseError> {
        let response = Response::decode(body).map_err(|error| super::ResponseError::Decode {
            operation: "GetComboMaxTrdQtys",
            message: error.to_string(),
        })?;
        if response.ret_type != 0 {
            return Err(super::ResponseError::ReturnCode {
                ret_type: response.ret_type,
                err_code: response.err_code.unwrap_or_default(),
                message: response.ret_msg.unwrap_or_default(),
            });
        }
        let payload = response.s2c.unwrap_or_default();
        super::validate_combo_max_trade_quantity_s2c(&payload)?;
        Ok(payload)
    }
}

fn validate_combo_max_trade_quantity_s2c(
    payload: &trd_get_combo_max_trd_qtys::S2c,
) -> Result<(), ResponseError> {
    let Some(maximum) = payload.max_trd_qtys.as_ref() else {
        return Ok(());
    };
    for (field, value) in [
        ("nlv_change", maximum.nlv_change),
        ("initial_margin_change", maximum.initial_margin_change),
        (
            "maintenance_margin_change",
            maximum.maintenance_margin_change,
        ),
        ("option_buy_power", maximum.option_buy_power),
        ("max_with_draw_change", maximum.max_with_draw_change),
        ("buy_power_decrease", maximum.buy_power_decrease),
    ] {
        if let Some(value) = value
            && !value.is_finite()
        {
            return Err(ValidationError::NonFinite {
                operation: "GetComboMaxTrdQtys",
                field: field.to_owned(),
            }
            .into());
        }
    }
    Ok(())
}

/// Adds the small amount of framing/response validation shared by OpenD trade
/// command protocols.  Command requests intentionally stay in this adapter;
/// the engine only sees the neutral request/result types from `trade_session`.
macro_rules! trade_command_proto {
    ($module:ident, $file:literal, $operation:literal, $protocol_id:literal) => {
        pub mod $module {
            use prost::Message;

            include!(concat!(env!("OUT_DIR"), "/", $file));

            pub const PROTOCOL_ID: u32 = $protocol_id;

            pub fn encode_request(request: &Request) -> Vec<u8> {
                request.encode_to_vec()
            }

            pub fn decode_response(body: &[u8]) -> Result<S2c, super::ResponseError> {
                let response =
                    Response::decode(body).map_err(|error| super::ResponseError::Decode {
                        operation: $operation,
                        message: error.to_string(),
                    })?;
                super::validate_response_for(
                    response.ret_type,
                    response.err_code,
                    response.ret_msg.as_deref(),
                    response.s2c.is_some(),
                )?;
                response.s2c.ok_or(super::ResponseError::MissingS2c)
            }
        }
    };
}

trade_command_proto!(trd_place_order, "trd_place_order.rs", "PlaceOrder", 2202);
pub mod trd_place_combo_order {
    use prost::Message;

    include!(concat!(env!("OUT_DIR"), "/trd_place_combo_order.rs"));

    pub const PROTOCOL_ID: u32 = 2227;

    pub fn encode_request(request: &Request) -> Vec<u8> {
        request.encode_to_vec()
    }

    /// Decodes the combo placement response.
    ///
    /// Go `PlaceComboOrder` returns an empty order id when the venue answered
    /// success without an `s2c`, and only fails on a non-zero retType; the
    /// strict `MissingS2c` rule would turn that documented empty success into
    /// a false failure.
    pub fn decode_response(body: &[u8]) -> Result<S2c, super::ResponseError> {
        let response = Response::decode(body).map_err(|error| super::ResponseError::Decode {
            operation: "PlaceComboOrder",
            message: error.to_string(),
        })?;
        if response.ret_type != 0 {
            return Err(super::ResponseError::ReturnCode {
                ret_type: response.ret_type,
                err_code: response.err_code.unwrap_or_default(),
                message: response.ret_msg.unwrap_or_default(),
            });
        }
        Ok(response.s2c.unwrap_or_default())
    }
}
trade_command_proto!(trd_modify_order, "trd_modify_order.rs", "ModifyOrder", 2205);
trade_command_proto!(trd_unlock_trade, "trd_unlock_trade.rs", "UnlockTrade", 2005);
trade_command_proto!(
    trd_sub_acc_push,
    "trd_sub_acc_push.rs",
    "SubscribeAccountPush",
    2008
);

/// Push-only trade notifications.  They have no request type, therefore the
/// modules only expose generated protobuf messages to the order-update worker.
pub mod trd_update_order {
    include!(concat!(env!("OUT_DIR"), "/trd_update_order.rs"));
}
pub mod trd_update_order_fill {
    include!(concat!(env!("OUT_DIR"), "/trd_update_order_fill.rs"));
}
pub mod trd_notify {
    include!(concat!(env!("OUT_DIR"), "/trd_notify.rs"));
}
pub mod notify {
    include!(concat!(env!("OUT_DIR"), "/notify.rs"));
}
pub mod keep_alive {
    include!(concat!(env!("OUT_DIR"), "/keep_alive.rs"));
}
pub mod get_user_info {
    include!(concat!(env!("OUT_DIR"), "/get_user_info.rs"));
}

#[cfg(test)]
#[path = "trade_proto_tests.rs"]
mod tests;
