//! Transport-neutral OpenD call observation boundary.
//!
//! Go's `pkg/futu/opend.Client.Call` records every OpenD RPC into the request
//! observability recorder running on the calling context
//! (`pkg/observability.RecordOpenDCall`). The Rust transport lives in
//! `jftrade-integration-futu`, which must not depend on the API transport
//! crate, so the recorder is expressed here as a tiny port that the
//! composition root adapts. This keeps the correlation fields
//! (`operation`, request ID, error) owned by OpenD without creating a second
//! observability writer.

use std::sync::Arc;

/// One completed OpenD RPC observation.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct OpenDCallRecord {
    /// Go's `fmt.Sprintf("proto_%d", protoID)` form, sanitized by the adapter.
    pub operation: String,
    /// Request correlation ID carried by the surrounding API request, when the
    /// transport was driven by one. Empty for background sessions.
    pub request_id: String,
    /// Sanitized failure text. `None` marks a successful call.
    pub error: Option<String>,
}

/// Sink for completed OpenD RPC observations.
///
/// Implementations must stay non-blocking: `OpenDManagedSession::call` invokes
/// this on the caller thread after the response resolves.
pub trait OpenDCallObserver: Send + Sync + std::fmt::Debug {
    fn record_open_d_call(&self, record: &OpenDCallRecord);
}

/// Shared observer handle stored by the OpenD transport.
pub type SharedOpenDCallObserver = Arc<dyn OpenDCallObserver>;

/// Observer used when no composition root wired observability.
#[derive(Debug, Default)]
pub struct NoopOpenDCallObserver;

impl OpenDCallObserver for NoopOpenDCallObserver {
    fn record_open_d_call(&self, _: &OpenDCallRecord) {}
}

/// Convenience handle for the default no-op observer.
pub static NOOP_OPEN_D_CALL_OBSERVER: NoopOpenDCallObserver = NoopOpenDCallObserver;

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Mutex;

    #[derive(Debug, Default)]
    struct RecordingObserver {
        records: Mutex<Vec<OpenDCallRecord>>,
    }

    impl OpenDCallObserver for RecordingObserver {
        fn record_open_d_call(&self, record: &OpenDCallRecord) {
            self.records.lock().expect("records").push(record.clone());
        }
    }

    #[test]
    fn noop_observer_accepts_records_without_side_effects() {
        NOOP_OPEN_D_CALL_OBSERVER.record_open_d_call(&OpenDCallRecord {
            operation: "proto_1001".to_owned(),
            request_id: "request-opend-1".to_owned(),
            error: Some("opend: client closed".to_owned()),
        });
    }

    #[test]
    fn observer_port_preserves_correlation_fields() {
        let observer = RecordingObserver::default();
        observer.record_open_d_call(&OpenDCallRecord {
            operation: "proto_1001".to_owned(),
            request_id: "request-opend-1".to_owned(),
            error: None,
        });
        let records = observer.records.lock().expect("records");
        assert_eq!(records.len(), 1);
        assert_eq!(records[0].operation, "proto_1001");
        assert_eq!(records[0].request_id, "request-opend-1");
        assert!(records[0].error.is_none());
    }
}
