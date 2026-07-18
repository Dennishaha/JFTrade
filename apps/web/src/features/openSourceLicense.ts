import type { SystemBuildInformation } from "../contracts";

export const JFTRADE_SOURCE_REPOSITORY_URL =
  "https://github.com/Dennishaha/jftrade";

const GIT_COMMIT_PATTERN = /^[0-9a-f]{7,40}$/i;

export function resolveCorrespondingSourceUrl(
  build: SystemBuildInformation | null | undefined,
): string {
  const commit = build?.commit?.trim() ?? "";
  if (!GIT_COMMIT_PATTERN.test(commit)) {
    return JFTRADE_SOURCE_REPOSITORY_URL;
  }
  return `${JFTRADE_SOURCE_REPOSITORY_URL}/tree/${commit}`;
}
