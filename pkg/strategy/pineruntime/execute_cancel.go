package pineruntime

import (
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func (r *strategyRuntime) executeCancelStatement(statement *strategyir.CancelStmt) {
	if r == nil || statement == nil || len(r.pendingOrders) == 0 {
		return
	}
	if statement.All {
		clear(r.pendingOrders)
		r.internalLog("cancelled all pending orders")
		return
	}
	id := strings.TrimSpace(statement.ID)
	if id == "" {
		return
	}
	if _, ok := r.pendingOrders[id]; ok {
		delete(r.pendingOrders, id)
		r.internalLog("cancelled pending order " + id)
		return
	}
	r.internalLog("pending order " + id + " not found for cancel")
}
