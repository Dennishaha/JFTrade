import { openExternalUrl } from "./externalLink";

const docsBaseUrl = "/docs/";

export function resolveDocsHref(path = ""): string {
  return `${docsBaseUrl}${path}`;
}

export function useDocsLink(): {
  docsBaseUrl: string;
  docsHomeUrl: string;
  resolveDocsHref: (path?: string) => string;
  openDocs: (path?: string) => void;
} {
  const docsHomeUrl = resolveDocsHref();

  function openDocs(path = ""): void {
    void openExternalUrl(resolveDocsHref(path));
  }

  return { docsBaseUrl, docsHomeUrl, resolveDocsHref, openDocs };
}
