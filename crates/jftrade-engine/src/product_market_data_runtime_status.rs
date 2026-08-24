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

    #[test]
    fn market_data_runtime_projection_matches_go_status_corpus() {
        let corpus: Corpus = serde_json::from_str(include_str!(
            "../../../tests/fixtures/rust-migration/stage9/market-data-runtime-status.json"
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
}
