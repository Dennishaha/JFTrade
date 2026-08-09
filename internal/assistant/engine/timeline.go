package adk

import (
	"context"
	"fmt"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

// SessionTimeline delegates the session timeline projection to the shared
// model package so engine keeps only the composition-root Store surface.
func (s *Store) SessionTimeline(ctx context.Context, sessionID string) ([]TimelineEntry, bool, error) {
	if s == nil || s.StoreCore == nil {
		return nil, false, nil
	}
	return jfadkmodel.SessionTimeline(ctx, s, sessionID)
}

// SessionRuns delegates run pagination to the shared model package.
func (s *Store) SessionRuns(ctx context.Context, sessionID string) ([]Run, error) {
	if s == nil || s.StoreCore == nil {
		return nil, fmt.Errorf("adk store is unavailable")
	}
	return jfadkmodel.SessionRuns(ctx, s, sessionID)
}
