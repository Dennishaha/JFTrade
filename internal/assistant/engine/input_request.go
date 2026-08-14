package adk

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
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
	inputDecisionMissingRequiredContext = "missing_required_context"
	inputDecisionMaterialTradeoff       = "material_tradeoff"
	inputDecisionScopeBoundary          = "scope_boundary"
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
	DecisionKind   string                    `json:"decisionKind"`
	BlockingReason string                    `json:"blockingReason"`
	Title          string                    `json:"title,omitempty"`
	Questions      []requestUserToolQuestion `json:"questions"`
}

func inputRequestToolDescriptor() ToolDescriptor {
	return ToolDescriptor{
		Name:         interactionRequestUserTool,
		DisplayName:  "向用户提问",
		Description:  "仅当缺少用户独有的必要信息、存在无法合并的重大取舍，或继续会越过权限/范围边界时，一次性提交所有阻塞问题。禁止询问可选下一步、是否继续、先看哪部分，或用该工具代替写操作审批。每题必须提供 2 到 3 个选项；可接受自由回答时设置 allowOther。",
		Category:     "interaction",
		Permission:   "read_internal",
		RiskLevel:    "low",
		AllowedModes: jfadkmodel.AllPermissionModes(),
		InputSchema:  inputRequestToolInputSchema(),
	}
}

func inputRequestToolInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"decisionKind": map[string]any{
				"type":        "string",
				"enum":        []string{inputDecisionMissingRequiredContext, inputDecisionMaterialTradeoff, inputDecisionScopeBoundary},
				"description": "The genuine blocking boundary. Optional next steps and whether to continue are not blocking decisions.",
			},
			"blockingReason": map[string]any{
				"type":        "string",
				"minLength":   1,
				"description": "Why the original task cannot safely continue without this user answer. Do not use workload reduction as a reason.",
			},
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
		"required": []string{"decisionKind", "blockingReason", "questions"},
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
		Description: "Ask only for genuinely blocking decisions that cannot be inferred or retrieved: missing user-only context, " +
			"an irreconcilable material tradeoff, or a permission/scope boundary. Never ask about an optional next step, whether to continue, " +
			"or which part to see first, and never substitute this tool for write approval. Collect every required decision in one call. " +
			"Each question must offer two or three options; set allowOther when a free-form alternative is acceptable.",
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
	decisionKind := strings.TrimSpace(args.DecisionKind)
	if !validInputDecisionKind(decisionKind) {
		return nil, fmt.Errorf("%w: decisionKind must describe a supported blocking boundary", errInputRequestInvalid)
	}
	blockingReason := strings.TrimSpace(args.BlockingReason)
	if blockingReason == "" {
		return nil, fmt.Errorf("%w: blockingReason is required", errInputRequestInvalid)
	}
	if isNonBlockingOptionalPrompt(blockingReason) {
		return nil, fmt.Errorf("%w: blockingReason describes a non-blocking optional next step", errInputRequestInvalid)
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
		if isNonBlockingOptionalPrompt(questionText) {
			return nil, fmt.Errorf("%w: question %d asks about a non-blocking optional next step", errInputRequestInvalid, questionIndex+1)
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

func validInputDecisionKind(value string) bool {
	switch value {
	case inputDecisionMissingRequiredContext, inputDecisionMaterialTradeoff, inputDecisionScopeBoundary:
		return true
	default:
		return false
	}
}

func isNonBlockingOptionalPrompt(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, phrase := range []string{
		"optional next step", "whether to continue", "do you want me to continue", "would you like me to continue",
		"if you want, i can", "which part would you like", "what would you like to see first",
		"是否继续", "要不要继续", "需要我继续", "如果需要我可以", "你想先做哪项", "你更想看哪部分", "先看哪部分",
	} {
		if strings.Contains(value, phrase) {
			return true
		}
	}
	return false
}

func requestUserToolArgsFromCall(call *genai.FunctionCall) (requestUserToolArgs, error) {
	if call == nil || call.Name != interactionRequestUserTool {
		return requestUserToolArgs{}, fmt.Errorf("%w: request-user call is missing", errInputRequestInvalid)
	}
	raw, err := json.Marshal(call.Args)
	if err != nil {
		return requestUserToolArgs{}, fmt.Errorf("%w: %w", errInputRequestInvalid, err)
	}
	var args requestUserToolArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return requestUserToolArgs{}, fmt.Errorf("%w: %w", errInputRequestInvalid, err)
	}
	return args, nil
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

func (r *Runtime) PendingInputRequests(ctx context.Context, execution WorkflowExecutionHandle) (map[string]*InputRequest, error) {
	if execution == nil {
		return nil, nil
	}
	response, err := execution.SessionService().Get(ctx, &adksession.GetRequest{
		AppName: execution.AppName(), UserID: googleADKUserID, SessionID: execution.SessionID(),
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
			runID, tracked := execution.TrackedRunIDForFunctionCall(call.ID)
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
			agentID := execution.AgentDefinition().ID
			if ok {
				agentID = stored.AgentID
			}
			request, err := buildInputRequest(runID, agentID, call.ID, args)
			if err != nil {
				continue
			}
			requests[runID] = request
			execution.MarkCallWaitingForInput(call.ID)
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
	return jfadkmodel.InputRequestErrorKind(err)
}
