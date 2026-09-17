//! Composition-root adapter from the kernel OpenD call port to the API
//! observability recorder.
//!
//! `jftrade-api` intentionally does not depend on `jftrade-kernel`, so the
//! adapter lives in the engine, which already owns both crates. This keeps the
//! transport crate free of API knowledge and still guarantees one writer.

use std::sync::Arc;

use jftrade_kernel::{OpenDCallObserver, OpenDCallRecord};

#[derive(Debug)]
pub(crate) struct TransportMetricsOpenDCallObserver {
    metrics: Arc<jftrade_api::TransportMetrics>,
}

impl TransportMetricsOpenDCallObserver {
    pub(crate) fn new(metrics: Arc<jftrade_api::TransportMetrics>) -> Self {
        Self { metrics }
    }

    pub(crate) fn shared(
        metrics: Arc<jftrade_api::TransportMetrics>,
    ) -> Arc<dyn OpenDCallObserver> {
        Arc::new(Self::new(metrics))
    }
}

impl OpenDCallObserver for TransportMetricsOpenDCallObserver {
    fn record_open_d_call(&self, record: &OpenDCallRecord) {
        self.metrics.record_open_d_call(
            &record.operation,
            &record.request_id,
            record.error.as_deref(),
        );
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn adapter_forwards_correlation_and_failure_text() {
        let metrics = Arc::new(jftrade_api::TransportMetrics::default());
        let observer = TransportMetricsOpenDCallObserver::new(Arc::clone(&metrics));
        observer.record_open_d_call(&OpenDCallRecord {
            operation: " proto_1001 ".to_owned(),
            request_id: " request-opend-1 ".to_owned(),
            error: Some(" opend: client closed ".to_owned()),
        });
        let snapshot = metrics.request_observability_snapshot().open_d;
        assert_eq!(snapshot.total_calls, 1);
        assert_eq!(snapshot.failed_calls, 1);
        assert_eq!(snapshot.last_operation.as_deref(), Some("proto_1001"));
        assert_eq!(snapshot.last_request_id.as_deref(), Some("request-opend-1"));
        assert_eq!(snapshot.last_error.as_deref(), Some("opend: client closed"));
    }
}
