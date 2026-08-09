import { nextTick, onBeforeUnmount, ref, watch, type Ref } from "vue";

import type {
  ADKAgent,
  ADKProvider,
  ADKRun,
  ADKSessionComposerState,
} from "@/types";
import { normalizeSessionComposerState } from "./adkPageRunHistory";
import { saveADKSessionComposerState } from "./adkPageSessionApi";

const COMPOSER_STATE_SAVE_DELAY_MS = 600;

interface ComposerPersistenceInput {
  activeGoalRun: Ref<ADKRun | null | undefined>;
  agents: Ref<ADKAgent[]>;
  chatDraft: Ref<string>;
  effectiveWorkMode: Ref<string>;
  goalObjectiveDraft: Ref<string>;
  goalObjectiveError: Ref<string>;
  goalObjectiveTouched: Ref<boolean>;
  permissionModeOverride: Ref<string>;
  selectedAgentId: Ref<string>;
  selectedProvider: Ref<ADKProvider | null>;
  selectedProviderId: Ref<string>;
  selectedSessionId: Ref<string>;
  workModeOverride: Ref<string>;
}

export function useADKComposerPersistence(input: ComposerPersistenceInput) {
  const draftRevision = ref(0);
  let applyingComposerState = false;
  let composerSaveTimer: ReturnType<typeof window.setTimeout> | null = null;
  let composerDirty = false;
  let composerRevision = 0;
  let composerFlushPromise: Promise<void> | null = null;

  function defaultProviderIdForSelectedAgent(): string {
    return (
      input.agents.value.find(
        (agent) => agent.id === input.selectedAgentId.value,
      )?.providerId ?? ""
    ).trim();
  }

  function currentProviderOverride(): { providerId: string; model: string } {
    const selectedProviderId = input.selectedProviderId.value.trim();
    if (
      selectedProviderId === "" ||
      selectedProviderId === defaultProviderIdForSelectedAgent()
    ) {
      return { providerId: "", model: "" };
    }
    return {
      providerId: selectedProviderId,
      model: input.selectedProvider.value?.model?.trim() ?? "",
    };
  }

  function currentComposerState(sessionId: string): ADKSessionComposerState {
    const providerOverride = currentProviderOverride();
    return normalizeSessionComposerState(sessionId, {
      sessionId,
      chatDraft: input.chatDraft.value,
      providerIdOverride: providerOverride.providerId,
      modelOverride: providerOverride.model,
      workModeOverride: input.workModeOverride.value,
      permissionModeOverride: input.permissionModeOverride.value,
      goalObjectiveDraft: input.goalObjectiveDraft.value,
      goalObjectiveTouched: input.goalObjectiveTouched.value,
    });
  }

  function markComposerStateDirty(): void {
    composerDirty = true;
    composerRevision += 1;
    scheduleComposerStateSave();
  }

  function scheduleComposerStateSave(): void {
    if (composerSaveTimer !== null) window.clearTimeout(composerSaveTimer);
    composerSaveTimer = window.setTimeout(() => {
      composerSaveTimer = null;
      void flushComposerState();
    }, COMPOSER_STATE_SAVE_DELAY_MS);
  }

  async function flushComposerState(
    options: { keepalive?: boolean } = {},
  ): Promise<void> {
    if (composerSaveTimer !== null) {
      window.clearTimeout(composerSaveTimer);
      composerSaveTimer = null;
    }
    if (input.selectedSessionId.value.trim() === "" || !composerDirty) return;
    if (composerFlushPromise !== null) {
      await composerFlushPromise;
      if (!composerDirty) return;
    }
    while (composerDirty) {
      const sessionId = input.selectedSessionId.value.trim();
      if (sessionId === "") return;
      const revision = composerRevision;
      const state = currentComposerState(sessionId);
      const savePromise = saveADKSessionComposerState(
        sessionId,
        {
          chatDraft: state.chatDraft,
          providerIdOverride: state.providerIdOverride,
          modelOverride: state.modelOverride,
          workModeOverride: state.workModeOverride,
          permissionModeOverride: state.permissionModeOverride,
          goalObjectiveDraft: state.goalObjectiveDraft,
          goalObjectiveTouched: state.goalObjectiveTouched,
        },
        options,
      );
      const trackedPromise = savePromise.then(
        () => undefined,
        () => undefined,
      );
      composerFlushPromise = trackedPromise;
      try {
        await savePromise;
        composerDirty = revision !== composerRevision;
      } catch {
        composerDirty = true;
        return;
      } finally {
        if (composerFlushPromise === trackedPromise) {
          composerFlushPromise = null;
        }
      }
    }
  }

  function emptyComposerState(sessionId: string): ADKSessionComposerState {
    return {
      sessionId: sessionId.trim(),
      chatDraft: "",
      providerIdOverride: "",
      modelOverride: "",
      workModeOverride: "",
      permissionModeOverride: "",
      goalObjectiveDraft: "",
      goalObjectiveTouched: false,
      updatedAt: "",
    };
  }

  function applyComposerState(state: ADKSessionComposerState): void {
    const sessionId = input.selectedSessionId.value.trim();
    const normalized = normalizeSessionComposerState(sessionId, state);
    applyingComposerState = true;
    input.selectedProviderId.value =
      normalized.providerIdOverride || defaultProviderIdForSelectedAgent();
    input.workModeOverride.value = normalized.workModeOverride;
    input.permissionModeOverride.value = normalized.permissionModeOverride;
    input.chatDraft.value = normalized.chatDraft;
    input.goalObjectiveTouched.value = normalized.goalObjectiveTouched;
    input.goalObjectiveDraft.value =
      !normalized.goalObjectiveTouched && input.activeGoalRun.value?.objective
        ? input.activeGoalRun.value.objective
        : normalized.goalObjectiveDraft;
    input.goalObjectiveError.value = "";
    composerDirty = false;
    if (composerSaveTimer !== null) {
      window.clearTimeout(composerSaveTimer);
      composerSaveTimer = null;
    }
    void nextTick(() => {
      applyingComposerState = false;
    });
  }

  function resetComposerState(
    sessionId = input.selectedSessionId.value,
  ): void {
    applyComposerState(emptyComposerState(sessionId));
    if (sessionId.trim() === "") {
      composerDirty = false;
      return;
    }
    markComposerStateDirty();
  }

  watch(input.effectiveWorkMode, (mode) => {
    if (applyingComposerState || input.activeGoalRun.value) return;
    if (mode !== "loop") {
      input.goalObjectiveTouched.value = false;
      input.goalObjectiveDraft.value = "";
      input.goalObjectiveError.value = "";
    } else if (!input.goalObjectiveTouched.value) {
      input.goalObjectiveDraft.value = input.chatDraft.value;
    }
  });
  watch(input.chatDraft, () => {
    if (
      applyingComposerState ||
      input.activeGoalRun.value ||
      input.effectiveWorkMode.value !== "loop"
    ) {
      return;
    }
    if (!input.goalObjectiveTouched.value) {
      input.goalObjectiveDraft.value = input.chatDraft.value;
    }
  });
  watch(input.chatDraft, () => {
    draftRevision.value += 1;
  }, { flush: "sync" });
  watch(
    () => [
      input.chatDraft.value,
      input.selectedProviderId.value,
      input.workModeOverride.value,
      input.permissionModeOverride.value,
      input.goalObjectiveDraft.value,
      input.goalObjectiveTouched.value,
    ],
    () => {
      if (!applyingComposerState) markComposerStateDirty();
    },
  );

  const flushBeforeUnload = () => {
    void flushComposerState({ keepalive: true });
  };
  window.addEventListener("pagehide", flushBeforeUnload);
  window.addEventListener("beforeunload", flushBeforeUnload);
  onBeforeUnmount(() => {
    window.removeEventListener("pagehide", flushBeforeUnload);
    window.removeEventListener("beforeunload", flushBeforeUnload);
    if (composerSaveTimer !== null) {
      window.clearTimeout(composerSaveTimer);
      composerSaveTimer = null;
    }
    void flushComposerState();
  });

  return {
    applyComposerState,
    draftRevision,
    emptyComposerState,
    flushComposerState,
    markComposerStateDirty,
    resetComposerState,
  };
}
