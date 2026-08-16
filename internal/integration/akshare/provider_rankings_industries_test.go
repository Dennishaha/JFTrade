package akshare

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

func writeRankingsFixture(writer http.ResponseWriter, market, kind string) {
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"market": market, "kind": kind,
		"entries": []map[string]any{
			{
				"instrument_id": "sh.600519", "name": "贵州茅台", "price": 1680.5,
				"change_rate": 5.42, "change_amount": 86.4, "volume": 123456,
				"turnover": 2.07e8, "turnover_ratio": 0.98, "pe_ttm": 24.6, "market_cap": 2.11e12,
			},
			{"instrument_id": "SZ.000001", "name": "平安银行", "price": nil, "change_rate": nil},
		},
		"source": "akshare-rankings",
	})
}

func TestClientRankingsEncodesMarketKindAndLimit(t *testing.T) {
	server, requests := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writeRankingsFixture(writer, "CN", "gainers")
	})
	client, err := NewClient(server.URL, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	response, err := client.rankings(context.Background(), "CN", "gainers", 30)
	if err != nil {
		t.Fatalf("rankings: %v", err)
	}
	if response.Kind != "gainers" || len(response.Entries) != 2 ||
		response.Entries[0].ChangeRate == nil || response.Entries[1].Price != nil {
		t.Fatalf("rankings response = %#v", response)
	}
	seen := requests()
	if len(seen) != 1 || seen[0].path != "/providers/akshare/rankings" ||
		seen[0].query.Get("market") != "CN" || seen[0].query.Get("kind") != "gainers" ||
		seen[0].query.Get("limit") != "30" {
		t.Fatalf("rankings request = %#v", seen)
	}
}

func TestClientIndustriesEncodesKindAndDecodesBoards(t *testing.T) {
	server, requests := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"market": "CN", "kind": "concept",
			"boards": []map[string]any{{
				"name": "人工智能", "change_rate": 2.31, "turnover": 1.5e9,
				"volume": 8.8e7, "leading_stock_name": "宁德时代", "leading_stock_change_rate": 7.02,
			}},
			"source": "akshare-industries",
		})
	})
	client, err := NewClient(server.URL, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	response, err := client.industries(context.Background(), "concept")
	if err != nil {
		t.Fatalf("industries: %v", err)
	}
	if len(response.Boards) != 1 || response.Boards[0].LeadingStockChangeRate == nil {
		t.Fatalf("industries response = %#v", response)
	}
	seen := requests()
	if len(seen) != 1 || seen[0].path != "/providers/akshare/industries" ||
		seen[0].query.Get("kind") != "concept" {
		t.Fatalf("industries request = %#v", seen)
	}
}

func TestClientIndustryMembersEscapesBoardNameAndOmitsEmptyKind(t *testing.T) {
	server, requests := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"market": "CN", "kind": "concept", "board": "人工智能",
			"entries": []map[string]any{{"instrument_id": "SZ.300750", "name": "宁德时代"}},
			"source":  "akshare-industries",
		})
	})
	client, err := NewClient(server.URL, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	response, err := client.industryMembers(context.Background(), "", "人工智能", 50)
	if err != nil {
		t.Fatalf("industryMembers: %v", err)
	}
	if response.Board != "人工智能" || len(response.Entries) != 1 {
		t.Fatalf("industry members response = %#v", response)
	}
	seen := requests()
	if len(seen) != 1 || seen[0].path != "/providers/akshare/industries/人工智能/members" ||
		seen[0].query.Has("kind") || seen[0].query.Get("limit") != "50" {
		t.Fatalf("industry members request = %#v", seen)
	}
}

func TestProviderRankingsConvertsEntriesAndNormalizesIdentity(t *testing.T) {
	server, requests := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writeRankingsFixture(writer, "CN", "gainers")
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	response, err := provider.Rankings(context.Background(), "cn", " Gainers ", 0)
	if err != nil {
		t.Fatalf("Rankings: %v", err)
	}
	if response.Market != "CN" || response.Kind != "gainers" || response.Source != "akshare-rankings" ||
		len(response.Entries) != 2 {
		t.Fatalf("rankings response = %#v", response)
	}
	first := response.Entries[0]
	if first.InstrumentID != "SH.600519" || first.Name != "贵州茅台" ||
		first.ChangeRate == nil || *first.ChangeRate != "5.42" || first.PETTM == nil {
		t.Fatalf("first entry = %#v", first)
	}
	if response.Entries[1].Price != nil || response.Entries[1].ChangeRate != nil {
		t.Fatalf("nullable entry = %#v", response.Entries[1])
	}
	seen := requests()
	if got := seen[0].query.Get("limit"); got != "20" {
		t.Fatalf("default limit query = %q", got)
	}
	if got := seen[0].query.Get("market"); got != "CN" {
		t.Fatalf("market query = %q", got)
	}
}

func TestProviderRankingsRejectsUnsupportedMarketAndKind(t *testing.T) {
	provider, err := NewProvider("http://127.0.0.1:1")
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	ctx := context.Background()
	if _, err := provider.Rankings(ctx, "US", "gainers", 20); !errors.Is(err, ErrUnsupported) ||
		!errors.Is(err, marketdata.ErrCapabilityUnsupported) {
		t.Fatalf("US rankings error = %v", err)
	}
	if _, err := provider.Rankings(ctx, "CN", "breakout", 20); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unknown kind error = %v", err)
	}
	if _, err := provider.Rankings(ctx, "MO", "gainers", 20); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unknown market error = %v", err)
	}
}

func TestProviderRankingsRejectsKindMismatch(t *testing.T) {
	server, _ := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writeRankingsFixture(writer, "CN", "losers")
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.Rankings(context.Background(), "CN", "gainers", 20); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("kind mismatch error = %v", err)
	}
}

func TestProviderIndustriesConvertsBoardsAndMembers(t *testing.T) {
	server, requests := newNewsRecordingServer(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/providers/akshare/industries" {
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"market": "CN", "kind": "industry",
				"boards": []map[string]any{{
					"name": "半导体", "change_rate": 1.8, "turnover": 9.6e8,
					"leading_stock_name": "中芯国际", "leading_stock_change_rate": 4.1,
				}},
				"source": "",
			})
			return
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"market": "CN", "kind": "industry", "board": "半导体",
			"entries": []map[string]any{{"instrument_id": "SH.688981", "name": "中芯国际"}},
			"source":  "akshare-industries",
		})
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	ctx := context.Background()

	boards, err := provider.Industries(ctx, "INDUSTRY")
	if err != nil {
		t.Fatalf("Industries: %v", err)
	}
	if boards.Market != "CN" || boards.Kind != "industry" || boards.Source != "akshare-industries" ||
		len(boards.Boards) != 1 || boards.Boards[0].Name != "半导体" ||
		boards.Boards[0].LeadingStockName != "中芯国际" {
		t.Fatalf("boards response = %#v", boards)
	}

	members, err := provider.IndustryMembers(ctx, "industry", " 半导体 ", 0)
	if err != nil {
		t.Fatalf("IndustryMembers: %v", err)
	}
	if members.Board != "半导体" || len(members.Entries) != 1 ||
		members.Entries[0].InstrumentID != "SH.688981" {
		t.Fatalf("members response = %#v", members)
	}
	seen := requests()
	if got := seen[1].query.Get("limit"); got != "20" {
		t.Fatalf("default members limit query = %q", got)
	}

	if _, err := provider.Industries(ctx, "region"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("region kind error = %v", err)
	}
	if _, err := provider.IndustryMembers(ctx, "concept", "  ", 20); err == nil {
		t.Fatal("empty board must fail")
	}
	if _, err := provider.IndustryMembers(ctx, "region", "半导体", 20); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("invalid members kind error = %v", err)
	}
}

func TestProviderIndustriesMapsSidecarUnsupportedToCapabilityError(t *testing.T) {
	server, requests := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"error":{"code":"AKSHARE_UNSUPPORTED","message":"US industries are not covered"}}`))
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.Industries(context.Background(), "industry"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("sidecar unsupported error = %v", err)
	}
	if len(requests()) != 1 {
		t.Fatalf("unsupported industries request retried: %d calls", len(requests()))
	}
}
