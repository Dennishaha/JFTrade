//! Go-compatible prediction push cache owned by the trade-read runtime.
//!
//! Go's `ProductFeatureService` registers one listener per broker id through
//! `PredictionMarketStreamSource.OnPredictionMarketUpdate`, stores the newest
//! update per `brokerID|INSTRUMENT|DATATYPE` key, and answers a matching read
//! from that cache when the sample is at most five seconds old. Keeping the
//! same owner here means a prediction depth/history read can be served by a
//! push frame the OpenD session already delivered instead of polling, and the
//! HTTP route, the extension executor and the subscription port all observe
//! the same cache.

use std::collections::BTreeMap;
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use serde_json::{Value, json};

/// Go hard-codes a five second freshness window for push-served reads.
pub(crate) const PREDICTION_PUSH_TTL: Duration = Duration::from_secs(5);

/// One cached push sample: the sequence Go de-duplicates on, the timestamp the
/// push carries, the projected entries, and the local receive instant used for
/// the freshness window.
#[derive(Clone, Debug)]
pub(crate) struct PredictionPushSample {
    pub(crate) sequence: String,
    pub(crate) as_of: String,
    pub(crate) entries: Vec<Value>,
    pub(crate) received_at: Instant,
}

/// Per-runtime push cache. `registered` mirrors Go's
/// `predictionPushUnsubscribe` map so the listener is attached exactly once per
/// broker even when several acquire/read calls race.
#[derive(Clone, Default)]
pub(crate) struct PredictionPushCache {
    inner: Arc<Mutex<PredictionPushCacheInner>>,
}

#[derive(Default)]
struct PredictionPushCacheInner {
    samples: BTreeMap<String, PredictionPushSample>,
    registered: BTreeMap<String, ()>,
}

impl std::fmt::Debug for PredictionPushCache {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("PredictionPushCache")
            .field("sample_count", &self.len())
            .finish()
    }
}

impl super::SharedTradeReadRuntime {
    /// Go runs `predictionEligibility` for every `prediction.*` feature before
    /// the adapter is reached, so the RFQ route must prove the selected account
    /// is a FUTUINC account with US authority.  The query-based owner lives in
    /// `product_production_ports_market_data_prediction`; this variant takes
    /// the value the request body already normalized.
    pub(crate) fn prediction_combo_quote_eligibility(
        &self,
        account_id: &str,
    ) -> Result<String, String> {
        let query = format!("accountId={account_id}");
        crate::product::product_production_ports::product_production_ports_market_data_prediction::
            prediction_account_eligibility(self, &query)
    }
}

impl PredictionPushCache {
    #[allow(dead_code)]
    pub(crate) fn len(&self) -> usize {
        self.inner
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .samples
            .len()
    }

    /// Mark one broker id as registered. Returns `true` when this call is the
    /// one that must attach the OpenD listener, i.e. Go's
    /// `if s.predictionPushUnsubscribe[brokerID] != nil { return }` guard.
    ///
    /// The listener that will call [`Self::ingest`] is attached by the runtime
    /// wiring; this guard keeps the attach point idempotent per broker.
    #[must_use]
    #[allow(dead_code)]
    pub(crate) fn register(&self, broker_id: &str) -> bool {
        let broker_id = broker_id.trim().to_ascii_lowercase();
        let mut inner = self.inner.lock().unwrap_or_else(|error| error.into_inner());
        if inner.registered.contains_key(&broker_id) {
            return false;
        }
        inner.registered.insert(broker_id, ());
        true
    }

    /// Store one update. An equal non-empty sequence is a duplicate and keeps
    /// the previous sample; a sequence-free push always replaces it, which is
    /// Go's `!exists || update.Sequence == "" || current.Sequence !=
    /// update.Sequence` condition.
    /// Ingest one decoded push frame. This is the runtime entry point the
    /// OpenD push listener calls, so the cache stays the single owner of push
    /// state instead of every read port growing its own copy.
    #[allow(dead_code)]
    pub(crate) fn ingest(
        &self,
        broker_id: &str,
        instrument_id: &str,
        data_type: &str,
        sequence: &str,
        as_of: &str,
        entries: Vec<Value>,
    ) {
        self.store(
            broker_id,
            instrument_id,
            data_type,
            sequence,
            as_of,
            entries,
        );
    }

    #[allow(dead_code)]
    pub(crate) fn store(
        &self,
        broker_id: &str,
        instrument_id: &str,
        data_type: &str,
        sequence: &str,
        as_of: &str,
        entries: Vec<Value>,
    ) {
        self.store_at(
            broker_id,
            instrument_id,
            data_type,
            sequence,
            as_of,
            entries,
            Instant::now(),
        );
    }

    /// Store with an explicit receive instant so the freshness boundary is
    /// deterministic in tests.
    #[allow(clippy::too_many_arguments)]
    #[allow(dead_code)]
    pub(crate) fn store_at(
        &self,
        broker_id: &str,
        instrument_id: &str,
        data_type: &str,
        sequence: &str,
        as_of: &str,
        entries: Vec<Value>,
        received_at: Instant,
    ) {
        let key = prediction_push_key(broker_id, instrument_id, data_type);
        let sequence = sequence.trim().to_owned();
        let mut inner = self.inner.lock().unwrap_or_else(|error| error.into_inner());
        if let Some(current) = inner.samples.get(&key)
            && !sequence.is_empty()
            && current.sequence == sequence
        {
            return;
        }
        inner.samples.insert(
            key,
            PredictionPushSample {
                sequence,
                as_of: as_of.trim().to_owned(),
                entries,
                received_at,
            },
        );
    }

    /// Resolve a fresh sample for one read and render Go's push-backed
    /// `FeatureResult`: `asOf` is the push timestamp, `entries` are the pushed
    /// rows, and metadata carries the source, data type and sequence. A stale
    /// or unknown sample answers `None` so the caller polls the provider.
    pub(crate) fn result(
        &self,
        broker_id: &str,
        instrument_id: &str,
        data_type: &str,
    ) -> Option<Value> {
        self.result_at(broker_id, instrument_id, data_type, Instant::now())
    }

    pub(crate) fn result_at(
        &self,
        broker_id: &str,
        instrument_id: &str,
        data_type: &str,
        now: Instant,
    ) -> Option<Value> {
        let key = prediction_push_key(broker_id, instrument_id, data_type);
        let inner = self.inner.lock().unwrap_or_else(|error| error.into_inner());
        let sample = inner.samples.get(&key)?;
        if now.saturating_duration_since(sample.received_at) > PREDICTION_PUSH_TTL {
            return None;
        }
        Some(json!({
            "asOf": sample.as_of,
            "entries": sample.entries,
            "metadata": {
                "source": "push",
                "dataType": data_type.trim().to_ascii_uppercase(),
                "sequence": sample.sequence,
            },
        }))
    }
}

impl super::SharedTradeReadRuntime {
    /// Resolve a push-served prediction read before polling, mirroring Go's
    /// `predictionPushResult`. Returns `None` when no sample exists for the key
    /// or when the sample is older than the five second window.
    pub(crate) fn prediction_push_result(
        &self,
        broker_id: &str,
        instrument_id: &str,
        data_type: &str,
    ) -> Option<Value> {
        self.prediction_push_cache
            .result(broker_id, instrument_id, data_type)
    }
}

/// Go's `predictionPushKey`: broker id folds to lower case, instrument and data
/// type fold to upper case.
pub(crate) fn prediction_push_key(
    broker_id: &str,
    instrument_id: &str,
    data_type: &str,
) -> String {
    format!(
        "{}|{}|{}",
        broker_id.trim().to_ascii_lowercase(),
        instrument_id.trim().to_ascii_uppercase(),
        data_type.trim().to_ascii_uppercase()
    )
}

#[cfg(test)]
#[path = "product_trade_runtime_prediction_push_tests.rs"]
mod tests;
