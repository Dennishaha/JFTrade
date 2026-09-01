//! Typed OpenD institution and ARK research readers.
//!
//! The eight institution protocols (3418-3425) use several wire shapes.  The
//! public projection below keeps those shapes explicit without leaking the
//! generated protobuf modules to engine consumers.

use std::sync::{Arc, Mutex};

use prost::Message;
use serde::Serialize;
use thiserror::Error;

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

#[path = "research_institutions_projection.rs"]
mod projection;
use projection::{
    base_result, ensure_success, map_ark_holding, map_ark_transaction, map_distribution,
    map_dynamic, map_holding, map_holding_change, map_institution_list, map_profile,
    normalize_next_page, optional_text,
};

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub enum InstitutionOperation {
    #[default]
    List,
    Profile,
    Distribution,
    HoldingChanges,
    Holdings,
    ArkFundHoldings,
    ArkStockActivity,
    ArkTransactions,
}

impl InstitutionOperation {
    fn as_str(self) -> &'static str {
        match self {
            Self::List => "list",
            Self::Profile => "profile",
            Self::Distribution => "distribution",
            Self::HoldingChanges => "holding_changes",
            Self::Holdings => "holdings",
            Self::ArkFundHoldings => "ark_fund_holdings",
            Self::ArkStockActivity => "ark_stock_activity",
            Self::ArkTransactions => "ark_transactions",
        }
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct InstitutionSecurity {
    pub market: String,
    pub code: String,
    pub instrument_id: String,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct InstitutionSecurityQuery {
    pub market: i32,
    pub code: String,
}

#[derive(Clone, Debug, PartialEq)]
pub struct InstitutionQuery {
    pub operation: InstitutionOperation,
    /// Required for regular institution operations; ignored for ARK queries.
    pub market: i32,
    pub institution_id: Option<i32>,
    pub security: Option<InstitutionSecurityQuery>,
    pub change_type: Option<i32>,
    pub holding_type: Option<i32>,
    pub cycle_type: Option<i32>,
    pub sort_field: Option<i32>,
    pub sort_dir: Option<i32>,
    pub count: Option<i32>,
    pub page: Option<String>,
    pub name_part: Option<String>,
    pub keyword: Option<String>,
}

impl Default for InstitutionQuery {
    fn default() -> Self {
        Self {
            operation: InstitutionOperation::List,
            market: 11,
            institution_id: None,
            security: None,
            change_type: None,
            holding_type: None,
            cycle_type: None,
            sort_field: None,
            sort_dir: None,
            count: Some(20),
            page: None,
            name_part: None,
            keyword: None,
        }
    }
}

impl InstitutionQuery {
    pub fn validate(&self) -> Result<(), InstitutionQueryError> {
        match self.operation {
            InstitutionOperation::List => validate_market(self.market)?,
            InstitutionOperation::Profile
            | InstitutionOperation::Distribution
            | InstitutionOperation::HoldingChanges
            | InstitutionOperation::Holdings => {
                validate_market(self.market)?;
                validate_institution_id(self.institution_id)?;
            }
            InstitutionOperation::ArkStockActivity => {
                let security = self.security.as_ref().ok_or_else(|| {
                    InstitutionQueryError::InvalidQuery(
                        "ARK stock activity requires a security".to_owned(),
                    )
                })?;
                if security.market != 11 {
                    return Err(InstitutionQueryError::InvalidQuery(
                        "ARK stock activity supports US securities only".to_owned(),
                    ));
                }
                validate_security(security)?;
            }
            InstitutionOperation::ArkFundHoldings | InstitutionOperation::ArkTransactions => {}
        }
        if let Some(count) = self.count
            && !(1..=200).contains(&count)
        {
            return Err(InstitutionQueryError::InvalidQuery(
                "institution count must be between 1 and 200".to_owned(),
            ));
        }
        if self.sort_dir.is_some_and(|value| !matches!(value, 0 | 1)) {
            return Err(InstitutionQueryError::InvalidQuery(
                "institution sortDir must be 0 or 1".to_owned(),
            ));
        }
        if self
            .page
            .as_deref()
            .is_some_and(|value| value.len() > 256 || value.chars().any(char::is_control))
        {
            return Err(InstitutionQueryError::InvalidQuery(
                "institution page is invalid".to_owned(),
            ));
        }
        for (field, value) in [
            ("namePart", self.name_part.as_deref()),
            ("keyword", self.keyword.as_deref()),
        ] {
            if value.is_some_and(|value| value.len() > 128 || value.chars().any(char::is_control)) {
                return Err(InstitutionQueryError::InvalidQuery(format!(
                    "institution {field} is invalid"
                )));
            }
        }
        if self
            .change_type
            .is_some_and(|value| !(0..=4).contains(&value))
        {
            return Err(InstitutionQueryError::InvalidQuery(
                "institution changeType must be between 0 and 4".to_owned(),
            ));
        }
        if self
            .holding_type
            .is_some_and(|value| !(0..=4).contains(&value))
        {
            return Err(InstitutionQueryError::InvalidQuery(
                "institution holdingType must be between 0 and 4".to_owned(),
            ));
        }
        if self
            .cycle_type
            .is_some_and(|value| !(0..=4).contains(&value))
        {
            return Err(InstitutionQueryError::InvalidQuery(
                "institution cycleType must be between 0 and 4".to_owned(),
            ));
        }
        Ok(())
    }
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct InstitutionEntry {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub security: Option<InstitutionSecurity>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub institution_id: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub industry_name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub market_value: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub market_value_change: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub holding_count: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub holding_count_change: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub holding_value: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub holding_pct: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_holding_pct: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub portfolio_pct: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub change_shares: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub change_pct: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub shares: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub shares_change: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub weight: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub weight_change: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub change_amount: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub holding_date: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub disclosure_date: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub source: Option<String>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct InstitutionSummary {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub institution_name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub position_value: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_position_value: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub position_value_change_pct: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub total_holding_count: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub holding_change_count: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub new_count: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub sold_out_count: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub increase_count: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub decrease_count: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub top10_pct: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub top10_pct_change: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub disclosure_date: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub currency: Option<String>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct InstitutionDistribution {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub industry_id: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub industry_name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub position_value: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub portfolio_pct: Option<f64>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct InstitutionDynamic {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub dynamic_type: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub transaction_count: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub net_shares: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_transaction_time: Option<String>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct InstitutionResult {
    pub operation: String,
    pub entries: Vec<InstitutionEntry>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub summary: Option<InstitutionSummary>,
    pub distribution: Vec<InstitutionDistribution>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub dynamic: Option<InstitutionDynamic>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub all_count: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub next_page: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub currency: Option<String>,
}

pub trait InstitutionReadPort: Send + Sync + std::fmt::Debug {
    fn query(&self, query: &InstitutionQuery) -> Result<InstitutionResult, InstitutionQueryError>;
}

#[derive(Clone)]
pub struct OpenDInstitutionReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDInstitutionReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDInstitutionReader")
            .finish_non_exhaustive()
    }
}

impl OpenDInstitutionReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl InstitutionReadPort for OpenDInstitutionReader {
    fn query(&self, query: &InstitutionQuery) -> Result<InstitutionResult, InstitutionQueryError> {
        query.validate()?;
        let coordinator = self
            .coordinator
            .lock()
            .map_err(|_| InstitutionQueryError::Session(OpenDSessionCoordinatorError::Closed))?;
        let session = coordinator.session()?;
        let body = match query.operation {
            InstitutionOperation::List => session
                .managed_session()
                .call(
                    crate::trade_proto::qot_get_institution_list::PROTOCOL_ID,
                    &encode_request(query),
                )
                .map_err(OpenDSessionCoordinatorError::from)?,
            InstitutionOperation::Profile => session
                .managed_session()
                .call(
                    crate::trade_proto::qot_get_institution_profile::PROTOCOL_ID,
                    &encode_request(query),
                )
                .map_err(OpenDSessionCoordinatorError::from)?,
            InstitutionOperation::Distribution => session
                .managed_session()
                .call(
                    crate::trade_proto::qot_get_institution_distribution::PROTOCOL_ID,
                    &encode_request(query),
                )
                .map_err(OpenDSessionCoordinatorError::from)?,
            InstitutionOperation::HoldingChanges => session
                .managed_session()
                .call(
                    crate::trade_proto::qot_get_institution_holding_change::PROTOCOL_ID,
                    &encode_request(query),
                )
                .map_err(OpenDSessionCoordinatorError::from)?,
            InstitutionOperation::Holdings => session
                .managed_session()
                .call(
                    crate::trade_proto::qot_get_institution_holding_list::PROTOCOL_ID,
                    &encode_request(query),
                )
                .map_err(OpenDSessionCoordinatorError::from)?,
            InstitutionOperation::ArkFundHoldings => session
                .managed_session()
                .call(
                    crate::trade_proto::qot_get_ark_fund_holding::PROTOCOL_ID,
                    &encode_request(query),
                )
                .map_err(OpenDSessionCoordinatorError::from)?,
            InstitutionOperation::ArkStockActivity => session
                .managed_session()
                .call(
                    crate::trade_proto::qot_get_ark_stock_dynamic::PROTOCOL_ID,
                    &encode_request(query),
                )
                .map_err(OpenDSessionCoordinatorError::from)?,
            InstitutionOperation::ArkTransactions => session
                .managed_session()
                .call(
                    crate::trade_proto::qot_get_ark_active_transaction::PROTOCOL_ID,
                    &encode_request(query),
                )
                .map_err(OpenDSessionCoordinatorError::from)?,
        };
        decode_response(query, &body)
    }
}

fn encode_request(query: &InstitutionQuery) -> Vec<u8> {
    match query.operation {
        InstitutionOperation::List => {
            use crate::trade_proto::qot_get_institution_list::{C2s, Request};
            Request {
                c2s: C2s {
                    market: query.market,
                    sort_field: query.sort_field,
                    sort_dir: query.sort_dir,
                    count: query.count,
                    page: query.page.clone(),
                    name_part: optional_text(query.name_part.clone()),
                },
            }
            .encode_to_vec()
        }
        InstitutionOperation::Profile => {
            use crate::trade_proto::qot_get_institution_profile::{C2s, Request};
            Request {
                c2s: C2s {
                    market: query.market,
                    institution_id: query.institution_id.unwrap_or_default(),
                },
            }
            .encode_to_vec()
        }
        InstitutionOperation::Distribution => {
            use crate::trade_proto::qot_get_institution_distribution::{C2s, Request};
            Request {
                c2s: C2s {
                    market: query.market,
                    institution_id: query.institution_id.unwrap_or_default(),
                },
            }
            .encode_to_vec()
        }
        InstitutionOperation::HoldingChanges => {
            use crate::trade_proto::qot_get_institution_holding_change::{C2s, Request};
            Request {
                c2s: C2s {
                    market: query.market,
                    institution_id: query.institution_id.unwrap_or_default(),
                    change_type: query.change_type,
                    sort_field: query.sort_field,
                    sort_dir: query.sort_dir,
                    count: query.count,
                    page: query.page.clone(),
                },
            }
            .encode_to_vec()
        }
        InstitutionOperation::Holdings => {
            use crate::trade_proto::qot_get_institution_holding_list::{C2s, Request};
            Request {
                c2s: C2s {
                    market: query.market,
                    institution_id: query.institution_id.unwrap_or_default(),
                    change_type: query.change_type,
                    sort_field: query.sort_field,
                    sort_dir: query.sort_dir,
                    count: query.count,
                    page: query.page.clone(),
                    keyword: optional_text(query.keyword.clone()),
                },
            }
            .encode_to_vec()
        }
        InstitutionOperation::ArkFundHoldings => {
            use crate::trade_proto::qot_get_ark_fund_holding::{C2s, Request};
            Request {
                c2s: C2s {
                    holding_type: query.holding_type,
                    cycle_type: query.cycle_type,
                    sort_field: query.sort_field,
                    sort_dir: query.sort_dir,
                    count: query.count,
                    page: query.page.clone(),
                },
            }
            .encode_to_vec()
        }
        InstitutionOperation::ArkStockActivity => {
            use crate::trade_proto::qot_get_ark_stock_dynamic::{C2s, Request};
            let security = query
                .security
                .as_ref()
                .expect("query validation ensures security");
            Request {
                c2s: C2s {
                    security: crate::trade_proto::qot_common::Security {
                        market: security.market,
                        code: security.code.trim().to_ascii_uppercase(),
                    },
                },
            }
            .encode_to_vec()
        }
        InstitutionOperation::ArkTransactions => {
            use crate::trade_proto::qot_get_ark_active_transaction::{C2s, Request};
            Request {
                c2s: C2s {
                    holding_type: query.holding_type,
                    cycle_type: query.cycle_type,
                    sort_field: query.sort_field,
                    sort_dir: query.sort_dir,
                    count: query.count,
                    page: query.page.clone(),
                },
            }
            .encode_to_vec()
        }
    }
}

fn decode_response(
    query: &InstitutionQuery,
    body: &[u8],
) -> Result<InstitutionResult, InstitutionQueryError> {
    match query.operation {
        InstitutionOperation::List => {
            use crate::trade_proto::qot_get_institution_list::Response;
            let response = Response::decode(body).map_err(InstitutionQueryError::Decode)?;
            ensure_success(
                response.ret_type,
                response.err_code,
                response.ret_msg,
                query.operation,
            )?;
            let s2c = response.s2c.ok_or(InstitutionQueryError::MissingS2c)?;
            let entries = s2c
                .data_list
                .into_iter()
                .map(map_institution_list)
                .collect::<Result<Vec<_>, _>>()?;
            Ok(base_result(
                query,
                entries,
                normalize_next_page(s2c.next_page),
                s2c.all_count,
                s2c.currency,
            ))
        }
        InstitutionOperation::Profile => {
            use crate::trade_proto::qot_get_institution_profile::Response;
            let response = Response::decode(body).map_err(InstitutionQueryError::Decode)?;
            ensure_success(
                response.ret_type,
                response.err_code,
                response.ret_msg,
                query.operation,
            )?;
            let s2c = response.s2c.ok_or(InstitutionQueryError::MissingS2c)?;
            Ok(InstitutionResult {
                operation: query.operation.as_str().to_owned(),
                entries: Vec::new(),
                summary: Some(map_profile(s2c)?),
                distribution: Vec::new(),
                dynamic: None,
                all_count: None,
                next_page: None,
                currency: None,
            })
        }
        InstitutionOperation::Distribution => {
            use crate::trade_proto::qot_get_institution_distribution::Response;
            let response = Response::decode(body).map_err(InstitutionQueryError::Decode)?;
            ensure_success(
                response.ret_type,
                response.err_code,
                response.ret_msg,
                query.operation,
            )?;
            let s2c = response.s2c.ok_or(InstitutionQueryError::MissingS2c)?;
            let distribution = s2c
                .data_list
                .into_iter()
                .map(map_distribution)
                .collect::<Result<Vec<_>, _>>()?;
            Ok(InstitutionResult {
                operation: query.operation.as_str().to_owned(),
                entries: Vec::new(),
                summary: None,
                distribution,
                dynamic: None,
                all_count: None,
                next_page: None,
                currency: None,
            })
        }
        InstitutionOperation::HoldingChanges => {
            use crate::trade_proto::qot_get_institution_holding_change::Response;
            let response = Response::decode(body).map_err(InstitutionQueryError::Decode)?;
            ensure_success(
                response.ret_type,
                response.err_code,
                response.ret_msg,
                query.operation,
            )?;
            let s2c = response.s2c.ok_or(InstitutionQueryError::MissingS2c)?;
            let entries = s2c
                .data_list
                .into_iter()
                .map(map_holding_change)
                .collect::<Result<Vec<_>, _>>()?;
            Ok(base_result(
                query,
                entries,
                normalize_next_page(s2c.next_page),
                s2c.all_count,
                None,
            ))
        }
        InstitutionOperation::Holdings => {
            use crate::trade_proto::qot_get_institution_holding_list::Response;
            let response = Response::decode(body).map_err(InstitutionQueryError::Decode)?;
            ensure_success(
                response.ret_type,
                response.err_code,
                response.ret_msg,
                query.operation,
            )?;
            let s2c = response.s2c.ok_or(InstitutionQueryError::MissingS2c)?;
            let entries = s2c
                .data_list
                .into_iter()
                .map(map_holding)
                .collect::<Result<Vec<_>, _>>()?;
            Ok(base_result(
                query,
                entries,
                normalize_next_page(s2c.next_page),
                s2c.all_count,
                s2c.currency,
            ))
        }
        InstitutionOperation::ArkFundHoldings => {
            use crate::trade_proto::qot_get_ark_fund_holding::Response;
            let response = Response::decode(body).map_err(InstitutionQueryError::Decode)?;
            ensure_success(
                response.ret_type,
                response.err_code,
                response.ret_msg,
                query.operation,
            )?;
            let s2c = response.s2c.ok_or(InstitutionQueryError::MissingS2c)?;
            let entries = s2c
                .data_list
                .into_iter()
                .map(map_ark_holding)
                .collect::<Result<Vec<_>, _>>()?;
            Ok(base_result(
                query,
                entries,
                normalize_next_page(s2c.next_page),
                s2c.all_count,
                None,
            ))
        }
        InstitutionOperation::ArkStockActivity => {
            use crate::trade_proto::qot_get_ark_stock_dynamic::Response;
            let response = Response::decode(body).map_err(InstitutionQueryError::Decode)?;
            ensure_success(
                response.ret_type,
                response.err_code,
                response.ret_msg,
                query.operation,
            )?;
            let s2c = response.s2c.ok_or(InstitutionQueryError::MissingS2c)?;
            Ok(InstitutionResult {
                operation: query.operation.as_str().to_owned(),
                entries: Vec::new(),
                summary: None,
                distribution: Vec::new(),
                dynamic: Some(map_dynamic(s2c)?),
                all_count: None,
                next_page: None,
                currency: None,
            })
        }
        InstitutionOperation::ArkTransactions => {
            use crate::trade_proto::qot_get_ark_active_transaction::Response;
            let response = Response::decode(body).map_err(InstitutionQueryError::Decode)?;
            ensure_success(
                response.ret_type,
                response.err_code,
                response.ret_msg,
                query.operation,
            )?;
            let s2c = response.s2c.ok_or(InstitutionQueryError::MissingS2c)?;
            let entries = s2c
                .data_list
                .into_iter()
                .map(map_ark_transaction)
                .collect::<Result<Vec<_>, _>>()?;
            Ok(base_result(
                query,
                entries,
                normalize_next_page(s2c.next_page),
                s2c.all_count,
                None,
            ))
        }
    }
}

fn validate_security(value: &InstitutionSecurityQuery) -> Result<(), InstitutionQueryError> {
    if value.code.trim().is_empty()
        || value.code.len() > 128
        || value.code.chars().any(|v| {
            v.is_whitespace() || v.is_control() || matches!(v, '.' | '/' | '\\' | '?' | '#')
        })
    {
        return Err(InstitutionQueryError::InvalidQuery(
            "institution security code is invalid".to_owned(),
        ));
    }
    Ok(())
}

fn validate_market(market: i32) -> Result<(), InstitutionQueryError> {
    market_label(market).ok_or_else(|| {
        InstitutionQueryError::InvalidQuery(
            "institution market must be HK, US, SH, or SZ".to_owned(),
        )
    })?;
    Ok(())
}

fn validate_institution_id(value: Option<i32>) -> Result<(), InstitutionQueryError> {
    if value.is_none_or(|value| value <= 0) {
        return Err(InstitutionQueryError::InvalidQuery(
            "institution operation requires a positive institutionId".to_owned(),
        ));
    }
    Ok(())
}

fn market_label(market: i32) -> Option<&'static str> {
    match market {
        1 => Some("HK"),
        11 => Some("US"),
        21 => Some("SH"),
        22 => Some("SZ"),
        _ => None,
    }
}

#[derive(Debug, Error)]
pub enum InstitutionQueryError {
    #[error("invalid OpenD institution query: {0}")]
    InvalidQuery(String),
    #[error("OpenD institution session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("decode OpenD institution response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD {operation} retType={ret_type} errCode={err_code}: {message}")]
    Rejected {
        operation: &'static str,
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD institution response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD institution response: {0}")]
    InvalidResponse(String),
}

pub type FutuInstitutionOperation = InstitutionOperation;
pub type FutuInstitutionQuery = InstitutionQuery;
pub type FutuInstitutionQueryError = InstitutionQueryError;
pub type FutuInstitutionResult = InstitutionResult;
pub type FutuInstitutionEntry = InstitutionEntry;
pub type FutuInstitutionSecurity = InstitutionSecurity;
pub type FutuInstitutionSummary = InstitutionSummary;
pub use InstitutionReadPort as FutuInstitutionReadPort;
