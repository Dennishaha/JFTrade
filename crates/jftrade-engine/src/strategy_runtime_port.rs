use super::strategy_runtime_activity::*;
use super::*;

/// The reference runtime and source-format pair an instance may start on. The
/// names stay local so this module compiles in every includer; the reference
/// view keeps legacy values visible while refusing to start them.
const STARTABLE_RUNTIME_ID: &str = "pine-pinets";
const STARTABLE_SOURCE_FORMAT: &str = "pine-v6";

/// The reference instance view only marks an instance startable when its
/// binding names the PineTS runtime on the pine-v6 source format. Legacy
/// runtime or source-format strings stay visible in the wire projection but
/// must not be relaunchable.
fn startable_runtime(runtime: Option<&str>, source_format: Option<&str>) -> bool {
    runtime == Some(STARTABLE_RUNTIME_ID) && source_format == Some(STARTABLE_SOURCE_FORMAT)
}

#[derive(Debug)]
pub(crate) struct ProductionStrategyRuntimePort {
    pub(crate) store: Arc<StrategyRuntimeStore>,
    pub(crate) definitions: Arc<StrategyDefinitionStore>,
    pub(crate) manager: Arc<StrategyRuntimeManager>,
}

impl StrategyReadSnapshotPort for ProductionStrategyRuntimePort {
    fn read(&self, path: &str, query: &str) -> Result<Option<Value>, StrategyReadSnapshotError> {
        if path == "/api/v1/strategies" {
            let instances = self
                .store
                .list_instances()
                .map_err(|e| StrategyReadSnapshotError::Unavailable(e.to_string()))?;
            let items = instances
                .into_iter()
                .map(|instance| self.runtime_instance_wire(instance))
                .collect::<Result<Vec<_>, _>>()?;
            return Ok(Some(Value::Array(items)));
        }
        let Some((instance_id, activity)) = strategy_activity_path(path) else {
            return Ok(None);
        };
        if self
            .store
            .get_instance(instance_id)
            .map_err(|error| StrategyReadSnapshotError::Unavailable(error.to_string()))?
            .is_none()
        {
            return Ok(None);
        }
        let query = parse_activity_query(query, activity)?;
        match activity {
            "logs" => self.logs(instance_id, &query).map(Some),
            "audit" => self.audit(instance_id, &query).map(Some),
            _ => Ok(None),
        }
    }
}

impl ProductionStrategyRuntimePort {
    /// Reconcile persisted RUNNING and PAUSED rows before the API listener is
    /// exposed. A process restart either re-acquires the demand and starts a
    /// real worker task, or durably converges the row to STOPPED with an
    /// actionable error. A PAUSED row has no live worker to resume, so
    /// `ReconcileOnStartup` resets it to STOPPED and records why; stale run
    /// state is never advertised as healthy.
    pub(crate) fn restore_running_instances(&self) -> Result<(), String> {
        let instances = self
            .store
            .list_instances()
            .map_err(|error| error.to_string())?;
        for instance in instances {
            if instance.runtime_active || instance.status.eq_ignore_ascii_case("RUNNING") {
                self.resume_persisted_running_instance(&instance)?;
            } else if instance.status.eq_ignore_ascii_case("PAUSED") {
                self.reset_stale_paused_instance(&instance)?;
            }
        }
        Ok(())
    }

    /// Re-acquire the demand for a persisted RUNNING row and start a real
    /// worker task, or durably converge the row to STOPPED when recovery
    /// cannot be proven.
    fn resume_persisted_running_instance(
        &self,
        instance: &jftrade_store_sqlite::StoredRuntimeInstance,
    ) -> Result<(), String> {
        let binding = match self.effective_binding(instance) {
            Ok(binding) => binding,
            Err(error) => {
                self.mark_recovery_failed(instance, strategy_write_error_message(error))?;
                return Ok(());
            }
        };
        if let Some(error) = self.manager.dependency_error() {
            self.mark_recovery_failed(instance, strategy_write_error_message(error))?;
            return Ok(());
        }
        if let Err(error) = self.manager.acquire_demand(&instance.id, &binding) {
            self.mark_recovery_failed(instance, strategy_write_error_message(error))?;
            return Ok(());
        }
        if let Err(error) =
            self.manager
                .spawn_task(instance.id.clone(), binding, Arc::clone(&self.store))
        {
            self.manager.release_demand(&instance.id);
            self.mark_recovery_failed(instance, strategy_write_error_message(error))?;
            return Ok(());
        }
        self.store
            .append_audit_event(
                &instance.id,
                "RECOVERED",
                "strategy runtime resumed after product restart",
                now_millis(),
            )
            .map_err(|error| error.to_string())
    }

    /// A paused row cannot be resumed across a restart: the reference
    /// `ReconcileOnStartup` resets RUNNING and PAUSED rows to STOPPED, so the
    /// API never advertises a paused worker that has no live task behind it.
    fn reset_stale_paused_instance(
        &self,
        instance: &jftrade_store_sqlite::StoredRuntimeInstance,
    ) -> Result<(), String> {
        let now = now_millis();
        self.store
            .update_status(&instance.id, "STOPPED", &now_rfc3339()?)
            .map_err(|error| error.to_string())?;
        self.store
            .append_log_event(
                &instance.id,
                "reconciled strategy state from PAUSED to STOPPED after server startup",
                "warning",
                now,
            )
            .map_err(|error| error.to_string())?;
        self.store
            .append_audit_event(
                &instance.id,
                "RECONCILED",
                "server startup reset stale paused state to STOPPED",
                now,
            )
            .map_err(|error| error.to_string())
    }

    fn mark_recovery_failed(
        &self,
        instance: &jftrade_store_sqlite::StoredRuntimeInstance,
        message: String,
    ) -> Result<(), String> {
        // The reference catalog has no FAILED strategy status: a runtime
        // failure reconciles the instance to STOPPED, appends a `runtime_exited`
        // audit entry and writes an error log (`ReconcileRuntimeFailure`).
        let now = now_millis();
        self.store
            .update_observation_with_events(
                &instance.id,
                "STOPPED",
                &binding_symbols(&instance.binding).unwrap_or_default(),
                Some(&message),
                None,
                None,
                None,
                now,
            )
            .map_err(|error| error.to_string())?;
        self.store
            .append_log_event(
                &instance.id,
                &format!("strategy runtime exited unexpectedly: {message}"),
                "error",
                now,
            )
            .map_err(|error| error.to_string())?;
        let timestamp = now_rfc3339()?;
        self.store
            .update_status(&instance.id, "STOPPED", &timestamp)
            .map_err(|error| error.to_string())?;
        self.store
            .append_audit_event(&instance.id, "RUNTIME_EXITED", &message, now)
            .map_err(|error| error.to_string())
    }

    pub(super) fn effective_binding(
        &self,
        instance: &jftrade_store_sqlite::StoredRuntimeInstance,
    ) -> Result<Value, StrategyRuntimeWritePortError> {
        let mut binding = instance.binding.clone();
        let object =
            binding
                .as_object_mut()
                .ok_or_else(|| StrategyRuntimeWritePortError::Failed {
                    status: 400,
                    code: "STRATEGY_BINDING_INVALID".to_owned(),
                    message: "strategy binding must be an object".to_owned(),
                })?;
        if let Some(definition_id) = instance.definition_id.as_deref()
            && let Some(definition) = self
                .definitions
                .get_definition(definition_id, false)
                .map_err(|error| StrategyRuntimeWritePortError::Failed {
                    status: 500,
                    code: "STRATEGY_DEFINITION_READ_FAILED".to_owned(),
                    message: error.to_string(),
                })?
        {
            if !object.contains_key("script") && !definition.script.trim().is_empty() {
                object.insert("script".to_owned(), Value::String(definition.script));
            }
            if !object.contains_key("symbol") && !definition.symbol.trim().is_empty() {
                object.insert("symbols".to_owned(), json!([definition.symbol]));
            }
            if !object.contains_key("interval") && !definition.interval.trim().is_empty() {
                object.insert("interval".to_owned(), Value::String(definition.interval));
            }
        }
        normalize_strategy_binding(&mut binding)?;
        Ok(binding)
    }

    fn runtime_instance_wire(
        &self,
        instance: jftrade_store_sqlite::StoredRuntimeInstance,
    ) -> Result<Value, StrategyReadSnapshotError> {
        let observation = self
            .store
            .get_observation(&instance.id)
            .map_err(|error| StrategyReadSnapshotError::Unavailable(error.to_string()))?;
        let mut object = serde_json::Map::new();
        object.insert("id".to_owned(), Value::String(instance.id.clone()));
        object.insert("status".to_owned(), Value::String(instance.status.clone()));
        object.insert("binding".to_owned(), instance.binding.clone());
        object.insert("runtimeRisk".to_owned(), instance.runtime_risk.clone());
        object.insert(
            "runtimeRiskRevision".to_owned(),
            Value::from(instance.runtime_risk_revision),
        );
        object.insert(
            "definitionRevision".to_owned(),
            Value::from(instance.definition_revision),
        );
        object.insert(
            "runtimeActive".to_owned(),
            Value::Bool(instance.runtime_active),
        );
        if !instance.plugin_id.is_empty() {
            object.insert(
                "pluginId".to_owned(),
                Value::String(instance.plugin_id.clone()),
            );
        }
        if let Some(created_at) = instance
            .created_at
            .clone()
            .or_else(|| (!instance.updated_at.is_empty()).then_some(instance.updated_at.clone()))
        {
            object.insert("createdAt".to_owned(), Value::String(created_at));
        }
        if !instance.updated_at.is_empty() {
            object.insert(
                "updatedAt".to_owned(),
                Value::String(instance.updated_at.clone()),
            );
        }
        // Older catalog rows may not have copied definition metadata into the
        // operation payload. Recover the identity from the persisted binding
        // before consulting the definition store; never invent a definition.
        let definition_id = instance
            .definition_id
            .clone()
            .or_else(|| binding_string_opt(&instance.binding, &["definitionId", "strategyId"]));
        if let Some(definition_id) = definition_id {
            let definition = self
                .definitions
                .get_definition(&definition_id, false)
                .map_err(|error| StrategyReadSnapshotError::Unavailable(error.to_string()))?;
            let definition_name = instance
                .definition_name
                .clone()
                .or_else(|| {
                    binding_string_opt(&instance.binding, &["definitionName", "strategyName"])
                })
                .unwrap_or_default();
            let definition_version = instance
                .definition_version
                .clone()
                .or_else(|| {
                    binding_string_opt(&instance.binding, &["definitionVersion", "version"])
                })
                .unwrap_or_default();
            object.insert(
                "definition".to_owned(),
                json!({
                    "strategyId": definition_id,
                    "name": definition
                        .as_ref()
                        .map(|item| item.name.clone())
                        .filter(|name| !name.is_empty())
                        .unwrap_or(definition_name),
                    "version": definition
                        .as_ref()
                        .map(|item| item.version.clone())
                        .filter(|version| !version.is_empty())
                        .unwrap_or(definition_version),
                }),
            );
            let latest_version = definition
                .as_ref()
                .map(|item| item.version.trim().to_owned())
                .unwrap_or_default();
            let applied_version = instance.definition_version.clone();
            let is_latest = definition.as_ref().is_some_and(|item| {
                let latest = item.version.trim();
                !latest.is_empty()
                    && applied_version
                        .as_deref()
                        .is_some_and(|applied| applied.trim() == latest)
            });
            let can_apply_latest = definition.is_some()
                && !is_latest
                && !instance.runtime_active
                && instance.status.eq_ignore_ascii_case("STOPPED")
                && !latest_version.is_empty();
            object.insert(
                "definitionSync".to_owned(),
                json!({
                    "definitionId": definition_id,
                    "appliedVersion": applied_version,
                    "latestVersion": latest_version,
                    "isLatest": is_latest,
                    "canApplyLatest": can_apply_latest,
                }),
            );
        }
        let runtime = instance
            .binding
            .get("runtime")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty());
        let source_format = instance
            .binding
            .get("sourceFormat")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty());
        if let Some(runtime) = runtime {
            object.insert("runtime".to_owned(), Value::String(runtime.to_owned()));
        }
        if let Some(source_format) = source_format {
            object.insert(
                "sourceFormat".to_owned(),
                Value::String(source_format.to_owned()),
            );
        }
        object.insert(
            "startable".to_owned(),
            Value::Bool(startable_runtime(runtime, source_format)),
        );
        let mut params = serde_json::Map::new();
        if let Some(definition_id) = instance.definition_id.as_deref() {
            params.insert(
                "definitionId".to_owned(),
                Value::String(definition_id.to_owned()),
            );
        }
        if let Some(runtime) = runtime {
            params.insert("runtime".to_owned(), Value::String(runtime.to_owned()));
        }
        if let Some(source_format) = source_format {
            params.insert(
                "sourceFormat".to_owned(),
                Value::String(source_format.to_owned()),
            );
        }
        object.insert("params".to_owned(), Value::Object(params));
        let logs = self
            .store
            .list_log_events(&instance.id)
            .map_err(|error| StrategyReadSnapshotError::Unavailable(error.to_string()))?
            .into_iter()
            .take(20)
            .map(|event| event.raw)
            .collect::<Vec<_>>();
        object.insert(
            "logs".to_owned(),
            Value::Array(logs.into_iter().map(Value::String).collect()),
        );
        if let Some(observation) = observation {
            object.insert(
                "runtimeObservation".to_owned(),
                json!({
                    "actualStatus": observation.actual_status,
                    "activeSymbols": observation.active_symbols,
                    "lastClosedKlineAt": observation.last_closed_kline_at,
                    "lastSignalAt": observation.last_signal_at,
                    "lastOrderAt": observation.last_order_at,
                    "lastErrorAt": observation.last_error_at,
                    "lastError": observation.last_error,
                    "updatedAt": observation.updated_at,
                }),
            );
        }
        Ok(Value::Object(object))
    }

    fn logs(
        &self,
        instance_id: &str,
        query: &StrategyActivityQuery,
    ) -> Result<Value, StrategyReadSnapshotError> {
        let events = match self.store.list_log_events(instance_id) {
            Ok(events) => events,
            Err(error) => {
                eprintln!("strategy log activity unavailable for {instance_id}: {error}");
                let (logs, page) = page_values(Vec::<String>::new(), query);
                return Ok(json!({"instanceId": instance_id, "logs": logs, "page": page}));
            }
        };
        let filtered = events
            .into_iter()
            .filter(|event| query.includes(event.at_ms))
            .filter(|event| query.selector.is_empty() || event.level == query.selector)
            .map(|event| event.raw)
            .collect::<Vec<_>>();
        let (logs, page) = page_values(filtered, query);
        Ok(json!({"instanceId": instance_id, "logs": logs, "page": page}))
    }

    fn audit(
        &self,
        instance_id: &str,
        query: &StrategyActivityQuery,
    ) -> Result<Value, StrategyReadSnapshotError> {
        let events = match self.store.list_audit_events(instance_id) {
            Ok(events) => events,
            Err(error) => {
                eprintln!("strategy audit activity unavailable for {instance_id}: {error}");
                let (entries, page) = page_values(Vec::<Value>::new(), query);
                return Ok(json!({"instanceId": instance_id, "entries": entries, "page": page}));
            }
        };
        let filtered = events
            .into_iter()
            .filter(|event| query.includes(event.at_ms))
            .filter(|event| query.selector.is_empty() || event.kind == query.selector)
            .map(|event| {
                let at = timestamp_from_millis(event.at_ms)?;
                Ok(json!({
                    "instanceId": event.instance_id,
                    "kind": event.kind,
                    "detail": event.detail,
                    "at": at,
                }))
            })
            .collect::<Result<Vec<_>, StrategyReadSnapshotError>>()?;
        let (entries, page) = page_values(filtered, query);
        Ok(json!({"instanceId": instance_id, "entries": entries, "page": page}))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use jftrade_store_sqlite::{STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE, StrategyDefinitionStore};

    #[test]
    fn startable_requires_the_pinets_runtime_on_the_pine_v6_source_format() {
        // Parity: go:452dea11:internal/strategy/instanceview/view_test.go:46 TestStartableRequiresPineV6AndPineRuntime
        let directory = tempfile::tempdir().expect("temporary directory");
        let path = directory.path().join("strategy.db");
        let connection = rusqlite::Connection::open(&path).expect("database");
        jftrade_store_sqlite::initialize_current(&connection, "strategy").expect("schema");
        drop(connection);
        let definitions = Arc::new(
            StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
                .expect("definitions"),
        );
        let store = Arc::new(StrategyRuntimeStore::from_definition_store(&definitions));
        for (id, runtime, source_format) in [
            ("startable", "pine-pinets", "pine-v6"),
            ("legacy-runtime", "legacy-runtime", "pine-v6"),
            ("legacy-source", "pine-pinets", "legacy-source"),
            ("missing-binding", "", ""),
        ] {
            let mut binding = json!({"symbols": ["US.AAPL"]});
            if !runtime.is_empty() {
                binding["runtime"] = Value::String(runtime.to_owned());
            }
            if !source_format.is_empty() {
                binding["sourceFormat"] = Value::String(source_format.to_owned());
            }
            store
                .seed_instance_with_binding(id, "STOPPED", binding, "2026-09-01T00:00:00Z")
                .expect("seed instance");
        }
        let manager = Arc::new(StrategyRuntimeManager::new(
            None,
            None,
            None,
            None,
            Arc::new(ActiveProviderState::default()),
        ));
        let port = ProductionStrategyRuntimePort {
            store: store.clone(),
            definitions,
            manager,
        };
        let wire = |id: &str| {
            let instance = store.get_instance(id).expect("read").expect("instance");
            port.runtime_instance_wire(instance).expect("instance wire")
        };

        let startable = wire("startable");
        assert_eq!(startable["runtime"], "pine-pinets");
        assert_eq!(startable["sourceFormat"], "pine-v6");
        assert_eq!(startable["startable"], true);
        for id in ["legacy-runtime", "legacy-source", "missing-binding"] {
            assert_eq!(wire(id)["startable"], false, "{id} must not be startable");
        }
    }

    fn activity_store_query_failure_response(activity: &str) -> Value {
        let directory = tempfile::tempdir().expect("temporary directory");
        let path = directory.path().join("strategy.db");
        let connection = rusqlite::Connection::open(&path).expect("database");
        jftrade_store_sqlite::initialize_current(&connection, "strategy").expect("schema");
        drop(connection);
        let definitions = Arc::new(
            StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
                .expect("definitions"),
        );
        let store = Arc::new(StrategyRuntimeStore::from_definition_store(&definitions));
        store
            .seed_instance_with_binding(
                "activity",
                "STOPPED",
                json!({"symbols": ["US.AAPL"]}),
                "2026-09-01T00:00:00Z",
            )
            .expect("seed instance");
        let failure_connection = rusqlite::Connection::open(&path).expect("failure connection");
        failure_connection
            .execute_batch("DROP TABLE strategy_log_events; DROP TABLE strategy_audit_events;")
            .expect("drop activity tables");
        let manager = Arc::new(StrategyRuntimeManager::new(
            None,
            None,
            None,
            None,
            Arc::new(ActiveProviderState::default()),
        ));
        let port = ProductionStrategyRuntimePort {
            store,
            definitions,
            manager,
        };
        let path = format!("/api/v1/strategies/activity/{activity}");
        port.read(&path, "limit=10&offset=0")
            .expect("activity query should degrade")
            .expect("activity route response")
    }

    fn assert_known_empty_activity_page(response: &Value, key: &str) {
        assert_eq!(response["instanceId"], "activity");
        assert_eq!(response["page"]["total"], 0);
        assert_eq!(response["page"]["returned"], 0);
        assert_eq!(response["page"]["hasMore"], false);
        let entries = response[key].as_array().expect("activity entries");
        assert!(entries.is_empty(), "{key} should be an empty page");
    }

    #[test]
    // Parity: go:452dea11:internal/strategy/catalog/activity_degraded_test.go:65 TestCatalogActivityReturnsEmptyPagesWhenActivityStoreIsUnavailable
    fn activity_store_log_query_failure_returns_known_empty_page() {
        let response = activity_store_query_failure_response("logs");
        assert_known_empty_activity_page(&response, "logs");
    }

    #[test]
    // Parity: go:452dea11:internal/strategy/catalog/catalog_boundary_behavior_test.go:34 TestCatalogActivityQueryFailureReturnsKnownEmptyPage
    fn activity_store_audit_query_failure_returns_known_empty_page() {
        let response = activity_store_query_failure_response("audit");
        assert_known_empty_activity_page(&response, "entries");
    }

    #[test]
    // Parity: go:452dea11:internal/strategy/catalog/runtime_reconciliation_business_test.go:52 TestCatalogRuntimeFailureReconcilesOnlyRunningInstance
    fn recovery_failure_converges_the_running_instance_to_stopped() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let path = directory.path().join("strategy.db");
        let connection = rusqlite::Connection::open(&path).expect("database");
        jftrade_store_sqlite::initialize_current(&connection, "strategy").expect("schema");
        drop(connection);
        let definitions = Arc::new(
            StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
                .expect("definitions"),
        );
        let store = Arc::new(StrategyRuntimeStore::from_definition_store(&definitions));
        store
            .seed_instance_with_binding(
                "stale",
                "RUNNING",
                Value::String("invalid".into()),
                "2026-09-01T00:00:00Z",
            )
            .expect("seed stale instance");
        // Parity: go:452dea11:internal/strategy/catalog/runtime_reconciliation_business_test.go:52
        // `TestCatalogRuntimeFailureReconcilesOnlyRunningInstance` reconciles a
        // STOPPED instance without persisting anything.
        store
            .seed_instance_with_binding(
                "already-stopped",
                "STOPPED",
                Value::String("invalid".into()),
                "2026-09-01T00:00:00Z",
            )
            .expect("seed stopped instance");
        let stopped_audits_before = store
            .list_audit_events("already-stopped")
            .expect("stopped audits before")
            .len();
        let stopped_logs_before = store
            .list_log_events("already-stopped")
            .expect("stopped logs before")
            .len();
        let manager = Arc::new(StrategyRuntimeManager::new(
            None,
            None,
            None,
            None,
            Arc::new(ActiveProviderState::default()),
        ));
        let port = ProductionStrategyRuntimePort {
            store: store.clone(),
            definitions,
            manager,
        };
        port.restore_running_instances().expect("reconcile");
        let instance = store
            .get_instance("stale")
            .expect("read")
            .expect("instance");
        assert_eq!(instance.status, "STOPPED");
        assert!(!instance.runtime_active);
        let logs = store.list_log_events("stale").expect("logs");
        assert!(
            logs.iter().any(|event| {
                event.level.eq_ignore_ascii_case("error")
                    && event.raw.contains("strategy runtime exited unexpectedly: ")
            }),
            "recovery failure logs = {logs:?}"
        );
        let stopped = store
            .get_instance("already-stopped")
            .expect("read stopped")
            .expect("stopped instance");
        assert_eq!(stopped.status, "STOPPED");
        assert_eq!(
            store
                .list_audit_events("already-stopped")
                .expect("stopped audits after")
                .len(),
            stopped_audits_before,
            "a stopped instance must gain no audit rows during reconcile"
        );
        assert_eq!(
            store
                .list_log_events("already-stopped")
                .expect("stopped logs after")
                .len(),
            stopped_logs_before,
            "a stopped instance must gain no log rows during reconcile"
        );
        let audits = store.list_audit_events("stale").expect("audit");
        let exit = audits
            .iter()
            .find(|event| event.kind == "RUNTIME_EXITED")
            .expect("runtime exit audit entry");
        assert!(
            !exit.detail.trim().is_empty(),
            "runtime exit detail must carry the failure reason"
        );
    }

    // Parity: go:452dea11:internal/app/apiserver/servercore/system_reconcile_strategy_states_test.go:11 TestNewServerReconcilesPersistedActiveStrategyStates
    #[test]
    // Parity: go:452dea11:internal/strategy/catalog/runtime_reconciliation_business_test.go:80 TestCatalogStartupReconcileResetsStaleRunningAndPausedState
    fn startup_reconcile_resets_stale_paused_state_and_keeps_stopped_instances() {
        let dir = tempfile::tempdir().expect("tempdir");
        let path = dir.path().join("test.db");
        let connection = rusqlite::Connection::open(&path).expect("open");
        jftrade_store_sqlite::initialize_current(&connection, "strategy").expect("schema");
        drop(connection);
        let definitions = Arc::new(
            StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
                .expect("definitions"),
        );
        let store = Arc::new(StrategyRuntimeStore::from_definition_store(&definitions));
        store
            .seed_instance_with_binding(
                "paused-1",
                "PAUSED",
                Value::String("invalid".into()),
                "2026-09-01T00:00:00Z",
            )
            .expect("seed paused instance");
        store
            .seed_instance_with_binding(
                "stopped-1",
                "STOPPED",
                Value::String("invalid".into()),
                "2026-09-01T00:00:00Z",
            )
            .expect("seed stopped instance");
        let manager = Arc::new(StrategyRuntimeManager::new(
            None,
            None,
            None,
            None,
            Arc::new(ActiveProviderState::default()),
        ));
        let port = ProductionStrategyRuntimePort {
            store: store.clone(),
            definitions,
            manager,
        };
        port.restore_running_instances().expect("reconcile");
        let paused = store.get_instance("paused-1").unwrap().unwrap();
        assert_eq!(paused.status, "STOPPED");
        assert!(!paused.runtime_active);
        let audits = store.list_audit_events("paused-1").unwrap();
        assert!(
            audits.iter().any(|event| {
                event.kind == "RECONCILED"
                    && event.detail == "server startup reset stale paused state to STOPPED"
            }),
            "paused startup reconcile audit = {audits:?}"
        );
        let logs = store.list_log_events("paused-1").unwrap();
        assert!(
            logs.iter().any(|event| {
                event.level == "warning"
                    && event.raw
                        == "reconciled strategy state from PAUSED to STOPPED after server startup"
            }),
            "paused startup reconcile logs = {logs:?}"
        );
        assert_eq!(
            store.get_instance("stopped-1").unwrap().unwrap().status,
            "STOPPED"
        );
        assert!(store.list_audit_events("stopped-1").unwrap().is_empty());
        assert!(store.list_log_events("stopped-1").unwrap().is_empty());

        let paused_audit_count = store.list_audit_events("paused-1").unwrap().len();
        let paused_log_count = store.list_log_events("paused-1").unwrap().len();
        port.restore_running_instances()
            .expect("reconcile is idempotent");
        assert_eq!(
            store.list_audit_events("paused-1").unwrap().len(),
            paused_audit_count,
            "a second startup reconcile must not append another audit"
        );
        assert_eq!(
            store.list_log_events("paused-1").unwrap().len(),
            paused_log_count,
            "a second startup reconcile must not append another log"
        );
    }
}
