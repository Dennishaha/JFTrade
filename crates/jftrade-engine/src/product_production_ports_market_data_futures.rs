//! Production projection for the Futu future-contract catalogue.
//!
//! The module is intentionally independent from the derivative port wiring:
//! callers provide an engine-neutral [`FutureInfoReadPort`] and this adapter
//! handles URL query parsing, bounded pagination, and the normalized response
//! envelope. Layout/runtime registration can then be composed separately.

use serde_json::{Value, json};

use crate::product::product_query::QueryMap;
use crate::product::MarketDataDerivativeReadSnapshotError;

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
    let (request, page_size) = parse_request(query)?;
    let mut items = reader.query(&request).map_err(map_reader_error)?;
    if let Some(market) = request.market {
        items.retain(|item| item.security.market.eq_ignore_ascii_case(market_label(market)));
    }
    let total = items.len();
    let has_more = page_size.is_some_and(|limit| total > limit);
    if let Some(limit) = page_size {
        items.truncate(limit);
    }
    let entries = items
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
    Ok(json!({
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
        "hasMore": has_more,
        "total": total,
        "metadata": Value::Object(metadata),
    }))
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
) -> Result<
    (jftrade_integration_futu::FutureInfoQuery, Option<usize>),
    MarketDataDerivativeReadSnapshotError,
> {
    let query_map = QueryMap::parse(query).map_err(|_| bad_request("invalid URL escape"))?;
    let market = query_map
        .get_first("market")
        .map(parse_market)
        .transpose()?;
    let page_size = query_map
        .get_first("pageSize")
        .or_else(|| query_map.get_first("limit"))
        .map(|value| {
            value
                .trim()
                .parse::<usize>()
                .map_err(|_| bad_request("pageSize must be between 1 and 1000"))
                .and_then(|value| {
                    if (1..=1000).contains(&value) {
                        Ok(value)
                    } else {
                        Err(bad_request("pageSize must be between 1 and 1000"))
                    }
                })
        })
        .transpose()?;

    let mut securities = Vec::new();
    for key in ["instrumentId", "symbol", "symbols", "code"] {
        for raw in query_map.get_all(key).into_iter().flatten() {
            for token in raw.split(',').map(str::trim).filter(|value| !value.is_empty()) {
                securities.push(parse_security(token, market, key)?);
            }
        }
    }
    let request = jftrade_integration_futu::FutureInfoQuery { market, securities };
    request
        .validate()
        .map_err(|error| bad_request(&error.to_string()))?;
    Ok((request, page_size))
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
        _ if requested_market.is_some() && !value.contains('.') => (requested_market.unwrap(), value),
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
        "SH" => Ok(21),
        "SZ" => Ok(22),
        "SG" => Ok(31),
        "JP" => Ok(41),
        "AU" => Ok(51),
        "MY" => Ok(61),
        "CA" => Ok(71),
        "FX" => Ok(81),
        "CRYPTO" | "CC" => Ok(91),
        _ => Err(bad_request("future market is unsupported")),
    }
}

fn market_label(market: i32) -> &'static str {
    match market {
        1 => "HK",
        11 => "US",
        21 => "SH",
        22 => "SZ",
        31 => "SG",
        41 => "JP",
        51 => "AU",
        61 => "MY",
        71 => "CA",
        81 => "FX",
        91 => "CRYPTO",
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
            last_trade_time: "2026-12-18".to_owned(),
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
            trade_time: vec![FutureTradeTime { begin: Some(60.0), end: Some(1_380.0) }],
            time_zone: "America/Chicago".to_owned(),
            exchange_format_url: "https://www.cmegroup.com".to_owned(),
            origin: None,
        }
    }

    #[test]
    fn projects_typed_futures_and_applies_market_page_size() {
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
        assert_eq!(value["entries"].as_array().map(Vec::len), Some(1));
        assert_eq!(value["entries"][0]["security"]["instrumentId"], "US.ESMAIN");
        assert_eq!(value["hasMore"], true);
        assert_eq!(value["total"], 2);
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
            read(Some(&reader), "/api/v1/market-data/futures", "market=US&instrumentId=HK.ES"),
            Err(MarketDataDerivativeReadSnapshotError::Failed { status: 400, .. })
        ));
    }

    #[test]
    fn reports_reader_unavailable_without_invoking_query() {
        let error = read(None, "/api/v1/market-data/futures", "market=US")
            .expect_err("missing reader");
        assert!(matches!(
            error,
            MarketDataDerivativeReadSnapshotError::Unavailable(message)
                if message == "Futu futures reader is not ready"
        ));
    }
}
