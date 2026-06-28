package servercore

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/mod/semver"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

func normalizeStrategyDesignDefinition(input strategyDesignDefinition) (strategyDesignDefinition, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	input.ID = strings.TrimSpace(input.ID)
	if input.ID == "" {
		input.ID = generateStrategyDefinitionID()
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		input.Name = input.ID
	}
	input.Version = normalizeStrategySemanticVersion(input.Version)
	input.Description = strings.TrimSpace(input.Description)
	sourceFormat, err := normalizeStrategyDesignSourceFormat(input.SourceFormat)
	if err != nil {
		return strategyDesignDefinition{}, err
	}
	runtime, err := normalizeStrategyRuntime(input.Runtime)
	if err != nil {
		return strategyDesignDefinition{}, err
	}
	input.SourceFormat = sourceFormat
	input.Runtime = runtime
	input.Symbol = strings.ToUpper(strings.TrimSpace(input.Symbol))
	input.Interval = strings.TrimSpace(input.Interval)
	visualModel, err := normalizeStrategyVisualModel(input.VisualModel)
	if err != nil {
		return strategyDesignDefinition{}, err
	}
	input.VisualModel = visualModel
	if strings.TrimSpace(input.Script) == "" {
		input.Script = defaultStrategyDesignScript(input.Name, input.SourceFormat)
	}
	input.Script = syncStrategyScriptVersion(input.Script, input.Version)
	if input.CreatedAt == "" {
		input.CreatedAt = now
	}
	if input.UpdatedAt == "" {
		input.UpdatedAt = now
	}
	return input, nil
}

func generateStrategyDefinitionID() string {
	id, err := uuid.NewRandom()
	if err != nil {
		return "pine-strategy-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	return id.String()
}

func defaultStrategyDesignScript(name string, sourceFormat string) string {
	_ = sourceFormat
	return defaultStrategyDesignPine(name)
}

func defaultStrategyDesignPine(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Pine Strategy"
	}
	escapedName := strings.ReplaceAll(name, `"`, `\"`)
	return "//@version=6\n" +
		"strategy(\"" + escapedName + "\", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10)\n\n" +
		"// JFTrade executes supported Pine strategy statements on each closed K line.\n" +
		"fast = ta.ema(close, 8)\n" +
		"slow = ta.ema(close, 21)\n" +
		"if ta.crossover(fast, slow)\n" +
		"    strategy.entry(\"Long\", strategy.long)\n"
}

func normalizeStrategySemanticVersion(value string) string {
	canonical := canonicalStrategySemanticVersion(value)
	if canonical == "" {
		return defaultStrategyVersion
	}
	return strings.TrimPrefix(canonical, "v")
}

func canonicalStrategySemanticVersion(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, "v") {
		value = "v" + value
	}
	return semver.Canonical(value)
}

func nextStrategyDefinitionVersion(current string) string {
	canonical := canonicalStrategySemanticVersion(current)
	if canonical == "" {
		return defaultStrategyVersion
	}
	parts := strings.Split(strings.TrimPrefix(canonical, "v"), ".")
	if len(parts) != 3 {
		return defaultStrategyVersion
	}
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	patch, patchErr := strconv.Atoi(parts[2])
	if majorErr != nil || minorErr != nil || patchErr != nil {
		return defaultStrategyVersion
	}
	return fmt.Sprintf("%d.%d.%d", major, minor, patch+1)
}

func syncStrategyScriptVersion(script string, version string) string {
	if strings.TrimSpace(script) == "" {
		return script
	}
	return script
}

func strategyDesignDefinitionMeaningfullyChanged(left, right strategyDesignDefinition) bool {
	left.CreatedAt = ""
	left.UpdatedAt = ""
	left.Version = ""
	left.Script = syncStrategyScriptVersion(left.Script, defaultStrategyVersion)
	right.CreatedAt = ""
	right.UpdatedAt = ""
	right.Version = ""
	right.Script = syncStrategyScriptVersion(right.Script, defaultStrategyVersion)
	return !strategyDesignDefinitionsEqual(left, right)
}

func normalizeStrategyRuntime(runtime string) (string, error) {
	normalized := pineworker.NormalizeRuntime(runtime)
	if normalized == strategyRuntimePinePlan {
		return normalized, nil
	}
	return "", fmt.Errorf("%w: runtime %q is no longer supported; use %s", errUnsupportedLegacyStrategyDefinition, runtime, strategyRuntimePinePlan)
}

func normalizeStrategyDesignSourceFormat(sourceFormat string) (string, error) {
	normalized := strings.TrimSpace(strings.ToLower(sourceFormat))
	if normalized == "" || normalized == strategydefinition.SourceFormatPineV6 {
		return strategydefinition.SourceFormatPineV6, nil
	}
	return "", fmt.Errorf("%w: sourceFormat %q is no longer supported; use %s", errUnsupportedLegacyStrategyDefinition, sourceFormat, strategydefinition.SourceFormatPineV6)
}

func normalizeStrategyVisualModel(model *strategyVisualModel) (*strategyVisualModel, error) {
	if model == nil {
		return nil, nil
	}
	normalized := *model
	if strings.TrimSpace(normalized.Engine) == "" {
		normalized.Engine = "logic-flow"
	}
	if normalized.Version == 0 {
		normalized.Version = 1
	}
	if normalized.Nodes == nil {
		normalized.Nodes = []strategyVisualNode{}
	}
	for index := range normalized.Nodes {
		if normalized.Nodes[index].Properties == nil {
			normalized.Nodes[index].Properties = map[string]any{}
		}
		if err := validateStrategyVisualNodeProperties(normalized.Nodes[index].Properties); err != nil {
			return nil, err
		}
	}
	if normalized.Edges == nil {
		normalized.Edges = []strategyVisualEdge{}
	}
	for index := range normalized.Edges {
		if normalized.Edges[index].Type == "" {
			normalized.Edges[index].Type = "polyline"
		}
		if normalized.Edges[index].Properties == nil {
			normalized.Edges[index].Properties = map[string]any{}
		}
	}
	return &normalized, nil
}

func validateStrategyVisualNodeProperties(properties map[string]any) error {
	blockKind := jftradeOptionalTypeAssertion[string](properties["blockKind"])
	switch strings.TrimSpace(blockKind) {
	case "codeBlock", "technicalIndicator":
		return fmt.Errorf("%w: visual block %q is no longer supported; rebuild it with Pine v6 blocks or pineSnippet", errUnsupportedLegacyStrategyDefinition, blockKind)
	default:
		return nil
	}
}

func strategyDesignDefinitionsEqual(left, right strategyDesignDefinition) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return string(leftJSON) == string(rightJSON)
}
