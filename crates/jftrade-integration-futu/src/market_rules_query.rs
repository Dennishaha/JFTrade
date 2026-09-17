//! Broker market-rule lookup through Qot_GetStaticInfo (3202) with the Go
//! Qot_GetSecuritySnapshot (3203) fallback.
//!
//! Parity source: `go:452dea11:pkg/futu/adapter_marketdata_reader.go:634`
//! `futuMarketDataReader.QueryMarketRules`.  The primary read answers per-lot
//! quantities from static info; when that call fails or carries no usable lot
//! size, the same symbols are retried through the security snapshot and every
//! fallback row is reported as a warning so callers can surface the degraded
//! source instead of treating it as a first-class read.

use std::sync::{Arc, Mutex};
use std::time::Duration;

use jftrade_broker::MarketRuleItem;
use jftrade_marketdata::BrokerSecuritySnapshot;
use prost::Message;
use thiserror::Error;

use crate::security_snapshot_query::{
    OpenDSecuritySnapshotReader, SecuritySnapshotReadPort, market_code, market_label,
};
use crate::trade_proto::qot_common::Security;
use crate::trade_proto::qot_get_static_info as static_wire;
use crate::{
    OpenDManagedSessionError, OpenDSessionCoordinator, OpenDSessionCoordinatorError,
    PROTO_GET_STATIC_INFO,
};

const STATIC_INFO_TIMEOUT: Duration = Duration::from_secs(5);

/// One neutral static-info row: only the fields market-rule lookup consumes.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct SecurityInfoItem {
    pub symbol: String,
    pub lot_size: Option<i32>,
}

/// Neutral market-rule result, mirroring Go's `broker.MarketRuleSnapshot`.
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct MarketRuleSnapshot {
    pub account_id: Option<String>,
    pub rules: Vec<MarketRuleItem>,
    pub warnings: Vec<String>,
}

#[derive(Debug, Error)]
pub enum MarketRulesQueryError {
    #[error("futu: QueryMarketRules requires at least one symbol")]
    EmptySymbols,
    #[error("futu: invalid security symbol in market-rule query")]
    InvalidSymbol,
    #[error("futu market rules static info session: {0}")]
    Session(String),
    #[error("futu market rules static info rejected ({ret_type}/{err_code}): {message}")]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("decode OpenD Qot_GetStaticInfo response: {0}")]
    Decode(#[from] prost::DecodeError),
    /// Snapshot-only failure where the primary static-info read already
    /// succeeded but returned no usable lot size.  Go returns that fallback
    /// error verbatim, so the message is preserved unchanged.
    #[error("{0}")]
    Snapshot(String),
    #[error("{primary}; fallback QuerySecuritySnapshot failed: {fallback}")]
    FallbackFailed { primary: String, fallback: String },
    #[error("{primary}; fallback QuerySecuritySnapshot returned no market rules")]
    FallbackEmpty { primary: String },
    #[error("futu: QueryMarketRules returned no market rules")]
    NoRules,
}

/// Engine-facing contract for broker-provided market rules.
pub trait MarketRulesReadPort: Send + Sync + std::fmt::Debug {
    fn query(&self, symbols: &[String]) -> Result<MarketRuleSnapshot, MarketRulesQueryError>;
}

/// Static-info read seam so the fallback order can be exercised without a
/// live OpenD socket.
pub trait SecurityInfoReadPort: Send + Sync + std::fmt::Debug {
    fn query_static_info(&self, symbols: &[String]) -> Result<Vec<SecurityInfoItem>, String>;
}

impl From<OpenDManagedSessionError> for MarketRulesQueryError {
    fn from(error: OpenDManagedSessionError) -> Self {
        Self::Session(error.to_string())
    }
}

impl From<OpenDSessionCoordinatorError> for MarketRulesQueryError {
    fn from(error: OpenDSessionCoordinatorError) -> Self {
        Self::Session(error.to_string())
    }
}

#[derive(Clone)]
pub struct OpenDSecurityInfoReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDSecurityInfoReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDSecurityInfoReader")
            .finish_non_exhaustive()
    }
}

impl OpenDSecurityInfoReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl SecurityInfoReadPort for OpenDSecurityInfoReader {
    fn query_static_info(&self, symbols: &[String]) -> Result<Vec<SecurityInfoItem>, String> {
        let securities = symbols
            .iter()
            .map(|symbol| parse_security(symbol))
            .collect::<Result<Vec<_>, _>>()?;
        let body = static_wire::Request {
            c2s: static_wire::C2s {
                market: None,
                sec_type: None,
                security_list: securities,
                header: None,
            },
        }
        .encode_to_vec();
        let coordinator = self
            .coordinator
            .lock()
            .map_err(|_| "futu market rules static info: coordinator lock poisoned".to_owned())?;
        let session = coordinator.session().map_err(|error| error.to_string())?;
        let bytes = session
            .managed_session()
            .call_with_timeout(PROTO_GET_STATIC_INFO, &body, STATIC_INFO_TIMEOUT)
            .map_err(|error| error.to_string())?;
        let response =
            static_wire::Response::decode(bytes.as_slice()).map_err(|error| error.to_string())?;
        if response.ret_type != 0 {
            return Err(format!(
                "OpenD Qot_GetStaticInfo returned retType={} errCode={}: {}",
                response.ret_type,
                response.err_code.unwrap_or_default(),
                response
                    .ret_msg
                    .unwrap_or_else(|| "Qot_GetStaticInfo failed".to_owned())
            ));
        }
        Ok(response
            .s2c
            .map(|s2c| {
                s2c.static_info_list
                    .into_iter()
                    .filter_map(|entry| {
                        let basic = entry.basic;
                        let market = market_label(basic.security.market)?;
                        let code = basic.security.code.trim().to_ascii_uppercase();
                        if code.is_empty() {
                            return None;
                        }
                        Some(SecurityInfoItem {
                            symbol: format!("{market}.{code}"),
                            lot_size: Some(basic.lot_size),
                        })
                    })
                    .collect()
            })
            .unwrap_or_default())
    }
}

/// Production reader answering market rules from OpenD static info with the
/// security-snapshot fallback owned by the same session.
pub struct OpenDMarketRulesReader {
    static_info: Arc<dyn SecurityInfoReadPort>,
    snapshots: Arc<dyn SecuritySnapshotReadPort>,
}

impl std::fmt::Debug for OpenDMarketRulesReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDMarketRulesReader")
            .finish_non_exhaustive()
    }
}

impl OpenDMarketRulesReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self {
            static_info: Arc::new(OpenDSecurityInfoReader::new(Arc::clone(&coordinator))),
            snapshots: Arc::new(OpenDSecuritySnapshotReader::new(coordinator)),
        }
    }

    pub fn with_ports(
        static_info: Arc<dyn SecurityInfoReadPort>,
        snapshots: Arc<dyn SecuritySnapshotReadPort>,
    ) -> Self {
        Self {
            static_info,
            snapshots,
        }
    }
}

impl MarketRulesReadPort for OpenDMarketRulesReader {
    fn query(&self, symbols: &[String]) -> Result<MarketRuleSnapshot, MarketRulesQueryError> {
        if symbols.is_empty() {
            return Err(MarketRulesQueryError::EmptySymbols);
        }
        let primary = self.static_info.query_static_info(symbols);
        if let Ok(items) = &primary {
            let rules = market_rules_from_static_info(items);
            if !rules.is_empty() {
                return Ok(MarketRuleSnapshot {
                    account_id: None,
                    rules,
                    warnings: Vec::new(),
                });
            }
        }
        let fallback_reason = match &primary {
            Ok(_) => "QuerySecurityInfo returned no usable market rules".to_owned(),
            Err(error) => format!("QuerySecurityInfo failed: {error}"),
        };
        match self.snapshots.query(symbols) {
            Ok(snapshots) => {
                let rules = market_rules_from_snapshots(&snapshots);
                if rules.is_empty() {
                    return match primary {
                        Err(primary) => Err(MarketRulesQueryError::FallbackEmpty { primary }),
                        Ok(_) => Err(MarketRulesQueryError::NoRules),
                    };
                }
                Ok(MarketRuleSnapshot {
                    account_id: None,
                    rules,
                    warnings: vec![format!(
                        "futu market rules loaded from QuerySecuritySnapshot fallback because {fallback_reason}"
                    )],
                })
            }
            Err(fallback) => match primary {
                Err(primary) => Err(MarketRulesQueryError::FallbackFailed { primary, fallback }),
                Ok(_) => Err(MarketRulesQueryError::Snapshot(fallback)),
            },
        }
    }
}

/// Go `marketRulesFromSecurityInfo`: skip blank symbols and non-positive lots.
fn market_rules_from_static_info(items: &[SecurityInfoItem]) -> Vec<MarketRuleItem> {
    items
        .iter()
        .filter(|item| !item.symbol.trim().is_empty() && item.lot_size.is_some_and(|lot| lot > 0))
        .map(|item| MarketRuleItem {
            symbol: item.symbol.clone(),
            lot_size: item.lot_size,
            ..MarketRuleItem::default()
        })
        .collect()
}

/// Go `marketRulesFromSecuritySnapshot`: same blank/non-positive filtering.
fn market_rules_from_snapshots(snapshots: &[BrokerSecuritySnapshot]) -> Vec<MarketRuleItem> {
    snapshots
        .iter()
        .filter_map(|snapshot| {
            let symbol = snapshot.symbol.as_deref()?.trim().to_owned();
            let lot_size = snapshot.lot_size.filter(|lot| *lot > 0)?;
            if symbol.is_empty() {
                return None;
            }
            Some(MarketRuleItem {
                symbol,
                lot_size: Some(lot_size),
                ..MarketRuleItem::default()
            })
        })
        .collect()
}

fn parse_security(symbol: &str) -> Result<Security, String> {
    let (market, code) = symbol
        .split_once('.')
        .ok_or_else(|| format!("futu: invalid security symbol {symbol:?}"))?;
    let market =
        market_code(market).ok_or_else(|| format!("futu: invalid security symbol {symbol:?}"))?;
    let code = code.trim().to_ascii_uppercase();
    if code.is_empty() {
        return Err(format!("futu: invalid security symbol {symbol:?}"));
    }
    Ok(Security { market, code })
}

#[cfg(test)]
#[path = "market_rules_query_tests.rs"]
mod tests;
