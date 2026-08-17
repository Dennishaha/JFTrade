import { readFileSync } from "node:fs";
import { fileURLToPath, pathToFileURL } from "node:url";

const repositoryRoot = fileURLToPath(new URL("..", import.meta.url));
const matrixPath = `${repositoryRoot}/docs/market-data-providers.md`;

export const requiredCapabilityStatements = Object.freeze([
  "| 榜单（领涨/领跌/成交活跃）",
  "| 板块热力与成分",
  "| 个股资料/财务/分析师/股权",
  "| 事件日历（财报/派息/经济/IPO）",
  "| 宏观指标",
  "| 股票筛选",
  "研究中心只读能力",
  "经济日历窗口上限 31 天",
  "多个排序键返回 409 capability 错误",
]);

export function validateCapabilityMatrix(source) {
  const missing = requiredCapabilityStatements.filter(
    (statement) => !source.includes(statement),
  );
  if (missing.length > 0) {
    throw new Error(
      `market-data provider capability matrix is missing: ${missing.join(", ")}`,
    );
  }
}

export function checkCapabilityMatrix(path = matrixPath) {
  validateCapabilityMatrix(readFileSync(path, "utf8"));
}

const invokedPath = process.argv[1]
  ? pathToFileURL(process.argv[1]).href
  : "";
if (invokedPath === import.meta.url) {
  checkCapabilityMatrix();
  console.log("embedded provider capability matrix: ok");
}
