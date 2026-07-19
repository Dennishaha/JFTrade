package trading

import (
	"context"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func (s *Service) PreviewExecutionOrder(req ExecutionPlaceRequest) (ExecutionPreview, error) {
	return s.PreviewExecutionOrderContext(context.Background(), req)
}

func (s *Service) PreviewExecutionOrderContext(
	ctx context.Context,
	req ExecutionPlaceRequest,
) (ExecutionPreview, error) {
	command, err := s.normalizeExecutionOrder(req)
	if err != nil {
		return ExecutionPreview{}, err
	}
	if requiresLockedExecutionPreview(command) && strings.TrimSpace(command.Query.ClientOrderID) == "" {
		return ExecutionPreview{}, requestErrorf("clientOrderId is required for derivative and event-contract previews")
	}
	if err := s.validateProductOrderPreview(ctx, command); err != nil {
		return ExecutionPreview{}, err
	}
	previewAt := time.Now().UTC()
	requestHash := executionCommandHash(command)
	previewID := "preview-" + requestHash[:20]
	expiresAt := previewAt.Add(5 * time.Minute)
	preview := ExecutionPreview{
		PreviewID: previewID, PreviewAt: previewAt.Format(time.RFC3339Nano),
		ExpiresAt:         expiresAt.Format(time.RFC3339Nano),
		CapabilityVersion: broker.BuiltinCapabilityCatalog.Version,
		BrokerID:          command.BrokerID, Symbol: command.Symbol, Side: command.Side,
		OrderType: command.OrderType, Quantity: command.Query.Quantity, Price: command.Query.Price,
		Amount: command.Query.Amount, PredictionSide: command.Query.PredictionSide,
		ProductClass: command.ProductClass, OrderKind: command.OrderKind, QuantityMode: command.QuantityMode,
		RequestHash:        requestHash,
		TradingEnvironment: command.Query.TradingEnvironment, AccountID: command.Query.AccountID,
		Market: command.Query.Market, PreviewValid: true,
	}
	if s.previewStore != nil {
		if err := s.previewStore.SavePreview(ExecutionPreviewRecord{
			PreviewID: previewID, RequestHash: requestHash, BrokerID: command.BrokerID,
			CapabilityVersion: broker.BuiltinCapabilityCatalog.Version, AccountID: command.Query.AccountID,
			ExpiresAt: expiresAt.Format(time.RFC3339Nano), NormalizedRequest: command.NormalizedRequest,
			CreatedAt: previewAt.Format(time.RFC3339Nano),
		}); err != nil {
			return ExecutionPreview{}, err
		}
	}
	return preview, nil
}

func (s *Service) CreateExecutionOrder(ctx context.Context, req ExecutionPlaceRequest) (ExecutionCommandResponse, error) {
	command, err := s.normalizeExecutionOrder(req)
	if err != nil {
		return ExecutionCommandResponse{}, err
	}
	if requiresLockedExecutionPreview(command) {
		if strings.TrimSpace(command.PreviewID) == "" {
			return ExecutionCommandResponse{}, requestErrorf("previewId is required for derivative and event-contract orders")
		}
		if strings.TrimSpace(command.Query.ClientOrderID) == "" {
			return ExecutionCommandResponse{}, requestErrorf("clientOrderId is required for idempotent derivative and event-contract submission")
		}
	}
	if strings.TrimSpace(command.PreviewID) != "" && strings.TrimSpace(command.Query.ClientOrderID) == "" {
		return ExecutionCommandResponse{}, requestErrorf("clientOrderId is required when previewId is supplied")
	}
	if s.previewStore != nil && strings.TrimSpace(command.PreviewID) != "" {
		if err := s.previewStore.ConsumePreview(
			command.PreviewID, command.BrokerID, command.Query.AccountID,
			executionCommandHash(command), command.Query.ClientOrderID,
		); err != nil {
			return ExecutionCommandResponse{}, requestErrorf("execution preview is invalid: %v", err)
		}
	}
	order, err := s.PlaceExecutionOrder(ctx, command)
	if err != nil {
		return ExecutionCommandResponse{}, err
	}
	return executionCommandResponse("PLACE", "order submitted to broker", order), nil
}

func requiresLockedExecutionPreview(command ExecutionOrderCommand) bool {
	return command.OrderKind == broker.OrderKindEventSingle ||
		command.ProductClass == broker.ProductClassOption ||
		command.ProductClass == broker.ProductClassFuture ||
		command.ProductClass == broker.ProductClassEventContract
}

func (s *Service) validateProductOrderPreview(ctx context.Context, command ExecutionOrderCommand) error {
	if !requiresLockedExecutionPreview(command) {
		return nil
	}
	selected := s.brokerRuntime.ActiveBroker()
	if resolver, ok := s.brokerRuntime.(interface{ ResolveBroker(string) broker.Broker }); ok {
		selected = resolver.ResolveBroker(command.BrokerID)
	} else if selected != nil && !strings.EqualFold(selected.ID(), command.BrokerID) {
		selected = nil
	}
	if selected == nil {
		return requestErrorf("brokerId %q is not available", command.BrokerID)
	}
	provider, ok := selected.(broker.ProductRuleProvider)
	if !ok {
		return requestErrorf("broker %q does not support product rule previews", command.BrokerID)
	}
	segment := broker.MarketSegmentDerivatives
	if command.ProductClass == broker.ProductClassEventContract {
		segment = broker.MarketSegmentPrediction
		if err := validatePredictionTradingAccount(ctx, selected, command.Query.AccountID); err != nil {
			return err
		}
	}
	if command.ProductClass == broker.ProductClassFuture &&
		strings.EqualFold(command.Query.TradingEnvironment, "REAL") {
		if err := validateFuturesTradingAuthority(ctx, selected, command.Query.AccountID); err != nil {
			return err
		}
	}
	quantity := command.Query.Quantity
	result, err := provider.ValidateProductOrder(ctx, broker.ProductRuleQuery{
		ReadQuery: command.Query.ReadQuery,
		FeatureID: broker.FeatureExecutionOrderPreview,
		Instrument: broker.Instrument{
			InstrumentID:  command.Symbol,
			Code:          strings.TrimPrefix(command.Symbol, command.Query.Market+"."),
			ProductClass:  command.ProductClass,
			MarketSegment: segment,
			QuoteMarket:   command.Query.Market,
			TradeMarket:   command.Query.Market,
			QuantityMode:  command.QuantityMode,
		},
		OrderKind: command.OrderKind,
		OrderType: command.OrderType,
		Session:   command.Session,
		Quantity:  &quantity,
		Amount:    command.Query.Amount,
		Price:     command.Query.Price,
		Legs:      command.Legs,
	})
	if err != nil {
		return err
	}
	if result == nil || !result.Allowed {
		reason := "product-rule preview rejected"
		if result != nil && strings.TrimSpace(result.Reason) != "" {
			reason = result.Reason
		}
		return requestErrorf("%s", reason)
	}
	return nil
}

func validateFuturesTradingAuthority(ctx context.Context, selected broker.Broker, accountID string) error {
	accounts, err := selected.DiscoverAccounts(ctx)
	if err != nil {
		return requestErrorf("futures authority could not be verified: %v", err)
	}
	for _, account := range accounts {
		if accountID != "" && account.ID != accountID {
			continue
		}
		if containsExecutionAuthority(account.MarketAuthorities, "FUTURES") {
			return nil
		}
	}
	return requestErrorf("REAL futures orders require FUTURES account authority")
}

func validatePredictionTradingAccount(ctx context.Context, selected broker.Broker, accountID string) error {
	accounts, err := selected.DiscoverAccounts(ctx)
	if err != nil {
		return requestErrorf("prediction account eligibility could not be verified: %v", err)
	}
	for _, account := range accounts {
		if accountID != "" && account.ID != accountID {
			continue
		}
		firm := ""
		if account.SecurityFirm != nil {
			firm = strings.ToUpper(strings.TrimSpace(*account.SecurityFirm))
		}
		if firm != "FUTUINC" {
			continue
		}
		if len(account.MarketAuthorities) == 0 || containsExecutionAuthority(account.MarketAuthorities, "US") {
			return nil
		}
	}
	return requestErrorf("prediction market requires an eligible Moomoo US account")
}

func containsExecutionAuthority(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
}

// PlaceExecutionOrder is the shared command boundary for manual and strategy
// orders. Every broker submission must pass through the pre-trade gateway.
func (s *Service) PlaceExecutionOrder(ctx context.Context, command ExecutionOrderCommand) (ExecutionOrder, error) {
	if s.preTradeRisk != nil {
		decision := s.preTradeRisk.EvaluatePlaceOrder(ctx, command)
		if !decision.Allows() {
			return ExecutionOrder{}, RiskRejectedError{Decision: decision}
		}
	}
	order, err := s.orderGateway.PlaceOrder(ctx, command)
	if err != nil {
		return ExecutionOrder{}, err
	}
	return order, nil
}

func (s *Service) CancelExecutionOrder(ctx context.Context, id string) (ExecutionCommandResponse, error) {
	order, err := s.orderGateway.CancelOrder(ctx, strings.TrimSpace(id))
	if err != nil {
		return ExecutionCommandResponse{}, err
	}
	return executionCommandResponse("CANCEL", "cancel request submitted to broker", order), nil
}
