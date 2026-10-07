use super::*;
use crate::product::product_production_ports::{ProductionPortBundle, production_ports};
use crate::product::product_production_ports::product_production_ports_adk::ProductionAdkPort;
use jftrade_settings::SecuritySettingsService;
use jftrade_store_settings_file::SettingsFileStore;
use jftrade_store_sqlite::AdkArtifactStore;

fn runtime_proxy(runtime: &Arc<ProductionAdkChatRuntime>) -> Arc<ProductionAdkPort> {
    let artifact_path = runtime.settings_path.parent().unwrap().join("adk-artifact.db");
    File::create(&artifact_path).unwrap();
    initialize_current(&Connection::open(&artifact_path).unwrap(), "adk-artifact").unwrap();
    let mut proxy = ProductionAdkPort::new_for_test(
        Arc::clone(&runtime.store), Arc::clone(&runtime.session_store),
        Arc::new(AdkArtifactStore::open(&artifact_path).unwrap()), runtime.settings_path.clone(),
    );
    proxy.chat_runtime = Some(runtime.clone());
    Arc::new(proxy)
}

async fn bundle_fixture(runtime: &Arc<ProductionAdkChatRuntime>) -> (tempfile::TempDir, ProductionPortBundle) {
    let directory = tempdir().unwrap();
    let settings = directory.path().join("settings.json");
    std::fs::write(&settings, b"{}").unwrap();
    crate::product::product_data_management::initialize_production_databases(&settings).unwrap();
    let security = SecuritySettingsService::new(Arc::new(SettingsFileStore::open(&settings).unwrap()));
    let mut config = crate::product::ProductConfig::new(
        "127.0.0.1:0".parse().unwrap(), &settings,
        jftrade_api::AccessPolicy::desktop(Some("a".repeat(32))),
    ).unwrap();
    config.production = true;
    config.capabilities = crate::product::ProductCapabilities::all();
    let mut bundle = production_ports(&config, &security).unwrap();
    bundle.shutdown_adk_runtime().await.unwrap();
    bundle.workflow_scheduler = None;
    bundle.adk_chat_stream = runtime_proxy(runtime);
    (directory, bundle)
}

#[tokio::test(flavor = "multi_thread", worker_threads = 4)]
async fn production_assistant_shutdown_reports_an_unfinished_continuation_deadline() {
    let (_runtime_directory, _store, runtime) = runtime_fixture();
    let (_bundle_directory, bundle) = bundle_fixture(&runtime).await;
    let (started, entered) = std::sync::mpsc::channel();
    let (cancelled, observed) = std::sync::mpsc::channel();
    let (release, cleanup) = std::sync::mpsc::channel();
    runtime.continuation_supervisor.spawn("shutdown-deadline", move |cancellation| {
        started.send(()).unwrap();
        wait_for_cancel(&cancellation);
        cancelled.send(()).unwrap();
        cleanup.recv_timeout(Duration::from_secs(12)).unwrap();
    }).unwrap();
    entered.recv_timeout(Duration::from_secs(1)).unwrap();
    let result = bundle.shutdown_adk_runtime().await;
    observed.recv_timeout(Duration::from_secs(1)).unwrap();
    let retained = runtime.continuation_supervisor.tasks.lock().unwrap().len();
    release.send(()).unwrap();
    assert_eq!(retained, 1);
    assert!(result.unwrap_err().contains("assistant continuations did not finish before shutdown deadline"));
    bundle.shutdown_adk_runtime().await.expect("released continuation can be joined on retry");
    assert!(runtime.continuation_supervisor.tasks.lock().unwrap().is_empty());
}

#[derive(Debug, Default)]
struct PortDetachObserver(std::sync::atomic::AtomicUsize);

impl AdkToolExecutor for PortDetachObserver {
    fn supports(&self, _: &str) -> bool { false }
    fn execute(&self, _: &str, _: &Value) -> Result<Value, String> { Err("unavailable".to_owned()) }
    fn detach_ports(&self) { self.0.fetch_add(1, Ordering::AcqRel); }
}

#[tokio::test]
async fn synchronous_assistant_termination_preserves_ports_until_background_cleanup_finishes() {
    let (directory, store, sessions) = initialized_stores();
    let observer = Arc::new(PortDetachObserver::default());
    let runtime = Arc::new(ProductionAdkChatRuntime::with_tool_executor_for_test(
        store, sessions, &directory.path().join("settings.json"),
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
        observer.clone(),
    ));
    let (_bundle_directory, bundle) = bundle_fixture(&runtime).await;
    let (release, cleanup) = std::sync::mpsc::channel();
    let (cancelled, observed) = std::sync::mpsc::channel();
    runtime.continuation_supervisor.spawn("sync-shutdown-deadline", move |cancellation| {
        wait_for_cancel(&cancellation);
        cancelled.send(()).unwrap();
        cleanup.recv_timeout(Duration::from_secs(12)).unwrap();
    }).unwrap();
    let result = bundle.terminate_adk_runtime();
    observed.recv_timeout(Duration::from_secs(1)).unwrap();
    let detached_before_cleanup = observer.0.load(Ordering::Acquire);
    release.send(()).unwrap();
    assert!(result.unwrap_err().contains("assistant continuations did not finish before shutdown deadline"));
    assert_eq!(detached_before_cleanup, 0);
    bundle.terminate_adk_runtime().expect("synchronous retry joins released cleanup");
    assert_eq!(observer.0.load(Ordering::Acquire), 1);
}

// Parity: go:452dea11:internal/assistant/service_contract_boundaries_test.go:312 TestServiceCloseClosesRuntimeOwnedResources
#[tokio::test]
async fn production_assistant_shutdown_closes_owned_runtime_and_accepts_repeated_close() {
    let (_runtime_directory, _store, runtime) = runtime_fixture();
    let (_bundle_directory, bundle) = bundle_fixture(&runtime).await;
    bundle.shutdown_adk_runtime().await.expect("first close");
    bundle.shutdown_adk_runtime().await.expect("repeated close");
    assert!(!bundle.adk_chat_stream.runtime_ready());
    assert!(runtime.continuation_supervisor.spawn("after-close", |_| panic!("closed admission")).is_err());
}
