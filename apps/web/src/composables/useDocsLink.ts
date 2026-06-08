const docsBaseUrl = "/docs/";

export function resolveDocsHref(path = "index.html"): string {
  if (docsBaseUrl.startsWith("http")) {
    return new URL(path, docsBaseUrl).toString();
  }

  return `${docsBaseUrl}${path}`;
}

export function useDocsLink(): {
  docsBaseUrl: string;
  docsHomeUrl: string;
  resolveDocsHref: (path?: string) => string;
  openDocs: (path?: string) => void;
} {
  const docsHomeUrl = resolveDocsHref("index.html");

  function openDocs(path = "index.html"): void {
    if (typeof window !== "undefined") {
      window.open(resolveDocsHref(path), "_blank", "noopener,noreferrer");
    }
  }

  return { docsBaseUrl, docsHomeUrl, resolveDocsHref, openDocs };
}
