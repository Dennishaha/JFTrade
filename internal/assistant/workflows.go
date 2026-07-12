package assistant

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	workflowrules "github.com/jftrade/jftrade-main/internal/assistant/workflow"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

const (
	defaultWorkflowSchedulerInterval = 30 * time.Second
	defaultWorkflowScheduleBatchSize = 20
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

type WorkflowStartResult struct {
	Accepted bool                     `json:"accepted"`
	Workflow jfadk.WorkflowDefinition `json:"workflow"`
	Trigger  *jfadk.WorkflowTrigger   `json:"trigger,omitempty"`
	Log      jfadk.WorkflowTriggerLog `json:"log"`
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
		AgentID:           jfadk.DefaultBuiltinAgentID,
		WorkMode:          jfadk.WorkModeLoop,
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
	workflow.Status = workflowrules.NormalizeWorkflowStatus(payload.Status, workflow.Status)
	workflow.AgentID = strings.TrimSpace(payload.AgentID)
	if mode := strings.ToLower(strings.TrimSpace(payload.WorkMode)); mode != "" && mode != jfadk.WorkModeChat && mode != jfadk.WorkModeLoop {
		return jfadk.WorkflowDefinition{}, fmt.Errorf("invalid workflow work mode")
	}
	workflow.WorkMode = workflowrules.NormalizeWorkflowWorkMode(payload.WorkMode, workflow.WorkMode)
	workflow.ProviderID = strings.TrimSpace(payload.ProviderID)
	workflow.Model = strings.TrimSpace(payload.Model)
	workflow.PermissionMode = workflowrules.NormalizeWorkflowPermissionMode(payload.PermissionMode, workflow.PermissionMode)
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

func (s *Service) GetWorkflowTrigger(ctx context.Context, workflowID string, triggerID string) (jfadk.WorkflowTrigger, error) {
	store, err := s.workflowStore()
	if err != nil {
		return jfadk.WorkflowTrigger{}, err
	}
	if _, err := s.GetWorkflow(ctx, workflowID); err != nil {
		return jfadk.WorkflowTrigger{}, err
	}
	trigger, ok, err := store.WorkflowTrigger(ctx, triggerID)
	if err != nil {
		return jfadk.WorkflowTrigger{}, err
	}
	if !ok || trigger.WorkflowID != strings.TrimSpace(workflowID) || trigger.DeletedAt != nil {
		return jfadk.WorkflowTrigger{}, fmt.Errorf("workflow trigger not found")
	}
	return sanitizeWorkflowTrigger(trigger), nil
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
	trigger.Type = workflowrules.NormalizeTriggerType(payload.Type, trigger.Type)
	trigger.Title = strings.TrimSpace(payload.Title)
	trigger.Status = workflowrules.NormalizeTriggerStatus(payload.Status, trigger.Status)
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
		trigger.Title = workflowrules.DefaultTriggerTitle(trigger.Type)
	}
	if err := s.prepareWorkflowTriggerSchedule(&trigger, time.Now().UTC()); err != nil {
		return WorkflowTriggerSaveResult{}, err
	}
	if err := workflowrules.ValidateTrigger(trigger); err != nil {
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

func (s *Service) GetWorkflowTriggerLog(ctx context.Context, logID string) (jfadk.WorkflowTriggerLog, error) {
	store, err := s.workflowStore()
	if err != nil {
		return jfadk.WorkflowTriggerLog{}, err
	}
	log, ok, err := store.WorkflowTriggerLog(ctx, strings.TrimSpace(logID))
	if err != nil {
		return jfadk.WorkflowTriggerLog{}, err
	}
	if !ok {
		return jfadk.WorkflowTriggerLog{}, fmt.Errorf("workflow run not found")
	}
	return log, nil
}

func (s *Service) RunWorkflow(ctx context.Context, workflowID string, inputs map[string]any) (WorkflowInvocationResult, error) {
	workflow, err := s.GetWorkflow(ctx, workflowID)
	if err != nil {
		return WorkflowInvocationResult{}, err
	}
	return s.invokeWorkflow(ctx, workflow, nil, jfadk.WorkflowTriggerTypeManual, inputs, nil)
}

func (s *Service) StartWorkflow(ctx context.Context, workflowID string, inputs map[string]any) (WorkflowStartResult, error) {
	workflow, err := s.GetWorkflow(ctx, workflowID)
	if err != nil {
		return WorkflowStartResult{}, err
	}
	return s.startWorkflowAsync(ctx, workflow, nil, jfadk.WorkflowTriggerTypeManual, inputs, nil)
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

func (s *Service) StartWorkflowTrigger(ctx context.Context, triggerID string, inputs map[string]any) (WorkflowStartResult, error) {
	store, err := s.workflowStore()
	if err != nil {
		return WorkflowStartResult{}, err
	}
	trigger, ok, err := store.WorkflowTrigger(ctx, triggerID)
	if err != nil {
		return WorkflowStartResult{}, err
	}
	if !ok || trigger.DeletedAt != nil {
		return WorkflowStartResult{}, fmt.Errorf("workflow trigger not found")
	}
	workflow, err := s.GetWorkflow(ctx, trigger.WorkflowID)
	if err != nil {
		return WorkflowStartResult{}, err
	}
	return s.startWorkflowAsync(ctx, workflow, &trigger, trigger.Type, inputs, nil)
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
		for _, instrumentID := range workflowrules.ConfigStringSlice(trigger.Config, "instrumentIds") {
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
				matches, changed := workflowrules.EvaluateMarketThresholdTrigger(trigger, []map[string]any{eventAsMap(event)}, time.Now().UTC())
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
		if !workflowrules.EventMatches(trigger.Config, event) {
			continue
		}
		if !workflowrules.EventCooldownAllows(&trigger, time.Now().UTC()) {
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
	prepared, accepted, err := prepareWorkflowInvocation(ctx, store, workflow, trigger, triggerType, inputs, matchedEvent)
	if err != nil || !accepted {
		return prepared, err
	}
	return s.executeQueuedWorkflowInvocation(ctx, store, workflow, trigger, inputs, matchedEvent, prepared.Log)
}

func (s *Service) startWorkflowAsync(ctx context.Context, workflow jfadk.WorkflowDefinition, trigger *jfadk.WorkflowTrigger, triggerType string, inputs map[string]any, matchedEvent map[string]any) (WorkflowStartResult, error) {
	store, err := s.workflowStore()
	if err != nil {
		return WorkflowStartResult{}, err
	}
	prepared, accepted, err := prepareWorkflowInvocation(ctx, store, workflow, trigger, triggerType, inputs, matchedEvent)
	result := WorkflowStartResult{
		Accepted: accepted,
		Workflow: prepared.Workflow,
		Trigger:  prepared.Trigger,
		Log:      prepared.Log,
	}
	if err != nil || !accepted {
		return result, err
	}
	workflowInputs := cloneMap(inputs)
	matched := cloneMap(matchedEvent)
	var triggerCopy *jfadk.WorkflowTrigger
	if trigger != nil {
		copyValue := *trigger
		triggerCopy = &copyValue
	}
	go s.executeQueuedWorkflowBackground(context.WithoutCancel(ctx), store, workflow, triggerCopy, workflowInputs, matched, prepared.Log)
	return result, nil
}

func prepareWorkflowInvocation(ctx context.Context, store workflowInvocationStore, workflow jfadk.WorkflowDefinition, trigger *jfadk.WorkflowTrigger, triggerType string, inputs map[string]any, matchedEvent map[string]any) (WorkflowInvocationResult, bool, error) {
	if err := validateWorkflowInvocation(workflow, trigger); err != nil {
		return WorkflowInvocationResult{}, false, err
	}
	if result, handled, err := invokeWorkflowActiveTriggerGuard(ctx, store, workflow, trigger, inputs, matchedEvent); handled || err != nil {
		return result, false, err
	}
	log, err := queueWorkflowInvocationLog(ctx, store, workflow, trigger, triggerType, inputs, matchedEvent)
	if err != nil {
		return WorkflowInvocationResult{}, false, err
	}
	return WorkflowInvocationResult{Workflow: workflow, Trigger: newSanitizedTriggerPtr(trigger), Log: log}, true, nil
}

func (s *Service) executeQueuedWorkflowInvocation(ctx context.Context, store workflowInvocationStore, workflow jfadk.WorkflowDefinition, trigger *jfadk.WorkflowTrigger, inputs map[string]any, matchedEvent map[string]any, log jfadk.WorkflowTriggerLog) (WorkflowInvocationResult, error) {
	log, started, err := markWorkflowInvocationRunning(ctx, store, workflow, trigger, inputs, matchedEvent, log)
	if err != nil {
		return WorkflowInvocationResult{}, err
	}
	message, objective, err := renderWorkflowInvocationMessage(workflow, trigger, inputs, matchedEvent)
	if err != nil {
		return s.failWorkflowInvocation(ctx, store, workflow, trigger, inputs, matchedEvent, log, started, message, objective, "", err, false)
	}
	session, err := s.CreateSession(ctx, CreateSessionRequest{
		AgentID:      workflow.AgentID,
		Title:        workflowSessionTitle(workflow.Name, time.Now()),
		WorkflowID:   workflow.ID,
		WorkflowName: workflow.Name,
	})
	if err != nil {
		return s.failWorkflowInvocation(ctx, store, workflow, trigger, inputs, matchedEvent, log, started, message, objective, "", err, false)
	}
	normalized, err := s.runWorkflowCanvas(ctx, workflow, trigger, session.ID, message, objective, inputs, matchedEvent)
	if err != nil {
		return s.failWorkflowInvocation(ctx, store, workflow, trigger, inputs, matchedEvent, log, started, message, objective, session.ID, err, true)
	}
	log = applyWorkflowResponse(log, workflow, trigger, inputs, matchedEvent, message, objective, normalized, started, time.Now().UTC())
	log, err = store.SaveWorkflowTriggerLog(ctx, log)
	if err != nil {
		return WorkflowInvocationResult{}, err
	}
	s.updateTriggerAfterRun(ctx, trigger, normalized.Run.ID, log.Error)
	return WorkflowInvocationResult{Workflow: workflow, Trigger: newSanitizedTriggerPtr(trigger), Log: log, Response: &normalized}, nil
}

func (s *Service) executeQueuedWorkflowBackground(ctx context.Context, store workflowInvocationStore, workflow jfadk.WorkflowDefinition, trigger *jfadk.WorkflowTrigger, inputs map[string]any, matchedEvent map[string]any, log jfadk.WorkflowTriggerLog) {
	if ctx == nil {
		ctx = context.Background()
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			s.finishQueuedWorkflowBackgroundFailure(store, workflow, trigger, inputs, matchedEvent, log, fmt.Errorf("workflow background panic: %v", recovered))
		}
	}()
	result, err := s.executeQueuedWorkflowInvocation(ctx, store, workflow, trigger, inputs, matchedEvent, log)
	if err != nil && result.Log.Status != jfadk.WorkflowTriggerLogStatusFailed {
		s.finishQueuedWorkflowBackgroundFailure(store, workflow, trigger, inputs, matchedEvent, log, err)
	}
}

func (s *Service) finishQueuedWorkflowBackgroundFailure(store workflowInvocationStore, workflow jfadk.WorkflowDefinition, trigger *jfadk.WorkflowTrigger, inputs map[string]any, matchedEvent map[string]any, log jfadk.WorkflowTriggerLog, cause error) {
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cleanupCancel()
	_, _ = s.failWorkflowInvocation(cleanupCtx, store, workflow, trigger, inputs, matchedEvent, log, "", "", "", log.SessionID, cause, true)
}

func validateWorkflowInvocation(workflow jfadk.WorkflowDefinition, trigger *jfadk.WorkflowTrigger) error {
	if workflow.Status != jfadk.WorkflowStatusEnabled {
		return fmt.Errorf("workflow is disabled")
	}
	if trigger != nil && trigger.Status != jfadk.WorkflowTriggerStatusEnabled {
		return fmt.Errorf("workflow trigger is disabled")
	}
	return nil
}

func invokeWorkflowActiveTriggerGuard(ctx context.Context, store workflowInvocationStore, workflow jfadk.WorkflowDefinition, trigger *jfadk.WorkflowTrigger, inputs map[string]any, matchedEvent map[string]any) (WorkflowInvocationResult, bool, error) {
	if trigger == nil {
		return WorkflowInvocationResult{}, false, nil
	}
	active, err := workflowTriggerHasActiveRun(ctx, store, trigger.ID)
	if err != nil {
		return WorkflowInvocationResult{}, false, err
	}
	if !active {
		return WorkflowInvocationResult{}, false, nil
	}
	finishedAt := time.Now().UTC().Format(time.RFC3339Nano)
	log, err := store.SaveWorkflowTriggerLog(ctx, jfadk.WorkflowTriggerLog{
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
	if err != nil {
		return WorkflowInvocationResult{}, false, err
	}
	return WorkflowInvocationResult{Workflow: workflow, Trigger: newSanitizedTrigger(*trigger), Log: log}, true, nil
}

func queueWorkflowInvocationLog(ctx context.Context, store workflowInvocationStore, workflow jfadk.WorkflowDefinition, trigger *jfadk.WorkflowTrigger, triggerType string, inputs map[string]any, matchedEvent map[string]any) (jfadk.WorkflowTriggerLog, error) {
	log, err := store.SaveWorkflowTriggerLog(ctx, jfadk.WorkflowTriggerLog{
		WorkflowID:   workflow.ID,
		TriggerID:    triggerID(trigger),
		TriggerType:  defaultString(triggerType, jfadk.WorkflowTriggerTypeManual),
		Status:       jfadk.WorkflowTriggerLogStatusQueued,
		Inputs:       cloneMap(inputs),
		MatchedEvent: cloneMap(matchedEvent),
	})
	if err != nil {
		return jfadk.WorkflowTriggerLog{}, err
	}
	return log, nil
}

func markWorkflowInvocationRunning(ctx context.Context, store workflowInvocationStore, workflow jfadk.WorkflowDefinition, trigger *jfadk.WorkflowTrigger, inputs map[string]any, matchedEvent map[string]any, log jfadk.WorkflowTriggerLog) (jfadk.WorkflowTriggerLog, string, error) {
	started := time.Now().UTC().Format(time.RFC3339Nano)
	log.Status = jfadk.WorkflowTriggerLogStatusRunning
	log.StartedAt = started
	log.NodeRuns = workflowNodeRuns(workflow, trigger, log.TriggerType, inputs, matchedEvent, "", "", nil, log.Status, "", started, "")
	log, err := store.SaveWorkflowTriggerLog(ctx, log)
	if err != nil {
		return jfadk.WorkflowTriggerLog{}, "", err
	}
	return log, started, nil
}

func renderWorkflowInvocationMessage(workflow jfadk.WorkflowDefinition, trigger *jfadk.WorkflowTrigger, inputs map[string]any, matchedEvent map[string]any) (string, string, error) {
	mergedInputs := workflowInputs(workflow, trigger, inputs, matchedEvent)
	message, err := renderWorkflowTemplate(workflow.PromptTemplate, mergedInputs)
	if err == nil && strings.TrimSpace(message) == "" {
		err = fmt.Errorf("workflow prompt template rendered an empty message")
	}
	objective := ""
	if err == nil && strings.TrimSpace(workflow.ObjectiveTemplate) != "" {
		objective, err = renderWorkflowTemplate(workflow.ObjectiveTemplate, mergedInputs)
	}
	return message, objective, err
}

func (s *Service) runWorkflowCanvas(
	ctx context.Context,
	workflow jfadk.WorkflowDefinition,
	trigger *jfadk.WorkflowTrigger,
	sessionID string,
	message string,
	objective string,
	inputs map[string]any,
	matchedEvent map[string]any,
) (jfadk.ChatResponse, error) {
	rendered, err := renderWorkflowCanvasTemplates(workflow, trigger, inputs, matchedEvent)
	if err != nil {
		return jfadk.ChatResponse{}, err
	}
	if s == nil || s.runtime == nil {
		return jfadk.ChatResponse{}, fmt.Errorf("adk runtime is unavailable")
	}
	response, err := s.runtime.RunCanvasWorkflow(ctx, jfadk.WorkflowCanvasRunRequest{
		Workflow: rendered, SessionID: sessionID, Message: message, Objective: objective,
	})
	if err != nil {
		return jfadk.ChatResponse{}, err
	}
	return jfadk.NormalizeChatResponse(response), nil
}

func (s *Service) failWorkflowInvocation(
	ctx context.Context,
	store workflowInvocationStore,
	workflow jfadk.WorkflowDefinition,
	trigger *jfadk.WorkflowTrigger,
	inputs map[string]any,
	matchedEvent map[string]any,
	log jfadk.WorkflowTriggerLog,
	started string,
	message string,
	objective string,
	sessionID string,
	cause error,
	updateTrigger bool,
) (WorkflowInvocationResult, error) {
	log.SessionID = strings.TrimSpace(sessionID)
	log.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	log.NodeRuns = workflowNodeRuns(workflow, trigger, log.TriggerType, inputs, matchedEvent, message, objective, nil, jfadk.WorkflowTriggerLogStatusFailed, cause.Error(), started, log.FinishedAt)
	log.Result = workflowResultFromError(cause)
	log = finishWorkflowLog(ctx, store, log, jfadk.WorkflowTriggerLogStatusFailed, cause.Error())
	if updateTrigger {
		s.updateTriggerAfterRun(ctx, trigger, "", cause.Error())
	}
	return WorkflowInvocationResult{Workflow: workflow, Trigger: newSanitizedTriggerPtr(trigger), Log: log}, cause
}
