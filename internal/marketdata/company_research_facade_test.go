package marketdata

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type companyResearchCapableProviderStub struct {
	dataProviderStub
	profile         CompanyProfileResponse
	profileErr      error
	profileMarket   string
	profileSymbol   string
	statements      FinancialStatementsResponse
	statementsErr   error
	statementMarket string
	statementSymbol string
	statementKind   string
	analyst         AnalystConsensusResponse
	analystErr      error
	analystMarket   string
	analystSymbol   string
	ownership       OwnershipResponse
	ownershipErr    error
	ownershipMarket string
	ownershipSymbol string
}

func (p *companyResearchCapableProviderStub) CompanyProfile(
	_ context.Context, market, symbol string,
) (CompanyProfileResponse, error) {
	p.profileMarket, p.profileSymbol = market, symbol
	return p.profile, p.profileErr
}

func (p *companyResearchCapableProviderStub) FinancialStatements(
	_ context.Context, market, symbol, statement string,
) (FinancialStatementsResponse, error) {
	p.statementMarket, p.statementSymbol, p.statementKind = market, symbol, statement
	return p.statements, p.statementsErr
}

func (p *companyResearchCapableProviderStub) AnalystConsensus(
	_ context.Context, market, symbol string,
) (AnalystConsensusResponse, error) {
	p.analystMarket, p.analystSymbol = market, symbol
	return p.analyst, p.analystErr
}

func (p *companyResearchCapableProviderStub) Ownership(
	_ context.Context, market, symbol string,
) (OwnershipResponse, error) {
	p.ownershipMarket, p.ownershipSymbol = market, symbol
	return p.ownership, p.ownershipErr
}

func TestServiceCompanyResearchRejectsProvidersWithoutCapability(t *testing.T) {
	service := NewService(&dataProviderStub{})
	ctx := context.Background()

	cases := []struct {
		name string
		call func() error
		want string
	}{
		{"profile", func() error {
			_, err := service.GetCompanyProfile(ctx, "US", "AAPL")
			return err
		}, "company profile"},
		{"financials", func() error {
			_, err := service.GetFinancialStatements(ctx, "US", "AAPL", "income")
			return err
		}, "financial statements"},
		{"analyst", func() error {
			_, err := service.GetAnalystConsensus(ctx, "US", "AAPL")
			return err
		}, "analyst consensus"},
		{"ownership", func() error {
			_, err := service.GetOwnership(ctx, "US", "AAPL")
			return err
		}, "ownership"},
	}
	for _, tc := range cases {
		err := tc.call()
		if !errors.Is(err, ErrCapabilityUnsupported) || !strings.Contains(err.Error(), "stub-provider") ||
			!strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s unsupported error = %v", tc.name, err)
		}
	}
}

func TestServiceCompanyResearchRequiresMarketAndSymbol(t *testing.T) {
	service := NewService(&companyResearchCapableProviderStub{})
	ctx := context.Background()

	if _, err := service.GetCompanyProfile(ctx, "", "AAPL"); err == nil {
		t.Fatal("empty market must fail")
	}
	if _, err := service.GetOwnership(ctx, "US", "  "); err == nil {
		t.Fatal("empty symbol must fail")
	}
	if _, err := service.GetAnalystConsensus(ctx, "", ""); err == nil {
		t.Fatal("empty instrument must fail")
	}
}

func TestServiceFinancialStatementsValidatesStatementAndDefaultsToIncome(t *testing.T) {
	provider := &companyResearchCapableProviderStub{
		statements: FinancialStatementsResponse{
			Market: "US", Symbol: "AAPL", InstrumentID: "US.AAPL", Statement: "income",
			Fields: []FinancialStatementField{{FieldID: "revenue", DisplayName: "Revenue"}},
		},
	}
	service := NewService(provider)
	ctx := context.Background()

	if _, err := service.GetFinancialStatements(ctx, "US", "AAPL", "annual"); err == nil ||
		!strings.Contains(err.Error(), "statement") {
		t.Fatalf("invalid statement error = %v", err)
	}
	response, err := service.GetFinancialStatements(ctx, "us", "aapl", "")
	if err != nil {
		t.Fatalf("GetFinancialStatements: %v", err)
	}
	if provider.statementMarket != "US" || provider.statementSymbol != "AAPL" ||
		provider.statementKind != StatementIncome {
		t.Fatalf("statements forwarding = %s/%s/%s",
			provider.statementMarket, provider.statementSymbol, provider.statementKind)
	}
	if response.Statement != "income" || len(response.Fields) != 1 {
		t.Fatalf("statements response = %#v", response)
	}
	if _, err := service.GetFinancialStatements(ctx, "US", "AAPL", " Balance "); err != nil {
		t.Fatalf("balance statement: %v", err)
	}
	if provider.statementKind != StatementBalance {
		t.Fatalf("statement kind = %q", provider.statementKind)
	}
}

func TestServiceCompanyResearchResolvesChinaAggregateToExchangeLeaf(t *testing.T) {
	provider := &companyResearchCapableProviderStub{
		profile: CompanyProfileResponse{Market: "SH", Symbol: "600519", InstrumentID: "SH.600519"},
	}
	service := NewService(provider)
	if _, err := service.GetCompanyProfile(context.Background(), "CN", "SH.600519"); err != nil {
		t.Fatalf("GetCompanyProfile CN aggregate: %v", err)
	}
	if provider.profileMarket != "SH" || provider.profileSymbol != "600519" {
		t.Fatalf("profile request = %s/%s", provider.profileMarket, provider.profileSymbol)
	}
}

func TestServiceCompanyResearchPassesProviderErrorsThrough(t *testing.T) {
	provider := &companyResearchCapableProviderStub{}
	service := NewService(provider)
	providerErr := errors.New("company research upstream failed")
	ctx := context.Background()

	provider.analystErr = providerErr
	if _, err := service.GetAnalystConsensus(ctx, "US", "AAPL"); !errors.Is(err, providerErr) {
		t.Fatalf("analyst error passthrough = %v", err)
	}
	provider.ownershipErr = providerErr
	if _, err := service.GetOwnership(ctx, "US", "AAPL"); !errors.Is(err, providerErr) {
		t.Fatalf("ownership error passthrough = %v", err)
	}
}
