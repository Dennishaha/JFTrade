import { STRATEGY_BLOCK_CATALOG } from "./strategyVisualBuilderCatalogData";

export function createStrategyPaletteItems(): Array<{
  type: string;
  text: string;
  label: string;
  icon: string;
  properties: Record<string, unknown>;
}> {
  return STRATEGY_BLOCK_CATALOG
    .filter((block) => block.paletteVisible !== false)
    .map((block) => ({
      type: block.shape,
      text: block.text,
      label: block.label,
      icon: buildPaletteIcon(block.accent, block.label.slice(0, 2)),
      properties: {
        ...block.properties,
      },
    }));
}

export function buildPaletteIcon(fill: string, text: string): string {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="72" height="72" viewBox="0 0 72 72"><rect width="72" height="72" rx="18" fill="${fill}"/><text x="36" y="41" text-anchor="middle" font-size="22" font-family="Georgia, serif" fill="white">${escapeXml(text)}</text></svg>`;
  return `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`;
}

export function escapeXml(value: string): string {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&apos;");
}


