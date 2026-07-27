import type {
  MarketDataProviderDescriptor,
  MarketDataProviderHealth,
  MarketDataSubscriptionQuotaBucketDto,
} from "../../contracts/generated/market-data";

export interface MarketDataQuoteSnapshotDto {
  lastPrice: number | null;
  openPrice: number | null;
  highPrice: number | null;
  lowPrice: number | null;
  previousClosePrice: number | null;
  volume: number | null;
  turnover: number | null;
  bidPrice: number | null;
  bidSize: number | null;
  askPrice: number | null;
  askSize: number | null;
  quoteCurrency: string | null;
  marketPhase: string;
}

export interface MarketDataCandleDto {
  interval: string;
  openTime: string;
  closeTime: string;
  openPrice: number;
  highPrice: number;
  lowPrice: number;
  closePrice: number;
  volume: number | null;
  turnover: number | null;
  closed: boolean;
}

export interface MarketDataTradeTickDto {
  price: number;
  size: number | null;
  turnover: number | null;
  side: string;
  tradeId: string | null;
}

export interface MarketDataQueryMetaDto {
  instrumentId: string;
  source: string | null;
  brokerId?: string | null;
  resolvedAt: string;
  fromCache: boolean;
}

export interface MarketDataExtendedQuote {
  price?: number | null;
  highPrice?: number | null;
  lowPrice?: number | null;
  volume?: number | null;
  turnover?: number | null;
  changeVal?: number | null;
  changeRate?: number | null;
  amplitude?: number | null;
  quoteTime?: string | null;
}

export interface MarketDataExtendedQuoteBlocks {
  preMarket?: MarketDataExtendedQuote | null;
  afterMarket?: MarketDataExtendedQuote | null;
  overnight?: MarketDataExtendedQuote | null;
}

export interface MarketSecurityRef {
  instrumentId: string;
  market: string;
  symbol: string;
}

export interface MarketSecurityEquityDetails {
  issuedShares: number;
  issuedMarketValue: number;
  netAsset: number;
  netProfit: number;
  earningsPerShare: number;
  outstandingShares: number;
  outstandingMarketVal: number;
  netAssetPerShare: number;
  earningsYieldRate: number;
  peRate: number;
  pbRate: number;
  peTTMRate: number;
  dividendTTM?: number | null;
  dividendRatioTTM?: number | null;
  dividendLFY?: number | null;
  dividendLFYRatio?: number | null;
}

export interface MarketSecurityWarrantDetails {
  conversionRate: number;
  warrantType: string;
  strikePrice: number;
  maturityTime: string;
  endTradeTime: string;
  owner?: MarketSecurityRef | null;
  recoveryPrice: number;
  streetVolume: number;
  issueVolume: number;
  streetRate: number;
  delta: number;
  impliedVolatility: number;
  premium: number;
  maturityTimestamp?: number | null;
  endTradeTimestamp?: number | null;
  leverage?: number | null;
  inOutPriceRatio?: number | null;
  breakEvenPoint?: number | null;
  conversionPrice?: number | null;
  priceRecoveryRatio?: number | null;
  score?: number | null;
  upperStrikePrice?: number | null;
  lowerStrikePrice?: number | null;
  inLinePriceStatus?: string | null;
  issuerCode?: string | null;
}

export interface MarketSecurityOptionDetails {
  optionType: string;
  owner?: MarketSecurityRef | null;
  strikeTime: string;
  strikePrice: number;
  contractSize: number;
  contractSizeFloat?: number | null;
  openInterest: number;
  impliedVolatility: number;
  premium: number;
  delta: number;
  gamma: number;
  vega: number;
  theta: number;
  rho: number;
  strikeTimestamp?: number | null;
  indexOptionType?: string | null;
  netOpenInterest?: number | null;
  expiryDateDistance?: number | null;
  contractNominalValue?: number | null;
  ownerLotMultiplier?: number | null;
  optionAreaType?: string | null;
  contractMultiplier?: number | null;
}

export interface MarketSecurityIndexDetails {
  raiseCount: number;
  fallCount: number;
  equalCount: number;
}

export interface MarketSecurityPlateDetails {
  raiseCount: number;
  fallCount: number;
  equalCount: number;
}

export interface MarketSecurityFutureDetails {
  lastSettlePrice: number;
  position: number;
  positionChange: number;
  lastTradeTime: string;
  lastTradeTimestamp?: number | null;
  isMainContract: boolean;
}

export interface MarketSecurityTrustDetails {
  dividendYield: number;
  aum: number;
  outstandingUnit: number;
  netAssetValue: number;
  premium: number;
  assetClass: string;
}

export interface MarketSecurityDetails {
  instrumentId: string;
  market: string;
  symbol: string;
  securityId?: number | null;
  name: string;
  securityType: string;
  productClass: string;
  marketSegment: string;
  exchangeType: string;
  listTime: string;
  listTimestamp?: number | null;
  delisting?: boolean | null;
  lotSize: number;
  isSuspend: boolean;
  priceSpread: number;
  updateTime: string;
  updateTimestamp?: number | null;
  highPrice: number;
  openPrice: number;
  lowPrice: number;
  lastClosePrice: number;
  currentPrice: number;
  volume: number;
  turnover: number;
  turnoverRate: number;
  askPrice?: number | null;
  bidPrice?: number | null;
  askVolume?: number | null;
  bidVolume?: number | null;
  amplitude?: number | null;
  averagePrice?: number | null;
  bidAskRatio?: number | null;
  volumeRatio?: number | null;
  highest52WeeksPrice?: number | null;
  lowest52WeeksPrice?: number | null;
  highestHistoryPrice?: number | null;
  lowestHistoryPrice?: number | null;
  sessionStatus?: string | null;
  closePrice5Minute?: number | null;
  highPrecisionVolume?: number | null;
  highPrecisionAskVol?: number | null;
  highPrecisionBidVol?: number | null;
  extended?: MarketDataExtendedQuoteBlocks | null;
  equity?: MarketSecurityEquityDetails | null;
  warrant?: MarketSecurityWarrantDetails | null;
  option?: MarketSecurityOptionDetails | null;
  index?: MarketSecurityIndexDetails | null;
  plate?: MarketSecurityPlateDetails | null;
  future?: MarketSecurityFutureDetails | null;
  trust?: MarketSecurityTrustDetails | null;
}

export interface MarketSecurityDetailsQueryResult {
  request: {
    market: string;
    symbol: string;
    instrumentId: string;
  };
  security: MarketSecurityDetails | null;
  meta: MarketDataQueryMetaDto;
}

export interface MarketDataSnapshotResponse {
  ok: boolean;
  instrumentId: string;
  snapshot: MarketDataQuoteSnapshotDto | null;
  meta: MarketDataQueryMetaDto;
  error: string | null;
}

export interface MarketDataCandlesResponse {
  ok: boolean;
  instrumentId: string;
  interval: string;
  fromTime: string | null;
  toTime: string | null;
  totalReturned: number;
  candles: MarketDataCandleDto[];
  pagination?: {
    hasMore: boolean;
    nextBefore?: string | null;
  };
  meta: MarketDataQueryMetaDto;
  error: string | null;
}

export interface MarketDataTicksResponse {
  ok: boolean;
  instrumentId: string;
  fromTime: string;
  toTime: string;
  totalReturned: number;
  ticks: MarketDataTradeTickDto[];
  meta: MarketDataQueryMetaDto;
  error: string | null;
}

// --- Depth (Order Book) ---

export interface OrderBookDetailItemDto {
  orderId: number;
  volume: number;
}

export interface OrderBookLevelDto {
  price: number;
  volume: number;
  orderCount: number;
  detailList?: OrderBookDetailItemDto[] | null;
}

export interface OrderBookSnapshotDto {
  accountId: string;
  symbol: string;
  name?: string | null;
  svrRecvTimeBid?: string | null;
  svrRecvTimeAsk?: string | null;
  bids: OrderBookLevelDto[];
  asks: OrderBookLevelDto[];
}

export interface MarketDataDepthResponse {
  request: {
    market: string;
    symbol: string;
    instrumentId: string;
    num: number;
  };
  depth: OrderBookSnapshotDto;
  meta: MarketDataQueryMetaDto;
}

export interface OrderBookDepthPreset {
  num: number;
  label: string;
}

export interface BrokerOrderBookCapability {
  defaultNum: number;
  minNum: number;
  maxNum: number;
  numPresets: number[];
  supportsRealTimePush: boolean;
}

export interface MarketDataRuntimeState {
  Connected: boolean;
  Generation: number;
  ActiveCount: number;
  LastRefreshAt: string;
  QuoteRetryAt: string;
  QuoteFailures: number;
  QuoteLastError: string;
  StreamRetryAt: string;
  StreamFailures: number;
  StreamLastError: string;
  Closed: boolean;
}

export interface MarketDataProviderStatusResponse {
  descriptor: MarketDataProviderDescriptor;
  health: MarketDataProviderHealth;
  runtime: MarketDataRuntimeState;
  subscriptions: MarketDataSubscriptionsResponse;
  checkedAt: string;
}

export interface MarketDataSubscriptionEntryDto {
  key: string;
  channel: string;
  market: string;
  symbol: string;
  instrumentId: string;
  interval: string | null;
  depthLevel: number | null;
  consumers: string[];
  refCount: number;
  createdAt: string;
  updatedAt: string;
  brokerState?: "active" | "pending_subscribe" | "pending_unsubscribe" | "retrying" | "unmanaged";
  subscribedAt?: string | null;
  unsubscribeEligibleAt?: string | null;
  lastError?: string | null;
}

export interface MarketDataSubscriptionsResponse {
  totalActiveSubscriptions: number;
  consumerId?: string;
  providerBrokerId?: string;
  action?: "acquired" | "released" | "heartbeat" | string;
  instruments?: Array<{
    channel?: string;
    market: string;
    symbol: string;
    interval?: string;
  }>;
  transport?: {
    mode: string;
  };
  desiredCount?: number;
  ownActiveCount?: number;
  pendingReleaseCount?: number;
  totalUsedQuota?: number | null;
  remainQuota?: number | null;
  quota: {
    totalUsed: number;
    totalLimit: number | null;
    totalRemaining: number | null;
    byMarket: MarketDataSubscriptionQuotaBucketDto[];
  };
  entries: MarketDataSubscriptionEntryDto[];
  brokerState?: {
    desiredCount: number;
    ownActiveCount: number;
    pendingReleaseCount: number;
    totalUsedQuota: number | null;
    remainQuota: number | null;
    ownUsedQuota?: number;
    checkedAt?: string | null;
    reconciledAt?: string | null;
    lastError?: string | null;
    entries: Array<{
      key: string;
      kind: string;
      instrumentId: string;
      interval: string | null;
      brokerState: string;
      subscribedAt: string | null;
      unsubscribeEligibleAt: string | null;
      lastError: string | null;
    }>;
  };
}

export const emptyMarketDataSubscriptions: MarketDataSubscriptionsResponse = {
  totalActiveSubscriptions: 0,
  quota: {
    totalUsed: 0,
    totalLimit: null,
    totalRemaining: null,
    byMarket: [],
  },
  entries: [],
};
