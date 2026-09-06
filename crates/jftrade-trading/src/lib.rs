#![forbid(unsafe_code)]

//! Side-effect-free trading plans and projections used by the product and compatibility replay.

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
pub use portfolio::{
    AccountPortfolio, AccountRefresh, PortfolioOutcome, PositionProjection,
    position_matches_symbol, sellable_quantity,
};
pub use real_trade::{
    RealTradeApprovalPolicy, RealTradeApprovalsResponse, RealTradeControlEvent,
    RealTradeControlState, RealTradeHardStopEntry, RealTradeHardStopEventsResponse,
    RealTradeHardStopsResponse, RealTradeKillSwitchEntry, RealTradeKillSwitchEventsResponse,
    RealTradeKillSwitchStateResponse, RealTradeRiskEventsResponse, RealTradeRiskLimitsResponse,
    RealTradeRiskSnapshot, RealTradeRuntimeRiskEntry,
};
pub use risk::{
    HardStop, PreTradeRiskComboLeg, PreTradeRiskDecision, PreTradeRiskOrder, PreTradeRiskPolicy,
    RUNTIME_RISK_MODE_ENFORCE, RUNTIME_RISK_MODE_MONITOR, RUNTIME_RISK_MODE_OFF, RiskConfig,
    RiskDecision, RiskEngine, RuntimeRiskContext, RuntimeRiskDecision, RuntimeRiskOrder,
    RuntimeRiskSettings, evaluate_pre_trade_risk, evaluate_runtime_risk,
};
pub use session::{AccountSnapshot, BrokerSession, SessionState};
