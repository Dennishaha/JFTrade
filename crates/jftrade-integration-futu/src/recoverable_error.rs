//! OpenD recoverable-error classification (Go `isRecoverableOpenDErr`).
//!
//! Go's exchange client decides whether a failed read may be replayed on a
//! freshly connected client (`pkg/futu/exchange_client.go`). Rust keeps its
//! retry policy per read port, so the classification itself needs one shared
//! owner: otherwise a new port can silently treat a permission error as
//! recoverable, or a closed socket as terminal.

use std::io;

/// Error categories the OpenD adapter treats as recoverable.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum OpenDRecoverableKind {
    /// The session is closed (peer close, local close, or EOF).
    Closed,
    /// The request timed out and may be replayed on a new session.
    RequestTimeout,
    /// Transport-level I/O failure (broken pipe, connection reset, use of a
    /// closed network connection, unexpected EOF).
    Transport,
}

/// Classify an error by its OpenD-recoverable category.
///
/// Mirrors Go exactly: `nil` and permission/business errors are not
/// recoverable, while closed/timeout errors and the transport substrings Go
/// inspects are. The substring check is retained deliberately — OpenD
/// transport failures reach Rust as `io::Error` text, and Go's own behavior is
/// what the compatibility checklist freezes.
pub fn classify_recoverable(error_text: Option<&str>) -> Option<OpenDRecoverableKind> {
    let text = error_text?;
    let lower = text.to_ascii_lowercase();
    // The first two substrings are Go's own sentinel texts
    // (`opend: client closed`, `opend: request timed out`), the next three are
    // the Rust session owners, and the last are the transport strings Go's
    // `isRecoverableOpenDErr` inspects.
    if lower.contains("opend: client closed")
        || lower.contains("opend managed session is closed")
        || lower.contains("opend session coordinator is closed")
        || lower.contains("opend request proto")
        || lower.contains("use of closed network connection")
        || lower == "eof"
        || lower.contains("unexpected eof")
    {
        return Some(OpenDRecoverableKind::Closed);
    }
    if lower.contains("opend: request timed out")
        || lower.contains("timed out")
        || lower.contains("timeout")
    {
        return Some(OpenDRecoverableKind::RequestTimeout);
    }
    if lower.contains("broken pipe")
        || lower.contains("connection reset")
        || lower.contains("connection aborted")
        || lower.contains("eof")
    {
        return Some(OpenDRecoverableKind::Transport);
    }
    None
}

/// Classify a `std::io::Error` directly, so callers that already hold the
/// typed error do not have to stringify it first.
pub fn classify_recoverable_io(error: &io::Error) -> Option<OpenDRecoverableKind> {
    match error.kind() {
        io::ErrorKind::BrokenPipe
        | io::ErrorKind::ConnectionReset
        | io::ErrorKind::ConnectionAborted
        | io::ErrorKind::UnexpectedEof
        | io::ErrorKind::NotConnected => Some(OpenDRecoverableKind::Transport),
        io::ErrorKind::TimedOut => Some(OpenDRecoverableKind::RequestTimeout),
        _ => classify_recoverable(Some(&error.to_string())),
    }
}

/// True when the error may be replayed after a reconnect.
pub fn is_recoverable_error(error_text: Option<&str>) -> bool {
    classify_recoverable(error_text).is_some()
}

#[cfg(test)]
#[path = "recoverable_error_tests.rs"]
mod tests;
