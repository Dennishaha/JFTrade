import {
  computed,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
  type ComputedRef,
  type Ref,
} from "vue";

import type { ThemeMode } from "./useTheme";

interface FitViewPaddingInput {
  top?: number;
  right?: number;
  bottom?: number;
  left?: number;
}

interface StrategyLogicFlowViewportInstance {
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
}

interface UseStrategyLogicFlowViewportOptions {
  container: Ref<HTMLElement | null>;
  panel: Ref<HTMLElement | null>;
  theme: Ref<ThemeMode>;
  resizable: ComputedRef<boolean>;
  fitViewPadding: ComputedRef<FitViewPaddingInput | undefined>;
  getInstance: () => StrategyLogicFlowViewportInstance | null;
}

export function useStrategyLogicFlowViewport(
  options: UseStrategyLogicFlowViewportOptions,
) {
  const zoomPercent = ref(100);

  let resizeObserver: ResizeObserver | null = null;
  let resizeAnimationFrameId = 0;
  let alignViewportAnimationFrameId = 0;
  let pendingForceResize = false;
  let isPointerResizeTracking = false;
  let lastCanvasWidth = 0;
  let lastCanvasHeight = 0;
  let fitViewBaseScale = 1;

  const normalizedFitViewPadding = computed(() => ({
    top: Math.max(0, options.fitViewPadding.value?.top ?? 0),
    right: Math.max(0, options.fitViewPadding.value?.right ?? 0),
    bottom: Math.max(0, options.fitViewPadding.value?.bottom ?? 0),
    left: Math.max(0, options.fitViewPadding.value?.left ?? 0),
  }));

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
    options.getInstance()?.setTheme?.(
      buildLogicFlowTheme(options.theme.value),
      options.theme.value === "dark" ? "dark" : "default",
    );
  };

  const resizeLogicFlowCanvas = (force = false) => {
    const logicFlowInstance = options.getInstance();
    if (logicFlowInstance === null || options.container.value === null) {
      return;
    }
    const nextWidth = options.container.value.clientWidth || 0;
    const nextHeight = options.container.value.clientHeight || 0;
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
    if (options.container.value === null) {
      return null;
    }

    const { top, right, bottom, left } = normalizedFitViewPadding.value;
    return [
      options.container.value.clientWidth / 2 + (left - right) / 2,
      options.container.value.clientHeight / 2 + (top - bottom) / 2,
    ];
  };

  const clampZoomPercent = (value: number) => Math.min(180, Math.max(60, value));

  const applyZoomPercent = (nextPercent: number) => {
    zoomPercent.value = clampZoomPercent(nextPercent);

    const logicFlowInstance = options.getInstance();
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

  const alignLogicFlowViewport = () => {
    const logicFlowInstance = options.getInstance();
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
    if (options.getInstance() === null) {
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

  const stopPointerResizeTracking = () => {
    if (!isPointerResizeTracking) {
      return;
    }
    isPointerResizeTracking = false;
    queueResizeLogicFlowCanvas(true);
  };

  const handlePanelPointerDown = (event: PointerEvent) => {
    if (!options.resizable.value || event.button !== 0) {
      return;
    }
    isPointerResizeTracking = true;
    queueResizeLogicFlowCanvas(true);
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

  onMounted(() => {
    if (typeof window !== "undefined") {
      window.addEventListener("pointerup", stopPointerResizeTracking);
      window.addEventListener("pointercancel", stopPointerResizeTracking);
    }

    if (options.container.value === null || typeof ResizeObserver === "undefined") {
      return;
    }

    resizeObserver = new ResizeObserver(() => {
      queueResizeLogicFlowCanvas();
      queueAlignLogicFlowViewport();
    });
    if (options.panel.value !== null) {
      resizeObserver.observe(options.panel.value);
    }
    resizeObserver.observe(options.container.value);
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
  });

  watch(options.theme, () => {
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

  return {
    zoomPercent,
    applyLogicFlowTheme,
    handlePanelPointerDown,
    handleZoomSliderInput,
    queueAlignLogicFlowViewport,
    queueResizeLogicFlowCanvas,
    resetViewportZoom,
  };
}