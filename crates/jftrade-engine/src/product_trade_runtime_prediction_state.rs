//! Prediction subscription bookkeeping shared by the trade read runtime.
//!
//! The runtime keeps one entry per leased prediction data stream so concurrent
//! subscribe/unsubscribe requests cannot double-count or leak a lease when the
//! engine restarts or clears the runtime.

use std::collections::BTreeMap;

#[derive(Default)]
pub(crate) struct PredictionSubscriptionState {
    pub(crate) counts: BTreeMap<String, usize>,
    pub(crate) leases: BTreeMap<String, PredictionSubscriptionLease>,
    pub(crate) sequence: u64,
}

pub(crate) struct PredictionSubscriptionLease {
    pub(crate) key: String,
    pub(crate) code: String,
    pub(crate) _data_types: Vec<String>,
}

impl PredictionSubscriptionState {
    #[allow(dead_code)]
    pub(crate) fn clear(&mut self) {
        self.counts.clear();
        self.leases.clear();
        self.sequence = 0;
    }
}
