//! Typed OpenD real-time K-line (Qot_GetKL/3006) query adapter.

use std::collections::BTreeMap;
use std::time::Duration;

use prost::Message;
use thiserror::Error;

use crate::{
    HistoricalKline, HistoricalSecurity, OpenDInitializedSession, OpenDManagedSessionError,
    OpenDSessionCoordinatorError, PROTO_GET_KL, trade_proto::qot_get_kl as wire,
};

pub const GET_KL_TIMEOUT: Duration = Duration::from_millis(900);

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct CurrentKlineQuery {
    pub market: i32,
    pub symbol: String,
    pub period: String,
    pub adjustment: i32,
    pub req_num: i32,
}

impl CurrentKlineQuery {
    pub fn new(market: i32, symbol: impl Into<String>, period: impl Into<String>) -> Self {
        Self {
            market,
            symbol: symbol.into(),
            period: period.into(),
            adjustment: 1, // Default forward rehab (前复权)
            req_num: 2,    // Default 2 (preceding closed + current unclosed)
        }
    }
}

#[derive(Clone, Debug, Default, PartialEq)]
pub struct CurrentKlineResult {
    pub security: HistoricalSecurity,
    pub name: Option<String>,
    pub klines: Vec<HistoricalKline>,
}

#[derive(Debug, Error)]
pub enum CurrentKlineError {
    #[error("OpenD session unavailable: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("OpenD managed session error: {0}")]
    ManagedSession(#[from] OpenDManagedSessionError),
    #[error("decode OpenD Qot_GetKL response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD Qot_GetKL retType={ret_type} errCode={err_code}: {message}")]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD Qot_GetKL response missing s2c")]
    MissingS2c,
}

pub trait CurrentKlineReadPort: Send + Sync + std::fmt::Debug {
    fn query_current(
        &self,
        query: &CurrentKlineQuery,
    ) -> Result<CurrentKlineResult, CurrentKlineError>;
}

pub fn period_to_kl_type(period: &str) -> i32 {
    match period.trim().to_ascii_lowercase().as_str() {
        "1m" => 1,
        "1d" | "day" => 2,
        "1w" | "week" => 3,
        "1mo" | "month" => 4,
        "1y" | "year" => 5,
        "5m" => 6,
        "15m" => 7,
        "30m" => 8,
        "60m" | "1h" => 9,
        "3m" => 10,
        "3mo" | "quarter" => 11,
        "10m" => 12,
        _ => 2,
    }
}

pub fn period_duration_seconds(period: &str) -> i64 {
    match period.trim().to_ascii_lowercase().as_str() {
        "1m" => 60,
        "3m" => 180,
        "5m" => 300,
        "10m" => 600,
        "15m" => 900,
        "30m" => 1800,
        "60m" | "1h" => 3600,
        "120m" => 7200,
        "180m" => 10800,
        "240m" => 14400,
        _ => 0, // Daily, weekly, monthly are not shifted
    }
}

fn parse_opend_datetime(value: &str) -> Option<time::PrimitiveDateTime> {
    let trimmed = if value.len() >= 19 {
        &value[..19]
    } else {
        value
    };
    let format = time::format_description::parse_borrowed::<2>(
        "[year]-[month]-[day] [hour]:[minute]:[second]",
    )
    .ok()?;
    time::PrimitiveDateTime::parse(trimmed, &format).ok()
}

fn format_opend_datetime(dt: time::PrimitiveDateTime) -> Option<String> {
    let format = time::format_description::parse_borrowed::<2>(
        "[year]-[month]-[day] [hour]:[minute]:[second]",
    )
    .ok()?;
    dt.format(&format).ok()
}

/// Normalizes Futu OpenD K-line label time to bucket start time.
/// Intraday bars: labelAt - duration.
/// Daily/weekly/monthly bars: unchanged.
pub fn adjust_kline_time(label_time: &str, period: &str) -> String {
    let duration_secs = period_duration_seconds(period);
    if duration_secs <= 0 {
        return label_time.to_owned();
    }
    if let Some(dt) = parse_opend_datetime(label_time) {
        let adjusted = dt - time::Duration::seconds(duration_secs);
        if let Some(formatted) = format_opend_datetime(adjusted) {
            return formatted;
        }
    }
    label_time.to_owned()
}

/// Decides whether the current unclosed candle should be queried.
/// Returns true if end_time falls within one candle period duration of now.
pub fn should_query_current_kline(
    period: &str,
    end_time: &str,
    now_utc: time::OffsetDateTime,
) -> bool {
    let duration_secs = period_duration_seconds(period);
    let check_duration = if duration_secs > 0 {
        duration_secs
    } else {
        86400 // Daily fallback
    };
    if let Some(dt) = parse_opend_datetime(end_time) {
        let end_utc = dt.assume_utc();
        return end_utc >= now_utc - time::Duration::seconds(check_duration);
    }
    // If end_time is unbounded or far future ("2999-..."), always query
    true
}

/// Merges closed historical candles with real-time candles.
/// Preserves chronological ordering, eliminates blanks, and updates existing
/// candles with latest quotes.
pub fn merge_klines_by_time(
    historical: &[HistoricalKline],
    current: &[HistoricalKline],
) -> Vec<HistoricalKline> {
    let mut map: BTreeMap<String, HistoricalKline> = BTreeMap::new();
    for kline in historical {
        if !kline.is_blank {
            map.insert(kline.time.clone(), kline.clone());
        }
    }
    for kline in current {
        if !kline.is_blank {
            map.insert(kline.time.clone(), kline.clone());
        }
    }
    map.into_values().collect()
}

pub fn encode_get_kl_request(query: &CurrentKlineQuery) -> Vec<u8> {
    wire::Request {
        c2s: wire::C2s {
            rehab_type: query.adjustment,
            kl_type: period_to_kl_type(&query.period),
            security: crate::trade_proto::qot_common::Security {
                market: query.market,
                code: query.symbol.clone(),
            },
            req_num: query.req_num,
            header: None,
        },
    }
    .encode_to_vec()
}

pub fn decode_get_kl_response(
    bytes: &[u8],
    period: &str,
) -> Result<CurrentKlineResult, CurrentKlineError> {
    let response = wire::Response::decode(bytes)?;
    if response.ret_type != 0 {
        return Err(CurrentKlineError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response
                .ret_msg
                .unwrap_or_else(|| "OpenD Qot_GetKL failed".to_owned()),
        });
    }
    let s2c = response.s2c.ok_or(CurrentKlineError::MissingS2c)?;
    let klines = s2c
        .kl_list
        .into_iter()
        .filter(|k| !k.is_blank)
        .map(|k| HistoricalKline {
            time: adjust_kline_time(&k.time, period),
            is_blank: k.is_blank,
            high_price: k.high_price,
            open_price: k.open_price,
            low_price: k.low_price,
            close_price: k.close_price,
            volume: k.volume,
            turnover: k.turnover,
            change_rate: k.change_rate,
        })
        .collect();

    Ok(CurrentKlineResult {
        security: HistoricalSecurity {
            market: s2c.security.market,
            code: s2c.security.code,
        },
        name: s2c.name,
        klines,
    })
}

pub fn query_current_klines(
    session: &OpenDInitializedSession,
    query: &CurrentKlineQuery,
    timeout: Duration,
) -> Result<CurrentKlineResult, CurrentKlineError> {
    let body = encode_get_kl_request(query);
    let bytes = session
        .managed_session()
        .call_with_timeout(PROTO_GET_KL, &body, timeout)?;
    decode_get_kl_response(&bytes, &query.period)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_get_kl_request_encoding() {
        let query = CurrentKlineQuery::new(1, "00700", "1m");
        let bytes = encode_get_kl_request(&query);
        let decoded = wire::Request::decode(bytes.as_slice()).expect("decode request");
        assert_eq!(decoded.c2s.rehab_type, 1);
        assert_eq!(decoded.c2s.kl_type, 1); // 1m -> 1
        assert_eq!(decoded.c2s.req_num, 2);
        assert_eq!(decoded.c2s.security.market, 1);
        assert_eq!(decoded.c2s.security.code, "00700");
    }

    #[test]
    fn test_get_kl_response_decoding_and_timestamp_adjustment() {
        let wire_response = wire::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(wire::S2c {
                security: crate::trade_proto::qot_common::Security {
                    market: 1,
                    code: "00700".to_owned(),
                },
                name: Some("腾讯控股".to_owned()),
                kl_list: vec![
                    crate::trade_proto::qot_common::KLine {
                        time: "2026-09-07 10:30:00".to_owned(),
                        is_blank: false,
                        high_price: Some(380.0),
                        open_price: Some(378.0),
                        low_price: Some(377.5),
                        close_price: Some(379.0),
                        last_close_price: None,
                        volume: Some(1000),
                        turnover: Some(379000.0),
                        turnover_rate: None,
                        pe: None,
                        change_rate: Some(0.5),
                        timestamp: None,
                        hp_volume: None,
                    },
                    crate::trade_proto::qot_common::KLine {
                        time: "2026-09-07 10:31:00".to_owned(),
                        is_blank: false,
                        high_price: Some(381.0),
                        open_price: Some(379.0),
                        low_price: Some(378.8),
                        close_price: Some(380.5),
                        last_close_price: None,
                        volume: Some(1500),
                        turnover: Some(570000.0),
                        turnover_rate: None,
                        pe: None,
                        change_rate: Some(0.8),
                        timestamp: None,
                        hp_volume: None,
                    },
                ],
            }),
        };
        let bytes = wire_response.encode_to_vec();
        let result = decode_get_kl_response(&bytes, "1m").expect("decode result");
        assert_eq!(result.name.as_deref(), Some("腾讯控股"));
        assert_eq!(result.security.market, 1);
        assert_eq!(result.security.code, "00700");
        assert_eq!(result.klines.len(), 2);
        // Intraday 1m timestamps shifted back by 60 seconds
        assert_eq!(result.klines[0].time, "2026-09-07 10:29:00");
        assert_eq!(result.klines[0].close_price, Some(379.0));
        assert_eq!(result.klines[1].time, "2026-09-07 10:30:00");
        assert_eq!(result.klines[1].close_price, Some(380.5));
    }

    #[test]
    fn test_get_kl_response_rejected() {
        let wire_response = wire::Response {
            ret_type: -1,
            ret_msg: Some("subscription quota exceeded".to_owned()),
            err_code: Some(1001),
            s2c: None,
        };
        let bytes = wire_response.encode_to_vec();
        let error = decode_get_kl_response(&bytes, "1m").expect_err("should fail");
        matches!(
            error,
            CurrentKlineError::Rejected {
                ret_type: -1,
                err_code: 1001,
                ..
            }
        );
    }

    #[test]
    fn test_adjust_kline_time_semantics() {
        // 1m: -60s
        assert_eq!(
            adjust_kline_time("2026-09-07 09:31:00", "1m"),
            "2026-09-07 09:30:00"
        );
        // 5m: -300s
        assert_eq!(
            adjust_kline_time("2026-09-07 09:35:00", "5m"),
            "2026-09-07 09:30:00"
        );
        // 1h: -3600s
        assert_eq!(
            adjust_kline_time("2026-09-07 10:30:00", "1h"),
            "2026-09-07 09:30:00"
        );
        // 1d: unshifted
        assert_eq!(
            adjust_kline_time("2026-09-07 00:00:00", "1d"),
            "2026-09-07 00:00:00"
        );
        assert_eq!(adjust_kline_time("2026-09-07", "1d"), "2026-09-07");
    }

    #[test]
    fn test_should_query_current_kline() {
        let now = time::OffsetDateTime::from_unix_timestamp(1788775200).expect("now"); // 2026-09-07 10:00:00 UTC
        // End time exactly at now
        assert!(should_query_current_kline("1m", "2026-09-07 10:00:00", now));
        // End time within 1m
        assert!(should_query_current_kline("1m", "2026-09-07 09:59:30", now));
        // End time 5 minutes ago for 1m period -> false
        assert!(!should_query_current_kline(
            "1m",
            "2026-09-07 09:50:00",
            now
        ));
        // Future end time -> true
        assert!(should_query_current_kline("1m", "2026-09-07 11:00:00", now));
    }

    #[test]
    fn test_merge_klines_by_time() {
        let hist = vec![
            HistoricalKline {
                time: "2026-09-07 09:28:00".to_owned(),
                is_blank: false,
                high_price: Some(10.0),
                open_price: Some(9.0),
                low_price: Some(9.0),
                close_price: Some(10.0),
                volume: Some(100),
                turnover: Some(1000.0),
                change_rate: Some(0.1),
            },
            HistoricalKline {
                time: "2026-09-07 09:29:00".to_owned(),
                is_blank: false,
                high_price: Some(10.5),
                open_price: Some(10.0),
                low_price: Some(9.8),
                close_price: Some(10.2),
                volume: Some(200),
                turnover: Some(2040.0),
                change_rate: Some(0.2),
            },
        ];

        let curr = vec![
            // Updated closed bar from real-time stream
            HistoricalKline {
                time: "2026-09-07 09:29:00".to_owned(),
                is_blank: false,
                high_price: Some(10.6),
                open_price: Some(10.0),
                low_price: Some(9.8),
                close_price: Some(10.5),
                volume: Some(250),
                turnover: Some(2550.0),
                change_rate: Some(0.25),
            },
            // Active unclosed bar
            HistoricalKline {
                time: "2026-09-07 09:30:00".to_owned(),
                is_blank: false,
                high_price: Some(11.0),
                open_price: Some(10.5),
                low_price: Some(10.4),
                close_price: Some(10.9),
                volume: Some(150),
                turnover: Some(1635.0),
                change_rate: Some(0.35),
            },
        ];

        let merged = merge_klines_by_time(&hist, &curr);
        assert_eq!(merged.len(), 3);
        assert_eq!(merged[0].time, "2026-09-07 09:28:00");
        assert_eq!(merged[1].time, "2026-09-07 09:29:00");
        assert_eq!(merged[1].close_price, Some(10.5)); // updated by curr
        assert_eq!(merged[2].time, "2026-09-07 09:30:00");
        assert_eq!(merged[2].close_price, Some(10.9)); // active bar
    }

    #[test]
    fn test_wire_frame_roundtrip() {
        let query = CurrentKlineQuery::new(1, "00700", "5m");
        let req_bytes = encode_get_kl_request(&query);
        let frame = crate::encode_frame(PROTO_GET_KL, 42, &req_bytes).expect("encode frame");
        let decoded_frame = crate::decode_frame(&frame).expect("decode frame");
        assert_eq!(decoded_frame.header.proto_id, PROTO_GET_KL);
        assert_eq!(decoded_frame.header.serial_no, 42);
    }
}
