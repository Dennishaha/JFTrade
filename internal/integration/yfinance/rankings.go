package yfinance

import (
	"context"
	"fmt"
	"strings"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

var _ marketdata.RankingsSource = (*Provider)(nil)

// Rankings returns a US market ranking list (gainers, losers, most active).
// Yahoo Finance predefined screeners only cover US equities; other markets are
// rejected Go-side as ErrUnsupported before any sidecar call.
func (p *Provider) Rankings(
	ctx context.Context,
	marketValue string,
	kind string,
	limit int,
) (marketdata.RankingsResponse, error) {
	canonical, err := canonicalMarket(marketValue)
	if err != nil {
		return marketdata.RankingsResponse{}, err
	}
	if canonical != yfinanceDefaultMarket {
		return marketdata.RankingsResponse{}, fmt.Errorf("%w: rankings market %q", ErrUnsupported, marketValue)
	}
	kind, err = rankingsKind(kind)
	if err != nil {
		return marketdata.RankingsResponse{}, err
	}
	limit = normalizeLimit(limit, marketdata.DefaultRankingsLimit, marketdata.MaxRankingsLimit)
	response, err := p.client.rankings(ctx, canonical, kind, limit)
	if err != nil {
		return marketdata.RankingsResponse{}, err
	}
	return convertRankings(response, canonical, kind)
}

func rankingsKind(kind string) (string, error) {
	switch normalized := strings.ToLower(strings.TrimSpace(kind)); normalized {
	case "gainers", "losers", "active":
		return normalized, nil
	default:
		return "", fmt.Errorf("%w: rankings kind %q", ErrUnsupported, kind)
	}
}

func convertRankings(
	response remoteRankings,
	expectedMarket string,
	expectedKind string,
) (marketdata.RankingsResponse, error) {
	if kind := strings.ToLower(strings.TrimSpace(response.Kind)); kind != "" && kind != expectedKind {
		return marketdata.RankingsResponse{}, fmt.Errorf(
			"%w: rankings kind %q does not match %q", ErrInvalidResponse, response.Kind, expectedKind,
		)
	}
	entries := make([]marketdata.RankingEntry, 0, len(response.Entries))
	for index, entry := range response.Entries {
		instrumentID := strings.ToUpper(strings.TrimSpace(entry.InstrumentID))
		if instrumentID == "" {
			return marketdata.RankingsResponse{}, fmt.Errorf(
				"%w: ranking entry %d instrument_id is required", ErrInvalidResponse, index,
			)
		}
		entries = append(entries, marketdata.RankingEntry{
			InstrumentID:  instrumentID,
			Name:          strings.TrimSpace(entry.Name),
			Price:         entry.Price,
			ChangeRate:    entry.ChangeRate,
			ChangeAmount:  entry.ChangeAmount,
			Volume:        entry.Volume,
			Turnover:      entry.Turnover,
			TurnoverRatio: entry.TurnoverRatio,
			PETTM:         entry.PETTM,
			MarketCap:     entry.MarketCap,
		})
	}
	market := strings.ToUpper(strings.TrimSpace(response.Market))
	if market == "" {
		market = expectedMarket
	}
	return marketdata.RankingsResponse{
		Market: market, Kind: expectedKind, Entries: entries,
		Source: rankingsSource(response.Source),
	}, nil
}

func rankingsSource(source string) string {
	if value := strings.TrimSpace(source); value != "" {
		return value
	}
	return "yfinance-rankings"
}
