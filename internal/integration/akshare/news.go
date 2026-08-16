package akshare

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

var (
	_ marketdata.NewsSource             = (*Provider)(nil)
	_ marketdata.CorporateActionsSource = (*Provider)(nil)
)

// News returns recent news entries for an instrument. US and HK news are not
// covered by the AKShare sidecar and surface as ErrUnsupported.
func (p *Provider) News(
	ctx context.Context,
	marketValue string,
	symbol string,
	limit int,
) (marketdata.NewsResponse, error) {
	instrument, err := normalizeIdentity(marketValue, symbol, "")
	if err != nil {
		return marketdata.NewsResponse{}, err
	}
	limit = normalizeLimit(limit, marketdata.DefaultNewsLimit, marketdata.MaxNewsLimit)
	response, err := p.client.news(ctx, instrument.market, instrument.symbol, limit)
	if err != nil {
		return marketdata.NewsResponse{}, err
	}
	return convertNews(response, instrument)
}

// CorporateActions returns dividend and split events for an instrument. US and
// HK events are not covered by the AKShare sidecar and surface as
// ErrUnsupported.
func (p *Provider) CorporateActions(
	ctx context.Context,
	marketValue string,
	symbol string,
	from time.Time,
	to time.Time,
) (marketdata.CorporateActionsResponse, error) {
	instrument, err := normalizeIdentity(marketValue, symbol, "")
	if err != nil {
		return marketdata.CorporateActionsResponse{}, err
	}
	response, err := p.client.corporateActions(ctx, instrument.market, instrument.symbol, from, to)
	if err != nil {
		return marketdata.CorporateActionsResponse{}, err
	}
	return convertCorporateActions(response, instrument)
}

func convertNews(response remoteNews, expected normalizedInstrument) (marketdata.NewsResponse, error) {
	identity, err := normalizeIdentity(response.Market, response.Symbol, response.InstrumentID)
	if err != nil || identity.id != expected.id {
		return marketdata.NewsResponse{}, fmt.Errorf(
			"%w: news identity does not match %s", ErrInvalidResponse, expected.id,
		)
	}
	entries := make([]marketdata.NewsEntry, 0, len(response.Entries))
	for index, entry := range response.Entries {
		converted, err := convertNewsEntry(entry)
		if err != nil {
			return marketdata.NewsResponse{}, fmt.Errorf("news entry %d: %w", index, err)
		}
		entries = append(entries, converted)
	}
	return marketdata.NewsResponse{
		Market:       identity.market,
		Symbol:       identity.symbol,
		InstrumentID: identity.id,
		Entries:      entries,
		Source:       newsSource(response.Source),
	}, nil
}

func convertNewsEntry(entry remoteNewsEntry) (marketdata.NewsEntry, error) {
	converted := marketdata.NewsEntry{
		Title:     optionalText(entry.Title),
		Link:      optionalText(entry.Link),
		Publisher: optionalText(entry.Publisher),
		Summary:   optionalText(entry.Summary),
	}
	if entry.PublishedAt == nil || strings.TrimSpace(*entry.PublishedAt) == "" {
		return converted, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(*entry.PublishedAt))
	if err != nil {
		return marketdata.NewsEntry{}, fmt.Errorf("%w: published_at must be RFC3339", ErrInvalidResponse)
	}
	formatted := parsed.UTC().Format(time.RFC3339Nano)
	converted.PublishedAt = &formatted
	return converted, nil
}

func convertCorporateActions(
	response remoteCorporateActions,
	expected normalizedInstrument,
) (marketdata.CorporateActionsResponse, error) {
	identity, err := normalizeIdentity(response.Market, response.Symbol, response.InstrumentID)
	if err != nil || identity.id != expected.id {
		return marketdata.CorporateActionsResponse{}, fmt.Errorf(
			"%w: corporate actions identity does not match %s", ErrInvalidResponse, expected.id,
		)
	}
	events := make([]marketdata.CorporateActionEvent, 0, len(response.Events))
	for index, event := range response.Events {
		converted, err := convertCorporateAction(event)
		if err != nil {
			return marketdata.CorporateActionsResponse{}, fmt.Errorf("corporate action %d: %w", index, err)
		}
		events = append(events, converted)
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].ExDate != events[j].ExDate {
			return events[i].ExDate < events[j].ExDate
		}
		return events[i].Kind < events[j].Kind
	})
	return marketdata.CorporateActionsResponse{
		Market:       identity.market,
		Symbol:       identity.symbol,
		InstrumentID: identity.id,
		Events:       events,
		Source:       corporateActionsSource(response.Source),
	}, nil
}

func convertCorporateAction(event remoteCorporateAction) (marketdata.CorporateActionEvent, error) {
	kind := strings.ToLower(strings.TrimSpace(event.Kind))
	if kind != "dividend" && kind != "split" {
		return marketdata.CorporateActionEvent{}, fmt.Errorf(
			"%w: corporate action kind %q", ErrInvalidResponse, event.Kind,
		)
	}
	exDate := strings.TrimSpace(event.ExDate)
	if _, err := time.Parse("2006-01-02", exDate); err != nil {
		return marketdata.CorporateActionEvent{}, fmt.Errorf(
			"%w: corporate action ex_date %q", ErrInvalidResponse, event.ExDate,
		)
	}
	return marketdata.CorporateActionEvent{
		Kind: kind, ExDate: exDate, Amount: event.Amount, Ratio: event.Ratio,
	}, nil
}

func newsSource(source string) string {
	if value := strings.TrimSpace(source); value != "" {
		return value
	}
	return "akshare-news"
}

func corporateActionsSource(source string) string {
	if value := strings.TrimSpace(source); value != "" {
		return value
	}
	return "akshare-actions"
}

func optionalText(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}
