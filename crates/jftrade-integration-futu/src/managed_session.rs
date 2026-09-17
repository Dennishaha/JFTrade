use std::collections::BTreeMap;
use std::io::{self, Write};
use std::net::{Shutdown, SocketAddr, TcpStream};
use std::sync::Condvar;
use std::sync::atomic::{AtomicBool, AtomicU32, Ordering};
use std::sync::mpsc::{self, Receiver, RecvTimeoutError, SyncSender};
use std::sync::{Arc, Mutex, MutexGuard};
use std::thread::{self, JoinHandle};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use prost::Message;
use thiserror::Error;

use crate::trade_proto::keep_alive::{
    C2s as KeepAliveC2s, Request as KeepAliveRequest, Response as KeepAliveResponse,
};
use crate::transport::{TcpTransportError, read_framed_frame};
use crate::{Frame, FrameError, PROTO_KEEP_ALIVE, encode_frame};
use jftrade_kernel::{OpenDCallRecord, SharedOpenDCallObserver};

#[derive(Clone, Debug, Eq, Error, PartialEq)]
pub enum OpenDSessionCloseReason {
    #[error("closed locally")]
    Local,
    #[error("OpenD peer closed the TCP session")]
    PeerClosed,
    #[error("OpenD TCP session failed: {0}")]
    Transport(String),
    #[error("OpenD sent an invalid frame: {0}")]
    InvalidFrame(FrameError),
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub enum OpenDSessionEvent {
    UnsolicitedFrame {
        generation: u64,
        frame: Frame,
    },
    Closed {
        generation: u64,
        reason: OpenDSessionCloseReason,
    },
}

#[derive(Debug, Error)]
pub enum OpenDManagedSessionError {
    #[error(transparent)]
    Frame(#[from] FrameError),
    #[error("OpenD managed session I/O failed: {0}")]
    Io(#[source] io::Error),
    #[error("OpenD managed session worker failed to start: {0}")]
    WorkerStart(#[source] io::Error),
    #[error("OpenD managed session worker panicked")]
    WorkerPanicked,
    #[error("OpenD managed session state is unavailable")]
    StateUnavailable,
    #[error("OpenD managed session is closed: {0}")]
    Closed(OpenDSessionCloseReason),
    #[error("OpenD request proto {protocol} serial {serial} timed out")]
    RequestTimeout { protocol: u32, serial: u32 },
}

struct PendingCall {
    protocol: u32,
    response: SyncSender<Result<Frame, OpenDSessionCloseReason>>,
}

struct SessionState {
    generation: u64,
    writer: Mutex<TcpStream>,
    pending: Mutex<BTreeMap<u32, PendingCall>>,
    events: mpsc::Sender<OpenDSessionEvent>,
    closed: AtomicBool,
    close_reason: Mutex<Option<OpenDSessionCloseReason>>,
    closed_lock: Mutex<()>,
    closed_signal: Condvar,
    next_serial: AtomicU32,
}

/// Explicit-composition OpenD session with one socket reader.
///
/// The reader routes exact protocol/serial responses to pending RPC calls and
/// emits every other frame as a generation-tagged unsolicited event. It owns
/// no reconnect policy, provider state or product lifecycle and is not wired
/// into the default Rust product composition.
pub struct OpenDManagedSession {
    state: Arc<SessionState>,
    request_timeout: Duration,
    events: Mutex<Receiver<OpenDSessionEvent>>,
    worker: Mutex<Option<JoinHandle<()>>>,
    keep_alive_worker: Mutex<Option<JoinHandle<()>>>,
    keep_alive_started: AtomicBool,
    observer: Option<SharedOpenDCallObserver>,
    request_id: String,
}

impl OpenDManagedSession {
    pub fn connect(
        address: SocketAddr,
        timeout: Duration,
        generation: u64,
    ) -> Result<Self, OpenDManagedSessionError> {
        let stream =
            TcpStream::connect_timeout(&address, timeout).map_err(OpenDManagedSessionError::Io)?;
        Self::from_stream(stream, timeout, generation)
    }

    /// Attaches the composition-root observability sink used to correlate
    /// failed OpenD RPCs, mirroring Go `Client.Call` calling
    /// `observability.RecordOpenDCall` with the surrounding request ID.
    pub fn with_observer(mut self, observer: SharedOpenDCallObserver) -> Self {
        self.observer = Some(observer);
        self
    }

    /// Attaches an optional observer; `None` keeps the transport silent.
    pub fn with_observer_opt(mut self, observer: Option<SharedOpenDCallObserver>) -> Self {
        self.observer = observer;
        self
    }

    /// Sets the request correlation ID reported with every observed call.
    ///
    /// Go reads this from the caller's context (`observability.FieldsFromContext`).
    /// Rust's OpenD port traits do not carry a request context, so a
    /// request-scoped session sets it explicitly; long-lived background
    /// sessions leave it empty.
    pub fn with_request_id(mut self, request_id: impl Into<String>) -> Self {
        self.request_id = request_id.into();
        self
    }

    pub fn from_stream(
        stream: TcpStream,
        timeout: Duration,
        generation: u64,
    ) -> Result<Self, OpenDManagedSessionError> {
        stream
            .set_read_timeout(None)
            .map_err(OpenDManagedSessionError::Io)?;
        stream
            .set_write_timeout(Some(timeout))
            .map_err(OpenDManagedSessionError::Io)?;
        let reader = stream.try_clone().map_err(OpenDManagedSessionError::Io)?;
        let (event_sender, event_receiver) = mpsc::channel();
        let state = Arc::new(SessionState {
            generation,
            writer: Mutex::new(stream),
            pending: Mutex::new(BTreeMap::new()),
            events: event_sender,
            closed: AtomicBool::new(false),
            close_reason: Mutex::new(None),
            closed_lock: Mutex::new(()),
            closed_signal: Condvar::new(),
            next_serial: AtomicU32::new(0),
        });
        let reader_state = Arc::clone(&state);
        let worker = thread::Builder::new()
            .name(format!("jftrade-opend-reader-{generation}"))
            .spawn(move || run_reader(reader, reader_state))
            .map_err(OpenDManagedSessionError::WorkerStart)?;
        Ok(Self {
            state,
            request_timeout: timeout,
            events: Mutex::new(event_receiver),
            worker: Mutex::new(Some(worker)),
            keep_alive_worker: Mutex::new(None),
            keep_alive_started: AtomicBool::new(false),
            observer: None,
            request_id: String::new(),
        })
    }

    pub fn generation(&self) -> u64 {
        self.state.generation
    }

    pub fn is_closed(&self) -> bool {
        self.state.closed.load(Ordering::Acquire)
    }

    pub fn close_reason(
        &self,
    ) -> Result<Option<OpenDSessionCloseReason>, OpenDManagedSessionError> {
        self.state
            .close_reason
            .lock()
            .map(|reason| reason.clone())
            .map_err(|_| OpenDManagedSessionError::StateUnavailable)
    }

    pub fn call(
        &self,
        protocol: u32,
        protobuf_body: &[u8],
    ) -> Result<Vec<u8>, OpenDManagedSessionError> {
        self.call_with_timeout(protocol, protobuf_body, self.request_timeout)
    }

    pub fn call_with_timeout(
        &self,
        protocol: u32,
        protobuf_body: &[u8],
        timeout: Duration,
    ) -> Result<Vec<u8>, OpenDManagedSessionError> {
        let result = call_state(&self.state, protocol, protobuf_body, timeout);
        if let Some(observer) = &self.observer {
            observer.record_open_d_call(&OpenDCallRecord {
                operation: format!("proto_{protocol}"),
                request_id: self.request_id.clone(),
                error: result.as_ref().err().map(ToString::to_string),
            });
        }
        result
    }

    /// Sends Futu `KeepAlive` (1004) frames until the session closes.
    ///
    /// OpenD advertises the required interval in `InitConnect.S2C.keepAliveInterval`.
    /// Missing heartbeats can leave a TCP session accepted but unresponsive
    /// until OpenD restarts, so the long-lived market-data role starts this
    /// worker from the handshake value. The first valid call owns the single
    /// worker; later calls are ignored. Returns whether this call started it.
    pub fn start_keep_alive(&self, interval: Duration) -> bool {
        if interval.is_zero() {
            return false;
        }
        if self.state.closed.load(Ordering::Acquire) {
            return false;
        }
        if self.keep_alive_started.swap(true, Ordering::AcqRel) {
            return false;
        }
        let state = Arc::clone(&self.state);
        let request_timeout = self.request_timeout;
        let Ok(handle) = thread::Builder::new()
            .name(format!("jftrade-opend-keepalive-{}", state.generation))
            .spawn(move || run_keep_alive(state, interval, request_timeout))
        else {
            self.keep_alive_started.store(false, Ordering::Release);
            return false;
        };
        if let Ok(mut slot) = self.keep_alive_worker.lock() {
            *slot = Some(handle);
        }
        if self.state.closed.load(Ordering::Acquire) {
            // The session closed while the worker was being registered; join it
            // here so a concurrent `close` that already returned cannot leak it.
            let _ = self.join_keep_alive();
        }
        true
    }

    pub fn keep_alive_started(&self) -> bool {
        self.keep_alive_started.load(Ordering::Acquire)
    }

    pub fn receive_event_timeout(
        &self,
        timeout: Duration,
    ) -> Result<OpenDSessionEvent, RecvTimeoutError> {
        let Ok(events) = self.events.lock() else {
            return Err(RecvTimeoutError::Disconnected);
        };
        events.recv_timeout(timeout)
    }

    pub fn close(&self) -> Result<bool, OpenDManagedSessionError> {
        let closed = terminate(&self.state, OpenDSessionCloseReason::Local);
        self.join_worker()?;
        self.join_keep_alive()?;
        Ok(closed)
    }

    fn join_worker(&self) -> Result<(), OpenDManagedSessionError> {
        let worker = self
            .worker
            .lock()
            .map_err(|_| OpenDManagedSessionError::StateUnavailable)?
            .take();
        match worker.map(JoinHandle::join) {
            Some(Err(_)) => Err(OpenDManagedSessionError::WorkerPanicked),
            _ => Ok(()),
        }
    }

    fn join_keep_alive(&self) -> Result<(), OpenDManagedSessionError> {
        let worker = self
            .keep_alive_worker
            .lock()
            .map_err(|_| OpenDManagedSessionError::StateUnavailable)?
            .take();
        match worker.map(JoinHandle::join) {
            Some(Err(_)) => Err(OpenDManagedSessionError::WorkerPanicked),
            _ => Ok(()),
        }
    }
}

impl Drop for OpenDManagedSession {
    fn drop(&mut self) {
        let _ = self.close();
    }
}

fn run_reader(mut reader: TcpStream, state: Arc<SessionState>) {
    loop {
        match read_framed_frame(&mut reader) {
            Ok(frame) => dispatch_frame(&state, frame),
            // Go's read loop skips frames that fail protobuf/wire validation
            // after their declared length has been consumed, and only closes
            // the session on I/O failures or unbounded length declarations.
            Err(TcpTransportError::Frame(FrameError::BadMagic | FrameError::BadBodyHash)) => {
                continue;
            }
            Err(error) => {
                terminate(&state, close_reason_from_read(error));
                return;
            }
        }
    }
}

fn call_state(
    state: &Arc<SessionState>,
    protocol: u32,
    protobuf_body: &[u8],
    timeout: Duration,
) -> Result<Vec<u8>, OpenDManagedSessionError> {
    let serial = next_serial(state);
    let packet = encode_frame(protocol, serial, protobuf_body)?;
    let (sender, receiver) = mpsc::sync_channel(1);
    register_pending(
        state,
        serial,
        PendingCall {
            protocol,
            response: sender,
        },
    )?;
    if let Err(error) = write_packet(state, &packet) {
        remove_pending(state, serial)?;
        terminate(state, OpenDSessionCloseReason::Transport(error.to_string()));
        return Err(error);
    }
    match receiver.recv_timeout(timeout) {
        Ok(Ok(frame)) => Ok(frame.body),
        Ok(Err(reason)) => Err(OpenDManagedSessionError::Closed(reason)),
        Err(RecvTimeoutError::Timeout) => {
            remove_pending(state, serial)?;
            Err(OpenDManagedSessionError::RequestTimeout { protocol, serial })
        }
        Err(RecvTimeoutError::Disconnected) => Err(OpenDManagedSessionError::Closed(
            current_close_reason(state)?,
        )),
    }
}

fn next_serial(state: &SessionState) -> u32 {
    loop {
        let current = state.next_serial.load(Ordering::Relaxed);
        let next = current.wrapping_add(1).max(1);
        if state
            .next_serial
            .compare_exchange_weak(current, next, Ordering::Relaxed, Ordering::Relaxed)
            .is_ok()
        {
            return next;
        }
    }
}

fn register_pending(
    state: &SessionState,
    serial: u32,
    call: PendingCall,
) -> Result<(), OpenDManagedSessionError> {
    let mut pending = state
        .pending
        .lock()
        .map_err(|_| OpenDManagedSessionError::StateUnavailable)?;
    if state.closed.load(Ordering::Acquire) {
        return Err(OpenDManagedSessionError::Closed(current_close_reason(
            state,
        )?));
    }
    pending.insert(serial, call);
    Ok(())
}

fn remove_pending(state: &SessionState, serial: u32) -> Result<(), OpenDManagedSessionError> {
    state
        .pending
        .lock()
        .map_err(|_| OpenDManagedSessionError::StateUnavailable)?
        .remove(&serial);
    Ok(())
}

fn write_packet(state: &SessionState, packet: &[u8]) -> Result<(), OpenDManagedSessionError> {
    state
        .writer
        .lock()
        .map_err(|_| OpenDManagedSessionError::StateUnavailable)?
        .write_all(packet)
        .map_err(OpenDManagedSessionError::Io)
}

fn current_close_reason(
    state: &SessionState,
) -> Result<OpenDSessionCloseReason, OpenDManagedSessionError> {
    Ok(lock_unpoisoned(&state.close_reason)
        .clone()
        .unwrap_or(OpenDSessionCloseReason::PeerClosed))
}

/// Mirrors Go `Client.keepAliveLoop`'s cadence: intervals longer than one
/// second are halved so a missed heartbeat is detected within the interval.
fn keep_alive_tick(interval: Duration) -> Duration {
    if interval > Duration::from_secs(1) {
        interval / 2
    } else {
        interval
    }
}

fn run_keep_alive(state: Arc<SessionState>, interval: Duration, request_timeout: Duration) {
    let tick = keep_alive_tick(interval);
    loop {
        if wait_for_close(&state, tick) {
            return;
        }
        let timeout = if request_timeout.is_zero() || request_timeout > tick {
            tick
        } else {
            request_timeout
        };
        let request = KeepAliveRequest {
            c2s: KeepAliveC2s {
                time: SystemTime::now()
                    .duration_since(UNIX_EPOCH)
                    .map(|elapsed| elapsed.as_secs() as i64)
                    .unwrap_or_default(),
            },
        };
        let body = request.encode_to_vec();
        let failure = match call_state(&state, PROTO_KEEP_ALIVE, &body, timeout) {
            Ok(raw) => match KeepAliveResponse::decode(raw.as_slice()) {
                Ok(response) if response.ret_type == 0 => None,
                Ok(response) => Some(format!(
                    "OpenD keep-alive returned retType={}: {}",
                    response.ret_type,
                    response.ret_msg.unwrap_or_else(|| "no message".to_owned())
                )),
                Err(error) => Some(format!("OpenD keep-alive response decode failed: {error}")),
            },
            // A failed or timed-out keep-alive leaves the session unusable, so
            // the worker terminates it exactly like Go's closeConn(true).
            Err(error) => Some(error.to_string()),
        };
        if let Some(message) = failure {
            terminate(&state, OpenDSessionCloseReason::Transport(message));
            return;
        }
    }
}

fn wait_for_close(state: &SessionState, timeout: Duration) -> bool {
    if state.closed.load(Ordering::Acquire) {
        return true;
    }
    let guard = lock_unpoisoned(&state.closed_lock);
    let (_guard, _) = state
        .closed_signal
        .wait_timeout_while(guard, timeout, |_| !state.closed.load(Ordering::Acquire))
        .unwrap_or_else(std::sync::PoisonError::into_inner);
    state.closed.load(Ordering::Acquire)
}

fn dispatch_frame(state: &SessionState, frame: Frame) {
    let pending = {
        let mut pending = lock_unpoisoned(&state.pending);
        let serial = frame.header.serial_no;
        let matches = pending
            .get(&serial)
            .is_some_and(|call| call.protocol == frame.header.proto_id);
        matches.then(|| pending.remove(&serial)).flatten()
    };
    if let Some(call) = pending {
        let _ = call.response.send(Ok(frame));
        return;
    }
    let _ = state.events.send(OpenDSessionEvent::UnsolicitedFrame {
        generation: state.generation,
        frame,
    });
}

fn terminate(state: &SessionState, reason: OpenDSessionCloseReason) -> bool {
    let pending = {
        // Hold the close gate while publishing the reason so a keep-alive
        // worker parked in `wait_for_close` cannot miss the wake-up.
        let _gate = lock_unpoisoned(&state.closed_lock);
        let mut pending = lock_unpoisoned(&state.pending);
        if state.closed.swap(true, Ordering::AcqRel) {
            return false;
        }
        *lock_unpoisoned(&state.close_reason) = Some(reason.clone());
        state.closed_signal.notify_all();
        std::mem::take(&mut *pending)
    };
    let _ = lock_unpoisoned(&state.writer).shutdown(Shutdown::Both);
    for call in pending.into_values() {
        let _ = call.response.send(Err(reason.clone()));
    }
    let _ = state.events.send(OpenDSessionEvent::Closed {
        generation: state.generation,
        reason,
    });
    state.closed_signal.notify_all();
    true
}

fn lock_unpoisoned<T>(mutex: &Mutex<T>) -> MutexGuard<'_, T> {
    mutex
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner)
}

fn close_reason_from_read(error: TcpTransportError) -> OpenDSessionCloseReason {
    match error {
        TcpTransportError::Io(error) if error.kind() == io::ErrorKind::UnexpectedEof => {
            OpenDSessionCloseReason::PeerClosed
        }
        TcpTransportError::Io(error) => OpenDSessionCloseReason::Transport(error.to_string()),
        TcpTransportError::Frame(error) => OpenDSessionCloseReason::InvalidFrame(error),
    }
}

#[cfg(test)]
#[path = "managed_session_tests.rs"]
mod tests;
