<script setup lang="ts">
import {
  computed,
  nextTick,
  onBeforeUnmount,
  ref,
  watch,
  type CSSProperties,
} from "vue";

import type { InstrumentResolutionCandidate } from "@/contracts";
import {
  categoryMarketForUser,
  formatInstrumentExchangeTag,
  formatInstrumentSecurityTypeLabel,
  formatUserMarketLabel,
  normalizeInstrumentSecurityType,
} from "@/composables/instrumentPresentation";
import { useInstrumentResolver } from "@/composables/instrumentResolver";

import InstrumentIdentity from "./InstrumentIdentity.vue";

const props = withDefaults(
  defineProps<{
    modelValue: string;
    placeholder?: string;
    actionLabel?: string;
    variant?: "topbar" | "backtest" | "dialog";
    rootTestId?: string;
    inputTestId?: string;
    submitTestId?: string;
  }>(),
  {
    placeholder: "输入代码或名称",
    actionLabel: "查询",
    variant: "backtest",
  },
);

const emit = defineEmits<{
  "update:modelValue": [value: string];
  select: [candidate: InstrumentResolutionCandidate];
  paste: [event: ClipboardEvent];
  "delimiter-keydown": [event: KeyboardEvent];
  blur: [event: FocusEvent];
}>();

const selectedMarketCategory = ref("");
const selectedSecurityType = ref("");
const rootElement = ref<HTMLElement | null>(null);
const panelPlacement = ref<CSSProperties>({});

const PANEL_GAP = 6;
const PANEL_MIN_WIDTH = 480;
const PANEL_MAX_HEIGHT = 560;
const VIEWPORT_MARGIN = 12;

function updateValue(event: Event): void {
  emit("update:modelValue", (event.target as HTMLInputElement).value);
}

function handleResolved(candidate: InstrumentResolutionCandidate): void {
  emit("select", candidate);
}

const {
  loading,
  panelOpen,
  candidates,
  failures,
  resolutionStatus,
  resolutionError,
  statusMessage,
  activeCandidateIndex,
  resolve,
  closePanel,
  selectCandidate,
} = useInstrumentResolver({
  market: "",
  query: () => props.modelValue,
  onResolved: handleResolved,
});

interface FilterOption {
  value: string;
  label: string;
}

function marketCategory(candidate: InstrumentResolutionCandidate): string {
  return categoryMarketForUser(candidate.market) || candidate.market.trim().toUpperCase();
}

function securityTypeCategory(candidate: InstrumentResolutionCandidate): string {
  return normalizeInstrumentSecurityType(candidate.securityType) || "UNKNOWN";
}

function uniqueOptions(
  valueFor: (candidate: InstrumentResolutionCandidate) => string,
  labelFor: (candidate: InstrumentResolutionCandidate) => string,
): FilterOption[] {
  const options: FilterOption[] = [];
  const seen = new Set<string>();
  for (const candidate of candidates.value) {
    const value = valueFor(candidate);
    if (value === "" || seen.has(value)) {
      continue;
    }
    seen.add(value);
    options.push({ value, label: labelFor(candidate) });
  }
  return options;
}

const marketFilterOptions = computed(() =>
  uniqueOptions(
    marketCategory,
    (candidate) =>
      formatInstrumentExchangeTag(marketCategory(candidate)) ??
      formatUserMarketLabel(marketCategory(candidate)),
  ),
);

const securityTypeFilterOptions = computed(() =>
  uniqueOptions(securityTypeCategory, (candidate) =>
    formatInstrumentSecurityTypeLabel(candidate.securityType),
  ),
);

const filteredCandidateEntries = computed(() =>
  candidates.value
    .map((candidate, index) => ({ candidate, index }))
    .filter(
      ({ candidate }) =>
        (selectedMarketCategory.value === "" ||
          marketCategory(candidate) === selectedMarketCategory.value) &&
        (selectedSecurityType.value === "" ||
          securityTypeCategory(candidate) === selectedSecurityType.value),
    ),
);

const visibleSelectableIndexes = computed(() =>
  filteredCandidateEntries.value
    .filter(({ candidate }) => candidate.selectable)
    .map(({ index }) => index),
);

const panelId = computed(() =>
  props.rootTestId == null ? undefined : `${props.rootTestId}-results`,
);

watch(
  () => props.modelValue,
  () => {
    selectedMarketCategory.value = "";
    selectedSecurityType.value = "";
  },
);

watch(candidates, () => {
  selectedMarketCategory.value = "";
  selectedSecurityType.value = "";
});

watch([selectedMarketCategory, selectedSecurityType], () => {
  if (!panelOpen.value) {
    return;
  }
  activeCandidateIndex.value = visibleSelectableIndexes.value[0] ?? -1;
});

function viewportBounds(): {
  left: number;
  top: number;
  width: number;
  height: number;
} {
  const visualViewport = window.visualViewport;
  return {
    left: visualViewport?.offsetLeft ?? 0,
    top: visualViewport?.offsetTop ?? 0,
    width:
      visualViewport?.width ||
      document.documentElement.clientWidth ||
      window.innerWidth,
    height:
      visualViewport?.height ||
      document.documentElement.clientHeight ||
      window.innerHeight,
  };
}

function updatePanelPlacement(): void {
  if (!panelOpen.value || rootElement.value == null) {
    return;
  }
  const rootBounds = rootElement.value.getBoundingClientRect();
  const viewport = viewportBounds();
  const viewportRight = viewport.left + viewport.width;
  const viewportBottom = viewport.top + viewport.height;
  const availableWidth = Math.max(0, viewport.width - VIEWPORT_MARGIN * 2);
  const width = Math.min(
    Math.max(rootBounds.width, PANEL_MIN_WIDTH),
    availableWidth,
  );
  const minLeft = viewport.left + VIEWPORT_MARGIN;
  const maxLeft = Math.max(
    minLeft,
    viewportRight - VIEWPORT_MARGIN - width,
  );
  const left = Math.min(Math.max(rootBounds.left, minLeft), maxLeft);
  const spaceBelow = Math.max(
    0,
    viewportBottom - VIEWPORT_MARGIN - rootBounds.bottom - PANEL_GAP,
  );
  const spaceAbove = Math.max(
    0,
    rootBounds.top - viewport.top - VIEWPORT_MARGIN - PANEL_GAP,
  );
  const opensAbove = spaceBelow < 280 && spaceAbove > spaceBelow;
  const availableHeight = opensAbove ? spaceAbove : spaceBelow;
  const maxHeight = Math.min(
    PANEL_MAX_HEIGHT,
    viewport.height * 0.82,
    availableHeight,
  );

  panelPlacement.value = {
    left: `${Math.round(left)}px`,
    width: `${Math.round(width)}px`,
    minWidth: `${Math.round(width)}px`,
    maxHeight: `${Math.max(0, Math.round(maxHeight))}px`,
    top: opensAbove ? "auto" : `${Math.round(rootBounds.bottom + PANEL_GAP)}px`,
    bottom: opensAbove
      ? `${Math.round(window.innerHeight - rootBounds.top + PANEL_GAP)}px`
      : "auto",
  };
}

function removeViewportListeners(): void {
  window.removeEventListener("resize", updatePanelPlacement);
  window.removeEventListener("scroll", updatePanelPlacement, true);
  window.visualViewport?.removeEventListener("resize", updatePanelPlacement);
  window.visualViewport?.removeEventListener("scroll", updatePanelPlacement);
}

function addViewportListeners(): void {
  removeViewportListeners();
  window.addEventListener("resize", updatePanelPlacement);
  window.addEventListener("scroll", updatePanelPlacement, true);
  window.visualViewport?.addEventListener("resize", updatePanelPlacement);
  window.visualViewport?.addEventListener("scroll", updatePanelPlacement);
}

watch(panelOpen, async (open) => {
  if (!open) {
    removeViewportListeners();
    return;
  }
  await nextTick();
  updatePanelPlacement();
  addViewportListeners();
});

onBeforeUnmount(removeViewportListeners);

function submit(): void {
  void resolve();
}

function moveActiveCandidate(offset: -1 | 1): void {
  const indexes = visibleSelectableIndexes.value;
  if (indexes.length === 0) {
    activeCandidateIndex.value = -1;
    return;
  }
  const current = indexes.indexOf(activeCandidateIndex.value);
  const base = current < 0 ? (offset > 0 ? -1 : 0) : current;
  const next = (base + offset + indexes.length) % indexes.length;
  activeCandidateIndex.value = indexes[next] ?? -1;
}

function selectActiveCandidate(): boolean {
  const candidate = candidates.value[activeCandidateIndex.value];
  if (candidate == null || !candidate.selectable) {
    return false;
  }
  selectCandidate(candidate);
  return true;
}

function handleKeydown(event: KeyboardEvent): void {
  if (event.isComposing) {
    return;
  }
  if (event.key === "Escape" && panelOpen.value) {
    event.preventDefault();
    closePanel();
    return;
  }
  if (event.key === "ArrowDown" && panelOpen.value) {
    event.preventDefault();
    moveActiveCandidate(1);
    return;
  }
  if (event.key === "ArrowUp" && panelOpen.value) {
    event.preventDefault();
    moveActiveCandidate(-1);
    return;
  }
  if (event.key === "Enter") {
    event.preventDefault();
    if (!panelOpen.value || !selectActiveCandidate()) {
      submit();
    }
    return;
  }
  if (
    event.key === "," ||
    event.key === "Tab" ||
    (event.key === "Backspace" && props.modelValue.trim() === "")
  ) {
    emit("delimiter-keydown", event);
  }
}

function candidateOptionId(index: number): string | undefined {
  return panelId.value == null ? undefined : `${panelId.value}-option-${index}`;
}
</script>

<template>
  <div
    ref="rootElement"
    class="instrument-search-box"
    :class="[
      `instrument-search-box--${variant}`,
      { 'tv-topbar-symbol': variant === 'topbar' },
    ]"
    :data-testid="rootTestId"
  >
    <div class="instrument-search-box__control">
      <span v-if="variant === 'topbar'" class="instrument-search-box__leading" aria-hidden="true">⌕</span>
      <input
        :value="modelValue"
        :placeholder="placeholder"
        :data-testid="inputTestId"
        :aria-controls="panelId"
        :aria-expanded="panelOpen"
        :aria-activedescendant="activeCandidateIndex >= 0 ? candidateOptionId(activeCandidateIndex) : undefined"
        autocomplete="off"
        enterkeyhint="search"
        role="combobox"
        spellcheck="false"
        type="search"
        @input="updateValue"
        @keydown="handleKeydown"
        @paste="emit('paste', $event)"
        @blur="emit('blur', $event)"
      >
      <button
        type="button"
        class="instrument-search-box__submit"
        :disabled="loading"
        :aria-label="loading ? '正在查询标的' : actionLabel"
        :title="loading ? '正在查询标的' : actionLabel"
        :data-testid="submitTestId"
        @mousedown.prevent
        @click="submit"
      >
        <template v-if="variant === 'dialog'">
          {{ loading ? "查询中" : actionLabel }}
        </template>
        <template v-else-if="variant === 'topbar'">
          <span class="instrument-search-box__submit-shortcut" aria-hidden="true">
            {{ loading ? "···" : "⏎" }}
          </span>
          <span class="instrument-search-box__submit-label">
            {{ loading ? "查询中" : actionLabel }}
          </span>
        </template>
        <span
          v-else
          aria-hidden="true"
          :class="loading ? 'fa-solid fa-spinner fa-spin' : 'fa-solid fa-magnifying-glass'"
        />
      </button>
    </div>

    <Teleport to="body">
      <div
        v-if="panelOpen"
        class="instrument-search-box__panel"
        :class="{ 'instrument-search-box__panel--warning': resolutionStatus === 'incomplete' }"
        :style="panelPlacement"
      >
        <div v-if="statusMessage" class="instrument-search-box__message" role="status">
          {{ statusMessage }}
        </div>
        <div v-if="failures.length" class="instrument-search-box__failures">
          <div v-for="failure in failures" :key="`${failure.market}:${failure.code}`">
            {{ formatInstrumentExchangeTag(failure.market) ?? formatUserMarketLabel(failure.market) }}：{{ failure.message }}
          </div>
        </div>

        <div
          v-if="marketFilterOptions.length > 1 || securityTypeFilterOptions.length > 1"
          class="instrument-search-box__filters"
        >
          <div v-if="marketFilterOptions.length > 1" class="instrument-search-box__filter-row">
            <span class="instrument-search-box__filter-label">市场</span>
            <div class="instrument-search-box__filter-scroll" data-testid="instrument-search-market-filters">
              <button
                type="button"
                class="instrument-search-box__filter-tag"
                :class="{ 'is-active': selectedMarketCategory === '' }"
                :aria-pressed="selectedMarketCategory === ''"
                @click="selectedMarketCategory = ''"
              >
                全部
              </button>
              <button
                v-for="option in marketFilterOptions"
                :key="option.value"
                type="button"
                class="instrument-search-box__filter-tag"
                :class="{ 'is-active': selectedMarketCategory === option.value }"
                :aria-pressed="selectedMarketCategory === option.value"
                :data-filter-value="option.value"
                @click="selectedMarketCategory = option.value"
              >
                {{ option.label }}
              </button>
            </div>
          </div>
          <div v-if="securityTypeFilterOptions.length > 1" class="instrument-search-box__filter-row">
            <span class="instrument-search-box__filter-label">类型</span>
            <div class="instrument-search-box__filter-scroll" data-testid="instrument-search-type-filters">
              <button
                type="button"
                class="instrument-search-box__filter-tag"
                :class="{ 'is-active': selectedSecurityType === '' }"
                :aria-pressed="selectedSecurityType === ''"
                @click="selectedSecurityType = ''"
              >
                全部
              </button>
              <button
                v-for="option in securityTypeFilterOptions"
                :key="option.value"
                type="button"
                class="instrument-search-box__filter-tag"
                :class="{ 'is-active': selectedSecurityType === option.value }"
                :aria-pressed="selectedSecurityType === option.value"
                :data-filter-value="option.value"
                @click="selectedSecurityType = option.value"
              >
                {{ option.label }}
              </button>
            </div>
          </div>
        </div>

        <div
          :id="panelId"
          class="instrument-search-box__results"
          data-testid="instrument-search-results"
          role="listbox"
        >
          <div v-if="filteredCandidateEntries.length === 0 && candidates.length > 0" class="instrument-search-box__empty">
            当前筛选下暂无结果
          </div>
          <button
            v-for="entry in filteredCandidateEntries"
            :id="candidateOptionId(entry.index)"
            :key="entry.candidate.instrumentId"
            type="button"
            role="option"
            class="instrument-search-box__option"
            :class="{
              'is-active': entry.index === activeCandidateIndex,
              'is-disabled': !entry.candidate.selectable,
            }"
            :disabled="!entry.candidate.selectable"
            :title="entry.candidate.unavailableReason || undefined"
            :aria-selected="entry.index === activeCandidateIndex"
            @mouseenter="entry.candidate.selectable && (activeCandidateIndex = entry.index)"
            @click="selectCandidate(entry.candidate)"
          >
            <InstrumentIdentity
              :market="entry.candidate.market"
              :code="entry.candidate.code"
              :instrument-id="entry.candidate.instrumentId"
              :name="entry.candidate.name"
              compact
              layout="stacked"
            />
            <span class="instrument-search-box__option-meta">
              <span>{{ formatInstrumentSecurityTypeLabel(entry.candidate.securityType) }}</span>
              <span v-if="entry.candidate.unavailableReason" class="instrument-search-box__unavailable">
                {{ entry.candidate.unavailableReason }}
              </span>
            </span>
          </button>
        </div>

        <div class="instrument-search-box__actions">
          <button
            v-if="resolutionStatus === 'incomplete' || resolutionStatus === 'not_found' || resolutionError"
            type="button"
            :disabled="loading"
            @click="submit"
          >
            重试
          </button>
          <button type="button" @click="closePanel">关闭</button>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.instrument-search-box {
  position: relative;
  min-width: 0;
  color: var(--tv-text, #0f172a);
}

.instrument-search-box__control {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 0.5rem;
}

.instrument-search-box--topbar .instrument-search-box__control {
  display: contents;
}

.instrument-search-box--topbar {
  padding-block: 2px;
}

.instrument-search-box--backtest .instrument-search-box__control,
.instrument-search-box--dialog .instrument-search-box__control {
  min-height: 40px;
  border: 1px solid var(--tv-border, #cbd5e1);
  border-radius: 0.45rem;
  background: var(--tv-bg-surface, #fff);
  padding-inline: 0.75rem 0.35rem;
}

.instrument-search-box--dialog .instrument-search-box__control {
  border-color: #cbd5e1;
  border-radius: 1rem;
  background: #fff;
  color: #0f172a;
}

.instrument-search-box__leading {
  color: var(--tv-text-muted, #64748b);
  font-size: 11px;
}

.instrument-search-box input {
  min-width: 0;
  flex: 1 1 auto;
  border: 0;
  outline: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  font-size: 0.8125rem;
}

.instrument-search-box input::placeholder {
  color: var(--tv-text-dim, #94a3b8);
}

.instrument-search-box__submit {
  display: inline-flex;
  flex: 0 0 auto;
  min-width: 2rem;
  min-height: 1.75rem;
  align-items: center;
  justify-content: center;
  border: 1px solid transparent;
  border-radius: 0.35rem;
  background: transparent;
  color: var(--tv-text-muted, #64748b);
  padding: 0 0.45rem;
  font-size: 0.6875rem;
  cursor: pointer;
}

.instrument-search-box--dialog .instrument-search-box__submit {
  min-width: 4rem;
  border-color: #cbd5e1;
  border-radius: 999px;
  color: #334155;
  font-size: 0.875rem;
  font-weight: 500;
}

.instrument-search-box__submit:hover,
.instrument-search-box__submit:focus-visible {
  border-color: var(--tv-border-strong, #94a3b8);
  background: var(--tv-bg-hover, #f1f5f9);
  color: var(--tv-text, #0f172a);
  outline: none;
}

.instrument-search-box__submit:disabled {
  cursor: wait;
  opacity: 0.65;
}

.instrument-search-box__submit-label {
  display: none;
}

.instrument-search-box--topbar .instrument-search-box__submit {
  min-height: 1.4rem;
}

.instrument-search-box__submit:hover .instrument-search-box__submit-shortcut,
.instrument-search-box__submit:focus-visible .instrument-search-box__submit-shortcut {
  display: none;
}

.instrument-search-box__submit:hover .instrument-search-box__submit-label,
.instrument-search-box__submit:focus-visible .instrument-search-box__submit-label {
  display: inline;
}

.instrument-search-box__panel {
  position: fixed;
  z-index: 2600;
  display: flex;
  flex-direction: column;
  max-width: calc(100vw - 1.5rem);
  max-height: min(35rem, 82vh);
  overflow: hidden;
  border: 1px solid var(--tv-border, #cbd5e1);
  border-radius: 0.6rem;
  background: var(--tv-bg-surface, #fff);
  box-shadow: var(--tv-shadow-lg, 0 18px 40px rgb(15 23 42 / 18%));
  color: var(--tv-text, #0f172a);
}

.instrument-search-box--dialog .instrument-search-box__panel {
  border-color: #e2e8f0;
  background: #fff;
  color: #334155;
}

.instrument-search-box__panel--warning {
  border-color: var(--tv-warn, #d97706);
}

.instrument-search-box__message,
.instrument-search-box__failures,
.instrument-search-box__empty {
  padding: 0.55rem 0.75rem;
  color: var(--tv-text-muted, #64748b);
  font-size: 0.6875rem;
}

.instrument-search-box__message,
.instrument-search-box__failures,
.instrument-search-box__filters,
.instrument-search-box__actions {
  flex: 0 0 auto;
}

.instrument-search-box__results {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
  scrollbar-width: thin;
}

.instrument-search-box__failures,
.instrument-search-box__unavailable {
  color: var(--tv-warn, #d97706);
}

.instrument-search-box__failures {
  padding-top: 0;
}

.instrument-search-box__filters {
  display: grid;
  gap: 0.4rem;
  border-top: 1px solid var(--tv-border, #e2e8f0);
  padding: 0.5rem 0.65rem;
}

.instrument-search-box__filter-row {
  display: grid;
  min-width: 0;
  grid-template-columns: auto minmax(0, 1fr);
  align-items: center;
  gap: 0.45rem;
}

.instrument-search-box__filter-label {
  color: var(--tv-text-dim, #94a3b8);
  font-size: 0.625rem;
}

.instrument-search-box__filter-scroll {
  display: flex;
  min-width: 0;
  gap: 0.35rem;
  overflow-x: auto;
  padding-bottom: 0.1rem;
  scrollbar-width: thin;
}

.instrument-search-box__filter-tag {
  flex: 0 0 auto;
  border: 1px solid var(--tv-border, #cbd5e1);
  border-radius: 999px;
  background: transparent;
  color: var(--tv-text-muted, #64748b);
  padding: 0.15rem 0.5rem;
  font-size: 0.625rem;
  cursor: pointer;
}

.instrument-search-box__filter-tag.is-active {
  border-color: var(--tv-accent, #0d9488);
  background: color-mix(in srgb, var(--tv-accent, #0d9488) 14%, transparent);
  color: var(--tv-accent, #0f766e);
}

.instrument-search-box__option {
  display: grid;
  width: 100%;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 0.75rem;
  border: 0;
  border-top: 1px solid var(--tv-border, #e2e8f0);
  background: transparent;
  color: inherit;
  padding: 0.55rem 0.75rem;
  text-align: left;
  cursor: pointer;
}

.instrument-search-box__option.is-active,
.instrument-search-box__option:hover {
  background: var(--tv-bg-hover, #f8fafc);
}

.instrument-search-box__option.is-disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.instrument-search-box__option-meta {
  display: grid;
  max-width: 10rem;
  justify-items: end;
  gap: 0.15rem;
  color: var(--tv-text-dim, #94a3b8);
  font-size: 0.625rem;
  text-align: right;
}

.instrument-search-box__actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.4rem;
  border-top: 1px solid var(--tv-border, #e2e8f0);
  padding: 0.4rem 0.5rem;
}

.instrument-search-box__actions button {
  border: 1px solid var(--tv-border, #cbd5e1);
  border-radius: 0.3rem;
  background: transparent;
  color: var(--tv-text-muted, #64748b);
  padding: 0.15rem 0.5rem;
  font-size: 0.625rem;
  cursor: pointer;
}

@media (max-width: 640px) {
  .instrument-search-box__option-meta {
    max-width: 7rem;
  }
}
</style>
