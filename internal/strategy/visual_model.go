package strategy

import (
	"errors"
	"fmt"
	"strings"
)

// ErrUnsupportedLegacyDefinition identifies persisted strategy shapes that no
// longer have a Pine v6 representation.
var ErrUnsupportedLegacyDefinition = errors.New("unsupported legacy strategy definition")

// NormalizeVisualModel applies the business defaults shared by persistence and
// assistant-driven strategy writes. Keeping this policy in the strategy domain
// prevents cross-domain adapters from depending on a concrete store package.
func NormalizeVisualModel(model *VisualModel) (*VisualModel, error) {
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
		normalized.Nodes = []VisualNode{}
	}
	for index := range normalized.Nodes {
		if normalized.Nodes[index].Properties == nil {
			normalized.Nodes[index].Properties = map[string]any{}
		}
		if err := validateVisualNodeProperties(normalized.Nodes[index].Properties); err != nil {
			return nil, err
		}
	}
	if normalized.Edges == nil {
		normalized.Edges = []VisualEdge{}
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

func validateVisualNodeProperties(properties map[string]any) error {
	blockKind, _ := properties["blockKind"].(string)
	switch strings.TrimSpace(blockKind) {
	case "codeBlock", "technicalIndicator":
		return fmt.Errorf(
			"%w: visual block %q is no longer supported; rebuild it with Pine v6 blocks or pineSnippet",
			ErrUnsupportedLegacyDefinition,
			blockKind,
		)
	default:
		return nil
	}
}
