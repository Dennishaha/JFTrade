import { computed, onMounted, onUnmounted, ref, watch } from "vue";

import type {
  ExecutionComboRequest,
  ExecutionCommandResponse as ExecutionResponse,
} from "@/contracts";

import {
  apiDeletePath,
  apiPost,
  apiPostPath,
  apiPostPathAction,
} from "@/composables/shared/apiClient";
import {
  useBrokerProviderSelection,
  withBrokerProvider,
} from "@/composables/trading/brokerProviderSelection";
import {
  fetchProductFeature,
  type ProductFeatureResult,
} from "@/composables/product/productFeatures";
import { useConsoleData } from "@/composables/workspace/useConsoleData";
import { usePolling } from "@/composables/shared/usePolling";

type Entry = Record<string, unknown>;
type DiscoverStage =
  | "categories"
  | "competitions"
  | "series"
  | "events"
  | "contracts"
  | "contract";
type Mode = "discover" | "parlay";
export type PredictionContractView =
  | "snapshot"
  | "depth"
  | "candles"
  | "ticks"
  | "milestones";

interface ComboPreview {
  previewId: string;
  expiresAt?: string;
  buyingPowerImpact?: number;
  warnings?: string[];
}
export interface PredictionResearchProps {
  presentation?: "workspace" | "research";
  seriesCode?: string;
  eventCode?: string;
  contractCode?: string;
  contractView?: PredictionContractView;
}

export interface PredictionResearchEmit {
  (
    event: "openInstrument",
    instrumentID: string,
    marketSegment: "prediction",
    productClass: "event_contract",
  ): void;
  (event: "update:seriesCode", seriesCode: string): void;
  (event: "update:eventCode", eventCode: string): void;
  (event: "update:contractCode", contractCode: string): void;
  (event: "update:contractView", contractView: PredictionContractView): void;
}

export function usePredictionResearch(
  props: Readonly<PredictionResearchProps>,
  emit: PredictionResearchEmit,
) {
const { selectedBrokerAccount, systemStatus } = useConsoleData();
const { selectedBrokerId } = useBrokerProviderSelection();
const mode = ref<Mode>("discover");
const initialSeriesCode = String(props.seriesCode ?? "").trim();
const initialEventCode = String(props.eventCode ?? "").trim();
const initialContractCode = String(props.contractCode ?? "").trim();
const stage = ref<DiscoverStage>(
  initialContractCode
    ? "contract"
    : initialEventCode
      ? "contracts"
      : initialSeriesCode
        ? "events"
        : "categories",
);
const loading = ref(false);
const error = ref("");
const result = ref<ProductFeatureResult | null>(null);
const category = ref("");
const tag = ref("");
const seriesCode = ref(initialSeriesCode);
const eventCode = ref(initialEventCode);
const contractCode = ref(initialContractCode);
const contractView = ref<PredictionContractView>(
  props.contractView ?? "snapshot",
);

const eligible = ref<ProductFeatureResult | null>(null);
const selectedLegs = ref<Record<string, "YES" | "NO">>({});
const quote = ref<ProductFeatureResult | null>(null);
const preview = ref<ComboPreview | null>(null);
const amount = ref(20);
const confirmed = ref(false);
const submitting = ref(false);
const execution = ref<ExecutionResponse | null>(null);
const quoteClock = ref(Date.now());
const parlayClientOrderID = ref("");
const pageVisible = ref(
  typeof document === "undefined" || document.visibilityState === "visible",
);
const activeSubscription = ref<{
  leaseId: string;
  code: string;
  dataType: string;
} | null>(null);
const contractRefresh = ref(0);
let subscriptionGeneration = 0;
const quoteClockPolling = usePolling(
  () => {
    quoteClock.value = Date.now();
  },
  { intervalMs: 1_000 },
);
const contractRefreshPolling = usePolling(
  () => {
    if (
      pageVisible.value &&
      stage.value === "contract" &&
      subscriptionReady.value &&
      ["snapshot", "depth", "candles", "ticks"].includes(contractView.value)
    ) {
      contractRefresh.value += 1;
    }
  },
  { intervalMs: 3_000 },
);

const stageLabels: Record<DiscoverStage, string> = {
  categories: "分类",
  competitions: "赛事",
  series: "系列",
  events: "事件",
  contracts: "合约",
  contract: "合约行情",
};

function asObject(value: unknown): Entry {
  return value != null && typeof value === "object" && !Array.isArray(value)
    ? (value as Entry)
    : {};
}
function securityCode(value: unknown): string {
  return String(asObject(value).code ?? "");
}
function itemTitle(entry: Entry, index: number): string {
  return String(
    entry.categoryName ??
      entry.eventName ??
      entry.seriesName ??
      entry.title ??
      entry.tag ??
      entry.category ??
      `结果 ${index + 1}`,
  );
}
function itemSubtitle(entry: Entry): string {
  const values = [
    entry.competition,
    entry.competitionScope,
    entry.status,
    entry.endDate,
    entry.closeTime,
  ].filter((value) => value != null && value !== "");
  return values.map(String).join(" · ");
}
function queryString(values: Record<string, string>): string {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(values)) {
    if (value) params.set(key, value);
  }
  params.set("pageSize", "100");
  return params.toString();
}

function discoverStageFromContext(): DiscoverStage {
  if (contractCode.value) return "contract";
  if (eventCode.value) return "contracts";
  if (seriesCode.value) return "events";
  return "categories";
}

async function loadDiscover(
  nextStage: DiscoverStage = stage.value,
): Promise<void> {
  if (nextStage === "contract") return;
  loading.value = true;
  error.value = "";
  try {
    let path = "/api/v1/market-data/prediction/categories?pageSize=100";
    if (nextStage === "competitions") {
      path = `/api/v1/market-data/prediction/competitions?${queryString({ category: category.value })}`;
    } else if (nextStage === "series") {
      path = `/api/v1/market-data/prediction/series?${queryString({ category: category.value, tag: tag.value })}`;
    } else if (nextStage === "events") {
      path = `/api/v1/market-data/prediction/events?${queryString({ seriesId: seriesCode.value })}`;
    } else if (nextStage === "contracts") {
      path = `/api/v1/market-data/prediction/events/${encodeURIComponent(eventCode.value)}/contracts?pageSize=100`;
    }
    result.value = await fetchProductFeature(
      withBrokerProvider(path, selectedBrokerId.value),
    );
    stage.value = nextStage;
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : String(cause);
    result.value = null;
  } finally {
    loading.value = false;
  }
}

function selectDiscoverEntry(entry: Entry): void {
  switch (stage.value) {
    case "categories":
      category.value = String(entry.category ?? "");
      void loadDiscover("competitions");
      break;
    case "competitions":
      tag.value = String(entry.tag ?? "");
      void loadDiscover("series");
      break;
    case "series":
      seriesCode.value = securityCode(entry.seriesSecurity);
      eventCode.value = "";
      contractCode.value = "";
      contractView.value = "snapshot";
      emit("update:seriesCode", seriesCode.value);
      void loadDiscover("events");
      break;
    case "events":
      eventCode.value = securityCode(entry.eventSecurity);
      contractCode.value = "";
      contractView.value = "snapshot";
      emit("update:eventCode", eventCode.value);
      void loadDiscover("contracts");
      break;
    case "contracts":
      contractCode.value = securityCode(entry.contractSecurity);
      contractView.value = "snapshot";
      stage.value = "contract";
      emit("update:contractCode", contractCode.value);
      break;
  }
}

function backDiscover(): void {
  const order: DiscoverStage[] = [
    "categories",
    "competitions",
    "series",
    "events",
    "contracts",
    "contract",
  ];
  const index = order.indexOf(stage.value);
  if (index <= 0) return;
  switch (stage.value) {
    case "contract":
      contractCode.value = "";
      contractView.value = "snapshot";
      emit("update:contractCode", "");
      break;
    case "contracts":
      eventCode.value = "";
      contractCode.value = "";
      contractView.value = "snapshot";
      emit("update:eventCode", "");
      break;
    case "events":
      seriesCode.value = "";
      eventCode.value = "";
      contractCode.value = "";
      contractView.value = "snapshot";
      emit("update:seriesCode", "");
      break;
    case "series":
      tag.value = "";
      break;
    case "competitions":
      category.value = "";
      break;
    default:
      break;
  }
  void loadDiscover(order[index - 1]!);
}

function selectContractView(view: PredictionContractView): void {
  if (view === contractView.value) return;
  contractView.value = view;
  emit("update:contractView", view);
}

const contractPath = computed(() => {
  const base = `/api/v1/market-data/prediction/contracts/${encodeURIComponent(contractCode.value)}`;
  switch (contractView.value) {
    case "depth":
      return withBrokerProvider(
        `${base}/order-book?pageSize=20`,
        selectedBrokerId.value,
      );
    case "candles":
      return withBrokerProvider(
        `${base}/candles?pageSize=100`,
        selectedBrokerId.value,
      );
    case "ticks":
      return withBrokerProvider(
        `${base}/ticks?pageSize=100`,
        selectedBrokerId.value,
      );
    case "milestones":
      return withBrokerProvider(
        `${base}/milestones?pageSize=100`,
        selectedBrokerId.value,
      );
    default:
      return withBrokerProvider(`${base}/snapshot`, selectedBrokerId.value);
  }
});
const contractSubscriptionType = computed(() => {
  switch (contractView.value) {
    case "depth":
      return "ORDER_BOOK";
    case "candles":
      return "KLINE";
    case "ticks":
      return "TICKER";
    default:
      return "";
  }
});
const contractPanelKey = computed(
  () => `${contractPath.value}:${contractRefresh.value}`,
);
const subscriptionReady = computed(
  () =>
    contractSubscriptionType.value === "" ||
    (activeSubscription.value?.code === contractCode.value &&
      activeSubscription.value?.dataType === contractSubscriptionType.value),
);

function subscriptionQuery(): string {
  const params = new URLSearchParams();
  const brokerId =
    selectedBrokerId.value ||
    selectedBrokerAccount.value?.brokerId ||
    systemStatus.value.defaultBroker;
  const accountId =
    selectedBrokerAccount.value?.brokerId === brokerId
      ? selectedBrokerAccount.value.accountId
      : "";
  if (brokerId) params.set("brokerId", brokerId);
  if (accountId) params.set("accountId", accountId);
  const value = params.toString();
  return value ? `?${value}` : "";
}

async function releaseContractSubscription(
  subscription: NonNullable<typeof activeSubscription.value>,
): Promise<void> {
  await apiDeletePath(
    "/api/v1/market-data/prediction/contracts/{code}/subscriptions/{leaseId}",
    `/api/v1/market-data/prediction/contracts/${encodeURIComponent(subscription.code)}/subscriptions/${encodeURIComponent(subscription.leaseId)}`,
  );
}

async function syncContractSubscription(): Promise<void> {
  const generation = ++subscriptionGeneration;
  const previous = activeSubscription.value;
  activeSubscription.value = null;
  if (previous != null) {
    try {
      await releaseContractSubscription(previous);
    } catch {
      // Lease release is idempotent; a disconnected OpenD session drops all
      // subscriptions with the connection.
    }
  }
  const dataType = contractSubscriptionType.value;
  const code = contractCode.value;
  if (
    generation !== subscriptionGeneration ||
    mode.value !== "discover" ||
    stage.value !== "contract" ||
    !pageVisible.value ||
    !code ||
    !dataType
  ) {
    return;
  }
  try {
    const lease = await apiPostPath(
      "/api/v1/market-data/prediction/contracts/{code}/subscriptions",
      `/api/v1/market-data/prediction/contracts/${encodeURIComponent(code)}/subscriptions${subscriptionQuery()}`,
      { dataTypes: [dataType] },
    );
    const acquired = { leaseId: lease.leaseId, code, dataType };
    if (generation !== subscriptionGeneration) {
      await releaseContractSubscription(acquired);
      return;
    }
    activeSubscription.value = acquired;
    contractRefresh.value++;
  } catch (cause) {
    if (generation === subscriptionGeneration) {
      error.value = cause instanceof Error ? cause.message : String(cause);
    }
  }
}

function handleVisibilityChange(): void {
  pageVisible.value =
    typeof document === "undefined" || document.visibilityState === "visible";
}

watch(
  [
    contractCode,
    contractView,
    stage,
    mode,
    pageVisible,
    () => selectedBrokerAccount.value?.brokerId,
    () => selectedBrokerAccount.value?.accountId,
    selectedBrokerId,
  ],
  () => {
    void syncContractSubscription();
  },
);
watch(selectedBrokerId, () => {
  if (mode.value === "parlay") {
    void loadEligible();
    return;
  }
  if (stage.value !== "contract") void loadDiscover(stage.value);
});

watch(
  () =>
    [
      props.seriesCode,
      props.eventCode,
      props.contractCode,
      props.contractView,
    ] as const,
  ([nextSeries, nextEvent, nextContract, nextView]) => {
    const normalizedSeries = String(nextSeries ?? "").trim();
    const normalizedEvent = String(nextEvent ?? "").trim();
    const normalizedContract = String(nextContract ?? "").trim();
    const normalizedView = nextView ?? "snapshot";
    const contextChanged =
      normalizedSeries !== seriesCode.value ||
      normalizedEvent !== eventCode.value ||
      normalizedContract !== contractCode.value ||
      normalizedView !== contractView.value;
    if (!contextChanged) return;

    seriesCode.value = normalizedSeries;
    eventCode.value = normalizedEvent;
    contractCode.value = normalizedContract;
    contractView.value = normalizedView;
    const nextStage = discoverStageFromContext();
    stage.value = nextStage;
    if (mode.value === "discover" && nextStage !== "contract") {
      void loadDiscover(nextStage);
    }
  },
);

const parlayContracts = computed(() => {
  const entries: Array<{ code: string; eventName: string }> = [];
  for (const event of eligible.value?.entries ?? []) {
    const eventName = String(
      event.eventName ?? event.competition ?? "预测事件",
    );
    const contracts = Array.isArray(event.comboContracts)
      ? event.comboContracts
      : [];
    for (const contract of contracts) {
      const code = securityCode(contract);
      if (code) entries.push({ code, eventName });
    }
  }
  return entries;
});
const selectedLegCount = computed(() => Object.keys(selectedLegs.value).length);
const mvc = computed(() => String(eligible.value?.metadata?.mvc ?? ""));
const quoteID = computed(() => String(quote.value?.metadata?.quoteId ?? ""));
const quoteExpiresAt = computed(() =>
  String(quote.value?.metadata?.quoteExpiresAt ?? ""),
);
const quoteExpired = computed(() => {
  const timestamp = Date.parse(quoteExpiresAt.value);
  return !Number.isFinite(timestamp) || quoteClock.value >= timestamp;
});

function toggleParlayContract(code: string): void {
  const next = { ...selectedLegs.value };
  if (next[code]) delete next[code];
  else next[code] = "YES";
  selectedLegs.value = next;
  quote.value = null;
  preview.value = null;
  execution.value = null;
  confirmed.value = false;
  parlayClientOrderID.value = "";
}
function setParlaySide(code: string, side: "YES" | "NO"): void {
  selectedLegs.value = { ...selectedLegs.value, [code]: side };
  quote.value = null;
  preview.value = null;
  confirmed.value = false;
  parlayClientOrderID.value = "";
}
function parlaySide(code: string): "YES" | "NO" {
  return selectedLegs.value[code] ?? "YES";
}

async function loadEligible(): Promise<void> {
  loading.value = true;
  error.value = "";
  try {
    eligible.value = await fetchProductFeature(
      withBrokerProvider(
        "/api/v1/market-data/prediction/combos/eligible-events?pageSize=100",
        selectedBrokerId.value,
      ),
    );
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : String(cause);
    eligible.value = null;
  } finally {
    loading.value = false;
  }
}

function comboLegs(): ExecutionComboRequest["legs"] {
  return Object.entries(selectedLegs.value).map(([code, side]) => ({
    instrumentId: code.toUpperCase().startsWith("US.") ? code : `US.${code}`,
    productClass: "event_contract",
    side: "BUY",
    ratio: 1,
    predictionSide: side,
  }));
}

async function requestRFQ(): Promise<void> {
  if (selectedLegCount.value < 2 || !mvc.value) return;
  loading.value = true;
  error.value = "";
  execution.value = null;
  try {
    const brokerId =
      selectedBrokerId.value ||
      selectedBrokerAccount.value?.brokerId ||
      systemStatus.value.defaultBroker;
    quote.value = await apiPost(
      "/api/v1/market-data/prediction/combos/quotes",
      {
        brokerId,
        accountId: selectedBrokerAccount.value?.accountId ?? "",
        tradingEnvironment:
          selectedBrokerAccount.value?.tradingEnvironment ??
          systemStatus.value.defaultTradingEnvironment,
        mvc: mvc.value,
        legs: comboLegs(),
      },
    );
    confirmed.value = false;
    parlayClientOrderID.value = clientOrderID();
    quoteClock.value = Date.now();
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : String(cause);
    quote.value = null;
    preview.value = null;
  } finally {
    loading.value = false;
  }
}

function clientOrderID(): string {
  const suffix =
    typeof crypto !== "undefined" && typeof crypto.randomUUID === "function"
      ? crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  return `jftrade-parlay-${suffix}`;
}

function parlayPayload(previewId = ""): ExecutionComboRequest {
  return {
    brokerId:
      selectedBrokerAccount.value?.brokerId ??
      selectedBrokerId.value ??
      systemStatus.value.defaultBroker,
    tradingEnvironment:
      selectedBrokerAccount.value?.tradingEnvironment ??
      systemStatus.value.defaultTradingEnvironment,
    accountId: selectedBrokerAccount.value?.accountId ?? "",
    market: "US",
    clientOrderId: parlayClientOrderID.value,
    orderKind: "event_parlay",
    productClass: "event_contract",
    rfqId: quoteID.value,
    quoteExpiresAt: quoteExpiresAt.value || null,
    mvc: mvc.value,
    amount: amount.value,
    price: null,
    spread: null,
    previewId,
    underlyingInstrumentId: "",
    optionStrategy: "",
    nearExpiry: "",
    farExpiry: "",
    legs: comboLegs(),
  };
}

async function previewParlay(): Promise<void> {
  if (
    quoteExpired.value ||
    !quoteID.value ||
    amount.value <= 0
  ) {
    return;
  }
  submitting.value = true;
  error.value = "";
  try {
    if (!parlayClientOrderID.value) {
      parlayClientOrderID.value = clientOrderID();
    }
    preview.value = await apiPost(
      "/api/v1/execution/combos/previews",
      parlayPayload(),
    );
    confirmed.value = false;
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : String(cause);
    preview.value = null;
  } finally {
    submitting.value = false;
  }
}

async function placeParlay(): Promise<void> {
  if (
    !confirmed.value ||
    quoteExpired.value ||
    !quoteID.value ||
    !preview.value?.previewId ||
    amount.value <= 0
  ) {
    return;
  }
  submitting.value = true;
  error.value = "";
  try {
    execution.value = await apiPost(
      "/api/v1/execution/combos",
      parlayPayload(preview.value.previewId),
    );
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : String(cause);
    execution.value = null;
  } finally {
    submitting.value = false;
  }
}

watch(amount, () => {
  preview.value = null;
  confirmed.value = false;
});

async function cancelParlay(): Promise<void> {
  const id = execution.value?.internalOrderId;
  if (!id) return;
  submitting.value = true;
  try {
    execution.value = await apiPostPathAction(
      "/api/v1/execution/combos/{internalOrderId}/cancel",
      `/api/v1/execution/combos/${encodeURIComponent(id)}/cancel`,
    );
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : String(cause);
  } finally {
    submitting.value = false;
  }
}

function switchMode(next: Mode): void {
  mode.value = next;
  error.value = "";
  if (next === "parlay" && eligible.value == null) void loadEligible();
}

onMounted(() => {
  if (stage.value === "contract") {
    void syncContractSubscription();
  } else {
    void loadDiscover(stage.value);
  }
  if (typeof document !== "undefined") {
    document.addEventListener("visibilitychange", handleVisibilityChange);
  }
  quoteClockPolling.start();
  contractRefreshPolling.start();
});
onUnmounted(() => {
  if (typeof document !== "undefined") {
    document.removeEventListener("visibilitychange", handleVisibilityChange);
  }
  subscriptionGeneration++;
  const subscription = activeSubscription.value;
  activeSubscription.value = null;
  if (subscription != null) void releaseContractSubscription(subscription);
});

  return {
    mode,
    initialSeriesCode,
    initialEventCode,
    initialContractCode,
    stage,
    loading,
    error,
    result,
    category,
    tag,
    seriesCode,
    eventCode,
    contractCode,
    contractView,
    eligible,
    selectedLegs,
    quote,
    preview,
    amount,
    confirmed,
    submitting,
    execution,
    quoteClock,
    parlayClientOrderID,
    pageVisible,
    activeSubscription,
    contractRefresh,
    quoteClockPolling,
    contractRefreshPolling,
    stageLabels,
    contractPath,
    contractSubscriptionType,
    contractPanelKey,
    subscriptionReady,
    parlayContracts,
    selectedLegCount,
    mvc,
    quoteID,
    quoteExpiresAt,
    quoteExpired,
    asObject,
    securityCode,
    itemTitle,
    itemSubtitle,
    queryString,
    discoverStageFromContext,
    loadDiscover,
    selectDiscoverEntry,
    backDiscover,
    selectContractView,
    subscriptionQuery,
    releaseContractSubscription,
    syncContractSubscription,
    handleVisibilityChange,
    toggleParlayContract,
    setParlaySide,
    parlaySide,
    loadEligible,
    comboLegs,
    requestRFQ,
    clientOrderID,
    parlayPayload,
    previewParlay,
    placeParlay,
    cancelParlay,
    switchMode,
    selectedBrokerAccount,
    systemStatus,
    selectedBrokerId,
  };
}
