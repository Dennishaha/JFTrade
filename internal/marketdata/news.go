package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const (
	// DefaultNewsLimit is the default number of news entries a provider returns.
	DefaultNewsLimit = 10
	// MaxNewsLimit bounds the entries accepted by one news request.
	MaxNewsLimit = 50
)

// NewsSource is an optional provider capability that supplies instrument news.
// Providers without an implementation leave the capability unsupported.
type NewsSource interface {
	News(ctx context.Context, market, symbol string, limit int) (NewsResponse, error)
}

// CorporateActionsSource is an optional provider capability that supplies
// dividend and split events for an instrument.
type CorporateActionsSource interface {
	CorporateActions(ctx context.Context, market, symbol string, from, to time.Time) (CorporateActionsResponse, error)
}

// NewsEntry is one provider-neutral news item. Every field is nullable because
// upstream feeds do not guarantee attribution or timestamps.
type NewsEntry struct {
	Title       *string `json:"title"`
	Link        *string `json:"link"`
	Publisher   *string `json:"publisher"`
	PublishedAt *string `json:"publishedAt"`
	Summary     *string `json:"summary"`
}

// NewsResponse is the provider-neutral news payload.
type NewsResponse struct {
	Market       string      `json:"market"`
	Symbol       string      `json:"symbol"`
	InstrumentID string      `json:"instrumentId"`
	Entries      []NewsEntry `json:"entries"`
	Source       string      `json:"source"`
}

// CorporateActionEvent is one dividend or split event keyed by its ex-date.
type CorporateActionEvent struct {
	Kind   string       `json:"kind" enums:"dividend,split"`
	ExDate string       `json:"exDate"`
	Amount *json.Number `json:"amount"`
	Ratio  *json.Number `json:"ratio"`
}

// CorporateActionsResponse is the provider-neutral corporate actions payload.
type CorporateActionsResponse struct {
	Market       string                 `json:"market"`
	Symbol       string                 `json:"symbol"`
	InstrumentID string                 `json:"instrumentId"`
	Events       []CorporateActionEvent `json:"events"`
	Source       string                 `json:"source"`
}

// GetNews 返回当前行情提供者的标的资讯。
func (s *Service) GetNews(ctx context.Context, market, symbol string, limit int) (NewsResponse, error) {
	s.providerLifecycleMu.RLock()
	defer s.providerLifecycleMu.RUnlock()
	source, err := s.newsSource(ctx)
	if err != nil {
		return NewsResponse{}, err
	}
	if limit == 0 {
		limit = DefaultNewsLimit
	}
	if limit < 1 || limit > MaxNewsLimit {
		return NewsResponse{}, fmt.Errorf("news limit must be between 1 and %d", MaxNewsLimit)
	}
	market, symbol = normalizeCNAggregateRead(market, symbol)
	return source.News(ctx, market, symbol, limit)
}

// GetCorporateActions 返回当前行情提供者的分红拆股事件。
func (s *Service) GetCorporateActions(
	ctx context.Context,
	market string,
	symbol string,
	from time.Time,
	to time.Time,
) (CorporateActionsResponse, error) {
	s.providerLifecycleMu.RLock()
	defer s.providerLifecycleMu.RUnlock()
	source, err := s.corporateActionsSource(ctx)
	if err != nil {
		return CorporateActionsResponse{}, err
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return CorporateActionsResponse{}, fmt.Errorf("corporate actions from must not be after to")
	}
	market, symbol = normalizeCNAggregateRead(market, symbol)
	return source.CorporateActions(ctx, market, symbol, from, to)
}

func (s *Service) newsSource(ctx context.Context) (NewsSource, error) {
	if source, ok := s.provider.(NewsSource); ok {
		return source, nil
	}
	return nil, s.optionalCapabilityError(ctx, "instrument news")
}

func (s *Service) corporateActionsSource(ctx context.Context) (CorporateActionsSource, error) {
	if source, ok := s.provider.(CorporateActionsSource); ok {
		return source, nil
	}
	return nil, s.optionalCapabilityError(ctx, "corporate actions")
}

func (s *Service) optionalCapabilityError(ctx context.Context, capability string) error {
	providerID := ""
	if descriptor, err := s.provider.Descriptor(ctx); err == nil {
		providerID = descriptor.ProviderID
	}
	return fmt.Errorf(
		"%w: active provider %q does not support %s",
		ErrCapabilityUnsupported, providerID, capability,
	)
}
