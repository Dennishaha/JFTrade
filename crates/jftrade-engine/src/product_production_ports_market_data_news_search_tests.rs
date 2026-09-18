use super::*;
use crate::product::MarketDataNewsSearchReadSnapshotPort;
use crate::product::product_active_provider_state::ActiveProviderState;
use std::sync::Arc;

#[test]
fn parser_matches_product_precedence_and_market_override() {
    let request = parse_search_request("instrumentId=us.aapl&market=hk&pageSize=500&limit=2")
        .expect("search request");
    assert_eq!(request.market, "HK");
    assert_eq!(request.symbol, "AAPL");
    assert_eq!(request.limit, 50);

    let request =
        parse_search_request("instrumentId=sh.600519&limit=7").expect("instrument request");
    assert_eq!(request.market, "SH");
    assert_eq!(request.symbol, "600519");
    assert_eq!(request.limit, 7);
}

#[test]
fn parser_rejects_missing_or_ambiguous_instrument() {
    let error = parse_search_request("market=US&limit=5").expect_err("missing instrument");
    assert!(matches!(
        error,
        MarketDataNewsSearchReadSnapshotError::Failed { status: 400, .. }
    ));

    let error = parse_search_request("instrumentId=CN.600519&market=CN")
        .expect_err("bare CN instrument");
    assert!(matches!(
        error,
        MarketDataNewsSearchReadSnapshotError::Failed { status: 400, .. }
    ));
}

#[test]
fn projection_accepts_sidecar_snake_case_and_keeps_public_camel_case() {
    let payload = json!({
        "market": "US",
        "symbol": "AAPL",
        "instrument_id": "US.AAPL",
        "entries": [{
            "title": "Headline",
            "published_at": "2026-08-15T14:30:00+08:00"
        }],
        "source": "yfinance-news"
    });
    let result = project_news(
        payload,
        "yfinance",
        NewsSearchRequest {
            market: "US".to_owned(),
            symbol: "AAPL".to_owned(),
            limit: 10,
        },
    )
    .expect("project sidecar response");
    assert_eq!(result["entries"][0]["publishedAt"], "2026-08-15T06:30:00Z");
    assert!(result["entries"][0].get("published_at").is_none());
    assert_eq!(result["metadata"]["source"], "yfinance-news");
    assert!(result["metadata"].get("providerGeneration").is_none());
}

#[test]
fn projection_treats_null_entries_as_empty_and_rejects_identity_drift() {
    let payload = json!({
        "market": "US",
        "symbol": "AAPL",
        "instrument_id": "US.AAPL",
        "entries": null,
        "source": "yfinance-news"
    });
    let result = project_news(
        payload,
        "yfinance",
        NewsSearchRequest {
            market: "US".to_owned(),
            symbol: "AAPL".to_owned(),
            limit: 10,
        },
    )
    .expect("null entries projection");
    assert_eq!(result["entries"], json!([]));
    assert_eq!(result["total"], 0);

    let error = project_news(
        json!({
            "market": "HK",
            "symbol": "AAPL",
            "instrument_id": "HK.AAPL",
            "entries": [],
            "source": "yfinance-news"
        }),
        "yfinance",
        NewsSearchRequest {
            market: "US".to_owned(),
            symbol: "AAPL".to_owned(),
            limit: 10,
        },
    )
    .expect_err("identity mismatch");
    assert!(matches!(
        error,
        MarketDataNewsSearchReadSnapshotError::Failed { status: 502, .. }
    ));
}

/// Parity: go:452dea11:internal/productfeatures/provider_projection_test.go:14
/// TestProviderNewsProjectionMapsNullableFieldsAndAsOf
///
/// Go omits every nil nullable field, keeps fully-null entries as empty
/// documents, stamps `asOf` from the newest `publishedAt` and reports the
/// embedded attribution envelope. Rust projects the same shape from the
/// helper payload, normalizing timestamps to UTC.
#[test]
fn projection_maps_nullable_fields_and_as_of() {
    let payload = json!({
        "market": "US",
        "symbol": "AAPL",
        "instrument_id": "US.AAPL",
        "source": "yfinance-news",
        "entries": [
            {"title": "Apple beats expectations", "link": "https://example.com/aapl", "published_at": "2026-08-15T21:30:00Z"},
            {"published_at": "2026-08-14T10:00:00Z"},
            {}
        ]
    });
    let result = project_news(
        payload,
        "yfinance",
        NewsSearchRequest {
            market: "US".to_owned(),
            symbol: "AAPL".to_owned(),
            limit: 10,
        },
    )
    .expect("project news");
    let entries = result["entries"].as_array().expect("entries");
    assert_eq!(entries.len(), 3);
    assert_eq!(entries[0]["title"], "Apple beats expectations");
    assert_eq!(entries[0]["link"], "https://example.com/aapl");
    assert_eq!(entries[0]["publishedAt"], "2026-08-15T21:30:00Z");
    assert!(entries[0].get("publisher").is_none());
    assert!(entries[0].get("summary").is_none());
    assert_eq!(entries[2], json!({}));
    assert_eq!(result["asOf"], "2026-08-15T21:30:00Z");
    assert_eq!(result["total"], 3);
    assert_eq!(result["hasMore"], false);
    assert_eq!(result["metadata"]["source"], "yfinance-news");
    assert_eq!(result["provider"]["brokerId"], "yfinance");
    assert_eq!(result["provider"]["featureId"], "research.news");
    assert_eq!(result["provider"]["capability"], "available");
    assert_eq!(
        result["provider"]["selectionReason"],
        "embedded-market-data-provider"
    );
    assert_eq!(result["provider"]["asOf"], result["asOf"]);
    assert_eq!(result["resolvedInstrument"]["instrumentId"], "US.AAPL");
    assert_eq!(result["resolvedInstrument"]["code"], "AAPL");
    assert_eq!(result["resolvedInstrument"]["quoteMarket"], "US");
}

/// Parity: go:452dea11:internal/productfeatures/provider_projection_test.go:79
/// TestProviderNewsProjectionFallsBackToNowWithoutTimestamps
#[test]
fn projection_falls_back_to_now_without_timestamps() {
    let payload = json!({
        "market": "SH",
        "symbol": "600519",
        "instrument_id": "SH.600519",
        "source": "akshare-news",
        "entries": [{"title": "untimed"}]
    });
    let result = project_news(
        payload,
        "akshare",
        NewsSearchRequest {
            market: "SH".to_owned(),
            symbol: "600519".to_owned(),
            limit: 10,
        },
    )
    .expect("project news");
    let as_of = result["asOf"].as_str().expect("asOf");
    assert!(
        time::OffsetDateTime::parse(as_of, &time::format_description::well_known::Rfc3339)
            .is_ok(),
        "asOf must be RFC3339, got {as_of}"
    );
    assert_eq!(result["provider"]["asOf"], result["asOf"]);
}

/// Parity: go:452dea11:internal/productfeatures/provider_projection_test.go:139
/// TestEmbeddedNewsLimitPrecedenceAndClamp
///
/// The embedded news facade prefers an explicit pageSize, then the legacy
/// `limit` param, then the provider default, clamped to [1, MaxNewsLimit].
#[test]
fn embedded_news_limit_precedence_and_clamp() {
    let request =
        parse_search_request("instrumentId=US.AAPL&pageSize=5").expect("explicit pageSize");
    assert_eq!(request.limit, 5);

    let request =
        parse_search_request("instrumentId=US.AAPL&pageSize=500").expect("clamped pageSize");
    assert_eq!(request.limit, 50);

    let request =
        parse_search_request("instrumentId=US.AAPL&limit=7").expect("legacy limit param");
    assert_eq!(request.limit, 7);

    let request = parse_search_request("instrumentId=US.AAPL").expect("default limit");
    assert_eq!(request.limit, 10);

    let request =
        parse_search_request("instrumentId=US.AAPL&limit=-3").expect("negative limit param");
    assert_eq!(request.limit, 10);

    let request = parse_search_request("instrumentId=US.AAPL&pageSize=bad&limit=3")
        .expect("malformed pageSize falls back to limit");
    assert_eq!(request.limit, 3);
}

/// Parity: go:452dea11:internal/api/productfeatures/provider_research_routes_test.go:229
/// TestEmbeddedProviderRouteErrorsKeepHTTPContract
///
/// The product-feature facade folds the market-data capability sentinel into
/// the broker capability contract: HTTP 409 with the prefixed code the
/// console's provider-unsupported fallback keys on.
/// Parity: go:452dea11:internal/productfeatures/service_test.go:30
/// TestQueryDoesNotFallbackWhenBrokerIsExplicit
///
/// The query-style `/api/v1/market-data/news` route belongs to the
/// product-feature family, so an explicit `brokerId` naming a provider other
/// than the active one must answer 409 `BROKER_CAPABILITY_UNAVAILABLE` — the
/// product-feature wire code — before any helper read, and must never fall back
/// to the registered broker.
#[test]
fn query_style_news_route_rejects_a_non_active_explicit_broker() {
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Yfinance)));
    state.set_readiness(true, false, false);
    let port = ProductionMarketDataNewsPort {
        active_provider_state: state,
        helper: None,
        trade_runtime: None,
    };
    for requested in ["akshare", "futu"] {
        let error = MarketDataNewsSearchReadSnapshotPort::read(
            &port,
            "/api/v1/market-data/news",
            &format!("instrumentId=US.AAPL&brokerId={requested}"),
        )
        .expect_err("a non-active explicit broker must not fall back");
        match error {
            MarketDataNewsSearchReadSnapshotError::Failed {
                status,
                code,
                message,
                ..
            } => {
                assert_eq!(status, 409);
                assert_eq!(code, "BROKER_CAPABILITY_UNAVAILABLE", "broker {requested}");
                assert!(
                    message.contains("does not match active provider"),
                    "broker {requested} message = {message}"
                );
            }
            other => panic!("expected the capability rejection, got {other:?}"),
        }
    }
}

#[test]
fn embedded_provider_capability_error_uses_the_broker_code() {
    let error = search_capability(
        "market-data capability is unsupported: active provider \"akshare\" does not support instrument news",
    );
    match error {
        MarketDataNewsSearchReadSnapshotError::Failed {
            status,
            code,
            message,
            retry_after_seconds,
        } => {
            assert_eq!(status, 409);
            assert_eq!(code, "BROKER_CAPABILITY_UNAVAILABLE");
            assert!(message.contains("instrument news"), "message = {message}");
            assert_eq!(retry_after_seconds, None);
        }
        other => panic!("expected a capability failure, got {other:?}"),
    }
}
