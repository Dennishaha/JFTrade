//! Typed OpenD 0DTE contract reader (Qot_GetOptionZeroDteContract/3312).
use crate::{
    EventIndicator, EventIndicatorValue, OpenDSessionCoordinator, OpenDSessionCoordinatorError,
    OptionEventSecurity, OptionZeroDteChainInfo,
};
use prost::Message;
use serde::Serialize;
use std::sync::{Arc, Mutex};
use thiserror::Error;

#[derive(Clone, Debug, PartialEq)]
pub struct OptionZeroDteContractQuery {
    pub owner: OptionEventSecurity,
    pub strike_date_timestamp: i64,
    pub chain_info: OptionZeroDteChainInfo,
    pub sort_type: Option<i32>,
    pub is_asc: Option<bool>,
    pub filters: Vec<EventIndicator>,
}

impl OptionZeroDteContractQuery {
    /// Strict OpenD validation for the translated request; mirrors Go's
    /// `opend.ValidateAdvancedC2S` step for Qot_GetOptionZeroDteContract.
    pub fn validate(&self) -> Result<(), OptionZeroDteContractQueryError> {
        validate_query(self)
    }
}
#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionZeroDteContractItem {
    pub option: OptionEventSecurity,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub option_type: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub option_price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub change_rate: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub volume: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub open_interest: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub iv: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub delta: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub gamma: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub vega: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub theta: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub rho: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub buy_break_even_point: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub buy_to_bep: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub buy_profit_probability: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub sell_profit_probability: Option<f64>,
}
pub trait OptionZeroDteContractReadPort: Send + Sync + std::fmt::Debug {
    fn query(
        &self,
        query: &OptionZeroDteContractQuery,
    ) -> Result<Vec<OptionZeroDteContractItem>, OptionZeroDteContractQueryError>;
}
#[derive(Clone)]
pub struct OpenDOptionZeroDteContractReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}
impl std::fmt::Debug for OpenDOptionZeroDteContractReader {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("OpenDOptionZeroDteContractReader")
            .finish_non_exhaustive()
    }
}
impl OpenDOptionZeroDteContractReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}
impl OptionZeroDteContractReadPort for OpenDOptionZeroDteContractReader {
    fn query(
        &self,
        query: &OptionZeroDteContractQuery,
    ) -> Result<Vec<OptionZeroDteContractItem>, OptionZeroDteContractQueryError> {
        validate_query(query)?;
        let coordinator = self.coordinator.lock().map_err(|_| {
            OptionZeroDteContractQueryError::Session(OpenDSessionCoordinatorError::Closed)
        })?;
        let session = coordinator.session()?;
        let response = session
            .managed_session()
            .call(
                crate::trade_proto::qot_get_option_zero_dte_contract::PROTOCOL_ID,
                &encode_request(query)?,
            )
            .map_err(crate::OpenDSessionCoordinatorError::from)?;
        decode_response(&response)
    }
}
fn validate_query(
    query: &OptionZeroDteContractQuery,
) -> Result<(), OptionZeroDteContractQueryError> {
    if !query.owner.market.eq_ignore_ascii_case("US") || query.owner.code.trim().is_empty() {
        return Err(OptionZeroDteContractQueryError::InvalidQuery(
            "0DTE contract owner must be a US security".into(),
        ));
    }
    if query.strike_date_timestamp <= 0 {
        return Err(OptionZeroDteContractQueryError::InvalidQuery(
            "0DTE strikeDateTimestamp must be positive".into(),
        ));
    }
    if query
        .chain_info
        .product_code
        .as_deref()
        .unwrap_or("")
        .trim()
        .is_empty()
    {
        return Err(OptionZeroDteContractQueryError::InvalidQuery(
            "0DTE chainInfo.productCode is required".into(),
        ));
    }
    if let Some(sort) = query.sort_type
        && !(1..=4).contains(&sort)
    {
        return Err(OptionZeroDteContractQueryError::InvalidQuery(
            "0DTE contract sortType is unsupported".into(),
        ));
    }
    for filter in &query.filters {
        if !(1..=15).contains(&filter.indicator_type) {
            return Err(OptionZeroDteContractQueryError::InvalidQuery(
                "0DTE contract indicatorType is unsupported".into(),
            ));
        }
    }
    Ok(())
}
fn encode_request(
    query: &OptionZeroDteContractQuery,
) -> Result<Vec<u8>, OptionZeroDteContractQueryError> {
    use crate::trade_proto::qot_get_option_zero_dte_contract::{C2s, Request};
    let chain = crate::trade_proto::qot_get_option_zero_dte_screener::OptionChainInfo {
        strike_date_timestamp: query
            .chain_info
            .strike_date_timestamp
            .or(Some(query.strike_date_timestamp)),
        product_code: query.chain_info.product_code.clone(),
        multiplier: query.chain_info.multiplier,
        contract_share_size: query.chain_info.contract_share_size,
        expiration_type: query.chain_info.expiration_type,
        underlying: query
            .chain_info
            .underlying
            .as_ref()
            .map(encode_security)
            .transpose()?,
    };
    let filters = query
        .filters
        .iter()
        .map(encode_indicator)
        .collect::<Result<Vec<_>, _>>()?;
    Ok(Request {
        c2s: C2s {
            owner: encode_security(&query.owner)?,
            strike_date_timestamp: query.strike_date_timestamp,
            chain_info: chain,
            sort_type: query.sort_type,
            is_asc: query.is_asc,
            filter_list: filters,
        },
    }
    .encode_to_vec())
}
fn encode_indicator(
    indicator: &EventIndicator,
) -> Result<
    crate::trade_proto::qot_option_common::ZeroDteContractIndicator,
    OptionZeroDteContractQueryError,
> {
    Ok(
        crate::trade_proto::qot_option_common::ZeroDteContractIndicator {
            indicator_type: indicator.indicator_type,
            indicator_value: indicator.value.as_ref().map(encode_value).transpose()?,
        },
    )
}
fn encode_value(
    value: &EventIndicatorValue,
) -> Result<crate::trade_proto::qot_option_common::IndicatorValue, OptionZeroDteContractQueryError>
{
    Ok(crate::trade_proto::qot_option_common::IndicatorValue {
        value_list: value.value_list.clone(),
        value_interval: value.value_interval.as_ref().map(|interval| {
            crate::trade_proto::qot_option_common::Interval {
                filter_min: interval.filter_min.as_ref().map(|boundary| {
                    crate::trade_proto::qot_option_common::Boundary {
                        value: boundary.value,
                        includes: boundary.includes,
                    }
                }),
                filter_max: interval.filter_max.as_ref().map(|boundary| {
                    crate::trade_proto::qot_option_common::Boundary {
                        value: boundary.value,
                        includes: boundary.includes,
                    }
                }),
            }
        }),
        string_value_list: value.string_value_list.clone(),
        security_list: value
            .security_list
            .iter()
            .map(encode_security)
            .collect::<Result<Vec<_>, _>>()?,
    })
}
fn encode_security(
    security: &OptionEventSecurity,
) -> Result<crate::trade_proto::qot_common::Security, OptionZeroDteContractQueryError> {
    let market = match security.market.to_ascii_uppercase().as_str() {
        "US" => 11,
        _ => {
            return Err(OptionZeroDteContractQueryError::InvalidQuery(
                "0DTE contract security must be US".into(),
            ));
        }
    };
    Ok(crate::trade_proto::qot_common::Security {
        market,
        code: security.code.trim().to_ascii_uppercase(),
    })
}
fn decode_response(
    body: &[u8],
) -> Result<Vec<OptionZeroDteContractItem>, OptionZeroDteContractQueryError> {
    use crate::trade_proto::qot_get_option_zero_dte_contract::Response;
    let response = Response::decode(body).map_err(OptionZeroDteContractQueryError::Decode)?;
    if response.ret_type != 0 {
        return Err(OptionZeroDteContractQueryError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response
                .ret_msg
                .unwrap_or_else(|| "OpenD 0DTE contract request failed".into()),
        });
    }
    let Some(s2c) = response.s2c else {
        return Err(OptionZeroDteContractQueryError::MissingS2c);
    };
    s2c.item_list.into_iter().map(map_item).collect()
}
fn map_item(
    item: crate::trade_proto::qot_get_option_zero_dte_contract::ZeroDteContractItem,
) -> Result<OptionZeroDteContractItem, OptionZeroDteContractQueryError> {
    Ok(OptionZeroDteContractItem {
        option: map_security(item.option)?,
        name: clean(item.name),
        option_type: item.option_type,
        option_price: finite(item.option_price, "optionPrice")?,
        change_rate: finite(item.change_rate, "changeRate")?,
        volume: non_negative(item.volume, "volume")?,
        open_interest: non_negative(item.open_interest, "openInterest")?,
        iv: finite(item.iv, "iv")?,
        delta: finite(item.delta, "delta")?,
        gamma: finite(item.gamma, "gamma")?,
        vega: finite(item.vega, "vega")?,
        theta: finite(item.theta, "theta")?,
        rho: finite(item.rho, "rho")?,
        buy_break_even_point: finite(item.buy_break_even_point, "buyBreakEvenPoint")?,
        buy_to_bep: finite(item.buy_to_bep, "buyToBep")?,
        buy_profit_probability: finite(item.buy_profit_probability, "buyProfitProbability")?,
        sell_profit_probability: finite(item.sell_profit_probability, "sellProfitProbability")?,
    })
}
fn map_security(
    security: crate::trade_proto::qot_common::Security,
) -> Result<OptionEventSecurity, OptionZeroDteContractQueryError> {
    let code = security.code.trim().to_ascii_uppercase();
    if security.market != 11 || code.is_empty() {
        return Err(OptionZeroDteContractQueryError::InvalidResponse(
            "0DTE contract option security is invalid".into(),
        ));
    }
    Ok(OptionEventSecurity {
        market: "US".into(),
        code: code.clone(),
        quote_market: "US".into(),
        trade_market: "US".into(),
        instrument_id: format!("US.{code}"),
    })
}
fn clean(value: Option<String>) -> Option<String> {
    value.and_then(|value| (!value.trim().is_empty()).then(|| value.trim().to_owned()))
}
fn finite(value: Option<f64>, field: &str) -> Result<Option<f64>, OptionZeroDteContractQueryError> {
    if value.is_some_and(|v| !v.is_finite()) {
        return Err(OptionZeroDteContractQueryError::InvalidResponse(format!(
            "0DTE contract {field} must be finite"
        )));
    }
    Ok(value)
}
fn non_negative(
    value: Option<i64>,
    field: &str,
) -> Result<Option<i64>, OptionZeroDteContractQueryError> {
    if value.is_some_and(|v| v < 0) {
        return Err(OptionZeroDteContractQueryError::InvalidResponse(format!(
            "0DTE contract {field} must be non-negative"
        )));
    }
    Ok(value)
}
#[cfg(test)]
mod tests {
    use super::*;

    fn owner() -> OptionEventSecurity {
        OptionEventSecurity {
            market: "US".into(),
            code: "BABA".into(),
            quote_market: "US".into(),
            trade_market: "US".into(),
            instrument_id: "US.BABA".into(),
        }
    }

    fn query(
        chain_info: OptionZeroDteChainInfo,
        strike_date_timestamp: i64,
        sort_type: Option<i32>,
        filters: Vec<EventIndicator>,
    ) -> OptionZeroDteContractQuery {
        OptionZeroDteContractQuery {
            owner: owner(),
            strike_date_timestamp,
            chain_info,
            sort_type,
            is_asc: None,
            filters,
        }
    }

    /// Parity: go:452dea11:pkg/futu/adapter_option_fix_test.go:261
    /// TestFutuZeroDteContractTranslationRejectsEachInvalidBoundary
    #[test]
    fn strict_validation_rejects_each_invalid_zero_dte_contract_boundary() {
        let valid_chain = OptionZeroDteChainInfo {
            strike_date_timestamp: Some(1_784_332_800),
            product_code: Some("BABA".into()),
            multiplier: Some(100.0),
            contract_share_size: Some(100.0),
            expiration_type: Some(2),
            underlying: Some(owner()),
        };

        let mut hk = query(valid_chain.clone(), 1_784_332_800, None, Vec::new());
        hk.owner.market = "HK".into();
        assert!(hk.validate().is_err(), "HK 0DTE contract owner");

        let mut invalid_owner = query(valid_chain.clone(), 1_784_332_800, None, Vec::new());
        invalid_owner.owner.code = "  ".into();
        assert!(invalid_owner.validate().is_err(), "invalid owner");

        assert!(
            query(valid_chain.clone(), 0, None, Vec::new())
                .validate()
                .is_err(),
            "missing expiry"
        );

        let mut missing_code = valid_chain.clone();
        missing_code.product_code = Some(" ".into());
        assert!(
            query(missing_code, 1_784_332_800, None, Vec::new())
                .validate()
                .is_err(),
            "missing product code"
        );

        assert!(
            query(valid_chain.clone(), 1_784_332_800, Some(9), Vec::new())
                .validate()
                .is_err(),
            "unsupported sort"
        );

        assert!(
            query(
                valid_chain.clone(),
                1_784_332_800,
                None,
                vec![EventIndicator {
                    indicator_type: 99,
                    value: None,
                }]
            )
            .validate()
            .is_err(),
            "unsupported indicator"
        );

        query(valid_chain, 1_784_332_800, Some(2), Vec::new())
            .validate()
            .expect("the rebuilt broker-neutral chain context must validate");
    }
}

#[derive(Debug, Error)]
pub enum OptionZeroDteContractQueryError {
    #[error("invalid 0DTE contract query: {0}")]
    InvalidQuery(String),
    #[error("decode OpenD 0DTE contract response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD retType={ret_type} errCode={err_code}: {message}")]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD 0DTE contract response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD 0DTE contract response: {0}")]
    InvalidResponse(String),
    #[error("OpenD session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
}
