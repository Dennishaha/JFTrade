import type { StrategyVisualModelDocument } from "@jftrade/ui-contracts";
import { computed, type ComputedRef, type Ref } from "vue";

import {
  cloneStrategyVisualModel,
  createDefaultStrategyVisualModel,
  getStrategyBlockKind,
} from "../features/strategyVisualBuilder";
import {
  nextGetTechnicalIndicatorNodeText,
  normalizeGetTechnicalIndicatorProperties,
  type GetTechnicalIndicatorBlockProperties,
} from "../features/strategyVisualBuilderIndicatorBlock";
import { suggestStrategyIndicatorVariableName } from "../features/strategyVisualBuilderIndicatorReferences";

interface ApplyVisualModelOptions {
  preserveSelection?: boolean;
  notice?: string;
}

interface UseStrategyIndicatorVariablesOptions {
  visualModel: ComputedRef<StrategyVisualModelDocument>;
  selectedVisualNodeId: Ref<string>;
  applyVisualModel: (
    model: StrategyVisualModelDocument,
    options?: ApplyVisualModelOptions,
  ) => void;
}

export interface StrategyIndicatorVariableItem {
  id: string;
  text: string;
  properties: GetTechnicalIndicatorBlockProperties;
}

export function useStrategyIndicatorVariables(
  { visualModel, selectedVisualNodeId, applyVisualModel }: UseStrategyIndicatorVariablesOptions,
) {
  const indicatorVariables = computed<StrategyIndicatorVariableItem[]>(() =>
    visualModel.value.nodes
      .filter((node) => getStrategyBlockKind(node) === "getTechnicalIndicator")
      .map((node) => {
        const properties = normalizeGetTechnicalIndicatorProperties(node.properties ?? {});
        return {
          id: node.id,
          text: nextGetTechnicalIndicatorNodeText(
            properties as unknown as Record<string, unknown>,
          ),
          properties,
        };
      }),
  );

  function addIndicatorVariable(): void {
    const currentModel = createWorkingVisualModel(visualModel.value);
    const baseProperties = normalizeGetTechnicalIndicatorProperties({
      blockKind: "getTechnicalIndicator",
      indicatorType: "rsi",
      period: 14,
    });
    const nextProperties = normalizeGetTechnicalIndicatorProperties({
      ...baseProperties,
      variableName: suggestStrategyIndicatorVariableName(
        baseProperties as unknown as Record<string, unknown>,
      ),
    });

    applyVisualModel(
      {
        ...currentModel,
        nodes: [
          ...currentModel.nodes,
          {
            id: buildVisualNodeId("indicator-var"),
            type: "rect",
            x: 0,
            y: 0,
            text: nextGetTechnicalIndicatorNodeText(nextProperties as unknown as Record<string, unknown>),
            properties: nextProperties as unknown as Record<string, unknown>,
          },
        ],
      },
      {
        preserveSelection: true,
        notice: "已新增指标变量。",
      },
    );
  }

  function updateIndicatorVariable(
    payload: { id: string; properties: Record<string, unknown> },
  ): void {
    const currentModel = createWorkingVisualModel(visualModel.value);
    let changed = false;

    const nextNodes = currentModel.nodes.map((node) => {
      if (node.id !== payload.id) {
        return node;
      }

      changed = true;
      const nextProperties = normalizeGetTechnicalIndicatorProperties(payload.properties);
      return {
        ...node,
        text: nextGetTechnicalIndicatorNodeText(nextProperties as unknown as Record<string, unknown>),
        properties: nextProperties as unknown as Record<string, unknown>,
      };
    });

    if (!changed) {
      return;
    }

    applyVisualModel(
      {
        ...currentModel,
        nodes: nextNodes,
      },
      { preserveSelection: true },
    );
  }

  function deleteIndicatorVariable(nodeId: string): void {
    const currentModel = createWorkingVisualModel(visualModel.value);

    applyVisualModel(
      {
        ...currentModel,
        nodes: currentModel.nodes.filter((node) => node.id !== nodeId),
        edges: currentModel.edges.filter(
          (edge) => edge.sourceNodeId !== nodeId && edge.targetNodeId !== nodeId,
        ),
      },
      {
        preserveSelection: selectedVisualNodeId.value !== nodeId,
        notice: "已删除指标变量。",
      },
    );
  }

  return {
    indicatorVariables,
    addIndicatorVariable,
    updateIndicatorVariable,
    deleteIndicatorVariable,
  };
}

function createWorkingVisualModel(
  visualModel: StrategyVisualModelDocument,
): StrategyVisualModelDocument {
  return cloneStrategyVisualModel(visualModel) ?? createDefaultStrategyVisualModel();
}

function buildVisualNodeId(prefix: string): string {
  return `${prefix}-${Math.random().toString(36).slice(2, 10)}`;
}