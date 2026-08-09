import type { StrategyVisualNodeDocument } from "@/types";

import {
  normalizeCollectionStatBlockProperties,
  normalizeDerivedSeriesBlockProperties,
  normalizeMtfSeriesBlockProperties,
  normalizeSeriesConditionBlockProperties,
  normalizeStateUpdateBlockProperties,
  normalizeStateVariableBlockProperties,
  normalizeStrategyInputBlockProperties,
  normalizeSessionFilterBlockProperties,
  normalizeTimeFilterBlockProperties,
  type StrategySeriesSource,
} from "./strategyVisualBuilderCatalog";
import { renderVisualExpressionToPine } from "./strategyVisualBuilderExpressions";
import { formatNumber, formatPineValue, toPineStringLiteral } from "./strategyVisualBuilderPineFormat";
import {
  pineIndicatorSource,
  pineSeriesSource,
} from "./strategyVisualBuilderPineIndicatorExpressions";

export function buildStrategyInputExpression(
  properties: ReturnType<typeof normalizeStrategyInputBlockProperties>,
): string {
  const title = toPineStringLiteral(properties.title);
  switch (properties.inputType) {
    case "float":
      return `input.float(defval=${formatPineValue(properties.defaultValue)}, title=${title})`;
    case "source":
      return `input.source(${pineIndicatorSource(String(properties.defaultValue))}, ${title})`;
    case "timeframe":
      return `input.timeframe(${toPineStringLiteral(String(properties.defaultValue))}, ${title})`;
    case "time":
      return `input.time(${formatPineValue(properties.defaultValue)}, ${title})`;
    case "color":
      return `input.color(${formatPineValue(properties.defaultValue)}, ${title})`;
    case "int":
    default:
      return `input.int(${formatPineValue(properties.defaultValue)}, ${title})`;
  }
}

export function buildDerivedSeriesStatement(node: StrategyVisualNodeDocument): string {
  const properties = normalizeDerivedSeriesBlockProperties(node.properties ?? {});
  return `${properties.variableName} = ${buildDerivedSeriesExpression(properties)}`;
}

export function buildDerivedSeriesExpression(
  properties: ReturnType<typeof normalizeDerivedSeriesBlockProperties>,
): string {
  const source = pineSeriesSource(properties.source);
  switch (properties.mode) {
    case "nz":
      return `nz(${renderVisualExpressionToPine(properties.sourceExpressionAst, source)}, ${renderVisualExpressionToPine(properties.fallbackExpressionAst, formatNumber(properties.fallbackValue))})`;
    case "math":
      if (properties.mathFunction === "min" || properties.mathFunction === "max") {
        return `math.${properties.mathFunction}(${renderVisualExpressionToPine(properties.leftExpressionAst, properties.leftExpression)}, ${renderVisualExpressionToPine(properties.rightExpressionAst, properties.rightExpression)})`;
      }
      return `math.${properties.mathFunction}(${renderVisualExpressionToPine(properties.leftExpressionAst, properties.leftExpression)})`;
    case "arithmetic":
      return `(${renderVisualExpressionToPine(properties.leftExpressionAst, properties.leftExpression)} ${properties.operator} ${renderVisualExpressionToPine(properties.rightExpressionAst, properties.rightExpression)})`;
    case "cross":
      return `ta.${properties.crossFunction}(${renderVisualExpressionToPine(properties.leftExpressionAst, properties.leftExpression)}, ${renderVisualExpressionToPine(properties.rightExpressionAst, properties.rightExpression)})`;
    case "history":
    default:
      return `${renderVisualExpressionToPine(properties.sourceExpressionAst, source)}[${properties.historyOffset}]`;
  }
}

export function buildMtfSeriesStatement(node: StrategyVisualNodeDocument): string {
  const properties = normalizeMtfSeriesBlockProperties(node.properties ?? {});
  const expression = buildMtfInnerExpression(properties);
  return `${properties.variableName} = request.security(syminfo.tickerid, ${toPineStringLiteral(properties.timeframe)}, ${expression})`;
}

export function buildMtfInnerExpression(
  properties: ReturnType<typeof normalizeMtfSeriesBlockProperties>,
): string {
  const source = pineSeriesSource(properties.source);
  switch (properties.expressionType) {
    case "history":
      return `${source}[${properties.historyOffset}]`;
    case "indicator":
      return appendMtfField(
        properties.indicatorExpressionAst === undefined
          ? properties.indicatorExpression
          : renderVisualExpressionToPine(properties.indicatorExpressionAst, properties.indicatorExpression),
        properties.mtfField,
      );
    case "source":
    default:
      return source;
  }
}

export function buildStateVariableStatement(node: StrategyVisualNodeDocument): string {
  const properties = normalizeStateVariableBlockProperties(node.properties ?? {});
  return `var ${properties.variableName} = ${formatPineValue(properties.initialValue)}`;
}

export function buildStateUpdateStatement(node: StrategyVisualNodeDocument): string {
  const properties = normalizeStateUpdateBlockProperties(node.properties ?? {});
  const expression = properties.expressionAst === undefined
    ? properties.expression
    : renderVisualExpressionToPine(properties.expressionAst, properties.expression);
  return `${properties.variableName} := ${expression}`;
}

export function buildCollectionStatStatement(node: StrategyVisualNodeDocument): string {
  const properties = normalizeCollectionStatBlockProperties(node.properties ?? {});
  const values = [
    renderVisualExpressionToPine(properties.sourceAExpressionAst, pineSeriesSource(properties.sourceA)),
    renderVisualExpressionToPine(properties.sourceBExpressionAst, pineSeriesSource(properties.sourceB)),
    renderVisualExpressionToPine(properties.sourceCExpressionAst, pineSeriesSource(properties.sourceC)),
  ].join(", ");
  const receiver = `array.from(${values})`;
  if (properties.statFunction === "percentile") {
    return `${properties.variableName} = ${receiver}.percentile_linear_interpolation(${formatNumber(properties.percentile)})`;
  }
  return `${properties.variableName} = ${receiver}.${properties.statFunction}()`;
}

export function buildTimeFilterExpression(rawProperties: Record<string, unknown>): string {
  const properties = normalizeTimeFilterBlockProperties(rawProperties);
  const startMinute = properties.startHour * 60 + properties.startMinute;
  const endMinute = properties.endHour * 60 + properties.endMinute;
  const currentMinute = "(hour * 60 + minute)";
  switch (properties.mode) {
    case "after":
      return `${currentMinute} >= ${formatNumber(startMinute)}`;
    case "before":
      return `${currentMinute} < ${formatNumber(endMinute)}`;
    case "dayOfWeek":
      return `dayofweek == ${formatNumber(properties.dayOfWeek)}`;
    case "between":
    default:
      return `${currentMinute} >= ${formatNumber(startMinute)} and ${currentMinute} < ${formatNumber(endMinute)}`;
  }
}

export function buildSessionFilterExpression(rawProperties: Record<string, unknown>): string {
  const properties = normalizeSessionFilterBlockProperties(rawProperties);
  switch (properties.scope) {
    case "premarket":
      return "session.ispremarket";
    case "postmarket":
      return "session.ispostmarket";
    case "market":
    default:
      return "session.ismarket";
  }
}

export function appendMtfField(expression: string, field: string | undefined): string {
  if (field === undefined || field.trim() === "") {
    return expression;
  }
  return `${expression}.${field}`;
}

export function buildSeriesConditionExpression(rawProperties: Record<string, unknown>): string {
  const properties = normalizeSeriesConditionBlockProperties(rawProperties);
  const source = renderVisualExpressionToPine(properties.sourceExpressionAst, pineSeriesSource(properties.source));
  const operator = properties.operator;
  const threshold = renderVisualExpressionToPine(properties.rightExpressionAst, formatNumber(properties.threshold));
  const leftExpression = renderVisualExpressionToPine(properties.leftExpressionAst, source);
  const eventExpression = renderVisualExpressionToPine(
    properties.eventExpressionAst,
    `${pineSeriesSource(properties.eventSource)} ${properties.eventOperator} ${formatNumber(properties.eventThreshold)}`,
  );
  switch (properties.mode) {
    case "rising":
      return `ta.rising(${source}, ${properties.length})`;
    case "falling":
      return `ta.falling(${source}, ${properties.length})`;
    case "barssince":
      return `ta.barssince(${eventExpression}) < ${properties.length}`;
    case "valuewhen":
      return `ta.valuewhen(${eventExpression}, ${renderVisualExpressionToPine(properties.valueExpressionAst, pineSeriesSource(properties.valueSource))}, ${properties.occurrence}) ${operator} ${threshold}`;
    case "compare":
    default:
      return `${leftExpression} ${operator} ${threshold}`;
  }
}
