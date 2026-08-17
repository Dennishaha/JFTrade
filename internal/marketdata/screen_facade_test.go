package marketdata

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type screenCapableProviderStub struct {
	dataProviderStub
	response ScreenResponse
	err      error
	lastReq  ScreenRequest
}

func (p *screenCapableProviderStub) Screen(_ context.Context, req ScreenRequest) (ScreenResponse, error) {
	p.lastReq = req
	return p.response, p.err
}

func TestServiceScreenRejectsProvidersWithoutCapability(t *testing.T) {
	service := NewService(&dataProviderStub{})
	_, err := service.GetScreen(context.Background(), ScreenRequest{Market: "US"})
	if !errors.Is(err, ErrCapabilityUnsupported) || !strings.Contains(err.Error(), "stock screen") {
		t.Fatalf("unsupported error = %v", err)
	}
}

func TestServiceScreenValidatesRequestAndForwards(t *testing.T) {
	lower := json.Number("10")
	provider := &screenCapableProviderStub{
		response: ScreenResponse{Total: 1, Source: "stub",
			Entries: []ScreenEntry{{InstrumentID: "US.AAPL"}}},
	}
	service := NewService(provider)
	ctx := context.Background()

	if _, err := service.GetScreen(ctx, ScreenRequest{}); err == nil {
		t.Fatal("empty market must fail")
	}
	if _, err := service.GetScreen(ctx, ScreenRequest{
		Market: "US", Conditions: []ScreenConditionRequest{{FactorKey: "simple.price"}},
	}); err == nil {
		t.Fatal("condition without bounds must fail")
	}
	if _, err := service.GetScreen(ctx, ScreenRequest{
		Market: "US", Sorts: []ScreenSortRequest{{FactorKey: "simple.price", Direction: "abs_desc"}},
	}); err == nil {
		t.Fatal("abs sort direction must fail")
	}
	if _, err := service.GetScreen(ctx, ScreenRequest{Market: "US", Offset: -1}); err == nil {
		t.Fatal("negative offset must fail")
	}
	if _, err := service.GetScreen(ctx, ScreenRequest{Market: "US", Limit: 500}); err == nil {
		t.Fatal("out-of-range limit must fail")
	}

	response, err := service.GetScreen(ctx, ScreenRequest{
		Market:     " us ",
		Conditions: []ScreenConditionRequest{{FactorKey: " Simple.PE_TTM ", Min: &lower}},
		Sorts:      []ScreenSortRequest{{FactorKey: "simple.price"}},
	})
	if err != nil {
		t.Fatalf("GetScreen: %v", err)
	}
	forwarded := provider.lastReq
	if forwarded.Market != "US" || forwarded.Conditions[0].FactorKey != "simple.pe_ttm" ||
		forwarded.Sorts[0].Direction != "desc" || forwarded.Limit != DefaultScreenLimit {
		t.Fatalf("forwarded request = %#v", forwarded)
	}
	if response.Total != 1 || response.Entries[0].InstrumentID != "US.AAPL" {
		t.Fatalf("screen response = %#v", response)
	}
}

func TestServiceScreenPassesProviderErrorsThrough(t *testing.T) {
	want := errors.New("screen upstream failed")
	service := NewService(&screenCapableProviderStub{err: want})
	if _, err := service.GetScreen(context.Background(), ScreenRequest{Market: "US"}); !errors.Is(err, want) {
		t.Fatalf("error passthrough = %v", err)
	}
}
