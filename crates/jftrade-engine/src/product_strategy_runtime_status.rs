use serde::{Deserialize, Serialize};
use serde_json::{Value, json};

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct StrategyRuntimeActiveInstance {
    pub instance_id: String,
    pub definition_name: String,
    pub actual_status: String,
    pub active_symbols: Option<Vec<String>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_closed_kline_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_signal_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_order_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_error_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_error: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub updated_at: Option<String>,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct StrategyRuntimeSummary {
    pub status: String,
    pub active_strategies: usize,
    pub supports_backtest_parity: bool,
    pub active_instances: Vec<StrategyRuntimeActiveInstance>,
}

impl StrategyRuntimeSummary {
    fn idle() -> Self {
        Self {
            status: "idle".to_owned(),
            supports_backtest_parity: true,
            ..Self::default()
        }
    }
}

pub trait StrategyRuntimeStatusPort: Send + Sync + std::fmt::Debug {
    fn snapshot(&self) -> StrategyRuntimeSummary;
}

impl StrategyRuntimeStatusPort for jftrade_strategy::StrategyRuntimeRegistry {
    fn snapshot(&self) -> StrategyRuntimeSummary {
        let snapshot = jftrade_strategy::StrategyRuntimeRegistry::snapshot(self);
        StrategyRuntimeSummary {
            status: snapshot.status().to_owned(),
            active_strategies: snapshot.active_strategies(),
            supports_backtest_parity: true,
            active_instances: snapshot
                .active_instances
                .into_iter()
                .map(|instance| StrategyRuntimeActiveInstance {
                    instance_id: instance.instance_id,
                    definition_name: instance.definition_name,
                    actual_status: instance.actual_state.as_str().to_owned(),
                    active_symbols: Some(instance.active_symbols),
                    last_closed_kline_at: timestamp_text(instance.last_closed_kline_at),
                    last_signal_at: timestamp_text(instance.last_signal_at),
                    last_order_at: timestamp_text(instance.last_order_at),
                    last_error_at: timestamp_text(instance.last_error_at),
                    last_error: instance.last_error,
                    updated_at: timestamp_text(instance.updated_at),
                })
                .collect(),
        }
    }
}

fn timestamp_text(value: Option<jftrade_kernel::WireTimestamp>) -> Option<String> {
    value.map(|timestamp| {
        jftrade_kernel::WireTimestamp::from_offset_datetime(
            timestamp.into_inner().to_offset(time::UtcOffset::UTC),
        )
        .to_string()
    })
}

pub(crate) fn strategy_runtime_projection(port: Option<&dyn StrategyRuntimeStatusPort>) -> Value {
    let summary = port.map_or_else(StrategyRuntimeSummary::idle, |port| port.snapshot());
    json!(summary)
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
        state: StrategyRuntimeSummary,
        expected: Value,
    }

    #[derive(Debug)]
    struct FixturePort(StrategyRuntimeSummary);

    impl StrategyRuntimeStatusPort for FixturePort {
        fn snapshot(&self) -> StrategyRuntimeSummary {
            self.0.clone()
        }
    }

    #[test]
    fn strategy_runtime_projection_matches_go_status_corpus() {
        let corpus: Corpus = serde_json::from_str(include_str!(
            "../../../tests/fixtures/compatibility/api-transport/strategy-runtime-status.json"
        ))
        .expect("strategy runtime corpus");
        assert_eq!(corpus.version, "stage9.strategy-runtime-status.v1");
        for case in corpus.cases {
            let port = FixturePort(case.state);
            let actual = strategy_runtime_projection(
                case.port_available
                    .then_some(&port as &dyn StrategyRuntimeStatusPort),
            );
            assert_eq!(actual, case.expected, "case {}", case.name);
        }
    }
}
