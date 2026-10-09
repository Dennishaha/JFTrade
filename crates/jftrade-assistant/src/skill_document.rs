use yaml_rust2::{Yaml, YamlEmitter, YamlLoader};

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct SkillDocumentMetadata {
    pub name: String,
    pub description: String,
    pub source: String,
    pub version: String,
    pub allowed_tools: Vec<String>,
}

#[derive(Debug, thiserror::Error)]
#[error("invalid skill document: {0}")]
pub struct SkillDocumentError(String);

pub fn parse_skill_document(document: &str) -> Result<SkillDocumentMetadata, SkillDocumentError> {
    let (metadata, _) = skill_document_parts(document)?;
    let name = text_field(&metadata["name"], "name")?;
    if name.trim().is_empty() {
        return Err(SkillDocumentError("name is required".to_owned()));
    }
    let description = text_field(&metadata["description"], "description")?;
    let source = text_field(&metadata["metadata"]["source"], "metadata.source")?;
    let version = match &metadata["metadata"]["version"] {
        Yaml::Real(value) => value.clone(),
        Yaml::Integer(value) => value.to_string(),
        value => text_field(value, "metadata.version")?,
    };
    let allowed_tools = match &metadata["allowed-tools"] {
        Yaml::BadValue | Yaml::Null => Vec::new(),
        Yaml::Array(items) => items
            .iter()
            .map(|item| text_field(item, "allowed-tools"))
            .collect::<Result<_, _>>()?,
        Yaml::String(value) => value.split_whitespace().map(str::to_owned).collect(),
        _ => {
            return Err(SkillDocumentError(
                "allowed-tools must be strings".to_owned(),
            ));
        }
    };
    Ok(SkillDocumentMetadata {
        name,
        description,
        source: if source.trim().is_empty() {
            "filesystem".to_owned()
        } else {
            source.trim().to_owned()
        },
        version: version.trim().to_owned(),
        allowed_tools,
    })
}

fn skill_document_parts(document: &str) -> Result<(Yaml, &str), SkillDocumentError> {
    let document = document.trim_start_matches('\u{feff}');
    let mut lines = document.split_inclusive('\n');
    let first = lines.next().unwrap_or_default();
    if first.trim() != "---" {
        return Err(SkillDocumentError("frontmatter is required".to_owned()));
    }
    let mut offset = first.len();
    let mut frontmatter = String::new();
    for line in lines {
        offset += line.len();
        if line.trim() == "---" {
            let documents = YamlLoader::load_from_str(&frontmatter)
                .map_err(|error| SkillDocumentError(error.to_string()))?;
            let [metadata] = documents.as_slice() else {
                return Err(SkillDocumentError(
                    "one frontmatter mapping is required".to_owned(),
                ));
            };
            if metadata.as_hash().is_none() {
                return Err(SkillDocumentError(
                    "frontmatter must be a mapping".to_owned(),
                ));
            }
            return Ok((metadata.clone(), &document[offset..]));
        }
        frontmatter.push_str(line);
    }
    Err(SkillDocumentError("frontmatter is not closed".to_owned()))
}

pub fn rewrite_skill_document_source(
    document: &str,
    source: &str,
) -> Result<String, SkillDocumentError> {
    parse_skill_document(document)?;
    let (mut header, instructions) = skill_document_parts(document)?;
    let Yaml::Hash(fields) = &mut header else {
        unreachable!()
    };
    let metadata = fields
        .entry(Yaml::String("metadata".to_owned()))
        .or_insert_with(|| Yaml::Hash(Default::default()));
    if matches!(metadata, Yaml::Null) {
        *metadata = Yaml::Hash(Default::default());
    }
    let Yaml::Hash(metadata) = metadata else {
        return Err(SkillDocumentError("metadata must be a mapping".to_owned()));
    };
    metadata.insert(
        Yaml::String("source".to_owned()),
        Yaml::String(source.to_owned()),
    );
    let mut serialized = String::new();
    YamlEmitter::new(&mut serialized)
        .dump(&header)
        .map_err(|error| SkillDocumentError(error.to_string()))?;
    // Go emits ordinary URL scalars without quotes. Only adopt that spelling
    // when a full YAML round trip preserves the same structured header.
    let plain = serialized
        .lines()
        .map(|line| {
            if line.starts_with("  source: ") {
                format!("  source: {source}")
            } else {
                line.to_owned()
            }
        })
        .collect::<Vec<_>>()
        .join("\n");
    if YamlLoader::load_from_str(&plain).ok().as_deref() == Some(std::slice::from_ref(&header)) {
        serialized = plain;
    }
    Ok(format!("{serialized}\n---\n{instructions}"))
}

fn text_field(value: &Yaml, field: &str) -> Result<String, SkillDocumentError> {
    match value {
        Yaml::String(value) => Ok(value.clone()),
        Yaml::BadValue | Yaml::Null => Ok(String::new()),
        _ => Err(SkillDocumentError(format!("{field} must be a string"))),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn downloaded_skill_source_rewrite_preserves_numeric_version_tools_and_instructions() {
        let raw = "---\nname: downloaded\ndescription: |\n  Multi line.\n  Details.\nallowed-tools: [http.fetch]\nmetadata:\n  source: builtin\n  version: 2026.06\n  custom: preserved\n---\nname: instruction text\nKeep this body.";
        let rewritten = rewrite_skill_document_source(raw, "https://example.com/plain.md").unwrap();
        let parsed = parse_skill_document(&rewritten).unwrap();
        assert_eq!(parsed.source, "https://example.com/plain.md");
        assert_eq!(parsed.version, "2026.06");
        assert_eq!(parsed.description, "Multi line.\nDetails.\n");
        assert_eq!(parsed.allowed_tools, ["http.fetch"]);
        let (metadata, body) = skill_document_parts(&rewritten).unwrap();
        assert_eq!(metadata["metadata"]["custom"].as_str(), Some("preserved"));
        assert_eq!(body, "name: instruction text\nKeep this body.");
        assert!(rewritten.contains("source: https://example.com/plain.md"));
    }

    #[test]
    fn skill_frontmatter_preserves_nested_metadata_lists_and_multiline_description() {
        let document = "---\nname: example-skill\ndescription: |\n  Line one.\n  Line two.\nallowed-tools: [system.status, market.snapshot]\nmetadata:\n  source: https://example.com/SKILL.md\n  version: '2'\n---\nname: body-must-not-replace-frontmatter\n";
        let parsed = parse_skill_document(document).unwrap();
        assert_eq!(parsed.name, "example-skill");
        assert_eq!(parsed.description, "Line one.\nLine two.\n");
        assert_eq!(parsed.source, "https://example.com/SKILL.md");
        assert_eq!(parsed.version, "2");
        assert_eq!(
            parsed.allowed_tools,
            vec!["system.status", "market.snapshot"]
        );
        let parsed =
            parse_skill_document("---\nname: filesystem-skill\n---\nInstructions.").unwrap();
        assert_eq!(parsed.source, "filesystem");
        assert!(parsed.allowed_tools.is_empty());
    }

    #[test]
    fn skill_frontmatter_rejects_duplicate_fields_wrong_types_and_unclosed_headers() {
        for invalid in [
            "name: outside-header",
            "---\nname: not-closed",
            "---\nname: one\nname: two\n---",
            "---\nname: [wrong]\n---",
            "---\nname: example\nallowed-tools: [42]\n---",
            "---\nname: example\nmetadata:\n  source: [wrong]\n---",
        ] {
            assert!(parse_skill_document(invalid).is_err(), "{invalid}");
        }
    }
}
