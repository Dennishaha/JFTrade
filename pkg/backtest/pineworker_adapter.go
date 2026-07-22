package backtest

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"

	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

type PineWorkerRunner interface {
	RunScript(context.Context, pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error)
}

type WorkerOrderCommand struct {
	Kind         string
	ID           string
	FromEntry    string
	Direction    string
	Side         types.SideType
	OrderType    types.OrderType
	Quantity     float64
	QuantityPct  float64
	LimitPrice   float64
	StopPrice    float64
	Comment      string
	AlertMessage string
	DisableAlert bool
	BarIndex     int
	Time         int64
}

type PineWorkerBacktestAdapter struct {
	Runner PineWorkerRunner
}

func (adapter PineWorkerBacktestAdapter) Run(ctx context.Context, request pineworker.RunScriptRequest) ([]WorkerOrderCommand, pineworker.WorkerMetadata, error) {
	if adapter.Runner == nil {
		return nil, pineworker.WorkerMetadata{}, fmt.Errorf("pine worker backtest runner is required")
	}
	request.Mode = pineworker.ModeBacktest
	response, err := adapter.Runner.RunScript(ctx, request)
	if err != nil {
		return nil, response.Metadata, fmt.Errorf("pine worker backtest run: %w", err)
	}
	if response.Error != "" {
		return nil, response.Metadata, fmt.Errorf("pine worker backtest error: %s", response.Error)
	}
	commands, err := CommandsFromOrderIntents(response.OrderIntents)
	if err != nil {
		return nil, response.Metadata, err
	}
	return commands, response.Metadata, nil
}

func CommandsFromOrderIntents(intents []pineworker.OrderIntent) ([]WorkerOrderCommand, error) {
	commands := make([]WorkerOrderCommand, 0, len(intents))
	for _, intent := range intents {
		command, ok, err := CommandFromOrderIntent(intent)
		if err != nil {
			return nil, err
		}
		if ok {
			commands = append(commands, command)
		}
	}
	return commands, nil
}

func CommandFromOrderIntent(intent pineworker.OrderIntent) (WorkerOrderCommand, bool, error) {
	kind := normalizeWorkerIntentKind(intent.Kind)
	if kind == "" {
		return WorkerOrderCommand{}, false, fmt.Errorf("unsupported pine worker order intent kind: %s", intent.Kind)
	}
	if kind == "cancel" || kind == "cancel_all" {
		return WorkerOrderCommand{
			Kind:     kind,
			ID:       intent.ID,
			BarIndex: intent.BarIndex,
			Time:     intent.Time,
		}, true, nil
	}
	direction := canonicalWorkerIntentDirection(kind, intent.Direction)
	side, err := sideForWorkerIntent(kind, direction)
	if err != nil {
		return WorkerOrderCommand{}, false, err
	}
	hasLimitPrice := intent.HasLimitPrice || intent.LimitPrice != 0
	hasStopPrice := intent.HasStopPrice || intent.StopPrice != 0
	if hasLimitPrice && (intent.LimitPrice <= 0 || math.IsNaN(intent.LimitPrice) || math.IsInf(intent.LimitPrice, 0)) {
		return WorkerOrderCommand{}, false, fmt.Errorf("pine worker %s intent limit price must be positive and finite", kind)
	}
	if hasStopPrice && (intent.StopPrice <= 0 || math.IsNaN(intent.StopPrice) || math.IsInf(intent.StopPrice, 0)) {
		return WorkerOrderCommand{}, false, fmt.Errorf("pine worker %s intent stop price must be positive and finite", kind)
	}
	orderType := types.OrderTypeMarket
	switch {
	case hasLimitPrice && hasStopPrice:
		if kind == "exit" {
			return WorkerOrderCommand{}, false, fmt.Errorf("pine worker exit intent with both limit and stop prices is an OCO bracket that the order command protocol cannot safely express")
		}
		if kind != "entry" && kind != "order" {
			return WorkerOrderCommand{}, false, fmt.Errorf("pine worker %s intent cannot combine limit and stop prices", kind)
		}
		orderType = types.OrderTypeStopLimit
	case hasStopPrice:
		orderType = types.OrderTypeStopMarket
	case hasLimitPrice:
		orderType = types.OrderTypeLimit
	}
	command := WorkerOrderCommand{
		Kind:         kind,
		ID:           intent.ID,
		FromEntry:    intent.FromEntry,
		Direction:    direction,
		Side:         side,
		OrderType:    orderType,
		Quantity:     intent.Quantity,
		QuantityPct:  intent.QuantityPct,
		LimitPrice:   intent.LimitPrice,
		StopPrice:    intent.StopPrice,
		Comment:      intent.Comment,
		AlertMessage: intent.AlertMessage,
		DisableAlert: intent.DisableAlert,
		BarIndex:     intent.BarIndex,
		Time:         intent.Time,
	}
	if !intent.HasQuantity && !intent.HasQuantityPct && (kind == "entry" || kind == "order") {
		command.Quantity = 1
	}
	return command, true, nil
}

func canonicalWorkerIntentDirection(kind string, direction string) string {
	normalizedDirection := strings.TrimSpace(strings.ToLower(direction))
	switch kind {
	case "entry", "order":
		switch normalizedDirection {
		case "buy":
			return "long"
		case "sell":
			return "short"
		}
	case "exit", "close", "close_all":
		switch normalizedDirection {
		case "buy", "cover":
			return "short"
		case "sell":
			return "long"
		}
	}
	return normalizedDirection
}

func normalizeWorkerIntentKind(kind string) string {
	switch strings.TrimSpace(strings.ToLower(kind)) {
	case "entry", "order", "exit", "close", "close_all", "cancel", "cancel_all":
		return strings.TrimSpace(strings.ToLower(kind))
	default:
		return ""
	}
}

func sideForWorkerIntent(kind string, direction string) (types.SideType, error) {
	normalizedDirection := strings.TrimSpace(strings.ToLower(direction))
	switch kind {
	case "entry", "order":
		switch normalizedDirection {
		case "long", "buy":
			return types.SideTypeBuy, nil
		case "short", "sell":
			return types.SideTypeSell, nil
		default:
			return "", fmt.Errorf("pine worker %s intent requires long/short direction", kind)
		}
	case "exit", "close", "close_all":
		switch normalizedDirection {
		case "short", "buy", "cover":
			return types.SideTypeBuy, nil
		case "long", "sell", "flat", "":
			return types.SideTypeSell, nil
		default:
			return "", fmt.Errorf("unsupported pine worker close direction: %s", direction)
		}
	default:
		return "", fmt.Errorf("unsupported pine worker order intent kind: %s", kind)
	}
}
