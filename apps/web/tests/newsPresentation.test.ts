import { describe, expect, it } from "vitest";

import {
  buildNewsItem,
  filterNewsItems,
  formatNewsTime,
  newsCategory,
  normalizeRelatedInstrument,
  stripMarkup,
} from "../src/composables/newsPresentation";

describe("news presentation", () => {
  it("normalizes broker-neutral news fields and safe related instruments", () => {
    const now = Date.parse("2026-07-17T12:00:00Z");
    expect(
      buildNewsItem(
        {
          title: "<em>Apple</em> &amp; AI",
          newsSubType: "NewsSubType_NOTICE",
          source: "Market Wire",
          publishTime: "2026-07-17T11:17:00Z",
          viewCount: 1234,
          relatedSecurities: ["AAPL.US", "HK.00700", "invalid"],
          url: "https://example.com/news",
          imageUrl: "javascript:alert(1)",
          summary: "<b>摘要</b>",
        },
        0,
        now,
      ),
    ).toMatchObject({
      title: "Apple & AI",
      summary: "摘要",
      category: "notice",
      categoryLabel: "公告",
      publishTime: "43分钟前",
      relatedInstruments: ["US.AAPL", "HK.00700"],
      imageUrl: "",
    });
    expect(normalizeRelatedInstrument("MSFT.US")).toBe("US.MSFT");
    expect(normalizeRelatedInstrument("US.MSFT")).toBe("US.MSFT");
    expect(normalizeRelatedInstrument("invalid")).toBe("");
    expect(stripMarkup("&lt;b&gt; text &#39;x&#39;")).toBe("<b> text 'x'");
  });

  it("formats categories, relative times and filters readable content", () => {
    const now = Date.parse("2026-07-17T12:00:00Z");
    expect(newsCategory(1)).toBe("news");
    expect(newsCategory("2")).toBe("notice");
    expect(newsCategory("评级")).toBe("rating");
    expect(newsCategory("unknown")).toBe("other");
    expect(formatNewsTime("2026-07-17T11:59:45Z", now)).toBe("刚刚");
    expect(formatNewsTime("2026-07-17T09:00:00Z", now)).toBe("3小时前");
    expect(formatNewsTime("2026-07-15T12:00:00Z", now)).toBe("2天前");
    expect(formatNewsTime("not-a-date", now)).toBe("not-a-date");

    const items = [
      buildNewsItem({ title: "Apple raises guidance", newsSubType: 1 }, 0, now),
      buildNewsItem(
        { title: "Tencent notice", newsSubType: 2, source: "HKEX" },
        1,
        now,
      ),
    ];
    expect(filterNewsItems(items, "news", "")).toHaveLength(1);
    expect(filterNewsItems(items, "all", "hkex")[0]?.title).toBe(
      "Tencent notice",
    );
  });

  it("handles broker enum variants, epoch times, old dates, and unsafe URLs", () => {
    const now = Date.parse("2026-07-17T12:00:00Z");
    expect(newsCategory(3)).toBe("rating");
    expect(newsCategory("NewsSubType_NEWS")).toBe("news");
    expect(
      buildNewsItem({ title: "评级调整", newsSubType: 3 }, 0, now),
    ).toMatchObject({ categoryLabel: "评级" });
    expect(
      buildNewsItem({ title: "其他资讯", newsSubType: "other" }, 1, now),
    ).toMatchObject({ categoryLabel: "资讯" });

    expect(formatNewsTime(String(Math.floor(now / 1000)), now)).toBe("刚刚");
    expect(formatNewsTime(String(now - 60_000), now)).toBe("1分钟前");
    // Absolute news times follow the browser's local timezone. The CI runners
    // use UTC while developer machines may use Asia/Shanghai.
    expect(formatNewsTime("2026-07-01T12:00:00Z", now)).toMatch(/07[/-]01/);
    expect(formatNewsTime("", now)).toBe("时间未知");
    expect(normalizeRelatedInstrument("XX.AAPL")).toBe("");

    const malformed = buildNewsItem(
      {
        newsType: "news",
        headline: "Malformed URL",
        link: "http://[invalid",
      },
      2,
      now,
    );
    expect(malformed.url).toBe("");
    expect(malformed.key).toBe("Malformed URL-2");
  });
});
