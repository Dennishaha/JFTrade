export interface ArchitectureCard {
  title: string;
  owner: string;
  status: string;
  summary: string;
  bullets: string[];
}

export interface RoadmapPhase {
  key: string;
  title: string;
  target: string;
  summary: string;
}

export interface ConsolePanel {
  name: string;
  state: string;
  description: string;
}

export const architectureCards: ArchitectureCard[] = [
  {
    title: "Broker Gateway",
    owner: "broker-core",
    status: "Phase 2 Active",
    summary:
      "统一承接 Futu / OpenD，并已具备最小 session probe 与账户发现能力。",
    bullets: [
      "显式区分 SIMULATE 与 REAL 环境",
      "中央化处理账户能力与市场能力",
      "原始券商 payload 不上浮到业务层",
    ],
  },
  {
    title: "Execution + Risk",
    owner: "execution / risk-engine",
    status: "Scaffolded",
    summary: "订单状态机与风控门禁会成为 live trading 的核心保护带。",
    bullets: [
      "策略只能产生下单意图，不直接触达券商",
      "所有下单路径先过风控",
      "拒单、撤单、成交都保留审计线索",
    ],
  },
  {
    title: "Data + Strategy Runtime",
    owner: "market-data / strategy-runtime",
    status: "Scaffolded",
    summary:
      "统一实时行情、回放、回测与策略运行接口，降低 live 和 backtest 偏差。",
    bullets: [
      "订阅额度与限频集中治理",
      "策略运行上下文可复用于回测",
      "市场日历与多市场规则进入统一模型",
    ],
  },
];

export const roadmapPhases: RoadmapPhase[] = [
  {
    key: "phase-0",
    title: "Phase 0 / Workspace Scaffold",
    target: "当前已完成",
    summary:
      "完成 monorepo 根配置、Express API、Worker、Vue 控制台和核心 packages 占位。",
  },
  {
    key: "phase-1",
    title: "Phase 1 / Persistence + Infra",
    target: "已完成",
    summary: "持久层、统一错误模型、日志与健康检查已经落地并完成运行验证。",
  },
  {
    key: "phase-2",
    title: "Phase 2 / Futu Minimum Loop",
    target: "当前进行中",
    summary:
      "已接入 OpenD session probe 与账户发现，下一步进入模拟账户下单、撤单与订单同步。",
  },
];

export const consolePanels: ConsolePanel[] = [
  {
    name: "策略台",
    state: "等待实现",
    description: "负责策略实例生命周期、参数管理、运行日志与人工干预。",
  },
  {
    name: "订单台",
    state: "等待实现",
    description: "显示订单状态机、撤改单通道、拒单原因与实时成交。",
  },
  {
    name: "风控看板",
    state: "等待实现",
    description: "展示 kill switch、规则命中、账户限制与环境门禁状态。",
  },
  {
    name: "组合面板",
    state: "等待实现",
    description: "负责持仓、资金、盈亏、多币种估值与对账视图。",
  },
];
