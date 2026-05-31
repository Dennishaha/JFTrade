import tailwindcss from "@tailwindcss/vite";
import vue from "@vitejs/plugin-vue";
import { defineConfig } from "vite";
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

export default defineConfig({
  plugins: [vue(), tailwindcss(), vueDevTools(devToolsOptions)],
  server: {
    port: 5173,
  },
});
