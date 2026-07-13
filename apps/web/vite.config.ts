import tailwindcss from "@tailwindcss/vite";
import vue from "@vitejs/plugin-vue";
import vueDevTools from "vite-plugin-vue-devtools";
import { defineConfig } from "vitest/config";
import type { Plugin } from "vite";

type RuntimeProcess = {
  env?: Record<string, string | undefined>;
  platform?: string;
};

function resolveLaunchEditor(): string | null {
  const runtimeProcess = (
    globalThis as typeof globalThis & {
      [key: string]: RuntimeProcess | undefined;
    }
  )["process"];
  const launchEditorFromEnv = runtimeProcess?.env?.LAUNCH_EDITOR;

  if (launchEditorFromEnv) {
    return launchEditorFromEnv;
  }

  if (runtimeProcess?.platform !== "darwin") {
    return null;
  }

  return "/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code";
}

const launchEditor = resolveLaunchEditor();
let devToolsOptions: Parameters<typeof vueDevTools>[0];

if (typeof launchEditor === "string") {
  devToolsOptions = { launchEditor } as NonNullable<
    Parameters<typeof vueDevTools>[0]
  >;
}

const developmentApiTarget = "http://127.0.0.1:3000";
const developmentDocsTarget = "http://127.0.0.1:3001";
const apiProxyTargets = ["/api", "/swagger"];

type ProxyEventEmitter = {
  on: (event: string, handler: (...args: unknown[]) => void) => void;
};

function createProxyEntry(target: string) {
  return {
    changeOrigin: true,
    headers: { Origin: target },
    target,
    ws: true,
    configure: (...args: unknown[]) => {
      const proxy = args[0] as Partial<ProxyEventEmitter>;
      proxy.on?.("error", () => {});
    },
  };
}

function runtimeEnv(): Record<string, string | undefined> {
  return (
    (
      globalThis as typeof globalThis & {
        [key: string]: RuntimeProcess | undefined;
      }
    )["process"]?.env ?? {}
  );
}

function apiTargetFromBind(bind: string | undefined): string | null {
  const trimmedBind = bind?.trim();
  if (!trimmedBind) {
    return null;
  }

  const port = trimmedBind.match(/:(\d+)$/)?.[1];
  if (!port) {
    return null;
  }

  const host = trimmedBind.replace(/:\d+$/, "");
  const browserHost =
    host === "" || host === "0.0.0.0" || host === "::" || host === "[::]"
      ? "127.0.0.1"
      : host.replace(/^\[(.*)\]$/, "$1");

  return `http://${browserHost}:${port}`;
}

function normalizeProxyTarget(target: string | undefined): string | null {
  const trimmedTarget = target?.trim().replace(/\/$/, "");
  return trimmedTarget ? trimmedTarget : null;
}

function resolveDevelopmentApiTarget(): string {
  const env = runtimeEnv();
  return (
    normalizeProxyTarget(env.VITE_DEV_API_TARGET) ??
    apiTargetFromBind(env.JFTRADE_API_BIND) ??
    developmentApiTarget
  );
}

const resolvedDevelopmentApiTarget = resolveDevelopmentApiTarget();

function desktopRuntimeConfigPlugin(): Plugin {
  return {
    name: "jftrade-desktop-runtime-config",
    configureServer(server) {
      server.middlewares.use(
        "/runtime-config.js",
        (_request, response, next) => {
          if (runtimeEnv().JFTRADE_DESKTOP_MODE !== "1") {
            next();
            return;
          }
          response.setHeader(
            "Content-Type",
            "application/javascript; charset=utf-8",
          );
          response.setHeader("Cache-Control", "no-store");
          response.end(
            `window.__JFTRADE_RUNTIME_CONFIG__ = Object.assign({}, window.__JFTRADE_RUNTIME_CONFIG__, ${JSON.stringify({ apiBaseUrl: resolvedDevelopmentApiTarget, authRequired: false, desktopMode: true })});\n`,
          );
        },
      );
    },
  };
}

export default defineConfig({
  resolve: {
    alias: {
      "@": new URL("./src", import.meta.url).pathname,
    },
    dedupe: [
      "@vue/reactivity",
      "@vue/runtime-core",
      "@vue/runtime-dom",
      "@vue/server-renderer",
      "@vue/shared",
      "vue",
    ],
  },
  plugins: [
    desktopRuntimeConfigPlugin(),
    vue(),
    tailwindcss(),
    vueDevTools(devToolsOptions),
  ],
  optimizeDeps: {
    include: [
      "@vue-flow/background",
      "@vue-flow/controls",
      "@vue-flow/core",
      "@vue-flow/minimap",
    ],
  },
  build: {
    chunkSizeWarningLimit: 4096,
  },
  test: {
    coverage: {
      provider: "v8",
      include: ["src/**/*.{ts,vue}"],
      thresholds: {
        statements: 85,
        lines: 85,
        "src/components/BacktestChart.vue": {
          statements: 95,
          lines: 95,
        },
        "src/components/MonacoCodeEditor.vue": {
          statements: 95,
          lines: 95,
        },
        "src/components/StrategyRuntimePanel.vue": {
          statements: 95,
          lines: 95,
        },
        "src/components/adk-page/ADKWorkflowStudio.vue": {
          statements: 95,
          lines: 95,
        },
        "src/composables/adkSettingsApi.ts": {
          statements: 95,
          lines: 95,
        },
        "src/composables/adkChatStream.ts": {
          statements: 95,
          lines: 95,
        },
        "src/composables/consoleDataBrokerSettings.ts": {
          statements: 95,
          lines: 95,
        },
        "src/composables/consoleDataMarketSubscriptions.ts": {
          statements: 95,
          lines: 95,
        },
        "src/composables/settingsManagedAccounts.ts": {
          statements: 95,
          lines: 95,
        },
        "src/composables/useADKPageChatState.ts": {
          statements: 95,
          lines: 95,
        },
        "src/composables/useBacktestRuns.ts": {
          statements: 95,
          lines: 95,
        },
        "src/pages/BacktestPage.vue": {
          statements: 95,
          lines: 95,
        },
      },
    },
    environmentOptions: {
      jsdom: {
        url: "http://localhost:3003/",
      },
    },
    fileParallelism: false,
    isolate: true,
    setupFiles: ["./tests/setup.ts"],
  },
  server: {
    port: 3003,
    // Wails loads FRONTEND_DEVSERVER_URL on this exact port. Silently moving
    // to 3004 when an old Vite process still owns 3003 makes the desktop load
    // the stale frontend and proxy API requests to the wrong backend.
    strictPort: true,
    proxy: {
      ...Object.fromEntries(
        apiProxyTargets.map((path) => [
          path,
          createProxyEntry(resolvedDevelopmentApiTarget),
        ]),
      ),
      "/docs": createProxyEntry(developmentDocsTarget),
    },
  },
});
