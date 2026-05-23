import type { StrategyVisualNodeDocument } from "@jftrade/ui-contracts";
import { computed, type ComputedRef } from "vue";

import {
  getStrategyBlockDefinition,
  getStrategyBlockKind,
  type StrategyBlockKind,
} from "../features/strategyVisualBuilder";

interface UseStrategyVisualNodeInspectorOptions {
  selectedVisualNode: ComputedRef<StrategyVisualNodeDocument | null>;
  mutateSelectedVisualNode: (
    mutator: (node: StrategyVisualNodeDocument) => StrategyVisualNodeDocument,
  ) => void;
}

export function useStrategyVisualNodeInspector(
  { selectedVisualNode, mutateSelectedVisualNode }: UseStrategyVisualNodeInspectorOptions,
) {
  const selectedVisualKind = computed<StrategyBlockKind | null>(() =>
    getStrategyBlockKind(selectedVisualNode.value),
  );

  const selectedVisualBlock = computed(() =>
    getStrategyBlockDefinition(selectedVisualKind.value),
  );

  const selectedVisualNodeText = computed({
    get: () => selectedVisualNode.value?.text ?? "",
    set: (value: string) => {
      mutateSelectedVisualNode((node) => ({
        ...node,
        text: value,
      }));
    },
  });

  const selectedVisualNodeMessage = computed({
    get: () => {
      const message = selectedVisualNode.value?.properties.message;
      return typeof message === "string" ? message : "";
    },
    set: (value: string) => {
      mutateSelectedVisualNode((node) => ({
        ...node,
        properties: {
          ...node.properties,
          message: value,
        },
      }));
    },
  });

  const showsCodeInput = computed(() => selectedVisualKind.value === "codeBlock");

  const selectedVisualNodeCode = computed({
    get: () => {
      const code = selectedVisualNode.value?.properties.code;
      return typeof code === "string" ? code : "";
    },
    set: (value: string) => {
      mutateSelectedVisualNode((node) => ({
        ...node,
        text: nextCodeBlockNodeText(value, node.text),
        properties: {
          ...node.properties,
          code: value,
        },
      }));
    },
  });

  const showsThresholdInput = computed(() => {
    switch (selectedVisualKind.value) {
      case "ifCloseAbove":
      case "ifCloseBelow":
      case "ifRsiAbove":
      case "ifRsiBelow":
        return true;
      default:
        return false;
    }
  });

  const selectedVisualNodeThreshold = computed({
    get: () => {
      const threshold = selectedVisualNode.value?.properties.threshold;
      if (typeof threshold === "number") {
        return String(threshold);
      }
      if (typeof threshold === "string") {
        return threshold;
      }
      return "";
    },
    set: (value: string) => {
      const kind = selectedVisualKind.value;
      const nextThreshold = normalizeNumber(value, 0);

      mutateSelectedVisualNode((node) => ({
        ...node,
        text: nextThresholdNodeText(kind, nextThreshold, node.text),
        properties: {
          ...node.properties,
          threshold: nextThreshold,
        },
      }));
    },
  });

  const showsPeriodInput = computed(() => {
    switch (selectedVisualKind.value) {
      case "movingAverageFast":
      case "movingAverageSlow":
      case "rsi":
      case "bollinger":
        return true;
      default:
        return false;
    }
  });

  const selectedVisualNodePeriod = computed({
    get: () => {
      const node = selectedVisualNode.value;
      if (node === null) {
        return "";
      }

      switch (selectedVisualKind.value) {
        case "movingAverageFast":
        case "movingAverageSlow": {
          const value = node.properties.windowSize;
          return typeof value === "number" || typeof value === "string"
            ? String(value)
            : "";
        }
        case "rsi":
        case "bollinger": {
          const value = node.properties.period;
          return typeof value === "number" || typeof value === "string"
            ? String(value)
            : "";
        }
        default:
          return "";
      }
    },
    set: (value: string) => {
      const kind = selectedVisualKind.value;
      if (kind === null) {
        return;
      }

      const nextPeriod = normalizeInteger(value, 1);
      mutateSelectedVisualNode((node) => {
        const properties = { ...node.properties };

        if (kind === "movingAverageFast" || kind === "movingAverageSlow") {
          properties.windowSize = nextPeriod;
        } else {
          properties.period = nextPeriod;
        }

        return {
          ...node,
          text: nextPeriodNodeText(kind, nextPeriod, properties, node.text),
          properties,
        };
      });
    },
  });

  const showsMacdInputs = computed(() => selectedVisualKind.value === "macd");

  const selectedMacdFastPeriod = computed({
    get: () => readNumericProperty(selectedVisualNode.value, "fastPeriod"),
    set: (value: string) => {
      mutateSelectedVisualNode((node) => {
        const properties = { ...node.properties };
        properties.fastPeriod = normalizeInteger(value, 12);
        return {
          ...node,
          text: nextMacdNodeText(properties),
          properties,
        };
      });
    },
  });

  const selectedMacdSlowPeriod = computed({
    get: () => readNumericProperty(selectedVisualNode.value, "slowPeriod"),
    set: (value: string) => {
      mutateSelectedVisualNode((node) => {
        const properties = { ...node.properties };
        properties.slowPeriod = normalizeInteger(value, 26);
        return {
          ...node,
          text: nextMacdNodeText(properties),
          properties,
        };
      });
    },
  });

  const selectedMacdSignalPeriod = computed({
    get: () => readNumericProperty(selectedVisualNode.value, "signalPeriod"),
    set: (value: string) => {
      mutateSelectedVisualNode((node) => {
        const properties = { ...node.properties };
        properties.signalPeriod = normalizeInteger(value, 9);
        return {
          ...node,
          text: nextMacdNodeText(properties),
          properties,
        };
      });
    },
  });

  const showsMultiplierInput = computed(
    () => selectedVisualKind.value === "bollinger",
  );

  const selectedBollingerMultiplier = computed({
    get: () => readNumericProperty(selectedVisualNode.value, "multiplier"),
    set: (value: string) => {
      mutateSelectedVisualNode((node) => {
        const properties = { ...node.properties };
        properties.multiplier = normalizeNumber(value, 2);
        return {
          ...node,
          text: nextPeriodNodeText(
            "bollinger",
            normalizeInteger(String(properties.period ?? 20), 20),
            properties,
            node.text,
          ),
          properties,
        };
      });
    },
  });

  return {
    selectedVisualKind,
    selectedVisualBlock,
    selectedVisualNodeText,
    selectedVisualNodeMessage,
    showsCodeInput,
    selectedVisualNodeCode,
    showsThresholdInput,
    selectedVisualNodeThreshold,
    showsPeriodInput,
    selectedVisualNodePeriod,
    showsMacdInputs,
    selectedMacdFastPeriod,
    selectedMacdSlowPeriod,
    selectedMacdSignalPeriod,
    showsMultiplierInput,
    selectedBollingerMultiplier,
  };
}

function nextCodeBlockNodeText(value: string, fallbackText: string): string {
  const firstLine = value
    .split(/\r?\n/)
    .map((line) => line.trim())
    .find((line) => line !== "");

  if (firstLine === undefined) {
    return fallbackText;
  }

  const preview = firstLine.replace(/\s+/g, " ");
  return preview.length > 12 ? `代码 · ${preview.slice(0, 12)}…` : `代码 · ${preview}`;
}

function readNumericProperty(
  node: StrategyVisualNodeDocument | null,
  key: string,
): string {
  if (node === null) {
    return "";
  }
  const value = node.properties[key];
  if (typeof value === "number" || typeof value === "string") {
    return String(value);
  }
  return "";
}

function normalizeInteger(value: string, fallback: number): number {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) {
    return fallback;
  }
  return Math.max(1, Math.round(parsed));
}

function normalizeNumber(value: string, fallback: number): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function nextThresholdNodeText(
  kind: StrategyBlockKind | null,
  threshold: number,
  fallbackText: string,
): string {
  switch (kind) {
    case "ifCloseAbove":
      return `收盘价 > ${threshold}`;
    case "ifCloseBelow":
      return `收盘价 < ${threshold}`;
    case "ifRsiAbove":
      return `RSI > ${threshold}`;
    case "ifRsiBelow":
      return `RSI < ${threshold}`;
    default:
      return fallbackText;
  }
}

function nextPeriodNodeText(
  kind: StrategyBlockKind | null,
  period: number,
  properties: Record<string, unknown>,
  fallbackText: string,
): string {
  switch (kind) {
    case "movingAverageFast":
      return `快均线 ${period}`;
    case "movingAverageSlow":
      return `慢均线 ${period}`;
    case "rsi":
      return `RSI ${period}`;
    case "bollinger": {
      const multiplier = normalizeNumber(String(properties.multiplier ?? 2), 2);
      return `布林带 ${period}x${multiplier}`;
    }
    default:
      return fallbackText;
  }
}

function nextMacdNodeText(properties: Record<string, unknown>): string {
  const fastPeriod = normalizeInteger(String(properties.fastPeriod ?? 12), 12);
  const slowPeriod = normalizeInteger(String(properties.slowPeriod ?? 26), 26);
  const signalPeriod = normalizeInteger(
    String(properties.signalPeriod ?? 9),
    9,
  );
  return `MACD ${fastPeriod}/${slowPeriod}/${signalPeriod}`;
}