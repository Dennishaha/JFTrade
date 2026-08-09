import { computed, ref, watch, type Ref } from "vue";

import type { ADKSessionContextSnapshot } from "@/types";
import {
  compactADKSessionContext,
  fetchADKSessionContext,
} from "./adkSessionContextApi";

const CONTEXT_REFRESH_THROTTLE_MS = 1000;

interface SessionContextInput {
  errorMessage: Ref<string>;
  reloadTimeline: (sessionId: string) => Promise<void>;
  selectedSessionId: Ref<string>;
}

export function useADKSessionContextState(input: SessionContextInput) {
  const contextBusy = ref(false);
  const contextDetailsOpen = ref(false);
  const sessionContext = ref<ADKSessionContextSnapshot | null>(null);
  const visibleSessionContext = computed(() => sessionContext.value);
  let lastContextRefreshAt = 0;

  function applySessionContext(
    incoming: ADKSessionContextSnapshot | null | undefined,
  ): void {
    if (!incoming) return;
    const current = sessionContext.value;
    if (!current || current.sessionId !== incoming.sessionId) {
      sessionContext.value = incoming;
      return;
    }
    const currentRevision = current.contextRevisionId?.trim() ?? "";
    const incomingRevision = incoming.contextRevisionId?.trim() ?? "";
    if (
      currentRevision === incomingRevision ||
      incoming.previousContextRevisionId?.trim() === currentRevision ||
      currentRevision === ""
    ) {
      sessionContext.value = incoming;
      return;
    }
    if (
      current.previousContextRevisionId?.trim() === incomingRevision ||
      incomingRevision === ""
    ) {
      return;
    }
    const currentCreatedAt = Date.parse(
      current.contextRevisionCreatedAt?.trim() ?? "",
    );
    const incomingCreatedAt = Date.parse(
      incoming.contextRevisionCreatedAt?.trim() ?? "",
    );
    if (
      Number.isFinite(currentCreatedAt) &&
      Number.isFinite(incomingCreatedAt) &&
      incomingCreatedAt < currentCreatedAt
    ) {
      return;
    }
    sessionContext.value = incoming;
  }

  async function refreshSessionContext(
    sessionId = input.selectedSessionId.value,
    showBusy = false,
  ): Promise<void> {
    if (!sessionId) {
      sessionContext.value = null;
      return;
    }
    lastContextRefreshAt = Date.now();
    if (showBusy) contextBusy.value = true;
    try {
      const context = await fetchADKSessionContext(sessionId);
      if (input.selectedSessionId.value === sessionId) {
        applySessionContext(context);
      }
    } catch {
      // Preserve the latest snapshot so the context indicator remains stable.
    } finally {
      if (showBusy) contextBusy.value = false;
    }
  }

  function scheduleSessionContextRefresh(
    sessionId = input.selectedSessionId.value,
  ): void {
    const normalized = sessionId.trim();
    if (
      normalized === "" ||
      Date.now() - lastContextRefreshAt < CONTEXT_REFRESH_THROTTLE_MS
    ) {
      return;
    }
    void refreshSessionContext(normalized);
  }

  function clearSessionContext(): void {
    sessionContext.value = null;
    contextDetailsOpen.value = false;
  }

  async function initializeSessionContext(sessionId: string): Promise<void> {
    await refreshSessionContext(sessionId, true);
  }

  function openContextDetails(): void {
    contextDetailsOpen.value = true;
  }

  async function runSlashCommand(
    command: "context" | "compact" | "compact-aggressive",
  ): Promise<void> {
    if (command === "context") {
      await refreshSessionContext();
      contextDetailsOpen.value = true;
      return;
    }
    await compactContext(command === "compact" ? "normal" : "aggressive");
  }

  async function compactContext(mode: "normal" | "aggressive"): Promise<void> {
    const sessionId = input.selectedSessionId.value.trim();
    if (sessionId === "") {
      input.errorMessage.value = "当前没有可压缩的会话";
      return;
    }
    contextBusy.value = true;
    try {
      applySessionContext(await compactADKSessionContext(sessionId, mode));
      contextDetailsOpen.value = true;
      await input.reloadTimeline(sessionId);
    } catch (error) {
      input.errorMessage.value =
        error instanceof Error ? error.message : "上下文压缩失败";
      try {
        await input.reloadTimeline(sessionId);
      } catch {
        // Preserve the explicit compaction error if refresh also fails.
      }
    } finally {
      contextBusy.value = false;
    }
  }

  watch(contextDetailsOpen, (open) => {
    if (open) void refreshSessionContext(undefined, true);
  });

  return {
    applySessionContext,
    clearSessionContext,
    compactContext,
    contextBusy,
    contextDetailsOpen,
    initializeSessionContext,
    openContextDetails,
    refreshSessionContext,
    runSlashCommand,
    scheduleSessionContextRefresh,
    sessionContext,
    visibleSessionContext,
  };
}
