package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Financial statement kinds accepted by the embedded providers.
const (
	StatementIncome   = "income"
	StatementBalance  = "balance"
	StatementCashflow = "cashflow"
)

// Ownership group kinds accepted by the embedded providers.
const (
	OwnershipGroupMajorHolders = "major_holders"
	OwnershipGroupHolderTypes  = "holder_types"
)

// CompanyProfileSource is an optional provider capability that supplies grouped
// company profile fields for an instrument.
type CompanyProfileSource interface {
	CompanyProfile(ctx context.Context, market, symbol string) (CompanyProfileResponse, error)
}

// FinancialStatementsSource is an optional provider capability that supplies
// tabular financial statements (income, balance sheet, cashflow).
type FinancialStatementsSource interface {
	FinancialStatements(ctx context.Context, market, symbol, statement string) (FinancialStatementsResponse, error)
}

// AnalystConsensusSource is an optional provider capability that supplies
// analyst rating, target price, and rating distribution for an instrument.
type AnalystConsensusSource interface {
	AnalystConsensus(ctx context.Context, market, symbol string) (AnalystConsensusResponse, error)
}

// OwnershipSource is an optional provider capability that supplies major
// holder and holder-type breakdowns for an instrument.
type OwnershipSource interface {
	Ownership(ctx context.Context, market, symbol string) (OwnershipResponse, error)
}

// CompanyProfileField is one provider-neutral name/value profile row.
type CompanyProfileField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// CompanyProfileGroup groups profile fields under a section title.
type CompanyProfileGroup struct {
	Title  string                `json:"title"`
	Fields []CompanyProfileField `json:"fields"`
}

// CompanyProfileResponse is the provider-neutral company profile payload.
type CompanyProfileResponse struct {
	Market       string                `json:"market"`
	Symbol       string                `json:"symbol"`
	InstrumentID string                `json:"instrumentId"`
	Currency     *string               `json:"currency"`
	Groups       []CompanyProfileGroup `json:"groups"`
	Source       string                `json:"source"`
}

// FinancialStatementField is one column of a financial statement table.
type FinancialStatementField struct {
	FieldID     string `json:"fieldId"`
	DisplayName string `json:"displayName"`
}

// FinancialStatementValue is one cell; YoY/QoQ are nullable because upstream
// feeds do not always publish comparisons.
type FinancialStatementValue struct {
	Data *json.Number `json:"data"`
	YoY  *json.Number `json:"yoy"`
	QoQ  *json.Number `json:"qoq"`
}

// FinancialStatementPeriod is one report-period row keyed by field id.
type FinancialStatementPeriod struct {
	PeriodText string                             `json:"periodText"`
	Values     map[string]FinancialStatementValue `json:"values"`
}

// FinancialStatementsResponse is the provider-neutral statement payload.
type FinancialStatementsResponse struct {
	Market       string                     `json:"market"`
	Symbol       string                     `json:"symbol"`
	InstrumentID string                     `json:"instrumentId"`
	Statement    string                     `json:"statement" enums:"income,balance,cashflow"`
	Currency     *string                    `json:"currency"`
	Fields       []FinancialStatementField  `json:"fields"`
	Periods      []FinancialStatementPeriod `json:"periods"`
	Source       string                     `json:"source"`
}

// AnalystTargetPrice is the provider-neutral target price range.
type AnalystTargetPrice struct {
	Lowest  *json.Number `json:"lowest"`
	Average *json.Number `json:"average"`
	Highest *json.Number `json:"highest"`
}

// AnalystDistribution is the rating distribution in percent (0-100); every
// bucket is nullable because upstream feeds do not guarantee full coverage.
type AnalystDistribution struct {
	StrongBuy    *json.Number `json:"strongBuy"`
	Buy          *json.Number `json:"buy"`
	Hold         *json.Number `json:"hold"`
	Underperform *json.Number `json:"underperform"`
	Sell         *json.Number `json:"sell"`
}

// AnalystConsensusResponse is the provider-neutral analyst consensus payload.
type AnalystConsensusResponse struct {
	Market       string               `json:"market"`
	Symbol       string               `json:"symbol"`
	InstrumentID string               `json:"instrumentId"`
	Rating       *json.Number         `json:"rating"`
	AnalystCount *json.Number         `json:"analystCount"`
	TargetPrice  *AnalystTargetPrice  `json:"targetPrice"`
	Distribution *AnalystDistribution `json:"distribution"`
	UpdateTime   *string              `json:"updateTime"`
	Source       string               `json:"source"`
}

// OwnershipItem is one holder or holder-type row with its stake in percent.
type OwnershipItem struct {
	Name      string       `json:"name"`
	HolderPct *json.Number `json:"holderPct"`
}

// OwnershipGroup is one ownership section (major holders or holder types).
type OwnershipGroup struct {
	Kind       string          `json:"kind" enums:"major_holders,holder_types"`
	StaticDate *string         `json:"staticDate"`
	Items      []OwnershipItem `json:"items"`
}

// OwnershipResponse is the provider-neutral ownership payload.
type OwnershipResponse struct {
	Market       string           `json:"market"`
	Symbol       string           `json:"symbol"`
	InstrumentID string           `json:"instrumentId"`
	Groups       []OwnershipGroup `json:"groups"`
	Source       string           `json:"source"`
}

// GetCompanyProfile 返回当前行情提供者的公司资料分组。
func (s *Service) GetCompanyProfile(
	ctx context.Context,
	market string,
	symbol string,
) (CompanyProfileResponse, error) {
	s.providerLifecycleMu.RLock()
	defer s.providerLifecycleMu.RUnlock()
	source, err := s.companyProfileSource(ctx)
	if err != nil {
		return CompanyProfileResponse{}, err
	}
	market, symbol, err = requireInstrument(market, symbol)
	if err != nil {
		return CompanyProfileResponse{}, err
	}
	market, symbol = normalizeCNAggregateRead(market, symbol)
	return source.CompanyProfile(ctx, market, symbol)
}

// GetFinancialStatements 返回当前行情提供者的财务报表明细。
func (s *Service) GetFinancialStatements(
	ctx context.Context,
	market string,
	symbol string,
	statement string,
) (FinancialStatementsResponse, error) {
	s.providerLifecycleMu.RLock()
	defer s.providerLifecycleMu.RUnlock()
	source, err := s.financialStatementsSource(ctx)
	if err != nil {
		return FinancialStatementsResponse{}, err
	}
	market, symbol, err = requireInstrument(market, symbol)
	if err != nil {
		return FinancialStatementsResponse{}, err
	}
	statement = strings.ToLower(strings.TrimSpace(statement))
	if statement == "" {
		statement = StatementIncome
	}
	switch statement {
	case StatementIncome, StatementBalance, StatementCashflow:
	default:
		return FinancialStatementsResponse{}, fmt.Errorf(
			"financial statement must be one of income, balance, cashflow",
		)
	}
	market, symbol = normalizeCNAggregateRead(market, symbol)
	return source.FinancialStatements(ctx, market, symbol, statement)
}

// GetAnalystConsensus 返回当前行情提供者的分析师评级共识。
func (s *Service) GetAnalystConsensus(
	ctx context.Context,
	market string,
	symbol string,
) (AnalystConsensusResponse, error) {
	s.providerLifecycleMu.RLock()
	defer s.providerLifecycleMu.RUnlock()
	source, err := s.analystConsensusSource(ctx)
	if err != nil {
		return AnalystConsensusResponse{}, err
	}
	market, symbol, err = requireInstrument(market, symbol)
	if err != nil {
		return AnalystConsensusResponse{}, err
	}
	market, symbol = normalizeCNAggregateRead(market, symbol)
	return source.AnalystConsensus(ctx, market, symbol)
}

// GetOwnership 返回当前行情提供者的股东结构。
func (s *Service) GetOwnership(
	ctx context.Context,
	market string,
	symbol string,
) (OwnershipResponse, error) {
	s.providerLifecycleMu.RLock()
	defer s.providerLifecycleMu.RUnlock()
	source, err := s.ownershipSource(ctx)
	if err != nil {
		return OwnershipResponse{}, err
	}
	market, symbol, err = requireInstrument(market, symbol)
	if err != nil {
		return OwnershipResponse{}, err
	}
	market, symbol = normalizeCNAggregateRead(market, symbol)
	return source.Ownership(ctx, market, symbol)
}

func requireInstrument(market, symbol string) (string, string, error) {
	market = strings.ToUpper(strings.TrimSpace(market))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if market == "" || symbol == "" {
		return "", "", fmt.Errorf("company research requires both market and symbol")
	}
	return market, symbol, nil
}

func (s *Service) companyProfileSource(ctx context.Context) (CompanyProfileSource, error) {
	if source, ok := s.provider.(CompanyProfileSource); ok {
		return source, nil
	}
	return nil, s.optionalCapabilityError(ctx, "company profile")
}

func (s *Service) financialStatementsSource(ctx context.Context) (FinancialStatementsSource, error) {
	if source, ok := s.provider.(FinancialStatementsSource); ok {
		return source, nil
	}
	return nil, s.optionalCapabilityError(ctx, "financial statements")
}

func (s *Service) analystConsensusSource(ctx context.Context) (AnalystConsensusSource, error) {
	if source, ok := s.provider.(AnalystConsensusSource); ok {
		return source, nil
	}
	return nil, s.optionalCapabilityError(ctx, "analyst consensus")
}

func (s *Service) ownershipSource(ctx context.Context) (OwnershipSource, error) {
	if source, ok := s.provider.(OwnershipSource); ok {
		return source, nil
	}
	return nil, s.optionalCapabilityError(ctx, "ownership")
}
