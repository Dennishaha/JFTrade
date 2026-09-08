#![forbid(unsafe_code)]

//! Transport- and provider-neutral market-data ownership rules.

mod cache;
pub mod catalog;
mod demand;
mod model;
mod router;
mod runtime;
mod snapshot_poll;

pub use cache::{CacheLookup, TickCache};
pub use catalog::{
    MarketCatalogError, MarketPrecision, MarketRule, NormalizedInstrument, TradingSessionWindow,
    cn_market_rule, default_markets, find_market_rule, hk_market_rule, infer_cn_prefix,
    normalize_instrument, sh_market_rule, sz_market_rule, us_market_rule,
};
pub use demand::{DemandBook, DemandSnapshot};
pub use model::{
    BrokerSecuritySnapshot, ExtendedQuoteSnapshot, HealthStatus, InstrumentRef, MarketDataError,
    PhysicalSubscriptionEntry, PhysicalSubscriptionSnapshot, PhysicalSubscriptionSnapshotPort,
    ProviderCapabilities, ProviderConstraints, ProviderDescriptor, ProviderReadiness, Tick,
    TradeQuoteSnapshot,
};
pub use router::{ActivationMode, ProviderRouter, ProviderRuntime};
pub use runtime::{CollectorRuntimeState, MarketDataRuntimeRecorder};
pub use snapshot_poll::{
    SnapshotPollExecutor, SnapshotPollOutcome, SnapshotPollPolicy, SnapshotPollSkipReason,
};
