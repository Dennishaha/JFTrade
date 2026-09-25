//! Agent write validation for the production ADK mutation port.
//!
//! The Go Assistant service validates an agent write before it reaches the
//! store: the status/work-mode/tool-access vocabularies, the loop iteration
//! budget, the referenced provider lifecycle, and tool/skill catalogue
//! membership. The HTTP edge classifies those failures as `400 BAD_REQUEST`
//! carrying the service message, so the text below is public contract rather
//! than an internal diagnostic.
//!
//! The primary builtin agent is the documented exception: its composed
//! template intentionally references product tools that a smaller catalogue
//! may not register, so Go skips the membership check only while the supplied
//! tool list equals the builtin tool set.

use super::super::{ProductionAdkPort, builtin_skills};
use super::*;

/// Go `assistantmodel.MaxLoopIterations`.
const AGENT_MAX_LOOP_ITERATIONS: u64 = 20;
/// Go `assistantmodel.DefaultBuiltinAgentID`.
const DEFAULT_BUILTIN_AGENT_ID: &str = "jftrade-default";

/// Validate one agent create/update payload against the Go service contract.
pub(super) fn validate_agent_write(
    port: &ProductionAdkPort,
    agent_id: &str,
    payload: &Value,
) -> Result<(), AdkMutationPortError> {
    let status = payload
        .get("status")
        .and_then(Value::as_str)
        .map(str::trim)
        .map(str::to_ascii_uppercase)
        .unwrap_or_default();
    if !matches!(status.as_str(), "" | "ENABLED" | "DISABLED") {
        return Err(invalid_mutation_input("invalid agent status"));
    }
    if let Some(work_mode) = payload
        .get("workMode")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        && !matches!(work_mode.to_ascii_lowercase().as_str(), "chat" | "loop")
    {
        return Err(invalid_mutation_input("invalid agent work mode"));
    }
    if let Some(mode) = payload
        .get("toolAccessMode")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        && !matches!(
            mode.to_ascii_lowercase().as_str(),
            "all" | "selected" | "none"
        )
    {
        return Err(invalid_mutation_input("invalid tool access mode"));
    }
    validate_loop_iterations(payload)?;
    validate_agent_provider(port, &status, payload)?;
    validate_agent_tools(port, agent_id, payload)?;
    validate_agent_skills(port, payload)
}

fn loop_iterations_message() -> String {
    format!("loop max iterations must be between 1 and {AGENT_MAX_LOOP_ITERATIONS}")
}

/// The Go write DTO decodes `loopMaxIterations` as an `int`, so JSON null keeps
/// the zero value while a non-integer is a binding failure rather than a range
/// failure. Reproduce both outcomes instead of collapsing them into one.
fn validate_loop_iterations(payload: &Value) -> Result<(), AdkMutationPortError> {
    let Some(iterations) = payload.get("loopMaxIterations") else {
        return Ok(());
    };
    match iterations {
        Value::Null => Ok(()),
        Value::Number(number) => {
            let Some(iterations) = number.as_i64() else {
                return Err(invalid_mutation_input("invalid agent payload"));
            };
            if iterations < 0 || iterations > AGENT_MAX_LOOP_ITERATIONS as i64 {
                return Err(invalid_mutation_input(&loop_iterations_message()));
            }
            Ok(())
        }
        _ => Err(invalid_mutation_input("invalid agent payload")),
    }
}

/// Resolve the referenced provider and, unless the agent is disabled, require
/// an enabled provider that has a usable API key.
fn validate_agent_provider(
    port: &ProductionAdkPort,
    status: &str,
    payload: &Value,
) -> Result<(), AdkMutationPortError> {
    let Some(provider_id) = payload
        .get("providerId")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
    else {
        return Ok(());
    };
    let Some(provider) = port
        .store
        .get_provider(provider_id)
        .map_err(storage_mutation_failed)?
    else {
        return Err(invalid_mutation_input("provider not found"));
    };
    if status == "DISABLED" {
        return Ok(());
    }
    let stored = decode_mutation_payload(&provider.payload_json, "provider")?;
    if !stored
        .get("enabled")
        .and_then(Value::as_bool)
        .unwrap_or(true)
    {
        return Err(invalid_mutation_input("provider is disabled"));
    }
    // The ADK secrets file is the provider-owned API-key store, so an entry is
    // present exactly when the provider has a usable key. Go performs the same
    // lookup through `ProviderAPIKey` inside `validateAgent`.
    let secrets = read_adk_secrets(&port.settings_path)?;
    if !secrets.get(provider_id).is_some_and(|key| !key.trim().is_empty()) {
        return Err(invalid_mutation_input(
            "provider API keys is not configured",
        ));
    }
    if let Some(effort) = payload
        .get("reasoningEffort")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
    {
        let mut normalized_provider = stored.clone();
        super::super::projection::normalize_provider_reasoning_config(&mut normalized_provider);
        let config = normalized_provider
            .get("reasoningConfig")
            .unwrap_or(&Value::Null);
        super::super::projection::validate_provider_reasoning_config(config)
            .map_err(|error| invalid_mutation_input(&error))?;
        let normalized_effort = effort.to_ascii_lowercase();
        let supported = config
            .get("mappings")
            .and_then(Value::as_array)
            .is_some_and(|mappings| {
                mappings.iter().any(|mapping| {
                    mapping
                        .get("effort")
                        .and_then(Value::as_str)
                        .is_some_and(|candidate| {
                            candidate.trim().eq_ignore_ascii_case(&normalized_effort)
                        })
                })
            });
        if !supported {
            return Err(invalid_mutation_input(&format!(
                "provider does not support reasoning effort: {normalized_effort}"
            )));
        }
    }
    Ok(())
}

fn validate_agent_tools(
    port: &ProductionAdkPort,
    agent_id: &str,
    payload: &Value,
) -> Result<(), AdkMutationPortError> {
    let Some(tools) = payload.get("tools").and_then(Value::as_array) else {
        return Ok(());
    };
    let known = port.tool_catalog.ids();
    let builtin_default = agent_id == DEFAULT_BUILTIN_AGENT_ID && same_tool_set(tools, &known);
    for tool in tools {
        let Some(name) = tool.as_str().map(str::trim).filter(|value| !value.is_empty()) else {
            continue;
        };
        if builtin_default || known.iter().any(|known| known == name) {
            continue;
        }
        return Err(invalid_mutation_input(&format!("unknown ADK tool: {name}")));
    }
    Ok(())
}

fn validate_agent_skills(
    port: &ProductionAdkPort,
    payload: &Value,
) -> Result<(), AdkMutationPortError> {
    let Some(skills) = payload.get("skills").and_then(Value::as_array) else {
        return Ok(());
    };
    // Builtin skills are projected by the read owner when the store holds no
    // rows, so membership must consult the projection rather than the table.
    let mut known = port
        .store
        .list_skills()
        .map_err(storage_mutation_failed)?
        .into_iter()
        .map(|row| row.id)
        .collect::<Vec<_>>();
    known.extend(
        builtin_skills(&port.tool_catalog)
            .into_iter()
            .filter_map(|skill| skill.get("id").and_then(Value::as_str).map(str::to_owned)),
    );
    for skill in skills {
        let Some(id) = skill.as_str().map(str::trim).filter(|value| !value.is_empty()) else {
            continue;
        };
        if known.iter().any(|known| known == id) {
            continue;
        }
        return Err(invalid_mutation_input(&format!("unknown ADK skill: {id}")));
    }
    Ok(())
}

/// Go `isDefaultBuiltinTool`: the composed default template keeps its
/// membership bypass only while the supplied list is exactly the builtin set.
fn same_tool_set(tools: &[Value], builtin: &[String]) -> bool {
    let supplied = tools
        .iter()
        .filter_map(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .collect::<Vec<_>>();
    supplied.len() == builtin.len()
        && builtin
            .iter()
            .all(|name| supplied.iter().any(|candidate| candidate == name))
}
