import { computed, type ComputedRef, type InjectionKey, type Ref, inject, provide, ref, watch } from "vue";

import type { ThemeMode } from "./useTheme";

const STORAGE_KEY = "jftrade.ui.colors.v1";

export interface UIColorPreferences {
  upColor: string;
  downColor: string;
}

export interface UIColorPreferencesStore {
  prefs: Ref<UIColorPreferences>;
  resolved: ComputedRef<UIColorPreferences>;
  update: (patch: Partial<UIColorPreferences>) => void;
  setRiseIsRed: (enabled: boolean) => void;
  reset: () => void;
}

type ThemeInput = Ref<ThemeMode> | { theme: Ref<ThemeMode> };

const darkDefaults: UIColorPreferences = {
  upColor: "#16c784",
  downColor: "#ea3943",
};

const lightDefaults: UIColorPreferences = {
  upColor: "#15803d",
  downColor: "#b91c1c",
};

const fallbackPreferences: UIColorPreferences = { ...darkDefaults };
const colorKey: InjectionKey<UIColorPreferencesStore> = Symbol("jftrade-ui-colors");

export function resolveDirectionalColors(
  theme: ThemeMode,
  preferMainlandConvention = false,
): UIColorPreferences {
  const defaults = theme === "light" ? lightDefaults : darkDefaults;
  if (!preferMainlandConvention) {
    return { ...defaults };
  }

  return {
    upColor: defaults.downColor,
    downColor: defaults.upColor,
  };
}

export function hexToRgba(hex: string, alpha: number): string {
  const normalized = normalizeHexColor(hex, fallbackPreferences.upColor);
  const red = Number.parseInt(normalized.slice(1, 3), 16);
  const green = Number.parseInt(normalized.slice(3, 5), 16);
  const blue = Number.parseInt(normalized.slice(5, 7), 16);
  const clampedAlpha = Math.min(1, Math.max(0, alpha));
  return `rgba(${red}, ${green}, ${blue}, ${clampedAlpha})`;
}

function normalizeHexColor(value: unknown, fallback: string): string {
  if (typeof value !== "string") {
    return fallback;
  }
  const trimmed = value.trim();
  if (/^#[0-9a-fA-F]{6}$/.test(trimmed)) {
    return trimmed.toLowerCase();
  }
  return fallback;
}

function readInitial(): UIColorPreferences {
  if (typeof window === "undefined") {
    return { ...fallbackPreferences };
  }

  try {
    const raw = window.localStorage?.getItem(STORAGE_KEY);
    if (raw == null || raw.trim() === "") {
      return { ...fallbackPreferences };
    }
    const parsed = JSON.parse(raw) as Partial<UIColorPreferences>;
    return {
      upColor: normalizeHexColor(parsed.upColor, fallbackPreferences.upColor),
      downColor: normalizeHexColor(parsed.downColor, fallbackPreferences.downColor),
    };
  } catch {
    return { ...fallbackPreferences };
  }
}

function applyDirectionalColorVariables(colors: UIColorPreferences): void {
  if (typeof document === "undefined") return;
  document.documentElement.style.setProperty("--tv-up", colors.upColor);
  document.documentElement.style.setProperty("--tv-down", colors.downColor);
}

export function provideUIColorPreferencesStore(
  themeInput?: ThemeInput,
): UIColorPreferencesStore {
  const theme = "theme" in (themeInput ?? {})
    ? (themeInput as { theme: Ref<ThemeMode> }).theme
    : themeInput as Ref<ThemeMode> | undefined;
  const prefs = ref<UIColorPreferences>(readInitial());
  const resolved = computed(() => {
    const defaults = resolveDirectionalColors(theme?.value ?? "dark");
    return {
      upColor: normalizeHexColor(prefs.value.upColor, defaults.upColor),
      downColor: normalizeHexColor(prefs.value.downColor, defaults.downColor),
    };
  });

  watch(
    prefs,
    (next) => {
      if (typeof window === "undefined" || window.localStorage == null) return;
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
    },
    { deep: true },
  );

  watch(resolved, applyDirectionalColorVariables, { immediate: true });

  const store: UIColorPreferencesStore = {
    prefs,
    resolved,
    update: (patch) => {
      prefs.value = {
        upColor: normalizeHexColor(patch.upColor, prefs.value.upColor),
        downColor: normalizeHexColor(patch.downColor, prefs.value.downColor),
      };
    },
    setRiseIsRed: (enabled) => {
      prefs.value = resolveDirectionalColors(theme?.value ?? "dark", enabled);
    },
    reset: () => {
      prefs.value = resolveDirectionalColors(theme?.value ?? "dark");
    },
  };

  provide(colorKey, store);
  return store;
}

export function useUIColorPreferences(): UIColorPreferencesStore {
  const store = inject(colorKey);
  if (!store) {
    throw new Error("UI color preferences store not provided.");
  }
  return store;
}

export function tryUseUIColorPreferences(): UIColorPreferencesStore | null {
  return inject(colorKey, null);
}