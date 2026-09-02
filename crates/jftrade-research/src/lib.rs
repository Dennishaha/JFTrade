#![forbid(unsafe_code)]

mod catalog;
mod definition;

pub use catalog::{ScreenCatalogError, screen_catalog};
pub use definition::{DefinitionFieldError, normalize_definition_v2};

use serde::{Deserialize, Serialize};
use serde_json::Value;
use thiserror::Error;

pub const QUERY_SCHEMA_VERSION: u32 = 2;
pub const MAX_PRESET_NAME_CHARS: usize = 80;

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ScreenPreset {
    pub preset_id: String,
    pub name: String,
    pub query_schema_version: u32,
    pub definition: Value,
    pub revision: u64,
}

#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PresetUpdate {
    pub name: Option<String>,
    pub definition: Option<Value>,
    pub expected_revision: u64,
}

#[derive(Clone, Debug, Eq, PartialEq, Error)]
pub enum ResearchError {
    #[error("preset id is required")]
    MissingPresetId,
    #[error("name is required")]
    MissingName,
    #[error("name must not exceed 80 characters")]
    NameTooLong,
    #[error("expectedRevision must be positive")]
    InvalidRevision,
    #[error("name or definition is required")]
    EmptyUpdate,
    #[error("preset revision conflict")]
    Conflict,
    #[error("definition must be a JSON object")]
    InvalidDefinition,
}

pub fn create_preset(
    preset_id: &str,
    name: &str,
    definition: Value,
) -> Result<ScreenPreset, ResearchError> {
    let preset_id = normalize_required(preset_id).ok_or(ResearchError::MissingPresetId)?;
    Ok(ScreenPreset {
        preset_id,
        name: normalize_name(name)?,
        query_schema_version: QUERY_SCHEMA_VERSION,
        definition: normalize_definition(definition)?,
        revision: 1,
    })
}

pub fn update_preset(
    current: &ScreenPreset,
    update: PresetUpdate,
) -> Result<ScreenPreset, ResearchError> {
    if update.expected_revision == 0 {
        return Err(ResearchError::InvalidRevision);
    }
    if update.name.is_none() && update.definition.is_none() {
        return Err(ResearchError::EmptyUpdate);
    }
    if current.revision != update.expected_revision {
        return Err(ResearchError::Conflict);
    }
    Ok(ScreenPreset {
        preset_id: current.preset_id.clone(),
        name: match update.name {
            Some(name) => normalize_name(&name)?,
            None => current.name.clone(),
        },
        query_schema_version: QUERY_SCHEMA_VERSION,
        definition: match update.definition {
            Some(definition) => normalize_definition(definition)?,
            None => current.definition.clone(),
        },
        revision: current.revision + 1,
    })
}

fn normalize_required(value: &str) -> Option<String> {
    let normalized = value.trim();
    (!normalized.is_empty()).then(|| normalized.to_owned())
}

fn normalize_name(value: &str) -> Result<String, ResearchError> {
    let normalized = normalize_required(value).ok_or(ResearchError::MissingName)?;
    if normalized.chars().count() > MAX_PRESET_NAME_CHARS {
        return Err(ResearchError::NameTooLong);
    }
    Ok(normalized)
}

fn normalize_definition(value: Value) -> Result<Value, ResearchError> {
    value
        .as_object()
        .is_some()
        .then_some(value)
        .ok_or(ResearchError::InvalidDefinition)
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;

    fn valid_definition() -> Value {
        json!({
            "market": "US",
            "pool": {},
            "catalogVersion": "futu-stock-screen-v1",
            "querySchemaVersion": 2
        })
    }

    #[test]
    fn update_is_revision_guarded_and_preserves_schema_owner() {
        let preset =
            create_preset(" preset-1 ", " Momentum ", valid_definition()).expect("valid preset");
        let updated = update_preset(
            &preset,
            PresetUpdate {
                name: Some("Value".into()),
                definition: None,
                expected_revision: 1,
            },
        )
        .expect("matching revision");
        assert_eq!(updated.name, "Value");
        assert_eq!(updated.revision, 2);
        assert_eq!(updated.query_schema_version, QUERY_SCHEMA_VERSION);
        assert_eq!(
            update_preset(
                &updated,
                PresetUpdate {
                    name: None,
                    definition: Some(valid_definition()),
                    expected_revision: 1,
                }
            ),
            Err(ResearchError::Conflict)
        );
    }

    #[test]
    fn names_use_character_count_and_definitions_fail_closed() {
        assert!(create_preset("id", &"界".repeat(80), valid_definition()).is_ok());
        assert_eq!(
            create_preset("id", &"界".repeat(81), valid_definition()),
            Err(ResearchError::NameTooLong)
        );
        assert_eq!(
            create_preset("id", "name", json!([])),
            Err(ResearchError::InvalidDefinition)
        );
    }
}
