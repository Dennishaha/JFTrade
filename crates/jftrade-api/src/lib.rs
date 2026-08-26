#![forbid(unsafe_code)]

mod auth;
mod envelope;
mod observability;
mod ports;
mod route;
mod router;
mod sse;
mod websocket;

pub use auth::{
    ACCESS_SURFACE_HEADER, AccessPolicy, DESKTOP_WEBSOCKET_PROTOCOL,
    INTERNAL_PROXY_PROTOCOL_HEADER, canonical_origin, desktop_trusted_origins,
};
pub use envelope::{ApiFailure, Clock, FixedClock, SystemClock};
pub use observability::{
    OpenDHealth, RequestObservabilitySnapshot, TransportEvent, TransportMetrics, TransportSnapshot,
};
pub use ports::{ApiOutput, ApiPort, ApiRequest, Asset, AssetBundle, PortFuture};
pub use route::{RouteCatalog, RouteCatalogError, RouteSpec};
pub use router::{ApiState, build_router};
pub use sse::{SseEvent, encode_comment, encode_event, encode_retry};
pub use websocket::{
    DEFAULT_WEBSOCKET_LIMIT, LiveConnectionMetrics, LiveConnectionPermit, LiveConnectionSnapshot,
    websocket_origin_allowed,
};
