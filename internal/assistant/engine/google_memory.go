package adk

import (
	"time"

	"github.com/jftrade/jftrade-main/internal/assistant/engine/providers"
	adkmemory "google.golang.org/adk/v2/memory"
)

func newGoogleADKMemoryService(store *Store) adkmemory.Service {
	if store == nil {
		return nil
	}
	return providers.NewMemoryService(store)
}

func googleADKMemoryEntry(entry MemoryEntry) adkmemory.Entry {
	return providers.MemoryEntryFromModel(entry)
}

func googleADKMemoryScore(entry MemoryEntry, query string) int {
	return providers.MemoryScore(entry, query)
}

func googleADKMemoryMatches(entry MemoryEntry, query string) bool {
	return providers.MemoryMatches(entry, query)
}

func parseMemoryTime(value string) time.Time {
	return providers.ParseMemoryTime(value)
}

func googleADKAgentIDFromAppName(appName string) string {
	return providers.AgentIDFromAppName(appName)
}

func GoogleADKAppName(id string) string {
	return providers.AppName(id)
}
