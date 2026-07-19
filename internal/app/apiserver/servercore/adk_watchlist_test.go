package servercore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	watchliststore "github.com/jftrade/jftrade-main/internal/store/watchlist"
	"github.com/jftrade/jftrade-main/internal/watchlist"
)

type countingADKWatchlistSnapshotSource struct{ calls int }

func (s *countingADKWatchlistSnapshotSource) BatchSnapshots(_ context.Context, instrumentIDs []string) ([]watchlist.Quote, []watchlist.QuoteError, error) {
	s.calls++
	quotes := make([]watchlist.Quote, 0, len(instrumentIDs))
	for _, instrumentID := range instrumentIDs {
		quotes = append(quotes, watchlist.Quote{InstrumentID: instrumentID, ObservedAt: time.Now().UTC()})
	}
	return quotes, nil, nil
}

func TestADKWatchlistListReturnsRealDataWithoutImplicitQuoteCalls(t *testing.T) {
	repository, err := watchliststore.Open(t.Context(), filepath.Join(t.TempDir(), "watchlists.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = repository.Close() }()
	snapshots := &countingADKWatchlistSnapshotSource{}
	service := watchlist.NewService(repository, watchlist.WithBatchSnapshotSource(snapshots))
	groups, err := service.ListGroups(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReplaceMemberships(t.Context(), watchlist.ReplaceMembershipsInput{
		InstrumentID: "US.AAPL", GroupIDs: []string{groups[0].ID}, ExpectedRevision: 0,
	}); err != nil {
		t.Fatal(err)
	}
	server := &Server{serverFacades: serverFacades{watchlistSvc: service}}

	summaryValue, err := server.adkWatchlistList(t.Context(), WatchlistListInput{})
	if err != nil {
		t.Fatal(err)
	}
	summary := summaryValue.(map[string]any)
	if summary["includeQuotes"] != false || len(summary["groups"].([]watchlist.Group)) != 1 || snapshots.calls != 0 {
		t.Fatalf("summary = %#v quote calls=%d", summary, snapshots.calls)
	}

	itemsValue, err := server.adkWatchlistList(t.Context(), WatchlistListInput{Group: watchlist.DefaultGroupName})
	if err != nil {
		t.Fatal(err)
	}
	items := itemsValue.(map[string]any)
	if len(items["items"].([]watchlist.Item)) != 1 || items["includeQuotes"] != false || snapshots.calls != 0 {
		t.Fatalf("items = %#v quote calls=%d", items, snapshots.calls)
	}
	quotedValue, err := server.adkWatchlistList(t.Context(), WatchlistListInput{
		Group: watchlist.DefaultGroupName, IncludeQuotes: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	quoted := quotedValue.(map[string]any)
	if snapshots.calls != 1 || len(quoted["quotes"].([]watchlist.Quote)) != 1 {
		t.Fatalf("quoted = %#v quote calls=%d", quoted, snapshots.calls)
	}
}
