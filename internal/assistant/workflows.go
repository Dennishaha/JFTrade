package assistant

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"math"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/robfig/cron/v3"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

const (
	defaultWorkflowSchedulerInterval = 30 * time.Second
	defaultWorkflowScheduleBatchSize = 20
	defaultMarketThresholdCooldown   = 900
)

type WorkflowMarketSnapshot func(ctx context.Context, instrumentID string) (map[string]any, error)

type WorkflowQuery struct {
	Status string
	Limit  int
	Offset int
}

type WorkflowTriggerLogQuery struct {
	WorkflowID string
	TriggerID  string
	Status     string
	Limit      int
	Offset     int
}

type WorkflowTriggerSaveResult struct {
	Trigger jfadk.WorkflowTrigger `json:"trigger"`
	Secret  string                `json:"secret,omitempty"`
}

type WorkflowInvocationResult struct {
	Workflow jfadk.WorkflowDefinition `json:"workflow"`
	Trigger  *jfadk.WorkflowTrigger   `json:"trigger,omitempty"`
	Log      jfadk.WorkflowTriggerLog `json:"log"`
	Response *jfadk.ChatResponse      `json:"response,omitempty"`
}

type WorkflowScheduler struct {
	service  *Service
	interval time.Duration
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

func (s *Service) StartWorkflowScheduler(ctx context.Context) {
	if s == nil || !s.Available() || s.workflowScheduler != nil {
		return
	}
	if err := s.EnsureBuiltinWorkflowTemplates(ctx); err != nil {
		log.Printf("JFTrade ADK workflow template initialization failed: %v", err)
	}
	interval := s.workflowInterval
	if interval <= 0 {
		interval = defaultWorkflowSchedulerInterval
	}
	s.workflowScheduler = &WorkflowScheduler{service: s, interval: interval}
	s.workflowScheduler.Start(ctx)
}

func (s *Service) EnsureBuiltinWorkflowTemplates(ctx context.Context) error {
	if s == nil || s.runtime == nil || s.runtime.Store() == nil {
		return fmt.Errorf("adk runtime is unavailable")
	}
	store := s.runtime.Store()
	if existing, ok, err := store.WorkflowDefinition(ctx, "daily-stock-review"); err != nil {
		return err
	} else if ok && existing.DeletedAt == nil {
		return nil
	}
	workflow := jfadk.WorkflowDefinition{
		ID:                "daily-stock-review",
		Name:              "每日股票盘点",
		Description:       "交易日上午盘点关注列表、持仓、风险事件与待办事项。",
		Status:            jfadk.WorkflowStatusDisabled,
		AgentID:           "investment-analyst",
		WorkMode:          jfadk.WorkModeTask,
		PermissionMode:    jfadk.PermissionModeApproval,
		PromptTemplate:    dailyStockReviewPrompt(),
		ObjectiveTemplate: "完成每日股票盘点，输出可审计的市场、持仓、风险和待办摘要。",
		DefaultInputs: map[string]any{
			"watchlist": []string{"US.AAPL", "US.MSFT", "HK.00700"},
			"market":    "US/HK",
		},
		Tags:            []string{"stock", "daily-review"},
		BuiltinTemplate: true,
	}
	created, err := store.SaveWorkflowDefinition(ctx, workflow)
	if err != nil {
		return err
	}
	_, err = store.SaveWorkflowTrigger(ctx, jfadk.WorkflowTrigger{
		ID:         "daily-stock-review-schedule",
		WorkflowID: created.ID,
		Type:       jfadk.WorkflowTriggerTypeSchedule,
		Title:      "工作日上午 8 点",
		Status:     jfadk.WorkflowTriggerStatusDisabled,
		Config: map[string]any{
			"cron":     "0 8 * * 1-5",
			"timezone": "Asia/Shanghai",
		},
	})
	return err
}

func (s *Service) ListWorkflows(ctx context.Context, query WorkflowQuery) (Page[jfadk.WorkflowDefinition], error) {
	if s == nil || s.runtime == nil || s.runtime.Store() == nil {
		return Page[jfadk.WorkflowDefinition]{}, fmt.Errorf("adk runtime is unavailable")
	}
	limit, offset := normalizedWorkflowPage(query.Limit, query.Offset)
	items, total, err := s.runtime.Store().ListWorkflowDefinitionsPage(ctx, query.Status, limit, offset)
	if err != nil {
		return Page[jfadk.WorkflowDefinition]{}, err
	}
	return Page[jfadk.WorkflowDefinition]{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

func (s *Service) GetWorkflow(ctx context.Context, workflowID string) (jfadk.WorkflowDefinition, error) {
	store, err := s.workflowStore()
	if err != nil {
		return jfadk.WorkflowDefinition{}, err
	}
	workflow, ok, err := store.WorkflowDefinition(ctx, workflowID)
	if err != nil {
		return jfadk.WorkflowDefinition{}, err
	}
	if !ok || workflow.DeletedAt != nil {
		return jfadk.WorkflowDefinition{}, fmt.Errorf("workflow not found")
	}
	return workflow, nil
}

func (s *Service) SaveWorkflow(ctx context.Context, workflowID string, payload jfadk.WorkflowDefinitionWriteRequest) (jfadk.WorkflowDefinition, error) {
	store, err := s.workflowStore()
	if err != nil {
		return jfadk.WorkflowDefinition{}, err
	}
	workflow := jfadk.WorkflowDefinition{}
	if strings.TrimSpace(workflowID) != "" {
		existing, ok, err := store.WorkflowDefinition(ctx, workflowID)
		if err != nil {
			return jfadk.WorkflowDefinition{}, err
		}
		if !ok || existing.DeletedAt != nil {
			return jfadk.WorkflowDefinition{}, fmt.Errorf("workflow not found")
		}
		workflow = existing
	} else if strings.TrimSpace(payload.ID) != "" {
		workflow.ID = strings.TrimSpace(payload.ID)
	}
	workflow.Name = strings.TrimSpace(payload.Name)
	workflow.Description = strings.TrimSpace(payload.Description)
	workflow.Status = normalizeWorkflowStatus(payload.Status, workflow.Status)
	workflow.AgentID = strings.TrimSpace(payload.AgentID)
	workflow.WorkMode = normalizeWorkflowWorkMode(payload.WorkMode, workflow.WorkMode)
	workflow.ProviderID = strings.TrimSpace(payload.ProviderID)
	workflow.Model = strings.TrimSpace(payload.Model)
	workflow.PermissionMode = normalizeWorkflowPermissionMode(payload.PermissionMode, workflow.PermissionMode)
	workflow.PromptTemplate = strings.TrimSpace(payload.PromptTemplate)
	workflow.ObjectiveTemplate = strings.TrimSpace(payload.ObjectiveTemplate)
	workflow.DefaultInputs = cloneMap(payload.DefaultInputs)
	workflow.CanvasGraph = payload.CanvasGraph
	workflow.Tags = normalizeStringList(payload.Tags)
	if err := s.validateWorkflowDefinition(ctx, workflow); err != nil {
		return jfadk.WorkflowDefinition{}, err
	}
	workflow, err = store.SaveWorkflowDefinition(ctx, workflow)
	if err != nil {
		return jfadk.WorkflowDefinition{}, err
	}
	s.runtime.RecordAudit(ctx, "workflow.saved", workflow.ID, "ADK workflow saved.", map[string]any{"status": workflow.Status})
	return workflow, nil
}

func (s *Service) DeleteWorkflow(ctx context.Context, workflowID string) (jfadk.WorkflowDefinition, error) {
	store, err := s.workflowStore()
	if err != nil {
		return jfadk.WorkflowDefinition{}, err
	}
	workflow, err := store.DeleteWorkflowDefinition(ctx, workflowID)
	if err != nil {
		return jfadk.WorkflowDefinition{}, err
	}
	s.runtime.RecordAudit(ctx, "workflow.deleted", workflow.ID, "ADK workflow disabled and deleted.", nil)
	return workflow, nil
}

func (s *Service) ListWorkflowTriggers(ctx context.Context, workflowID string) ([]jfadk.WorkflowTrigger, error) {
	store, err := s.workflowStore()
	if err != nil {
		return nil, err
	}
	if _, err := s.GetWorkflow(ctx, workflowID); err != nil {
		return nil, err
	}
	triggers, err := store.ListWorkflowTriggers(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	for index := range triggers {
		triggers[index] = sanitizeWorkflowTrigger(triggers[index])
	}
	return triggers, nil
}

func (s *Service) SaveWorkflowTrigger(ctx context.Context, workflowID string, triggerID string, payload jfadk.WorkflowTriggerWriteRequest) (WorkflowTriggerSaveResult, error) {
	store, err := s.workflowStore()
	if err != nil {
		return WorkflowTriggerSaveResult{}, err
	}
	workflow, err := s.GetWorkflow(ctx, workflowID)
	if err != nil {
		return WorkflowTriggerSaveResult{}, err
	}
	trigger := jfadk.WorkflowTrigger{WorkflowID: workflow.ID}
	isCreate := strings.TrimSpace(triggerID) == ""
	if !isCreate {
		existing, ok, err := store.WorkflowTrigger(ctx, triggerID)
		if err != nil {
			return WorkflowTriggerSaveResult{}, err
		}
		if !ok || existing.WorkflowID != workflow.ID || existing.DeletedAt != nil {
			return WorkflowTriggerSaveResult{}, fmt.Errorf("workflow trigger not found")
		}
		trigger = existing
	} else if strings.TrimSpace(payload.ID) != "" {
		trigger.ID = strings.TrimSpace(payload.ID)
	}
	trigger.WorkflowID = workflow.ID
	trigger.Type = normalizeTriggerType(payload.Type, trigger.Type)
	trigger.Title = strings.TrimSpace(payload.Title)
	trigger.Status = normalizeTriggerStatus(payload.Status, trigger.Status)
	trigger.Config = cloneMap(payload.Config)
	secret := ""
	if trigger.Type == jfadk.WorkflowTriggerTypeWebhook && (isCreate || payload.ResetSecret || trigger.SecretHash == "") {
		secret, err = newWorkflowSecret()
		if err != nil {
			return WorkflowTriggerSaveResult{}, err
		}
		trigger.SecretHash = hashWorkflowSecret(secret)
		trigger.HasSecret = true
	}
	if trigger.Title == "" {
		trigger.Title = defaultTriggerTitle(trigger.Type)
	}
	if err := s.prepareWorkflowTriggerSchedule(&trigger, time.Now().UTC()); err != nil {
		return WorkflowTriggerSaveResult{}, err
	}
	if err := validateWorkflowTrigger(trigger); err != nil {
		return WorkflowTriggerSaveResult{}, err
	}
	trigger, err = store.SaveWorkflowTrigger(ctx, trigger)
	if err != nil {
		return WorkflowTriggerSaveResult{}, err
	}
	s.runtime.RecordAudit(ctx, "workflow.trigger.saved", trigger.ID, "ADK workflow trigger saved.", map[string]any{"workflowId": workflow.ID, "type": trigger.Type})
	return WorkflowTriggerSaveResult{Trigger: sanitizeWorkflowTrigger(trigger), Secret: secret}, nil
}

func (s *Service) DeleteWorkflowTrigger(ctx context.Context, workflowID string, triggerID string) (jfadk.WorkflowTrigger, error) {
	store, err := s.workflowStore()
	if err != nil {
		return jfadk.WorkflowTrigger{}, err
	}
	trigger, ok, err := store.WorkflowTrigger(ctx, triggerID)
	if err != nil {
		return jfadk.WorkflowTrigger{}, err
	}
	if !ok || trigger.WorkflowID != strings.TrimSpace(workflowID) || trigger.DeletedAt != nil {
		return jfadk.WorkflowTrigger{}, fmt.Errorf("workflow trigger not found")
	}
	trigger, err = store.DeleteWorkflowTrigger(ctx, trigger.ID)
	if err != nil {
		return jfadk.WorkflowTrigger{}, err
	}
	s.runtime.RecordAudit(ctx, "workflow.trigger.deleted", trigger.ID, "ADK workflow trigger disabled and deleted.", map[string]any{"workflowId": workflowID})
	return sanitizeWorkflowTrigger(trigger), nil
}

func (s *Service) ListWorkflowTriggerLogs(ctx context.Context, query WorkflowTriggerLogQuery) (Page[jfadk.WorkflowTriggerLog], error) {
	store, err := s.workflowStore()
	if err != nil {
		return Page[jfadk.WorkflowTriggerLog]{}, err
	}
	limit, offset := normalizedWorkflowPage(query.Limit, query.Offset)
	items, total, err := store.ListWorkflowTriggerLogsPage(ctx, query.WorkflowID, query.TriggerID, query.Status, limit, offset)
	if err != nil {
		return Page[jfadk.WorkflowTriggerLog]{}, err
	}
	return Page[jfadk.WorkflowTriggerLog]{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

func (s *Service) RunWorkflow(ctx context.Context, workflowID string, inputs map[string]any) (WorkflowInvocationResult, error) {
	workflow, err := s.GetWorkflow(ctx, workflowID)
	if err != nil {
		return WorkflowInvocationResult{}, err
	}
	return s.invokeWorkflow(ctx, workflow, nil, jfadk.WorkflowTriggerTypeManual, inputs, nil)
}

func (s *Service) RunWorkflowTrigger(ctx context.Context, triggerID string, inputs map[string]any) (WorkflowInvocationResult, error) {
	store, err := s.workflowStore()
	if err != nil {
		return WorkflowInvocationResult{}, err
	}
	trigger, ok, err := store.WorkflowTrigger(ctx, triggerID)
	if err != nil {
		return WorkflowInvocationResult{}, err
	}
	if !ok || trigger.DeletedAt != nil {
		return WorkflowInvocationResult{}, fmt.Errorf("workflow trigger not found")
	}
	workflow, err := s.GetWorkflow(ctx, trigger.WorkflowID)
	if err != nil {
		return WorkflowInvocationResult{}, err
	}
	return s.invokeWorkflow(ctx, workflow, &trigger, trigger.Type, inputs, nil)
}

func (s *Service) RunWorkflowWebhook(ctx context.Context, triggerID string, secret string, inputs map[string]any) (WorkflowInvocationResult, error) {
	store, err := s.workflowStore()
	if err != nil {
		return WorkflowInvocationResult{}, err
	}
	trigger, ok, err := store.WorkflowTrigger(ctx, triggerID)
	if err != nil {
		return WorkflowInvocationResult{}, err
	}
	if !ok || trigger.Type != jfadk.WorkflowTriggerTypeWebhook || trigger.DeletedAt != nil {
		return WorkflowInvocationResult{}, fmt.Errorf("workflow webhook not found")
	}
	if trigger.Status != jfadk.WorkflowTriggerStatusEnabled {
		return WorkflowInvocationResult{}, fmt.Errorf("workflow webhook is disabled")
	}
	if !verifyWorkflowSecret(secret, trigger.SecretHash) {
		return WorkflowInvocationResult{}, fmt.Errorf("invalid workflow webhook secret")
	}
	workflow, err := s.GetWorkflow(ctx, trigger.WorkflowID)
	if err != nil {
		return WorkflowInvocationResult{}, err
	}
	return s.invokeWorkflow(ctx, workflow, &trigger, trigger.Type, inputs, map[string]any{"type": "workflow.webhook", "triggerId": trigger.ID})
}

func (s *Service) WatchedWorkflowInstruments(ctx context.Context) []string {
	if s == nil || s.runtime == nil || s.runtime.Store() == nil {
		return nil
	}
	triggers, err := s.runtime.Store().ListEnabledWorkflowTriggersByType(ctx, jfadk.WorkflowTriggerTypeMarketThreshold)
	if err != nil {
		return nil
	}
	seen := map[string]struct{}{}
	out := []string{}
	for _, trigger := range triggers {
		for _, instrumentID := range configStringSlice(trigger.Config, "instrumentIds") {
			instrumentID = strings.ToUpper(strings.TrimSpace(instrumentID))
			if instrumentID == "" {
				continue
			}
			if _, ok := seen[instrumentID]; ok {
				continue
			}
			seen[instrumentID] = struct{}{}
			out = append(out, instrumentID)
		}
	}
	return out
}

func (s *Service) HandleWorkflowEvent(ctx context.Context, event jfadk.WorkflowEvent) {
	if s == nil || s.runtime == nil || s.runtime.Store() == nil {
		return
	}
	store := s.runtime.Store()
	if event.Type == "market-data.tick" {
		triggers, err := store.ListEnabledWorkflowTriggersByType(ctx, jfadk.WorkflowTriggerTypeMarketThreshold)
		if err == nil {
			for _, trigger := range triggers {
				matches, changed := evaluateMarketThresholdTrigger(trigger, []map[string]any{eventAsMap(event)}, time.Now().UTC())
				if changed {
					updated, saveErr := store.SaveWorkflowTrigger(ctx, trigger)
					if saveErr == nil {
						trigger = updated
					}
				}
				for _, matched := range matches {
					workflow, wfErr := s.GetWorkflow(ctx, trigger.WorkflowID)
					if wfErr != nil {
						continue
					}
					go s.invokeWorkflowBackground(workflow, trigger, matched)
				}
			}
		}
	}
	triggers, err := store.ListEnabledWorkflowTriggersByType(ctx, jfadk.WorkflowTriggerTypeEvent)
	if err != nil {
		return
	}
	for _, trigger := range triggers {
		if !workflowEventMatches(trigger.Config, event) {
			continue
		}
		if !eventTriggerCooldownAllows(&trigger, time.Now().UTC()) {
			_, _ = store.SaveWorkflowTrigger(ctx, trigger)
			continue
		}
		updated, saveErr := store.SaveWorkflowTrigger(ctx, trigger)
		if saveErr == nil {
			trigger = updated
		}
		workflow, wfErr := s.GetWorkflow(ctx, trigger.WorkflowID)
		if wfErr != nil {
			continue
		}
		go s.invokeWorkflowBackground(workflow, trigger, eventAsMap(event))
	}
}

func (scheduler *WorkflowScheduler) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	scheduler.cancel = cancel
	scheduler.wg.Go(func() {
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
	})
}

func (scheduler *WorkflowScheduler) Stop() {
	if scheduler == nil {
		return
	}
	if scheduler.cancel != nil {
		scheduler.cancel()
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
				trigger.NextRunAt = nextRunAtString(trigger.Config, now)
				_, _ = store.SaveWorkflowTrigger(ctx, trigger)
				continue
			}
			trigger.LastError = ""
			trigger.LastRunAt = now.Format(time.RFC3339Nano)
			trigger.NextRunAt = nextRunAtString(trigger.Config, now)
			updated, saveErr := store.SaveWorkflowTrigger(ctx, trigger)
			if saveErr == nil {
				trigger = updated
			}
			go service.invokeWorkflowBackground(workflow, trigger, map[string]any{"scheduledAt": now.Format(time.RFC3339Nano)})
		}
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
		for _, instrumentID := range configStringSlice(trigger.Config, "instrumentIds") {
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
		matches, changed := evaluateMarketThresholdTrigger(trigger, events, now)
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
			go service.invokeWorkflowBackground(workflow, trigger, matched)
		}
	}
}

func (s *Service) invokeWorkflowBackground(workflow jfadk.WorkflowDefinition, trigger jfadk.WorkflowTrigger, matchedEvent map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), jfadk.DefaultRunTimeout+time.Minute)
	defer cancel()
	_, _ = s.invokeWorkflow(ctx, workflow, &trigger, trigger.Type, map[string]any{"event": matchedEvent}, matchedEvent)
}

func (s *Service) invokeWorkflow(ctx context.Context, workflow jfadk.WorkflowDefinition, trigger *jfadk.WorkflowTrigger, triggerType string, inputs map[string]any, matchedEvent map[string]any) (WorkflowInvocationResult, error) {
	store, err := s.workflowStore()
	if err != nil {
		return WorkflowInvocationResult{}, err
	}
	return s.invokeWorkflowWithStore(ctx, store, workflow, trigger, triggerType, inputs, matchedEvent)
}

type workflowInvocationStore interface {
	SaveWorkflowTriggerLog(context.Context, jfadk.WorkflowTriggerLog) (jfadk.WorkflowTriggerLog, error)
	ListActiveWorkflowTriggerLogs(context.Context, string) ([]jfadk.WorkflowTriggerLog, error)
	Run(context.Context, string) (jfadk.Run, bool, error)
}

func (s *Service) invokeWorkflowWithStore(ctx context.Context, store workflowInvocationStore, workflow jfadk.WorkflowDefinition, trigger *jfadk.WorkflowTrigger, triggerType string, inputs map[string]any, matchedEvent map[string]any) (WorkflowInvocationResult, error) {
	if workflow.Status != jfadk.WorkflowStatusEnabled {
		return WorkflowInvocationResult{}, fmt.Errorf("workflow is disabled")
	}
	if trigger != nil {
		if trigger.Status != jfadk.WorkflowTriggerStatusEnabled {
			return WorkflowInvocationResult{}, fmt.Errorf("workflow trigger is disabled")
		}
		active, err := workflowTriggerHasActiveRun(ctx, store, trigger.ID)
		if err != nil {
			return WorkflowInvocationResult{}, err
		}
		if active {
			finishedAt := time.Now().UTC().Format(time.RFC3339Nano)
			log, saveErr := store.SaveWorkflowTriggerLog(ctx, jfadk.WorkflowTriggerLog{
				WorkflowID:   workflow.ID,
				TriggerID:    trigger.ID,
				TriggerType:  trigger.Type,
				Status:       jfadk.WorkflowTriggerLogStatusSkipped,
				Inputs:       cloneMap(inputs),
				MatchedEvent: cloneMap(matchedEvent),
				Error:        "previous trigger run is still active",
				FinishedAt:   finishedAt,
				NodeRuns:     workflowNodeRuns(workflow, trigger, trigger.Type, inputs, matchedEvent, "", "", nil, jfadk.WorkflowTriggerLogStatusSkipped, "previous trigger run is still active", finishedAt, finishedAt),
			})
			if saveErr != nil {
				return WorkflowInvocationResult{}, saveErr
			}
			return WorkflowInvocationResult{Workflow: workflow, Trigger: newSanitizedTrigger(*trigger), Log: log}, nil
		}
	}
	log, err := store.SaveWorkflowTriggerLog(ctx, jfadk.WorkflowTriggerLog{
		WorkflowID:   workflow.ID,
		TriggerID:    triggerID(trigger),
		TriggerType:  defaultString(triggerType, jfadk.WorkflowTriggerTypeManual),
		Status:       jfadk.WorkflowTriggerLogStatusQueued,
		Inputs:       cloneMap(inputs),
		MatchedEvent: cloneMap(matchedEvent),
	})
	if err != nil {
		return WorkflowInvocationResult{}, err
	}
	started := time.Now().UTC().Format(time.RFC3339Nano)
	log.Status = jfadk.WorkflowTriggerLogStatusRunning
	log.StartedAt = started
	log.NodeRuns = workflowNodeRuns(workflow, trigger, log.TriggerType, inputs, matchedEvent, "", "", nil, log.Status, "", started, "")
	log, err = store.SaveWorkflowTriggerLog(ctx, log)
	if err != nil {
		return WorkflowInvocationResult{}, err
	}
	mergedInputs := workflowInputs(workflow, trigger, inputs, matchedEvent)
	message, err := renderWorkflowTemplate(workflow.PromptTemplate, mergedInputs)
	if err == nil && strings.TrimSpace(message) == "" {
		err = fmt.Errorf("workflow prompt template rendered an empty message")
	}
	objective := ""
	if err == nil && strings.TrimSpace(workflow.ObjectiveTemplate) != "" {
		objective, err = renderWorkflowTemplate(workflow.ObjectiveTemplate, mergedInputs)
	}
	if err != nil {
		log.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		log.NodeRuns = workflowNodeRuns(workflow, trigger, log.TriggerType, inputs, matchedEvent, message, objective, nil, jfadk.WorkflowTriggerLogStatusFailed, err.Error(), started, log.FinishedAt)
		log.Result = workflowResultFromError(err)
		log = finishWorkflowLog(ctx, store, log, jfadk.WorkflowTriggerLogStatusFailed, err.Error())
		return WorkflowInvocationResult{Workflow: workflow, Trigger: newSanitizedTriggerPtr(trigger), Log: log}, err
	}
	session, err := s.CreateSession(ctx, CreateSessionRequest{
		AgentID: workflow.AgentID,
		Title:   workflowSessionTitle(workflow.Name, time.Now()),
	})
	if err != nil {
		log.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		log.NodeRuns = workflowNodeRuns(workflow, trigger, log.TriggerType, inputs, matchedEvent, message, objective, nil, jfadk.WorkflowTriggerLogStatusFailed, err.Error(), started, log.FinishedAt)
		log.Result = workflowResultFromError(err)
		log = finishWorkflowLog(ctx, store, log, jfadk.WorkflowTriggerLogStatusFailed, err.Error())
		return WorkflowInvocationResult{Workflow: workflow, Trigger: newSanitizedTriggerPtr(trigger), Log: log}, err
	}
	req := jfadk.ChatRequest{
		AgentID:                workflow.AgentID,
		SessionID:              session.ID,
		Message:                message,
		ProviderID:             workflow.ProviderID,
		Model:                  workflow.Model,
		WorkModeOverride:       workflow.WorkMode,
		PermissionModeOverride: workflow.PermissionMode,
		Objective:              objective,
	}
	response, err := s.Chat(ctx, req)
	if err != nil {
		log.SessionID = session.ID
		log.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		log.NodeRuns = workflowNodeRuns(workflow, trigger, log.TriggerType, inputs, matchedEvent, message, objective, nil, jfadk.WorkflowTriggerLogStatusFailed, err.Error(), started, log.FinishedAt)
		log.Result = workflowResultFromError(err)
		log = finishWorkflowLog(ctx, store, log, jfadk.WorkflowTriggerLogStatusFailed, err.Error())
		s.updateTriggerAfterRun(ctx, trigger, "", err.Error())
		return WorkflowInvocationResult{Workflow: workflow, Trigger: newSanitizedTriggerPtr(trigger), Log: log}, err
	}
	normalized := jfadk.NormalizeChatResponse(response)
	log = applyWorkflowResponse(log, workflow, trigger, inputs, matchedEvent, message, objective, normalized, started, time.Now().UTC())
	log, err = store.SaveWorkflowTriggerLog(ctx, log)
	if err != nil {
		return WorkflowInvocationResult{}, err
	}
	s.updateTriggerAfterRun(ctx, trigger, normalized.Run.ID, log.Error)
	return WorkflowInvocationResult{Workflow: workflow, Trigger: newSanitizedTriggerPtr(trigger), Log: log, Response: &normalized}, nil
}

func applyWorkflowResponse(
	log jfadk.WorkflowTriggerLog,
	workflow jfadk.WorkflowDefinition,
	trigger *jfadk.WorkflowTrigger,
	inputs map[string]any,
	matchedEvent map[string]any,
	message string,
	objective string,
	response jfadk.ChatResponse,
	started string,
	finishedAt time.Time,
) jfadk.WorkflowTriggerLog {
	log.SessionID = response.Session.ID
	log.RunID = response.Run.ID
	log.Status = workflowLogStatusFromRun(response.Run)
	if log.Status != jfadk.WorkflowTriggerLogStatusRunning && log.Status != jfadk.WorkflowTriggerLogStatusPendingApproval {
		log.FinishedAt = finishedAt.Format(time.RFC3339Nano)
	}
	if response.Run.FailureReason != "" {
		log.Error = response.Run.FailureReason
	}
	log.Result = workflowResultFromResponse(response)
	log.NodeRuns = workflowNodeRuns(workflow, trigger, log.TriggerType, inputs, matchedEvent, message, objective, &response, log.Status, log.Error, started, log.FinishedAt)
	return log
}

func workflowResultFromResponse(response jfadk.ChatResponse) *jfadk.WorkflowResult {
	result := &jfadk.WorkflowResult{
		Format:      "markdown",
		Markdown:    strings.TrimSpace(response.Reply),
		RawResponse: &response,
	}
	if result.Markdown == "" {
		result.Markdown = strings.TrimSpace(response.Run.FailureReason)
	}
	return result
}

func workflowResultFromError(err error) *jfadk.WorkflowResult {
	if err == nil {
		return nil
	}
	return &jfadk.WorkflowResult{
		Format:   "markdown",
		Markdown: strings.TrimSpace(err.Error()),
		JSON: map[string]any{
			"error": strings.TrimSpace(err.Error()),
		},
	}
}

func workflowNodeRuns(
	workflow jfadk.WorkflowDefinition,
	trigger *jfadk.WorkflowTrigger,
	triggerType string,
	inputs map[string]any,
	matchedEvent map[string]any,
	message string,
	objective string,
	response *jfadk.ChatResponse,
	status string,
	errorMessage string,
	startedAt string,
	finishedAt string,
) []jfadk.WorkflowNodeRun {
	status = defaultString(strings.ToUpper(strings.TrimSpace(status)), jfadk.WorkflowTriggerLogStatusRunning)
	errorMessage = strings.TrimSpace(errorMessage)
	triggerNodeID := "trigger:manual"
	triggerTitle := "Manual"
	if trigger != nil {
		triggerNodeID = "trigger:" + strings.TrimSpace(trigger.ID)
		triggerTitle = strings.TrimSpace(trigger.Title)
	}
	if triggerTitle == "" {
		triggerTitle = defaultTriggerTitle(defaultString(triggerType, jfadk.WorkflowTriggerTypeManual))
	}

	triggerStatus := jfadk.WorkflowTriggerLogStatusSucceeded
	startStatus := jfadk.WorkflowTriggerLogStatusSucceeded
	agentStatus := status
	monitorStatus := status
	if status == jfadk.WorkflowTriggerLogStatusSkipped {
		triggerStatus = status
		startStatus = status
		agentStatus = status
		monitorStatus = status
	}
	if strings.TrimSpace(message) == "" && errorMessage != "" {
		startStatus = jfadk.WorkflowTriggerLogStatusFailed
		agentStatus = jfadk.WorkflowTriggerLogStatusSkipped
		monitorStatus = jfadk.WorkflowTriggerLogStatusSkipped
	}

	startOutputs := map[string]any{}
	if strings.TrimSpace(message) != "" {
		startOutputs["message"] = message
	}
	if strings.TrimSpace(objective) != "" {
		startOutputs["objective"] = objective
	}

	agentInputs := map[string]any{}
	if strings.TrimSpace(message) != "" {
		agentInputs["message"] = message
	}
	if strings.TrimSpace(workflow.AgentID) != "" {
		agentInputs["agentId"] = workflow.AgentID
	}
	if strings.TrimSpace(workflow.WorkMode) != "" {
		agentInputs["workMode"] = workflow.WorkMode
	}

	agentOutputs := map[string]any{}
	monitorOutputs := map[string]any{}
	if response != nil {
		agentOutputs["reply"] = response.Reply
		agentOutputs["sessionId"] = response.Session.ID
		agentOutputs["runId"] = response.Run.ID
		monitorOutputs["reply"] = response.Reply
		monitorOutputs["sessionId"] = response.Session.ID
		monitorOutputs["runId"] = response.Run.ID
	}
	if errorMessage != "" {
		monitorOutputs["error"] = errorMessage
	}

	return []jfadk.WorkflowNodeRun{
		{
			NodeID:     triggerNodeID,
			NodeType:   "trigger",
			Title:      triggerTitle,
			Status:     triggerStatus,
			StartedAt:  startedAt,
			FinishedAt: defaultString(finishedAt, startedAt),
			Inputs:     cloneMap(inputs),
			Outputs:    cloneMap(matchedEvent),
			Error:      errorForNode(triggerStatus, errorMessage),
		},
		{
			NodeID:     "start",
			NodeType:   "start",
			Title:      "Start",
			Status:     startStatus,
			StartedAt:  startedAt,
			FinishedAt: defaultString(finishedAt, startedAt),
			Inputs:     cloneMap(inputs),
			Outputs:    startOutputs,
			Error:      errorForNode(startStatus, errorMessage),
		},
		{
			NodeID:     "agent",
			NodeType:   "agent",
			Title:      workflow.Name,
			Status:     agentStatus,
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
			Inputs:     agentInputs,
			Outputs:    agentOutputs,
			Error:      errorForNode(agentStatus, errorMessage),
		},
		{
			NodeID:     "monitor",
			NodeType:   "monitor",
			Title:      "Monitor",
			Status:     monitorStatus,
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
			Outputs:    monitorOutputs,
			Error:      errorForNode(monitorStatus, errorMessage),
		},
	}
}

func errorForNode(status string, message string) string {
	switch status {
	case jfadk.WorkflowTriggerLogStatusFailed, jfadk.WorkflowTriggerLogStatusCancelled, jfadk.WorkflowTriggerLogStatusSkipped:
		return strings.TrimSpace(message)
	default:
		return ""
	}
}

func (s *Service) workflowStore() (*jfadk.Store, error) {
	if s == nil || s.runtime == nil || s.runtime.Store() == nil {
		return nil, fmt.Errorf("adk runtime is unavailable")
	}
	return s.runtime.Store(), nil
}

func (s *Service) validateWorkflowDefinition(ctx context.Context, workflow jfadk.WorkflowDefinition) error {
	if strings.TrimSpace(workflow.Name) == "" {
		return fmt.Errorf("workflow name is required")
	}
	if strings.TrimSpace(workflow.AgentID) == "" {
		return fmt.Errorf("workflow agentId is required")
	}
	agent, ok, err := s.runtime.Store().Agent(ctx, workflow.AgentID)
	if err != nil {
		return err
	}
	if !ok || agent.DeletedAt != nil {
		return fmt.Errorf("workflow agent not found")
	}
	if workflow.Status == jfadk.WorkflowStatusEnabled && agent.Status != jfadk.AgentStatusEnabled {
		return fmt.Errorf("enabled workflow requires an enabled agent")
	}
	if strings.TrimSpace(workflow.PromptTemplate) == "" {
		return fmt.Errorf("workflow promptTemplate is required")
	}
	return nil
}

func (s *Service) prepareWorkflowTriggerSchedule(trigger *jfadk.WorkflowTrigger, now time.Time) error {
	if trigger == nil || trigger.Type != jfadk.WorkflowTriggerTypeSchedule {
		if trigger != nil {
			trigger.NextRunAt = ""
		}
		return nil
	}
	next, err := nextWorkflowScheduleRun(trigger.Config, now)
	if err != nil {
		return err
	}
	if trigger.Status == jfadk.WorkflowTriggerStatusEnabled {
		trigger.NextRunAt = next.Format(time.RFC3339Nano)
	} else {
		trigger.NextRunAt = ""
	}
	return nil
}

func (s *Service) workflowTriggerHasActiveRun(ctx context.Context, triggerID string) (bool, error) {
	return workflowTriggerHasActiveRun(ctx, s.runtime.Store(), triggerID)
}

func workflowTriggerHasActiveRun(ctx context.Context, store workflowInvocationStore, triggerID string) (bool, error) {
	logs, err := store.ListActiveWorkflowTriggerLogs(ctx, triggerID)
	if err != nil {
		return false, err
	}
	active := false
	for _, log := range logs {
		if log.RunID == "" {
			active = true
			continue
		}
		run, ok, err := store.Run(ctx, log.RunID)
		if err != nil {
			return false, err
		}
		if !ok {
			log = finishWorkflowLog(ctx, store, log, jfadk.WorkflowTriggerLogStatusFailed, "run not found")
			continue
		}
		status := workflowLogStatusFromRun(run)
		if status == jfadk.WorkflowTriggerLogStatusRunning || status == jfadk.WorkflowTriggerLogStatusPendingApproval {
			active = true
			continue
		}
		log.Status = status
		if run.FailureReason != "" {
			log.Error = run.FailureReason
		}
		if log.FinishedAt == "" {
			log.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
		_, _ = store.SaveWorkflowTriggerLog(ctx, log)
	}
	return active, nil
}

func (s *Service) reconcileActiveWorkflowLogs(ctx context.Context) {
	store := s.runtime.Store()
	for _, status := range []string{
		jfadk.WorkflowTriggerLogStatusQueued,
		jfadk.WorkflowTriggerLogStatusRunning,
		jfadk.WorkflowTriggerLogStatusPendingApproval,
	} {
		logs, _, err := store.ListWorkflowTriggerLogsPage(ctx, "", "", status, 100, 0)
		if err != nil {
			continue
		}
		for _, log := range logs {
			if log.TriggerID == "" {
				continue
			}
			_, _ = s.workflowTriggerHasActiveRun(ctx, log.TriggerID)
		}
	}
}

func (s *Service) updateTriggerAfterRun(ctx context.Context, trigger *jfadk.WorkflowTrigger, runID string, lastError string) {
	if trigger == nil || s == nil || s.runtime == nil || s.runtime.Store() == nil {
		return
	}
	current, ok, err := s.runtime.Store().WorkflowTrigger(ctx, trigger.ID)
	if err != nil || !ok {
		return
	}
	current.LastRunAt = time.Now().UTC().Format(time.RFC3339Nano)
	current.LastRunID = strings.TrimSpace(runID)
	current.LastError = strings.TrimSpace(lastError)
	_, _ = s.runtime.Store().SaveWorkflowTrigger(ctx, current)
}

func finishWorkflowLog(ctx context.Context, store workflowInvocationStore, log jfadk.WorkflowTriggerLog, status string, message string) jfadk.WorkflowTriggerLog {
	log.Status = status
	log.Error = strings.TrimSpace(message)
	if log.FinishedAt == "" {
		log.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	updated, err := store.SaveWorkflowTriggerLog(ctx, log)
	if err != nil {
		return log
	}
	return updated
}

func workflowLogStatusFromRun(run jfadk.Run) string {
	switch run.Status {
	case jfadk.RunStatusCompleted:
		return jfadk.WorkflowTriggerLogStatusSucceeded
	case jfadk.RunStatusPending:
		return jfadk.WorkflowTriggerLogStatusPendingApproval
	case jfadk.RunStatusCancelled, jfadk.RunStatusDenied:
		return jfadk.WorkflowTriggerLogStatusCancelled
	case jfadk.RunStatusFailed, jfadk.RunStatusTimedOut:
		return jfadk.WorkflowTriggerLogStatusFailed
	default:
		return jfadk.WorkflowTriggerLogStatusRunning
	}
}

func nextWorkflowScheduleRun(config map[string]any, from time.Time) (time.Time, error) {
	cronExpression := strings.TrimSpace(configString(config, "cron"))
	if cronExpression == "" {
		return time.Time{}, fmt.Errorf("schedule cron is required")
	}
	if len(strings.Fields(cronExpression)) != 5 {
		return time.Time{}, fmt.Errorf("schedule cron must contain 5 fields")
	}
	timezone := configString(config, "timezone")
	if strings.TrimSpace(timezone) == "" {
		timezone = "Asia/Shanghai"
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid schedule timezone: %w", err)
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(cronExpression)
	if err != nil {
		return time.Time{}, err
	}
	return schedule.Next(from.In(location)).UTC(), nil
}

func nextRunAtString(config map[string]any, from time.Time) string {
	next, err := nextWorkflowScheduleRun(config, from)
	if err != nil {
		return ""
	}
	return next.Format(time.RFC3339Nano)
}

func renderWorkflowTemplate(raw string, inputs map[string]any) (string, error) {
	tpl, err := template.New("workflow").Option("missingkey=zero").Parse(raw)
	if err != nil {
		return "", err
	}
	var buffer bytes.Buffer
	if err := tpl.Execute(&buffer, inputs); err != nil {
		return "", err
	}
	return strings.TrimSpace(buffer.String()), nil
}

func workflowInputs(workflow jfadk.WorkflowDefinition, trigger *jfadk.WorkflowTrigger, inputs map[string]any, matchedEvent map[string]any) map[string]any {
	merged := cloneMap(workflow.DefaultInputs)
	maps.Copy(merged, inputs)
	merged["workflow"] = map[string]any{"id": workflow.ID, "name": workflow.Name}
	if trigger != nil {
		merged["trigger"] = map[string]any{"id": trigger.ID, "type": trigger.Type, "title": trigger.Title}
	}
	if matchedEvent != nil {
		merged["event"] = matchedEvent
	}
	merged["now"] = time.Now().UTC().Format(time.RFC3339Nano)
	return merged
}

func evaluateMarketThresholdTrigger(trigger jfadk.WorkflowTrigger, events []map[string]any, now time.Time) ([]map[string]any, bool) {
	instruments := map[string]struct{}{}
	for _, instrumentID := range configStringSlice(trigger.Config, "instrumentIds") {
		instruments[strings.ToUpper(strings.TrimSpace(instrumentID))] = struct{}{}
	}
	if len(instruments) == 0 {
		return nil, false
	}
	path := configString(trigger.Config, "snapshotPath")
	if strings.TrimSpace(path) == "" {
		path = "snapshot.price"
	}
	threshold, ok := configFloat(trigger.Config, "value")
	if !ok {
		return nil, false
	}
	edge := strings.ToLower(strings.TrimSpace(configString(trigger.Config, "edge")))
	if edge == "" {
		edge = "cross_up"
	}
	operator := strings.TrimSpace(configString(trigger.Config, "operator"))
	cooldownSec := configInt(trigger.Config, "cooldownSec", defaultMarketThresholdCooldown)
	state := ensureConfigState(trigger.Config)
	lastValues := ensureStateMap(state, "lastValues")
	lastTriggeredAt := ensureStateMap(state, "lastTriggeredAt")
	matches := []map[string]any{}
	changed := false
	for _, event := range events {
		instrumentID := eventInstrumentID(event)
		if instrumentID == "" {
			continue
		}
		if _, ok := instruments[instrumentID]; !ok {
			continue
		}
		current, ok := numericAtPath(event, path)
		if !ok {
			if payload, payloadOK := event["payload"].(map[string]any); payloadOK {
				current, ok = numericAtPath(payload, path)
			}
		}
		if !ok {
			continue
		}
		previous, hadPrevious := anyFloat(lastValues[instrumentID])
		fired := thresholdFired(edge, operator, previous, hadPrevious, current, threshold)
		lastValues[instrumentID] = current
		changed = true
		if !fired || !cooldownAllows(lastTriggeredAt[instrumentID], now, cooldownSec) {
			continue
		}
		lastTriggeredAt[instrumentID] = now.Format(time.RFC3339Nano)
		matched := cloneMap(event)
		matched["threshold"] = map[string]any{
			"instrumentId": instrumentID,
			"path":         path,
			"edge":         edge,
			"operator":     operator,
			"value":        threshold,
			"previous":     previous,
			"current":      current,
		}
		matches = append(matches, matched)
		changed = true
	}
	return matches, changed
}

func workflowEventMatches(config map[string]any, event jfadk.WorkflowEvent) bool {
	if expected := strings.TrimSpace(configString(config, "source")); expected != "" && expected != event.Source {
		return false
	}
	if expected := strings.TrimSpace(configString(config, "eventType")); expected != "" && expected != event.Type {
		return false
	}
	if expected := strings.TrimSpace(configString(config, "entityId")); expected != "" && expected != event.EntityID {
		return false
	}
	if expected := strings.TrimSpace(configString(config, "category")); expected != "" && expected != mapString(event.Payload, "category") {
		return false
	}
	if expected := strings.TrimSpace(configString(config, "level")); expected != "" && expected != mapString(event.Payload, "level") {
		return false
	}
	return true
}

func eventTriggerCooldownAllows(trigger *jfadk.WorkflowTrigger, now time.Time) bool {
	if trigger == nil {
		return false
	}
	cooldownSec := configInt(trigger.Config, "cooldownSec", 0)
	state := ensureConfigState(trigger.Config)
	last := state["lastTriggeredAt"]
	if !cooldownAllows(last, now, cooldownSec) {
		return false
	}
	state["lastTriggeredAt"] = now.Format(time.RFC3339Nano)
	return true
}

func thresholdFired(edge string, operator string, previous float64, hadPrevious bool, current float64, threshold float64) bool {
	switch edge {
	case "cross_down":
		return hadPrevious && previous >= threshold && current < threshold
	case "above":
		return compareThreshold(defaultString(operator, ">"), current, threshold)
	case "below":
		return compareThreshold(defaultString(operator, "<"), current, threshold)
	default:
		return hadPrevious && previous <= threshold && current > threshold
	}
}

func compareThreshold(operator string, current float64, threshold float64) bool {
	switch strings.TrimSpace(operator) {
	case ">=":
		return current >= threshold
	case "<":
		return current < threshold
	case "<=":
		return current <= threshold
	default:
		return current > threshold
	}
}

func validateWorkflowTrigger(trigger jfadk.WorkflowTrigger) error {
	if strings.TrimSpace(trigger.WorkflowID) == "" {
		return fmt.Errorf("workflowId is required")
	}
	switch trigger.Type {
	case jfadk.WorkflowTriggerTypeSchedule:
		_, err := nextWorkflowScheduleRun(trigger.Config, time.Now().UTC())
		return err
	case jfadk.WorkflowTriggerTypeManual, jfadk.WorkflowTriggerTypeWebhook, jfadk.WorkflowTriggerTypeEvent:
		return nil
	case jfadk.WorkflowTriggerTypeMarketThreshold:
		if len(configStringSlice(trigger.Config, "instrumentIds")) == 0 {
			return fmt.Errorf("market threshold trigger requires instrumentIds")
		}
		if _, ok := configFloat(trigger.Config, "value"); !ok {
			return fmt.Errorf("market threshold trigger requires numeric value")
		}
		return nil
	default:
		return fmt.Errorf("unsupported workflow trigger type %q", trigger.Type)
	}
}

func normalizeWorkflowStatus(input string, fallback string) string {
	status := strings.ToUpper(strings.TrimSpace(input))
	if status == "" {
		status = strings.ToUpper(strings.TrimSpace(fallback))
	}
	if status == jfadk.WorkflowStatusDisabled {
		return jfadk.WorkflowStatusDisabled
	}
	return jfadk.WorkflowStatusEnabled
}

func normalizeTriggerStatus(input string, fallback string) string {
	status := strings.ToUpper(strings.TrimSpace(input))
	if status == "" {
		status = strings.ToUpper(strings.TrimSpace(fallback))
	}
	switch status {
	case jfadk.WorkflowTriggerStatusDisabled, jfadk.WorkflowTriggerStatusError:
		return status
	default:
		return jfadk.WorkflowTriggerStatusEnabled
	}
}

func normalizeWorkflowWorkMode(input string, fallback string) string {
	mode := strings.ToLower(strings.TrimSpace(input))
	if mode == "" {
		mode = strings.ToLower(strings.TrimSpace(fallback))
	}
	switch mode {
	case jfadk.WorkModeChat, jfadk.WorkModeLoop:
		return mode
	default:
		return jfadk.WorkModeTask
	}
}

func normalizeWorkflowPermissionMode(input string, fallback string) string {
	mode := strings.ToLower(strings.TrimSpace(input))
	if mode == "" {
		mode = strings.ToLower(strings.TrimSpace(fallback))
	}
	switch mode {
	case "", jfadk.PermissionModeApproval, jfadk.PermissionModeLessApproval, jfadk.PermissionModeAll:
		return mode
	default:
		return jfadk.PermissionModeApproval
	}
}

func normalizeTriggerType(input string, fallback string) string {
	value := strings.ToLower(strings.TrimSpace(input))
	if value == "" {
		value = strings.ToLower(strings.TrimSpace(fallback))
	}
	switch value {
	case jfadk.WorkflowTriggerTypeSchedule, jfadk.WorkflowTriggerTypeWebhook, jfadk.WorkflowTriggerTypeEvent, jfadk.WorkflowTriggerTypeMarketThreshold:
		return value
	default:
		return jfadk.WorkflowTriggerTypeManual
	}
}

func defaultTriggerTitle(triggerType string) string {
	switch triggerType {
	case jfadk.WorkflowTriggerTypeSchedule:
		return "定时触发"
	case jfadk.WorkflowTriggerTypeWebhook:
		return "Webhook"
	case jfadk.WorkflowTriggerTypeEvent:
		return "事件触发"
	case jfadk.WorkflowTriggerTypeMarketThreshold:
		return "行情阈值"
	default:
		return "手动触发"
	}
}

func normalizedWorkflowPage(limit int, offset int) (int, int) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func sanitizeWorkflowTrigger(trigger jfadk.WorkflowTrigger) jfadk.WorkflowTrigger {
	trigger.HasSecret = trigger.HasSecret || strings.TrimSpace(trigger.SecretHash) != ""
	trigger.SecretHash = ""
	return trigger
}

func newSanitizedTrigger(trigger jfadk.WorkflowTrigger) *jfadk.WorkflowTrigger {
	sanitized := sanitizeWorkflowTrigger(trigger)
	return &sanitized
}

func newSanitizedTriggerPtr(trigger *jfadk.WorkflowTrigger) *jfadk.WorkflowTrigger {
	if trigger == nil {
		return nil
	}
	return newSanitizedTrigger(*trigger)
}

func triggerID(trigger *jfadk.WorkflowTrigger) string {
	if trigger == nil {
		return ""
	}
	return trigger.ID
}

func workflowSessionTitle(name string, now time.Time) string {
	if strings.TrimSpace(name) == "" {
		name = "ADK 工作流"
	}
	return strings.TrimSpace(name) + " - " + now.Format("2006-01-02 15:04")
}

func newWorkflowSecret() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "wfsec-" + base64.RawURLEncoding.EncodeToString(raw), nil
}

func hashWorkflowSecret(secret string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(secret)))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func verifyWorkflowSecret(secret string, hash string) bool {
	secret = strings.TrimSpace(secret)
	hash = strings.TrimSpace(hash)
	return secret != "" && hash != "" && hashWorkflowSecret(secret) == hash
}

func cloneMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	maps.Copy(out, input)
	return out
}

func normalizeStringList(values []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func configString(config map[string]any, key string) string {
	if value, ok := config[key]; ok {
		switch typed := value.(type) {
		case string:
			return typed
		case fmt.Stringer:
			return typed.String()
		}
	}
	return ""
}

func configStringSlice(config map[string]any, key string) []string {
	value, ok := config[key]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return normalizeStringList(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, strings.TrimSpace(fmt.Sprint(item)))
		}
		return normalizeStringList(out)
	case string:
		parts := strings.Split(typed, ",")
		return normalizeStringList(parts)
	default:
		return nil
	}
}

func configFloat(config map[string]any, key string) (float64, bool) {
	return anyFloat(config[key])
}

func configInt(config map[string]any, key string, fallback int) int {
	value, ok := anyFloat(config[key])
	if !ok {
		return fallback
	}
	return int(math.Round(value))
}

func anyFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func numericAtPath(root map[string]any, path string) (float64, bool) {
	value := any(root)
	for part := range strings.SplitSeq(path, ".") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		current, ok := value.(map[string]any)
		if !ok {
			return 0, false
		}
		value = current[part]
	}
	return anyFloat(value)
}

func eventInstrumentID(event map[string]any) string {
	for _, key := range []string{"entityId", "instrumentId"} {
		if value := strings.ToUpper(strings.TrimSpace(fmt.Sprint(event[key]))); value != "" && value != "<NIL>" {
			return value
		}
	}
	if instrument, ok := event["instrument"].(map[string]any); ok {
		if value := strings.ToUpper(strings.TrimSpace(fmt.Sprint(instrument["instrumentId"]))); value != "" && value != "<NIL>" {
			return value
		}
	}
	if payload, ok := event["payload"].(map[string]any); ok {
		return eventInstrumentID(payload)
	}
	return ""
}

func ensureConfigState(config map[string]any) map[string]any {
	if config == nil {
		return map[string]any{}
	}
	if state, ok := config["state"].(map[string]any); ok {
		return state
	}
	state := map[string]any{}
	config["state"] = state
	return state
}

func ensureStateMap(state map[string]any, key string) map[string]any {
	if current, ok := state[key].(map[string]any); ok {
		return current
	}
	out := map[string]any{}
	state[key] = out
	return out
}

func cooldownAllows(value any, now time.Time, cooldownSec int) bool {
	if cooldownSec <= 0 {
		return true
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" {
		return true
	}
	previous, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		previous, err = time.Parse(time.RFC3339, text)
	}
	if err != nil {
		return true
	}
	return now.Sub(previous.UTC()) >= time.Duration(cooldownSec)*time.Second
}

func eventAsMap(event jfadk.WorkflowEvent) map[string]any {
	out := map[string]any{
		"type":     event.Type,
		"source":   event.Source,
		"entityId": event.EntityID,
		"at":       event.At,
		"payload":  cloneMap(event.Payload),
	}
	if event.ID != "" {
		out["id"] = event.ID
	}
	for key, value := range event.Payload {
		if _, exists := out[key]; !exists {
			out[key] = value
		}
	}
	return out
}

func mapString(input map[string]any, key string) string {
	if input == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(input[key]))
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func defaultString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func dailyStockReviewPrompt() string {
	return strings.TrimSpace(`请进行每日股票盘点。

关注列表：{{ .watchlist }}
市场范围：{{ .market }}

请使用可用工具读取行情快照、近期 K 线、组合摘要、风险状态和风险事件。输出：
1. 关注标的的关键变化和异常；
2. 持仓、订单和资金风险摘要；
3. 今日需要人工确认的待办事项；
4. 如果需要后续跟踪，请用 tasks.create 创建任务。

不要承诺收益，不要直接下单。涉及写入、策略保存或交易动作时必须保留审批。`)
}
