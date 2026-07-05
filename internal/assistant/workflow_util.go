package assistant

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"maps"
	"strings"
	"text/template"
	"time"

	workflowrules "github.com/jftrade/jftrade-main/internal/assistant/workflow"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func nextWorkflowScheduleRun(config map[string]any, from time.Time) (time.Time, error) {
	return workflowrules.NextScheduleRun(config, from)
}

func nextRunAtString(config map[string]any, from time.Time) string {
	return workflowrules.NextRunAtString(config, from)
}

func renderWorkflowTemplate(raw string, inputs map[string]any) (string, error) {
	tpl, err := template.New("workflow").Option("missingkey=zero").Parse(raw)
	if err != nil {
		return "", err
	}
	var buffer bytes.Buffer
	if err := tpl.Execute(&buffer, inputs); err != nil {
		return "", err
	}
	return strings.TrimSpace(buffer.String()), nil
}

func workflowInputs(workflow jfadk.WorkflowDefinition, trigger *jfadk.WorkflowTrigger, inputs map[string]any, matchedEvent map[string]any) map[string]any {
	merged := cloneMap(workflow.DefaultInputs)
	maps.Copy(merged, inputs)
	merged["workflow"] = map[string]any{"id": workflow.ID, "name": workflow.Name}
	if trigger != nil {
		merged["trigger"] = map[string]any{"id": trigger.ID, "type": trigger.Type, "title": trigger.Title}
	}
	if matchedEvent != nil {
		merged["event"] = matchedEvent
	}
	merged["now"] = time.Now().UTC().Format(time.RFC3339Nano)
	return merged
}

func evaluateMarketThresholdTrigger(trigger jfadk.WorkflowTrigger, events []map[string]any, now time.Time) ([]map[string]any, bool) {
	return workflowrules.EvaluateMarketThresholdTrigger(trigger, events, now)
}

func workflowEventMatches(config map[string]any, event jfadk.WorkflowEvent) bool {
	return workflowrules.EventMatches(config, event)
}

func eventTriggerCooldownAllows(trigger *jfadk.WorkflowTrigger, now time.Time) bool {
	return workflowrules.EventCooldownAllows(trigger, now)
}

func thresholdFired(edge string, operator string, previous float64, hadPrevious bool, current float64, threshold float64) bool {
	return workflowrules.ThresholdFired(edge, operator, previous, hadPrevious, current, threshold)
}

func compareThreshold(operator string, current float64, threshold float64) bool {
	return workflowrules.CompareThreshold(operator, current, threshold)
}

func validateWorkflowTrigger(trigger jfadk.WorkflowTrigger) error {
	return workflowrules.ValidateTrigger(trigger)
}

func normalizeWorkflowStatus(input string, fallback string) string {
	return workflowrules.NormalizeWorkflowStatus(input, fallback)
}

func normalizeTriggerStatus(input string, fallback string) string {
	return workflowrules.NormalizeTriggerStatus(input, fallback)
}

func normalizeWorkflowWorkMode(input string, fallback string) string {
	return workflowrules.NormalizeWorkflowWorkMode(input, fallback)
}

func normalizeWorkflowPermissionMode(input string, fallback string) string {
	return workflowrules.NormalizeWorkflowPermissionMode(input, fallback)
}

func normalizeTriggerType(input string, fallback string) string {
	return workflowrules.NormalizeTriggerType(input, fallback)
}

func defaultTriggerTitle(triggerType string) string {
	return workflowrules.DefaultTriggerTitle(triggerType)
}

func normalizedWorkflowPage(limit int, offset int) (int, int) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func sanitizeWorkflowTrigger(trigger jfadk.WorkflowTrigger) jfadk.WorkflowTrigger {
	trigger.HasSecret = trigger.HasSecret || strings.TrimSpace(trigger.SecretHash) != ""
	trigger.SecretHash = ""
	return trigger
}

func newSanitizedTrigger(trigger jfadk.WorkflowTrigger) *jfadk.WorkflowTrigger {
	sanitized := sanitizeWorkflowTrigger(trigger)
	return &sanitized
}

func newSanitizedTriggerPtr(trigger *jfadk.WorkflowTrigger) *jfadk.WorkflowTrigger {
	if trigger == nil {
		return nil
	}
	return newSanitizedTrigger(*trigger)
}

func triggerID(trigger *jfadk.WorkflowTrigger) string {
	if trigger == nil {
		return ""
	}
	return trigger.ID
}

func workflowSessionTitle(name string, now time.Time) string {
	if strings.TrimSpace(name) == "" {
		name = "ADK 工作流"
	}
	return strings.TrimSpace(name) + " - " + now.Format("2006-01-02 15:04")
}

func newWorkflowSecret() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "wfsec-" + base64.RawURLEncoding.EncodeToString(raw), nil
}

func hashWorkflowSecret(secret string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(secret)))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func verifyWorkflowSecret(secret string, hash string) bool {
	secret = strings.TrimSpace(secret)
	hash = strings.TrimSpace(hash)
	return secret != "" && hash != "" && hashWorkflowSecret(secret) == hash
}

func cloneMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	maps.Copy(out, input)
	return out
}

func normalizeStringList(values []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func configString(config map[string]any, key string) string {
	return workflowrules.ConfigString(config, key)
}

func configStringSlice(config map[string]any, key string) []string {
	return workflowrules.ConfigStringSlice(config, key)
}

func anyFloat(value any) (float64, bool) {
	return workflowrules.AnyFloat(value)
}

func numericAtPath(root map[string]any, path string) (float64, bool) {
	return workflowrules.NumericAtPath(root, path)
}

func eventInstrumentID(event map[string]any) string {
	return workflowrules.EventInstrumentID(event)
}

func ensureConfigState(config map[string]any) map[string]any {
	return workflowrules.EnsureConfigState(config)
}

func cooldownAllows(value any, now time.Time, cooldownSec int) bool {
	return workflowrules.CooldownAllows(value, now, cooldownSec)
}

func eventAsMap(event jfadk.WorkflowEvent) map[string]any {
	out := map[string]any{
		"type":     event.Type,
		"source":   event.Source,
		"entityId": event.EntityID,
		"at":       event.At,
		"payload":  cloneMap(event.Payload),
	}
	if event.ID != "" {
		out["id"] = event.ID
	}
	for key, value := range event.Payload {
		if _, exists := out[key]; !exists {
			out[key] = value
		}
	}
	return out
}

func mapString(input map[string]any, key string) string {
	return workflowrules.MapString(input, key)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func defaultString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func dailyStockReviewPrompt() string {
	return strings.TrimSpace(`请进行每日股票盘点。

关注列表：{{ .watchlist }}
市场范围：{{ .market }}

请使用可用工具读取行情快照、近期 K 线、组合摘要、风险状态和风险事件。输出：
1. 关注标的的关键变化和异常；
2. 持仓、订单和资金风险摘要；
3. 今日需要人工确认的待办事项；
4. 如果需要后续跟踪，请用 tasks.create 创建任务。

不要承诺收益，不要直接下单。涉及写入、策略保存或交易动作时必须保留审批。`)
}
