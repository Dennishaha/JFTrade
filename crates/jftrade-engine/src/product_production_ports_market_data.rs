//! Production market-data adapters bundle.
//!
//! Connects catalog reads, quote reads, subscription mutations, and provider
//! actions to real production state without mock fixtures or dummy arrays.

#[path = "product_production_ports_market_data_actions.rs"]
mod product_production_ports_market_data_actions;
#[path = "product_production_ports_market_data_catalog.rs"]
mod product_production_ports_market_data_catalog;
#[path = "product_production_ports_market_data_projection.rs"]
pub(crate) mod product_production_ports_market_data_projection;
#[path = "product_production_ports_market_data_quote.rs"]
mod product_production_ports_market_data_quote;
#[path = "product_production_ports_market_data_subscription.rs"]
mod product_production_ports_market_data_subscription;

pub(crate) use product_production_ports_market_data_actions::ProductionMarketDataProviderActionsPort;
pub(crate) use product_production_ports_market_data_catalog::ProductionMarketDataCatalogPort;
pub(crate) use product_production_ports_market_data_quote::ProductionMarketDataQuotePort;
pub(crate) use product_production_ports_market_data_subscription::ProductionMarketDataSubscriptionMutationPort;

use std::sync::Arc;
use std::thread;
use jftrade_integration_marketdata_helper::{HelperClient, HttpAdapterError};
use serde_json::Value;
use jftrade_settings::MarketDataProvider;
use crate::product::product_active_provider_state::ActiveProviderState;
use crate::product::{
    MarketDataDerivativeReadSnapshotError, MarketDataDerivativeReadSnapshotPort,
    MarketDataNewsActionsReadSnapshotError, MarketDataNewsActionsReadSnapshotPort,
    MarketDataNewsSearchReadSnapshotError, MarketDataNewsSearchReadSnapshotPort,
    MarketDataOptionsReadSnapshotError, MarketDataOptionsReadSnapshotPort,
    MarketDataPredictionReadSnapshotError, MarketDataPredictionReadSnapshotPort,
};
use super::product_production_ports_trade::SharedTradeReadRuntime;

#[derive(Debug)]
pub(crate) struct ProductionMarketDataDerivativePort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
}

impl MarketDataDerivativeReadSnapshotPort for ProductionMarketDataDerivativePort {
    fn read(&self, _path: &str, _query: &str) -> Result<Value, MarketDataDerivativeReadSnapshotError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() || !snapshot.opend_ready {
            return Err(MarketDataDerivativeReadSnapshotError::Unavailable(
                "derivative market-data provider is not configured".to_owned(),
            ));
        }
        Err(MarketDataDerivativeReadSnapshotError::Unavailable(
            "derivative market-data provider is not configured".to_owned(),
        ))
    }
}

#[derive(Debug)]
pub(crate) struct ProductionMarketDataOptionsPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
    pub(crate) trade_runtime: Option<Arc<SharedTradeReadRuntime>>,
}

impl MarketDataOptionsReadSnapshotPort for ProductionMarketDataOptionsPort {
    fn read(&self, path: &str, query: &str) -> Result<Value, MarketDataOptionsReadSnapshotError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider != Some(MarketDataProvider::Futu) || !snapshot.opend_ready {
            return Err(MarketDataOptionsReadSnapshotError::Unavailable(
                "Futu options market-data provider is not ready".to_owned(),
            ));
        }
        let runtime = self.trade_runtime.as_ref().ok_or_else(|| {
            MarketDataOptionsReadSnapshotError::Unavailable(
                "Futu options market-data runtime is not configured".to_owned(),
            )
        })?;
        if !runtime.option_expirations_available() {
            return Err(MarketDataOptionsReadSnapshotError::Unavailable(
                "Futu option expiration reader is not ready".to_owned(),
            ));
        }
        let request = parse_option_expiration_request(path, query)?;
        let dates = runtime
            .option_expirations(&request)
            .map_err(map_option_expiration_error)?;
        if dates.is_empty() {
            return Err(MarketDataOptionsReadSnapshotError::Failed {
                status: 404,
                code: "OPTION_EXPIRATIONS_NOT_FOUND".to_owned(),
                message: "OpenD returned no option expiration dates".to_owned(),
            });
        }
        Ok(option_expiration_result(dates))
    }
}

fn parse_option_expiration_request(
    path: &str,
    query: &str,
) -> Result<jftrade_integration_futu::OptionExpirationQuery, MarketDataOptionsReadSnapshotError>
{
    let instrument = path
        .strip_prefix("/api/v1/market-data/options/expirations/")
        .filter(|value| !value.is_empty() && !value.contains('/'))
        .ok_or_else(|| options_bad_request("unsupported options route"))?;
    let (market, symbol) = instrument
        .split_once('.')
        .filter(|(market, symbol)| !market.is_empty() && !symbol.is_empty() && !symbol.contains('.'))
        .ok_or_else(|| options_bad_request("instrumentId must be MARKET.CODE"))?;
    let market = market.trim().to_ascii_uppercase();
    let market_code = match market.as_str() {
        "HK" => 1,
        "US" => 11,
        _ => return Err(options_bad_request("option expiration market must be HK or US")),
    };
    let symbol = symbol.trim();
    if symbol.is_empty() || symbol.chars().any(char::is_whitespace) {
        return Err(options_bad_request("option expiration symbol is invalid"));
    }
    let query_map = crate::product::product_query::QueryMap::parse(query)
        .map_err(|_| options_bad_request("invalid URL escape"))?;
    if let Some(operation) = query_map.get_first("operation")
        && !operation.trim().is_empty()
        && operation.trim() != "expirations"
    {
        return Err(options_bad_request("operation must be expirations"));
    }
    if let Some(requested_market) = query_map.get_first("market")
        && !requested_market.trim().is_empty()
        && requested_market.trim().to_ascii_uppercase() != market
    {
        return Err(options_bad_request("market does not match instrumentId"));
    }
    let index_option_type = query_map
        .get_first("indexOptionType")
        .filter(|value| !value.trim().is_empty())
        .map(|value| {
            value.trim().parse::<i32>().ok().filter(|value| (0..=2).contains(value)).ok_or_else(|| {
                options_bad_request("indexOptionType must be 0, 1, or 2")
            })
        })
        .transpose()?;
    Ok(jftrade_integration_futu::OptionExpirationQuery {
        market: market_code,
        symbol: symbol.to_ascii_uppercase(),
        index_option_type,
    })
}

fn option_expiration_result(
    dates: Vec<jftrade_integration_futu::OptionExpirationDate>,
) -> Value {
    let as_of = super::provider_now_rfc3339();
    let entries = dates
        .iter()
        .map(|date| {
            let mut entry = serde_json::Map::new();
            if let Some(strike_time) = date.strike_time.as_ref() {
                entry.insert("strikeTime".to_owned(), Value::String(strike_time.clone()));
            }
            if let Some(strike_timestamp) = date.strike_timestamp {
                entry.insert("strikeTimestamp".to_owned(), serde_json::json!(strike_timestamp));
            }
            entry.insert(
                "optionExpiryDateDistance".to_owned(),
                serde_json::json!(date.expiry_date_distance),
            );
            if let Some(cycle) = date.cycle {
                entry.insert("cycle".to_owned(), serde_json::json!(cycle));
            }
            entry
        })
        .map(Value::Object)
        .collect::<Vec<_>>();
    let count = entries.len();
    let broker_id = "futu";
    serde_json::json!({
        "provider": {
            "brokerId": broker_id,
            "securityFirm": "Futu/Moomoo via OpenD",
            "featureId": "derivatives.option_chain",
            "capability": "available",
            "selectionReason": "adapter_request",
            "resolvedAt": as_of,
            "asOf": as_of,
        },
        "asOf": as_of,
        "entries": entries,
        "hasMore": false,
        "total": count,
    })
}

fn options_bad_request(message: &str) -> MarketDataOptionsReadSnapshotError {
    MarketDataOptionsReadSnapshotError::Failed {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: message.to_owned(),
    }
}

fn map_option_expiration_error(
    error: jftrade_integration_futu::OptionExpirationQueryError,
) -> MarketDataOptionsReadSnapshotError {
    match error {
        jftrade_integration_futu::OptionExpirationQueryError::InvalidQuery(message) => {
            options_bad_request(&message)
        }
        other => MarketDataOptionsReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: other.to_string(),
        },
    }
}

pub(crate) struct ProductionMarketDataNewsPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
    pub(crate) helper: Option<HelperClient>,
}

impl std::fmt::Debug for ProductionMarketDataNewsPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ProductionMarketDataNewsPort")
            .field("has_helper", &self.helper.is_some())
            .finish()
    }
}

impl MarketDataNewsActionsReadSnapshotPort for ProductionMarketDataNewsPort {
    fn read(&self, path: &str, query: &str) -> Result<Value, MarketDataNewsActionsReadSnapshotError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider != Some(MarketDataProvider::Yfinance) || !snapshot.helper_ready {
            return Err(MarketDataNewsActionsReadSnapshotError::Unavailable(
                "yfinance news provider is not ready".to_owned(),
            ));
        }
        let Some(helper) = self.helper.clone() else {
            return Err(MarketDataNewsActionsReadSnapshotError::Unavailable(
                "market-data helper is not configured".to_owned(),
            ));
        };
        let (operation, market, symbol, query_pairs) =
            news_actions_helper_request(path, query)?;
        let result = thread::spawn(move || {
            let query_refs = query_pairs
                .iter()
                .map(|(key, value)| (*key, value.as_str()))
                .collect::<Vec<_>>();
            let runtime = tokio::runtime::Builder::new_current_thread()
                .enable_all()
                .build()
                .map_err(|error| HttpAdapterError::Unavailable(error.to_string()))?;
            runtime.block_on(helper.get_provider_json_with_query::<Value>(
                "yfinance",
                &[operation, market.as_str(), symbol.as_str()],
                &query_refs,
            ))
        })
        .join()
        .map_err(|_| {
            MarketDataNewsActionsReadSnapshotError::Unavailable(
                "market-data helper task panicked".to_owned(),
            )
        })?;
        let payload = result.map_err(map_news_actions_helper_error)?;
        validate_news_actions_payload(payload)
    }
}

impl MarketDataNewsSearchReadSnapshotPort for ProductionMarketDataNewsPort {
    fn read(&self, _path: &str, _query: &str) -> Result<Value, MarketDataNewsSearchReadSnapshotError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() || (!snapshot.helper_ready && !snapshot.opend_ready) {
            return Err(MarketDataNewsSearchReadSnapshotError::Unavailable(
                "news provider is not configured".to_owned(),
            ));
        }
        Err(MarketDataNewsSearchReadSnapshotError::Unavailable(
            "news provider is not configured".to_owned(),
        ))
    }
}

type NewsActionsHelperRequest = (&'static str, String, String, Vec<(&'static str, String)>);

fn news_actions_helper_request(
    path: &str,
    query: &str,
) -> Result<NewsActionsHelperRequest, MarketDataNewsActionsReadSnapshotError> {
    let (operation, suffix) = if let Some(value) = path.strip_prefix("/api/v1/market-data/news/") {
        ("news", value)
    } else if let Some(value) = path.strip_prefix("/api/v1/market-data/corporate-actions/") {
        ("corporate-actions", value)
    } else {
        return Err(news_actions_bad_request("unsupported news/actions path"));
    };
    let mut parts = suffix.split('/');
    let market = parts.next().unwrap_or_default().trim();
    let symbol = parts.next().unwrap_or_default().trim();
    if market.is_empty() || symbol.is_empty() || parts.next().is_some() {
        return Err(news_actions_bad_request("invalid instrument"));
    }

    let query_map = crate::product::product_query::QueryMap::parse(query)
        .map_err(|_| news_actions_bad_request("invalid URL escape"))?;
    let mut query_pairs = Vec::new();
    if operation == "news" {
        if let Some(raw_limit) = query_map.get_first("limit") {
            let limit = raw_limit
                .trim()
                .parse::<u16>()
                .ok()
                .filter(|value| (1..=50).contains(value))
                .ok_or_else(|| news_actions_bad_request("limit must be between 1 and 50"))?;
            query_pairs.push(("limit", limit.to_string()));
        }
    } else {
        for key in ["from", "to"] {
            if let Some(value) = query_map.get_first(key).filter(|value| !value.trim().is_empty()) {
                query_pairs.push((key, value.to_owned()));
            }
        }
    }
    Ok((operation, market.to_owned(), symbol.to_owned(), query_pairs))
}

fn news_actions_bad_request(message: &str) -> MarketDataNewsActionsReadSnapshotError {
    MarketDataNewsActionsReadSnapshotError::Failed {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: message.to_owned(),
        retry_after_seconds: None,
    }
}

fn validate_news_actions_payload(
    payload: Value,
) -> Result<Value, MarketDataNewsActionsReadSnapshotError> {
    let Some(object) = payload.as_object() else {
        return Err(MarketDataNewsActionsReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: "market-data helper returned a non-object news response".to_owned(),
            retry_after_seconds: None,
        });
    };
    for key in ["market", "symbol", "instrumentId", "entries", "source"] {
        if !object.contains_key(key) {
            return Err(MarketDataNewsActionsReadSnapshotError::Failed {
                status: 502,
                code: "BAD_GATEWAY".to_owned(),
                message: format!("market-data helper response is missing {key}"),
                retry_after_seconds: None,
            });
        }
    }
    if !object.get("entries").is_some_and(Value::is_array) {
        return Err(MarketDataNewsActionsReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: "market-data helper response entries must be an array".to_owned(),
            retry_after_seconds: None,
        });
    }
    Ok(payload)
}

fn map_news_actions_helper_error(
    error: HttpAdapterError,
) -> MarketDataNewsActionsReadSnapshotError {
    match error {
        HttpAdapterError::Remote {
            status,
            code,
            message,
            retry_after_seconds,
        } => MarketDataNewsActionsReadSnapshotError::Failed {
            status,
            code: if code.is_empty() { "BAD_GATEWAY".to_owned() } else { code },
            message,
            retry_after_seconds,
        },
        HttpAdapterError::Timeout => MarketDataNewsActionsReadSnapshotError::Failed {
            status: 504,
            code: "GATEWAY_TIMEOUT".to_owned(),
            message: "market-data helper request timed out".to_owned(),
            retry_after_seconds: None,
        },
        HttpAdapterError::InvalidResponse(message) => MarketDataNewsActionsReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message,
            retry_after_seconds: None,
        },
        HttpAdapterError::Unavailable(message) => {
            MarketDataNewsActionsReadSnapshotError::Unavailable(message)
        }
        other => MarketDataNewsActionsReadSnapshotError::Failed {
            status: 500,
            code: "MARKET_DATA_NEWS_FAILED".to_owned(),
            message: other.to_string(),
            retry_after_seconds: None,
        },
    }
}

#[cfg(test)]
mod news_actions_tests {
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

    fn port(base_url: String) -> ProductionMarketDataNewsPort {
        let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Yfinance)));
        state.set_readiness(true, false, false);
        ProductionMarketDataNewsPort {
            active_provider_state: state,
            helper: Some(helper(base_url)),
        }
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 2)]
    async fn production_news_actions_port_forwards_yfinance_news_request() {
        let listener = TcpListener::bind("127.0.0.1:0").await.expect("listen");
        let address = listener.local_addr().expect("address");
        let server = tokio::spawn(async move {
            let (mut stream, _) = listener.accept().await.expect("accept");
            let mut request = vec![0_u8; 4096];
            let read = stream.read(&mut request).await.expect("read");
            let request = String::from_utf8_lossy(&request[..read]);
            assert!(request.starts_with(
                "GET /providers/yfinance/news/US/AAPL?limit=5 HTTP/1.1\r\n"
            ));
            let body = r#"{"market":"US","symbol":"AAPL","instrumentId":"US.AAPL","entries":[],"source":"yfinance-news"}"#;
            let response = format!(
                "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                body.len(), body
            );
            stream.write_all(response.as_bytes()).await.expect("write");
        });
        let value = MarketDataNewsActionsReadSnapshotPort::read(
            &port(format!("http://{address}")),
            "/api/v1/market-data/news/US/AAPL",
            "limit=5",
        )
            .expect("news response");
        assert_eq!(value["instrumentId"], "US.AAPL");
        server.await.expect("server");
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 2)]
    async fn production_news_actions_port_forwards_corporate_actions_window() {
        let listener = TcpListener::bind("127.0.0.1:0").await.expect("listen");
        let address = listener.local_addr().expect("address");
        let server = tokio::spawn(async move {
            let (mut stream, _) = listener.accept().await.expect("accept");
            let mut request = vec![0_u8; 4096];
            let read = stream.read(&mut request).await.expect("read");
            let request = String::from_utf8_lossy(&request[..read]);
            assert!(request.starts_with(
                "GET /providers/yfinance/corporate-actions/SH/600519?from=2026-01-01&to=2026-01-31 HTTP/1.1\r\n"
            ));
            let body = r#"{"market":"SH","symbol":"600519","instrumentId":"SH.600519","entries":[{"kind":"dividend","exDate":"2026-01-10","amount":1.2}],"source":"yfinance-actions"}"#;
            let response = format!(
                "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                body.len(), body
            );
            stream.write_all(response.as_bytes()).await.expect("write");
        });
        let value = MarketDataNewsActionsReadSnapshotPort::read(
            &port(format!("http://{address}")),
            "/api/v1/market-data/corporate-actions/SH/600519",
            "from=2026-01-01&to=2026-01-31",
        )
        .expect("corporate actions response");
        assert_eq!(value["entries"][0]["kind"], "dividend");
        server.await.expect("server");
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 2)]
    async fn production_news_actions_port_maps_helper_failure_and_rejects_bad_limit() {
        let listener = TcpListener::bind("127.0.0.1:0").await.expect("listen");
        let address = listener.local_addr().expect("address");
        let server = tokio::spawn(async move {
            let (mut stream, _) = listener.accept().await.expect("accept");
            let mut request = [0_u8; 4096];
            let _ = stream.read(&mut request).await.expect("read");
            let body = r#"{"error":{"code":"upstream_error","message":"Yahoo unavailable"}}"#;
            let response = format!(
                "HTTP/1.1 502 Bad Gateway\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                body.len(), body
            );
            stream.write_all(response.as_bytes()).await.expect("write");
        });
        let result = MarketDataNewsActionsReadSnapshotPort::read(
            &port(format!("http://{address}")),
            "/api/v1/market-data/news/US/AAPL",
            "limit=0",
        )
            .expect_err("invalid limit");
        assert!(matches!(result, MarketDataNewsActionsReadSnapshotError::Failed { status: 400, ref code, .. } if code == "BAD_REQUEST"));
        let result = MarketDataNewsActionsReadSnapshotPort::read(
            &port(format!("http://{address}")),
            "/api/v1/market-data/news/US/AAPL",
            "limit=5",
        )
            .expect_err("helper failure");
        assert!(matches!(result, MarketDataNewsActionsReadSnapshotError::Failed { status: 502, ref code, .. } if code == "upstream_error"));
        server.await.expect("server");
    }
}

#[cfg(test)]
mod option_expiration_tests {
    use super::*;

    #[derive(Debug)]
    struct FixtureOptionExpirationReader;

    impl jftrade_integration_futu::OptionExpirationReadPort for FixtureOptionExpirationReader {
        fn query(
            &self,
            query: &jftrade_integration_futu::OptionExpirationQuery,
        ) -> Result<Vec<jftrade_integration_futu::OptionExpirationDate>, jftrade_integration_futu::OptionExpirationQueryError> {
            assert_eq!(query.market, 11);
            assert_eq!(query.symbol, "AAPL");
            Ok(vec![jftrade_integration_futu::OptionExpirationDate {
                strike_time: Some("2026-09-18".to_owned()),
                strike_timestamp: Some(1_789_000_000.0),
                expiry_date_distance: 21,
                cycle: Some(1),
            }])
        }
    }

    fn ready_port() -> ProductionMarketDataOptionsPort {
        let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
        state.set_readiness(false, true, true);
        let runtime = Arc::new(SharedTradeReadRuntime::default());
        runtime.set_option_expirations(Some(Arc::new(FixtureOptionExpirationReader)));
        ProductionMarketDataOptionsPort {
            active_provider_state: state,
            trade_runtime: Some(runtime),
        }
    }

    #[test]
    fn expiration_projection_uses_typed_reader_and_neutral_wire() {
        let value = ready_port()
            .read(
                "/api/v1/market-data/options/expirations/US.AAPL",
                "market=US&operation=expirations",
            )
            .expect("expiration response");
        assert_eq!(value["provider"]["featureId"], "derivatives.option_chain");
        assert_eq!(value["entries"][0]["strikeTime"], "2026-09-18");
        assert_eq!(value["entries"][0]["optionExpiryDateDistance"], 21);
        assert_eq!(value["hasMore"], false);
        assert_eq!(value["total"], 1);
    }

    #[test]
    fn expiration_projection_rejects_mismatched_market_before_opend() {
        let error = ready_port()
            .read(
                "/api/v1/market-data/options/expirations/US.AAPL",
                "market=HK",
            )
            .expect_err("mismatched market");
        assert!(matches!(
            error,
            MarketDataOptionsReadSnapshotError::Failed { status: 400, ref code, .. }
                if code == "BAD_REQUEST"
        ));
    }

    #[test]
    fn expiration_projection_is_unavailable_without_typed_reader() {
        let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
        state.set_readiness(false, true, true);
        let port = ProductionMarketDataOptionsPort {
            active_provider_state: state,
            trade_runtime: Some(Arc::new(SharedTradeReadRuntime::default())),
        };
        assert!(matches!(
            port.read("/api/v1/market-data/options/expirations/US.AAPL", ""),
            Err(MarketDataOptionsReadSnapshotError::Unavailable(_))
        ));
    }
}

#[derive(Debug)]
pub(crate) struct ProductionMarketDataPredictionPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
}

impl MarketDataPredictionReadSnapshotPort for ProductionMarketDataPredictionPort {
    fn read(&self, _path: &str, _query: &str) -> Result<Value, MarketDataPredictionReadSnapshotError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() {
            return Err(MarketDataPredictionReadSnapshotError::Unavailable(
                "prediction market-data provider is not configured".to_owned(),
            ));
        }
        Err(MarketDataPredictionReadSnapshotError::Unavailable(
            "prediction market-data provider is not configured".to_owned(),
        ))
    }
}
