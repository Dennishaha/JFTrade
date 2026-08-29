//! Typed OpenD valuation-detail reader (`Qot_GetValuationDetail/3232`).
//!
//! The protobuf response is one aggregate object rather than a row-oriented
//! list. Generated messages stay private to this crate; the public DTOs below
//! are stable, broker-neutral projections for the engine and API layers.

use std::sync::{Arc, Mutex};

use prost::Message;
use serde::Serialize;
use thiserror::Error;

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct ValuationDetailQuery {
    pub market: i32,
    pub code: String,
    pub valuation_type: Option<i32>,
    pub interval_type: Option<i32>,
}

impl ValuationDetailQuery {
    pub fn validate(&self) -> Result<(), ValuationDetailQueryError> {
        validate_query(self)
    }
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ValuationDetailSecurity {
    pub market: String,
    pub code: String,
    pub instrument_id: String,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ValuationDetailHistoricalItem {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub value: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub time: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub time_str: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub plate_value: Option<f64>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ValuationDetailTrend {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub current_value: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub average_value: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub avg_minus1_stddev: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub avg_plus1_stddev: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub valuation_percentile: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub forward_value: Option<f64>,
    pub historical_items: Vec<ValuationDetailHistoricalItem>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ValuationDetailMarketDistributionSection {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub start: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub end: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub number: Option<i32>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ValuationDetailMarketDistribution {
    pub sections: Vec<ValuationDetailMarketDistributionSection>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub total: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ranking: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub average_value: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub median_value: Option<f64>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ValuationDetailPlateStockItem {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub security: Option<ValuationDetailSecurity>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub value: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub market_cap: Option<f64>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ValuationDetailPlateDistribution {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub plate: Option<ValuationDetailSecurity>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub plate_name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub plate_average_value: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub plate_ranking: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub plate_stock_item_count: Option<i32>,
    pub stock_items: Vec<ValuationDetailPlateStockItem>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ValuationDetailProfitGrowthItem {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub financial_year: Option<u32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub financial_quarter: Option<u32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub period_str: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub report_date: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub report_date_str: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub market_cap_multiple: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub finance_data_multiple: Option<f64>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ValuationDetailProfitGrowth {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub financial_ttm_multiple: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub market_cap_multiple: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub year_count: Option<i32>,
    pub profit_data: Vec<ValuationDetailProfitGrowthItem>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub conclusion_detailed: Option<String>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ValuationDetailSnapshot {
    pub security: ValuationDetailSecurity,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub valuation_type: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_update_time: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_update_time_str: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub trend: Option<ValuationDetailTrend>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub market_distribution: Option<ValuationDetailMarketDistribution>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub plate_distribution: Option<ValuationDetailPlateDistribution>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub profit_growth_rate: Option<ValuationDetailProfitGrowth>,
}

pub trait ValuationDetailReadPort: Send + Sync + std::fmt::Debug {
    fn query(
        &self,
        query: &ValuationDetailQuery,
    ) -> Result<ValuationDetailSnapshot, ValuationDetailQueryError>;
}

#[derive(Clone)]
pub struct OpenDValuationDetailReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDValuationDetailReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDValuationDetailReader")
            .finish_non_exhaustive()
    }
}

impl OpenDValuationDetailReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl ValuationDetailReadPort for OpenDValuationDetailReader {
    fn query(
        &self,
        query: &ValuationDetailQuery,
    ) -> Result<ValuationDetailSnapshot, ValuationDetailQueryError> {
        query.validate()?;
        let coordinator = self.coordinator.lock().map_err(|_| {
            ValuationDetailQueryError::Session(OpenDSessionCoordinatorError::Closed)
        })?;
        let session = coordinator.session()?;
        let response = session
            .managed_session()
            .call(
                crate::trade_proto::qot_get_valuation_detail::PROTOCOL_ID,
                &encode_request(query),
            )
            .map_err(crate::OpenDSessionCoordinatorError::from)?;
        decode_response(&response, query)
    }
}

fn validate_query(query: &ValuationDetailQuery) -> Result<(), ValuationDetailQueryError> {
    let market = market_label(query.market).ok_or_else(|| {
        ValuationDetailQueryError::InvalidQuery(
            "valuation detail market must be HK, US, SH, SZ, SG, JP, AU, MY, CA, FX, or crypto"
                .to_owned(),
        )
    })?;
    let code = query.code.trim();
    if code.is_empty()
        || code.len() > 128
        || code.chars().any(|value| {
            value.is_whitespace() || value.is_control() || matches!(value, '/' | '\\' | '?' | '#')
        })
    {
        return Err(ValuationDetailQueryError::InvalidQuery(format!(
            "valuation detail {market} code is invalid"
        )));
    }
    if let Some(value) = query.valuation_type
        && !(0..=3).contains(&value)
    {
        return Err(ValuationDetailQueryError::InvalidQuery(
            "valuationType must be 0, 1, 2, or 3".to_owned(),
        ));
    }
    if let Some(value) = query.interval_type
        && !(0..=10).contains(&value)
    {
        return Err(ValuationDetailQueryError::InvalidQuery(
            "intervalType must be between 0 and 10".to_owned(),
        ));
    }
    Ok(())
}

fn encode_request(query: &ValuationDetailQuery) -> Vec<u8> {
    use crate::trade_proto::qot_common::Security;
    use crate::trade_proto::qot_get_valuation_detail::{C2s, Request};
    Request {
        c2s: C2s {
            security: Security {
                market: query.market,
                code: query.code.trim().to_ascii_uppercase(),
            },
            valuation_type: query.valuation_type,
            interval_type: query.interval_type,
        },
    }
    .encode_to_vec()
}

fn decode_response(
    body: &[u8],
    query: &ValuationDetailQuery,
) -> Result<ValuationDetailSnapshot, ValuationDetailQueryError> {
    use crate::trade_proto::qot_get_valuation_detail::Response;
    let response = Response::decode(body).map_err(ValuationDetailQueryError::Decode)?;
    if response.ret_type != 0 {
        return Err(ValuationDetailQueryError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response
                .ret_msg
                .unwrap_or_else(|| "OpenD valuation detail request failed".to_owned()),
        });
    }
    let Some(s2c) = response.s2c else {
        return Err(ValuationDetailQueryError::MissingS2c);
    };
    if let Some(value) = s2c.valuation_type
        && !(0..=3).contains(&value)
    {
        return Err(ValuationDetailQueryError::InvalidResponse(
            "valuation detail response valuationType is unsupported".to_owned(),
        ));
    }
    let security = security_from_query(query);
    let trend = s2c.trend.map(map_trend).transpose()?;
    let market_distribution = s2c
        .market_distribution
        .map(map_market_distribution)
        .transpose()?;
    let plate_distribution = s2c
        .plate_distribution
        .map(map_plate_distribution)
        .transpose()?;
    let profit_growth_rate = s2c.profit_growth_rate.map(map_profit_growth).transpose()?;
    Ok(ValuationDetailSnapshot {
        security,
        valuation_type: s2c.valuation_type,
        last_update_time: s2c.last_update_time,
        last_update_time_str: optional_text(s2c.last_update_time_str),
        trend,
        market_distribution,
        plate_distribution,
        profit_growth_rate,
    })
}

fn security_from_query(query: &ValuationDetailQuery) -> ValuationDetailSecurity {
    let market = market_label(query.market).expect("query validation ensures market");
    let code = query.code.trim().to_ascii_uppercase();
    ValuationDetailSecurity {
        market: market.to_owned(),
        instrument_id: format!("{market}.{code}"),
        code,
    }
}

fn map_security(
    value: crate::trade_proto::qot_common::Security,
    field: &str,
) -> Result<ValuationDetailSecurity, ValuationDetailQueryError> {
    let market = market_label(value.market).ok_or_else(|| {
        ValuationDetailQueryError::InvalidResponse(format!(
            "valuation detail {field} has unsupported market"
        ))
    })?;
    let code = value.code.trim();
    if code.is_empty() || code.chars().any(char::is_whitespace) {
        return Err(ValuationDetailQueryError::InvalidResponse(format!(
            "valuation detail {field} code is empty or invalid"
        )));
    }
    let code = code.to_ascii_uppercase();
    Ok(ValuationDetailSecurity {
        market: market.to_owned(),
        instrument_id: format!("{market}.{code}"),
        code,
    })
}

fn map_trend(
    value: crate::trade_proto::qot_get_valuation_detail::ValuationTrend,
) -> Result<ValuationDetailTrend, ValuationDetailQueryError> {
    validate_fields([
        ("currentValue", value.current_value),
        ("averageValue", value.average_value),
        ("avgMinus1Stddev", value.avg_minus1_stddev),
        ("avgPlus1Stddev", value.avg_plus1_stddev),
        ("valuationPercentile", value.valuation_percentile),
        ("forwardValue", value.forward_value),
    ])?;
    let historical_items = value
        .historical_items
        .into_iter()
        .map(map_historical_item)
        .collect::<Result<Vec<_>, _>>()?;
    Ok(ValuationDetailTrend {
        current_value: value.current_value,
        average_value: value.average_value,
        avg_minus1_stddev: value.avg_minus1_stddev,
        avg_plus1_stddev: value.avg_plus1_stddev,
        valuation_percentile: value.valuation_percentile,
        forward_value: value.forward_value,
        historical_items,
    })
}

fn map_historical_item(
    value: crate::trade_proto::qot_get_valuation_detail::valuation_trend::ValuationHistoricalItem,
) -> Result<ValuationDetailHistoricalItem, ValuationDetailQueryError> {
    validate_fields([("value", value.value), ("plateValue", value.plate_value)])?;
    Ok(ValuationDetailHistoricalItem {
        value: value.value,
        time: value.time,
        time_str: optional_text(value.time_str),
        plate_value: value.plate_value,
    })
}

fn map_market_distribution(
    value: crate::trade_proto::qot_get_valuation_detail::MarketDistribution,
) -> Result<ValuationDetailMarketDistribution, ValuationDetailQueryError> {
    validate_fields([
        ("averageValue", value.average_value),
        ("medianValue", value.median_value),
    ])?;
    let sections = value
        .sections
        .into_iter()
        .map(|section| {
            validate_fields([("start", section.start), ("end", section.end)])?;
            validate_non_negative(section.number, "number")?;
            Ok(ValuationDetailMarketDistributionSection {
                start: section.start,
                end: section.end,
                number: section.number,
            })
        })
        .collect::<Result<Vec<_>, ValuationDetailQueryError>>()?;
    validate_non_negative(value.total, "total")?;
    validate_non_negative(value.ranking, "ranking")?;
    Ok(ValuationDetailMarketDistribution {
        sections,
        total: value.total,
        ranking: value.ranking,
        average_value: value.average_value,
        median_value: value.median_value,
    })
}

fn map_plate_distribution(
    value: crate::trade_proto::qot_get_valuation_detail::PlateDistribution,
) -> Result<ValuationDetailPlateDistribution, ValuationDetailQueryError> {
    validate_fields([("plateAverageValue", value.plate_average_value)])?;
    validate_non_negative(value.plate_ranking, "plateRanking")?;
    validate_non_negative(value.plate_stock_item_count, "plateStockItemCount")?;
    let plate = value
        .plate
        .map(|security| map_security(security, "plate"))
        .transpose()?;
    let stock_items = value
        .stock_items
        .into_iter()
        .map(|item| {
            validate_fields([("value", item.value), ("marketCap", item.market_cap)])?;
            let security = item
                .security
                .map(|value| map_security(value, "plate stock item security"))
                .transpose()?;
            Ok(ValuationDetailPlateStockItem {
                security,
                name: optional_text(item.name),
                value: item.value,
                market_cap: item.market_cap,
            })
        })
        .collect::<Result<Vec<_>, ValuationDetailQueryError>>()?;
    Ok(ValuationDetailPlateDistribution {
        plate,
        plate_name: optional_text(value.plate_name),
        plate_average_value: value.plate_average_value,
        plate_ranking: value.plate_ranking,
        plate_stock_item_count: value.plate_stock_item_count,
        stock_items,
    })
}

fn map_profit_growth(
    value: crate::trade_proto::qot_get_valuation_detail::ProfitGrowthRate,
) -> Result<ValuationDetailProfitGrowth, ValuationDetailQueryError> {
    validate_fields([
        ("financialTtmMultiple", value.financial_ttm_multiple),
        ("marketCapMultiple", value.market_cap_multiple),
    ])?;
    validate_non_negative(value.year_count, "yearCount")?;
    let profit_data = value
        .profit_data
        .into_iter()
        .map(|item| {
            validate_fields([
                ("marketCapMultiple", item.market_cap_multiple),
                ("financeDataMultiple", item.finance_data_multiple),
            ])?;
            if item.report_date.is_some_and(|date| date < 0) {
                return Err(ValuationDetailQueryError::InvalidResponse(
                    "valuation detail reportDate must be non-negative".to_owned(),
                ));
            }
            Ok(ValuationDetailProfitGrowthItem {
                financial_year: item.financial_year,
                financial_quarter: item.financial_quarter,
                period_str: optional_text(item.period_str),
                report_date: item.report_date,
                report_date_str: optional_text(item.report_date_str),
                market_cap_multiple: item.market_cap_multiple,
                finance_data_multiple: item.finance_data_multiple,
            })
        })
        .collect::<Result<Vec<_>, ValuationDetailQueryError>>()?;
    Ok(ValuationDetailProfitGrowth {
        financial_ttm_multiple: value.financial_ttm_multiple,
        market_cap_multiple: value.market_cap_multiple,
        year_count: value.year_count,
        profit_data,
        conclusion_detailed: optional_text(value.conclusion_detailed),
    })
}

fn validate_fields<const N: usize>(
    values: [(&str, Option<f64>); N],
) -> Result<(), ValuationDetailQueryError> {
    for (field, value) in values {
        if value.is_some_and(|number| !number.is_finite()) {
            return Err(ValuationDetailQueryError::InvalidResponse(format!(
                "valuation detail {field} must be finite"
            )));
        }
    }
    Ok(())
}

fn validate_non_negative<T>(value: Option<T>, field: &str) -> Result<(), ValuationDetailQueryError>
where
    T: PartialOrd + Default,
{
    if value.is_some_and(|number| number < T::default()) {
        return Err(ValuationDetailQueryError::InvalidResponse(format!(
            "valuation detail {field} must be non-negative"
        )));
    }
    Ok(())
}

fn optional_text(value: Option<String>) -> Option<String> {
    value
        .map(|text| text.trim().to_owned())
        .filter(|text| !text.is_empty())
}

fn market_label(market: i32) -> Option<&'static str> {
    match market {
        1 => Some("HK"),
        11 => Some("US"),
        21 => Some("SH"),
        22 => Some("SZ"),
        31 => Some("SG"),
        41 => Some("JP"),
        51 => Some("AU"),
        61 => Some("MY"),
        71 => Some("CA"),
        81 => Some("FX"),
        91 => Some("CRYPTO"),
        _ => None,
    }
}

#[derive(Debug, Error)]
pub enum ValuationDetailQueryError {
    #[error("invalid OpenD valuation detail query: {0}")]
    InvalidQuery(String),
    #[error("OpenD valuation detail session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("decode OpenD Qot_GetValuationDetail response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD Qot_GetValuationDetail retType={ret_type} errCode={err_code}: {message}")]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD Qot_GetValuationDetail response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD valuation detail response: {0}")]
    InvalidResponse(String),
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::trade_proto::qot_common::Security;
    use crate::trade_proto::qot_get_valuation_detail::{
        MarketDistribution, PlateDistribution, ProfitGrowthRate, Response, S2c, ValuationTrend,
    };

    fn query() -> ValuationDetailQuery {
        ValuationDetailQuery {
            market: 11,
            code: "aapl".to_owned(),
            valuation_type: Some(1),
            interval_type: Some(3),
        }
    }

    #[test]
    fn query_validation_rejects_invalid_scope_and_enums() {
        let mut invalid = query();
        invalid.market = 999;
        assert!(matches!(
            invalid.validate(),
            Err(ValuationDetailQueryError::InvalidQuery(_))
        ));
        let mut invalid = query();
        invalid.code = "AAPL/evil".to_owned();
        assert!(invalid.validate().is_err());
        let mut invalid = query();
        invalid.valuation_type = Some(4);
        assert!(invalid.validate().is_err());
        let mut invalid = query();
        invalid.interval_type = Some(11);
        assert!(invalid.validate().is_err());
    }

    #[test]
    fn response_maps_aggregate_fields_and_nested_security() {
        let body = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                valuation_type: Some(1),
                last_update_time: Some(1_700_000_000),
                last_update_time_str: Some(" 2026-08-29 12:00:00 ".to_owned()),
                trend: Some(ValuationTrend {
                    current_value: Some(25.5),
                    average_value: Some(20.0),
                    avg_minus1_stddev: None,
                    avg_plus1_stddev: None,
                    valuation_percentile: Some(65.0),
                    forward_value: Some(22.0),
                    historical_items: Vec::new(),
                }),
                market_distribution: Some(MarketDistribution {
                    sections: Vec::new(),
                    total: Some(100),
                    ranking: Some(7),
                    average_value: Some(18.0),
                    median_value: Some(16.0),
                }),
                plate_distribution: Some(PlateDistribution {
                    plate: Some(Security {
                        market: 11,
                        code: "XLK".to_owned(),
                    }),
                    plate_name: Some("Technology".to_owned()),
                    plate_average_value: Some(24.0),
                    plate_ranking: Some(3),
                    plate_stock_item_count: Some(50),
                    stock_items: Vec::new(),
                }),
                profit_growth_rate: Some(ProfitGrowthRate {
                    financial_ttm_multiple: Some(1.2),
                    market_cap_multiple: Some(1.1),
                    year_count: Some(3),
                    profit_data: Vec::new(),
                    conclusion_detailed: Some("Growth".to_owned()),
                }),
            }),
        }
        .encode_to_vec();
        let snapshot = decode_response(&body, &query()).expect("valuation response");
        assert_eq!(snapshot.security.instrument_id, "US.AAPL");
        assert_eq!(
            snapshot.last_update_time_str.as_deref(),
            Some("2026-08-29 12:00:00")
        );
        assert_eq!(
            snapshot
                .trend
                .as_ref()
                .and_then(|trend| trend.current_value),
            Some(25.5)
        );
        assert_eq!(
            snapshot
                .plate_distribution
                .as_ref()
                .and_then(|plate| plate.plate.as_ref())
                .map(|security| security.instrument_id.as_str()),
            Some("US.XLK")
        );
    }

    #[test]
    fn response_rejects_non_finite_values_and_missing_s2c() {
        let missing = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: None,
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&missing, &query()),
            Err(ValuationDetailQueryError::MissingS2c)
        ));
        let invalid = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                valuation_type: None,
                last_update_time: None,
                last_update_time_str: None,
                trend: Some(ValuationTrend {
                    current_value: Some(f64::NAN),
                    average_value: None,
                    avg_minus1_stddev: None,
                    avg_plus1_stddev: None,
                    valuation_percentile: None,
                    forward_value: None,
                    historical_items: Vec::new(),
                }),
                market_distribution: None,
                plate_distribution: None,
                profit_growth_rate: None,
            }),
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&invalid, &query()),
            Err(ValuationDetailQueryError::InvalidResponse(_))
        ));
    }

    #[test]
    fn response_rejection_preserves_return_details() {
        let body = Response {
            ret_type: 3,
            ret_msg: Some("permission denied".to_owned()),
            err_code: Some(42),
            s2c: None,
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&body, &query()),
            Err(ValuationDetailQueryError::Rejected {
                ret_type: 3,
                err_code: 42,
                ref message,
            }) if message == "permission denied"
        ));
    }
}
