package skillsruntime

import (
	"reflect"
	"slices"
	"testing"
)

func TestWorkflowSchemasStayStrict(t *testing.T) {
	names := []string{
		"workflows.list", "workflows.get", "workflows.create", "workflows.update",
		"workflow_triggers.list", "workflow_triggers.create", "workflow_triggers.update",
		"workflow_runs.list", "workflow_runs.get",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			schema := DefaultToolInputSchema(name)
			if schema["type"] != "object" || schema["additionalProperties"] != false {
				t.Fatalf("schema = %#v, want strict object", schema)
			}
		})
	}
	assertRequired(t, "workflows.create", "name", "agentId", "promptTemplate")
	assertRequired(t, "workflows.update", "workflowId")
	assertRequired(t, "workflow_triggers.update", "workflowId", "triggerId")
	assertRequired(t, "workflow_runs.get", "logId")
}

func TestToolMetadataNormalization(t *testing.T) {
	if got := NormalizeToolAlias(" @JFTrade  market//snapshot::latest "); got != "market.snapshot.latest" {
		t.Fatalf("NormalizeToolAlias = %q", got)
	}
	if got := DefaultToolRiskLevelForTool("tasks.create", "write_task"); got != "low" {
		t.Fatalf("task risk = %q", got)
	}
	if got := DefaultToolRiskLevel("live_trading"); got != "critical" {
		t.Fatalf("live trading risk = %q", got)
	}
	input := map[string]any{"string": "42", "float": 12.8}
	if got := StringValue(input, "string"); got != "42" {
		t.Fatalf("StringValue = %q", got)
	}
	if got := IntValue(input, "float", 0); got != 12 {
		t.Fatalf("IntValue = %d", got)
	}
}

func TestBacktestAndStrategyLifecycleSchemasCloseNestedObjects(t *testing.T) {
	research := DefaultToolInputSchema("strategy.research_backtest")["properties"].(map[string]any)
	tradingCosts := research["tradingCosts"].(map[string]any)["properties"].(map[string]any)
	brokerFees := tradingCosts["brokerFees"].(map[string]any)
	rules := brokerFees["properties"].(map[string]any)["rules"].(map[string]any)
	feeRule := rules["items"].(map[string]any)
	if feeRule["additionalProperties"] != false {
		t.Fatalf("fee rule schema is open: %#v", feeRule)
	}
	if got := feeRule["required"]; !reflect.DeepEqual(got, []string{"id", "label", "category", "basis"}) {
		t.Fatalf("fee rule required = %#v", got)
	}

	riskInput := DefaultToolInputSchema("strategy.instance_risk.update")["properties"].(map[string]any)
	risk := riskInput["risk"].(map[string]any)
	if risk["additionalProperties"] != false || !reflect.DeepEqual(risk["required"], []string{"mode"}) {
		t.Fatalf("runtime risk schema = %#v", risk)
	}
	riskProperties := risk["properties"].(map[string]any)
	if got := riskProperties["mode"].(map[string]any)["enum"]; !reflect.DeepEqual(got, []string{"off", "monitor", "enforce"}) {
		t.Fatalf("risk mode enum = %#v", got)
	}
	if riskProperties["maxOrderQuantity"].(map[string]any)["exclusiveMinimum"] != 0 || riskProperties["dailyMaxOrders"].(map[string]any)["minimum"] != 1 {
		t.Fatalf("risk bounds = %#v", riskProperties)
	}

	instantiate := DefaultToolInputSchema("strategy.instantiate")["properties"].(map[string]any)
	binding := instantiate["binding"].(map[string]any)["properties"].(map[string]any)
	if binding["runtimeRisk"].(map[string]any)["additionalProperties"] != false {
		t.Fatalf("instantiate runtime risk schema is open: %#v", binding["runtimeRisk"])
	}
	instrument := binding["instruments"].(map[string]any)["items"].(map[string]any)
	if !reflect.DeepEqual(instrument["required"], []string{"market", "code"}) {
		t.Fatalf("binding instrument required = %#v", instrument["required"])
	}

	resultView := DefaultToolInputSchema("backtest.result_view")["properties"].(map[string]any)
	views := resultView["view"].(map[string]any)["enum"].([]string)
	if !slices.Contains(views, "warnings") {
		t.Fatalf("backtest result views = %#v, missing warnings", views)
	}
}

func assertRequired(t *testing.T, name string, fields ...string) {
	t.Helper()
	required, ok := DefaultToolInputSchema(name)["required"].([]string)
	if !ok {
		t.Fatalf("%s required is not []string", name)
	}
	for _, field := range fields {
		if !slices.Contains(required, field) {
			t.Fatalf("%s required = %#v, missing %q", name, required, field)
		}
	}
}
