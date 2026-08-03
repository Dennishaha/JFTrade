<script setup lang="ts">
import AppTabs from "@/components/shared/AppTabs.vue";
import PredictionContractDataView from "../research/PredictionContractDataView.vue";
import {
  usePredictionResearch,
  type PredictionResearchEmit,
  type PredictionResearchProps,
} from "@/composables/research/usePredictionResearch";

const props = withDefaults(defineProps<PredictionResearchProps>(), {
  presentation: "workspace",
  seriesCode: "",
  eventCode: "",
  contractCode: "",
  contractView: "snapshot",
});
const emit = defineEmits<PredictionResearchEmit>();

const {
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
} = usePredictionResearch(props, emit);

const researchModes = [
  { value: "discover", label: "事件与合约" },
  { value: "parlay", label: "Parlay 组合" },
] as const;
const contractViews = [
  { value: "snapshot", label: "快照" },
  { value: "depth", label: "YES/NO 盘口" },
  { value: "candles", label: "K 线" },
  { value: "ticks", label: "逐笔" },
  { value: "milestones", label: "里程碑" },
] as const;

function selectResearchMode(value: string): void {
  if (value !== "discover" && value !== "parlay") return;
  switchMode(value);
}

function selectDataView(value: string): void {
  if (!contractViews.some((item) => item.value === value)) return;
  selectContractView(value as (typeof contractViews)[number]["value"]);
}
</script>

<template>
  <section
    :class="[
      'prediction-research',
      `prediction-research--${presentation}`,
    ]"
  >
    <header class="prediction-research__header">
      <AppTabs class="prediction-research__segments" variant="compact" :model-value="mode"
        :items="researchModes" label="预测市场研究模式" @update:model-value="selectResearchMode" />
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
        <AppTabs
          class="prediction-research__segments prediction-research__segments--contract"
          variant="compact"
          :model-value="contractView"
          :items="contractViews"
          label="合约数据视图"
          @update:model-value="selectDataView"
        />

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
  font-size: var(--jf-text-6);
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
  font-size: var(--jf-text-5);
}

.prediction-research__segments {
  display: inline-flex;
  align-items: center;
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-bg-surface-2);
}

.prediction-research__segments :deep(.app-tabs__tab) {
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

.prediction-research__segments :deep(.app-tabs__tab:last-child) {
  border-right: 0;
}

.prediction-research__segments :deep(.app-tabs__tab.is-active) {
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
  border-color: color-mix(in srgb, var(--tv-warn) 45%, var(--tv-border));
  color: var(--tv-warn);
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
  font-size: var(--jf-text-6);
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
  font-size: var(--jf-text-4);
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
