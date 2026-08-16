package marketdata

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type rankingsCapableProviderStub struct {
	dataProviderStub
	rankingsResponse RankingsResponse
	rankingsErr      error
	rankingsMarket   string
	rankingsKind     string
	rankingsLimit    int
	boards           IndustryBoardsResponse
	boardsErr        error
	boardsKind       string
	members          IndustryMembersResponse
	membersErr       error
	membersKind      string
	membersBoard     string
	membersLimit     int
}

func (p *rankingsCapableProviderStub) Rankings(
	_ context.Context,
	market, kind string,
	limit int,
) (RankingsResponse, error) {
	p.rankingsMarket, p.rankingsKind, p.rankingsLimit = market, kind, limit
	return p.rankingsResponse, p.rankingsErr
}

func (p *rankingsCapableProviderStub) Industries(
	_ context.Context,
	kind string,
) (IndustryBoardsResponse, error) {
	p.boardsKind = kind
	return p.boards, p.boardsErr
}

func (p *rankingsCapableProviderStub) IndustryMembers(
	_ context.Context,
	kind, board string,
	limit int,
) (IndustryMembersResponse, error) {
	p.membersKind, p.membersBoard, p.membersLimit = kind, board, limit
	return p.members, p.membersErr
}

func TestServiceRankingsRejectsProvidersWithoutCapability(t *testing.T) {
	service := NewService(&dataProviderStub{})

	_, err := service.GetRankings(context.Background(), "US", "gainers", 20)
	if !errors.Is(err, ErrCapabilityUnsupported) || !strings.Contains(err.Error(), "stub-provider") ||
		!strings.Contains(err.Error(), "rankings") {
		t.Fatalf("GetRankings unsupported error = %v", err)
	}
	_, err = service.GetIndustries(context.Background(), "CN", "industry")
	if !errors.Is(err, ErrCapabilityUnsupported) || !strings.Contains(err.Error(), "industry boards") {
		t.Fatalf("GetIndustries unsupported error = %v", err)
	}
	_, err = service.GetIndustryMembers(context.Background(), "CN", "industry", "半导体", 20)
	if !errors.Is(err, ErrCapabilityUnsupported) {
		t.Fatalf("GetIndustryMembers unsupported error = %v", err)
	}
}

func TestServiceRankingsValidatesKindAndLimit(t *testing.T) {
	provider := &rankingsCapableProviderStub{
		rankingsResponse: RankingsResponse{Market: "US", Kind: "gainers", Entries: []RankingEntry{}},
	}
	service := NewService(provider)
	ctx := context.Background()

	if _, err := service.GetRankings(ctx, "US", "breakout", 20); err == nil ||
		!strings.Contains(err.Error(), "kind") {
		t.Fatalf("invalid kind error = %v", err)
	}
	for _, limit := range []int{-1, MaxRankingsLimit + 1} {
		if _, err := service.GetRankings(ctx, "US", "gainers", limit); err == nil ||
			!strings.Contains(err.Error(), "limit") {
			t.Fatalf("GetRankings limit %d error = %v", limit, err)
		}
	}
}

func TestServiceRankingsForwardsNormalizedKindAndDefaultLimit(t *testing.T) {
	changeRate := json.Number("5.42")
	provider := &rankingsCapableProviderStub{
		rankingsResponse: RankingsResponse{
			Market: "CN", Kind: "gainers", Source: "akshare-rankings",
			Entries: []RankingEntry{{InstrumentID: "SH.600519", Name: "贵州茅台", ChangeRate: &changeRate}},
		},
	}
	service := NewService(provider)

	response, err := service.GetRankings(context.Background(), "CN", " Gainers ", 0)
	if err != nil {
		t.Fatalf("GetRankings: %v", err)
	}
	if provider.rankingsMarket != "CN" || provider.rankingsKind != "gainers" ||
		provider.rankingsLimit != DefaultRankingsLimit {
		t.Fatalf("rankings forwarding = %s/%s/%d",
			provider.rankingsMarket, provider.rankingsKind, provider.rankingsLimit)
	}
	if response.Source != "akshare-rankings" || len(response.Entries) != 1 ||
		response.Entries[0].InstrumentID != "SH.600519" {
		t.Fatalf("rankings response = %#v", response)
	}
	providerErr := errors.New("rankings upstream failed")
	provider.rankingsErr = providerErr
	if _, err := service.GetRankings(context.Background(), "CN", "gainers", 20); !errors.Is(err, providerErr) {
		t.Fatalf("provider error passthrough = %v", err)
	}
}

func TestServiceIndustriesRejectsNonCNMarkets(t *testing.T) {
	provider := &rankingsCapableProviderStub{}
	service := NewService(provider)
	ctx := context.Background()

	for _, market := range []string{"US", "HK"} {
		if _, err := service.GetIndustries(ctx, market, "industry"); !errors.Is(err, ErrCapabilityUnsupported) {
			t.Fatalf("GetIndustries market %s error = %v", market, err)
		}
		if _, err := service.GetIndustryMembers(ctx, market, "industry", "半导体", 20); !errors.Is(err, ErrCapabilityUnsupported) {
			t.Fatalf("GetIndustryMembers market %s error = %v", market, err)
		}
	}
	if provider.boardsKind != "" {
		t.Fatalf("non-CN market must not reach the provider, kind = %q", provider.boardsKind)
	}
}

func TestServiceIndustriesForwardsKindAndMembersArguments(t *testing.T) {
	provider := &rankingsCapableProviderStub{
		boards: IndustryBoardsResponse{
			Market: "CN", Kind: "concept", Source: "akshare-industries",
			Boards: []IndustryBoard{{Name: "人工智能"}},
		},
		members: IndustryMembersResponse{
			Market: "CN", Kind: "concept", Board: "人工智能", Source: "akshare-industries",
			Entries: []RankingEntry{{InstrumentID: "SZ.300750", Name: "宁德时代"}},
		},
	}
	service := NewService(provider)
	ctx := context.Background()

	boards, err := service.GetIndustries(ctx, "CN", " Concept ")
	if err != nil {
		t.Fatalf("GetIndustries: %v", err)
	}
	if provider.boardsKind != "concept" || boards.Boards[0].Name != "人工智能" {
		t.Fatalf("industries forwarding = %q, response = %#v", provider.boardsKind, boards)
	}

	members, err := service.GetIndustryMembers(ctx, "sz", "concept", " 人工智能 ", 0)
	if err != nil {
		t.Fatalf("GetIndustryMembers: %v", err)
	}
	if provider.membersKind != "concept" || provider.membersBoard != "人工智能" ||
		provider.membersLimit != DefaultRankingsLimit {
		t.Fatalf("members forwarding = %q/%q/%d",
			provider.membersKind, provider.membersBoard, provider.membersLimit)
	}
	if members.Entries[0].InstrumentID != "SZ.300750" {
		t.Fatalf("members response = %#v", members)
	}

	if _, err := service.GetIndustryMembers(ctx, "CN", "concept", "  ", 20); err == nil ||
		!strings.Contains(err.Error(), "board") {
		t.Fatalf("empty board error = %v", err)
	}
	if _, err := service.GetIndustries(ctx, "CN", "region"); err == nil ||
		!strings.Contains(err.Error(), "kind") {
		t.Fatalf("invalid kind error = %v", err)
	}
}

func TestServiceIndustriesDefaultsEmptyKindToIndustry(t *testing.T) {
	provider := &rankingsCapableProviderStub{
		boards: IndustryBoardsResponse{Market: "CN", Kind: "industry", Boards: []IndustryBoard{}},
	}
	service := NewService(provider)
	if _, err := service.GetIndustries(context.Background(), "CN", ""); err != nil {
		t.Fatalf("GetIndustries: %v", err)
	}
	if provider.boardsKind != "industry" {
		t.Fatalf("default kind = %q", provider.boardsKind)
	}
}
