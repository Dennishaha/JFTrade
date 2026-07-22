package trading

import (
	"context"
	"errors"
	"math"
	"strings"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

const (
	RiskDecisionAllow           = "ALLOW"
	RiskDecisionReject          = "REJECT"
	RiskDecisionRequireApproval = "REQUIRE_APPROVAL"
)

type PreTradeRiskGateway interface {
	EvaluatePlaceOrder(context.Context, ExecutionOrderCommand) PreTradeRiskDecision
	Snapshot() RealTradeRiskSnapshot
}

// preTradeRiskExecutionGateway lets a mutable gateway serialize its final
// decision with broker submission. This closes the evaluate-then-activate gap
// without exposing callback execution as part of the public gateway contract.
type preTradeRiskExecutionGateway interface {
	executePlaceOrder(context.Context, ExecutionOrderCommand, func() error) error
}

type PreTradeRiskDecision struct {
	Decision      string                 `json:"decision"`
	ReasonCode    string                 `json:"reasonCode,omitempty"`
	ReasonMessage string                 `json:"reasonMessage,omitempty"`
	Snapshot      *RealTradeRiskSnapshot `json:"riskSnapshot,omitempty"`
}

func (d PreTradeRiskDecision) Allows() bool {
	return strings.EqualFold(strings.TrimSpace(d.Decision), RiskDecisionAllow)
}

func (d PreTradeRiskDecision) RequiresApproval() bool {
	return strings.EqualFold(strings.TrimSpace(d.Decision), RiskDecisionRequireApproval)
}

type RiskRejectedError struct {
	Decision PreTradeRiskDecision
}

func (e RiskRejectedError) Error() string {
	if strings.TrimSpace(e.Decision.ReasonMessage) != "" {
		return e.Decision.ReasonMessage
	}
	if strings.TrimSpace(e.Decision.ReasonCode) != "" {
		return e.Decision.ReasonCode
	}
	return "pre-trade risk rejected the execution order"
}

func IsRiskRejected(err error) bool {
	var target RiskRejectedError
	return errors.As(err, &target)
}

type PreTradeRiskConfig struct {
	RealTradingEnabled      bool
	RuntimeKillSwitch       bool
	RuntimeError            string
	RuntimeMaxOrderQty      *float64
	RuntimeMaxOrderNotional *float64
	RuntimeHardStops        []RealTradeHardStopEntry
	RuntimeRiskEntry        *RealTradeRuntimeRiskEntry
	KillSwitchEntry         *RealTradeKillSwitchEntry
	RuntimeEvents           []RealTradeControlEvent
}

type StaticPreTradeRiskGateway struct {
	config func() PreTradeRiskConfig
}

func NewStaticPreTradeRiskGateway(config func() PreTradeRiskConfig) *StaticPreTradeRiskGateway {
	return &StaticPreTradeRiskGateway{config: config}
}

func (g *StaticPreTradeRiskGateway) EvaluatePlaceOrder(_ context.Context, command ExecutionOrderCommand) PreTradeRiskDecision {
	snapshot := g.Snapshot()
	if !strings.EqualFold(strings.TrimSpace(command.Query.TradingEnvironment), "REAL") {
		return PreTradeRiskDecision{Decision: RiskDecisionAllow, Snapshot: &snapshot}
	}
	if !snapshot.RealTradingEnabled {
		return riskRejected("REAL_TRADING_DISABLED", "real trading is disabled; enable runtime real-trade risk config before placing REAL orders", snapshot)
	}
	if snapshot.KillSwitchActive {
		return riskRejected("REAL_TRADE_KILL_SWITCH_ACTIVE", "real-trade kill switch is active; PLACE orders are blocked", snapshot)
	}
	if matched := matchHardStop(PreTradeRiskConfig{RuntimeHardStops: snapshot.HardStopEntries}, command); matched != nil {
		snapshot.MatchedHardStop = matched
		return riskRejected("REAL_TRADE_HARD_STOP_ACTIVE", "real-trade hard stop is active for this order scope; PLACE orders are blocked", snapshot)
	}
	if code, message := commandRiskShapeError(command); code != "" {
		return riskRejected(code, message, snapshot)
	}
	amountMode := commandUsesAmount(command)
	if limit := snapshot.EffectiveMaxOrderQuantity; limit != nil {
		if amountMode {
			amount := commandRiskAmount(command)
			if amount == nil {
				return riskRejected("RISK_AMOUNT_UNAVAILABLE", "order amount is required to enforce the configured real-trade quantity limit", snapshot)
			}
			// Amount is the primary quantity for QuantityModeAmount. Never derive
			// risk from a caller-controlled limit price or RFQ display price.
			if *amount > *limit {
				return riskRejected("MAX_ORDER_QUANTITY_EXCEEDED", "order amount exceeds the configured real-trade quantity-mode limit", snapshot)
			}
		} else if command.Query.Quantity > *limit {
			return riskRejected("MAX_ORDER_QUANTITY_EXCEEDED", "order quantity exceeds the configured real-trade limit", snapshot)
		}
	}
	if limit := snapshot.EffectiveMaxOrderNotional; limit != nil {
		if amountMode {
			amount := commandRiskAmount(command)
			if amount == nil {
				return riskRejected("RISK_AMOUNT_UNAVAILABLE", "order amount is required to enforce the configured real-trade notional limit", snapshot)
			}
			if *amount > *limit {
				return riskRejected("MAX_ORDER_NOTIONAL_EXCEEDED", "order notional exceeds the configured real-trade limit", snapshot)
			}
		} else {
			riskPrice := commandRiskPrice(command)
			if riskPrice == nil {
				return riskRejected("RISK_PRICE_UNAVAILABLE", "order price is required to enforce the configured real-trade notional limit", snapshot)
			}
			if command.Query.Quantity*(*riskPrice) > *limit {
				return riskRejected("MAX_ORDER_NOTIONAL_EXCEEDED", "order notional exceeds the configured real-trade limit", snapshot)
			}
		}
	}
	return PreTradeRiskDecision{Decision: RiskDecisionAllow, Snapshot: &snapshot}
}

func (g *StaticPreTradeRiskGateway) Snapshot() RealTradeRiskSnapshot {
	config := PreTradeRiskConfig{}
	if g != nil && g.config != nil {
		config = g.config()
	}
	return realTradeRiskSnapshotFromConfig(config)
}

func matchHardStop(config PreTradeRiskConfig, command ExecutionOrderCommand) *RealTradeHardStopEntry {
	for _, entry := range config.RuntimeHardStops {
		if !hardStopMatches(entry, command) {
			continue
		}
		matched := entry
		return &matched
	}
	return nil
}

func hardStopMatches(entry RealTradeHardStopEntry, command ExecutionOrderCommand) bool {
	if value := strings.TrimSpace(entry.BrokerID); value != "" && value != "*" && !strings.EqualFold(value, command.BrokerID) {
		return false
	}
	if value := strings.TrimSpace(entry.TradingEnvironment); value != "" && !strings.EqualFold(value, command.Query.TradingEnvironment) {
		return false
	}
	if value := strings.TrimSpace(entry.AccountID); value != "" && value != "*" && !strings.EqualFold(value, command.Query.AccountID) {
		return false
	}
	if entry.Market != nil && strings.TrimSpace(*entry.Market) != "" && !strings.EqualFold(*entry.Market, command.Query.Market) {
		return false
	}
	if entry.Symbol != nil && strings.TrimSpace(*entry.Symbol) != "" && !symbolMatches(*entry.Symbol, command.Query.Market, command.Symbol) {
		return false
	}
	return true
}

func symbolMatches(entrySymbol string, marketCode string, commandSymbol string) bool {
	entry := strings.ToUpper(strings.TrimSpace(entrySymbol))
	symbol := strings.ToUpper(strings.TrimSpace(commandSymbol))
	marketCode = strings.ToUpper(strings.TrimSpace(marketCode))
	if entry == "" || symbol == "" {
		return false
	}
	if entry == symbol {
		return true
	}
	if marketCode != "" && entry == marketCode+"."+symbol {
		return true
	}
	if marketCode != "" && symbol == marketCode+"."+entry {
		return true
	}
	return false
}

func filterRealTradeControlEvents(events []RealTradeControlEvent, actionPrefix string) []RealTradeControlEvent {
	filtered := make([]RealTradeControlEvent, 0, len(events))
	for _, event := range events {
		if strings.HasPrefix(strings.ToUpper(event.Action), actionPrefix) {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

func filterRealTradeRiskEvents(events []RealTradeControlEvent) []RealTradeControlEvent {
	filtered := make([]RealTradeControlEvent, 0, len(events))
	for _, event := range events {
		action := strings.ToUpper(event.Action)
		if strings.HasPrefix(action, "RISK_CONFIG_") || strings.HasPrefix(action, "RISK_LIMIT_") {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

func commandRiskPrice(command ExecutionOrderCommand) *float64 {
	if command.Query.Price != nil && finiteRiskValue(*command.Query.Price) {
		return command.Query.Price
	}
	if command.Query.StopPrice != nil && finiteRiskValue(*command.Query.StopPrice) {
		return command.Query.StopPrice
	}
	return nil
}

func commandUsesAmount(command ExecutionOrderCommand) bool {
	return effectiveRiskQuantityMode(command) == broker.QuantityModeAmount && commandIsEventContract(command)
}

func commandRiskAmount(command ExecutionOrderCommand) *float64 {
	if command.Query.Amount == nil || !finiteRiskValue(*command.Query.Amount) {
		return nil
	}
	return command.Query.Amount
}

func commandRiskShapeError(command ExecutionOrderCommand) (string, string) {
	queryProduct := command.Query.ProductClass
	if queryProduct != "" && command.ProductClass != "" && queryProduct != command.ProductClass {
		return "INVALID_ORDER_RISK_SHAPE", "order product class is inconsistent across the execution command"
	}
	queryMode := command.Query.QuantityMode
	if queryMode != "" && command.QuantityMode != "" && queryMode != command.QuantityMode {
		return "INVALID_ORDER_RISK_SHAPE", "order quantity mode is inconsistent across the execution command"
	}
	eventContract := commandIsEventContract(command)
	mode := effectiveRiskQuantityMode(command)
	if eventContract {
		if mode != broker.QuantityModeAmount {
			return "INVALID_ORDER_RISK_SHAPE", "event-contract orders must use amount quantity mode"
		}
		if commandRiskAmount(command) == nil {
			return "RISK_AMOUNT_UNAVAILABLE", "a positive finite order amount is required for event-contract risk evaluation"
		}
		return "", ""
	}
	if mode == broker.QuantityModeAmount || command.Query.Amount != nil {
		return "INVALID_ORDER_RISK_SHAPE", "amount quantity mode is supported for event-contract orders only"
	}
	if strings.TrimSpace(command.Query.PredictionSide) != "" {
		return "INVALID_ORDER_RISK_SHAPE", "predictionSide is supported for event-contract orders only"
	}
	if !finiteRiskValue(command.Query.Quantity) {
		return "RISK_QUANTITY_UNAVAILABLE", "a positive finite order quantity is required for risk evaluation"
	}
	return "", ""
}

func commandIsEventContract(command ExecutionOrderCommand) bool {
	return command.ProductClass == broker.ProductClassEventContract ||
		command.Query.ProductClass == broker.ProductClassEventContract ||
		command.OrderKind == broker.OrderKindEventSingle ||
		command.OrderKind == broker.OrderKindEventParlay
}

func effectiveRiskQuantityMode(command ExecutionOrderCommand) broker.QuantityMode {
	if command.Query.QuantityMode != "" {
		return command.Query.QuantityMode
	}
	return command.QuantityMode
}

func finiteRiskValue(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func riskRejected(code, message string, snapshot RealTradeRiskSnapshot) PreTradeRiskDecision {
	return PreTradeRiskDecision{
		Decision:      RiskDecisionReject,
		ReasonCode:    code,
		ReasonMessage: message,
		Snapshot:      &snapshot,
	}
}

func positiveFloat(value *float64) *float64 {
	if value == nil || *value <= 0 {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func runtimeSwitchSource(config PreTradeRiskConfig) *string {
	if config.RuntimeKillSwitch {
		source := "RUNTIME"
		return &source
	}
	return nil
}
