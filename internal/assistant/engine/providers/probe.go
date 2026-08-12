package providers

import (
	"context"
	"strings"
	"time"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

const MaxProviderProbeTimeout = 30 * time.Second

func ProviderProbeTimeout(provider jfadkmodel.Provider) time.Duration {
	configured := provider.RequestTimeout()
	if configured < MaxProviderProbeTimeout {
		return configured
	}
	return MaxProviderProbeTimeout
}

// ProbeProvider owns protocol-level health and reasoning probes. The caller
// remains responsible for persisting capabilities and audit metadata.
func ProbeProvider(
	ctx context.Context,
	provider jfadkmodel.Provider,
	apiKey string,
	mode jfadkmodel.ProviderTestMode,
) (jfadkmodel.ProviderTestResponse, error) {
	mode, err := jfadkmodel.NormalizeProviderTestMode(mode)
	if err != nil {
		return jfadkmodel.ProviderTestResponse{}, err
	}
	reply, err := probeProviderRequest(ctx, provider, apiKey, false, "")
	if err != nil {
		return jfadkmodel.ProviderTestResponse{}, err
	}
	_, toolErr := probeProviderRequest(ctx, provider, apiKey, true, "")
	reasoning, err := probeProviderReasoning(ctx, provider, apiKey, mode)
	if err != nil {
		return jfadkmodel.ProviderTestResponse{}, err
	}
	capabilities := map[string]bool{
		"streaming": true,
		"tools":     toolErr == nil,
		"reasoning": len(reasoning.Results) > 0 && reasoning.OK,
	}
	return jfadkmodel.ProviderTestResponse{
		OK: reasoning.OK, Reply: reply, Capabilities: capabilities, Reasoning: reasoning,
	}, nil
}

func probeProviderReasoning(
	ctx context.Context,
	provider jfadkmodel.Provider,
	apiKey string,
	mode jfadkmodel.ProviderTestMode,
) (jfadkmodel.ProviderReasoningTestResponse, error) {
	config := jfadkmodel.NormalizeProviderReasoningConfig(provider.ReasoningConfig, provider.APIProtocol)
	if err := jfadkmodel.ValidateProviderReasoningConfig(config); err != nil {
		return jfadkmodel.ProviderReasoningTestResponse{}, err
	}
	mappings := config.Mappings
	if mode == jfadkmodel.ProviderTestModeQuick {
		mappings = representativeReasoningMapping(mappings)
	}
	results := make([]jfadkmodel.ProviderReasoningTestResult, 0, len(mappings))
	for _, mapping := range mappings {
		_, probeErr := probeProviderRequest(ctx, provider, apiKey, false, mapping.Effort)
		result := jfadkmodel.ProviderReasoningTestResult{
			Effort: mapping.Effort, Value: mapping.Value, OK: probeErr == nil,
		}
		if probeErr != nil {
			result.Error = probeErr.Error()
		}
		results = append(results, result)
	}
	return jfadkmodel.ProviderReasoningTestResponse{
		Mode: mode, RequestField: config.RequestField,
		OK: len(results) == 0 || allReasoningProbesPassed(results), Results: results,
	}, nil
}

func representativeReasoningMapping(mappings []jfadkmodel.ProviderReasoningMapping) []jfadkmodel.ProviderReasoningMapping {
	if len(mappings) == 0 {
		return []jfadkmodel.ProviderReasoningMapping{}
	}
	for _, mapping := range mappings {
		if mapping.Effort == jfadkmodel.ReasoningEffortMedium {
			return []jfadkmodel.ProviderReasoningMapping{mapping}
		}
	}
	return []jfadkmodel.ProviderReasoningMapping{mappings[0]}
}

func probeProviderRequest(
	ctx context.Context,
	provider jfadkmodel.Provider,
	apiKey string,
	includeTool bool,
	effort jfadkmodel.ReasoningEffort,
) (string, error) {
	probeCtx, cancel := context.WithTimeout(ctx, ProviderProbeTimeout(provider))
	defer cancel()
	if provider.APIProtocol == jfadkmodel.ProviderAPIProtocolResponses {
		if effort != "" {
			return ProbeOpenAIResponsesProviderReasoning(probeCtx, provider, apiKey, effort)
		}
		return ProbeOpenAIResponsesProvider(probeCtx, provider, apiKey, includeTool)
	}
	if effort != "" {
		return ProbeOpenAICompatibleProviderReasoning(probeCtx, provider, apiKey, effort)
	}
	return ProbeOpenAICompatibleProvider(probeCtx, provider, apiKey, includeTool)
}

func allReasoningProbesPassed(results []jfadkmodel.ProviderReasoningTestResult) bool {
	for _, result := range results {
		if !result.OK || strings.TrimSpace(result.Error) != "" {
			return false
		}
	}
	return true
}
