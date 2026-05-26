package jftradeapi

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultStrategyDesignFilename = "strategy-definitions.json"
	strategyRuntimeQuickJS        = "quickjs-js"
)

var (
	legacyMovingAverageIndicatorPattern = regexp.MustCompile(`(^|[^[:alnum:]_])ma:(MA|EMA|WMA|VWMA):(5|20)([^:[:alnum:]_]|$)`)
	legacySimpleMovingAveragePattern    = regexp.MustCompile(`(^|[^[:alnum:]_])ma:(5|20)([^:[:alnum:]_]|$)`)
)

type strategyVisualNode struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	X          float64        `json:"x"`
	Y          float64        `json:"y"`
	Text       string         `json:"text,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
}

type strategyVisualEdge struct {
	ID           string         `json:"id,omitempty"`
	Type         string         `json:"type,omitempty"`
	SourceNodeID string         `json:"sourceNodeId"`
	TargetNodeID string         `json:"targetNodeId"`
	Text         string         `json:"text,omitempty"`
	Properties   map[string]any `json:"properties,omitempty"`
}

type strategyVisualModel struct {
	Engine  string               `json:"engine,omitempty"`
	Version int                  `json:"version,omitempty"`
	Nodes   []strategyVisualNode `json:"nodes,omitempty"`
	Edges   []strategyVisualEdge `json:"edges,omitempty"`
}

type strategyDesignDefinition struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Version     string               `json:"version"`
	Description string               `json:"description"`
	Runtime     string               `json:"runtime"`
	Symbol      string               `json:"symbol"`
	Interval    string               `json:"interval"`
	Script      string               `json:"script"`
	VisualModel *strategyVisualModel `json:"visualModel,omitempty"`
	CreatedAt   string               `json:"createdAt"`
	UpdatedAt   string               `json:"updatedAt"`
}

type strategyDesignFile struct {
	Definitions []strategyDesignDefinition `json:"definitions,omitempty"`
}

type strategyDesignStore struct {
	path string
	mu   sync.RWMutex
	data strategyDesignFile
}

func NewStrategyDesignStore(path string) (*strategyDesignStore, error) {
	store := &strategyDesignStore{path: path}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func deriveStrategyDesignPath(settingsPath string) string {
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return defaultStrategyDesignFilename
	}
	return filepath.Join(directory, defaultStrategyDesignFilename)
}

func (s *strategyDesignStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.data = strategyDesignFile{}
			return nil
		}
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		s.data = strategyDesignFile{}
		return nil
	}
	if err := json.Unmarshal(data, &s.data); err != nil {
		return err
	}
	migrated := false
	for index := range s.data.Definitions {
		normalized := normalizeStrategyDesignDefinition(s.data.Definitions[index])
		if !strategyDesignDefinitionsEqual(s.data.Definitions[index], normalized) {
			migrated = true
		}
		s.data.Definitions[index] = normalized
	}
	if migrated {
		return s.persistLocked()
	}
	return nil
}

func (s *strategyDesignStore) listDefinitions() []strategyDesignDefinition {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]strategyDesignDefinition, 0, len(s.data.Definitions))
	for _, definition := range s.data.Definitions {
		items = append(items, normalizeStrategyDesignDefinition(definition))
	}
	sort.Slice(items, func(i int, j int) bool {
		if items[i].UpdatedAt == items[j].UpdatedAt {
			return items[i].ID < items[j].ID
		}
		return items[i].UpdatedAt > items[j].UpdatedAt
	})
	return items
}

func (s *strategyDesignStore) definition(id string) (strategyDesignDefinition, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id = strings.TrimSpace(id)
	for _, definition := range s.data.Definitions {
		normalized := normalizeStrategyDesignDefinition(definition)
		if normalized.ID == id {
			return normalized, true
		}
	}
	return strategyDesignDefinition{}, false
}

func (s *strategyDesignStore) saveDefinition(input strategyDesignDefinition) (strategyDesignDefinition, error) {
	normalized := normalizeStrategyDesignDefinition(input)

	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Definitions {
		if s.data.Definitions[index].ID != normalized.ID {
			continue
		}
		normalized.CreatedAt = normalizeStrategyDesignDefinition(s.data.Definitions[index]).CreatedAt
		normalized.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		s.data.Definitions[index] = normalized
		return normalized, s.persistLocked()
	}
	s.data.Definitions = append(s.data.Definitions, normalized)
	return normalized, s.persistLocked()
}

func normalizeStrategyDesignDefinition(input strategyDesignDefinition) strategyDesignDefinition {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	input.ID = strings.TrimSpace(input.ID)
	if input.ID == "" {
		input.ID = "js-strategy-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		input.Name = input.ID
	}
	input.Version = strings.TrimSpace(input.Version)
	if input.Version == "" {
		input.Version = "0.1.0"
	}
	input.Description = strings.TrimSpace(input.Description)
	input.Runtime = normalizeStrategyRuntime(input.Runtime)
	input.Symbol = strings.ToUpper(strings.TrimSpace(input.Symbol))
	input.Interval = strings.TrimSpace(input.Interval)
	input.VisualModel = normalizeStrategyVisualModel(input.VisualModel)
	input.Script = migrateLegacyMovingAverageScript(input.Script)
	if input.Interval == "" {
		input.Interval = "1m"
	}
	if input.Script == "" {
		input.Script = defaultStrategyDesignScript(input.Name)
	}
	if input.CreatedAt == "" {
		input.CreatedAt = now
	}
	if input.UpdatedAt == "" {
		input.UpdatedAt = now
	}
	return input
}

func defaultStrategyDesignScript(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "quickjs-strategy"
	}
	return "function onInit(ctx) {\n" +
		"  console.log('init strategy', ctx.name, ctx.symbol, ctx.interval);\n" +
		"}\n\n" +
		"function onKLineClosed(ctx) {\n" +
		"  console.log('kline closed', ctx.kline.symbol, ctx.kline.close);\n" +
		"}\n"
}

func (s *strategyDesignStore) persistLocked() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

func normalizeStrategyRuntime(runtime string) string {
	runtime = strings.TrimSpace(runtime)
	if runtime == "" {
		return strategyRuntimeQuickJS
	}
	return runtime
}

func normalizeStrategyVisualModel(model *strategyVisualModel) *strategyVisualModel {
	if model == nil {
		return nil
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
		normalized.Nodes[index].Properties = migrateLegacyMovingAverageNodeProperties(
			normalized.Nodes[index].Properties,
		)
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
	return &normalized
}

func strategyDesignDefinitionsEqual(left, right strategyDesignDefinition) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return string(leftJSON) == string(rightJSON)
}

func migrateLegacyMovingAverageNodeProperties(properties map[string]any) map[string]any {
	if properties == nil {
		return map[string]any{}
	}
	blockKind, _ := properties["blockKind"].(string)
	indicatorType, _ := properties["indicatorType"].(string)
	if blockKind != "getTechnicalIndicator" || indicatorType != "movingAverage" {
		return properties
	}
	if unit, ok := properties["periodUnit"].(string); ok && strings.TrimSpace(unit) != "" {
		return properties
	}
	period := normalizeLegacyMovingAveragePeriod(properties["windowSize"])
	if period != 5 && period != 20 {
		return properties
	}
	next := cloneStringAnyMap(properties)
	next["periodUnit"] = "day"
	return next
}

func migrateLegacyMovingAverageScript(script string) string {
	trimmed := strings.TrimSpace(script)
	if trimmed == "" {
		return script
	}
	migrated := legacyMovingAverageIndicatorPattern.ReplaceAllString(script, "${1}ma:${2}:${3}:day${4}")
	migrated = legacySimpleMovingAveragePattern.ReplaceAllString(migrated, "${1}ma:MA:${2}:day${3}")
	return migrated
}

func normalizeLegacyMovingAveragePeriod(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0
		}
		return int(parsed)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

func cloneStringAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}
