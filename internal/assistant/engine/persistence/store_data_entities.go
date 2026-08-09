package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/google/uuid"
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func (s *StoreCore) ListSkills(ctx context.Context) ([]jfadkmodel.Skill, error) {
	var skills []jfadkmodel.Skill
	if err := s.ListJSON(ctx, tableSkills, "id ASC", &skills); err != nil {
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

func (s *StoreCore) SaveSkill(ctx context.Context, skill jfadkmodel.Skill) (jfadkmodel.Skill, error) {
	now := jfadkmodel.NowString()
	if skill.ID == "" {
		skill.ID = jfadkmodel.NormalizeID(skill.DisplayName)
	}
	if skill.ID == "" {
		skill.ID = "skill-" + uuid.NewString()
	}
	existing, ok, err := s.Skill(ctx, skill.ID)
	if err != nil {
		return jfadkmodel.Skill{}, err
	}
	if ok && skill.CreatedAt == "" {
		skill.CreatedAt = existing.CreatedAt
	}
	if skill.CreatedAt == "" {
		skill.CreatedAt = now
	}
	skill.UpdatedAt = now
	return skill, s.SaveJSON(ctx, tableSkills, skill.ID, skill.CreatedAt, skill.UpdatedAt, skill)
}

func (s *StoreCore) Skill(ctx context.Context, id string) (jfadkmodel.Skill, bool, error) {
	var skill jfadkmodel.Skill
	ok, err := s.GetJSON(ctx, tableSkills, id, &skill)
	return skill, ok, err
}

func (s *StoreCore) DeleteSkill(ctx context.Context, id string) error {
	if _, err := s.DB().ExecContext(ctx, `DELETE FROM `+tableSkills+` WHERE id = ? AND json_extract(payload_json, '$.builtin') = 0`, strings.TrimSpace(id)); err != nil {
		return err
	}
	return nil
}

func (s *StoreCore) SaveOptimizationTask(ctx context.Context, task jfadkmodel.OptimizationTask) (jfadkmodel.OptimizationTask, error) {
	now := jfadkmodel.NowString()
	if strings.TrimSpace(task.ID) == "" {
		task.ID = "opt-" + uuid.NewString()
	}
	existing, ok, err := s.OptimizationTask(ctx, task.ID)
	if err != nil {
		return jfadkmodel.OptimizationTask{}, err
	}
	if ok && task.CreatedAt == "" {
		task.CreatedAt = existing.CreatedAt
	}
	if task.CreatedAt == "" {
		task.CreatedAt = now
	}
	task.UpdatedAt = now
	return task, s.SaveJSON(ctx, tableOptimizations, task.ID, task.CreatedAt, task.UpdatedAt, task)
}

func (s *StoreCore) OptimizationTask(ctx context.Context, id string) (jfadkmodel.OptimizationTask, bool, error) {
	var task jfadkmodel.OptimizationTask
	ok, err := s.GetJSON(ctx, tableOptimizations, id, &task)
	return task, ok, err
}

func (s *StoreCore) ListOptimizationTasks(ctx context.Context) ([]jfadkmodel.OptimizationTask, error) {
	var tasks []jfadkmodel.OptimizationTask
	return tasks, s.ListJSON(ctx, tableOptimizations, "updated_at DESC, id ASC", &tasks)
}

func (s *StoreCore) SaveTask(ctx context.Context, req jfadkmodel.TaskWriteRequest) (jfadkmodel.Task, error) {
	id := jfadkmodel.NormalizeID(req.ID)
	if id == "" {
		id = "task-" + uuid.NewString()
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return jfadkmodel.Task{}, fmt.Errorf("task title is required")
	}
	status, err := jfadkmodel.NormalizeTaskStatus(req.Status)
	if err != nil {
		return jfadkmodel.Task{}, err
	}
	dependsOn, err := jfadkmodel.NormalizeTaskDependsOn(id, req.DependsOn)
	if err != nil {
		return jfadkmodel.Task{}, err
	}
	now := jfadkmodel.NowString()
	existing, ok, err := s.Task(ctx, id)
	if err != nil {
		return jfadkmodel.Task{}, err
	}
	createdAt := now
	if ok {
		createdAt = existing.CreatedAt
	}
	task := jfadkmodel.Task{
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
		PlannerWarnings:     jfadkmodel.NormalizeStringSlice(req.PlannerWarnings),
		CreatedAt:           createdAt, UpdatedAt: now,
	}
	return s.saveTask(ctx, task)
}

func (s *StoreCore) UpdateTask(ctx context.Context, id string, req jfadkmodel.TaskPatchRequest) (jfadkmodel.Task, error) {
	id = jfadkmodel.NormalizeID(id)
	if id == "" {
		return jfadkmodel.Task{}, os.ErrNotExist
	}
	task, ok, err := s.Task(ctx, id)
	if err != nil {
		return jfadkmodel.Task{}, err
	}
	if !ok {
		return jfadkmodel.Task{}, os.ErrNotExist
	}
	if err := applyTaskPatch(&task, id, req); err != nil {
		return jfadkmodel.Task{}, err
	}
	task.UpdatedAt = jfadkmodel.NowString()
	return s.saveTask(ctx, task)
}

func applyTaskPatch(task *jfadkmodel.Task, id string, req jfadkmodel.TaskPatchRequest) error {
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
		status, err := jfadkmodel.NormalizeTaskStatus(*req.Status)
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
		dependsOn, err := jfadkmodel.NormalizeTaskDependsOn(id, req.DependsOn)
		if err != nil {
			return err
		}
		task.DependsOn = dependsOn
	}
	applyTaskMetadataPatch(task, req)
	return nil
}

func applyTaskMetadataPatch(task *jfadkmodel.Task, req jfadkmodel.TaskPatchRequest) {
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
		task.PlannerWarnings = jfadkmodel.NormalizeStringSlice(req.PlannerWarnings)
	}
}

func (s *StoreCore) saveTask(ctx context.Context, task jfadkmodel.Task) (jfadkmodel.Task, error) {
	payload, err := json.Marshal(task)
	if err != nil {
		return jfadkmodel.Task{}, err
	}
	_, err = s.DB().ExecContext(ctx, `INSERT INTO `+tableTasks+` (id, status, agent_id, run_id, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET status = excluded.status, agent_id = excluded.agent_id, run_id = excluded.run_id, payload_json = excluded.payload_json, updated_at = excluded.updated_at`, task.ID, task.Status, task.AgentID, task.RunID, string(payload), task.CreatedAt, task.UpdatedAt)
	return task, err
}

func (s *StoreCore) Task(ctx context.Context, id string) (jfadkmodel.Task, bool, error) {
	var task jfadkmodel.Task
	ok, err := s.GetJSON(ctx, tableTasks, id, &task)
	return task, ok, err
}

func (s *StoreCore) ListTasksPage(ctx context.Context, status string, agentID string, runID string, limit int, offset int) ([]jfadkmodel.Task, int, error) {
	clauses := make([]string, 0, 3)
	args := make([]any, 0, 3)
	if status = strings.ToUpper(strings.TrimSpace(status)); status != "" {
		if _, err := jfadkmodel.NormalizeTaskStatus(status); err != nil {
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
	var tasks []jfadkmodel.Task
	total, err := s.ListJSONPage(ctx, tableTasks, clauses, args, "updated_at DESC, id ASC", limit, offset, &tasks)
	return tasks, total, err
}

func (s *StoreCore) DeleteTask(ctx context.Context, id string) error {
	id = jfadkmodel.NormalizeID(id)
	if id == "" {
		return os.ErrNotExist
	}
	result, err := s.DB().ExecContext(ctx, `DELETE FROM `+tableTasks+` WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if rows, rowErr := result.RowsAffected(); rowErr == nil && rows == 0 {
		return os.ErrNotExist
	}
	return nil
}

func (s *StoreCore) SaveMemory(ctx context.Context, req jfadkmodel.MemoryWriteRequest) (jfadkmodel.MemoryEntry, error) {
	key := jfadkmodel.NormalizeMemoryKey(req.Key)
	if key == "" {
		return jfadkmodel.MemoryEntry{}, fmt.Errorf("memory key is required")
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
		return jfadkmodel.MemoryEntry{}, fmt.Errorf("memory scope must be workspace or agent")
	}
	agentID := strings.TrimSpace(req.AgentID)
	if scope == "workspace" {
		agentID = ""
	} else if agentID == "" {
		return jfadkmodel.MemoryEntry{}, fmt.Errorf("agent memory requires agentId")
	} else if _, ok, err := s.Agent(ctx, agentID); err != nil {
		return jfadkmodel.MemoryEntry{}, err
	} else if !ok {
		return jfadkmodel.MemoryEntry{}, fmt.Errorf("agent not found")
	}
	id := jfadkmodel.NormalizeID(scope + "-" + agentID + "-" + key)
	now := jfadkmodel.NowString()
	existing, ok, err := s.Memory(ctx, id)
	if err != nil {
		return jfadkmodel.MemoryEntry{}, err
	}
	createdAt := now
	if ok {
		createdAt = existing.CreatedAt
	}
	entry := jfadkmodel.MemoryEntry{ID: id, AgentID: agentID, Key: key, Value: value, Scope: scope, CreatedAt: createdAt, UpdatedAt: now}
	payload, err := json.Marshal(entry)
	if err != nil {
		return jfadkmodel.MemoryEntry{}, err
	}
	_, err = s.DB().ExecContext(ctx, `INSERT INTO `+tableMemory+` (id, agent_id, scope, memory_key, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(agent_id, scope, memory_key) DO UPDATE SET payload_json = excluded.payload_json, updated_at = excluded.updated_at`, entry.ID, entry.AgentID, entry.Scope, entry.Key, string(payload), entry.CreatedAt, entry.UpdatedAt)
	return entry, err
}

func (s *StoreCore) Memory(ctx context.Context, id string) (jfadkmodel.MemoryEntry, bool, error) {
	var entry jfadkmodel.MemoryEntry
	ok, err := s.GetJSON(ctx, tableMemory, id, &entry)
	return entry, ok, err
}

func (s *StoreCore) ListMemory(ctx context.Context, agentID string) ([]jfadkmodel.MemoryEntry, error) {
	return s.ListMemoryFiltered(ctx, "", agentID, "")
}

func (s *StoreCore) ListMemoryFiltered(ctx context.Context, scope string, agentID string, key string) ([]jfadkmodel.MemoryEntry, error) {
	scope = strings.ToLower(strings.TrimSpace(scope))
	agentID = strings.TrimSpace(agentID)
	key = jfadkmodel.NormalizeMemoryKey(key)
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
	if err := s.DB().SelectContext(ctx, &rows, `SELECT payload_json FROM `+tableMemory+whereSQL+` ORDER BY updated_at DESC, id ASC`, args...); err != nil {
		return nil, err
	}
	entries := make([]jfadkmodel.MemoryEntry, 0, len(rows))
	for _, row := range rows {
		var entry jfadkmodel.MemoryEntry
		if err := json.Unmarshal([]byte(row.PayloadJSON), &entry); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (s *StoreCore) DeleteMemory(ctx context.Context, id string) error {
	id = jfadkmodel.NormalizeID(id)
	if id == "" {
		return os.ErrNotExist
	}
	result, err := s.DB().ExecContext(ctx, `DELETE FROM `+tableMemory+` WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if rows, rowErr := result.RowsAffected(); rowErr == nil && rows == 0 {
		return os.ErrNotExist
	}
	return nil
}
