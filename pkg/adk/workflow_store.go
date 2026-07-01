package adk

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func (s *Store) ensureWorkflowSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS ` + tableWorkflows + ` (id TEXT PRIMARY KEY, status TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS ` + tableWorkflowTriggers + ` (id TEXT PRIMARY KEY, workflow_id TEXT NOT NULL, trigger_type TEXT NOT NULL, status TEXT NOT NULL, next_run_at TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS ` + tableWorkflowTriggerLog + ` (id TEXT PRIMARY KEY, workflow_id TEXT NOT NULL, trigger_id TEXT NOT NULL, trigger_type TEXT NOT NULL, status TEXT NOT NULL, run_id TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_adk_workflows_status ON ` + tableWorkflows + ` (status, updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_adk_workflow_triggers_workflow ON ` + tableWorkflowTriggers + ` (workflow_id, updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_adk_workflow_triggers_due ON ` + tableWorkflowTriggers + ` (trigger_type, status, next_run_at ASC)`,
		`CREATE INDEX IF NOT EXISTS idx_adk_workflow_trigger_logs_workflow ON ` + tableWorkflowTriggerLog + ` (workflow_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_adk_workflow_trigger_logs_trigger ON ` + tableWorkflowTriggerLog + ` (trigger_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_adk_workflow_trigger_logs_status ON ` + tableWorkflowTriggerLog + ` (status, updated_at DESC)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SaveWorkflowDefinition(ctx context.Context, workflow WorkflowDefinition) (WorkflowDefinition, error) {
	workflow = NormalizeWorkflowDefinition(workflow)
	if workflow.ID == "" {
		workflow.ID = "workflow-" + uuid.NewString()
	}
	if workflow.CreatedAt == "" {
		workflow.CreatedAt = nowString()
	}
	workflow.UpdatedAt = nowString()
	if err := s.saveWorkflowDefinition(ctx, workflow); err != nil {
		return WorkflowDefinition{}, err
	}
	return workflow, nil
}

func (s *Store) saveWorkflowDefinition(ctx context.Context, workflow WorkflowDefinition) error {
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

func (s *Store) WorkflowDefinition(ctx context.Context, id string) (WorkflowDefinition, bool, error) {
	var workflow WorkflowDefinition
	ok, err := s.getJSON(ctx, tableWorkflows, id, &workflow)
	if err != nil || !ok {
		return WorkflowDefinition{}, ok, err
	}
	return NormalizeWorkflowDefinition(workflow), true, nil
}

func (s *Store) ListWorkflowDefinitionsPage(ctx context.Context, status string, limit int, offset int) ([]WorkflowDefinition, int, error) {
	clauses := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if status = strings.ToUpper(strings.TrimSpace(status)); status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, status)
	}
	clauses = append(clauses, "COALESCE(json_extract(payload_json, '$.deletedAt'), '') = ''")
	var workflows []WorkflowDefinition
	total, err := s.listJSONPage(ctx, tableWorkflows, clauses, args, "updated_at DESC, id ASC", limit, offset, &workflows)
	if err != nil {
		return nil, 0, err
	}
	for index := range workflows {
		workflows[index] = NormalizeWorkflowDefinition(workflows[index])
	}
	return workflows, total, nil
}

func (s *Store) DeleteWorkflowDefinition(ctx context.Context, id string) (WorkflowDefinition, error) {
	workflow, ok, err := s.WorkflowDefinition(ctx, id)
	if err != nil {
		return WorkflowDefinition{}, err
	}
	if !ok || workflow.DeletedAt != nil {
		return WorkflowDefinition{}, os.ErrNotExist
	}
	now := nowString()
	workflow.Status = WorkflowStatusDisabled
	workflow.DeletedAt = &now
	workflow.UpdatedAt = now
	if err := s.saveWorkflowDefinition(ctx, workflow); err != nil {
		return WorkflowDefinition{}, err
	}
	triggers, err := s.ListWorkflowTriggers(ctx, workflow.ID)
	if err != nil {
		return WorkflowDefinition{}, err
	}
	for _, trigger := range triggers {
		if trigger.DeletedAt != nil {
			continue
		}
		trigger.Status = WorkflowTriggerStatusDisabled
		trigger.UpdatedAt = nowString()
		if _, err := s.SaveWorkflowTrigger(ctx, trigger); err != nil {
			return WorkflowDefinition{}, err
		}
	}
	return workflow, nil
}

func (s *Store) SaveWorkflowTrigger(ctx context.Context, trigger WorkflowTrigger) (WorkflowTrigger, error) {
	trigger = NormalizeWorkflowTrigger(trigger)
	if trigger.ID == "" {
		trigger.ID = "workflow-trigger-" + uuid.NewString()
	}
	if trigger.CreatedAt == "" {
		trigger.CreatedAt = nowString()
	}
	trigger.UpdatedAt = nowString()
	if err := s.saveWorkflowTrigger(ctx, trigger); err != nil {
		return WorkflowTrigger{}, err
	}
	return trigger, nil
}

func (s *Store) saveWorkflowTrigger(ctx context.Context, trigger WorkflowTrigger) error {
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

func (s *Store) WorkflowTrigger(ctx context.Context, id string) (WorkflowTrigger, bool, error) {
	var trigger WorkflowTrigger
	ok, err := s.getJSON(ctx, tableWorkflowTriggers, id, &trigger)
	if err != nil || !ok {
		return WorkflowTrigger{}, ok, err
	}
	return NormalizeWorkflowTrigger(trigger), true, nil
}

func (s *Store) ListWorkflowTriggers(ctx context.Context, workflowID string) ([]WorkflowTrigger, error) {
	clauses := []string{"COALESCE(json_extract(payload_json, '$.deletedAt'), '') = ''"}
	args := make([]any, 0, 1)
	if workflowID = strings.TrimSpace(workflowID); workflowID != "" {
		clauses = append(clauses, "workflow_id = ?")
		args = append(args, workflowID)
	}
	var triggers []WorkflowTrigger
	_, err := s.listJSONPage(ctx, tableWorkflowTriggers, clauses, args, "updated_at DESC, id ASC", 1000, 0, &triggers)
	if err != nil {
		return nil, err
	}
	for index := range triggers {
		triggers[index] = NormalizeWorkflowTrigger(triggers[index])
	}
	return triggers, nil
}

func (s *Store) ListEnabledWorkflowTriggersByType(ctx context.Context, triggerType string) ([]WorkflowTrigger, error) {
	clauses := []string{
		"trigger_type = ?",
		"status = ?",
		"COALESCE(json_extract(payload_json, '$.deletedAt'), '') = ''",
	}
	args := []any{strings.TrimSpace(triggerType), WorkflowTriggerStatusEnabled}
	var triggers []WorkflowTrigger
	_, err := s.listJSONPage(ctx, tableWorkflowTriggers, clauses, args, "updated_at DESC, id ASC", 1000, 0, &triggers)
	if err != nil {
		return nil, err
	}
	for index := range triggers {
		triggers[index] = NormalizeWorkflowTrigger(triggers[index])
	}
	return triggers, nil
}

func (s *Store) ListDueWorkflowScheduleTriggers(ctx context.Context, now string, limit int) ([]WorkflowTrigger, error) {
	clauses := []string{
		"trigger_type = ?",
		"status = ?",
		"next_run_at <> ''",
		"next_run_at <= ?",
		"COALESCE(json_extract(payload_json, '$.deletedAt'), '') = ''",
	}
	args := []any{WorkflowTriggerTypeSchedule, WorkflowTriggerStatusEnabled, strings.TrimSpace(now)}
	var triggers []WorkflowTrigger
	_, err := s.listJSONPage(ctx, tableWorkflowTriggers, clauses, args, "next_run_at ASC, id ASC", limit, 0, &triggers)
	if err != nil {
		return nil, err
	}
	for index := range triggers {
		triggers[index] = NormalizeWorkflowTrigger(triggers[index])
	}
	return triggers, nil
}

func (s *Store) DeleteWorkflowTrigger(ctx context.Context, id string) (WorkflowTrigger, error) {
	trigger, ok, err := s.WorkflowTrigger(ctx, id)
	if err != nil {
		return WorkflowTrigger{}, err
	}
	if !ok || trigger.DeletedAt != nil {
		return WorkflowTrigger{}, os.ErrNotExist
	}
	now := nowString()
	trigger.Status = WorkflowTriggerStatusDisabled
	trigger.DeletedAt = &now
	trigger.UpdatedAt = now
	return s.SaveWorkflowTrigger(ctx, trigger)
}

func (s *Store) SaveWorkflowTriggerLog(ctx context.Context, log WorkflowTriggerLog) (WorkflowTriggerLog, error) {
	log = NormalizeWorkflowTriggerLog(log)
	if log.ID == "" {
		log.ID = "workflow-trigger-log-" + uuid.NewString()
	}
	if log.CreatedAt == "" {
		log.CreatedAt = nowString()
	}
	log.UpdatedAt = nowString()
	payload, err := json.Marshal(log)
	if err != nil {
		return WorkflowTriggerLog{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.ExecContext(ctx, `INSERT INTO `+tableWorkflowTriggerLog+` (id, workflow_id, trigger_id, trigger_type, status, run_id, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET workflow_id = excluded.workflow_id, trigger_id = excluded.trigger_id, trigger_type = excluded.trigger_type, status = excluded.status, run_id = excluded.run_id, payload_json = excluded.payload_json, updated_at = excluded.updated_at`,
		log.ID, log.WorkflowID, log.TriggerID, log.TriggerType, log.Status, log.RunID, string(payload), log.CreatedAt, log.UpdatedAt,
	)
	if err != nil {
		return WorkflowTriggerLog{}, err
	}
	return log, nil
}

func (s *Store) WorkflowTriggerLog(ctx context.Context, id string) (WorkflowTriggerLog, bool, error) {
	var log WorkflowTriggerLog
	ok, err := s.getJSON(ctx, tableWorkflowTriggerLog, id, &log)
	if err != nil || !ok {
		return WorkflowTriggerLog{}, ok, err
	}
	return NormalizeWorkflowTriggerLog(log), true, nil
}

func (s *Store) ListWorkflowTriggerLogsPage(ctx context.Context, workflowID string, triggerID string, status string, limit int, offset int) ([]WorkflowTriggerLog, int, error) {
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
	var logs []WorkflowTriggerLog
	total, err := s.listJSONPage(ctx, tableWorkflowTriggerLog, clauses, args, "created_at DESC, id ASC", limit, offset, &logs)
	if err != nil {
		return nil, 0, err
	}
	for index := range logs {
		logs[index] = NormalizeWorkflowTriggerLog(logs[index])
	}
	return logs, total, nil
}

func (s *Store) ListActiveWorkflowTriggerLogs(ctx context.Context, triggerID string) ([]WorkflowTriggerLog, error) {
	clauses := []string{
		"trigger_id = ?",
		"status IN (?, ?, ?)",
	}
	args := []any{strings.TrimSpace(triggerID), WorkflowTriggerLogStatusQueued, WorkflowTriggerLogStatusRunning, WorkflowTriggerLogStatusPendingApproval}
	var logs []WorkflowTriggerLog
	_, err := s.listJSONPage(ctx, tableWorkflowTriggerLog, clauses, args, "created_at DESC, id ASC", 100, 0, &logs)
	if err != nil {
		return nil, err
	}
	for index := range logs {
		logs[index] = NormalizeWorkflowTriggerLog(logs[index])
	}
	return logs, nil
}

func saveWorkflowTriggerWithExecutor(ctx context.Context, executor sqlx.ExtContext, trigger WorkflowTrigger) error {
	payload, err := json.Marshal(NormalizeWorkflowTrigger(trigger))
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `INSERT INTO `+tableWorkflowTriggers+` (id, workflow_id, trigger_type, status, next_run_at, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET workflow_id = excluded.workflow_id, trigger_type = excluded.trigger_type, status = excluded.status, next_run_at = excluded.next_run_at, payload_json = excluded.payload_json, updated_at = excluded.updated_at`,
		trigger.ID, trigger.WorkflowID, trigger.Type, trigger.Status, trigger.NextRunAt, string(payload), trigger.CreatedAt, trigger.UpdatedAt,
	)
	return err
}
