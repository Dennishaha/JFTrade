package adk

import (
	"context"
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
	memories := make([]adkmemory.Entry, 0, len(entries))
	for _, entry := range entries {
		if !googleADKMemoryMatches(entry, query) {
			continue
		}
		memories = append(memories, googleADKMemoryEntry(entry))
	}
	return &adkmemory.SearchResponse{Memories: memories}, nil
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

func googleADKMemoryMatches(entry MemoryEntry, query string) bool {
	if query == "" {
		return true
	}
	haystack := strings.ToLower(strings.TrimSpace(entry.Key) + " " + strings.TrimSpace(entry.Value) + " " + strings.TrimSpace(entry.Scope))
	for token := range strings.FieldsSeq(query) {
		if strings.Contains(haystack, token) {
			return true
		}
	}
	return false
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
