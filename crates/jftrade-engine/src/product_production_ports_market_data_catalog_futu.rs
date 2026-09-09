//! Futu catalog reads through the shared OpenD runtime; no quote subscriptions.

use super::{MarketDataCatalogReadSnapshotError as Error, ProductionMarketDataCatalogPort};
use jftrade_integration_futu::{InstrumentSearchEntry, InstrumentSearchError};
use serde_json::{Value, json};
use std::collections::HashSet;

pub(super) async fn read(
    port: &ProductionMarketDataCatalogPort,
    market: &str,
    query: &str,
    limit: usize,
) -> Result<Value, Error> {
    let market = market.trim().to_ascii_uppercase();
    let qualified = qualified_instrument(&market, query)?;
    let reader = port
        .trade_runtime
        .as_ref()
        .and_then(|runtime| {
            runtime
                .instrument_search_reader
                .read()
                .unwrap_or_else(|error| error.into_inner())
                .clone()
        })
        .ok_or_else(|| {
            Error::Unavailable("Futu instrument search runtime is unavailable".to_owned())
        })?;
    let keyword = query.to_owned();
    let target = qualified.clone();
    let entries = tokio::task::spawn_blocking(move || {
        if let Some(instrument) = target {
            reader.lookup(&instrument.prefix, &instrument.code)
        } else {
            reader.search(&keyword)
        }
    })
    .await
    .map_err(|error| Error::Unavailable(error.to_string()))?
    .map_err(map_error)?;
    Ok(project(entries, &market, query, qualified.as_ref(), limit))
}

fn qualified_instrument(
    market: &str,
    query: &str,
) -> Result<Option<jftrade_marketdata::NormalizedInstrument>, Error> {
    let normalized = query.trim().to_ascii_uppercase().replace(':', ".");
    let Some((prefix, _)) = normalized.split_once('.') else {
        return Ok(None);
    };
    if !jftrade_marketdata::catalog::is_supported_market(prefix) {
        return Ok(None);
    }
    jftrade_marketdata::normalize_instrument(
        (!market.is_empty()).then_some(market),
        Some(&normalized),
    )
    .map(Some)
    .map_err(|error| Error::Invalid {
        code: "MARKET_INSTRUMENT_INVALID".to_owned(),
        message: error.to_string(),
    })
}

fn project(
    entries: Vec<InstrumentSearchEntry>,
    market: &str,
    query: &str,
    qualified: Option<&jftrade_marketdata::NormalizedInstrument>,
    limit: usize,
) -> Value {
    let mut seen = HashSet::new();
    // Preserve OpenD relevance order and apply the caller's limit only after
    // market filtering, deduplication and ambiguity classification.
    let mut entries: Vec<_> = entries
        .into_iter()
        .filter(|entry| {
            let market_matches = market.is_empty()
                || entry.market == market
                || (market == "CN" && matches!(entry.market.as_str(), "SH" | "SZ"));
            let target_matches = qualified.is_none_or(|target| {
                entry.market == target.prefix && entry.code.eq_ignore_ascii_case(&target.code)
            });
            market_matches
                && target_matches
                && seen.insert((entry.market.clone(), entry.code.clone()))
        })
        .collect();
    if qualified.is_none()
        && entries
            .iter()
            .any(|entry| entry.code.eq_ignore_ascii_case(query.trim()))
    {
        entries.retain(|entry| entry.code.eq_ignore_ascii_case(query.trim()));
    }
    let status = if !entries.is_empty() && !entries.iter().any(|entry| selectable(&entry.market)) {
        "unavailable"
    } else {
        match entries.len() {
            0 => "not_found",
            1 => "resolved",
            _ => "ambiguous",
        }
    };
    entries.truncate(limit);
    let entries: Vec<_> = entries.into_iter().map(candidate).collect();
    json!({"query": query, "requestedMarket": market, "resolutionStatus": status,
        "totalReturned": entries.len(), "entries": entries, "failures": []})
}

fn selectable(market: &str) -> bool {
    matches!(market, "HK" | "US" | "SH" | "SZ")
}

fn candidate(entry: InstrumentSearchEntry) -> Value {
    let selectable = selectable(&entry.market);
    json!({
        "instrumentId": format!("{}.{}", entry.market, entry.code),
        "resolvedMarket": if matches!(entry.market.as_str(), "SH" | "SZ") { "CN" } else { &entry.market },
        "market": entry.market, "code": entry.code, "symbol": entry.code,
        "name": entry.name, "securityType": entry.security_type, "lotSize": entry.lot_size,
        "source": "futu", "isWatched": entry.is_watched, "selectable": selectable,
        "unavailableReason": if selectable { None } else { Some(format!("当前版本暂不支持 {} 市场", entry.market)) },
    })
}

fn map_error(error: InstrumentSearchError) -> Error {
    match error {
        InstrumentSearchError::InvalidQuery => Error::Invalid {
            code: "MARKET_INSTRUMENT_INVALID".to_owned(),
            message: error.to_string(),
        },
        InstrumentSearchError::Session(_) => Error::Unavailable(error.to_string()),
        _ => Error::Failed {
            status: 502,
            code: "MARKET_INSTRUMENT_SEARCH_FAILED".to_owned(),
            message: error.to_string(),
        },
    }
}
