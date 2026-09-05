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
                let status_upper = current.status.to_ascii_uppercase();
                if current.runtime_active || status_upper == "RUNNING" {
                    return Err(StrategyRuntimeWritePortError::Failed {
                        status: 409,
                        code: "CONFLICT".to_owned(),
                        message: "strategy instance is already running".to_owned(),
                    });
                }
                if let Some(error) = self.manager.dependency_error() {
                    return Err(error);
                }

                if status_upper == "PAUSED" && self.manager.is_task_alive(&input.instance_id) {
                    let instance = self
                        .store
                        .update_status_cas(&input.instance_id, &["PAUSED"], "RUNNING", &timestamp)
                        .map_err(|e| StrategyRuntimeWritePortError::Failed {
                            status: 409,
                            code: "CONFLICT".to_owned(),
                            message: format!("resume paused strategy failed: {e}"),
                        })?;
                    self.manager.wake(&input.instance_id);
                    return Ok(json!({
                        "id": instance.id,
                        "status": instance.status,
                        "binding": instance.binding,
                        "runtimeRisk": instance.runtime_risk,
                        "runtimeRiskRevision": instance.runtime_risk_revision,
                        "definitionRevision": instance.definition_revision,
                        "runtimeActive": instance.runtime_active,
                        "deleted": instance.deleted,
                        "updatedAt": instance.updated_at,
                        "createdAt": instance.created_at,
                    }));
                }

                let runtime_binding = self.effective_binding(&current)?;
                self.store
                    .update_status_cas(
                        &input.instance_id,
                        &["STOPPED", "FAILED", "PAUSED"],
                        "STARTING",
                        &timestamp,
                    )
                    .map_err(|e| {
                        StrategyRuntimeWritePortError::Failed {
                            status: 409,
                            code: "CONFLICT".to_owned(),
                            message: format!("transition to STARTING failed: {e}"),
                        }
                    })?;

                if let Err(error) = self
                    .manager
                    .acquire_demand(&input.instance_id, &runtime_binding)
                {
                    let _ = self.store.update_status_cas(
                        &input.instance_id,
                        &["STARTING"],
                        "FAILED",
                        &timestamp,
                    );
                    return Err(error);
                }

                match self.manager.spawn_task(
                    input.instance_id.clone(),
                    runtime_binding,
                    Arc::clone(&self.store),
                ) {
                    Ok(()) => {
                        match self.store.update_status_cas(
                            &input.instance_id,
                            &["STARTING"],
                            "RUNNING",
                            &timestamp,
                        ) {
                            Ok(running_instance) => Ok(running_instance),
                            Err(error) => {
                                self.manager.cancel(&input.instance_id);
                                self.manager.release_demand(&input.instance_id);
                                Err(StrategyRuntimeWritePortError::Failed {
                                    status: 409,
                                    code: "CONFLICT".to_owned(),
                                    message: format!("transition to RUNNING failed: {error}"),
                                })
                            }
                        }
                    }
                    Err(error) => {
                        let _ = self.store.update_status_cas(
                            &input.instance_id,
                            &["STARTING"],
                            "FAILED",
                            &timestamp,
                        );
                        self.manager.release_demand(&input.instance_id);
                        Err(error)
                    }
                }
            }
            StrategyRuntimeWriteOperation::Stop => {
                if current.status.eq_ignore_ascii_case("STOPPED") {
                    return Err(StrategyRuntimeWritePortError::Failed {
                        status: 409,
                        code: "CONFLICT".to_owned(),
                        message: "strategy instance is already stopped".to_owned(),
                    });
                }
                let stopped = self.store
                    .update_status_cas(
                        &input.instance_id,
                        &["RUNNING", "PAUSED", "STARTING", "FAILED"],
                        "STOPPED",
                        &timestamp,
                    )
                    .map_err(StrategyRuntimeWritePortError::from)?;
                self.manager.cancel(&input.instance_id);
                self.manager.release_demand(&input.instance_id);
                Ok(stopped)
            }
            StrategyRuntimeWriteOperation::Pause => {
                if !current.status.eq_ignore_ascii_case("RUNNING") {
                    return Err(StrategyRuntimeWritePortError::Failed {
                        status: 409,
                        code: "CONFLICT".to_owned(),
                        message: "strategy instance is not running".to_owned(),
                    });
                }
                self.store
                    .update_status_cas(&input.instance_id, &["RUNNING"], "PAUSED", &timestamp)
                    .map_err(Into::into)
            }
            StrategyRuntimeWriteOperation::Delete => {
                let status_upper = current.status.to_ascii_uppercase();
                if status_upper != "STOPPED" && status_upper != "FAILED" {
                    return Err(StrategyRuntimeWritePortError::Failed {
                        status: 400,
                        code: "BAD_REQUEST".to_owned(),
                        message: "only STOPPED or FAILED strategy instances can be deleted".to_owned(),
                    });
                }
                self.manager.cancel(&input.instance_id);
                self.manager.release_demand(&input.instance_id);
                self.store
                    .delete_instance(&input.instance_id, &timestamp)
                    .map_err(Into::into)
            }
            StrategyRuntimeWriteOperation::Update => {
                let mut binding = input
                    .binding
                    .clone()
                    .ok_or_else(|| StrategyRuntimeWritePortError::Failed {
                        status: 400,
                        code: "BAD_REQUEST".to_owned(),
                        message: "strategy binding is required".to_owned(),
                    })?;
                if current.runtime_active || !current.status.eq_ignore_ascii_case("STOPPED") {
                    return Err(StrategyRuntimeWritePortError::Failed {
                        status: 400,
                        code: "BAD_REQUEST".to_owned(),
                        message: "strategy instance must be stopped before modification".to_owned(),
                    });
                }
                normalize_strategy_binding(&mut binding)?;
                self.store
                    .update_binding(&input.instance_id, binding, &timestamp)
                    .map_err(Into::into)
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
                    return Err(StrategyRuntimeWritePortError::Failed {
                        status: 409,
                        code: "STRATEGY_REFRESH_REQUIRES_STOP".to_owned(),
                        message: "refresh definition requires an explicitly stopped strategy instance".to_owned(),
                    });
                }
                let refreshed = match self
                    .store
                    .refresh_definition(&input.instance_id, &timestamp)
                {
                    Ok(refreshed) => refreshed,
                    Err(error) => return Err(error.into()),
                };
                Ok(refreshed)
            }
        };

        match result {
            Ok(inst) => Ok(json!({
                "id": inst.id,
                "status": inst.status,
                "binding": inst.binding,
                "runtimeRisk": inst.runtime_risk,
                "runtimeRiskRevision": inst.runtime_risk_revision,
                "definitionRevision": inst.definition_revision,
                "runtimeActive": inst.runtime_active,
                "deleted": inst.deleted,
            })),
            Err(error) => Err(error),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use rusqlite::Connection;
    use std::sync::Arc;
    use jftrade_store_sqlite::{
        STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE, StrategyDefinitionStore, StrategyRuntimeStore,
    };

    fn seed_strategy_test_db(path: &std::path::Path) {
        let conn = Connection::open(path).expect("open test db");
        jftrade_store_sqlite::initialize_current(&conn, "strategy")
            .expect("initialize strategy schema");
    }

    #[test]
    fn test_running_strategy_update_rejected_with_bad_request() {
        let dir = tempfile::tempdir().expect("tempdir");
        let path = dir.path().join("strategy.db");
        seed_strategy_test_db(&path);

        let def_store = Arc::new(
            StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
                .expect("open definition store"),
        );
        let store = Arc::new(StrategyRuntimeStore::from_definition_store(&def_store));
        store
            .seed_instance("running-inst", "RUNNING", "2026-08-30T00:00:00Z")
            .expect("seed running instance");

        let active_provider = Arc::new(
            crate::product::product_active_provider_state::ActiveProviderState::default(),
        );
        let manager = Arc::new(StrategyRuntimeManager::new(
            None,
            None,
            None,
            None,
            active_provider,
        ));
        let port = ProductionStrategyRuntimePort {
            store,
            definitions: def_store,
            manager,
        };

        let update_input = StrategyRuntimeWriteInput {
            operation: StrategyRuntimeWriteOperation::Update,
            instance_id: "running-inst".to_owned(),
            binding: Some(json!({"symbols": ["US.AAPL"], "interval": "5m"})),
            runtime_risk: None,
        };

        let err = port
            .mutate(&update_input)
            .expect_err("must reject update when running");
        match err {
            StrategyRuntimeWritePortError::Failed {
                status,
                code,
                message,
            } => {
                assert_eq!(status, 400);
                assert_eq!(code, "BAD_REQUEST");
                assert_eq!(
                    message,
                    "strategy instance must be stopped before modification"
                );
            }
            other => panic!("expected 400 BAD_REQUEST, got: {:?}", other),
        }
    }

    #[test]
    fn test_stopped_strategy_update_succeeds_with_normalization() {
        let dir = tempfile::tempdir().expect("tempdir");
        let path = dir.path().join("strategy.db");
        seed_strategy_test_db(&path);

        let def_store = Arc::new(
            StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
                .expect("open definition store"),
        );
        let store = Arc::new(StrategyRuntimeStore::from_definition_store(&def_store));
        store
            .seed_instance("stopped-inst", "STOPPED", "2026-08-30T00:00:00Z")
            .expect("seed stopped instance");

        let active_provider = Arc::new(
            crate::product::product_active_provider_state::ActiveProviderState::default(),
        );
        let manager = Arc::new(StrategyRuntimeManager::new(
            None,
            None,
            None,
            None,
            active_provider,
        ));
        let port = ProductionStrategyRuntimePort {
            store,
            definitions: def_store,
            manager,
        };

        let update_input = StrategyRuntimeWriteInput {
            operation: StrategyRuntimeWriteOperation::Update,
            instance_id: "stopped-inst".to_owned(),
            binding: Some(json!({"symbols": ["US:AAPL"]})), // Colon delimiter, missing interval
            runtime_risk: None,
        };

        let result = port.mutate(&update_input).expect("update stopped instance");
        assert_eq!(result["status"], "STOPPED");
        assert_eq!(result["binding"]["symbols"], json!(["US.AAPL"])); // Normalized to dot
        assert_eq!(result["binding"]["interval"], "5m"); // Defaulted interval
    }
}
