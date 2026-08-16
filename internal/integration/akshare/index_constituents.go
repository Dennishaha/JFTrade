package akshare

import (
	"context"
	"fmt"
	"strings"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

var _ marketdata.IndexConstituentsSource = (*Provider)(nil)

// IndexConstituents returns the member list of a CN index. Only CSI and CN
// exchange indices (SH/SZ) are covered by the AKShare sidecar; other
// instruments and markets surface as ErrUnsupported.
func (p *Provider) IndexConstituents(
	ctx context.Context,
	marketValue string,
	symbol string,
	limit int,
) (marketdata.IndexConstituentsResponse, error) {
	instrument, err := normalizeIdentity(marketValue, symbol, "")
	if err != nil {
		return marketdata.IndexConstituentsResponse{}, err
	}
	limit = normalizeLimit(limit, marketdata.DefaultIndexConstituentsLimit, marketdata.MaxIndexConstituentsLimit)
	response, err := p.client.indexConstituents(ctx, instrument.market, instrument.symbol, limit)
	if err != nil {
		return marketdata.IndexConstituentsResponse{}, err
	}
	return convertIndexConstituents(response, instrument)
}

func convertIndexConstituents(
	response remoteIndexConstituents,
	expected normalizedInstrument,
) (marketdata.IndexConstituentsResponse, error) {
	identity, err := normalizeIdentity(response.Market, response.Symbol, response.InstrumentID)
	if err != nil || identity.id != expected.id {
		return marketdata.IndexConstituentsResponse{}, fmt.Errorf(
			"%w: index constituents identity does not match %s", ErrInvalidResponse, expected.id,
		)
	}
	constituents := make([]marketdata.IndexConstituent, 0, len(response.Constituents))
	for index, entry := range response.Constituents {
		converted, err := convertIndexConstituent(entry)
		if err != nil {
			return marketdata.IndexConstituentsResponse{}, fmt.Errorf("index constituent %d: %w", index, err)
		}
		constituents = append(constituents, converted)
	}
	return marketdata.IndexConstituentsResponse{
		Market:       identity.market,
		Symbol:       identity.symbol,
		InstrumentID: identity.id,
		Constituents: constituents,
		Source:       indexConstituentsSource(response.Source),
	}, nil
}

func convertIndexConstituent(entry remoteIndexConstituent) (marketdata.IndexConstituent, error) {
	code := strings.TrimSpace(entry.Code)
	if code == "" {
		return marketdata.IndexConstituent{}, fmt.Errorf("%w: constituent code is required", ErrInvalidResponse)
	}
	return marketdata.IndexConstituent{
		Code:   code,
		Name:   strings.TrimSpace(entry.Name),
		Weight: entry.Weight,
	}, nil
}

func indexConstituentsSource(source string) string {
	if value := strings.TrimSpace(source); value != "" {
		return value
	}
	return "akshare-index-constituents"
}
