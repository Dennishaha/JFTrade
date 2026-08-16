package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
)

const (
	// DefaultIndexConstituentsLimit is the default number of constituents a
	// provider returns for one index.
	DefaultIndexConstituentsLimit = 200
	// MaxIndexConstituentsLimit bounds the constituents accepted by one request.
	MaxIndexConstituentsLimit = 1000
)

// IndexConstituentsSource is an optional provider capability that supplies the
// constituent list of an index. Providers without an implementation leave the
// capability unsupported.
type IndexConstituentsSource interface {
	IndexConstituents(ctx context.Context, market, symbol string, limit int) (IndexConstituentsResponse, error)
}

// IndexConstituent is one provider-neutral index member. Weight is nullable
// because the upstream sources do not always publish index weights.
type IndexConstituent struct {
	Code   string       `json:"code"`
	Name   string       `json:"name"`
	Weight *json.Number `json:"weight"`
}

// IndexConstituentsResponse is the provider-neutral index constituents payload.
type IndexConstituentsResponse struct {
	Market       string             `json:"market"`
	Symbol       string             `json:"symbol"`
	InstrumentID string             `json:"instrumentId"`
	Constituents []IndexConstituent `json:"constituents"`
	Source       string             `json:"source"`
}

// GetIndexConstituents 返回当前行情提供者的指数成分股列表。
func (s *Service) GetIndexConstituents(
	ctx context.Context,
	market string,
	symbol string,
	limit int,
) (IndexConstituentsResponse, error) {
	s.providerLifecycleMu.RLock()
	defer s.providerLifecycleMu.RUnlock()
	source, err := s.indexConstituentsSource(ctx)
	if err != nil {
		return IndexConstituentsResponse{}, err
	}
	if limit == 0 {
		limit = DefaultIndexConstituentsLimit
	}
	if limit < 1 || limit > MaxIndexConstituentsLimit {
		return IndexConstituentsResponse{}, fmt.Errorf(
			"index constituents limit must be between 1 and %d", MaxIndexConstituentsLimit,
		)
	}
	market, symbol = normalizeCNAggregateRead(market, symbol)
	return source.IndexConstituents(ctx, market, symbol, limit)
}

func (s *Service) indexConstituentsSource(ctx context.Context) (IndexConstituentsSource, error) {
	if source, ok := s.provider.(IndexConstituentsSource); ok {
		return source, nil
	}
	return nil, s.optionalCapabilityError(ctx, "index constituents")
}
