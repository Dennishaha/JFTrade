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
