//! Read the current builtin agent and validate its requested provider configuration.

use super::*;
use jftrade_assistant::{BuiltinAgentConfigurationError, prepare_builtin_agent_configuration};

pub(super) fn configuration_payload(
    port: &ProductionAdkPort,
    body: &Value,
) -> Result<Value, AdkMutationPortError> {
    let incoming = Value::Object(object_body(body, "agent")?);
    let current = match port
        .store
        .get_agent("jftrade-default")
        .map_err(storage_mutation_failed)?
    {
        Some(stored) => decode_mutation_payload(&stored.payload_json, "agent")?,
        None => super::super::builtin_agent(&port.tool_catalog),
    };
    let prepared =
        prepare_builtin_agent_configuration(&current, &incoming).map_err(|error| match error {
            BuiltinAgentConfigurationError::Protected => AdkMutationPortError::Failed {
                status: 409,
                code: "ADK_AGENT_PROTECTED".to_owned(),
                message: error.to_string(),
            },
            BuiltinAgentConfigurationError::InvalidPayload => {
                invalid_mutation_input(&error.to_string())
            }
            BuiltinAgentConfigurationError::InvalidCurrentPayload => storage_mutation_failed(error),
        })?;
    super::agent_validation::validate_agent_write(
        port,
        "jftrade-default",
        &prepared.validation_payload,
    )?;
    Ok(prepared.payload)
}
