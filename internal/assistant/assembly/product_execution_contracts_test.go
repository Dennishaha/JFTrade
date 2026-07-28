package assembly

import (
	"context"
	"testing"

	"github.com/jftrade/jftrade-main/internal/productfeatures"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

type contractProductFeatures struct {
	queries        []broker.FeatureQuery
	snapshots      []string
	customizations []broker.CustomizationAction
}

func (s *contractProductFeatures) CapabilitiesContext(context.Context, productfeatures.CapabilityQuery) map[string]any {
	return map[string]any{"available": true}
}

func (s *contractProductFeatures) Query(_ context.Context, query broker.FeatureQuery) (*broker.FeatureResult, error) {
	s.queries = append(s.queries, query)
	return &broker.FeatureResult{Entries: []map[string]any{{"feature": string(query.FeatureID), "instrument": query.InstrumentID}}}, nil
}

func (s *contractProductFeatures) BatchSnapshots(_ context.Context, _ broker.FeatureQuery, symbols []string) (*broker.FeatureResult, error) {
	s.snapshots = append([]string(nil), symbols...)
	return &broker.FeatureResult{Entries: []map[string]any{{"symbols": symbols}}}, nil
}

func (s *contractProductFeatures) ApplyCustomization(_ context.Context, action broker.CustomizationAction) (*broker.CustomizationResult, error) {
	s.customizations = append(s.customizations, action)
	return &broker.CustomizationResult{Entries: []map[string]any{{"action": action.Action}}}, nil
}

type contractExecutionService struct {
	orderPreview  trdsrv.ExecutionPlaceRequest
	comboPreview  trdsrv.ExecutionComboRequest
	orderCancels  []string
	comboCancels  []string
	buyingPowerID broker.FeatureID
}

func (s *contractExecutionService) PreviewExecutionOrderContext(_ context.Context, request trdsrv.ExecutionPlaceRequest) (trdsrv.ExecutionPreview, error) {
	s.orderPreview = request
	return trdsrv.ExecutionPreview{PreviewID: "preview-order", BrokerID: request.BrokerID, Symbol: request.Symbol}, nil
}

func (s *contractExecutionService) CreateExecutionOrder(_ context.Context, request trdsrv.ExecutionPlaceRequest) (trdsrv.ExecutionCommandResponse, error) {
	s.orderPreview = request
	id := "order-1"
	return trdsrv.ExecutionCommandResponse{Accepted: true, InternalOrderID: &id}, nil
}

func (s *contractExecutionService) CancelExecutionOrder(_ context.Context, id string) (trdsrv.ExecutionCommandResponse, error) {
	s.orderCancels = append(s.orderCancels, id)
	return trdsrv.ExecutionCommandResponse{Accepted: true}, nil
}

func (s *contractExecutionService) PreviewExecutionCombo(_ context.Context, request trdsrv.ExecutionComboRequest) (trdsrv.ExecutionComboPreview, error) {
	s.comboPreview = request
	return trdsrv.ExecutionComboPreview{PreviewID: "preview-combo", BrokerID: request.BrokerID, Allowed: true}, nil
}

func (s *contractExecutionService) CreateExecutionCombo(_ context.Context, request trdsrv.ExecutionComboRequest) (trdsrv.ExecutionCommandResponse, error) {
	s.comboPreview = request
	id := "combo-1"
	return trdsrv.ExecutionCommandResponse{Accepted: true, InternalOrderID: &id}, nil
}

func (s *contractExecutionService) CancelExecutionCombo(_ context.Context, id string) (trdsrv.ExecutionCommandResponse, error) {
	s.comboCancels = append(s.comboCancels, id)
	return trdsrv.ExecutionCommandResponse{Accepted: true}, nil
}

func (s *contractExecutionService) PreviewExecutionBuyingPower(_ context.Context, query broker.ProductRuleQuery) (*broker.ProductRuleResult, error) {
	s.buyingPowerID = query.FeatureID
	return &broker.ProductRuleResult{Allowed: true}, nil
}

func TestProductExecutionAdapterPreservesProductAndExecutionBoundaries(t *testing.T) {
	products := &contractProductFeatures{}
	execution := &contractExecutionService{}
	adapter := NewProductExecutionAdapter(products, execution)
	ctx := t.Context()

	capabilities, err := adapter.InvokeProductTool(ctx, "market.capabilities", map[string]any{"brokerId": "futu", "market": "us"})
	if err != nil || capabilities.(map[string]any)["available"] != true {
		t.Fatalf("market.capabilities = %#v, err=%v", capabilities, err)
	}
	if _, err := adapter.InvokeProductTool(ctx, "market.search", map[string]any{"brokerId": "futu", "market": "us", "query": "apple", "pageSize": 500}); err != nil {
		t.Fatalf("market.search: %v", err)
	}
	if len(products.queries) != 1 || products.queries[0].Market != "US" || products.queries[0].PageSize != 100 || products.queries[0].Params["keyword"] != "apple" {
		t.Fatalf("search query = %#v", products.queries)
	}
	if _, err := adapter.InvokeProductTool(ctx, "market.snapshots", map[string]any{"market": "us", "symbols": []any{"aapl", "MSFT"}}); err != nil {
		t.Fatalf("market.snapshots: %v", err)
	}
	if len(products.snapshots) != 2 || products.snapshots[0] != "aapl" || products.snapshots[1] != "MSFT" {
		t.Fatalf("snapshot symbols = %#v", products.snapshots)
	}
	if _, err := adapter.InvokeProductTool(ctx, "alerts.price.set", map[string]any{"brokerId": "futu", "payload": map[string]any{"symbol": "AAPL"}}); err != nil {
		t.Fatalf("alerts.price.set: %v", err)
	}
	if len(products.customizations) != 1 || products.customizations[0].Action != "set" || products.customizations[0].Payload["symbol"] != "AAPL" {
		t.Fatalf("customization = %#v", products.customizations)
	}

	orderInput := map[string]any{"brokerId": "futu", "market": "us", "symbol": "aapl", "side": "BUY", "orderType": "LIMIT", "quantity": 2, "price": 10.5}
	if _, err := adapter.InvokeExecutionTool(ctx, "execution.order_preview", orderInput); err != nil {
		t.Fatalf("execution.order_preview: %v", err)
	}
	if execution.orderPreview.BrokerID != "futu" || execution.orderPreview.Symbol != "aapl" {
		t.Fatalf("order request = %#v", execution.orderPreview)
	}
	placed, err := adapter.InvokeExecutionTool(ctx, "execution.order_place", orderInput)
	if err != nil || placed.(trdsrv.ExecutionCommandResponse).InternalOrderID == nil {
		t.Fatalf("execution.order_place = %#v, err=%v", placed, err)
	}
	if _, err := adapter.InvokeExecutionTool(ctx, "execution.order_cancel", map[string]any{"internalOrderId": "order-1"}); err != nil {
		t.Fatalf("execution.order_cancel: %v", err)
	}
	comboInput := map[string]any{"brokerId": "futu", "market": "us", "orderKind": "option_combo", "productClass": "option", "legs": []any{map[string]any{"instrumentId": "US.AAPL", "side": "BUY", "ratio": 1}}}
	if _, err := adapter.InvokeExecutionTool(ctx, "execution.combo_preview", comboInput); err != nil {
		t.Fatalf("execution.combo_preview: %v", err)
	}
	if _, err := adapter.InvokeExecutionTool(ctx, "execution.combo_place", comboInput); err != nil {
		t.Fatalf("execution.combo_place: %v", err)
	}
	if _, err := adapter.InvokeExecutionTool(ctx, "execution.combo_cancel", map[string]any{"internalOrderId": "combo-1"}); err != nil {
		t.Fatalf("execution.combo_cancel: %v", err)
	}
	if len(execution.orderCancels) != 1 || len(execution.comboCancels) != 1 || execution.comboPreview.BrokerID != "futu" {
		t.Fatalf("execution calls = order=%#v combo=%#v request=%#v", execution.orderCancels, execution.comboCancels, execution.comboPreview)
	}
	if _, err := adapter.InvokeProductTool(ctx, "execution.buying_power", map[string]any{"brokerId": "futu"}); err != nil {
		t.Fatalf("execution.buying_power: %v", err)
	}
	if execution.buyingPowerID != broker.FeatureExecutionBuyingPower {
		t.Fatalf("buying power feature = %q", execution.buyingPowerID)
	}
}
