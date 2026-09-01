//! Native MCP leaves for Pine specification and validation.
//!
//! These leaves cover the reviewed native Pine subset only. They do not start
//! or call the PineTS worker; full Pine runtime execution remains a separate
//! production capability.

use jftrade_strategy::pinespec::{build_tool_payload, validate_script};
use serde_json::{Value, json};

pub const PINE_SPEC_TOOL: &str = "strategy.pine_spec";
pub const VALIDATE_PINE_TOOL: &str = "strategy.validate_pine";

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct StrategyPineMcpFailure {
    pub status: u16,
    pub code: String,
    pub message: String,
}

impl StrategyPineMcpFailure {
    fn invalid(message: impl Into<String>) -> Self {
        Self {
            status: 400,
            code: "BAD_REQUEST".to_owned(),
            message: message.into(),
        }
    }
    fn unavailable(message: impl Into<String>) -> Self {
        Self {
            status: 503,
            code: "MCP_TOOL_UNAVAILABLE".to_owned(),
            message: message.into(),
        }
    }
    #[allow(dead_code)]
    pub fn envelope(&self) -> Value {
        json!({"ok": false, "error": {"code": self.code, "message": self.message}, "status": self.status})
    }
}

/// Execute one of the two native Pine MCP leaves without touching stores or
/// external workers. The production MCP executor registers this dispatch
/// alongside the reviewed read-only tool catalog.
pub fn dispatch_strategy_pine_mcp(
    name: &str,
    arguments: &Value,
) -> Result<Value, StrategyPineMcpFailure> {
    match name {
        PINE_SPEC_TOOL => dispatch_spec(arguments),
        VALIDATE_PINE_TOOL => dispatch_validate(arguments),
        _ => Err(StrategyPineMcpFailure::unavailable(format!(
            "native Pine MCP leaf is not implemented for {name}"
        ))),
    }
}

fn dispatch_spec(arguments: &Value) -> Result<Value, StrategyPineMcpFailure> {
    let object = arguments
        .as_object()
        .ok_or_else(|| StrategyPineMcpFailure::invalid("tool arguments must be an object"))?;
    let section = optional_string(object.get("section"), "section")?.unwrap_or_default();
    let include_examples =
        optional_bool(object.get("includeExamples"), "includeExamples")?.unwrap_or(false);
    build_tool_payload(&section, include_examples)
        .map_err(|error| StrategyPineMcpFailure::invalid(error.to_string()))
}

fn dispatch_validate(arguments: &Value) -> Result<Value, StrategyPineMcpFailure> {
    let object = arguments
        .as_object()
        .ok_or_else(|| StrategyPineMcpFailure::invalid("tool arguments must be an object"))?;
    let script = object
        .get("script")
        .ok_or_else(|| StrategyPineMcpFailure::invalid("script is required"))?
        .as_str()
        .ok_or_else(|| StrategyPineMcpFailure::invalid("script must be a string"))?;
    let include_requirements =
        optional_bool(object.get("includeRequirements"), "includeRequirements")?.unwrap_or(true);
    serde_json::to_value(validate_script(script, include_requirements, false)).map_err(|error| {
        StrategyPineMcpFailure {
            status: 500,
            code: "MCP_PINE_SERIALIZATION_FAILED".to_owned(),
            message: error.to_string(),
        }
    })
}

fn optional_string(
    value: Option<&Value>,
    key: &str,
) -> Result<Option<String>, StrategyPineMcpFailure> {
    match value {
        None | Some(Value::Null) => Ok(None),
        Some(Value::String(value)) => Ok(Some(value.clone())),
        Some(_) => Err(StrategyPineMcpFailure::invalid(format!(
            "{key} must be a string"
        ))),
    }
}
fn optional_bool(value: Option<&Value>, key: &str) -> Result<Option<bool>, StrategyPineMcpFailure> {
    match value {
        None | Some(Value::Null) => Ok(None),
        Some(Value::Bool(value)) => Ok(Some(*value)),
        Some(_) => Err(StrategyPineMcpFailure::invalid(format!(
            "{key} must be a boolean"
        ))),
    }
}
