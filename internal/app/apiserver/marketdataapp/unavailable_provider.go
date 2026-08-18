package marketdataapp

import (
	"context"
	"errors"
	"fmt"

	akshareintegration "github.com/jftrade/jftrade-main/internal/integration/akshare"
	yfinanceintegration "github.com/jftrade/jftrade-main/internal/integration/yfinance"
	"github.com/jftrade/jftrade-main/internal/marketdata"
)

// MarkProviderUnavailable keeps a configured Python provider visible without
// exposing the initial Futu runtime when helper activation fails during app
// startup. The state is intentionally not placed in providerPool so a later
// activation retries helper startup instead of reusing the failed placeholder.
func (r *Runtime) MarkProviderUnavailable(providerID string, activationErr error) {
	if r == nil {
		return
	}
	providerID = normalizeProviderID(providerID)
	if providerID == ProviderFutu || !isPythonProvider(providerID) {
		return
	}
	r.switchMu.Lock()
	defer r.switchMu.Unlock()
	if r.closed {
		return
	}
	state := runtimeState{
		providerID: providerID,
		provider:   newUnavailableProvider(providerID, activationErr),
	}
	r.mu.Lock()
	r.active = state
	if r.unavailable == nil {
		r.unavailable = make(map[string]struct{})
	}
	r.unavailable[providerID] = struct{}{}
	r.mu.Unlock()
}

// NeedsProviderActivation reports whether a same-provider settings write
// should retry a failed startup activation.
func (r *Runtime) NeedsProviderActivation(providerID string) bool {
	if r == nil {
		return false
	}
	providerID = normalizeProviderID(providerID)
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, unavailable := r.unavailable[providerID]
	return unavailable && r.active.providerID == providerID
}

func newUnavailableProvider(providerID string, activationErr error) marketdata.Provider {
	if activationErr == nil {
		activationErr = errors.New("provider activation failed")
	}
	providerErr := fmt.Errorf(
		"%s market-data provider is unavailable: %w",
		providerID,
		activationErr,
	)
	return NewProvider(ProviderOptions{
		Descriptor: func(context.Context) (marketdata.ProviderDescriptor, error) {
			return unavailableProviderDescriptor(providerID), nil
		},
		Markets: func(context.Context) ([]marketdata.MarketProfile, error) {
			return nil, providerErr
		},
		NormalizeInstrument: func(context.Context, map[string]any) (map[string]any, error) {
			return nil, providerErr
		},
		SecurityDetails: func(context.Context, string, string) (marketdata.SecurityDetails, error) {
			return nil, providerErr
		},
		LookupInstrument: func(context.Context, string, string) ([]marketdata.InstrumentCandidate, error) {
			return nil, providerErr
		},
		SearchInstruments: func(context.Context, string, int) ([]marketdata.InstrumentCandidate, error) {
			return nil, providerErr
		},
		QuerySnapshot: func(context.Context, string) (*marketdata.Tick, error) {
			return nil, providerErr
		},
		QueryTicker: func(context.Context, string) (*marketdata.Tick, error) {
			return nil, providerErr
		},
		HistoricalCandles: func(context.Context, marketdata.HistoricalCandlesQuery) (marketdata.CandlesResponse, error) {
			return nil, providerErr
		},
		Depth: func(context.Context, string, string, int) (marketdata.DepthResponse, error) {
			return nil, providerErr
		},
		Health: func(context.Context) (marketdata.HealthStatus, error) {
			return marketdata.HealthStatus{
				Connected:  false,
				StreamMode: "snapshot-poll-delayed",
				Readiness:  marketdata.ProviderReadinessFailed,
				LastError:  providerErr.Error(),
			}, nil
		},
	})
}

func unavailableProviderDescriptor(providerID string) marketdata.ProviderDescriptor {
	switch normalizeProviderID(providerID) {
	case ProviderYFinance:
		return yfinanceintegration.ProviderDescriptor()
	case ProviderAKShare:
		return akshareintegration.ProviderDescriptor()
	default:
		return marketdata.ProviderDescriptor{
			SelectionID: providerID,
			ProviderID:  providerID,
			DisplayName: providerID,
			Source:      providerID,
		}
	}
}
