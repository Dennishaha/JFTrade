//! The only module allowed to import Rig types.

use rig_core::completion::{
    AssistantContent, CompletionRequest, CompletionResponse, Message, ToolDefinition,
};
use rig_core::message::UserContent;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use thiserror::Error;

use crate::{
    CompletionInput, CompletionTurn, JftradeMessage, MessageRole, RunUsage, StreamDelta,
    ToolDescriptor, ToolRequest,
};

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct RigToolProjection {
    pub name: String,
    pub description: String,
    pub parameters: Value,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct RigRequestProjection {
    pub model: String,
    pub messages: Value,
    pub tools: Vec<RigToolProjection>,
    pub record_telemetry_content: bool,
}

#[derive(Debug, Error)]
pub enum RigAdapterError {
    #[error("completion request must contain at least one non-empty message")]
    EmptyMessages,
    #[error("tool {0} has an invalid object schema")]
    InvalidToolSchema(String),
    #[error("Rig rejected the completion request: {0}")]
    InvalidRigRequest(String),
    #[error("Rig request projection failed: {0}")]
    Projection(String),
    #[error("Rig response decoding failed: {0}")]
    InvalidRigResponse(String),
    #[error("Rig response contains an unsupported image block")]
    UnsupportedResponseImage,
}

pub fn project_completion_request(
    input: &CompletionInput,
) -> Result<RigRequestProjection, RigAdapterError> {
    if input.messages.is_empty()
        || input
            .messages
            .iter()
            .any(|message| message.content.trim().is_empty())
    {
        return Err(RigAdapterError::EmptyMessages);
    }
    let messages = input
        .messages
        .iter()
        .map(to_rig_message)
        .collect::<Vec<_>>();
    let tools = input
        .tools
        .iter()
        .map(to_rig_tool)
        .collect::<Result<Vec<_>, _>>()?;
    let request = CompletionRequest {
        model: Some(input.model.clone()),
        preamble: None,
        chat_history: messages,
        documents: Vec::new(),
        tools: tools.clone(),
        temperature: None,
        max_tokens: None,
        tool_choice: None,
        additional_params: None,
        output_schema: None,
        record_telemetry_content: false,
    };
    request
        .validate_message_content()
        .map_err(|error| RigAdapterError::InvalidRigRequest(error.to_string()))?;
    Ok(RigRequestProjection {
        model: input.model.clone(),
        messages: serde_json::to_value(&request.chat_history)
            .map_err(|error| RigAdapterError::Projection(error.to_string()))?,
        tools: tools
            .into_iter()
            .map(|tool| RigToolProjection {
                name: tool.name,
                description: tool.description,
                parameters: tool.parameters,
            })
            .collect(),
        record_telemetry_content: request.record_telemetry_content,
    })
}

/// Decodes Rig's provider-neutral response without exposing a Rig type to the
/// rest of JFTrade. Provider adapters may serialize the normalized response at
/// this seam; persisted state remains entirely JFTrade-owned.
pub fn project_completion_response_json(bytes: &[u8]) -> Result<CompletionTurn, RigAdapterError> {
    let response: CompletionResponse = serde_json::from_slice(bytes)
        .map_err(|error| RigAdapterError::InvalidRigResponse(error.to_string()))?;
    let mut deltas = Vec::new();
    let mut tool_requests = Vec::new();
    for content in response.choice {
        match content {
            AssistantContent::Text(text) => {
                deltas.push(StreamDelta::Reply { text: text.text });
            }
            AssistantContent::Reasoning(reasoning) => {
                let text = reasoning.display_text();
                if !text.is_empty() {
                    deltas.push(StreamDelta::Reasoning { text });
                }
            }
            AssistantContent::ToolCall(call) => {
                tool_requests.push(ToolRequest {
                    id: call.id.into_string(),
                    name: call.function.name,
                    arguments: call.function.arguments,
                });
            }
            AssistantContent::Image(_) => return Err(RigAdapterError::UnsupportedResponseImage),
        }
    }
    let tool_calls_total = u64::try_from(tool_requests.len()).unwrap_or(u64::MAX);
    Ok(CompletionTurn {
        deltas,
        tool_requests,
        usage: RunUsage {
            model_calls: 1,
            tool_calls_total,
            duration_ms: 0,
            tokens_in: response.usage.input_tokens,
            tokens_out: response.usage.output_tokens,
        },
        provider_response_id: response.response_id.or(response.message_id),
    })
}

fn to_rig_message(message: &JftradeMessage) -> Message {
    match message.role {
        MessageRole::System => Message::System {
            content: message.content.clone(),
        },
        MessageRole::User => Message::User {
            content: vec![UserContent::Text(message.content.clone().into())],
        },
        MessageRole::Assistant => Message::Assistant {
            id: None,
            content: vec![AssistantContent::Text(message.content.clone().into())],
        },
    }
}

fn to_rig_tool(descriptor: &ToolDescriptor) -> Result<ToolDefinition, RigAdapterError> {
    let is_object = descriptor
        .input_schema
        .get("type")
        .and_then(Value::as_str)
        .is_some_and(|kind| kind == "object");
    if descriptor.name.trim().is_empty() || !is_object {
        return Err(RigAdapterError::InvalidToolSchema(descriptor.name.clone()));
    }
    Ok(ToolDefinition {
        name: descriptor.name.clone(),
        description: descriptor.description.clone(),
        parameters: descriptor.input_schema.clone(),
    })
}

#[cfg(test)]
mod tests {
    use rig_core::completion::Usage;
    use rig_core::message::{Reasoning, ToolCall, ToolCallId, ToolFunction};
    use serde_json::json;

    use super::*;
    use crate::ToolIdempotencyMode;

    #[test]
    fn projection_preserves_owned_tool_schema_and_disables_content_telemetry() {
        let input = CompletionInput {
            provider_id: "fake".to_owned(),
            model: "fixed".to_owned(),
            messages: vec![JftradeMessage {
                role: MessageRole::User,
                content: "hello".to_owned(),
            }],
            tools: vec![ToolDescriptor {
                name: "interaction.request_user".to_owned(),
                display_name: "Ask".to_owned(),
                description: "Ask a blocking question".to_owned(),
                category: "interaction".to_owned(),
                permission: "read_internal".to_owned(),
                risk_level: "low".to_owned(),
                idempotency_mode: ToolIdempotencyMode::ReplaySafe,
                allowed_modes: vec!["chat".to_owned()],
                requires_approval_in: Vec::new(),
                input_schema: json!({"type": "object", "required": ["questions"]}),
            }],
        };
        let projection = project_completion_request(&input).expect("projection");
        assert_eq!(projection.tools[0].parameters, input.tools[0].input_schema);
        assert!(!projection.record_telemetry_content);
    }

    #[test]
    fn response_projection_returns_only_jftrade_owned_types() {
        let response = CompletionResponse::new(
            vec![
                AssistantContent::Reasoning(Reasoning::new("checking")),
                AssistantContent::Text("done".into()),
                AssistantContent::ToolCall(ToolCall::new(
                    ToolCallId::new("call-1").expect("id"),
                    ToolFunction::new("marketdata.quote".to_owned(), json!({"symbol": "AAPL"})),
                )),
            ],
            Usage {
                input_tokens: 10,
                output_tokens: 4,
                total_tokens: 14,
                ..Usage::new()
            },
            "fake",
        )
        .with_response_id("response-1");
        let bytes = serde_json::to_vec(&response).expect("serialize Rig response");
        let projected = project_completion_response_json(&bytes).expect("project response");
        assert_eq!(projected.usage.tokens_in, 10);
        assert_eq!(projected.usage.tokens_out, 4);
        assert_eq!(
            projected.provider_response_id.as_deref(),
            Some("response-1")
        );
        assert_eq!(projected.tool_requests[0].name, "marketdata.quote");
        assert_eq!(projected.deltas.len(), 2);
    }
}
