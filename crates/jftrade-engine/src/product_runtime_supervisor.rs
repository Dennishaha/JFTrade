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
use jftrade_integration_pine::{PineProcess, PineReadinessMonitor};
use std::sync::{Arc, Mutex};

use super::ProductRuntimeError;
use crate::product::product_production_ports::{
    BacktestExecutionTaskRegistry, BacktestSyncWorkerRegistry, ExecutionReconciliationWorker,
    ProductionPortBundle,
};
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
    pub(crate) pine_health_monitors: Vec<Arc<PineReadinessMonitor>>,
    pub(crate) production_ports: Option<ProductionPortBundle>,
    pub(crate) backtest_sync_workers: Option<Arc<BacktestSyncWorkerRegistry>>,
    pub(crate) backtest_execution_workers: Option<Arc<BacktestExecutionTaskRegistry>>,
    pub(crate) execution_reconciliation_worker: Option<Arc<ExecutionReconciliationWorker>>,
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
            pine_health_monitors: Vec::new(),
            production_ports: None,
            backtest_sync_workers: None,
            backtest_execution_workers: None,
            execution_reconciliation_worker: None,
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
            || !self.pine_health_monitors.is_empty()
            || self.backtest_sync_workers.is_some()
            || self.backtest_execution_workers.is_some()
            || self.execution_reconciliation_worker.is_some()
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
        // Strategy runtime tasks own Pine calls and DemandBook consumers;
        // stop and join them before tearing down provider workers or stores.
        if let Some(ports) = self.production_ports.as_ref() {
            ports.shutdown_strategy_runtime();
            ports.shutdown_adk_runtime();
        }
        // 2. Stop reconciliation before provider/OpenD teardown.  It reads
        // the trade session at scan time, so leaving it alive while the
        // provider is closing would race a disappearing reader and could
        // persist a false UNKNOWN state.
        if let Some(worker) = self.execution_reconciliation_worker.take() {
            worker.shutdown().await;
            self.recorder.record("execution_reconciliation_worker");
        }
        // 3. Release provider demand & bridge
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
        // 4. Stop OpenD task runtime
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
        // 5. Close OpenD coordinator
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
        // 6. Stop market-data helper (health monitor first, then process)
        if let Some(workers) = self.backtest_sync_workers.take() {
            workers.shutdown().await;
        }
        if let Some(workers) = self.backtest_execution_workers.take() {
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
        // 7. Stop Pine health monitors before the process handles.  The
        // monitor owns a real join handle; awaiting it here prevents a late
        // probe from publishing readiness after the worker has been torn down.
        while let Some(monitor) = self.pine_health_monitors.pop() {
            monitor.shutdown().await;
        }
        // 8. Stop Pine workers
        let had_pine = !self.pine_workers.is_empty();
        while let Some(worker) = self.pine_workers.pop() {
            if let Err(error) = worker.stop().await {
                failures.push(error.to_string());
            }
        }
        if had_pine {
            self.recorder.record("pine_worker");
        }
        // 9. Release SQLite stores & 10 WriterLease locks last
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
        if let Some(ports) = self.production_ports.as_ref() {
            ports.shutdown_strategy_runtime();
            ports.shutdown_adk_runtime();
        }
        // 2. Stop reconciliation before provider/OpenD teardown.  Keep the
        // synchronous Drop path in the same lifecycle order as async
        // shutdown; the worker's terminate operation is non-blocking.
        if let Some(worker) = self.execution_reconciliation_worker.take() {
            worker.terminate();
            self.recorder.record("execution_reconciliation_worker");
        }
        // 3. Release provider demand & bridge
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
        // 4. Stop OpenD task runtime & coordinator
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
        // 5. Terminate helper (health monitor first, then process)
        if let Some(workers) = self.backtest_sync_workers.take() {
            workers.terminate();
        }
        if let Some(workers) = self.backtest_execution_workers.take() {
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
        // 6. Terminate Pine health monitors and workers
        while let Some(monitor) = self.pine_health_monitors.pop() {
            monitor.terminate();
        }
        let had_pine = !self.pine_workers.is_empty();
        while let Some(mut worker) = self.pine_workers.pop() {
            worker.terminate();
        }
        if had_pine {
            self.recorder.record("pine_worker");
        }
        // 7. Release SQLite stores & 9 WriterLease locks last
        if self.production_ports.is_some() {
            drop(self.production_ports.take());
            self.recorder.record("sqlite_lease");
        }
    }
}
