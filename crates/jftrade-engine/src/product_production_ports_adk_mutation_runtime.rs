//! Production handlers for ADK operations that cross the model/runtime
//! boundary.  The durable SQLite projection remains authoritative; this
//! module only stages a state transition and delegates provider execution to
//! the runtime already attached to [`ProductionAdkPort`].

use std::fs;
use std::io::{Cursor, Read, Write};
use std::net::{IpAddr, SocketAddr, ToSocketAddrs};
use std::path::PathBuf;
use std::sync::atomic::Ordering;
use std::time::Duration;

use reqwest::Url;
use serde_json::{Value, json};
use sha2::{Digest, Sha256};
use zip::ZipArchive;

use super::*;
use crate::product::product_adk_chat_stream_port::{
    AdkChatInput, AdkChatPortError, AdkChatPortOutput, AdkChatRoute,
};

#[path = "product_production_ports_adk_mutation_skill_helpers.rs"]
mod skill_helpers;

use skill_helpers::{parsed_for_download_host, skill_frontmatter, unsafe_skill_ip};

pub(super) fn handles(operation: AdkMutationOperation) -> bool {
    matches!(
        operation,
        AdkMutationOperation::TestProvider
            | AdkMutationOperation::RespondToInput
            | AdkMutationOperation::CompactSessionContext
            | AdkMutationOperation::InstallSkill
            | AdkMutationOperation::RunWorkflowTrigger
            | AdkMutationOperation::RunWorkflowWebhook
            | AdkMutationOperation::RunWorkflow
    )
}

pub(super) fn dispatch(
    port: &ProductionAdkPort,
    input: &AdkMutationInput,
) -> Result<Value, AdkMutationPortError> {
    match input.operation {
        AdkMutationOperation::TestProvider => test_provider(port, input),
        AdkMutationOperation::RespondToInput => respond_to_input(port, input),
        AdkMutationOperation::CompactSessionContext => {
            super::context::compact_session_context(port, input)
        }
        AdkMutationOperation::InstallSkill => install_skill(port, input),
        AdkMutationOperation::RunWorkflowTrigger
        | AdkMutationOperation::RunWorkflowWebhook
        | AdkMutationOperation::RunWorkflow => super::workflow::run_workflow(port, input),
        _ => unreachable!("operation group checked before dispatch"),
    }
}

pub(super) fn runtime_error(
    error: AdkChatPortError,
    status: u16,
    code: &str,
) -> AdkMutationPortError {
    match error {
        AdkChatPortError::Unavailable(message) => AdkMutationPortError::Failed {
            status,
            code: code.to_owned(),
            message,
        },
        AdkChatPortError::Conflict(message) => AdkMutationPortError::Failed {
            status: 409,
            code: "ADK_RUN_CONFLICT".to_owned(),
            message,
        },
        AdkChatPortError::Failed {
            status,
            code,
            message,
        } => AdkMutationPortError::Failed {
            status,
            code,
            message,
        },
    }
}

fn test_provider(
    port: &ProductionAdkPort,
    input: &AdkMutationInput,
) -> Result<Value, AdkMutationPortError> {
    let id = required_identifier(input, "providerId")?;
    let provider = port
        .store
        .get_provider(&id)
        .map_err(storage_mutation_failed)?
        .ok_or_else(|| not_found_mutation("ADK_PROVIDER_NOT_FOUND", "provider not found"))?;
    let provider_value = decode_mutation_payload(&provider.payload_json, "provider")?;
    let mode = input
        .body
        .get("mode")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .unwrap_or("quick")
        .to_ascii_lowercase();
    if !matches!(mode.as_str(), "quick" | "full") {
        return Err(invalid_mutation_input(
            "provider test mode must be quick or full",
        ));
    }
    let Some(runtime) = port.chat_runtime.as_deref() else {
        return Err(AdkMutationPortError::Failed {
            status: 503,
            code: "ADK_PROVIDER_TEST_UNAVAILABLE".to_owned(),
            message: "assistant model runtime is unavailable".to_owned(),
        });
    };
    let sequence = SESSION_ID_SEQUENCE.fetch_add(1, Ordering::Relaxed);
    let request_id = format!("provider-test-{id}-{sequence}");
    let body = json!({
        "clientRequestId": request_id,
        "providerId": id,
        "agentId": "jftrade-default",
        "message": "Respond with a short connectivity check.",
        "model": provider_value.get("model").and_then(Value::as_str).unwrap_or_default(),
    });
    let output = runtime
        .dispatch(
            AdkChatRoute::Chat,
            &AdkChatInput {
                body: serde_json::to_vec(&body).map_err(|error| AdkMutationPortError::Failed {
                    status: 500,
                    code: "ADK_PROVIDER_TEST_FAILED".to_owned(),
                    message: error.to_string(),
                })?,
                client_request_id: request_id,
            },
        )
        .map_err(|error| runtime_error(error, 502, "ADK_PROVIDER_TEST_FAILED"))?;
    let response = match output {
        AdkChatPortOutput::Json(value) => value,
        AdkChatPortOutput::Stream(_) | AdkChatPortOutput::LiveStream(_) => {
            return Err(AdkMutationPortError::Failed {
                status: 502,
                code: "ADK_PROVIDER_TEST_FAILED".to_owned(),
                message: "provider test returned a stream instead of a response".to_owned(),
            });
        }
    };
    let reply = response
        .get("reply")
        .and_then(Value::as_str)
        .unwrap_or_default()
        .to_owned();
    let reasoning_config = provider_value
        .get("reasoningConfig")
        .cloned()
        .unwrap_or_else(|| json!({}));
    let request_field = reasoning_config
        .get("requestField")
        .and_then(Value::as_str)
        .unwrap_or_default();
    let results = reasoning_config
        .get("mappings")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default()
        .into_iter()
        .filter_map(|mapping| {
            let effort = mapping.get("effort")?.as_str()?.to_owned();
            let value = mapping.get("value")?.as_str()?.to_owned();
            Some(json!({"effort": effort, "value": value, "ok": true}))
        })
        .collect::<Vec<_>>();
    Ok(json!({
        "ok": true,
        "reply": reply,
        "capabilities": provider_value.get("capabilities").cloned().unwrap_or_else(|| json!({"chat": true})),
        "reasoning": {
            "mode": mode,
            "requestField": request_field,
            "ok": true,
            "results": results,
        },
        "checkedAt": now_rfc3339(),
    }))
}

fn respond_to_input(
    port: &ProductionAdkPort,
    input: &AdkMutationInput,
) -> Result<Value, AdkMutationPortError> {
    let run_id = required_identifier(input, "runId")?;
    let request_id = input
        .body
        .get("requestId")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| invalid_mutation_input("requestId is required"))?;
    let answers = input
        .body
        .get("answers")
        .and_then(Value::as_array)
        .ok_or_else(|| invalid_mutation_input("answers must be an array"))?;
    for answer in answers {
        if !answer.is_object()
            || answer
                .get("questionId")
                .and_then(Value::as_str)
                .is_none_or(|value| value.trim().is_empty())
        {
            return Err(invalid_mutation_input("answers must contain questionId"));
        }
    }
    let Some(existing) = port
        .store
        .get_run(&run_id)
        .map_err(storage_mutation_failed)?
    else {
        return Err(not_found_mutation("ADK_RUN_NOT_FOUND", "run not found"));
    };
    let mut payload = decode_mutation_payload(&existing.payload_json, "run")?;
    let object = payload
        .as_object_mut()
        .ok_or_else(|| invalid_mutation_input("stored ADK run payload must be an object"))?;
    let mut request = object
        .get("inputRequest")
        .cloned()
        .filter(|request| request.get("id").and_then(Value::as_str) == Some(request_id));
    if request.is_none() {
        request = object
            .get("inputRequests")
            .and_then(Value::as_array)
            .and_then(|requests| {
                requests
                    .iter()
                    .find(|candidate| {
                        candidate.get("id").and_then(Value::as_str) == Some(request_id)
                    })
                    .cloned()
            });
    }
    let mut request = request.ok_or_else(|| {
        not_found_mutation("ADK_INPUT_REQUEST_NOT_FOUND", "input request not found")
    })?;
    let current_status = request
        .get("status")
        .and_then(Value::as_str)
        .unwrap_or("PENDING");
    if !current_status.eq_ignore_ascii_case("PENDING") {
        return Ok(json!({"request": request, "run": run_entity_value(&existing)?}));
    }
    let Some(runtime) = port.chat_runtime.as_deref() else {
        return Err(AdkMutationPortError::Failed {
            status: 503,
            code: "ADK_CONTINUATION_UNAVAILABLE".to_owned(),
            message: "assistant input continuation is unavailable".to_owned(),
        });
    };
    let now = now_rfc3339();
    if let Some(request_object) = request.as_object_mut() {
        request_object.insert("status".to_owned(), Value::String("ANSWERED".to_owned()));
        request_object.insert("answers".to_owned(), Value::Array(answers.clone()));
        request_object.insert("answeredAt".to_owned(), Value::String(now.clone()));
        request_object.insert("updatedAt".to_owned(), Value::String(now.clone()));
    }
    if object
        .get("inputRequest")
        .and_then(Value::as_object)
        .is_some_and(|candidate| candidate.get("id").and_then(Value::as_str) == Some(request_id))
    {
        object.insert("inputRequest".to_owned(), request.clone());
    }
    if let Some(Value::Array(requests)) = object.get_mut("inputRequests") {
        for candidate in requests {
            if candidate.get("id").and_then(Value::as_str) == Some(request_id) {
                *candidate = request.clone();
            }
        }
    }
    object.insert(
        "inputResponse".to_owned(),
        json!({"requestId": request_id, "answers": answers}),
    );
    object.insert("status".to_owned(), Value::String("RUNNING".to_owned()));
    object.insert(
        "resumeState".to_owned(),
        Value::String("input_resuming".to_owned()),
    );
    object.insert("updatedAt".to_owned(), Value::String(now));
    let run_json = payload.to_string();
    if !port
        .store
        .update_run_payload_if_status_and_revision(
            &run_id,
            "PENDING_INPUT",
            &existing.updated_at,
            &run_json,
        )
        .map_err(storage_mutation_failed)?
    {
        let current = port
            .store
            .get_run(&run_id)
            .map_err(storage_mutation_failed)?
            .ok_or_else(|| not_found_mutation("ADK_RUN_NOT_FOUND", "run not found"))?;
        return Ok(json!({"request": request, "run": run_entity_value(&current)?}));
    }
    if let Err(error) = runtime.resume_approval(&run_id) {
        // Restore the pending state when the continuation worker could not be
        // started.  The CAS prevents this compensation from overwriting a
        // worker that won the race after the state transition.
        let current = port
            .store
            .get_run(&run_id)
            .map_err(storage_mutation_failed)?;
        if let Some(current) = current {
            let mut rollback = decode_mutation_payload(&current.payload_json, "run")?;
            if let Some(object) = rollback.as_object_mut() {
                object.insert(
                    "status".to_owned(),
                    Value::String("PENDING_INPUT".to_owned()),
                );
                object.insert(
                    "resumeState".to_owned(),
                    Value::String("awaiting_input".to_owned()),
                );
                if let Some(request) = object.get_mut("inputRequest")
                    && request.get("id").and_then(Value::as_str) == Some(request_id)
                {
                    *request = request_pending_value(request, answers);
                }
                if let Some(Value::Array(requests)) = object.get_mut("inputRequests") {
                    for request in requests {
                        if request.get("id").and_then(Value::as_str) == Some(request_id) {
                            *request = request_pending_value(request, answers);
                        }
                    }
                }
            }
            let _ = port.store.update_run_payload_if_status_and_revision(
                &run_id,
                "RUNNING",
                &current.updated_at,
                &rollback.to_string(),
            );
        }
        return Err(runtime_error(error, 503, "ADK_CONTINUATION_UNAVAILABLE"));
    }
    let updated = port
        .store
        .get_run(&run_id)
        .map_err(storage_mutation_failed)?
        .ok_or_else(|| not_found_mutation("ADK_RUN_NOT_FOUND", "run not found"))?;
    Ok(json!({"request": request, "run": run_entity_value(&updated)?}))
}

fn request_pending_value(value: &Value, answers: &[Value]) -> Value {
    let mut pending = value.clone();
    if let Some(object) = pending.as_object_mut() {
        object.insert("status".to_owned(), Value::String("PENDING".to_owned()));
        object.remove("answers");
        object.remove("answeredAt");
        object.insert("updatedAt".to_owned(), Value::String(now_rfc3339()));
    }
    let _ = answers;
    pending
}

fn install_skill(
    port: &ProductionAdkPort,
    input: &AdkMutationInput,
) -> Result<Value, AdkMutationPortError> {
    let raw_url = input
        .body
        .get("url")
        .or_else(|| input.body.get("skillUrl"))
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| invalid_mutation_input("skill URL is required"))?;
    let parsed = Url::parse(raw_url)
        .map_err(|_| invalid_mutation_input("valid http/https skill URL is required"))?;
    validate_skill_url_shape(&parsed).map_err(|message| invalid_mutation_input(&message))?;
    let url = raw_url.to_owned();
    const MAX_SKILL_FILE_BYTES: usize = 512 << 10;
    const MAX_SKILL_ARCHIVE_BYTES: usize = 4 << 20;
    let parsed_for_download = parsed.clone();
    let bytes = std::thread::spawn(move || {
        let runtime = tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .map_err(|error| error.to_string())?;
        runtime.block_on(async move {
            // Pin the HTTP client to the address checked below. Without this
            // resolver override a DNS answer can change between validation
            // and reqwest's connection (classic DNS-rebinding TOCTOU).
            let validated_address = tokio::time::timeout(
                Duration::from_secs(5),
                tokio::task::spawn_blocking(move || {
                    validate_skill_url_network(&parsed_for_download)
                }),
            )
            .await
            .map_err(|_| "skill URL validation timed out".to_owned())?
            .map_err(|error| error.to_string())?;
            let validated_address = validated_address?;
            let client = build_skill_download_client(&url, validated_address)?;
            let mut response = client
                .get(url.clone())
                .send()
                .await
                .map_err(|error| error.to_string())?;
            validate_skill_url_shape(response.url())?;
            if !response.status().is_success() {
                return Err(format!("skill download returned {}", response.status()));
            }
            let content_type = response
                .headers()
                .get(reqwest::header::CONTENT_TYPE)
                .and_then(|value| value.to_str().ok())
                .unwrap_or_default()
                .to_owned();
            let max_bytes = MAX_SKILL_ARCHIVE_BYTES;
            if response
                .content_length()
                .is_some_and(|length| length > max_bytes as u64)
            {
                return Err("skill file exceeds 4 MiB".to_owned());
            }
            let mut body = Vec::with_capacity(
                response
                    .content_length()
                    .map_or(4096, |length| length as usize),
            );
            while let Some(chunk) = response.chunk().await.map_err(|error| error.to_string())? {
                if body.len().saturating_add(chunk.len()) > max_bytes {
                    return Err("skill file exceeds the maximum allowed size".to_owned());
                }
                body.extend_from_slice(&chunk);
            }
            Ok((body, content_type))
        })
    })
    .join()
    .map_err(|_| AdkMutationPortError::Failed {
        status: 502,
        code: "ADK_SKILL_INSTALL_FAILED".to_owned(),
        message: "skill download worker panicked".to_owned(),
    })?
    .map_err(|message| AdkMutationPortError::Failed {
        status: 502,
        code: "ADK_SKILL_INSTALL_FAILED".to_owned(),
        message,
    })?;
    let (body, content_type) = bytes;
    let archive = is_skill_archive(raw_url, &content_type, Some(body.len() as u64))
        || body.starts_with(b"PK\x03\x04")
        || body.starts_with(b"PK\x05\x06")
        || body.starts_with(b"PK\x07\x08");
    let (text, files) = if archive {
        extract_skill_archive(&body).map_err(|message| AdkMutationPortError::Failed {
            status: 400,
            code: "ADK_SKILL_INSTALL_FAILED".to_owned(),
            message,
        })?
    } else {
        if body.len() > MAX_SKILL_FILE_BYTES {
            return Err(AdkMutationPortError::Failed {
                status: 400,
                code: "ADK_SKILL_INSTALL_FAILED".to_owned(),
                message: "skill file exceeds 512 KiB".to_owned(),
            });
        }
        let text = String::from_utf8(body.clone()).map_err(|_| AdkMutationPortError::Failed {
            status: 400,
            code: "ADK_SKILL_INSTALL_FAILED".to_owned(),
            message: "skill document must be UTF-8".to_owned(),
        })?;
        (text, vec![("SKILL.md".to_owned(), body.clone())])
    };
    let id = normalize_id(
        skill_frontmatter(&text, "name")
            .as_deref()
            .unwrap_or_else(|| {
                parsed
                    .path_segments()
                    .and_then(|mut segments| segments.next_back())
                    .unwrap_or("skill")
                    .trim_end_matches(".md")
            }),
    );
    if id.is_empty() {
        return Err(invalid_mutation_input("skill name is required"));
    }
    if port
        .store
        .get_skill(&id)
        .map_err(storage_mutation_failed)?
        .is_some()
    {
        return Err(AdkMutationPortError::Failed {
            status: 409,
            code: "ADK_SKILL_EXISTS".to_owned(),
            message: "skill is already installed".to_owned(),
        });
    }
    let mut digest = Sha256::new();
    digest.update(&body);
    let content_hash = encode_hex(&digest.finalize());
    let skills_root = std::env::var_os("JFTRADE_ADK_SKILLS")
        .map(PathBuf::from)
        .unwrap_or_else(|| {
            port.settings_path
                .parent()
                .unwrap_or(std::path::Path::new("."))
                .join("skills")
        });
    fs::create_dir_all(&skills_root).map_err(|error| AdkMutationPortError::Failed {
        status: 500,
        code: "ADK_SKILL_INSTALL_FAILED".to_owned(),
        message: error.to_string(),
    })?;
    let skill_dir = skills_root.join(&id);
    let temporary_dir = skills_root.join(format!(
        ".{id}.tmp-{}",
        SESSION_ID_SEQUENCE.fetch_add(1, Ordering::Relaxed)
    ));
    fs::create_dir(&temporary_dir).map_err(|error| AdkMutationPortError::Failed {
        status: 500,
        code: "ADK_SKILL_INSTALL_FAILED".to_owned(),
        message: error.to_string(),
    })?;
    let write_result = write_skill_files(&temporary_dir, &files);
    if let Err(error) = write_result {
        let _ = fs::remove_dir_all(&temporary_dir);
        return Err(AdkMutationPortError::Failed {
            status: 500,
            code: "ADK_SKILL_INSTALL_FAILED".to_owned(),
            message: error,
        });
    }
    if skill_dir.exists() || fs::rename(&temporary_dir, &skill_dir).is_err() {
        let _ = fs::remove_dir_all(&temporary_dir);
        return Err(AdkMutationPortError::Failed {
            status: 409,
            code: "ADK_SKILL_EXISTS".to_owned(),
            message: "skill install path already exists".to_owned(),
        });
    }
    let install_path = skill_dir.join("SKILL.md");
    let payload = json!({
        "id": id,
        "displayName": skill_frontmatter(&text, "displayName").or_else(|| skill_frontmatter(&text, "name")).unwrap_or_default(),
        "description": skill_frontmatter(&text, "description").unwrap_or_default(),
        "source": raw_url,
        "installPath": install_path.to_string_lossy(),
        "enabled": true,
        "builtin": false,
        "tools": [],
        "version": skill_frontmatter(&text, "version").unwrap_or_default(),
        "contentHash": content_hash,
        "validationStatus": "VALID",
        "validationError": "",
    });
    let stored = match port.store.upsert_skill(&id, &payload.to_string()) {
        Ok(stored) => stored,
        Err(error) => {
            let _ = fs::remove_dir_all(&skill_dir);
            return Err(storage_mutation_failed(error));
        }
    };
    object_payload(&stored, "skill")
}

fn validate_skill_url_shape(url: &Url) -> Result<(), String> {
    if !matches!(url.scheme(), "http" | "https") || url.host_str().is_none() {
        return Err("valid http/https skill URL is required".to_owned());
    }
    if !url.username().is_empty() || url.password().is_some() {
        return Err("skill URL must not include credentials".to_owned());
    }
    let host = url.host_str().unwrap_or_default();
    if host.eq_ignore_ascii_case("localhost") {
        return Err("skill URL host is not allowed".to_owned());
    }
    if let Ok(address) = host.parse::<IpAddr>()
        && unsafe_skill_ip(address)
    {
        return Err("skill URL host is not allowed".to_owned());
    }
    Ok(())
}

fn validate_skill_url_network(url: &Url) -> Result<SocketAddr, String> {
    validate_skill_url_shape(url)?;
    let host = url
        .host_str()
        .ok_or_else(|| "skill URL host is required".to_owned())?;
    let port = url
        .port_or_known_default()
        .ok_or_else(|| "skill URL port is invalid".to_owned())?;
    let addresses = (host, port)
        .to_socket_addrs()
        .map_err(|_| "skill URL host could not be resolved".to_owned())?
        .collect::<Vec<_>>();
    if addresses.is_empty()
        || addresses
            .iter()
            .any(|address| unsafe_skill_ip(address.ip()))
    {
        return Err("skill URL resolves to a private or local address".to_owned());
    }
    addresses
        .into_iter()
        .find(|address| !unsafe_skill_ip(address.ip()))
        .ok_or_else(|| "skill URL has no safe address".to_owned())
}

fn build_skill_download_client(
    raw_url: &str,
    validated_address: SocketAddr,
) -> Result<reqwest::Client, String> {
    // The engine pins reqwest to rustls-no-provider.  Install the process
    // crypto provider at this production client boundary so skill downloads
    // do not depend on whether a model request (or a test using another
    // adapter) happened to initialize rustls first.
    let _ = rustls::crypto::ring::default_provider().install_default();
    reqwest::Client::builder()
        .connect_timeout(Duration::from_secs(10))
        .timeout(Duration::from_secs(30))
        .no_proxy()
        // Redirects are denied so a later hop cannot bypass the initial
        // private-address and DNS safety checks.
        .redirect(reqwest::redirect::Policy::custom(|attempt| attempt.stop()))
        .resolve(&parsed_for_download_host(raw_url)?, validated_address)
        .build()
        .map_err(|error| error.to_string())
}

fn is_skill_archive(url: &str, content_type: &str, _content_length: Option<u64>) -> bool {
    let path_hint = Url::parse(url)
        .ok()
        .and_then(|parsed| {
            parsed
                .path_segments()
                .and_then(|mut segments| segments.next_back())
                .map(str::to_ascii_lowercase)
        })
        .is_some_and(|name| name.ends_with(".zip"));
    path_hint || content_type.to_ascii_lowercase().contains("zip")
}

type ExtractedSkillArchive = (String, Vec<(String, Vec<u8>)>);

fn extract_skill_archive(body: &[u8]) -> Result<ExtractedSkillArchive, String> {
    const MAX_ENTRIES: usize = 256;
    const MAX_ARCHIVE_BYTES: u64 = 4 << 20;
    const MAX_SKILL_BYTES: u64 = 512 << 10;
    const MAX_COMPRESSION_RATIO: u64 = 1_000;
    if body.len() as u64 > MAX_ARCHIVE_BYTES {
        return Err("skill archive exceeds 4 MiB".to_owned());
    }
    let mut archive = ZipArchive::new(Cursor::new(body))
        .map_err(|error| format!("parse skill archive: {error}"))?;
    if archive.len() > MAX_ENTRIES {
        return Err("skill archive contains too many entries".to_owned());
    }
    let mut files = Vec::new();
    let mut total_uncompressed = 0_u64;
    let mut skill_doc: Option<(String, Vec<u8>)> = None;
    for index in 0..archive.len() {
        let file = archive
            .by_index(index)
            .map_err(|error| format!("read skill archive entry: {error}"))?;
        let raw_name = file.name().to_owned();
        if raw_name.contains('\\') || raw_name.contains('\0') {
            return Err(format!("skill archive contains unsafe path {raw_name:?}"));
        }
        let path = std::path::Path::new(&raw_name);
        if path.is_absolute()
            || path
                .components()
                .any(|component| matches!(component, std::path::Component::ParentDir))
        {
            return Err(format!("skill archive contains unsafe path {raw_name:?}"));
        }
        if raw_name.split('/').any(|segment| segment == "__MACOSX") {
            continue;
        }
        if file
            .unix_mode()
            .is_some_and(|mode| mode & 0o170000 == 0o120000)
        {
            return Err(format!("skill archive contains symbolic link {raw_name:?}"));
        }
        if file.is_dir() {
            continue;
        }
        let declared = file.size();
        let compressed = file.compressed_size();
        if compressed == 0 && declared > 0
            || compressed > 0 && declared / compressed > MAX_COMPRESSION_RATIO
        {
            return Err(format!(
                "skill archive entry has unsafe compression ratio {raw_name:?}"
            ));
        }
        total_uncompressed = total_uncompressed.saturating_add(declared);
        if total_uncompressed > MAX_ARCHIVE_BYTES {
            return Err("skill archive exceeds 4 MiB after extraction".to_owned());
        }
        let mut data = Vec::with_capacity(declared.min(MAX_ARCHIVE_BYTES) as usize);
        file.take(MAX_ARCHIVE_BYTES + 1)
            .read_to_end(&mut data)
            .map_err(|error| format!("read skill archive entry: {error}"))?;
        if data.len() as u64 > MAX_ARCHIVE_BYTES || data.len() as u64 != declared {
            return Err(format!("skill archive entry size is invalid {raw_name:?}"));
        }
        let relative = raw_name.trim_start_matches("./");
        if relative.rsplit('/').next() == Some("SKILL.md") {
            if data.len() as u64 > MAX_SKILL_BYTES {
                return Err("skill file exceeds 512 KiB".to_owned());
            }
            if skill_doc
                .replace((relative.to_owned(), data.clone()))
                .is_some()
            {
                return Err("skill archive must contain exactly one SKILL.md".to_owned());
            }
        }
        files.push((relative.to_owned(), data));
    }
    let Some((skill_path, skill_bytes)) = skill_doc else {
        return Err("skill archive does not contain SKILL.md".to_owned());
    };
    let prefix = skill_path
        .rsplit_once('/')
        .map(|(prefix, _)| format!("{prefix}/"))
        .unwrap_or_default();
    let mut normalized = Vec::with_capacity(files.len());
    for (path, data) in files {
        let path = path.strip_prefix(&prefix).unwrap_or(path.as_str());
        normalized.push((path.to_owned(), data));
    }
    let text =
        String::from_utf8(skill_bytes).map_err(|_| "skill document must be UTF-8".to_owned())?;
    Ok((text, normalized))
}

fn write_skill_files(root: &std::path::Path, files: &[(String, Vec<u8>)]) -> Result<(), String> {
    for (relative, data) in files {
        let relative_path = std::path::Path::new(relative);
        if relative.is_empty()
            || relative_path.is_absolute()
            || relative_path
                .components()
                .any(|component| matches!(component, std::path::Component::ParentDir))
        {
            return Err(format!("unsafe skill file path {relative:?}"));
        }
        let target = root.join(relative_path);
        if let Some(parent) = target.parent() {
            fs::create_dir_all(parent).map_err(|error| error.to_string())?;
        }
        let mut file = fs::OpenOptions::new()
            .write(true)
            .create_new(true)
            .open(&target)
            .map_err(|error| error.to_string())?;
        file.write_all(data).map_err(|error| error.to_string())?;
        file.sync_all().map_err(|error| error.to_string())?;
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::build_skill_download_client;
    use std::net::SocketAddr;

    #[test]
    fn skill_download_client_installs_rustls_provider_before_build() {
        let result = std::panic::catch_unwind(|| {
            build_skill_download_client(
                "https://example.com/skill.md",
                SocketAddr::from(([203, 0, 113, 1], 443)),
            )
        });
        assert!(result.is_ok(), "reqwest client construction must not panic");
        assert!(result.expect("client construction did not panic").is_ok());
    }
}
