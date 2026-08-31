//! Production research preset and read adapters.
use super::generate_strategy_id;
use crate::product::product_active_provider_state::ActiveProviderState;
use crate::product::product_production_ports::SharedTradeReadRuntime;
use crate::product::product_query::QueryMap;
use crate::product::product_research_preset_write_port::{
    ResearchPresetWriteMutation, ResearchPresetWritePort, ResearchPresetWritePortError,
};
use crate::product::{
    ResearchPresetReadSnapshotError, ResearchPresetReadSnapshotPort, ResearchReadSnapshotError,
    ResearchReadSnapshotPort,
};
use jftrade_integration_marketdata_helper::{HelperClient, HttpAdapterError};
use jftrade_research::normalize_definition_v2;
use jftrade_store_sqlite::{ResearchPresetMutation, ResearchPresetStore, ResearchPresetStoreError};
use serde_json::{Value, json};
use std::sync::Arc;
use std::thread;

#[path = "product_production_ports_research_company.rs"]
mod company;
use company::{project_research_payload, research_helper_request};

#[path = "product_production_ports_research_screen.rs"]
mod screen;
pub(crate) use screen::ProductionResearchScreenHelperPort;

#[derive(Debug)]
pub(crate) struct ProductionResearchPresetPort {
    pub(crate) store: Arc<ResearchPresetStore>,
}
impl ResearchPresetReadSnapshotPort for ProductionResearchPresetPort {
    fn read(&self, path: &str, _query: &str) -> Result<Value, ResearchPresetReadSnapshotError> {
        if path == "/api/v1/research/screens/presets" {
            let presets = self
                .store
                .list()
                .map_err(|e| ResearchPresetReadSnapshotError::Unavailable(e.to_string()))?;
            let items: Vec<Value> = presets
                .into_iter()
                .map(|p| {
                    serde_json::to_value(&p).map_err(|error| {
                        ResearchPresetReadSnapshotError::Unavailable(error.to_string())
                    })
                })
                .collect::<Result<Vec<_>, _>>()?;
            return Ok(json!({ "presets": items }));
        }
        if let Some(id) = path.strip_prefix("/api/v1/research/screens/presets/") {
            if id.is_empty() || id.contains('/') {
                return Err(ResearchPresetReadSnapshotError::NotFound);
            }
            let preset = self.store.get(id).map_err(|e| match e {
                ResearchPresetStoreError::NotFound => ResearchPresetReadSnapshotError::NotFound,
                other => ResearchPresetReadSnapshotError::Unavailable(other.to_string()),
            })?;
            return serde_json::to_value(&preset)
                .map_err(|error| ResearchPresetReadSnapshotError::Unavailable(error.to_string()));
        }
        Err(ResearchPresetReadSnapshotError::NotFound)
    }
}
impl ResearchPresetWritePort for ProductionResearchPresetPort {
    fn mutate(
        &self,
        mutation: &ResearchPresetWriteMutation,
    ) -> Result<Value, ResearchPresetWritePortError> {
        let timestamp = time::OffsetDateTime::now_utc()
            .format(&time::format_description::well_known::Rfc3339)
            .unwrap_or_default();

        match mutation {
            ResearchPresetWriteMutation::Create { payload } => {
                let object = payload
                    .as_object()
                    .ok_or_else(|| invalid_preset("name is required"))?;
                let name = normalized_preset_name(object.get("name"))?;
                let definition = normalized_preset_definition(object.get("definition"))?;
                let id = object
                    .get("id")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|s| !s.is_empty())
                    .map(ToOwned::to_owned)
                    .unwrap_or_else(|| format!("preset_{}", generate_strategy_id()));
                let preset = ResearchPresetMutation {
                    preset_id: id,
                    name,
                    query_schema_version: 2,
                    definition,
                    revision: 1,
                };
                let stored = self
                    .store
                    .insert(&preset, &timestamp)
                    .map_err(map_research_preset_store_error)?;
                serde_json::to_value(&stored).map_err(|e| {
                    ResearchPresetWritePortError::Failed(format!(
                        "encode stored research preset: {e}"
                    ))
                })
            }
            ResearchPresetWriteMutation::Update { preset_id, payload } => {
                if preset_id.trim().is_empty() {
                    return Err(invalid_preset("preset id is required"));
                }
                let object = payload
                    .as_object()
                    .ok_or_else(|| invalid_preset("expectedRevision must be positive"))?;
                let expected_revision = object
                    .get("expectedRevision")
                    .and_then(Value::as_u64)
                    .filter(|r| *r > 0)
                    .ok_or_else(|| invalid_preset("expectedRevision must be positive"))?;
                let has_name = object.get("name").is_some_and(|v| !v.is_null());
                let has_definition = object.get("definition").is_some_and(|v| !v.is_null());
                if !has_name && !has_definition {
                    return Err(invalid_preset("name or definition is required"));
                }
                let current = self
                    .store
                    .get(preset_id)
                    .map_err(map_research_preset_store_error)?;
                let name = if has_name {
                    normalized_preset_name(object.get("name"))?
                } else {
                    current.preset.name.clone()
                };
                let definition = if has_definition {
                    normalized_preset_definition(object.get("definition"))?
                } else {
                    current.preset.definition.clone()
                };
                let next_revision = expected_revision
                    .checked_add(1)
                    .ok_or_else(|| invalid_preset("expectedRevision exceeds supported range"))?;
                let preset = ResearchPresetMutation {
                    preset_id: current.preset.preset_id,
                    name,
                    query_schema_version: 2,
                    definition,
                    revision: next_revision,
                };
                let stored = self
                    .store
                    .replace_revision(&preset, expected_revision, &timestamp)
                    .map_err(map_research_preset_store_error)?;
                serde_json::to_value(&stored).map_err(|e| {
                    ResearchPresetWritePortError::Failed(format!(
                        "encode stored research preset: {e}"
                    ))
                })
            }
            ResearchPresetWriteMutation::Delete { preset_id } => {
                if preset_id.trim().is_empty() {
                    return Err(invalid_preset("preset id is required"));
                }
                self.store
                    .delete(preset_id)
                    .map_err(map_research_preset_store_error)?;
                Ok(json!({"deleted": true}))
            }
        }
    }
}
fn normalized_preset_name(value: Option<&Value>) -> Result<String, ResearchPresetWritePortError> {
    let name = value
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|name| !name.is_empty())
        .ok_or_else(|| invalid_preset("name is required"))?;
    if name.chars().count() > 80 {
        return Err(invalid_preset("name must not exceed 80 characters"));
    }
    Ok(name.to_owned())
}
fn normalized_preset_definition(
    value: Option<&Value>,
) -> Result<Value, ResearchPresetWritePortError> {
    let value = value
        .cloned()
        .ok_or_else(|| invalid_preset("definition is required"))?;
    normalize_definition_v2(value).map_err(|error| invalid_preset(error.to_string()))
}
fn invalid_preset(message: impl Into<String>) -> ResearchPresetWritePortError {
    ResearchPresetWritePortError::Invalid(format!(
        "invalid research screen preset: {}",
        message.into()
    ))
}
fn map_research_preset_store_error(
    error: ResearchPresetStoreError,
) -> ResearchPresetWritePortError {
    match error {
        ResearchPresetStoreError::NotFound => {
            ResearchPresetWritePortError::NotFound("research screen preset not found".to_owned())
        }
        ResearchPresetStoreError::Conflict => {
            ResearchPresetWritePortError::Conflict("research screen preset conflict".to_owned())
        }
        ResearchPresetStoreError::Incompatible(message) => invalid_preset(message),
        ResearchPresetStoreError::UnsupportedProfile(_) => {
            ResearchPresetWritePortError::Unavailable
        }
        ResearchPresetStoreError::NotRegularFile(_)
        | ResearchPresetStoreError::EmptyPath
        | ResearchPresetStoreError::WriterLease(_)
        | ResearchPresetStoreError::Open(_)
        | ResearchPresetStoreError::Configure(_)
        | ResearchPresetStoreError::Schema(_)
        | ResearchPresetStoreError::LockUnavailable
        | ResearchPresetStoreError::Query(_) => ResearchPresetWritePortError::Unavailable,
    }
}

// ---------------------------------------------------------------------------
// Research Read & Screen Write
// ---------------------------------------------------------------------------
pub(crate) struct ProductionResearchPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
    pub(crate) helper: Option<HelperClient>,
    pub(crate) trade_runtime: Option<Arc<SharedTradeReadRuntime>>,
}
impl std::fmt::Debug for ProductionResearchPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ProductionResearchPort")
            .field("helper", &self.helper.is_some())
            .finish()
    }
}

impl ResearchReadSnapshotPort for ProductionResearchPort {
    fn read(&self, path: &str, query: &str) -> Result<Value, ResearchReadSnapshotError> {
        let snapshot = self.active_provider_state.snapshot();
        let Some(provider) = snapshot.provider else {
            return Err(ResearchReadSnapshotError::Unavailable(
                "research provider is not configured".to_owned(),
            ));
        };
        let query_map = QueryMap::parse(query)
            .map_err(|_| ResearchReadSnapshotError::Invalid("invalid URL escape".to_owned()))?;
        if !super::super::provider_request_matches(provider, &query_map) {
            let requested = query_map
                .get_first("brokerId")
                .or_else(|| query_map.get_first("providerBrokerId"))
                .unwrap_or_default();
            return Err(capability(
                "research",
                &format!("requested broker {requested:?} does not match active provider"),
            ));
        }
        if matches!(
            path,
            "/api/v1/research/rankings" | "/api/v1/research/industries"
        ) {
            return super::read_market_research(
                provider,
                snapshot.helper_ready,
                self.helper.as_ref(),
                path,
                query,
            );
        }
        if matches!(
            path,
            "/api/v1/research/calendars" | "/api/v1/research/macro"
        ) {
            return super::read_market_calendar(
                provider,
                snapshot.helper_ready,
                self.helper.as_ref(),
                path,
                query,
            );
        }
        if provider == jftrade_settings::MarketDataProvider::Futu {
            if path.starts_with("/api/v1/research/corporate-actions/") {
                if !snapshot.opend_ready {
                    return Err(ResearchReadSnapshotError::Unavailable(
                        "Futu OpenD corporate actions runtime is not ready".to_owned(),
                    ));
                }
                return read_futu_corporate_actions(self.trade_runtime.as_ref(), path, query);
            }
            if !path.starts_with("/api/v1/research/valuation/") {
                return Err(capability(
                    "research",
                    "Futu research runtime currently supports valuation detail only",
                ));
            }
            if !snapshot.opend_ready {
                return Err(ResearchReadSnapshotError::Unavailable(
                    "Futu OpenD research runtime is not ready".to_owned(),
                ));
            }
            return read_futu_valuation(self.trade_runtime.as_ref(), path, query);
        }
        let (operation, market, symbol, extra_query) = research_helper_request(path, query)?;
        if !snapshot.helper_ready {
            return Err(ResearchReadSnapshotError::Unavailable(
                "market-data helper is not ready".to_owned(),
            ));
        }
        let provider = match provider {
            jftrade_settings::MarketDataProvider::Yfinance => "yfinance",
            jftrade_settings::MarketDataProvider::Akshare => "akshare",
            jftrade_settings::MarketDataProvider::Futu => unreachable!(),
        };
        let Some(helper) = self.helper.clone() else {
            return Err(ResearchReadSnapshotError::Unavailable(
                "market-data helper is not configured".to_owned(),
            ));
        };
        let requested_market = market.clone();
        let requested_symbol = symbol.clone();
        let expected_statement = extra_query
            .iter()
            .find(|(key, _)| *key == "statement")
            .map(|(_, value)| value.clone());
        let result = thread::spawn(move || {
            let query_refs = extra_query
                .iter()
                .map(|(key, value)| (*key, value.as_str()))
                .collect::<Vec<_>>();
            let runtime = tokio::runtime::Builder::new_current_thread()
                .enable_all()
                .build()
                .map_err(|error| HttpAdapterError::Unavailable(error.to_string()))?;
            runtime.block_on(helper.get_provider_json_with_query::<Value>(
                provider,
                &[operation, &requested_market, &requested_symbol],
                &query_refs,
            ))
        })
        .join()
        .map_err(|_| {
            ResearchReadSnapshotError::Unavailable("research helper task panicked".to_owned())
        })?;
        let payload = result.map_err(map_research_helper_error)?;
        project_research_payload(
            operation,
            payload,
            &market,
            &symbol,
            provider,
            expected_statement.as_deref(),
        )
    }
}

fn read_futu_corporate_actions(
    runtime: Option<&Arc<SharedTradeReadRuntime>>,
    path: &str,
    query: &str,
) -> Result<Value, ResearchReadSnapshotError> {
    let runtime = runtime.ok_or_else(|| {
        ResearchReadSnapshotError::Unavailable(
            "Futu corporate actions runtime is not configured".to_owned(),
        )
    })?;
    if !runtime.corporate_actions_reader_available() {
        return Err(ResearchReadSnapshotError::Unavailable(
            "Futu corporate actions reader is not ready".to_owned(),
        ));
    }
    let (_, market, symbol, _) = research_helper_request(path, query)?;
    let synthetic_path = format!("/api/v1/market-data/corporate-actions/{market}/{symbol}");
    let payload = super::super::product_production_ports_market_data::product_production_ports_market_data_news_actions::read_futu(
        Some(runtime),
        &synthetic_path,
        query,
    )
    .map_err(map_futu_corporate_actions_error)?;
    let events = payload
        .get("events")
        .and_then(Value::as_array)
        .cloned()
        .ok_or_else(|| {
            ResearchReadSnapshotError::Failed {
                status: 502,
                code: "BAD_GATEWAY".to_owned(),
                message: "market-data helper response is missing events".to_owned(),
                retry_after_seconds: None,
            }
        })?;
    let total = events.len();
    let as_of = super::super::provider_now_rfc3339();
    Ok(json!({
        "provider": {
            "brokerId": "futu",
            "securityFirm": "Futu/Moomoo via OpenD",
            "featureId": "research.corporate_actions",
            "capability": "available",
            "selectionReason": "adapter_request",
            "resolvedAt": as_of,
            "asOf": as_of,
        },
        "resolvedInstrument": {
            "instrumentId": format!("{market}.{symbol}"),
            "code": symbol,
            "productClass": "unknown",
            "marketSegment": "securities",
            "quoteMarket": market,
            "tradeMarket": market,
            "quantityMode": "units",
        },
        "asOf": as_of,
        "entries": events,
        "hasMore": false,
        "total": total,
    }))
}

fn map_futu_corporate_actions_error(
    error: crate::product::MarketDataNewsActionsReadSnapshotError,
) -> ResearchReadSnapshotError {
    match error {
        crate::product::MarketDataNewsActionsReadSnapshotError::Unavailable(message) => {
            ResearchReadSnapshotError::Unavailable(message)
        }
        crate::product::MarketDataNewsActionsReadSnapshotError::Failed {
            status,
            code,
            message,
            retry_after_seconds,
        } => ResearchReadSnapshotError::Failed {
            status,
            code,
            message,
            retry_after_seconds,
        },
    }
}

fn map_research_helper_error(error: HttpAdapterError) -> ResearchReadSnapshotError {
    match error {
        HttpAdapterError::Remote {
            status,
            code,
            message,
            retry_after_seconds,
        } => ResearchReadSnapshotError::Failed {
            status,
            code: if code.is_empty() {
                "BAD_GATEWAY".to_owned()
            } else {
                code
            },
            message,
            retry_after_seconds,
        },
        HttpAdapterError::Timeout => ResearchReadSnapshotError::Failed {
            status: 504,
            code: "GATEWAY_TIMEOUT".to_owned(),
            message: "market-data helper request timed out".to_owned(),
            retry_after_seconds: None,
        },
        HttpAdapterError::InvalidResponse(message) => ResearchReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message,
            retry_after_seconds: None,
        },
        other => ResearchReadSnapshotError::Unavailable(other.to_string()),
    }
}

fn capability(feature: &str, operation: &str) -> ResearchReadSnapshotError {
    ResearchReadSnapshotError::Failed {
        status: 409,
        code: "CAPABILITY_UNAVAILABLE".to_owned(),
        message: format!(
            "embedded market-data provider does not serve {feature} operation {operation:?}"
        ),
        retry_after_seconds: None,
    }
}

fn read_futu_valuation(
    runtime: Option<&Arc<SharedTradeReadRuntime>>,
    path: &str,
    query: &str,
) -> Result<Value, ResearchReadSnapshotError> {
    let runtime = runtime.ok_or_else(|| {
        ResearchReadSnapshotError::Unavailable(
            "Futu valuation detail runtime is not configured".to_owned(),
        )
    })?;
    if !runtime.valuation_detail_available() {
        return Err(ResearchReadSnapshotError::Unavailable(
            "Futu valuation detail reader is not ready".to_owned(),
        ));
    }
    let instrument = path
        .strip_prefix("/api/v1/research/valuation/")
        .filter(|value| !value.is_empty() && !value.contains('/'))
        .ok_or_else(|| {
            ResearchReadSnapshotError::Invalid("unsupported valuation route".to_owned())
        })?;
    let (market, code) = instrument.split_once('.').ok_or_else(|| {
        ResearchReadSnapshotError::Invalid("instrumentId must be MARKET.CODE".to_owned())
    })?;
    let market = market.trim().to_ascii_uppercase();
    let code = code.trim().to_ascii_uppercase();
    if market.is_empty()
        || code.is_empty()
        || code
            .chars()
            .any(|value| value.is_whitespace() || value.is_control())
    {
        return Err(ResearchReadSnapshotError::Invalid(
            "instrumentId must be MARKET.CODE".to_owned(),
        ));
    }
    let market_code = valuation_market_code(&market).ok_or_else(|| {
        ResearchReadSnapshotError::Invalid("valuation detail market is unsupported".to_owned())
    })?;
    let query_map = QueryMap::parse(query)
        .map_err(|_| ResearchReadSnapshotError::Invalid("invalid URL escape".to_owned()))?;
    for key in valuation_query_keys(query)? {
        if !matches!(
            key.as_str(),
            "brokerId" | "accountId" | "market" | "operation" | "valuationType" | "intervalType"
        ) {
            return Err(ResearchReadSnapshotError::Invalid(format!(
                "unsupported valuation query parameter {key}"
            )));
        }
    }
    if let Some(requested_market) = query_map.get_first("market")
        && !requested_market.trim().is_empty()
        && requested_market.trim().to_ascii_uppercase() != market
    {
        return Err(ResearchReadSnapshotError::Invalid(
            "market does not match instrumentId".to_owned(),
        ));
    }
    if let Some(operation) = query_map.get_first("operation")
        && !operation.trim().is_empty()
        && !matches!(operation.trim(), "valuation" | "detail")
    {
        return Err(ResearchReadSnapshotError::Invalid(
            "valuation operation must be valuation or detail".to_owned(),
        ));
    }
    let valuation_type = parse_optional_i32(&query_map, "valuationType")?;
    let interval_type = parse_optional_i32(&query_map, "intervalType")?;
    let request = jftrade_integration_futu::ValuationDetailQuery {
        market: market_code,
        code,
        valuation_type,
        interval_type,
    };
    let snapshot = runtime
        .valuation_detail(&request)
        .map_err(map_valuation_error)?;
    let entry =
        serde_json::to_value(snapshot).map_err(|error| ResearchReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: format!("serialize Futu valuation detail response: {error}"),
            retry_after_seconds: None,
        })?;
    let as_of = crate::product::product_production_ports::provider_now_rfc3339();
    Ok(json!({
        "provider": {
            "brokerId": "futu",
            "securityFirm": "Futu/Moomoo via OpenD",
            "featureId": "research.valuation",
            "capability": "available",
            "selectionReason": "adapter_request",
            "resolvedAt": as_of,
            "asOf": as_of,
        },
        "asOf": as_of,
        "entries": [entry],
        "hasMore": false,
        "total": 1,
    }))
}

fn valuation_query_keys(query: &str) -> Result<Vec<String>, ResearchReadSnapshotError> {
    let mut keys = Vec::new();
    for pair in query.trim().trim_start_matches('?').split('&') {
        if pair.is_empty() {
            continue;
        }
        let raw_key = pair.split_once('=').map_or(pair, |(key, _)| key);
        let key = crate::product::product_query::decode_query_component(raw_key)
            .map_err(|_| ResearchReadSnapshotError::Invalid("invalid URL escape".to_owned()))?;
        keys.push(key);
    }
    Ok(keys)
}

fn parse_optional_i32(
    query: &QueryMap,
    key: &str,
) -> Result<Option<i32>, ResearchReadSnapshotError> {
    let Some(value) = query.get_first(key) else {
        return Ok(None);
    };
    if value.trim().is_empty() {
        return Err(ResearchReadSnapshotError::Invalid(format!(
            "{key} must be an integer"
        )));
    }
    value
        .trim()
        .parse::<i32>()
        .map(Some)
        .map_err(|_| ResearchReadSnapshotError::Invalid(format!("{key} must be an integer")))
}

fn map_valuation_error(
    error: jftrade_integration_futu::ValuationDetailQueryError,
) -> ResearchReadSnapshotError {
    use jftrade_integration_futu::ValuationDetailQueryError;
    match error {
        ValuationDetailQueryError::InvalidQuery(message)
            if message.contains("runtime is unavailable") =>
        {
            ResearchReadSnapshotError::Unavailable(message)
        }
        ValuationDetailQueryError::InvalidQuery(message) => {
            ResearchReadSnapshotError::Invalid(message)
        }
        ValuationDetailQueryError::Rejected {
            ret_type,
            err_code,
            message,
        } => ResearchReadSnapshotError::Failed {
            status: 502,
            code: "FUTU_VALUATION_REJECTED".to_owned(),
            message: format!(
                "OpenD valuation detail retType={ret_type} errCode={err_code}: {message}"
            ),
            retry_after_seconds: None,
        },
        ValuationDetailQueryError::Session(error) => ResearchReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: error.to_string(),
            retry_after_seconds: None,
        },
        ValuationDetailQueryError::Decode(error) => ResearchReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: error.to_string(),
            retry_after_seconds: None,
        },
        ValuationDetailQueryError::MissingS2c | ValuationDetailQueryError::InvalidResponse(_) => {
            ResearchReadSnapshotError::Failed {
                status: 502,
                code: "BAD_GATEWAY".to_owned(),
                message: error.to_string(),
                retry_after_seconds: None,
            }
        }
    }
}

fn valuation_market_code(market: &str) -> Option<i32> {
    match market {
        "HK" => Some(1),
        "US" => Some(11),
        "SH" => Some(21),
        "SZ" => Some(22),
        "SG" => Some(31),
        "JP" => Some(41),
        "AU" => Some(51),
        "MY" => Some(61),
        "CA" => Some(71),
        "FX" => Some(81),
        "CRYPTO" => Some(91),
        _ => None,
    }
}

#[cfg(test)]
#[path = "product_production_ports_research_tests.rs"]
mod research_helper_tests;
