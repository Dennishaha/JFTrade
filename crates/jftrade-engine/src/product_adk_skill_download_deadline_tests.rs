//! The production download/install owner runs against real TCP responses with
//! a paused Tokio clock. Request handshakes precede every clock advance.

use super::super::{AdkMutationPortError, install_downloaded_skill};
use super::*;
use crate::product::product_adk_chat_stream_port::AdkChatStreamPort;
use std::io::{BufRead, BufReader};
use std::net::{SocketAddr, TcpListener, TcpStream};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Mutex, mpsc};
use std::time::{Duration, Instant};

#[derive(Clone, Copy)]
enum ResponseStage {
    Headers,
    Body,
    Redirect,
}

enum Release {
    Redirect,
    Complete,
    Abort,
}

struct HeldDownload {
    address: SocketAddr,
    requests: mpsc::Receiver<String>,
    release: mpsc::Sender<Release>,
    stop: Arc<AtomicBool>,
    worker: Option<std::thread::JoinHandle<()>>,
}

const DOCUMENT: &[u8] = b"---\nname: deadline-skill\ndescription: Deadline skill\nallowed-tools: [http.fetch]\n---\nUse the downloaded document.";

impl HeldDownload {
    fn start(stage: ResponseStage) -> Self {
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let address = listener.local_addr().unwrap();
        listener.set_nonblocking(true).unwrap();
        let (request_sender, requests) = mpsc::channel();
        let (release, releases) = mpsc::channel();
        let stop = Arc::new(AtomicBool::new(false));
        let worker_stop = stop.clone();
        let worker = std::thread::spawn(move || {
            let Some(mut stream) = accept_request(&listener, &worker_stop) else {
                return;
            };
            let path = read_request(&stream);
            if matches!(stage, ResponseStage::Redirect) {
                request_sender.send(path).unwrap();
                if !matches!(
                    releases.recv_timeout(Duration::from_secs(5)).unwrap(),
                    Release::Redirect
                ) {
                    return;
                }
                stream.write_all(b"HTTP/1.1 302 Found\r\nLocation: /skill.md\r\nContent-Length: 0\r\nConnection: close\r\n\r\n").unwrap();
                drop(stream);
                let Some(next) = accept_request(&listener, &worker_stop) else {
                    return;
                };
                stream = next;
                let path = read_request(&stream);
                write_headers(&mut stream);
                request_sender.send(path).unwrap();
            } else {
                if matches!(stage, ResponseStage::Body) {
                    write_headers(&mut stream);
                }
                request_sender.send(path).unwrap();
            }
            if matches!(
                releases.recv_timeout(Duration::from_secs(5)).unwrap(),
                Release::Complete
            ) {
                if matches!(stage, ResponseStage::Headers) {
                    write_headers(&mut stream);
                }
                stream.write_all(DOCUMENT).unwrap();
            }
        });
        Self {
            address,
            requests,
            release,
            stop,
            worker: Some(worker),
        }
    }
}

impl Drop for HeldDownload {
    fn drop(&mut self) {
        self.stop.store(true, Ordering::Release);
        let _ = self.release.send(Release::Abort);
        self.worker.take().unwrap().join().unwrap();
    }
}

fn accept_request(listener: &TcpListener, stop: &AtomicBool) -> Option<TcpStream> {
    let deadline = Instant::now() + Duration::from_secs(5);
    while !stop.load(Ordering::Acquire) {
        match listener.accept() {
            Ok((stream, _)) => {
                stream.set_nonblocking(false).unwrap();
                stream
                    .set_read_timeout(Some(Duration::from_secs(5)))
                    .unwrap();
                stream
                    .set_write_timeout(Some(Duration::from_secs(5)))
                    .unwrap();
                return Some(stream);
            }
            Err(error) if error.kind() == std::io::ErrorKind::WouldBlock => {
                assert!(
                    Instant::now() < deadline,
                    "fixture never received a connection"
                );
                std::thread::yield_now();
            }
            Err(error) => panic!("fixture accept: {error}"),
        }
    }
    None
}

fn read_request(stream: &TcpStream) -> String {
    let mut reader = BufReader::new(stream.try_clone().unwrap());
    let mut request = String::new();
    assert!(reader.read_line(&mut request).unwrap() > 0);
    loop {
        let mut line = String::new();
        assert!(reader.read_line(&mut line).unwrap() > 0);
        if line == "\r\n" {
            break;
        }
    }
    request.split_whitespace().nth(1).unwrap().to_owned()
}

fn write_headers(stream: &mut TcpStream) {
    write!(stream, "HTTP/1.1 200 OK\r\nContent-Type: text/markdown\r\nContent-Length: {}\r\nConnection: close\r\n\r\n", DOCUMENT.len()).unwrap();
}

async fn await_request(server: &HeldDownload, path: &str) {
    let deadline = Instant::now() + Duration::from_secs(3);
    loop {
        match server.requests.try_recv() {
            Ok(actual) => {
                assert_eq!(actual, path);
                return;
            }
            Err(mpsc::TryRecvError::Empty) => {
                assert!(
                    Instant::now() < deadline,
                    "request handshake missing: {path}"
                );
                // This runnable root task prevents paused-clock auto-advance
                // while the actual socket and blocking resolver make progress.
                tokio::task::yield_now().await;
            }
            Err(error) => panic!("fixture handshake: {error}"),
        }
    }
}

async fn await_finished<T>(task: &tokio::task::JoinHandle<T>) {
    let deadline = Instant::now() + Duration::from_secs(3);
    while !task.is_finished() {
        assert!(
            Instant::now() < deadline,
            "production owner did not finish at the advanced deadline"
        );
        tokio::task::yield_now().await;
    }
}

async fn download_deadline_case(stage: ResponseStage, complete_before_deadline: bool) {
    let root = tempdir().unwrap();
    let port = test_port(root.path());
    let rows = port.store.list_skills().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let server = HeldDownload::start(stage);
    let address = server.address;
    let resolutions = Arc::new(Mutex::new(Vec::new()));
    let recorded = resolutions.clone();
    let resolve = Arc::new(move |url: reqwest::Url| {
        recorded.lock().unwrap().push(url.path().to_owned());
        Ok(address)
    });
    let path = if matches!(stage, ResponseStage::Redirect) {
        "/redirect"
    } else {
        "/skill.md"
    };
    let url = format!("http://skills.example:{}{path}", address.port());
    let task_port = port.clone();
    let source = url.clone();
    let start = tokio::time::Instant::now();
    let task = tokio::spawn(async move {
        install_downloaded_skill(
            &task_port,
            &source,
            reqwest::Url::parse(&source).unwrap(),
            resolve,
        )
        .await
    });
    await_request(&server, path).await;
    assert_eq!(
        tokio::time::Instant::now(),
        start,
        "handshake must not advance the clock"
    );
    if matches!(stage, ResponseStage::Redirect) {
        tokio::time::advance(Duration::from_secs(12)).await;
        assert!(!task.is_finished());
        server.release.send(Release::Redirect).unwrap();
        await_request(&server, "/skill.md").await;
        assert_eq!(tokio::time::Instant::now() - start, Duration::from_secs(12));
    }
    tokio::time::advance(Duration::from_secs(
        if matches!(stage, ResponseStage::Redirect) {
            7
        } else {
            19
        },
    ))
    .await;
    assert!(
        !task.is_finished(),
        "download must remain active before 20 seconds"
    );
    if complete_before_deadline {
        server.release.send(Release::Complete).unwrap();
    } else {
        tokio::time::advance(Duration::from_secs(1)).await;
    }
    await_finished(&task).await;
    assert_eq!(
        tokio::time::Instant::now() - start,
        Duration::from_secs(if complete_before_deadline { 19 } else { 20 })
    );
    let result = task.await.unwrap();
    port.shutdown_with_error().unwrap();
    if complete_before_deadline {
        let installed = result.unwrap();
        assert_eq!(installed["id"], "deadline-skill");
        assert_eq!(installed["source"], url);
        assert_eq!(installed["tools"], serde_json::json!(["http.fetch"]));
        assert_eq!(port.store.list_skills().unwrap().len(), rows.len() + 1);
        assert!(root.path().join("skills/deadline-skill/SKILL.md").exists());
    } else {
        let error = result.unwrap_err();
        assert!(
            matches!(&error, AdkMutationPortError::Failed { status: 400, code, message } if code == "ADK_SKILL_INSTALL_FAILED" && message == "skill download timed out"),
            "{error}"
        );
        assert_eq!(port.store.list_skills().unwrap(), rows);
        assert_eq!(port.store.list_audit_events().unwrap(), audit);
        assert!(!root.path().join("skills").exists());
    }
    let expected = if matches!(stage, ResponseStage::Redirect) {
        vec!["/redirect", "/skill.md"]
    } else {
        vec!["/skill.md"]
    };
    assert_eq!(*resolutions.lock().unwrap(), expected);
}

#[tokio::test(start_paused = true)]
async fn production_skill_download_deadline_covers_stalled_headers_at_twenty_seconds() {
    download_deadline_case(ResponseStage::Headers, false).await;
}

#[tokio::test(start_paused = true)]
async fn production_skill_download_deadline_covers_stalled_body_at_twenty_seconds() {
    download_deadline_case(ResponseStage::Body, false).await;
}

#[tokio::test(start_paused = true)]
async fn production_skill_download_deadline_is_shared_across_redirects() {
    download_deadline_case(ResponseStage::Redirect, false).await;
}

#[tokio::test(start_paused = true)]
async fn production_skill_download_deadline_accepts_a_document_before_twenty_seconds() {
    download_deadline_case(ResponseStage::Body, true).await;
}
