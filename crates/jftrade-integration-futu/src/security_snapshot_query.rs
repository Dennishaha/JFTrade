//! Typed Qot_GetSecuritySnapshot (3203) reader.

use std::str::FromStr;
use std::sync::{Arc, Mutex};
use std::time::Duration;

use jftrade_kernel::{Decimal, DecimalText};
use jftrade_marketdata::BrokerSecuritySnapshot;
use prost::Message;
use thiserror::Error;

use crate::{
    OpenDManagedSessionError, OpenDSessionCoordinator, OpenDSessionCoordinatorError,
    PROTO_GET_SECURITY_SNAPSHOT, trade_proto::qot_get_security_snapshot as wire,
};

const SNAPSHOT_TIMEOUT: Duration = Duration::from_millis(900);

pub trait SecuritySnapshotReadPort: Send + Sync {
    fn query(&self, instruments: &[String]) -> Result<Vec<BrokerSecuritySnapshot>, String>;
}

#[derive(Clone)]
pub struct OpenDSecuritySnapshotReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDSecuritySnapshotReader {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("OpenDSecuritySnapshotReader")
            .finish_non_exhaustive()
    }
}

impl OpenDSecuritySnapshotReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }

    fn query_raw_securities(
        &self,
        securities: &[crate::trade_proto::qot_common::Security],
    ) -> Result<Vec<BrokerSecuritySnapshot>, SecuritySnapshotQueryError> {
        let body = wire::Request {
            c2s: wire::C2s {
                security_list: securities.to_vec(),
                header: None,
            },
        }
        .encode_to_vec();
        let coordinator = self.coordinator.lock().map_err(|_| {
            SecuritySnapshotQueryError::Session("coordinator lock poisoned".to_owned())
        })?;
        let session = coordinator.session()?;
        let bytes = session.managed_session().call_with_timeout(
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

    pub fn query(
        &self,
        instruments: &[String],
    ) -> Result<Vec<BrokerSecuritySnapshot>, SecuritySnapshotQueryError> {
        if instruments.is_empty() {
            return Ok(Vec::new());
        }
        let mut securities = Vec::with_capacity(instruments.len());
        let mut first_invalid = None;
        for value in instruments {
            match value.split_once('.').and_then(|(market, code)| {
                let m = market_code(market)?;
                let c = code.trim().to_ascii_uppercase();
                if c.is_empty() { None } else { Some((m, c)) }
            }) {
                Some((market, code)) => {
                    securities.push(crate::trade_proto::qot_common::Security { market, code });
                }
                None => {
                    if first_invalid.is_none() {
                        first_invalid =
                            Some(SecuritySnapshotQueryError::InvalidInstrument(value.clone()));
                    }
                }
            }
        }
        if securities.is_empty() {
            if let Some(err) = first_invalid {
                return Err(err);
            }
            return Ok(Vec::new());
        }

        let mut by_market: std::collections::BTreeMap<
            i32,
            Vec<crate::trade_proto::qot_common::Security>,
        > = std::collections::BTreeMap::new();
        for sec in securities {
            by_market.entry(sec.market).or_default().push(sec);
        }

        let mut all_snapshots = Vec::new();
        let mut last_error = None;

        for (_market, list) in by_market {
            for chunk in list.chunks(20) {
                match self.query_raw_securities(chunk) {
                    Ok(snaps) => all_snapshots.extend(snaps),
                    Err(err) => {
                        let mut chunk_succeeded = false;
                        if chunk.len() > 1 {
                            for single in chunk {
                                if let Ok(snaps) =
                                    self.query_raw_securities(std::slice::from_ref(single))
                                    && !snaps.is_empty()
                                {
                                    chunk_succeeded = true;
                                    all_snapshots.extend(snaps);
                                }
                            }
                        }
                        if !chunk_succeeded {
                            last_error = Some(err);
                        }
                    }
                }
            }
        }

        if all_snapshots.is_empty()
            && let Some(err) = last_error.or(first_invalid)
        {
            return Err(err);
        }

        Ok(all_snapshots)
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
    Session(String),
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

impl From<OpenDManagedSessionError> for SecuritySnapshotQueryError {
    fn from(error: OpenDManagedSessionError) -> Self {
        Self::Session(error.to_string())
    }
}

impl From<OpenDSessionCoordinatorError> for SecuritySnapshotQueryError {
    fn from(error: OpenDSessionCoordinatorError) -> Self {
        Self::Session(error.to_string())
    }
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
        bid_price: optional_price(basic.bid_price),
        ask_price: optional_price(basic.ask_price),
        last_price: optional_price(Some(basic.cur_price)),
        volume: DecimalText::from_str(&basic.volume.to_string()).ok(),
        lot_size: Some(basic.lot_size),
        security_type: Some(security_type(basic.r#type).to_owned()),
        open_price: optional_price(Some(basic.open_price)),
        high_price: optional_price(Some(basic.high_price)),
        low_price: optional_price(Some(basic.low_price)),
        previous_close: optional_price(Some(basic.last_close_price)),
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
        ..Default::default()
    })
}

fn optional_price(value: Option<f64>) -> Option<Decimal> {
    value
        .filter(|v| v.is_finite())
        .and_then(|v| Decimal::from_str(&v.to_string()).ok())
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
        "SH" | "CN" => Some(21),
        "SZ" => Some(22),
        "SG" => Some(31),
        "JP" => Some(41),
        "AU" => Some(51),
        "MY" => Some(61),
        "CA" => Some(71),
        _ => None,
    }
}
fn market_label(value: i32) -> Option<&'static str> {
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
fn security_type(value: i32) -> &'static str {
    match value {
        1 => "BOND",
        2 => "BWRT",
        3 => "EQUITY",
        4 => "TRUST",
        5 => "WARRANT",
        6 => "INDEX",
        7 => "PLATE",
        8 => "OPTION",
        10 => "FUTURE",
        _ => "UNKNOWN",
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::trade_proto::qot_common::Security;
    use crate::{Frame, OpenDTcpProbeConfig, decode_frame, encode_frame};
    use prost::Message;
    use std::io::{Read, Write};
    use std::net::{TcpListener, TcpStream};
    use std::time::Duration;

    #[derive(Clone, PartialEq, Message)]
    struct InitResponse {
        #[prost(int32, optional, tag = "1")]
        ret_type: Option<i32>,
        #[prost(message, optional, tag = "4")]
        s2c: Option<InitState>,
    }
    #[derive(Clone, PartialEq, Message)]
    struct InitState {
        #[prost(int32, tag = "1")]
        server_ver: i32,
        #[prost(uint64, tag = "3")]
        conn_id: u64,
    }

    fn read_frame(stream: &mut TcpStream) -> Frame {
        let mut header = [0u8; 44];
        stream.read_exact(&mut header).expect("header");
        let body_len = u32::from_le_bytes(header[12..16].try_into().expect("length")) as usize;
        let mut packet = vec![0u8; 44 + body_len];
        packet[..44].copy_from_slice(&header);
        stream.read_exact(&mut packet[44..]).expect("body");
        decode_frame(&packet).expect("frame")
    }

    #[test]
    fn maps_security_snapshot_bbo_and_equity_metrics_without_defaults() {
        let value = wire::Snapshot {
            basic: wire::SnapshotBasicData {
                security: Security {
                    market: 11,
                    code: "AAPL".to_owned(),
                },
                name: Some("Apple Inc.".to_owned()),
                r#type: 3,
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

    #[test]
    fn framed_opend_security_snapshot_request_preserves_bbo_fields() {
        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind");
        let address = listener.local_addr().expect("address");
        let server = std::thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept");
            let init = read_frame(&mut stream);
            stream
                .write_all(
                    &encode_frame(
                        init.header.proto_id,
                        init.header.serial_no,
                        &InitResponse {
                            ret_type: Some(0),
                            s2c: Some(InitState {
                                server_ver: 1009,
                                conn_id: 7,
                            }),
                        }
                        .encode_to_vec(),
                    )
                    .expect("init frame"),
                )
                .expect("init response");
            let request = read_frame(&mut stream);
            assert_eq!(request.header.proto_id, PROTO_GET_SECURITY_SNAPSHOT);
            let response = wire::Response {
                ret_type: 0,
                ret_msg: None,
                err_code: None,
                s2c: Some(wire::S2c {
                    snapshot_list: vec![wire::Snapshot {
                        basic: wire::SnapshotBasicData {
                            security: Security {
                                market: 11,
                                code: "AAPL".to_owned(),
                            },
                            name: Some("Apple Inc.".to_owned()),
                            r#type: 3,
                            is_suspend: false,
                            list_time: "1980".to_owned(),
                            lot_size: 1,
                            price_spread: 0.01,
                            update_time: "09:30:00".to_owned(),
                            high_price: 2.0,
                            open_price: 1.0,
                            low_price: 0.5,
                            last_close_price: 1.5,
                            cur_price: 1.8,
                            volume: 10,
                            turnover: 18.0,
                            turnover_rate: 0.1,
                            list_timestamp: None,
                            update_timestamp: None,
                            ask_price: Some(1.9),
                            bid_price: Some(1.7),
                            ask_vol: None,
                            bid_vol: None,
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
                        ..Default::default()
                    }],
                }),
            };
            stream
                .write_all(
                    &encode_frame(
                        request.header.proto_id,
                        request.header.serial_no,
                        &response.encode_to_vec(),
                    )
                    .expect("response frame"),
                )
                .expect("response");
        });
        let coordinator = Arc::new(Mutex::new(
            OpenDSessionCoordinator::connect(
                OpenDTcpProbeConfig::new(address, Duration::from_secs(1)),
                Arc::new(jftrade_marketdata::MarketDataRuntimeRecorder::default()),
                Vec::new(),
                0,
            )
            .expect("coordinator"),
        ));
        let snapshots = OpenDSecuritySnapshotReader::new(coordinator)
            .query(&["US.AAPL".to_owned()])
            .expect("snapshot query");
        assert_eq!(snapshots[0].bid_price.expect("bid").to_string(), "1.7");
        assert_eq!(snapshots[0].ask_price.expect("ask").to_string(), "1.9");
        server.join().expect("server");
    }

    #[test]
    fn security_type_mapping_matches_futu_proto_definitions() {
        assert_eq!(security_type(1), "BOND");
        assert_eq!(security_type(2), "BWRT");
        assert_eq!(security_type(3), "EQUITY");
        assert_eq!(security_type(4), "TRUST");
        assert_eq!(security_type(5), "WARRANT");
        assert_eq!(security_type(6), "INDEX");
        assert_eq!(security_type(7), "PLATE");
        assert_eq!(security_type(8), "OPTION");
        assert_eq!(security_type(10), "FUTURE");
        assert_eq!(security_type(99), "UNKNOWN");
    }

    #[test]
    fn market_code_and_label_supports_all_standard_markets() {
        assert_eq!(market_code("HK"), Some(1));
        assert_eq!(market_code("US"), Some(11));
        assert_eq!(market_code("SH"), Some(21));
        assert_eq!(market_code("CN"), Some(21));
        assert_eq!(market_code("SZ"), Some(22));
        assert_eq!(market_code("SG"), Some(31));
        assert_eq!(market_code("JP"), Some(41));
        assert_eq!(market_code("AU"), Some(51));
        assert_eq!(market_code("MY"), Some(61));
        assert_eq!(market_code("CA"), Some(71));
        assert_eq!(market_code("UNKNOWN"), None);

        assert_eq!(market_label(1), Some("HK"));
        assert_eq!(market_label(11), Some("US"));
        assert_eq!(market_label(21), Some("SH"));
        assert_eq!(market_label(22), Some("SZ"));
        assert_eq!(market_label(31), Some("SG"));
        assert_eq!(market_label(41), Some("JP"));
        assert_eq!(market_label(51), Some("AU"));
        assert_eq!(market_label(61), Some("MY"));
        assert_eq!(market_label(71), Some("CA"));
        assert_eq!(market_label(999), None);
    }
}
