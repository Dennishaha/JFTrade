package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	// DefaultScreenLimit matches the console's default page size.
	DefaultScreenLimit = 50
	// MaxScreenLimit matches the public HTTP page limit for stock screens.
	MaxScreenLimit = 100
)

// ScreenerSource is an optional provider capability that executes stock
// screens against the embedded factor catalog (researchscreen
// EmbeddedCatalogVersion). Providers without an implementation leave the
// capability unsupported.
type ScreenerSource interface {
	Screen(ctx context.Context, req ScreenRequest) (ScreenResponse, error)
}

// ScreenConditionRequest is one provider-neutral interval condition; Min and
// Max are independently nullable (one-sided intervals).
type ScreenConditionRequest struct {
	FactorKey string       `json:"factorKey"`
	Min       *json.Number `json:"min"`
	Max       *json.Number `json:"max"`
}

// ScreenSortRequest is one provider-neutral sort; Direction is asc or desc.
type ScreenSortRequest struct {
	FactorKey string `json:"factorKey"`
	Direction string `json:"direction" enums:"asc,desc"`
}

// ScreenRequest is the provider-neutral stock-screen query.
type ScreenRequest struct {
	Market     string                   `json:"market"`
	Conditions []ScreenConditionRequest `json:"conditions"`
	Sorts      []ScreenSortRequest      `json:"sorts"`
	Offset     int                      `json:"offset"`
	Limit      int                      `json:"limit"`
}

// ScreenEntry is one provider-neutral screened stock; Values keys are factor
// keys from the embedded catalog.
type ScreenEntry struct {
	InstrumentID  string                 `json:"instrumentId"`
	Name          string                 `json:"name"`
	Symbol        string                 `json:"symbol"`
	Industry      *string                `json:"industry"`
	QuoteCurrency string                 `json:"quoteCurrency"`
	Values        map[string]json.Number `json:"values"`
}

// ScreenResponse is the provider-neutral stock-screen page.
type ScreenResponse struct {
	Entries    []ScreenEntry `json:"entries"`
	Total      int           `json:"total"`
	HasMore    bool          `json:"hasMore"`
	NextOffset *int          `json:"nextOffset,omitempty"`
	AsOf       string        `json:"asOf"`
	Source     string        `json:"source"`
}

// GetScreen 返回当前行情提供者的股票筛选结果页（嵌入式因子目录）。
func (s *Service) GetScreen(
	ctx context.Context,
	req ScreenRequest,
) (ScreenResponse, error) {
	s.providerLifecycleMu.RLock()
	defer s.providerLifecycleMu.RUnlock()
	source, ok := s.provider.(ScreenerSource)
	if !ok {
		return ScreenResponse{}, s.optionalCapabilityError(ctx, "stock screen")
	}
	req, err := normalizeScreenRequest(req)
	if err != nil {
		return ScreenResponse{}, err
	}
	return source.Screen(ctx, req)
}

// normalizeScreenRequest validates the structural contract; factor-key
// membership in the embedded catalog is enforced by the facade, which owns
// the catalog version negotiation.
func normalizeScreenRequest(req ScreenRequest) (ScreenRequest, error) {
	req.Market = strings.ToUpper(strings.TrimSpace(req.Market))
	if req.Market == "" {
		return req, fmt.Errorf("stock screen requires a market")
	}
	for index, condition := range req.Conditions {
		req.Conditions[index].FactorKey = strings.ToLower(strings.TrimSpace(condition.FactorKey))
		if req.Conditions[index].FactorKey == "" {
			return req, fmt.Errorf("stock screen condition %d requires a factor key", index)
		}
		if condition.Min == nil && condition.Max == nil {
			return req, fmt.Errorf("stock screen condition %d requires min or max", index)
		}
	}
	for index, sort := range req.Sorts {
		req.Sorts[index].FactorKey = strings.ToLower(strings.TrimSpace(sort.FactorKey))
		if req.Sorts[index].FactorKey == "" {
			return req, fmt.Errorf("stock screen sort %d requires a factor key", index)
		}
		direction := strings.ToLower(strings.TrimSpace(sort.Direction))
		if direction == "" {
			direction = "desc"
		}
		if direction != "asc" && direction != "desc" {
			return req, fmt.Errorf("stock screen sort %d direction must be asc or desc", index)
		}
		req.Sorts[index].Direction = direction
	}
	if req.Offset < 0 {
		return req, fmt.Errorf("stock screen offset must be non-negative")
	}
	if req.Limit == 0 {
		req.Limit = DefaultScreenLimit
	}
	if req.Limit < 1 || req.Limit > MaxScreenLimit {
		return req, fmt.Errorf("stock screen limit must be between 1 and %d", MaxScreenLimit)
	}
	return req, nil
}
