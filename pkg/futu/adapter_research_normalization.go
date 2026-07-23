package futu

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

// normalizeResearchProtocolPayload adds a stable, broker-neutral projection to
// OpenD research rows while retaining every original protocol field. This lets
// clients consume instrument identity and the common ranking/calendar fields
// without guessing each protocol's nested protobuf shape.
func normalizeResearchProtocolPayload(protocol string, payload map[string]any) map[string]any {
	if len(payload) == 0 || !isResearchNormalizationProtocol(protocol) {
		return payload
	}
	result := cloneMap(payload)
	if payloadContainsOnlyPaginationMetadata(result) {
		return result
	}
	result = normalizeResearchEntry(protocol, result)
	for key, raw := range result {
		values, ok := raw.([]any)
		if !ok {
			continue
		}
		normalized := make([]any, len(values))
		for index, value := range values {
			entry, ok := value.(map[string]any)
			if !ok {
				normalized[index] = value
				continue
			}
			normalized[index] = normalizeResearchEntry(protocol, cloneMap(entry))
		}
		result[key] = normalized
	}
	return result
}

func applyResearchLocalPagination(
	result *broker.FeatureResult,
	query broker.FeatureQuery,
	protocol string,
) error {
	if result == nil || query.PageSize <= 0 || !researchProtocolUsesLocalPagination(protocol) {
		return nil
	}
	offset, err := researchLocalPaginationOffset(query.Cursor)
	if err != nil {
		return err
	}
	total := len(result.Entries)
	if offset > total {
		offset = total
	}
	end := min(offset+query.PageSize, total)
	result.Entries = append([]map[string]any(nil), result.Entries[offset:end]...)
	result.Total = &total
	hasMore := end < total
	result.HasMore = &hasMore
	result.NextCursor = ""
	if hasMore {
		result.NextCursor = "local:" + strconv.Itoa(end)
	}
	return nil
}

func researchProtocolUsesLocalPagination(protocol string) bool {
	switch protocol {
	case "Qot_GetPlateSet", "Qot_GetPlateSecurity", "Qot_GetStaticInfo":
		return true
	default:
		return false
	}
}

func researchLocalPaginationOffset(cursor string) (int, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return 0, nil
	}
	value := strings.TrimPrefix(cursor, "local:")
	offset, err := strconv.Atoi(value)
	if err != nil || offset < 0 || value == cursor {
		return 0, fmt.Errorf("futu: invalid local research cursor %q", cursor)
	}
	return offset, nil
}

func isResearchNormalizationProtocol(protocol string) bool {
	return strings.HasPrefix(protocol, "Qot_GetEarnings") ||
		strings.HasPrefix(protocol, "Qot_GetDividend") ||
		strings.HasPrefix(protocol, "Qot_GetUSPreMarket") ||
		strings.HasPrefix(protocol, "Qot_GetUSAfterHours") ||
		strings.HasPrefix(protocol, "Qot_GetUSOvernight") ||
		strings.HasPrefix(protocol, "Qot_GetTopMovers") ||
		strings.HasPrefix(protocol, "Qot_GetHotList") ||
		strings.HasPrefix(protocol, "Qot_GetShortSelling") ||
		strings.HasPrefix(protocol, "Qot_GetPeriodChange") ||
		strings.HasPrefix(protocol, "Qot_GetHighDividend") ||
		strings.HasPrefix(protocol, "Qot_GetHeatMap") ||
		strings.HasPrefix(protocol, "Qot_GetRiseFall") ||
		strings.HasPrefix(protocol, "Qot_GetInstitution") ||
		strings.HasPrefix(protocol, "Qot_GetArk") ||
		strings.HasPrefix(protocol, "Qot_GetIndustrial") ||
		protocol == "Qot_GetEconomicCalendar" ||
		protocol == "Qot_GetIpoList" ||
		protocol == "Qot_GetPlateSet" ||
		protocol == "Qot_GetPlateSecurity" ||
		protocol == "Qot_GetOwnerPlate" ||
		protocol == "Qot_GetStaticInfo"
}

func normalizeResearchEntry(protocol string, entry map[string]any) map[string]any {
	if protocol == "Qot_GetIpoList" {
		flattenResearchIPO(entry)
	}
	security, source, basic := researchEntrySecurity(entry)
	if security != nil {
		entry["instrumentId"] = stringValue(security["instrumentId"])
		entry["market"] = strings.ToUpper(stringValue(security["market"]))
		entry["symbol"] = strings.ToUpper(stringValue(security["code"]))
		if name := researchEntryName(entry, basic); name != "" {
			entry["name"] = name
		}
		if productClass := researchProductClass(protocol, entry, source, basic); productClass != "" {
			entry["productClass"] = productClass
		}
	}
	if _, exists := entry["changeRate"]; !exists && entry["changeRatio"] != nil {
		entry["changeRate"] = entry["changeRatio"]
	}
	if _, exists := entry["price"]; !exists && entry["curPrice"] != nil {
		entry["price"] = entry["curPrice"]
	}
	if _, exists := entry["marketValue"]; !exists && entry["marketVal"] != nil {
		entry["marketValue"] = entry["marketVal"]
	}
	if _, exists := entry["dividendYield"]; !exists && entry["dividendYieldTTM"] != nil {
		entry["dividendYield"] = entry["dividendYieldTTM"]
	}
	normalizeResearchCalendarFields(protocol, entry)
	normalizeResearchInstitutionFields(protocol, entry)
	applyResearchContractProjection(protocol, entry)
	return entry
}

func researchEntrySecurity(entry map[string]any) (map[string]any, string, map[string]any) {
	for _, key := range []string{"plate", "security"} {
		if security, ok := entry[key].(map[string]any); ok && stringValue(security["instrumentId"]) != "" {
			return security, key, nil
		}
	}
	basic, _ := entry["basic"].(map[string]any)
	if security, ok := basic["security"].(map[string]any); ok && stringValue(security["instrumentId"]) != "" {
		return security, "basic.security", basic
	}
	return nil, "", basic
}

func researchEntryName(entry, basic map[string]any) string {
	if name := firstString(entry, "name", "plateName", "institutionName"); name != "" {
		return name
	}
	return firstString(basic, "name")
}

func researchProductClass(
	protocol string,
	entry map[string]any,
	securitySource string,
	basic map[string]any,
) string {
	if value := strings.ToLower(stringValue(entry["productClass"])); value != "" {
		return value
	}
	if securitySource == "plate" || protocol == "Qot_GetPlateSet" ||
		protocol == "Qot_GetOwnerPlate" || protocol == "Qot_GetHeatMapData" {
		return "plate"
	}
	if secType := researchSecurityType(basic["secType"]); secType != "" {
		return secType
	}
	if protocol == "Qot_GetStaticInfo" {
		return "fund"
	}
	return "equity"
}

func researchSecurityType(value any) string {
	if text := strings.ToLower(stringValue(value)); text != "" {
		switch text {
		case "eqty", "equity":
			return "equity"
		case "trust", "fund":
			return "fund"
		case "drvt", "option":
			return "option"
		case "bwrt", "warrant":
			return "warrant"
		case "index", "plate", "future", "bond", "forex", "crypto":
			return text
		}
	}
	valueInt, ok := integerValue(value)
	if !ok {
		return ""
	}
	return map[int]string{
		1: "bond", 2: "warrant", 3: "equity", 4: "fund", 5: "warrant",
		6: "index", 7: "plate", 8: "option", 9: "plate", 10: "future",
		11: "forex", 12: "crypto",
	}[valueInt]
}

func flattenResearchIPO(entry map[string]any) {
	basic, _ := entry["basic"].(map[string]any)
	copyResearchField(entry, basic, "name")
	copyResearchField(entry, basic, "listTime")
	copyResearchField(entry, basic, "listTimestamp")
	for _, key := range []string{"cnExData", "hkExData", "usExData"} {
		extra, ok := entry[key].(map[string]any)
		if !ok {
			continue
		}
		for field, value := range extra {
			if _, exists := entry[field]; !exists {
				entry[field] = value
			}
		}
	}
	if entry["issuePrice"] == nil && entry["ipoPrice"] != nil {
		entry["issuePrice"] = entry["ipoPrice"]
	}
	if entry["issuePrice"] == nil && entry["listPrice"] != nil {
		entry["issuePrice"] = entry["listPrice"]
	}
	if entry["issuePriceMin"] == nil && entry["ipoPriceMin"] != nil {
		entry["issuePriceMin"] = entry["ipoPriceMin"]
	}
	if entry["issuePriceMax"] == nil && entry["ipoPriceMax"] != nil {
		entry["issuePriceMax"] = entry["ipoPriceMax"]
	}
	if entry["listingDate"] == nil && entry["listTime"] != nil {
		entry["listingDate"] = entry["listTime"]
	}
	if entry["issueVolume"] == nil && entry["issueSize"] != nil {
		entry["issueVolume"] = entry["issueSize"]
	}
}

func copyResearchField(destination, source map[string]any, key string) {
	if destination[key] == nil && source[key] != nil {
		destination[key] = source[key]
	}
}

func normalizeResearchCalendarFields(protocol string, entry map[string]any) {
	switch protocol {
	case "Qot_GetEarningsCalendar":
		setResearchEventFields(entry, "earnings", stringValue(entry["earningsDate"]), entry["earningsTimestamp"])
		entry["calendarType"] = "earnings"
	case "Qot_GetEconomicCalendar":
		setResearchEventFields(entry, "economic", "", entry["timestamp"])
		copyResearchAlias(entry, "region", "country")
		copyResearchAlias(entry, "importance", "star")
		copyResearchAlias(entry, "previousValue", "previous")
		copyResearchAlias(entry, "forecastValue", "consensus")
		copyResearchAlias(entry, "actualValue", "actual")
	case "Qot_GetDividendCalendar":
		setResearchEventFields(entry, "dividend", stringValue(entry["exDate"]), nil)
	case "Qot_GetIpoList":
		setResearchEventFields(entry, "ipo", stringValue(entry["listingDate"]), entry["listTimestamp"])
	}
}

func setResearchEventFields(entry map[string]any, calendarType, explicitDate string, timestampValue any) {
	entry["calendarType"] = calendarType
	if explicitDate != "" {
		entry["eventDate"] = explicitDate
		if entry["date"] == nil {
			entry["date"] = explicitDate
		}
	}
	seconds, at, ok := canonicalResearchTimestamp(timestampValue)
	if !ok {
		return
	}
	entry["eventTimestamp"] = seconds
	entry["eventTime"] = at.Format(time.RFC3339Nano)
	if stringValue(entry["eventDate"]) == "" {
		entry["eventDate"] = at.Format("2006-01-02")
	}
}

func canonicalResearchTimestamp(value any) (float64, time.Time, bool) {
	timestamp, ok := researchNumber(value)
	if !ok || timestamp <= 0 {
		return 0, time.Time{}, false
	}
	if timestamp > 100_000_000_000 {
		timestamp /= 1000
	}
	seconds := int64(timestamp)
	nanoseconds := int64((timestamp - float64(seconds)) * float64(time.Second))
	return timestamp, time.Unix(seconds, nanoseconds).UTC(), true
}

func copyResearchAlias(entry map[string]any, target, source string) {
	if entry[target] == nil && entry[source] != nil {
		entry[target] = entry[source]
	}
}

func normalizeResearchInstitutionFields(protocol string, entry map[string]any) {
	if !strings.HasPrefix(protocol, "Qot_GetInstitution") {
		return
	}
	if entry["institutionId"] != nil && entry["id"] == nil {
		entry["id"] = entry["institutionId"]
	}
	if name := stringValue(entry["institutionName"]); name != "" {
		entry["name"] = name
	}
	if entry["marketValue"] == nil && entry["positionValue"] != nil {
		entry["marketValue"] = entry["positionValue"]
	}
	if entry["marketValueChange"] == nil && entry["positionValueChange"] != nil {
		entry["marketValueChange"] = entry["positionValueChange"]
	}
	if entry["holdingCount"] == nil && entry["positionCount"] != nil {
		entry["holdingCount"] = entry["positionCount"]
	}
	if entry["holdingCountChange"] == nil && entry["positionCountChange"] != nil {
		entry["holdingCountChange"] = entry["positionCountChange"]
	}
	if entry["asOfDate"] == nil && entry["disclosureDate"] != nil {
		entry["asOfDate"] = entry["disclosureDate"]
	}
}

func applyResearchContractProjection(protocol string, entry map[string]any) {
	instrument := broker.ResearchInstrumentEntry{
		InstrumentID: stringValue(entry["instrumentId"]),
		Market:       stringValue(entry["market"]),
		Symbol:       stringValue(entry["symbol"]),
		Name:         stringValue(entry["name"]),
		ProductClass: broker.ProductClass(stringValue(entry["productClass"])),
		Price:        researchFloatPointer(entry["price"]),
		ChangeRate:   researchFloatPointer(entry["changeRate"]),
	}
	mergeResearchProjection(entry, instrument)

	switch protocol {
	case "Qot_GetPlateSet", "Qot_GetOwnerPlate", "Qot_GetHeatMapData":
		mergeResearchProjection(entry, broker.ResearchPlateEntry{
			ResearchInstrumentEntry: instrument,
			MarketValue:             researchFloatPointer(entry["marketValue"]),
			RiseCount:               researchInt64Pointer(entry["riseCount"]),
			FallCount:               researchInt64Pointer(entry["fallCount"]),
			EqualCount:              researchInt64Pointer(entry["equalCount"]),
			Description:             stringValue(entry["description"]),
		})
	case "Qot_GetEarningsCalendar", "Qot_GetEconomicCalendar", "Qot_GetDividendCalendar":
		mergeResearchProjection(entry, researchCalendarProjection(entry, instrument))
	case "Qot_GetIpoList":
		mergeResearchProjection(entry, broker.ResearchIpoEntry{
			ResearchCalendarEvent: researchCalendarProjection(entry, instrument),
			ListingDate:           stringValue(entry["listingDate"]),
			IssueVolume:           researchInt64Pointer(entry["issueVolume"]),
			IssuePrice:            researchFloatPointer(entry["issuePrice"]),
			IssuePriceMin:         researchFloatPointer(entry["issuePriceMin"]),
			IssuePriceMax:         researchFloatPointer(entry["issuePriceMax"]),
		})
	}
	if strings.HasPrefix(protocol, "Qot_GetInstitution") {
		mergeResearchProjection(entry, broker.ResearchInstitutionEntry{
			InstitutionID:      researchInt64Pointer(entry["institutionId"]),
			Name:               stringValue(entry["name"]),
			MarketValue:        researchFloatPointer(entry["marketValue"]),
			MarketValueChange:  researchFloatPointer(entry["marketValueChange"]),
			HoldingCount:       researchInt64Pointer(entry["holdingCount"]),
			HoldingCountChange: researchInt64Pointer(entry["holdingCountChange"]),
			AsOfDate:           stringValue(entry["asOfDate"]),
		})
	}
}

func researchCalendarProjection(
	entry map[string]any,
	instrument broker.ResearchInstrumentEntry,
) broker.ResearchCalendarEvent {
	return broker.ResearchCalendarEvent{
		ResearchInstrumentEntry: instrument,
		CalendarType:            stringValue(entry["calendarType"]),
		Title:                   stringValue(entry["title"]),
		Region:                  stringValue(entry["region"]),
		Importance:              researchInt64Pointer(entry["importance"]),
		PreviousValue:           stringValue(entry["previousValue"]),
		ForecastValue:           stringValue(entry["forecastValue"]),
		ActualValue:             stringValue(entry["actualValue"]),
		EventTimestamp:          researchFloatPointer(entry["eventTimestamp"]),
		EventDate:               stringValue(entry["eventDate"]),
		EventTime:               stringValue(entry["eventTime"]),
	}
}

func mergeResearchProjection(entry map[string]any, projection any) {
	for key, value := range jsonSafeStructMap(projection) {
		entry[key] = value
	}
}

func researchFloatPointer(value any) *float64 {
	number, ok := researchNumber(value)
	if !ok {
		return nil
	}
	return &number
}

func researchInt64Pointer(value any) *int64 {
	number, ok := researchNumber(value)
	if !ok {
		return nil
	}
	integer := int64(number)
	return &integer
}

func researchNumber(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int32:
		return float64(number), true
	case int64:
		return float64(number), true
	case uint:
		return float64(number), true
	case uint32:
		return float64(number), true
	case uint64:
		return float64(number), true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(number), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}
