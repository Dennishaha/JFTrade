use std::fs;

use super::*;

#[path = "product_production_ports_adk_read_context.rs"]
mod context_projection;

use context_projection::rebuild_context_snapshot;

impl From<AdkStoreError> for AdkReadSnapshotError {
    fn from(error: AdkStoreError) -> Self {
        Self::Unavailable(error.to_string())
    }
}

impl From<AdkSessionStoreError> for AdkReadSnapshotError {
    fn from(error: AdkSessionStoreError) -> Self {
        Self::Unavailable(error.to_string())
    }
}

impl AdkReadSnapshotPort for ProductionAdkPort {
    fn read(&self, path: &str, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        match path {
            "/api/v1/adk" => self.snapshot(),
            "/api/v1/adk/agents" => self.agents(query),
            "/api/v1/adk/providers" => self.providers(),
            "/api/v1/adk/skills" => self.skills(),
            "/api/v1/adk/tasks" => self.tasks(query),
            "/api/v1/adk/workflows" => self.workflows(query),
            "/api/v1/adk/approvals" => self.approvals(query),
            "/api/v1/adk/runs" => self.runs(query),
            "/api/v1/adk/sessions" => self.sessions(query),
            "/api/v1/adk/memory" => self.memories(query),
            "/api/v1/adk/audit" => self.audit(query),
            "/api/v1/adk/optimization-tasks" => self.optimization_tasks(query),
            "/api/v1/adk/workflow-trigger-logs" => self.workflow_logs(query),
            "/api/v1/adk/metrics" => self.metrics(),
            "/api/v1/adk/tools" => Ok(AdkReadSnapshot::Json(
                json!({"tools": self.tool_catalog.values()}),
            )),
            _ => self.dynamic(path, query),
        }
    }
}

impl ProductionAdkPort {
    /// Attach a runtime-owned assistant chat adapter after the ADK stores and
    /// catalog have been validated.  This is intentionally explicit: a
    /// missing adapter is surfaced as 503 by [`AdkChatStreamPort::dispatch`]
    /// below and never masquerades as a successful synthetic response.
    #[allow(dead_code)]
    pub(crate) fn with_chat_runtime(mut self, chat_runtime: Arc<dyn AdkChatStreamPort>) -> Self {
        self.chat_runtime = Some(chat_runtime);
        self
    }

    fn snapshot(&self) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let mut agents = self.entities(self.store.list_agents()?, "agent")?;
        if agents.is_empty() {
            agents.push(builtin_agent(&self.tool_catalog));
        }
        let providers = self.entities(self.store.list_providers()?, "provider")?;
        let mut skills = self.entities(self.store.list_skills()?, "skill")?;
        if skills.is_empty() {
            skills = builtin_skills(&self.tool_catalog);
        }
        Ok(AdkReadSnapshot::Json(json!({
            "agents": agents,
            "providers": providers,
            "skills": skills,
            "tools": self.tool_catalog.values(),
            "runtimeSettings": self.runtime_settings()?,
        })))
    }

    fn runtime_settings(&self) -> Result<Value, AdkReadSnapshotError> {
        let store = SettingsFileStore::open_read_only(&self.settings_path)
            .map_err(|error| AdkReadSnapshotError::Unavailable(error.to_string()))?;
        let settings = store
            .load_assistant_runtime()
            .map_err(|error| AdkReadSnapshotError::Unavailable(error.to_string()))?
            .map(|settings| normalize_assistant_runtime_settings(&settings))
            .unwrap_or_default();
        serde_json::to_value(settings).map_err(|error| {
            AdkReadSnapshotError::Unavailable(format!("encode ADK runtime settings: {error}"))
        })
    }

    fn entities(
        &self,
        rows: Vec<jftrade_store_sqlite::StoredAdkEntity>,
        kind: &str,
    ) -> Result<Vec<Value>, AdkReadSnapshotError> {
        rows.into_iter()
            .map(|row| {
                let mut value: Value = serde_json::from_str(&row.payload_json)
                    .map_err(|error| invalid_payload(kind, error))?;
                if kind == "provider" {
                    sanitize_provider(&mut value, &row.id, &self.settings_path)?;
                }
                put_string(&mut value, "id", row.id);
                put_string(&mut value, "createdAt", row.created_at);
                put_string(&mut value, "updatedAt", row.updated_at);
                Ok(value)
            })
            .collect()
    }

    fn agents(&self, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let mut items = self.entities(self.store.list_agents()?, "agent")?;
        if items.is_empty() {
            items.push(builtin_agent(&self.tool_catalog));
        }
        Ok(AdkReadSnapshot::Json(page("agents", items, query, 100)))
    }

    fn providers(&self) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        Ok(AdkReadSnapshot::Json(json!({
            "providers": self.entities(self.store.list_providers()?, "provider")?
        })))
    }

    fn skills(&self) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let mut skills = self.entities(self.store.list_skills()?, "skill")?;
        if skills.is_empty() {
            skills = builtin_skills(&self.tool_catalog);
        }
        Ok(AdkReadSnapshot::Json(json!({"skills": skills})))
    }

    fn tasks(&self, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let values = self
            .store
            .list_tasks()?
            .into_iter()
            .map(|row| {
                payload(
                    &row.payload_json,
                    "task",
                    [
                        ("id", row.id),
                        ("status", row.status),
                        ("agentId", row.agent_id),
                        ("runId", row.run_id),
                        ("createdAt", row.created_at),
                        ("updatedAt", row.updated_at),
                    ],
                )
            })
            .collect::<Result<Vec<_>, _>>()?;
        let status = query_param(query, "status")
            .map(|value| value.trim().to_ascii_uppercase())
            .filter(|value| !value.is_empty());
        if let Some(status) = status.as_deref()
            && !matches!(
                status,
                "TODO" | "IN_PROGRESS" | "BLOCKED" | "DONE" | "CANCELLED"
            )
        {
            return Err(AdkReadSnapshotError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "invalid tasks query".to_owned(),
                retry_after_seconds: None,
            });
        }
        let agent_id = query_param(query, "agentId")
            .map(|value| value.trim().to_owned())
            .filter(|value| !value.is_empty());
        let run_id = query_param(query, "runId")
            .map(|value| value.trim().to_owned())
            .filter(|value| !value.is_empty());
        let values = values
            .into_iter()
            .filter(|value| {
                let matches_status = status.as_deref().is_none_or(|expected| {
                    value.get("status").and_then(Value::as_str) == Some(expected)
                });
                let matches_agent = agent_id.as_deref().is_none_or(|expected| {
                    value.get("agentId").and_then(Value::as_str) == Some(expected)
                });
                let matches_run = run_id.as_deref().is_none_or(|expected| {
                    value.get("runId").and_then(Value::as_str) == Some(expected)
                });
                matches_status && matches_agent && matches_run
            })
            .collect();
        Ok(AdkReadSnapshot::Json(page("tasks", values, query, 20)))
    }

    fn workflows(&self, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let values = self
            .store
            .list_workflows()?
            .into_iter()
            .map(|row| {
                if is_deleted_payload(&row.payload_json, "workflow")? {
                    return Ok(None);
                }
                payload(
                    &row.payload_json,
                    "workflow",
                    [
                        ("id", row.id),
                        ("status", row.status),
                        ("createdAt", row.created_at),
                        ("updatedAt", row.updated_at),
                    ],
                )
                .map(Some)
            })
            .collect::<Result<Vec<_>, _>>()?
            .into_iter()
            .flatten()
            .collect::<Vec<_>>();
        Ok(AdkReadSnapshot::Json(page("workflows", values, query, 100)))
    }

    fn approvals(&self, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let values = self
            .store
            .list_approvals()?
            .into_iter()
            .map(|row| {
                payload(
                    &row.payload_json,
                    "approval",
                    [
                        ("id", row.id),
                        ("runId", row.run_id),
                        ("agentId", row.agent_id),
                        ("status", row.status),
                        ("createdAt", row.created_at),
                        ("updatedAt", row.updated_at),
                    ],
                )
            })
            .collect::<Result<Vec<_>, _>>()?;
        Ok(AdkReadSnapshot::Json(page("approvals", values, query, 100)))
    }

    fn runs(&self, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let values = self
            .store
            .list_runs()?
            .into_iter()
            .map(|row| {
                payload(
                    &row.payload_json,
                    "run",
                    [
                        ("id", row.id),
                        ("sessionId", row.session_id),
                        ("agentId", row.agent_id),
                        ("status", row.status),
                        ("createdAt", row.created_at),
                        ("updatedAt", row.updated_at),
                    ],
                )
            })
            .collect::<Result<Vec<_>, _>>()?;
        Ok(AdkReadSnapshot::Json(page("runs", values, query, 100)))
    }

    fn sessions(&self, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let values = self
            .store
            .list_sessions()?
            .into_iter()
            .map(session_entity_value)
            .collect::<Result<Vec<_>, _>>()?;
        let agent_id = query_param(query, "agentId")
            .map(|value| value.trim().to_owned())
            .filter(|value| !value.is_empty());
        let title_query = query_param(query, "query")
            .map(|value| value.trim().to_lowercase())
            .filter(|value| !value.is_empty());
        let values = values
            .into_iter()
            .filter(|value| {
                let matches_agent = agent_id.as_deref().is_none_or(|agent| {
                    value.get("agentId").and_then(Value::as_str) == Some(agent)
                });
                let matches_title = title_query.as_deref().is_none_or(|needle| {
                    value
                        .get("title")
                        .and_then(Value::as_str)
                        .is_some_and(|title| title.to_lowercase().contains(needle))
                });
                matches_agent && matches_title
            })
            .collect();
        Ok(AdkReadSnapshot::Json(page("sessions", values, query, 100)))
    }

    fn memories(&self, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let scope = query_param(query, "scope")
            .map(|value| value.trim().to_ascii_lowercase())
            .filter(|value| !value.is_empty());
        if let Some(scope) = scope.as_deref()
            && scope != "workspace"
            && scope != "agent"
        {
            return Err(AdkReadSnapshotError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "memory scope must be workspace or agent".to_owned(),
                retry_after_seconds: None,
            });
        }
        let agent_id = query_param(query, "agentId")
            .map(|value| value.trim().to_owned())
            .filter(|value| !value.is_empty());
        let key = query_param(query, "key")
            .map(|value| normalize_memory_key(&value))
            .filter(|value| !value.is_empty());
        let values = self
            .store
            .list_memories()?
            .into_iter()
            .map(|row| {
                payload(
                    &row.payload_json,
                    "memory",
                    [
                        ("id", row.id),
                        ("agentId", row.agent_id),
                        ("scope", row.scope),
                        ("key", row.memory_key),
                        ("createdAt", row.created_at),
                        ("updatedAt", row.updated_at),
                    ],
                )
            })
            .collect::<Result<Vec<_>, _>>()?;
        let values = values
            .into_iter()
            .filter(|value| {
                let row_scope = value
                    .get("scope")
                    .and_then(Value::as_str)
                    .unwrap_or_default();
                let row_agent = value
                    .get("agentId")
                    .and_then(Value::as_str)
                    .unwrap_or_default();
                let row_key = value.get("key").and_then(Value::as_str).unwrap_or_default();
                let matches_scope = scope
                    .as_deref()
                    .is_none_or(|expected| row_scope == expected);
                let matches_agent = match scope.as_deref() {
                    Some("agent") => agent_id
                        .as_deref()
                        .is_none_or(|expected| row_agent == expected),
                    Some("workspace") => row_agent.is_empty(),
                    _ => agent_id
                        .as_deref()
                        .is_none_or(|expected| row_scope == "workspace" || row_agent == expected),
                };
                let matches_key = key.as_deref().is_none_or(|expected| row_key == expected);
                matches_scope && matches_agent && matches_key
            })
            .collect::<Vec<_>>();
        Ok(AdkReadSnapshot::Json(json!({"entries": values})))
    }

    fn audit(&self, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let values = self
            .store
            .list_audit_events()?
            .into_iter()
            .map(|row| {
                payload(
                    &row.payload_json,
                    "audit event",
                    [
                        ("id", row.id),
                        ("kind", row.kind),
                        ("subjectId", row.subject_id),
                        ("createdAt", row.created_at),
                    ],
                )
            })
            .collect::<Result<Vec<_>, _>>()?;
        Ok(AdkReadSnapshot::Json(page("events", values, query, 100)))
    }

    fn optimization_tasks(&self, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let values = self
            .store
            .list_optimization_tasks()?
            .into_iter()
            .map(|row| {
                payload(
                    &row.payload_json,
                    "optimization task",
                    [
                        ("id", row.id),
                        ("createdAt", row.created_at),
                        ("updatedAt", row.updated_at),
                    ],
                )
            })
            .collect::<Result<Vec<_>, _>>()?;
        Ok(AdkReadSnapshot::Json(page("tasks", values, query, 100)))
    }

    fn workflow_logs(&self, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let values = self
            .store
            .list_workflow_trigger_logs()?
            .into_iter()
            .map(|row| {
                payload(
                    &row.payload_json,
                    "workflow trigger log",
                    [
                        ("id", row.id),
                        ("workflowId", row.workflow_id),
                        ("triggerId", row.trigger_id),
                        ("triggerType", row.trigger_type),
                        ("status", row.status),
                        ("runId", row.run_id),
                        ("createdAt", row.created_at),
                        ("updatedAt", row.updated_at),
                    ],
                )
            })
            .collect::<Result<Vec<_>, _>>()?;
        Ok(AdkReadSnapshot::Json(page("logs", values, query, 100)))
    }

    fn metrics(&self) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        metrics::read(self)
    }

    fn dynamic(&self, path: &str, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        if let Some(id) = dynamic_id(path, "/api/v1/adk/optimization-tasks/", "") {
            let Some(row) = self.store.get_optimization_task(&id)? else {
                return Err(not_found("optimization task not found"));
            };
            return Ok(AdkReadSnapshot::Json(payload(
                &row.payload_json,
                "optimization task",
                [
                    ("id", row.id),
                    ("createdAt", row.created_at),
                    ("updatedAt", row.updated_at),
                ],
            )?));
        }
        if let Some(id) = dynamic_id(path, "/api/v1/adk/runs/", "/stream") {
            return self.stream_snapshot(&id, query);
        }
        if let Some(id) = dynamic_id(path, "/api/v1/adk/runs/", "") {
            let Some(row) = self.store.get_run(&id)? else {
                return Err(not_found("run not found"));
            };
            return Ok(AdkReadSnapshot::Json(payload(
                &row.payload_json,
                "run",
                [
                    ("id", row.id),
                    ("sessionId", row.session_id),
                    ("agentId", row.agent_id),
                    ("status", row.status),
                    ("createdAt", row.created_at),
                    ("updatedAt", row.updated_at),
                ],
            )?));
        }
        if let Some(id) = dynamic_id(path, "/api/v1/adk/sessions/", "/context") {
            let Some(session) = self.store.get_session(&id)? else {
                return Err(not_found("session not found"));
            };
            if let Some(state) = self.store.get_session_context(&id)? {
                return Ok(AdkReadSnapshot::Json(payload(
                    &state.payload_json,
                    "session context",
                    [("sessionId", id.clone())],
                )?));
            }
            // Older Go-owned databases may contain the session and transcript
            // events but no context-state row.  Rebuild the same durable
            // handoff projection used after compaction instead of returning a
            // synthetic zero-boundary snapshot.
            let events = self
                .session_store
                .list_events(&id)
                .map_err(|error| AdkReadSnapshotError::Unavailable(error.to_string()))?;
            return Ok(AdkReadSnapshot::Json(rebuild_context_snapshot(
                &id,
                &session.payload_json,
                &events,
                &self.store.list_handoff_segments(&id, true)?,
            )?));
        }
        if let Some(id) = dynamic_id(path, "/api/v1/adk/sessions/", "") {
            let Some(session) = self.store.get_session(&id)? else {
                return Err(not_found("session not found"));
            };
            let timeline = self
                .session_store
                .list_events(&id)
                .map_err(|e| AdkReadSnapshotError::Unavailable(e.to_string()))?
                .into_iter()
                .enumerate()
                .map(|(sequence, event)| timeline_value(event, sequence))
                .collect::<Vec<_>>();
            let runs = self
                .store
                .list_runs()?
                .into_iter()
                .filter(|run| run.session_id == id)
                .map(|run| {
                    payload(
                        &run.payload_json,
                        "run",
                        [
                            ("id", run.id),
                            ("sessionId", run.session_id),
                            ("agentId", run.agent_id),
                            ("status", run.status),
                            ("createdAt", run.created_at),
                            ("updatedAt", run.updated_at),
                        ],
                    )
                })
                .collect::<Result<Vec<_>, _>>()?;
            let artifacts = self.artifact_store.list_session_artifacts(&id).map_err(|e| AdkReadSnapshotError::Unavailable(e.to_string()))?.into_iter().map(|artifact| { let part: Value = serde_json::from_str(&artifact.part_json).map_err(|e| invalid_payload("artifact", e))?; Ok(json!({"appName": artifact.app_name, "userId": artifact.user_id, "sessionId": artifact.session_id, "fileName": artifact.file_name, "version": artifact.version, "part": part, "mimeType": artifact.mime_type, "customMetadata": artifact.custom_metadata_json.as_deref().map(serde_json::from_str::<Value>).transpose().map_err(|e| invalid_payload("artifact metadata", e))?, "createdAt": artifact.created_at, "updatedAt": artifact.updated_at})) }).collect::<Result<Vec<_>, AdkReadSnapshotError>>()?;
            return Ok(AdkReadSnapshot::Json(
                json!({"session": session_entity_value(session)?, "timeline": timeline, "runs": runs, "artifacts": artifacts, "composerState": composer_state_value(&id, self.store.get_session_composer_state(&id)?)?}),
            ));
        }
        if let Some(id) = dynamic_id(path, "/api/v1/adk/tasks/", "") {
            let Some(row) = self.store.get_task(&id)? else {
                return Err(not_found("task not found"));
            };
            return Ok(AdkReadSnapshot::Json(payload(
                &row.payload_json,
                "task",
                [
                    ("id", row.id),
                    ("status", row.status),
                    ("agentId", row.agent_id),
                    ("runId", row.run_id),
                    ("createdAt", row.created_at),
                    ("updatedAt", row.updated_at),
                ],
            )?));
        }
        if let Some(id) = dynamic_id(path, "/api/v1/adk/workflows/", "/triggers") {
            if self.store.get_workflow(&id)?.is_none() {
                return Err(not_found("workflow not found"));
            }
            let values = self
                .store
                .list_workflow_triggers(&id)?
                .into_iter()
                .map(|row| {
                    let deleted = is_deleted_payload(&row.payload_json, "workflow trigger")?;
                    if deleted {
                        return Ok(None);
                    }
                    workflow_trigger_value(
                        &row.payload_json,
                        [
                            ("id", row.id),
                            ("workflowId", row.workflow_id),
                            ("type", row.trigger_type),
                            ("status", row.status),
                            ("nextRunAt", row.next_run_at),
                            ("createdAt", row.created_at),
                            ("updatedAt", row.updated_at),
                        ],
                    )
                    .map(Some)
                })
                .collect::<Result<Vec<_>, _>>()?
                .into_iter()
                .flatten()
                .collect::<Vec<_>>();
            return Ok(AdkReadSnapshot::Json(json!({"triggers": values})));
        }
        if let Some(id) = dynamic_id(path, "/api/v1/adk/workflows/", "") {
            let Some(row) = self.store.get_workflow(&id)? else {
                return Err(not_found("workflow not found"));
            };
            if is_deleted_payload(&row.payload_json, "workflow")? {
                return Err(not_found("workflow not found"));
            }
            return Ok(AdkReadSnapshot::Json(payload(
                &row.payload_json,
                "workflow",
                [
                    ("id", row.id),
                    ("status", row.status),
                    ("createdAt", row.created_at),
                    ("updatedAt", row.updated_at),
                ],
            )?));
        }
        if let Some(id) = dynamic_id(path, "/api/v1/adk/streams/", "") {
            return self.stream_snapshot(&id, query);
        }
        Err(not_found("path not found"))
    }

    fn stream_snapshot(
        &self,
        stream_id: &str,
        query: &str,
    ) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let after = query_param(query, "after")
            .and_then(|value| value.trim().parse::<u64>().ok())
            .unwrap_or(0);
        let mut selected = None;
        for row in self.store.list_runs()? {
            if row.id == stream_id {
                selected = Some(row);
                break;
            }
            let value: Value =
                serde_json::from_str(&row.payload_json).map_err(|e| invalid_payload("run", e))?;
            if value.get("streamId").and_then(Value::as_str) == Some(stream_id) {
                selected = Some(row);
                break;
            }
        }
        let Some(row) = selected else {
            return Err(not_found("stream not found"));
        };
        let value: Value =
            serde_json::from_str(&row.payload_json).map_err(|e| invalid_payload("run", e))?;
        let Some(events) = value.get("streamEvents").and_then(Value::as_array) else {
            return Err(not_found("stream not found"));
        };
        let events = events
            .iter()
            .enumerate()
            .filter_map(|(index, data)| {
                let sequence = data
                    .get("sequence")
                    .and_then(Value::as_u64)
                    .unwrap_or(index as u64 + 1);
                (sequence > after).then(|| AdkReadEvent {
                    id: Some(sequence.to_string()),
                    data: data.clone(),
                })
            })
            .collect();
        Ok(AdkReadSnapshot::Stream(AdkReadStream {
            headers: vec![("X-ADK-Stream-ID".into(), stream_id.into())],
            events,
        }))
    }
}


fn estimate_context_tokens(value: &str) -> usize {
    let bytes = value.trim().len();
    if bytes == 0 {
        0
    } else {
        bytes.saturating_add(3) / 4
    }
}

fn context_status_for_read(ratio: f64, window: usize) -> &'static str {
    if window == 0 {
        "unknown"
    } else if ratio >= 0.93 {
        "critical"
    } else if ratio >= 0.85 {
        "near_limit"
    } else if ratio >= 0.70 {
        "warning"
    } else {
        "healthy"
    }
}

fn is_context_user_event(event: &jftrade_store_sqlite::StoredAdkEvent) -> bool {
    event.author.trim().eq_ignore_ascii_case("user")
        || event.author.to_ascii_lowercase().contains("user")
}

fn recent_context_event_start(
    events: &[jftrade_store_sqlite::StoredAdkEvent],
    window: usize,
) -> usize {
    let mut hits = 0;
    for index in (0..events.len()).rev() {
        if !is_context_user_event(&events[index]) {
            continue;
        }
        hits += 1;
        if hits >= window {
            return index;
        }
    }
    0
}

fn protected_context_event_start(events: &[jftrade_store_sqlite::StoredAdkEvent]) -> usize {
    events
        .iter()
        .position(|event| {
            let content = event.content.to_ascii_lowercase();
            content.contains("approval")
                || content.contains("pending_input")
                || content.contains("pending approval")
                || content.contains("awaiting_input")
        })
        .unwrap_or(events.len())
}

fn sanitize_provider(
    value: &mut Value,
    provider_id: &str,
    settings_path: &std::path::Path,
) -> Result<(), AdkReadSnapshotError> {
    let object = value.as_object_mut().ok_or_else(|| {
        invalid_payload(
            "provider",
            "stored ADK provider payload must be a JSON object",
        )
    })?;
    let payload_has_key = object
        .get("apiKey")
        .and_then(Value::as_str)
        .is_some_and(|key| !key.trim().is_empty());
    object.remove("apiKey");
    let secret_has_key = read_secret_presence(settings_path, provider_id)?;
    object.insert(
        "hasApiKey".to_owned(),
        Value::Bool(payload_has_key || secret_has_key),
    );
    Ok(())
}

fn read_secret_presence(
    settings_path: &std::path::Path,
    provider_id: &str,
) -> Result<bool, AdkReadSnapshotError> {
    let path = std::env::var_os("JFTRADE_ADK_SECRETS")
        .filter(|value| !value.is_empty())
        .map(std::path::PathBuf::from)
        .unwrap_or_else(|| {
            settings_path
                .parent()
                .filter(|parent| !parent.as_os_str().is_empty())
                .map_or_else(
                    || std::path::PathBuf::from("secrets/adk-secrets.json"),
                    |parent| parent.join("secrets/adk-secrets.json"),
                )
        });
    let bytes = match fs::read(path) {
        Ok(bytes) => bytes,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(false),
        Err(error) => return Err(AdkReadSnapshotError::Unavailable(error.to_string())),
    };
    if bytes.iter().all(u8::is_ascii_whitespace) {
        return Ok(false);
    }
    let secrets: std::collections::BTreeMap<String, String> = serde_json::from_slice(&bytes)
        .map_err(|error| AdkReadSnapshotError::Unavailable(error.to_string()))?;
    Ok(secrets
        .get(provider_id)
        .is_some_and(|key| !key.trim().is_empty()))
}
