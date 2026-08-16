package marketdataapp

import (
	"context"
	"fmt"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

// CompanyProfile 向运行时 provider 转发个股画像读取，仅当提供者声明该可选能力。
func (r *Runtime) CompanyProfile(
	ctx context.Context,
	market string,
	symbol string,
) (marketdata.CompanyProfileResponse, error) {
	state := r.snapshot()
	source, ok := state.provider.(marketdata.CompanyProfileSource)
	if !ok {
		return marketdata.CompanyProfileResponse{}, fmt.Errorf(
			"%w: active provider %q does not support company profile",
			marketdata.ErrCapabilityUnsupported, state.providerID,
		)
	}
	return source.CompanyProfile(ctx, market, symbol)
}

// FinancialStatements 向运行时 provider 转发财务报表读取，仅当提供者声明该可选能力。
func (r *Runtime) FinancialStatements(
	ctx context.Context,
	market string,
	symbol string,
	statement string,
) (marketdata.FinancialStatementsResponse, error) {
	state := r.snapshot()
	source, ok := state.provider.(marketdata.FinancialStatementsSource)
	if !ok {
		return marketdata.FinancialStatementsResponse{}, fmt.Errorf(
			"%w: active provider %q does not support financial statements",
			marketdata.ErrCapabilityUnsupported, state.providerID,
		)
	}
	return source.FinancialStatements(ctx, market, symbol, statement)
}

// AnalystConsensus 向运行时 provider 转发分析师共识读取，仅当提供者声明该可选能力。
func (r *Runtime) AnalystConsensus(
	ctx context.Context,
	market string,
	symbol string,
) (marketdata.AnalystConsensusResponse, error) {
	state := r.snapshot()
	source, ok := state.provider.(marketdata.AnalystConsensusSource)
	if !ok {
		return marketdata.AnalystConsensusResponse{}, fmt.Errorf(
			"%w: active provider %q does not support analyst consensus",
			marketdata.ErrCapabilityUnsupported, state.providerID,
		)
	}
	return source.AnalystConsensus(ctx, market, symbol)
}

// Ownership 向运行时 provider 转发股权结构读取，仅当提供者声明该可选能力。
func (r *Runtime) Ownership(
	ctx context.Context,
	market string,
	symbol string,
) (marketdata.OwnershipResponse, error) {
	state := r.snapshot()
	source, ok := state.provider.(marketdata.OwnershipSource)
	if !ok {
		return marketdata.OwnershipResponse{}, fmt.Errorf(
			"%w: active provider %q does not support ownership",
			marketdata.ErrCapabilityUnsupported, state.providerID,
		)
	}
	return source.Ownership(ctx, market, symbol)
}
