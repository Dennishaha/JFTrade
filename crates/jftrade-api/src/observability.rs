use std::sync::Mutex;
use std::sync::atomic::{AtomicU64, AtomicUsize, Ordering};
use std::time::Duration;

use serde::Serialize;
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

const DEFAULT_EVENT_LIMIT: usize = 20;
const DEFAULT_SLOW_THRESHOLD: Duration = Duration::from_millis(750);
const OBSERVABILITY_MIN_IMPORTANCE_ENV: &str = "JFTRADE_OBSERVABILITY_MIN_IMPORTANCE";

#[derive(Debug)]
pub struct TransportMetrics {
    started: AtomicU64,
    completed: AtomicU64,
    failures: AtomicU64,
    in_flight: AtomicUsize,
    event_limit: usize,
    slow_threshold: Duration,
    minimum_importance: &'static str,
    observability: Mutex<TransportObservability>,
}

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub struct TransportSnapshot {
    pub started: u64,
    pub completed: u64,
    pub failures: u64,
    pub in_flight: usize,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RequestObservabilitySnapshot {
    pub recent_errors: Vec<TransportEvent>,
    pub recent_slow_requests: Vec<TransportEvent>,
    pub open_d: OpenDHealth,
    pub slow_threshold_ms: u64,
    pub minimum_importance: &'static str,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct TransportEvent {
    pub at: String,
    pub level: &'static str,
    pub importance: &'static str,
    pub message: &'static str,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub error: Option<String>,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub method: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub path: String,
    #[serde(skip_serializing_if = "is_zero_u16")]
    pub status: u16,
    #[serde(skip_serializing_if = "is_zero_u64")]
    pub latency_ms: u64,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub request_id: String,
    pub source: &'static str,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OpenDHealth {
    pub total_calls: u64,
    pub failed_calls: u64,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_call_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_success_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_error_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_error: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_operation: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_request_id: Option<String>,
}

#[derive(Debug, Default)]
struct TransportObservability {
    recent_errors: Vec<TransportEvent>,
    recent_slow_requests: Vec<TransportEvent>,
    open_d: OpenDHealth,
}

impl Default for TransportMetrics {
    fn default() -> Self {
        let minimum_importance = std::env::var(OBSERVABILITY_MIN_IMPORTANCE_ENV)
            .ok()
            .map(|value| normalize_minimum_importance(&value))
            .unwrap_or("low");
        Self::new(
            DEFAULT_EVENT_LIMIT,
            DEFAULT_SLOW_THRESHOLD,
            minimum_importance,
        )
    }
}

impl TransportMetrics {
    pub fn new(event_limit: usize, slow_threshold: Duration, minimum_importance: &str) -> Self {
        Self {
            started: AtomicU64::new(0),
            completed: AtomicU64::new(0),
            failures: AtomicU64::new(0),
            in_flight: AtomicUsize::new(0),
            event_limit: if event_limit == 0 {
                DEFAULT_EVENT_LIMIT
            } else {
                event_limit
            },
            slow_threshold: if slow_threshold.is_zero() {
                DEFAULT_SLOW_THRESHOLD
            } else {
                slow_threshold
            },
            minimum_importance: normalize_minimum_importance(minimum_importance),
            observability: Mutex::new(TransportObservability::default()),
        }
    }

    pub(crate) fn start(&self) {
        self.started.fetch_add(1, Ordering::Relaxed);
        self.in_flight.fetch_add(1, Ordering::Relaxed);
    }

    pub(crate) fn finish_request(
        &self,
        method: &str,
        path: &str,
        status: u16,
        latency: Duration,
        request_id: &str,
    ) {
        self.completed.fetch_add(1, Ordering::Relaxed);
        if status >= 500 {
            self.failures.fetch_add(1, Ordering::Relaxed);
        }
        self.in_flight.fetch_sub(1, Ordering::Relaxed);
        self.record_http_request(method, path, status, latency, request_id);
    }

    pub fn snapshot(&self) -> TransportSnapshot {
        TransportSnapshot {
            started: self.started.load(Ordering::Relaxed),
            completed: self.completed.load(Ordering::Relaxed),
            failures: self.failures.load(Ordering::Relaxed),
            in_flight: self.in_flight.load(Ordering::Relaxed),
        }
    }

    pub fn request_observability_snapshot(&self) -> RequestObservabilitySnapshot {
        let state = self
            .observability
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        RequestObservabilitySnapshot {
            recent_errors: state.recent_errors.clone(),
            recent_slow_requests: state.recent_slow_requests.clone(),
            open_d: state.open_d.clone(),
            slow_threshold_ms: self.slow_threshold.as_millis() as u64,
            minimum_importance: self.minimum_importance,
        }
    }

    pub fn record_open_d_call(&self, operation: &str, request_id: &str, error: Option<&str>) {
        let now = now_rfc3339();
        let mut state = self
            .observability
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        state.open_d.total_calls = state.open_d.total_calls.saturating_add(1);
        state.open_d.last_call_at = Some(now.clone());
        state.open_d.last_operation = non_empty_summary(operation);
        state.open_d.last_request_id = non_empty_trimmed(request_id);
        match error {
            Some(error) => {
                state.open_d.failed_calls = state.open_d.failed_calls.saturating_add(1);
                state.open_d.last_error_at = Some(now.clone());
                state.open_d.last_error = non_empty_summary(error);
                if importance_meets("high", self.minimum_importance) {
                    let event = open_d_failure_event(&now, request_id, error);
                    prepend_bounded(&mut state.recent_errors, event, self.event_limit);
                }
            }
            None => state.open_d.last_success_at = Some(now),
        }
    }

    fn record_http_request(
        &self,
        method: &str,
        path: &str,
        status: u16,
        latency: Duration,
        request_id: &str,
    ) {
        let mut event = request_event(method, path, status, latency, request_id);
        let mut state = self
            .observability
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        if latency >= self.slow_threshold && importance_meets("low", self.minimum_importance) {
            prepend_bounded(
                &mut state.recent_slow_requests,
                event.clone(),
                self.event_limit,
            );
        }
        if status >= 500 && importance_meets("high", self.minimum_importance) {
            event.level = "error";
            event.importance = "high";
            event.message = "api request failed";
            event.error = Some(format!("HTTP {status}"));
            prepend_bounded(&mut state.recent_errors, event, self.event_limit);
        }
    }
}

fn request_event(
    method: &str,
    path: &str,
    status: u16,
    latency: Duration,
    request_id: &str,
) -> TransportEvent {
    TransportEvent {
        at: now_rfc3339(),
        level: "info",
        importance: "low",
        message: "api request",
        error: None,
        method: method.trim().to_owned(),
        path: path.trim().to_owned(),
        status,
        latency_ms: latency.as_millis() as u64,
        request_id: request_id.trim().to_owned(),
        source: "api",
    }
}

/// Go's `Recorder.RecordOpenDCall` funnels a failed RPC through
/// `ErrorWithImportance`, so the recorder that owns the OpenD health counters
/// also prepends one `opend call failed` event. That event carries only the
/// correlation fields and the sanitized error; Go's extra `operation` and
/// `latency_ms` arguments are structured-log attributes, not recorded fields.
fn open_d_failure_event(at: &str, request_id: &str, error: &str) -> TransportEvent {
    TransportEvent {
        at: at.to_owned(),
        level: "error",
        importance: "high",
        message: "opend call failed",
        error: non_empty_summary(error),
        method: String::new(),
        path: String::new(),
        status: 0,
        latency_ms: 0,
        request_id: request_id.trim().to_owned(),
        source: "opend",
    }
}

fn now_rfc3339() -> String {
    OffsetDateTime::now_utc()
        .format(&Rfc3339)
        .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned())
}

fn sanitize_summary_text(value: &str) -> String {
    const MAX_SUMMARY_TEXT_BYTES: usize = 500;
    let value = value.split_whitespace().collect::<Vec<_>>().join(" ");
    if value.len() <= MAX_SUMMARY_TEXT_BYTES {
        return value;
    }
    let boundary = value
        .char_indices()
        .map(|(index, _)| index)
        .take_while(|index| *index <= MAX_SUMMARY_TEXT_BYTES)
        .last()
        .unwrap_or(0);
    format!("{}...", &value[..boundary])
}

fn non_empty_summary(value: &str) -> Option<String> {
    let value = sanitize_summary_text(value);
    (!value.is_empty()).then_some(value)
}

fn non_empty_trimmed(value: &str) -> Option<String> {
    let value = value.trim();
    (!value.is_empty()).then(|| value.to_owned())
}

const fn is_zero_u16(value: &u16) -> bool {
    *value == 0
}

const fn is_zero_u64(value: &u64) -> bool {
    *value == 0
}

fn prepend_bounded(events: &mut Vec<TransportEvent>, event: TransportEvent, limit: usize) {
    events.insert(0, event);
    events.truncate(limit);
}

fn normalize_minimum_importance(value: &str) -> &'static str {
    match value.trim().to_ascii_lowercase().as_str() {
        "low" | "debug" | "trace" => "low",
        "normal" | "info" | "default" => "normal",
        "high" | "warn" | "warning" | "error" => "high",
        "critical" | "fatal" | "panic" => "critical",
        _ => "low",
    }
}

fn importance_meets(value: &str, minimum: &str) -> bool {
    importance_rank(value) >= importance_rank(minimum)
}

fn importance_rank(value: &str) -> u8 {
    // Go ranks `NormalizeImportance(value)`, so aliases rank as their canonical
    // level and every unknown value ranks as `normal`.
    match value.trim().to_ascii_lowercase().as_str() {
        "low" | "debug" | "trace" => 0,
        "normal" | "info" | "default" => 1,
        "high" | "warn" | "warning" | "error" => 2,
        "critical" | "fatal" | "panic" => 3,
        _ => 1,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde::Deserialize;
    use serde_json::{Value, json};

    #[derive(Deserialize)]
    #[serde(rename_all = "camelCase")]
    struct ParityFixture {
        event_limit: usize,
        slow_threshold_ms: u64,
        minimum_importance: String,
        requests: Vec<ParityRequest>,
        open_d_calls: Vec<ParityOpenDCall>,
        expected: Value,
    }

    #[derive(Deserialize)]
    #[serde(rename_all = "camelCase")]
    struct ParityRequest {
        method: String,
        path: String,
        status: u16,
        latency_ms: u64,
        request_id: String,
    }

    #[derive(Deserialize)]
    #[serde(rename_all = "camelCase")]
    struct ParityOpenDCall {
        operation: String,
        request_id: String,
        error: Option<String>,
    }

    // Parity: go:452dea11:internal/app/apiserver/servercore/request_observability_test.go:14 TestRequestObservabilityInjectsStableContextAndRecordsSummary
    #[test]
    fn request_observability_matches_go_shape_and_bounded_order() {
        let metrics = TransportMetrics::new(2, Duration::from_millis(10), "low");
        for (path, status) in [("/first", 200), ("/second", 503), ("/third", 500)] {
            metrics.start();
            metrics.finish_request("GET", path, status, Duration::from_millis(10), "request-id");
        }

        let snapshot = metrics.request_observability_snapshot();
        assert_eq!(snapshot.recent_slow_requests.len(), 2);
        assert_eq!(snapshot.recent_slow_requests[0].path, "/third");
        assert_eq!(snapshot.recent_slow_requests[1].path, "/second");
        assert_eq!(snapshot.recent_errors.len(), 2);
        assert_eq!(snapshot.recent_errors[0].error.as_deref(), Some("HTTP 500"));
        assert_eq!(snapshot.open_d, OpenDHealth::default());
        assert_eq!(snapshot.slow_threshold_ms, 10);
        assert_eq!(snapshot.minimum_importance, "low");
        assert_eq!(metrics.snapshot().failures, 2);
    }

    // Parity: go:452dea11:pkg/observability/observability_test.go:45 TestRecorderSnapshotSerializesEmptyCollectionsAsArrays
    #[test]
    fn request_observability_snapshot_serializes_empty_collections_as_arrays() {
        let snapshot = serde_json::to_value(
            TransportMetrics::new(5, Duration::from_secs(1), "low")
                .request_observability_snapshot(),
        )
        .expect("serialize empty request observability");

        assert_eq!(snapshot["recentErrors"], json!([]));
        assert_eq!(snapshot["recentSlowRequests"], json!([]));
        assert_eq!(
            snapshot["openD"],
            json!({"totalCalls": 0, "failedCalls": 0})
        );
    }

    // Parity: go:452dea11:pkg/observability/observability_test.go:113 TestRecorderBoundsErrorsSlowRequestsAndOpenDHealth
    #[test]
    fn recorder_bounds_errors_slow_requests_and_open_d_correlation_like_go() {
        let metrics = TransportMetrics::new(2, Duration::from_millis(10), "low");
        metrics.record_http_request("GET", "/slow", 200, Duration::from_millis(15), "request-1");
        metrics.record_http_request(
            "POST",
            "/failed",
            503,
            Duration::from_millis(1),
            "request-1",
        );
        metrics.record_open_d_call("proto_3006", "request-1", Some("quote permission denied"));
        metrics.record_http_request(
            "GET",
            "/failed-again",
            500,
            Duration::from_millis(1),
            "request-1",
        );

        let snapshot = metrics.request_observability_snapshot();
        assert_eq!(snapshot.recent_errors.len(), 2);
        assert_eq!(snapshot.recent_slow_requests.len(), 1);
        assert_eq!(snapshot.recent_slow_requests[0].path, "/slow");
        assert_eq!(snapshot.recent_slow_requests[0].importance, "low");
        for event in &snapshot.recent_errors {
            assert_eq!(event.importance, "high", "error event: {event:?}");
        }
        assert_eq!(snapshot.recent_errors[0].source, "api");
        assert_eq!(snapshot.recent_errors[0].error.as_deref(), Some("HTTP 500"));
        assert_eq!(snapshot.recent_errors[1].source, "opend");
        assert_eq!(snapshot.recent_errors[1].message, "opend call failed");
        assert_eq!(snapshot.recent_errors[1].request_id, "request-1");
        assert_eq!(snapshot.open_d.total_calls, 1);
        assert_eq!(snapshot.open_d.failed_calls, 1);
        assert_eq!(
            snapshot.open_d.last_operation.as_deref(),
            Some("proto_3006")
        );
        assert_eq!(
            snapshot.open_d.last_request_id.as_deref(),
            Some("request-1")
        );
        assert_eq!(snapshot.minimum_importance, "low");
    }

    // Parity: go:452dea11:pkg/observability/observability_test.go:62 TestImportanceThresholdSuppressesLowerImportanceLogs
    #[test]
    fn importance_threshold_suppresses_slow_events_and_keeps_high_events() {
        let metrics = TransportMetrics::new(5, Duration::from_secs(1), "high");
        metrics.record_http_request("GET", "/slow", 200, Duration::from_secs(2), "request-1");
        metrics.record_open_d_call("proto_3007", "request-1", Some("quote permission denied"));
        metrics.record_http_request("GET", "/failed", 500, Duration::from_millis(1), "request-1");

        let snapshot = metrics.request_observability_snapshot();
        assert!(snapshot.recent_slow_requests.is_empty());
        assert_eq!(snapshot.recent_errors.len(), 2);
        assert_eq!(snapshot.recent_errors[0].source, "api");
        assert_eq!(snapshot.recent_errors[1].source, "opend");
        assert_eq!(snapshot.minimum_importance, "high");
    }

    // Parity: go:452dea11:pkg/observability/observability_test.go:94 TestGlobalImportanceThresholdAppliesWithoutRecorder
    #[test]
    fn minimum_importance_aliases_normalize_like_go() {
        for (input, expected) in [
            ("low", "low"),
            ("DEBUG", "low"),
            (" trace ", "low"),
            ("normal", "normal"),
            ("info", "normal"),
            ("default", "normal"),
            ("high", "high"),
            ("warn", "high"),
            ("warning", "high"),
            ("error", "high"),
            ("critical", "critical"),
            ("fatal", "critical"),
            ("panic", "critical"),
            ("unknown", "low"),
            ("", "low"),
        ] {
            assert_eq!(
                normalize_minimum_importance(input),
                expected,
                "minimum importance {input}"
            );
        }
    }

    // Parity: go:452dea11:pkg/observability/context_detach_and_importance_test.go:10 TestObservabilityBackgroundContextsAndImportanceRanks
    #[test]
    fn importance_rank_orders_go_levels_and_defaults_unknown_to_normal() {
        for (input, expected) in [
            ("low", 0),
            ("normal", 1),
            ("high", 2),
            ("critical", 3),
            ("debug", 0),
            ("info", 1),
            ("warning", 2),
            ("fatal", 3),
            ("unknown", 1),
            ("", 1),
        ] {
            assert_eq!(importance_rank(input), expected, "rank of {input}");
        }
        assert!(importance_meets("critical", "high"));
        assert!(importance_meets("high", "high"));
        assert!(!importance_meets("low", "high"));
    }

    // Parity: go:452dea11:pkg/observability/observability_test.go:163 TestObservabilityBoundaryDefaultsAndGlobalOpenDLogging
    #[test]
    fn recorder_defaults_fall_back_to_go_limits_and_thresholds() {
        let metrics = TransportMetrics::new(0, Duration::ZERO, "weird");
        for index in 0..25 {
            metrics.record_http_request(
                "GET",
                &format!("/failed-{index}"),
                500,
                Duration::from_millis(1),
                "request-1",
            );
        }

        let snapshot = metrics.request_observability_snapshot();
        assert_eq!(snapshot.slow_threshold_ms, 750);
        assert_eq!(snapshot.minimum_importance, "low");
        assert_eq!(snapshot.recent_errors.len(), 20);
        assert_eq!(snapshot.recent_errors[0].path, "/failed-24");
        assert_eq!(snapshot.recent_errors[19].path, "/failed-5");
    }

    // Parity: go:452dea11:pkg/observability/observability_test.go:163 TestObservabilityBoundaryDefaultsAndGlobalOpenDLogging
    #[test]
    fn summary_text_collapses_whitespace_and_truncates_at_go_limit() {
        assert_eq!(
            sanitize_summary_text("  quote   permission \n denied "),
            "quote permission denied"
        );
        assert_eq!(sanitize_summary_text(" \t "), "");

        let long = "x ".repeat(400);
        let sanitized = sanitize_summary_text(&long);
        assert_eq!(sanitized, format!("{}...", "x ".repeat(250)));
        assert_eq!(sanitized.len(), 503);
        assert_eq!(sanitize_summary_text(&"é".repeat(400)).len(), 503);
    }

    // Parity: go:452dea11:pkg/observability/observability_test.go:14 TestStructuredLogIncludesCanonicalCorrelationFields
    #[test]
    fn request_observability_events_trim_and_serialize_correlation_keys() {
        let metrics = TransportMetrics::new(5, Duration::from_millis(1), "low");
        metrics.record_http_request(" get ", " /failed ", 503, Duration::from_millis(1), " r-1 ");
        metrics.record_open_d_call(" proto_3007 ", " request-2 ", Some(" quote   permission "));

        let snapshot = serde_json::to_value(metrics.request_observability_snapshot())
            .expect("serialize request observability");
        let open_d_event = &snapshot["recentErrors"][0];
        assert_eq!(open_d_event["requestId"], json!("request-2"));
        assert_eq!(open_d_event["source"], json!("opend"));
        assert_eq!(open_d_event["error"], json!("quote permission"));
        let mut keys = open_d_event
            .as_object()
            .expect("OpenD error event")
            .keys()
            .map(String::as_str)
            .collect::<Vec<_>>();
        keys.sort_unstable();
        assert_eq!(
            keys,
            [
                "at",
                "error",
                "importance",
                "level",
                "message",
                "requestId",
                "source"
            ]
        );

        let http_event = &snapshot["recentErrors"][1];
        assert_eq!(http_event["requestId"], json!("r-1"));
        assert_eq!(http_event["method"], json!("get"));
        assert_eq!(http_event["path"], json!("/failed"));
        assert_eq!(http_event["source"], json!("api"));
    }

    // Parity: go:452dea11:pkg/observability/observability_test.go:215 TestObservabilityNilContextAndOpenDSuccessBoundaries
    #[test]
    fn open_d_success_records_last_success_without_failure_counters() {
        let metrics = TransportMetrics::new(3, Duration::from_secs(1), "low");
        metrics.record_open_d_call("proto_3007", "request-success", None);

        let snapshot = metrics.request_observability_snapshot();
        assert_eq!(snapshot.open_d.total_calls, 1);
        assert_eq!(snapshot.open_d.failed_calls, 0);
        assert_eq!(
            snapshot.open_d.last_operation.as_deref(),
            Some("proto_3007")
        );
        assert_eq!(
            snapshot.open_d.last_request_id.as_deref(),
            Some("request-success")
        );
        assert!(snapshot.open_d.last_success_at.is_some());
        assert!(snapshot.open_d.last_error.is_none());
        assert!(snapshot.open_d.last_error_at.is_none());
        assert!(snapshot.recent_errors.is_empty());

        metrics.record_open_d_call("  ", " ", None);
        let blank = metrics.request_observability_snapshot();
        assert_eq!(blank.open_d.total_calls, 2);
        assert!(blank.open_d.last_operation.is_none());
        assert!(blank.open_d.last_request_id.is_none());
    }

    #[test]
    fn minimum_importance_filters_low_and_high_events_like_go() {
        let metrics = TransportMetrics::new(20, Duration::from_millis(1), "critical");
        metrics.start();
        metrics.finish_request(
            "GET",
            "/failure",
            500,
            Duration::from_millis(2),
            "request-id",
        );

        let snapshot = metrics.request_observability_snapshot();
        assert!(snapshot.recent_slow_requests.is_empty());
        assert!(snapshot.recent_errors.is_empty());
        assert_eq!(snapshot.minimum_importance, "critical");
    }

    #[test]
    fn open_d_health_tracks_success_failure_and_correlation() {
        let metrics = TransportMetrics::default();
        metrics.record_open_d_call(" proto_3006 ", "request-1", None);
        metrics.record_open_d_call(
            "proto_3007",
            "request-2",
            Some(" quote   permission denied "),
        );

        let health = metrics.request_observability_snapshot().open_d;
        assert_eq!(health.total_calls, 2);
        assert_eq!(health.failed_calls, 1);
        assert_eq!(health.last_operation.as_deref(), Some("proto_3007"));
        assert_eq!(health.last_request_id.as_deref(), Some("request-2"));
        assert_eq!(
            health.last_error.as_deref(),
            Some("quote permission denied")
        );
        assert!(health.last_success_at.is_some());
        assert!(health.last_error_at.is_some());
    }

    #[test]
    fn request_observability_matches_frozen_compatibility_corpus() {
        let fixture: ParityFixture = serde_json::from_str(include_str!(
            "../../../tests/fixtures/compatibility/api-transport/request-observability.json"
        ))
        .expect("request observability fixture");
        let metrics = TransportMetrics::new(
            fixture.event_limit,
            Duration::from_millis(fixture.slow_threshold_ms),
            &fixture.minimum_importance,
        );
        for request in fixture.requests {
            metrics.start();
            metrics.finish_request(
                &request.method,
                &request.path,
                request.status,
                Duration::from_millis(request.latency_ms),
                &request.request_id,
            );
        }
        for call in fixture.open_d_calls {
            metrics.record_open_d_call(&call.operation, &call.request_id, call.error.as_deref());
        }

        let mut actual = serde_json::to_value(metrics.request_observability_snapshot())
            .expect("serialize request observability");
        remove_dynamic_timestamps(&mut actual);
        assert_eq!(actual, fixture.expected);
    }

    fn remove_dynamic_timestamps(snapshot: &mut Value) {
        for key in ["recentErrors", "recentSlowRequests"] {
            if let Some(events) = snapshot.get_mut(key).and_then(Value::as_array_mut) {
                for event in events {
                    event.as_object_mut().expect("event object").remove("at");
                }
            }
        }
        let open_d = snapshot
            .get_mut("openD")
            .and_then(Value::as_object_mut)
            .expect("OpenD object");
        for key in ["lastCallAt", "lastSuccessAt", "lastErrorAt"] {
            open_d.remove(key);
        }
    }
}
