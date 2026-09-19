// Memory and audit listings for the production ADK read port.
//
// Extracted from `product_production_ports_adk_read.rs` to keep the production
// file under the 800-line architecture limit.  These are inherent methods on
// `ProductionAdkPort`; the `AdkReadSnapshotPort` impl keeps dispatching to them
// so the wire behavior is unchanged.

impl ProductionAdkPort {
    fn memories(&self, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let scope = query_param(query, "scope")
            .map(|value| value.trim().to_ascii_lowercase())
            .filter(|value| !value.is_empty());
        if let Some(scope) = scope.as_deref()
            && scope != "workspace"
            && scope != "agent"
        {
            // Go's `handleADKMemory` reports an invalid scope through the
            // same `ADK_MEMORY_LIST_FAILED` code it uses for store faults.
            return Err(AdkReadSnapshotError::Failed {
                status: 400,
                code: "ADK_MEMORY_LIST_FAILED".to_owned(),
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
        // Go maps every `ListMemory` fault to `400 ADK_MEMORY_LIST_FAILED`,
        // including the invalid-scope branch above.
        let rows = self
            .store
            .list_memories()
            .map_err(|error| resource_list_failed(400, "ADK_MEMORY_LIST_FAILED", error))?;
        let values = rows
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
        // Parity: go:452dea11:internal/api/assistant/observability.go handleADKAudit
        // and persistence/store_audit.go auditEventWhere — `kind` and
        // `subjectId` narrow the store query before pagination, so both the
        // filtered total and the returned rows reflect the predicate.
        let kind = query_param(query, "kind")
            .map(|value| value.trim().to_owned())
            .filter(|value| !value.is_empty());
        let subject_id = query_param(query, "subjectId")
            .map(|value| value.trim().to_owned())
            .filter(|value| !value.is_empty());
        let values = self
            .store
            .list_audit_events()?
            .into_iter()
            .filter(|row| {
                kind.as_deref()
                    .is_none_or(|expected| row.kind == expected)
                    && subject_id
                        .as_deref()
                        .is_none_or(|expected| row.subject_id == expected)
            })
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

}
