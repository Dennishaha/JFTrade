use std::sync::atomic::{AtomicU64, AtomicUsize, Ordering};

#[derive(Debug, Default)]
pub struct TransportMetrics {
    started: AtomicU64,
    completed: AtomicU64,
    failures: AtomicU64,
    in_flight: AtomicUsize,
}

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub struct TransportSnapshot {
    pub started: u64,
    pub completed: u64,
    pub failures: u64,
    pub in_flight: usize,
}

impl TransportMetrics {
    pub(crate) fn start(&self) {
        self.started.fetch_add(1, Ordering::Relaxed);
        self.in_flight.fetch_add(1, Ordering::Relaxed);
    }

    pub(crate) fn finish(&self, failed: bool) {
        self.completed.fetch_add(1, Ordering::Relaxed);
        if failed {
            self.failures.fetch_add(1, Ordering::Relaxed);
        }
        self.in_flight.fetch_sub(1, Ordering::Relaxed);
    }

    pub fn snapshot(&self) -> TransportSnapshot {
        TransportSnapshot {
            started: self.started.load(Ordering::Relaxed),
            completed: self.completed.load(Ordering::Relaxed),
            failures: self.failures.load(Ordering::Relaxed),
            in_flight: self.in_flight.load(Ordering::Relaxed),
        }
    }
}
