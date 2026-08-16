package marketdataapp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

type rankingsCapableForwardingProviderStub struct {
	forwardingProviderStub
	market      string
	kind        string
	board       string
	limit       int
	callErr     error
	rankingsRan bool
}

func (p *rankingsCapableForwardingProviderStub) Rankings(
	_ context.Context,
	market string,
	kind string,
	limit int,
) (marketdata.RankingsResponse, error) {
	p.record("Rankings")
	p.rankingsRan = true
	p.market, p.kind, p.limit = market, kind, limit
	return marketdata.RankingsResponse{
		Market: market, Kind: kind,
		Entries: []marketdata.RankingEntry{{InstrumentID: "US.AAPL", Name: "Apple"}},
		Source:  "stub-rankings",
	}, p.callErr
}

func (p *rankingsCapableForwardingProviderStub) Industries(
	_ context.Context,
	kind string,
) (marketdata.IndustryBoardsResponse, error) {
	p.record("Industries")
	p.kind = kind
	return marketdata.IndustryBoardsResponse{
		Market: "CN", Kind: kind,
		Boards: []marketdata.IndustryBoard{{Name: "半导体"}},
		Source: "stub-industries",
	}, p.callErr
}

func (p *rankingsCapableForwardingProviderStub) IndustryMembers(
	_ context.Context,
	kind string,
	board string,
	limit int,
) (marketdata.IndustryMembersResponse, error) {
	p.record("IndustryMembers")
	p.kind, p.board, p.limit = kind, board, limit
	return marketdata.IndustryMembersResponse{
		Market: "CN", Kind: kind, Board: board,
		Entries: []marketdata.RankingEntry{{InstrumentID: "SH.688981", Name: "中芯国际"}},
		Source:  "stub-industries",
	}, p.callErr
}

func TestRuntimeForwardsRankingsToCapableActiveProvider(t *testing.T) {
	provider := &rankingsCapableForwardingProviderStub{}
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: provider})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	ctx := t.Context()

	response, err := runtime.Rankings(ctx, "US", "gainers", 50)
	if err != nil || response.Kind != "gainers" || response.Source != "stub-rankings" ||
		len(response.Entries) != 1 {
		t.Fatalf("Rankings = %#v, err=%v", response, err)
	}
	if provider.market != "US" || provider.kind != "gainers" || provider.limit != 50 {
		t.Fatalf("rankings forwarding = %s/%s/%d", provider.market, provider.kind, provider.limit)
	}
	if provider.calls["Rankings"] != 1 {
		t.Fatalf("calls = %#v", provider.calls)
	}

	provider.callErr = errors.New("rankings upstream failed")
	if _, err := runtime.Rankings(ctx, "US", "gainers", 50); !errors.Is(err, provider.callErr) {
		t.Fatalf("Rankings error passthrough = %v", err)
	}
}

func TestRuntimeForwardsIndustryReadsToCapableActiveProvider(t *testing.T) {
	provider := &rankingsCapableForwardingProviderStub{}
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: provider})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	ctx := t.Context()

	boards, err := runtime.Industries(ctx, "concept")
	if err != nil || boards.Kind != "concept" || len(boards.Boards) != 1 ||
		boards.Boards[0].Name != "半导体" {
		t.Fatalf("Industries = %#v, err=%v", boards, err)
	}
	members, err := runtime.IndustryMembers(ctx, "concept", "半导体", 30)
	if err != nil || members.Board != "半导体" || len(members.Entries) != 1 {
		t.Fatalf("IndustryMembers = %#v, err=%v", members, err)
	}
	if provider.board != "半导体" || provider.limit != 30 {
		t.Fatalf("members forwarding = %q/%d", provider.board, provider.limit)
	}
	if provider.calls["Industries"] != 1 || provider.calls["IndustryMembers"] != 1 {
		t.Fatalf("calls = %#v", provider.calls)
	}

	provider.callErr = errors.New("industry upstream failed")
	if _, err := runtime.Industries(ctx, "concept"); !errors.Is(err, provider.callErr) {
		t.Fatalf("Industries error passthrough = %v", err)
	}
	if _, err := runtime.IndustryMembers(ctx, "concept", "半导体", 30); !errors.Is(err, provider.callErr) {
		t.Fatalf("IndustryMembers error passthrough = %v", err)
	}
}

func TestRuntimeRankingsAndIndustriesRejectProvidersWithoutCapability(t *testing.T) {
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: &forwardingProviderStub{}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	ctx := t.Context()

	_, err = runtime.Rankings(ctx, "US", "gainers", 20)
	if !errors.Is(err, marketdata.ErrCapabilityUnsupported) ||
		!strings.Contains(err.Error(), ProviderFutu) || !strings.Contains(err.Error(), "rankings") {
		t.Fatalf("Rankings unsupported error = %v", err)
	}
	_, err = runtime.Industries(ctx, "industry")
	if !errors.Is(err, marketdata.ErrCapabilityUnsupported) ||
		!strings.Contains(err.Error(), "industry boards") {
		t.Fatalf("Industries unsupported error = %v", err)
	}
	_, err = runtime.IndustryMembers(ctx, "industry", "半导体", 20)
	if !errors.Is(err, marketdata.ErrCapabilityUnsupported) {
		t.Fatalf("IndustryMembers unsupported error = %v", err)
	}
}
