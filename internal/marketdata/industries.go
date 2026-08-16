package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// IndustrySource is an optional provider capability that supplies CN industry
// and concept board rankings plus board membership. Providers without an
// implementation leave the capability unsupported.
type IndustrySource interface {
	Industries(ctx context.Context, kind string) (IndustryBoardsResponse, error)
	IndustryMembers(ctx context.Context, kind, board string, limit int) (IndustryMembersResponse, error)
}

// IndustryBoard is one provider-neutral industry or concept board. Numeric
// fields are nullable because upstream feeds do not guarantee every field.
type IndustryBoard struct {
	Name                   string       `json:"name"`
	ChangeRate             *json.Number `json:"changeRate"`
	Turnover               *json.Number `json:"turnover"`
	Volume                 *json.Number `json:"volume"`
	LeadingStockName       string       `json:"leadingStockName"`
	LeadingStockChangeRate *json.Number `json:"leadingStockChangeRate"`
}

// IndustryBoardsResponse is the provider-neutral board list payload.
type IndustryBoardsResponse struct {
	Market string          `json:"market"`
	Kind   string          `json:"kind" enums:"industry,concept"`
	Boards []IndustryBoard `json:"boards"`
	Source string          `json:"source"`
}

// IndustryMembersResponse is the provider-neutral board membership payload.
type IndustryMembersResponse struct {
	Market  string         `json:"market"`
	Kind    string         `json:"kind" enums:"industry,concept"`
	Board   string         `json:"board"`
	Entries []RankingEntry `json:"entries"`
	Source  string         `json:"source"`
}

// GetIndustries 返回当前行情提供者的 CN 行业/概念板块榜单。
func (s *Service) GetIndustries(ctx context.Context, market, kind string) (IndustryBoardsResponse, error) {
	s.providerLifecycleMu.RLock()
	defer s.providerLifecycleMu.RUnlock()
	source, err := s.industrySource(ctx)
	if err != nil {
		return IndustryBoardsResponse{}, err
	}
	if err := validateIndustryMarket(market); err != nil {
		return IndustryBoardsResponse{}, err
	}
	kind = normalizeIndustryKind(kind)
	if kind == "" {
		kind = "industry"
	}
	if !isIndustryKind(kind) {
		return IndustryBoardsResponse{}, fmt.Errorf("industry board kind must be one of industry, concept")
	}
	return source.Industries(ctx, kind)
}

// GetIndustryMembers 返回当前行情提供者的 CN 板块成分股榜单。
func (s *Service) GetIndustryMembers(
	ctx context.Context,
	market string,
	kind string,
	board string,
	limit int,
) (IndustryMembersResponse, error) {
	s.providerLifecycleMu.RLock()
	defer s.providerLifecycleMu.RUnlock()
	source, err := s.industrySource(ctx)
	if err != nil {
		return IndustryMembersResponse{}, err
	}
	if err := validateIndustryMarket(market); err != nil {
		return IndustryMembersResponse{}, err
	}
	kind = normalizeIndustryKind(kind)
	if kind != "" && !isIndustryKind(kind) {
		return IndustryMembersResponse{}, fmt.Errorf("industry board kind must be one of industry, concept")
	}
	board = strings.TrimSpace(board)
	if board == "" {
		return IndustryMembersResponse{}, fmt.Errorf("industry board name is required")
	}
	if limit == 0 {
		limit = DefaultRankingsLimit
	}
	if limit < 1 || limit > MaxRankingsLimit {
		return IndustryMembersResponse{}, fmt.Errorf(
			"industry members limit must be between 1 and %d", MaxRankingsLimit,
		)
	}
	return source.IndustryMembers(ctx, kind, board, limit)
}

func (s *Service) industrySource(ctx context.Context) (IndustrySource, error) {
	if source, ok := s.provider.(IndustrySource); ok {
		return source, nil
	}
	return nil, s.optionalCapabilityError(ctx, "industry boards")
}

// validateIndustryMarket gates the capability to the CN aggregate and its SH/SZ
// leaf markets, which are the only markets the board feeds cover.
func validateIndustryMarket(market string) error {
	switch strings.ToUpper(strings.TrimSpace(market)) {
	case "", "CN", "SH", "SZ":
		return nil
	default:
		return fmt.Errorf(
			"%w: industry boards only cover the CN market (requested %q)",
			ErrCapabilityUnsupported, strings.ToUpper(strings.TrimSpace(market)),
		)
	}
}

func normalizeIndustryKind(kind string) string {
	return strings.ToLower(strings.TrimSpace(kind))
}

func isIndustryKind(kind string) bool {
	return kind == "industry" || kind == "concept"
}
