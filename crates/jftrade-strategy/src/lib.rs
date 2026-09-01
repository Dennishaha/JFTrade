#![forbid(unsafe_code)]

//! Strategy runtime control with a consumer-owned narrow trading port.

mod model;
pub mod pine;
pub mod pinespec;
mod runtime_registry;
mod service;

pub use model::{
    ExecutionMode, PluginUninstallCommands, PluginUninstallGuidance, RuntimeState, Signal,
    SignalOutcome, StrategyError, StrategyNotification, TradeIntent, TradePlanReceipt,
};
pub use runtime_registry::{
    RuntimeInstanceSummary, RuntimeRegistryError, RuntimeRegistrySnapshot, StrategyRuntimeRegistry,
    format_timestamp, max_timestamp, normalize_timestamp, optional_string,
};
pub use service::{StrategyCoordinator, TradePlannerPort};
