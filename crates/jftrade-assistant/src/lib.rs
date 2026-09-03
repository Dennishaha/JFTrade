#![forbid(unsafe_code)]

//! Provider-neutral Assistant domain and runtime contracts.
//!
//! JFTrade owns every exported model and port in this crate. Rig is confined to
//! [`rig_adapter`], so provider SDK types cannot become persisted business state.

mod artifact;
mod claims;
mod model;
mod ports;
pub mod rig_adapter;
mod runtime;
mod workflow;

pub use artifact::{ArtifactError, ArtifactStore};
pub use claims::{
    ClaimCheckpoint, ClaimError, ClaimStore, RunLease, ToolClaimRequest, ToolInvocation,
    ToolInvocationStatus, ToolInvocationTicket,
};
pub use model::{
    Approval, ApprovalStatus, AssistantCheckpoint, AuditEvent, ChatDelta, InputAnswer,
    InputDecisionKind, InputOption, InputOptionDraft, InputQuestion, InputQuestionDraft,
    InputRequest, InputRequestDraft, InputRequestStatus, Run, RunStatus, RunUsage, Session,
    StreamDelta, ToolCall, ToolCallStatus, ToolDescriptor, ToolIdempotencyMode, VersionedArtifact,
    WorkflowTask, WorkflowTaskStatus,
};
pub use ports::{
    CompletionInput, CompletionPort, CompletionTurn, JftradeMessage, MessageRole, ProviderFailure,
    ProviderFailureKind, ToolRequest,
};
pub use runtime::{AssistantRuntime, RuntimeError, TransitionResult};
pub use workflow::{TaskGraph, WorkflowError};
