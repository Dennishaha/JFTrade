package akshare

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

func TestProviderNewsConvertsEntriesAndAppliesDefaultLimit(t *testing.T) {
	server, requests := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writeNewsFixture(writer, "SH", "600519")
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	response, err := provider.News(context.Background(), "cn", "SH.600519", 0)
	if err != nil {
		t.Fatalf("News: %v", err)
	}
	if response.InstrumentID != "SH.600519" || response.Market != "SH" || response.Symbol != "600519" ||
		response.Source != "akshare-news" || len(response.Entries) != 2 {
		t.Fatalf("news response = %#v", response)
	}
	if response.Entries[0].PublishedAt == nil || *response.Entries[0].PublishedAt != "2026-08-10T01:30:00Z" {
		t.Fatalf("publishedAt not normalized to UTC: %#v", response.Entries[0].PublishedAt)
	}
	if response.Entries[1].Title != nil || response.Entries[1].PublishedAt != nil {
		t.Fatalf("nullable entry = %#v", response.Entries[1])
	}
	if got := requests()[0].query.Get("limit"); got != "10" {
		t.Fatalf("default limit query = %q", got)
	}
}

func TestProviderCorporateActionsSortsAndValidatesEvents(t *testing.T) {
	server, _ := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writeCorporateActionsFixture(writer, "SZ", "000001")
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	response, err := provider.CorporateActions(
		context.Background(), "SZ", "000001",
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{},
	)
	if err != nil {
		t.Fatalf("CorporateActions: %v", err)
	}
	if response.Source != "akshare-actions" || len(response.Events) != 3 {
		t.Fatalf("corporate actions response = %#v", response)
	}
	got := make([][2]string, 0, len(response.Events))
	for _, event := range response.Events {
		got = append(got, [2]string{event.ExDate, event.Kind})
	}
	want := [][2]string{{"2026-05-11", "dividend"}, {"2026-06-09", "split"}, {"2026-08-10", "dividend"}}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("event order = %#v, want %#v", got, want)
		}
	}
}

func TestProviderNewsAndCorporateActionsRejectMalformedPayloads(t *testing.T) {
	server, _ := newNewsRecordingServer(t, func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.HasPrefix(request.URL.Path, "/providers/akshare/news/"):
			_, _ = writer.Write([]byte(`{
				"market":"SH","symbol":"600519","instrument_id":"SH.600519",
				"entries":[{"title":"t","link":null,"publisher":null,"published_at":"yesterday","summary":null}],
				"source":"akshare-news"}`))
		default:
			_, _ = writer.Write([]byte(`{
				"market":"SH","symbol":"600519","instrument_id":"SH.600519",
				"events":[{"kind":"buyback","ex_date":"2026-05-11","amount":null,"ratio":null}],
				"source":"akshare-actions"}`))
		}
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.News(context.Background(), "SH", "600519", 5); !errors.Is(err, ErrInvalidResponse) ||
		!strings.Contains(err.Error(), "news entry 0") {
		t.Fatalf("invalid published_at error = %v", err)
	}
	if _, err := provider.CorporateActions(context.Background(), "SH", "600519", time.Time{}, time.Time{}); !errors.Is(err, ErrInvalidResponse) ||
		!strings.Contains(err.Error(), "kind") {
		t.Fatalf("unknown kind error = %v", err)
	}
}

func TestProviderNewsRejectsIdentityMismatch(t *testing.T) {
	server, _ := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{
			"market":"SH","symbol":"000001","instrument_id":"SH.000001","entries":[],"source":"akshare-news"}`))
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.News(context.Background(), "SH", "600519", 5); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("identity mismatch error = %v", err)
	}
}

func TestProviderNewsAndCorporateActionsSurfaceUnsupportedUSAndHK(t *testing.T) {
	server, _ := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(400)
		_, _ = writer.Write([]byte(`{"error":{"code":"AKSHARE_UNSUPPORTED","message":"market not covered"}}`))
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.News(context.Background(), "US", "AAPL", 5); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("US news error = %v", err)
	}
	if _, err := provider.News(context.Background(), "HK", "00700", 5); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("HK news error = %v", err)
	}
	if _, err := provider.CorporateActions(context.Background(), "US", "AAPL", time.Time{}, time.Time{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("US corporate actions error = %v", err)
	}
}

func TestProviderNewsAndCorporateActionsMeetOptionalCapabilityContracts(t *testing.T) {
	provider, err := NewProvider("http://127.0.0.1:7788")
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	var _ marketdata.NewsSource = provider
	var _ marketdata.CorporateActionsSource = provider
}

func writeNewsFixture(writer http.ResponseWriter, market, symbol string) {
	_, _ = writer.Write([]byte(`{
		"market":"` + market + `","symbol":"` + symbol + `","instrument_id":"` + market + `.` + symbol + `",
		"entries":[
			{"title":"公告","link":"https://example.test/1","publisher":"FixtureWire","published_at":"2026-08-10T09:30:00+08:00","summary":"摘要"},
			{"title":null,"link":null,"publisher":null,"published_at":null,"summary":null}
		],
		"source":"akshare-news"}`))
}

func writeCorporateActionsFixture(writer http.ResponseWriter, market, symbol string) {
	_, _ = writer.Write([]byte(`{
		"market":"` + market + `","symbol":"` + symbol + `","instrument_id":"` + market + `.` + symbol + `",
		"events":[
			{"kind":"split","ex_date":"2026-06-09","amount":null,"ratio":4},
			{"kind":"dividend","ex_date":"2026-08-10","amount":0.26,"ratio":null},
			{"kind":"dividend","ex_date":"2026-05-11","amount":0.25,"ratio":null}
		],
		"source":"akshare-actions"}`))
}
