package futu

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdgetcombomaxpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetcombomaxtrdqtys"
	trdmodifyorderpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdmodifyorder"
	trdplacecombopb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdplacecomboorder"
)

var directBrokerProtocols = map[string]broker.FeatureID{
	"Trd_GetComboMaxTrdQtys": broker.FeatureExecutionComboPreview,
	"Trd_PlaceComboOrder":    broker.FeatureExecutionComboPlace,
}

func (a *futuAdapter) ValidateProductOrder(
	ctx context.Context,
	query broker.ProductRuleQuery,
) (*broker.ProductRuleResult, error) {
	if query.OrderKind == broker.OrderKindEventSingle {
		if query.Instrument.ProductClass != broker.ProductClassEventContract ||
			query.Instrument.MarketSegment != broker.MarketSegmentPrediction {
			return deniedProductRule("PRODUCT_MISMATCH", "event order requires an event-contract instrument"), nil
		}
		if !strings.EqualFold(query.Instrument.TradeMarket, "US") &&
			!strings.EqualFold(query.Instrument.QuoteMarket, "US") {
			return deniedProductRule("MARKET_MISMATCH", "prediction contracts trade in the US market"), nil
		}
		if query.Amount == nil || *query.Amount <= 0 {
			return deniedProductRule("INVALID_AMOUNT", "event-contract amount must be positive"), nil
		}
		if query.Price == nil || *query.Price < 0.01 || *query.Price > 0.99 {
			return deniedProductRule("INVALID_PRICE", "event-contract price must be between 0.01 and 0.99"), nil
		}
		if strings.ToUpper(strings.TrimSpace(query.OrderType)) != "LIMIT" {
			return deniedProductRule("INVALID_ORDER_TYPE", "event-contract orders require LIMIT order type"), nil
		}
		if err := a.validateActiveEventContracts(ctx, []string{query.Instrument.InstrumentID}); err != nil {
			return deniedProductRule("EVENT_NOT_TRADABLE", err.Error()), nil
		}
	}
	if query.Instrument.ProductClass == broker.ProductClassOption ||
		query.Instrument.ProductClass == broker.ProductClassFuture {
		if query.Quantity == nil || *query.Quantity <= 0 || *query.Quantity != float64(int64(*query.Quantity)) {
			return deniedProductRule("INVALID_CONTRACT_QUANTITY", "derivative quantity must be a positive integer"), nil
		}
	}
	if query.Instrument.ProductClass == broker.ProductClassOption &&
		strings.TrimSpace(query.Session) != "" {
		return deniedProductRule("INVALID_SESSION", "option orders do not inherit stock extended-hours sessions"), nil
	}
	return &broker.ProductRuleResult{Allowed: true}, nil
}

func deniedProductRule(code, reason string) *broker.ProductRuleResult {
	return &broker.ProductRuleResult{Allowed: false, ReasonCode: code, Reason: reason}
}

func (a *futuAdapter) PreviewComboOrder(
	ctx context.Context,
	intent broker.ComboOrderIntent,
) (*broker.ProductRuleResult, error) {
	kind, err := validateComboIntent(intent)
	if err != nil {
		return nil, err
	}
	if kind == broker.OrderKindEventParlay {
		if err := a.validateActiveEventContracts(ctx, comboInstrumentIDs(intent.Legs)); err != nil {
			return deniedProductRule("EVENT_NOT_TRADABLE", err.Error()), nil
		}
		return &broker.ProductRuleResult{Allowed: true}, nil
	}
	legs, err := futuComboLegs(intent.Legs, false)
	if err != nil {
		return nil, err
	}
	// validateComboIntent has already validated and normalized the strategy.
	optionStrategy, _ := futuOptionStrategyValue(intent.OptionStrategy)
	owner, _, err := futuSecurityFromSymbol(intent.UnderlyingID)
	if err != nil {
		return nil, fmt.Errorf("futu: option combo underlying: %w", err)
	}
	var result *broker.ProductRuleResult
	if err := a.exchange.withClient(ctx, func(client *opend.Client) error {
		legality, legalityErr := a.validateOptionComboLegality(
			ctx, client, intent, owner, optionStrategy, legs,
		)
		if legalityErr != nil {
			return legalityErr
		}
		if legality != nil {
			result = legality
			return nil
		}
		resolved, resolveErr := a.exchange.resolveTradeAccountWithClient(ctx, client, BrokerReadQuery{
			AccountID:          intent.AccountID,
			TradingEnvironment: intent.TradingEnvironment,
			Market:             intent.Market,
		})
		if resolveErr != nil {
			return resolveErr
		}
		quantity := comboQuantity(intent)
		maximum, queryErr := client.GetComboMaxTrdQtys(ctx, &trdgetcombomaxpb.C2S{
			Header:    resolved.header(),
			ComboLegs: legs,
			Qty:       new(quantity),
			Price:     intent.Price,
			OrderType: new(int32(trdcommonpb.OrderType_OrderType_Normal)),
		})
		if queryErr != nil {
			return queryErr
		}
		analysis, analysisErr := client.CallAdvanced(ctx, "Qot_GetOptionStrategyAnalysis", map[string]any{
			"multiLegs": comboLegMaps(legs),
		})
		if analysisErr != nil {
			return analysisErr
		}
		result = &broker.ProductRuleResult{
			Allowed:           true,
			BuyingPowerImpact: cloneFloat64Ptr(maximum.BuyPowerDecrease),
			AccountImpact: &broker.OptionComboAccountImpact{
				NLVChange:               cloneFloat64Ptr(maximum.NlvChange),
				InitialMarginChange:     cloneFloat64Ptr(maximum.InitialMarginChange),
				MaintenanceMarginChange: cloneFloat64Ptr(maximum.MaintenanceMarginChange),
				OptionBuyingPower:       cloneFloat64Ptr(maximum.OptionBuyPower),
				MaxWithdrawalChange:     cloneFloat64Ptr(maximum.MaxWithDrawChange),
				BuyingPowerDecrease:     cloneFloat64Ptr(maximum.BuyPowerDecrease),
			},
			OptionAnalysis: optionComboAnalysis(intent.OptionStrategy, analysis),
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func futuOptionStrategyValue(value string) (int32, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "vertical":
		return int32(qotcommonpb.OptionStrategyType_OptionStrategyType_Spread), nil
	case "straddle":
		return int32(qotcommonpb.OptionStrategyType_OptionStrategyType_Straddle), nil
	case "strangle":
		return int32(qotcommonpb.OptionStrategyType_OptionStrategyType_Strangle), nil
	case "butterfly":
		return int32(qotcommonpb.OptionStrategyType_OptionStrategyType_Butterfly), nil
	case "calendar":
		return int32(qotcommonpb.OptionStrategyType_OptionStrategyType_CalendarSpread), nil
	default:
		return 0, fmt.Errorf("futu: unsupported option combo strategy %q", value)
	}
}

func (a *futuAdapter) validateOptionComboLegality(
	ctx context.Context,
	client *opend.Client,
	intent broker.ComboOrderIntent,
	owner *qotcommonpb.Security,
	optionStrategy int32,
	legs []*qotcommonpb.ComboLeg,
) (*broker.ProductRuleResult, error) {
	params := map[string]any{
		"owner": map[string]any{
			"market": owner.GetMarket(),
			"code":   owner.GetCode(),
		},
		"optionStrategy": optionStrategy,
		"expireTime":     intent.NearExpiry,
	}
	if intent.FarExpiry != "" {
		params["farExpireTime"] = intent.FarExpiry
	}
	if optionStrategySupportsSpread(optionStrategy) {
		if intent.Spread == nil || *intent.Spread <= 0 {
			return deniedProductRule(
				"INVALID_OPTION_SPREAD",
				"the selected option strategy requires a positive spread",
			), nil
		}
		payload, err := client.CallAdvanced(ctx, "Qot_GetOptionStrategySpread", params)
		if err != nil {
			return nil, err
		}
		if !containsFloat(numberSlice(payload["spreadList"]), *intent.Spread) {
			return deniedProductRule(
				"ILLEGAL_OPTION_SPREAD",
				fmt.Sprintf("spread %.6g is not legal for the selected strategy and expiry", *intent.Spread),
			), nil
		}
		return nil, nil
	}

	payload, err := client.CallAdvanced(ctx, "Qot_GetOptionStrategy", params)
	if err != nil {
		return nil, err
	}
	if !containsOptionStrategyLegs(objectSlice(payload["strategyList"]), legs) {
		return deniedProductRule(
			"ILLEGAL_OPTION_COMBINATION",
			"the selected contracts are not a legal OpenD option strategy for the requested expiries",
		), nil
	}
	return nil, nil
}

func optionStrategySupportsSpread(strategy int32) bool {
	switch qotcommonpb.OptionStrategyType(strategy) {
	case qotcommonpb.OptionStrategyType_OptionStrategyType_Spread,
		qotcommonpb.OptionStrategyType_OptionStrategyType_Strangle,
		qotcommonpb.OptionStrategyType_OptionStrategyType_Butterfly:
		return true
	default:
		return false
	}
}

func comboLegMaps(legs []*qotcommonpb.ComboLeg) []any {
	result := make([]any, 0, len(legs))
	for _, leg := range legs {
		result = append(result, map[string]any{
			"security": map[string]any{
				"market": leg.GetSecurity().GetMarket(),
				"code":   leg.GetSecurity().GetCode(),
			},
			"side": leg.GetSide(), "qtyRatio": leg.GetQtyRatio(),
		})
	}
	return result
}

func numberSlice(raw any) []float64 {
	items, _ := raw.([]any)
	result := make([]float64, 0, len(items))
	for _, item := range items {
		if value, ok := item.(float64); ok {
			result = append(result, value)
		}
	}
	return result
}

func containsFloat(values []float64, expected float64) bool {
	for _, value := range values {
		difference := value - expected
		if difference < 0 {
			difference = -difference
		}
		if difference <= 1e-8 {
			return true
		}
	}
	return false
}

func containsOptionStrategyLegs(strategies []map[string]any, expected []*qotcommonpb.ComboLeg) bool {
	expectedKeys := comboLegKeys(comboLegMaps(expected))
	for _, strategy := range strategies {
		if equalStringSlices(comboLegKeys(objectSlice(strategy["multiLegs"])), expectedKeys) {
			return true
		}
	}
	return false
}

func comboLegKeys(rawLegs any) []string {
	var legs []any
	switch typed := rawLegs.(type) {
	case []any:
		legs = typed
	case []map[string]any:
		legs = make([]any, 0, len(typed))
		for _, leg := range typed {
			legs = append(legs, leg)
		}
	}
	keys := make([]string, 0, len(legs))
	for _, raw := range legs {
		leg, _ := raw.(map[string]any)
		security, _ := leg["security"].(map[string]any)
		code := strings.ToUpper(strings.TrimSpace(fmt.Sprint(security["code"])))
		keys = append(keys, fmt.Sprintf(
			"%s|%d|%.8g", code, int(numericValue(leg["side"])), numericValue(leg["qtyRatio"]),
		))
	}
	sort.Strings(keys)
	return keys
}

func numericValue(raw any) float64 {
	switch value := raw.(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int32:
		return float64(value)
	case int64:
		return float64(value)
	default:
		return 0
	}
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func optionComboAnalysis(strategy string, payload map[string]any) *broker.OptionComboAnalysis {
	if payload == nil {
		return nil
	}
	maxProfit, maxProfitUnlimited := optionComboBound(payload["maxProfit"])
	maxLoss, maxLossUnlimited := optionComboBound(payload["maxLoss"])
	return &broker.OptionComboAnalysis{
		Strategy: strategy,
		Bid:      numberPointer(payload["bid1"]), Ask: numberPointer(payload["ask1"]),
		MaxProfit: maxProfit, MaxLoss: maxLoss,
		MaxProfitUnlimited: maxProfitUnlimited, MaxLossUnlimited: maxLossUnlimited,
		BreakevenPoints: numberSlice(payload["breakevenPoints"]),
		Probability:     numberPointer(payload["probOfProfit"]),
		Delta:           numberPointer(payload["delta"]), Theta: numberPointer(payload["theta"]),
	}
}

func optionComboBound(raw any) (*float64, bool) {
	value := numberPointer(raw)
	if value == nil {
		return nil, false
	}
	if *value >= 9_999_999 {
		return nil, true
	}
	return value, false
}

func numberPointer(raw any) *float64 {
	value, ok := raw.(float64)
	if !ok {
		return nil
	}
	return &value
}

func (a *futuAdapter) PlaceComboOrder(
	ctx context.Context,
	intent broker.ComboOrderIntent,
) (*broker.ComboOrderResult, error) {
	if strings.TrimSpace(intent.PreviewID) == "" {
		return nil, fmt.Errorf("futu: combo order requires previewId")
	}
	kind, err := validateComboIntent(intent)
	if err != nil {
		return nil, err
	}
	eventContract := kind == broker.OrderKindEventParlay
	if eventContract {
		if err := a.validateActiveEventContracts(ctx, comboInstrumentIDs(intent.Legs)); err != nil {
			return nil, err
		}
	}
	legs, err := futuComboLegs(intent.Legs, eventContract)
	if err != nil {
		return nil, err
	}
	var result broker.ComboOrderResult
	if err := a.exchange.withClient(ctx, func(client *opend.Client) error {
		resolved, resolveErr := a.exchange.resolveTradeAccountWithClient(ctx, client, BrokerReadQuery{
			AccountID:          intent.AccountID,
			TradingEnvironment: intent.TradingEnvironment,
			Market:             intent.Market,
		})
		if resolveErr != nil {
			return resolveErr
		}
		orderIDEx, placeErr := client.PlaceComboOrder(ctx, &trdplacecombopb.C2S{
			Header:    resolved.header(),
			ComboLegs: legs,
			Qty:       new(comboQuantity(intent)),
			Price:     intent.Price,
			OrderType: new(int32(trdcommonpb.OrderType_OrderType_Normal)),
			Remark:    optionalNonEmptyString(intent.ClientOrderID),
			QuoteID:   optionalNonEmptyString(intent.RFQID),
		})
		if placeErr != nil {
			return placeErr
		}
		result.BrokerOrderID = orderIDEx
		result.Status = "SUBMITTED"
		for _, leg := range intent.Legs {
			snapshot := broker.OrderLegSnapshot{
				InstrumentID: leg.InstrumentID, ProductClass: leg.ProductClass,
				Side: leg.Side, Ratio: leg.Ratio, PredictionSide: leg.PredictionSide,
				Status: "SUBMITTED",
			}
			if leg.Quantity != nil {
				snapshot.RequestedQuantity = *leg.Quantity
			}
			if leg.Amount != nil {
				snapshot.RequestedAmount = *leg.Amount
			}
			if leg.Price != nil {
				snapshot.RequestedPrice = *leg.Price
			}
			result.Legs = append(result.Legs, snapshot)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return &result, nil
}

func (a *futuAdapter) CancelComboOrder(ctx context.Context, query broker.ReadQuery, brokerOrderID string) error {
	brokerOrderID = strings.TrimSpace(brokerOrderID)
	if brokerOrderID == "" {
		return fmt.Errorf("futu: combo cancel requires broker order id")
	}
	return a.exchange.withClient(ctx, func(client *opend.Client) error {
		resolved, err := a.exchange.resolveTradeAccountWithClient(ctx, client, BrokerReadQuery{
			AccountID:          query.AccountID,
			TradingEnvironment: query.TradingEnvironment,
			Market:             query.Market,
		})
		if err != nil {
			return err
		}
		zero := uint64(0)
		_, err = client.ModifyOrder(ctx, &trdmodifyorderpb.C2S{
			Header:        resolved.header(),
			OrderID:       &zero,
			OrderIDEx:     &brokerOrderID,
			ModifyOrderOp: new(int32(trdcommonpb.ModifyOrderOp_ModifyOrderOp_Cancel)),
		})
		return err
	})
}

func (a *futuAdapter) PreviewEventOrder(ctx context.Context, intent broker.ComboOrderIntent) (*broker.ProductRuleResult, error) {
	return a.PreviewComboOrder(ctx, intent)
}

func (a *futuAdapter) PlaceEventOrder(ctx context.Context, intent broker.ComboOrderIntent) (*broker.ComboOrderResult, error) {
	return a.PlaceComboOrder(ctx, intent)
}

func (a *futuAdapter) CancelEventOrder(ctx context.Context, query broker.ReadQuery, brokerOrderID string) error {
	return a.CancelComboOrder(ctx, query, brokerOrderID)
}

func validateComboIntent(intent broker.ComboOrderIntent) (broker.OrderKind, error) {
	if len(intent.Legs) < 2 {
		return "", fmt.Errorf("futu: combo order requires at least two legs")
	}
	kind := intent.OrderKind
	if kind != broker.OrderKindOptionCombo && kind != broker.OrderKindEventParlay {
		return "", fmt.Errorf("futu: unsupported combo order kind %q", kind)
	}
	expectedProduct := broker.ProductClassOption
	if kind == broker.OrderKindEventParlay {
		expectedProduct = broker.ProductClassEventContract
		if strings.TrimSpace(intent.RFQID) == "" {
			return "", fmt.Errorf("futu: event parlay requires an RFQ quote id")
		}
		if intent.QuoteExpiresAt == nil || !time.Now().Before(*intent.QuoteExpiresAt) {
			return "", fmt.Errorf("futu: event parlay RFQ quote has expired; request a new quote")
		}
		if intent.Amount == nil || *intent.Amount <= 0 {
			return "", fmt.Errorf("futu: event parlay amount must be positive")
		}
	} else {
		if strings.TrimSpace(intent.UnderlyingID) == "" {
			return "", fmt.Errorf("futu: option combo requires underlyingInstrumentId")
		}
		if strings.TrimSpace(intent.NearExpiry) == "" {
			return "", fmt.Errorf("futu: option combo requires nearExpiry")
		}
		strategy, strategyErr := futuOptionStrategyValue(intent.OptionStrategy)
		if strategyErr != nil {
			return "", strategyErr
		}
		if strategy == int32(qotcommonpb.OptionStrategyType_OptionStrategyType_CalendarSpread) &&
			strings.TrimSpace(intent.FarExpiry) == "" {
			return "", fmt.Errorf("futu: calendar option combo requires farExpiry")
		}
	}
	for _, leg := range intent.Legs {
		if leg.ProductClass != expectedProduct {
			return "", fmt.Errorf("futu: combo cannot mix %s with %s legs", expectedProduct, leg.ProductClass)
		}
		if leg.Ratio <= 0 {
			return "", fmt.Errorf("futu: combo leg ratio must be positive")
		}
		if kind == broker.OrderKindEventParlay {
			side := strings.ToUpper(strings.TrimSpace(leg.PredictionSide))
			if side != "YES" && side != "NO" {
				return "", fmt.Errorf("futu: event parlay legs require predictionSide YES or NO")
			}
		}
	}
	return kind, nil
}

func comboInstrumentIDs(legs []broker.OrderLegIntent) []string {
	ids := make([]string, 0, len(legs))
	for _, leg := range legs {
		ids = append(ids, leg.InstrumentID)
	}
	return ids
}

func (a *futuAdapter) validateActiveEventContracts(ctx context.Context, instrumentIDs []string) error {
	securityList := make([]any, 0, len(instrumentIDs))
	expected := make(map[string]struct{}, len(instrumentIDs))
	for _, instrumentID := range instrumentIDs {
		code := strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(instrumentID)), "US.")
		if code == "" {
			return fmt.Errorf("futu: event contract code is required")
		}
		expected[code] = struct{}{}
		securityList = append(securityList, map[string]any{"market": 101, "code": code})
	}
	var payload map[string]any
	if err := a.exchange.withClient(ctx, func(client *opend.Client) error {
		var callErr error
		payload, callErr = client.CallAdvanced(ctx, "Qot_GetEventContractSnapshot", map[string]any{
			"securityList": securityList,
		})
		return callErr
	}); err != nil {
		return err
	}
	items, _ := payload["snapshotList"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		security, _ := item["code"].(map[string]any)
		code := strings.ToUpper(strings.TrimSpace(fmt.Sprint(security["code"])))
		if _, ok := expected[code]; !ok {
			continue
		}
		status := strings.ToUpper(strings.TrimSpace(fmt.Sprint(item["status"])))
		if status != "EC_STATUS_ACTIVE" && status != "2" {
			return fmt.Errorf("futu: event contract %s is not active (%s)", code, status)
		}
		delete(expected, code)
	}
	if len(expected) != 0 {
		missing := make([]string, 0, len(expected))
		for code := range expected {
			missing = append(missing, code)
		}
		return fmt.Errorf("futu: event contract snapshot missing %s", strings.Join(missing, ","))
	}
	return nil
}

func futuComboLegs(legs []broker.OrderLegIntent, eventContract bool) ([]*qotcommonpb.ComboLeg, error) {
	result := make([]*qotcommonpb.ComboLeg, 0, len(legs))
	for _, leg := range legs {
		var security *qotcommonpb.Security
		if eventContract {
			code := strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(leg.InstrumentID)), "US.")
			security = &qotcommonpb.Security{Market: new(int32(101)), Code: new(code)}
		} else {
			value, _, err := futuSecurityFromSymbol(leg.InstrumentID)
			if err != nil {
				return nil, err
			}
			security = value
		}
		item := &qotcommonpb.ComboLeg{
			Security: security,
			QtyRatio: new(float64(leg.Ratio)),
		}
		switch strings.ToUpper(strings.TrimSpace(leg.Side)) {
		case "BUY":
			item.Side = new(int32(trdcommonpb.TrdSide_TrdSide_Buy))
		case "SELL":
			item.Side = new(int32(trdcommonpb.TrdSide_TrdSide_Sell))
		default:
			return nil, fmt.Errorf("futu: combo leg side must be BUY or SELL")
		}
		if eventContract {
			value, err := predictionSideValue(leg.PredictionSide)
			if err != nil {
				return nil, err
			}
			item.PredSide = &value
		}
		result = append(result, item)
	}
	return result, nil
}

func comboQuantity(intent broker.ComboOrderIntent) float64 {
	if intent.Amount != nil {
		return *intent.Amount
	}
	for _, leg := range intent.Legs {
		if leg.Quantity != nil && *leg.Quantity > 0 {
			return *leg.Quantity / float64(leg.Ratio)
		}
	}
	return 1
}

var (
	_ broker.ProductRuleProvider         = (*futuAdapter)(nil)
	_ broker.ComboTradingService         = (*futuAdapter)(nil)
	_ broker.EventContractTradingService = (*futuAdapter)(nil)
)
