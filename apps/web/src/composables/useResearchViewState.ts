export interface ResearchViewState {
  railCollapsed: boolean;
  paneSizes: [number, number];
}

export interface ResearchPaneBounds {
  leftMinSize: number;
  leftMaxSize: number;
  railMinSize: number;
  railMaxSize: number;
}

const RESEARCH_VIEW_STORAGE_KEY = "jftrade.research.view.v1";
const RESEARCH_RAIL_MIN_WIDTH = 360;
const RESEARCH_RAIL_MAX_WIDTH = 520;
const RESEARCH_SPLITTER_WIDTH = 6;
const DEFAULT_RESEARCH_VIEW_STATE: ResearchViewState = {
  railCollapsed: false,
  paneSizes: [72, 28],
};

function normalizePaneSizes(value: unknown): [number, number] {
  if (!Array.isArray(value) || value.length !== 2) {
    return [...DEFAULT_RESEARCH_VIEW_STATE.paneSizes];
  }
  const left = Number(value[0]);
  const right = Number(value[1]);
  if (
    !Number.isFinite(left) ||
    !Number.isFinite(right) ||
    left < 45 ||
    left > 85 ||
    right < 15 ||
    right > 55 ||
    Math.abs(left + right - 100) > 0.5
  ) {
    return [...DEFAULT_RESEARCH_VIEW_STATE.paneSizes];
  }
  return [left, right];
}

export function researchPaneBoundsForWidth(
  containerWidth: number,
): ResearchPaneBounds {
  if (!Number.isFinite(containerWidth) || containerWidth <= 0) {
    return {
      leftMinSize: 45,
      leftMaxSize: 78,
      railMinSize: 22,
      railMaxSize: 55,
    };
  }
  const paneWidth = Math.max(1, containerWidth - RESEARCH_SPLITTER_WIDTH);
  const railMinSize = Math.min(
    55,
    (RESEARCH_RAIL_MIN_WIDTH / paneWidth) * 100,
  );
  const railMaxSize = Math.min(
    55,
    Math.max(
      railMinSize,
      (RESEARCH_RAIL_MAX_WIDTH / paneWidth) * 100,
    ),
  );
  return {
    leftMinSize: 100 - railMaxSize,
    leftMaxSize: 100 - railMinSize,
    railMinSize,
    railMaxSize,
  };
}

export function clampResearchPaneSizesForWidth(
  paneSizes: [number, number],
  containerWidth: number,
): [number, number] {
  const bounds = researchPaneBoundsForWidth(containerWidth);
  const railSize = Math.min(
    bounds.railMaxSize,
    Math.max(bounds.railMinSize, paneSizes[1]),
  );
  return [100 - railSize, railSize];
}

function normalizeResearchViewState(value: unknown): ResearchViewState {
  const candidate =
    value != null && typeof value === "object"
      ? (value as Partial<ResearchViewState>)
      : {};
  return {
    railCollapsed:
      typeof candidate.railCollapsed === "boolean"
        ? candidate.railCollapsed
        : DEFAULT_RESEARCH_VIEW_STATE.railCollapsed,
    paneSizes: normalizePaneSizes(candidate.paneSizes),
  };
}

function readStorage(storage: Storage | null | undefined): unknown {
  if (storage == null) return null;
  const raw = storage.getItem(RESEARCH_VIEW_STORAGE_KEY);
  if (raw == null || raw.trim() === "") return null;
  try {
    return JSON.parse(raw);
  } catch {
    return null;
  }
}

export function readResearchViewState(): ResearchViewState {
  if (typeof window === "undefined") {
    return normalizeResearchViewState(null);
  }
  return normalizeResearchViewState(
    readStorage(window.sessionStorage) ?? readStorage(window.localStorage),
  );
}

export function writeResearchViewState(state: ResearchViewState): void {
  if (typeof window === "undefined") return;
  const normalized = normalizeResearchViewState(state);
  const serialized = JSON.stringify(normalized);
  try {
    window.sessionStorage.setItem(RESEARCH_VIEW_STORAGE_KEY, serialized);
  } catch {
    // A storage outage must not prevent the research page from working.
  }
  try {
    window.localStorage.setItem(RESEARCH_VIEW_STORAGE_KEY, serialized);
  } catch {
    // Keep the in-memory state when persistent storage is unavailable.
  }
}

export { RESEARCH_VIEW_STORAGE_KEY };
