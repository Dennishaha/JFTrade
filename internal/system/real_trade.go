package system

import "github.com/jftrade/jftrade-main/internal/trading"

// 本文件定义 /system/real-trade-* 读取端点的响应 DTO。
// JSON 键与历史 map 响应逐键一致：指针字段为空时序列化为 null，
// 切片字段始终非 nil（序列化为 [] 而不是 null）。
// 所有 DTO 由 trading.RealTradeRiskSnapshot 构造，保证状态来源唯一。

// RealTradeApprovalsResponse GET /system/real-trade-approvals 响应。
// 审批工作流未实现，entries 恒为空数组。
type RealTradeApprovalsResponse struct {
	RealTradingEnabled        bool                    `json:"realTradingEnabled" binding:"required"`
	RequiredConfirmationText  string                  `json:"requiredConfirmationText" binding:"required"`
	MaxApprovalAgeMs          int64                   `json:"maxApprovalAgeMs" binding:"required"`
	ApprovalWorkflowAvailable bool                    `json:"approvalWorkflowAvailable" binding:"required"`
	ApprovalWorkflowStatus    string                  `json:"approvalWorkflowStatus" binding:"required"`
	ApprovalWorkflowMessage   string                  `json:"approvalWorkflowMessage" binding:"required"`
	ApprovalPolicy            RealTradeApprovalPolicy `json:"approvalPolicy" binding:"required"`
	Entries                   []any                   `json:"entries" binding:"required"`
}

// RealTradeApprovalPolicy 大额订单审批策略（当前未启用）。
type RealTradeApprovalPolicy struct {
	ApproverAllowlistEnabled  bool     `json:"approverAllowlistEnabled" binding:"required"`
	ApproverCount             int      `json:"approverCount" binding:"required"`
	LargeOrderNotional        *float64 `json:"largeOrderNotional"`
	ApprovalWorkflowAvailable bool     `json:"approvalWorkflowAvailable" binding:"required"`
	ApprovalMode              string   `json:"approvalMode" binding:"required"`
}

// RealTradeHardStopsResponse GET /system/real-trade-hard-stops 响应。
type RealTradeHardStopsResponse struct {
	BlockedOperations []string                         `json:"blockedOperations" binding:"required"`
	AllowsCancel      bool                             `json:"allowsCancel" binding:"required"`
	Entries           []trading.RealTradeHardStopEntry `json:"entries" binding:"required"`
}

// RealTradeHardStopEventsResponse GET /system/real-trade-hard-stop-events 响应。
type RealTradeHardStopEventsResponse struct {
	RealTradingEnabled bool                            `json:"realTradingEnabled" binding:"required"`
	BlockedOperations  []string                        `json:"blockedOperations" binding:"required"`
	AllowsCancel       bool                            `json:"allowsCancel" binding:"required"`
	Entries            []trading.RealTradeControlEvent `json:"entries" binding:"required"`
}

// RealTradeKillSwitchStateResponse GET /system/real-trade-kill-switch 响应。
type RealTradeKillSwitchStateResponse struct {
	RealTradingEnabled bool                              `json:"realTradingEnabled" binding:"required"`
	KillSwitchActive   bool                              `json:"killSwitchActive" binding:"required"`
	KillSwitchSource   *string                           `json:"killSwitchSource"`
	RuntimeActive      bool                              `json:"runtimeActive" binding:"required"`
	BlockedOperations  []string                          `json:"blockedOperations" binding:"required"`
	AllowsCancel       bool                              `json:"allowsCancel" binding:"required"`
	Entry              *trading.RealTradeKillSwitchEntry `json:"entry"`
}

// RealTradeKillSwitchEventsResponse GET /system/real-trade-kill-switch-events 响应。
type RealTradeKillSwitchEventsResponse struct {
	RealTradingEnabled bool                            `json:"realTradingEnabled" binding:"required"`
	KillSwitchActive   bool                            `json:"killSwitchActive" binding:"required"`
	RuntimeActive      bool                            `json:"runtimeActive" binding:"required"`
	BlockedOperations  []string                        `json:"blockedOperations" binding:"required"`
	AllowsCancel       bool                            `json:"allowsCancel" binding:"required"`
	Entries            []trading.RealTradeControlEvent `json:"entries" binding:"required"`
}

// RealTradeRiskLimitsResponse GET /system/real-trade-risk-limits 响应。
type RealTradeRiskLimitsResponse struct {
	RealTradingEnabled                bool                               `json:"realTradingEnabled" binding:"required"`
	RiskEnabled                       bool                               `json:"riskEnabled" binding:"required"`
	RuntimeRiskConfigured             bool                               `json:"runtimeRiskConfigured" binding:"required"`
	RuntimeConfiguredMaxOrderQuantity *float64                           `json:"runtimeConfiguredMaxOrderQuantity"`
	RuntimeConfiguredMaxOrderNotional *float64                           `json:"runtimeConfiguredMaxOrderNotional"`
	EffectiveMaxOrderQuantity         *float64                           `json:"effectiveMaxOrderQuantity"`
	EffectiveMaxOrderNotional         *float64                           `json:"effectiveMaxOrderNotional"`
	Entry                             *trading.RealTradeRuntimeRiskEntry `json:"entry"`
}

// RealTradeRiskEventsResponse GET /system/real-trade-risk-events 响应。
type RealTradeRiskEventsResponse struct {
	RealTradingEnabled                bool                            `json:"realTradingEnabled" binding:"required"`
	RiskEnabled                       bool                            `json:"riskEnabled" binding:"required"`
	RuntimeRiskConfigured             bool                            `json:"runtimeRiskConfigured" binding:"required"`
	RuntimeConfiguredMaxOrderQuantity *float64                        `json:"runtimeConfiguredMaxOrderQuantity"`
	RuntimeConfiguredMaxOrderNotional *float64                        `json:"runtimeConfiguredMaxOrderNotional"`
	EffectiveMaxOrderQuantity         *float64                        `json:"effectiveMaxOrderQuantity"`
	EffectiveMaxOrderNotional         *float64                        `json:"effectiveMaxOrderNotional"`
	MaxOrderQuantity                  *float64                        `json:"maxOrderQuantity"`
	MaxOrderNotional                  *float64                        `json:"maxOrderNotional"`
	Entries                           []trading.RealTradeControlEvent `json:"entries" binding:"required"`
}

func realTradeApprovalsResponse(snapshot *trading.RealTradeRiskSnapshot) RealTradeApprovalsResponse {
	return RealTradeApprovalsResponse{
		RealTradingEnabled:        snapshot.RealTradingEnabled,
		RequiredConfirmationText:  "ENABLE_REAL_TRADING",
		MaxApprovalAgeMs:          5 * 60 * 1000,
		ApprovalWorkflowAvailable: false,
		ApprovalWorkflowStatus:    "not_configured",
		ApprovalWorkflowMessage:   "real-trade approval workflow is not configured; runtime risk limits are enforced before broker submission.",
		ApprovalPolicy: RealTradeApprovalPolicy{
			ApproverAllowlistEnabled:  false,
			ApproverCount:             0,
			LargeOrderNotional:        nil,
			ApprovalWorkflowAvailable: false,
			ApprovalMode:              "none",
		},
		Entries: []any{},
	}
}

func realTradeHardStopsResponse(snapshot *trading.RealTradeRiskSnapshot) RealTradeHardStopsResponse {
	return RealTradeHardStopsResponse{
		BlockedOperations: realTradeBlockedOperations(),
		AllowsCancel:      true,
		Entries:           realTradeHardStopsOrEmpty(snapshot.HardStopEntries),
	}
}

func realTradeHardStopEventsResponse(snapshot *trading.RealTradeRiskSnapshot) RealTradeHardStopEventsResponse {
	return RealTradeHardStopEventsResponse{
		RealTradingEnabled: snapshot.RealTradingEnabled,
		BlockedOperations:  realTradeBlockedOperations(),
		AllowsCancel:       true,
		Entries:            realTradeControlEventsOrEmpty(snapshot.HardStopEvents),
	}
}

func realTradeKillSwitchStateResponse(snapshot *trading.RealTradeRiskSnapshot) RealTradeKillSwitchStateResponse {
	return RealTradeKillSwitchStateResponse{
		RealTradingEnabled: snapshot.RealTradingEnabled,
		KillSwitchActive:   snapshot.KillSwitchActive,
		KillSwitchSource:   snapshot.KillSwitchSource,
		RuntimeActive:      snapshot.RuntimeKillSwitchActive,
		BlockedOperations:  realTradeBlockedOperations(),
		AllowsCancel:       true,
		Entry:              snapshot.KillSwitchEntry,
	}
}

func realTradeKillSwitchEventsResponse(snapshot *trading.RealTradeRiskSnapshot) RealTradeKillSwitchEventsResponse {
	return RealTradeKillSwitchEventsResponse{
		RealTradingEnabled: snapshot.RealTradingEnabled,
		KillSwitchActive:   snapshot.KillSwitchActive,
		RuntimeActive:      snapshot.RuntimeKillSwitchActive,
		BlockedOperations:  realTradeBlockedOperations(),
		AllowsCancel:       true,
		Entries:            realTradeControlEventsOrEmpty(snapshot.KillSwitchEvents),
	}
}

func realTradeRiskLimitsResponse(snapshot *trading.RealTradeRiskSnapshot) RealTradeRiskLimitsResponse {
	return RealTradeRiskLimitsResponse{
		RealTradingEnabled:                snapshot.RealTradingEnabled,
		RiskEnabled:                       snapshot.RiskEnabled,
		RuntimeRiskConfigured:             snapshot.RuntimeRiskConfigured,
		RuntimeConfiguredMaxOrderQuantity: snapshot.RuntimeConfiguredMaxOrderQuantity,
		RuntimeConfiguredMaxOrderNotional: snapshot.RuntimeConfiguredMaxOrderNotional,
		EffectiveMaxOrderQuantity:         snapshot.EffectiveMaxOrderQuantity,
		EffectiveMaxOrderNotional:         snapshot.EffectiveMaxOrderNotional,
		Entry:                             snapshot.RiskEntry,
	}
}

func realTradeRiskEventsResponse(snapshot *trading.RealTradeRiskSnapshot) RealTradeRiskEventsResponse {
	return RealTradeRiskEventsResponse{
		RealTradingEnabled:                snapshot.RealTradingEnabled,
		RiskEnabled:                       snapshot.RiskEnabled,
		RuntimeRiskConfigured:             snapshot.RuntimeRiskConfigured,
		RuntimeConfiguredMaxOrderQuantity: snapshot.RuntimeConfiguredMaxOrderQuantity,
		RuntimeConfiguredMaxOrderNotional: snapshot.RuntimeConfiguredMaxOrderNotional,
		EffectiveMaxOrderQuantity:         snapshot.EffectiveMaxOrderQuantity,
		EffectiveMaxOrderNotional:         snapshot.EffectiveMaxOrderNotional,
		MaxOrderQuantity:                  snapshot.EffectiveMaxOrderQuantity,
		MaxOrderNotional:                  snapshot.EffectiveMaxOrderNotional,
		Entries:                           realTradeControlEventsOrEmpty(snapshot.RiskEvents),
	}
}

func realTradeBlockedOperations() []string {
	return []string{"PLACE", "MODIFY"}
}

func realTradeHardStopsOrEmpty(entries []trading.RealTradeHardStopEntry) []trading.RealTradeHardStopEntry {
	if entries == nil {
		return []trading.RealTradeHardStopEntry{}
	}
	return entries
}

func realTradeControlEventsOrEmpty(events []trading.RealTradeControlEvent) []trading.RealTradeControlEvent {
	if events == nil {
		return []trading.RealTradeControlEvent{}
	}
	return events
}
