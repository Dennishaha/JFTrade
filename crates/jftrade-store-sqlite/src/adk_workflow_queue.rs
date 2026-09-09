//! Atomically advance a trigger and enqueue its invocation(s).
use super::*;

impl AdkStore {
    pub fn enqueue_workflow_trigger_invocations(
        &self,
        trigger: &StoredAdkWorkflowTrigger,
        next_payload: &Value,
        next_run_at: &str,
        invocations: &[(String, Value)],
    ) -> Result<bool, AdkStoreError> {
        let now = Self::now_rfc3339();
        let mut connection = self.lock_connection()?;
        let tx = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(AdkStoreError::Query)?;
        let affected = tx
            .execute(
                "UPDATE adk_workflow_triggers SET next_run_at=?1, payload_json=?2, updated_at=?3
             WHERE id=?4 AND updated_at=?5 AND status='ENABLED'
             AND COALESCE(json_extract(payload_json,'$.deletedAt'),'')=''",
                params![
                    next_run_at,
                    next_payload.to_string(),
                    now,
                    trigger.id,
                    trigger.updated_at
                ],
            )
            .map_err(AdkStoreError::Query)?;
        if affected == 0 {
            return Ok(false);
        }
        for (id, body) in invocations {
            let payload = serde_json::json!({"id":id,"workflowId":trigger.workflow_id,
                "triggerId":trigger.id,"triggerType":trigger.trigger_type,"status":"QUEUED",
                "runId":"","sessionId":"", "inputs":body,
                "schedulerInvocation":{"triggerId":trigger.id,"body":body}});
            tx.execute("INSERT INTO adk_workflow_trigger_logs
                (id,workflow_id,trigger_id,trigger_type,status,run_id,payload_json,created_at,updated_at)
                VALUES(?1,?2,?3,?4,'QUEUED','',?5,?6,?6)",
                params![id,trigger.workflow_id,trigger.id,trigger.trigger_type,payload.to_string(),now],
            ).map_err(AdkStoreError::Query)?;
        }
        tx.commit().map_err(AdkStoreError::Query)?;
        Ok(true)
    }
}
