//! Read projections for authoritative strategy definitions and immutable versions.

use super::ProductionStrategyDefinitionPort;
use crate::product::{
    StrategyDefinitionPreview, StrategyDefinitionSnapshotError, StrategyDefinitionSnapshotPort,
};
use serde_json::{Value, json};

impl StrategyDefinitionSnapshotPort for ProductionStrategyDefinitionPort {
    fn list(&self) -> Result<Vec<Value>, StrategyDefinitionSnapshotError> {
        let definitions = self
            .store
            .list_definitions(false)
            .map_err(|e| StrategyDefinitionSnapshotError::Unavailable(e.to_string()))?;
        definitions
            .into_iter()
            .map(|d| {
                serde_json::to_value(&d).map_err(|error| {
                    StrategyDefinitionSnapshotError::Unavailable(error.to_string())
                })
            })
            .collect::<Result<Vec<_>, _>>()
    }

    fn get(
        &self,
        definition_id: &str,
        preview: &StrategyDefinitionPreview,
    ) -> Result<Option<Value>, StrategyDefinitionSnapshotError> {
        let def = self
            .store
            .get_definition(definition_id, true)
            .map_err(|e| StrategyDefinitionSnapshotError::Unavailable(e.to_string()))?;
        let Some(mut value) = def
            .map(|d| {
                serde_json::to_value(&d)
                    .map_err(|error| StrategyDefinitionSnapshotError::Unavailable(error.to_string()))
            })
            .transpose()?
        else {
            return Ok(None);
        };

        if let Some(obj) = value.as_object_mut() {
            let script = obj.get("script").and_then(Value::as_str).unwrap_or_default();
            let validation = jftrade_strategy::pinespec::validate_script(script, true, false);
            let symbol = preview
                .symbol
                .as_deref()
                .or_else(|| obj.get("symbol").and_then(Value::as_str))
                .unwrap_or_default();
            let interval = preview
                .interval
                .clone()
                .or_else(|| obj.get("interval").and_then(Value::as_str).map(str::to_owned))
                .unwrap_or_else(|| "5m".to_owned());
            let warmup_bars = validation
                .requirements
                .as_ref()
                .map(|r| {
                    r.derived_warmup_bars_with_session(symbol, &interval, preview.use_extended_hours)
                })
                .unwrap_or(0);
            obj.insert("derivedWarmupBars".to_owned(), json!(warmup_bars));
            obj.insert("derivedWarmupInterval".to_owned(), json!(interval));
        }
        Ok(Some(value))
    }

    fn versions(
        &self,
        definition_id: &str,
    ) -> Result<Option<Vec<Value>>, StrategyDefinitionSnapshotError> {
        // Go's version store answers found=false for a definition id that was
        // never saved, while a soft-deleted definition still lists its
        // history.  The existence probe therefore has to include deleted rows
        // before the immutable version rows are read, otherwise an unknown id
        // would answer 200 with an empty list instead of the documented 404.
        let definition = self
            .store
            .get_definition(definition_id, true)
            .map_err(|e| StrategyDefinitionSnapshotError::Unavailable(e.to_string()))?;
        let Some(definition) = definition else {
            return Ok(None);
        };
        let versions = self
            .store
            .list_versions(definition_id)
            .map_err(|e| StrategyDefinitionSnapshotError::Unavailable(e.to_string()))?;
        Ok(Some(
            versions
                .into_iter()
                .map(|v| {
                    project_version(&v, Some(&definition.version))
                })
                .collect::<Result<Vec<_>, _>>()?,
        ))
    }

    fn version(
        &self,
        definition_id: &str,
        version: &str,
    ) -> Result<Option<Value>, StrategyDefinitionSnapshotError> {
        let ver = self
            .store
            .get_version(definition_id, version)
            .map_err(|e| StrategyDefinitionSnapshotError::Unavailable(e.to_string()))?;
        let current_version = self
            .store
            .get_definition(definition_id, true)
            .map_err(|e| StrategyDefinitionSnapshotError::Unavailable(e.to_string()))?
            .map(|definition| definition.version);
        ver.map(|v| project_version(&v, current_version.as_deref()))
        .transpose()
    }
}

fn project_version(
    version: &jftrade_store_sqlite::StoredStrategyVersion,
    current_version: Option<&str>,
) -> Result<Value, StrategyDefinitionSnapshotError> {
    let mut value = serde_json::to_value(version)
        .map_err(|error| StrategyDefinitionSnapshotError::Unavailable(error.to_string()))?;
    value["isCurrent"] = json!(current_version == Some(version.version.as_str()));
    Ok(value)
}
