package adk

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/assistant/engine/completionreview"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

const (
	completionReviewReasonComplete          = completionreview.ReasonComplete
	completionReviewReasonMissingActionPlan = completionreview.ReasonMissingActionPlan
)

func (r *Runtime) maybeReviewChatCompletion(
	ctx context.Context,
	agent Agent,
	run Run,
	execution *googleADKExecution,
) (Run, bool) {
	if r == nil || execution == nil || strings.TrimSpace(run.ID) == "" || agent.ID != DefaultBuiltinAgentID ||
		normalizeAgentDefaultWorkMode(agent.WorkMode) != WorkModeChat || strings.TrimSpace(run.ParentRunID) != "" {
		return run, false
	}
	if r.completionReviews == nil {
		return run, false
	}
	outcome := r.completionReviews.Once(run.ID, func() completionreview.Outcome {
		return r.performCompletionReview(ctx, agent, run, execution.ResultForRun(run.ID))
	})
	if !outcome.Appended {
		return run, false
	}
	if r.completionReviews.MarkApplied(run.ID, execution) {
		jftradeErr := execution.appendVisibleTextForRun(run.ID, "\n\n"+outcome.Continuation, "")
		besteffort.LogError(jftradeErr)
	}
	return run, true
}

func (r *Runtime) performCompletionReview(
	ctx context.Context,
	agent Agent,
	run Run,
	result assistantExecutionResult,
) completionreview.Outcome {
	reasonCode, prompt, err := completionreview.Prepare(agent, run, result)
	if reasonCode != "" {
		outcome := completionreview.Outcome{Outcome: "skipped", ReasonCode: reasonCode}
		r.auditCompletionReview(ctx, run, outcome)
		return outcome
	}
	if err != nil {
		outcome := completionreview.Outcome{Outcome: "failed", ReasonCode: "prompt_encoding_failed"}
		r.auditCompletionReview(ctx, run, outcome)
		return outcome
	}
	review, durationMs, failureCode := r.requestCompletionReview(ctx, agent, prompt)
	if failureCode != "" {
		outcome := completionreview.Outcome{Outcome: "failed", ReasonCode: failureCode, DurationMs: durationMs}
		r.auditCompletionReview(ctx, run, outcome)
		return outcome
	}
	outcome := completionreview.Decide(review, durationMs)
	r.auditCompletionReview(ctx, run, outcome)
	return outcome
}

func (r *Runtime) requestCompletionReview(
	ctx context.Context,
	agent Agent,
	prompt string,
) (completionreview.Response, int64, string) {
	startedAt := time.Now()
	provider, err := r.effectiveProvider(ctx, agent.ProviderID)
	if err != nil || !provider.Enabled {
		return completionreview.Response{}, time.Since(startedAt).Milliseconds(), "provider_unavailable"
	}
	apiKey, hasKey, err := r.store.ProviderAPIKey(provider.ID)
	if err != nil || !hasKey || strings.TrimSpace(apiKey) == "" {
		return completionreview.Response{}, time.Since(startedAt).Milliseconds(), "provider_unavailable"
	}
	reviewCtx, cancel := context.WithTimeout(ctx, completionreview.Timeout)
	defer cancel()
	generator := r.completionReviewText
	if generator == nil {
		generator = r.responses.generateCompletionReview
	}
	generated, err := generator(
		reviewCtx,
		provider,
		apiKey,
		defaultString(agent.Model, provider.Model),
		completionreview.SystemInstruction,
		prompt,
	)
	durationMs := time.Since(startedAt).Milliseconds()
	if err != nil {
		reasonCode := "provider_error"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(reviewCtx.Err(), context.DeadlineExceeded) {
			reasonCode = "timeout"
		}
		return completionreview.Response{}, durationMs, reasonCode
	}
	review, err := completionreview.Parse(generated.Text)
	if err != nil {
		return completionreview.Response{}, durationMs, "invalid_response"
	}
	return review, durationMs, ""
}

func (r *Runtime) auditCompletionReview(ctx context.Context, run Run, outcome completionreview.Outcome) {
	r.audit(ctx, "run.completion_review", run.ID, "Chat completion review finished.", map[string]any{
		"runId": run.ID, "agentId": run.AgentID, "outcome": outcome.Outcome,
		"reasonCode": outcome.ReasonCode, "durationMs": outcome.DurationMs,
		"confidence": outcome.Confidence, "appended": outcome.Appended,
	})
}

func reviewedExecutionResult(execution *googleADKExecution, runID string, appended bool) assistantExecutionResult {
	result := execution.ResultForRun(runID)
	if appended {
		result.SourceEventID = ""
		result.SyntheticKind = "completion_review"
	}
	return result
}

func (r *Runtime) clearCompletionReview(runID string) {
	if r == nil || r.completionReviews == nil {
		return
	}
	r.completionReviews.Clear(strings.TrimSpace(runID))
}
