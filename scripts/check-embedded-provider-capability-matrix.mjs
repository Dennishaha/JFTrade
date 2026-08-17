import { readFileSync } from "node:fs";
import { fileURLToPath, pathToFileURL } from "node:url";

const repositoryRoot = fileURLToPath(new URL("..", import.meta.url));
const matrixPath = `${repositoryRoot}/docs/market-data-providers.md`;

export const requiredCapabilityRows = Object.freeze([
  "| 新闻与公司行动 | 支持（经 broker 查询管线） | 支持，覆盖全部四个市场；公开 API `GET /api/v1/market-data/news/{market}/{symbol}` 与 `/api/v1/market-data/corporate-actions/{market}/{symbol}` | 支持，新闻与分红/送转仅覆盖沪深；同一公开 API，美港返回 409 capability 错误 |",
  "| 榜单（领涨/领跌/成交活跃） | 支持 | 仅美股（Yahoo 预定义榜单 day_gainers/day_losers/most_actives） | 沪深/港股（`SH`/`SZ`/`CN`/`HK`，本地排序 15 秒全市场目录快照） |",
  "| 板块热力与成分 | 支持 | 不支持 | 沪深（东财行业/概念板块及成员） |",
  "| 个股资料/财务/分析师/股权 | 支持 | US/HK | CN/SH/SZ/HK；港股仅公司资料；分析师为东财个股研报 180 天聚合，无目标价 |",
  "| 估值/卖空 | 支持 | 不支持 | 不支持 |",
  "| 事件日历（财报/派息/经济/IPO） | 支持 | 不支持 | 沪深全市场 |",
  "| 宏观指标 | 支持 | 不支持 | 16 个中美策划指标目录与历史序列（无联邦基金利率） |",
  "| 股票筛选 | 支持（402 因子目录） | 仅美股，9 因子子集（3 个 basic 标识 + 6 个 simple 数值），单页 `limit` ≤250 且 `offset` 独立直传 | CN/SH/SZ/HK/US，同一 9 因子子集（US/HK 现货帧经东财 clist 直连补齐市净率/PE TTM 与总市值）；`basic.name` 只读不排 |",
]);

export const requiredCapabilityStatements = Object.freeze([
  "## 研究中心只读能力",
  "经济日历窗口上限 31 天",
  "多个排序键返回 409 capability 错误",
]);

export function validateCapabilityMatrix(source) {
  const lines = new Set(source.split(/\r?\n/u).map((line) => line.trim()));
  const missingRows = requiredCapabilityRows.filter((row) => !lines.has(row));
  const missingStatements = requiredCapabilityStatements.filter(
    (statement) => !source.includes(statement),
  );
  if (missingRows.length > 0 || missingStatements.length > 0) {
    throw new Error(
      "market-data provider capability matrix is missing or changed: " +
        [...missingRows, ...missingStatements].join(", "),
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
