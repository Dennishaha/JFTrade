<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";

import { fetchEnvelopeWithInit } from "../../composables/apiClient";
import { useBrokerProviderSelection } from "../../composables/brokerProviderSelection";
import {
  type OptionComboPriceSource,
  type OptionComboStrategy,
  type OptionContractChoice,
  useOptionComboDraftStore,
} from "../../composables/optionComboDraft";
import {
  buildOptionComboTemplate,
  optionComboLocalQuote,
  optionComboSpread,
  optionComboStrategyLabel,
  optionComboValidationMessage,
  recognizeOptionComboStrategy,
} from "../../composables/optionComboStrategies";
import { useConsoleData } from "../../composables/useConsoleData";
import OptionComboConfirmDialog from "./OptionComboConfirmDialog.vue";
import OptionComboLegEditor from "./OptionComboLegEditor.vue";
import OptionComboPreviewResult from "./OptionComboPreviewResult.vue";
import OptionComboRiskStrip from "./OptionComboRiskStrip.vue";

export type { OptionContractChoice } from "../../composables/optionComboDraft";

interface ComboAccountImpact {
  nlvChange?: number | null;
  initialMarginChange?: number | null;
  maintenanceMarginChange?: number | null;
  optionBuyingPower?: number | null;
  maxWithdrawalChange?: number | null;
  buyingPowerDecrease?: number | null;
}

interface ComboAnalysis {
  bid?: number | null;
  ask?: number | null;
  maxProfit?: number | null;
  maxLoss?: number | null;
  maxProfitUnlimited?: boolean;
  maxLossUnlimited?: boolean;
  breakevenPoints?: number[];
  probability?: number | null;
  delta?: number | null;
  theta?: number | null;
}

interface ComboPreview {
  previewId: string;
  allowed?: boolean;
  buyingPowerImpact?: number | null;
  accountImpact?: ComboAccountImpact | null;
  warnings?: string[];
  expiresAt?: string;
  optionAnalysis?: ComboAnalysis | null;
}

interface ExecutionResponse {
  accepted: boolean;
  internalOrderId?: string;
  brokerOrderId?: string;
  orderStatus?: string;
  message?: string;
}

const props = withDefaults(
  defineProps<{
    instrumentId?: string;
    market?: string;
    contracts?: OptionContractChoice[];
    defaultExpiry?: string;
    underlyingPrice?: number | null;
  }>(),
  {
    instrumentId: "",
    market: "",
    contracts: () => [],
    defaultExpiry: "",
    underlyingPrice: null,
  },
);
const emit = defineEmits<{ submitted: [internalOrderId: string] }>();

const draft = useOptionComboDraftStore();
const {
  realTradeApprovals,
  selectedBrokerAccount,
  systemStatus,
} = useConsoleData();
const { selectedBrokerId } = useBrokerProviderSelection();
const selectedTemplate = ref<OptionComboStrategy>("vertical");
const preview = ref<ComboPreview | null>(null);
const execution = ref<ExecutionResponse | null>(null);
const loading = ref(false);
const error = ref("");
const confirmationMode = ref<"place" | "cancel" | null>(null);
const stableClientOrderId = ref("");
const now = ref(Date.now());
let countdownTimer: ReturnType<typeof setInterval> | undefined;
let submissionStarted = false;

const legs = computed(() => draft.legs.value);
const contracts = computed(() => draft.contracts.value);
const instrumentId = computed(
  () => props.instrumentId || draft.underlyingInstrumentId.value,
);
const market = computed(() => props.market || draft.market.value);
const quantity = computed({
  get: () => draft.quantity.value,
  set: (value: number) => {
    draft.quantity.value = Math.max(1, Math.round(Number(value) || 1));
  },
});
const comboPrice = computed({
  get: () => draft.comboPrice.value,
  set: (value: number) => {
    draft.comboPrice.value = Math.max(0, Number(value) || 0);
  },
});
const priceSource = computed({
  get: () => draft.priceSource.value,
  set: (value: OptionComboPriceSource) => {
    draft.priceSource.value = value;
  },
});
const recognizedStrategy = computed(() =>
  recognizeOptionComboStrategy(legs.value),
);
const strategyLabel = computed(() =>
  optionComboStrategyLabel(recognizedStrategy.value),
);
const strategyError = computed(() =>
  optionComboValidationMessage(legs.value),
);
const localQuote = computed(() => optionComboLocalQuote(legs.value));
const expiries = computed(() =>
  [...new Set(legs.value.map((leg) => leg.expiry).filter(Boolean))].sort(),
);
const nearExpiry = computed(
  () => expiries.value[0] ?? props.defaultExpiry ?? "",
);
const farExpiry = computed(() => expiries.value[1] ?? "");
const spread = computed(() =>
  optionComboSpread(recognizedStrategy.value, legs.value),
);
const brokerId = computed(
  () =>
    selectedBrokerAccount.value?.brokerId ||
    selectedBrokerId.value ||
    systemStatus.value.defaultBroker,
);
const environment = computed(
  () =>
    selectedBrokerAccount.value?.tradingEnvironment ??
    systemStatus.value.defaultTradingEnvironment,
);
const accountId = computed(
  () => selectedBrokerAccount.value?.accountId ?? "",
);
const accountLabel = computed(
  () =>
    selectedBrokerAccount.value?.displayName ||
    selectedBrokerAccount.value?.accountId ||
    brokerId.value,
);
const isReal = computed(() => environment.value === "REAL");
const requiredConfirmationText = computed(
  () =>
    realTradeApprovals?.value?.requiredConfirmationText?.trim() ||
    "ENABLE_REAL_TRADING",
);
const previewExpiryMs = computed(() => {
  const value = Date.parse(preview.value?.expiresAt ?? "");
  return Number.isFinite(value) ? value : null;
});
const remainingSeconds = computed(() => {
  if (preview.value == null || previewExpiryMs.value == null) return null;
  return Math.max(0, Math.ceil((previewExpiryMs.value - now.value) / 1000));
});
const previewExpired = computed(() => remainingSeconds.value === 0);
const canPreview = computed(
  () =>
    !loading.value &&
    strategyError.value === "" &&
    Number.isInteger(quantity.value) &&
    quantity.value > 0 &&
    comboPrice.value > 0,
);
const canPlace = computed(
  () =>
    canPreview.value &&
    preview.value != null &&
    !previewExpired.value &&
    execution.value == null &&
    !submissionStarted,
);
const strategyItems: Array<{ value: OptionComboStrategy; label: string }> = [
  { value: "vertical", label: "垂直价差" },
  { value: "straddle", label: "跨式" },
  { value: "strangle", label: "宽跨式" },
  { value: "calendar", label: "日历价差" },
  { value: "butterfly", label: "蝶式" },
];

function invalidatePreview(resetClientOrderId = true): void {
  preview.value = null;
  execution.value = null;
  confirmationMode.value = null;
  error.value = "";
  submissionStarted = false;
  if (resetClientOrderId) stableClientOrderId.value = "";
}

function applyPrice(source: Exclude<OptionComboPriceSource, "custom">): void {
  priceSource.value = source;
  const value = localQuote.value[source];
  if (value != null) comboPrice.value = Number(value.toFixed(4));
}

function updateManualPrice(value: string): void {
  priceSource.value = "custom";
  comboPrice.value = Number(value);
}

function addContract(value: string): void {
  const contract = contracts.value.find(
    (candidate) =>
      candidate.instrumentId.trim().toUpperCase() ===
      value.trim().toUpperCase(),
  );
  if (contract != null) draft.addLeg(contract);
}

function applyTemplate(): void {
  const next = buildOptionComboTemplate(
    selectedTemplate.value,
    contracts.value,
    props.defaultExpiry,
    props.underlyingPrice,
  );
  if (next.length === 0) {
    error.value = "当前期权链没有足够的合约生成该策略。";
    return;
  }
  error.value = "";
  draft.replaceLegs(next);
}

function clientOrderId(): string {
  if (stableClientOrderId.value) return stableClientOrderId.value;
  const suffix =
    typeof crypto !== "undefined" && typeof crypto.randomUUID === "function"
      ? crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  stableClientOrderId.value = `jftrade-option-combo-${suffix}`;
  return stableClientOrderId.value;
}

function executionLegs(): Array<Record<string, unknown>> {
  return legs.value.map((leg) => ({
    instrumentId: leg.instrumentId,
    productClass: "option",
    side: leg.side,
    ratio: leg.ratio,
    quantity: quantity.value * leg.ratio,
  }));
}

function comboPayload(): Record<string, unknown> {
  return {
    brokerId: brokerId.value,
    tradingEnvironment: environment.value,
    accountId: accountId.value,
    market: market.value,
    clientOrderId: clientOrderId(),
    orderKind: "option_combo",
    productClass: "option",
    underlyingInstrumentId: instrumentId.value,
    optionStrategy: recognizedStrategy.value,
    nearExpiry: nearExpiry.value,
    farExpiry: farExpiry.value || undefined,
    spread: spread.value,
    price: comboPrice.value,
    legs: executionLegs(),
  };
}

async function previewCombo(): Promise<void> {
  if (!canPreview.value) return;
  loading.value = true;
  error.value = "";
  try {
    preview.value = await fetchEnvelopeWithInit<ComboPreview>(
      "/api/v1/execution/combos/previews",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(comboPayload()),
      },
    );
    execution.value = null;
    now.value = Date.now();
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : String(cause);
    preview.value = null;
  } finally {
    loading.value = false;
  }
}

function openPlaceConfirmation(): void {
  if (canPlace.value) confirmationMode.value = "place";
}

async function placeCombo(): Promise<void> {
  if (!canPlace.value || preview.value == null) return;
  submissionStarted = true;
  loading.value = true;
  confirmationMode.value = null;
  error.value = "";
  try {
    execution.value = await fetchEnvelopeWithInit<ExecutionResponse>(
      "/api/v1/execution/combos",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          ...comboPayload(),
          previewId: preview.value.previewId,
        }),
      },
    );
    const internalOrderId = execution.value.internalOrderId ?? "";
    if (internalOrderId) {
      draft.submittedOrderId.value = internalOrderId;
      emit("submitted", internalOrderId);
    }
  } catch (cause) {
    submissionStarted = false;
    error.value = cause instanceof Error ? cause.message : String(cause);
  } finally {
    loading.value = false;
  }
}

async function cancelCombo(): Promise<void> {
  const internalOrderId = execution.value?.internalOrderId;
  if (!internalOrderId || loading.value) return;
  loading.value = true;
  confirmationMode.value = null;
  error.value = "";
  try {
    execution.value = await fetchEnvelopeWithInit<ExecutionResponse>(
      `/api/v1/execution/combos/${encodeURIComponent(internalOrderId)}/cancel`,
      { method: "POST" },
    );
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : String(cause);
  } finally {
    loading.value = false;
  }
}

watch(
  () => [props.instrumentId, props.market],
  ([nextInstrumentId, nextMarket]) => {
    if (nextInstrumentId) {
      draft.setContext(nextInstrumentId ?? "", nextMarket ?? "");
    }
  },
  { immediate: true },
);
watch(
  () => props.contracts,
  (next) => {
    if (next.length > 0) draft.setContracts(next);
  },
  { immediate: true },
);
watch(
  () => draft.revision.value,
  () => {
    invalidatePreview();
    if (priceSource.value !== "custom") applyPrice(priceSource.value);
  },
);
watch(
  () => [quantity.value, comboPrice.value, brokerId.value, accountId.value, environment.value],
  () => invalidatePreview(),
);

countdownTimer = setInterval(() => {
  now.value = Date.now();
}, 1_000);
onBeforeUnmount(() => {
  if (countdownTimer != null) clearInterval(countdownTimer);
});
</script>

<template>
  <section class="option-combo-ticket">
    <div class="option-combo-ticket__main">
      <div class="option-combo-ticket__left tv-scrollbar">
        <header class="option-combo-ticket__toolbar">
          <div class="option-combo-ticket__heading">
            <strong>{{ strategyLabel }}</strong>
            <small>{{ legs.length }} 腿 · 限价 · 当日有效</small>
          </div>
          <div class="option-combo-ticket__tools">
            <select v-model="selectedTemplate" aria-label="组合策略模板">
              <option v-for="item in strategyItems" :key="item.value" :value="item.value">
                {{ item.label }}
              </option>
            </select>
            <button type="button" @click="applyTemplate">生成</button>
            <button
              type="button"
              :disabled="legs.length === 0"
              title="反向全部期权腿"
              @click="draft.reverseLegs()"
            >
              全部反向
            </button>
            <button
              type="button"
              :disabled="!draft.canUndo.value"
              aria-label="撤销选腿"
              title="撤销"
              @click="draft.undo()"
            >
              ↶
            </button>
            <button
              type="button"
              :disabled="!draft.canRedo.value"
              aria-label="重做选腿"
              title="重做"
              @click="draft.redo()"
            >
              ↷
            </button>
            <button
              type="button"
              :disabled="legs.length === 0"
              title="清空全部期权腿"
              @click="draft.clearLegs()"
            >
              清空
            </button>
          </div>
        </header>
        <div class="option-combo-ticket__legs">
          <OptionComboLegEditor
            :legs="legs"
            :contracts="contracts"
            @add="addContract"
            @remove="draft.removeLeg"
            @update="draft.updateLeg"
          />
        </div>
      </div>

      <aside class="option-combo-ticket__order tv-scrollbar">
        <div class="option-combo-ticket__scope">
          <strong>{{ accountLabel }}</strong>
          <span :class="{ 'is-real': isReal }">{{ environment }}</span>
        </div>
        <label>订单类型 <strong>限价 · 当日有效</strong></label>
        <div class="option-combo-ticket__anchors">
          <button type="button" :disabled="localQuote.bid == null" :class="{ 'is-active': priceSource === 'bid' }" @click="applyPrice('bid')">
            买一 <strong>{{ localQuote.bid ?? "—" }}</strong>
          </button>
          <button type="button" :disabled="localQuote.mid == null" :class="{ 'is-active': priceSource === 'mid' }" @click="applyPrice('mid')">
            中间价 <strong>{{ localQuote.mid?.toFixed(3) ?? "—" }}</strong>
          </button>
          <button type="button" :disabled="localQuote.ask == null" :class="{ 'is-active': priceSource === 'ask' }" @click="applyPrice('ask')">
            卖一 <strong>{{ localQuote.ask ?? "—" }}</strong>
          </button>
        </div>
        <label>
          组合限价
          <input
            :value="comboPrice"
            type="number"
            min="0.001"
            step="0.001"
            aria-label="组合限价"
            @input="updateManualPrice(($event.target as HTMLInputElement).value)"
          />
        </label>
        <label>
          组合数量
          <span class="option-combo-ticket__stepper">
            <button type="button" :disabled="quantity <= 1" aria-label="减少组合数量" @click="quantity -= 1">−</button>
            <input v-model.number="quantity" type="number" min="1" step="1" aria-label="组合数量" />
            <button type="button" aria-label="增加组合数量" @click="quantity += 1">+</button>
          </span>
        </label>
        <div class="option-combo-ticket__primary-actions">
          <button type="button" :disabled="!canPreview" @click="previewCombo">
            {{ loading && preview == null ? "预检中…" : "预检组合" }}
          </button>
          <button type="button" class="is-primary" :disabled="!canPlace" @click="openPlaceConfirmation">
            确认下单
          </button>
        </div>
      </aside>
    </div>

    <p v-if="legs.length > 0 && strategyError" class="option-combo-ticket__message">
      {{ strategyError }}
    </p>
    <p v-if="error" class="option-combo-ticket__message is-error">{{ error }}</p>
    <OptionComboRiskStrip :analysis="preview?.optionAnalysis ?? null" />
    <OptionComboPreviewResult
      v-if="preview"
      :account-impact="preview.accountImpact ?? null"
      :buying-power-impact="preview.buyingPowerImpact ?? null"
      :warnings="preview.warnings ?? []"
      :remaining-seconds="remainingSeconds"
    />
    <footer v-if="execution" class="option-combo-ticket__execution">
      <span>
        {{ execution.orderStatus ?? (execution.accepted ? "已提交" : "未接受") }}
        · {{ execution.brokerOrderId ?? execution.internalOrderId }}
        · {{ execution.message }}
      </span>
      <button
        v-if="execution.internalOrderId"
        type="button"
        :disabled="loading"
        @click="confirmationMode = 'cancel'"
      >
        撤单
      </button>
    </footer>

    <OptionComboConfirmDialog
      :open="confirmationMode != null"
      :mode="confirmationMode ?? 'place'"
      :account-label="accountLabel"
      :environment="environment"
      :strategy-label="strategyLabel"
      :legs="legs"
      :price="comboPrice"
      :quantity="quantity"
      :real-confirmation-required="isReal && confirmationMode === 'place'"
      :required-confirmation-text="requiredConfirmationText"
      @close="confirmationMode = null"
      @confirm="confirmationMode === 'cancel' ? cancelCombo() : placeCombo()"
    />
  </section>
</template>

<style scoped src="./OptionComboBuilder.css"></style>
