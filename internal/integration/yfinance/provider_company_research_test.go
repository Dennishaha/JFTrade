package yfinance

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/integration/yfinance/testkit"
	"github.com/jftrade/jftrade-main/internal/marketdata"
)

func TestClientCompanyResearchEndpointsEncodePathAndStatement(t *testing.T) {
	server := testkit.New(t)
	server.Queue("/profile/US/AAPL", testkit.Response{Body: `{
		"instrument_id":"US.AAPL","market":"US","symbol":"AAPL","currency":"USD",
		"groups":[{"title":"Company","fields":[{"name":"Sector","value":"Technology"}]}]}`})
	server.Queue("/financials/US/AAPL", testkit.Response{Body: `{
		"instrument_id":"US.AAPL","statement":"cashflow","currency":"USD",
		"fields":[{"field_id":"operating_cf","display_name":"Operating Cash Flow"}],
		"periods":[{"period_text":"2025FY","values":{"operating_cf":{"data":1.1e11,"yoy":3.2,"qoq":null}}}]}`})
	server.Queue("/analyst/US/AAPL", testkit.Response{Body: `{
		"instrument_id":"US.AAPL","rating":4,"analyst_count":38,
		"target_price":{"lowest":180.5,"average":240.1,"highest":300},
		"distribution":{"strong_buy":42.1,"buy":31.6,"hold":21.1,"underperform":5.2,"sell":0},
		"update_time":"2026-08-15T20:00:00Z"}`})
	server.Queue("/ownership/US/AAPL", testkit.Response{Body: `{
		"instrument_id":"US.AAPL",
		"groups":[{"kind":"major_holders","static_date":"2026-06-30",
			"items":[{"name":"Vanguard","holder_pct":8.6}]}]}`})
	client, err := NewClient(server.URL(), &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx := context.Background()

	profile, err := client.companyProfile(ctx, "US", "AAPL")
	if err != nil || len(profile.Groups) != 1 || profile.Groups[0].Fields[0].Name != "Sector" {
		t.Fatalf("profile = %#v, err=%v", profile, err)
	}
	statements, err := client.financialStatements(ctx, "US", "AAPL", "cashflow")
	if err != nil || statements.Statement != "cashflow" || len(statements.Periods) != 1 {
		t.Fatalf("financials = %#v, err=%v", statements, err)
	}
	analyst, err := client.analystConsensus(ctx, "US", "AAPL")
	if err != nil || analyst.TargetPrice == nil || analyst.Distribution == nil {
		t.Fatalf("analyst = %#v, err=%v", analyst, err)
	}
	ownership, err := client.ownership(ctx, "US", "AAPL")
	if err != nil || len(ownership.Groups) != 1 || ownership.Groups[0].Kind != "major_holders" {
		t.Fatalf("ownership = %#v, err=%v", ownership, err)
	}

	requests := server.Requests()
	if len(requests) != 4 {
		t.Fatalf("requests = %#v", requests)
	}
	if requests[0].Path != "/providers/yfinance/profile/US/AAPL" ||
		requests[2].Path != "/providers/yfinance/analyst/US/AAPL" ||
		requests[3].Path != "/providers/yfinance/ownership/US/AAPL" {
		t.Fatalf("request paths = %#v", requests)
	}
	if got := requests[1].Query.Get("statement"); got != "cashflow" {
		t.Fatalf("statement query = %q", got)
	}
}

func TestProviderCompanyProfileConvertsGroupsAndSkipsEmptyFields(t *testing.T) {
	server := testkit.New(t)
	server.Queue("/profile/US/AAPL", testkit.Response{Body: `{
		"instrument_id":"US.AAPL","market":"US","symbol":"AAPL","currency":" USD ",
		"groups":[
			{"title":" Company ","fields":[
				{"name":" Sector ","value":" Technology "},
				{"name":"","value":""}]},
			{"title":"Listing","fields":[{"name":"Exchange","value":"NASDAQ"}]}]}`})
	provider, err := NewProvider(server.URL())
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	response, err := provider.CompanyProfile(context.Background(), "us", "aapl")
	if err != nil {
		t.Fatalf("CompanyProfile: %v", err)
	}
	if response.InstrumentID != "US.AAPL" || response.Source != "yfinance-profile" ||
		len(response.Groups) != 2 {
		t.Fatalf("profile response = %#v", response)
	}
	company := response.Groups[0]
	if company.Title != "Company" || len(company.Fields) != 1 ||
		company.Fields[0].Name != "Sector" || company.Fields[0].Value != "Technology" {
		t.Fatalf("company group = %#v", company)
	}
	if response.Currency == nil || *response.Currency != "USD" {
		t.Fatalf("currency = %#v", response.Currency)
	}
}

func TestProviderFinancialStatementsConvertsFieldsPeriodsAndNullableRatios(t *testing.T) {
	server := testkit.New(t)
	server.Queue("/financials/HK/00700", testkit.Response{Body: `{
		"instrument_id":"HK.00700","statement":"income","currency":null,
		"fields":[{"field_id":"revenue","display_name":"Revenue"}],
		"periods":[
			{"period_text":"2025FY","values":{"revenue":{"data":6.6e11,"yoy":8.4,"qoq":null}}},
			{"period_text":"2024FY","values":{"revenue":{"data":6.09e11,"yoy":null,"qoq":null}}}]}`})
	provider, err := NewProvider(server.URL())
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	response, err := provider.FinancialStatements(context.Background(), "HK", "700", " INCOME ")
	if err != nil {
		t.Fatalf("FinancialStatements: %v", err)
	}
	if response.InstrumentID != "HK.00700" || response.Statement != "income" ||
		response.Source != "yfinance-financials" || response.Currency != nil {
		t.Fatalf("statements response = %#v", response)
	}
	first := response.Periods[0]
	value := first.Values["revenue"]
	if value.Data == nil || *value.Data != "6.6e11" || value.YoY == nil || value.QoQ != nil {
		t.Fatalf("first period value = %#v", value)
	}
	if response.Periods[1].Values["revenue"].YoY != nil {
		t.Fatalf("second period yoy = %#v", response.Periods[1].Values["revenue"])
	}
	requests := server.Requests()
	if got := requests[0].Query.Get("statement"); got != "income" {
		t.Fatalf("statement query = %q", got)
	}
}

func TestProviderAnalystConsensusConvertsRatingTargetAndDistribution(t *testing.T) {
	server := testkit.New(t)
	server.Queue("/analyst/US/AAPL", testkit.Response{Body: `{
		"instrument_id":"US.AAPL","rating":4,"analyst_count":38,
		"target_price":{"lowest":180.5,"average":240.1,"highest":300},
		"distribution":{"strong_buy":42.1,"buy":31.6,"hold":21.1,"underperform":5.2,"sell":0},
		"update_time":" 2026-08-15T20:00:00Z "}`})
	provider, err := NewProvider(server.URL())
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	response, err := provider.AnalystConsensus(context.Background(), "US", "AAPL")
	if err != nil {
		t.Fatalf("AnalystConsensus: %v", err)
	}
	if response.Rating == nil || *response.Rating != "4" ||
		response.AnalystCount == nil || *response.AnalystCount != "38" ||
		response.TargetPrice == nil || *response.TargetPrice.Average != "240.1" ||
		response.Distribution == nil || *response.Distribution.StrongBuy != "42.1" {
		t.Fatalf("analyst response = %#v", response)
	}
	if response.UpdateTime == nil || *response.UpdateTime != "2026-08-15T20:00:00Z" ||
		response.Source != "yfinance-analyst" {
		t.Fatalf("analyst meta = %#v", response)
	}
}

func TestProviderOwnershipConvertsGroupsAndValidatesKind(t *testing.T) {
	server := testkit.New(t)
	server.Queue("/ownership/US/AAPL", testkit.Response{Body: `{
		"instrument_id":"US.AAPL",
		"groups":[
			{"kind":"major_holders","static_date":"2026-06-30",
				"items":[{"name":"Vanguard","holder_pct":8.6}]},
			{"kind":"holder_types","static_date":null,
				"items":[{"name":"Institutions","holder_pct":61.2}]}]}`})
	provider, err := NewProvider(server.URL())
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	response, err := provider.Ownership(context.Background(), "US", "AAPL")
	if err != nil {
		t.Fatalf("Ownership: %v", err)
	}
	if len(response.Groups) != 2 || response.Groups[0].Kind != "major_holders" ||
		response.Groups[1].Kind != "holder_types" || response.Source != "yfinance-ownership" {
		t.Fatalf("ownership response = %#v", response)
	}
	if response.Groups[0].StaticDate == nil || *response.Groups[0].StaticDate != "2026-06-30" ||
		response.Groups[1].StaticDate != nil {
		t.Fatalf("static dates = %#v", response.Groups)
	}
	if got := response.Groups[0].Items[0]; got.Name != "Vanguard" ||
		got.HolderPct == nil || *got.HolderPct != "8.6" {
		t.Fatalf("ownership item = %#v", got)
	}
}

func TestProviderCompanyResearchRejectsUnsupportedMarketsWithoutSidecarCall(t *testing.T) {
	server := testkit.New(t)
	provider, err := NewProvider(server.URL())
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	ctx := context.Background()
	if _, err := provider.CompanyProfile(ctx, "SH", "600519"); !errors.Is(err, ErrUnsupported) ||
		!errors.Is(err, marketdata.ErrCapabilityUnsupported) {
		t.Fatalf("SH profile error = %v", err)
	}
	if _, err := provider.FinancialStatements(ctx, "CN", "SH.600519", "income"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("CN financials error = %v", err)
	}
	if _, err := provider.AnalystConsensus(ctx, "SZ", "000001"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("SZ analyst error = %v", err)
	}
	if _, err := provider.Ownership(ctx, "BJ", "430047"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("BJ ownership error = %v", err)
	}
	if _, err := provider.FinancialStatements(ctx, "US", "AAPL", "annual"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unknown statement error = %v", err)
	}
	if len(server.Requests()) != 0 {
		t.Fatalf("rejected requests reached the sidecar: %#v", server.Requests())
	}
}

func TestProviderCompanyResearchMapsSidecarUnsupportedMarket(t *testing.T) {
	server := testkit.New(t)
	server.Queue("/profile/HK/00700", testkit.Response{
		Status: http.StatusBadRequest,
		Body:   `{"error":{"code":"unsupported_market","message":"HK profile is not covered"}}`,
	})
	provider, err := NewProvider(server.URL())
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.CompanyProfile(context.Background(), "HK", "00700"); !errors.Is(err, ErrUnsupported) ||
		!errors.Is(err, marketdata.ErrCapabilityUnsupported) {
		t.Fatalf("sidecar unsupported error = %v", err)
	}
}

func TestProviderCompanyResearchRejectsIdentityMismatch(t *testing.T) {
	server := testkit.New(t)
	server.Queue("/profile/US/AAPL", testkit.Response{Body: `{
		"instrument_id":"US.MSFT","market":"US","symbol":"MSFT","currency":"USD","groups":[]}`})
	provider, err := NewProvider(server.URL())
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.CompanyProfile(context.Background(), "US", "AAPL"); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("identity mismatch error = %v", err)
	}
}
