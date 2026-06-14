package strategy

import srv "github.com/jftrade/jftrade-main/internal/strategy"

// AnalyzePineRequest documents the Pine analysis payload.
type AnalyzePineRequest struct {
	Script       string `json:"script"`
	SourceFormat string `json:"sourceFormat,omitempty"`
	IncludeAST   bool   `json:"includeAst,omitempty"`
}

// StrategyDesignDefinition documents the strategy definition write contract.
type StrategyDesignDefinition struct {
	ID           string               `json:"id,omitempty"`
	Name         string               `json:"name,omitempty"`
	Description  string               `json:"description,omitempty"`
	Runtime      string               `json:"runtime,omitempty"`
	SourceFormat string               `json:"sourceFormat,omitempty"`
	Script       string               `json:"script,omitempty"`
	Symbol       string               `json:"symbol,omitempty"`
	Interval     string               `json:"interval,omitempty"`
	Version      string               `json:"version,omitempty"`
	VisualModel  *StrategyVisualModel `json:"visualModel,omitempty"`
	CreatedAt    string               `json:"createdAt,omitempty"`
	UpdatedAt    string               `json:"updatedAt,omitempty"`
}

// StrategyVisualModel documents the visual strategy editor payload.
type StrategyVisualModel struct {
	Engine  string               `json:"engine,omitempty"`
	Version int                  `json:"version,omitempty"`
	Nodes   []StrategyVisualNode `json:"nodes,omitempty"`
	Edges   []StrategyVisualEdge `json:"edges,omitempty"`
}

// StrategyVisualNode documents a visual strategy node.
type StrategyVisualNode struct {
	ID         string         `json:"id,omitempty"`
	Type       string         `json:"type,omitempty"`
	Text       string         `json:"text,omitempty"`
	X          float64        `json:"x,omitempty"`
	Y          float64        `json:"y,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
}

// StrategyVisualEdge documents a visual strategy edge.
type StrategyVisualEdge struct {
	ID           string         `json:"id,omitempty"`
	Type         string         `json:"type,omitempty"`
	Text         string         `json:"text,omitempty"`
	SourceNodeID string         `json:"sourceNodeId,omitempty"`
	TargetNodeID string         `json:"targetNodeId,omitempty"`
	Properties   map[string]any `json:"properties,omitempty"`
}

// StrategyBindingRequest documents strategy instance binding updates.
type StrategyBindingRequest = srv.InstanceBinding
