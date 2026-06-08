import tailwindcss from "@tailwindcss/vite";
import vue from "@vitejs/plugin-vue";
import vueDevTools from "vite-plugin-vue-devtools";
import { defineConfig } from "vitest/config";

type RuntimeProcess = {
  env?: Record<string, string | undefined>;
  platform?: string;
};

function resolveLaunchEditor(): string | null {
  const runtimeProcess = (globalThis as typeof globalThis & {
    [key: string]: RuntimeProcess | undefined;
  })["process"];
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
  devToolsOptions = { launchEditor } as NonNullable<Parameters<typeof vueDevTools>[0]>;
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
    target,
    ws: true,
    configure: (...args: unknown[]) => {
      const proxy = args[0] as Partial<ProxyEventEmitter>;
      proxy.on?.("error", () => {});
    },
  };
}

export default defineConfig({
  plugins: [vue(), tailwindcss(), vueDevTools(devToolsOptions)],
  build: {
    chunkSizeWarningLimit: 4096,
  },
  test: {
    environmentOptions: {
      jsdom: {
        url: "http://localhost:5173/",
      },
    },
    fileParallelism: false,
    isolate: true,
    setupFiles: ["./tests/setup.ts"],
  },
  server: {
    port: 5173,
    proxy: {
      ...Object.fromEntries(
        apiProxyTargets.map((path) => [path, createProxyEntry(developmentApiTarget)]),
      ),
      "/docs": createProxyEntry(developmentDocsTarget),
    },
  },
});
