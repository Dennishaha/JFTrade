use std::sync::Arc;
use std::time::Duration;

use jftrade_kernel::WireTimestamp;
use jftrade_marketdata::{
    InstrumentRef, MarketDataRuntimeRecorder, SnapshotPollExecutor, SnapshotPollOutcome, TickCache,
};
use thiserror::Error;

use crate::{
    OpenDInitializedSession, OpenDManagedSessionError, OpenDSessionCloseReason,
    OpenDSessionEventPump, OpenDSessionPumpError, OpenDSessionPumpOutcome,
    OpenDSubscriptionExecutor, OpenDSubscriptionLifecycle, OpenDTcpProbeConfig, ReconcileAction,
    SubscriptionExecutorError, desired_subscriptions,
};

#[derive(Debug, Error)]
pub enum OpenDSessionCoordinatorError {
    #[error("OpenD session coordinator is closed")]
    Closed,
    #[error("OpenD InitConnect handshake failed: {0}")]
    Handshake(#[from] crate::OpenDTcpProbeError),
    #[error("OpenD managed session failed: {0}")]
    Session(#[from] OpenDManagedSessionError),
    #[error("OpenD subscription replay failed: {0}")]
    Subscription(#[from] SubscriptionExecutorError),
    #[error("OpenD session pump failed: {0}")]
    Pump(#[from] OpenDSessionPumpError),
    #[error("OpenD session coordinator topology changes require production epoch ownership")]
    TopologyChangeUnsupported,
    #[error("OpenD timestamp conversion failed: {0}")]
    Time(#[from] jftrade_kernel::CodecError),
}

#[derive(Clone, Debug, PartialEq)]
pub enum OpenDSessionCoordinatorOutcome {
    Idle,
    Push(crate::QuotePush),
    Dropped,
    Reconnected {
        generation: u64,
        reason: OpenDSessionCloseReason,
    },
}

#[derive(Clone, Debug)]
struct PendingReconnect {
    generation: u64,
    reason: OpenDSessionCloseReason,
    actions: Vec<ReconcileAction>,
}

/// Synchronous composition seam for the managed OpenD session boundary.
///
/// The coordinator owns one authenticated session, lifecycle generation and
/// replay fencing. It deliberately does not own a timer, thread, ProviderRouter
/// activation or default product registration; a composition root must inject
/// it explicitly and drive `poll_once`/`poll_snapshot` from its own lifecycle.
pub struct OpenDSessionCoordinator {
    config: OpenDTcpProbeConfig,
    recorder: Arc<MarketDataRuntimeRecorder>,
    lifecycle: OpenDSubscriptionLifecycle,
    desired: Vec<InstrumentRef>,
    session: Option<OpenDInitializedSession>,
    pending_reconnect: Option<PendingReconnect>,
    closed: bool,
}

impl std::fmt::Debug for OpenDSessionCoordinator {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDSessionCoordinator")
            .field("desired_count", &self.desired.len())
            .field("generation", &self.generation())
            .field("has_session", &self.session.is_some())
            .field("pending_reconnect", &self.pending_reconnect.is_some())
            .field("closed", &self.closed)
            .finish()
    }
}

impl OpenDSessionCoordinator {
    pub fn connect(
        config: OpenDTcpProbeConfig,
        recorder: Arc<MarketDataRuntimeRecorder>,
        desired: Vec<InstrumentRef>,
        now_ms: i64,
    ) -> Result<Self, OpenDSessionCoordinatorError> {
        let mut lifecycle = OpenDSubscriptionLifecycle::new(recorder, 60_000);
        let recorder = lifecycle.recorder();
        let actions = lifecycle.reconcile_demand(&desired, now_ms);
        let generation = lifecycle.generation();
        let session =
            match OpenDInitializedSession::connect_with_push_notifications(&config, generation) {
                Ok(session) => session,
                Err(error) => {
                    lifecycle.close();
                    return Err(error.into());
                }
            };
        let mut coordinator = Self {
            config,
            recorder,
            lifecycle,
            desired,
            session: Some(session),
            pending_reconnect: None,
            closed: false,
        };
        if let Err(error) = coordinator.execute_actions(&actions, now_ms) {
            let _ = coordinator.close();
            return Err(error);
        }
        if !actions.is_empty() {
            assert!(coordinator.lifecycle.stream_connected(generation));
        }
        Ok(coordinator)
    }

    pub fn reconcile(
        &mut self,
        desired: &[InstrumentRef],
        now_ms: i64,
    ) -> Result<(), OpenDSessionCoordinatorError> {
        self.ensure_open()?;
        if desired_subscriptions(&self.desired).physical != desired_subscriptions(desired).physical
        {
            return Err(OpenDSessionCoordinatorError::TopologyChangeUnsupported);
        }
        self.reconcile_topology(desired, now_ms)
    }

    /// Reconciles a changed physical subscription topology. The caller must
    /// be the composition-owned epoch coordinator; this method performs only
    /// the generation-fenced Qot_Sub actions and never activates a provider.
    pub fn reconcile_topology(
        &mut self,
        desired: &[InstrumentRef],
        now_ms: i64,
    ) -> Result<(), OpenDSessionCoordinatorError> {
        self.ensure_open()?;
        self.desired = desired.to_vec();
        let actions = self.lifecycle.reconcile_demand(desired, now_ms);
        self.execute_actions(&actions, now_ms)
    }

    pub fn poll_once(
        &mut self,
        now: WireTimestamp,
        timeout: Duration,
    ) -> Result<OpenDSessionCoordinatorOutcome, OpenDSessionCoordinatorError> {
        self.ensure_open()?;
        if self.pending_reconnect.is_some() {
            let (generation, reason) = self.attempt_reconnect(now_unix_millis(now)?)?;
            return Ok(OpenDSessionCoordinatorOutcome::Reconnected { generation, reason });
        }
        let Some(session) = self.session.as_ref() else {
            return Err(OpenDSessionCoordinatorError::Closed);
        };
        if session.managed_session().is_closed() {
            let generation = session.managed_session().generation();
            let reason = session
                .managed_session()
                .close_reason()?
                .unwrap_or(OpenDSessionCloseReason::PeerClosed);
            self.lifecycle
                .ingest_session_event(
                    &crate::OpenDSessionEvent::Closed {
                        generation,
                        reason: reason.clone(),
                    },
                    now,
                )
                .map_err(OpenDSessionPumpError::from)?;
            if reason == OpenDSessionCloseReason::Local {
                return Ok(OpenDSessionCoordinatorOutcome::Dropped);
            }
            self.begin_reconnect(reason)?;
            let (generation, reason) = self.attempt_reconnect(now_unix_millis(now)?)?;
            return Ok(OpenDSessionCoordinatorOutcome::Reconnected { generation, reason });
        }
        let pump = OpenDSessionEventPump::new(session.clone());
        match pump.poll_once(&self.lifecycle, now, timeout)? {
            OpenDSessionPumpOutcome::ReconnectRequired { reason, .. } => {
                self.begin_reconnect(reason)?;
                let (generation, reason) = self.attempt_reconnect(now_unix_millis(now)?)?;
                Ok(OpenDSessionCoordinatorOutcome::Reconnected { generation, reason })
            }
            OpenDSessionPumpOutcome::Idle => Ok(OpenDSessionCoordinatorOutcome::Idle),
            OpenDSessionPumpOutcome::Push(push) => Ok(OpenDSessionCoordinatorOutcome::Push(push)),
            OpenDSessionPumpOutcome::Dropped => Ok(OpenDSessionCoordinatorOutcome::Dropped),
        }
    }

    pub fn session(&self) -> Result<&OpenDInitializedSession, OpenDSessionCoordinatorError> {
        self.ensure_open()?;
        self.session
            .as_ref()
            .ok_or(OpenDSessionCoordinatorError::Closed)
    }

    pub fn lifecycle(&self) -> &OpenDSubscriptionLifecycle {
        &self.lifecycle
    }

    pub fn generation(&self) -> u64 {
        self.lifecycle.generation()
    }

    pub fn close(&mut self) -> Result<bool, OpenDSessionCoordinatorError> {
        if self.closed {
            return Ok(false);
        }
        self.closed = true;
        self.pending_reconnect = None;
        self.lifecycle.close();
        if let Some(session) = self.session.take() {
            session.managed_session().close()?;
        }
        Ok(true)
    }

    /// Polls BasicQot into a caller-owned generation-fenced cache.
    ///
    /// The caller remains responsible for cadence and invoking `poll_once` to
    /// consume push/close events. This method only composes the already
    /// authenticated session, lifecycle-owned BASIC subscriptions and the
    /// broker-neutral snapshot poll executor.
    pub fn poll_snapshot(
        &mut self,
        cache: &mut TickCache,
        now: WireTimestamp,
    ) -> Result<SnapshotPollOutcome, OpenDSessionCoordinatorError> {
        self.ensure_open()?;
        let demand = basic_snapshot_demand(&self.lifecycle.active_basic_instruments());
        let generation = self.generation();
        let session = self
            .session
            .as_ref()
            .ok_or(OpenDSessionCoordinatorError::Closed)?
            .clone();
        let lifecycle = &self.lifecycle;
        let observed_at_ms = now_unix_millis(now)?;
        Ok(SnapshotPollExecutor::default().execute(
            &self.recorder,
            cache,
            &demand,
            generation,
            now,
            |instruments| {
                crate::OpenDBasicQuoteExecutor::new(session)
                    .query_ticks(lifecycle, instruments, observed_at_ms)
                    .map_err(|error| error.to_string())
            },
        ))
    }

    fn begin_reconnect(
        &mut self,
        reason: OpenDSessionCloseReason,
    ) -> Result<(), OpenDSessionCoordinatorError> {
        self.ensure_open()?;
        if self.pending_reconnect.is_some() {
            return Ok(());
        }
        let actions = self.lifecycle.reconfigure_for_reconnect(&self.desired);
        let generation = self.lifecycle.generation();
        self.pending_reconnect = Some(PendingReconnect {
            generation,
            reason,
            actions,
        });
        if let Some(session) = self.session.take() {
            session.managed_session().close()?;
        }
        Ok(())
    }

    fn attempt_reconnect(
        &mut self,
        now_ms: i64,
    ) -> Result<(u64, OpenDSessionCloseReason), OpenDSessionCoordinatorError> {
        let pending = self
            .pending_reconnect
            .clone()
            .ok_or(OpenDSessionCoordinatorError::Closed)?;
        let session = OpenDInitializedSession::connect_with_push_notifications(
            &self.config,
            pending.generation,
        )?;
        self.session = Some(session);
        if let Err(error) = self.execute_actions(&pending.actions, now_ms) {
            if let Some(session) = self.session.take() {
                let _ = session.managed_session().close();
            }
            return Err(error);
        }
        if !pending.actions.is_empty() {
            assert!(self.lifecycle.stream_connected(pending.generation));
        }
        self.pending_reconnect = None;
        Ok((pending.generation, pending.reason))
    }

    fn execute_actions(
        &mut self,
        actions: &[ReconcileAction],
        now_ms: i64,
    ) -> Result<(), OpenDSessionCoordinatorError> {
        let Some(session) = self.session.as_ref() else {
            return Err(OpenDSessionCoordinatorError::Closed);
        };
        let mut executor = OpenDSubscriptionExecutor::from_session(session.clone());
        let generation = self.lifecycle.generation();
        for action in actions {
            self.lifecycle
                .execute_action(action, now_ms, generation, &mut executor)?;
        }
        Ok(())
    }

    fn ensure_open(&self) -> Result<(), OpenDSessionCoordinatorError> {
        if self.closed {
            Err(OpenDSessionCoordinatorError::Closed)
        } else {
            Ok(())
        }
    }
}

fn now_unix_millis(now: WireTimestamp) -> Result<i64, OpenDSessionCoordinatorError> {
    Ok(now.unix_millis()?)
}

fn basic_snapshot_demand(instruments: &[String]) -> Vec<InstrumentRef> {
    instruments
        .iter()
        .filter_map(|instrument| {
            let (market, symbol) = instrument.split_once('.')?;
            Some(InstrumentRef {
                channel: "SNAPSHOT".to_owned(),
                market: market.to_owned(),
                symbol: symbol.to_owned(),
                interval: None,
            })
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use std::io::{Read, Write};
    use std::net::{TcpListener, TcpStream};
    use std::sync::{Arc, mpsc};
    use std::thread;

    use jftrade_marketdata::MarketDataRuntimeRecorder;
    use prost::Message;

    use super::*;
    use crate::transport::read_framed_frame;
    use crate::{PROTO_INIT_CONNECT, PROTO_QOT_SUB, PROTO_UPDATE_BASIC_QOT, encode_frame};

    #[derive(Clone, PartialEq, Message)]
    struct InitRequest {
        #[prost(message, optional, tag = "1")]
        c2s: Option<InitRequestState>,
    }

    #[derive(Clone, PartialEq, Message)]
    struct InitRequestState {
        #[prost(bool, optional, tag = "3")]
        recv_notify: Option<bool>,
    }

    #[derive(Clone, PartialEq, Message)]
    struct InitResponse {
        #[prost(int32, optional, tag = "1")]
        ret_type: Option<i32>,
        #[prost(message, optional, tag = "4")]
        s2c: Option<InitState>,
    }

    #[derive(Clone, PartialEq, Message)]
    struct InitState {
        #[prost(int32, tag = "1")]
        server_ver: i32,
        #[prost(uint64, tag = "3")]
        conn_id: u64,
    }

    #[derive(Clone, PartialEq, Message)]
    struct SubRequest {
        #[prost(message, optional, tag = "1")]
        c2s: Option<SubState>,
    }

    #[derive(Clone, PartialEq, Message)]
    struct SubState {
        #[prost(int32, repeated, tag = "2")]
        sub_types: Vec<i32>,
        #[prost(bool, optional, tag = "3")]
        subscribe: Option<bool>,
    }

    #[derive(Clone, PartialEq, Message)]
    struct SubResponse {
        #[prost(int32, optional, tag = "1")]
        ret_type: Option<i32>,
    }

    #[derive(Clone, PartialEq, Message)]
    struct BasicQuoteResponse {
        #[prost(int32, optional, tag = "1")]
        ret_type: Option<i32>,
        #[prost(message, optional, tag = "4")]
        s2c: Option<BasicQuoteState>,
    }

    #[derive(Clone, PartialEq, Message)]
    struct BasicQuoteState {
        #[prost(message, repeated, tag = "1")]
        quotes: Vec<BasicQuoteRow>,
    }

    #[derive(Clone, PartialEq, Message)]
    struct BasicQuoteRow {
        #[prost(message, optional, tag = "1")]
        security: Option<Security>,
        #[prost(bool, optional, tag = "2")]
        suspended: Option<bool>,
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
        current_price: Option<f64>,
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

    #[derive(Clone, PartialEq, Message)]
    struct Security {
        #[prost(int32, optional, tag = "1")]
        market: Option<i32>,
        #[prost(string, optional, tag = "2")]
        code: Option<String>,
    }

    fn reference(channel: &str, interval: Option<&str>) -> InstrumentRef {
        InstrumentRef {
            channel: channel.to_owned(),
            market: "US".to_owned(),
            symbol: "AAPL".to_owned(),
            interval: interval.map(str::to_owned),
        }
    }

    fn write_response(stream: &mut TcpStream, protocol: u32, serial: u32, body: Vec<u8>) {
        stream
            .write_all(&encode_frame(protocol, serial, &body).expect("response frame"))
            .expect("write response");
    }

    fn basic_quote_push() -> Vec<u8> {
        BasicQuoteResponse {
            ret_type: Some(0),
            s2c: Some(BasicQuoteState {
                quotes: vec![BasicQuoteRow {
                    security: Some(Security {
                        market: Some(11),
                        code: Some("AAPL".to_owned()),
                    }),
                    suspended: Some(false),
                    list_time: Some("1980-12-12".to_owned()),
                    price_spread: Some(0.01),
                    update_time: Some("2026-08-24 09:30:00".to_owned()),
                    high_price: Some(124.0),
                    open_price: Some(122.0),
                    low_price: Some(121.0),
                    current_price: Some(123.45),
                    last_close_price: Some(120.0),
                    volume: Some(9),
                    turnover: Some(1_111.0),
                    turnover_rate: Some(0.1),
                    amplitude: Some(1.2),
                }],
            }),
        }
        .encode_to_vec()
    }

    #[test]
    fn peer_close_advances_generation_replays_subscriptions_and_accepts_push() {
        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
        let address = listener.local_addr().expect("address");
        let (action_sender, action_receiver) = mpsc::channel();
        let (release_sender, release_receiver) = mpsc::channel();
        let server = thread::spawn(move || {
            for connection in 0..2_u64 {
                let (mut stream, _) = listener.accept().expect("accept");
                let init = read_framed_frame(&mut stream).expect("init request");
                assert_eq!(init.header.proto_id, PROTO_INIT_CONNECT);
                assert_eq!(
                    InitRequest::decode(init.body.as_slice())
                        .expect("decode init request")
                        .c2s
                        .and_then(|state| state.recv_notify),
                    Some(true)
                );
                write_response(
                    &mut stream,
                    PROTO_INIT_CONNECT,
                    init.header.serial_no,
                    InitResponse {
                        ret_type: Some(0),
                        s2c: Some(InitState {
                            server_ver: 1009,
                            conn_id: connection + 1,
                        }),
                    }
                    .encode_to_vec(),
                );
                for _ in 0..2 {
                    let request = read_framed_frame(&mut stream).expect("subscription request");
                    assert_eq!(request.header.proto_id, PROTO_QOT_SUB);
                    let decoded = SubRequest::decode(request.body.as_slice()).expect("sub request");
                    let state = decoded.c2s.expect("sub state");
                    action_sender
                        .send((connection, state.sub_types[0], state.subscribe))
                        .expect("record action");
                    write_response(
                        &mut stream,
                        PROTO_QOT_SUB,
                        request.header.serial_no,
                        SubResponse { ret_type: Some(0) }.encode_to_vec(),
                    );
                }
                if connection == 0 {
                    release_receiver.recv().expect("release first session");
                } else {
                    stream
                        .write_all(
                            &encode_frame(PROTO_UPDATE_BASIC_QOT, 0, &basic_quote_push())
                                .expect("push frame"),
                        )
                        .expect("write push");
                    let mut byte = [0_u8; 1];
                    match stream.read(&mut byte) {
                        Ok(0) => {}
                        Err(error)
                            if matches!(
                                error.kind(),
                                std::io::ErrorKind::ConnectionReset
                                    | std::io::ErrorKind::ConnectionAborted
                                    | std::io::ErrorKind::BrokenPipe
                            ) => {}
                        result => panic!("unexpected client close result: {result:?}"),
                    }
                }
            }
        });

        let recorder = Arc::new(MarketDataRuntimeRecorder::default());
        let desired = vec![reference("KLINE", Some("1m"))];
        let config = OpenDTcpProbeConfig::new(address, Duration::from_secs(1));
        let mut coordinator =
            OpenDSessionCoordinator::connect(config, Arc::clone(&recorder), desired.clone(), 0)
                .expect("coordinator");
        assert_eq!(coordinator.generation(), 1);
        assert!(recorder.snapshot().connected);
        assert_eq!(
            coordinator
                .session()
                .expect("session")
                .managed_session()
                .generation(),
            1
        );
        assert_eq!(
            coordinator.lifecycle().active_basic_instruments(),
            vec!["US.AAPL".to_owned()]
        );
        assert_eq!(
            action_receiver.recv().expect("basic action"),
            (0, 1, Some(true))
        );
        assert_eq!(
            action_receiver.recv().expect("kline action"),
            (0, 11, Some(true))
        );

        coordinator
            .reconcile(&desired, 1)
            .expect("identical reconcile");
        assert!(matches!(
            coordinator.reconcile(&[reference("SNAPSHOT", None)], 2),
            Err(OpenDSessionCoordinatorError::TopologyChangeUnsupported)
        ));
        assert_eq!(coordinator.generation(), 1);
        assert!(matches!(
            action_receiver.recv_timeout(Duration::from_millis(25)),
            Err(mpsc::RecvTimeoutError::Timeout)
        ));

        release_sender.send(()).expect("close first session");
        let now: WireTimestamp = "2026-08-24T00:00:00Z".parse().expect("timestamp");
        assert_eq!(
            coordinator
                .poll_once(now, Duration::from_secs(1))
                .expect("reconnect"),
            OpenDSessionCoordinatorOutcome::Reconnected {
                generation: 2,
                reason: OpenDSessionCloseReason::PeerClosed,
            }
        );
        assert!(recorder.snapshot().connected);
        assert_eq!(
            action_receiver.recv().expect("replay basic"),
            (1, 1, Some(true))
        );
        assert_eq!(
            action_receiver.recv().expect("replay kline"),
            (1, 11, Some(true))
        );
        assert!(matches!(
            coordinator
                .poll_once(now, Duration::from_secs(1))
                .expect("push"),
            OpenDSessionCoordinatorOutcome::Push(crate::QuotePush::Basic(_))
        ));
        assert_eq!(recorder.snapshot().generation, 2);
        assert!(coordinator.close().expect("close"));
        assert!(!coordinator.close().expect("idempotent close"));
        server.join().expect("server");
    }

    #[test]
    fn reconnect_replay_contains_only_subscriptions_and_resets_retry_state() {
        let recorder = Arc::new(MarketDataRuntimeRecorder::default());
        let mut lifecycle = OpenDSubscriptionLifecycle::new(Arc::clone(&recorder), 60_000);
        let desired = [reference("KLINE", Some("1m"))];
        let initial = lifecycle.reconcile_demand(&desired, 0);
        let generation = lifecycle.generation();
        for action in &initial {
            lifecycle.record_subscription_success(action, 123, generation);
        }
        let subscription = match &initial[0] {
            ReconcileAction::Subscribe { subscription } => subscription,
            _ => panic!("expected subscribe action"),
        };
        assert_eq!(
            lifecycle.record_subscription_failure(subscription, 200, generation),
            Some(5_000)
        );
        let replay = lifecycle.reconfigure_for_reconnect(&desired);
        assert_eq!(replay.len(), 2);
        assert!(
            replay
                .iter()
                .all(|action| matches!(action, ReconcileAction::Subscribe { .. }))
        );
        for action in &replay {
            assert!(lifecycle.record_subscription_success(action, 6_000, generation + 1));
        }
        assert!(lifecycle.reconcile_demand(&desired, 6_000).is_empty());
        assert_eq!(lifecycle.generation(), generation + 1);

        let cleared = lifecycle.reconfigure_for_reconnect(&[]);
        assert!(cleared.is_empty());
        assert_eq!(lifecycle.generation(), generation + 2);
        assert_eq!(recorder.snapshot().active_count, 0);
    }

    #[test]
    fn public_coordinator_polls_basic_quotes_into_a_generation_fenced_cache() {
        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
        let address = listener.local_addr().expect("address");
        let server = thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept");
            let init = read_framed_frame(&mut stream).expect("init request");
            assert_eq!(init.header.proto_id, PROTO_INIT_CONNECT);
            write_response(
                &mut stream,
                PROTO_INIT_CONNECT,
                init.header.serial_no,
                InitResponse {
                    ret_type: Some(0),
                    s2c: Some(InitState {
                        server_ver: 1009,
                        conn_id: 1,
                    }),
                }
                .encode_to_vec(),
            );
            let subscribe = read_framed_frame(&mut stream).expect("subscribe request");
            assert_eq!(subscribe.header.proto_id, PROTO_QOT_SUB);
            write_response(
                &mut stream,
                PROTO_QOT_SUB,
                subscribe.header.serial_no,
                SubResponse { ret_type: Some(0) }.encode_to_vec(),
            );
            let quote = read_framed_frame(&mut stream).expect("basic quote request");
            assert_eq!(quote.header.proto_id, crate::PROTO_GET_BASIC_QOT);
            write_response(
                &mut stream,
                crate::PROTO_GET_BASIC_QOT,
                quote.header.serial_no,
                basic_quote_push(),
            );
        });

        let recorder = Arc::new(MarketDataRuntimeRecorder::default());
        let config = OpenDTcpProbeConfig::new(address, Duration::from_secs(1));
        let mut coordinator = OpenDSessionCoordinator::connect(
            config,
            Arc::clone(&recorder),
            vec![reference("SNAPSHOT", None)],
            0,
        )
        .expect("coordinator");
        let mut cache = TickCache::new(2);
        let now: WireTimestamp = "2026-08-24T00:00:00Z".parse().expect("timestamp");
        assert!(matches!(
            coordinator
                .poll_snapshot(&mut cache, now)
                .expect("snapshot poll"),
            SnapshotPollOutcome::Applied {
                requested: 1,
                inserted: 1
            }
        ));
        assert!(matches!(
            cache.lookup_for_generation("US.AAPL", now.unix_millis().expect("millis"), 1_500, 1),
            jftrade_marketdata::CacheLookup::Fresh(_)
        ));
        coordinator.close().expect("close coordinator");
        server.join().expect("server");
    }
}
