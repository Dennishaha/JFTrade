use super::*;

#[derive(Debug)]
struct TestPort;

impl ResearchScreenWritePort for TestPort {
    fn query(
        &self,
        _request: &ResearchScreenWriteQuery,
    ) -> Result<Value, ResearchScreenWritePortError> {
        Ok(json!({"entries": [], "hasMore": false}))
    }
}

#[test]
fn research_screen_route_inventory_has_only_the_post_route() {
    assert_eq!(
        research_screen_write_routes(),
        &[("POST", RESEARCH_SCREEN_PATH)]
    );
}

#[test]
fn decoder_rejects_unknown_and_trailing_json_before_the_port() {
    for body in [
        br#"{"brokerId":"api-test","unknown":true}"#.as_slice(),
        br#"{"brokerId":"api-test"} {}"#.as_slice(),
    ] {
        let response = dispatch_research_screen_write(
            &ResearchScreenWriteRequest {
                method: "POST".to_owned(),
                path: RESEARCH_SCREEN_PATH.to_owned(),
                body: Some(body.to_vec()),
            },
            Some(&TestPort),
            "2026-08-23T12:00:00Z",
        );
        assert_eq!(response.status, 400);
        assert_eq!(response.body["error"]["message"], INVALID_REQUEST_MESSAGE);
    }
}

#[test]
fn valid_request_fails_closed_without_a_query_port() {
    let response = dispatch_research_screen_write(
        &ResearchScreenWriteRequest {
            method: "POST".to_owned(),
            path: RESEARCH_SCREEN_PATH.to_owned(),
            body: Some(
                br#"{"brokerId":"api-test","market":"US","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}"#.to_vec(),
            ),
        },
        None,
        "2026-08-23T12:00:00Z",
    );
    assert_eq!(response.status, 503);
    assert_eq!(
        response.body["error"]["code"],
        "RESEARCH_SCREEN_UNAVAILABLE"
    );
}

#[test]
// Parity: go:452dea11:internal/api/productfeatures/research_screen_test.go:185 TestResearchScreenPostRejectsV1Payload
fn route_and_page_validation_precede_provider_calls() {
    let request = ResearchScreenWriteRequest {
        method: "POST".to_owned(),
        path: format!("{RESEARCH_SCREEN_PATH}?market=HK"),
        body: Some(
            br#"{"brokerId":"api-test","market":"US","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2,"page":{"offset":-1}}"#.to_vec(),
        ),
    };
    let response = dispatch_research_screen_write(&request, Some(&TestPort), "fixture-time");
    assert_eq!(response.status, 400);
    assert_eq!(
        response.body["error"]["message"],
        "page.offset must be non-negative"
    );
    let wrong_method = ResearchScreenWriteRequest {
        method: "GET".to_owned(),
        ..request
    };
    assert_eq!(
        dispatch_research_screen_write(&wrong_method, None, "fixture-time").status,
        404
    );
}

#[test]
fn futu_interval_factor_rejects_non_catalog_operator_before_provider_call() {
    let response = dispatch_research_screen_write(
        &ResearchScreenWriteRequest {
            method: "POST".to_owned(),
            path: RESEARCH_SCREEN_PATH.to_owned(),
            body: Some(
                br#"{
                    "brokerId":"futu",
                    "market":"US",
                    "catalogVersion":"futu-stock-screen-v1",
                    "querySchemaVersion":2,
                    "conditions":[{
                        "factor":{"factorKey":"simple.price"},
                        "operator":"gte",
                        "value":10
                    }]
                }"#
                .to_vec(),
            ),
        },
        Some(&TestPort),
        "fixture-time",
    );
    assert_eq!(response.status, 400);
    assert!(response.body["error"]["message"]
        .as_str()
        .is_some_and(|message| message.contains("operator")));
}

/// A port that fails loudly: stable-key/market validation must reject the
/// request before the provider port is ever consulted.
#[derive(Debug)]
struct UnreachablePort;

impl ResearchScreenWritePort for UnreachablePort {
    fn query(
        &self,
        _request: &ResearchScreenWriteQuery,
    ) -> Result<Value, ResearchScreenWritePortError> {
        panic!("provider port must not be called for an invalid definition")
    }
}

#[test]
fn research_screen_definition_rejects_unsupported_market_and_stable_keys() {
    for (body, expected) in [
        (
            r#"{"brokerId":"futu","market":"SG","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}"#,
            "market",
        ),
        (
            r#"{"brokerId":"futu","market":"HK","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2,"conditions":[{"factor":{"instanceId":"missing","factorKey":"missing.factor"},"operator":"between","value":1}]}"#,
            "missing.factor",
        ),
        (
            r#"{"brokerId":"futu","market":"HK","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2,"columns":[{"columnId":"x","factor":{"instanceId":"x","factorKey":"not.a.factor"}}]}"#,
            "not.a.factor",
        ),
        (
            r#"{"brokerId":"futu","market":"HK","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2,"sorts":[{"factor":{"factorKey":"simple.price"},"direction":"sideways"}]}"#,
            "direction",
        ),
        (
            r#"{"brokerId":"api-test","market":"US","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2,"page":{"limit":101}}"#,
            "page.limit",
        ),
    ] {
        let response = dispatch_research_screen_write(
            &ResearchScreenWriteRequest {
                method: "POST".to_owned(),
                path: RESEARCH_SCREEN_PATH.to_owned(),
                body: Some(body.as_bytes().to_vec()),
            },
            Some(&UnreachablePort),
            "fixture-time",
        );
        assert_eq!(response.status, 400, "{body}");
        let message = response.body["error"]["message"]
            .as_str()
            .unwrap_or_default();
        assert!(message.contains(expected), "{body} -> {message}");
    }
}

#[derive(Debug, Default)]
struct RecordingScreenPort {
    queries: std::sync::Mutex<Vec<ResearchScreenWriteQuery>>,
}

impl ResearchScreenWritePort for RecordingScreenPort {
    fn query(
        &self,
        request: &ResearchScreenWriteQuery,
    ) -> Result<Value, ResearchScreenWritePortError> {
        self.queries
            .lock()
            .expect("screen queries")
            .push(request.clone());
        Ok(json!({"entries": [], "hasMore": false}))
    }
}

/// Parity: go:452dea11:internal/assistant/assembly/product_adapters_test.go:155
/// TestProductExecutionAdapterNormalizesScreenAndCalendarV2Inputs (screen half).
///
/// Go defaults the screen page limit to 50, keeps the requested offset, and
/// projects the catalog version plus the column factor keys back to the
/// caller. The Rust port normalizes the same fields before the provider call.
// Parity: go:452dea11:internal/api/productfeatures/research_screen_test.go:47 TestNormalizeResearchScreenQueryDefaultsAndRejectsNonV2Input
#[test]
fn screen_query_defaults_the_page_and_keeps_catalog_columns() {
    let port = RecordingScreenPort::default();
    let response = dispatch_research_screen_write(
        &ResearchScreenWriteRequest {
            method: "POST".to_owned(),
            path: RESEARCH_SCREEN_PATH.to_owned(),
            body: Some(
                br#"{"brokerId":"api-test","market":"US","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2,"pool":{"watchlistStockIds":["1"]},"columns":[{"columnId":"close","factor":{"factorKey":"simple.last_close"}}],"page":{"offset":2}}"#
                    .to_vec(),
            ),
        },
        Some(&port),
        "fixture-time",
    );
    assert_eq!(response.status, 200, "{}", response.body);
    assert_eq!(response.body["data"]["catalogVersion"], "futu-stock-screen-v1");
    assert_eq!(response.body["data"]["columns"][0]["factorKey"], "simple.last_close");

    let queries = port.queries.lock().expect("screen queries");
    let query = queries.first().expect("screen query");
    assert_eq!(query.limit, 50, "Go defaults the page limit to 50");
    assert_eq!(query.offset, 2, "the requested offset is preserved");
    assert_eq!(query.market, "US");
    assert_eq!(query.definition["catalogVersion"], "futu-stock-screen-v1");
    assert_eq!(query.columns.len(), 1);
    assert_eq!(query.columns[0].column_id, "close");
    assert_eq!(query.columns[0].factor_key, "simple.last_close");
}

/// Parity: go:452dea11:internal/assistant/assembly/product_adapters_test.go:186
/// TestProductExecutionAdapterRejectsInvalidScreenPageAndValue (version half).
///
/// Go rejects a screen request whose `catalogVersion` is not the active
/// catalog and whose `querySchemaVersion` is not V2. Rust keeps the same
/// fail-closed gate and never consults the provider port.
// Parity: go:452dea11:internal/api/productfeatures/research_screen_test.go:47 TestNormalizeResearchScreenQueryDefaultsAndRejectsNonV2Input
#[test]
fn screen_query_rejects_wrong_catalog_and_schema_versions() {
    for (body, expected) in [
        (
            r#"{"brokerId":"api-test","market":"US","catalogVersion":"wrong","querySchemaVersion":2}"#,
            "catalogVersion",
        ),
        (
            r#"{"brokerId":"api-test","market":"US","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":1}"#,
            "querySchemaVersion",
        ),
        (
            r#"{"brokerId":"api-test","market":"US","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2,"page":{"limit":101}}"#,
            "page.limit",
        ),
    ] {
        let response = dispatch_research_screen_write(
            &ResearchScreenWriteRequest {
                method: "POST".to_owned(),
                path: RESEARCH_SCREEN_PATH.to_owned(),
                body: Some(body.as_bytes().to_vec()),
            },
            Some(&UnreachablePort),
            "fixture-time",
        );
        assert_eq!(response.status, 400, "{body}");
        let message = response.body["error"]["message"]
            .as_str()
            .unwrap_or_default();
        assert!(message.contains(expected), "{body} -> {message}");
    }
}
