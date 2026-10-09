#![forbid(unsafe_code)]

//! Provider-neutral Assistant domain and runtime contracts.
//!
//! JFTrade owns every exported model and port in this crate. Rig is confined to
//! [`rig_adapter`], so provider SDK types cannot become persisted business state.

mod artifact;
mod builtin_agent;
mod chat_identity;
mod claims;
mod model;
mod persisted_run;
mod ports;
pub mod rig_adapter;
mod runtime;
mod skill_document;
mod skill_registry;
mod stream_retention;
mod workflow;
mod workflow_canvas;

pub use artifact::{ArtifactError, ArtifactStore};
pub use builtin_agent::{
    BuiltinAgentConfiguration, BuiltinAgentConfigurationError, prepare_builtin_agent_configuration,
};
pub use chat_identity::canonical_chat_request_json;
pub use claims::{
    ClaimCheckpoint, ClaimError, ClaimStore, RunLease, ToolClaimRequest, ToolInvocation,
    ToolInvocationStatus, ToolInvocationTicket,
};
pub use model::tool_policy::{
    ALL_PERMISSION_MODES, PERMISSION_MODE_ALL, PERMISSION_MODE_APPROVAL,
    PERMISSION_MODE_LESS_APPROVAL, normalize_permission_mode, tool_allowed_in_mode,
    tool_requires_approval,
};
pub use model::{
    Approval, ApprovalStatus, AssistantCheckpoint, AuditEvent, ChatDelta, InputAnswer,
    InputDecisionKind, InputOption, InputOptionDraft, InputQuestion, InputQuestionDraft,
    InputRequest, InputRequestDraft, InputRequestStatus, Run, RunStatus, RunUsage, Session,
    StreamDelta, ToolCall, ToolCallStatus, ToolDescriptor, ToolIdempotencyMode, VersionedArtifact,
    WorkflowTask, WorkflowTaskStatus,
};
pub use persisted_run::validate_persisted_run_payload;
pub use ports::{
    CompletionInput, CompletionPort, CompletionTurn, JftradeMessage, MessageRole, ProviderFailure,
    ProviderFailureKind, ToolRequest,
};
pub use runtime::{AssistantRuntime, RuntimeError, TransitionResult};
pub use skill_document::{SkillDocumentError, SkillDocumentMetadata, parse_skill_document};
pub use skill_registry::{SkillToolValidation, compare_skill_catalog_order, validate_skill_tools};
pub use stream_retention::{DEFAULT_RUN_TIMEOUT_MS, active_run_stream_retention_expired};
pub use workflow::{TaskGraph, WorkflowError};
pub use workflow_canvas::{
    CanvasCompiler, CanvasCompilerError, WorkflowCanvasEdge, WorkflowCanvasGraph,
    WorkflowCanvasNode, WorkflowNodeRun,
};
