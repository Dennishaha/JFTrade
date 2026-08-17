package marketdataapp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

// screenStub 覆盖股票筛选读取，供运行时转发测试使用。
type screenStub struct {
	forwardingProviderStub
	lastReq marketdata.ScreenRequest
	err     error
}

func (p *screenStub) Screen(
	_ context.Context,
	req marketdata.ScreenRequest,
) (marketdata.ScreenResponse, error) {
	p.record("screen")
	p.lastReq = req
	if p.err != nil {
		return marketdata.ScreenResponse{}, p.err
	}
	return marketdata.ScreenResponse{
		Entries: []marketdata.ScreenEntry{{
			InstrumentID: "US.AAPL",
			Values:       map[string]json.Number{"simple.price": json.Number("189.25")},
		}},
		Total:  1,
		Source: "stub",
	}, nil
}

func TestRuntimeScreenForwarding(t *testing.T) {
	provider := &screenStub{}
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: provider})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	min := json.Number("100")
	response, err := runtime.Screen(context.Background(), marketdata.ScreenRequest{
		Market:     "US",
		Conditions: []marketdata.ScreenConditionRequest{{FactorKey: "simple.price", Min: &min}},
		Limit:      25,
	})
	if err != nil {
		t.Fatalf("Screen: %v", err)
	}
	if provider.calls["screen"] != 1 || provider.lastReq.Market != "US" ||
		provider.lastReq.Limit != 25 || len(provider.lastReq.Conditions) != 1 {
		t.Fatalf("forwarded request = %#v calls=%v", provider.lastReq, provider.calls)
	}
	if len(response.Entries) != 1 || response.Entries[0].InstrumentID != "US.AAPL" {
		t.Fatalf("response = %#v", response)
	}
}

func TestRuntimeScreenPropagatesError(t *testing.T) {
	want := errors.New("screen failed")
	provider := &screenStub{err: want}
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: provider})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	if _, err := runtime.Screen(context.Background(), marketdata.ScreenRequest{
		Market: "US", Limit: 10,
	}); !errors.Is(err, want) {
		t.Fatalf("expected provider error, got %v", err)
	}
}

func TestRuntimeScreenCapabilityUnsupported(t *testing.T) {
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: &forwardingProviderStub{}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	_, err = runtime.Screen(context.Background(), marketdata.ScreenRequest{Market: "US", Limit: 10})
	if !errors.Is(err, marketdata.ErrCapabilityUnsupported) {
		t.Fatalf("expected ErrCapabilityUnsupported, got %v", err)
	}
	if got := err.Error(); !strings.Contains(got, ProviderFutu) || !strings.Contains(got, "stock screen") {
		t.Fatalf("unexpected error message: %q", got)
	}
}
