#![forbid(unsafe_code)]

//! Futu OpenD protocol and subscription adapter boundaries.

mod frame;
mod health;
mod probe;
mod provider;
mod subscriptions;
mod trading;
mod transport;

pub use frame::{Frame, FrameError, Header, decode_frame, encode_frame};
pub use health::{
    OpenDTcpProbe, OpenDTcpProbeConfig, OpenDTcpProbeError, market_data_health_from_probe,
};
pub use probe::{MarketState, OpenDProbe, WireGlobalState};
pub use provider::{broker_descriptor, provider_descriptor};
pub use subscriptions::{
    PhysicalSubscription, ReconcileAction, SubscriptionKind, SubscriptionPlan,
    SubscriptionReconciler, desired_subscriptions, retry_delay_ms,
};
pub use trading::{
    RawOrderUpdate, TradeProtocol, TradeProtocolError, TradeProtocolPlan, map_order_update,
    plan_shadow_protocol,
};
pub use transport::{
    OpenDClient, OpenDTcpTransport, OpenDTransport, TcpTransportError, TransportError,
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
