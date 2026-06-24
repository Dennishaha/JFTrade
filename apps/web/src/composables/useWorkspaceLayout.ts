import {
  type InjectionKey,
  type Ref,
  computed,
  inject,
  provide,
  ref,
  watch,
} from "vue";

import { normalizeKlinePeriod } from "../charting/kline";

const LEGACY_STORAGE_KEY = "jftrade.workspace.layout.v1";
const VIEW_STORAGE_KEY = "jftrade.workspace.view.v1";
const TRADING_STORAGE_KEY = "jftrade.workspace.trading.v1";

export type WorkspacePaneSizeKey =
  | "main"
  | "leftColumn"
  | "bottom"
  | "rightColumn";

export type WorkspacePaneSizes = Record<WorkspacePaneSizeKey, [number, number]>;

export type WorkspaceRightDockTab = "notifications" | "ai";

export interface WorkspaceViewState {
  rightDockOpen: boolean;
  rightDockTab: WorkspaceRightDockTab;
  rightDockSize: number;
  paneSizes: WorkspacePaneSizes;
}

export type WorkspaceViewStatePatch = Partial<
  Omit<WorkspaceViewState, "paneSizes">
> & {
  paneSizes?: Partial<WorkspacePaneSizes>;
};

export interface WorkspaceTradingPreferences {
  symbol: string;
  market: string;
  period: string;
  selectedBrokerAccountKey: string | null;
  favoriteBrokerAccountKeys: string[];
}

export type WorkspaceTradingPreferencesPatch =
  Partial<WorkspaceTradingPreferences>;

const defaultPaneSizes: WorkspacePaneSizes = {
  main: [72, 28],
  leftColumn: [60, 40],
  bottom: [60, 40],
  rightColumn: [60, 40],
};

const defaultViewState: WorkspaceViewState = {
  rightDockOpen: false,
  rightDockTab: "notifications",
  rightDockSize: 28,
  paneSizes: { ...defaultPaneSizes },
};

const defaultTradingPreferences: WorkspaceTradingPreferences = {
  symbol: "00700",
  market: "HK",
  period: "1m",
  selectedBrokerAccountKey: null,
  favoriteBrokerAccountKeys: [],
};

function normalizeFavoriteBrokerAccountKeys(
  keys: string[] | null | undefined,
): string[] {
  if (!Array.isArray(keys)) {
    return [];
  }

  const deduped = new Set<string>();
  for (const key of keys) {
    if (typeof key !== "string") {
      continue;
    }
    const normalized = key.trim();
    if (normalized === "") {
      continue;
    }
    deduped.add(normalized);
  }
  return Array.from(deduped);
}

function clonePaneSizes(paneSizes: WorkspacePaneSizes): WorkspacePaneSizes {
  return {
    main: [...paneSizes.main],
    leftColumn: [...paneSizes.leftColumn],
    bottom: [...paneSizes.bottom],
    rightColumn: [...paneSizes.rightColumn],
  };
}

function createDefaultViewState(): WorkspaceViewState {
  return {
    ...defaultViewState,
    paneSizes: clonePaneSizes(defaultPaneSizes),
  };
}

function createDefaultTradingPreferences(): WorkspaceTradingPreferences {
  return {
    ...defaultTradingPreferences,
    favoriteBrokerAccountKeys: [
      ...defaultTradingPreferences.favoriteBrokerAccountKeys,
    ],
  };
}

function normalizePaneSizePair(
  input: unknown,
  fallback: [number, number],
): [number, number] {
  if (!Array.isArray(input) || input.length !== 2) {
    return [...fallback];
  }

  const first = Number(input[0]);
  const second = Number(input[1]);
  if (!Number.isFinite(first) || !Number.isFinite(second)) {
    return [...fallback];
  }
  if (first <= 0 || second <= 0 || first > 100 || second > 100) {
    return [...fallback];
  }
  if (Math.abs(first + second - 100) > 0.5) {
    return [...fallback];
  }

  return [first, second];
}

function normalizePaneSizes(input: unknown): WorkspacePaneSizes {
  const source =
    input != null && typeof input === "object"
      ? (input as Partial<Record<WorkspacePaneSizeKey, unknown>>)
      : {};

  return {
    main: normalizePaneSizePair(source.main, defaultPaneSizes.main),
    leftColumn: normalizePaneSizePair(
      source.leftColumn,
      defaultPaneSizes.leftColumn,
    ),
    bottom: normalizePaneSizePair(source.bottom, defaultPaneSizes.bottom),
    rightColumn: normalizePaneSizePair(
      source.rightColumn,
      defaultPaneSizes.rightColumn,
    ),
  };
}

function normalizeRightDockSize(input: unknown): number {
  const value = Number(input);
  if (!Number.isFinite(value)) {
    return defaultViewState.rightDockSize;
  }
  return Math.min(48, Math.max(18, value));
}

function normalizeRightDockTab(input: unknown): WorkspaceRightDockTab {
  return input === "ai" ? input : "notifications";
}

function normalizeViewState(
  input: Partial<WorkspaceViewState>,
): WorkspaceViewState {
  const merged = { ...createDefaultViewState(), ...input };
  return {
    rightDockOpen: merged.rightDockOpen === true,
    rightDockTab: normalizeRightDockTab(merged.rightDockTab),
    rightDockSize: normalizeRightDockSize(merged.rightDockSize),
    paneSizes: normalizePaneSizes(input.paneSizes),
  };
}

function normalizeTradingPreferences(
  input: Partial<WorkspaceTradingPreferences>,
): WorkspaceTradingPreferences {
  const merged = { ...createDefaultTradingPreferences(), ...input };
  return {
    market: merged.market.trim().toUpperCase(),
    symbol: merged.symbol.trim().toUpperCase(),
    period: normalizeKlinePeriod(merged.period),
    selectedBrokerAccountKey:
      typeof merged.selectedBrokerAccountKey === "string" &&
      merged.selectedBrokerAccountKey.trim() !== ""
        ? merged.selectedBrokerAccountKey.trim()
        : null,
    favoriteBrokerAccountKeys: normalizeFavoriteBrokerAccountKeys(
      merged.favoriteBrokerAccountKeys,
    ),
  };
}

function readStorage(storage: Storage | null | undefined, key: string): string | null {
  try {
    return storage?.getItem(key) ?? null;
  } catch {
    return null;
  }
}

function writeStorage(
  storage: Storage | null | undefined,
  key: string,
  value: WorkspaceViewState | WorkspaceTradingPreferences,
): void {
  try {
    storage?.setItem(key, JSON.stringify(value));
  } catch {
    // Ignore unavailable or full browser storage; the in-memory ref remains valid.
  }
}

function readLegacyState():
  | {
      view: WorkspaceViewState;
      trading: WorkspaceTradingPreferences;
    }
  | null {
  if (typeof window === "undefined") {
    return null;
  }

  try {
    const raw =
      readStorage(window.sessionStorage, LEGACY_STORAGE_KEY) ??
      readStorage(window.localStorage, LEGACY_STORAGE_KEY);
    if (!raw) {
      return null;
    }

    const parsed = JSON.parse(raw) as Partial<
      WorkspaceViewState & WorkspaceTradingPreferences
    >;
    return {
      view: normalizeViewState(parsed),
      trading: normalizeTradingPreferences(parsed),
    };
  } catch {
    return null;
  }
}

function readInitialViewState(): WorkspaceViewState {
  if (typeof window === "undefined") {
    return createDefaultViewState();
  }

  try {
    const raw =
      readStorage(window.sessionStorage, VIEW_STORAGE_KEY) ??
      readStorage(window.localStorage, VIEW_STORAGE_KEY);
    if (raw) {
      return normalizeViewState(JSON.parse(raw) as Partial<WorkspaceViewState>);
    }
  } catch {
    // Fall back to legacy/default state below.
  }

  return readLegacyState()?.view ?? createDefaultViewState();
}

function readInitialTradingPreferences(): WorkspaceTradingPreferences {
  if (typeof window === "undefined") {
    return createDefaultTradingPreferences();
  }

  try {
    const raw =
      readStorage(window.sessionStorage, TRADING_STORAGE_KEY) ??
      readStorage(window.localStorage, TRADING_STORAGE_KEY);
    if (raw) {
      return normalizeTradingPreferences(
        JSON.parse(raw) as Partial<WorkspaceTradingPreferences>,
      );
    }
  } catch {
    // Fall back to legacy/default state below.
  }

  return readLegacyState()?.trading ?? createDefaultTradingPreferences();
}

export interface WorkspaceViewStateStore {
  prefs: Ref<WorkspaceViewState>;
  update: (patch: WorkspaceViewStatePatch) => void;
  reset: () => void;
}

export interface WorkspaceTradingPreferencesStore {
  prefs: Ref<WorkspaceTradingPreferences>;
  update: (patch: WorkspaceTradingPreferencesPatch) => void;
  reset: () => void;
}

export type WorkspaceLayoutPreferences = WorkspaceViewState &
  WorkspaceTradingPreferences;
export type WorkspaceLayoutPatch = WorkspaceViewStatePatch &
  WorkspaceTradingPreferencesPatch;

export interface WorkspaceLayoutStore {
  prefs: Ref<WorkspaceLayoutPreferences>;
  update: (patch: WorkspaceLayoutPatch) => void;
  reset: () => void;
}

const viewStateKey: InjectionKey<WorkspaceViewStateStore> = Symbol(
  "jftrade-workspace-view-state",
);
const tradingPrefsKey: InjectionKey<WorkspaceTradingPreferencesStore> = Symbol(
  "jftrade-workspace-trading-preferences",
);

export function provideWorkspaceViewStateStore(): WorkspaceViewStateStore {
  const prefs = ref<WorkspaceViewState>(readInitialViewState());

  watch(
    prefs,
    (next) => {
      if (typeof window === "undefined") {
        return;
      }
      writeStorage(window.sessionStorage, VIEW_STORAGE_KEY, next);
      writeStorage(window.localStorage, VIEW_STORAGE_KEY, next);
    },
    { deep: true, immediate: true },
  );

  const store: WorkspaceViewStateStore = {
    prefs,
    update: (patch) => {
      prefs.value = normalizeViewState({
        ...prefs.value,
        ...patch,
        paneSizes:
          patch.paneSizes == null
            ? prefs.value.paneSizes
            : {
                ...prefs.value.paneSizes,
                ...patch.paneSizes,
              },
      });
    },
    reset: () => {
      prefs.value = createDefaultViewState();
    },
  };

  provide(viewStateKey, store);
  return store;
}

export function provideWorkspaceTradingPreferencesStore():
  WorkspaceTradingPreferencesStore {
  const prefs = ref<WorkspaceTradingPreferences>(readInitialTradingPreferences());

  watch(
    prefs,
    (next) => {
      if (typeof window === "undefined") {
        return;
      }
      writeStorage(window.sessionStorage, TRADING_STORAGE_KEY, next);
      writeStorage(window.localStorage, TRADING_STORAGE_KEY, next);
    },
    { deep: true, immediate: true },
  );

  const store: WorkspaceTradingPreferencesStore = {
    prefs,
    update: (patch) => {
      prefs.value = normalizeTradingPreferences({
        ...prefs.value,
        ...patch,
      });
    },
    reset: () => {
      prefs.value = createDefaultTradingPreferences();
    },
  };

  provide(tradingPrefsKey, store);
  return store;
}

export function provideWorkspaceLayoutStore(): WorkspaceLayoutStore {
  const view = provideWorkspaceViewStateStore();
  const trading = provideWorkspaceTradingPreferencesStore();
  const prefs = computed<WorkspaceLayoutPreferences>(() => ({
    ...view.prefs.value,
    ...trading.prefs.value,
  }));

  watch(
    prefs,
    (next) => {
      if (typeof window === "undefined") {
        return;
      }
      writeStorage(window.sessionStorage, LEGACY_STORAGE_KEY, next);
      writeStorage(window.localStorage, LEGACY_STORAGE_KEY, next);
    },
    { deep: true, immediate: true },
  );

  return {
    prefs,
    update: (patch) => {
      view.update(patch);
      trading.update(patch);
    },
    reset: () => {
      view.reset();
      trading.reset();
    },
  };
}

export function useWorkspaceViewState(): WorkspaceViewStateStore {
  const store = inject(viewStateKey);
  if (!store) {
    throw new Error("Workspace view state store not provided.");
  }
  return store;
}

export function useWorkspaceTradingPrefs(): WorkspaceTradingPreferencesStore {
  const store = inject(tradingPrefsKey);
  if (!store) {
    throw new Error("Workspace trading preferences store not provided.");
  }
  return store;
}
