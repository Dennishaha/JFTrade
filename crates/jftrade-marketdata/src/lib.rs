#![forbid(unsafe_code)]

//! Transport- and provider-neutral market-data ownership rules.

mod cache;
mod demand;
mod model;
mod router;

pub use cache::{CacheLookup, TickCache};
pub use demand::{DemandBook, DemandSnapshot};
pub use model::{
    HealthStatus, InstrumentRef, MarketDataError, ProviderCapabilities, ProviderConstraints,
    ProviderDescriptor, ProviderReadiness, Tick,
};
pub use router::{ActivationMode, ProviderRouter, ProviderRuntime};
