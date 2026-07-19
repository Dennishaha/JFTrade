package trading

import "strings"

// RealTradeRiskSnapshot 是实盘风控（real-trade）控制面的完整状态快照。
// JSON 键与历史 map 响应逐键一致：指针字段为空时序列化为 null，
// 切片字段始终非 nil（序列化为 [] 而不是 null）。
// 该结构同时服务三个场景：HTTP 状态响应、下单前风控评估（EvaluatePlaceOrder）
// 与熔断/硬停止命令的写后响应。
type RealTradeRiskSnapshot struct {
	RealTradingEnabled                bool                       `json:"realTradingEnabled" binding:"required"`
	KillSwitchActive                  bool                       `json:"killSwitchActive" binding:"required"`
	KillSwitchSource                  *string                    `json:"killSwitchSource"`
	RuntimeKillSwitchActive           bool                       `json:"runtimeKillSwitchActive" binding:"required"`
	ControlPlaneAvailable             bool                       `json:"controlPlaneAvailable" binding:"required"`
	ControlPlaneError                 *string                    `json:"controlPlaneError"`
	KillSwitchEntry                   *RealTradeKillSwitchEntry  `json:"killSwitchEntry"`
	KillSwitchEvents                  []RealTradeControlEvent    `json:"killSwitchEvents" binding:"required"`
	BlockedOperations                 []string                   `json:"blockedOperations" binding:"required"`
	AllowsCancel                      bool                       `json:"allowsCancel" binding:"required"`
	HardStopsActive                   bool                       `json:"hardStopsActive" binding:"required"`
	HardStopEntries                   []RealTradeHardStopEntry   `json:"hardStopEntries" binding:"required"`
	HardStopEvents                    []RealTradeControlEvent    `json:"hardStopEvents" binding:"required"`
	RiskEnabled                       bool                       `json:"riskEnabled" binding:"required"`
	RuntimeRiskConfigured             bool                       `json:"runtimeRiskConfigured" binding:"required"`
	RuntimeConfiguredMaxOrderQuantity *float64                   `json:"runtimeConfiguredMaxOrderQuantity"`
	RuntimeConfiguredMaxOrderNotional *float64                   `json:"runtimeConfiguredMaxOrderNotional"`
	EffectiveMaxOrderQuantity         *float64                   `json:"effectiveMaxOrderQuantity"`
	EffectiveMaxOrderNotional         *float64                   `json:"effectiveMaxOrderNotional"`
	RiskEntry                         *RealTradeRuntimeRiskEntry `json:"riskEntry"`
	RiskEvents                        []RealTradeControlEvent    `json:"riskEvents" binding:"required"`
	// MatchedHardStop 仅在下单评估命中硬停止时填充，HTTP 状态响应中缺省。
	MatchedHardStop *RealTradeHardStopEntry `json:"matchedHardStop,omitempty"`
}

// realTradeRiskSnapshotFromConfig 由风控配置构造完整快照，是
// StaticPreTradeRiskGateway.Snapshot 与 RealTradeControlPlane 快照的唯一来源。
func realTradeRiskSnapshotFromConfig(config PreTradeRiskConfig) RealTradeRiskSnapshot {
	maxQty := positiveFloat(config.RuntimeMaxOrderQty)
	maxNotional := positiveFloat(config.RuntimeMaxOrderNotional)
	hardStops := append([]RealTradeHardStopEntry{}, config.RuntimeHardStops...)
	events := append([]RealTradeControlEvent{}, config.RuntimeEvents...)
	return RealTradeRiskSnapshot{
		RealTradingEnabled:                config.RealTradingEnabled,
		KillSwitchActive:                  config.RuntimeKillSwitch,
		KillSwitchSource:                  runtimeSwitchSource(config),
		RuntimeKillSwitchActive:           config.RuntimeKillSwitch,
		ControlPlaneAvailable:             strings.TrimSpace(config.RuntimeError) == "",
		ControlPlaneError:                 nullableString(config.RuntimeError),
		KillSwitchEntry:                   cloneKillSwitchEntry(config.KillSwitchEntry),
		KillSwitchEvents:                  filterRealTradeControlEvents(events, "KILL_SWITCH_"),
		BlockedOperations:                 []string{"PLACE", "MODIFY"},
		AllowsCancel:                      true,
		HardStopsActive:                   len(hardStops) > 0,
		HardStopEntries:                   hardStops,
		HardStopEvents:                    filterRealTradeControlEvents(events, "HARD_STOP_"),
		RiskEnabled:                       maxQty != nil || maxNotional != nil,
		RuntimeRiskConfigured:             config.RuntimeRiskEntry != nil,
		RuntimeConfiguredMaxOrderQuantity: maxQty,
		RuntimeConfiguredMaxOrderNotional: maxNotional,
		EffectiveMaxOrderQuantity:         maxQty,
		EffectiveMaxOrderNotional:         maxNotional,
		RiskEntry:                         cloneRuntimeRiskEntry(config.RuntimeRiskEntry),
		RiskEvents:                        filterRealTradeRiskEvents(events),
	}
}
