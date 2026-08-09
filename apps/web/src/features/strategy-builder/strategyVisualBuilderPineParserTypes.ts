import type {
  StrategyVisualEdgeDocument,
  StrategyVisualNodeDocument,
} from "@/types";

import type { StrategyFlowNodeJsDoc } from "./strategyVisualBuilderShared";

export interface ParsedPineEntry {
  lineNumber: number;
  raw: string;
  trimmed: string;
  indent: number;
  start: number;
  end: number;
  annotation: StrategyFlowNodeJsDoc | null;
  annotationStart: number | null;
}

export interface ParseState {
  entries: ParsedPineEntry[];
  index: number;
  nodes: StrategyVisualNodeDocument[];
  edges: StrategyVisualEdgeDocument[];
  nodeIds: Set<string>;
  existingNodeById: Map<string, StrategyVisualNodeDocument>;
  aliasByName: Map<string, IndicatorAliasBinding>;
  sequence: number;
  error: string | null;
}

export interface IndicatorAliasBinding {
  alias: string;
  nodeId: string;
  indicatorType: string;
}

export interface ParsedNodeResult {
  node: StrategyVisualNodeDocument;
  isCondition: boolean;
}

