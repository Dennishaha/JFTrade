package assistant

import (
	"context"
	"strings"
	"time"

	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine"
	workflowrules "github.com/jftrade/jftrade-main/internal/assistant/workflow"
)

func (scheduler *WorkflowScheduler) Start(parent context.Context) {
	if scheduler == nil {
		return
	}
	if parent == nil {
		parent = context.Background()
	}
	scheduler.mu.Lock()
	if scheduler.started || scheduler.stopped {
		scheduler.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	scheduler.cancel = cancel
	scheduler.started = true
	scheduler.wg.Add(1)
	scheduler.mu.Unlock()
	go func() {
		defer scheduler.wg.Done()
		if ctx.Err() != nil {
			return
		}
		scheduler.tick(ctx)
		ticker := time.NewTicker(scheduler.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				scheduler.tick(ctx)
			}
		}
	}()
}

func (scheduler *WorkflowScheduler) Stop() {
	if scheduler == nil {
		return
	}
	scheduler.mu.Lock()
	scheduler.stopped = true
	cancel := scheduler.cancel
	scheduler.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	scheduler.wg.Wait()
}

func (scheduler *WorkflowScheduler) tick(ctx context.Context) {
	if scheduler == nil || scheduler.service == nil || scheduler.service.runtime == nil || scheduler.service.runtime.Store() == nil {
		return
	}
	service := scheduler.service
	store := service.runtime.Store()
	now := time.Now().UTC()
	service.reconcileActiveWorkflowLogs(ctx)
	triggers, err := store.ListDueWorkflowScheduleTriggers(ctx, now.Format(time.RFC3339Nano), defaultWorkflowScheduleBatchSize)
	if err == nil {
		for _, trigger := range triggers {
			workflow, wfErr := service.GetWorkflow(ctx, trigger.WorkflowID)
			if wfErr != nil || workflow.Status != jfadk.WorkflowStatusEnabled {
				trigger.LastError = errorString(wfErr)
				trigger.NextRunAt = workflowrules.NextRunAtString(trigger.Config, now)
				_, _ = store.SaveWorkflowTrigger(ctx, trigger)
				continue
			}
			trigger.LastError = ""
			trigger.LastRunAt = now.Format(time.RFC3339Nano)
			trigger.NextRunAt = workflowrules.NextRunAtString(trigger.Config, now)
			updated, saveErr := store.SaveWorkflowTrigger(ctx, trigger)
			if saveErr == nil {
				trigger = updated
			}
			service.launchWorkflowInvocation(ctx, workflow, trigger, map[string]any{"scheduledAt": now.Format(time.RFC3339Nano)})
		}
	}
	if ctx.Err() != nil {
		return
	}
	scheduler.pollMarketThresholds(ctx, now)
}

func (scheduler *WorkflowScheduler) pollMarketThresholds(ctx context.Context, now time.Time) {
	service := scheduler.service
	if service == nil || service.marketSnapshot == nil || service.runtime == nil || service.runtime.Store() == nil {
		return
	}
	store := service.runtime.Store()
	triggers, err := store.ListEnabledWorkflowTriggersByType(ctx, jfadk.WorkflowTriggerTypeMarketThreshold)
	if err != nil {
		return
	}
	for _, trigger := range triggers {
		events := make([]map[string]any, 0)
		for _, instrumentID := range workflowrules.ConfigStringSlice(trigger.Config, "instrumentIds") {
			snapshot, err := service.marketSnapshot(ctx, instrumentID)
			if err != nil {
				trigger.LastError = err.Error()
				continue
			}
			events = append(events, map[string]any{
				"type":       "market-data.tick",
				"source":     "workflow.poll",
				"entityId":   strings.ToUpper(strings.TrimSpace(instrumentID)),
				"at":         now.Format(time.RFC3339Nano),
				"instrument": map[string]any{"instrumentId": strings.ToUpper(strings.TrimSpace(instrumentID))},
				"payload":    snapshot,
			})
		}
		matches, changed := workflowrules.EvaluateMarketThresholdTrigger(trigger, events, now)
		if changed || trigger.LastError != "" {
			updated, saveErr := store.SaveWorkflowTrigger(ctx, trigger)
			if saveErr == nil {
				trigger = updated
			}
		}
		for _, matched := range matches {
			workflow, wfErr := service.GetWorkflow(ctx, trigger.WorkflowID)
			if wfErr != nil || workflow.Status != jfadk.WorkflowStatusEnabled {
				continue
			}
			service.launchWorkflowInvocation(ctx, workflow, trigger, matched)
		}
	}
}

func (s *Service) invokeWorkflowBackground(workflow jfadk.WorkflowDefinition, trigger jfadk.WorkflowTrigger, matchedEvent map[string]any) {
	s.invokeWorkflowBackgroundContext(context.Background(), workflow, trigger, matchedEvent)
}

func (s *Service) launchWorkflowInvocation(ctx context.Context, workflow jfadk.WorkflowDefinition, trigger jfadk.WorkflowTrigger, matchedEvent map[string]any) bool {
	return s.goWorkflowBackground(ctx, func(runCtx context.Context) {
		s.invokeWorkflowBackgroundContext(runCtx, workflow, trigger, matchedEvent)
	})
}

func (s *Service) invokeWorkflowBackgroundContext(ctx context.Context, workflow jfadk.WorkflowDefinition, trigger jfadk.WorkflowTrigger, matchedEvent map[string]any) {
	ctx, cancel := context.WithTimeout(ctx, jfadk.DefaultRunTimeout+time.Minute)
	defer cancel()
	_, _ = s.invokeWorkflow(ctx, workflow, &trigger, trigger.Type, map[string]any{"event": matchedEvent}, matchedEvent)
}
