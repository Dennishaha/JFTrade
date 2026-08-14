package completionreview

import (
	"strings"
	"testing"

	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func TestParseRejectsInconsistentCompletionReviewResponses(t *testing.T) {
	tests := []string{
		`{"decision":"complete","confidence":0.9,"reasonCode":"answer_complete","continuation":""} trailing`,
		`{"decision":"complete","confidence":1.1,"reasonCode":"answer_complete","continuation":""}`,
		`{"decision":"complete","confidence":0.9,"reasonCode":"unknown","continuation":""}`,
		`{"decision":"complete","confidence":0.9,"reasonCode":"answer_complete","continuation":"extra"}`,
		`{"decision":"append","confidence":0.9,"reasonCode":"answer_complete","continuation":"extra"}`,
		`{"decision":"unknown","confidence":0.9,"reasonCode":"missing_action_plan","continuation":"extra"}`,
	}
	for _, raw := range tests {
		if response, err := Parse(raw); err == nil {
			t.Fatalf("Parse(%q)=%+v, want error", raw, response)
		}
	}
	response, err := Parse(`{"decision":"append","confidence":0.9,"reasonCode":"missing_action_plan","continuation":" next "}`)
	if err != nil || response.Continuation != "next" {
		t.Fatalf("valid append response=%+v err=%v", response, err)
	}
}

func TestPrepareIncludesLatestAnsweredInputWithoutToolOutputs(t *testing.T) {
	run := assistantmodel.Run{
		Status: assistantmodel.RunStatusRunning, WorkMode: assistantmodel.WorkModeChat, UserMessage: "分析并给方案",
		ToolCalls: []assistantmodel.ToolCall{
			{ToolName: "portfolio.accounts", Status: "SUCCEEDED", Permission: "read_internal"},
			{ToolName: "portfolio.positions", Status: "SUCCEEDED", Permission: "read_external"},
		},
		InputRequests: []assistantmodel.InputRequest{{
			ID: "request-1", Status: assistantmodel.InputRequestStatusAnswered,
			Questions: []assistantmodel.InputQuestion{
				{ID: "market", Question: "哪个市场？", Options: []assistantmodel.InputOption{{ID: "hk", Label: "港股"}}},
				{ID: "focus", Question: "关注什么？", AllowOther: true},
			},
			Answers: []assistantmodel.InputAnswer{
				{QuestionID: "market", OptionID: "hk"},
				{QuestionID: "focus", OtherText: "集中度"},
				{QuestionID: "missing", OptionID: "none"},
			},
		}},
	}
	reason, prompt, err := Prepare(
		assistantmodel.Agent{ID: assistantmodel.DefaultBuiltinAgentID, WorkMode: assistantmodel.WorkModeChat},
		run,
		assistantmodel.AssistantExecutionResult{Reply: "当前回复"},
	)
	if err != nil || reason != "" {
		t.Fatalf("Prepare reason=%q err=%v", reason, err)
	}
	for _, expected := range []string{"request-1", "哪个市场？", "港股", "集中度", "portfolio.accounts", "当前回复"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt missing %q: %s", expected, prompt)
		}
	}
	if _, err := Prompt("request", make(chan int), "reply", nil); err == nil {
		t.Fatal("Prompt accepted a non-JSON answer")
	}
}

func TestIneligibleReasonClassifiesControlAndToolStates(t *testing.T) {
	eligible := Eligibility{
		DefaultAgent: true, ChatMode: true, Reply: "reply", SuccessfulState: true,
		Tools: []ToolStatus{
			{Name: "one", Status: "SUCCEEDED", Permission: "read"},
			{Name: "two", Status: "SUCCEEDED", Permission: "read_external"},
		},
	}
	tests := []struct {
		name   string
		mutate func(*Eligibility)
		want   string
	}{
		{name: "custom agent", mutate: func(input *Eligibility) { input.DefaultAgent = false }, want: "custom_agent"},
		{name: "loop mode", mutate: func(input *Eligibility) { input.ChatMode = false }, want: "non_chat_mode"},
		{name: "workflow child", mutate: func(input *Eligibility) { input.WorkflowChild = true }, want: "workflow_child"},
		{name: "empty reply", mutate: func(input *Eligibility) { input.Reply = " " }, want: "empty_reply"},
		{name: "pending input", mutate: func(input *Eligibility) { input.PendingInput = true }, want: "pending_input"},
		{name: "requires user", mutate: func(input *Eligibility) { input.Tools[0].RequiresUser = true }, want: "tool_not_succeeded"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := eligible
			input.Tools = append([]ToolStatus(nil), eligible.Tools...)
			test.mutate(&input)
			if got := IneligibleReason(input); got != test.want {
				t.Fatalf("IneligibleReason=%q want=%q", got, test.want)
			}
		})
	}
}

func TestCoordinatorRejectsDuplicateAndMissingApplications(t *testing.T) {
	coordinator := NewCoordinator()
	if coordinator.MarkApplied("missing", "target") {
		t.Fatal("missing memo was marked applied")
	}
	coordinator.Once("run", func() Outcome { return Outcome{Outcome: "complete"} })
	if !coordinator.MarkApplied("run", "target") || coordinator.MarkApplied("run", "target") {
		t.Fatal("coordinator did not enforce one application per target")
	}
	coordinator.Clear("run")
	if coordinator.MarkApplied("run", "target") {
		t.Fatal("cleared memo was marked applied")
	}
	var nilCoordinator *Coordinator
	nilCoordinator.Clear("run")
}
