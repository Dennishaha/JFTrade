use std::collections::BTreeMap;
use std::io::{self, Write};
use std::net::{Shutdown, SocketAddr, TcpStream};
use std::sync::atomic::{AtomicBool, AtomicU32, Ordering};
use std::sync::mpsc::{self, Receiver, RecvTimeoutError, SyncSender};
use std::sync::{Arc, Mutex, MutexGuard};
use std::thread::{self, JoinHandle};
use std::time::Duration;

use thiserror::Error;

use crate::transport::{TcpTransportError, read_framed_frame};
use crate::{Frame, FrameError, encode_frame};

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
}

/// Test-composition OpenD session with one socket reader.
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
    next_serial: AtomicU32,
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
            next_serial: AtomicU32::new(0),
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
        let serial = self.next_serial();
        let packet = encode_frame(protocol, serial, protobuf_body)?;
        let (sender, receiver) = mpsc::sync_channel(1);
        self.register_pending(
            serial,
            PendingCall {
                protocol,
                response: sender,
            },
        )?;
        if let Err(error) = self.write_packet(&packet) {
            self.remove_pending(serial)?;
            terminate(
                &self.state,
                OpenDSessionCloseReason::Transport(error.to_string()),
            );
            self.join_worker()?;
            return Err(error);
        }
        match receiver.recv_timeout(self.request_timeout) {
            Ok(Ok(frame)) => Ok(frame.body),
            Ok(Err(reason)) => Err(OpenDManagedSessionError::Closed(reason)),
            Err(RecvTimeoutError::Timeout) => {
                self.remove_pending(serial)?;
                Err(OpenDManagedSessionError::RequestTimeout { protocol, serial })
            }
            Err(RecvTimeoutError::Disconnected) => Err(OpenDManagedSessionError::Closed(
                self.current_close_reason()?,
            )),
        }
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
        Ok(closed)
    }

    fn next_serial(&self) -> u32 {
        loop {
            let current = self.next_serial.load(Ordering::Relaxed);
            let next = current.wrapping_add(1).max(1);
            if self
                .next_serial
                .compare_exchange_weak(current, next, Ordering::Relaxed, Ordering::Relaxed)
                .is_ok()
            {
                return next;
            }
        }
    }

    fn register_pending(
        &self,
        serial: u32,
        call: PendingCall,
    ) -> Result<(), OpenDManagedSessionError> {
        let mut pending = self
            .state
            .pending
            .lock()
            .map_err(|_| OpenDManagedSessionError::StateUnavailable)?;
        if self.state.closed.load(Ordering::Acquire) {
            return Err(OpenDManagedSessionError::Closed(
                self.current_close_reason()?,
            ));
        }
        pending.insert(serial, call);
        Ok(())
    }

    fn remove_pending(&self, serial: u32) -> Result<(), OpenDManagedSessionError> {
        self.state
            .pending
            .lock()
            .map_err(|_| OpenDManagedSessionError::StateUnavailable)?
            .remove(&serial);
        Ok(())
    }

    fn write_packet(&self, packet: &[u8]) -> Result<(), OpenDManagedSessionError> {
        self.state
            .writer
            .lock()
            .map_err(|_| OpenDManagedSessionError::StateUnavailable)?
            .write_all(packet)
            .map_err(OpenDManagedSessionError::Io)
    }

    fn current_close_reason(&self) -> Result<OpenDSessionCloseReason, OpenDManagedSessionError> {
        Ok(self
            .close_reason()?
            .unwrap_or(OpenDSessionCloseReason::PeerClosed))
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
            Err(error) => {
                terminate(&state, close_reason_from_read(error));
                return;
            }
        }
    }
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
        let mut pending = lock_unpoisoned(&state.pending);
        if state.closed.swap(true, Ordering::AcqRel) {
            return false;
        }
        *lock_unpoisoned(&state.close_reason) = Some(reason.clone());
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
