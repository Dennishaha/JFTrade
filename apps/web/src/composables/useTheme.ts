import { type InjectionKey, type Ref, inject, provide, ref, watch } from "vue";
import { useTheme as useVuetifyTheme } from "vuetify";

export type ThemeMode = "dark" | "light";

const THEME_STORAGE_KEY = "jftrade.theme";

export interface ThemeStore {
  theme: Ref<ThemeMode>;
  toggle: () => void;
  set: (mode: ThemeMode) => void;
}

const themeKey: InjectionKey<ThemeStore> = Symbol("jftrade-theme");

function readInitialTheme(): ThemeMode {
  if (typeof window === "undefined") return "dark";
  const stored = window.localStorage?.getItem(THEME_STORAGE_KEY);
  if (stored === "light" || stored === "dark") return stored;
  return "dark";
}

function applyTheme(mode: ThemeMode): void {
  if (typeof document === "undefined") return;
  document.documentElement.dataset.theme = mode;
  document.documentElement.dataset.vuetifyTheme = mode;
  document.documentElement.style.colorScheme = mode;
  document.documentElement.classList.toggle("dark", mode === "dark");
}

function applyVuetifyTheme(mode: ThemeMode): void {
  try {
    const vuetifyTheme = useVuetifyTheme();
    vuetifyTheme.change(mode);
  } catch {
    // No Vuetify theme context in this component tree (e.g. isolated tests).
  }
}

export function provideThemeStore(): ThemeStore {
  const theme = ref<ThemeMode>(readInitialTheme());
  applyTheme(theme.value);
  applyVuetifyTheme(theme.value);

  watch(theme, (next) => {
    applyTheme(next);
    applyVuetifyTheme(next);
    if (typeof window !== "undefined" && window.localStorage != null) {
      window.localStorage.setItem(THEME_STORAGE_KEY, next);
    }
  });

  const store: ThemeStore = {
    theme,
    toggle: () => {
      theme.value = theme.value === "dark" ? "light" : "dark";
    },
    set: (mode) => {
      theme.value = mode;
    },
  };

  provide(themeKey, store);
  return store;
}

export function useTheme(): ThemeStore {
  const store = inject(themeKey);
  if (!store) {
    throw new Error(
      "Theme store not provided. Wrap your app with provideThemeStore().",
    );
  }
  return store;
}
