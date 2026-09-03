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
      path.join(repositoryRoot, "proto/pineworker/pineworker.proto"),
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
  const bundle = {
    // Updater artifacts are emitted only by the publish lane.  Keeping
    // dry-runs free of signing requirements preserves local verification
    // while the release workflow fails closed before any publish.
    createUpdaterArtifacts: sourceEnvironment.JFTRADE_DESKTOP_PUBLISH === "true",
  };
  const macSigningIdentity = String(sourceEnvironment.JFTRADE_MACOS_SIGN_IDENTITY ?? "").trim();
  if (macSigningIdentity) bundle.macOS = { signingIdentity: macSigningIdentity };
  const windowsCertificateThumbprint = String(
    sourceEnvironment.JFTRADE_WINDOWS_CERTIFICATE_THUMBPRINT ?? "",
  ).trim();
  if (windowsCertificateThumbprint) {
    bundle.windows = {
      certificateThumbprint: windowsCertificateThumbprint,
      digestAlgorithm: "sha256",
      timestampUrl: "http://timestamp.digicert.com",
    };
  }
  return {
    environment: {
      ...sourceEnvironment,
      JFTRADE_BUILD_VERSION: metadata.version,
      JFTRADE_BUILD_COMMIT: metadata.commit,
      JFTRADE_BUILD_TIME: metadata.buildTime,
    },
    finalOptions: [
      "--config",
      JSON.stringify({
        version: metadata.version,
        bundle,
      }),
    ],
    metadata,
  };
}

export function tauriCommandOptions(command, userOptions, releaseBuild = null) {
  const sanitized = userOptions.filter((arg) => arg !== "--");
  if (command === "dev") {
    return ["--config", "tauri.dev.conf.json", ...sanitized];
  }
  if (command === "build" && releaseBuild !== null) {
    return [...sanitized, ...releaseBuild.finalOptions];
  }
  return [...sanitized];
}
