//! Typed OpenD option-strategy analysis reader
//! (`Qot_GetOptionStrategyAnalysis/3257`).
//!
//! Analysis requests accept one or more neutral combo legs and return the
//! calculated quote, payoff, probability, and Greek metrics without exposing
//! generated protobuf messages to the engine.

use std::sync::{Arc, Mutex};

use prost::Message;
use serde::Serialize;
use thiserror::Error;

use crate::option_strategy_query::{OptionStrategyLeg, encode_combo_leg};
use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

#[derive(Clone, Debug, PartialEq)]
pub struct OptionStrategyAnalysisQuery {
    pub multi_legs: Vec<OptionStrategyLeg>,
}

impl OptionStrategyAnalysisQuery {
    pub fn validate(&self) -> Result<(), OptionStrategyAnalysisQueryError> {
        if self.multi_legs.is_empty() {
            return Err(OptionStrategyAnalysisQueryError::InvalidQuery(
                "option strategy analysis requires at least one combo leg".to_owned(),
            ));
        }
        for (index, leg) in self.multi_legs.iter().enumerate() {
            encode_combo_leg(leg).map_err(|error| {
                OptionStrategyAnalysisQueryError::InvalidQuery(format!(
                    "option strategy analysis leg {index}: {error}"
                ))
            })?;
        }
        Ok(())
    }
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionStrategyAnalysisSnapshot {
    pub code: String,
    pub name: String,
    pub option_strategy: i32,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub bid1: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ask1: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub max_profit: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub max_loss: Option<f64>,
    pub breakeven_points: Vec<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub prob_of_profit: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub delta: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub theta: Option<f64>,
}

pub trait OptionStrategyAnalysisReadPort: Send + Sync + std::fmt::Debug {
    fn query(
        &self,
        query: &OptionStrategyAnalysisQuery,
    ) -> Result<OptionStrategyAnalysisSnapshot, OptionStrategyAnalysisQueryError>;
}

#[derive(Clone)]
pub struct OpenDOptionStrategyAnalysisReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDOptionStrategyAnalysisReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDOptionStrategyAnalysisReader")
            .finish_non_exhaustive()
    }
}

impl OpenDOptionStrategyAnalysisReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl OptionStrategyAnalysisReadPort for OpenDOptionStrategyAnalysisReader {
    fn query(
        &self,
        query: &OptionStrategyAnalysisQuery,
    ) -> Result<OptionStrategyAnalysisSnapshot, OptionStrategyAnalysisQueryError> {
        query.validate()?;
        let coordinator = self.coordinator.lock().map_err(|_| {
            OptionStrategyAnalysisQueryError::Session(OpenDSessionCoordinatorError::Closed)
        })?;
        let session = coordinator.session()?;
        let response = session
            .managed_session()
            .call(
                crate::trade_proto::qot_get_option_strategy_analysis::PROTOCOL_ID,
                &encode_request(query)?,
            )
            .map_err(crate::OpenDSessionCoordinatorError::from)?;
        decode_response(&response)
    }
}

fn encode_request(
    query: &OptionStrategyAnalysisQuery,
) -> Result<Vec<u8>, OptionStrategyAnalysisQueryError> {
    use crate::trade_proto::qot_get_option_strategy_analysis::{C2s, Request};
    let multi_legs = query
        .multi_legs
        .iter()
        .map(|leg| encode_combo_leg(leg).map_err(OptionStrategyAnalysisQueryError::InvalidQuery))
        .collect::<Result<Vec<_>, _>>()?;
    Ok(Request {
        c2s: C2s {
            multi_legs,
            header: None,
        },
    }
    .encode_to_vec())
}

fn decode_response(
    body: &[u8],
) -> Result<OptionStrategyAnalysisSnapshot, OptionStrategyAnalysisQueryError> {
    use crate::trade_proto::qot_get_option_strategy_analysis::Response;
    let response = Response::decode(body).map_err(OptionStrategyAnalysisQueryError::Decode)?;
    if response.ret_type != 0 {
        return Err(OptionStrategyAnalysisQueryError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response
                .ret_msg
                .unwrap_or_else(|| "OpenD option strategy analysis request failed".to_owned()),
        });
    }
    let Some(s2c) = response.s2c else {
        return Err(OptionStrategyAnalysisQueryError::MissingS2c);
    };
    let code = s2c.code.trim().to_owned();
    if code.is_empty() {
        return Err(OptionStrategyAnalysisQueryError::InvalidResponse(
            "option strategy analysis response code must not be empty".to_owned(),
        ));
    }
    let name = s2c.name.trim().to_owned();
    if name.is_empty() {
        return Err(OptionStrategyAnalysisQueryError::InvalidResponse(
            "option strategy analysis response name must not be empty".to_owned(),
        ));
    }
    if ![1, 2, 4, 6, 7, 8, 9, 11, 13, 14, 15, 16, 100].contains(&s2c.option_strategy) {
        return Err(OptionStrategyAnalysisQueryError::InvalidResponse(
            "option strategy analysis response has an unsupported strategy type".to_owned(),
        ));
    }
    for (field, value) in [
        ("bid1", s2c.bid1),
        ("ask1", s2c.ask1),
        ("maxProfit", s2c.max_profit),
        ("maxLoss", s2c.max_loss),
        ("probOfProfit", s2c.prob_of_profit),
        ("delta", s2c.delta),
        ("theta", s2c.theta),
    ] {
        if value.is_some_and(|value| !value.is_finite()) {
            return Err(OptionStrategyAnalysisQueryError::InvalidResponse(format!(
                "option strategy analysis {field} must be finite"
            )));
        }
    }
    if s2c.breakeven_points.iter().any(|value| !value.is_finite()) {
        return Err(OptionStrategyAnalysisQueryError::InvalidResponse(
            "option strategy analysis breakevenPoints must be finite".to_owned(),
        ));
    }
    Ok(OptionStrategyAnalysisSnapshot {
        code,
        name,
        option_strategy: s2c.option_strategy,
        bid1: s2c.bid1,
        ask1: s2c.ask1,
        max_profit: s2c.max_profit,
        max_loss: s2c.max_loss,
        breakeven_points: s2c.breakeven_points,
        prob_of_profit: s2c.prob_of_profit,
        delta: s2c.delta,
        theta: s2c.theta,
    })
}

#[derive(Debug, Error)]
pub enum OptionStrategyAnalysisQueryError {
    #[error("invalid OpenD option strategy analysis query: {0}")]
    InvalidQuery(String),
    #[error("OpenD option strategy analysis session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("decode OpenD Qot_GetOptionStrategyAnalysis response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD Qot_GetOptionStrategyAnalysis retType={ret_type} errCode={err_code}: {message}")]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD Qot_GetOptionStrategyAnalysis response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD option strategy analysis response: {0}")]
    InvalidResponse(String),
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::option_strategy_query::{OptionStrategyLeg, OptionStrategySecurity};
    use crate::trade_proto::qot_get_option_strategy_analysis::{Response, S2c};
    use prost::Message;

    fn leg() -> OptionStrategyLeg {
        OptionStrategyLeg {
            security: OptionStrategySecurity {
                market: "US".to_owned(),
                code: "AAPL260918C00100000".to_owned(),
                quote_market: "US".to_owned(),
                trade_market: "US".to_owned(),
                instrument_id: "US.AAPL260918C00100000".to_owned(),
            },
            side: Some(1),
            qty_ratio: Some(1.0),
            position_id: None,
            pred_side: None,
        }
    }

    #[test]
    fn request_requires_and_encodes_combo_legs() {
        let query = OptionStrategyAnalysisQuery {
            multi_legs: vec![leg()],
        };
        let body = encode_request(&query).expect("request");
        let request =
            crate::trade_proto::qot_get_option_strategy_analysis::Request::decode(body.as_slice())
                .expect("decode request");
        assert_eq!(request.c2s.multi_legs.len(), 1);
        assert_eq!(
            request.c2s.multi_legs[0].security.code,
            "AAPL260918C00100000"
        );
    }

    #[test]
    fn response_maps_metrics_and_rejects_non_finite_values() {
        let body = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                code: "AAPL260918C/P100".to_owned(),
                name: "AAPL straddle".to_owned(),
                option_strategy: 6,
                bid1: Some(1.0),
                ask1: Some(2.0),
                max_profit: Some(9_999_999.0),
                max_loss: Some(-100.0),
                breakeven_points: vec![95.0, 105.0],
                prob_of_profit: Some(0.5),
                delta: Some(0.1),
                theta: Some(-0.2),
            }),
        }
        .encode_to_vec();
        let snapshot = decode_response(&body).expect("snapshot");
        assert_eq!(snapshot.code, "AAPL260918C/P100");
        assert_eq!(snapshot.breakeven_points, vec![95.0, 105.0]);

        let invalid = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                code: "strategy".to_owned(),
                name: "strategy".to_owned(),
                option_strategy: 6,
                bid1: Some(f64::NAN),
                ask1: None,
                max_profit: None,
                max_loss: None,
                breakeven_points: Vec::new(),
                prob_of_profit: None,
                delta: None,
                theta: None,
            }),
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&invalid),
            Err(OptionStrategyAnalysisQueryError::InvalidResponse(_))
        ));
    }

    #[test]
    fn rejects_empty_or_invalid_legs() {
        let empty = OptionStrategyAnalysisQuery {
            multi_legs: Vec::new(),
        };
        assert!(matches!(
            empty.validate(),
            Err(OptionStrategyAnalysisQueryError::InvalidQuery(_))
        ));
        let mut invalid = leg();
        invalid.qty_ratio = Some(f64::INFINITY);
        let query = OptionStrategyAnalysisQuery {
            multi_legs: vec![invalid],
        };
        assert!(matches!(
            query.validate(),
            Err(OptionStrategyAnalysisQueryError::InvalidQuery(_))
        ));
    }
}
