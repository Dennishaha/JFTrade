//! Tick-candle read owner for the production market-data quote port.
//!
//! Parity: `internal/marketdata/service.go::GetCandles` (tick branch),
//! `internal/marketdata/candles.go::TickCandles` and
//! `internal/app/apiserver/marketdataapp/market_http_test.go` tick cases.

use super::*;
use jftrade_marketdata::{CacheLookup, tick_candles};

/// Freshness window Go applies before it decides a cached sample can answer a
/// live read without touching the provider (`TickFreshness`).
pub(super) const TICK_FRESHNESS_MS: i64 = 1_500;

impl ProductionMarketDataQuotePort {
    pub(super) async fn read_tick_candles(
        &self,
        market: &str,
        symbol: &str,
        sessions: &[&str],
        limit: usize,
        from_time: Option<String>,
        to_time: Option<String>,
    ) -> Result<Value, MarketDataQuoteReadSnapshotError> {
        let instrument_id = format!("{market}.{symbol}");
        let now_ms = current_unix_millis();
        let from_ms = parse_tick_window_bound(from_time.as_deref())?;
        let to_ms = parse_tick_window_bound(to_time.as_deref())?;

        // Go reads `cache.Latest(instrumentID, TickFreshness)` first and only
        // queries the provider ticker when nothing fresh is cached.
        let mut from_cache = false;
        if let Some((cache_handle, _)) = self.tick_cache_handle() {
            let cache_guard = cache_handle
                .lock()
                .unwrap_or_else(|error| error.into_inner());
            from_cache = matches!(
                cache_guard.lookup(&instrument_id, now_ms, TICK_FRESHNESS_MS),
                CacheLookup::Fresh(_)
            );
        }

        if !from_cache {
            let provider_generation = self.active_provider_state.snapshot().generation;
            let ticker_error = match self
                .trade_runtime
                .as_ref()
                .and_then(|runtime| runtime.ticker_quotes_reader())
            {
                Some(reader) => match reader.query_ticker(&instrument_id, now_ms) {
                    Ok(Some(mut tick)) => {
                        if let Some((cache_handle, cache_generation)) = self.tick_cache_handle() {
                            let mut cache_guard = cache_handle
                                .lock()
                                .unwrap_or_else(|error| error.into_inner());
                            // Go's in-memory cache has no generation fence, so
                            // `Ingest` always stores the sample it just read.
                            // Rust's cache fences on `provider_generation`, and
                            // the read owner has already confirmed above and
                            // re-confirms below that the active provider did
                            // not change, so the sample is stamped with the
                            // router's generation rather than being rejected.
                            tick.provider_generation = cache_generation;
                            let _ = cache_guard.insert(tick, cache_generation);
                        }
                        None
                    }
                    // Go's `QueryTicker` may answer without a usable sample
                    // (`nil, nil`). Nothing is ingested and the read still
                    // succeeds from whatever the cache retained.
                    Ok(None) => None,
                    Err(error) => Some(error.to_string()),
                },
                None => Some("Futu ticker reader is unavailable".to_owned()),
            };
            if self.active_provider_state.snapshot().generation != provider_generation {
                return Err(provider_changed_error());
            }
            // Go's fallback answers from whatever the cache retained and only
            // surfaces the provider error when no candle can be produced.
            if let Some(error) = ticker_error {
                let candles = self.project_tick_candles(
                    &instrument_id,
                    sessions,
                    limit,
                    from_ms,
                    to_ms,
                    now_ms,
                );
                if candles.is_empty() {
                    return Err(MarketDataQuoteReadSnapshotError::Unavailable(error));
                }
                return Ok(tick_candles_response(TickCandlesResponse {
                    market,
                    symbol,
                    instrument_id: &instrument_id,
                    limit,
                    candles,
                    source: "futu",
                    from_cache: true,
                    sessions,
                }));
            }
        }

        let candles =
            self.project_tick_candles(&instrument_id, sessions, limit, from_ms, to_ms, now_ms);
        Ok(tick_candles_response(TickCandlesResponse {
            market,
            symbol,
            instrument_id: &instrument_id,
            limit,
            candles,
            source: "futu",
            from_cache,
            sessions,
        }))
    }

    /// Returns the router-owned cache together with the runtime generation the
    /// snapshot poller fences it with. Go stores every ingested sample; Rust's
    /// cache rejects a sample whose `provider_generation` no longer matches, so
    /// the ingester must stamp the same recorder generation
    /// `SnapshotPollExecutor` uses instead of the settings provider's.
    fn tick_cache_handle(
        &self,
    ) -> Option<(Arc<Mutex<jftrade_marketdata::TickCache>>, u64)> {
        let router = self.router.as_ref()?;
        let router_guard = router.lock().unwrap_or_else(|error| error.into_inner());
        let generation = router_guard.runtime_recorder().snapshot().generation;
        Some((router_guard.cache_handle(), generation))
    }

    fn project_tick_candles(
        &self,
        instrument_id: &str,
        sessions: &[&str],
        limit: usize,
        from_ms: Option<i64>,
        to_ms: Option<i64>,
        now_ms: i64,
    ) -> Vec<Value> {
        let Some((cache_handle, _)) = self.tick_cache_handle() else {
            return Vec::new();
        };
        let history = {
            let cache_guard = cache_handle
                .lock()
                .unwrap_or_else(|error| error.into_inner());
            cache_guard.history(instrument_id)
        };
        // Go filters by session first and only then applies the limit to the
        // newest candles (`limitCandleMaps`), so a requested `extended` page
        // must not be truncated by regular-session samples.
        let mut candles = tick_candles(&history, from_ms, to_ms, now_ms, 0)
            .into_iter()
            .filter(|candle| {
                session_group(candle.session.as_deref()).is_some_and(|group| {
                    sessions
                        .iter()
                        .any(|requested| requested.eq_ignore_ascii_case(group))
                })
            })
            .collect::<Vec<_>>();
        if limit > 0 && candles.len() > limit {
            candles = candles.split_off(candles.len() - limit);
        }
        candles
            .into_iter()
            .map(|candle| candle.to_value(&format_unix_millis_rfc3339(candle.observed_at_ms)))
            .collect()
    }
}

/// Maps a raw provider session label onto the requested candle session group.
/// An unlabelled sample counts as `regular`, matching Go's
/// `CandleSessionForLabel` fallback in `FilterCandlesBySessions`.
pub(super) fn session_group(label: Option<&str>) -> Option<&'static str> {
    let label = label.unwrap_or("regular").trim().to_ascii_lowercase();
    match label.as_str() {
        "regular" => Some("regular"),
        "pre" | "post" | "after" | "extended" => Some("extended"),
        "overnight" => Some("overnight"),
        _ => None,
    }
}

/// Parses a candle `from`/`to` query value into the millisecond bound used by
/// the tick window. Go stores these as RFC3339 strings and only compares them
/// against the sample `ObservedAt`.
fn parse_tick_window_bound(
    raw: Option<&str>,
) -> Result<Option<i64>, MarketDataQuoteReadSnapshotError> {
    let Some(raw) = raw else {
        return Ok(None);
    };
    let parsed = time::OffsetDateTime::parse(
        raw,
        &time::format_description::well_known::Rfc3339,
    )
    .map_err(|_| MarketDataQuoteReadSnapshotError::Failed {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: "time must be a valid timestamp".to_owned(),
        retry_after_seconds: None,
    })?;
    Ok(Some(
        i64::try_from(parsed.unix_timestamp_nanos() / 1_000_000).unwrap_or_default(),
    ))
}

/// Inputs of the Go `tickCandlesResponse` envelope.
struct TickCandlesResponse<'a> {
    market: &'a str,
    symbol: &'a str,
    instrument_id: &'a str,
    limit: usize,
    candles: Vec<Value>,
    source: &'a str,
    from_cache: bool,
    sessions: &'a [&'a str],
}

/// Builds the Go `tickCandlesResponse` envelope. `meta.session` is emitted
/// only for US reads, which is exactly Go's `includeSession := market == "US"`.
fn tick_candles_response(response: TickCandlesResponse<'_>) -> Value {
    let TickCandlesResponse {
        market,
        symbol,
        instrument_id,
        limit,
        candles,
        source,
        from_cache,
        sessions,
    } = response;
    // `tickCandlesResponse` sets both `ExtendedHours` and `IncludeSession` from
    // `market == "US"`, so a US tick read always reports `extendedHours: true`
    // and `meta.session: "all"` regardless of which sessions were requested.
    let include_session = market.eq_ignore_ascii_case("US");
    let mut meta = json!({
        "brokerId": "futu",
        "extendedHours": include_session,
        "fromCache": from_cache,
        "instrumentId": instrument_id,
        "resolvedAt": format_unix_millis_rfc3339(current_unix_millis()),
        "sessions": sessions,
        "source": source,
    });
    if include_session {
        meta["session"] = json!("all");
    }
    json!({
        "candles": candles,
        "meta": meta,
        "pagination": { "hasMore": false },
        "request": {
            "instrument": {
                "instrumentId": instrument_id,
                "market": market,
                "symbol": symbol,
            },
            "limit": limit,
            "period": "tick",
            "sessions": sessions,
        },
        "totalReturned": candles.len(),
    })
}
