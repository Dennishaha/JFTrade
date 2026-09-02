#![forbid(unsafe_code)]

use std::collections::VecDeque;
use std::error::Error;
use std::fmt::Write;
use std::future::Future;
use std::path::PathBuf;

use jftrade_assistant::rig_adapter::project_completion_request;
use jftrade_assistant::{
    Approval, ApprovalStatus, ArtifactStore, AssistantRuntime, ClaimError, ClaimStore,
    CompletionInput, CompletionPort, CompletionTurn, InputAnswer, InputRequestDraft,
    ProviderFailure, RunStatus, Session, TaskGraph, ToolCall, ToolClaimRequest,
    ToolIdempotencyMode, VersionedArtifact, WorkflowTask,
};
use jftrade_engine::stage6::Stage6Assembly;
use jftrade_kernel::WireTimestamp;
use serde::{Deserialize, Serialize};
use serde_json::{Value, json};
use sha2::{Digest, Sha256};

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct Stage6Input {
    version: String,
    now: WireTimestamp,
    session: Session,
    statuses: Vec<RunStatus>,
    transitions: Vec<TransitionCase>,
    completion_input: CompletionInput,
    approval: ApprovalScenario,
    input: InputScenario,
    invalid_inputs: Vec<InputRequestDraft>,
    claims: ClaimScenario,
    workflow_tasks: Vec<WorkflowTask>,
    invalid_workflows: Vec<Vec<WorkflowTask>>,
    artifacts: Vec<VersionedArtifact>,
    provider_script: Vec<ProviderStep>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct TransitionCase {
    from: RunStatus,
    to: RunStatus,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct ApprovalScenario {
    run_id: String,
    approval: Approval,
    tool_call: ToolCall,
    approved: bool,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct InputScenario {
    run_id: String,
    request_id: String,
    function_call_id: String,
    draft: InputRequestDraft,
    answers: Vec<InputAnswer>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct ClaimScenario {
    run_id: String,
    first_owner: String,
    second_owner: String,
    third_owner: String,
    fourth_owner: String,
    start_unix_ms: i64,
    ttl_ms: i64,
    replay_safe: ToolClaimSeed,
    fail_closed: ToolClaimSeed,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct ToolClaimSeed {
    idempotency_key: String,
    tool_name: String,
    input: Value,
}

#[derive(Clone, Deserialize)]
#[serde(tag = "kind", rename_all = "snake_case", deny_unknown_fields)]
enum ProviderStep {
    Failure { failure: ProviderFailure },
    Turn { turn: CompletionTurn },
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct Stage6Output {
    version: String,
    statuses: Vec<Value>,
    transitions: Vec<Value>,
    rig: Value,
    approval: Value,
    input: Value,
    invalid_inputs: Vec<Value>,
    claims: Value,
    workflow: Value,
    artifacts: Value,
    provider: Value,
}

struct FixedProvider(VecDeque<ProviderStep>);

impl CompletionPort for FixedProvider {
    fn complete(
        &mut self,
        _input: CompletionInput,
    ) -> impl Future<Output = Result<CompletionTurn, ProviderFailure>> + Send {
        std::future::ready(
            match self
                .0
                .pop_front()
                .expect("fixed provider transcript exhausted")
            {
                ProviderStep::Failure { failure } => Err(failure),
                ProviderStep::Turn { turn } => Ok(turn),
            },
        )
    }
}

#[tokio::main]
async fn main() {
    if let Err(error) = execute().await {
        eprintln!("jftrade-stage6-shadow: {error}");
        std::process::exit(1);
    }
}

async fn execute() -> Result<(), Box<dyn Error>> {
    let input_path = parse_input(std::env::args().skip(1))?;
    let input: Stage6Input = serde_json::from_slice(&std::fs::read(input_path)?)?;
    let statuses = input
        .statuses
        .iter()
        .map(|status| json!({"status": status, "terminal": status.is_terminal()}))
        .collect();
    let transitions = input
        .transitions
        .iter()
        .enumerate()
        .map(|(index, case)| transition_result(&input.session, input.now, index, case))
        .collect();
    let rig = serde_json::to_value(project_completion_request(&input.completion_input)?)?;
    let approval = run_approval(&input.session, input.now, input.approval)?;
    let (input_result, invalid_inputs) =
        run_input(&input.session, input.now, input.input, input.invalid_inputs)?;
    let claims = run_claims(input.claims)?;
    let workflow = run_workflow(input.workflow_tasks, input.invalid_workflows)?;
    let artifacts = run_artifacts(input.artifacts)?;
    let provider = run_provider(
        input.session,
        input.now,
        input.completion_input,
        input.provider_script,
    )
    .await?;
    println!(
        "{}",
        serde_json::to_string(&Stage6Output {
            version: input.version,
            statuses,
            transitions,
            rig,
            approval,
            input: input_result,
            invalid_inputs,
            claims,
            workflow,
            artifacts,
            provider,
        })?
    );
    Ok(())
}

fn transition_result(
    session: &Session,
    now: WireTimestamp,
    index: usize,
    case: &TransitionCase,
) -> Value {
    let mut runtime = AssistantRuntime::default();
    runtime.save_session(session.clone());
    let run_id = format!("transition-{index}");
    runtime
        .create_run(&run_id, &session.id, &session.agent_id, now)
        .expect("fixture session");
    if case.from != RunStatus::Running {
        runtime
            .transition(&run_id, case.from, now)
            .expect("fixture transition source");
    }
    match runtime.transition(&run_id, case.to, now) {
        Ok(result) => json!({
            "from": case.from,
            "to": case.to,
            "accepted": true,
            "changed": result.changed,
            "status": result.current,
        }),
        Err(error) => json!({
            "from": case.from,
            "to": case.to,
            "accepted": false,
            "error": error.to_string(),
            "status": case.from,
        }),
    }
}

fn run_approval(
    session: &Session,
    now: WireTimestamp,
    scenario: ApprovalScenario,
) -> Result<Value, Box<dyn Error>> {
    let mut runtime = AssistantRuntime::default();
    runtime.save_session(session.clone());
    runtime.create_run(&scenario.run_id, &session.id, &session.agent_id, now)?;
    let approval_id = scenario.approval.id.clone();
    runtime.request_approval(scenario.approval, scenario.tool_call, now)?;
    let checkpoint = runtime.checkpoint_json()?;
    let persisted = AssistantRuntime::restore(&checkpoint)?;
    let persisted_run = &persisted.checkpoint().runs[&scenario.run_id];
    let persisted_status = persisted_run.status;
    let persisted_pending = persisted_run
        .pending_approvals
        .iter()
        .filter(|approval| approval.status == ApprovalStatus::Pending)
        .count();
    let first = runtime.resolve_approval(&scenario.run_id, &approval_id, scenario.approved, now)?;
    let replay =
        runtime.resolve_approval(&scenario.run_id, &approval_id, scenario.approved, now)?;
    let run = &runtime.checkpoint().runs[&scenario.run_id];
    Ok(json!({
        "persistedStatus": persisted_status,
        "persistedPending": persisted_pending,
        "checkpointSha256": sha256_hex(&checkpoint),
        "firstResolutionChanged": first,
        "replayResolutionChanged": replay,
        "status": run.status,
        "approvalStatus": run.pending_approvals[0].status,
        "toolStatus": run.tool_calls[0].status,
        "requiresUser": run.tool_calls[0].requires_user,
        "auditKinds": runtime.checkpoint().audit.iter().map(|event| event.kind.clone()).collect::<Vec<_>>(),
    }))
}

fn run_input(
    session: &Session,
    now: WireTimestamp,
    scenario: InputScenario,
    invalid_drafts: Vec<InputRequestDraft>,
) -> Result<(Value, Vec<Value>), Box<dyn Error>> {
    let mut runtime = AssistantRuntime::default();
    runtime.save_session(session.clone());
    runtime.create_run(&scenario.run_id, &session.id, &session.agent_id, now)?;
    let request = runtime.request_input(
        &scenario.run_id,
        &scenario.request_id,
        &scenario.function_call_id,
        scenario.draft,
        now,
    )?;
    let checkpoint = runtime.checkpoint_json()?;
    let persisted = AssistantRuntime::restore(&checkpoint)?;
    let first = runtime.answer_input(
        &scenario.run_id,
        &scenario.request_id,
        scenario.answers.clone(),
        now,
    )?;
    let replay = runtime.answer_input(
        &scenario.run_id,
        &scenario.request_id,
        scenario.answers,
        now,
    )?;
    let run = &runtime.checkpoint().runs[&scenario.run_id];
    let result = json!({
        "decisionKind": request.decision_kind,
        "persistedStatus": persisted.checkpoint().runs[&scenario.run_id].status,
        "persistedRequestStatus": persisted.checkpoint().runs[&scenario.run_id].input_requests[0].status,
        "checkpointSha256": sha256_hex(&checkpoint),
        "firstResolutionChanged": first,
        "replayResolutionChanged": replay,
        "status": run.status,
        "activeRequest": run.input_request,
        "requestStatus": run.input_requests[0].status,
        "answers": run.input_requests[0].answers,
        "auditKinds": runtime.checkpoint().audit.iter().map(|event| event.kind.clone()).collect::<Vec<_>>(),
    });
    let invalid_inputs = invalid_drafts
        .into_iter()
        .enumerate()
        .map(|(index, draft)| {
            let result = runtime.request_input(
                &scenario.run_id,
                format!("invalid-{index}"),
                format!("invalid-call-{index}"),
                draft,
                now,
            );
            match result {
                Ok(_) => json!({"accepted": true}),
                Err(error) => json!({"accepted": false, "error": error.to_string()}),
            }
        })
        .collect();
    Ok((result, invalid_inputs))
}

fn run_claims(scenario: ClaimScenario) -> Result<Value, Box<dyn Error>> {
    let first_at = scenario.start_unix_ms;
    let takeover_at = first_at + scenario.ttl_ms + 1;
    let replay_takeover_at = takeover_at + scenario.ttl_ms + 1;
    let fail_closed_at = replay_takeover_at + scenario.ttl_ms + 3;
    let mut store = ClaimStore::default();
    let first = store.claim_run(
        &scenario.run_id,
        &scenario.first_owner,
        first_at,
        scenario.ttl_ms,
    )?;
    let held = store
        .claim_run(
            &scenario.run_id,
            &scenario.second_owner,
            first_at + 1,
            scenario.ttl_ms,
        )
        .expect_err("active lease must be held");
    let checkpoint = store.checkpoint_json()?;
    let mut store = ClaimStore::restore(&checkpoint)?;
    let second = store.claim_run(
        &scenario.run_id,
        &scenario.second_owner,
        takeover_at,
        scenario.ttl_ms,
    )?;
    let replay_first = store.claim_tool(tool_request(
        &scenario,
        &scenario.replay_safe,
        &second,
        ToolIdempotencyMode::ReplaySafe,
        takeover_at,
    ))?;
    let in_flight = store
        .claim_tool(tool_request(
            &scenario,
            &scenario.replay_safe,
            &second,
            ToolIdempotencyMode::ReplaySafe,
            takeover_at + 1,
        ))
        .expect_err("active invocation must remain in flight");
    let third = store.claim_run(
        &scenario.run_id,
        &scenario.third_owner,
        replay_takeover_at,
        scenario.ttl_ms,
    )?;
    let replay_takeover = store.claim_tool(tool_request(
        &scenario,
        &scenario.replay_safe,
        &third,
        ToolIdempotencyMode::ReplaySafe,
        replay_takeover_at,
    ))?;
    store.complete_tool(
        &replay_takeover,
        json!({"price": "100.00"}),
        replay_takeover_at + 1,
    )?;
    let replay = store.claim_tool(tool_request(
        &scenario,
        &scenario.replay_safe,
        &third,
        ToolIdempotencyMode::ReplaySafe,
        replay_takeover_at + 2,
    ))?;
    let fail_closed_first = store.claim_tool(tool_request(
        &scenario,
        &scenario.fail_closed,
        &third,
        ToolIdempotencyMode::FailClosed,
        replay_takeover_at + 2,
    ))?;
    let fourth = store.claim_run(
        &scenario.run_id,
        &scenario.fourth_owner,
        fail_closed_at,
        scenario.ttl_ms,
    )?;
    let outcome_unknown = store
        .claim_tool(tool_request(
            &scenario,
            &scenario.fail_closed,
            &fourth,
            ToolIdempotencyMode::FailClosed,
            fail_closed_at,
        ))
        .expect_err("fail-closed stale invocation must be unknown");
    let final_checkpoint = store.checkpoint_json()?;
    Ok(json!({
        "firstLeaseToken": first.fencing_token,
        "heldError": claim_error_code(&held),
        "restoredCheckpointSha256": sha256_hex(&checkpoint),
        "takeoverLeaseToken": second.fencing_token,
        "firstToolToken": replay_first.fencing_token,
        "inFlightError": claim_error_code(&in_flight),
        "takeoverToolToken": replay_takeover.fencing_token,
        "replay": replay,
        "failClosedFirstToken": fail_closed_first.fencing_token,
        "outcomeUnknownError": claim_error_code(&outcome_unknown),
        "finalCheckpointSha256": sha256_hex(&final_checkpoint),
        "invocations": store.checkpoint().tool_invocations.values().collect::<Vec<_>>(),
    }))
}

fn tool_request(
    scenario: &ClaimScenario,
    seed: &ToolClaimSeed,
    lease: &jftrade_assistant::RunLease,
    mode: ToolIdempotencyMode,
    now_unix_ms: i64,
) -> ToolClaimRequest {
    ToolClaimRequest {
        run_id: scenario.run_id.clone(),
        idempotency_key: seed.idempotency_key.clone(),
        tool_name: seed.tool_name.clone(),
        owner_id: lease.owner_id.clone(),
        run_lease_token: lease.fencing_token,
        input: seed.input.clone(),
        mode,
        now_unix_ms,
        ttl_ms: scenario.ttl_ms,
    }
}

fn claim_error_code(error: &ClaimError) -> &'static str {
    match error {
        ClaimError::Incomplete => "INCOMPLETE",
        ClaimError::InvalidTtl => "INVALID_TTL",
        ClaimError::RunLeaseHeld => "RUN_LEASE_HELD",
        ClaimError::RunLeaseLost => "RUN_LEASE_LOST",
        ClaimError::ToolKeyReused => "TOOL_KEY_REUSED",
        ClaimError::ToolInvocationInFlight => "TOOL_INVOCATION_IN_FLIGHT",
        ClaimError::ToolOutcomeUnknown => "TOOL_OUTCOME_UNKNOWN",
        ClaimError::ToolInvocationLost => "TOOL_INVOCATION_LOST",
        ClaimError::InvalidCheckpoint(_) => "INVALID_CHECKPOINT",
    }
}

fn run_workflow(
    tasks: Vec<WorkflowTask>,
    invalid_workflows: Vec<Vec<WorkflowTask>>,
) -> Result<Value, Box<dyn Error>> {
    let mut graph = TaskGraph::new(tasks)?;
    let first_ready = graph.ready_task().map(|task| task.id);
    if let Some(task_id) = first_ready.as_deref() {
        graph.claim(task_id)?;
        graph.complete(task_id, "first complete")?;
    }
    let second_ready = graph.ready_task().map(|task| task.id);
    if let Some(task_id) = second_ready.as_deref() {
        graph.claim(task_id)?;
    }
    let invalid = invalid_workflows
        .into_iter()
        .map(|tasks| match TaskGraph::new(tasks) {
            Ok(_) => json!({"accepted": true}),
            Err(error) => json!({"accepted": false, "error": error.to_string()}),
        })
        .collect::<Vec<_>>();
    Ok(json!({
        "firstReady": first_ready,
        "secondReady": second_ready,
        "tasks": graph.tasks(),
        "invalid": invalid,
    }))
}

fn run_artifacts(artifacts: Vec<VersionedArtifact>) -> Result<Value, Box<dyn Error>> {
    let mut store = ArtifactStore::default();
    for artifact in &artifacts {
        store.save(artifact.clone())?;
    }
    let snapshot = store.snapshot();
    let restored = ArtifactStore::restore(snapshot.clone())?;
    let first = artifacts
        .first()
        .ok_or("artifact fixture must not be empty")?;
    let latest = restored.latest(&first.session_id, &first.namespace, &first.filename);
    let version_one = restored.load(&first.session_id, &first.namespace, &first.filename, 1);
    Ok(json!({
        "versions": snapshot.values().map(Vec::len).sum::<usize>(),
        "latest": latest,
        "versionOne": version_one,
        "snapshotSha256": sha256_hex(&serde_json::to_vec(&snapshot)?),
    }))
}

async fn run_provider(
    session: Session,
    now: WireTimestamp,
    completion_input: CompletionInput,
    script: Vec<ProviderStep>,
) -> Result<Value, Box<dyn Error>> {
    let mut assembly = Stage6Assembly::default();
    assembly.runtime_mut().save_session(session.clone());
    assembly
        .runtime_mut()
        .create_run("provider-run", &session.id, &session.agent_id, now)?;
    let mut provider = FixedProvider(VecDeque::from(script));
    let outcome = assembly
        .execute_turn(&mut provider, "provider-run", completion_input, 2, now)
        .await?;
    let checkpoint = assembly.runtime().checkpoint_json()?;
    let restored = AssistantRuntime::restore(&checkpoint)?;
    let run = &restored.checkpoint().runs["provider-run"];
    Ok(json!({
        "attempts": outcome.attempts,
        "deltas": outcome.deltas,
        "toolRequests": outcome.turn.tool_requests,
        "providerResponseId": outcome.turn.provider_response_id,
        "status": run.status,
        "message": run.message,
        "degraded": run.degraded,
        "errorCode": run.error_code,
        "failureReason": run.failure_reason,
        "usage": run.usage,
        "auditKinds": restored.checkpoint().audit.iter().map(|event| event.kind.clone()).collect::<Vec<_>>(),
        "checkpointSha256": sha256_hex(&checkpoint),
    }))
}

fn sha256_hex(bytes: &[u8]) -> String {
    let digest = Sha256::digest(bytes);
    let mut encoded = String::with_capacity(digest.len() * 2);
    for byte in digest {
        write!(&mut encoded, "{byte:02x}").expect("writing to a string cannot fail");
    }
    encoded
}

fn parse_input(arguments: impl Iterator<Item = String>) -> Result<PathBuf, Box<dyn Error>> {
    let mut arguments = arguments;
    match (arguments.next().as_deref(), arguments.next()) {
        (Some("--input"), Some(path)) if arguments.next().is_none() => Ok(path.into()),
        _ => Err("usage: jftrade-stage6-shadow --input <fixture.json>".into()),
    }
}
