package akshare

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/market"
)

var (
	_ marketdata.CompanyProfileSource      = (*Provider)(nil)
	_ marketdata.FinancialStatementsSource = (*Provider)(nil)
	_ marketdata.AnalystConsensusSource    = (*Provider)(nil)
	_ marketdata.OwnershipSource           = (*Provider)(nil)
)

// CompanyProfile returns grouped company profile fields for a CN or HK
// instrument. The AKShare sidecar covers CN/SH/SZ securities and HK company
// profiles; US and BJ are rejected Go-side as ErrUnsupported before any
// sidecar call. Financials/analyst/ownership remain CN-only: the sidecar
// rejects HK with unsupported_market, which classifyCompanyResearchError
// folds into the capability contract.
func (p *Provider) CompanyProfile(
	ctx context.Context,
	marketValue string,
	symbol string,
) (marketdata.CompanyProfileResponse, error) {
	instrument, err := companyResearchInstrument(marketValue, symbol)
	if err != nil {
		return marketdata.CompanyProfileResponse{}, err
	}
	response, err := p.client.companyProfile(ctx, instrument.market, instrument.symbol)
	if err != nil {
		return marketdata.CompanyProfileResponse{}, classifyCompanyResearchError(err)
	}
	converted, err := convertCompanyProfile(response, instrument)
	if err != nil {
		return marketdata.CompanyProfileResponse{}, err
	}
	converted.Source = "akshare-profile"
	return converted, nil
}

// FinancialStatements returns one financial statement table for a CN instrument.
func (p *Provider) FinancialStatements(
	ctx context.Context,
	marketValue string,
	symbol string,
	statement string,
) (marketdata.FinancialStatementsResponse, error) {
	instrument, err := companyResearchInstrument(marketValue, symbol)
	if err != nil {
		return marketdata.FinancialStatementsResponse{}, err
	}
	statement, err = companyStatement(statement)
	if err != nil {
		return marketdata.FinancialStatementsResponse{}, err
	}
	response, err := p.client.financialStatements(ctx, instrument.market, instrument.symbol, statement)
	if err != nil {
		return marketdata.FinancialStatementsResponse{}, classifyCompanyResearchError(err)
	}
	converted, err := convertFinancialStatements(response, instrument, statement)
	if err != nil {
		return marketdata.FinancialStatementsResponse{}, err
	}
	converted.Source = "akshare-financials"
	return converted, nil
}

// AnalystConsensus returns the analyst rating and rating distribution for a CN
// instrument. The sidecar aggregates Eastmoney research reports and publishes
// no price targets, so TargetPrice stays nil.
func (p *Provider) AnalystConsensus(
	ctx context.Context,
	marketValue string,
	symbol string,
) (marketdata.AnalystConsensusResponse, error) {
	instrument, err := companyResearchInstrument(marketValue, symbol)
	if err != nil {
		return marketdata.AnalystConsensusResponse{}, err
	}
	response, err := p.client.analystConsensus(ctx, instrument.market, instrument.symbol)
	if err != nil {
		return marketdata.AnalystConsensusResponse{}, classifyCompanyResearchError(err)
	}
	converted, err := convertAnalystConsensus(response, instrument)
	if err != nil {
		return marketdata.AnalystConsensusResponse{}, err
	}
	converted.Source = "akshare-analyst"
	return converted, nil
}

// Ownership returns major holder and holder-type breakdowns for a CN
// instrument.
func (p *Provider) Ownership(
	ctx context.Context,
	marketValue string,
	symbol string,
) (marketdata.OwnershipResponse, error) {
	instrument, err := companyResearchInstrument(marketValue, symbol)
	if err != nil {
		return marketdata.OwnershipResponse{}, err
	}
	response, err := p.client.ownership(ctx, instrument.market, instrument.symbol)
	if err != nil {
		return marketdata.OwnershipResponse{}, classifyCompanyResearchError(err)
	}
	converted, err := convertOwnership(response, instrument)
	if err != nil {
		return marketdata.OwnershipResponse{}, err
	}
	converted.Source = "akshare-ownership"
	return converted, nil
}

// companyInstrument is the research-scoped identity the company research
// endpoints operate on; unlike normalizeIdentity it accepts the CN aggregate.
type companyInstrument struct {
	market string
	symbol string
}

func companyResearchInstrument(marketValue, symbol string) (companyInstrument, error) {
	canonical, err := companyResearchMarket(marketValue)
	if err != nil {
		return companyInstrument{}, err
	}
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if prefix, code, found := strings.Cut(symbol, "."); found {
		if prefixed, perr := companyResearchMarket(prefix); perr == nil {
			canonical = prefixed
		}
		symbol = code
	}
	if symbol == "" {
		return companyInstrument{}, fmt.Errorf("%w: instrument code is required", ErrUnsupported)
	}
	if canonical == "CN" {
		if parsed, perr := market.ParseInstrument(market.InstrumentInput{Market: "CN", Symbol: symbol}); perr == nil &&
			(parsed.Prefix == "SH" || parsed.Prefix == "SZ") {
			canonical, symbol = parsed.Prefix, parsed.Code
		}
	}
	return companyInstrument{market: canonical, symbol: symbol}, nil
}

// companyResearchMarket accepts the CN aggregate, its SH/SZ leaves, and HK.
// US and BJ (including the BJSE/BSE aliases) stay unsupported: the sidecar
// does not cover either market for stock research.
func companyResearchMarket(marketValue string) (string, error) {
	canonical, err := canonicalMarket(marketValue)
	if err != nil {
		return "", err
	}
	switch canonical {
	case "CN", "SH", "SZ", "HK":
		return canonical, nil
	default:
		return "", fmt.Errorf("%w: company research market %q", ErrUnsupported, marketValue)
	}
}

func companyStatement(statement string) (string, error) {
	switch normalized := strings.ToLower(strings.TrimSpace(statement)); normalized {
	case "":
		return marketdata.StatementIncome, nil
	case marketdata.StatementIncome, marketdata.StatementBalance, marketdata.StatementCashflow:
		return normalized, nil
	default:
		return "", fmt.Errorf("%w: financial statement %q", ErrUnsupported, statement)
	}
}

// classifyCompanyResearchError folds the sidecar 400 unsupported_market code
// into the capability contract alongside the existing AKSHARE_UNSUPPORTED
// classification.
func classifyCompanyResearchError(err error) error {
	var remoteErr *HTTPError
	if errors.As(err, &remoteErr) && strings.EqualFold(strings.TrimSpace(remoteErr.Code), "unsupported_market") {
		return fmt.Errorf("%w: %w", ErrUnsupported, remoteErr)
	}
	return err
}

// verifyCompanySymbol checks the echoed symbol when the sidecar provides one;
// the symbol echo is the stable identity signal across markets.
func verifyCompanySymbol(symbol string, expected companyInstrument) error {
	if symbol == "" {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(symbol), expected.symbol) {
		return fmt.Errorf(
			"%w: company research symbol %q does not match %s",
			ErrInvalidResponse, symbol, expected.symbol,
		)
	}
	return nil
}

// verifyCompanyInstrumentID checks the echoed instrument_id by symbol suffix
// for endpoints whose payload carries no separate symbol field.
func verifyCompanyInstrumentID(instrumentID string, expected companyInstrument) error {
	instrumentID = strings.ToUpper(strings.TrimSpace(instrumentID))
	if instrumentID == "" {
		return nil
	}
	if !strings.HasSuffix(instrumentID, "."+expected.symbol) {
		return fmt.Errorf(
			"%w: company research identity %q does not match %s.%s",
			ErrInvalidResponse, instrumentID, expected.market, expected.symbol,
		)
	}
	return nil
}

func convertCompanyProfile(
	response remoteCompanyProfile,
	expected companyInstrument,
) (marketdata.CompanyProfileResponse, error) {
	if err := verifyCompanySymbol(response.Symbol, expected); err != nil {
		return marketdata.CompanyProfileResponse{}, err
	}
	groups := make([]marketdata.CompanyProfileGroup, 0, len(response.Groups))
	for _, group := range response.Groups {
		fields := make([]marketdata.CompanyProfileField, 0, len(group.Fields))
		for _, field := range group.Fields {
			name, value := strings.TrimSpace(field.Name), strings.TrimSpace(field.Value)
			if name == "" && value == "" {
				continue
			}
			fields = append(fields, marketdata.CompanyProfileField{Name: name, Value: value})
		}
		groups = append(groups, marketdata.CompanyProfileGroup{
			Title: strings.TrimSpace(group.Title), Fields: fields,
		})
	}
	return marketdata.CompanyProfileResponse{
		Market: expected.market, Symbol: expected.symbol,
		InstrumentID: expected.market + "." + expected.symbol,
		Currency:     optionalText(response.Currency), Groups: groups,
	}, nil
}

func convertFinancialStatements(
	response remoteFinancialStatements,
	expected companyInstrument,
	statement string,
) (marketdata.FinancialStatementsResponse, error) {
	if err := verifyCompanyInstrumentID(response.InstrumentID, expected); err != nil {
		return marketdata.FinancialStatementsResponse{}, err
	}
	if echoed := strings.ToLower(strings.TrimSpace(response.Statement)); echoed != "" && echoed != statement {
		return marketdata.FinancialStatementsResponse{}, fmt.Errorf(
			"%w: financial statement %q does not match %q", ErrInvalidResponse, response.Statement, statement,
		)
	}
	fields := make([]marketdata.FinancialStatementField, 0, len(response.Fields))
	for index, field := range response.Fields {
		fieldID := strings.TrimSpace(field.FieldID)
		if fieldID == "" {
			return marketdata.FinancialStatementsResponse{}, fmt.Errorf(
				"%w: financial statement field %d field_id is required", ErrInvalidResponse, index,
			)
		}
		fields = append(fields, marketdata.FinancialStatementField{
			FieldID: fieldID, DisplayName: strings.TrimSpace(field.DisplayName),
		})
	}
	periods := make([]marketdata.FinancialStatementPeriod, 0, len(response.Periods))
	for _, period := range response.Periods {
		values := make(map[string]marketdata.FinancialStatementValue, len(period.Values))
		for fieldID, value := range period.Values {
			values[fieldID] = marketdata.FinancialStatementValue{
				Data: value.Data, YoY: value.YoY, QoQ: value.QoQ,
			}
		}
		periods = append(periods, marketdata.FinancialStatementPeriod{
			PeriodText: strings.TrimSpace(period.PeriodText), Values: values,
		})
	}
	return marketdata.FinancialStatementsResponse{
		Market: expected.market, Symbol: expected.symbol,
		InstrumentID: expected.market + "." + expected.symbol,
		Statement:    statement, Currency: optionalText(response.Currency),
		Fields: fields, Periods: periods,
	}, nil
}

func convertAnalystConsensus(
	response remoteAnalystConsensus,
	expected companyInstrument,
) (marketdata.AnalystConsensusResponse, error) {
	if err := verifyCompanyInstrumentID(response.InstrumentID, expected); err != nil {
		return marketdata.AnalystConsensusResponse{}, err
	}
	result := marketdata.AnalystConsensusResponse{
		Market: expected.market, Symbol: expected.symbol,
		InstrumentID: expected.market + "." + expected.symbol,
		Rating:       response.Rating, AnalystCount: response.AnalystCount,
		UpdateTime: optionalText(response.UpdateTime),
	}
	if target := response.TargetPrice; target != nil {
		result.TargetPrice = &marketdata.AnalystTargetPrice{
			Lowest: target.Lowest, Average: target.Average, Highest: target.Highest,
		}
	}
	if distribution := response.Distribution; distribution != nil {
		result.Distribution = &marketdata.AnalystDistribution{
			StrongBuy: distribution.StrongBuy, Buy: distribution.Buy, Hold: distribution.Hold,
			Underperform: distribution.Underperform, Sell: distribution.Sell,
		}
	}
	return result, nil
}

func convertOwnership(
	response remoteOwnership,
	expected companyInstrument,
) (marketdata.OwnershipResponse, error) {
	groups := make([]marketdata.OwnershipGroup, 0, len(response.Groups))
	for index, group := range response.Groups {
		kind := strings.ToLower(strings.TrimSpace(group.Kind))
		if kind != marketdata.OwnershipGroupMajorHolders && kind != marketdata.OwnershipGroupHolderTypes {
			return marketdata.OwnershipResponse{}, fmt.Errorf(
				"%w: ownership group %d kind %q", ErrInvalidResponse, index, group.Kind,
			)
		}
		items := make([]marketdata.OwnershipItem, 0, len(group.Items))
		for _, item := range group.Items {
			items = append(items, marketdata.OwnershipItem{
				Name: strings.TrimSpace(item.Name), HolderPct: item.HolderPct,
			})
		}
		groups = append(groups, marketdata.OwnershipGroup{
			Kind: kind, StaticDate: optionalText(group.StaticDate), Items: items,
		})
	}
	return marketdata.OwnershipResponse{
		Market: expected.market, Symbol: expected.symbol,
		InstrumentID: expected.market + "." + expected.symbol,
		Groups:       groups,
	}, nil
}
