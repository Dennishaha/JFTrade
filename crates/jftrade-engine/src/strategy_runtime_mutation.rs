use super::*;
use super::strategy_runtime_port::ProductionStrategyRuntimePort;
impl StrategyRuntimeWritePort for ProductionStrategyRuntimePort {
    fn mutate(
        &self,
        input: &StrategyRuntimeWriteInput,
    ) -> Result<Value, StrategyRuntimeWritePortError> {
        let timestamp = time::OffsetDateTime::now_utc()
            .format(&time::format_description::well_known::Rfc3339)
            .map_err(|error| StrategyRuntimeWritePortError::Failed {
                status: 500,
                code: "STRATEGY_RUNTIME_TIMESTAMP_FAILED".to_owned(),
                message: format!("format strategy runtime timestamp: {error}"),
            })?;

        let current = self
            .store
            .get_instance(&input.instance_id)
            .map_err(|e| StrategyRuntimeWritePortError::Failed {
                status: 500,
                code: "STRATEGY_RUNTIME_READ_FAILED".to_owned(),
                message: e.to_string(),
            })?
            .ok_or_else(|| StrategyRuntimeWritePortError::Failed {
                status: 404,
                code: "NOT_FOUND".to_owned(),
                message: "strategy instance not found".to_owned(),
            })?;
        let result = match input.operation {
            StrategyRuntimeWriteOperation::Start => {
                if current.runtime_active || current.status.eq_ignore_ascii_case("RUNNING") {
                    return Err(StrategyRuntimeWritePortError::Failed {
                        status: 409,
                        code: "CONFLICT".to_owned(),
                        message: "strategy instance is already running".to_owned(),
                    });
                }
                if let Some(error) = self.manager.dependency_error() {
                    return Err(error);
                }
                let runtime_binding = self.effective_binding(&current)?;
                self.manager
                    .acquire_demand(&input.instance_id, &runtime_binding)?;
                match self
                    .store
                    .update_status(&input.instance_id, "RUNNING", &timestamp)
                {
                    Ok(instance) => {
                        if let Err(error) = self.manager.spawn_task(
                            input.instance_id.clone(),
                            runtime_binding,
                            Arc::clone(&self.store),
                        ) {
                            let _ =
                                self.store
                                    .update_status(&input.instance_id, "STOPPED", &timestamp);
                            self.manager.release_demand(&input.instance_id);
                            Err(error)
                        } else {
                            Ok(instance)
                        }
                    }
                    Err(error) => {
                        self.manager.release_demand(&input.instance_id);
                        Err(error.into())
                    }
                }
            }
            StrategyRuntimeWriteOperation::Stop | StrategyRuntimeWriteOperation::Pause => {
                if input.operation == StrategyRuntimeWriteOperation::Pause
                    && !current.status.eq_ignore_ascii_case("RUNNING")
                {
                    return Err(StrategyRuntimeWritePortError::Failed {
                        status: 409,
                        code: "CONFLICT".to_owned(),
                        message: "strategy instance is not running".to_owned(),
                    });
                }
                if input.operation == StrategyRuntimeWriteOperation::Stop
                    && current.status.eq_ignore_ascii_case("STOPPED")
                {
                    return Err(StrategyRuntimeWritePortError::Failed {
                        status: 409,
                        code: "CONFLICT".to_owned(),
                        message: "strategy instance is already stopped".to_owned(),
                    });
                }
                self.manager.cancel(&input.instance_id);
                self.manager.release_demand(&input.instance_id);
                let status = if input.operation == StrategyRuntimeWriteOperation::Pause {
                    "PAUSED"
                } else {
                    "STOPPED"
                };
                self.store
                    .update_status(&input.instance_id, status, &timestamp)
                    .map_err(Into::into)
            }
            StrategyRuntimeWriteOperation::Delete => {
                if current.runtime_active || current.status.eq_ignore_ascii_case("RUNNING") {
                    return Err(StrategyRuntimeWritePortError::Failed {
                        status: 400,
                        code: "BAD_REQUEST".to_owned(),
                        message: "strategy instance is busy".to_owned(),
                    });
                }
                self.manager.cancel(&input.instance_id);
                self.manager.release_demand(&input.instance_id);
                self.store
                    .delete_instance(&input.instance_id, &timestamp)
                    .map_err(Into::into)
            }
            StrategyRuntimeWriteOperation::Update => {
                let binding =
                    input
                        .binding
                        .clone()
                        .ok_or_else(|| StrategyRuntimeWritePortError::Failed {
                            status: 400,
                            code: "BAD_REQUEST".to_owned(),
                            message: "strategy binding is required".to_owned(),
                        })?;
                let was_running =
                    current.runtime_active || current.status.eq_ignore_ascii_case("RUNNING");
                if was_running {
                    self.manager.cancel(&input.instance_id);
                    self.manager.release_demand(&input.instance_id);
                }
                let updated =
                    match self
                        .store
                        .update_binding(&input.instance_id, binding.clone(), &timestamp)
                    {
                        Ok(updated) => updated,
                        Err(error) => {
                            if was_running {
                                let _ = self.store.update_status(
                                    &input.instance_id,
                                    "STOPPED",
                                    &timestamp,
                                );
                            }
                            return Err(error.into());
                        }
                    };
                if was_running {
                    if let Some(error) = self.manager.dependency_error() {
                        let _ = self
                            .store
                            .update_status(&input.instance_id, "STOPPED", &timestamp);
                        return Err(error);
                    }
                    if let Err(error) = self.manager.acquire_demand(&input.instance_id, &binding) {
                        let _ = self
                            .store
                            .update_status(&input.instance_id, "STOPPED", &timestamp);
                        return Err(error);
                    }
                    let running =
                        match self
                            .store
                            .update_status(&input.instance_id, "RUNNING", &timestamp)
                        {
                            Ok(running) => running,
                            Err(error) => {
                                self.manager.release_demand(&input.instance_id);
                                return Err(error.into());
                            }
                        };
                    match self.manager.spawn_task(
                        input.instance_id.clone(),
                        binding,
                        Arc::clone(&self.store),
                    ) {
                        Ok(()) => Ok(running),
                        Err(error) => {
                            let _ =
                                self.store
                                    .update_status(&input.instance_id, "STOPPED", &timestamp);
                            self.manager.release_demand(&input.instance_id);
                            Err(error)
                        }
                    }
                } else {
                    Ok(updated)
                }
            }
            StrategyRuntimeWriteOperation::UpdateRuntimeRisk => {
                let risk = input.runtime_risk.clone().ok_or_else(|| {
                    StrategyRuntimeWritePortError::Failed {
                        status: 400,
                        code: "BAD_REQUEST".to_owned(),
                        message: "runtime risk is required".to_owned(),
                    }
                })?;
                self.store
                    .update_risk(&input.instance_id, risk, &timestamp)
                    .map_err(Into::into)
            }
            StrategyRuntimeWriteOperation::RefreshDefinition => {
                let was_running =
                    current.runtime_active || current.status.eq_ignore_ascii_case("RUNNING");
                if was_running {
                    self.manager.cancel(&input.instance_id);
                    self.manager.release_demand(&input.instance_id);
                }
                let refreshed = match self
                    .store
                    .refresh_definition(&input.instance_id, &timestamp)
                {
                    Ok(refreshed) => refreshed,
                    Err(error) => {
                        if was_running {
                            let _ =
                                self.store
                                    .update_status(&input.instance_id, "STOPPED", &timestamp);
                        }
                        return Err(error.into());
                    }
                };
                if was_running {
                    if let Some(error) = self.manager.dependency_error() {
                        let _ = self
                            .store
                            .update_status(&input.instance_id, "STOPPED", &timestamp);
                        return Err(error);
                    }
                    let runtime_binding = match self.effective_binding(&refreshed) {
                        Ok(binding) => binding,
                        Err(error) => {
                            let _ =
                                self.store
                                    .update_status(&input.instance_id, "STOPPED", &timestamp);
                            return Err(error);
                        }
                    };
                    if let Err(error) = self
                        .manager
                        .acquire_demand(&input.instance_id, &runtime_binding)
                    {
                        let _ = self
                            .store
                            .update_status(&input.instance_id, "STOPPED", &timestamp);
                        return Err(error);
                    }
                    let running =
                        match self
                            .store
                            .update_status(&input.instance_id, "RUNNING", &timestamp)
                        {
                            Ok(running) => running,
                            Err(error) => {
                                self.manager.release_demand(&input.instance_id);
                                return Err(error.into());
                            }
                        };
                    match self.manager.spawn_task(
                        input.instance_id.clone(),
                        runtime_binding,
                        Arc::clone(&self.store),
                    ) {
                        Ok(()) => Ok(running),
                        Err(error) => {
                            let _ =
                                self.store
                                    .update_status(&input.instance_id, "STOPPED", &timestamp);
                            self.manager.release_demand(&input.instance_id);
                            Err(error)
                        }
                    }
                } else {
                    Ok(refreshed)
                }
            }
        };

        match result {
            Ok(inst) => Ok(json!({
                "id": inst.id,
                "status": inst.status,
                "binding": inst.binding,
                "runtimeRisk": inst.runtime_risk,
                "definitionRevision": inst.definition_revision,
                "runtimeActive": inst.runtime_active,
                "deleted": inst.deleted,
            })),
            Err(error) => Err(error),
        }
    }
}
