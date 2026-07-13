package adk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	adkagent "google.golang.org/adk/v2/agent"
	adkmodel "google.golang.org/adk/v2/model"
	adksession "google.golang.org/adk/v2/session"
	adktool "google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
	"google.golang.org/genai"
)

const interactionRequestUserTool = "interaction.request_user"

const maxInputRequestOptions = 3

const (
	InputRequestStatusPending   = "PENDING"
	InputRequestStatusAnswered  = "ANSWERED"
	InputRequestStatusCancelled = "CANCELLED"
)

type InputOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Recommended bool   `json:"recommended,omitempty"`
}

type InputQuestion struct {
	ID         string        `json:"id"`
	Question   string        `json:"question"`
	Options    []InputOption `json:"options"`
	AllowOther bool          `json:"allowOther"`
}

type InputAnswer struct {
	QuestionID string `json:"questionId"`
	OptionID   string `json:"optionId,omitempty"`
	OtherText  string `json:"otherText,omitempty"`
}

type InputRequest struct {
	ID             string          `json:"id"`
	RunID          string          `json:"runId"`
	AgentID        string          `json:"agentId"`
	FunctionCallID string          `json:"functionCallId"`
	Title          string          `json:"title,omitempty"`
	Status         string          `json:"status"`
	Questions      []InputQuestion `json:"questions"`
	Answers        []InputAnswer   `json:"answers,omitempty"`
	CreatedAt      string          `json:"createdAt"`
	UpdatedAt      string          `json:"updatedAt"`
	AnsweredAt     *string         `json:"answeredAt,omitempty"`
}

type InputResponseRequest struct {
	RequestID string        `json:"requestId"`
	Answers   []InputAnswer `json:"answers"`
}

type InputResolution struct {
	Request   InputRequest `json:"request"`
	Run       *Run         `json:"run,omitempty"`
	ParentRun *Run         `json:"parentRun,omitempty"`
	Message   *Message     `json:"message,omitempty"`
}

var (
	errInputRequestNotFound        = errors.New("input request not found")
	errInputRequestInvalid         = errors.New("input response is invalid")
	errInputRequestConflict        = errors.New("input request conflict")
	errInputRequestAlreadyAnswered = errors.New("input request already answered")
)

type requestUserToolOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Recommended bool   `json:"recommended,omitempty"`
}

type requestUserToolQuestion struct {
	Question   string                  `json:"question"`
	Options    []requestUserToolOption `json:"options"`
	AllowOther bool                    `json:"allowOther"`
}

type requestUserToolArgs struct {
	Title     string                    `json:"title,omitempty"`
	Questions []requestUserToolQuestion `json:"questions"`
}

func inputRequestToolDescriptor() ToolDescriptor {
	return ToolDescriptor{
		Name:         interactionRequestUserTool,
		DisplayName:  "向用户提问",
		Description:  "当任务确实需要用户偏好或方案决策时，一次性提交当前这一轮的所有问题并等待统一回答。每题必须提供 2 到 3 个选项；可接受自由回答时设置 allowOther。同一运行可在恢复后再次提问，但不能同时存在两个待回答请求。",
		Category:     "interaction",
		Permission:   "read_internal",
		RiskLevel:    "low",
		AllowedModes: allPermissionModes(),
		InputSchema:  inputRequestToolInputSchema(),
	}
}

func inputRequestToolInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{
				"type":        "string",
				"description": "Optional short title displayed above the questions.",
			},
			"questions": map[string]any{
				"type":        "array",
				"minItems":    1,
				"description": "All decisions needed for the current step. Ask them together.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"question": map[string]any{
							"type": "string",
						},
						"options": map[string]any{
							"type":        "array",
							"minItems":    2,
							"maxItems":    maxInputRequestOptions,
							"description": "Present exactly two or three concise choices.",
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"label": map[string]any{
										"type": "string",
									},
									"description": map[string]any{
										"type": "string",
									},
									"recommended": map[string]any{
										"type": "boolean",
									},
								},
								"required": []string{"label"},
							},
						},
						"allowOther": map[string]any{
							"type": "boolean",
						},
					},
					"required": []string{"question", "options"},
				},
			},
		},
		"required": []string{"questions"},
	}
}

type googleADKInputTool struct {
	descriptor ToolDescriptor
	tool       googleADKInputRunnableTool
}

type googleADKInputRunnableTool interface {
	adktool.Tool
	Declaration() *genai.FunctionDeclaration
	ProcessRequest(adkagent.Context, *adkmodel.LLMRequest) error
	Run(adkagent.Context, any) (map[string]any, error)
}

func (t *googleADKInputTool) Name() string {
	if t == nil || t.tool == nil {
		return interactionRequestUserTool
	}
	return t.tool.Name()
}

func (t *googleADKInputTool) Description() string {
	if t == nil || t.tool == nil {
		return inputRequestToolDescriptor().Description
	}
	return t.tool.Description()
}

func (t *googleADKInputTool) IsLongRunning() bool {
	return t != nil && t.tool != nil && t.tool.IsLongRunning()
}

func (t *googleADKInputTool) Declaration() *genai.FunctionDeclaration {
	if t == nil || t.tool == nil {
		return nil
	}
	return t.tool.Declaration()
}

func (t *googleADKInputTool) ProcessRequest(ctx adkagent.Context, request *adkmodel.LLMRequest) error {
	if t == nil || t.tool == nil {
		return fmt.Errorf("GO-ADK input tool is unavailable")
	}
	return t.tool.ProcessRequest(ctx, request)
}

func (t *googleADKInputTool) Run(ctx adkagent.Context, args any) (map[string]any, error) {
	if t == nil || t.tool == nil {
		return nil, fmt.Errorf("GO-ADK input tool is unavailable")
	}
	return t.tool.Run(ctx, args)
}

func (t *googleADKInputTool) googleADKToolDescriptor() ToolDescriptor {
	if t == nil {
		return inputRequestToolDescriptor()
	}
	return t.descriptor
}

func newGoogleADKInputTool() (*googleADKInputTool, error) {
	descriptor := inputRequestToolDescriptor()
	schema, err := googleADKJSONSchemaFromMap(descriptor.InputSchema)
	if err != nil {
		return nil, fmt.Errorf("convert GO-ADK input tool schema: %w", err)
	}
	inner, err := functiontool.New(functiontool.Config{
		Name: interactionRequestUserTool,
		Description: "Ask the user for decisions that cannot be inferred or retrieved. " +
			"Collect every required decision in one call. Each question must offer two or three options; " +
			"set allowOther when a free-form alternative is acceptable. Wait for the current response before asking again.",
		InputSchema:   schema,
		IsLongRunning: true,
	}, func(_ adkagent.Context, args requestUserToolArgs) (map[string]any, error) {
		if _, err := buildInputRequest("validation", "validation", "validation", args); err != nil {
			return nil, err
		}
		return map[string]any{"status": "pending"}, nil
	})
	if err != nil {
		return nil, err
	}
	tool, ok := inner.(googleADKInputRunnableTool)
	if !ok {
		return nil, fmt.Errorf("GO-ADK input tool is not runnable")
	}
	return &googleADKInputTool{descriptor: descriptor, tool: tool}, nil
}

func buildInputRequest(runID string, agentID string, functionCallID string, args requestUserToolArgs) (*InputRequest, error) {
	runID = strings.TrimSpace(runID)
	functionCallID = strings.TrimSpace(functionCallID)
	if runID == "" || functionCallID == "" {
		return nil, fmt.Errorf("%w: run and function call are required", errInputRequestInvalid)
	}
	if len(args.Questions) == 0 {
		return nil, fmt.Errorf("%w: at least one question is required", errInputRequestInvalid)
	}
	questions := make([]InputQuestion, 0, len(args.Questions))
	for questionIndex, source := range args.Questions {
		questionText := strings.TrimSpace(source.Question)
		if questionText == "" {
			return nil, fmt.Errorf("%w: question %d is empty", errInputRequestInvalid, questionIndex+1)
		}
		if len(source.Options) < 2 || len(source.Options) > maxInputRequestOptions {
			return nil, fmt.Errorf("%w: question %d requires two to %d options", errInputRequestInvalid, questionIndex+1, maxInputRequestOptions)
		}
		questionID := fmt.Sprintf("q%d", questionIndex+1)
		options := make([]InputOption, 0, len(source.Options))
		for optionIndex, sourceOption := range source.Options {
			label := strings.TrimSpace(sourceOption.Label)
			if label == "" {
				return nil, fmt.Errorf("%w: question %d option %d is empty", errInputRequestInvalid, questionIndex+1, optionIndex+1)
			}
			options = append(options, InputOption{
				ID:          fmt.Sprintf("%s-o%d", questionID, optionIndex+1),
				Label:       label,
				Description: strings.TrimSpace(sourceOption.Description),
				Recommended: sourceOption.Recommended,
			})
		}
		questions = append(questions, InputQuestion{
			ID: questionID, Question: questionText, Options: options, AllowOther: source.AllowOther,
		})
	}
	now := nowString()
	return &InputRequest{
		ID: "input-" + uuid.NewString(), RunID: runID, AgentID: strings.TrimSpace(agentID),
		FunctionCallID: functionCallID, Title: strings.TrimSpace(args.Title), Status: InputRequestStatusPending,
		Questions: questions, Answers: []InputAnswer{}, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func requestUserToolArgsFromCall(call *genai.FunctionCall) (requestUserToolArgs, error) {
	if call == nil || call.Name != interactionRequestUserTool {
		return requestUserToolArgs{}, fmt.Errorf("%w: request-user call is missing", errInputRequestInvalid)
	}
	raw, err := json.Marshal(call.Args)
	if err != nil {
		return requestUserToolArgs{}, fmt.Errorf("%w: %v", errInputRequestInvalid, err)
	}
	var args requestUserToolArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return requestUserToolArgs{}, fmt.Errorf("%w: %v", errInputRequestInvalid, err)
	}
	return args, nil
}

func validateInputAnswers(request InputRequest, submitted []InputAnswer) ([]InputAnswer, error) {
	if len(submitted) != len(request.Questions) {
		return nil, fmt.Errorf("%w: every question must be answered", errInputRequestInvalid)
	}
	byQuestion := make(map[string]InputAnswer, len(submitted))
	for _, answer := range submitted {
		questionID := strings.TrimSpace(answer.QuestionID)
		if questionID == "" {
			return nil, fmt.Errorf("%w: questionId is required", errInputRequestInvalid)
		}
		if _, exists := byQuestion[questionID]; exists {
			return nil, fmt.Errorf("%w: duplicate answer for %s", errInputRequestInvalid, questionID)
		}
		answer.QuestionID = questionID
		answer.OptionID = strings.TrimSpace(answer.OptionID)
		answer.OtherText = strings.TrimSpace(answer.OtherText)
		byQuestion[questionID] = answer
	}
	canonical := make([]InputAnswer, 0, len(request.Questions))
	for _, question := range request.Questions {
		answer, ok := byQuestion[question.ID]
		if !ok {
			return nil, fmt.Errorf("%w: missing answer for %s", errInputRequestInvalid, question.ID)
		}
		usesOption := answer.OptionID != ""
		usesOther := answer.OtherText != ""
		if usesOption == usesOther {
			return nil, fmt.Errorf("%w: %s must use exactly one answer type", errInputRequestInvalid, question.ID)
		}
		if usesOther {
			if !question.AllowOther {
				return nil, fmt.Errorf("%w: %s does not allow other text", errInputRequestInvalid, question.ID)
			}
			canonical = append(canonical, InputAnswer{QuestionID: question.ID, OtherText: answer.OtherText})
			continue
		}
		validOption := false
		for _, option := range question.Options {
			if option.ID == answer.OptionID {
				validOption = true
				break
			}
		}
		if !validOption {
			return nil, fmt.Errorf("%w: invalid option for %s", errInputRequestInvalid, question.ID)
		}
		canonical = append(canonical, InputAnswer{QuestionID: question.ID, OptionID: answer.OptionID})
	}
	return canonical, nil
}

func inputAnswersEqual(left []InputAnswer, right []InputAnswer) bool {
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return string(leftJSON) == string(rightJSON)
}

func inputResponsePayload(request InputRequest) map[string]any {
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

func (r *Runtime) pendingInputRequests(ctx context.Context, execution *googleADKExecution) (map[string]*InputRequest, error) {
	if execution == nil {
		return nil, nil
	}
	response, err := execution.sessionService.Get(ctx, &adksession.GetRequest{
		AppName: execution.appName, UserID: googleADKUserID, SessionID: execution.sessionID,
	})
	if err != nil {
		return nil, err
	}
	requests := map[string]*InputRequest{}
	for event := range response.Session.Events().All() {
		if event.Content == nil {
			continue
		}
		for _, part := range event.Content.Parts {
			call := part.FunctionCall
			if call == nil || call.Name != interactionRequestUserTool || !sliceContainsExact(event.LongRunningToolIDs, call.ID) {
				continue
			}
			runID, tracked := execution.trackedRunIDForFunctionCall(call.ID)
			if !tracked {
				continue
			}
			stored, ok, err := r.store.Run(ctx, runID)
			if err != nil {
				return nil, err
			}
			if ok && runHasInputFunctionCall(stored, call.ID) {
				continue
			}
			if ok && stored.InputRequest != nil && stored.InputRequest.Status == InputRequestStatusPending {
				return nil, fmt.Errorf("%w: run %s already has a pending input request", errInputRequestConflict, runID)
			}
			if requests[runID] != nil {
				return nil, fmt.Errorf("%w: simultaneous input requests are not supported for run %s", errInputRequestConflict, runID)
			}
			args, err := requestUserToolArgsFromCall(call)
			if err != nil {
				continue
			}
			agentID := execution.agent.ID
			if ok {
				agentID = stored.AgentID
			}
			request, err := buildInputRequest(runID, agentID, call.ID, args)
			if err != nil {
				continue
			}
			requests[runID] = request
			execution.markCallWaitingForInput(call.ID)
		}
	}
	return requests, nil
}

func runHasInputFunctionCall(run Run, functionCallID string) bool {
	for _, request := range run.InputRequests {
		if request.FunctionCallID == functionCallID {
			return true
		}
	}
	return run.InputRequest != nil && run.InputRequest.FunctionCallID == functionCallID
}

func sliceContainsExact(values []string, target string) bool {
	return slices.Contains(values, target)
}

func InputRequestErrorKind(err error) string {
	switch {
	case errors.Is(err, errInputRequestInvalid):
		return "invalid"
	case errors.Is(err, errInputRequestNotFound):
		return "not_found"
	case errors.Is(err, errInputRequestAlreadyAnswered), errors.Is(err, errInputRequestConflict):
		return "conflict"
	default:
		return "internal"
	}
}
