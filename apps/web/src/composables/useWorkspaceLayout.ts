import { type InjectionKey, type Ref, inject, provide, ref, watch } from "vue";

import { normalizeKlinePeriod } from "../charting/kline";

const STORAGE_KEY = "jftrade.workspace.layout.v1";

export interface WorkspacePreferences {
  rightDockOpen: boolean;
  rightDockTab: "notifications" | "ai" | "context";
  symbol: string;
  market: string;
  period: string;
  selectedBrokerAccountKey: string | null;
  favoriteBrokerAccountKeys: string[];
}

const defaults: WorkspacePreferences = {
  rightDockOpen: false,
  rightDockTab: "notifications",
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

export interface WorkspaceLayoutStore {
  prefs: Ref<WorkspacePreferences>;
  update: (patch: Partial<WorkspacePreferences>) => void;
  reset: () => void;
}

const layoutKey: InjectionKey<WorkspaceLayoutStore> = Symbol("jftrade-layout");

function readInitial(): WorkspacePreferences {
  if (typeof window === "undefined") return { ...defaults };
  try {
    const raw = window.localStorage?.getItem(STORAGE_KEY);
    if (!raw) return { ...defaults };
    const parsed = JSON.parse(raw) as Partial<WorkspacePreferences>;
    const merged = { ...defaults, ...parsed };
    return {
      ...merged,
      period: normalizeKlinePeriod(merged.period),
      market: merged.market.trim().toUpperCase(),
      symbol: merged.symbol.trim().toUpperCase(),
      favoriteBrokerAccountKeys: normalizeFavoriteBrokerAccountKeys(
        merged.favoriteBrokerAccountKeys,
      ),
    };
  } catch {
    return { ...defaults };
  }
}

export function provideWorkspaceLayoutStore(): WorkspaceLayoutStore {
  const prefs = ref<WorkspacePreferences>(readInitial());

  watch(
    prefs,
    (next) => {
      if (typeof window === "undefined" || window.localStorage == null) return;
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
    },
    { deep: true },
  );

  const store: WorkspaceLayoutStore = {
    prefs,
    update: (patch) => {
      prefs.value = { ...prefs.value, ...patch };
    },
    reset: () => {
      prefs.value = { ...defaults };
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
