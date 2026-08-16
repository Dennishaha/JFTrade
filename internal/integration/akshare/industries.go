package akshare

import (
	"context"
	"fmt"
	"strings"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

var _ marketdata.IndustrySource = (*Provider)(nil)

// Industries returns the CN industry or concept board ranking. US and HK board
// feeds are not covered by the AKShare sidecar and surface as ErrUnsupported.
func (p *Provider) Industries(
	ctx context.Context,
	kind string,
) (marketdata.IndustryBoardsResponse, error) {
	kind, err := industryKind(kind)
	if err != nil {
		return marketdata.IndustryBoardsResponse{}, err
	}
	response, err := p.client.industries(ctx, kind)
	if err != nil {
		return marketdata.IndustryBoardsResponse{}, err
	}
	return convertIndustryBoards(response, kind)
}

// IndustryMembers returns the ranked member list of one CN board.
func (p *Provider) IndustryMembers(
	ctx context.Context,
	kind string,
	board string,
	limit int,
) (marketdata.IndustryMembersResponse, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind != "" {
		validated, err := industryKind(kind)
		if err != nil {
			return marketdata.IndustryMembersResponse{}, err
		}
		kind = validated
	}
	board = strings.TrimSpace(board)
	if board == "" {
		return marketdata.IndustryMembersResponse{}, fmt.Errorf("industry board name is required")
	}
	limit = normalizeLimit(limit, marketdata.DefaultRankingsLimit, marketdata.MaxRankingsLimit)
	response, err := p.client.industryMembers(ctx, kind, board, limit)
	if err != nil {
		return marketdata.IndustryMembersResponse{}, err
	}
	return convertIndustryMembers(response, board)
}

func industryKind(kind string) (string, error) {
	switch normalized := strings.ToLower(strings.TrimSpace(kind)); normalized {
	case "industry", "concept":
		return normalized, nil
	default:
		return "", fmt.Errorf("%w: industry board kind %q", ErrUnsupported, kind)
	}
}

func convertIndustryBoards(
	response remoteIndustryBoards,
	expectedKind string,
) (marketdata.IndustryBoardsResponse, error) {
	if kind := strings.ToLower(strings.TrimSpace(response.Kind)); kind != "" && kind != expectedKind {
		return marketdata.IndustryBoardsResponse{}, fmt.Errorf(
			"%w: industry board kind %q does not match %q", ErrInvalidResponse, response.Kind, expectedKind,
		)
	}
	boards := make([]marketdata.IndustryBoard, 0, len(response.Boards))
	for index, board := range response.Boards {
		name := strings.TrimSpace(board.Name)
		if name == "" {
			return marketdata.IndustryBoardsResponse{}, fmt.Errorf(
				"%w: industry board %d name is required", ErrInvalidResponse, index,
			)
		}
		boards = append(boards, marketdata.IndustryBoard{
			Name:                   name,
			ChangeRate:             board.ChangeRate,
			Turnover:               board.Turnover,
			Volume:                 board.Volume,
			LeadingStockName:       strings.TrimSpace(board.LeadingStockName),
			LeadingStockChangeRate: board.LeadingStockChangeRate,
		})
	}
	market := strings.ToUpper(strings.TrimSpace(response.Market))
	if market == "" {
		market = "CN"
	}
	return marketdata.IndustryBoardsResponse{
		Market: market, Kind: expectedKind, Boards: boards,
		Source: industryBoardsSource(response.Source),
	}, nil
}

func convertIndustryMembers(
	response remoteIndustryMembers,
	expectedBoard string,
) (marketdata.IndustryMembersResponse, error) {
	board := strings.TrimSpace(response.Board)
	if board != "" && !strings.EqualFold(board, expectedBoard) {
		return marketdata.IndustryMembersResponse{}, fmt.Errorf(
			"%w: industry members board %q does not match %q", ErrInvalidResponse, board, expectedBoard,
		)
	}
	entries, err := convertRankingEntries(response.Entries)
	if err != nil {
		return marketdata.IndustryMembersResponse{}, err
	}
	market := strings.ToUpper(strings.TrimSpace(response.Market))
	if market == "" {
		market = "CN"
	}
	return marketdata.IndustryMembersResponse{
		Market: market, Kind: strings.ToLower(strings.TrimSpace(response.Kind)),
		Board: expectedBoard, Entries: entries,
		Source: industryBoardsSource(response.Source),
	}, nil
}

func industryBoardsSource(source string) string {
	if value := strings.TrimSpace(source); value != "" {
		return value
	}
	return "akshare-industries"
}
