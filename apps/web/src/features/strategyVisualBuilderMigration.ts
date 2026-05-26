import type {
  StrategyDefinitionDocument,
  StrategyVisualModelDocument,
  StrategyVisualNodeDocument,
} from "@jftrade/ui-contracts";

import { nextGetTechnicalIndicatorNodeText } from "./strategyVisualBuilderIndicatorBlock";
import { cloneStrategyVisualModel } from "./strategyVisualBuilderShared";

const LEGACY_MOVING_AVERAGE_PATTERN = /(^|[^A-Za-z0-9_])ma:(MA|EMA|WMA|VWMA):(5|20)([^:A-Za-z0-9_]|$)/g;
const LEGACY_SIMPLE_MOVING_AVERAGE_PATTERN = /(^|[^A-Za-z0-9_])ma:(5|20)([^:A-Za-z0-9_]|$)/g;

export function migrateLegacyMovingAverageDefinition(
  definition: StrategyDefinitionDocument,
): StrategyDefinitionDocument {
  const visualModel = migrateLegacyMovingAverageVisualModel(definition.visualModel);
  return {
    ...definition,
    script: migrateLegacyMovingAverageScript(definition.script),
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

export function migrateLegacyMovingAverageScript(script: string): string {
  if (script.trim() === "") {
    return script;
  }

  return script
    .replace(
      LEGACY_MOVING_AVERAGE_PATTERN,
      (_match, prefix: string, averageType: string, windowSize: string, suffix: string) =>
        `${prefix}ma:${averageType}:${windowSize}:day${suffix}`,
    )
    .replace(
      LEGACY_SIMPLE_MOVING_AVERAGE_PATTERN,
      (_match, prefix: string, windowSize: string, suffix: string) =>
        `${prefix}ma:MA:${windowSize}:day${suffix}`,
    );
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