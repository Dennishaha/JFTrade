package adk

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
	"github.com/jmoiron/sqlx"
)

func (s *Store) SaveRun(ctx context.Context, run Run) error {
	run, err := s.prepareRunForSave(ctx, run)
	if err != nil {
		return err
	}
	return s.savePreparedRun(ctx, run)
}

func (s *Store) prepareRunForSave(ctx context.Context, run Run) (Run, error) {
	if run.CreatedAt == "" {
		run.CreatedAt = nowString()
	}
	if isRootLoopGoalRun(run) {
		latest, ok, err := s.Run(ctx, run.ID)
		if err != nil {
			return Run{}, err
		}
		if ok {
			run = preserveUserGoalPauseLifecycle(latest, run)
		}
	}
	run = NormalizeRun(run)
	run.UpdatedAt = nowString()
	return run, nil
}

func (s *Store) savePreparedRun(ctx context.Context, run Run) error {
	return savePreparedRunWithExecutor(ctx, s.db, run)
}

func savePreparedRunWithExecutor(ctx context.Context, executor sqlx.ExtContext, run Run) error {
	payload, err := json.Marshal(run)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `INSERT INTO `+tableRuns+` (id, session_id, agent_id, status, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET session_id = excluded.session_id, agent_id = excluded.agent_id, status = excluded.status, payload_json = excluded.payload_json, updated_at = excluded.updated_at WHERE `+tableRuns+`.status NOT IN (?, ?, ?, ?, ?) OR (`+tableRuns+`.status = excluded.status AND `+tableRuns+`.status <> ?) OR (`+tableRuns+`.status = ? AND COALESCE(json_extract(`+tableRuns+`.payload_json, '$.finalMessageId'), '') = '' AND COALESCE(json_extract(excluded.payload_json, '$.finalMessageId'), '') <> '') OR (`+tableRuns+`.status = ? AND json_extract(`+tableRuns+`.payload_json, '$.workflowStatus') = ? AND excluded.status IN (?, ?, ?, ?, ?)) OR (`+tableRuns+`.status = ? AND excluded.status = ? AND json_array_length(json_extract(excluded.payload_json, '$.pendingApprovals')) > 0)`,
		run.ID, run.SessionID, run.AgentID, run.Status, string(payload), run.CreatedAt, run.UpdatedAt,
		RunStatusCompleted, RunStatusFailed, RunStatusDenied, RunStatusCancelled, RunStatusTimedOut,
		RunStatusCancelled, RunStatusCancelled, RunStatusCompleted, workflowStatusRunning,
		RunStatusCompleted, RunStatusFailed, RunStatusDenied, RunStatusCancelled, RunStatusTimedOut,
		RunStatusCompleted, RunStatusPending,
	)
	return err
}

func (s *Store) SaveRunAndDenyPendingApprovals(ctx context.Context, run Run) error {
	run, err := s.prepareRunForSave(ctx, run)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginWrite(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if tx != nil {
			jftradeErr := tx.Rollback()
			besteffort.LogError(jftradeErr)
		}
	}()
	rows := []struct {
		PayloadJSON string `db:"payload_json"`
	}{}
	if err := tx.SelectContext(ctx, &rows, `SELECT payload_json FROM `+tableApprovals+` WHERE run_id = ? AND status = ?`, run.ID, ApprovalStatusPending); err != nil {
		return err
	}
	for _, row := range rows {
		var approval Approval
		if err := json.Unmarshal([]byte(row.PayloadJSON), &approval); err != nil {
			return err
		}
		approval.Status = ApprovalStatusDenied
		approval.UpdatedAt = nowString()
		payload, err := json.Marshal(approval)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE `+tableApprovals+` SET status = ?, payload_json = ?, updated_at = ? WHERE id = ? AND status = ?`,
			approval.Status, string(payload), approval.UpdatedAt, approval.ID, ApprovalStatusPending,
		); err != nil {
			return err
		}
	}
	if err := savePreparedRunWithExecutor(ctx, tx, run); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}

func (s *Store) Run(ctx context.Context, id string) (Run, bool, error) {
	var run Run
	ok, err := s.getJSON(ctx, tableRuns, id, &run)
	if err != nil || !ok {
		return Run{}, ok, err
	}
	return NormalizeRun(run), true, nil
}

func (s *Store) ListRuns(ctx context.Context) ([]Run, error) {
	var runs []Run
	if err := s.listJSON(ctx, tableRuns, "created_at DESC, id ASC", &runs); err != nil {
		return nil, err
	}
	for index := range runs {
		runs[index] = NormalizeRun(runs[index])
	}
	return runs, nil
}

func (s *Store) ListRunsPage(ctx context.Context, status string, agentID string, sessionID string, limit int, offset int) ([]Run, int, error) {
	clauses := make([]string, 0, 3)
	args := make([]any, 0, 3)
	if status = strings.ToUpper(strings.TrimSpace(status)); status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, status)
	}
	if agentID = strings.TrimSpace(agentID); agentID != "" {
		clauses = append(clauses, "agent_id = ?")
		args = append(args, agentID)
	}
	if sessionID = strings.TrimSpace(sessionID); sessionID != "" {
		clauses = append(clauses, "session_id = ?")
		args = append(args, sessionID)
	}
	var runs []Run
	total, err := s.listJSONPage(ctx, tableRuns, clauses, args, "created_at DESC, id ASC", limit, offset, &runs)
	for index := range runs {
		runs[index] = NormalizeRun(runs[index])
	}
	return runs, total, err
}

func (s *Store) SaveApproval(ctx context.Context, approval Approval) error {
	if approval.CreatedAt == "" {
		approval.CreatedAt = nowString()
	}
	approval.UpdatedAt = nowString()
	payload, err := json.Marshal(approval)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO `+tableApprovals+` (id, run_id, agent_id, status, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET run_id = excluded.run_id, agent_id = excluded.agent_id, status = excluded.status, payload_json = excluded.payload_json, updated_at = excluded.updated_at`, approval.ID, approval.RunID, approval.AgentID, approval.Status, string(payload), approval.CreatedAt, approval.UpdatedAt)
	return err
}

func (s *Store) SaveApprovalIfConfirmationAbsent(ctx context.Context, approval Approval) (Approval, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	confirmationID := strings.TrimSpace(approval.ConfirmationCallID)
	if confirmationID != "" {
		existing, ok, err := s.approvalByConfirmationCallID(ctx, confirmationID)
		if err != nil {
			return Approval{}, false, err
		}
		if ok {
			return existing, false, nil
		}
	}
	if approval.CreatedAt == "" {
		approval.CreatedAt = nowString()
	}
	approval.UpdatedAt = nowString()
	payload, err := json.Marshal(approval)
	if err != nil {
		return Approval{}, false, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO `+tableApprovals+` (id, run_id, agent_id, status, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, approval.ID, approval.RunID, approval.AgentID, approval.Status, string(payload), approval.CreatedAt, approval.UpdatedAt)
	if err != nil {
		if confirmationID != "" {
			existing, ok, lookupErr := s.approvalByConfirmationCallID(ctx, confirmationID)
			if lookupErr == nil && ok {
				return existing, false, nil
			}
		}
		return Approval{}, false, err
	}
	return approval, true, nil
}

func (s *Store) ApprovalByConfirmationCallID(ctx context.Context, confirmationID string) (Approval, bool, error) {
	return s.approvalByConfirmationCallID(ctx, strings.TrimSpace(confirmationID))
}

func (s *Store) approvalByConfirmationCallID(ctx context.Context, confirmationID string) (Approval, bool, error) {
	var approval Approval
	if confirmationID == "" {
		return approval, false, nil
	}
	var payload string
	err := s.db.QueryRowxContext(ctx, `SELECT payload_json FROM `+tableApprovals+` WHERE json_extract(payload_json, '$.confirmationCallId') = ? ORDER BY created_at ASC, id ASC LIMIT 1`, confirmationID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return approval, false, nil
	}
	if err != nil {
		return approval, false, err
	}
	if err := json.Unmarshal([]byte(payload), &approval); err != nil {
		return Approval{}, false, err
	}
	return approval, true, nil
}

func (s *Store) ResolvePendingApproval(ctx context.Context, id string, status string) (Approval, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var approval Approval
	ok, err := s.getJSON(ctx, tableApprovals, id, &approval)
	if err != nil || !ok {
		return Approval{}, ok, err
	}
	if approval.Status != ApprovalStatusPending {
		return approval, false, nil
	}
	approval.Status = status
	approval.UpdatedAt = nowString()
	payload, err := json.Marshal(approval)
	if err != nil {
		return Approval{}, false, err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE `+tableApprovals+` SET status = ?, payload_json = ?, updated_at = ? WHERE id = ? AND status = ?`, approval.Status, string(payload), approval.UpdatedAt, approval.ID, ApprovalStatusPending)
	if err != nil {
		return Approval{}, false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Approval{}, false, err
	}
	if affected == 0 {
		current, currentOK, currentErr := s.Approval(ctx, id)
		return current, false, currentErrOrNotFound(currentErr, currentOK)
	}
	return approval, true, nil
}

func (s *Store) Approval(ctx context.Context, id string) (Approval, bool, error) {
	var approval Approval
	ok, err := s.getJSON(ctx, tableApprovals, id, &approval)
	return approval, ok, err
}

func (s *Store) ListApprovals(ctx context.Context) ([]Approval, error) {
	var approvals []Approval
	return approvals, s.listJSON(ctx, tableApprovals, "updated_at DESC, id ASC", &approvals)
}

func (s *Store) ListApprovalsPage(ctx context.Context, status string, agentID string, limit int, offset int) ([]Approval, int, error) {
	clauses := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if status = strings.ToUpper(strings.TrimSpace(status)); status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, status)
	}
	if agentID = strings.TrimSpace(agentID); agentID != "" {
		clauses = append(clauses, "agent_id = ?")
		args = append(args, agentID)
	}
	var approvals []Approval
	total, err := s.listJSONPage(ctx, tableApprovals, clauses, args, "updated_at DESC, id ASC", limit, offset, &approvals)
	return approvals, total, err
}

func (s *Store) ListSkills(ctx context.Context) ([]Skill, error) {
	var skills []Skill
	if err := s.listJSON(ctx, tableSkills, "id ASC", &skills); err != nil {
		return nil, err
	}
	sort.Slice(skills, func(i int, j int) bool {
		if skills[i].Builtin != skills[j].Builtin {
			return skills[i].Builtin
		}
		return skills[i].DisplayName < skills[j].DisplayName
	})
	return skills, nil
}

func (s *Store) SaveSkill(ctx context.Context, skill Skill) (Skill, error) {
	now := nowString()
	if skill.ID == "" {
		skill.ID = normalizeID(skill.DisplayName)
	}
	if skill.ID == "" {
		skill.ID = "skill-" + uuid.NewString()
	}
	existing, ok, err := s.Skill(ctx, skill.ID)
	if err != nil {
		return Skill{}, err
	}
	if ok && skill.CreatedAt == "" {
		skill.CreatedAt = existing.CreatedAt
	}
	if skill.CreatedAt == "" {
		skill.CreatedAt = now
	}
	skill.UpdatedAt = now
	return skill, s.saveJSON(ctx, tableSkills, skill.ID, skill.CreatedAt, skill.UpdatedAt, skill)
}

func (s *Store) Skill(ctx context.Context, id string) (Skill, bool, error) {
	var skill Skill
	ok, err := s.getJSON(ctx, tableSkills, id, &skill)
	return skill, ok, err
}

func (s *Store) DeleteSkill(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM `+tableSkills+` WHERE id = ? AND json_extract(payload_json, '$.builtin') = 0`, strings.TrimSpace(id)); err != nil {
		return err
	}
	return nil
}

func (s *Store) AddAuditEvent(ctx context.Context, event AuditEvent) error {
	if strings.TrimSpace(event.ID) == "" {
		event.ID = "audit-" + uuid.NewString()
	}
	if strings.TrimSpace(event.CreatedAt) == "" {
		event.CreatedAt = nowString()
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO `+tableAudit+` (id, kind, subject_id, payload_json, created_at) VALUES (?, ?, ?, ?, ?)`, event.ID, event.Kind, event.SubjectID, string(payload), event.CreatedAt)
	return err
}

func (s *Store) ListAuditEvents(ctx context.Context) ([]AuditEvent, error) {
	rows := []struct {
		PayloadJSON string `db:"payload_json"`
	}{}
	if err := s.db.SelectContext(ctx, &rows, `SELECT payload_json FROM `+tableAudit+` ORDER BY created_at DESC, id ASC`); err != nil {
		return nil, err
	}
	events := make([]AuditEvent, 0, len(rows))
	for _, row := range rows {
		var event AuditEvent
		if err := json.Unmarshal([]byte(row.PayloadJSON), &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func (s *Store) SaveOptimizationTask(ctx context.Context, task OptimizationTask) (OptimizationTask, error) {
	now := nowString()
	if strings.TrimSpace(task.ID) == "" {
		task.ID = "opt-" + uuid.NewString()
	}
	existing, ok, err := s.OptimizationTask(ctx, task.ID)
	if err != nil {
		return OptimizationTask{}, err
	}
	if ok && task.CreatedAt == "" {
		task.CreatedAt = existing.CreatedAt
	}
	if task.CreatedAt == "" {
		task.CreatedAt = now
	}
	task.UpdatedAt = now
	return task, s.saveJSON(ctx, tableOptimizations, task.ID, task.CreatedAt, task.UpdatedAt, task)
}

func (s *Store) OptimizationTask(ctx context.Context, id string) (OptimizationTask, bool, error) {
	var task OptimizationTask
	ok, err := s.getJSON(ctx, tableOptimizations, id, &task)
	return task, ok, err
}

func (s *Store) ListOptimizationTasks(ctx context.Context) ([]OptimizationTask, error) {
	var tasks []OptimizationTask
	return tasks, s.listJSON(ctx, tableOptimizations, "updated_at DESC, id ASC", &tasks)
}

func (s *Store) SaveTask(ctx context.Context, req TaskWriteRequest) (Task, error) {
	id := normalizeID(req.ID)
	if id == "" {
		id = "task-" + uuid.NewString()
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return Task{}, fmt.Errorf("task title is required")
	}
	status, err := normalizeTaskStatus(req.Status)
	if err != nil {
		return Task{}, err
	}
	dependsOn, err := normalizeTaskDependsOn(id, req.DependsOn)
	if err != nil {
		return Task{}, err
	}
	now := nowString()
	existing, ok, err := s.Task(ctx, id)
	if err != nil {
		return Task{}, err
	}
	createdAt := now
	if ok {
		createdAt = existing.CreatedAt
	}
	task := Task{
		ID: id, Title: title, Description: strings.TrimSpace(req.Description), Status: status,
		AgentID: strings.TrimSpace(req.AgentID), RunID: strings.TrimSpace(req.RunID),
		DependsOn: dependsOn, Order: req.Order,
		ModeHint: strings.TrimSpace(req.ModeHint), AgentRole: strings.TrimSpace(req.AgentRole),
		PlannerStepID: strings.TrimSpace(req.PlannerStepID), PlanSource: strings.TrimSpace(req.PlanSource),
		WorkflowMode: strings.TrimSpace(req.WorkflowMode), Objective: strings.TrimSpace(req.Objective),
		Message: strings.TrimSpace(req.Message), Executor: strings.TrimSpace(req.Executor),
		ChildAgentID:        strings.TrimSpace(req.ChildAgentID),
		ChildProviderID:     strings.TrimSpace(req.ChildProviderID),
		ChildModel:          strings.TrimSpace(req.ChildModel),
		ChildPermissionMode: strings.TrimSpace(req.ChildPermissionMode),
		ResultSummary:       strings.TrimSpace(req.ResultSummary),
		PlannerWarnings:     normalizeStringSlice(req.PlannerWarnings),
		CreatedAt:           createdAt, UpdatedAt: now,
	}
	return s.saveTask(ctx, task)
}

func (s *Store) UpdateTask(ctx context.Context, id string, req TaskPatchRequest) (Task, error) {
	id = normalizeID(id)
	if id == "" {
		return Task{}, os.ErrNotExist
	}
	task, ok, err := s.Task(ctx, id)
	if err != nil {
		return Task{}, err
	}
	if !ok {
		return Task{}, os.ErrNotExist
	}
	if err := applyTaskPatch(&task, id, req); err != nil {
		return Task{}, err
	}
	task.UpdatedAt = nowString()
	return s.saveTask(ctx, task)
}

func applyTaskPatch(task *Task, id string, req TaskPatchRequest) error {
	if task == nil {
		return nil
	}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			return fmt.Errorf("task title is required")
		}
		task.Title = title
	}
	if req.Description != nil {
		task.Description = strings.TrimSpace(*req.Description)
	}
	if req.Status != nil {
		status, err := normalizeTaskStatus(*req.Status)
		if err != nil {
			return err
		}
		task.Status = status
	}
	if req.AgentID != nil {
		task.AgentID = strings.TrimSpace(*req.AgentID)
	}
	if req.RunID != nil {
		task.RunID = strings.TrimSpace(*req.RunID)
	}
	if req.DependsOn != nil {
		dependsOn, err := normalizeTaskDependsOn(id, req.DependsOn)
		if err != nil {
			return err
		}
		task.DependsOn = dependsOn
	}
	applyTaskMetadataPatch(task, req)
	return nil
}

func applyTaskMetadataPatch(task *Task, req TaskPatchRequest) {
	if req.Order != nil {
		task.Order = *req.Order
	}
	if req.ModeHint != nil {
		task.ModeHint = strings.TrimSpace(*req.ModeHint)
	}
	if req.AgentRole != nil {
		task.AgentRole = strings.TrimSpace(*req.AgentRole)
	}
	if req.PlannerStepID != nil {
		task.PlannerStepID = strings.TrimSpace(*req.PlannerStepID)
	}
	if req.PlanSource != nil {
		task.PlanSource = strings.TrimSpace(*req.PlanSource)
	}
	if req.WorkflowMode != nil {
		task.WorkflowMode = strings.TrimSpace(*req.WorkflowMode)
	}
	if req.Objective != nil {
		task.Objective = strings.TrimSpace(*req.Objective)
	}
	if req.Message != nil {
		task.Message = strings.TrimSpace(*req.Message)
	}
	if req.Executor != nil {
		task.Executor = strings.TrimSpace(*req.Executor)
	}
	if req.ChildAgentID != nil {
		task.ChildAgentID = strings.TrimSpace(*req.ChildAgentID)
	}
	if req.ChildProviderID != nil {
		task.ChildProviderID = strings.TrimSpace(*req.ChildProviderID)
	}
	if req.ChildModel != nil {
		task.ChildModel = strings.TrimSpace(*req.ChildModel)
	}
	if req.ChildPermissionMode != nil {
		task.ChildPermissionMode = strings.TrimSpace(*req.ChildPermissionMode)
	}
	if req.ResultSummary != nil {
		task.ResultSummary = strings.TrimSpace(*req.ResultSummary)
	}
	if req.PlannerWarnings != nil {
		task.PlannerWarnings = normalizeStringSlice(req.PlannerWarnings)
	}
}

func (s *Store) saveTask(ctx context.Context, task Task) (Task, error) {
	payload, err := json.Marshal(task)
	if err != nil {
		return Task{}, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO `+tableTasks+` (id, status, agent_id, run_id, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET status = excluded.status, agent_id = excluded.agent_id, run_id = excluded.run_id, payload_json = excluded.payload_json, updated_at = excluded.updated_at`, task.ID, task.Status, task.AgentID, task.RunID, string(payload), task.CreatedAt, task.UpdatedAt)
	return task, err
}

func (s *Store) Task(ctx context.Context, id string) (Task, bool, error) {
	var task Task
	ok, err := s.getJSON(ctx, tableTasks, id, &task)
	return task, ok, err
}

func (s *Store) ListTasksPage(ctx context.Context, status string, agentID string, runID string, limit int, offset int) ([]Task, int, error) {
	clauses := make([]string, 0, 3)
	args := make([]any, 0, 3)
	if status = strings.ToUpper(strings.TrimSpace(status)); status != "" {
		if _, err := normalizeTaskStatus(status); err != nil {
			return nil, 0, err
		}
		clauses = append(clauses, "status = ?")
		args = append(args, status)
	}
	if agentID = strings.TrimSpace(agentID); agentID != "" {
		clauses = append(clauses, "agent_id = ?")
		args = append(args, agentID)
	}
	if runID = strings.TrimSpace(runID); runID != "" {
		clauses = append(clauses, "run_id = ?")
		args = append(args, runID)
	}
	var tasks []Task
	total, err := s.listJSONPage(ctx, tableTasks, clauses, args, "updated_at DESC, id ASC", limit, offset, &tasks)
	return tasks, total, err
}

func (s *Store) DeleteTask(ctx context.Context, id string) error {
	id = normalizeID(id)
	if id == "" {
		return os.ErrNotExist
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM `+tableTasks+` WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if rows, rowErr := result.RowsAffected(); rowErr == nil && rows == 0 {
		return os.ErrNotExist
	}
	return nil
}

func (s *Store) SaveMemory(ctx context.Context, req MemoryWriteRequest) (MemoryEntry, error) {
	key := normalizeMemoryKey(req.Key)
	if key == "" {
		return MemoryEntry{}, fmt.Errorf("memory key is required")
	}
	value := strings.TrimSpace(req.Value)
	if len([]rune(value)) > 2000 {
		value = string([]rune(value)[:2000])
	}
	scope := strings.ToLower(strings.TrimSpace(req.Scope))
	if scope == "" {
		scope = "workspace"
	}
	if scope != "workspace" && scope != "agent" {
		return MemoryEntry{}, fmt.Errorf("memory scope must be workspace or agent")
	}
	agentID := strings.TrimSpace(req.AgentID)
	if scope == "workspace" {
		agentID = ""
	} else if agentID == "" {
		return MemoryEntry{}, fmt.Errorf("agent memory requires agentId")
	} else if _, ok, err := s.Agent(ctx, agentID); err != nil {
		return MemoryEntry{}, err
	} else if !ok {
		return MemoryEntry{}, fmt.Errorf("agent not found")
	}
	id := normalizeID(scope + "-" + agentID + "-" + key)
	now := nowString()
	existing, ok, err := s.Memory(ctx, id)
	if err != nil {
		return MemoryEntry{}, err
	}
	createdAt := now
	if ok {
		createdAt = existing.CreatedAt
	}
	entry := MemoryEntry{ID: id, AgentID: agentID, Key: key, Value: value, Scope: scope, CreatedAt: createdAt, UpdatedAt: now}
	payload, err := json.Marshal(entry)
	if err != nil {
		return MemoryEntry{}, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO `+tableMemory+` (id, agent_id, scope, memory_key, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(agent_id, scope, memory_key) DO UPDATE SET payload_json = excluded.payload_json, updated_at = excluded.updated_at`, entry.ID, entry.AgentID, entry.Scope, entry.Key, string(payload), entry.CreatedAt, entry.UpdatedAt)
	return entry, err
}

func (s *Store) Memory(ctx context.Context, id string) (MemoryEntry, bool, error) {
	var entry MemoryEntry
	ok, err := s.getJSON(ctx, tableMemory, id, &entry)
	return entry, ok, err
}

func (s *Store) ListMemory(ctx context.Context, agentID string) ([]MemoryEntry, error) {
	return s.ListMemoryFiltered(ctx, "", agentID, "")
}

func (s *Store) ListMemoryFiltered(ctx context.Context, scope string, agentID string, key string) ([]MemoryEntry, error) {
	scope = strings.ToLower(strings.TrimSpace(scope))
	agentID = strings.TrimSpace(agentID)
	key = normalizeMemoryKey(key)
	clauses := make([]string, 0, 3)
	args := make([]any, 0, 3)
	if scope != "" {
		if scope != "workspace" && scope != "agent" {
			return nil, fmt.Errorf("memory scope must be workspace or agent")
		}
		clauses = append(clauses, "scope = ?")
		args = append(args, scope)
	} else if agentID != "" {
		clauses = append(clauses, "(scope = 'workspace' OR agent_id = ?)")
		args = append(args, agentID)
	}
	if scope == "agent" && agentID != "" {
		clauses = append(clauses, "agent_id = ?")
		args = append(args, agentID)
	}
	if scope == "workspace" {
		clauses = append(clauses, "agent_id = ''")
	}
	if key != "" {
		clauses = append(clauses, "memory_key = ?")
		args = append(args, key)
	}
	whereSQL := ""
	if len(clauses) > 0 {
		whereSQL = " WHERE " + strings.Join(clauses, " AND ")
	}
	rows := []struct {
		PayloadJSON string `db:"payload_json"`
	}{}
	if err := s.db.SelectContext(ctx, &rows, `SELECT payload_json FROM `+tableMemory+whereSQL+` ORDER BY updated_at DESC, id ASC`, args...); err != nil {
		return nil, err
	}
	entries := make([]MemoryEntry, 0, len(rows))
	for _, row := range rows {
		var entry MemoryEntry
		if err := json.Unmarshal([]byte(row.PayloadJSON), &entry); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (s *Store) DeleteMemory(ctx context.Context, id string) error {
	id = normalizeID(id)
	if id == "" {
		return os.ErrNotExist
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM `+tableMemory+` WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if rows, rowErr := result.RowsAffected(); rowErr == nil && rows == 0 {
		return os.ErrNotExist
	}
	return nil
}
