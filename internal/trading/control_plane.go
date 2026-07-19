package trading

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const realTradeControlEventLimit = 200

type RealTradeControlPlane struct {
	mu             sync.Mutex
	path           string
	state          realTradeControlState
	unavailableErr error
}

type realTradeControlState struct {
	RiskConfig *RealTradeRuntimeRiskEntry `json:"riskConfig,omitempty"`
	KillSwitch *RealTradeKillSwitchEntry  `json:"killSwitch,omitempty"`
	HardStops  []RealTradeHardStopEntry   `json:"hardStops,omitempty"`
	Events     []RealTradeControlEvent    `json:"events,omitempty"`
}

type RealTradeRuntimeRiskEntry struct {
	ID                 string   `json:"id" binding:"required"`
	TradingEnvironment string   `json:"tradingEnvironment" binding:"required"`
	RealTradingEnabled bool     `json:"realTradingEnabled" binding:"required"`
	MaxOrderQuantity   *float64 `json:"maxOrderQuantity"`
	MaxOrderNotional   *float64 `json:"maxOrderNotional"`
	OperatorID         string   `json:"operatorId" binding:"required"`
	Reason             string   `json:"reason" binding:"required"`
	ActivatedAt        string   `json:"activatedAt" binding:"required"`
	UpdatedAt          string   `json:"updatedAt" binding:"required"`
}

type RealTradeKillSwitchEntry struct {
	ID                 string `json:"id" binding:"required"`
	TradingEnvironment string `json:"tradingEnvironment" binding:"required"`
	OperatorID         string `json:"operatorId" binding:"required"`
	Reason             string `json:"reason" binding:"required"`
	ActivatedAt        string `json:"activatedAt" binding:"required"`
	UpdatedAt          string `json:"updatedAt" binding:"required"`
}

type RealTradeHardStopEntry struct {
	ID                 string  `json:"id" binding:"required"`
	BrokerID           string  `json:"brokerId" binding:"required"`
	TradingEnvironment string  `json:"tradingEnvironment" binding:"required"`
	AccountID          string  `json:"accountId" binding:"required"`
	Market             *string `json:"market"`
	Symbol             *string `json:"symbol"`
	HardStopScope      string  `json:"hardStopScope" binding:"required"`
	OperatorID         string  `json:"operatorId" binding:"required"`
	Reason             string  `json:"reason" binding:"required"`
	ActivatedAt        string  `json:"activatedAt" binding:"required"`
	UpdatedAt          string  `json:"updatedAt" binding:"required"`
}

type RealTradeControlEvent struct {
	ID                         string   `json:"id" binding:"required"`
	EventType                  string   `json:"eventType" binding:"required"`
	Action                     string   `json:"action" binding:"required"`
	BrokerID                   string   `json:"brokerId" binding:"required"`
	Operation                  *string  `json:"operation"`
	TradingEnvironment         *string  `json:"tradingEnvironment"`
	AccountID                  *string  `json:"accountId"`
	Market                     *string  `json:"market"`
	Symbol                     *string  `json:"symbol"`
	OrderID                    *string  `json:"orderId"`
	Quantity                   *float64 `json:"quantity"`
	Price                      *float64 `json:"price"`
	KillSwitchSource           *string  `json:"killSwitchSource"`
	HardStopScope              *string  `json:"hardStopScope"`
	OperatorID                 *string  `json:"operatorId"`
	Reason                     *string  `json:"reason"`
	ErrorCode                  *string  `json:"errorCode"`
	HardStopID                 *string  `json:"hardStopId"`
	RealTradingEnabled         *bool    `json:"realTradingEnabled"`
	ConfiguredMaxOrderQuantity *float64 `json:"configuredMaxOrderQuantity"`
	ConfiguredMaxOrderNotional *float64 `json:"configuredMaxOrderNotional"`
	ActivatedAt                *string  `json:"activatedAt"`
	CreatedAt                  string   `json:"createdAt" binding:"required"`
}

type RealTradeKillSwitchCommand struct {
	TradingEnvironment string
	OperatorID         string
	Reason             string
}

type RealTradeHardStopCommand struct {
	BrokerID           string
	TradingEnvironment string
	AccountID          string
	Market             string
	Symbol             string
	HardStopScope      string
	OperatorID         string
	Reason             string
}

type RealTradeRuntimeRiskCommand struct {
	TradingEnvironment string
	RealTradingEnabled bool
	MaxOrderQuantity   *float64
	MaxOrderNotional   *float64
	OperatorID         string
	Reason             string
}

func NewRealTradeControlPlane(path string) (*RealTradeControlPlane, error) {
	plane := &RealTradeControlPlane{path: strings.TrimSpace(path)}
	if plane.path == "" {
		return plane, nil
	}
	if err := plane.load(); err != nil {
		plane.unavailableErr = err
		return plane, err
	}
	return plane, nil
}

func (p *RealTradeControlPlane) EvaluatePlaceOrder(_ context.Context, command ExecutionOrderCommand) PreTradeRiskDecision {
	config := p.config()
	gateway := NewStaticPreTradeRiskGateway(func() PreTradeRiskConfig { return config })
	decision := gateway.EvaluatePlaceOrder(context.Background(), command)
	if decision.Allows() || !strings.EqualFold(command.Query.TradingEnvironment, "REAL") {
		return decision
	}
	if decision.ReasonCode == "REAL_TRADE_HARD_STOP_ACTIVE" {
		p.recordRejectedHardStop(command, decision)
	}
	return decision
}

func (p *RealTradeControlPlane) Snapshot() RealTradeRiskSnapshot {
	return NewStaticPreTradeRiskGateway(func() PreTradeRiskConfig {
		return p.config()
	}).Snapshot()
}

func (p *RealTradeControlPlane) ActivateKillSwitch(_ context.Context, command RealTradeKillSwitchCommand) (RealTradeRiskSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.availabilityErrorLocked(); err != nil {
		return RealTradeRiskSnapshot{}, err
	}
	previousState := cloneRealTradeControlState(p.state)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	env := normalizeRealTradeEnvironment(command.TradingEnvironment)
	entry := &RealTradeKillSwitchEntry{
		ID:                 "kill-switch-control-plane",
		TradingEnvironment: env,
		OperatorID:         normalizeOperatorID(command.OperatorID),
		Reason:             strings.TrimSpace(command.Reason),
		ActivatedAt:        now,
		UpdatedAt:          now,
	}
	if p.state.KillSwitch != nil && p.state.KillSwitch.ActivatedAt != "" {
		entry.ActivatedAt = p.state.KillSwitch.ActivatedAt
	}
	p.state.KillSwitch = entry
	source := "RUNTIME"
	p.appendEventLocked(RealTradeControlEvent{
		ID:                 nextRealTradeControlID("rtks-event"),
		EventType:          "activated",
		Action:             "KILL_SWITCH_ACTIVATE",
		BrokerID:           "*",
		TradingEnvironment: new(env),
		KillSwitchSource:   &source,
		OperatorID:         new(entry.OperatorID),
		Reason:             nullableString(entry.Reason),
		ActivatedAt:        new(entry.ActivatedAt),
		CreatedAt:          now,
	})
	if err := p.persistLocked(); err != nil {
		p.state = previousState
		return RealTradeRiskSnapshot{}, err
	}
	return p.snapshotLocked(), nil
}

func (p *RealTradeControlPlane) ReleaseKillSwitch(_ context.Context, command RealTradeKillSwitchCommand) (RealTradeRiskSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.availabilityErrorLocked(); err != nil {
		return RealTradeRiskSnapshot{}, err
	}
	previousState := cloneRealTradeControlState(p.state)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	previous := p.state.KillSwitch
	p.state.KillSwitch = nil
	source := "RUNTIME"
	env := normalizeRealTradeEnvironment(command.TradingEnvironment)
	operatorID := normalizeOperatorID(command.OperatorID)
	reason := strings.TrimSpace(command.Reason)
	activatedAt := (*string)(nil)
	if previous != nil {
		env = previous.TradingEnvironment
		activatedAt = new(previous.ActivatedAt)
	}
	p.appendEventLocked(RealTradeControlEvent{
		ID:                 nextRealTradeControlID("rtks-event"),
		EventType:          "released",
		Action:             "KILL_SWITCH_RELEASE",
		BrokerID:           "*",
		TradingEnvironment: new(env),
		KillSwitchSource:   &source,
		OperatorID:         new(operatorID),
		Reason:             nullableString(reason),
		ActivatedAt:        activatedAt,
		CreatedAt:          now,
	})
	if err := p.persistLocked(); err != nil {
		p.state = previousState
		return RealTradeRiskSnapshot{}, err
	}
	return p.snapshotLocked(), nil
}

func (p *RealTradeControlPlane) UpdateRuntimeRiskConfig(_ context.Context, command RealTradeRuntimeRiskCommand) (RealTradeRiskSnapshot, error) {
	if command.RealTradingEnabled && !hasPositiveLimit(command.MaxOrderQuantity) && !hasPositiveLimit(command.MaxOrderNotional) {
		return RealTradeRiskSnapshot{}, fmt.Errorf("at least one positive runtime risk limit is required before enabling real trading")
	}
	if err := validateRuntimeRiskLimit(command.MaxOrderQuantity, "maxOrderQuantity"); err != nil {
		return RealTradeRiskSnapshot{}, err
	}
	if err := validateRuntimeRiskLimit(command.MaxOrderNotional, "maxOrderNotional"); err != nil {
		return RealTradeRiskSnapshot{}, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.availabilityErrorLocked(); err != nil {
		return RealTradeRiskSnapshot{}, err
	}
	previousState := cloneRealTradeControlState(p.state)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	env := normalizeRealTradeEnvironment(command.TradingEnvironment)
	operatorID := normalizeOperatorID(command.OperatorID)
	reason := strings.TrimSpace(command.Reason)
	entry := &RealTradeRuntimeRiskEntry{
		ID:                 "runtime-risk-config",
		TradingEnvironment: env,
		RealTradingEnabled: command.RealTradingEnabled,
		MaxOrderQuantity:   cloneFloat(command.MaxOrderQuantity),
		MaxOrderNotional:   cloneFloat(command.MaxOrderNotional),
		OperatorID:         operatorID,
		Reason:             reason,
		ActivatedAt:        now,
		UpdatedAt:          now,
	}
	if p.state.RiskConfig != nil && p.state.RiskConfig.ActivatedAt != "" {
		entry.ActivatedAt = p.state.RiskConfig.ActivatedAt
	}
	p.state.RiskConfig = entry
	p.appendEventLocked(RealTradeControlEvent{
		ID:                         nextRealTradeControlID("rtrc-event"),
		EventType:                  "updated",
		Action:                     "RISK_CONFIG_UPDATED",
		BrokerID:                   "*",
		TradingEnvironment:         new(env),
		OperatorID:                 new(operatorID),
		Reason:                     nullableString(reason),
		RealTradingEnabled:         new(command.RealTradingEnabled),
		ConfiguredMaxOrderQuantity: cloneFloat(command.MaxOrderQuantity),
		ConfiguredMaxOrderNotional: cloneFloat(command.MaxOrderNotional),
		ActivatedAt:                new(entry.ActivatedAt),
		CreatedAt:                  now,
	})
	if err := p.persistLocked(); err != nil {
		p.state = previousState
		return RealTradeRiskSnapshot{}, err
	}
	return p.snapshotLocked(), nil
}

func (p *RealTradeControlPlane) DisableRuntimeRiskConfig(_ context.Context, command RealTradeRuntimeRiskCommand) (RealTradeRiskSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.availabilityErrorLocked(); err != nil {
		return RealTradeRiskSnapshot{}, err
	}
	previousState := cloneRealTradeControlState(p.state)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	previous := p.state.RiskConfig
	p.state.RiskConfig = nil
	env := normalizeRealTradeEnvironment(command.TradingEnvironment)
	operatorID := normalizeOperatorID(command.OperatorID)
	reason := strings.TrimSpace(command.Reason)
	activatedAt := (*string)(nil)
	if previous != nil {
		env = previous.TradingEnvironment
		activatedAt = new(previous.ActivatedAt)
	}
	disabled := false
	p.appendEventLocked(RealTradeControlEvent{
		ID:                 nextRealTradeControlID("rtrc-event"),
		EventType:          "disabled",
		Action:             "RISK_CONFIG_DISABLED",
		BrokerID:           "*",
		TradingEnvironment: new(env),
		OperatorID:         new(operatorID),
		Reason:             nullableString(reason),
		RealTradingEnabled: &disabled,
		ActivatedAt:        activatedAt,
		CreatedAt:          now,
	})
	if err := p.persistLocked(); err != nil {
		p.state = previousState
		return RealTradeRiskSnapshot{}, err
	}
	return p.snapshotLocked(), nil
}

func (p *RealTradeControlPlane) ActivateHardStop(_ context.Context, command RealTradeHardStopCommand) (RealTradeRiskSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.availabilityErrorLocked(); err != nil {
		return RealTradeRiskSnapshot{}, err
	}
	previousState := cloneRealTradeControlState(p.state)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	entry := RealTradeHardStopEntry{
		ID:                 nextRealTradeControlID("rths"),
		BrokerID:           normalizeBrokerID(command.BrokerID),
		TradingEnvironment: normalizeRealTradeEnvironment(command.TradingEnvironment),
		AccountID:          normalizeAccountID(command.AccountID),
		Market:             nullableUpper(command.Market),
		Symbol:             nullableUpper(command.Symbol),
		HardStopScope:      normalizeHardStopScope(command.HardStopScope, command.Market, command.Symbol),
		OperatorID:         normalizeOperatorID(command.OperatorID),
		Reason:             strings.TrimSpace(command.Reason),
		ActivatedAt:        now,
		UpdatedAt:          now,
	}
	p.state.HardStops = append(p.state.HardStops, entry)
	p.appendEventLocked(RealTradeControlEvent{
		ID:                 nextRealTradeControlID("rths-event"),
		EventType:          "activated",
		Action:             "HARD_STOP_ACTIVATE",
		BrokerID:           entry.BrokerID,
		TradingEnvironment: new(entry.TradingEnvironment),
		AccountID:          new(entry.AccountID),
		Market:             entry.Market,
		Symbol:             entry.Symbol,
		HardStopScope:      new(entry.HardStopScope),
		OperatorID:         new(entry.OperatorID),
		Reason:             nullableString(entry.Reason),
		HardStopID:         new(entry.ID),
		ActivatedAt:        new(entry.ActivatedAt),
		CreatedAt:          now,
	})
	if err := p.persistLocked(); err != nil {
		p.state = previousState
		return RealTradeRiskSnapshot{}, err
	}
	return p.snapshotLocked(), nil
}

func (p *RealTradeControlPlane) ReleaseHardStop(_ context.Context, id string, command RealTradeHardStopCommand) (RealTradeRiskSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.availabilityErrorLocked(); err != nil {
		return RealTradeRiskSnapshot{}, err
	}
	previousState := cloneRealTradeControlState(p.state)

	id = strings.TrimSpace(id)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	operatorID := normalizeOperatorID(command.OperatorID)
	reason := strings.TrimSpace(command.Reason)
	next := make([]RealTradeHardStopEntry, 0, len(p.state.HardStops))
	var released *RealTradeHardStopEntry
	for _, entry := range p.state.HardStops {
		if entry.ID == id {
			copyEntry := entry
			released = &copyEntry
			continue
		}
		next = append(next, entry)
	}
	if released == nil {
		return RealTradeRiskSnapshot{}, fmt.Errorf("real-trade hard stop not found")
	}
	p.state.HardStops = next
	p.appendEventLocked(RealTradeControlEvent{
		ID:                 nextRealTradeControlID("rths-event"),
		EventType:          "released",
		Action:             "HARD_STOP_RELEASE",
		BrokerID:           released.BrokerID,
		TradingEnvironment: new(released.TradingEnvironment),
		AccountID:          new(released.AccountID),
		Market:             released.Market,
		Symbol:             released.Symbol,
		HardStopScope:      new(released.HardStopScope),
		OperatorID:         new(operatorID),
		Reason:             nullableString(reason),
		HardStopID:         new(released.ID),
		ActivatedAt:        new(released.ActivatedAt),
		CreatedAt:          now,
	})
	if err := p.persistLocked(); err != nil {
		p.state = previousState
		return RealTradeRiskSnapshot{}, err
	}
	return p.snapshotLocked(), nil
}

func (p *RealTradeControlPlane) config() PreTradeRiskConfig {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.configLocked()
}

func (p *RealTradeControlPlane) snapshotLocked() RealTradeRiskSnapshot {
	config := p.configLocked()
	return NewStaticPreTradeRiskGateway(func() PreTradeRiskConfig { return config }).Snapshot()
}

func (p *RealTradeControlPlane) configLocked() PreTradeRiskConfig {
	config := PreTradeRiskConfig{}
	if p.state.RiskConfig != nil {
		config.RealTradingEnabled = p.state.RiskConfig.RealTradingEnabled
		config.RuntimeMaxOrderQty = cloneFloat(p.state.RiskConfig.MaxOrderQuantity)
		config.RuntimeMaxOrderNotional = cloneFloat(p.state.RiskConfig.MaxOrderNotional)
		config.RuntimeRiskEntry = cloneRuntimeRiskEntry(p.state.RiskConfig)
	}
	config.RuntimeKillSwitch = p.state.KillSwitch != nil
	if p.unavailableErr != nil {
		config.RealTradingEnabled = true
		config.RuntimeKillSwitch = true
		config.RuntimeError = p.unavailableErr.Error()
	}
	config.RuntimeHardStops = append([]RealTradeHardStopEntry(nil), p.state.HardStops...)
	config.KillSwitchEntry = cloneKillSwitchEntry(p.state.KillSwitch)
	config.RuntimeEvents = append([]RealTradeControlEvent(nil), p.state.Events...)
	return config
}

func (p *RealTradeControlPlane) recordRejectedHardStop(command ExecutionOrderCommand, decision PreTradeRiskDecision) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	event := RealTradeControlEvent{
		ID:                 nextRealTradeControlID("rths-event"),
		EventType:          "rejected",
		Action:             "HARD_STOP_REJECT",
		BrokerID:           normalizeBrokerID(command.BrokerID),
		Operation:          new("PLACE"),
		TradingEnvironment: new(strings.ToUpper(strings.TrimSpace(command.Query.TradingEnvironment))),
		AccountID:          nullableString(command.Query.AccountID),
		Market:             nullableUpper(command.Query.Market),
		Symbol:             nullableUpper(command.Symbol),
		Quantity:           &command.Query.Quantity,
		Price:              commandRiskPrice(command),
		ErrorCode:          nullableString(decision.ReasonCode),
		Reason:             nullableString(decision.ReasonMessage),
		CreatedAt:          now,
	}
	if decision.Snapshot != nil {
		if matched := decision.Snapshot.MatchedHardStop; matched != nil {
			event.HardStopScope = new(matched.HardStopScope)
			event.HardStopID = new(matched.ID)
			event.ActivatedAt = new(matched.ActivatedAt)
		}
	}
	p.appendEventLocked(event)
	_ = p.persistLocked()
}

func (p *RealTradeControlPlane) appendEventLocked(event RealTradeControlEvent) {
	p.state.Events = append([]RealTradeControlEvent{event}, p.state.Events...)
	if len(p.state.Events) > realTradeControlEventLimit {
		p.state.Events = p.state.Events[:realTradeControlEventLimit]
	}
}

func (p *RealTradeControlPlane) load() error {
	data, err := os.ReadFile(p.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read real-trade control state: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, &p.state); err != nil {
		return fmt.Errorf("decode real-trade control state: %w", err)
	}
	return nil
}

func (p *RealTradeControlPlane) persistLocked() error {
	if strings.TrimSpace(p.path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p.path), 0o700); err != nil {
		return fmt.Errorf("create real-trade control dir: %w", err)
	}
	data, err := json.MarshalIndent(p.state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode real-trade control state: %w", err)
	}
	tmp := p.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write real-trade control state: %w", err)
	}
	if err := os.Rename(tmp, p.path); err != nil {
		return fmt.Errorf("replace real-trade control state: %w", err)
	}
	return nil
}

func (p *RealTradeControlPlane) availabilityErrorLocked() error {
	if p == nil || p.unavailableErr == nil {
		return nil
	}
	return fmt.Errorf("real-trade control plane is unavailable: %w", p.unavailableErr)
}

func cloneRealTradeControlState(state realTradeControlState) realTradeControlState {
	return realTradeControlState{
		RiskConfig: cloneRuntimeRiskEntry(state.RiskConfig),
		KillSwitch: cloneKillSwitchEntry(state.KillSwitch),
		HardStops:  append([]RealTradeHardStopEntry(nil), state.HardStops...),
		Events:     append([]RealTradeControlEvent(nil), state.Events...),
	}
}

func cloneRuntimeRiskEntry(entry *RealTradeRuntimeRiskEntry) *RealTradeRuntimeRiskEntry {
	if entry == nil {
		return nil
	}
	clone := *entry
	clone.MaxOrderQuantity = cloneFloat(entry.MaxOrderQuantity)
	clone.MaxOrderNotional = cloneFloat(entry.MaxOrderNotional)
	return &clone
}

func hasPositiveLimit(value *float64) bool {
	return value != nil && *value > 0
}

func validateRuntimeRiskLimit(value *float64, name string) error {
	if value == nil || *value > 0 {
		return nil
	}
	return fmt.Errorf("%s must be positive when provided", name)
}

func normalizeRealTradeEnvironment(value string) string {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	if normalized == "" {
		return "REAL"
	}
	return normalized
}

func normalizeBrokerID(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "*"
	}
	return normalized
}

func normalizeAccountID(value string) string {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "*"
	}
	return normalized
}

func normalizeOperatorID(value string) string {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "local"
	}
	return normalized
}

func normalizeHardStopScope(value string, market string, symbol string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "ACCOUNT", "MARKET", "SYMBOL":
		return strings.ToUpper(strings.TrimSpace(value))
	default:
		if strings.TrimSpace(symbol) != "" {
			return "SYMBOL"
		}
		if strings.TrimSpace(market) != "" {
			return "MARKET"
		}
		return "ACCOUNT"
	}
}

func nullableUpper(value string) *string {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	if normalized == "" {
		return nil
	}
	return &normalized
}

func nullableString(value string) *string {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return nil
	}
	return &normalized
}

func cloneKillSwitchEntry(entry *RealTradeKillSwitchEntry) *RealTradeKillSwitchEntry {
	if entry == nil {
		return nil
	}
	cloned := *entry
	return &cloned
}

func nextRealTradeControlID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().UnixNano())
}
