//! Ordered shutdown supervisor for JFTrade product runtime.
//!
//! Enforces single-direction, reverse-of-construction teardown order:
//! 1. HTTP server shutdown & JoinHandle await (HTTP requests drain)
//! 2. OpenD provider runtime & demand bridge
//! 3. OpenD session task runtime
//! 4. OpenD session coordinator close
//! 5. Market-data helper & PineTS workers stop/terminate
//! 6. Production ports & 9 SQLite WriterLease locks release

use jftrade_integration_futu::{
    OpenDProviderRuntime, OpenDSessionCoordinator, OpenDSessionRuntime,
};
use jftrade_integration_marketdata_helper::HelperProcess;
use jftrade_integration_pine::PineProcess;
use std::sync::{Arc, Mutex};

use super::ProductRuntimeError;
use crate::product::product_production_ports::{BacktestSyncWorkerRegistry, ProductionPortBundle};
use crate::product::{ActiveProviderState, ProductHandle};
use crate::product_runtime::product_runtime_composition::SharedOpenDProviderRuntime;

#[derive(Clone, Debug, Default)]
pub(crate) struct ShutdownEventRecorder {
    events: Arc<Mutex<Vec<&'static str>>>,
}

impl ShutdownEventRecorder {
    #[cfg(test)]
    pub(crate) fn new() -> Self {
        Self::default()
    }

    pub(crate) fn record(&self, event: &'static str) {
        let mut list = self.events.lock().unwrap_or_else(|e| e.into_inner());
        list.push(event);
    }

    #[cfg(test)]
    pub(crate) fn events(&self) -> Vec<&'static str> {
        self.events
            .lock()
            .unwrap_or_else(|e| e.into_inner())
            .clone()
    }
}

pub(crate) struct ProductShutdownSupervisor {
    pub(crate) product: Option<ProductHandle>,
    pub(crate) active_provider_state: Option<Arc<ActiveProviderState>>,
    pub(crate) market_data_opend_provider: Option<OpenDProviderRuntime>,
    pub(crate) market_data_dynamic_opend: Option<SharedOpenDProviderRuntime>,
    pub(crate) market_data_opend_runtime: Option<OpenDSessionRuntime>,
    pub(crate) market_data_opend: Option<Arc<Mutex<OpenDSessionCoordinator>>>,
    pub(crate) marketdata_helper: Option<Arc<Mutex<Option<HelperProcess>>>>,
    pub(crate) helper_health: Option<Arc<super::HelperHealthMonitor>>,
    pub(crate) pine_workers: Vec<PineProcess>,
    pub(crate) production_ports: Option<ProductionPortBundle>,
    pub(crate) backtest_sync_workers: Option<Arc<BacktestSyncWorkerRegistry>>,
    pub(crate) recorder: ShutdownEventRecorder,
}

impl Drop for ProductShutdownSupervisor {
    fn drop(&mut self) {
        self.execute_sync_drop();
    }
}

impl Default for ProductShutdownSupervisor {
    fn default() -> Self {
        Self::new()
    }
}

impl ProductShutdownSupervisor {
    pub(crate) fn new() -> Self {
        Self::with_recorder(ShutdownEventRecorder::default())
    }

    pub(crate) fn with_recorder(recorder: ShutdownEventRecorder) -> Self {
        Self {
            product: None,
            active_provider_state: None,
            market_data_opend_provider: None,
            market_data_dynamic_opend: None,
            market_data_opend_runtime: None,
            market_data_opend: None,
            marketdata_helper: None,
            helper_health: None,
            pine_workers: Vec::new(),
            production_ports: None,
            backtest_sync_workers: None,
            recorder,
        }
    }

    pub fn has_active_resources(&self) -> bool {
        self.product.is_some()
            || self.market_data_opend_provider.is_some()
            || self.market_data_dynamic_opend.is_some()
            || self.market_data_opend_runtime.is_some()
            || self.market_data_opend.is_some()
            || self
                .marketdata_helper
                .as_ref()
                .is_some_and(|h| h.lock().is_ok_and(|g| g.is_some()))
            || !self.pine_workers.is_empty()
            || self.backtest_sync_workers.is_some()
            || self.production_ports.is_some()
    }

    pub async fn execute_shutdown(&mut self) -> Result<(), ProductRuntimeError> {
        if let Some(state) = self.active_provider_state.as_ref() {
            state.begin_shutdown();
        }
        let mut failures = Vec::new();
        // 1. Stop HTTP server and live hub first (reverse of construction)
        if let Some(product) = self.product.take() {
            if let Err(error) = product.shutdown().await {
                failures.push(error.to_string());
            }
            self.recorder.record("http_join");
        }
        // 2. Release provider demand & bridge
        let mut had_provider = false;
        if let Some(provider) = self.market_data_opend_provider.take() {
            had_provider = true;
            if let Err(error) = provider.shutdown() {
                failures.push(error.to_string());
            }
        }
        if let Some(runtime) = self.market_data_dynamic_opend.take()
            && let Some(provider) = runtime.lock().unwrap_or_else(|e| e.into_inner()).take()
        {
            had_provider = true;
            if let Err(error) = provider.shutdown() {
                failures.push(error.to_string());
            }
        }
        if had_provider {
            self.recorder.record("provider");
            self.recorder.record("opend");
        }
        // 3. Stop OpenD task runtime
        let mut opend_closed = false;
        if let Some(mut task) = self.market_data_opend_runtime.take() {
            opend_closed = true;
            if let Err(error) = task.shutdown() {
                failures.push(error.to_string());
            }
            if !had_provider {
                self.recorder.record("opend");
            }
        }
        // 4. Close OpenD coordinator
        if let Some(coordinator) = self.market_data_opend.take() {
            if let Err(error) = coordinator
                .lock()
                .unwrap_or_else(|poisoned| poisoned.into_inner())
                .close()
            {
                failures.push(error.to_string());
            }
            if !opend_closed && !had_provider {
                self.recorder.record("opend");
            }
        }
        // 5. Stop market-data helper (health monitor first, then process)
        if let Some(workers) = self.backtest_sync_workers.take() {
            workers.shutdown().await;
        }
        if let Some(monitor) = self.helper_health.take() {
            monitor.stop();
        }
        let helper_opt = self
            .marketdata_helper
            .take()
            .and_then(|arc| arc.lock().ok()?.take());
        if let Some(mut helper) = helper_opt {
            if let Err(error) = helper.stop().await {
                failures.push(error.to_string());
            }
            self.recorder.record("marketdata_helper");
        }
        // 6. Stop Pine workers
        let had_pine = !self.pine_workers.is_empty();
        while let Some(worker) = self.pine_workers.pop() {
            if let Err(error) = worker.stop().await {
                failures.push(error.to_string());
            }
        }
        if had_pine {
            self.recorder.record("pine_worker");
        }
        // 7. Release SQLite stores & 9 WriterLease locks last
        if self.production_ports.is_some() {
            drop(self.production_ports.take());
            self.recorder.record("sqlite_lease");
        }

        if failures.is_empty() {
            Ok(())
        } else {
            Err(ProductRuntimeError::Shutdown(failures.join("; ")))
        }
    }

    pub(crate) fn execute_sync_drop(&mut self) {
        if let Some(state) = self.active_provider_state.as_ref() {
            state.begin_shutdown();
        }
        if !self.has_active_resources() {
            return;
        }
        // 1. Stop HTTP server
        if let Some(mut product) = self.product.take() {
            product.sync_terminate();
            self.recorder.record("http_join");
        }
        // 2. Release provider demand & bridge
        let mut had_provider = false;
        if let Some(provider) = self.market_data_opend_provider.take() {
            had_provider = true;
            let _ = provider.shutdown();
        }
        if let Some(runtime) = self.market_data_dynamic_opend.take()
            && let Some(provider) = runtime.lock().unwrap_or_else(|e| e.into_inner()).take()
        {
            had_provider = true;
            let _ = provider.shutdown();
        }
        if had_provider {
            self.recorder.record("provider");
            self.recorder.record("opend");
        }
        // 3. Stop OpenD task runtime & coordinator
        let mut opend_recorded = false;
        if let Some(mut task) = self.market_data_opend_runtime.take() {
            opend_recorded = true;
            let _ = task.shutdown();
            if !had_provider {
                self.recorder.record("opend");
            }
        }
        if let Some(coordinator) = self.market_data_opend.take() {
            let _ = coordinator
                .lock()
                .unwrap_or_else(|poisoned| poisoned.into_inner())
                .close();
            if !opend_recorded && !had_provider {
                self.recorder.record("opend");
            }
        }
        // 4. Terminate helper (health monitor first, then process)
        if let Some(workers) = self.backtest_sync_workers.take() {
            workers.terminate();
        }
        if let Some(monitor) = self.helper_health.take() {
            monitor.stop();
        }
        let helper_opt = self
            .marketdata_helper
            .take()
            .and_then(|arc| arc.lock().ok()?.take());
        if let Some(mut helper) = helper_opt {
            helper.terminate();
            self.recorder.record("marketdata_helper");
        }
        // 5. Terminate Pine workers
        let had_pine = !self.pine_workers.is_empty();
        while let Some(mut worker) = self.pine_workers.pop() {
            worker.terminate();
        }
        if had_pine {
            self.recorder.record("pine_worker");
        }
        // 6. Release SQLite stores & 9 WriterLease locks last
        if self.production_ports.is_some() {
            drop(self.production_ports.take());
            self.recorder.record("sqlite_lease");
        }
    }
}
