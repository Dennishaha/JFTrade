//! Typed OpenD technical-indicator readers (`3259` and `3260`).
//!
//! Indicator calculation is asynchronous in OpenD: the request returns a
//! calculation id and a later push carries values.  This leaf intentionally
//! exposes only that acknowledged id; it never turns an absent push into a
//! fabricated result.

use std::sync::{Arc, Mutex};

use prost::Message;
use serde::Serialize;
use thiserror::Error;

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct IndicatorListQuery {
    pub search_key: Option<String>,
    pub lang_type: Option<i32>,
    pub search_mode: Option<i32>,
}

#[derive(Clone, Debug, PartialEq)]
pub struct IndicatorCalcQuery {
    pub short_name: String,
    pub lang_type: i32,
    pub market: i32,
    pub code: String,
    pub kl_type: i32,
    pub k_line: Vec<IndicatorKline>,
    pub num: Option<i32>,
    pub inputs: Vec<FutuIndicatorInput>,
}

#[derive(Clone, Debug, PartialEq)]
pub struct IndicatorKline {
    pub time: String,
    pub is_blank: bool,
    pub high_price: Option<f64>,
    pub open_price: Option<f64>,
    pub low_price: Option<f64>,
    pub close_price: Option<f64>,
    pub last_close_price: Option<f64>,
    pub volume: Option<i64>,
    pub turnover: Option<f64>,
    pub turnover_rate: Option<f64>,
    pub pe: Option<f64>,
    pub change_rate: Option<f64>,
    pub timestamp: Option<f64>,
    pub hp_volume: Option<f64>,
}

impl IndicatorKline {
    pub fn validate(&self) -> Result<(), TechnicalIndicatorQueryError> {
        if self.time.trim().is_empty() || self.time.chars().any(char::is_control) {
            return Err(TechnicalIndicatorQueryError::InvalidQuery(
                "indicator K-line time is invalid".to_owned(),
            ));
        }
        if self.volume.is_some_and(|value| value < 0) {
            return Err(TechnicalIndicatorQueryError::InvalidQuery(
                "indicator K-line volume must be non-negative".to_owned(),
            ));
        }
        validate_finite([
            ("highPrice", self.high_price),
            ("openPrice", self.open_price),
            ("lowPrice", self.low_price),
            ("closePrice", self.close_price),
            ("lastClosePrice", self.last_close_price),
            ("turnover", self.turnover),
            ("turnoverRate", self.turnover_rate),
            ("pe", self.pe),
            ("changeRate", self.change_rate),
            ("timestamp", self.timestamp),
            ("hpVolume", self.hp_volume),
        ])
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct FutuIndicatorInput {
    pub index: i32,
    pub value: Option<String>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FutuIndicatorInputParameter {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub index: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub value: Option<FutuIndicatorParamValue>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub var_name: Option<String>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FutuIndicatorParamValue {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub value_type: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub int_value: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub float_value: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub string_value: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub bool_value: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub color_value: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub shape_value: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub line_value: Option<i32>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FutuIndicatorOutputParameter {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub index: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct TechnicalIndicatorInfo {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub short_name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub full_name: Option<String>,
    pub inputs: Vec<FutuIndicatorInputParameter>,
    pub outputs: Vec<FutuIndicatorOutputParameter>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub script: Option<String>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct TechnicalIndicatorEntry {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub my_lang: Option<TechnicalIndicatorInfo>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub python: Option<TechnicalIndicatorInfo>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct TechnicalIndicatorList {
    pub indicators: Vec<TechnicalIndicatorEntry>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct TechnicalIndicatorCalculation {
    pub calc_id: String,
}

#[derive(Clone, Debug, PartialEq)]
pub enum TechnicalIndicatorQuery {
    List(IndicatorListQuery),
    Calculate(IndicatorCalcQuery),
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", tag = "kind", content = "value")]
pub enum TechnicalIndicatorResult {
    List(TechnicalIndicatorList),
    Calculate(TechnicalIndicatorCalculation),
}

pub trait TechnicalIndicatorReadPort: Send + Sync + std::fmt::Debug {
    fn query(
        &self,
        query: &TechnicalIndicatorQuery,
    ) -> Result<TechnicalIndicatorResult, TechnicalIndicatorQueryError>;

    fn list(
        &self,
        query: &IndicatorListQuery,
    ) -> Result<TechnicalIndicatorList, TechnicalIndicatorQueryError>;

    fn calculate(
        &self,
        query: &IndicatorCalcQuery,
    ) -> Result<TechnicalIndicatorCalculation, TechnicalIndicatorQueryError>;
}

#[derive(Clone)]
pub struct FutuTechnicalIndicatorReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for FutuTechnicalIndicatorReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("FutuTechnicalIndicatorReader")
            .finish_non_exhaustive()
    }
}

impl FutuTechnicalIndicatorReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl TechnicalIndicatorReadPort for FutuTechnicalIndicatorReader {
    fn query(
        &self,
        query: &TechnicalIndicatorQuery,
    ) -> Result<TechnicalIndicatorResult, TechnicalIndicatorQueryError> {
        match query {
            TechnicalIndicatorQuery::List(query) => {
                self.list(query).map(TechnicalIndicatorResult::List)
            }
            TechnicalIndicatorQuery::Calculate(query) => self
                .calculate(query)
                .map(TechnicalIndicatorResult::Calculate),
        }
    }

    fn list(
        &self,
        query: &IndicatorListQuery,
    ) -> Result<TechnicalIndicatorList, TechnicalIndicatorQueryError> {
        validate_list_query(query)?;
        let body = self.call(
            crate::trade_proto::qot_get_indicator_list::PROTOCOL_ID,
            encode_list(query),
        )?;
        decode_list(&body)
    }

    fn calculate(
        &self,
        query: &IndicatorCalcQuery,
    ) -> Result<TechnicalIndicatorCalculation, TechnicalIndicatorQueryError> {
        validate_calc_query(query)?;
        let body = self.call(
            crate::trade_proto::qot_request_indicator_calc::PROTOCOL_ID,
            encode_calculate(query),
        )?;
        decode_calculate(&body)
    }
}

impl FutuTechnicalIndicatorReader {
    fn call(&self, protocol: u32, body: Vec<u8>) -> Result<Vec<u8>, TechnicalIndicatorQueryError> {
        let coordinator = self.coordinator.lock().map_err(|_| {
            TechnicalIndicatorQueryError::Session(OpenDSessionCoordinatorError::Closed)
        })?;
        let session = coordinator.session()?;
        Ok(session
            .managed_session()
            .call(protocol, &body)
            .map_err(OpenDSessionCoordinatorError::from)?)
    }
}

fn validate_list_query(query: &IndicatorListQuery) -> Result<(), TechnicalIndicatorQueryError> {
    if query
        .search_key
        .as_deref()
        .is_some_and(|value| value.len() > 128 || value.chars().any(char::is_control))
    {
        return Err(TechnicalIndicatorQueryError::InvalidQuery(
            "indicator searchKey is invalid".to_owned(),
        ));
    }
    if query
        .lang_type
        .is_some_and(|value| !(0..=2).contains(&value))
    {
        return Err(TechnicalIndicatorQueryError::InvalidQuery(
            "indicator langType must be 0, 1, or 2".to_owned(),
        ));
    }
    if query
        .search_mode
        .is_some_and(|value| !matches!(value, 0 | 1))
    {
        return Err(TechnicalIndicatorQueryError::InvalidQuery(
            "indicator searchMode must be 0 or 1".to_owned(),
        ));
    }
    Ok(())
}

fn validate_calc_query(query: &IndicatorCalcQuery) -> Result<(), TechnicalIndicatorQueryError> {
    validate_market(query.market)?;
    if query.short_name.trim().is_empty()
        || query.short_name.len() > 128
        || query.short_name.chars().any(char::is_control)
    {
        return Err(TechnicalIndicatorQueryError::InvalidQuery(
            "indicator shortName is invalid".to_owned(),
        ));
    }
    if !matches!(query.lang_type, 1 | 2) {
        return Err(TechnicalIndicatorQueryError::InvalidQuery(
            "indicator calculation langType must be MyLang (1) or Python (2)".to_owned(),
        ));
    }
    if !(1..=15).contains(&query.kl_type) {
        return Err(TechnicalIndicatorQueryError::InvalidQuery(
            "indicator calculation klType is unsupported".to_owned(),
        ));
    }
    let code = query.code.trim();
    if code.is_empty()
        || code.len() > 128
        || code.chars().any(|value| {
            value.is_whitespace()
                || value.is_control()
                || matches!(value, '.' | '/' | '\\' | '?' | '#')
        })
    {
        return Err(TechnicalIndicatorQueryError::InvalidQuery(
            "indicator security code is invalid".to_owned(),
        ));
    }
    if query.k_line.len() > 2000 {
        return Err(TechnicalIndicatorQueryError::InvalidQuery(
            "indicator K-line list cannot exceed 2000 entries".to_owned(),
        ));
    }
    if query.num.is_some_and(|value| !(1..=2000).contains(&value)) {
        return Err(TechnicalIndicatorQueryError::InvalidQuery(
            "indicator num must be between 1 and 2000".to_owned(),
        ));
    }
    if query.inputs.len() > 100 {
        return Err(TechnicalIndicatorQueryError::InvalidQuery(
            "indicator inputs cannot exceed 100 entries".to_owned(),
        ));
    }
    for kline in &query.k_line {
        kline.validate()?;
    }
    for input in &query.inputs {
        if input.index < 0
            || input
                .value
                .as_deref()
                .is_some_and(|value| value.len() > 256 || value.chars().any(char::is_control))
        {
            return Err(TechnicalIndicatorQueryError::InvalidQuery(
                "indicator input override is invalid".to_owned(),
            ));
        }
    }
    Ok(())
}

fn validate_market(market: i32) -> Result<(), TechnicalIndicatorQueryError> {
    if !matches!(market, 1 | 11 | 21 | 22 | 31 | 41 | 51 | 61 | 71 | 81 | 91) {
        return Err(TechnicalIndicatorQueryError::InvalidQuery(
            "indicator market is unsupported".to_owned(),
        ));
    }
    Ok(())
}

fn encode_list(query: &IndicatorListQuery) -> Vec<u8> {
    use crate::trade_proto::qot_get_indicator_list::{C2s, Request};
    Request {
        c2s: C2s {
            search_key: query.search_key.clone().and_then(trimmed),
            lang_type: query.lang_type,
            search_mode: query.search_mode,
        },
    }
    .encode_to_vec()
}

fn encode_calculate(query: &IndicatorCalcQuery) -> Vec<u8> {
    use crate::trade_proto::qot_request_indicator_calc::{
        C2s, IndicatorCalcData, IndicatorInputItem, Request,
    };
    Request {
        c2s: C2s {
            short_name: query.short_name.trim().to_owned(),
            lang_type: query.lang_type,
            data: IndicatorCalcData {
                security: crate::trade_proto::qot_common::Security {
                    market: query.market,
                    code: query.code.trim().to_ascii_uppercase(),
                },
                kl_type: query.kl_type,
                k_line: query.k_line.iter().map(encode_kline).collect(),
            },
            num: query.num,
            inputs: query
                .inputs
                .iter()
                .map(|input| IndicatorInputItem {
                    index: input.index,
                    value: input.value.clone().and_then(trimmed),
                })
                .collect(),
        },
    }
    .encode_to_vec()
}

fn encode_kline(value: &IndicatorKline) -> crate::trade_proto::qot_common::KLine {
    crate::trade_proto::qot_common::KLine {
        time: value.time.trim().to_owned(),
        is_blank: value.is_blank,
        high_price: value.high_price,
        open_price: value.open_price,
        low_price: value.low_price,
        close_price: value.close_price,
        last_close_price: value.last_close_price,
        volume: value.volume,
        turnover: value.turnover,
        turnover_rate: value.turnover_rate,
        pe: value.pe,
        change_rate: value.change_rate,
        timestamp: value.timestamp,
        hp_volume: value.hp_volume,
    }
}

fn decode_list(body: &[u8]) -> Result<TechnicalIndicatorList, TechnicalIndicatorQueryError> {
    use crate::trade_proto::qot_get_indicator_list::Response;
    let response = Response::decode(body).map_err(TechnicalIndicatorQueryError::Decode)?;
    ensure_success(
        response.ret_type,
        response.err_code,
        response.ret_msg,
        "indicator list",
    )?;
    let s2c = response
        .s2c
        .ok_or(TechnicalIndicatorQueryError::MissingS2c)?;
    let indicators = s2c
        .indicator_list
        .into_iter()
        .map(map_indicator_entry)
        .collect::<Result<Vec<_>, _>>()?;
    Ok(TechnicalIndicatorList { indicators })
}

fn decode_calculate(
    body: &[u8],
) -> Result<TechnicalIndicatorCalculation, TechnicalIndicatorQueryError> {
    use crate::trade_proto::qot_request_indicator_calc::Response;
    let response = Response::decode(body).map_err(TechnicalIndicatorQueryError::Decode)?;
    ensure_success(
        response.ret_type,
        response.err_code,
        response.ret_msg,
        "indicator calculation",
    )?;
    let s2c = response
        .s2c
        .ok_or(TechnicalIndicatorQueryError::MissingS2c)?;
    let calc_id = s2c.calc_id.trim().to_owned();
    if calc_id.is_empty() || calc_id.chars().any(char::is_control) {
        return Err(TechnicalIndicatorQueryError::InvalidResponse(
            "OpenD indicator calculation response has an invalid calcId".to_owned(),
        ));
    }
    Ok(TechnicalIndicatorCalculation { calc_id })
}

fn map_indicator_entry(
    value: crate::trade_proto::qot_get_indicator_list::IndicatorEntry,
) -> Result<TechnicalIndicatorEntry, TechnicalIndicatorQueryError> {
    Ok(TechnicalIndicatorEntry {
        my_lang: value.my_lang.map(map_indicator_info).transpose()?,
        python: value.python.map(map_indicator_info).transpose()?,
    })
}

fn map_indicator_info(
    value: crate::trade_proto::qot_get_indicator_list::IndicatorInfo,
) -> Result<TechnicalIndicatorInfo, TechnicalIndicatorQueryError> {
    let inputs = value
        .inputs
        .into_iter()
        .map(map_input_parameter)
        .collect::<Result<Vec<_>, _>>()?;
    let outputs = value
        .outputs
        .into_iter()
        .map(map_output_parameter)
        .collect::<Result<Vec<_>, _>>()?;
    Ok(TechnicalIndicatorInfo {
        short_name: optional_text(value.short_name),
        full_name: optional_text(value.full_name),
        inputs,
        outputs,
        script: optional_text(value.script),
    })
}

fn map_input_parameter(
    value: crate::trade_proto::qot_common::IndicatorInputParam,
) -> Result<FutuIndicatorInputParameter, TechnicalIndicatorQueryError> {
    validate_optional_index(value.index, "indicator input index")?;
    Ok(FutuIndicatorInputParameter {
        index: value.index,
        name: optional_text(value.name),
        value: value.value.map(map_param_value).transpose()?,
        var_name: optional_text(value.var_name),
    })
}

fn map_output_parameter(
    value: crate::trade_proto::qot_common::IndicatorOutputParam,
) -> Result<FutuIndicatorOutputParameter, TechnicalIndicatorQueryError> {
    validate_optional_index(value.index, "indicator output index")?;
    Ok(FutuIndicatorOutputParameter {
        index: value.index,
        name: optional_text(value.name),
    })
}

fn map_param_value(
    value: crate::trade_proto::qot_common::IndicatorParamValue,
) -> Result<FutuIndicatorParamValue, TechnicalIndicatorQueryError> {
    if value.float_value.is_some_and(|value| !value.is_finite()) {
        return Err(TechnicalIndicatorQueryError::InvalidResponse(
            "OpenD indicator float parameter is not finite".to_owned(),
        ));
    }
    Ok(FutuIndicatorParamValue {
        value_type: value.r#type,
        int_value: value.int_value,
        float_value: value.float_value,
        string_value: optional_text(value.string_value),
        bool_value: value.bool_value,
        color_value: optional_text(value.color_value),
        shape_value: value.shape_value,
        line_value: value.line_value,
    })
}

fn validate_optional_index(
    value: Option<i32>,
    field: &str,
) -> Result<(), TechnicalIndicatorQueryError> {
    if value.is_some_and(|value| value < 0) {
        return Err(TechnicalIndicatorQueryError::InvalidResponse(format!(
            "OpenD {field} must be non-negative"
        )));
    }
    Ok(())
}

fn validate_finite<const N: usize>(
    values: [(&str, Option<f64>); N],
) -> Result<(), TechnicalIndicatorQueryError> {
    for (field, value) in values {
        if value.is_some_and(|value| !value.is_finite()) {
            return Err(TechnicalIndicatorQueryError::InvalidQuery(format!(
                "indicator {field} must be finite"
            )));
        }
    }
    Ok(())
}

fn ensure_success(
    ret_type: i32,
    err_code: Option<i32>,
    ret_msg: Option<String>,
    operation: &'static str,
) -> Result<(), TechnicalIndicatorQueryError> {
    if ret_type == 0 {
        return Ok(());
    }
    Err(TechnicalIndicatorQueryError::Rejected {
        operation,
        ret_type,
        err_code: err_code.unwrap_or_default(),
        message: ret_msg.unwrap_or_else(|| format!("OpenD {operation} request failed")),
    })
}

fn optional_text(value: Option<String>) -> Option<String> {
    value.and_then(trimmed)
}

fn trimmed(value: String) -> Option<String> {
    let value = value.trim().to_owned();
    (!value.is_empty()).then_some(value)
}

#[derive(Debug, Error)]
pub enum TechnicalIndicatorQueryError {
    #[error("invalid OpenD technical indicator query: {0}")]
    InvalidQuery(String),
    #[error("OpenD technical indicator session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("decode OpenD technical indicator response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD {operation} retType={ret_type} errCode={err_code}: {message}")]
    Rejected {
        operation: &'static str,
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD technical indicator response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD technical indicator response: {0}")]
    InvalidResponse(String),
}

pub type FutuIndicatorList = TechnicalIndicatorList;
pub type FutuIndicatorCalculation = TechnicalIndicatorCalculation;
pub type FutuIndicatorListQuery = IndicatorListQuery;
pub type FutuIndicatorQueryError = TechnicalIndicatorQueryError;
pub use TechnicalIndicatorReadPort as FutuIndicatorReadPort;
