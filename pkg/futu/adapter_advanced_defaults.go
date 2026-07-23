package futu

import (
	"fmt"
	"math"
	"strings"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func injectAdvancedProtocolDefaults(params map[string]any, protocol string, query broker.FeatureQuery) error {
	if handled, err := injectAdvancedOptionDefaults(params, protocol, query); handled {
		return err
	}
	return injectAdvancedResearchDefaults(params, protocol, query)
}

func injectAdvancedOptionDefaults(
	params map[string]any,
	protocol string,
	query broker.FeatureQuery,
) (bool, error) {
	switch protocol {
	case "Qot_GetOptionChain":
		injectOptionChainDates(params)
	case "Qot_OptionScreen":
		if params["marketCategoryList"] == nil {
			category := 0
			if strings.EqualFold(query.Market, "HK") {
				category = 3
			}
			params["marketCategoryList"] = []any{category}
		}
	case "Qot_GetOptionEvent", "Qot_GetOptionZeroDteScreener",
		"Qot_GetOptionEarningsScreener", "Qot_GetOptionSellerScreener":
		return true, injectOptionEventDefaults(params, protocol, query)
	case "Qot_GetOptionZeroDteContract":
		return true, injectZeroDteContractParams(params, query)
	case "Qot_GetOptionMarketStatistic":
		injectOptionMarketStatisticDefaults(params, query.Market)
	case "Qot_GetOptionUnderlyingHisStatistic", "Qot_GetOptionUnderlyingHisVolatility":
		injectHistoricalDateRange(params)
	case "Qot_GetWarrant":
		injectWarrantDefaults(params)
	case "Qot_WarrantScreen":
		if params["marketType"] == nil {
			params["marketType"] = 1
		}
	case "Qot_GetMacroIndicatorList":
		if params["region"] == nil {
			params["region"] = macroRegion(query.Market)
		}
	case "Qot_GetTopMoversRank":
		return true, translateTopMoversDirection(params)
	case "Qot_GetHighDividendSOERank":
		// OpenD 10.9.6908 exposes no market field for this protocol and the
		// complete result set is HK securities. Reject a misleading SH/SZ
		// scope instead of relabelling those rows as an A-share ranking.
		if !strings.EqualFold(strings.TrimSpace(query.Market), "HK") {
			return true, fmt.Errorf(
				"futu: high_dividend_state is available only for HK; OpenD does not accept a market filter",
			)
		}
	default:
		return false, nil
	}
	return true, nil
}

func injectAdvancedResearchDefaults(
	params map[string]any,
	protocol string,
	query broker.FeatureQuery,
) error {
	switch protocol {
	case "Qot_GetHeatMapData":
		return translateHeatMapPlateType(params)
	case "Qot_GetPlateSet":
		if err := translatePlateSetType(params); err != nil {
			return err
		}
		if strings.TrimSpace(query.Market) == "" || params["market"] == nil {
			return fmt.Errorf("futu: plate_list requires market")
		}
	case "Qot_GetPlateSecurity":
		if params["plate"] == nil {
			return fmt.Errorf("futu: plate_members requires an exact plate instrumentId")
		}
	case "Qot_GetStaticInfo":
		if strings.TrimSpace(query.Market) == "" || params["market"] == nil {
			return fmt.Errorf("futu: fund_catalog requires market")
		}
		// This protocol is mapped to research.rankings/fund_catalog only. Keep
		// the catalog bounded to funds/trusts even if a caller supplies another
		// security type.
		params["secType"] = 4
	case "Qot_GetEconomicCalendar":
		if strings.TrimSpace(stringValue(params["beginDate"])) == "" {
			return fmt.Errorf("futu: economic calendar requires beginDate")
		}
		if params["marketList"] == nil && strings.TrimSpace(query.Market) != "" {
			market, err := futuQotMarketForCode(query.Market)
			if err != nil {
				return err
			}
			params["marketList"] = []any{int32(market)}
		}
	case "Qot_GetDividendCalendar":
		if strings.TrimSpace(stringValue(params["date"])) == "" {
			return fmt.Errorf("futu: dividend calendar requires date")
		}
	case "Qot_GetInstitutionProfile", "Qot_GetInstitutionDistribution",
		"Qot_GetInstitutionHoldingChange", "Qot_GetInstitutionHoldingList":
		institutionID, ok := researchNumber(params["institutionId"])
		if !ok || institutionID <= 0 || institutionID > math.MaxInt32 ||
			institutionID != math.Trunc(institutionID) {
			return fmt.Errorf("futu: %s requires a positive integer institutionId", protocol)
		}
		params["institutionId"] = int32(institutionID)
	case "Qot_GetSearchNews":
		if strings.TrimSpace(stringValue(params["keyword"])) == "" {
			keyword := strings.TrimPrefix(query.InstrumentID, query.Market+".")
			if keyword == "" {
				return fmt.Errorf("futu: news search requires keyword or instrumentId")
			}
			params["keyword"] = keyword
		}
	case "Qot_GetEventContractKline":
		if params["klineSource"] == nil {
			params["klineSource"] = 1
		}
	}
	return nil
}

func translateTopMoversDirection(params map[string]any) error {
	value := strings.ToLower(strings.TrimSpace(stringValue(params["direction"])))
	delete(params, "direction")
	if value == "" {
		return nil
	}
	switch value {
	case "up", "gainers", "descending":
		params["sortDir"] = 0
	case "down", "losers", "ascending":
		params["sortDir"] = 1
	default:
		return fmt.Errorf("futu: unsupported top movers direction %q", value)
	}
	return nil
}

func translateHeatMapPlateType(params map[string]any) error {
	value, exists := params["plateType"]
	if !exists {
		return nil
	}
	text := strings.ToLower(strings.TrimSpace(stringValue(value)))
	if text == "" {
		number, ok := boundedResearchEnum(value, 0, 2)
		if !ok {
			return fmt.Errorf("futu: unsupported heatmap plateType %v", value)
		}
		params["plateType"] = number
		return nil
	}
	switch text {
	case "industry":
		params["plateType"] = 0
	case "concept":
		params["plateType"] = 1
	case "theme":
		params["plateType"] = 2
	default:
		return fmt.Errorf("futu: unsupported heatmap plateType %q", text)
	}
	return nil
}

func translatePlateSetType(params map[string]any) error {
	value, exists := params["plateSetType"]
	if !exists {
		value, exists = params["plateType"]
	}
	delete(params, "plateType")
	if !exists {
		return fmt.Errorf("futu: plate_list requires plateType")
	}
	text := strings.ToLower(strings.TrimSpace(stringValue(value)))
	if text == "" {
		number, ok := boundedResearchEnum(value, 0, 3)
		if !ok {
			return fmt.Errorf("futu: unsupported plateType %v", value)
		}
		params["plateSetType"] = number
		return nil
	}
	switch text {
	case "all":
		params["plateSetType"] = 0
	case "industry":
		params["plateSetType"] = 1
	case "region":
		params["plateSetType"] = 2
	case "concept":
		params["plateSetType"] = 3
	default:
		return fmt.Errorf("futu: unsupported plateType %q", text)
	}
	return nil
}

func boundedResearchEnum(value any, minimum, maximum int) (int, bool) {
	number, ok := researchNumber(value)
	if !ok || number != float64(int(number)) {
		return 0, false
	}
	integer := int(number)
	return integer, integer >= minimum && integer <= maximum
}
