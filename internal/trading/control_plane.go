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
	KillSwitch *RealTradeKillSwitchEntry `json:"killSwitch,omitempty"`
	HardStops  []RealTradeHardStopEntry  `json:"hardStops,omitempty"`
	Events     []RealTradeControlEvent   `json:"events,omitempty"`
}

type RealTradeKillSwitchEntry struct {
	ID                 string `json:"id"`
	TradingEnvironment string `json:"tradingEnvironment"`
	OperatorID         string `json:"operatorId"`
	Reason             string `json:"reason"`
	ActivatedAt        string `json:"activatedAt"`
	UpdatedAt          string `json:"updatedAt"`
}

type RealTradeHardStopEntry struct {
	ID                 string  `json:"id"`
	BrokerID           string  `json:"brokerId"`
	TradingEnvironment string  `json:"tradingEnvironment"`
	AccountID          string  `json:"accountId"`
	Market             *string `json:"market"`
	Symbol             *string `json:"symbol"`
	HardStopScope      string  `json:"hardStopScope"`
	OperatorID         string  `json:"operatorId"`
	Reason             string  `json:"reason"`
	ActivatedAt        string  `json:"activatedAt"`
	UpdatedAt          string  `json:"updatedAt"`
}

type RealTradeControlEvent struct {
	ID                 string   `json:"id"`
	EventType          string   `json:"eventType"`
	Action             string   `json:"action"`
	BrokerID           string   `json:"brokerId"`
	Operation          *string  `json:"operation"`
	TradingEnvironment *string  `json:"tradingEnvironment"`
	AccountID          *string  `json:"accountId"`
	Market             *string  `json:"market"`
	Symbol             *string  `json:"symbol"`
	OrderID            *string  `json:"orderId"`
	Quantity           *float64 `json:"quantity"`
	Price              *float64 `json:"price"`
	KillSwitchSource   *string  `json:"killSwitchSource"`
	HardStopScope      *string  `json:"hardStopScope"`
	OperatorID         *string  `json:"operatorId"`
	Reason             *string  `json:"reason"`
	ErrorCode          *string  `json:"errorCode"`
	HardStopID         *string  `json:"hardStopId"`
	ActivatedAt        *string  `json:"activatedAt"`
	CreatedAt          string   `json:"createdAt"`
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

func (p *RealTradeControlPlane) Snapshot() map[string]any {
	return NewStaticPreTradeRiskGateway(func() PreTradeRiskConfig {
		return p.config()
	}).Snapshot()
}

func (p *RealTradeControlPlane) ActivateKillSwitch(_ context.Context, command RealTradeKillSwitchCommand) (map[string]any, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.availabilityErrorLocked(); err != nil {
		return nil, err
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
	source := "CONTROL_PLANE"
	p.appendEventLocked(RealTradeControlEvent{
		ID:                 nextRealTradeControlID("rtks-event"),
		EventType:          "activated",
		Action:             "KILL_SWITCH_ACTIVATE",
		BrokerID:           "futu",
		TradingEnvironment: new(env),
		KillSwitchSource:   &source,
		OperatorID:         new(entry.OperatorID),
		Reason:             nullableString(entry.Reason),
		ActivatedAt:        new(entry.ActivatedAt),
		CreatedAt:          now,
	})
	if err := p.persistLocked(); err != nil {
		p.state = previousState
		return nil, err
	}
	return p.snapshotLocked(), nil
}

func (p *RealTradeControlPlane) ReleaseKillSwitch(_ context.Context, command RealTradeKillSwitchCommand) (map[string]any, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.availabilityErrorLocked(); err != nil {
		return nil, err
	}
	previousState := cloneRealTradeControlState(p.state)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	previous := p.state.KillSwitch
	p.state.KillSwitch = nil
	source := "CONTROL_PLANE"
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
		BrokerID:           "futu",
		TradingEnvironment: new(env),
		KillSwitchSource:   &source,
		OperatorID:         new(operatorID),
		Reason:             nullableString(reason),
		ActivatedAt:        activatedAt,
		CreatedAt:          now,
	})
	if err := p.persistLocked(); err != nil {
		p.state = previousState
		return nil, err
	}
	return p.snapshotLocked(), nil
}

func (p *RealTradeControlPlane) ActivateHardStop(_ context.Context, command RealTradeHardStopCommand) (map[string]any, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.availabilityErrorLocked(); err != nil {
		return nil, err
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
		return nil, err
	}
	return p.snapshotLocked(), nil
}

func (p *RealTradeControlPlane) ReleaseHardStop(_ context.Context, id string, command RealTradeHardStopCommand) (map[string]any, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.availabilityErrorLocked(); err != nil {
		return nil, err
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
		return nil, fmt.Errorf("real-trade hard stop not found")
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
		return nil, err
	}
	return p.snapshotLocked(), nil
}

func (p *RealTradeControlPlane) config() PreTradeRiskConfig {
	env := NewEnvPreTradeRiskGateway().config()
	p.mu.Lock()
	defer p.mu.Unlock()
	env.ControlPlaneKillSwitch = p.state.KillSwitch != nil
	if p.unavailableErr != nil {
		env.ControlPlaneKillSwitch = true
		env.ControlPlaneError = p.unavailableErr.Error()
	}
	env.ControlPlaneHardStops = append([]RealTradeHardStopEntry(nil), p.state.HardStops...)
	env.KillSwitchEntry = cloneKillSwitchEntry(p.state.KillSwitch)
	env.ControlPlaneEvents = append([]RealTradeControlEvent(nil), p.state.Events...)
	return env
}

func (p *RealTradeControlPlane) snapshotLocked() map[string]any {
	config := NewEnvPreTradeRiskGateway().config()
	config.ControlPlaneKillSwitch = p.state.KillSwitch != nil
	if p.unavailableErr != nil {
		config.ControlPlaneKillSwitch = true
		config.ControlPlaneError = p.unavailableErr.Error()
	}
	config.ControlPlaneHardStops = append([]RealTradeHardStopEntry(nil), p.state.HardStops...)
	config.KillSwitchEntry = cloneKillSwitchEntry(p.state.KillSwitch)
	config.ControlPlaneEvents = append([]RealTradeControlEvent(nil), p.state.Events...)
	return NewStaticPreTradeRiskGateway(func() PreTradeRiskConfig { return config }).Snapshot()
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
	if matched, ok := decision.Snapshot["matchedHardStop"].(RealTradeHardStopEntry); ok {
		event.HardStopScope = new(matched.HardStopScope)
		event.HardStopID = new(matched.ID)
		event.ActivatedAt = new(matched.ActivatedAt)
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
		KillSwitch: cloneKillSwitchEntry(state.KillSwitch),
		HardStops:  append([]RealTradeHardStopEntry(nil), state.HardStops...),
		Events:     append([]RealTradeControlEvent(nil), state.Events...),
	}
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
		return "futu"
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
