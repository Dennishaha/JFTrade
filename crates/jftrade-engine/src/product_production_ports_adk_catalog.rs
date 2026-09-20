#[derive(Clone, Copy)]
pub(crate) struct ProductionToolDefinition {
    pub(crate) id: &'static str,
    pub(crate) category: &'static str,
    pub(crate) display_name: &'static str,
    pub(crate) adapter: ProductionRouteAdapter,
    /// Optional operation-level readiness key.  This is used for research
    /// tools whose public HTTP routes share the `ResearchRead` umbrella.
    pub(crate) research_operation: Option<&'static str>,
}

/// Permission class and risk level for one production tool.
///
/// The reference runtime keeps these on every `ToolDescriptor` and derives
/// `RequireConfirmation` from them (`ToolRequiresApproval`).  The Rust catalog
/// persists the same three fields on the wire projection, so the runtime can
/// decide whether a call runs immediately or waits for the operator instead of
/// treating every tool as approval-gated.
#[derive(Clone, Copy)]
pub(crate) struct ToolAccessPolicy {
    pub(crate) permission: &'static str,
    pub(crate) risk_level: &'static str,
    /// Modes whose call must be confirmed; `None` means "permission class
    /// default" rather than an explicit list.
    pub(crate) requires_approval_in: Option<&'static [&'static str]>,
}

const APPROVAL_MODES: &[&str] = &["approval", "less_approval", "all"];

/// `strategy.optimize` declares an explicit `RequiresApprovalIn` list holding
/// only `approval`, so the low-risk catalog entry is gated in `approval` mode
/// and released in `less_approval`/`all` — unlike `market.provider.select`,
/// which the reference confirms in every mode.
const OPTIMIZE_APPROVAL_MODES: &[&str] = &["approval"];

const READ_ONLY_POLICY: ToolAccessPolicy = ToolAccessPolicy {
    permission: "read_internal",
    risk_level: "low",
    requires_approval_in: None,
};

/// Look up the access policy for a production tool id.
///
/// Reads default to `read_internal`/low, which is the reference class for every
/// catalog entry that does not explicitly opt into a write class.  The
/// remaining entries mirror the reference descriptor metadata one for one
/// (`strategy.research_backtest` is `optimize_strategy` but low risk, and
/// `interaction.request_user` is a low-risk interaction, so both stay
/// automatically executable).
pub(crate) fn tool_access_policy(id: &str) -> ToolAccessPolicy {
    match id {
        // Global settings change: confirmed in every mode, like Go's
        // `market.provider.select`.
        "market.provider.select" => ToolAccessPolicy {
            permission: "write_settings",
            risk_level: "high",
            requires_approval_in: Some(APPROVAL_MODES),
        },
        // `http.fetch` is `read_external`/medium: the reference keeps no
        // explicit per-mode list, so approval mode gates it through the
        // medium-risk rule while `all` executes it.
        "http.fetch" => ToolAccessPolicy {
            permission: "read_external",
            risk_level: "medium",
            requires_approval_in: None,
        },
        "strategy.research_backtest" => ToolAccessPolicy {
            permission: "optimize_strategy",
            risk_level: "low",
            requires_approval_in: None,
        },
        "strategy.optimize" => ToolAccessPolicy {
            permission: "optimize_strategy",
            risk_level: "low",
            requires_approval_in: Some(OPTIMIZE_APPROVAL_MODES),
        },
        _ => READ_ONLY_POLICY,
    }
}

pub(crate) const PRODUCTION_TOOL_DEFINITIONS: &[ProductionToolDefinition] = &[
    ProductionToolDefinition {
        id: "interaction.request_user",
        category: "interaction",
        display_name: "向用户提问",
        adapter: ProductionRouteAdapter::AdkChat,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "workflow.wait",
        category: "workflow",
        display_name: "等待工作流",
        adapter: ProductionRouteAdapter::AdkChat,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "http.fetch",
        category: "external",
        display_name: "抓取网页",
        adapter: ProductionRouteAdapter::AdkChat,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "tools.search",
        category: "system",
        display_name: "搜索工具",
        adapter: ProductionRouteAdapter::AdkChat,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "models.list",
        category: "system",
        display_name: "查询可调用模型",
        adapter: ProductionRouteAdapter::AdkRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "system.status",
        category: "system",
        display_name: "查询系统状态",
        adapter: ProductionRouteAdapter::SystemCore,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "system.futu_opend",
        category: "system",
        display_name: "查询 OpenD 状态",
        adapter: ProductionRouteAdapter::SystemRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "plugins.catalog",
        category: "plugins",
        display_name: "查询插件目录",
        adapter: ProductionRouteAdapter::PluginsRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "market.capabilities",
        category: "market",
        display_name: "查询行情能力",
        adapter: ProductionRouteAdapter::MarketDataProviderRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "market.search",
        category: "market",
        display_name: "搜索标的",
        adapter: ProductionRouteAdapter::MarketDataSearchRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "market.snapshot",
        category: "market",
        display_name: "查询行情快照",
        adapter: ProductionRouteAdapter::MarketDataSnapshotsRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "market.snapshots",
        category: "market",
        display_name: "批量查询行情快照",
        adapter: ProductionRouteAdapter::MarketDataBatchSnapshotsWrite,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "market.candles",
        category: "market",
        display_name: "查询 K 线",
        adapter: ProductionRouteAdapter::MarketDataCandlesRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "market.intraday",
        category: "market",
        display_name: "查询分时行情",
        adapter: ProductionRouteAdapter::MarketDataIntradayRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "market.subscriptions",
        category: "market",
        display_name: "查询行情订阅",
        adapter: ProductionRouteAdapter::MarketDataSubscriptionRead,
        research_operation: None,
    },
    // Reference `market.index_constituents` (`market_capability_tools.go`):
    // CN index member lists are AKShare-only and stay off the public HTTP
    // contract, so this descriptor is the only surface for the capability.
    ProductionToolDefinition {
        id: "market.index_constituents",
        category: "market",
        display_name: "指数成分股",
        adapter: ProductionRouteAdapter::MarketIndexConstituentsRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "watchlist.list",
        category: "watchlist",
        display_name: "查询自选列表",
        adapter: ProductionRouteAdapter::WatchlistRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "research.instrument",
        category: "research",
        display_name: "查询标的信息",
        adapter: ProductionRouteAdapter::ResearchRead,
        research_operation: Some("instrument"),
    },
    ProductionToolDefinition {
        id: "research.financials",
        category: "research",
        display_name: "查询财务数据",
        adapter: ProductionRouteAdapter::ResearchRead,
        research_operation: Some("financials"),
    },
    ProductionToolDefinition {
        id: "research.valuation",
        category: "research",
        display_name: "查询估值数据",
        adapter: ProductionRouteAdapter::ResearchRead,
        research_operation: Some("valuation"),
    },
    ProductionToolDefinition {
        id: "research.news",
        category: "research",
        display_name: "查询研究新闻",
        adapter: ProductionRouteAdapter::ResearchRead,
        research_operation: Some("news"),
    },
    ProductionToolDefinition {
        id: "research.screen",
        category: "research",
        display_name: "执行研究筛选",
        adapter: ProductionRouteAdapter::ResearchScreenWrite,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "account.orders",
        category: "account",
        display_name: "查询订单",
        adapter: ProductionRouteAdapter::ExecutionRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "risk.state",
        category: "risk",
        display_name: "查询风控状态",
        adapter: ProductionRouteAdapter::SystemCore,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "strategy.definitions",
        category: "strategy",
        display_name: "查询策略定义",
        adapter: ProductionRouteAdapter::StrategyDefinitionRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "strategy.validate_pine",
        category: "strategy",
        display_name: "校验 Pine 策略",
        adapter: ProductionRouteAdapter::StrategyPine,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "strategy.research_backtest",
        category: "strategy",
        display_name: "执行策略回测",
        adapter: ProductionRouteAdapter::BacktestStart,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "strategy.optimize",
        category: "strategy",
        display_name: "策略优化",
        adapter: ProductionRouteAdapter::BacktestStart,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "backtest.runs",
        category: "backtest",
        display_name: "查询回测运行",
        adapter: ProductionRouteAdapter::BacktestRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "backtest.result_view",
        category: "backtest",
        display_name: "查询回测结果",
        adapter: ProductionRouteAdapter::BacktestRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "backtest.kline_sync_status",
        category: "backtest",
        display_name: "查询 K 线同步状态",
        adapter: ProductionRouteAdapter::BacktestSyncRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "portfolio.accounts",
        category: "portfolio",
        display_name: "账户发现",
        adapter: ProductionRouteAdapter::PortfolioRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "portfolio.overview",
        category: "portfolio",
        display_name: "组合概览",
        adapter: ProductionRouteAdapter::PortfolioRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "portfolio.positions",
        category: "portfolio",
        display_name: "组合持仓",
        adapter: ProductionRouteAdapter::PortfolioRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "portfolio.summary",
        category: "portfolio",
        display_name: "查询组合摘要",
        adapter: ProductionRouteAdapter::PortfolioRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "market.providers",
        category: "market",
        display_name: "查询行情源列表",
        adapter: ProductionRouteAdapter::MarketDataProviderRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "market.instrument_profile",
        category: "market",
        display_name: "查询标的档案",
        adapter: ProductionRouteAdapter::MarketDataProfileRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "market.ticks",
        category: "market",
        display_name: "查询逐笔成交",
        adapter: ProductionRouteAdapter::MarketDataTicksRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "market.depth",
        category: "market",
        display_name: "查询深度摆盘",
        adapter: ProductionRouteAdapter::MarketDataDepthRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "market.broker_queue",
        category: "market",
        display_name: "查询经纪席位队列",
        adapter: ProductionRouteAdapter::MarketDataBrokerQueueRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "market.capital_flow",
        category: "market",
        display_name: "查询资金流向",
        adapter: ProductionRouteAdapter::MarketDataCapitalFlowRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "broker.cash_flows",
        category: "broker",
        display_name: "查询资金流水",
        adapter: ProductionRouteAdapter::BrokerRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "broker.fees",
        category: "broker",
        display_name: "查询佣金费用",
        adapter: ProductionRouteAdapter::BrokerRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "broker.margin_ratios",
        category: "broker",
        display_name: "查询保证金比率",
        adapter: ProductionRouteAdapter::BrokerRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "broker.orders",
        category: "broker",
        display_name: "查询券商委托",
        adapter: ProductionRouteAdapter::BrokerRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "broker.fills",
        category: "broker",
        display_name: "查询成交明细",
        adapter: ProductionRouteAdapter::BrokerRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "execution.order_events",
        category: "execution",
        display_name: "查询订单事件",
        adapter: ProductionRouteAdapter::ExecutionRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "execution.buying_power",
        category: "execution",
        display_name: "查询购买力",
        adapter: ProductionRouteAdapter::ExecutionWrite,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "watchlist.remote.list",
        category: "watchlist",
        display_name: "查询远端自选列表",
        adapter: ProductionRouteAdapter::RemoteWatchlistRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "strategy.definition_versions.list",
        category: "strategy",
        display_name: "查询策略版本列表",
        adapter: ProductionRouteAdapter::StrategyDefinitionRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "strategy.definition_versions.get",
        category: "strategy",
        display_name: "查询策略版本详情",
        adapter: ProductionRouteAdapter::StrategyDefinitionRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "strategy.instance_activity",
        category: "strategy",
        display_name: "查询策略实例活动",
        adapter: ProductionRouteAdapter::StrategyRuntimeRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "strategy.pine_spec",
        category: "strategy",
        display_name: "查询 Pine 规范",
        adapter: ProductionRouteAdapter::StrategyPine,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "risk.events",
        category: "risk",
        display_name: "查询风控事件",
        adapter: ProductionRouteAdapter::SystemRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "system.runtime_dependencies",
        category: "system",
        display_name: "查询系统运行时依赖",
        adapter: ProductionRouteAdapter::SystemRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "alerts.price.list",
        category: "alerts",
        display_name: "查询价格预警列表",
        adapter: ProductionRouteAdapter::AlertsRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "alerts.option_event.list",
        category: "alerts",
        display_name: "查询期权事件预警列表",
        adapter: ProductionRouteAdapter::AlertsRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "research.institutions",
        category: "research",
        display_name: "查询机构持仓研究",
        adapter: ProductionRouteAdapter::ResearchRead,
        research_operation: Some("institutions"),
    },
    ProductionToolDefinition {
        id: "research.analyst",
        category: "research",
        display_name: "查询分析师评级",
        adapter: ProductionRouteAdapter::ResearchRead,
        research_operation: Some("analyst"),
    },
    ProductionToolDefinition {
        id: "research.ownership",
        category: "research",
        display_name: "查询股权结构",
        adapter: ProductionRouteAdapter::ResearchRead,
        research_operation: Some("ownership"),
    },
    ProductionToolDefinition {
        id: "research.corporate_actions",
        category: "research",
        display_name: "查询公司行动",
        adapter: ProductionRouteAdapter::ResearchRead,
        research_operation: Some("corporate_actions"),
    },
    ProductionToolDefinition {
        id: "research.short_interest",
        category: "research",
        display_name: "查询做空数据",
        adapter: ProductionRouteAdapter::ResearchRead,
        research_operation: Some("short_interest"),
    },
    ProductionToolDefinition {
        id: "research.technical_indicators",
        category: "research",
        display_name: "查询技术指标",
        adapter: ProductionRouteAdapter::ResearchRead,
        research_operation: Some("technical_indicators"),
    },
    ProductionToolDefinition {
        id: "research.rankings",
        category: "research",
        display_name: "查询市场排行",
        adapter: ProductionRouteAdapter::ResearchRead,
        research_operation: Some("rankings"),
    },
    ProductionToolDefinition {
        id: "research.industry",
        category: "research",
        display_name: "查询行业板块",
        adapter: ProductionRouteAdapter::ResearchRead,
        research_operation: Some("industry"),
    },
    ProductionToolDefinition {
        id: "research.calendar",
        category: "research",
        display_name: "查询财经日历",
        adapter: ProductionRouteAdapter::ResearchRead,
        research_operation: Some("calendar"),
    },
    ProductionToolDefinition {
        id: "research.macro",
        category: "research",
        display_name: "查询宏观经济数据",
        adapter: ProductionRouteAdapter::ResearchRead,
        research_operation: Some("macro"),
    },
    ProductionToolDefinition {
        id: "research.screen_catalog",
        category: "research",
        display_name: "查询研究选股条件目录",
        adapter: ProductionRouteAdapter::ResearchCatalog,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "derivatives.futures",
        category: "derivatives",
        display_name: "查询期货信息",
        adapter: ProductionRouteAdapter::MarketDataFuturesRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "derivatives.warrants",
        category: "derivatives",
        display_name: "查询窝轮牛熊证",
        adapter: ProductionRouteAdapter::MarketDataDerivativeRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "derivatives.option_chain",
        category: "derivatives",
        display_name: "查询期权链",
        adapter: ProductionRouteAdapter::MarketDataOptionsChainRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "derivatives.option_analysis",
        category: "derivatives",
        display_name: "查询期权分析",
        adapter: ProductionRouteAdapter::MarketDataOptionsAnalysisRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "derivatives.option_events",
        category: "derivatives",
        display_name: "查询期权异常异动",
        adapter: ProductionRouteAdapter::MarketDataOptionsEventsRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "derivatives.option_screen",
        category: "derivatives",
        display_name: "期权条件选股",
        adapter: ProductionRouteAdapter::MarketDataOptionsScreenRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "prediction.discover",
        category: "prediction",
        display_name: "发现预测合约",
        adapter: ProductionRouteAdapter::MarketDataPredictionRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "prediction.snapshot",
        category: "prediction",
        display_name: "查询预测合约快照",
        adapter: ProductionRouteAdapter::MarketDataPredictionRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "prediction.depth",
        category: "prediction",
        display_name: "查询预测合约深度",
        adapter: ProductionRouteAdapter::MarketDataPredictionRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "prediction.history",
        category: "prediction",
        display_name: "查询预测合约历史",
        adapter: ProductionRouteAdapter::MarketDataPredictionRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "prediction.combo_eligible",
        category: "prediction",
        display_name: "查询组合候选合约",
        adapter: ProductionRouteAdapter::MarketDataPredictionRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "prediction.combo_quote",
        category: "prediction",
        display_name: "获取预测组合报价",
        adapter: ProductionRouteAdapter::MarketDataPredictionCombosWrite,
        research_operation: None,
    },
];

/// Go's `inputRequestToolDescriptor` wire declaration.
///
/// The description carries the blocking-only guidance, the two-or-three option
/// rule and the resume contract, and the schema pins the same budgets the
/// runtime validator enforces (`buildInputRequest`), so the model sees the
/// contract before it calls the tool.  Kept beside the catalog policy so the
/// tool-catalog owner file stays inside the 800-line production budget.
pub(crate) fn input_request_tool_value(name: &str) -> Value {
    json!({
        "type": "function",
        "name": name,
        "description": "向用户提问以解决关键阻塞问题（缺少必要信息、重大取舍、越界授权）。禁止询问可选下一步、是否继续或先看哪部分，也不得替代写操作审批；每题必须提供 2 到 3 个选项 (each question must offer two or three options)，可自由回答时设置 allowOther。用户回答后的工具结果会携带 originalRequest 与 continuationInstruction，必须据此继续完成原始请求。",
        "parameters": {
            "type": "object",
            "properties": {
                "title": {"type": "string", "description": "提问标题"},
                "decisionKind": {
                    "type": "string",
                    "enum": ["missing_required_context", "material_tradeoff", "scope_boundary"],
                    "description": "The genuine blocking boundary. Optional next steps and whether to continue are not blocking decisions."
                },
                "blockingReason": {
                    "type": "string",
                    "minLength": 1,
                    "description": "Why the original task cannot safely continue without this user answer."
                },
                "questions": {
                    "type": "array",
                    "minItems": 1,
                    "description": "All decisions needed for the current step. Ask them together.",
                    "items": {
                        "type": "object",
                        "properties": {
                            "question": {"type": "string"},
                            "options": {
                                "type": "array",
                                "minItems": 2,
                                "maxItems": 3,
                                "description": "Present exactly two or three concise choices.",
                                "items": {
                                    "type": "object",
                                    "properties": {
                                        "label": {"type": "string"},
                                        "description": {"type": "string"},
                                        "recommended": {"type": "boolean"}
                                    },
                                    "required": ["label"]
                                }
                            },
                            "allowOther": {"type": "boolean"}
                        },
                        "required": ["question", "options"]
                    }
                }
            },
            "required": ["decisionKind", "blockingReason", "questions"]
        }
    })
}
