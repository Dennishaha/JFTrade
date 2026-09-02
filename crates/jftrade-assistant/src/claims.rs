use std::collections::BTreeMap;

use serde::{Deserialize, Serialize};
use serde_json::Value;
use thiserror::Error;

use crate::ToolIdempotencyMode;

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct RunLease {
    pub run_id: String,
    pub owner_id: String,
    pub fencing_token: u64,
    pub heartbeat_at_unix_ms: i64,
    pub expires_at_unix_ms: i64,
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum ToolInvocationStatus {
    Running,
    Completed,
    Indeterminate,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct ToolInvocation {
    pub run_id: String,
    pub idempotency_key: String,
    pub tool_name: String,
    pub status: ToolInvocationStatus,
    pub owner_id: String,
    pub fencing_token: u64,
    pub run_lease_token: u64,
    pub input: Value,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub output: Option<Value>,
    pub lease_expires_at_unix_ms: i64,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct ToolClaimRequest {
    pub run_id: String,
    pub idempotency_key: String,
    pub tool_name: String,
    pub owner_id: String,
    pub run_lease_token: u64,
    pub input: Value,
    pub mode: ToolIdempotencyMode,
    pub now_unix_ms: i64,
    pub ttl_ms: i64,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct ToolInvocationTicket {
    pub run_id: String,
    pub idempotency_key: String,
    pub owner_id: String,
    pub fencing_token: u64,
    pub run_lease_token: u64,
    pub execute: bool,
    pub replayed: bool,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub output: Option<Value>,
}

#[derive(Clone, Debug, Default, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct ClaimCheckpoint {
    #[serde(default)]
    pub run_leases: BTreeMap<String, RunLease>,
    #[serde(default)]
    pub tool_invocations: BTreeMap<String, ToolInvocation>,
}

#[derive(Debug, Error, Eq, PartialEq)]
pub enum ClaimError {
    #[error("execution claim is incomplete")]
    Incomplete,
    #[error("execution claim TTL must be positive")]
    InvalidTtl,
    #[error("run lease is held by another executor")]
    RunLeaseHeld,
    #[error("run lease fencing token is no longer current")]
    RunLeaseLost,
    #[error("tool invocation key was reused with different tool input")]
    ToolKeyReused,
    #[error("tool invocation is already in flight")]
    ToolInvocationInFlight,
    #[error("tool invocation outcome is unknown after executor failure")]
    ToolOutcomeUnknown,
    #[error("tool invocation fencing token is no longer current")]
    ToolInvocationLost,
    #[error("claim checkpoint is invalid: {0}")]
    InvalidCheckpoint(String),
}

#[derive(Clone, Debug, Default)]
pub struct ClaimStore {
    checkpoint: ClaimCheckpoint,
}

impl ClaimStore {
    pub fn restore(bytes: &[u8]) -> Result<Self, ClaimError> {
        let checkpoint = serde_json::from_slice(bytes)
            .map_err(|error| ClaimError::InvalidCheckpoint(error.to_string()))?;
        Ok(Self { checkpoint })
    }

    pub fn checkpoint_json(&self) -> Result<Vec<u8>, ClaimError> {
        serde_json::to_vec(&self.checkpoint)
            .map_err(|error| ClaimError::InvalidCheckpoint(error.to_string()))
    }

    pub fn checkpoint(&self) -> &ClaimCheckpoint {
        &self.checkpoint
    }

    pub fn claim_run(
        &mut self,
        run_id: &str,
        owner_id: &str,
        now_unix_ms: i64,
        ttl_ms: i64,
    ) -> Result<RunLease, ClaimError> {
        if run_id.trim().is_empty() || owner_id.trim().is_empty() {
            return Err(ClaimError::Incomplete);
        }
        if ttl_ms <= 0 {
            return Err(ClaimError::InvalidTtl);
        }
        let expires_at_unix_ms = now_unix_ms.saturating_add(ttl_ms);
        let lease = match self.checkpoint.run_leases.get_mut(run_id) {
            Some(lease) if lease.expires_at_unix_ms > now_unix_ms && lease.owner_id != owner_id => {
                return Err(ClaimError::RunLeaseHeld);
            }
            Some(lease) if lease.expires_at_unix_ms > now_unix_ms => {
                lease.heartbeat_at_unix_ms = now_unix_ms;
                lease.expires_at_unix_ms = expires_at_unix_ms;
                lease.clone()
            }
            Some(lease) => {
                lease.owner_id = owner_id.to_owned();
                lease.fencing_token = lease.fencing_token.saturating_add(1);
                lease.heartbeat_at_unix_ms = now_unix_ms;
                lease.expires_at_unix_ms = expires_at_unix_ms;
                lease.clone()
            }
            None => {
                let lease = RunLease {
                    run_id: run_id.to_owned(),
                    owner_id: owner_id.to_owned(),
                    fencing_token: 1,
                    heartbeat_at_unix_ms: now_unix_ms,
                    expires_at_unix_ms,
                };
                self.checkpoint
                    .run_leases
                    .insert(run_id.to_owned(), lease.clone());
                lease
            }
        };
        Ok(lease)
    }

    pub fn heartbeat_run(
        &mut self,
        lease: &RunLease,
        now_unix_ms: i64,
        ttl_ms: i64,
    ) -> Result<RunLease, ClaimError> {
        if ttl_ms <= 0 {
            return Err(ClaimError::InvalidTtl);
        }
        let current = self
            .checkpoint
            .run_leases
            .get_mut(&lease.run_id)
            .ok_or(ClaimError::RunLeaseLost)?;
        if current.owner_id != lease.owner_id
            || current.fencing_token != lease.fencing_token
            || current.expires_at_unix_ms <= now_unix_ms
        {
            return Err(ClaimError::RunLeaseLost);
        }
        current.heartbeat_at_unix_ms = now_unix_ms;
        current.expires_at_unix_ms = now_unix_ms.saturating_add(ttl_ms);
        Ok(current.clone())
    }

    pub fn release_run(&mut self, lease: &RunLease, now_unix_ms: i64) -> bool {
        let Some(current) = self.checkpoint.run_leases.get_mut(&lease.run_id) else {
            return false;
        };
        if current.owner_id != lease.owner_id || current.fencing_token != lease.fencing_token {
            return false;
        }
        current.owner_id.clear();
        current.fencing_token = current.fencing_token.saturating_add(1);
        current.heartbeat_at_unix_ms = now_unix_ms;
        current.expires_at_unix_ms = now_unix_ms;
        true
    }

    pub fn claim_tool(
        &mut self,
        request: ToolClaimRequest,
    ) -> Result<ToolInvocationTicket, ClaimError> {
        validate_tool_request(&request)?;
        self.require_run_lease(
            &request.run_id,
            &request.owner_id,
            request.run_lease_token,
            request.now_unix_ms,
        )?;
        let key = invocation_key(&request.run_id, &request.idempotency_key);
        let expires_at = request.now_unix_ms.saturating_add(request.ttl_ms);
        let invocation = match self.checkpoint.tool_invocations.get_mut(&key) {
            None => {
                let invocation = ToolInvocation {
                    run_id: request.run_id,
                    idempotency_key: request.idempotency_key,
                    tool_name: request.tool_name,
                    status: ToolInvocationStatus::Running,
                    owner_id: request.owner_id,
                    fencing_token: 1,
                    run_lease_token: request.run_lease_token,
                    input: request.input,
                    output: None,
                    lease_expires_at_unix_ms: expires_at,
                };
                self.checkpoint
                    .tool_invocations
                    .insert(key, invocation.clone());
                return Ok(ticket(&invocation, true, false));
            }
            Some(invocation) => invocation,
        };
        if invocation.tool_name != request.tool_name || invocation.input != request.input {
            return Err(ClaimError::ToolKeyReused);
        }
        match invocation.status {
            ToolInvocationStatus::Completed => Ok(ticket(invocation, false, true)),
            ToolInvocationStatus::Indeterminate => Err(ClaimError::ToolOutcomeUnknown),
            ToolInvocationStatus::Running
                if invocation.lease_expires_at_unix_ms > request.now_unix_ms =>
            {
                Err(ClaimError::ToolInvocationInFlight)
            }
            ToolInvocationStatus::Running if request.mode == ToolIdempotencyMode::FailClosed => {
                invocation.status = ToolInvocationStatus::Indeterminate;
                invocation.lease_expires_at_unix_ms = request.now_unix_ms;
                Err(ClaimError::ToolOutcomeUnknown)
            }
            ToolInvocationStatus::Running => {
                invocation.owner_id = request.owner_id;
                invocation.fencing_token = invocation.fencing_token.saturating_add(1);
                invocation.run_lease_token = request.run_lease_token;
                invocation.lease_expires_at_unix_ms = expires_at;
                Ok(ticket(invocation, true, false))
            }
        }
    }

    pub fn complete_tool(
        &mut self,
        ticket: &ToolInvocationTicket,
        output: Value,
        now_unix_ms: i64,
    ) -> Result<(), ClaimError> {
        self.require_run_lease(
            &ticket.run_id,
            &ticket.owner_id,
            ticket.run_lease_token,
            now_unix_ms,
        )?;
        let invocation = self
            .checkpoint
            .tool_invocations
            .get_mut(&invocation_key(&ticket.run_id, &ticket.idempotency_key))
            .ok_or(ClaimError::ToolInvocationLost)?;
        if invocation.owner_id != ticket.owner_id
            || invocation.fencing_token != ticket.fencing_token
            || invocation.status != ToolInvocationStatus::Running
            || invocation.lease_expires_at_unix_ms <= now_unix_ms
        {
            return Err(ClaimError::ToolInvocationLost);
        }
        invocation.status = ToolInvocationStatus::Completed;
        invocation.output = Some(output);
        invocation.lease_expires_at_unix_ms = now_unix_ms;
        Ok(())
    }

    fn require_run_lease(
        &self,
        run_id: &str,
        owner_id: &str,
        fencing_token: u64,
        now_unix_ms: i64,
    ) -> Result<(), ClaimError> {
        let current = self
            .checkpoint
            .run_leases
            .get(run_id)
            .ok_or(ClaimError::RunLeaseLost)?;
        if current.owner_id != owner_id
            || current.fencing_token != fencing_token
            || current.expires_at_unix_ms <= now_unix_ms
        {
            return Err(ClaimError::RunLeaseLost);
        }
        Ok(())
    }
}

fn validate_tool_request(request: &ToolClaimRequest) -> Result<(), ClaimError> {
    if request.run_id.trim().is_empty()
        || request.idempotency_key.trim().is_empty()
        || request.tool_name.trim().is_empty()
        || request.owner_id.trim().is_empty()
    {
        return Err(ClaimError::Incomplete);
    }
    if request.ttl_ms <= 0 {
        return Err(ClaimError::InvalidTtl);
    }
    Ok(())
}

fn ticket(invocation: &ToolInvocation, execute: bool, replayed: bool) -> ToolInvocationTicket {
    ToolInvocationTicket {
        run_id: invocation.run_id.clone(),
        idempotency_key: invocation.idempotency_key.clone(),
        owner_id: invocation.owner_id.clone(),
        fencing_token: invocation.fencing_token,
        run_lease_token: invocation.run_lease_token,
        execute,
        replayed,
        output: invocation.output.clone(),
    }
}

fn invocation_key(run_id: &str, idempotency_key: &str) -> String {
    format!("{run_id}\0{idempotency_key}")
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;

    fn request(lease: &RunLease, mode: ToolIdempotencyMode, now: i64) -> ToolClaimRequest {
        ToolClaimRequest {
            run_id: lease.run_id.clone(),
            idempotency_key: "key-1".to_owned(),
            tool_name: "read.market".to_owned(),
            owner_id: lease.owner_id.clone(),
            run_lease_token: lease.fencing_token,
            input: json!({"symbol": "AAPL"}),
            mode,
            now_unix_ms: now,
            ttl_ms: 30,
        }
    }

    #[test]
    fn stale_replay_safe_claim_takes_over_with_fencing() {
        let mut store = ClaimStore::default();
        let first_lease = store.claim_run("run", "owner-a", 100, 40).expect("lease");
        let first = store
            .claim_tool(request(&first_lease, ToolIdempotencyMode::ReplaySafe, 100))
            .expect("claim");
        assert_eq!(first.fencing_token, 1);
        let second_lease = store
            .claim_run("run", "owner-b", 141, 40)
            .expect("takeover");
        let second = store
            .claim_tool(request(&second_lease, ToolIdempotencyMode::ReplaySafe, 141))
            .expect("takeover claim");
        assert_eq!(second.fencing_token, 2);
        assert_eq!(
            store.complete_tool(&first, json!({"stale": true}), 142),
            Err(ClaimError::RunLeaseLost)
        );
    }

    #[test]
    fn fail_closed_stale_claim_becomes_indeterminate() {
        let mut store = ClaimStore::default();
        let first = store.claim_run("run", "owner-a", 100, 40).expect("lease");
        store
            .claim_tool(request(&first, ToolIdempotencyMode::FailClosed, 100))
            .expect("claim");
        let second = store.claim_run("run", "owner-b", 141, 40).expect("lease");
        assert_eq!(
            store.claim_tool(request(&second, ToolIdempotencyMode::FailClosed, 141)),
            Err(ClaimError::ToolOutcomeUnknown)
        );
    }
}
