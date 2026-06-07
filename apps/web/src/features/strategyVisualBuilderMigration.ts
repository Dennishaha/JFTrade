import type {
  StrategyDefinitionDocument,
  StrategyVisualModelDocument,
  StrategyVisualNodeDocument,
} from "@jftrade/ui-contracts";

import { nextGetTechnicalIndicatorNodeText } from "./strategyVisualBuilderIndicatorBlock";
import { cloneStrategyVisualModel } from "./strategyVisualBuilderShared";

export function migrateLegacyMovingAverageDefinition(
  definition: StrategyDefinitionDocument,
): StrategyDefinitionDocument {
  const legacyIndicators = collectLegacyMovingAverageIndicators(
    definition.visualModel,
  );
  const visualModel = migrateLegacyMovingAverageVisualModel(definition.visualModel);
  return {
    ...definition,
    script: migrateLegacyMovingAverageScript(
      definition.script,
      legacyIndicators,
    ),
    visualModel,
  };
}

export function migrateLegacyMovingAverageVisualModel(
  model: StrategyVisualModelDocument | null | undefined,
): StrategyVisualModelDocument | null {
  const cloned = cloneStrategyVisualModel(model);
  if (cloned === null) {
    return null;
  }

  cloned.nodes = cloned.nodes.map((node) => migrateLegacyMovingAverageNode(node));
  return cloned;
}
function migrateLegacyMovingAverageNode(
  node: StrategyVisualNodeDocument,
): StrategyVisualNodeDocument {
  const properties = node.properties ?? {};
  if (
    properties.blockKind !== "getTechnicalIndicator" ||
    properties.indicatorType !== "movingAverage"
  ) {
    return node;
  }

  if (typeof properties.periodUnit === "string" && properties.periodUnit.trim() !== "") {
    return node;
  }

  const windowSize = normalizeWindowSize(properties.windowSize);
  if (windowSize !== 5 && windowSize !== 20) {
    return node;
  }

  const legacyProperties = { ...properties };
  const nextProperties = {
    ...properties,
    periodUnit: "day",
  };
  const nextText = nextGetTechnicalIndicatorNodeText(nextProperties);
  const legacyText = nextGetTechnicalIndicatorNodeText(legacyProperties);

  return {
    ...node,
    text: node.text.trim() === "" || node.text === legacyText ? nextText : node.text,
    properties: nextProperties,
  };
}

function normalizeWindowSize(value: unknown): number {
  if (typeof value === "number" && Number.isFinite(value)) {
    return Math.trunc(value);
  }
  if (typeof value === "string") {
    const parsed = Number.parseInt(value.trim(), 10);
    return Number.isFinite(parsed) ? parsed : 0;
  }
  return 0;
}

function collectLegacyMovingAverageIndicators(
  model: StrategyVisualModelDocument | null | undefined,
): Set<string> {
  const indicators = new Set<string>();
  for (const node of model?.nodes ?? []) {
    const properties = node.properties ?? {};
    if (
      properties.blockKind !== "getTechnicalIndicator" ||
      properties.indicatorType !== "movingAverage" ||
      (typeof properties.periodUnit === "string" &&
        properties.periodUnit.trim() !== "")
    ) {
      continue;
    }
    const windowSize = normalizeWindowSize(properties.windowSize);
    if (windowSize !== 5 && windowSize !== 20) {
      continue;
    }
    const movingAverageType =
      typeof properties.movingAverageType === "string" &&
      properties.movingAverageType.trim() !== ""
        ? properties.movingAverageType.trim().toUpperCase()
        : "MA";
    indicators.add(`${movingAverageType}:${windowSize}`);
  }
  return indicators;
}

function migrateLegacyMovingAverageScript(
  script: string,
  legacyIndicators: Set<string>,
): string {
  return script.replace(
    /(ctx\.indicators\[\s*)(["'])ma:(?:(MA|EMA|SMA|SMMA|LWMA|TMA):)?(5|20)\2(\s*\])/g,
    (match, prefix, quote, rawType, rawWindowSize, suffix) => {
      const movingAverageType = rawType ?? "MA";
      const indicator = `${movingAverageType}:${rawWindowSize}`;
      if (!legacyIndicators.has(indicator)) {
        return match;
      }
      return `${prefix}${quote}ma:${indicator}:day${quote}${suffix}`;
    },
  );
}
