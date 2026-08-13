package productfeatures

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

// ReadContext is the stable, broker-neutral part shared by typed product reads.
type ReadContext struct {
	BrokerID           string
	AccountID          string
	TradingEnvironment string
	Market             string
	Cursor             string
	PageSize           int
}

type CalendarRequest struct {
	ReadContext
	Operation       string
	Date            string
	BeginDate       string
	EndDate         string
	Sort            string
	StockScope      string
	MarketCapMin    string
	MarketCapMax    string
	OptionVolumeMin string
	OptionVolumeMax string
	IVMin           string
	IVMax           string
	IVRankMin       string
	IVRankMax       string
	IVPercentileMin string
	IVPercentileMax string
	Refresh         bool
}

type RankingsRequest struct {
	ReadContext
	Operation string
	Direction string
	PlateType string
	Refresh   bool
}

type InstrumentResearchFamily string

const (
	InstrumentProfile         InstrumentResearchFamily = "instrument"
	InstrumentFinancials      InstrumentResearchFamily = "financials"
	InstrumentValuation       InstrumentResearchFamily = "valuation"
	InstrumentAnalyst         InstrumentResearchFamily = "analyst"
	InstrumentOwnership       InstrumentResearchFamily = "ownership"
	InstrumentCorporateAction InstrumentResearchFamily = "corporate-action"
	InstrumentShortInterest   InstrumentResearchFamily = "short-interest"
)

type InstrumentResearchRequest struct {
	ReadContext
	Family       InstrumentResearchFamily
	InstrumentID string
	Operation    string
	Refresh      bool
}

// DocumentResult is the typed internal compatibility boundary for feature
// families whose broker-specific row shapes are intentionally heterogeneous.
// Raw JSON documents preserve the public wire without allowing unconstrained
// maps to leak into the service ports.
type DocumentResult struct {
	Provider           broker.ProviderAttribution
	ResolvedInstrument *broker.Instrument
	AsOf               time.Time
	Entries            []json.RawMessage
	NextCursor         string
	HasMore            *bool
	Total              *int
	Warnings           []string
	PartialErrors      []broker.FeaturePartialError
	Metadata           json.RawMessage
}

func (s *Service) QueryCalendar(ctx context.Context, request CalendarRequest) (*DocumentResult, error) {
	return s.queryDocuments(ctx, broker.FeatureQuery{
		BrokerID: request.BrokerID, AccountID: request.AccountID,
		TradingEnvironment: request.TradingEnvironment, Market: request.Market,
		FeatureID: typedFeatureID("research.calendar"), Cursor: request.Cursor, PageSize: request.PageSize,
		Params: compactParams([]paramValue{
			{"operation", request.Operation}, {"date", request.Date},
			{"beginDate", request.BeginDate}, {"endDate", request.EndDate}, {"sort", request.Sort},
			{"stockScope", request.StockScope}, {"marketCapMin", request.MarketCapMin}, {"marketCapMax", request.MarketCapMax},
			{"optionVolumeMin", request.OptionVolumeMin}, {"optionVolumeMax", request.OptionVolumeMax},
			{"ivMin", request.IVMin}, {"ivMax", request.IVMax}, {"ivRankMin", request.IVRankMin},
			{"ivRankMax", request.IVRankMax}, {"ivPercentileMin", request.IVPercentileMin},
			{"ivPercentileMax", request.IVPercentileMax}, {"refresh", request.Refresh},
		}),
	})
}

func (s *Service) QueryRankings(ctx context.Context, request RankingsRequest) (*DocumentResult, error) {
	return s.queryDocuments(ctx, broker.FeatureQuery{
		BrokerID: request.BrokerID, AccountID: request.AccountID,
		TradingEnvironment: request.TradingEnvironment, Market: request.Market,
		FeatureID: typedFeatureID("research.rankings"), Cursor: request.Cursor, PageSize: request.PageSize,
		Params: compactParams([]paramValue{
			{"operation", request.Operation}, {"direction", request.Direction},
			{"plateType", request.PlateType}, {"refresh", request.Refresh},
		}),
	})
}

func (s *Service) QueryInstrumentResearch(
	ctx context.Context,
	request InstrumentResearchRequest,
) (*DocumentResult, error) {
	feature, ok := instrumentResearchFeature(request.Family)
	if !ok {
		return nil, errors.New("unsupported instrument research family")
	}
	return s.queryDocuments(ctx, broker.FeatureQuery{
		BrokerID: request.BrokerID, AccountID: request.AccountID,
		TradingEnvironment: request.TradingEnvironment, Market: request.Market,
		InstrumentID: request.InstrumentID, FeatureID: feature,
		Cursor: request.Cursor, PageSize: request.PageSize,
		Params: compactParams([]paramValue{{"operation", request.Operation}, {"refresh", request.Refresh}}),
	})
}

func (s *Service) QueryScreen(
	ctx context.Context,
	request broker.ScreenQueryV2,
) (broker.ResearchScreenResult, error) {
	result, err := s.Query(ctx, broker.FeatureQuery{
		BrokerID: request.BrokerID, AccountID: request.AccountID,
		TradingEnvironment: request.TradingEnvironment, Market: request.Market,
		FeatureID: typedFeatureID("research.screen"),
		Cursor:    strconv.Itoa(request.Page.Offset), PageSize: request.Page.Limit,
		Params: map[string]any{
			"operation":                "stock_v2",
			"researchScreenDefinition": request.ScreenDefinitionV2,
			"pageFrom":                 request.Page.Offset,
		},
	})
	if err != nil {
		return broker.ResearchScreenResult{}, err
	}
	return ProjectScreenResult(result)
}

func ProjectScreenResult(result *broker.FeatureResult) (broker.ResearchScreenResult, error) {
	typed := broker.ResearchScreenResult{Entries: []broker.ResearchScreenRow{}}
	if result == nil {
		return typed, nil
	}
	typed.Provider = result.Provider
	typed.AsOf = result.AsOf
	typed.Warnings = append([]string(nil), result.Warnings...)
	typed.PartialErrors = append([]broker.FeaturePartialError(nil), result.PartialErrors...)
	if result.Total != nil {
		total := *result.Total
		typed.Total = &total
	}
	if result.HasMore != nil {
		typed.HasMore = *result.HasMore
	}
	if typed.HasMore {
		next, err := strconv.Atoi(result.NextCursor)
		if err != nil || next < 0 {
			return typed, errors.New("broker returned an invalid stock-screen offset")
		}
		typed.NextOffset = &next
	}
	for _, entry := range result.Entries {
		content, err := json.Marshal(entry)
		if err != nil {
			return typed, errors.New("broker returned an invalid stock-screen row")
		}
		var row broker.ResearchScreenRow
		if err := json.Unmarshal(content, &row); err != nil {
			return typed, errors.New("broker returned an invalid stock-screen row")
		}
		if row.Cells == nil {
			row.Cells = map[string]broker.ScreenResultCell{}
		}
		typed.Entries = append(typed.Entries, row)
	}
	return typed, nil
}

func (s *Service) queryDocuments(ctx context.Context, query broker.FeatureQuery) (*DocumentResult, error) {
	result, err := s.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	return documentResult(result)
}

func documentResult(result *broker.FeatureResult) (*DocumentResult, error) {
	if result == nil {
		result = &broker.FeatureResult{Entries: []map[string]any{}}
	}
	documents := make([]json.RawMessage, 0, len(result.Entries))
	for _, entry := range result.Entries {
		content, err := json.Marshal(entry)
		if err != nil {
			return nil, err
		}
		documents = append(documents, content)
	}
	metadata, err := json.Marshal(result.Metadata)
	if err != nil {
		return nil, err
	}
	return &DocumentResult{
		Provider: result.Provider, ResolvedInstrument: result.ResolvedInstrument,
		AsOf: result.AsOf, Entries: documents, NextCursor: result.NextCursor,
		HasMore: result.HasMore, Total: result.Total,
		Warnings:      append([]string(nil), result.Warnings...),
		PartialErrors: append([]broker.FeaturePartialError(nil), result.PartialErrors...),
		Metadata:      metadata,
	}, nil
}

// FeatureResult projects the compatibility documents only at the HTTP edge.
func (result *DocumentResult) FeatureResult() (*broker.FeatureResult, error) {
	if result == nil {
		return &broker.FeatureResult{Entries: []map[string]any{}}, nil
	}
	entries := make([]map[string]any, 0, len(result.Entries))
	for _, document := range result.Entries {
		var entry map[string]any
		if err := json.Unmarshal(document, &entry); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	metadata := map[string]any(nil)
	if len(result.Metadata) > 0 && string(result.Metadata) != "null" {
		if err := json.Unmarshal(result.Metadata, &metadata); err != nil {
			return nil, err
		}
	}
	return &broker.FeatureResult{
		Provider: result.Provider, ResolvedInstrument: result.ResolvedInstrument,
		AsOf: result.AsOf, Entries: entries, NextCursor: result.NextCursor,
		HasMore: result.HasMore, Total: result.Total,
		Warnings:      append([]string(nil), result.Warnings...),
		PartialErrors: append([]broker.FeaturePartialError(nil), result.PartialErrors...),
		Metadata:      metadata,
	}, nil
}

type paramValue struct {
	name  string
	value any
}

func compactParams(values []paramValue) map[string]any {
	result := make(map[string]any)
	for _, item := range values {
		switch value := item.value.(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				result[item.name] = value
			}
		case bool:
			if value {
				result[item.name] = true
			}
		case int:
			if value != 0 {
				result[item.name] = strconv.Itoa(value)
			}
		}
	}
	return result
}

func instrumentResearchFeature(family InstrumentResearchFamily) (broker.FeatureID, bool) {
	tools := map[InstrumentResearchFamily]string{
		InstrumentProfile: "research.instrument", InstrumentFinancials: "research.financials",
		InstrumentValuation: "research.valuation", InstrumentAnalyst: "research.analyst",
		InstrumentOwnership: "research.ownership", InstrumentCorporateAction: "research.corporate_actions",
		InstrumentShortInterest: "research.short_interest",
	}
	toolName, ok := tools[family]
	if !ok {
		return "", false
	}
	return typedFeatureID(toolName), true
}
