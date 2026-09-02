use std::collections::BTreeMap;

use jftrade_kernel::WireTimestamp;
use serde_json::{Value, json};
use thiserror::Error;

use crate::{
    Approval, ApprovalStatus, AssistantCheckpoint, AuditEvent, ChatDelta, InputAnswer, InputOption,
    InputQuestion, InputRequest, InputRequestDraft, InputRequestStatus, Run, RunStatus, RunUsage,
    Session, StreamDelta, ToolCall, ToolCallStatus,
};

const NON_BLOCKING_PROMPTS: &[&str] = &[
    "optional next step",
    "whether to continue",
    "do you want me to continue",
    "would you like me to continue",
    "if you want, i can",
    "which part would you like",
    "what would you like to see first",
    "是否继续",
    "要不要继续",
    "需要我继续",
    "如果需要我可以",
    "你想先做哪项",
    "你更想看哪部分",
    "先看哪部分",
];

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct TransitionResult {
    pub previous: RunStatus,
    pub current: RunStatus,
    pub changed: bool,
}

#[derive(Debug, Error, Eq, PartialEq)]
pub enum RuntimeError {
    #[error("session {0} was not found")]
    SessionNotFound(String),
    #[error("run {0} was not found")]
    RunNotFound(String),
    #[error("approval {0} was not found")]
    ApprovalNotFound(String),
    #[error("input request {0} was not found")]
    InputRequestNotFound(String),
    #[error("run transition from {from:?} to {to:?} is not allowed")]
    InvalidTransition { from: RunStatus, to: RunStatus },
    #[error("approval resolution conflicts with the persisted result")]
    ApprovalConflict,
    #[error("input resolution conflicts with the persisted result")]
    InputConflict,
    #[error("input request is invalid: {0}")]
    InvalidInputRequest(String),
    #[error("input answers do not cover each question exactly once")]
    InvalidInputAnswers,
    #[error("checkpoint is invalid: {0}")]
    InvalidCheckpoint(String),
}

#[derive(Clone, Debug, Default)]
pub struct AssistantRuntime {
    checkpoint: AssistantCheckpoint,
    next_audit_sequence: u64,
}

impl AssistantRuntime {
    pub fn restore(json_bytes: &[u8]) -> Result<Self, RuntimeError> {
        let checkpoint: AssistantCheckpoint = serde_json::from_slice(json_bytes)
            .map_err(|error| RuntimeError::InvalidCheckpoint(error.to_string()))?;
        let next_audit_sequence = checkpoint
            .audit
            .last()
            .map_or(1, |event| event.sequence.saturating_add(1));
        if checkpoint
            .audit
            .windows(2)
            .any(|pair| pair[0].sequence >= pair[1].sequence)
        {
            return Err(RuntimeError::InvalidCheckpoint(
                "audit sequences must be strictly increasing".to_owned(),
            ));
        }
        Ok(Self {
            checkpoint,
            next_audit_sequence,
        })
    }

    pub fn checkpoint_json(&self) -> Result<Vec<u8>, RuntimeError> {
        serde_json::to_vec(&self.checkpoint)
            .map_err(|error| RuntimeError::InvalidCheckpoint(error.to_string()))
    }

    pub fn checkpoint(&self) -> &AssistantCheckpoint {
        &self.checkpoint
    }

    pub fn save_session(&mut self, session: Session) {
        self.checkpoint.sessions.insert(session.id.clone(), session);
    }

    pub fn create_run(
        &mut self,
        id: impl Into<String>,
        session_id: impl Into<String>,
        agent_id: impl Into<String>,
        now: WireTimestamp,
    ) -> Result<Run, RuntimeError> {
        let id = id.into();
        let session_id = session_id.into();
        if !self.checkpoint.sessions.contains_key(&session_id) {
            return Err(RuntimeError::SessionNotFound(session_id));
        }
        let run = Run {
            id: id.clone(),
            session_id,
            agent_id: agent_id.into(),
            status: RunStatus::Running,
            message: String::new(),
            tool_calls: Vec::new(),
            pending_approvals: Vec::new(),
            input_request: None,
            input_requests: Vec::new(),
            workflow_plan: Vec::new(),
            usage: None,
            failure_reason: String::new(),
            error_code: String::new(),
            degraded: false,
            created_at: now,
            updated_at: now,
            completed_at: None,
            cancelled_at: None,
        };
        self.checkpoint.runs.insert(id.clone(), run.clone());
        self.audit(
            "run.started",
            &id,
            "Agent run started.",
            now,
            BTreeMap::new(),
        );
        Ok(run)
    }

    pub fn transition(
        &mut self,
        run_id: &str,
        to: RunStatus,
        now: WireTimestamp,
    ) -> Result<TransitionResult, RuntimeError> {
        let run = self.run_mut(run_id)?;
        let from = run.status;
        if from == to {
            return Ok(TransitionResult {
                previous: from,
                current: to,
                changed: false,
            });
        }
        if !allowed_transition(from, to) {
            return Err(RuntimeError::InvalidTransition { from, to });
        }
        run.status = to;
        run.updated_at = now;
        if to.is_terminal() {
            run.completed_at = Some(now);
        }
        if to == RunStatus::Cancelled {
            run.cancelled_at = Some(now);
        }
        self.audit(
            "run.status",
            run_id,
            "Agent run status changed.",
            now,
            BTreeMap::from([
                ("from".to_owned(), json!(from)),
                ("to".to_owned(), json!(to)),
            ]),
        );
        Ok(TransitionResult {
            previous: from,
            current: to,
            changed: true,
        })
    }

    pub fn request_approval(
        &mut self,
        approval: Approval,
        mut tool_call: ToolCall,
        now: WireTimestamp,
    ) -> Result<(), RuntimeError> {
        let run_id = approval.run_id.clone();
        {
            let run = self.run_mut(&run_id)?;
            if run.status.is_terminal() {
                return Err(RuntimeError::InvalidTransition {
                    from: run.status,
                    to: RunStatus::PendingApproval,
                });
            }
            tool_call.status = ToolCallStatus::PendingApproval;
            tool_call.requires_user = true;
            tool_call.updated_at = now;
            run.tool_calls.push(tool_call);
            run.pending_approvals.push(approval);
            run.status = RunStatus::PendingApproval;
            run.updated_at = now;
        }
        self.audit(
            "approval.pending",
            &run_id,
            "Tool call requires user approval.",
            now,
            BTreeMap::new(),
        );
        Ok(())
    }

    pub fn resolve_approval(
        &mut self,
        run_id: &str,
        approval_id: &str,
        approved: bool,
        now: WireTimestamp,
    ) -> Result<bool, RuntimeError> {
        let run = self.run_mut(run_id)?;
        let desired = if approved {
            ApprovalStatus::Approved
        } else {
            ApprovalStatus::Denied
        };
        let approval = run
            .pending_approvals
            .iter_mut()
            .find(|approval| approval.id == approval_id)
            .ok_or_else(|| RuntimeError::ApprovalNotFound(approval_id.to_owned()))?;
        if approval.status != ApprovalStatus::Pending {
            if approval.status == desired {
                return Ok(false);
            }
            return Err(RuntimeError::ApprovalConflict);
        }
        approval.status = desired;
        approval.updated_at = now;
        if !approved {
            for approval in &mut run.pending_approvals {
                if approval.status == ApprovalStatus::Pending {
                    approval.status = ApprovalStatus::Denied;
                    approval.updated_at = now;
                }
            }
        }
        let still_pending = run
            .pending_approvals
            .iter()
            .any(|approval| approval.status == ApprovalStatus::Pending);
        if !still_pending {
            for call in &mut run.tool_calls {
                if call.status != ToolCallStatus::PendingApproval {
                    continue;
                }
                call.requires_user = false;
                call.status = if approved {
                    ToolCallStatus::Running
                } else {
                    ToolCallStatus::Denied
                };
                call.updated_at = now;
                if !approved {
                    call.completed_at = Some(now);
                }
            }
        }
        run.status = if still_pending {
            RunStatus::PendingApproval
        } else {
            // Go stages both approval and denial continuations as RUNNING. The
            // resumed executor owns the eventual COMPLETED or DENIED terminal.
            RunStatus::Running
        };
        run.updated_at = now;
        self.audit(
            if approved {
                "approval.approved"
            } else {
                "approval.denied"
            },
            run_id,
            "User resolved a tool approval.",
            now,
            BTreeMap::new(),
        );
        Ok(true)
    }

    pub fn request_input(
        &mut self,
        run_id: &str,
        request_id: impl Into<String>,
        function_call_id: impl Into<String>,
        draft: InputRequestDraft,
        now: WireTimestamp,
    ) -> Result<InputRequest, RuntimeError> {
        validate_input_draft(&draft)?;
        let request = {
            let run = self.run_mut(run_id)?;
            if run.status.is_terminal() {
                return Err(RuntimeError::InvalidTransition {
                    from: run.status,
                    to: RunStatus::PendingInput,
                });
            }
            let request = InputRequest {
                id: request_id.into(),
                run_id: run_id.to_owned(),
                agent_id: run.agent_id.clone(),
                function_call_id: function_call_id.into(),
                title: draft.title,
                status: InputRequestStatus::Pending,
                decision_kind: draft.decision_kind,
                blocking_reason: draft.blocking_reason.trim().to_owned(),
                questions: draft
                    .questions
                    .into_iter()
                    .enumerate()
                    .map(|(question_index, question)| {
                        let id = format!("q{}", question_index + 1);
                        InputQuestion {
                            id: id.clone(),
                            question: question.question.trim().to_owned(),
                            options: question
                                .options
                                .into_iter()
                                .enumerate()
                                .map(|(option_index, option)| InputOption {
                                    id: format!("{id}-o{}", option_index + 1),
                                    label: option.label.trim().to_owned(),
                                    description: option.description.trim().to_owned(),
                                    recommended: option.recommended,
                                })
                                .collect(),
                            allow_other: question.allow_other,
                        }
                    })
                    .collect(),
                answers: Vec::new(),
                created_at: now,
                updated_at: now,
                answered_at: None,
            };
            run.status = RunStatus::PendingInput;
            run.input_request = Some(request.clone());
            run.input_requests.push(request.clone());
            run.updated_at = now;
            request
        };
        self.audit(
            "input.pending",
            run_id,
            "Agent run is awaiting required user input.",
            now,
            BTreeMap::from([(
                "decisionKind".to_owned(),
                serde_json::to_value(request.decision_kind).unwrap_or(Value::Null),
            )]),
        );
        Ok(request)
    }

    pub fn answer_input(
        &mut self,
        run_id: &str,
        request_id: &str,
        answers: Vec<InputAnswer>,
        now: WireTimestamp,
    ) -> Result<bool, RuntimeError> {
        let run = self.run_mut(run_id)?;
        let index = run
            .input_requests
            .iter()
            .position(|request| request.id == request_id)
            .ok_or_else(|| RuntimeError::InputRequestNotFound(request_id.to_owned()))?;
        let request = &mut run.input_requests[index];
        if request.status != InputRequestStatus::Pending {
            if request.status == InputRequestStatus::Answered && request.answers == answers {
                return Ok(false);
            }
            return Err(RuntimeError::InputConflict);
        }
        validate_answers(request, &answers)?;
        request.status = InputRequestStatus::Answered;
        request.answers = answers;
        request.updated_at = now;
        request.answered_at = Some(now);
        run.input_request = None;
        run.status = RunStatus::Running;
        run.updated_at = now;
        self.audit(
            "input.answered",
            run_id,
            "Required user input was answered; the original request must continue.",
            now,
            BTreeMap::new(),
        );
        Ok(true)
    }

    pub fn apply_stream_delta(
        &mut self,
        run_id: &str,
        delta: &StreamDelta,
        now: WireTimestamp,
    ) -> Result<ChatDelta, RuntimeError> {
        let run = self.run_mut(run_id)?;
        run.updated_at = now;
        let projected = match delta {
            StreamDelta::Reply { text } => {
                run.message.push_str(text);
                ChatDelta {
                    reply: text.clone(),
                    ..ChatDelta::default()
                }
            }
            StreamDelta::Reasoning { text } => ChatDelta {
                reasoning_content: text.clone(),
                ..ChatDelta::default()
            },
            StreamDelta::ToolProgress { text } => ChatDelta {
                tool_progress: text.clone(),
                ..ChatDelta::default()
            },
        };
        Ok(projected)
    }

    pub fn record_usage(
        &mut self,
        run_id: &str,
        usage: RunUsage,
        now: WireTimestamp,
    ) -> Result<(), RuntimeError> {
        let run = self.run_mut(run_id)?;
        let accumulated = run.usage.get_or_insert_with(RunUsage::default);
        accumulated.model_calls = accumulated.model_calls.saturating_add(usage.model_calls);
        accumulated.tool_calls_total = accumulated
            .tool_calls_total
            .saturating_add(usage.tool_calls_total);
        accumulated.duration_ms = accumulated.duration_ms.saturating_add(usage.duration_ms);
        accumulated.tokens_in = accumulated.tokens_in.saturating_add(usage.tokens_in);
        accumulated.tokens_out = accumulated.tokens_out.saturating_add(usage.tokens_out);
        run.updated_at = now;
        Ok(())
    }

    pub fn record_provider_failure(
        &mut self,
        run_id: &str,
        code: &str,
        message: &str,
        retryable: bool,
        now: WireTimestamp,
    ) -> Result<(), RuntimeError> {
        let run = self.run_mut(run_id)?;
        run.error_code = code.trim().to_owned();
        run.failure_reason = message.trim().to_owned();
        run.updated_at = now;
        if retryable {
            run.degraded = true;
        } else {
            run.status = RunStatus::Failed;
            run.completed_at = Some(now);
        }
        self.audit(
            "provider.error",
            run_id,
            "Completion provider returned an error.",
            now,
            BTreeMap::from([("retryable".to_owned(), json!(retryable))]),
        );
        Ok(())
    }

    pub fn clear_provider_failure(
        &mut self,
        run_id: &str,
        now: WireTimestamp,
    ) -> Result<(), RuntimeError> {
        let run = self.run_mut(run_id)?;
        run.error_code.clear();
        run.failure_reason.clear();
        run.updated_at = now;
        Ok(())
    }

    fn run_mut(&mut self, run_id: &str) -> Result<&mut Run, RuntimeError> {
        self.checkpoint
            .runs
            .get_mut(run_id)
            .ok_or_else(|| RuntimeError::RunNotFound(run_id.to_owned()))
    }

    fn audit(
        &mut self,
        kind: &str,
        subject_id: &str,
        detail: &str,
        now: WireTimestamp,
        metadata: BTreeMap<String, Value>,
    ) {
        let sequence = self.next_audit_sequence;
        self.next_audit_sequence = self.next_audit_sequence.saturating_add(1);
        self.checkpoint.audit.push(AuditEvent {
            sequence,
            id: format!("audit-{sequence}"),
            kind: kind.to_owned(),
            subject_id: subject_id.to_owned(),
            detail: detail.to_owned(),
            metadata,
            created_at: now,
        });
    }
}

fn allowed_transition(from: RunStatus, to: RunStatus) -> bool {
    match from {
        RunStatus::Running => matches!(
            to,
            RunStatus::Completed
                | RunStatus::PendingApproval
                | RunStatus::PendingInput
                | RunStatus::Failed
                | RunStatus::Cancelled
                | RunStatus::TimedOut
                | RunStatus::Paused
        ),
        RunStatus::PendingApproval => matches!(
            to,
            RunStatus::Running | RunStatus::Denied | RunStatus::Cancelled | RunStatus::TimedOut
        ),
        RunStatus::PendingInput => matches!(
            to,
            RunStatus::Running | RunStatus::Cancelled | RunStatus::TimedOut
        ),
        RunStatus::Paused => matches!(
            to,
            RunStatus::Running | RunStatus::Cancelled | RunStatus::TimedOut
        ),
        RunStatus::Completed
        | RunStatus::Failed
        | RunStatus::Denied
        | RunStatus::Cancelled
        | RunStatus::TimedOut => false,
    }
}

fn validate_input_draft(draft: &InputRequestDraft) -> Result<(), RuntimeError> {
    if draft.blocking_reason.trim().is_empty() {
        return Err(RuntimeError::InvalidInputRequest(
            "blockingReason is required".to_owned(),
        ));
    }
    if is_non_blocking_prompt(&draft.blocking_reason) {
        return Err(RuntimeError::InvalidInputRequest(
            "blockingReason describes a non-blocking optional next step".to_owned(),
        ));
    }
    if draft.questions.is_empty() {
        return Err(RuntimeError::InvalidInputRequest(
            "at least one question is required".to_owned(),
        ));
    }
    for question in &draft.questions {
        if question.question.trim().is_empty() || is_non_blocking_prompt(&question.question) {
            return Err(RuntimeError::InvalidInputRequest(
                "question is empty or non-blocking".to_owned(),
            ));
        }
        if !(2..=3).contains(&question.options.len()) {
            return Err(RuntimeError::InvalidInputRequest(
                "each question requires two or three options".to_owned(),
            ));
        }
        if question
            .options
            .iter()
            .any(|option| option.label.trim().is_empty())
        {
            return Err(RuntimeError::InvalidInputRequest(
                "option labels must not be empty".to_owned(),
            ));
        }
    }
    Ok(())
}

fn validate_answers(request: &InputRequest, answers: &[InputAnswer]) -> Result<(), RuntimeError> {
    if answers.len() != request.questions.len() {
        return Err(RuntimeError::InvalidInputAnswers);
    }
    for question in &request.questions {
        let matching = answers
            .iter()
            .filter(|answer| answer.question_id == question.id)
            .collect::<Vec<_>>();
        if matching.len() != 1 {
            return Err(RuntimeError::InvalidInputAnswers);
        }
        let answer = matching[0];
        let selected = !answer.option_id.is_empty()
            && question
                .options
                .iter()
                .any(|option| option.id == answer.option_id);
        let other = question.allow_other && !answer.other_text.trim().is_empty();
        if selected == other {
            return Err(RuntimeError::InvalidInputAnswers);
        }
    }
    Ok(())
}

fn is_non_blocking_prompt(value: &str) -> bool {
    let value = value.trim().to_lowercase();
    NON_BLOCKING_PROMPTS
        .iter()
        .any(|phrase| value.contains(phrase))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{InputOptionDraft, InputQuestionDraft};

    fn timestamp() -> WireTimestamp {
        "2026-08-19T00:00:00Z".parse().expect("timestamp")
    }

    fn runtime() -> AssistantRuntime {
        let now = timestamp();
        let mut runtime = AssistantRuntime::default();
        runtime.save_session(Session {
            id: "session-1".to_owned(),
            agent_id: "agent-1".to_owned(),
            title: "test".to_owned(),
            workflow_id: None,
            created_at: now,
            updated_at: now,
        });
        runtime
            .create_run("run-1", "session-1", "agent-1", now)
            .expect("run");
        runtime
    }

    #[test]
    fn pending_input_is_persisted_and_answer_resume_is_idempotent() {
        let now = timestamp();
        let mut runtime = runtime();
        let draft = InputRequestDraft {
            decision_kind: crate::InputDecisionKind::MaterialTradeoff,
            blocking_reason: "The execution mode changes the result.".to_owned(),
            title: "Choose mode".to_owned(),
            questions: vec![InputQuestionDraft {
                question: "Choose a mode".to_owned(),
                options: vec![
                    InputOptionDraft {
                        label: "Paper".to_owned(),
                        description: String::new(),
                        recommended: true,
                    },
                    InputOptionDraft {
                        label: "Live".to_owned(),
                        description: String::new(),
                        recommended: false,
                    },
                ],
                allow_other: false,
            }],
        };
        runtime
            .request_input("run-1", "input-1", "call-1", draft, now)
            .expect("request input");
        let restored = AssistantRuntime::restore(&runtime.checkpoint_json().expect("checkpoint"))
            .expect("restore");
        assert_eq!(
            restored.checkpoint().runs["run-1"].status,
            RunStatus::PendingInput
        );
        let answers = vec![InputAnswer {
            question_id: "q1".to_owned(),
            option_id: "q1-o1".to_owned(),
            other_text: String::new(),
        }];
        assert!(
            runtime
                .answer_input("run-1", "input-1", answers.clone(), now)
                .expect("answer")
        );
        assert!(
            !runtime
                .answer_input("run-1", "input-1", answers, now)
                .expect("replay answer")
        );
    }

    #[test]
    fn terminal_run_cannot_be_resumed() {
        let now = timestamp();
        let mut runtime = runtime();
        runtime
            .transition("run-1", RunStatus::Completed, now)
            .expect("complete");
        assert_eq!(
            runtime.transition("run-1", RunStatus::Running, now),
            Err(RuntimeError::InvalidTransition {
                from: RunStatus::Completed,
                to: RunStatus::Running,
            })
        );
    }

    #[test]
    fn sibling_approvals_resume_once_after_every_decision() {
        let now = timestamp();
        let mut runtime = runtime();
        for suffix in ["a", "b"] {
            runtime
                .request_approval(
                    Approval {
                        id: format!("approval-{suffix}"),
                        run_id: "run-1".to_owned(),
                        agent_id: "agent-1".to_owned(),
                        tool_name: format!("write.{suffix}"),
                        input: serde_json::json!({}),
                        status: ApprovalStatus::Pending,
                        reason: "write".to_owned(),
                        function_call_id: format!("call-{suffix}"),
                        confirmation_call_id: format!("confirm-{suffix}"),
                        created_at: now,
                        updated_at: now,
                    },
                    ToolCall {
                        id: format!("call-{suffix}"),
                        run_id: "run-1".to_owned(),
                        tool_name: format!("write.{suffix}"),
                        permission: "write".to_owned(),
                        status: ToolCallStatus::Pending,
                        input: serde_json::json!({}),
                        output: None,
                        error: None,
                        requires_user: false,
                        idempotency_key: suffix.to_owned(),
                        created_at: now,
                        updated_at: now,
                        completed_at: None,
                    },
                    now,
                )
                .expect("approval");
        }
        runtime
            .resolve_approval("run-1", "approval-a", true, now)
            .expect("first approval");
        let pending = &runtime.checkpoint().runs["run-1"];
        assert_eq!(pending.status, RunStatus::PendingApproval);
        assert!(
            pending
                .tool_calls
                .iter()
                .all(|call| call.status == ToolCallStatus::PendingApproval)
        );
        runtime
            .resolve_approval("run-1", "approval-b", true, now)
            .expect("second approval");
        let resumed = &runtime.checkpoint().runs["run-1"];
        assert_eq!(resumed.status, RunStatus::Running);
        assert!(
            resumed
                .tool_calls
                .iter()
                .all(|call| call.status == ToolCallStatus::Running && !call.requires_user)
        );
    }
}
