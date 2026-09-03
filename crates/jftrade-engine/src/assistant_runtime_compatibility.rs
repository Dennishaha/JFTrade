use jftrade_assistant::{
    ArtifactStore, AssistantRuntime, ChatDelta, ClaimStore, CompletionInput, CompletionPort,
    CompletionTurn, ProviderFailure, ProviderFailureKind,
};
use jftrade_kernel::WireTimestamp;
use thiserror::Error;

#[derive(Clone, Debug, PartialEq)]
pub struct AssistantRuntimeTurnOutcome {
    pub attempts: usize,
    pub deltas: Vec<ChatDelta>,
    pub turn: CompletionTurn,
}

#[derive(Default)]
pub struct AssistantRuntimeReplay {
    runtime: AssistantRuntime,
    claims: ClaimStore,
    artifacts: ArtifactStore,
}

impl AssistantRuntimeReplay {
    pub fn runtime(&self) -> &AssistantRuntime {
        &self.runtime
    }

    pub fn runtime_mut(&mut self) -> &mut AssistantRuntime {
        &mut self.runtime
    }

    pub fn claims(&self) -> &ClaimStore {
        &self.claims
    }

    pub fn claims_mut(&mut self) -> &mut ClaimStore {
        &mut self.claims
    }

    pub fn artifacts(&self) -> &ArtifactStore {
        &self.artifacts
    }

    pub fn artifacts_mut(&mut self) -> &mut ArtifactStore {
        &mut self.artifacts
    }

    pub async fn execute_turn<P: CompletionPort>(
        &mut self,
        provider: &mut P,
        run_id: &str,
        input: CompletionInput,
        max_attempts: usize,
        now: WireTimestamp,
    ) -> Result<AssistantRuntimeTurnOutcome, AssistantRuntimeReplayError> {
        let max_attempts = max_attempts.max(1);
        for attempt in 1..=max_attempts {
            match provider.complete(input.clone()).await {
                Ok(turn) => {
                    self.runtime.clear_provider_failure(run_id, now)?;
                    let deltas = turn
                        .deltas
                        .iter()
                        .map(|delta| self.runtime.apply_stream_delta(run_id, delta, now))
                        .collect::<Result<Vec<_>, _>>()?;
                    self.runtime.record_usage(run_id, turn.usage.clone(), now)?;
                    return Ok(AssistantRuntimeTurnOutcome {
                        attempts: attempt,
                        deltas,
                        turn,
                    });
                }
                Err(failure)
                    if failure.kind == ProviderFailureKind::Transient && attempt < max_attempts =>
                {
                    self.runtime.record_provider_failure(
                        run_id,
                        &failure.code,
                        &failure.message,
                        true,
                        now,
                    )?;
                }
                Err(failure) => {
                    self.runtime.record_provider_failure(
                        run_id,
                        &failure.code,
                        &failure.message,
                        false,
                        now,
                    )?;
                    return Err(AssistantRuntimeReplayError::Provider(failure));
                }
            }
        }
        unreachable!("at least one provider attempt is always executed")
    }
}

#[derive(Debug, Error)]
pub enum AssistantRuntimeReplayError {
    #[error(transparent)]
    Runtime(#[from] jftrade_assistant::RuntimeError),
    #[error("completion provider failed: {0:?}")]
    Provider(ProviderFailure),
}

#[cfg(test)]
mod tests {
    use std::collections::VecDeque;
    use std::future::Future;

    use jftrade_assistant::{
        CompletionTurn, JftradeMessage, MessageRole, ProviderFailure, ProviderFailureKind,
        RunUsage, Session, StreamDelta,
    };

    use super::*;

    struct FakeProvider(VecDeque<Result<CompletionTurn, ProviderFailure>>);

    impl CompletionPort for FakeProvider {
        fn complete(
            &mut self,
            _input: CompletionInput,
        ) -> impl Future<Output = Result<CompletionTurn, ProviderFailure>> + Send {
            std::future::ready(self.0.pop_front().expect("fixed provider transcript"))
        }
    }

    #[tokio::test]
    async fn composition_recovers_transient_provider_failure_without_losing_stream_or_usage() {
        let now: WireTimestamp = "2026-08-19T00:00:00Z".parse().expect("timestamp");
        let mut assembly = AssistantRuntimeReplay::default();
        assembly.runtime_mut().save_session(Session {
            id: "session".to_owned(),
            agent_id: "agent".to_owned(),
            title: "test".to_owned(),
            workflow_id: None,
            created_at: now,
            updated_at: now,
        });
        assembly
            .runtime_mut()
            .create_run("run", "session", "agent", now)
            .expect("run");
        let mut provider = FakeProvider(VecDeque::from([
            Err(ProviderFailure {
                kind: ProviderFailureKind::Transient,
                code: "NETWORK".to_owned(),
                message: "temporary".to_owned(),
            }),
            Ok(CompletionTurn {
                deltas: vec![StreamDelta::Reply {
                    text: "done".to_owned(),
                }],
                tool_requests: Vec::new(),
                usage: RunUsage {
                    model_calls: 1,
                    tokens_in: 2,
                    tokens_out: 1,
                    ..RunUsage::default()
                },
                provider_response_id: Some("response-1".to_owned()),
            }),
        ]));
        let outcome = assembly
            .execute_turn(
                &mut provider,
                "run",
                CompletionInput {
                    provider_id: "fake".to_owned(),
                    model: "fixed".to_owned(),
                    messages: vec![JftradeMessage {
                        role: MessageRole::User,
                        content: "hello".to_owned(),
                    }],
                    tools: Vec::new(),
                },
                2,
                now,
            )
            .await
            .expect("retry succeeds");
        assert_eq!(outcome.attempts, 2);
        let run = &assembly.runtime().checkpoint().runs["run"];
        assert_eq!(run.message, "done");
        assert_eq!(run.usage.as_ref().expect("usage").model_calls, 1);
        assert!(run.degraded);
        assert!(run.error_code.is_empty());
    }
}
