#![forbid(unsafe_code)]

//! Futu OpenD protocol and subscription adapter boundaries.

mod basic_quote_query;
mod basic_quote_tick;
mod frame;
mod health;
mod managed_session;
mod probe;
mod provider;
mod quote_push;
mod session_coordinator;
mod session_event_pump;
mod subscription_executor;
mod subscriptions;
mod trading;
mod transport;

pub use basic_quote_query::{BasicQuoteQueryError, OpenDBasicQuoteExecutor};
pub use basic_quote_tick::{BasicQuoteTickError, basic_quote_ticks};
pub use frame::{Frame, FrameError, Header, decode_frame, encode_frame};
pub use health::{
    OpenDInitializedSession, OpenDTcpProbe, OpenDTcpProbeConfig, OpenDTcpProbeError,
    market_data_health_from_probe,
};
pub use managed_session::{
    OpenDManagedSession, OpenDManagedSessionError, OpenDSessionCloseReason, OpenDSessionEvent,
};
pub use probe::{MarketState, OpenDProbe, WireGlobalState};
pub use provider::{broker_descriptor, provider_descriptor};
pub use quote_push::{
    BasicQuote, BasicQuotePush, Kline, KlinePush, OrderBookDetail, OrderBookLevel, OrderBookPush,
    PreAfterMarketData, QuotePush, QuotePushDecodeError, Security, decode_quote_push,
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
pub const PROTO_GET_BASIC_QOT: u32 = 3004;
pub const PROTO_UPDATE_BASIC_QOT: u32 = 3005;
pub const PROTO_GET_KL: u32 = 3006;
pub const PROTO_UPDATE_KL: u32 = 3007;
pub const PROTO_GET_ORDER_BOOK: u32 = 3012;
pub const PROTO_UPDATE_ORDER_BOOK: u32 = 3013;
pub const PROTO_REQUEST_HISTORY_KL: u32 = 3103;
pub const MINIMUM_OPEND_VERSION: &str = "10.9.6908";
