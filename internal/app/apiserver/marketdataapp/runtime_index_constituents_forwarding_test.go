package marketdataapp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

type indexConstituentsCapableForwardingProviderStub struct {
	forwardingProviderStub
	market  string
	symbol  string
	limit   int
	callErr error
}

func (p *indexConstituentsCapableForwardingProviderStub) IndexConstituents(
	_ context.Context,
	market string,
	symbol string,
	limit int,
) (marketdata.IndexConstituentsResponse, error) {
	p.record("IndexConstituents")
	p.market, p.symbol, p.limit = market, symbol, limit
	return marketdata.IndexConstituentsResponse{
		Market: market, Symbol: symbol, InstrumentID: market + "." + symbol,
		Constituents: []marketdata.IndexConstituent{{Code: "600519", Name: "贵州茅台"}},
		Source:       "stub-index-constituents",
	}, p.callErr
}

func TestRuntimeForwardsIndexConstituentsToCapableActiveProvider(t *testing.T) {
	provider := &indexConstituentsCapableForwardingProviderStub{}
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: provider})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	ctx := t.Context()

	response, err := runtime.IndexConstituents(ctx, "SH", "000300", 300)
	if err != nil || response.InstrumentID != "SH.000300" || response.Source != "stub-index-constituents" ||
		len(response.Constituents) != 1 {
		t.Fatalf("IndexConstituents = %#v, err=%v", response, err)
	}
	if provider.market != "SH" || provider.symbol != "000300" || provider.limit != 300 {
		t.Fatalf("index constituents forwarding = %s/%s/%d", provider.market, provider.symbol, provider.limit)
	}
	if provider.calls["IndexConstituents"] != 1 {
		t.Fatalf("calls = %#v", provider.calls)
	}

	provider.callErr = errors.New("index constituents upstream failed")
	if _, err := runtime.IndexConstituents(ctx, "SH", "000300", 300); !errors.Is(err, provider.callErr) {
		t.Fatalf("IndexConstituents error passthrough = %v", err)
	}
}

func TestRuntimeIndexConstituentsRejectsProvidersWithoutCapability(t *testing.T) {
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: &forwardingProviderStub{}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	_, err = runtime.IndexConstituents(t.Context(), "SH", "000300", 200)
	if !errors.Is(err, marketdata.ErrCapabilityUnsupported) ||
		!strings.Contains(err.Error(), ProviderFutu) || !strings.Contains(err.Error(), "index constituents") {
		t.Fatalf("IndexConstituents unsupported error = %v", err)
	}
}
