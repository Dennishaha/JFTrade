use std::path::Path;

use super::{
    ExecutionOrderStore, ExecutionOrderStoreError, StoredExecutionOrder, StoredExecutionOrderEvent,
    StoredExecutionOrderEventRecord,
};

#[derive(Debug)]
pub struct ExecutionOrderTestCutoverStore {
    inner: ExecutionOrderStore,
}

impl ExecutionOrderTestCutoverStore {
    pub fn open_existing(
        path: impl AsRef<Path>,
        profile: &str,
    ) -> Result<Self, ExecutionOrderStoreError> {
        let inner = ExecutionOrderStore::open_existing(path, profile)?;
        Ok(Self { inner })
    }

    pub fn path(&self) -> &Path {
        self.inner.path()
    }

    pub fn save_order(
        &self,
        order: StoredExecutionOrder,
        timestamp: &str,
    ) -> Result<StoredExecutionOrder, ExecutionOrderStoreError> {
        self.inner.save_order(order, timestamp)
    }

    pub fn reserve_order_with_preview(
        &self,
        order: StoredExecutionOrder,
        request_hash: &str,
        timestamp: &str,
    ) -> Result<
        crate::execution_order_reservation::ExecutionOrderReservation,
        ExecutionOrderStoreError,
    > {
        self.inner
            .reserve_order_with_preview(order, request_hash, timestamp)
    }

    pub fn reserve_order_with_preview_checked(
        &self,
        order: StoredExecutionOrder,
        request_hash: &str,
        timestamp: &str,
        expected_capability_version: Option<&str>,
    ) -> Result<
        crate::execution_order_reservation::ExecutionOrderReservation,
        ExecutionOrderStoreError,
    > {
        self.inner.reserve_order_with_preview_checked(
            order,
            request_hash,
            timestamp,
            expected_capability_version,
        )
    }

    pub fn find_order_by_client_identity(
        &self,
        broker_id: &str,
        trading_environment: &str,
        account_id: &str,
        client_order_id: &str,
    ) -> Result<Option<StoredExecutionOrder>, ExecutionOrderStoreError> {
        self.inner.find_order_by_client_identity(
            broker_id,
            trading_environment,
            account_id,
            client_order_id,
        )
    }

    pub fn save_order_and_event(
        &self,
        order: StoredExecutionOrder,
        timestamp: &str,
        event: &StoredExecutionOrderEvent<'_>,
    ) -> Result<StoredExecutionOrder, ExecutionOrderStoreError> {
        self.inner.save_order_and_event(order, timestamp, event)
    }

    pub fn transition_order_and_event(
        &self,
        order: StoredExecutionOrder,
        timestamp: &str,
        event: &StoredExecutionOrderEvent<'_>,
        expected_status: &str,
        expected_updated_at: &str,
    ) -> Result<StoredExecutionOrder, ExecutionOrderStoreError> {
        self.inner.transition_order_and_event(
            order,
            timestamp,
            event,
            expected_status,
            expected_updated_at,
        )
    }

    pub fn transition_order_and_event_fenced(
        &self,
        order: StoredExecutionOrder,
        timestamp: &str,
        event: &StoredExecutionOrderEvent<'_>,
        expected_status: &str,
        expected_updated_at: &str,
        expected_revision: Option<u64>,
    ) -> Result<StoredExecutionOrder, ExecutionOrderStoreError> {
        self.inner.transition_order_and_event_fenced(
            order,
            timestamp,
            event,
            expected_status,
            expected_updated_at,
            expected_revision,
        )
    }

    pub fn order_revision(&self, id: &str) -> Result<u64, ExecutionOrderStoreError> {
        self.inner.order_revision(id)
    }

    pub fn list_reconciliation_candidates(
        &self,
    ) -> Result<Vec<StoredExecutionOrder>, ExecutionOrderStoreError> {
        self.inner.list_reconciliation_candidates()
    }

    pub fn get_order(
        &self,
        id: &str,
    ) -> Result<Option<StoredExecutionOrder>, ExecutionOrderStoreError> {
        self.inner.get_order(id)
    }

    pub fn order_count(&self) -> Result<u64, ExecutionOrderStoreError> {
        self.inner.order_count()
    }

    pub fn list_orders(&self) -> Result<Vec<StoredExecutionOrder>, ExecutionOrderStoreError> {
        self.inner.list_orders()
    }

    pub fn cancel_order(
        &self,
        id: &str,
        timestamp: &str,
    ) -> Result<bool, ExecutionOrderStoreError> {
        self.inner.cancel_order(id, timestamp)
    }

    pub fn record_event(
        &self,
        event: &StoredExecutionOrderEvent<'_>,
    ) -> Result<(), ExecutionOrderStoreError> {
        self.inner.record_event(event)
    }

    pub fn event_count(&self, event_type: &str) -> Result<u64, ExecutionOrderStoreError> {
        self.inner.event_count(event_type)
    }

    pub fn list_order_events(
        &self,
        internal_order_id: &str,
    ) -> Result<Vec<StoredExecutionOrderEventRecord>, ExecutionOrderStoreError> {
        self.inner.list_order_events(internal_order_id)
    }

    pub fn next_sequence(&self, name: &str) -> Result<i64, ExecutionOrderStoreError> {
        self.inner.next_sequence(name)
    }
}
