//! Typed Qot_GetSecuritySnapshot (3203) reader.

use std::str::FromStr;
use std::time::Duration;

use jftrade_kernel::{DecimalText, Fixed8};
use jftrade_marketdata::BrokerSecuritySnapshot;
use prost::Message;
use thiserror::Error;

use crate::{
    OpenDInitializedSession, PROTO_GET_SECURITY_SNAPSHOT,
    trade_proto::qot_get_security_snapshot as wire,
};

const SNAPSHOT_TIMEOUT: Duration = Duration::from_millis(900);

pub trait SecuritySnapshotReadPort: Send + Sync {
    fn query(&self, instruments: &[String]) -> Result<Vec<BrokerSecuritySnapshot>, String>;
}

#[derive(Clone)]
pub struct OpenDSecuritySnapshotReader {
    session: OpenDInitializedSession,
}

impl OpenDSecuritySnapshotReader {
    pub fn new(session: OpenDInitializedSession) -> Self {
        Self { session }
    }

    pub fn query(
        &self,
        instruments: &[String],
    ) -> Result<Vec<BrokerSecuritySnapshot>, SecuritySnapshotQueryError> {
        if instruments.is_empty() {
            return Ok(Vec::new());
        }
        let securities = instruments
            .iter()
            .map(|value| {
                let (market, code) = value
                    .split_once('.')
                    .ok_or_else(|| SecuritySnapshotQueryError::InvalidInstrument(value.clone()))?;
                let market = market_code(market)
                    .ok_or_else(|| SecuritySnapshotQueryError::InvalidInstrument(value.clone()))?;
                Ok::<_, SecuritySnapshotQueryError>(crate::trade_proto::qot_common::Security {
                    market,
                    code: code.trim().to_ascii_uppercase(),
                })
            })
            .collect::<Result<Vec<_>, _>>()?;
        let body = wire::Request {
            c2s: wire::C2s {
                security_list: securities,
                header: None,
            },
        }
        .encode_to_vec();
        let bytes = self.session.managed_session().call_with_timeout(
            PROTO_GET_SECURITY_SNAPSHOT,
            &body,
            SNAPSHOT_TIMEOUT,
        )?;
        let response =
            wire::Response::decode(bytes.as_slice()).map_err(SecuritySnapshotQueryError::Decode)?;
        if response.ret_type != 0 {
            return Err(SecuritySnapshotQueryError::Rejected {
                ret_type: response.ret_type,
                err_code: response.err_code.unwrap_or_default(),
                message: response
                    .ret_msg
                    .unwrap_or_else(|| "OpenD GetSecuritySnapshot failed".to_owned()),
            });
        }
        Ok(response
            .s2c
            .map(|s2c| {
                s2c.snapshot_list
                    .into_iter()
                    .filter_map(map_snapshot)
                    .collect()
            })
            .unwrap_or_default())
    }
}

impl SecuritySnapshotReadPort for OpenDSecuritySnapshotReader {
    fn query(&self, instruments: &[String]) -> Result<Vec<BrokerSecuritySnapshot>, String> {
        OpenDSecuritySnapshotReader::query(self, instruments).map_err(|error| error.to_string())
    }
}

#[derive(Debug, Error)]
pub enum SecuritySnapshotQueryError {
    #[error("invalid OpenD security snapshot instrument: {0}")]
    InvalidInstrument(String),
    #[error("OpenD security snapshot session: {0}")]
    Session(#[from] crate::OpenDManagedSessionError),
    #[error("decode OpenD Qot_GetSecuritySnapshot response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error(
        "OpenD Qot_GetSecuritySnapshot returned retType={ret_type} errCode={err_code}: {message}"
    )]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
}

fn map_snapshot(snapshot: wire::Snapshot) -> Option<BrokerSecuritySnapshot> {
    let basic = snapshot.basic;
    let market = market_label(basic.security.market)?;
    let code = basic.security.code.trim().to_ascii_uppercase();
    if code.is_empty() {
        return None;
    }
    Some(BrokerSecuritySnapshot {
        symbol: Some(format!("{market}.{code}")),
        market: Some(market.to_owned()),
        name: basic.name,
        is_suspended: Some(basic.is_suspend),
        bid_price: optional_fixed8(basic.bid_price),
        ask_price: optional_fixed8(basic.ask_price),
        lot_size: Some(basic.lot_size),
        security_type: Some(security_type(basic.r#type).to_owned()),
        open_price: optional_fixed8(Some(basic.open_price)),
        high_price: optional_fixed8(Some(basic.high_price)),
        low_price: optional_fixed8(Some(basic.low_price)),
        previous_close: optional_fixed8(Some(basic.last_close_price)),
        turnover: optional_decimal(Some(basic.turnover)),
        update_time: Some(basic.update_time),
        status: basic.sec_status,
        pe_rate: snapshot
            .equity_ex_data
            .as_ref()
            .and_then(|v| optional_decimal(Some(v.pe_rate))),
        pb_rate: snapshot
            .equity_ex_data
            .as_ref()
            .and_then(|v| optional_decimal(Some(v.pb_rate))),
    })
}

fn optional_fixed8(value: Option<f64>) -> Option<Fixed8> {
    value
        .filter(|v| v.is_finite())
        .and_then(|v| Fixed8::from_str(&v.to_string()).ok())
}
fn optional_decimal(value: Option<f64>) -> Option<DecimalText> {
    value
        .filter(|v| v.is_finite())
        .and_then(|v| DecimalText::from_str(&v.to_string()).ok())
}
fn market_code(value: &str) -> Option<i32> {
    match value.trim().to_ascii_uppercase().as_str() {
        "HK" => Some(1),
        "US" => Some(11),
        "SH" => Some(21),
        "SZ" => Some(22),
        _ => None,
    }
}
fn market_label(value: i32) -> Option<&'static str> {
    match value {
        1 => Some("HK"),
        11 => Some("US"),
        21 => Some("SH"),
        22 => Some("SZ"),
        _ => None,
    }
}
fn security_type(value: i32) -> &'static str {
    match value {
        1 => "EQUITY",
        2 => "BOND",
        3 => "WARRANT",
        4 => "OPTION",
        5 => "FUTURE",
        6 => "INDEX",
        7 => "PLATE",
        8 => "FUND",
        _ => "UNKNOWN",
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::trade_proto::qot_common::Security;

    #[test]
    fn maps_security_snapshot_bbo_and_equity_metrics_without_defaults() {
        let value = wire::Snapshot {
            basic: wire::SnapshotBasicData {
                security: Security {
                    market: 11,
                    code: "AAPL".to_owned(),
                },
                name: Some("Apple Inc.".to_owned()),
                r#type: 1,
                is_suspend: false,
                list_time: "1980-12-12".to_owned(),
                lot_size: 1,
                price_spread: 0.01,
                update_time: "09:30:00".to_owned(),
                high_price: 190.0,
                open_price: 188.0,
                low_price: 187.0,
                last_close_price: 187.5,
                cur_price: 189.5,
                volume: 10,
                turnover: 1895.0,
                turnover_rate: 0.2,
                list_timestamp: None,
                update_timestamp: None,
                ask_price: Some(189.6),
                bid_price: Some(189.4),
                ask_vol: Some(20),
                bid_vol: Some(30),
                enable_margin: None,
                mortgage_ratio: None,
                long_margin_initial_ratio: None,
                enable_short_sell: None,
                short_sell_rate: None,
                short_available_volume: None,
                short_margin_initial_ratio: None,
                amplitude: None,
                avg_price: None,
                bid_ask_ratio: None,
                volume_ratio: None,
                highest52_weeks_price: None,
                lowest52_weeks_price: None,
                highest_history_price: None,
                lowest_history_price: None,
                pre_market: None,
                after_market: None,
                sec_status: Some(3),
                close_price5_minute: None,
                overnight: None,
                hp_volume: None,
                hp_ask_vol: None,
                hp_bid_vol: None,
            },
            equity_ex_data: Some(wire::EquitySnapshotExData {
                issued_shares: 1,
                issued_market_val: 1.0,
                net_asset: 1.0,
                net_profit: 1.0,
                earnings_pershare: 1.0,
                outstanding_shares: 1,
                outstanding_market_val: 1.0,
                net_asset_pershare: 1.0,
                ey_rate: 1.0,
                pe_rate: 30.0,
                pb_rate: 5.0,
                pe_ttm_rate: 29.0,
                dividend_ttm: None,
                dividend_ratio_ttm: None,
                dividend_lfy: None,
                dividend_lfy_ratio: None,
            }),
            ..Default::default()
        };
        let snapshot = map_snapshot(value).expect("snapshot");
        assert_eq!(snapshot.symbol.as_deref(), Some("US.AAPL"));
        assert_eq!(snapshot.bid_price.expect("bid").to_string(), "189.4");
        assert_eq!(snapshot.ask_price.expect("ask").to_string(), "189.6");
        assert_eq!(snapshot.lot_size, Some(1));
        assert_eq!(snapshot.security_type.as_deref(), Some("EQUITY"));
        assert_eq!(snapshot.pe_rate.as_ref().expect("pe").to_string(), "30");
        assert_eq!(snapshot.pb_rate.as_ref().expect("pb").to_string(), "5");
    }
}
