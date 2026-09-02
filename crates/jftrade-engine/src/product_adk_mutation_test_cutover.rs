//! Durable ADK mutations test-cutover adapter backed by `jftrade-store-sqlite`.
//!
//! This module is compiled only for Rust tests. Its SQLite schema connects to
//! the real `adk` component with schema validation and single-writer lease.

use std::path::{Path, PathBuf};
use std::sync::Arc;

use jftrade_store_sqlite::{ADK_TEST_CUTOVER_PROFILE, AdkTestCutoverStore};
use serde_json::{Value, json};

use super::product_adk_mutation_port::{
    AdkMutationInput, AdkMutationOperation, AdkMutationPort, AdkMutationPortError,
};

pub struct AdkMutationSqliteTestCutoverPort {
    path: PathBuf,
    store: Arc<AdkTestCutoverStore>,
}

impl std::fmt::Debug for AdkMutationSqliteTestCutoverPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("AdkMutationSqliteTestCutoverPort")
            .field("path", &self.path)
            .finish()
    }
}

#[allow(dead_code)]
impl AdkMutationSqliteTestCutoverPort {
    pub fn open(path: impl AsRef<Path>) -> Result<Self, String> {
        let path = path.as_ref().to_owned();
        let store = AdkTestCutoverStore::open_existing(&path, ADK_TEST_CUTOVER_PROFILE)
            .map_err(|err| err.to_string())?;
        Ok(Self {
            path,
            store: Arc::new(store),
        })
    }

    pub fn path(&self) -> &Path {
        &self.path
    }

    pub fn store(&self) -> &AdkTestCutoverStore {
        &self.store
    }
}

impl AdkMutationPort for AdkMutationSqliteTestCutoverPort {
    fn mutate(&self, input: &AdkMutationInput) -> Result<Value, AdkMutationPortError> {
        let body_str = serde_json::to_string(&input.body).unwrap_or_else(|_| "{}".to_owned());
        match input.operation {
            AdkMutationOperation::CreateAgent => {
                let id = input
                    .body
                    .get("id")
                    .and_then(Value::as_str)
                    .unwrap_or("agent-default");
                self.store
                    .upsert_agent(id, &body_str)
                    .map_err(|e| AdkMutationPortError::Unavailable(e.to_string()))?;
                Ok(json!({ "id": id, "accepted": true }))
            }
            AdkMutationOperation::UpdateAgent => {
                let id = input
                    .identifiers
                    .get("agentId")
                    .map(String::as_str)
                    .unwrap_or("agent-default");
                self.store
                    .upsert_agent(id, &body_str)
                    .map_err(|e| AdkMutationPortError::Unavailable(e.to_string()))?;
                Ok(json!({ "id": id, "accepted": true }))
            }
            AdkMutationOperation::DeleteAgent => {
                let id = input
                    .identifiers
                    .get("agentId")
                    .map(String::as_str)
                    .unwrap_or("");
                self.store
                    .delete_agent(id)
                    .map_err(|e| AdkMutationPortError::Unavailable(e.to_string()))?;
                Ok(json!({ "id": id, "deleted": true }))
            }
            AdkMutationOperation::CreateProvider => {
                let id = input
                    .body
                    .get("id")
                    .and_then(Value::as_str)
                    .unwrap_or("provider-default");
                self.store
                    .upsert_provider(id, &body_str)
                    .map_err(|e| AdkMutationPortError::Unavailable(e.to_string()))?;
                Ok(json!({ "id": id, "accepted": true }))
            }
            AdkMutationOperation::UpdateProvider => {
                let id = input
                    .identifiers
                    .get("providerId")
                    .map(String::as_str)
                    .unwrap_or("provider-default");
                self.store
                    .upsert_provider(id, &body_str)
                    .map_err(|e| AdkMutationPortError::Unavailable(e.to_string()))?;
                Ok(json!({ "id": id, "accepted": true }))
            }
            AdkMutationOperation::DeleteProvider => {
                let id = input
                    .identifiers
                    .get("providerId")
                    .map(String::as_str)
                    .unwrap_or("");
                self.store
                    .delete_provider(id)
                    .map_err(|e| AdkMutationPortError::Unavailable(e.to_string()))?;
                Ok(json!({ "id": id, "deleted": true }))
            }
            AdkMutationOperation::SetDefaultProvider => {
                let id = input
                    .identifiers
                    .get("providerId")
                    .map(String::as_str)
                    .unwrap_or("");
                Ok(json!({ "id": id, "isDefault": true }))
            }
            AdkMutationOperation::TestProvider => Ok(json!({ "ok": true, "latencyMs": 12 })),
            AdkMutationOperation::CreateSession => {
                let id = input
                    .body
                    .get("id")
                    .and_then(Value::as_str)
                    .unwrap_or("sess-default");
                let agent_id = input
                    .body
                    .get("agentId")
                    .and_then(Value::as_str)
                    .unwrap_or("agent-default");
                self.store
                    .upsert_session(id, agent_id, &body_str)
                    .map_err(|e| AdkMutationPortError::Unavailable(e.to_string()))?;
                Ok(json!({ "id": id, "agentId": agent_id, "accepted": true }))
            }
            AdkMutationOperation::DeleteSession => {
                let id = input
                    .identifiers
                    .get("sessionId")
                    .map(String::as_str)
                    .unwrap_or("");
                self.store
                    .delete_session(id)
                    .map_err(|e| AdkMutationPortError::Unavailable(e.to_string()))?;
                Ok(json!({ "id": id, "deleted": true }))
            }
            AdkMutationOperation::RenameSession => {
                let id = input
                    .identifiers
                    .get("sessionId")
                    .map(String::as_str)
                    .unwrap_or("");
                Ok(json!({ "id": id, "renamed": true }))
            }
            AdkMutationOperation::CancelRun => {
                let id = input
                    .identifiers
                    .get("runId")
                    .map(String::as_str)
                    .unwrap_or("");
                self.store
                    .update_run_status(id, "cancelled")
                    .map_err(|e| AdkMutationPortError::Unavailable(e.to_string()))?;
                Ok(json!({ "id": id, "status": "cancelled" }))
            }
            AdkMutationOperation::PauseRun => {
                let id = input
                    .identifiers
                    .get("runId")
                    .map(String::as_str)
                    .unwrap_or("");
                self.store
                    .update_run_status(id, "paused")
                    .map_err(|e| AdkMutationPortError::Unavailable(e.to_string()))?;
                Ok(json!({ "id": id, "status": "paused" }))
            }
            AdkMutationOperation::ResumeRun => {
                let id = input
                    .identifiers
                    .get("runId")
                    .map(String::as_str)
                    .unwrap_or("");
                self.store
                    .update_run_status(id, "running")
                    .map_err(|e| AdkMutationPortError::Unavailable(e.to_string()))?;
                Ok(json!({ "id": id, "status": "running" }))
            }
            AdkMutationOperation::Approve => {
                let id = input
                    .identifiers
                    .get("approvalId")
                    .map(String::as_str)
                    .unwrap_or("");
                self.store
                    .update_approval_status(id, "approved")
                    .map_err(|e| AdkMutationPortError::Unavailable(e.to_string()))?;
                Ok(json!({ "id": id, "status": "approved" }))
            }
            AdkMutationOperation::Deny => {
                let id = input
                    .identifiers
                    .get("approvalId")
                    .map(String::as_str)
                    .unwrap_or("");
                self.store
                    .update_approval_status(id, "denied")
                    .map_err(|e| AdkMutationPortError::Unavailable(e.to_string()))?;
                Ok(json!({ "id": id, "status": "denied" }))
            }
            AdkMutationOperation::CreateMemory => {
                let id = input
                    .body
                    .get("id")
                    .and_then(Value::as_str)
                    .unwrap_or("mem-default");
                let agent_id = input
                    .body
                    .get("agentId")
                    .and_then(Value::as_str)
                    .unwrap_or("agent-default");
                let scope = input
                    .body
                    .get("scope")
                    .and_then(Value::as_str)
                    .unwrap_or("session");
                let key = input
                    .body
                    .get("key")
                    .and_then(Value::as_str)
                    .unwrap_or("default");
                self.store
                    .upsert_memory(id, agent_id, scope, key, &body_str)
                    .map_err(|e| AdkMutationPortError::Unavailable(e.to_string()))?;
                Ok(json!({ "id": id, "accepted": true }))
            }
            AdkMutationOperation::DeleteMemory => {
                let id = input
                    .identifiers
                    .get("memoryId")
                    .map(String::as_str)
                    .unwrap_or("");
                self.store
                    .delete_memory(id)
                    .map_err(|e| AdkMutationPortError::Unavailable(e.to_string()))?;
                Ok(json!({ "id": id, "deleted": true }))
            }
            AdkMutationOperation::CreateWorkflow => {
                let id = input
                    .body
                    .get("id")
                    .and_then(Value::as_str)
                    .unwrap_or("wf-default");
                self.store
                    .upsert_workflow(id, "active", &body_str)
                    .map_err(|e| AdkMutationPortError::Unavailable(e.to_string()))?;
                Ok(json!({ "id": id, "accepted": true }))
            }
            AdkMutationOperation::UpdateWorkflow => {
                let id = input
                    .identifiers
                    .get("workflowId")
                    .map(String::as_str)
                    .unwrap_or("wf-default");
                self.store
                    .upsert_workflow(id, "active", &body_str)
                    .map_err(|e| AdkMutationPortError::Unavailable(e.to_string()))?;
                Ok(json!({ "id": id, "accepted": true }))
            }
            AdkMutationOperation::DeleteWorkflow => {
                let id = input
                    .identifiers
                    .get("workflowId")
                    .map(String::as_str)
                    .unwrap_or("");
                self.store
                    .delete_workflow(id)
                    .map_err(|e| AdkMutationPortError::Unavailable(e.to_string()))?;
                Ok(json!({ "id": id, "deleted": true }))
            }
            _ => Ok(json!({
                "accepted": true,
                "operation": input.operation.name(),
            })),
        }
    }
}
