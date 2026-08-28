//! Production market-data provider actions adapter.

use std::sync::Arc;
use serde::Deserialize;
use serde_json::{Value, json};

use crate::product::product_market_data_provider_actions_port::{
    BATCH_SNAPSHOTS_PATH, MarketDataProviderActionsFuture, MarketDataProviderActionsPort,
    MarketDataProviderActionsPortError, MarketDataProviderActionsRequest,
    NORMALIZE_INSTRUMENT_PATH, OPTION_ANALYSIS_PATH, PREDICTION_COMBO_QUOTES_PATH,
    ZERO_DTE_CONTRACTS_PATH, is_market_data_provider_action_path,
};
use crate::product::{MarketDataQuoteReadSnapshotError, MarketDataQuoteReadSnapshotPort};

#[derive(Clone, Default)]
pub(crate) struct ProductionMarketDataProviderActionsPort {
    quote_port: Option<Arc<dyn MarketDataQuoteReadSnapshotPort>>,
}

impl std::fmt::Debug for ProductionMarketDataProviderActionsPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ProductionMarketDataProviderActionsPort")
            .field("has_quote_port", &self.quote_port.is_some())
            .finish()
    }
}

impl ProductionMarketDataProviderActionsPort {
    pub(crate) fn new(quote_port: Option<Arc<dyn MarketDataQuoteReadSnapshotPort>>) -> Self {
        Self { quote_port }
    }
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct NormalizeRequestBody {
    market: Option<String>,
    symbol: Option<String>,
    code: Option<String>,
    instrument_id: Option<String>,
    instrument: Option<String>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct BatchSnapshotsRequestBody {
    instrument_ids: Option<Vec<String>>,
    symbols: Option<Vec<String>>,
    instruments: Option<Vec<String>>,
}

impl MarketDataProviderActionsPort for ProductionMarketDataProviderActionsPort {
    fn dispatch<'a>(
        &'a self,
        request: &'a MarketDataProviderActionsRequest,
    ) -> MarketDataProviderActionsFuture<'a> {
        Box::pin(async move {
            let path = request.path.as_str();

            if path == NORMALIZE_INSTRUMENT_PATH {
                return self.normalize_instrument(request);
            }
            if path == BATCH_SNAPSHOTS_PATH {
                return self.batch_snapshots(request).await;
            }
            if path == ZERO_DTE_CONTRACTS_PATH
                || path == OPTION_ANALYSIS_PATH
                || path.starts_with("/api/v1/market-data/options/analysis/")
            {
                return Err(MarketDataProviderActionsPortError::Unavailable(
                    "options analysis provider is not configured".to_owned(),
                ));
            }
            if path == PREDICTION_COMBO_QUOTES_PATH {
                return Err(MarketDataProviderActionsPortError::Unavailable(
                    "prediction combo quotes provider is not configured".to_owned(),
                ));
            }

            if !is_market_data_provider_action_path(path) {
                return Err(MarketDataProviderActionsPortError::Unavailable(format!(
                    "unsupported provider action path: {path}"
                )));
            }

            Err(MarketDataProviderActionsPortError::Unavailable(
                "market-data provider action is not configured".to_owned(),
            ))
        })
    }
}

impl ProductionMarketDataProviderActionsPort {
    fn normalize_instrument(
        &self,
        request: &MarketDataProviderActionsRequest,
    ) -> Result<Value, MarketDataProviderActionsPortError> {
        let body: NormalizeRequestBody = serde_json::from_slice(&request.body).map_err(|_| {
            MarketDataProviderActionsPortError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "invalid normalize request".to_owned(),
                retry_after_seconds: None,
            }
        })?;

        let candidate = body
            .instrument_id
            .clone()
            .or_else(|| body.instrument.clone())
            .or_else(|| body.code.clone())
            .or_else(|| body.symbol.clone());

        let (market, symbol) = if let Some(candidate) = candidate {
            let trimmed = candidate.trim();
            if trimmed.contains('.') {
                let mut parts = trimmed.splitn(2, '.');
                let m = parts.next().unwrap_or("US").to_ascii_uppercase();
                let s = parts.next().unwrap_or("").to_ascii_uppercase();
                (m, s)
            } else {
                let m = body
                    .market
                    .as_deref()
                    .map(str::trim)
                    .filter(|m| !m.is_empty())
                    .unwrap_or("US")
                    .to_ascii_uppercase();
                (m, trimmed.to_ascii_uppercase())
            }
        } else if let (Some(m), Some(s)) = (body.market.as_deref(), body.symbol.as_deref()) {
            (m.trim().to_ascii_uppercase(), s.trim().to_ascii_uppercase())
        } else {
            return Err(MarketDataProviderActionsPortError::Failed {
                status: 400,
                code: "MARKET_INSTRUMENT_INVALID".to_owned(),
                message: "symbol or code is required".to_owned(),
                retry_after_seconds: None,
            });
        };

        if symbol.is_empty() {
            return Err(MarketDataProviderActionsPortError::Failed {
                status: 400,
                code: "MARKET_INSTRUMENT_INVALID".to_owned(),
                message: "symbol or code is required".to_owned(),
                retry_after_seconds: None,
            });
        }

        let instrument_id = format!("{market}.{symbol}");
        Ok(json!({
            "code": symbol,
            "instrumentId": instrument_id,
            "market": market,
            "prefix": market,
            "resolvedMarket": market,
            "symbol": format!("{market}.{symbol}"),
        }))
    }

    async fn batch_snapshots(
        &self,
        request: &MarketDataProviderActionsRequest,
    ) -> Result<Value, MarketDataProviderActionsPortError> {
        let body: BatchSnapshotsRequestBody =
            serde_json::from_slice(&request.body).map_err(|_| {
                MarketDataProviderActionsPortError::Failed {
                    status: 400,
                    code: "BAD_REQUEST".to_owned(),
                    message: "invalid request body".to_owned(),
                    retry_after_seconds: None,
                }
            })?;

        let quote_port = self.quote_port.as_ref().ok_or_else(|| {
            MarketDataProviderActionsPortError::Unavailable(
                "market-data quote provider is not configured".to_owned(),
            )
        })?;

        let mut raw_items = Vec::new();
        if let Some(list) = body.instrument_ids {
            raw_items.extend(list);
        }
        if let Some(list) = body.symbols {
            raw_items.extend(list);
        }
        if let Some(list) = body.instruments {
            raw_items.extend(list);
        }

        let mut requested_symbols = Vec::new();
        for item in raw_items {
            let trimmed = item.trim();
            if trimmed.is_empty() {
                continue;
            }
            let formatted = if trimmed.contains('.') {
                let mut parts = trimmed.splitn(2, '.');
                let m = parts.next().unwrap_or("US").to_ascii_uppercase();
                let s = parts.next().unwrap_or("").to_ascii_uppercase();
                format!("{m}.{s}")
            } else {
                format!("US.{}", trimmed.to_ascii_uppercase())
            };
            if !requested_symbols.contains(&formatted) {
                requested_symbols.push(formatted);
            }
        }

        if requested_symbols.is_empty() {
            return Err(MarketDataProviderActionsPortError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "invalid product feature query: at least one instrumentId is required".to_owned(),
                retry_after_seconds: None,
            });
        }

        let mut entries = Vec::new();
        let mut snapshots_map = serde_json::Map::new();

        for symbol_str in &requested_symbols {
            let (market, symbol) = if symbol_str.contains('.') {
                let mut parts = symbol_str.splitn(2, '.');
                let m = parts.next().unwrap_or("US");
                let s = parts.next().unwrap_or(symbol_str);
                (m, s)
            } else {
                ("US", symbol_str.as_str())
            };

            let snapshot_path = format!("/api/v1/market-data/snapshots/{market}/{symbol}");
            let snap_res = quote_port
                .read(&snapshot_path, "")
                .await
                .map_err(|error| match error {
                    MarketDataQuoteReadSnapshotError::Unavailable(msg) => {
                        MarketDataProviderActionsPortError::Unavailable(msg)
                    }
                    MarketDataQuoteReadSnapshotError::Failed {
                        status,
                        code,
                        message,
                        retry_after_seconds,
                    } => MarketDataProviderActionsPortError::Failed {
                        status,
                        code,
                        message,
                        retry_after_seconds,
                    },
                })?;

            let snap_obj = snap_res.get("snapshot").ok_or_else(|| {
                MarketDataProviderActionsPortError::Failed {
                    status: 502,
                    code: "BROKER_FEATURE_FAILED".to_owned(),
                    message: format!("snapshot payload missing for {symbol_str}"),
                    retry_after_seconds: None,
                }
            })?;

            let price = snap_obj
                .get("price")
                .and_then(|p| p.as_str())
                .and_then(|p| p.parse::<f64>().ok())
                .or_else(|| snap_obj.get("price").and_then(Value::as_f64))
                .ok_or_else(|| MarketDataProviderActionsPortError::Failed {
                    status: 502,
                    code: "BROKER_FEATURE_FAILED".to_owned(),
                    message: format!("snapshot price missing for {symbol_str}"),
                    retry_after_seconds: None,
                })?;

            let observed_at = snap_obj
                .get("observedAt")
                .and_then(Value::as_str)
                .filter(|s| !s.is_empty())
                .ok_or_else(|| MarketDataProviderActionsPortError::Failed {
                    status: 502,
                    code: "BROKER_FEATURE_FAILED".to_owned(),
                    message: format!("snapshot observedAt missing for {symbol_str}"),
                    retry_after_seconds: None,
                })?
                .to_owned();

            entries.push(json!({
                "lastPrice": price,
                "observedAt": observed_at,
                "symbol": symbol_str,
            }));

            snapshots_map.insert(
                symbol_str.clone(),
                json!({
                    "instrumentId": symbol_str,
                    "price": price,
                    "observedAt": observed_at,
                }),
            );
        }

        let as_of = current_utc_rfc3339();
        let query_map = parse_query(&request.query);
        let broker_id = query_map
            .get("brokerId")
            .map(String::as_str)
            .unwrap_or("active");
        let selection_reason = if query_map.contains_key("brokerId") {
            "explicit_broker"
        } else {
            "active_provider"
        };

        Ok(json!({
            "asOf": as_of,
            "entries": entries,
            "metadata": {
                "requestedSymbols": requested_symbols,
                "subscriptionCreated": false,
            },
            "provider": {
                "asOf": as_of,
                "brokerId": broker_id,
                "capability": "available",
                "featureId": "market.snapshots",
                "resolvedAt": as_of,
                "selectionReason": selection_reason,
            },
            "snapshots": snapshots_map,
        }))
    }
}

fn parse_query(query_str: &str) -> std::collections::BTreeMap<String, String> {
    let mut map = std::collections::BTreeMap::new();
    for pair in query_str.split('&') {
        if pair.is_empty() {
            continue;
        }
        let mut parts = pair.splitn(2, '=');
        let key = parts.next().unwrap_or_default().trim();
        let value = parts.next().unwrap_or_default().trim();
        if !key.is_empty() {
            map.insert(key.to_owned(), value.to_owned());
        }
    }
    map
}

fn current_utc_rfc3339() -> String {
    time::OffsetDateTime::now_utc()
        .format(&time::format_description::well_known::Rfc3339)
        .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned())
}
