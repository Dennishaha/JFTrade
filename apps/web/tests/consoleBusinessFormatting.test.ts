import { describe, expect, it } from "vitest";

import * as formatting from "../src/composables/consoleDataFormatting";

describe("console business formatting", () => {
  it("maps broker, order and connectivity enums while preserving unknown backend values", () => {
    expect(formatting.formatTradingEnvironment("REAL")).toBe("实盘");
    expect(formatting.formatTradingEnvironment(null)).toBe("未设置");
    expect(formatting.formatMarketLabel("hk")).toBe("港股");
    expect(formatting.formatMarketLabel("MOON")).toBe("MOON");
    expect(formatting.formatConnectivityLabel("connected")).toBe("已连接");
    expect(formatting.formatConnectivityLabel("")).toBe("未知");
    expect(formatting.formatConnectivityLabel("custom")).toBe("custom");
    expect(formatting.formatNotificationLevelLabel("warning")).toBe("警告");
    expect(formatting.formatOrderSideLabel("BUY")).toBe("买入");
    expect(formatting.formatOrderTypeLabel("STOP_LIMIT")).toBe("止损限价");
    expect(formatting.formatTimeInForceLabel("IOC")).toBe("立即成交剩余取消");
    expect(formatting.formatExecutionOrderStatusLabel("PARTIAL_FILLED")).toBe("部分成交");
    expect(formatting.formatExecutionOrderSourceLabel("SYSTEM")).toBe("本系统");
    expect(formatting.formatExecutionOrderSourceLabel("BROKER", "broker.fill")).toBe("券商成交");
    expect(formatting.formatExecutionOrderSourceLabel("BROKER", "custom.detail")).toBe("custom.detail");
    expect(formatting.isFinalExecutionOrderStatus("filled")).toBe(true);
    expect(formatting.isFinalExecutionOrderStatus("submitted")).toBe(false);
    expect(formatting.formatExecutionEventTypeLabel("BROKER_FILL_RECEIVED")).toBe("收到券商成交回报");
    expect(formatting.formatAccountTypeLabel("MARGIN")).toBe("保证金账户");
    expect(formatting.formatCashFlowTypeLabel("DIVIDEND")).toBe("股息");
    expect(formatting.formatBooleanLabel(null)).toBe("未知");
    expect(formatting.formatBooleanLabel(true, "开", "关")).toBe("开");
    expect(formatting.formatBooleanLabel(false, "开", "关")).toBe("关");
    expect(formatting.formatGenericStatusLabel("RUNNING")).toBe("运行中");
    expect(formatting.formatRealTradeOperationLabel("MODIFY_ORDER")).toBe("改单");
    expect(formatting.formatRealTradeEventTypeLabel("ACTIVATED")).toBe("已激活");
    expect(formatting.formatWorkerBrokerSubscriptionStatusLabel("RETRYING")).toBe("重试中");
    expect(formatting.formatWorkerBrokerActionLabel("SYNC-ORDERS")).toBe("同步订单");
    expect(formatting.formatWorkerBrokerBackoffSourceLabel("DISCONNECTED")).toBe("连接中断");
    expect(formatting.formatMarketDataChannelLabel("ORDERBOOK")).toBe("盘口");
  });

  it("formats OpenD and scoped real-trade controls", () => {
    expect(formatting.formatFutuProgramStatusLabel(null)).toBe("暂无");
    expect(formatting.formatFutuProgramStatusLabel("PROGRAMSTATUSTYPE_READY")).toBe("已就绪");
    expect(formatting.formatFutuProgramStatusLabel("PROGRAMSTATUSTYPE_LOGINFAILED:bad password"))
      .toBe("登录失败：bad password");
    expect(formatting.formatFutuProgramStatusLabel("CUSTOM:detail")).toBe("CUSTOM：detail");

    const account = { market: null, symbol: null };
    const market = { market: "US", symbol: null };
    const symbol = { market: "US", symbol: "AAPL" };
    expect(formatting.formatRealTradeHardStopScope(account)).toBe("账户");
    expect(formatting.formatRealTradeHardStopScope(market)).toBe("市场 / 美股");
    expect(formatting.formatRealTradeHardStopScope(symbol)).toBe("标的 / 美股 / AAPL");
    expect(formatting.resolveRealTradeHardStopScopeTagType(account)).toBe("info");
    expect(formatting.resolveRealTradeHardStopScopeTagType(market)).toBe("warning");
    expect(formatting.resolveRealTradeHardStopScopeTagType(symbol)).toBe("danger");
    expect(formatting.formatRealTradeHardStopScope({ ...account, hardStopScope: "SYMBOL" })).toContain("标的");

    expect(formatting.formatRealTradeKillSwitchSource("RUNTIME")).toBe("运行时");
    expect(formatting.formatRealTradeKillSwitchSource(null)).toBe("未启用");

    for (const [event, tag] of [
      ["released", "success"],
      ["activated", "warning"],
      ["rejected", "danger"],
    ] as const) {
      expect(formatting.resolveRealTradeKillSwitchEventTagType(event)).toBe(tag);
      expect(formatting.resolveRealTradeRiskEventTagType(event)).toBe(tag);
    }
    expect(formatting.resolveRealTradeRiskEventTagType("updated")).toBe("warning");
    expect(formatting.resolveRealTradeRiskEventTagType("disabled")).toBe("success");
    expect(formatting.resolveWorkerBrokerSubscriptionTagType("active")).toBe("success");
    expect(formatting.resolveWorkerBrokerSubscriptionTagType("retrying")).toBe("warning");
    expect(formatting.resolveWorkerBrokerSubscriptionTagType("inactive")).toBe("info");
  });

  it("formats timestamps, durations, error context and approval decisions", () => {
    expect(formatting.formatDateTime(null)).toBe("暂无");
    expect(formatting.formatDateTime("invalid")).toBe("invalid");
    expect(formatting.formatDateTime("2026-07-01T00:00:00.000Z")).not.toBe("暂无");
    expect(formatting.formatStrategyRuntimeStatus("RUNNING")).toBe("运行中");
    expect(formatting.formatStrategyRuntimeStatus("PAUSED")).toBe("已暂停");
    expect(formatting.formatStrategyRuntimeStatus("STOPPED")).toBe("已停止");
    expect(formatting.formatDurationMs(null)).toBe("暂无");
    expect(formatting.formatDurationMs(999)).toBe("999毫秒");
    expect(formatting.formatDurationMs(2_000)).toBe("2秒");
    expect(formatting.formatDurationMs(120_000)).toBe("2分");
    expect(formatting.formatDurationMs(7_200_000)).toBe("2小时");
    expect(formatting.formatWorkerBrokerErrorContext({ summary: "socket closed" } as never, null)).toBe("socket closed");
    expect(formatting.formatWorkerBrokerErrorContext(null, "fallback")).toBe("fallback");
    expect(formatting.formatWorkerBrokerErrorContext(null, null)).toBe("暂无错误上下文");
    expect(formatting.resolveRealTradeApprovalDecisionTagType("approved")).toBe("success");
    expect(formatting.resolveRealTradeApprovalDecisionTagType("rejected")).toBe("danger");
    expect(formatting.formatApprovalDecisionLabel("approved")).toBe("已批准");
    expect(formatting.formatApprovalDecisionLabel("rejected")).toBe("已拒绝");
  });
});
