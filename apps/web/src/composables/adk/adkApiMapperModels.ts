export interface ADKPageEnvelope {
  limit: number;
  offset: number;
  total: number;
  returned: number;
  hasMore: boolean;
}

export interface ADKMetricsView {
  runs: {
    total: number;
    last7Days: number;
    byStatus: Record<string, number>;
    byAgent: Record<string, number>;
    byProvider: Record<string, number>;
    lifecycle: {
      failed: number;
      timedOut: number;
      cancelled: number;
      resumed: number;
      orphaned: number;
    };
  };
  tools: {
    total: number;
    successful: number;
    averageDurationMs: number;
    byName: Record<string, number>;
    byStatus: Record<string, number>;
  };
  approvals: {
    pending: number;
    total: number;
    last7Days: number;
    approved: number;
    denied: number;
    recoverablePending: number;
    pendingWaitMs: { average: number; max: number };
    resolutionWaitMs: { average: number; max: number; count: number };
  };
  usage: {
    samples: number;
    tokensInTotal: number | null;
    tokensOutTotal: number | null;
    tokensInAverage: number | null;
    tokensOutAverage: number | null;
  };
  sessions: {
    total: number;
    last7Days: number;
  };
  workflows: {
    definitions: number;
    enabledDefinitions: number;
    triggers: number;
    enabledTriggers: number;
    invocations: number;
    invocationsLast7Days: number;
    byStatus: Record<string, number>;
    byTriggerType: Record<string, number>;
  };
  measurementWindow: {
    days: number;
    since: string;
  };
}


