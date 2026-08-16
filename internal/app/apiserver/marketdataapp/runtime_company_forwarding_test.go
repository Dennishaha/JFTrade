package marketdataapp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

// companyResearchStub 覆盖全部四项公司研究读取，供运行时转发测试使用。
type companyResearchStub struct {
	forwardingProviderStub
	lastInstrument string
	lastStatement  string
	companyErr     error
}

func (p *companyResearchStub) CompanyProfile(_ context.Context, market, symbol string) (marketdata.CompanyProfileResponse, error) {
	p.record("company-profile")
	p.lastInstrument = market + "." + symbol
	if p.companyErr != nil {
		return marketdata.CompanyProfileResponse{}, p.companyErr
	}
	return marketdata.CompanyProfileResponse{Source: "stub", Groups: []marketdata.CompanyProfileGroup{{Title: "概要"}}}, nil
}

func (p *companyResearchStub) FinancialStatements(_ context.Context, market, symbol, statement string) (marketdata.FinancialStatementsResponse, error) {
	p.record("financial-statements")
	p.lastInstrument = market + "." + symbol
	p.lastStatement = statement
	if p.companyErr != nil {
		return marketdata.FinancialStatementsResponse{}, p.companyErr
	}
	return marketdata.FinancialStatementsResponse{Source: "stub"}, nil
}

func (p *companyResearchStub) AnalystConsensus(_ context.Context, market, symbol string) (marketdata.AnalystConsensusResponse, error) {
	p.record("analyst-consensus")
	p.lastInstrument = market + "." + symbol
	if p.companyErr != nil {
		return marketdata.AnalystConsensusResponse{}, p.companyErr
	}
	return marketdata.AnalystConsensusResponse{Source: "stub"}, nil
}

func (p *companyResearchStub) Ownership(_ context.Context, market, symbol string) (marketdata.OwnershipResponse, error) {
	p.record("ownership")
	p.lastInstrument = market + "." + symbol
	if p.companyErr != nil {
		return marketdata.OwnershipResponse{}, p.companyErr
	}
	return marketdata.OwnershipResponse{Source: "stub"}, nil
}

func TestRuntimeCompanyResearchForwarding(t *testing.T) {
	provider := &companyResearchStub{}
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: provider})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	ctx := context.Background()

	if _, err := runtime.CompanyProfile(ctx, "US", "AAPL"); err != nil {
		t.Fatalf("CompanyProfile: %v", err)
	}
	if got := provider.calls["company-profile"]; got != 1 {
		t.Fatalf("expected company-profile call, got %d", got)
	}

	if _, err := runtime.FinancialStatements(ctx, "SH", "600519", marketdata.StatementCashflow); err != nil {
		t.Fatalf("FinancialStatements: %v", err)
	}
	if provider.lastInstrument != "SH.600519" {
		t.Fatalf("expected instrument forwarded, got %q", provider.lastInstrument)
	}
	if provider.lastStatement != marketdata.StatementCashflow {
		t.Fatalf("expected statement forwarded, got %q", provider.lastStatement)
	}

	if _, err := runtime.AnalystConsensus(ctx, "US", "AAPL"); err != nil {
		t.Fatalf("AnalystConsensus: %v", err)
	}
	if _, err := runtime.Ownership(ctx, "US", "AAPL"); err != nil {
		t.Fatalf("Ownership: %v", err)
	}
	for _, method := range []string{"financial-statements", "analyst-consensus", "ownership"} {
		if got := provider.calls[method]; got != 1 {
			t.Fatalf("expected %s call, got %d", method, got)
		}
	}
}

func TestRuntimeCompanyResearchPropagatesError(t *testing.T) {
	want := errors.New("research failed")
	provider := &companyResearchStub{companyErr: want}
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: provider})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	if _, err := runtime.CompanyProfile(context.Background(), "US", "AAPL"); !errors.Is(err, want) {
		t.Fatalf("expected provider error, got %v", err)
	}
}

func TestRuntimeCompanyResearchCapabilityUnsupported(t *testing.T) {
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: &forwardingProviderStub{}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	ctx := context.Background()

	cases := []struct {
		name string
		call func() error
		want string
	}{
		{"profile", func() error {
			_, err := runtime.CompanyProfile(ctx, "US", "AAPL")
			return err
		}, "company profile"},
		{"financials", func() error {
			_, err := runtime.FinancialStatements(ctx, "US", "AAPL", marketdata.StatementIncome)
			return err
		}, "financial statements"},
		{"analyst", func() error {
			_, err := runtime.AnalystConsensus(ctx, "US", "AAPL")
			return err
		}, "analyst consensus"},
		{"ownership", func() error {
			_, err := runtime.Ownership(ctx, "US", "AAPL")
			return err
		}, "ownership"},
	}
	for _, tc := range cases {
		err := tc.call()
		if err == nil {
			t.Fatalf("%s: expected capability error", tc.name)
		}
		if !errors.Is(err, marketdata.ErrCapabilityUnsupported) {
			t.Fatalf("%s: expected ErrCapabilityUnsupported, got %v", tc.name, err)
		}
		if got := err.Error(); !strings.Contains(got, ProviderFutu) || !strings.Contains(got, tc.want) {
			t.Fatalf("%s: unexpected error message: %q", tc.name, got)
		}
	}
}
