//! File-backed production plugin adapter and lifecycle operations.

use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Mutex;
use std::time::{SystemTime, UNIX_EPOCH};

use serde_json::{json, Value};

use crate::product::product_plugins_write_port::{
    PluginWriteOperation, PluginWritePort, PluginWritePortError,
};
use crate::product::{
    PluginSnapshotError, PluginSnapshotPort, PluginUninstallGuidance,
    PluginUninstallGuidanceSnapshotError, PluginUninstallGuidanceSnapshotPort,
};

use super::provider_now_rfc3339;

#[derive(Debug)]
pub(crate) struct ProductionPluginPort {
    root: PathBuf,
    operation_lock: Mutex<()>,
}

static PLUGIN_OPERATION_SEQUENCE: AtomicU64 = AtomicU64::new(1);
static PLUGIN_MARKER_TEMP_SEQUENCE: AtomicU64 = AtomicU64::new(1);

impl ProductionPluginPort {
    pub(super) fn open(settings_path: &std::path::Path) -> Result<Self, String> {
        let root = settings_path
            .parent()
            .filter(|parent| !parent.as_os_str().is_empty())
            .map_or_else(|| PathBuf::from("plugins"), |parent| parent.join("plugins"));
        std::fs::create_dir_all(&root).map_err(|error| {
            format!(
                "create production plugin directory {}: {error}",
                root.display()
            )
        })?;
        Ok(Self {
            root,
            operation_lock: Mutex::new(()),
        })
    }

    fn plugin_id_from_path(path: &std::path::Path) -> Option<String> {
        path.file_stem()
            .and_then(|stem| stem.to_str())
            .map(str::trim)
            .filter(|id| !id.is_empty())
            .map(str::to_owned)
    }

    fn read_catalog(&self) -> Result<Value, PluginSnapshotError> {
        let mut plugins = Vec::new();
        let entries = match std::fs::read_dir(&self.root) {
            Ok(entries) => entries,
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
                return Err(PluginSnapshotError::Unavailable(format!(
                    "production plugin directory {} disappeared after startup",
                    self.root.display()
                )));
            }
            Err(error) => {
                return Err(PluginSnapshotError::Unavailable(format!(
                    "read plugin directory {}: {error}",
                    self.root.display()
                )))
            }
        };
        for entry in entries {
            let entry = entry.map_err(|error| {
                PluginSnapshotError::Unavailable(format!("read plugin directory entry: {error}"))
            })?;
            let path = entry.path();
            if path.extension().and_then(|ext| ext.to_str()) != Some("json") {
                continue;
            }
            let Some(id) = Self::plugin_id_from_path(&path) else {
                continue;
            };
            let marker = std::fs::read(&path).map_err(|error| {
                PluginSnapshotError::Unavailable(format!("read plugin marker {}: {error}", path.display()))
            })?;
            let marker: Value = serde_json::from_slice(&marker).map_err(|error| {
                PluginSnapshotError::Unavailable(format!("decode plugin marker {}: {error}", path.display()))
            })?;
            let descriptor = marker.get("descriptor").cloned().ok_or_else(|| {
                PluginSnapshotError::Unavailable(format!(
                    "plugin marker {} is missing descriptor",
                    path.display()
                ))
            })?;
            if !descriptor.is_object() {
                return Err(PluginSnapshotError::Unavailable(format!(
                    "plugin marker {} descriptor must be an object",
                    path.display()
                )));
            }
            let install_path = self.root.join(format!("{id}.so"));
            let installation = marker
                .get("installation")
                .and_then(Value::as_object);
            // The artifact on disk is the source of truth.  A stale marker
            // must not report a plugin as installed after the file was
            // removed by an operator or a failed upgrade.
            let installed = install_path.is_file();
            let status = installation
                .and_then(|value| value.get("status"))
                .and_then(Value::as_str)
                .filter(|value| !value.trim().is_empty())
                .unwrap_or(if installed { "INSTALLED" } else { "NOT_INSTALLED" });
            plugins.push(json!({
                "descriptor": descriptor,
                "installation": {
                    "targetDir": self.root,
                    "installPath": install_path,
                    "markerPath": path,
                    "installed": installed,
                    "status": status,
                    "currentOperation": installation.and_then(|value| value.get("currentOperation")).cloned().unwrap_or(Value::Null),
                    "lastOperation": installation.and_then(|value| value.get("lastOperation")).cloned().unwrap_or(Value::Null),
                    "uninstallGuidance": plugin_guidance_value(&id, &install_path),
                },
                "compatibility": {"mode": "plugin", "supported": true, "requiresRebuild": false, "host": {"goos": std::env::consts::OS, "goarch": std::env::consts::ARCH}},
            }));
        }
        plugins.sort_by(|left, right| left["descriptor"]["id"].as_str().cmp(&right["descriptor"]["id"].as_str()));
        Ok(json!({"plugins": plugins, "targetDir": self.root}))
    }
}

fn plugin_guidance_value(id: &str, path: &std::path::Path) -> Value {
    let display = path.to_string_lossy();
    let posix = display.replace('\'', "'\\''");
    json!({
        "pluginId": id,
        "path": display,
        "exists": path.is_file(),
        "commands": {
            "posix": format!("rm -f '{posix}'"),
            "powershell": format!("Remove-Item -LiteralPath '{}' -Force", display.replace('\'', "''")),
        }
    })
}

impl PluginSnapshotPort for ProductionPluginPort {
    fn catalog(&self) -> Result<Value, PluginSnapshotError> {
        self.read_catalog()
    }

    fn operation(&self, operation_id: &str) -> Result<Option<Value>, PluginSnapshotError> {
        let entries = std::fs::read_dir(&self.root).map_err(|error| {
            PluginSnapshotError::Unavailable(format!(
                "read plugin directory {}: {error}",
                self.root.display()
            ))
        })?;
        for entry in entries {
            let entry = entry.map_err(|error| {
                PluginSnapshotError::Unavailable(format!("read plugin directory entry: {error}"))
            })?;
            let path = entry.path();
            if path.extension().and_then(|extension| extension.to_str()) != Some("json") {
                continue;
            }
            let marker = std::fs::read(&path).map_err(|error| {
                PluginSnapshotError::Unavailable(format!("read plugin marker {}: {error}", path.display()))
            })?;
            let marker: Value = serde_json::from_slice(&marker).map_err(|error| {
                PluginSnapshotError::Unavailable(format!("decode plugin marker {}: {error}", path.display()))
            })?;
            let Some(operations) = marker.get("operations").and_then(Value::as_array) else {
                continue;
            };
            if let Some(operation) = operations.iter().rev().find(|operation| {
                operation
                    .get("operationId")
                    .and_then(Value::as_str)
                    .is_some_and(|id| id == operation_id)
            }) {
                return Ok(Some(operation.clone()));
            }
        }
        Ok(None)
    }
}

impl PluginUninstallGuidanceSnapshotPort for ProductionPluginPort {
    fn guidance(
        &self,
        plugin_id: &str,
    ) -> Result<Option<PluginUninstallGuidance>, PluginUninstallGuidanceSnapshotError> {
        let marker_path = self.root.join(format!("{plugin_id}.json"));
        let path = self.root.join(format!("{plugin_id}.so"));
        Ok((marker_path.is_file() || path.is_file()).then(|| {
                PluginUninstallGuidance {
                    plugin_id: plugin_id.to_owned(),
                    path: path.to_string_lossy().into_owned(),
                    exists: path.is_file(),
                    commands: jftrade_strategy::PluginUninstallCommands {
                        posix: format!("rm -f '{}'", path.to_string_lossy().replace('\'', "'\\''")),
                        powershell: format!("Remove-Item -LiteralPath '{}' -Force", path.to_string_lossy().replace('\'', "''")),
                    },
                }
            }))
    }
}

impl PluginWritePort for ProductionPluginPort {
    fn mutate(
        &self,
        operation: PluginWriteOperation,
        plugin_id: &str,
    ) -> Result<Value, PluginWritePortError> {
        if !is_safe_plugin_id(plugin_id) {
            return Err(PluginWritePortError::NotFound(
                "plugin not found".to_owned(),
            ));
        }
        let _guard = self.operation_lock.lock().map_err(|_| {
            PluginWritePortError::Internal("plugin operation lock is poisoned".to_owned())
        })?;
        let marker_path = self.root.join(format!("{plugin_id}.json"));
        let bytes = std::fs::read(&marker_path).map_err(|error| {
            if error.kind() == std::io::ErrorKind::NotFound {
                PluginWritePortError::NotFound("plugin not found".to_owned())
            } else {
                PluginWritePortError::Internal(format!(
                    "read plugin marker {}: {error}",
                    marker_path.display()
                ))
            }
        })?;
        let mut marker: Value = serde_json::from_slice(&bytes).map_err(|error| {
            PluginWritePortError::Internal(format!(
                "decode plugin marker {}: {error}",
                marker_path.display()
            ))
        })?;
        let install_path = self.root.join(format!("{plugin_id}.so"));
        let artifact_change = match operation {
            PluginWriteOperation::Install => {
                ensure_plugin_artifact(&self.root, &marker_path, &marker, &install_path)?
            }
            PluginWriteOperation::Uninstall => remove_plugin_artifact(&install_path)?,
        };
        let object = marker.as_object_mut().ok_or_else(|| {
            PluginWritePortError::Internal("plugin marker must be a JSON object".to_owned())
        })?;
        let now = provider_now_rfc3339();
        let installed = matches!(operation, PluginWriteOperation::Install);
        let phase = if installed { "installed" } else { "uninstalled" };
        let operation_value = json!({
            "operationId": next_plugin_operation_id(plugin_id),
            "pluginId": plugin_id,
            "status": "SUCCEEDED",
            "phase": phase,
            "progress": 100,
            "message": if installed { "plugin metadata installed" } else { "plugin metadata uninstalled" },
            "targetDir": self.root,
            "installPath": install_path,
            "startedAt": now,
            "updatedAt": now,
            "completedAt": now,
            "error": null,
        });
        let installation = object
            .entry("installation".to_owned())
            .or_insert_with(|| json!({}));
        let installation = installation.as_object_mut().ok_or_else(|| {
            PluginWritePortError::Internal("plugin installation must be a JSON object".to_owned())
        })?;
        installation.insert("installed".to_owned(), Value::Bool(installed));
        installation.insert(
            "status".to_owned(),
            Value::String(if installed { "INSTALLED" } else { "NOT_INSTALLED" }.to_owned()),
        );
        installation.insert("currentOperation".to_owned(), Value::Null);
        installation.insert("lastOperation".to_owned(), operation_value.clone());
        let operations = object
            .entry("operations".to_owned())
            .or_insert_with(|| Value::Array(Vec::new()));
        let operations = operations.as_array_mut().ok_or_else(|| {
            PluginWritePortError::Internal("plugin operations must be a JSON array".to_owned())
        })?;
        operations.push(operation_value.clone());
        if operations.len() > 100 {
            let remove_count = operations.len() - 100;
            operations.drain(..remove_count);
        }
        if let Err(error) = persist_plugin_marker(&marker_path, &marker) {
            if let Err(rollback_error) = rollback_plugin_artifact(&install_path, artifact_change) {
                return Err(PluginWritePortError::Internal(format!(
                    "persist plugin marker failed: {error:?}; artifact rollback failed: {rollback_error}"
                )));
            }
            return Err(error);
        }
        Ok(operation_value)
    }
}

#[derive(Debug)]
enum PluginArtifactChange {
    Created,
    Removed(Vec<u8>),
    Unchanged,
}

fn is_safe_plugin_id(plugin_id: &str) -> bool {
    !plugin_id.is_empty()
        && plugin_id != "."
        && plugin_id != ".."
        && !plugin_id.contains('/')
        && !plugin_id.contains('\\')
        && !plugin_id.chars().any(char::is_control)
}

fn ensure_plugin_artifact(
    root: &Path,
    marker_path: &Path,
    marker: &Value,
    install_path: &Path,
) -> Result<PluginArtifactChange, PluginWritePortError> {
    match std::fs::symlink_metadata(install_path) {
        Ok(metadata) if metadata.file_type().is_file() => {
            return Ok(PluginArtifactChange::Unchanged);
        }
        Ok(_) => {
            return Err(PluginWritePortError::Internal(format!(
                "plugin install path is not a regular file: {}",
                install_path.display()
            )));
        }
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {}
        Err(error) => {
            return Err(PluginWritePortError::Internal(format!(
                "inspect plugin install path {}: {error}",
                install_path.display()
            )));
        }
    }

    let source = resolve_plugin_artifact_source(root, marker_path, marker)?;
    let temporary_path = install_path.with_file_name(format!(
        ".{}.tmp-{}",
        install_path
            .file_name()
            .and_then(|name| name.to_str())
            .unwrap_or("plugin.so"),
        PLUGIN_MARKER_TEMP_SEQUENCE.fetch_add(1, Ordering::Relaxed)
    ));
    if let Err(error) = std::fs::copy(&source, &temporary_path) {
        let _ = std::fs::remove_file(&temporary_path);
        return Err(PluginWritePortError::Internal(format!(
            "copy plugin artifact {} to {}: {error}",
            source.display(),
            install_path.display()
        )));
    }
    if let Err(error) = std::fs::rename(&temporary_path, install_path) {
        let _ = std::fs::remove_file(&temporary_path);
        return Err(PluginWritePortError::Internal(format!(
            "install plugin artifact {}: {error}",
            install_path.display()
        )));
    }
    Ok(PluginArtifactChange::Created)
}

fn resolve_plugin_artifact_source(
    root: &Path,
    marker_path: &Path,
    marker: &Value,
) -> Result<PathBuf, PluginWritePortError> {
    let raw = marker
        .get("installation")
        .and_then(Value::as_object)
        .and_then(|installation| installation.get("sourcePath"))
        .or_else(|| marker.get("artifact").and_then(|artifact| artifact.get("path")))
        .or_else(|| marker.get("sourcePath"))
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| {
            PluginWritePortError::Unavailable(
                "plugin artifact is not configured; provide installation.sourcePath".to_owned(),
            )
        })?;
    let raw_path = PathBuf::from(raw);
    let mut candidates = Vec::new();
    if raw_path.is_absolute() {
        candidates.push(raw_path);
    } else {
        if let Some(parent) = marker_path.parent() {
            candidates.push(parent.join(&raw_path));
        }
        if let Some(parent) = root.parent() {
            candidates.push(parent.join(&raw_path));
        }
        candidates.push(raw_path);
    }
    for candidate in candidates {
        let metadata = match std::fs::symlink_metadata(&candidate) {
            Ok(metadata) => metadata,
            Err(_) => continue,
        };
        if !metadata.file_type().is_file() {
            continue;
        }
        return std::fs::canonicalize(&candidate).map_err(|error| {
            PluginWritePortError::Unavailable(format!(
                "resolve plugin artifact {}: {error}",
                candidate.display()
            ))
        });
    }
    Err(PluginWritePortError::Unavailable(format!(
        "plugin artifact {raw} is unavailable"
    )))
}

fn remove_plugin_artifact(path: &Path) -> Result<PluginArtifactChange, PluginWritePortError> {
    let metadata = match std::fs::symlink_metadata(path) {
        Ok(metadata) => metadata,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
            return Ok(PluginArtifactChange::Unchanged);
        }
        Err(error) => {
            return Err(PluginWritePortError::Internal(format!(
                "inspect plugin artifact {}: {error}",
                path.display()
            )));
        }
    };
    if !metadata.file_type().is_file() {
        return Err(PluginWritePortError::Internal(format!(
            "plugin artifact is not a regular file: {}",
            path.display()
        )));
    }
    let contents = std::fs::read(path).map_err(|error| {
        PluginWritePortError::Internal(format!(
            "read plugin artifact {}: {error}",
            path.display()
        ))
    })?;
    std::fs::remove_file(path).map_err(|error| {
        PluginWritePortError::Internal(format!(
            "uninstall plugin artifact {}: {error}",
            path.display()
        ))
    })?;
    Ok(PluginArtifactChange::Removed(contents))
}

fn rollback_plugin_artifact(
    path: &Path,
    change: PluginArtifactChange,
) -> Result<(), String> {
    match change {
        PluginArtifactChange::Created => match std::fs::remove_file(path) {
            Ok(()) => Ok(()),
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(()),
            Err(error) => Err(format!("remove {}: {error}", path.display())),
        },
        PluginArtifactChange::Removed(contents) => persist_plugin_artifact(path, &contents),
        PluginArtifactChange::Unchanged => Ok(()),
    }
}

fn persist_plugin_artifact(path: &Path, contents: &[u8]) -> Result<(), String> {
    let temporary_path = path.with_file_name(format!(
        ".{}.restore-{}",
        path.file_name()
            .and_then(|name| name.to_str())
            .unwrap_or("plugin.so"),
        PLUGIN_MARKER_TEMP_SEQUENCE.fetch_add(1, Ordering::Relaxed)
    ));
    std::fs::write(&temporary_path, contents)
        .map_err(|error| format!("write {}: {error}", temporary_path.display()))?;
    if let Err(error) = std::fs::rename(&temporary_path, path) {
        let _ = std::fs::remove_file(&temporary_path);
        return Err(format!("restore {}: {error}", path.display()));
    }
    Ok(())
}

fn next_plugin_operation_id(plugin_id: &str) -> String {
    let timestamp = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .ok()
        .map(|duration| duration.as_nanos())
        .unwrap_or_default();
    let sequence = PLUGIN_OPERATION_SEQUENCE.fetch_add(1, Ordering::Relaxed);
    format!(
        "{}-{timestamp}-{sequence}",
        plugin_id.to_ascii_lowercase().replace(' ', "-")
    )
}

fn persist_plugin_marker(path: &std::path::Path, marker: &Value) -> Result<(), PluginWritePortError> {
    let bytes = serde_json::to_vec_pretty(marker).map_err(|error| {
        PluginWritePortError::Internal(format!("encode plugin marker {}: {error}", path.display()))
    })?;
    let temp_path = path.with_file_name(format!(
        ".{}.tmp-{}-{}-{}",
        path.file_name().and_then(|name| name.to_str()).unwrap_or("plugin.json"),
        std::process::id(),
        PLUGIN_MARKER_TEMP_SEQUENCE.fetch_add(1, Ordering::Relaxed),
        timestamp_suffix()
    ));
    if let Err(error) = std::fs::write(&temp_path, bytes) {
        return Err(PluginWritePortError::Internal(format!(
            "write plugin marker {}: {error}",
            temp_path.display()
        )));
    }
    if let Err(error) = std::fs::rename(&temp_path, path) {
        let _ = std::fs::remove_file(&temp_path);
        return Err(PluginWritePortError::Internal(format!(
            "replace plugin marker {}: {error}",
            path.display()
        )));
    }
    Ok(())
}

fn timestamp_suffix() -> u128 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .ok()
        .map(|duration| duration.as_nanos())
        .unwrap_or_default()
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    #[test]
    fn catalog_rejects_marker_without_descriptor() {
        let dir = tempdir().expect("temp plugin directory");
        let settings = dir.path().join("settings.json");
        std::fs::write(&settings, b"{}").expect("settings");
        let plugin_dir = dir.path().join("plugins");
        std::fs::create_dir_all(&plugin_dir).expect("plugins");
        std::fs::write(plugin_dir.join("broken.json"), br#"{"installation":{}}"#)
            .expect("marker");

        let port = ProductionPluginPort::open(&settings).expect("open plugin port");
        let error = port.catalog().expect_err("malformed marker must fail closed");
        assert!(error.to_string().contains("missing descriptor"));
    }
}
