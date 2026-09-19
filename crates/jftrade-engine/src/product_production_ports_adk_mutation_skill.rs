/// `400 ADK_SKILL_INSTALL_FAILED` carrying the registry message.
///
/// The Go skill handler wraps every `InstallSkill` error in this code, so URL
/// parsing, host validation and download failures stay distinguishable from the
/// generic mutation failure while still classifying as a client error.
fn skill_install_failed(message: &str) -> AdkMutationPortError {
    AdkMutationPortError::Failed {
        status: 400,
        code: "ADK_SKILL_INSTALL_FAILED".to_owned(),
        message: message.to_owned(),
    }
}

fn install_skill(
    port: &ProductionAdkPort,
    input: &AdkMutationInput,
) -> Result<Value, AdkMutationPortError> {
    // Go maps every `InstallSkill` failure to `400 ADK_SKILL_INSTALL_FAILED`
    // with the registry message, so the skill transport owns this code rather
    // than the generic `ADK_INVALID_REQUEST` mutation failure.
    let raw_url = input
        .body
        .get("url")
        .or_else(|| input.body.get("skillUrl"))
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| skill_install_failed("skill URL is required"))?;
    let parsed = Url::parse(raw_url)
        .map_err(|_| skill_install_failed("valid http/https skill URL is required"))?;
    validate_skill_url_shape(&parsed).map_err(|message| skill_install_failed(&message))?;
    let url = raw_url.to_owned();
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
    install_skill_document(port, raw_url, &parsed, &body, &content_type)
}

/// Persist a downloaded skill document or archive.
///
/// Split out from [`install_skill`] so the download/URL-safety half and the
/// registry/filesystem half stay independently testable; the split is pure
/// refactoring, the wire behavior is unchanged.
fn install_skill_document(
    port: &ProductionAdkPort,
    raw_url: &str,
    parsed: &Url,
    body: &[u8],
    content_type: &str,
) -> Result<Value, AdkMutationPortError> {
    const MAX_SKILL_FILE_BYTES: usize = 512 << 10;
    let archive = is_skill_archive(raw_url, content_type, Some(body.len() as u64))
        || body.starts_with(b"PK\x03\x04")
        || body.starts_with(b"PK\x05\x06")
        || body.starts_with(b"PK\x07\x08");
    let (text, files) = if archive {
        extract_skill_archive(body).map_err(|message| AdkMutationPortError::Failed {
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
        let text = String::from_utf8(body.to_vec()).map_err(|_| AdkMutationPortError::Failed {
            status: 400,
            code: "ADK_SKILL_INSTALL_FAILED".to_owned(),
            message: "skill document must be UTF-8".to_owned(),
        })?;
        (text, vec![("SKILL.md".to_owned(), body.to_vec())])
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
    digest.update(body);
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
        crate::product_id::generate_uuid_v4()
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

/// Uninstall one external skill.
///
/// Go resolves the skill through the filesystem registry first, refuses
/// `source == "builtin"`, and then removes the install directory. A missing
/// skill surfaces as the registry's `file does not exist` error, which the
/// transport reports as `500 ADK_SKILL_UNINSTALL_FAILED`; Rust keeps that
/// frozen projection instead of repairing the Go status classification.
pub(super) fn uninstall_skill(
    port: &ProductionAdkPort,
    id: &str,
) -> Result<Value, AdkMutationPortError> {
    // Go syncs the builtin bundles into the skills directory during runtime
    // construction, so `Get` finds them with `source: builtin`. Rust projects
    // builtins from the catalogue instead of duplicating them into the store,
    // so the projection has to be consulted before reporting a missing skill.
    let builtin = super::super::builtin_skills(&port.tool_catalog)
        .into_iter()
        .find(|skill| skill.get("id").and_then(Value::as_str) == Some(id));
    let stored = port.store.get_skill(id).map_err(storage_mutation_failed)?;
    let payload = match stored {
        Some(stored) => decode_mutation_payload(&stored.payload_json, "skill")?,
        None => match builtin {
            Some(_) => {
                return Err(skill_uninstall_failed(
                    "builtin skills cannot be uninstalled",
                ));
            }
            None => return Err(skill_uninstall_failed("file does not exist")),
        },
    };
    let source = payload
        .get("source")
        .and_then(Value::as_str)
        .unwrap_or_default()
        .trim();
    if source.eq_ignore_ascii_case("builtin") || builtin.is_some() {
        return Err(skill_uninstall_failed("builtin skills cannot be uninstalled"));
    }
    port.store
        .delete_skill(id)
        .map_err(|error| skill_uninstall_failed(&error.to_string()))?;
    if let Some(path) = payload.get("installPath").and_then(Value::as_str)
        && let Some(parent) = std::path::Path::new(path).parent()
    {
        let _ = fs::remove_dir_all(parent);
    }
    Ok(json!({"id": id, "deleted": true}))
}

/// `500 ADK_SKILL_UNINSTALL_FAILED`, the Go skill handler's uninstall code.
fn skill_uninstall_failed(message: &str) -> AdkMutationPortError {
    AdkMutationPortError::Failed {
        status: 500,
        code: "ADK_SKILL_UNINSTALL_FAILED".to_owned(),
        message: message.to_owned(),
    }
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
