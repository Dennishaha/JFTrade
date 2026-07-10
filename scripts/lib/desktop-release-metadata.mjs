import process from "node:process";
import { spawnSync } from "node:child_process";

export const desktopReleaseTagPattern = /^v(\d+)\.(\d+)\.(\d+)$/;

export function resolveDesktopBuildMetadata(
  env = process.env,
  now = new Date(),
) {
  const tag = String(
    env.JFTRADE_DESKTOP_RELEASE_TAG || env.GITHUB_REF_NAME || "",
  ).trim();
  const match = tag.match(desktopReleaseTagPattern);
  const version = match ? `${match[1]}.${match[2]}.${match[3]}` : "dev";
  const commit = String(
    env.JFTRADE_DESKTOP_COMMIT ||
      gitOutput(["rev-parse", "HEAD"]) ||
      env.GITHUB_SHA ||
      "unknown",
  ).trim();
  const buildTime = resolveBuildTime(env, now);
  return {
    tag: match ? tag : "",
    version,
    numericVersion: match ? version : "0.0.0",
    commit,
    buildTime,
  };
}

export function requireDesktopReleaseMetadata(metadata) {
  if (
    !metadata?.tag ||
    metadata.version === "dev" ||
    metadata.numericVersion === "0.0.0"
  ) {
    throw new Error(
      "Desktop releases require JFTRADE_DESKTOP_RELEASE_TAG=vX.Y.Z (v0.0.0 and dev are forbidden).",
    );
  }
  return metadata;
}

function resolveBuildTime(env, now) {
  const explicit = String(env.JFTRADE_DESKTOP_BUILD_TIME || "").trim();
  if (explicit) return new Date(explicit).toISOString();
  const epoch = String(env.SOURCE_DATE_EPOCH || "").trim();
  if (/^\d+$/.test(epoch)) return new Date(Number(epoch) * 1000).toISOString();
  return now.toISOString();
}

function gitOutput(args) {
  const result = spawnSync("git", args, { encoding: "utf8" });
  return result.status === 0 ? result.stdout : "";
}
