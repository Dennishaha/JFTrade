package assistant

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	jfadkruntime "github.com/jftrade/jftrade-main/internal/assistant/engine/workflowruntime"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	workflowrules "github.com/jftrade/jftrade-main/internal/assistant/workflow"
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
	Trigger assistantmodel.WorkflowTrigger `json:"trigger"`
	Secret  string                         `json:"secret,omitempty"`
}

type WorkflowInvocationResult struct {
	Workflow assistantmodel.WorkflowDefinition `json:"workflow"`
	Trigger  *assistantmodel.WorkflowTrigger   `json:"trigger,omitempty"`
	Log      assistantmodel.WorkflowTriggerLog `json:"log"`
	Response *assistantmodel.ChatResponse      `json:"response,omitempty"`
}

type WorkflowStartResult struct {
	Accepted bool                              `json:"accepted"`
	Workflow assistantmodel.WorkflowDefinition `json:"workflow"`
	Trigger  *assistantmodel.WorkflowTrigger   `json:"trigger,omitempty"`
	Log      assistantmodel.WorkflowTriggerLog `json:"log"`
}

type WorkflowScheduler struct {
	service  *Service
	interval time.Duration
	mu       sync.Mutex
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	started  bool
	stopped  bool
}

func (s *Service) StartWorkflowScheduler(ctx context.Context) {
	if s == nil || !s.Available() {
		return
	}
	interval := s.workflowInterval
	if interval <= 0 {
		interval = defaultWorkflowSchedulerInterval
	}
	scheduler, ownerCtx, release, admitted := s.reserveWorkflowScheduler(interval)
	if !admitted {
		return
	}
	defer release()
	if ctx == nil {
		ctx = context.Background()
	}
	initCtx, cancelInit := context.WithCancel(ctx)
	stopOwnerCancel := context.AfterFunc(ownerCtx, cancelInit)
	if err := s.EnsureBuiltinWorkflowTemplates(initCtx); err != nil {
		log.Printf("JFTrade ADK workflow template initialization failed: %v", err)
	}
	stopOwnerCancel()
	cancelInit()
	scheduler.Start(ctx)
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
	workflow := assistantmodel.WorkflowDefinition{
		ID:                "daily-stock-review",
		Name:              "每日股票盘点",
		Description:       "交易日上午盘点关注列表、持仓、风险事件与待办事项。",
		Status:            assistantmodel.WorkflowStatusDisabled,
		AgentID:           assistantmodel.DefaultBuiltinAgentID,
		WorkMode:          assistantmodel.WorkModeLoop,
		PermissionMode:    assistantmodel.PermissionModeApproval,
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
	_, err = store.SaveWorkflowTrigger(ctx, assistantmodel.WorkflowTrigger{
		ID:         "daily-stock-review-schedule",
		WorkflowID: created.ID,
		Type:       assistantmodel.WorkflowTriggerTypeSchedule,
		Title:      "工作日上午 8 点",
		Status:     assistantmodel.WorkflowTriggerStatusDisabled,
		Config: map[string]any{
			"cron":     "0 8 * * 1-5",
			"timezone": "Asia/Shanghai",
		},
	})
	return err
}

func (s *Service) ListWorkflows(ctx context.Context, query WorkflowQuery) (Page[assistantmodel.WorkflowDefinition], error) {
	if s == nil || s.runtime == nil || s.runtime.Store() == nil {
		return Page[assistantmodel.WorkflowDefinition]{}, fmt.Errorf("adk runtime is unavailable")
	}
	limit, offset := normalizedWorkflowPage(query.Limit, query.Offset)
	items, total, err := s.runtime.Store().ListWorkflowDefinitionsPage(ctx, query.Status, limit, offset)
	if err != nil {
		return Page[assistantmodel.WorkflowDefinition]{}, err
	}
	return Page[assistantmodel.WorkflowDefinition]{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

func (s *Service) GetWorkflow(ctx context.Context, workflowID string) (assistantmodel.WorkflowDefinition, error) {
	store, err := s.workflowStore()
	if err != nil {
		return assistantmodel.WorkflowDefinition{}, err
	}
	workflow, ok, err := store.WorkflowDefinition(ctx, workflowID)
	if err != nil {
		return assistantmodel.WorkflowDefinition{}, err
	}
	if !ok || workflow.DeletedAt != nil {
		return assistantmodel.WorkflowDefinition{}, fmt.Errorf("workflow not found")
	}
	return workflow, nil
}

func (s *Service) SaveWorkflow(ctx context.Context, workflowID string, payload assistantmodel.WorkflowDefinitionWriteRequest) (assistantmodel.WorkflowDefinition, error) {
	store, err := s.workflowStore()
	if err != nil {
		return assistantmodel.WorkflowDefinition{}, err
	}
	workflow := assistantmodel.WorkflowDefinition{}
	if strings.TrimSpace(workflowID) != "" {
		existing, ok, err := store.WorkflowDefinition(ctx, workflowID)
		if err != nil {
			return assistantmodel.WorkflowDefinition{}, err
		}
		if !ok || existing.DeletedAt != nil {
			return assistantmodel.WorkflowDefinition{}, fmt.Errorf("workflow not found")
		}
		workflow = existing
	} else if strings.TrimSpace(payload.ID) != "" {
		workflow.ID = strings.TrimSpace(payload.ID)
	}
	workflow.Name = strings.TrimSpace(payload.Name)
	workflow.Description = strings.TrimSpace(payload.Description)
	workflow.Status = workflowrules.NormalizeWorkflowStatus(payload.Status, workflow.Status)
	workflow.AgentID = strings.TrimSpace(payload.AgentID)
	if mode := strings.ToLower(strings.TrimSpace(payload.WorkMode)); mode != "" && mode != assistantmodel.WorkModeChat && mode != assistantmodel.WorkModeLoop {
		return assistantmodel.WorkflowDefinition{}, fmt.Errorf("invalid workflow work mode")
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
		return assistantmodel.WorkflowDefinition{}, err
	}
	workflow, err = store.SaveWorkflowDefinition(ctx, workflow)
	if err != nil {
		return assistantmodel.WorkflowDefinition{}, err
	}
	s.runtime.RecordAudit(ctx, "workflow.saved", workflow.ID, "ADK workflow saved.", map[string]any{"status": workflow.Status})
	return workflow, nil
}

func (s *Service) DeleteWorkflow(ctx context.Context, workflowID string) (assistantmodel.WorkflowDefinition, error) {
	store, err := s.workflowStore()
	if err != nil {
		return assistantmodel.WorkflowDefinition{}, err
	}
	workflow, err := store.DeleteWorkflowDefinition(ctx, workflowID)
	if err != nil {
		return assistantmodel.WorkflowDefinition{}, err
	}
	s.runtime.RecordAudit(ctx, "workflow.deleted", workflow.ID, "ADK workflow disabled and deleted.", nil)
	return workflow, nil
}

func (s *Service) ListWorkflowTriggers(ctx context.Context, workflowID string) ([]assistantmodel.WorkflowTrigger, error) {
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

func (s *Service) GetWorkflowTrigger(ctx context.Context, workflowID string, triggerID string) (assistantmodel.WorkflowTrigger, error) {
	store, err := s.workflowStore()
	if err != nil {
		return assistantmodel.WorkflowTrigger{}, err
	}
	if _, err := s.GetWorkflow(ctx, workflowID); err != nil {
		return assistantmodel.WorkflowTrigger{}, err
	}
	trigger, ok, err := store.WorkflowTrigger(ctx, triggerID)
	if err != nil {
		return assistantmodel.WorkflowTrigger{}, err
	}
	if !ok || trigger.WorkflowID != strings.TrimSpace(workflowID) || trigger.DeletedAt != nil {
		return assistantmodel.WorkflowTrigger{}, fmt.Errorf("workflow trigger not found")
	}
	return sanitizeWorkflowTrigger(trigger), nil
}

func (s *Service) SaveWorkflowTrigger(ctx context.Context, workflowID string, triggerID string, payload assistantmodel.WorkflowTriggerWriteRequest) (WorkflowTriggerSaveResult, error) {
	store, err := s.workflowStore()
	if err != nil {
		return WorkflowTriggerSaveResult{}, err
	}
	workflow, err := s.GetWorkflow(ctx, workflowID)
	if err != nil {
		return WorkflowTriggerSaveResult{}, err
	}
	trigger := assistantmodel.WorkflowTrigger{WorkflowID: workflow.ID}
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
	if trigger.Type == assistantmodel.WorkflowTriggerTypeWebhook && (isCreate || payload.ResetSecret || trigger.SecretHash == "") {
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

func (s *Service) DeleteWorkflowTrigger(ctx context.Context, workflowID string, triggerID string) (assistantmodel.WorkflowTrigger, error) {
	store, err := s.workflowStore()
	if err != nil {
		return assistantmodel.WorkflowTrigger{}, err
	}
	trigger, ok, err := store.WorkflowTrigger(ctx, triggerID)
	if err != nil {
		return assistantmodel.WorkflowTrigger{}, err
	}
	if !ok || trigger.WorkflowID != strings.TrimSpace(workflowID) || trigger.DeletedAt != nil {
		return assistantmodel.WorkflowTrigger{}, fmt.Errorf("workflow trigger not found")
	}
	trigger, err = store.DeleteWorkflowTrigger(ctx, trigger.ID)
	if err != nil {
		return assistantmodel.WorkflowTrigger{}, err
	}
	s.runtime.RecordAudit(ctx, "workflow.trigger.deleted", trigger.ID, "ADK workflow trigger disabled and deleted.", map[string]any{"workflowId": workflowID})
	return sanitizeWorkflowTrigger(trigger), nil
}

func (s *Service) ListWorkflowTriggerLogs(ctx context.Context, query WorkflowTriggerLogQuery) (Page[assistantmodel.WorkflowTriggerLog], error) {
	store, err := s.workflowStore()
	if err != nil {
		return Page[assistantmodel.WorkflowTriggerLog]{}, err
	}
	limit, offset := normalizedWorkflowPage(query.Limit, query.Offset)
	items, total, err := store.ListWorkflowTriggerLogsPage(ctx, query.WorkflowID, query.TriggerID, query.Status, limit, offset)
	if err != nil {
		return Page[assistantmodel.WorkflowTriggerLog]{}, err
	}
	return Page[assistantmodel.WorkflowTriggerLog]{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

func (s *Service) GetWorkflowTriggerLog(ctx context.Context, logID string) (assistantmodel.WorkflowTriggerLog, error) {
	store, err := s.workflowStore()
	if err != nil {
		return assistantmodel.WorkflowTriggerLog{}, err
	}
	log, ok, err := store.WorkflowTriggerLog(ctx, strings.TrimSpace(logID))
	if err != nil {
		return assistantmodel.WorkflowTriggerLog{}, err
	}
	if !ok {
		return assistantmodel.WorkflowTriggerLog{}, fmt.Errorf("workflow run not found")
	}
	return log, nil
}

func (s *Service) RunWorkflow(ctx context.Context, workflowID string, inputs map[string]any) (WorkflowInvocationResult, error) {
	workflow, err := s.GetWorkflow(ctx, workflowID)
	if err != nil {
		return WorkflowInvocationResult{}, err
	}
	return s.invokeWorkflow(ctx, workflow, nil, assistantmodel.WorkflowTriggerTypeManual, inputs, nil)
}

func (s *Service) StartWorkflow(ctx context.Context, workflowID string, inputs map[string]any) (WorkflowStartResult, error) {
	workflow, err := s.GetWorkflow(ctx, workflowID)
	if err != nil {
		return WorkflowStartResult{}, err
	}
	return s.startWorkflowAsync(ctx, workflow, nil, assistantmodel.WorkflowTriggerTypeManual, inputs, nil)
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
	if !ok || trigger.Type != assistantmodel.WorkflowTriggerTypeWebhook || trigger.DeletedAt != nil {
		return WorkflowInvocationResult{}, fmt.Errorf("workflow webhook not found")
	}
	if trigger.Status != assistantmodel.WorkflowTriggerStatusEnabled {
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
	triggers, err := s.runtime.Store().ListEnabledWorkflowTriggersByType(ctx, assistantmodel.WorkflowTriggerTypeMarketThreshold)
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

func (s *Service) HandleWorkflowEvent(ctx context.Context, event assistantmodel.WorkflowEvent) {
	if s == nil {
		return
	}
	event.Payload = cloneMap(event.Payload)
	s.goWorkflowBackground(ctx, func(runCtx context.Context) {
		s.handleWorkflowEvent(runCtx, event)
	})
}

func (s *Service) handleWorkflowEvent(ctx context.Context, event assistantmodel.WorkflowEvent) {
	if s == nil || s.runtime == nil || s.runtime.Store() == nil {
		return
	}
	store := s.runtime.Store()
	if event.Type == "market-data.tick" {
		triggers, err := store.ListEnabledWorkflowTriggersByType(ctx, assistantmodel.WorkflowTriggerTypeMarketThreshold)
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
					s.launchWorkflowInvocation(ctx, workflow, trigger, matched)
				}
			}
		}
	}
	triggers, err := store.ListEnabledWorkflowTriggersByType(ctx, assistantmodel.WorkflowTriggerTypeEvent)
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
		s.launchWorkflowInvocation(ctx, workflow, trigger, eventAsMap(event))
	}
}

func (s *Service) invokeWorkflow(ctx context.Context, workflow assistantmodel.WorkflowDefinition, trigger *assistantmodel.WorkflowTrigger, triggerType string, inputs map[string]any, matchedEvent map[string]any) (WorkflowInvocationResult, error) {
	store, err := s.workflowStore()
	if err != nil {
		return WorkflowInvocationResult{}, err
	}
	return s.invokeWorkflowWithStore(ctx, store, workflow, trigger, triggerType, inputs, matchedEvent)
}

type workflowInvocationStore interface {
	SaveWorkflowTriggerLog(context.Context, assistantmodel.WorkflowTriggerLog) (assistantmodel.WorkflowTriggerLog, error)
	ListActiveWorkflowTriggerLogs(context.Context, string) ([]assistantmodel.WorkflowTriggerLog, error)
	Run(context.Context, string) (assistantmodel.Run, bool, error)
}

func (s *Service) invokeWorkflowWithStore(ctx context.Context, store workflowInvocationStore, workflow assistantmodel.WorkflowDefinition, trigger *assistantmodel.WorkflowTrigger, triggerType string, inputs map[string]any, matchedEvent map[string]any) (WorkflowInvocationResult, error) {
	prepared, accepted, err := prepareWorkflowInvocation(ctx, store, workflow, trigger, triggerType, inputs, matchedEvent)
	if err != nil || !accepted {
		return prepared, err
	}
	return s.executeQueuedWorkflowInvocation(ctx, store, workflow, trigger, inputs, matchedEvent, prepared.Log)
}

func (s *Service) startWorkflowAsync(ctx context.Context, workflow assistantmodel.WorkflowDefinition, trigger *assistantmodel.WorkflowTrigger, triggerType string, inputs map[string]any, matchedEvent map[string]any) (WorkflowStartResult, error) {
	runCtx, release, admitted := s.reserveWorkflowBackground(ctx)
	if !admitted {
		return WorkflowStartResult{}, errAssistantServiceClosing
	}
	releaseOnReturn := true
	defer func() {
		if releaseOnReturn {
			release()
		}
	}()
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
	var triggerCopy *assistantmodel.WorkflowTrigger
	if trigger != nil {
		copyValue := *trigger
		triggerCopy = &copyValue
	}
	releaseOnReturn = false
	go func() {
		defer release()
		s.executeQueuedWorkflowBackground(runCtx, store, workflow, triggerCopy, workflowInputs, matched, prepared.Log)
	}()
	return result, nil
}

func prepareWorkflowInvocation(ctx context.Context, store workflowInvocationStore, workflow assistantmodel.WorkflowDefinition, trigger *assistantmodel.WorkflowTrigger, triggerType string, inputs map[string]any, matchedEvent map[string]any) (WorkflowInvocationResult, bool, error) {
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

func (s *Service) executeQueuedWorkflowInvocation(ctx context.Context, store workflowInvocationStore, workflow assistantmodel.WorkflowDefinition, trigger *assistantmodel.WorkflowTrigger, inputs map[string]any, matchedEvent map[string]any, log assistantmodel.WorkflowTriggerLog) (WorkflowInvocationResult, error) {
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

func (s *Service) executeQueuedWorkflowBackground(ctx context.Context, store workflowInvocationStore, workflow assistantmodel.WorkflowDefinition, trigger *assistantmodel.WorkflowTrigger, inputs map[string]any, matchedEvent map[string]any, log assistantmodel.WorkflowTriggerLog) {
	if ctx == nil {
		ctx = context.Background()
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			s.finishQueuedWorkflowBackgroundFailure(store, workflow, trigger, inputs, matchedEvent, log, fmt.Errorf("workflow background panic: %v", recovered))
		}
	}()
	result, err := s.executeQueuedWorkflowInvocation(ctx, store, workflow, trigger, inputs, matchedEvent, log)
	if err != nil && result.Log.Status != assistantmodel.WorkflowTriggerLogStatusFailed {
		s.finishQueuedWorkflowBackgroundFailure(store, workflow, trigger, inputs, matchedEvent, log, err)
	}
}

func (s *Service) finishQueuedWorkflowBackgroundFailure(store workflowInvocationStore, workflow assistantmodel.WorkflowDefinition, trigger *assistantmodel.WorkflowTrigger, inputs map[string]any, matchedEvent map[string]any, log assistantmodel.WorkflowTriggerLog, cause error) {
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cleanupCancel()
	_, _ = s.failWorkflowInvocation(cleanupCtx, store, workflow, trigger, inputs, matchedEvent, log, "", "", "", log.SessionID, cause, true)
}

func validateWorkflowInvocation(workflow assistantmodel.WorkflowDefinition, trigger *assistantmodel.WorkflowTrigger) error {
	if workflow.Status != assistantmodel.WorkflowStatusEnabled {
		return fmt.Errorf("workflow is disabled")
	}
	if trigger != nil && trigger.Status != assistantmodel.WorkflowTriggerStatusEnabled {
		return fmt.Errorf("workflow trigger is disabled")
	}
	return nil
}

func invokeWorkflowActiveTriggerGuard(ctx context.Context, store workflowInvocationStore, workflow assistantmodel.WorkflowDefinition, trigger *assistantmodel.WorkflowTrigger, inputs map[string]any, matchedEvent map[string]any) (WorkflowInvocationResult, bool, error) {
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
	log, err := store.SaveWorkflowTriggerLog(ctx, assistantmodel.WorkflowTriggerLog{
		WorkflowID:   workflow.ID,
		TriggerID:    trigger.ID,
		TriggerType:  trigger.Type,
		Status:       assistantmodel.WorkflowTriggerLogStatusSkipped,
		Inputs:       cloneMap(inputs),
		MatchedEvent: cloneMap(matchedEvent),
		Error:        "previous trigger run is still active",
		FinishedAt:   finishedAt,
		NodeRuns:     workflowNodeRuns(workflow, trigger, trigger.Type, inputs, matchedEvent, "", "", nil, assistantmodel.WorkflowTriggerLogStatusSkipped, "previous trigger run is still active", finishedAt, finishedAt),
	})
	if err != nil {
		return WorkflowInvocationResult{}, false, err
	}
	return WorkflowInvocationResult{Workflow: workflow, Trigger: newSanitizedTrigger(*trigger), Log: log}, true, nil
}

func queueWorkflowInvocationLog(ctx context.Context, store workflowInvocationStore, workflow assistantmodel.WorkflowDefinition, trigger *assistantmodel.WorkflowTrigger, triggerType string, inputs map[string]any, matchedEvent map[string]any) (assistantmodel.WorkflowTriggerLog, error) {
	log, err := store.SaveWorkflowTriggerLog(ctx, assistantmodel.WorkflowTriggerLog{
		WorkflowID:   workflow.ID,
		TriggerID:    triggerID(trigger),
		TriggerType:  defaultString(triggerType, assistantmodel.WorkflowTriggerTypeManual),
		Status:       assistantmodel.WorkflowTriggerLogStatusQueued,
		Inputs:       cloneMap(inputs),
		MatchedEvent: cloneMap(matchedEvent),
	})
	if err != nil {
		return assistantmodel.WorkflowTriggerLog{}, err
	}
	return log, nil
}

func markWorkflowInvocationRunning(ctx context.Context, store workflowInvocationStore, workflow assistantmodel.WorkflowDefinition, trigger *assistantmodel.WorkflowTrigger, inputs map[string]any, matchedEvent map[string]any, log assistantmodel.WorkflowTriggerLog) (assistantmodel.WorkflowTriggerLog, string, error) {
	started := time.Now().UTC().Format(time.RFC3339Nano)
	log.Status = assistantmodel.WorkflowTriggerLogStatusRunning
	log.StartedAt = started
	log.NodeRuns = workflowNodeRuns(workflow, trigger, log.TriggerType, inputs, matchedEvent, "", "", nil, log.Status, "", started, "")
	log, err := store.SaveWorkflowTriggerLog(ctx, log)
	if err != nil {
		return assistantmodel.WorkflowTriggerLog{}, "", err
	}
	return log, started, nil
}

func renderWorkflowInvocationMessage(workflow assistantmodel.WorkflowDefinition, trigger *assistantmodel.WorkflowTrigger, inputs map[string]any, matchedEvent map[string]any) (string, string, error) {
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
	workflow assistantmodel.WorkflowDefinition,
	trigger *assistantmodel.WorkflowTrigger,
	sessionID string,
	message string,
	objective string,
	inputs map[string]any,
	matchedEvent map[string]any,
) (assistantmodel.ChatResponse, error) {
	rendered, err := renderWorkflowCanvasTemplates(workflow, trigger, inputs, matchedEvent)
	if err != nil {
		return assistantmodel.ChatResponse{}, err
	}
	if s == nil || s.runtime == nil {
		return assistantmodel.ChatResponse{}, fmt.Errorf("adk runtime is unavailable")
	}
	response, err := s.runtime.RunCanvasWorkflow(ctx, jfadkruntime.WorkflowCanvasRunRequest{
		Workflow: rendered, SessionID: sessionID, Message: message, Objective: objective,
	})
	if err != nil {
		return assistantmodel.ChatResponse{}, err
	}
	return assistantmodel.NormalizeChatResponse(response), nil
}

func (s *Service) failWorkflowInvocation(
	ctx context.Context,
	store workflowInvocationStore,
	workflow assistantmodel.WorkflowDefinition,
	trigger *assistantmodel.WorkflowTrigger,
	inputs map[string]any,
	matchedEvent map[string]any,
	log assistantmodel.WorkflowTriggerLog,
	started string,
	message string,
	objective string,
	sessionID string,
	cause error,
	updateTrigger bool,
) (WorkflowInvocationResult, error) {
	log.SessionID = strings.TrimSpace(sessionID)
	log.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	log.NodeRuns = workflowNodeRuns(workflow, trigger, log.TriggerType, inputs, matchedEvent, message, objective, nil, assistantmodel.WorkflowTriggerLogStatusFailed, cause.Error(), started, log.FinishedAt)
	log.Result = workflowResultFromError(cause)
	log = finishWorkflowLog(ctx, store, log, assistantmodel.WorkflowTriggerLogStatusFailed, cause.Error())
	if updateTrigger {
		s.updateTriggerAfterRun(ctx, trigger, "", cause.Error())
	}
	return WorkflowInvocationResult{Workflow: workflow, Trigger: newSanitizedTriggerPtr(trigger), Log: log}, cause
}
