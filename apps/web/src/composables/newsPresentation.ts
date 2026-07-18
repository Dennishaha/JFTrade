export type NewsCategory = "all" | "news" | "notice" | "rating";

export interface NewsItemModel {
  key: string;
  title: string;
  summary: string;
  source: string;
  category: Exclude<NewsCategory, "all"> | "other";
  categoryLabel: string;
  publishTime: string;
  viewCount: string;
  url: string;
  imageUrl: string;
  relatedInstruments: string[];
}

type NewsEntry = Record<string, unknown>;

function firstText(entry: NewsEntry, keys: string[]): string {
  for (const key of keys) {
    const value = entry[key];
    if (typeof value === "string" && value.trim()) {
      return stripMarkup(value);
    }
  }
  return "";
}

export function stripMarkup(value: string): string {
  return value
    .replace(/<[^>]*>/g, "")
    .replaceAll("&amp;", "&")
    .replaceAll("&lt;", "<")
    .replaceAll("&gt;", ">")
    .replaceAll("&quot;", '"')
    .replaceAll("&#39;", "'")
    .replace(/\s+/g, " ")
    .trim();
}

export function newsCategory(
  value: unknown,
): NewsItemModel["category"] {
  if (value === 1) return "news";
  if (value === 2) return "notice";
  if (value === 3) return "rating";
  const normalized = String(value ?? "").trim().toLowerCase();
  if (
    normalized === "2" ||
    normalized.includes("notice") ||
    normalized === "公告"
  ) {
    return "notice";
  }
  if (
    normalized === "3" ||
    normalized.includes("rating") ||
    normalized === "评级"
  ) {
    return "rating";
  }
  if (
    normalized === "1" ||
    normalized.endsWith("_news") ||
    normalized === "news" ||
    normalized === "新闻"
  ) {
    return "news";
  }
  return "other";
}

function categoryLabel(value: NewsItemModel["category"]): string {
  switch (value) {
    case "news":
      return "新闻";
    case "notice":
      return "公告";
    case "rating":
      return "评级";
    default:
      return "资讯";
  }
}

function parseNewsTime(value: string): number | null {
  if (/^\d{10}$/.test(value)) return Number(value) * 1000;
  if (/^\d{13}$/.test(value)) return Number(value);
  const parsed = Date.parse(value);
  return Number.isNaN(parsed) ? null : parsed;
}

export function formatNewsTime(value: string, now = Date.now()): string {
  const timestamp = parseNewsTime(value);
  if (timestamp == null) return value || "时间未知";
  const elapsed = Math.max(0, now - timestamp);
  if (elapsed < 60_000) return "刚刚";
  if (elapsed < 3_600_000) return `${Math.floor(elapsed / 60_000)}分钟前`;
  if (elapsed < 86_400_000) return `${Math.floor(elapsed / 3_600_000)}小时前`;
  if (elapsed < 604_800_000) return `${Math.floor(elapsed / 86_400_000)}天前`;
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(timestamp);
}

export function normalizeRelatedInstrument(value: string): string {
  const normalized = value.trim().toUpperCase();
  const parts = normalized.split(".");
  if (parts.length !== 2) return "";
  const markets = new Set(["HK", "US", "SH", "SZ"]);
  if (markets.has(parts[0]!)) return normalized;
  if (markets.has(parts[1]!)) return `${parts[1]}.${parts[0]}`;
  return "";
}

function safeURL(value: string): string {
  if (!value) return "";
  try {
    const url = new URL(value);
    return url.protocol === "http:" || url.protocol === "https:"
      ? url.toString()
      : "";
  } catch {
    return "";
  }
}

function relatedInstruments(entry: NewsEntry): string[] {
  const value =
    entry.relatedSecurities ??
    entry.relatedInstruments ??
    entry.securities;
  if (!Array.isArray(value)) return [];
  return [
    ...new Set(
      value
        .map((item) => normalizeRelatedInstrument(String(item)))
        .filter(Boolean),
    ),
  ];
}

export function buildNewsItem(
  entry: NewsEntry,
  index: number,
  now = Date.now(),
): NewsItemModel {
  const category = newsCategory(
    entry.newsSubType ?? entry.newsType ?? entry.category ?? entry.type,
  );
  const title = firstText(entry, ["title", "headline", "name"]);
  const rawPublishTime = firstText(entry, [
    "publishTime",
    "publishedAt",
    "createdAt",
    "time",
  ]);
  const views = Number(entry.viewCount ?? entry.views ?? 0);
  const url = safeURL(firstText(entry, ["url", "newsUrl", "link"]));
  return {
    key: `${url || title || "news"}-${index}`,
    title: title || `资讯 ${index + 1}`,
    summary: firstText(entry, [
      "summary",
      "description",
      "brief",
      "abstract",
      "content",
    ]),
    source: firstText(entry, ["source", "publisher", "author"]) || "资讯",
    category,
    categoryLabel: categoryLabel(category),
    publishTime: formatNewsTime(rawPublishTime, now),
    viewCount:
      Number.isFinite(views) && views > 0
        ? `${new Intl.NumberFormat("zh-CN", { notation: "compact" }).format(views)} 阅读`
        : "",
    url,
    imageUrl: safeURL(
      firstText(entry, ["imageUrl", "thumbnail", "coverUrl", "image"]),
    ),
    relatedInstruments: relatedInstruments(entry),
  };
}

export function filterNewsItems(
  items: NewsItemModel[],
  category: NewsCategory,
  keyword: string,
): NewsItemModel[] {
  const normalizedKeyword = keyword.trim().toLocaleLowerCase();
  return items.filter((item) => {
    if (category !== "all" && item.category !== category) return false;
    if (!normalizedKeyword) return true;
    return [
      item.title,
      item.summary,
      item.source,
      item.relatedInstruments.join(" "),
    ]
      .join(" ")
      .toLocaleLowerCase()
      .includes(normalizedKeyword);
  });
}
