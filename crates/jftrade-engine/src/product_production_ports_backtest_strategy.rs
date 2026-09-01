//! Strategy-definition resolution and backtest request parsing.

use jftrade_store_sqlite::{
    StoredStrategyDefinition, StoredStrategyVersion, StrategyDefinitionStore,
    StrategyDefinitionStoreError,
};
use serde_json::Value;

use super::product_production_ports_backtest_parse::{
    ParsedBacktestStart, normalize_execution_model_name, parse_end_timestamp,
    parse_start_timestamp,
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
    let raw_symbol = text("symbol")
        .or_else(|| corpus_case.and_then(|case| case.get("symbol")).and_then(Value::as_str))
        .unwrap_or("");
    let (_resolved_market, symbol) = normalize_start_instrument(
        text("market").unwrap_or(""),
        raw_symbol,
        text("code").unwrap_or(""),
    )?;
    let execution_model = match object.get("executionModel") {
        None => normalize_execution_model_name("")?,
        Some(value) => {
            let value = value.as_str().ok_or_else(|| {
                BacktestsWritePortError::BadRequest("executionModel must be a string".to_owned())
            })?;
            normalize_execution_model_name(value)?
        }
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
        execution_model,
        start_time_ms,
        end_time_ms,
    })
}

/// Mirror pkg/market.ParseInstrument for the production backtest boundary.
/// Keeping this normalization here ensures a run and its local K-line lookup
/// use exactly one canonical PREFIX.CODE identity.
fn normalize_start_instrument(
    market: &str,
    symbol: &str,
    code: &str,
) -> Result<(String, String), BacktestsWritePortError> {
    let market = market.trim().to_ascii_uppercase();
    let (resolved_market, preferred_prefix) = match market.as_str() {
        "" => (String::new(), String::new()),
        "CN" => ("CN".to_owned(), String::new()),
        "CNSH" => ("CN".to_owned(), "SH".to_owned()),
        "CNSZ" => ("CN".to_owned(), "SZ".to_owned()),
        "US" | "HK" | "SH" | "SZ" | "SG" | "JP" | "AU" | "MY" | "CA" => {
            let resolved = if matches!(market.as_str(), "SH" | "SZ") {
                "CN"
            } else {
                market.as_str()
            };
            (resolved.to_owned(), market.clone())
        }
        _ => {
            return Err(BacktestsWritePortError::BadRequest(format!(
                "unsupported market {market:?}"
            )));
        }
    };
    let normalized_symbol = symbol.trim().to_ascii_uppercase().replace(':', ".");
    let normalized_code = code.trim().to_ascii_uppercase();
    if normalized_symbol.is_empty() && normalized_code.is_empty() {
        return Err(BacktestsWritePortError::BadRequest(
            "symbol or code is required".to_owned(),
        ));
    }
    if let Some((prefix, value)) = normalized_symbol.split_once('.') {
        if prefix.is_empty() || value.trim().is_empty() || value.contains('.') {
            return Err(BacktestsWritePortError::BadRequest(format!(
                "symbol {normalized_symbol:?} must be in MARKET.CODE form"
            )));
        }
        let prefix = prefix.to_owned();
        let value = value.to_owned();
        let (parsed_market, parsed_prefix) = match prefix.as_str() {
            "US" | "HK" | "SG" | "JP" | "AU" | "MY" | "CA" => {
                (prefix.clone(), prefix.clone())
            }
            "SH" => ("CN".to_owned(), "SH".to_owned()),
            "SZ" => ("CN".to_owned(), "SZ".to_owned()),
            "CNSH" => ("CN".to_owned(), "SH".to_owned()),
            "CNSZ" => ("CN".to_owned(), "SZ".to_owned()),
            _ => {
                return Err(BacktestsWritePortError::BadRequest(format!(
                    "unsupported market {prefix:?}"
                )));
            }
        };
        if !normalized_code.is_empty() && normalized_code != value {
            return Err(BacktestsWritePortError::BadRequest(
                "code does not match symbol".to_owned(),
            ));
        }
        if !resolved_market.is_empty()
            && !(resolved_market == parsed_market
                || (resolved_market == "CN" && parsed_market == "CN"))
        {
            return Err(BacktestsWritePortError::BadRequest(
                "market does not match symbol".to_owned(),
            ));
        }
        return Ok((parsed_market, format!("{parsed_prefix}.{value}")));
    }
    if !normalized_code.is_empty() && normalized_code != normalized_symbol {
        return Err(BacktestsWritePortError::BadRequest(
            "code does not match symbol".to_owned(),
        ));
    }
    let chosen_code = if normalized_symbol.is_empty() {
        normalized_code
    } else {
        normalized_symbol
    };
    if resolved_market.is_empty() {
        return Err(BacktestsWritePortError::BadRequest(
            "market is required when symbol has no market prefix".to_owned(),
        ));
    }
    if preferred_prefix.is_empty() {
        return Err(BacktestsWritePortError::BadRequest(format!(
            "market {market:?} requires an exchange-qualified symbol like SH.600519 or SZ.000001"
        )));
    }
    if chosen_code.chars().any(|ch| ch.is_whitespace() || ch == '.') {
        return Err(BacktestsWritePortError::BadRequest(
            "invalid symbol code".to_owned(),
        ));
    }
    Ok((resolved_market, format!("{preferred_prefix}.{chosen_code}")))
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

#[cfg(test)]
mod execution_model_tests {
    use super::*;
    use serde_json::{Value, json};

    fn request_with_execution_model(value: Option<Value>) -> Value {
        let mut request = json!({
            "corpus": {
                "cases": [{
                    "symbol": "US.AAPL",
                    "candles": [{
                        "start": "2026-08-01T00:00:00Z",
                        "end": "2026-08-01T00:01:00Z"
                    }]
                }]
            }
        });
        if let Some(value) = value {
            request["executionModel"] = value;
        }
        request
    }

    #[test]
    fn execution_model_defaults_and_normalizes_ascii_case() {
        let omitted = parse_start_request(&request_with_execution_model(None))
            .expect("omitted execution model");
        assert_eq!(omitted.execution_model, "conservative-bar-v1");

        let mixed_case = parse_start_request(&request_with_execution_model(Some(json!(
            "  CONSERVATIVE-BAR-V1  "
        ))))
        .expect("mixed-case execution model");
        assert_eq!(mixed_case.execution_model, "conservative-bar-v1");
    }

    #[test]
    fn execution_model_rejects_unsupported_name_with_original_value() {
        let error = parse_start_request(&request_with_execution_model(Some(json!(
            "  optimistic  "
        ))))
        .expect_err("unsupported execution model");
        assert_eq!(
            error,
            BacktestsWritePortError::BadRequest(
                "unsupported backtest executionModel:   optimistic  ".to_owned()
            )
        );
    }

    #[test]
    fn execution_model_rejects_non_string_value_as_bad_request() {
        let error = parse_start_request(&request_with_execution_model(Some(json!(42))))
            .expect_err("non-string execution model");
        assert_eq!(
            error,
            BacktestsWritePortError::BadRequest("executionModel must be a string".to_owned())
        );
    }
}
