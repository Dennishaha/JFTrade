package assembly

import (
	"context"
	"fmt"
	"strings"

	"github.com/jftrade/jftrade-main/internal/watchlist"
)

// WatchlistService is the narrow watchlist surface consumed by assistant
// tools. Quote reads remain opt-in and do not create live subscription demand.
type WatchlistService interface {
	ListGroups(context.Context) ([]watchlist.Group, error)
	ListImportRuns(context.Context, string, string, int) (watchlist.ImportRunPage, error)
	ListSources(context.Context) ([]watchlist.Source, error)
	ListItems(context.Context, watchlist.ListItemsOptions) (watchlist.ItemPage, error)
	BatchQuotes(context.Context, []string) (watchlist.BatchQuotes, error)
}

// WatchlistToolAdapter maps assistant inputs onto the watchlist service.
type WatchlistToolAdapter struct {
	service WatchlistService
}

// NewWatchlistToolAdapter creates the assistant watchlist adapter.
func NewWatchlistToolAdapter(service WatchlistService) *WatchlistToolAdapter {
	return &WatchlistToolAdapter{service: service}
}

// List returns group summaries or one group's paged members. Quotes are fetched
// only when the caller explicitly sets IncludeQuotes.
func (a *WatchlistToolAdapter) List(ctx context.Context, input WatchlistListInput) (any, error) {
	if a == nil || a.service == nil {
		return nil, fmt.Errorf("watchlist is unavailable")
	}
	groups, err := a.service.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	recentImports, err := a.service.ListImportRuns(ctx, "", "", 10)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Group) == "" {
		sources, sourceErr := a.service.ListSources(ctx)
		if sourceErr != nil {
			return nil, sourceErr
		}
		return map[string]any{
			"groups": groups, "sources": sources, "recentImports": recentImports.Items,
			"includeQuotes": false, "checkedAt": NowStringRFC3339Nano(),
		}, nil
	}
	group, ok := resolveWatchlistGroup(groups, input.Group)
	if !ok {
		return nil, fmt.Errorf("watchlist group %q not found", input.Group)
	}
	page, err := a.service.ListItems(ctx, watchlist.ListItemsOptions{
		GroupID: group.ID, Cursor: input.Cursor, Limit: input.Limit, Query: input.Query, Market: input.Market,
	})
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"group": group, "items": page.Items, "nextCursor": page.NextCursor,
		"recentImports": recentImports.Items, "includeQuotes": input.IncludeQuotes,
	}
	if input.IncludeQuotes && len(page.Items) > 0 {
		instrumentIDs := make([]string, 0, len(page.Items))
		for _, item := range page.Items {
			instrumentIDs = append(instrumentIDs, item.ID)
		}
		quotes, quoteErr := a.service.BatchQuotes(ctx, instrumentIDs)
		if quoteErr != nil {
			return nil, quoteErr
		}
		result["quotes"] = quotes.Quotes
		result["quoteErrors"] = quotes.Errors
		result["quotesObservedAt"] = quotes.ObservedAt
	}
	return result, nil
}

func resolveWatchlistGroup(groups []watchlist.Group, value string) (watchlist.Group, bool) {
	value = strings.TrimSpace(value)
	for _, group := range groups {
		if group.ID == value || strings.EqualFold(group.Name, value) {
			return group, true
		}
	}
	return watchlist.Group{}, false
}
