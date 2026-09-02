//! Production projection for the Futu future-contract catalogue.
//!
//! The module is intentionally independent from the derivative port wiring:
//! callers provide an engine-neutral [`FutureInfoReadPort`] and this adapter
//! handles URL query parsing and the normalized response envelope. The
//! underlying `Qot_GetFutureInfo` protocol has no pagination fields, so
//! page-size/cursor query values are accepted for Go compatibility but do not
//! alter the returned catalogue. Layout/runtime registration can then be
//! composed separately.

use std::{collections::HashSet, sync::Arc};

use serde_json::{Value, json};

use super::super::product_production_ports_trade::SharedTradeReadRuntime;
use crate::product::MarketDataDerivativeReadSnapshotError;
use crate::product::product_query::QueryMap;

/// Read the `/api/v1/market-data/futures` catalogue through a typed reader.
pub(crate) fn read(
    reader: Option<&dyn jftrade_integration_futu::FutureInfoReadPort>,
    path: &str,
    query: &str,
) -> Result<Value, MarketDataDerivativeReadSnapshotError> {
    if path != "/api/v1/market-data/futures" {
        return Err(bad_request("unsupported futures route"));
    }
    let reader = reader.ok_or_else(|| {
        MarketDataDerivativeReadSnapshotError::Unavailable(
            "Futu futures reader is not ready".to_owned(),
        )
    })?;
    let request = parse_request(query)?;
    let mut items = reader.query(&request).map_err(map_reader_error)?;
    project_items(request, &mut items)
}

/// Runtime-backed variant used by the production composition root.
pub(crate) fn read_runtime(
    runtime: Option<&Arc<SharedTradeReadRuntime>>,
    path: &str,
    query: &str,
) -> Result<Value, MarketDataDerivativeReadSnapshotError> {
    if path != "/api/v1/market-data/futures" {
        return Err(bad_request("unsupported futures route"));
    }
    let runtime = runtime.ok_or_else(|| {
        MarketDataDerivativeReadSnapshotError::Unavailable(
            "Futu futures runtime is not configured".to_owned(),
        )
    })?;
    if !runtime.future_info_available() {
        return Err(MarketDataDerivativeReadSnapshotError::Unavailable(
            "Futu futures reader is not ready".to_owned(),
        ));
    }
    let request = parse_request(query)?;
    let mut items = runtime.future_info(&request).map_err(map_reader_error)?;
    project_items(request, &mut items)
}

fn project_items(
    request: jftrade_integration_futu::FutureInfoQuery,
    items: &mut Vec<jftrade_integration_futu::FutureInfo>,
) -> Result<Value, MarketDataDerivativeReadSnapshotError> {
    if let Some(market) = request.market {
        items.retain(|item| {
            item.security
                .market
                .eq_ignore_ascii_case(market_label(market))
        });
    }
    // Qot_GetFutureInfo has no page or cursor fields.  Keep the complete
    // OpenD response in provider order: Go's advanced adapter forwards
    // pageSize/cursor query values but does not apply local pagination for
    // this protocol.
    let total = items.len();
    let entries = std::mem::take(items)
        .into_iter()
        .map(|item| {
            serde_json::to_value(item).map_err(|error| {
                MarketDataDerivativeReadSnapshotError::Failed {
                    status: 502,
                    code: "BAD_GATEWAY".to_owned(),
                    message: format!("failed to serialize OpenD future info: {error}"),
                }
            })
        })
        .collect::<Result<Vec<_>, _>>()?;
    let as_of = super::super::provider_now_rfc3339();
    let mut metadata = serde_json::Map::new();
    if let Some(market) = request.market {
        metadata.insert("market".to_owned(), json!(market_label(market)));
    }
    metadata.insert("requestedCount".to_owned(), json!(request.securities.len()));
    let result = json!({
        "provider": {
            "brokerId": "futu",
            "securityFirm": "Futu/Moomoo via OpenD",
            "featureId": "derivatives.futures",
            "capability": "available",
            "selectionReason": "adapter_request",
            "resolvedAt": as_of,
            "asOf": as_of,
        },
        "asOf": as_of,
        "entries": entries,
        "hasMore": false,
        "total": total,
        "metadata": Value::Object(metadata),
    });
    Ok(result)
}

/// Keep this helper available for a future `MarketDataDerivativeReadSnapshotPort`
/// implementation without coupling this file to runtime state. It is useful in
/// focused tests and lets composition roots delegate the same projection.
#[allow(dead_code)]
pub(crate) fn read_port(
    reader: Option<&dyn jftrade_integration_futu::FutureInfoReadPort>,
    path: &str,
    query: &str,
) -> Result<Value, MarketDataDerivativeReadSnapshotError> {
    read(reader, path, query)
}

fn parse_request(
    query: &str,
) -> Result<jftrade_integration_futu::FutureInfoQuery, MarketDataDerivativeReadSnapshotError> {
    let query_map = QueryMap::parse(query).map_err(|_| bad_request("invalid URL escape"))?;
    if let Some(operation) = query_map.get_first("operation") {
        let operation = operation.trim().to_ascii_lowercase();
        if !operation.is_empty() && operation != "contracts" {
            return Err(unsupported_operation(&operation));
        }
    }
    let market = query_map
        .get_first("market")
        .map(parse_market)
        .transpose()?;
    let mut securities = Vec::new();
    let mut seen = HashSet::new();
    for key in ["instrumentId", "symbol", "symbols", "code"] {
        if let Some(values) = query_map.get_all(key) {
            for raw in values {
                if raw.trim().is_empty() {
                    return Err(bad_request(&format!("{key} must not be empty")));
                }
                for token in raw.split(',').map(str::trim) {
                    if token.is_empty() {
                        return Err(bad_request(&format!("{key} contains an empty value")));
                    }
                    let security = parse_security(token, market, key)?;
                    if seen.insert((security.market, security.code.clone())) {
                        securities.push(security);
                    }
                }
            }
        }
    }
    let request = jftrade_integration_futu::FutureInfoQuery { market, securities };
    request
        .validate()
        .map_err(|error| bad_request(&error.to_string()))?;
    Ok(request)
}

fn parse_security(
    value: &str,
    requested_market: Option<i32>,
    key: &str,
) -> Result<jftrade_integration_futu::FutureInfoSecurityQuery, MarketDataDerivativeReadSnapshotError>
{
    let (market, code) = match value.split_once('.') {
        Some((market, code)) if !market.trim().is_empty() && !code.trim().is_empty() => {
            let market = parse_market(market)?;
            if requested_market.is_some_and(|requested| requested != market) {
                return Err(bad_request("instrument market does not match market"));
            }
            (market, code.trim())
        }
        _ if requested_market.is_some() && !value.contains('.') => {
            (requested_market.unwrap(), value)
        }
        _ => {
            return Err(bad_request(&format!("{key} must be MARKET.CODE")));
        }
    };
    if code.is_empty()
        || code.len() > 128
        || code.chars().any(|character| {
            character.is_whitespace()
                || character.is_control()
                || matches!(character, '.' | '/' | '\\' | '?' | '#')
        })
    {
        return Err(bad_request("future security code is invalid"));
    }
    Ok(jftrade_integration_futu::FutureInfoSecurityQuery {
        market,
        code: code.to_ascii_uppercase(),
    })
}

fn parse_market(value: &str) -> Result<i32, MarketDataDerivativeReadSnapshotError> {
    match value.trim().to_ascii_uppercase().as_str() {
        "HK" => Ok(1),
        "US" => Ok(11),
        _ => Err(bad_request("future market is unsupported")),
    }
}

fn market_label(market: i32) -> &'static str {
    match market {
        1 | 2 => "HK",
        11 => "US",
        _ => "UNKNOWN",
    }
}

fn map_reader_error(
    error: jftrade_integration_futu::FutureInfoQueryError,
) -> MarketDataDerivativeReadSnapshotError {
    match error {
        jftrade_integration_futu::FutureInfoQueryError::InvalidQuery(message) => {
            bad_request(&message)
        }
        jftrade_integration_futu::FutureInfoQueryError::Session(error) => {
            MarketDataDerivativeReadSnapshotError::Unavailable(error.to_string())
        }
        other => MarketDataDerivativeReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: other.to_string(),
        },
    }
}

fn bad_request(message: &str) -> MarketDataDerivativeReadSnapshotError {
    MarketDataDerivativeReadSnapshotError::Failed {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: message.to_owned(),
    }
}

fn unsupported_operation(operation: &str) -> MarketDataDerivativeReadSnapshotError {
    // Go's Qot_GetFutureInfo adapter rejects unknown operations after protocol
    // dispatch; the route maps that adapter error to BROKER_FEATURE_FAILED
    // (502), rather than treating it as a transport-level malformed query.
    MarketDataDerivativeReadSnapshotError::Failed {
        status: 502,
        code: "BROKER_FEATURE_FAILED".to_owned(),
        message: format!("futu: unsupported derivatives.futures operation \"{operation}\""),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use jftrade_integration_futu::{FutureInfo, FutureInfoSecurity, FutureTradeTime};

    #[derive(Debug)]
    struct FixtureReader {
        values: Vec<FutureInfo>,
    }

    impl jftrade_integration_futu::FutureInfoReadPort for FixtureReader {
        fn query(
            &self,
            query: &jftrade_integration_futu::FutureInfoQuery,
        ) -> Result<Vec<FutureInfo>, jftrade_integration_futu::FutureInfoQueryError> {
            assert_eq!(query.market, Some(11));
            assert_eq!(query.securities.len(), 1);
            assert_eq!(query.securities[0].code, "ESMAIN");
            Ok(self.values.clone())
        }
    }

    fn sample(code: &str, market: &str) -> FutureInfo {
        FutureInfo {
            name: "E-mini S&P".to_owned(),
            security: FutureInfoSecurity {
                market: market.to_owned(),
                code: code.to_owned(),
                quote_market: market.to_owned(),
                trade_market: market.to_owned(),
                instrument_id: format!("{market}.{code}"),
            },
            last_trade_time: Some("2026-12-18".to_owned()),
            last_trade_timestamp: None,
            owner: None,
            owner_other: "S&P 500".to_owned(),
            exchange: "CME".to_owned(),
            contract_type: "Main".to_owned(),
            contract_size: 50.0,
            contract_size_unit: "USD".to_owned(),
            quote_currency: "USD".to_owned(),
            min_var: 0.25,
            min_var_unit: "point".to_owned(),
            quote_unit: Some("point".to_owned()),
            trade_time: vec![FutureTradeTime {
                begin: Some(60.0),
                end: Some(1_380.0),
            }],
            time_zone: "America/Chicago".to_owned(),
            exchange_format_url: "https://www.cmegroup.com".to_owned(),
            origin: None,
        }
    }

    #[test]
    fn projects_typed_futures_without_applying_unsupported_page_size() {
        let reader = FixtureReader {
            values: vec![sample("ESMAIN", "US"), sample("NQMAIN", "US")],
        };
        let value = read(
            Some(&reader),
            "/api/v1/market-data/futures",
            "market=US&instrumentId=US.ESmain&pageSize=1",
        )
        .expect("futures projection");
        assert_eq!(value["provider"]["featureId"], "derivatives.futures");
        assert_eq!(value["entries"].as_array().map(Vec::len), Some(2));
        assert_eq!(value["entries"][0]["security"]["instrumentId"], "US.ESMAIN");
        assert_eq!(value["hasMore"], false);
        assert_eq!(value["total"], 2);
        assert!(value.get("nextCursor").is_none());
    }

    #[test]
    fn rejects_bad_route_market_and_instrument_encoding_before_reader() {
        let reader = FixtureReader { values: Vec::new() };
        assert!(matches!(
            read(Some(&reader), "/api/v1/market-data/warrants", ""),
            Err(MarketDataDerivativeReadSnapshotError::Failed { status: 400, .. })
        ));
        assert!(matches!(
            read(Some(&reader), "/api/v1/market-data/futures", "market=BAD"),
            Err(MarketDataDerivativeReadSnapshotError::Failed { status: 400, .. })
        ));
        assert!(matches!(
            read(
                Some(&reader),
                "/api/v1/market-data/futures",
                "market=US&instrumentId=HK.ES"
            ),
            Err(MarketDataDerivativeReadSnapshotError::Failed { status: 400, .. })
        ));
    }

    #[test]
    fn reports_reader_unavailable_without_invoking_query() {
        let error =
            read(None, "/api/v1/market-data/futures", "market=US").expect_err("missing reader");
        assert!(matches!(
            error,
            MarketDataDerivativeReadSnapshotError::Unavailable(message)
                if message == "Futu futures reader is not ready"
        ));
    }

    #[test]
    fn forwards_opaque_cursor_without_local_pagination() {
        let reader = FixtureReader {
            values: vec![sample("NQMAIN", "US"), sample("ESMAIN", "US")],
        };
        let value = read(
            Some(&reader),
            "/api/v1/market-data/futures",
            "market=US&instrumentId=US.ESmain&pageSize=1",
        )
        .expect("catalogue response");
        assert_eq!(value["entries"].as_array().map(Vec::len), Some(2));
        assert_eq!(value["entries"][0]["security"]["code"], "NQMAIN");
        assert_eq!(value["entries"][1]["security"]["code"], "ESMAIN");
        assert_eq!(value["hasMore"], false);
        assert!(value.get("nextCursor").is_none());

        let value = read(
            Some(&reader),
            "/api/v1/market-data/futures",
            "market=US&instrumentId=US.ESmain&pageSize=garbage&cursor=opaque",
        )
        .expect("opaque pagination values are ignored");
        assert_eq!(value["entries"].as_array().map(Vec::len), Some(2));
        assert_eq!(value["total"], 2);
    }

    #[test]
    fn rejects_unsupported_operation_empty_symbol_and_non_futu_market() {
        let reader = FixtureReader { values: Vec::new() };
        for query in ["market=US&instrumentId=", "market=SH"] {
            assert!(
                matches!(
                    read(Some(&reader), "/api/v1/market-data/futures", query),
                    Err(MarketDataDerivativeReadSnapshotError::Failed { status: 400, .. }),
                ),
                "query {query}"
            );
        }
        let error = read(
            Some(&reader),
            "/api/v1/market-data/futures",
            "market=US&operation=list",
        )
        .expect_err("unknown operation must fail");
        assert!(matches!(
            error,
            MarketDataDerivativeReadSnapshotError::Failed {
                status: 502,
                ref code,
                ref message,
            } if code == "BROKER_FEATURE_FAILED"
                && message.contains("unsupported derivatives.futures operation")
        ));
    }

    #[test]
    fn maps_closed_opend_session_to_unavailable() {
        let error = map_reader_error(jftrade_integration_futu::FutureInfoQueryError::Session(
            jftrade_integration_futu::OpenDSessionCoordinatorError::Closed,
        ));
        assert!(matches!(
            error,
            MarketDataDerivativeReadSnapshotError::Unavailable(message)
                if message.contains("session coordinator is closed")
        ));
    }
}
