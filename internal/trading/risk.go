package trading

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
)

const (
	RiskDecisionAllow           = "ALLOW"
	RiskDecisionReject          = "REJECT"
	RiskDecisionRequireApproval = "REQUIRE_APPROVAL"
)

type PreTradeRiskGateway interface {
	EvaluatePlaceOrder(context.Context, ExecutionOrderCommand) PreTradeRiskDecision
	Snapshot() map[string]any
}

type PreTradeRiskDecision struct {
	Decision      string         `json:"decision"`
	ReasonCode    string         `json:"reasonCode,omitempty"`
	ReasonMessage string         `json:"reasonMessage,omitempty"`
	Snapshot      map[string]any `json:"riskSnapshot,omitempty"`
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
	RealTradingEnabled            bool
	EnvConfiguredKillSwitch       bool
	ControlPlaneKillSwitch        bool
	ControlPlaneError             string
	EnvConfiguredMaxOrderQty      *float64
	EnvConfiguredMaxOrderNotional *float64
	EnvConfiguredApprovalNotional *float64
	ControlPlaneMaxOrderQty       *float64
	ControlPlaneMaxOrderNotional  *float64
	ControlPlaneHardStops         []RealTradeHardStopEntry
	KillSwitchEntry               *RealTradeKillSwitchEntry
	ControlPlaneEvents            []RealTradeControlEvent
}

type StaticPreTradeRiskGateway struct {
	config func() PreTradeRiskConfig
}

func NewStaticPreTradeRiskGateway(config func() PreTradeRiskConfig) *StaticPreTradeRiskGateway {
	return &StaticPreTradeRiskGateway{config: config}
}

func NewEnvPreTradeRiskGateway() *StaticPreTradeRiskGateway {
	return NewStaticPreTradeRiskGateway(func() PreTradeRiskConfig {
		return PreTradeRiskConfig{
			RealTradingEnabled:            truthyEnv("JFTRADE_ALLOW_REAL_TRADING"),
			EnvConfiguredKillSwitch:       truthyEnv("JFTRADE_REAL_TRADE_KILL_SWITCH"),
			EnvConfiguredMaxOrderQty:      positiveFloatEnv("JFTRADE_REAL_TRADE_MAX_ORDER_QUANTITY"),
			EnvConfiguredMaxOrderNotional: positiveFloatEnv("JFTRADE_REAL_TRADE_MAX_ORDER_NOTIONAL"),
			EnvConfiguredApprovalNotional: positiveFloatEnv("JFTRADE_REAL_TRADE_APPROVAL_NOTIONAL"),
		}
	})
}

func (g *StaticPreTradeRiskGateway) EvaluatePlaceOrder(_ context.Context, command ExecutionOrderCommand) PreTradeRiskDecision {
	snapshot := g.Snapshot()
	if !strings.EqualFold(strings.TrimSpace(command.Query.TradingEnvironment), "REAL") {
		return PreTradeRiskDecision{Decision: RiskDecisionAllow, Snapshot: snapshot}
	}
	if enabled, _ := snapshot["realTradingEnabled"].(bool); !enabled {
		return riskRejected("REAL_TRADING_DISABLED", "real trading is disabled; set JFTRADE_ALLOW_REAL_TRADING=true before placing REAL orders", snapshot)
	}
	if active, _ := snapshot["killSwitchActive"].(bool); active {
		return riskRejected("REAL_TRADE_KILL_SWITCH_ACTIVE", "real-trade kill switch is active; PLACE orders are blocked", snapshot)
	}
	if matched := matchHardStop(configFromSnapshot(snapshot), command); matched != nil {
		snapshot["matchedHardStop"] = *matched
		return riskRejected("REAL_TRADE_HARD_STOP_ACTIVE", "real-trade hard stop is active for this order scope; PLACE orders are blocked", snapshot)
	}
	if limit, ok := snapshot["effectiveMaxOrderQuantity"].(*float64); ok && limit != nil && command.Query.Quantity > *limit {
		return riskRejected("MAX_ORDER_QUANTITY_EXCEEDED", "order quantity exceeds the configured real-trade limit", snapshot)
	}
	riskPrice := commandRiskPrice(command)
	if limit, ok := snapshot["effectiveMaxOrderNotional"].(*float64); ok && limit != nil {
		if riskPrice == nil {
			return riskRejected("RISK_PRICE_UNAVAILABLE", "order price is required to enforce the configured real-trade notional limit", snapshot)
		}
		if command.Query.Quantity*(*riskPrice) > *limit {
			return riskRejected("MAX_ORDER_NOTIONAL_EXCEEDED", "order notional exceeds the configured real-trade limit", snapshot)
		}
	}
	if threshold, ok := snapshot["approvalRequiredNotional"].(*float64); ok && threshold != nil {
		if riskPrice == nil {
			return riskRequiresApproval("REAL_TRADE_APPROVAL_REQUIRED", "real-trade order matches the approval threshold, but the approval workflow is not implemented yet; order is blocked before broker submission because its notional cannot be determined", snapshot)
		}
		if command.Query.Quantity*(*riskPrice) > *threshold {
			return riskRequiresApproval("REAL_TRADE_APPROVAL_REQUIRED", "real-trade order notional matches the approval threshold, but the approval workflow is not implemented yet; order is blocked before broker submission", snapshot)
		}
	}
	return PreTradeRiskDecision{Decision: RiskDecisionAllow, Snapshot: snapshot}
}

func (g *StaticPreTradeRiskGateway) Snapshot() map[string]any {
	config := PreTradeRiskConfig{}
	if g != nil && g.config != nil {
		config = g.config()
	}
	killSwitchActive := config.EnvConfiguredKillSwitch || config.ControlPlaneKillSwitch
	maxQty := minPositiveFloat(config.EnvConfiguredMaxOrderQty, config.ControlPlaneMaxOrderQty)
	maxNotional := minPositiveFloat(config.EnvConfiguredMaxOrderNotional, config.ControlPlaneMaxOrderNotional)
	riskConfigSource := riskConfigSource(config)
	riskControlPlaneActive := config.ControlPlaneMaxOrderQty != nil || config.ControlPlaneMaxOrderNotional != nil
	hardStops := append([]RealTradeHardStopEntry{}, config.ControlPlaneHardStops...)
	events := append([]RealTradeControlEvent{}, config.ControlPlaneEvents...)
	return map[string]any{
		"realTradingEnabled":            config.RealTradingEnabled,
		"killSwitchActive":              killSwitchActive,
		"killSwitchSource":              killSwitchSource(config),
		"envConfiguredActive":           config.EnvConfiguredKillSwitch,
		"controlPlaneActive":            config.ControlPlaneKillSwitch,
		"killSwitchControlPlaneActive":  config.ControlPlaneKillSwitch,
		"controlPlaneAvailable":         strings.TrimSpace(config.ControlPlaneError) == "",
		"controlPlaneError":             nullableString(config.ControlPlaneError),
		"killSwitchEntry":               cloneKillSwitchEntry(config.KillSwitchEntry),
		"killSwitchEvents":              filterRealTradeControlEvents(events, "KILL_SWITCH_"),
		"blockedOperations":             []string{"PLACE", "MODIFY"},
		"allowsCancel":                  true,
		"hardStopsActive":               len(hardStops) > 0,
		"hardStopEntries":               hardStops,
		"hardStopEvents":                filterRealTradeControlEvents(events, "HARD_STOP_"),
		"riskEnabled":                   maxQty != nil || maxNotional != nil,
		"riskConfigSource":              riskConfigSource,
		"envConfiguredMaxOrderQuantity": config.EnvConfiguredMaxOrderQty,
		"envConfiguredMaxOrderNotional": config.EnvConfiguredMaxOrderNotional,
		"approvalRequiredNotional":      config.EnvConfiguredApprovalNotional,
		"riskControlPlaneActive":        riskControlPlaneActive,
		"controlPlaneMaxOrderQuantity":  config.ControlPlaneMaxOrderQty,
		"controlPlaneMaxOrderNotional":  config.ControlPlaneMaxOrderNotional,
		"effectiveMaxOrderQuantity":     maxQty,
		"effectiveMaxOrderNotional":     maxNotional,
	}
}

func configFromSnapshot(snapshot map[string]any) PreTradeRiskConfig {
	config := PreTradeRiskConfig{}
	if entries, ok := snapshot["hardStopEntries"].([]RealTradeHardStopEntry); ok {
		config.ControlPlaneHardStops = entries
	}
	return config
}

func matchHardStop(config PreTradeRiskConfig, command ExecutionOrderCommand) *RealTradeHardStopEntry {
	for _, entry := range config.ControlPlaneHardStops {
		if !hardStopMatches(entry, command) {
			continue
		}
		matched := entry
		return &matched
	}
	return nil
}

func hardStopMatches(entry RealTradeHardStopEntry, command ExecutionOrderCommand) bool {
	if value := strings.TrimSpace(entry.BrokerID); value != "" && !strings.EqualFold(value, command.BrokerID) {
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

func commandRiskPrice(command ExecutionOrderCommand) *float64 {
	if command.Query.Price != nil {
		return command.Query.Price
	}
	return command.Query.StopPrice
}

func riskRejected(code, message string, snapshot map[string]any) PreTradeRiskDecision {
	return PreTradeRiskDecision{
		Decision:      RiskDecisionReject,
		ReasonCode:    code,
		ReasonMessage: message,
		Snapshot:      snapshot,
	}
}

func riskRequiresApproval(code, message string, snapshot map[string]any) PreTradeRiskDecision {
	return PreTradeRiskDecision{
		Decision:      RiskDecisionRequireApproval,
		ReasonCode:    code,
		ReasonMessage: message,
		Snapshot:      snapshot,
	}
}

func truthyEnv(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "y", "on", "enabled", "enable":
		return true
	default:
		return false
	}
}

func positiveFloatEnv(name string) *float64 {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 {
		return nil
	}
	return &value
}

func minPositiveFloat(values ...*float64) *float64 {
	var selected *float64
	for _, value := range values {
		if value == nil || *value <= 0 {
			continue
		}
		if selected == nil || *value < *selected {
			selected = value
		}
	}
	return selected
}

func riskConfigSource(config PreTradeRiskConfig) any {
	hasEnv := config.EnvConfiguredMaxOrderQty != nil || config.EnvConfiguredMaxOrderNotional != nil
	hasControlPlane := config.ControlPlaneMaxOrderQty != nil || config.ControlPlaneMaxOrderNotional != nil
	switch {
	case hasEnv && hasControlPlane:
		return "MERGED"
	case hasEnv:
		return "ENV"
	case hasControlPlane:
		return "CONTROL_PLANE"
	default:
		return nil
	}
}

func killSwitchSource(config PreTradeRiskConfig) any {
	switch {
	case config.EnvConfiguredKillSwitch:
		return "ENV"
	case config.ControlPlaneKillSwitch:
		return "CONTROL_PLANE"
	default:
		return nil
	}
}
