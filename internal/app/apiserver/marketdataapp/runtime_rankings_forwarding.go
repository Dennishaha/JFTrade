package marketdataapp

import (
	"context"
	"fmt"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

// Rankings forwards market ranking reads to the active provider only when it
// offers the optional capability.
func (r *Runtime) Rankings(
	ctx context.Context,
	market string,
	kind string,
	limit int,
) (marketdata.RankingsResponse, error) {
	state := r.snapshot()
	source, ok := state.provider.(marketdata.RankingsSource)
	if !ok {
		return marketdata.RankingsResponse{}, fmt.Errorf(
			"%w: active provider %q does not support market rankings",
			marketdata.ErrCapabilityUnsupported, state.providerID,
		)
	}
	return source.Rankings(ctx, market, kind, limit)
}

// Industries forwards CN industry/concept board reads to the active provider
// only when it offers the optional capability.
func (r *Runtime) Industries(ctx context.Context, kind string) (marketdata.IndustryBoardsResponse, error) {
	state := r.snapshot()
	source, ok := state.provider.(marketdata.IndustrySource)
	if !ok {
		return marketdata.IndustryBoardsResponse{}, fmt.Errorf(
			"%w: active provider %q does not support industry boards",
			marketdata.ErrCapabilityUnsupported, state.providerID,
		)
	}
	return source.Industries(ctx, kind)
}

// IndustryMembers forwards CN board membership reads to the active provider
// only when it offers the optional capability.
func (r *Runtime) IndustryMembers(
	ctx context.Context,
	kind string,
	board string,
	limit int,
) (marketdata.IndustryMembersResponse, error) {
	state := r.snapshot()
	source, ok := state.provider.(marketdata.IndustrySource)
	if !ok {
		return marketdata.IndustryMembersResponse{}, fmt.Errorf(
			"%w: active provider %q does not support industry boards",
			marketdata.ErrCapabilityUnsupported, state.providerID,
		)
	}
	return source.IndustryMembers(ctx, kind, board, limit)
}
