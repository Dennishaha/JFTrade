use std::collections::{BTreeMap, BTreeSet};
use std::sync::Arc;
use std::sync::RwLock;
use std::sync::atomic::{AtomicUsize, Ordering};

use axum::http::HeaderMap;

use crate::AccessPolicy;
use crate::auth::{origin_provided, request_origin};

pub const DEFAULT_WEBSOCKET_LIMIT: usize = 20;

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct LiveConnectionSnapshot {
    pub connected: usize,
    pub limit: usize,
    pub at_limit: bool,
    pub active_instruments: Vec<String>,
}

#[derive(Debug)]
pub struct LiveConnectionMetrics {
    limit: usize,
    connected: AtomicUsize,
    next_client_id: AtomicUsize,
    active_instruments: RwLock<BTreeMap<usize, Vec<String>>>,
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
            next_client_id: AtomicUsize::new(0),
            active_instruments: RwLock::new(BTreeMap::new()),
        }
    }

    pub fn snapshot(&self) -> LiveConnectionSnapshot {
        let connected = self.connected.load(Ordering::Acquire);
        let active_instruments = self
            .active_instruments
            .read()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .values()
            .flatten()
            .cloned()
            .collect::<BTreeSet<_>>()
            .into_iter()
            .collect();
        LiveConnectionSnapshot {
            connected,
            limit: self.limit,
            at_limit: connected >= self.limit,
            active_instruments,
        }
    }

    pub fn try_acquire(self: &Arc<Self>) -> Option<LiveConnectionPermit> {
        self.connected
            .fetch_update(Ordering::AcqRel, Ordering::Acquire, |current| {
                (current < self.limit).then_some(current + 1)
            })
            .ok()
            .map(|_| {
                let client_id = self.next_client_id.fetch_add(1, Ordering::Relaxed) + 1;
                self.active_instruments
                    .write()
                    .unwrap_or_else(std::sync::PoisonError::into_inner)
                    .insert(client_id, Vec::new());
                LiveConnectionPermit {
                    client_id,
                    metrics: Arc::clone(self),
                }
            })
    }
}

#[derive(Debug)]
pub struct LiveConnectionPermit {
    client_id: usize,
    metrics: Arc<LiveConnectionMetrics>,
}

impl LiveConnectionPermit {
    pub fn set_active_instruments(&self, values: &[String]) {
        let normalized = values
            .iter()
            .map(|value| value.trim().to_uppercase())
            .filter(|value| !value.is_empty())
            .collect::<BTreeSet<_>>()
            .into_iter()
            .collect();
        self.metrics
            .active_instruments
            .write()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .insert(self.client_id, normalized);
    }
}

impl Drop for LiveConnectionPermit {
    fn drop(&mut self) {
        self.metrics
            .active_instruments
            .write()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .remove(&self.client_id);
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
                active_instruments: Vec::new(),
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
                active_instruments: Vec::new(),
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

    #[test]
    fn active_instruments_are_normalized_unioned_replaced_and_released() {
        let metrics = Arc::new(LiveConnectionMetrics::new(2));
        let first = metrics.try_acquire().expect("first connection");
        let second = metrics.try_acquire().expect("second connection");
        first.set_active_instruments(&[
            " us.aapl ".to_owned(),
            "HK.00700".to_owned(),
            "US.AAPL".to_owned(),
        ]);
        second.set_active_instruments(&[" cn.600000 ".to_owned(), "us.aapl".to_owned()]);
        assert_eq!(
            metrics.snapshot().active_instruments,
            ["CN.600000", "HK.00700", "US.AAPL"]
        );

        first.set_active_instruments(&["US.MSFT".to_owned()]);
        assert_eq!(
            metrics.snapshot().active_instruments,
            ["CN.600000", "US.AAPL", "US.MSFT"]
        );
        drop(second);
        assert_eq!(metrics.snapshot().active_instruments, ["US.MSFT"]);
        drop(first);
        assert!(metrics.snapshot().active_instruments.is_empty());
    }
}
