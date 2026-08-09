import { watch, type Ref } from "vue";

import {
  emptyADKSessionRuntimeState,
  readADKPagePersistentState,
  writeADKPagePersistentState,
  type ADKSessionRuntimeState,
} from "./adkPagePersistence";

interface PageRuntimePersistenceInput {
  activeChildRunId: Ref<string>;
  selectedSessionId: Ref<string>;
}

export function useADKPageRuntimePersistence(
  input: PageRuntimePersistenceInput,
) {
  const pageState = readADKPagePersistentState();

  function sessionRuntimeState(sessionId: string): ADKSessionRuntimeState {
    const normalized = sessionId.trim();
    if (normalized === "") return emptyADKSessionRuntimeState();
    pageState.sessions[normalized] ??= emptyADKSessionRuntimeState();
    return pageState.sessions[normalized]!;
  }

  function updateSessionRuntimeState(
    sessionId: string,
    patch: Partial<ADKSessionRuntimeState>,
  ): void {
    const normalized = sessionId.trim();
    if (normalized === "") return;
    pageState.sessions[normalized] = {
      ...sessionRuntimeState(normalized),
      ...patch,
    };
    writeADKPagePersistentState(pageState);
  }

  function clearSessionRuntimeState(sessionId: string): void {
    updateSessionRuntimeState(sessionId, {
      streamId: "",
      runId: "",
      sequence: 0,
    });
  }

  function removeSessionRuntimeState(sessionId: string): void {
    const normalized = sessionId.trim();
    if (normalized === "") return;
    delete pageState.sessions[normalized];
    if (pageState.selectedSessionId === normalized) {
      pageState.selectedSessionId = "";
    }
    writeADKPagePersistentState(pageState);
  }

  watch(input.activeChildRunId, (runId) => {
    const sessionId = input.selectedSessionId.value.trim();
    if (sessionId !== "") {
      updateSessionRuntimeState(sessionId, { activeChildRunId: runId });
    }
  });
  watch(input.selectedSessionId, (sessionId) => {
    pageState.selectedSessionId = sessionId.trim();
    writeADKPagePersistentState(pageState);
  });

  return {
    clearSessionRuntimeState,
    pageState,
    removeSessionRuntimeState,
    sessionRuntimeState,
    updateSessionRuntimeState,
  };
}
