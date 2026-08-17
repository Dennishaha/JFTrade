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

func TestClientCompanyResearchEndpointsEncodePathAndStatement(t *testing.T) {
	server, requests := newNewsRecordingServer(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/providers/akshare/profile/SH/600519":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"instrument_id": "SH.600519", "market": "SH", "symbol": "600519", "currency": "CNY",
				"groups": []map[string]any{{
					"title": "公司资料", "fields": []map[string]any{{"name": "行业", "value": "白酒"}},
				}},
			})
		case "/providers/akshare/financials/SH/600519":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"instrument_id": "SH.600519", "statement": "balance", "currency": "CNY",
				"fields":  []map[string]any{{"field_id": "total_assets", "display_name": "总资产"}},
				"periods": []map[string]any{},
			})
		case "/providers/akshare/ownership/SH/600519":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"instrument_id": "SH.600519",
				"groups": []map[string]any{{
					"kind": "major_holders", "static_date": "2026-06-30",
					"items": []map[string]any{{"name": "茅台集团", "holder_pct": 54.1}},
				}},
			})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	})
	client, err := NewClient(server.URL, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx := context.Background()

	profile, err := client.companyProfile(ctx, "SH", "600519")
	if err != nil || len(profile.Groups) != 1 {
		t.Fatalf("profile = %#v, err=%v", profile, err)
	}
	statements, err := client.financialStatements(ctx, "SH", "600519", "balance")
	if err != nil || statements.Statement != "balance" {
		t.Fatalf("financials = %#v, err=%v", statements, err)
	}
	ownership, err := client.ownership(ctx, "SH", "600519")
	if err != nil || len(ownership.Groups) != 1 {
		t.Fatalf("ownership = %#v, err=%v", ownership, err)
	}

	seen := requests()
	if len(seen) != 3 || seen[0].path != "/providers/akshare/profile/SH/600519" ||
		seen[1].path != "/providers/akshare/financials/SH/600519" ||
		seen[2].path != "/providers/akshare/ownership/SH/600519" {
		t.Fatalf("requests = %#v", seen)
	}
	if got := seen[1].query.Get("statement"); got != "balance" {
		t.Fatalf("statement query = %q", got)
	}
}

func TestProviderCompanyProfileConvertsCNInstrument(t *testing.T) {
	server, requests := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"instrument_id": "SH.600519", "market": "SH", "symbol": "600519", "currency": "CNY",
			"groups": []map[string]any{{
				"title": "公司资料",
				"fields": []map[string]any{
					{"name": "行业", "value": "白酒"},
					{"name": "", "value": ""},
				},
			}},
		})
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	response, err := provider.CompanyProfile(context.Background(), "cn", "SH.600519")
	if err != nil {
		t.Fatalf("CompanyProfile: %v", err)
	}
	if response.InstrumentID != "SH.600519" || response.Market != "SH" ||
		response.Source != "akshare-profile" || len(response.Groups) != 1 {
		t.Fatalf("profile response = %#v", response)
	}
	if len(response.Groups[0].Fields) != 1 || response.Groups[0].Fields[0].Value != "白酒" {
		t.Fatalf("profile fields = %#v", response.Groups[0].Fields)
	}
	seen := requests()
	if seen[0].path != "/providers/akshare/profile/SH/600519" {
		t.Fatalf("CN aggregate not resolved to leaf: %#v", seen)
	}
}

func TestProviderFinancialStatementsConvertsPeriodsAndValidatesEcho(t *testing.T) {
	server, requests := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"instrument_id": "BJ.430047", "statement": "income", "currency": "CNY",
			"fields": []map[string]any{{"field_id": "revenue", "display_name": "营业收入"}},
			"periods": []map[string]any{{
				"period_text": "2025FY",
				"values": map[string]any{"revenue": map[string]any{
					"data": 1.2e9, "yoy": 12.5, "qoq": nil,
				}},
			}},
		})
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	response, err := provider.FinancialStatements(context.Background(), "bj", "430047", "")
	if err != nil {
		t.Fatalf("FinancialStatements: %v", err)
	}
	if response.InstrumentID != "BJ.430047" || response.Statement != "income" ||
		response.Source != "akshare-financials" {
		t.Fatalf("statements response = %#v", response)
	}
	value := response.Periods[0].Values["revenue"]
	if value.Data == nil || value.YoY == nil || *value.YoY != "12.5" || value.QoQ != nil {
		t.Fatalf("period value = %#v", value)
	}
	seen := requests()
	if seen[0].path != "/providers/akshare/financials/BJ/430047" ||
		seen[0].query.Get("statement") != "income" {
		t.Fatalf("financials request = %#v", seen)
	}
}

func TestProviderOwnershipConvertsMajorHoldersAndHolderTypes(t *testing.T) {
	server, _ := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"instrument_id": "SZ.000001",
			"groups": []map[string]any{
				{"kind": "major_holders", "static_date": "2026-06-30",
					"items": []map[string]any{{"name": "平安集团", "holder_pct": 49.6}}},
				{"kind": "holder_types", "static_date": nil,
					"items": []map[string]any{{"name": "机构", "holder_pct": 55.3}}},
			},
		})
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	response, err := provider.Ownership(context.Background(), "SZ", "000001")
	if err != nil {
		t.Fatalf("Ownership: %v", err)
	}
	if len(response.Groups) != 2 || response.Groups[0].Kind != "major_holders" ||
		response.Groups[1].Kind != "holder_types" || response.Source != "akshare-ownership" {
		t.Fatalf("ownership response = %#v", response)
	}
	if response.Groups[0].Items[0].Name != "平安集团" ||
		*response.Groups[0].Items[0].HolderPct != "49.6" {
		t.Fatalf("ownership item = %#v", response.Groups[0].Items[0])
	}
}

func TestProviderCompanyResearchRejectsUnsupportedMarketsAndStatement(t *testing.T) {
	provider, err := NewProvider("http://127.0.0.1:1")
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	ctx := context.Background()
	if _, err := provider.CompanyProfile(ctx, "US", "AAPL"); !errors.Is(err, ErrUnsupported) ||
		!errors.Is(err, marketdata.ErrCapabilityUnsupported) {
		t.Fatalf("US profile error = %v", err)
	}
	if _, err := provider.Ownership(ctx, "HK", "00700"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("HK ownership error = %v", err)
	}
	if _, err := provider.FinancialStatements(ctx, "SH", "600519", "annual"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unknown statement error = %v", err)
	}
	if _, err := provider.CompanyProfile(ctx, "MO", "AAPL"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unknown market error = %v", err)
	}
}

func TestProviderCompanyResearchMapsSidecarUnsupportedMarket(t *testing.T) {
	server, requests := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"error":{"code":"unsupported_market","message":"BJ profile is not covered"}}`))
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.CompanyProfile(context.Background(), "BJ", "430047"); !errors.Is(err, ErrUnsupported) ||
		!errors.Is(err, marketdata.ErrCapabilityUnsupported) {
		t.Fatalf("sidecar unsupported error = %v", err)
	}
	if len(requests()) != 1 {
		t.Fatalf("unsupported request retried: %d calls", len(requests()))
	}
}

func TestProviderOwnershipRejectsUnknownGroupKind(t *testing.T) {
	server, _ := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"instrument_id": "SH.600519",
			"groups":        []map[string]any{{"kind": "executives", "items": []map[string]any{}}},
		})
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.Ownership(context.Background(), "SH", "600519"); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("unknown group kind error = %v", err)
	}
}

func TestProviderAnalystConsensusConvertsEastmoneyAggregate(t *testing.T) {
	server, requests := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"instrument_id": "SH.600519", "rating": 4.2, "analyst_count": 12,
			"target_price": nil,
			"distribution": map[string]any{
				"strong_buy": 45.0, "buy": 30.0, "hold": 20.0, "underperform": 5.0, "sell": 0.0,
			},
			"update_time": "2026-08-10",
		})
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	response, err := provider.AnalystConsensus(context.Background(), "cn", "SH.600519")
	if err != nil {
		t.Fatalf("AnalystConsensus: %v", err)
	}
	if response.InstrumentID != "SH.600519" || response.Market != "SH" ||
		response.Source != "akshare-analyst" {
		t.Fatalf("analyst response = %#v", response)
	}
	if response.Rating == nil || *response.Rating != "4.2" ||
		response.AnalystCount == nil || *response.AnalystCount != "12" {
		t.Fatalf("rating/analystCount = %#v", response)
	}
	if response.TargetPrice != nil {
		t.Fatalf("akshare target price must stay nil: %#v", response.TargetPrice)
	}
	if response.Distribution == nil || response.Distribution.StrongBuy == nil ||
		*response.Distribution.StrongBuy != "45" ||
		response.Distribution.Underperform == nil || *response.Distribution.Underperform != "5" {
		t.Fatalf("distribution = %#v", response.Distribution)
	}
	if response.UpdateTime == nil || *response.UpdateTime != "2026-08-10" {
		t.Fatalf("updateTime = %#v", response.UpdateTime)
	}
	seen := requests()
	if len(seen) != 1 || seen[0].path != "/providers/akshare/analyst/SH/600519" {
		t.Fatalf("analyst request = %#v", seen)
	}
}

func TestProviderAnalystConsensusMapsSidecarUnsupportedMarket(t *testing.T) {
	server, requests := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"error":{"code":"unsupported_market","message":"BJ analyst is not covered"}}`))
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.AnalystConsensus(context.Background(), "BJ", "430047"); !errors.Is(err, ErrUnsupported) ||
		!errors.Is(err, marketdata.ErrCapabilityUnsupported) {
		t.Fatalf("sidecar unsupported error = %v", err)
	}
	if len(requests()) != 1 {
		t.Fatalf("unsupported request retried: %d calls", len(requests()))
	}
}

func TestProviderAnalystConsensusPassesThroughNotFound(t *testing.T) {
	server, _ := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
		_, _ = writer.Write([]byte(`{"error":{"code":"not_found","message":"no analyst reports in window"}}`))
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	_, err = provider.AnalystConsensus(context.Background(), "SZ", "000001")
	var remoteErr *HTTPError
	if !errors.As(err, &remoteErr) || remoteErr.StatusCode != http.StatusNotFound {
		t.Fatalf("404 must surface as HTTPError, got %v", err)
	}
	if errors.Is(err, ErrUnsupported) || errors.Is(err, marketdata.ErrCapabilityUnsupported) {
		t.Fatalf("404 must not fold into capability errors: %v", err)
	}
}

func TestProviderAnalystConsensusRejectsUnsupportedMarketsGoSide(t *testing.T) {
	provider, err := NewProvider("http://127.0.0.1:1")
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	ctx := context.Background()
	if _, err := provider.AnalystConsensus(ctx, "US", "AAPL"); !errors.Is(err, ErrUnsupported) ||
		!errors.Is(err, marketdata.ErrCapabilityUnsupported) {
		t.Fatalf("US analyst error = %v", err)
	}
	if _, err := provider.AnalystConsensus(ctx, "HK", "00700"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("HK analyst error = %v", err)
	}
}
