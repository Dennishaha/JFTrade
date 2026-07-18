package opend

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	_ "github.com/jftrade/jftrade-main/pkg/futu/pb/registerall"
)

// AdvancedProtocol is an allowlisted OpenD request/response pair. Package is
// the protobuf package name, not a Go import path.
type AdvancedProtocol struct {
	Key     string `json:"key"`
	ID      uint32 `json:"id"`
	Package string `json:"package"`
}

func advancedProtocol(key string, id uint32) AdvancedProtocol {
	return AdvancedProtocol{Key: key, ID: id, Package: key}
}

// AdvancedProtocols maps the OpenD 10.9 product/research/customization surface.
// Push-only messages are intentionally omitted from this request dispatcher.
var AdvancedProtocols = []AdvancedProtocol{
	advancedProtocol("Qot_GetRT", 3008),
	advancedProtocol("Qot_GetTicker", 3010),
	advancedProtocol("Qot_GetBroker", 3014),
	advancedProtocol("Qot_GetReference", 3206),
	advancedProtocol("Qot_GetOwnerPlate", 3207),
	advancedProtocol("Qot_GetHoldingChangeList", 3208),
	advancedProtocol("Qot_GetOptionChain", 3209),
	advancedProtocol("Qot_GetWarrant", 3210),
	advancedProtocol("Qot_GetCapitalFlow", 3211),
	advancedProtocol("Qot_GetCapitalDistribution", 3212),
	advancedProtocol("Qot_ModifyUserSecurity", 3214),
	advancedProtocol("Qot_StockFilter", 3215),
	advancedProtocol("Qot_GetCodeChange", 3216),
	advancedProtocol("Qot_GetIpoList", 3217),
	advancedProtocol("Qot_GetFutureInfo", 3218),
	advancedProtocol("Qot_RequestTradeDate", 3219),
	advancedProtocol("Qot_SetPriceReminder", 3220),
	advancedProtocol("Qot_GetPriceReminder", 3221),
	advancedProtocol("Qot_GetMarketState", 3223),
	advancedProtocol("Qot_GetOptionExpirationDate", 3224),
	advancedProtocol("Qot_GetFinancialsEarningsPriceMove", 3225),
	advancedProtocol("Qot_GetFinancialsEarningsPriceHistory", 3226),
	advancedProtocol("Qot_GetFinancialsStatements", 3227),
	advancedProtocol("Qot_GetFinancialsRevenueBreakdown", 3228),
	advancedProtocol("Qot_GetResearchAnalystConsensus", 3229),
	advancedProtocol("Qot_GetResearchRatingSummary", 3230),
	advancedProtocol("Qot_GetResearchMorningstarReport", 3231),
	advancedProtocol("Qot_GetValuationDetail", 3232),
	advancedProtocol("Qot_GetValuationPlateStockList", 3233),
	advancedProtocol("Qot_GetCorporateActionsDividends", 3234),
	advancedProtocol("Qot_GetCorporateActionsBuybacks", 3235),
	advancedProtocol("Qot_GetCorporateActionsStockSplits", 3236),
	advancedProtocol("Qot_GetShareholdersOverview", 3237),
	advancedProtocol("Qot_GetShareholdersHoldingChanges", 3238),
	advancedProtocol("Qot_GetShareholdersHolderDetail", 3239),
	advancedProtocol("Qot_GetShareholdersInstitutional", 3240),
	advancedProtocol("Qot_GetInsiderHolderList", 3241),
	advancedProtocol("Qot_GetInsiderTradeList", 3242),
	advancedProtocol("Qot_GetCompanyProfile", 3243),
	advancedProtocol("Qot_GetCompanyExecutives", 3244),
	advancedProtocol("Qot_GetCompanyExecutiveBackground", 3245),
	advancedProtocol("Qot_GetCompanyOperationalEfficiency", 3246),
	advancedProtocol("Qot_GetTopTenBuySellBrokers", 3247),
	advancedProtocol("Qot_GetDailyShortVolume", 3248),
	advancedProtocol("Qot_GetShortInterest", 3249),
	advancedProtocol("Qot_GetOptionVolatility", 3250),
	advancedProtocol("Qot_GetOptionExerciseProbability", 3251),
	advancedProtocol("Qot_StockScreen", 3252),
	advancedProtocol("Qot_OptionScreen", 3253),
	advancedProtocol("Qot_WarrantScreen", 3254),
	advancedProtocol("Qot_GetOptionQuote", 3255),
	advancedProtocol("Qot_GetOptionStrategy", 3256),
	advancedProtocol("Qot_GetOptionStrategyAnalysis", 3257),
	advancedProtocol("Qot_GetOptionStrategySpread", 3258),
	advancedProtocol("Qot_GetIndicatorList", 3259),
	advancedProtocol("Qot_RequestIndicatorCalc", 3260),
	advancedProtocol("Qot_GetSearchNews", 3263),
	advancedProtocol("Qot_GetOptionMarketStatistic", 3301),
	advancedProtocol("Qot_GetOptionUnderlyingHisStatistic", 3302),
	advancedProtocol("Qot_GetOptionUnderlyingOverview", 3303),
	advancedProtocol("Qot_GetOptionUnderlyingHisVolatility", 3304),
	advancedProtocol("Qot_GetOptionUnderlyingRank", 3305),
	advancedProtocol("Qot_GetOptionRank", 3306),
	advancedProtocol("Qot_GetOptionEvent", 3307),
	advancedProtocol("Qot_GetOptionEventAlert", 3308),
	advancedProtocol("Qot_SetOptionEventAlert", 3309),
	advancedProtocol("Qot_GetOptionZeroDteScreener", 3311),
	advancedProtocol("Qot_GetOptionZeroDteContract", 3312),
	advancedProtocol("Qot_GetOptionEarningsScreener", 3313),
	advancedProtocol("Qot_GetOptionSellerScreener", 3314),
	advancedProtocol("Qot_GetEarningsCalendar", 3401),
	advancedProtocol("Qot_GetMacroIndicatorList", 3402),
	advancedProtocol("Qot_GetMacroIndicatorHistory", 3403),
	advancedProtocol("Qot_GetFedWatchTargetRate", 3404),
	advancedProtocol("Qot_GetFedWatchDotPlot", 3405),
	advancedProtocol("Qot_GetEarningsBeatRank", 3406),
	advancedProtocol("Qot_GetDividendRank", 3407),
	advancedProtocol("Qot_GetDividendCalendar", 3408),
	advancedProtocol("Qot_GetEconomicCalendar", 3409),
	advancedProtocol("Qot_GetUSPreMarketRank", 3410),
	advancedProtocol("Qot_GetUSAfterHoursRank", 3411),
	advancedProtocol("Qot_GetUSOvernightRank", 3412),
	advancedProtocol("Qot_GetTopMoversRank", 3413),
	advancedProtocol("Qot_GetHotList", 3414),
	advancedProtocol("Qot_GetShortSellingRank", 3415),
	advancedProtocol("Qot_GetPeriodChangeRank", 3416),
	advancedProtocol("Qot_GetHighDividendSOERank", 3417),
	advancedProtocol("Qot_GetInstitutionList", 3418),
	advancedProtocol("Qot_GetInstitutionProfile", 3419),
	advancedProtocol("Qot_GetInstitutionDistribution", 3420),
	advancedProtocol("Qot_GetInstitutionHoldingChange", 3421),
	advancedProtocol("Qot_GetInstitutionHoldingList", 3422),
	advancedProtocol("Qot_GetArkFundHolding", 3423),
	advancedProtocol("Qot_GetArkStockDynamic", 3424),
	advancedProtocol("Qot_GetArkActiveTransaction", 3425),
	advancedProtocol("Qot_GetRatingChange", 3426),
	advancedProtocol("Qot_GetIndustrialChainList", 3427),
	advancedProtocol("Qot_GetIndustrialChainDetail", 3428),
	advancedProtocol("Qot_GetIndustrialChainByPlate", 3429),
	advancedProtocol("Qot_GetIndustrialPlateInfo", 3430),
	advancedProtocol("Qot_GetIndustrialPlateStock", 3431),
	advancedProtocol("Qot_GetHeatMapData", 3432),
	advancedProtocol("Qot_GetRiseFallDistribution", 3433),
	advancedProtocol("Qot_GetEventContractCategory", 3434),
	advancedProtocol("Qot_FilterCompetition", 3435),
	advancedProtocol("Qot_GetEventContractSeriesList", 3436),
	advancedProtocol("Qot_GetEventContractEventList", 3437),
	advancedProtocol("Qot_GetEventContract", 3438),
	advancedProtocol("Qot_GetEventContractMilestoneList", 3439),
	advancedProtocol("Qot_GetEventContractSnapshot", 3445),
	advancedProtocol("Qot_GetEventContractOrderBook", 3446),
	advancedProtocol("Qot_GetEventContractKline", 3447),
	advancedProtocol("Qot_GetEventContractTicker", 3448),
	advancedProtocol("Qot_GetEventContractComboList", 3453),
	advancedProtocol("Qot_GetEventContractComboRfq", 3454),
	advancedProtocol("Qot_SubEventContract", 3455),
	advancedProtocol("Qot_RequestHistoryEventContractKL", 3456),
	advancedProtocol("Trd_GetComboMaxTrdQtys", 2112),
	advancedProtocol("Trd_PlaceComboOrder", 2227),
}

var advancedProtocolByKey = buildAdvancedProtocolIndex()

func buildAdvancedProtocolIndex() map[string]AdvancedProtocol {
	result := make(map[string]AdvancedProtocol, len(AdvancedProtocols))
	for _, protocol := range AdvancedProtocols {
		if _, exists := result[protocol.Key]; exists {
			panic("duplicate advanced OpenD protocol " + protocol.Key)
		}
		result[protocol.Key] = protocol
	}
	return result
}

func AdvancedProtocolKeys() []string {
	keys := make([]string, 0, len(advancedProtocolByKey))
	for key := range advancedProtocolByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// AdvancedC2SHasField reports whether the generated protocol request accepts a
// JSON field in its C2S payload. Broker-neutral adapters use this to translate
// common pagination without leaking unsupported fields into strict protojson.
func AdvancedC2SHasField(key, field string) bool {
	spec, ok := advancedProtocolByKey[strings.TrimSpace(key)]
	if !ok {
		return false
	}
	request, err := newRegisteredMessage(spec.Package + ".Request")
	if err != nil {
		return false
	}
	c2sField := request.ProtoReflect().Descriptor().Fields().ByName("c2s")
	if c2sField == nil || c2sField.Message() == nil {
		return false
	}
	fields := c2sField.Message().Fields()
	if fields.ByName(protoreflect.Name(field)) != nil {
		return true
	}
	for index := 0; index < fields.Len(); index++ {
		if fields.Get(index).JSONName() == field {
			return true
		}
	}
	return false
}

// ValidateAdvancedC2S applies the same strict generated-message validation as
// CallAdvanced without performing network I/O.
func ValidateAdvancedC2S(key string, c2s map[string]any) error {
	spec, ok := advancedProtocolByKey[strings.TrimSpace(key)]
	if !ok {
		return fmt.Errorf("opend advanced protocol %q is not allowlisted", key)
	}
	request, err := newRegisteredMessage(spec.Package + ".Request")
	if err != nil {
		return err
	}
	requestJSON, err := json.Marshal(map[string]any{"c2s": c2s})
	if err != nil {
		return fmt.Errorf("marshal %s request: %w", key, err)
	}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(requestJSON, request); err != nil {
		return fmt.Errorf("build %s request: %w", key, err)
	}
	return nil
}

// CallAdvanced executes an allowlisted OpenD 10.9 request using registered
// descriptors, validates the common response envelope, and returns the S2C
// payload as ordinary JSON-compatible values.
func (c *Client) CallAdvanced(ctx context.Context, key string, c2s map[string]any) (map[string]any, error) {
	spec, ok := advancedProtocolByKey[strings.TrimSpace(key)]
	if !ok {
		return nil, fmt.Errorf("opend advanced protocol %q is not allowlisted", key)
	}
	request, err := newRegisteredMessage(spec.Package + ".Request")
	if err != nil {
		return nil, err
	}
	response, err := newRegisteredMessage(spec.Package + ".Response")
	if err != nil {
		return nil, err
	}
	requestJSON, err := json.Marshal(map[string]any{"c2s": c2s})
	if err != nil {
		return nil, fmt.Errorf("marshal %s request: %w", key, err)
	}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(requestJSON, request); err != nil {
		return nil, fmt.Errorf("build %s request: %w", key, err)
	}
	if err := c.Call(ctx, spec.ID, request, response); err != nil {
		return nil, err
	}
	if err := validateAdvancedResponse(key, response); err != nil {
		return nil, err
	}
	return advancedResponsePayload(key, response)
}

func newRegisteredMessage(name string) (proto.Message, error) {
	messageType, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(name))
	if err != nil {
		return nil, fmt.Errorf("registered protobuf message %s: %w", name, err)
	}
	return messageType.New().Interface(), nil
}

func validateAdvancedResponse(key string, response proto.Message) error {
	message := response.ProtoReflect()
	retTypeField := message.Descriptor().Fields().ByName("retType")
	if retTypeField == nil {
		return fmt.Errorf("opend %s response has no retType", key)
	}
	retType := int32(message.Get(retTypeField).Int())
	if retType == 0 {
		return nil
	}
	var errCode int32
	if field := message.Descriptor().Fields().ByName("errCode"); field != nil {
		errCode = int32(message.Get(field).Int())
	}
	var retMsg string
	if field := message.Descriptor().Fields().ByName("retMsg"); field != nil {
		retMsg = message.Get(field).String()
	}
	return fmt.Errorf("opend %s retType=%d errCode=%d retMsg=%s", key, retType, errCode, retMsg)
}

func advancedResponsePayload(key string, response proto.Message) (map[string]any, error) {
	content, err := (protojson.MarshalOptions{UseProtoNames: false}).Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("marshal %s response: %w", key, err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(content, &envelope); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", key, err)
	}
	payload, _ := envelope["s2c"].(map[string]any)
	if payload == nil {
		return map[string]any{}, nil
	}
	return payload, nil
}
