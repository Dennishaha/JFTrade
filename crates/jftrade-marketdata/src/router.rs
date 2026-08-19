use std::collections::BTreeMap;

use serde::{Deserialize, Serialize};

use crate::{
    DemandBook, DemandSnapshot, HealthStatus, InstrumentRef, MarketDataError, ProviderDescriptor,
    ProviderReadiness, TickCache,
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
}

impl ProviderRouter {
    pub fn new(cache_capacity: usize) -> Self {
        Self {
            providers: BTreeMap::new(),
            active: String::new(),
            generation: 0,
            demand: DemandBook::default(),
            cache: TickCache::new(cache_capacity),
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
        self.demand.acquire(consumer_id, refs, managed, now_ms)
    }

    pub fn release_demand(&mut self, consumer_id: &str) -> bool {
        self.demand.release(consumer_id)
    }

    pub fn expire_demand(&mut self, now_ms: i64, ttl_ms: i64) -> Vec<String> {
        self.demand.expire(now_ms, ttl_ms)
    }

    pub fn demand(&self) -> DemandSnapshot {
        self.demand.snapshot()
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
}
