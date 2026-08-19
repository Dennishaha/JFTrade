use std::collections::BTreeMap;

use jftrade_kernel::WireTimestamp;
use serde::{Deserialize, Serialize};
use serde_json::Value;

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct Session {
    pub id: String,
    pub agent_id: String,
    pub title: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub workflow_id: Option<String>,
    pub created_at: WireTimestamp,
    pub updated_at: WireTimestamp,
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum RunStatus {
    Running,
    Completed,
    PendingApproval,
    PendingInput,
    Failed,
    Denied,
    Cancelled,
    TimedOut,
    Paused,
}

impl RunStatus {
    pub const fn is_terminal(self) -> bool {
        matches!(
            self,
            Self::Completed | Self::Failed | Self::Denied | Self::Cancelled | Self::TimedOut
        )
    }
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum ApprovalStatus {
    Pending,
    Approved,
    Denied,
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum InputRequestStatus {
    Pending,
    Answered,
    Cancelled,
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum InputDecisionKind {
    MissingRequiredContext,
    MaterialTradeoff,
    ScopeBoundary,
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum ToolCallStatus {
    Pending,
    Running,
    PendingApproval,
    Succeeded,
    Failed,
    Denied,
    Cancelled,
    TimedOut,
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum ToolIdempotencyMode {
    FailClosed,
    ReplaySafe,
    Keyed,
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, Ord, PartialEq, PartialOrd, Serialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum WorkflowTaskStatus {
    Todo,
    InProgress,
    Blocked,
    Done,
    Cancelled,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct ToolDescriptor {
    pub name: String,
    pub display_name: String,
    pub description: String,
    pub category: String,
    pub permission: String,
    pub risk_level: String,
    pub idempotency_mode: ToolIdempotencyMode,
    pub allowed_modes: Vec<String>,
    pub requires_approval_in: Vec<String>,
    pub input_schema: Value,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct ToolCall {
    pub id: String,
    pub run_id: String,
    pub tool_name: String,
    pub permission: String,
    pub status: ToolCallStatus,
    #[serde(default)]
    pub input: Value,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub output: Option<Value>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub error: Option<String>,
    pub requires_user: bool,
    pub idempotency_key: String,
    pub created_at: WireTimestamp,
    pub updated_at: WireTimestamp,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub completed_at: Option<WireTimestamp>,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct Approval {
    pub id: String,
    pub run_id: String,
    pub agent_id: String,
    pub tool_name: String,
    #[serde(default)]
    pub input: Value,
    pub status: ApprovalStatus,
    pub reason: String,
    pub function_call_id: String,
    pub confirmation_call_id: String,
    pub created_at: WireTimestamp,
    pub updated_at: WireTimestamp,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct InputOption {
    pub id: String,
    pub label: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub description: String,
    #[serde(default)]
    pub recommended: bool,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct InputQuestion {
    pub id: String,
    pub question: String,
    pub options: Vec<InputOption>,
    pub allow_other: bool,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct InputAnswer {
    pub question_id: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub option_id: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub other_text: String,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct InputOptionDraft {
    pub label: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub description: String,
    #[serde(default)]
    pub recommended: bool,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct InputQuestionDraft {
    pub question: String,
    pub options: Vec<InputOptionDraft>,
    pub allow_other: bool,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct InputRequestDraft {
    pub decision_kind: InputDecisionKind,
    pub blocking_reason: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub title: String,
    pub questions: Vec<InputQuestionDraft>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct InputRequest {
    pub id: String,
    pub run_id: String,
    pub agent_id: String,
    pub function_call_id: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub title: String,
    pub status: InputRequestStatus,
    pub decision_kind: InputDecisionKind,
    pub blocking_reason: String,
    pub questions: Vec<InputQuestion>,
    #[serde(default)]
    pub answers: Vec<InputAnswer>,
    pub created_at: WireTimestamp,
    pub updated_at: WireTimestamp,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub answered_at: Option<WireTimestamp>,
}

#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct RunUsage {
    pub model_calls: u64,
    pub tool_calls_total: u64,
    pub duration_ms: u64,
    pub tokens_in: u64,
    pub tokens_out: u64,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct WorkflowTask {
    pub id: String,
    pub title: String,
    pub status: WorkflowTaskStatus,
    #[serde(default)]
    pub depends_on: Vec<String>,
    #[serde(default)]
    pub order: i32,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub result_summary: String,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct Run {
    pub id: String,
    pub session_id: String,
    pub agent_id: String,
    pub status: RunStatus,
    pub message: String,
    #[serde(default)]
    pub tool_calls: Vec<ToolCall>,
    #[serde(default)]
    pub pending_approvals: Vec<Approval>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub input_request: Option<InputRequest>,
    #[serde(default)]
    pub input_requests: Vec<InputRequest>,
    #[serde(default)]
    pub workflow_plan: Vec<WorkflowTask>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub usage: Option<RunUsage>,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub failure_reason: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub error_code: String,
    #[serde(default)]
    pub degraded: bool,
    pub created_at: WireTimestamp,
    pub updated_at: WireTimestamp,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub completed_at: Option<WireTimestamp>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub cancelled_at: Option<WireTimestamp>,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct AuditEvent {
    pub sequence: u64,
    pub id: String,
    pub kind: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub subject_id: String,
    pub detail: String,
    #[serde(default)]
    pub metadata: BTreeMap<String, Value>,
    pub created_at: WireTimestamp,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(tag = "kind", rename_all = "snake_case", deny_unknown_fields)]
pub enum StreamDelta {
    Reply { text: String },
    Reasoning { text: String },
    ToolProgress { text: String },
}

#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct ChatDelta {
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub reply: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub reasoning_content: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub tool_progress: String,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct VersionedArtifact {
    pub session_id: String,
    pub namespace: String,
    pub filename: String,
    pub version: u64,
    pub content_sha256: String,
    pub content_base64: String,
    pub created_at: WireTimestamp,
}

#[derive(Clone, Debug, Default, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct AssistantCheckpoint {
    #[serde(default)]
    pub sessions: BTreeMap<String, Session>,
    #[serde(default)]
    pub runs: BTreeMap<String, Run>,
    #[serde(default)]
    pub audit: Vec<AuditEvent>,
    #[serde(default)]
    pub artifacts: BTreeMap<String, Vec<VersionedArtifact>>,
}
