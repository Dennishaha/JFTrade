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
    super::super::projection::normalize_provider_reasoning_config(&mut payload);
    {
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
    }
    super::super::projection::normalize_provider_reasoning_config(&mut payload);
    if body.get("reasoningConfig").is_some()
        && let Err(error) = super::super::projection::validate_provider_reasoning_config(
            payload
                .get("reasoningConfig")
                .unwrap_or(&Value::Null),
        )
    {
        return Err(invalid_mutation_input(&format!(
            "invalid provider reasoning configuration: {error}"
        )));
    }
    let object = payload
        .as_object_mut()
        .ok_or_else(|| invalid_mutation_input("invalid provider payload"))?;
    // Go's `StoreCore.SaveProvider` persists the normalized timeout, so a
    // provider saved without one reports the shared 180s default and a
    // hand-written value is clamped to the supported range instead of being
    // echoed back unchanged.
    let stored_timeout = object
        .get("requestTimeoutMs")
        .and_then(Value::as_i64)
        .unwrap_or_default();
    object.insert(
        "requestTimeoutMs".to_owned(),
        json!(normalize_provider_request_timeout_ms(stored_timeout)),
    );
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
    // Go's provider store treats the API key as optional durable state.  A
    // provider may be created before credentials are available (for example,
    // a disabled provider staged during onboarding); runtime readiness and
    // chat dispatch remain fail-closed until an enabled provider has a key.
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

/// Go `model.NormalizeProviderRequestTimeoutMs`: a missing or non-positive
/// timeout falls back to the shared 180s default, everything else is clamped
/// into `[15s, 600s]`.
fn normalize_provider_request_timeout_ms(value: i64) -> i64 {
    if value <= 0 {
        180_000
    } else {
        value.clamp(15_000, 600_000)
    }
}

#[cfg(test)]
mod tests {
    use std::collections::BTreeMap;
    use std::sync::Arc;

    use jftrade_store_sqlite::{AdkArtifactStore, AdkSessionStore, AdkStore};
    use serde_json::json;
    use tempfile::TempDir;

    use super::*;
    use crate::product::product_production_ports::ProductionToolCatalog;
    use crate::product::product_adk_chat_stream_port::{
        AdkChatInput, AdkChatPortError, AdkChatRoute, AdkChatStreamPort,
    };
    use crate::product::product_adk_model_runtime::{
        ProductionAdkChatRuntime, RunCancellationRegistry,
    };
    use crate::product::product_adk_mutation_port::{
        AdkMutationInput, AdkMutationOperation,
    };

    fn production_port() -> (ProductionAdkPort, TempDir) {
        let directory = tempfile::tempdir().expect("temporary directory");
        let adk_path = directory.path().join("adk.db");
        let session_path = directory.path().join("adk-session.db");
        let artifact_path = directory.path().join("adk-artifact.db");
        for (path, component) in [
            (&adk_path, "adk"),
            (&session_path, "adk-session"),
            (&artifact_path, "adk-artifact"),
        ] {
            let connection = rusqlite::Connection::open(path).expect("create ADK database");
            jftrade_store_sqlite::initialize_current(&connection, component)
                .expect("initialize ADK schema");
        }
        let tool_catalog = ProductionToolCatalog {
            tools: Vec::new(),
            bindings: BTreeMap::new(),
            research_bindings: BTreeMap::new(),
            active_provider_state: None,
            trade_runtime: None,
            backtest_execution_ready: false,
            pine_readiness: None,
        };
        let port = ProductionAdkPort {
            store: Arc::new(AdkStore::open(&adk_path).expect("open adk store")),
            session_store: Arc::new(
                AdkSessionStore::open(&session_path).expect("open adk session store"),
            ),
            artifact_store: Arc::new(
                AdkArtifactStore::open(&artifact_path).expect("open adk artifact store"),
            ),
            tool_catalog: Arc::new(tool_catalog),
            settings_path: directory.path().join("settings.json"),
            chat_runtime: None,
            unavailable_streams: Default::default(),
        };
        (port, directory)
    }

    #[test]
    fn disabled_provider_without_api_key_is_persisted_but_chat_stays_unavailable() {
        let (mut port, _directory) = production_port();
        let input = AdkMutationInput {
            operation: AdkMutationOperation::CreateProvider,
            identifiers: BTreeMap::new(),
            body: json!({
                "id": "provider-disabled-no-key",
                "displayName": "Disabled provider",
                "baseUrl": "https://example.test/v1",
                "model": "test-model",
                "enabled": false,
            }),
            webhook_secret: None,
        };
        let response = super::super::dispatch_mutation(&port, &input)
            .expect("Go-compatible provider persistence");
        assert_eq!(response["id"], "provider-disabled-no-key");
        assert_eq!(response["enabled"], false);
        assert_eq!(response["hasApiKey"], false);
        assert!(port
            .store
            .get_provider("provider-disabled-no-key")
            .expect("read persisted provider")
            .is_some());

        let runtime = ProductionAdkChatRuntime::new(
            Arc::clone(&port.store),
            Arc::clone(&port.session_store),
            &port.settings_path,
            Arc::new(RunCancellationRegistry::default()),
            Arc::clone(&port.tool_catalog),
        );
        assert!(!runtime.runtime_ready());
        port.chat_runtime = Some(runtime);
        let error = port
            .dispatch(
                AdkChatRoute::Chat,
                &AdkChatInput {
                    body: br#"{}"#.to_vec(),
                    client_request_id: "provider-disabled-no-key-request".to_owned(),
                },
            )
            .expect_err("provider without key must remain unavailable");
        assert!(matches!(error, AdkChatPortError::Unavailable(_)));
        port.shutdown();
    }

    /// Go's `StoreCore.SaveProvider` never echoes the credential: the saved row
    /// reports `hasApiKey`, the secret stays in the sidecar, and an omitted
    /// `defaultHeaders` field stays absent instead of being invented.
    ///
    /// Reference: go:452dea11:internal/assistant/engine/store_test.go
    /// `TestProviderSecretIsNotEchoed`.
/// Parity: go:452dea11:internal/assistant/engine/store_test.go:385
/// `TestProviderSecretIsNotEchoed`.
    #[test]
    fn saved_provider_hides_the_credential_from_the_row_and_projection() {
        let (port, directory) = production_port();
        let create = |body: serde_json::Value| AdkMutationInput {
            operation: AdkMutationOperation::CreateProvider,
            identifiers: BTreeMap::new(),
            body,
            webhook_secret: None,
        };

        let saved = super::super::dispatch_mutation(
            &port,
            &create(json!({
                "id": "openai",
                "displayName": "OpenAI",
                "baseUrl": "https://api.openai.com/v1",
                "model": "gpt-4o-mini",
                "apiKey": "fixture-credential",
                "enabled": true,
            })),
        )
        .expect("save provider");
        assert_eq!(saved["hasApiKey"], true, "{saved}");
        assert!(
            saved.get("apiKey").is_none(),
            "the credential must never be echoed back: {saved}"
        );
        assert!(saved["defaultHeaders"].is_null(), "{saved}");

        // The credential is durably stored in the sidecar, not in the row.
        let sidecar: serde_json::Value = serde_json::from_str(
            &std::fs::read_to_string(directory.path().join("secrets/adk-secrets.json"))
                .expect("read credential sidecar"),
        )
        .expect("decode credential sidecar");
        assert_eq!(sidecar["openai"], "fixture-credential");
        let row = port
            .store
            .get_provider("openai")
            .expect("read provider")
            .expect("provider row");
        assert!(
            !row.payload_json.contains("fixture-credential"),
            "the stored row must not carry the credential: {}",
            row.payload_json
        );
    }

    /// Go's `StoreCore.SaveProvider` persists the normalized request timeout:
    /// a provider created without one reports the shared 180s default, a
    /// hand-written 1s value is clamped to the 15s floor instead of being
    /// stored verbatim, and a later save that omits the field keeps the stored
    /// value.
    ///
    /// Reference: go:452dea11:internal/assistant/engine/store_test.go
    /// `TestProviderRequestTimeoutDefaultsAndClamp`.
/// Parity: go:452dea11:internal/assistant/engine/store_test.go:437
/// `TestProviderRequestTimeoutDefaultsAndClamp`.
    #[test]
    fn saved_provider_normalizes_the_request_timeout_on_write() {
        let (port, _directory) = production_port();
        let saved = super::super::dispatch_mutation(
            &port,
            &AdkMutationInput {
                operation: AdkMutationOperation::CreateProvider,
                identifiers: BTreeMap::new(),
                body: json!({
                    "id": "openai-clamped",
                    "displayName": "OpenAI",
                    "baseUrl": "https://api.openai.com/v1",
                    "model": "gpt-4o-mini",
                    "enabled": true,
                }),
                webhook_secret: None,
            },
        )
        .expect("save provider with the default timeout");
        assert_eq!(
            saved["requestTimeoutMs"], 180_000,
            "a provider created without a timeout keeps Go's default: {saved}"
        );

        // 1s is clamped to the 15s floor on save, and later saves preserve the
        // stored timeout when the request omits the field.
        let clamped = super::super::dispatch_mutation(
            &port,
            &AdkMutationInput {
                operation: AdkMutationOperation::UpdateProvider,
                identifiers: BTreeMap::from([(
                    "providerId".to_owned(),
                    "openai-clamped".to_owned(),
                )]),
                body: json!({
                    "displayName": "OpenAI",
                    "baseUrl": "https://api.openai.com/v1",
                    "model": "gpt-4o-mini",
                    "requestTimeoutMs": 1_000,
                    "enabled": true,
                }),
                webhook_secret: None,
            },
        )
        .expect("update provider");
        assert_eq!(clamped["requestTimeoutMs"], 15_000, "{clamped}");

        let unchanged = super::super::dispatch_mutation(
            &port,
            &AdkMutationInput {
                operation: AdkMutationOperation::UpdateProvider,
                identifiers: BTreeMap::from([(
                    "providerId".to_owned(),
                    "openai-clamped".to_owned(),
                )]),
                body: json!({
                    "displayName": "OpenAI",
                    "baseUrl": "https://api.openai.com/v1",
                    "model": "gpt-4o-mini",
                    "enabled": true,
                }),
                webhook_secret: None,
            },
        )
        .expect("update provider without a timeout");
        assert_eq!(
            unchanged["requestTimeoutMs"], 15_000,
            "an omitted timeout keeps the stored one: {unchanged}"
        );
    }

    /// Parity: go:452dea11:internal/api/assistant/adk_routes_test.go:597
    /// `TestADKProviderSaveReturnsRequestTimeoutMs`.
    ///
    /// The Go route posts a provider through the HTTP handler and checks the
    /// response data, including the caller-supplied timeout and the removal of
    /// the retired `apiProtocol` field.  Exercise the production mutation port
    /// with the same payload so the durable save and sanitized response are
    /// both covered, rather than only checking the transport parser.
    #[test]
    fn provider_save_preserves_request_timeout_and_omits_removed_api_protocol() {
        let (port, _directory) = production_port();
        let saved = super::super::dispatch_mutation(
            &port,
            &AdkMutationInput {
                operation: AdkMutationOperation::CreateProvider,
                identifiers: BTreeMap::new(),
                body: json!({
                    "displayName": "Slow Provider",
                    "baseUrl": "https://api.openai.com/v1",
                    "model": "gpt-4o-mini",
                    "requestTimeoutMs": 250_000,
                    "enabled": true,
                    "apiProtocol": "legacy",
                }),
                webhook_secret: None,
            },
        )
        .expect("save provider");

        assert_eq!(saved["requestTimeoutMs"], 250_000, "{saved}");
        assert!(
            saved
                .as_object()
                .is_some_and(|object| !object.contains_key("apiProtocol")),
            "provider response leaked removed apiProtocol: {saved}"
        );
    }
}
