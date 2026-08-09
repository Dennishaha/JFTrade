package persistence

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"github.com/google/uuid"
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func (s *StoreCore) SaveWorkflowDefinition(ctx context.Context, workflow jfadkmodel.WorkflowDefinition) (jfadkmodel.WorkflowDefinition, error) {
	workflow = s.normalizeWorkflowDefinition(workflow)
	if workflow.ID == "" {
		workflow.ID = "workflow-" + uuid.NewString()
	}
	if workflow.CreatedAt == "" {
		workflow.CreatedAt = jfadkmodel.NowString()
	}
	workflow.UpdatedAt = jfadkmodel.NowString()
	if err := s.saveWorkflowDefinition(ctx, workflow); err != nil {
		return jfadkmodel.WorkflowDefinition{}, err
	}
	return workflow, nil
}

func (s *StoreCore) saveWorkflowDefinition(ctx context.Context, workflow jfadkmodel.WorkflowDefinition) error {
	payload, err := json.Marshal(workflow)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.ExecContext(ctx, `INSERT INTO `+tableWorkflows+` (id, status, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET status = excluded.status, payload_json = excluded.payload_json, updated_at = excluded.updated_at`,
		workflow.ID, workflow.Status, string(payload), workflow.CreatedAt, workflow.UpdatedAt,
	)
	return err
}

func (s *StoreCore) WorkflowDefinition(ctx context.Context, id string) (jfadkmodel.WorkflowDefinition, bool, error) {
	var workflow jfadkmodel.WorkflowDefinition
	ok, err := s.getJSON(ctx, tableWorkflows, id, &workflow)
	if err != nil || !ok {
		return jfadkmodel.WorkflowDefinition{}, ok, err
	}
	return s.normalizeWorkflowDefinition(workflow), true, nil
}

func (s *StoreCore) ListWorkflowDefinitionsPage(ctx context.Context, status string, limit int, offset int) ([]jfadkmodel.WorkflowDefinition, int, error) {
	clauses := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if status = strings.ToUpper(strings.TrimSpace(status)); status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, status)
	}
	clauses = append(clauses, "COALESCE(json_extract(payload_json, '$.deletedAt'), '') = ''")
	var workflows []jfadkmodel.WorkflowDefinition
	total, err := s.listJSONPage(ctx, tableWorkflows, clauses, args, "updated_at DESC, id ASC", limit, offset, &workflows)
	if err != nil {
		return nil, 0, err
	}
	for index := range workflows {
		workflows[index] = s.normalizeWorkflowDefinition(workflows[index])
	}
	return workflows, total, nil
}

func (s *StoreCore) DeleteWorkflowDefinition(ctx context.Context, id string) (jfadkmodel.WorkflowDefinition, error) {
	workflow, ok, err := s.WorkflowDefinition(ctx, id)
	if err != nil {
		return jfadkmodel.WorkflowDefinition{}, err
	}
	if !ok || workflow.DeletedAt != nil {
		return jfadkmodel.WorkflowDefinition{}, os.ErrNotExist
	}
	now := jfadkmodel.NowString()
	workflow.Status = jfadkmodel.WorkflowStatusDisabled
	workflow.DeletedAt = &now
	workflow.UpdatedAt = now
	if err := s.saveWorkflowDefinition(ctx, workflow); err != nil {
		return jfadkmodel.WorkflowDefinition{}, err
	}
	triggers, err := s.ListWorkflowTriggers(ctx, workflow.ID)
	if err != nil {
		return jfadkmodel.WorkflowDefinition{}, err
	}
	for _, trigger := range triggers {
		if trigger.DeletedAt != nil {
			continue
		}
		trigger.Status = jfadkmodel.WorkflowTriggerStatusDisabled
		trigger.UpdatedAt = jfadkmodel.NowString()
		if _, err := s.SaveWorkflowTrigger(ctx, trigger); err != nil {
			return jfadkmodel.WorkflowDefinition{}, err
		}
	}
	return workflow, nil
}

func (s *StoreCore) SaveWorkflowTrigger(ctx context.Context, trigger jfadkmodel.WorkflowTrigger) (jfadkmodel.WorkflowTrigger, error) {
	trigger = s.normalizeWorkflowTrigger(trigger)
	if trigger.ID == "" {
		trigger.ID = "workflow-trigger-" + uuid.NewString()
	}
	if trigger.CreatedAt == "" {
		trigger.CreatedAt = jfadkmodel.NowString()
	}
	trigger.UpdatedAt = jfadkmodel.NowString()
	if err := s.saveWorkflowTrigger(ctx, trigger); err != nil {
		return jfadkmodel.WorkflowTrigger{}, err
	}
	return trigger, nil
}

func (s *StoreCore) saveWorkflowTrigger(ctx context.Context, trigger jfadkmodel.WorkflowTrigger) error {
	payload, err := json.Marshal(trigger)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.ExecContext(ctx, `INSERT INTO `+tableWorkflowTriggers+` (id, workflow_id, trigger_type, status, next_run_at, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET workflow_id = excluded.workflow_id, trigger_type = excluded.trigger_type, status = excluded.status, next_run_at = excluded.next_run_at, payload_json = excluded.payload_json, updated_at = excluded.updated_at`,
		trigger.ID, trigger.WorkflowID, trigger.Type, trigger.Status, trigger.NextRunAt, string(payload), trigger.CreatedAt, trigger.UpdatedAt,
	)
	return err
}

func (s *StoreCore) WorkflowTrigger(ctx context.Context, id string) (jfadkmodel.WorkflowTrigger, bool, error) {
	var trigger jfadkmodel.WorkflowTrigger
	ok, err := s.getJSON(ctx, tableWorkflowTriggers, id, &trigger)
	if err != nil || !ok {
		return jfadkmodel.WorkflowTrigger{}, ok, err
	}
	return s.normalizeWorkflowTrigger(trigger), true, nil
}

func (s *StoreCore) ListWorkflowTriggers(ctx context.Context, workflowID string) ([]jfadkmodel.WorkflowTrigger, error) {
	clauses := []string{"COALESCE(json_extract(payload_json, '$.deletedAt'), '') = ''"}
	args := make([]any, 0, 1)
	if workflowID = strings.TrimSpace(workflowID); workflowID != "" {
		clauses = append(clauses, "workflow_id = ?")
		args = append(args, workflowID)
	}
	var triggers []jfadkmodel.WorkflowTrigger
	_, err := s.listJSONPage(ctx, tableWorkflowTriggers, clauses, args, "updated_at DESC, id ASC", 1000, 0, &triggers)
	if err != nil {
		return nil, err
	}
	for index := range triggers {
		triggers[index] = s.normalizeWorkflowTrigger(triggers[index])
	}
	return triggers, nil
}

func (s *StoreCore) ListEnabledWorkflowTriggersByType(ctx context.Context, triggerType string) ([]jfadkmodel.WorkflowTrigger, error) {
	clauses := []string{
		"trigger_type = ?",
		"status = ?",
		"COALESCE(json_extract(payload_json, '$.deletedAt'), '') = ''",
	}
	args := []any{strings.TrimSpace(triggerType), jfadkmodel.WorkflowTriggerStatusEnabled}
	var triggers []jfadkmodel.WorkflowTrigger
	_, err := s.listJSONPage(ctx, tableWorkflowTriggers, clauses, args, "updated_at DESC, id ASC", 1000, 0, &triggers)
	if err != nil {
		return nil, err
	}
	for index := range triggers {
		triggers[index] = s.normalizeWorkflowTrigger(triggers[index])
	}
	return triggers, nil
}

func (s *StoreCore) ListDueWorkflowScheduleTriggers(ctx context.Context, now string, limit int) ([]jfadkmodel.WorkflowTrigger, error) {
	clauses := []string{
		"trigger_type = ?",
		"status = ?",
		"next_run_at <> ''",
		"next_run_at <= ?",
		"COALESCE(json_extract(payload_json, '$.deletedAt'), '') = ''",
	}
	args := []any{jfadkmodel.WorkflowTriggerTypeSchedule, jfadkmodel.WorkflowTriggerStatusEnabled, strings.TrimSpace(now)}
	var triggers []jfadkmodel.WorkflowTrigger
	_, err := s.listJSONPage(ctx, tableWorkflowTriggers, clauses, args, "next_run_at ASC, id ASC", limit, 0, &triggers)
	if err != nil {
		return nil, err
	}
	for index := range triggers {
		triggers[index] = s.normalizeWorkflowTrigger(triggers[index])
	}
	return triggers, nil
}

func (s *StoreCore) DeleteWorkflowTrigger(ctx context.Context, id string) (jfadkmodel.WorkflowTrigger, error) {
	trigger, ok, err := s.WorkflowTrigger(ctx, id)
	if err != nil {
		return jfadkmodel.WorkflowTrigger{}, err
	}
	if !ok || trigger.DeletedAt != nil {
		return jfadkmodel.WorkflowTrigger{}, os.ErrNotExist
	}
	now := jfadkmodel.NowString()
	trigger.Status = jfadkmodel.WorkflowTriggerStatusDisabled
	trigger.DeletedAt = &now
	trigger.UpdatedAt = now
	return s.SaveWorkflowTrigger(ctx, trigger)
}

func (s *StoreCore) SaveWorkflowTriggerLog(ctx context.Context, log jfadkmodel.WorkflowTriggerLog) (jfadkmodel.WorkflowTriggerLog, error) {
	log = s.normalizeWorkflowTriggerLog(log)
	if log.ID == "" {
		log.ID = "workflow-trigger-log-" + uuid.NewString()
	}
	if log.CreatedAt == "" {
		log.CreatedAt = jfadkmodel.NowString()
	}
	log.UpdatedAt = jfadkmodel.NowString()
	payload, err := json.Marshal(log)
	if err != nil {
		return jfadkmodel.WorkflowTriggerLog{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.ExecContext(ctx, `INSERT INTO `+tableWorkflowTriggerLog+` (id, workflow_id, trigger_id, trigger_type, status, run_id, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET workflow_id = excluded.workflow_id, trigger_id = excluded.trigger_id, trigger_type = excluded.trigger_type, status = excluded.status, run_id = excluded.run_id, payload_json = excluded.payload_json, updated_at = excluded.updated_at`,
		log.ID, log.WorkflowID, log.TriggerID, log.TriggerType, log.Status, log.RunID, string(payload), log.CreatedAt, log.UpdatedAt,
	)
	if err != nil {
		return jfadkmodel.WorkflowTriggerLog{}, err
	}
	return log, nil
}

func (s *StoreCore) WorkflowTriggerLog(ctx context.Context, id string) (jfadkmodel.WorkflowTriggerLog, bool, error) {
	var log jfadkmodel.WorkflowTriggerLog
	ok, err := s.getJSON(ctx, tableWorkflowTriggerLog, id, &log)
	if err != nil || !ok {
		return jfadkmodel.WorkflowTriggerLog{}, ok, err
	}
	return s.normalizeWorkflowTriggerLog(log), true, nil
}

func (s *StoreCore) ListWorkflowTriggerLogsPage(ctx context.Context, workflowID string, triggerID string, status string, limit int, offset int) ([]jfadkmodel.WorkflowTriggerLog, int, error) {
	clauses := make([]string, 0, 3)
	args := make([]any, 0, 3)
	if workflowID = strings.TrimSpace(workflowID); workflowID != "" {
		clauses = append(clauses, "workflow_id = ?")
		args = append(args, workflowID)
	}
	if triggerID = strings.TrimSpace(triggerID); triggerID != "" {
		clauses = append(clauses, "trigger_id = ?")
		args = append(args, triggerID)
	}
	if status = strings.ToUpper(strings.TrimSpace(status)); status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, status)
	}
	var logs []jfadkmodel.WorkflowTriggerLog
	total, err := s.listJSONPage(ctx, tableWorkflowTriggerLog, clauses, args, "created_at DESC, id ASC", limit, offset, &logs)
	if err != nil {
		return nil, 0, err
	}
	for index := range logs {
		logs[index] = s.normalizeWorkflowTriggerLog(logs[index])
	}
	return logs, total, nil
}

func (s *StoreCore) ListActiveWorkflowTriggerLogs(ctx context.Context, triggerID string) ([]jfadkmodel.WorkflowTriggerLog, error) {
	clauses := []string{
		"trigger_id = ?",
		"status IN (?, ?, ?)",
	}
	args := []any{strings.TrimSpace(triggerID), jfadkmodel.WorkflowTriggerLogStatusQueued, jfadkmodel.WorkflowTriggerLogStatusRunning, jfadkmodel.WorkflowTriggerLogStatusPendingApproval}
	var logs []jfadkmodel.WorkflowTriggerLog
	_, err := s.listJSONPage(ctx, tableWorkflowTriggerLog, clauses, args, "created_at DESC, id ASC", 100, 0, &logs)
	if err != nil {
		return nil, err
	}
	for index := range logs {
		logs[index] = s.normalizeWorkflowTriggerLog(logs[index])
	}
	return logs, nil
}
