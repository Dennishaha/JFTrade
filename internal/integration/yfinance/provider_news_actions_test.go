package yfinance

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/integration/yfinance/testkit"
	"github.com/jftrade/jftrade-main/internal/marketdata"
)

func TestProviderNewsNormalizesEntriesAndDefaults(t *testing.T) {
	server := testkit.New(t)
	provider, err := NewProvider(server.URL())
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	response, err := provider.News(context.Background(), "us", "aapl", 0)
	if err != nil {
		t.Fatalf("News: %v", err)
	}
	if response.InstrumentID != "US.AAPL" || response.Market != "US" || response.Symbol != "AAPL" ||
		response.Source != "yfinance-news" || len(response.Entries) != 2 {
		t.Fatalf("news response = %#v", response)
	}
	first := response.Entries[0]
	if first.Title == nil || *first.Title != "AAPL beats expectations" {
		t.Fatalf("first entry title = %#v", first.Title)
	}
	if first.PublishedAt == nil || *first.PublishedAt != "2026-08-10T13:30:00Z" {
		t.Fatalf("publishedAt not normalized to UTC: %#v", first.PublishedAt)
	}
	second := response.Entries[1]
	if second.Title != nil || second.Link != nil || second.Publisher != nil ||
		second.PublishedAt != nil || second.Summary != nil {
		t.Fatalf("nullable entry = %#v", second)
	}
	// limit 0 falls back to the default before reaching the sidecar.
	if got := server.Requests()[0].Query.Get("limit"); got != "10" {
		t.Fatalf("default limit query = %q", got)
	}
}

func TestProviderNewsRejectsMismatchedIdentityAndInvalidTimestamps(t *testing.T) {
	server := testkit.New(t)
	server.Queue("/news/US/AAPL", testkit.Response{Body: `{
		"market":"US","symbol":"MSFT","instrument_id":"US.MSFT","entries":[],"source":"yfinance-news"}`})
	server.Queue("/news/US/AAPL", testkit.Response{Body: `{
		"market":"US","symbol":"AAPL","instrument_id":"US.AAPL",
		"entries":[{"title":"t","link":null,"publisher":null,"published_at":"not-a-time","summary":null}],
		"source":"yfinance-news"}`})
	provider, err := NewProvider(server.URL())
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.News(context.Background(), "US", "AAPL", 5); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("identity mismatch error = %v", err)
	}
	if _, err := provider.News(context.Background(), "US", "AAPL", 5); !errors.Is(err, ErrInvalidResponse) ||
		!strings.Contains(err.Error(), "news entry 0") {
		t.Fatalf("invalid published_at error = %v", err)
	}
}

func TestProviderCorporateActionsSortsEventsByExDateAndKind(t *testing.T) {
	server := testkit.New(t)
	provider, err := NewProvider(server.URL())
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	response, err := provider.CorporateActions(
		context.Background(), "US", "AAPL",
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{},
	)
	if err != nil {
		t.Fatalf("CorporateActions: %v", err)
	}
	if response.Source != "yfinance-actions" || len(response.Events) != 3 {
		t.Fatalf("corporate actions response = %#v", response)
	}
	got := [][2]string{}
	for _, event := range response.Events {
		got = append(got, [2]string{event.ExDate, event.Kind})
	}
	want := [][2]string{{"2026-05-11", "dividend"}, {"2026-06-09", "split"}, {"2026-08-10", "dividend"}}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("event order = %#v, want %#v", got, want)
		}
	}
	if response.Events[0].Amount == nil || response.Events[0].Ratio != nil ||
		response.Events[1].Ratio == nil || response.Events[1].Amount != nil {
		t.Fatalf("event amount/ratio = %#v", response.Events)
	}
	if query := server.Requests()[0].Query; query.Get("from") != "2024-01-01T00:00:00Z" || query.Has("to") {
		t.Fatalf("corporate actions query = %#v", query)
	}
}

func TestProviderCorporateActionsRejectsUnknownKindsAndBadExDates(t *testing.T) {
	server := testkit.New(t)
	server.Queue("/corporate-actions/US/AAPL", testkit.Response{Body: `{
		"market":"US","symbol":"AAPL","instrument_id":"US.AAPL",
		"events":[{"kind":"buyback","ex_date":"2026-05-11","amount":null,"ratio":null}],"source":"yfinance-actions"}`})
	server.Queue("/corporate-actions/US/AAPL", testkit.Response{Body: `{
		"market":"US","symbol":"AAPL","instrument_id":"US.AAPL",
		"events":[{"kind":"dividend","ex_date":"2026-05","amount":1,"ratio":null}],"source":"yfinance-actions"}`})
	provider, err := NewProvider(server.URL())
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.CorporateActions(context.Background(), "US", "AAPL", time.Time{}, time.Time{}); !errors.Is(err, ErrInvalidResponse) ||
		!strings.Contains(err.Error(), "kind") {
		t.Fatalf("unknown kind error = %v", err)
	}
	if _, err := provider.CorporateActions(context.Background(), "US", "AAPL", time.Time{}, time.Time{}); !errors.Is(err, ErrInvalidResponse) ||
		!strings.Contains(err.Error(), "ex_date") {
		t.Fatalf("bad ex_date error = %v", err)
	}
}

func TestProviderNewsAndCorporateActionsSupportEveryLeafMarket(t *testing.T) {
	server := testkit.New(t)
	provider, err := NewProvider(server.URL())
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	for _, instrument := range [][2]string{{"US", "AAPL"}, {"HK", "00700"}, {"SH", "600519"}, {"SZ", "000001"}} {
		if _, err := provider.News(context.Background(), instrument[0], instrument[1], 3); err != nil {
			t.Fatalf("News(%s.%s): %v", instrument[0], instrument[1], err)
		}
		if _, err := provider.CorporateActions(context.Background(), instrument[0], instrument[1], time.Time{}, time.Time{}); err != nil {
			t.Fatalf("CorporateActions(%s.%s): %v", instrument[0], instrument[1], err)
		}
	}
}

func TestProviderNewsAndCorporateActionsMeetOptionalCapabilityContracts(t *testing.T) {
	provider, err := NewProvider("http://127.0.0.1:7788")
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	var _ marketdata.NewsSource = provider
	var _ marketdata.CorporateActionsSource = provider
	if _, err := provider.News(context.Background(), "XX", "AAPL", 5); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported market news error = %v", err)
	}
	if _, err := provider.CorporateActions(context.Background(), "XX", "AAPL", time.Time{}, time.Time{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported market corporate actions error = %v", err)
	}
}
