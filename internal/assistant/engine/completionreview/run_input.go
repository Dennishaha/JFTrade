package completionreview

import (
	"strings"

	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

const requestUserTool = "interaction.request_user"

func Prepare(
	agent assistantmodel.Agent,
	run assistantmodel.Run,
	result assistantmodel.AssistantExecutionResult,
) (string, string, error) {
	tools := make([]ToolStatus, 0, len(run.ToolCalls))
	for _, call := range run.ToolCalls {
		tools = append(tools, ToolStatus{
			Name: call.ToolName, Status: call.Status, Permission: call.Permission,
			Failed: call.Error != nil, RequiresUser: call.RequiresUser, InputTool: call.ToolName == requestUserTool,
		})
	}
	workMode := run.WorkMode
	if strings.TrimSpace(workMode) == "" {
		workMode = agent.WorkMode
	}
	pendingInput := run.InputRequest != nil && run.InputRequest.Status == assistantmodel.InputRequestStatusPending
	for _, request := range run.InputRequests {
		if request.Status == assistantmodel.InputRequestStatusPending {
			pendingInput = true
			break
		}
	}
	successfulState := run.Status == "" || run.Status == assistantmodel.RunStatusRunning || run.Status == assistantmodel.RunStatusCompleted
	reason := IneligibleReason(Eligibility{
		DefaultAgent:  agent.ID == assistantmodel.DefaultBuiltinAgentID,
		ChatMode:      assistantmodel.NormalizeAgentDefaultWorkMode(workMode) == assistantmodel.WorkModeChat,
		WorkflowChild: strings.TrimSpace(run.ParentRunID) != "", Reply: result.Reply,
		Degraded:        run.Degraded || strings.TrimSpace(run.ErrorCode) != "" || strings.TrimSpace(run.FailureReason) != "",
		SuccessfulState: successfulState, PendingApproval: len(assistantmodel.PendingApprovalsOnly(run.PendingApprovals)) > 0,
		PendingInput: pendingInput, Tools: tools,
	})
	if reason != "" {
		return reason, "", nil
	}
	prompt, err := buildRunPrompt(run, result.Reply, tools)
	return "", prompt, err
}

func buildRunPrompt(run assistantmodel.Run, reply string, tools []ToolStatus) (string, error) {
	var latestAnswer any
	if request := latestAnsweredInputRequest(run); request != nil {
		latestAnswer = inputResponsePayload(*request)
	}
	prompt, err := Prompt(run.UserMessage, latestAnswer, reply, tools)
	return prompt, err
}

func latestAnsweredInputRequest(run assistantmodel.Run) *assistantmodel.InputRequest {
	if run.InputRequest != nil && run.InputRequest.Status == assistantmodel.InputRequestStatusAnswered {
		return assistantmodel.NormalizeInputRequest(run.InputRequest)
	}
	for index := len(run.InputRequests) - 1; index >= 0; index-- {
		if run.InputRequests[index].Status == assistantmodel.InputRequestStatusAnswered {
			return assistantmodel.NormalizeInputRequest(&run.InputRequests[index])
		}
	}
	return nil
}

func inputResponsePayload(request assistantmodel.InputRequest) map[string]any {
	answers := make([]map[string]any, 0, len(request.Answers))
	for _, answer := range request.Answers {
		item := map[string]any{"questionId": answer.QuestionID}
		for _, question := range request.Questions {
			if question.ID != answer.QuestionID {
				continue
			}
			item["question"] = question.Question
			if answer.OtherText != "" {
				item["otherText"] = answer.OtherText
				break
			}
			for _, option := range question.Options {
				if option.ID == answer.OptionID {
					item["optionId"] = option.ID
					item["answer"] = option.Label
					break
				}
			}
			break
		}
		answers = append(answers, item)
	}
	return map[string]any{"requestId": request.ID, "answers": answers}
}
