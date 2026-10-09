use yaml_rust2::{Yaml, YamlLoader};

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
    let mut lines = document.trim_start_matches('\u{feff}').lines();
    if lines.next().map(str::trim) != Some("---") {
        return Err(SkillDocumentError("frontmatter is required".to_owned()));
    }
    let mut frontmatter = String::new();
    let mut closed = false;
    for line in lines {
        if line.trim() == "---" {
            closed = true;
            break;
        }
        frontmatter.push_str(line);
        frontmatter.push('\n');
    }
    if !closed {
        return Err(SkillDocumentError("frontmatter is not closed".to_owned()));
    }
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
    let name = text_field(&metadata["name"], "name")?;
    if name.trim().is_empty() {
        return Err(SkillDocumentError("name is required".to_owned()));
    }
    let description = text_field(&metadata["description"], "description")?;
    let source = text_field(&metadata["metadata"]["source"], "metadata.source")?;
    let version = text_field(&metadata["metadata"]["version"], "metadata.version")?;
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
