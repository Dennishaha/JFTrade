package adk

import (
	"context"
	"sort"
	"strings"
	"time"

	adkmemory "google.golang.org/adk/v2/memory"
	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

const googleADKAppNamePrefix = "jftrade-"

type googleADKMemoryService struct {
	store *Store
}

func newGoogleADKMemoryService(store *Store) adkmemory.Service {
	if store == nil {
		return nil
	}
	return &googleADKMemoryService{store: store}
}

func (s *googleADKMemoryService) AddSessionToMemory(context.Context, adksession.Session) error {
	return nil
}

func (s *googleADKMemoryService) SearchMemory(ctx context.Context, req *adkmemory.SearchRequest) (*adkmemory.SearchResponse, error) {
	if s == nil || s.store == nil || req == nil {
		return &adkmemory.SearchResponse{}, nil
	}
	agentID := googleADKAgentIDFromAppName(req.AppName)
	var (
		entries []MemoryEntry
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
	scored := make([]googleADKScoredMemory, 0, len(entries))
	for _, entry := range entries {
		score := googleADKMemoryScore(entry, query)
		if score <= 0 && query != "" {
			continue
		}
		scored = append(scored, googleADKScoredMemory{entry: entry, score: score})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		left := scored[i]
		right := scored[j]
		if left.score != right.score {
			return left.score > right.score
		}
		leftUpdated := parseMemoryTime(left.entry.UpdatedAt)
		rightUpdated := parseMemoryTime(right.entry.UpdatedAt)
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
		memories = append(memories, googleADKMemoryEntry(item.entry))
	}
	return &adkmemory.SearchResponse{Memories: memories}, nil
}

type googleADKScoredMemory struct {
	entry MemoryEntry
	score int
}

func googleADKMemoryEntry(entry MemoryEntry) adkmemory.Entry {
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

func googleADKMemoryScore(entry MemoryEntry, query string) int {
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

func googleADKMemoryMatches(entry MemoryEntry, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	return query == "" || googleADKMemoryScore(entry, query) > 0
}

func parseMemoryTime(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed
}

func googleADKAgentIDFromAppName(appName string) string {
	appName = strings.TrimSpace(appName)
	if appName == "" || appName == "jftrade-default" {
		return ""
	}
	if after, ok := strings.CutPrefix(appName, googleADKAppNamePrefix); ok {
		return after
	}
	return strings.TrimSpace(appName)
}

func googleADKAppName(id string) string {
	normalized := normalizeID(id)
	if normalized == "" {
		return "jftrade-default"
	}
	return googleADKAppNamePrefix + normalized
}
