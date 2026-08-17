package marketdataapp

import (
	"context"
	"fmt"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

// Screen 向运行时 provider 转发股票筛选读取，仅当提供者声明该可选能力。
func (r *Runtime) Screen(
	ctx context.Context,
	req marketdata.ScreenRequest,
) (marketdata.ScreenResponse, error) {
	state := r.snapshot()
	source, ok := state.provider.(marketdata.ScreenerSource)
	if !ok {
		return marketdata.ScreenResponse{}, fmt.Errorf(
			"%w: active provider %q does not support stock screen",
			marketdata.ErrCapabilityUnsupported, state.providerID,
		)
	}
	return source.Screen(ctx, req)
}
