import { emptyBrokerRuntime } from "@/contracts"
import type { BrokerRuntimeResponse } from "@/contracts"

export function buildPineScript(
  name: string,
  body: string[] = ['log "close"'],
  options?: {
    version?: string;
    symbol?: string;
    interval?: string;
  },
) {
  const version = options?.version ?? "0.1.0"
  const symbol = options?.symbol ?? "00700"
  const interval = options?.interval ?? "1m"

  return [
    "//@version=6",
    `strategy("${name}", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10)`,
    `// test metadata version=${version} symbol=${symbol} interval=${interval}`,
    "",
    ...body.map((line) => normalizeTestPineLine(line)),
  ].join("\n")
}

function normalizeTestPineLine(line: string): string {
  const leadingWhitespace = line.match(/^\s*/)?.[0] ?? ""
  const trimmed = line.trim()
  if (trimmed === "") {
    return ""
  }
  const logMatch = trimmed.match(/^log\s+"(.*)"$/)
  if (logMatch !== null) {
    return `${leadingWhitespace}log.info("${logMatch[1]}")`
  }
  const notifyMatch = trimmed.match(/^notify\s+"(.*)"$/)
  if (notifyMatch !== null) {
    return `${leadingWhitespace}alert("${notifyMatch[1]}")`
  }
  const letMatch = trimmed.match(/^let\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.+)$/)
  if (letMatch !== null) {
    return `${leadingWhitespace}${letMatch[1]} = ${normalizeTestPineExpression(letMatch[2])}`
  }
  const ifMatch = trimmed.match(/^if\s+(.+):$/)
  if (ifMatch !== null) {
    return `${leadingWhitespace}if ${normalizeTestPineExpression(ifMatch[1])}`
  }
  const buyMatch = trimmed.match(/^buy\s+shares\s+([0-9.]+)/)
  if (buyMatch !== null) {
    return `${leadingWhitespace}strategy.entry("Long", strategy.long, qty=${buyMatch[1]})`
  }
  if (trimmed.startsWith("sell shares")) {
    return `${leadingWhitespace}strategy.close("Long")`
  }
  return `${leadingWhitespace}// ${trimmed}`
}

function normalizeTestPineExpression(expression: string): string {
  return expression
    .replace(/\brsi\(([^)]+)\)/g, "ta.rsi(close, $1)")
    .replace(/\bma\([^,]+,\s*([^,)]+)(?:,[^)]+)?\)/g, "ta.ema(close, $1)")
    .replace(/\bcross_over\(([^,]+),\s*([^)]+)\)/g, "ta.crossover($1, $2)")
    .replace(/\bcross_under\(([^,]+),\s*([^)]+)\)/g, "ta.crossunder($1, $2)")
}

export function buildRuntimeAccount(overrides?: Partial<BrokerRuntimeResponse>): BrokerRuntimeResponse {
  return {
    ...emptyBrokerRuntime,
    ...overrides,
    accounts: overrides?.accounts ?? emptyBrokerRuntime.accounts,
  }
}
