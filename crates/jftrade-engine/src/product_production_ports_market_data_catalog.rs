//! Production market-data catalog adapter for markets and instrument search.

#[cfg(test)]
#[path = "product_production_ports_market_data_catalog_tests.rs"]
mod tests;

use jftrade_integration_marketdata_helper::{
    HelperClient, HelperMarketsResponse, HelperSearchResponse,
};
use jftrade_settings::MarketDataProvider;
use serde_json::{Value, json};
use std::sync::Arc;

use crate::product::product_active_provider_state::ActiveProviderState;
use crate::product::{
    MarketDataCatalogReadFuture, MarketDataCatalogReadSnapshotError,
    MarketDataCatalogReadSnapshotPort,
};

#[derive(Clone)]
pub(crate) struct ProductionMarketDataCatalogPort {
    active_provider_state: Arc<ActiveProviderState>,
    helper: Option<HelperClient>,
}

impl std::fmt::Debug for ProductionMarketDataCatalogPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ProductionMarketDataCatalogPort")
            .field("has_helper", &self.helper.is_some())
            .finish()
    }
}

impl ProductionMarketDataCatalogPort {
    pub(crate) fn new(
        active_provider_state: Arc<ActiveProviderState>,
        helper: Option<HelperClient>,
    ) -> Self {
        Self {
            active_provider_state,
            helper,
        }
    }

    fn active_provider(&self) -> Result<MarketDataProvider, MarketDataCatalogReadSnapshotError> {
        self.active_provider_state.get().ok_or_else(|| {
            MarketDataCatalogReadSnapshotError::Unavailable(
                "active market-data provider is not configured".to_owned(),
            )
        })
    }
}

impl MarketDataCatalogReadSnapshotPort for ProductionMarketDataCatalogPort {
    fn read<'a>(&'a self, path: &'a str, query: &'a str) -> MarketDataCatalogReadFuture<'a> {
        Box::pin(async move {
            match path {
                "/api/v1/market-data/markets" => self.read_markets(query).await,
                "/api/v1/market-data/instruments" => self.read_instruments(query).await,
                _ => Err(MarketDataCatalogReadSnapshotError::Unavailable(format!(
                    "unsupported market-data catalog path: {path}"
                ))),
            }
        })
    }
}

impl ProductionMarketDataCatalogPort {
    async fn read_markets(
        &self,
        _query: &str,
    ) -> Result<Value, MarketDataCatalogReadSnapshotError> {
        let provider = self.active_provider()?;
        match provider {
            MarketDataProvider::Yfinance | MarketDataProvider::Akshare => {
                let Some(helper) = &self.helper else {
                    return Err(MarketDataCatalogReadSnapshotError::Unavailable(format!(
                        "market data helper is not configured for {provider:?}"
                    )));
                };
                let provider_str = match provider {
                    MarketDataProvider::Yfinance => "yfinance",
                    MarketDataProvider::Akshare => "akshare",
                    MarketDataProvider::Futu => "futu",
                };
                let markets_resp = helper
                    .get_provider_json::<HelperMarketsResponse>(provider_str, &["markets"])
                    .await
                    .map_err(|error| map_helper_catalog_error(error, "MARKET_DATA_FAILED"))?;
                let markets_array = markets_resp
                    .markets
                    .iter()
                    .map(|p| {
                        json!({
                            "code": p.code,
                            "market": p.code,
                            "resolvedMarket": p.resolved_market,
                            "preferredPrefix": p.preferred_prefix,
                            "name": p.display_name,
                            "displayName": p.display_name,
                            "quoteCurrency": p.quote_currency,
                            "timezone": p.timezone,
                            "supportsExtendedHours": p.supports_extended_hours,
                            "requiresExchangePrefix": p.requires_exchange_prefix,
                            "aliases": p.aliases,
                            "regularSessions": p.regular_sessions,
                            "precision": {
                                "price": p.precision.price,
                                "quote": p.precision.quote,
                            },
                            "tickSize": p.tick_size,
                        })
                    })
                    .collect::<Vec<_>>();
                let default_market = markets_resp
                    .default_market
                    .unwrap_or_else(|| "US".to_owned());
                Ok(json!({
                    "defaultMarket": default_market,
                    "markets": markets_array,
                }))
            }
            MarketDataProvider::Futu => Ok(json!({
                "defaultMarket": "HK",
                "markets": [
                    {
                        "code": "HK",
                        "market": "HK",
                        "resolvedMarket": "HK",
                        "preferredPrefix": "HK",
                        "name": "Hong Kong",
                        "displayName": "Hong Kong",
                        "quoteCurrency": "HKD",
                        "timezone": "Asia/Hong_Kong",
                        "supportsExtendedHours": false,
                        "requiresExchangePrefix": false,
                        "aliases": ["HKEX"],
                        "regularSessions": [
                            {"startMinute": 570, "endMinute": 720, "label": "09:30-12:00"},
                            {"startMinute": 780, "endMinute": 960, "label": "13:00-16:00"}
                        ],
                        "precision": {"price": 3, "quote": 3},
                        "tickSize": 0.001
                    },
                    {
                        "code": "US",
                        "market": "US",
                        "resolvedMarket": "US",
                        "preferredPrefix": "US",
                        "name": "United States",
                        "displayName": "United States",
                        "quoteCurrency": "USD",
                        "timezone": "America/New_York",
                        "supportsExtendedHours": true,
                        "requiresExchangePrefix": false,
                        "aliases": ["NYSE", "NASDAQ"],
                        "regularSessions": [
                            {"startMinute": 570, "endMinute": 960, "label": "09:30-16:00"}
                        ],
                        "precision": {"price": 2, "quote": 2},
                        "tickSize": 0.01
                    },
                    {
                        "code": "SH",
                        "market": "SH",
                        "resolvedMarket": "CN",
                        "preferredPrefix": "SH",
                        "name": "Shanghai",
                        "displayName": "Shanghai",
                        "quoteCurrency": "CNY",
                        "timezone": "Asia/Shanghai",
                        "supportsExtendedHours": false,
                        "requiresExchangePrefix": true,
                        "aliases": ["CNSH"],
                        "regularSessions": [
                            {"startMinute": 570, "endMinute": 690, "label": "09:30-11:30"},
                            {"startMinute": 780, "endMinute": 900, "label": "13:00-15:00"}
                        ],
                        "precision": {"price": 2, "quote": 2},
                        "tickSize": 0.01
                    },
                    {
                        "code": "SZ",
                        "market": "SZ",
                        "resolvedMarket": "CN",
                        "preferredPrefix": "SZ",
                        "name": "Shenzhen",
                        "displayName": "Shenzhen",
                        "quoteCurrency": "CNY",
                        "timezone": "Asia/Shanghai",
                        "supportsExtendedHours": false,
                        "requiresExchangePrefix": true,
                        "aliases": ["CNSZ"],
                        "regularSessions": [
                            {"startMinute": 570, "endMinute": 690, "label": "09:30-11:30"},
                            {"startMinute": 780, "endMinute": 900, "label": "13:00-15:00"}
                        ],
                        "precision": {"price": 2, "quote": 2},
                        "tickSize": 0.01
                    },
                ],
            })),
        }
    }

    async fn read_instruments(
        &self,
        query_str: &str,
    ) -> Result<Value, MarketDataCatalogReadSnapshotError> {
        let parsed_query =
            crate::product::product_query::QueryMap::parse(query_str).map_err(|_| {
                MarketDataCatalogReadSnapshotError::Invalid {
                    code: "MARKET_INSTRUMENT_INVALID".to_owned(),
                    message: "invalid URL escape".to_owned(),
                }
            })?;
        let search_query = parsed_query
            .get_first("query")
            .or_else(|| parsed_query.get_first("q"))
            .map(str::trim)
            .filter(|q| !q.is_empty())
            .ok_or_else(|| MarketDataCatalogReadSnapshotError::Invalid {
                code: "MARKET_INSTRUMENT_INVALID".to_owned(),
                message: "query is required".to_owned(),
            })?;

        let limit: usize = if let Some(limit_str) = parsed_query.get_first("limit") {
            let parsed = limit_str.trim().parse::<usize>().map_err(|_| {
                MarketDataCatalogReadSnapshotError::Invalid {
                    code: "MARKET_INSTRUMENT_INVALID".to_owned(),
                    message: "limit must be between 1 and 100".to_owned(),
                }
            })?;
            if !(1..=100).contains(&parsed) {
                return Err(MarketDataCatalogReadSnapshotError::Invalid {
                    code: "MARKET_INSTRUMENT_INVALID".to_owned(),
                    message: "limit must be between 1 and 100".to_owned(),
                });
            }
            parsed
        } else {
            20
        };

        let requested_market = parsed_query
            .get_first("market")
            .or_else(|| parsed_query.get_first("requestedMarket"))
            .unwrap_or_default();

        if !requested_market.is_empty() {
            let valid_markets = ["US", "HK", "CN", "SH", "SZ"];
            if !valid_markets.contains(&requested_market.to_ascii_uppercase().as_str()) {
                return Err(MarketDataCatalogReadSnapshotError::Invalid {
                    code: "MARKET_INSTRUMENT_INVALID".to_owned(),
                    message: format!("unsupported market: {requested_market}"),
                });
            }
        }

        let provider = self.active_provider()?;

        match provider {
            MarketDataProvider::Yfinance | MarketDataProvider::Akshare => {
                let Some(helper) = &self.helper else {
                    return Err(MarketDataCatalogReadSnapshotError::Unavailable(format!(
                        "market data helper is not configured for {provider:?}"
                    )));
                };
                let provider_str = match provider {
                    MarketDataProvider::Yfinance => "yfinance",
                    MarketDataProvider::Akshare => "akshare",
                    MarketDataProvider::Futu => "futu",
                };
                let limit_str = limit.to_string();
                let query_params = [("q", search_query), ("limit", limit_str.as_str())];
                let search_resp = helper
                    .get_provider_json_with_query::<HelperSearchResponse>(
                        provider_str,
                        &["search"],
                        &query_params,
                    )
                    .await
                    .map_err(|error| {
                        map_helper_catalog_error(error, "MARKET_INSTRUMENT_SEARCH_FAILED")
                    })?;

                let mut entries = if !requested_market.is_empty() {
                    let req_upper = requested_market.to_ascii_uppercase();
                    if req_upper == "CN" {
                        search_resp
                            .entries
                            .into_iter()
                            .filter(|e| {
                                let m = e.market.to_ascii_uppercase();
                                m == "CN" || m == "SH" || m == "SZ"
                            })
                            .collect::<Vec<_>>()
                    } else {
                        search_resp
                            .entries
                            .into_iter()
                            .filter(|e| e.market.to_ascii_uppercase() == req_upper)
                            .collect::<Vec<_>>()
                    }
                } else {
                    search_resp.entries
                };

                let norm_q = search_query.trim().to_ascii_uppercase();
                let exact: Vec<_> = entries
                    .iter()
                    .filter(|e| {
                        e.symbol.trim().eq_ignore_ascii_case(&norm_q)
                            || e.code.trim().eq_ignore_ascii_case(&norm_q)
                            || e.instrument_id
                                .split('.')
                                .next_back()
                                .is_some_and(|s| s.trim().eq_ignore_ascii_case(&norm_q))
                    })
                    .cloned()
                    .collect();
                if !exact.is_empty() {
                    entries = exact;
                }

                entries.sort_by(|a, b| {
                    (
                        a.market.as_str(),
                        a.symbol.as_str(),
                        a.instrument_id.as_str(),
                    )
                        .cmp(&(
                            b.market.as_str(),
                            b.symbol.as_str(),
                            b.instrument_id.as_str(),
                        ))
                });

                let total_all_matches = entries.len();
                let resolution_status =
                    if total_all_matches > 0 && !entries.iter().any(|e| e.selectable) {
                        "unavailable"
                    } else {
                        match total_all_matches {
                            0 => "not_found",
                            1 => "resolved",
                            _ => "ambiguous",
                        }
                    };

                if entries.len() > limit {
                    entries.truncate(limit);
                }
                let total_returned = entries.len();

                Ok(json!({
                    "entries": entries,
                    "failures": [],
                    "query": search_query,
                    "requestedMarket": requested_market,
                    "resolutionStatus": resolution_status,
                    "totalReturned": total_returned,
                }))
            }
            MarketDataProvider::Futu => {
                let norm_q = search_query.trim().to_ascii_uppercase();
                let (market_part, symbol_part) = if let Some((m, s)) = norm_q.split_once('.') {
                    (m.trim(), s.trim())
                } else if let Some((m, s)) = norm_q.split_once(':') {
                    (m.trim(), s.trim())
                } else if !requested_market.is_empty() {
                    (requested_market.trim(), norm_q.as_str())
                } else {
                    ("", norm_q.as_str())
                };

                let market_upper = market_part.to_ascii_uppercase();
                let is_valid_market = matches!(
                    market_upper.as_str(),
                    "HK" | "US" | "SH" | "SZ" | "CN" | "SG" | "JP" | "AU" | "MY" | "CA"
                );

                let mut entries = Vec::new();
                if is_valid_market && !symbol_part.is_empty() {
                    let canonical_market = if market_upper == "CNSH" {
                        "SH".to_owned()
                    } else if market_upper == "CNSZ" {
                        "SZ".to_owned()
                    } else {
                        market_upper.clone()
                    };
                    let resolved_market = if canonical_market == "SH" || canonical_market == "SZ" {
                        "CN".to_owned()
                    } else {
                        canonical_market.clone()
                    };
                    let selectable = matches!(
                        canonical_market.as_str(),
                        "HK" | "US" | "SH" | "SZ" | "CN"
                    );
                    let instrument_id = format!("{canonical_market}.{symbol_part}");
                    let entry = json!({
                        "instrumentId": instrument_id,
                        "market": canonical_market,
                        "resolvedMarket": resolved_market,
                        "symbol": symbol_part,
                        "code": symbol_part,
                        "name": "",
                        "securityType": "stock",
                        "source": "futu",
                        "selectable": selectable,
                    });
                    entries.push(entry);
                }

                let resolution_status = if entries.is_empty() {
                    "not_found"
                } else {
                    "resolved"
                };
                let total_returned = entries.len();

                Ok(json!({
                    "entries": entries,
                    "failures": [],
                    "query": search_query,
                    "requestedMarket": requested_market,
                    "resolutionStatus": resolution_status,
                    "totalReturned": total_returned,
                }))
            }
        }
    }
}

fn map_helper_catalog_error(
    error: jftrade_integration_marketdata_helper::HttpAdapterError,
    default_code: &str,
) -> MarketDataCatalogReadSnapshotError {
    match error {
        jftrade_integration_marketdata_helper::HttpAdapterError::Remote {
            status,
            code,
            message,
            ..
        } => {
            let error_code = if !code.is_empty() {
                code
            } else {
                default_code.to_owned()
            };
            MarketDataCatalogReadSnapshotError::Failed {
                status,
                code: error_code,
                message,
            }
        }
        jftrade_integration_marketdata_helper::HttpAdapterError::Timeout => {
            MarketDataCatalogReadSnapshotError::Failed {
                status: 504,
                code: "GATEWAY_TIMEOUT".to_owned(),
                message: "market-data helper request timed out".to_owned(),
            }
        }
        jftrade_integration_marketdata_helper::HttpAdapterError::Unavailable(msg) => {
            MarketDataCatalogReadSnapshotError::Unavailable(msg)
        }
        jftrade_integration_marketdata_helper::HttpAdapterError::InvalidResponse(msg) => {
            MarketDataCatalogReadSnapshotError::Failed {
                status: 502,
                code: "BAD_GATEWAY".to_owned(),
                message: msg,
            }
        }
        other => MarketDataCatalogReadSnapshotError::Failed {
            status: 500,
            code: default_code.to_owned(),
            message: other.to_string(),
        },
    }
}
