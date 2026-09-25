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
    // Go's MarketSubsetInstrumentResolver always asks its provider for the
    // maximum candidate window, then applies market filtering, de-duplication,
    // exact-code narrowing, and the public limit. Passing the public limit to
    // OpenD would truncate candidates before the CN/SH/SZ filter and change an
    // ambiguous result into a false single match.
    let requested = jftrade_integration_futu::MAX_SEARCH_QUOTE_COUNT;
    let entries = tokio::task::spawn_blocking(move || {
        if let Some(instrument) = target {
            reader.lookup(&instrument.prefix, &instrument.code)
        } else {
            reader.search_with_limit(&keyword, requested)
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
    // Go canonicalizes the row through `canonicalSearchQuoteSymbol`: an OpenD
    // code that already carries its own market prefix (`US.AAPL`,
    // `CNSH.600519`) must not be prefixed a second time, otherwise the public
    // `instrumentId` becomes `US.US.AAPL` / `SH.CNSH.600519` and no longer
    // resolves to the security the broker returned.
    let code = canonical_search_code(&entry.market, &entry.code);
    json!({
        "instrumentId": format!("{}.{}", entry.market, code),
        "resolvedMarket": if matches!(entry.market.as_str(), "SH" | "SZ") { "CN" } else { &entry.market },
        "market": entry.market, "code": code, "symbol": code,
        "name": entry.name, "securityType": entry.security_type, "lotSize": entry.lot_size,
        "source": "futu", "isWatched": entry.is_watched, "selectable": selectable,
        "unavailableReason": if selectable { None } else { Some(format!("当前版本暂不支持 {} 市场", entry.market)) },
    })
}

/// Go `canonicalSearchQuoteMarketPrefix` + `canonicalSearchQuoteSymbol`.
///
/// A leading prefix is stripped only when it names the row's own market, with
/// `CNSH`/`CNSZ` collapsing onto `SH`/`SZ` and `CC` onto `CRYPTO`. Anything
/// else (including a nested `BRK.B`) is preserved verbatim so the code stays
/// the exact broker identity.
fn canonical_search_code(market: &str, code: &str) -> String {
    let code = code.trim().to_ascii_uppercase().replace(':', ".");
    let Some((prefix, bare)) = code.split_once('.') else {
        return code;
    };
    let prefix = canonical_search_market_prefix(prefix);
    let bare = bare.trim();
    if prefix == market && !bare.is_empty() {
        bare.to_owned()
    } else {
        code
    }
}

fn canonical_search_market_prefix(value: &str) -> String {
    match value.trim().to_ascii_uppercase().as_str() {
        "CNSH" => "SH".to_owned(),
        "CNSZ" => "SZ".to_owned(),
        "HKFUTURE" | "HK_FUTURES" => "HK_FUTURE".to_owned(),
        "CC" => "CRYPTO".to_owned(),
        other => other.to_owned(),
    }
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

#[cfg(test)]
mod tests {
    use super::*;

    // Parity: go:452dea11:internal/app/apiserver/marketdataapp/provider_test.go:86 TestBrokerSearchInstrumentPartsNormalizesKnownPrefixes
    /// Parity: go:452dea11:pkg/futu/marketdata_reader_boundaries_test.go:188
    /// TestMarketDataRuleHelpersRejectIncompleteBrokerPayloads (prefix half).
    ///
    /// Go's `canonicalSearchQuoteMarketPrefix` maps the mainland aliases onto
    /// the exchange prefixes, folds the HK future alias onto `HK_FUTURE`, and
    /// rewrites the crypto alias, leaving every other market untouched.
    #[test]
    fn search_quote_market_prefixes_follow_the_go_aliases() {
        for (input, expected) in [
            ("cnsh", "SH"),
            ("cnsz", "SZ"),
            ("hkfuture", "HK_FUTURE"),
            ("CC", "CRYPTO"),
            ("us", "US"),
        ] {
            assert_eq!(
                canonical_search_market_prefix(input),
                expected,
                "input={input:?}"
            );
        }
    }

    // Parity: go:452dea11:internal/app/apiserver/marketdataapp/provider_boundaries_test.go:271 TestBrokerSearchInstrumentPartsPreservesDottedCodes
    /// Parity: go:452dea11:pkg/futu/adapter_marketdata_search_test.go
    /// `canonicalSearchQuoteCode`: the bare code is kept only when the entry's
    /// market matches the caller's market prefix, otherwise the provider code
    /// (which already carries its own prefix) is preserved.
    #[test]
    fn canonical_search_code_does_not_double_prefix_the_market() {
        assert_eq!(canonical_search_code("HK", "00700"), "00700");
        assert_eq!(canonical_search_code("US", "HK.00700"), "HK.00700");
        assert_eq!(canonical_search_code("CNSH", "600519"), "600519");
        assert_eq!(canonical_search_code("US", "AAPL"), "AAPL");
    }

    // Parity: go:452dea11:internal/app/apiserver/marketdataapp/provider_test.go:107 TestBrokerSearchInstrumentPartsRejectsUnknownPrefixInference
    /// An unknown prefix is never folded onto a market: the helper keeps the
    /// prefix itself instead of inferring HK/SH/SZ. Case folding is the shared
    /// normalization used by every search entrypoint.
    #[test]
    fn unknown_search_prefixes_are_never_inferred_as_a_market() {
        assert_eq!(canonical_search_market_prefix("bad"), "BAD");
        assert_eq!(canonical_search_code("", "bad.CODE"), "BAD.CODE");
        assert_eq!(canonical_search_code("", "00700"), "00700");
    }
}
