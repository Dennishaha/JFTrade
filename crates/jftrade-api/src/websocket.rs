use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use axum::http::HeaderMap;

use crate::AccessPolicy;
use crate::auth::{origin_provided, request_origin};

pub const DEFAULT_WEBSOCKET_LIMIT: usize = 20;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct LiveConnectionSnapshot {
    pub connected: usize,
    pub limit: usize,
    pub at_limit: bool,
}

#[derive(Debug)]
pub struct LiveConnectionMetrics {
    limit: usize,
    connected: AtomicUsize,
}

impl Default for LiveConnectionMetrics {
    fn default() -> Self {
        Self::new(DEFAULT_WEBSOCKET_LIMIT)
    }
}

impl LiveConnectionMetrics {
    pub fn new(limit: usize) -> Self {
        Self {
            limit: limit.max(1),
            connected: AtomicUsize::new(0),
        }
    }

    pub fn snapshot(&self) -> LiveConnectionSnapshot {
        let connected = self.connected.load(Ordering::Acquire);
        LiveConnectionSnapshot {
            connected,
            limit: self.limit,
            at_limit: connected >= self.limit,
        }
    }

    pub fn try_acquire(self: &Arc<Self>) -> Option<LiveConnectionPermit> {
        self.connected
            .fetch_update(Ordering::AcqRel, Ordering::Acquire, |current| {
                (current < self.limit).then_some(current + 1)
            })
            .ok()
            .map(|_| LiveConnectionPermit {
                metrics: Arc::clone(self),
            })
    }
}

#[derive(Debug)]
pub struct LiveConnectionPermit {
    metrics: Arc<LiveConnectionMetrics>,
}

impl Drop for LiveConnectionPermit {
    fn drop(&mut self) {
        self.metrics.connected.fetch_sub(1, Ordering::AcqRel);
    }
}

pub fn websocket_origin_allowed(headers: &HeaderMap, policy: &AccessPolicy) -> bool {
    if !origin_provided(headers) {
        return true;
    }
    request_origin(headers).is_some_and(|origin| policy.allowed_origins.contains(&origin))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn live_connection_snapshot_tracks_concurrency_and_limit() {
        let metrics = Arc::new(LiveConnectionMetrics::new(2));
        assert_eq!(
            metrics.snapshot(),
            LiveConnectionSnapshot {
                connected: 0,
                limit: 2,
                at_limit: false,
            }
        );

        let first = metrics.try_acquire().expect("first connection");
        assert_eq!(metrics.snapshot().connected, 1);
        assert!(!metrics.snapshot().at_limit);

        let second = metrics.try_acquire().expect("second connection");
        assert_eq!(
            metrics.snapshot(),
            LiveConnectionSnapshot {
                connected: 2,
                limit: 2,
                at_limit: true,
            }
        );
        assert!(metrics.try_acquire().is_none());

        drop(first);
        assert_eq!(metrics.snapshot().connected, 1);
        assert!(!metrics.snapshot().at_limit);
        drop(second);
        assert_eq!(metrics.snapshot().connected, 0);
    }

    #[test]
    fn live_connection_limit_is_never_zero() {
        let metrics = Arc::new(LiveConnectionMetrics::new(0));
        let permit = metrics.try_acquire().expect("effective first connection");
        assert_eq!(metrics.snapshot().limit, 1);
        assert!(metrics.snapshot().at_limit);
        assert!(metrics.try_acquire().is_none());
        drop(permit);
    }
}
