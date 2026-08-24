use prost::Message;
use thiserror::Error;

use crate::{Frame, PROTO_UPDATE_BASIC_QOT, PROTO_UPDATE_KL, PROTO_UPDATE_ORDER_BOOK};

/// A decoded OpenD quote push. The wire structs stay private so the adapter
/// exposes only the fields that the Go handlers can observe.
#[derive(Clone, Debug, PartialEq)]
pub enum QuotePush {
    Basic(BasicQuotePush),
    Kline(KlinePush),
    OrderBook(OrderBookPush),
}

impl QuotePush {
    pub fn protocol_id(&self) -> u32 {
        match self {
            Self::Basic(_) => PROTO_UPDATE_BASIC_QOT,
            Self::Kline(_) => PROTO_UPDATE_KL,
            Self::OrderBook(_) => PROTO_UPDATE_ORDER_BOOK,
        }
    }
}

#[derive(Clone, Debug, PartialEq)]
pub struct BasicQuotePush {
    pub quotes: Vec<BasicQuote>,
}

#[derive(Clone, Debug, PartialEq)]
pub struct BasicQuote {
    pub security: Option<Security>,
    pub name: Option<String>,
    pub is_suspended: Option<bool>,
    pub list_time: Option<String>,
    pub price_spread: Option<f64>,
    pub update_time: Option<String>,
    pub high_price: Option<f64>,
    pub open_price: Option<f64>,
    pub low_price: Option<f64>,
    pub cur_price: Option<f64>,
    pub last_close_price: Option<f64>,
    pub volume: Option<i64>,
    pub turnover: Option<f64>,
    pub turnover_rate: Option<f64>,
    pub amplitude: Option<f64>,
    pub dark_status: Option<i32>,
    pub list_timestamp: Option<f64>,
    pub update_timestamp: Option<f64>,
    pub pre_market: Option<PreAfterMarketData>,
    pub after_market: Option<PreAfterMarketData>,
    pub sec_status: Option<i32>,
    pub overnight: Option<PreAfterMarketData>,
    pub hp_volume: Option<f64>,
}

#[derive(Clone, Debug, PartialEq)]
pub struct KlinePush {
    pub rehab_type: Option<i32>,
    pub kl_type: Option<i32>,
    pub security: Option<Security>,
    pub name: Option<String>,
    pub klines: Vec<Kline>,
}

#[derive(Clone, Debug, PartialEq)]
pub struct Kline {
    pub time: Option<String>,
    pub is_blank: Option<bool>,
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

#[derive(Clone, Debug, PartialEq)]
pub struct OrderBookPush {
    pub security: Option<Security>,
    pub name: Option<String>,
    pub asks: Vec<OrderBookLevel>,
    pub bids: Vec<OrderBookLevel>,
    pub server_receive_time_bid: Option<String>,
    pub server_receive_time_bid_timestamp: Option<f64>,
    pub server_receive_time_ask: Option<String>,
    pub server_receive_time_ask_timestamp: Option<f64>,
    pub order_book_type: Option<i32>,
}

#[derive(Clone, Debug, PartialEq)]
pub struct OrderBookLevel {
    pub price: Option<f64>,
    pub volume: Option<i64>,
    pub order_count: Option<i32>,
    pub details: Vec<OrderBookDetail>,
    pub high_precision_volume: Option<f64>,
}

#[derive(Clone, Debug, PartialEq)]
pub struct OrderBookDetail {
    pub order_id: Option<i64>,
    pub volume: Option<i64>,
}

#[derive(Clone, Debug, PartialEq)]
pub struct Security {
    pub market: Option<i32>,
    pub code: Option<String>,
}

#[derive(Clone, Debug, PartialEq)]
pub struct PreAfterMarketData {
    pub price: Option<f64>,
    pub high_price: Option<f64>,
    pub low_price: Option<f64>,
    pub volume: Option<i64>,
    pub turnover: Option<f64>,
    pub change_value: Option<f64>,
    pub change_rate: Option<f64>,
    pub amplitude: Option<f64>,
}

#[derive(Debug, Error)]
pub enum QuotePushDecodeError {
    #[error("decode OpenD quote push proto {protocol}: {source}")]
    Decode {
        protocol: u32,
        #[source]
        source: prost::DecodeError,
    },
}

/// Decode one unsolicited OpenD frame.
///
/// Unknown protocols and Go-compatible rejected/empty responses return
/// Ok(None). Malformed protobuf is returned as an error so the lifecycle can
/// record a stream failure and trigger its existing recovery fence.
pub fn decode_quote_push(frame: &Frame) -> Result<Option<QuotePush>, QuotePushDecodeError> {
    match frame.header.proto_id {
        PROTO_UPDATE_BASIC_QOT => {
            let response = BasicQuoteResponse::decode(frame.body.as_slice()).map_err(|source| {
                QuotePushDecodeError::Decode {
                    protocol: PROTO_UPDATE_BASIC_QOT,
                    source,
                }
            })?;
            if !response.is_success() {
                return Ok(None);
            }
            let Some(s2c) = response.s2c else {
                return Ok(None);
            };
            if !s2c.is_complete() {
                return Ok(None);
            }
            Ok(Some(QuotePush::Basic(BasicQuotePush {
                quotes: s2c
                    .basic_quotes
                    .into_iter()
                    .map(BasicQuote::from_wire)
                    .collect(),
            })))
        }
        PROTO_UPDATE_KL => {
            let response = KlineResponse::decode(frame.body.as_slice()).map_err(|source| {
                QuotePushDecodeError::Decode {
                    protocol: PROTO_UPDATE_KL,
                    source,
                }
            })?;
            if !response.is_success() {
                return Ok(None);
            }
            let Some(s2c) = response.s2c else {
                return Ok(None);
            };
            if !s2c.is_complete() {
                return Ok(None);
            }
            Ok(Some(QuotePush::Kline(KlinePush {
                rehab_type: s2c.rehab_type,
                kl_type: s2c.kl_type,
                security: s2c.security.map(Security::from_wire),
                name: s2c.name,
                klines: s2c.klines.into_iter().map(Kline::from_wire).collect(),
            })))
        }
        PROTO_UPDATE_ORDER_BOOK => {
            let response = OrderBookResponse::decode(frame.body.as_slice()).map_err(|source| {
                QuotePushDecodeError::Decode {
                    protocol: PROTO_UPDATE_ORDER_BOOK,
                    source,
                }
            })?;
            if !response.is_success() {
                return Ok(None);
            }
            let Some(s2c) = response.s2c else {
                return Ok(None);
            };
            if !s2c.is_complete() {
                return Ok(None);
            }
            Ok(Some(QuotePush::OrderBook(OrderBookPush {
                security: s2c.security.map(Security::from_wire),
                name: s2c.name,
                asks: s2c
                    .asks
                    .into_iter()
                    .map(OrderBookLevel::from_wire)
                    .collect(),
                bids: s2c
                    .bids
                    .into_iter()
                    .map(OrderBookLevel::from_wire)
                    .collect(),
                server_receive_time_bid: s2c.server_receive_time_bid,
                server_receive_time_bid_timestamp: s2c.server_receive_time_bid_timestamp,
                server_receive_time_ask: s2c.server_receive_time_ask,
                server_receive_time_ask_timestamp: s2c.server_receive_time_ask_timestamp,
                order_book_type: s2c.order_book_type,
            })))
        }
        _ => Ok(None),
    }
}

impl BasicQuote {
    fn from_wire(value: WireBasicQuote) -> Self {
        Self {
            security: value.security.map(Security::from_wire),
            name: value.name,
            is_suspended: value.is_suspended,
            list_time: value.list_time,
            price_spread: value.price_spread,
            update_time: value.update_time,
            high_price: value.high_price,
            open_price: value.open_price,
            low_price: value.low_price,
            cur_price: value.cur_price,
            last_close_price: value.last_close_price,
            volume: value.volume,
            turnover: value.turnover,
            turnover_rate: value.turnover_rate,
            amplitude: value.amplitude,
            dark_status: value.dark_status,
            list_timestamp: value.list_timestamp,
            update_timestamp: value.update_timestamp,
            pre_market: value.pre_market.map(PreAfterMarketData::from_wire),
            after_market: value.after_market.map(PreAfterMarketData::from_wire),
            sec_status: value.sec_status,
            overnight: value.overnight.map(PreAfterMarketData::from_wire),
            hp_volume: value.hp_volume,
        }
    }
}

impl Kline {
    fn from_wire(value: WireKline) -> Self {
        Self {
            time: value.time,
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
}

impl OrderBookLevel {
    fn from_wire(value: WireOrderBook) -> Self {
        Self {
            price: value.price,
            volume: value.volume,
            order_count: value.order_count,
            details: value
                .details
                .into_iter()
                .map(|detail| OrderBookDetail {
                    order_id: detail.order_id,
                    volume: detail.volume,
                })
                .collect(),
            high_precision_volume: value.high_precision_volume,
        }
    }
}

impl Security {
    fn from_wire(value: WireSecurity) -> Self {
        Self {
            market: value.market,
            code: value.code,
        }
    }
}

impl PreAfterMarketData {
    fn from_wire(value: WirePreAfterMarketData) -> Self {
        Self {
            price: value.price,
            high_price: value.high_price,
            low_price: value.low_price,
            volume: value.volume,
            turnover: value.turnover,
            change_value: value.change_value,
            change_rate: value.change_rate,
            amplitude: value.amplitude,
        }
    }
}

trait ResponseStatus {
    fn ret_type(&self) -> Option<i32>;
    fn has_s2c(&self) -> bool;

    fn is_success(&self) -> bool {
        self.ret_type().unwrap_or(-400) == 0 && self.has_s2c()
    }
}

#[derive(Clone, PartialEq, Message)]
struct BasicQuoteResponse {
    #[prost(int32, optional, tag = "1")]
    ret_type: Option<i32>,
    #[prost(string, optional, tag = "2")]
    _ret_msg: Option<String>,
    #[prost(int32, optional, tag = "3")]
    _err_code: Option<i32>,
    #[prost(message, optional, tag = "4")]
    s2c: Option<BasicQuoteS2c>,
}

impl ResponseStatus for BasicQuoteResponse {
    fn ret_type(&self) -> Option<i32> {
        self.ret_type
    }

    fn has_s2c(&self) -> bool {
        self.s2c.is_some()
    }
}

#[derive(Clone, PartialEq, Message)]
struct BasicQuoteS2c {
    #[prost(message, repeated, tag = "1")]
    basic_quotes: Vec<WireBasicQuote>,
}

impl BasicQuoteS2c {
    fn is_complete(&self) -> bool {
        self.basic_quotes.iter().all(WireBasicQuote::is_complete)
    }
}

#[derive(Clone, PartialEq, Message)]
struct KlineResponse {
    #[prost(int32, optional, tag = "1")]
    ret_type: Option<i32>,
    #[prost(string, optional, tag = "2")]
    _ret_msg: Option<String>,
    #[prost(int32, optional, tag = "3")]
    _err_code: Option<i32>,
    #[prost(message, optional, tag = "4")]
    s2c: Option<KlineS2c>,
}

impl ResponseStatus for KlineResponse {
    fn ret_type(&self) -> Option<i32> {
        self.ret_type
    }

    fn has_s2c(&self) -> bool {
        self.s2c.is_some()
    }
}

#[derive(Clone, PartialEq, Message)]
struct KlineS2c {
    #[prost(int32, optional, tag = "1")]
    rehab_type: Option<i32>,
    #[prost(int32, optional, tag = "2")]
    kl_type: Option<i32>,
    #[prost(message, optional, tag = "3")]
    security: Option<WireSecurity>,
    #[prost(message, repeated, tag = "4")]
    klines: Vec<WireKline>,
    #[prost(string, optional, tag = "5")]
    name: Option<String>,
}

impl KlineS2c {
    fn is_complete(&self) -> bool {
        self.rehab_type.is_some()
            && self.kl_type.is_some()
            && self
                .security
                .as_ref()
                .is_some_and(WireSecurity::is_complete)
            && self.klines.iter().all(WireKline::is_complete)
    }
}

#[derive(Clone, PartialEq, Message)]
struct OrderBookResponse {
    #[prost(int32, optional, tag = "1")]
    ret_type: Option<i32>,
    #[prost(string, optional, tag = "2")]
    _ret_msg: Option<String>,
    #[prost(int32, optional, tag = "3")]
    _err_code: Option<i32>,
    #[prost(message, optional, tag = "4")]
    s2c: Option<OrderBookS2c>,
}

impl ResponseStatus for OrderBookResponse {
    fn ret_type(&self) -> Option<i32> {
        self.ret_type
    }

    fn has_s2c(&self) -> bool {
        self.s2c.is_some()
    }
}

#[derive(Clone, PartialEq, Message)]
struct OrderBookS2c {
    #[prost(message, optional, tag = "1")]
    security: Option<WireSecurity>,
    #[prost(message, repeated, tag = "2")]
    asks: Vec<WireOrderBook>,
    #[prost(message, repeated, tag = "3")]
    bids: Vec<WireOrderBook>,
    #[prost(string, optional, tag = "4")]
    server_receive_time_bid: Option<String>,
    #[prost(double, optional, tag = "5")]
    server_receive_time_bid_timestamp: Option<f64>,
    #[prost(string, optional, tag = "6")]
    server_receive_time_ask: Option<String>,
    #[prost(double, optional, tag = "7")]
    server_receive_time_ask_timestamp: Option<f64>,
    #[prost(string, optional, tag = "8")]
    name: Option<String>,
    #[prost(int32, optional, tag = "9")]
    order_book_type: Option<i32>,
}

impl OrderBookS2c {
    fn is_complete(&self) -> bool {
        self.security
            .as_ref()
            .is_some_and(WireSecurity::is_complete)
            && self.asks.iter().all(WireOrderBook::is_complete)
            && self.bids.iter().all(WireOrderBook::is_complete)
    }
}

#[derive(Clone, PartialEq, Message)]
struct WireSecurity {
    #[prost(int32, optional, tag = "1")]
    market: Option<i32>,
    #[prost(string, optional, tag = "2")]
    code: Option<String>,
}

impl WireSecurity {
    fn is_complete(&self) -> bool {
        self.market.is_some() && self.code.is_some()
    }
}

#[derive(Clone, PartialEq, Message)]
struct WireKline {
    #[prost(string, optional, tag = "1")]
    time: Option<String>,
    #[prost(bool, optional, tag = "2")]
    is_blank: Option<bool>,
    #[prost(double, optional, tag = "3")]
    high_price: Option<f64>,
    #[prost(double, optional, tag = "4")]
    open_price: Option<f64>,
    #[prost(double, optional, tag = "5")]
    low_price: Option<f64>,
    #[prost(double, optional, tag = "6")]
    close_price: Option<f64>,
    #[prost(double, optional, tag = "7")]
    last_close_price: Option<f64>,
    #[prost(int64, optional, tag = "8")]
    volume: Option<i64>,
    #[prost(double, optional, tag = "9")]
    turnover: Option<f64>,
    #[prost(double, optional, tag = "10")]
    turnover_rate: Option<f64>,
    #[prost(double, optional, tag = "11")]
    pe: Option<f64>,
    #[prost(double, optional, tag = "12")]
    change_rate: Option<f64>,
    #[prost(double, optional, tag = "13")]
    timestamp: Option<f64>,
    #[prost(double, optional, tag = "14")]
    hp_volume: Option<f64>,
}

impl WireKline {
    fn is_complete(&self) -> bool {
        self.time.is_some() && self.is_blank.is_some()
    }
}

#[derive(Clone, PartialEq, Message)]
struct WirePreAfterMarketData {
    #[prost(double, optional, tag = "1")]
    price: Option<f64>,
    #[prost(double, optional, tag = "2")]
    high_price: Option<f64>,
    #[prost(double, optional, tag = "3")]
    low_price: Option<f64>,
    #[prost(int64, optional, tag = "4")]
    volume: Option<i64>,
    #[prost(double, optional, tag = "5")]
    turnover: Option<f64>,
    #[prost(double, optional, tag = "6")]
    change_value: Option<f64>,
    #[prost(double, optional, tag = "7")]
    change_rate: Option<f64>,
    #[prost(double, optional, tag = "8")]
    amplitude: Option<f64>,
}

#[derive(Clone, PartialEq, Message)]
struct WireBasicQuote {
    #[prost(message, optional, tag = "1")]
    security: Option<WireSecurity>,
    #[prost(bool, optional, tag = "2")]
    is_suspended: Option<bool>,
    #[prost(string, optional, tag = "3")]
    list_time: Option<String>,
    #[prost(double, optional, tag = "4")]
    price_spread: Option<f64>,
    #[prost(string, optional, tag = "5")]
    update_time: Option<String>,
    #[prost(double, optional, tag = "6")]
    high_price: Option<f64>,
    #[prost(double, optional, tag = "7")]
    open_price: Option<f64>,
    #[prost(double, optional, tag = "8")]
    low_price: Option<f64>,
    #[prost(double, optional, tag = "9")]
    cur_price: Option<f64>,
    #[prost(double, optional, tag = "10")]
    last_close_price: Option<f64>,
    #[prost(int64, optional, tag = "11")]
    volume: Option<i64>,
    #[prost(double, optional, tag = "12")]
    turnover: Option<f64>,
    #[prost(double, optional, tag = "13")]
    turnover_rate: Option<f64>,
    #[prost(double, optional, tag = "14")]
    amplitude: Option<f64>,
    #[prost(int32, optional, tag = "15")]
    dark_status: Option<i32>,
    #[prost(double, optional, tag = "17")]
    list_timestamp: Option<f64>,
    #[prost(double, optional, tag = "18")]
    update_timestamp: Option<f64>,
    #[prost(message, optional, tag = "19")]
    pre_market: Option<WirePreAfterMarketData>,
    #[prost(message, optional, tag = "20")]
    after_market: Option<WirePreAfterMarketData>,
    #[prost(int32, optional, tag = "21")]
    sec_status: Option<i32>,
    #[prost(message, optional, tag = "25")]
    overnight: Option<WirePreAfterMarketData>,
    #[prost(double, optional, tag = "26")]
    hp_volume: Option<f64>,
    #[prost(string, optional, tag = "24")]
    name: Option<String>,
}

impl WireBasicQuote {
    fn is_complete(&self) -> bool {
        self.security
            .as_ref()
            .is_some_and(WireSecurity::is_complete)
            && self.is_suspended.is_some()
            && self.list_time.is_some()
            && self.price_spread.is_some()
            && self.update_time.is_some()
            && self.high_price.is_some()
            && self.open_price.is_some()
            && self.low_price.is_some()
            && self.cur_price.is_some()
            && self.last_close_price.is_some()
            && self.volume.is_some()
            && self.turnover.is_some()
            && self.turnover_rate.is_some()
            && self.amplitude.is_some()
    }
}

#[derive(Clone, PartialEq, Message)]
struct WireOrderBook {
    #[prost(double, optional, tag = "1")]
    price: Option<f64>,
    #[prost(int64, optional, tag = "2")]
    volume: Option<i64>,
    #[prost(int32, optional, tag = "3")]
    order_count: Option<i32>,
    #[prost(message, repeated, tag = "4")]
    details: Vec<WireOrderBookDetail>,
    #[prost(double, optional, tag = "5")]
    high_precision_volume: Option<f64>,
}

impl WireOrderBook {
    fn is_complete(&self) -> bool {
        self.price.is_some()
            && self.volume.is_some()
            && self.order_count.is_some()
            && self.details.iter().all(WireOrderBookDetail::is_complete)
    }
}

#[derive(Clone, PartialEq, Message)]
struct WireOrderBookDetail {
    #[prost(int64, optional, tag = "1")]
    order_id: Option<i64>,
    #[prost(int64, optional, tag = "2")]
    volume: Option<i64>,
}

impl WireOrderBookDetail {
    fn is_complete(&self) -> bool {
        self.order_id.is_some() && self.volume.is_some()
    }
}

#[cfg(test)]
#[path = "quote_push_tests.rs"]
mod tests;
