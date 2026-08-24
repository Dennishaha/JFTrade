use std::collections::BTreeMap;
use std::sync::Arc;

use serde::{Deserialize, Serialize};

use crate::{
    DemandBook, DemandSnapshot, HealthStatus, InstrumentRef, MarketDataError,
    MarketDataRuntimeRecorder, ProviderDescriptor, ProviderReadiness, TickCache,
};

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum ActivationMode {
    Explicit,
    StartupRestore,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ProviderRuntime {
    pub active_provider: String,
    pub generation: u64,
    pub readiness: ProviderReadiness,
    pub connected: bool,
    pub active_demand: usize,
}

#[derive(Clone, Debug)]
struct ProviderSlot {
    descriptor: ProviderDescriptor,
    health: HealthStatus,
}

#[derive(Clone, Debug)]
pub struct ProviderRouter {
    providers: BTreeMap<String, ProviderSlot>,
    active: String,
    generation: u64,
    demand: DemandBook,
    cache: TickCache,
    runtime_recorder: Arc<MarketDataRuntimeRecorder>,
}

impl ProviderRouter {
    pub fn new(cache_capacity: usize) -> Self {
        Self {
            providers: BTreeMap::new(),
            active: String::new(),
            generation: 0,
            demand: DemandBook::default(),
            cache: TickCache::new(cache_capacity),
            runtime_recorder: Arc::new(MarketDataRuntimeRecorder::default()),
        }
    }

    pub fn register(
        &mut self,
        descriptor: ProviderDescriptor,
        health: HealthStatus,
    ) -> Result<(), MarketDataError> {
        descriptor.validate()?;
        self.providers.insert(
            descriptor.selection_id.clone(),
            ProviderSlot { descriptor, health },
        );
        Ok(())
    }

    pub fn update_health(
        &mut self,
        provider_id: &str,
        health: HealthStatus,
    ) -> Result<(), MarketDataError> {
        let slot = self
            .providers
            .get_mut(provider_id)
            .ok_or_else(|| MarketDataError::ProviderNotFound(provider_id.to_owned()))?;
        slot.health = health;
        if provider_id == self.active {
            let runtime = self.runtime_recorder.snapshot();
            let _ = self.runtime_recorder.set_stream_state(
                runtime.generation,
                slot.health.connected && runtime.active_count > 0,
                (runtime.active_count > 0)
                    .then(|| slot.health.last_error.clone())
                    .flatten(),
            );
        }
        Ok(())
    }

    pub fn activate(
        &mut self,
        provider_id: &str,
        mode: ActivationMode,
    ) -> Result<ProviderRuntime, MarketDataError> {
        if self.demand.has_managed_consumers() && self.active != provider_id {
            return Err(MarketDataError::ManagedSubscriptionsActive);
        }
        let slot = self
            .providers
            .get(provider_id)
            .ok_or_else(|| MarketDataError::ProviderNotFound(provider_id.to_owned()))?;
        let warming_restore = mode == ActivationMode::StartupRestore
            && slot.health.readiness == ProviderReadiness::Warming
            && slot.health.last_error.is_none();
        if !slot.health.is_ready() && !warming_restore {
            let reason = slot
                .health
                .last_error
                .clone()
                .unwrap_or_else(|| format!("readiness is {:?}", slot.health.readiness));
            return Err(MarketDataError::ProviderUnavailable {
                provider_id: provider_id.to_owned(),
                reason,
            });
        }
        if self.active != provider_id {
            self.cache.clear();
            self.generation = self.generation.saturating_add(1);
            self.active = provider_id.to_owned();
            let generation = self.runtime_recorder.reconfigure();
            let active_count = self.runtime_recorder.snapshot().active_count;
            let _ = self.runtime_recorder.set_stream_state(
                generation,
                slot.health.connected && active_count > 0,
                (active_count > 0)
                    .then(|| slot.health.last_error.clone())
                    .flatten(),
            );
        }
        Ok(self.runtime())
    }

    pub fn require_streaming(&self) -> Result<(), MarketDataError> {
        let slot = self
            .providers
            .get(&self.active)
            .ok_or_else(|| MarketDataError::ProviderNotFound(self.active.clone()))?;
        if slot.descriptor.capabilities.streaming_quotes {
            Ok(())
        } else {
            Err(MarketDataError::StreamingUnavailable(self.active.clone()))
        }
    }

    pub fn runtime(&self) -> ProviderRuntime {
        let health = self.providers.get(&self.active).map(|slot| &slot.health);
        ProviderRuntime {
            active_provider: self.active.clone(),
            generation: self.generation,
            readiness: health.map_or(ProviderReadiness::Unknown, |value| value.readiness),
            connected: health.is_some_and(|value| value.connected),
            active_demand: self.demand.snapshot().logical_count,
        }
    }

    pub fn generation(&self) -> u64 {
        self.generation
    }

    pub fn acquire_demand(
        &mut self,
        consumer_id: &str,
        refs: impl IntoIterator<Item = InstrumentRef>,
        managed: bool,
        now_ms: i64,
    ) -> Result<DemandSnapshot, MarketDataError> {
        if managed {
            self.require_streaming()?;
        }
        let snapshot = self.demand.acquire(consumer_id, refs, managed, now_ms)?;
        self.sync_runtime_demand(&snapshot);
        Ok(snapshot)
    }

    pub fn release_demand(&mut self, consumer_id: &str) -> bool {
        let released = self.demand.release(consumer_id);
        if released {
            let snapshot = self.demand.snapshot();
            self.sync_runtime_demand(&snapshot);
        }
        released
    }

    pub fn expire_demand(&mut self, now_ms: i64, ttl_ms: i64) -> Vec<String> {
        let expired = self.demand.expire(now_ms, ttl_ms);
        if !expired.is_empty() {
            let snapshot = self.demand.snapshot();
            self.sync_runtime_demand(&snapshot);
        }
        expired
    }

    pub fn demand(&self) -> DemandSnapshot {
        self.demand.snapshot()
    }

    pub fn runtime_recorder(&self) -> Arc<MarketDataRuntimeRecorder> {
        Arc::clone(&self.runtime_recorder)
    }

    fn sync_runtime_demand(&self, snapshot: &DemandSnapshot) {
        let _ = self
            .runtime_recorder
            .reconcile(snapshot.active.iter().map(InstrumentRef::instrument_id));
    }

    pub fn cache(&self) -> &TickCache {
        &self.cache
    }

    pub fn cache_mut(&mut self) -> &mut TickCache {
        &mut self.cache
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{InstrumentRef, ProviderCapabilities, ProviderConstraints};

    fn descriptor(id: &str, streaming: bool) -> ProviderDescriptor {
        ProviderDescriptor {
            selection_id: id.to_owned(),
            provider_id: id.to_owned(),
            display_name: id.to_owned(),
            broker_id: None,
            source: id.to_owned(),
            default_market: "US".to_owned(),
            supported_markets: vec!["US".to_owned()],
            transports: vec![if streaming { "stream" } else { "poll" }.to_owned()],
            capabilities: ProviderCapabilities {
                snapshots: true,
                streaming_quotes: streaming,
                ..ProviderCapabilities::default()
            },
            constraints: ProviderConstraints::default(),
            notes: Vec::new(),
        }
    }

    fn health(readiness: ProviderReadiness, connected: bool) -> HealthStatus {
        HealthStatus {
            connected,
            readiness,
            stream_mode: "snapshot-poll-delayed".to_owned(),
            ..HealthStatus::default()
        }
    }

    #[test]
    fn explicit_activation_fails_closed_and_switch_clears_cache() {
        let mut router = ProviderRouter::new(2);
        router
            .register(
                descriptor("futu", true),
                health(ProviderReadiness::Ready, true),
            )
            .expect("futu");
        router
            .register(
                descriptor("akshare", false),
                health(ProviderReadiness::Unknown, false),
            )
            .expect("akshare");
        assert_eq!(
            router
                .activate("futu", ActivationMode::Explicit)
                .expect("activate")
                .generation,
            1
        );
        assert!(matches!(
            router.activate("akshare", ActivationMode::Explicit),
            Err(MarketDataError::ProviderUnavailable { .. })
        ));
        router
            .update_health("akshare", health(ProviderReadiness::Ready, true))
            .expect("health");
        assert_eq!(
            router
                .activate("akshare", ActivationMode::Explicit)
                .expect("switch")
                .generation,
            2
        );
        assert!(matches!(
            router.require_streaming(),
            Err(MarketDataError::StreamingUnavailable(_))
        ));
    }

    #[test]
    fn managed_demand_blocks_provider_switch() {
        let mut router = ProviderRouter::new(2);
        for id in ["futu", "other"] {
            router
                .register(descriptor(id, true), health(ProviderReadiness::Ready, true))
                .expect("register");
        }
        router
            .activate("futu", ActivationMode::Explicit)
            .expect("activate");
        router
            .acquire_demand(
                "strategy",
                [InstrumentRef {
                    channel: "KLINE".to_owned(),
                    market: "US".to_owned(),
                    symbol: "AAPL".to_owned(),
                    interval: Some("1m".to_owned()),
                }],
                true,
                0,
            )
            .expect("lease");
        assert_eq!(
            router.activate("other", ActivationMode::Explicit),
            Err(MarketDataError::ManagedSubscriptionsActive)
        );
    }

    #[test]
    fn router_drives_runtime_recorder_from_provider_and_demand_state() {
        let mut router = ProviderRouter::new(2);
        router
            .register(
                descriptor("futu", true),
                health(ProviderReadiness::Ready, true),
            )
            .expect("register");
        router
            .activate("futu", ActivationMode::Explicit)
            .expect("activate");
        let snapshot = router
            .acquire_demand(
                "chart",
                [InstrumentRef {
                    channel: "SNAPSHOT".to_owned(),
                    market: "US".to_owned(),
                    symbol: "AAPL".to_owned(),
                    interval: None,
                }],
                false,
                0,
            )
            .expect("demand");
        let runtime = router.runtime_recorder().snapshot();
        assert_eq!(snapshot.logical_count, 1);
        assert_eq!(runtime.active_count, 1);
        assert!(runtime.generation > 0);
        assert!(!runtime.connected);

        router
            .update_health(
                "futu",
                HealthStatus {
                    connected: false,
                    last_error: Some("provider down".to_owned()),
                    ..health(ProviderReadiness::Failed, false)
                },
            )
            .expect("health");
        let runtime = router.runtime_recorder().snapshot();
        assert!(!runtime.connected);
        assert_eq!(runtime.stream_last_error.as_deref(), Some("provider down"));
        assert!(router.release_demand("chart"));
        assert_eq!(router.runtime_recorder().snapshot().active_count, 0);
    }
}
