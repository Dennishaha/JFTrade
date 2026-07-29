import {
  computed,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
} from "vue";
import type { Ref } from "vue";
import type { SplitpanesResizedPayload } from "splitpanes";

type MermaidApi = typeof import("mermaid")["default"];
const timelineRenderWindow = 240;

export function resolveADKWorkspacePaneSizes(
  payload: SplitpanesResizedPayload,
): [number, number] | null {
  const sizes = payload.panes?.map((pane) => pane.size);
  if (
    sizes == null ||
    sizes.length !== 2 ||
    !sizes.every((size) => Number.isFinite(size) && size > 0 && size <= 100)
  ) return null;
  return [sizes[0]!, sizes[1]!];
}

export function useADKResponsiveLayout(
  requestedLayout: () => "desktop" | "mobile",
) {
  const isNarrowViewport = ref(false);
  let mediaQuery: MediaQueryList | null = null;
  const effectiveLayout = computed<"desktop" | "mobile">(() =>
    requestedLayout() === "mobile" || isNarrowViewport.value
      ? "mobile"
      : "desktop",
  );

  function syncViewport(event: MediaQueryListEvent | MediaQueryList): void {
    isNarrowViewport.value = event.matches;
  }

  onMounted(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
      return;
    }
    mediaQuery = window.matchMedia("(max-width: 768px)");
    isNarrowViewport.value = mediaQuery.matches;
    if (typeof mediaQuery.addEventListener === "function") {
      mediaQuery.addEventListener("change", syncViewport);
    } else {
      mediaQuery.addListener(syncViewport);
    }
  });

  onBeforeUnmount(() => {
    if (!mediaQuery) return;
    if (typeof mediaQuery.removeEventListener === "function") {
      mediaQuery.removeEventListener("change", syncViewport);
    } else {
      mediaQuery.removeListener(syncViewport);
    }
    mediaQuery = null;
  });
  return { effectiveLayout };
}

export function useADKTimelineWindow<T>(entries: Ref<readonly T[]>) {
  const timelineRenderOffset = ref(0);
  const timelineWindowEnd = computed(() =>
    Math.max(
      0,
      Math.min(entries.value.length, entries.value.length - timelineRenderOffset.value),
    ),
  );
  const timelineWindowStart = computed(() =>
    Math.max(0, timelineWindowEnd.value - timelineRenderWindow),
  );
  const renderTimelineEntries = computed(() =>
    entries.value.slice(timelineWindowStart.value, timelineWindowEnd.value),
  );
  const timelineAtLatest = computed(() => timelineRenderOffset.value === 0);

  function clampTimelineRenderOffset(): void {
    const maxOffset = Math.max(0, entries.value.length - timelineRenderWindow);
    timelineRenderOffset.value = Math.min(timelineRenderOffset.value, maxOffset);
  }
  function showOlderTimelineWindow(): void {
    timelineRenderOffset.value = Math.min(
      Math.max(0, entries.value.length - timelineRenderWindow),
      timelineRenderOffset.value + timelineRenderWindow,
    );
  }
  function showNewerTimelineWindow(): void {
    timelineRenderOffset.value = Math.max(
      0,
      timelineRenderOffset.value - timelineRenderWindow,
    );
  }
  function showLatestTimelineWindow(): void {
    timelineRenderOffset.value = 0;
  }
  return {
    clampTimelineRenderOffset,
    renderTimelineEntries,
    showLatestTimelineWindow,
    showNewerTimelineWindow,
    showOlderTimelineWindow,
    timelineAtLatest,
    timelineRenderOffset,
    timelineWindowEnd,
    timelineWindowStart,
  };
}

export function useADKMermaidRenderer<
  T extends { id: unknown; runId?: unknown; status?: unknown; text?: unknown },
>(threadRef: Ref<HTMLElement | null>, entries: Ref<readonly T[]>): void {
  let renderFrame: number | null = null;
  let mermaidModule: MermaidApi | null = null;
  let modulePromise: Promise<MermaidApi> | null = null;
  const signature = computed(() =>
    entries.value
      .filter((entry) => String(entry.text ?? "").includes("```mermaid"))
      .map((entry) =>
        [entry.id, entry.runId ?? "", entry.status ?? "", entry.text ?? ""].join("\u0000"),
      )
      .join("\u0001"),
  );

  async function loadMermaid(): Promise<MermaidApi> {
    if (mermaidModule) return mermaidModule;
    modulePromise ??= import("mermaid").then((module) => {
      module.default.initialize({ startOnLoad: false, securityLevel: "strict" });
      mermaidModule = module.default;
      return module.default;
    });
    return modulePromise;
  }

  async function renderDiagrams(): Promise<void> {
    const nodes = threadRef.value?.querySelectorAll<HTMLElement>(".mermaid");
    if (!nodes || nodes.length === 0) return;
    try {
      const mermaid = await loadMermaid();
      await mermaid.run({ nodes, suppressErrors: true });
    } catch (error) {
      console.warn("Failed to render mermaid diagrams", error);
    }
  }

  function scheduleRender(): void {
    if (typeof window === "undefined" || renderFrame !== null) return;
    renderFrame = window.requestAnimationFrame(() => {
      renderFrame = null;
      void renderDiagrams();
    });
  }

  onMounted(() => {
    if (signature.value !== "") scheduleRender();
  });
  watch(signature, (value) => {
    if (value !== "") scheduleRender();
  }, { flush: "post" });
  onBeforeUnmount(() => {
    if (renderFrame !== null) window.cancelAnimationFrame(renderFrame);
  });
}
