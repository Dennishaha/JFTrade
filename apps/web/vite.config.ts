import { createRequire } from "node:module";
import { dirname } from "node:path";
import tailwindcss from "@tailwindcss/vite";
import vue from "@vitejs/plugin-vue";
import vuetify from "vite-plugin-vuetify";
import vueDevTools from "vite-plugin-vue-devtools";
import { defineConfig } from "vitest/config";
import type { Plugin } from "vite";
import coveragePolicy from "./coverage-policy.json" with { type: "json" };

const require = createRequire(import.meta.url);
const typescript6Package = require.resolve("@typescript/typescript6/package.json");
const typescript6 = require(
  require.resolve("@typescript/old", { paths: [dirname(typescript6Package)] }),
);
const compilerSfc = require("vue/compiler-sfc") as {
  registerTS: (loadTypeScript: () => unknown) => void;
};

compilerSfc.registerTS(() => typescript6);

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
const defaultDevelopmentVitePort = 3003;
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

function resolveDevelopmentVitePort(): number {
  const value = Number(runtimeEnv().TAURI_VITE_PORT);
  return Number.isInteger(value) && value > 0 && value < 65536
    ? value
    : defaultDevelopmentVitePort;
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

// 核心框架依赖拆成稳定 vendor chunk，便于长期缓存。lazy 依赖
// （monaco / mermaid / @vue-flow / lightweight-charts）不匹配以下规则，
// 保持原有的按需 chunk 行为。
function vendorChunk(id: string): string | undefined {
  if (!id.includes("node_modules")) {
    return undefined;
  }
  if (/\/node_modules\/(vue|vue-router|@vue)\//.test(id)) {
    return "vendor-vue";
  }
  if (/\/node_modules\/vuetify\//.test(id)) {
    return "vendor-vuetify";
  }
  if (/\/node_modules\/@tanstack\//.test(id)) {
    return "vendor-query";
  }
  return undefined;
}

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
    alias: [
      // "monaco-editor" 裸导入替换为按需入口（src/monacoEditorEntry.ts），
      // 剔除 80+ 语言定义、css/html/json 语言服务及对应 worker chunk。
      // 类型解析不受影响（import type 仍指向 monaco-editor 官方声明）。
      {
        find: /^monaco-editor$/,
        replacement: new URL("./src/monacoEditorEntry.ts", import.meta.url)
          .pathname,
      },
      {
        find: "@",
        replacement: new URL("./src", import.meta.url).pathname,
      },
    ],
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
    // 按需引入 vuetify 组件/指令及其样式。测试环境关闭：vitest 用例大量
    // 依赖字符串 stubs（如 "v-btn"），autoImport 会把模板组件改写为直接
    // import，绕过 VTU 的 stubs 机制。
    vuetify({ autoImport: process.env.NODE_ENV !== "test" }),
    tailwindcss(),
    vueDevTools(devToolsOptions),
  ],
  optimizeDeps: {
    // Tauri starts as soon as the dev server binds. Keep the runtime and
    // Vuetify auto-import entries in the initial bundle to avoid cold-start
    // requests racing Vite's dependency optimizer.
    include: [
      "@tauri-apps/api/core",
      "@tauri-apps/api/event",
      "@tanstack/vue-query",
      "@vue-flow/background",
      "@vue-flow/controls",
      "@vue-flow/core",
      "@vue-flow/minimap",
      "vuetify/components/VAlert",
      "vuetify/components/VBtn",
      "vuetify/components/VBtnToggle",
      "vuetify/components/VCard",
      "vuetify/components/VChip",
      "vuetify/components/VDialog",
      "vuetify/components/VEmptyState",
      "vuetify/components/VIcon",
      "vuetify/components/VList",
      "vuetify/components/VMenu",
      "vuetify/components/VPagination",
      "vuetify/components/VProgressCircular",
      "vuetify/components/VProgressLinear",
      "vuetify/components/VSelect",
      "vuetify/components/VSwitch",
      "vuetify/components/VTable",
      "vuetify/components/VTextField",
      "vuetify/components/VTextarea",
      "vuetify/iconsets/fa",
    ],
  },
  build: {
    chunkSizeWarningLimit: 600,
    rollupOptions: {
      output: {
        manualChunks: vendorChunk,
      },
    },
  },
  test: {
    coverage: {
      provider: "v8",
      include: ["src/**/*.{ts,vue}"],
      thresholds: {
        // Global coverage protects all executable frontend source. Narrow
        // gates keep the high-risk trading paths strict without requiring
        // artificial line-coverage tests for unrelated presentation code.
        ...coveragePolicy.globalThresholds,
        ...Object.fromEntries(
          coveragePolicy.viteCriticalGlobs.map((glob) => [
            glob,
            coveragePolicy.criticalThresholds,
          ]),
        ),
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
    port: resolveDevelopmentVitePort(),
    // The Tauri dev shell uses TAURI_VITE_PORT. Strict mode keeps the desktop
    // from silently loading a stale frontend on another port when the
    // configured listener is already in use.
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
