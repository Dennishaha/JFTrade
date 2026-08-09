package providers

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/assistant/model"
	adkmemory "google.golang.org/adk/v2/memory"
	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

const AppNamePrefix = "jftrade-"

// MemoryStore is the persistence surface the ADK memory adapter needs.
type MemoryStore interface {
	ListMemory(ctx context.Context, agentID string) ([]model.MemoryEntry, error)
	ListMemoryFiltered(ctx context.Context, scope string, agentID string, query string) ([]model.MemoryEntry, error)
}

// NewMemoryService adapts the JFTrade memory store to the ADK memory service.
func NewMemoryService(store MemoryStore) *MemoryService {
	if store == nil {
		return nil
	}
	return &MemoryService{store: store}
}

// MemoryService is the ADK memory adapter over the JFTrade memory store.
type MemoryService struct {
	store MemoryStore
}

func (s *MemoryService) AddSessionToMemory(context.Context, adksession.Session) error {
	return nil
}

func (s *MemoryService) SearchMemory(ctx context.Context, req *adkmemory.SearchRequest) (*adkmemory.SearchResponse, error) {
	if s == nil || s.store == nil || req == nil {
		return &adkmemory.SearchResponse{}, nil
	}
	agentID := AgentIDFromAppName(req.AppName)
	var (
		entries []model.MemoryEntry
		err     error
	)
	if agentID == "" {
		entries, err = s.store.ListMemoryFiltered(ctx, "workspace", "", "")
	} else {
		entries, err = s.store.ListMemory(ctx, agentID)
	}
	if err != nil {
		return nil, err
	}
	query := strings.ToLower(strings.TrimSpace(req.Query))
	scored := make([]scoredMemory, 0, len(entries))
	for _, entry := range entries {
		score := MemoryScore(entry, query)
		if score <= 0 && query != "" {
			continue
		}
		scored = append(scored, scoredMemory{entry: entry, score: score})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		left := scored[i]
		right := scored[j]
		if left.score != right.score {
			return left.score > right.score
		}
		leftUpdated := ParseMemoryTime(left.entry.UpdatedAt)
		rightUpdated := ParseMemoryTime(right.entry.UpdatedAt)
		if !leftUpdated.Equal(rightUpdated) {
			return leftUpdated.After(rightUpdated)
		}
		if left.entry.Key != right.entry.Key {
			return left.entry.Key < right.entry.Key
		}
		return left.entry.ID < right.entry.ID
	})
	if len(scored) > 8 {
		scored = scored[:8]
	}
	memories := make([]adkmemory.Entry, 0, len(scored))
	for _, item := range scored {
		memories = append(memories, MemoryEntryFromModel(item.entry))
	}
	return &adkmemory.SearchResponse{Memories: memories}, nil
}

type scoredMemory struct {
	entry model.MemoryEntry
	score int
}

// MemoryEntryFromModel converts a JFTrade memory row into an ADK memory entry.
func MemoryEntryFromModel(entry model.MemoryEntry) adkmemory.Entry {
	timestamp, _ := time.Parse(time.RFC3339Nano, entry.UpdatedAt)
	text := strings.TrimSpace(entry.Value)
	if key := strings.TrimSpace(entry.Key); key != "" {
		text = key + ": " + text
	}
	return adkmemory.Entry{
		ID:        entry.ID,
		Content:   genai.NewContentFromText(text, genai.RoleUser),
		Author:    "jftrade.memory." + strings.TrimSpace(entry.Scope),
		Timestamp: timestamp,
		CustomMetadata: map[string]any{
			"agentId": entry.AgentID,
			"key":     entry.Key,
			"scope":   entry.Scope,
		},
	}
}

// MemoryScore ranks a memory entry against a normalized query.
func MemoryScore(entry model.MemoryEntry, query string) int {
	if query == "" {
		return 1
	}
	key := strings.ToLower(strings.TrimSpace(entry.Key))
	value := strings.ToLower(strings.TrimSpace(entry.Value))
	scope := strings.ToLower(strings.TrimSpace(entry.Scope))
	score := 0
	for token := range strings.FieldsSeq(query) {
		if token == "" {
			continue
		}
		if key == token {
			score += 4
		} else if strings.Contains(key, token) {
			score += 3
		}
		if strings.Contains(value, token) {
			score += 2
		}
		if strings.Contains(scope, token) {
			score++
		}
	}
	return score
}

// MemoryMatches reports whether a query should surface the memory entry.
func MemoryMatches(entry model.MemoryEntry, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	return query == "" || MemoryScore(entry, query) > 0
}

// ParseMemoryTime parses an RFC3339Nano memory timestamp.
func ParseMemoryTime(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed
}

// AgentIDFromAppName resolves the agent id from an ADK app name.
func AgentIDFromAppName(appName string) string {
	appName = strings.TrimSpace(appName)
	if appName == "" || appName == "jftrade-default" {
		return ""
	}
	if after, ok := strings.CutPrefix(appName, AppNamePrefix); ok {
		return after
	}
	return strings.TrimSpace(appName)
}

// AppName builds the stable ADK app name for an agent id.
func AppName(id string) string {
	normalized := model.NormalizeID(id)
	if normalized == "" {
		return "jftrade-default"
	}
	return AppNamePrefix + normalized
}
