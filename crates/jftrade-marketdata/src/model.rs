use std::collections::BTreeMap;

use jftrade_kernel::{DecimalText, Fixed8};
use serde::{Deserialize, Serialize, Serializer};
use thiserror::Error;

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "lowercase")]
pub enum ProviderReadiness {
    Warming,
    Ready,
    Failed,
    #[default]
    Unknown,
}

#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct HealthStatus {
    pub connected: bool,
    pub stream_mode: String,
    pub active_count: usize,
    pub readiness: ProviderReadiness,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub last_error: Option<String>,
}

impl HealthStatus {
    pub fn is_ready(&self) -> bool {
        self.connected && self.readiness == ProviderReadiness::Ready && self.last_error.is_none()
    }
}

#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ProviderCapabilities {
    pub snapshots: bool,
    pub streaming_quotes: bool,
    pub streaming_candles: bool,
    pub streaming_depth: bool,
    pub historical_candles: bool,
    pub tick_candles: bool,
    pub order_book_depth: bool,
    pub instrument_search: bool,
    pub extended_hours: bool,
    #[serde(default)]
    pub candle_intervals: Vec<String>,
    #[serde(default)]
    pub order_book_levels: Vec<u16>,
    #[serde(default)]
    pub sessions: Vec<String>,
    #[serde(default)]
    pub price_adjustments: Vec<String>,
    #[serde(default)]
    pub historical_lookback_days: BTreeMap<String, u32>,
}

#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ProviderConstraints {
    pub requires_open_d: bool,
    pub requires_market_data_right: bool,
    pub uses_subscription_quota: bool,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ProviderDescriptor {
    pub selection_id: String,
    pub provider_id: String,
    pub display_name: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub broker_id: Option<String>,
    pub source: String,
    pub default_market: String,
    pub supported_markets: Vec<String>,
    pub transports: Vec<String>,
    pub capabilities: ProviderCapabilities,
    pub constraints: ProviderConstraints,
    #[serde(default)]
    pub notes: Vec<String>,
}

impl ProviderDescriptor {
    pub fn validate(&self) -> Result<(), MarketDataError> {
        for (field, value) in [
            ("selectionId", self.selection_id.as_str()),
            ("providerId", self.provider_id.as_str()),
            ("displayName", self.display_name.as_str()),
            ("source", self.source.as_str()),
            ("defaultMarket", self.default_market.as_str()),
        ] {
            if value.trim().is_empty() {
                return Err(MarketDataError::InvalidDescriptor(field));
            }
        }
        if self.supported_markets.is_empty() || self.transports.is_empty() {
            return Err(MarketDataError::InvalidDescriptor(
                "supportedMarkets/transports",
            ));
        }
        Ok(())
    }
}

#[derive(Clone, Debug, Deserialize, Eq, Ord, PartialEq, PartialOrd, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct InstrumentRef {
    pub channel: String,
    pub market: String,
    pub symbol: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub interval: Option<String>,
}

impl InstrumentRef {
    pub fn normalize(mut self) -> Result<Self, MarketDataError> {
        self.channel = self.channel.trim().to_ascii_uppercase();
        if self.channel.is_empty() {
            self.channel = "SNAPSHOT".to_owned();
        }
        self.market = self.market.trim().to_ascii_uppercase();
        self.symbol = self.symbol.trim().to_ascii_uppercase();
        if let Some((prefix, symbol)) = self.symbol.split_once('.') {
            if self.market.is_empty() {
                self.market = prefix.to_owned();
            }
            self.symbol = symbol.to_owned();
        }
        if !matches!(
            self.channel.as_str(),
            "SNAPSHOT" | "TICK" | "KLINE" | "ORDER_BOOK"
        ) {
            return Err(MarketDataError::InvalidSubscription(format!(
                "unsupported channel {}",
                self.channel
            )));
        }
        if self.market.is_empty() || self.symbol.is_empty() {
            return Err(MarketDataError::InvalidSubscription(
                "market and symbol are required".to_owned(),
            ));
        }
        let interval = self
            .interval
            .take()
            .map(|value| value.trim().to_ascii_lowercase())
            .filter(|value| !value.is_empty());
        if self.channel == "KLINE" {
            let Some(interval_val) = interval else {
                return Err(MarketDataError::InvalidSubscription(
                    "KLINE requires interval".to_owned(),
                ));
            };
            let valid_intervals = ["1m", "3m", "5m", "15m", "30m", "1h", "1d", "1w", "1mo"];
            if !valid_intervals.contains(&interval_val.as_str()) {
                return Err(MarketDataError::InvalidSubscription(format!(
                    "unsupported KLINE interval: {interval_val}",
                )));
            }
            self.interval = Some(interval_val);
        } else if interval.is_some() {
            return Err(MarketDataError::InvalidSubscription(format!(
                "subscription interval is only valid for KLINE, got channel {}",
                self.channel,
            )));
        } else {
            self.interval = None;
        }
        Ok(self)
    }

    pub fn instrument_id(&self) -> String {
        format!("{}.{}", self.market, self.symbol)
    }

    pub fn key(&self) -> String {
        match &self.interval {
            Some(interval) => format!(
                "{}:{}:{}:{}",
                self.channel, self.market, self.symbol, interval
            ),
            None => format!("{}:{}:{}", self.channel, self.market, self.symbol),
        }
    }
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Tick {
    pub instrument_id: String,
    pub price: Fixed8,
    #[serde(serialize_with = "serialize_decimal_number")]
    pub volume: DecimalText,
    /// Optional fields preserved from a typed provider snapshot.  Keeping
    /// this broker-neutral lets consumers project rich quote/securities
    /// responses without reaching back into a concrete OpenD protocol.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub snapshot: Option<TradeQuoteSnapshot>,
    pub observed_at_ms: i64,
    pub provider_generation: u64,
}

#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct TradeQuoteSnapshot {
    pub symbol: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub is_suspended: Option<bool>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub bid_price: Option<Fixed8>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub ask_price: Option<Fixed8>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub last_price: Option<Fixed8>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub volume: Option<DecimalText>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub lot_size: Option<i32>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub security_type: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub market: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub open_price: Option<Fixed8>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub high_price: Option<Fixed8>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub low_price: Option<Fixed8>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub previous_close: Option<Fixed8>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub turnover: Option<DecimalText>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub update_time: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub status: Option<i32>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub pe_rate: Option<DecimalText>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub pb_rate: Option<DecimalText>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub pre_market: Option<ExtendedQuoteSnapshot>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub after_market: Option<ExtendedQuoteSnapshot>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub overnight: Option<ExtendedQuoteSnapshot>,
}

#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ExtendedQuoteSnapshot {
    pub price: Option<Fixed8>,
    pub high_price: Option<Fixed8>,
    pub low_price: Option<Fixed8>,
    pub volume: Option<DecimalText>,
    pub turnover: Option<DecimalText>,
    pub change: Option<DecimalText>,
    pub change_rate: Option<DecimalText>,
    pub amplitude: Option<DecimalText>,
}

/// Security routes consume the same provider-neutral BasicQot row.  Keep the
/// semantic name available to adapters without introducing a second cache or
/// a broker-specific protocol type.
pub type BrokerSecuritySnapshot = TradeQuoteSnapshot;

fn serialize_decimal_number<S>(value: &DecimalText, serializer: S) -> Result<S::Ok, S::Error>
where
    S: Serializer,
{
    value.serialize_number(serializer)
}

#[derive(Clone, Debug, Eq, Error, PartialEq)]
pub enum MarketDataError {
    #[error("invalid provider descriptor field {0}")]
    InvalidDescriptor(&'static str),
    #[error("invalid subscription: {0}")]
    InvalidSubscription(String),
    #[error("consumer id is required")]
    MissingConsumer,
    #[error("provider {0} is not registered")]
    ProviderNotFound(String),
    #[error("provider {provider_id} is not ready: {reason}")]
    ProviderUnavailable { provider_id: String, reason: String },
    #[error("managed market-data subscriptions are active")]
    ManagedSubscriptionsActive,
    #[error("provider {0} does not support streaming subscriptions")]
    StreamingUnavailable(String),
    #[error("market-data cache miss for {0}")]
    CacheMiss(String),
    #[error("market-data cache entry for {0} is stale")]
    CacheStale(String),
    #[error("market-data value belongs to a stale provider generation")]
    ProviderChanged,
}

#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct PhysicalSubscriptionSnapshot {
    pub desired_count: usize,
    pub own_active_count: usize,
    pub pending_release_count: usize,
    pub fallback_count: usize,
    pub connection_generation: Option<u64>,
    pub observed_connection_generation: Option<u64>,
    pub total_used_quota: Option<u64>,
    pub remain_quota: Option<u64>,
    pub own_used_quota: Option<u64>,
    pub checked_at: Option<String>,
    pub last_error: Option<String>,
    pub reconciled_at: Option<String>,
    pub entries: Vec<PhysicalSubscriptionEntry>,
}

#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct PhysicalSubscriptionEntry {
    pub key: String,
    pub kind: String,
    pub instrument_id: String,
    pub interval: Option<String>,
    pub broker_state: String,
    pub subscribed_at: Option<String>,
    pub unsubscribe_eligible_at: Option<String>,
    pub last_error: Option<String>,
}

pub trait PhysicalSubscriptionSnapshotPort: Send + Sync + std::fmt::Debug {
    fn physical_subscription_snapshot(
        &self,
    ) -> Result<Option<PhysicalSubscriptionSnapshot>, String>;
}
