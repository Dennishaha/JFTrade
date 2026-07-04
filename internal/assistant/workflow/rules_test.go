package workflow

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestNextScheduleRunUsesFiveFieldCronAndTimezone(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	config := map[string]any{"cron": "0 8 * * 1-5", "timezone": "Asia/Shanghai"}

	next, err := NextScheduleRun(config, time.Date(2026, 7, 1, 7, 59, 0, 0, location).UTC())
	if err != nil {
		t.Fatalf("NextScheduleRun weekday: %v", err)
	}
	if want := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC); !next.Equal(want) {
		t.Fatalf("next weekday run = %s, want %s", next, want)
	}

	next, err = NextScheduleRun(config, time.Date(2026, 7, 3, 8, 1, 0, 0, location).UTC())
	if err != nil {
		t.Fatalf("NextScheduleRun weekend rollover: %v", err)
	}
	if want := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC); !next.Equal(want) {
		t.Fatalf("next rollover run = %s, want %s", next, want)
	}

	if _, err := NextScheduleRun(map[string]any{"cron": "0 0 8 * * 1-5"}, time.Now()); err == nil {
		t.Fatal("NextScheduleRun accepted six-field cron, want error")
	}
}

func TestEvaluateMarketThresholdTriggerEdgesAndCooldown(t *testing.T) {
	now := time.Date(2026, 7, 1, 1, 0, 0, 0, time.UTC)
	trigger := jfadk.WorkflowTrigger{
		Config: map[string]any{
			"instrumentIds": []string{"US.AAPL"},
			"snapshotPath":  "snapshot.price",
			"value":         100,
			"edge":          "cross_up",
			"cooldownSec":   900,
		},
	}

	matches, changed := EvaluateMarketThresholdTrigger(trigger, []map[string]any{marketThresholdEvent("US.AAPL", 99)}, now)
	if len(matches) != 0 || !changed {
		t.Fatalf("first below-threshold event matches=%+v changed=%v, want no match with state change", matches, changed)
	}
	matches, changed = EvaluateMarketThresholdTrigger(trigger, []map[string]any{marketThresholdEvent("US.AAPL", 101)}, now.Add(time.Second))
	if len(matches) != 1 || !changed {
		t.Fatalf("cross-up event matches=%+v changed=%v, want one match", matches, changed)
	}
	threshold, ok := matches[0]["threshold"].(map[string]any)
	if !ok || threshold["instrumentId"] != "US.AAPL" || threshold["edge"] != "cross_up" {
		t.Fatalf("matched threshold payload = %+v", matches[0]["threshold"])
	}

	above := jfadk.WorkflowTrigger{
		Config: map[string]any{
			"instrumentIds": []string{"US.AAPL"},
			"snapshotPath":  "snapshot.price",
			"operator":      ">",
			"value":         100,
			"edge":          "above",
			"cooldownSec":   900,
		},
	}
	matches, _ = EvaluateMarketThresholdTrigger(above, []map[string]any{marketThresholdEvent("US.AAPL", 101)}, now)
	if len(matches) != 1 {
		t.Fatalf("above threshold first match = %+v, want one", matches)
	}
	matches, _ = EvaluateMarketThresholdTrigger(above, []map[string]any{marketThresholdEvent("US.AAPL", 102)}, now.Add(time.Second))
	if len(matches) != 0 {
		t.Fatalf("above threshold cooldown match = %+v, want none", matches)
	}
	matches, _ = EvaluateMarketThresholdTrigger(above, []map[string]any{marketThresholdEvent("US.AAPL", 103)}, now.Add(901*time.Second))
	if len(matches) != 1 {
		t.Fatalf("above threshold after cooldown match = %+v, want one", matches)
	}
}

func TestEventRulesNormalizeAndValidate(t *testing.T) {
	event := jfadk.WorkflowEvent{
		Type:     "risk.alert",
		Source:   "strategy",
		EntityID: "runtime-1",
		Payload:  map[string]any{"category": "risk", "level": "warning"},
	}
	if !EventMatches(map[string]any{"source": "strategy", "eventType": "risk.alert", "category": "risk"}, event) {
		t.Fatal("EventMatches = false, want true")
	}
	if EventMatches(map[string]any{"level": "error"}, event) {
		t.Fatal("EventMatches mismatched level = true, want false")
	}

	now := time.Date(2026, 7, 1, 1, 0, 0, 0, time.UTC)
	trigger := jfadk.WorkflowTrigger{Config: map[string]any{"cooldownSec": 600}}
	if !EventCooldownAllows(&trigger, now) {
		t.Fatal("first EventCooldownAllows = false, want true")
	}
	if EventCooldownAllows(&trigger, now.Add(time.Second)) {
		t.Fatal("second EventCooldownAllows during cooldown = true, want false")
	}
	if !EventCooldownAllows(&trigger, now.Add(601*time.Second)) {
		t.Fatal("EventCooldownAllows after cooldown = false, want true")
	}

	if NormalizeWorkflowStatus("", jfadk.WorkflowStatusDisabled) != jfadk.WorkflowStatusDisabled {
		t.Fatal("NormalizeWorkflowStatus fallback disabled failed")
	}
	if NormalizeTriggerStatus("", jfadk.WorkflowTriggerStatusError) != jfadk.WorkflowTriggerStatusError {
		t.Fatal("NormalizeTriggerStatus fallback error failed")
	}
	if NormalizeWorkflowWorkMode("", jfadk.WorkModeChat) != jfadk.WorkModeChat {
		t.Fatal("NormalizeWorkflowWorkMode fallback chat failed")
	}
	if NormalizeWorkflowPermissionMode("bad", "") != jfadk.PermissionModeApproval {
		t.Fatal("NormalizeWorkflowPermissionMode bad value did not fall back to approval")
	}
	if NormalizeTriggerType("", jfadk.WorkflowTriggerTypeWebhook) != jfadk.WorkflowTriggerTypeWebhook {
		t.Fatal("NormalizeTriggerType fallback webhook failed")
	}
	if DefaultTriggerTitle(jfadk.WorkflowTriggerTypeEvent) != "事件触发" || DefaultTriggerTitle(jfadk.WorkflowTriggerTypeMarketThreshold) != "行情阈值" {
		t.Fatal("DefaultTriggerTitle event/market mismatch")
	}
}

func TestRuleHelpersAndValidationEdges(t *testing.T) {
	now := time.Date(2026, 7, 1, 1, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		trigger jfadk.WorkflowTrigger
		want    string
	}{
		{trigger: jfadk.WorkflowTrigger{Type: jfadk.WorkflowTriggerTypeManual}, want: "workflowId"},
		{trigger: jfadk.WorkflowTrigger{WorkflowID: "wf", Type: jfadk.WorkflowTriggerTypeSchedule, Config: map[string]any{}}, want: "cron"},
		{trigger: jfadk.WorkflowTrigger{WorkflowID: "wf", Type: jfadk.WorkflowTriggerTypeMarketThreshold, Config: map[string]any{"value": 1}}, want: "instrumentIds"},
		{trigger: jfadk.WorkflowTrigger{WorkflowID: "wf", Type: jfadk.WorkflowTriggerTypeMarketThreshold, Config: map[string]any{"instrumentIds": []string{"US.AAPL"}}}, want: "numeric value"},
	} {
		if err := ValidateTrigger(test.trigger); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("ValidateTrigger err = %v, want containing %q", err, test.want)
		}
	}
	if _, err := NextScheduleRun(map[string]any{"cron": "0 8 * * 1-5", "timezone": "Mars/Base"}, now); err == nil {
		t.Fatal("NextScheduleRun invalid timezone succeeded, want error")
	}
	if NextRunAtString(map[string]any{"cron": "bad"}, now) != "" {
		t.Fatal("NextRunAtString invalid cron returned non-empty")
	}

	if ThresholdFired("below", "<=", 0, false, 100, 100) != true {
		t.Fatal("ThresholdFired below <= failed")
	}
	if ThresholdFired("above", ">=", 0, false, 100, 100) != true {
		t.Fatal("ThresholdFired above >= failed")
	}
	if CompareThreshold("<", 99, 100) != true || CompareThreshold(">", 99, 100) != false {
		t.Fatal("CompareThreshold produced unexpected comparison result")
	}
	if _, ok := NumericAtPath(map[string]any{"snapshot": "bad"}, "snapshot.price"); ok {
		t.Fatal("NumericAtPath through scalar = true, want false")
	}
	if value, ok := NumericAtPath(map[string]any{"snapshot": map[string]any{"price": 101}}, "snapshot..price"); !ok || value != 101 {
		t.Fatalf("NumericAtPath empty segment value=%v ok=%v", value, ok)
	}
	if got := EventInstrumentID(map[string]any{"payload": map[string]any{"entityId": "hk.00700"}}); got != "HK.00700" {
		t.Fatalf("EventInstrumentID nested payload = %q", got)
	}
	if got := ConfigStringSlice(map[string]any{"ids": []any{" US.AAPL ", 700, ""}}, "ids"); strings.Join(got, ",") != "US.AAPL,700" {
		t.Fatalf("ConfigStringSlice []any = %+v", got)
	}
	if got := ConfigStringSlice(map[string]any{"ids": []string{"a", "a", " "}}, "ids"); strings.Join(got, ",") != "a" {
		t.Fatalf("ConfigStringSlice []string = %+v", got)
	}
	if got := ConfigStringSlice(map[string]any{}, "ids"); got != nil {
		t.Fatalf("ConfigStringSlice missing = %+v, want nil", got)
	}
	for _, value := range []any{float32(1), int64(2), int32(3), json.Number("4"), "5"} {
		if _, ok := AnyFloat(value); !ok {
			t.Fatalf("AnyFloat(%T) = false, want true", value)
		}
	}
	if _, ok := AnyFloat("bad"); ok {
		t.Fatal("AnyFloat bad string = true, want false")
	}
	if state := EnsureConfigState(nil); len(state) != 0 {
		t.Fatalf("EnsureConfigState nil = %+v, want empty detached state", state)
	}
	if !CooldownAllows("bad timestamp", now, 60) {
		t.Fatal("CooldownAllows malformed timestamp = false, want permissive true")
	}
	if CooldownAllows(now.Add(-30*time.Second).Format(time.RFC3339Nano), now, 60) {
		t.Fatal("CooldownAllows recent timestamp = true, want false")
	}
}

type workflowTestStringer string

func (value workflowTestStringer) String() string { return string(value) }

func TestRuleFallbacksMismatchesAndValidVariants(t *testing.T) {
	now := time.Date(2026, 7, 1, 1, 0, 0, 0, time.UTC)
	if _, err := NextScheduleRun(map[string]any{}, now); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("missing cron error = %v", err)
	}
	if _, err := NextScheduleRun(map[string]any{"cron": "x x x x x"}, now); err == nil {
		t.Fatal("invalid five-field cron should fail parsing")
	}
	if next := NextRunAtString(map[string]any{"cron": "0 8 * * *"}, now); next == "" {
		t.Fatal("default-timezone schedule returned empty next run")
	}

	if matches, changed := EvaluateMarketThresholdTrigger(jfadk.WorkflowTrigger{}, nil, now); matches != nil || changed {
		t.Fatalf("empty threshold trigger = %#v, %v", matches, changed)
	}
	missingValue := jfadk.WorkflowTrigger{Config: map[string]any{"instrumentIds": []string{"US.AAPL"}}}
	if matches, changed := EvaluateMarketThresholdTrigger(missingValue, nil, now); matches != nil || changed {
		t.Fatalf("missing threshold value = %#v, %v", matches, changed)
	}
	defaultEdge := jfadk.WorkflowTrigger{Config: map[string]any{"instrumentIds": []string{"US.AAPL"}, "value": 100}}
	if matches, changed := EvaluateMarketThresholdTrigger(defaultEdge, nil, now); len(matches) != 0 || changed {
		t.Fatalf("default-edge empty events = %#v, %v", matches, changed)
	}
	crossDown := jfadk.WorkflowTrigger{Config: map[string]any{
		"instrumentIds": []string{"US.AAPL"},
		"value":         100,
		"edge":          "cross_down",
		"cooldownSec":   0,
	}}
	events := []map[string]any{
		{},
		marketThresholdEvent("US.MSFT", 101),
		{"entityId": "US.AAPL", "snapshot": map[string]any{"price": "bad"}},
		{"entityId": "US.AAPL", "payload": map[string]any{"snapshot": map[string]any{"price": 101}}},
	}
	if matches, changed := EvaluateMarketThresholdTrigger(crossDown, events, now); len(matches) != 0 || !changed {
		t.Fatalf("cross-down priming = %#v, %v", matches, changed)
	}
	if matches, changed := EvaluateMarketThresholdTrigger(crossDown, []map[string]any{{
		"instrument": map[string]any{"instrumentId": "us.aapl"},
		"snapshot":   map[string]any{"price": 99},
	}}, now.Add(time.Second)); len(matches) != 1 || !changed {
		t.Fatalf("cross-down firing = %#v, %v", matches, changed)
	}

	event := jfadk.WorkflowEvent{Type: "risk.alert", Source: "strategy", EntityID: "instance-1", Payload: map[string]any{"category": "risk", "level": "warn"}}
	for _, config := range []map[string]any{
		{"source": "broker"},
		{"eventType": "order.fill"},
		{"entityId": "instance-2"},
		{"category": "execution"},
	} {
		if EventMatches(config, event) {
			t.Fatalf("EventMatches(%#v) = true", config)
		}
	}
	if EventCooldownAllows(nil, now) {
		t.Fatal("nil trigger cooldown should be rejected")
	}
	if !ThresholdFired("above", "", 0, false, 101, 100) {
		t.Fatal("above threshold should use the default greater-than operator")
	}

	validTriggers := []jfadk.WorkflowTrigger{
		{WorkflowID: "wf", Type: jfadk.WorkflowTriggerTypeSchedule, Config: map[string]any{"cron": "0 8 * * *"}},
		{WorkflowID: "wf", Type: jfadk.WorkflowTriggerTypeManual},
		{WorkflowID: "wf", Type: jfadk.WorkflowTriggerTypeWebhook},
		{WorkflowID: "wf", Type: jfadk.WorkflowTriggerTypeEvent},
		{WorkflowID: "wf", Type: jfadk.WorkflowTriggerTypeMarketThreshold, Config: map[string]any{"instrumentIds": "US.AAPL", "value": 100}},
	}
	for _, trigger := range validTriggers {
		if err := ValidateTrigger(trigger); err != nil {
			t.Fatalf("ValidateTrigger(%s): %v", trigger.Type, err)
		}
	}
	if err := ValidateTrigger(jfadk.WorkflowTrigger{WorkflowID: "wf", Type: "unsupported"}); err == nil {
		t.Fatal("unsupported trigger type should fail")
	}

	if NormalizeWorkflowStatus("disabled", "") != jfadk.WorkflowStatusDisabled || NormalizeWorkflowStatus("unknown", "") != jfadk.WorkflowStatusEnabled {
		t.Fatal("workflow status variants mismatch")
	}
	if NormalizeTriggerStatus("disabled", "") != jfadk.WorkflowTriggerStatusDisabled || NormalizeTriggerStatus("unknown", "") != jfadk.WorkflowTriggerStatusEnabled {
		t.Fatal("trigger status variants mismatch")
	}
	if NormalizeWorkflowWorkMode("loop", "") != jfadk.WorkModeLoop || NormalizeWorkflowWorkMode("unknown", "") != jfadk.WorkModeTask {
		t.Fatal("work mode variants mismatch")
	}
	if NormalizeWorkflowPermissionMode("all", "") != jfadk.PermissionModeAll || NormalizeWorkflowPermissionMode("", "") != "" {
		t.Fatal("permission mode variants mismatch")
	}
	if NormalizeTriggerType("schedule", "") != jfadk.WorkflowTriggerTypeSchedule || NormalizeTriggerType("unknown", "") != jfadk.WorkflowTriggerTypeManual {
		t.Fatal("trigger type variants mismatch")
	}
	for triggerType, want := range map[string]string{
		jfadk.WorkflowTriggerTypeSchedule: "定时触发",
		jfadk.WorkflowTriggerTypeWebhook:  "Webhook",
		jfadk.WorkflowTriggerTypeManual:   "手动触发",
	} {
		if got := DefaultTriggerTitle(triggerType); got != want {
			t.Fatalf("DefaultTriggerTitle(%q) = %q", triggerType, got)
		}
	}

	if got := ConfigString(map[string]any{"value": workflowTestStringer("stringer")}, "value"); got != "stringer" {
		t.Fatalf("ConfigString Stringer = %q", got)
	}
	if got := ConfigStringSlice(map[string]any{"ids": " A, B, A "}, "ids"); strings.Join(got, ",") != "A,B" {
		t.Fatalf("ConfigStringSlice string = %#v", got)
	}
	if got := ConfigStringSlice(map[string]any{"ids": 42}, "ids"); got != nil {
		t.Fatalf("ConfigStringSlice scalar = %#v", got)
	}
	if ConfigInt(map[string]any{"count": 1.6}, "count", 0) != 2 || ConfigInt(nil, "count", 7) != 7 {
		t.Fatal("ConfigInt variants mismatch")
	}
	if EventInstrumentID(map[string]any{"instrumentId": "hk.00700"}) != "HK.00700" || EventInstrumentID(map[string]any{}) != "" {
		t.Fatal("EventInstrumentID variants mismatch")
	}
	state := map[string]any{"existing": true}
	config := map[string]any{"state": state}
	if EnsureConfigState(config)["existing"] != true {
		t.Fatal("EnsureConfigState did not reuse existing state")
	}
	if !CooldownAllows(now, now, 0) || !CooldownAllows(nil, now, 60) || !CooldownAllows(now.Add(-time.Minute).Format(time.RFC3339), now, 60) {
		t.Fatal("CooldownAllows fallback variants mismatch")
	}
	if MapString(nil, "value") != "" || MapString(map[string]any{"value": " x "}, "value") != "x" {
		t.Fatal("MapString variants mismatch")
	}
	if len(cloneMap(nil)) != 0 || defaultString(" value ", "fallback") != "value" {
		t.Fatal("map/default helper variants mismatch")
	}
}

func marketThresholdEvent(instrumentID string, price float64) map[string]any {
	return map[string]any{
		"type":     "market-data.tick",
		"entityId": instrumentID,
		"snapshot": map[string]any{"price": price},
	}
}
