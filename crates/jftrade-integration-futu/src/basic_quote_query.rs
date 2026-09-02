use std::collections::BTreeSet;
use std::thread;
use std::time::Duration;

use prost::Message;
use thiserror::Error;

use jftrade_marketdata::Tick;

use crate::quote_push::decode_basic_quote_response;
use crate::subscription_executor::split_instrument;
use crate::{
    BasicQuote, BasicQuoteTickError, OpenDInitializedSession, OpenDManagedSessionError,
    OpenDSubscriptionLifecycle, OpenDTcpProbeError, PROTO_GET_BASIC_QOT, basic_quote_ticks,
};

const BASIC_QUOTE_QUERY_TIMEOUT: Duration = Duration::from_millis(900);
const BASIC_QUOTE_QUERY_ATTEMPTS: usize = 2;
// Go's withRetryingClient invalidates and reconnects immediately; there is no
// provider backoff between the two replay-safe read attempts.
const BASIC_QUOTE_RETRY_BACKOFF: Duration = Duration::ZERO;

#[derive(Clone)]
pub struct OpenDBasicQuoteExecutor {
    session: OpenDInitializedSession,
    query_timeout: Duration,
}

impl OpenDBasicQuoteExecutor {
    pub fn new(session: OpenDInitializedSession) -> Self {
        Self {
            session,
            query_timeout: BASIC_QUOTE_QUERY_TIMEOUT,
        }
    }

    #[cfg(test)]
    fn with_query_timeout(mut self, timeout: Duration) -> Self {
        self.query_timeout = timeout;
        self
    }

    /// Executes one replay-safe BasicQot read for subscriptions already owned by
    /// the active lifecycle. This method never subscribes implicitly.
    pub fn query(
        &self,
        lifecycle: &OpenDSubscriptionLifecycle,
        instruments: &[String],
    ) -> Result<Vec<BasicQuote>, BasicQuoteQueryError> {
        let request_body = prepare_request(
            lifecycle,
            instruments,
            self.session.managed_session().generation(),
        )?;
        query_session(&self.session, &request_body, self.query_timeout)
    }

    /// Replays one BasicQot read once after a recoverable session failure.
    ///
    /// The reconnect callback owns the new authenticated session and must
    /// replay the lifecycle's already-approved subscriptions before returning;
    /// this method never issues an implicit Qot_Sub. The two-attempt limit and
    /// zero backoff match Go's `withRetryingClient` read policy. Each attempt
    /// receives the same 900ms collector deadline (or the test override), and
    /// the generation fence is checked before every request and after every
    /// reconnect.
    pub fn query_with_retry<F>(
        &mut self,
        lifecycle: &OpenDSubscriptionLifecycle,
        instruments: &[String],
        mut reconnect: F,
    ) -> Result<Vec<BasicQuote>, BasicQuoteQueryError>
    where
        F: FnMut() -> Result<OpenDInitializedSession, OpenDTcpProbeError>,
    {
        let mut session = self.session.clone();
        let request_body = prepare_request(
            lifecycle,
            instruments,
            session.managed_session().generation(),
        )?;
        for attempt in 0..BASIC_QUOTE_QUERY_ATTEMPTS {
            ensure_generation(&session, lifecycle)?;
            match query_session(&session, &request_body, self.query_timeout) {
                Ok(quotes) => return Ok(quotes),
                Err(error)
                    if attempt + 1 < BASIC_QUOTE_QUERY_ATTEMPTS
                        && is_recoverable_session_error(&error) =>
                {
                    let _ = session.managed_session().close();
                    if !BASIC_QUOTE_RETRY_BACKOFF.is_zero() {
                        thread::sleep(BASIC_QUOTE_RETRY_BACKOFF);
                    }
                    session = reconnect()?;
                    ensure_generation(&session, lifecycle)?;
                    self.session = session.clone();
                }
                Err(error) => return Err(error),
            }
        }
        unreachable!("BasicQot retry loop always returns within the attempt bound")
    }

    /// Executes one fenced BasicQot query and maps the result into the current
    /// collector model without mutating cache or runtime state.
    pub fn query_ticks(
        &self,
        lifecycle: &OpenDSubscriptionLifecycle,
        instruments: &[String],
        observed_at_ms: i64,
    ) -> Result<Vec<Tick>, BasicQuoteQueryError> {
        let generation = lifecycle.generation();
        let quotes = self.query(lifecycle, instruments)?;
        Ok(basic_quote_ticks(quotes, observed_at_ms, generation)?)
    }
}

#[derive(Debug, Error)]
pub enum BasicQuoteQueryError {
    #[error(transparent)]
    Tick(#[from] BasicQuoteTickError),
    #[error(transparent)]
    Session(#[from] OpenDManagedSessionError),
    #[error("OpenD BasicQot retry reconnect handshake failed: {0}")]
    Handshake(#[from] crate::OpenDTcpProbeError),
    #[error(transparent)]
    Decode(#[from] crate::QuotePushDecodeError),
    #[error("invalid OpenD BasicQot instrument: {0}")]
    InvalidInstrument(String),
    #[error("OpenD BasicQot requires an active BASIC subscription for {0}")]
    SubscriptionRequired(String),
    #[error("OpenD BasicQot request requires at least one instrument")]
    EmptyRequest,
    #[error(
        "OpenD BasicQot generation mismatch: session={session_generation}, lifecycle={lifecycle_generation}"
    )]
    StaleGeneration {
        session_generation: u64,
        lifecycle_generation: u64,
    },
    #[error("OpenD GetBasicQot returned retType={ret_type} errCode={error_code}: {message}")]
    Rejected {
        ret_type: i32,
        error_code: i32,
        message: String,
    },
    #[error("OpenD GetBasicQot response omitted required quote fields")]
    IncompleteResponse,
}

fn prepare_request(
    lifecycle: &OpenDSubscriptionLifecycle,
    instruments: &[String],
    session_generation: u64,
) -> Result<Vec<u8>, BasicQuoteQueryError> {
    let instruments = normalized_instruments(instruments)?;
    if instruments.is_empty() {
        return Err(BasicQuoteQueryError::EmptyRequest);
    }
    ensure_generation_for_lifecycle(lifecycle, session_generation)?;
    let active = lifecycle
        .active_basic_instruments()
        .into_iter()
        .map(|instrument| instrument.trim().to_ascii_uppercase())
        .collect::<BTreeSet<_>>();
    if let Some(instrument) = instruments
        .iter()
        .find(|instrument| !active.contains(*instrument))
    {
        return Err(BasicQuoteQueryError::SubscriptionRequired(
            instrument.clone(),
        ));
    }
    let security_list = instruments
        .iter()
        .map(|instrument| {
            let (market, code) = split_instrument(instrument)
                .map_err(|_| BasicQuoteQueryError::InvalidInstrument(instrument.clone()))?;
            Ok(QotSecurity {
                market: Some(market),
                code: Some(code),
            })
        })
        .collect::<Result<Vec<_>, BasicQuoteQueryError>>()?;
    Ok(BasicQuoteRequest {
        c2s: Some(BasicQuoteC2s { security_list }),
    }
    .encode_to_vec())
}

fn ensure_generation(
    session: &OpenDInitializedSession,
    lifecycle: &OpenDSubscriptionLifecycle,
) -> Result<(), BasicQuoteQueryError> {
    ensure_generation_for_lifecycle(lifecycle, session.managed_session().generation())
}

fn ensure_generation_for_lifecycle(
    lifecycle: &OpenDSubscriptionLifecycle,
    session_generation: u64,
) -> Result<(), BasicQuoteQueryError> {
    let lifecycle_generation = lifecycle.generation();
    if session_generation != lifecycle_generation {
        return Err(BasicQuoteQueryError::StaleGeneration {
            session_generation,
            lifecycle_generation,
        });
    }
    Ok(())
}

fn query_session(
    session: &OpenDInitializedSession,
    request_body: &[u8],
    timeout: Duration,
) -> Result<Vec<BasicQuote>, BasicQuoteQueryError> {
    let body =
        session
            .managed_session()
            .call_with_timeout(PROTO_GET_BASIC_QOT, request_body, timeout)?;
    let response = decode_basic_quote_response(PROTO_GET_BASIC_QOT, &body)?;
    if response.ret_type != 0 {
        return Err(BasicQuoteQueryError::Rejected {
            ret_type: response.ret_type,
            error_code: response.err_code,
            message: response
                .ret_msg
                .unwrap_or_else(|| "OpenD GetBasicQot request failed".to_owned()),
        });
    }
    if !response.s2c_present {
        return Ok(Vec::new());
    }
    response
        .quotes
        .ok_or(BasicQuoteQueryError::IncompleteResponse)
}

fn is_recoverable_session_error(error: &BasicQuoteQueryError) -> bool {
    matches!(
        error,
        BasicQuoteQueryError::Session(OpenDManagedSessionError::Closed(_))
            | BasicQuoteQueryError::Session(OpenDManagedSessionError::Io(_))
            | BasicQuoteQueryError::Session(OpenDManagedSessionError::RequestTimeout { .. })
    )
}

fn normalized_instruments(instruments: &[String]) -> Result<Vec<String>, BasicQuoteQueryError> {
    let mut normalized = Vec::with_capacity(instruments.len());
    for instrument in instruments {
        let instrument = instrument.trim().to_ascii_uppercase();
        split_instrument(&instrument)
            .map_err(|_| BasicQuoteQueryError::InvalidInstrument(instrument.clone()))?;
        normalized.push(instrument);
    }
    normalized.sort();
    normalized.dedup();
    Ok(normalized)
}

#[derive(Clone, PartialEq, Message)]
struct BasicQuoteRequest {
    #[prost(message, optional, tag = "1")]
    c2s: Option<BasicQuoteC2s>,
}

#[derive(Clone, PartialEq, Message)]
struct BasicQuoteC2s {
    #[prost(message, repeated, tag = "1")]
    security_list: Vec<QotSecurity>,
}

#[derive(Clone, PartialEq, Message)]
struct QotSecurity {
    #[prost(int32, optional, tag = "1")]
    market: Option<i32>,
    #[prost(string, optional, tag = "2")]
    code: Option<String>,
}

#[cfg(test)]
mod tests {
    use std::io::Write;
    use std::net::{TcpListener, TcpStream};
    use std::sync::{Arc, mpsc};
    use std::thread;
    use std::time::Duration;

    use jftrade_marketdata::{
        InstrumentRef, MarketDataRuntimeRecorder, SnapshotPollExecutor, SnapshotPollOutcome,
        SnapshotPollSkipReason, TickCache,
    };

    use super::*;
    use crate::transport::read_framed_frame;
    use crate::{Frame, OpenDTcpProbeConfig, ReconcileAction, SubscriptionKind, encode_frame};

    #[derive(Clone, PartialEq, Message)]
    struct Response {
        #[prost(int32, optional, tag = "1")]
        ret_type: Option<i32>,
        #[prost(string, optional, tag = "2")]
        ret_msg: Option<String>,
        #[prost(int32, optional, tag = "3")]
        err_code: Option<i32>,
        #[prost(message, optional, tag = "4")]
        s2c: Option<ResponseS2c>,
    }

    #[derive(Clone, PartialEq, Message)]
    struct ResponseS2c {
        #[prost(message, repeated, tag = "1")]
        quotes: Vec<WireQuote>,
    }

    #[derive(Clone, PartialEq, Message)]
    struct WireQuote {
        #[prost(message, optional, tag = "1")]
        security: Option<QotSecurity>,
        #[prost(bool, optional, tag = "2")]
        is_suspended: Option<bool>,
        #[prost(string, optional, tag = "3")]
        list_time: Option<String>,
        #[prost(double, optional, tag = "4")]
        price_spread: Option<f64>,
        #[prost(string, optional, tag = "5")]
        update_time: Option<String>,
        #[prost(double, optional, tag = "6")]
        high_price: Option<f64>,
        #[prost(double, optional, tag = "7")]
        open_price: Option<f64>,
        #[prost(double, optional, tag = "8")]
        low_price: Option<f64>,
        #[prost(double, optional, tag = "9")]
        cur_price: Option<f64>,
        #[prost(double, optional, tag = "10")]
        last_close_price: Option<f64>,
        #[prost(int64, optional, tag = "11")]
        volume: Option<i64>,
        #[prost(double, optional, tag = "12")]
        turnover: Option<f64>,
        #[prost(double, optional, tag = "13")]
        turnover_rate: Option<f64>,
        #[prost(double, optional, tag = "14")]
        amplitude: Option<f64>,
    }

    fn lifecycle() -> OpenDSubscriptionLifecycle {
        lifecycle_with_recorder().0
    }

    fn lifecycle_with_recorder() -> (OpenDSubscriptionLifecycle, Arc<MarketDataRuntimeRecorder>) {
        let recorder = Arc::new(MarketDataRuntimeRecorder::default());
        let mut lifecycle = OpenDSubscriptionLifecycle::new(Arc::clone(&recorder), 60_000);
        let actions = lifecycle.reconcile_demand(
            &[InstrumentRef {
                channel: "SNAPSHOT".to_owned(),
                market: "US".to_owned(),
                symbol: "AAPL".to_owned(),
                interval: None,
            }],
            0,
        );
        let generation = lifecycle.generation();
        let action = actions
            .iter()
            .find(|action| matches!(action, ReconcileAction::Subscribe { subscription } if subscription.kind == SubscriptionKind::Basic))
            .expect("basic subscription");
        assert!(lifecycle.record_subscription_success(action, 0, generation));
        (lifecycle, recorder)
    }

    fn quote() -> WireQuote {
        WireQuote {
            security: Some(QotSecurity {
                market: Some(11),
                code: Some("AAPL".to_owned()),
            }),
            is_suspended: Some(false),
            list_time: Some("1980-12-12".to_owned()),
            price_spread: Some(0.01),
            update_time: Some("2026-08-24 09:30:00".to_owned()),
            high_price: Some(190.0),
            open_price: Some(188.0),
            low_price: Some(187.0),
            cur_price: Some(189.5),
            last_close_price: Some(187.5),
            volume: Some(10),
            turnover: Some(1_895.0),
            turnover_rate: Some(0.2),
            amplitude: Some(1.6),
        }
    }

    #[test]
    fn basic_quote_query_requires_subscription_and_maps_success_rejection_and_empty() {
        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
        let address = listener.local_addr().expect("address");
        let server = thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept");
            let init = read_framed_frame(&mut stream).expect("init request");
            respond(&mut stream, &init, vec![0x08, 0x00, 0x22, 0x00]);

            let success = read_framed_frame(&mut stream).expect("success request");
            assert_request(&success);
            respond(
                &mut stream,
                &success,
                Response {
                    ret_type: Some(0),
                    ret_msg: None,
                    err_code: None,
                    s2c: Some(ResponseS2c {
                        quotes: vec![quote()],
                    }),
                }
                .encode_to_vec(),
            );

            let rejected = read_framed_frame(&mut stream).expect("rejected request");
            respond(
                &mut stream,
                &rejected,
                Response {
                    ret_type: Some(-3),
                    ret_msg: Some("quote denied".to_owned()),
                    err_code: Some(1001),
                    s2c: None,
                }
                .encode_to_vec(),
            );

            let empty = read_framed_frame(&mut stream).expect("empty request");
            respond(
                &mut stream,
                &empty,
                Response {
                    ret_type: Some(0),
                    ret_msg: None,
                    err_code: None,
                    s2c: Some(ResponseS2c { quotes: Vec::new() }),
                }
                .encode_to_vec(),
            );

            let missing = read_framed_frame(&mut stream).expect("missing s2c request");
            respond(
                &mut stream,
                &missing,
                Response {
                    ret_type: Some(0),
                    ret_msg: None,
                    err_code: None,
                    s2c: None,
                }
                .encode_to_vec(),
            );

            let incomplete = read_framed_frame(&mut stream).expect("incomplete request");
            let mut incomplete_quote = quote();
            incomplete_quote.security = None;
            respond(
                &mut stream,
                &incomplete,
                Response {
                    ret_type: Some(0),
                    ret_msg: None,
                    err_code: None,
                    s2c: Some(ResponseS2c {
                        quotes: vec![incomplete_quote],
                    }),
                }
                .encode_to_vec(),
            );
        });

        let config = OpenDTcpProbeConfig::new(address, Duration::from_secs(1));
        let session =
            OpenDInitializedSession::connect_with_push_notifications(&config, 1).expect("session");
        let executor = OpenDBasicQuoteExecutor::new(session);
        let mut lifecycle = lifecycle();
        let requested = vec![" us.aapl ".to_owned(), "US.AAPL".to_owned()];
        let ticks = executor
            .query_ticks(&lifecycle, &requested, 1_724_464_001_250)
            .expect("ticks");
        assert_eq!(ticks.len(), 1);
        assert_eq!(ticks[0].instrument_id, "US.AAPL");
        assert_eq!(ticks[0].price.to_string(), "189.5");
        assert_eq!(ticks[0].volume.to_string(), "10");
        assert_eq!(ticks[0].observed_at_ms, 1_724_464_001_250);
        assert_eq!(ticks[0].provider_generation, 1);

        assert!(matches!(
            executor.query(&lifecycle, &["US.AAPL".to_owned()]),
            Err(BasicQuoteQueryError::Rejected {
                ret_type: -3,
                error_code: 1001,
                message,
            }) if message == "quote denied"
        ));
        assert_eq!(
            executor
                .query(&lifecycle, &["US.AAPL".to_owned()])
                .expect("empty list matches Go"),
            Vec::new()
        );
        assert_eq!(
            executor
                .query(&lifecycle, &["US.AAPL".to_owned()])
                .expect("missing s2c matches Go"),
            Vec::new()
        );
        assert!(matches!(
            executor.query(&lifecycle, &["US.AAPL".to_owned()]),
            Err(BasicQuoteQueryError::IncompleteResponse)
        ));
        assert!(matches!(
            executor.query(&lifecycle, &["US.MSFT".to_owned()]),
            Err(BasicQuoteQueryError::SubscriptionRequired(instrument))
                if instrument == "US.MSFT"
        ));
        lifecycle.reconfigure();
        assert!(matches!(
            executor.query(&lifecycle, &["US.AAPL".to_owned()]),
            Err(BasicQuoteQueryError::StaleGeneration {
                session_generation: 1,
                lifecycle_generation: 2,
            })
        ));
        server.join().expect("server thread");
    }

    #[test]
    fn basic_quote_query_uses_the_collector_900ms_deadline_boundary() {
        assert_eq!(BASIC_QUOTE_QUERY_TIMEOUT, Duration::from_millis(900));

        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
        let address = listener.local_addr().expect("address");
        let server = thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept");
            let init = read_framed_frame(&mut stream).expect("init request");
            respond(&mut stream, &init, vec![0x08, 0x00, 0x22, 0x00]);
            let query = read_framed_frame(&mut stream).expect("query request");
            thread::sleep(Duration::from_millis(30));
            respond(
                &mut stream,
                &query,
                Response {
                    ret_type: Some(0),
                    ret_msg: None,
                    err_code: None,
                    s2c: Some(ResponseS2c {
                        quotes: vec![quote()],
                    }),
                }
                .encode_to_vec(),
            );
        });

        let config = OpenDTcpProbeConfig::new(address, Duration::from_secs(1));
        let session =
            OpenDInitializedSession::connect_with_push_notifications(&config, 1).expect("session");
        let executor =
            OpenDBasicQuoteExecutor::new(session).with_query_timeout(Duration::from_millis(10));
        assert!(matches!(
            executor.query(&lifecycle(), &["US.AAPL".to_owned()]),
            Err(BasicQuoteQueryError::Session(
                OpenDManagedSessionError::RequestTimeout {
                    protocol: PROTO_GET_BASIC_QOT,
                    ..
                }
            ))
        ));
        server.join().expect("server thread");
    }

    #[test]
    fn basic_quote_query_replays_once_after_recoverable_session_timeout() {
        assert_eq!(BASIC_QUOTE_QUERY_ATTEMPTS, 2);
        assert!(BASIC_QUOTE_RETRY_BACKOFF.is_zero());

        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
        let address = listener.local_addr().expect("address");
        let (release_sender, release_receiver) = mpsc::channel();
        let server = thread::spawn(move || {
            for attempt in 0..2 {
                let (mut stream, _) = listener.accept().expect("accept session");
                let init = read_framed_frame(&mut stream).expect("init request");
                respond(&mut stream, &init, vec![0x08, 0x00, 0x22, 0x00]);
                let query = read_framed_frame(&mut stream).expect("query request");
                assert_request(&query);
                if attempt == 1 {
                    respond(
                        &mut stream,
                        &query,
                        Response {
                            ret_type: Some(0),
                            ret_msg: None,
                            err_code: None,
                            s2c: Some(ResponseS2c {
                                quotes: vec![quote()],
                            }),
                        }
                        .encode_to_vec(),
                    );
                    release_receiver.recv().expect("release second session");
                } else {
                    // The first call intentionally times out. The executor
                    // closes this session before asking its reconnect owner
                    // for a fresh authenticated session.
                    thread::sleep(Duration::from_millis(40));
                }
            }
        });

        let config = OpenDTcpProbeConfig::new(address, Duration::from_secs(1));
        let session = OpenDInitializedSession::connect_with_push_notifications(&config, 1)
            .expect("initial session");
        let mut executor =
            OpenDBasicQuoteExecutor::new(session).with_query_timeout(Duration::from_millis(10));
        let lifecycle = lifecycle();
        let quotes = executor
            .query_with_retry(&lifecycle, &["US.AAPL".to_owned()], || {
                OpenDInitializedSession::connect_with_push_notifications(&config, 1)
            })
            .expect("replayed BasicQot query");
        assert_eq!(quotes.len(), 1);
        assert_eq!(quotes[0].cur_price, Some(189.5));
        let replay_session_is_open = !executor.session.managed_session().is_closed();
        release_sender.send(()).expect("release second session");
        assert!(
            replay_session_is_open,
            "successful replay must replace the closed session for the next read"
        );
        server.join().expect("server thread");
    }

    #[test]
    fn basic_quote_query_feeds_snapshot_poll_cache_with_generation_fencing() {
        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
        let address = listener.local_addr().expect("address");
        let server = thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept");
            let init = read_framed_frame(&mut stream).expect("init request");
            respond(&mut stream, &init, vec![0x08, 0x00, 0x22, 0x00]);
            let query = read_framed_frame(&mut stream).expect("query request");
            assert_request(&query);
            respond(
                &mut stream,
                &query,
                Response {
                    ret_type: Some(0),
                    ret_msg: None,
                    err_code: None,
                    s2c: Some(ResponseS2c {
                        quotes: vec![quote()],
                    }),
                }
                .encode_to_vec(),
            );
        });

        let config = OpenDTcpProbeConfig::new(address, Duration::from_secs(1));
        let session =
            OpenDInitializedSession::connect_with_push_notifications(&config, 1).expect("session");
        let executor = OpenDBasicQuoteExecutor::new(session);
        let (lifecycle, recorder) = lifecycle_with_recorder();
        let demand = vec![InstrumentRef {
            channel: "SNAPSHOT".to_owned(),
            market: "US".to_owned(),
            symbol: "AAPL".to_owned(),
            interval: None,
        }];
        let now: jftrade_kernel::WireTimestamp =
            "2026-08-24T09:30:01Z".parse().expect("poll timestamp");
        let offset = now.into_inner();
        let observed_at_ms =
            offset.unix_timestamp().saturating_mul(1_000) + i64::from(offset.millisecond());
        let generation = recorder.snapshot().generation;
        let mut cache = TickCache::new(2);

        let outcome = SnapshotPollExecutor::default().execute(
            &recorder,
            &mut cache,
            &demand,
            generation,
            now,
            |instruments| {
                executor
                    .query_ticks(&lifecycle, instruments, observed_at_ms)
                    .map_err(|error| error.to_string())
            },
        );
        assert_eq!(
            outcome,
            SnapshotPollOutcome::Applied {
                requested: 1,
                inserted: 1,
            }
        );
        assert_eq!(cache.instrument_count(), 1);
        let tick = cache
            .require_fresh("US.AAPL", observed_at_ms, 1_500)
            .expect("fresh tick");
        assert_eq!(tick.price.to_string(), "189.5");
        let rich = tick.snapshot.as_ref().expect("basic quote metadata");
        assert_eq!(rich.open_price.expect("open").to_string(), "188");
        assert_eq!(rich.previous_close.expect("close").to_string(), "187.5");
        assert_eq!(rich.update_time.as_deref(), Some("2026-08-24 09:30:00"));

        let fresh = SnapshotPollExecutor::default().execute(
            &recorder,
            &mut cache,
            &demand,
            generation,
            now,
            |_| panic!("fresh cache must not query OpenD"),
        );
        assert_eq!(
            fresh,
            SnapshotPollOutcome::Skipped(SnapshotPollSkipReason::Fresh)
        );

        let next_generation = recorder.reconfigure();
        assert_eq!(next_generation, generation + 1);
        let stale = SnapshotPollExecutor::default().execute(
            &recorder,
            &mut cache,
            &demand,
            generation,
            now,
            |_| panic!("stale generation must not query OpenD"),
        );
        assert_eq!(
            stale,
            SnapshotPollOutcome::Skipped(SnapshotPollSkipReason::InactiveGeneration)
        );
        server.join().expect("server thread");
    }

    fn assert_request(frame: &Frame) {
        assert_eq!(frame.header.proto_id, PROTO_GET_BASIC_QOT);
        let request = BasicQuoteRequest::decode(frame.body.as_slice()).expect("request body");
        let securities = request.c2s.expect("c2s").security_list;
        assert_eq!(securities.len(), 1);
        assert_eq!(securities[0].market, Some(11));
        assert_eq!(securities[0].code.as_deref(), Some("AAPL"));
    }

    fn respond(stream: &mut TcpStream, request: &Frame, body: Vec<u8>) {
        let packet = encode_frame(request.header.proto_id, request.header.serial_no, &body)
            .expect("response");
        stream.write_all(&packet).expect("write response");
    }
}
