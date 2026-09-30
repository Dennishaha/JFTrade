use super::*;

#[test]
// Parity: go:452dea11:pkg/futu/read_account_test.go:12 TestRecoverableOpenDErrClassifiesConnectionFailures
fn recoverable_errors_match_go_is_recoverable_opend_err() {
    // Parity: go:pkg/futu/client_exchange_recovery_boundaries_test.go:19
    // TestWithClientReplayPolicyForRecoverableErrors. Go accepts closed
    // sessions, request timeouts and four transport substrings, and rejects
    // business/permission errors.
    for text in [
        "OpenD session coordinator is closed",
        "OpenD managed session is closed: peer",
        "OpenD request proto 3004 serial 1 timed out",
        "request timeout while reading",
        "write tcp: broken pipe",
        "read tcp: connection reset by peer",
        "read: EOF",
        "write: use of closed network connection",
    ] {
        assert!(
            is_recoverable_error(Some(text)),
            "recoverable text rejected: {text}"
        );
    }
    assert!(!is_recoverable_error(None));
    assert!(!is_recoverable_error(Some("permission denied")));
    assert!(!is_recoverable_error(Some(
        "Qot_GetKL retType=-1 errCode=9 retMsg=kline entitlement denied"
    )));
    assert_eq!(
        classify_recoverable(Some("broken pipe")),
        Some(OpenDRecoverableKind::Transport)
    );
    assert_eq!(
        classify_recoverable(Some("OpenD managed session is closed: peer")),
        Some(OpenDRecoverableKind::Closed)
    );
}

#[test]
// Parity: go:452dea11:pkg/futu/read_account_test.go:12 TestRecoverableOpenDErrClassifiesConnectionFailures
fn io_error_kinds_are_classified_without_string_matching() {
    for kind in [
        std::io::ErrorKind::BrokenPipe,
        std::io::ErrorKind::ConnectionReset,
        std::io::ErrorKind::ConnectionAborted,
        std::io::ErrorKind::UnexpectedEof,
        std::io::ErrorKind::NotConnected,
    ] {
        assert_eq!(
            classify_recoverable_io(&std::io::Error::from(kind)),
            Some(OpenDRecoverableKind::Transport),
            "{kind:?}"
        );
    }
    assert_eq!(
        classify_recoverable_io(&std::io::Error::from(std::io::ErrorKind::TimedOut)),
        Some(OpenDRecoverableKind::RequestTimeout)
    );
    assert_eq!(
        classify_recoverable_io(&std::io::Error::from(std::io::ErrorKind::PermissionDenied)),
        None
    );
}
