package yfinance

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

var (
	_ marketdata.CompanyProfileSource      = (*Provider)(nil)
	_ marketdata.FinancialStatementsSource = (*Provider)(nil)
	_ marketdata.AnalystConsensusSource    = (*Provider)(nil)
	_ marketdata.OwnershipSource           = (*Provider)(nil)
)

// CompanyProfile returns grouped company profile fields. Yahoo Finance covers
// US and HK instruments; other markets are rejected Go-side as ErrUnsupported
// before any sidecar call.
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
	converted.Source = "yfinance-profile"
	return converted, nil
}

// FinancialStatements returns one financial statement table for an instrument.
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
	converted.Source = "yfinance-financials"
	return converted, nil
}

// AnalystConsensus returns the analyst rating, target price, and distribution.
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
	if err := verifyCompanyIdentity(response.InstrumentID, instrument); err != nil {
		return marketdata.AnalystConsensusResponse{}, err
	}
	return marketdata.AnalystConsensusResponse{
		Market: instrument.market, Symbol: instrument.symbol, InstrumentID: instrument.id,
		Rating: response.Rating, AnalystCount: response.AnalystCount,
		TargetPrice:  convertAnalystTargetPrice(response.TargetPrice),
		Distribution: convertAnalystDistribution(response.Distribution),
		UpdateTime:   optionalText(response.UpdateTime),
		Source:       "yfinance-analyst",
	}, nil
}

// Ownership returns major holder and holder-type breakdowns.
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
	converted.Source = "yfinance-ownership"
	return converted, nil
}

// companyResearchInstrument validates market and symbol, rejecting markets the
// Yahoo Finance company-research endpoints do not cover.
func companyResearchInstrument(marketValue, symbol string) (normalizedInstrument, error) {
	instrument, err := normalizeIdentity(marketValue, symbol, "")
	if err != nil {
		return normalizedInstrument{}, err
	}
	if instrument.market != yfinanceDefaultMarket && instrument.market != "HK" {
		return normalizedInstrument{}, fmt.Errorf(
			"%w: company research market %q", ErrUnsupported, instrument.market,
		)
	}
	return instrument, nil
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
// into the capability contract; the yfinance client only classifies warming.
func classifyCompanyResearchError(err error) error {
	var remoteErr *HTTPError
	if errors.As(err, &remoteErr) && strings.EqualFold(strings.TrimSpace(remoteErr.Code), "unsupported_market") {
		return fmt.Errorf("%w: %w", ErrUnsupported, remoteErr)
	}
	return err
}

func verifyCompanyIdentity(instrumentID string, expected normalizedInstrument) error {
	identity, err := normalizeIdentity("", "", instrumentID)
	if err != nil || identity.id != expected.id {
		return fmt.Errorf(
			"%w: company research identity %q does not match %s",
			ErrInvalidResponse, instrumentID, expected.id,
		)
	}
	return nil
}

func convertCompanyProfile(
	response remoteCompanyProfile,
	expected normalizedInstrument,
) (marketdata.CompanyProfileResponse, error) {
	if err := verifyCompanyIdentity(response.InstrumentID, expected); err != nil {
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
		Market: expected.market, Symbol: expected.symbol, InstrumentID: expected.id,
		Currency: optionalText(response.Currency), Groups: groups,
	}, nil
}

func convertFinancialStatements(
	response remoteFinancialStatements,
	expected normalizedInstrument,
	statement string,
) (marketdata.FinancialStatementsResponse, error) {
	if err := verifyCompanyIdentity(response.InstrumentID, expected); err != nil {
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
		Market: expected.market, Symbol: expected.symbol, InstrumentID: expected.id,
		Statement: statement, Currency: optionalText(response.Currency),
		Fields: fields, Periods: periods,
	}, nil
}

func convertAnalystTargetPrice(remote *remoteAnalystTargetPrice) *marketdata.AnalystTargetPrice {
	if remote == nil {
		return nil
	}
	return &marketdata.AnalystTargetPrice{
		Lowest: remote.Lowest, Average: remote.Average, Highest: remote.Highest,
	}
}

func convertAnalystDistribution(remote *remoteAnalystDistribution) *marketdata.AnalystDistribution {
	if remote == nil {
		return nil
	}
	return &marketdata.AnalystDistribution{
		StrongBuy: remote.StrongBuy, Buy: remote.Buy, Hold: remote.Hold,
		Underperform: remote.Underperform, Sell: remote.Sell,
	}
}

func convertOwnership(
	response remoteOwnership,
	expected normalizedInstrument,
) (marketdata.OwnershipResponse, error) {
	if err := verifyCompanyIdentity(response.InstrumentID, expected); err != nil {
		return marketdata.OwnershipResponse{}, err
	}
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
		Market: expected.market, Symbol: expected.symbol, InstrumentID: expected.id, Groups: groups,
	}, nil
}
