use std::sync::mpsc::RecvTimeoutError;
use std::time::Duration;

use jftrade_kernel::WireTimestamp;
use thiserror::Error;

use crate::{
    OpenDInitializedSession, OpenDSessionCloseReason, OpenDSessionEvent,
    OpenDSubscriptionLifecycle, QuotePush, QuotePushDecodeError,
};

/// Result of one bounded read from the managed OpenD event channel.
#[derive(Clone, Debug, PartialEq)]
pub enum OpenDSessionPumpOutcome {
    Idle,
    Push(QuotePush),
    Dropped,
    ReconnectRequired {
        generation: u64,
        reason: OpenDSessionCloseReason,
    },
}

#[derive(Debug, Error)]
pub enum OpenDSessionPumpError {
    #[error(transparent)]
    Push(#[from] QuotePushDecodeError),
    #[error("OpenD managed session event channel disconnected")]
    EventChannelDisconnected,
}

/// Test-composition bridge from the managed session's single reader to the
/// generation-fenced subscription lifecycle.
///
/// Each call consumes at most one event. This type owns no thread, reconnect
/// policy, subscription replay, router mutation or product lifecycle.
#[derive(Clone)]
pub struct OpenDSessionEventPump {
    session: OpenDInitializedSession,
}

impl OpenDSessionEventPump {
    pub fn new(session: OpenDInitializedSession) -> Self {
        Self { session }
    }

    pub fn poll_once(
        &self,
        lifecycle: &OpenDSubscriptionLifecycle,
        now: WireTimestamp,
        timeout: Duration,
    ) -> Result<OpenDSessionPumpOutcome, OpenDSessionPumpError> {
        match self
            .session
            .managed_session()
            .receive_event_timeout(timeout)
        {
            Ok(event) => Self::dispatch_event(lifecycle, &event, now),
            Err(RecvTimeoutError::Timeout) => Ok(OpenDSessionPumpOutcome::Idle),
            Err(RecvTimeoutError::Disconnected) => {
                Err(OpenDSessionPumpError::EventChannelDisconnected)
            }
        }
    }

    fn dispatch_event(
        lifecycle: &OpenDSubscriptionLifecycle,
        event: &OpenDSessionEvent,
        now: WireTimestamp,
    ) -> Result<OpenDSessionPumpOutcome, OpenDSessionPumpError> {
        match event {
            OpenDSessionEvent::UnsolicitedFrame { .. } => lifecycle
                .ingest_session_event(event, now)
                .map(|push| {
                    push.map_or(
                        OpenDSessionPumpOutcome::Dropped,
                        OpenDSessionPumpOutcome::Push,
                    )
                })
                .map_err(Into::into),
            OpenDSessionEvent::Closed { generation, reason } => {
                let reconnect = *reason != OpenDSessionCloseReason::Local
                    && lifecycle.accepts_session_generation(*generation);
                lifecycle.ingest_session_event(event, now)?;
                if reconnect {
                    Ok(OpenDSessionPumpOutcome::ReconnectRequired {
                        generation: *generation,
                        reason: reason.clone(),
                    })
                } else {
                    Ok(OpenDSessionPumpOutcome::Dropped)
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use std::io::{Read, Write};
    use std::net::TcpListener;
    use std::sync::{Arc, mpsc};
    use std::thread;

    use jftrade_marketdata::{InstrumentRef, MarketDataRuntimeRecorder};

    use super::*;
    use crate::transport::read_framed_frame;
    use crate::{
        OpenDTcpProbeConfig, PROTO_INIT_CONNECT, PROTO_UPDATE_BASIC_QOT, decode_frame, encode_frame,
    };

    fn lifecycle(recorder: Arc<MarketDataRuntimeRecorder>) -> OpenDSubscriptionLifecycle {
        let mut lifecycle = OpenDSubscriptionLifecycle::new(recorder, 60_000);
        lifecycle.reconcile_demand(
            &[InstrumentRef {
                channel: "SNAPSHOT".to_owned(),
                market: "US".to_owned(),
                symbol: "AAPL".to_owned(),
                interval: None,
            }],
            0,
        );
        lifecycle
    }

    #[test]
    fn poll_once_is_bounded_and_routes_push_decode_and_peer_close() {
        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
        let address = listener.local_addr().expect("address");
        let (release_sender, release_receiver) = mpsc::channel();
        let server = thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept");
            let init = read_framed_frame(&mut stream).expect("init request");
            assert_eq!(init.header.proto_id, PROTO_INIT_CONNECT);
            let response = encode_frame(
                PROTO_INIT_CONNECT,
                init.header.serial_no,
                &[0x08, 0x00, 0x22, 0x00],
            )
            .expect("init response");
            stream.write_all(&response).expect("write init response");
            release_receiver.recv().expect("release events");
            stream
                .write_all(&encode_frame(9_999, 0, &[]).expect("unknown push"))
                .expect("write unknown push");
            stream
                .write_all(
                    &encode_frame(PROTO_UPDATE_BASIC_QOT, 0, &[0xff]).expect("malformed push"),
                )
                .expect("write malformed push");
        });

        let config = OpenDTcpProbeConfig::new(address, Duration::from_secs(1));
        let session = OpenDInitializedSession::connect_with_push_notifications(&config, 1)
            .expect("initialized session");
        let pump = OpenDSessionEventPump::new(session);
        let recorder = Arc::new(MarketDataRuntimeRecorder::default());
        let lifecycle = lifecycle(Arc::clone(&recorder));
        let now = "2026-08-24T00:00:00Z".parse().expect("timestamp");

        assert_eq!(
            pump.poll_once(&lifecycle, now, Duration::from_millis(10))
                .expect("idle poll"),
            OpenDSessionPumpOutcome::Idle
        );
        release_sender.send(()).expect("release server");
        assert_eq!(
            pump.poll_once(&lifecycle, now, Duration::from_secs(1))
                .expect("unknown push"),
            OpenDSessionPumpOutcome::Dropped
        );
        assert_eq!(
            pump.poll_once(&lifecycle, now, Duration::from_secs(1))
                .expect("malformed push is dropped"),
            OpenDSessionPumpOutcome::Dropped
        );
        assert_eq!(recorder.snapshot().stream_failures, 0);
        assert_eq!(
            pump.poll_once(&lifecycle, now, Duration::from_secs(1))
                .expect("peer close"),
            OpenDSessionPumpOutcome::ReconnectRequired {
                generation: 1,
                reason: OpenDSessionCloseReason::PeerClosed,
            }
        );
        assert_eq!(recorder.snapshot().stream_failures, 1);
        server.join().expect("server thread");
    }

    #[test]
    fn stale_and_local_close_events_are_dropped_without_failure() {
        let recorder = Arc::new(MarketDataRuntimeRecorder::default());
        let mut lifecycle = lifecycle(Arc::clone(&recorder));
        let generation = lifecycle.generation();
        let now = "2026-08-24T00:00:00Z".parse().expect("timestamp");
        let stale = OpenDSessionEvent::UnsolicitedFrame {
            generation: generation + 1,
            frame: decode_frame(
                &encode_frame(PROTO_UPDATE_BASIC_QOT, 0, &[0xff]).expect("malformed frame"),
            )
            .expect("decoded frame"),
        };
        assert_eq!(
            OpenDSessionEventPump::dispatch_event(&lifecycle, &stale, now)
                .expect("stale malformed push"),
            OpenDSessionPumpOutcome::Dropped
        );

        let local = OpenDSessionEvent::Closed {
            generation,
            reason: OpenDSessionCloseReason::Local,
        };
        assert_eq!(
            OpenDSessionEventPump::dispatch_event(&lifecycle, &local, now).expect("local close"),
            OpenDSessionPumpOutcome::Dropped
        );
        assert_eq!(recorder.snapshot().stream_failures, 0);

        lifecycle.close();
        let peer_after_lifecycle_close = OpenDSessionEvent::Closed {
            generation,
            reason: OpenDSessionCloseReason::PeerClosed,
        };
        assert_eq!(
            OpenDSessionEventPump::dispatch_event(&lifecycle, &peer_after_lifecycle_close, now)
                .expect("closed lifecycle"),
            OpenDSessionPumpOutcome::Dropped
        );
        assert_eq!(recorder.snapshot().stream_failures, 0);
    }

    #[test]
    fn lifecycle_first_managed_session_close_does_not_request_reconnect() {
        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
        let address = listener.local_addr().expect("address");
        let server = thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept");
            let init = read_framed_frame(&mut stream).expect("init request");
            let response = encode_frame(
                PROTO_INIT_CONNECT,
                init.header.serial_no,
                &[0x08, 0x00, 0x22, 0x00],
            )
            .expect("init response");
            stream.write_all(&response).expect("write init response");
            let mut byte = [0_u8; 1];
            assert_eq!(stream.read(&mut byte).expect("local shutdown"), 0);
        });

        let config = OpenDTcpProbeConfig::new(address, Duration::from_secs(1));
        let session = OpenDInitializedSession::connect_with_push_notifications(&config, 1)
            .expect("initialized session");
        let pump = OpenDSessionEventPump::new(session.clone());
        let recorder = Arc::new(MarketDataRuntimeRecorder::default());
        let mut lifecycle = lifecycle(Arc::clone(&recorder));
        let now = "2026-08-24T00:00:00Z".parse().expect("timestamp");

        assert!(lifecycle.close());
        assert!(
            session
                .managed_session()
                .close()
                .expect("close managed session")
        );
        assert_eq!(
            pump.poll_once(&lifecycle, now, Duration::from_secs(1))
                .expect("local close event"),
            OpenDSessionPumpOutcome::Dropped
        );
        assert_eq!(recorder.snapshot().stream_failures, 0);
        server.join().expect("server thread");
    }
}
