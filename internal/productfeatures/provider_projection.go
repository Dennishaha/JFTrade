package productfeatures

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

// embeddedProviderSelectionReason explains that a product feature read was
// served by the embedded market-data provider instead of a registered broker.
const embeddedProviderSelectionReason = "embedded-market-data-provider"

// projectProviderNews converts provider-neutral news into the broker feature
// envelope. Nullable upstream fields are omitted from the entry documents.
func projectProviderNews(
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	response marketdata.NewsResponse,
	market string,
	now time.Time,
) *broker.FeatureResult {
	entries := make([]map[string]any, 0, len(response.Entries))
	latest := time.Time{}
	for _, entry := range response.Entries {
		projected := map[string]any{}
		if entry.Title != nil {
			projected["title"] = *entry.Title
		}
		if entry.Link != nil {
			projected["link"] = *entry.Link
		}
		if entry.Publisher != nil {
			projected["publisher"] = *entry.Publisher
		}
		if entry.Summary != nil {
			projected["summary"] = *entry.Summary
		}
		if entry.PublishedAt != nil {
			projected["publishedAt"] = *entry.PublishedAt
			if parsed, err := time.Parse(time.RFC3339Nano, *entry.PublishedAt); err == nil &&
				parsed.After(latest) {
				latest = parsed
			}
		}
		entries = append(entries, projected)
	}
	asOf := now
	if !latest.IsZero() {
		asOf = latest.UTC()
	}
	return embeddedFeatureResult(descriptor, query, response.InstrumentID, market, asOf, now, entries, response.Source)
}

// projectProviderCorporateActions converts dividend/split events into the
// broker feature envelope with a plain-language statement per event.
func projectProviderCorporateActions(
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	response marketdata.CorporateActionsResponse,
	market string,
	now time.Time,
) *broker.FeatureResult {
	entries := make([]map[string]any, 0, len(response.Events))
	for _, event := range response.Events {
		projected := map[string]any{}
		if kind := strings.TrimSpace(event.Kind); kind != "" {
			projected["kind"] = kind
		}
		if exDate := strings.TrimSpace(event.ExDate); exDate != "" {
			projected["exDate"] = exDate
		}
		if statement := corporateActionStatement(event); statement != "" {
			projected["statement"] = statement
		}
		entries = append(entries, projected)
	}
	return embeddedFeatureResult(descriptor, query, response.InstrumentID, market, now, now, entries, response.Source)
}

// corporateActionStatement renders the event terms the console displays, for
// example "每股派息 0.5" or "1 拆 4".
func corporateActionStatement(event marketdata.CorporateActionEvent) string {
	switch strings.ToLower(strings.TrimSpace(event.Kind)) {
	case "dividend":
		if event.Amount != nil {
			return "每股派息 " + event.Amount.String()
		}
	case "split":
		if event.Ratio != nil {
			return "1 拆 " + event.Ratio.String()
		}
	}
	return ""
}

// projectProviderRankings converts a provider-neutral ranking list into the
// broker feature envelope using the keys the Futu path emits
// (pkg/futu/adapter_research_normalization.go:119-151) and the research
// rankings views read.
func projectProviderRankings(
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	response marketdata.RankingsResponse,
	market string,
	now time.Time,
) *broker.FeatureResult {
	return embeddedFeatureResult(
		descriptor, query, "", market, now, now,
		rankingEntryDocuments(response.Entries), response.Source,
	)
}

// rankingEntryDocuments keys match the frontend consumers:
// apps/web/src/components/research/RankListPanel.vue:65-79 (instrumentId/
// symbol/name/price/changeRate), apps/web/src/components/research/
// ConceptSectorView.vue:123-129 SORT_FIELDS (price/changeAmount/changeRate/
// volume/turnover), apps/web/src/composables/research/useResearchFeature.ts:
// 317-324 (changeRate merge sorting).
func rankingEntryDocuments(entries []marketdata.RankingEntry) []map[string]any {
	documents := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		projected := map[string]any{}
		if id := strings.ToUpper(strings.TrimSpace(entry.InstrumentID)); id != "" {
			projected["instrumentId"] = id
			if prefix, code, found := strings.Cut(id, "."); found {
				projected["market"] = prefix
				projected["symbol"] = code
			}
		}
		if name := strings.TrimSpace(entry.Name); name != "" {
			projected["name"] = name
		}
		putProviderNumber(projected, "price", entry.Price)
		putProviderNumber(projected, "changeRate", entry.ChangeRate)
		putProviderNumber(projected, "changeAmount", entry.ChangeAmount)
		putProviderNumber(projected, "volume", entry.Volume)
		putProviderNumber(projected, "turnover", entry.Turnover)
		putProviderNumber(projected, "turnoverRatio", entry.TurnoverRatio)
		putProviderNumber(projected, "peTTM", entry.PETTM)
		putProviderNumber(projected, "marketCap", entry.MarketCap)
		documents = append(documents, projected)
	}
	return documents
}

// projectProviderIndustryBoards converts CN industry/concept boards into the
// broker feature envelope. Keys match the frontend consumers:
// apps/web/src/components/research/ConceptSectorView.vue:80-98 (instrumentId
// must contain "." so plate_members can derive the market; name, price,
// changeRate at :222-231) and apps/web/src/components/research/
// SectorHeatmap.vue:8,72-89 (name, changeRate, turnover weight fallback).
func projectProviderIndustryBoards(
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	response marketdata.IndustryBoardsResponse,
	market string,
	now time.Time,
) *broker.FeatureResult {
	boardMarket := strings.ToUpper(strings.TrimSpace(response.Market))
	if boardMarket == "" {
		boardMarket = market
	}
	entries := make([]map[string]any, 0, len(response.Boards))
	for _, board := range response.Boards {
		name := strings.TrimSpace(board.Name)
		projected := map[string]any{
			"instrumentId": boardMarket + "." + name,
			"market":       boardMarket,
			"name":         name,
			"productClass": string(broker.ProductClassPlate),
		}
		putProviderNumber(projected, "changeRate", board.ChangeRate)
		putProviderNumber(projected, "turnover", board.Turnover)
		putProviderNumber(projected, "volume", board.Volume)
		if leading := strings.TrimSpace(board.LeadingStockName); leading != "" {
			projected["leadingStockName"] = leading
		}
		putProviderNumber(projected, "leadingStockChangeRate", board.LeadingStockChangeRate)
		entries = append(entries, projected)
	}
	return embeddedFeatureResult(descriptor, query, "", market, now, now, entries, response.Source)
}

// projectProviderIndustryMembers converts board members into the ranking-entry
// document shape the member tables read
// (apps/web/src/components/research/ConceptSectorView.vue:275-306).
func projectProviderIndustryMembers(
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	response marketdata.IndustryMembersResponse,
	market string,
	now time.Time,
) *broker.FeatureResult {
	return embeddedFeatureResult(
		descriptor, query, "", market, now, now,
		rankingEntryDocuments(response.Entries), response.Source,
	)
}

func putProviderNumber(document map[string]any, key string, value *json.Number) {
	if value != nil {
		document[key] = *value
	}
}

func embeddedFeatureResult(
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	instrumentID string,
	market string,
	asOf time.Time,
	now time.Time,
	entries []map[string]any,
	source string,
) *broker.FeatureResult {
	total := len(entries)
	hasMore := false
	if instrumentID == "" {
		instrumentID = strings.ToUpper(strings.TrimSpace(query.InstrumentID))
	}
	var resolved *broker.Instrument
	if instrumentID != "" {
		resolved = embeddedResolvedInstrument(query, instrumentID, market)
	}
	return &broker.FeatureResult{
		Provider: broker.ProviderAttribution{
			BrokerID:        strings.TrimSpace(descriptor.BrokerID),
			FeatureID:       query.FeatureID,
			Capability:      broker.CapabilityAvailable,
			SelectionReason: embeddedProviderSelectionReason,
			ResolvedAt:      now,
			AsOf:            asOf,
		},
		ResolvedInstrument: resolved,
		AsOf:               asOf,
		Entries:            entries,
		HasMore:            &hasMore,
		Total:              &total,
		Metadata:           map[string]any{"source": source},
	}
}

func embeddedResolvedInstrument(
	query *broker.FeatureQuery,
	instrumentID string,
	market string,
) *broker.Instrument {
	code := instrumentID
	if _, value, ok := strings.Cut(instrumentID, "."); ok {
		code = value
	}
	productClass := query.ProductClass
	if productClass == "" {
		productClass = broker.ProductClassUnknown
	}
	segment := query.MarketSegment
	if segment == "" {
		segment = broker.MarketSegmentSecurities
	}
	return &broker.Instrument{
		InstrumentID:  instrumentID,
		Code:          code,
		ProductClass:  productClass,
		MarketSegment: segment,
		QuoteMarket:   market,
		TradeMarket:   market,
		QuantityMode:  broker.QuantityModeUnits,
	}
}

// projectProviderCompanyProfile flattens grouped profile fields into the
// entry stream the console reads: a {fieldType:"title",name} row opens a group
// and {fieldType:"text",name,value} rows fill it
// (apps/web/src/components/research/useInstrumentResearchController.ts:104-132).
func projectProviderCompanyProfile(
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	response marketdata.CompanyProfileResponse,
	market string,
	now time.Time,
) *broker.FeatureResult {
	entries := make([]map[string]any, 0)
	for _, group := range response.Groups {
		if title := strings.TrimSpace(group.Title); title != "" {
			entries = append(entries, map[string]any{"fieldType": "title", "name": title})
		}
		for _, field := range group.Fields {
			name, value := strings.TrimSpace(field.Name), strings.TrimSpace(field.Value)
			if name == "" && value == "" {
				continue
			}
			entries = append(entries, map[string]any{"fieldType": "text", "name": name, "value": value})
		}
	}
	return embeddedFeatureResult(
		descriptor, query, response.InstrumentID, market, now, now, entries, response.Source,
	)
}

// projectProviderFinancialStatements projects the statement table into
// metadata.structureList plus one entry per period with itemList cells
// (apps/web/src/components/research/useInstrumentResearchController.ts:134-181);
// yoy/qoq keys are omitted when the upstream feed has no comparison.
func projectProviderFinancialStatements(
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	response marketdata.FinancialStatementsResponse,
	market string,
	now time.Time,
) *broker.FeatureResult {
	structureList := make([]map[string]any, 0, len(response.Fields))
	for _, field := range response.Fields {
		structureList = append(structureList, map[string]any{
			"fieldId": field.FieldID, "displayName": field.DisplayName,
		})
	}
	entries := make([]map[string]any, 0, len(response.Periods))
	for _, period := range response.Periods {
		itemList := make([]map[string]any, 0, len(period.Values))
		for _, field := range response.Fields {
			value, ok := period.Values[field.FieldID]
			if !ok {
				continue
			}
			item := map[string]any{"fieldId": field.FieldID}
			putProviderNumber(item, "data", value.Data)
			putProviderNumber(item, "yoy", value.YoY)
			putProviderNumber(item, "qoq", value.QoQ)
			itemList = append(itemList, item)
		}
		entry := map[string]any{"periodText": period.PeriodText, "itemList": itemList}
		if response.Currency != nil {
			entry["currencyCode"] = *response.Currency
		}
		entries = append(entries, entry)
	}
	result := embeddedFeatureResult(
		descriptor, query, response.InstrumentID, market, now, now, entries, response.Source,
	)
	result.Metadata["structureList"] = structureList
	return result
}

// projectProviderAnalystConsensus projects the consensus into the single entry
// the console reads at index 0
// (apps/web/src/components/research/useInstrumentResearchController.ts:412-442,
// apps/web/src/components/research/InstrumentResearchView.vue:66-99,280-282,309).
// Nullable upstream fields are omitted from the entry.
func projectProviderAnalystConsensus(
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	response marketdata.AnalystConsensusResponse,
	market string,
	now time.Time,
) *broker.FeatureResult {
	entry := map[string]any{}
	putProviderNumber(entry, "rating", response.Rating)
	putProviderNumber(entry, "analystCount", response.AnalystCount)
	if target := response.TargetPrice; target != nil {
		putProviderNumber(entry, "lowest", target.Lowest)
		putProviderNumber(entry, "average", target.Average)
		putProviderNumber(entry, "highest", target.Highest)
	}
	if distribution := response.Distribution; distribution != nil {
		putProviderNumber(entry, "strongBuy", distribution.StrongBuy)
		putProviderNumber(entry, "buy", distribution.Buy)
		putProviderNumber(entry, "hold", distribution.Hold)
		putProviderNumber(entry, "underperform", distribution.Underperform)
		putProviderNumber(entry, "sell", distribution.Sell)
	}
	if response.UpdateTime != nil {
		if updateTime := strings.TrimSpace(*response.UpdateTime); updateTime != "" {
			entry["updateTimeStr"] = updateTime
		}
	}
	return embeddedFeatureResult(
		descriptor, query, response.InstrumentID, market, now, now,
		[]map[string]any{entry}, response.Source,
	)
}

// projectProviderOwnership splits ownership groups into the metadata lists the
// console reads: mainHolderInfoList for major_holders and holderTypeInfoList
// for holder_types
// (apps/web/src/components/research/useInstrumentResearchController.ts:443-514).
func projectProviderOwnership(
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	response marketdata.OwnershipResponse,
	market string,
	now time.Time,
) *broker.FeatureResult {
	mainHolderInfoList := make([]map[string]any, 0)
	holderTypeInfoList := make([]map[string]any, 0)
	for _, group := range response.Groups {
		itemList := make([]map[string]any, 0, len(group.Items))
		for _, item := range group.Items {
			projected := map[string]any{"name": item.Name}
			putProviderNumber(projected, "holderPct", item.HolderPct)
			itemList = append(itemList, projected)
		}
		entry := map[string]any{"itemList": itemList}
		if group.StaticDate != nil && strings.TrimSpace(*group.StaticDate) != "" {
			entry["staticDateStr"] = strings.TrimSpace(*group.StaticDate)
		}
		if group.Kind == marketdata.OwnershipGroupHolderTypes {
			holderTypeInfoList = append(holderTypeInfoList, entry)
		} else {
			mainHolderInfoList = append(mainHolderInfoList, entry)
		}
	}
	result := embeddedFeatureResult(
		descriptor, query, response.InstrumentID, market, now, now, []map[string]any{}, response.Source,
	)
	result.Metadata["mainHolderInfoList"] = mainHolderInfoList
	result.Metadata["holderTypeInfoList"] = holderTypeInfoList
	return result
}
