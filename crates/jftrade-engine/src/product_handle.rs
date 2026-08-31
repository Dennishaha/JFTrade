impl ProductHandle {
    pub const fn startup_record(&self) -> &ProductStartupRecord {
        &self.startup_record
    }

    pub fn live_hub(&self) -> Arc<LiveHub> {
        Arc::clone(&self.live_hub)
    }

    pub(crate) fn take_production_ports(
        &mut self,
    ) -> Option<crate::product::product_production_ports::ProductionPortBundle> {
        self.production_ports.take()
    }

    /// Stop runtime tasks that are owned by the production port bundle before
    /// its stores (and their WriterLeases) are released.  This mirrors the
    /// shutdown supervisor's reverse-of-construction order for the direct
    /// `start_product` entrypoint, where the bundle remains on this handle.
    async fn shutdown_production_runtime(&self) {
        let Some(ports) = self.production_ports.as_ref() else {
            return;
        };
        ports.shutdown_strategy_runtime();
        ports.shutdown_adk_runtime();
        if let Some(worker) = ports.execution_reconciliation_worker() {
            worker.shutdown().await;
        }
    }

    fn terminate_production_runtime(&self) {
        let Some(ports) = self.production_ports.as_ref() else {
            return;
        };
        ports.shutdown_strategy_runtime();
        ports.shutdown_adk_runtime();
        if let Some(worker) = ports.execution_reconciliation_worker() {
            worker.terminate();
        }
    }

    pub(crate) fn sync_terminate(&mut self) {
        // Shutdown must first stop accepting new websocket connections; the
        // HTTP drain below then completes without serving fresh upgrades.
        self.live_hub.begin_shutdown();
        if let Some(state) = &self.active_provider_state {
            state.begin_shutdown();
        }
        if let Some(mut server) = self.server.take() {
            let _ = server.shutdown_blocking();
        }
        if let Some(runtime) = self.web_runtime.take() {
            let _ = runtime.shutdown_blocking();
        }
        if let Some(runtime) = self.mcp_server_runtime.take() {
            let _ = runtime.shutdown_blocking();
        }
        self.live_hub.mark_stopped();
        if let Some(manager) = self.calendar_manager.take() {
            let _ = manager.close();
        }
        self.terminate_production_runtime();
        drop(self.production_ports.take());
    }

    pub async fn shutdown(mut self) -> Result<(), ProductError> {
        self.live_hub.begin_shutdown();
        if let Some(state) = &self.active_provider_state {
            state.begin_shutdown();
        }
        if let Some(server) = self.server.take() {
            server.shutdown().await?;
        }
        if let Some(runtime) = self.web_runtime.take() {
            runtime
                .shutdown_blocking()
                .map_err(|message| ProductError::SecurityRuntime { message })?;
        }
        if let Some(runtime) = self.mcp_server_runtime.take() {
            runtime
                .shutdown_blocking()
                .map_err(|message| ProductError::SecurityRuntime {
                    message: format!("MCP server: {message}"),
                })?;
        }
        self.live_hub.mark_stopped();
        if let Some(manager) = self.calendar_manager.take() {
            manager.close().map_err(ProductError::Calendar)?;
        }
        self.shutdown_production_runtime().await;
        drop(self.production_ports.take());
        Ok(())
    }
}

impl Drop for ProductHandle {
    fn drop(&mut self) {
        self.sync_terminate();
    }
}
