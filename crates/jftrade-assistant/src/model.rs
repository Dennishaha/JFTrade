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

/// Tool approval policy owned by the Assistant domain.
///
/// This is the Rust port of the reference `ToolRequiresApproval` /
/// `toolExplicitlySkipsApproval` / `mediumOrHigherRisk` trio.  It is a pure
/// function over the caller's descriptor plus the resolved agent permission
/// mode so every runtime (ADK chat, workflow tools, MCP) can share one
/// decision instead of re-deriving it at each call site.
pub mod tool_policy {
    use super::ToolDescriptor;

    pub const PERMISSION_MODE_APPROVAL: &str = "approval";
    pub const PERMISSION_MODE_LESS_APPROVAL: &str = "less_approval";
    pub const PERMISSION_MODE_ALL: &str = "all";

    /// Every supported permission mode, in the reference's declaration order.
    pub const ALL_PERMISSION_MODES: &[&str] = &[
        PERMISSION_MODE_APPROVAL,
        PERMISSION_MODE_LESS_APPROVAL,
        PERMISSION_MODE_ALL,
    ];

    /// Unknown and blank modes fall back to the approval default, like the
    /// reference `NormalizePermissionMode`.
    pub fn normalize_permission_mode(value: &str) -> &'static str {
        match value.trim().to_ascii_lowercase().as_str() {
            PERMISSION_MODE_LESS_APPROVAL => PERMISSION_MODE_LESS_APPROVAL,
            PERMISSION_MODE_ALL => PERMISSION_MODE_ALL,
            _ => PERMISSION_MODE_APPROVAL,
        }
    }

    /// Whether a resolved tool descriptor requires user approval in `mode`.
    pub fn tool_requires_approval(descriptor: &ToolDescriptor, mode: &str) -> bool {
        let mode = normalize_permission_mode(mode);
        if tool_explicitly_skips_approval(&descriptor.name) {
            return false;
        }
        if descriptor
            .requires_approval_in
            .iter()
            .any(|entry| normalize_permission_mode(entry) == mode)
        {
            return true;
        }
        if mode == PERMISSION_MODE_APPROVAL && medium_or_higher_risk(&descriptor.risk_level) {
            return true;
        }
        match descriptor.permission.trim() {
            "install_skill" | "write_strategy" | "optimize_strategy" | "write_task"
            | "write_memory" => mode == PERMISSION_MODE_APPROVAL,
            "create_strategy_instance" => mode != PERMISSION_MODE_ALL,
            "live_trading" => true,
            _ => false,
        }
    }

    /// Whether the descriptor is selectable in `mode` at all.
    pub fn tool_allowed_in_mode(descriptor: &ToolDescriptor, mode: &str) -> bool {
        let mode = normalize_permission_mode(mode);
        descriptor.allowed_modes.is_empty()
            || descriptor
                .allowed_modes
                .iter()
                .any(|entry| normalize_permission_mode(entry) == mode)
    }

    fn medium_or_higher_risk(risk: &str) -> bool {
        matches!(
            risk.trim().to_ascii_lowercase().as_str(),
            "medium" | "high" | "critical"
        )
    }

    /// Low-risk workflow writes the reference explicitly exempts from approval
    /// even when their permission class (`write_task`/`write_memory`/
    /// `write_strategy`) would otherwise gate them.
    fn tool_explicitly_skips_approval(name: &str) -> bool {
        matches!(
            name.trim(),
            "tasks.create"
                | "tasks.update"
                | "tasks.delete"
                | "memory.remember"
                | "memory.forget"
                | "strategy.save_draft"
                | "strategy.research_backtest"
        )
    }
}

#[cfg(test)]
mod tool_policy_tests {
    use super::tool_policy::{
        ALL_PERMISSION_MODES, PERMISSION_MODE_ALL, PERMISSION_MODE_APPROVAL,
        PERMISSION_MODE_LESS_APPROVAL, normalize_permission_mode, tool_allowed_in_mode,
        tool_requires_approval,
    };
    use super::{ToolDescriptor, ToolIdempotencyMode};
    use serde_json::json;

    fn descriptor(name: &str, permission: &str, risk: &str) -> ToolDescriptor {
        ToolDescriptor {
            name: name.to_owned(),
            display_name: name.to_owned(),
            description: name.to_owned(),
            category: "workflow".to_owned(),
            permission: permission.to_owned(),
            risk_level: risk.to_owned(),
            idempotency_mode: ToolIdempotencyMode::ReplaySafe,
            allowed_modes: ALL_PERMISSION_MODES
                .iter()
                .map(|mode| (*mode).to_owned())
                .collect(),
            requires_approval_in: Vec::new(),
            input_schema: json!({"type": "object"}),
        }
    }

    /// Parity: go:452dea11:internal/assistant/engine/store_ops_test.go:998
    /// TestWorkflowWriteToolsRequireApprovalExceptLowRiskTaskWrites.
    ///
    /// The reference exempts low-risk workflow writes from approval even in
    /// `approval` mode: `tasks.create/update/delete`, `memory.remember/forget`
    /// and `strategy.save_draft` all execute without asking the user.
    #[test]
    fn low_risk_workflow_writes_skip_approval_in_approval_mode() {
        for name in [
            "tasks.create",
            "tasks.update",
            "tasks.delete",
            "memory.remember",
            "memory.forget",
            "strategy.save_draft",
            "strategy.research_backtest",
        ] {
            for permission in ["write_task", "write_memory", "write_strategy"] {
                let descriptor = descriptor(name, permission, "low");
                assert!(
                    !tool_requires_approval(&descriptor, PERMISSION_MODE_APPROVAL),
                    "{name} must execute without approval in approval mode"
                );
            }
        }
    }

    /// Parity: go:452dea11:internal/assistant/engine/tools_test.go:129
    /// TestApprovalModeRequiresMediumAndHigherRiskApproval.
    #[test]
    fn medium_and_higher_risk_require_approval_only_in_approval_mode() {
        for risk in ["medium", "high", "critical"] {
            let descriptor = descriptor(&format!("risk.{risk}"), "write_external", risk);
            assert!(
                tool_requires_approval(&descriptor, PERMISSION_MODE_APPROVAL),
                "risk {risk} must require approval in approval mode"
            );
            assert!(
                !tool_requires_approval(&descriptor, PERMISSION_MODE_ALL),
                "risk {risk} must not require approval in all mode"
            );
        }
        let low = descriptor("risk.low", "write_external", "low");
        assert!(!tool_requires_approval(&low, PERMISSION_MODE_APPROVAL));
        // Risk comparison trims and folds case like the reference helper.
        let padded = descriptor("risk.padded", "write_external", " HIGH ");
        assert!(tool_requires_approval(&padded, PERMISSION_MODE_APPROVAL));
    }

    /// Parity: go:452dea11:internal/assistant/engine/tools_test.go:16
    /// TestApprovalRequiresLiveTradingAndStrategyInstanceAlways.
    ///
    /// `live_trading` is gated in every mode, `create_strategy_instance` in
    /// every mode except `all`, and an explicit `requiresApprovalIn` entry wins
    /// over the permission-class default.  `strategy.research_backtest` keeps
    /// its explicit skip even though it declares `optimize_strategy`.
    #[test]
    fn permission_classes_keep_their_mode_specific_gates() {
        let live = descriptor("trade.place_order", "live_trading", "critical");
        for mode in ALL_PERMISSION_MODES {
            assert!(
                tool_requires_approval(&live, mode),
                "live_trading is gated in {mode}"
            );
        }

        let instance = descriptor(
            "strategy.create_instance",
            "create_strategy_instance",
            "medium",
        );
        assert!(tool_requires_approval(&instance, PERMISSION_MODE_APPROVAL));
        assert!(tool_requires_approval(
            &instance,
            PERMISSION_MODE_LESS_APPROVAL
        ));
        assert!(!tool_requires_approval(&instance, PERMISSION_MODE_ALL));

        for permission in [
            "install_skill",
            "write_strategy",
            "optimize_strategy",
            "write_task",
            "write_memory",
        ] {
            let descriptor = descriptor(&format!("gated.{permission}"), permission, "low");
            assert!(tool_requires_approval(
                &descriptor,
                PERMISSION_MODE_APPROVAL
            ));
            assert!(!tool_requires_approval(
                &descriptor,
                PERMISSION_MODE_LESS_APPROVAL
            ));
            assert!(!tool_requires_approval(&descriptor, PERMISSION_MODE_ALL));
        }

        let mut explicit = descriptor("strategy.optimize", "optimize_strategy", "low");
        explicit.requires_approval_in = vec![PERMISSION_MODE_APPROVAL.to_owned()];
        assert!(tool_requires_approval(&explicit, PERMISSION_MODE_APPROVAL));

        let mut research = descriptor("strategy.research_backtest", "optimize_strategy", "low");
        research.requires_approval_in = vec![PERMISSION_MODE_APPROVAL.to_owned()];
        for mode in ALL_PERMISSION_MODES {
            assert!(
                !tool_requires_approval(&research, mode),
                "research_backtest explicitly skips approval in {mode}"
            );
            assert!(tool_allowed_in_mode(&research, mode));
        }
    }

    /// Parity: go:452dea11:internal/assistant/engine/tools_test.go:87
    /// TestTaskWriteToolsMarkedLowRiskCanSkipApproval.
    ///
    /// The `tasks.*` family is `write_task` but carries the low risk level, so
    /// the reference lets it run without a confirmation in `approval` mode.
    #[test]
    fn task_write_tools_are_low_risk_and_skip_approval() {
        for name in ["tasks.create", "tasks.update", "tasks.delete"] {
            let task_write = descriptor(name, "write_task", "low");
            assert!(
                !tool_requires_approval(&task_write, PERMISSION_MODE_APPROVAL),
                "{name} is a low-risk task write and must skip approval"
            );
            // The exemption is name-based, so the same permission class
            // without the exempt name is still gated.
            let sibling = descriptor(&format!("{name}.other"), "write_task", "low");
            assert!(
                tool_requires_approval(&sibling, PERMISSION_MODE_APPROVAL),
                "an exempt name must not exempt the whole write_task class"
            );
        }
    }

    /// Parity: go:452dea11:internal/assistant/engine/tools_test.go:106
    /// TestLowRiskWriteToolsCanSkipApproval.
    ///
    /// `memory.remember` / `memory.forget` (`write_memory`) and
    /// `strategy.save_draft` (`write_strategy`) are the remaining low-risk
    /// writes the reference executes without asking the user.
    #[test]
    fn memory_and_draft_writes_skip_approval_in_approval_mode() {
        for (name, permission) in [
            ("memory.remember", "write_memory"),
            ("memory.forget", "write_memory"),
            ("strategy.save_draft", "write_strategy"),
        ] {
            let descriptor = descriptor(name, permission, "low");
            assert!(
                !tool_requires_approval(&descriptor, PERMISSION_MODE_APPROVAL),
                "{name} must execute without approval"
            );
            // The same descriptor still has to satisfy the mode gate in
            // `less_approval`/`all`, where it also runs unconfirmed.
            for mode in [PERMISSION_MODE_LESS_APPROVAL, PERMISSION_MODE_ALL] {
                assert!(
                    !tool_requires_approval(&descriptor, mode),
                    "{name} runs unconfirmed in {mode}"
                );
            }
        }
    }

    /// Parity: go:452dea11:internal/assistant/engine/tools_test.go:700
    /// TestLiveTradingToolsAreAvailableInAllModesWithApproval.
    ///
    /// A `live_trading` tool stays selectable in every permission mode but is
    /// never released from confirmation, including `all`.
    #[test]
    fn live_trading_tools_stay_available_and_gated_in_every_mode() {
        let mut live = descriptor("orders.place", "live_trading", "critical");
        live.allowed_modes = ALL_PERMISSION_MODES
            .iter()
            .map(|mode| (*mode).to_owned())
            .collect();
        for mode in ALL_PERMISSION_MODES {
            assert!(
                tool_allowed_in_mode(&live, mode),
                "a live trading tool stays selectable in {mode}"
            );
            assert!(
                tool_requires_approval(&live, mode),
                "a live trading tool is confirmed in every mode, including {mode}"
            );
        }
    }

    /// Unknown or blank modes normalize to the approval default, so a
    /// malformed agent payload is gated rather than silently unrestricted.
    #[test]
    fn unknown_permission_modes_normalize_to_approval() {
        for value in ["", "  ", "APPROVAL", "less-approval", "bogus"] {
            assert_eq!(
                normalize_permission_mode(value),
                PERMISSION_MODE_APPROVAL,
                "{value:?} must fall back to the approval default"
            );
        }
        assert_eq!(
            normalize_permission_mode(" Less_Approval "),
            PERMISSION_MODE_LESS_APPROVAL
        );
        assert_eq!(normalize_permission_mode("ALL"), PERMISSION_MODE_ALL);
        let write = descriptor("gated.write_task", "write_task", "low");
        assert!(
            tool_requires_approval(&write, "bogus"),
            "an unknown mode must fall back to approval gating"
        );
    }
}
