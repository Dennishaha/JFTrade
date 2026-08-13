import type { BacktestTradingCostsPayload } from "@/types";
import type { ChartType, HeikinAshiSeed } from "@/charting/kline";
import type { BacktestTrade, BacktestPnlPoint, BacktestDrawdownPoint, BacktestCandle } from "@/components/backtest/BacktestChart.vue";

export interface BacktestTradeView extends BacktestTrade {
  priceText?: string | undefined;
  qtyText?: string | undefined;
}

export interface BacktestCandleView extends BacktestCandle {
  openText?: string | undefined;
  highText?: string | undefined;
  lowText?: string | undefined;
  closeText?: string | undefined;
  volumeText?: string | undefined;
}

export interface BacktestOrderBookEntry {
  orderId: string;
  clientOrderId?: string | undefined;
  symbol: string;
  side: string;
  quantity: number;
  quantityText?: string | undefined;
  orderType?: string | undefined;
  orderPrice?: number | undefined;
  orderPriceText?: string | undefined;
  submittedAt?: string | undefined;
  status: string;
  filledQuantity?: number | undefined;
  filledQuantityText?: string | undefined;
  filledPrice?: number | undefined;
  filledPriceText?: string | undefined;
  filledAt?: string | undefined;
  brokerFee?: number | undefined;
  marketFee?: number | undefined;
  totalFee?: number | undefined;
  feeCurrency?: string | undefined;
  warmup?: boolean | undefined;
}

export interface BacktestFeeBreakdownEntry {
  ruleId: string;
  label: string;
  group: string;
  category: string;
  currency: string;
  amount: number;
  count: number;
}

export interface BacktestRunResult {
  symbol: string;
  marketDataProvider?: string | undefined;
  interval: string;
  chartType?: ChartType | undefined;
  startTime: string;
  endTime: string;
  quoteCurrency?: string | undefined;
  finalBalance: number;
  pnl: number;
  totalBrokerFees?: number | undefined;
  totalMarketFees?: number | undefined;
  totalFees?: number | undefined;
  feeBreakdown?: BacktestFeeBreakdownEntry[] | undefined;
  tradingCosts?: BacktestTradingCostsPayload | undefined;
  maxDrawdown?: number | undefined;
  currentDrawdown?: number | undefined;
  tradeStatsVersion?: number | undefined;
  totalFills?: number | undefined;
  totalTrades: number;
  winRate: number;
  trades?: BacktestTradeView[] | undefined;
  orderBook?: BacktestOrderBookEntry[] | undefined;
  pnlCurve?: BacktestPnlPoint[] | undefined;
  drawdownCurve?: BacktestDrawdownPoint[] | undefined;
  candles?: BacktestCandleView[] | undefined;
  heikinAshiSeed?: HeikinAshiSeed | undefined;
  logs?: string[] | undefined;
  warnings?: string[] | undefined;
  warningTotal?: number | undefined;
  warningsTruncated?: boolean | undefined;
  ignoredOrders?: number | undefined;
  executionModel?: "conservative-bar-v1" | undefined;
  runtimeErrors?: string[] | undefined;
  runtimeErrorCounts?: Record<string, number> | undefined;
  runtimeErrorTotal?: number | undefined;
  runtimeErrorsTruncated?: boolean | undefined;
  error?: string | undefined;
}

export interface BacktestRun {
  id: string;
  status: string;
  marketDataProvider?: string | undefined;
  request: {
    definitionId: string;
    definitionVersion?: string;
    market?: string;
    code?: string;
    symbol: string;
    instrumentType?: string;
    interval: string;
    chartType: ChartType;
    startDate?: string;
    endDate?: string;
    startTime: string;
    endTime: string;
    marketTimezone?: string;
    initialBalance: number;
    rehabType?: string;
    useExtendedHours?: boolean;
    tradingCosts?: BacktestTradingCostsPayload;
    executionModel?: "conservative-bar-v1";
  };
  result?: BacktestRunResult | undefined;
  createdAt: string;
  updatedAt: string;
}
