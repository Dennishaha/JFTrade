package trading

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

type ExecutionComboRequest struct {
	BrokerID           string                  `json:"brokerId"`
	TradingEnvironment string                  `json:"tradingEnvironment"`
	AccountID          string                  `json:"accountId"`
	Market             string                  `json:"market"`
	ClientOrderID      string                  `json:"clientOrderId"`
	OrderKind          broker.OrderKind        `json:"orderKind"`
	ProductClass       broker.ProductClass     `json:"productClass"`
	PreviewID          string                  `json:"previewId"`
	RFQID              string                  `json:"rfqId"`
	MVC                string                  `json:"mvc"`
	UnderlyingID       string                  `json:"underlyingInstrumentId"`
	OptionStrategy     string                  `json:"optionStrategy"`
	NearExpiry         string                  `json:"nearExpiry"`
	FarExpiry          string                  `json:"farExpiry"`
	Spread             *float64                `json:"spread"`
	QuoteExpiresAt     *time.Time              `json:"quoteExpiresAt"`
	Amount             *float64                `json:"amount"`
	Price              *float64                `json:"price"`
	Legs               []broker.OrderLegIntent `json:"legs"`
}

type ExecutionComboPreview struct {
	PreviewID         string                           `json:"previewId"`
	RequestHash       string                           `json:"requestHash"`
	PreviewAt         string                           `json:"previewAt"`
	ExpiresAt         string                           `json:"expiresAt"`
	CapabilityVersion string                           `json:"capabilityVersion"`
	BrokerID          string                           `json:"brokerId"`
	AccountID         string                           `json:"accountId"`
	Market            string                           `json:"market"`
	OrderKind         broker.OrderKind                 `json:"orderKind"`
	ProductClass      broker.ProductClass              `json:"productClass"`
	Legs              []broker.OrderLegIntent          `json:"legs"`
	Allowed           bool                             `json:"allowed"`
	BuyingPowerImpact *float64                         `json:"buyingPowerImpact,omitempty"`
	AccountImpact     *broker.OptionComboAccountImpact `json:"accountImpact,omitempty"`
	Warnings          []string                         `json:"warnings,omitempty"`
	OptionAnalysis    *broker.OptionComboAnalysis      `json:"optionAnalysis,omitempty"`
}

func (s *Service) PreviewExecutionBuyingPower(ctx context.Context, query broker.ProductRuleQuery) (*broker.ProductRuleResult, error) {
	brokerID := strings.ToLower(strings.TrimSpace(query.BrokerID))
	selected := s.brokerRuntime.ActiveBroker()
	if resolver, ok := s.brokerRuntime.(interface{ ResolveBroker(string) broker.Broker }); ok {
		selected = resolver.ResolveBroker(brokerID)
	} else if brokerID != "" && selected != nil && !strings.EqualFold(selected.ID(), brokerID) {
		selected = nil
	}
	if selected == nil {
		return nil, requestErrorf("brokerId %q is not available", brokerID)
	}
	provider, ok := selected.(broker.ProductRuleProvider)
	if !ok {
		return nil, requestErrorf("broker %q does not support product rule previews", selected.ID())
	}
	query.BrokerID = selected.ID()
	query.FeatureID = broker.FeatureExecutionBuyingPower
	return provider.ValidateProductOrder(ctx, query)
}

func (s *Service) PreviewExecutionCombo(ctx context.Context, request ExecutionComboRequest) (ExecutionComboPreview, error) {
	intent, selected, err := s.normalizeComboRequest(request, false)
	if err != nil {
		return ExecutionComboPreview{}, err
	}
	if intent.OrderKind == broker.OrderKindEventParlay {
		if s.predictionQuotes != nil {
			intent, err = s.bindPredictionQuote(ctx, intent)
			if err != nil {
				return ExecutionComboPreview{}, err
			}
		} else if strings.EqualFold(intent.TradingEnvironment, "REAL") {
			return ExecutionComboPreview{}, requestErrorf("prediction quote persistence is unavailable for REAL orders")
		} else if intent.QuoteExpiresAt == nil || !time.Now().Before(*intent.QuoteExpiresAt) {
			return ExecutionComboPreview{}, requestErrorf("Parlay quote expired; request a new RFQ")
		}
	}
	service, ok := selected.(broker.ComboTradingService)
	if !ok {
		return ExecutionComboPreview{}, requestErrorf("broker %q does not support combo trading", intent.BrokerID)
	}
	result, err := service.PreviewComboOrder(ctx, intent)
	if err != nil {
		return ExecutionComboPreview{}, err
	}
	if result == nil || !result.Allowed {
		reason := "combo preview rejected"
		if result != nil && strings.TrimSpace(result.Reason) != "" {
			reason = result.Reason
		}
		return ExecutionComboPreview{}, requestErrorf("%s", reason)
	}
	now := time.Now().UTC()
	expiresAt := now.Add(5 * time.Minute)
	if intent.QuoteExpiresAt != nil && intent.QuoteExpiresAt.Before(expiresAt) {
		expiresAt = intent.QuoteExpiresAt.UTC()
	}
	requestHash := comboIntentHash(intent)
	previewID := "preview-" + requestHash[:20]
	preview := ExecutionComboPreview{
		PreviewID:         previewID,
		RequestHash:       requestHash,
		PreviewAt:         now.Format(time.RFC3339Nano),
		ExpiresAt:         expiresAt.Format(time.RFC3339Nano),
		CapabilityVersion: broker.BuiltinCapabilityCatalog.Version,
		BrokerID:          intent.BrokerID,
		AccountID:         intent.AccountID,
		Market:            intent.Market,
		OrderKind:         intent.OrderKind,
		ProductClass:      intent.ProductClass,
		Legs:              append([]broker.OrderLegIntent(nil), intent.Legs...),
		Allowed:           true,
		BuyingPowerImpact: result.BuyingPowerImpact,
		AccountImpact:     result.AccountImpact,
		Warnings:          append([]string(nil), result.Warnings...),
		OptionAnalysis:    result.OptionAnalysis,
	}
	if preview.BuyingPowerImpact == nil && preview.AccountImpact != nil {
		preview.BuyingPowerImpact = preview.AccountImpact.BuyingPowerDecrease
	}
	if s.previewStore != nil {
		quoteExpiresAt := ""
		if intent.QuoteExpiresAt != nil {
			quoteExpiresAt = intent.QuoteExpiresAt.UTC().Format(time.RFC3339Nano)
		}
		if err := s.previewStore.SavePreview(ExecutionPreviewRecord{
			PreviewID: previewID, RequestHash: requestHash, BrokerID: intent.BrokerID,
			CapabilityVersion: broker.BuiltinCapabilityCatalog.Version, AccountID: intent.AccountID,
			ExpiresAt: expiresAt.Format(time.RFC3339Nano), QuoteExpiresAt: quoteExpiresAt,
			RFQID: intent.RFQID, NormalizedRequest: normalizedComboIntent(intent),
			CreatedAt: now.Format(time.RFC3339Nano),
		}); err != nil {
			return ExecutionComboPreview{}, err
		}
	}
	return preview, nil
}

func (s *Service) CreateExecutionCombo(ctx context.Context, request ExecutionComboRequest) (ExecutionCommandResponse, error) {
	intent, _, err := s.normalizeComboRequest(request, true)
	if err != nil {
		return ExecutionCommandResponse{}, err
	}
	if intent.OrderKind == broker.OrderKindEventParlay {
		if s.predictionQuotes != nil {
			intent, err = s.bindPredictionQuote(ctx, intent)
			if err != nil {
				return ExecutionCommandResponse{}, err
			}
		} else if strings.EqualFold(intent.TradingEnvironment, "REAL") {
			return ExecutionCommandResponse{}, requestErrorf("prediction quote persistence is unavailable for REAL orders")
		} else if intent.QuoteExpiresAt == nil || !time.Now().Before(*intent.QuoteExpiresAt) {
			return ExecutionCommandResponse{}, requestErrorf("Parlay quote expired; request a new RFQ")
		}
	}
	if err := s.evaluatePlaceExecutionOrderRisk(ctx, comboRiskCommand(intent)); err != nil {
		return ExecutionCommandResponse{}, err
	}
	if s.previewStore != nil {
		if err := s.previewStore.ConsumePreview(
			intent.PreviewID, intent.BrokerID, intent.AccountID,
			comboIntentHash(intent), intent.ClientOrderID,
		); err != nil {
			return ExecutionCommandResponse{}, requestErrorf("execution preview is invalid: %v", err)
		}
	}
	if intent.OrderKind == broker.OrderKindEventParlay && s.predictionQuotes != nil {
		if err := s.predictionQuotes.ConsumePredictionQuote(
			ctx, intent.RFQID, intent.BrokerID, intent.AccountID,
			intent.TradingEnvironment, intent.MVC,
			broker.PredictionQuoteLegsHash(intent.MVC, intent.Legs),
			intent.PreviewID, intent.ClientOrderID,
		); err != nil {
			return ExecutionCommandResponse{}, requestErrorf("prediction RFQ is invalid: %v", err)
		}
	}
	// Keep the final decision and broker submission in the same control-plane
	// placement window. This also covers combo and event-parlay orders when a
	// kill switch or hard stop is activated concurrently.
	var order ExecutionOrder
	err = s.executePlaceOrderWithRisk(ctx, comboRiskCommand(intent), func() error {
		var submitErr error
		order, submitErr = s.comboGateway.PlaceCombo(ctx, intent)
		return submitErr
	})
	if err != nil {
		return ExecutionCommandResponse{}, err
	}
	return executionCommandResponse("PLACE_COMBO", "combo order submitted to broker", order), nil
}

func comboRiskCommand(intent broker.ComboOrderIntent) ExecutionOrderCommand {
	return ExecutionOrderCommand{
		BrokerID:     intent.BrokerID,
		OrderKind:    intent.OrderKind,
		ProductClass: intent.ProductClass,
		PreviewID:    intent.PreviewID,
		Legs:         intent.Legs,
		Query: broker.PlaceOrderQuery{
			ReadQuery:     intent.ReadQuery,
			ProductClass:  intent.ProductClass,
			QuantityMode:  comboQuantityMode(intent.OrderKind),
			Quantity:      comboRiskQuantity(intent),
			Amount:        intent.Amount,
			Price:         intent.Price,
			ClientOrderID: intent.ClientOrderID,
		},
	}
}

func (s *Service) CancelExecutionCombo(ctx context.Context, internalOrderID string) (ExecutionCommandResponse, error) {
	order, err := s.comboGateway.CancelCombo(ctx, strings.TrimSpace(internalOrderID))
	if err != nil {
		return ExecutionCommandResponse{}, err
	}
	return executionCommandResponse("CANCEL_COMBO", "combo cancel submitted to broker", order), nil
}

func (s *Service) normalizeComboRequest(
	request ExecutionComboRequest,
	requirePreview bool,
) (broker.ComboOrderIntent, broker.Broker, error) {
	brokerID, selected, err := s.resolveExecutionBroker(request.BrokerID)
	if err != nil {
		return broker.ComboOrderIntent{}, nil, err
	}

	productClass, err := normalizeComboProductClass(request.OrderKind, request.ProductClass)
	if err != nil {
		return broker.ComboOrderIntent{}, nil, err
	}
	if err := normalizeComboLegs(&request, productClass); err != nil {
		return broker.ComboOrderIntent{}, nil, err
	}
	if requirePreview && strings.TrimSpace(request.PreviewID) == "" {
		return broker.ComboOrderIntent{}, nil, requestErrorf("previewId is required")
	}
	if err := validateEventParlayRequest(request); err != nil {
		return broker.ComboOrderIntent{}, nil, err
	}
	if err := validateOptionComboRequest(request); err != nil {
		return broker.ComboOrderIntent{}, nil, err
	}
	environment := s.executionEnvironment(request.TradingEnvironment)
	intent := broker.ComboOrderIntent{
		ReadQuery: broker.ReadQuery{
			BrokerID: brokerID, TradingEnvironment: environment,
			AccountID: strings.TrimSpace(request.AccountID), Market: strings.ToUpper(strings.TrimSpace(request.Market)),
		},
		ClientOrderID: strings.TrimSpace(request.ClientOrderID),
		OrderKind:     request.OrderKind, ProductClass: productClass, PreviewID: strings.TrimSpace(request.PreviewID),
		RFQID: strings.TrimSpace(request.RFQID), MVC: strings.TrimSpace(request.MVC),
		UnderlyingID:   strings.ToUpper(strings.TrimSpace(request.UnderlyingID)),
		OptionStrategy: strings.ToLower(strings.TrimSpace(request.OptionStrategy)),
		NearExpiry:     strings.TrimSpace(request.NearExpiry), FarExpiry: strings.TrimSpace(request.FarExpiry),
		Spread:         request.Spread,
		QuoteExpiresAt: request.QuoteExpiresAt,
		Amount:         request.Amount, Price: request.Price, Legs: request.Legs,
	}
	return intent, selected, nil
}

func normalizeComboProductClass(
	kind broker.OrderKind,
	productClass broker.ProductClass,
) (broker.ProductClass, error) {
	switch kind {
	case broker.OrderKindOptionCombo:
		if productClass == "" {
			productClass = broker.ProductClassOption
		}
	case broker.OrderKindEventParlay:
		if productClass == "" {
			productClass = broker.ProductClassEventContract
		}
	default:
		return "", requestErrorf("orderKind must be option_combo or event_parlay")
	}
	return productClass, nil
}

func normalizeComboLegs(request *ExecutionComboRequest, productClass broker.ProductClass) error {
	if len(request.Legs) < 2 {
		return requestErrorf("combo requires at least two legs")
	}
	if strings.TrimSpace(request.ClientOrderID) == "" {
		return requestErrorf("clientOrderId is required for idempotent combo preview and submission")
	}
	for index := range request.Legs {
		leg := &request.Legs[index]
		leg.InstrumentID = strings.ToUpper(strings.TrimSpace(leg.InstrumentID))
		leg.Side = strings.ToUpper(strings.TrimSpace(leg.Side))
		leg.PredictionSide = strings.ToUpper(strings.TrimSpace(leg.PredictionSide))
		if leg.ProductClass == "" {
			leg.ProductClass = productClass
		}
		if leg.ProductClass != productClass {
			return requestErrorf("combo cannot mix product classes")
		}
		if leg.InstrumentID == "" || (leg.Side != "BUY" && leg.Side != "SELL") || leg.Ratio <= 0 {
			return requestErrorf("each combo leg requires instrumentId, BUY/SELL side, and positive ratio")
		}
	}
	return nil
}

func validateOptionComboRequest(request ExecutionComboRequest) error {
	if request.OrderKind != broker.OrderKindOptionCombo {
		return nil
	}
	if request.Amount != nil {
		return requestErrorf("amount is supported for event parlay orders only")
	}
	if strings.TrimSpace(request.UnderlyingID) == "" {
		return requestErrorf("option combo requires underlyingInstrumentId")
	}
	if strings.TrimSpace(request.NearExpiry) == "" {
		return requestErrorf("option combo requires nearExpiry")
	}
	strategy := strings.ToLower(strings.TrimSpace(request.OptionStrategy))
	switch strategy {
	case "vertical", "strangle", "butterfly":
		if request.Spread == nil || *request.Spread <= 0 {
			return requestErrorf("%s option combo requires a positive spread", strategy)
		}
	case "straddle":
	case "calendar":
		if strings.TrimSpace(request.FarExpiry) == "" {
			return requestErrorf("calendar option combo requires farExpiry")
		}
	default:
		return requestErrorf("unsupported optionStrategy %q", request.OptionStrategy)
	}
	return nil
}

func validateEventParlayRequest(request ExecutionComboRequest) error {
	if request.OrderKind != broker.OrderKindEventParlay {
		return nil
	}
	if strings.ToUpper(strings.TrimSpace(request.Market)) != "US" {
		return requestErrorf("event parlay must use market US")
	}
	if strings.TrimSpace(request.RFQID) == "" ||
		request.Amount == nil || !finitePositive(*request.Amount) {
		return requestErrorf("event parlay requires rfqId and positive amount")
	}
	if request.Price != nil {
		return requestErrorf("event parlay price is bound to the server-side RFQ and must not be provided")
	}
	for _, leg := range request.Legs {
		if leg.PredictionSide != "YES" && leg.PredictionSide != "NO" {
			return requestErrorf("event parlay legs require predictionSide YES or NO")
		}
	}
	return nil
}

func (s *Service) bindPredictionQuote(
	ctx context.Context,
	intent broker.ComboOrderIntent,
) (broker.ComboOrderIntent, error) {
	if s.predictionQuotes == nil {
		return broker.ComboOrderIntent{}, requestErrorf("prediction quote persistence is unavailable")
	}
	if strings.TrimSpace(intent.MVC) == "" {
		return broker.ComboOrderIntent{}, requestErrorf("prediction RFQ requires mvc")
	}
	record, err := s.predictionQuotes.ValidatePredictionQuote(
		ctx, intent.RFQID, intent.BrokerID, intent.AccountID,
		intent.TradingEnvironment, intent.MVC,
		broker.PredictionQuoteLegsHash(intent.MVC, intent.Legs),
	)
	if err != nil {
		return broker.ComboOrderIntent{}, requestErrorf("prediction RFQ is invalid: %v", err)
	}
	expiresAt := record.ExpiresAt.UTC()
	intent.QuoteExpiresAt = &expiresAt
	return intent, nil
}

func (s *Service) executionEnvironment(raw string) string {
	environment := strings.ToUpper(strings.TrimSpace(raw))
	if environment == "" && s.defaultTradingEnvironment != nil {
		environment = strings.ToUpper(strings.TrimSpace(s.defaultTradingEnvironment()))
	}
	if environment == "" {
		return "SIMULATE"
	}
	return environment
}

func comboIntentHash(intent broker.ComboOrderIntent) string {
	intent.PreviewID = ""
	content, _ := json.Marshal(intent)
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func comboRiskQuantity(intent broker.ComboOrderIntent) float64 {
	if intent.Amount != nil {
		return *intent.Amount
	}
	for _, leg := range intent.Legs {
		if leg.Quantity != nil {
			return *leg.Quantity
		}
	}
	return 1
}

func comboQuantityMode(kind broker.OrderKind) broker.QuantityMode {
	if kind == broker.OrderKindEventParlay {
		return broker.QuantityModeAmount
	}
	return broker.QuantityModeContracts
}

func normalizedComboIntent(intent broker.ComboOrderIntent) string {
	content, err := json.Marshal(intent)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(content)
}
