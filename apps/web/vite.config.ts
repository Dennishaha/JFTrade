import tailwindcss from "@tailwindcss/vite";
import vue from "@vitejs/plugin-vue";
import { defineConfig, type ProxyOptions } from "vite";
import vueDevTools from "vite-plugin-vue-devtools";

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
const apiProxyTargets = ["/api", "/openapi.json", "/swagger"];

type ProxyServerInstance = Parameters<
  NonNullable<ProxyOptions["configure"]>
>[0];
type ProxyEventEmitter = {
  on: (event: string, handler: (...args: unknown[]) => void) => void;
};

function createProxyEntry(target: string): ProxyOptions {
  return {
    changeOrigin: true,
    target,
    ws: true,
    configure: (proxy: ProxyServerInstance) => {
      const eventProxy = proxy as ProxyServerInstance & Partial<ProxyEventEmitter>;
      eventProxy.on?.("error", () => {});
    },
  };
}

export default defineConfig({
  plugins: [vue(), tailwindcss(), vueDevTools(devToolsOptions)],
  server: {
    port: 5173,
    proxy: Object.fromEntries(
      apiProxyTargets.map((path) => [path, createProxyEntry(developmentApiTarget)]),
    ),
  },
});
