package adk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"sync"
	"time"

	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const localMCPRuntimeStatusURI = "jftrade://runtime/status"

// LocalMCPReadOnlyToolNames is the explicit security boundary for the local
// MCP server. Newly registered ADK tools are intentionally not exposed until
// they are reviewed and added here.
var LocalMCPReadOnlyToolNames = []string{
	"system.status",
	"system.futu_opend",
	"plugins.catalog",
	"market.subscriptions",
	"market.capabilities",
	"market.search",
	"market.instrument_profile",
	"market.snapshot",
	"market.snapshots",
	"market.candles",
	"market.intraday",
	"market.ticks",
	"market.depth",
	"market.broker_queue",
	"market.capital_flow",
	"derivatives.option_chain",
	"derivatives.option_screen",
	"derivatives.option_analysis",
	"derivatives.option_events",
	"derivatives.warrants",
	"derivatives.futures",
	"research.instrument",
	"research.financials",
	"research.valuation",
	"research.analyst",
	"research.ownership",
	"research.corporate_actions",
	"research.short_interest",
	"research.news",
	"research.screen",
	"research.calendar",
	"research.macro",
	"research.rankings",
	"research.institutions",
	"research.industry",
	"research.technical_indicators",
	"prediction.discover",
	"prediction.snapshot",
	"prediction.depth",
	"prediction.history",
	"prediction.combo_eligible",
	"prediction.combo_quote",
	"execution.buying_power",
	"alerts.price.list",
	"alerts.option_event.list",
	"watchlist.remote.list",
	"watchlist.list",
	"portfolio.summary",
	"account.orders",
	"broker.orders",
	"broker.fills",
	"broker.cash_flows",
	"broker.fees",
	"broker.margin_ratios",
	"execution.order_events",
	"risk.state",
	"risk.events",
	"strategy.definitions",
	"strategy.definition_versions.list",
	"strategy.definition_versions.get",
	"strategy.pine_spec",
	"strategy.validate_pine",
	"backtest.runs",
	"backtest.result_view",
	"backtest.kline_sync_status",
}

// LocalMCPHandler bridges the reviewed read-only ADK tools to MCP's Streamable
// HTTP transport and owns the registry listener used for dynamic tool refresh.
type LocalMCPHandler struct {
	http.Handler

	closeOnce   sync.Once
	unsubscribe func()
}

func (h *LocalMCPHandler) Close() {
	if h == nil {
		return
	}
	h.closeOnce.Do(func() {
		if h.unsubscribe != nil {
			h.unsubscribe()
		}
	})
}

// NewLocalMCPHandler keeps authentication and loopback enforcement with the
// listener owner because they are deployment settings, not tool concerns.
func NewLocalMCPHandler(runtime *Runtime) (*LocalMCPHandler, error) {
	if runtime == nil || runtime.Tools() == nil {
		return nil, errors.New("ADK runtime is unavailable")
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "jftrade", Version: "1.0"}, &mcp.ServerOptions{
		Instructions: "JFTrade local read-only market, portfolio, risk, strategy, and backtest tools.",
		Capabilities: &mcp.ServerCapabilities{
			Tools:     &mcp.ToolCapabilities{ListChanged: true},
			Resources: &mcp.ResourceCapabilities{ListChanged: true, Subscribe: true},
		},
		SubscribeHandler:   subscribeLocalMCPRuntimeStatus,
		UnsubscribeHandler: unsubscribeLocalMCPRuntimeStatus,
	})
	bridge := &localMCPBridge{runtime: runtime, server: server, exposed: make(map[string]localMCPRegisteredTool)}
	server.AddResource(&mcp.Resource{
		Name:        "runtime-status",
		Title:       "JFTrade Runtime Status",
		Description: "Sanitized JFTrade assistant runtime status.",
		MIMEType:    "application/json",
		URI:         localMCPRuntimeStatusURI,
	}, bridge.readRuntimeStatus)
	if bridge.syncTools(false) == 0 {
		return nil, errors.New("no reviewed MCP tools are registered")
	}
	unsubscribe := runtime.Tools().OnChange(func() { bridge.syncTools(true) })

	// Leave DisableLocalhostProtection unset so the SDK's DNS rebinding defense
	// remains active even though the listener itself is loopback-only.
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{
		JSONResponse:   true,
		Stateless:      true,
		SessionTimeout: 5 * time.Minute,
	})
	return &LocalMCPHandler{Handler: handler, unsubscribe: unsubscribe}, nil
}

type localMCPBridge struct {
	runtime *Runtime
	server  *mcp.Server

	mu      sync.Mutex
	exposed map[string]localMCPRegisteredTool
}

type localMCPRegisteredTool struct {
	descriptor ToolDescriptor
	revision   uint64
}

type localMCPToolSnapshot struct {
	state      map[string]localMCPRegisteredTool
	registered map[string]RegisteredTool
}

func (b *localMCPBridge) syncTools(notifyResource bool) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	snapshot := localMCPReadOnlyTools(b.runtime.Tools())
	current := snapshot.state
	if !reflect.DeepEqual(b.exposed, current) {
		b.server.RemoveTools(localMCPToolNames(b.exposed)...)
		for _, name := range LocalMCPReadOnlyToolNames {
			if registered, exposed := snapshot.registered[name]; exposed {
				addLocalMCPTool(b.server, b.runtime, registered)
			}
		}
		b.exposed = current
	}
	if notifyResource {
		_ = b.server.ResourceUpdated(context.Background(), &mcp.ResourceUpdatedNotificationParams{URI: localMCPRuntimeStatusURI})
	}
	return len(current)
}

func localMCPReadOnlyTools(registry *ToolRegistry) localMCPToolSnapshot {
	snapshot := localMCPToolSnapshot{
		state:      make(map[string]localMCPRegisteredTool),
		registered: make(map[string]RegisteredTool),
	}
	if registry == nil {
		return snapshot
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	for _, name := range LocalMCPReadOnlyToolNames {
		registered, ok := registry.tools[name]
		if ok && registered.Descriptor.Permission == "read_internal" {
			snapshot.state[name] = localMCPRegisteredTool{descriptor: registered.Descriptor, revision: registered.revision}
			snapshot.registered[name] = registered
		}
	}
	return snapshot
}

func localMCPToolNames(tools map[string]localMCPRegisteredTool) []string {
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	return names
}

func addLocalMCPTool(server *mcp.Server, runtime *Runtime, registered RegisteredTool) {
	descriptor := registered.Descriptor
	inputSchema := descriptor.InputSchema
	if inputSchema == nil {
		inputSchema = map[string]any{"type": "object"}
	}
	mcp.AddTool[map[string]any, any](server, &mcp.Tool{
		Name: descriptor.Name, Title: descriptor.DisplayName, Description: descriptor.Description, InputSchema: inputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, any, error) {
		runtime.RecordAudit(ctx, "mcp.tool.called", descriptor.Name, "local MCP read-only tool call", map[string]any{"transport": "streamable_http"})
		output, err := executeRegisteredTool(ctx, registered, input)
		if err != nil {
			runtime.RecordAudit(ctx, "mcp.tool.failed", descriptor.Name, "local MCP read-only tool call failed", map[string]any{"transport": "streamable_http"})
			return nil, nil, err
		}
		return nil, limitToolOutput(output), nil
	})
}

func (b *localMCPBridge) readRuntimeStatus(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	payload, err := json.Marshal(sanitizedMCPRuntimeStatus(ctx, b.runtime))
	if err != nil {
		return nil, err
	}
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
		URI: localMCPRuntimeStatusURI, MIMEType: "application/json", Text: string(payload),
	}}}, nil
}

func sanitizedMCPRuntimeStatus(ctx context.Context, runtime *Runtime) map[string]any {
	status := map[string]any{
		"storeConfigured": runtime.Store() != nil,
		"tools":           sanitizedMCPToolDescriptors(runtime.Tools().List()),
	}
	if runtime.Store() == nil {
		status["providers"] = []any{}
		status["agents"] = []any{}
		status["skills"] = []any{}
		return status
	}
	snapshot, err := runtime.Snapshot(ctx)
	if err != nil {
		status["snapshotError"] = "runtime snapshot unavailable"
		return status
	}
	status["providers"] = sanitizedMCPProviders(snapshot.Providers)
	status["agents"] = sanitizedMCPAgents(snapshot.Agents)
	status["skills"] = sanitizedMCPSkills(snapshot.Skills)
	return status
}

func sanitizedMCPToolDescriptors(tools []ToolDescriptor) []map[string]any {
	items := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		items = append(items, map[string]any{"name": tool.Name, "displayName": tool.DisplayName, "category": tool.Category, "permission": tool.Permission, "riskLevel": tool.RiskLevel})
	}
	return items
}

func sanitizedMCPProviders(providers []Provider) []map[string]any {
	items := make([]map[string]any, 0, len(providers))
	for _, provider := range providers {
		items = append(items, map[string]any{"id": provider.ID, "displayName": provider.DisplayName, "model": provider.Model, "enabled": provider.Enabled, "default": provider.Default, "hasApiKey": provider.HasAPIKey, "capabilities": provider.Capabilities})
	}
	return items
}

func sanitizedMCPAgents(agents []Agent) []map[string]any {
	items := make([]map[string]any, 0, len(agents))
	for _, agent := range agents {
		items = append(items, map[string]any{"id": agent.ID, "name": agent.Name, "providerId": agent.ProviderID, "model": agent.Model, "tools": agent.Tools, "toolAccessMode": assistantmodel.NormalizeToolAccessMode(agent.ToolAccessMode, agent.Tools), "skills": agent.Skills, "permissionMode": agent.PermissionMode, "status": agent.Status, "builtin": agent.Builtin})
	}
	return items
}

func sanitizedMCPSkills(skills []Skill) []map[string]any {
	items := make([]map[string]any, 0, len(skills))
	for _, skill := range skills {
		items = append(items, map[string]any{"id": skill.ID, "displayName": skill.DisplayName, "description": skill.Description, "source": skill.Source, "enabled": skill.Enabled, "builtin": skill.Builtin, "tools": skill.Tools, "version": skill.Version, "validationStatus": skill.ValidationStatus, "validationError": skill.ValidationError})
	}
	return items
}

func subscribeLocalMCPRuntimeStatus(_ context.Context, request *mcp.SubscribeRequest) error {
	if request == nil || request.Params == nil {
		return mcp.ResourceNotFoundError("")
	}
	return validateLocalMCPRuntimeStatusURI(request.Params.URI)
}

func unsubscribeLocalMCPRuntimeStatus(_ context.Context, request *mcp.UnsubscribeRequest) error {
	if request == nil || request.Params == nil {
		return mcp.ResourceNotFoundError("")
	}
	return validateLocalMCPRuntimeStatusURI(request.Params.URI)
}

func validateLocalMCPRuntimeStatusURI(uri string) error {
	if uri != localMCPRuntimeStatusURI {
		return mcp.ResourceNotFoundError(uri)
	}
	return nil
}
