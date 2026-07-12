package assistant

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestWorkflowTriggerValidationAndBoundaryHelpers(t *testing.T) {
	for _, tc := range []struct {
		name    string
		trigger jfadk.WorkflowTrigger
		want    string
	}{
		{
			name:    "missing workflow id",
			trigger: jfadk.WorkflowTrigger{Type: jfadk.WorkflowTriggerTypeManual},
			want:    "workflowId",
		},
		{
			name: "schedule missing cron",
			trigger: jfadk.WorkflowTrigger{
				WorkflowID: "workflow", Type: jfadk.WorkflowTriggerTypeSchedule,
				Config: map[string]any{},
			},
			want: "cron",
		},
		{
			name: "schedule six fields",
			trigger: jfadk.WorkflowTrigger{
				WorkflowID: "workflow", Type: jfadk.WorkflowTriggerTypeSchedule,
				Config: map[string]any{"cron": "0 0 8 * * 1"},
			},
			want: "5 fields",
		},
		{
			name: "schedule invalid timezone",
			trigger: jfadk.WorkflowTrigger{
				WorkflowID: "workflow", Type: jfadk.WorkflowTriggerTypeSchedule,
				Config: map[string]any{"cron": "0 8 * * 1-5", "timezone": "Mars/Base"},
			},
			want: "timezone",
		},
		{
			name: "market missing instruments",
			trigger: jfadk.WorkflowTrigger{
				WorkflowID: "workflow", Type: jfadk.WorkflowTriggerTypeMarketThreshold,
				Config: map[string]any{"value": 100},
			},
			want: "instrumentIds",
		},
		{
			name: "unsupported type",
			trigger: jfadk.WorkflowTrigger{
				WorkflowID: "workflow", Type: "unknown",
			},
			want: "unsupported",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateWorkflowTrigger(tc.trigger); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("validateWorkflowTrigger err = %v, want containing %q", err, tc.want)
			}
		})
	}

	if _, err := nextWorkflowScheduleRun(map[string]any{"cron": "0 8 * * 1-5", "timezone": "Mars/Base"}, time.Now()); err == nil {
		t.Fatal("nextWorkflowScheduleRun invalid timezone succeeded, want error")
	}
	if _, err := renderWorkflowTemplate(`{{ call .notAFunction }}`, map[string]any{}); err == nil {
		t.Fatal("renderWorkflowTemplate invalid call succeeded, want execute error")
	}

	now := time.Date(2026, 7, 1, 1, 0, 0, 0, time.UTC)
	if matches, changed := evaluateMarketThresholdTrigger(jfadk.WorkflowTrigger{Config: map[string]any{}}, []map[string]any{{"entityId": "US.AAPL"}}, now); len(matches) != 0 || changed {
		t.Fatalf("evaluateMarketThresholdTrigger without instruments matches=%+v changed=%v, want none/false", matches, changed)
	}
	if matches, changed := evaluateMarketThresholdTrigger(jfadk.WorkflowTrigger{Config: map[string]any{"instrumentIds": []string{"US.AAPL"}}}, []map[string]any{{"entityId": "US.AAPL"}}, now); len(matches) != 0 || changed {
		t.Fatalf("evaluateMarketThresholdTrigger without threshold matches=%+v changed=%v, want none/false", matches, changed)
	}
	coolingTrigger := jfadk.WorkflowTrigger{Config: map[string]any{
		"instrumentIds": []string{"US.AAPL"},
		"value":         100,
		"edge":          "above",
		"cooldownSec":   60,
		"state": map[string]any{
			"lastTriggeredAt": map[string]any{"US.AAPL": now.Format(time.RFC3339Nano)},
		},
	}}
	matches, changed := evaluateMarketThresholdTrigger(coolingTrigger, []map[string]any{{"entityId": "US.AAPL", "snapshot": map[string]any{"price": 101}}}, now.Add(10*time.Second))
	if len(matches) != 0 || !changed {
		t.Fatalf("cooldown threshold matches=%+v changed=%v, want changed without firing", matches, changed)
	}
	if matches, changed := evaluateMarketThresholdTrigger(jfadk.WorkflowTrigger{Config: map[string]any{
		"instrumentIds": []string{"US.AAPL"}, "value": 100,
	}}, []map[string]any{{"entityId": "US.AAPL", "snapshot": map[string]any{"bad": 101}}}, now); len(matches) != 0 || changed {
		t.Fatalf("missing numeric path matches=%+v changed=%v, want no match or state update", matches, changed)
	}

	if state := ensureConfigState(nil); len(state) != 0 {
		t.Fatalf("ensureConfigState nil = %+v, want empty detached state", state)
	}
	config := map[string]any{"state": "legacy"}
	if state := ensureConfigState(config); len(state) != 0 || config["state"] == "legacy" {
		t.Fatalf("ensureConfigState legacy config=%+v state=%+v, want replaced map", config, state)
	}
	if !cooldownAllows("bad timestamp", now, 60) {
		t.Fatal("cooldownAllows malformed timestamp = false, want permissive true")
	}
	if !cooldownAllows(now.Add(-time.Minute).Format(time.RFC3339), now, 60) {
		t.Fatal("cooldownAllows RFC3339 boundary = false, want true")
	}
	if cooldownAllows(now.Add(-30*time.Second).Format(time.RFC3339Nano), now, 60) {
		t.Fatal("cooldownAllows recent timestamp = true, want false")
	}
	if got := configStringSlice(map[string]any{}, "ids"); got != nil {
		t.Fatalf("configStringSlice missing = %+v, want nil", got)
	}
	if got := configStringSlice(map[string]any{"ids": 42}, "ids"); got != nil {
		t.Fatalf("configStringSlice unsupported = %+v, want nil", got)
	}
	if _, ok := numericAtPath(map[string]any{"snapshot": map[string]any{"price": "bad"}}, "snapshot.price"); ok {
		t.Fatal("numericAtPath bad numeric string = true, want false")
	}
	if got := eventInstrumentID(map[string]any{"payload": map[string]any{"instrument": map[string]any{"instrumentId": nil}}}); got != "" {
		t.Fatalf("eventInstrumentID nil nested = %q, want empty", got)
	}
}

func TestWorkflowBuiltinTemplatesWatchedInstrumentsAndScheduleHelpers(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	ctx := t.Context()

	if err := service.EnsureBuiltinWorkflowTemplates(ctx); err != nil {
		t.Fatalf("EnsureBuiltinWorkflowTemplates: %v", err)
	}
	if err := service.EnsureBuiltinWorkflowTemplates(ctx); err != nil {
		t.Fatalf("EnsureBuiltinWorkflowTemplates second call: %v", err)
	}
	builtin, err := service.GetWorkflow(ctx, "daily-stock-review")
	if err != nil {
		t.Fatalf("GetWorkflow builtin: %v", err)
	}
	if !builtin.BuiltinTemplate || builtin.Status != jfadk.WorkflowStatusDisabled || !strings.Contains(builtin.PromptTemplate, "每日股票盘点") {
		t.Fatalf("builtin workflow = %+v", builtin)
	}
	if builtin.AgentID != jfadk.DefaultBuiltinAgentID {
		t.Fatalf("builtin workflow agent = %q, want %q", builtin.AgentID, jfadk.DefaultBuiltinAgentID)
	}
	triggers, err := service.ListWorkflowTriggers(ctx, builtin.ID)
	if err != nil {
		t.Fatalf("ListWorkflowTriggers builtin: %v", err)
	}
	if len(triggers) != 1 || triggers[0].NextRunAt != "" {
		t.Fatalf("builtin triggers = %+v, want disabled schedule without nextRunAt", triggers)
	}

	_, workflow := saveWorkflowTestAgentAndDefinition(t, runtime, service, "workflow-watch", jfadk.WorkflowStatusEnabled)
	if _, err := service.SaveWorkflowTrigger(ctx, workflow.ID, "", jfadk.WorkflowTriggerWriteRequest{
		ID:     "workflow-watch-market",
		Type:   jfadk.WorkflowTriggerTypeMarketThreshold,
		Status: jfadk.WorkflowTriggerStatusEnabled,
		Config: map[string]any{
			"instrumentIds": []any{" us.aapl ", "US.AAPL", "hk.00700"},
			"value":         100,
		},
	}); err != nil {
		t.Fatalf("SaveWorkflowTrigger market: %v", err)
	}
	if got := strings.Join(service.WatchedWorkflowInstruments(ctx), ","); got != "US.AAPL,HK.00700" {
		t.Fatalf("WatchedWorkflowInstruments = %q", got)
	}
	if got := strings.Join((&Service{}).WatchedWorkflowInstruments(ctx), ","); got != "" {
		t.Fatalf("WatchedWorkflowInstruments unavailable = %q, want empty", got)
	}

	scheduleTrigger := jfadk.WorkflowTrigger{
		Type:   jfadk.WorkflowTriggerTypeSchedule,
		Status: jfadk.WorkflowTriggerStatusEnabled,
		Config: map[string]any{"cron": "0 8 * * 1-5", "timezone": "Asia/Shanghai"},
	}
	if err := service.prepareWorkflowTriggerSchedule(&scheduleTrigger, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("prepareWorkflowTriggerSchedule enabled: %v", err)
	}
	if scheduleTrigger.NextRunAt == "" || nextRunAtString(scheduleTrigger.Config, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)) == "" {
		t.Fatalf("schedule next run not set: %+v", scheduleTrigger)
	}
	manualTrigger := jfadk.WorkflowTrigger{Type: jfadk.WorkflowTriggerTypeManual, NextRunAt: "stale"}
	if err := service.prepareWorkflowTriggerSchedule(&manualTrigger, time.Now()); err != nil {
		t.Fatalf("prepareWorkflowTriggerSchedule manual: %v", err)
	}
	if manualTrigger.NextRunAt != "" {
		t.Fatalf("manual trigger nextRunAt = %q, want cleared", manualTrigger.NextRunAt)
	}
	if err := service.prepareWorkflowTriggerSchedule(nil, time.Now()); err != nil {
		t.Fatalf("prepareWorkflowTriggerSchedule nil: %v", err)
	}
	if nextRunAtString(map[string]any{"cron": "bad"}, time.Now()) != "" {
		t.Fatal("nextRunAtString invalid cron returned non-empty")
	}
}

func TestWorkflowSchedulerTickAndMarketPollingStablePaths(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t, WithWorkflowMarketSnapshot(func(ctx context.Context, instrumentID string) (map[string]any, error) {
		if strings.EqualFold(instrumentID, "US.BAD") {
			return nil, context.Canceled
		}
		return map[string]any{"snapshot": map[string]any{"price": 99.0}}, nil
	}))
	ctx := t.Context()
	_, disabledWorkflow := saveWorkflowTestAgentAndDefinition(t, runtime, service, "workflow-scheduler-disabled", jfadk.WorkflowStatusDisabled)
	dueTrigger, err := runtime.Store().SaveWorkflowTrigger(ctx, jfadk.WorkflowTrigger{
		ID:         "workflow-scheduler-due",
		WorkflowID: disabledWorkflow.ID,
		Type:       jfadk.WorkflowTriggerTypeSchedule,
		Title:      "Due schedule",
		Status:     jfadk.WorkflowTriggerStatusEnabled,
		NextRunAt:  "2026-01-01T00:00:00Z",
		Config:     map[string]any{"cron": "0 8 * * 1-5", "timezone": "Asia/Shanghai"},
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger due schedule: %v", err)
	}
	_, marketWorkflow := saveWorkflowTestAgentAndDefinition(t, runtime, service, "workflow-scheduler-market", jfadk.WorkflowStatusEnabled)
	marketTrigger, err := runtime.Store().SaveWorkflowTrigger(ctx, jfadk.WorkflowTrigger{
		ID:         "workflow-scheduler-market",
		WorkflowID: marketWorkflow.ID,
		Type:       jfadk.WorkflowTriggerTypeMarketThreshold,
		Title:      "Market poll",
		Status:     jfadk.WorkflowTriggerStatusEnabled,
		Config: map[string]any{
			"instrumentIds": []string{"US.BAD", "US.AAPL"},
			"snapshotPath":  "snapshot.price",
			"value":         100,
			"edge":          "cross_up",
		},
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger market: %v", err)
	}

	scheduler := &WorkflowScheduler{service: service, interval: time.Hour}
	scheduler.tick(ctx)

	updatedDue, ok, err := runtime.Store().WorkflowTrigger(ctx, dueTrigger.ID)
	if err != nil || !ok {
		t.Fatalf("WorkflowTrigger due ok=%v err=%v", ok, err)
	}
	if updatedDue.LastRunAt != "" || updatedDue.NextRunAt == "" {
		t.Fatalf("updated due trigger = %+v, want rescheduled without run for disabled workflow", updatedDue)
	}
	updatedMarket, ok, err := runtime.Store().WorkflowTrigger(ctx, marketTrigger.ID)
	if err != nil || !ok {
		t.Fatalf("WorkflowTrigger market ok=%v err=%v", ok, err)
	}
	if !strings.Contains(updatedMarket.LastError, context.Canceled.Error()) {
		t.Fatalf("market trigger lastError = %q, want snapshot error", updatedMarket.LastError)
	}

	service.HandleWorkflowEvent(ctx, jfadk.WorkflowEvent{Type: "market-data.tick", Source: "unit-test", EntityID: "US.MSFT"})
	(&Service{}).HandleWorkflowEvent(ctx, jfadk.WorkflowEvent{Type: "system.notification"})

	emptyScheduler := &WorkflowScheduler{interval: time.Millisecond}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	emptyScheduler.Start(cancelled)
	emptyScheduler.Stop()
	(*WorkflowScheduler)(nil).Stop()
	(*WorkflowScheduler)(nil).tick(ctx)
	(&WorkflowScheduler{}).pollMarketThresholds(ctx, time.Now())
}

func TestWorkflowEventAndSchedulerTriggerBackgroundRuns(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t,
		WithWorkflowSchedulerInterval(time.Hour),
		WithWorkflowMarketSnapshot(func(ctx context.Context, instrumentID string) (map[string]any, error) {
			return map[string]any{"snapshot": map[string]any{"price": 105.0}}, nil
		}),
	)
	assistantServiceProvider(t, runtime)
	ctx := t.Context()

	agent, eventWorkflow := saveWorkflowTestAgentAndDefinition(t, runtime, service, "workflow-event-background", jfadk.WorkflowStatusEnabled)
	eventWorkflow, err := service.SaveWorkflow(ctx, eventWorkflow.ID, jfadk.WorkflowDefinitionWriteRequest{
		Name: eventWorkflow.Name, Status: jfadk.WorkflowStatusEnabled, AgentID: agent.ID,
		WorkMode: jfadk.WorkModeChat, PromptTemplate: "notification {{ .event.category }}",
		CanvasGraph: workflowTestCanvasGraph(),
	})
	if err != nil {
		t.Fatalf("SaveWorkflow event: %v", err)
	}
	eventTrigger, err := service.SaveWorkflowTrigger(ctx, eventWorkflow.ID, "", jfadk.WorkflowTriggerWriteRequest{
		ID: "workflow-event-background-trigger", Type: jfadk.WorkflowTriggerTypeEvent,
		Status: jfadk.WorkflowTriggerStatusEnabled,
		Config: map[string]any{
			"eventType": "system.notification",
			"category":  "broker.connection",
			"level":     "warn",
		},
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger event: %v", err)
	}
	service.HandleWorkflowEvent(ctx, jfadk.WorkflowEvent{
		ID: "event-background-1", Type: "system.notification", Source: "notification",
		EntityID: "broker", At: time.Now().UTC().Format(time.RFC3339Nano),
		Payload: map[string]any{"category": "broker.connection", "level": "warn"},
	})
	eventLogs := waitForWorkflowLogs(t, runtime, eventTrigger.Trigger.ID, jfadk.WorkflowTriggerLogStatusSucceeded, 1)
	if eventLogs[0].Status != jfadk.WorkflowTriggerLogStatusSucceeded || eventLogs[0].MatchedEvent["category"] != "broker.connection" {
		t.Fatalf("event logs = %+v, want succeeded broker connection event", eventLogs)
	}

	cooldownTrigger, err := runtime.Store().SaveWorkflowTrigger(ctx, jfadk.WorkflowTrigger{
		ID:         "workflow-event-cooldown-trigger",
		WorkflowID: eventWorkflow.ID,
		Type:       jfadk.WorkflowTriggerTypeEvent,
		Title:      "Cooldown Event",
		Status:     jfadk.WorkflowTriggerStatusEnabled,
		Config: map[string]any{
			"eventType":   "system.notification",
			"category":    "cooldown",
			"cooldownSec": 600,
			"state": map[string]any{
				"lastTriggeredAt": time.Now().UTC().Format(time.RFC3339Nano),
			},
		},
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger cooldown event: %v", err)
	}
	service.HandleWorkflowEvent(ctx, jfadk.WorkflowEvent{
		ID: "event-cooldown-1", Type: "system.notification", Source: "notification",
		EntityID: "broker", At: time.Now().UTC().Format(time.RFC3339Nano),
		Payload: map[string]any{"category": "cooldown"},
	})
	if logs := workflowLogsForTrigger(t, runtime, cooldownTrigger.ID, ""); len(logs) != 0 {
		t.Fatalf("cooldown event logs = %+v, want no workflow run during cooldown", logs)
	}

	missingWorkflowTrigger, err := runtime.Store().SaveWorkflowTrigger(ctx, jfadk.WorkflowTrigger{
		ID:         "workflow-event-missing-workflow-trigger",
		WorkflowID: "missing-workflow",
		Type:       jfadk.WorkflowTriggerTypeEvent,
		Title:      "Missing workflow Event",
		Status:     jfadk.WorkflowTriggerStatusEnabled,
		Config: map[string]any{
			"eventType": "system.notification",
			"category":  "missing-workflow",
		},
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger missing workflow event: %v", err)
	}
	service.HandleWorkflowEvent(ctx, jfadk.WorkflowEvent{
		ID: "event-missing-workflow-1", Type: "system.notification", Source: "notification",
		EntityID: "broker", At: time.Now().UTC().Format(time.RFC3339Nano),
		Payload: map[string]any{"category": "missing-workflow"},
	})
	if logs := workflowLogsForTrigger(t, runtime, missingWorkflowTrigger.ID, ""); len(logs) != 0 {
		t.Fatalf("missing workflow event logs = %+v, want no workflow run", logs)
	}

	agent, scheduleWorkflow := saveWorkflowTestAgentAndDefinition(t, runtime, service, "workflow-schedule-background", jfadk.WorkflowStatusEnabled)
	scheduleWorkflow, err = service.SaveWorkflow(ctx, scheduleWorkflow.ID, jfadk.WorkflowDefinitionWriteRequest{
		Name: scheduleWorkflow.Name, Status: jfadk.WorkflowStatusEnabled, AgentID: agent.ID,
		WorkMode: jfadk.WorkModeChat, PromptTemplate: "scheduled {{ .event.scheduledAt }}",
		CanvasGraph: workflowTestCanvasGraph(),
	})
	if err != nil {
		t.Fatalf("SaveWorkflow schedule: %v", err)
	}
	scheduleTrigger, err := runtime.Store().SaveWorkflowTrigger(ctx, jfadk.WorkflowTrigger{
		ID:         "workflow-schedule-background-trigger",
		WorkflowID: scheduleWorkflow.ID,
		Type:       jfadk.WorkflowTriggerTypeSchedule,
		Title:      "Due schedule",
		Status:     jfadk.WorkflowTriggerStatusEnabled,
		NextRunAt:  "2026-01-01T00:00:00Z",
		Config:     map[string]any{"cron": "0 8 * * 1-5", "timezone": "Asia/Shanghai"},
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger schedule: %v", err)
	}
	scheduler := &WorkflowScheduler{service: service, interval: time.Hour}
	scheduler.tick(ctx)
	scheduleLogs := waitForWorkflowLogs(t, runtime, scheduleTrigger.ID, jfadk.WorkflowTriggerLogStatusSucceeded, 1)
	if scheduleLogs[0].Status != jfadk.WorkflowTriggerLogStatusSucceeded || scheduleLogs[0].MatchedEvent["scheduledAt"] == nil {
		t.Fatalf("schedule logs = %+v, want succeeded scheduled event", scheduleLogs)
	}

	agent, marketWorkflow := saveWorkflowTestAgentAndDefinition(t, runtime, service, "workflow-market-background", jfadk.WorkflowStatusEnabled)
	marketWorkflow, err = service.SaveWorkflow(ctx, marketWorkflow.ID, jfadk.WorkflowDefinitionWriteRequest{
		Name: marketWorkflow.Name, Status: jfadk.WorkflowStatusEnabled, AgentID: agent.ID,
		WorkMode: jfadk.WorkModeChat, PromptTemplate: "market {{ .event.threshold.current }}",
		CanvasGraph: workflowTestCanvasGraph(),
	})
	if err != nil {
		t.Fatalf("SaveWorkflow market: %v", err)
	}
	marketTrigger, err := runtime.Store().SaveWorkflowTrigger(ctx, jfadk.WorkflowTrigger{
		ID:         "workflow-market-background-trigger",
		WorkflowID: marketWorkflow.ID,
		Type:       jfadk.WorkflowTriggerTypeMarketThreshold,
		Title:      "Market threshold",
		Status:     jfadk.WorkflowTriggerStatusEnabled,
		Config: map[string]any{
			"instrumentIds": []string{"US.AAPL"},
			"snapshotPath":  "snapshot.price",
			"value":         100,
			"edge":          "cross_up",
			"state": map[string]any{
				"lastValues": map[string]any{"US.AAPL": 99.0},
			},
		},
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger market: %v", err)
	}
	scheduler.pollMarketThresholds(ctx, time.Now().UTC())
	marketLogs := waitForWorkflowLogs(t, runtime, marketTrigger.ID, jfadk.WorkflowTriggerLogStatusSucceeded, 1)
	if marketLogs[0].Status != jfadk.WorkflowTriggerLogStatusSucceeded || marketLogs[0].MatchedEvent["threshold"] == nil {
		t.Fatalf("market logs = %+v, want succeeded threshold event", marketLogs)
	}

	agent, tickWorkflow := saveWorkflowTestAgentAndDefinition(t, runtime, service, "workflow-market-tick", jfadk.WorkflowStatusEnabled)
	tickWorkflow, err = service.SaveWorkflow(ctx, tickWorkflow.ID, jfadk.WorkflowDefinitionWriteRequest{
		Name: tickWorkflow.Name, Status: jfadk.WorkflowStatusEnabled, AgentID: agent.ID,
		WorkMode: jfadk.WorkModeChat, PromptTemplate: "tick {{ .event.threshold.current }}",
		CanvasGraph: workflowTestCanvasGraph(),
	})
	if err != nil {
		t.Fatalf("SaveWorkflow market tick: %v", err)
	}
	tickTrigger, err := runtime.Store().SaveWorkflowTrigger(ctx, jfadk.WorkflowTrigger{
		ID:         "workflow-market-tick-trigger",
		WorkflowID: tickWorkflow.ID,
		Type:       jfadk.WorkflowTriggerTypeMarketThreshold,
		Title:      "Market tick threshold",
		Status:     jfadk.WorkflowTriggerStatusEnabled,
		Config: map[string]any{
			"instrumentIds": []string{"US.TSLA"},
			"snapshotPath":  "snapshot.price",
			"value":         250,
			"edge":          "cross_up",
			"state": map[string]any{
				"lastValues": map[string]any{"US.TSLA": 240.0},
			},
		},
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger market tick: %v", err)
	}
	service.HandleWorkflowEvent(ctx, jfadk.WorkflowEvent{
		ID: "market-tick-1", Type: "market-data.tick", Source: "market",
		EntityID: "US.TSLA", At: time.Now().UTC().Format(time.RFC3339Nano),
		Payload: map[string]any{"snapshot": map[string]any{"price": 260.0}},
	})
	tickLogs := waitForWorkflowLogs(t, runtime, tickTrigger.ID, jfadk.WorkflowTriggerLogStatusSucceeded, 1)
	threshold, _ := tickLogs[0].MatchedEvent["threshold"].(map[string]any)
	if tickLogs[0].MatchedEvent["entityId"] != "US.TSLA" || threshold["instrumentId"] != "US.TSLA" {
		t.Fatalf("market tick logs = %+v, want matched threshold event", tickLogs)
	}

	missingMarket, err := runtime.Store().SaveWorkflowTrigger(ctx, jfadk.WorkflowTrigger{
		ID:         "workflow-market-missing-workflow-trigger",
		WorkflowID: "missing-market-workflow",
		Type:       jfadk.WorkflowTriggerTypeMarketThreshold,
		Title:      "Missing workflow market threshold",
		Status:     jfadk.WorkflowTriggerStatusEnabled,
		Config: map[string]any{
			"instrumentIds": []string{"US.MISSING"},
			"snapshotPath":  "snapshot.price",
			"value":         100,
			"edge":          "above",
		},
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger missing market workflow: %v", err)
	}
	service.HandleWorkflowEvent(ctx, jfadk.WorkflowEvent{
		ID: "market-missing-workflow", Type: "market-data.tick", Source: "market",
		EntityID: "US.MISSING", At: time.Now().UTC().Format(time.RFC3339Nano),
		Payload: map[string]any{"snapshot": map[string]any{"price": 105.0}},
	})
	if logs := workflowLogsForTrigger(t, runtime, missingMarket.ID, ""); len(logs) != 0 {
		t.Fatalf("missing market workflow logs = %+v, want no run", logs)
	}

	missingPoll, err := runtime.Store().SaveWorkflowTrigger(ctx, jfadk.WorkflowTrigger{
		ID:         "workflow-market-missing-poll-workflow-trigger",
		WorkflowID: "missing-poll-workflow",
		Type:       jfadk.WorkflowTriggerTypeMarketThreshold,
		Title:      "Missing poll workflow market threshold",
		Status:     jfadk.WorkflowTriggerStatusEnabled,
		Config: map[string]any{
			"instrumentIds": []string{"US.MISSING-POLL"},
			"snapshotPath":  "snapshot.price",
			"value":         100,
			"edge":          "above",
		},
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger missing poll workflow: %v", err)
	}
	scheduler.pollMarketThresholds(ctx, time.Now().UTC())
	if logs := workflowLogsForTrigger(t, runtime, missingPoll.ID, ""); len(logs) != 0 {
		t.Fatalf("missing poll workflow logs = %+v, want no run", logs)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	service.HandleWorkflowEvent(cancelled, jfadk.WorkflowEvent{Type: "system.notification"})

	service.StartWorkflowScheduler(ctx)
	if service.workflowScheduler == nil {
		t.Fatal("StartWorkflowScheduler did not install scheduler")
	}
	service.StartWorkflowScheduler(ctx)
	service.workflowScheduler.Stop()
}

func TestWorkflowActiveRunSkipAndReconciliation(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	assistantServiceProvider(t, runtime)
	ctx := t.Context()
	agent, workflow := saveWorkflowTestAgentAndDefinition(t, runtime, service, "workflow-active", jfadk.WorkflowStatusEnabled)
	workflow, err := service.SaveWorkflow(ctx, workflow.ID, jfadk.WorkflowDefinitionWriteRequest{
		Name:           workflow.Name,
		Status:         jfadk.WorkflowStatusEnabled,
		AgentID:        agent.ID,
		WorkMode:       jfadk.WorkModeChat,
		PermissionMode: jfadk.PermissionModeApproval,
		PromptTemplate: workflow.PromptTemplate,
		DefaultInputs:  workflow.DefaultInputs,
		CanvasGraph:    workflowTestCanvasGraph(),
	})
	if err != nil {
		t.Fatalf("SaveWorkflow chat mode: %v", err)
	}
	triggerResult, err := service.SaveWorkflowTrigger(ctx, workflow.ID, "", jfadk.WorkflowTriggerWriteRequest{
		ID:     "workflow-active-trigger",
		Type:   jfadk.WorkflowTriggerTypeManual,
		Status: jfadk.WorkflowTriggerStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger manual: %v", err)
	}
	activeLog, err := runtime.Store().SaveWorkflowTriggerLog(ctx, jfadk.WorkflowTriggerLog{
		WorkflowID:  workflow.ID,
		TriggerID:   triggerResult.Trigger.ID,
		TriggerType: triggerResult.Trigger.Type,
		Status:      jfadk.WorkflowTriggerLogStatusQueued,
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTriggerLog active: %v", err)
	}
	active, err := service.workflowTriggerHasActiveRun(ctx, triggerResult.Trigger.ID)
	if err != nil {
		t.Fatalf("workflowTriggerHasActiveRun active: %v", err)
	}
	if !active {
		t.Fatal("workflowTriggerHasActiveRun active = false, want true")
	}
	skipped, err := service.RunWorkflowTrigger(ctx, triggerResult.Trigger.ID, map[string]any{"symbol": "US.AAPL"})
	if err != nil {
		t.Fatalf("RunWorkflowTrigger active skip: %v", err)
	}
	if skipped.Log.Status != jfadk.WorkflowTriggerLogStatusSkipped || !strings.Contains(skipped.Log.Error, "previous trigger run") {
		t.Fatalf("skipped log = %+v", skipped.Log)
	}

	completedRun := jfadk.Run{
		ID:               "workflow-active-completed-run",
		SessionID:        "session-active",
		AgentID:          agent.ID,
		Status:           jfadk.RunStatusCompleted,
		Message:          "done",
		ToolCalls:        []jfadk.ToolCall{},
		CreatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
		PendingApprovals: []jfadk.Approval{},
	}
	if err := runtime.Store().SaveRun(ctx, completedRun); err != nil {
		t.Fatalf("SaveRun completed: %v", err)
	}
	activeLog.RunID = completedRun.ID
	if _, err := runtime.Store().SaveWorkflowTriggerLog(ctx, activeLog); err != nil {
		t.Fatalf("SaveWorkflowTriggerLog completed run: %v", err)
	}
	active, err = service.workflowTriggerHasActiveRun(ctx, triggerResult.Trigger.ID)
	if err != nil {
		t.Fatalf("workflowTriggerHasActiveRun completed: %v", err)
	}
	if active {
		t.Fatal("workflowTriggerHasActiveRun completed = true, want false")
	}
	reconciled, ok, err := runtime.Store().WorkflowTriggerLog(ctx, activeLog.ID)
	if err != nil || !ok {
		t.Fatalf("WorkflowTriggerLog reconciled ok=%v err=%v", ok, err)
	}
	if reconciled.Status != jfadk.WorkflowTriggerLogStatusSucceeded || reconciled.FinishedAt == "" {
		t.Fatalf("reconciled log = %+v, want succeeded with finishedAt", reconciled)
	}

	missingRunLog, err := runtime.Store().SaveWorkflowTriggerLog(ctx, jfadk.WorkflowTriggerLog{
		WorkflowID:  workflow.ID,
		TriggerID:   triggerResult.Trigger.ID,
		TriggerType: triggerResult.Trigger.Type,
		Status:      jfadk.WorkflowTriggerLogStatusRunning,
		RunID:       "missing-run",
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTriggerLog missing run: %v", err)
	}
	service.reconcileActiveWorkflowLogs(ctx)
	missingRunLog, ok, err = runtime.Store().WorkflowTriggerLog(ctx, missingRunLog.ID)
	if err != nil || !ok {
		t.Fatalf("WorkflowTriggerLog missing run ok=%v err=%v", ok, err)
	}
	if missingRunLog.Status != jfadk.WorkflowTriggerLogStatusFailed || !strings.Contains(missingRunLog.Error, "run not found") {
		t.Fatalf("missing run log = %+v, want failed run not found", missingRunLog)
	}
}

func TestWorkflowResultAndRunStatusHelpers(t *testing.T) {
	runtime, _, _ := newAssistantServiceHarness(t)
	response := jfadk.ChatResponse{
		Reply: "",
		Run:   jfadk.Run{ID: "run-failed", Status: jfadk.RunStatusFailed, FailureReason: "provider down"},
	}
	result := workflowResultFromResponse(response)
	if result.Markdown != "provider down" || result.RawResponse == nil {
		t.Fatalf("workflowResultFromResponse = %+v", result)
	}
	for _, tc := range []struct {
		status string
		want   string
	}{
		{jfadk.RunStatusCompleted, jfadk.WorkflowTriggerLogStatusSucceeded},
		{jfadk.RunStatusPending, jfadk.WorkflowTriggerLogStatusPendingApproval},
		{jfadk.RunStatusDenied, jfadk.WorkflowTriggerLogStatusCancelled},
		{jfadk.RunStatusCancelled, jfadk.WorkflowTriggerLogStatusCancelled},
		{jfadk.RunStatusFailed, jfadk.WorkflowTriggerLogStatusFailed},
		{jfadk.RunStatusTimedOut, jfadk.WorkflowTriggerLogStatusFailed},
		{jfadk.RunStatusRunning, jfadk.WorkflowTriggerLogStatusRunning},
	} {
		if got := workflowLogStatusFromRun(jfadk.Run{Status: tc.status}); got != tc.want {
			t.Fatalf("workflowLogStatusFromRun(%q) = %q, want %q", tc.status, got, tc.want)
		}
	}
	finished := finishWorkflowLog(t.Context(), runtime.Store(), jfadk.WorkflowTriggerLog{Status: jfadk.WorkflowTriggerLogStatusRunning}, jfadk.WorkflowTriggerLogStatusFailed, "boom")
	if finished.Status != jfadk.WorkflowTriggerLogStatusFailed || finished.Error != "boom" || finished.FinishedAt == "" {
		t.Fatalf("finishWorkflowLog nil store = %+v", finished)
	}
	if errorString(context.Canceled) != context.Canceled.Error() {
		t.Fatal("errorString context.Canceled mismatch")
	}

	nodeRuns := workflowNodeRuns(
		jfadk.WorkflowDefinition{Name: "Fallback Trace", AgentID: "agent-1", WorkMode: jfadk.WorkModeLoop},
		&jfadk.WorkflowTrigger{ID: "trigger-1", Type: jfadk.WorkflowTriggerTypeEvent, Title: "   "},
		jfadk.WorkflowTriggerTypeEvent,
		map[string]any{"symbol": "US.AAPL"},
		nil,
		"run",
		"review",
		nil,
		jfadk.WorkflowTriggerLogStatusRunning,
		"",
		"2026-07-01T00:00:00Z",
		"",
	)
	if nodeRuns[0].Title != "事件触发" || nodeRuns[1].Outputs["objective"] != "review" {
		t.Fatalf("workflowNodeRuns fallback trace = %+v", nodeRuns)
	}

	thresholdTrigger := jfadk.WorkflowTrigger{Config: map[string]any{
		"instrumentIds": []string{"US.AAPL"},
		"value":         100,
	}}
	matches, changed := evaluateMarketThresholdTrigger(thresholdTrigger, []map[string]any{{"payload": map[string]any{"snapshot": map[string]any{"price": 101}}}}, time.Now())
	if len(matches) != 0 || changed {
		t.Fatalf("threshold event without instrument matches=%+v changed=%v", matches, changed)
	}
	if value, ok := numericAtPath(map[string]any{"snapshot": map[string]any{"price": 101}}, "snapshot..price"); !ok || value != 101 {
		t.Fatalf("numericAtPath empty segment value=%v ok=%v", value, ok)
	}

	finishedAt := time.Date(2026, 7, 1, 0, 0, 5, 0, time.UTC)
	failedLog := applyWorkflowResponse(
		jfadk.WorkflowTriggerLog{TriggerType: jfadk.WorkflowTriggerTypeManual},
		jfadk.WorkflowDefinition{Name: "Failed workflow"}, nil, nil, nil, "run", "",
		jfadk.ChatResponse{
			Session: jfadk.Session{ID: "session-failed"},
			Run:     jfadk.Run{ID: "run-failed", Status: jfadk.RunStatusFailed, FailureReason: "provider down"},
		},
		"2026-07-01T00:00:00Z",
		finishedAt,
	)
	if failedLog.Status != jfadk.WorkflowTriggerLogStatusFailed || failedLog.Error != "provider down" || failedLog.FinishedAt != finishedAt.Format(time.RFC3339Nano) {
		t.Fatalf("applyWorkflowResponse failed log = %+v", failedLog)
	}
	pendingLog := applyWorkflowResponse(
		jfadk.WorkflowTriggerLog{TriggerType: jfadk.WorkflowTriggerTypeManual},
		jfadk.WorkflowDefinition{Name: "Pending workflow"}, nil, nil, nil, "run", "",
		jfadk.ChatResponse{
			Session: jfadk.Session{ID: "session-pending"},
			Run:     jfadk.Run{ID: "run-pending", Status: jfadk.RunStatusPending},
		},
		"2026-07-01T00:00:00Z",
		finishedAt,
	)
	if pendingLog.Status != jfadk.WorkflowTriggerLogStatusPendingApproval || pendingLog.FinishedAt != "" || pendingLog.Error != "" {
		t.Fatalf("applyWorkflowResponse pending log = %+v", pendingLog)
	}
}

var errWorkflowLogWriteInjected = errors.New("workflow log write injected")

type workflowInvocationFaultStore struct {
	base          *jfadk.Store
	listErr       error
	activeLogsSet bool
	activeLogs    []jfadk.WorkflowTriggerLog
	failSaveAt    int
	saveCalls     int
	savedLogs     []jfadk.WorkflowTriggerLog
	runErr        error
	runsSet       bool
	runs          map[string]jfadk.Run
}

func (s *workflowInvocationFaultStore) SaveWorkflowTriggerLog(ctx context.Context, log jfadk.WorkflowTriggerLog) (jfadk.WorkflowTriggerLog, error) {
	s.saveCalls++
	if s.saveCalls == s.failSaveAt {
		return jfadk.WorkflowTriggerLog{}, errWorkflowLogWriteInjected
	}
	s.savedLogs = append(s.savedLogs, log)
	return s.base.SaveWorkflowTriggerLog(ctx, log)
}

func (s *workflowInvocationFaultStore) ListActiveWorkflowTriggerLogs(ctx context.Context, triggerID string) ([]jfadk.WorkflowTriggerLog, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.activeLogsSet {
		return s.activeLogs, nil
	}
	return s.base.ListActiveWorkflowTriggerLogs(ctx, triggerID)
}

func (s *workflowInvocationFaultStore) Run(ctx context.Context, runID string) (jfadk.Run, bool, error) {
	if s.runErr != nil {
		return jfadk.Run{}, false, s.runErr
	}
	if s.runsSet {
		run, ok := s.runs[runID]
		return run, ok, nil
	}
	return s.base.Run(ctx, runID)
}

func marketThresholdEvent(instrumentID string, price float64) map[string]any {
	return map[string]any{
		"type":     "market-data.tick",
		"entityId": instrumentID,
		"snapshot": map[string]any{
			"price": price,
		},
	}
}

func workflowLogsForTrigger(t *testing.T, runtime *jfadk.Runtime, triggerID string, status string) []jfadk.WorkflowTriggerLog {
	t.Helper()
	logs, _, err := runtime.Store().ListWorkflowTriggerLogsPage(t.Context(), "", triggerID, status, 20, 0)
	if err != nil {
		t.Fatalf("ListWorkflowTriggerLogsPage: %v", err)
	}
	return logs
}

func waitForWorkflowLogs(t *testing.T, runtime *jfadk.Runtime, triggerID string, status string, count int) []jfadk.WorkflowTriggerLog {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		logs := workflowLogsForTrigger(t, runtime, triggerID, status)
		if len(logs) >= count {
			return logs
		}
		if time.Now().After(deadline) {
			t.Fatalf("workflow logs for trigger %q status %q = %d, want at least %d", triggerID, status, len(logs), count)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func saveWorkflowTestAgentAndDefinition(t *testing.T, runtime *jfadk.Runtime, service *Service, id string, status string) (jfadk.Agent, jfadk.WorkflowDefinition) {
	t.Helper()
	ctx := context.Background()
	agent, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID:         id + "-agent",
		Name:       id + " Agent",
		Status:     jfadk.AgentStatusEnabled,
		ProviderID: "test-provider",
		Model:      "test-model",
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	workflow, err := service.SaveWorkflow(ctx, "", jfadk.WorkflowDefinitionWriteRequest{
		ID:             id,
		Name:           id + " Workflow",
		Status:         status,
		AgentID:        agent.ID,
		WorkMode:       jfadk.WorkModeLoop,
		PermissionMode: jfadk.PermissionModeApproval,
		PromptTemplate: "run {{ .symbol }}",
		DefaultInputs:  map[string]any{"symbol": "US.AAPL"},
		CanvasGraph:    workflowTestCanvasGraph(),
	})
	if err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}
	return agent, workflow
}

func workflowTestCanvasGraph() *jfadk.WorkflowCanvasGraph {
	return &jfadk.WorkflowCanvasGraph{
		Version: "adk-workflow-canvas/v1",
		Nodes: []jfadk.WorkflowCanvasNode{
			{ID: "start", Type: "start", Position: jfadk.WorkflowCanvasPoint{}},
			{ID: "agent:primary", Type: "agent", Position: jfadk.WorkflowCanvasPoint{}},
			{ID: "monitor", Type: "monitor", Position: jfadk.WorkflowCanvasPoint{}},
		},
		Edges: []jfadk.WorkflowCanvasEdge{
			{ID: "start-agent", Source: "start", Target: "agent:primary"},
			{ID: "agent-monitor", Source: "agent:primary", Target: "monitor"},
		},
	}
}
