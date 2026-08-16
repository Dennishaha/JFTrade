package marketdata

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type indexConstituentsCapableProviderStub struct {
	dataProviderStub
	response  IndexConstituentsResponse
	err       error
	market    string
	symbol    string
	limit     int
	callCount int
}

func (p *indexConstituentsCapableProviderStub) IndexConstituents(
	_ context.Context,
	market string,
	symbol string,
	limit int,
) (IndexConstituentsResponse, error) {
	p.market, p.symbol, p.limit = market, symbol, limit
	p.callCount++
	return p.response, p.err
}

func TestServiceIndexConstituentsRejectsProvidersWithoutCapability(t *testing.T) {
	service := NewService(&dataProviderStub{})

	_, err := service.GetIndexConstituents(context.Background(), "SH", "000300", 200)
	if !errors.Is(err, ErrCapabilityUnsupported) || !strings.Contains(err.Error(), "stub-provider") ||
		!strings.Contains(err.Error(), "index constituents") {
		t.Fatalf("GetIndexConstituents unsupported error = %v", err)
	}
}

func TestServiceIndexConstituentsValidatesLimitAndForwardsArguments(t *testing.T) {
	provider := &indexConstituentsCapableProviderStub{
		response: IndexConstituentsResponse{
			Market: "SH", Symbol: "000300", InstrumentID: "SH.000300",
			Constituents: []IndexConstituent{{Code: "600519", Name: "贵州茅台"}},
			Source:       "akshare-index-constituents",
		},
	}
	service := NewService(provider)
	ctx := context.Background()

	response, err := service.GetIndexConstituents(ctx, "sh", "000300", 0)
	if err != nil {
		t.Fatalf("GetIndexConstituents: %v", err)
	}
	if response.InstrumentID != "SH.000300" || provider.limit != DefaultIndexConstituentsLimit {
		t.Fatalf("GetIndexConstituents forwarding = %s/%s/%d, response = %#v",
			provider.market, provider.symbol, provider.limit, response)
	}
	for _, limit := range []int{-1, MaxIndexConstituentsLimit + 1} {
		if _, err := service.GetIndexConstituents(ctx, "SH", "000300", limit); err == nil ||
			!strings.Contains(err.Error(), "limit") {
			t.Fatalf("GetIndexConstituents limit %d error = %v", limit, err)
		}
	}
	providerErr := errors.New("upstream unavailable")
	provider.err = providerErr
	if _, err := service.GetIndexConstituents(ctx, "SH", "000300", 100); !errors.Is(err, providerErr) {
		t.Fatalf("provider error passthrough = %v", err)
	}
}

func TestServiceIndexConstituentsResolvesChinaAggregateToExchangeLeaf(t *testing.T) {
	provider := &indexConstituentsCapableProviderStub{
		response: IndexConstituentsResponse{
			Market: "SH", Symbol: "000300", InstrumentID: "SH.000300",
			Constituents: []IndexConstituent{},
		},
	}
	service := NewService(provider)
	if _, err := service.GetIndexConstituents(context.Background(), "CN", "SH.000300", 50); err != nil {
		t.Fatalf("GetIndexConstituents CN aggregate: %v", err)
	}
	if provider.market != "SH" || provider.symbol != "000300" || provider.limit != 50 {
		t.Fatalf("index constituents request = %s/%s/%d", provider.market, provider.symbol, provider.limit)
	}
}
