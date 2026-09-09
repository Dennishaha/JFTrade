//! Read-only Qot_GetSearchQuote adapter. Searches never acquire subscriptions.

use std::sync::{Arc, Mutex};
use std::time::Duration;

use prost::Message;
use thiserror::Error;

use crate::{OpenDSessionCoordinator, trade_proto::qot_get_search_quote as wire};

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct InstrumentSearchEntry {
    pub market: String,
    pub code: String,
    pub name: Option<String>,
    pub security_type: Option<String>,
    pub is_watched: bool,
    pub lot_size: Option<i32>,
}

#[derive(Debug, Error)]
pub enum InstrumentSearchError {
    #[error("invalid instrument search keyword")]
    InvalidQuery,
    #[error("OpenD instrument search unavailable: {0}")]
    Session(String),
    #[error("decode OpenD instrument search: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD instrument search rejected ({ret_type}/{err_code}): {message}")]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD instrument search response is missing {0}")]
    MissingField(&'static str),
}

pub trait InstrumentSearchReadPort: Send + Sync + std::fmt::Debug {
    fn search(&self, keyword: &str) -> Result<Vec<InstrumentSearchEntry>, InstrumentSearchError>;
    fn lookup(
        &self,
        market: &str,
        code: &str,
    ) -> Result<Vec<InstrumentSearchEntry>, InstrumentSearchError>;
}

#[derive(Clone)]
pub struct OpenDInstrumentSearchReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDInstrumentSearchReader {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("OpenDInstrumentSearchReader")
            .finish_non_exhaustive()
    }
}

impl OpenDInstrumentSearchReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }

    fn call(&self, protocol: u32, request: &[u8]) -> Result<Vec<u8>, InstrumentSearchError> {
        let coordinator = self
            .coordinator
            .lock()
            .map_err(|_| InstrumentSearchError::Session("coordinator lock poisoned".to_owned()))?;
        let session = coordinator
            .session()
            .map_err(|error| InstrumentSearchError::Session(error.to_string()))?;
        session
            .managed_session()
            .call_with_timeout(protocol, request, Duration::from_secs(5))
            .map_err(|error| InstrumentSearchError::Session(error.to_string()))
    }
}

impl InstrumentSearchReadPort for OpenDInstrumentSearchReader {
    fn search(&self, keyword: &str) -> Result<Vec<InstrumentSearchEntry>, InstrumentSearchError> {
        decode_response(&self.call(wire::PROTOCOL_ID, &encode_request(keyword)?)?)
    }

    fn lookup(
        &self,
        market: &str,
        code: &str,
    ) -> Result<Vec<InstrumentSearchEntry>, InstrumentSearchError> {
        use crate::trade_proto::qot_get_static_info as static_wire;
        let request = encode_lookup(market, code)?;
        decode_lookup(&self.call(static_wire::PROTOCOL_ID, &request)?)
    }
}

fn encode_request(keyword: &str) -> Result<Vec<u8>, InstrumentSearchError> {
    let keyword = keyword.trim();
    if keyword.is_empty() || keyword.chars().any(char::is_control) {
        return Err(InstrumentSearchError::InvalidQuery);
    }
    Ok(wire::Request {
        c2s: wire::C2s {
            keyword: keyword.to_owned(),
            max_count: Some(100),
            header: None,
        },
    }
    .encode_to_vec())
}

fn decode_response(body: &[u8]) -> Result<Vec<InstrumentSearchEntry>, InstrumentSearchError> {
    let response = wire::Response::decode(body)?;
    if response.ret_type != 0 {
        return Err(InstrumentSearchError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response.ret_msg.unwrap_or_default(),
        });
    }
    response
        .s2c
        .ok_or(InstrumentSearchError::MissingField("s2c"))?
        .search_quote_list
        .into_iter()
        .map(map_entry)
        .collect()
}

fn map_entry(entry: wire::SearchQuote) -> Result<InstrumentSearchEntry, InstrumentSearchError> {
    let market = match entry
        .market
        .ok_or(InstrumentSearchError::MissingField("market"))?
    {
        1 => "HK",
        2 => "HK_FUTURE",
        11 => "US",
        21 => "SH",
        22 => "SZ",
        31 => "SG",
        41 => "JP",
        51 => "AU",
        61 => "MY",
        71 => "CA",
        81 => "FX",
        91 => "CRYPTO",
        _ => "UNKNOWN",
    };
    let code = entry
        .code
        .filter(|code| !code.trim().is_empty())
        .ok_or(InstrumentSearchError::MissingField("code"))?;
    let security_type = entry.sec_type.map(|value| {
        match value {
            1 => "BOND",
            2 => "BWRT",
            3 => "EQUITY",
            4 => "TRUST",
            5 => "WARRANT",
            6 => "INDEX",
            7 => "PLATE",
            8 => "OPTION",
            9 => "PLATESET",
            10 => "FUTURE",
            11 => "FOREX",
            12 => "CRYPTO",
            _ => "UNKNOWN",
        }
        .to_owned()
    });
    Ok(InstrumentSearchEntry {
        market: market.to_owned(),
        code: code.trim().to_ascii_uppercase(),
        name: entry.name,
        security_type,
        is_watched: entry.is_watched.unwrap_or(false),
        lot_size: None,
    })
}

fn encode_lookup(market: &str, code: &str) -> Result<Vec<u8>, InstrumentSearchError> {
    use crate::trade_proto::{qot_common::Security, qot_get_static_info as static_wire};
    let market = match market {
        "HK" => 1,
        "US" => 11,
        "SH" => 21,
        "SZ" => 22,
        "SG" => 31,
        "JP" => 41,
        "AU" => 51,
        "MY" => 61,
        "CA" => 71,
        _ => return Err(InstrumentSearchError::InvalidQuery),
    };
    if code.trim().is_empty() || code.chars().any(char::is_control) {
        return Err(InstrumentSearchError::InvalidQuery);
    }
    Ok(static_wire::Request {
        c2s: static_wire::C2s {
            market: None,
            sec_type: None,
            header: None,
            security_list: vec![Security {
                market,
                code: code.trim().to_owned(),
            }],
        },
    }
    .encode_to_vec())
}

fn decode_lookup(body: &[u8]) -> Result<Vec<InstrumentSearchEntry>, InstrumentSearchError> {
    use crate::trade_proto::qot_get_static_info as static_wire;
    let response = static_wire::Response::decode(body)?;
    if response.ret_type != 0 {
        return Err(InstrumentSearchError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response.ret_msg.unwrap_or_default(),
        });
    }
    response
        .s2c
        .ok_or(InstrumentSearchError::MissingField("s2c"))?
        .static_info_list
        .into_iter()
        .map(|entry| {
            let basic = entry.basic;
            let mut candidate = map_entry(wire::SearchQuote {
                market: Some(basic.security.market),
                code: Some(basic.security.code),
                name: Some(basic.name),
                sec_type: Some(basic.sec_type),
                is_watched: None,
            })?;
            candidate.lot_size = Some(basic.lot_size);
            Ok(candidate)
        })
        .collect()
}

#[cfg(test)]
#[path = "instrument_search_query_tests.rs"]
mod tests;
