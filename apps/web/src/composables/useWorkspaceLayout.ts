import { type InjectionKey, type Ref, inject, provide, ref, watch } from "vue";

import { normalizeKlinePeriod } from "../charting/kline";

const STORAGE_KEY = "jftrade.workspace.layout.v1";

export type WorkspacePaneSizeKey =
  | "main"
  | "leftColumn"
  | "bottom"
  | "rightColumn";

export type WorkspacePaneSizes = Record<WorkspacePaneSizeKey, [number, number]>;

export interface WorkspacePreferences {
  rightDockOpen: boolean;
  rightDockTab: "notifications" | "ai" | "context";
  symbol: string;
  market: string;
  period: string;
  selectedBrokerAccountKey: string | null;
  favoriteBrokerAccountKeys: string[];
  paneSizes: WorkspacePaneSizes;
}

export type WorkspacePreferencesPatch = Partial<
  Omit<WorkspacePreferences, "paneSizes">
> & {
  paneSizes?: Partial<WorkspacePaneSizes>;
};

const defaultPaneSizes: WorkspacePaneSizes = {
  main: [72, 28],
  leftColumn: [60, 40],
  bottom: [60, 40],
  rightColumn: [60, 40],
};

const defaults: WorkspacePreferences = {
  rightDockOpen: false,
  rightDockTab: "notifications",
  symbol: "00700",
  market: "HK",
  period: "1m",
  selectedBrokerAccountKey: null,
  favoriteBrokerAccountKeys: [],
  paneSizes: { ...defaultPaneSizes },
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

function createDefaults(): WorkspacePreferences {
  return {
    ...defaults,
    favoriteBrokerAccountKeys: [...defaults.favoriteBrokerAccountKeys],
    paneSizes: clonePaneSizes(defaultPaneSizes),
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

function normalizePreferences(
  input: Partial<WorkspacePreferences>,
): WorkspacePreferences {
  const merged = { ...createDefaults(), ...input };
  return {
    ...merged,
    period: normalizeKlinePeriod(merged.period),
    market: merged.market.trim().toUpperCase(),
    symbol: merged.symbol.trim().toUpperCase(),
    favoriteBrokerAccountKeys: normalizeFavoriteBrokerAccountKeys(
      merged.favoriteBrokerAccountKeys,
    ),
    paneSizes: normalizePaneSizes(input.paneSizes),
  };
}

function readStorage(storage: Storage | null | undefined): string | null {
  try {
    return storage?.getItem(STORAGE_KEY) ?? null;
  } catch {
    return null;
  }
}

function writeStorage(
  storage: Storage | null | undefined,
  prefs: WorkspacePreferences,
): void {
  try {
    storage?.setItem(STORAGE_KEY, JSON.stringify(prefs));
  } catch {
    // Ignore unavailable or full browser storage; the in-memory ref remains valid.
  }
}

export interface WorkspaceLayoutStore {
  prefs: Ref<WorkspacePreferences>;
  update: (patch: WorkspacePreferencesPatch) => void;
  reset: () => void;
}

const layoutKey: InjectionKey<WorkspaceLayoutStore> = Symbol("jftrade-layout");

function readInitial(): WorkspacePreferences {
  if (typeof window === "undefined") return createDefaults();
  try {
    const raw =
      readStorage(window.sessionStorage) ?? readStorage(window.localStorage);
    if (!raw) return createDefaults();
    const parsed = JSON.parse(raw) as Partial<WorkspacePreferences>;
    return normalizePreferences(parsed);
  } catch {
    return createDefaults();
  }
}

export function provideWorkspaceLayoutStore(): WorkspaceLayoutStore {
  const prefs = ref<WorkspacePreferences>(readInitial());

  watch(
    prefs,
    (next) => {
      if (typeof window === "undefined") return;
      writeStorage(window.sessionStorage, next);
      writeStorage(window.localStorage, next);
    },
    { deep: true },
  );

  const store: WorkspaceLayoutStore = {
    prefs,
    update: (patch) => {
      prefs.value = normalizePreferences({
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
      prefs.value = createDefaults();
    },
  };

  provide(layoutKey, store);
  return store;
}

export function useWorkspaceLayout(): WorkspaceLayoutStore {
  const store = inject(layoutKey);
  if (!store) throw new Error("Workspace layout store not provided.");
  return store;
}
