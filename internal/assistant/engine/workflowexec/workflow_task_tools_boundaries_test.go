package workflowexec

import (
	"context"
	"errors"
	"strings"
	"testing"

	enginepersistence "github.com/jftrade/jftrade-main/internal/assistant/engine/persistence"
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func TestWorkflowTaskToolsetBusinessBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("delegate reuses live child run", func(t *testing.T) {
		runtime := newTestRuntime(t)
		now := jfadkmodel.NowString()
		parent := mustSaveRun(t, runtime, Run{
			ID: "task-tools-delegate-parent", SessionID: "task-tools-delegate-session", AgentID: "agent",
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: now, UpdatedAt: now,
		})
		task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID: "task-tools-delegate-task", Title: "delegate", Status: "TODO", AgentID: parent.AgentID,
			RunID: parent.ID, Executor: workflowTaskExecutorChild, Order: 1, WorkflowMode: parent.WorkMode,
		})
		if err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		child := mustSaveRun(t, runtime, Run{
			ID: "task-tools-delegate-child", SessionID: parent.SessionID, AgentID: parent.AgentID, ParentRunID: parent.ID,
			Status: RunStatusPending, Message: "waiting for approval", CreatedAt: now, UpdatedAt: now,
		})
		if _, err := runtime.Store().UpdateTask(ctx, task.ID, TaskPatchRequest{RunID: new(child.ID)}); err != nil {
			t.Fatalf("UpdateTask child run: %v", err)
		}
		toolset := NewWorkflowTaskToolset(&WorkflowExecutor{runtime: runtime}, parent.ID, "")
		toolset.Req = workflowRequest{Mode: WorkModeLoop, Session: Session{ID: parent.SessionID, AgentID: parent.AgentID}}
		reused, err := toolset.Delegate(map[string]any{"taskId": task.ID})
		if err != nil {
			t.Fatalf("delegate reuse: %v", err)
		}
		if reused["reused"] != true || reused["childRunId"] != child.ID || reused["pendingApproval"] != true {
			t.Fatalf("delegate reused result = %#v", reused)
		}
	})

	t.Run("merge task child projection pauses parent for pending child", func(t *testing.T) {
		runtime := newTestRuntime(t)
		parent := Run{
			ID: "task-tools-merge-parent", Status: RunStatusRunning,
			WorkflowPlan: []WorkflowStepState{{TaskID: "task-tools-merge-task"}},
		}
		child := Run{
			ID: "task-tools-merge-child", Status: RunStatusPending,
			PendingApprovals: []Approval{{ID: "task-tools-merge-approval", Status: ApprovalStatusPending}},
		}
		merged, err := (&WorkflowExecutor{runtime: runtime}).MergeTaskChildProjectionAt(ctx, parent, child, 0)
		if err != nil {
			t.Fatalf("merge task child projection: %v", err)
		}
		if merged.Status != RunStatusPending || len(merged.PendingApprovals) != 1 || merged.ChildRunIDs[0] != child.ID {
			t.Fatalf("merged parent = %+v", merged)
		}
	})
}

func TestWorkflowTaskToolsetErrorBranches(t *testing.T) {
	ctx := context.Background()

	newParentAndTask := func(t *testing.T, suffix string) (*Runtime, Run, Task, *WorkflowTaskToolset) {
		t.Helper()
		runtime := newTestRuntime(t)
		now := jfadkmodel.NowString()
		parent := mustSaveRun(t, runtime, Run{
			ID: "task-tools-error-parent-" + suffix, SessionID: "task-tools-error-session-" + suffix, AgentID: "agent",
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: now, UpdatedAt: now,
		})
		task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID: "task-tools-error-task-" + suffix, Title: "task", Status: "TODO", AgentID: parent.AgentID,
			RunID: parent.ID, Order: 1, WorkflowMode: parent.WorkMode,
		})
		if err != nil {
			t.Fatalf("SaveTask %s: %v", suffix, err)
		}
		toolset := NewWorkflowTaskToolset(&WorkflowExecutor{runtime: runtime}, parent.ID, "")
		toolset.Req = workflowRequest{Mode: WorkModeLoop, Session: Session{ID: parent.SessionID, AgentID: parent.AgentID}}
		return runtime, parent, task, toolset
	}

	t.Run("list surfaces task lookup failures after parent load succeeds", func(t *testing.T) {
		runtime, _, _, toolset := newParentAndTask(t, "list")
		if _, err := runtime.Store().DB().ExecContext(ctx, `DROP TABLE `+enginepersistence.TableTasks); err != nil {
			t.Fatalf("drop tasks: %v", err)
		}
		if _, err := toolset.List(nil); err == nil || !strings.Contains(err.Error(), enginepersistence.TableTasks) {
			t.Fatalf("list dropped tasks err = %v, want %s", err, enginepersistence.TableTasks)
		}
	})

	t.Run("claim complete and block surface task update failures", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			call func(*WorkflowTaskToolset, Task) (map[string]any, error)
		}{
			{name: "claim", call: func(toolset *WorkflowTaskToolset, task Task) (map[string]any, error) {
				return toolset.Claim(map[string]any{"taskId": task.ID})
			}},
			{name: "complete", call: func(toolset *WorkflowTaskToolset, task Task) (map[string]any, error) {
				return toolset.Complete(map[string]any{"taskId": task.ID, "resultSummary": "done"})
			}},
			{name: "block", call: func(toolset *WorkflowTaskToolset, task Task) (map[string]any, error) {
				return toolset.Block(map[string]any{"taskId": task.ID, "reason": "blocked"})
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				runtime, _, task, toolset := newParentAndTask(t, tc.name)
				if _, err := runtime.Store().DB().ExecContext(ctx, `CREATE TRIGGER fail_task_update_`+tc.name+` BEFORE UPDATE ON `+enginepersistence.TableTasks+` BEGIN SELECT RAISE(FAIL, 'task update failed'); END`); err != nil {
					t.Fatalf("create task update trigger: %v", err)
				}
				if _, err := tc.call(toolset, task); err == nil || !strings.Contains(err.Error(), "task update failed") {
					t.Fatalf("%s err = %v, want task update failed", tc.name, err)
				}
			})
		}
	})

	t.Run("complete and delegate surface malformed delegated child runs", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			call func(*WorkflowTaskToolset, Task) (map[string]any, error)
		}{
			{name: "complete", call: func(toolset *WorkflowTaskToolset, task Task) (map[string]any, error) {
				return toolset.Complete(map[string]any{"taskId": task.ID})
			}},
			{name: "delegate", call: func(toolset *WorkflowTaskToolset, task Task) (map[string]any, error) {
				return toolset.Delegate(map[string]any{"taskId": task.ID})
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				runtime, parent, task, toolset := newParentAndTask(t, "bad-child-"+tc.name)
				childRunID := "task-tools-bad-child-" + tc.name
				if _, err := runtime.Store().UpdateTask(ctx, task.ID, TaskPatchRequest{Executor: new(workflowTaskExecutorChild), RunID: &childRunID}); err != nil {
					t.Fatalf("UpdateTask child metadata: %v", err)
				}
				now := jfadkmodel.NowString()
				if _, err := runtime.Store().DB().ExecContext(ctx,
					`INSERT INTO `+enginepersistence.TableRuns+` (id, session_id, agent_id, status, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
					childRunID, parent.SessionID, parent.AgentID, RunStatusCompleted, `{`, now, now,
				); err != nil {
					t.Fatalf("insert malformed child run: %v", err)
				}
				if _, err := tc.call(toolset, task); err == nil {
					t.Fatalf("%s accepted malformed child run", tc.name)
				}
			})
		}
	})

	t.Run("delegate surfaces child execution and parent reload failures", func(t *testing.T) {
		t.Run("child execution errors are returned as unsuccessful delegate results", func(t *testing.T) {
			_, parent, task, toolset := newParentAndTask(t, "delegate-child-error")
			toolset.Req.Agent = Agent{ID: parent.AgentID, ProviderID: testProviderID, Status: AgentStatusEnabled}
			toolset.Req.EmitRun = true
			toolset.Req.OnDelta = func(ChatDelta) error { return errors.New("emit run failed") }

			result, err := toolset.Delegate(map[string]any{
				"taskId": task.ID, "agentRole": "Researcher",
			})
			if err != nil {
				t.Fatalf("delegate child err = %v", err)
			}
			if result["success"] != false || !strings.Contains(result["message"].(string), "emit run failed") {
				t.Fatalf("delegate child error result = %#v", result)
			}
		})

		t.Run("parent reload errors after a successful child run are surfaced", func(t *testing.T) {
			runtime, parent, task, toolset := newParentAndTask(t, "delegate-parent-error")
			toolset.Req.Agent = Agent{ID: parent.AgentID, ProviderID: testProviderID, Status: AgentStatusEnabled}
			if _, err := runtime.Store().DB().ExecContext(ctx, `CREATE TRIGGER delegate_corrupt_parent_after_child_update AFTER UPDATE ON `+enginepersistence.TableRuns+` WHEN NEW.id = '`+parent.ID+`' BEGIN UPDATE `+enginepersistence.TableRuns+` SET payload_json = '{' WHERE id = '`+parent.ID+`'; END`); err != nil {
				t.Fatalf("create corrupt parent trigger: %v", err)
			}

			if _, err := toolset.Delegate(map[string]any{"taskId": task.ID, "prompt": "finish the task"}); err == nil {
				t.Fatal("delegate accepted corrupted parent reload state")
			}
		})

		t.Run("missing parent reload after a successful child run returns not found", func(t *testing.T) {
			runtime, parent, task, toolset := newParentAndTask(t, "delegate-parent-missing")
			toolset.Req.Agent = Agent{ID: parent.AgentID, ProviderID: testProviderID, Status: AgentStatusEnabled}
			if _, err := runtime.Store().DB().ExecContext(ctx, `CREATE TRIGGER delegate_delete_parent_after_child_update AFTER UPDATE ON `+enginepersistence.TableRuns+` WHEN NEW.id = '`+parent.ID+`' BEGIN DELETE FROM `+enginepersistence.TableRuns+` WHERE id = '`+parent.ID+`'; END`); err != nil {
				t.Fatalf("create delete parent trigger: %v", err)
			}

			if _, err := toolset.Delegate(map[string]any{"taskId": task.ID, "prompt": "finish the task"}); err == nil || !strings.Contains(err.Error(), "parent run not found") {
				t.Fatalf("delegate missing parent err = %v, want parent run not found", err)
			}
		})
	})
}
