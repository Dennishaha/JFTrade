<script setup lang="ts">
import { computed } from "vue";

import {
  GET_TECHNICAL_INDICATOR_OPTIONS,
  INDICATOR_TIMEFRAME_OPTIONS,
  MOVING_AVERAGE_INDICATOR_OPTIONS,
  getTechnicalIndicatorDefinition,
  nextGetTechnicalIndicatorNodeText,
  normalizeIndicatorTimeframe,
  type GetTechnicalIndicatorBlockProperties,
} from "../features/strategyVisualBuilderIndicatorBlock";
import { SERIES_SOURCE_OPTIONS } from "../features/strategyVisualBuilderCatalog";
import { suggestStrategyIndicatorVariableName } from "../features/strategyVisualBuilderIndicatorReferences";

interface StrategyIndicatorVariableItem {
  id: string;
  text: string;
  properties: GetTechnicalIndicatorBlockProperties;
}

const props = withDefaults(defineProps<{
  variables: StrategyIndicatorVariableItem[];
  selectedVariableId?: string;
}>(), {
  selectedVariableId: "",
});

const emit = defineEmits<{
  "add-variable": [];
  "delete-variable": [nodeId: string];
  "select-variable": [nodeId: string];
  "update-variable": [payload: { id: string; properties: Record<string, unknown> }];
}>();

const selectedVariable = computed(() => {
  if (props.variables.length === 0) {
    return null;
  }

  return props.variables.find((item) => item.id === props.selectedVariableId)
    ?? props.variables[0]
    ?? null;
});

const selectedIndicatorDefinition = computed(() =>
  getTechnicalIndicatorDefinition(selectedVariable.value?.properties.indicatorType ?? "rsi"),
);

const showsMovingAverageTypeInput = computed(
  () => selectedVariable.value?.properties.indicatorType === "movingAverage",
);

const showsWindowSizeInput = computed(
  () => selectedIndicatorDefinition.value.parameterShape === "windowSize",
);
const showsTimeframeInput = computed(() =>
  [
    "movingAverage",
    "rsi",
    "macd",
    "atr",
    "cci",
    "bollinger",
    "stdev",
    "variance",
    "highest",
    "lowest",
    "sum",
    "mfi",
    "supertrend",
    "linreg",
    "obv",
    "pivotHigh",
    "pivotLow",
    "keltner",
    "alma",
  ].includes(selectedVariable.value?.properties.indicatorType ?? ""),
);

const showsPeriodInput = computed(
  () => selectedIndicatorDefinition.value.parameterShape === "period"
    || selectedIndicatorDefinition.value.parameterShape === "bollinger"
    || selectedIndicatorDefinition.value.parameterShape === "kdj"
    || selectedIndicatorDefinition.value.parameterShape === "dmi"
    || selectedIndicatorDefinition.value.parameterShape === "supertrend"
    || selectedIndicatorDefinition.value.parameterShape === "linreg"
    || selectedIndicatorDefinition.value.parameterShape === "sourcePeriodMultiplier"
    || selectedIndicatorDefinition.value.parameterShape === "alma",
);

const showsMacdInputs = computed(
  () => selectedIndicatorDefinition.value.parameterShape === "macd",
);

const showsKdjInputs = computed(
  () => selectedIndicatorDefinition.value.parameterShape === "kdj",
);

const showsMultiplierInput = computed(
  () => selectedIndicatorDefinition.value.parameterShape === "bollinger"
    || selectedIndicatorDefinition.value.parameterShape === "sourcePeriodMultiplier",
);

const showsSourceInput = computed(() =>
  [
    "movingAverage",
    "cci",
    "stdev",
    "variance",
    "highest",
    "lowest",
    "sum",
    "vwap",
    "mfi",
    "linreg",
    "obv",
    "pivotHigh",
    "pivotLow",
    "keltner",
    "alma",
  ].includes(selectedVariable.value?.properties.indicatorType ?? ""),
);

const showsAdxSmoothingInput = computed(
  () => selectedIndicatorDefinition.value.parameterShape === "dmi",
);

const showsFactorInput = computed(
  () => selectedIndicatorDefinition.value.parameterShape === "supertrend",
);

const showsSarInputs = computed(
  () => selectedIndicatorDefinition.value.parameterShape === "sar",
);

const showsOffsetInput = computed(
  () => selectedIndicatorDefinition.value.parameterShape === "linreg"
    || selectedIndicatorDefinition.value.parameterShape === "alma",
);

const showsSigmaInput = computed(
  () => selectedIndicatorDefinition.value.parameterShape === "alma",
);

const showsPivotBarsInput = computed(
  () => selectedIndicatorDefinition.value.parameterShape === "pivot",
);

const indicatorSourceOptions = SERIES_SOURCE_OPTIONS;

const selectedVariablePlaceholder = computed(() => {
  if (selectedVariable.value === null) {
    return "";
  }

  return suggestStrategyIndicatorVariableName(
    selectedVariable.value.properties as unknown as Record<string, unknown>,
  );
});

const selectedVariableName = computed({
  get: () => selectedVariable.value?.properties.variableName ?? "",
  set: (value: string) => {
    mutateSelectedVariable((properties) => {
      const nextProperties = { ...properties };
      const normalized = value.trim();
      if (normalized === "") {
        delete nextProperties.variableName;
      } else {
        nextProperties.variableName = normalized;
      }
      return nextProperties;
    });
  },
});

const selectedIndicatorType = computed({
  get: () => selectedVariable.value?.properties.indicatorType ?? "rsi",
  set: (value: string) => {
    mutateSelectedVariable((properties) => ({
      ...properties,
      indicatorType: value,
    }));
  },
});

const selectedMovingAverageType = computed({
  get: () => selectedVariable.value?.properties.movingAverageType ?? "MA",
  set: (value: string) => {
    mutateSelectedVariable((properties) => ({
      ...properties,
      movingAverageType: value,
    }));
  },
});

const selectedTimeframe = computed({
  get: () => selectedVariable.value?.properties.timeframe ?? "",
  set: (value: string) => {
    mutateSelectedVariable((properties) => {
      const nextProperties = { ...properties };
      const timeframe = normalizeIndicatorTimeframe(value);
      if (timeframe === "") {
        delete nextProperties.timeframe;
      } else {
        nextProperties.timeframe = timeframe;
      }
      return nextProperties;
    });
  },
});

const selectedWindowSize = computed({
  get: () => readNumberString(selectedVariable.value?.properties.windowSize),
  set: (value: string) => {
    mutateSelectedVariable((properties) => ({
      ...properties,
      windowSize: normalizeInteger(value, 20),
    }));
  },
});

const selectedPeriod = computed({
  get: () => readNumberString(selectedVariable.value?.properties.period),
  set: (value: string) => {
    mutateSelectedVariable((properties) => ({
      ...properties,
      period: normalizeInteger(value, 14),
    }));
  },
});

const selectedFastPeriod = computed({
  get: () => readNumberString(selectedVariable.value?.properties.fastPeriod),
  set: (value: string) => {
    mutateSelectedVariable((properties) => ({
      ...properties,
      fastPeriod: normalizeInteger(value, 12),
    }));
  },
});

const selectedSlowPeriod = computed({
  get: () => readNumberString(selectedVariable.value?.properties.slowPeriod),
  set: (value: string) => {
    mutateSelectedVariable((properties) => ({
      ...properties,
      slowPeriod: normalizeInteger(value, 26),
    }));
  },
});

const selectedSignalPeriod = computed({
  get: () => readNumberString(selectedVariable.value?.properties.signalPeriod),
  set: (value: string) => {
    mutateSelectedVariable((properties) => ({
      ...properties,
      signalPeriod: normalizeInteger(value, 9),
    }));
  },
});

const selectedKdjM1 = computed({
  get: () => readNumberString(selectedVariable.value?.properties.m1),
  set: (value: string) => {
    mutateSelectedVariable((properties) => ({
      ...properties,
      m1: normalizeInteger(value, 3),
    }));
  },
});

const selectedKdjM2 = computed({
  get: () => readNumberString(selectedVariable.value?.properties.m2),
  set: (value: string) => {
    mutateSelectedVariable((properties) => ({
      ...properties,
      m2: normalizeInteger(value, 3),
    }));
  },
});

const selectedMultiplier = computed({
  get: () => readNumberString(selectedVariable.value?.properties.multiplier),
  set: (value: string) => {
    mutateSelectedVariable((properties) => ({
      ...properties,
      multiplier: normalizeDecimal(value, 2),
    }));
  },
});

const selectedSource = computed({
  get: () => selectedVariable.value?.properties.source ?? "close",
  set: (value: string) => {
    mutateSelectedVariable((properties) => ({
      ...properties,
      source: value,
    }));
  },
});

const selectedAdxSmoothing = computed({
  get: () => readNumberString(selectedVariable.value?.properties.adxSmoothing),
  set: (value: string) => {
    mutateSelectedVariable((properties) => ({
      ...properties,
      adxSmoothing: normalizeInteger(value, 14),
    }));
  },
});

const selectedFactor = computed({
  get: () => readNumberString(selectedVariable.value?.properties.factor),
  set: (value: string) => {
    mutateSelectedVariable((properties) => ({
      ...properties,
      factor: normalizeDecimal(value, 3),
    }));
  },
});

const selectedSarStart = computed({
  get: () => readNumberString(selectedVariable.value?.properties.start),
  set: (value: string) => {
    mutateSelectedVariable((properties) => ({
      ...properties,
      start: normalizeDecimal(value, 0.02),
    }));
  },
});

const selectedSarIncrement = computed({
  get: () => readNumberString(selectedVariable.value?.properties.increment),
  set: (value: string) => {
    mutateSelectedVariable((properties) => ({
      ...properties,
      increment: normalizeDecimal(value, 0.02),
    }));
  },
});

const selectedSarMaximum = computed({
  get: () => readNumberString(selectedVariable.value?.properties.maximum),
  set: (value: string) => {
    mutateSelectedVariable((properties) => ({
      ...properties,
      maximum: normalizeDecimal(value, 0.2),
    }));
  },
});

const selectedOffset = computed({
  get: () => readNumberString(selectedVariable.value?.properties.offset),
  set: (value: string) => {
    mutateSelectedVariable((properties) => ({
      ...properties,
      offset: normalizeDecimal(value, 0),
    }));
  },
});

const selectedSigma = computed({
  get: () => readNumberString(selectedVariable.value?.properties.sigma),
  set: (value: string) => {
    mutateSelectedVariable((properties) => ({
      ...properties,
      sigma: normalizeDecimal(value, 6),
    }));
  },
});

const selectedLeftBars = computed({
  get: () => readNumberString(selectedVariable.value?.properties.leftBars),
  set: (value: string) => {
    mutateSelectedVariable((properties) => ({
      ...properties,
      leftBars: normalizeInteger(value, 2),
    }));
  },
});

const selectedRightBars = computed({
  get: () => readNumberString(selectedVariable.value?.properties.rightBars),
  set: (value: string) => {
    mutateSelectedVariable((properties) => ({
      ...properties,
      rightBars: normalizeInteger(value, 2),
    }));
  },
});

const selectedVariablePreview = computed(() => {
  if (selectedVariable.value === null) {
    return "";
  }

  return nextGetTechnicalIndicatorNodeText(
    selectedVariable.value.properties as unknown as Record<string, unknown>,
  );
});

function mutateSelectedVariable(
  mutator: (properties: Record<string, unknown>) => Record<string, unknown>,
): void {
  if (selectedVariable.value === null) {
    return;
  }

  emit("update-variable", {
    id: selectedVariable.value.id,
    properties: mutator({ ...selectedVariable.value.properties }),
  });
}

function readNumberString(value: unknown): string {
  if (typeof value === "number" && Number.isFinite(value)) {
    return String(value);
  }
  if (typeof value === "string") {
    return value;
  }
  return "";
}

function normalizeInteger(value: string, fallback: number): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? Math.max(1, Math.round(parsed)) : fallback;
}

function normalizeDecimal(value: string, fallback: number): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : fallback;
}
</script>

<template>
  <div class="strategy-variable-manager">
    <div class="strategy-variable-manager__header">
      <div>
        <div class="strategy-variable-manager__title">变量管理</div>
        <div class="strategy-variable-manager__subtitle">
          变量会写入生成代码，并可被指标条件判断直接引用。
        </div>
      </div>
      <button
        class="strategy-variable-manager__add"
        data-testid="strategy-variable-add"
        type="button"
        @click="emit('add-variable')"
      >
        新增变量
      </button>
    </div>

    <div v-if="props.variables.length === 0" class="strategy-variable-manager__empty">
      暂无变量。新增一个指标变量后，就能在“指标条件判断”里直接绑定它。
    </div>

    <template v-else>
      <div class="strategy-variable-manager__workspace">
        <section class="strategy-variable-manager__pane strategy-variable-manager__pane--list">
          <div class="strategy-variable-manager__pane-head">
            <div class="strategy-variable-manager__pane-title">已有变量</div>
            <div class="strategy-variable-manager__pane-meta">{{ props.variables.length }} 个</div>
          </div>

          <div class="strategy-variable-manager__list">
            <div
              v-for="variable in props.variables"
              :key="variable.id"
              class="strategy-variable-manager__item"
              :class="{ 'is-active': variable.id === (selectedVariable?.id ?? '') }"
            >
              <button
                class="strategy-variable-manager__item-main"
                :data-testid="`strategy-variable-item-${variable.id}`"
                type="button"
                @click="emit('select-variable', variable.id)"
              >
                <span class="strategy-variable-manager__item-name">
                  {{ variable.properties.variableName || '未命名变量' }}
                </span>
                <span class="strategy-variable-manager__item-summary">{{ variable.text }}</span>
              </button>
              <button
                class="strategy-variable-manager__delete"
                :data-testid="`strategy-variable-delete-${variable.id}`"
                type="button"
                @click="emit('delete-variable', variable.id)"
              >
                删除
              </button>
            </div>
          </div>
        </section>

        <section v-if="selectedVariable !== null" class="strategy-variable-manager__pane strategy-variable-manager__pane--editor">
          <div class="strategy-variable-manager__pane-head">
            <div class="strategy-variable-manager__pane-title">变量设置</div>
            <div class="strategy-variable-manager__pane-meta">{{ selectedVariable.properties.variableName || selectedVariablePreview }}</div>
          </div>

          <div class="strategy-variable-manager__editor">
            <div class="strategy-variable-manager__preview">
              {{ selectedVariablePreview }}
            </div>

            <label class="strategy-variable-manager__field">
              <span>变量名</span>
              <input
                v-model="selectedVariableName"
                data-testid="strategy-variable-name-input"
                :placeholder="selectedVariablePlaceholder"
                type="text"
              />
            </label>

            <label class="strategy-variable-manager__field">
              <span>技术指标类型</span>
              <select v-model="selectedIndicatorType" data-testid="strategy-variable-indicator-type-select">
                <option v-for="option in GET_TECHNICAL_INDICATOR_OPTIONS" :key="option.value" :value="option.value">
                  {{ option.label }}
                </option>
              </select>
            </label>

            <label v-if="showsMovingAverageTypeInput" class="strategy-variable-manager__field">
              <span>均线指标类型</span>
              <select v-model="selectedMovingAverageType" data-testid="strategy-variable-moving-average-type-select">
                <option v-for="option in MOVING_AVERAGE_INDICATOR_OPTIONS" :key="option.value" :value="option.value">
                  {{ option.label }}
                </option>
              </select>
            </label>

            <label v-if="showsWindowSizeInput" class="strategy-variable-manager__field">
              <span>窗口</span>
              <input v-model="selectedWindowSize" data-testid="strategy-variable-window-size-input" min="1" step="1" type="number" />
            </label>

            <label v-if="showsTimeframeInput" class="strategy-variable-manager__field">
              <span>固定周期</span>
              <select v-model="selectedTimeframe" data-testid="strategy-variable-timeframe-select">
                <option v-for="option in INDICATOR_TIMEFRAME_OPTIONS" :key="option.value" :value="option.value">
                  {{ option.label }}
                </option>
              </select>
            </label>

            <label v-if="showsSourceInput" class="strategy-variable-manager__field">
              <span>数据源</span>
              <select v-model="selectedSource" data-testid="strategy-variable-source-select">
                <option v-for="option in indicatorSourceOptions" :key="option.value" :value="option.value">
                  {{ option.label }}
                </option>
              </select>
            </label>

            <label v-if="showsPeriodInput" class="strategy-variable-manager__field">
              <span>周期</span>
              <input v-model="selectedPeriod" data-testid="strategy-variable-period-input" min="1" step="1" type="number" />
            </label>

            <div v-if="showsMacdInputs" class="strategy-variable-manager__field-grid">
              <label class="strategy-variable-manager__field">
                <span>快线</span>
                <input v-model="selectedFastPeriod" data-testid="strategy-variable-fast-period-input" min="1" step="1" type="number" />
              </label>
              <label class="strategy-variable-manager__field">
                <span>慢线</span>
                <input v-model="selectedSlowPeriod" data-testid="strategy-variable-slow-period-input" min="1" step="1" type="number" />
              </label>
              <label class="strategy-variable-manager__field">
                <span>信号线</span>
                <input v-model="selectedSignalPeriod" data-testid="strategy-variable-signal-period-input" min="1" step="1" type="number" />
              </label>
            </div>

            <div v-if="showsKdjInputs" class="strategy-variable-manager__field-grid strategy-variable-manager__field-grid--kdj">
              <label class="strategy-variable-manager__field">
                <span>M1</span>
                <input v-model="selectedKdjM1" data-testid="strategy-variable-kdj-m1-input" min="1" step="1" type="number" />
              </label>
              <label class="strategy-variable-manager__field">
                <span>M2</span>
                <input v-model="selectedKdjM2" data-testid="strategy-variable-kdj-m2-input" min="1" step="1" type="number" />
              </label>
            </div>

            <label v-if="showsMultiplierInput" class="strategy-variable-manager__field">
              <span>乘数</span>
              <input v-model="selectedMultiplier" data-testid="strategy-variable-multiplier-input" min="0.1" step="0.1" type="number" />
            </label>

            <label v-if="showsAdxSmoothingInput" class="strategy-variable-manager__field">
              <span>ADX 平滑周期</span>
              <input v-model="selectedAdxSmoothing" data-testid="strategy-variable-adx-smoothing-input" min="1" step="1" type="number" />
            </label>

            <label v-if="showsFactorInput" class="strategy-variable-manager__field">
              <span>因子</span>
              <input v-model="selectedFactor" data-testid="strategy-variable-factor-input" min="0.01" step="0.01" type="number" />
            </label>

            <div v-if="showsSarInputs" class="strategy-variable-manager__field-grid">
              <label class="strategy-variable-manager__field">
                <span>起始</span>
                <input v-model="selectedSarStart" data-testid="strategy-variable-sar-start-input" min="0.01" step="0.01" type="number" />
              </label>
              <label class="strategy-variable-manager__field">
                <span>增量</span>
                <input v-model="selectedSarIncrement" data-testid="strategy-variable-sar-increment-input" min="0.01" step="0.01" type="number" />
              </label>
              <label class="strategy-variable-manager__field">
                <span>最大值</span>
                <input v-model="selectedSarMaximum" data-testid="strategy-variable-sar-maximum-input" min="0.01" step="0.01" type="number" />
              </label>
            </div>

            <div v-if="showsPivotBarsInput" class="strategy-variable-manager__field-grid strategy-variable-manager__field-grid--kdj">
              <label class="strategy-variable-manager__field">
                <span>左侧柱数</span>
                <input v-model="selectedLeftBars" data-testid="strategy-variable-left-bars-input" min="1" step="1" type="number" />
              </label>
              <label class="strategy-variable-manager__field">
                <span>右侧柱数</span>
                <input v-model="selectedRightBars" data-testid="strategy-variable-right-bars-input" min="1" step="1" type="number" />
              </label>
            </div>

            <label v-if="showsOffsetInput" class="strategy-variable-manager__field">
              <span>Offset</span>
              <input v-model="selectedOffset" data-testid="strategy-variable-offset-input" step="0.01" type="number" />
            </label>

            <label v-if="showsSigmaInput" class="strategy-variable-manager__field">
              <span>Sigma</span>
              <input v-model="selectedSigma" data-testid="strategy-variable-sigma-input" min="0.01" step="0.01" type="number" />
            </label>
          </div>
        </section>
      </div>
    </template>
  </div>
</template>

<style scoped>
.strategy-variable-manager {
  display: flex;
  min-height: 0;
  flex-direction: column;
  gap: 0.9rem;
}

.strategy-variable-manager__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.8rem;
}

.strategy-variable-manager__title {
  font-size: 0.95rem;
  font-weight: 700;
  color: var(--tv-text);
}

.strategy-variable-manager__subtitle {
  margin-top: 0.2rem;
  font-size: 0.78rem;
  line-height: 1.5;
  color: var(--tv-text-muted);
}

.strategy-variable-manager__add,
.strategy-variable-manager__delete,
.strategy-variable-manager__item-main,
.strategy-variable-manager__field input,
.strategy-variable-manager__field select {
  font: inherit;
}

.strategy-variable-manager__add,
.strategy-variable-manager__delete {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 2rem;
  padding: 0.4rem 0.8rem;
  border-radius: 999px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: color-mix(in srgb, var(--tv-bg-elevated) 42%, transparent);
  color: var(--tv-text);
  font-size: 0.78rem;
  font-weight: 700;
}

.strategy-variable-manager__empty {
  padding: 0.8rem 0.1rem 0.1rem;
  color: var(--tv-text-muted);
  font-size: 0.8rem;
  line-height: 1.6;
}

.strategy-variable-manager__workspace {
  display: grid;
  grid-template-columns: minmax(13rem, 15rem) minmax(19rem, 1fr);
  gap: 0.9rem;
  min-height: 0;
}

.strategy-variable-manager__pane {
  display: flex;
  min-height: 0;
  flex-direction: column;
  gap: 0.75rem;
  padding: 0.8rem;
  border-radius: 1.15rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: color-mix(in srgb, var(--tv-bg-elevated) 22%, transparent);
}

.strategy-variable-manager__pane--editor {
  min-width: 0;
  overflow: hidden;
}

.strategy-variable-manager__pane-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.75rem;
}

.strategy-variable-manager__pane-title {
  font-size: 0.8rem;
  font-weight: 700;
  letter-spacing: 0.08em;
  color: var(--tv-text);
}

.strategy-variable-manager__pane-meta {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.72rem;
  color: var(--tv-text-muted);
}

.strategy-variable-manager__list {
  display: grid;
  gap: 0.55rem;
  min-height: 0;
  max-height: 100%;
  overflow-y: auto;
  overscroll-behavior: contain;
  scrollbar-gutter: stable;
}

.strategy-variable-manager__item {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 0.55rem;
  align-items: stretch;
}

.strategy-variable-manager__item-main {
  display: grid;
  justify-items: start;
  gap: 0.14rem;
  min-width: 0;
  padding: 0.65rem 0.8rem;
  border-radius: 1rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: color-mix(in srgb, var(--tv-bg-elevated) 34%, transparent);
  color: var(--tv-text);
  text-align: left;
}

.strategy-variable-manager__item.is-active .strategy-variable-manager__item-main {
  border-color: color-mix(in srgb, var(--tv-accent) 55%, transparent);
  background: color-mix(in srgb, var(--tv-accent) 16%, transparent);
}

.strategy-variable-manager__item-name {
  min-width: 0;
  font-size: 0.84rem;
  font-weight: 700;
}

.strategy-variable-manager__item-summary {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.75rem;
  color: var(--tv-text-muted);
}

.strategy-variable-manager__editor {
  display: grid;
  gap: 0.8rem;
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
  scrollbar-gutter: stable;
  padding-right: 0.15rem;
}

.strategy-variable-manager__preview {
  padding: 0.6rem 0.8rem;
  border-radius: 0.95rem;
  background: color-mix(in srgb, var(--tv-accent) 12%, transparent);
  color: color-mix(in srgb, var(--tv-accent) 70%, var(--tv-text));
  font-size: 0.78rem;
  font-weight: 600;
}

.strategy-variable-manager__field {
  display: grid;
  gap: 0.35rem;
  color: var(--tv-text);
  font-size: 0.78rem;
}

.strategy-variable-manager__field span {
  font-weight: 600;
}

.strategy-variable-manager__field input,
.strategy-variable-manager__field select {
  min-height: 2.35rem;
  padding: 0.5rem 0.75rem;
  border-radius: 0.95rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: color-mix(in srgb, var(--tv-bg-elevated) 50%, transparent);
  color: var(--tv-text);
  outline: none;
}

.strategy-variable-manager__field-grid {
  display: grid;
  gap: 0.65rem;
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.strategy-variable-manager__field-grid--kdj {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

@media (max-width: 720px) {
  .strategy-variable-manager__workspace {
    grid-template-columns: 1fr;
  }

  .strategy-variable-manager__field-grid,
  .strategy-variable-manager__field-grid--kdj {
    grid-template-columns: 1fr;
  }
}
</style>
