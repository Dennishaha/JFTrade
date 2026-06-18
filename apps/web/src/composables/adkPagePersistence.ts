import { readLocalStorage, writeLocalStorage } from "./safeStorage";

const ADK_PAGE_STATE_STORAGE_KEY = "jftrade.adk.page-state.v1";

export interface ADKSessionRuntimeState {
  streamId: string;
  runId: string;
  sequence: number;
  activeChildRunId: string;
}

export interface ADKPagePersistentState {
  selectedSessionId: string;
  sessions: Record<string, ADKSessionRuntimeState>;
}

export function readADKPagePersistentState(): ADKPagePersistentState {
  const empty: ADKPagePersistentState = {
    selectedSessionId: "",
    sessions: {},
  };
  const raw = readLocalStorage(ADK_PAGE_STATE_STORAGE_KEY);
  if (!raw) return empty;
  try {
    const parsed = JSON.parse(raw) as Partial<ADKPagePersistentState>;
    const sessions: Record<string, ADKSessionRuntimeState> = {};
    for (const [sessionId, state] of Object.entries(parsed.sessions ?? {})) {
      sessions[sessionId] = normalizeSessionRuntimeState(state);
    }
    return {
      selectedSessionId: String(parsed.selectedSessionId ?? "").trim(),
      sessions,
    };
  } catch {
    return empty;
  }
}

export function writeADKPagePersistentState(
  state: ADKPagePersistentState,
): void {
  writeLocalStorage(ADK_PAGE_STATE_STORAGE_KEY, JSON.stringify(state));
}

export function emptyADKSessionRuntimeState(): ADKSessionRuntimeState {
  return {
    streamId: "",
    runId: "",
    sequence: 0,
    activeChildRunId: "",
  };
}

function normalizeSessionRuntimeState(
  state: Partial<ADKSessionRuntimeState> | null | undefined,
): ADKSessionRuntimeState {
  return {
    streamId: String(state?.streamId ?? "").trim(),
    runId: String(state?.runId ?? "").trim(),
    sequence:
      Number.isFinite(state?.sequence) && (state?.sequence ?? 0) > 0
        ? Math.floor(state!.sequence!)
        : 0,
    activeChildRunId: String(state?.activeChildRunId ?? "").trim(),
  };
}
