package adk

import (
	"strings"
	"testing"
)

func TestWorkflowHelpersSummariesStatusesAndChildren(t *testing.T) {
	t.Run("workflow self task summary prefers explicit result", func(t *testing.T) {
		task := Task{Title: "Research NVDA", ResultSummary: "已完成研究并给出结论。"}
		if got := workflowSelfTaskSummary(task); got != task.ResultSummary {
			t.Fatalf("workflowSelfTaskSummary = %q, want explicit result summary", got)
		}
	})

	t.Run("workflow self task summary falls back and truncates", func(t *testing.T) {
		detail := strings.Repeat("细节", 80)
		task := Task{Title: "研究任务", Description: detail}
		got := workflowSelfTaskSummary(task)
		if !strings.HasPrefix(got, "研究任务 已由父智能体完成：") || !strings.HasSuffix(got, "...") {
			t.Fatalf("workflowSelfTaskSummary = %q, want titled truncated fallback", got)
		}
	})

	t.Run("workflow self task summary defaults to generic subject", func(t *testing.T) {
		if got := workflowSelfTaskSummary(Task{}); got != "任务 已由父智能体完成。" {
			t.Fatalf("workflowSelfTaskSummary empty = %q", got)
		}
	})

	t.Run("child run ids stay unique", func(t *testing.T) {
		children := []Run{{ID: "child-1"}, {ID: " child-2 "}, {ID: "child-1"}, {ID: ""}}
		if got := childRunIDs(children); len(got) != 2 || got[0] != "child-1" || got[1] != "child-2" {
			t.Fatalf("childRunIDs = %#v, want unique trimmed ids", got)
		}
	})

	t.Run("workflow pending reply reflects mode and failures", func(t *testing.T) {
		cases := []struct {
			name string
			run  Run
			want string
		}{
			{name: "task pending", run: Run{Status: RunStatusPending, WorkMode: WorkModeTask}, want: "任务编排正在等待审批。"},
			{name: "loop pending", run: Run{Status: RunStatusPending, WorkMode: WorkModeLoop}, want: "目标模式正在等待审批。"},
			{name: "generic pending", run: Run{Status: RunStatusPending, WorkMode: WorkModeChat}, want: "工作流正在等待审批。"},
			{name: "failure reason wins", run: Run{Status: RunStatusFailed, FailureReason: "provider timeout", Message: "ignored"}, want: "provider timeout"},
			{name: "fallback message", run: Run{Status: RunStatusCompleted, Message: "all done"}, want: "all done"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				if got := workflowPendingReply(tc.run); got != tc.want {
					t.Fatalf("workflowPendingReply(%+v) = %q, want %q", tc.run, got, tc.want)
				}
			})
		}
	})

	t.Run("workflow summary includes objective children and replies", func(t *testing.T) {
		parent := Run{
			WorkMode:    WorkModeLoop,
			Objective:   "完成 NVDA 策略研究",
			ChildRunIDs: []string{"child-1", "child-2"},
		}
		replies := []string{" 第一条结果 ", "", strings.Repeat("很长的结果", 80)}
		got := workflowSummary(parent, replies)
		if !strings.Contains(got, "目标模式已完成。") || !strings.Contains(got, "目标：完成 NVDA 策略研究") || !strings.Contains(got, "子运行：2 个") {
			t.Fatalf("workflowSummary = %q, want mode/objective/child count", got)
		}
		if !strings.Contains(got, "\n- 第一条结果") || !strings.Contains(got, "...") {
			t.Fatalf("workflowSummary = %q, want trimmed bullet replies and truncation", got)
		}
	})

	t.Run("workflow blocking status set", func(t *testing.T) {
		blocking := []string{RunStatusPending, RunStatusFailed, RunStatusTimedOut, RunStatusCancelled, RunStatusDenied}
		for _, status := range blocking {
			if !isWorkflowBlockingStatus(status) {
				t.Fatalf("isWorkflowBlockingStatus(%q) = false, want true", status)
			}
		}
		if isWorkflowBlockingStatus(RunStatusCompleted) {
			t.Fatalf("isWorkflowBlockingStatus(%q) = true, want false", RunStatusCompleted)
		}
	})

	t.Run("missing final reply error text is stable", func(t *testing.T) {
		if err := errADKMissingFinalReply(); err == nil || !strings.Contains(err.Error(), "最终回复") {
			t.Fatalf("errADKMissingFinalReply = %v, want final reply error", err)
		}
	})
}

func TestWorkflowTaskHelperPromptsAndSummaries(t *testing.T) {
	parent := Run{Objective: "整理回测结论", UserMessage: "请帮我形成最后答复"}

	if got := taskOrchestratorNudge(parent); !strings.Contains(got, "workflow.tasks.list") || !strings.Contains(got, parent.Objective) {
		t.Fatalf("taskOrchestratorNudge = %q, want workflow task guidance with objective", got)
	}
	if got := goalFinalReplyPrompt(parent); !strings.Contains(got, "不要再调用工具") || !strings.Contains(got, parent.Objective) {
		t.Fatalf("goalFinalReplyPrompt = %q, want final-reply guidance with objective", got)
	}

	tasks := []Task{
		{ID: "task-1", ResultSummary: "第一条结论"},
		{ID: "task-2", ResultSummary: "   "},
		{ID: "task-3", ResultSummary: "第二条结论"},
	}
	if got := workflowTaskResultSummaries(tasks); len(got) != 2 || got[0] != "第一条结论" || got[1] != "第二条结论" {
		t.Fatalf("workflowTaskResultSummaries = %#v, want non-empty summaries only", got)
	}

	args := map[string]any{"dependsOn": []any{" task-a ", 42, "", "task-b"}}
	if got := plannerStringSliceArg(args, "dependsOn"); len(got) != 3 || got[0] != "task-a" || got[1] != "42" || got[2] != "task-b" {
		t.Fatalf("plannerStringSliceArg = %#v, want trimmed stringified values", got)
	}
	if got := plannerStringSliceArg(nil, "dependsOn"); got != nil {
		t.Fatalf("plannerStringSliceArg nil args = %#v, want nil", got)
	}
	if got := plannerStringSliceArg(map[string]any{"dependsOn": "task-a"}, "dependsOn"); got != nil {
		t.Fatalf("plannerStringSliceArg scalar = %#v, want nil", got)
	}
}

func TestWorkflowTaskToolsetNameAndTaskSorting(t *testing.T) {
	if got := (&workflowTaskToolset{}).Name(); got != "jftrade-workflow-task-tools" {
		t.Fatalf("workflowTaskToolset.Name() = %q", got)
	}

	tasks := []Task{
		{ID: "task-z", Order: 0, CreatedAt: "2026-06-22T09:02:00Z"},
		{ID: "task-b", Order: 2, CreatedAt: "2026-06-22T09:01:00Z"},
		{ID: "task-a", Order: 1, CreatedAt: "2026-06-22T09:03:00Z"},
		{ID: "task-c", Order: 2, CreatedAt: "2026-06-22T09:00:00Z"},
	}
	sortWorkflowTasks(tasks)
	if got := []string{tasks[0].ID, tasks[1].ID, tasks[2].ID, tasks[3].ID}; strings.Join(got, ",") != "task-a,task-c,task-b,task-z" {
		t.Fatalf("sorted task order = %v, want task-a,task-c,task-b,task-z", got)
	}
}

func TestWorkflowTaskGoalStateHelpers(t *testing.T) {
	decision := &workflowGoalDecision{}
	decision.beginDecision()
	if !decision.decisionPhase() {
		t.Fatal("decisionPhase = false after beginDecision")
	}
	decision.setContinue("需要更多数据")
	if snapshot := decision.snapshot(); snapshot.reason != "需要更多数据" || snapshot.status != "continue" {
		t.Fatalf("goal decision snapshot fields = status:%q reason:%q, want continue/需要更多数据", snapshot.status, snapshot.reason)
	}
	decision.setComplete("目标已完成")
	if snapshot := decision.snapshot(); snapshot.summary != "目标已完成" || snapshot.status != "complete" {
		t.Fatalf("goal decision snapshot fields = status:%q summary:%q, want complete/目标已完成", snapshot.status, snapshot.summary)
	}
	decision.reset()
	if decision.decisionPhase() {
		t.Fatal("decisionPhase = true after reset")
	}
}

func TestWorkflowAppendUniqueStringAndErrorShape(t *testing.T) {
	values := appendUniqueString([]string{"task-a"}, " task-b ")
	values = appendUniqueString(values, "task-a")
	if len(values) != 2 || values[1] != "task-b" {
		t.Fatalf("appendUniqueString = %#v, want unique trimmed append", values)
	}

	err := errADKMissingFinalReply()
	if !strings.Contains(err.Error(), "模型未返回最终回复") {
		t.Fatalf("errADKMissingFinalReply = %v, want stable user-facing text", err)
	}
}
