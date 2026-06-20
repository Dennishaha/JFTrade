package pineruntime

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/strategy/indicatorbinding"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func (r *strategyRuntime) shouldStorePendingOrder(statement *strategyir.OrderStmt, intent strategyir.OrderIntent) bool {
	if statement == nil {
		return false
	}
	if strings.TrimSpace(statement.StopExpression) != "" {
		return true
	}
	if strings.TrimSpace(statement.LimitExpression) == "" {
		return false
	}
	switch intent {
	case strategyir.OrderIntentEntry, strategyir.OrderIntentNet:
		return true
	default:
		return false
	}
}

func (r *strategyRuntime) storePendingOrder(statement *strategyir.OrderStmt, action strategyir.OrderAction, intent strategyir.OrderIntent, quantity float64, limitPrice float64, stopPrice float64) {
	if r == nil || statement == nil || quantity <= 0 {
		return
	}
	id := strings.TrimSpace(statement.ID)
	if id == "" {
		id = fmt.Sprintf("line:%d", statement.Range.StartLine)
	}
	r.pendingSequence++
	orderType := normalizeOrderType(statement.OrderType)
	pending := pendingOrder{
		id:                 id,
		sequence:           r.pendingSequence,
		action:             action,
		intent:             intent,
		orderType:          orderType,
		quantity:           quantity,
		quantityMode:       statement.QuantityMode,
		quantityExpression: statement.QuantityExpression,
		entryPolicy:        statement.EntryPolicy,
		rangeInfo:          statement.Range,
		limitPrice:         limitPrice,
		stopPrice:          stopPrice,
		hasLimit:           strings.TrimSpace(statement.LimitExpression) != "",
		hasStop:            strings.TrimSpace(statement.StopExpression) != "",
		comment:            statement.Comment,
		alert:              statement.AlertMessage,
		disable:            statement.DisableAlert,
	}
	if existing, ok := r.pendingOrders[id]; ok &&
		existing.action == pending.action &&
		existing.intent == pending.intent &&
		existing.hasStop == pending.hasStop &&
		existing.hasLimit == pending.hasLimit &&
		existing.stopPrice == pending.stopPrice &&
		existing.limitPrice == pending.limitPrice {
		pending.sequence = existing.sequence
		pending.activated = existing.activated
		pending.submitted = existing.submitted
	}
	if !pending.hasLimit {
		pending.limitPrice = 0
	}
	if !pending.hasStop {
		pending.stopPrice = 0
	}
	r.pendingOrders[id] = pending
	r.internalLog("registered pending order " + id)
}

func (r *strategyRuntime) triggerPendingOrders(kline *types.KLine, scope *evaluationScope) error {
	if r == nil || kline == nil || len(r.pendingOrders) == 0 {
		return nil
	}
	orders := make([]pendingOrder, 0, len(r.pendingOrders))
	for _, order := range r.pendingOrders {
		orders = append(orders, order)
	}
	sort.Slice(orders, func(left, right int) bool {
		return orders[left].sequence < orders[right].sequence
	})
	high := kline.High.Float64()
	low := kline.Low.Float64()
	for _, order := range orders {
		if order.submitted {
			continue
		}
		stopLimitActivated := false
		if order.hasStop && order.hasLimit && !order.activated {
			if !pendingStopTriggered(order, high, low) {
				continue
			}
			order.activated = true
			r.internalLog("activated stop-limit order " + order.id)
			stopLimitActivated = true
		}
		if !stopLimitActivated && !pendingOrderTriggered(order, high, low) {
			continue
		}
		if order.hasStop && order.hasLimit {
			r.pendingOrders[order.id] = order
		} else {
			delete(r.pendingOrders, order.id)
		}
		side, err := exchangeSideForAction(order.action)
		if err != nil {
			return err
		}
		orderType := types.OrderTypeMarket
		limitPrice := 0.0
		if order.hasLimit {
			orderType = types.OrderTypeLimit
			limitPrice = order.limitPrice
		}
		quantity, adjustment, err := r.resolveTriggeredPendingQuantity(order, scope)
		if err != nil {
			return err
		}
		if quantity <= 0 {
			continue
		}
		if err := r.submitOrder(side, orderType, quantity, limitPrice); err != nil {
			return err
		}
		if order.hasStop && order.hasLimit {
			order.submitted = true
			r.pendingOrders[order.id] = order
		}
		r.emitOrderMetadata(order.comment, order.alert, order.disable)
		switch order.intent {
		case strategyir.OrderIntentNet:
			r.resetEntrySubmitCount("LONG")
			r.resetEntrySubmitCount("SHORT")
		case strategyir.OrderIntentEntry:
			r.recordEntryOrderAction(order.action, quantity, 0, 0, adjustment)
		default:
			r.recordSubmittedOrderAction(order.action, quantity, 0, 0)
		}
	}
	return nil
}

func (r *strategyRuntime) resolveTriggeredPendingQuantity(order pendingOrder, scope *evaluationScope) (float64, entryOrderAdjustment, error) {
	position := r.getPosition(r.symbol, timeFromScope(scope))
	availablePositionQty := 0.0
	if position != nil {
		availablePositionQty = math.Floor(absFloat(firstPositiveFloat(absFloat(position.AvailableQuantity), absFloat(position.Quantity))))
	}
	price := order.limitPrice
	if price <= 0 {
		price = order.stopPrice
	}
	if price <= 0 && scope != nil && scope.currentKline != nil {
		price = scope.currentKline.Close.Float64()
	}
	mode, ok := indicatorbinding.ParseQuantityMode(order.quantityMode)
	if !ok {
		return 0, entryOrderAdjustment{}, fmt.Errorf("pine line %d: unsupported order quantity mode %q", order.rangeInfo.StartLine, order.quantityMode)
	}
	quantityExpression := strings.TrimSpace(order.quantityExpression)
	if quantityExpression == "" {
		quantityExpression = "1"
	}
	quantity, err := r.resolveOrderQuantity(&strategyir.OrderStmt{
		Range:              order.rangeInfo,
		Action:             order.action,
		Intent:             order.intent,
		QuantityMode:       mode,
		QuantityExpression: quantityExpression,
	}, scope, position, availablePositionQty, price, mode)
	if err != nil {
		return 0, entryOrderAdjustment{}, fmt.Errorf("pine line %d: %w", order.rangeInfo.StartLine, err)
	}
	if quantity <= 0 {
		return 0, entryOrderAdjustment{}, nil
	}
	quantity, adjustment := r.adjustEntryOrderQuantity(order.action, order.intent, position, availablePositionQty, quantity)
	return quantity, adjustment, nil
}

func timeFromScope(scope *evaluationScope) time.Time {
	if scope == nil {
		return time.Time{}
	}
	return scope.currentKlineTime
}

func pendingOrderTriggered(order pendingOrder, high float64, low float64) bool {
	switch {
	case order.hasStop && order.hasLimit:
		if !order.activated {
			return false
		}
		return pendingLimitTriggered(order, high, low)
	case order.hasStop:
		return pendingStopTriggered(order, high, low)
	case order.hasLimit:
		return pendingLimitTriggered(order, high, low)
	default:
		return false
	}
}

func pendingStopTriggered(order pendingOrder, high float64, low float64) bool {
	switch order.action {
	case strategyir.OrderActionBuy, strategyir.OrderActionCover:
		return high >= order.stopPrice
	case strategyir.OrderActionSell, strategyir.OrderActionShort:
		return low <= order.stopPrice
	default:
		return false
	}
}

func pendingLimitTriggered(order pendingOrder, high float64, low float64) bool {
	switch order.action {
	case strategyir.OrderActionBuy, strategyir.OrderActionCover:
		return low <= order.limitPrice
	case strategyir.OrderActionSell, strategyir.OrderActionShort:
		return high >= order.limitPrice
	default:
		return false
	}
}
