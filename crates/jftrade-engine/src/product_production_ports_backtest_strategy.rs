//! Strategy-definition resolution and backtest request parsing.

use jftrade_store_sqlite::{
    StoredStrategyDefinition, StoredStrategyVersion, StrategyDefinitionStore,
    StrategyDefinitionStoreError,
};
use serde_json::Value;

use super::product_production_ports_backtest_parse::{
    ParsedBacktestStart, parse_end_timestamp, parse_start_timestamp,
};
use crate::product::product_backtests_write_port::BacktestsWritePortError;

#[derive(Clone, Debug)]
struct ResolvedStrategyDefinition {
    id: String,
    version: String,
    source_format: String,
    symbol: String,
    interval: String,
    script: String,
}

/// Resolve a definition-backed request before any run row is persisted. The
/// public request stores only the definition id/version; the worker adapter
/// needs the immutable source and binding metadata in its private execution
/// payload. Explicit source requests remain self-contained and bypass the
/// store, preserving fixture/research callers.
pub(super) fn resolve_strategy_payload(
    payload: &Value,
    store: &StrategyDefinitionStore,
) -> Result<Value, BacktestsWritePortError> {
    let Some(source) = request_text(
        payload,
        &[
            "strategyScript",
            "strategySource",
            "strategy_source",
            "source",
            "script",
        ],
        "strategy source",
    )?
    else {
        return resolve_definition_payload(payload, store);
    };
    if source.is_empty() {
        return Err(BacktestsWritePortError::Failed(
            "strategy source is empty".to_owned(),
        ));
    }
    Ok(payload.clone())
}

fn resolve_definition_payload(
    payload: &Value,
    store: &StrategyDefinitionStore,
) -> Result<Value, BacktestsWritePortError> {
    resolve_definition_payload_with(payload, store)
}

trait DefinitionLookup {
    fn get_definition(
        &self,
        id: &str,
    ) -> Result<Option<StoredStrategyDefinition>, StrategyDefinitionStoreError>;

    fn get_version(
        &self,
        definition_id: &str,
        version: &str,
    ) -> Result<Option<StoredStrategyVersion>, StrategyDefinitionStoreError>;
}

impl DefinitionLookup for StrategyDefinitionStore {
    fn get_definition(
        &self,
        id: &str,
    ) -> Result<Option<StoredStrategyDefinition>, StrategyDefinitionStoreError> {
        self.get_definition(id, false)
    }

    fn get_version(
        &self,
        definition_id: &str,
        version: &str,
    ) -> Result<Option<StoredStrategyVersion>, StrategyDefinitionStoreError> {
        self.get_version(definition_id, version)
    }
}

fn resolve_definition_payload_with<L: DefinitionLookup>(
    payload: &Value,
    store: &L,
) -> Result<Value, BacktestsWritePortError> {
    let definition_id =
        request_text(payload, &["definitionId"], "definitionId")?.ok_or_else(|| {
            BacktestsWritePortError::BadRequest("definitionId is required".to_owned())
        })?;
    let requested_version = requested_definition_version(payload)?;
    let definition = store
        .get_definition(&definition_id)
        .map_err(map_strategy_resolution_error)?
        .ok_or_else(|| {
            BacktestsWritePortError::StrategyNotFound("strategy resource not found".to_owned())
        })?;
    let resolved = if let Some(version) = requested_version {
        let stored = store
            .get_version(&definition_id, &version)
            .map_err(map_strategy_resolution_error)?
            .ok_or_else(|| {
                BacktestsWritePortError::Conflict(format!(
                    "strategy definition version {version} is not available"
                ))
            })?;
        ResolvedStrategyDefinition {
            id: stored.definition_id,
            version: stored.version,
            source_format: stored.source_format,
            symbol: stored.symbol,
            interval: stored.interval,
            script: stored.script,
        }
    } else {
        ResolvedStrategyDefinition {
            id: definition.id,
            version: definition.version,
            source_format: definition.source_format,
            symbol: definition.symbol,
            interval: definition.interval,
            script: definition.script,
        }
    };
    if resolved.script.trim().is_empty() {
        return Err(BacktestsWritePortError::Failed(format!(
            "strategy definition {} has an empty source",
            resolved.id
        )));
    }
    if !matches!(
        resolved.source_format.trim().to_ascii_lowercase().as_str(),
        "pine" | "pine-v6" | "pinev6" | "pine-pinets"
    ) {
        return Err(BacktestsWritePortError::BadRequest(format!(
            "unsupported strategy source format: {}",
            resolved.source_format
        )));
    }
    let mut resolved_payload = payload.clone();
    let object = resolved_payload.as_object_mut().ok_or_else(|| {
        BacktestsWritePortError::BadRequest("invalid backtest request".to_owned())
    })?;
    object.insert("strategyScript".to_owned(), Value::String(resolved.script));
    object.insert("definitionId".to_owned(), Value::String(resolved.id));
    object.insert(
        "definitionVersion".to_owned(),
        Value::String(resolved.version),
    );
    object.insert(
        "sourceFormat".to_owned(),
        Value::String(resolved.source_format),
    );
    if !object
        .get("symbol")
        .and_then(Value::as_str)
        .is_some_and(|value| !value.trim().is_empty())
    {
        object.insert("symbol".to_owned(), Value::String(resolved.symbol));
    }
    if !object
        .get("interval")
        .and_then(Value::as_str)
        .is_some_and(|value| !value.trim().is_empty())
    {
        object.insert("interval".to_owned(), Value::String(resolved.interval));
    }
    Ok(resolved_payload)
}

fn requested_definition_version(
    payload: &Value,
) -> Result<Option<String>, BacktestsWritePortError> {
    let definition_version = optional_request_text(payload, "definitionVersion")?;
    let version = optional_request_text(payload, "version")?;
    if let (Some(definition_version), Some(version)) = (&definition_version, &version)
        && definition_version != version
    {
        return Err(BacktestsWritePortError::Conflict(
            "definitionVersion and version do not match".to_owned(),
        ));
    }
    Ok(definition_version.or(version))
}

fn optional_request_text(
    payload: &Value,
    field: &str,
) -> Result<Option<String>, BacktestsWritePortError> {
    let Some(object) = payload.as_object() else {
        return Err(BacktestsWritePortError::BadRequest(
            "invalid backtest request".to_owned(),
        ));
    };
    let Some(value) = object.get(field) else {
        return Ok(None);
    };
    let text = value
        .as_str()
        .ok_or_else(|| BacktestsWritePortError::BadRequest(format!("{field} must be a string")))?;
    let text = text.trim();
    if text.is_empty() {
        return Ok(None);
    }
    Ok(Some(text.to_owned()))
}

fn request_text(
    payload: &Value,
    names: &[&str],
    field: &str,
) -> Result<Option<String>, BacktestsWritePortError> {
    let Some(object) = payload.as_object() else {
        return Err(BacktestsWritePortError::BadRequest(
            "invalid backtest request".to_owned(),
        ));
    };
    let Some(value) = names.iter().find_map(|name| object.get(*name)) else {
        return Ok(None);
    };
    let text = value
        .as_str()
        .ok_or_else(|| BacktestsWritePortError::BadRequest(format!("{field} must be a string")))?;
    let text = text.trim();
    if text.is_empty() {
        return Err(BacktestsWritePortError::BadRequest(format!(
            "{field} must not be empty"
        )));
    }
    Ok(Some(text.to_owned()))
}

fn map_strategy_resolution_error(error: StrategyDefinitionStoreError) -> BacktestsWritePortError {
    match error {
        StrategyDefinitionStoreError::NotFound => {
            BacktestsWritePortError::StrategyNotFound("strategy resource not found".to_owned())
        }
        other => BacktestsWritePortError::Unavailable(format!(
            "strategy definition store is unavailable: {other}"
        )),
    }
}

pub(super) fn parse_start_request(
    payload: &Value,
) -> Result<ParsedBacktestStart, BacktestsWritePortError> {
    let object = payload.as_object().ok_or_else(|| {
        BacktestsWritePortError::BadRequest("invalid backtest request".to_owned())
    })?;
    let corpus_case = payload
        .get("corpus")
        .and_then(|value| value.get("cases"))
        .and_then(Value::as_array)
        .and_then(|cases| cases.first());
    let text = |name: &str| {
        object
            .get(name)
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
    };
    if payload.get("corpus").is_none() && text("definitionId").is_none() {
        return Err(BacktestsWritePortError::BadRequest(
            "definitionId is required".to_owned(),
        ));
    }
    let symbol = if let Some(symbol) = text("symbol") {
        symbol.to_owned()
    } else if let (Some(market), Some(code)) = (text("market"), text("code")) {
        format!("{market}.{code}")
    } else if let Some(symbol) = corpus_case
        .and_then(|case| case.get("symbol"))
        .and_then(Value::as_str)
    {
        symbol.to_owned()
    } else {
        return Err(BacktestsWritePortError::BadRequest(
            "symbol is required".to_owned(),
        ));
    };
    let interval = text("interval").unwrap_or("1m").to_owned();
    if !matches!(
        interval.as_str(),
        "1m" | "5m" | "15m" | "30m" | "1h" | "1d" | "1w" | "1mo"
    ) {
        return Err(BacktestsWritePortError::BadRequest(
            "invalid interval".to_owned(),
        ));
    }
    let rehab_type = text("rehabType").unwrap_or("forward").to_ascii_lowercase();
    if !matches!(rehab_type.as_str(), "forward" | "backward" | "none") {
        return Err(BacktestsWritePortError::BadRequest(
            "invalid rehabType".to_owned(),
        ));
    }
    let session_scope = if object
        .get("useExtendedHours")
        .and_then(Value::as_bool)
        .unwrap_or(false)
    {
        "extended"
    } else {
        "regular"
    }
    .to_owned();
    let start = text("startTime")
        .or_else(|| text("startDate"))
        .map(parse_start_timestamp)
        .transpose()?;
    let end = text("endTime")
        .or_else(|| text("endDate"))
        .map(parse_end_timestamp)
        .transpose()?;
    let (start_time_ms, end_time_ms) = match (start, end) {
        (Some(start), Some(end)) => (start, end),
        _ => {
            let candles = corpus_case
                .and_then(|case| case.get("candles"))
                .and_then(Value::as_array)
                .filter(|candles| !candles.is_empty())
                .ok_or_else(|| {
                    BacktestsWritePortError::BadRequest(
                        "startTime and endTime are required".to_owned(),
                    )
                })?;
            let start = candles
                .first()
                .and_then(|candle| candle.get("start"))
                .and_then(Value::as_str)
                .map(parse_start_timestamp)
                .transpose()?
                .ok_or_else(|| {
                    BacktestsWritePortError::BadRequest("invalid candle start".to_owned())
                })?;
            let end = candles
                .last()
                .and_then(|candle| candle.get("end"))
                .and_then(Value::as_str)
                .map(parse_end_timestamp)
                .transpose()?
                .ok_or_else(|| {
                    BacktestsWritePortError::BadRequest("invalid candle end".to_owned())
                })?;
            (start, end)
        }
    };
    if end_time_ms <= start_time_ms {
        return Err(BacktestsWritePortError::BadRequest(
            "endTime must be after startTime".to_owned(),
        ));
    }
    Ok(ParsedBacktestStart {
        symbol,
        interval,
        rehab_type,
        session_scope,
        start_time_ms,
        end_time_ms,
    })
}

#[cfg(test)]
mod definition_resolution_tests {
    use super::*;
    use serde_json::json;

    #[derive(Default)]
    struct FakeDefinitionStore {
        definition: Option<StoredStrategyDefinition>,
        version: Option<StoredStrategyVersion>,
    }

    impl DefinitionLookup for FakeDefinitionStore {
        fn get_definition(
            &self,
            _id: &str,
        ) -> Result<Option<StoredStrategyDefinition>, StrategyDefinitionStoreError> {
            Ok(self.definition.clone())
        }

        fn get_version(
            &self,
            _definition_id: &str,
            _version: &str,
        ) -> Result<Option<StoredStrategyVersion>, StrategyDefinitionStoreError> {
            Ok(self.version.clone())
        }
    }

    fn definition() -> StoredStrategyDefinition {
        StoredStrategyDefinition {
            id: "strategy-1".to_owned(),
            name: "Fixture".to_owned(),
            version: "1.0.0".to_owned(),
            description: String::new(),
            runtime: "pine-pinets".to_owned(),
            source_format: "pine-v6".to_owned(),
            symbol: "US.AAPL".to_owned(),
            interval: "1m".to_owned(),
            script: "strategy('fixture')".to_owned(),
            visual_model_json: "{}".to_owned(),
            created_at: String::new(),
            updated_at: String::new(),
            deleted_at: None,
        }
    }

    #[test]
    fn definition_resolution_injects_source_binding_and_version() {
        let resolved = resolve_definition_payload_with(
            &json!({"definitionId": "strategy-1", "definitionVersion": ""}),
            &FakeDefinitionStore {
                definition: Some(definition()),
                ..FakeDefinitionStore::default()
            },
        )
        .expect("resolve definition");
        assert_eq!(resolved["strategyScript"], "strategy('fixture')");
        assert_eq!(resolved["symbol"], "US.AAPL");
        assert_eq!(resolved["interval"], "1m");
        assert_eq!(resolved["definitionVersion"], "1.0.0");
    }

    #[test]
    fn definition_resolution_rejects_missing_definition_and_version_conflict() {
        let missing = resolve_definition_payload_with(
            &json!({"definitionId": "missing"}),
            &FakeDefinitionStore::default(),
        )
        .expect_err("missing definition");
        assert!(matches!(
            missing,
            BacktestsWritePortError::StrategyNotFound(_)
        ));

        let conflict = resolve_definition_payload_with(
            &json!({"definitionId": "strategy-1", "definitionVersion": "2.0.0"}),
            &FakeDefinitionStore {
                definition: Some(definition()),
                ..FakeDefinitionStore::default()
            },
        )
        .expect_err("missing requested version");
        assert!(matches!(conflict, BacktestsWritePortError::Conflict(_)));
    }

    #[test]
    fn definition_resolution_rejects_empty_or_unsupported_source() {
        let mut empty = definition();
        empty.script.clear();
        let error = resolve_definition_payload_with(
            &json!({"definitionId": "strategy-1"}),
            &FakeDefinitionStore {
                definition: Some(empty),
                ..FakeDefinitionStore::default()
            },
        )
        .expect_err("empty source");
        assert!(
            matches!(error, BacktestsWritePortError::Failed(message) if message.contains("empty source"))
        );

        let mut unsupported = definition();
        unsupported.source_format = "javascript".to_owned();
        let error = resolve_definition_payload_with(
            &json!({"definitionId": "strategy-1"}),
            &FakeDefinitionStore {
                definition: Some(unsupported),
                ..FakeDefinitionStore::default()
            },
        )
        .expect_err("unsupported source");
        assert!(
            matches!(error, BacktestsWritePortError::BadRequest(message) if message.contains("unsupported strategy source format"))
        );
    }
}
