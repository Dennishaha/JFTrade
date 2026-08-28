#![forbid(unsafe_code)]

//! Transport- and provider-neutral market-data ownership rules.

mod cache;
mod demand;
mod model;
mod router;
mod runtime;
mod snapshot_poll;

pub use cache::{CacheLookup, TickCache};
pub use demand::{DemandBook, DemandSnapshot};
pub use model::{
    HealthStatus, InstrumentRef, MarketDataError, PhysicalSubscriptionEntry,
    PhysicalSubscriptionSnapshot, PhysicalSubscriptionSnapshotPort, ProviderCapabilities,
    ProviderConstraints, ProviderDescriptor, ProviderReadiness, Tick,
};
pub use router::{ActivationMode, ProviderRouter, ProviderRuntime};
pub use runtime::{CollectorRuntimeState, MarketDataRuntimeRecorder};
pub use snapshot_poll::{
    SnapshotPollExecutor, SnapshotPollOutcome, SnapshotPollPolicy, SnapshotPollSkipReason,
};
