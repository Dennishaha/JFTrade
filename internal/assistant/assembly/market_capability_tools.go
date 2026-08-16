package assembly

import (
	"context"
	"fmt"

	jfadkruntime "github.com/jftrade/jftrade-main/internal/assistant/engine/workflowruntime"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

// registerJFTradeADKMarketCapabilityTools registers the optional market-data
// capabilities that only some providers offer (news, corporate actions, index
// constituents). Providers without the capability surface a clear unsupported
// error instead of a generic failure.
func registerJFTradeADKMarketCapabilityTools(registry *jfadkruntime.ToolRegistry, deps ToolDeps) {
	registry.Register(assistantmodel.ToolDescriptor{Name: "market.news", DisplayName: "标的资讯", Description: "读取指定标的的最近资讯条目；仅 yfinance/AKShare 行情提供者支持，Futu 会返回明确的不支持说明。", Category: "market", Permission: "read_internal", RiskLevel: "low", OutputSummary: "标题、链接、发布方、发布时间和摘要列表。", RequiredSkills: []string{"jftrade-market"}}, func(ctx context.Context, input map[string]any) (any, error) {
		market, symbol := inferMarketSymbol(input)
		if market == "" || symbol == "" {
			return nil, fmt.Errorf("market and symbol are required")
		}
		if deps.MarketNews == nil {
			return nil, fmt.Errorf("market news is unavailable")
		}
		limit := intValue(input, "limit", 10)
		if limit < 1 || limit > 50 {
			return nil, fmt.Errorf("limit must be between 1 and 50")
		}
		return deps.MarketNews(ctx, market, symbol, limit)
	})
	registry.Register(assistantmodel.ToolDescriptor{Name: "market.corporate_actions", DisplayName: "公司行动", Description: "读取指定标的的分红和拆股事件；仅 yfinance/AKShare 行情提供者支持，Futu 会返回明确的不支持说明。", Category: "market", Permission: "read_internal", RiskLevel: "low", OutputSummary: "按除权除息日排序的分红/拆股事件列表。", RequiredSkills: []string{"jftrade-market"}}, func(ctx context.Context, input map[string]any) (any, error) {
		market, symbol := inferMarketSymbol(input)
		if market == "" || symbol == "" {
			return nil, fmt.Errorf("market and symbol are required")
		}
		if deps.MarketCorporateActions == nil {
			return nil, fmt.Errorf("market corporate actions are unavailable")
		}
		from, err := optionalToolTime(input, "from")
		if err != nil {
			return nil, err
		}
		to, err := optionalToolTime(input, "to")
		if err != nil {
			return nil, err
		}
		if !from.IsZero() && !to.IsZero() && from.After(to) {
			return nil, fmt.Errorf("from must not be after to")
		}
		return deps.MarketCorporateActions(ctx, market, symbol, from, to)
	})
	registry.Register(assistantmodel.ToolDescriptor{Name: "market.index_constituents", DisplayName: "指数成分股", Description: "读取中证及沪深交易所指数的成分股列表；仅 AKShare 行情提供者支持，其他提供者或市场会返回明确的不支持说明。", Category: "market", Permission: "read_internal", RiskLevel: "low", OutputSummary: "成分股代码、名称和可选权重列表。", RequiredSkills: []string{"jftrade-market"}}, func(ctx context.Context, input map[string]any) (any, error) {
		market, symbol := inferMarketSymbol(input)
		if market == "" || symbol == "" {
			return nil, fmt.Errorf("market and symbol are required")
		}
		if deps.MarketIndexConstituents == nil {
			return nil, fmt.Errorf("market index constituents are unavailable")
		}
		limit := intValue(input, "limit", 200)
		if limit < 1 || limit > 1000 {
			return nil, fmt.Errorf("limit must be between 1 and 1000")
		}
		return deps.MarketIndexConstituents(ctx, market, symbol, limit)
	})
}
