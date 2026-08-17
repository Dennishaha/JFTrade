package akshare

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

var _ marketdata.ScreenerSource = (*Provider)(nil)

// Screen executes an embedded-catalog stock screen against the AKShare
// sidecar. CN, its SH/SZ leaves, HK, and US are forwarded; the CN aggregate is
// passed through unchanged so the sidecar owns the SH/SZ merge. US rows come
// from the sidecar's Eastmoney clist feed with PB/PE TTM filled in. Other
// markets are rejected Go-side as ErrUnsupported before any sidecar call.
func (p *Provider) Screen(
	ctx context.Context,
	req marketdata.ScreenRequest,
) (marketdata.ScreenResponse, error) {
	market, err := screenMarket(req.Market)
	if err != nil {
		return marketdata.ScreenResponse{}, err
	}
	response, err := p.client.screen(ctx, toRemoteScreenRequest(req, market))
	if err != nil {
		return marketdata.ScreenResponse{}, classifyScreenError(err)
	}
	return convertScreen(response, market)
}

// classifyScreenError folds the sidecar 400 capability codes into the
// capability contract on top of the company-research unsupported_market
// mapping; invalid_request and other codes pass through unchanged.
func classifyScreenError(err error) error {
	var remoteErr *HTTPError
	if errors.As(err, &remoteErr) && strings.EqualFold(strings.TrimSpace(remoteErr.Code), "unsupported_kind") {
		return fmt.Errorf("%w: %w", ErrUnsupported, remoteErr)
	}
	return classifyCompanyResearchError(err)
}

func screenMarket(marketValue string) (string, error) {
	canonical, err := canonicalMarket(marketValue)
	if err != nil {
		return "", err
	}
	switch canonical {
	case "CN", "SH", "SZ", "HK", "US":
		return canonical, nil
	default:
		return "", fmt.Errorf("%w: stock screen market %q", ErrUnsupported, marketValue)
	}
}

func toRemoteScreenRequest(req marketdata.ScreenRequest, market string) remoteScreenRequest {
	remote := remoteScreenRequest{
		Market: market,
		Offset: req.Offset,
		Limit:  req.Limit,
	}
	for _, condition := range req.Conditions {
		entry := remoteScreenCondition{FactorKey: condition.FactorKey}
		if condition.Min != nil {
			entry.Min = *condition.Min
		}
		if condition.Max != nil {
			entry.Max = *condition.Max
		}
		remote.Conditions = append(remote.Conditions, entry)
	}
	for _, sort := range req.Sorts {
		remote.Sorts = append(remote.Sorts, remoteScreenSort{
			FactorKey: sort.FactorKey,
			Direction: sort.Direction,
		})
	}
	return remote
}

func convertScreen(
	response remoteScreenResponse,
	expectedMarket string,
) (marketdata.ScreenResponse, error) {
	entries := make([]marketdata.ScreenEntry, 0, len(response.Entries))
	for index, entry := range response.Entries {
		instrumentID := strings.ToUpper(strings.TrimSpace(entry.InstrumentID))
		if instrumentID == "" {
			return marketdata.ScreenResponse{}, fmt.Errorf(
				"%w: screen entry %d instrument_id is required", ErrInvalidResponse, index,
			)
		}
		symbol := instrumentID
		if entry.Symbol != nil {
			if trimmed := strings.TrimSpace(*entry.Symbol); trimmed != "" {
				symbol = strings.ToUpper(trimmed)
			}
		} else if parts := strings.SplitN(instrumentID, ".", 2); len(parts) == 2 {
			symbol = parts[1]
		}
		entries = append(entries, marketdata.ScreenEntry{
			InstrumentID:  instrumentID,
			Name:          strings.TrimSpace(entry.Name),
			Symbol:        symbol,
			Industry:      entry.Industry,
			QuoteCurrency: strings.ToUpper(strings.TrimSpace(entry.QuoteCurrency)),
			Values:        entry.Values,
		})
	}
	return marketdata.ScreenResponse{
		Entries:    entries,
		Total:      response.Total,
		HasMore:    response.HasMore,
		NextOffset: response.NextOffset,
		AsOf:       strings.TrimSpace(response.AsOf),
		Source:     screenSource(response.Source, expectedMarket),
	}, nil
}

func screenSource(source, market string) string {
	if value := strings.TrimSpace(source); value != "" {
		return value
	}
	return "akshare-screen-" + strings.ToLower(market)
}
