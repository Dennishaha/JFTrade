<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";

import { fetchEnvelopeWithInit } from "../../composables/apiClient";
import {
  useBrokerProviderSelection,
  withBrokerProvider,
} from "../../composables/brokerProviderSelection";
import {
  fetchProductFeature,
  type ProductFeatureResult,
} from "../../composables/productFeatures";
import { useConsoleData } from "../../composables/useConsoleData";
import { usePolling } from "../../composables/usePolling";
import PredictionContractDataView from "../research/PredictionContractDataView.vue";

type Entry = Record<string, unknown>;
type DiscoverStage =
  | "categories"
  | "competitions"
  | "series"
  | "events"
  | "contracts"
  | "contract";
type Mode = "discover" | "parlay";
type ContractView =
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
interface ExecutionResponse {
  accepted: boolean;
  internalOrderId?: string;
  brokerOrderId?: string;
  orderStatus?: string;
  message?: string;
}
interface PredictionSubscriptionLease {
  leaseId: string;
  instrumentId: string;
  dataTypes: string[];
}

const props = withDefaults(
  defineProps<{
    presentation?: "workspace" | "research";
    seriesCode?: string;
    eventCode?: string;
    contractCode?: string;
    contractView?: ContractView;
  }>(),
  {
    presentation: "workspace",
    seriesCode: "",
    eventCode: "",
    contractCode: "",
    contractView: "snapshot",
  },
);

const emit = defineEmits<{
  openInstrument: [
    instrumentID: string,
    marketSegment: "prediction",
    productClass: "event_contract",
  ];
  "update:seriesCode": [seriesCode: string];
  "update:eventCode": [eventCode: string];
  "update:contractCode": [contractCode: string];
  "update:contractView": [contractView: ContractView];
}>();
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
const contractView = ref<ContractView>(props.contractView);

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

function selectContractView(view: ContractView): void {
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
  await fetchEnvelopeWithInit(
    `/api/v1/market-data/prediction/contracts/${encodeURIComponent(subscription.code)}/subscriptions/${encodeURIComponent(subscription.leaseId)}`,
    { method: "DELETE" },
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
    const lease = await fetchEnvelopeWithInit<PredictionSubscriptionLease>(
      `/api/v1/market-data/prediction/contracts/${encodeURIComponent(code)}/subscriptions${subscriptionQuery()}`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ dataTypes: [dataType] }),
      },
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
    const contextChanged =
      normalizedSeries !== seriesCode.value ||
      normalizedEvent !== eventCode.value ||
      normalizedContract !== contractCode.value ||
      nextView !== contractView.value;
    if (!contextChanged) return;

    seriesCode.value = normalizedSeries;
    eventCode.value = normalizedEvent;
    contractCode.value = normalizedContract;
    contractView.value = nextView;
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

function comboLegs() {
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
    quote.value = await fetchEnvelopeWithInit<ProductFeatureResult>(
      "/api/v1/market-data/prediction/combos/quotes",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          brokerId,
          accountId: selectedBrokerAccount.value?.accountId ?? "",
          tradingEnvironment:
            selectedBrokerAccount.value?.tradingEnvironment ??
            systemStatus.value.defaultTradingEnvironment,
          mvc: mvc.value,
          legs: comboLegs(),
        }),
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

function parlayPayload() {
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
    mvc: mvc.value,
    amount: amount.value,
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
    preview.value = await fetchEnvelopeWithInit<ComboPreview>(
      "/api/v1/execution/combos/previews",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(parlayPayload()),
      },
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
    execution.value = await fetchEnvelopeWithInit<ExecutionResponse>(
      "/api/v1/execution/combos",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          ...parlayPayload(),
          previewId: preview.value.previewId,
        }),
      },
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
    execution.value = await fetchEnvelopeWithInit<ExecutionResponse>(
      `/api/v1/execution/combos/${encodeURIComponent(id)}/cancel`,
      { method: "POST" },
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
</script>

<template>
  <section
    :class="[
      'prediction-research',
      `prediction-research--${presentation}`,
    ]"
  >
    <header class="prediction-research__header">
      <div class="prediction-research__segments" role="tablist" aria-label="预测市场研究模式">
        <button
          type="button"
          role="tab"
          :aria-selected="mode === 'discover'"
          :class="{ 'is-active': mode === 'discover' }"
          @click="switchMode('discover')"
        >
          事件与合约
        </button>
        <button
          type="button"
          role="tab"
          :aria-selected="mode === 'parlay'"
          :class="{ 'is-active': mode === 'parlay' }"
          @click="switchMode('parlay')"
        >
          Parlay 组合
        </button>
      </div>
      <div class="prediction-research__eligibility">
        US · prediction · 运行时账户资格
      </div>
    </header>

    <div
      v-if="loading || submitting"
      class="prediction-research__progress"
      role="progressbar"
      aria-label="加载中"
    />
    <div v-if="error" class="prediction-research__notice is-warning" role="alert">
      {{ error }}
    </div>

    <template v-if="mode === 'discover'">
      <nav class="prediction-research__breadcrumb" aria-label="预测市场层级">
        <button
          type="button"
          class="prediction-research__button"
          :disabled="stage === 'categories'"
          @click="backDiscover"
        >
          返回
        </button>
        <strong>{{ stageLabels[stage] }}</strong>
        <span v-if="category">{{ category }}</span>
        <span v-if="tag">/ {{ tag }}</span>
        <span v-if="seriesCode">/ {{ seriesCode }}</span>
        <span v-if="eventCode">/ {{ eventCode }}</span>
      </nav>

      <div v-if="stage !== 'contract'" class="prediction-research__grid">
        <button
          v-for="(entry, index) in result?.entries ?? []"
          :key="`${itemTitle(entry, index)}-${index}`"
          type="button"
          class="prediction-research__card"
          @click="selectDiscoverEntry(entry)"
        >
          <strong>{{ itemTitle(entry, index) }}</strong>
          <span>{{ itemSubtitle(entry) }}</span>
          <small v-if="Array.isArray(entry.tags)">
            {{ entry.tags.join(" · ") }}
          </small>
          <small v-if="Array.isArray(entry.competitionList)">
            {{ entry.competitionList.join(" · ") }}
          </small>
        </button>
      </div>

      <div v-else class="prediction-research__contract">
        <div
          class="prediction-research__segments prediction-research__segments--contract"
          role="tablist"
          aria-label="合约数据视图"
        >
          <button
            v-for="item in [
              ['snapshot', '快照'],
              ['depth', 'YES/NO 盘口'],
              ['candles', 'K 线'],
              ['ticks', '逐笔'],
              ['milestones', '里程碑'],
            ] as const"
            :key="item[0]"
            type="button"
            role="tab"
            :aria-selected="contractView === item[0]"
            :class="{ 'is-active': contractView === item[0] }"
            @click="selectContractView(item[0])"
          >
            {{ item[1] }}
          </button>
        </div>

        <PredictionContractDataView
          v-if="subscriptionReady"
          :key="contractPanelKey"
          :path="contractPath"
          :view="contractView"
        />
        <div v-else class="prediction-research__subscription">
          正在建立行情订阅…
        </div>
        <footer class="prediction-research__contract-footer">
          <span>关闭、待确认、确定、结算及取消状态按 OpenD 原始语义展示</span>
          <button
            type="button"
            class="prediction-research__button prediction-research__button--primary"
            @click="
              emit(
                'openInstrument',
                contractCode.toUpperCase().startsWith('US.')
                  ? contractCode
                  : `US.${contractCode}`,
                'prediction',
                'event_contract',
              )
            "
          >
            在交易工作区打开
          </button>
        </footer>
      </div>
    </template>

    <template v-else>
      <div class="prediction-research__parlay">
        <section>
          <h3>1. 选择至少两个合格合约</h3>
          <div class="prediction-research__leg-list">
            <label
              v-for="contract in parlayContracts"
              :key="contract.code"
              class="prediction-research__leg"
            >
              <input
                type="checkbox"
                :checked="selectedLegs[contract.code] != null"
                @change="toggleParlayContract(contract.code)"
              />
              <span class="prediction-research__leg-label">
                <strong>{{ contract.eventName }}</strong>
                <small>{{ contract.code }}</small>
              </span>
              <span
                v-if="selectedLegs[contract.code]"
                class="prediction-research__segments"
              >
                <button
                  type="button"
                  :class="{ 'is-active': parlaySide(contract.code) === 'YES' }"
                  @click.prevent="setParlaySide(contract.code, 'YES')"
                >
                  YES
                </button>
                <button
                  type="button"
                  :class="{ 'is-active': parlaySide(contract.code) === 'NO' }"
                  @click.prevent="setParlaySide(contract.code, 'NO')"
                >
                  NO
                </button>
              </span>
            </label>
          </div>
          <button
            type="button"
            class="prediction-research__button prediction-research__button--primary"
            :disabled="selectedLegCount < 2 || !mvc"
            @click="requestRFQ"
          >
            获取 RFQ（{{ selectedLegCount }} 腿）
          </button>
        </section>

        <section>
          <h3>2. 报价与提交</h3>
          <div v-if="quote" class="prediction-research__quote">
            <div>
              <span>Bid</span>
              <strong>{{ quote.metadata?.bidPrice ?? "—" }}</strong>
            </div>
            <div>
              <span>Ask</span>
              <strong>{{ quote.metadata?.askPrice ?? "—" }}</strong>
            </div>
            <div>
              <span>Quote ID</span>
              <strong>{{ quoteID }}</strong>
            </div>
            <div>
              <span>有效期</span>
              <strong>{{ quoteExpiresAt }}</strong>
            </div>
          </div>
          <div
            v-if="quote && quoteExpired"
            class="prediction-research__notice is-warning"
          >
            RFQ 已失效，必须重新询价。
          </div>

          <label class="prediction-research__field">
            <span>投入金额</span>
            <input v-model.number="amount" type="number" min="1" />
          </label>
          <label class="prediction-research__confirm">
            <input v-model="confirmed" type="checkbox" />
            <span>我确认腿、YES/NO 方向、投入金额和当前短时 RFQ</span>
          </label>

          <div v-if="preview" class="prediction-research__preview">
            <strong>预检通过</strong>
            <span>有效至 {{ preview.expiresAt ?? "—" }}</span>
            <span>购买力影响 {{ preview.buyingPowerImpact ?? "—" }}</span>
            <small v-for="warning in preview.warnings ?? []" :key="warning">
              {{ warning }}
            </small>
          </div>
          <div class="prediction-research__actions">
            <button
              type="button"
              class="prediction-research__button prediction-research__button--primary"
              :disabled="submitting || quoteExpired || !quoteID"
              @click="previewParlay"
            >
              预检
            </button>
            <button
              type="button"
              class="prediction-research__button prediction-research__button--primary"
              :disabled="
                submitting || !confirmed || quoteExpired || !preview?.previewId
              "
              @click="placeParlay"
            >
              提交 Parlay
            </button>
            <button
              v-if="execution?.internalOrderId"
              type="button"
              class="prediction-research__button"
              :disabled="submitting"
              @click="cancelParlay"
            >
              撤单
            </button>
          </div>
          <div
            v-if="execution"
            class="prediction-research__notice is-success"
          >
            {{ execution.orderStatus }} ·
            {{ execution.brokerOrderId ?? execution.internalOrderId }} ·
            {{ execution.message }}
          </div>
        </section>
      </div>
    </template>
  </section>
</template>

<style scoped>
.prediction-research {
  display: flex;
  height: 100%;
  min-height: 0;
  flex-direction: column;
  overflow: auto;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  font-size: 12px;
}

.prediction-research__header,
.prediction-research__breadcrumb {
  display: flex;
  min-height: 36px;
  align-items: center;
  gap: 8px;
  padding: 0 8px;
  border-bottom: 1px solid var(--tv-border);
}

.prediction-research__header {
  justify-content: space-between;
}

.prediction-research__eligibility,
.prediction-research__breadcrumb span,
.prediction-research__contract-footer span {
  color: var(--tv-text-dim);
  font-size: 11px;
}

.prediction-research__segments {
  display: inline-flex;
  align-items: center;
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-bg-surface-2);
}

.prediction-research__segments button {
  height: 26px;
  padding: 0 9px;
  border: 0;
  border-right: 1px solid var(--tv-border);
  background: transparent;
  color: var(--tv-text-dim);
  cursor: pointer;
  font: inherit;
  white-space: nowrap;
}

.prediction-research__segments button:last-child {
  border-right: 0;
}

.prediction-research__segments button.is-active {
  background: color-mix(in srgb, var(--tv-accent) 14%, transparent);
  color: var(--tv-accent);
  font-weight: 600;
}

.prediction-research__progress {
  position: relative;
  height: 2px;
  flex: 0 0 2px;
  overflow: hidden;
  background: color-mix(in srgb, var(--tv-accent) 18%, transparent);
}

.prediction-research__progress::after {
  position: absolute;
  width: 35%;
  height: 100%;
  animation: prediction-progress 1s linear infinite;
  background: var(--tv-accent);
  content: "";
}

@keyframes prediction-progress {
  from {
    transform: translateX(-100%);
  }
  to {
    transform: translateX(390%);
  }
}

.prediction-research__notice {
  margin: 8px;
  padding: 7px 9px;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-bg-surface-2);
}

.prediction-research__notice.is-warning {
  border-color: color-mix(in srgb, var(--tv-warn, #f5a623) 45%, var(--tv-border));
  color: var(--tv-warn, #f5a623);
}

.prediction-research__notice.is-success {
  border-color: color-mix(
    in srgb,
    var(--tv-status-success-fg) 45%,
    var(--tv-border)
  );
  color: var(--tv-status-success-fg);
}

.prediction-research__button {
  min-height: 26px;
  padding: 0 9px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
  cursor: pointer;
  font: inherit;
}

.prediction-research__button--primary {
  border-color: color-mix(in srgb, var(--tv-accent) 55%, var(--tv-border));
  color: var(--tv-accent);
}

.prediction-research__button:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.prediction-research__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 1px;
  padding: 1px;
  background: var(--tv-border);
}

.prediction-research__card {
  display: flex;
  min-height: 76px;
  flex-direction: column;
  gap: 4px;
  padding: 9px;
  border: 0;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  text-align: left;
  cursor: pointer;
}

.prediction-research__card:hover {
  background: var(--tv-bg-hover);
}

.prediction-research__card span,
.prediction-research__card small,
.prediction-research__leg small {
  color: var(--tv-text-dim);
}

.prediction-research__contract {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  gap: 8px;
  padding: 8px;
}

.prediction-research__segments--contract {
  align-self: flex-start;
}

.prediction-research__subscription {
  display: grid;
  min-height: 120px;
  flex: 1;
  place-items: center;
  color: var(--tv-text-dim);
}

.prediction-research__contract-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.prediction-research__parlay {
  display: grid;
  grid-template-columns: minmax(0, 1.5fr) minmax(300px, 1fr);
  gap: 8px;
  padding: 8px;
}

.prediction-research__parlay section {
  padding: 10px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface-2);
}

.prediction-research__parlay h3 {
  margin: 0 0 9px;
  font-size: 12px;
}

.prediction-research__leg-list {
  max-height: 420px;
  margin-bottom: 8px;
  overflow: auto;
}

.prediction-research__leg {
  display: grid;
  min-height: 36px;
  grid-template-columns: 20px 1fr auto;
  align-items: center;
  gap: 7px;
  border-bottom: 1px solid var(--tv-border);
  cursor: pointer;
}

.prediction-research__leg-label {
  display: flex;
  min-width: 0;
  flex-direction: column;
}

.prediction-research__quote {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 1px;
  margin-bottom: 8px;
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-border);
}

.prediction-research__quote div {
  display: flex;
  min-height: 52px;
  flex-direction: column;
  justify-content: center;
  gap: 3px;
  padding: 7px;
  background: var(--tv-bg-surface);
}

.prediction-research__quote span,
.prediction-research__field > span {
  color: var(--tv-text-dim);
  font-size: 10px;
}

.prediction-research__field {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin: 8px 0;
}

.prediction-research__field input {
  height: 28px;
  padding: 0 7px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  outline: 0;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  font: inherit;
}

.prediction-research__field input:focus {
  border-color: var(--tv-accent);
}

.prediction-research__confirm {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 8px 0;
}

.prediction-research__preview {
  display: flex;
  flex-direction: column;
  gap: 3px;
  margin: 8px 0;
  padding: 7px;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-bg-surface);
}

.prediction-research__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin: 8px 0;
}

@media (max-width: 960px) {
  .prediction-research__parlay {
    grid-template-columns: 1fr;
  }

  .prediction-research__eligibility {
    display: none;
  }
}
</style>
