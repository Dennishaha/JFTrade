//! Typed OpenD event-contract push decoding and listener dispatch.
//!
//! Go registers three `Qot_UpdateEventContract*` subscribers on each OpenD
//! client (protocols 3450/3451/3452) and forwards every accepted `S2C` into the
//! adapter's `predictionStreamListeners` map. Rust keeps the same boundary
//! here: decode one unsolicited frame into typed rows, project the identity and
//! sequence the public update contract needs, and dispatch it to the listeners
//! registered at that moment. Unknown protocols, rejected responses, responses
//! without `s2c` and malformed bodies never reach a listener.

use std::collections::BTreeMap;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex};

use prost::Message;
use serde_json::{Value, json};

use crate::frame::Frame;
use crate::quote_push::QuotePushDecodeError;
use crate::trade_proto;

/// Which prediction stream a push belongs to. Go carries the same label as a
/// plain string (`ORDER_BOOK`/`KLINE`/`TICKER`); the enum keeps the Rust cache
/// and listener keys from inventing their own spellings.
#[derive(Clone, Copy, Debug, Eq, Ord, PartialEq, PartialOrd)]
pub enum PredictionDataType {
    OrderBook,
    Kline,
    Ticker,
}

impl PredictionDataType {
    pub fn label(self) -> &'static str {
        match self {
            Self::OrderBook => "ORDER_BOOK",
            Self::Kline => "KLINE",
            Self::Ticker => "TICKER",
        }
    }

    /// The `Qot_UpdateEventContract*` protocol that carries this stream.
    pub fn protocol_id(self) -> u32 {
        match self {
            Self::OrderBook => trade_proto::qot_update_event_contract_order_book::PROTOCOL_ID,
            Self::Kline => trade_proto::qot_update_event_contract_kline::PROTOCOL_ID,
            Self::Ticker => trade_proto::qot_update_event_contract_ticker::PROTOCOL_ID,
        }
    }
}

/// One delivered push row: the contract identity, the sequence Go derives from
/// the last nested point, and the projected entry itself.
#[derive(Clone, Debug, PartialEq)]
pub struct PredictionPushRow {
    pub instrument_id: String,
    pub data_type: PredictionDataType,
    pub sequence: String,
    pub entry: Value,
}

/// A registered prediction push listener, i.e. Go's
/// `func(broker.PredictionMarketUpdate)`.
pub type PredictionPushListener = Arc<dyn Fn(&PredictionPushRow) + Send + Sync>;

/// Listener registry for the event-contract push stream.
///
/// Go keeps `predictionStreamListeners` on the adapter keyed by a monotonic id
/// and hands back a `remove()` closure; a nil handler returns a no-op closure
/// and installs nothing so it cannot break a later registration. The same
/// semantics are kept here without borrowing the adapter.
#[derive(Clone, Default)]
pub struct PredictionPushRegistry {
    inner: Arc<PredictionPushRegistryInner>,
}

#[derive(Default)]
struct PredictionPushRegistryInner {
    next_id: AtomicU64,
    listeners: Mutex<BTreeMap<u64, PredictionPushListener>>,
}

impl std::fmt::Debug for PredictionPushRegistry {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("PredictionPushRegistry")
            .field("listener_count", &self.listener_count())
            .finish()
    }
}

impl PredictionPushRegistry {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn listener_count(&self) -> usize {
        self.inner
            .listeners
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .len()
    }

    /// Register one listener and return its removal handle.
    pub fn subscribe(&self, listener: PredictionPushListener) -> PredictionPushUnsubscribe {
        let id = self.inner.next_id.fetch_add(1, Ordering::SeqCst);
        self.inner
            .listeners
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .insert(id, listener);
        PredictionPushUnsubscribe {
            inner: Arc::clone(&self.inner),
            id,
        }
    }

    /// Deliver one decoded push to every listener registered right now.
    ///
    /// The listener set is snapshotted before dispatch so a listener that
    /// unregisters (or registers) another listener cannot change the fan-out of
    /// the push already being delivered.
    pub fn dispatch(&self, rows: &[PredictionPushRow]) {
        if rows.is_empty() {
            return;
        }
        let listeners = self
            .inner
            .listeners
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .values()
            .cloned()
            .collect::<Vec<_>>();
        for row in rows {
            for listener in &listeners {
                listener(row);
            }
        }
    }
}

/// Removal handle returned by [`PredictionPushRegistry::subscribe`]. Dropping
/// it unregisters the listener, mirroring Go's `remove()` closure being called.
///
/// Go hands back a closure the caller invokes explicitly; Rust cannot express
/// the same call syntax on an owned handle, so removal happens when the handle
/// is dropped. [`PredictionPushUnsubscribe::unsubscribe`] exists for callers
/// that want to make the removal point explicit, and removal is idempotent.
pub struct PredictionPushUnsubscribe {
    inner: Arc<PredictionPushRegistryInner>,
    id: u64,
}

impl PredictionPushUnsubscribe {
    pub fn unsubscribe(self) {}
}

impl std::fmt::Debug for PredictionPushUnsubscribe {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("PredictionPushUnsubscribe")
            .field("id", &self.id)
            .finish()
    }
}

impl Drop for PredictionPushUnsubscribe {
    fn drop(&mut self) {
        self.inner
            .listeners
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .remove(&self.id);
    }
}

/// Decode one unsolicited OpenD frame into projected prediction rows.
///
/// `Ok(None)` covers every Go-compatible drop: a non-prediction protocol, a
/// non-zero `retType` or a response without `s2c` (`subscribePredictionPush`
/// checks exactly those three). Malformed protobuf is reported as `Err` so a
/// direct caller sees the damage while the session pump can drop it, matching
/// the existing quote-push contract.
pub fn decode_prediction_push(
    frame: &Frame,
) -> Result<Option<(PredictionDataType, Vec<PredictionPushRow>)>, QuotePushDecodeError> {
    let protocol = frame.header.proto_id;
    if protocol == trade_proto::qot_update_event_contract_order_book::PROTOCOL_ID {
        use trade_proto::qot_get_event_contract_order_book as wire;
        let Some(s2c) = accepted_s2c::<wire::Response>(protocol, &frame.body)? else {
            return Ok(None);
        };
        return Ok(Some(project(
            PredictionDataType::OrderBook,
            s2c.order_book_list
                .into_iter()
                .map(order_book_row)
                .collect(),
        )));
    }
    if protocol == trade_proto::qot_update_event_contract_kline::PROTOCOL_ID {
        use trade_proto::qot_get_event_contract_kline as wire;
        let Some(s2c) = accepted_s2c::<wire::Response>(protocol, &frame.body)? else {
            return Ok(None);
        };
        return Ok(Some(project(
            PredictionDataType::Kline,
            s2c.kline_list.into_iter().map(kline_row).collect(),
        )));
    }
    if protocol == trade_proto::qot_update_event_contract_ticker::PROTOCOL_ID {
        use trade_proto::qot_get_event_contract_ticker as wire;
        let Some(s2c) = accepted_s2c::<wire::Response>(protocol, &frame.body)? else {
            return Ok(None);
        };
        return Ok(Some(project(
            PredictionDataType::Ticker,
            s2c.ticker_list.into_iter().map(ticker_row).collect(),
        )));
    }
    Ok(None)
}

/// The three update protocols differ only by their `S2C` payload; each wraps it
/// in the same `retType`/`s2c` envelope. Decoding through a trait keeps one
/// accept rule instead of three near-identical copies.
trait AcceptedS2c: Message + Default {
    type S2c;

    fn accept(self) -> Option<Self::S2c>;
}

fn accepted_s2c<R: AcceptedS2c>(
    protocol: u32,
    body: &[u8],
) -> Result<Option<R::S2c>, QuotePushDecodeError> {
    let response =
        R::decode(body).map_err(|source| QuotePushDecodeError::Decode { protocol, source })?;
    Ok(response.accept())
}

impl AcceptedS2c for trade_proto::qot_get_event_contract_order_book::Response {
    type S2c = trade_proto::qot_get_event_contract_order_book::S2c;

    fn accept(self) -> Option<Self::S2c> {
        (self.ret_type == 0).then_some(self.s2c).flatten()
    }
}

impl AcceptedS2c for trade_proto::qot_get_event_contract_kline::Response {
    type S2c = trade_proto::qot_get_event_contract_kline::S2c;

    fn accept(self) -> Option<Self::S2c> {
        (self.ret_type == 0).then_some(self.s2c).flatten()
    }
}

impl AcceptedS2c for trade_proto::qot_get_event_contract_ticker::Response {
    type S2c = trade_proto::qot_get_event_contract_ticker::S2c;

    fn accept(self) -> Option<Self::S2c> {
        (self.ret_type == 0).then_some(self.s2c).flatten()
    }
}

fn project(
    data_type: PredictionDataType,
    entries: Vec<Value>,
) -> (PredictionDataType, Vec<PredictionPushRow>) {
    let rows = entries
        .into_iter()
        .filter_map(|entry| {
            let instrument_id = entry_instrument_id(&entry)?;
            Some(PredictionPushRow {
                instrument_id,
                data_type,
                sequence: entry_sequence(data_type, &entry),
                entry,
            })
        })
        .collect();
    (data_type, rows)
}

/// Go `predictionEntryInstrumentID`: the row identity is the `code` security,
/// falling back to `contractSecurity`. A row without either is skipped rather
/// than dispatched with an empty instrument.
pub fn entry_instrument_id(entry: &Value) -> Option<String> {
    for key in ["code", "contractSecurity"] {
        let value = entry
            .get(key)
            .and_then(|security| security.get("instrumentId"))
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty());
        if let Some(value) = value {
            return Some(value.to_owned());
        }
    }
    None
}

/// Go `predictionEntrySequence`: the last point's `sequence`, then `timeKey`,
/// then `time`. Only the ticker and K-line rows carry a nested list, and an
/// unknown/nested-invalid list yields an empty sequence instead of a panic.
pub fn entry_sequence(data_type: PredictionDataType, entry: &Value) -> String {
    let key = match data_type {
        PredictionDataType::Ticker => "tickerList",
        PredictionDataType::Kline => "klineList",
        PredictionDataType::OrderBook => return String::new(),
    };
    let Some(points) = entry.get(key).and_then(Value::as_array) else {
        return String::new();
    };
    let Some(last) = points.last().and_then(Value::as_object) else {
        return String::new();
    };
    for key in ["sequence", "timeKey", "time"] {
        let value = last.get(key);
        let text = match value {
            Some(Value::String(text)) => text.trim().to_owned(),
            Some(Value::Number(number)) => number.to_string(),
            _ => String::new(),
        };
        if !text.is_empty() {
            return text;
        }
    }
    String::new()
}

fn order_book_row(item: trade_proto::qot_get_event_contract_order_book::OrderBookItem) -> Value {
    let levels = |items: Vec<trade_proto::qot_get_event_contract_order_book::OrderBookLevel>| {
        items
            .into_iter()
            .map(|level| json!({"price": level.price, "size": level.size}))
            .collect::<Vec<_>>()
    };
    json!({
        "code": security_value(&item.code),
        "yesBids": levels(item.yes_bids),
        "yesAsks": levels(item.yes_asks),
        "noBids": levels(item.no_bids),
        "noAsks": levels(item.no_asks),
    })
}

fn kline_row(item: trade_proto::qot_get_event_contract_kline::KlineItem) -> Value {
    json!({
        "code": security_value(&item.code),
        "preSide": item.pre_side,
        "name": item.name,
        "klineList": item.kline_list.into_iter().map(|point| json!({
            "timeKey": point.time_key,
            "open": point.open,
            "high": point.high,
            "low": point.low,
            "close": point.close,
            "volume": point.volume,
        })).collect::<Vec<_>>(),
    })
}

fn ticker_row(item: trade_proto::qot_get_event_contract_ticker::TickerItem) -> Value {
    json!({
        "code": security_value(&item.code),
        "tickerList": item.ticker_list.into_iter().map(|point| json!({
            "time": point.time,
            "yesPrice": point.yes_price,
            "noPrice": point.no_price,
            "volume": point.volume,
            "side": point.side,
            "sequence": point.sequence,
        })).collect::<Vec<_>>(),
    })
}

fn security_value(value: &trade_proto::qot_common::Security) -> Value {
    // Go's `normalizeOpenDSecurity` requires a non-empty `code` before it will
    // derive an `instrumentId`; a blank contract code yields no identity at
    // all, so the row is skipped instead of surfacing as a bogus `US.`.
    let code = value.code.trim();
    let mut row = json!({"market": value.market, "code": value.code});
    if !code.is_empty() {
        row["instrumentId"] = Value::String(format!("US.{}", code.to_ascii_uppercase()));
    }
    row
}
