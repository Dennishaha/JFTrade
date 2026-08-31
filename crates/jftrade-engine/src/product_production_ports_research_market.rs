//! Production adapters for market-scoped research helper routes.
//!
//! Rankings and industry boards intentionally bypass the broker facade.  The
//! helper response is validated and projected into the public FeatureResult
//! envelope so the Rust path has the same wire shape as the Go embedded feed.

use std::thread;

use jftrade_integration_marketdata_helper::{HelperClient, HttpAdapterError};
use jftrade_settings::MarketDataProvider;
use serde_json::{Map, Value, json};

use crate::product::ResearchReadSnapshotError;
use crate::product::product_query::QueryMap;

const DEFAULT_LIMIT: usize = 20;
const MAX_LIMIT: usize = 100;

/// Read rankings, industry boards, or board members from the configured
/// yfinance/AKShare helper.  Futu has no embedded implementation and is
/// reported as a capability error rather than a synthetic empty result.
pub(crate) fn read_market_research(
    provider: MarketDataProvider,
    helper_ready: bool,
    helper: Option<&HelperClient>,
    path: &str,
    query: &str,
) -> Result<Value, ResearchReadSnapshotError> {
    let query = QueryMap::parse(query)
        .map_err(|_| ResearchReadSnapshotError::Invalid("invalid URL escape".to_owned()))?;
    if provider == MarketDataProvider::Futu {
        return Err(capability("research", "futu"));
    }
    if !helper_ready {
        return Err(ResearchReadSnapshotError::Unavailable(
            "market-data helper is not ready".to_owned(),
        ));
    }
    let helper = helper.ok_or_else(|| {
        ResearchReadSnapshotError::Unavailable("market-data helper is not configured".to_owned())
    })?;
    match path {
        "/api/v1/research/rankings" => read_rankings(provider, helper, &query),
        "/api/v1/research/industries" => read_industries(provider, helper, &query),
        _ => Err(ResearchReadSnapshotError::Unavailable(
            "unsupported market research route".to_owned(),
        )),
    }
}

fn read_rankings(
    provider: MarketDataProvider,
    helper: &HelperClient,
    query: &QueryMap,
) -> Result<Value, ResearchReadSnapshotError> {
    let operation = query
        .get_first("operation")
        .unwrap_or("")
        .trim()
        .to_ascii_lowercase();
    let (kind, feature_id) = match operation.as_str() {
        "top_movers" => {
            let direction = query
                .get_first("direction")
                .unwrap_or("up")
                .trim()
                .to_ascii_lowercase();
            let kind = if direction == "down" {
                "losers"
            } else {
                "gainers"
            };
            (kind, "research.rankings")
        }
        "hot" => ("active", "research.rankings"),
        // The Go facade renders heatmap through the industry feed while
        // retaining the rankings feature id.
        "heatmap" => return read_industry_boards(provider, helper, query, "research.rankings"),
        _ => return Err(capability("research.rankings", &operation)),
    };
    let market = requested_market(provider, query, false)?;
    let limit = request_limit(query)?;
    ensure_ranking_market(provider, &market)?;
    let provider_name = helper_provider(provider)?;
    let payload = fetch_json(
        helper,
        provider_name,
        &["rankings"],
        vec![
            ("market", market.clone()),
            ("kind", kind.to_owned()),
            ("limit", limit.to_string()),
        ],
    )?;
    let (entries, source, _response_market) = ranking_entries(&payload, &market, kind)?;
    Ok(feature_result(
        provider_name,
        feature_id,
        entries,
        source,
        None,
    ))
}

fn read_industries(
    provider: MarketDataProvider,
    helper: &HelperClient,
    query: &QueryMap,
) -> Result<Value, ResearchReadSnapshotError> {
    let operation = query
        .get_first("operation")
        .unwrap_or("")
        .trim()
        .to_ascii_lowercase();
    match operation.as_str() {
        "plate_list" => read_industry_boards(provider, helper, query, "research.industry"),
        "plate_members" => read_industry_members(provider, helper, query),
        _ => Err(capability("research.industry", &operation)),
    }
}

fn read_industry_boards(
    provider: MarketDataProvider,
    helper: &HelperClient,
    query: &QueryMap,
    feature_id: &str,
) -> Result<Value, ResearchReadSnapshotError> {
    let market = requested_market(provider, query, true)?;
    ensure_industry_market(&market)?;
    let kind = board_kind(query)?;
    let provider_name = helper_provider(provider)?;
    if provider != MarketDataProvider::Akshare {
        return Err(capability(feature_id, "plate_list"));
    }
    let payload = fetch_json(
        helper,
        provider_name,
        &["industries"],
        vec![("kind", kind.to_owned()), ("market", market.clone())],
    )?;
    let (entries, source, _response_market) = industry_board_entries(&payload, &market, kind)?;
    Ok(feature_result(
        provider_name,
        feature_id,
        entries,
        source,
        None,
    ))
}

fn read_industry_members(
    provider: MarketDataProvider,
    helper: &HelperClient,
    query: &QueryMap,
) -> Result<Value, ResearchReadSnapshotError> {
    let provider_name = helper_provider(provider)?;
    if provider != MarketDataProvider::Akshare {
        return Err(capability("research.industry", "plate_members"));
    }
    let (market, board) = member_board(query)?;
    ensure_industry_market(&market)?;
    let kind = member_board_kind(query)?;
    let limit = request_limit(query)?;
    let mut extra = vec![("limit", limit.to_string()), ("market", market.clone())];
    if !kind.is_empty() {
        extra.push(("kind", kind.to_owned()));
    }
    let payload = fetch_json(
        helper,
        provider_name,
        &["industries", board.as_str(), "members"],
        extra,
    )?;
    let (entries, source, _response_market) = member_entries(
        &payload,
        &board,
        &market,
        (!kind.is_empty()).then_some(kind),
    )?;
    Ok(feature_result(
        provider_name,
        "research.industry",
        entries,
        source,
        Some(resolved_plate_instrument(&market, &board)),
    ))
}

fn helper_provider(
    provider: MarketDataProvider,
) -> Result<&'static str, ResearchReadSnapshotError> {
    match provider {
        MarketDataProvider::Yfinance => Ok("yfinance"),
        MarketDataProvider::Akshare => Ok("akshare"),
        MarketDataProvider::Futu => Err(capability("research", "futu")),
    }
}

fn requested_market(
    provider: MarketDataProvider,
    query: &QueryMap,
    industry: bool,
) -> Result<String, ResearchReadSnapshotError> {
    let fallback = if industry || provider == MarketDataProvider::Akshare {
        "CN"
    } else {
        "US"
    };
    let market = query
        .get_first("market")
        .unwrap_or(fallback)
        .trim()
        .to_ascii_uppercase();
    if market.is_empty() {
        return Err(invalid("market is required"));
    }
    Ok(market)
}

fn ensure_ranking_market(
    provider: MarketDataProvider,
    market: &str,
) -> Result<(), ResearchReadSnapshotError> {
    let supported = match provider {
        MarketDataProvider::Yfinance => market == "US",
        MarketDataProvider::Akshare => matches!(market, "CN" | "SH" | "SZ" | "HK"),
        MarketDataProvider::Futu => false,
    };
    supported
        .then_some(())
        .ok_or_else(|| capability("research.rankings", market))
}

fn ensure_industry_market(market: &str) -> Result<(), ResearchReadSnapshotError> {
    matches!(market, "CN" | "SH" | "SZ")
        .then_some(())
        .ok_or_else(|| capability("research.industry", market))
}

fn board_kind(query: &QueryMap) -> Result<&'static str, ResearchReadSnapshotError> {
    match query
        .get_first("plateType")
        .unwrap_or("")
        .trim()
        .to_ascii_lowercase()
        .as_str()
    {
        "" | "industry" => Ok("industry"),
        "concept" => Ok("concept"),
        value => Err(capability("research.industry", value)),
    }
}

fn member_board_kind(query: &QueryMap) -> Result<&'static str, ResearchReadSnapshotError> {
    match query
        .get_first("plateType")
        .unwrap_or("")
        .trim()
        .to_ascii_lowercase()
        .as_str()
    {
        "" => Ok(""),
        "industry" => Ok("industry"),
        "concept" => Ok("concept"),
        value => Err(capability("research.industry", value)),
    }
}

fn member_board(query: &QueryMap) -> Result<(String, String), ResearchReadSnapshotError> {
    let instrument = query
        .get_first("instrumentId")
        .map(str::trim)
        .filter(|v| !v.is_empty());
    let requested = query
        .get_first("market")
        .map(str::trim)
        .filter(|v| !v.is_empty());
    let (market, board) = if let Some(instrument) = instrument {
        let (market, board) = instrument
            .split_once('.')
            .ok_or_else(|| invalid("plate_members requires a plate instrumentId"))?;
        (market.to_ascii_uppercase(), board.trim().to_owned())
    } else if let Some(board) = query
        .get_first("board")
        .or_else(|| query.get_first("plateId"))
        .map(str::trim)
        .filter(|v| !v.is_empty())
    {
        (
            requested.unwrap_or("CN").to_ascii_uppercase(),
            board.to_owned(),
        )
    } else {
        return Err(invalid("plate_members requires a plate instrumentId"));
    };
    if let Some(requested) = requested
        && requested.to_ascii_uppercase() != market
    {
        return Err(invalid("market does not match instrumentId"));
    }
    if board.is_empty() || board.contains('/') || board.contains('.') {
        return Err(invalid("plate_members board is invalid"));
    }
    Ok((market, board))
}

fn request_limit(query: &QueryMap) -> Result<usize, ResearchReadSnapshotError> {
    // Go's embeddedRankingsLimit receives pageSize through strconv.Atoi and
    // treats zero, negatives, and malformed values as "not supplied".  It
    // then falls back to the legacy limit query before applying the default
    // and the provider's [1, 100] bounds.  Keep the same precedence here so a
    // stale pageSize cannot hide an explicitly supplied legacy limit.
    let positive = |value: Option<&str>| {
        value
            .and_then(|value| value.trim().parse::<i64>().ok())
            .filter(|value| *value > 0)
            .and_then(|value| usize::try_from(value).ok())
    };
    let parsed = positive(query.get_first("pageSize"))
        .or_else(|| positive(query.get_first("limit")))
        .unwrap_or(DEFAULT_LIMIT);
    Ok(parsed.clamp(1, MAX_LIMIT))
}

fn fetch_json(
    helper: &HelperClient,
    provider: &str,
    segments: &[&str],
    query: Vec<(&str, String)>,
) -> Result<Value, ResearchReadSnapshotError> {
    let helper = helper.clone();
    let provider = provider.to_owned();
    let query = query
        .into_iter()
        .map(|(key, value)| (key.to_owned(), value))
        .collect::<Vec<_>>();
    let segments = segments.iter().map(|s| (*s).to_owned()).collect::<Vec<_>>();
    let result = thread::spawn(move || {
        let runtime = tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .map_err(|error| HttpAdapterError::Unavailable(error.to_string()))?;
        let segment_refs = segments.iter().map(String::as_str).collect::<Vec<_>>();
        let query_refs = query
            .iter()
            .map(|(key, value)| (key.as_str(), value.as_str()))
            .collect::<Vec<_>>();
        runtime.block_on(helper.get_provider_json_with_query::<Value>(
            &provider,
            &segment_refs,
            &query_refs,
        ))
    })
    .join()
    .map_err(|_| {
        ResearchReadSnapshotError::Unavailable("research helper task panicked".to_owned())
    })?;
    result.map_err(map_helper_error)
}

fn ranking_entries(
    payload: &Value,
    expected_market: &str,
    expected_kind: &str,
) -> Result<(Vec<Value>, String, String), ResearchReadSnapshotError> {
    let object = payload
        .as_object()
        .ok_or_else(|| bad_gateway("market rankings response must be an object"))?;
    let entries = object
        .get("entries")
        .and_then(Value::as_array)
        .ok_or_else(|| bad_gateway("market rankings response is missing entries"))?;
    let projected = entries
        .iter()
        .map(project_ranking_entry)
        .collect::<Result<Vec<_>, _>>()?;
    let market = object
        .get("market")
        .and_then(Value::as_str)
        .filter(|v| !v.trim().is_empty())
        .ok_or_else(|| bad_gateway("market rankings response is missing market"))?
        .to_ascii_uppercase();
    if market != expected_market {
        return Err(bad_gateway(
            "market rankings response market does not match request",
        ));
    }
    let kind = object
        .get("kind")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|v| !v.is_empty())
        .ok_or_else(|| bad_gateway("market rankings response is missing kind"))?;
    if !kind.eq_ignore_ascii_case(expected_kind) {
        return Err(bad_gateway(
            "market rankings response kind does not match request",
        ));
    }
    let source = object
        .get("source")
        .and_then(Value::as_str)
        .filter(|v| !v.trim().is_empty())
        .unwrap_or("market-data-rankings")
        .to_owned();
    Ok((projected, source, market))
}

fn industry_board_entries(
    payload: &Value,
    expected_market: &str,
    expected_kind: &str,
) -> Result<(Vec<Value>, String, String), ResearchReadSnapshotError> {
    let object = payload
        .as_object()
        .ok_or_else(|| bad_gateway("industry response must be an object"))?;
    let boards = object
        .get("boards")
        .and_then(Value::as_array)
        .ok_or_else(|| bad_gateway("industry response is missing boards"))?;
    let market = object
        .get("market")
        .and_then(Value::as_str)
        .filter(|v| !v.trim().is_empty())
        .ok_or_else(|| bad_gateway("industry response is missing market"))?
        .to_ascii_uppercase();
    if market != expected_market {
        return Err(bad_gateway(
            "industry response market does not match request",
        ));
    }
    let kind = object
        .get("kind")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|v| !v.is_empty())
        .ok_or_else(|| bad_gateway("industry response is missing kind"))?;
    if !kind.eq_ignore_ascii_case(expected_kind) {
        return Err(bad_gateway("industry response kind does not match request"));
    }
    let mut projected = Vec::with_capacity(boards.len());
    for board in boards {
        let object = board
            .as_object()
            .ok_or_else(|| bad_gateway("industry board entry must be an object"))?;
        let name = object
            .get("name")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|v| !v.is_empty())
            .ok_or_else(|| bad_gateway("industry board entry is missing name"))?;
        let mut value = Map::new();
        value.insert("instrumentId".to_owned(), json!(format!("{market}.{name}")));
        value.insert("market".to_owned(), json!(market));
        value.insert("name".to_owned(), json!(name));
        value.insert("productClass".to_owned(), json!("plate"));
        copy_field(object, &mut value, "change_rate", "changeRate");
        copy_field(object, &mut value, "turnover", "turnover");
        copy_field(object, &mut value, "volume", "volume");
        copy_field(object, &mut value, "leading_stock_name", "leadingStockName");
        copy_field(
            object,
            &mut value,
            "leading_stock_change_rate",
            "leadingStockChangeRate",
        );
        projected.push(Value::Object(value));
    }
    let source = object_source(payload, "market-data-industries");
    Ok((projected, source, market))
}

fn member_entries(
    payload: &Value,
    expected_board: &str,
    expected_market: &str,
    expected_kind: Option<&str>,
) -> Result<(Vec<Value>, String, String), ResearchReadSnapshotError> {
    let object = payload
        .as_object()
        .ok_or_else(|| bad_gateway("industry members response must be an object"))?;
    let entries = object
        .get("entries")
        .and_then(Value::as_array)
        .ok_or_else(|| bad_gateway("industry members response is missing entries"))?;
    let projected = entries
        .iter()
        .map(project_ranking_entry)
        .collect::<Result<Vec<_>, _>>()?;
    let market = object
        .get("market")
        .and_then(Value::as_str)
        .filter(|v| !v.trim().is_empty())
        .ok_or_else(|| bad_gateway("industry members response is missing market"))?
        .to_ascii_uppercase();
    if market != expected_market {
        return Err(bad_gateway(
            "industry members response market does not match request",
        ));
    }
    let response_kind = object
        .get("kind")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|v| !v.is_empty())
        .ok_or_else(|| bad_gateway("industry members response is missing kind"))?;
    if expected_kind.is_some_and(|kind| !response_kind.eq_ignore_ascii_case(kind)) {
        return Err(bad_gateway(
            "industry members response kind does not match request",
        ));
    }
    let board = object
        .get("board")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|v| !v.is_empty())
        .ok_or_else(|| bad_gateway("industry members response is missing board"))?;
    if !board.eq_ignore_ascii_case(expected_board) {
        return Err(bad_gateway("industry members board does not match request"));
    }
    Ok((
        projected,
        object_source(payload, "market-data-industries"),
        market,
    ))
}

fn project_ranking_entry(value: &Value) -> Result<Value, ResearchReadSnapshotError> {
    let object = value
        .as_object()
        .ok_or_else(|| bad_gateway("ranking entry must be an object"))?;
    let instrument_id = object
        .get("instrument_id")
        .or_else(|| object.get("instrumentId"))
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|v| !v.is_empty())
        .ok_or_else(|| bad_gateway("ranking entry is missing instrument_id"))?
        .to_ascii_uppercase();
    let mut projected = Map::new();
    projected.insert("instrumentId".to_owned(), json!(instrument_id));
    if let Some((market, symbol)) = instrument_id.split_once('.') {
        projected.insert("market".to_owned(), json!(market));
        projected.insert("symbol".to_owned(), json!(symbol));
    }
    copy_field(object, &mut projected, "name", "name");
    for (source, target) in [
        ("price", "price"),
        ("change_rate", "changeRate"),
        ("change_amount", "changeAmount"),
        ("volume", "volume"),
        ("turnover", "turnover"),
        ("turnover_ratio", "turnoverRatio"),
        ("pe_ttm", "peTTM"),
        ("market_cap", "marketCap"),
    ] {
        copy_field(object, &mut projected, source, target);
    }
    Ok(Value::Object(projected))
}

fn copy_field(source: &Map<String, Value>, target: &mut Map<String, Value>, from: &str, to: &str) {
    if let Some(value) = source.get(from).filter(|value| !value.is_null()) {
        target.insert(to.to_owned(), value.clone());
    }
}

fn object_source(payload: &Value, fallback: &str) -> String {
    payload
        .get("source")
        .and_then(Value::as_str)
        .filter(|v| !v.trim().is_empty())
        .unwrap_or(fallback)
        .to_owned()
}

fn feature_result(
    provider: &str,
    feature_id: &str,
    entries: Vec<Value>,
    source: String,
    resolved_instrument: Option<Value>,
) -> Value {
    let as_of = crate::product::product_production_ports::provider_now_rfc3339();
    let total = entries.len();
    let mut result = json!({
        "provider": {
            "brokerId": provider,
            "featureId": feature_id,
            "capability": "available",
            "selectionReason": "embedded-market-data-provider",
            "resolvedAt": as_of,
            "asOf": as_of,
        },
        "asOf": as_of,
        "entries": entries,
        "hasMore": false,
        "total": total,
        "metadata": {"source": source},
    });
    if let Some(resolved) = resolved_instrument {
        result["resolvedInstrument"] = resolved;
    }
    result
}

fn resolved_plate_instrument(market: &str, board: &str) -> Value {
    let instrument_id = format!("{market}.{board}");
    json!({
        "instrumentId": instrument_id,
        "code": board,
        "productClass": "plate",
        "marketSegment": "securities",
        "quoteMarket": market,
        "tradeMarket": market,
        "quantityMode": "units",
    })
}

fn invalid(message: &str) -> ResearchReadSnapshotError {
    ResearchReadSnapshotError::Invalid(message.to_owned())
}

fn capability(feature: &str, operation: &str) -> ResearchReadSnapshotError {
    ResearchReadSnapshotError::Failed {
        status: 409,
        code: "CAPABILITY_UNAVAILABLE".to_owned(),
        message: format!(
            "embedded market-data provider does not serve {feature} operation/market {operation:?}"
        ),
        retry_after_seconds: None,
    }
}

fn bad_gateway(message: &str) -> ResearchReadSnapshotError {
    ResearchReadSnapshotError::Failed {
        status: 502,
        code: "BAD_GATEWAY".to_owned(),
        message: message.to_owned(),
        retry_after_seconds: None,
    }
}

fn map_helper_error(error: HttpAdapterError) -> ResearchReadSnapshotError {
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rankings_limit_prefers_positive_page_size() {
        let query = QueryMap::parse("pageSize=40&limit=5").expect("query");
        assert_eq!(request_limit(&query).expect("limit"), 40);
    }

    #[test]
    fn rankings_limit_falls_back_to_legacy_limit_and_default() {
        let zero = QueryMap::parse("pageSize=0&limit=5").expect("query");
        assert_eq!(request_limit(&zero).expect("legacy fallback"), 5);

        let malformed = QueryMap::parse("pageSize=bad&limit=-2").expect("query");
        assert_eq!(request_limit(&malformed).expect("default"), DEFAULT_LIMIT);

        let empty = QueryMap::parse("").expect("query");
        assert_eq!(request_limit(&empty).expect("default"), DEFAULT_LIMIT);
    }

    #[test]
    fn rankings_limit_clamps_to_provider_bounds() {
        let query = QueryMap::parse("pageSize=10000").expect("query");
        assert_eq!(request_limit(&query).expect("clamp"), MAX_LIMIT);
    }
}
