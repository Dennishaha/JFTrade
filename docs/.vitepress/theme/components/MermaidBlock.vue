<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { useData } from "vitepress";

const props = defineProps<{
  code: string;
}>();

const minScale = 0.25;
const maxScale = 3;
const scaleStep = 0.15;
const { isDark } = useData();
const canvasHeight = ref<number | null>(null);
const canvasWidth = ref<number | null>(null);
const error = ref<string | null>(null);
const graphRef = ref<HTMLElement | null>(null);
const graphViewportRef = ref<HTMLElement | null>(null);
const hasCustomScale = ref(false);
const isFullscreen = ref(false);
const isRendering = ref(false);
const mode = ref<"graph" | "code">("graph");
const renderVersion = ref(0);
const scale = ref(1);
const svg = ref("");
const viewerRef = ref<HTMLElement | null>(null);
const baseId = `jftrade-mermaid-${Math.random().toString(36).slice(2)}`;
let resizeObserver: ResizeObserver | null = null;

const source = computed(() => {
  try {
    return decodeURIComponent(props.code);
  } catch {
    return props.code;
  }
});

const canvasStyle = computed(() => {
  const width = canvasWidth.value;
  const height = canvasHeight.value;

  if (!width || !height) {
    return {};
  }

  return {
    width: `${Math.round(width * scale.value)}px`,
    height: `${Math.round(height * scale.value)}px`,
  };
});

const canZoomIn = computed(() => scale.value < maxScale);
const canZoomOut = computed(() => scale.value > minScale);
const scaleLabel = computed(() => `${Math.round(scale.value * 100)}%`);

function clampScale(value: number): number {
  return Math.min(maxScale, Math.max(minScale, value));
}

function messageFromError(value: unknown): string {
  if (value instanceof Error) return value.message;
  return String(value);
}

function readSvgSize(svgElement: SVGSVGElement): { width: number; height: number } {
  const viewBox = svgElement.getAttribute("viewBox")?.trim().split(/\s+/).map(Number);
  if (viewBox?.length === 4 && Number.isFinite(viewBox[2]) && Number.isFinite(viewBox[3])) {
    return {
      width: Math.max(viewBox[2] ?? 0, 1),
      height: Math.max(viewBox[3] ?? 0, 1),
    };
  }

  const width = Number.parseFloat(svgElement.getAttribute("width") ?? "");
  const height = Number.parseFloat(svgElement.getAttribute("height") ?? "");

  return {
    width: Number.isFinite(width) && width > 0 ? width : 800,
    height: Number.isFinite(height) && height > 0 ? height : 420,
  };
}

function updateCanvasSize(): void {
  const svgElement = graphRef.value?.querySelector<SVGSVGElement>("svg");
  if (!svgElement) return;

  const size = readSvgSize(svgElement);
  canvasWidth.value = size.width;
  canvasHeight.value = size.height;
}

function fitToWidth(): void {
  const viewport = graphViewportRef.value;
  const width = canvasWidth.value;
  if (!viewport || !width) {
    scale.value = 1;
    return;
  }

  const availableWidth = Math.max(viewport.clientWidth - 32, 1);
  scale.value = clampScale(Math.min(1, availableWidth / width));
}

function zoomBy(delta: number): void {
  scale.value = clampScale(Number((scale.value + delta).toFixed(2)));
  hasCustomScale.value = true;
}

function resetZoom(): void {
  hasCustomScale.value = false;
  fitToWidth();
}

async function toggleFullscreen(): Promise<void> {
  if (typeof document === "undefined") return;

  if (document.fullscreenElement) {
    await document.exitFullscreen();
    return;
  }

  await viewerRef.value?.requestFullscreen();
}

function onFullscreenChange(): void {
  if (typeof document === "undefined") return;
  isFullscreen.value = document.fullscreenElement === viewerRef.value;
  if (mode.value === "graph" && !hasCustomScale.value) {
    window.setTimeout(fitToWidth, 0);
  }
}

function onViewportResize(): void {
  if (!hasCustomScale.value) {
    fitToWidth();
  }
}

async function renderDiagram(): Promise<void> {
  if (typeof window === "undefined") return;

  const currentVersion = renderVersion.value + 1;
  renderVersion.value = currentVersion;
  error.value = null;
  isRendering.value = true;

  try {
    const mermaid = (await import("mermaid")).default;
    mermaid.initialize({
      startOnLoad: false,
      securityLevel: "strict",
      theme: isDark.value ? "dark" : "default",
    });

    const result = await mermaid.render(`${baseId}-${currentVersion}`, source.value);
    if (renderVersion.value !== currentVersion) return;

    svg.value = result.svg;
    await nextTick();
    updateCanvasSize();
    if (!hasCustomScale.value) {
      fitToWidth();
    }
    if (graphRef.value) {
      result.bindFunctions?.(graphRef.value);
    }
  } catch (renderError) {
    if (renderVersion.value !== currentVersion) return;
    svg.value = "";
    error.value = messageFromError(renderError);
  } finally {
    if (renderVersion.value === currentVersion) {
      isRendering.value = false;
    }
  }
}

onMounted(() => {
  document.addEventListener("fullscreenchange", onFullscreenChange);
  if (typeof ResizeObserver !== "undefined" && graphViewportRef.value) {
    resizeObserver = new ResizeObserver(onViewportResize);
    resizeObserver.observe(graphViewportRef.value);
  }
  void renderDiagram();
});

onUnmounted(() => {
  document.removeEventListener("fullscreenchange", onFullscreenChange);
  resizeObserver?.disconnect();
});

watch([source, isDark], () => {
  hasCustomScale.value = false;
  void renderDiagram();
});

watch(mode, async (nextMode) => {
  if (nextMode !== "graph" || hasCustomScale.value) return;
  await nextTick();
  fitToWidth();
});
</script>

<template>
  <figure ref="viewerRef" class="jftrade-mermaid">
    <div class="jftrade-mermaid__viewer">
      <div class="jftrade-mermaid__toolbar" aria-label="Mermaid 图工具栏">
        <div class="jftrade-mermaid__switch" aria-label="Mermaid 展示模式">
          <button
            type="button"
            class="jftrade-mermaid__switch-option"
            :class="{ 'jftrade-mermaid__switch-option--active': mode === 'graph' }"
            :aria-pressed="mode === 'graph'"
            @click="mode = 'graph'"
          >
            图
          </button>
          <button
            type="button"
            class="jftrade-mermaid__switch-option"
            :class="{ 'jftrade-mermaid__switch-option--active': mode === 'code' }"
            :aria-pressed="mode === 'code'"
            @click="mode = 'code'"
          >
            代码
          </button>
        </div>

        <div class="jftrade-mermaid__actions">
          <template v-if="mode === 'graph'">
            <button
              type="button"
              class="jftrade-mermaid__button"
              :disabled="!canZoomOut"
              title="缩小"
              @click="zoomBy(-scaleStep)"
            >
              -
            </button>
            <span class="jftrade-mermaid__scale" aria-live="polite">{{ scaleLabel }}</span>
            <button
              type="button"
              class="jftrade-mermaid__button"
              :disabled="!canZoomIn"
              title="放大"
              @click="zoomBy(scaleStep)"
            >
              +
            </button>
            <button
              type="button"
              class="jftrade-mermaid__button jftrade-mermaid__button--text"
              title="适应宽度"
              @click="resetZoom"
            >
              适应
            </button>
          </template>
        <button
          type="button"
          class="jftrade-mermaid__button jftrade-mermaid__button--text"
          :title="isFullscreen ? '退出全屏' : '全屏'"
          @click="toggleFullscreen"
        >
          {{ isFullscreen ? "退出全屏" : "全屏" }}
        </button>
        </div>
      </div>

      <div
        v-show="mode === 'graph'"
        ref="graphViewportRef"
        class="jftrade-mermaid__graph"
      >
        <div
          v-if="svg"
          ref="graphRef"
          class="jftrade-mermaid__canvas"
          :style="canvasStyle"
          v-html="svg"
        />
        <p v-else-if="error" class="jftrade-mermaid__error">
          Mermaid 渲染失败：{{ error }}
        </p>
        <p v-else class="jftrade-mermaid__loading">
          {{ isRendering ? "Mermaid 图渲染中..." : "等待 Mermaid 渲染..." }}
        </p>
      </div>

      <div v-show="mode === 'code'" class="jftrade-mermaid__code">
        <pre><code>{{ source }}</code></pre>
      </div>
    </div>
  </figure>
</template>
