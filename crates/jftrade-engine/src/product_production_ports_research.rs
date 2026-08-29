//! Production research preset and read adapters.

use std::sync::Arc;
use std::thread;
use jftrade_research::normalize_definition_v2;
use jftrade_store_sqlite::{ResearchPresetMutation, ResearchPresetStore, ResearchPresetStoreError};
use serde_json::{Value, json};
use jftrade_integration_marketdata_helper::{HelperClient, HttpAdapterError};
use crate::product::product_active_provider_state::ActiveProviderState;
use crate::product::product_research_preset_write_port::{ResearchPresetWriteMutation, ResearchPresetWritePort, ResearchPresetWritePortError};
use crate::product::product_research_screen_write_port::{ResearchScreenWritePort, ResearchScreenWritePortError, ResearchScreenWriteQuery};
use crate::product::{ResearchPresetReadSnapshotError, ResearchPresetReadSnapshotPort, ResearchReadSnapshotError, ResearchReadSnapshotPort};
use super::generate_strategy_id;

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
                .map(|p| serde_json::to_value(&p).map_err(|error| {
                    ResearchPresetReadSnapshotError::Unavailable(error.to_string())
                }))
                .collect::<Result<Vec<_>, _>>()?;
            return Ok(json!({ "presets": items }));
        }

        if let Some(id) = path.strip_prefix("/api/v1/research/screens/presets/") {
            if id.is_empty() || id.contains('/') {
                return Err(ResearchPresetReadSnapshotError::NotFound);
            }
            let preset = self
                .store
                .get(id)
                .map_err(|e| match e {
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
                    ResearchPresetWritePortError::Failed(format!("encode stored research preset: {e}"))
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
                let current = self.store.get(preset_id).map_err(map_research_preset_store_error)?;
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
                let next_revision = expected_revision.checked_add(1).ok_or_else(|| {
                    invalid_preset("expectedRevision exceeds supported range")
                })?;
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
                    ResearchPresetWritePortError::Failed(format!("encode stored research preset: {e}"))
                })
            }
            ResearchPresetWriteMutation::Delete { preset_id } => {
                if preset_id.trim().is_empty() {
                    return Err(invalid_preset("preset id is required"));
                }
                self.store.delete(preset_id).map_err(map_research_preset_store_error)?;
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

fn normalized_preset_definition(value: Option<&Value>) -> Result<Value, ResearchPresetWritePortError> {
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

fn map_research_preset_store_error(error: ResearchPresetStoreError) -> ResearchPresetWritePortError {
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
        if !snapshot.helper_ready {
            return Err(ResearchReadSnapshotError::Unavailable(
                "market-data helper is not ready".to_owned(),
            ));
        }
        let provider = match provider {
            jftrade_settings::MarketDataProvider::Yfinance => "yfinance",
            jftrade_settings::MarketDataProvider::Akshare => "akshare",
            jftrade_settings::MarketDataProvider::Futu => {
                return Err(ResearchReadSnapshotError::Unavailable(
                    "research helper does not support the Futu provider".to_owned(),
                ));
            }
        };
        let (operation, market, symbol, extra_query) = research_helper_request(path, query)?;
        let Some(helper) = self.helper.clone() else {
            return Err(ResearchReadSnapshotError::Unavailable(
                "market-data helper is not configured".to_owned(),
            ));
        };
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
                &[operation, &market, &symbol],
                &query_refs,
            ))
        })
        .join()
        .map_err(|_| ResearchReadSnapshotError::Unavailable("research helper task panicked".to_owned()))?;
        let payload = result.map_err(map_research_helper_error)?;
        validate_research_payload(operation, payload)
    }
}

type ResearchHelperRequest = (&'static str, String, String, Vec<(&'static str, String)>);

fn research_helper_request(
    path: &str,
    query: &str,
) -> Result<ResearchHelperRequest, ResearchReadSnapshotError> {
    let (operation, suffix) = if let Some(value) = path.strip_prefix("/api/v1/research/financials/") {
        ("financials", value)
    } else if let Some(value) = path.strip_prefix("/api/v1/research/analyst/") {
        ("analyst", value)
    } else if let Some(value) = path.strip_prefix("/api/v1/research/ownership/") {
        ("ownership", value)
    } else if let Some(value) = path.strip_prefix("/api/v1/research/corporate-actions/") {
        ("corporate-actions", value)
    } else if let Some(value) = path.strip_prefix("/api/v1/research/instruments/") {
        ("profile", value)
    } else {
        return Err(ResearchReadSnapshotError::Unavailable(
            "research operation is not backed by the market-data helper".to_owned(),
        ));
    };
    let mut parts = suffix.splitn(2, '/');
    let first = parts.next().unwrap_or_default().trim();
    let second = parts.next().unwrap_or_default().trim();
    let (market, symbol) = if operation == "profile" {
        first.split_once('.').ok_or_else(|| ResearchReadSnapshotError::Invalid(
            "instrument must use MARKET.SYMBOL form".to_owned(),
        ))?
    } else if !first.is_empty() && !second.is_empty() && !second.contains('/') {
        (first, second)
    } else {
        return Err(ResearchReadSnapshotError::Invalid(
            "research instrument path is invalid".to_owned(),
        ));
    };
    if market.is_empty() || symbol.is_empty() || market.contains('.') {
        return Err(ResearchReadSnapshotError::Invalid(
            "research instrument path is invalid".to_owned(),
        ));
    }
    let mut extra_query = Vec::new();
    for pair in query.split('&').filter(|pair| !pair.is_empty()) {
        let Some((key, value)) = pair.split_once('=') else { continue };
        if operation == "financials" && key == "statement" {
            extra_query.push(("statement", value.to_owned()));
        } else if operation == "corporate-actions" && (key == "from" || key == "to") {
            extra_query.push((if key == "from" { "from" } else { "to" }, value.to_owned()));
        }
    }
    Ok((operation, market.to_owned(), symbol.to_owned(), extra_query))
}

fn validate_research_payload(
    operation: &str,
    payload: Value,
) -> Result<Value, ResearchReadSnapshotError> {
    let Some(object) = payload.as_object() else {
        return Err(ResearchReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: "market-data helper returned a non-object research response".to_owned(),
            retry_after_seconds: None,
        });
    };
    let required = match operation {
        "financials" => &["instrumentId", "statement", "fields", "periods"][..],
        "analyst" => &["instrumentId"][..],
        "ownership" => &["instrumentId", "groups"][..],
        "corporate-actions" => &["market", "symbol", "instrumentId", "events", "source"][..],
        "profile" => &["instrumentId"][..],
        _ => &[][..],
    };
    if let Some(missing) = required.iter().find(|key| !object.contains_key(**key)) {
        return Err(ResearchReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: format!("market-data helper response is missing {missing}"),
            retry_after_seconds: None,
        });
    }
    Ok(payload)
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
            code: if code.is_empty() { "BAD_GATEWAY".to_owned() } else { code },
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

#[cfg(test)]
mod research_helper_tests {
    use super::*;
    use std::time::Duration;
    use tokio::io::{AsyncReadExt, AsyncWriteExt};
    use tokio::net::TcpListener;

    fn helper(base_url: String) -> HelperClient {
        HelperClient::new(jftrade_integration_marketdata_helper::HelperClientConfig {
            base_url,
            bearer_token: None,
            request_timeout: Duration::from_secs(1),
            max_attempts: 1,
            retry_delay: Duration::ZERO,
        })
        .expect("helper client")
    }

    #[test]
    fn research_helper_request_rejects_unsupported_or_malformed_paths() {
        assert!(matches!(
            research_helper_request("/api/v1/research/technical-indicators/US.AAPL", ""),
            Err(ResearchReadSnapshotError::Unavailable(_))
        ));
        assert!(matches!(
            research_helper_request("/api/v1/research/financials/US", ""),
            Err(ResearchReadSnapshotError::Invalid(_))
        ));
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 2)]
    async fn production_research_port_forwards_financials_to_helper() {
        let listener = TcpListener::bind("127.0.0.1:0").await.expect("listen");
        let address = listener.local_addr().expect("address");
        let server = tokio::spawn(async move {
            let (mut stream, _) = listener.accept().await.expect("accept");
            let mut request = vec![0_u8; 4096];
            let read = stream.read(&mut request).await.expect("read");
            let request = String::from_utf8_lossy(&request[..read]);
            assert!(request.starts_with(
                "GET /providers/yfinance/financials/US/AAPL?statement=balance HTTP/1.1\r\n"
            ));
            let body = r#"{"instrumentId":"US.AAPL","statement":"balance","fields":[],"periods":[]}"#;
            let response = format!(
                "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                body.len(), body
            );
            stream.write_all(response.as_bytes()).await.expect("write");
        });
        let state = Arc::new(ActiveProviderState::new(Some(
            jftrade_settings::MarketDataProvider::Yfinance,
        )));
        state.set_readiness(true, false, false);
        let port = ProductionResearchPort {
            active_provider_state: state,
            helper: Some(helper(format!("http://{address}"))),
        };
        let value = port
            .read(
                "/api/v1/research/financials/US/AAPL",
                "statement=balance",
            )
            .expect("research response");
        assert_eq!(value["statement"], "balance");
        server.await.expect("server");
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 2)]
    async fn production_research_port_preserves_helper_http_errors() {
        let listener = TcpListener::bind("127.0.0.1:0").await.expect("listen");
        let address = listener.local_addr().expect("address");
        let server = tokio::spawn(async move {
            let (mut stream, _) = listener.accept().await.expect("accept");
            let body = r#"{"error":{"code":"NOT_FOUND","message":"financials not found"}}"#;
            let response = format!(
                "HTTP/1.1 404 Not Found\r\nContent-Type: application/json\r\nContent-Length: {}\r\nRetry-After: 3\r\nConnection: close\r\n\r\n{}",
                body.len(), body
            );
            stream.write_all(response.as_bytes()).await.expect("write");
            let mut request = [0_u8; 1024];
            let _ = stream.read(&mut request).await;
        });
        let state = Arc::new(ActiveProviderState::new(Some(
            jftrade_settings::MarketDataProvider::Yfinance,
        )));
        state.set_readiness(true, false, false);
        let port = ProductionResearchPort {
            active_provider_state: state,
            helper: Some(helper(format!("http://{address}"))),
        };
        let result = port.read("/api/v1/research/analyst/US/AAPL", "");
        assert!(matches!(
            result,
            Err(ResearchReadSnapshotError::Failed {
                status: 404,
                ref code,
                ref message,
                retry_after_seconds: Some(3),
            }) if code == "NOT_FOUND" && message == "financials not found"
        ));
        server.await.expect("server");
    }
}

#[derive(Debug)]
pub(crate) struct ProductionResearchScreenPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
}

impl ResearchScreenWritePort for ProductionResearchScreenPort {
    fn query(
        &self,
        _request: &ResearchScreenWriteQuery,
    ) -> Result<Value, ResearchScreenWritePortError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() || (!snapshot.helper_ready && !snapshot.opend_ready) {
            return Err(ResearchScreenWritePortError::Unavailable);
        }
        Err(ResearchScreenWritePortError::Unavailable)
    }
}
