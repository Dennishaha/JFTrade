#![forbid(unsafe_code)]

//! Side-effect-free trading plans and projections used during the Stage 5 shadow.

mod ledger;
mod model;
mod notification;
mod portfolio;
mod real_trade;
mod risk;
mod session;

pub use ledger::{ShadowCheckpoint, ShadowTrading};
pub use model::{
    AuditEntry, BrokerOrderEvent, EventOutcome, OrderCommand, OrderProjection, OrderSide,
    OrderStatus, ShadowCommandPlan, TradingEnvironment, TradingError, canonical_broker_status,
    canonical_stored_status, reconcile_status,
};
pub use notification::{NotificationEnvelope, NotificationPlanner};
pub use portfolio::{AccountPortfolio, AccountRefresh, PortfolioOutcome, PositionProjection};
pub use real_trade::{
    RealTradeApprovalPolicy, RealTradeApprovalsResponse, RealTradeControlEvent,
    RealTradeControlState, RealTradeHardStopEntry, RealTradeHardStopEventsResponse,
    RealTradeHardStopsResponse, RealTradeKillSwitchEntry, RealTradeKillSwitchEventsResponse,
    RealTradeKillSwitchStateResponse, RealTradeRiskEventsResponse, RealTradeRiskLimitsResponse,
    RealTradeRiskSnapshot, RealTradeRuntimeRiskEntry,
};
pub use risk::{HardStop, RiskConfig, RiskDecision, RiskEngine};
pub use session::{AccountSnapshot, BrokerSession, SessionState};
