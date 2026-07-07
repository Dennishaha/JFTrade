package workflow

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

const defaultMarketThresholdCooldown = 900

func NextScheduleRun(config map[string]any, from time.Time) (time.Time, error) {
	cronExpression := strings.TrimSpace(ConfigString(config, "cron"))
	if cronExpression == "" {
		return time.Time{}, fmt.Errorf("schedule cron is required")
	}
	if len(strings.Fields(cronExpression)) != 5 {
		return time.Time{}, fmt.Errorf("schedule cron must contain 5 fields")
	}
	timezone := ConfigString(config, "timezone")
	if strings.TrimSpace(timezone) == "" {
		timezone = "Asia/Shanghai"
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid schedule timezone: %w", err)
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(cronExpression)
	if err != nil {
		return time.Time{}, err
	}
	return schedule.Next(from.In(location)).UTC(), nil
}

func NextRunAtString(config map[string]any, from time.Time) string {
	next, err := NextScheduleRun(config, from)
	if err != nil {
		return ""
	}
	return next.Format(time.RFC3339Nano)
}

func EvaluateMarketThresholdTrigger(trigger jfadk.WorkflowTrigger, events []map[string]any, now time.Time) ([]map[string]any, bool) {
	instruments := map[string]struct{}{}
	for _, instrumentID := range ConfigStringSlice(trigger.Config, "instrumentIds") {
		instruments[strings.ToUpper(strings.TrimSpace(instrumentID))] = struct{}{}
	}
	if len(instruments) == 0 {
		return nil, false
	}
	path := ConfigString(trigger.Config, "snapshotPath")
	if strings.TrimSpace(path) == "" {
		path = "snapshot.price"
	}
	threshold, ok := ConfigFloat(trigger.Config, "value")
	if !ok {
		return nil, false
	}
	edge := strings.ToLower(strings.TrimSpace(ConfigString(trigger.Config, "edge")))
	if edge == "" {
		edge = "cross_up"
	}
	operator := strings.TrimSpace(ConfigString(trigger.Config, "operator"))
	cooldownSec := ConfigInt(trigger.Config, "cooldownSec", defaultMarketThresholdCooldown)
	state := EnsureConfigState(trigger.Config)
	lastValues := ensureStateMap(state, "lastValues")
	lastTriggeredAt := ensureStateMap(state, "lastTriggeredAt")
	matches := []map[string]any{}
	changed := false
	for _, event := range events {
		instrumentID := EventInstrumentID(event)
		if instrumentID == "" {
			continue
		}
		if _, ok := instruments[instrumentID]; !ok {
			continue
		}
		current, ok := NumericAtPath(event, path)
		if !ok {
			if payload, payloadOK := event["payload"].(map[string]any); payloadOK {
				current, ok = NumericAtPath(payload, path)
			}
		}
		if !ok {
			continue
		}
		previous, hadPrevious := AnyFloat(lastValues[instrumentID])
		fired := ThresholdFired(edge, operator, previous, hadPrevious, current, threshold)
		lastValues[instrumentID] = current
		changed = true
		if !fired || !CooldownAllows(lastTriggeredAt[instrumentID], now, cooldownSec) {
			continue
		}
		lastTriggeredAt[instrumentID] = now.Format(time.RFC3339Nano)
		matched := cloneMap(event)
		matched["threshold"] = map[string]any{
			"instrumentId": instrumentID,
			"path":         path,
			"edge":         edge,
			"operator":     operator,
			"value":        threshold,
			"previous":     previous,
			"current":      current,
		}
		matches = append(matches, matched)
		changed = true
	}
	return matches, changed
}

func EventMatches(config map[string]any, event jfadk.WorkflowEvent) bool {
	if expected := strings.TrimSpace(ConfigString(config, "source")); expected != "" && expected != event.Source {
		return false
	}
	if expected := strings.TrimSpace(ConfigString(config, "eventType")); expected != "" && expected != event.Type {
		return false
	}
	if expected := strings.TrimSpace(ConfigString(config, "entityId")); expected != "" && expected != event.EntityID {
		return false
	}
	if expected := strings.TrimSpace(ConfigString(config, "category")); expected != "" && expected != MapString(event.Payload, "category") {
		return false
	}
	if expected := strings.TrimSpace(ConfigString(config, "level")); expected != "" && expected != MapString(event.Payload, "level") {
		return false
	}
	return true
}

func EventCooldownAllows(trigger *jfadk.WorkflowTrigger, now time.Time) bool {
	if trigger == nil {
		return false
	}
	cooldownSec := ConfigInt(trigger.Config, "cooldownSec", 0)
	state := EnsureConfigState(trigger.Config)
	last := state["lastTriggeredAt"]
	if !CooldownAllows(last, now, cooldownSec) {
		return false
	}
	state["lastTriggeredAt"] = now.Format(time.RFC3339Nano)
	return true
}

func ThresholdFired(edge string, operator string, previous float64, hadPrevious bool, current float64, threshold float64) bool {
	switch edge {
	case "cross_down":
		return hadPrevious && previous >= threshold && current < threshold
	case "above":
		return CompareThreshold(defaultString(operator, ">"), current, threshold)
	case "below":
		return CompareThreshold(defaultString(operator, "<"), current, threshold)
	default:
		return hadPrevious && previous <= threshold && current > threshold
	}
}

func CompareThreshold(operator string, current float64, threshold float64) bool {
	switch strings.TrimSpace(operator) {
	case ">=":
		return current >= threshold
	case "<":
		return current < threshold
	case "<=":
		return current <= threshold
	default:
		return current > threshold
	}
}

func ValidateTrigger(trigger jfadk.WorkflowTrigger) error {
	if strings.TrimSpace(trigger.WorkflowID) == "" {
		return fmt.Errorf("workflowId is required")
	}
	switch trigger.Type {
	case jfadk.WorkflowTriggerTypeSchedule:
		_, err := NextScheduleRun(trigger.Config, time.Now().UTC())
		return err
	case jfadk.WorkflowTriggerTypeManual, jfadk.WorkflowTriggerTypeWebhook, jfadk.WorkflowTriggerTypeEvent:
		return nil
	case jfadk.WorkflowTriggerTypeMarketThreshold:
		if len(ConfigStringSlice(trigger.Config, "instrumentIds")) == 0 {
			return fmt.Errorf("market threshold trigger requires instrumentIds")
		}
		if _, ok := ConfigFloat(trigger.Config, "value"); !ok {
			return fmt.Errorf("market threshold trigger requires numeric value")
		}
		return nil
	default:
		return fmt.Errorf("unsupported workflow trigger type %q", trigger.Type)
	}
}

func NormalizeWorkflowStatus(input string, fallback string) string {
	status := strings.ToUpper(strings.TrimSpace(input))
	if status == "" {
		status = strings.ToUpper(strings.TrimSpace(fallback))
	}
	if status == jfadk.WorkflowStatusDisabled {
		return jfadk.WorkflowStatusDisabled
	}
	return jfadk.WorkflowStatusEnabled
}

func NormalizeTriggerStatus(input string, fallback string) string {
	status := strings.ToUpper(strings.TrimSpace(input))
	if status == "" {
		status = strings.ToUpper(strings.TrimSpace(fallback))
	}
	switch status {
	case jfadk.WorkflowTriggerStatusDisabled, jfadk.WorkflowTriggerStatusError:
		return status
	default:
		return jfadk.WorkflowTriggerStatusEnabled
	}
}

func NormalizeWorkflowWorkMode(input string, fallback string) string {
	mode := strings.ToLower(strings.TrimSpace(input))
	if mode == "" {
		mode = strings.ToLower(strings.TrimSpace(fallback))
	}
	switch mode {
	case jfadk.WorkModeChat, jfadk.WorkModeLoop:
		return mode
	default:
		return jfadk.WorkModeLoop
	}
}

func NormalizeWorkflowPermissionMode(input string, fallback string) string {
	mode := strings.ToLower(strings.TrimSpace(input))
	if mode == "" {
		mode = strings.ToLower(strings.TrimSpace(fallback))
	}
	switch mode {
	case "", jfadk.PermissionModeApproval, jfadk.PermissionModeLessApproval, jfadk.PermissionModeAll:
		return mode
	default:
		return jfadk.PermissionModeApproval
	}
}

func NormalizeTriggerType(input string, fallback string) string {
	value := strings.ToLower(strings.TrimSpace(input))
	if value == "" {
		value = strings.ToLower(strings.TrimSpace(fallback))
	}
	switch value {
	case jfadk.WorkflowTriggerTypeSchedule, jfadk.WorkflowTriggerTypeWebhook, jfadk.WorkflowTriggerTypeEvent, jfadk.WorkflowTriggerTypeMarketThreshold:
		return value
	default:
		return jfadk.WorkflowTriggerTypeManual
	}
}

func DefaultTriggerTitle(triggerType string) string {
	switch triggerType {
	case jfadk.WorkflowTriggerTypeSchedule:
		return "定时触发"
	case jfadk.WorkflowTriggerTypeWebhook:
		return "Webhook"
	case jfadk.WorkflowTriggerTypeEvent:
		return "事件触发"
	case jfadk.WorkflowTriggerTypeMarketThreshold:
		return "行情阈值"
	default:
		return "手动触发"
	}
}

func ConfigString(config map[string]any, key string) string {
	if value, ok := config[key]; ok {
		switch typed := value.(type) {
		case string:
			return typed
		case fmt.Stringer:
			return typed.String()
		}
	}
	return ""
}

func ConfigStringSlice(config map[string]any, key string) []string {
	value, ok := config[key]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return normalizeStringList(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, strings.TrimSpace(fmt.Sprint(item)))
		}
		return normalizeStringList(out)
	case string:
		parts := strings.Split(typed, ",")
		return normalizeStringList(parts)
	default:
		return nil
	}
}

func ConfigFloat(config map[string]any, key string) (float64, bool) {
	return AnyFloat(config[key])
}

func ConfigInt(config map[string]any, key string, fallback int) int {
	value, ok := AnyFloat(config[key])
	if !ok {
		return fallback
	}
	return int(math.Round(value))
}

func AnyFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func NumericAtPath(root map[string]any, path string) (float64, bool) {
	value := any(root)
	for part := range strings.SplitSeq(path, ".") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		current, ok := value.(map[string]any)
		if !ok {
			return 0, false
		}
		value = current[part]
	}
	return AnyFloat(value)
}

func EventInstrumentID(event map[string]any) string {
	for _, key := range []string{"entityId", "instrumentId"} {
		if value := strings.ToUpper(strings.TrimSpace(fmt.Sprint(event[key]))); value != "" && value != "<NIL>" {
			return value
		}
	}
	if instrument, ok := event["instrument"].(map[string]any); ok {
		if value := strings.ToUpper(strings.TrimSpace(fmt.Sprint(instrument["instrumentId"]))); value != "" && value != "<NIL>" {
			return value
		}
	}
	if payload, ok := event["payload"].(map[string]any); ok {
		return EventInstrumentID(payload)
	}
	return ""
}

func EnsureConfigState(config map[string]any) map[string]any {
	if config == nil {
		return map[string]any{}
	}
	if state, ok := config["state"].(map[string]any); ok {
		return state
	}
	state := map[string]any{}
	config["state"] = state
	return state
}

func CooldownAllows(value any, now time.Time, cooldownSec int) bool {
	if cooldownSec <= 0 {
		return true
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" {
		return true
	}
	previous, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		previous, err = time.Parse(time.RFC3339, text)
	}
	if err != nil {
		return true
	}
	return now.Sub(previous.UTC()) >= time.Duration(cooldownSec)*time.Second
}

func MapString(input map[string]any, key string) string {
	if input == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(input[key]))
}

func ensureStateMap(state map[string]any, key string) map[string]any {
	if current, ok := state[key].(map[string]any); ok {
		return current
	}
	out := map[string]any{}
	state[key] = out
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

func cloneMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	maps.Copy(out, input)
	return out
}

func defaultString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
