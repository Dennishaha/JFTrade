package adk

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

func (r *Runtime) registerModelCatalogTool() {
	if r == nil || r.tools == nil {
		return
	}
	r.tools.Register(ToolDescriptor{
		Name:               "models.list",
		DisplayName:        "查询可调用模型",
		Description:        "查询当前 ADK 已配置 Provider 暴露的可调用模型信息，供主智能体选择子智能体运行模型；不会返回 API Key。",
		Category:           "system",
		Permission:         "read_internal",
		AllowedModes:       allPermissionModes(),
		RequiresApprovalIn: nil,
		OutputSummary:      "Provider、模型、上下文窗口、能力和是否可调用。",
		RiskLevel:          "low",
	}, r.modelsListTool)
}

func (r *Runtime) modelsListTool(ctx context.Context, input map[string]any) (any, error) {
	if r == nil || r.store == nil {
		return nil, fmt.Errorf("adk runtime is unavailable")
	}
	providers, err := r.store.ListProviders(ctx)
	if err != nil {
		return nil, err
	}
	query := strings.ToLower(strings.TrimSpace(toolStringValue(input, "query")))
	providerID := strings.TrimSpace(toolStringValue(input, "providerId"))
	callableOnly := toolBoolValue(input, "callableOnly", true)
	limit := min(max(toolIntValue(input, "limit", 50), 1), 100)
	items := make([]map[string]any, 0, len(providers))
	for _, provider := range providers {
		if providerID != "" && provider.ID != providerID {
			continue
		}
		callable := provider.Enabled && provider.HasAPIKey
		if callableOnly && !callable {
			continue
		}
		haystack := strings.ToLower(strings.Join([]string{
			provider.ID,
			provider.DisplayName,
			provider.Model,
			provider.BaseURL,
			capabilityNames(provider.Capabilities),
		}, " "))
		if query != "" && !strings.Contains(haystack, query) {
			continue
		}
		items = append(items, map[string]any{
			"providerId":          provider.ID,
			"providerName":        provider.DisplayName,
			"model":               provider.Model,
			"baseUrl":             provider.BaseURL,
			"contextWindowTokens": provider.ContextWindowTokens,
			"enabled":             provider.Enabled,
			"default":             provider.Default,
			"hasApiKey":           provider.HasAPIKey,
			"callable":            callable,
			"capabilities":        provider.Capabilities,
		})
		if len(items) >= limit {
			break
		}
	}
	return map[string]any{
		"query":         query,
		"providerId":    providerID,
		"callableOnly":  callableOnly,
		"models":        items,
		"totalReturned": len(items),
	}, nil
}

func capabilityNames(capabilities map[string]bool) string {
	if len(capabilities) == 0 {
		return ""
	}
	names := make([]string, 0, len(capabilities))
	for name, enabled := range capabilities {
		if enabled {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return strings.Join(names, " ")
}

func toolBoolValue(input map[string]any, key string, defaultValue bool) bool {
	if input == nil {
		return defaultValue
	}
	switch value := input[key].(type) {
	case bool:
		return value
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "true", "1", "yes", "y":
			return true
		case "false", "0", "no", "n":
			return false
		}
	}
	return defaultValue
}
