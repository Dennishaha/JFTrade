import type {
  RealTradeHardStopEventsResponse,
  RealTradeHardStopsResponse,
  RealTradeKillSwitchEventsResponse,
  RealTradeKillSwitchStateResponse,
  RealTradeRiskEventsResponse,
  RealTradeRiskStateResponse,
} from "@/contracts";
import { formatDateTime as formatSharedDateTime } from "../utils/dateTime";

type RealTradeHardStopScope = "ACCOUNT" | "MARKET" | "SYMBOL";

const MARKET_LABELS: Record<string, string> = {
  HK: "港股",
  US: "美股",
  CN: "沪深",
  SH: "上交所",
  SZ: "深交所",
  SG: "新加坡",
  JP: "日本",
  AU: "澳洲",
  MY: "马来西亚",
  CA: "加拿大",
  CRYPTO: "加密资产",
};

const TRADING_ENVIRONMENT_LABELS: Record<string, string> = {
  REAL: "实盘",
  PAPER: "仿真",
  SIMULATE: "模拟",
  SIM: "模拟",
};

const CONNECTIVITY_LABELS: Record<string, string> = {
  connected: "已连接",
  degraded: "部分可用",
  disconnected: "未连接",
  connecting: "连接中",
  unsupported: "不支持",
  error: "错误",
  idle: "空闲",
  ok: "正常",
};

const ORDER_SIDE_LABELS: Record<string, string> = {
  BUY: "买入",
  SELL: "卖出",
};

const ORDER_TYPE_LABELS: Record<string, string> = {
  LIMIT: "限价",
  MARKET: "市价",
  STOP: "止损",
  STOP_LIMIT: "止损限价",
  NORMAL: "普通",
};

const TIME_IN_FORCE_LABELS: Record<string, string> = {
  DAY: "当日有效",
  GTC: "撤单前有效",
  IOC: "立即成交剩余取消",
  FOK: "全部成交否则取消",
};

const EXECUTION_ORDER_STATUS_LABELS: Record<string, string> = {
  NEW: "已下单",
  CREATED: "已创建",
  PENDING: "待处理",
  SUBMITTING: "提交中",
  SUBMITTED: "已提交",
  BROKER_ACCEPTED: "券商已接受",
  PARTIAL_FILLED: "部分成交",
  PARTIALLY_FILLED: "部分成交",
  FILLED: "已成交",
  CANCELING: "撤单中",
  CANCEL_REQUESTED: "撤单已提交",
  CANCELLED: "已撤单",
  CANCELED: "已撤单",
  REJECTED: "已拒绝",
  FAILED: "失败",
  EXPIRED: "已过期",
  DELETED: "已删除",
  DISABLED: "已停用",
  PRECHECK_REJECTED: "风控拒绝",
  UNKNOWN: "状态待确认",
  MODIFYING: "改单中",
  PENDING_CANCEL: "待撤销",
  PENDING_MODIFY: "待改单",
};

const EXECUTION_EVENT_TYPE_LABELS: Record<string, string> = {
  COMMAND_PLACE_ACCEPTED: "下单已受理",
  COMMAND_PLACE_REJECTED: "下单被拒绝",
  COMMAND_CANCEL_ACCEPTED: "撤单已受理",
  COMMAND_CANCEL_REJECTED: "撤单被拒绝",
  COMMAND_MODIFY_ACCEPTED: "改单已受理",
  COMMAND_MODIFY_REJECTED: "改单被拒绝",
  BROKER_SYNC: "券商同步",
  BROKER_SYNC_DISCOVERED: "券商同步首次发现订单",
  BROKER_SYNC_UPDATED: "券商同步更新订单",
  BROKER_PUSH: "券商推送",
  BROKER_PUSH_DISCOVERED: "券商推送首次发现订单",
  BROKER_PUSH_ORDER: "券商推送更新订单",
  BROKER_FILL_RECEIVED: "收到券商成交回报",
};

const EXECUTION_ORDER_SOURCE_LABELS: Record<string, string> = {
  SYSTEM: "本系统",
  BROKER: "券商",
};

const EXECUTION_ORDER_SOURCE_DETAIL_LABELS: Record<string, string> = {
  "COMMAND.PLACE": "本系统下单",
  "BROKER.CURRENT": "券商当前订单",
  "BROKER.HISTORY": "券商历史订单",
  "BROKER.PUSH": "券商推送",
  "BROKER.FILL": "券商成交",
};

const ACCOUNT_TYPE_LABELS: Record<string, string> = {
  CASH: "现金账户",
  MARGIN: "保证金账户",
  STOCK: "股票账户",
  FUTURES: "期货账户",
  OPTIONS: "期权账户",
};

const CASH_FLOW_TYPE_LABELS: Record<string, string> = {
  BUY_SETTLEMENT: "买入结算",
  SELL_SETTLEMENT: "卖出结算",
  DIVIDEND: "股息",
  INTEREST: "利息",
  FEE: "费用",
};

const STRATEGY_RUNTIME_STATUS_LABELS: Record<string, string> = {
  RUNNING: "运行中",
  PAUSED: "已暂停",
  STOPPED: "已停止",
};

const BOOLEAN_LABELS: Record<string, string> = {
  OK: "正常",
  YES: "是",
  NO: "否",
  ACTIVE: "已激活",
  INACTIVE: "未激活",
  ENABLED: "已启用",
  DISABLED: "已禁用",
  GATED: "受限",
  ENFORCED: "已生效",
  CLEAR: "正常",
  ON: "开启",
  OFF: "关闭",
  READY: "就绪",
  TODO: "待办",
  PENDING: "待处理",
  PENDING_APPROVAL: "等待审批",
  IN_PROGRESS: "进行中",
  FOUND: "已命中",
  EMPTY: "空",
  LIVE: "实时",
  NONE: "无",
  DONE: "已完成",
  COMPLETED: "已完成",
  CANCELLED: "已取消",
  CANCELED: "已取消",
  FAILED: "失败",
  RUNNING: "运行中",
  QUEUED: "排队中",
  RETRYING: "重试中",
  APPROVED: "已批准",
  REJECTED: "已拒绝",
  ALLOWED: "允许",
  BLOCKED: "已阻断",
  DISCONNECTED: "未连接",
  CONNECTED: "已连接",
};

const REAL_TRADE_EVENT_TYPE_LABELS: Record<string, string> = {
  ACTIVATED: "已激活",
  RELEASED: "已解除",
  REJECTED: "已拒绝",
  UPDATED: "已更新",
  DISABLED: "已关闭",
};

const FUTU_PROGRAM_STATUS_LABELS: Record<string, string> = {
  UNAVAILABLE: "不可用",
  PROGRAMSTATUSTYPE_NONE: "暂无",
  PROGRAMSTATUSTYPE_LOADED: "已加载",
  PROGRAMSTATUSTYPE_LOGING: "登录中",
  PROGRAMSTATUSTYPE_NEEDPICVERIFYCODE: "需要图形验证码",
  PROGRAMSTATUSTYPE_NEEDPHONEVERIFYCODE: "需要手机验证码",
  PROGRAMSTATUSTYPE_LOGINFAILED: "登录失败",
  PROGRAMSTATUSTYPE_FORCEUPDATE: "需要升级客户端",
  PROGRAMSTATUSTYPE_NESSARYDATAPREPARING: "正在准备必要数据",
  PROGRAMSTATUSTYPE_NESSARYDATAMISSING: "缺少必要数据",
  PROGRAMSTATUSTYPE_UNAGREEDISCLAIMER: "未同意免责声明",
  PROGRAMSTATUSTYPE_READY: "已就绪",
  PROGRAMSTATUSTYPE_FORCELOGOUT: "已被强制登出",
  PROGRAMSTATUSTYPE_DISCLAIMERPULLFAILED: "拉取免责声明失败",
};

const FINAL_ORDER_STATUSES = new Set([
  "FILLED",
  "CANCELLED",
  "CANCELED",
  "REJECTED",
  "FAILED",
  "EXPIRED",
  "DELETED",
  "DISABLED",
]);

function normalizeEnumValue(value: string | null | undefined): string {
  return (value ?? "").trim().toUpperCase();
}

function resolveLabel(
  value: string | null | undefined,
  labels: Record<string, string>,
  fallback = "未设置",
): string {
  if (value == null || value.trim() === "") {
    return fallback;
  }

  const normalized = normalizeEnumValue(value);
  return labels[normalized] ?? value;
}

export function formatTradingEnvironment(
  env: string | null | undefined,
): string {
  return resolveLabel(env, TRADING_ENVIRONMENT_LABELS, "未设置");
}

export function formatMarketLabel(market: string | null | undefined): string {
  return resolveLabel(market, MARKET_LABELS, "未设置");
}

export function formatConnectivityLabel(
  connectivity: string | null | undefined,
): string {
  if (connectivity == null || connectivity.trim() === "") {
    return "未知";
  }

  return CONNECTIVITY_LABELS[connectivity.trim().toLowerCase()] ?? connectivity;
}

export function formatNotificationLevelLabel(
  level: string | null | undefined,
): string {
  return resolveLabel(level, {
    INFO: "提示",
    SUCCESS: "成功",
    WARN: "警告",
    WARNING: "警告",
    ERROR: "错误",
  });
}

export function formatOrderSideLabel(side: string | null | undefined): string {
  return resolveLabel(side, ORDER_SIDE_LABELS);
}

export function formatOrderTypeLabel(
  orderType: string | null | undefined,
): string {
  return resolveLabel(orderType, ORDER_TYPE_LABELS);
}

export function formatTimeInForceLabel(
  timeInForce: string | null | undefined,
): string {
  return resolveLabel(timeInForce, TIME_IN_FORCE_LABELS);
}

export function formatExecutionOrderStatusLabel(
  status: string | null | undefined,
): string {
  return resolveLabel(status, EXECUTION_ORDER_STATUS_LABELS);
}

export function formatExecutionOrderSourceLabel(
  source: string | null | undefined,
  sourceDetail?: string | null,
): string {
  if (sourceDetail != null && sourceDetail.trim() !== "") {
    return (
      EXECUTION_ORDER_SOURCE_DETAIL_LABELS[sourceDetail.trim().toUpperCase()] ??
      sourceDetail
    );
  }
  return resolveLabel(source, EXECUTION_ORDER_SOURCE_LABELS);
}

export function isFinalExecutionOrderStatus(
  status: string | null | undefined,
): boolean {
  return FINAL_ORDER_STATUSES.has(normalizeEnumValue(status));
}

export function formatExecutionEventTypeLabel(
  eventType: string | null | undefined,
): string {
  return resolveLabel(eventType, EXECUTION_EVENT_TYPE_LABELS);
}

export function formatAccountTypeLabel(
  accountType: string | null | undefined,
): string {
  return resolveLabel(accountType, ACCOUNT_TYPE_LABELS);
}

export function formatCashFlowTypeLabel(
  flowType: string | null | undefined,
): string {
  return resolveLabel(flowType, CASH_FLOW_TYPE_LABELS);
}

export function formatStrategyRuntimeStatus(
  status: string | null | undefined,
): string {
  return resolveLabel(status, STRATEGY_RUNTIME_STATUS_LABELS, "未知");
}

export function formatBooleanLabel(
  value: boolean | null | undefined,
  truthy = "是",
  falsy = "否",
): string {
  if (value == null) {
    return "未知";
  }

  return value ? truthy : falsy;
}

export function formatGenericStatusLabel(
  value: string | null | undefined,
): string {
  return resolveLabel(value, BOOLEAN_LABELS, "未知");
}

export function formatRealTradeEventTypeLabel(
  eventType: string | null | undefined,
): string {
  return resolveLabel(eventType, REAL_TRADE_EVENT_TYPE_LABELS, "未设置");
}

export function formatFutuProgramStatusLabel(
  status: string | null | undefined,
): string {
  if (status == null || status.trim() === "") {
    return "暂无";
  }

  const separatorIndex = status.indexOf(":");
  const statusType = separatorIndex >= 0 ? status.slice(0, separatorIndex) : status;
  const description = separatorIndex >= 0 ? status.slice(separatorIndex + 1).trim() : "";
  const label = resolveLabel(statusType, FUTU_PROGRAM_STATUS_LABELS, statusType);
  return description === "" ? label : `${label}：${description}`;
}

function resolveRealTradeHardStopScope(entry: {
  market?: string | null;
  symbol?: string | null;
  hardStopScope?: string | null;
}): RealTradeHardStopScope {
  switch (entry.hardStopScope) {
    case "ACCOUNT":
    case "MARKET":
    case "SYMBOL":
      return entry.hardStopScope;
  }

  if (entry.symbol != null) {
    return "SYMBOL";
  }

  if (entry.market != null) {
    return "MARKET";
  }

  return "ACCOUNT";
}

export function formatRealTradeHardStopScope(entry: {
  market?: string | null;
  symbol?: string | null;
  hardStopScope?: string | null;
}): string {
  const scope = resolveRealTradeHardStopScope(entry);

  switch (scope) {
    case "SYMBOL":
      return `标的 / ${formatMarketLabel(entry.market)} / ${entry.symbol ?? "未设置"}`;
    case "MARKET":
      return `市场 / ${formatMarketLabel(entry.market)}`;
    case "ACCOUNT":
      return "账户";
  }
}

export function resolveRealTradeHardStopScopeTagType(entry: {
  market?: string | null;
  symbol?: string | null;
  hardStopScope?: string | null;
}): "info" | "warning" | "danger" {
  switch (resolveRealTradeHardStopScope(entry)) {
    case "ACCOUNT":
      return "info";
    case "MARKET":
      return "warning";
    case "SYMBOL":
      return "danger";
  }
}

export function formatRealTradeKillSwitchSource(
  source: RealTradeKillSwitchStateResponse["killSwitchSource"],
): string {
  switch (source) {
    case "RUNTIME":
      return "运行时";
    default:
      return "未启用";
  }
}

export function resolveRealTradeKillSwitchEventTagType(
  eventType: RealTradeKillSwitchEventsResponse["entries"][number]["eventType"],
): "success" | "warning" | "danger" | "info" {
  switch (eventType) {
    case "released":
      return "success";
    case "activated":
      return "warning";
    case "rejected":
      return "danger";
    default:
      return "info";
  }
}

export function resolveRealTradeRiskEventTagType(
  eventType: RealTradeRiskEventsResponse["entries"][number]["eventType"],
): "success" | "warning" | "danger" | "info" {
  switch (eventType) {
    case "released":
    case "disabled":
      return "success";
    case "activated":
    case "updated":
      return "warning";
    case "rejected":
      return "danger";
    default:
      return "info";
  }
}

export function formatDateTime(value: string | null | undefined): string {
  return formatSharedDateTime(value, {
    fallback: "暂无",
    timeZoneName: "short",
  });
}

export function formatDurationMs(value: number | null | undefined): string {
  if (value == null) {
    return "暂无";
  }

  if (value < 1000) {
    return `${value}毫秒`;
  }

  if (value < 60_000) {
    return `${Math.round(value / 1000)}秒`;
  }

  if (value < 3_600_000) {
    return `${Math.round(value / 60_000)}分`;
  }

  return `${Math.round(value / 3_600_000)}小时`;
}
