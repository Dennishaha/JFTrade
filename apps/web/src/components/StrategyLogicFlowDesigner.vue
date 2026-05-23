<script setup lang="ts">
import type { StrategyVisualModelDocument } from "@jftrade/ui-contracts";

import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";

import "@logicflow/core/lib/style/index.css";
import "@logicflow/extension/lib/style/index.css";

import { useTheme, type ThemeMode } from "../composables/useTheme";
import {
  createStrategyPaletteItems,
  fromLogicFlowGraphData,
  toLogicFlowGraphData,
} from "../features/strategyVisualBuilder";

interface FitViewPadding {
  top?: number;
  right?: number;
  bottom?: number;
  left?: number;
}

const props = withDefaults(defineProps<{
  modelValue: StrategyVisualModelDocument;
  chrome?: boolean;
  height?: number | string;
  minHeight?: number | string;
  resizable?: boolean;
  showZoomSlider?: boolean;
  showBuilderActions?: boolean;
  zoomRightOffset?: string;
  fitViewPadding?: FitViewPadding;
}>(), {
  chrome: true,
  height: "560px",
  minHeight: "360px",
  resizable: false,
  showZoomSlider: false,
  showBuilderActions: false,
  zoomRightOffset: "1rem",
  fitViewPadding: () => ({}),
});

const emit = defineEmits<{
  "update:modelValue": [value: StrategyVisualModelDocument];
  "select-node": [nodeId: string | null];
}>();

const { theme } = useTheme();
const panel = ref<HTMLElement | null>(null);
const container = ref<HTMLElement | null>(null);
const isFallbackMode = ref(false);
const isPaletteExpanded = ref(false);
const paletteSearchQuery = ref("");
const zoomPercent = ref(100);
const paletteItems = createStrategyPaletteItems();
let resizeObserver: ResizeObserver | null = null;
let resizeAnimationFrameId = 0;
let alignViewportAnimationFrameId = 0;
let pendingForceResize = false;
let isPointerResizeTracking = false;
let lastCanvasWidth = 0;
let lastCanvasHeight = 0;
let fitViewBaseScale = 1;

const panelHeight = computed(() =>
  typeof props.height === "number" ? `${props.height}px` : props.height,
);

const panelMinHeight = computed(() =>
  typeof props.minHeight === "number" ? `${props.minHeight}px` : props.minHeight,
);

const zoomRightOffset = computed(() => props.zoomRightOffset);

const filteredPaletteItems = computed(() => {
  const query = normalizePaletteSearchQuery(paletteSearchQuery.value);
  if (query === "") {
    return paletteItems;
  }

  return paletteItems.filter((item) => buildPaletteSearchText(item).includes(query));
});

const normalizedFitViewPadding = computed(() => ({
  top: Math.max(0, props.fitViewPadding?.top ?? 0),
  right: Math.max(0, props.fitViewPadding?.right ?? 0),
  bottom: Math.max(0, props.fitViewPadding?.bottom ?? 0),
  left: Math.max(0, props.fitViewPadding?.left ?? 0),
}));

let logicFlowInstance: {
  dnd?: {
    startDrag: (nodeConfig: {
      type?: string;
      properties?: Record<string, unknown>;
      text?: string;
    }) => void;
  };
  extension?: {
    dndPanel?: {
      setPatternItems: (items: ReturnType<typeof createStrategyPaletteItems>) => void;
    };
  };
  render: (data: ReturnType<typeof toLogicFlowGraphData>) => void;
  getGraphData: () => unknown;
  on: (event: string, handler: (payload: any) => void) => void;
  off?: (event: string, handler: (payload: any) => void) => void;
  resize: (width?: number, height?: number) => void;
  fitView?: (verticalOffset?: number, horizontalOffset?: number) => void;
  translateCenter?: () => void;
  translate?: (x: number, y: number) => void;
  zoom?: (zoomSize?: boolean | number, point?: [number, number]) => string;
  getTransform?: () => {
    SCALE_X: number;
    SCALE_Y: number;
    TRANSLATE_X: number;
    TRANSLATE_Y: number;
  };
  setTheme?: (style: Record<string, unknown>, themeMode?: string) => void;
  updateText: (id: string, value: string) => void;
  setProperties: (id: string, properties: Record<string, unknown>) => void;
  deleteNode: (id: string) => void;
  deleteEdge: (id: string) => void;
  destroy?: () => void;
} | null = null;

let logicFlowPluginsInstalled = false;
let lastGraphSignature = "";

const emitGraphChange = () => {
  if (logicFlowInstance === null) {
    return;
  }
  const graphData = logicFlowInstance.getGraphData();
  const nextModel = fromLogicFlowGraphData(graphData as ReturnType<typeof toLogicFlowGraphData>);
  lastGraphSignature = JSON.stringify(toLogicFlowGraphData(nextModel));
  emit("update:modelValue", nextModel);
};

const handleNodeSelected = (payload: { data?: { id?: string } }) => {
  emit("select-node", payload.data?.id ?? null);
};

const handleBlankClicked = () => {
  emit("select-node", null);
};

const togglePaletteExpanded = () => {
  const nextExpanded = !isPaletteExpanded.value;
  isPaletteExpanded.value = nextExpanded;
  if (!nextExpanded) {
    paletteSearchQuery.value = "";
  }
};

const handlePaletteItemPointerDown = (
  item: ReturnType<typeof createStrategyPaletteItems>[number],
  event: PointerEvent,
) => {
  if (logicFlowInstance === null) {
    return;
  }

  logicFlowInstance.dnd?.startDrag({
    type: item.type,
    properties: item.properties,
    text: item.text,
  });
  event.preventDefault();
};

function normalizePaletteSearchQuery(value: string): string {
  return value.trim().toLowerCase();
}

function buildPaletteSearchText(
  item: ReturnType<typeof createStrategyPaletteItems>[number],
): string {
  const blockKind = item.properties.blockKind;
  return [
    item.label,
    item.text,
    typeof blockKind === "string" ? blockKind : "",
  ].join(" ").toLowerCase();
}

const buildLogicFlowTheme = (themeMode: ThemeMode) => {
  if (themeMode === "light") {
    return {
      rect: {
        radius: 18,
        stroke: "#d97706",
        fill: "#ffffff",
      },
      circle: {
        stroke: "#d97706",
        fill: "#fff7ed",
      },
      polygon: {
        stroke: "#d97706",
        fill: "#fff7ed",
      },
      polyline: {
        stroke: "#d97706",
        hoverStroke: "#b45309",
        selectedStroke: "#b45309",
      },
      text: {
        color: "#334155",
      },
      edgeText: {
        textWidth: 160,
        overflowMode: "autoWrap",
        fontSize: 12,
        background: {
          fill: "#fffaf0",
          stroke: "#fde68a",
          radius: 8,
        },
      },
    };
  }

  return {
    rect: {
      radius: 18,
      stroke: "#fbbf24",
      fill: "#182235",
    },
    circle: {
      stroke: "#fbbf24",
      fill: "#111827",
    },
    polygon: {
      stroke: "#fbbf24",
      fill: "#111827",
    },
    polyline: {
      stroke: "#fbbf24",
      hoverStroke: "#f59e0b",
      selectedStroke: "#f59e0b",
    },
    text: {
      color: "#e2e8f0",
    },
    edgeText: {
      textWidth: 160,
      overflowMode: "autoWrap",
      fontSize: 12,
      background: {
        fill: "#0f172a",
        stroke: "rgba(251,191,36,0.32)",
        radius: 8,
      },
    },
  };
};

const applyLogicFlowTheme = () => {
  logicFlowInstance?.setTheme?.(
    buildLogicFlowTheme(theme.value),
    theme.value === "dark" ? "dark" : "default",
  );
};

const resizeLogicFlowCanvas = (force = false) => {
  if (logicFlowInstance === null || container.value === null) {
    return;
  }
  const nextWidth = container.value.clientWidth || 0;
  const nextHeight = container.value.clientHeight || 0;
  if (!force && nextWidth === lastCanvasWidth && nextHeight === lastCanvasHeight) {
    return;
  }
  lastCanvasWidth = nextWidth;
  lastCanvasHeight = nextHeight;
  logicFlowInstance.resize(
    nextWidth || undefined,
    nextHeight || undefined,
  );
};

const queueResizeLogicFlowCanvas = (force = false) => {
  pendingForceResize = pendingForceResize || force;

  if (typeof requestAnimationFrame === "undefined") {
    const shouldForce = pendingForceResize;
    pendingForceResize = false;
    resizeLogicFlowCanvas(shouldForce);
    return;
  }

  if (resizeAnimationFrameId !== 0) {
    return;
  }

  resizeAnimationFrameId = requestAnimationFrame(() => {
    resizeAnimationFrameId = 0;
    const shouldForce = pendingForceResize;
    pendingForceResize = false;
    resizeLogicFlowCanvas(shouldForce);
    if (isPointerResizeTracking) {
      queueResizeLogicFlowCanvas();
    }
  });
};

const getViewportFocusPoint = (): [number, number] | null => {
  if (container.value === null) {
    return null;
  }

  const { top, right, bottom, left } = normalizedFitViewPadding.value;
  return [
    container.value.clientWidth / 2 + (left - right) / 2,
    container.value.clientHeight / 2 + (top - bottom) / 2,
  ];
};

const clampZoomPercent = (value: number) => Math.min(180, Math.max(60, value));

const applyZoomPercent = (nextPercent: number) => {
  zoomPercent.value = clampZoomPercent(nextPercent);

  if (logicFlowInstance === null) {
    return;
  }

  const point = getViewportFocusPoint();
  if (point === null) {
    return;
  }

  const targetScale = fitViewBaseScale * (zoomPercent.value / 100);
  logicFlowInstance.zoom?.(targetScale, point);
};

const resetViewportZoom = () => {
  zoomPercent.value = 100;
  queueResizeLogicFlowCanvas(true);
  queueAlignLogicFlowViewport();
};

const handleZoomSliderInput = (event: Event) => {
  const value = Number((event.target as HTMLInputElement).value);
  if (!Number.isFinite(value)) {
    return;
  }
  applyZoomPercent(value);
};

const alignLogicFlowViewport = () => {
  if (logicFlowInstance === null) {
    return;
  }

  const { top, right, bottom, left } = normalizedFitViewPadding.value;
  const verticalOffset = top + bottom + 24;
  const horizontalOffset = left + right + 24;

  if (logicFlowInstance.fitView !== undefined) {
    logicFlowInstance.fitView(verticalOffset, horizontalOffset);
  } else {
    logicFlowInstance.translateCenter?.();
  }

  const shiftX = Math.round((left - right) / 2);
  const shiftY = Math.round((top - bottom) / 2);

  if (shiftX !== 0 || shiftY !== 0) {
    logicFlowInstance.translate?.(shiftX, shiftY);
  }

  fitViewBaseScale = logicFlowInstance.getTransform?.().SCALE_X ?? 1;

  if (zoomPercent.value !== 100) {
    applyZoomPercent(zoomPercent.value);
  }
};

const queueAlignLogicFlowViewport = () => {
  if (logicFlowInstance === null) {
    return;
  }

  if (typeof requestAnimationFrame === "undefined") {
    alignLogicFlowViewport();
    return;
  }

  if (alignViewportAnimationFrameId !== 0) {
    cancelAnimationFrame(alignViewportAnimationFrameId);
  }

  alignViewportAnimationFrameId = requestAnimationFrame(() => {
    alignViewportAnimationFrameId = 0;
    alignLogicFlowViewport();
  });
};

const handlePanelPointerDown = (event: PointerEvent) => {
  if (!props.resizable || event.button !== 0) {
    return;
  }
  isPointerResizeTracking = true;
  queueResizeLogicFlowCanvas(true);
};

const stopPointerResizeTracking = () => {
  if (!isPointerResizeTracking) {
    return;
  }
  isPointerResizeTracking = false;
  queueResizeLogicFlowCanvas(true);
};

const graphMutationEvents = [
  "node:add",
  "node:delete",
  "node:drag",
  "node:drop",
  "node:properties-change",
  "edge:add",
  "edge:delete",
  "text:update",
];

watch(
  () => props.modelValue,
  (modelValue) => {
    if (logicFlowInstance === null) {
      return;
    }
    const nextGraphData = toLogicFlowGraphData(modelValue);
    const nextSignature = JSON.stringify(nextGraphData);
    if (nextSignature === lastGraphSignature) {
      return;
    }
    logicFlowInstance.render(nextGraphData);
    lastGraphSignature = nextSignature;
    queueResizeLogicFlowCanvas(true);
    queueAlignLogicFlowViewport();
  },
  { deep: true },
);

watch(theme, () => {
  applyLogicFlowTheme();
  queueResizeLogicFlowCanvas(true);
});

watch(
  normalizedFitViewPadding,
  () => {
    queueResizeLogicFlowCanvas(true);
    queueAlignLogicFlowViewport();
  },
  { deep: true },
);

onMounted(async () => {
  if (typeof window !== "undefined") {
    window.addEventListener("pointerup", stopPointerResizeTracking);
    window.addEventListener("pointercancel", stopPointerResizeTracking);
  }

  if (container.value === null) {
    return;
  }

  if (
    typeof navigator !== "undefined" &&
    navigator.userAgent.toLowerCase().includes("jsdom")
  ) {
    isFallbackMode.value = true;
    return;
  }

  const [{ default: LogicFlow }, { DndPanel }] = await Promise.all([
    import("@logicflow/core"),
    import("@logicflow/extension"),
  ]);

  if (!logicFlowPluginsInstalled) {
    LogicFlow.use(DndPanel);
    logicFlowPluginsInstalled = true;
  }

  const nextLogicFlowInstance = new LogicFlow({
    container: container.value,
    width: container.value.clientWidth || 860,
    height: container.value.clientHeight || 460,
    background: {
      backgroundColor: "transparent",
    },
    grid: {
      size: 20,
      visible: true,
      type: "mesh",
      config: {
        color: theme.value === "light" ? "#efe7d9" : "rgba(148,163,184,0.16)",
      },
    },
    edgeTextEdit: false,
    nodeTextEdit: false,
    stopScrollGraph: true,
    stopZoomGraph: true,
    plugins: [DndPanel],
  }) as NonNullable<typeof logicFlowInstance>;

  logicFlowInstance = nextLogicFlowInstance;

  nextLogicFlowInstance.extension?.dndPanel?.setPatternItems(
    createStrategyPaletteItems(),
  );

  const initialGraphData = toLogicFlowGraphData(props.modelValue);
  nextLogicFlowInstance.render(initialGraphData);
  lastGraphSignature = JSON.stringify(initialGraphData);
  applyLogicFlowTheme();
  queueResizeLogicFlowCanvas(true);
  queueAlignLogicFlowViewport();

  for (const eventName of graphMutationEvents) {
    nextLogicFlowInstance.on(eventName, emitGraphChange);
  }

  nextLogicFlowInstance.on("node:click", handleNodeSelected);
  nextLogicFlowInstance.on("blank:click", handleBlankClicked);

  if (typeof ResizeObserver !== "undefined") {
    resizeObserver = new ResizeObserver(() => {
      queueResizeLogicFlowCanvas();
      queueAlignLogicFlowViewport();
    });
    if (panel.value !== null) {
      resizeObserver.observe(panel.value);
    }
    resizeObserver.observe(container.value);
  }
});

onBeforeUnmount(() => {
  stopPointerResizeTracking();
  if (typeof window !== "undefined") {
    window.removeEventListener("pointerup", stopPointerResizeTracking);
    window.removeEventListener("pointercancel", stopPointerResizeTracking);
  }
  if (resizeAnimationFrameId !== 0 && typeof cancelAnimationFrame !== "undefined") {
    cancelAnimationFrame(resizeAnimationFrameId);
  }
  if (alignViewportAnimationFrameId !== 0 && typeof cancelAnimationFrame !== "undefined") {
    cancelAnimationFrame(alignViewportAnimationFrameId);
  }
  alignViewportAnimationFrameId = 0;
  resizeAnimationFrameId = 0;
  pendingForceResize = false;
  resizeObserver?.disconnect();
  resizeObserver = null;
  if (logicFlowInstance === null) {
    return;
  }
  for (const eventName of graphMutationEvents) {
    logicFlowInstance.off?.(eventName, emitGraphChange);
  }
  logicFlowInstance.off?.("node:click", handleNodeSelected);
  logicFlowInstance.off?.("blank:click", handleBlankClicked);
  logicFlowInstance.destroy?.();
  logicFlowInstance = null;
});

function selectNodeById(nodeId: string | null): void {
  if (nodeId === null || logicFlowInstance === null) {
    return;
  }

  const graphData = logicFlowInstance.getGraphData() as {
    nodes?: Array<{ id?: string }>;
  };

  const nodeExists = (graphData.nodes ?? []).some(
    (node) => node.id === nodeId,
  );

  if (!nodeExists) {
    return;
  }

  emit("select-node", nodeId);
}

defineExpose({
  selectNodeById,
});
</script>

<template>
  <div ref="panel"
    class="strategy-logic-flow-panel relative flex min-w-0 flex-col rounded-[28px] border border-slate-200 bg-[#fffdf7] shadow-[0_24px_80px_-48px_rgba(15,23,42,0.45)]"
    :class="{
      'strategy-logic-flow-panel--bare': !chrome,
      'strategy-logic-flow-panel--chrome': chrome,
      'strategy-logic-flow-panel--resizable': resizable,
    }" :style="{ height: panelHeight, minHeight: panelMinHeight }" @pointerdown="handlePanelPointerDown">
    <div v-if="chrome" class="mb-3 flex flex-wrap items-center justify-between gap-3 px-4 pt-4">
      <div>
        <div class="text-sm font-semibold uppercase tracking-[0.22em] text-slate-500">流程画布</div>
        <div class="mt-1 text-sm text-slate-600">
          从左上角拖入触发器、条件和动作块，再用连线表达执行顺序。
        </div>
      </div>
      <div class="strategy-logic-flow-badge rounded-full px-3 py-1 text-xs font-medium">
        拖拽图块进入画布
      </div>
    </div>

    <div ref="container" data-testid="strategy-logic-flow-canvas"
      class="strategy-logic-flow-canvas relative min-h-[260px] w-full flex-1" :class="{ 'mt-1': chrome }">
      <div v-if="isFallbackMode"
        class="strategy-logic-flow-fallback absolute inset-0 flex items-center justify-center px-6 text-center text-sm text-slate-500">
        测试环境已跳过流程图真实初始化，运行浏览器页面时会启用完整拖拽画布。
      </div>
    </div>

    <div
      v-if="showBuilderActions"
      class="strategy-logic-flow-builder"
      :class="{ 'strategy-logic-flow-builder--expanded': isPaletteExpanded }"
      data-testid="strategy-logic-flow-builder"
    >
      <div v-if="isPaletteExpanded" class="strategy-logic-flow-builder__sheet">
        <div class="strategy-logic-flow-builder__search-row">
          <input
            v-model="paletteSearchQuery"
            class="strategy-logic-flow-builder__search"
            data-testid="strategy-logic-flow-builder-search"
            placeholder="搜索图块，例如 RSI、通知"
            type="search"
          />
          <span class="strategy-logic-flow-builder__count">{{ filteredPaletteItems.length }}/{{ paletteItems.length }}</span>
        </div>

        <div v-if="filteredPaletteItems.length > 0" class="strategy-logic-flow-builder__grid">
          <button
            v-for="item in filteredPaletteItems"
            :key="`${item.label}-${item.type}`"
            class="strategy-logic-flow-builder__item"
            type="button"
            @pointerdown="handlePaletteItemPointerDown(item, $event)"
          >
            <span class="strategy-logic-flow-builder__icon" :style="{ backgroundImage: `url(${item.icon})` }" />
            <span class="strategy-logic-flow-builder__label">{{ item.label }}</span>
          </button>
        </div>

        <div v-else class="strategy-logic-flow-builder__empty">
          没有匹配的图块，试试搜索指标名或动作名。
        </div>
      </div>

      <button
        class="strategy-logic-flow-builder__toggle"
        data-testid="strategy-logic-flow-builder-toggle"
        type="button"
        @click="togglePaletteExpanded"
      >
        {{ isPaletteExpanded ? '关闭创建器' : '展开创建器' }}
      </button>
    </div>

    <div v-if="showZoomSlider" class="strategy-logic-flow-zoom" data-testid="strategy-visual-builder-section">
      <div class="strategy-logic-flow-zoom__scale" data-testid="strategy-logic-flow-zoom">
        <button
          aria-label="适应视图"
          class="strategy-logic-flow-zoom__reset"
          data-testid="strategy-logic-flow-zoom-fit"
          title="适应视图"
          type="button"
          @click="resetViewportZoom"
        >
          <svg aria-hidden="true" class="strategy-logic-flow-zoom__fit-icon" fill="none" viewBox="0 0 16 16">
            <path d="M5.25 1.75H1.75v3.5M10.75 1.75h3.5v3.5M1.75 10.75v3.5h3.5M14.25 10.75v3.5h-3.5" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="1.35" />
          </svg>
        </button>
        <input class="strategy-logic-flow-zoom__slider" max="180" min="60" step="5" type="range" :value="zoomPercent"
          @input="handleZoomSliderInput" />
        <div class="strategy-logic-flow-zoom__value">{{ zoomPercent }}%</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.strategy-logic-flow-canvas :deep(.lf-graph),
.strategy-logic-flow-canvas :deep(.lf-canvas-overlay) {
  height: 100% !important;
  width: 100% !important;
}

.strategy-logic-flow-canvas :deep(.lf-dndpanel) {
  display: none !important;
}

.strategy-logic-flow-panel--resizable {
  resize: vertical;
}

.strategy-logic-flow-panel {
  --strategy-logic-flow-dnd-top: 1rem;
  --strategy-logic-flow-dnd-bottom-gap: 1rem;
  overflow: hidden;
  background: linear-gradient(180deg,
      color-mix(in srgb, var(--card-surface) 96%, var(--card-amber-surface) 4%) 0%,
      color-mix(in srgb, var(--card-surface) 92%, transparent) 100%);
}

.strategy-logic-flow-panel--bare {
  --strategy-logic-flow-dnd-top: 9.4rem;
  --strategy-logic-flow-dnd-bottom-gap: 6rem;
  border: 0;
  border-radius: 0;
  background: transparent;
  box-shadow: none;
  overflow: visible;
}

.strategy-logic-flow-badge {
  border: 1px solid var(--card-amber-border);
  background: var(--card-amber-surface);
  color: var(--card-amber-text);
}

.strategy-logic-flow-canvas {
  overflow: hidden;
  border-top: 1px solid color-mix(in srgb, var(--card-amber-border) 72%, var(--card-border));
  background: linear-gradient(135deg,
      color-mix(in srgb, var(--card-surface) 94%, var(--card-amber-surface) 6%) 0%,
      color-mix(in srgb, var(--card-surface-raised) 88%, var(--card-amber-surface) 12%) 55%,
      color-mix(in srgb, var(--card-surface) 90%, var(--card-amber-surface) 10%) 100%);
}

.strategy-logic-flow-panel--bare .strategy-logic-flow-canvas {
  border-top: 0;
  overflow: visible;
  background:
    linear-gradient(180deg,
      color-mix(in srgb, var(--tv-bg-surface) 68%, transparent) 0%,
      color-mix(in srgb, var(--tv-bg-app) 86%, transparent) 100%);
}

.strategy-logic-flow-fallback {
  background: color-mix(in srgb, var(--card-surface) 82%, transparent);
}

.strategy-logic-flow-zoom {
  position: absolute;
  right: max(1rem, v-bind(zoomRightOffset));
  left: auto;
  bottom: 1rem;
  z-index: 5;
  display: inline-flex;
  align-items: center;
  justify-content: stretch;
  width: min(18em, calc(100% - 2rem));
  max-width: calc(100% - 2rem);
  padding: 0.65rem 0.85rem;
  border-radius: 999px;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: color-mix(in srgb, var(--tv-bg-surface) 55%, transparent);
  box-shadow:
    0 8px 24px rgba(2, 6, 23, 0.24),
    inset 0 1px 0 rgba(255, 255, 255, 0.06);
  color: var(--tv-text);
  backdrop-filter: blur(28px) saturate(180%);
}

.strategy-logic-flow-zoom__scale {
  display: inline-flex;
  align-items: center;
  gap: 0.65rem;
}

.strategy-logic-flow-zoom__scale {
  width: 100%;
}

.strategy-logic-flow-zoom__reset {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 2rem;
  min-width: 2.2rem;
  padding: 0.35rem 0.55rem;
  border-radius: 999px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: color-mix(in srgb, var(--tv-bg-elevated) 54%, transparent);
  color: inherit;
}

.strategy-logic-flow-zoom__fit-icon {
  width: 1rem;
  height: 1rem;
}

.strategy-logic-flow-zoom__slider {
  flex: 1 1 auto;
  min-width: 8rem;
  accent-color: var(--tv-accent);
}

.strategy-logic-flow-zoom__value {
  min-width: 3.5rem;
  text-align: right;
  font-size: 0.82rem;
  font-weight: 700;
  color: var(--tv-text-muted);
}

.strategy-logic-flow-canvas :deep(.lf-graph) {
  background: transparent;
  overflow: visible !important;
}

.strategy-logic-flow-builder {
  position: absolute;
  left: 1rem;
  bottom: 1rem;
  z-index: 70;
  display: block;
  max-width: min(24rem, calc(100% - 2rem));
}

.strategy-logic-flow-builder--expanded {
  width: min(22rem, calc(100vw - 2rem));
}

.strategy-logic-flow-builder__toggle {
  position: relative;
  z-index: 2;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 2.2rem;
  padding: 0.45rem 0.95rem;
  border-radius: 999px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: color-mix(in srgb, var(--tv-bg-surface) 58%, transparent);
  color: var(--tv-text);
  font-size: 0.82rem;
  font-weight: 700;
  backdrop-filter: blur(24px) saturate(160%);
}

.strategy-logic-flow-builder__sheet {
  position: absolute;
  left: 0;
  bottom: calc(100% + 0.7rem);
  z-index: 1;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  width: 100%;
  max-height: min(32rem, calc(100dvh - 13rem));
  padding: 0.8rem;
  border-radius: 1.4rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: color-mix(in srgb, var(--tv-bg-surface) 58%, transparent);
  box-shadow:
    0 8px 24px rgba(2, 6, 23, 0.24),
    inset 0 1px 0 rgba(255, 255, 255, 0.06);
  backdrop-filter: blur(28px) saturate(180%);
  overflow: hidden;
}

.strategy-logic-flow-builder__search-row {
  display: flex;
  align-items: center;
  gap: 0.65rem;
  flex-shrink: 0;
}

.strategy-logic-flow-builder__search {
  flex: 1 1 auto;
  min-width: 0;
  min-height: 2.35rem;
  padding: 0.5rem 0.8rem;
  border-radius: 0.95rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: color-mix(in srgb, var(--tv-bg-elevated) 62%, transparent);
  color: var(--tv-text);
  font-size: 0.82rem;
  outline: none;
}

.strategy-logic-flow-builder__search::placeholder {
  color: var(--tv-text-dim);
}

.strategy-logic-flow-builder__count {
  flex: 0 0 auto;
  min-width: 3rem;
  text-align: right;
  font-size: 0.75rem;
  font-weight: 700;
  color: var(--tv-text-muted);
}

.strategy-logic-flow-builder__grid {
  flex: 1 1 auto;
  min-height: 0;
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(4.9rem, 1fr));
  gap: 0.65rem;
  overflow-y: auto;
  overscroll-behavior: contain;
  scrollbar-gutter: stable;
  padding-right: 0.15rem;
}

.strategy-logic-flow-builder__item {
  display: grid;
  justify-items: center;
  gap: 0.45rem;
  min-height: 5.8rem;
  padding: 0.6rem 0.45rem;
  border-radius: 1rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: color-mix(in srgb, var(--tv-bg-elevated) 52%, transparent);
  color: var(--tv-text);
  text-align: center;
}

.strategy-logic-flow-builder__item:hover {
  border-color: color-mix(in srgb, var(--tv-accent) 58%, transparent);
}

.strategy-logic-flow-builder__item:focus-visible,
.strategy-logic-flow-builder__toggle:focus-visible,
.strategy-logic-flow-zoom__reset:focus-visible {
  outline: 2px solid color-mix(in srgb, var(--tv-accent) 72%, white 28%);
  outline-offset: 2px;
}

.strategy-logic-flow-builder__icon {
  width: 2.4rem;
  height: 2.4rem;
  border-radius: 0.8rem;
  background-position: center;
  background-repeat: no-repeat;
  background-size: contain;
}

.strategy-logic-flow-builder__label {
  font-size: 0.75rem;
  font-weight: 700;
  line-height: 1.3;
}

.strategy-logic-flow-builder__empty {
  padding: 0.45rem 0.1rem 0.1rem;
  color: var(--tv-text-muted);
  font-size: 0.78rem;
  line-height: 1.5;
}

.strategy-logic-flow-canvas :deep(.lf-background-area) {
  background: transparent !important;
}

.strategy-logic-flow-panel--bare .strategy-logic-flow-canvas :deep(.lf-canvas-overlay),
.strategy-logic-flow-panel--bare .strategy-logic-flow-canvas :deep(svg) {
  overflow: visible !important;
}

.strategy-logic-flow-canvas :deep(.lf-grid path),
.strategy-logic-flow-canvas :deep(.lf-grid circle),
.strategy-logic-flow-canvas :deep(.lf-grid line) {
  stroke: color-mix(in srgb, var(--card-border) 74%, var(--card-amber-border)) !important;
}

.strategy-logic-flow-canvas :deep(.lf-dndpanel),
.strategy-logic-flow-canvas :deep(.lf-menu),
.strategy-logic-flow-canvas :deep(.lf-control) {
  z-index: 4;
  border: 1px solid var(--card-border);
  background: color-mix(in srgb, var(--card-surface) 88%, transparent) !important;
  box-shadow: 0 18px 48px rgba(15, 23, 42, 0.18);
  color: var(--card-text-1);
}

.strategy-logic-flow-canvas :deep(.lf-dnd-item),
.strategy-logic-flow-canvas :deep(.lf-menu-item),
.strategy-logic-flow-canvas :deep(.lf-control-item) {
  color: var(--card-text-2);
}

.strategy-logic-flow-canvas :deep(.lf-dnd-item:hover),
.strategy-logic-flow-canvas :deep(.lf-menu-item:hover),
.strategy-logic-flow-canvas :deep(.lf-control-item:hover) {
  background: color-mix(in srgb, var(--card-surface-raised) 90%, transparent) !important;
}

.strategy-logic-flow-canvas :deep(.lf-label-editor) {
  background: var(--card-surface) !important;
  color: var(--card-text-1);
}

@media (max-width: 720px) {
  .strategy-logic-flow-zoom {
    width: min(18em, calc(100% - 2rem));
    border-radius: 1.4rem;
  }

  .strategy-logic-flow-zoom__scale {
    width: 100%;
  }

  .strategy-logic-flow-builder {
    max-width: calc(100% - 2rem);
  }

  .strategy-logic-flow-builder--expanded {
    width: min(20rem, calc(100vw - 2rem));
  }

  .strategy-logic-flow-builder__sheet {
    max-height: min(24rem, calc(100dvh - 12rem));
  }
}
</style>