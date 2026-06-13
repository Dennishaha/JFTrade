package pineruntime

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	"github.com/jftrade/jftrade-main/pkg/strategy/indicatorbinding"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func (r *strategyRuntime) executeOrderStatement(statement *strategyir.OrderStmt, scope *evaluationScope) error {
	position := r.getPosition(r.symbol, scope.currentKlineTime)
	availablePositionQty := 0.0
	if position != nil {
		availablePositionQty = math.Floor(absFloat(firstPositiveFloat(absFloat(position.AvailableQuantity), absFloat(position.Quantity))))
	}

	intent := normalizeOrderIntent(statement.Intent)
	action := statement.Action
	if intent == strategyir.OrderIntentFlatten {
		if position == nil || availablePositionQty <= 0 {
			r.internalLog("无持仓可平，跳过全部平仓")
			return nil
		}
		switch position.Direction {
		case "LONG":
			action = strategyir.OrderActionSell
		case "SHORT":
			action = strategyir.OrderActionCover
		default:
			r.internalLog("无持仓可平，跳过全部平仓")
			return nil
		}
	}

	entryPolicy := normalizeEntryPolicy(statement.EntryPolicy)
	sameDirectionEntryCount := 0
	switch action {
	case strategyir.OrderActionBuy:
		if intent == strategyir.OrderIntentEntry {
			sameDirectionEntryCount = r.sameDirectionEntryCount("LONG", position, availablePositionQty)
		}
		if intent == strategyir.OrderIntentEntry && shouldSkipLongEntry(position, availablePositionQty, entryPolicy, r.maxPyramiding, sameDirectionEntryCount) {
			r.internalLog("已有多头持仓，按策略跳过开多")
			return nil
		}
	case strategyir.OrderActionSell:
		if intent != strategyir.OrderIntentNet && (position == nil || position.Direction != "LONG" || availablePositionQty <= 0) {
			r.internalLog("无多头持仓可平，跳过卖出")
			return nil
		}
	case strategyir.OrderActionShort:
		if intent == strategyir.OrderIntentEntry {
			sameDirectionEntryCount = r.sameDirectionEntryCount("SHORT", position, availablePositionQty)
		}
		if intent == strategyir.OrderIntentEntry && shouldSkipShortEntry(position, availablePositionQty, entryPolicy, r.maxPyramiding, sameDirectionEntryCount) {
			r.internalLog("已有空头持仓，按策略跳过开空")
			return nil
		}
	case strategyir.OrderActionCover:
		if intent != strategyir.OrderIntentNet && (position == nil || position.Direction != "SHORT" || availablePositionQty <= 0) {
			r.internalLog("无空头持仓可平，跳过买入平空")
			return nil
		}
	default:
		return fmt.Errorf("pine line %d: unsupported order action %q", statement.Range.StartLine, action)
	}

	orderPrice, limitPrice, err := r.resolveOrderPrice(statement, scope)
	if err != nil {
		return fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
	}
	if orderPrice <= 0 || math.IsNaN(orderPrice) || math.IsInf(orderPrice, 0) {
		return fmt.Errorf("pine line %d: order price must be positive", statement.Range.StartLine)
	}

	quantityMode, ok := indicatorbinding.ParseQuantityMode(statement.QuantityMode)
	if !ok {
		return fmt.Errorf("pine line %d: unsupported order quantity mode %q", statement.Range.StartLine, statement.QuantityMode)
	}
	quantity, err := r.resolveOrderQuantity(statement, scope, position, availablePositionQty, orderPrice, quantityMode)
	if err != nil {
		return fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
	}
	if quantity <= 0 {
		return nil
	}
	if r.isPlaceBlockedDuringWarmup(scope.currentKlineTime) {
		r.internalLog("place order suppressed during warmup")
		return nil
	}

	if r.shouldStorePendingOrder(statement, intent) {
		r.storePendingOrder(statement, action, intent, quantity, limitPrice, orderPrice)
		return nil
	}

	orderSide, err := exchangeSideForAction(action)
	if err != nil {
		return fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
	}
	if err := r.submitOrder(orderSide, normalizeOrderType(statement.OrderType), quantity, limitPrice); err != nil {
		return fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
	}
	r.emitOrderMetadata(statement.Comment, statement.AlertMessage, statement.DisableAlert)
	switch intent {
	case strategyir.OrderIntentNet:
		r.resetEntrySubmitCount("LONG")
		r.resetEntrySubmitCount("SHORT")
	default:
		r.recordSubmittedOrderAction(action, quantity, availablePositionQty, sameDirectionEntryCount)
	}
	return nil
}

func (r *strategyRuntime) resolveOrderPrice(statement *strategyir.OrderStmt, scope *evaluationScope) (float64, float64, error) {
	closePrice := 0.0
	if scope.currentKline != nil {
		closePrice = scope.currentKline.Close.Float64()
	}
	if strings.TrimSpace(statement.StopExpression) != "" {
		value, err := evaluateFloatExpression(statement.StopExpression, scope)
		if err != nil {
			return 0, 0, err
		}
		return value, 0, nil
	}
	orderType := normalizeOrderType(statement.OrderType)
	if orderType == types.OrderTypeMarket {
		return closePrice, 0, nil
	}
	limitPrice := closePrice
	if strings.TrimSpace(statement.LimitExpression) != "" {
		value, err := evaluateFloatExpression(statement.LimitExpression, scope)
		if err != nil {
			return 0, 0, err
		}
		limitPrice = value
	}
	return limitPrice, limitPrice, nil
}

func (r *strategyRuntime) resolveOrderQuantity(
	statement *strategyir.OrderStmt,
	scope *evaluationScope,
	position *positionSnapshot,
	availablePositionQty float64,
	orderPrice float64,
	quantityMode string,
) (float64, error) {
	value, err := evaluateFloatExpression(statement.QuantityExpression, scope)
	if err != nil {
		return 0, err
	}
	closingAction := isClosingOrderQuantity(statement)
	switch quantityMode {
	case "shares":
		quantity := math.Floor(value)
		if quantity <= 0 {
			r.internalLog("shares quantity computed as 0, skipping order")
			return 0, nil
		}
		return quantity, nil
	case "amount":
		quantity := math.Floor(value / orderPrice)
		if quantity <= 0 {
			r.internalLog("amount quantity computed as 0, skipping order")
			return 0, nil
		}
		return quantity, nil
	case "account_position_percent":
		accountTotalValue := r.getTotalAccountValue()
		targetAmount := accountTotalValue * value / 100
		rawQuantity := 0.0
		if targetAmount > 0 {
			rawQuantity = math.Floor(targetAmount / orderPrice)
		}
		return clampPercentBasedQuantity(rawQuantity, availablePositionQty, closingAction), nil
	case "symbol_position_percent":
		if closingAction {
			rawQuantity := 0.0
			if availablePositionQty > 0 {
				rawQuantity = math.Floor(availablePositionQty * value / 100)
			}
			return clampPercentBasedQuantity(rawQuantity, availablePositionQty, true), nil
		}
		currentPositionValue := 0.0
		if position != nil {
			currentPositionValue = absFloat(position.MarketValue)
		}
		targetValue := currentPositionValue * value / 100
		rawQuantity := 0.0
		if targetValue > 0 {
			rawQuantity = math.Floor(targetValue / orderPrice)
		}
		return clampPercentBasedQuantity(rawQuantity, availablePositionQty, closingAction), nil
	default:
		quantity := math.Floor(value)
		if quantity <= 0 {
			return 0, nil
		}
		return quantity, nil
	}
}

func isClosingOrderQuantity(statement *strategyir.OrderStmt) bool {
	if statement == nil {
		return false
	}
	switch statement.Intent {
	case strategyir.OrderIntentClose, strategyir.OrderIntentFlatten:
		return true
	case strategyir.OrderIntentNet:
		return false
	case "":
		return statement.Action == strategyir.OrderActionSell || statement.Action == strategyir.OrderActionCover
	default:
		return false
	}
}

func clampPercentBasedQuantity(rawQuantity float64, availablePositionQty float64, closingAction bool) float64 {
	if rawQuantity > 0 {
		if availablePositionQty > 0 {
			return math.Min(rawQuantity, availablePositionQty)
		}
		return rawQuantity
	}
	if closingAction && availablePositionQty > 0 {
		return 1
	}
	return 0
}

func (r *strategyRuntime) submitOrder(side types.SideType, orderType types.OrderType, quantity float64, limitPrice float64) error {
	if r.executor == nil {
		return fmt.Errorf("order executor is not available")
	}
	if r.session == nil {
		return fmt.Errorf("exchange session is not available")
	}
	symbol := r.symbol
	market, ok := r.session.Market(symbol)
	if !ok {
		return fmt.Errorf("market %s is not loaded in this session", symbol)
	}
	order := types.SubmitOrder{
		ClientOrderID: fmt.Sprintf("pine-go-%d", time.Now().UnixNano()),
		Symbol:        symbol,
		Side:          side,
		Type:          orderType,
		Quantity:      fixedpoint.NewFromFloat(quantity),
		Market:        market,
	}
	if orderType == types.OrderTypeLimit {
		if limitPrice <= 0 {
			return fmt.Errorf("limit price must be positive")
		}
		order.Price = fixedpoint.NewFromFloat(limitPrice)
		order.TimeInForce = types.TimeInForceGTC
	}
	if _, err := r.executor.SubmitOrders(r.ctx, order); err != nil {
		return fmt.Errorf("submit order: %w", err)
	}
	r.clearPositionCache()
	return nil
}

func (r *strategyRuntime) emitOrderMetadata(comment string, alertMessage string, disableAlert bool) {
	if trimmed := strings.TrimSpace(comment); trimmed != "" {
		r.internalLog("order comment: " + trimmed)
	}
	if disableAlert {
		return
	}
	if trimmed := strings.TrimSpace(alertMessage); trimmed != "" {
		r.notify(trimmed)
	}
}

func evaluateOptionalFloatExpression(expression string, scope *evaluationScope) (float64, bool, error) {
	trimmed := strings.TrimSpace(expression)
	if trimmed == "" {
		return 0, false, nil
	}
	value, err := evaluateFloatExpression(trimmed, scope)
	if err != nil {
		return 0, false, err
	}
	return value, true, nil
}

func currentBarPrices(scope *evaluationScope) (float64, float64, float64) {
	if scope == nil || scope.currentKline == nil {
		return 0, 0, 0
	}
	return scope.currentKline.High.Float64(), scope.currentKline.Low.Float64(), scope.currentKline.Close.Float64()
}
