import path from "node:path";

export function tauriDevelopmentEnvironment(repositoryRoot, sourceEnvironment, nodeRuntime) {
  return {
    ...sourceEnvironment,
    JFTRADE_PINEWORKER_BUNDLE:
      sourceEnvironment.JFTRADE_PINEWORKER_BUNDLE ?? path.join(repositoryRoot, "var/pineworker/worker.mjs"),
    JFTRADE_PINEWORKER_RUNTIME: sourceEnvironment.JFTRADE_PINEWORKER_RUNTIME ?? nodeRuntime,
    JFTRADE_PINEWORKER_PROTO:
      sourceEnvironment.JFTRADE_PINEWORKER_PROTO ??
      path.join(repositoryRoot, "pkg/strategy/pineworker/proto/pineworker.proto"),
  };
}

export function tauriPreparation(command) {
  if (command === "dev") return ["pnpm", ["run", "build:pineworker:dev"]];
  if (command === "build") return ["pnpm", ["run", "prepare:tauri-release"]];
  return null;
}
