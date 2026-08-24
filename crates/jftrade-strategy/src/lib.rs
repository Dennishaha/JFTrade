#![forbid(unsafe_code)]

//! Strategy runtime control with a consumer-owned narrow trading port.

mod model;
mod runtime_registry;
mod service;

pub use model::{
    ExecutionMode, PluginUninstallCommands, PluginUninstallGuidance, RuntimeState, Signal,
    SignalOutcome, StrategyError, StrategyNotification, TradeIntent, TradePlanReceipt,
};
pub use runtime_registry::{
    RuntimeInstanceSummary, RuntimeRegistryError, RuntimeRegistrySnapshot, StrategyRuntimeRegistry,
};
pub use service::{StrategyCoordinator, TradePlannerPort};
