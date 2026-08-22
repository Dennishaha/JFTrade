#[derive(Clone, Copy, Debug, Eq, Hash, PartialEq)]
pub enum AdkReadRoute {
    Snapshot,
    Agents,
    Approvals,
    Audit,
    Memory,
    Metrics,
    OptimizationTasks,
    OptimizationTask,
    Providers,
    Runs,
    Run,
    RunStream,
    Sessions,
    Session,
    SessionContext,
    Skills,
    Stream,
    Tasks,
    Task,
    Tools,
    WorkflowTriggerLogs,
    Workflows,
    Workflow,
    WorkflowTriggers,
}

pub const ADK_READ_ROUTES: [(&str, &str); 24] = [
    ("GET", "/api/v1/adk"),
    ("GET", "/api/v1/adk/agents"),
    ("GET", "/api/v1/adk/approvals"),
    ("GET", "/api/v1/adk/audit"),
    ("GET", "/api/v1/adk/memory"),
    ("GET", "/api/v1/adk/metrics"),
    ("GET", "/api/v1/adk/optimization-tasks"),
    ("GET", "/api/v1/adk/optimization-tasks/{taskId}"),
    ("GET", "/api/v1/adk/providers"),
    ("GET", "/api/v1/adk/runs"),
    ("GET", "/api/v1/adk/runs/{runId}"),
    ("GET", "/api/v1/adk/runs/{runId}/stream"),
    ("GET", "/api/v1/adk/sessions"),
    ("GET", "/api/v1/adk/sessions/{sessionId}"),
    ("GET", "/api/v1/adk/sessions/{sessionId}/context"),
    ("GET", "/api/v1/adk/skills"),
    ("GET", "/api/v1/adk/streams/{streamId}"),
    ("GET", "/api/v1/adk/tasks"),
    ("GET", "/api/v1/adk/tasks/{taskId}"),
    ("GET", "/api/v1/adk/tools"),
    ("GET", "/api/v1/adk/workflow-trigger-logs"),
    ("GET", "/api/v1/adk/workflows"),
    ("GET", "/api/v1/adk/workflows/{workflowId}"),
    ("GET", "/api/v1/adk/workflows/{workflowId}/triggers"),
];

fn product_adk_read_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Vec<RouteSpec> {
    if !ports.adk_read || !capabilities.contains(ProductCapability::AdkRead) {
        return Vec::new();
    }
    ADK_READ_ROUTES
        .iter()
        .map(|(method, path)| route(method, path))
        .collect()
}

pub fn route_for(path: &str) -> Option<AdkReadRoute> {
    match path {
        "/api/v1/adk" => Some(AdkReadRoute::Snapshot),
        "/api/v1/adk/agents" => Some(AdkReadRoute::Agents),
        "/api/v1/adk/approvals" => Some(AdkReadRoute::Approvals),
        "/api/v1/adk/audit" => Some(AdkReadRoute::Audit),
        "/api/v1/adk/memory" => Some(AdkReadRoute::Memory),
        "/api/v1/adk/metrics" => Some(AdkReadRoute::Metrics),
        "/api/v1/adk/optimization-tasks" => Some(AdkReadRoute::OptimizationTasks),
        "/api/v1/adk/providers" => Some(AdkReadRoute::Providers),
        "/api/v1/adk/runs" => Some(AdkReadRoute::Runs),
        "/api/v1/adk/sessions" => Some(AdkReadRoute::Sessions),
        "/api/v1/adk/skills" => Some(AdkReadRoute::Skills),
        "/api/v1/adk/tasks" => Some(AdkReadRoute::Tasks),
        "/api/v1/adk/tools" => Some(AdkReadRoute::Tools),
        "/api/v1/adk/workflow-trigger-logs" => Some(AdkReadRoute::WorkflowTriggerLogs),
        "/api/v1/adk/workflows" => Some(AdkReadRoute::Workflows),
        _ => dynamic_route(path),
    }
}

fn dynamic_route(path: &str) -> Option<AdkReadRoute> {
    if matches_segment(path, "/api/v1/adk/optimization-tasks/", "") {
        return Some(AdkReadRoute::OptimizationTask);
    }
    if matches_segment(path, "/api/v1/adk/runs/", "/stream") {
        return Some(AdkReadRoute::RunStream);
    }
    if matches_segment(path, "/api/v1/adk/runs/", "") {
        return Some(AdkReadRoute::Run);
    }
    if matches_segment(path, "/api/v1/adk/sessions/", "/context") {
        return Some(AdkReadRoute::SessionContext);
    }
    if matches_segment(path, "/api/v1/adk/sessions/", "") {
        return Some(AdkReadRoute::Session);
    }
    if matches_segment(path, "/api/v1/adk/streams/", "") {
        return Some(AdkReadRoute::Stream);
    }
    if matches_segment(path, "/api/v1/adk/tasks/", "") {
        return Some(AdkReadRoute::Task);
    }
    if matches_segment(path, "/api/v1/adk/workflows/", "/triggers") {
        return Some(AdkReadRoute::WorkflowTriggers);
    }
    if matches_segment(path, "/api/v1/adk/workflows/", "") {
        return Some(AdkReadRoute::Workflow);
    }
    None
}

fn matches_segment(path: &str, prefix: &str, suffix: &str) -> bool {
    let Some(value) = path.strip_prefix(prefix) else {
        return false;
    };
    let Some(segment) = value.strip_suffix(suffix) else {
        return false;
    };
    !segment.is_empty() && !segment.contains('/')
}

pub fn operation_name(route: AdkReadRoute) -> &'static str {
    match route {
        AdkReadRoute::Snapshot => "GET /api/v1/adk",
        AdkReadRoute::Agents => "GET /api/v1/adk/agents",
        AdkReadRoute::Approvals => "GET /api/v1/adk/approvals",
        AdkReadRoute::Audit => "GET /api/v1/adk/audit",
        AdkReadRoute::Memory => "GET /api/v1/adk/memory",
        AdkReadRoute::Metrics => "GET /api/v1/adk/metrics",
        AdkReadRoute::OptimizationTasks => "GET /api/v1/adk/optimization-tasks",
        AdkReadRoute::OptimizationTask => "GET /api/v1/adk/optimization-tasks/{taskId}",
        AdkReadRoute::Providers => "GET /api/v1/adk/providers",
        AdkReadRoute::Runs => "GET /api/v1/adk/runs",
        AdkReadRoute::Run => "GET /api/v1/adk/runs/{runId}",
        AdkReadRoute::RunStream => "GET /api/v1/adk/runs/{runId}/stream",
        AdkReadRoute::Sessions => "GET /api/v1/adk/sessions",
        AdkReadRoute::Session => "GET /api/v1/adk/sessions/{sessionId}",
        AdkReadRoute::SessionContext => "GET /api/v1/adk/sessions/{sessionId}/context",
        AdkReadRoute::Skills => "GET /api/v1/adk/skills",
        AdkReadRoute::Stream => "GET /api/v1/adk/streams/{streamId}",
        AdkReadRoute::Tasks => "GET /api/v1/adk/tasks",
        AdkReadRoute::Task => "GET /api/v1/adk/tasks/{taskId}",
        AdkReadRoute::Tools => "GET /api/v1/adk/tools",
        AdkReadRoute::WorkflowTriggerLogs => "GET /api/v1/adk/workflow-trigger-logs",
        AdkReadRoute::Workflows => "GET /api/v1/adk/workflows",
        AdkReadRoute::Workflow => "GET /api/v1/adk/workflows/{workflowId}",
        AdkReadRoute::WorkflowTriggers => "GET /api/v1/adk/workflows/{workflowId}/triggers",
    }
}
