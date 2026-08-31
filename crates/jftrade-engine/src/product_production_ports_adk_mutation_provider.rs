use std::fs;
use std::path::{Path, PathBuf};

use serde_json::{Map, Value};

use super::*;

fn adk_secrets_path(settings_path: &Path) -> PathBuf {
    std::env::var_os("JFTRADE_ADK_SECRETS")
        .filter(|value| !value.is_empty())
        .map(PathBuf::from)
        .unwrap_or_else(|| {
            settings_path
                .parent()
                .filter(|parent| !parent.as_os_str().is_empty())
                .map_or_else(
                    || PathBuf::from("secrets/adk-secrets.json"),
                    |parent| parent.join("secrets/adk-secrets.json"),
                )
        })
}

pub(super) fn read_adk_secrets(
    settings_path: &Path,
) -> Result<std::collections::BTreeMap<String, String>, AdkMutationPortError> {
    let path = adk_secrets_path(settings_path);
    let bytes = match fs::read(path) {
        Ok(bytes) => bytes,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
            return Ok(Default::default());
        }
        Err(error) => {
            return Err(AdkMutationPortError::Failed {
                status: 500,
                code: "ADK_SECRET_STORAGE_FAILED".to_owned(),
                message: error.to_string(),
            });
        }
    };
    if bytes.iter().all(u8::is_ascii_whitespace) {
        return Ok(Default::default());
    }
    serde_json::from_slice(&bytes).map_err(|error| AdkMutationPortError::Failed {
        status: 500,
        code: "ADK_SECRET_STORAGE_FAILED".to_owned(),
        message: error.to_string(),
    })
}

pub(super) fn write_adk_secrets(
    settings_path: &Path,
    secrets: &std::collections::BTreeMap<String, String>,
) -> Result<(), AdkMutationPortError> {
    let path = adk_secrets_path(settings_path);
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent).map_err(|error| AdkMutationPortError::Failed {
            status: 500,
            code: "ADK_SECRET_STORAGE_FAILED".to_owned(),
            message: error.to_string(),
        })?;
    }
    let bytes =
        serde_json::to_vec_pretty(secrets).map_err(|error| AdkMutationPortError::Failed {
            status: 500,
            code: "ADK_SECRET_STORAGE_FAILED".to_owned(),
            message: error.to_string(),
        })?;
    // A same-directory temporary keeps readers from observing a truncated
    // credentials file. Rename is atomic on the local filesystems supported
    // by the desktop runtime.
    let temporary = path.with_extension("json.tmp");
    fs::write(&temporary, bytes).map_err(|error| AdkMutationPortError::Failed {
        status: 500,
        code: "ADK_SECRET_STORAGE_FAILED".to_owned(),
        message: error.to_string(),
    })?;
    fs::rename(&temporary, &path).map_err(|error| AdkMutationPortError::Failed {
        status: 500,
        code: "ADK_SECRET_STORAGE_FAILED".to_owned(),
        message: error.to_string(),
    })?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let _ = fs::set_permissions(&path, fs::Permissions::from_mode(0o600));
    }
    Ok(())
}

pub(super) fn provider_payload(
    port: &ProductionAdkPort,
    id: &str,
    body: &Value,
    existing: Option<&jftrade_store_sqlite::StoredAdkEntity>,
) -> Result<(Value, Option<String>), AdkMutationPortError> {
    let body = object_body(body, "provider")?;
    let mut payload = existing
        .map(|row| decode_mutation_payload(&row.payload_json, "provider"))
        .transpose()?
        .unwrap_or_else(|| Value::Object(Map::new()));
    let object = payload
        .as_object_mut()
        .ok_or_else(|| invalid_mutation_input("invalid provider payload"))?;
    for key in [
        "displayName",
        "baseUrl",
        "model",
        "reasoningConfig",
        "contextWindowTokens",
        "requestTimeoutMs",
        "defaultHeaders",
        "enabled",
        "default",
    ] {
        if let Some(value) = body.get(key) {
            object.insert(key.to_owned(), value.clone());
        }
    }
    let display_name = object
        .get("displayName")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| invalid_mutation_input("provider displayName is required"))?;
    object.insert(
        "displayName".to_owned(),
        Value::String(display_name.to_owned()),
    );
    let base_url = object
        .get("baseUrl")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| invalid_mutation_input("provider baseUrl is required"))?;
    let parsed = reqwest::Url::parse(base_url)
        .map_err(|_| invalid_mutation_input("provider baseUrl must be a valid URL"))?;
    if !matches!(parsed.scheme(), "http" | "https") || parsed.host_str().is_none() {
        return Err(invalid_mutation_input(
            "provider baseUrl must use http or https",
        ));
    }
    object.insert("baseUrl".to_owned(), Value::String(base_url.to_owned()));
    let model = object
        .get("model")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| invalid_mutation_input("provider model is required"))?;
    object.insert("model".to_owned(), Value::String(model.to_owned()));
    if let Some(default) = object.get("default") {
        if !default.is_boolean() {
            return Err(invalid_mutation_input("provider default must be a boolean"));
        }
    } else {
        object.insert("default".to_owned(), Value::Bool(false));
    }
    if let Some(enabled) = object.get("enabled") {
        if !enabled.is_boolean() {
            return Err(invalid_mutation_input("provider enabled must be a boolean"));
        }
    } else {
        object.insert("enabled".to_owned(), Value::Bool(true));
    }

    let mut secrets = read_adk_secrets(&port.settings_path)?;
    let submitted = body.get("apiKey");
    let key = match submitted {
        Some(value) => {
            let key = value
                .as_str()
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .ok_or_else(|| {
                    invalid_mutation_input("provider apiKey must be a non-empty string")
                })?;
            secrets.insert(id.to_owned(), key.to_owned());
            Some(key.to_owned())
        }
        None => secrets.get(id).cloned().or_else(|| {
            object
                .get("apiKey")
                .and_then(Value::as_str)
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .map(str::to_owned)
        }),
    };
    if existing.is_none() && key.is_none() {
        return Err(invalid_mutation_input("provider apiKey is required"));
    }
    if let Some(key) = key.as_ref() {
        secrets.insert(id.to_owned(), key.clone());
    }
    object.remove("apiKey");
    object.insert("hasApiKey".to_owned(), Value::Bool(key.is_some()));
    Ok((payload, key))
}

/// Commit the credentials sidecar after the provider row has been accepted.
/// If the sidecar write fails, restore the previous row so callers never see
/// a provider that advertises a key which was not durably stored.
pub(super) fn commit_provider_secret(
    port: &ProductionAdkPort,
    id: &str,
    key: Option<&str>,
    previous: Option<&jftrade_store_sqlite::StoredAdkEntity>,
    previous_rows: &[jftrade_store_sqlite::StoredAdkEntity],
) -> Result<(), AdkMutationPortError> {
    let mut secrets = match read_adk_secrets(&port.settings_path) {
        Ok(secrets) => secrets,
        Err(error) => {
            return match restore_provider_snapshot(port, id, previous, previous_rows) {
                Ok(()) => Err(error),
                Err(rollback) => Err(rollback),
            };
        }
    };
    let before = secrets.clone();
    if let Some(key) = key {
        secrets.insert(id.to_owned(), key.to_owned());
    }
    if let Err(error) = write_adk_secrets(&port.settings_path, &secrets) {
        let secret_rollback = write_adk_secrets(&port.settings_path, &before).err();
        let row_rollback = restore_provider_snapshot(port, id, previous, previous_rows).err();
        if secret_rollback.is_some() || row_rollback.is_some() {
            return Err(provider_rollback_failed());
        }
        return Err(error);
    }
    Ok(())
}

fn restore_provider_snapshot(
    port: &ProductionAdkPort,
    id: &str,
    previous: Option<&jftrade_store_sqlite::StoredAdkEntity>,
    previous_rows: &[jftrade_store_sqlite::StoredAdkEntity],
) -> Result<(), AdkMutationPortError> {
    if previous_rows.is_empty() {
        return restore_provider_row(port, id, previous);
    }
    let mut failed = restore_provider_rows(port, previous_rows).is_err();
    if !previous_rows.iter().any(|row| row.id == id) && port.store.delete_provider(id).is_err() {
        failed = true;
    }
    if failed {
        Err(provider_rollback_failed())
    } else {
        Ok(())
    }
}

fn restore_provider_row(
    port: &ProductionAdkPort,
    id: &str,
    previous: Option<&jftrade_store_sqlite::StoredAdkEntity>,
) -> Result<(), AdkMutationPortError> {
    let result = match previous {
        Some(row) => port
            .store
            .upsert_provider(&row.id, &row.payload_json)
            .map(|_| ()),
        None => port.store.delete_provider(id).map(|_| ()),
    };
    result.map_err(|_| provider_rollback_failed())
}

pub(super) fn restore_provider_rows(
    port: &ProductionAdkPort,
    rows: &[jftrade_store_sqlite::StoredAdkEntity],
) -> Result<(), AdkMutationPortError> {
    let mut failed = false;
    for row in rows {
        if port
            .store
            .upsert_provider(&row.id, &row.payload_json)
            .is_err()
        {
            failed = true;
        }
    }
    if failed {
        Err(provider_rollback_failed())
    } else {
        Ok(())
    }
}

/// Restore both sides of a provider mutation.  The row and credentials
/// stores are separate durable resources, so each compensation is attempted
/// even when the other one fails.  Callers retain the original operation
/// error only when both restores succeed.
pub(super) fn provider_delete_failure(
    port: &ProductionAdkPort,
    rows: &[jftrade_store_sqlite::StoredAdkEntity],
    secrets: &std::collections::BTreeMap<String, String>,
    failure: AdkMutationPortError,
) -> AdkMutationPortError {
    let rows_failed = restore_provider_rows(port, rows).is_err();
    let secrets_failed = write_adk_secrets(&port.settings_path, secrets).is_err();
    if rows_failed || secrets_failed {
        provider_rollback_failed()
    } else {
        failure
    }
}

pub(super) fn provider_rollback_failed() -> AdkMutationPortError {
    AdkMutationPortError::Failed {
        status: 500,
        code: "ADK_PROVIDER_ROLLBACK_FAILED".to_owned(),
        message: "provider mutation failed and durable rollback was incomplete".to_owned(),
    }
}

pub(super) fn sanitized_provider_payload(
    value: Value,
    id: &str,
    settings_path: &Path,
) -> Result<Value, AdkMutationPortError> {
    let mut value = value;
    let object = value
        .as_object_mut()
        .ok_or_else(|| invalid_mutation_input("invalid provider payload"))?;
    object.remove("apiKey");
    let secrets = read_adk_secrets(settings_path)?;
    object.insert(
        "hasApiKey".to_owned(),
        Value::Bool(
            secrets
                .get(id)
                .is_some_and(|value| !value.trim().is_empty()),
        ),
    );
    Ok(value)
}
