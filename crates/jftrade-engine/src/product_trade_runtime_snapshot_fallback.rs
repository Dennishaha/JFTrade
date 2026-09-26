//! Delayed StockScreen fallback and tick-cache helpers for the trade runtime.
//!
//! Go keeps the delayed capability between the 3203 snapshot read and the tick
//! cache: when the primary read leaves instruments unanswered (typically
//! because the BasicQot entitlement is missing) the watchlist asks the
//! adapter's `SnapshotFallbackSource` for delayed rows, and only then falls back
//! to cached ticks. The wire shape and the single-owner rules are unchanged;
//! this file exists so `product_trade_runtime_projection` stays inside the
//! bounded production-file budget.

use std::sync::Arc;

use serde_json::{json, Map, Value};

use jftrade_integration_futu::DelayedSnapshotItem;

use jftrade_marketdata::CacheLookup;

use super::product_trade_runtime_projection_values::insert_rich_security_fields;
use super::super::super::product_production_ports_market_data::product_production_ports_market_data_projection::format_unix_millis_rfc3339;
use super::{SharedTradeReadRuntime, TradeSecurity, qot_market_label};

/// Project one delayed row onto the `snapshots[]` wire object.
///
/// `source` stays `futu:stock-screen-delayed` so the console never mistakes a
/// delayed read for a BasicQot sample.
pub(super) fn delayed_snapshot_value(item: &DelayedSnapshotItem) -> Value {
    let mut projected = Map::new();
    projected.insert("symbol".to_owned(), json!(item.symbol));
    projected.insert("source".to_owned(), json!(item.source));
    if let Some(name) = item.name.as_ref() {
        projected.insert("name".to_owned(), json!(name));
    }
    for (key, value) in [
        ("lastPrice", item.last_price),
        ("previousClose", item.previous_close),
        ("openPrice", item.open_price),
        ("highPrice", item.high_price),
        ("lowPrice", item.low_price),
        ("bidPrice", item.bid_price),
        ("askPrice", item.ask_price),
    ] {
        if let Some(value) = value {
            projected.insert(key.to_owned(), json!(value));
        }
    }
    if let Some(session) = item.session.as_ref() {
        projected.insert("session".to_owned(), json!(session));
    }
    Value::Object(projected)
}

/// Go `lookupTickSnapshot`: a fresh-or-stale cached tick projected onto the
/// `snapshots[]` shape, enriched with whatever rich fields the tick carries.
pub(super) fn lookup_tick_snapshot(
    cache: &jftrade_marketdata::TickCache,
    security: &TradeSecurity,
    now_ms: i64,
) -> Option<Value> {
    let market = qot_market_label(security.market)?;
    let instrument_id = format!("{market}.{}", security.code);
    let tick = match cache.lookup(&instrument_id, now_ms, 30_000) {
        CacheLookup::Fresh(tick) | CacheLookup::Stale(tick) => tick,
        CacheLookup::Missing => return None,
    };
    let volume = tick.volume.as_str().parse::<serde_json::Number>().ok()?;
    let mut snapshot = Map::from_iter([
        ("symbol".to_owned(), Value::String(tick.instrument_id)),
        ("lastPrice".to_owned(), json!(tick.price)),
        ("volume".to_owned(), Value::Number(volume)),
        (
            "observedAt".to_owned(),
            Value::String(format_unix_millis_rfc3339(tick.observed_at_ms)),
        ),
    ]);
    if let Some(rich) = tick.snapshot {
        let _ = insert_rich_security_fields(&mut snapshot, &rich);
    }
    Some(Value::Object(snapshot))
}

impl SharedTradeReadRuntime {
    pub(crate) fn set_snapshot_fallback(
        &self,
        fallback: Option<Arc<jftrade_integration_futu::StockScreenSnapshotFallback>>,
    ) {
        *self
            .snapshot_fallback
            .write()
            .unwrap_or_else(|error| error.into_inner()) = fallback;
    }

    fn snapshot_fallback(
        &self,
    ) -> Option<Arc<jftrade_integration_futu::StockScreenSnapshotFallback>> {
        self.snapshot_fallback
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
    }

    /// Ask the delayed adapter capability for the instruments the primary read
    /// left unanswered, appending the resolved rows to `results`.
    ///
    /// Returns the securities that are still unresolved so the caller keeps
    /// falling through to the tick cache, exactly like Go's
    /// `queryFutuSnapshotBatch`. A symbol the delayed provider did not answer
    /// for stays unresolved: Go never synthesizes a row here.
    pub(super) fn resolve_delayed_snapshots<'a>(
        &self,
        results: &mut Vec<Value>,
        missing: Vec<&'a TradeSecurity>,
    ) -> (Vec<&'a TradeSecurity>, Option<String>) {
        if missing.is_empty() {
            return (missing, None);
        }
        let instruments = missing
            .iter()
            .filter_map(|security| {
                qot_market_label(security.market)
                    .map(|market| format!("{}.{}", market, security.code.trim()))
            })
            .collect::<Vec<_>>();
        let Some(fallback) = self.snapshot_fallback() else {
            return (missing, None);
        };
        let items = match fallback.query(&instruments) {
            Ok(items) => items,
            Err(error) => {
                // Keep successful realtime rows in mixed requests, while
                // preserving the delayed error for fallback-only requests.
                return (missing, Some(error.to_string()));
            }
        };
        let resolved = items
            .iter()
            .map(|item| item.symbol.trim().to_ascii_uppercase())
            .collect::<std::collections::HashSet<_>>();
        for item in &items {
            results.push(delayed_snapshot_value(item));
        }
        (
            missing
                .into_iter()
                .filter(|security| {
                    let code = security.code.trim().to_ascii_uppercase();
                    qot_market_label(security.market)
                        .is_none_or(|market| !resolved.contains(&format!("{market}.{code}")))
                })
                .collect(),
            None,
        )
    }
}
