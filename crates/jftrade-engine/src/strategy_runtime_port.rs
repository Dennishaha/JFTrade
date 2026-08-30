use super::*;
use super::strategy_runtime_activity::*;
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
    /// Reconcile persisted RUNNING rows before the API listener is exposed.
    /// A process restart either re-acquires the demand and starts a real
    /// worker task, or durably marks the row FAILED with an actionable error;
    /// stale RUNNING state is never advertised as healthy.
    pub(crate) fn restore_running_instances(&self) -> Result<(), String> {
        let instances = self
            .store
            .list_instances()
            .map_err(|error| error.to_string())?;
        for instance in instances.into_iter().filter(|instance| {
            instance.runtime_active || instance.status.eq_ignore_ascii_case("RUNNING")
        }) {
            let binding = match self.effective_binding(&instance) {
                Ok(binding) => binding,
                Err(error) => {
                    self.mark_recovery_failed(&instance, strategy_write_error_message(error))?;
                    continue;
                }
            };
            if let Some(error) = self.manager.dependency_error() {
                self.mark_recovery_failed(&instance, strategy_write_error_message(error))?;
                continue;
            }
            if let Err(error) = self.manager.acquire_demand(&instance.id, &binding) {
                self.mark_recovery_failed(&instance, strategy_write_error_message(error))?;
                continue;
            }
            if let Err(error) = self.manager.spawn_task(
                instance.id.clone(),
                binding,
                Arc::clone(&self.store),
            ) {
                self.manager.release_demand(&instance.id);
                self.mark_recovery_failed(&instance, strategy_write_error_message(error))?;
                continue;
            }
            self.store
                .append_audit_event(
                    &instance.id,
                    "RECOVERED",
                    "strategy runtime resumed after product restart",
                    now_millis(),
                )
                .map_err(|error| error.to_string())?;
        }
        Ok(())
    }

    fn mark_recovery_failed(
        &self,
        instance: &jftrade_store_sqlite::StoredRuntimeInstance,
        message: String,
    ) -> Result<(), String> {
        let now = now_millis();
        self.store
            .update_observation_with_events(
                &instance.id,
                "FAILED",
                &binding_symbols(&instance.binding).unwrap_or_default(),
                Some(&message),
                None,
                None,
                None,
                now,
            )
            .map_err(|error| error.to_string())?;
        self.store
            .append_log_event(&instance.id, &message, "error", now)
            .map_err(|error| error.to_string())?;
        let timestamp = now_rfc3339()?;
        self.store
            .update_status(&instance.id, "FAILED", &timestamp)
            .map(|_| ())
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
            Value::Bool(runtime.is_some() && source_format.is_some()),
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
        let events = self
            .store
            .list_log_events(instance_id)
            .map_err(|error| StrategyReadSnapshotError::Unavailable(error.to_string()))?;
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
        let events = self
            .store
            .list_audit_events(instance_id)
            .map_err(|error| StrategyReadSnapshotError::Unavailable(error.to_string()))?;
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
