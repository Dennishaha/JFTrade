use std::future::Future;

use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::{RunUsage, StreamDelta, ToolDescriptor};

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum MessageRole {
    System,
    User,
    Assistant,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct JftradeMessage {
    pub role: MessageRole,
    pub content: String,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct CompletionInput {
    pub provider_id: String,
    pub model: String,
    pub messages: Vec<JftradeMessage>,
    #[serde(default)]
    pub tools: Vec<ToolDescriptor>,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct ToolRequest {
    pub id: String,
    pub name: String,
    pub arguments: Value,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct CompletionTurn {
    #[serde(default)]
    pub deltas: Vec<StreamDelta>,
    #[serde(default)]
    pub tool_requests: Vec<ToolRequest>,
    pub usage: RunUsage,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub provider_response_id: Option<String>,
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum ProviderFailureKind {
    Transient,
    Permanent,
    Cancelled,
    TimedOut,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct ProviderFailure {
    pub kind: ProviderFailureKind,
    pub code: String,
    pub message: String,
}

pub trait CompletionPort {
    fn complete(
        &mut self,
        input: CompletionInput,
    ) -> impl Future<Output = Result<CompletionTurn, ProviderFailure>> + Send;
}
