#![forbid(unsafe_code)]

mod auth;
mod envelope;
mod observability;
mod ports;
mod route;
mod router;
mod sse;
mod websocket;

pub use auth::{AccessPolicy, canonical_origin};
pub use envelope::{ApiFailure, Clock, FixedClock, SystemClock};
pub use observability::{TransportMetrics, TransportSnapshot};
pub use ports::{ApiOutput, ApiPort, ApiRequest, Asset, AssetBundle, PortFuture};
pub use route::{RouteCatalog, RouteCatalogError, RouteSpec};
pub use router::{ApiState, build_router};
pub use sse::{SseEvent, encode_comment, encode_event, encode_retry};
pub use websocket::{DEFAULT_WEBSOCKET_LIMIT, websocket_origin_allowed};
