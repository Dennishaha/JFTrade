import path from "node:path";

import {
  requireDesktopReleaseMetadata,
  resolveDesktopBuildMetadata,
} from "./desktop-release-metadata.mjs";

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

export function tauriReleaseBuild(sourceEnvironment, now = new Date()) {
  const metadata = requireDesktopReleaseMetadata(
    resolveDesktopBuildMetadata(sourceEnvironment, now),
  );
  return {
    environment: {
      ...sourceEnvironment,
      JFTRADE_BUILD_VERSION: metadata.version,
      JFTRADE_BUILD_COMMIT: metadata.commit,
      JFTRADE_BUILD_TIME: metadata.buildTime,
    },
    finalOptions: ["--config", JSON.stringify({ version: metadata.version })],
    metadata,
  };
}

export function tauriCommandOptions(command, userOptions, releaseBuild = null) {
  if (command === "dev") {
    return ["--config", "tauri.dev.conf.json", ...userOptions];
  }
  if (command === "build" && releaseBuild !== null) {
    return [...userOptions, ...releaseBuild.finalOptions];
  }
  return [...userOptions];
}
