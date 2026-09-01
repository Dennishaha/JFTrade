use std::collections::BTreeMap;
use std::sync::RwLock;

use jftrade_kernel::WireTimestamp;
use thiserror::Error;
use time::UtcOffset;

use crate::RuntimeState;

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct RuntimeInstanceSummary {
    pub instance_id: String,
    pub definition_name: String,
    pub actual_state: RuntimeState,
    pub active_symbols: Vec<String>,
    pub last_closed_kline_at: Option<WireTimestamp>,
    pub last_signal_at: Option<WireTimestamp>,
    pub last_order_at: Option<WireTimestamp>,
    pub last_error_at: Option<WireTimestamp>,
    pub last_error: Option<String>,
    pub updated_at: Option<WireTimestamp>,
}

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct RuntimeRegistrySnapshot {
    pub active_instances: Vec<RuntimeInstanceSummary>,
}

impl RuntimeRegistrySnapshot {
    pub fn status(&self) -> &'static str {
        if self.active_instances.is_empty() {
            "idle"
        } else {
            "active"
        }
    }

    pub fn active_strategies(&self) -> usize {
        self.active_instances.len()
    }
}

#[derive(Debug, Default)]
pub struct StrategyRuntimeRegistry {
    instances: RwLock<BTreeMap<String, RuntimeInstanceSummary>>,
}

impl StrategyRuntimeRegistry {
    pub fn upsert(&self, mut instance: RuntimeInstanceSummary) -> Result<(), RuntimeRegistryError> {
        instance.instance_id = instance.instance_id.trim().to_owned();
        if instance.instance_id.is_empty() {
            return Err(RuntimeRegistryError::MissingInstanceId);
        }
        instance.definition_name = instance.definition_name.trim().to_owned();
        instance.active_symbols = normalize_symbols(instance.active_symbols);
        instance.last_closed_kline_at = normalize_timestamp(instance.last_closed_kline_at);
        instance.last_signal_at = normalize_timestamp(instance.last_signal_at);
        instance.last_order_at = normalize_timestamp(instance.last_order_at);
        instance.last_error_at = normalize_timestamp(instance.last_error_at);
        instance.updated_at = normalize_timestamp(instance.updated_at);
        instance.last_error = instance
            .last_error
            .map(|error| error.trim().to_owned())
            .filter(|error| !error.is_empty());
        write_instances(&self.instances).insert(instance.instance_id.clone(), instance);
        Ok(())
    }

    pub fn remove(&self, instance_id: &str) -> bool {
        write_instances(&self.instances)
            .remove(instance_id.trim())
            .is_some()
    }

    pub fn snapshot(&self) -> RuntimeRegistrySnapshot {
        RuntimeRegistrySnapshot {
            active_instances: read_instances(&self.instances).values().cloned().collect(),
        }
    }
}

/// Converts an optional observation timestamp to the UTC wire representation.
///
/// `None` is the Rust equivalent of Go's zero `time.Time`: it is omitted from
/// an observation rather than serialized as a sentinel date. Keeping this
/// normalization in the strategy registry means every consumer receives the
/// same instant and precision regardless of the source offset.
pub fn normalize_timestamp(value: Option<WireTimestamp>) -> Option<WireTimestamp> {
    value.map(|timestamp| {
        WireTimestamp::from_offset_datetime(timestamp.into_inner().to_offset(UtcOffset::UTC))
    })
}

/// Formats an optional observation timestamp using the canonical UTC wire
/// representation. `None` remains omitted.
pub fn format_timestamp(value: Option<WireTimestamp>) -> Option<String> {
    normalize_timestamp(value).map(|timestamp| timestamp.to_string())
}

/// Returns the newest of two optional observation timestamps.
pub fn max_timestamp(
    left: Option<WireTimestamp>,
    right: Option<WireTimestamp>,
) -> Option<WireTimestamp> {
    match (left, right) {
        (None, value) | (value, None) => value,
        (Some(left), Some(right)) => Some(if right > left { right } else { left }),
    }
}

/// Trims an optional diagnostic and omits blank values.
pub fn optional_string(value: Option<String>) -> Option<String> {
    value
        .map(|value| value.trim().to_owned())
        .filter(|value| !value.is_empty())
}

fn normalize_symbols(symbols: Vec<String>) -> Vec<String> {
    let mut symbols = symbols
        .into_iter()
        .map(|symbol| symbol.trim().to_owned())
        .filter(|symbol| !symbol.is_empty())
        .collect::<Vec<_>>();
    symbols.sort();
    symbols.dedup();
    symbols
}

fn read_instances(
    instances: &RwLock<BTreeMap<String, RuntimeInstanceSummary>>,
) -> std::sync::RwLockReadGuard<'_, BTreeMap<String, RuntimeInstanceSummary>> {
    instances.read().unwrap_or_else(|error| error.into_inner())
}

fn write_instances(
    instances: &RwLock<BTreeMap<String, RuntimeInstanceSummary>>,
) -> std::sync::RwLockWriteGuard<'_, BTreeMap<String, RuntimeInstanceSummary>> {
    instances.write().unwrap_or_else(|error| error.into_inner())
}

#[derive(Clone, Debug, Eq, Error, PartialEq)]
pub enum RuntimeRegistryError {
    #[error("strategy runtime instance id is required")]
    MissingInstanceId,
}

#[cfg(test)]
mod tests {
    use super::*;

    fn instance(id: &str, symbols: &[&str]) -> RuntimeInstanceSummary {
        RuntimeInstanceSummary {
            instance_id: id.to_owned(),
            definition_name: format!(" {id} definition "),
            actual_state: RuntimeState::Running,
            active_symbols: symbols.iter().map(|symbol| (*symbol).to_owned()).collect(),
            last_closed_kline_at: None,
            last_signal_at: None,
            last_order_at: None,
            last_error_at: None,
            last_error: Some("  ".to_owned()),
            updated_at: None,
        }
    }

    #[test]
    fn registry_normalizes_replaces_sorts_and_removes_active_instances() {
        let registry = StrategyRuntimeRegistry::default();
        assert_eq!(registry.snapshot().status(), "idle");
        assert_eq!(
            registry.upsert(instance(" ", &[])),
            Err(RuntimeRegistryError::MissingInstanceId)
        );

        registry
            .upsert(instance("z-runtime", &[" US.TSLA ", "US.AAPL", "US.AAPL"]))
            .expect("z runtime");
        registry
            .upsert(instance("a-runtime", &["HK.00700"]))
            .expect("a runtime");
        registry
            .upsert(instance("z-runtime", &["US.MSFT"]))
            .expect("replace z runtime");

        let snapshot = registry.snapshot();
        assert_eq!(snapshot.status(), "active");
        assert_eq!(snapshot.active_strategies(), 2);
        assert_eq!(snapshot.active_instances[0].instance_id, "a-runtime");
        assert_eq!(snapshot.active_instances[1].instance_id, "z-runtime");
        assert_eq!(snapshot.active_instances[1].active_symbols, ["US.MSFT"]);
        assert_eq!(
            snapshot.active_instances[1].definition_name,
            "z-runtime definition"
        );
        assert_eq!(snapshot.active_instances[1].last_error, None);

        assert!(registry.remove(" z-runtime "));
        assert!(!registry.remove("missing"));
        assert_eq!(registry.snapshot().active_strategies(), 1);
    }

    #[test]
    fn observation_values_use_utc_optional_and_newest_timestamp_semantics() {
        let local = "2026-01-02T03:04:05.000000006+01:00"
            .parse::<WireTimestamp>()
            .expect("local timestamp");
        let newer = "2026-01-02T04:04:05.000000006+01:00"
            .parse::<WireTimestamp>()
            .expect("newer timestamp");
        let normalized = normalize_timestamp(Some(local)).expect("normalized timestamp");
        assert_eq!(normalized.to_string(), "2026-01-02T02:04:05.000000006Z");
        assert_eq!(format_timestamp(None), None);
        assert_eq!(
            format_timestamp(Some(local)).as_deref(),
            Some("2026-01-02T02:04:05.000000006Z")
        );
        assert_eq!(max_timestamp(None, Some(local)), Some(local));
        assert_eq!(max_timestamp(Some(local), Some(newer)), Some(newer));
        assert_eq!(
            optional_string(Some("  reason ".to_owned())).as_deref(),
            Some("reason")
        );
        assert_eq!(optional_string(Some("  ".to_owned())), None);

        let registry = StrategyRuntimeRegistry::default();
        registry
            .upsert(RuntimeInstanceSummary {
                instance_id: "runtime-1".to_owned(),
                definition_name: "strategy".to_owned(),
                actual_state: RuntimeState::Running,
                active_symbols: Vec::new(),
                last_closed_kline_at: Some(local),
                last_signal_at: None,
                last_order_at: None,
                last_error_at: None,
                last_error: None,
                updated_at: None,
            })
            .expect("upsert");
        assert_eq!(
            registry.snapshot().active_instances[0]
                .last_closed_kline_at
                .expect("timestamp")
                .to_string(),
            "2026-01-02T02:04:05.000000006Z"
        );
    }
}
