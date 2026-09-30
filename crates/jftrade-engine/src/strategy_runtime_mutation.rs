use super::strategy_runtime_port::ProductionStrategyRuntimePort;
use super::*;
impl StrategyRuntimeWritePort for ProductionStrategyRuntimePort {
    fn mutate(
        &self,
        input: &StrategyRuntimeWriteInput,
    ) -> Result<Value, StrategyRuntimeWritePortError> {
        let _mutation = self
            .manager
            .mutation_lock
            .lock()
            .unwrap_or_else(|e| e.into_inner());
        if self.manager.stopping.load(Ordering::Acquire) {
            return Err(StrategyRuntimeWritePortError::Unavailable(
                "strategy runtime is stopping".to_owned(),
            ));
        }
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
                if self.manager.is_task_alive(&input.instance_id)
                    && !self.manager.cancel(&input.instance_id)
                {
                    return Err(StrategyRuntimeWritePortError::Unavailable(
                        "strategy is still stopping".to_owned(),
                    ));
                }
                self.store
                    .update_status_cas(
                        &input.instance_id,
                        &["STOPPED", "FAILED", "PAUSED"],
                        "STARTING",
                        &timestamp,
                    )
                    .map_err(|e| StrategyRuntimeWritePortError::Failed {
                        status: 409,
                        code: "CONFLICT".to_owned(),
                        message: format!("transition to STARTING failed: {e}"),
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
                let stopped = self
                    .store
                    .update_status_cas(
                        &input.instance_id,
                        &["RUNNING", "PAUSED", "STARTING", "FAILED"],
                        "STOPPED",
                        &timestamp,
                    )
                    .map_err(StrategyRuntimeWritePortError::from)?;
                if !self.manager.cancel(&input.instance_id) {
                    return Err(StrategyRuntimeWritePortError::Unavailable(
                        "strategy stop timed out; task owner retained".to_owned(),
                    ));
                }
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
                let paused = self
                    .store
                    .update_status_cas(&input.instance_id, &["RUNNING"], "PAUSED", &timestamp)
                    .map_err(StrategyRuntimeWritePortError::from)?;
                if !self.manager.cancel(&input.instance_id) {
                    return Err(StrategyRuntimeWritePortError::Unavailable(
                        "strategy pause timed out; task owner retained".to_owned(),
                    ));
                }
                self.manager.release_demand(&input.instance_id);
                Ok(paused)
            }
            StrategyRuntimeWriteOperation::Delete => {
                let status_upper = current.status.to_ascii_uppercase();
                if status_upper != "STOPPED" && status_upper != "FAILED" {
                    return Err(StrategyRuntimeWritePortError::Failed {
                        status: 400,
                        code: "BAD_REQUEST".to_owned(),
                        message: "only STOPPED or FAILED strategy instances can be deleted"
                            .to_owned(),
                    });
                }
                if !self.manager.cancel(&input.instance_id) {
                    return Err(StrategyRuntimeWritePortError::Unavailable(
                        "strategy is still stopping".to_owned(),
                    ));
                }
                self.manager.release_demand(&input.instance_id);
                self.store
                    .delete_instance(&input.instance_id, &timestamp)
                    .map_err(Into::into)
            }
            StrategyRuntimeWriteOperation::Update => {
                let mut binding =
                    input
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
                        message:
                            "refresh definition requires an explicitly stopped strategy instance"
                                .to_owned(),
                    });
                }
                let refreshed = self
                    .store
                    .refresh_definition(&input.instance_id, &timestamp)
                    .map_err(StrategyRuntimeWritePortError::from)?;
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
    use jftrade_store_sqlite::{
        STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE, StrategyDefinitionStore, StrategyRuntimeStore,
    };
    use rusqlite::Connection;
    use std::sync::atomic::AtomicBool;
    use std::sync::{Arc, mpsc};
    use std::thread;
    use std::time::Duration;

    fn seed_strategy_test_db(path: &std::path::Path) {
        let conn = Connection::open(path).expect("open test db");
        jftrade_store_sqlite::initialize_current(&conn, "strategy")
            .expect("initialize strategy schema");
    }

    fn insert_test_runtime_task(manager: &Arc<StrategyRuntimeManager>, instance_id: &str) {
        let cancel = Arc::new(AtomicBool::new(false));
        let (done_tx, done_rx) = mpsc::channel();
        let cancel_for_thread = Arc::clone(&cancel);
        let thread_handle = thread::spawn(move || {
            while !cancel_for_thread.load(Ordering::Acquire) {
                thread::sleep(Duration::from_millis(1));
            }
            let _ = done_tx.send(());
        });
        manager.tasks.lock().expect("runtime task map").insert(
            instance_id.to_owned(),
            RuntimeTask {
                cancel,
                wake: Arc::new(tokio::sync::Notify::new()),
                done_rx,
                thread_handle: Some(thread_handle),
                close_errors: Arc::new(Mutex::new(Vec::new())),
            },
        );
    }

    fn runtime_port_fixture(
        path: &std::path::Path,
        manager: Arc<StrategyRuntimeManager>,
    ) -> ProductionStrategyRuntimePort {
        let definitions = Arc::new(
            StrategyDefinitionStore::open_existing(path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
                .expect("open definition store"),
        );
        let store = Arc::new(StrategyRuntimeStore::from_definition_store(&definitions));
        ProductionStrategyRuntimePort {
            store,
            definitions,
            manager,
        }
    }

    fn reject_status_transition(path: &std::path::Path) {
        let connection = Connection::open(path).expect("open rejection connection");
        connection
            .execute_batch(
                "CREATE TRIGGER strategy_runtime_test_reject_lifecycle_status
                 BEFORE UPDATE OF status ON strategy_catalog_operations
                 WHEN NEW.status IN ('PAUSED', 'STOPPED') BEGIN
                     SELECT RAISE(ABORT, 'test lifecycle status rejection');
                 END;",
            )
            .expect("install status rejection trigger");
    }

    // Parity: go:452dea11:internal/strategy/service_test.go:278 TestServicePauseAndStopInstancesStopRuntimeAfterStateTransition
    #[test]
    fn lifecycle_transition_failure_does_not_stop_runtime_before_state_write() {
        let dir = tempfile::tempdir().expect("tempdir");
        let path = dir.path().join("strategy.db");
        seed_strategy_test_db(&path);
        let active_provider =
            Arc::new(crate::product::product_active_provider_state::ActiveProviderState::default());
        let manager = Arc::new(StrategyRuntimeManager::new(
            None,
            None,
            None,
            None,
            active_provider,
        ));
        let port = runtime_port_fixture(&path, Arc::clone(&manager));
        for instance_id in ["stop-transition", "pause-transition"] {
            port.store
                .seed_instance(instance_id, "RUNNING", "2026-08-30T00:00:00Z")
                .expect("seed running instance");
            insert_test_runtime_task(&manager, instance_id);
        }
        reject_status_transition(&path);

        for (instance_id, operation) in [
            ("stop-transition", StrategyRuntimeWriteOperation::Stop),
            ("pause-transition", StrategyRuntimeWriteOperation::Pause),
        ] {
            let input = StrategyRuntimeWriteInput {
                operation,
                instance_id: instance_id.to_owned(),
                binding: None,
                runtime_risk: None,
            };
            assert!(port.mutate(&input).is_err(), "transition must fail");
            assert!(
                manager.is_task_alive(instance_id),
                "runtime must remain owned when the state transition fails"
            );
        }

        manager.cancel("stop-transition");
        manager.cancel("pause-transition");
    }

    // Parity: go:452dea11:internal/strategy/service_test.go:278 TestServicePauseAndStopInstancesStopRuntimeAfterStateTransition
    #[test]
    fn pause_and_stop_transition_state_before_stopping_runtime() {
        let dir = tempfile::tempdir().expect("tempdir");
        let path = dir.path().join("strategy.db");
        seed_strategy_test_db(&path);
        let active_provider =
            Arc::new(crate::product::product_active_provider_state::ActiveProviderState::default());
        let manager = Arc::new(StrategyRuntimeManager::new(
            None,
            None,
            None,
            None,
            active_provider,
        ));
        let port = runtime_port_fixture(&path, Arc::clone(&manager));
        for instance_id in ["stop-success", "pause-success"] {
            port.store
                .seed_instance(instance_id, "RUNNING", "2026-08-30T00:00:00Z")
                .expect("seed running instance");
            insert_test_runtime_task(&manager, instance_id);
        }

        for (instance_id, operation, expected_status) in [
            (
                "stop-success",
                StrategyRuntimeWriteOperation::Stop,
                "STOPPED",
            ),
            (
                "pause-success",
                StrategyRuntimeWriteOperation::Pause,
                "PAUSED",
            ),
        ] {
            let input = StrategyRuntimeWriteInput {
                operation,
                instance_id: instance_id.to_owned(),
                binding: None,
                runtime_risk: None,
            };
            let result = port.mutate(&input).expect("lifecycle transition");
            assert_eq!(result["status"], expected_status);
            assert_eq!(
                port.store
                    .get_instance(instance_id)
                    .expect("read transitioned instance")
                    .expect("transitioned instance")
                    .status,
                expected_status
            );
            assert!(
                !manager.is_task_alive(instance_id),
                "{operation:?} must stop the runtime after the persisted state transition"
            );
        }
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

        let active_provider =
            Arc::new(crate::product::product_active_provider_state::ActiveProviderState::default());
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

        let active_provider =
            Arc::new(crate::product::product_active_provider_state::ActiveProviderState::default());
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

    #[test]
    fn binding_normalization_validates_execution_enums_and_nested_account() {
        let mut binding = json!({
            "symbols": ["aapl", "US:AAPL"],
            "executionMode": "LIVE",
            "chartType": "HeikinAshi",
            "market": "us",
            "sessions": "REGULAR, extended",
            "brokerAccount": {"accountId": 42, "tradingEnvironment": "simulate"}
        });
        normalize_strategy_binding(&mut binding).expect("binding should normalize");
        assert_eq!(binding["executionMode"], "live");
        assert_eq!(binding["executeOrders"], true);
        assert_eq!(binding["chartType"], "heikinashi");
        assert_eq!(binding["market"], "US");
        assert_eq!(binding["symbols"], json!(["US.AAPL"]));
        assert_eq!(binding["sessions"], json!(["extended", "regular"]));
        assert_eq!(binding["brokerAccount"]["tradingEnvironment"], "SIMULATE");

        let mut invalid = json!({"chartType": "renko"});
        assert!(normalize_strategy_binding(&mut invalid).is_err());
        let mut invalid = json!({"market": "EU"});
        assert!(normalize_strategy_binding(&mut invalid).is_err());
        let mut invalid = json!({"sessions": ["regular", "premarket"]});
        assert!(normalize_strategy_binding(&mut invalid).is_err());
        for mut invalid in [
            json!({"chartType": 1}),
            json!({"interval": 5}),
            json!({"symbols": 1}),
            json!({"symbols": [false]}),
            json!({"executeOrders": "false"}),
            json!({"brokerAccount": []}),
        ] {
            assert!(
                normalize_strategy_binding(&mut invalid).is_err(),
                "{invalid}"
            );
        }
        assert!(trading_environment_is_real(
            &json!({"brokerAccount": {"env": "REAL"}})
        ));
        assert!(!trading_environment_is_real(
            &json!({"tradingEnvironment": "SIMULATE", "brokerAccount": {"env": "REAL"}})
        ));
    }

    #[test]
    fn pine_runtime_checkpoint_restores_latest_bar_and_intent_keys() {
        let dir = tempfile::tempdir().expect("tempdir");
        let path = dir.path().join("strategy.db");
        seed_strategy_test_db(&path);
        let definitions = Arc::new(
            StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
                .expect("open definition store"),
        );
        let store = StrategyRuntimeStore::from_definition_store(&definitions);
        store
            .seed_instance_with_binding(
                "checkpoint-inst",
                "RUNNING",
                json!({"symbols": ["US.AAPL"]}),
                "2026-09-05T00:00:00Z",
            )
            .expect("seed instance");
        let mut keys = BTreeSet::new();
        keys.insert("100:entry:0".to_owned());
        persist_pine_runtime_checkpoint(&store, "checkpoint-inst", "US.AAPL", 2, 200, &keys)
            .expect("persist first checkpoint");
        keys.insert("200:entry:0".to_owned());
        persist_pine_runtime_checkpoint(&store, "checkpoint-inst", "US.AAPL", 3, 300, &keys)
            .expect("persist second checkpoint");

        let (last_closed, sessions) =
            restore_pine_runtime_state(&store, "checkpoint-inst", &["US.AAPL".to_owned()])
                .expect("restore checkpoint");
        assert_eq!(last_closed.get("US.AAPL"), Some(&300));
        assert_eq!(sessions["US.AAPL"].revision, 3);
        assert_eq!(sessions["US.AAPL"].submitted_intents.len(), 2);
        store
            .update_status("checkpoint-inst", "STOPPED", "2026-09-05T00:30:00Z")
            .expect("stop before binding update");
        store
            .update_binding(
                "checkpoint-inst",
                json!({"symbols": ["US.AAPL"], "interval": "1h"}),
                "2026-09-05T01:00:00Z",
            )
            .expect("change binding scope");
        let (last_closed, _) =
            restore_pine_runtime_state(&store, "checkpoint-inst", &["US.AAPL".to_owned()])
                .expect("restore changed binding");
        assert!(
            last_closed.is_empty(),
            "old binding checkpoints must not suppress new signals"
        );
        store
            .append_audit_event(
                "checkpoint-inst",
                "PINE_SESSION_CHECKPOINT",
                "{broken",
                i64::MAX,
            )
            .expect("inject corrupt checkpoint");
        assert!(
            restore_pine_runtime_state(&store, "checkpoint-inst", &["US.AAPL".to_owned()]).is_err()
        );
    }
}
