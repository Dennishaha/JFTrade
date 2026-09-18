//! HTTP transport for the official calendar providers.
//!
//! Go's `HTTPCalendarSource` fetches a provider document, runs the provider
//! parser, applies the provider validator and returns a snapshot with checksum
//! and validity metadata. The Rust adapter keeps the same shape behind the
//! domain's [`CalendarSourcePort`], so the manager stays transport-agnostic.

use std::sync::Arc;
use std::time::Duration as StdDuration;

use jftrade_calendar::{
    CalendarCancellationToken, CalendarSnapshot, CalendarSourceDescriptor, CalendarSourceError,
    CalendarSourcePort, CalendarSourceRegistry,
};
use jftrade_kernel::WireTimestamp;
use sha2::{Digest, Sha256};
use time::OffsetDateTime;

use crate::parser::{
    ParseFn, ValidateFn, hong_kong_holiday_ical_parser, minimum_anchor_year_schedules_validator,
    nyse_holiday_schedule_parser, sse_trading_schedule_parser,
};

/// Default request timeout, matching Go's `defaultHTTPTimeout`.
pub const DEFAULT_HTTP_TIMEOUT: StdDuration = StdDuration::from_secs(15);

/// Transport seam so tests can drive the adapter without network access.
pub trait CalendarHttpClient: Send + Sync {
    /// Perform a `GET` and return the response body, or a transport error.
    fn get(&self, url: &str, cancellation: &CalendarCancellationToken) -> Result<Vec<u8>, String>;
}

/// Production client for the official providers.
///
/// Each request runs on its own short-lived OS thread. The calendar port
/// (`CalendarSourcePort::fetch`) is synchronous and is invoked from both the
/// manager's refresh worker and the API's async handlers, so the request must
/// not depend on an ambient runtime:
///
/// * `reqwest`'s blocking client owns an internal runtime, and dropping that
///   runtime from inside an async context panics. Creating and dropping the
///   client entirely inside the request thread keeps that lifecycle off the
///   caller's executor.
/// * Calendar providers are fetched rarely (a handful of providers per refresh
///   interval, plus explicit operator refreshes), so a thread per request is a
///   bounded cost rather than a hot path.
pub struct ReqwestCalendarClient {
    timeout: StdDuration,
}

impl ReqwestCalendarClient {
    pub fn new(timeout: StdDuration) -> Result<Self, CalendarSourceError> {
        if timeout.is_zero() {
            return Err(CalendarSourceError::Failed(
                "calendar http timeout must be positive".to_owned(),
            ));
        }
        Ok(Self { timeout })
    }

    /// Build the production client with the same default timeout Go uses.
    pub fn with_default_timeout() -> Result<Self, CalendarSourceError> {
        Self::new(DEFAULT_HTTP_TIMEOUT)
    }

    fn request(&self, url: &str) -> Result<Vec<u8>, String> {
        let url = url.to_owned();
        let timeout = self.timeout;
        // `join` is used rather than detaching so a provider error always
        // reaches the caller and no request outlives the fetch that made it.
        std::thread::Builder::new()
            .name("jftrade-calendar-http".to_owned())
            .spawn(move || fetch_on_thread(&url, timeout))
            .map_err(|error| format!("calendar http worker: {error}"))?
            .join()
            .map_err(|_| "calendar http worker panicked".to_owned())?
    }
}

/// Install the process-wide rustls crypto provider before building a client.
///
/// `reqwest` is pinned to `rustls-no-provider`, so the first client built in the
/// process must install a provider. This mirrors
/// `jftrade-integration-marketdata-helper::install_rustls_provider` and is
/// idempotent, which matters because the engine wires both adapters into one
/// process.
fn install_rustls_provider() {
    let _ = rustls::crypto::ring::default_provider().install_default();
}

/// Fetch one provider document on a dedicated thread.
fn fetch_on_thread(url: &str, timeout: StdDuration) -> Result<Vec<u8>, String> {
    install_rustls_provider();
    let client = reqwest::blocking::Client::builder()
        .timeout(timeout)
        .build()
        .map_err(|error| error.to_string())?;
    let mut response = client.get(url).send().map_err(|error| error.to_string())?;
    let status = response.status();
    if !status.is_success() {
        return Err(format!("request returned status {}", status.as_u16()));
    }
    let mut body = Vec::new();
    std::io::Read::read_to_end(&mut response, &mut body).map_err(|error| error.to_string())?;
    Ok(body)
}

impl CalendarHttpClient for ReqwestCalendarClient {
    fn get(&self, url: &str, cancellation: &CalendarCancellationToken) -> Result<Vec<u8>, String> {
        if cancellation.is_cancelled() {
            return Err("calendar request was cancelled".to_owned());
        }
        let body = self.request(url)?;
        if cancellation.is_cancelled() {
            return Err("calendar request was cancelled".to_owned());
        }
        Ok(body)
    }
}

/// One official provider: transport identity, document parser and validator.
pub struct HttpCalendarSource {
    descriptor: CalendarSourceDescriptor,
    url: String,
    parse: ParseFn,
    validate: Option<ValidateFn>,
    valid_for: time::Duration,
    client: Arc<dyn CalendarHttpClient>,
    clock: Arc<dyn Fn() -> OffsetDateTime + Send + Sync>,
}

impl HttpCalendarSource {
    pub fn new(
        descriptor: CalendarSourceDescriptor,
        url: impl Into<String>,
        parse: ParseFn,
        valid_for: time::Duration,
        client: Arc<dyn CalendarHttpClient>,
    ) -> Self {
        Self {
            descriptor,
            url: url.into(),
            parse,
            validate: None,
            valid_for,
            client,
            clock: Arc::new(OffsetDateTime::now_utc),
        }
    }

    #[must_use]
    pub fn with_validate(mut self, validate: ValidateFn) -> Self {
        self.validate = Some(validate);
        self
    }

    #[must_use]
    pub fn with_clock(mut self, clock: Arc<dyn Fn() -> OffsetDateTime + Send + Sync>) -> Self {
        self.clock = clock;
        self
    }

    pub fn url(&self) -> &str {
        &self.url
    }

    /// Fetch, parse and validate one provider document.
    pub fn fetch_snapshot(
        &self,
        market: &str,
        from: WireTimestamp,
        to: WireTimestamp,
        cancellation: &CalendarCancellationToken,
    ) -> Result<CalendarSnapshot, CalendarSourceError> {
        let body = self
            .client
            .get(&self.url, cancellation)
            .map_err(classify_transport_error)?;
        let schedules = (self.parse)(market, &body, Some(from), Some(to))?;
        if let Some(validate) = self.validate {
            validate(market, &schedules, Some(from), Some(to))?;
        }
        let checksum = hex_digest(&body);
        let fetched_at = (self.clock)().to_offset(time::UtcOffset::UTC);
        Ok(crate::parser::build_snapshot(
            crate::parser::SnapshotParts {
                source_id: &self.descriptor.id,
                market,
                from,
                to,
                schedules,
                checksum: &checksum,
                fetched_at,
                valid_for: self.valid_for,
            },
        ))
    }
}

impl CalendarSourcePort for HttpCalendarSource {
    fn descriptor(&self) -> CalendarSourceDescriptor {
        self.descriptor.clone()
    }

    fn fetch(
        &self,
        market: &str,
        from: WireTimestamp,
        to: WireTimestamp,
        cancellation: &CalendarCancellationToken,
    ) -> Result<CalendarSnapshot, CalendarSourceError> {
        self.fetch_snapshot(market, from, to, cancellation)
    }
}

/// Map a transport failure onto the domain error taxonomy.
///
/// A cancelled request must stay recognisable so the manager can report it as a
/// cancellation rather than a provider outage; everything else is a failure.
fn classify_transport_error(message: String) -> CalendarSourceError {
    let lower = message.to_lowercase();
    if lower.contains("cancel") {
        CalendarSourceError::Cancelled
    } else {
        CalendarSourceError::Failed(message)
    }
}

fn hex_digest(body: &[u8]) -> String {
    let mut hasher = Sha256::new();
    hasher.update(body);
    hasher
        .finalize()
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect()
}

/// Descriptor list for the four official providers.
pub fn default_sources(client: Arc<dyn CalendarHttpClient>) -> Vec<HttpCalendarSource> {
    let verifying = minimum_anchor_year_schedules_validator(8);
    vec![
        HttpCalendarSource::new(
            descriptor("nyse_official", "official_html", "NYSE", &["US"]),
            "https://www.nyse.com/trade/hours-calendars",
            nyse_holiday_schedule_parser,
            time::Duration::days(14),
            Arc::clone(&client),
        )
        .with_validate(verifying),
        HttpCalendarSource::new(
            descriptor("nasdaq_verifier", "official_html", "Nasdaq", &["US"]),
            "https://www.nasdaq.com/market-activity/stock-market-holiday-schedule",
            crate::parser::default_holiday_override_parser,
            time::Duration::days(14),
            Arc::clone(&client),
        )
        .with_validate(verifying),
        HttpCalendarSource::new(
            descriptor("hk_gov_1823_ical", "official_ical", "GovHK 1823", &["HK"]),
            "https://www.1823.gov.hk/common/ical/en.ics",
            hong_kong_holiday_ical_parser,
            time::Duration::days(30),
            Arc::clone(&client),
        )
        .with_validate(verifying),
        HttpCalendarSource::new(
            descriptor(
                "mainland_official_notice",
                "official_html",
                "Shanghai Stock Exchange",
                &["CN", "SH", "SZ"],
            ),
            "https://english.sse.com.cn/start/trading/schedule/",
            sse_trading_schedule_parser,
            time::Duration::days(30),
            Arc::clone(&client),
        )
        .with_validate(verifying),
    ]
}

/// Build the production registry the engine injects into the manager.
pub fn default_registry(
    client: Arc<dyn CalendarHttpClient>,
) -> Result<CalendarSourceRegistry, CalendarSourceError> {
    let mut registry = CalendarSourceRegistry::default();
    for source in default_sources(client) {
        registry
            .register(Arc::new(source))
            .map_err(|error| CalendarSourceError::Failed(error.to_string()))?;
    }
    Ok(registry)
}

fn descriptor(id: &str, kind: &str, authority: &str, markets: &[&str]) -> CalendarSourceDescriptor {
    CalendarSourceDescriptor {
        id: id.to_owned(),
        kind: kind.to_owned(),
        authority: authority.to_owned(),
        markets: markets.iter().map(|market| (*market).to_owned()).collect(),
    }
}
