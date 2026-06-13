package pineruntime

import (
	"fmt"
	"sort"
	"strings"

	"github.com/c9s/bbgo/pkg/types"
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
		id:         id,
		sequence:   r.pendingSequence,
		action:     action,
		intent:     intent,
		orderType:  orderType,
		quantity:   quantity,
		limitPrice: limitPrice,
		stopPrice:  stopPrice,
		hasLimit:   strings.TrimSpace(statement.LimitExpression) != "",
		hasStop:    strings.TrimSpace(statement.StopExpression) != "",
		comment:    statement.Comment,
		alert:      statement.AlertMessage,
		disable:    statement.DisableAlert,
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

func (r *strategyRuntime) triggerPendingOrders(kline *types.KLine) error {
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
		if !pendingOrderTriggered(order, high, low) {
			continue
		}
		delete(r.pendingOrders, order.id)
		side, err := exchangeSideForAction(order.action)
		if err != nil {
			return err
		}
		orderType := types.OrderTypeMarket
		limitPrice := 0.0
		if order.hasLimit && !order.hasStop {
			orderType = types.OrderTypeLimit
			limitPrice = order.limitPrice
		}
		if err := r.submitOrder(side, orderType, order.quantity, limitPrice); err != nil {
			return err
		}
		r.emitOrderMetadata(order.comment, order.alert, order.disable)
		switch order.intent {
		case strategyir.OrderIntentNet:
			r.resetEntrySubmitCount("LONG")
			r.resetEntrySubmitCount("SHORT")
		default:
			r.recordSubmittedOrderAction(order.action, order.quantity, 0, 0)
		}
	}
	return nil
}

func pendingOrderTriggered(order pendingOrder, high float64, low float64) bool {
	switch {
	case order.hasStop:
		switch order.action {
		case strategyir.OrderActionBuy, strategyir.OrderActionCover:
			return high >= order.stopPrice
		case strategyir.OrderActionSell, strategyir.OrderActionShort:
			return low <= order.stopPrice
		default:
			return false
		}
	case order.hasLimit:
		switch order.action {
		case strategyir.OrderActionBuy, strategyir.OrderActionCover:
			return low <= order.limitPrice
		case strategyir.OrderActionSell, strategyir.OrderActionShort:
			return high >= order.limitPrice
		default:
			return false
		}
	default:
		return false
	}
}
