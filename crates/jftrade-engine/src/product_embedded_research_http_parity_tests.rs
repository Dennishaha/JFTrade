//! Original embedded research requests through production HTTP and helper owners.
use super::*;
use jftrade_integration_marketdata_helper::{HelperClient, HelperClientConfig};
use std::sync::Mutex;

const AUTH: &[(&str, &str)] = &[("Authorization", "Bearer aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")];

struct Fixture {
    client: HelperClient,
    replies: Arc<Mutex<BTreeMap<String, (u16, Value)>>>,
    calls: Arc<Mutex<Vec<String>>>,
    task: Option<tokio::task::JoinHandle<()>>,
}

impl Fixture {
    async fn new() -> Self {
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
            .await
            .expect("helper");
        let address = listener.local_addr().expect("address");
        let replies = Arc::new(Mutex::new(BTreeMap::<String, (u16, Value)>::new()));
        let calls = Arc::new(Mutex::new(Vec::new()));
        let responses = Arc::clone(&replies);
        let recorded = Arc::clone(&calls);
        let task = tokio::spawn(async move {
            loop {
                let (mut stream, _) = listener.accept().await.expect("accept");
                let target =
                    tokio::time::timeout(Duration::from_secs(3), helper_target(&mut stream))
                        .await
                        .expect("bounded headers");
                recorded.lock().expect("calls").push(target.clone());
                let (status, body) = responses
                    .lock()
                    .expect("replies")
                    .get(&target)
                    .cloned()
                    .unwrap_or_else(|| {
                        (
                            500,
                            json!({"error":{"code":"UNEXPECTED_FIXTURE_REQUEST","message":target}}),
                        )
                    });
                let body = body.to_string();
                let response = format!(
                    "HTTP/1.1 {status} Fixture\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
                    body.len()
                );
                stream.write_all(response.as_bytes()).await.expect("reply");
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
            .expect("client"),
            replies,
            calls,
            task: Some(task),
        }
    }

    fn reply(&self, target: &str, status: u16, body: Value) {
        self.replies
            .lock()
            .expect("replies")
            .insert(target.into(), (status, body));
    }

    fn calls(&self) -> Vec<String> {
        self.calls.lock().expect("calls").clone()
    }

    async fn product(&self, directory: &tempfile::TempDir) -> ProductHandle {
        let settings = directory.path().join("settings.json");
        std::fs::write(&settings, "{}").expect("settings");
        product_data_management::initialize_production_databases(&settings).expect("databases");
        let mut config = ProductConfig::desktop_production(
            "127.0.0.1:0".parse().expect("address"),
            &settings,
            "a".repeat(32),
        )
        .expect("config");
        config.capabilities = ProductCapabilities::all();
        let state = Arc::new(crate::product::ActiveProviderState::new(Some(
            jftrade_settings::MarketDataProvider::Yfinance,
        )));
        start_product(
            config
                .with_active_provider_state(state)
                .with_market_data_helper(self.client.clone()),
        )
        .await
        .expect("production product")
    }

    async fn shutdown(&mut self) {
        let task = self.task.take().expect("task");
        task.abort();
        assert!(task.await.expect_err("cancelled listener").is_cancelled());
    }
}

impl Drop for Fixture {
    fn drop(&mut self) {
        if let Some(task) = &self.task {
            task.abort();
        }
    }
}

async fn helper_target(stream: &mut tokio::net::TcpStream) -> String {
    let mut bytes = Vec::new();
    let mut chunk = [0; 1024];
    loop {
        let read = stream.read(&mut chunk).await.expect("headers");
        assert_ne!(read, 0, "headers ended early");
        bytes.extend_from_slice(&chunk[..read]);
        assert!(bytes.len() < 16 * 1024, "bounded headers");
        if bytes.windows(4).any(|window| window == b"\r\n\r\n") {
            break;
        }
    }
    let headers = String::from_utf8(bytes).expect("UTF-8 headers");
    let line = headers.lines().next().expect("request line");
    let mut parts = line.split_whitespace();
    assert_eq!(parts.next(), Some("GET"));
    parts.next().expect("target").to_owned()
}

async fn get(handle: &ProductHandle, target: &str) -> (u16, BTreeMap<String, String>, Value) {
    tokio::time::timeout(
        Duration::from_secs(5),
        request_json_with_status_and_headers(
            handle.startup_record().address,
            "GET",
            target,
            None,
            AUTH,
        ),
    )
    .await
    .expect("bounded HTTP request")
}

// Parity: go:452dea11:internal/api/productfeatures/provider_research_routes_test.go:179 TestEmbeddedProviderNewsAndCorporateActionRoutes
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn embedded_news_and_corporate_actions_http_preserve_original_projection() {
    let directory = tempdir().expect("directory");
    let mut fixture = Fixture::new().await;
    fixture.reply("/providers/yfinance/news/US/AAPL?limit=5", 200, json!({"market":"US","symbol":"AAPL","instrument_id":"US.AAPL","source":"yfinance-news","entries":[{"title":"Apple beats expectations","published_at":"2026-08-15T21:30:00Z"}]}));
    fixture.reply("/providers/yfinance/corporate-actions/US/AAPL", 200, json!({"market":"US","symbol":"AAPL","instrument_id":"US.AAPL","source":"yfinance-actions","events":[{"kind":"dividend","ex_date":"2026-08-10","amount":0.5}]}));
    let handle = fixture.product(&directory).await;
    let (status, _, news) = get(
        &handle,
        "/api/v1/market-data/news?brokerId=yfinance&instrumentId=US.AAPL&pageSize=5",
    )
    .await;
    assert_eq!(status, 200, "{news}");
    assert_eq!(news["ok"], true);
    assert_eq!(
        news["data"]["entries"][0]["title"],
        "Apple beats expectations"
    );
    assert_eq!(news["data"]["provider"]["brokerId"], "yfinance");
    assert_eq!(
        news["data"]["provider"]["selectionReason"],
        "embedded-market-data-provider"
    );
    let (status, _, actions) = get(&handle, "/api/v1/research/corporate-actions/US.AAPL").await;
    assert_eq!(status, 200, "{actions}");
    assert_eq!(actions["ok"], true);
    assert_eq!(actions["data"]["entries"][0]["statement"], "每股派息 0.5");
    assert_eq!(actions["data"]["entries"][0]["exDate"], "2026-08-10");
    assert_eq!(
        fixture.calls(),
        [
            "/providers/yfinance/news/US/AAPL?limit=5",
            "/providers/yfinance/corporate-actions/US/AAPL"
        ]
    );
    handle.shutdown().await.expect("shutdown");
    fixture.shutdown().await;
}

// Parity: go:452dea11:internal/api/productfeatures/provider_research_routes_test.go:229 TestEmbeddedProviderRouteErrorsKeepHTTPContract
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn embedded_news_actions_http_expose_original_lifecycle_error_boundaries() {
    let directory = tempdir().expect("directory");
    let mut fixture = Fixture::new().await;
    let target = "/providers/yfinance/news/US/AAPL?limit=10";
    let handle = fixture.product(&directory).await;
    fixture.reply(target, 409, json!({"error":{"code":"MARKET_DATA_CAPABILITY_UNSUPPORTED","message":"active provider akshare does not support instrument news"}}));
    let (status, _, response) = get(&handle, "/api/v1/market-data/news?instrumentId=US.AAPL").await;
    assert_eq!(status, 409, "{response}");
    assert_eq!(
        response["error"]["code"],
        "MARKET_DATA_CAPABILITY_UNSUPPORTED"
    );
    fixture.reply(
        target,
        503,
        json!({"error":{"code":"YFINANCE_RUNTIME_WARMING","message":"warming"}}),
    );
    let (status, headers, response) =
        get(&handle, "/api/v1/market-data/news?instrumentId=US.AAPL").await;
    assert_eq!(status, 503, "{response}");
    assert_eq!(headers.get("retry-after").map(String::as_str), Some("1"));
    assert_eq!(response["error"]["code"], "MARKET_DATA_PROVIDER_WARMING");
    fixture.reply(
        "/providers/yfinance/corporate-actions/SH/600519",
        503,
        json!({"error":{"code":"AKSHARE_POOL_BUSY","message":"busy"}}),
    );
    let (status, headers, response) =
        get(&handle, "/api/v1/research/corporate-actions/SH.600519").await;
    // Original SH input is rejected before the helper; its expected 503 is
    // retained as a mapping gap. Do not grant a cross-provider capability.
    assert_eq!(status, 409, "{response}");
    assert_eq!(response["error"]["code"], "BROKER_CAPABILITY_UNAVAILABLE");
    assert!(!headers.contains_key("retry-after"));
    assert_eq!(fixture.calls(), [target, target]);
    fixture.reply(
        "/providers/yfinance/corporate-actions/US/AAPL",
        503,
        json!({"error":{"code":"AKSHARE_POOL_BUSY","message":"busy"}}),
    );
    let (status, headers, response) =
        get(&handle, "/api/v1/research/corporate-actions/US.AAPL").await;
    assert_eq!(status, 503, "{response}");
    assert_eq!(headers.get("retry-after").map(String::as_str), Some("2"));
    assert_eq!(response["error"]["code"], "MARKET_DATA_PROVIDER_BUSY");
    assert_eq!(
        fixture.calls(),
        [
            target,
            target,
            "/providers/yfinance/corporate-actions/US/AAPL"
        ]
    );
    handle.shutdown().await.expect("shutdown");
    fixture.shutdown().await;
}

// Parity: go:452dea11:internal/api/productfeatures/provider_research_routes_test.go:264 TestEmbeddedProviderRankingsAndIndustryRoutes
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn embedded_rankings_and_industries_http_expose_original_provider_scope() {
    let directory = tempdir().expect("directory");
    let mut fixture = Fixture::new().await;
    fixture.reply("/providers/yfinance/rankings?market=US&kind=gainers&limit=10", 200, json!({"market":"US","kind":"gainers","source":"yfinance-rankings","entries":[{"instrument_id":"US.AAPL","name":"Apple Inc.","price":232.1,"change_rate":1.25}]}));
    let handle = fixture.product(&directory).await;
    let (status, _, rankings) = get(&handle, "/api/v1/research/rankings?brokerId=yfinance&market=US&operation=top_movers&direction=up&pageSize=10").await;
    assert_eq!(status, 200, "{rankings}");
    assert_eq!(rankings["data"]["entries"][0]["instrumentId"], "US.AAPL");
    assert_eq!(
        rankings["data"]["provider"]["selectionReason"],
        "embedded-market-data-provider"
    );
    let (status, _, boards) = get(&handle, "/api/v1/research/industries?brokerId=yfinance&market=CN&operation=plate_list&plateType=concept").await;
    assert_eq!(status, 409, "{boards}");
    assert_eq!(boards["error"]["code"], "BROKER_CAPABILITY_UNAVAILABLE");
    let (status, _, members) = get(&handle, "/api/v1/research/industries?brokerId=yfinance&market=CN&operation=plate_members&instrumentId=CN.%E5%8D%8A%E5%AF%BC%E4%BD%93&pageSize=20").await;
    assert_eq!(status, 409, "{members}");
    assert_eq!(members["error"]["code"], "BROKER_CAPABILITY_UNAVAILABLE");
    assert_eq!(
        fixture.calls(),
        ["/providers/yfinance/rankings?market=US&kind=gainers&limit=10"]
    );
    handle.shutdown().await.expect("shutdown");
    fixture.shutdown().await;
}

// Parity: go:452dea11:internal/api/productfeatures/provider_research_routes_test.go:370 TestEmbeddedProviderCompanyResearchRoutes
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn embedded_company_http_exposes_original_instrument_and_statement_scope() {
    let directory = tempdir().expect("directory");
    let mut fixture = Fixture::new().await;
    fixture.reply("/providers/yfinance/profile/US/AAPL", 200, json!({"market":"US","symbol":"AAPL","instrument_id":"US.AAPL","source":"yfinance-profile","groups":[{"title":"公司概要","fields":[{"name":"行业","value":"消费电子"}]}]}));
    fixture.reply("/providers/yfinance/analyst/US/AAPL", 200, json!({"market":"US","symbol":"AAPL","instrument_id":"US.AAPL","source":"yfinance-analyst","rating":4}));
    let handle = fixture.product(&directory).await;
    let (status, _, profile) = get(
        &handle,
        "/api/v1/research/instruments/US.AAPL?brokerId=yfinance&operation=profile",
    )
    .await;
    assert_eq!(status, 200, "{profile}");
    assert!(profile.to_string().contains("\"fieldType\":\"title\""));
    assert!(profile.to_string().contains("\"fieldType\":\"text\""));
    let (status, _, financials) = get(&handle, "/api/v1/research/financials/SH.600519?brokerId=yfinance&operation=statements&statement=cashflow").await;
    assert_eq!(status, 409, "{financials}");
    assert_eq!(financials["error"]["code"], "BROKER_CAPABILITY_UNAVAILABLE");
    let (status, _, analyst) = get(
        &handle,
        "/api/v1/research/analyst/US.AAPL?brokerId=yfinance&operation=consensus",
    )
    .await;
    assert_eq!(status, 200, "{analyst}");
    assert!(analyst.to_string().contains("\"rating\":4"));
    let (status, _, ownership) = get(
        &handle,
        "/api/v1/research/ownership/SH.600519?brokerId=yfinance&operation=overview",
    )
    .await;
    assert_eq!(status, 409, "{ownership}");
    assert_eq!(ownership["error"]["code"], "BROKER_CAPABILITY_UNAVAILABLE");
    assert_eq!(
        fixture.calls(),
        [
            "/providers/yfinance/profile/US/AAPL",
            "/providers/yfinance/analyst/US/AAPL"
        ]
    );
    handle.shutdown().await.expect("shutdown");
    fixture.shutdown().await;
}

// Parity: go:452dea11:internal/api/productfeatures/provider_research_routes_test.go:498 TestEmbeddedProviderCalendarAndMacroRoutes
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn embedded_calendar_macro_http_exposes_original_active_provider_scope() {
    let directory = tempdir().expect("directory");
    let mut fixture = Fixture::new().await;
    let handle = fixture.product(&directory).await;
    let targets = [
        "/api/v1/research/calendars?brokerId=yfinance&market=CN&operation=earnings&beginDate=2026-08-01&endDate=2026-08-31",
        "/api/v1/research/calendars?brokerId=yfinance&market=SH&operation=dividends&date=2026-08-15",
        "/api/v1/research/calendars?brokerId=yfinance&market=SH&operation=economic&beginDate=2026-08-01&endDate=2026-08-07",
        "/api/v1/research/calendars?brokerId=yfinance&market=CN&operation=ipos",
        "/api/v1/research/macro?brokerId=yfinance&market=US&operation=indicators",
        "/api/v1/research/macro?brokerId=yfinance&market=US&operation=indicator_history&indicatorId=cpi_yoy&pageSize=60",
    ];
    for target in targets {
        let (status, _, response) = get(&handle, target).await;
        assert_eq!(status, 409, "{target}: {response}");
        assert_eq!(response["error"]["code"], "BROKER_CAPABILITY_UNAVAILABLE");
        assert!(
            response["error"]["message"]
                .as_str()
                .expect("message")
                .contains("provider")
        );
    }
    assert!(fixture.calls().is_empty(), "no cross-provider fallback");
    handle.shutdown().await.expect("shutdown");
    fixture.shutdown().await;
}
