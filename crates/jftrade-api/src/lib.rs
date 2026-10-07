#![forbid(unsafe_code)]

mod auth;
mod browser_access;
mod envelope;
mod observability;
mod ports;
mod route;
mod router;
mod session_lifecycle;
mod sse;
mod swagger_docs;
mod websocket;

pub use auth::{
    ACCESS_SURFACE_HEADER, AccessOriginProvider, AccessPolicy, DESKTOP_WEBSOCKET_PROTOCOL,
    INTERNAL_PROXY_PROTOCOL_HEADER, SESSION_COOKIE, WebSessionValidator, canonical_origin,
    desktop_trusted_origins,
};
pub use browser_access::{WebAccessState, WebAccessStatePort};
pub use envelope::{ApiFailure, Clock, FixedClock, SystemClock};
pub use observability::{
    OpenDHealth, RequestObservabilitySnapshot, TransportEvent, TransportMetrics, TransportSnapshot,
};
pub use ports::{
    ApiOutput, ApiPort, ApiRequest, ApiStream, ApiStreamSender, Asset, AssetBundle, PortFuture,
};
pub use route::{RouteCatalog, RouteCatalogError, RouteSpec};
pub use router::{
    ApiState, LiveMarketDataStatus, LiveMarketDataStatusPort, RequestContext, build_router,
    current_request_context,
};
pub use sse::{
    BufferedSseSink, SSE_RETRY_MILLIS, SseError, SseEvent, SseLoopError, SseLoopFuture, SseSink,
    SseStreamLoopOptions, SseWriter, encode_comment, encode_event, encode_retry,
    run_sse_stream_loop,
};
pub use swagger_docs::SwaggerDocs;
pub use websocket::{
    DEFAULT_WEBSOCKET_LIMIT, LiveConnectionMetrics, LiveConnectionPermit, LiveConnectionSnapshot,
    LiveDemandListener, LiveDepthSubscription, LiveHub, LiveHubConnection, LiveHubLifecycle,
    LiveHubSnapshot, LiveSecuritySubscription, LiveSubscriptionSnapshot, websocket_origin_allowed,
};
