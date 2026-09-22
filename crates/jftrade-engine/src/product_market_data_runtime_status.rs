use jftrade_kernel::WireTimestamp;
use serde::{Deserialize, Serialize};
use serde_json::{Value, json};

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct MarketDataRuntimeState {
    pub connected: bool,
    pub closed: bool,
    pub generation: u64,
    pub active_count: usize,
    pub last_refresh_at: Option<WireTimestamp>,
    pub quote_retry_at: Option<WireTimestamp>,
    pub quote_failures: usize,
    pub quote_last_error: Option<String>,
    pub stream_retry_at: Option<WireTimestamp>,
    pub stream_failures: usize,
    pub stream_last_error: Option<String>,
}

pub trait MarketDataRuntimeStatusPort: Send + Sync + std::fmt::Debug {
    fn snapshot(&self) -> MarketDataRuntimeState;
}

impl MarketDataRuntimeStatusPort for jftrade_marketdata::MarketDataRuntimeRecorder {
    fn snapshot(&self) -> MarketDataRuntimeState {
        let state = jftrade_marketdata::MarketDataRuntimeRecorder::snapshot(self);
        MarketDataRuntimeState {
            connected: state.connected,
            closed: state.closed,
            generation: state.generation,
            active_count: state.active_count,
            last_refresh_at: state.last_refresh_at,
            quote_retry_at: state.quote_retry_at,
            quote_failures: state.quote_failures,
            quote_last_error: state.quote_last_error,
            stream_retry_at: state.stream_retry_at,
            stream_failures: state.stream_failures,
            stream_last_error: state.stream_last_error,
        }
    }
}

pub(crate) fn market_data_runtime_projection(
    port: Option<&dyn MarketDataRuntimeStatusPort>,
) -> Value {
    let Some(port) = port else {
        return market_data_runtime_wire("unavailable", MarketDataRuntimeState::default());
    };
    let state = port.snapshot();
    let status = match () {
        () if state.closed => "closed",
        () if state.connected => "connected",
        () if present(state.quote_last_error.as_deref())
            || present(state.stream_last_error.as_deref()) =>
        {
            "degraded"
        }
        () if state.active_count > 0 => "connecting",
        () => "idle",
    };
    market_data_runtime_wire(status, state)
}

fn market_data_runtime_wire(status: &str, state: MarketDataRuntimeState) -> Value {
    json!({
        "status": status,
        "connected": state.connected,
        "closed": state.closed,
        "generation": state.generation,
        "activeCount": state.active_count,
        "lastRefreshAt": utc_timestamp(state.last_refresh_at),
        "quoteRetryAt": utc_timestamp(state.quote_retry_at),
        "quoteFailures": state.quote_failures,
        "quoteLastError": nonblank(state.quote_last_error.as_deref()),
        "streamRetryAt": utc_timestamp(state.stream_retry_at),
        "streamFailures": state.stream_failures,
        "streamLastError": nonblank(state.stream_last_error.as_deref()),
    })
}

fn utc_timestamp(value: Option<WireTimestamp>) -> Option<WireTimestamp> {
    value.map(|timestamp| {
        WireTimestamp::from_offset_datetime(timestamp.into_inner().to_offset(time::UtcOffset::UTC))
    })
}

fn nonblank(value: Option<&str>) -> Option<&str> {
    value.map(str::trim).filter(|value| !value.is_empty())
}

fn present(value: Option<&str>) -> bool {
    value.is_some_and(|value| !value.is_empty())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[derive(Debug, Deserialize)]
    #[serde(rename_all = "camelCase")]
    struct Corpus {
        version: String,
        cases: Vec<Case>,
    }

    #[derive(Debug, Deserialize)]
    #[serde(rename_all = "camelCase")]
    struct Case {
        name: String,
        port_available: bool,
        state: MarketDataRuntimeState,
        expected: Value,
    }

    #[derive(Debug)]
    struct FixturePort(MarketDataRuntimeState);

    impl MarketDataRuntimeStatusPort for FixturePort {
        fn snapshot(&self) -> MarketDataRuntimeState {
            self.0.clone()
        }
    }

    // Parity: go:452dea11:internal/app/apiserver/status/status_test.go:26 TestMarketDataRuntimeSummaryStates
    #[test]
    fn market_data_runtime_projection_matches_go_status_corpus() {
        let corpus: Corpus = serde_json::from_str(include_str!(
            "../../../tests/fixtures/compatibility/api-transport/market-data-runtime-status.json"
        ))
        .expect("market-data runtime corpus");
        assert_eq!(corpus.version, "stage9.market-data-runtime-status.v1");
        for case in corpus.cases {
            let port = FixturePort(case.state);
            let actual = market_data_runtime_projection(
                case.port_available
                    .then_some(&port as &dyn MarketDataRuntimeStatusPort),
            );
            assert_eq!(actual, case.expected, "case {}", case.name);
        }
    }

    #[test]
    fn runtime_status_wire_drops_absent_and_blank_values_and_normalizes_utc() {
        // Parity: go:452dea11:internal/app/apiserver/status/status_test.go:88 TestTimeAndStringPointers
        // Go returns nil for the zero time and for blank strings, and renders a
        // non-UTC timestamp in UTC (RFC3339). The Rust envelope expresses the
        // same guarantee through Option-based absence and the wire helpers.
        let state = MarketDataRuntimeState {
            last_refresh_at: Some(
                "2026-06-01T12:00:00.123+08:00"
                    .parse::<WireTimestamp>()
                    .expect("timestamp"),
            ),
            quote_last_error: Some("  futu  ".to_owned()),
            stream_last_error: Some("   ".to_owned()),
            ..MarketDataRuntimeState::default()
        };
        let wire = market_data_runtime_wire("idle", state);
        assert_eq!(wire["lastRefreshAt"], "2026-06-01T04:00:00.123Z");
        assert_eq!(wire["quoteLastError"], "futu");
        assert_eq!(wire["streamLastError"], Value::Null);

        let empty = market_data_runtime_wire("idle", MarketDataRuntimeState::default());
        for field in [
            "lastRefreshAt",
            "quoteRetryAt",
            "streamRetryAt",
            "quoteLastError",
            "streamLastError",
        ] {
            assert_eq!(empty[field], Value::Null, "{field} must stay absent");
        }
    }
}
