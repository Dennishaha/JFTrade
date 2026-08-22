use std::collections::BTreeMap;

use percent_encoding::percent_decode_str;
use serde_json::{Value, json};

pub const PLUGIN_INSTALL_PATH: &str = "/api/v1/plugins/{pluginId}/install";
pub const PLUGIN_UNINSTALL_PATH: &str = "/api/v1/plugins/{pluginId}/uninstall";

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum PluginWriteOperation {
    Install,
    Uninstall,
}

impl PluginWriteOperation {
    fn suffix(self) -> &'static str {
        match self {
            Self::Install => "/install",
            Self::Uninstall => "/uninstall",
        }
    }

    fn name(self) -> &'static str {
        match self {
            Self::Install => "install",
            Self::Uninstall => "uninstall",
        }
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct PluginWriteRequest {
    pub method: String,
    pub path: String,
    pub body: Option<Vec<u8>>,
}

#[allow(dead_code)]
#[derive(Clone, Debug, Eq, PartialEq)]
pub enum PluginWritePortError {
    NotFound(String),
    Internal(String),
    Unavailable(String),
}

/// Consumer-owned mutation boundary for plugin metadata.
///
/// The port returns the complete Go operation projection. It deliberately has
/// no filesystem, dynamic-library, process, event, or persistence methods;
/// those remain behind the Go owner until a separately qualified cutover.
pub trait PluginWritePort: Send + Sync {
    fn mutate(
        &self,
        operation: PluginWriteOperation,
        plugin_id: &str,
    ) -> Result<Value, PluginWritePortError>;
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct PluginWriteResponse {
    pub status: u16,
    pub headers: BTreeMap<String, String>,
    pub body: Value,
}

pub fn dispatch_plugin_write(
    request: &PluginWriteRequest,
    port: Option<&dyn PluginWritePort>,
    timestamp: &str,
) -> PluginWriteResponse {
    let Some(operation) = parse_route(&request.method, &request.path) else {
        return error_response(404, "NOT_FOUND", "resource not found", timestamp);
    };
    let Some(plugin_id) = parse_plugin_id(&request.path) else {
        return error_response(400, "BAD_REQUEST", "pluginId is invalid", timestamp);
    };
    let Some(port) = port else {
        return error_response(
            503,
            "PLUGINS_UNAVAILABLE",
            "plugin write port is unavailable",
            timestamp,
        );
    };
    let operation = match port.mutate(operation, &plugin_id) {
        Ok(operation) => operation,
        Err(error) => return port_error_response(error, operation, timestamp),
    };
    success_response(json!({"operation": operation}), timestamp)
}

fn parse_route(method: &str, path: &str) -> Option<PluginWriteOperation> {
    if method != "POST" {
        return None;
    }
    [
        (PluginWriteOperation::Install, PLUGIN_INSTALL_PATH),
        (PluginWriteOperation::Uninstall, PLUGIN_UNINSTALL_PATH),
    ]
    .into_iter()
    .find_map(|(operation, _template)| {
        let prefix = "/api/v1/plugins/";
        let suffix = operation.suffix();
        let id = path.strip_prefix(prefix)?.strip_suffix(suffix)?;
        (!id.is_empty() && !id.contains('/')).then_some(operation)
    })
}

fn parse_plugin_id(path: &str) -> Option<String> {
    let encoded = path
        .strip_prefix("/api/v1/plugins/")?
        .strip_suffix("/install")
        .or_else(|| {
            path.strip_prefix("/api/v1/plugins/")?
                .strip_suffix("/uninstall")
        })?;
    if encoded.is_empty() || encoded.contains('/') {
        return None;
    }
    let decoded = percent_decode_str(encoded).decode_utf8().ok()?;
    let plugin_id = decoded.trim();
    (!plugin_id.is_empty() && !plugin_id.contains('/')).then(|| plugin_id.to_owned())
}

fn port_error_response(
    error: PluginWritePortError,
    operation: PluginWriteOperation,
    timestamp: &str,
) -> PluginWriteResponse {
    match error {
        PluginWritePortError::NotFound(message) => {
            error_response(404, "NOT_FOUND", &message, timestamp)
        }
        PluginWritePortError::Internal(_) => error_response(
            500,
            "INTERNAL_ERROR",
            &format!("plugin {} failed", operation.name()),
            timestamp,
        ),
        PluginWritePortError::Unavailable(message) => {
            error_response(503, "PLUGINS_UNAVAILABLE", &message, timestamp)
        }
    }
}

fn success_response(data: Value, timestamp: &str) -> PluginWriteResponse {
    PluginWriteResponse {
        status: 200,
        headers: json_headers(),
        body: json!({"ok": true, "data": data, "timestamp": timestamp}),
    }
}

fn error_response(status: u16, code: &str, message: &str, timestamp: &str) -> PluginWriteResponse {
    PluginWriteResponse {
        status,
        headers: json_headers(),
        body: json!({
            "ok": false,
            "error": {"code": code, "message": message},
            "timestamp": timestamp,
        }),
    }
}

fn json_headers() -> BTreeMap<String, String> {
    BTreeMap::from([(
        "Content-Type".to_owned(),
        "application/json; charset=utf-8".to_owned(),
    )])
}
