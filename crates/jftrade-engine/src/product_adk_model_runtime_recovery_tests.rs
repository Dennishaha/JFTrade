use super::{
    AdkChatPortError, DurableRecoveryStatus, DurableRunRecoverySupervisor,
    is_recovery_infrastructure_error, record_recovery_resume_failure,
};
use jftrade_store_sqlite::{AdkSessionStore, AdkStore, CreateAdkRunParams, initialize_current};
use rusqlite::Connection;
use serde_json::json;
use std::collections::BTreeMap;
use std::fs::{self, File};
use std::sync::{Arc, Weak};
use tempfile::tempdir;

#[test]
fn successful_supervisor_start_is_reported_as_ready() {
    let supervisor = DurableRunRecoverySupervisor::start(Weak::new());
    assert!(supervisor.startup_error().is_none());
    assert!(supervisor.is_ready());
    let health = supervisor.health_snapshot();
    assert_eq!(health.status, DurableRecoveryStatus::Ready);
    assert!(health.healthy);
    assert!(health.running);
    supervisor.shutdown();
    assert!(!supervisor.is_ready());
}

#[test]
fn failed_recovery_scan_marks_readiness_degraded_until_next_success() {
    let supervisor = DurableRunRecoverySupervisor::start(Weak::new());

    supervisor.record_scan_failure("ADK storage unavailable: locked");
    let failed = supervisor.health_snapshot();
    assert_eq!(failed.status, DurableRecoveryStatus::Degraded);
    assert!(!failed.healthy);
    assert!(failed.running);
    assert_eq!(
        failed.last_error.as_deref(),
        Some("ADK storage unavailable: locked")
    );
    assert!(failed.checked_at.is_some());
    assert!(!supervisor.is_ready());

    supervisor.record_scan_success();
    let recovered = supervisor.health_snapshot();
    assert_eq!(recovered.status, DurableRecoveryStatus::Ready);
    assert!(recovered.healthy);
    assert!(recovered.running);
    assert!(recovered.last_success_at.is_some());
    assert!(recovered.last_error.is_none());
    assert!(supervisor.is_ready());

    supervisor.shutdown();
}

#[test]
fn shutdown_marks_recovery_unready_and_ignores_late_scan_success() {
    let supervisor = DurableRunRecoverySupervisor::start(Weak::new());
    supervisor.record_scan_failure("temporary scan failure");

    supervisor.shutdown();
    supervisor.record_scan_success();

    let stopped = supervisor.health_snapshot();
    assert_eq!(stopped.status, DurableRecoveryStatus::Shutdown);
    assert!(!stopped.healthy);
    assert!(!stopped.running);
    assert_eq!(
        stopped.last_error.as_deref(),
        Some("ADK durable recovery supervisor stopped")
    );
    assert!(!supervisor.is_ready());
}

#[test]
fn per_run_recovery_errors_do_not_degrade_scanner_readiness() {
    let supervisor = DurableRunRecoverySupervisor::start(Weak::new());
    let per_run_errors = [
        AdkChatPortError::Unavailable("persisted ADK run has no resumable request".to_owned()),
        AdkChatPortError::Unavailable("persisted ADK run payload must be an object".to_owned()),
    ];

    for error in &per_run_errors {
        assert!(!is_recovery_infrastructure_error(error));
        record_recovery_resume_failure(Some(&supervisor), error);
    }

    let health = supervisor.health_snapshot();
    assert_eq!(health.status, DurableRecoveryStatus::Ready);
    assert!(health.healthy);
    assert!(health.running);
    assert!(supervisor.is_ready());
    supervisor.shutdown();
}

#[test]
fn infrastructure_resume_error_degrades_until_a_successful_scan() {
    let supervisor = DurableRunRecoverySupervisor::start(Weak::new());
    let error = AdkChatPortError::Unavailable("ADK storage unavailable: locked".to_owned());

    assert!(is_recovery_infrastructure_error(&error));
    record_recovery_resume_failure(Some(&supervisor), &error);
    assert_eq!(
        supervisor.health_snapshot().status,
        DurableRecoveryStatus::Degraded
    );
    assert!(!supervisor.is_ready());

    supervisor.record_scan_success();
    assert!(supervisor.is_ready());
    supervisor.shutdown();
}

#[test]
fn non_resumable_running_run_does_not_make_runtime_unready() {
    let directory = tempdir().expect("temporary directory");
    let adk_path = directory.path().join("adk.db");
    let session_path = directory.path().join("adk-session.db");
    let settings_path = directory.path().join("settings.json");
    File::create(&adk_path).expect("create ADK database");
    File::create(&session_path).expect("create ADK session database");
    fs::write(&settings_path, b"{}").expect("create settings");
    initialize_current(
        &Connection::open(&adk_path).expect("initialize ADK database"),
        "adk",
    )
    .expect("initialize ADK schema");
    initialize_current(
        &Connection::open(&session_path).expect("initialize ADK session database"),
        "adk-session",
    )
    .expect("initialize ADK session schema");
    fs::create_dir_all(directory.path().join("secrets")).expect("create secrets directory");
    fs::write(
        directory.path().join("secrets/adk-secrets.json"),
        br#"{"provider-recovery":"test-key"}"#,
    )
    .expect("write provider secret");

    let store = Arc::new(AdkStore::open(&adk_path).expect("open ADK store"));
    let session_store = Arc::new(AdkSessionStore::open(&session_path).expect("open session store"));
    store
        .upsert_provider(
            "provider-recovery",
            &json!({
                "displayName": "Recovery fixture",
                "baseUrl": "https://example.test/v1",
                "model": "fixture-model",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist provider");
    store
        .create_run(CreateAdkRunParams {
            id: "run-non-resumable",
            session_id: "session-non-resumable",
            agent_id: "agent-non-resumable",
            status: "RUNNING",
            client_request_id: "request-non-resumable",
            request_fingerprint: "fingerprint-non-resumable",
            payload_json: r#"{
                "status":"RUNNING",
                "resumeState":"provider_waiting",
                "toolCalls":[{"id":"call-1","status":"RUNNING"}]
            }"#,
        })
        .expect("persist non-resumable run");
    let bindings =
        crate::product::product_production_ports::product_production_ports_adk::PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|d| {
            (
                d.adapter,
                crate::product::product_production_ports::ProductionAdapterBinding::Ready,
            )
        })
        .collect::<BTreeMap<_, _>>();
    let catalog =
        crate::product::product_production_ports::ProductionToolCatalog::from_bindings(&bindings)
            .expect("build tool catalog");
    let runtime = super::super::ProductionAdkChatRuntime::new(
        store,
        session_store,
        &settings_path,
        Arc::new(super::super::RunCancellationRegistry::default()),
        Arc::new(catalog),
    );

    runtime.recover_durable_provider_runs();
    assert!(runtime.runtime_ready());
    assert_eq!(
        runtime
            .recovery_supervisor
            .as_ref()
            .expect("recovery supervisor")
            .health_snapshot()
            .status,
        DurableRecoveryStatus::Ready
    );
    runtime.shutdown();
}
