#![forbid(unsafe_code)]

//! Futu OpenD protocol and subscription adapter boundaries.

mod basic_quote_query;
mod basic_quote_tick;
mod frame;
mod health;
mod history;
mod managed_session;
mod probe;
mod provider;
mod provider_runtime;
mod quote_push;
mod runtime_task;
mod security_snapshot_query;
mod session_coordinator;
mod session_event_pump;
mod subscription_executor;
mod subscriptions;
// The generated module is crate-internal; generated messages must not leak to
// engine consumers.  Generated code is intentionally exempt from local lint
// rules because its field/enum names are dictated by the OpenD schema.
#[allow(dead_code, clippy::all)]
mod trade_proto;
mod trade_proto_fee_validation;
mod trade_proto_fill_validation;
mod trade_proto_margin_ratio_validation;
mod trade_proto_max_qty_validation;
mod trade_proto_order_validation;
mod trade_proto_validation;
mod trade_session;
mod trade_snapshots;
mod trading;
mod transport;

pub use basic_quote_query::{BasicQuoteQueryError, OpenDBasicQuoteExecutor};
pub use basic_quote_tick::{BasicQuoteTickError, basic_quote_ticks};
pub use frame::{Frame, FrameError, Header, decode_frame, encode_frame};
pub use health::{
    OpenDInitializedSession, OpenDTcpProbe, OpenDTcpProbeConfig, OpenDTcpProbeError,
    market_data_health_from_probe,
};
pub use history::{
    HistoricalKline, HistoricalKlineError, HistoricalKlineQuery, HistoricalKlineReadPort,
    HistoricalKlineResult, HistoricalSecurity, OpenDHistoricalKlineReader,
};
pub use managed_session::{
    OpenDManagedSession, OpenDManagedSessionError, OpenDSessionCloseReason, OpenDSessionEvent,
};
pub use probe::{MarketState, OpenDProbe, WireGlobalState};
pub use provider::{broker_descriptor, provider_descriptor};
pub use provider_runtime::{
    OpenDProviderRuntime, OpenDProviderRuntimeConfig, OpenDProviderRuntimeError,
};
pub use quote_push::{
    BasicQuote, BasicQuotePush, Kline, KlinePush, OrderBookDetail, OrderBookLevel, OrderBookPush,
    PreAfterMarketData, QuotePush, QuotePushDecodeError, Security, decode_quote_push,
};
pub use runtime_task::{
    OpenDSessionEventListener, OpenDSessionRuntime, OpenDSessionRuntimeConfig,
    OpenDSessionRuntimeError, OpenDSessionRuntimeStatus,
};
pub use security_snapshot_query::{
    OpenDSecuritySnapshotReader, SecuritySnapshotQueryError, SecuritySnapshotReadPort,
};
pub use session_coordinator::{
    OpenDSessionCoordinator, OpenDSessionCoordinatorError, OpenDSessionCoordinatorOutcome,
};
pub use session_event_pump::{
    OpenDSessionEventPump, OpenDSessionPumpError, OpenDSessionPumpOutcome,
};
pub use subscription_executor::{OpenDSubscriptionExecutor, SubscriptionExecutorError};
pub use subscriptions::{
    OpenDSubscriptionLifecycle, PhysicalSubscription, ReconcileAction, SubscriptionKind,
    SubscriptionPlan, SubscriptionReconciler, desired_subscriptions, retry_delay_ms,
};
pub use trade_proto::ResponseError;
pub use trade_session::{OpenDTradeReadClient, TradeReadPort, TradeSessionError, trade_header};
pub use trade_snapshots::{
    TradeAccountSnapshot, TradeCashFlowSnapshot, TradeCashInfo, TradeComboLeg, TradeFillSnapshot,
    TradeFilter, TradeFunds, TradeFundsSnapshot, TradeHeader, TradeMarginRatioSnapshot,
    TradeMarketInfo, TradeMaxTradeQuantityRequest, TradeMaxTradeQuantitySnapshot,
    TradeOrderFeeItemSnapshot, TradeOrderFeeSnapshot, TradeOrderSnapshot, TradePositionSnapshot,
    TradeSecurity,
};
pub use trading::{
    RawOrderUpdate, TradeProtocol, TradeProtocolError, TradeProtocolPlan, map_order_update,
    plan_shadow_protocol,
};
pub use transport::{
    OpenDClient, OpenDFrameReader, OpenDTcpTransport, OpenDTransport, TcpTransportError,
    TransportError,
};

pub const PROTO_INIT_CONNECT: u32 = 1001;
pub const PROTO_GET_GLOBAL_STATE: u32 = 1002;
pub const PROTO_KEEP_ALIVE: u32 = 1004;
pub const PROTO_QOT_SUB: u32 = 3001;
pub const PROTO_GET_SUB_INFO: u32 = 3003;
pub const PROTO_GET_BASIC_QOT: u32 = 3004;
pub const PROTO_GET_SECURITY_SNAPSHOT: u32 = 3203;
pub const PROTO_UPDATE_BASIC_QOT: u32 = 3005;
pub const PROTO_GET_KL: u32 = 3006;
pub const PROTO_UPDATE_KL: u32 = 3007;
pub const PROTO_GET_ORDER_BOOK: u32 = 3012;
pub const PROTO_UPDATE_ORDER_BOOK: u32 = 3013;
pub const PROTO_REQUEST_HISTORY_KL: u32 = 3103;
pub const MINIMUM_OPEND_VERSION: &str = "10.9.6908";
