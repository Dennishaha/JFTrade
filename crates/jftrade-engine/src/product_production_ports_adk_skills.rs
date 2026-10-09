use super::*;
use sha2::Digest;
use std::fs;
use std::io::Read;
use std::path::{Component, Path};

#[derive(Debug, thiserror::Error)]
pub(crate) enum SkillRegistryError {
    #[error(transparent)]
    Io(#[from] std::io::Error),
    #[error(transparent)]
    Document(#[from] jftrade_assistant::SkillDocumentError),
    #[error("{0}")]
    Invalid(String),
}

pub(crate) fn skills_root(port: &ProductionAdkPort) -> PathBuf {
    std::env::var_os("JFTRADE_ADK_SKILLS")
        .map(PathBuf::from)
        .unwrap_or_else(|| {
            port.settings_path
                .parent()
                .unwrap_or(Path::new("."))
                .join("skills")
        })
}

pub(crate) fn filesystem_skills(
    port: &ProductionAdkPort,
) -> Result<Vec<Value>, SkillRegistryError> {
    let entries = match fs::read_dir(skills_root(port)) {
        Ok(entries) => entries,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(Vec::new()),
        Err(error) => return Err(error.into()),
    };
    let mut skills = Vec::new();
    for entry in entries {
        let entry = entry?;
        if !entry.file_type()?.is_dir() {
            continue;
        }
        let Some(id) = entry.file_name().to_str().map(str::to_owned) else {
            continue;
        };
        if id.starts_with('.') {
            continue;
        }
        if let Some(skill) = filesystem_skill(port, &id)? {
            skills.push(skill);
        }
    }
    Ok(skills)
}

pub(crate) fn filesystem_skill(
    port: &ProductionAdkPort,
    id: &str,
) -> Result<Option<Value>, SkillRegistryError> {
    let mut components = Path::new(id).components();
    if !matches!(components.next(), Some(Component::Normal(_))) || components.next().is_some() {
        return Ok(None);
    }
    let path = skills_root(port).join(id).join("SKILL.md");
    let file = match fs::File::open(&path) {
        Ok(file) => file,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(None),
        Err(error) => return Err(error.into()),
    };
    let modified = file.metadata()?.modified()?;
    let mut raw = Vec::new();
    file.take(512 * 1024 + 1).read_to_end(&mut raw)?;
    if raw.len() > 512 * 1024 {
        return Err(SkillRegistryError::Invalid(
            "skill file exceeds 524288 bytes".to_owned(),
        ));
    }
    let text = std::str::from_utf8(&raw)
        .map_err(|error| SkillRegistryError::Invalid(error.to_string()))?;
    let document = jftrade_assistant::parse_skill_document(text)?;
    if document.name != id {
        return Err(SkillRegistryError::Invalid(
            "skill name does not match its install directory".to_owned(),
        ));
    }
    let builtin = document.source.eq_ignore_ascii_case("builtin");
    let available = port.tool_catalog.values();
    let unknown = document
        .allowed_tools
        .iter()
        .filter(|tool| {
            !available
                .iter()
                .any(|entry| entry.get("id").and_then(Value::as_str) == Some(tool.as_str()))
        })
        .cloned()
        .collect::<Vec<_>>();
    let valid = builtin || unknown.is_empty();
    let modified = time::OffsetDateTime::from(modified)
        .format(&time::format_description::well_known::Rfc3339)
        .map_err(|error| SkillRegistryError::Invalid(error.to_string()))?;
    let digest = sha2::Sha256::digest(&raw)
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect::<String>();
    Ok(Some(
        json!({"id":document.name, "displayName":document.name,
        "description":document.description, "source":document.source, "installPath":path,
        "enabled":true, "builtin":builtin, "tools":document.allowed_tools, "version":document.version,
        "contentHash":digest, "validationStatus":if valid {"VALID"} else {"INVALID"},
        "validationError":if valid {String::new()} else {format!("unknown ADK tools: {}",unknown.join(", "))},
        "createdAt":modified, "updatedAt":modified}),
    ))
}
