use super::*;
use std::io::{BufRead, Read, Write};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::mpsc;
use std::thread;
use std::time::Instant;

use super::super::{AdkChatPortError, AdkChatRoute, RUN_LEASE_HEARTBEAT, RUN_LEASE_TTL};

// Parity: go:452dea11:internal/assistant/engine/runtime_execution_lease_boundaries_test.go:161 TestRunExecutionLeaseUsesSafeDefaults
#[test]
fn production_run_lease_defaults_use_thirty_second_ttl_and_join_on_drop() {
    let (_directory, store, _sessions) = initialized_stores();
    create_running_run(&store, "run-default", json!([]));
    let baseline = Arc::strong_count(&store);
    let guard = RunLeaseGuard::acquire(store.clone(), "run-default", "default-owner").unwrap();
    assert_eq!(RUN_LEASE_TTL, Duration::from_secs(30));
    assert_eq!(RUN_LEASE_HEARTBEAT, Duration::from_secs(10));
    assert_eq!(
        guard.lease.expires_at_unix_ms - guard.lease.heartbeat_at_unix_ms,
        30_000
    );
    assert!(!guard.is_lost());
    drop(guard);
    let released = store.get_run_lease("run-default").unwrap().unwrap();
    assert_eq!(released.owner_id, "");
    assert_eq!(released.expires_at_unix_ms, released.heartbeat_at_unix_ms);
    assert_eq!(
        Arc::strong_count(&store),
        baseline,
        "heartbeat thread joined"
    );
}

// Parity: go:452dea11:internal/assistant/engine/runtime_execution_lease_boundaries_test.go:14 TestRunExecutionLeaseContextAndReuseBoundaries
#[test]
fn production_run_lease_reuses_its_owner_and_rejects_a_foreign_owner() {
    let (_directory, store, _sessions) = initialized_stores();
    create_running_run(&store, "run-reuse", json!([]));
    let guard = RunLeaseGuard::acquire(store.clone(), "run-reuse", "reuse-owner").unwrap();
    let reused = store
        .claim_run_lease("run-reuse", "reuse-owner", RUN_LEASE_TTL)
        .unwrap();
    assert_eq!(reused.fencing_token, guard.token());
    assert_eq!(reused.owner_id, guard.owner_id());
    assert!(matches!(
        RunLeaseGuard::acquire(store.clone(), "run-reuse", "foreign-owner"),
        Err(AdkChatPortError::Conflict(_))
    ));
    assert_eq!(store.get_run_lease("run-reuse").unwrap().unwrap(), reused);
    assert!(!guard.is_lost());
}

// Parity: go:452dea11:internal/assistant/engine/runtime_execution_lease_boundaries_test.go:133 TestRefreshRunExecutionLeaseRejectsExpiredLeaseBeforeStoreWrite
#[test]
fn production_heartbeat_rejects_expired_durable_lease_without_changing_the_row() {
    let (directory, store, _sessions) = initialized_stores();
    create_running_run(&store, "run-expired", json!([]));
    let lease = store
        .claim_run_lease("run-expired", "expiry-owner", RUN_LEASE_TTL)
        .unwrap();
    Connection::open(directory.path().join("adk.db"))
        .unwrap()
        .execute(
            "UPDATE adk_run_leases SET expires_at_unix_ms = 0 WHERE run_id = 'run-expired'",
            [],
        )
        .unwrap();
    let before = store.get_run_lease("run-expired").unwrap().unwrap();
    assert!(matches!(
        store.heartbeat_run_lease(&lease, Duration::from_secs(1)),
        Err(jftrade_store_sqlite::AdkStoreError::LeaseLost(_))
    ));
    assert_eq!(store.get_run_lease("run-expired").unwrap().unwrap(), before);
}

// Parity: go:452dea11:internal/assistant/engine/runtime_execution_lease_boundaries_test.go:144 TestRefreshRunExecutionLeaseUsesRemainingTTLForNearExpiryLease
#[test]
fn production_heartbeat_near_expiry_snapshot_restores_the_requested_ttl() {
    let (_directory, store, _sessions) = initialized_stores();
    create_running_run(&store, "run-near-expiry", json!([]));
    let mut lease = store
        .claim_run_lease("run-near-expiry", "near-owner", RUN_LEASE_TTL)
        .unwrap();
    // Go shortens only the caller's snapshot; keep the durable lease live so
    // scheduler load cannot turn this TTL assertion into an expiry race.
    lease.expires_at_unix_ms = lease.heartbeat_at_unix_ms + 100;
    let refreshed = store
        .heartbeat_run_lease(&lease, Duration::from_secs(1))
        .unwrap();
    assert_eq!(
        refreshed.expires_at_unix_ms - refreshed.heartbeat_at_unix_ms,
        1_000
    );
    assert_eq!(refreshed.owner_id, lease.owner_id);
    assert_eq!(refreshed.fencing_token, lease.fencing_token);
    assert_eq!(
        store.get_run_lease("run-near-expiry").unwrap().unwrap(),
        refreshed
    );
}

fn read_provider_request(socket: &mut std::net::TcpStream) {
    let mut reader = std::io::BufReader::new(socket);
    let mut length = None;
    loop {
        let mut line = String::new();
        assert!(reader.read_line(&mut line).unwrap() > 0);
        if line == "\r\n" {
            break;
        }
        if let Some(value) = line.to_ascii_lowercase().strip_prefix("content-length:") {
            length = Some(value.trim().parse::<usize>().unwrap());
        }
    }
    let mut body = vec![0; length.unwrap()];
    reader.read_exact(&mut body).unwrap();
    assert!(serde_json::from_slice::<Value>(&body).unwrap().is_object());
}

fn blocked_provider(
    streaming: bool,
    send_headers: bool,
    json_body: bool,
) -> (
    String,
    mpsc::Receiver<()>,
    mpsc::Sender<()>,
    thread::JoinHandle<()>,
) {
    let listener = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
    listener.set_nonblocking(true).unwrap();
    let address = listener.local_addr().unwrap();
    let (started, entered) = mpsc::channel();
    let (release, released) = mpsc::channel();
    let provider = thread::spawn(move || {
        let deadline = Instant::now() + Duration::from_secs(15);
        let mut socket = loop {
            match listener.accept() {
                Ok((socket, _)) => break socket,
                Err(error) if error.kind() == std::io::ErrorKind::WouldBlock => {
                    assert!(Instant::now() < deadline, "provider request must arrive");
                    thread::sleep(Duration::from_millis(5));
                }
                Err(error) => panic!("provider accept: {error}"),
            }
        };
        socket.set_nonblocking(false).unwrap();
        socket
            .set_read_timeout(Some(Duration::from_secs(15)))
            .unwrap();
        read_provider_request(&mut socket);
        if streaming && send_headers && !json_body {
            socket.write_all(b"HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nConnection: close\r\n\r\n").unwrap();
        }
        if streaming && send_headers && json_body {
            socket.write_all(b"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 22\r\nConnection: close\r\n\r\n").unwrap();
        }
        started.send(()).unwrap();
        released.recv_timeout(Duration::from_secs(45)).unwrap();
        let response = if streaming && !json_body {
            let headers = if send_headers {
                ""
            } else {
                "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nConnection: close\r\n\r\n"
            };
            format!(
                "{headers}data: {{\"type\":\"response.completed\",\"response\":{{\"output_text\":\"late\"}}}}\n\n"
            )
        } else {
            let body = "{\"output_text\":\"late\"}";
            if send_headers {
                body.to_owned()
            } else {
                format!(
                    "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
                    body.len()
                )
            }
        };
        let _ = socket.write_all(response.as_bytes());
    });
    (
        format!("http://{address}/v1/responses"),
        entered,
        release,
        provider,
    )
}

fn heartbeat_failure_cancels_provider(streaming: bool, send_headers: bool, json_body: bool) {
    let (directory, store, sessions) = initialized_stores();
    let runtime = runtime_for(&directory, &store, &sessions);
    let run_id = "run-heartbeat-failure";
    create_running_run(&store, run_id, json!([]));
    let before = store.get_run(run_id).unwrap().unwrap();
    let lease = RunLeaseGuard::acquire(store.clone(), run_id, "heartbeat-owner").unwrap();
    let lost = lease.lost.clone();
    let (endpoint, entered, release, provider) =
        blocked_provider(streaming, send_headers, json_body);
    let mut chat = chat_for(run_id);
    chat.request.endpoint = endpoint.parse().unwrap();
    chat.request.timeout = Duration::from_secs(60);
    if streaming {
        chat.route = AdkChatRoute::Stream;
    }
    let (stream, sender) = jftrade_api::ApiStream::channel(8);
    let (finished, done) = mpsc::channel();
    let worker = thread::spawn(move || {
        if streaming {
            runtime.run_live_stream(chat, sender, Arc::new(AtomicBool::new(false)), lease);
        } else {
            assert!(
                matches!(runtime.execute_chat(chat, lease), Err(AdkChatPortError::Unavailable(message)) if message.contains("lease was lost"))
            );
        }
        finished.send(()).unwrap();
    });
    entered.recv_timeout(Duration::from_secs(15)).unwrap();
    // Fail the actual production heartbeat UPDATE while reads and the
    // existing lease remain valid. This is no takeover or short-TTL race.
    Connection::open(directory.path().join("adk.db")).unwrap().execute_batch(
        "CREATE TRIGGER fail_run_heartbeat BEFORE UPDATE OF heartbeat_at_unix_ms ON adk_run_leases
         WHEN NEW.owner_id != ''
         BEGIN SELECT RAISE(FAIL, 'fixture heartbeat write unavailable'); END;",
    ).unwrap();
    let deadline = Instant::now() + Duration::from_secs(25);
    while !lost.load(Ordering::Acquire) && Instant::now() < deadline {
        thread::sleep(Duration::from_millis(5));
    }
    let lost_observed = lost.load(Ordering::Acquire);
    let cancelled_before_reply = done.recv_timeout(Duration::from_secs(2)).is_ok();
    release.send(()).unwrap();
    provider.join().unwrap();
    worker.join().unwrap();
    drop(stream);
    assert!(
        lost_observed,
        "production heartbeat must observe the storage fault"
    );
    assert!(
        cancelled_before_reply,
        "heartbeat failure must cancel and join the provider owner before a reply"
    );
    assert_eq!(
        store.get_run(run_id).unwrap().unwrap(),
        before,
        "lost owner cannot write a late terminal state"
    );
    assert!(store.list_audit_events().unwrap().is_empty());
    assert!(
        sessions
            .list_events(&format!("session-{run_id}"))
            .unwrap()
            .is_empty()
    );
    assert_eq!(store.get_run_lease(run_id).unwrap().unwrap().owner_id, "");
}

// Parity: go:452dea11:internal/assistant/engine/runtime_execution_lease_boundaries_test.go:108 TestRunExecutionLeaseHeartbeatFailureCancelsOwner
#[test]
fn production_heartbeat_write_failure_cancels_an_idle_stream_provider_and_joins() {
    heartbeat_failure_cancels_provider(true, true, false);
}

// Parity: go:452dea11:internal/assistant/engine/runtime_execution_lease_boundaries_test.go:108 TestRunExecutionLeaseHeartbeatFailureCancelsOwner
#[test]
fn production_heartbeat_write_failure_cancels_a_waiting_sync_provider_and_joins() {
    heartbeat_failure_cancels_provider(false, false, false);
}

// Parity: go:452dea11:internal/assistant/engine/runtime_execution_lease_boundaries_test.go:108 TestRunExecutionLeaseHeartbeatFailureCancelsOwner
#[test]
fn production_heartbeat_write_failure_cancels_a_stream_waiting_for_headers_and_joins() {
    heartbeat_failure_cancels_provider(true, false, false);
}

// Parity: go:452dea11:internal/assistant/engine/runtime_execution_lease_boundaries_test.go:108 TestRunExecutionLeaseHeartbeatFailureCancelsOwner
#[test]
fn production_heartbeat_write_failure_cancels_a_stream_waiting_for_json_body_and_joins() {
    heartbeat_failure_cancels_provider(true, true, true);
}
