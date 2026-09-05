use std::path::Path;

use serde_json::Value;

use crate::strategy_runtime::{
    StoredRuntimeInstance, StoredRuntimeObservation, StoredStrategyAuditEvent,
    StoredStrategyLogEvent, StrategyRuntimeStore, StrategyRuntimeStoreError,
};

#[derive(Debug)]
pub struct StrategyRuntimeTestCutoverStore {
    inner: StrategyRuntimeStore,
}

impl StrategyRuntimeTestCutoverStore {
    pub fn open_existing(
        path: impl AsRef<Path>,
        profile: &str,
    ) -> Result<Self, StrategyRuntimeStoreError> {
        let inner = StrategyRuntimeStore::open_existing(path, profile)?;
        Ok(Self { inner })
    }

    pub fn path(&self) -> &Path {
        self.inner.path()
    }

    pub fn seed_instance(
        &self,
        instance_id: &str,
        status: &str,
        timestamp: &str,
    ) -> Result<(), StrategyRuntimeStoreError> {
        self.inner.seed_instance(instance_id, status, timestamp)
    }

    pub fn seed_instance_with_binding(
        &self,
        instance_id: &str,
        status: &str,
        binding: Value,
        timestamp: &str,
    ) -> Result<(), StrategyRuntimeStoreError> {
        self.inner
            .seed_instance_with_binding(instance_id, status, binding, timestamp)
    }

    #[allow(clippy::too_many_arguments)]
    pub fn seed_instance_with_definition(
        &self,
        instance_id: &str,
        status: &str,
        binding: Value,
        definition_id: &str,
        definition_name: &str,
        definition_version: &str,
        timestamp: &str,
    ) -> Result<(), StrategyRuntimeStoreError> {
        self.inner.seed_instance_with_definition(
            instance_id,
            status,
            binding,
            definition_id,
            definition_name,
            definition_version,
            timestamp,
        )
    }

    pub fn list_instances(&self) -> Result<Vec<StoredRuntimeInstance>, StrategyRuntimeStoreError> {
        self.inner.list_instances()
    }

    pub fn get_instance(
        &self,
        instance_id: &str,
    ) -> Result<Option<StoredRuntimeInstance>, StrategyRuntimeStoreError> {
        self.inner.get_instance(instance_id)
    }

    pub fn get_observation(
        &self,
        instance_id: &str,
    ) -> Result<Option<StoredRuntimeObservation>, StrategyRuntimeStoreError> {
        self.inner.get_observation(instance_id)
    }

    pub fn list_log_events(
        &self,
        instance_id: &str,
    ) -> Result<Vec<StoredStrategyLogEvent>, StrategyRuntimeStoreError> {
        self.inner.list_log_events(instance_id)
    }

    pub fn list_audit_events(
        &self,
        instance_id: &str,
    ) -> Result<Vec<StoredStrategyAuditEvent>, StrategyRuntimeStoreError> {
        self.inner.list_audit_events(instance_id)
    }

    pub fn count_daily_orders(
        &self,
        instance_id: &str,
        since_ms: i64,
    ) -> Result<i64, StrategyRuntimeStoreError> {
        self.inner.count_daily_orders(instance_id, since_ms)
    }

    pub fn update_status(
        &self,
        instance_id: &str,
        new_status: &str,
        timestamp: &str,
    ) -> Result<StoredRuntimeInstance, StrategyRuntimeStoreError> {
        self.inner.update_status(instance_id, new_status, timestamp)
    }

    pub fn update_binding(
        &self,
        instance_id: &str,
        binding: Value,
        timestamp: &str,
    ) -> Result<StoredRuntimeInstance, StrategyRuntimeStoreError> {
        self.inner.update_binding(instance_id, binding, timestamp)
    }

    pub fn update_risk(
        &self,
        instance_id: &str,
        risk: Value,
        timestamp: &str,
    ) -> Result<StoredRuntimeInstance, StrategyRuntimeStoreError> {
        self.inner.update_risk(instance_id, risk, timestamp)
    }

    pub fn delete_instance(
        &self,
        instance_id: &str,
        timestamp: &str,
    ) -> Result<StoredRuntimeInstance, StrategyRuntimeStoreError> {
        self.inner.delete_instance(instance_id, timestamp)
    }

    pub fn refresh_definition(
        &self,
        instance_id: &str,
        timestamp: &str,
    ) -> Result<StoredRuntimeInstance, StrategyRuntimeStoreError> {
        self.inner.refresh_definition(instance_id, timestamp)
    }
}
