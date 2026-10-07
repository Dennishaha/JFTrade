//! Screen parity through concrete production adapters and a loopback helper.

use super::*;
use crate::product::product_research_screen_write_port::{
    ResearchScreenWritePort, ResearchScreenWritePortError, ResearchScreenWriteQuery,
};
use jftrade_integration_marketdata_helper::{HelperClient, HelperClientConfig};
use std::sync::Mutex;

const AUTH: &[(&str, &str)] = &[("Authorization", "Bearer aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")];

fn production_config(directory: &tempfile::TempDir) -> ProductConfig {
    let path = directory.path().join("settings.json");
    std::fs::write(&path, "{}").expect("settings");
    product_data_management::initialize_production_databases(&path).expect("databases");
    let mut config = ProductConfig::desktop_production(
        "127.0.0.1:0".parse().expect("address"),
        &path,
        "a".repeat(32),
    )
    .expect("production config");
    config.capabilities = ProductCapabilities::all();
    config
}

fn embedded_body() -> Value {
    json!({"brokerId":"yfinance","market":"US","catalogVersion":"embedded-stock-screen-v1","querySchemaVersion":2,
        "conditions":[{"id":"price-filter","factor":{"instanceId":"price-filter","factorKey":"simple.price"},"operator":"between","value":{"min":100,"max":300}}],
        "columns":[{"columnId":"code-column","factor":{"instanceId":"code-column","factorKey":"basic.code"}},
            {"columnId":"price-column","factor":{"instanceId":"price-column","factorKey":"simple.price"},"label":"最新价"}],
        "sorts":[{"sortId":"cap-sort","factor":{"instanceId":"cap-sort","factorKey":"simple.market_cap"},"direction":"desc"}],
        "page":{"offset":0,"limit":25}})
}

struct HelperFixture {
    client: HelperClient,
    calls: Arc<Mutex<Vec<(String, Value)>>>,
    task: Option<tokio::task::JoinHandle<()>>,
}

impl HelperFixture {
    async fn new(status: u16, body: Value) -> Self {
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
            .await
            .expect("helper listener");
        let address = listener.local_addr().expect("helper address");
        let calls = Arc::new(Mutex::new(Vec::new()));
        let recorded = Arc::clone(&calls);
        let task = tokio::spawn(async move {
            loop {
                let (mut stream, _) = listener.accept().await.expect("accept helper");
                let (target, request) =
                    tokio::time::timeout(Duration::from_secs(5), read_helper_request(&mut stream))
                        .await
                        .expect("bounded helper request");
                recorded.lock().expect("calls").push((target, request));
                let body = body.to_string();
                let response = format!(
                    "HTTP/1.1 {status} Fixture\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
                    body.len()
                );
                stream
                    .write_all(response.as_bytes())
                    .await
                    .expect("helper response");
            }
        });
        Self {
            client: HelperClient::new(HelperClientConfig {
                base_url: format!("http://{address}"),
                bearer_token: None,
                request_timeout: Duration::from_secs(2),
                max_attempts: 1,
                retry_delay: Duration::ZERO,
            })
            .expect("helper client"),
            calls,
            task: Some(task),
        }
    }

    async fn product(&self, directory: &tempfile::TempDir) -> ProductHandle {
        let state = Arc::new(crate::product::ActiveProviderState::new(Some(
            jftrade_settings::MarketDataProvider::Yfinance,
        )));
        start_product(
            production_config(directory)
                .with_active_provider_state(state)
                .with_market_data_helper(self.client.clone()),
        )
        .await
        .expect("concrete production helper adapter")
    }

    async fn shutdown(&mut self) {
        let task = self.task.take().expect("fixture task");
        task.abort();
        assert!(
            task.await
                .expect_err("cancelled helper listener")
                .is_cancelled()
        );
    }
}

impl Drop for HelperFixture {
    fn drop(&mut self) {
        if let Some(task) = &self.task {
            task.abort();
        }
    }
}

async fn read_helper_request(stream: &mut tokio::net::TcpStream) -> (String, Value) {
    let mut request = Vec::new();
    let mut chunk = [0_u8; 1024];
    let (header_end, length) = loop {
        let read = stream.read(&mut chunk).await.expect("helper request");
        assert_ne!(read, 0, "request ended before headers");
        request.extend_from_slice(&chunk[..read]);
        assert!(request.len() < 64 * 1024, "bounded fixture request");
        if let Some(end) = request.windows(4).position(|bytes| bytes == b"\r\n\r\n") {
            let headers = String::from_utf8_lossy(&request[..end]);
            let length = headers
                .lines()
                .find_map(|line| {
                    let (key, value) = line.split_once(':')?;
                    key.eq_ignore_ascii_case("content-length")
                        .then(|| value.trim().parse::<usize>().expect("content length"))
                })
                .expect("content length header");
            break (end + 4, length);
        }
    };
    assert!(length < 32 * 1024, "bounded fixture body");
    while request.len() < header_end + length {
        let read = stream.read(&mut chunk).await.expect("helper body");
        assert_ne!(read, 0, "request ended before body");
        request.extend_from_slice(&chunk[..read]);
    }
    let headers = String::from_utf8_lossy(&request[..header_end]);
    let target = headers.lines().next().expect("request line").to_owned();
    (
        target,
        serde_json::from_slice(&request[header_end..header_end + length]).expect("request JSON"),
    )
}

async fn screen_post(handle: &ProductHandle, body: &Value) -> (u16, Value) {
    request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/research/screens",
        Some(&body.to_string()),
        AUTH,
    )
    .await
}

// Parity: go:452dea11:internal/api/productfeatures/research_screen_test.go:272 TestResearchScreenCatalogServesEmbeddedProviders
#[tokio::test]
async fn production_http_screen_catalog_enforces_embedded_provider_market_matrix() {
    let directory = tempdir().expect("directory");
    let handle = start_product(production_config(&directory))
        .await
        .expect("production");
    for (query, expected, provider, market) in [
        ("brokerId=yfinance", 200, "yfinance", ""),
        ("brokerId=akshare&market=CN", 200, "akshare", "CN"),
        ("brokerId=yfinance&market=HK", 400, "", ""),
        ("brokerId=akshare&market=US", 200, "akshare", "US"),
        ("brokerId=akshare&market=MO", 400, "", ""),
        ("brokerId=unknown", 409, "", ""),
    ] {
        let (status, response) = request_json_with_status(
            handle.startup_record().address,
            "GET",
            &format!("/api/v1/research/screens/catalog?{query}"),
            None,
            AUTH,
        )
        .await;
        assert_eq!(status, expected, "{query}: {response}");
        if expected == 200 {
            let data = &response["data"];
            assert_eq!(data["version"], "embedded-stock-screen-v1");
            assert_eq!(data["provider"], provider);
            if market.is_empty() {
                assert!(data.get("market").is_none());
            } else {
                assert_eq!(data["market"], market);
            }
            if provider == "yfinance" {
                assert!(data.to_string().contains("simple.change_pct"));
                assert!(!data.to_string().contains("\"HK\""));
            }
            if market == "CN" {
                assert!(data.to_string().contains("\"HK\""));
            }
        }
    }
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/productfeatures/research_screen_test.go:355 TestResearchScreenPostServesEmbeddedProvider
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn production_http_screen_executes_embedded_query_and_projects_typed_cells() {
    let directory = tempdir().expect("directory");
    let mut fixture = HelperFixture::new(200,json!({"entries":[{"instrument_id":"US.AAPL","name":"Apple","quote_currency":"USD","values":{"simple.price":189.25}}],
        "total":7,"has_more":true,"next_offset":1,"as_of":"2026-08-15T20:00:00Z","source":"yfinance-screen-us"})).await;
    let handle = fixture.product(&directory).await;
    let (status, response) = screen_post(&handle, &embedded_body()).await;
    assert_eq!(status, 200, "{response}");
    let calls = fixture.calls.lock().expect("calls").clone();
    assert_eq!(calls.len(), 1);
    assert_eq!(calls[0].0, "POST /providers/yfinance/screen HTTP/1.1");
    assert_eq!(
        calls[0].1,
        json!({"market":"US","conditions":[{"factor_key":"simple.price","min":100,"max":300}],
        "sorts":[{"factor_key":"simple.market_cap","direction":"desc"}],"offset":0,"limit":25})
    );
    let data = &response["data"];
    assert_eq!(data["catalogVersion"], "embedded-stock-screen-v1");
    assert_eq!(data["provider"]["brokerId"], "yfinance");
    assert_eq!(
        data["provider"]["selectionReason"],
        "embedded-market-data-provider"
    );
    assert_eq!(data["nextOffset"], 1);
    assert_eq!(data["hasMore"], true);
    assert_eq!(data["total"], 7);
    assert_eq!(data["asOf"], "2026-08-15T20:00:00Z");
    let row = &data["entries"][0];
    assert_eq!(row["instrumentId"], "US.AAPL");
    assert_eq!(row["symbol"], "AAPL");
    assert_eq!(row["cells"]["code-column"]["columnId"], "code-column");
    assert_eq!(
        row["cells"]["code-column"]["value"],
        json!({"type":"string","string":"AAPL","unit":""})
    );
    assert_eq!(row["cells"]["price-column"]["columnId"], "price-column");
    assert_eq!(
        row["cells"]["price-column"]["value"],
        json!({"type":"number","number":189.25,"unit":"currency"})
    );
    handle.shutdown().await.expect("shutdown product");
    fixture.shutdown().await;
}

// Parity: go:452dea11:internal/api/productfeatures/research_screen_test.go:387 TestResearchScreenPostEmbeddedProviderConflictMatrix
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn production_http_screen_conflicts_reject_before_helper_and_forward_unsupported_market() {
    let directory = tempdir().expect("directory");
    let mut fixture = HelperFixture::new(
        409,
        json!({"error":{"code":"CAPABILITY_UNSUPPORTED","message":"stock screen market HK"}}),
    )
    .await;
    let handle = fixture.product(&directory).await;
    for (field, replacement, expected) in [
        ("catalogVersion", json!("futu-stock-screen-v1"), 409),
        ("direction", json!("abs_desc"), 409),
        ("operator", json!("gt"), 400),
    ] {
        let mut body = embedded_body();
        match field {
            "direction" => body["sorts"][0][field] = replacement,
            "operator" => body["conditions"][0][field] = replacement,
            _ => body[field] = replacement,
        }
        let (status, response) = screen_post(&handle, &body).await;
        assert_eq!(status, expected, "{field}: {response}");
        assert!(
            fixture.calls.lock().expect("calls").is_empty(),
            "{field} reached helper"
        );
    }
    let mut body = embedded_body();
    body["market"] = json!("HK");
    let (status, response) = screen_post(&handle, &body).await;
    assert_eq!(status, 409, "{response}");
    assert_eq!(response["error"]["code"], "BROKER_CAPABILITY_UNAVAILABLE");
    let calls = fixture.calls.lock().expect("calls").clone();
    assert_eq!(calls.len(), 1);
    assert_eq!(calls[0].1["market"], "HK");
    handle.shutdown().await.expect("shutdown product");
    fixture.shutdown().await;
}

// Parity: go:452dea11:internal/marketdata/screen_facade_test.go:78 TestServiceScreenPassesProviderErrorsThrough
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn production_http_screen_preserves_upstream_failure_message() {
    let directory = tempdir().expect("directory");
    let mut fixture = HelperFixture::new(
        500,
        json!({"error":{"code":"UPSTREAM_FAILED","message":"screen upstream failed"}}),
    )
    .await;
    let handle = fixture.product(&directory).await;
    let (status, response) = screen_post(&handle, &embedded_body()).await;
    assert_eq!(status, 502, "{response}");
    assert_eq!(response["error"]["code"], "BROKER_FEATURE_FAILED");
    assert_eq!(response["error"]["message"], "screen upstream failed");
    assert_eq!(fixture.calls.lock().expect("calls").len(), 1);
    handle.shutdown().await.expect("shutdown product");
    fixture.shutdown().await;
}

#[derive(Debug)]
struct ResultPort(Value);
impl ResearchScreenWritePort for ResultPort {
    fn query(&self, _: &ResearchScreenWriteQuery) -> Result<Value, ResearchScreenWritePortError> {
        Ok(self.0.clone())
    }
}

// Parity: go:452dea11:internal/api/productfeatures/research_screen_test.go:213 TestTypedResearchScreenResultOmitsUnknownTotal
#[tokio::test]
async fn http_screen_result_omits_unknown_total_and_preserves_cells_only_quote_currency() {
    for (input, known_total) in [
        (
            json!({"entries":[],"hasMore":false,"warnings":["combined A-share total is not exact"]}),
            None,
        ),
        (json!({"entries":[],"total":7}), Some(7)),
        (
            json!({"entries":[{"stockId":"80700","instrumentId":"HK.80700","market":"HK","symbol":"80700","name":"腾讯控股-R","quoteCurrency":"CNY","productClass":"equity"}]}),
            None,
        ),
    ] {
        let directory = tempdir().expect("directory");
        let mut config = ProductConfig::test_cutover(
            "127.0.0.1:0".parse().expect("address"),
            directory.path().join("settings.json"),
        )
        .expect("rehearsal config")
        .with_research_screen_write_port(Arc::new(ResultPort(input)));
        config.access = AccessPolicy::desktop(Some("a".repeat(32)));
        let handle = start_product(config).await.expect("projection rehearsal");
        let (status, response) = screen_post(&handle, &embedded_body()).await;
        assert_eq!(status, 200, "{response}");
        let data = &response["data"];
        match known_total {
            Some(total) => assert_eq!(data["total"], total),
            None => assert!(data.get("total").is_none(), "{data}"),
        }
        if let Some(row) = data["entries"].as_array().expect("rows").first() {
            assert_eq!(row["quoteCurrency"], "CNY");
            assert_eq!(row["cells"], json!({}));
            assert!(row.get("values").is_none());
        }
        handle.shutdown().await.expect("shutdown");
    }
}
