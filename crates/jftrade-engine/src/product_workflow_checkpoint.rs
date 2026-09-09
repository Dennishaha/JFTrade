//! Incremental workflow progress, sharing the existing trigger log transaction/CAS.
use std::cell::RefCell;

use jftrade_assistant::WorkflowNodeRun;
use jftrade_store_sqlite::{AdkStore, StoredAdkWorkflowTriggerLog};
use serde_json::{Value, json};

pub(crate) struct WorkflowCheckpoint<'a> {
    store: &'a AdkStore,
    row: RefCell<StoredAdkWorkflowTriggerLog>,
}

impl<'a> WorkflowCheckpoint<'a> {
    pub(crate) fn new(store: &'a AdkStore, row: StoredAdkWorkflowTriggerLog) -> Self {
        Self {
            store,
            row: RefCell::new(row),
        }
    }

    pub(crate) fn payload(&self) -> Result<Value, (String, String)> {
        serde_json::from_str(&self.row.borrow().payload_json).map_err(failure)
    }

    pub(crate) fn node_runs(&self) -> Result<Vec<WorkflowNodeRun>, (String, String)> {
        serde_json::from_value(
            self.payload()?
                .get("nodeRuns")
                .cloned()
                .unwrap_or_else(|| json!([])),
        )
        .map_err(failure)
    }

    pub(crate) fn revision(&self) -> String {
        self.row.borrow().updated_at.clone()
    }

    pub(crate) fn save(
        &self,
        nodes: &[WorkflowNodeRun],
        status: &str,
        run_id: &str,
        session_id: &str,
        response: &Value,
    ) -> Result<(), (String, String)> {
        let mut payload = self.payload()?;
        payload["nodeRuns"] = json!(nodes);
        payload["status"] = json!(status);
        payload["runId"] = json!(run_id);
        payload["sessionId"] = json!(session_id);
        payload["result"] = response.clone();
        let row = self.row.borrow();
        let updated = self
            .store
            .update_workflow_trigger_log_if_revision(
                &row.id,
                &row.updated_at,
                status,
                run_id,
                &payload.to_string(),
            )
            .map_err(failure)?
            .ok_or_else(|| failure("workflow progress was superseded"))?;
        drop(row);
        *self.row.borrow_mut() = updated;
        Ok(())
    }
}

fn failure(error: impl std::fmt::Display) -> (String, String) {
    (
        error.to_string(),
        "ADK_WORKFLOW_CHECKPOINT_FAILED".to_owned(),
    )
}
