package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	// DefaultRankingsLimit is the default number of ranking entries a provider
	// returns for one market.
	DefaultRankingsLimit = 20
	// MaxRankingsLimit bounds the entries accepted by one rankings request.
	MaxRankingsLimit = 100
)

// RankingsSource is an optional provider capability that supplies market-wide
// rankings (gainers, losers, most active). Providers without an implementation
// leave the capability unsupported.
type RankingsSource interface {
	Rankings(ctx context.Context, market, kind string, limit int) (RankingsResponse, error)
}

// RankingEntry is one provider-neutral ranked instrument. Numeric fields are
// nullable because upstream feeds do not guarantee every quote field.
type RankingEntry struct {
	InstrumentID  string       `json:"instrumentId"`
	Name          string       `json:"name"`
	Price         *json.Number `json:"price"`
	ChangeRate    *json.Number `json:"changeRate"`
	ChangeAmount  *json.Number `json:"changeAmount"`
	Volume        *json.Number `json:"volume"`
	Turnover      *json.Number `json:"turnover"`
	TurnoverRatio *json.Number `json:"turnoverRatio"`
	PETTM         *json.Number `json:"peTTM"`
	MarketCap     *json.Number `json:"marketCap"`
}

// RankingsResponse is the provider-neutral market rankings payload.
type RankingsResponse struct {
	Market  string         `json:"market"`
	Kind    string         `json:"kind" enums:"gainers,losers,active"`
	Entries []RankingEntry `json:"entries"`
	Source  string         `json:"source"`
}

// GetRankings 返回当前行情提供者的市场榜单（涨幅、跌幅、活跃）。
func (s *Service) GetRankings(
	ctx context.Context,
	market string,
	kind string,
	limit int,
) (RankingsResponse, error) {
	s.providerLifecycleMu.RLock()
	defer s.providerLifecycleMu.RUnlock()
	source, err := s.rankingsSource(ctx)
	if err != nil {
		return RankingsResponse{}, err
	}
	if limit == 0 {
		limit = DefaultRankingsLimit
	}
	if limit < 1 || limit > MaxRankingsLimit {
		return RankingsResponse{}, fmt.Errorf("rankings limit must be between 1 and %d", MaxRankingsLimit)
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case "gainers", "losers", "active":
	default:
		return RankingsResponse{}, fmt.Errorf("rankings kind must be one of gainers, losers, active")
	}
	// CN 聚合市场没有单一标的可解析，原样透传给提供者（akshare 直接接受 CN）。
	market, _ = normalizeCNAggregateRead(market, "")
	return source.Rankings(ctx, market, kind, limit)
}

func (s *Service) rankingsSource(ctx context.Context) (RankingsSource, error) {
	if source, ok := s.provider.(RankingsSource); ok {
		return source, nil
	}
	return nil, s.optionalCapabilityError(ctx, "market rankings")
}
