//! Production implementation of the product-feature news search route.
//!
//! The public `/market-data/news` operation is a broker-feature projection,
//! while the provider helper exposes an instrument-scoped news endpoint.  This
//! adapter resolves the product query to that endpoint and projects the
//! provider-neutral payload without manufacturing entries.

use std::thread;

use jftrade_integration_marketdata_helper::{HelperClient, HttpAdapterError};
use jftrade_settings::MarketDataProvider;
use serde_json::{Map, Value, json};

use super::ProductionMarketDataNewsPort;
use crate::product::MarketDataNewsSearchReadSnapshotError;
use crate::product::product_query::QueryMap;

/// Dispatch the query-side news operation through the configured helper.
pub(crate) fn read(
    port: &ProductionMarketDataNewsPort,
    path: &str,
    query: &str,
) -> Result<Value, MarketDataNewsSearchReadSnapshotError> {
    if path != "/api/v1/market-data/news" {
        return Err(search_bad_request("unsupported market-data news path"));
    }
    let snapshot = port.active_provider_state.snapshot();
    let provider = snapshot.provider.ok_or_else(|| {
        MarketDataNewsSearchReadSnapshotError::Unavailable(
            "active market-data provider is not configured".to_owned(),
        )
    })?;
    let query_map = QueryMap::parse(query).map_err(|_| search_bad_request("invalid URL escape"))?;
    if !super::super::provider_request_matches(provider, &query_map) {
        let requested = query_map
            .get_first("brokerId")
            .or_else(|| query_map.get_first("providerBrokerId"))
            .unwrap_or_default();
        return Err(search_capability(&format!(
            "requested broker {requested:?} does not match active provider"
        )));
    }
    if provider == MarketDataProvider::Futu {
        if !snapshot.opend_ready {
            return Err(MarketDataNewsSearchReadSnapshotError::Unavailable(
                "Futu OpenD news runtime is not ready".to_owned(),
            ));
        }
        return read_futu_news(port.trade_runtime.as_ref(), query);
    }
    let provider_name = helper_provider(provider)?;
    if !snapshot.helper_ready {
        return Err(MarketDataNewsSearchReadSnapshotError::Unavailable(
            "market-data helper is not ready".to_owned(),
        ));
    }
    let helper = port.helper.clone().ok_or_else(|| {
        MarketDataNewsSearchReadSnapshotError::Unavailable(
            "market-data helper is not configured".to_owned(),
        )
    })?;
    let request = parse_search_request(query)?;
    if provider_name == "akshare" && !matches!(request.market.as_str(), "SH" | "SZ") {
        return Err(search_capability(
            "AKShare news search is only available for CN markets",
        ));
    }
    let result = fetch_news(helper, provider_name, request.clone())?;
    project_news(result, provider_name, request)
}

#[derive(Clone, Debug)]
struct NewsSearchRequest {
    market: String,
    symbol: String,
    limit: usize,
}

fn helper_provider(
    provider: MarketDataProvider,
) -> Result<&'static str, MarketDataNewsSearchReadSnapshotError> {
    match provider {
        MarketDataProvider::Yfinance => Ok("yfinance"),
        MarketDataProvider::Akshare => Ok("akshare"),
        MarketDataProvider::Futu => Err(search_capability(
            "Futu news search reader is not registered",
        )),
    }
}

pub(crate) fn read_futu_news(
    runtime: Option<
        &std::sync::Arc<super::super::product_production_ports_trade::SharedTradeReadRuntime>,
    >,
    query: &str,
) -> Result<Value, MarketDataNewsSearchReadSnapshotError> {
    let runtime = runtime.ok_or_else(|| {
        MarketDataNewsSearchReadSnapshotError::Unavailable(
            "Futu news runtime is not configured".to_owned(),
        )
    })?;
    if !runtime.news_reader_available() {
        return Err(MarketDataNewsSearchReadSnapshotError::Unavailable(
            "Futu news reader is not ready".to_owned(),
        ));
    }
    let query_map = QueryMap::parse(query).map_err(|_| search_bad_request("invalid URL escape"))?;
    let instrument = query_map
        .get_first("instrumentId")
        .or_else(|| query_map.get_first("instrument"))
        .map(str::trim)
        .filter(|value| !value.is_empty());
    let instrument_keyword = instrument
        .and_then(|value| value.rsplit_once('.').map(|(_, code)| code))
        .or(instrument)
        .map(str::trim)
        .filter(|value| !value.is_empty());
    let keyword = query_map
        .get_first("keyword")
        .or_else(|| query_map.get_first("query"))
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .or(instrument_keyword)
        .ok_or_else(|| search_bad_request("news search requires keyword or instrumentId"))?;
    let max_count = query_map
        .get_first("pageSize")
        .or_else(|| query_map.get_first("limit"))
        .and_then(|value| value.trim().parse::<i32>().ok())
        .map_or(10, |value| value.clamp(1, 50));
    let news_sub_type = query_map
        .get_first("newsSubType")
        .or_else(|| query_map.get_first("newsType"))
        .map(|value| {
            value
                .trim()
                .parse::<i32>()
                .map_err(|_| search_bad_request("newsSubType must be an integer"))
        })
        .transpose()?;
    let request = jftrade_integration_futu::FutuNewsQuery {
        keyword: keyword.to_owned(),
        max_count,
        news_sub_type,
    };
    let result = runtime.news(&request).map_err(map_futu_news_error)?;
    let entries = result
        .entries
        .into_iter()
        .map(|entry| {
            let mut value = Map::new();
            value.insert(
                "title".to_owned(),
                entry.title.map(Value::String).unwrap_or(Value::Null),
            );
            value.insert(
                "link".to_owned(),
                entry.url.map(Value::String).unwrap_or(Value::Null),
            );
            value.insert(
                "publisher".to_owned(),
                entry.source.map(Value::String).unwrap_or(Value::Null),
            );
            value.insert(
                "publishedAt".to_owned(),
                entry.published_at.map(Value::String).unwrap_or(Value::Null),
            );
            if let Some(view_count) = entry.view_count {
                value.insert("viewCount".to_owned(), json!(view_count));
            }
            if !entry.related_securities.is_empty() {
                value.insert(
                    "relatedSecurities".to_owned(),
                    json!(entry.related_securities),
                );
            }
            Value::Object(value)
        })
        .collect::<Vec<_>>();
    let as_of = entries
        .iter()
        .filter_map(|entry| entry.get("publishedAt").and_then(Value::as_str))
        .filter_map(|value| {
            time::OffsetDateTime::parse(value, &time::format_description::well_known::Rfc3339).ok()
        })
        .max()
        .and_then(|value| {
            value
                .format(&time::format_description::well_known::Rfc3339)
                .ok()
        })
        .unwrap_or_else(super::super::provider_now_rfc3339);
    let total = entries.len();
    let mut response = json!({
        "provider": {
            "brokerId": "futu",
            "securityFirm": "Futu/Moomoo via OpenD",
            "featureId": "research.news",
            "capability": "available",
            "selectionReason": "adapter_request",
            "resolvedAt": as_of,
            "asOf": as_of,
        },
        "asOf": as_of,
        "entries": entries,
        "hasMore": false,
        "total": total,
        "metadata": {"source": "futu-opend", "keyword": keyword},
    });
    if let Some(instrument) = instrument {
        let (market, code) = instrument
            .split_once('.')
            .map_or(("", instrument), |pair| pair);
        let market = market.trim().to_ascii_uppercase();
        let code = code.trim().to_ascii_uppercase();
        if !market.is_empty() && !code.is_empty() {
            response["resolvedInstrument"] = json!({
                "instrumentId": format!("{market}.{code}"),
                "code": code,
                "productClass": "unknown",
                "marketSegment": "securities",
                "quoteMarket": market,
                "tradeMarket": market,
                "quantityMode": "units",
            });
        }
    }
    Ok(response)
}

fn map_futu_news_error(message: String) -> MarketDataNewsSearchReadSnapshotError {
    if message.starts_with("invalid OpenD news query") {
        return search_bad_request(&message);
    }
    MarketDataNewsSearchReadSnapshotError::Failed {
        status: 502,
        code: "FUTU_NEWS_FAILED".to_owned(),
        message,
        retry_after_seconds: None,
    }
}

fn parse_search_request(
    query: &str,
) -> Result<NewsSearchRequest, MarketDataNewsSearchReadSnapshotError> {
    let query = QueryMap::parse(query).map_err(|_| search_bad_request("invalid URL escape"))?;
    let raw_instrument = query
        .get_first("instrumentId")
        .or_else(|| query.get_first("instrument"))
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| search_bad_request("instrumentId is required"))?;

    // Only a recognized market token introduces the qualified `MARKET.CODE`
    // form.  US symbols such as `BRK.B` are valid codes and must remain
    // intact instead of being mistaken for a market prefix.
    let (prefix, code) = raw_instrument
        .split_once('.')
        .filter(|(prefix, _)| is_market_token(prefix.trim()))
        .map_or((None, raw_instrument), |(prefix, code)| {
            (Some(prefix.trim()), code.trim())
        });
    if code.is_empty() || code.contains('/') || code.chars().any(char::is_whitespace) {
        return Err(search_bad_request("instrumentId must use MARKET.CODE form"));
    }
    let requested_market = query
        .get_first("market")
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .or(prefix)
        .ok_or_else(|| search_bad_request("market is required"))?
        .to_ascii_uppercase();
    let market = normalize_search_market(&requested_market, prefix)?;
    let symbol = normalize_search_symbol(&market, code);
    let limit = search_limit(&query);
    Ok(NewsSearchRequest {
        market,
        symbol,
        limit,
    })
}

fn is_market_token(value: &str) -> bool {
    matches!(
        value.to_ascii_uppercase().as_str(),
        "US" | "USA"
            | "NYSE"
            | "NASDAQ"
            | "AMEX"
            | "HK"
            | "HKEX"
            | "HKG"
            | "CN"
            | "SH"
            | "CNSH"
            | "SHH"
            | "SSE"
            | "SHSE"
            | "SZ"
            | "CNSZ"
            | "SHZ"
            | "SZSE"
            | "SHE"
    )
}

fn normalize_search_market(
    market: &str,
    instrument_prefix: Option<&str>,
) -> Result<String, MarketDataNewsSearchReadSnapshotError> {
    let market = match market {
        "USA" | "NYSE" | "NASDAQ" | "AMEX" => "US",
        "HKEX" | "HKG" => "HK",
        "CNSH" | "SHH" | "SSE" | "SHSE" => "SH",
        "CNSZ" | "SHZ" | "SZSE" | "SHE" => "SZ",
        value => value,
    };
    if market == "CN" {
        // CN is a UI aggregate.  Keep an exchange-qualified instrument on
        // its leaf market; a bare CN symbol cannot be routed unambiguously.
        if let Some(prefix) = instrument_prefix
            .map(str::trim)
            .map(str::to_ascii_uppercase)
        {
            let exchange = match prefix.as_str() {
                "SH" | "CNSH" | "SHH" | "SSE" | "SHSE" => Some("SH"),
                "SZ" | "CNSZ" | "SHZ" | "SZSE" | "SHE" => Some("SZ"),
                _ => None,
            };
            if let Some(exchange) = exchange {
                return Ok(exchange.to_owned());
            }
        }
        return Err(search_bad_request(
            "CN market requires an exchange-qualified instrumentId",
        ));
    }
    if matches!(market, "US" | "HK" | "SH" | "SZ") {
        Ok(market.to_owned())
    } else {
        Err(search_bad_request("unsupported market"))
    }
}

fn normalize_search_symbol(market: &str, code: &str) -> String {
    let code = code.trim().to_ascii_uppercase();
    if market == "HK" && code.chars().all(|value| value.is_ascii_digit()) && code.len() < 5 {
        format!("{code:0>5}")
    } else {
        code
    }
}

fn search_limit(query: &QueryMap) -> usize {
    // The Go feature facade prefers pageSize, treats malformed/non-positive
    // values as absent, then applies limit and finally the provider default.
    let raw = query
        .get_first("pageSize")
        .and_then(parse_positive)
        .or_else(|| query.get_first("limit").and_then(parse_positive));
    raw.unwrap_or(10).clamp(1, 50)
}

fn parse_positive(raw: &str) -> Option<usize> {
    raw.trim().parse::<usize>().ok().filter(|value| *value > 0)
}

fn fetch_news(
    helper: HelperClient,
    provider: &str,
    request: NewsSearchRequest,
) -> Result<Value, MarketDataNewsSearchReadSnapshotError> {
    let provider = provider.to_owned();
    let NewsSearchRequest {
        market,
        symbol,
        limit,
    } = request;
    let result = thread::spawn(move || {
        let runtime = tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .map_err(|error| HttpAdapterError::Unavailable(error.to_string()))?;
        let limit = limit.to_string();
        runtime.block_on(helper.get_provider_json_with_query::<Value>(
            &provider,
            &["news", market.as_str(), symbol.as_str()],
            &[("limit", limit.as_str())],
        ))
    })
    .join()
    .map_err(|_| {
        MarketDataNewsSearchReadSnapshotError::Unavailable(
            "market-data news helper task panicked".to_owned(),
        )
    })?;
    result.map_err(map_helper_error)
}

fn project_news(
    payload: Value,
    provider: &str,
    request: NewsSearchRequest,
) -> Result<Value, MarketDataNewsSearchReadSnapshotError> {
    let object = payload.as_object().ok_or_else(|| {
        search_bad_gateway("market-data helper returned a non-object news response")
    })?;
    // The Python sidecar serializes its Pydantic models with their native
    // snake_case field names (there is no alias generator on WireModel). Keep
    // the decoder tolerant of the camelCase form used by older helper
    // fixtures while projecting one stable camelCase product response.
    let response_market = required_string(object, &["market"])?.to_ascii_uppercase();
    let response_symbol = required_string(object, &["symbol"])?.to_ascii_uppercase();
    let response_id =
        required_string(object, &["instrumentId", "instrument_id"])?.to_ascii_uppercase();
    let expected_id = format!("{}.{}", request.market, request.symbol);
    if response_market != request.market
        || response_symbol != request.symbol
        || response_id != expected_id
    {
        return Err(search_bad_gateway(
            "news response identity does not match request",
        ));
    }
    let source = required_string(object, &["source"])?;
    let entries = project_entries(object.get("entries"))?;
    let latest = entries
        .iter()
        .filter_map(|entry| {
            entry
                .get("publishedAt")
                .or_else(|| entry.get("published_at"))
                .and_then(Value::as_str)
        })
        .filter_map(|value| {
            time::OffsetDateTime::parse(value, &time::format_description::well_known::Rfc3339).ok()
        })
        .max();
    let now = super::super::provider_now_rfc3339();
    let as_of = latest
        .and_then(|value| {
            value
                .format(&time::format_description::well_known::Rfc3339)
                .ok()
        })
        .unwrap_or_else(|| now.clone());
    let total = entries.len();
    let result = json!({
        "provider": {
            "brokerId": provider,
            "featureId": "research.news",
            "capability": "available",
            "selectionReason": "embedded-market-data-provider",
            "resolvedAt": now,
            "asOf": as_of,
        },
        "resolvedInstrument": {
            "instrumentId": expected_id,
            "code": request.symbol,
            "productClass": "unknown",
            "marketSegment": "securities",
            "quoteMarket": request.market,
            "tradeMarket": request.market,
            "quantityMode": "units",
        },
        "asOf": as_of,
        "entries": entries,
        "hasMore": false,
        "total": total,
        "metadata": {"source": source},
    });
    Ok(result)
}

fn required_string(
    object: &Map<String, Value>,
    keys: &[&str],
) -> Result<String, MarketDataNewsSearchReadSnapshotError> {
    keys.iter()
        .find_map(|key| {
            object
                .get(*key)
                .and_then(Value::as_str)
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .map(ToOwned::to_owned)
        })
        .ok_or_else(|| {
            search_bad_gateway(&format!(
                "market-data helper response is missing {}",
                keys[0]
            ))
        })
}

fn project_entries(
    value: Option<&Value>,
) -> Result<Vec<Value>, MarketDataNewsSearchReadSnapshotError> {
    let Some(value) = value else {
        return Ok(Vec::new());
    };
    let Some(entries) = value.as_array() else {
        // A nil Go slice serializes as null and is projected to an empty
        // feature list by the embedded facade.
        if value.is_null() {
            return Ok(Vec::new());
        }
        return Err(search_bad_gateway(
            "market-data helper response entries must be an array",
        ));
    };
    entries.iter().map(project_entry).collect()
}

fn project_entry(value: &Value) -> Result<Value, MarketDataNewsSearchReadSnapshotError> {
    let object = value
        .as_object()
        .ok_or_else(|| search_bad_gateway("news entry must be an object"))?;
    let mut projected = Map::new();
    for (source, target) in [
        ("title", "title"),
        ("link", "link"),
        ("publisher", "publisher"),
        ("summary", "summary"),
    ] {
        if let Some(value) = object.get(source).filter(|value| !value.is_null()) {
            if !value.is_string() {
                return Err(search_bad_gateway(&format!(
                    "news entry {source} must be a string"
                )));
            }
            projected.insert(target.to_owned(), value.clone());
        }
    }
    if let Some(value) = object
        .get("publishedAt")
        .or_else(|| object.get("published_at"))
        .filter(|value| !value.is_null())
    {
        let timestamp = value
            .as_str()
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| search_bad_gateway("news entry publishedAt must be RFC3339"))?;
        let parsed =
            time::OffsetDateTime::parse(timestamp, &time::format_description::well_known::Rfc3339)
                .map_err(|_| search_bad_gateway("news entry publishedAt must be RFC3339"))?;
        let normalized = parsed
            .to_offset(time::UtcOffset::UTC)
            .format(&time::format_description::well_known::Rfc3339)
            .map_err(|_| search_bad_gateway("news entry publishedAt must be RFC3339"))?;
        projected.insert("publishedAt".to_owned(), Value::String(normalized));
    }
    Ok(Value::Object(projected))
}

fn map_helper_error(error: HttpAdapterError) -> MarketDataNewsSearchReadSnapshotError {
    match error {
        HttpAdapterError::Remote {
            status,
            code,
            message,
            retry_after_seconds,
        } => MarketDataNewsSearchReadSnapshotError::Failed {
            status,
            code: if code.is_empty() {
                "BAD_GATEWAY".to_owned()
            } else {
                code
            },
            message,
            retry_after_seconds,
        },
        HttpAdapterError::Timeout => MarketDataNewsSearchReadSnapshotError::Failed {
            status: 504,
            code: "GATEWAY_TIMEOUT".to_owned(),
            message: "market-data helper request timed out".to_owned(),
            retry_after_seconds: None,
        },
        HttpAdapterError::InvalidResponse(message) => search_bad_gateway(&message),
        HttpAdapterError::Unavailable(message) => {
            MarketDataNewsSearchReadSnapshotError::Unavailable(message)
        }
        other => MarketDataNewsSearchReadSnapshotError::Failed {
            status: 500,
            code: "MARKET_DATA_NEWS_SEARCH_FAILED".to_owned(),
            message: other.to_string(),
            retry_after_seconds: None,
        },
    }
}

fn search_bad_request(message: &str) -> MarketDataNewsSearchReadSnapshotError {
    MarketDataNewsSearchReadSnapshotError::Failed {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: message.to_owned(),
        retry_after_seconds: None,
    }
}

fn search_bad_gateway(message: &str) -> MarketDataNewsSearchReadSnapshotError {
    MarketDataNewsSearchReadSnapshotError::Failed {
        status: 502,
        code: "BAD_GATEWAY".to_owned(),
        message: message.to_owned(),
        retry_after_seconds: None,
    }
}

fn search_capability(message: &str) -> MarketDataNewsSearchReadSnapshotError {
    MarketDataNewsSearchReadSnapshotError::Failed {
        status: 409,
        code: "CAPABILITY_UNAVAILABLE".to_owned(),
        message: message.to_owned(),
        retry_after_seconds: None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

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
}
