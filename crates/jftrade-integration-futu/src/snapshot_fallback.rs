//! Delayed `Qot_StockScreen` (3252) snapshot fallback.
//!
//! Parity source: `go:452dea11:pkg/futu/snapshot_fallback.go`.
//!
//! Go keeps this as an *adapter capability* rather than a second market-data
//! provider: when a symbol cannot be quoted through `Qot_Sub`/`Qot_GetBasicQot`
//! because the account holds no BasicQot entitlement, the watchlist asks the
//! broker object for a `SnapshotFallbackSource` and reads the same fields from
//! the delayed StockScreen protocol instead. The delayed read is strictly
//! read-only - it never creates a subscription, which is why the Go-side
//! fixture asserts `subCallCount() == 0` on every path.

use std::collections::{BTreeMap, HashMap};
use std::sync::{Arc, Condvar, Mutex};
use std::time::Duration;

use jftrade_kernel::Decimal;
use prost::Message;
use thiserror::Error;

use crate::OpenDManagedSessionError;
use crate::OpenDSessionCoordinator;
use crate::OpenDSessionCoordinatorError;
use crate::STOCK_SCREEN_PROTOCOL_ID;
use crate::security_snapshot_query::market_code;
use crate::trade_proto::qot_common::Security as WireSecurity;
use crate::trade_proto::qot_get_static_info as static_wire;
use crate::trade_proto::qot_stock_screen as screen_wire;

/// Go `stockScreenSnapshotCacheTTL`: a successful *and* a negative result stay
/// valid for 15s so several quote surfaces asking for the same unentitled
/// symbol collapse onto one delayed read.
pub const STOCK_SCREEN_SNAPSHOT_CACHE_TTL: Duration = Duration::from_secs(15);
/// Go `stockScreenSnapshotPageSize`: `Qot_StockScreen.pageCount` maximum.
pub const STOCK_SCREEN_SNAPSHOT_PAGE_SIZE: usize = 300;
/// The delayed rows are attributed to this source, not to the BasicQot path.
pub const STOCK_SCREEN_SNAPSHOT_SOURCE: &str = "futu:stock-screen-delayed";

/// `SimpleProperty` names Go retrieves for the fallback projection.
const SIMPLE_PROPERTIES: [i32; 7] = [2201, 2202, 2203, 2204, 2205, 2207, 2208];
/// `CumulativeProperty.ChangePct` over one day, used to derive the previous close.
const CUMULATIVE_CHANGE_PCT: i32 = 3101;
/// How often a parked caller re-checks its cancellation token.
const CANCEL_POLL_INTERVAL: Duration = Duration::from_millis(2);
/// `SimpleField` selectors: 1 = market, 4 = watchlist membership.
const SIMPLE_FIELD_MARKET: i32 = 1;
const SIMPLE_FIELD_WATCHLIST: i32 = 4;

/// Neutral delayed-snapshot row, mirroring Go's `broker.SecuritySnapshotItem`
/// subset the fallback fills.
#[derive(Clone, Debug, Default, PartialEq)]
pub struct DelayedSnapshotItem {
    pub symbol: String,
    /// Always [`STOCK_SCREEN_SNAPSHOT_SOURCE`]; Go stamps every delayed row so
    /// the console can label it apart from a BasicQot sample.
    pub source: String,
    pub name: Option<String>,
    pub last_price: Option<Decimal>,
    pub previous_close: Option<Decimal>,
    pub open_price: Option<Decimal>,
    pub high_price: Option<Decimal>,
    pub low_price: Option<Decimal>,
    pub bid_price: Option<Decimal>,
    pub ask_price: Option<Decimal>,
    /// Broker-neutral session label for the row's observation time.
    pub session: Option<String>,
}

#[derive(Clone, Debug, Error)]
pub enum SnapshotFallbackError {
    #[error("futu stock-screen snapshot fallback is unavailable")]
    Unavailable,
    #[error("futu: QuerySnapshotFallback requires at least one symbol")]
    EmptySymbols,
    #[error("invalid OpenD security snapshot instrument: {0}")]
    InvalidInstrument(String),
    #[error(
        "futu stock-screen snapshot page requires 1..{STOCK_SCREEN_SNAPSHOT_PAGE_SIZE} stock ids"
    )]
    InvalidPageSize,
    #[error("futu stock-screen snapshot fallback returned an invalid result")]
    InvalidResult,
    /// A physical static-info / StockScreen failure. The message is passed
    /// through unchanged so the OpenD protocol name survives into the caller.
    #[error("{0}")]
    Fetch(String),
    /// Go checks `ctx.Err()` before the physical read and parks on the
    /// single-flight channel behind a `select`; Rust has no context object, so
    /// the same "the caller gave up" contract is a token the caller may flip.
    #[error("futu stock-screen snapshot query was canceled before it completed")]
    Canceled,
    #[error("OpenD Qot_GetStaticInfo returned retType={ret_type} errCode={err_code}: {message}")]
    StaticInfoRejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("decode OpenD static-info response: {0}")]
    StaticInfoDecode(#[from] prost::DecodeError),
    #[error("OpenD Qot_StockScreen returned retType={ret_type} errCode={err_code}: {message}")]
    ScreenRejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
}

impl From<OpenDSessionCoordinatorError> for SnapshotFallbackError {
    fn from(error: OpenDSessionCoordinatorError) -> Self {
        Self::Fetch(error.to_string())
    }
}

impl From<OpenDManagedSessionError> for SnapshotFallbackError {
    fn from(error: OpenDManagedSessionError) -> Self {
        Self::Fetch(error.to_string())
    }
}

/// The physical reads the fallback needs. Split from the session so the
/// projection, batching and cache rules can be exercised without OpenD.
pub trait SnapshotFallbackFetchPort: Send + Sync + std::fmt::Debug {
    /// Resolves `MARKET.CODE` symbols onto their OpenD stock ids plus names.
    fn static_info_ids(&self, symbols: &[String]) -> Result<Vec<StockIdentity>, String>;

    /// One delayed `Qot_StockScreen` page for a single market value.
    fn stock_screen_page(
        &self,
        market_value: i64,
        stock_ids: &[u64],
    ) -> Result<Vec<ScreenRow>, String>;
}

/// One static-info row reduced to what the fallback consumes.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct StockIdentity {
    pub symbol: String,
    pub stock_id: u64,
    pub name: Option<String>,
}

/// One delayed StockScreen row, already reduced to property→value pairs.
#[derive(Clone, Debug, Default, PartialEq)]
pub struct ScreenRow {
    pub stock_id: u64,
    pub simple: BTreeMap<i32, f64>,
    pub cumulative: BTreeMap<i32, f64>,
}

#[derive(Default)]
struct Flight {
    state: Mutex<FlightState>,
    ready: Condvar,
}

#[derive(Default)]
struct FlightState {
    /// Set by the caller that claimed the flight. Guarded by the same mutex as
    /// the outcome so exactly one caller performs the physical read.
    started: bool,
    outcome: Option<Result<HashMap<String, DelayedSnapshotItem>, SnapshotFallbackError>>,
}

struct CoordinatorInner {
    cache: Mutex<HashMap<String, CachedRow>>,
    flights: Mutex<HashMap<String, Arc<Flight>>>,
    clock: Arc<dyn Fn() -> std::time::SystemTime + Send + Sync>,
    ttl: Duration,
}

#[derive(Clone)]
struct CachedRow {
    item: Option<DelayedSnapshotItem>,
    expires_at: std::time::SystemTime,
}

/// Delayed-read coordinator: 15s positive/negative TTL plus single-flight
/// coalescing keyed by the resolved symbol batch.
///
/// Go keeps one coordinator per *adapter* (`stockScreenSnapshotCoordinator`),
/// so several quote surfaces asking for the same unentitled symbol share one
/// delayed read. A failed fetch is deliberately not cached.
#[derive(Clone)]
pub struct StockScreenSnapshotCoordinator {
    inner: Arc<CoordinatorInner>,
}

impl std::fmt::Debug for StockScreenSnapshotCoordinator {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("StockScreenSnapshotCoordinator")
            .finish_non_exhaustive()
    }
}

impl Default for StockScreenSnapshotCoordinator {
    fn default() -> Self {
        Self::with_settings(
            Arc::new(std::time::SystemTime::now),
            STOCK_SCREEN_SNAPSHOT_CACHE_TTL,
        )
    }
}

impl StockScreenSnapshotCoordinator {
    pub fn new() -> Self {
        Self::default()
    }

    /// Clock/TTL seam so the TTL boundary can be exercised deterministically.
    pub fn with_settings(
        clock: Arc<dyn Fn() -> std::time::SystemTime + Send + Sync>,
        ttl: Duration,
    ) -> Self {
        Self {
            inner: Arc::new(CoordinatorInner {
                cache: Mutex::new(HashMap::new()),
                flights: Mutex::new(HashMap::new()),
                clock,
                ttl,
            }),
        }
    }

    /// Resolves `symbols` from the delayed cache and, for everything missing,
    /// through `fetch` in one coalesced physical read.
    ///
    /// The returned map is keyed by canonical (upper-cased) symbol. A symbol
    /// the provider answered nothing for is stored as a negative entry, exactly
    /// like Go's nil `cachedStockScreenSnapshot.item`.
    pub fn query<F>(
        &self,
        symbols: &[String],
        fetch: F,
    ) -> Result<BTreeMap<String, DelayedSnapshotItem>, SnapshotFallbackError>
    where
        F: Fn(&[String]) -> Result<HashMap<String, DelayedSnapshotItem>, SnapshotFallbackError>,
    {
        self.query_with_cancel(symbols, None, fetch)
    }

    /// Cancel-aware variant of [`Self::query`].
    ///
    /// Go parks on `singleflight.DoChan` behind a `select` with `ctx.Done()`,
    /// so a caller that gives up stops waiting for the coalesced read. The
    /// token is the Rust equivalent. The leader's physical read is deliberately
    /// *not* interrupted (the same rule the 3203 coordinator documents), and a
    /// canceled waiter leaves the flight entry in place so it cannot start a
    /// second physical read behind the leader's back.
    pub fn query_with_cancel<F>(
        &self,
        symbols: &[String],
        cancel: Option<&crate::SecuritySnapshotCancelToken>,
        fetch: F,
    ) -> Result<BTreeMap<String, DelayedSnapshotItem>, SnapshotFallbackError>
    where
        F: Fn(&[String]) -> Result<HashMap<String, DelayedSnapshotItem>, SnapshotFallbackError>,
    {
        if cancel.is_some_and(crate::SecuritySnapshotCancelToken::is_canceled) {
            return Err(SnapshotFallbackError::Canceled);
        }
        let canonical = canonical_fallback_symbols(symbols)?;
        if canonical.is_empty() {
            return Ok(BTreeMap::new());
        }
        let (mut result, missing) = self.cached(&canonical);
        if missing.is_empty() {
            return Ok(result);
        }
        let key = missing.join("\u{0}");
        let flight = {
            let mut flights = self
                .inner
                .flights
                .lock()
                .unwrap_or_else(|error| error.into_inner());
            Arc::clone(
                flights
                    .entry(key.clone())
                    .or_insert_with(|| Arc::new(Flight::default())),
            )
        };
        // Exactly one caller claims the flight and performs the physical read;
        // everyone else parks on the same outcome, which is Go's
        // `singleflight.Group.DoChan` contract.
        let is_leader = {
            let mut state = flight
                .state
                .lock()
                .unwrap_or_else(|error| error.into_inner());
            if state.started {
                false
            } else {
                state.started = true;
                true
            }
        };
        if is_leader {
            let fetch_symbols = missing.clone();
            let store_symbols = missing.clone();
            let mut fetched = fetch(&fetch_symbols);
            if let Ok(items) = fetched.as_mut() {
                self.store(&store_symbols, items);
            }
            let mut state = flight
                .state
                .lock()
                .unwrap_or_else(|error| error.into_inner());
            state.outcome = Some(fetched);
            flight.ready.notify_all();
        }
        let mut state = flight
            .state
            .lock()
            .unwrap_or_else(|error| error.into_inner());
        while state.outcome.is_none() {
            if cancel.is_some_and(crate::SecuritySnapshotCancelToken::is_canceled) {
                // Leave the flight in place: the leader is still reading, and
                // a fresh caller must join it instead of duplicating the read.
                return Err(SnapshotFallbackError::Canceled);
            }
            let (next, _) = flight
                .ready
                .wait_timeout(state, CANCEL_POLL_INTERVAL)
                .unwrap_or_else(|error| error.into_inner());
            state = next;
        }
        if cancel.is_some_and(crate::SecuritySnapshotCancelToken::is_canceled) {
            drop(state);
            return Err(SnapshotFallbackError::Canceled);
        }
        let outcome: Result<HashMap<String, DelayedSnapshotItem>, SnapshotFallbackError> =
            match state.outcome.as_ref() {
                Some(Ok(items)) => Ok(items.clone()),
                Some(Err(error)) => Err(error.clone()),
                None => Err(SnapshotFallbackError::InvalidResult),
            };
        drop(state);
        self.inner
            .flights
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .remove(&key);
        let items = outcome?;
        for symbol in missing {
            if let Some(item) = items.get(&symbol) {
                result.insert(symbol.clone(), item.clone());
            }
        }
        Ok(result)
    }

    fn cached(&self, symbols: &[String]) -> (BTreeMap<String, DelayedSnapshotItem>, Vec<String>) {
        let now = (self.inner.clock)();
        let mut cache = self
            .inner
            .cache
            .lock()
            .unwrap_or_else(|error| error.into_inner());
        let mut result = BTreeMap::new();
        let mut missing = Vec::new();
        for symbol in symbols {
            match cache.get(symbol) {
                Some(entry) if entry.expires_at > now => {
                    if let Some(item) = &entry.item {
                        result.insert(symbol.clone(), item.clone());
                    }
                }
                Some(_) => {
                    cache.remove(symbol);
                    missing.push(symbol.clone());
                }
                None => missing.push(symbol.clone()),
            }
        }
        (result, missing)
    }

    fn store(&self, symbols: &[String], items: &HashMap<String, DelayedSnapshotItem>) {
        let expires_at = (self.inner.clock)() + self.inner.ttl;
        let mut cache = self
            .inner
            .cache
            .lock()
            .unwrap_or_else(|error| error.into_inner());
        for symbol in symbols {
            cache.insert(
                symbol.clone(),
                CachedRow {
                    item: items.get(symbol).cloned(),
                    expires_at,
                },
            );
        }
    }
}

/// Canonicalize `symbols` like Go's `canonicalStockScreenSnapshotSymbols`:
/// upper-cased, de-duplicated, sorted, and a malformed symbol fails the whole
/// request instead of being silently dropped.
pub fn canonical_fallback_symbols(
    symbols: &[String],
) -> Result<Vec<String>, SnapshotFallbackError> {
    let mut seen = std::collections::BTreeSet::new();
    for symbol in symbols {
        let trimmed = symbol.trim().to_ascii_uppercase();
        let Some((market, code)) = trimmed.split_once('.') else {
            return Err(SnapshotFallbackError::InvalidInstrument(symbol.clone()));
        };
        if market_code(market).is_none() || code.trim().is_empty() {
            return Err(SnapshotFallbackError::InvalidInstrument(symbol.clone()));
        }
        seen.insert(format!("{market}.{}", code.trim()));
    }
    Ok(seen.into_iter().collect())
}

/// Production delayed-snapshot reader owning the static-info resolution, the
/// per-market paging and the StockScreen projection.
pub struct OpenDSnapshotFallbackReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDSnapshotFallbackReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDSnapshotFallbackReader")
            .finish_non_exhaustive()
    }
}

impl OpenDSnapshotFallbackReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl SnapshotFallbackFetchPort for OpenDSnapshotFallbackReader {
    fn static_info_ids(&self, symbols: &[String]) -> Result<Vec<StockIdentity>, String> {
        let securities = symbols
            .iter()
            .map(|symbol| parse_symbol(symbol))
            .collect::<Result<Vec<_>, _>>()?;
        let request = static_wire::Request {
            c2s: static_wire::C2s {
                market: None,
                sec_type: None,
                security_list: securities,
                header: None,
            },
        }
        .encode_to_vec();
        let coordinator = self
            .coordinator
            .lock()
            .map_err(|_| "static-info coordinator lock poisoned".to_owned())?;
        let session = coordinator.session().map_err(|e| e.to_string())?;
        let body = session
            .managed_session()
            .call(static_wire::PROTOCOL_ID, &request)
            .map_err(|e| e.to_string())?;
        let response = static_wire::Response::decode(body.as_slice())
            .map_err(|e| format!("decode OpenD static-info response: {e}"))?;
        if response.ret_type != 0 {
            return Err(format!(
                "OpenD Qot_GetStaticInfo returned retType={} errCode={}: {}",
                response.ret_type,
                response.err_code.unwrap_or_default(),
                response
                    .ret_msg
                    .unwrap_or_else(|| "OpenD static-info request failed".to_owned())
            ));
        }
        let mut identities = Vec::new();
        for info in response
            .s2c
            .map(|s2c| s2c.static_info_list)
            .unwrap_or_default()
        {
            // Go skips rows without a positive stock id; it never invents one.
            if info.basic.id <= 0 {
                continue;
            }
            let Some(market) =
                crate::security_snapshot_query::market_label(info.basic.security.market)
            else {
                continue;
            };
            let code = info.basic.security.code.trim().to_ascii_uppercase();
            if code.is_empty() {
                continue;
            }
            identities.push(StockIdentity {
                symbol: format!("{market}.{code}"),
                stock_id: info.basic.id as u64,
                name: Some(info.basic.name).filter(|name| !name.trim().is_empty()),
            });
        }
        Ok(identities)
    }

    fn stock_screen_page(
        &self,
        market_value: i64,
        stock_ids: &[u64],
    ) -> Result<Vec<ScreenRow>, String> {
        validate_snapshot_page(stock_ids).map_err(|error| error.to_string())?;
        let request = encode_snapshot_page(market_value, stock_ids);
        let coordinator = self
            .coordinator
            .lock()
            .map_err(|_| "stock-screen coordinator lock poisoned".to_owned())?;
        let session = coordinator.session().map_err(|e| e.to_string())?;
        let body = session
            .managed_session()
            .call(STOCK_SCREEN_PROTOCOL_ID, &request)
            .map_err(|e| e.to_string())?;
        let response = screen_wire::Response::decode(body.as_slice())
            .map_err(|e| format!("decode OpenD stock-screen response: {e}"))?;
        if response.ret_type != 0 {
            return Err(format!(
                "OpenD Qot_StockScreen returned retType={} errCode={}: {}",
                response.ret_type,
                response.err_code.unwrap_or_default(),
                response
                    .ret_msg
                    .unwrap_or_else(|| "OpenD stock-screen request failed".to_owned())
            ));
        }
        Ok(response
            .s2c
            .map(|s2c| s2c.data_list.into_iter().filter_map(screen_row).collect())
            .unwrap_or_default())
    }
}

/// Go's `queryStockScreenSnapshotPage` guard: a delayed page must carry between
/// one and [`STOCK_SCREEN_SNAPSHOT_PAGE_SIZE`] stock ids, and an out-of-range
/// request is rejected before any OpenD call.
pub fn validate_snapshot_page(stock_ids: &[u64]) -> Result<(), SnapshotFallbackError> {
    if stock_ids.is_empty() || stock_ids.len() > STOCK_SCREEN_SNAPSHOT_PAGE_SIZE {
        return Err(SnapshotFallbackError::InvalidPageSize);
    }
    Ok(())
}

/// Encodes Go's `stockScreenSnapshotParams`: two simple-field filters (market,
/// watchlist membership), the strict delayed-quote retrieve list, the caller's
/// stock ids and `pageCount` equal to the id count.
fn encode_snapshot_page(market_value: i64, stock_ids: &[u64]) -> Vec<u8> {
    let mut retrieve_list: Vec<screen_wire::RetrieveQuery> = SIMPLE_PROPERTIES
        .iter()
        .map(|name| screen_wire::RetrieveQuery {
            simple_property: Some(screen_wire::PropertySimple { name: Some(*name) }),
            ..Default::default()
        })
        .collect();
    retrieve_list.push(screen_wire::RetrieveQuery {
        cumulative_property: Some(screen_wire::PropertyCumulative {
            name: Some(CUMULATIVE_CHANGE_PCT),
            days: Some(1),
            period_average: None,
        }),
        ..Default::default()
    });
    let filter = |field: i32, value: i64| screen_wire::ScreenQuery {
        simple_field_query: Some(screen_wire::QuerySimpleField {
            simple_field: Some(field),
            screen_value_list: vec![value],
        }),
        ..Default::default()
    };
    screen_wire::Request {
        c2s: screen_wire::C2s {
            filter_list: vec![
                filter(SIMPLE_FIELD_MARKET, market_value),
                filter(SIMPLE_FIELD_WATCHLIST, 1),
            ],
            retrieve_list,
            watchlist_stock_ids: stock_ids.to_vec(),
            sort: None,
            page_from: None,
            page_count: Some(stock_ids.len() as i32),
            sort_list: Vec::new(),
        },
    }
    .encode_to_vec()
}

fn screen_row(row: screen_wire::StockScreenItem) -> Option<ScreenRow> {
    let stock_id = row.stock_id?;
    if stock_id == 0 {
        return None;
    }
    let mut simple = BTreeMap::new();
    let mut cumulative = BTreeMap::new();
    for result in row.results {
        if let Some(value) = result.simple_property_result
            && let (Some(name), Some(number)) = (
                value.property.and_then(|p| p.name),
                result_number(value.dval, value.ival),
            )
        {
            simple.insert(name, number);
        }
        if let Some(value) = result.cumulative_property_result
            && let (Some(name), Some(number)) = (
                value.property.and_then(|p| p.name),
                result_number(value.dval, value.ival),
            )
        {
            cumulative.insert(name, number);
        }
    }
    Some(ScreenRow {
        stock_id,
        simple,
        cumulative,
    })
}

fn result_number(dval: Option<f64>, ival: Option<i64>) -> Option<f64> {
    if let Some(value) = dval.filter(|value| value.is_finite()) {
        return Some(value);
    }
    ival.map(|value| value as f64)
}

/// Projects one delayed page onto neutral snapshot rows.
///
/// Go keeps a row only when `2201` (last price) is present and strictly
/// positive; `previousClose` falls back to `lastPrice - 3101` (the one-day
/// change percent) when the provider omitted `2203`. Symbols the provider did
/// not answer for are simply absent, which the coordinator records as negative
/// cache entries.
pub fn project_screen_page(
    rows: &[ScreenRow],
    identities: &[StockIdentity],
) -> HashMap<String, DelayedSnapshotItem> {
    project_screen_page_at(rows, identities, None)
}

/// Same projection with the observation timestamp the session label needs.
///
/// Go passes `time.Now().UTC()` into `stockScreenSnapshotItems`, so every row
/// carries `source` plus the calendar-classified `session` for the moment the
/// delayed page was read.
pub fn project_screen_page_at(
    rows: &[ScreenRow],
    identities: &[StockIdentity],
    observed_at_ms: Option<i64>,
) -> HashMap<String, DelayedSnapshotItem> {
    let by_id: HashMap<u64, &StockIdentity> = identities
        .iter()
        .map(|info| (info.stock_id, info))
        .collect();
    let mut result = HashMap::new();
    for row in rows {
        let Some(identity) = by_id.get(&row.stock_id) else {
            continue;
        };
        let Some(price) = row
            .simple
            .get(&2201)
            .copied()
            .filter(|value| positive(*value))
        else {
            continue;
        };
        let last_price = decimal(price);
        let previous_close = row
            .simple
            .get(&2203)
            .copied()
            .filter(|value| value.is_finite())
            .and_then(decimal)
            .or_else(|| {
                row.cumulative
                    .get(&CUMULATIVE_CHANGE_PCT)
                    .copied()
                    .filter(|value| value.is_finite())
                    .and_then(|change| {
                        let previous = price - change;
                        if positive(previous) {
                            decimal(previous)
                        } else {
                            None
                        }
                    })
            });
        result.insert(
            identity.symbol.clone(),
            DelayedSnapshotItem {
                symbol: identity.symbol.clone(),
                source: STOCK_SCREEN_SNAPSHOT_SOURCE.to_owned(),
                name: identity.name.clone(),
                last_price,
                previous_close,
                open_price: optional(&row.simple, 2202),
                high_price: optional(&row.simple, 2204),
                low_price: optional(&row.simple, 2205),
                bid_price: optional(&row.simple, 2207),
                ask_price: optional(&row.simple, 2208),
                session: observed_at_ms.map(|observed_at_ms| {
                    crate::quote_session_label(&identity.symbol, observed_at_ms, None)
                }),
            },
        );
    }
    result
}

fn optional(values: &BTreeMap<i32, f64>, key: i32) -> Option<Decimal> {
    values
        .get(&key)
        .copied()
        .filter(|value| value.is_finite())
        .and_then(decimal)
}

fn decimal(value: f64) -> Option<Decimal> {
    value.to_string().parse().ok()
}

fn positive(value: f64) -> bool {
    value.is_finite() && value > 0.0
}

fn parse_symbol(symbol: &str) -> Result<WireSecurity, String> {
    let (market, code) = symbol
        .split_once('.')
        .ok_or_else(|| format!("futu: invalid security symbol {symbol:?}"))?;
    let market =
        market_code(market).ok_or_else(|| format!("futu: invalid security symbol {symbol:?}"))?;
    let code = code.trim().to_ascii_uppercase();
    if code.is_empty() {
        return Err(format!("futu: invalid security symbol {symbol:?}"));
    }
    Ok(WireSecurity { market, code })
}

/// Go's `stockScreenMarketValue`: the delayed protocol uses its own market
/// enumeration, which is *not* the `Qot_Common.QotMarket` one.
pub fn screen_market_value(market: &str) -> Option<i64> {
    match market.trim().to_ascii_uppercase().as_str() {
        "HK" => Some(1),
        "US" => Some(2),
        "CN" | "SH" | "SZ" => Some(3),
        "SG" => Some(4),
        "CA" => Some(5),
        "AU" => Some(6),
        "JP" => Some(7),
        "MY" => Some(8),
        _ => None,
    }
}

/// Reads the delayed snapshots for `symbols` through `fetch`.
///
/// Market values are visited in ascending order and each market is paged at
/// [`STOCK_SCREEN_SNAPSHOT_PAGE_SIZE`], which is the batching Go performs
/// before any physical call.
pub fn fetch_delayed_snapshots<P: SnapshotFallbackFetchPort + ?Sized>(
    port: &P,
    symbols: &[String],
) -> Result<HashMap<String, DelayedSnapshotItem>, SnapshotFallbackError> {
    let identities = port
        .static_info_ids(symbols)
        .map_err(SnapshotFallbackError::Fetch)?;
    let mut groups: BTreeMap<i64, Vec<u64>> = BTreeMap::new();
    for identity in &identities {
        let Some(market) = identity.symbol.split_once('.').map(|(market, _)| market) else {
            continue;
        };
        let Some(value) = screen_market_value(market) else {
            continue;
        };
        groups.entry(value).or_default().push(identity.stock_id);
    }
    let mut result = HashMap::new();
    for (market_value, stock_ids) in groups {
        for page in stock_ids.chunks(STOCK_SCREEN_SNAPSHOT_PAGE_SIZE) {
            let rows = port
                .stock_screen_page(market_value, page)
                .map_err(SnapshotFallbackError::Fetch)?;
            result.extend(project_screen_page_at(
                &rows,
                &identities,
                Some(current_unix_millis()),
            ));
        }
    }
    Ok(result)
}

/// Wall-clock seam for the delayed reads: Go stamps every row with
/// `time.Now().UTC()` so the session label describes the observation, not the
/// request.
fn current_unix_millis() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|elapsed| elapsed.as_millis() as i64)
        .unwrap_or_default()
}

/// Adapter-level delayed fallback: the single owner the watchlist asks for
/// symbols whose BasicQot subscription could not be established.
///
/// Go exposes this as the `broker.SnapshotFallbackSource` capability on the
/// broker adapter, with one 15s coordinator per adapter so several quote
/// surfaces share the delayed read. Kept read-only on purpose: no `Qot_Sub` is
/// ever issued here, which is what the Go-side fixtures assert with
/// `subCallCount() == 0`.
pub struct StockScreenSnapshotFallback {
    reader: Arc<dyn SnapshotFallbackFetchPort>,
    cache: StockScreenSnapshotCoordinator,
    /// The same coordinator the reader issues calls through, kept so
    /// composition and tests can close the physical session deterministically.
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for StockScreenSnapshotFallback {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("StockScreenSnapshotFallback")
            .finish_non_exhaustive()
    }
}

impl StockScreenSnapshotFallback {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self::with_reader(
            Arc::new(OpenDSnapshotFallbackReader::new(Arc::clone(&coordinator))),
            StockScreenSnapshotCoordinator::new(),
            coordinator,
        )
    }

    /// Test/composition seam: swap the physical reader and the delayed cache.
    pub fn with_reader(
        reader: Arc<dyn SnapshotFallbackFetchPort>,
        cache: StockScreenSnapshotCoordinator,
        coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
    ) -> Self {
        Self {
            reader,
            cache,
            coordinator,
        }
    }

    /// The OpenD coordinator backing the delayed reads.
    pub fn reader_coordinator(&self) -> &Arc<Mutex<OpenDSessionCoordinator>> {
        &self.coordinator
    }

    /// Delayed read for `symbols`, in the caller's requested order.
    ///
    /// Mirrors Go's `QuerySnapshotFallback`: at least one symbol is required,
    /// malformed symbols fail the whole request, and a symbol OpenD did not
    /// answer for is simply absent (never synthesized).
    pub fn query(
        &self,
        symbols: &[String],
    ) -> Result<Vec<DelayedSnapshotItem>, SnapshotFallbackError> {
        if symbols.is_empty() {
            return Err(SnapshotFallbackError::EmptySymbols);
        }
        let items = self.cache.query(symbols, |batch| {
            fetch_delayed_snapshots(self.reader.as_ref(), batch)
        })?;
        Ok(symbols
            .iter()
            .map(|symbol| symbol.trim().to_ascii_uppercase())
            .filter_map(|symbol| items.get(&symbol).cloned())
            .collect())
    }
}

#[cfg(test)]
#[path = "snapshot_fallback_tests.rs"]
mod tests;
